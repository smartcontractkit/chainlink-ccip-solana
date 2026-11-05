use anchor_lang::prelude::*;
use anchor_spl::{
    associated_token::get_associated_token_address_with_program_id, token_interface::Mint,
};
use ccip_common::state::PoolChainConfigOverrideType;
use ccip_common::types::{CrossChainGas, UsdCents};
use std::ops::Deref;

use crate::rate_limiter::{RateLimitConfig, RateLimitTokenBucket};

use crate::constants::*;
use crate::errors::*;
use crate::events::*;

#[derive(InitSpace, AnchorSerialize, AnchorDeserialize, Clone)]
pub struct BaseConfig {
    // token config
    pub token_program: Pubkey,
    pub mint: Pubkey,
    pub decimals: u8,
    pub pool_signer: Pubkey,        // PDA for token ownership
    pub pool_token_account: Pubkey, // associated token account for pool_signer + token

    // ownership
    pub owner: Pubkey,
    pub proposed_owner: Pubkey,
    pub rate_limit_admin: Pubkey,
    pub router_onramp_authority: Pubkey, // signer PDA for CCIP onramp calls
    pub router: Pubkey,                  // router address

    // rebalancing - only used for lock/release pools
    /// Set the rebalancer address for the pool; if set to the default "no rebalance" address
    /// (Pubkey::default()), no rebalance is allowed.
    pub rebalancer: Pubkey,
    /// DEPRECATED: this field is no longer enforced at runtime. It is kept for backward
    /// compatibility (no PDA migration) and is always initialized to `true`.
    /// To disable inflows operationally, set the rebalancer to `Pubkey::default()`.
    pub can_accept_liquidity: bool,

    // lockOrBurn allow list
    pub list_enabled: bool,
    #[max_len(0)]
    // max_len = 0 for InitSpace calculation, the actual length is calculated using the length of the allow_list. `realloc` should be used to handle the increase in account size accordingly
    pub allow_list: Vec<Pubkey>,

    // RMN Remote
    pub rmn_remote: Pubkey,
}

impl BaseConfig {
    pub fn init(
        mint: &InterfaceAccount<Mint>,
        pool_program: Pubkey,
        owner: Pubkey,
        router: Pubkey,
        rmn_remote: Pubkey,
    ) -> Self {
        let token_info = mint.to_account_info();
        let token_program = *token_info.owner;

        let (pool_signer, _) =
            Pubkey::find_program_address(&[POOL_SIGNER_SEED, mint.key().as_ref()], &pool_program);

        let (router_onramp_authority, _) = Pubkey::find_program_address(
            &[EXTERNAL_TOKEN_POOLS_SIGNER, pool_program.as_ref()],
            &router,
        );

        let pool_token_account =
            get_associated_token_address_with_program_id(&pool_signer, &mint.key(), &token_program);

        emit!(TokenPoolInitialized {
            mint: mint.key(),
            token_program,
            owner,
        });

        Self {
            token_program,
            mint: mint.key(),
            decimals: mint.decimals,
            pool_signer,
            pool_token_account,
            owner,
            proposed_owner: Pubkey::default(),
            rate_limit_admin: owner,
            router_onramp_authority,
            router,
            rebalancer: Pubkey::default(),
            can_accept_liquidity: true, // DEPRECATED: always true, no longer enforced
            list_enabled: false,
            allow_list: vec![],
            rmn_remote,
        }
    }

    pub fn transfer_ownership(&mut self, proposed_owner: Pubkey) -> Result<()> {
        require_keys_neq!(
            proposed_owner,
            self.owner,
            CcipTokenPoolError::InvalidInputs
        );
        require_keys_neq!(
            proposed_owner,
            Pubkey::default(),
            CcipTokenPoolError::InvalidInputs
        );
        self.proposed_owner = proposed_owner;
        Ok(())
    }

    pub fn accept_ownership(&mut self) -> Result<()> {
        // NOTE: take() resets proposed_owner to default
        self.owner = std::mem::take(&mut self.proposed_owner);
        Ok(())
    }

    pub fn set_router(&mut self, new_router: Pubkey, pool_program: &Pubkey) -> Result<()> {
        require_keys_neq!(
            new_router,
            Pubkey::default(),
            CcipTokenPoolError::InvalidInputs
        );

        let old_router = self.router;
        self.router = new_router;
        (self.router_onramp_authority, _) = Pubkey::find_program_address(
            &[EXTERNAL_TOKEN_POOLS_SIGNER, pool_program.as_ref()],
            &new_router,
        );
        emit!(RouterUpdated {
            mint: self.mint,
            old_router,
            new_router,
        });
        Ok(())
    }

    pub fn set_rmn(&mut self, rmn_address: Pubkey) -> Result<()> {
        require_keys_neq!(
            rmn_address,
            Pubkey::default(),
            CcipTokenPoolError::InvalidInputs
        );

        let old_rmn = self.rmn_remote;
        self.rmn_remote = rmn_address;

        emit!(RmnRemoteUpdated {
            old_rmn_remote: old_rmn,
            new_rmn_remote: rmn_address,
            mint: self.mint.key(),
        });

        Ok(())
    }

    pub fn set_rebalancer(&mut self, address: Pubkey) -> Result<()> {
        self.rebalancer = address;
        Ok(())
    }

    pub fn set_rate_limit_admin(&mut self, new_rate_limit_admin: Pubkey) -> Result<()> {
        require_keys_neq!(
            new_rate_limit_admin,
            Pubkey::default(),
            CcipTokenPoolError::InvalidInputs
        );

        let old_rate_limit_admin = self.rate_limit_admin;
        self.rate_limit_admin = new_rate_limit_admin;

        emit!(RateLimitAdminSet {
            mint: self.mint,
            old_rate_limit_admin,
            new_rate_limit_admin,
        });

        Ok(())
    }
}

#[derive(InitSpace, AnchorSerialize, AnchorDeserialize, Clone)]
pub struct BaseChain {
    pub remote: RemoteConfig,
    pub inbound_rate_limit: RateLimitTokenBucket,
    pub outbound_rate_limit: RateLimitTokenBucket,
}

impl BaseChain {
    pub fn set(
        &mut self,
        remote_chain_selector: u64,
        mint: Pubkey,
        new_cfg: RemoteConfig,
    ) -> Result<()> {
        let old_mint = self.remote.token_address.clone();
        let old_pools = self.remote.pool_addresses.clone();

        self.remote = new_cfg;

        emit!(RemoteChainConfigured {
            chain_selector: remote_chain_selector,
            mint,
            token: self.remote.token_address.clone(),
            previous_token: old_mint,
            pool_addresses: self.remote.pool_addresses.clone(),
            previous_pool_addresses: old_pools,
        });
        Ok(())
    }

    pub fn append_remote_pool_addresses(
        &mut self,
        remote_chain_selector: u64,
        mint: Pubkey,
        addresses: Vec<RemoteAddress>,
    ) -> Result<()> {
        let old_pools = self.remote.pool_addresses.clone();

        for address in addresses {
            if !self.remote.pool_addresses.contains(&address) {
                self.remote.pool_addresses.push(address);
            } else {
                return Err(CcipTokenPoolError::RemotePoolAddressAlreadyExisted.into());
            }
        }

        emit!(RemotePoolsAppended {
            chain_selector: remote_chain_selector,
            mint,
            pool_addresses: self.remote.pool_addresses.clone(),
            previous_pool_addresses: old_pools,
        });
        Ok(())
    }

    pub fn set_chain_rate_limit(
        &mut self,
        remote_chain_selector: u64,
        mint: Pubkey,
        inbound: RateLimitConfig,
        outbound: RateLimitConfig,
    ) -> Result<()> {
        self.inbound_rate_limit
            .set_token_bucket_config(inbound.clone())?;
        self.outbound_rate_limit
            .set_token_bucket_config(outbound.clone())?;

        emit!(RateLimitConfigured {
            chain_selector: remote_chain_selector,
            mint,
            outbound_rate_limit: outbound,
            inbound_rate_limit: inbound,
        });

        Ok(())
    }
}

#[derive(InitSpace, Clone, AnchorSerialize, AnchorDeserialize)]
pub struct RemoteConfig {
    // Remote pool addresses are a vec to support multiple pool versions of the same token.
    // Although for a given remote chain, there should be one active pool address at a time,
    // tokens could have been sent from earlier versions of that pool.
    // On SVM, pools are expected to upgrade in place, but on EVM, earlier version have different addresses.
    // Tokens could be stuck in flight for a long time, we need to support validation of these tokens even
    // after the pool they were sent from has been decommissioned at source.
    #[max_len(0)] // used to calculate InitSpace
    pub pool_addresses: Vec<RemoteAddress>,
    pub token_address: RemoteAddress,
    pub decimals: u8, // needed to track decimals from remote to convert properly
}

impl RemoteConfig {
    pub fn space(&self) -> usize {
        Self::INIT_SPACE
            + self
                .pool_addresses
                .iter()
                .map(RemoteAddress::space)
                .sum::<usize>()
    }
}

#[derive(Default, InitSpace, Clone, AnchorDeserialize, AnchorSerialize, PartialEq, Eq, Debug)]
pub struct RemoteAddress {
    #[max_len(64)]
    pub address: Vec<u8>,
}

impl RemoteAddress {
    pub const ZERO: Self = Self {
        address: Vec::new(),
    };

    pub fn space(&self) -> usize {
        4 // vector prefix
            + self.address.len() // data length in bytes
    }

    pub fn is_zero(&self) -> bool {
        self.address.is_empty() || self.address.iter().all(|byte| *byte == 0)
    }
}

impl Deref for RemoteAddress {
    type Target = Vec<u8>;

    fn deref(&self) -> &Self::Target {
        &self.address
    }
}

// Override configuration for a remote chain.
// Stored in a separate PDA with seeds:
//   [POOL_OVERRIDE_CHAINCONFIG_SEED, chain_selector, mint, override_type]
// so different override types (e.g. FTF, future CCVs) each get their own PDA
// without migrating the existing ChainConfig.
#[derive(InitSpace, AnchorSerialize, AnchorDeserialize, Clone)]
pub struct BaseOverrideConfig {
    pub remote_chain_selector: u64,
    pub mint: Pubkey,
    pub inbound_rate_limit: RateLimitTokenBucket,
    pub outbound_rate_limit: RateLimitTokenBucket,
    pub override_type: PoolChainConfigOverrideType,
}

impl BaseOverrideConfig {
    pub fn set_rate_limit(
        &mut self,
        inbound: RateLimitConfig,
        outbound: RateLimitConfig,
    ) -> Result<()> {
        self.inbound_rate_limit
            .set_token_bucket_config(inbound.clone())?;
        self.outbound_rate_limit
            .set_token_bucket_config(outbound.clone())?;

        Ok(())
    }
}

#[derive(InitSpace, AnchorSerialize, AnchorDeserialize, Clone)]
pub struct CCVConfig {
    pub threshold_amount: u64, // In Solana this is configured per remote chain (not per token pool as in EVM) for simplicity.
    // max_len = 0 for InitSpace calculation, the actual length is calculated using the length of the vectors, `realloc` should be used to handle the increase in account size accordingly.
    #[max_len(0)]
    pub outbound_ccvs: Vec<Pubkey>, // CCVs required for outgoing messages to the remote chain.
    #[max_len(0)]
    pub threshold_outbound_ccvs: Vec<Pubkey>, // Additional CCVs that are required for outgoing messages after reaching the threshold amount.
    #[max_len(0)]
    pub inbound_ccvs: Vec<Pubkey>, // CCVs required for incoming messages from the remote chain.
    #[max_len(0)]
    pub threshold_inbound_ccvs: Vec<Pubkey>, // Additional CCVs that are required for incoming messages after reaching the threshold amount.
}

impl CCVConfig {
    pub fn dynamic_space(
        outbound_ccvs: &[Pubkey],
        threshold_outbound_ccvs: &[Pubkey],
        inbound_ccvs: &[Pubkey],
        threshold_inbound_ccvs: &[Pubkey],
    ) -> usize {
        (outbound_ccvs.len()
            + threshold_outbound_ccvs.len()
            + inbound_ccvs.len()
            + threshold_inbound_ccvs.len())
            * std::mem::size_of::<Pubkey>()
    }
}

#[derive(InitSpace, AnchorSerialize, AnchorDeserialize, Clone, Default, PartialEq, Eq)]
pub struct TokenTransferFeeConfig {
    pub dest_gas_overhead: CrossChainGas, // Gas charged to execute the token transfer on the destination chain.
    pub dest_bytes_overhead: u32,         // Data availability bytes.
    pub finality_fee: UsdCents, // Fee to charge for token transfer with default (wait-for-finality) finality, multiples of 0.01 USD.
    pub fast_finality_fee: UsdCents, // Fee to charge for token transfer with fast finality (FTF), multiples of 0.01 USD.
    // The following two fees are deducted from the transferred asset, not added on top.
    pub finality_bps_fee: u16, // Fee in basis points for default finality transfers [0-10_000].
    pub fast_finality_bps_fee: u16, // Fee in basis points for custom finality transfers [0-10_000].
    pub is_enabled: bool,
}

pub const MAX_TOKEN_TRANSFER_FEE_BPS: u16 = 10_000;
/// Basis-point denominator, as EVM's `TokenPool.BPS_DIVIDER`.
pub const BPS_DIVIDER: u16 = 10_000;
// Minimum data availability bytes for a token transfer: the destination pool's
// lock-or-burn return data is at least this size (EVM's CCIP_LOCK_OR_BURN_V1_RET_BYTES).
pub const MIN_TOKEN_TRANSFER_DEST_BYTES_OVERHEAD: u32 = 32;

impl TokenTransferFeeConfig {
    pub fn validate(&self) -> Result<()> {
        if self == &Self::default() {
            return Ok(());
        }

        require_gte!(
            MAX_TOKEN_TRANSFER_FEE_BPS,
            self.finality_bps_fee,
            CcipTokenPoolError::InvalidTokenTransferFeeBps
        );
        require_gte!(
            MAX_TOKEN_TRANSFER_FEE_BPS,
            self.fast_finality_bps_fee,
            CcipTokenPoolError::InvalidTokenTransferFeeBps
        );
        require_gte!(
            self.dest_bytes_overhead,
            MIN_TOKEN_TRANSFER_DEST_BYTES_OVERHEAD,
            CcipTokenPoolError::InvalidTokenTransferFeeDestBytesOverhead
        );
        Ok(())
    }
}
