package adapters

import (
	"fmt"

	"github.com/Masterminds/semver/v3"

	ccipadapters "github.com/smartcontractkit/chainlink-ccip/deployment/v2_0_0/adapters"
	ccvadapters "github.com/smartcontractkit/chainlink-ccv/deployment/adapters"
	cldf_chain "github.com/smartcontractkit/chainlink-deployments-framework/chain"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations"
)

// familyExtraDeployOverrides is the ProtocolContractsDeployInput.FamilyExtras key
// carrying an optional *ccipadapters.DeployContractParamsOverrides. When absent the
// Solana defaults are used unchanged.
const familyExtraDeployOverrides = "deployContractParamsOverrides"

// SolanaProtocolContractsDeployAdapter implements CCV ProtocolContractsDeployAdapter
// (topology-free deploy path, chainlink-ccv #1127) for Solana by delegating to
// SolanaDeployChainContractsAdapter and mapping the sparse CCV input/output shapes.
type SolanaProtocolContractsDeployAdapter struct{}

var _ ccvadapters.ProtocolContractsDeployAdapter = (*SolanaProtocolContractsDeployAdapter)(nil)

func (a *SolanaProtocolContractsDeployAdapter) DeployProtocolContracts() *operations.Sequence[ccvadapters.ProtocolContractsDeployInput, ccvadapters.ProtocolContractsDeployOutput, cldf_chain.BlockChains] {
	return operations.NewSequence(
		"solana-deploy-protocol-contracts",
		semver.MustParse("2.0.0"),
		"Deploys CCIP 2.0 protocol contracts on a Solana chain (topology-free CCV path)",
		func(b operations.Bundle, chains cldf_chain.BlockChains, input ccvadapters.ProtocolContractsDeployInput) (ccvadapters.ProtocolContractsDeployOutput, error) {
			deployAdapter := &SolanaDeployChainContractsAdapter{}

			// Start from the Solana defaults; let the CCV-supplied executor list override
			// the default single-executor params when provided. Merge rather than replace
			// so Solana-specific fields (MaxCCVsPerMsg, DynamicConfig) are preserved.
			defaults := deployAdapter.GetDefaultDeployContractParams(input.ChainSelector)
			if execs := mapExecutorDeployParams(defaults.Executors, input.Executors); len(execs) > 0 {
				defaults.Executors = execs
			}

			overrides, err := deployContractParamsOverridesFromExtras(input.FamilyExtras)
			if err != nil {
				return ccvadapters.ProtocolContractsDeployOutput{}, err
			}

			params, err := deployAdapter.BuildDeployContractParams(ccipadapters.BuildDeployContractParamsInput{
				Defaults:  defaults,
				Overrides: overrides,
			})
			if err != nil {
				return ccvadapters.ProtocolContractsDeployOutput{}, fmt.Errorf("build deploy contract params: %w", err)
			}

			report, err := operations.ExecuteSequence(
				b,
				deployAdapter.DeployChainContracts(),
				chains,
				ccipadapters.DeployChainContractsInput{
					ChainSelector:     input.ChainSelector,
					DeployerContract:  input.DeployerContract,
					DeployTestRouter:  input.DeployTestRouter,
					ExistingAddresses: input.ExistingAddresses,
					ContractParams:    params,
					DeployerKeyOwned:  input.DeployerKeyOwned,
				},
			)
			if err != nil {
				return ccvadapters.ProtocolContractsDeployOutput{}, fmt.Errorf("execute Solana DeployChainContracts: %w", err)
			}

			return ccvadapters.ProtocolContractsDeployOutput{
				Addresses:               report.Output.Addresses,
				BatchOps:                report.Output.BatchOps,
				RefsToTransferOwnership: report.Output.RefsToTransferOwnership,
			}, nil
		},
	)
}

// mapExecutorDeployParams merges qualifier+version from the CCV executor list into the CCIP
// defaults, preserving Solana-specific fields (MaxCCVsPerMsg, DynamicConfig). Unrecognized
// qualifiers fall back to the first default entry.
func mapExecutorDeployParams(defaults []ccipadapters.ExecutorDeployParams, in []ccvadapters.ExecutorDeployParams) []ccipadapters.ExecutorDeployParams {
	if len(in) == 0 {
		return nil
	}
	byQualifier := make(map[string]ccipadapters.ExecutorDeployParams, len(defaults))
	for _, d := range defaults {
		byQualifier[d.Qualifier] = d
	}
	var fallback ccipadapters.ExecutorDeployParams
	if len(defaults) > 0 {
		fallback = defaults[0]
	}
	out := make([]ccipadapters.ExecutorDeployParams, 0, len(in))
	for _, e := range in {
		base, ok := byQualifier[e.Qualifier]
		if !ok {
			base = fallback
		}
		base.Version = e.Version
		base.Qualifier = e.Qualifier
		out = append(out, base)
	}
	return out
}

func deployContractParamsOverridesFromExtras(extras map[string]any) (*ccipadapters.DeployContractParamsOverrides, error) {
	raw, ok := extras[familyExtraDeployOverrides]
	if !ok {
		return nil, nil
	}
	ov, ok := raw.(*ccipadapters.DeployContractParamsOverrides)
	if !ok {
		return nil, fmt.Errorf("FamilyExtras[%q] must be *adapters.DeployContractParamsOverrides, got %T", familyExtraDeployOverrides, raw)
	}
	return ov, nil
}
