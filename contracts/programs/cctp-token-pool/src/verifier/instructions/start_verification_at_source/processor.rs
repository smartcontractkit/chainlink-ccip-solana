use anchor_lang::prelude::instruction::Instruction;
use anchor_lang::prelude::program::invoke_signed;
use anchor_lang::prelude::*;
use anchor_spl::token_interface::{Mint, TokenAccount, TokenInterface};

use ccip_common::messages::{MessageV1, SVMTokenAmount};
use ccip_common::resolution::find;
use ccip_common::seed::CCV_SIGNER;

use crate::error::CctpPoolVerifierError;
use crate::verifier::cctp_constants::*;
use crate::verifier::instructions::utils;
use crate::verifier::state::*;

#[derive(AnchorSerialize, AnchorDeserialize, Debug)]
pub struct StartVerificationAtSourceParams {
    pub message: MessageV1,
    pub message_id: [u8; 32],
    pub fee_token: SVMTokenAmount,
    pub verifier_args: Vec<u8>,
}

/// Called on the sending side as a verifier hook during message sending.
pub fn start_verification_at_source(
    ctx: Context<StartVerificationAtSource>,
    params: StartVerificationAtSourceParams,
) -> Result<Vec<u8>> {
    let cctp_configs = &ctx.accounts.remote_chain_config.cctp;

    require!(cctp_configs.enabled, CctpPoolVerifierError::UnknownDomain);

    validate_sender_allowlist(&ctx, &params)?;

    let nonce = get_and_increase_nonce(&ctx, &params)?;
    let event_bump = validate_event_pda(&ctx, &params.message, nonce)?;

    // CCTP requires a cctp-enabled token being bridged (e.g. USDC)
    let token_transfer = params
        .message
        .token_transfer
        .as_ref()
        .ok_or(CctpPoolVerifierError::MissingTokenTransfer)?;

    validate_source_pool_token_and_mint(&ctx, token_transfer)?;

    let mint_recipient = if let Some(recipient) = cctp_configs.mint_recipient_on_dest {
        recipient
    } else {
        // CCTP limits the address to 32 bytes, see
        // https://developers.circle.com/cctp/references/technical-guide#message-body
        utils::left_padded_pubkey(token_transfer.token_receiver.as_slice())?
    };

    let amount: u64 = token_transfer
        .amount
        .try_into()
        .map_err(|_| CctpPoolVerifierError::InvalidTransferAmount)?;

    let (max_fee, min_finality_threshold) = process_finality_requirements(&ctx, &params, amount)?;

    // The hook data includes the version tag and the message ID.
    // The version tag allows the destination verifier entity to route the message to the correct implementation.
    // Inclusion of the message ID ensures that the contents of the CCIP message can't be tampered with on destination.
    let mut hook_data = Vec::with_capacity(VERSION_TAG.len() + params.message_id.len());
    hook_data.extend_from_slice(&VERSION_TAG);
    hook_data.extend_from_slice(&params.message_id);

    deposit_for_burn_with_hook(
        &ctx,
        DepositForBurnWithHookParams {
            amount,
            destination_domain: cctp_configs.domain_identifier,
            mint_recipient,
            destination_caller: cctp_configs.allowed_caller_receive,
            max_fee,
            min_finality_threshold,
            hook_data,
        },
        &params.message,
        nonce,
        event_bump,
    )?;

    // We do not return the verifier version here.
    // Offchain verifier is expected to pull verifier version from the hook data & prefix the verifierResults with it.
    Ok(Vec::new())
}

/// The only account list the router resolves for this verifier; [`GetFee`] must present the same
/// one, since the router reuses this resolution for its `get_fee` CPI. See [`GetFee`] for why the
/// two are duplicated rather than macro-generated, and
/// `account_lists_match_start_verification_at_source` in `instructions/mod.rs` for the guard that
/// enforces it.
#[derive(Accounts)]
#[instruction(params: StartVerificationAtSourceParams)]
pub struct StartVerificationAtSource<'info> {
    #[account(
        seeds = [verifier_seed::CONFIG],
        bump = config.bump,
        constraint = config.is_latest_version() @ CctpPoolVerifierError::InvalidVersion,
    )]
    pub config: Box<Account<'info, Config>>,

    #[account(
        seeds = [CCV_SIGNER, crate::ID.as_ref()],
        bump,
        seeds::program = remote_chain_config.admin_config.router,
    )]
    pub authority: Signer<'info>,

    #[account(
        seeds = [verifier_seed::REMOTE_CHAIN, &params.message.dest_chain_selector.to_le_bytes()],
        bump = remote_chain_config.bump,
        constraint = remote_chain_config.is_latest_version() @ CctpPoolVerifierError::InvalidVersion,
    )]
    pub remote_chain_config: Box<Account<'info, RemoteChainConfig>>,

    #[account(
        mut,
        owner = token_program.key(),
        // consistency with the mint in the message is checked in the instruction body
    )]
    pub mint: InterfaceAccount<'info, Mint>,

    pub token_program: Interface<'info, TokenInterface>,

    /// CHECK: CPI signer. This account is intentionally not initialized, and it will
    /// hold a balance to pay for the rent of initializing the CCTP MessageSentEvent account.
    /// This is intentionally reusing the signer from the pool module of this same contract, so as
    /// to keep SOL on a single PDA and a single owner across CCTPv1 & CCTPv2 event accounts.
    #[account(
        mut,
        seeds = [verifier_seed::POOL_SIGNER_SEED, mint.key().as_ref()],
        bump,
    )]
    pub cpi_signer: UncheckedAccount<'info>,

    #[cfg(feature = "idl-build")]
    // the Nonce account is an UncheckedAccount in the context, so it isn't automatically in the IDL. By leveraging
    // the #[cfg(feature = "idl-build")] attribute, we can include it in the IDL and bindings as an account,
    // while still keeping it as an UncheckedAccount in the actual program.
    #[account(
        mut,
        seeds = [
            verifier_seed::CCTPV2_NONCE,
            &params.message.sender,
            &params.message.dest_chain_selector.to_le_bytes(),
            mint.key().as_ref()
        ],
        bump,
    )]
    pub nonce: Account<'info, Nonce>,
    #[cfg(not(feature = "idl-build"))]
    /// CHECK: used to store a nonce counter for the user/chain/mint tuple, which is then used as a seed
    /// to derive the CCTP MessageSentEvent account. Managed as an `UncheckedAccount` because the initialization
    /// (when needed) is paid by this contract, and not by the RouterSigner that invokes this method.
    #[account(
        mut,
        seeds = [
            verifier_seed::CCTPV2_NONCE,
            &params.message.sender,
            &params.message.dest_chain_selector.to_le_bytes(),
            mint.key().as_ref()
        ],
        bump,
    )]
    pub nonce: UncheckedAccount<'info>,

    /// CHECK this is the account in which CCTP will store the event. It is not a PDA of CCTP,
    /// but CCTP will initialize it and become the owner for it. It's address is checked in the instruction body.
    #[account(
        mut,
        // the address will be checked in the instruction body as it requires figuring out the nonce first
        owner = System::id(), // this is not initialized yet, it will later be owned by CCTP
    )]
    pub cctp_message_sent_event: UncheckedAccount<'info>,

    #[account(
        mut,
        associated_token::mint = mint,
        associated_token::authority = cpi_signer,
        associated_token::token_program = token_program,
    )]
    pub burn_token_account: Box<InterfaceAccount<'info, TokenAccount>>,

    pub system_program: Program<'info, System>,

    // ----------
    // CCTP accounts for deposit_for_burn_with_hook CPI
    // ----------
    /// CHECK: Forwarded to CCTP and checked there
    pub sender_authority_pda: UncheckedAccount<'info>,

    /// CHECK: Forwarded to CCTP and checked there
    pub denylist_account: UncheckedAccount<'info>,

    /// CHECK: Forwarded to CCTP and checked there
    pub message_transmitter: UncheckedAccount<'info>,

    /// CHECK: Forwarded to CCTP and checked there
    pub token_messenger: UncheckedAccount<'info>,

    /// CHECK: Forwarded to CCTP and checked there
    pub remote_token_messenger: UncheckedAccount<'info>,

    /// CHECK: Forwarded to CCTP and checked there
    pub token_minter: UncheckedAccount<'info>,

    /// CHECK: Forwarded to CCTP and checked there
    pub local_token: UncheckedAccount<'info>,

    /// CHECK: Forwarded to CCTP and checked there
    pub token_messenger_minter_event_authority: UncheckedAccount<'info>,

    /// CHECK: Forwarded to CCTP and checked there
    pub message_transmitter_event_authority: UncheckedAccount<'info>,

    #[account(address = MESSAGE_TRANSMITTER_V2)]
    pub message_transmitter_program: UncheckedAccount<'info>,

    #[account(address = TOKEN_MESSENGER_MINTER_V2)]
    pub token_messenger_minter_program: UncheckedAccount<'info>,
}

#[repr(C)]
#[derive(InitSpace, AnchorSerialize, AnchorDeserialize, Clone)]
// Copied from https://github.com/circlefin/solana-cctp-contracts/blob/ec16e95d28ee47f7832df4203ae07b5981d146fc/programs/v2/token-messenger-minter-v2/src/token_messenger_v2/instructions/deposit_for_burn_with_hook.rs#L34-L45
pub struct DepositForBurnWithHookParams {
    pub amount: u64,
    pub destination_domain: u32,
    pub mint_recipient: Pubkey,
    pub destination_caller: Pubkey,
    pub max_fee: u64,
    pub min_finality_threshold: u32,
    #[max_len(0)]
    pub hook_data: Vec<u8>,
}

impl DepositForBurnWithHookParams {
    pub fn len(&self) -> usize {
        Self::INIT_SPACE + self.hook_data.len()
    }
}

fn validate_sender_allowlist(
    ctx: &Context<'_, StartVerificationAtSource<'_>>,
    params: &StartVerificationAtSourceParams,
) -> Result<()> {
    let remote_chain_config = &ctx.accounts.remote_chain_config;

    if !remote_chain_config.allowlist_config.allowlist_enabled {
        return Ok(());
    }

    let sender = Pubkey::try_from(params.message.sender.as_slice())
        .map_err(|_| CctpPoolVerifierError::InvalidInstructionData)?;

    require!(
        remote_chain_config
            .allowlist_config
            .allowed_senders
            .contains(&sender),
        CctpPoolVerifierError::SenderNotAllowed
    );

    Ok(())
}

fn validate_source_pool_token_and_mint(
    ctx: &Context<'_, StartVerificationAtSource<'_>>,
    token_transfer: &ccip_common::messages::TokenTransferV1,
) -> Result<()> {
    let source_token_pubkey = Pubkey::try_from(token_transfer.source_token_address.as_slice())
        .map_err(|_| CctpPoolVerifierError::InvalidInstructionData)?;

    require_keys_eq!(
        source_token_pubkey,
        ctx.accounts.mint.key(),
        CctpPoolVerifierError::InvalidInstructionData
    );

    require!(
        ctx.accounts
            .remote_chain_config
            .cctp
            .supported_tokens
            .iter()
            .any(|pair| pair.matches(&source_token_pubkey, &token_transfer.dest_token_address)),
        CctpPoolVerifierError::UnsupportedTokenForDomain
    );

    // Check that the source pool is the one from this program, as that ensures that the funds managed in
    // pool_token_account are indeed fully controlled by this program and we don't burn funds that were actually
    // accrued as older bps fees
    let source_pool_pubkey = Pubkey::try_from(token_transfer.source_pool_address.as_slice())
        .map_err(|_| CctpPoolVerifierError::InvalidInstructionData)?;
    let pool_config_pda = find(
        &[
            base_token_pool::constants::POOL_STATE_SEED,
            source_token_pubkey.as_ref(),
        ],
        crate::ID,
    );
    require_keys_eq!(
        source_pool_pubkey,
        pool_config_pda,
        CctpPoolVerifierError::UnexpectedSourcePool
    );

    Ok(())
}

fn validate_event_pda(
    ctx: &Context<'_, StartVerificationAtSource<'_>>,
    message: &MessageV1,
    nonce: u64,
) -> Result<u8> {
    let (expected_event_pda, bump) = Pubkey::find_program_address(
        &[
            verifier_seed::CCTPV2_EVENT,
            &message.sender,
            &message.dest_chain_selector.to_le_bytes(),
            &ctx.accounts.mint.key().to_bytes(),
            &nonce.to_le_bytes(),
        ],
        ctx.program_id,
    );

    require_keys_eq!(
        expected_event_pda,
        ctx.accounts.cctp_message_sent_event.key(),
        CctpPoolVerifierError::WrongEventAccount
    );

    Ok(bump)
}

pub fn get_and_increase_nonce(
    ctx: &Context<'_, StartVerificationAtSource<'_>>,
    params: &StartVerificationAtSourceParams,
) -> Result<u64> {
    let account_info = &ctx.accounts.nonce.to_account_info();

    let mut nonce_pda: Nonce = match *account_info.owner {
        // Account not initialized yet: initialize it
        system_program::ID => {
            let mint_key = ctx.accounts.mint.key();

            let cpi_signer_seeds: &[&[u8]] = &[
                verifier_seed::POOL_SIGNER_SEED,
                mint_key.as_ref(),
                &[ctx.bumps.cpi_signer],
            ];
            let nonce_seeds: &[&[u8]] = &[
                verifier_seed::CCTPV2_NONCE,
                &params.message.sender,
                &params.message.dest_chain_selector.to_le_bytes(),
                mint_key.as_ref(),
                &[ctx.bumps.nonce],
            ];
            let signer_seeds = [cpi_signer_seeds, nonce_seeds];

            ccip_common::account_management::initialize_from_pda(
                account_info,
                &ctx.accounts.system_program,
                &ctx.accounts.cpi_signer.to_account_info(),
                Nonce::DISCRIMINATOR.len() + Nonce::INIT_SPACE,
                &signer_seeds,
                &crate::ID,
            )?;

            Nonce {
                version: Nonce::MAX_V,
                bump: ctx.bumps.nonce,
                sender: Pubkey::try_from(params.message.sender.as_slice())
                    .map_err(|_| CctpPoolVerifierError::InvalidInstructionData)?,
                mint: ctx.accounts.mint.key(),
                remote_chain_selector: params.message.dest_chain_selector,
                nonce: 0,
            }
        }
        // Already initialized: load it
        crate::ID_CONST => {
            let data = account_info
                .try_borrow_data()
                .map_err(|_| CctpPoolVerifierError::InvalidNonceAccountData)?;

            let nonce_pda = Nonce::try_deserialize(&mut &data[..])
                .map_err(|_| CctpPoolVerifierError::InvalidNonceAccountData)?;

            require!(
                nonce_pda.is_latest_version(),
                CctpPoolVerifierError::InvalidVersion
            );

            nonce_pda
        }
        // Any other case (not valid)
        _ => {
            return Err(CctpPoolVerifierError::InvalidNoncePDAOwner.into());
        }
    };

    let incremented_nonce = nonce_pda
        .nonce
        .checked_add(1)
        .ok_or(CctpPoolVerifierError::InvalidNonceAccountData)?;
    nonce_pda.nonce = incremented_nonce;

    // Persist it back onchain
    nonce_pda.try_serialize(&mut &mut account_info.try_borrow_mut_data()?[..])?;

    msg!("Using nonce: {}", incremented_nonce);

    Ok(incremented_nonce)
}

fn process_finality_requirements(
    ctx: &Context<'_, StartVerificationAtSource<'_>>,
    params: &StartVerificationAtSourceParams,
    transfer_amount: u64,
) -> Result<(u64, u32)> {
    if params.message.finality.is_wait_for_finality() {
        return Ok((0, CCTP_STANDARD_FINALITY_THRESHOLD));
    }

    if params.verifier_args.is_empty() {
        // If no verifierArgs are provided, we compute the max fee according to the bps stored on this contract.
        // The bps values stored on this contract should be kept in-sync with those used by CCTP.
        // If out of sync, the following scenarios are possible:
        // - If stored bps < actual bps, the user just gets a standard, free transfer.
        // - If stored bps > actual bps, the user pays less on destination than they were expecting to.
        // Neither scenario results in a user paying more than they were expecting to.
        // We also add `+1` to the maxFee just to avoid stalling due to truncation in the math. Circle will
        // only bill a fee up to the max, so having a tiny buffer doesn't increase the actual fee billed.
        // If we want to do a bigger buffer on the maxFee, we can increase the fast_finality_bps config
        // (though precision limits us to an integer number of bps).
        let max_fee = ((transfer_amount as u128) * (ctx.accounts.config.fast_finality_bps as u128))
            .saturating_div(BPS_DIVIDER as u128)
            .saturating_add(1)
            .try_into()
            .inspect_err(|_| {
                // Cannot happen unless the `fast_finality_bps` is ever set above the BPS_DIVIDER scaling factor,
                // which based on Circle's fees for Solana is not expected to ever happen. We also validate it
                // when configuring the fast_finality_bps, so this is just a sanity check.
                msg!("Math error computing the CCTP max fee");
            })?;

        return Ok((max_fee, CCTP_FAST_FINALITY_THRESHOLD));
    }

    // We interpret verifierArgs as the max fee.
    // CCTP defines bps offchain, so computing a max fee based on the API and inputting it into ccipSend
    // is the best way to ensure that your max fee aligns with what CCTP will charge for your transfer.
    require_eq!(
        params.verifier_args.len(),
        8,
        CctpPoolVerifierError::InvalidVerifierArgs
    );
    let max_fee =
        u64::from_le_bytes(params.verifier_args.as_slice().try_into().expect(
            "verifier args must be 8 bytes, and is covered by previous `require_eq!` check",
        ));

    Ok((max_fee, CCTP_FAST_FINALITY_THRESHOLD))
}

fn deposit_for_burn_with_hook(
    ctx: &Context<'_, StartVerificationAtSource<'_>>,
    burn_params: DepositForBurnWithHookParams,
    message: &MessageV1,
    nonce: u64,
    event_bump: u8,
) -> Result<()> {
    let mut ix_data =
        Vec::with_capacity(DEPOSIT_FOR_BURN_WITH_HOOK_DISCRIMINATOR.len() + burn_params.len());

    ix_data.extend_from_slice(&DEPOSIT_FOR_BURN_WITH_HOOK_DISCRIMINATOR);
    borsh::to_writer(&mut ix_data, &burn_params)?;

    let acc_infos = [
        ctx.accounts.cpi_signer.to_account_info(),
        ctx.accounts.cpi_signer.to_account_info(),
        ctx.accounts.sender_authority_pda.to_account_info(),
        ctx.accounts.burn_token_account.to_account_info(),
        ctx.accounts.denylist_account.to_account_info(),
        ctx.accounts.message_transmitter.to_account_info(),
        ctx.accounts.token_messenger.to_account_info(),
        ctx.accounts.remote_token_messenger.to_account_info(),
        ctx.accounts.token_minter.to_account_info(),
        ctx.accounts.local_token.to_account_info(),
        ctx.accounts.mint.to_account_info(),
        ctx.accounts.cctp_message_sent_event.to_account_info(),
        ctx.accounts.message_transmitter_program.to_account_info(),
        ctx.accounts
            .token_messenger_minter_program
            .to_account_info(),
        ctx.accounts.token_program.to_account_info(),
        ctx.accounts.system_program.to_account_info(),
        // additional accounts required for the emit_cpi! macros inside CCTP contracts
        // (the program ones are duplicates, they do need to be repeated in the account_metas list,
        // and it is idempotent to repeat them in the account_infos list anyway)
        ctx.accounts
            .token_messenger_minter_event_authority
            .to_account_info(),
        ctx.accounts
            .token_messenger_minter_program
            .to_account_info(),
        ctx.accounts
            .message_transmitter_event_authority
            .to_account_info(),
        ctx.accounts.message_transmitter_program.to_account_info(),
    ];

    let signer_keys = [
        ctx.accounts.cpi_signer.key(),
        ctx.accounts.cctp_message_sent_event.key(),
    ];

    let cpi_signer_seeds: &[&[u8]] = &[
        verifier_seed::POOL_SIGNER_SEED,
        &ctx.accounts.mint.key().to_bytes(),
        &[ctx.bumps.cpi_signer],
    ];

    let cctp_message_sent_event_seeds: &[&[u8]] = &[
        verifier_seed::CCTPV2_EVENT,
        &message.sender,
        &message.dest_chain_selector.to_le_bytes(),
        &ctx.accounts.mint.key().to_bytes(),
        &nonce.to_le_bytes(),
        &[event_bump],
    ];

    let acc_metas: Vec<AccountMeta> = acc_infos
        .iter()
        .flat_map(|acc_info| {
            let is_signer = signer_keys.contains(acc_info.key);
            acc_info.to_account_metas(Some(is_signer))
        })
        .collect();

    let instruction = Instruction {
        program_id: ctx.accounts.token_messenger_minter_program.key(),
        accounts: acc_metas,
        data: ix_data,
    };

    invoke_signed(
        &instruction,
        &acc_infos,
        &[cpi_signer_seeds, cctp_message_sent_event_seeds],
    )?;

    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_left_padded_pubkey() {
        let input = vec![1, 2, 3];
        let expected_output = Pubkey::new_from_array(
            [0; 29]
                .iter()
                .chain(&[1, 2, 3])
                .cloned()
                .collect::<Vec<u8>>()
                .try_into()
                .unwrap(),
        );
        let output = utils::left_padded_pubkey(&input).unwrap();
        assert_eq!(output, expected_output);
    }

    #[test]
    fn test_left_padded_pubkey_exact_length() {
        let input = vec![1; 32];
        let expected_output = Pubkey::new_from_array([1; 32]);
        let output = utils::left_padded_pubkey(&input).unwrap();
        assert_eq!(output, expected_output);
    }

    #[test]
    fn test_left_padded_pubkey_too_long() {
        let input = vec![1; 33];
        let result = utils::left_padded_pubkey(&input);
        assert!(result.is_err());
    }
}
