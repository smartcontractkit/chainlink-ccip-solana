package examples

import (
	"math"
	"strconv"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/token"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/arbitrary_cpi_proxy"
	burnmint_tokenpool "github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/burnmint_token_pool"
	tokenpool "github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/lockrelease_token_pool"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/rmn_remote"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/config"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/testutils"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/tokens"
)

type PoolInfo struct {
	poolName    string
	poolProgram solana.PublicKey
}

var pools = []PoolInfo{
	{poolName: "lockrelease", poolProgram: config.CcipBasePoolLockRelease},
	{poolName: "burnmint", poolProgram: config.CcipBasePoolBurnMint},
}

type TokenInfo struct {
	tokenName    string
	tokenProgram solana.PublicKey
	multisig     bool
}

var tokenPrograms = []TokenInfo{
	{tokenName: "spl-token", tokenProgram: solana.TokenProgramID, multisig: false},
	{tokenName: "spl-token", tokenProgram: solana.TokenProgramID, multisig: true},
	{tokenName: "spl-token-2022", tokenProgram: config.Token2022Program, multisig: false},
	{tokenName: "spl-token-2022", tokenProgram: config.Token2022Program, multisig: true},
}

// TestBaseTokenPoolHappyPath does basic happy path checks on the lock/release and burn/mint example pools
// more detailed token pool tests are handled by the test-token-pool which is used in the tokenpool_test.go and ccip_router_test.go
func TestBaseTokenPoolHappyPath(t *testing.T) {
	// acting as "dumb" onramp & offramp, proxying calls to the pool that are signed by PDA
	dumbRamp := config.ArbitraryCpiProxyProgram
	allowedOfframpPDA, _ := state.FindAllowedOfframpPDA(config.EvmChainSelector, dumbRamp, dumbRamp)

	admin, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)
	ctx := t.Context()

	solanaGoClient := testutils.DeployAllPrograms(t, testutils.PathToAnchorConfig, admin)
	getBalance := func(account solana.PublicKey) string {
		return testutils.GetTokenBalance(ctx, t, solanaGoClient, account)
	}

	remotePool := tokenpool.RemoteAddress{Address: []byte{1, 2, 3}}
	remoteToken := tokenpool.RemoteAddress{Address: []byte{4, 5, 6}}

	t.Run("setup", func(t *testing.T) {
		t.Run("funding", func(t *testing.T) {
			require.NoError(t, testutils.FundAccounts(ctx, []solana.PrivateKey{admin}, solanaGoClient))
		})

		t.Run("register offramp", func(t *testing.T) {
			ix, ixErr := arbitrary_cpi_proxy.NewAddOfframpInstruction(
				config.EvmChainSelector,
				dumbRamp,
				allowedOfframpPDA,
				admin.PublicKey(),
				solana.SystemProgramID,
			)
			require.NoError(t, ixErr)
			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)
		})

		t.Run("RMN Remote", func(t *testing.T) {
			rmnProgramDataAddress := testutils.GetProgramDataAddress(ctx, t, solanaGoClient, config.RMNRemoteProgram)

			ix, ixErr := rmn_remote.NewInitializeInstruction(
				config.RMNRemoteConfigPDA,
				config.RMNRemoteCursesPDA,
				admin.PublicKey(),
				solana.SystemProgramID,
				config.RMNRemoteProgram,
				rmnProgramDataAddress,
			)

			require.NoError(t, ixErr)
			result := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)
			require.NotNil(t, result)
		})
	})

	// test functionality with each pool type (burnmint & lockrelease)
	for _, p := range pools {
		var poolProgramDataAddress solana.PublicKey
		var configPDA solana.PublicKey
		poolProgram := p.poolProgram

		t.Run("setup:tokenPool "+p.poolName, func(t *testing.T) {
			poolProgramDataAddress = testutils.GetProgramDataAddress(ctx, t, solanaGoClient, p.poolProgram)

			// Global Configuration
			configPDA, err = tokens.TokenPoolGlobalConfigPDA(p.poolProgram)
			require.NoError(t, err)

			ix, err := tokenpool.NewInitGlobalConfigInstruction(dumbRamp, config.RMNRemoteProgram, configPDA, admin.PublicKey(), solana.SystemProgramID, poolProgram, poolProgramDataAddress)
			require.NoError(t, err)

			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{
				&tokens.TokenInstruction{Instruction: ix, Program: p.poolProgram},
			}, admin, config.DefaultCommitment)
		})

		// test functionality with token & token-2022 standards
		for _, v := range tokenPrograms {
			t.Run(p.poolName+" "+v.tokenName, func(t *testing.T) {
				t.Parallel()

				decimals := uint8(0)
				amount := uint64(1000)

				// for _, poolProgram := range []solana.PublicKey{config.CcipBasePoolBurnMint} {
				rampPoolSignerPDA, _, _ := state.FindExternalTokenPoolsSignerPDA(poolProgram, dumbRamp)

				mintPriv, err := solana.NewRandomPrivateKey()
				require.NoError(t, err)
				tokenPool, err := tokens.NewTokenPool(v.tokenProgram, poolProgram, mintPriv.PublicKey())
				require.NoError(t, err)
				mint := tokenPool.Mint

				t.Run("setup:token", func(t *testing.T) {
					// create token
					instructions, err := tokens.CreateToken(ctx, v.tokenProgram, mint, admin.PublicKey(), decimals, solanaGoClient, config.DefaultCommitment)
					require.NoError(t, err)

					// create admin associated token account
					createI, tokenAccount, err := tokens.CreateAssociatedTokenAccount(v.tokenProgram, mint, admin.PublicKey(), admin.PublicKey())
					require.NoError(t, err)
					tokenPool.User[admin.PublicKey()] = tokenAccount // set admin token account

					// mint tokens to admin
					mintToI, err := tokens.MintTo(amount, v.tokenProgram, mint, tokenAccount, admin.PublicKey())
					require.NoError(t, err)

					instructions = append(instructions, createI, mintToI)
					testutils.SendAndConfirm(ctx, t, solanaGoClient, instructions, admin, config.DefaultCommitment, common.AddSigners(mintPriv))

					// validate
					outDec, outVal, err := tokens.TokenBalance(ctx, solanaGoClient, tokenPool.User[admin.PublicKey()], config.DefaultCommitment)
					require.NoError(t, err)
					require.Equal(t, int(amount), outVal) //nolint:gosec // G115 test amount fits in int
					require.Equal(t, decimals, outDec)
				})

				t.Run("pool:"+poolProgram.String(), func(t *testing.T) {
					poolConfig, err := tokens.TokenPoolConfigAddress(mint, poolProgram)
					require.NoError(t, err)
					poolSigner, err := tokens.TokenPoolSignerAddress(mint, poolProgram)
					require.NoError(t, err)
					createI, poolTokenAccount, err := tokens.CreateAssociatedTokenAccount(v.tokenProgram, mint, poolSigner, admin.PublicKey())
					require.NoError(t, err)

					t.Run("setup", func(t *testing.T) {
						poolInitI, initErr := tokenpool.NewInitializeInstruction(poolConfig, mint, admin.PublicKey(), solana.SystemProgramID, poolProgram, poolProgramDataAddress, configPDA)
						require.NoError(t, initErr)

						newMintAuthority := poolSigner

						if v.multisig {
							// create multisig
							multisig, newKeyErr := solana.NewRandomPrivateKey()
							require.NoError(t, newKeyErr)
							ixMsig, ixErrMsig := tokens.CreateMultisig(ctx, admin.PublicKey(), v.tokenProgram, multisig.PublicKey(), 1, []solana.PublicKey{admin.PublicKey(), poolSigner}, solanaGoClient, config.DefaultCommitment)
							require.NoError(t, ixErrMsig)
							testutils.SendAndConfirm(ctx, t, solanaGoClient, ixMsig, admin, config.DefaultCommitment, common.AddSigners(multisig, admin))

							newMintAuthority = multisig.PublicKey()
							tokenPool.MintAuthorityMultisig = multisig.PublicKey() // set multisig as mint authority
						}

						// make pool mint_authority for token (required for burn/mint) poolsigner or multisig
						authI, setAuthorityErr := tokens.SetTokenMintAuthority(v.tokenProgram, newMintAuthority, mint, admin.PublicKey())
						require.NoError(t, setAuthorityErr)

						// set pool config
						ixConfigure, initChainRemoteErr := tokenpool.NewInitChainRemoteConfigInstruction(
							config.EvmChainSelector,
							tokenPool.Mint,
							tokenpool.RemoteConfig{
								TokenAddress: remoteToken,
								Decimals:     9,
							},
							poolConfig,
							tokenPool.Chain[config.EvmChainSelector],
							admin.PublicKey(),
							solana.SystemProgramID,
						)
						require.NoError(t, initChainRemoteErr)

						ixAppend, appendErr := tokenpool.NewAppendRemotePoolAddressesInstruction(
							config.EvmChainSelector, tokenPool.Mint, []tokenpool.RemoteAddress{remotePool}, poolConfig, tokenPool.Chain[config.EvmChainSelector], admin.PublicKey(), solana.SystemProgramID,
						)
						require.NoError(t, appendErr)

						testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{
							&tokens.TokenInstruction{Instruction: poolInitI, Program: poolProgram},
							createI,
							authI,
							&tokens.TokenInstruction{Instruction: ixConfigure, Program: poolProgram},
							&tokens.TokenInstruction{Instruction: ixAppend, Program: poolProgram},
						}, admin, config.DefaultCommitment)

						// Test growing and shrinking allowlist (realloc test)
						a, _ := solana.NewRandomPrivateKey()
						b, _ := solana.NewRandomPrivateKey()

						ixGrowWithDuplicates, allowlistConfigDupErr := tokenpool.NewConfigureAllowListInstruction([]solana.PublicKey{a.PublicKey(), b.PublicKey(), b.PublicKey()}, false, poolConfig, mint, admin.PublicKey(), solana.SystemProgramID)
						require.NoError(t, allowlistConfigDupErr)
						testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{&tokens.TokenInstruction{Instruction: ixGrowWithDuplicates, Program: poolProgram}}, admin, rpc.CommitmentConfirmed, []string{"Key already existed in the allowlist"})

						ixGrow, allowlistConfigErr := tokenpool.NewConfigureAllowListInstruction([]solana.PublicKey{a.PublicKey(), b.PublicKey()}, false, poolConfig, mint, admin.PublicKey(), solana.SystemProgramID)
						require.NoError(t, allowlistConfigErr)
						testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{&tokens.TokenInstruction{Instruction: ixGrow, Program: poolProgram}}, admin, rpc.CommitmentConfirmed)

						ixDuplicatedRemove, duplicatedRemoveErr := tokenpool.NewRemoveFromAllowListInstruction([]solana.PublicKey{b.PublicKey(), b.PublicKey()}, poolConfig, mint, admin.PublicKey(), solana.SystemProgramID)
						require.NoError(t, duplicatedRemoveErr)
						testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{&tokens.TokenInstruction{Instruction: ixDuplicatedRemove, Program: poolProgram}}, admin, rpc.CommitmentConfirmed, []string{"DuplicatedKeys"})

						ixShrink, removeAllowlistErr := tokenpool.NewRemoveFromAllowListInstruction([]solana.PublicKey{a.PublicKey(), b.PublicKey()}, poolConfig, mint, admin.PublicKey(), solana.SystemProgramID)
						require.NoError(t, removeAllowlistErr)
						testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{&tokens.TokenInstruction{Instruction: ixShrink, Program: poolProgram}}, admin, rpc.CommitmentConfirmed)

						// Shrinking fails now as the entries do not exist anymore
						testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{&tokens.TokenInstruction{Instruction: ixShrink, Program: poolProgram}}, admin, rpc.CommitmentConfirmed, []string{"Key did not exist in the allowlist"})
					})

					t.Run("lockOrBurn", func(t *testing.T) {
						require.Equal(t, strconv.FormatUint(amount, 10), getBalance(tokenPool.User[admin.PublicKey()]))

						transferI, err := tokens.TokenTransferChecked(amount, decimals, v.tokenProgram, tokenPool.User[admin.PublicKey()], mint, poolTokenAccount, admin.PublicKey(), solana.PublicKeySlice{})
						require.NoError(t, err)

						lbI, err := testutils.NewPoolProxyLockOrBurnInstruction(
							burnmint_tokenpool.LockOrBurnInV1{
								LocalToken:          mint,
								Amount:              amount,
								RemoteChainSelector: config.EvmChainSelector,
							},
							poolProgram,
							rampPoolSignerPDA,

							poolConfig,
							v.tokenProgram,
							mint,
							poolSigner,
							poolTokenAccount,
							config.RMNRemoteProgram,
							config.RMNRemoteCursesPDA,
							config.RMNRemoteConfigPDA,
							tokenPool.Chain[config.EvmChainSelector],
						)
						require.NoError(t, err)

						res := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{transferI, lbI}, admin, config.DefaultCommitment)
						require.NotNil(t, res)

						// validate balances
						require.Equal(t, "0", getBalance(tokenPool.User[admin.PublicKey()]))
						expectedPoolBal := uint64(0)
						if poolProgram == config.CcipBasePoolLockRelease {
							expectedPoolBal = amount
						}
						require.Equal(t, strconv.FormatUint(expectedPoolBal, 10), getBalance(poolTokenAccount))
					})

					t.Run("releaseOrMint", func(t *testing.T) {
						require.Equal(t, "0", getBalance(tokenPool.User[admin.PublicKey()]))

						rmI, err := testutils.NewPoolProxyReleaseOrMintInstruction(
							burnmint_tokenpool.ReleaseOrMintInV1{
								LocalToken:          mint,
								SourcePoolAddress:   burnmint_tokenpool.RemoteAddress{Address: remotePool.Address},
								Amount:              tokens.ToLittleEndianU256(amount * 1e9), // scale to proper decimals
								Receiver:            admin.PublicKey(),
								RemoteChainSelector: config.EvmChainSelector,
							},
							poolProgram,
							rampPoolSignerPDA,

							dumbRamp,
							allowedOfframpPDA,
							poolConfig,
							v.tokenProgram,
							mint,
							poolSigner,
							poolTokenAccount,
							tokenPool.Chain[config.EvmChainSelector],
							config.RMNRemoteProgram,
							config.RMNRemoteCursesPDA,
							config.RMNRemoteConfigPDA,
							tokenPool.User[admin.PublicKey()],
						)
						require.NoError(t, err)

						if v.multisig {
							genInst := rmI.(*solana.GenericInstruction)
							genInst.AccountValues = append(genInst.AccountValues, solana.Meta(tokenPool.MintAuthorityMultisig))
						}

						cu := testutils.GetRequiredCU(ctx, t, solanaGoClient, []solana.Instruction{rmI}, admin.PublicKey())

						// This validation is like a snapshot for gas consumption
						// Release or Mint CPI
						require.LessOrEqual(t, cu, uint32(120_000))

						res := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{rmI}, admin, config.DefaultCommitment, common.AddComputeUnitLimit(cu))
						require.NotNil(t, res)

						// validate balances
						require.Equal(t, strconv.FormatUint(amount, 10), getBalance(tokenPool.User[admin.PublicKey()]))
						require.Equal(t, "0", getBalance(poolTokenAccount))
					})

					t.Run("rebalance", func(t *testing.T) {
						// test only relevant for lockrelease pool
						if poolProgram != config.CcipBasePoolLockRelease {
							return
						}

						require.Equal(t, strconv.FormatUint(amount, 10), getBalance(tokenPool.User[admin.PublicKey()]))
						require.Equal(t, "0", getBalance(poolTokenAccount))

						approveIx, err := tokens.TokenApproveChecked(amount, decimals, v.tokenProgram, tokenPool.User[admin.PublicKey()], tokenPool.Mint, poolSigner, admin.PublicKey(), solana.PublicKeySlice{})
						require.NoError(t, err)

						setRebalancerIx, err := tokenpool.NewSetRebalancerInstruction(admin.PublicKey(), poolConfig, mint, admin.PublicKey())
						require.NoError(t, err)

						provideIx, err := tokenpool.NewProvideLiquidityInstruction(amount, poolConfig, v.tokenProgram, tokenPool.Mint, poolSigner, poolTokenAccount, tokenPool.User[admin.PublicKey()], admin.PublicKey())
						require.NoError(t, err)

						testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{
							approveIx,
							&tokens.TokenInstruction{Instruction: setRebalancerIx, Program: poolProgram},
							&tokens.TokenInstruction{Instruction: provideIx, Program: poolProgram},
						}, admin, rpc.CommitmentConfirmed)

						require.Equal(t, "0", getBalance(tokenPool.User[admin.PublicKey()]))
						require.Equal(t, strconv.FormatUint(amount, 10), getBalance(poolTokenAccount))

						withdrawIx, err := tokenpool.NewWithdrawLiquidityInstruction(amount, poolConfig, v.tokenProgram, tokenPool.Mint, poolSigner, poolTokenAccount, tokenPool.User[admin.PublicKey()], admin.PublicKey())
						require.NoError(t, err)

						testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{
							&tokens.TokenInstruction{Instruction: withdrawIx, Program: poolProgram},
						}, admin, rpc.CommitmentConfirmed)

						require.Equal(t, strconv.FormatUint(amount, 10), getBalance(tokenPool.User[admin.PublicKey()]))
						require.Equal(t, "0", getBalance(poolTokenAccount))

						// Test withdraw with u64::MAX (withdraw all)
						// First, provide liquidity again
						approveIx2, err := tokens.TokenApproveChecked(amount, decimals, v.tokenProgram, tokenPool.User[admin.PublicKey()], tokenPool.Mint, poolSigner, admin.PublicKey(), solana.PublicKeySlice{})
						require.NoError(t, err)
						provideIx2, err := tokenpool.NewProvideLiquidityInstruction(amount, poolConfig, v.tokenProgram, tokenPool.Mint, poolSigner, poolTokenAccount, tokenPool.User[admin.PublicKey()], admin.PublicKey())
						require.NoError(t, err)
						testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{
							approveIx2,
							&tokens.TokenInstruction{Instruction: provideIx2, Program: poolProgram},
						}, admin, rpc.CommitmentConfirmed)
						require.Equal(t, "0", getBalance(tokenPool.User[admin.PublicKey()]))
						require.Equal(t, strconv.FormatUint(amount, 10), getBalance(poolTokenAccount))

						// Withdraw all using math.MaxUint64 sentinel
						withdrawAllIx, err := tokenpool.NewWithdrawLiquidityInstruction(math.MaxUint64, poolConfig, v.tokenProgram, tokenPool.Mint, poolSigner, poolTokenAccount, tokenPool.User[admin.PublicKey()], admin.PublicKey())
						require.NoError(t, err)
						testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{
							&tokens.TokenInstruction{Instruction: withdrawAllIx, Program: poolProgram},
						}, admin, rpc.CommitmentConfirmed)
						require.Equal(t, strconv.FormatUint(amount, 10), getBalance(tokenPool.User[admin.PublicKey()]))
						require.Equal(t, "0", getBalance(poolTokenAccount))

						// Test withdraw more than pool balance fails with InsufficientPoolBalance
						// Provide liquidity first
						approveIx3, err := tokens.TokenApproveChecked(amount, decimals, v.tokenProgram, tokenPool.User[admin.PublicKey()], tokenPool.Mint, poolSigner, admin.PublicKey(), solana.PublicKeySlice{})
						require.NoError(t, err)
						provideIx3, err := tokenpool.NewProvideLiquidityInstruction(amount, poolConfig, v.tokenProgram, tokenPool.Mint, poolSigner, poolTokenAccount, tokenPool.User[admin.PublicKey()], admin.PublicKey())
						require.NoError(t, err)
						testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{
							approveIx3,
							&tokens.TokenInstruction{Instruction: provideIx3, Program: poolProgram},
						}, admin, rpc.CommitmentConfirmed)

						withdrawTooMuchIx, err := tokenpool.NewWithdrawLiquidityInstruction(amount+1, poolConfig, v.tokenProgram, tokenPool.Mint, poolSigner, poolTokenAccount, tokenPool.User[admin.PublicKey()], admin.PublicKey())
						require.NoError(t, err)
						testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{
							&tokens.TokenInstruction{Instruction: withdrawTooMuchIx, Program: poolProgram},
						}, admin, rpc.CommitmentConfirmed, []string{"Insufficient pool balance"})

						// Test withdraw u64::MAX on empty pool fails with TransferZeroTokensNotAllowed
						// First withdraw all to empty the pool
						withdrawAllIx2, err := tokenpool.NewWithdrawLiquidityInstruction(amount, poolConfig, v.tokenProgram, tokenPool.Mint, poolSigner, poolTokenAccount, tokenPool.User[admin.PublicKey()], admin.PublicKey())
						require.NoError(t, err)
						testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{
							&tokens.TokenInstruction{Instruction: withdrawAllIx2, Program: poolProgram},
						}, admin, rpc.CommitmentConfirmed)
						require.Equal(t, "0", getBalance(poolTokenAccount))

						withdrawEmptyIx, err := tokenpool.NewWithdrawLiquidityInstruction(math.MaxUint64, poolConfig, v.tokenProgram, tokenPool.Mint, poolSigner, poolTokenAccount, tokenPool.User[admin.PublicKey()], admin.PublicKey())
						require.NoError(t, err)
						testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{
							&tokens.TokenInstruction{Instruction: withdrawEmptyIx, Program: poolProgram},
						}, admin, rpc.CommitmentConfirmed, []string{"Transferring zero tokens is not allowed"})
					})
				})

				// If it wasn't a multisig, then we test the upgrade path to change the mint authority to a multisig
				if p.poolName == "burnmint" && !v.multisig {
					poolSigner, err := tokens.TokenPoolSignerAddress(mint, poolProgram)
					require.NoError(t, err)
					poolConfig, err := tokens.TokenPoolConfigAddress(mint, poolProgram)
					require.NoError(t, err)

					// create multisig
					multisig, err := solana.NewRandomPrivateKey()
					require.NoError(t, err)
					ixMsig, ixErrMsig := tokens.CreateMultisig(ctx, admin.PublicKey(), v.tokenProgram, multisig.PublicKey(), 1, []solana.PublicKey{admin.PublicKey(), poolSigner}, solanaGoClient, config.DefaultCommitment)
					require.NoError(t, ixErrMsig)
					testutils.SendAndConfirm(ctx, t, solanaGoClient, ixMsig, admin, config.DefaultCommitment, common.AddSigners(multisig, admin))

					t.Run("try to upgrade to invalid token program multisig", func(t *testing.T) {
						// create invalidMultisig
						invalidMultisig, err := solana.NewRandomPrivateKey()
						require.NoError(t, err)

						var invalidTokenProgram solana.PublicKey
						if v.tokenProgram == solana.TokenProgramID {
							invalidTokenProgram = solana.Token2022ProgramID
						} else {
							invalidTokenProgram = solana.TokenProgramID
						}
						ixMsig, ixErrMsig := tokens.CreateMultisig(ctx, admin.PublicKey(), invalidTokenProgram, invalidMultisig.PublicKey(), 1, []solana.PublicKey{admin.PublicKey(), poolSigner}, solanaGoClient, config.DefaultCommitment)
						require.NoError(t, ixErrMsig)
						testutils.SendAndConfirm(ctx, t, solanaGoClient, ixMsig, admin, config.DefaultCommitment, common.AddSigners(invalidMultisig, admin))

						newMintAuthority := invalidMultisig.PublicKey()
						tokenPool.MintAuthorityMultisig = newMintAuthority

						// set multisig as mint authority
						ixTransferMint, err := burnmint_tokenpool.NewTransferMintAuthorityToMultisigInstruction(
							poolConfig,
							tokenPool.Mint,
							v.tokenProgram,
							poolSigner,
							admin.PublicKey(),
							newMintAuthority,
							p.poolProgram,
							poolProgramDataAddress,
						)
						require.NoError(t, err)

						testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{
							&tokens.TokenInstruction{Instruction: ixTransferMint, Program: poolProgram},
						}, admin, config.DefaultCommitment, []string{"InvalidMultisigOwner."})

						// Check that the mint authority was not changed in the Mint Account
						mintAccount := token.Mint{}
						err = common.GetAccountDataBorshInto(ctx, solanaGoClient, mint, config.DefaultCommitment, &mintAccount)
						require.NoError(t, err)

						require.Equal(t, poolSigner, *mintAccount.MintAuthority)
					})

					t.Run("try to upgrade to invalid m configured multisig", func(t *testing.T) {
						// create invalidMultisig
						invalidMultisig, err := solana.NewRandomPrivateKey()
						require.NoError(t, err)

						ixMsig, ixErrMsig := tokens.CreateMultisig(ctx, admin.PublicKey(), v.tokenProgram, invalidMultisig.PublicKey(), 2, []solana.PublicKey{admin.PublicKey(), poolSigner}, solanaGoClient, config.DefaultCommitment)
						require.NoError(t, ixErrMsig)
						testutils.SendAndConfirm(ctx, t, solanaGoClient, ixMsig, admin, config.DefaultCommitment, common.AddSigners(invalidMultisig, admin))

						newMintAuthority := invalidMultisig.PublicKey()
						tokenPool.MintAuthorityMultisig = newMintAuthority

						// set multisig as mint authority
						ixTransferMint, err := burnmint_tokenpool.NewTransferMintAuthorityToMultisigInstruction(
							poolConfig,
							tokenPool.Mint,
							v.tokenProgram,
							poolSigner,
							admin.PublicKey(),
							newMintAuthority,
							p.poolProgram,
							poolProgramDataAddress,
						)
						require.NoError(t, err)

						testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{
							&tokens.TokenInstruction{Instruction: ixTransferMint, Program: poolProgram},
						}, admin, config.DefaultCommitment, []string{"PoolSignerNotInMultisig"})

						// Check that the mint authority was not changed in the Mint Account
						mintAccount := token.Mint{}
						err = common.GetAccountDataBorshInto(ctx, solanaGoClient, mint, config.DefaultCommitment, &mintAccount)
						require.NoError(t, err)

						require.Equal(t, poolSigner, *mintAccount.MintAuthority)
					})

					t.Run("try to upgrade to multisig without poolsigner", func(t *testing.T) {
						// create invalidMultisig
						invalidMultisig, err := solana.NewRandomPrivateKey()
						require.NoError(t, err)

						ixMsig, ixErrMsig := tokens.CreateMultisig(ctx, admin.PublicKey(), v.tokenProgram, invalidMultisig.PublicKey(), 1, []solana.PublicKey{admin.PublicKey(), admin.PublicKey()}, solanaGoClient, config.DefaultCommitment)
						require.NoError(t, ixErrMsig)
						testutils.SendAndConfirm(ctx, t, solanaGoClient, ixMsig, admin, config.DefaultCommitment, common.AddSigners(invalidMultisig, admin))

						newMintAuthority := invalidMultisig.PublicKey()
						tokenPool.MintAuthorityMultisig = newMintAuthority

						// set multisig as mint authority
						ixTransferMint, err := burnmint_tokenpool.NewTransferMintAuthorityToMultisigInstruction(
							poolConfig,
							tokenPool.Mint,
							v.tokenProgram,
							poolSigner,
							admin.PublicKey(),
							newMintAuthority,
							p.poolProgram,
							poolProgramDataAddress,
						)
						require.NoError(t, err)

						testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{
							&tokens.TokenInstruction{Instruction: ixTransferMint, Program: poolProgram},
						}, admin, config.DefaultCommitment, []string{"PoolSignerNotInMultisig"})

						// Check that the mint authority was not changed in the Mint Account
						mintAccount := token.Mint{}
						err = common.GetAccountDataBorshInto(ctx, solanaGoClient, mint, config.DefaultCommitment, &mintAccount)
						require.NoError(t, err)

						require.Equal(t, poolSigner, *mintAccount.MintAuthority)
					})
					t.Run("upgrade to valid multisig", func(t *testing.T) {
						newMintAuthority := multisig.PublicKey()
						tokenPool.MintAuthorityMultisig = newMintAuthority

						// set multisig as mint authority
						ixTransferMint, err := burnmint_tokenpool.NewTransferMintAuthorityToMultisigInstruction(
							poolConfig,
							tokenPool.Mint,
							v.tokenProgram,
							poolSigner,
							admin.PublicKey(),
							newMintAuthority,
							p.poolProgram,
							poolProgramDataAddress,
						)
						require.NoError(t, err)

						testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{
							&tokens.TokenInstruction{Instruction: ixTransferMint, Program: poolProgram},
						}, admin, config.DefaultCommitment)

						// Check that the mint authority was changed in the Mint Account
						mintAccount := token.Mint{}
						err = common.GetAccountDataBorshInto(ctx, solanaGoClient, mint, config.DefaultCommitment, &mintAccount)
						require.NoError(t, err)

						require.Equal(t, newMintAuthority, *mintAccount.MintAuthority)
					})
					t.Run("try to transfer back", func(t *testing.T) {
						poolSigner, err := tokens.TokenPoolSignerAddress(mint, poolProgram)
						require.NoError(t, err)
						poolConfig, err := tokens.TokenPoolConfigAddress(mint, poolProgram)
						require.NoError(t, err)

						// rolling back the change needs to be done manually, as the pool program does not support it
						ixTransferMint, err := burnmint_tokenpool.NewTransferMintAuthorityToMultisigInstruction(
							poolConfig,
							tokenPool.Mint,
							v.tokenProgram,
							poolSigner,
							admin.PublicKey(),
							poolSigner,
							p.poolProgram,
							poolProgramDataAddress,
						)
						require.NoError(t, err)

						testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{
							&tokens.TokenInstruction{Instruction: ixTransferMint, Program: poolProgram},
						}, admin, config.DefaultCommitment, []string{"InvalidMultisigOwner"})

						// Check that the mint authority was not changed in the Mint Account
						mintAccount := token.Mint{}
						err = common.GetAccountDataBorshInto(ctx, solanaGoClient, mint, config.DefaultCommitment, &mintAccount)
						require.NoError(t, err)

						require.Equal(t, multisig.PublicKey(), *mintAccount.MintAuthority)
					})
					t.Run("transfer to another multisig and test rollback using transfer_mint_authority_to_pda_signer", func(t *testing.T) {
						poolSigner, err := tokens.TokenPoolSignerAddress(mint, poolProgram)
						require.NoError(t, err)
						poolConfig, err := tokens.TokenPoolConfigAddress(mint, poolProgram)
						require.NoError(t, err)

						// create multisig
						anotherMultisig, err := solana.NewRandomPrivateKey()
						require.NoError(t, err)
						ixMsig, ixErrMsig := tokens.CreateMultisig(ctx, admin.PublicKey(), v.tokenProgram, anotherMultisig.PublicKey(), 1, []solana.PublicKey{admin.PublicKey(), poolSigner}, solanaGoClient, config.DefaultCommitment)
						require.NoError(t, ixErrMsig)
						testutils.SendAndConfirm(ctx, t, solanaGoClient, ixMsig, admin, config.DefaultCommitment, common.AddSigners(anotherMultisig, admin))

						ixTransferMint, err := burnmint_tokenpool.NewTransferMintAuthorityToMultisigInstruction(
							poolConfig,
							tokenPool.Mint,
							v.tokenProgram,
							poolSigner,
							admin.PublicKey(),
							anotherMultisig.PublicKey(),
							p.poolProgram,
							poolProgramDataAddress,
						)
						require.NoError(t, err)

						genInst := ixTransferMint.(*solana.GenericInstruction)
						genInst.AccountValues = append(genInst.AccountValues, solana.Meta(multisig.PublicKey()))

						testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{
							&tokens.TokenInstruction{Instruction: ixTransferMint, Program: poolProgram},
						}, admin, config.DefaultCommitment)

						// Check that the mint authority was changed in the Mint Account
						mintAccount := token.Mint{}
						err = common.GetAccountDataBorshInto(ctx, solanaGoClient, mint, config.DefaultCommitment, &mintAccount)
						require.NoError(t, err)

						require.Equal(t, anotherMultisig.PublicKey(), *mintAccount.MintAuthority)

						t.Run("try to transfer with non-upgrade-authority signer", func(t *testing.T) {
							// Run while mint authority is still anotherMultisig, so the
							// valid_current_mint_authority constraint passes and we reach
							// the Unauthorized check on program_data.
							nonAdmin, err := solana.NewRandomPrivateKey()
							require.NoError(t, err)
							require.NoError(t, testutils.FundAccounts(ctx, []solana.PrivateKey{nonAdmin}, solanaGoClient))

							poolSigner, err := tokens.TokenPoolSignerAddress(mint, poolProgram)
							require.NoError(t, err)
							poolConfig, err := tokens.TokenPoolConfigAddress(mint, poolProgram)
							require.NoError(t, err)

							ixTransferMint, err := burnmint_tokenpool.NewTransferMintAuthorityToPdaSignerInstruction(
								poolConfig,
								tokenPool.Mint,
								v.tokenProgram,
								poolSigner,
								nonAdmin.PublicKey(),
								anotherMultisig.PublicKey(),
								p.poolProgram,
								poolProgramDataAddress,
							)
							require.NoError(t, err)

							testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{
								&tokens.TokenInstruction{Instruction: ixTransferMint, Program: poolProgram},
							}, nonAdmin, config.DefaultCommitment, []string{"Unauthorized"})

							// Check that the mint authority was not changed
							mintAccount := token.Mint{}
							err = common.GetAccountDataBorshInto(ctx, solanaGoClient, mint, config.DefaultCommitment, &mintAccount)
							require.NoError(t, err)

							require.Equal(t, anotherMultisig.PublicKey(), *mintAccount.MintAuthority)
						})

						t.Run("transfer from multisig back to pool signer", func(t *testing.T) {
							poolSigner, err := tokens.TokenPoolSignerAddress(mint, poolProgram)
							require.NoError(t, err)
							poolConfig, err := tokens.TokenPoolConfigAddress(mint, poolProgram)
							require.NoError(t, err)

							ixTransferMint, err := burnmint_tokenpool.NewTransferMintAuthorityToPdaSignerInstruction(
								poolConfig,
								tokenPool.Mint,
								v.tokenProgram,
								poolSigner,
								admin.PublicKey(),
								anotherMultisig.PublicKey(),
								p.poolProgram,
								poolProgramDataAddress,
							)
							require.NoError(t, err)

							testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{
								&tokens.TokenInstruction{Instruction: ixTransferMint, Program: poolProgram},
							}, admin, config.DefaultCommitment)

							// Check that the mint authority was changed back to pool signer
							mintAccount := token.Mint{}
							err = common.GetAccountDataBorshInto(ctx, solanaGoClient, mint, config.DefaultCommitment, &mintAccount)
							require.NoError(t, err)

							require.Equal(t, poolSigner, *mintAccount.MintAuthority)
						})

						t.Run("try to transfer when already pool signer", func(t *testing.T) {
							poolSigner, err := tokens.TokenPoolSignerAddress(mint, poolProgram)
							require.NoError(t, err)
							poolConfig, err := tokens.TokenPoolConfigAddress(mint, poolProgram)
							require.NoError(t, err)

							ixTransferMint, err := burnmint_tokenpool.NewTransferMintAuthorityToPdaSignerInstruction(
								poolConfig,
								tokenPool.Mint,
								v.tokenProgram,
								poolSigner,
								admin.PublicKey(),
								poolSigner,
								p.poolProgram,
								poolProgramDataAddress,
							)
							require.NoError(t, err)

							// The account constraint rejects this because mint.mint_authority == pool_signer,
							// which fails the valid_current_mint_authority check before the function body runs.
							testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{
								&tokens.TokenInstruction{Instruction: ixTransferMint, Program: poolProgram},
							}, admin, config.DefaultCommitment, []string{"InvalidMultisig"})

							// Check that the mint authority was not changed
							mintAccount := token.Mint{}
							err = common.GetAccountDataBorshInto(ctx, solanaGoClient, mint, config.DefaultCommitment, &mintAccount)
							require.NoError(t, err)

							require.Equal(t, poolSigner, *mintAccount.MintAuthority)
						})
					})
				}
			})
		}

		// Test self onboarding to the pool
		t.Run("self-onboard", func(t *testing.T) {
			// Enable self-served token pool onboarding
			ix, err := tokenpool.NewUpdateSelfServedAllowedInstruction(true, configPDA, admin.PublicKey(), poolProgram, poolProgramDataAddress)
			require.NoError(t, err)

			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{
				&tokens.TokenInstruction{Instruction: ix, Program: p.poolProgram},
			}, admin, config.DefaultCommitment)

			// Create a new token owner
			newOwner, err := solana.NewRandomPrivateKey()
			require.NoError(t, err)
			require.NoError(t, testutils.FundAccounts(ctx, []solana.PrivateKey{newOwner}, solanaGoClient))

			// Initialize a Token Pool in the CLL Token Pool Program for my own mint
			for _, v := range tokenPrograms {
				t.Run("self-onboard "+v.tokenName, func(t *testing.T) {
					t.Parallel()

					mintPriv, err := solana.NewRandomPrivateKey()
					require.NoError(t, err)
					p, err := tokens.NewTokenPool(v.tokenProgram, p.poolProgram, mintPriv.PublicKey())
					require.NoError(t, err)
					mint := p.Mint

					// create token
					instructions, err := tokens.CreateToken(ctx, v.tokenProgram, mint, newOwner.PublicKey(), uint8(6), solanaGoClient, config.DefaultCommitment)
					require.NoError(t, err)

					testutils.SendAndConfirm(ctx, t, solanaGoClient, instructions, newOwner, config.DefaultCommitment, common.AddSigners(mintPriv))

					poolConfig, err := tokens.TokenPoolConfigAddress(mint, p.PoolProgram)
					require.NoError(t, err)

					poolInitI, err := tokenpool.NewInitializeInstruction(poolConfig, mint, newOwner.PublicKey(), solana.SystemProgramID, p.PoolProgram, poolProgramDataAddress, configPDA)
					require.NoError(t, err)

					testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{
						&tokens.TokenInstruction{Instruction: poolInitI, Program: p.PoolProgram},
					}, newOwner, config.DefaultCommitment)
				})
			}
		})

		// Test self onboarding with a multisig mint authority
		t.Run("self-onboard-multisig", func(t *testing.T) {
			for _, v := range tokenPrograms {
				if v.multisig {
					continue // skip the "multisig" token variant since we create our own multisig below
				}
				t.Run("multisig self-onboard "+v.tokenName, func(t *testing.T) {
					t.Parallel()

					newOwner, ownerErr := solana.NewRandomPrivateKey()
					require.NoError(t, ownerErr)
					require.NoError(t, testutils.FundAccounts(ctx, []solana.PrivateKey{newOwner}, solanaGoClient))

					// Create a new token
					mintPriv, mintErr := solana.NewRandomPrivateKey()
					require.NoError(t, mintErr)
					tp, tpErr := tokens.NewTokenPool(v.tokenProgram, p.poolProgram, mintPriv.PublicKey())
					require.NoError(t, tpErr)
					mint := tp.Mint

					instructions, createErr := tokens.CreateToken(ctx, v.tokenProgram, mint, newOwner.PublicKey(), uint8(6), solanaGoClient, config.DefaultCommitment)
					require.NoError(t, createErr)
					testutils.SendAndConfirm(ctx, t, solanaGoClient, instructions, newOwner, config.DefaultCommitment, common.AddSigners(mintPriv))

					// Create a multisig (1-of-2) with newOwner and another signer
					anotherSigner, anotherErr := solana.NewRandomPrivateKey()
					require.NoError(t, anotherErr)
					require.NoError(t, testutils.FundAccounts(ctx, []solana.PrivateKey{anotherSigner}, solanaGoClient))

					multisigAcct, msigKeyErr := solana.NewRandomPrivateKey()
					require.NoError(t, msigKeyErr)
					ixMsig, msigErr := tokens.CreateMultisig(ctx, newOwner.PublicKey(), v.tokenProgram, multisigAcct.PublicKey(), 1, []solana.PublicKey{newOwner.PublicKey(), anotherSigner.PublicKey()}, solanaGoClient, config.DefaultCommitment)
					require.NoError(t, msigErr)
					testutils.SendAndConfirm(ctx, t, solanaGoClient, ixMsig, newOwner, config.DefaultCommitment, common.AddSigners(multisigAcct))

					// Transfer mint authority to the multisig
					ixSetAuth, setAuthErr := tokens.SetTokenMintAuthority(v.tokenProgram, multisigAcct.PublicKey(), mint, newOwner.PublicKey())
					require.NoError(t, setAuthErr)
					testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ixSetAuth}, newOwner, config.DefaultCommitment)

					poolConfig, poolConfigErr := tokens.TokenPoolConfigAddress(mint, tp.PoolProgram)
					require.NoError(t, poolConfigErr)

					t.Run("multisig signer can initialize token pool", func(t *testing.T) {
						poolInitI, initErr := tokenpool.NewInitializeByMultisigInstruction(poolConfig, mint, newOwner.PublicKey(), solana.SystemProgramID, tp.PoolProgram, poolProgramDataAddress, configPDA, multisigAcct.PublicKey())
						require.NoError(t, initErr)

						testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{
							&tokens.TokenInstruction{Instruction: poolInitI, Program: tp.PoolProgram},
						}, newOwner, config.DefaultCommitment)
					})
				})
			}
		})
	}
}
