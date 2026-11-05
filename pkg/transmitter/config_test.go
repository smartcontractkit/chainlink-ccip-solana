package transmitter

import (
	"testing"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/config"
)

func TestDefaultConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()
	require.Equal(t, 4*time.Hour, cfg.TxRetentionTimeout.Duration())
	require.Equal(t, 5*time.Minute, cfg.BufferedFlowTimeout.Duration())
	require.Equal(t, 500*time.Millisecond, cfg.TxStatusPollInterval.Duration())
	require.Equal(t, 5*time.Second, cfg.BufferCleanupTimeout.Duration())
}

func TestDefaultBufferedConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultBufferedConfig()
	require.Equal(t, 5*time.Minute, cfg.BufferedFlowTimeout.Duration())
	require.Equal(t, 500*time.Millisecond, cfg.TxStatusPollInterval.Duration())
	require.Equal(t, 5*time.Second, cfg.BufferCleanupTimeout.Duration())
}

func TestBufferedConfig_WithDefaults(t *testing.T) {
	t.Parallel()
	t.Run("zero_config_gets_all_defaults", func(t *testing.T) {
		t.Parallel()
		cfg := Config{}.WithDefaults()
		defaults := DefaultConfig()
		require.Equal(t, defaults, cfg)
	})

	t.Run("partial_config_preserves_set_fields", func(t *testing.T) {
		t.Parallel()
		customTimeout := *config.MustNewDuration(10 * time.Minute)
		cfg := Config{
			BufferedConfig: BufferedConfig{
				BufferedFlowTimeout: customTimeout,
			},
		}.WithDefaults()
		require.Equal(t, 10*time.Minute, cfg.BufferedFlowTimeout.Duration())
		require.Equal(t, 500*time.Millisecond, cfg.TxStatusPollInterval.Duration())
		require.Equal(t, 5*time.Second, cfg.BufferCleanupTimeout.Duration())
	})
}

func TestBufferedConfig_Validate(t *testing.T) {
	t.Parallel()
	t.Run("defaults_pass", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, DefaultConfig().Validate())
	})

	t.Run("zero_buffered_flow_timeout_fails", func(t *testing.T) {
		t.Parallel()
		cfg := DefaultConfig()
		cfg.BufferedFlowTimeout = config.Duration{}
		require.Error(t, cfg.Validate())
		require.Contains(t, cfg.Validate().Error(), "buffered_flow_timeout")
	})

	t.Run("zero_tx_status_poll_interval_fails", func(t *testing.T) {
		t.Parallel()
		cfg := DefaultConfig()
		cfg.TxStatusPollInterval = config.Duration{}
		require.Error(t, cfg.Validate())
		require.Contains(t, cfg.Validate().Error(), "tx_status_poll_interval")
	})

	t.Run("zero_buffer_cleanup_timeout_fails", func(t *testing.T) {
		t.Parallel()
		cfg := DefaultConfig()
		cfg.BufferCleanupTimeout = config.Duration{}
		require.Error(t, cfg.Validate())
		require.Contains(t, cfg.Validate().Error(), "buffer_cleanup_timeout")
	})
}

func TestConfig_TOMLDecoding(t *testing.T) {
	t.Parallel()
	input := `
buffered_flow_timeout = "10m"
tx_status_poll_interval = "1s"
buffer_cleanup_timeout = "15s"
tx_retention_timeout = "30m"
`
	var cfg Config
	_, err := toml.Decode(input, &cfg)
	require.NoError(t, err)
	require.Equal(t, 10*time.Minute, cfg.BufferedFlowTimeout.Duration())
	require.Equal(t, time.Second, cfg.TxStatusPollInterval.Duration())
	require.Equal(t, 15*time.Second, cfg.BufferCleanupTimeout.Duration())
	require.Equal(t, 30*time.Minute, cfg.WithDefaults().TxRetentionTimeout.Duration())
}

func TestConfig_TxRetentionTimeout(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name      string
		retention time.Duration
		want      time.Duration
		wantErr   bool
	}{
		{name: "omitted", want: 4 * time.Hour},
		{name: "override", retention: 30 * time.Minute, want: 30 * time.Minute},
		{name: "flow_boundary", retention: 5 * time.Minute, want: 5 * time.Minute},
		{name: "shorter_than_flow", retention: time.Minute, wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := Config{TxRetentionTimeout: *config.MustNewDuration(tt.retention)}.WithDefaults()
			if tt.wantErr {
				require.ErrorContains(t, cfg.Validate(), "tx_retention_timeout")
				return
			}
			require.NoError(t, cfg.Validate())
			require.Equal(t, tt.want, cfg.TxRetentionTimeout.Duration())
		})
	}
}

func TestConfig_TxRetentionTimeoutNegative(t *testing.T) {
	t.Parallel()
	var cfg Config
	_, err := toml.Decode(`tx_retention_timeout = "-1m"`, &cfg)
	require.ErrorContains(t, err, "negative")
}
