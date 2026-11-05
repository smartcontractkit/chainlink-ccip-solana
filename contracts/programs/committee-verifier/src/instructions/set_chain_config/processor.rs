use anchor_lang::prelude::*;

use ccip_common::seed;

use crate::event::ChainConfigSet;
use crate::state::valid_version;
use crate::state::{
    Config, RemoteChainAdminConfig, RemoteChainAllowlistConfig, RemoteChainConfig,
    RemoteChainSignerConfig, ANCHOR_DISCRIMINATOR, MAX_CONFIG_V,
};
use crate::CommitteeVerifierError;

#[derive(AnchorSerialize, AnchorDeserialize, Debug)]
pub struct SetChainConfigParams {
    pub remote_chain_selector: u64,
    pub chain_config: RemoteChainAdminConfig,
}

pub fn init_chain_config(
    ctx: Context<InitChainConfig>,
    params: SetChainConfigParams,
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
            signer_config: RemoteChainSignerConfig::new_disabled(),
        });

    Ok(())
}

pub fn edit_chain_config(
    ctx: Context<EditChainConfig>,
    params: SetChainConfigParams,
) -> Result<()> {
    validate(&params)?;
    emit_cpi!(chain_config_set_event(&params));

    // Only the fixed-size admin config is replaced, so the account never changes size and the
    // signer and allowlist entries are preserved.
    ctx.accounts.remote_chain_config.admin_config = params.chain_config;

    Ok(())
}

fn validate(params: &SetChainConfigParams) -> Result<()> {
    // INV-CFG-2: reject the default (zero-value) pubkey in CCV configuration fields so a lane
    // cannot be configured with an unset router signer.
    require_keys_neq!(
        params.chain_config.router_signer,
        Pubkey::default(),
        CommitteeVerifierError::ZeroRouterSigner
    );
    Ok(())
}

fn chain_config_set_event(params: &SetChainConfigParams) -> ChainConfigSet {
    ChainConfigSet {
        remote_chain_selector: params.remote_chain_selector,
        router: params.chain_config.router_signer,
        fee: params.chain_config.fee,
        gas_for_verification: params.chain_config.gas_for_verification,
        payload_size_bytes: params.chain_config.payload_size_bytes,
    }
}

#[event_cpi]
#[derive(Accounts)]
#[instruction(params: SetChainConfigParams)]
pub struct InitChainConfig<'info> {
    #[account(
        seeds = [seed::CONFIG],
        bump = config.bump,
        constraint = valid_version(config.version, MAX_CONFIG_V) @ CommitteeVerifierError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,

    #[account(mut, address = config.owner @ CommitteeVerifierError::Unauthorized)]
    pub authority: Signer<'info>,

    #[account(
        init,
        seeds = [seed::REMOTE_CHAIN, &params.remote_chain_selector.to_le_bytes()],
        bump,
        payer = authority,
        space = ANCHOR_DISCRIMINATOR + RemoteChainConfig::INIT_SPACE,
    )]
    pub remote_chain_config: Account<'info, RemoteChainConfig>,

    pub system_program: Program<'info, System>,
}

#[event_cpi]
#[derive(Accounts)]
#[instruction(params: SetChainConfigParams)]
pub struct EditChainConfig<'info> {
    #[account(
        seeds = [seed::CONFIG],
        bump = config.bump,
        constraint = valid_version(config.version, MAX_CONFIG_V) @ CommitteeVerifierError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,

    #[account(address = config.owner @ CommitteeVerifierError::Unauthorized)]
    pub authority: Signer<'info>,

    #[account(
        mut,
        seeds = [seed::REMOTE_CHAIN, &params.remote_chain_selector.to_le_bytes()],
        bump = remote_chain_config.bump,
        constraint = valid_version(remote_chain_config.version, MAX_CONFIG_V) @ CommitteeVerifierError::InvalidVersion,
    )]
    pub remote_chain_config: Account<'info, RemoteChainConfig>,
}
