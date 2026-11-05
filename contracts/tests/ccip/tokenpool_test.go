// tokenpool_test.go tests the token pool programs' core functionality.
// This includes lock/release and burn/mint operations, rate limiting configuration,
// remote chain setup, and pool-specific token transfer logic.
package contracts

import (
	"context"
	"testing"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/arbitrary_cpi_proxy"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/base_token_pool"
	tp "github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/burnmint_token_pool"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/rmn_remote"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/config"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/testutils"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/tokens"
)

func TestTokenPool(t *testing.T) {
	// This file covers pool ADMIN and CONFIG surface. Every transfer flow a real ramp can drive is
	// covered against the real programs instead: lock_or_burn[_v2] in onramp_test.go, and
	// release_or_mint[_v2] (including the pool's rate limit, allow list and source-pool rejections)
	// in offramp_test.go, plus the devenv Solana2EVM/EVM2Solana suites end to end. The base pools
	// here therefore run with the real CcipRouter as their router and never CPI.
	//
	// "CCTP token pool" is the one block left that still needs a mock ramp signing as the router's
	// pool signer. TODO: the 2.0 OffRamp drops offchain_token_data and the pool's
	// derived accounts on the legacy transfer path (see
	// ccip-offramp/src/instructions/execute/token_transfer.rs), and ccip_send_v2 zeroes
	// msg_total_nonce, which CCTP uses as an event-PDA seed. Until both are addressed the CCTP pool
	// cannot be driven by the real ramps at all. Once it can, this mock and the whole
	// arbitrary-cpi-proxy program can go (the examples package is its only other user).
	mockRamp := config.ArbitraryCpiProxyProgram

	type poolInfo struct {
		poolName    string
		poolProgram solana.PublicKey
		isBurnMint  bool
	}
	poolInfos := []poolInfo{
		{poolName: "LockAndRelease", poolProgram: config.CcipBasePoolLockRelease, isBurnMint: false},
		{poolName: "BurnAndMint", poolProgram: config.CcipBasePoolBurnMint, isBurnMint: true},
	}

	admin := solana.MustPrivateKeyFromBase58("4whgxZhpxcArYWzM1iTmokruAzws9YVi2f9M7pWwchQniaFXBr1WGSGXgadeqHtiRooxNiPosdLj2g2ohbtkWtu5")
	anotherAdmin := solana.MustPrivateKeyFromBase58("3T3TwgX851KixNcZ2nJDftvXb5gMckHg8zzonKXpSthdDQEtttP4iY5VwetMRmoJkMDUPqSbrypFHVV2aC9FWHKE")
	user := solana.MustPrivateKeyFromBase58("iVu6VhFc44zTZx6LceYup18rQfxqFvKWFWzJtrNpFcWHCNnbmDfSMYFbDrj9R7RZon4t6YHHmFU8Fh6461PBvfC")

	ctx := t.Context()

	// Only the SVM lane is registered: CCTP is the sole remaining mock-driven flow and it loops a
	// message back to this chain.
	allowedOfframpSvmPDA, err := state.FindAllowedOfframpPDA(config.SvmChainSelector, mockRamp, mockRamp)
	require.NoError(t, err)

	solanaGoClient := testutils.DeployAllPrograms(t, testutils.PathToAnchorConfig, admin)

	remotePool := tp.RemoteAddress{Address: []byte{1, 2, 3}}
	remoteToken := tp.RemoteAddress{Address: []byte{4, 5, 6}}

	// Common setup — executed directly (not in t.Run) so it always runs regardless of -run filters.
	require.NoError(t, testutils.FundAccounts(ctx, []solana.PrivateKey{admin, anotherAdmin, user}, solanaGoClient))

	{
		svmIx, err := arbitrary_cpi_proxy.NewAddOfframpInstruction(
			config.SvmChainSelector,
			mockRamp,
			allowedOfframpSvmPDA,
			admin.PublicKey(),
			solana.SystemProgramID,
		)
		require.NoError(t, err)

		testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{svmIx}, admin, config.DefaultCommitment)
	}

	{
		programDataAddr := testutils.GetProgramDataAddress(ctx, t, solanaGoClient, config.RMNRemoteProgram)

		ix, err := rmn_remote.NewInitializeInstruction(
			config.RMNRemoteConfigPDA,
			config.RMNRemoteCursesPDA,
			admin.PublicKey(),
			solana.SystemProgramID,
			config.RMNRemoteProgram,
			programDataAddr,
		)

		require.NoError(t, err)
		result := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)
		require.NotNil(t, result)
	}

	for _, pi := range poolInfos {
		configPDA, err := tokens.TokenPoolGlobalConfigPDA(pi.poolProgram)
		require.NoError(t, err)
		programDataAddr := testutils.GetProgramDataAddress(ctx, t, solanaGoClient, pi.poolProgram)
		ix, err := tp.NewInitGlobalConfigInstruction(
			config.CcipRouterProgram, config.RMNRemoteProgram,
			configPDA, admin.PublicKey(), solana.SystemProgramID,
			pi.poolProgram, programDataAddr,
		)
		require.NoError(t, err)
		testutils.SendAndConfirm(ctx, t, solanaGoClient,
			[]solana.Instruction{&tokens.TokenInstruction{Instruction: ix, Program: pi.poolProgram}},
			admin, config.DefaultCommitment)
	}

	// test functionality with token & token-2022 standards
	for _, v := range []struct {
		tokenName    string
		tokenProgram solana.PublicKey
	}{
		{tokenName: "spl-token", tokenProgram: solana.TokenProgramID},
		{tokenName: "spl-token-2022", tokenProgram: config.Token2022Program},
	} {
		t.Run(v.tokenName, func(t *testing.T) {
			t.Parallel()

			decimals := uint8(0)
			amount := uint64(1000)

			// V1 token pool subtests (lock_or_burn, release_or_mint)
			for _, pi := range poolInfos {
				mintPriv, err := solana.NewRandomPrivateKey()
				require.NoError(t, err)
				p, err := tokens.NewTokenPool(v.tokenProgram, pi.poolProgram, mintPriv.PublicKey())
				require.NoError(t, err)
				mint := p.Mint

				t.Run("setup:token", func(t *testing.T) {
					// create token
					instructions, err := tokens.CreateToken(ctx, v.tokenProgram, mint, admin.PublicKey(), decimals, solanaGoClient, config.DefaultCommitment)
					require.NoError(t, err)

					// create admin associated token account
					createI, tokenAccount, err := tokens.CreateAssociatedTokenAccount(v.tokenProgram, mint, admin.PublicKey(), admin.PublicKey())
					require.NoError(t, err)
					p.User[admin.PublicKey()] = tokenAccount // set admin token account

					// mint tokens to admin
					mintToI, err := tokens.MintTo(amount, v.tokenProgram, mint, tokenAccount, admin.PublicKey())
					require.NoError(t, err)

					instructions = append(instructions, createI, mintToI)
					testutils.SendAndConfirm(ctx, t, solanaGoClient, instructions, admin, config.DefaultCommitment, common.AddSigners(mintPriv))

					// validate
					outDec, outVal, err := tokens.TokenBalance(ctx, solanaGoClient, p.User[admin.PublicKey()], config.DefaultCommitment)
					require.NoError(t, err)
					require.Equal(t, int(amount), outVal) //nolint:gosec // G115
					require.Equal(t, decimals, outDec)
				})

				t.Run("pool:"+pi.poolName, func(t *testing.T) {
					poolConfig, errConfig := tokens.TokenPoolConfigAddress(mint, pi.poolProgram)
					require.NoError(t, errConfig)
					poolSigner, errSigner := tokens.TokenPoolSignerAddress(mint, pi.poolProgram)
					require.NoError(t, errSigner)
					createI, poolTokenAccount, errATA := tokens.CreateAssociatedTokenAccount(v.tokenProgram, mint, poolSigner, admin.PublicKey())
					require.NoError(t, errATA)

					wrapPool := func(ix solana.Instruction) solana.Instruction {
						return &tokens.TokenInstruction{Instruction: ix, Program: pi.poolProgram}
					}

					t.Run("setup", func(t *testing.T) {
						programDataAddr := testutils.GetProgramDataAddress(ctx, t, solanaGoClient, pi.poolProgram)

						globalConfigPDA, err := tokens.TokenPoolGlobalConfigPDA(pi.poolProgram)
						require.NoError(t, err)
						poolInitIx, err := tp.NewInitializeInstruction(
							poolConfig,
							mint,
							admin.PublicKey(),
							solana.SystemProgramID,
							pi.poolProgram,
							programDataAddr,
							globalConfigPDA,
						)
						require.NoError(t, err)
						poolInitI := wrapPool(poolInitIx)

						// make pool mint_authority for token (required for burn/mint)
						authI, err := tokens.SetTokenMintAuthority(v.tokenProgram, poolSigner, mint, admin.PublicKey())
						require.NoError(t, err)

						// set rate limit
						ixRatesRaw, err := tp.NewSetChainRateLimitInstruction(config.EvmChainSelector, p.Mint, tp.RateLimitConfig{
							Enabled:  true,
							Capacity: amount,
							Rate:     1, // slow refill
						}, tp.RateLimitConfig{
							Enabled:  false,
							Capacity: 0,
							Rate:     0,
						}, poolConfig, p.Chain[config.EvmChainSelector], admin.PublicKey())
						require.NoError(t, err)
						ixRates := wrapPool(ixRatesRaw)

						// set pool config
						ixConfigureRaw, err := tp.NewInitChainRemoteConfigInstruction(config.EvmChainSelector, p.Mint, tp.RemoteConfig{
							TokenAddress: remoteToken,
							Decimals:     9,
						}, poolConfig, p.Chain[config.EvmChainSelector], admin.PublicKey(), solana.SystemProgramID)
						require.NoError(t, err)
						ixConfigure := wrapPool(ixConfigureRaw)

						ixAppendRaw, err := tp.NewAppendRemotePoolAddressesInstruction(
							config.EvmChainSelector, p.Mint, []tp.RemoteAddress{remotePool}, poolConfig, p.Chain[config.EvmChainSelector], admin.PublicKey(), solana.SystemProgramID,
						)
						require.NoError(t, err)
						ixAppend := wrapPool(ixAppendRaw)

						res := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{poolInitI, createI, authI, ixConfigure, ixAppend, ixRates}, admin, config.DefaultCommitment)
						require.NotNil(t, res)

						var configAccount tp.State
						require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, poolConfig, config.DefaultCommitment, &configAccount))
						require.Equal(t, poolTokenAccount, configAccount.Config.PoolTokenAccount)

						eventConfigured, err := common.ParseEventFromLogs(res.Meta.LogMessages, base_token_pool.ParseEvent_RemoteChainConfigured)
						require.NoError(t, err)
						require.Equal(t, config.EvmChainSelector, eventConfigured.ChainSelector)
						require.Empty(t, eventConfigured.PoolAddresses)
						require.Empty(t, eventConfigured.PreviousPoolAddresses)
						require.Equal(t, remoteToken.Address, eventConfigured.Token.Address)
						require.Empty(t, eventConfigured.PreviousToken.Address)
						require.Equal(t, mint, eventConfigured.Mint)

						eventAppended, err := common.ParseEventFromLogs(res.Meta.LogMessages, base_token_pool.ParseEvent_RemotePoolsAppended)
						require.NoError(t, err)
						require.Equal(t, config.EvmChainSelector, eventAppended.ChainSelector)
						require.Len(t, eventAppended.PoolAddresses, 1)
						require.Equal(t, remotePool.Address, eventAppended.PoolAddresses[0].Address)
						require.Empty(t, eventAppended.PreviousPoolAddresses)
						require.Equal(t, mint, eventAppended.Mint)

						eventRateLimit, err := common.ParseEventFromLogs(res.Meta.LogMessages, base_token_pool.ParseEvent_RateLimitConfigured)
						require.NoError(t, err)
						require.Equal(t, config.EvmChainSelector, eventRateLimit.ChainSelector)
						require.True(t, eventRateLimit.InboundRateLimit.Enabled)
						require.Equal(t, amount, eventRateLimit.InboundRateLimit.Capacity)
						require.Equal(t, uint64(1), eventRateLimit.InboundRateLimit.Rate)
						require.False(t, eventRateLimit.OutboundRateLimit.Enabled)
						require.Equal(t, uint64(0), eventRateLimit.OutboundRateLimit.Capacity)
						require.Equal(t, uint64(0), eventRateLimit.OutboundRateLimit.Rate)
						require.Equal(t, mint, eventRateLimit.Mint)
					})

					t.Run("admin:setRouter", func(t *testing.T) {
						programDataAddr := testutils.GetProgramDataAddress(ctx, t, solanaGoClient, pi.poolProgram)

						// Point the router at an arbitrary key — admin is both upgrade authority and pool owner
						newRouter := solana.NewWallet().PublicKey()
						ixSetRaw, err := tp.NewSetRouterInstruction(newRouter, poolConfig, mint, admin.PublicKey(), pi.poolProgram, programDataAddr)
						require.NoError(t, err)
						res := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{wrapPool(ixSetRaw)}, admin, config.DefaultCommitment)
						require.NotNil(t, res)

						eventRouter, err := common.ParseEventFromLogs(res.Meta.LogMessages, base_token_pool.ParseEvent_RouterUpdated)
						require.NoError(t, err)
						require.Equal(t, newRouter, eventRouter.NewRouter)
						require.Equal(t, config.CcipRouterProgram, eventRouter.OldRouter)
						require.Equal(t, mint, eventRouter.Mint)

						// Restore the real router so the pool is left in the state the rest of the suite expects
						ixRestoreRaw, err := tp.NewSetRouterInstruction(config.CcipRouterProgram, poolConfig, mint, admin.PublicKey(), pi.poolProgram, programDataAddr)
						require.NoError(t, err)
						res = testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{wrapPool(ixRestoreRaw)}, admin, config.DefaultCommitment)
						require.NotNil(t, res)

						eventRouter, err = common.ParseEventFromLogs(res.Meta.LogMessages, base_token_pool.ParseEvent_RouterUpdated)
						require.NoError(t, err)
						require.Equal(t, config.CcipRouterProgram, eventRouter.NewRouter)
						require.Equal(t, newRouter, eventRouter.OldRouter)
						require.Equal(t, mint, eventRouter.Mint)
					})

					t.Run("admin:setRmn", func(t *testing.T) {
						programDataAddr := testutils.GetProgramDataAddress(ctx, t, solanaGoClient, pi.poolProgram)

						// Change RMN to CcipRouterProgram (from RMNRemoteProgram) — admin is both upgrade authority and pool owner
						ixSetRaw, err := tp.NewSetRmnInstruction(config.CcipRouterProgram, poolConfig, mint, admin.PublicKey(), pi.poolProgram, programDataAddr)
						require.NoError(t, err)
						res := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{wrapPool(ixSetRaw)}, admin, config.DefaultCommitment)
						require.NotNil(t, res)

						eventRmn, err := common.ParseEventFromLogs(res.Meta.LogMessages, base_token_pool.ParseEvent_RmnRemoteUpdated)
						require.NoError(t, err)
						require.Equal(t, config.CcipRouterProgram, eventRmn.NewRmnRemote)
						require.Equal(t, config.RMNRemoteProgram, eventRmn.OldRmnRemote)
						require.Equal(t, mint, eventRmn.Mint)

						// Restore RMN to RMNRemoteProgram so subsequent tests remain authorized
						ixRestoreRaw, err := tp.NewSetRmnInstruction(config.RMNRemoteProgram, poolConfig, mint, admin.PublicKey(), pi.poolProgram, programDataAddr)
						require.NoError(t, err)
						res = testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{wrapPool(ixRestoreRaw)}, admin, config.DefaultCommitment)
						require.NotNil(t, res)

						eventRmn, err = common.ParseEventFromLogs(res.Meta.LogMessages, base_token_pool.ParseEvent_RmnRemoteUpdated)
						require.NoError(t, err)
						require.Equal(t, config.RMNRemoteProgram, eventRmn.NewRmnRemote)
						require.Equal(t, config.CcipRouterProgram, eventRmn.OldRmnRemote)
						require.Equal(t, mint, eventRmn.Mint)
					})

					t.Run("admin:ownership", func(t *testing.T) {
						// successfully transfer ownership
						transferOwnershipIx, err := tp.NewTransferOwnershipInstruction(
							anotherAdmin.PublicKey(),
							poolConfig,
							mint,
							admin.PublicKey(),
						)
						require.NoError(t, err)
						instruction := wrapPool(transferOwnershipIx)
						result := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{instruction}, admin, config.DefaultCommitment)
						require.NotNil(t, result)

						// Successfully accept ownership
						// anotherAdmin becomes owner for remaining tests
						acceptOwnershipIx, err := tp.NewAcceptOwnershipInstruction(
							poolConfig,
							mint,
							anotherAdmin.PublicKey(),
						)
						require.NoError(t, err)
						instruction = wrapPool(acceptOwnershipIx)
						result = testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{instruction}, anotherAdmin, config.DefaultCommitment)
						require.NotNil(t, result)
					})

					t.Run("invalid", func(t *testing.T) {
						t.Run("config", func(t *testing.T) {
							t.Parallel()

							cfgs := []struct {
								name   string
								c      tp.RateLimitConfig
								errStr string
							}{
								{
									name: "enabled-rate-larger-than-capacity",
									c: tp.RateLimitConfig{
										Enabled:  true,
										Capacity: 1,
										Rate:     100,
									},
									errStr: "invalid rate limit rate",
								},
								{
									name: "disabled-nonzero-rate",
									c: tp.RateLimitConfig{
										Enabled:  false,
										Capacity: 0,
										Rate:     100,
									},
									errStr: "disabled non-zero rate limit",
								},
								{
									name: "disabled-nonzero-capacity",
									c: tp.RateLimitConfig{
										Enabled:  false,
										Capacity: 10,
										Rate:     0,
									},
									errStr: "disabled non-zero rate limit",
								},
							}

							for _, cfg := range cfgs {
								t.Run(cfg.name, func(t *testing.T) {
									t.Parallel()

									ixRatesRaw, err := tp.NewSetChainRateLimitInstruction(config.EvmChainSelector, p.Mint, cfg.c, tp.RateLimitConfig{}, poolConfig, p.Chain[config.EvmChainSelector], anotherAdmin.PublicKey())
									require.NoError(t, err)
									ixRates := wrapPool(ixRatesRaw)

									testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ixRates}, anotherAdmin, config.DefaultCommitment, []string{cfg.errStr})
								})
							}
						})

						t.Run("Rate Limit Admin", func(t *testing.T) {
							// set invalid a new rate limit admin
							ixRateAdminRaw, err := tp.NewSetRateLimitAdminInstruction(
								p.Mint,
								user.PublicKey(),
								poolConfig,
								anotherAdmin.PublicKey(),
							)
							require.NoError(t, err)
							ixRateAdmin := wrapPool(ixRateAdminRaw)

							ixRatesValidRaw, err := tp.NewSetChainRateLimitInstruction(config.EvmChainSelector, p.Mint,
								tp.RateLimitConfig{
									Enabled:  true,
									Capacity: amount,
									Rate:     1,
								}, tp.RateLimitConfig{
									Enabled:  true,
									Capacity: 10,
									Rate:     0,
								}, poolConfig, p.Chain[config.EvmChainSelector], user.PublicKey())
							require.NoError(t, err)
							ixRatesValid := wrapPool(ixRatesValidRaw)

							// undo rate limit admin
							ixRateAdmin2Raw, err := tp.NewSetRateLimitAdminInstruction(
								p.Mint,
								anotherAdmin.PublicKey(),
								poolConfig,
								anotherAdmin.PublicKey(),
							)
							require.NoError(t, err)
							ixRateAdmin2 := wrapPool(ixRateAdmin2Raw)

							// try to modify rate limit with invalid admin
							ixRatesRaw, err := tp.NewSetChainRateLimitInstruction(config.EvmChainSelector, p.Mint,
								tp.RateLimitConfig{
									Enabled:  true,
									Capacity: amount,
									Rate:     1,
								}, tp.RateLimitConfig{
									Enabled:  false,
									Capacity: 0,
									Rate:     0,
								}, poolConfig, p.Chain[config.EvmChainSelector], user.PublicKey())
							require.NoError(t, err)
							ixRates := wrapPool(ixRatesRaw)

							testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ixRateAdmin, ixRatesValid, ixRateAdmin2, ixRates}, user, config.DefaultCommitment, []string{"SetChainRateLimit"}, common.AddSigners(anotherAdmin))
						})
					})

					t.Run("closing", func(t *testing.T) {
						ixDeleteRaw, err := tp.NewDeleteChainConfigInstruction(config.EvmChainSelector, mint, poolConfig, p.Chain[config.EvmChainSelector], anotherAdmin.PublicKey())
						require.NoError(t, err)
						ixDelete := wrapPool(ixDeleteRaw)

						res := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ixDelete}, anotherAdmin, config.DefaultCommitment)
						require.NotNil(t, res)

						eventDelete, err := common.ParseEventFromLogs(res.Meta.LogMessages, base_token_pool.ParseEvent_RemoteChainRemoved)
						require.NoError(t, err)
						require.Equal(t, config.EvmChainSelector, eventDelete.ChainSelector)
						require.Equal(t, mint, eventDelete.Mint)
					})
				})
			}

			// V2 token pool subtests (lock_or_burn_v2, release_or_mint_v2)
			for _, pi := range poolInfos {
				mintPriv, err := solana.NewRandomPrivateKey()
				require.NoError(t, err)
				p, err := tokens.NewTokenPool(v.tokenProgram, pi.poolProgram, mintPriv.PublicKey())
				require.NoError(t, err)
				mint := p.Mint

				t.Run("pool-v2:"+pi.poolName, func(t *testing.T) {
					poolConfig, errConfig := tokens.TokenPoolConfigAddress(mint, pi.poolProgram)
					require.NoError(t, errConfig)
					poolSigner, errSigner := tokens.TokenPoolSignerAddress(mint, pi.poolProgram)
					require.NoError(t, errSigner)
					createPoolATA, _, errATA := tokens.CreateAssociatedTokenAccount(v.tokenProgram, mint, poolSigner, admin.PublicKey())
					require.NoError(t, errATA)

					wrapPool := func(ix solana.Instruction) solana.Instruction {
						return &tokens.TokenInstruction{Instruction: ix, Program: pi.poolProgram}
					}

					t.Run("setup", func(t *testing.T) {
						// Create token and mint to admin
						tokenIxs, err := tokens.CreateToken(ctx, v.tokenProgram, mint, admin.PublicKey(), decimals, solanaGoClient, config.DefaultCommitment)
						require.NoError(t, err)

						createUserATA, tokenAccount, err := tokens.CreateAssociatedTokenAccount(v.tokenProgram, mint, admin.PublicKey(), admin.PublicKey())
						require.NoError(t, err)
						p.User[admin.PublicKey()] = tokenAccount

						mintToI, err := tokens.MintTo(amount, v.tokenProgram, mint, tokenAccount, admin.PublicKey())
						require.NoError(t, err)

						tokenIxs = append(tokenIxs, createUserATA, mintToI)
						testutils.SendAndConfirm(ctx, t, solanaGoClient, tokenIxs, admin, config.DefaultCommitment, common.AddSigners(mintPriv))

						// Initialize pool
						programDataAddr := testutils.GetProgramDataAddress(ctx, t, solanaGoClient, pi.poolProgram)

						globalConfigPDA, err := tokens.TokenPoolGlobalConfigPDA(pi.poolProgram)
						require.NoError(t, err)
						poolInitIx, err := tp.NewInitializeInstruction(
							poolConfig,
							mint,
							admin.PublicKey(),
							solana.SystemProgramID,
							pi.poolProgram,
							programDataAddr,
							globalConfigPDA,
						)
						require.NoError(t, err)
						poolInitI := wrapPool(poolInitIx)

						authI, err := tokens.SetTokenMintAuthority(v.tokenProgram, poolSigner, mint, admin.PublicKey())
						require.NoError(t, err)

						// V2 validates both inbound and outbound rate limits
						ixRatesRaw, err := tp.NewSetChainRateLimitInstruction(config.EvmChainSelector, p.Mint, tp.RateLimitConfig{
							Enabled:  true,
							Capacity: amount,
							Rate:     amount, // fast refill for tests
						}, tp.RateLimitConfig{
							Enabled:  true,
							Capacity: amount,
							Rate:     amount, // fast refill for tests
						}, poolConfig, p.Chain[config.EvmChainSelector], admin.PublicKey())
						require.NoError(t, err)
						ixRates := wrapPool(ixRatesRaw)

						ixConfigureRaw, err := tp.NewInitChainRemoteConfigInstruction(config.EvmChainSelector, p.Mint, tp.RemoteConfig{
							TokenAddress: remoteToken,
							Decimals:     9,
						}, poolConfig, p.Chain[config.EvmChainSelector], admin.PublicKey(), solana.SystemProgramID)
						require.NoError(t, err)
						ixConfigure := wrapPool(ixConfigureRaw)

						ixAppendRaw, err := tp.NewAppendRemotePoolAddressesInstruction(
							config.EvmChainSelector, p.Mint, []tp.RemoteAddress{remotePool}, poolConfig, p.Chain[config.EvmChainSelector], admin.PublicKey(), solana.SystemProgramID,
						)
						require.NoError(t, err)
						ixAppend := wrapPool(ixAppendRaw)

						res := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{poolInitI, createPoolATA, authI, ixConfigure, ixAppend, ixRates}, admin, config.DefaultCommitment)
						require.NotNil(t, res)
					})

					t.Run("withdraw_fee_tokens", func(t *testing.T) {
						// ccip_send_v2 pays every token-pool receipt fee into the ATA of the pool's
						// ["fee_vault", mint] PDA, for whichever token the sender billed in (see
						// ccip_common::seed::vault_ata). The fee token is unrelated to the token the
						// pool wraps, so it gets its own mint here.
						feeMintPriv, err := solana.NewRandomPrivateKey()
						require.NoError(t, err)
						feeMint := feeMintPriv.PublicKey()
						feeTokenProgram := config.Token2022Program

						feeVault, _, err := state.FindFeeVaultPDA(pi.poolProgram, mint.Bytes())
						require.NoError(t, err)

						feeTokenIxs, err := tokens.CreateToken(ctx, feeTokenProgram, feeMint, admin.PublicKey(), 9, solanaGoClient, config.DefaultCommitment)
						require.NoError(t, err)

						createVaultATA, feeVaultATA, err := tokens.CreateAssociatedTokenAccount(feeTokenProgram, feeMint, feeVault, admin.PublicKey())
						require.NoError(t, err)
						expectedFeeVaultATA, err := tokens.FindFeeVaultATA(feeTokenProgram, feeMint, pi.poolProgram, mint.Bytes())
						require.NoError(t, err)
						require.Equal(t, expectedFeeVaultATA, feeVaultATA)

						createRecipientATA, recipientATA, err := tokens.CreateAssociatedTokenAccount(feeTokenProgram, feeMint, user.PublicKey(), admin.PublicKey())
						require.NoError(t, err)

						testutils.SendAndConfirm(ctx, t, solanaGoClient, append(feeTokenIxs, createVaultATA, createRecipientATA), admin, config.DefaultCommitment, common.AddSigners(feeMintPriv))

						withdrawIx := func(authority solana.PublicKey) solana.Instruction {
							ix, ixErr := tp.NewWithdrawFeeTokensInstruction(
								nil,
								poolConfig,
								mint,
								feeVault,
								feeVaultATA,
								recipientATA,
								feeMint,
								feeTokenProgram,
								authority,
							)
							require.NoError(t, ixErr)
							return wrapPool(ix)
						}

						t.Run("fails when the vault is empty", func(t *testing.T) {
							testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{withdrawIx(admin.PublicKey())}, admin, config.DefaultCommitment,
								[]string{"Error Code: " + "NoFeesToWithdraw"})
						})

						t.Run("fails for a non-owner", func(t *testing.T) {
							ixMintTo, mintErr := tokens.MintTo(1000, feeTokenProgram, feeMint, feeVaultATA, admin.PublicKey())
							require.NoError(t, mintErr)
							testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ixMintTo}, admin, config.DefaultCommitment, common.AddSigners(feeMintPriv))

							testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{withdrawIx(user.PublicKey())}, user, config.DefaultCommitment,
								[]string{"Error Code: " + "Unauthorized"})
						})

						t.Run("sweeps the vault for the owner", func(t *testing.T) {
							testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{withdrawIx(admin.PublicKey())}, admin, config.DefaultCommitment)

							_, vaultBalance, err := tokens.TokenBalance(ctx, solanaGoClient, feeVaultATA, config.DefaultCommitment)
							require.NoError(t, err)
							require.Zero(t, vaultBalance)

							_, recipientBalance, err := tokens.TokenBalance(ctx, solanaGoClient, recipientATA, config.DefaultCommitment)
							require.NoError(t, err)
							require.Equal(t, 1000, recipientBalance)
						})
					})

					t.Run("chain_config_v2_token_transfer_fee_config", func(t *testing.T) {
						chainConfigV2PDA := p.ChainConfigV2[config.EvmChainSelector]
						eventAuthority, _, _ := state.FindEventAuthorityPDA(pi.poolProgram)
						// init_chain_config_v2 creates the fee vault ATA for the pool's own mint:
						// the account lock_or_burn_tokens_v2 moves the lane's in-token BPS fee
						// into, so configuring a fee always leaves it collectable.
						feeVault, _, err := state.FindFeeVaultPDA(pi.poolProgram, mint.Bytes())
						require.NoError(t, err)
						feeVaultATA, err := tokens.FindFeeVaultATA(v.tokenProgram, mint, pi.poolProgram, mint.Bytes())
						require.NoError(t, err)

						initialCfg := tp.TokenTransferFeeConfig{
							DestGasOverhead:    tp.CrossChainGas{V0: 90000},
							DestBytesOverhead:  32,
							FinalityFee:        tp.UsdCents{V0: 50},
							FastFinalityFee:    tp.UsdCents{V0: 75},
							FinalityBpsFee:     10,
							FastFinalityBpsFee: 20,
							IsEnabled:          true,
						}

						chainConfigV2Params := func(feeConfig tp.TokenTransferFeeConfig, allowedFinality tp.FinalityConfig, ccvConfig tp.CcvConfig) tp.SetChainConfigV2Params {
							return tp.SetChainConfigV2Params{
								Mint:                   mint,
								RemoteChainSelector:    config.EvmChainSelector,
								AllowedFinalityConfig:  allowedFinality,
								TokenTransferFeeConfig: feeConfig,
								CcvConfig:              ccvConfig,
							}
						}

						initChainConfigV2 := func(t *testing.T, authority solana.PublicKey, feeConfig tp.TokenTransferFeeConfig, allowedFinality tp.FinalityConfig, ccvConfig tp.CcvConfig) solana.Instruction {
							t.Helper()
							ixRaw, err := tp.NewInitChainConfigV2Instruction(
								chainConfigV2Params(feeConfig, allowedFinality, ccvConfig),
								poolConfig,
								chainConfigV2PDA,
								authority,
								solana.SystemProgramID,
								mint,
								feeVault,
								feeVaultATA,
								v.tokenProgram,
								solana.SPLAssociatedTokenAccountProgramID,
								eventAuthority,
								pi.poolProgram,
							)
							require.NoError(t, err)
							return wrapPool(ixRaw)
						}

						editChainConfigV2 := func(t *testing.T, authority solana.PublicKey, feeConfig tp.TokenTransferFeeConfig, allowedFinality tp.FinalityConfig, ccvConfig tp.CcvConfig) solana.Instruction {
							t.Helper()
							ixRaw, err := tp.NewEditChainConfigV2Instruction(
								chainConfigV2Params(feeConfig, allowedFinality, ccvConfig),
								poolConfig,
								chainConfigV2PDA,
								authority,
								solana.SystemProgramID,
								eventAuthority,
								pi.poolProgram,
							)
							require.NoError(t, err)
							return wrapPool(ixRaw)
						}

						t.Run("set_unauthorized", func(t *testing.T) {
							ix := initChainConfigV2(t, user.PublicKey(), initialCfg, tp.FinalityConfig{}, tp.CcvConfig{})
							testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ix}, user, config.DefaultCommitment, []string{"ConstraintAddress"})
						})

						t.Run("set_invalid_config", func(t *testing.T) {
							invalidBpsCfg := initialCfg
							invalidBpsCfg.FinalityBpsFee = 10_001
							ix := initChainConfigV2(t, admin.PublicKey(), invalidBpsCfg, tp.FinalityConfig{}, tp.CcvConfig{})
							testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment, []string{"InvalidTokenTransferFeeBps"})

							invalidOverheadCfg := initialCfg
							invalidOverheadCfg.DestBytesOverhead = 31 // below the 32-byte minimum
							ix = initChainConfigV2(t, admin.PublicKey(), invalidOverheadCfg, tp.FinalityConfig{}, tp.CcvConfig{})
							testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment, []string{"InvalidTokenTransferFeeDestBytesOverhead"})
						})

						t.Run("set_initial", func(t *testing.T) {
							ix := initChainConfigV2(t, admin.PublicKey(), initialCfg, tp.FinalityConfig{}, tp.CcvConfig{})
							res := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)
							require.NotNil(t, res)

							tx, err := res.Transaction.GetTransaction()
							require.NoError(t, err)
							event, err := common.ExtractCPIEvent(tx, res.Meta, pi.poolProgram, base_token_pool.ParseEvent_ConfigV2Updated)
							require.NoError(t, err)
							require.Equal(t, config.EvmChainSelector, event.RemoteChainSelector)
							require.Equal(t, mint, event.Mint)
							require.Equal(t, uint32(90000), event.TokenTransferFee.DestGasOverhead.V0)
							require.True(t, event.TokenTransferFee.IsEnabled)

							var chainConfigV2 tp.ChainConfigV2
							err = common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, chainConfigV2PDA, config.DefaultCommitment, &chainConfigV2)
							require.NoError(t, err)
							require.Equal(t, uint8(1), chainConfigV2.Version)
							require.Equal(t, config.EvmChainSelector, chainConfigV2.RemoteChainSelector)
							require.Equal(t, mint, chainConfigV2.Mint)
							require.Equal(t, uint32(90000), chainConfigV2.TokenTransferFeeConfig.DestGasOverhead.V0)
							require.Equal(t, uint32(32), chainConfigV2.TokenTransferFeeConfig.DestBytesOverhead)
							require.Equal(t, uint32(50), chainConfigV2.TokenTransferFeeConfig.FinalityFee.V0)
							require.Equal(t, uint32(75), chainConfigV2.TokenTransferFeeConfig.FastFinalityFee.V0)
							require.Equal(t, uint16(10), chainConfigV2.TokenTransferFeeConfig.FinalityBpsFee)
							require.Equal(t, uint16(20), chainConfigV2.TokenTransferFeeConfig.FastFinalityBpsFee)
							require.True(t, chainConfigV2.TokenTransferFeeConfig.IsEnabled)
						})

						t.Run("edit_resizes_ccv_lists", func(t *testing.T) {
							// The account is sized from the CCV lists in the params, so editing must grow
							// and shrink it: otherwise a lane's verifier set would be frozen at whatever
							// it was created with.
							a := solana.NewWallet().PublicKey()
							b := solana.NewWallet().PublicKey()

							outboundCcvs := func(t *testing.T) []solana.PublicKey {
								t.Helper()
								var chainConfigV2 tp.ChainConfigV2
								err := common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, chainConfigV2PDA, config.DefaultCommitment, &chainConfigV2)
								require.NoError(t, err)
								return chainConfigV2.CcvConfig.OutboundCcvs
							}

							grow := editChainConfigV2(t, admin.PublicKey(), initialCfg, tp.FinalityConfig{}, tp.CcvConfig{OutboundCcvs: []solana.PublicKey{a, b}})
							testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{grow}, admin, config.DefaultCommitment)
							require.Equal(t, []solana.PublicKey{a, b}, outboundCcvs(t))

							shrink := editChainConfigV2(t, admin.PublicKey(), initialCfg, tp.FinalityConfig{}, tp.CcvConfig{OutboundCcvs: []solana.PublicKey{a}})
							testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{shrink}, admin, config.DefaultCommitment)
							require.Equal(t, []solana.PublicKey{a}, outboundCcvs(t))

							// Restore the empty CCV lists the subtests below expect.
							restore := editChainConfigV2(t, admin.PublicKey(), initialCfg, tp.FinalityConfig{}, tp.CcvConfig{})
							testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{restore}, admin, config.DefaultCommitment)
							require.Empty(t, outboundCcvs(t))
						})

						t.Run("edit", func(t *testing.T) {
							updatedCfg := initialCfg
							updatedCfg.FinalityFee = tp.UsdCents{V0: 999}
							updatedCfg.IsEnabled = false

							ix := editChainConfigV2(t, admin.PublicKey(), updatedCfg, tp.FinalityConfig{}, tp.CcvConfig{})
							res := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)
							require.NotNil(t, res)

							tx, err := res.Transaction.GetTransaction()
							require.NoError(t, err)
							event, err := common.ExtractCPIEvent(tx, res.Meta, pi.poolProgram, base_token_pool.ParseEvent_ConfigV2Updated)
							require.NoError(t, err)
							require.Equal(t, uint32(999), event.TokenTransferFee.FinalityFee.V0)
							require.False(t, event.TokenTransferFee.IsEnabled)

							var chainConfigV2 tp.ChainConfigV2
							err = common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, chainConfigV2PDA, config.DefaultCommitment, &chainConfigV2)
							require.NoError(t, err)
							require.Equal(t, uint32(999), chainConfigV2.TokenTransferFeeConfig.FinalityFee.V0)
							require.False(t, chainConfigV2.TokenTransferFeeConfig.IsEnabled)
						})

						t.Run("edit_unauthorized", func(t *testing.T) {
							ix := editChainConfigV2(t, user.PublicKey(), initialCfg, tp.FinalityConfig{}, tp.CcvConfig{})
							testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ix}, user, config.DefaultCommitment, []string{"ConstraintAddress"})
						})

						t.Run("edit_invalid_config", func(t *testing.T) {
							invalidCfg := initialCfg
							invalidCfg.FastFinalityBpsFee = 10_001
							ix := editChainConfigV2(t, admin.PublicKey(), invalidCfg, tp.FinalityConfig{}, tp.CcvConfig{})
							testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment, []string{"InvalidTokenTransferFeeBps"})
						})

						t.Run("get_fee", func(t *testing.T) {
							chainConfigPDA := p.Chain[config.EvmChainSelector]

							getFee := func(t *testing.T, finality tp.FinalityConfig) solana.Instruction {
								ixRaw, err := tp.NewGetPoolFeeInstruction(
									tp.GetFeeParams{
										RemoteChainSelector:     config.EvmChainSelector,
										Amount:                  1000,
										LocalToken:              mint,
										FeeToken:                solana.PublicKey{},
										RequestedFinalityConfig: finality,
										TokenArgs:               []byte{},
									},
									poolConfig,
									chainConfigPDA,
									chainConfigV2PDA,
								)
								require.NoError(t, err)
								return wrapPool(ixRaw)
							}

							setAllowedFinality := func(t *testing.T, finality tp.FinalityConfig) {
								ix := editChainConfigV2(t, admin.PublicKey(), initialCfg, finality, tp.CcvConfig{})
								testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)
							}

							t.Run("disabled config surfaces is_enabled false", func(t *testing.T) {
								// The preceding edit left the config disabled (with FinalityFee = 999). The pool
								// still returns the stored values; the router uses is_enabled to pick the fallback.
								res := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{getFee(t, tp.FinalityConfig{})}, user, config.DefaultCommitment)
								require.NotNil(t, res)
								result, err := common.ExtractAnchorTypedReturnValue[tp.GetFeeResult](ctx, res.Meta.LogMessages, pi.poolProgram.String())
								require.NoError(t, err)
								require.False(t, result.IsEnabled)
								require.Equal(t, uint32(999), result.Fee.V0)
							})

							// Re-enable the fee config for the quoting tests below.
							ixEnable := editChainConfigV2(t, admin.PublicKey(), initialCfg, tp.FinalityConfig{}, tp.CcvConfig{})
							testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ixEnable}, admin, config.DefaultCommitment)

							t.Run("default finality quotes the finality fee", func(t *testing.T) {
								res := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{getFee(t, tp.FinalityConfig{})}, user, config.DefaultCommitment)
								require.NotNil(t, res)
								result, err := common.ExtractAnchorTypedReturnValue[tp.GetFeeResult](ctx, res.Meta.LogMessages, pi.poolProgram.String())
								require.NoError(t, err)
								require.True(t, result.IsEnabled)
								require.Equal(t, uint32(90000), result.DestGasLimit.V0)
								require.Equal(t, uint32(32), result.DestBytesOverhead)
								require.Equal(t, uint32(50), result.Fee.V0)
								require.Equal(t, uint16(10), result.TokenFeeBps)
							})

							t.Run("fast finality rejected when not allowed", func(t *testing.T) {
								// chain_config_v2 still holds the default WAIT_FOR_FINALITY policy, so FTF quoting is rejected.
								testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{getFee(t, tp.FinalityConfig{Flags: 1})}, user, config.DefaultCommitment, []string{"Error Code: " + "InvalidRequestedFinality"})
							})

							t.Run("fast finality quotes the fast finality fee", func(t *testing.T) {
								setAllowedFinality(t, tp.FinalityConfig{Flags: 1})

								res := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{getFee(t, tp.FinalityConfig{Flags: 1})}, user, config.DefaultCommitment)
								require.NotNil(t, res)
								result, err := common.ExtractAnchorTypedReturnValue[tp.GetFeeResult](ctx, res.Meta.LogMessages, pi.poolProgram.String())
								require.NoError(t, err)
								require.True(t, result.IsEnabled)
								require.Equal(t, uint32(75), result.Fee.V0)
								require.Equal(t, uint16(20), result.TokenFeeBps)

								// Restore the default finality policy so later subtests see the original lane state.
								setAllowedFinality(t, tp.FinalityConfig{})
							})
						})

						t.Run("query resolvers return the full account list", func(t *testing.T) {
							resolve := func(inputs []byte) (solana.Instruction, error) {
								ix, err := tp.NewNestedResolveAccountsStartInstruction(
									tp.NestedResolveAccountsParams{
										Caller:              user.PublicKey(),
										CallerProgram:       config.CcipRouterProgram,
										CpiResolutionInputs: inputs,
									},
									poolConfig,
								)
								return wrapPool(ix), err
							}
							ccvsParams := tp.GetRequiredCcVsIn{
								RemoteChainSelector: config.EvmChainSelector,
								LocalToken:          mint,
								Direction:           tp.MessageDirection_Outbound,
							}
							requireQueriesResolveFullAccountList(ctx, t, solanaGoClient, user, pi.poolProgram, resolve,
								[]solana.PublicKey{poolConfig, p.Chain[config.EvmChainSelector], chainConfigV2PDA},
								[]poolQuery{
									{"get_required_ccvs", resolutionInputsGetRequiredCcv, &ccvsParams, func(a []solana.PublicKey) (solana.Instruction, error) {
										ix, err := tp.NewGetRequiredCcvsInstruction(ccvsParams, a[0], a[1], a[2])
										return wrapPool(ix), err
									}},
								},
							)
						})

						t.Run("get_required_ccvs", func(t *testing.T) {
							// Threshold CCVs apply to the amount actually sent cross-chain: outbound, the
							// router queries with the gross amount, so the pool must deduct its BPS fee
							// (10 bps at finality, 20 bps fast, from initialCfg) before comparing.
							const threshold = uint64(1_000_000)
							outbound := solana.NewWallet().PublicKey()
							thresholdOutbound := solana.NewWallet().PublicKey()
							inbound := solana.NewWallet().PublicKey()
							thresholdInbound := solana.NewWallet().PublicKey()
							ccvConfig := tp.CcvConfig{
								ThresholdAmount:       threshold,
								OutboundCcvs:          []solana.PublicKey{outbound},
								ThresholdOutboundCcvs: []solana.PublicKey{thresholdOutbound},
								InboundCcvs:           []solana.PublicKey{inbound},
								ThresholdInboundCcvs:  []solana.PublicKey{thresholdInbound},
							}

							setLane := func(t *testing.T, feeConfig tp.TokenTransferFeeConfig) {
								ix := editChainConfigV2(t, admin.PublicKey(), feeConfig, tp.FinalityConfig{}, ccvConfig)
								testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)
							}

							getRequiredCcvs := func(t *testing.T, direction tp.MessageDirection, amount uint64, finality tp.FinalityConfig) []solana.PublicKey {
								t.Helper()
								ixRaw, err := tp.NewGetRequiredCcvsInstruction(
									tp.GetRequiredCcVsIn{
										RemoteChainSelector:     config.EvmChainSelector,
										Amount:                  tp.ProtocolAmount{BeBytes: tokens.ToBigEndianU256(amount)},
										LocalToken:              mint,
										Direction:               direction,
										RequestedFinalityConfig: finality,
									},
									poolConfig,
									p.Chain[config.EvmChainSelector],
									chainConfigV2PDA,
								)
								require.NoError(t, err)
								res := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{wrapPool(ixRaw)}, user, config.DefaultCommitment)
								require.NotNil(t, res)
								result, err := common.ExtractAnchorTypedReturnValue[tp.GetRequiredCcVsOut](ctx, res.Meta.LogMessages, pi.poolProgram.String())
								require.NoError(t, err)
								return result.RequiredCcvs
							}

							setLane(t, initialCfg)

							t.Run("outbound gross amount at threshold is below it after the fee", func(t *testing.T) {
								// 1,000,000 - 1,000 (10 bps) = 999,000 sent.
								require.Equal(t, []solana.PublicKey{outbound},
									getRequiredCcvs(t, tp.MessageDirection_Outbound, threshold, tp.FinalityConfig{}))
							})

							t.Run("outbound post-fee amount reaching threshold adds threshold CCVs", func(t *testing.T) {
								// 1,001,001 - 1,001 (10 bps, floored) = 1,000,000 sent.
								require.Equal(t, []solana.PublicKey{outbound, thresholdOutbound},
									getRequiredCcvs(t, tp.MessageDirection_Outbound, 1_001_001, tp.FinalityConfig{}))
							})

							t.Run("outbound fee follows the requested finality", func(t *testing.T) {
								// 1,001,001 - 2,002 (20 bps, floored) = 998,999 sent.
								require.Equal(t, []solana.PublicKey{outbound},
									getRequiredCcvs(t, tp.MessageDirection_Outbound, 1_001_001, tp.FinalityConfig{Flags: 1}))
							})

							t.Run("inbound amount is not fee-adjusted", func(t *testing.T) {
								// Already post-fee; only normalized from the remote token's 9 decimals to the
								// local token's 0.
								require.Equal(t, uint8(0), decimals)
								remoteAmount := threshold * 1_000_000_000
								require.Equal(t, []solana.PublicKey{inbound, thresholdInbound},
									getRequiredCcvs(t, tp.MessageDirection_Inbound, remoteAmount, tp.FinalityConfig{}))
							})

							t.Run("outbound without an enabled fee uses the gross amount", func(t *testing.T) {
								disabled := initialCfg
								disabled.IsEnabled = false
								setLane(t, disabled)

								require.Equal(t, []solana.PublicKey{outbound, thresholdOutbound},
									getRequiredCcvs(t, tp.MessageDirection_Outbound, threshold, tp.FinalityConfig{}))
							})

							// Restore the enabled fee config with empty CCV lists for later subtests.
							ix := editChainConfigV2(t, admin.PublicKey(), initialCfg, tp.FinalityConfig{}, tp.CcvConfig{})
							testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)
						})

						t.Run("get_fee_default_disabled_config", func(t *testing.T) {
							// A default disabled fee config signals the OnRamp to use the FeeQuoter fallback.
							ixDisable := editChainConfigV2(t, admin.PublicKey(), tp.TokenTransferFeeConfig{}, tp.FinalityConfig{}, tp.CcvConfig{})
							testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ixDisable}, admin, config.DefaultCommitment)

							ixRaw, err := tp.NewGetPoolFeeInstruction(
								tp.GetFeeParams{
									RemoteChainSelector:     config.EvmChainSelector,
									Amount:                  1000,
									LocalToken:              mint,
									FeeToken:                solana.PublicKey{},
									RequestedFinalityConfig: tp.FinalityConfig{},
									TokenArgs:               []byte{},
								},
								poolConfig,
								p.Chain[config.EvmChainSelector],
								p.ChainConfigV2[config.EvmChainSelector],
							)
							require.NoError(t, err)

							res := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{wrapPool(ixRaw)}, user, config.DefaultCommitment)
							require.NotNil(t, res)
							result, err := common.ExtractAnchorTypedReturnValue[tp.GetFeeResult](ctx, res.Meta.LogMessages, pi.poolProgram.String())
							require.NoError(t, err)
							require.False(t, result.IsEnabled)
							require.Equal(t, uint32(0), result.Fee.V0)
							require.Equal(t, uint16(0), result.TokenFeeBps)
							require.Equal(t, uint32(0), result.DestGasLimit.V0)
							require.Equal(t, uint32(0), result.DestBytesOverhead)
						})
					})

					t.Run("chain_config_override", func(t *testing.T) {
						overrideType := tp.PoolChainConfigOverrideType_Ftf
						overridePDA := p.ChainOverride[config.EvmChainSelector]
						eventAuthority, _, _ := state.FindEventAuthorityPDA(pi.poolProgram)

						t.Run("init_override_unauthorized", func(t *testing.T) {
							ixRaw, err := tp.NewInitChainConfigOverrideInstruction(
								tp.InitChainConfigOverrideParams{
									RemoteChainSelector: config.EvmChainSelector,
									Mint:                mint,
									OverrideType:        overrideType,
									Inbound:             tp.RateLimitConfig{Enabled: true, Capacity: amount * 10, Rate: 1},
									Outbound:            tp.RateLimitConfig{Enabled: true, Capacity: amount * 10, Rate: 1},
								},
								poolConfig,
								overridePDA,
								user.PublicKey(), // unauthorized user
								solana.SystemProgramID,
								eventAuthority,
								pi.poolProgram,
							)
							require.NoError(t, err)
							ix := wrapPool(ixRaw)
							testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ix}, user, config.DefaultCommitment, []string{"ConstraintAddress"})
						})

						t.Run("init_override", func(t *testing.T) {
							ixRaw, err := tp.NewInitChainConfigOverrideInstruction(
								tp.InitChainConfigOverrideParams{
									RemoteChainSelector: config.EvmChainSelector,
									Mint:                mint,
									OverrideType:        overrideType,
									Inbound: tp.RateLimitConfig{
										Enabled:  true,
										Capacity: amount * 10,
										Rate:     1,
									},
									Outbound: tp.RateLimitConfig{
										Enabled:  true,
										Capacity: amount * 10,
										Rate:     1,
									},
								},
								poolConfig,
								overridePDA,
								admin.PublicKey(),
								solana.SystemProgramID,
								eventAuthority,
								pi.poolProgram,
							)
							require.NoError(t, err)
							ix := wrapPool(ixRaw)

							res := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)
							require.NotNil(t, res)

							tx, err := res.Transaction.GetTransaction()
							require.NoError(t, err)
							event, err := common.ExtractCPIEvent(tx, res.Meta, pi.poolProgram, base_token_pool.ParseEvent_OverrideChainConfigConfigured)
							require.NoError(t, err)
							require.Equal(t, config.EvmChainSelector, event.ChainSelector)
							require.Equal(t, base_token_pool.PoolChainConfigOverrideType(overrideType), event.OverrideType)
							require.True(t, event.InboundRateLimit.Enabled)
							require.Equal(t, amount*10, event.InboundRateLimit.Capacity)
							require.True(t, event.OutboundRateLimit.Enabled)
							require.Equal(t, amount*10, event.OutboundRateLimit.Capacity)
							require.Equal(t, mint, event.Mint)

							// Verify PDA was created with correct content
							var override tp.ChainConfigOverride
							err = common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, overridePDA, config.DefaultCommitment, &override)
							require.NoError(t, err)
							require.Equal(t, uint8(1), override.Version)
							require.Equal(t, config.EvmChainSelector, override.Base.RemoteChainSelector)
							require.Equal(t, mint, override.Base.Mint)
							require.Equal(t, overrideType, override.Base.OverrideType)
							require.True(t, override.Base.InboundRateLimit.Cfg.Enabled)
							require.Equal(t, amount*10, override.Base.InboundRateLimit.Cfg.Capacity)
							require.True(t, override.Base.OutboundRateLimit.Cfg.Enabled)
							require.Equal(t, amount*10, override.Base.OutboundRateLimit.Cfg.Capacity)
						})

						t.Run("set_override_rate_limit", func(t *testing.T) {
							ixRaw, err := tp.NewSetChainConfigOverrideRateLimitInstruction(
								tp.SetChainConfigOverrideRateLimitParams{
									RemoteChainSelector: config.EvmChainSelector,
									Mint:                mint,
									OverrideType:        overrideType,
									Inbound: tp.RateLimitConfig{
										Enabled:  true,
										Capacity: amount * 5,
										Rate:     1,
									},
									Outbound: tp.RateLimitConfig{
										Enabled:  true,
										Capacity: amount * 5,
										Rate:     1,
									},
								},
								poolConfig,
								overridePDA,
								admin.PublicKey(),
								eventAuthority,
								pi.poolProgram,
							)
							require.NoError(t, err)
							ix := wrapPool(ixRaw)

							res := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)
							require.NotNil(t, res)

							tx, err := res.Transaction.GetTransaction()
							require.NoError(t, err)
							event, err := common.ExtractCPIEvent(tx, res.Meta, pi.poolProgram, base_token_pool.ParseEvent_OverrideChainConfigConfigured)
							require.NoError(t, err)
							require.Equal(t, config.EvmChainSelector, event.ChainSelector)
							require.True(t, event.InboundRateLimit.Enabled)
							require.Equal(t, amount*5, event.InboundRateLimit.Capacity)
							require.True(t, event.OutboundRateLimit.Enabled)
							require.Equal(t, amount*5, event.OutboundRateLimit.Capacity)

							// Verify PDA reflects the update
							var override tp.ChainConfigOverride
							err = common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, overridePDA, config.DefaultCommitment, &override)
							require.NoError(t, err)
							require.Equal(t, amount*5, override.Base.InboundRateLimit.Cfg.Capacity)
							require.Equal(t, amount*5, override.Base.OutboundRateLimit.Cfg.Capacity)
						})

						t.Run("set_override_rate_limit_unauthorized", func(t *testing.T) {
							ixRaw, err := tp.NewSetChainConfigOverrideRateLimitInstruction(
								tp.SetChainConfigOverrideRateLimitParams{
									RemoteChainSelector: config.EvmChainSelector,
									Mint:                mint,
									OverrideType:        overrideType,
									Inbound:             tp.RateLimitConfig{Enabled: true, Capacity: amount * 3, Rate: 1},
									Outbound:            tp.RateLimitConfig{Enabled: true, Capacity: amount * 3, Rate: 1},
								},
								poolConfig,
								overridePDA,
								user.PublicKey(), // unauthorized user (not owner, not rate_limit_admin)
								eventAuthority,
								pi.poolProgram,
							)
							require.NoError(t, err)
							ix := wrapPool(ixRaw)
							testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ix}, user, config.DefaultCommitment, []string{"Error Code: Unauthorized"})
						})

						t.Run("init_override_already_exists", func(t *testing.T) {
							ixRaw, err := tp.NewInitChainConfigOverrideInstruction(
								tp.InitChainConfigOverrideParams{
									RemoteChainSelector: config.EvmChainSelector,
									Mint:                mint,
									OverrideType:        overrideType,
									Inbound:             tp.RateLimitConfig{Enabled: true, Capacity: amount * 10, Rate: 1},
									Outbound:            tp.RateLimitConfig{Enabled: true, Capacity: amount * 10, Rate: 1},
								},
								poolConfig,
								overridePDA,
								admin.PublicKey(),
								solana.SystemProgramID,
								eventAuthority,
								pi.poolProgram,
							)
							require.NoError(t, err)
							ix := wrapPool(ixRaw)
							// PDA already initialized, Anchor returns error
							testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment, []string{"already in use"})
						})
						// set_ccv_config rejects malformed CCV lists (mirrors the router's dest-config checks):
						// no zero-address CCVs, and threshold CCVs require a non-zero threshold amount.
						t.Run("set_ccv_config rejects invalid CCV config", func(t *testing.T) {
							// Use a fresh remote selector so each case init's chain_config_v2 from scratch.
							// Each case reverts, so the account is never persisted and the next case init's
							// cleanly.
							const invalidCfgSelector = uint64(987654321)
							chainConfigV2PDA, _, err := tokens.TokenPoolChainConfigV2PDA(invalidCfgSelector, mint, pi.poolProgram)
							require.NoError(t, err)
							feeVault, _, err := state.FindFeeVaultPDA(pi.poolProgram, mint.Bytes())
							require.NoError(t, err)
							feeVaultATA, err := tokens.FindFeeVaultATA(v.tokenProgram, mint, pi.poolProgram, mint.Bytes())
							require.NoError(t, err)

							setCcv := func(params tp.SetChainConfigV2Params, wantErr string) {
								ixRaw, err := tp.NewInitChainConfigV2Instruction(
									params,
									poolConfig,
									chainConfigV2PDA,
									admin.PublicKey(),
									solana.SystemProgramID,
									mint,
									feeVault,
									feeVaultATA,
									v.tokenProgram,
									solana.SPLAssociatedTokenAccountProgramID,
									eventAuthority,
									pi.poolProgram,
								)
								require.NoError(t, err)
								testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{wrapPool(ixRaw)}, admin, config.DefaultCommitment, []string{wantErr})
							}

							a := solana.NewWallet().PublicKey()
							b := solana.NewWallet().PublicKey()

							// The zero/default pubkey is not a valid CCV.
							setCcv(tp.SetChainConfigV2Params{
								Mint:                   mint,
								RemoteChainSelector:    invalidCfgSelector,
								TokenTransferFeeConfig: tp.TokenTransferFeeConfig{},
								CcvConfig: tp.CcvConfig{
									OutboundCcvs: []solana.PublicKey{{}},
								},
							}, "CcvZeroAddressNotAllowed")

							// Threshold CCVs configured but threshold amount is zero => they would never apply.
							setCcv(tp.SetChainConfigV2Params{
								Mint:                   mint,
								RemoteChainSelector:    invalidCfgSelector,
								TokenTransferFeeConfig: tp.TokenTransferFeeConfig{},
								CcvConfig: tp.CcvConfig{
									ThresholdAmount:       0,
									OutboundCcvs:          []solana.PublicKey{a},
									ThresholdOutboundCcvs: []solana.PublicKey{b},
								},
							}, "ThresholdAmountRequiredForThresholdCcvs")
						})
						t.Run("delete_override", func(t *testing.T) {
							ixRaw, err := tp.NewDeleteChainConfigOverrideInstruction(
								tp.DeleteChainConfigOverrideParams{
									RemoteChainSelector: config.EvmChainSelector,
									Mint:                mint,
									OverrideType:        overrideType,
								},
								poolConfig,
								overridePDA,
								admin.PublicKey(),
								eventAuthority,
								pi.poolProgram,
							)
							require.NoError(t, err)
							ix := wrapPool(ixRaw)

							res := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)
							require.NotNil(t, res)

							tx, err := res.Transaction.GetTransaction()
							require.NoError(t, err)
							event, err := common.ExtractCPIEvent(tx, res.Meta, pi.poolProgram, base_token_pool.ParseEvent_OverrideChainConfigRemoved)
							require.NoError(t, err)
							require.Equal(t, config.EvmChainSelector, event.ChainSelector)
							require.Equal(t, base_token_pool.PoolChainConfigOverrideType(overrideType), event.OverrideType)
							require.Equal(t, mint, event.Mint)

							// Verify PDA was actually closed
							testutils.AssertClosedAccount(ctx, t, solanaGoClient, overridePDA, config.DefaultCommitment)
						})

						t.Run("delete_override_unauthorized", func(t *testing.T) {
							// First reinitialize the override so we can test unauthorized delete
							initRaw, err := tp.NewInitChainConfigOverrideInstruction(
								tp.InitChainConfigOverrideParams{
									RemoteChainSelector: config.EvmChainSelector,
									Mint:                mint,
									OverrideType:        overrideType,
									Inbound:             tp.RateLimitConfig{Enabled: true, Capacity: amount * 10, Rate: 1},
									Outbound:            tp.RateLimitConfig{Enabled: true, Capacity: amount * 10, Rate: 1},
								},
								poolConfig,
								overridePDA,
								admin.PublicKey(),
								solana.SystemProgramID,
								eventAuthority,
								pi.poolProgram,
							)
							require.NoError(t, err)
							testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{wrapPool(initRaw)}, admin, config.DefaultCommitment)

							// Try to delete with unauthorized user
							ixRaw, err := tp.NewDeleteChainConfigOverrideInstruction(
								tp.DeleteChainConfigOverrideParams{
									RemoteChainSelector: config.EvmChainSelector,
									Mint:                mint,
									OverrideType:        overrideType,
								},
								poolConfig,
								overridePDA,
								user.PublicKey(),
								eventAuthority,
								pi.poolProgram,
							)
							require.NoError(t, err)
							ix := wrapPool(ixRaw)
							testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ix}, user, config.DefaultCommitment, []string{"ConstraintAddress"})

							// Clean up - delete with authorized user
							deleteRaw, err := tp.NewDeleteChainConfigOverrideInstruction(
								tp.DeleteChainConfigOverrideParams{
									RemoteChainSelector: config.EvmChainSelector,
									Mint:                mint,
									OverrideType:        overrideType,
								},
								poolConfig,
								overridePDA,
								admin.PublicKey(),
								eventAuthority,
								pi.poolProgram,
							)
							require.NoError(t, err)
							testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{wrapPool(deleteRaw)}, admin, config.DefaultCommitment)
						})
					})
				})
			}
		})
	}
}

// Borsh variant indices of the queries in base_token_pool::v2::TokenPoolCpiResolutionInputs.
const (
	resolutionInputsGetRequiredCcv byte = 2
)

// poolQuery is a pool query whose accounts a client resolves through the pool's own
// nested_resolve_accounts_start.
type poolQuery struct {
	name    string
	variant byte
	params  any
	call    func(accounts []solana.PublicKey) (solana.Instruction, error)
}

// requireQueriesResolveFullAccountList checks that the pool's resolver returns exactly `expected`,
// all read-only, for each query, and that each query succeeds when called with that list as is.
func requireQueriesResolveFullAccountList(
	ctx context.Context,
	t *testing.T,
	client *rpc.Client,
	payer solana.PrivateKey,
	poolProgram solana.PublicKey,
	resolve func(cpiResolutionInputs []byte) (solana.Instruction, error),
	expected []solana.PublicKey,
	queries []poolQuery,
) {
	for _, q := range queries {
		t.Run(q.name, func(t *testing.T) {
			encoded, err := bin.MarshalBorsh(q.params)
			require.NoError(t, err)
			ix, err := resolve(append([]byte{q.variant}, encoded...))
			require.NoError(t, err)
			sim := testutils.SimulateTransaction(ctx, t, client, []solana.Instruction{ix}, payer.PublicKey())
			require.Nil(t, sim.Value.Err)
			// Every pool returns base_token_pool's NestedResolveAccountsResponse, so any binding decodes it.
			response, err := common.ExtractAnchorTypedReturnValue[tp.NestedResolveAccountsResponse](ctx, sim.Value.Logs, poolProgram.String())
			require.NoError(t, err)
			require.Empty(t, response.AskAgainWith)

			accounts := make([]solana.PublicKey, 0, len(response.AccountsToSave))
			for _, meta := range response.AccountsToSave {
				require.False(t, meta.IsWritable)
				accounts = append(accounts, meta.Pubkey)
			}
			require.Equal(t, expected, accounts)

			queryIx, err := q.call(accounts)
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{queryIx}, payer, config.DefaultCommitment)
		})
	}
}
