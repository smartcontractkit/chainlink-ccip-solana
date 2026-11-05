package main

import (
	"fmt"
	"os"

	"github.com/smartcontractkit/chainlink-ccv/bootstrap"
	cmdexecutor "github.com/smartcontractkit/chainlink-ccv/cmd/executor"
	"github.com/smartcontractkit/chainlink-ccv/verifier/pkg/commit"
	"github.com/smartcontractkit/chainlink-common/keystore"

	_ "github.com/smartcontractkit/chainlink-ccip-solana/pkg/accessors" // solana accessor driver
	"github.com/smartcontractkit/chainlink-ccip-solana/pkg/transmitter"
)

func main() {
	err := bootstrap.Run(
		"SolanaExecutor",
		cmdexecutor.NewFactory(),
		bootstrap.WithKey(commit.DefaultECDSASigningKeyName, "signing", keystore.ECDSA_S256),
		bootstrap.WithKey(transmitter.DefaultKeyName, "transmitting", keystore.Ed25519),
	)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Failed to run Solana executor: %v\n", err)
		os.Exit(1)
	}
}
