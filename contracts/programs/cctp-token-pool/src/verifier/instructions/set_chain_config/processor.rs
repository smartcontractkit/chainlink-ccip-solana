use anchor_lang::prelude::*;

use ccip_common::types::{CrossChainGas, UsdCents};

use crate::error::CctpPoolVerifierError;
use crate::verifier::state::*;

#[derive(AnchorSerialize, AnchorDeserialize, Debug)]
pub struct SetVerifierChainConfigParams {
    pub remote_chain_selector: u64,
    pub chain_config: RemoteChainAdminConfig,
}

pub fn init_verifier_chain_config(
    ctx: Context<InitVerifierChainConfig>,
    params: SetVerifierChainConfigParams,
) -> Result<()> {
    validate(&params)?;
    emit_cpi!(chain_config_set_event(&params));

    ctx.accounts
        .remote_chain_config
        .set_inner(RemoteChainConfig {
            version: 1,
            bump: ctx.bumps.remote_chain_config,
            remote_chain_selector: params.remote_chain_selector,
            admin_config: params.chain_config,
            allowlist_config: RemoteChainAllowlistConfig {
                allowlist_enabled: false,
                allowed_senders: vec![],
            },
            cctp: RemoteCctpConfig {
                enabled: false,
                domain_identifier: 0,
                allowed_caller_receive: Pubkey::default(),
                allowed_caller_deposit: Pubkey::default(),
                mint_recipient_on_dest: None,
                lookup_table: None,
                supported_tokens: vec![],
            },
        });

    Ok(())
}

pub fn edit_verifier_chain_config(
    ctx: Context<EditVerifierChainConfig>,
    params: SetVerifierChainConfigParams,
) -> Result<()> {
    validate(&params)?;
    emit_cpi!(chain_config_set_event(&params));

    // Only the fixed-size admin config is replaced, so the account never changes size and the
    // allowlist and CCTP configs are preserved.
    ctx.accounts.remote_chain_config.admin_config = params.chain_config;

    Ok(())
}

fn validate(params: &SetVerifierChainConfigParams) -> Result<()> {
    // INV-CFG-2: reject the default (zero-value) pubkey in CCV configuration fields so a lane
    // cannot be configured with an unset router signer.
    require_keys_neq!(
        params.chain_config.router,
        Pubkey::default(),
        CctpPoolVerifierError::ZeroRouter
    );
    Ok(())
}

fn chain_config_set_event(params: &SetVerifierChainConfigParams) -> ChainConfigSet {
    ChainConfigSet {
        remote_chain_selector: params.remote_chain_selector,
        router: params.chain_config.router,
        fee: params.chain_config.fee,
        gas_for_verification: params.chain_config.gas_for_verification,
        payload_size_bytes: params.chain_config.payload_size_bytes,
    }
}

#[event_cpi]
#[derive(Accounts)]
#[instruction(params: SetVerifierChainConfigParams)]
pub struct InitVerifierChainConfig<'info> {
    #[account(
        seeds = [verifier_seed::CONFIG],
        bump = config.bump,
        constraint = config.is_latest_version() @ CctpPoolVerifierError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,

    #[account(mut, address = config.owner @ CctpPoolVerifierError::Unauthorized)]
    pub authority: Signer<'info>,

    #[account(
        init,
        seeds = [verifier_seed::REMOTE_CHAIN, &params.remote_chain_selector.to_le_bytes()],
        bump,
        payer = authority,
        space = RemoteChainConfig::DISCRIMINATOR.len() + RemoteChainConfig::INIT_SPACE,
    )]
    pub remote_chain_config: Account<'info, RemoteChainConfig>,

    pub system_program: Program<'info, System>,
}

#[event_cpi]
#[derive(Accounts)]
#[instruction(params: SetVerifierChainConfigParams)]
pub struct EditVerifierChainConfig<'info> {
    #[account(
        seeds = [verifier_seed::CONFIG],
        bump = config.bump,
        constraint = config.is_latest_version() @ CctpPoolVerifierError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,

    #[account(address = config.owner @ CctpPoolVerifierError::Unauthorized)]
    pub authority: Signer<'info>,

    #[account(
        mut,
        seeds = [verifier_seed::REMOTE_CHAIN, &params.remote_chain_selector.to_le_bytes()],
        bump = remote_chain_config.bump,
        constraint = remote_chain_config.is_latest_version() @ CctpPoolVerifierError::InvalidVersion,
    )]
    pub remote_chain_config: Account<'info, RemoteChainConfig>,
}

#[event]
pub struct ChainConfigSet {
    pub remote_chain_selector: u64,
    pub router: Pubkey,
    pub fee: UsdCents,
    pub gas_for_verification: CrossChainGas,
    pub payload_size_bytes: u32,
}
