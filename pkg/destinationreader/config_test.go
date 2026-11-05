package destinationreader

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/config"
)

func TestDefaultConfig(t *testing.T) {
	t.Parallel()

	want := Config{
		PollInterval:           *config.MustNewDuration(5 * time.Second),
		LookbackWindow:         *config.MustNewDuration(15 * time.Minute),
		AttemptCacheTTL:        *config.MustNewDuration(15 * time.Minute),
		MaxEventsPerPoll:       200,
		StartupBackfillTimeout: *config.MustNewDuration(30 * time.Second),
		PollFetchTimeout:       *config.MustNewDuration(2 * time.Minute),
	}
	got := DefaultConfig()
	require.Equal(t, want, got)
	require.NoError(t, got.Validate())
	require.Equal(t, want, (Config{}).WithDefaults())
}

func TestConfigWithDefaults(t *testing.T) {
	t.Parallel()

	custom := Config{PollInterval: *config.MustNewDuration(time.Second)}
	got := custom.WithDefaults()
	require.Equal(t, time.Second, got.PollInterval.Duration())
	require.Equal(t, DefaultConfig().LookbackWindow, got.LookbackWindow)
	require.Equal(t, DefaultConfig().AttemptCacheTTL, got.AttemptCacheTTL)
	require.Equal(t, DefaultConfig().MaxEventsPerPoll, got.MaxEventsPerPoll)
	require.Equal(t, DefaultConfig().StartupBackfillTimeout, got.StartupBackfillTimeout)
	require.Equal(t, DefaultConfig().PollFetchTimeout, got.PollFetchTimeout)

	longLookback := Config{LookbackWindow: *config.MustNewDuration(time.Hour)}
	require.Equal(t, time.Hour, longLookback.WithDefaults().AttemptCacheTTL.Duration())
}

func TestConfigWithDefaultsRejectsNegativeMaxEvents(t *testing.T) {
	t.Parallel()

	cfg := (Config{MaxEventsPerPoll: -1}).WithDefaults()
	require.Equal(t, -1, cfg.MaxEventsPerPoll)
	require.ErrorContains(t, cfg.Validate(), "max_events_per_poll")
}

func TestConfigValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr string
	}{
		{name: "valid", mutate: func(*Config) {}},
		{name: "zero_poll_interval", mutate: func(c *Config) { c.PollInterval = config.Duration{} }, wantErr: "poll_interval"},
		{name: "short_lookback", mutate: func(c *Config) { c.LookbackWindow = *config.MustNewDuration(100 * time.Millisecond) }, wantErr: "lookback_window"},
		{name: "zero_attempt_cache_ttl", mutate: func(c *Config) { c.AttemptCacheTTL = config.Duration{} }, wantErr: "attempt_cache_ttl"},
		{name: "attempt_cache_ttl_below_lookback", mutate: func(c *Config) {
			c.AttemptCacheTTL = *config.MustNewDuration(time.Minute)
		}, wantErr: "attempt_cache_ttl"},
		{name: "zero_max_events", mutate: func(c *Config) { c.MaxEventsPerPoll = 0 }, wantErr: "max_events_per_poll"},
		{name: "zero_startup_timeout", mutate: func(c *Config) { c.StartupBackfillTimeout = config.Duration{} }, wantErr: "startup_backfill_timeout"},
		{name: "zero_poll_fetch_timeout", mutate: func(c *Config) { c.PollFetchTimeout = config.Duration{} }, wantErr: "poll_fetch_timeout"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := DefaultConfig()
			tc.mutate(&cfg)
			err := cfg.Validate()
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}
