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
    /// Each interface version allows a single account resolution mode, so the two can't be
    /// misconfigured against each other:
    ///
    /// * A v1 pool takes no account resolution, as 2.0 lanes reject a v1 pool that has it.
    /// * A v2 pool takes `WithoutExtendedLookupTable`. 2.0 lanes resolve every account a v2 pool
    ///   needs beyond the lookup table's required entries, so they never take the additional
    ///   entries. A table may still carry them for the token's 1.6 lanes, which ignore the skip
    ///   flag and read the mode as resolution enabled.
    pub fn validate(&self) -> Result<()> {
        TokenAdminRegistry::validate_interface_version(self.interface_version)?;
        require!(
            !self.account_resolution.is_enabled() || self.interface_version >= 2,
            CcipRouterError::AccountResolutionRequiresV2
        );
        require!(
            self.interface_version < 2
                || self.account_resolution == ResolutionType::WithoutExtendedLookupTable,
            CcipRouterError::PoolV2RequiresWithoutExtendedLookupTable
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
    token_admin_registry.set_account_resolution(&account_resolution);
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
        constraint = token_admin_registry.version == TokenAdminRegistry::LATEST_VERSION @ CcipRouterError::InvalidVersion,
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
    fn test_v2_without_extended_lookup_table_ok() {
        assert!(make_params(2, ResolutionType::WithoutExtendedLookupTable)
            .validate()
            .is_ok());
    }

    #[test]
    fn test_v2_with_other_account_resolutions_fails() {
        for resolution in [ResolutionType::NoResolution, ResolutionType::Standard] {
            let err = make_params(2, resolution).validate().unwrap_err();
            assert_eq!(
                err,
                CcipRouterError::PoolV2RequiresWithoutExtendedLookupTable.into()
            );
        }
    }

    #[test]
    fn test_v1_with_account_resolution_fails() {
        for resolution in [
            ResolutionType::Standard,
            ResolutionType::WithoutExtendedLookupTable,
        ] {
            let err = make_params(1, resolution).validate().unwrap_err();
            assert_eq!(err, CcipRouterError::AccountResolutionRequiresV2.into());
        }
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
