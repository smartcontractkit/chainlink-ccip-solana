package logpoller

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	t.Parallel()
	require.Equal(t, Config{SlotsBatchSize: 1000}, DefaultConfig())
}

func TestConfig_WithDefaults(t *testing.T) {
	t.Parallel()

	t.Run("applies defaults to empty config", func(t *testing.T) {
		t.Parallel()
		cfg := Config{}.WithDefaults()
		require.Equal(t, DefaultConfig(), cfg)
	})

	t.Run("applies default when SlotsBatchSize is zero", func(t *testing.T) {
		t.Parallel()
		cfg := Config{SlotsBatchSize: 0}.WithDefaults()
		require.Equal(t, int64(1000), cfg.SlotsBatchSize)
	})

	t.Run("applies default when SlotsBatchSize is negative", func(t *testing.T) {
		t.Parallel()
		cfg := Config{SlotsBatchSize: -1}.WithDefaults()
		require.Equal(t, int64(1000), cfg.SlotsBatchSize)
	})

	t.Run("preserves custom SlotsBatchSize", func(t *testing.T) {
		t.Parallel()
		cfg := Config{SlotsBatchSize: 250}.WithDefaults()
		require.Equal(t, int64(250), cfg.SlotsBatchSize)
	})

	t.Run("all fields set - nothing should change", func(t *testing.T) {
		t.Parallel()
		original := Config{SlotsBatchSize: 500}
		require.Equal(t, original, original.WithDefaults())
	})
}

func TestConfig_Validate(t *testing.T) {
	t.Parallel()

	t.Run("accepts default config", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, DefaultConfig().Validate())
	})

	t.Run("accepts IncludeReverted false", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, Config{SlotsBatchSize: 100, IncludeReverted: false}.Validate())
	})

	t.Run("rejects IncludeReverted true", func(t *testing.T) {
		t.Parallel()
		err := Config{SlotsBatchSize: 100, IncludeReverted: true}.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "include_reverted must not be set in config")
	})
}
