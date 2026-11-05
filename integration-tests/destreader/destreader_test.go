package destreader

import (
	"context"
	"maps"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gagliardetto/solana-go"
	computebudget "github.com/gagliardetto/solana-go/programs/compute-budget"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-ccv/protocol"
	commonconfig "github.com/smartcontractkit/chainlink-common/pkg/config"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/fees"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/ccip_offramp"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/committee_verifier"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/test_ccip_receiver"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/config"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/testutils"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/testutils/eth"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/ccip"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	"github.com/smartcontractkit/chainlink-ccip-solana/pkg/destinationreader"
	"github.com/smartcontractkit/chainlink-ccip-solana/pkg/logpoller"
)

// TestExecutionAttemptPoller_HonestRace pins the multi-submitter property: two
// executors submit execute_v2 for the same message at the same moment. On-chain the
// winner executes the message; the loser finds the message already Successful and
// returns Ok as a no-op WITHOUT emitting InProgress (processor.rs returns early).
// The poller must therefore cache exactly one attempt for the message, and no failed
// attempt can exist, so GetAttempts returns a single entry.
func TestExecutionAttemptPoller_HonestRace(t *testing.T) {
	env := testutils.NewTestEnv(t).WithTrafficConfig().Build()
	ctx := env.Ctx()
	client := env.Client
	lggr := logger.Test(t)

	ccvSigners := configureCommitteeVerifier(ctx, t, client)
	msg, encoded, verifierResults := newSignedMessage(t, ccvSigners)

	// Resolve per payer: the resolved ix binds the payer as the executor signer,
	// so each submitter needs its own resolution of the same inputs.
	payers := []solana.PrivateKey{testutils.User, testutils.AnotherUser}
	execInputs := ccip_offramp.ExecutionInputsV2{
		EncodedMessage:  encoded,
		Ccvs:            []solana.PublicKey{config.CommitteeVerifierProgram},
		VerifierResults: [][]byte{verifierResults},
	}
	resolved := make([]ccip.ResolveAccountsResult, len(payers))
	lutMaps := make([]map[solana.PublicKey]solana.PublicKeySlice, len(payers))
	var err error
	for i, payer := range payers {
		resolved[i], err = ccip.ResolveExecAccounts(ctx, client, config.CcipOfframpProgram, execInputs, payer.PublicKey(), lggr)
		require.NoError(t, err)
		lutMaps[i], err = common.GetAddressLookupTableMap(ctx, client, resolved[i].LookupTables)
		require.NoError(t, err)
	}

	// The poller under test, watching the offramp for ExecutionStateChangedV2 events.
	lp := newOfframpLogPoller(t, client)
	require.NoError(t, lp.Start(ctx))
	t.Cleanup(func() { require.NoError(t, lp.Close()) })

	poller, err := destinationreader.NewExecutionAttemptPoller(destinationreader.ExecutionAttemptPollerParams{
		Lggr:             lggr,
		LogPoller:        lp,
		RPC:              directRPC{client},
		OfframpProgramID: config.CcipOfframpProgram,
		Config:           destinationreader.Config{PollInterval: *commonconfig.MustNewDuration(300 * time.Millisecond)},
	})
	require.NoError(t, err)
	require.NoError(t, poller.Start(ctx))
	t.Cleanup(func() { require.NoError(t, poller.Close()) })

	// Fire the same execution from two payers off a shared barrier. The wrapped
	// common.SendAndConfirm variant is used directly: the testutils wrapper calls
	// require.NoError, which is only safe on the test goroutine.
	type sendOutcome struct {
		res *rpc.GetTransactionResult
		err error
	}
	barrier := make(chan struct{})
	outcomes := make(chan sendOutcome, len(payers))
	for i, payer := range payers {
		go func(i int, payer solana.PrivateKey) {
			<-barrier
			res, sendErr := common.SendAndConfirmWithLookupTables(ctx, client,
				[]solana.Instruction{resolved[i].Ix}, payer, config.DefaultCommitment, lutMaps[i],
				common.AddComputeUnitLimit(honestCU))
			outcomes <- sendOutcome{res, sendErr}
		}(i, payer)
	}
	close(barrier)
	for range payers {
		o := <-outcomes
		require.NoError(t, o.err)
		require.NotNil(t, o.res)
		// Both txs succeed: the loser of the race is a successful no-op.
		require.Nil(t, o.res.Meta.Err)
	}

	// Exactly one attempt is cached: one InProgress event exists per message, ever.
	require.Eventually(t, func() bool {
		attempts, pollErr := poller.GetAttempts(msg)
		return pollErr == nil && len(attempts) == 1
	}, 30*time.Second, 500*time.Millisecond, "poller must cache exactly one attempt for the raced message")
}

// TestExecutionAttemptPoller_BatchedBufferedExecute sends one transaction with two buffered
// execute_v2 instructions. The poller must match each InProgress event to its own instruction.
func TestExecutionAttemptPoller_BatchedBufferedExecute(t *testing.T) {
	env := testutils.NewTestEnv(t).WithTrafficConfig().Build()
	ctx := env.Ctx()
	client := env.Client
	lggr := logger.Test(t)
	payer := testutils.User

	ccvSigners := configureCommitteeVerifier(ctx, t, client)
	msgs := make([]protocol.Message, 2)
	results := make([][]byte, 2)
	ixs := make([]solana.Instruction, 2)
	luts := map[solana.PublicKey]solana.PublicKeySlice{}
	for i := range msgs {
		msg := testutils.MinimalMessageV1(config.EvmChainSelector)
		msg.SequenceNumber += protocol.SequenceNumber(i)
		encoded, err := msg.Encode()
		require.NoError(t, err)
		msgID, err := msg.MessageID()
		require.NoError(t, err)
		results[i], err = testutils.SignCommitteeVerifierMessage([32]byte(msgID), ccvSigners, committeeThreshold)
		require.NoError(t, err)
		msgs[i] = msg

		bufferPDA := bufferExecInputs(ctx, t, client, payer, ccip_offramp.ExecutionInputsV2{
			EncodedMessage:  encoded,
			Ccvs:            []solana.PublicKey{config.CommitteeVerifierProgram},
			VerifierResults: [][]byte{results[i]},
		})
		resolved, err := ccip.ResolveExecAccountsWithBuffer(ctx, client, config.CcipOfframpProgram, payer.PublicKey(), bufferPDA, lggr)
		require.NoError(t, err)
		ixs[i] = resolved.Ix
		lutMap, err := common.GetAddressLookupTableMap(ctx, client, resolved.LookupTables)
		require.NoError(t, err)
		maps.Copy(luts, lutMap)
	}

	lp := newOfframpLogPoller(t, client)
	require.NoError(t, lp.Start(ctx))
	t.Cleanup(func() { require.NoError(t, lp.Close()) })

	poller, err := destinationreader.NewExecutionAttemptPoller(destinationreader.ExecutionAttemptPollerParams{
		Lggr:             lggr,
		LogPoller:        lp,
		RPC:              directRPC{client},
		OfframpProgramID: config.CcipOfframpProgram,
		Config:           destinationreader.Config{PollInterval: *commonconfig.MustNewDuration(300 * time.Millisecond)},
	})
	require.NoError(t, err)
	require.NoError(t, poller.Start(ctx))
	t.Cleanup(func() { require.NoError(t, poller.Close()) })

	res := testutils.SendAndConfirmWithLookupTables(ctx, t, client, ixs, payer, config.DefaultCommitment, luts,
		common.AddComputeUnitLimit(1_400_000)) // usually enough for two ixs
	require.Nil(t, res.Meta.Err)

	for i, msg := range msgs {
		require.Eventually(t, func() bool {
			attempts, pollErr := poller.GetAttempts(msg)
			return pollErr == nil && len(attempts) == 1
		}, 30*time.Second, 500*time.Millisecond, "poller must cache the attempt of message %d", i)
		attempts, err := poller.GetAttempts(msg)
		require.NoError(t, err)
		require.Equal(t, [][]byte{results[i]}, attempts[0].Report.CCVData)
	}
}

func TestExecutionAttemptPoller_MultiSlotIngestion(t *testing.T) {
	env := testutils.NewTestEnv(t).WithTrafficConfig().Build()
	ctx := env.Ctx()
	client := env.Client
	lggr := logger.Test(t)
	payer := testutils.User

	ccvSigners := configureCommitteeVerifier(ctx, t, client)
	msgs := make([]protocol.Message, 2)
	execInputs := make([]ccip_offramp.ExecutionInputsV2, 2)
	resolved := make([]ccip.ResolveAccountsResult, 2)
	luts := make([]map[solana.PublicKey]solana.PublicKeySlice, 2)
	for i := range msgs {
		msg := testutils.MinimalMessageV1(config.EvmChainSelector)
		msg.SequenceNumber += protocol.SequenceNumber(i)
		encoded, err := msg.Encode()
		require.NoError(t, err)
		msgID, err := msg.MessageID()
		require.NoError(t, err)
		results, err := testutils.SignCommitteeVerifierMessage([32]byte(msgID), ccvSigners, committeeThreshold)
		require.NoError(t, err)
		msgs[i] = msg
		execInputs[i] = ccip_offramp.ExecutionInputsV2{
			EncodedMessage:  encoded,
			Ccvs:            []solana.PublicKey{config.CommitteeVerifierProgram},
			VerifierResults: [][]byte{results},
		}
		resolved[i], err = ccip.ResolveExecAccounts(ctx, client, config.CcipOfframpProgram, execInputs[i], payer.PublicKey(), lggr)
		require.NoError(t, err)
		luts[i], err = common.GetAddressLookupTableMap(ctx, client, resolved[i].LookupTables)
		require.NoError(t, err)
	}

	for i := range msgs {
		res := testutils.SendAndConfirmWithLookupTables(ctx, t, client,
			[]solana.Instruction{resolved[i].Ix}, payer, config.DefaultCommitment, luts[i],
			common.AddComputeUnitLimit(honestCU))
		require.Nil(t, res.Meta.Err)
		if i == 0 {
			slot, err := client.GetSlot(ctx, rpc.CommitmentFinalized)
			require.NoError(t, err)
			require.Eventually(t, func() bool {
				next, err := client.GetSlot(ctx, rpc.CommitmentFinalized)
				return err == nil && next > slot
			}, 30*time.Second, 500*time.Millisecond, "finalized slot must advance before the second execution")
		}
	}

	lp := newOfframpLogPoller(t, client)
	require.NoError(t, lp.Start(ctx))
	t.Cleanup(func() { require.NoError(t, lp.Close()) })

	poller, err := destinationreader.NewExecutionAttemptPoller(destinationreader.ExecutionAttemptPollerParams{
		Lggr:             lggr,
		LogPoller:        lp,
		RPC:              directRPC{client},
		OfframpProgramID: config.CcipOfframpProgram,
		Config: destinationreader.Config{
			PollInterval:   *commonconfig.MustNewDuration(300 * time.Millisecond),
			LookbackWindow: *commonconfig.MustNewDuration(time.Minute),
		},
	})
	require.NoError(t, err)
	require.NoError(t, poller.Start(ctx))
	t.Cleanup(func() { require.NoError(t, poller.Close()) })

	for i, msg := range msgs {
		require.Eventually(t, func() bool {
			attempts, pollErr := poller.GetAttempts(msg)
			return pollErr == nil && len(attempts) == 1
		}, 30*time.Second, 500*time.Millisecond, "poller must cache the attempt of message %d", i)
	}
	require.Eventually(t, func() bool {
		return poller.Ready() == nil
	}, 30*time.Second, 500*time.Millisecond, "poller must report ready after catching up")
}

// bufferExecInputs writes the execution inputs to a buffer PDA in one chunk and returns the PDA.
func bufferExecInputs(ctx context.Context, t *testing.T, client *rpc.Client, payer solana.PrivateKey, execInputs ccip_offramp.ExecutionInputsV2) solana.PublicKey {
	t.Helper()
	data, err := execInputs.Marshal()
	require.NoError(t, err)
	var bufferID [32]byte
	copy(bufferID[:], eth.Keccak256(execInputs.EncodedMessage))
	bufferPDA, _, err := state.FindExecutionInputsBufferPDA(bufferID, payer.PublicKey(), config.CcipOfframpProgram)
	require.NoError(t, err)
	ix, err := ccip_offramp.NewBufferExecutionInputsInstruction(
		ccip_offramp.BufferExecutionInputsParams{
			BufferId:    bufferID,
			TotalLength: uint32(len(data)), //nolint:gosec // G115
			Chunk:       data,
			ChunkIndex:  0,
			NumChunks:   1,
		},
		bufferPDA,
		config.OfframpConfigPDA,
		payer.PublicKey(),
		solana.SystemProgramID,
		config.OfframpEventAuthorityPDA,
		config.CcipOfframpProgram)
	require.NoError(t, err)
	testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{ix}, payer, config.DefaultCommitment)
	return bufferPDA
}

// directRPC adapts *rpc.Client to destinationreader.RPCClient (commitment-aware
// signatures). Mirrors the adapter used by the package's unit tests.
type directRPC struct {
	*rpc.Client
}

func (c directRPC) GetAccountInfo(ctx context.Context, addr solana.PublicKey, commitment rpc.CommitmentType) (*rpc.GetAccountInfoResult, error) {
	return c.GetAccountInfoWithOpts(ctx, addr, &rpc.GetAccountInfoOpts{Commitment: commitment})
}

func (c directRPC) GetTransaction(ctx context.Context, sig solana.Signature) (*rpc.GetTransactionResult, error) {
	maxTxVersion := uint64(0)
	return c.Client.GetTransaction(ctx, sig, &rpc.GetTransactionOpts{
		Commitment:                     rpc.CommitmentFinalized,
		Encoding:                       solana.EncodingBase64,
		MaxSupportedTransactionVersion: &maxTxVersion,
	})
}

// TestDestinationReader_PlantThenHonest pins the failed-attempt handling end to end.
// An execute_v2 that reverts after emitting InProgress (the signers are not in the
// configured set, so the verify CPI fails) is cached as a failed attempt, but it is
// not skip evidence: its blobs are not the executor's, so it never triggers a
// simulation. The executor's own attempt then succeeds and is terminal evidence.
func TestDestinationReader_PlantThenHonest(t *testing.T) {
	env := testutils.NewTestEnv(t).WithTrafficConfig().Build()
	ctx := env.Ctx()
	client := env.Client
	lggr := logger.Test(t)

	ccvSigners := configureCommitteeVerifier(ctx, t, client)
	msg, encoded, validResults := newSignedMessage(t, ccvSigners)

	simRPC := &simCountingRPC{RPCClient: directRPC{client}}
	dr, err := destinationreader.NewDestinationReader(destinationreader.Params{
		Lggr:             lggr,
		ChainSelector:    msg.DestChainSelector,
		RPC:              simRPC,
		OfframpProgramID: config.CcipOfframpProgram,
		RMNRemoteAddress: config.RMNRemoteProgram,
		LogPoller:        newOfframpLogPoller(t, client),
	})
	require.NoError(t, err)
	require.NoError(t, dr.Start(ctx))
	t.Cleanup(func() { require.NoError(t, dr.Close()) })

	// The plant: structurally valid signatures over the same message ID, from keys
	// outside the configured set.
	msgID, err := msg.MessageID()
	require.NoError(t, err)
	badKeys, err := eth.GenerateEthPrivateKeys(committeeThreshold)
	require.NoError(t, err)
	badSigners, err := eth.GetEvmSigners(badKeys)
	require.NoError(t, err)
	plantResults, err := testutils.SignCommitteeVerifierMessage([32]byte(msgID), badSigners, committeeThreshold)
	require.NoError(t, err)

	plantResolved, err := ccip.ResolveExecAccounts(ctx, client, config.CcipOfframpProgram, ccip_offramp.ExecutionInputsV2{
		EncodedMessage:  encoded,
		Ccvs:            []solana.PublicKey{config.CommitteeVerifierProgram},
		VerifierResults: [][]byte{plantResults},
	}, testutils.AnotherUser.PublicKey(), lggr)
	require.NoError(t, err)
	plantLuts, err := common.GetAddressLookupTableMap(ctx, client, plantResolved.LookupTables)
	require.NoError(t, err)
	testutils.SendAndFailWithLookupTables(ctx, t, client, []solana.Instruction{plantResolved.Ix},
		testutils.AnotherUser, config.DefaultCommitment, plantLuts, []string{"Unauthorized signer"},
		common.AddComputeUnitLimit(honestCU))

	// The reverted plant is cached, but with blobs that are not the executor's.
	require.Eventually(t, func() bool {
		attempts, pollErr := dr.GetExecutionAttempts(ctx, msg)
		return pollErr == nil && len(attempts) == 1
	}, 30*time.Second, 500*time.Millisecond, "the reverted plant must be cached")

	ccvAddr := protocol.UnknownAddress(config.CommitteeVerifierProgram.Bytes())
	ccvInfo := protocol.CCVAddressInfo{RequiredCCVs: []protocol.UnknownAddress{ccvAddr}}
	honestResults := []protocol.VerifierResult{{VerifierDestAddress: ccvAddr, CCVData: validResults}}

	executed, err := dr.GetMessageSuccess(ctx, msg)
	require.NoError(t, err)
	require.False(t, executed, "a reverted plant executes nothing")

	honest, err := dr.HasHonestAttempt(ctx, msg, honestResults, ccvInfo)
	require.NoError(t, err)
	require.False(t, honest.Honest, "a failed attempt with foreign blobs is not skip evidence")
	require.Zero(t, simRPC.sims.Load(), "no simulation without a matching failed attempt")

	// The honest attempt lands and becomes terminal evidence.
	honestResolved, err := ccip.ResolveExecAccounts(ctx, client, config.CcipOfframpProgram, ccip_offramp.ExecutionInputsV2{
		EncodedMessage:  encoded,
		Ccvs:            []solana.PublicKey{config.CommitteeVerifierProgram},
		VerifierResults: [][]byte{validResults},
	}, testutils.User.PublicKey(), lggr)
	require.NoError(t, err)
	honestLuts, err := common.GetAddressLookupTableMap(ctx, client, honestResolved.LookupTables)
	require.NoError(t, err)
	testutils.SendAndConfirmWithLookupTables(ctx, t, client, []solana.Instruction{honestResolved.Ix},
		testutils.User, config.DefaultCommitment, honestLuts, common.AddComputeUnitLimit(honestCU))

	// The state PDA is read at Finalized, so allow for the finality lag.
	require.Eventually(t, func() bool {
		executed, execErr := dr.GetMessageSuccess(ctx, msg)
		return execErr == nil && executed
	}, 30*time.Second, 500*time.Millisecond, "the honest attempt must execute the message")

	require.Eventually(t, func() bool {
		attempts, pollErr := dr.GetExecutionAttempts(ctx, msg)
		return pollErr == nil && len(attempts) == 2
	}, 30*time.Second, 500*time.Millisecond, "the honest attempt must be cached alongside the failed plant")

	honest, err = dr.HasHonestAttempt(ctx, msg, honestResults, ccvInfo)
	require.NoError(t, err)
	require.True(t, honest.Honest, "the successful attempt is terminal evidence")
	require.Zero(t, simRPC.sims.Load(), "a successful attempt short-circuits before simulation")
}

// TestDestinationReader_VerificationRevertIsHonest pins that a matching failed attempt is
// skip evidence when the executor's own simulation reverts in CCV verification, as on EVM.
func TestDestinationReader_VerificationRevertIsHonest(t *testing.T) {
	env := testutils.NewTestEnv(t).WithTrafficConfig().Build()
	ctx := env.Ctx()
	client := env.Client
	lggr := logger.Test(t)

	ccvSigners := newCommitteeSigners(t)
	// Without the data payload the tx fits inline with 3 signatures, so the reader simulates it.
	msg := testutils.MinimalMessageV1(config.EvmChainSelector)
	msg.Data, msg.DataLength = nil, 0
	msg, encoded, results := signMessage(t, ccvSigners, msg)

	simRPC := &simCountingRPC{RPCClient: directRPC{client}}
	dr, err := destinationreader.NewDestinationReader(destinationreader.Params{
		Lggr:             lggr,
		ChainSelector:    msg.DestChainSelector,
		RPC:              simRPC,
		OfframpProgramID: config.CcipOfframpProgram,
		RMNRemoteAddress: config.RMNRemoteProgram,
		LogPoller:        newOfframpLogPoller(t, client),
	})
	require.NoError(t, err)
	dr.SetSimulationFeePayer(testutils.User.PublicKey())
	require.NoError(t, dr.Start(ctx))
	t.Cleanup(func() { require.NoError(t, dr.Close()) })

	ccvAddr := protocol.UnknownAddress(config.CommitteeVerifierProgram.Bytes())
	ccvInfo := protocol.CCVAddressInfo{RequiredCCVs: []protocol.UnknownAddress{ccvAddr}}
	honestResults := []protocol.VerifierResult{{VerifierDestAddress: ccvAddr, CCVData: results}}

	resolved, err := ccip.ResolveExecAccounts(ctx, client, config.CcipOfframpProgram, ccip_offramp.ExecutionInputsV2{
		EncodedMessage:  encoded,
		Ccvs:            []solana.PublicKey{config.CommitteeVerifierProgram},
		VerifierResults: [][]byte{results},
	}, testutils.AnotherUser.PublicKey(), lggr)
	require.NoError(t, err)
	luts, err := common.GetAddressLookupTableMap(ctx, client, resolved.LookupTables)
	require.NoError(t, err)
	testutils.SendAndFailWithLookupTables(ctx, t, client, []solana.Instruction{resolved.Ix},
		testutils.AnotherUser, config.DefaultCommitment, luts, []string{"SourceNotConfigured"},
		common.AddComputeUnitLimit(honestCU))

	require.Eventually(t, func() bool {
		attempts, pollErr := dr.GetExecutionAttempts(ctx, msg)
		return pollErr == nil && len(attempts) == 1
	}, 30*time.Second, 500*time.Millisecond, "the failed attempt must be cached")

	honest, err := dr.HasHonestAttempt(ctx, msg, honestResults, ccvInfo)
	require.NoError(t, err)
	require.True(t, honest.Honest, "a verification revert in the simulation is skip evidence")
	require.Positive(t, simRPC.sims.Load(), "a matching failed attempt must trigger a simulation")

	configureCommitteeVerifierSigners(ctx, t, client, ccvSigners)

	honestResolved, err := ccip.ResolveExecAccounts(ctx, client, config.CcipOfframpProgram, ccip_offramp.ExecutionInputsV2{
		EncodedMessage:  encoded,
		Ccvs:            []solana.PublicKey{config.CommitteeVerifierProgram},
		VerifierResults: [][]byte{results},
	}, testutils.User.PublicKey(), lggr)
	require.NoError(t, err)
	honestLuts, err := common.GetAddressLookupTableMap(ctx, client, honestResolved.LookupTables)
	require.NoError(t, err)
	testutils.SendAndConfirmWithLookupTables(ctx, t, client, []solana.Instruction{honestResolved.Ix},
		testutils.User, config.DefaultCommitment, honestLuts, common.AddComputeUnitLimit(honestCU))

	require.Eventually(t, func() bool {
		honest, hErr := dr.HasHonestAttempt(ctx, msg, honestResults, ccvInfo)
		return hErr == nil && honest.Honest
	}, 30*time.Second, 500*time.Millisecond, "the successful attempt is terminal evidence")
}

// TestDestinationReader_ReceiverRevertIsHonest pins that a matching failed attempt is skip
// evidence when the executor's own simulation reverts in the receiver, as on EVM, and that
// the same message is not skipped when the simulation cannot run.
func TestDestinationReader_ReceiverRevertIsHonest(t *testing.T) {
	t.Run("funded_fee_payer", func(t *testing.T) { testReceiverRevert(t, testutils.User.PublicKey(), true) })
	t.Run("unfunded_fee_payer", func(t *testing.T) { testReceiverRevert(t, solana.NewWallet().PublicKey(), false) })
}

func testReceiverRevert(t *testing.T, simFeePayer solana.PublicKey, wantHonest bool) {
	env := testutils.NewTestEnv(t).WithTrafficConfig().Build()
	ctx := env.Ctx()
	client := env.Client
	lggr := logger.Test(t)

	ccvSigners := newCommitteeSigners(t)
	configureCommitteeVerifierSigners(ctx, t, client, ccvSigners)
	msg := testutils.MinimalMessageV1(config.EvmChainSelector)
	msg.Data, msg.DataLength = nil, 0
	msg, encoded, results := signMessage(t, ccvSigners, msg)

	// The env is per test, so reject_all needs no reset.
	rejectIx, err := test_ccip_receiver.NewSetRejectAllInstruction(true, config.ReceiverTargetAccountPDA, testutils.User.PublicKey())
	require.NoError(t, err)
	testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{rejectIx}, testutils.User, config.DefaultCommitment)

	simRPC := &simCountingRPC{RPCClient: directRPC{client}}
	dr, err := destinationreader.NewDestinationReader(destinationreader.Params{
		Lggr:             lggr,
		ChainSelector:    msg.DestChainSelector,
		RPC:              simRPC,
		OfframpProgramID: config.CcipOfframpProgram,
		RMNRemoteAddress: config.RMNRemoteProgram,
		LogPoller:        newOfframpLogPoller(t, client),
	})
	require.NoError(t, err)
	dr.SetSimulationFeePayer(simFeePayer)
	require.NoError(t, dr.Start(ctx))
	t.Cleanup(func() { require.NoError(t, dr.Close()) })

	ccvAddr := protocol.UnknownAddress(config.CommitteeVerifierProgram.Bytes())
	ccvInfo := protocol.CCVAddressInfo{RequiredCCVs: []protocol.UnknownAddress{ccvAddr}}
	honestResults := []protocol.VerifierResult{{VerifierDestAddress: ccvAddr, CCVData: results}}

	resolved, err := ccip.ResolveExecAccounts(ctx, client, config.CcipOfframpProgram, ccip_offramp.ExecutionInputsV2{
		EncodedMessage:  encoded,
		Ccvs:            []solana.PublicKey{config.CommitteeVerifierProgram},
		VerifierResults: [][]byte{results},
	}, testutils.AnotherUser.PublicKey(), lggr)
	require.NoError(t, err)
	luts, err := common.GetAddressLookupTableMap(ctx, client, resolved.LookupTables)
	require.NoError(t, err)
	testutils.SendAndFailWithLookupTables(ctx, t, client, []solana.Instruction{resolved.Ix},
		testutils.AnotherUser, config.DefaultCommitment, luts, []string{"RejectAll"},
		common.AddComputeUnitLimit(honestCU))

	require.Eventually(t, func() bool {
		attempts, pollErr := dr.GetExecutionAttempts(ctx, msg)
		return pollErr == nil && len(attempts) == 1
	}, 30*time.Second, 500*time.Millisecond, "the failed attempt must be cached")

	// An unfunded fee payer fails account resolution, which must not skip the message.
	honest, err := dr.HasHonestAttempt(ctx, msg, honestResults, ccvInfo)
	require.NoError(t, err)
	require.Equal(t, wantHonest, honest.Honest)
	require.Positive(t, simRPC.sims.Load(), "a matching failed attempt must trigger a simulation")
}

// TestDestinationReader_AppendedInstructionFailureIsNotHonest pins that a failed attempt
// is not skip evidence when the executor's own execute_v2 simulation passes. The failed
// attempt includes a valid execute_v2 followed by a failing instruction, which is a cheap
// way to emit InProgress and Success and still fail the transaction.
func TestDestinationReader_AppendedInstructionFailureIsNotHonest(t *testing.T) {
	env := testutils.NewTestEnv(t).WithTrafficConfig().Build()
	ctx := env.Ctx()
	client := env.Client
	lggr := logger.Test(t)

	ccvSigners := newCommitteeSigners(t)
	configureCommitteeVerifierSigners(ctx, t, client, ccvSigners)
	msg := testutils.MinimalMessageV1(config.EvmChainSelector)
	msg.Data, msg.DataLength = nil, 0
	msg, encoded, results := signMessage(t, ccvSigners, msg)

	simRPC := &simCountingRPC{RPCClient: directRPC{client}}
	dr, err := destinationreader.NewDestinationReader(destinationreader.Params{
		Lggr:             lggr,
		ChainSelector:    msg.DestChainSelector,
		RPC:              simRPC,
		OfframpProgramID: config.CcipOfframpProgram,
		RMNRemoteAddress: config.RMNRemoteProgram,
		LogPoller:        newOfframpLogPoller(t, client),
	})
	require.NoError(t, err)
	dr.SetSimulationFeePayer(testutils.User.PublicKey())
	require.NoError(t, dr.Start(ctx))
	t.Cleanup(func() { require.NoError(t, dr.Close()) })

	resolved, err := ccip.ResolveExecAccounts(ctx, client, config.CcipOfframpProgram, ccip_offramp.ExecutionInputsV2{
		EncodedMessage:  encoded,
		Ccvs:            []solana.PublicKey{config.CommitteeVerifierProgram},
		VerifierResults: [][]byte{results},
	}, testutils.AnotherUser.PublicKey(), lggr)
	require.NoError(t, err)
	luts, err := common.GetAddressLookupTableMap(ctx, client, resolved.LookupTables)
	require.NoError(t, err)

	// The execute succeeds, then the trailing instruction fails on the owner check.
	// The transaction reverts, but the InProgress and Success events stay in the logs.
	trailingIx, err := test_ccip_receiver.NewSetRejectAllInstruction(true, config.ReceiverTargetAccountPDA, testutils.AnotherUser.PublicKey())
	require.NoError(t, err)
	testutils.SendAndFailWithLookupTables(ctx, t, client,
		[]solana.Instruction{resolved.Ix, trailingIx},
		testutils.AnotherUser, config.DefaultCommitment, luts, []string{"OnlyOwner"},
		common.AddComputeUnitLimit(honestCU))

	require.Eventually(t, func() bool {
		attempts, pollErr := dr.GetExecutionAttempts(ctx, msg)
		return pollErr == nil && len(attempts) == 1
	}, 30*time.Second, 500*time.Millisecond, "the failed attempt must be cached")

	ccvAddr := protocol.UnknownAddress(config.CommitteeVerifierProgram.Bytes())
	ccvInfo := protocol.CCVAddressInfo{RequiredCCVs: []protocol.UnknownAddress{ccvAddr}}
	honestResults := []protocol.VerifierResult{{VerifierDestAddress: ccvAddr, CCVData: results}}

	honest, err := dr.HasHonestAttempt(ctx, msg, honestResults, ccvInfo)
	require.NoError(t, err)
	require.False(t, honest.Honest, "a passing simulation must not trust the failed attempt")
	require.Positive(t, simRPC.sims.Load(), "a matching failed attempt must trigger a simulation")
	// The execute simulation itself must have run and returned a successful
	// result. A resolver failure also gives a positive total and must not
	// satisfy this check.
	require.Equal(t, int32(1), simRPC.executeSims.Load(), "exactly one execute simulation must run")
	res := simRPC.executeRes()
	require.NotNil(t, res, "the execute simulation must return a response")
	require.NotNil(t, res.Value, "the execute simulation must return a result")
	require.NoError(t, simRPC.executeRPCErr(), "the execute simulation must not fail on the RPC call")
	require.Nil(t, res.Value.Err, "the execute simulation must succeed on-chain")
}

const committeeThreshold = 3

// honestCU is the budget an honest executor attaches (and the poller's gas gate):
// ExecutionGasLimit of MinimalMessageV1.
const honestCU = 500_000

// configureCommitteeVerifier sets a threshold-of-5 signer set for the EVM lane on the
// committee verifier, the lane default whose verify CPI runs on execute.
func configureCommitteeVerifier(ctx context.Context, t *testing.T, client *rpc.Client) []eth.Signer {
	t.Helper()
	ccvSigners := newCommitteeSigners(t)
	configureCommitteeVerifierSigners(ctx, t, client, ccvSigners)
	return ccvSigners
}

func configureCommitteeVerifierSigners(ctx context.Context, t *testing.T, client *rpc.Client, ccvSigners []eth.Signer) {
	t.Helper()
	signerAddresses := make([][20]uint8, len(ccvSigners))
	for i, s := range ccvSigners {
		signerAddresses[i] = s.Address
	}

	admin := testutils.Admin
	lggr := logger.Test(t)
	resolvedChainConfig, err := ccip.ResolveCVEditChainConfigAccounts(ctx, client, config.CommitteeVerifierProgram, committee_verifier.SetChainConfigParams{
		RemoteChainSelector: config.EvmChainSelector,
		ChainConfig:         committee_verifier.RemoteChainAdminConfig{RouterSigner: config.CCVSignerPDA},
	}, admin.PublicKey(), lggr)
	require.NoError(t, err)
	testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{resolvedChainConfig.Ix}, admin, config.DefaultCommitment)

	resolvedSigners, err := ccip.ResolveCVSetSignerConfigAccounts(ctx, client, config.CommitteeVerifierProgram, committee_verifier.SetSignerConfigParams{
		SourceChainSelector: config.EvmChainSelector,
		Threshold:           committeeThreshold,
		Signers:             signerAddresses,
	}, admin.PublicKey(), lggr)
	require.NoError(t, err)
	testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{resolvedSigners.Ix}, admin, config.DefaultCommitment)
}

func newCommitteeSigners(t *testing.T) []eth.Signer {
	t.Helper()
	privateKeys, err := eth.GenerateEthPrivateKeys(5)
	require.NoError(t, err)
	ccvSigners, err := eth.GetEvmSigners(privateKeys)
	require.NoError(t, err)
	return ccvSigners
}

// newSignedMessage builds a minimal message for the EVM lane plus a valid committee
// verifier result over its ID (keccak256 of the canonical encoding).
func newSignedMessage(t *testing.T, signers []eth.Signer) (protocol.Message, []byte, []byte) {
	t.Helper()
	return signMessage(t, signers, testutils.MinimalMessageV1(config.EvmChainSelector))
}

func signMessage(t *testing.T, signers []eth.Signer, msg protocol.Message) (protocol.Message, []byte, []byte) {
	t.Helper()
	encoded, err := msg.Encode()
	require.NoError(t, err)
	msgID, err := msg.MessageID()
	require.NoError(t, err)
	verifierResults, err := testutils.SignCommitteeVerifierMessage([32]byte(msgID), signers, committeeThreshold)
	require.NoError(t, err)
	return msg, encoded, verifierResults
}

// newOfframpLogPoller mirrors the factory wiring for the execution-attempt poller:
// IncludeReverted is set because failed txs carry the InProgress events it consumes.
func newOfframpLogPoller(t *testing.T, client *rpc.Client) logpoller.Service {
	t.Helper()
	cfg := logpoller.DefaultConfig()
	cfg.IncludeReverted = true
	lp, err := logpoller.NewService(client, config.CcipOfframpProgram, "destreader-integration", logger.TestSugared(t), cfg)
	require.NoError(t, err)
	return lp
}

// simCountingRPC counts simulations so tests can assert none happened, and
// tracks the execute simulation separately: only the execute simulation tx
// carries a compute-budget SetComputeUnitPrice instruction. The account
// resolver's read-only round carries limit and heap-frame instructions only,
// so a resolver error cannot satisfy the execute assertions.
type simCountingRPC struct {
	destinationreader.RPCClient
	sims        atomic.Int32
	executeSims atomic.Int32
	mu          sync.Mutex
	execRes     *rpc.SimulateTransactionResponse
	execRPCErr  error
}

func (c *simCountingRPC) SimulateTransactionWithOpts(ctx context.Context, tx *solana.Transaction, opts *rpc.SimulateTransactionOpts) (*rpc.SimulateTransactionResponse, error) {
	c.sims.Add(1)
	res, err := c.RPCClient.SimulateTransactionWithOpts(ctx, tx, opts)
	if hasComputeUnitPriceIx(tx) {
		c.executeSims.Add(1)
		c.mu.Lock()
		c.execRes, c.execRPCErr = res, err
		c.mu.Unlock()
	}
	return res, err
}

func (c *simCountingRPC) executeRPCErr() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.execRPCErr
}

func (c *simCountingRPC) executeRes() *rpc.SimulateTransactionResponse {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.execRes
}

// hasComputeUnitPriceIx reports whether the tx carries a compute-budget
// SetComputeUnitPrice instruction.
func hasComputeUnitPriceIx(tx *solana.Transaction) bool {
	for _, ix := range tx.Message.Instructions {
		if !tx.Message.AccountKeys[ix.ProgramIDIndex].Equals(computebudget.ProgramID) {
			continue
		}
		if _, err := fees.ParseComputeUnitPrice([]byte(ix.Data)); err == nil {
			return true
		}
	}
	return false
}
