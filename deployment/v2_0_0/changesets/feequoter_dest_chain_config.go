package changesets

import (
	"errors"
	"fmt"

	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/changesets"
	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/mcms"
	cldf "github.com/smartcontractkit/chainlink-deployments-framework/deployment"
	cldf_ops "github.com/smartcontractkit/chainlink-deployments-framework/operations"

	feeqops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/feequoter"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/sequences"
)

// UpdateFeeQuoterDestChainConfigChangesetInput changes selected dynamic fields of the FeeQuoter
// config of a Solana chain's existing 2.0 lanes. A nil field keeps each lane's current value; a
// non-nil field replaces it. LaneCodeVersion and ChainFamilySelector are fixed by the lane and
// are not inputs. The USD per unit gas price is a separate price update.
type UpdateFeeQuoterDestChainConfigChangesetInput struct {
	ChainSelector               uint64     `json:"chainSelector"`
	RemoteChainSelectors        []uint64   `json:"remoteChainSelectors"`
	IsEnabled                   *bool      `json:"isEnabled"`
	MaxDataBytes                *uint32    `json:"maxDataBytes"`
	MaxPerMsgGasLimit           *uint32    `json:"maxPerMsgGasLimit"`
	DestGasOverhead             *uint32    `json:"destGasOverhead"`
	DestGasPerPayloadByteBase   *uint32    `json:"destGasPerPayloadByteBase"`
	DefaultTokenFeeUsdCents     *uint16    `json:"defaultTokenFeeUsdCents"`
	DefaultTokenDestGasOverhead *uint32    `json:"defaultTokenDestGasOverhead"`
	DefaultTxGasLimit           *uint32    `json:"defaultTxGasLimit"`
	NetworkFeeUSDCents          *uint16    `json:"networkFeeUSDCents"`
	LinkFeeMultiplierPercent    *uint8     `json:"linkFeeMultiplierPercent"`
	MCMS                        mcms.Input `json:"mcms"`
}

func (input UpdateFeeQuoterDestChainConfigChangesetInput) destChainConfigUpdate() feeqops.DestChainConfigUpdate {
	return feeqops.DestChainConfigUpdate{
		IsEnabled:                   input.IsEnabled,
		MaxDataBytes:                input.MaxDataBytes,
		MaxPerMsgGasLimit:           input.MaxPerMsgGasLimit,
		DestGasOverhead:             input.DestGasOverhead,
		DestGasPerPayloadByteBase:   input.DestGasPerPayloadByteBase,
		DefaultTokenFeeUsdCents:     input.DefaultTokenFeeUsdCents,
		DefaultTokenDestGasOverhead: input.DefaultTokenDestGasOverhead,
		DefaultTxGasLimit:           input.DefaultTxGasLimit,
		NetworkFeeUSDCents:          input.NetworkFeeUSDCents,
		LinkFeeMultiplierPercent:    input.LinkFeeMultiplierPercent,
	}
}

// UpdateFeeQuoterDestChainConfigChangeset applies the requested changes to the FeeQuoter config of
// existing 2.0 lanes, e.g. raising dest_gas_per_payload_byte_base when executions run out of gas
// on the destination. Each proposal restates the lane's whole config as read at generation time;
// do not run other lane config changes for the same lanes between generating the proposal and
// executing it.
func UpdateFeeQuoterDestChainConfigChangeset() cldf.ChangeSetV2[UpdateFeeQuoterDestChainConfigChangesetInput] {
	return cldf.CreateChangeSet(updateFeeQuoterDestChainConfigApply, updateFeeQuoterDestChainConfigVerify)
}

func updateFeeQuoterDestChainConfigVerify(env cldf.Environment, input UpdateFeeQuoterDestChainConfigChangesetInput) error {
	if input.destChainConfigUpdate().IsEmpty() {
		return errors.New("at least one dest chain config field is required")
	}
	return verifyLanesV2(env, input.ChainSelector, input.RemoteChainSelectors, input.MCMS)
}

func updateFeeQuoterDestChainConfigApply(e cldf.Environment, input UpdateFeeQuoterDestChainConfigChangesetInput) (cldf.ChangesetOutput, error) {
	report, err := cldf_ops.ExecuteSequence(e.OperationsBundle, sequences.UpdateFeeQuoterDestChainConfig, e.BlockChains, sequences.UpdateFeeQuoterDestChainConfigInput{
		DataStore:             e.DataStore,
		ChainSelector:         input.ChainSelector,
		RemoteChainSelectors:  input.RemoteChainSelectors,
		DestChainConfigUpdate: input.destChainConfigUpdate(),
	})
	if err != nil {
		return cldf.ChangesetOutput{}, fmt.Errorf("failed to update fee quoter dest chain config: %w", err)
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
