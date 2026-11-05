package main

import (
	"os"

	"github.com/smartcontractkit/chainlink-ccv/bootstrap"
	cmd "github.com/smartcontractkit/chainlink-ccv/cmd/verifier"
	"github.com/smartcontractkit/chainlink-ccv/verifier/pkg/commit"
	"github.com/smartcontractkit/chainlink-ccv/verifier/pkg/vsecrets"
	"github.com/smartcontractkit/chainlink-common/keystore"

	_ "github.com/smartcontractkit/chainlink-ccip-solana/pkg/accessors" // solana accessor driver
)

func main() {
	if len(os.Args) >= 2 && os.Args[1] == "ccv" {
		cmd.RunCCVCLI(os.Args[1:], vsecrets.CommitteeVerifierSecretsPathEnv, vsecrets.DefaultCommitteeVerifierSecretsPath)
		return
	}

	if err := bootstrap.Run(
		"SolanaCommitteeVerifier",
		cmd.NewCommitteeVerifierServiceFactory(),
		bootstrap.WithKey(commit.DefaultECDSASigningKeyName, "signing", keystore.ECDSA_S256),
	); err != nil {
		panic("failed to run Solana committee verifier: " + err.Error())
	}
}
