// committee_verifier_config_test.go tests the configuration and setup of the Committee Verifier program.
// This includes initialization, ownership transfers, code version management, allowlist admin settings,
// and storage location admin configuration.
package contracts

import (
	"testing"
	"unicode/utf8"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/committee_verifier"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/config"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/testutils"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/testutils/eth"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/ccip"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/tokens"
)

func TestCommitteeVerifierConfig(t *testing.T) {
	ctx := t.Context()

	admin := solana.MustPrivateKeyFromBase58("5dYf8bzhFbmwTNyq8vAoeqe19vdjCbSEKkPrN6DQoJB2RwjyuVapCmU46pEBvbw7aeg5wBncTc8HQJnKZy2LYsmF")
	user := solana.MustPrivateKeyFromBase58("ZZdVf32Npuhci4u4ir2NW9491Y3FTv2Gwk41HMpvgJoh81UM42LcNqAN8SXapHfPcr61QP7sJj7K2mKHt7qFCoV")
	newAdmin := solana.MustPrivateKeyFromBase58("5Biqymxxk6joWPiWdEdToM8nNMx7jPr1Sv7qxdyVnXERa35jqjrTtG9kfDkMDxuD25Rob4WiLpauAgHehnPBWxkj")

	solanaGoClient := testutils.DeployAllPrograms(t, testutils.PathToAnchorConfig, admin)

	t.Run("setup", func(t *testing.T) {
		t.Run("funding", func(t *testing.T) {
			require.NoError(t, testutils.FundAccounts(ctx, []solana.PrivateKey{admin, user, newAdmin}, solanaGoClient))
		})

		t.Run("initialize", func(t *testing.T) {
			cvProgramDataAddress := testutils.GetProgramDataAddress(ctx, t, solanaGoClient, config.CommitteeVerifierProgram)

			ix, err := committee_verifier.NewInitializeInstruction(
				config.CVConfigPDA,
				config.CVStorageLocationsPDA,
				admin.PublicKey(),
				solana.SystemProgramID,
				config.CommitteeVerifierProgram,
				cvProgramDataAddress,
			)
			require.NoError(t, err)

			result := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)
			require.NotNil(t, result)

			// Verify config state
			var cfg committee_verifier.Config
			err = common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, config.CVConfigPDA, config.DefaultCommitment, &cfg)
			require.NoError(t, err)
			require.Equal(t, uint8(1), cfg.Version)
			require.Equal(t, committee_verifier.CodeVersion_V1, cfg.DefaultCodeVersion)
			require.Equal(t, admin.PublicKey(), cfg.Owner)
			require.Equal(t, solana.PublicKey{}, cfg.ProposedOwner)
		})
	})

	t.Run("ownership", func(t *testing.T) {
		t.Run("transfer_ownership fails when called by non-owner", func(t *testing.T) {
			ix, err := committee_verifier.NewTransferOwnershipInstruction(
				newAdmin.PublicKey(),
				config.CVConfigPDA,
				user.PublicKey(),
				config.CVEventAuthorityPDA,
				config.CommitteeVerifierProgram,
			)
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ix}, user, config.DefaultCommitment,
				[]string{"Error Code: " + "Unauthorized"})
		})

		t.Run("transfer_ownership fails with default pubkey", func(t *testing.T) {
			resolved, err := ccip.ResolveTransferOwnershipAccounts(ctx, solanaGoClient, config.CommitteeVerifierProgram, solana.PublicKey{}, admin.PublicKey(), logger.Test(t))
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, admin, config.DefaultCommitment,
				[]string{"Error Code: " + "DefaultOwnerProposal"})
		})

		t.Run("transfer_ownership fails when proposing current owner", func(t *testing.T) {
			resolved, err := ccip.ResolveTransferOwnershipAccounts(ctx, solanaGoClient, config.CommitteeVerifierProgram, admin.PublicKey(), admin.PublicKey(), logger.Test(t))
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, admin, config.DefaultCommitment,
				[]string{"Error Code: " + "RedundantOwnerProposal"})
		})

		t.Run("transfer_ownership succeeds and emits event", func(t *testing.T) {
			resolved, err := ccip.ResolveTransferOwnershipAccounts(ctx, solanaGoClient, config.CommitteeVerifierProgram, newAdmin.PublicKey(), admin.PublicKey(), logger.Test(t))
			require.NoError(t, err)

			result := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, admin, config.DefaultCommitment)
			require.NotNil(t, result)

			tx, err := result.Transaction.GetTransaction()
			require.NoError(t, err)
			event, err := common.ExtractCPIEvent(tx, result.Meta, config.CommitteeVerifierProgram, committee_verifier.ParseEvent_OwnershipTransferRequested)
			require.NoError(t, err)
			require.Equal(t, admin.PublicKey(), event.From)
			require.Equal(t, newAdmin.PublicKey(), event.To)

			// Verify proposed_owner is set but owner hasn't changed
			var cfg committee_verifier.Config
			err = common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, config.CVConfigPDA, config.DefaultCommitment, &cfg)
			require.NoError(t, err)
			require.Equal(t, admin.PublicKey(), cfg.Owner)
			require.Equal(t, newAdmin.PublicKey(), cfg.ProposedOwner)
		})

		t.Run("accept_ownership fails when called by wrong signer", func(t *testing.T) {
			ix, err := committee_verifier.NewAcceptOwnershipInstruction(
				config.CVConfigPDA,
				user.PublicKey(),
				config.CVEventAuthorityPDA,
				config.CommitteeVerifierProgram,
			)
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ix}, user, config.DefaultCommitment,
				[]string{"Error Code: " + "Unauthorized"})
		})

		t.Run("accept_ownership succeeds and emits event", func(t *testing.T) {
			resolved, err := ccip.ResolveCVAcceptOwnershipAccounts(ctx, solanaGoClient, config.CommitteeVerifierProgram, newAdmin.PublicKey(), logger.Test(t))
			require.NoError(t, err)

			result := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, newAdmin, config.DefaultCommitment)
			require.NotNil(t, result)

			tx, err := result.Transaction.GetTransaction()
			require.NoError(t, err)
			event, err := common.ExtractCPIEvent(tx, result.Meta, config.CommitteeVerifierProgram, committee_verifier.ParseEvent_OwnershipTransferred)
			require.NoError(t, err)
			require.Equal(t, admin.PublicKey(), event.From)
			require.Equal(t, newAdmin.PublicKey(), event.To)

			// Verify ownership transferred and proposed_owner cleared
			var cfg committee_verifier.Config
			err = common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, config.CVConfigPDA, config.DefaultCommitment, &cfg)
			require.NoError(t, err)
			require.Equal(t, newAdmin.PublicKey(), cfg.Owner)
			require.Equal(t, solana.PublicKey{}, cfg.ProposedOwner)
		})

		// Transfer ownership back to admin for subsequent tests
		t.Run("transfer ownership back to admin", func(t *testing.T) {
			resolved, err := ccip.ResolveTransferOwnershipAccounts(ctx, solanaGoClient, config.CommitteeVerifierProgram, admin.PublicKey(), newAdmin.PublicKey(), logger.Test(t))
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, newAdmin, config.DefaultCommitment)

			resolved, err = ccip.ResolveCVAcceptOwnershipAccounts(ctx, solanaGoClient, config.CommitteeVerifierProgram, admin.PublicKey(), logger.Test(t))
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, admin, config.DefaultCommitment)
		})
	})

	t.Run("code_version", func(t *testing.T) {
		t.Run("set_default_code_version fails with Default variant", func(t *testing.T) {
			resolved, err := ccip.ResolveSetDefaultCodeVersionAccounts(ctx, solanaGoClient, config.CommitteeVerifierProgram, committee_verifier.CodeVersion_Default, admin.PublicKey(), logger.Test(t))
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, admin, config.DefaultCommitment,
				[]string{"Error Code: " + "InvalidCodeVersion"})
		})

		t.Run("set_default_code_version fails when called by non-owner", func(t *testing.T) {
			ix, err := committee_verifier.NewSetDefaultCodeVersionInstruction(
				committee_verifier.CodeVersion_V1,
				config.CVConfigPDA,
				user.PublicKey(),
				config.CVEventAuthorityPDA,
				config.CommitteeVerifierProgram,
			)
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ix}, user, config.DefaultCommitment,
				[]string{"Error Code: " + "Unauthorized"})
		})

		t.Run("set_default_code_version succeeds with V1", func(t *testing.T) {
			resolved, err := ccip.ResolveSetDefaultCodeVersionAccounts(ctx, solanaGoClient, config.CommitteeVerifierProgram, committee_verifier.CodeVersion_V1, admin.PublicKey(), logger.Test(t))
			require.NoError(t, err)

			result := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, admin, config.DefaultCommitment)
			require.NotNil(t, result)

			tx, err := result.Transaction.GetTransaction()
			require.NoError(t, err)
			event, err := common.ExtractCPIEvent(tx, result.Meta, config.CommitteeVerifierProgram, committee_verifier.ParseEvent_DefaultCodeVersionSet)
			require.NoError(t, err)
			require.Equal(t, committee_verifier.CodeVersion_V1, event.Old)
			require.Equal(t, committee_verifier.CodeVersion_V1, event.New)
		})
	})

	t.Run("allowlist_admin", func(t *testing.T) {
		t.Run("set_allowlist_admin fails when called by non-owner", func(t *testing.T) {
			ix, err := committee_verifier.NewSetAllowlistAdminInstruction(
				newAdmin.PublicKey(),
				config.CVConfigPDA,
				user.PublicKey(),
				config.CVEventAuthorityPDA,
				config.CommitteeVerifierProgram,
			)
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ix}, user, config.DefaultCommitment,
				[]string{"Error Code: " + "Unauthorized"})
		})

		t.Run("set_allowlist_admin succeeds and emits event", func(t *testing.T) {
			resolved, err := ccip.ResolveCCVSetAllowlistAdminAccounts(ctx, solanaGoClient, config.CommitteeVerifierProgram, newAdmin.PublicKey(), admin.PublicKey(), logger.Test(t))
			require.NoError(t, err)

			result := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, admin, config.DefaultCommitment)
			require.NotNil(t, result)

			tx, err := result.Transaction.GetTransaction()
			require.NoError(t, err)
			event, err := common.ExtractCPIEvent(tx, result.Meta, config.CommitteeVerifierProgram, committee_verifier.ParseEvent_AllowlistAdminSet)
			require.NoError(t, err)
			require.Equal(t, admin.PublicKey(), event.Old)
			require.Equal(t, newAdmin.PublicKey(), event.New)

			// Verify config state
			var cfg committee_verifier.Config
			err = common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, config.CVConfigPDA, config.DefaultCommitment, &cfg)
			require.NoError(t, err)
			require.Equal(t, newAdmin.PublicKey(), cfg.AllowlistAdmin)
		})

		t.Run("set_allowlist_admin back to admin", func(t *testing.T) {
			resolved, err := ccip.ResolveCCVSetAllowlistAdminAccounts(ctx, solanaGoClient, config.CommitteeVerifierProgram, admin.PublicKey(), admin.PublicKey(), logger.Test(t))
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, admin, config.DefaultCommitment)
		})
	})

	t.Run("storage_location_admin", func(t *testing.T) {
		t.Run("transfer fails when called by non-storage-location-admin", func(t *testing.T) {
			ix, err := committee_verifier.NewTransferStorageLocationAdminInstruction(
				newAdmin.PublicKey(),
				config.CVConfigPDA,
				user.PublicKey(),
				config.CVEventAuthorityPDA,
				config.CommitteeVerifierProgram,
			)
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ix}, user, config.DefaultCommitment,
				[]string{"Error Code: " + "Unauthorized"})
		})

		t.Run("transfer fails with default pubkey", func(t *testing.T) {
			resolved, err := ccip.ResolveCVTransferStorageLocationAdminAccounts(ctx, solanaGoClient, config.CommitteeVerifierProgram, solana.PublicKey{}, admin.PublicKey(), logger.Test(t))
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, admin, config.DefaultCommitment,
				[]string{"Error Code: " + "DefaultStorageLocationAdminProposal"})
		})

		t.Run("transfer fails when proposing current storage location admin", func(t *testing.T) {
			resolved, err := ccip.ResolveCVTransferStorageLocationAdminAccounts(ctx, solanaGoClient, config.CommitteeVerifierProgram, admin.PublicKey(), admin.PublicKey(), logger.Test(t))
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, admin, config.DefaultCommitment,
				[]string{"Error Code: " + "RedundantStorageLocationAdminProposal"})
		})

		t.Run("transfer succeeds and emits event", func(t *testing.T) {
			resolved, err := ccip.ResolveCVTransferStorageLocationAdminAccounts(ctx, solanaGoClient, config.CommitteeVerifierProgram, newAdmin.PublicKey(), admin.PublicKey(), logger.Test(t))
			require.NoError(t, err)

			result := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, admin, config.DefaultCommitment)
			require.NotNil(t, result)

			tx, err := result.Transaction.GetTransaction()
			require.NoError(t, err)
			event, err := common.ExtractCPIEvent(tx, result.Meta, config.CommitteeVerifierProgram, committee_verifier.ParseEvent_StorageLocationAdminTransferRequested)
			require.NoError(t, err)
			require.Equal(t, admin.PublicKey(), event.From)
			require.Equal(t, newAdmin.PublicKey(), event.To)

			// Verify proposed is set but current admin hasn't changed
			var cfg committee_verifier.Config
			err = common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, config.CVConfigPDA, config.DefaultCommitment, &cfg)
			require.NoError(t, err)
			require.Equal(t, admin.PublicKey(), cfg.StorageLocationAdmin)
			require.Equal(t, newAdmin.PublicKey(), cfg.ProposedStorageLocationAdmin)
		})

		t.Run("accept fails when called by wrong signer", func(t *testing.T) {
			ix, err := committee_verifier.NewAcceptStorageLocationAdminInstruction(
				config.CVConfigPDA,
				user.PublicKey(),
				config.CVEventAuthorityPDA,
				config.CommitteeVerifierProgram,
			)
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ix}, user, config.DefaultCommitment,
				[]string{"Error Code: " + "Unauthorized"})
		})

		t.Run("accept succeeds and emits event", func(t *testing.T) {
			resolved, err := ccip.ResolveCVAcceptStorageLocationAdminAccounts(ctx, solanaGoClient, config.CommitteeVerifierProgram, newAdmin.PublicKey(), logger.Test(t))
			require.NoError(t, err)

			result := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, newAdmin, config.DefaultCommitment)
			require.NotNil(t, result)

			tx, err := result.Transaction.GetTransaction()
			require.NoError(t, err)
			event, err := common.ExtractCPIEvent(tx, result.Meta, config.CommitteeVerifierProgram, committee_verifier.ParseEvent_StorageLocationAdminTransferred)
			require.NoError(t, err)
			require.Equal(t, admin.PublicKey(), event.From)
			require.Equal(t, newAdmin.PublicKey(), event.To)

			// Verify admin transferred and proposed cleared
			var cfg committee_verifier.Config
			err = common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, config.CVConfigPDA, config.DefaultCommitment, &cfg)
			require.NoError(t, err)
			require.Equal(t, newAdmin.PublicKey(), cfg.StorageLocationAdmin)
			require.Equal(t, solana.PublicKey{}, cfg.ProposedStorageLocationAdmin)
		})

		// Transfer back to admin so subsequent tests are unaffected
		t.Run("transfer storage location admin back", func(t *testing.T) {
			resolved, err := ccip.ResolveCVTransferStorageLocationAdminAccounts(ctx, solanaGoClient, config.CommitteeVerifierProgram, admin.PublicKey(), newAdmin.PublicKey(), logger.Test(t))
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, newAdmin, config.DefaultCommitment)

			resolved, err = ccip.ResolveCVAcceptStorageLocationAdminAccounts(ctx, solanaGoClient, config.CommitteeVerifierProgram, admin.PublicKey(), logger.Test(t))
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, admin, config.DefaultCommitment)
		})

		t.Run("owner cannot transfer storage location admin", func(t *testing.T) {
			// After transferring back, admin is both owner and storage_location_admin.
			// Transfer ownership to newAdmin, then verify newAdmin (owner) can't transfer storage location admin.
			resolved, err := ccip.ResolveTransferOwnershipAccounts(ctx, solanaGoClient, config.CommitteeVerifierProgram, newAdmin.PublicKey(), admin.PublicKey(), logger.Test(t))
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, admin, config.DefaultCommitment)

			resolved, err = ccip.ResolveCVAcceptOwnershipAccounts(ctx, solanaGoClient, config.CommitteeVerifierProgram, newAdmin.PublicKey(), logger.Test(t))
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, newAdmin, config.DefaultCommitment)

			// Now newAdmin is owner, admin is storage_location_admin.
			// newAdmin (owner but not storage_location_admin) should fail.
			ix, err := committee_verifier.NewTransferStorageLocationAdminInstruction(
				user.PublicKey(),
				config.CVConfigPDA,
				newAdmin.PublicKey(),
				config.CVEventAuthorityPDA,
				config.CommitteeVerifierProgram,
			)
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ix}, newAdmin, config.DefaultCommitment,
				[]string{"Error Code: " + "Unauthorized"})

			// Transfer ownership back to admin for subsequent tests
			resolved, err = ccip.ResolveTransferOwnershipAccounts(ctx, solanaGoClient, config.CommitteeVerifierProgram, admin.PublicKey(), newAdmin.PublicKey(), logger.Test(t))
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, newAdmin, config.DefaultCommitment)

			resolved, err = ccip.ResolveCVAcceptOwnershipAccounts(ctx, solanaGoClient, config.CommitteeVerifierProgram, admin.PublicKey(), logger.Test(t))
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, admin, config.DefaultCommitment)
		})
	})

	// Locations are held on-chain as opaque bytes (ByteString) rather than String, and the account is
	// realloc'd to fit them, so exercise the round trip: write, read back, and grow/shrink the set.
	// `admin` is both owner and storage_location_admin again by this point.
	t.Run("storage_locations", func(t *testing.T) {
		setLocations := func(t *testing.T, locations []string) {
			t.Helper()
			params := committee_verifier.SetStorageLocationsParams{
				StorageLocations: make([]committee_verifier.ByteString, len(locations)),
			}
			for i, location := range locations {
				params.StorageLocations[i] = committee_verifier.ByteString{V0: []byte(location)}
			}
			resolved, err := ccip.ResolveCVSetStorageLocationsAccounts(ctx, solanaGoClient, config.CommitteeVerifierProgram, params, admin.PublicKey(), logger.Test(t))
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, admin, config.DefaultCommitment)
		}

		decode := func(locations []committee_verifier.ByteString) []string {
			decoded := make([]string, len(locations))
			for i, location := range locations {
				decoded[i] = string(location.V0)
			}
			return decoded
		}

		// Reads the account directly. The realloc sizing lives in the set instruction's constraints,
		// so what is actually persisted is the thing worth asserting on.
		getStoredLocations := func(t *testing.T) []string {
			t.Helper()
			var stored committee_verifier.StorageLocations
			require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, config.CVStorageLocationsPDA, config.DefaultCommitment, &stored))
			return decode(stored.Locations)
		}

		// Exercises the getter instruction too, since its return type changed alongside the account.
		getReturnedLocations := func(t *testing.T) []string {
			t.Helper()
			resolved, err := ccip.ResolveCVGetStorageLocationsAccounts(ctx, solanaGoClient, config.CommitteeVerifierProgram, admin.PublicKey(), logger.Test(t))
			require.NoError(t, err)
			result := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, admin, config.DefaultCommitment)

			returned, err := common.ExtractTypedReturnValue(ctx, result.Meta.LogMessages, config.CommitteeVerifierProgram.String(), func(bts []byte) []committee_verifier.ByteString {
				var locations []committee_verifier.ByteString
				require.NoError(t, bin.NewBorshDecoder(bts).Decode(&locations))
				return locations
			})
			require.NoError(t, err)
			return decode(returned)
		}

		t.Run("round trips through the account", func(t *testing.T) {
			// The second entry is deliberately not valid UTF-8 (0xff/0xfe are never legal UTF-8
			// bytes). That is what actually exercises the ByteString change: Borsh String
			// deserialization validates UTF-8, so the old Vec<String> field would have rejected this
			// value on every account load. Valid-but-non-ASCII text would not have shown any
			// difference.
			want := []string{"postgres://default-aggregator-db:5432/aggregator", string([]byte{0xff, 0xfe, 'o', 'k'})}
			require.False(t, utf8.ValidString(want[1]), "the second entry must be invalid UTF-8 for this test to mean anything")

			setLocations(t, want)
			require.Equal(t, want, getStoredLocations(t))
			require.Equal(t, want, getReturnedLocations(t))
		})

		t.Run("grows and shrinks the account", func(t *testing.T) {
			// Each set_storage_locations reallocs to exactly INIT_SPACE + dynamic_space, so growing
			// past the previous size and then shrinking both have to work.
			grown := []string{"one", "two-is-considerably-longer-than-one", "three"}
			setLocations(t, grown)
			require.Equal(t, grown, getStoredLocations(t))

			setLocations(t, []string{"just-one"})
			require.Equal(t, []string{"just-one"}, getStoredLocations(t))

			setLocations(t, nil)
			require.Empty(t, getStoredLocations(t))
		})
	})

	t.Run("verify", func(t *testing.T) {
		verifyChainSelector := uint64(77777)

		// Generate 5 EVM keypairs, sorted by address
		privateKeys, err := eth.GenerateEthPrivateKeys(5)
		require.NoError(t, err)
		signers, err := eth.GetEvmSigners(privateKeys)
		require.NoError(t, err)

		signerAddresses := make([][20]uint8, len(signers))
		for i, s := range signers {
			signerAddresses[i] = s.Address
		}

		encodedMessage := testutils.EncodeMinimalMessageV1(verifyChainSelector)
		messageID := [32]uint8(eth.Keccak256(encodedMessage))

		// `verify` is normally invoked via CPI, so its account resolution is nested within the
		// offramp's resolver. To call it directly, its two accounts are derived by hand instead.
		remoteChainConfigPDA, _, err := state.FindCVRemoteChainConfigPDA(verifyChainSelector, config.CommitteeVerifierProgram)
		require.NoError(t, err)

		t.Run("setup chain config and signers", func(t *testing.T) {
			// Create RemoteChainConfig PDA for verify tests
			resolved, err := ccip.ResolveCVInitChainConfigAccounts(ctx, solanaGoClient, config.CommitteeVerifierProgram, committee_verifier.SetChainConfigParams{
				RemoteChainSelector: verifyChainSelector,
				ChainConfig: committee_verifier.RemoteChainAdminConfig{
					RouterSigner:       admin.PublicKey(),
					Fee:                committee_verifier.UsdCents{V0: 0},
					GasForVerification: committee_verifier.CrossChainGas{V0: 0},
					PayloadSizeBytes:   0,
				},
			}, admin.PublicKey(), logger.Test(t))
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, admin, config.DefaultCommitment)

			// Configure signers: 5 signers, threshold=3
			resolved, err = ccip.ResolveCVSetSignerConfigAccounts(ctx, solanaGoClient, config.CommitteeVerifierProgram, committee_verifier.SetSignerConfigParams{
				SourceChainSelector: verifyChainSelector,
				Threshold:           3,
				Signers:             signerAddresses,
			}, admin.PublicKey(), logger.Test(t))
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, admin, config.DefaultCommitment)

			pda, _, err := state.FindCVRemoteChainConfigPDA(verifyChainSelector, config.CommitteeVerifierProgram)
			require.NoError(t, err)

			var chainConfig committee_verifier.RemoteChainConfig
			err = common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, pda, config.DefaultCommitment, &chainConfig)
			require.NoError(t, err)
			require.Equal(t, uint8(1), chainConfig.Version)
			require.Equal(t, uint8(3), chainConfig.SignerConfig.Threshold)
			require.Equal(t, signerAddresses, chainConfig.SignerConfig.Signers)
		})

		t.Run("succeeds with exactly threshold signatures", func(t *testing.T) {
			verifierResults, err := testutils.SignCommitteeVerifierMessage(messageID, signers, 3)
			require.NoError(t, err)

			ix, err := committee_verifier.NewVerifyInstruction(committee_verifier.VerifyParams{
				MessageId:       messageID,
				EncodedMessage:  encodedMessage,
				VerifierResults: verifierResults,
			}, config.CVConfigPDA, remoteChainConfigPDA)
			require.NoError(t, err)

			result := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)
			require.NotNil(t, result)
		})

		t.Run("succeeds with more than threshold signatures", func(t *testing.T) {
			verifierResults, err := testutils.SignCommitteeVerifierMessage(messageID, signers, 4)
			require.NoError(t, err)

			ix, err := committee_verifier.NewVerifyInstruction(committee_verifier.VerifyParams{
				MessageId:       messageID,
				EncodedMessage:  encodedMessage,
				VerifierResults: verifierResults,
			}, config.CVConfigPDA, remoteChainConfigPDA)
			require.NoError(t, err)

			result := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)
			require.NotNil(t, result)
		})

		t.Run("fails with fewer signatures than threshold", func(t *testing.T) {
			verifierResults, err := testutils.SignCommitteeVerifierMessage(messageID, signers, 2)
			require.NoError(t, err)

			ix, err := committee_verifier.NewVerifyInstruction(committee_verifier.VerifyParams{
				MessageId:       messageID,
				EncodedMessage:  encodedMessage,
				VerifierResults: verifierResults,
			}, config.CVConfigPDA, remoteChainConfigPDA)
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment,
				[]string{"Error Code: " + "InsufficientSignatures"})
		})
	})

	t.Run("set_chain_config", func(t *testing.T) {
		remoteChainSelector := uint64(12345)

		t.Run("creates and populates a new remote chain config", func(t *testing.T) {
			resolved, err := ccip.ResolveCVInitChainConfigAccounts(ctx, solanaGoClient, config.CommitteeVerifierProgram, committee_verifier.SetChainConfigParams{
				RemoteChainSelector: remoteChainSelector,
				ChainConfig: committee_verifier.RemoteChainAdminConfig{
					RouterSigner:       newAdmin.PublicKey(),
					Fee:                committee_verifier.UsdCents{V0: 100},
					GasForVerification: committee_verifier.CrossChainGas{V0: 5000},
					PayloadSizeBytes:   1024,
				},
			}, admin.PublicKey(), logger.Test(t))
			require.NoError(t, err)

			result := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, admin, config.DefaultCommitment)
			require.NotNil(t, result)

			remoteChainConfigPDA := resolved.AccountsToSave[2].PublicKey
			var cfg committee_verifier.RemoteChainConfig
			err = common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, remoteChainConfigPDA, config.DefaultCommitment, &cfg)
			require.NoError(t, err)
			require.Equal(t, newAdmin.PublicKey(), cfg.AdminConfig.RouterSigner)
			require.Equal(t, uint32(100), cfg.AdminConfig.Fee.V0)
			require.Equal(t, uint32(5000), cfg.AdminConfig.GasForVerification.V0)
			require.Equal(t, uint32(1024), cfg.AdminConfig.PayloadSizeBytes)
			require.False(t, cfg.AllowlistConfig.AllowlistEnabled)
			require.Empty(t, cfg.AllowlistConfig.AllowedSenders)
		})

		t.Run("init fails for an already configured remote chain", func(t *testing.T) {
			resolved, err := ccip.ResolveCVInitChainConfigAccounts(ctx, solanaGoClient, config.CommitteeVerifierProgram, committee_verifier.SetChainConfigParams{
				RemoteChainSelector: remoteChainSelector,
				ChainConfig: committee_verifier.RemoteChainAdminConfig{
					RouterSigner: newAdmin.PublicKey(),
				},
			}, admin.PublicKey(), logger.Test(t))
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, admin, config.DefaultCommitment,
				[]string{"already in use"})
		})

		t.Run("edit fails for an unconfigured remote chain", func(t *testing.T) {
			resolved, err := ccip.ResolveCVEditChainConfigAccounts(ctx, solanaGoClient, config.CommitteeVerifierProgram, committee_verifier.SetChainConfigParams{
				RemoteChainSelector: 424242,
				ChainConfig: committee_verifier.RemoteChainAdminConfig{
					RouterSigner: newAdmin.PublicKey(),
				},
			}, admin.PublicKey(), logger.Test(t))
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, admin, config.DefaultCommitment,
				[]string{"Error Code: " + "AccountNotInitialized"})
		})

		t.Run("overwrites an existing remote chain config", func(t *testing.T) {
			resolved, err := ccip.ResolveCVEditChainConfigAccounts(ctx, solanaGoClient, config.CommitteeVerifierProgram, committee_verifier.SetChainConfigParams{
				RemoteChainSelector: remoteChainSelector,
				ChainConfig: committee_verifier.RemoteChainAdminConfig{
					RouterSigner:       user.PublicKey(),
					Fee:                committee_verifier.UsdCents{V0: 200},
					GasForVerification: committee_verifier.CrossChainGas{V0: 9000},
					PayloadSizeBytes:   2048,
				},
			}, admin.PublicKey(), logger.Test(t))
			require.NoError(t, err)

			result := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, admin, config.DefaultCommitment)
			require.NotNil(t, result)

			remoteChainConfigPDA := resolved.AccountsToSave[2].PublicKey
			var cfg committee_verifier.RemoteChainConfig
			err = common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, remoteChainConfigPDA, config.DefaultCommitment, &cfg)
			require.NoError(t, err)
			require.Equal(t, user.PublicKey(), cfg.AdminConfig.RouterSigner)
			require.Equal(t, uint32(200), cfg.AdminConfig.Fee.V0)
			require.Equal(t, uint32(9000), cfg.AdminConfig.GasForVerification.V0)
			require.Equal(t, uint32(2048), cfg.AdminConfig.PayloadSizeBytes)
		})

		t.Run("fails when called by non-owner", func(t *testing.T) {
			remoteChainConfigPDA, _, err := state.FindCVRemoteChainConfigPDA(remoteChainSelector, config.CommitteeVerifierProgram)
			require.NoError(t, err)

			ix, err := committee_verifier.NewEditChainConfigInstruction(
				committee_verifier.SetChainConfigParams{
					RemoteChainSelector: remoteChainSelector,
					ChainConfig:         committee_verifier.RemoteChainAdminConfig{},
				},
				config.CVConfigPDA,
				user.PublicKey(),
				remoteChainConfigPDA,
				config.CVEventAuthorityPDA,
				config.CommitteeVerifierProgram,
			)
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ix}, user, config.DefaultCommitment,
				[]string{"Error Code: " + "Unauthorized"})
		})
	})

	t.Run("allowlist", func(t *testing.T) {
		sender1 := solana.NewWallet().PrivateKey.PublicKey()
		sender2 := solana.NewWallet().PrivateKey.PublicKey()
		sender3 := solana.NewWallet().PrivateKey.PublicKey()

		remoteChainPDA := func(t *testing.T, chainSelector uint64) solana.PublicKey {
			pda, _, err := state.FindCVRemoteChainConfigPDA(chainSelector, config.CommitteeVerifierProgram)
			require.NoError(t, err)
			return pda
		}

		createChainConfig := func(t *testing.T, chainSelector uint64) {
			resolved, err := ccip.ResolveCVInitChainConfigAccounts(ctx, solanaGoClient, config.CommitteeVerifierProgram, committee_verifier.SetChainConfigParams{
				RemoteChainSelector: chainSelector,
				ChainConfig: committee_verifier.RemoteChainAdminConfig{
					RouterSigner:       admin.PublicKey(),
					Fee:                committee_verifier.UsdCents{V0: 100},
					GasForVerification: committee_verifier.CrossChainGas{V0: 5000},
					PayloadSizeBytes:   1024,
				},
			}, admin.PublicKey(), logger.Test(t))
			require.NoError(t, err)
			result := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, admin, config.DefaultCommitment)
			require.NotNil(t, result)
		}

		createChainConfig(t, config.EvmChainSelector)

		t.Run("set_allowlist succeeds with senders and enabled", func(t *testing.T) {
			resolved, err := ccip.ResolveCCVSetAllowlistAccounts(ctx, solanaGoClient, config.CommitteeVerifierProgram, committee_verifier.SetAllowlistParams{
				DestChainSelector: config.EvmChainSelector,
				Enabled:           true,
				AllowedSenders:    []solana.PublicKey{sender1, sender2},
			}, admin.PublicKey(), logger.Test(t))
			require.NoError(t, err)

			result := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, admin, config.DefaultCommitment)
			require.NotNil(t, result)

			tx, err := result.Transaction.GetTransaction()
			require.NoError(t, err)
			event, err := common.ExtractCPIEvent(tx, result.Meta, config.CommitteeVerifierProgram, committee_verifier.ParseEvent_AllowlistSet)
			require.NoError(t, err)
			require.Equal(t, config.EvmChainSelector, event.DestChainSelector)
			require.True(t, event.Enabled)
			require.Equal(t, []solana.PublicKey{sender1, sender2}, event.AllowedSenders)

			var cfg committee_verifier.RemoteChainConfig
			err = common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, remoteChainPDA(t, config.EvmChainSelector), config.DefaultCommitment, &cfg)
			require.NoError(t, err)
			require.True(t, cfg.AllowlistConfig.AllowlistEnabled)
			require.Equal(t, []solana.PublicKey{sender1, sender2}, cfg.AllowlistConfig.AllowedSenders)
		})

		t.Run("set_allowlist overwrites previous state", func(t *testing.T) {
			resolved, err := ccip.ResolveCCVSetAllowlistAccounts(ctx, solanaGoClient, config.CommitteeVerifierProgram, committee_verifier.SetAllowlistParams{
				DestChainSelector: config.EvmChainSelector,
				Enabled:           false,
				AllowedSenders:    []solana.PublicKey{sender3},
			}, admin.PublicKey(), logger.Test(t))
			require.NoError(t, err)

			result := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, admin, config.DefaultCommitment)
			require.NotNil(t, result)

			var cfg committee_verifier.RemoteChainConfig
			err = common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, remoteChainPDA(t, config.EvmChainSelector), config.DefaultCommitment, &cfg)
			require.NoError(t, err)
			require.False(t, cfg.AllowlistConfig.AllowlistEnabled)
			require.Equal(t, []solana.PublicKey{sender3}, cfg.AllowlistConfig.AllowedSenders)
		})

		t.Run("set_allowlist with empty list", func(t *testing.T) {
			resolved, err := ccip.ResolveCCVSetAllowlistAccounts(ctx, solanaGoClient, config.CommitteeVerifierProgram, committee_verifier.SetAllowlistParams{
				DestChainSelector: config.EvmChainSelector,
				Enabled:           false,
				AllowedSenders:    []solana.PublicKey{},
			}, admin.PublicKey(), logger.Test(t))
			require.NoError(t, err)

			result := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, admin, config.DefaultCommitment)
			require.NotNil(t, result)

			var cfg committee_verifier.RemoteChainConfig
			err = common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, remoteChainPDA(t, config.EvmChainSelector), config.DefaultCommitment, &cfg)
			require.NoError(t, err)
			require.False(t, cfg.AllowlistConfig.AllowlistEnabled)
			require.Empty(t, cfg.AllowlistConfig.AllowedSenders)
		})

		t.Run("set_allowlist fails when called by non-allowlist-admin", func(t *testing.T) {
			evmPDA := remoteChainPDA(t, config.EvmChainSelector)
			ix, err := committee_verifier.NewSetAllowlistInstruction(
				committee_verifier.SetAllowlistParams{
					DestChainSelector: config.EvmChainSelector,
					Enabled:           true,
					AllowedSenders:    []solana.PublicKey{sender1},
				},
				config.CVConfigPDA,
				user.PublicKey(),
				evmPDA,
				solana.SystemProgramID,
				config.CVEventAuthorityPDA,
				config.CommitteeVerifierProgram,
			)
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ix}, user, config.DefaultCommitment,
				[]string{"Error Code: " + "Unauthorized"})
		})

		t.Run("set_allowlist succeeds when called by the owner or the allowlist admin", func(t *testing.T) {
			setAdmin := func(t *testing.T, allowlistAdmin solana.PublicKey) {
				resolved, err := ccip.ResolveCCVSetAllowlistAdminAccounts(ctx, solanaGoClient, config.CommitteeVerifierProgram, allowlistAdmin, admin.PublicKey(), logger.Test(t))
				require.NoError(t, err)
				testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, admin, config.DefaultCommitment)
			}
			setAllowlist := func(t *testing.T, signer solana.PrivateKey, senders []solana.PublicKey) {
				resolved, err := ccip.ResolveCCVSetAllowlistAccounts(ctx, solanaGoClient, config.CommitteeVerifierProgram, committee_verifier.SetAllowlistParams{
					DestChainSelector: config.EvmChainSelector,
					Enabled:           true,
					AllowedSenders:    senders,
				}, signer.PublicKey(), logger.Test(t))
				require.NoError(t, err)
				testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, signer, config.DefaultCommitment)
				var cfg committee_verifier.RemoteChainConfig
				require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, remoteChainPDA(t, config.EvmChainSelector), config.DefaultCommitment, &cfg))
				require.Equal(t, senders, cfg.AllowlistConfig.AllowedSenders)
			}

			// Give the allowlist admin role to another key, so the owner and the admin differ.
			setAdmin(t, newAdmin.PublicKey())
			defer setAdmin(t, admin.PublicKey())

			setAllowlist(t, newAdmin, []solana.PublicKey{sender1})
			setAllowlist(t, admin, []solana.PublicKey{sender2})
		})

		t.Run("per-chain isolation", func(t *testing.T) {
			// Create chain config for SVM chain (EVM already created above)
			createChainConfig(t, config.SvmChainSelector)

			// Set allowlist for SVM chain
			resolved, err := ccip.ResolveCCVSetAllowlistAccounts(ctx, solanaGoClient, config.CommitteeVerifierProgram, committee_verifier.SetAllowlistParams{
				DestChainSelector: config.SvmChainSelector,
				Enabled:           true,
				AllowedSenders:    []solana.PublicKey{sender2, sender3},
			}, admin.PublicKey(), logger.Test(t))
			require.NoError(t, err)
			result := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, admin, config.DefaultCommitment)
			require.NotNil(t, result)

			// Set EVM chain to a different state
			resolved2, err := ccip.ResolveCCVSetAllowlistAccounts(ctx, solanaGoClient, config.CommitteeVerifierProgram, committee_verifier.SetAllowlistParams{
				DestChainSelector: config.EvmChainSelector,
				Enabled:           true,
				AllowedSenders:    []solana.PublicKey{sender1},
			}, admin.PublicKey(), logger.Test(t))
			require.NoError(t, err)
			result = testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{resolved2.Ix}, admin, config.DefaultCommitment)
			require.NotNil(t, result)

			// Verify EVM chain allowlist
			var evmCfg committee_verifier.RemoteChainConfig
			err = common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, remoteChainPDA(t, config.EvmChainSelector), config.DefaultCommitment, &evmCfg)
			require.NoError(t, err)
			require.True(t, evmCfg.AllowlistConfig.AllowlistEnabled)
			require.Equal(t, []solana.PublicKey{sender1}, evmCfg.AllowlistConfig.AllowedSenders)

			// Verify SVM chain allowlist is independent
			var svmCfg committee_verifier.RemoteChainConfig
			err = common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, remoteChainPDA(t, config.SvmChainSelector), config.DefaultCommitment, &svmCfg)
			require.NoError(t, err)
			require.True(t, svmCfg.AllowlistConfig.AllowlistEnabled)
			require.Equal(t, []solana.PublicKey{sender2, sender3}, svmCfg.AllowlistConfig.AllowedSenders)
		})
	})

	t.Run("signer_config", func(t *testing.T) {
		signerA := [20]uint8{0x01}
		signerB := [20]uint8{0x02}
		signerC := [20]uint8{0x03}
		zeroSigner := [20]uint8{}

		remoteChainPDA := func(t *testing.T, chainSelector uint64) solana.PublicKey {
			pda, _, err := state.FindCVRemoteChainConfigPDA(chainSelector, config.CommitteeVerifierProgram)
			require.NoError(t, err)
			return pda
		}

		// Use EvmChainSelector which already has a chain config from earlier tests
		chainSelector := config.EvmChainSelector

		t.Run("set_signer_config succeeds with valid signers", func(t *testing.T) {
			resolved, err := ccip.ResolveCVSetSignerConfigAccounts(ctx, solanaGoClient, config.CommitteeVerifierProgram, committee_verifier.SetSignerConfigParams{
				SourceChainSelector: chainSelector,
				Threshold:           2,
				Signers:             [][20]uint8{signerC, signerA, signerB},
			}, admin.PublicKey(), logger.Test(t))
			require.NoError(t, err)

			result := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, admin, config.DefaultCommitment)
			require.NotNil(t, result)

			// Verify event
			tx, err := result.Transaction.GetTransaction()
			require.NoError(t, err)
			event, err := common.ExtractCPIEvent(tx, result.Meta, config.CommitteeVerifierProgram, committee_verifier.ParseEvent_SignerConfigSet)
			require.NoError(t, err)
			require.Equal(t, chainSelector, event.SourceChainSelector)
			require.Equal(t, uint8(2), event.Threshold)

			// Verify on-chain state
			var cfg committee_verifier.RemoteChainConfig
			err = common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, remoteChainPDA(t, chainSelector), config.DefaultCommitment, &cfg)
			require.NoError(t, err)
			require.Equal(t, uint8(2), cfg.SignerConfig.Threshold)
			require.Len(t, cfg.SignerConfig.Signers, 3)

			// Verify signers are stored sorted regardless of input order
			require.Equal(t, signerA, cfg.SignerConfig.Signers[0])
			require.Equal(t, signerB, cfg.SignerConfig.Signers[1])
			require.Equal(t, signerC, cfg.SignerConfig.Signers[2])
		})

		t.Run("set_signer_config overwrites previous config", func(t *testing.T) {
			resolved, err := ccip.ResolveCVSetSignerConfigAccounts(ctx, solanaGoClient, config.CommitteeVerifierProgram, committee_verifier.SetSignerConfigParams{
				SourceChainSelector: chainSelector,
				Threshold:           1,
				Signers:             [][20]uint8{signerB},
			}, admin.PublicKey(), logger.Test(t))
			require.NoError(t, err)

			result := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, admin, config.DefaultCommitment)
			require.NotNil(t, result)

			var cfg committee_verifier.RemoteChainConfig
			err = common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, remoteChainPDA(t, chainSelector), config.DefaultCommitment, &cfg)
			require.NoError(t, err)
			require.Equal(t, uint8(1), cfg.SignerConfig.Threshold)
			require.Equal(t, [][20]uint8{signerB}, cfg.SignerConfig.Signers)
		})

		t.Run("set_signer_config fails with threshold 0", func(t *testing.T) {
			resolved, err := ccip.ResolveCVSetSignerConfigAccounts(ctx, solanaGoClient, config.CommitteeVerifierProgram, committee_verifier.SetSignerConfigParams{
				SourceChainSelector: chainSelector,
				Threshold:           0,
				Signers:             [][20]uint8{signerA},
			}, admin.PublicKey(), logger.Test(t))
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, admin, config.DefaultCommitment,
				[]string{"Error Code: " + "InvalidSignerThreshold"})
		})

		t.Run("set_signer_config fails with threshold > signers length", func(t *testing.T) {
			resolved, err := ccip.ResolveCVSetSignerConfigAccounts(ctx, solanaGoClient, config.CommitteeVerifierProgram, committee_verifier.SetSignerConfigParams{
				SourceChainSelector: chainSelector,
				Threshold:           3,
				Signers:             [][20]uint8{signerA, signerB},
			}, admin.PublicKey(), logger.Test(t))
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, admin, config.DefaultCommitment,
				[]string{"Error Code: " + "InvalidSignerThreshold"})
		})

		t.Run("set_signer_config fails with zero address signer", func(t *testing.T) {
			resolved, err := ccip.ResolveCVSetSignerConfigAccounts(ctx, solanaGoClient, config.CommitteeVerifierProgram, committee_verifier.SetSignerConfigParams{
				SourceChainSelector: chainSelector,
				Threshold:           1,
				Signers:             [][20]uint8{zeroSigner},
			}, admin.PublicKey(), logger.Test(t))
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, admin, config.DefaultCommitment,
				[]string{"Error Code: " + "ZeroAddressSigner"})
		})

		t.Run("set_signer_config fails with duplicate signers", func(t *testing.T) {
			resolved, err := ccip.ResolveCVSetSignerConfigAccounts(ctx, solanaGoClient, config.CommitteeVerifierProgram, committee_verifier.SetSignerConfigParams{
				SourceChainSelector: chainSelector,
				Threshold:           1,
				Signers:             [][20]uint8{signerA, signerA},
			}, admin.PublicKey(), logger.Test(t))
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, admin, config.DefaultCommitment,
				[]string{"Error Code: " + "DuplicateSigner"})
		})

		t.Run("set_signer_config fails when called by non-owner", func(t *testing.T) {
			pda := remoteChainPDA(t, chainSelector)
			ix, err := committee_verifier.NewSetSignerConfigInstruction(
				committee_verifier.SetSignerConfigParams{
					SourceChainSelector: chainSelector,
					Threshold:           1,
					Signers:             [][20]uint8{signerA},
				},
				config.CVConfigPDA,
				user.PublicKey(),
				pda,
				solana.SystemProgramID,
				config.CVEventAuthorityPDA,
				config.CommitteeVerifierProgram,
			)
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ix}, user, config.DefaultCommitment,
				[]string{"Error Code: " + "Unauthorized"})
		})

		t.Run("signer config preserves allowlist and vice versa", func(t *testing.T) {
			sender := solana.NewWallet().PrivateKey.PublicKey()

			// Set allowlist
			resolved, err := ccip.ResolveCCVSetAllowlistAccounts(ctx, solanaGoClient, config.CommitteeVerifierProgram, committee_verifier.SetAllowlistParams{
				DestChainSelector: chainSelector,
				Enabled:           true,
				AllowedSenders:    []solana.PublicKey{sender},
			}, admin.PublicKey(), logger.Test(t))
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, admin, config.DefaultCommitment)

			// Set signer config — should preserve allowlist
			resolved2, err := ccip.ResolveCVSetSignerConfigAccounts(ctx, solanaGoClient, config.CommitteeVerifierProgram, committee_verifier.SetSignerConfigParams{
				SourceChainSelector: chainSelector,
				Threshold:           2,
				Signers:             [][20]uint8{signerA, signerB, signerC},
			}, admin.PublicKey(), logger.Test(t))
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{resolved2.Ix}, admin, config.DefaultCommitment)

			// Verify both fields are preserved
			var cfg committee_verifier.RemoteChainConfig
			err = common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, remoteChainPDA(t, chainSelector), config.DefaultCommitment, &cfg)
			require.NoError(t, err)
			require.True(t, cfg.AllowlistConfig.AllowlistEnabled)
			require.Equal(t, []solana.PublicKey{sender}, cfg.AllowlistConfig.AllowedSenders)
			require.Equal(t, uint8(2), cfg.SignerConfig.Threshold)
			require.Len(t, cfg.SignerConfig.Signers, 3)

			// Now update allowlist to a different value — should preserve signer config
			sender2 := solana.NewWallet().PrivateKey.PublicKey()
			resolved3, err := ccip.ResolveCCVSetAllowlistAccounts(ctx, solanaGoClient, config.CommitteeVerifierProgram, committee_verifier.SetAllowlistParams{
				DestChainSelector: chainSelector,
				Enabled:           false,
				AllowedSenders:    []solana.PublicKey{sender2},
			}, admin.PublicKey(), logger.Test(t))
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{resolved3.Ix}, admin, config.DefaultCommitment)

			err = common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, remoteChainPDA(t, chainSelector), config.DefaultCommitment, &cfg)
			require.NoError(t, err)
			require.False(t, cfg.AllowlistConfig.AllowlistEnabled)
			require.Equal(t, []solana.PublicKey{sender2}, cfg.AllowlistConfig.AllowedSenders)
			require.Equal(t, uint8(2), cfg.SignerConfig.Threshold)
			require.Len(t, cfg.SignerConfig.Signers, 3)
		})

		// Regression: storing signers and allowed senders grows the account past its initial size,
		// which used to make every later chain config update fail with ConstraintSpace.
		t.Run("edit_chain_config succeeds and preserves signers and allowlist", func(t *testing.T) {
			var before committee_verifier.RemoteChainConfig
			err := common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, remoteChainPDA(t, chainSelector), config.DefaultCommitment, &before)
			require.NoError(t, err)
			require.NotEmpty(t, before.SignerConfig.Signers)
			require.NotEmpty(t, before.AllowlistConfig.AllowedSenders)

			adminConfig := before.AdminConfig
			adminConfig.Fee = committee_verifier.UsdCents{V0: before.AdminConfig.Fee.V0 + 5}
			resolved, err := ccip.ResolveCVEditChainConfigAccounts(ctx, solanaGoClient, config.CommitteeVerifierProgram, committee_verifier.SetChainConfigParams{
				RemoteChainSelector: chainSelector,
				ChainConfig:         adminConfig,
			}, admin.PublicKey(), logger.Test(t))
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, admin, config.DefaultCommitment)

			var after committee_verifier.RemoteChainConfig
			err = common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, remoteChainPDA(t, chainSelector), config.DefaultCommitment, &after)
			require.NoError(t, err)
			require.Equal(t, adminConfig, after.AdminConfig)
			require.Equal(t, before.SignerConfig, after.SignerConfig)
			require.Equal(t, before.AllowlistConfig, after.AllowlistConfig)
		})
	})

	t.Run("remove_signer_config", func(t *testing.T) {
		signerA := [20]uint8{0x11}
		signerB := [20]uint8{0x22}
		signerC := [20]uint8{0x33}

		remoteChainPDA := func(t *testing.T, chainSelector uint64) solana.PublicKey {
			pda, _, err := state.FindCVRemoteChainConfigPDA(chainSelector, config.CommitteeVerifierProgram)
			require.NoError(t, err)
			return pda
		}

		// Use the same EvmChainSelector that the signer_config tests left configured. Re-seed
		// the signer set so each remove_signer_config case starts from a known state.
		chainSelector := config.EvmChainSelector

		seedSigners := func(t *testing.T, signers [][20]uint8, threshold uint8) {
			resolved, err := ccip.ResolveCVSetSignerConfigAccounts(ctx, solanaGoClient, config.CommitteeVerifierProgram, committee_verifier.SetSignerConfigParams{
				SourceChainSelector: chainSelector,
				Threshold:           threshold,
				Signers:             signers,
			}, admin.PublicKey(), logger.Test(t))
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, admin, config.DefaultCommitment)
		}

		t.Run("fails when called by non-owner", func(t *testing.T) {
			seedSigners(t, [][20]uint8{signerA, signerB}, 1)

			ix, err := committee_verifier.NewRemoveSignerConfigInstruction(
				committee_verifier.RemoveSignerConfigParams{
					SourceChainSelector: chainSelector,
				},
				config.CVConfigPDA,
				user.PublicKey(),
				remoteChainPDA(t, chainSelector),
				solana.SystemProgramID,
				config.CVEventAuthorityPDA,
				config.CommitteeVerifierProgram,
			)
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ix}, user, config.DefaultCommitment,
				[]string{"Error Code: " + "Unauthorized"})
		})

		t.Run("fails when remote chain config PDA does not exist", func(t *testing.T) {
			// Pick a chain selector that is guaranteed not to have a chain config in this test
			// run. The PDA constraint on remote_chain_config will fail before the body runs.
			unconfiguredSelector := uint64(424242)

			resolved, err := ccip.ResolveCVRemoveSignerConfigAccounts(ctx, solanaGoClient, config.CommitteeVerifierProgram, committee_verifier.RemoveSignerConfigParams{
				SourceChainSelector: unconfiguredSelector,
			}, admin.PublicKey(), logger.Test(t))
			require.NoError(t, err)

			// The exact error string is Anchor's "AccountNotInitialized" / constraint failure;
			// asserting on it being a failure (no exact message) keeps the test resilient to
			// Anchor wording changes. SendAndFailWith requires a substring; we use the generic
			// "custom program error" prefix that always appears for constraint failures.
			testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, admin, config.DefaultCommitment,
				[]string{"custom program error"})
		})

		t.Run("clears signer config and emits empty event", func(t *testing.T) {
			seedSigners(t, [][20]uint8{signerA, signerB, signerC}, 2)

			resolved, err := ccip.ResolveCVRemoveSignerConfigAccounts(ctx, solanaGoClient, config.CommitteeVerifierProgram, committee_verifier.RemoveSignerConfigParams{
				SourceChainSelector: chainSelector,
			}, admin.PublicKey(), logger.Test(t))
			require.NoError(t, err)
			result := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, admin, config.DefaultCommitment)
			require.NotNil(t, result)

			// Event mirrors the EVM SignatureConfigSet(sel, [], 0) used for removal.
			tx, err := result.Transaction.GetTransaction()
			require.NoError(t, err)
			event, err := common.ExtractCPIEvent(tx, result.Meta, config.CommitteeVerifierProgram, committee_verifier.ParseEvent_SignerConfigSet)
			require.NoError(t, err)
			require.Equal(t, chainSelector, event.SourceChainSelector)
			require.Equal(t, uint8(0), event.Threshold)
			require.Empty(t, event.Signers)

			var cfg committee_verifier.RemoteChainConfig
			err = common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, remoteChainPDA(t, chainSelector), config.DefaultCommitment, &cfg)
			require.NoError(t, err)
			require.Equal(t, uint8(0), cfg.SignerConfig.Threshold)
			require.Empty(t, cfg.SignerConfig.Signers)
		})

		t.Run("preserves admin and allowlist config", func(t *testing.T) {
			// Re-seed signers so we have something to remove.
			seedSigners(t, [][20]uint8{signerA, signerB}, 1)

			// Configure an allowlist alongside the signers.
			allowedSender := solana.NewWallet().PrivateKey.PublicKey()
			allowResolved, err := ccip.ResolveCCVSetAllowlistAccounts(ctx, solanaGoClient, config.CommitteeVerifierProgram, committee_verifier.SetAllowlistParams{
				DestChainSelector: chainSelector,
				Enabled:           true,
				AllowedSenders:    []solana.PublicKey{allowedSender},
			}, admin.PublicKey(), logger.Test(t))
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{allowResolved.Ix}, admin, config.DefaultCommitment)

			// Snapshot admin_config so we can assert it survived.
			var before committee_verifier.RemoteChainConfig
			err = common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, remoteChainPDA(t, chainSelector), config.DefaultCommitment, &before)
			require.NoError(t, err)

			removeResolved, err := ccip.ResolveCVRemoveSignerConfigAccounts(ctx, solanaGoClient, config.CommitteeVerifierProgram, committee_verifier.RemoveSignerConfigParams{
				SourceChainSelector: chainSelector,
			}, admin.PublicKey(), logger.Test(t))
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{removeResolved.Ix}, admin, config.DefaultCommitment)

			var after committee_verifier.RemoteChainConfig
			err = common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, remoteChainPDA(t, chainSelector), config.DefaultCommitment, &after)
			require.NoError(t, err)

			require.Equal(t, before.AdminConfig, after.AdminConfig, "admin_config must be preserved")
			require.True(t, after.AllowlistConfig.AllowlistEnabled)
			require.Equal(t, []solana.PublicKey{allowedSender}, after.AllowlistConfig.AllowedSenders)

			require.Equal(t, uint8(0), after.SignerConfig.Threshold)
			require.Empty(t, after.SignerConfig.Signers)
		})

		t.Run("shrinks account size", func(t *testing.T) {
			seedSigners(t, [][20]uint8{signerA, signerB, signerC}, 2)

			pda := remoteChainPDA(t, chainSelector)
			beforeAcc, err := solanaGoClient.GetAccountInfoWithOpts(ctx, pda, &rpc.GetAccountInfoOpts{
				Commitment: config.DefaultCommitment,
			})
			require.NoError(t, err)
			require.NotNil(t, beforeAcc.Value)
			beforeSize := len(beforeAcc.Value.Data.GetBinary())

			resolved, err := ccip.ResolveCVRemoveSignerConfigAccounts(ctx, solanaGoClient, config.CommitteeVerifierProgram, committee_verifier.RemoveSignerConfigParams{
				SourceChainSelector: chainSelector,
			}, admin.PublicKey(), logger.Test(t))
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, admin, config.DefaultCommitment)

			var after committee_verifier.RemoteChainConfig
			err = common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, pda, config.DefaultCommitment, &after)
			require.NoError(t, err)
			require.Empty(t, after.SignerConfig.Signers)

			afterAcc, err := solanaGoClient.GetAccountInfoWithOpts(ctx, pda, &rpc.GetAccountInfoOpts{
				Commitment: config.DefaultCommitment,
			})
			require.NoError(t, err)
			require.NotNil(t, afterAcc.Value)
			afterSize := len(afterAcc.Value.Data.GetBinary())

			require.Less(t, afterSize, beforeSize, "PDA should shrink after removing signers (was %d, now %d)", beforeSize, afterSize)
		})

		t.Run("set_signer_config after remove succeeds", func(t *testing.T) {
			// Confirm we're starting from the cleared state from the previous subtest.
			var pre committee_verifier.RemoteChainConfig
			err := common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, remoteChainPDA(t, chainSelector), config.DefaultCommitment, &pre)
			require.NoError(t, err)
			require.Equal(t, uint8(0), pre.SignerConfig.Threshold)
			require.Empty(t, pre.SignerConfig.Signers)

			resolved, err := ccip.ResolveCVSetSignerConfigAccounts(ctx, solanaGoClient, config.CommitteeVerifierProgram, committee_verifier.SetSignerConfigParams{
				SourceChainSelector: chainSelector,
				Threshold:           1,
				Signers:             [][20]uint8{signerA},
			}, admin.PublicKey(), logger.Test(t))
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, admin, config.DefaultCommitment)

			var post committee_verifier.RemoteChainConfig
			err = common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, remoteChainPDA(t, chainSelector), config.DefaultCommitment, &post)
			require.NoError(t, err)
			require.Equal(t, uint8(1), post.SignerConfig.Threshold)
			require.Equal(t, [][20]uint8{signerA}, post.SignerConfig.Signers)
		})
	})

	t.Run("start_verification_at_source", func(t *testing.T) {
		destChainSelector := uint64(99999)
		allowedSender := solana.NewWallet().PrivateKey
		disallowedSender := solana.NewWallet().PrivateKey
		require.NoError(t, testutils.FundAccounts(ctx, []solana.PrivateKey{allowedSender, disallowedSender}, solanaGoClient))

		makeMessage := func(sender solana.PublicKey) committee_verifier.MessageV1 {
			return committee_verifier.MessageV1{
				DestChainSelector: destChainSelector,
				Sender:            sender.Bytes(),
				OnRampAddress:     []byte{0x01},
				OffRampAddress:    []byte{0x02},
				Receiver:          []byte{0x04},
			}
		}

		makeParams := func(sender solana.PublicKey) committee_verifier.StartVerificationAtSourceParams {
			return committee_verifier.StartVerificationAtSourceParams{
				Message:   makeMessage(sender),
				MessageId: [32]uint8{},
				FeeToken:  committee_verifier.SvmTokenAmount{},
			}
		}

		remoteChainPDA := func(t *testing.T) solana.PublicKey {
			pda, _, err := state.FindCVRemoteChainConfigPDA(destChainSelector, config.CommitteeVerifierProgram)
			require.NoError(t, err)
			return pda
		}

		t.Run("setup chain config with router=admin", func(t *testing.T) {
			resolved, err := ccip.ResolveCVInitChainConfigAccounts(ctx, solanaGoClient, config.CommitteeVerifierProgram, committee_verifier.SetChainConfigParams{
				RemoteChainSelector: destChainSelector,
				ChainConfig: committee_verifier.RemoteChainAdminConfig{
					RouterSigner: admin.PublicKey(),
				},
			}, admin.PublicKey(), logger.Test(t))
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, admin, config.DefaultCommitment)
		})

		t.Run("succeeds with allowlist disabled", func(t *testing.T) {
			params := makeParams(disallowedSender.PublicKey())
			ix, err := committee_verifier.NewStartVerificationAtSourceInstruction(
				params,
				config.CVConfigPDA,
				admin.PublicKey(),
				remoteChainPDA(t),
			)
			require.NoError(t, err)

			result := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)
			require.NotNil(t, result)
		})

		t.Run("enable allowlist with one allowed sender", func(t *testing.T) {
			resolved, err := ccip.ResolveCCVSetAllowlistAccounts(ctx, solanaGoClient, config.CommitteeVerifierProgram, committee_verifier.SetAllowlistParams{
				DestChainSelector: destChainSelector,
				Enabled:           true,
				AllowedSenders:    []solana.PublicKey{allowedSender.PublicKey()},
			}, admin.PublicKey(), logger.Test(t))
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, admin, config.DefaultCommitment)
		})

		t.Run("succeeds with allowed sender", func(t *testing.T) {
			params := makeParams(allowedSender.PublicKey())
			ix, err := committee_verifier.NewStartVerificationAtSourceInstruction(
				params,
				config.CVConfigPDA,
				admin.PublicKey(),
				remoteChainPDA(t),
			)
			require.NoError(t, err)

			result := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)
			require.NotNil(t, result)
		})

		t.Run("fails with disallowed sender", func(t *testing.T) {
			params := makeParams(disallowedSender.PublicKey())
			ix, err := committee_verifier.NewStartVerificationAtSourceInstruction(
				params,
				config.CVConfigPDA,
				admin.PublicKey(),
				remoteChainPDA(t),
			)
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment,
				[]string{"Error Code: " + "SenderNotAllowed"})
		})

		t.Run("fails with wrong authority (not the router)", func(t *testing.T) {
			params := makeParams(allowedSender.PublicKey())
			ix, err := committee_verifier.NewStartVerificationAtSourceInstruction(
				params,
				config.CVConfigPDA,
				user.PublicKey(),
				remoteChainPDA(t),
			)
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ix}, user, config.DefaultCommitment,
				[]string{"Error Code: " + "Unauthorized"})
		})
	})

	t.Run("withdraw_fee_tokens", func(t *testing.T) {
		mint := solana.NewWallet().PrivateKey

		tokenProgram := solana.Token2022ProgramID

		ixs, ixErr := tokens.CreateToken(ctx, tokenProgram, mint.PublicKey(), admin.PublicKey(), 9, solanaGoClient, config.DefaultCommitment)
		require.NoError(t, ixErr)

		// ccip_send_v2 pays every CCV receipt fee into the ATA of the verifier's ["fee_vault"] PDA
		// (see ccip_common::seed::vault_ata), so that ATA is the only account withdraw_fee_tokens
		// can sweep.
		feeVault, _, err := state.FindFeeVaultPDA(committee_verifier.ProgramID, nil)
		require.NoError(t, err)

		ixAta, feeVaultATA, ataErr := tokens.CreateAssociatedTokenAccount(tokenProgram, mint.PublicKey(), feeVault, admin.PublicKey())
		require.NoError(t, ataErr)
		expectedFeeVaultATA, err := tokens.FindFeeVaultATA(tokenProgram, mint.PublicKey(), committee_verifier.ProgramID, nil)
		require.NoError(t, err)
		require.Equal(t, expectedFeeVaultATA, feeVaultATA)

		ixAtaOwner, adminATA, ataOwnerErr := tokens.CreateAssociatedTokenAccount(tokenProgram, mint.PublicKey(), admin.PublicKey(), admin.PublicKey())
		require.NoError(t, ataOwnerErr)

		testutils.SendAndConfirm(ctx, t, solanaGoClient, append(ixs, ixAta, ixAtaOwner), admin, config.DefaultCommitment, common.AddSigners(mint))

		withdrawIx := func() solana.Instruction {
			ix, ixErr := committee_verifier.NewWithdrawFeeTokensInstruction(
				config.CVConfigPDA,
				feeVaultATA,
				feeVault,
				adminATA,
				mint.PublicKey(),
				tokenProgram,
			)
			require.NoError(t, ixErr)
			return ix
		}

		t.Run("fails if fee aggregator unset", func(t *testing.T) {
			// The aggregator is still the default pubkey, so the ATA constraint on
			// fee_aggregator_token_account can never be satisfied.
			testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{withdrawIx()}, user, config.DefaultCommitment,
				[]string{"Error Code: "})
		})

		t.Run("fails if no funds", func(t *testing.T) {
			ix, err := committee_verifier.NewSetFeeAggregatorInstruction(
				admin.PublicKey(), // aggregate funds to the admin wallet
				config.CVConfigPDA,
				admin.PublicKey(),
				config.CVEventAuthorityPDA,
				config.CommitteeVerifierProgram,
			)
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)

			testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{withdrawIx()}, user, config.DefaultCommitment,
				[]string{"Error Code: " + "InsufficientFunds"})
		})

		t.Run("withdraws tokens", func(t *testing.T) {
			ixMintTo, mintErr := tokens.MintTo(123, tokenProgram, mint.PublicKey(), feeVaultATA, admin.PublicKey())
			require.NoError(t, mintErr)
			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ixMintTo}, admin, config.DefaultCommitment, common.AddSigners(mint))

			// Permissionless: a non-owner can trigger the sweep, but only into the configured
			// aggregator's ATA.
			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{withdrawIx()}, user, config.DefaultCommitment)

			_, vaultBalance, err := tokens.TokenBalance(ctx, solanaGoClient, feeVaultATA, config.DefaultCommitment)
			require.NoError(t, err)
			require.Zero(t, vaultBalance)

			_, balance, err := tokens.TokenBalance(ctx, solanaGoClient, adminATA, config.DefaultCommitment)
			require.NoError(t, err)
			require.Equal(t, 123, balance)
		})
	})
}
