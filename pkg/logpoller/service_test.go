package logpoller_test

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	lptypes "github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller/types"

	"github.com/smartcontractkit/chainlink-ccip-solana/internal/mocks"
	"github.com/smartcontractkit/chainlink-ccip-solana/pkg/logpoller"
)

const mockLogCollectorName = "mockLogCollector"

func TestServiceHealthReport(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		healthErr error
	}{
		{"healthy", nil},
		{"unhealthy", errors.New("maybe worker pool stuck")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			collector := mocks.NewMockLogCollector(t)
			collector.On("HealthReport").Return(map[string]error{mockLogCollectorName: tc.healthErr}).Once()

			svc, err := logpoller.NewServiceWithLogCollector(collector, solana.PublicKey{0x01}, logger.Nop())
			require.NoError(t, err)

			report := svc.HealthReport()
			require.ErrorIs(t, report[mockLogCollectorName], tc.healthErr)
		})
	}
}

func TestServiceFetchEventsFiltersAndDecodesCPIEvents(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	programID := solana.PublicKey{0x01}
	otherProgramID := solana.PublicKey{0x02}
	signature := solana.Signature{0xAA}
	payload := []byte{0x10, 0x20, 0x30}

	blocks := []lptypes.Block{
		{
			SlotNumber: 99,
			Events: []lptypes.ProgramEvent{
				{
					Program: programID.String(),
					Data:    base64.StdEncoding.EncodeToString(payload),
					IsCPI:   false,
				},
				{
					Program: otherProgramID.String(),
					Data:    base64.StdEncoding.EncodeToString(payload),
					IsCPI:   true,
				},
			},
		},
		{
			SlotNumber: 123,
			Events: []lptypes.ProgramEvent{
				{
					Program:   programID.String(),
					Data:      base64.StdEncoding.EncodeToString(payload),
					IsCPI:     true,
					BlockData: lptypes.BlockData{SlotNumber: 123, TransactionHash: signature},
				},
			},
		},
	}

	var cleanupCalled bool
	collector := mocks.NewMockLogCollector(t)
	collector.On("BackfillForAddresses", mock.Anything, mock.Anything, uint64(10), uint64(20)).
		Return(func(_ context.Context, addresses []lptypes.PublicKey, fromSlot, toSlot uint64) (<-chan lptypes.Block, func(), error) {
			require.Equal(t, []lptypes.PublicKey{lptypes.PublicKey(programID)}, addresses)
			require.Equal(t, uint64(10), fromSlot)
			require.Equal(t, uint64(20), toSlot)

			ch := make(chan lptypes.Block, len(blocks))
			for _, b := range blocks {
				ch <- b
			}
			close(ch)
			return ch, func() { cleanupCalled = true }, nil
		}).Once()

	svc, err := logpoller.NewServiceWithLogCollector(collector, programID, logger.Nop())
	require.NoError(t, err)

	events, err := svc.FetchEvents(ctx, 10, 20)
	require.NoError(t, err)
	require.True(t, cleanupCalled)

	require.Len(t, events, 1)
	require.Equal(t, programID, events[0].Program)
	require.Equal(t, payload, events[0].Data)
	require.Equal(t, uint64(123), events[0].SlotNumber)
	require.Equal(t, signature, events[0].TxSignature)
}

func TestServiceFetchEventsLogsOnlyNonEmptyResultsAtInfo(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name          string
		blocks        []lptypes.Block
		expectedCount int
		expectedLogs  int
	}{
		{
			name: "no events",
			blocks: []lptypes.Block{
				{SlotNumber: 10},
			},
		},
		{
			name: "matched event",
			blocks: []lptypes.Block{
				{
					SlotNumber: 10,
					Events: []lptypes.ProgramEvent{
						{
							Program:   solana.PublicKey{0x01}.String(),
							Data:      base64.StdEncoding.EncodeToString([]byte{0x10}),
							IsCPI:     true,
							BlockData: lptypes.BlockData{SlotNumber: 10},
						},
					},
				},
			},
			expectedCount: 1,
			expectedLogs:  1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			collector := mocks.NewMockLogCollector(t)
			collector.On("BackfillForAddresses", mock.Anything, mock.Anything, uint64(10), uint64(20)).
				Return(func(_ context.Context, _ []lptypes.PublicKey, _, _ uint64) (<-chan lptypes.Block, func(), error) {
					ch := make(chan lptypes.Block, len(tc.blocks))
					for _, block := range tc.blocks {
						ch <- block
					}
					close(ch)
					return ch, func() {}, nil
				}).Once()

			lggr, observed := logger.TestObserved(t, zapcore.DebugLevel)
			svc, err := logpoller.NewServiceWithLogCollector(collector, solana.PublicKey{0x01}, lggr)
			require.NoError(t, err)

			events, err := svc.FetchEvents(context.Background(), 10, 20)
			require.NoError(t, err)
			require.Len(t, events, tc.expectedCount)

			logs := observed.FilterMessage("FetchEvents: found events").All()
			require.Len(t, logs, tc.expectedLogs)
			if tc.expectedLogs > 0 {
				require.Equal(t, zapcore.InfoLevel, logs[0].Level)
			}
		})
	}
}

// TestServiceFetchEventsSkipsUndecodableEvent checks that FetchEvents skips an event it
// cannot decode and returns the other events in the range. This case does not occur in
// production: the same process encodes the data. One bad event must not fail the full fetch.
func TestServiceFetchEventsSkipsUndecodableEvent(t *testing.T) {
	t.Parallel()
	programID := solana.PublicKey{0x01}
	goodPayload := []byte{0x10, 0x20, 0x30}
	goodSignature := solana.Signature{0xCC}
	blocks := []lptypes.Block{
		{
			SlotNumber: 99,
			Events: []lptypes.ProgramEvent{
				{
					Program:   programID.String(),
					Data:      "not-base64",
					IsCPI:     true,
					BlockData: lptypes.BlockData{SlotNumber: 99, TransactionHash: solana.Signature{0xBB}},
				},
				{
					Program:   programID.String(),
					Data:      base64.StdEncoding.EncodeToString(goodPayload),
					IsCPI:     true,
					BlockData: lptypes.BlockData{SlotNumber: 99, TransactionHash: goodSignature},
				},
			},
		},
	}

	var cleanupCalled bool
	collector := mocks.NewMockLogCollector(t)
	collector.On("BackfillForAddresses", mock.Anything, mock.Anything, uint64(1), uint64(2)).
		Return(func(_ context.Context, _ []lptypes.PublicKey, _, _ uint64) (<-chan lptypes.Block, func(), error) {
			ch := make(chan lptypes.Block, 1)
			ch <- blocks[0]
			close(ch)
			return ch, func() { cleanupCalled = true }, nil
		}).Once()

	lggr, observed := logger.TestObserved(t, zapcore.DebugLevel)
	svc, err := logpoller.NewServiceWithLogCollector(collector, programID, lggr)
	require.NoError(t, err)

	events, err := svc.FetchEvents(context.Background(), 1, 2)
	require.NoError(t, err)
	require.True(t, cleanupCalled)

	require.Len(t, events, 1, "the decodable sibling event must survive")
	require.Equal(t, goodPayload, events[0].Data)
	require.Equal(t, goodSignature, events[0].TxSignature)

	logs := observed.FilterMessage("FetchEvents: skipping undecodable CPI event").All()
	require.Len(t, logs, 1)
	require.Equal(t, zapcore.DPanicLevel, logs[0].Level, "Criticalw logs at DPanic level")
}

// TestServiceFetchEventsHonorsContextCancellation checks that FetchEvents returns the
// context error when the context expires. The chainlink-solana loader does not close the
// block channel after a failed backfill, and the sorter's last flush stops at an aborted
// block. FetchEvents must not wait on a channel that can stay open forever.
func TestServiceFetchEventsHonorsContextCancellation(t *testing.T) {
	t.Parallel()
	programID := solana.PublicKey{0x01}

	var cleanupCalled bool
	collector := mocks.NewMockLogCollector(t)
	collector.On("BackfillForAddresses", mock.Anything, mock.Anything, uint64(1), uint64(2)).
		Return(func(_ context.Context, _ []lptypes.PublicKey, _, _ uint64) (<-chan lptypes.Block, func(), error) {
			// This channel receives no data and does not close.
			// The loader leaves the channel in this state after a failed backfill.
			ch := make(chan lptypes.Block)
			return ch, func() { cleanupCalled = true }, nil
		}).Once()

	svc, err := logpoller.NewServiceWithLogCollector(collector, programID, logger.Nop())
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	events, err := svc.FetchEvents(ctx, 1, 2)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Nil(t, events)
	require.True(t, cleanupCalled, "cleanup must run so the sorter and jobs are torn down")
}

func TestServiceFetchEventsReturnsBackfillError(t *testing.T) {
	t.Parallel()
	programID := solana.PublicKey{0x01}
	collector := mocks.NewMockLogCollector(t)
	collector.On("BackfillForAddresses", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil, func() {}, errors.New("boom")).Once()

	svc, err := logpoller.NewServiceWithLogCollector(collector, programID, logger.Nop())
	require.NoError(t, err)

	_, err = svc.FetchEvents(context.Background(), 1, 2)
	require.Error(t, err)
	require.Contains(t, err.Error(), "backfill failed")
}

func TestServiceFetchEventsDropsRevertedByDefault(t *testing.T) {
	t.Parallel()
	programID := solana.PublicKey{0x01}
	payload := []byte{0x10, 0x20}

	revertedErr := map[string]any{"InstructionError": []any{float64(2), map[string]any{"Custom": float64(6001)}}}

	blocks := []lptypes.Block{
		{
			SlotNumber: 50,
			Events: []lptypes.ProgramEvent{
				{
					Program:   programID.String(),
					Data:      base64.StdEncoding.EncodeToString(payload),
					IsCPI:     true,
					BlockData: lptypes.BlockData{SlotNumber: 50, Error: revertedErr},
				},
				{
					Program:   programID.String(),
					Data:      base64.StdEncoding.EncodeToString(payload),
					IsCPI:     true,
					BlockData: lptypes.BlockData{SlotNumber: 50, Error: nil},
				},
			},
		},
	}

	makeCollector := func(t *testing.T) *mocks.MockLogCollector {
		c := mocks.NewMockLogCollector(t)
		c.On("BackfillForAddresses", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(func(_ context.Context, _ []lptypes.PublicKey, _, _ uint64) (<-chan lptypes.Block, func(), error) {
				ch := make(chan lptypes.Block, 1)
				ch <- blocks[0]
				close(ch)
				return ch, func() {}, nil
			}).Once()
		return c
	}

	t.Run("default drops reverted", func(t *testing.T) {
		t.Parallel()
		svc, err := logpoller.NewServiceWithLogCollector(makeCollector(t), programID, logger.Nop())
		require.NoError(t, err)

		events, err := svc.FetchEvents(context.Background(), 50, 50)
		require.NoError(t, err)
		require.Len(t, events, 1, "reverted event must be dropped")
	})

	t.Run("WithIncludeReverted keeps reverted", func(t *testing.T) {
		t.Parallel()
		svc, err := logpoller.NewServiceWithLogCollector(makeCollector(t), programID, logger.Nop(), logpoller.WithIncludeReverted(true))
		require.NoError(t, err)

		events, err := svc.FetchEvents(context.Background(), 50, 50)
		require.NoError(t, err)
		require.Len(t, events, 2, "both events must be returned")
	})
}

func TestServiceFetchEventsRejectsInvertedRange(t *testing.T) {
	t.Parallel()
	programID := solana.PublicKey{0x01}
	collector := mocks.NewMockLogCollector(t)

	svc, err := logpoller.NewServiceWithLogCollector(collector, programID, logger.Nop())
	require.NoError(t, err)

	_, err = svc.FetchEvents(context.Background(), 20, 10)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid slot range")
}
