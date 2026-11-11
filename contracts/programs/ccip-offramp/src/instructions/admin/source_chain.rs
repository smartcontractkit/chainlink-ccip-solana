use anchor_lang::prelude::*;
use ccip_common::seed;
use ccip_common::state::AccountSchemaVersion;

use crate::event::admin::{SourceChainAdded, SourceChainConfigUpdated, SourceChainDeleted};
use crate::instructions::ccv_resolution::validate_default_and_mandated_ccvs;
use crate::state::{
    Config, SourceChain, SourceChainConfig, MAX_LEGACY_ADDRESS_LENGTH, MAX_ON_RAMPS,
};
use crate::CcipOfframpError;

#[derive(AnchorSerialize, AnchorDeserialize, Clone)]
pub struct AddSourceChainParams {
    pub new_chain_selector: u64,
    pub source_chain_config: SourceChainConfig,
}

#[event_cpi]
#[derive(Accounts)]
#[instruction(params: AddSourceChainParams)]
pub struct AddSourceChain<'info> {
    /// Adding a chain selector implies initializing the state for a new chain
    #[account(
        init,
        seeds = [seed::SOURCE_CHAIN, params.new_chain_selector.to_le_bytes().as_ref()],
        bump,
        payer = authority,
        space = SourceChain::DISCRIMINATOR.len() + SourceChain::INIT_SPACE + SourceChain::dynamic_space(&params.source_chain_config.on_ramps, &params.source_chain_config.default_ccvs, &params.source_chain_config.lane_mandated_ccvs),
    )]
    pub source_chain: Account<'info, SourceChain>,

    #[account(
        seeds = [seed::CONFIG],
        bump = config.bump,
        constraint = config.version == Config::MAX_V @ CcipOfframpError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,

    #[account(mut, address = config.owner @ CcipOfframpError::Unauthorized)]
    pub authority: Signer<'info>,
    pub system_program: Program<'info, System>,
}

pub fn add_source_chain(ctx: Context<AddSourceChain>, params: AddSourceChainParams) -> Result<()> {
    // Set source chain config & state
    let source_chain = &mut ctx.accounts.source_chain;
    validate_source_chain_config(params.new_chain_selector, &params.source_chain_config)?;
    source_chain.set_inner(SourceChain {
        version: SourceChain::MAX_V,
        bump: ctx.bumps.source_chain,
        chain_selector: params.new_chain_selector,
        config: params.source_chain_config.clone(),
    });

    emit_cpi!(SourceChainAdded {
        source_chain_selector: params.new_chain_selector,
        source_chain_config: params.source_chain_config,
    });

    Ok(())
}

// ---

#[event_cpi]
#[derive(Accounts)]
#[instruction(source_chain_selector: u64)]
pub struct DisableSourceChain<'info> {
    #[account(
        mut,
        seeds = [seed::SOURCE_CHAIN, source_chain_selector.to_le_bytes().as_ref()],
        bump = source_chain.bump,
        constraint = source_chain.version == SourceChain::MAX_V @ CcipOfframpError::InvalidVersion,
    )]
    pub source_chain: Account<'info, SourceChain>,

    #[account(
        seeds = [seed::CONFIG],
        bump = config.bump,
        constraint = config.version == Config::MAX_V @ CcipOfframpError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,

    #[account(mut, address = config.owner @ CcipOfframpError::Unauthorized)]
    pub authority: Signer<'info>,
}

pub fn disable_source_chain_selector(
    ctx: Context<DisableSourceChain>,
    source_chain_selector: u64,
) -> Result<()> {
    let source_chain = &mut ctx.accounts.source_chain;

    source_chain.config.is_enabled = false;

    emit_cpi!(SourceChainConfigUpdated {
        source_chain_selector,
        source_chain_config: source_chain.config.clone(),
    });

    Ok(())
}

// ---

#[event_cpi]
#[derive(Accounts)]
#[instruction(source_chain_selector: u64)]
pub struct DeleteSourceChain<'info> {
    #[account(
        mut,
        seeds = [seed::SOURCE_CHAIN, source_chain_selector.to_le_bytes().as_ref()],
        bump,
        owner = crate::ID,
    )]
    pub source_chain: UncheckedAccount<'info>,

    #[account(
        seeds = [seed::CONFIG],
        bump = config.bump,
        constraint = config.version == Config::MAX_V @ CcipOfframpError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,

    #[account(mut, address = config.owner @ CcipOfframpError::Unauthorized)]
    pub authority: Signer<'info>,
}

pub fn delete_source_chain(
    ctx: Context<DeleteSourceChain>,
    source_chain_selector: u64,
) -> Result<()> {
    ccip_common::account_management::close(
        &ctx.accounts.source_chain.to_account_info(),
        &ctx.accounts.authority.to_account_info(),
    )?;

    emit_cpi!(SourceChainDeleted {
        source_chain_selector,
    });

    Ok(())
}

// ---

#[derive(AnchorSerialize, AnchorDeserialize, Clone)]
pub struct UpdateSourceChainParams {
    pub source_chain_selector: u64,
    pub source_chain_config: SourceChainConfig,
}

#[event_cpi]
#[derive(Accounts)]
#[instruction(params: UpdateSourceChainParams)]
pub struct UpdateSourceChain<'info> {
    #[account(
        mut,
        seeds = [seed::SOURCE_CHAIN, params.source_chain_selector.to_le_bytes().as_ref()],
        bump = source_chain.bump,
        realloc = SourceChain::DISCRIMINATOR.len() + SourceChain::INIT_SPACE + SourceChain::dynamic_space(&params.source_chain_config.on_ramps, &params.source_chain_config.default_ccvs, &params.source_chain_config.lane_mandated_ccvs),
        realloc::payer = authority,
        realloc::zero = false,
        constraint = source_chain.version == SourceChain::MAX_V @ CcipOfframpError::InvalidVersion,
    )]
    pub source_chain: Account<'info, SourceChain>,

    #[account(
        seeds = [seed::CONFIG],
        bump = config.bump,
        constraint = config.version == Config::MAX_V @ CcipOfframpError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,

    #[account(mut, address = config.owner @ CcipOfframpError::Unauthorized)]
    pub authority: Signer<'info>,

    pub system_program: Program<'info, System>,
}

/// Replaces the entire source chain config, `on_ramps` included: the list provided here becomes the
/// complete set of allowed OnRamps, so callers adding an OnRamp must pass the ones they want to keep
/// as well.
///
/// Dropping an OnRamp that still has inflight messages makes those messages unexecutable, but only
/// for as long as it stays dropped: the check runs at execution time against whatever this account
/// holds, and a rejected execution reverts, leaving no trace behind (a message's `MessageState` PDA
/// only ever records a *successful* execution). So re-adding a mistakenly dropped OnRamp makes its
/// inflight messages executable again, with no manual recovery step beyond the config fix.
pub fn update_source_chain_config(
    ctx: Context<UpdateSourceChain>,
    params: UpdateSourceChainParams,
) -> Result<()> {
    validate_source_chain_config(params.source_chain_selector, &params.source_chain_config)?;

    ctx.accounts.source_chain.config = params.source_chain_config.clone();

    emit_cpi!(SourceChainConfigUpdated {
        source_chain_selector: params.source_chain_selector,
        source_chain_config: params.source_chain_config,
    });
    Ok(())
}

fn validate_source_chain_config(
    _source_chain_selector: u64,
    config: &SourceChainConfig,
) -> Result<()> {
    require!(
        !config.on_ramps.is_empty(),
        CcipOfframpError::InvalidOnrampAddress
    );
    require_gte!(
        MAX_ON_RAMPS,
        config.on_ramps.len(),
        CcipOfframpError::TooManyOnRamps
    );
    require_gt!(
        config.legacy_address_length,
        0,
        CcipOfframpError::InvalidLegacyAddressLength
    );
    require_gte!(
        MAX_LEGACY_ADDRESS_LENGTH,
        config.legacy_address_length,
        CcipOfframpError::InvalidLegacyAddressLength
    );

    for (i, on_ramp) in config.on_ramps.iter().enumerate() {
        require!(!on_ramp.is_empty(), CcipOfframpError::InvalidOnrampAddress);
        require!(!on_ramp.is_zero(), CcipOfframpError::InvalidOnrampAddress);
        require!(
            !config.on_ramps[..i]
                .iter()
                .any(|previous| previous.bytes() == on_ramp.bytes()),
            CcipOfframpError::DuplicateOnRampNotAllowed
        );
    }

    require!(
        !config.default_ccvs.is_empty(),
        CcipOfframpError::ZeroPubkeyNotAllowedInCCVConfig
    );

    validate_default_and_mandated_ccvs(&config.default_ccvs, &config.lane_mandated_ccvs)?;

    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::state::{OnRampAddress, SourceChainConfig};

    fn pk(seed: u8) -> Pubkey {
        let mut bytes = [0u8; 32];
        bytes[0] = seed;
        Pubkey::from(bytes)
    }

    #[test]
    fn source_chain_config_validation() {
        let valid_config = SourceChainConfig {
            is_enabled: false,
            on_ramps: vec![OnRampAddress::from([0xAB; 64])],
            default_ccvs: vec![pk(1)],
            lane_mandated_ccvs: vec![],
            legacy_address_length: 20,
        };

        validate_source_chain_config(1, &valid_config).unwrap();

        let invalid_config_zero_address = SourceChainConfig {
            is_enabled: false,
            on_ramps: vec![OnRampAddress::from([0x00; 64])],
            default_ccvs: vec![pk(1)],
            lane_mandated_ccvs: vec![],
            legacy_address_length: 20,
        };

        assert_eq!(
            validate_source_chain_config(1, &invalid_config_zero_address).unwrap_err(),
            CcipOfframpError::InvalidOnrampAddress.into()
        );

        let invalid_config_empty_address = SourceChainConfig {
            is_enabled: false,
            on_ramps: vec![OnRampAddress::EMPTY],
            default_ccvs: vec![pk(1)],
            lane_mandated_ccvs: vec![],
            legacy_address_length: 20,
        };

        assert_eq!(
            validate_source_chain_config(1, &invalid_config_empty_address).unwrap_err(),
            CcipOfframpError::InvalidOnrampAddress.into()
        );
    }

    #[test]
    fn source_chain_config_legacy_address_length_validation() {
        let config_with = |legacy_address_length: u8| SourceChainConfig {
            is_enabled: true,
            on_ramps: vec![OnRampAddress::from([0xAB; 64])],
            default_ccvs: vec![pk(1)],
            lane_mandated_ccvs: vec![],
            legacy_address_length,
        };

        for valid in [1, 20, 32, MAX_LEGACY_ADDRESS_LENGTH] {
            validate_source_chain_config(1, &config_with(valid)).unwrap();
        }

        for invalid in [0, MAX_LEGACY_ADDRESS_LENGTH + 1, u8::MAX] {
            assert_eq!(
                validate_source_chain_config(1, &config_with(invalid)).unwrap_err(),
                CcipOfframpError::InvalidLegacyAddressLength.into(),
                "legacy_address_length {invalid} should be rejected"
            );
        }
    }

    #[test]
    fn source_chain_config_on_ramps_validation() {
        let config_with = |on_ramps: Vec<OnRampAddress>| SourceChainConfig {
            is_enabled: true,
            on_ramps,
            default_ccvs: vec![pk(1)],
            lane_mandated_ccvs: vec![],
            legacy_address_length: 20,
        };

        // Several OnRamps at once is the configuration a source chain sits in while it upgrades its
        // own OnRamp: the old one keeps draining its inflight messages while the new one sends.
        validate_source_chain_config(
            1,
            &config_with(vec![
                OnRampAddress::from([0xAB; 64]),
                OnRampAddress::from([0xCD; 32]),
            ]),
        )
        .unwrap();

        assert_eq!(
            validate_source_chain_config(1, &config_with(vec![])).unwrap_err(),
            CcipOfframpError::InvalidOnrampAddress.into()
        );

        assert_eq!(
            validate_source_chain_config(
                1,
                &config_with(vec![
                    OnRampAddress::from([0xAB; 64]),
                    OnRampAddress::from([0xAB; 64]),
                ])
            )
            .unwrap_err(),
            CcipOfframpError::DuplicateOnRampNotAllowed.into()
        );

        let too_many = (0..MAX_ON_RAMPS + 1)
            .map(|i| OnRampAddress::from([i as u8 + 1; 32]))
            .collect();
        assert_eq!(
            validate_source_chain_config(1, &config_with(too_many)).unwrap_err(),
            CcipOfframpError::TooManyOnRamps.into()
        );
    }

    #[test]
    fn source_chain_config_ccv_validation() {
        let config_without_ccvs = SourceChainConfig {
            is_enabled: false,
            on_ramps: vec![OnRampAddress::from([0xAB; 64])],
            default_ccvs: vec![],
            lane_mandated_ccvs: vec![],
            legacy_address_length: 20,
        };

        assert_eq!(
            validate_source_chain_config(1, &config_without_ccvs).unwrap_err(),
            CcipOfframpError::ZeroPubkeyNotAllowedInCCVConfig.into()
        );

        let config_with_zero = SourceChainConfig {
            is_enabled: false,
            on_ramps: vec![OnRampAddress::from([0xAB; 64])],
            default_ccvs: vec![Pubkey::default()],
            lane_mandated_ccvs: vec![],
            legacy_address_length: 20,
        };

        assert_eq!(
            validate_source_chain_config(1, &config_with_zero).unwrap_err(),
            CcipOfframpError::ZeroPubkeyNotAllowedInCCVConfig.into()
        );

        let config_with_duplicate = SourceChainConfig {
            is_enabled: false,
            on_ramps: vec![OnRampAddress::from([0xAB; 64])],
            default_ccvs: vec![pk(1)],
            lane_mandated_ccvs: vec![pk(1)],
            legacy_address_length: 20,
        };

        assert_eq!(
            validate_source_chain_config(1, &config_with_duplicate).unwrap_err(),
            CcipOfframpError::DuplicateCCVNotAllowed.into()
        );

        let no_default = SourceChainConfig {
            is_enabled: false,
            on_ramps: vec![OnRampAddress::from([0xAB; 64])],
            default_ccvs: vec![],
            lane_mandated_ccvs: vec![pk(1)],
            legacy_address_length: 20,
        };

        assert_eq!(
            validate_source_chain_config(1, &no_default).unwrap_err(),
            CcipOfframpError::ZeroPubkeyNotAllowedInCCVConfig.into()
        );
    }
}
