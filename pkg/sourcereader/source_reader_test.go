package sourcereader

import (
	"context"
	"errors"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/gagliardetto/solana-go/rpc/jsonrpc"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	chainsel "github.com/smartcontractkit/chain-selectors"
	"github.com/smartcontractkit/chainlink-ccv/protocol"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/rmn_remote"
	"github.com/smartcontractkit/chainlink-ccip-solana/internal/mocks"
	"github.com/smartcontractkit/chainlink-ccip-solana/pkg/logpoller"
)

// Source selector that matches the hardcoded switch in FetchMessageSentEvents
// so the reader doesn't panic during tests.
var testSourceSelector = protocol.ChainSelector(
	chainsel.TEST_22222222222222222222222222222222222222222222.Selector,
)

var noopParser MessageSentEventParser = func([]byte) (protocol.MessageSentEvent, bool, error) {
	return protocol.MessageSentEvent{}, true, nil
}

// newDefaultMockRPC stubs RPCClient for source-reader tests that only need slot
// defaults and never call GetBlockWithOpts (or expect it to error if called).
func newDefaultMockRPC(t *testing.T) *mocks.MockSourceReaderRPCClient {
	t.Helper()
	m := &mocks.MockSourceReaderRPCClient{}
	m.On("GetBlockWithOpts", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("GetBlockWithOpts not stubbed")).Maybe()
	m.On("GetSlot", mock.Anything, mock.Anything).
		Return(uint64(0), nil).Maybe()
	m.On("GetAccountInfo", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("GetAccountInfo not stubbed")).Maybe()
	m.On("GetTransaction", mock.Anything, mock.Anything).
		Return(nil, errors.New("GetTransaction not stubbed")).Maybe()
	t.Cleanup(func() { m.AssertExpectations(t) })
	return m
}

// newUnusedLogPollerMock is a logpoller.Service mock with no expectations (fails if called).
func newUnusedLogPollerMock(t *testing.T) *mocks.MockService {
	t.Helper()
	m := &mocks.MockService{}
	t.Cleanup(func() { m.AssertExpectations(t) })
	return m
}

// newReader is a thin constructor helper used by table cases that don't
// need to override individual wiring.
func newReader(t *testing.T, lp logpoller.Service, rpcClient RPCClient, parser MessageSentEventParser) *SolanaSourceReader {
	t.Helper()
	sr, err := NewSolanaSourceReader(
		solana.NewWallet().PublicKey(),
		solana.NewWallet().PublicKey(),
		testSourceSelector,
		rpcClient,
		lp,
		logger.Test(t),
		parser,
	)
	require.NoError(t, err)
	return sr.(*SolanaSourceReader)
}

type readerArgs struct {
	onRamp   solana.PublicKey
	rmn      solana.PublicKey
	selector protocol.ChainSelector
	rpc      RPCClient
	lp       logpoller.Service
	parser   MessageSentEventParser
}

func validReaderArgs(t *testing.T) readerArgs {
	t.Helper()
	return readerArgs{
		onRamp:   solana.NewWallet().PublicKey(),
		rmn:      solana.NewWallet().PublicKey(),
		selector: testSourceSelector,
		rpc:      newDefaultMockRPC(t),
		lp:       newUnusedLogPollerMock(t),
		parser:   noopParser,
	}
}

func TestNewSolanaSourceReader_Validation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		mutate  func(*readerArgs)
		wantErr string
	}{
		{"all valid", func(*readerArgs) {}, ""},
		{"zero onramp", func(a *readerArgs) { a.onRamp = solana.PublicKey{} }, "onRampAddress is not set"},
		{"zero rmn", func(a *readerArgs) { a.rmn = solana.PublicKey{} }, "rmnRemoteAddress is not set"},
		{"zero selector", func(a *readerArgs) { a.selector = 0 }, "chainSelector is not set"},
		{"nil rpc", func(a *readerArgs) { a.rpc = nil }, "rpcClient is not set"},
		{"nil logpoller", func(a *readerArgs) { a.lp = nil }, "logPoller is not set"},
		{"nil parser", func(a *readerArgs) { a.parser = nil }, "eventParser is not set"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			args := validReaderArgs(t)
			tc.mutate(&args)
			_, err := NewSolanaSourceReader(
				args.onRamp,
				args.rmn,
				args.selector,
				args.rpc,
				args.lp,
				logger.Test(t),
				args.parser,
			)
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestFetchMessageSentEvents(t *testing.T) {
	t.Parallel()
	// Reused signatures/events so the table stays readable.
	sig1 := solana.Signature{0x01, 0x02, 0x03}
	sig2 := solana.Signature{0x04, 0x05, 0x06}
	sig3 := solana.Signature{0x07, 0x08, 0x09}

	goodEvent := func(slot uint64, sig solana.Signature, tag byte) logpoller.Event {
		return logpoller.Event{Data: []byte{tag}, SlotNumber: slot, TxSignature: sig}
	}
	// A message event that cannot be decoded.
	badEvent := func(slot uint64, sig solana.Signature) logpoller.Event {
		return logpoller.Event{Data: []byte("BAD"), SlotNumber: slot, TxSignature: sig}
	}
	// A sibling router CPI event (receiver registration, dest-chain admin, and
	// so on): not a message event. The reader filters it instead of failing.
	skipEvent := func(slot uint64, sig solana.Signature) logpoller.Event {
		return logpoller.Event{Data: []byte("SKIP"), SlotNumber: slot, TxSignature: sig}
	}

	// Tag-based parser: the MessageID's first byte mirrors the raw data's first
	// byte. The two sentinels stand in for the classes the real parser
	// distinguishes: "BAD" is an unreadable message event, "SKIP" is not a
	// message event.
	parseErr := errors.New("synthetic parse failure")
	taggingParser := MessageSentEventParser(func(data []byte) (protocol.MessageSentEvent, bool, error) {
		switch string(data) {
		case "BAD":
			return protocol.MessageSentEvent{}, true, parseErr
		case "SKIP":
			return protocol.MessageSentEvent{}, false, nil
		}
		var id protocol.Bytes32
		id[0] = data[0]
		return protocol.MessageSentEvent{MessageID: id}, true, nil
	})

	lpErr := errors.New("logpoller exploded")

	tests := []struct {
		name     string
		events   []logpoller.Event
		lpErr    error
		parser   MessageSentEventParser
		rpcSetup func(t *testing.T) RPCClient
		from     uint64
		to       uint64

		wantErrSubstr string
		wantErrIs     error
		wantLPCalls   int
		wantLPFrom    uint64
		wantLPTo      uint64
		assertEvents  func(t *testing.T, events []protocol.MessageSentEvent)
	}{
		{
			name:        "attaches block number and tx hash from raw event",
			events:      []logpoller.Event{goodEvent(4242, sig1, 0xAA)},
			parser:      taggingParser,
			from:        1000,
			to:          5000,
			wantLPCalls: 1, wantLPFrom: 1000, wantLPTo: 5000,
			assertEvents: func(t *testing.T, events []protocol.MessageSentEvent) {
				require.Len(t, events, 1)
				require.Equal(t, uint64(4242), events[0].BlockNumber)
				require.Equal(t, protocol.ByteSlice(sig1[:]), events[0].TxHash)
				require.Equal(t, byte(0xAA), events[0].MessageID[0])
			},
		},
		{
			// Sibling router events come through the same program-scoped
			// logpoller filter on every admin op and receiver registration.
			// Filtering them is routine and must not disturb the range.			name: "skips sibling router events and keeps message events",
			events: []logpoller.Event{
				goodEvent(10, sig1, 0x01),
				skipEvent(11, sig2),
				goodEvent(12, sig3, 0x03),
			},
			parser:      taggingParser,
			from:        0,
			to:          100,
			wantLPCalls: 1, wantLPFrom: 0, wantLPTo: 100,
			assertEvents: func(t *testing.T, events []protocol.MessageSentEvent) {
				require.Len(t, events, 2)
				require.Equal(t, uint64(10), events[0].BlockNumber)
				require.Equal(t, protocol.ByteSlice(sig1[:]), events[0].TxHash)
				require.Equal(t, uint64(12), events[1].BlockNumber)
				require.Equal(t, protocol.ByteSlice(sig3[:]), events[1].TxHash)
			},
		},
		{
			// An undecodable CcipMessageSentV2 must abort the fetch. The caller advances
			// its slot cursor on any nil error. Returning the other two events here would
			// silently drop the message at slot 11 forever.
			name: "undecodable message event aborts the fetch without partial results",
			events: []logpoller.Event{
				goodEvent(10, sig1, 0x01),
				badEvent(11, sig2),
				goodEvent(12, sig3, 0x03),
			},
			parser:      taggingParser,
			from:        0,
			to:          100,
			wantLPCalls: 1, wantLPFrom: 0, wantLPTo: 100,
			// Slot and signature must be in the message: they are the only handle
			// an operator has on the message that stalled the lane.
			wantErrSubstr: "decode CcipMessageSentV2 at slot 11 tx " + sig2.String(),
			wantErrIs:     parseErr,
			assertEvents: func(t *testing.T, events []protocol.MessageSentEvent) {
				require.Nil(t, events)
			},
		},
		{
			name:        "empty logpoller result returns no events",
			events:      nil,
			parser:      taggingParser,
			from:        0,
			to:          10,
			wantLPCalls: 1, wantLPFrom: 0, wantLPTo: 10,
			assertEvents: func(t *testing.T, events []protocol.MessageSentEvent) {
				require.Empty(t, events)
			},
		},
		{
			name:   "zero toBlock falls back to finalized slot from rpc",
			events: nil,
			parser: taggingParser,
			rpcSetup: func(t *testing.T) RPCClient {
				m := &mocks.MockSourceReaderRPCClient{}
				m.On("GetBlockWithOpts", mock.Anything, mock.Anything, mock.Anything).
					Return(nil, errors.New("GetBlockWithOpts not stubbed")).Maybe()
				m.On("GetSlot", mock.Anything, rpc.CommitmentFinalized).
					Return(uint64(999), nil).Once()
				m.On("GetAccountInfo", mock.Anything, mock.Anything, mock.Anything).
					Return(nil, errors.New("GetAccountInfo not stubbed")).Maybe()
				m.On("GetTransaction", mock.Anything, mock.Anything).
					Return(nil, errors.New("GetTransaction not stubbed")).Maybe()
				t.Cleanup(func() { m.AssertExpectations(t) })
				return m
			},
			from:        0,
			to:          0,
			wantLPCalls: 1, wantLPFrom: 0, wantLPTo: 999,
			assertEvents: func(t *testing.T, events []protocol.MessageSentEvent) {
				require.Empty(t, events)
			},
		},
		{
			name:          "inverted slot range is rejected without hitting logpoller",
			events:        nil,
			parser:        taggingParser,
			from:          500,
			to:            100,
			wantErrSubstr: "invalid slot range",
			wantLPCalls:   0,
		},
		{
			name:          "logpoller error propagates",
			events:        nil,
			lpErr:         lpErr,
			parser:        taggingParser,
			from:          0,
			to:            10,
			wantErrSubstr: "fetch events",
			wantLPCalls:   1, wantLPFrom: 0, wantLPTo: 10,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			lp := &mocks.MockService{}
			if tc.wantLPCalls > 0 {
				lp.On("FetchEvents", mock.Anything, tc.wantLPFrom, tc.wantLPTo).
					Return(tc.events, tc.lpErr).Once()
			}
			t.Cleanup(func() { lp.AssertExpectations(t) })

			var rpcClient RPCClient
			if tc.rpcSetup != nil {
				rpcClient = tc.rpcSetup(t)
			} else {
				rpcClient = newDefaultMockRPC(t)
			}

			sr := newReader(t, lp, rpcClient, tc.parser)

			events, err := sr.FetchMessageSentEvents(t.Context(), tc.from, tc.to)

			if tc.wantErrSubstr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.wantErrSubstr)
			} else {
				require.NoError(t, err)
			}
			if tc.wantErrIs != nil {
				require.ErrorIs(t, err, tc.wantErrIs)
			}

			if tc.assertEvents != nil {
				tc.assertEvents(t, events)
			}
		})
	}
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
				curses := rmn_remote.Curses{
					Version:        1,
					CursedSubjects: []rmn_remote.CurseSubject{globalCurseSubject, chainSelectorSubject},
				}
				body, err := curses.Marshal()
				require.NoError(t, err)
				data := append(append([]byte{}, rmn_remote.Account_Curses[:]...), body...)
				return &rpc.GetAccountInfoResult{
					Value: &rpc.Account{
						Data: rpc.DataBytesOrJSONFromBytes(data),
					},
				}, nil
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

			m := &mocks.MockSourceReaderRPCClient{}
			m.On("GetAccountInfo", mock.Anything, mock.Anything, mock.Anything).
				Return(tc.getAccountFn).Once()
			m.On("GetSlot", mock.Anything, mock.Anything).
				Return(uint64(0), nil).Maybe()
			m.On("GetBlockWithOpts", mock.Anything, mock.Anything, mock.Anything).
				Return(nil, errors.New("GetBlockWithOpts not stubbed")).Maybe()
			m.On("GetTransaction", mock.Anything, mock.Anything).
				Return(nil, errors.New("GetTransaction not stubbed")).Maybe()
			t.Cleanup(func() { m.AssertExpectations(t) })

			sr := newReader(t, newUnusedLogPollerMock(t), m, noopParser)

			got, err := sr.GetRMNCursedSubjects(t.Context())
			if tc.wantErr {
				require.Error(t, err)
				if tc.errContains != "" {
					require.Contains(t, err.Error(), tc.errContains)
				}
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantSubjects, got)
		})
	}
}

func TestGetBlocksHeaders(t *testing.T) {
	t.Parallel()

	slot1 := uint64(100)
	slot2 := uint64(200)
	slot3 := uint64(300)
	blockTime := solana.UnixTimeSeconds(1700000000)

	block1 := &rpc.GetBlockResult{
		Blockhash:         solana.Hash{0xAA, 0xBB},
		PreviousBlockhash: solana.Hash{0x11, 0x22},
		BlockTime:         &blockTime,
	}
	wantHeader1 := protocol.BlockHeader{
		Number:     slot1,
		Hash:       protocol.Bytes32{0xAA, 0xBB},
		ParentHash: protocol.Bytes32{0x11, 0x22},
		Timestamp:  block1.BlockTime.Time(),
	}
	wantHeader3 := wantHeader1
	wantHeader3.Number = slot3

	blockMock := func(t *testing.T, stubs func(m *mocks.MockSourceReaderRPCClient)) RPCClient {
		t.Helper()
		m := &mocks.MockSourceReaderRPCClient{}
		stubs(m)
		t.Cleanup(func() { m.AssertExpectations(t) })
		return m
	}

	tests := []struct {
		name          string
		blockNumbers  []uint64
		rpcSetup      func(t *testing.T) RPCClient
		wantErrSubstr string
		wantHeaders   map[uint64]protocol.BlockHeader
	}{
		{
			name:         "returns headers for requested slots",
			blockNumbers: []uint64{slot1},
			rpcSetup: func(t *testing.T) RPCClient {
				return blockMock(t, func(m *mocks.MockSourceReaderRPCClient) {
					m.On("GetBlockWithOpts", mock.Anything, slot1, mock.Anything).
						Return(block1, nil).Once()
				})
			},
			wantHeaders: map[uint64]protocol.BlockHeader{slot1: wantHeader1},
		},
		{
			name:         "null block omitted from result",
			blockNumbers: []uint64{slot1, slot2, slot3},
			rpcSetup: func(t *testing.T) RPCClient {
				return blockMock(t, func(m *mocks.MockSourceReaderRPCClient) {
					m.On("GetBlockWithOpts", mock.Anything, slot1, mock.Anything).
						Return(block1, nil).Once()
					m.On("GetBlockWithOpts", mock.Anything, slot2, mock.Anything).
						Return(nil, rpc.ErrNotConfirmed).Once()
					m.On("GetBlockWithOpts", mock.Anything, slot3, mock.Anything).
						Return(block1, nil).Once()
				})
			},
			wantHeaders: map[uint64]protocol.BlockHeader{
				slot1: wantHeader1,
				slot3: wantHeader3,
			},
		},
		{
			name:         "skipped slot rpc error propagates",
			blockNumbers: []uint64{slot1, slot2},
			rpcSetup: func(t *testing.T) RPCClient {
				return blockMock(t, func(m *mocks.MockSourceReaderRPCClient) {
					m.On("GetBlockWithOpts", mock.Anything, slot1, mock.Anything).
						Return(block1, nil).Once()
					m.On("GetBlockWithOpts", mock.Anything, slot2, mock.Anything).
						Return(nil, &jsonrpc.RPCError{Code: -32007, Message: "Slot 200 was skipped, or missing due to ledger jump to recent snapshot"}).Once()
				})
			},
			wantErrSubstr: "get block header for slot 200",
		},
		{
			name:         "rpc error propagates instead of swallowing",
			blockNumbers: []uint64{slot1, slot2},
			rpcSetup: func(t *testing.T) RPCClient {
				return blockMock(t, func(m *mocks.MockSourceReaderRPCClient) {
					m.On("GetBlockWithOpts", mock.Anything, slot1, mock.Anything).
						Return(block1, nil).Once()
					m.On("GetBlockWithOpts", mock.Anything, slot2, mock.Anything).
						Return(nil, errors.New("rpc timeout")).Once()
				})
			},
			wantErrSubstr: "get block header for slot 200",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rpcClient := tc.rpcSetup(t)
			sr := newReader(t, newUnusedLogPollerMock(t), rpcClient, noopParser)

			headers, err := sr.GetBlocksHeaders(t.Context(), tc.blockNumbers)

			if tc.wantErrSubstr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.wantErrSubstr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantHeaders, headers)
		})
	}
}
