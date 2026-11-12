use anchor_lang::prelude::*;
use anchor_spl::token_interface::Mint;

use base_token_pool::errors::CcipTokenPoolError;
use base_token_pool::state::BaseConfig;
use base_token_pool::v2::{ReleaseOrMintInV2, TokenPoolCpiResolutionInputs};
use ccip_common::resolution::*;
use ccip_common::seed;

use super::super::resolve_accounts_start::NestedResolveAccountsStart;

/// Account resolution for the `release_or_mint_tokens_v2` instruction.
///
/// Besides its `#[event_cpi]` accounts, the instruction needs the mint authority among its
/// remaining accounts whenever that authority is a multisig rather than the pool signer, and only
/// the mint account says which one it is. The caller starts with just the pool's `State`, so a
/// round without the mint asks for it and saves nothing, and the round that has it returns every
/// account. Both rounds take the same inputs; what tells them apart is whether the mint was passed.
///
/// The pool's lookup table cannot deliver the multisig on a 2.0 lane instead. Its additional
/// entries reach the pool ahead of the accounts resolved here and would shift the `#[event_cpi]`
/// pair, and the same table serves `lock_or_burn_tokens_v2`, whose resolved accounts they would
/// shift too.
pub fn start(
    ctx: Context<NestedResolveAccountsStart>,
    release_or_mint: &ReleaseOrMintInV2,
) -> Result<NestedResolveAccountsResponse> {
    resolve(
        ctx.accounts.state.key(),
        &ctx.accounts.state.config,
        ctx.remaining_accounts.first(),
        release_or_mint,
    )
}

fn resolve(
    state: Pubkey,
    config: &BaseConfig,
    mint: Option<&AccountInfo>,
    release_or_mint: &ReleaseOrMintInV2,
) -> Result<NestedResolveAccountsResponse> {
    let Some(mint) = mint else {
        return Ok(NestedResolveAccountsResponse {
            ask_again_with: vec![state, config.mint],
            accounts_to_save: vec![],
            lookup_tables_to_save: vec![],
            next_cpi_resolution_inputs: serialize_metadata(
                &TokenPoolCpiResolutionInputs::ReleaseOrMint(Box::new(release_or_mint.clone())),
            )?,
        });
    };

    let mut accounts_to_save = vec![
        // These could've easily been derived by the offramp resolver, but we leave them here to
        // prove nested resolution. In the case of a real third party token pool, they won't be
        // mandatory anyway as they depend on whether the pool emits CPI events.
        find(&[seed::EVENT_AUTHORITY], crate::ID).readonly(),
        crate::ID.readonly(),
    ];

    // A mint without an authority can't be minted to at all; the instruction reports that itself.
    if let Some(mint_authority) = mint_authority(mint, config)? {
        if mint_authority != config.pool_signer {
            accounts_to_save.push(mint_authority.readonly());
        }
    }

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
    use base_token_pool::state::RemoteAddress;
    use ccip_common::messages::{FinalityConfig, SVMTokenReceiverUsage};

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

    fn release_or_mint(mint: Pubkey) -> ReleaseOrMintInV2 {
        ReleaseOrMintInV2 {
            original_sender: RemoteAddress {
                address: vec![1; 20],
            },
            remote_chain_selector: 1,
            receiver: Pubkey::new_unique(),
            amount: 1u64.into(),
            local_token: mint,
            source_pool_address: RemoteAddress {
                address: vec![2; 32],
            },
            source_pool_data: vec![0; 32],
            requested_finality_config: FinalityConfig::default(),
            receiver_usage: SVMTokenReceiverUsage::DeriveAtaAndCreate,
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

    fn saved(response: &NestedResolveAccountsResponse) -> Vec<Pubkey> {
        response.accounts_to_save.iter().map(|a| a.pubkey).collect()
    }

    fn event_cpi_accounts() -> Vec<Pubkey> {
        vec![find(&[seed::EVENT_AUTHORITY], crate::ID), crate::ID]
    }

    #[test]
    fn without_the_mint_it_asks_for_it_with_the_same_inputs() {
        let config = config(spl_token::ID);
        let state = Pubkey::new_unique();
        let inputs = release_or_mint(config.mint);

        let response = resolve(state, &config, None, &inputs).unwrap();

        assert_eq!(response.ask_again_with, vec![state, config.mint]);
        assert!(response.accounts_to_save.is_empty());
        assert!(response.lookup_tables_to_save.is_empty());
        // The next round must be told what this one was, or the caller ends resolution here.
        match TokenPoolCpiResolutionInputs::try_from_slice(&response.next_cpi_resolution_inputs)
            .unwrap()
        {
            TokenPoolCpiResolutionInputs::ReleaseOrMint(forwarded) => {
                assert_eq!(forwarded.receiver, inputs.receiver);
                assert_eq!(forwarded.local_token, inputs.local_token);
            }
            other => panic!("forwarded the wrong inputs: {other:?}"),
        }
    }

    #[test]
    fn a_multisig_mint_authority_is_returned_after_the_event_cpi_accounts() {
        for token_program in [spl_token::ID, spl_token_2022::ID] {
            let config = config(token_program);
            let multisig = Pubkey::new_unique();
            let mut lamports = 0;
            let mut data = packed_mint(Some(multisig));
            let mint = AccountInfo::new(
                &config.mint,
                false,
                false,
                &mut lamports,
                &mut data,
                &token_program,
                false,
            );

            let response = resolve(
                Pubkey::new_unique(),
                &config,
                Some(&mint),
                &release_or_mint(config.mint),
            )
            .unwrap();

            // The `#[event_cpi]` pair must come first: the instruction expects it right after its
            // named accounts, and finds the multisig anywhere among the remaining ones.
            let mut expected = event_cpi_accounts();
            expected.push(multisig);
            assert_eq!(saved(&response), expected);
            assert!(response
                .accounts_to_save
                .iter()
                .all(|a| !a.is_writable && !a.is_signer));
            assert!(response.ask_again_with.is_empty());
            assert!(response.next_cpi_resolution_inputs.is_empty());
        }
    }

    #[test]
    fn the_pool_signer_as_mint_authority_needs_nothing_more() {
        let config = config(spl_token::ID);
        let mut lamports = 0;
        let mut data = packed_mint(Some(config.pool_signer));
        let mint = AccountInfo::new(
            &config.mint,
            false,
            false,
            &mut lamports,
            &mut data,
            &spl_token::ID,
            false,
        );

        let response = resolve(
            Pubkey::new_unique(),
            &config,
            Some(&mint),
            &release_or_mint(config.mint),
        )
        .unwrap();

        assert_eq!(saved(&response), event_cpi_accounts());
        assert!(response.next_cpi_resolution_inputs.is_empty());
    }

    #[test]
    fn a_mint_without_an_authority_is_left_to_the_instruction() {
        let config = config(spl_token::ID);
        let mut lamports = 0;
        let mut data = packed_mint(None);
        let mint = AccountInfo::new(
            &config.mint,
            false,
            false,
            &mut lamports,
            &mut data,
            &spl_token::ID,
            false,
        );

        let response = resolve(
            Pubkey::new_unique(),
            &config,
            Some(&mint),
            &release_or_mint(config.mint),
        )
        .unwrap();

        assert_eq!(saved(&response), event_cpi_accounts());
    }

    #[test]
    fn a_mint_other_than_the_pools_is_rejected() {
        let config = config(spl_token::ID);
        let other_mint = Pubkey::new_unique();
        let mut lamports = 0;
        let mut data = packed_mint(Some(Pubkey::new_unique()));
        let mint = AccountInfo::new(
            &other_mint,
            false,
            false,
            &mut lamports,
            &mut data,
            &spl_token::ID,
            false,
        );

        assert!(resolve(
            Pubkey::new_unique(),
            &config,
            Some(&mint),
            &release_or_mint(config.mint)
        )
        .is_err());
    }

    #[test]
    fn a_mint_owned_by_another_token_program_is_rejected() {
        let config = config(spl_token::ID);
        let mut lamports = 0;
        let mut data = packed_mint(Some(Pubkey::new_unique()));
        let mint = AccountInfo::new(
            &config.mint,
            false,
            false,
            &mut lamports,
            &mut data,
            &spl_token_2022::ID,
            false,
        );

        assert!(resolve(
            Pubkey::new_unique(),
            &config,
            Some(&mint),
            &release_or_mint(config.mint)
        )
        .is_err());
    }
}
