package config

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/BurntSushi/toml"
	"github.com/gagliardetto/solana-go"
)

// Structure to match Anchor.toml format
type Config struct {
	Programs map[string]map[string]string `toml:"programs"`
}

var AnchorToml = parseAnchorToml()

func parseAnchorToml() Config {
	var config Config
	startDir, err := os.Getwd()
	if err != nil {
		return config
	}

	dir := startDir
	var tomlPath string
	for {
		tomlPath = filepath.Join(dir, "Anchor.toml")
		if _, err := os.Stat(tomlPath); err == nil {
			break
		}

		// Move up one level
		parentDir := filepath.Dir(dir)
		if parentDir == dir {
			tomlPath = ""
			break
		}
		dir = parentDir
	}

	if tomlPath == "" {
		// Tests run from another module (e.g. integration-tests) have no Anchor.toml
		// ancestor; fall back to the contracts dir relative to this source file.
		_, b, _, _ := runtime.Caller(0)
		tomlPath = filepath.Join(filepath.Dir(b), "..", "..", "Anchor.toml")
	}

	if _, err := toml.DecodeFile(tomlPath, &config); err != nil {
		return config
	}

	return config
}

// GetProgramID retrieves the program ID dynamically
func GetProgramID(programName string) solana.PublicKey {
	programID, exists := AnchorToml.Programs["localnet"][programName]
	if !exists {
		return solana.PublicKey{}
	}

	return solana.MustPublicKeyFromBase58(programID)
}
