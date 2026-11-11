use anchor_lang::solana_program::log::sol_log;
use anchor_lang::{prelude::*, system_program, Discriminator};
use anchor_spl::associated_token::get_associated_token_address_with_program_id;
use ccip_common::seed;
use solana_address_lookup_table_interface as address_lookup_table;
use solana_address_lookup_table_interface::state::AddressLookupTable;

use ccip_common::router_accounts::TokenAdminRegistry;

use crate::context::ANCHOR_DISCRIMINATOR;
use crate::events::token_admin_registry as events;
use crate::instructions_v1::interfaces::TokenAdminRegistry as TokenAdminRegistryTrait;
use crate::token_admin_registry_writable;
use crate::token_context::{
    OverridePendingTokenAdminRegistryByCCIPAdmin, OverridePendingTokenAdminRegistryByMultisigOwner,
    OverridePendingTokenAdminRegistryByOwner, RegisterTokenAdminRegistryByCCIPAdmin,
    RegisterTokenAdminRegistryByMultisigOwner, RegisterTokenAdminRegistryByOwner,
    SetPoolTokenAdminRegistry, UpgradeTokenAdminRegistry,
};
use crate::{AcceptAdminRoleTokenAdminRegistry, CcipRouterError, ModifyTokenAdminRegistry};

const MINIMUM_TOKEN_POOL_ACCOUNTS: usize = 10;

pub struct Impl;
impl TokenAdminRegistryTrait for Impl {
    fn ccip_admin_propose_administrator(
        &self,
        ctx: Context<RegisterTokenAdminRegistryByCCIPAdmin>,
        token_admin_registry_admin: Pubkey,
    ) -> Result<()> {
        let token_mint = ctx.accounts.mint.key().to_owned();
        let token_admin_registry = &mut ctx.accounts.token_admin_registry;

        init_with_pending_administrator(
            token_admin_registry,
            ctx.bumps.token_admin_registry,
            token_mint,
            token_admin_registry_admin,
        )?;

        Ok(())
    }

    fn ccip_admin_override_pending_administrator(
        &self,
        ctx: Context<OverridePendingTokenAdminRegistryByCCIPAdmin>,
        token_admin_registry_admin: Pubkey,
    ) -> Result<()> {
        let token_admin_registry = &mut ctx.accounts.token_admin_registry;

        require_eq!(
            token_admin_registry.administrator,
            Pubkey::new_from_array([0; 32]),
            CcipRouterError::InvalidTokenAdminRegistryProposedAdmin
        );

        token_admin_registry.pending_administrator = token_admin_registry_admin;

        Ok(())
    }

    fn owner_propose_administrator(
        &self,
        ctx: Context<RegisterTokenAdminRegistryByOwner>,
        token_admin_registry_admin: Pubkey,
    ) -> Result<()> {
        let token_mint = ctx.accounts.mint.key().to_owned();
        let token_admin_registry = &mut ctx.accounts.token_admin_registry;

        init_with_pending_administrator(
            token_admin_registry,
            ctx.bumps.token_admin_registry,
            token_mint,
            token_admin_registry_admin,
        )?;

        Ok(())
    }

    fn multisig_owner_propose_administrator(
        &self,
        ctx: Context<RegisterTokenAdminRegistryByMultisigOwner>,
        token_admin_registry_admin: Pubkey,
    ) -> Result<()> {
        let token_mint = ctx.accounts.mint.key().to_owned();
        let token_admin_registry = &mut ctx.accounts.token_admin_registry;

        init_with_pending_administrator(
            token_admin_registry,
            ctx.bumps.token_admin_registry,
            token_mint,
            token_admin_registry_admin,
        )?;

        Ok(())
    }

    fn owner_override_pending_administrator(
        &self,
        ctx: Context<OverridePendingTokenAdminRegistryByOwner>,
        token_admin_registry_admin: Pubkey,
    ) -> Result<()> {
        let token_admin_registry = &mut ctx.accounts.token_admin_registry;

        require_eq!(
            token_admin_registry.administrator,
            Pubkey::new_from_array([0; 32]),
            CcipRouterError::InvalidTokenAdminRegistryProposedAdmin
        );

        token_admin_registry.pending_administrator = token_admin_registry_admin;

        Ok(())
    }

    fn multisig_owner_override_pending_administrator(
        &self,
        ctx: Context<OverridePendingTokenAdminRegistryByMultisigOwner>,
        token_admin_registry_admin: Pubkey,
    ) -> Result<()> {
        let token_admin_registry = &mut ctx.accounts.token_admin_registry;

        require_eq!(
            token_admin_registry.administrator,
            Pubkey::new_from_array([0; 32]),
            CcipRouterError::InvalidTokenAdminRegistryProposedAdmin
        );

        token_admin_registry.pending_administrator = token_admin_registry_admin;

        Ok(())
    }

    fn transfer_admin_role_token_admin_registry(
        &self,
        ctx: Context<ModifyTokenAdminRegistry>,
        new_admin: Pubkey,
    ) -> Result<()> {
        let token_mint = ctx.accounts.mint.key().to_owned();
        let token_admin_registry = &mut ctx.accounts.token_admin_registry;

        token_admin_registry.pending_administrator = new_admin;

        emit!(events::AdministratorTransferRequested {
            token: token_mint,
            current_admin: token_admin_registry.administrator,
            new_admin,
        });

        Ok(())
    }

    fn accept_admin_role_token_admin_registry(
        &self,
        ctx: Context<AcceptAdminRoleTokenAdminRegistry>,
    ) -> Result<()> {
        let token_mint = ctx.accounts.mint.key().to_owned();
        let token_admin_registry = &mut ctx.accounts.token_admin_registry;
        let new_admin = token_admin_registry.pending_administrator;

        token_admin_registry.administrator = new_admin;
        token_admin_registry.pending_administrator = Pubkey::new_from_array([0; 32]);

        emit!(events::AdministratorTransferred {
            token: token_mint,
            new_admin,
        });

        Ok(())
    }

    fn upgrade_token_admin_registry(&self, ctx: Context<UpgradeTokenAdminRegistry>) -> Result<()> {
        let acc_info = ctx.accounts.token_admin_registry.to_account_info();
        let bump = ctx.bumps.token_admin_registry;

        // Validate the account to upgrade (the context already checks initialization and size)
        {
            const DISCRIM_LEN: usize = TokenAdminRegistry::DISCRIMINATOR.len();
            let data = acc_info.try_borrow_data()?;
            require!(
                data[..DISCRIM_LEN] == *TokenAdminRegistry::DISCRIMINATOR,
                CcipRouterError::InvalidInputsTokenAdminRegistryAccounts
            );

            // Check that version is within range of old versions that need to be migrated
            // (currently v1, v2 and v3, being migrated to v4 which is the latest version as of late 2026)
            let version_byte = data[DISCRIM_LEN];
            require_gte!(
                version_byte,
                1,
                CcipRouterError::InvalidInputsTokenAdminRegistryAccounts
            );
            require_gt!(
                TokenAdminRegistry::LATEST_VERSION,
                version_byte,
                CcipRouterError::InvalidInputsTokenAdminRegistryAccounts
            )
        }

        // Decode the old layout before resizing. The loader picks the layout from the account length
        // (which the context restricts to the v1, v2 and v3 sizes), checks that the stored version byte
        // agrees with it, and fills the fields that layout lacks with their safe defaults. Decoding the
        // resized data as the latest layout instead would misread v3: its resolution byte may hold 2,
        // which is not a valid v4 bool.
        let old =
            ccip_common::v1::load_token_admin_registry_checked(&acc_info, ctx.accounts.mint.key())?;

        // Extend the account to the new size (realloc)
        {
            let required_space = ANCHOR_DISCRIMINATOR + TokenAdminRegistry::INIT_SPACE;
            let minimum_balance = Rent::get()?.minimum_balance(required_space);
            let current_lamports = acc_info.lamports();

            if current_lamports < minimum_balance {
                system_program::transfer(
                    CpiContext::new(
                        ctx.accounts.system_program.key(),
                        system_program::Transfer {
                            from: ctx.accounts.authority.to_account_info(),
                            to: acc_info.clone(),
                        },
                    ),
                    minimum_balance.checked_sub(current_lamports).unwrap(),
                )?;
            }
            acc_info.resize(required_space)?;
        }

        // Upgrade data from v1/v2/v3 to v4. The loader already mapped every field:
        // - account_resolution_enabled: false for v1 (no such field), kept for v2/v3
        // - interface_version: 1 for v1/v2 (they only served pools implementing the v1 interface), kept for v3
        // - skip_lookup_table_additional_entries: set only if a v3 registry held WithoutExtendedLookupTable
        let old_version = old.version;
        let new_token_admin_registry = TokenAdminRegistry {
            version: TokenAdminRegistry::LATEST_VERSION,
            bump, // set the bump, introduced in v3 (v1/v2 load it as unset)
            ..old
        };

        // Persist the upgraded account data
        new_token_admin_registry.try_serialize(&mut &mut acc_info.try_borrow_mut_data()?[..])?;

        emit!(crate::events::PdaUpgraded {
            address: acc_info.key(),
            old_version,
            new_version: TokenAdminRegistry::LATEST_VERSION,
            name: "TokenAdminRegistry".to_string(),
            seeds: [
                seed::TOKEN_ADMIN_REGISTRY,
                new_token_admin_registry.mint.as_ref()
            ]
            .concat(),
        });

        Ok(())
    }

    fn set_pool(
        &self,
        ctx: Context<SetPoolTokenAdminRegistry>,
        writable_indexes: Vec<u8>,
    ) -> Result<()> {
        let token_mint = ctx.accounts.mint.key().to_owned();

        // Enable to overwrite the lookup table pool with 0 to delist from CCIP
        let token_admin_registry = &mut ctx.accounts.token_admin_registry;
        let previous_pool = token_admin_registry.lookup_table;
        let new_pool = ctx.accounts.pool_lookuptable.key();
        token_admin_registry.lookup_table = new_pool;

        // set writable indexes
        // used to build offchain tokenpool additional accounts in the token transactions
        // indexes can be checked to indicate is_writable for the specific account
        // this is also used to validate to ensure the correct write permissions are used according the admin
        token_admin_registry_writable::reset(token_admin_registry);
        for ind in writable_indexes {
            token_admin_registry_writable::set(token_admin_registry, ind)
        }

        // validate lookup table contains minimum required accounts if not zero address
        if new_pool != Pubkey::default() {
            // Verify the account is actually owned by the address lookup table program.
            // Without this check, an admin could register an account that happens to
            // deserialize as a valid AddressLookupTable but is not a real one, causing
            // runtime failures later during CCIP token transfers.
            require_keys_eq!(
                *ctx.accounts.pool_lookuptable.owner,
                address_lookup_table::program::id(),
                CcipRouterError::InvalidInputsLookupTableAccounts
            );

            // deserialize lookup table account
            let lookup_table_data = &mut &ctx.accounts.pool_lookuptable.data.borrow()[..];
            let lookup_table_account: AddressLookupTable =
                AddressLookupTable::deserialize(lookup_table_data)
                    .map_err(|_| CcipRouterError::InvalidInputsLookupTableAccounts)?;
            require_gte!(
                lookup_table_account.addresses.len(),
                MINIMUM_TOKEN_POOL_ACCOUNTS,
                CcipRouterError::InvalidInputsLookupTableAccounts
            );

            // The mandatory accounts (PDAs) are stored in the lookup table to save space even if they can be inferred
            let token_admin_registry = seed::token_admin_registry(&token_mint, *ctx.program_id);
            let pool_program = lookup_table_account.addresses[2]; // cannot be calculated, can be custom per pool
            let token_program = lookup_table_account.addresses[6]; // cannot be calculated, can be custom per token
            let (pool_config, _) = Pubkey::find_program_address(
                &[seed::POOL_STATE_SEED, token_mint.as_ref()],
                &pool_program,
            );
            let (pool_signer, _) = Pubkey::find_program_address(
                &[seed::POOL_SIGNER_SEED, token_mint.as_ref()],
                &pool_program,
            );
            let (fee_billing_config, _) = Pubkey::find_program_address(
                &[seed::FEE_BILLING_TOKEN_CONFIG, token_mint.as_ref()],
                &ctx.accounts.config.fee_quoter,
            );
            // The router's signer for pool CPIs. The runtime token account validators
            // (both v1 and v2) require it as the tenth mandatory LUT entry, so it must be
            // validated here too or registration would accept unusable lookup tables.
            let (ccip_router_pool_signer, _) = Pubkey::find_program_address(
                &[seed::EXTERNAL_TOKEN_POOLS_SIGNER, pool_program.as_ref()],
                ctx.program_id,
            );

            // verify token mint owner matches token program
            require_keys_eq!(
                *ctx.accounts.mint.to_account_info().owner,
                token_program,
                CcipRouterError::InvalidInputsMint
            );

            let min_accounts = [
                ctx.accounts.pool_lookuptable.key(),
                token_admin_registry,
                pool_program,
                pool_config,
                get_associated_token_address_with_program_id(
                    &pool_signer.key(),
                    &token_mint.key(),
                    &token_program.key(),
                ),
                pool_signer,
                token_program,
                token_mint,
                fee_billing_config,
                ccip_router_pool_signer,
            ];

            for (i, acc) in min_accounts.iter().enumerate() {
                // for easier debugging UX
                if lookup_table_account.addresses[i] != *acc {
                    sol_log(&i.to_string());
                    sol_log(&acc.to_string());
                    sol_log(&lookup_table_account.addresses[i].to_string());
                }
                require_eq!(
                    lookup_table_account.addresses[i],
                    *acc,
                    CcipRouterError::InvalidInputsLookupTableAccounts
                );
            }
        }

        emit!(events::PoolSet {
            token: token_mint,
            previous_pool_lookup_table: previous_pool,
            new_pool_lookup_table: new_pool,
        });

        Ok(())
    }
}

fn init_with_pending_administrator(
    token_admin_registry: &mut Account<'_, TokenAdminRegistry>,
    bump: u8,
    token_mint: Pubkey,
    new_admin: Pubkey,
) -> Result<()> {
    require_neq!(
        new_admin,
        Pubkey::new_from_array([0; 32]),
        CcipRouterError::InvalidTokenAdminRegistryInputsZeroAddress
    );

    token_admin_registry.set_inner(TokenAdminRegistry {
        version: TokenAdminRegistry::LATEST_VERSION,
        administrator: Pubkey::new_from_array([0; 32]),
        pending_administrator: new_admin,
        lookup_table: Pubkey::new_from_array([0; 32]),
        writable_indexes: [0, 0],
        mint: token_mint,
        account_resolution_enabled: TokenAdminRegistry::DEFAULT_ACCOUNT_RESOLUTION_ENABLED,
        interface_version: TokenAdminRegistry::DEFAULT_INTERFACE_VERSION,
        bump,
        skip_lookup_table_additional_entries:
            TokenAdminRegistry::DEFAULT_SKIP_LOOKUP_TABLE_ADDITIONAL_ENTRIES,
    });

    emit!(events::AdministratorTransferRequested {
        token: token_mint,
        current_admin: Pubkey::new_from_array([0; 32]),
        new_admin,
    });

    Ok(())
}
