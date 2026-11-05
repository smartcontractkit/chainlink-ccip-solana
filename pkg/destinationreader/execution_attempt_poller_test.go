package destinationreader

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-ccv/protocol"
	commonconfig "github.com/smartcontractkit/chainlink-common/pkg/config"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/ccip_offramp"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/committee_verifier"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/fees"
	"github.com/smartcontractkit/chainlink-ccip-solana/internal/mocks"
	"github.com/smartcontractkit/chainlink-ccip-solana/pkg/logpoller"
)

// encodeExecutionStateChangedV2Event returns the bytes placed into logpoller.Event.Data for
// an ExecutionStateChangedV2 emission: 8-byte event discriminator + Borsh-encoded payload.
func encodeExecutionStateChangedV2Event(t *testing.T, ev ccip_offramp.ExecutionStateChangedV2) []byte {
	t.Helper()
	var buf bytes.Buffer
	enc := bin.NewBorshEncoder(&buf)
	require.NoError(t, ev.MarshalWithEncoder(enc))
	out := append([]byte{}, ccip_offramp.Event_ExecutionStateChangedV2[:]...)
	return append(out, buf.Bytes()...)
}

// TestIngestEventPayloadClassification checks the boundary between two cases:
//  1. An offramp event that is not ours: ordinary noise on a program-scoped filter.
//  2. An event that is ours but unreadable: the deployed offramp and the pinned
//     bindings disagree.
//
// In case 2, ingestBatch must log the error, not discard the event silently.
// The error stays permanent (not errTransientIngest): a retry cannot fix a binding mismatch.
func TestIngestEventPayloadClassification(t *testing.T) {
	t.Parallel()

	offrampProgramID := solana.MustPublicKeyFromBase58("HMPYFPRJqiYPUqbkCcAJnyN4hbYqCEBhTvhWNvNwK7Xs")

	withDiscriminator := func(disc [8]byte, body ...byte) []byte {
		return append(append([]byte{}, disc[:]...), body...)
	}

	tests := []struct {
		name    string
		data    []byte
		wantErr bool
	}{
		{
			name: "sibling offramp event is ignored",
			data: withDiscriminator(ccip_offramp.Event_BufferInitialized,
				bytes.Repeat([]byte{0xAB}, 32)...),
		},
		{
			name: "unknown discriminator is ignored",
			data: withDiscriminator([8]byte{9, 9, 9, 9, 9, 9, 9, 9}, 0x01, 0x02),
		},
		{
			name: "empty payload is ignored without panicking",
			data: nil,
		},
		{
			name: "payload shorter than the discriminator is ignored without panicking",
			data: ccip_offramp.Event_ExecutionStateChangedV2[:7],
		},
		// NOTE: The discriminator matches, but the body is garbage (stale bindings).
		{
			name:    "our discriminator with a garbage body is reported",
			data:    withDiscriminator(ccip_offramp.Event_ExecutionStateChangedV2, 0xFF, 0xFF, 0xFF),
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// No RPC stub: classification happens before any tx lookup. Any call
			// into the RPC is itself a failure.
			p := newTestPoller(t, newDefaultLogPollerForDest(t), newMockRPCPollerTxOnly(t,
				func(_ context.Context, _ solana.Signature) (*rpc.GetTransactionResult, error) {
					t.Fatal("RPC must not be consulted for a payload that never decodes")
					return nil, nil
				}), offrampProgramID)

			err := p.ingestEvent(context.Background(), logpoller.Event{
				Data:        tc.data,
				SlotNumber:  77,
				TxSignature: solana.Signature{0x01, 0x02},
			})

			if !tc.wantErr {
				require.NoError(t, err)
				require.Zero(t, p.attemptCache.Len())
				return
			}
			require.Error(t, err)
			require.Contains(t, err.Error(), "decode ExecutionStateChangedV2 at slot 77")
			require.NotErrorIs(t, err, errTransientIngest)
			require.Zero(t, p.attemptCache.Len())
		})
	}
}

// testCCVs are the verifier program IDs embedded in every fixture ExecuteV2 tx,
// positionally aligned with the verifierResults passed to buildExecuteV2Tx.
var testCCVs = []solana.PublicKey{
	solana.MustPublicKeyFromBase58("Ccv1111111111111111111111111111111111111111"),
	solana.MustPublicKeyFromBase58("Ccv2222222222222222222222222222222222222222"),
}

// buildExecuteV2Tx constructs a two-instruction Solana transaction:
//
//	[0] ComputeBudget::SetComputeUnitLimit(cuLimit)   (omitted when cuLimit == 0)
//	[1] ccip_offramp::ExecuteV2(ExecInputs{msg, vResults})
//
// The resulting tx round-trips through solana-go's TransactionResultEnvelope base64
// path so the poller's GetTransaction pipeline decodes a real *solana.Transaction.
func buildExecuteV2Tx(t *testing.T, offrampProgramID solana.PublicKey, msg protocol.Message, verifierResults [][]byte, cuLimit uint32) *solana.Transaction {
	t.Helper()

	encodedMessage, err := msg.Encode()
	require.NoError(t, err)

	// The poller asserts positionally aligned CCVS/CCVData, and the contract requires equal
	// lengths, so keep the fixture aligned: one CCV per verifier result.
	require.LessOrEqual(t, len(verifierResults), len(testCCVs), "fixture: testCCVs must cover one CCV per verifier result")
	params := ccip_offramp.ExecuteParams{
		ExecInputs: &ccip_offramp.ExecutionInputsV2{
			EncodedMessage:  encodedMessage,
			Ccvs:            testCCVs[:len(verifierResults)],
			VerifierResults: verifierResults,
		},
	}
	var paramsBuf bytes.Buffer
	require.NoError(t, params.MarshalWithEncoder(bin.NewBorshEncoder(&paramsBuf)))

	executeIxData := append([]byte{}, ccip_offramp.Instruction_ExecuteV2[:]...)
	executeIxData = append(executeIxData, paramsBuf.Bytes()...)

	// AccountKeys[0] = offramp (target of ExecuteV2), AccountKeys[1] = ComputeBudget.
	accountKeys := []solana.PublicKey{offrampProgramID, fees.ComputeBudgetProgram}

	instructions := make([]solana.CompiledInstruction, 0, 2)
	if cuLimit > 0 {
		cuData, cuErr := fees.ComputeUnitLimit(cuLimit).Data()
		require.NoError(t, cuErr)
		instructions = append(instructions, solana.CompiledInstruction{
			ProgramIDIndex: 1, // ComputeBudget
			Data:           cuData,
		})
	}
	instructions = append(instructions, solana.CompiledInstruction{
		ProgramIDIndex: 0, // offramp
		Data:           executeIxData,
	})

	return &solana.Transaction{
		Message: solana.Message{
			Header: solana.MessageHeader{
				NumReadonlyUnsignedAccounts: uint8(len(accountKeys)), //nolint:gosec // G115: test fixture, account count bounded by the surrounding scenario
			},
			AccountKeys:  accountKeys,
			Instructions: instructions,
		},
	}
}

// buildTxResult wraps a hand-built Solana transaction into a *rpc.GetTransactionResult
// via JSON round-trip. metaErr is placed on Meta.Err so the pipeline can test both the
// Success (Err == nil) and Failure (Err != nil) terminal-outcome paths.
func buildTxResult(t *testing.T, tx *solana.Transaction, metaErr any) *rpc.GetTransactionResult {
	t.Helper()
	txBytes, err := tx.MarshalBinary()
	require.NoError(t, err)

	envelope := new(rpc.TransactionResultEnvelope)
	jsonStr := fmt.Sprintf(`[%q,"base64"]`, base64.StdEncoding.EncodeToString(txBytes))
	require.NoError(t, envelope.UnmarshalJSON([]byte(jsonStr)))

	result := &rpc.GetTransactionResult{
		Transaction: envelope,
		Meta:        &rpc.TransactionMeta{Err: metaErr},
	}
	return result
}

// buildBufferedExecuteV2Tx constructs a transaction whose ExecuteV2 ix carries
// ExecInputs == nil (the buffered form, where inputs live in a buffer account the poller
// never sees). ResolutionMetadata is opaque to the poller, so it is left empty. The
// execute ix's position (0 or 1, after the optional ComputeBudget ix) is returned so the
// caller can key the inner-instruction group correctly.
func buildBufferedExecuteV2Tx(t *testing.T, offrampProgramID solana.PublicKey, cuLimit uint32) (*solana.Transaction, uint16) {
	t.Helper()

	params := ccip_offramp.ExecuteParams{ExecInputs: nil}
	var paramsBuf bytes.Buffer
	require.NoError(t, params.MarshalWithEncoder(bin.NewBorshEncoder(&paramsBuf)))

	executeIxData := append([]byte{}, ccip_offramp.Instruction_ExecuteV2[:]...)
	executeIxData = append(executeIxData, paramsBuf.Bytes()...)

	accountKeys := []solana.PublicKey{offrampProgramID, fees.ComputeBudgetProgram}

	instructions := make([]solana.CompiledInstruction, 0, 2)
	if cuLimit > 0 {
		cuData, cuErr := fees.ComputeUnitLimit(cuLimit).Data()
		require.NoError(t, cuErr)
		instructions = append(instructions, solana.CompiledInstruction{
			ProgramIDIndex: 1, // ComputeBudget
			Data:           cuData,
		})
	}
	execIxIndex := len(instructions)
	instructions = append(instructions, solana.CompiledInstruction{
		ProgramIDIndex: 0, // offramp
		Data:           executeIxData,
	})

	return &solana.Transaction{
		Message: solana.Message{
			Header: solana.MessageHeader{
				NumReadonlyUnsignedAccounts: uint8(len(accountKeys)), //nolint:gosec // G115: test fixture, account count bounded by the surrounding scenario
			},
			AccountKeys:  accountKeys,
			Instructions: instructions,
		},
	}, uint16(execIxIndex) //nolint:gosec // G115: fixture has at most 2 instructions
}

// buildVerifyInnerInstruction builds one verify CPI as recorded in tx meta: data is the
// committee-verifier Verify discriminator + Borsh VerifyParams, the format the on-chain
// call_verify hardcodes for every CCV. programIDIndex identifies the CCV (static key index
// or, for the ALT case, an index beyond the static keys resolved via LoadedAddresses).
func buildVerifyInnerInstruction(t *testing.T, programIDIndex uint16, encodedMessage []byte, messageID [32]byte, verifierResult []byte) rpc.CompiledInstruction {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, committee_verifier.VerifyParams{
		MessageId:       messageID,
		EncodedMessage:  encodedMessage,
		VerifierResults: verifierResult,
	}.MarshalWithEncoder(bin.NewBorshEncoder(&buf)))
	data := append([]byte{}, committee_verifier.Instruction_Verify[:]...)
	return rpc.CompiledInstruction{
		ProgramIDIndex: programIDIndex,
		Data:           solana.Base58(append(data, buf.Bytes()...)),
	}
}

// buildBufferedTxResult pairs a buffered ExecuteV2 tx with meta carrying the given inner
// instructions and ALT-loaded addresses, for the buffered reconstruction tests.
func buildBufferedTxResult(t *testing.T, tx *solana.Transaction, metaErr any, inner []rpc.InnerInstruction, loaded rpc.LoadedAddresses) *rpc.GetTransactionResult {
	t.Helper()
	res := buildTxResult(t, tx, metaErr)
	res.Meta.InnerInstructions = inner
	res.Meta.LoadedAddresses = loaded
	return res
}

// TestNewExecutionAttemptPoller covers the constructor's required-field validation.
func TestNewExecutionAttemptPoller(t *testing.T) {
	t.Parallel()
	validParams := func(t *testing.T) ExecutionAttemptPollerParams {
		return ExecutionAttemptPollerParams{
			Lggr:             logger.Nop(),
			LogPoller:        newDefaultLogPollerForDest(t),
			RPC:              newDefaultDestMockRPC(t),
			OfframpProgramID: testOfframpProgramID,
		}
	}
	tests := []struct {
		name        string
		mutate      func(p *ExecutionAttemptPollerParams)
		wantErr     bool
		errContains string
	}{
		{name: "nil_logger", mutate: func(p *ExecutionAttemptPollerParams) { p.Lggr = nil }, wantErr: true, errContains: "logger is required"},
		{name: "nil_log_poller", mutate: func(p *ExecutionAttemptPollerParams) { p.LogPoller = nil }, wantErr: true, errContains: "log poller is required"},
		{name: "nil_rpc", mutate: func(p *ExecutionAttemptPollerParams) { p.RPC = nil }, wantErr: true, errContains: "rpc client is required"},
		{name: "zero_offramp_program_id", mutate: func(p *ExecutionAttemptPollerParams) { p.OfframpProgramID = solana.PublicKey{} }, wantErr: true, errContains: "offramp program ID is required"},
		{name: "lookback_below_one_slot", mutate: func(p *ExecutionAttemptPollerParams) {
			p.Config = Config{LookbackWindow: *commonconfig.MustNewDuration(100 * time.Millisecond)}
		}, wantErr: true, errContains: "lookback_window must be at least"},
		{name: "valid_params", mutate: func(*ExecutionAttemptPollerParams) {}, wantErr: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			params := validParams(t)
			tc.mutate(&params)
			p, err := NewExecutionAttemptPoller(params)
			if tc.wantErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.errContains)
				require.Nil(t, p)
			} else {
				require.NoError(t, err)
				require.NotNil(t, p)
				require.Equal(t, defaultMaxEventsPerPoll, p.maxEventsPerPoll)
			}
		})
	}
}

// TestExecutionAttemptPoller_IngestPipeline checks which events are safe to cache.
func TestExecutionAttemptPoller_IngestPipeline(t *testing.T) {
	t.Parallel()
	const destSel = protocol.ChainSelector(9837465928374658293)
	offrampProgramID := testOfframpProgramID
	baseMsg := buildTestMessage(t, destSel)
	msgID, err := baseMsg.MessageID()
	require.NoError(t, err)

	verifierResults := [][]byte{{0xaa, 0xaa}, {0xbb, 0xbb}}
	const cuLimit uint32 = 200_000
	sig := solana.Signature{0x01, 0x02, 0x03}

	tests := []struct {
		name         string
		event        ccip_offramp.ExecutionStateChangedV2
		eventDataFn  func(t *testing.T, ev ccip_offramp.ExecutionStateChangedV2) []byte
		metaErr      any
		wantCached   bool
		wantCCVData  [][]byte
		wantCULimitU uint32
	}{
		{
			name: "in_progress_success_tx_caches_attempt",
			event: ccip_offramp.ExecutionStateChangedV2{
				MessageId: msgID,
				State:     ccip_offramp.MessageExecutionState_InProgress,
			},
			metaErr:      nil,
			wantCached:   true,
			wantCCVData:  verifierResults,
			wantCULimitU: cuLimit,
		},
		{
			name: "in_progress_failed_tx_is_cached",
			event: ccip_offramp.ExecutionStateChangedV2{
				MessageId: msgID,
				State:     ccip_offramp.MessageExecutionState_InProgress,
			},
			metaErr:      map[string]any{"InstructionError": []any{float64(1), "Custom"}},
			wantCached:   true,
			wantCCVData:  verifierResults,
			wantCULimitU: cuLimit,
		},
		{
			name: "success_event_is_skipped",
			event: ccip_offramp.ExecutionStateChangedV2{
				MessageId: msgID,
				State:     ccip_offramp.MessageExecutionState_Success,
			},
			wantCached: false,
		},
		{
			name: "failure_event_is_skipped",
			event: ccip_offramp.ExecutionStateChangedV2{
				MessageId: msgID,
				State:     ccip_offramp.MessageExecutionState_Failure,
			},
			wantCached: false,
		},
		{
			name: "untouched_event_is_skipped",
			event: ccip_offramp.ExecutionStateChangedV2{
				MessageId: msgID,
				State:     ccip_offramp.MessageExecutionState_Untouched,
			},
			wantCached: false,
		},
		{
			name: "wrong_discriminator_is_skipped",
			event: ccip_offramp.ExecutionStateChangedV2{
				MessageId: msgID,
				State:     ccip_offramp.MessageExecutionState_InProgress,
			},
			eventDataFn: func(t *testing.T, _ ccip_offramp.ExecutionStateChangedV2) []byte {
				t.Helper()
				return []byte{0, 1, 2, 3, 4, 5, 6, 7, 42} // wrong 8-byte prefix
			},
			wantCached: false,
		},
		{
			name: "event_message_id_mismatch_is_dropped",
			event: ccip_offramp.ExecutionStateChangedV2{
				MessageId: [32]uint8{0xff, 0xff, 0xff}, // != msgID
				State:     ccip_offramp.MessageExecutionState_InProgress,
			},
			wantCached: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rpcFake := newMockRPCPollerTxOnly(t, func(_ context.Context, gotSig solana.Signature) (*rpc.GetTransactionResult, error) {
				require.Equal(t, sig, gotSig)
				tx := buildExecuteV2Tx(t, offrampProgramID, baseMsg, verifierResults, cuLimit)
				return buildTxResult(t, tx, tc.metaErr), nil
			})
			p := newTestPoller(t, newDefaultLogPollerForDest(t), rpcFake, offrampProgramID)

			dataFn := tc.eventDataFn
			if dataFn == nil {
				dataFn = encodeExecutionStateChangedV2Event
			}
			err := p.ingestEvent(context.Background(), logpoller.Event{
				Data:        dataFn(t, tc.event),
				TxSignature: sig,
			})

			got, gErr := p.GetAttempts(baseMsg)
			require.NoError(t, gErr)
			if !tc.wantCached {
				require.Empty(t, got, "expected no cached attempt")
				_ = err
				return
			}
			require.NoError(t, err)
			require.Len(t, got, 1)
			attempt := got[0]
			require.Equal(t, tc.wantCCVData, attempt.Report.CCVData)
			wantCCVS := make([]protocol.UnknownAddress, len(testCCVs))
			for i, pk := range testCCVs {
				wantCCVS[i] = protocol.UnknownAddress(pk.Bytes())
			}
			require.Equal(t, wantCCVS, attempt.Report.CCVS)

			wantID, idErr := attempt.Report.Message.MessageID()
			require.NoError(t, idErr)
			require.Equal(t, wantID, msgID, "decoded message ID must match event")

			require.NotNil(t, attempt.TransactionGasLimit)
			require.Equal(t, uint64(tc.wantCULimitU), attempt.TransactionGasLimit.Uint64())
		})
	}
}

func TestExecutionAttemptPoller_IngestMatchesBatchedExecuteV2(t *testing.T) {
	t.Parallel()
	const destSel = protocol.ChainSelector(9837465928374658293)
	const cuLimit uint32 = 200_000
	sig := solana.Signature{0x04, 0x05}

	firstMsg := buildTestMessage(t, destSel)
	secondMsg := buildTestMessage(t, destSel)
	secondMsg.SequenceNumber = 2
	secondID, err := secondMsg.MessageID()
	require.NoError(t, err)

	// Instructions: [SetComputeUnitLimit, ExecuteV2(firstMsg), ExecuteV2(secondMsg)].
	tx := buildExecuteV2Tx(t, testOfframpProgramID, secondMsg, [][]byte{{0xbb}}, cuLimit)
	firstIx := buildExecuteV2Tx(t, testOfframpProgramID, firstMsg, [][]byte{{0xaa}}, 0).Message.Instructions[0]
	tx.Message.Instructions = []solana.CompiledInstruction{tx.Message.Instructions[0], firstIx, tx.Message.Instructions[1]}

	rpcFake := newMockRPCPollerTxOnly(t, func(context.Context, solana.Signature) (*rpc.GetTransactionResult, error) {
		return buildTxResult(t, tx, nil), nil
	})
	p := newTestPoller(t, newDefaultLogPollerForDest(t), rpcFake, testOfframpProgramID)

	require.NoError(t, p.ingestEvent(context.Background(), logpoller.Event{
		Data: encodeExecutionStateChangedV2Event(t, ccip_offramp.ExecutionStateChangedV2{
			MessageId: secondID,
			State:     ccip_offramp.MessageExecutionState_InProgress,
		}),
		TxSignature: sig,
	}))

	got, err := p.GetAttempts(secondMsg)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, [][]byte{{0xbb}}, got[0].Report.CCVData)
	require.Equal(t, uint64(cuLimit), got[0].TransactionGasLimit.Uint64())
}

// TestExecutionAttemptPoller_BufferedIngest covers the buffered ExecuteV2 form
// (ExecInputs == nil): the attempt is reconstructed from the verify CPIs recorded in the
// tx's inner instructions rather than from the ix data. Identification filters, ix-index
// group, Verify discriminator, VerifyParams messageID, are each exercised, along with
// ALT-loaded CCV program IDs and the transient/permanent failure taxonomy.
func TestExecutionAttemptPoller_BufferedIngest(t *testing.T) {
	t.Parallel()
	const destSel = protocol.ChainSelector(9837465928374658293)
	offrampProgramID := testOfframpProgramID
	msg := buildTestMessage(t, destSel)
	msgID, err := msg.MessageID()
	require.NoError(t, err)
	encodedMessage, err := msg.Encode()
	require.NoError(t, err)

	verifierResults := [][]byte{{0xaa, 0xaa}, {0xbb, 0xbb}}
	const cuLimit uint32 = 200_000
	sig := solana.Signature{0x0a, 0x0b}

	// verifyGroup returns the inner-instruction list for one group: two verify CPIs whose
	// programs are programIndices[0..1], message-pinned to messageID.
	verifyGroup := func(messageID [32]byte, programIndices ...uint16) []rpc.CompiledInstruction {
		require.Len(t, programIndices, len(verifierResults), "fixture: one program index per verifier result")
		ixs := make([]rpc.CompiledInstruction, 0, len(programIndices))
		for i, idx := range programIndices {
			ixs = append(ixs, buildVerifyInnerInstruction(t, idx, encodedMessage, messageID, verifierResults[i]))
		}
		return ixs
	}

	// Static-key fixtures: the tx's AccountKeys are [offramp, ComputeBudget], so the CCV
	// programs ride along as extra static keys at indices 2 and 3.
	const (
		ccv0StaticIdx = 2
		ccv1StaticIdx = 3
	)
	withStaticCCVs := func(tx *solana.Transaction) *solana.Transaction {
		tx.Message.AccountKeys = append(tx.Message.AccountKeys, testCCVs...)
		return tx
	}

	wantCCVS := make([]protocol.UnknownAddress, len(testCCVs))
	for i, pk := range testCCVs {
		wantCCVS[i] = protocol.UnknownAddress(pk.Bytes())
	}

	tests := []struct {
		name string
		// resultFor builds the GetTransactionResult for the tx and its execute-ix index.
		resultFor     func(tx *solana.Transaction, execIxIndex uint16) *rpc.GetTransactionResult
		mutate        func(tx *solana.Transaction) *solana.Transaction // defaults to withStaticCCVs
		wantCached    bool
		wantTransient bool
	}{
		{
			name: "success_tx_caches_attempt",
			resultFor: func(tx *solana.Transaction, execIxIndex uint16) *rpc.GetTransactionResult {
				return buildBufferedTxResult(t, tx, nil,
					[]rpc.InnerInstruction{{Index: execIxIndex, Instructions: verifyGroup(msgID, ccv0StaticIdx, ccv1StaticIdx)}},
					rpc.LoadedAddresses{})
			},
			wantCached: true,
		},
		{
			name: "failed_tx_still_caches_attempt",
			resultFor: func(tx *solana.Transaction, execIxIndex uint16) *rpc.GetTransactionResult {
				return buildBufferedTxResult(t, tx, map[string]any{"InstructionError": []any{float64(1), "Custom"}},
					[]rpc.InnerInstruction{{Index: execIxIndex, Instructions: verifyGroup(msgID, ccv0StaticIdx, ccv1StaticIdx)}},
					rpc.LoadedAddresses{})
			},
			wantCached: true,
		},
		{
			name: "verify_cpi_under_different_top_level_ix_ignored",
			resultFor: func(tx *solana.Transaction, execIxIndex uint16) *rpc.GetTransactionResult {
				// Group keyed to the ComputeBudget ix (index 0), not the execute ix.
				return buildBufferedTxResult(t, tx, nil,
					[]rpc.InnerInstruction{{Index: 0, Instructions: verifyGroup(msgID, ccv0StaticIdx, ccv1StaticIdx)}},
					rpc.LoadedAddresses{})
			},
			wantCached: false,
		},
		{
			name: "wrong_discriminator_ignored",
			resultFor: func(tx *solana.Transaction, execIxIndex uint16) *rpc.GetTransactionResult {
				other := buildVerifyInnerInstruction(t, ccv0StaticIdx, encodedMessage, msgID, verifierResults[0])
				copy(other.Data[:8], []byte{1, 2, 3, 4, 5, 6, 7, 8}) // clobber the discriminator
				return buildBufferedTxResult(t, tx, nil,
					[]rpc.InnerInstruction{{Index: execIxIndex, Instructions: []rpc.CompiledInstruction{other}}},
					rpc.LoadedAddresses{})
			},
			wantCached: false,
		},
		{
			name: "verify_params_message_id_mismatch_ignored",
			resultFor: func(tx *solana.Transaction, execIxIndex uint16) *rpc.GetTransactionResult {
				otherID := [32]uint8{0xff, 0xff, 0xff}
				return buildBufferedTxResult(t, tx, nil,
					[]rpc.InnerInstruction{{Index: execIxIndex, Instructions: verifyGroup(otherID, ccv0StaticIdx, ccv1StaticIdx)}},
					rpc.LoadedAddresses{})
			},
			wantCached: false,
		},
		{
			name: "alt_loaded_ccv_program_resolved",
			// CCV programs are NOT static keys; ProgramIDIndex reaches past the static
			// account keys into LoadedAddresses (writable first, then readonly).
			mutate: func(tx *solana.Transaction) *solana.Transaction { return tx },
			resultFor: func(tx *solana.Transaction, execIxIndex uint16) *rpc.GetTransactionResult {
				staticLen := uint16(len(tx.Message.AccountKeys)) //nolint:gosec // G115: fixture has 2 keys
				return buildBufferedTxResult(t, tx, nil,
					[]rpc.InnerInstruction{{Index: execIxIndex, Instructions: verifyGroup(msgID, staticLen, staticLen+1)}},
					rpc.LoadedAddresses{
						Writable: solana.PublicKeySlice{testCCVs[0]},
						ReadOnly: solana.PublicKeySlice{testCCVs[1]},
					})
			},
			wantCached: true,
		},
		{
			name: "recorded_but_no_verify_cpis_is_permanent",
			resultFor: func(tx *solana.Transaction, execIxIndex uint16) *rpc.GetTransactionResult {
				// Inner instructions recorded, but the execute group is empty (tx failed
				// before any verify CPI), nothing cacheable exists.
				return buildBufferedTxResult(t, tx, map[string]any{"InstructionError": "Custom"},
					[]rpc.InnerInstruction{{Index: execIxIndex, Instructions: []rpc.CompiledInstruction{}}},
					rpc.LoadedAddresses{})
			},
			wantCached:    false,
			wantTransient: false,
		},
		{
			name: "nil_inner_instructions_is_transient",
			resultFor: func(tx *solana.Transaction, _ uint16) *rpc.GetTransactionResult {
				// Meta present but innerInstructions null (node didn't record them).
				return buildTxResult(t, tx, nil)
			},
			wantCached:    false,
			wantTransient: true,
		},
		{
			name: "nil_meta_is_transient",
			resultFor: func(tx *solana.Transaction, _ uint16) *rpc.GetTransactionResult {
				res := buildTxResult(t, tx, nil)
				res.Meta = nil
				return res
			},
			wantCached:    false,
			wantTransient: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rpcFake := newMockRPCPollerTxOnly(t, func(_ context.Context, gotSig solana.Signature) (*rpc.GetTransactionResult, error) {
				require.Equal(t, sig, gotSig)
				tx, execIxIndex := buildBufferedExecuteV2Tx(t, offrampProgramID, cuLimit)
				if tc.mutate != nil {
					tx = tc.mutate(tx)
				} else {
					tx = withStaticCCVs(tx)
				}
				return tc.resultFor(tx, execIxIndex), nil
			})
			p := newTestPoller(t, newDefaultLogPollerForDest(t), rpcFake, offrampProgramID)

			event := ccip_offramp.ExecutionStateChangedV2{MessageId: msgID, State: ccip_offramp.MessageExecutionState_InProgress}
			err := p.ingestEvent(context.Background(), logpoller.Event{
				Data:        encodeExecutionStateChangedV2Event(t, event),
				TxSignature: sig,
			})

			switch {
			case tc.wantTransient:
				require.Error(t, err)
				require.ErrorIs(t, err, errTransientIngest)
			case !tc.wantCached:
				require.Error(t, err, "permanent decode failure should surface an error")
				require.NotErrorIs(t, err, errTransientIngest)
			default:
				require.NoError(t, err)
			}

			got, gErr := p.GetAttempts(msg)
			require.NoError(t, gErr)
			if !tc.wantCached {
				require.Empty(t, got, "expected no cached attempt")
				return
			}
			require.Len(t, got, 1)
			attempt := got[0]
			require.Equal(t, verifierResults, attempt.Report.CCVData)
			require.Equal(t, wantCCVS, attempt.Report.CCVS)
			gotID, idErr := attempt.Report.Message.MessageID()
			require.NoError(t, idErr)
			require.Equal(t, msgID, gotID)
			require.NotNil(t, attempt.TransactionGasLimit)
			require.Equal(t, uint64(cuLimit), attempt.TransactionGasLimit.Uint64())
		})
	}
}

// TestExtractComputeUnitLimit verifies that the helper correctly discriminates between
// ComputeBudget instruction variants: only SetComputeUnitLimit yields a value; Price /
// HeapFrame / no-ComputeBudget all return nil.
func TestExtractComputeUnitLimit(t *testing.T) {
	t.Parallel()
	computeBudgetIx := func(data []byte) solana.CompiledInstruction {
		return solana.CompiledInstruction{ProgramIDIndex: 0, Data: data}
	}
	cuLimitData, err := fees.ComputeUnitLimit(200_000).Data()
	require.NoError(t, err)
	cuPriceData, err := fees.ComputeUnitPrice(50_000).Data()
	require.NoError(t, err)

	mkTx := func(ixs ...solana.CompiledInstruction) *solana.Transaction {
		return &solana.Transaction{
			Message: solana.Message{
				Header:       solana.MessageHeader{NumReadonlyUnsignedAccounts: 1},
				AccountKeys:  []solana.PublicKey{fees.ComputeBudgetProgram},
				Instructions: ixs,
			},
		}
	}

	tests := []struct {
		name string
		tx   *solana.Transaction
		want *big.Int
	}{
		{
			name: "set_compute_unit_limit_returns_value",
			tx:   mkTx(computeBudgetIx(cuLimitData)),
			want: big.NewInt(200_000),
		},
		{
			name: "set_compute_unit_price_returns_nil",
			tx:   mkTx(computeBudgetIx(cuPriceData)),
			want: nil,
		},
		{
			name: "no_compute_budget_ix_returns_nil",
			tx: &solana.Transaction{
				Message: solana.Message{
					Header:      solana.MessageHeader{NumReadonlyUnsignedAccounts: 1},
					AccountKeys: []solana.PublicKey{solana.PublicKey(common.MakeRandom32ByteArray())},
				},
			},
			want: nil,
		},
		{
			name: "price_then_limit_returns_limit",
			tx:   mkTx(computeBudgetIx(cuPriceData), computeBudgetIx(cuLimitData)),
			want: big.NewInt(200_000),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := extractComputeUnitLimit(tc.tx)
			if tc.want == nil {
				require.Nil(t, got)
			} else {
				require.NotNil(t, got)
				require.Equal(t, tc.want.String(), got.String())
			}
		})
	}
}

// TestExecutionAttemptPoller_IngestAccumulatesAttempts verifies that failed attempts are retained.
func TestExecutionAttemptPoller_IngestAccumulatesAttempts(t *testing.T) {
	t.Parallel()
	const destSel = protocol.ChainSelector(9837465928374658293)
	offrampProgramID := testOfframpProgramID
	msg := buildTestMessage(t, destSel)
	msgID, err := msg.MessageID()
	require.NoError(t, err)

	rpcM := &mocks.MockDestinationReaderRPCClient{}
	rpcM.On("GetAccountInfo", mock.Anything, mock.Anything, mock.Anything).
		Return(func(context.Context, solana.PublicKey, rpc.CommitmentType) (*rpc.GetAccountInfoResult, error) {
			panic("unexpected call to GetAccountInfo")
		}).Maybe()
	rpcM.On("GetSlot", mock.Anything, mock.Anything).Return(uint64(0), nil).Maybe()
	rpcM.On("GetTransaction", mock.Anything, solana.Signature{0x01}).
		Return(func(_ context.Context, _ solana.Signature) (*rpc.GetTransactionResult, error) {
			tx := buildExecuteV2Tx(t, offrampProgramID, msg, [][]byte{{0xcc}}, 150_000)
			return buildTxResult(t, tx, nil), nil
		}).Once()
	rpcM.On("GetTransaction", mock.Anything, solana.Signature{0x02}).
		Return(func(_ context.Context, _ solana.Signature) (*rpc.GetTransactionResult, error) {
			tx := buildExecuteV2Tx(t, offrampProgramID, msg, [][]byte{{0xcc}}, 150_000)
			return buildTxResult(t, tx, map[string]any{"InstructionError": "Custom"}), nil
		}).Once()
	t.Cleanup(func() { rpcM.AssertExpectations(t) })

	p := newTestPoller(t, newDefaultLogPollerForDest(t), rpcM, offrampProgramID)

	ev := ccip_offramp.ExecutionStateChangedV2{
		MessageId: msgID,
		State:     ccip_offramp.MessageExecutionState_InProgress,
	}
	data := encodeExecutionStateChangedV2Event(t, ev)
	// First attempt succeeds. The failed retry is still an attempt.
	require.NoError(t, p.ingestEvent(context.Background(), logpoller.Event{Data: data, TxSignature: solana.Signature{0x01}}))
	require.NoError(t, p.ingestEvent(context.Background(), logpoller.Event{Data: data, TxSignature: solana.Signature{0x02}}))

	got, gErr := p.GetAttempts(msg)
	require.NoError(t, gErr)
	require.Len(t, got, 2)
}

// TestExecutionAttemptPollerCacheAttemptCapsAttemptsPerMessage verifies the hard cap:
// once a messageID's slice reaches maxAttemptsPerMessage, the oldest entries are
// dropped so repeated re-ingestion from a held cursor cannot grow memory unboundedly.
func TestExecutionAttemptPollerCacheAttemptCapsAttemptsPerMessage(t *testing.T) {
	t.Parallel()
	p := newTestPoller(t, newDefaultLogPollerForDest(t), newDefaultDestMockRPC(t), testOfframpProgramID)

	var msgID [32]byte
	msgID[0] = 0x42
	const total = 60
	for i := range total {
		p.cacheAttempt(decodedExecutionAttempt{
			messageID: msgID,
			attempt:   protocol.ExecutionAttempt{TransactionGasLimit: big.NewInt(int64(i))},
		})
	}

	got, ok := p.attemptCache.Get(msgID)
	require.True(t, ok)
	require.Len(t, got, maxAttemptsPerMessage)
	// Newest entries are kept: oldest retained is #10 (60-50), newest is #59.
	require.Equal(t, int64(total-maxAttemptsPerMessage), got[0].TransactionGasLimit.Int64())
	require.Equal(t, int64(total-1), got[len(got)-1].TransactionGasLimit.Int64())
}

func TestExecutionAttemptPollerCacheAttemptCapsMessageIDs(t *testing.T) {
	t.Parallel()
	p := newTestPoller(t, newDefaultLogPollerForDest(t), newDefaultDestMockRPC(t), testOfframpProgramID)

	const total = maxCachedMessageIDs + 10
	for i := range total {
		var msgID [32]byte
		binary.BigEndian.PutUint64(msgID[:], uint64(i))
		p.cacheAttempt(decodedExecutionAttempt{messageID: msgID, failed: true})
	}

	require.Equal(t, maxCachedMessageIDs, p.failedAttemptCache.Len())
	_, oldestPresent := p.failedAttemptCache.Get([32]byte{})
	require.False(t, oldestPresent)
	var newest [32]byte
	binary.BigEndian.PutUint64(newest[:], total-1)
	_, newestPresent := p.failedAttemptCache.Get(newest)
	require.True(t, newestPresent)
}

func TestExecutionAttemptPollerIngestBatchSkipsFailedEvents(t *testing.T) {
	t.Parallel()
	const destSel = protocol.ChainSelector(9837465928374658293)
	offrampProgramID := testOfframpProgramID
	msg := buildTestMessage(t, destSel)
	msgID, err := msg.MessageID()
	require.NoError(t, err)

	rpcFake := newMockRPCPollerTxOnly(t, func(_ context.Context, _ solana.Signature) (*rpc.GetTransactionResult, error) {
		tx := buildExecuteV2Tx(t, offrampProgramID, msg, [][]byte{{0xcc}}, 150_000)
		return buildTxResult(t, tx, nil), nil
	})
	p := newTestPoller(t, newDefaultLogPollerForDest(t), rpcFake, offrampProgramID)

	goodEvent := ccip_offramp.ExecutionStateChangedV2{
		MessageId: msgID,
		State:     ccip_offramp.MessageExecutionState_InProgress,
	}
	badEvent := ccip_offramp.ExecutionStateChangedV2{
		MessageId: [32]uint8{0xff, 0xff, 0xff},
		State:     ccip_offramp.MessageExecutionState_InProgress,
	}

	processedThrough, err := p.ingestBatch(context.Background(), []logpoller.Event{
		{Data: encodeExecutionStateChangedV2Event(t, goodEvent), TxSignature: solana.Signature{0x01}, SlotNumber: 10},
		{Data: encodeExecutionStateChangedV2Event(t, badEvent), TxSignature: solana.Signature{0x02}, SlotNumber: 10},
	}, 10)
	require.NoError(t, err, "permanent per-event failures are skipped, not propagated")
	require.Equal(t, uint64(10), processedThrough)

	got, gErr := p.GetAttempts(msg)
	require.NoError(t, gErr)
	require.Len(t, got, 1, "failed events should be skipped without dropping successful attempts from the same batch")
}

func TestExecutionAttemptPollerStartupBackfillFailureRetriesFromStartSlot(t *testing.T) {
	t.Parallel()
	const latestSlot uint64 = 100
	const lookbackSlots uint64 = 10

	var fetchRanges [][2]uint64
	var fetchCalls int
	lp := newLogPollerFetchEventsRunAndReturn(t, func(_ context.Context, from, to uint64) ([]logpoller.Event, error) {
		fetchCalls++
		fetchRanges = append(fetchRanges, [2]uint64{from, to})
		if fetchCalls == 1 {
			return nil, errors.New("temporary backfill failure")
		}
		return nil, nil
	})
	rpcClient := newMockRPCPollerSlotAndTx(t, func(context.Context, rpc.CommitmentType) (uint64, error) {
		return latestSlot, nil
	}, nil)

	p := newTestPoller(t, lp, rpcClient, testOfframpProgramID)
	p.lookbackWindow = time.Duration(lookbackSlots) * minSolanaSlotTime

	p.runStartupSequence(context.Background())
	require.Equal(t, slotBefore(latestSlot-lookbackSlots), p.lastPolledSlot.Load())
	require.False(t, p.lookbackCovered.Load())

	require.NoError(t, p.pollOnce(context.Background()))
	require.Equal(t, [][2]uint64{
		{latestSlot - lookbackSlots, latestSlot},
		{latestSlot - lookbackSlots, latestSlot},
	}, fetchRanges)
	require.Equal(t, latestSlot, p.lastPolledSlot.Load())
	require.True(t, p.lookbackCovered.Load())
}

func TestExecutionAttemptPollerHealthyIgnoresTransientPollFailure(t *testing.T) {
	t.Parallel()
	const latestSlot uint64 = 100

	var fetchCalls int
	lp := newLogPollerFetchEventsRunAndReturn(t, func(context.Context, uint64, uint64) ([]logpoller.Event, error) {
		fetchCalls++
		if fetchCalls == 1 {
			return nil, nil
		}
		return nil, errors.New("temporary RPC outage")
	})
	var slotCalls uint64
	rpcClient := newMockRPCPollerSlotAndTx(t, func(context.Context, rpc.CommitmentType) (uint64, error) {
		slotCalls++
		return latestSlot + slotCalls - 1, nil
	}, nil)

	p := newTestPoller(t, lp, rpcClient, testOfframpProgramID)
	p.pollInterval = time.Hour
	require.NoError(t, p.Start(context.Background()))
	t.Cleanup(func() { require.NoError(t, p.Close()) })

	require.Eventually(t, func() bool {
		return p.lookbackCovered.Load()
	}, time.Second, time.Millisecond)

	require.ErrorContains(t, p.pollOnce(context.Background()), "temporary RPC outage")
	require.NoError(t, p.Healthy(), "transient polling failures must not poison message processing")
}

func TestExecutionAttemptPollerRetriesStartupFailure(t *testing.T) {
	t.Parallel()
	var slotCalls atomic.Uint64
	retryGate := make(chan struct{})
	lp := newLogPollerFetchEventsRunAndReturn(t, func(context.Context, uint64, uint64) ([]logpoller.Event, error) {
		return nil, nil
	})
	rpcClient := newMockRPCPollerSlotAndTx(t, func(ctx context.Context, _ rpc.CommitmentType) (uint64, error) {
		if slotCalls.Add(1) == 1 {
			return 0, errors.New("startup RPC failure")
		}
		select {
		case <-retryGate:
			return 100, nil
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	}, nil)

	p := newTestPoller(t, lp, rpcClient, testOfframpProgramID)
	p.pollInterval = time.Millisecond
	require.NoError(t, p.Start(context.Background()))
	t.Cleanup(func() { require.NoError(t, p.Close()) })

	require.Eventually(t, func() bool {
		return slotCalls.Load() >= 2
	}, time.Second, time.Millisecond)
	require.NoError(t, p.Healthy(), "startup RPC failures must not poison message processing")
	require.ErrorIs(t, p.Ready(), ErrBackfillInProgress)

	close(retryGate)
	require.Eventually(t, func() bool {
		return p.Ready() == nil
	}, time.Second, time.Millisecond)
	require.NoError(t, p.Healthy())
}

func TestExecutionAttemptPollerStartReturnsDuringStartup(t *testing.T) {
	t.Parallel()

	started := make(chan struct{}, 1)
	release := make(chan struct{})
	rpcClient := newMockRPCPollerSlotAndTx(t, func(ctx context.Context, _ rpc.CommitmentType) (uint64, error) {
		select {
		case started <- struct{}{}:
		default:
		}
		select {
		case <-release:
			return 100, nil
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	}, nil)
	lp := newLogPollerFetchEventsRunAndReturn(t, func(context.Context, uint64, uint64) ([]logpoller.Event, error) {
		return nil, nil
	})

	p := newTestPoller(t, lp, rpcClient, testOfframpProgramID)
	p.pollInterval = time.Hour
	require.NoError(t, p.Start(context.Background()))
	t.Cleanup(func() { require.NoError(t, p.Close()) })

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("poller startup did not start")
	}

	require.ErrorIs(t, p.Ready(), ErrBackfillInProgress)
	require.NoError(t, p.Healthy(), "Healthy must ignore backfill progress")
	require.ErrorIs(t, p.HealthReport()[p.Name()], ErrBackfillInProgress)

	close(release)
	require.Eventually(t, func() bool {
		return p.Ready() == nil
	}, time.Second, time.Millisecond)
}

func TestExecutionAttemptPollerCloseCancelsStartup(t *testing.T) {
	t.Parallel()

	started := make(chan struct{}, 1)
	rpcClient := newMockRPCPollerSlotAndTx(t, func(ctx context.Context, _ rpc.CommitmentType) (uint64, error) {
		select {
		case started <- struct{}{}:
		default:
		}
		<-ctx.Done()
		return 0, ctx.Err()
	}, nil)
	lp := newLogPollerNoFetchEvents(t)

	p := newTestPoller(t, lp, rpcClient, testOfframpProgramID)
	require.NoError(t, p.Start(context.Background()))

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("poller startup did not start")
	}

	done := make(chan error, 1)
	go func() { done <- p.Close() }()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("Close did not unblock startup")
	}
}

func TestExecutionAttemptPollerCloseCancelsStartupIngest(t *testing.T) {
	t.Parallel()

	msg := buildTestMessage(t, protocol.ChainSelector(9837465928374658293))
	msgID, err := msg.MessageID()
	require.NoError(t, err)

	started := make(chan struct{}, 1)
	lp := newLogPollerFetchEventsRunAndReturn(t, func(context.Context, uint64, uint64) ([]logpoller.Event, error) {
		return []logpoller.Event{{
			SlotNumber:  90,
			TxSignature: solana.Signature{0x01},
			Data: encodeExecutionStateChangedV2Event(t, ccip_offramp.ExecutionStateChangedV2{
				MessageId: msgID,
				State:     ccip_offramp.MessageExecutionState_InProgress,
			}),
		}}, nil
	})
	rpcClient := newMockRPCPollerSlotAndTx(t, func(context.Context, rpc.CommitmentType) (uint64, error) {
		return 100, nil
	}, func(ctx context.Context, _ solana.Signature) (*rpc.GetTransactionResult, error) {
		select {
		case started <- struct{}{}:
		default:
		}
		<-ctx.Done()
		return nil, ctx.Err()
	})

	p := newTestPoller(t, lp, rpcClient, testOfframpProgramID)
	p.lookbackWindow = 10 * minSolanaSlotTime
	require.NoError(t, p.Start(context.Background()))
	t.Cleanup(func() { _ = p.Close() })

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("startup ingest did not call GetTransaction")
	}

	done := make(chan error, 1)
	go func() { done <- p.Close() }()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("Close did not unblock startup ingest")
	}
	require.False(t, p.lookbackCovered.Load())
	require.Zero(t, p.attemptCache.Len())
}

func TestExecutionAttemptPollerStartupSkipsBackfillWhenStartSlotAtLatest(t *testing.T) {
	t.Parallel()
	const latestSlot uint64 = 100

	lp := newLogPollerNoFetchEvents(t)
	rpcClient := newMockRPCPollerSlotAndTx(t, func(context.Context, rpc.CommitmentType) (uint64, error) {
		return latestSlot, nil
	}, nil)

	p := newTestPoller(t, lp, rpcClient, testOfframpProgramID)
	p.lookbackWindow = time.Nanosecond

	p.runStartupSequence(context.Background())
	require.Equal(t, latestSlot, p.lastPolledSlot.Load())
	require.True(t, p.lookbackCovered.Load())
}

func TestExecutionAttemptPollerPollOnceCursorBehavior(t *testing.T) {
	t.Parallel()
	const fromSlot uint64 = 50
	const toSlot uint64 = 60

	event := ccip_offramp.ExecutionStateChangedV2{
		MessageId: [32]uint8{0xAB},
		State:     ccip_offramp.MessageExecutionState_InProgress,
	}
	eventData := encodeExecutionStateChangedV2Event(t, event)

	// mismatchedTxResult decodes to a different messageID than the event carries, so
	// fetchAttempt fails with a permanent (non-transient) messageID mismatch.
	mismatchedMsg := buildTestMessage(t, protocol.ChainSelector(9837465928374658293))
	mismatchedTxResult := buildTxResult(t, buildExecuteV2Tx(t, testOfframpProgramID, mismatchedMsg, [][]byte{{0xcc}}, 150_000), nil)

	// bufferedNoInnerResult is a buffered ExecuteV2 tx whose meta lacks inner instructions
	// (an RPC node that doesn't record them), fetchAttempt fails transiently and the
	// cursor must hold, since every buffered attempt is undecodable on such a node.
	bufferedTx, _ := buildBufferedExecuteV2Tx(t, testOfframpProgramID, 150_000)
	bufferedNoInnerResult := buildTxResult(t, bufferedTx, nil)

	tests := []struct {
		name                 string
		events               []logpoller.Event
		getSlotErr           error
		fetchErr             error
		getTransactionErr    error
		getTransactionResult *rpc.GetTransactionResult
		wantErrSubstr        string
		wantFetchCalls       int
		wantLastPolled       uint64
		wantCovered          bool
	}{
		{
			name:           "successful range with no events advances",
			wantFetchCalls: 1,
			wantLastPolled: toSlot,
			wantCovered:    true,
		},
		{
			name: "per-event transaction lookup failure holds cursor",
			events: []logpoller.Event{
				{
					Data:        eventData,
					TxSignature: solana.Signature{0x01},
				},
			},
			getTransactionErr: errors.New("temporary transaction lookup failure"),
			wantErrSubstr:     "ingest batch",
			wantFetchCalls:    1,
			wantLastPolled:    slotBefore(fromSlot),
			wantCovered:       false,
		},
		{
			name: "permanent decode failure is skipped and advances",
			events: []logpoller.Event{
				{
					Data:        eventData,
					TxSignature: solana.Signature{0x01},
				},
			},
			getTransactionResult: mismatchedTxResult,
			wantFetchCalls:       1,
			wantLastPolled:       toSlot,
			wantCovered:          true,
		},
		{
			name: "buffered tx without inner instructions holds cursor",
			events: []logpoller.Event{
				{
					Data:        eventData,
					TxSignature: solana.Signature{0x01},
				},
			},
			getTransactionResult: bufferedNoInnerResult,
			wantErrSubstr:        "ingest batch",
			wantFetchCalls:       1,
			wantLastPolled:       slotBefore(fromSlot),
			wantCovered:          false,
		},
		{
			name:           "fetch failure does not advance",
			fetchErr:       errors.New("temporary event fetch failure"),
			wantErrSubstr:  "fetch events",
			wantFetchCalls: 1,
			wantLastPolled: slotBefore(fromSlot),
		},
		{
			name:           "slot failure does not advance",
			getSlotErr:     errors.New("temporary slot lookup failure"),
			wantErrSubstr:  "get slot",
			wantLastPolled: slotBefore(fromSlot),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fetchCalls := 0
			lp := newLogPollerFetchEventsRunAndReturn(t, func(_ context.Context, from, to uint64) ([]logpoller.Event, error) {
				fetchCalls++
				require.Equal(t, fromSlot, from)
				require.Equal(t, toSlot, to)
				return tc.events, tc.fetchErr
			})
			rpcClient := newMockRPCPollerSlotAndTx(t, func(context.Context, rpc.CommitmentType) (uint64, error) {
				if tc.getSlotErr != nil {
					return 0, tc.getSlotErr
				}
				return toSlot, nil
			}, func(_ context.Context, _ solana.Signature) (*rpc.GetTransactionResult, error) {
				if tc.getTransactionErr != nil {
					return nil, tc.getTransactionErr
				}
				if tc.getTransactionResult != nil {
					return tc.getTransactionResult, nil
				}
				return nil, errors.New("unexpected transaction lookup")
			})

			p := newTestPoller(t, lp, rpcClient, testOfframpProgramID)
			p.lastPolledSlot.Store(slotBefore(fromSlot))
			p.coverageTarget.Store(toSlot)

			err := p.pollOnce(context.Background())
			if tc.wantErrSubstr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.wantErrSubstr)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.wantFetchCalls, fetchCalls)
			require.Equal(t, tc.wantLastPolled, p.lastPolledSlot.Load())
			require.Equal(t, tc.wantCovered, p.lookbackCovered.Load())
		})
	}
}

func TestExecutionAttemptPollerMissingTransactionMetaHoldsCursor(t *testing.T) {
	t.Parallel()
	msg := buildTestMessage(t, protocol.ChainSelector(9837465928374658293))
	msgID, err := msg.MessageID()
	require.NoError(t, err)
	result := buildTxResult(t, buildExecuteV2Tx(t, testOfframpProgramID, msg, [][]byte{{0xcc}}, 150_000), nil)
	result.Meta = nil

	lp := newLogPollerFetchEventsRunAndReturn(t, func(context.Context, uint64, uint64) ([]logpoller.Event, error) {
		return []logpoller.Event{{
			SlotNumber:  50,
			TxSignature: solana.Signature{0x01},
			Data: encodeExecutionStateChangedV2Event(t, ccip_offramp.ExecutionStateChangedV2{
				MessageId: msgID,
				State:     ccip_offramp.MessageExecutionState_InProgress,
			}),
		}}, nil
	})
	rpcClient := newMockRPCPollerSlotAndTx(t, func(context.Context, rpc.CommitmentType) (uint64, error) {
		return 60, nil
	}, func(context.Context, solana.Signature) (*rpc.GetTransactionResult, error) {
		return result, nil
	})
	p := newTestPoller(t, lp, rpcClient, testOfframpProgramID)
	p.lastPolledSlot.Store(49)
	p.coverageTarget.Store(60)

	require.ErrorIs(t, p.pollOnce(context.Background()), errTransientIngest)
	require.Equal(t, uint64(49), p.lastPolledSlot.Load())
	require.False(t, p.lookbackCovered.Load())
	require.Zero(t, p.attemptCache.Len())
}

// TestExecutionAttemptPollerPollOnceCapsRangeAtLookback verifies that a cursor lagging past
// the lookback window drops the backlog and fetches only the window, instead of retrying an
// ever-growing range that outgrows the fetch deadline and wedges the poller.
func TestExecutionAttemptPollerPollOnceCapsRangeAtLookback(t *testing.T) {
	t.Parallel()
	const lastPolled uint64 = 100
	const toSlot uint64 = 200
	// lookbackSlots = 5, so the window is [195, 200] and the backlog is 101..194.
	lookback := 5 * minSolanaSlotTime

	var gotFrom, gotTo uint64
	lp := newLogPollerFetchEventsRunAndReturn(t, func(_ context.Context, from, to uint64) ([]logpoller.Event, error) {
		gotFrom, gotTo = from, to
		return nil, nil
	})
	rpcClient := newMockRPCPollerSlotAndTx(t, func(context.Context, rpc.CommitmentType) (uint64, error) {
		return toSlot, nil
	}, nil)

	p, err := NewExecutionAttemptPoller(ExecutionAttemptPollerParams{
		Lggr:             logger.Nop(),
		LogPoller:        lp,
		RPC:              rpcClient,
		OfframpProgramID: testOfframpProgramID,
		Config:           Config{LookbackWindow: *commonconfig.MustNewDuration(lookback)},
	})
	require.NoError(t, err)
	p.lastPolledSlot.Store(lastPolled)
	p.coverageTarget.Store(toSlot)

	require.NoError(t, p.pollOnce(context.Background()))

	// The backlog is dropped; only the lookback window is fetched.
	require.Equal(t, toSlot-5, gotFrom)
	require.Equal(t, toSlot, gotTo)
	require.Equal(t, toSlot, p.lastPolledSlot.Load())
	require.True(t, p.lookbackCovered.Load())
}

// TestExecutionAttemptPollerPollOnceHoldsCursorOnPerEventFailureThenRecovers verifies
// that when ingestBatch has a per-event failure (e.g. transient GetTransaction error),
// pollOnce holds the cursor so the next poll re-covers the failed range. Once the
// transient failure resolves, the cursor advances.
func TestExecutionAttemptPollerPollOnceHoldsCursorOnPerEventFailureThenRecovers(t *testing.T) {
	t.Parallel()
	const fromSlot uint64 = 50
	const toSlot uint64 = 60
	const destSel = protocol.ChainSelector(9837465928374658293)
	offrampProgramID := testOfframpProgramID
	msg := buildTestMessage(t, destSel)
	msgID, err := msg.MessageID()
	require.NoError(t, err)

	goodEvent := ccip_offramp.ExecutionStateChangedV2{
		MessageId: msgID,
		State:     ccip_offramp.MessageExecutionState_InProgress,
	}
	goodEventData := encodeExecutionStateChangedV2Event(t, goodEvent)
	goodTx := buildExecuteV2Tx(t, offrampProgramID, msg, [][]byte{{0xcc}}, 150_000)

	txCallCount := 0
	rpcClient := newMockRPCPollerSlotAndTx(t, func(context.Context, rpc.CommitmentType) (uint64, error) {
		return toSlot, nil
	}, func(_ context.Context, _ solana.Signature) (*rpc.GetTransactionResult, error) {
		txCallCount++
		if txCallCount == 1 {
			return nil, errors.New("transient RPC timeout")
		}
		return buildTxResult(t, goodTx, nil), nil
	})

	fetchCalls := 0
	lp := newLogPollerFetchEventsRunAndReturn(t, func(_ context.Context, from, to uint64) ([]logpoller.Event, error) {
		fetchCalls++
		require.Equal(t, fromSlot, from)
		require.Equal(t, toSlot, to)
		return []logpoller.Event{{Data: goodEventData, TxSignature: solana.Signature{0x01}}}, nil
	})

	p := newTestPoller(t, lp, rpcClient, offrampProgramID)
	p.lastPolledSlot.Store(slotBefore(fromSlot))
	p.coverageTarget.Store(toSlot)

	// First poll: GetTransaction fails transiently, cursor must NOT advance.
	err = p.pollOnce(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "ingest batch")
	require.Equal(t, slotBefore(fromSlot), p.lastPolledSlot.Load())
	require.False(t, p.lookbackCovered.Load())

	// The good event was not cached because GetTransaction failed before caching.
	got, gErr := p.GetAttempts(msg)
	require.NoError(t, gErr)
	require.Empty(t, got)

	// Second poll: same range re-fetched, GetTransaction succeeds, cursor advances.
	err = p.pollOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, toSlot, p.lastPolledSlot.Load())
	require.True(t, p.lookbackCovered.Load())

	// The event is now cached.
	got, gErr = p.GetAttempts(msg)
	require.NoError(t, gErr)
	require.Len(t, got, 1)

	require.Equal(t, 2, fetchCalls, "failed range should be re-fetched on next poll")
}

// TestExecutionAttemptPollerConcurrentReadWrite pins the cache concurrency contract:
// one writer (runLoop's ingest path) plus any number of readers must be race-free.
// cacheAttempt's get-append-add is safe only with a single writer, so the point of
// this test is running it with -race: the detector is the assertion.
func TestExecutionAttemptPollerConcurrentReadWrite(t *testing.T) {
	t.Parallel()
	p := newTestPoller(t, newDefaultLogPollerForDest(t), newDefaultDestMockRPC(t), testOfframpProgramID)
	msg := buildTestMessage(t, protocol.ChainSelector(9837465928374658293))
	msgID, err := msg.MessageID()
	require.NoError(t, err)

	const writes = 200
	var wg sync.WaitGroup
	wg.Go(func() {
		for i := range writes {
			p.cacheAttempt(decodedExecutionAttempt{
				messageID: msgID,
				attempt:   protocol.ExecutionAttempt{TransactionGasLimit: big.NewInt(int64(i))},
				failed:    i%2 == 0,
			})
		}
	})
	for range 4 {
		wg.Go(func() {
			for range writes {
				_, _ = p.GetAttempts(msg)
			}
		})
	}
	wg.Wait()

	// Alternating outcomes fill each cache to its cap; no write is lost.
	successful, failed := p.getAttemptsByOutcome(msgID)
	require.Len(t, successful, maxAttemptsPerMessage)
	require.Len(t, failed, maxFailedAttemptsPerMessage)
}

func TestExecutionAttemptPollerCancellationStopsIngestion(t *testing.T) {
	t.Parallel()
	for _, beforeBatch := range []bool{true, false} {
		t.Run(fmt.Sprintf("cancel_before_batch_%t", beforeBatch), func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := 0
			rpcClient := newMockRPCPollerSlotAndTx(t, func(context.Context, rpc.CommitmentType) (uint64, error) {
				return 60, nil
			}, func(ctx context.Context, _ solana.Signature) (*rpc.GetTransactionResult, error) {
				calls++
				cancel()
				return nil, ctx.Err()
			})
			event := logpoller.Event{Data: encodeExecutionStateChangedV2Event(t, ccip_offramp.ExecutionStateChangedV2{
				MessageId: [32]uint8{0xab},
				State:     ccip_offramp.MessageExecutionState_InProgress,
			})}
			lp := newLogPollerFetchEventsRunAndReturn(t, func(context.Context, uint64, uint64) ([]logpoller.Event, error) {
				if beforeBatch {
					cancel()
				}
				return []logpoller.Event{event, event}, nil
			})
			p := newTestPoller(t, lp, rpcClient, testOfframpProgramID)
			p.lastPolledSlot.Store(49)
			p.coverageTarget.Store(60)
			require.ErrorIs(t, p.pollOnce(ctx), context.Canceled)
			if beforeBatch {
				require.Zero(t, calls)
			} else {
				require.Equal(t, 1, calls)
			}
			require.Equal(t, uint64(49), p.lastPolledSlot.Load())
			require.False(t, p.lookbackCovered.Load())
		})
	}
}

func TestExecutionAttemptPollerStartupBackfillCoversLookbackAtMinSlotTime(t *testing.T) {
	t.Parallel()
	const latestSlot uint64 = 1_000_000
	// 15 minutes at 200ms slots.
	const wantLookbackSlots uint64 = 4500

	var gotFrom uint64
	lp := newLogPollerFetchEventsRunAndReturn(t, func(_ context.Context, from, _ uint64) ([]logpoller.Event, error) {
		gotFrom = from
		return nil, nil
	})
	rpcClient := newMockRPCPollerSlotAndTx(t, func(context.Context, rpc.CommitmentType) (uint64, error) {
		return latestSlot, nil
	}, nil)

	p := newTestPoller(t, lp, rpcClient, testOfframpProgramID)
	require.Equal(t, defaultLookbackWindow, p.lookbackWindow)

	p.runStartupSequence(context.Background())
	require.Equal(t, latestSlot-wantLookbackSlots, gotFrom)
}

func ignoredExecutionAttemptEvent(slot uint64) logpoller.Event {
	return logpoller.Event{
		Data:       []byte{9, 9, 9, 9, 9, 9, 9, 9},
		SlotNumber: slot,
	}
}

func TestExecutionAttemptPollerIngestBatchTruncatesAtSlotBoundary(t *testing.T) {
	t.Parallel()
	p := newTestPoller(t, newDefaultLogPollerForDest(t), newDefaultDestMockRPC(t), testOfframpProgramID)
	p.maxEventsPerPoll = 2

	processedThrough, err := p.ingestBatch(context.Background(), []logpoller.Event{
		ignoredExecutionAttemptEvent(12),
		ignoredExecutionAttemptEvent(10),
		ignoredExecutionAttemptEvent(10),
		ignoredExecutionAttemptEvent(11),
	}, 12)
	require.NoError(t, err)
	require.Equal(t, uint64(10), processedThrough)
	require.Equal(t, uint64(11), slotAfter(processedThrough))
	require.Equal(t, uint64(11), p.pendingSlot)
	require.Equal(t, 0, p.pendingSlotProcessed)
}

func TestExecutionAttemptPollerIngestBatchSkipsEmptySlots(t *testing.T) {
	t.Parallel()
	p := newTestPoller(t, newDefaultLogPollerForDest(t), newDefaultDestMockRPC(t), testOfframpProgramID)
	p.maxEventsPerPoll = 1

	processedThrough, err := p.ingestBatch(context.Background(), []logpoller.Event{
		ignoredExecutionAttemptEvent(10),
		ignoredExecutionAttemptEvent(20),
	}, 20)
	require.NoError(t, err)
	require.Equal(t, uint64(19), processedThrough)
	require.Equal(t, uint64(20), p.pendingSlot)
	require.Equal(t, 0, p.pendingSlotProcessed)
}

func TestExecutionAttemptPollerIngestBatchCapsWithinSlotAndRetainsProgress(t *testing.T) {
	t.Parallel()
	msg := buildTestMessage(t, protocol.ChainSelector(1234))
	msgID, err := msg.MessageID()
	require.NoError(t, err)
	eventData := encodeExecutionStateChangedV2Event(t, ccip_offramp.ExecutionStateChangedV2{
		MessageId: msgID,
		State:     ccip_offramp.MessageExecutionState_InProgress,
	})
	calls := 0
	rpcClient := newMockRPCPollerTxOnly(t, func(context.Context, solana.Signature) (*rpc.GetTransactionResult, error) {
		calls++
		tx := buildExecuteV2Tx(t, testOfframpProgramID, msg, [][]byte{{0xcc}}, 150_000)
		return buildTxResult(t, tx, nil), nil
	})
	p := newTestPoller(t, newDefaultLogPollerForDest(t), rpcClient, testOfframpProgramID)
	p.maxEventsPerPoll = 2
	p.lastPolledSlot.Store(9)

	events := []logpoller.Event{
		{Data: eventData, TxSignature: solana.Signature{0x01}, SlotNumber: 10},
		{Data: eventData, TxSignature: solana.Signature{0x02}, SlotNumber: 10},
		{Data: eventData, TxSignature: solana.Signature{0x03}, SlotNumber: 10},
	}

	processedThrough, err := p.ingestBatch(context.Background(), events, 10)
	require.NoError(t, err)
	require.Equal(t, uint64(9), processedThrough)
	require.Equal(t, uint64(10), p.pendingSlot)
	require.Equal(t, 2, p.pendingSlotProcessed)
	require.Equal(t, 2, calls)

	processedThrough, err = p.ingestBatch(context.Background(), events, 10)
	require.NoError(t, err)
	require.Equal(t, uint64(10), processedThrough)
	require.Equal(t, pendingSlotUnset, p.pendingSlot)
	require.Equal(t, 0, p.pendingSlotProcessed)
	require.Equal(t, 3, calls)

	attempts, getErr := p.GetAttempts(msg)
	require.NoError(t, getErr)
	require.Len(t, attempts, 3)
}

func TestExecutionAttemptPollerIngestBatchRetainsDenseSlotProgress(t *testing.T) {
	t.Parallel()
	msg := buildTestMessage(t, protocol.ChainSelector(1234))
	msgID, err := msg.MessageID()
	require.NoError(t, err)
	eventData := encodeExecutionStateChangedV2Event(t, ccip_offramp.ExecutionStateChangedV2{
		MessageId: msgID,
		State:     ccip_offramp.MessageExecutionState_InProgress,
	})

	calls := 0
	failedOnce := false
	rpcClient := newMockRPCPollerTxOnly(t, func(_ context.Context, _ solana.Signature) (*rpc.GetTransactionResult, error) {
		calls++
		if calls == 201 && !failedOnce {
			failedOnce = true
			return nil, errors.New("transient RPC timeout")
		}
		tx := buildExecuteV2Tx(t, testOfframpProgramID, msg, [][]byte{{0xcc}}, 150_000)
		return buildTxResult(t, tx, nil), nil
	})
	p := newTestPoller(t, newDefaultLogPollerForDest(t), rpcClient, testOfframpProgramID)
	p.lastPolledSlot.Store(9)

	const slot = uint64(10)
	events := make([]logpoller.Event, 201)
	for i := range events {
		var sig solana.Signature
		binary.LittleEndian.PutUint64(sig[:8], uint64(i+1))
		events[i] = logpoller.Event{
			Data:        eventData,
			TxSignature: sig,
			SlotNumber:  slot,
		}
	}

	processedThrough, err := p.ingestBatch(context.Background(), events, slot)
	require.NoError(t, err)
	require.Equal(t, uint64(9), processedThrough)
	require.Equal(t, slot, p.pendingSlot)
	require.Equal(t, 200, p.pendingSlotProcessed)
	require.Equal(t, 200, calls)

	processedThrough, err = p.ingestBatch(context.Background(), events, slot)
	require.Error(t, err)
	require.Equal(t, uint64(9), processedThrough)
	require.Equal(t, slot, p.pendingSlot)
	require.Equal(t, 200, p.pendingSlotProcessed)
	require.Equal(t, 201, calls)

	processedThrough, err = p.ingestBatch(context.Background(), events, slot)
	require.NoError(t, err)
	require.Equal(t, slot, processedThrough)
	require.Equal(t, pendingSlotUnset, p.pendingSlot)
	require.Equal(t, 0, p.pendingSlotProcessed)
	require.Equal(t, 202, calls)

	attempts, getErr := p.GetAttempts(msg)
	require.NoError(t, getErr)
	require.Len(t, attempts, maxAttemptsPerMessage)
}

func TestExecutionAttemptPollerPollOnceResumesFromFirstUncoveredSlot(t *testing.T) {
	t.Parallel()
	const fromSlot, toSlot = uint64(10), uint64(12)
	msg := buildTestMessage(t, protocol.ChainSelector(1234))
	msgID, err := msg.MessageID()
	require.NoError(t, err)
	eventData := encodeExecutionStateChangedV2Event(t, ccip_offramp.ExecutionStateChangedV2{
		MessageId: msgID,
		State:     ccip_offramp.MessageExecutionState_InProgress,
	})
	events := []logpoller.Event{
		{Data: eventData, TxSignature: solana.Signature{0x0A}, SlotNumber: 10},
		{Data: eventData, TxSignature: solana.Signature{0x0B}, SlotNumber: 11},
		{Data: eventData, TxSignature: solana.Signature{0x0C}, SlotNumber: 12},
	}
	failedOnce := false
	rpcClient := newMockRPCPollerSlotAndTx(t, func(context.Context, rpc.CommitmentType) (uint64, error) {
		return toSlot, nil
	}, func(_ context.Context, sig solana.Signature) (*rpc.GetTransactionResult, error) {
		if sig == (solana.Signature{0x0B}) && !failedOnce {
			failedOnce = true
			return nil, errors.New("transient RPC timeout")
		}
		tx := buildExecuteV2Tx(t, testOfframpProgramID, msg, [][]byte{{0xcc}}, 150_000)
		return buildTxResult(t, tx, nil), nil
	})
	var fetchRanges [][2]uint64
	fetchCalls := 0
	lp := newLogPollerFetchEventsRunAndReturn(t, func(_ context.Context, from, to uint64) ([]logpoller.Event, error) {
		fetchRanges = append(fetchRanges, [2]uint64{from, to})
		fetchCalls++
		if fetchCalls == 1 {
			return events, nil
		}
		return events[1:], nil
	})
	p := newTestPoller(t, lp, rpcClient, testOfframpProgramID)
	p.lastPolledSlot.Store(slotBefore(fromSlot))
	p.coverageTarget.Store(toSlot)

	require.ErrorContains(t, p.pollOnce(context.Background()), "ingest batch")
	require.Equal(t, fromSlot, p.lastPolledSlot.Load())
	require.False(t, p.lookbackCovered.Load())

	require.NoError(t, p.pollOnce(context.Background()))
	require.Equal(t, [][2]uint64{{fromSlot, toSlot}, {fromSlot + 1, toSlot}}, fetchRanges)
	require.Equal(t, toSlot, p.lastPolledSlot.Load())
	require.True(t, p.lookbackCovered.Load())
}

func TestExecutionAttemptPollerStartupTruncationKeepsLookbackUncovered(t *testing.T) {
	t.Parallel()
	const latestSlot = uint64(12)
	var fetchRanges [][2]uint64
	fetchCalls := 0
	lp := newLogPollerFetchEventsRunAndReturn(t, func(_ context.Context, from, to uint64) ([]logpoller.Event, error) {
		fetchRanges = append(fetchRanges, [2]uint64{from, to})
		fetchCalls++
		if fetchCalls == 1 {
			return []logpoller.Event{
				ignoredExecutionAttemptEvent(10),
				ignoredExecutionAttemptEvent(11),
				ignoredExecutionAttemptEvent(12),
			}, nil
		}
		return []logpoller.Event{ignoredExecutionAttemptEvent(12)}, nil
	})
	rpcClient := newMockRPCPollerSlotAndTx(t, func(context.Context, rpc.CommitmentType) (uint64, error) {
		return latestSlot, nil
	}, nil)
	p := newTestPoller(t, lp, rpcClient, testOfframpProgramID)
	p.lookbackWindow = 2 * minSolanaSlotTime
	p.maxEventsPerPoll = 2

	p.runStartupSequence(context.Background())
	require.Equal(t, uint64(11), p.lastPolledSlot.Load())
	require.False(t, p.lookbackCovered.Load())

	require.NoError(t, p.pollOnce(context.Background()))
	require.Equal(t, [][2]uint64{{10, latestSlot}, {latestSlot, latestSlot}}, fetchRanges)
	require.Equal(t, latestSlot, p.lastPolledSlot.Load())
	require.True(t, p.lookbackCovered.Load())
}

func TestExecutionAttemptPollerStartupTransientRetainsPartialProgress(t *testing.T) {
	t.Parallel()
	const latestSlot = uint64(12)
	msg := buildTestMessage(t, protocol.ChainSelector(1234))
	msgID, err := msg.MessageID()
	require.NoError(t, err)
	eventData := encodeExecutionStateChangedV2Event(t, ccip_offramp.ExecutionStateChangedV2{
		MessageId: msgID,
		State:     ccip_offramp.MessageExecutionState_InProgress,
	})
	events := []logpoller.Event{
		{Data: eventData, TxSignature: solana.Signature{0x0A}, SlotNumber: 10},
		{Data: eventData, TxSignature: solana.Signature{0x0B}, SlotNumber: 11},
		{Data: eventData, TxSignature: solana.Signature{0x0C}, SlotNumber: 12},
	}
	failedOnce := false
	rpcClient := newMockRPCPollerSlotAndTx(t, func(context.Context, rpc.CommitmentType) (uint64, error) {
		return latestSlot, nil
	}, func(_ context.Context, sig solana.Signature) (*rpc.GetTransactionResult, error) {
		if sig == (solana.Signature{0x0B}) && !failedOnce {
			failedOnce = true
			return nil, errors.New("transient RPC timeout")
		}
		tx := buildExecuteV2Tx(t, testOfframpProgramID, msg, [][]byte{{0xcc}}, 150_000)
		return buildTxResult(t, tx, nil), nil
	})
	var fetchRanges [][2]uint64
	fetchCalls := 0
	lp := newLogPollerFetchEventsRunAndReturn(t, func(_ context.Context, from, to uint64) ([]logpoller.Event, error) {
		fetchRanges = append(fetchRanges, [2]uint64{from, to})
		fetchCalls++
		if fetchCalls == 1 {
			return events, nil
		}
		return events[1:], nil
	})
	p := newTestPoller(t, lp, rpcClient, testOfframpProgramID)
	p.lookbackWindow = 2 * minSolanaSlotTime

	p.runStartupSequence(context.Background())
	require.Equal(t, uint64(10), p.lastPolledSlot.Load())
	require.False(t, p.lookbackCovered.Load())

	require.NoError(t, p.pollOnce(context.Background()))
	require.Equal(t, [][2]uint64{{10, latestSlot}, {11, latestSlot}}, fetchRanges)
	require.Equal(t, latestSlot, p.lastPolledSlot.Load())
	require.True(t, p.lookbackCovered.Load())
}

func TestExecutionAttemptPollerPollOnceSetsCoverageTargetFromFirstPoll(t *testing.T) {
	t.Parallel()
	const fromSlot, toSlot = uint64(50), uint64(60)

	lp := newLogPollerFetchEventsRunAndReturn(t, func(_ context.Context, from, to uint64) ([]logpoller.Event, error) {
		require.Equal(t, fromSlot, from)
		require.Equal(t, toSlot, to)
		return nil, nil
	})
	rpcClient := newMockRPCPollerSlotAndTx(t, func(context.Context, rpc.CommitmentType) (uint64, error) {
		return toSlot, nil
	}, nil)

	p, err := NewExecutionAttemptPoller(ExecutionAttemptPollerParams{
		Lggr:             logger.Nop(),
		LogPoller:        lp,
		RPC:              rpcClient,
		OfframpProgramID: testOfframpProgramID,
	})
	require.NoError(t, err)
	p.lastPolledSlot.Store(slotBefore(fromSlot))
	require.Equal(t, coverageTargetUnset, p.coverageTarget.Load())

	require.NoError(t, p.pollOnce(context.Background()))

	require.Equal(t, toSlot, p.coverageTarget.Load())
	require.Equal(t, toSlot, p.lastPolledSlot.Load())
	require.True(t, p.lookbackCovered.Load())
}

func TestExecutionAttemptPollerCacheTTL(t *testing.T) {
	t.Parallel()
	const lookback = 200 * time.Millisecond
	tests := []struct {
		name     string
		cacheTTL time.Duration
		wantHit  bool
	}{
		{name: "ttl_follows_cache_ttl", cacheTTL: time.Minute, wantHit: true},
		{name: "ttl_defaults_to_lookback", cacheTTL: 0, wantHit: false},
		{name: "ttl_never_below_lookback", cacheTTL: time.Millisecond, wantHit: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p, err := NewExecutionAttemptPoller(ExecutionAttemptPollerParams{
				Lggr:             logger.Nop(),
				LogPoller:        newDefaultLogPollerForDest(t),
				RPC:              newDefaultDestMockRPC(t),
				OfframpProgramID: testOfframpProgramID,
				Config: Config{
					LookbackWindow:  *commonconfig.MustNewDuration(lookback),
					AttemptCacheTTL: *commonconfig.MustNewDuration(tc.cacheTTL),
				},
			})
			require.NoError(t, err)
			msgID := [32]byte{1}
			p.failedAttemptCache.Add(msgID, []protocol.ExecutionAttempt{{}})
			time.Sleep(3 * lookback)
			_, ok := p.failedAttemptCache.Get(msgID)
			require.Equal(t, tc.wantHit, ok)
		})
	}
}
