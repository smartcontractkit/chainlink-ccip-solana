use anchor_lang::error::ErrorCode;
use anchor_lang::prelude::*;
use anchor_spl::token_interface::Mint;

use base_token_pool::errors::CcipTokenPoolError;
use base_token_pool::state::BaseConfig;
use base_token_pool::v2::ReleaseOrMintInV2;
use ccip_common::resolution::*;
use ccip_common::seed;

use super::super::resolve_accounts_start::{
    BurnMintCpiResolutionInputs, NestedResolveAccountsStart,
};

/// Stage one of account resolution for the `release_or_mint_tokens_v2` instruction.
///
/// Besides its `#[event_cpi]` accounts, which this stage returns, the instruction needs the mint
/// authority among its remaining accounts whenever that authority is a multisig rather than the
/// pool signer. Only the mint account says which one it is, and the caller starts with just the
/// pool's `State`, so this stage asks for the mint and [`with_mint`] reads it.
///
/// The pool's lookup table cannot deliver the multisig on a 2.0 lane instead. Its additional
/// entries reach the pool ahead of the accounts resolved here and would shift the `#[event_cpi]`
/// pair, and the same table serves `lock_or_burn_tokens_v2`, whose resolved accounts they would
/// shift too.
pub fn start(
    ctx: Context<NestedResolveAccountsStart>,
    _release_or_mint: &ReleaseOrMintInV2,
) -> Result<NestedResolveAccountsResponse> {
    resolve_start(ctx.accounts.state.key(), ctx.accounts.state.config.mint)
}

/// Stage two of account resolution for the `release_or_mint_tokens_v2` instruction. The mint that
/// [`start`] asked for is the first of the resolver's remaining accounts.
pub fn with_mint(
    ctx: Context<NestedResolveAccountsStart>,
) -> Result<NestedResolveAccountsResponse> {
    resolve_with_mint(&ctx.accounts.state.config, ctx.remaining_accounts.first())
}

fn resolve_start(state: Pubkey, mint: Pubkey) -> Result<NestedResolveAccountsResponse> {
    Ok(NestedResolveAccountsResponse {
        ask_again_with: vec![state, mint],
        accounts_to_save: vec![
            // These could've easily been derived by the offramp resolver, but we leave them here to
            // prove nested resolution. In the case of a real third party token pool, they won't be
            // mandatory anyway as they depend on whether the pool emits CPI events.
            find(&[seed::EVENT_AUTHORITY], crate::ID).readonly(),
            crate::ID.readonly(),
        ],
        lookup_tables_to_save: vec![],
        next_cpi_resolution_inputs: serialize_metadata(
            &BurnMintCpiResolutionInputs::ReleaseOrMintWithMint,
        )?,
    })
}

fn resolve_with_mint(
    config: &BaseConfig,
    mint: Option<&AccountInfo>,
) -> Result<NestedResolveAccountsResponse> {
    let mint = mint.ok_or(ErrorCode::AccountNotEnoughKeys)?;

    let accounts_to_save = match mint_authority(mint, config)? {
        Some(mint_authority) if mint_authority != config.pool_signer => {
            vec![mint_authority.readonly()]
        }
        // The pool signer mints on its own. A mint without an authority can't be minted to at all,
        // and the instruction reports that itself.
        _ => vec![],
    };

    Ok(NestedResolveAccountsResponse {
        ask_again_with: vec![],
        accounts_to_save,
        lookup_tables_to_save: vec![],
        next_cpi_resolution_inputs: vec![],
    })
}

fn mint_authority(mint: &AccountInfo, config: &BaseConfig) -> Result<Option<Pubkey>> {
    require_keys_eq!(mint.key(), config.mint, CcipTokenPoolError::InvalidToken);
    require_keys_eq!(
        *mint.owner,
        config.token_program,
        CcipTokenPoolError::InvalidToken
    );

    let data = mint.try_borrow_data()?;
    let mint = Mint::try_deserialize(&mut &data[..])?;
    Ok(mint.mint_authority.into())
}

#[cfg(test)]
mod tests {
    use super::*;
    use anchor_lang::solana_program::program_option::COption;
    use anchor_lang::solana_program::program_pack::Pack;
    use anchor_spl::token::spl_token;
    use anchor_spl::token_2022::spl_token_2022;

    fn config(token_program: Pubkey) -> BaseConfig {
        BaseConfig {
            token_program,
            mint: Pubkey::new_unique(),
            decimals: 9,
            pool_signer: Pubkey::new_unique(),
            pool_token_account: Pubkey::new_unique(),
            owner: Pubkey::new_unique(),
            proposed_owner: Pubkey::default(),
            rate_limit_admin: Pubkey::new_unique(),
            router_onramp_authority: Pubkey::new_unique(),
            router: Pubkey::new_unique(),
            rebalancer: Pubkey::default(),
            can_accept_liquidity: false,
            list_enabled: false,
            allow_list: vec![],
            rmn_remote: Pubkey::new_unique(),
        }
    }

    /// The packed bytes of an SPL Token mint with the given authority. The Token-2022 base layout is
    /// the same, so these also unpack as a Token-2022 mint without extensions.
    fn packed_mint(mint_authority: Option<Pubkey>) -> Vec<u8> {
        let mint = spl_token::state::Mint {
            mint_authority: mint_authority.map_or(COption::None, COption::Some),
            supply: 0,
            decimals: 9,
            is_initialized: true,
            freeze_authority: COption::None,
        };
        let mut data = vec![0; spl_token::state::Mint::LEN];
        spl_token::state::Mint::pack(mint, &mut data).unwrap();
        data
    }

    /// Runs stage two against a mint account with the given key, owner and authority.
    fn resolve_with(
        config: &BaseConfig,
        key: Pubkey,
        owner: Pubkey,
        mint_authority: Option<Pubkey>,
    ) -> Result<NestedResolveAccountsResponse> {
        let mut lamports = 0;
        let mut data = packed_mint(mint_authority);
        let mint = AccountInfo::new(&key, false, false, &mut lamports, &mut data, &owner, false);
        resolve_with_mint(config, Some(&mint))
    }

    fn saved(response: &NestedResolveAccountsResponse) -> Vec<Pubkey> {
        response.accounts_to_save.iter().map(|a| a.pubkey).collect()
    }

    #[test]
    fn stage_one_returns_the_event_cpi_accounts_and_asks_for_the_mint() {
        let state = Pubkey::new_unique();
        let mint = Pubkey::new_unique();

        let response = resolve_start(state, mint).unwrap();

        // The `#[event_cpi]` pair comes first, as the instruction expects it right after its named
        // accounts; stage two's accounts follow it.
        assert_eq!(
            saved(&response),
            vec![find(&[seed::EVENT_AUTHORITY], crate::ID), crate::ID]
        );
        assert_eq!(response.ask_again_with, vec![state, mint]);
        assert!(response.lookup_tables_to_save.is_empty());
        assert!(matches!(
            BurnMintCpiResolutionInputs::try_from_slice(&response.next_cpi_resolution_inputs)
                .unwrap(),
            BurnMintCpiResolutionInputs::ReleaseOrMintWithMint
        ));
    }

    #[test]
    fn stage_two_returns_a_multisig_mint_authority() {
        for token_program in [spl_token::ID, spl_token_2022::ID] {
            let config = config(token_program);
            let multisig = Pubkey::new_unique();

            let response =
                resolve_with(&config, config.mint, token_program, Some(multisig)).unwrap();

            assert_eq!(saved(&response), vec![multisig]);
            assert!(response
                .accounts_to_save
                .iter()
                .all(|a| !a.is_writable && !a.is_signer));
            assert!(response.ask_again_with.is_empty());
            // Nothing follows, so the caller ends resolution here.
            assert!(response.next_cpi_resolution_inputs.is_empty());
        }
    }

    #[test]
    fn stage_two_needs_nothing_more_when_the_pool_signer_mints() {
        let config = config(spl_token::ID);

        let response = resolve_with(
            &config,
            config.mint,
            spl_token::ID,
            Some(config.pool_signer),
        )
        .unwrap();

        assert!(response.accounts_to_save.is_empty());
        assert!(response.next_cpi_resolution_inputs.is_empty());
    }

    #[test]
    fn stage_two_leaves_a_mint_without_an_authority_to_the_instruction() {
        let config = config(spl_token::ID);

        let response = resolve_with(&config, config.mint, spl_token::ID, None).unwrap();

        assert!(response.accounts_to_save.is_empty());
    }

    #[test]
    fn stage_two_requires_the_mint() {
        let config = config(spl_token::ID);

        assert!(resolve_with_mint(&config, None).is_err());
    }

    #[test]
    fn stage_two_rejects_a_mint_other_than_the_pools() {
        let config = config(spl_token::ID);

        assert!(resolve_with(
            &config,
            Pubkey::new_unique(),
            spl_token::ID,
            Some(Pubkey::new_unique())
        )
        .is_err());
    }

    #[test]
    fn stage_two_rejects_a_mint_owned_by_another_token_program() {
        let config = config(spl_token::ID);

        assert!(resolve_with(
            &config,
            config.mint,
            spl_token_2022::ID,
            Some(Pubkey::new_unique())
        )
        .is_err());
    }
}
