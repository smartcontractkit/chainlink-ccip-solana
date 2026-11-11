package logpoller

import (
	"context"
	"errors"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	lptypes "github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller/types"
)

// recordingSlotRecorder captures SetLatestProcessedSlot calls for assertions.
type recordingSlotRecorder struct {
	slots []int64
}

func (r *recordingSlotRecorder) SetLatestProcessedSlot(_ context.Context, slot int64) {
	r.slots = append(r.slots, slot)
}

// fakeLogCollector stubs BackfillForAddresses. internal/mocks cannot be used here
// because it imports this package (import cycle). Lifecycle methods are never invoked
// by FetchEvents and panic via the nil embedded interface if called.
type fakeLogCollector struct {
	LogCollector
	blocks <-chan lptypes.Block
	err    error
}

func (f *fakeLogCollector) BackfillForAddresses(context.Context, []lptypes.PublicKey, uint64, uint64) (<-chan lptypes.Block, func(), error) {
	return f.blocks, func() {}, f.err
}

func TestServiceFetchEventsRecordsLatestProcessedSlot(t *testing.T) {
	t.Parallel()
	programID := solana.PublicKey{0x01}

	blocks := make(chan lptypes.Block)
	close(blocks)
	collector := &fakeLogCollector{blocks: blocks}

	recorder := &recordingSlotRecorder{}
	svc, err := NewServiceWithLogCollector(collector, programID, logger.Nop(), withSlotRecorder(recorder))
	require.NoError(t, err)

	events, err := svc.FetchEvents(context.Background(), 100, 110)
	require.NoError(t, err)
	require.Empty(t, events)
	require.Equal(t, []int64{110}, recorder.slots)
}

func TestServiceFetchEventsSkipsRecordingOnBackfillError(t *testing.T) {
	t.Parallel()
	programID := solana.PublicKey{0x01}

	collector := &fakeLogCollector{err: errors.New("rpc unavailable")}

	recorder := &recordingSlotRecorder{}
	svc, err := NewServiceWithLogCollector(collector, programID, logger.Nop(), withSlotRecorder(recorder))
	require.NoError(t, err)

	_, err = svc.FetchEvents(context.Background(), 100, 110)
	require.Error(t, err)
	require.Empty(t, recorder.slots)
}
