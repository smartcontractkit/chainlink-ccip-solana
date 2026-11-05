use anchor_lang::prelude::*;
use ccip_common::messages::FinalityConfig;

use crate::error::ExecutorError;
use crate::event::{
    CCVAdded, CCVAllowlistUpdated, CCVRemoved, ConfigSet, OwnershipTransferRequested,
    OwnershipTransferred,
};
use crate::state::{valid_version, Config, MAX_ALLOWED_CCVS, MAX_CONFIG_V};

use super::context::{InitializeConfig, UpdateConfig};

#[derive(AnchorSerialize, AnchorDeserialize, Debug)]
pub struct InitializeParams {
    max_ccvs_per_msg: u8,
    fee_aggregator: Pubkey,
    allowed_finality_config: FinalityConfig,
    ccv_allowlist_enabled: bool,
}

pub fn initialize(ctx: Context<InitializeConfig>, params: InitializeParams) -> Result<()> {
    require_gt!(params.max_ccvs_per_msg, 0, ExecutorError::InvalidMaxCCVs);

    ctx.accounts.config.set_inner(Config {
        version: MAX_CONFIG_V,
        bump: ctx.bumps.config,
        owner: ctx.accounts.authority.key(),
        proposed_owner: Pubkey::default(),
        max_ccvs_per_msg: params.max_ccvs_per_msg,
        fee_aggregator: params.fee_aggregator,
        allowed_finality_config: params.allowed_finality_config,
        ccv_allowlist_enabled: params.ccv_allowlist_enabled,
        allowed_ccvs: Vec::new(),
    });

    // Matches EVM, whose `Executor` constructor routes through `_setDynamicConfig` and so emits
    // `ConfigSet(dynamicConfig)` at deploy time. `max_ccvs_per_msg` is left out on purpose: it is
    // immutable on EVM (`i_maxCCVsPerMsg`) and not part of `DynamicConfig`. No ownership event
    // either — EVM's `Ownable2Step` constructor emits nothing for the initial owner.
    //
    // See `InitializeConfig` for why this context hand-rolls `event_authority` instead of using
    // `#[event_cpi]`.
    emit_cpi!(ConfigSet {
        fee_aggregator: params.fee_aggregator,
        allowed_finality_config: params.allowed_finality_config,
        ccv_allowlist_enabled: params.ccv_allowlist_enabled,
    });

    Ok(())
}

#[derive(AnchorSerialize, AnchorDeserialize, InitSpace, PartialEq, Copy, Clone, Debug)]
pub struct SetConfigParams {
    pub fee_aggregator: Pubkey,
    pub allowed_finality_config: FinalityConfig,
    pub ccv_allowlist_enabled: bool,
}

pub fn set_config(ctx: Context<UpdateConfig>, params: SetConfigParams) -> Result<()> {
    ctx.accounts.config.fee_aggregator = params.fee_aggregator;
    ctx.accounts.config.allowed_finality_config = params.allowed_finality_config;
    ctx.accounts.config.ccv_allowlist_enabled = params.ccv_allowlist_enabled;

    emit_cpi!(ConfigSet {
        fee_aggregator: params.fee_aggregator,
        allowed_finality_config: params.allowed_finality_config,
        ccv_allowlist_enabled: params.ccv_allowlist_enabled
    });

    Ok(())
}

/// Upper bound on `CCVAdded` + `CCVRemoved` emissions in a single call. Each `emit_cpi!` is a
/// self-CPI, i.e. one more instruction in the transaction, and the Solana runtime caps a
/// transaction at 64 instructions total. Left well under that so the top-level instruction, the
/// `CCVAllowlistUpdated` emission and any compute-budget instructions still fit. EVM has no
/// equivalent limit because its allowlist API is delta-based, so the caller sizes each batch.
const MAX_CCV_ALLOWLIST_CHURN: usize = 48;

#[derive(AnchorSerialize, AnchorDeserialize, Clone)]
pub struct SetAllowedCcvsParams {
    pub ccvs: Vec<Pubkey>,
    pub allowlist_enabled: bool,
}

/// Replaces the CCV allowlist wholesale.
///
/// EVM has no counterpart to this shape: `Executor.applyAllowedCCVUpdates(ccvsToRemove, ccvsToAdd,
/// ccvAllowlistEnabled)` takes a delta rather than a full list. This keeps the replace-style
/// signature and derives the delta here, so the events that come out match EVM's one for one. See
/// [`apply_allowed_ccv_updates`] for the delta-shaped instruction that mirrors EVM's signature
/// directly; the two write the same state and emit the same events, so callers can pick whichever
/// fits — a full rewrite in one transaction here, or an incremental change there.
pub fn set_allowed_ccvs(
    mut ctx: Context<UpdateConfig>,
    mut params: SetAllowedCcvsParams,
) -> Result<()> {
    params.ccvs.sort_unstable();
    // NOTE: replace windows calls with array_windows::<2> when anchor gets updated
    let duplicates = params.ccvs.windows(2).any(|pair| pair[0] == pair[1]);
    require!(!duplicates, ExecutorError::DuplicateCCV);
    // EVM rejects the zero address in `applyAllowedCCVUpdates` with `InvalidCCV`.
    require!(
        !params.ccvs.contains(&Pubkey::default()),
        ExecutorError::InvalidCCV
    );

    // Linear scans rather than a sorted merge: both lists are sorted in practice, but leaning on
    // that would make this depend on an invariant nothing else enforces, and allowlists are small.
    let previous = std::mem::replace(&mut ctx.accounts.config.allowed_ccvs, params.ccvs);
    let current = &ctx.accounts.config.allowed_ccvs;
    let removed: Vec<Pubkey> = previous
        .iter()
        .filter(|ccv| !current.contains(ccv))
        .copied()
        .collect();
    let added: Vec<Pubkey> = current
        .iter()
        .filter(|ccv| !previous.contains(ccv))
        .copied()
        .collect();

    commit_allowlist_changes(&mut ctx, removed, added, params.allowlist_enabled)
}

#[derive(AnchorSerialize, AnchorDeserialize, Clone)]
pub struct ApplyAllowedCcvUpdatesParams {
    pub ccvs_to_remove: Vec<Pubkey>,
    pub ccvs_to_add: Vec<Pubkey>,
    pub allowlist_enabled: bool,
}

/// Applies a delta to the CCV allowlist. Same signature and same semantics as EVM's
/// `Executor.applyAllowedCCVUpdates(ccvsToRemove, ccvsToAdd, ccvAllowlistEnabled)`:
///
/// - removals are processed first, then additions, then the flag;
/// - both lists are idempotent — removing a CCV that isn't allowlisted, or adding one that already
///   is, changes nothing and emits nothing, matching `EnumerableSet.remove`/`add` returning false
///   on EVM (which is why duplicates *within* a list are tolerated too);
/// - a CCV that appears in both lists ends up allowlisted, because additions come second;
/// - the zero pubkey is rejected in `ccvs_to_add` with `InvalidCCV`, and (as on EVM) not checked in
///   `ccvs_to_remove`, where it can only ever be a no-op.
///
/// [`set_allowed_ccvs`] is the replace-style alternative over the same state.
pub fn apply_allowed_ccv_updates(
    mut ctx: Context<UpdateConfig>,
    params: ApplyAllowedCcvUpdatesParams,
) -> Result<()> {
    // Linear scans rather than a sorted lookup: the allowlist holds at most MAX_ALLOWED_CCVS
    // entries and nothing enforces an ordering invariant on it.
    let allowed = &mut ctx.accounts.config.allowed_ccvs;

    // The mutations are collected first and emitted afterwards because `emit_cpi!` borrows
    // `ctx.accounts` immutably, which can't overlap the mutable borrow of the allowlist.
    let mut removed: Vec<Pubkey> = Vec::new();
    for ccv in params.ccvs_to_remove {
        if let Some(i) = allowed.iter().position(|allowed_ccv| *allowed_ccv == ccv) {
            allowed.remove(i);
            removed.push(ccv);
        }
    }

    let mut added: Vec<Pubkey> = Vec::new();
    for ccv in params.ccvs_to_add {
        require_keys_neq!(ccv, Pubkey::default(), ExecutorError::InvalidCCV);
        if !allowed.contains(&ccv) {
            allowed.push(ccv);
            added.push(ccv);
        }
    }
    allowed.sort_unstable();

    commit_allowlist_changes(&mut ctx, removed, added, params.allowlist_enabled)
}

/// Shared tail of both allowlist instructions: the allowlist itself is already mutated in place by
/// the caller, so this only validates the outcome and emits, in the order EVM emits in — removals,
/// then additions, then the flag.
///
/// `removed`/`added` are the CCVs that actually left and entered the list, which is both what EVM
/// emits and what the churn bound counts, so a batch of no-ops can't be rejected for churn it
/// doesn't cause.
fn commit_allowlist_changes<'a, 'info>(
    ctx: &mut Context<'a, UpdateConfig<'info>>,
    removed: Vec<Pubkey>,
    added: Vec<Pubkey>,
    allowlist_enabled: bool,
) -> Result<()> {
    require_gte!(
        MAX_ALLOWED_CCVS,
        ctx.accounts.config.allowed_ccvs.len(),
        ExecutorError::TooManyCCVs
    );
    require_gte!(
        MAX_CCV_ALLOWLIST_CHURN,
        removed.len() + added.len(),
        ExecutorError::TooManyCCVUpdates
    );

    for ccv in removed {
        emit_cpi!(CCVRemoved { ccv });
    }
    for ccv in added {
        emit_cpi!(CCVAdded { ccv });
    }

    // EVM only emits when the flag actually changes.
    if ctx.accounts.config.ccv_allowlist_enabled != allowlist_enabled {
        ctx.accounts.config.ccv_allowlist_enabled = allowlist_enabled;
        emit_cpi!(CCVAllowlistUpdated {
            enabled: allowlist_enabled
        });
    }

    Ok(())
}

pub fn transfer_ownership(ctx: Context<UpdateConfig>, proposed_owner: Pubkey) -> Result<()> {
    let config = &mut ctx.accounts.config;

    require_keys_neq!(
        proposed_owner,
        Pubkey::default(),
        ExecutorError::DefaultOwnerProposal
    );
    require_keys_neq!(
        proposed_owner,
        config.owner,
        ExecutorError::RedundantOwnerProposal
    );

    emit_cpi!(OwnershipTransferRequested {
        from: config.owner,
        to: proposed_owner,
    });

    config.proposed_owner = proposed_owner;
    Ok(())
}

pub fn accept_ownership(ctx: Context<AcceptOwnership>) -> Result<()> {
    let config = &mut ctx.accounts.config;

    emit_cpi!(OwnershipTransferred {
        from: config.owner,
        to: config.proposed_owner,
    });

    config.owner = std::mem::take(&mut config.proposed_owner);
    Ok(())
}

#[event_cpi]
#[derive(Accounts)]
pub struct AcceptOwnership<'info> {
    #[account(
        mut,
        seeds = [ccip_common::seed::CONFIG],
        bump = config.bump,
        constraint = valid_version(config.version, MAX_CONFIG_V) @ ExecutorError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,

    #[account(address = config.proposed_owner @ ExecutorError::Unauthorized)]
    pub authority: Signer<'info>,
}
