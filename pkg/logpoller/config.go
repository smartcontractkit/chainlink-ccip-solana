package logpoller

import "errors"

// Config holds tunable parameters for the logpoller Service.
type Config struct {
	// SlotsBatchSize is the number of slots to process in a batch when fetching
	// blocks from the Solana RPC. Higher values use more memory but fewer
	// round-trips; lower values do the opposite.
	SlotsBatchSize int64 `toml:"slots_batch_size"`

	// IncludeReverted controls whether FetchEvents returns events from reverted
	// (failed) transactions. Not user-configurable: the factory sets it per
	// consumer (false for source reader, true for execution-attempt poller).
	IncludeReverted bool `toml:"-"`
}

// DefaultConfig returns production defaults that match chainlink-solana's
// upstream LogPollerSlotsBatchSize setting.
func DefaultConfig() Config {
	return Config{
		SlotsBatchSize: 1000,
	}
}

// Validate rejects configs with fields that must not be set via TOML.
func (c Config) Validate() error {
	if c.IncludeReverted {
		return errors.New("include_reverted must not be set in config; it is controlled by the factory per consumer")
	}
	return nil
}

// WithDefaults fills any zero-valued field with its default.
func (c Config) WithDefaults() Config {
	if c.SlotsBatchSize <= 0 {
		c.SlotsBatchSize = DefaultConfig().SlotsBatchSize
	}
	return c
}
