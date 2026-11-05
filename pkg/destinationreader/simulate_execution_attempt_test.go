package destinationreader

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-ccv/protocol"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/ccip_offramp"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/ccip"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
)

// These cover the simulation arm of HasHonestAttempt. A messaging-only message needs two
// simulate calls (one resolver round, one execution simulation) and zero account reads.

// simFixture is the single-CCV world every simulation-arm test uses: one CCV address,
// its blob and verifier result, and a message carrying a real CU budget. The shared
// buildTestMessage fixture has zero gas limits, which the simulated submission rejects.
type simFixture struct {
	msg      protocol.Message
	ccvA     protocol.UnknownAddress
	dataA    []byte
	vResultA protocol.VerifierResult
}

func newSimFixture(t *testing.T, destSel protocol.ChainSelector) simFixture {
	t.Helper()
	ccvA, err := protocol.NewUnknownAddressFromHex("0x" + hex32('a'))
	require.NoError(t, err)
	dataA := []byte("verifier-a-blob")
	msg := buildTestMessage(t, destSel)
	msg.ExecutionGasLimit = 200_000
	msg.CcipReceiveGasLimit = 100_000
	return simFixture{
		msg:      msg,
		ccvA:     ccvA,
		dataA:    dataA,
		vResultA: protocol.VerifierResult{VerifierDestAddress: ccvA, CCVData: dataA},
	}
}

// seedAttemptWithBlobs places an attempt in the poller cache of the given outcome
// (failed → failedAttemptCache), with a valid CU budget and the CCV data the executor
// is about to submit.
func seedAttemptWithBlobs(t *testing.T, dr *DestinationReader, msg protocol.Message, failed bool, ccvs []protocol.UnknownAddress, ccvData [][]byte, extra ...protocol.ExecutionAttempt) {
	t.Helper()
	msgID, err := msg.MessageID()
	require.NoError(t, err)
	cache := make([]protocol.ExecutionAttempt, 0, 1+len(extra))
	cache = append(cache, newAttemptFor(t, msg, ccvs, ccvData))
	cache = append(cache, extra...)
	if failed {
		dr.executionAttemptPoller.failedAttemptCache.Add(msgID, cache)
		return
	}
	dr.executionAttemptPoller.attemptCache.Add(msgID, cache)
}

func TestHasHonestAttempt_SimulationArm(t *testing.T) {
	t.Parallel()
	const destSel = protocol.ChainSelector(9837465928374658293)

	fx := newSimFixture(t, destSel)
	okResolve := simResponse(simReturnLog(t, testOfframpProgramID, marshalAnchor(t, resolveDone())), nil)

	tests := []struct {
		name         string
		simResults   []*rpc.SimulateTransactionResponse
		withFeePayer bool
		want         bool
		wantErr      string
		wantSims     int32
	}{
		{
			name:         "failed_cached_attempt_with_tx_level_error_returns_error",
			simResults:   []*rpc.SimulateTransactionResponse{okResolve, simResponse(nil, "InsufficientFundsForFee")},
			withFeePayer: true,
			wantErr:      "InsufficientFundsForFee",
			wantSims:     2,
		},
		{
			name:         "execute_instruction_custom_error_is_honest",
			simResults:   []*rpc.SimulateTransactionResponse{okResolve, simResponse(nil, ixErr(executeIx, map[string]any{"Custom": json.Number("6000")}))},
			withFeePayer: true,
			want:         true,
			wantSims:     2,
		},
		{
			name:         "execute_instruction_budget_exceeded_is_honest",
			simResults:   []*rpc.SimulateTransactionResponse{okResolve, simResponse(nil, ixErr(executeIx, "ComputationalBudgetExceeded"))},
			withFeePayer: true,
			want:         true,
			wantSims:     2,
		},
		{
			name:         "other_instruction_error_returns_error",
			simResults:   []*rpc.SimulateTransactionResponse{okResolve, simResponse(nil, ixErr(executeIx-1, "ComputationalBudgetExceeded"))},
			withFeePayer: true,
			wantErr:      "simulated execution failed",
			wantSims:     2,
		},
		{
			name:         "malformed_instruction_error_returns_error",
			simResults:   []*rpc.SimulateTransactionResponse{okResolve, simResponse(nil, map[string]any{"InstructionError": []any{"1", "Custom"}})},
			withFeePayer: true,
			wantErr:      "simulated execution failed",
			wantSims:     2,
		},
		{
			name:         "failed_cached_attempt_with_passing_sim_is_not_honest",
			simResults:   []*rpc.SimulateTransactionResponse{okResolve, simResponse(nil, nil)},
			withFeePayer: true,
			want:         false,
			wantSims:     2,
		},
		{
			name:         "failed_cached_attempt_with_rpc_error_proceeds",
			simResults:   nil,
			withFeePayer: true,
			want:         false,
			wantSims:     1,
		},
		{
			name:         "fee_payer_not_configured_proceeds_without_rpc",
			simResults:   nil,
			withFeePayer: false,
			want:         false,
			wantSims:     0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rpcClient, calls := newSimClient(t, tc.simResults...)
			dr := newReaderWithRPC(t, rpcClient, tc.withFeePayer)

			seedAttemptWithBlobs(t, dr, fx.msg, true, []protocol.UnknownAddress{fx.ccvA}, [][]byte{fx.dataA})

			got, hErr := dr.HasHonestAttempt(context.Background(), fx.msg, []protocol.VerifierResult{fx.vResultA},
				protocol.CCVAddressInfo{RequiredCCVs: []protocol.UnknownAddress{fx.ccvA}})
			if tc.wantErr != "" {
				require.ErrorContains(t, hErr, tc.wantErr)
			} else {
				require.NoError(t, hErr)
			}
			require.Equal(t, tc.want, got.Honest)
			require.Equal(t, !tc.want && tc.wantErr == "", got.NotHonestAttemptExists)
			require.Equal(t, tc.wantSims, calls.Load())
		})
	}
}

// executeIx is the index of execute_v2 in the simulated transaction: the compute budget
// instruction comes first, as in TXM.
const executeIx = 1

// ixErr builds an error fixture; the RPC decoder converts its numbers to float64.
func ixErr(index int, detail any) map[string]any {
	return map[string]any{"InstructionError": []any{json.Number(strconv.Itoa(index)), detail}}
}

// TestExecuteInstructionReverted_PositionOnly pins the positional classification of a
// simulation error: any instruction error at an execute_v2 position matches, and nothing
// else does. Direct fixtures use float64 indices, as the decoded RPC error carries float64.
func TestExecuteInstructionReverted_PositionOnly(t *testing.T) {
	t.Parallel()

	rpcClient, _ := newSimClient(t)
	dr := newReaderWithRPC(t, rpcClient, true)

	executeData := func() []byte {
		return append([]byte{}, append(ccip_offramp.Instruction_ExecuteV2[:], make([]byte, 8)...)...)
	}
	otherProgram := solana.PublicKey(common.MakeRandom32ByteArray())
	onOfframp := solana.NewInstruction(testOfframpProgramID, nil, []byte{9, 9, 9, 9, 9, 9, 9, 9})
	onOfframpExecute := solana.NewInstruction(testOfframpProgramID, nil, executeData())
	onOtherProgramExecute := solana.NewInstruction(otherProgram, nil, executeData())
	buildTx := func(ixs ...solana.Instruction) *solana.Transaction {
		tx, err := solana.NewTransaction(ixs, solana.Hash{}, solana.TransactionPayer(testOfframpProgramID))
		require.NoError(t, err)
		return tx
	}
	ixErrAt := func(index float64) map[string]any {
		return map[string]any{"InstructionError": []any{index, "Custom"}}
	}

	tests := []struct {
		name string
		tx   *solana.Transaction
		err  any
		want bool
	}{
		{
			name: "error_at_execute_v2_after_other_instructions",
			tx:   buildTx(onOfframp, onOfframpExecute),
			err:  ixErrAt(1),
			want: true,
		},
		{
			name: "error_at_other_instruction_of_same_program",
			tx:   buildTx(onOfframp, onOfframpExecute),
			err:  ixErrAt(0),
			want: false,
		},
		{
			name: "error_at_execute_discriminator_on_other_program",
			tx:   buildTx(onOtherProgramExecute),
			err:  ixErrAt(0),
			want: false,
		},
		{
			name: "tx_without_execute_v2",
			tx:   buildTx(onOfframp),
			err:  ixErrAt(0),
			want: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, dr.executeInstructionReverted(tc.tx, tc.err))
		})
	}
}

// TestHasHonestAttempt_SuccessfulAttemptSkipsSimulation pins that a committed successful
// transaction stays terminal evidence: it must return true without touching the RPC.
func TestHasHonestAttempt_SuccessfulAttemptSkipsSimulation(t *testing.T) {
	t.Parallel()
	const destSel = protocol.ChainSelector(9837465928374658293)

	fx := newSimFixture(t, destSel)

	rpcClient, calls := newSimClient(t)
	dr := newReaderWithRPC(t, rpcClient, true)

	seedAttemptWithBlobs(t, dr, fx.msg, false, []protocol.UnknownAddress{fx.ccvA}, [][]byte{fx.dataA})

	got, hErr := dr.HasHonestAttempt(context.Background(), fx.msg, []protocol.VerifierResult{fx.vResultA},
		protocol.CCVAddressInfo{RequiredCCVs: []protocol.UnknownAddress{fx.ccvA}})
	require.NoError(t, hErr)
	require.True(t, got.Honest)
	require.False(t, got.NotHonestAttemptExists)
	require.Zero(t, calls.Load())
}

// TestHasHonestAttempt_SimulationRunsOnce pins that several matching failed attempts
// still produce a single simulation: the outcome depends only on the message and the
// executor's inputs, which are identical across attempts.
func TestHasHonestAttempt_SimulationRunsOnce(t *testing.T) {
	t.Parallel()
	const destSel = protocol.ChainSelector(9837465928374658293)

	fx := newSimFixture(t, destSel)

	// The simulation passes, so a repeat simulation for the second matching
	// attempt would exceed the expected two calls: one resolver round and one
	// execute simulation.
	rpcClient, calls := newSimClient(t,
		simResponse(simReturnLog(t, testOfframpProgramID, marshalAnchor(t, resolveDone())), nil),
		simResponse(nil, nil),
	)
	dr := newReaderWithRPC(t, rpcClient, true)

	// Both attempts carry the required CCV data, so both match and the poller
	// must simulate only once.
	seedAttemptWithBlobs(t, dr, fx.msg, true, []protocol.UnknownAddress{fx.ccvA}, [][]byte{fx.dataA},
		newAttemptFor(t, fx.msg, []protocol.UnknownAddress{fx.ccvA}, [][]byte{fx.dataA}))

	got, hErr := dr.HasHonestAttempt(context.Background(), fx.msg, []protocol.VerifierResult{fx.vResultA},
		protocol.CCVAddressInfo{RequiredCCVs: []protocol.UnknownAddress{fx.ccvA}})
	require.NoError(t, hErr)
	require.False(t, got.Honest)
	require.True(t, got.NotHonestAttemptExists)
	require.Equal(t, int32(2), calls.Load())
}

// TestBuildExecutionInputsV2 covers the shared input assembly guards directly.
func TestBuildExecutionInputsV2(t *testing.T) {
	t.Parallel()
	t.Run("invalid_ccv_address_length", func(t *testing.T) {
		t.Parallel()
		_, err := ccip.BuildExecutionInputsV2([]byte("msg"), []protocol.UnknownAddress{{0x01}}, [][]byte{[]byte("x")})
		require.ErrorContains(t, err, "invalid CCV address length")
	})
	t.Run("ccv_and_verifier_result_count_mismatch", func(t *testing.T) {
		t.Parallel()
		addr := solana.PublicKey(common.MakeRandom32ByteArray())
		_, err := ccip.BuildExecutionInputsV2([]byte("msg"), []protocol.UnknownAddress{protocol.UnknownAddress(addr.Bytes())}, nil)
		require.ErrorContains(t, err, "1 CCVs but 0 verifier results")
	})
	t.Run("addresses_and_blobs_are_positionally_aligned", func(t *testing.T) {
		t.Parallel()
		addr := solana.PublicKey(common.MakeRandom32ByteArray())
		blob := []byte("blob")
		inputs, err := ccip.BuildExecutionInputsV2([]byte("msg"), []protocol.UnknownAddress{protocol.UnknownAddress(addr.Bytes())}, [][]byte{blob})
		require.NoError(t, err)
		require.Equal(t, addr, inputs.Ccvs[0])
		require.Equal(t, blob, inputs.VerifierResults[0])
	})
}

func TestQuorumInputs(t *testing.T) {
	t.Parallel()
	addr := func(c byte) protocol.UnknownAddress {
		a, err := protocol.NewUnknownAddressFromHex("0x" + hex32(c))
		require.NoError(t, err)
		return a
	}
	req1, req2, opt1, opt2, extra := addr('1'), addr('2'), addr('3'), addr('4'), addr('5')
	result := func(a protocol.UnknownAddress, blob string) protocol.VerifierResult {
		return protocol.VerifierResult{VerifierDestAddress: a, CCVData: []byte(blob)}
	}

	ccvs, data, err := quorumInputs(
		[]protocol.VerifierResult{
			result(extra, "extra"), result(opt2, "opt2"), result(req2, "req2-old"),
			result(opt1, "opt1"), result(req1, "req1"), result(req2, "req2-new"),
		},
		protocol.CCVAddressInfo{
			RequiredCCVs:      []protocol.UnknownAddress{req1, req2},
			OptionalCCVs:      []protocol.UnknownAddress{opt1, opt2},
			OptionalThreshold: 1,
		})
	require.NoError(t, err)
	require.Equal(t, []protocol.UnknownAddress{req1, req2, opt1}, ccvs)
	require.Equal(t, [][]byte{[]byte("req1"), []byte("req2-new"), []byte("opt1")}, data)
}
