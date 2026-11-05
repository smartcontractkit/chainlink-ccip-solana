use crate::messages::FinalityConfig;
use crate::types::ToTxData;
use crate::v2::{MessageDirection, ProtocolAmount};
use crate::CommonCcipError;
use anchor_discriminator::anchor_discriminator;
use anchor_lang::prelude::*;
use anchor_lang::solana_program::{
    instruction::{AccountMeta, Instruction},
    program::invoke,
};

pub const GET_REQUIRED_CCVS: [u8; 8] = anchor_discriminator!("get_required_ccvs");

/// Reads CPI return data, verifies it originated from `expected_program`, and deserializes it,
/// reporting failures as [`CommonCcipError`]. Every CCIP CPI that consumes return data MUST use
/// this helper or [`crate::cpi::get_verified_cpi_return_data`] (see INV-SOL-CPI-RETURN).
///
/// Mirrors Anchor's own CPI return handling:
/// https://github.com/coral-xyz/anchor/blob/0109f4a3cf4117570f627e3ae465b6247d504f69/lang/syn/src/codegen/program/cpi.rs#L83
pub fn parse_verified_cpi_return<T: AnchorDeserialize>(expected_program: Pubkey) -> Result<T> {
    let data = crate::cpi::get_verified_cpi_return_data(
        expected_program,
        CommonCcipError::InvalidCpiReturnProgram,
    )?;
    T::try_from_slice(&data).map_err(Into::into)
}

// Resolution-time mirror of the `ccip_send_v2` pool CPI: queries the token pool for its required
// CCVs so the final list (and thus the resolved accounts) matches execution. Reads the pool
// accounts from the slice in the order produced by `pool_ccv_resolution_accounts`:
// [pool_program, pool_config, pool_chain_config, ...pool-resolved extra accounts].
pub fn get_pool_required_ccvs_cpi<'info>(
    pool_accounts: &[AccountInfo<'info>],
    amount: ProtocolAmount,
    local_token: Pubkey,
    remote_chain_selector: u64,
    requested_finality_config: FinalityConfig,
    direction: MessageDirection,
) -> Result<Vec<Pubkey>> {
    require_gte!(
        pool_accounts.len(),
        4,
        CommonCcipError::InvalidAccountListLengths
    );
    let pool_program = &pool_accounts[0];
    let pool_config = &pool_accounts[1];
    let pool_chain_config = &pool_accounts[2];
    let chain_config_v2 = &pool_accounts[3];

    let ix = Instruction {
        program_id: pool_program.key(),
        accounts: vec![
            AccountMeta::new_readonly(pool_config.key(), false),
            AccountMeta::new_readonly(pool_chain_config.key(), false),
            AccountMeta::new_readonly(chain_config_v2.key(), false),
        ],
        data: GetRequiredCCVsIn {
            remote_chain_selector,
            amount,
            local_token,
            direction,
            requested_finality_config,
        }
        .to_tx_data(),
    };

    let account_infos = [
        pool_config.clone(),
        pool_chain_config.clone(),
        chain_config_v2.clone(),
    ];
    invoke(&ix, &account_infos)?;

    let response: GetRequiredCCVsOut = parse_verified_cpi_return(pool_program.key())?;

    Ok(response.required_ccvs)
}

// CCV (Cross-Chain Verifier) types

#[derive(Clone, AnchorSerialize, AnchorDeserialize, Debug)]
pub struct GetRequiredCCVsIn {
    pub remote_chain_selector: u64,

    /// Token amount used to determine the required CCVs.
    ///
    /// For inbound transfers, the amount is denominated in the remote token's decimals
    /// and must be converted to local token decimals before applying amount-based CCV
    /// policies. For outbound transfers, the amount is already denominated in local
    /// token decimals, but is the gross amount before the pool's in-token BPS fee: the
    /// pool deducts that fee before applying amount-based CCV policies, as it only sends
    /// the post-fee remainder cross-chain. Inbound amounts are already post-fee.
    pub amount: ProtocolAmount,

    pub local_token: Pubkey,
    pub direction: MessageDirection,
    pub requested_finality_config: FinalityConfig,
}

impl ToTxData for GetRequiredCCVsIn {
    fn to_tx_data(&self) -> Vec<u8> {
        let mut data = Vec::with_capacity(8);
        data.extend_from_slice(&GET_REQUIRED_CCVS);
        borsh::to_writer(&mut data, &self).unwrap();
        data
    }
}

#[derive(Clone, AnchorSerialize, AnchorDeserialize, Debug, Default)]
pub struct GetRequiredCCVsOut {
    pub required_ccvs: Vec<Pubkey>,
}

// Receiver CCV interface (CCIP v2). Mirrors EVM's
// `IAny2EVMMessageReceiverV2.getCCVsAndFinalityConfig(sourceChainSelector, sender)`.
//
// These types are the compatibility contract between a v2 receiver program (which implements the
// `get_ccvs_and_finality_config` instruction) and the OffRamp that queries it (see
// `get_receiver_ccvs_cpi` below). They live here in `ccip-common` so both sides reference the same
// wire format and can't drift.
pub const GET_CCVS_AND_FINALITY_CONFIG: [u8; 8] =
    anchor_discriminator!("get_ccvs_and_finality_config");

/// Anchor discriminator of a v2 receiver's execution entrypoint. Distinct from the v1
/// `ccip_receive` discriminator, so the OffRamp can dispatch to the correct entrypoint based on the
/// receiver's interface version.
pub const CCIP_RECEIVE_V2: [u8; 8] = anchor_discriminator!("ccip_receive_v2");

/// PDA seed for a v2 receiver's per-source-chain CCV/finality config account. Part of the v2
/// receiver interface contract: the receiver stores its config at
/// `[RECEIVER_CCV_CONFIG_SEED, source_chain_selector.to_le_bytes()]` under its own program, and the
/// OffRamp derives the same address to pass into `get_receiver_ccvs_cpi`.
pub const RECEIVER_CCV_CONFIG_SEED: &[u8] = b"ccv_config";

#[derive(Clone, AnchorSerialize, AnchorDeserialize, Debug)]
pub struct GetReceiverCcvsIn {
    pub source_chain_selector: u64,
    /// Cross-chain sender address, provided so receivers can implement sender-specific policies.
    pub sender: Vec<u8>,
}

impl ToTxData for GetReceiverCcvsIn {
    fn to_tx_data(&self) -> Vec<u8> {
        let mut data = Vec::with_capacity(8);
        data.extend_from_slice(&GET_CCVS_AND_FINALITY_CONFIG);
        borsh::to_writer(&mut data, &self).unwrap();
        data
    }
}

/// Receiver's CCV + finality requirements for a given source chain.
///
/// Placeholder semantics (mirrors EVM `address(0)` via [`crate::INCLUDE_DEFAULT_CCVS`]):
/// an empty `required_ccvs`/`optional_ccvs` with `optional_threshold == 0` means "use the lane
/// default CCVs". An `INCLUDE_DEFAULT_CCVS` entry inside `required_ccvs` is the explicit "also
/// include the lane defaults" marker. `INCLUDE_DEFAULT_CCVS` is not permitted in `optional_ccvs`.
#[derive(Clone, AnchorSerialize, AnchorDeserialize, Debug, Default)]
pub struct GetReceiverCcvsOut {
    pub required_ccvs: Vec<Pubkey>,
    pub optional_ccvs: Vec<Pubkey>,
    pub optional_threshold: u8,
    pub allowed_finality_config: FinalityConfig,
}

/// Queries a v2 receiver's `get_ccvs_and_finality_config` via CPI and returns its response.
/// Resolution-time and execution-time mirror of the receiver CCV interface, analogous to
/// [`get_pool_required_ccvs_cpi`] for token pools.
///
/// `accounts` is a dynamic remaining-account slice (Anchor cannot validate its contents), laid out
/// as `[receiver_program, source_chain_ccv_config]`. Because it is executor-supplied, this helper
/// validates the CPI *target* against `expected_receiver` (the message's authoritative receiver) —
/// otherwise a malicious executor could point the query at a program that under-reports the
/// receiver's required CCVs, downgrading verification. The config PDA is not re-derived here: the
/// receiver's own `get_ccvs_and_finality_config` account context PDA-checks it (a wrong PDA fails
/// inside the CPI). See INV-CPI-ACCOUNTS. The config PDA may be uninitialized; the receiver returns
/// an empty response in that case (see the receiver's `try_load`).
pub fn get_receiver_ccvs_cpi<'info>(
    accounts: &[AccountInfo<'info>],
    expected_receiver: Pubkey,
    source_chain_selector: u64,
    sender: Vec<u8>,
) -> Result<GetReceiverCcvsOut> {
    require_gte!(
        accounts.len(),
        2,
        CommonCcipError::InvalidAccountListLengths
    );
    let receiver_program = &accounts[0];
    let source_chain_ccv_config = &accounts[1];

    // The receiver program is executor-supplied; bind it to the message's receiver so the query
    // can't be redirected to an attacker-controlled program.
    require_keys_eq!(
        receiver_program.key(),
        expected_receiver,
        CommonCcipError::InvalidCpiTargetProgram,
    );

    let ix = Instruction {
        program_id: receiver_program.key(),
        accounts: vec![AccountMeta::new_readonly(
            source_chain_ccv_config.key(),
            false,
        )],
        data: GetReceiverCcvsIn {
            source_chain_selector,
            sender,
        }
        .to_tx_data(),
    };

    invoke(&ix, std::slice::from_ref(source_chain_ccv_config))?;

    parse_verified_cpi_return(receiver_program.key())
}
