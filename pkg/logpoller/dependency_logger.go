package logpoller

import (
	"fmt"

	"go.uber.org/zap"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
)

// chainSolanaLogPollerLogger demotes known high-frequency lifecycle and
// speculative CPI parsing messages emitted by chainlink-solana's logpoller.
// Unexpected warnings and all error-or-higher logs retain their original level.
type chainSolanaLogPollerLogger struct {
	logger.Logger
}

func newChainSolanaLogPollerLogger(lggr logger.Logger) logger.Logger {
	return chainSolanaLogPollerLogger{Logger: lggr}
}

// Preserve optional logger scoping while retaining the demotion adapter. The
// chainlink-common logger helpers discover these methods dynamically.
func (l chainSolanaLogPollerLogger) Named(name string) logger.Logger {
	return chainSolanaLogPollerLogger{Logger: logger.Named(l.Logger, name)}
}

func (l chainSolanaLogPollerLogger) With(keysAndValues ...any) logger.Logger {
	return chainSolanaLogPollerLogger{Logger: logger.With(l.Logger, keysAndValues...)}
}

func (l chainSolanaLogPollerLogger) WithOptions(options ...zap.Option) logger.Logger {
	return chainSolanaLogPollerLogger{Logger: logger.WithOptions(l.Logger, options...)}
}

func (l chainSolanaLogPollerLogger) Helper(skip int) logger.Logger {
	return chainSolanaLogPollerLogger{Logger: logger.Helper(l.Logger, skip)}
}

func (l chainSolanaLogPollerLogger) Info(args ...any) {
	if shouldDemoteChainSolanaInfo(fmt.Sprint(args...)) {
		l.Debug(args...)
		return
	}
	l.Logger.Info(args...)
}

func (l chainSolanaLogPollerLogger) Infow(msg string, keysAndValues ...any) {
	if shouldDemoteChainSolanaInfo(msg) {
		l.Debugw(msg, keysAndValues...)
		return
	}
	l.Logger.Infow(msg, keysAndValues...)
}

func (l chainSolanaLogPollerLogger) Warnw(msg string, keysAndValues ...any) {
	if shouldDemoteChainSolanaWarning(msg) {
		l.Debugw(msg, keysAndValues...)
		return
	}
	l.Logger.Warnw(msg, keysAndValues...)
}

func shouldDemoteChainSolanaInfo(msg string) bool {
	switch msg {
	case "Starting", "Started", "Closing", "Closed",
		"Finished fetching all blocks.",
		"Found CPI event":
		return true
	default:
		return false
	}
}

func shouldDemoteChainSolanaWarning(msg string) bool {
	switch msg {
	case "anchor CPI event data shorter than method discriminator",
		"data shorter than cpiEventDataOffset",
		"cpi event vec length is zero",
		"cpi event vec length exceeds remaining bytes",
		"cpi event vec length does not match remaining bytes":
		return true
	default:
		return false
	}
}
