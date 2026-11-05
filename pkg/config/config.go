package ccip

import (
	"fmt"
	"os"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/smartcontractkit/chainlink-common/pkg/config"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	solconfig "github.com/smartcontractkit/chainlink-solana/pkg/solana/config"

	"github.com/smartcontractkit/chainlink-ccip-solana/pkg/destinationreader"
	"github.com/smartcontractkit/chainlink-ccip-solana/pkg/sourcereader"
	"github.com/smartcontractkit/chainlink-ccip-solana/pkg/transmitter"
)

const (
	SolanaConfigPathEnv      = "SOLANA_CONFIG_PATH"
	DefaultSolanaConfigPath  = "/config/solana-config.toml"
	DefaultRPCRequestTimeout = 30 * time.Second
)

// Config holds NOP-owned Solana chain config (Canton pattern). Everything for one
// chain lives under a single [chains.<selector>] table. Connection topology and
// contract addresses come from the JD job spec, not this file.
type Config struct {
	// Chains is keyed by chain selector (decimal string). One entry per Solana chain.
	Chains map[string]ChainConfig `toml:"chains"`
}

// ChainConfig is all NOP-owned config for a single Solana chain.
type ChainConfig struct {
	// ChainID overrides the chain ID (genesis hash) used by MultiNode health checks.
	// If empty, the factory derives it from the chain selector via chain-selectors.
	// Local validators have to fetch genesis hash because it's randomly generated.
	ChainID string `toml:"chain_id,omitempty"`
	// Nodes are the Solana RPC endpoints for this chain, in chainlink-solana's Node format.
	// They are fed to MultiNode, which health-checks each and fails over between them by Order.
	Nodes []*solconfig.Node `toml:"nodes"`
	// RPCRequestTimeout bounds a single RPC request made by the MultiNode clients.
	// If zero, defaults to 30s.
	RPCRequestTimeout config.Duration `toml:"rpc_request_timeout,omitempty"`
	// Reader tunes the logpoller used by source and destination readers.
	Reader sourcereader.ReaderConfig `toml:"reader,omitempty"`
	// DestinationReader tunes the execution attempt poller used by destination readers.
	DestinationReader destinationreader.Config `toml:"destination_reader,omitempty"`
	// Transmitter tunes the executor's contract transmitter (destination chains only).
	Transmitter transmitter.Config `toml:"transmitter,omitempty"`
}

// ResolveConfigPath returns the Solana NOP config path and whether it came from
// SOLANA_CONFIG_PATH (explicit operator config) vs the built-in default.
func ResolveConfigPath() (path string, fromEnv bool) {
	if p, ok := os.LookupEnv(SolanaConfigPathEnv); ok && p != "" {
		return p, true
	}
	return DefaultSolanaConfigPath, false
}

// LoadConfig loads Solana NOP config from SOLANA_CONFIG_PATH or DefaultSolanaConfigPath.
func LoadConfig(lggr logger.Logger) (*Config, error) {
	path, _ := ResolveConfigPath()
	return LoadConfigFile(lggr, path)
}

// LoadConfigFile loads Solana NOP config from the given path.
func LoadConfigFile(lggr logger.Logger, configPath string) (*Config, error) {
	var cfg Config
	if md, err := toml.DecodeFile(configPath, &cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config file %s: %w", configPath, err)
	} else if len(md.Undecoded()) > 0 {
		return nil, fmt.Errorf("unknown fields in config file %s: %v", configPath, md.Undecoded())
	}

	lggr.Infow("loaded solana config",
		"path", configPath,
		"numChains", len(cfg.Chains),
	)

	return &cfg, nil
}
