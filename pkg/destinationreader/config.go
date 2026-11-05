package destinationreader

import (
	"fmt"
	"time"

	"github.com/smartcontractkit/chainlink-common/pkg/config"
)

const (
	defaultPollInterval           = 5 * time.Second
	defaultLookbackWindow         = 15 * time.Minute
	defaultMaxEventsPerPoll       = 200
	defaultStartupBackfillTimeout = 30 * time.Second
	defaultPollFetchTimeout       = 2 * time.Minute
)

// Config controls the execution attempt poller.
type Config struct {
	// PollInterval is the time between polls.
	PollInterval config.Duration `json:"poll_interval,omitzero" toml:"poll_interval,omitempty"`
	// LookbackWindow is the startup backfill horizon.
	LookbackWindow config.Duration `json:"lookback_window,omitzero" toml:"lookback_window,omitempty"`
	// AttemptCacheTTL is how long a cached execution attempt stays valid.
	// It defaults to LookbackWindow and is never less.
	// The accessors factory raises it to the JD MaxRetryDuration when that is longer.
	AttemptCacheTTL config.Duration `json:"attempt_cache_ttl,omitzero" toml:"attempt_cache_ttl,omitempty"`
	// MaxEventsPerPoll bounds the number of events ingested in one poll.
	// A slot with more events than the bound is processed whole.
	MaxEventsPerPoll int `json:"max_events_per_poll,omitzero" toml:"max_events_per_poll,omitempty"`
	// StartupBackfillTimeout bounds GetSlot and FetchEvents during startup.
	StartupBackfillTimeout config.Duration `json:"startup_backfill_timeout,omitzero" toml:"startup_backfill_timeout,omitempty"`
	// PollFetchTimeout bounds GetSlot and FetchEvents in one poll.
	PollFetchTimeout config.Duration `json:"poll_fetch_timeout,omitzero" toml:"poll_fetch_timeout,omitempty"`
}

// DefaultConfig returns the default execution attempt poller config.
func DefaultConfig() Config {
	return Config{
		PollInterval:           *config.MustNewDuration(defaultPollInterval),
		LookbackWindow:         *config.MustNewDuration(defaultLookbackWindow),
		AttemptCacheTTL:        *config.MustNewDuration(defaultLookbackWindow),
		MaxEventsPerPoll:       defaultMaxEventsPerPoll,
		StartupBackfillTimeout: *config.MustNewDuration(defaultStartupBackfillTimeout),
		PollFetchTimeout:       *config.MustNewDuration(defaultPollFetchTimeout),
	}
}

// WithDefaults fills unset fields with default values.
func (c Config) WithDefaults() Config {
	defaults := DefaultConfig()
	if c.PollInterval.Duration() <= 0 {
		c.PollInterval = defaults.PollInterval
	}
	if c.LookbackWindow.Duration() <= 0 {
		c.LookbackWindow = defaults.LookbackWindow
	}
	if c.AttemptCacheTTL.Duration() < c.LookbackWindow.Duration() {
		c.AttemptCacheTTL = c.LookbackWindow
	}
	if c.MaxEventsPerPoll == 0 {
		c.MaxEventsPerPoll = defaults.MaxEventsPerPoll
	}
	if c.StartupBackfillTimeout.Duration() <= 0 {
		c.StartupBackfillTimeout = defaults.StartupBackfillTimeout
	}
	if c.PollFetchTimeout.Duration() <= 0 {
		c.PollFetchTimeout = defaults.PollFetchTimeout
	}
	return c
}

// Validate checks the execution attempt poller config.
func (c Config) Validate() error {
	if c.PollInterval.Duration() <= 0 {
		return fmt.Errorf("poll_interval must be positive, got %s", c.PollInterval)
	}
	if c.LookbackWindow.Duration() < minSolanaSlotTime {
		return fmt.Errorf("lookback_window must be at least %s, got %s", minSolanaSlotTime, c.LookbackWindow)
	}
	if c.AttemptCacheTTL.Duration() <= 0 {
		return fmt.Errorf("attempt_cache_ttl must be positive, got %s", c.AttemptCacheTTL)
	}
	if c.AttemptCacheTTL.Duration() < c.LookbackWindow.Duration() {
		return fmt.Errorf("attempt_cache_ttl must not be below lookback_window, got %s", c.AttemptCacheTTL)
	}
	if c.MaxEventsPerPoll <= 0 {
		return fmt.Errorf("max_events_per_poll must be positive, got %d", c.MaxEventsPerPoll)
	}
	if c.StartupBackfillTimeout.Duration() <= 0 {
		return fmt.Errorf("startup_backfill_timeout must be positive, got %s", c.StartupBackfillTimeout)
	}
	if c.PollFetchTimeout.Duration() <= 0 {
		return fmt.Errorf("poll_fetch_timeout must be positive, got %s", c.PollFetchTimeout)
	}
	return nil
}
