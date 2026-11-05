use anchor_lang::prelude::*;
use base_token_pool::constants::valid_version;
use base_token_pool::errors::CcipTokenPoolError;

use crate::constants::VERIFIER_REMOTE_CHAIN_SEED;
use crate::errors::LombardTokenPoolError;
use crate::events::*;
use crate::state::{
    LombardVerifierPath, VerifierConfig, VerifierRemoteChainAdminConfig,
    VerifierRemoteChainAllowlistConfig, VerifierRemoteChainConfig, MAX_VERIFIER_CONFIG_V,
    MAX_VERIFIER_REMOTE_CHAIN_CONFIG_V,
};

#[derive(AnchorSerialize, AnchorDeserialize, Clone, Debug)]
pub struct SetVerifierChainConfigParams {
    pub remote_chain_selector: u64,
    pub chain_config: VerifierRemoteChainAdminConfig,
}

pub fn init_verifier_chain_config(
    ctx: Context<InitVerifierChainConfig>,
    params: SetVerifierChainConfigParams,
) -> Result<()> {
    validate(&params)?;
    emit!(chain_config_set_event(&params));

    ctx.accounts
        .remote_chain_config
        .set_inner(VerifierRemoteChainConfig {
            version: 1,
            bump: ctx.bumps.remote_chain_config,
            remote_chain_selector: params.remote_chain_selector,
            admin_config: params.chain_config,
            allowlist_config: VerifierRemoteChainAllowlistConfig {
                allowlist_enabled: false,
                allowed_senders: vec![],
            },
            path: LombardVerifierPath::default(),
            lookup_table: None,
            remote_adapters: vec![],
        });

    Ok(())
}

pub fn edit_verifier_chain_config(
    ctx: Context<EditVerifierChainConfig>,
    params: SetVerifierChainConfigParams,
) -> Result<()> {
    validate(&params)?;
    emit!(chain_config_set_event(&params));

    // Only the fixed-size admin config is replaced, so the account never changes size and the
    // rest of the remote chain configuration is preserved.
    ctx.accounts.remote_chain_config.admin_config = params.chain_config;

    Ok(())
}

fn validate(params: &SetVerifierChainConfigParams) -> Result<()> {
    require_keys_neq!(
        params.chain_config.router_signer,
        Pubkey::default(),
        LombardTokenPoolError::ZeroRouterSigner
    );
    Ok(())
}

fn chain_config_set_event(params: &SetVerifierChainConfigParams) -> VerifierChainConfigSet {
    VerifierChainConfigSet {
        remote_chain_selector: params.remote_chain_selector,
        router_signer: params.chain_config.router_signer,
        fee: params.chain_config.fee,
        gas_for_verification: params.chain_config.gas_for_verification,
        payload_size_bytes: params.chain_config.payload_size_bytes,
    }
}

#[derive(Accounts)]
#[instruction(params: SetVerifierChainConfigParams)]
pub struct InitVerifierChainConfig<'info> {
    #[account(
        seeds = [crate::constants::VERIFIER_CONFIG_SEED],
        bump = verifier_config.bump,
        constraint = valid_version(verifier_config.version, MAX_VERIFIER_CONFIG_V) @ CcipTokenPoolError::InvalidVersion,
    )]
    pub verifier_config: Account<'info, VerifierConfig>,

    #[account(mut, address = verifier_config.owner @ CcipTokenPoolError::Unauthorized)]
    pub authority: Signer<'info>,

    #[account(
        init,
        seeds = [VERIFIER_REMOTE_CHAIN_SEED, &params.remote_chain_selector.to_le_bytes()],
        bump,
        payer = authority,
        space = VerifierRemoteChainConfig::DISCRIMINATOR.len() + VerifierRemoteChainConfig::INIT_SPACE,
    )]
    pub remote_chain_config: Account<'info, VerifierRemoteChainConfig>,

    pub system_program: Program<'info, System>,
}

#[derive(Accounts)]
#[instruction(params: SetVerifierChainConfigParams)]
pub struct EditVerifierChainConfig<'info> {
    #[account(
        seeds = [crate::constants::VERIFIER_CONFIG_SEED],
        bump = verifier_config.bump,
        constraint = valid_version(verifier_config.version, MAX_VERIFIER_CONFIG_V) @ CcipTokenPoolError::InvalidVersion,
    )]
    pub verifier_config: Account<'info, VerifierConfig>,

    #[account(address = verifier_config.owner @ CcipTokenPoolError::Unauthorized)]
    pub authority: Signer<'info>,

    #[account(
        mut,
        seeds = [VERIFIER_REMOTE_CHAIN_SEED, &params.remote_chain_selector.to_le_bytes()],
        bump = remote_chain_config.bump,
        constraint = valid_version(remote_chain_config.version, MAX_VERIFIER_REMOTE_CHAIN_CONFIG_V) @ CcipTokenPoolError::InvalidVersion,
    )]
    pub remote_chain_config: Account<'info, VerifierRemoteChainConfig>,
}

/// Sets (or clears) this remote chain's own address lookup table.
pub fn set_verifier_lookup_table(
    ctx: Context<SetVerifierLookupTable>,
    remote_chain_selector: u64,
    lookup_table: Option<Pubkey>,
) -> Result<()> {
    let remote_chain_config = &mut ctx.accounts.remote_chain_config;
    let old = remote_chain_config.lookup_table;
    remote_chain_config.lookup_table = lookup_table;

    emit!(VerifierLookupTableSet {
        remote_chain_selector,
        old,
        new: lookup_table,
    });

    Ok(())
}

#[derive(Accounts)]
#[instruction(remote_chain_selector: u64)]
pub struct SetVerifierLookupTable<'info> {
    #[account(
        seeds = [crate::constants::VERIFIER_CONFIG_SEED],
        bump = verifier_config.bump,
        constraint = valid_version(verifier_config.version, MAX_VERIFIER_CONFIG_V) @ CcipTokenPoolError::InvalidVersion,
    )]
    pub verifier_config: Account<'info, VerifierConfig>,

    #[account(address = verifier_config.owner @ CcipTokenPoolError::Unauthorized)]
    pub authority: Signer<'info>,

    #[account(
        mut,
        seeds = [VERIFIER_REMOTE_CHAIN_SEED, &remote_chain_selector.to_le_bytes()],
        bump = remote_chain_config.bump,
        constraint = valid_version(remote_chain_config.version, MAX_VERIFIER_REMOTE_CHAIN_CONFIG_V) @ CcipTokenPoolError::InvalidVersion,
    )]
    pub remote_chain_config: Account<'info, VerifierRemoteChainConfig>,
}

#[derive(AnchorSerialize, AnchorDeserialize, Clone, Debug)]
pub struct SetVerifierAllowlistParams {
    pub dest_chain_selector: u64,
    pub enabled: bool,
    pub allowed_senders: Vec<Pubkey>,
}

pub fn set_verifier_allowlist(
    ctx: Context<SetVerifierAllowlist>,
    params: SetVerifierAllowlistParams,
) -> Result<()> {
    ctx.accounts.remote_chain_config.allowlist_config = VerifierRemoteChainAllowlistConfig {
        allowlist_enabled: params.enabled,
        allowed_senders: params.allowed_senders.clone(),
    };

    emit!(VerifierAllowlistSet {
        dest_chain_selector: params.dest_chain_selector,
        enabled: params.enabled,
        allowed_senders: params.allowed_senders,
    });

    Ok(())
}

#[derive(Accounts)]
#[instruction(params: SetVerifierAllowlistParams)]
pub struct SetVerifierAllowlist<'info> {
    #[account(
        seeds = [crate::constants::VERIFIER_CONFIG_SEED],
        bump = verifier_config.bump,
        constraint = valid_version(verifier_config.version, MAX_VERIFIER_CONFIG_V) @ CcipTokenPoolError::InvalidVersion,
    )]
    pub verifier_config: Account<'info, VerifierConfig>,

    /// The owner or the allowlist admin, like the committee and CCTP verifiers.
    #[account(mut, constraint = authority.key() == verifier_config.owner || authority.key() == verifier_config.allowlist_admin @ CcipTokenPoolError::Unauthorized)]
    pub authority: Signer<'info>,

    #[account(
        mut,
        seeds = [VERIFIER_REMOTE_CHAIN_SEED, &params.dest_chain_selector.to_le_bytes()],
        bump = remote_chain_config.bump,
        constraint = valid_version(remote_chain_config.version, MAX_VERIFIER_REMOTE_CHAIN_CONFIG_V) @ CcipTokenPoolError::InvalidVersion,
        realloc = VerifierRemoteChainConfig::DISCRIMINATOR.len() + VerifierRemoteChainConfig::INIT_SPACE + VerifierRemoteChainConfig::dynamic_space(&params.allowed_senders, &remote_chain_config.remote_adapters),
        realloc::payer = authority,
        realloc::zero = false,
    )]
    pub remote_chain_config: Account<'info, VerifierRemoteChainConfig>,

    pub system_program: Program<'info, System>,
}
