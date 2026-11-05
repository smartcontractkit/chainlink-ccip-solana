package testutils

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-testing-framework/framework/components/blockchain"
	"github.com/smartcontractkit/freeport"
)

// DefaultValidatorFlags are the base flags for all Solana test validators.
// These match mainnet conditions and should be used consistently across
// contract tests, integration tests, and devenv.
var DefaultValidatorFlags = []string{
	"--ticks-per-slot", "8", // faster slots (mainnet: 64)
	"--limit-ledger-size", "200000000", // this prevents the validator from dropping historical blocks too quickly
}

// NewLocalValidator starts a local Solana validator via CTF Docker.
// It injects DefaultValidatorFlags and fixes the CTF ChainID propagation bug.
// Shared by contract tests (DeployAllPrograms) and devenv (DeployLocalNetwork).
func NewLocalValidator(ctx context.Context, input *blockchain.Input) (*blockchain.Output, error) {
	input.Type = "solana"
	input.DockerCmdParamsOverrides = append(DefaultValidatorFlags, input.DockerCmdParamsOverrides...)

	// Assign a random free port when not explicitly set to avoid collisions
	// when multiple tests run in parallel (CTF defaults to a fixed port 8999).
	// Reserve 2 ports: CTF binds both the RPC port (N) and WebSocket port (N+1).
	if input.Port == "" {
		ports, err := freeport.Take(2)
		if err != nil {
			return nil, fmt.Errorf("allocate free port: %w", err)
		}
		input.Port = strconv.Itoa(ports[0])
	}

	out, err := blockchain.NewBlockchainNetwork(input)
	if err != nil {
		return nil, fmt.Errorf("failed to create Solana blockchain network: %w", err)
	}
	// CTF's newSolana does not propagate ChainID to the Output, fix it here.
	if out.ChainID == "" {
		out.ChainID = input.ChainID
	}
	return out, nil
}

// SetupLocalSolNode starts a local Solana validator via CTF Docker and returns the RPC URL.
// No programs are loaded; use DeployAllPrograms when Anchor programs are needed.
func SetupLocalSolNode(t *testing.T) string {
	t.Helper()
	ctx := t.Context()

	net, err := NewLocalValidator(ctx, &blockchain.Input{
		ContractsDir: t.TempDir(),
		PublicKey:    blockchain.DefaultSolanaPublicKey,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		if net.Container != nil {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := net.Container.Terminate(cleanupCtx); err != nil {
				t.Logf("warning: failed to terminate container: %v", err)
			}
		}
	})

	return net.Nodes[0].ExternalHTTPUrl
}
