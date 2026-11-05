package logpoller

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
)

func TestChainSolanaLogPollerLoggerDemotesKnownNoise(t *testing.T) {
	t.Parallel()

	lggr, observed := logger.TestObserved(t, zapcore.DebugLevel)
	wrapped := newChainSolanaLogPollerLogger(lggr)

	wrapped.Info("Starting")
	wrapped.Info("Started")
	wrapped.Info("Closing")
	wrapped.Info("Closed")
	wrapped.Infow("Finished fetching all blocks.", "from", 10, "to", 20)
	wrapped.Infow("Found CPI event", "sourceProgram", "source", "destProgram", "dest")
	wrapped.Warnw("anchor CPI event data shorter than method discriminator", "dataLen", 8)
	wrapped.Warnw("data shorter than cpiEventDataOffset", "dataLen", 8)
	wrapped.Warnw("cpi event vec length is zero", "declaredLen", 0)
	wrapped.Warnw("cpi event vec length exceeds remaining bytes", "declaredLen", 100)
	wrapped.Warnw("cpi event vec length does not match remaining bytes", "declaredLen", 10)

	require.Len(t, observed.FilterLevelExact(zapcore.DebugLevel).All(), 11)
	require.Empty(t, observed.FilterLevelExact(zapcore.InfoLevel).All())
	require.Empty(t, observed.FilterLevelExact(zapcore.WarnLevel).All())
}

func TestChainSolanaLogPollerLoggerPreservesOtherMessages(t *testing.T) {
	t.Parallel()

	lggr, observed := logger.TestObserved(t, zapcore.DebugLevel)
	wrapped := newChainSolanaLogPollerLogger(lggr)

	wrapped.Infow("last processed slot is older than lookback window", "lastProcessed", 10)
	wrapped.Warnw("Encountered truncated logs", "program", "program")

	infoLogs := observed.FilterLevelExact(zapcore.InfoLevel).All()
	require.Len(t, infoLogs, 1)
	require.Equal(t, "last processed slot is older than lookback window", infoLogs[0].Message)

	warnLogs := observed.FilterLevelExact(zapcore.WarnLevel).All()
	require.Len(t, warnLogs, 1)
	require.Equal(t, "Encountered truncated logs", warnLogs[0].Message)
}
