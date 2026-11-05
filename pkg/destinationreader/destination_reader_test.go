package destinationreader

import (
	"context"
	"errors"
	"math/big"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-ccv/protocol"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/ccip_offramp"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/rmn_remote"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/internal/mocks"
	"github.com/smartcontractkit/chainlink-ccip-solana/pkg/logpoller"
	"github.com/smartcontractkit/chainlink-ccip-solana/pkg/solutil"
)

// testOfframpProgramID is a random 32-byte key used wherever tests need a non-zero
// program ID without caring about the value. Initialized once at package load so all
// tests share the same key (avoids churn in error messages / diffs).
var testOfframpProgramID = solana.PublicKey(common.MakeRandom32ByteArray())
var testRMNRemoteProgramID = solana.PublicKey(common.MakeRandom32ByteArray())

func TestMain(m *testing.M) {
	solutil.SetProgramID(&ccip_offramp.ProgramID, ccip_offramp.ParseInstructionWithoutAccounts, testOfframpProgramID)
	os.Exit(m.Run())
}

// newDefaultParams returns a fully-populated Params with default mock RPC and
// log poller. Tests can override individual fields (e.g. p.RPC = customMock)
// without rebuilding the entire literal.
func newDefaultParams(t *testing.T) Params {
	t.Helper()
	return Params{
		Lggr:             logger.Nop(),
		ChainSelector:    protocol.ChainSelector(9837465928374658293),
		OfframpProgramID: testOfframpProgramID,
		RMNRemoteAddress: testRMNRemoteProgramID,
		RPC:              newDefaultDestMockRPC(t),
		LogPoller:        newDefaultLogPollerForDest(t),
	}
}

// encodeCursesAccount builds an rpc.GetAccountInfoResult containing a properly
// Borsh-encoded Curses account: the 8-byte Account_Curses discriminator
// followed by the Borsh-encoded Curses struct.
func encodeCursesAccount(t *testing.T, curses rmn_remote.Curses) *rpc.GetAccountInfoResult {
	t.Helper()
	body, err := curses.Marshal()
	require.NoError(t, err)
	data := append(append([]byte{}, rmn_remote.Account_Curses[:]...), body...)
	return &rpc.GetAccountInfoResult{
		Value: &rpc.Account{
			Data: rpc.DataBytesOrJSONFromBytes(data),
		},
	}
}

// newDefaultDestMockRPC returns a mockery RPCClient with permissive defaults matching the
// old &mockRPC{} fake: GetSlot (0,nil), GetTransaction errors, GetAccountInfo panics if called.
func newDefaultDestMockRPC(t *testing.T) *mocks.MockDestinationReaderRPCClient {
	t.Helper()
	m := &mocks.MockDestinationReaderRPCClient{}
	m.On("GetBlockWithOpts", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("unexpected GetBlockWithOpts")).Maybe()
	m.On("GetAccountInfo", mock.Anything, mock.Anything, mock.Anything).
		Return(func(context.Context, solana.PublicKey, rpc.CommitmentType) (*rpc.GetAccountInfoResult, error) {
			panic("unexpected call to GetAccountInfo")
		}).Maybe()
	m.On("GetSlot", mock.Anything, mock.Anything).Return(uint64(0), nil).Maybe()
	m.On("GetTransaction", mock.Anything, mock.Anything).
		Return(nil, errors.New("unexpected GetTransaction")).Maybe()
	m.On("SimulateTransactionWithOpts", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("unexpected SimulateTransactionWithOpts")).Maybe()
	t.Cleanup(func() { m.AssertExpectations(t) })
	return m
}

// newDefaultLogPollerForDest returns a mockery logpoller.Service with permissive defaults
// matching the old mockLogPoller{} value fake.
func newDefaultLogPollerForDest(t *testing.T) *mocks.MockService {
	t.Helper()
	m := &mocks.MockService{}
	m.On("Start", mock.Anything).Return(nil).Maybe()
	m.On("Close").Return(nil).Maybe()
	m.On("Ready").Return(nil).Maybe()
	m.On("Name").Return("MockLogPoller").Maybe()
	m.On("HealthReport").Return(map[string]error{"MockLogPoller": nil}).Maybe()
	m.On("FetchEvents", mock.Anything, mock.Anything, mock.Anything).Return(nil, nil).Maybe()
	t.Cleanup(func() { m.AssertExpectations(t) })
	return m
}

// newLogPollerNoFetchEvents stubs logpoller.Service lifecycle methods but leaves
// FetchEvents unset so an unexpected call fails the mock (strict).
func newLogPollerNoFetchEvents(t *testing.T) *mocks.MockService {
	t.Helper()
	m := &mocks.MockService{}
	m.On("Start", mock.Anything).Return(nil).Maybe()
	m.On("Close").Return(nil).Maybe()
	m.On("Ready").Return(nil).Maybe()
	m.On("Name").Return("MockLogPoller").Maybe()
	m.On("HealthReport").Return(map[string]error{"MockLogPoller": nil}).Maybe()
	t.Cleanup(func() { m.AssertExpectations(t) })
	return m
}

// newMockRPCPollerTxOnly wires RPCClient for poller tests that only exercise GetTransaction.
func newMockRPCPollerTxOnly(t *testing.T, getTx func(context.Context, solana.Signature) (*rpc.GetTransactionResult, error)) *mocks.MockDestinationReaderRPCClient {
	t.Helper()
	m := &mocks.MockDestinationReaderRPCClient{}
	m.On("GetBlockWithOpts", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("unexpected GetBlockWithOpts")).Maybe()
	m.On("GetAccountInfo", mock.Anything, mock.Anything, mock.Anything).
		Return(func(context.Context, solana.PublicKey, rpc.CommitmentType) (*rpc.GetAccountInfoResult, error) {
			panic("unexpected call to GetAccountInfo")
		}).Maybe()
	m.On("GetSlot", mock.Anything, mock.Anything).Return(uint64(0), nil).Maybe()
	m.On("GetTransaction", mock.Anything, mock.Anything).Return(getTx).Maybe()
	t.Cleanup(func() { m.AssertExpectations(t) })
	return m
}

// newLogPollerFetchEventsRunAndReturn wires FetchEvents (and permissive service methods) for poller tests.
func newLogPollerFetchEventsRunAndReturn(t *testing.T, fn func(context.Context, uint64, uint64) ([]logpoller.Event, error)) *mocks.MockService {
	t.Helper()
	m := &mocks.MockService{}
	m.On("Start", mock.Anything).Return(nil).Maybe()
	m.On("Close").Return(nil).Maybe()
	m.On("Ready").Return(nil).Maybe()
	m.On("Name").Return("MockLogPoller").Maybe()
	m.On("HealthReport").Return(map[string]error{"MockLogPoller": nil}).Maybe()
	m.On("FetchEvents", mock.Anything, mock.Anything, mock.Anything).Return(fn).Maybe()
	t.Cleanup(func() { m.AssertExpectations(t) })
	return m
}

// newMockRPCPollerSlotAndTx wires GetSlot and GetTransaction for poller startup / poll tests.
func newMockRPCPollerSlotAndTx(t *testing.T,
	getSlot func(context.Context, rpc.CommitmentType) (uint64, error),
	getTx func(context.Context, solana.Signature) (*rpc.GetTransactionResult, error),
) *mocks.MockDestinationReaderRPCClient {
	t.Helper()
	m := &mocks.MockDestinationReaderRPCClient{}
	m.On("GetBlockWithOpts", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("unexpected GetBlockWithOpts")).Maybe()
	m.On("GetAccountInfo", mock.Anything, mock.Anything, mock.Anything).
		Return(func(context.Context, solana.PublicKey, rpc.CommitmentType) (*rpc.GetAccountInfoResult, error) {
			panic("unexpected call to GetAccountInfo")
		}).Maybe()
	if getSlot != nil {
		m.On("GetSlot", mock.Anything, mock.Anything).Return(getSlot).Maybe()
	} else {
		m.On("GetSlot", mock.Anything, mock.Anything).Return(uint64(0), nil).Maybe()
	}
	if getTx != nil {
		m.On("GetTransaction", mock.Anything, mock.Anything).Return(getTx).Maybe()
	} else {
		m.On("GetTransaction", mock.Anything, mock.Anything).
			Return(nil, errors.New("unexpected GetTransaction")).Maybe()
	}
	t.Cleanup(func() { m.AssertExpectations(t) })
	return m
}

// newTestPoller returns a never-started ExecutionAttemptPoller wired to the supplied
// fakes. Pass newDefaultLogPollerForDest(t) and newDefaultDestMockRPC(t) for defaults.
// No goroutine leaks because Start is never called.
func newTestPoller(t *testing.T, lp logpoller.Service, rpcClient RPCClient, offrampProgramID solana.PublicKey) *ExecutionAttemptPoller {
	t.Helper()
	p, err := NewExecutionAttemptPoller(ExecutionAttemptPollerParams{
		Lggr:             logger.Nop(),
		LogPoller:        lp,
		RPC:              rpcClient,
		OfframpProgramID: offrampProgramID,
	})
	require.NoError(t, err)
	return p
}

// buildTestMessage constructs a protocol.Message with the given destination chain selector.
func buildTestMessage(t *testing.T, destSel protocol.ChainSelector) protocol.Message {
	t.Helper()
	return protocol.Message{
		SourceChainSelector:  protocol.ChainSelector(12463857294658392847),
		DestChainSelector:    destSel,
		SequenceNumber:       1,
		Version:              1,
		Sender:               make([]byte, 32),
		Receiver:             make([]byte, 32),
		OffRampAddress:       make([]byte, 32),
		OnRampAddress:        make([]byte, 32),
		Data:                 []byte{0xde, 0xad, 0xbe, 0xef},
		DataLength:           4,
		ReceiverLength:       32,
		SenderLength:         32,
		OffRampAddressLength: 32,
		OnRampAddressLength:  32,
	}
}

// encodeMessageState builds an rpc.GetAccountInfoResult containing a properly Borsh-encoded
// MessageState account with the given execution state. The 8-byte Account_MessageState
// discriminator is prepended before the Borsh-encoded struct.
func encodeMessageState(t *testing.T, execState ccip_offramp.MessageExecutionState) *rpc.GetAccountInfoResult {
	t.Helper()
	msgState := ccip_offramp.MessageState{
		Version:        1,
		Bump:           255,
		ExecutionState: execState,
		MessageId:      [32]byte{}, // empty just to simplify tests setup, this is not realistic
	}
	borshEncoded, err := msgState.Marshal()
	require.NoError(t, err)

	discrimLen := len(ccip_offramp.Account_MessageState)
	data := make([]byte, discrimLen+len(borshEncoded))
	copy(data[:discrimLen], ccip_offramp.Account_MessageState[:])
	copy(data[discrimLen:], borshEncoded)

	return &rpc.GetAccountInfoResult{
		Value: &rpc.Account{
			Data: rpc.DataBytesOrJSONFromBytes(data),
		},
	}
}

// TestGetMessageSuccess verifies all documented GetMessageSuccess behaviors.
func TestGetMessageSuccess(t *testing.T) {
	t.Parallel()
	const destSel = protocol.ChainSelector(9837465928374658293)

	tests := []struct {
		name         string
		getAccountFn func(ctx context.Context, acct solana.PublicKey, opts rpc.CommitmentType) (*rpc.GetAccountInfoResult, error)
		msgOverride  *protocol.Message // if non-nil, use instead of buildTestMessage(destSel)
		wantSuccess  bool
		wantErr      bool
		errContains  string
	}{
		{
			name: "happy_path_success_state",
			getAccountFn: func(_ context.Context, _ solana.PublicKey, _ rpc.CommitmentType) (*rpc.GetAccountInfoResult, error) {
				return encodeMessageState(t, ccip_offramp.MessageExecutionState_Success), nil
			},
			wantSuccess: true,
		},
		{
			name: "untouched_state",
			getAccountFn: func(_ context.Context, _ solana.PublicKey, _ rpc.CommitmentType) (*rpc.GetAccountInfoResult, error) {
				return encodeMessageState(t, ccip_offramp.MessageExecutionState_Untouched), nil
			},
			wantSuccess: false,
		},
		{
			name: "in_progress_state",
			getAccountFn: func(_ context.Context, _ solana.PublicKey, _ rpc.CommitmentType) (*rpc.GetAccountInfoResult, error) {
				return encodeMessageState(t, ccip_offramp.MessageExecutionState_InProgress), nil
			},
			wantSuccess: false,
		},
		{
			name: "failure_state",
			getAccountFn: func(_ context.Context, _ solana.PublicKey, _ rpc.CommitmentType) (*rpc.GetAccountInfoResult, error) {
				return encodeMessageState(t, ccip_offramp.MessageExecutionState_Failure), nil
			},
			wantSuccess: false,
		},
		{
			name: "missing_account_returns_false_nil",
			getAccountFn: func(_ context.Context, _ solana.PublicKey, _ rpc.CommitmentType) (*rpc.GetAccountInfoResult, error) {
				return nil, rpc.ErrNotFound
			},
			wantSuccess: false,
			wantErr:     false,
		},
		{
			// Prefunded PDA (1 lamport, no data): it exists but is not a MessageState.
			// Read it as "not executed", not a hard error, so the executor proceeds.
			name: "empty_data_prefunded_account_returns_false_nil",
			getAccountFn: func(_ context.Context, _ solana.PublicKey, _ rpc.CommitmentType) (*rpc.GetAccountInfoResult, error) {
				return &rpc.GetAccountInfoResult{
					Value: &rpc.Account{Data: rpc.DataBytesOrJSONFromBytes(nil)},
				}, nil
			},
			wantSuccess: false,
		},
		{
			name: "malformed_account_data_short",
			getAccountFn: func(_ context.Context, _ solana.PublicKey, _ rpc.CommitmentType) (*rpc.GetAccountInfoResult, error) {
				return &rpc.GetAccountInfoResult{
					Value: &rpc.Account{
						Data: rpc.DataBytesOrJSONFromBytes([]byte{0x01, 0x02, 0x03}),
					},
				}, nil
			},
			wantSuccess: false,
		},
		{
			name: "malformed_account_data_bad_discriminator",
			getAccountFn: func(_ context.Context, _ solana.PublicKey, _ rpc.CommitmentType) (*rpc.GetAccountInfoResult, error) {
				// Correct length but wrong discriminator bytes
				data := make([]byte, 10)
				data[0] = 0xFF
				data[1] = 0xFF
				return &rpc.GetAccountInfoResult{
					Value: &rpc.Account{
						Data: rpc.DataBytesOrJSONFromBytes(data),
					},
				}, nil
			},
			wantSuccess: false,
		},
		{
			name: "wrong_dest_chain",
			msgOverride: func() *protocol.Message {
				m := buildTestMessage(t, protocol.ChainSelector(99999))
				return &m
			}(),
			wantSuccess: false,
			wantErr:     true,
			errContains: "does not match",
		},
		{
			name: "rpc_error",
			getAccountFn: func(_ context.Context, _ solana.PublicKey, _ rpc.CommitmentType) (*rpc.GetAccountInfoResult, error) {
				return nil, errors.New("connection refused")
			},
			wantSuccess: false,
			wantErr:     true,
			errContains: "get message state",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rpcM := &mocks.MockDestinationReaderRPCClient{}
			if tc.getAccountFn != nil {
				rpcM.On("GetAccountInfo", mock.Anything, mock.Anything, mock.Anything).
					Return(tc.getAccountFn).Once()
			} else {
				rpcM.On("GetAccountInfo", mock.Anything, mock.Anything, mock.Anything).
					Return(func(context.Context, solana.PublicKey, rpc.CommitmentType) (*rpc.GetAccountInfoResult, error) {
						panic("unexpected call to GetAccountInfo")
					}).Maybe()
			}
			rpcM.On("GetSlot", mock.Anything, mock.Anything).Return(uint64(0), nil).Maybe()
			rpcM.On("GetTransaction", mock.Anything, mock.Anything).
				Return(nil, errors.New("unexpected GetTransaction")).Maybe()
			t.Cleanup(func() { rpcM.AssertExpectations(t) })

			p := newDefaultParams(t)
			p.RPC = rpcM
			dr, err := NewDestinationReader(p)
			require.NoError(t, err)

			msg := buildTestMessage(t, destSel)
			if tc.msgOverride != nil {
				msg = *tc.msgOverride
			}

			got, err := dr.GetMessageSuccess(context.Background(), msg)
			if tc.wantErr {
				require.Error(t, err)
				if tc.errContains != "" {
					require.Contains(t, err.Error(), tc.errContains)
				}
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.wantSuccess, got)
		})
	}
}

// TestGetCCVSForMessage verifies dest-chain validation before any RPC simulation.
func TestGetCCVSForMessage(t *testing.T) {
	t.Parallel()

	t.Run("wrong_dest_chain_errors", func(t *testing.T) {
		t.Parallel()
		dr, drErr := NewDestinationReader(newDefaultParams(t))
		require.NoError(t, drErr)

		_, gErr := dr.GetCCVSForMessage(context.Background(), buildTestMessage(t, protocol.ChainSelector(99999)))
		require.Error(t, gErr)
		require.Contains(t, gErr.Error(), "does not match")
	})
}

// newAttemptFor wraps a protocol.Message + CCVData into an ExecutionAttempt
// suitable for seeding the poller cache directly.
// newAttemptFor builds an attempt as the poller would: ccvs[i] is the verifier that
// produced ccvData[i]. The two slices must be the same length for the positional match
// in HasHonestAttempt to see the pairing.
func newAttemptFor(t *testing.T, msg protocol.Message, ccvs []protocol.UnknownAddress, ccvData [][]byte) protocol.ExecutionAttempt {
	t.Helper()
	require.Len(t, ccvs, len(ccvData), "fixture: CCVS and CCVData must be positionally aligned")
	return protocol.ExecutionAttempt{
		Report: protocol.AbstractAggregatedReport{
			Message: msg,
			CCVS:    ccvs,
			CCVData: ccvData,
		},
		// An honest submission always attaches a compute-unit limit. The gas-specific
		// cases below override it.
		TransactionGasLimit: big.NewInt(1_000_000),
	}
}

// TestHasHonestAttempt covers the per-CCV positional match: for each required/optional
// CCV address the caller supplies VerifierResults for, we find that CCV's slot in the
// attempt's CCVS and check the CCVData blob at that same slot against the known results.
// A blob only counts for the CCV it sits alongside, a blob in the wrong slot is not a
// match. Selection mirrors the on-chain quorum check: the configured list is walked in
// order, the first `threshold` entries present in the attempt are selected, and every
// selected slot's blob must be valid, a selected slot with an invalid blob aborts
// on-chain, so it reads as dishonest here too.
//
// Gas / CU comparison is intentionally absent, EVM-gas vs Solana-CU is a unit
// mismatch, so we do not assert on TransactionGasLimit here.
func TestHasHonestAttempt(t *testing.T) {
	t.Parallel()
	const destSel = protocol.ChainSelector(9837465928374658293)

	ccvA, err := protocol.NewUnknownAddressFromHex("0x" + hex32('a'))
	require.NoError(t, err)
	ccvB, err := protocol.NewUnknownAddressFromHex("0x" + hex32('b'))
	require.NoError(t, err)
	ccvC, err := protocol.NewUnknownAddressFromHex("0x" + hex32('c'))
	require.NoError(t, err)

	dataA := []byte("verifier-a-blob")
	dataB := []byte("verifier-b-blob")
	dataC := []byte("verifier-c-blob")

	vResult := func(addr protocol.UnknownAddress, data []byte) protocol.VerifierResult {
		return protocol.VerifierResult{
			VerifierDestAddress: addr,
			CCVData:             data,
		}
	}

	tests := []struct {
		name            string
		msgMutate       func(m *protocol.Message)
		seedAttempts    func(t *testing.T, msg protocol.Message) []protocol.ExecutionAttempt
		verifierResults []protocol.VerifierResult
		ccvAddressInfo  protocol.CCVAddressInfo
		want            bool
	}{
		{
			name:            "no_cached_attempts_returns_false",
			seedAttempts:    func(*testing.T, protocol.Message) []protocol.ExecutionAttempt { return nil },
			verifierResults: []protocol.VerifierResult{vResult(ccvA, dataA)},
			ccvAddressInfo:  protocol.CCVAddressInfo{RequiredCCVs: []protocol.UnknownAddress{ccvA}},
			want:            false,
		},
		{
			name: "empty_quorum_cannot_establish_honesty",
			seedAttempts: func(t *testing.T, msg protocol.Message) []protocol.ExecutionAttempt {
				return []protocol.ExecutionAttempt{newAttemptFor(t, msg, []protocol.UnknownAddress{ccvA}, [][]byte{dataA})}
			},
			verifierResults: []protocol.VerifierResult{vResult(ccvA, dataA)},
			ccvAddressInfo:  protocol.CCVAddressInfo{OptionalCCVs: []protocol.UnknownAddress{ccvA}},
			want:            false,
		},
		{
			name: "all_required_ccvs_match_returns_true",
			seedAttempts: func(t *testing.T, msg protocol.Message) []protocol.ExecutionAttempt {
				return []protocol.ExecutionAttempt{newAttemptFor(t, msg, []protocol.UnknownAddress{ccvA, ccvB}, [][]byte{dataA, dataB})}
			},
			verifierResults: []protocol.VerifierResult{vResult(ccvA, dataA), vResult(ccvB, dataB)},
			ccvAddressInfo:  protocol.CCVAddressInfo{RequiredCCVs: []protocol.UnknownAddress{ccvA, ccvB}},
			want:            true,
		},
		{
			name: "missing_one_required_ccv_returns_false",
			seedAttempts: func(t *testing.T, msg protocol.Message) []protocol.ExecutionAttempt {
				return []protocol.ExecutionAttempt{newAttemptFor(t, msg, []protocol.UnknownAddress{ccvA}, [][]byte{dataA})}
			},
			verifierResults: []protocol.VerifierResult{vResult(ccvA, dataA), vResult(ccvB, dataB)},
			ccvAddressInfo:  protocol.CCVAddressInfo{RequiredCCVs: []protocol.UnknownAddress{ccvA, ccvB}},
			want:            false,
		},
		{
			name: "order_independent_match",
			seedAttempts: func(t *testing.T, msg protocol.Message) []protocol.ExecutionAttempt {
				// Attempt slot order reversed from required order; pairings preserved.
				return []protocol.ExecutionAttempt{newAttemptFor(t, msg, []protocol.UnknownAddress{ccvB, ccvA}, [][]byte{dataB, dataA})}
			},
			verifierResults: []protocol.VerifierResult{vResult(ccvA, dataA), vResult(ccvB, dataB)},
			ccvAddressInfo:  protocol.CCVAddressInfo{RequiredCCVs: []protocol.UnknownAddress{ccvA, ccvB}},
			want:            true,
		},
		{
			name: "misattributed_blobs_return_false",
			seedAttempts: func(t *testing.T, msg protocol.Message) []protocol.ExecutionAttempt {
				// Both blobs are present but swapped relative to their CCVs: ccvA's slot
				// holds ccvB's data. A set-membership check would call this honest; the
				// positional check must not.
				return []protocol.ExecutionAttempt{newAttemptFor(t, msg, []protocol.UnknownAddress{ccvA, ccvB}, [][]byte{dataB, dataA})}
			},
			verifierResults: []protocol.VerifierResult{vResult(ccvA, dataA), vResult(ccvB, dataB)},
			ccvAddressInfo:  protocol.CCVAddressInfo{RequiredCCVs: []protocol.UnknownAddress{ccvA, ccvB}},
			want:            false,
		},
		{
			name: "ccv_absent_from_attempt_returns_false",
			seedAttempts: func(t *testing.T, msg protocol.Message) []protocol.ExecutionAttempt {
				// ccvB's blob is in the attempt but attributed to ccvC, so ccvB has no slot.
				return []protocol.ExecutionAttempt{newAttemptFor(t, msg, []protocol.UnknownAddress{ccvA, ccvC}, [][]byte{dataA, dataB})}
			},
			verifierResults: []protocol.VerifierResult{vResult(ccvA, dataA), vResult(ccvB, dataB)},
			ccvAddressInfo:  protocol.CCVAddressInfo{RequiredCCVs: []protocol.UnknownAddress{ccvA, ccvB}},
			want:            false,
		},
		{
			name: "optional_threshold_zero_with_required_match_returns_true",
			seedAttempts: func(t *testing.T, msg protocol.Message) []protocol.ExecutionAttempt {
				return []protocol.ExecutionAttempt{newAttemptFor(t, msg, []protocol.UnknownAddress{ccvA}, [][]byte{dataA})}
			},
			verifierResults: []protocol.VerifierResult{vResult(ccvA, dataA), vResult(ccvB, dataB)},
			ccvAddressInfo: protocol.CCVAddressInfo{
				RequiredCCVs:      []protocol.UnknownAddress{ccvA},
				OptionalCCVs:      []protocol.UnknownAddress{ccvB, ccvC},
				OptionalThreshold: 0,
			},
			want: true,
		},
		{
			name: "optional_threshold_met",
			seedAttempts: func(t *testing.T, msg protocol.Message) []protocol.ExecutionAttempt {
				return []protocol.ExecutionAttempt{newAttemptFor(t, msg, []protocol.UnknownAddress{ccvA, ccvB}, [][]byte{dataA, dataB})}
			},
			verifierResults: []protocol.VerifierResult{vResult(ccvA, dataA), vResult(ccvB, dataB), vResult(ccvC, dataC)},
			ccvAddressInfo: protocol.CCVAddressInfo{
				RequiredCCVs:      []protocol.UnknownAddress{ccvA},
				OptionalCCVs:      []protocol.UnknownAddress{ccvB, ccvC},
				OptionalThreshold: 1,
			},
			want: true,
		},
		{
			name: "optional_threshold_not_met",
			seedAttempts: func(t *testing.T, msg protocol.Message) []protocol.ExecutionAttempt {
				// Only ccvA's data present, no optional CCV data.
				return []protocol.ExecutionAttempt{newAttemptFor(t, msg, []protocol.UnknownAddress{ccvA}, [][]byte{dataA})}
			},
			verifierResults: []protocol.VerifierResult{vResult(ccvA, dataA), vResult(ccvB, dataB), vResult(ccvC, dataC)},
			ccvAddressInfo: protocol.CCVAddressInfo{
				RequiredCCVs:      []protocol.UnknownAddress{ccvA},
				OptionalCCVs:      []protocol.UnknownAddress{ccvB, ccvC},
				OptionalThreshold: 2,
			},
			want: false,
		},
		{
			name: "selected_optional_with_invalid_blob_is_not_honest",
			seedAttempts: func(t *testing.T, msg protocol.Message) []protocol.ExecutionAttempt {
				// The on-chain attack shape: configured optional [A, B] with threshold 1.
				// The attempt carries a garbage blob in A's slot and a valid blob in B's
				// slot. On-chain selects A (first in configured order) and aborts, so the
				// attempt is not honest. Counting any threshold entries would wrongly
				// call it honest.
				return []protocol.ExecutionAttempt{newAttemptFor(t, msg, []protocol.UnknownAddress{ccvA, ccvB}, [][]byte{[]byte("garbage"), dataB})}
			},
			verifierResults: []protocol.VerifierResult{vResult(ccvA, dataA), vResult(ccvB, dataB)},
			ccvAddressInfo: protocol.CCVAddressInfo{
				OptionalCCVs:      []protocol.UnknownAddress{ccvA, ccvB},
				OptionalThreshold: 1,
			},
			want: false,
		},
		{
			name: "optional_selection_follows_configured_order",
			seedAttempts: func(t *testing.T, msg protocol.Message) []protocol.ExecutionAttempt {
				// Attempt slot order reversed from configured order. ccvA is still
				// selected first (configured order, position lookup), and its slot holds
				// garbage, so the attempt aborts on-chain and is not honest, even
				// though ccvB's slot is valid.
				return []protocol.ExecutionAttempt{newAttemptFor(t, msg, []protocol.UnknownAddress{ccvB, ccvA}, [][]byte{dataB, []byte("garbage")})}
			},
			verifierResults: []protocol.VerifierResult{vResult(ccvA, dataA), vResult(ccvB, dataB)},
			ccvAddressInfo: protocol.CCVAddressInfo{
				OptionalCCVs:      []protocol.UnknownAddress{ccvA, ccvB},
				OptionalThreshold: 1,
			},
			want: false,
		},
		{
			name: "optional_slots_past_threshold_do_not_poison",
			seedAttempts: func(t *testing.T, msg protocol.Message) []protocol.ExecutionAttempt {
				// ccvA is selected and valid; ccvB's garbage slot is never verified
				// on-chain once the threshold is met, so it must not matter here.
				return []protocol.ExecutionAttempt{newAttemptFor(t, msg, []protocol.UnknownAddress{ccvA, ccvB}, [][]byte{dataA, []byte("garbage")})}
			},
			verifierResults: []protocol.VerifierResult{vResult(ccvA, dataA), vResult(ccvB, dataB)},
			ccvAddressInfo: protocol.CCVAddressInfo{
				OptionalCCVs:      []protocol.UnknownAddress{ccvA, ccvB},
				OptionalThreshold: 1,
			},
			want: true,
		},
		{
			name: "multi_attempt_one_matches_returns_true",
			seedAttempts: func(t *testing.T, msg protocol.Message) []protocol.ExecutionAttempt {
				return []protocol.ExecutionAttempt{
					newAttemptFor(t, msg, []protocol.UnknownAddress{ccvA}, [][]byte{dataA}),              // missing ccvB
					newAttemptFor(t, msg, []protocol.UnknownAddress{ccvA, ccvB}, [][]byte{dataA, dataB}), // full match
				}
			},
			verifierResults: []protocol.VerifierResult{vResult(ccvA, dataA), vResult(ccvB, dataB)},
			ccvAddressInfo:  protocol.CCVAddressInfo{RequiredCCVs: []protocol.UnknownAddress{ccvA, ccvB}},
			want:            true,
		},
		{
			name:            "under_supplied_gas_not_honest",
			msgMutate:       func(m *protocol.Message) { m.ExecutionGasLimit = 200_000; m.CcipReceiveGasLimit = 100_000 },
			verifierResults: []protocol.VerifierResult{vResult(ccvA, dataA), vResult(ccvB, dataB)},
			ccvAddressInfo:  protocol.CCVAddressInfo{RequiredCCVs: []protocol.UnknownAddress{ccvA, ccvB}},
			seedAttempts: func(t *testing.T, msg protocol.Message) []protocol.ExecutionAttempt {
				// Valid CCV data but a compute budget too small to execute. It must not
				// be treated as an honest, terminal attempt.
				a := newAttemptFor(t, msg, []protocol.UnknownAddress{ccvA, ccvB}, [][]byte{dataA, dataB})
				a.TransactionGasLimit = big.NewInt(10_000)
				return []protocol.ExecutionAttempt{a}
			},
			want: false,
		},
		{
			name:            "sufficient_gas_returns_true",
			msgMutate:       func(m *protocol.Message) { m.ExecutionGasLimit = 200_000; m.CcipReceiveGasLimit = 100_000 },
			verifierResults: []protocol.VerifierResult{vResult(ccvA, dataA), vResult(ccvB, dataB)},
			ccvAddressInfo:  protocol.CCVAddressInfo{RequiredCCVs: []protocol.UnknownAddress{ccvA, ccvB}},
			seedAttempts: func(t *testing.T, msg protocol.Message) []protocol.ExecutionAttempt {
				a := newAttemptFor(t, msg, []protocol.UnknownAddress{ccvA, ccvB}, [][]byte{dataA, dataB})
				a.TransactionGasLimit = big.NewInt(200_000)
				return []protocol.ExecutionAttempt{a}
			},
			want: true,
		},
		{
			name:            "nil_gas_limit_is_not_honest",
			verifierResults: []protocol.VerifierResult{vResult(ccvA, dataA), vResult(ccvB, dataB)},
			ccvAddressInfo:  protocol.CCVAddressInfo{RequiredCCVs: []protocol.UnknownAddress{ccvA, ccvB}},
			seedAttempts: func(t *testing.T, msg protocol.Message) []protocol.ExecutionAttempt {
				// A nil TransactionGasLimit means no SetComputeUnitLimit was attached.
				// The honest transmitter always sets one, so this is not an honest
				// attempt (a default-budget revert after InProgress is the bypass).
				a := newAttemptFor(t, msg, []protocol.UnknownAddress{ccvA, ccvB}, [][]byte{dataA, dataB})
				a.TransactionGasLimit = nil
				return []protocol.ExecutionAttempt{a}
			},
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			msg := buildTestMessage(t, destSel)
			if tc.msgMutate != nil {
				tc.msgMutate(&msg)
			}
			msgID, idErr := msg.MessageID()
			require.NoError(t, idErr)

			dr, drErr := NewDestinationReader(newDefaultParams(t))
			require.NoError(t, drErr)

			// Seed the internal poller's cache directly, bypasses the polling pipeline,
			// which is exercised in execution_attempt_poller_test.go.
			attempts := tc.seedAttempts(t, msg)
			if len(attempts) > 0 {
				dr.executionAttemptPoller.attemptCache.Add(msgID, attempts)
			}
			wantNotHonest := !tc.want && len(attempts) > 0

			got, hErr := dr.HasHonestAttempt(context.Background(), msg, tc.verifierResults, tc.ccvAddressInfo)
			require.NoError(t, hErr)
			require.Equal(t, tc.want, got.Honest)
			require.Equal(t, wantNotHonest, got.NotHonestAttemptExists)
		})
	}
}

// hex32 returns a 64-char hex string (32-byte address) filled with the given single hex char.
func hex32(c byte) string { return strings.Repeat(string(c), 64) }

// TestNewDestinationReader verifies constructor validation behavior.
func TestNewDestinationReader(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		mutate      func(*Params)
		wantErr     bool
		errContains string
	}{
		{
			name:        "nil_logger",
			mutate:      func(p *Params) { p.Lggr = nil },
			wantErr:     true,
			errContains: "logger is required",
		},
		{
			name: "zero_chain_selector",
			mutate: func(p *Params) {
				p.ChainSelector = 0
			},
			wantErr:     true,
			errContains: "chain selector is required",
		},
		{
			name:        "nil_rpc",
			mutate:      func(p *Params) { p.RPC = nil },
			wantErr:     true,
			errContains: "rpc client is required",
		},
		{
			name:        "nil_log_poller",
			mutate:      func(p *Params) { p.LogPoller = nil },
			wantErr:     true,
			errContains: "log poller is required",
		},
		{
			name: "zero_offramp_program_id",
			mutate: func(p *Params) {
				p.OfframpProgramID = solana.PublicKey{}
			},
			wantErr:     true,
			errContains: "offramp program ID is required",
		},
		{
			name: "zero_rmn_remote_address",
			mutate: func(p *Params) {
				p.RMNRemoteAddress = solana.PublicKey{}
			},
			wantErr:     true,
			errContains: "rmn remote address is required",
		},
		{
			name:    "valid_params",
			mutate:  func(*Params) {},
			wantErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := newDefaultParams(t)
			p.ChainSelector = 1
			tc.mutate(&p)
			dr, err := NewDestinationReader(p)
			if tc.wantErr {
				require.Error(t, err)
				if tc.errContains != "" {
					require.Contains(t, err.Error(), tc.errContains)
				}
				require.Nil(t, dr)
			} else {
				require.NoError(t, err)
				require.NotNil(t, dr)
			}
		})
	}
}

// TestDestinationReader_ServiceLifecycle verifies services.StateMachine wiring: Start/Close,
// duplicate Start rejection, HealthReport keys, and IsReady reflecting reader + poller.
func TestDestinationReader_ServiceLifecycle(t *testing.T) {
	t.Parallel()
	const destSel = protocol.ChainSelector(9837465928374658293)

	rpcM := &mocks.MockDestinationReaderRPCClient{}
	rpcM.On("GetAccountInfo", mock.Anything, mock.Anything, mock.Anything).
		Return(func(context.Context, solana.PublicKey, rpc.CommitmentType) (*rpc.GetAccountInfoResult, error) {
			panic("unexpected call to GetAccountInfo")
		}).Maybe()
	rpcM.On("GetSlot", mock.Anything, mock.Anything).Return(uint64(100), nil).Maybe()
	rpcM.On("GetTransaction", mock.Anything, mock.Anything).
		Return(nil, errors.New("unexpected GetTransaction")).Maybe()
	t.Cleanup(func() { rpcM.AssertExpectations(t) })

	lpM := &mocks.MockService{}
	lpM.On("Start", mock.Anything).Return(nil).Once()
	lpM.On("Close").Return(nil).Maybe()
	lpM.On("Ready").Return(nil).Maybe()
	lpM.On("Name").Return("MockLogPoller").Maybe()
	lpM.On("HealthReport").Return(map[string]error{"MockLogPoller": nil}).Maybe()
	lpM.On("FetchEvents", mock.Anything, mock.Anything, mock.Anything).Return(nil, nil).Maybe()
	t.Cleanup(func() { lpM.AssertExpectations(t) })

	p := newDefaultParams(t)
	p.RPC = rpcM
	p.LogPoller = lpM
	dr, err := NewDestinationReader(p)
	require.NoError(t, err)

	require.False(t, dr.IsReady(destSel), "reader must not be ready before Start")

	ctx := context.Background()
	require.NoError(t, dr.Start(ctx))
	defer func() { require.NoError(t, dr.Close()) }()

	require.Eventually(t, func() bool {
		return dr.IsReady(destSel)
	}, time.Second, time.Millisecond)
	require.NoError(t, dr.Ready())
	require.NoError(t, dr.CheckHealth(destSel), "CheckHealth nil when reader and poller are healthy")

	err = dr.Start(ctx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "already been started once")

	hr := dr.HealthReport()
	require.Len(t, hr, 2)
	require.Contains(t, hr, dr.Name())
	require.Contains(t, hr, destSel.String()+".ExecutionAttemptPoller")
}

func TestDestinationReader_RetriesExecutionPollerStartupFailure(t *testing.T) {
	t.Parallel()
	const destSel = protocol.ChainSelector(9837465928374658293)

	var closeCalls atomic.Uint32
	var slotCalls atomic.Uint64
	retryGate := make(chan struct{})
	lpM := &mocks.MockService{}
	lpM.On("Start", mock.Anything).Return(nil).Once()
	lpM.On("FetchEvents", mock.Anything, mock.Anything, mock.Anything).Return(nil, nil).Maybe()
	lpM.On("Close").Run(func(mock.Arguments) { closeCalls.Add(1) }).Return(nil).Once()
	lpM.On("Ready").Return(nil).Maybe()
	lpM.On("Name").Return("MockLogPoller").Maybe()
	lpM.On("HealthReport").Return(map[string]error{"MockLogPoller": nil}).Maybe()
	t.Cleanup(func() { lpM.AssertExpectations(t) })

	rpcM := &mocks.MockDestinationReaderRPCClient{}
	rpcM.On("GetAccountInfo", mock.Anything, mock.Anything, mock.Anything).
		Return(func(context.Context, solana.PublicKey, rpc.CommitmentType) (*rpc.GetAccountInfoResult, error) {
			panic("unexpected call to GetAccountInfo")
		}).Maybe()
	rpcM.On("GetSlot", mock.Anything, mock.Anything).Return(func(ctx context.Context, _ rpc.CommitmentType) (uint64, error) {
		if slotCalls.Add(1) == 1 {
			return 0, errors.New("slot rpc down")
		}
		select {
		case <-retryGate:
			return 100, nil
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	}).Maybe()
	rpcM.On("GetTransaction", mock.Anything, mock.Anything).
		Return(nil, errors.New("unexpected GetTransaction")).Maybe()
	t.Cleanup(func() { rpcM.AssertExpectations(t) })

	p := newDefaultParams(t)
	p.RPC = rpcM
	p.LogPoller = lpM
	dr, err := NewDestinationReader(p)
	require.NoError(t, err)
	dr.executionAttemptPoller.pollInterval = time.Millisecond

	require.NoError(t, dr.Start(context.Background()))
	t.Cleanup(func() { _ = dr.Close() })

	require.Eventually(t, func() bool {
		return slotCalls.Load() >= 2
	}, time.Second, time.Millisecond)
	require.NoError(t, dr.CheckHealth(destSel), "transient startup failures must not poison message processing")
	require.False(t, dr.IsReady(destSel), "Ready must stay false until backfill coverage")
	require.ErrorIs(t, dr.HealthReport()[dr.executionAttemptPollerHealthName()], ErrBackfillInProgress)

	close(retryGate)
	require.Eventually(t, func() bool {
		return dr.IsReady(destSel)
	}, time.Second, time.Millisecond)
	require.NoError(t, dr.CheckHealth(destSel))
	require.NoError(t, dr.HealthReport()[dr.executionAttemptPollerHealthName()])

	require.NoError(t, dr.Close())
	require.Equal(t, uint32(1), closeCalls.Load(), "logpoller must close once on reader shutdown")
}

func TestDestinationReader_HealthReportUsesReadyDuringBackfill(t *testing.T) {
	t.Parallel()
	const destSel = protocol.ChainSelector(9837465928374658293)

	started := make(chan struct{}, 1)
	release := make(chan struct{})
	rpcM := newMockRPCPollerSlotAndTx(t, func(ctx context.Context, _ rpc.CommitmentType) (uint64, error) {
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
	lpM := newLogPollerFetchEventsRunAndReturn(t, func(context.Context, uint64, uint64) ([]logpoller.Event, error) {
		return nil, nil
	})

	p := newDefaultParams(t)
	p.RPC = rpcM
	p.LogPoller = lpM
	dr, err := NewDestinationReader(p)
	require.NoError(t, err)

	require.NoError(t, dr.Start(context.Background()))
	defer func() { require.NoError(t, dr.Close()) }()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("poller startup did not start")
	}

	require.Eventually(t, func() bool {
		return errors.Is(dr.HealthReport()[dr.executionAttemptPollerHealthName()], ErrBackfillInProgress)
	}, time.Second, time.Millisecond)
	require.NoError(t, dr.CheckHealth(destSel), "Healthy must ignore backfill progress")
	require.False(t, dr.IsReady(destSel), "Ready must gate on backfill progress")
	require.NoError(t, dr.HealthReport()[dr.Name()])

	close(release)
	require.Eventually(t, func() bool {
		return dr.IsReady(destSel)
	}, time.Second, time.Millisecond)
	require.NoError(t, dr.HealthReport()[dr.executionAttemptPollerHealthName()])
}

func TestGetRMNCursedSubjects(t *testing.T) {
	t.Parallel()

	globalCurseSubject := rmn_remote.CurseSubject{
		Value: [16]uint8{0x01, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x01},
	}
	// On Solana, chain selectors are stored as LE uint64 in bytes [0:8].
	chainSelectorSubject := rmn_remote.CurseSubject{
		Value: [16]uint8{0x08, 0x07, 0x06, 0x05, 0x04, 0x03, 0x02, 0x01, 0, 0, 0, 0, 0, 0, 0, 0},
	}

	expectedGlobalCurse := protocol.Bytes16{0x01, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x01}
	// Normalized to BE in [8:16] with [0:8] zeroed.
	expectedChainSelector := protocol.Bytes16{
		0, 0, 0, 0, 0, 0, 0, 0,
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
	}

	tests := []struct {
		name         string
		getAccountFn func(context.Context, solana.PublicKey, rpc.CommitmentType) (*rpc.GetAccountInfoResult, error)
		wantSubjects []protocol.Bytes16
		wantErr      bool
		errContains  string
	}{
		{
			name: "returns_cursed_subjects_from_chain",
			getAccountFn: func(_ context.Context, _ solana.PublicKey, _ rpc.CommitmentType) (*rpc.GetAccountInfoResult, error) {
				return encodeCursesAccount(t, rmn_remote.Curses{
					Version:        1,
					CursedSubjects: []rmn_remote.CurseSubject{globalCurseSubject, chainSelectorSubject},
				}), nil
			},
			wantSubjects: []protocol.Bytes16{expectedGlobalCurse, expectedChainSelector},
		},
		{
			name: "not_found_returns_error",
			getAccountFn: func(_ context.Context, _ solana.PublicKey, _ rpc.CommitmentType) (*rpc.GetAccountInfoResult, error) {
				return nil, rpc.ErrNotFound
			},
			wantErr:     true,
			errContains: "get RMN curses account",
		},
		{
			name: "rpc_error_propagates",
			getAccountFn: func(_ context.Context, _ solana.PublicKey, _ rpc.CommitmentType) (*rpc.GetAccountInfoResult, error) {
				return nil, errors.New("connection refused")
			},
			wantErr:     true,
			errContains: "get RMN curses account",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m := &mocks.MockDestinationReaderRPCClient{}
			m.On("GetAccountInfo", mock.Anything, mock.Anything, mock.Anything).
				Return(tc.getAccountFn).Once()
			m.On("GetSlot", mock.Anything, mock.Anything).Return(uint64(0), nil).Maybe()
			m.On("GetTransaction", mock.Anything, mock.Anything).
				Return(nil, errors.New("unexpected GetTransaction")).Maybe()
			m.On("GetBlockWithOpts", mock.Anything, mock.Anything, mock.Anything).
				Return(nil, errors.New("unexpected GetBlockWithOpts")).Maybe()
			m.On("SimulateTransactionWithOpts", mock.Anything, mock.Anything, mock.Anything).
				Return(nil, errors.New("unexpected SimulateTransactionWithOpts")).Maybe()
			t.Cleanup(func() { m.AssertExpectations(t) })

			p := newDefaultParams(t)
			p.RPC = m
			dr, err := NewDestinationReader(p)
			require.NoError(t, err)

			got, gErr := dr.GetRMNCursedSubjects(context.Background())
			if tc.wantErr {
				require.Error(t, gErr)
				if tc.errContains != "" {
					require.Contains(t, gErr.Error(), tc.errContains)
				}
				return
			}
			require.NoError(t, gErr)
			require.Equal(t, tc.wantSubjects, got)
		})
	}
}

func TestGetExecutionAttempts_ReturnsCachedCopy(t *testing.T) {
	t.Parallel()
	const destSel = protocol.ChainSelector(9837465928374658293)
	dr, err := NewDestinationReader(newDefaultParams(t))
	require.NoError(t, err)

	msg := buildTestMessage(t, destSel)
	msgID, idErr := msg.MessageID()
	require.NoError(t, idErr)
	want := []protocol.ExecutionAttempt{newAttemptFor(t, msg, []protocol.UnknownAddress{{0x01}}, [][]byte{[]byte("blob")})}
	dr.executionAttemptPoller.attemptCache.Add(msgID, want)

	got, gErr := dr.GetExecutionAttempts(context.Background(), msg)
	require.NoError(t, gErr)
	require.Len(t, got, 1)
	require.Equal(t, want[0].Report.CCVData, got[0].Report.CCVData)

	other := buildTestMessage(t, destSel)
	other.SequenceNumber = 2
	gotMiss, missErr := dr.GetExecutionAttempts(context.Background(), other)
	require.NoError(t, missErr)
	require.Nil(t, gotMiss, "cache miss stays (nil, nil) through DestinationReader")
}

func TestCheckHealth_NotStarted(t *testing.T) {
	t.Parallel()
	const destSel = protocol.ChainSelector(9837465928374658293)
	dr, err := NewDestinationReader(newDefaultParams(t))
	require.NoError(t, err)
	chErr := dr.CheckHealth(destSel)
	require.Error(t, chErr, "executor treats non-nil CheckHealth as critical before message work")
}

func TestHasHonestAttempt_MessageIDError(t *testing.T) {
	t.Parallel()
	const destSel = protocol.ChainSelector(9837465928374658293)
	dr, err := NewDestinationReader(newDefaultParams(t))
	require.NoError(t, err)

	bad := buildTestMessage(t, destSel)
	bad.DataLength = 99
	bad.Data = []byte{1, 2, 3}

	ccv, err := protocol.NewUnknownAddressFromHex("0x" + hex32('a'))
	require.NoError(t, err)
	quorum := protocol.CCVAddressInfo{RequiredCCVs: []protocol.UnknownAddress{ccv}}

	got, hErr := dr.HasHonestAttempt(context.Background(), bad, nil, quorum)
	require.Error(t, hErr)
	require.Contains(t, hErr.Error(), "compute message ID")
	require.Zero(t, got)
}

func TestHasHonestAttempt_EmptyQuorumMalformedMessage(t *testing.T) {
	t.Parallel()
	const destSel = protocol.ChainSelector(9837465928374658293)
	dr, err := NewDestinationReader(newDefaultParams(t))
	require.NoError(t, err)

	bad := buildTestMessage(t, destSel)
	bad.DataLength = 99
	bad.Data = []byte{1, 2, 3}

	got, hErr := dr.HasHonestAttempt(context.Background(), bad, nil, protocol.CCVAddressInfo{})
	require.ErrorContains(t, hErr, "compute message ID")
	require.Zero(t, got)
}
