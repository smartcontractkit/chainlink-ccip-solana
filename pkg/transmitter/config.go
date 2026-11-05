package transmitter

import (
	"fmt"
	"time"

	"github.com/smartcontractkit/chainlink-common/pkg/config"
)

// Config holds tunable parameters for the SolanaContractTransmitter.
type Config struct {
	// TxRetentionTimeout sets how long TXM keeps terminal transaction status records.
	TxRetentionTimeout config.Duration `json:"tx_retention_timeout,omitzero" toml:"tx_retention_timeout,omitempty"`
	// Buffered groups the timings for the buffered (chunked) submission flow.
	BufferedConfig
}

// BufferedConfig tunes the buffered (chunked) submission flow in buffer.go.
type BufferedConfig struct {
	// TxStatusPollInterval is how often waitForFinalized polls txm for transaction status.
	TxStatusPollInterval config.Duration `json:"tx_status_poll_interval,omitzero" toml:"tx_status_poll_interval,omitempty"`
	// BufferedFlowTimeout bounds a single synchronous buffered submission. Chunked txs are
	// enqueued in parallel, so a healthy flow finishes well within this default of 5 min.
	// The cap mainly protects an executor worker from being parked indefinitely on a
	// stalled RPC. On timeout the flow returns an error and the executor retries the message.
	// Cleanup after a failure (wait for in-flight txs + close) is granted a fresh budget of the
	// same size, scoped to the caller's context rather than the flow's.
	BufferedFlowTimeout config.Duration `json:"buffered_flow_timeout,omitzero" toml:"buffered_flow_timeout,omitempty"`
	// BufferCleanupTimeout bounds cleanup only when the caller's context has already been
	// cancelled (shutdown path), so Close cannot block on it indefinitely. It is usually shorter
	// than the wait for in-flight txs, so a write can still land after the process stops, or the
	// close is not sent. The next attempt of that message can then fail once.
	BufferCleanupTimeout config.Duration `json:"buffer_cleanup_timeout,omitzero" toml:"buffer_cleanup_timeout,omitempty"`
}

// DefaultConfig returns production defaults for the transmitter.
func DefaultConfig() Config {
	return Config{
		TxRetentionTimeout: *config.MustNewDuration(4 * time.Hour),
		BufferedConfig:     DefaultBufferedConfig(),
	}
}

// DefaultBufferedConfig returns production defaults for the buffered submission flow.
func DefaultBufferedConfig() BufferedConfig {
	return BufferedConfig{
		BufferedFlowTimeout:  *config.MustNewDuration(5 * time.Minute),
		TxStatusPollInterval: *config.MustNewDuration(500 * time.Millisecond),
		BufferCleanupTimeout: *config.MustNewDuration(5 * time.Second),
	}
}

// WithDefaults fills any zero-valued field with its default (per-field defaulting,
// so partial TOML config still works).
func (c Config) WithDefaults() Config {
	if c.TxRetentionTimeout.Duration() == 0 {
		c.TxRetentionTimeout = DefaultConfig().TxRetentionTimeout
	}
	c.BufferedConfig = c.BufferedConfig.WithDefaults()
	return c
}

// WithDefaults fills any zero-valued field with its default.
func (c BufferedConfig) WithDefaults() BufferedConfig {
	defaults := DefaultBufferedConfig()
	if c.BufferedFlowTimeout.Duration() == 0 {
		c.BufferedFlowTimeout = defaults.BufferedFlowTimeout
	}
	if c.TxStatusPollInterval.Duration() == 0 {
		c.TxStatusPollInterval = defaults.TxStatusPollInterval
	}
	if c.BufferCleanupTimeout.Duration() == 0 {
		c.BufferCleanupTimeout = defaults.BufferCleanupTimeout
	}
	return c
}

// Validate ensures all durations are positive (call after WithDefaults).
func (c Config) Validate() error {
	if err := c.BufferedConfig.Validate(); err != nil {
		return err
	}
	if c.TxRetentionTimeout.Duration() <= 0 {
		return fmt.Errorf("tx_retention_timeout must be positive, got %s", c.TxRetentionTimeout)
	}
	if c.TxRetentionTimeout.Duration() < c.BufferedFlowTimeout.Duration() {
		return fmt.Errorf("tx_retention_timeout (%s) must be >= buffered_flow_timeout (%s)", c.TxRetentionTimeout, c.BufferedFlowTimeout)
	}
	return nil
}

// Validate ensures all durations are positive.
func (c BufferedConfig) Validate() error {
	if c.BufferedFlowTimeout.Duration() <= 0 {
		return fmt.Errorf("buffered_flow_timeout must be positive, got %s", c.BufferedFlowTimeout)
	}
	if c.TxStatusPollInterval.Duration() <= 0 {
		return fmt.Errorf("tx_status_poll_interval must be positive, got %s", c.TxStatusPollInterval)
	}
	if c.BufferCleanupTimeout.Duration() <= 0 {
		return fmt.Errorf("buffer_cleanup_timeout must be positive, got %s", c.BufferCleanupTimeout)
	}
	return nil
}
