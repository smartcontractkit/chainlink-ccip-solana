// lombard_verifier_config_test.go tests the configuration and setup of Lombard's embedded
// verifier (the same lombard_token_pool program as the pool, with its own PDAs/instructions).
// Ports the "basic idea" of the EVM LombardVerifier config-style tests
// (_vendor/chainlink-ccip/chains/evm/contracts/test/ccvs/LombardVerifier/*.t.sol), dropping
// EVM-specific mechanics (Ownable2Step reverts, event-based assertions - Lombard's verifier
// instructions use plain emit!(), not emit_cpi!(), so there's no existing test helper to decode
// them the way committee_verifier_config_test.go does with ExtractCPIEvent) in favor of
// asserting on-chain state directly. verifyMessage/forwardToVerifier/withdrawFeeTokens aren't
// ported here: they depend on Lombard's EVM-specific ABI-encoded GMP envelope format, which has
// no Solana analog - message-ID-binding verification is exercised in the devenv e2e test instead.
package contracts

import (
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/lombard_token_pool"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/config"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/testutils"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
)

func TestLombardVerifierConfig(t *testing.T) {
	ctx := t.Context()

	admin := solana.MustPrivateKeyFromBase58("5aPkg2kDDPHfTu76ahwZTLJbtxTbVaj8iKdWcxPQavNa4HUTzDpqd7LVk7oXNmH5gQZ7SF2AYecEg57Hm8MTrzEa")
	user := solana.MustPrivateKeyFromBase58("34FKwin2P89rJfKmnX2icXxtApSN6BXi7b5DMGY1GxZBKwKGR6Y6uFDxoPKTcbf3kz7t4RyM8hTpo8JVHi1VZij3")
	newAdmin := solana.MustPrivateKeyFromBase58("5sJYiPbCMGVPPYMvScqjjt15TJQiZEDsDGAELzAJG46PUnrfeoNKnndqtjrq9BcURci1AZVjF9UH7wv3MYyhjnv1")

	routerSigner := solana.MustPublicKeyFromBase58("D8Bkq9tRy8n1RhYYaqPZbNPjwqPy1cW2rSNTHtCwFZ6c")

	remoteChainSelector := config.EvmChainSelector

	solanaGoClient := testutils.DeployAllPrograms(t, testutils.PathToAnchorConfig, admin)

	t.Run("setup", func(t *testing.T) {
		t.Run("funding", func(t *testing.T) {
			require.NoError(t, testutils.FundAccounts(ctx, []solana.PrivateKey{admin, user, newAdmin}, solanaGoClient))
		})

		t.Run("initialize", func(t *testing.T) {
			programDataAddress := testutils.GetProgramDataAddress(ctx, t, solanaGoClient, config.LombardTokenPoolProgram)

			ix, err := lombard_token_pool.NewInitializeVerifierInstruction(
				config.LombardVerifierConfigPDA,
				config.LombardVerifierStorageLocationsPDA,
				admin.PublicKey(),
				solana.SystemProgramID,
				config.LombardTokenPoolProgram,
				programDataAddress,
			)
			require.NoError(t, err)

			result := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)
			require.NotNil(t, result)

			var cfg lombard_token_pool.VerifierConfig
			require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, config.LombardVerifierConfigPDA, config.DefaultCommitment, &cfg))
			require.Equal(t, uint8(1), cfg.Version)
			require.Equal(t, admin.PublicKey(), cfg.Owner)
			require.Equal(t, solana.PublicKey{}, cfg.ProposedOwner)
			require.Equal(t, admin.PublicKey(), cfg.StorageLocationAdmin)
			require.Equal(t, admin.PublicKey(), cfg.AllowlistAdmin)
			require.Empty(t, cfg.SupportedTokens)

			// Ports LombardVerifier.versionTag.t.sol: versionTag is a compile-time-derived domain
			// separation tag (keccak256("LombardVerifier 2.0.0")[:4]) burned into message binding.
			require.NotEqual(t, [4]uint8{}, cfg.VersionTag)
		})
	})

	t.Run("ownership", func(t *testing.T) {
		t.Run("transfer_ownership fails when called by non-owner", func(t *testing.T) {
			ix, err := lombard_token_pool.NewTransferVerifierOwnershipInstruction(
				newAdmin.PublicKey(), config.LombardVerifierConfigPDA, user.PublicKey(),
			)
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ix}, user, config.DefaultCommitment,
				[]string{"Error Code: " + "Unauthorized"})
		})

		t.Run("transfer_ownership fails with default pubkey", func(t *testing.T) {
			ix, err := lombard_token_pool.NewTransferVerifierOwnershipInstruction(
				solana.PublicKey{}, config.LombardVerifierConfigPDA, admin.PublicKey(),
			)
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment,
				[]string{"Error Code: " + "DefaultAdminProposal"})
		})

		t.Run("transfer_ownership fails when proposing current owner", func(t *testing.T) {
			ix, err := lombard_token_pool.NewTransferVerifierOwnershipInstruction(
				admin.PublicKey(), config.LombardVerifierConfigPDA, admin.PublicKey(),
			)
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment,
				[]string{"Error Code: " + "RedundantAdminProposal"})
		})

		t.Run("transfer_ownership succeeds", func(t *testing.T) {
			ix, err := lombard_token_pool.NewTransferVerifierOwnershipInstruction(
				newAdmin.PublicKey(), config.LombardVerifierConfigPDA, admin.PublicKey(),
			)
			require.NoError(t, err)

			result := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)
			require.NotNil(t, result)

			var cfg lombard_token_pool.VerifierConfig
			require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, config.LombardVerifierConfigPDA, config.DefaultCommitment, &cfg))
			require.Equal(t, admin.PublicKey(), cfg.Owner)
			require.Equal(t, newAdmin.PublicKey(), cfg.ProposedOwner)
		})

		t.Run("accept_ownership fails when called by wrong signer", func(t *testing.T) {
			ix, err := lombard_token_pool.NewAcceptVerifierOwnershipInstruction(config.LombardVerifierConfigPDA, user.PublicKey())
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ix}, user, config.DefaultCommitment,
				[]string{"Error Code: " + "Unauthorized"})
		})

		t.Run("accept_ownership succeeds", func(t *testing.T) {
			ix, err := lombard_token_pool.NewAcceptVerifierOwnershipInstruction(config.LombardVerifierConfigPDA, newAdmin.PublicKey())
			require.NoError(t, err)

			result := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, newAdmin, config.DefaultCommitment)
			require.NotNil(t, result)

			var cfg lombard_token_pool.VerifierConfig
			require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, config.LombardVerifierConfigPDA, config.DefaultCommitment, &cfg))
			require.Equal(t, newAdmin.PublicKey(), cfg.Owner)
			require.Equal(t, solana.PublicKey{}, cfg.ProposedOwner)
		})

		t.Run("restore admin to original owner for later subtests", func(t *testing.T) {
			ix, err := lombard_token_pool.NewTransferVerifierOwnershipInstruction(
				admin.PublicKey(), config.LombardVerifierConfigPDA, newAdmin.PublicKey(),
			)
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, newAdmin, config.DefaultCommitment)

			ix, err = lombard_token_pool.NewAcceptVerifierOwnershipInstruction(config.LombardVerifierConfigPDA, admin.PublicKey())
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)
		})
	})

	t.Run("storage location admin", func(t *testing.T) {
		t.Run("transfer fails when called by non-owner", func(t *testing.T) {
			ix, err := lombard_token_pool.NewTransferVerifierStorageLocationAdminInstruction(
				newAdmin.PublicKey(), config.LombardVerifierConfigPDA, user.PublicKey(),
			)
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ix}, user, config.DefaultCommitment,
				[]string{"Error Code: " + "Unauthorized"})
		})

		t.Run("transfer and accept succeed", func(t *testing.T) {
			ix, err := lombard_token_pool.NewTransferVerifierStorageLocationAdminInstruction(
				newAdmin.PublicKey(), config.LombardVerifierConfigPDA, admin.PublicKey(),
			)
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)

			ix, err = lombard_token_pool.NewAcceptVerifierStorageLocationAdminInstruction(config.LombardVerifierConfigPDA, newAdmin.PublicKey())
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, newAdmin, config.DefaultCommitment)

			var cfg lombard_token_pool.VerifierConfig
			require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, config.LombardVerifierConfigPDA, config.DefaultCommitment, &cfg))
			require.Equal(t, newAdmin.PublicKey(), cfg.StorageLocationAdmin)

			// restore for later subtests - transfer_verifier_storage_location_admin is
			// owner-gated (not storage-location-admin-gated), so admin (still owner) is the
			// signer here even though newAdmin currently holds the storage_location_admin role.
			ix, err = lombard_token_pool.NewTransferVerifierStorageLocationAdminInstruction(
				admin.PublicKey(), config.LombardVerifierConfigPDA, admin.PublicKey(),
			)
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)
			ix, err = lombard_token_pool.NewAcceptVerifierStorageLocationAdminInstruction(config.LombardVerifierConfigPDA, admin.PublicKey())
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)
		})
	})

	t.Run("set_verifier_allowlist_admin", func(t *testing.T) {
		t.Run("fails when called by non-owner", func(t *testing.T) {
			ix, err := lombard_token_pool.NewSetVerifierAllowlistAdminInstruction(newAdmin.PublicKey(), config.LombardVerifierConfigPDA, user.PublicKey())
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ix}, user, config.DefaultCommitment,
				[]string{"Error Code: " + "Unauthorized"})
		})

		t.Run("succeeds", func(t *testing.T) {
			ix, err := lombard_token_pool.NewSetVerifierAllowlistAdminInstruction(admin.PublicKey(), config.LombardVerifierConfigPDA, admin.PublicKey())
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)

			var cfg lombard_token_pool.VerifierConfig
			require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, config.LombardVerifierConfigPDA, config.DefaultCommitment, &cfg))
			require.Equal(t, admin.PublicKey(), cfg.AllowlistAdmin)
		})
	})

	t.Run("set_verifier_fee_aggregator", func(t *testing.T) {
		t.Run("fails when called by non-owner", func(t *testing.T) {
			ix, err := lombard_token_pool.NewSetVerifierFeeAggregatorInstruction(admin.PublicKey(), config.LombardVerifierConfigPDA, user.PublicKey())
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ix}, user, config.DefaultCommitment,
				[]string{"Error Code: " + "Unauthorized"})
		})

		t.Run("succeeds", func(t *testing.T) {
			ix, err := lombard_token_pool.NewSetVerifierFeeAggregatorInstruction(admin.PublicKey(), config.LombardVerifierConfigPDA, admin.PublicKey())
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)

			var cfg lombard_token_pool.VerifierConfig
			require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, config.LombardVerifierConfigPDA, config.DefaultCommitment, &cfg))
			require.Equal(t, admin.PublicKey(), cfg.FeeAggregator)
		})
	})

	// Ports LombardVerifier.setAllowedFinalityConfig.t.sol.
	t.Run("set_verifier_allowed_finality_config", func(t *testing.T) {
		newFinality := lombard_token_pool.FinalityConfig{Flags: 0, BlockDepth: 42}

		t.Run("fails when called by non-owner", func(t *testing.T) {
			ix, err := lombard_token_pool.NewSetVerifierAllowedFinalityConfigInstruction(newFinality, config.LombardVerifierConfigPDA, user.PublicKey())
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ix}, user, config.DefaultCommitment,
				[]string{"Error Code: " + "Unauthorized"})
		})

		t.Run("succeeds", func(t *testing.T) {
			ix, err := lombard_token_pool.NewSetVerifierAllowedFinalityConfigInstruction(newFinality, config.LombardVerifierConfigPDA, admin.PublicKey())
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)

			var cfg lombard_token_pool.VerifierConfig
			require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, config.LombardVerifierConfigPDA, config.DefaultCommitment, &cfg))
			require.Equal(t, newFinality, cfg.AllowedFinalityConfig)
		})
	})

	// Ports LombardVerifier.updateStorageLocations.t.sol.
	t.Run("set_verifier_storage_locations", func(t *testing.T) {
		t.Run("fails when called by non-storage-location-admin", func(t *testing.T) {
			ix, err := lombard_token_pool.NewSetVerifierStorageLocationsInstruction(
				lombard_token_pool.SetVerifierStorageLocationsParams{StorageLocations: []lombard_token_pool.ByteString{{V0: []byte("new/location")}}},
				config.LombardVerifierConfigPDA, user.PublicKey(), config.LombardVerifierStorageLocationsPDA, solana.SystemProgramID,
			)
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ix}, user, config.DefaultCommitment,
				[]string{"Error Code: " + "Unauthorized"})
		})

		t.Run("succeeds and is readable via get_verifier_storage_locations", func(t *testing.T) {
			newLocations := []lombard_token_pool.ByteString{{V0: []byte("https://test.chain.link.fake")}}
			ix, err := lombard_token_pool.NewSetVerifierStorageLocationsInstruction(
				lombard_token_pool.SetVerifierStorageLocationsParams{StorageLocations: newLocations},
				config.LombardVerifierConfigPDA, admin.PublicKey(), config.LombardVerifierStorageLocationsPDA, solana.SystemProgramID,
			)
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)

			var locs lombard_token_pool.VerifierStorageLocations
			require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, config.LombardVerifierStorageLocationsPDA, config.DefaultCommitment, &locs))
			require.Equal(t, newLocations, locs.Locations)
		})
	})

	// Ports LombardVerifier.setPath.t.sol / .removePaths.t.sol, and lays the groundwork
	// (init_verifier_chain_config) for setRemoteAdapters.t.sol / applyAllowlistUpdates.t.sol below.
	t.Run("chain config, path, remote adapters, allowlist", func(t *testing.T) {
		t.Run("init_verifier_chain_config fails with zero router signer", func(t *testing.T) {
			ix, err := lombard_token_pool.NewInitVerifierChainConfigInstruction(
				lombard_token_pool.SetVerifierChainConfigParams{
					RemoteChainSelector: remoteChainSelector,
					ChainConfig: lombard_token_pool.VerifierRemoteChainAdminConfig{
						RouterSigner:       solana.PublicKey{},
						Fee:                lombard_token_pool.UsdCents{V0: 45},
						GasForVerification: lombard_token_pool.CrossChainGas{V0: 500_000},
						PayloadSizeBytes:   448,
					},
				},
				config.LombardVerifierConfigPDA, admin.PublicKey(), config.LombardVerifierEvmRemoteChainConfigPDA, solana.SystemProgramID,
			)
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment,
				[]string{"Error Code: " + "ZeroRouterSigner"})
		})

		t.Run("init_verifier_chain_config fails when called by non-owner", func(t *testing.T) {
			ix, err := lombard_token_pool.NewInitVerifierChainConfigInstruction(
				lombard_token_pool.SetVerifierChainConfigParams{
					RemoteChainSelector: remoteChainSelector,
					ChainConfig: lombard_token_pool.VerifierRemoteChainAdminConfig{
						RouterSigner:       routerSigner,
						Fee:                lombard_token_pool.UsdCents{V0: 45},
						GasForVerification: lombard_token_pool.CrossChainGas{V0: 500_000},
						PayloadSizeBytes:   448,
					},
				},
				config.LombardVerifierConfigPDA, user.PublicKey(), config.LombardVerifierEvmRemoteChainConfigPDA, solana.SystemProgramID,
			)
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ix}, user, config.DefaultCommitment,
				[]string{"Error Code: " + "Unauthorized"})
		})

		t.Run("init_verifier_chain_config succeeds", func(t *testing.T) {
			ix, err := lombard_token_pool.NewInitVerifierChainConfigInstruction(
				lombard_token_pool.SetVerifierChainConfigParams{
					RemoteChainSelector: remoteChainSelector,
					ChainConfig: lombard_token_pool.VerifierRemoteChainAdminConfig{
						RouterSigner:       routerSigner,
						Fee:                lombard_token_pool.UsdCents{V0: 45},
						GasForVerification: lombard_token_pool.CrossChainGas{V0: 500_000},
						PayloadSizeBytes:   448,
					},
				},
				config.LombardVerifierConfigPDA, admin.PublicKey(), config.LombardVerifierEvmRemoteChainConfigPDA, solana.SystemProgramID,
			)
			require.NoError(t, err)
			result := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)
			require.NotNil(t, result)

			var rc lombard_token_pool.VerifierRemoteChainConfig
			require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, config.LombardVerifierEvmRemoteChainConfigPDA, config.DefaultCommitment, &rc))
			require.Equal(t, uint8(1), rc.Version)
			require.Equal(t, remoteChainSelector, rc.RemoteChainSelector)
			require.Equal(t, routerSigner, rc.AdminConfig.RouterSigner)
			require.Equal(t, uint32(45), rc.AdminConfig.Fee.V0)
			require.Equal(t, uint32(500_000), rc.AdminConfig.GasForVerification.V0)
			require.Equal(t, uint32(448), rc.AdminConfig.PayloadSizeBytes)
		})

		lombardChainID := [32]byte{}
		lombardChainID[31] = 42
		allowedCaller := [32]byte{}
		allowedCaller[31] = 0x12
		remoteBridgeSender := [32]byte{}
		remoteBridgeSender[31] = 0x34

		t.Run("set_verifier_path fails with zero fields", func(t *testing.T) {
			ix, err := lombard_token_pool.NewSetVerifierPathInstruction(
				remoteChainSelector, [32]byte{}, allowedCaller, remoteBridgeSender,
				config.LombardVerifierConfigPDA, admin.PublicKey(), config.LombardVerifierEvmRemoteChainConfigPDA,
			)
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment,
				[]string{"Error Code: " + "InvalidPath"})
		})

		t.Run("set_verifier_path fails when called by non-owner", func(t *testing.T) {
			ix, err := lombard_token_pool.NewSetVerifierPathInstruction(
				remoteChainSelector, lombardChainID, allowedCaller, remoteBridgeSender,
				config.LombardVerifierConfigPDA, user.PublicKey(), config.LombardVerifierEvmRemoteChainConfigPDA,
			)
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ix}, user, config.DefaultCommitment,
				[]string{"Error Code: " + "Unauthorized"})
		})

		t.Run("set_verifier_path succeeds and is readable", func(t *testing.T) {
			ix, err := lombard_token_pool.NewSetVerifierPathInstruction(
				remoteChainSelector, lombardChainID, allowedCaller, remoteBridgeSender,
				config.LombardVerifierConfigPDA, admin.PublicKey(), config.LombardVerifierEvmRemoteChainConfigPDA,
			)
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)

			var rc lombard_token_pool.VerifierRemoteChainConfig
			require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, config.LombardVerifierEvmRemoteChainConfigPDA, config.DefaultCommitment, &rc))
			require.Equal(t, lombard_token_pool.LombardVerifierPath{
				AllowedCaller:      allowedCaller,
				LChainId:           lombardChainID,
				RemoteBridgeSender: remoteBridgeSender,
			}, rc.Path)
		})

		// Ports LombardVerifier.setRemoteAdapters.t.sol - requires init_verifier_chain_config to
		// have already created the remote chain config account.
		t.Run("set_verifier_remote_adapters fails when called by non-owner", func(t *testing.T) {
			ix, err := lombard_token_pool.NewSetVerifierRemoteAdaptersInstruction(
				remoteChainSelector, []lombard_token_pool.RemoteAdapter{{Token: config.LombardTokenPoolProgram, RemoteAdapter: [32]byte{1}}},
				config.LombardVerifierConfigPDA, user.PublicKey(), config.LombardVerifierEvmRemoteChainConfigPDA, solana.SystemProgramID,
			)
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ix}, user, config.DefaultCommitment,
				[]string{"Error Code: " + "Unauthorized"})
		})

		t.Run("set_verifier_remote_adapters succeeds and is readable", func(t *testing.T) {
			localToken := solana.MustPublicKeyFromBase58("So11111111111111111111111111111111111111112")
			remoteAdapter := [32]byte{}
			remoteAdapter[31] = 0x99

			ix, err := lombard_token_pool.NewSetVerifierRemoteAdaptersInstruction(
				remoteChainSelector, []lombard_token_pool.RemoteAdapter{{Token: localToken, RemoteAdapter: remoteAdapter}},
				config.LombardVerifierConfigPDA, admin.PublicKey(), config.LombardVerifierEvmRemoteChainConfigPDA, solana.SystemProgramID,
			)
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)

			var rc lombard_token_pool.VerifierRemoteChainConfig
			require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, config.LombardVerifierEvmRemoteChainConfigPDA, config.DefaultCommitment, &rc))
			require.Contains(t, rc.RemoteAdapters, lombard_token_pool.RemoteAdapter{Token: localToken, RemoteAdapter: remoteAdapter})
		})

		// The owner or the allowlist admin can set the allowlist, like the committee and CCTP verifiers.
		t.Run("set_verifier_allowlist fails when called by neither the owner nor the allowlist admin", func(t *testing.T) {
			ix, err := lombard_token_pool.NewSetVerifierAllowlistInstruction(
				lombard_token_pool.SetVerifierAllowlistParams{DestChainSelector: remoteChainSelector, Enabled: true, AllowedSenders: []solana.PublicKey{user.PublicKey()}},
				config.LombardVerifierConfigPDA, user.PublicKey(), config.LombardVerifierEvmRemoteChainConfigPDA, solana.SystemProgramID,
			)
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ix}, user, config.DefaultCommitment,
				[]string{"Error Code: " + "Unauthorized"})
		})

		t.Run("set_verifier_allowlist succeeds and is readable", func(t *testing.T) {
			ix, err := lombard_token_pool.NewSetVerifierAllowlistInstruction(
				lombard_token_pool.SetVerifierAllowlistParams{DestChainSelector: remoteChainSelector, Enabled: true, AllowedSenders: []solana.PublicKey{user.PublicKey()}},
				config.LombardVerifierConfigPDA, admin.PublicKey(), config.LombardVerifierEvmRemoteChainConfigPDA, solana.SystemProgramID,
			)
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)

			var rc lombard_token_pool.VerifierRemoteChainConfig
			require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, config.LombardVerifierEvmRemoteChainConfigPDA, config.DefaultCommitment, &rc))
			require.True(t, rc.AllowlistConfig.AllowlistEnabled)
			require.Equal(t, []solana.PublicKey{user.PublicKey()}, rc.AllowlistConfig.AllowedSenders)
		})

		t.Run("set_verifier_allowlist succeeds when called by the owner or the allowlist admin", func(t *testing.T) {
			setAdmin := func(t *testing.T, allowlistAdmin solana.PublicKey) {
				ix, err := lombard_token_pool.NewSetVerifierAllowlistAdminInstruction(allowlistAdmin, config.LombardVerifierConfigPDA, admin.PublicKey())
				require.NoError(t, err)
				testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)
			}
			setAllowlist := func(t *testing.T, signer solana.PrivateKey, senders []solana.PublicKey) {
				ix, err := lombard_token_pool.NewSetVerifierAllowlistInstruction(
					lombard_token_pool.SetVerifierAllowlistParams{DestChainSelector: remoteChainSelector, Enabled: true, AllowedSenders: senders},
					config.LombardVerifierConfigPDA, signer.PublicKey(), config.LombardVerifierEvmRemoteChainConfigPDA, solana.SystemProgramID,
				)
				require.NoError(t, err)
				testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, signer, config.DefaultCommitment)
				var rc lombard_token_pool.VerifierRemoteChainConfig
				require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, config.LombardVerifierEvmRemoteChainConfigPDA, config.DefaultCommitment, &rc))
				require.Equal(t, senders, rc.AllowlistConfig.AllowedSenders)
			}

			// Give the allowlist admin role to another key, so the owner and the admin differ.
			setAdmin(t, newAdmin.PublicKey())
			defer setAdmin(t, admin.PublicKey())

			setAllowlist(t, newAdmin, []solana.PublicKey{newAdmin.PublicKey()})
			setAllowlist(t, admin, []solana.PublicKey{user.PublicKey()})
		})

		// Regression: remote adapters and allowed senders grow the account past its initial size,
		// which used to make every later chain config update fail with ConstraintSpace.
		t.Run("edit_verifier_chain_config succeeds and preserves the rest of the config", func(t *testing.T) {
			var before lombard_token_pool.VerifierRemoteChainConfig
			require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, config.LombardVerifierEvmRemoteChainConfigPDA, config.DefaultCommitment, &before))
			require.NotEmpty(t, before.RemoteAdapters)
			require.NotEmpty(t, before.AllowlistConfig.AllowedSenders)

			adminConfig := before.AdminConfig
			adminConfig.Fee = lombard_token_pool.UsdCents{V0: before.AdminConfig.Fee.V0 + 5}
			ix, err := lombard_token_pool.NewEditVerifierChainConfigInstruction(
				lombard_token_pool.SetVerifierChainConfigParams{RemoteChainSelector: remoteChainSelector, ChainConfig: adminConfig},
				config.LombardVerifierConfigPDA, admin.PublicKey(), config.LombardVerifierEvmRemoteChainConfigPDA,
			)
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)

			var after lombard_token_pool.VerifierRemoteChainConfig
			require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, config.LombardVerifierEvmRemoteChainConfigPDA, config.DefaultCommitment, &after))
			require.Equal(t, adminConfig, after.AdminConfig)
			before.AdminConfig = adminConfig
			require.Equal(t, before, after)
		})

		t.Run("edit_verifier_chain_config fails when called by non-owner", func(t *testing.T) {
			ix, err := lombard_token_pool.NewEditVerifierChainConfigInstruction(
				lombard_token_pool.SetVerifierChainConfigParams{
					RemoteChainSelector: remoteChainSelector,
					ChainConfig:         lombard_token_pool.VerifierRemoteChainAdminConfig{RouterSigner: routerSigner},
				},
				config.LombardVerifierConfigPDA, user.PublicKey(), config.LombardVerifierEvmRemoteChainConfigPDA,
			)
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ix}, user, config.DefaultCommitment,
				[]string{"Error Code: " + "Unauthorized"})
		})

		// Ports LombardVerifier.setPath's remove idea (LombardVerifier.removePaths.t.sol).
		t.Run("remove_verifier_path succeeds and zeroes the path", func(t *testing.T) {
			ix, err := lombard_token_pool.NewRemoveVerifierPathInstruction(
				remoteChainSelector, config.LombardVerifierConfigPDA, admin.PublicKey(), config.LombardVerifierEvmRemoteChainConfigPDA,
			)
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)

			var rc lombard_token_pool.VerifierRemoteChainConfig
			require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, config.LombardVerifierEvmRemoteChainConfigPDA, config.DefaultCommitment, &rc))
			require.Equal(t, lombard_token_pool.LombardVerifierPath{}, rc.Path)
		})

		t.Run("remove_verifier_path fails a second time (already cleared)", func(t *testing.T) {
			ix, err := lombard_token_pool.NewRemoveVerifierPathInstruction(
				remoteChainSelector, config.LombardVerifierConfigPDA, admin.PublicKey(), config.LombardVerifierEvmRemoteChainConfigPDA,
			)
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment,
				[]string{"Error Code: " + "PathNotConfigured"})
		})
	})

	// Ports LombardVerifier.updateSupportedTokens.t.sol.
	t.Run("set_verifier_supported_tokens", func(t *testing.T) {
		tokenA := solana.MustPublicKeyFromBase58("So11111111111111111111111111111111111111112")
		tokenB := solana.MustPublicKeyFromBase58("EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v")

		t.Run("fails when called by non-owner", func(t *testing.T) {
			ix, err := lombard_token_pool.NewSetVerifierSupportedTokensInstruction(
				nil, []lombard_token_pool.SupportedToken{{LocalToken: tokenA}},
				config.LombardVerifierConfigPDA, user.PublicKey(), solana.SystemProgramID,
			)
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ix}, user, config.DefaultCommitment,
				[]string{"Error Code: " + "Unauthorized"})
		})

		t.Run("add succeeds", func(t *testing.T) {
			ix, err := lombard_token_pool.NewSetVerifierSupportedTokensInstruction(
				nil, []lombard_token_pool.SupportedToken{{LocalToken: tokenA}, {LocalToken: tokenB}},
				config.LombardVerifierConfigPDA, admin.PublicKey(), solana.SystemProgramID,
			)
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)

			var cfg lombard_token_pool.VerifierConfig
			require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, config.LombardVerifierConfigPDA, config.DefaultCommitment, &cfg))
			require.ElementsMatch(t, []lombard_token_pool.SupportedToken{{LocalToken: tokenA}, {LocalToken: tokenB}}, cfg.SupportedTokens)
		})

		t.Run("remove succeeds", func(t *testing.T) {
			ix, err := lombard_token_pool.NewSetVerifierSupportedTokensInstruction(
				[]solana.PublicKey{tokenA}, nil,
				config.LombardVerifierConfigPDA, admin.PublicKey(), solana.SystemProgramID,
			)
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)

			var cfg lombard_token_pool.VerifierConfig
			require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, config.LombardVerifierConfigPDA, config.DefaultCommitment, &cfg))
			require.Equal(t, []lombard_token_pool.SupportedToken{{LocalToken: tokenB}}, cfg.SupportedTokens)
		})
	})
}
