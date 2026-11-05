package changesets

import (
	"fmt"

	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/changesets"
	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/mcms"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	cldf "github.com/smartcontractkit/chainlink-deployments-framework/deployment"
	cldf_ops "github.com/smartcontractkit/chainlink-deployments-framework/operations"

	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/sequences"
)

type ExtendCcipSendLookupTableChangesetInput struct {
	ChainSelector uint64 `json:"chainSelector"`
	// Only used when the router is owned by MCMS and its reference addresses still have to point at
	// the lookup table. As with the offramp's table, the deployer key is the table's authority and
	// extends it directly; the owner controls which table the router returns.
	MCMS mcms.Input `json:"mcms"`
}

func ExtendCcipSendLookupTableChangeset() cldf.ChangeSetV2[ExtendCcipSendLookupTableChangesetInput] {
	return cldf.CreateChangeSet(extendCcipSendLookupTableApply, extendCcipSendLookupTableVerify)
}

func extendCcipSendLookupTableVerify(env cldf.Environment, input ExtendCcipSendLookupTableChangesetInput) error {
	if err := cldf.IsValidChainSelector(input.ChainSelector); err != nil {
		return fmt.Errorf("invalid chain selector %d: %w", input.ChainSelector, err)
	}
	if !env.BlockChains.Exists(input.ChainSelector) {
		return fmt.Errorf("chain with selector %d does not exist", input.ChainSelector)
	}
	if _, ok := env.BlockChains.SolanaChains()[input.ChainSelector]; !ok {
		return fmt.Errorf("chain with selector %d is not a Solana chain", input.ChainSelector)
	}
	// The deployer creates and extends the table directly before the MCMS proposal is built, so a
	// bad MCMS input has to be caught here, before anything is sent.
	return validateMCMSInput(env.Logger, input.MCMS)
}

func extendCcipSendLookupTableApply(e cldf.Environment, input ExtendCcipSendLookupTableChangesetInput) (cldf.ChangesetOutput, error) {
	return applyCcipSendLut(e, input.ChainSelector, input.MCMS, false)
}

func applyCcipSendLut(e cldf.Environment, chainSelector uint64, mcmsInput mcms.Input, createIfMissing bool) (cldf.ChangesetOutput, error) {
	addresses, err := e.DataStore.Addresses().Fetch()
	if err != nil {
		return cldf.ChangesetOutput{}, fmt.Errorf("fetch existing addresses: %w", err)
	}
	report, err := cldf_ops.ExecuteSequence(e.OperationsBundle, sequences.ExtendCcipSendLut, e.BlockChains, sequences.ExtendLutInput{
		ChainSelector:     chainSelector,
		ExistingAddresses: addresses,
		CreateIfMissing:   createIfMissing,
	})
	if err != nil {
		return cldf.ChangesetOutput{}, fmt.Errorf("extend ccip_send lookup table: %w", err)
	}
	ds := datastore.NewMemoryDataStore()
	for _, ref := range report.Output.Addresses {
		if err := ds.Addresses().Add(ref); err != nil {
			return cldf.ChangesetOutput{Reports: report.ExecutionReports}, fmt.Errorf("add ccip_send lookup table ref: %w", err)
		}
	}
	if err := requireMCMSForBatchOps(e.Logger, mcmsInput, report.Output.BatchOps); err != nil {
		return cldf.ChangesetOutput{}, err
	}
	return changesets.
		NewOutputBuilder(e, changesets.GetRegistry()).
		WithReports(append([]cldf_ops.Report[any, any](nil), report.ExecutionReports...)).
		WithDataStore(ds).
		WithBatchOps(report.Output.BatchOps).
		Build(mcmsInput)
}
