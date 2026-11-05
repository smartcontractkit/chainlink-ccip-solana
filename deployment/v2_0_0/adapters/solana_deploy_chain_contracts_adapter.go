package adapters

import (
	"math/big"

	"github.com/gagliardetto/solana-go"

	ccvadapters "github.com/smartcontractkit/chainlink-ccip/deployment/v2_0_0/adapters"
	cldf_chain "github.com/smartcontractkit/chainlink-deployments-framework/chain"
	"github.com/smartcontractkit/chainlink-deployments-framework/deployment"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations"

	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/sequences"
)

// SolanaDeployChainContractsAdapter satisfies DeployChainContractsAdapter for the shared
// ccv DeployContractsForSelector.
type SolanaDeployChainContractsAdapter struct{}

var _ ccvadapters.DeployChainContractsAdapter = (*SolanaDeployChainContractsAdapter)(nil)

func (a *SolanaDeployChainContractsAdapter) GetDefaultDeployContractParams(_ uint64) ccvadapters.DeployContractParams {
	return ccvadapters.DeployContractParams{
		OffRamp: ccvadapters.OffRampDeployParams{
			Version: solcommon.Version2_0_0,
		},
		OnRamp: ccvadapters.OnRampDeployParams{
			Version: solcommon.Version2_0_0,
		},
		FeeQuoter: ccvadapters.FeeQuoterDeployParams{
			Version:           solcommon.Version2_0_0,
			MaxFeeJuelsPerMsg: new(big.Int).Mul(big.NewInt(2e2), big.NewInt(1e9)),
		},
		Executors: []ccvadapters.ExecutorDeployParams{
			{
				Version:       solcommon.Version2_0_0,
				MaxCCVsPerMsg: 10,
			},
		},
	}
}

func (a *SolanaDeployChainContractsAdapter) ResolveDeployAddresses(_ deployment.Environment, _ uint64) (ccvadapters.DeployChainResolvedAddresses, error) {
	return ccvadapters.DeployChainResolvedAddresses{
		DeployerContract: solana.SystemProgramID.String(),
	}, nil
}

func (a *SolanaDeployChainContractsAdapter) BuildDeployContractParams(input ccvadapters.BuildDeployContractParamsInput) (ccvadapters.DeployContractParams, error) {
	params := input.Defaults
	params.CommitteeVerifiers = input.CommitteeVerifiers
	return ccvadapters.ApplyDeployContractParamsOverrides(params, input.Overrides), nil
}

func (a *SolanaDeployChainContractsAdapter) DeployChainContracts() *operations.Sequence[ccvadapters.DeployChainContractsInput, ccvadapters.DeployChainContractsOutput, cldf_chain.BlockChains] {
	return sequences.DeployChainContracts
}
