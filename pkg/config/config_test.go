package ccip_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"

	"github.com/smartcontractkit/chainlink-ccip-solana/pkg/chainselector"
	ccip "github.com/smartcontractkit/chainlink-ccip-solana/pkg/config"
	"github.com/smartcontractkit/chainlink-ccip-solana/pkg/destinationreader"
	"github.com/smartcontractkit/chainlink-ccip-solana/pkg/transmitter"
)

func TestLoadConfigFile_Chains(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	require.NoError(t, os.WriteFile(path, []byte(`
[[chains."16423721717087811551".nodes]]
Name = "primary"
URL = "https://rpcs.cldev.sh/solana/devnet"
IsLoadBalancedRPC = true
`), 0o600))

	cfg, err := ccip.LoadConfigFile(logger.Nop(), path)
	require.NoError(t, err)

	sel, err := chainselector.Parse("16423721717087811551")
	require.NoError(t, err)
	require.NotZero(t, sel)

	chain := cfg.Chains["16423721717087811551"]
	require.Len(t, chain.Nodes, 1)
	require.Equal(t, "https://rpcs.cldev.sh/solana/devnet", chain.Nodes[0].URL.String())
	require.True(t, *chain.Nodes[0].IsLoadBalancedRPC)
}

func TestLoadConfigFile_ChainReaderAndTransmitter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	selector := "16423721717087811551"
	require.NoError(t, os.WriteFile(path, []byte(`
[[chains."16423721717087811551".nodes]]
name = "primary"
url = "https://rpcs.cldev.sh/solana/devnet"

[chains."16423721717087811551".reader]
slots_batch_size = 250

[chains."16423721717087811551".transmitter]
buffered_flow_timeout = "10m"
tx_retention_timeout = "30m"
`), 0o600))

	cfg, err := ccip.LoadConfigFile(logger.Nop(), path)
	require.NoError(t, err)

	chain := cfg.Chains[selector]
	require.Equal(t, int64(250), chain.Reader.WithDefaults().SlotsBatchSize)

	tx := chain.Transmitter.WithDefaults()
	require.Equal(t, 10*time.Minute, tx.BufferedFlowTimeout.Duration())
	require.Equal(t, 30*time.Minute, tx.TxRetentionTimeout.Duration())
	require.NoError(t, tx.Validate())
	require.Equal(t, 500*time.Millisecond, tx.TxStatusPollInterval.Duration())

	missing := ccip.ChainConfig{}
	require.Equal(t, int64(1000), missing.Reader.WithDefaults().SlotsBatchSize)
	require.Equal(t, transmitter.DefaultConfig(), missing.Transmitter.WithDefaults())
}

func TestLoadConfigFile_DestinationReader(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	selector := "16423721717087811551"
	require.NoError(t, os.WriteFile(path, []byte(`
[[chains."16423721717087811551".nodes]]
name = "primary"
url = "https://rpcs.cldev.sh/solana/devnet"

[chains."16423721717087811551".destination_reader]
poll_interval = "2s"
lookback_window = "10m"
attempt_cache_ttl = "30m"
max_events_per_poll = 50
startup_backfill_timeout = "20s"
poll_fetch_timeout = "1m"
`), 0o600))

	cfg, err := ccip.LoadConfigFile(logger.Nop(), path)
	require.NoError(t, err)

	dr := cfg.Chains[selector].DestinationReader.WithDefaults()
	require.Equal(t, 2*time.Second, dr.PollInterval.Duration())
	require.Equal(t, 10*time.Minute, dr.LookbackWindow.Duration())
	require.Equal(t, 30*time.Minute, dr.AttemptCacheTTL.Duration())
	require.Equal(t, 50, dr.MaxEventsPerPoll)
	require.Equal(t, 20*time.Second, dr.StartupBackfillTimeout.Duration())
	require.Equal(t, time.Minute, dr.PollFetchTimeout.Duration())
	require.NoError(t, dr.Validate())

	require.Equal(t, destinationreader.DefaultConfig(), ccip.ChainConfig{}.DestinationReader.WithDefaults())
}

func TestResolveConfigPath_Default(t *testing.T) {
	t.Parallel()
	path, fromEnv := ccip.ResolveConfigPath()
	require.Equal(t, ccip.DefaultSolanaConfigPath, path)
	require.False(t, fromEnv)
}

func TestResolveConfigPath_FromEnv(t *testing.T) {
	t.Setenv(ccip.SolanaConfigPathEnv, "/custom/solana.toml")
	path, fromEnv := ccip.ResolveConfigPath()
	require.Equal(t, "/custom/solana.toml", path)
	require.True(t, fromEnv)
}

func TestLoadConfigFile_RejectsLegacyTopLevelMaps(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	require.NoError(t, os.WriteFile(path, []byte(`
[blockchain_infos."16423721717087811551"]
rpc_urls = ["https://example.com"]
`), 0o600))

	_, err := ccip.LoadConfigFile(logger.Nop(), path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown fields")
}

func TestLoadConfig_ExplicitPathMissingFails(t *testing.T) {
	t.Setenv(ccip.SolanaConfigPathEnv, filepath.Join(t.TempDir(), "missing.toml"))
	_, err := ccip.LoadConfig(logger.Nop())
	require.Error(t, err)
}
