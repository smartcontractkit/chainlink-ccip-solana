// rmn_test.go tests the RMN (Risk Management Network) Remote program's runtime behavior.
// This includes global and per-chain cursing functionality, curse verification during ccip_send
// and commit operations, and curse management (apply/remove).
package contracts

import (
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/ccip_offramp"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/ccip_router"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/rmn_remote"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/config"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/testutils"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/ccip"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
)

func TestRMN(t *testing.T) {
	t.Parallel()
	env := testutils.NewTestEnv(t).WithTrafficConfig().Build()
	ctx := env.Ctx()
	client := env.Client
	user := testutils.User
	anotherUser := testutils.AnotherUser
	ccipAdmin := testutils.CcipAdmin
	legacyAdmin := testutils.Admin
	curser := testutils.TokenlessUser // reusing the TokenlessUser just for convenience, any separate key could be used here

	t.Run("RMNRemote", func(t *testing.T) {
		t.Run("Unauthorized event authorities cannot invoke cpi_event", func(t *testing.T) {
			ix, err := rmn_remote.NewCpiEventInstruction(
				[]byte("test"),
				config.RMNRemoteConfigPDA,
				anotherUser.PublicKey(), // not an authorized event authority
			)
			require.NoError(t, err)
			testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{ix}, anotherUser, config.DefaultCommitment, []string{"Error Code: " + ccip.Unauthorized_RmnRemoteError.String()})
			// the positive case of successful cpi_event calls is covered in the CCIP Send onramp tests
		})

		t.Run("Can transfer ownership", func(t *testing.T) {
			// setupTrafficConfig leaves ccipAdmin as the RMN owner. Exercise the full
			// ownership flow by transferring to legacyAdmin and then restoring ccipAdmin.
			instruction, err := rmn_remote.NewTransferOwnershipInstruction(
				legacyAdmin.PublicKey(),
				config.RMNRemoteConfigPDA,
				config.RMNRemoteCursesPDA,
				user.PublicKey(),
			)
			require.NoError(t, err)
			result := testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{instruction}, user, config.DefaultCommitment, []string{"Error Code: " + ccip.Unauthorized_RmnRemoteError.String()})
			require.NotNil(t, result)

			instruction, err = rmn_remote.NewTransferOwnershipInstruction(
				legacyAdmin.PublicKey(),
				config.RMNRemoteConfigPDA,
				config.RMNRemoteCursesPDA,
				ccipAdmin.PublicKey(),
			)
			require.NoError(t, err)
			result = testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{instruction}, ccipAdmin, config.DefaultCommitment)
			require.NotNil(t, result)

			transferEvent, err := common.ParseEventFromLogs(result.Meta.LogMessages, rmn_remote.ParseEvent_OwnershipTransferRequested)
			require.NoError(t, err)
			require.Equal(t, ccipAdmin.PublicKey(), transferEvent.From)
			require.Equal(t, legacyAdmin.PublicKey(), transferEvent.To)

			instruction, err = rmn_remote.NewAcceptOwnershipInstruction(
				config.RMNRemoteConfigPDA,
				user.PublicKey(),
			)
			require.NoError(t, err)
			result = testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{instruction}, user, config.DefaultCommitment, []string{"Error Code: " + ccip.Unauthorized_RmnRemoteError.String()})
			require.NotNil(t, result)

			instruction, err = rmn_remote.NewAcceptOwnershipInstruction(
				config.RMNRemoteConfigPDA,
				legacyAdmin.PublicKey(),
			)
			require.NoError(t, err)
			result = testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{instruction}, legacyAdmin, config.DefaultCommitment)
			require.NotNil(t, result)

			acceptEvent, err := common.ParseEventFromLogs(result.Meta.LogMessages, rmn_remote.ParseEvent_OwnershipTransferred)
			require.NoError(t, err)
			require.Equal(t, ccipAdmin.PublicKey(), acceptEvent.From)
			require.Equal(t, legacyAdmin.PublicKey(), acceptEvent.To)

			instruction, err = rmn_remote.NewTransferOwnershipInstruction(
				legacyAdmin.PublicKey(),
				config.RMNRemoteConfigPDA,
				config.RMNRemoteCursesPDA,
				legacyAdmin.PublicKey(),
			)
			require.NoError(t, err)
			result = testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{instruction}, legacyAdmin, config.DefaultCommitment, []string{"Error Code: " + ccip.RedundantOwnerProposal_RmnRemoteError.String()})
			require.NotNil(t, result)

			var configAccount rmn_remote.Config
			err = common.GetAccountDataBorshIntoAnchor(ctx, client, config.RMNRemoteConfigPDA, config.DefaultCommitment, &configAccount)
			require.NoError(t, err, "failed to get account info")
			require.Equal(t, solana.PublicKey{}, configAccount.ProposedOwner)

			instruction, err = rmn_remote.NewTransferOwnershipInstruction(
				ccipAdmin.PublicKey(),
				config.RMNRemoteConfigPDA,
				config.RMNRemoteCursesPDA,
				legacyAdmin.PublicKey(),
			)
			require.NoError(t, err)
			result = testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{instruction}, legacyAdmin, config.DefaultCommitment)
			require.NotNil(t, result)

			instruction, err = rmn_remote.NewAcceptOwnershipInstruction(
				config.RMNRemoteConfigPDA,
				ccipAdmin.PublicKey(),
			)
			require.NoError(t, err)
			result = testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{instruction}, ccipAdmin, config.DefaultCommitment)
			require.NotNil(t, result)

			err = common.GetAccountDataBorshIntoAnchor(ctx, client, config.RMNRemoteConfigPDA, config.DefaultCommitment, &configAccount)
			require.NoError(t, err, "failed to get account info")
			require.Equal(t, ccipAdmin.PublicKey(), configAccount.Owner)
			require.Equal(t, solana.PublicKey{}, configAccount.ProposedOwner)
		})

		t.Run("setup: set curser", func(t *testing.T) {
			ix, err := rmn_remote.NewSetCurserInstruction(
				curser.PublicKey(), // new curser
				config.RMNRemoteConfigPDA,
				config.RMNRemoteCursesPDA,
				ccipAdmin.PublicKey(),
			)
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{ix}, ccipAdmin, config.DefaultCommitment)

			var configAccount rmn_remote.Config
			err = common.GetAccountDataBorshIntoAnchor(ctx, client, config.RMNRemoteConfigPDA, config.DefaultCommitment, &configAccount)
			require.NoError(t, err, "failed to get account info")
			require.Equal(t, curser.PublicKey(), configAccount.Curser)
			require.Equal(t, ccipAdmin.PublicKey(), configAccount.Owner) // owner is unchanged
		})
	})

	t.Run("Curser authority", func(t *testing.T) {
		t.Run("Unauthorized user cannot curse", func(t *testing.T) {
			globalCurse := testutils.GlobalCurseSubject()

			ix, err := rmn_remote.NewCurseInstruction(
				globalCurse,
				config.RMNRemoteConfigPDA,
				user.PublicKey(),
				config.RMNRemoteCursesPDA,
				solana.SystemProgramID,
			)
			require.NoError(t, err)
			testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{ix}, user, config.DefaultCommitment, []string{ccip.Unauthorized_RmnRemoteError.String()})
		})

		t.Run("Curser can curse", func(t *testing.T) {
			globalCurse := testutils.GlobalCurseSubject()

			ix, err := rmn_remote.NewCurseInstruction(
				globalCurse,
				config.RMNRemoteConfigPDA,
				curser.PublicKey(),
				config.RMNRemoteCursesPDA,
				solana.SystemProgramID,
			)
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{ix}, curser, config.DefaultCommitment)
		})

		t.Run("Curser cannot uncurse", func(t *testing.T) {
			globalCurse := testutils.GlobalCurseSubject()

			ix, err := rmn_remote.NewUncurseInstruction(
				globalCurse,
				config.RMNRemoteConfigPDA,
				curser.PublicKey(),
				config.RMNRemoteCursesPDA,
				solana.SystemProgramID,
			)
			require.NoError(t, err)
			testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{ix}, curser, config.DefaultCommitment, []string{ccip.Unauthorized_RmnRemoteError.String()})
		})

		t.Run("Cleanup: Owner can uncurse", func(t *testing.T) {
			globalCurse := testutils.GlobalCurseSubject()

			ix, err := rmn_remote.NewUncurseInstruction(
				globalCurse,
				config.RMNRemoteConfigPDA,
				ccipAdmin.PublicKey(),
				config.RMNRemoteCursesPDA,
				solana.SystemProgramID,
			)
			require.NoError(t, err)
			result := testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{ix}, ccipAdmin, config.DefaultCommitment)
			require.NotNil(t, result)

			var curses rmn_remote.Curses
			err = common.GetAccountDataBorshIntoAnchor(ctx, client, config.RMNRemoteCursesPDA, config.DefaultCommitment, &curses)
			require.NoError(t, err, "failed to get account info")
			require.Empty(t, curses.CursedSubjects)
		})
	})

	t.Run("Manual Cursing", func(t *testing.T) {
		t.Run("no curses by default", func(t *testing.T) {
			svmCurse := testutils.ChainCurseSubject(config.SvmChainSelector)
			evmCurse := testutils.ChainCurseSubject(config.EvmChainSelector)

			ix, err := rmn_remote.NewVerifyNotCursedInstruction(
				evmCurse,
				config.RMNRemoteCursesPDA,
				config.RMNRemoteConfigPDA,
			)
			require.NoError(t, err)
			result := testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{ix}, user, config.DefaultCommitment)
			require.NotNil(t, result)

			ix, err = rmn_remote.NewVerifyNotCursedInstruction(
				svmCurse,
				config.RMNRemoteCursesPDA,
				config.RMNRemoteConfigPDA,
			)
			require.NoError(t, err)
			result = testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{ix}, user, config.DefaultCommitment)
			require.NotNil(t, result)
		})

		t.Run("applying a global curse", func(t *testing.T) {
			globalCurse := testutils.GlobalCurseSubject()

			ix, err := rmn_remote.NewCurseInstruction(
				globalCurse,
				config.RMNRemoteConfigPDA,
				ccipAdmin.PublicKey(),
				config.RMNRemoteCursesPDA,
				solana.SystemProgramID,
			)
			require.NoError(t, err)
			result := testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{ix}, ccipAdmin, config.DefaultCommitment)
			require.NotNil(t, result)

			var curses rmn_remote.Curses
			err = common.GetAccountDataBorshIntoAnchor(ctx, client, config.RMNRemoteCursesPDA, config.DefaultCommitment, &curses)
			require.NoError(t, err, "failed to get account info")
			require.Len(t, curses.CursedSubjects, 1)
			require.Equal(t, curses.CursedSubjects[0], globalCurse)

			// All subjects are cursed now
			svmCurse := testutils.ChainCurseSubject(config.SvmChainSelector)
			evmCurse := testutils.ChainCurseSubject(config.EvmChainSelector)

			ix, err = rmn_remote.NewVerifyNotCursedInstruction(
				evmCurse,
				config.RMNRemoteCursesPDA,
				config.RMNRemoteConfigPDA,
			)
			require.NoError(t, err)
			result = testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{ix}, user, config.DefaultCommitment, []string{"Error Code: GloballyCursed"})
			require.NotNil(t, result)

			ix, err = rmn_remote.NewVerifyNotCursedInstruction(
				svmCurse,
				config.RMNRemoteCursesPDA,
				config.RMNRemoteConfigPDA,
			)
			require.NoError(t, err)
			result = testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{ix}, user, config.DefaultCommitment, []string{"Error Code: GloballyCursed"})
			require.NotNil(t, result)

			ix, err = rmn_remote.NewVerifyNotCursedInstruction(
				globalCurse,
				config.RMNRemoteCursesPDA,
				config.RMNRemoteConfigPDA,
			)
			require.NoError(t, err)
			result = testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{ix}, user, config.DefaultCommitment, []string{"Error Code: GloballyCursed"})
			require.NotNil(t, result)
		})

		t.Run("ccip_send fails with a global curse active", func(t *testing.T) {
			destinationChainSelector := config.EvmChainSelector
			destinationChainStatePDA := config.EvmDestChainStatePDA
			message := ccip_router.Svm2AnyMessage{
				FeeToken:  env.WSOL.Mint,
				Receiver:  env.ValidEvmAbiReceiverAddress[:],
				Data:      []byte{4, 5, 6},
				ExtraArgs: []byte{},
			}

			instruction, err := ccip_router.NewCcipSendInstruction(
				destinationChainSelector,
				message,
				[]byte{},
				config.RouterConfigPDA,
				destinationChainStatePDA,
				env.NonceEvmPDA,
				user.PublicKey(),
				solana.SystemProgramID,
				env.WSOL.Program,
				env.WSOL.Mint,
				env.WSOL.UserATA,
				env.WSOL.BillingATA,
				config.BillingSignerPDA,
				config.FeeQuoterProgram,
				config.FqConfigPDA,
				config.FqEvmDestChainPDA,
				env.WSOL.FqBillingCfgPDA,
				env.Link22.FqBillingCfgPDA,
				config.RMNRemoteProgram,
				config.RMNRemoteCursesPDA,
				config.RMNRemoteConfigPDA,
			)
			require.NoError(t, err)
			instruction.(*solana.GenericInstruction).AccountValues[7].IsWritable = true
			result := testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{instruction}, user, config.DefaultCommitment, []string{"Error Code: GloballyCursed"})
			require.NotNil(t, result)
		})

		t.Run("ccip_execute fails with a global curse active", func(t *testing.T) {
			encodedMsg := testutils.EncodeMinimalMessageV1(config.EvmChainSelector)
			execInputs := ccip_offramp.ExecutionInputsV2{
				EncodedMessage:  encodedMsg,
				VerifierResults: [][]byte{},
				Ccvs:            []solana.PublicKey{},
			}

			resolved, err := ccip.ResolveExecAccounts(ctx, client, config.CcipOfframpProgram, execInputs, user.PublicKey(), logger.Test(t))
			require.NoError(t, err)

			result := testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{resolved.Ix}, user, config.DefaultCommitment, []string{"Error Code: GloballyCursed"})
			require.NotNil(t, result)
		})

		t.Run("removing a global curse", func(t *testing.T) {
			globalCurse := testutils.GlobalCurseSubject()

			ix, err := rmn_remote.NewUncurseInstruction(
				globalCurse,
				config.RMNRemoteConfigPDA,
				ccipAdmin.PublicKey(),
				config.RMNRemoteCursesPDA,
				solana.SystemProgramID,
			)
			require.NoError(t, err)
			result := testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{ix}, ccipAdmin, config.DefaultCommitment)
			require.NotNil(t, result)

			var curses rmn_remote.Curses
			err = common.GetAccountDataBorshIntoAnchor(ctx, client, config.RMNRemoteCursesPDA, config.DefaultCommitment, &curses)
			require.NoError(t, err, "failed to get account info")
			require.Empty(t, curses.CursedSubjects)
		})

		t.Run("adding chain selector curses", func(t *testing.T) {
			svmCurse := testutils.ChainCurseSubject(config.SvmChainSelector)
			evmCurse := testutils.ChainCurseSubject(config.EvmChainSelector)

			ix, err := rmn_remote.NewCurseInstruction(
				svmCurse,
				config.RMNRemoteConfigPDA,
				ccipAdmin.PublicKey(),
				config.RMNRemoteCursesPDA,
				solana.SystemProgramID,
			)
			require.NoError(t, err)
			result := testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{ix}, ccipAdmin, config.DefaultCommitment)
			require.NotNil(t, result)

			var curses rmn_remote.Curses
			err = common.GetAccountDataBorshIntoAnchor(ctx, client, config.RMNRemoteCursesPDA, config.DefaultCommitment, &curses)
			require.NoError(t, err, "failed to get account info")
			require.Len(t, curses.CursedSubjects, 1)
			require.Equal(t, curses.CursedSubjects[0], svmCurse)

			ix, err = rmn_remote.NewCurseInstruction(
				evmCurse,
				config.RMNRemoteConfigPDA,
				ccipAdmin.PublicKey(),
				config.RMNRemoteCursesPDA,
				solana.SystemProgramID,
			)
			require.NoError(t, err)
			result = testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{ix}, ccipAdmin, config.DefaultCommitment)
			require.NotNil(t, result)

			err = common.GetAccountDataBorshIntoAnchor(ctx, client, config.RMNRemoteCursesPDA, config.DefaultCommitment, &curses)
			require.NoError(t, err, "failed to get account info")
			require.Len(t, curses.CursedSubjects, 2)
			require.Equal(t, curses.CursedSubjects[0], svmCurse)
			require.Equal(t, curses.CursedSubjects[1], evmCurse)

			ix, err = rmn_remote.NewUncurseInstruction(
				svmCurse,
				config.RMNRemoteConfigPDA,
				ccipAdmin.PublicKey(),
				config.RMNRemoteCursesPDA,
				solana.SystemProgramID,
			)
			require.NoError(t, err)
			result = testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{ix}, ccipAdmin, config.DefaultCommitment)
			require.NotNil(t, result)

			err = common.GetAccountDataBorshIntoAnchor(ctx, client, config.RMNRemoteCursesPDA, config.DefaultCommitment, &curses)
			require.NoError(t, err, "failed to get account info")
			require.Len(t, curses.CursedSubjects, 1)
			require.Equal(t, curses.CursedSubjects[0], evmCurse)

			ix, err = rmn_remote.NewVerifyNotCursedInstruction(
				evmCurse,
				config.RMNRemoteCursesPDA,
				config.RMNRemoteConfigPDA,
			)
			require.NoError(t, err)
			result = testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{ix}, user, config.DefaultCommitment, []string{"Error Code: SubjectCursed"})
			require.NotNil(t, result)

			ix, err = rmn_remote.NewVerifyNotCursedInstruction(
				svmCurse,
				config.RMNRemoteCursesPDA,
				config.RMNRemoteConfigPDA,
			)
			require.NoError(t, err)
			result = testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{ix}, legacyAdmin, config.DefaultCommitment)
			require.NotNil(t, result)

			globalCurse := testutils.GlobalCurseSubject()

			ix, err = rmn_remote.NewVerifyNotCursedInstruction(
				globalCurse,
				config.RMNRemoteCursesPDA,
				config.RMNRemoteConfigPDA,
			)
			require.NoError(t, err)
			result = testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{ix}, legacyAdmin, config.DefaultCommitment)
			require.NotNil(t, result)
		})

		t.Run("ccip_send fails with a cursed destination", func(t *testing.T) {
			destinationChainSelector := config.EvmChainSelector
			destinationChainStatePDA := config.EvmDestChainStatePDA
			message := ccip_router.Svm2AnyMessage{
				FeeToken:  env.WSOL.Mint,
				Receiver:  env.ValidEvmAbiReceiverAddress[:],
				Data:      []byte{4, 5, 6},
				ExtraArgs: []byte{},
			}

			instruction, err := ccip_router.NewCcipSendInstruction(
				destinationChainSelector,
				message,
				[]byte{},
				config.RouterConfigPDA,
				destinationChainStatePDA,
				env.NonceEvmPDA,
				user.PublicKey(),
				solana.SystemProgramID,
				env.WSOL.Program,
				env.WSOL.Mint,
				env.WSOL.UserATA,
				env.WSOL.BillingATA,
				config.BillingSignerPDA,
				config.FeeQuoterProgram,
				config.FqConfigPDA,
				config.FqEvmDestChainPDA,
				env.WSOL.FqBillingCfgPDA,
				env.Link22.FqBillingCfgPDA,
				config.RMNRemoteProgram,
				config.RMNRemoteCursesPDA,
				config.RMNRemoteConfigPDA,
			)
			require.NoError(t, err)
			instruction.(*solana.GenericInstruction).AccountValues[7].IsWritable = true
			result := testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{instruction}, user, config.DefaultCommitment, []string{"Error Code: SubjectCursed"})
			require.NotNil(t, result)
		})

		t.Run("ccip_execute fails with a cursed destination", func(t *testing.T) {
			encodedMsg := testutils.EncodeMinimalMessageV1(config.EvmChainSelector)
			execInputs := ccip_offramp.ExecutionInputsV2{
				EncodedMessage:  encodedMsg,
				VerifierResults: [][]byte{},
				Ccvs:            []solana.PublicKey{},
			}
			resolved, err := ccip.ResolveExecAccounts(ctx, client, config.CcipOfframpProgram, execInputs, user.PublicKey(), logger.Test(t))

			require.NoError(t, err)

			result := testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{resolved.Ix}, user, config.DefaultCommitment, []string{"Error Code: SubjectCursed"})
			require.NotNil(t, result)
		})

		t.Run("cleanup", func(t *testing.T) {
			evmCurse := testutils.ChainCurseSubject(config.EvmChainSelector)
			ix, err := rmn_remote.NewUncurseInstruction(
				evmCurse,
				config.RMNRemoteConfigPDA,
				ccipAdmin.PublicKey(),
				config.RMNRemoteCursesPDA,
				solana.SystemProgramID,
			)
			require.NoError(t, err)
			result := testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{ix}, ccipAdmin, config.DefaultCommitment)
			require.NotNil(t, result)

			var curses rmn_remote.Curses
			err = common.GetAccountDataBorshIntoAnchor(ctx, client, config.RMNRemoteCursesPDA, config.DefaultCommitment, &curses)
			require.NoError(t, err, "failed to get account info")
			require.Empty(t, curses.CursedSubjects)
		})
	})
}
