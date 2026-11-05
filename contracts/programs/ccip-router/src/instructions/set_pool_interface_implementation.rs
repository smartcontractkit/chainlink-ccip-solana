use anchor_lang::prelude::*;
use ccip_common::router_accounts::{ResolutionType, TokenAdminRegistry};
use ccip_common::seed;

use crate::context::{valid_version, MAX_CONFIG_V};
use crate::error::CcipRouterError;
use crate::event::events::token_admin_registry::PoolInterfaceImplementationUpdated;
use crate::state::Config;

#[derive(AnchorSerialize, AnchorDeserialize, Debug)]
pub struct SetPoolInterfaceImplementationParams {
    pub mint: Pubkey,
    pub interface_version: u8,
    pub account_resolution: ResolutionType,
}

impl SetPoolInterfaceImplementationParams {
    pub fn validate(&self) -> Result<()> {
        TokenAdminRegistry::validate_interface_version(self.interface_version)?;
        require!(
            !self.account_resolution.is_enabled() || self.interface_version >= 2,
            CcipRouterError::AccountResolutionRequiresV2
        );
        Ok(())
    }
}

pub fn set_pool_interface_implementation(
    ctx: Context<EditPoolTokenAdminRegistryV3>,
    params: SetPoolInterfaceImplementationParams,
) -> Result<()> {
    let token_admin_registry = &mut ctx.accounts.token_admin_registry;

    params.validate()?;

    let SetPoolInterfaceImplementationParams {
        mint,
        interface_version,
        account_resolution,
    } = params;

    token_admin_registry.interface_version = interface_version;
    token_admin_registry.account_resolution = account_resolution.clone();
    token_admin_registry.bump = ctx.bumps.token_admin_registry; // update the bump in case it was not set before

    emit_cpi!(PoolInterfaceImplementationUpdated {
        token: mint,
        interface_version,
        account_resolution,
    });

    Ok(())
}

#[event_cpi]
#[derive(Accounts)]
#[instruction(params: SetPoolInterfaceImplementationParams)]
pub struct EditPoolTokenAdminRegistryV3<'info> {
    #[account(
        seeds = [seed::CONFIG],
        bump,
        constraint = valid_version(config.version, MAX_CONFIG_V) @ CcipRouterError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,

    #[account(
        mut,
        seeds = [seed::TOKEN_ADMIN_REGISTRY, params.mint.as_ref()],
        bump,
        constraint = token_admin_registry.version == 3 @ CcipRouterError::InvalidVersion,
    )]
    pub token_admin_registry: Account<'info, TokenAdminRegistry>,

    #[account(mut, address = token_admin_registry.administrator @ CcipRouterError::Unauthorized)]
    pub authority: Signer<'info>,
}

#[cfg(test)]
mod tests {
    use super::*;
    use ccip_common::router_accounts::ResolutionType;

    fn make_params(
        interface_version: u8,
        account_resolution: ResolutionType,
    ) -> SetPoolInterfaceImplementationParams {
        SetPoolInterfaceImplementationParams {
            mint: Pubkey::default(),
            interface_version,
            account_resolution,
        }
    }

    #[test]
    fn test_v1_without_account_resolution_ok() {
        assert!(make_params(1, ResolutionType::NoResolution)
            .validate()
            .is_ok());
    }

    #[test]
    fn test_v2_without_account_resolution_ok() {
        assert!(make_params(2, ResolutionType::NoResolution)
            .validate()
            .is_ok());
    }

    #[test]
    fn test_v2_with_account_resolution_ok() {
        assert!(make_params(2, ResolutionType::Standard).validate().is_ok());
    }

    #[test]
    fn test_v1_with_account_resolution_fails() {
        let err = make_params(1, ResolutionType::Standard)
            .validate()
            .unwrap_err();
        assert_eq!(err, CcipRouterError::AccountResolutionRequiresV2.into());
    }

    #[test]
    fn test_invalid_version_zero_fails() {
        assert!(make_params(0, ResolutionType::NoResolution)
            .validate()
            .is_err());
    }

    #[test]
    fn test_invalid_version_three_fails() {
        assert!(make_params(3, ResolutionType::NoResolution)
            .validate()
            .is_err());
    }
}
