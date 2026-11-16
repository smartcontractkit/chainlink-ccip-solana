package changesets

import (
	"fmt"

	cldf "github.com/smartcontractkit/chainlink-deployments-framework/deployment"
	cldf_ops "github.com/smartcontractkit/chainlink-deployments-framework/operations"
	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/mcms"

	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/changesets"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/sequences"
)

// UpdateDestChainConfigChangesetInput changes selected fields of the Router config of a Solana
// chain's existing 2.0 lanes. A nil field keeps each lane's current value; a non-nil field
// replaces it.
type UpdateDestChainConfigChangesetInput struct {
	ChainSelector        uint64   `json:"chainSelector"`
	RemoteChainSelectors []uint64 `json:"remoteChainSelectors"`
	BaseExecutionGasCost            *uint32 `json:"baseExecutionGasCost"`
	MessageNetworkFeeUSDCents       *uint16 `json:"messageNetworkFeeUSDCents"`
	TokenTransferNetworkFeeUSDCents *uint16 `json:"tokenTransferNetworkFeeUSDCents"`
	MaxFeePerMessageUSDCents        *uint16 `json:"maxFeePerMessageUSDCents"`
	TokenReceiverAllowed            *bool   `json:"tokenReceiverAllowed"`
	MCMS                   mcms.Input `json:"mcms"`
}

// UpdateDestChainConfigChangeset applies the requested changes to the Router config of existing
// 2.0 lanes, e.g. raising base_execution_gas_cost when executions run out of gas on the
// destination. Each proposal restates the lane's whole config as read at generation time; do not
// run other lane config changes for the same lanes between generating the proposal and executing
// it.
func UpdateDestChainConfigChangeset() cldf.ChangeSetV2[UpdateDestChainConfigChangesetInput] {
	return cldf.CreateChangeSet(updateDestChainConfigApply, updateDestChainConfigVerify)
}

func updateDestChainConfigVerify(env cldf.Environment, input UpdateDestChainConfigChangesetInput) error {
	return verifyLanesV2(env, input.ChainSelector, input.RemoteChainSelectors, input.MCMS)
}

func updateDestChainConfigApply(e cldf.Environment, input UpdateDestChainConfigChangesetInput) (cldf.ChangesetOutput, error) {
	report, err := cldf_ops.ExecuteSequence(e.OperationsBundle, sequences.UpdateDestChainConfig, e.BlockChains, sequences.UpdateDestChainConfigInput{
		DataStore:                       e.DataStore,
		ChainSelector:                   input.ChainSelector,
		RemoteChainSelectors:            input.RemoteChainSelectors,
		BaseExecutionGasCost:            input.BaseExecutionGasCost,
		MessageNetworkFeeUSDCents:       input.MessageNetworkFeeUSDCents,
		TokenTransferNetworkFeeUSDCents: input.TokenTransferNetworkFeeUSDCents,
		MaxFeePerMessageUSDCents:        input.MaxFeePerMessageUSDCents,
		TokenReceiverAllowed:            input.TokenReceiverAllowed,
	})
	if err != nil {
		return cldf.ChangesetOutput{}, fmt.Errorf("failed to update dest chain config: %w", err)
	}
	if err := requireMCMSForBatchOps(e.Logger, input.MCMS, report.Output.BatchOps); err != nil {
		return cldf.ChangesetOutput{}, err
	}
	return changesets.
		NewOutputBuilder(e, changesets.GetRegistry()).
		WithReports(append([]cldf_ops.Report[any, any](nil), report.ExecutionReports...)).
		WithBatchOps(report.Output.BatchOps).
		Build(input.MCMS)
}
