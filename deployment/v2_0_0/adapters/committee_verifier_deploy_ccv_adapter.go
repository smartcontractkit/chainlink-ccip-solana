package adapters

import (
	"fmt"

	"github.com/Masterminds/semver/v3"
	"github.com/gagliardetto/solana-go"

	common_utils "github.com/smartcontractkit/chainlink-ccip/deployment/utils"
	ccvadapters "github.com/smartcontractkit/chainlink-ccv/deployment/adapters"
	cldf_chain "github.com/smartcontractkit/chainlink-deployments-framework/chain"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations"

	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
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

			out := ccvadapters.CommitteeVerifierDeployOutput{
				Addresses: report.Output.Addresses,
				BatchOps:  report.Output.BatchOps,
			}
			if !input.DeployerKeyOwned {
				chain := chains.SolanaChains()[input.ChainSelector]
				out.RefsToTransferOwnership, err = verifierRefsForHandover(b, chain, input, report.Output.Addresses)
				if err != nil {
					return ccvadapters.CommitteeVerifierDeployOutput{}, err
				}
			}
			return out, nil
		},
	)
}

// verifierRefsForHandover returns the committee verifier refs the CLLCCIP timelock
// signer does not own yet, so a re-run does not propose a finished handover again.
func verifierRefsForHandover(
	b operations.Bundle,
	chain cldf_solana.Chain,
	input ccvadapters.CommitteeVerifierDeployInput,
	deployed []datastore.AddressRef,
) ([]datastore.AddressRef, error) {
	timelock, found, err := solcommon.FindTimelockSignerPDA(input.ExistingAddresses, input.ChainSelector, common_utils.CLLQualifier)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve timelock for ownership handover: %w", err)
	}
	if !found {
		return nil, fmt.Errorf(
			"timelock (qualifier %q) not found in existing addresses for chain %d: deploy MCMS first, or set DeployerKeyOwned to keep the deployer as owner",
			common_utils.CLLQualifier, input.ChainSelector)
	}
	var refs []datastore.AddressRef
	for _, ref := range deployed {
		if ref.Type != datastore.ContractType(cvops.ContractType) {
			continue
		}
		program, err := solana.PublicKeyFromBase58(ref.Address)
		if err != nil {
			return nil, fmt.Errorf("parse %s address %q: %w", ref.Type, ref.Address, err)
		}
		owner, err := cvops.GetAuthority(b.GetContext(), chain, program)
		if err != nil {
			return nil, err
		}
		if owner != timelock {
			refs = append(refs, ref)
		}
	}
	return refs, nil
}
