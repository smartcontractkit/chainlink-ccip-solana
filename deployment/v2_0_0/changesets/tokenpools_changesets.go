package changesets

import (
	"errors"
	"fmt"

	common_utils "github.com/smartcontractkit/chainlink-ccip/deployment/utils"
	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/changesets"
	datastore_utils "github.com/smartcontractkit/chainlink-ccip/deployment/utils/datastore"
	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/mcms"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	cldf "github.com/smartcontractkit/chainlink-deployments-framework/deployment"
	cldf_ops "github.com/smartcontractkit/chainlink-deployments-framework/operations"

	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/sequences"
)

// UpdateSelfServedAllowedChangesetInput enables or disables self-served pool initialization
// on a BurnMint or LockRelease token pool global config.
type UpdateSelfServedAllowedChangesetInput struct {
	ChainSelector     uint64               `json:"chainSelector"`
	TokenPoolRef      datastore.AddressRef `json:"tokenPoolRef"`
	SelfServedAllowed bool                 `json:"selfServedAllowed"`
	MCMS              mcms.Input           `json:"mcms"`
}

// UpdateSelfServedAllowedChangeset wraps the UpdateSelfServedAllowed sequence. The update is signed by
// the pool program's upgrade authority; when that authority is not the deployer, the instruction is
// returned as an MCMS batch instead of being sent.
func UpdateSelfServedAllowedChangeset() cldf.ChangeSetV2[UpdateSelfServedAllowedChangesetInput] {
	return cldf.CreateChangeSet(updateSelfServedAllowedApply, updateSelfServedAllowedVerify)
}

func updateSelfServedAllowedVerify(env cldf.Environment, input UpdateSelfServedAllowedChangesetInput) error {
	if err := cldf.IsValidChainSelector(input.ChainSelector); err != nil {
		return fmt.Errorf("invalid chain selector: %d - %w", input.ChainSelector, err)
	}

	if !env.BlockChains.Exists(input.ChainSelector) {
		return fmt.Errorf("chain with selector %d does not exist", input.ChainSelector)
	}

	if _, ok := env.BlockChains.SolanaChains()[input.ChainSelector]; !ok {
		return fmt.Errorf("chain with selector %d is not a Solana chain", input.ChainSelector)
	}

	if datastore_utils.IsAddressRefEmpty(input.TokenPoolRef) {
		return errors.New("tokenPoolRef must not be empty")
	}

	fullRef, err := solcommon.FindAndFormatCompatibleRef(env.DataStore, input.TokenPoolRef, input.ChainSelector, datastore_utils.FullRef)
	if err != nil {
		return fmt.Errorf("failed to resolve token pool ref: %w", err)
	}

	switch fullRef.Type.String() {
	case common_utils.BurnMintTokenPool.String(), common_utils.LockReleaseTokenPool.String():
	default:
		return fmt.Errorf("self served allowed is only supported for BurnMint and LockRelease token pools, but the pool ref has type '%s'", fullRef.Type)
	}

	// MCMS is optional: with the deployer as the pool's upgrade authority the update is sent directly.
	if err := validateMCMSInput(env.Logger, input.MCMS); err != nil {
		return fmt.Errorf("invalid MCMS configuration: %w", err)
	}
	return nil
}

func updateSelfServedAllowedApply(e cldf.Environment, input UpdateSelfServedAllowedChangesetInput) (cldf.ChangesetOutput, error) {
	fullRef, err := solcommon.FindAndFormatCompatibleRef(e.DataStore, input.TokenPoolRef, input.ChainSelector, datastore_utils.FullRef)
	if err != nil {
		return cldf.ChangesetOutput{}, fmt.Errorf("failed to resolve token pool ref: %w", err)
	}

	report, err := cldf_ops.ExecuteSequence(e.OperationsBundle, sequences.UpdateSelfServedAllowed, e.BlockChains, sequences.UpdateSelfServedAllowedInput{
		ChainSelector:     input.ChainSelector,
		TokenPoolRef:      fullRef,
		SelfServedAllowed: input.SelfServedAllowed,
	})
	if err != nil {
		return cldf.ChangesetOutput{}, fmt.Errorf("failed to update self served allowed: %w", err)
	}

	if err := requireMCMSForBatchOps(e.Logger, input.MCMS, report.Output.BatchOps); err != nil {
		return cldf.ChangesetOutput{}, err
	}
	return changesets.NewOutputBuilder(e, changesets.GetRegistry()).
		WithReports(report.ExecutionReports).
		WithSingleBatchOpPerChain(report.Output.BatchOps).
		Build(input.MCMS)
}
