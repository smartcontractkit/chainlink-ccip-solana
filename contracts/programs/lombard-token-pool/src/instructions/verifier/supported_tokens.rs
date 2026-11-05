use anchor_lang::prelude::*;
use base_token_pool::constants::valid_version;
use base_token_pool::errors::CcipTokenPoolError;

use crate::constants::VERIFIER_CONFIG_SEED;
use crate::errors::LombardTokenPoolError;
use crate::events::VerifierSupportedTokensUpdated;
use crate::state::{SupportedToken, VerifierConfig, MAX_VERIFIER_CONFIG_V};

/// Adds/updates/removes supported local tokens
pub fn set_verifier_supported_tokens(
    ctx: Context<SetVerifierSupportedTokens>,
    tokens_to_remove: Vec<Pubkey>,
    tokens_to_set: Vec<SupportedToken>,
) -> Result<()> {
    // No underlying mechanism exists to actually substitute a token identity when calling the Lombard bridge unlike EVM's bridge.
    require!(
        tokens_to_set.iter().all(|t| t.local_adapter.is_none()),
        LombardTokenPoolError::LocalAdapterNotSupported
    );

    let config = &mut ctx.accounts.verifier_config;

    config
        .supported_tokens
        .retain(|t| !tokens_to_remove.contains(&t.local_token));

    for token in &tokens_to_set {
        if let Some(existing) = config
            .supported_tokens
            .iter_mut()
            .find(|t| t.local_token == token.local_token)
        {
            existing.local_adapter = token.local_adapter;
        } else {
            config.supported_tokens.push(*token);
        }
    }

    emit!(VerifierSupportedTokensUpdated {
        tokens_removed: tokens_to_remove,
        tokens_set: tokens_to_set,
    });

    Ok(())
}

#[derive(Accounts)]
#[instruction(tokens_to_remove: Vec<Pubkey>, tokens_to_set: Vec<SupportedToken>)]
pub struct SetVerifierSupportedTokens<'info> {
    #[account(
        mut,
        seeds = [VERIFIER_CONFIG_SEED],
        bump = verifier_config.bump,
        constraint = valid_version(verifier_config.version, MAX_VERIFIER_CONFIG_V) @ CcipTokenPoolError::InvalidVersion,
        // Worst-case realloc: every incoming token is new (no removals overlapping). Slightly
        // over-allocates when updating/removing existing entries, which is safe (rent is repaid
        // on later shrink-reallocs elsewhere in this account's lifecycle, matching the pattern
        // used for other dynamic-Vec accounts in this program).
        realloc = VerifierConfig::DISCRIMINATOR.len() + VerifierConfig::INIT_SPACE + (verifier_config.supported_tokens.len() + tokens_to_set.len()) * SupportedToken::INIT_SPACE,
        realloc::payer = authority,
        realloc::zero = false,
    )]
    pub verifier_config: Account<'info, VerifierConfig>,

    #[account(mut, address = verifier_config.owner @ CcipTokenPoolError::Unauthorized)]
    pub authority: Signer<'info>,

    pub system_program: Program<'info, System>,
}
