package adapters

import (
	"fmt"

	"github.com/Masterminds/semver/v3"

	ccvadapters "github.com/smartcontractkit/chainlink-ccv/deployment/adapters"
	cldf_chain "github.com/smartcontractkit/chainlink-deployments-framework/chain"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations"

	cvops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/committeeverifier"
	solseq "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/sequences"
)

// SolanaCommitteeVerifierDeployAdapter implements the consolidated CCV
// CommitteeVerifierDeployAdapter for Solana by delegating to sequences.DeployCommitteeVerifier
// Signer sets are owned by apply_verifier_config, not here.
type SolanaCommitteeVerifierDeployAdapter struct{}

var _ ccvadapters.CommitteeVerifierDeployAdapter = (*SolanaCommitteeVerifierDeployAdapter)(nil)

func (a *SolanaCommitteeVerifierDeployAdapter) DeployCommitteeVerifier() *operations.Sequence[ccvadapters.CommitteeVerifierDeployInput, ccvadapters.CommitteeVerifierDeployOutput, cldf_chain.BlockChains] {
	return operations.NewSequence(
		"solana-deploy-committee-verifier",
		semver.MustParse("2.0.0"),
		"Deploys and configures a Solana CommitteeVerifier (topology-free CCV path)",
		func(b operations.Bundle, chains cldf_chain.BlockChains, input ccvadapters.CommitteeVerifierDeployInput) (ccvadapters.CommitteeVerifierDeployOutput, error) {
			if input.Params.Qualifier != cvops.DeployQualifier {
				return ccvadapters.CommitteeVerifierDeployOutput{}, fmt.Errorf("solana supports only the committee qualifier %q, got %q", cvops.DeployQualifier, input.Params.Qualifier)
			}
			report, err := operations.ExecuteSequence(b, solseq.DeployCommitteeVerifier, chains, solseq.DeployCommitteeVerifierInput{
				ChainSelector:     input.ChainSelector,
				ExistingAddresses: input.ExistingAddresses,
				FeeAggregator:     input.Params.FeeAggregator,
				AllowlistAdmin:    input.Params.AllowlistAdmin,
				StorageLocations:  input.Params.StorageLocations,
			})
			if err != nil {
				return ccvadapters.CommitteeVerifierDeployOutput{}, fmt.Errorf("execute Solana DeployCommitteeVerifier: %w", err)
			}

			return ccvadapters.CommitteeVerifierDeployOutput{
				Addresses: report.Output.Addresses,
				BatchOps:  report.Output.BatchOps,
			}, nil
		},
	)
}
