package examples

import (
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/require"

	receiver "github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/example_ccip_receiver_v2"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/config"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/testutils"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
)

// TestCcipReceiverV2 exercises the example v2 receiver on-chain. It focuses on the areas flagged as
// untested in review: that PDA `version`/`bump` are actually persisted on init (the exact bug class
// this program is susceptible to), the CCV-config lifecycle (set/get/remove + on-chain validation),
// and sender approval.
//
// NOTE: end-to-end `ccip_receive_v2` delivery is intentionally not exercised here. The
// `arbitrary_cpi_proxy` helper (testutils.NewReceiverProxyExecuteInstruction) is hardcoded to the
// v1 `ccip_receive` discriminator and message layout; driving `ccip_receive_v2` needs either a v2
// proxy helper or the real OffRamp v2 CPI (NONEVM-4970).
func TestCcipReceiverV2(t *testing.T) {
	ctx := t.Context()

	program := config.CcipBaseReceiverV2

	ccipAdmin := solana.MustPrivateKeyFromBase58("4D7Hw7YFWqN3jknCRuViYqxF3AKmYosQPnm3szmrR3bvnCPrxKchUCxfFWbQqMCb4oe7jfxynGmjFCTDSrPBdcUB")
	user := solana.MustPrivateKeyFromBase58("5VNkUFwLJ12f71vBMW3XWUfRUpMUnzBxXhPPePi8CzaSXfmQAC842BQtSDkBXR85q4pp6kR7DSiFWBVWGLbFTSoq")

	receiverConfig, _, stateErr := solana.FindProgramAddress([][]byte{[]byte("config")}, program)
	require.NoError(t, stateErr)
	eventAuthority, _, eventErr := state.FindEventAuthorityPDA(program)
	require.NoError(t, eventErr)

	solClient := testutils.DeployAllPrograms(t, testutils.PathToAnchorConfig, ccipAdmin)
	require.NoError(t, testutils.FundAccounts(ctx, []solana.PrivateKey{ccipAdmin, user}, solClient))

	selector := config.EvmChainSelector
	ccvConfigPDA, _, ccvPDAErr := solana.FindProgramAddress(
		[][]byte{[]byte("ccv_config"), common.Uint64ToLE(selector)}, program)
	require.NoError(t, ccvPDAErr)

	ccvA := solana.NewWallet().PublicKey()
	ccvB := solana.NewWallet().PublicKey()
	ccvC := solana.NewWallet().PublicKey()

	t.Run("initialize is restricted to the upgrade authority", func(t *testing.T) {
		programData := testutils.GetProgramDataAddress(ctx, t, solClient, program)
		ix, err := receiver.NewInitializeInstruction(
			config.CcipRouterProgram, receiverConfig, user.PublicKey(), solana.SystemProgramID, program, programData)
		require.NoError(t, err)
		testutils.SendAndFailWith(ctx, t, solClient, []solana.Instruction{ix}, user, rpc.CommitmentConfirmed, []string{"Error Code: Unauthorized"})
	})

	t.Run("initialize persists version + bump", func(t *testing.T) {
		programData := testutils.GetProgramDataAddress(ctx, t, solClient, program)
		ix, err := receiver.NewInitializeInstruction(
			config.CcipRouterProgram, receiverConfig, ccipAdmin.PublicKey(), solana.SystemProgramID, program, programData)
		require.NoError(t, err)
		// The upgrade authority hands ownership to the user, who drives the receiver below.
		transferIx, err := receiver.NewTransferOwnershipInstruction(user.PublicKey(), receiverConfig, ccipAdmin.PublicKey())
		require.NoError(t, err)
		testutils.SendAndConfirm(ctx, t, solClient, []solana.Instruction{ix, transferIx}, ccipAdmin, rpc.CommitmentConfirmed)
		acceptIx, err := receiver.NewAcceptOwnershipInstruction(receiverConfig, user.PublicKey())
		require.NoError(t, err)
		testutils.SendAndConfirm(ctx, t, solClient, []solana.Instruction{acceptIx}, user, rpc.CommitmentConfirmed)

		info, err := solClient.GetAccountInfoWithOpts(ctx, receiverConfig, &rpc.GetAccountInfoOpts{
			Commitment: rpc.CommitmentConfirmed,
		})
		require.NoError(t, err)
		st, err := receiver.ParseAccount_Config(info.Bytes())
		require.NoError(t, err)
		// The bug class this guards against: fields declared but never written at init.
		require.Equal(t, uint8(1), st.Version)
		require.NotZero(t, st.Bump)
		require.Equal(t, user.PublicKey(), st.Owner)
		require.Equal(t, config.CcipRouterProgram, st.Router)
	})

	t.Run("set_ccv_config persists config + version + bump", func(t *testing.T) {
		params := receiver.SetCcvConfigParams{
			SourceChainSelector:   selector,
			RequiredCcvs:          []solana.PublicKey{ccvA},
			OptionalCcvs:          []solana.PublicKey{ccvB, ccvC},
			OptionalThreshold:     1,
			AllowedFinalityConfig: receiver.FinalityConfig{}, // WAIT_FOR_FINALITY
		}
		ix, err := receiver.NewSetCcvConfigInstruction(
			params, receiverConfig, ccvConfigPDA, user.PublicKey(), solana.SystemProgramID, eventAuthority, program)
		require.NoError(t, err)
		testutils.SendAndConfirm(ctx, t, solClient, []solana.Instruction{ix}, user, rpc.CommitmentConfirmed)

		info, err := solClient.GetAccountInfoWithOpts(ctx, ccvConfigPDA, &rpc.GetAccountInfoOpts{
			Commitment: rpc.CommitmentConfirmed,
		})
		require.NoError(t, err)
		cfg, err := receiver.ParseAccount_SourceChainCcvConfig(info.Bytes())
		require.NoError(t, err)
		require.Equal(t, uint8(1), cfg.Version)
		require.NotZero(t, cfg.Bump)
		require.Equal(t, []solana.PublicKey{ccvA}, cfg.RequiredCcvs)
		require.Equal(t, []solana.PublicKey{ccvB, ccvC}, cfg.OptionalCcvs)
		require.Equal(t, uint8(1), cfg.OptionalThreshold)
	})

	t.Run("set_ccv_config rejects duplicate CCVs on-chain", func(t *testing.T) {
		params := receiver.SetCcvConfigParams{
			SourceChainSelector: selector,
			RequiredCcvs:        []solana.PublicKey{ccvA, ccvA},
		}
		ix, err := receiver.NewSetCcvConfigInstruction(
			params, receiverConfig, ccvConfigPDA, user.PublicKey(), solana.SystemProgramID, eventAuthority, program)
		require.NoError(t, err)
		testutils.SendAndFailWith(ctx, t, solClient, []solana.Instruction{ix}, user, rpc.CommitmentConfirmed,
			[]string{"Duplicate CCV in configuration"})
	})

	t.Run("set_ccv_config from non-owner fails", func(t *testing.T) {
		// ccipAdmin is not the receiver owner (user is), so the owner constraint must reject it.
		params := receiver.SetCcvConfigParams{
			SourceChainSelector: selector,
			RequiredCcvs:        []solana.PublicKey{ccvA},
		}
		ix, err := receiver.NewSetCcvConfigInstruction(
			params, receiverConfig, ccvConfigPDA, ccipAdmin.PublicKey(), solana.SystemProgramID, eventAuthority, program)
		require.NoError(t, err)
		testutils.SendAndFailWith(ctx, t, solClient, []solana.Instruction{ix}, ccipAdmin, rpc.CommitmentConfirmed,
			[]string{"Address is not owner"})
	})

	t.Run("get_ccvs_and_finality_config executes (configured + unconfigured)", func(t *testing.T) {
		ixConfigured, err := receiver.NewGetCcvsAndFinalityConfigInstruction(
			receiver.GetReceiverCcvsIn{SourceChainSelector: selector, Sender: []byte{1, 2, 3}}, ccvConfigPDA)
		require.NoError(t, err)
		testutils.SendAndConfirm(ctx, t, solClient, []solana.Instruction{ixConfigured}, user, rpc.CommitmentConfirmed)

		// Unconfigured source chain: the PDA is uninitialized, and the instruction must still succeed
		// (returning the lane-defaults fallback) rather than erroring.
		unsetSelector := config.SvmChainSelector
		unsetPDA, _, pdaErr := solana.FindProgramAddress(
			[][]byte{[]byte("ccv_config"), common.Uint64ToLE(unsetSelector)}, program)
		require.NoError(t, pdaErr)
		ixUnset, err := receiver.NewGetCcvsAndFinalityConfigInstruction(
			receiver.GetReceiverCcvsIn{SourceChainSelector: unsetSelector}, unsetPDA)
		require.NoError(t, err)
		testutils.SendAndConfirm(ctx, t, solClient, []solana.Instruction{ixUnset}, user, rpc.CommitmentConfirmed)
	})

	t.Run("remove_ccv_config closes the account", func(t *testing.T) {
		ix, err := receiver.NewRemoveCcvConfigInstruction(
			selector, receiverConfig, ccvConfigPDA, user.PublicKey(), eventAuthority, program)
		require.NoError(t, err)
		testutils.SendAndConfirm(ctx, t, solClient, []solana.Instruction{ix}, user, rpc.CommitmentConfirmed)

		_, err = solClient.GetAccountInfo(ctx, ccvConfigPDA)
		require.ErrorContains(t, err, "not found")
	})

	t.Run("approve/unapprove sender lifecycle", func(t *testing.T) {
		sender := []byte{1, 2, 3}
		approvedPDA, findErr := state.FindApprovedSenderV2(selector, sender, program)
		require.NoError(t, findErr)

		ixApprove, err := receiver.NewApproveSenderInstruction(
			receiver.SenderParams{ChainSelector: selector, RemoteSender: sender},
			receiverConfig, approvedPDA, user.PublicKey(), solana.SystemProgramID)
		require.NoError(t, err)
		testutils.SendAndConfirm(ctx, t, solClient, []solana.Instruction{ixApprove}, user, rpc.CommitmentConfirmed)

		// ApprovedSender is a data-less marker PDA: approval is conveyed by the account's mere
		// existence (owned by the program), so we only assert it was created and parses.
		info, err := solClient.GetAccountInfoWithOpts(ctx, approvedPDA, &rpc.GetAccountInfoOpts{
			Commitment: rpc.CommitmentConfirmed,
		})
		require.NoError(t, err)
		_, err = receiver.ParseAccount_ApprovedSender(info.Bytes())
		require.NoError(t, err)

		ixUnapprove, err := receiver.NewUnapproveSenderInstruction(
			receiver.SenderParams{ChainSelector: selector, RemoteSender: sender},
			receiverConfig, approvedPDA, user.PublicKey(), solana.SystemProgramID)
		require.NoError(t, err)
		testutils.SendAndConfirm(ctx, t, solClient, []solana.Instruction{ixUnapprove}, user, rpc.CommitmentConfirmed)

		_, err = solClient.GetAccountInfo(ctx, approvedPDA)
		require.ErrorContains(t, err, "not found")
	})
}
