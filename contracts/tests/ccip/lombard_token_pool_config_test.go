// lombard_token_pool_config_test.go tests lombard_token_pool's ADMIN and CONFIG surface, ported
// from the "basic idea" of the EVM LombardTokenPool tests
// (_vendor/chainlink-ccip/chains/evm/contracts/test/pools/LombardTokenPool/*.t.sol), dropping
// EVM-specific mechanics in favor of direct on-chain state assertions (see
// lombard_verifier_config_test.go's doc comment for why: Lombard's instructions use plain
// emit!(), not emit_cpi!(), so there's no existing helper to decode their events in tests).
//
// lock_or_burn_tokens/release_or_mint_tokens (LombardTokenPool.lockOrBurn.t.sol/.releaseOrMint.t.sol)
// aren't ported here: like every other pool tested in tokenpool_test.go (see that file's top
// comment), they're only callable via CPI from the real router/offramp's PDA signer - no test
// keypair can drive them directly. That flow is exercised through the real router/offramp
// end-to-end via the devenv e2e test instead.
package contracts

import (
	"testing"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/base_token_pool"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/lombard_token_pool"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/config"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/testutils"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/tokens"
)

// Ports LombardTokenPool.constructor.t.sol, .setPath.t.sol / ._removePath.t.sol, and
// .getRequiredCCVs.t.sol.
func TestLombardTokenPoolConfig(t *testing.T) {
	ctx := t.Context()

	admin := solana.MustPrivateKeyFromBase58("438yKYsUwihVUzqkQ31QhjieaKwVXYisNv977e39MpXKJygiMZzgrXYwpHH2JNyq8rdEGRNUFnFvN4ki8JfUS2XL")
	user := solana.MustPrivateKeyFromBase58("4xAL7iN37J9LGArJp6xnJxkBrkJNJdjKNmKVocL7d4reN73dPN1ZgetjgKv4wvkz144jtgrKBNnfhEUsbCTa5L6i")

	bridge := solana.MustPublicKeyFromBase58("Lomva5kTftuXE8992qRufkaeq3XrXV47qv3C1W9xW6Z") // mock_lombard_bridge's genesis-loaded address

	solanaGoClient := testutils.DeployAllPrograms(t, testutils.PathToAnchorConfig, admin)

	require.NoError(t, testutils.FundAccounts(ctx, []solana.PrivateKey{admin, user}, solanaGoClient))

	mintPriv, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)
	mint := mintPriv.PublicKey()

	// Not tokens.TokenPoolGlobalConfigPDA: Lombard's PoolConfig moved off the generic "config"
	// seed to make room for VerifierConfig there (see contracts/utils/state/pda.go).
	globalConfigPDA, _, err := state.FindLombardPoolConfigPDA(config.LombardTokenPoolProgram)
	require.NoError(t, err)
	poolConfigPDA, err := tokens.TokenPoolConfigAddress(mint, config.LombardTokenPoolProgram)
	require.NoError(t, err)

	t.Run("setup", func(t *testing.T) {
		t.Run("token", func(t *testing.T) {
			instructions, err1 := tokens.CreateToken(ctx, solana.TokenProgramID, mint, admin.PublicKey(), 9, solanaGoClient, config.DefaultCommitment)
			require.NoError(t, err1)
			testutils.SendAndConfirm(ctx, t, solanaGoClient, instructions, admin, config.DefaultCommitment, common.AddSigners(mintPriv))
		})

		t.Run("init_global_config", func(t *testing.T) {
			programDataAddress := testutils.GetProgramDataAddress(ctx, t, solanaGoClient, config.LombardTokenPoolProgram)
			ix, err1 := lombard_token_pool.NewInitGlobalConfigInstruction(
				globalConfigPDA, admin.PublicKey(), solana.SystemProgramID, config.LombardTokenPoolProgram, programDataAddress,
			)
			require.NoError(t, err1)
			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)

			var cfg lombard_token_pool.PoolConfig
			require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, globalConfigPDA, config.DefaultCommitment, &cfg))
			require.Equal(t, uint8(1), cfg.Version)
		})

		// Ports LombardTokenPool.constructor.t.sol - the pool records the bridge program address
		// it CPIs into for lock_or_burn, which EVM's constructor takes as an immutable too.
		t.Run("initialize", func(t *testing.T) {
			programDataAddress := testutils.GetProgramDataAddress(ctx, t, solanaGoClient, config.LombardTokenPoolProgram)
			ix, err1 := lombard_token_pool.NewInitializeInstruction(
				config.CcipRouterProgram, config.RMNRemoteProgram, bridge,
				poolConfigPDA, mint, admin.PublicKey(), config.LombardTokenPoolProgram, programDataAddress, globalConfigPDA, solana.SystemProgramID,
			)
			require.NoError(t, err1)
			result := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)
			require.NotNil(t, result)

			var state lombard_token_pool.State
			require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, poolConfigPDA, config.DefaultCommitment, &state))
			require.Equal(t, uint8(1), state.Version)
			require.Equal(t, bridge, state.Bridge)
			require.Equal(t, admin.PublicKey(), state.Config.Owner)
			require.Equal(t, mint, state.Config.Mint)
			require.Nil(t, state.Alt)
		})
	})

	remoteToken := lombard_token_pool.RemoteAddress{Address: []byte{4, 5, 6}}
	remotePool := lombard_token_pool.RemoteAddress{Address: []byte{1, 2, 3}}
	destChainID := [32]byte{}
	destChainID[31] = 10
	destCaller := [32]byte{}
	destCaller[31] = 0x11

	chainConfigPDA, _, err := tokens.TokenPoolChainConfigPDA(config.EvmChainSelector, mint, config.LombardTokenPoolProgram)
	require.NoError(t, err)

	t.Run("chain remote config", func(t *testing.T) {
		t.Run("init_chain_remote_config fails when called by non-owner", func(t *testing.T) {
			ix, err := lombard_token_pool.NewInitChainRemoteConfigInstruction(
				config.EvmChainSelector, mint,
				lombard_token_pool.RemoteConfig{TokenAddress: remoteToken, Decimals: 18},
				destChainID, destCaller,
				poolConfigPDA, chainConfigPDA, user.PublicKey(), solana.SystemProgramID,
			)
			require.NoError(t, err)

			// InitializeChainConfig's authority uses a plain `address = state.config.owner`
			// constraint (no custom error override), so Anchor's generic ConstraintAddress error
			// surfaces here rather than CcipTokenPoolError::Unauthorized.
			testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ix}, user, config.DefaultCommitment,
				[]string{"Error Code: " + "ConstraintAddress"})
		})

		t.Run("init_chain_remote_config rejects a zero bridge path", func(t *testing.T) {
			for name, path := range map[string][2][32]byte{
				"chain id": {{}, destCaller},
				"caller":   {destChainID, {}},
			} {
				t.Run(name, func(t *testing.T) {
					ix, err := lombard_token_pool.NewInitChainRemoteConfigInstruction(
						config.EvmChainSelector, mint,
						lombard_token_pool.RemoteConfig{TokenAddress: remoteToken, Decimals: 18},
						path[0], path[1],
						poolConfigPDA, chainConfigPDA, admin.PublicKey(), solana.SystemProgramID,
					)
					require.NoError(t, err)
					testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment,
						[]string{"Error Code: InvalidPath"})
				})
			}
		})

		t.Run("init_chain_remote_config succeeds", func(t *testing.T) {
			ix, err := lombard_token_pool.NewInitChainRemoteConfigInstruction(
				config.EvmChainSelector, mint,
				lombard_token_pool.RemoteConfig{TokenAddress: remoteToken, Decimals: 18},
				destChainID, destCaller,
				poolConfigPDA, chainConfigPDA, admin.PublicKey(), solana.SystemProgramID,
			)
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)

			var cc lombard_token_pool.ChainConfig
			require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, chainConfigPDA, config.DefaultCommitment, &cc))
			require.Equal(t, uint8(1), cc.Version)
			require.Equal(t, remoteToken, cc.Base.Remote.TokenAddress)
			require.Equal(t, uint8(18), cc.Base.Remote.Decimals)
			require.Equal(t, destChainID, cc.Bridge.DestinationChainId)
			require.Equal(t, destCaller, cc.Bridge.DestinationCaller)
		})

		t.Run("append_remote_pool_addresses succeeds", func(t *testing.T) {
			ix, err := lombard_token_pool.NewAppendRemotePoolAddressesInstruction(
				config.EvmChainSelector, mint, []lombard_token_pool.RemoteAddress{remotePool},
				poolConfigPDA, chainConfigPDA, admin.PublicKey(), solana.SystemProgramID,
			)
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)

			var cc lombard_token_pool.ChainConfig
			require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, chainConfigPDA, config.DefaultCommitment, &cc))
			require.Contains(t, cc.Base.Remote.PoolAddresses, remotePool)
		})

		// Ports LombardTokenPool.setPath.t.sol's "edit" idea: dest_chain_id/dest_caller are
		// edited independently of the generic CCIP remote token/pool config.
		newDestChainID := [32]byte{}
		newDestChainID[31] = 20
		newDestCaller := [32]byte{}
		newDestCaller[31] = 0x22

		t.Run("edit_chain_remote_config_lombard fails when called by non-owner", func(t *testing.T) {
			ix, err := lombard_token_pool.NewEditChainRemoteConfigLombardInstruction(
				config.EvmChainSelector, mint,
				lombard_token_pool.LombardChain{DestinationChainId: newDestChainID, DestinationCaller: newDestCaller},
				poolConfigPDA, chainConfigPDA, user.PublicKey(),
			)
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ix}, user, config.DefaultCommitment,
				[]string{"Error Code: " + "Unauthorized"})
		})

		t.Run("edit_chain_remote_config_lombard rejects a zero bridge path", func(t *testing.T) {
			for name, path := range map[string]lombard_token_pool.LombardChain{
				"chain id": {DestinationCaller: newDestCaller},
				"caller":   {DestinationChainId: newDestChainID},
			} {
				t.Run(name, func(t *testing.T) {
					ix, err := lombard_token_pool.NewEditChainRemoteConfigLombardInstruction(
						config.EvmChainSelector, mint, path,
						poolConfigPDA, chainConfigPDA, admin.PublicKey(),
					)
					require.NoError(t, err)
					testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment,
						[]string{"Error Code: InvalidPath"})
				})
			}
		})

		t.Run("edit_chain_remote_config_lombard succeeds", func(t *testing.T) {
			ix, err := lombard_token_pool.NewEditChainRemoteConfigLombardInstruction(
				config.EvmChainSelector, mint,
				lombard_token_pool.LombardChain{DestinationChainId: newDestChainID, DestinationCaller: newDestCaller},
				poolConfigPDA, chainConfigPDA, admin.PublicKey(),
			)
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)

			var cc lombard_token_pool.ChainConfig
			require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, chainConfigPDA, config.DefaultCommitment, &cc))
			require.Equal(t, newDestChainID, cc.Bridge.DestinationChainId)
			require.Equal(t, newDestCaller, cc.Bridge.DestinationCaller)
			// Generic CCIP config from init_chain_remote_config is untouched by the Lombard-only edit.
			require.Equal(t, remoteToken, cc.Base.Remote.TokenAddress)
		})

		t.Run("edit_chain_remote_config overwrites the generic CCIP config", func(t *testing.T) {
			overwrittenToken := lombard_token_pool.RemoteAddress{Address: []byte{7, 8, 9}}
			ix, err := lombard_token_pool.NewEditChainRemoteConfigInstruction(
				config.EvmChainSelector, mint,
				lombard_token_pool.RemoteConfig{TokenAddress: overwrittenToken, Decimals: 6},
				poolConfigPDA, chainConfigPDA, admin.PublicKey(), solana.SystemProgramID,
			)
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)

			var cc lombard_token_pool.ChainConfig
			require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, chainConfigPDA, config.DefaultCommitment, &cc))
			require.Equal(t, overwrittenToken, cc.Base.Remote.TokenAddress)
			require.Equal(t, uint8(6), cc.Base.Remote.Decimals)
			// The Lombard-only bridge config from the previous edit is untouched.
			require.Equal(t, newDestChainID, cc.Bridge.DestinationChainId)
		})
	})

	// Ports LombardTokenPool.getRequiredCCVs.t.sol: the result is hardcoded to this program's own
	// address plus the INCLUDE_DEFAULT_CCVS sentinel, regardless of direction/amount/chain config.
	t.Run("get_required_ccvs", func(t *testing.T) {
		chainConfigV2PDA, _, err := tokens.TokenPoolChainConfigV2PDA(config.EvmChainSelector, mint, config.LombardTokenPoolProgram)
		require.NoError(t, err)

		ix, err := lombard_token_pool.NewGetRequiredCcvsInstruction(
			lombard_token_pool.GetRequiredCcVsIn{
				RemoteChainSelector: config.EvmChainSelector,
				Amount:              lombard_token_pool.ProtocolAmount{BeBytes: tokens.ToBigEndianU256(1_000_000)},
				LocalToken:          mint,
				Direction:           lombard_token_pool.MessageDirection_Outbound,
			},
			poolConfigPDA, chainConfigPDA, chainConfigV2PDA,
		)
		require.NoError(t, err)

		result := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)
		require.NotNil(t, result)
		require.NotNil(t, result.Meta.ReturnData)

		var out lombard_token_pool.GetRequiredCcVsOut
		require.NoError(t, bin.UnmarshalBorsh(&out, result.Meta.ReturnData.Data.Content))
		require.Equal(t, []solana.PublicKey{config.LombardTokenPoolProgram, {}}, out.RequiredCcvs)
	})

	t.Run("nested_resolve_accounts_start rejects an account not owned by the program", func(t *testing.T) {
		ix, err := lombard_token_pool.NewNestedResolveAccountsStartInstruction(
			lombard_token_pool.NestedResolveAccountsParams{
				Caller:        user.PublicKey(),
				CallerProgram: config.CcipRouterProgram,
			},
			mint, // owned by the token program
		)
		require.NoError(t, err)
		testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ix}, user, config.DefaultCommitment,
			[]string{"Error Code: InvalidResolutionAccountOwner"})
	})

	t.Run("query resolvers return the full account list", func(t *testing.T) {
		chainConfigV2PDA, _, err := tokens.TokenPoolChainConfigV2PDA(config.EvmChainSelector, mint, config.LombardTokenPoolProgram)
		require.NoError(t, err)
		resolve := func(inputs []byte) (solana.Instruction, error) {
			return lombard_token_pool.NewNestedResolveAccountsStartInstruction(
				lombard_token_pool.NestedResolveAccountsParams{
					Caller:              user.PublicKey(),
					CallerProgram:       config.CcipRouterProgram,
					CpiResolutionInputs: inputs,
				},
				poolConfigPDA,
			)
		}
		ccvsParams := lombard_token_pool.GetRequiredCcVsIn{
			RemoteChainSelector: config.EvmChainSelector,
			LocalToken:          mint,
			Direction:           lombard_token_pool.MessageDirection_Outbound,
		}
		requireQueriesResolveFullAccountList(ctx, t, solanaGoClient, user, config.LombardTokenPoolProgram, resolve,
			[]solana.PublicKey{poolConfigPDA, chainConfigPDA, chainConfigV2PDA},
			[]poolQuery{
				{"get_required_ccvs", resolutionInputsGetRequiredCcv, &ccvsParams, func(a []solana.PublicKey) (solana.Instruction, error) {
					return lombard_token_pool.NewGetRequiredCcvsInstruction(ccvsParams, a[0], a[1], a[2])
				}},
			},
		)
	})

	// Admin CRUD for the FTF rate-limit override, mirroring tokenpool_test.go's
	// "chain_config_override" coverage for burnmint/lockrelease (base-token-pool's shared
	// init/set/delete_chain_config_override instructions, mirrored 1:1 into lombard-token-pool).
	// Unlike burnmint/lockrelease, actually driving lock_or_burn_v2/release_or_mint_v2 to prove the
	// override bucket (rather than the default one) gets consumed isn't done here: those
	// instructions are only callable via CPI from the real router/offramp (see this file's top
	// comment), and for Lombard that CPI is entangled with the verifier's start_verification_at_source
	// bridge-deposit CPI in the same ccip_send_v2 transaction, which needs the full mock-lombard-bridge
	// setup - that flow is exercised end to end via the devenv e2e test instead.
	t.Run("chain_config_override", func(t *testing.T) {
		overrideType := lombard_token_pool.PoolChainConfigOverrideType_Ftf
		overridePDA, _, err := tokens.TokenPoolChainConfigOverridePDA(config.EvmChainSelector, mint, config.LombardTokenPoolProgram, []byte("ftf"))
		require.NoError(t, err)
		eventAuthority, _, err := state.FindEventAuthorityPDA(config.LombardTokenPoolProgram)
		require.NoError(t, err)

		const overrideCapacity = uint64(1_000_000_000)

		t.Run("init_override fails when called by non-owner", func(t *testing.T) {
			ix, err := lombard_token_pool.NewInitChainConfigOverrideInstruction(
				lombard_token_pool.InitChainConfigOverrideParams{
					RemoteChainSelector: config.EvmChainSelector,
					Mint:                mint,
					OverrideType:        overrideType,
					Inbound:             lombard_token_pool.RateLimitConfig{Enabled: true, Capacity: overrideCapacity, Rate: 1},
					Outbound:            lombard_token_pool.RateLimitConfig{Enabled: true, Capacity: overrideCapacity, Rate: 1},
				},
				poolConfigPDA, overridePDA, user.PublicKey(), solana.SystemProgramID, eventAuthority, config.LombardTokenPoolProgram,
			)
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ix}, user, config.DefaultCommitment,
				[]string{"Error Code: " + "ConstraintAddress"})
		})

		t.Run("init_override succeeds", func(t *testing.T) {
			ix, err := lombard_token_pool.NewInitChainConfigOverrideInstruction(
				lombard_token_pool.InitChainConfigOverrideParams{
					RemoteChainSelector: config.EvmChainSelector,
					Mint:                mint,
					OverrideType:        overrideType,
					Inbound:             lombard_token_pool.RateLimitConfig{Enabled: true, Capacity: overrideCapacity, Rate: 1},
					Outbound:            lombard_token_pool.RateLimitConfig{Enabled: true, Capacity: overrideCapacity, Rate: 1},
				},
				poolConfigPDA, overridePDA, admin.PublicKey(), solana.SystemProgramID, eventAuthority, config.LombardTokenPoolProgram,
			)
			require.NoError(t, err)

			res := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)
			require.NotNil(t, res)

			tx, err := res.Transaction.GetTransaction()
			require.NoError(t, err)
			event, err := common.ExtractCPIEvent(tx, res.Meta, config.LombardTokenPoolProgram, base_token_pool.ParseEvent_OverrideChainConfigConfigured)
			require.NoError(t, err)
			require.Equal(t, config.EvmChainSelector, event.ChainSelector)
			require.Equal(t, base_token_pool.PoolChainConfigOverrideType(overrideType), event.OverrideType)
			require.True(t, event.InboundRateLimit.Enabled)
			require.Equal(t, overrideCapacity, event.InboundRateLimit.Capacity)
			require.True(t, event.OutboundRateLimit.Enabled)
			require.Equal(t, overrideCapacity, event.OutboundRateLimit.Capacity)
			require.Equal(t, mint, event.Mint)

			var override lombard_token_pool.ChainConfigOverride
			require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, overridePDA, config.DefaultCommitment, &override))
			require.Equal(t, uint8(1), override.Version)
			require.Equal(t, config.EvmChainSelector, override.Base.RemoteChainSelector)
			require.Equal(t, mint, override.Base.Mint)
			require.Equal(t, overrideType, override.Base.OverrideType)
			require.True(t, override.Base.InboundRateLimit.Cfg.Enabled)
			require.Equal(t, overrideCapacity, override.Base.InboundRateLimit.Cfg.Capacity)
			require.True(t, override.Base.OutboundRateLimit.Cfg.Enabled)
			require.Equal(t, overrideCapacity, override.Base.OutboundRateLimit.Cfg.Capacity)
		})

		t.Run("set_override_rate_limit succeeds", func(t *testing.T) {
			newCapacity := overrideCapacity / 2
			ix, err := lombard_token_pool.NewSetChainConfigOverrideRateLimitInstruction(
				lombard_token_pool.SetChainConfigOverrideRateLimitParams{
					RemoteChainSelector: config.EvmChainSelector,
					Mint:                mint,
					OverrideType:        overrideType,
					Inbound:             lombard_token_pool.RateLimitConfig{Enabled: true, Capacity: newCapacity, Rate: 1},
					Outbound:            lombard_token_pool.RateLimitConfig{Enabled: true, Capacity: newCapacity, Rate: 1},
				},
				poolConfigPDA, overridePDA, admin.PublicKey(), eventAuthority, config.LombardTokenPoolProgram,
			)
			require.NoError(t, err)

			res := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)
			require.NotNil(t, res)

			var override lombard_token_pool.ChainConfigOverride
			require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, overridePDA, config.DefaultCommitment, &override))
			require.Equal(t, newCapacity, override.Base.InboundRateLimit.Cfg.Capacity)
			require.Equal(t, newCapacity, override.Base.OutboundRateLimit.Cfg.Capacity)
		})

		t.Run("set_override_rate_limit fails when called by unauthorized user", func(t *testing.T) {
			ix, err := lombard_token_pool.NewSetChainConfigOverrideRateLimitInstruction(
				lombard_token_pool.SetChainConfigOverrideRateLimitParams{
					RemoteChainSelector: config.EvmChainSelector,
					Mint:                mint,
					OverrideType:        overrideType,
					Inbound:             lombard_token_pool.RateLimitConfig{Enabled: true, Capacity: overrideCapacity, Rate: 1},
					Outbound:            lombard_token_pool.RateLimitConfig{Enabled: true, Capacity: overrideCapacity, Rate: 1},
				},
				poolConfigPDA, overridePDA, user.PublicKey(), eventAuthority, config.LombardTokenPoolProgram,
			)
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ix}, user, config.DefaultCommitment,
				[]string{"Error Code: Unauthorized"})
		})

		t.Run("init_override fails when already initialized", func(t *testing.T) {
			ix, err := lombard_token_pool.NewInitChainConfigOverrideInstruction(
				lombard_token_pool.InitChainConfigOverrideParams{
					RemoteChainSelector: config.EvmChainSelector,
					Mint:                mint,
					OverrideType:        overrideType,
					Inbound:             lombard_token_pool.RateLimitConfig{Enabled: true, Capacity: overrideCapacity, Rate: 1},
					Outbound:            lombard_token_pool.RateLimitConfig{Enabled: true, Capacity: overrideCapacity, Rate: 1},
				},
				poolConfigPDA, overridePDA, admin.PublicKey(), solana.SystemProgramID, eventAuthority, config.LombardTokenPoolProgram,
			)
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment,
				[]string{"already in use"})
		})

		t.Run("delete_override fails when called by non-owner", func(t *testing.T) {
			ix, err := lombard_token_pool.NewDeleteChainConfigOverrideInstruction(
				lombard_token_pool.DeleteChainConfigOverrideParams{
					RemoteChainSelector: config.EvmChainSelector,
					Mint:                mint,
					OverrideType:        overrideType,
				},
				poolConfigPDA, overridePDA, user.PublicKey(), eventAuthority, config.LombardTokenPoolProgram,
			)
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ix}, user, config.DefaultCommitment,
				[]string{"Error Code: " + "ConstraintAddress"})
		})

		t.Run("delete_override succeeds", func(t *testing.T) {
			ix, err := lombard_token_pool.NewDeleteChainConfigOverrideInstruction(
				lombard_token_pool.DeleteChainConfigOverrideParams{
					RemoteChainSelector: config.EvmChainSelector,
					Mint:                mint,
					OverrideType:        overrideType,
				},
				poolConfigPDA, overridePDA, admin.PublicKey(), eventAuthority, config.LombardTokenPoolProgram,
			)
			require.NoError(t, err)

			res := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)
			require.NotNil(t, res)

			tx, err := res.Transaction.GetTransaction()
			require.NoError(t, err)
			event, err := common.ExtractCPIEvent(tx, res.Meta, config.LombardTokenPoolProgram, base_token_pool.ParseEvent_OverrideChainConfigRemoved)
			require.NoError(t, err)
			require.Equal(t, config.EvmChainSelector, event.ChainSelector)
			require.Equal(t, base_token_pool.PoolChainConfigOverrideType(overrideType), event.OverrideType)
			require.Equal(t, mint, event.Mint)

			testutils.AssertClosedAccount(ctx, t, solanaGoClient, overridePDA, config.DefaultCommitment)
		})
	})

	// Ports LombardTokenPool_removePath.t.sol.
	t.Run("delete_chain_config", func(t *testing.T) {
		t.Run("fails when called by non-owner", func(t *testing.T) {
			ix, err := lombard_token_pool.NewDeleteChainConfigInstruction(
				config.EvmChainSelector, mint, poolConfigPDA, chainConfigPDA, user.PublicKey(),
			)
			require.NoError(t, err)

			// Same plain `address = state.config.owner` constraint as InitializeChainConfig above.
			testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ix}, user, config.DefaultCommitment,
				[]string{"Error Code: " + "ConstraintAddress"})
		})

		t.Run("succeeds and closes the account", func(t *testing.T) {
			ix, err := lombard_token_pool.NewDeleteChainConfigInstruction(
				config.EvmChainSelector, mint, poolConfigPDA, chainConfigPDA, admin.PublicKey(),
			)
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)

			_, err = solanaGoClient.GetAccountInfoWithOpts(ctx, chainConfigPDA, &rpc.GetAccountInfoOpts{Commitment: config.DefaultCommitment})
			require.ErrorContains(t, err, "not found")
		})
	})
}
