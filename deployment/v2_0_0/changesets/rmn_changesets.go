package changesets

import (
	"errors"
	"fmt"

	"github.com/gagliardetto/solana-go"

	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/changesets"
	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/mcms"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	cldf "github.com/smartcontractkit/chainlink-deployments-framework/deployment"
	cldf_ops "github.com/smartcontractkit/chainlink-deployments-framework/operations"

	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/sequences" // register sequences
)

type RMNRemoteSetEventAuthoritiesChangesetInput struct {
	ChainSelector uint64     `json:"chainSelector"`
	MCMS          mcms.Input `json:"mcms"`
}

func RMNRemoteSetEventAuthoritiesChangeset() cldf.ChangeSetV2[RMNRemoteSetEventAuthoritiesChangesetInput] {
	return cldf.CreateChangeSet(rmnSetEventAuthoritiesApply, rmnSetEventAuthoritiesVerify)
}

func rmnSetEventAuthoritiesVerify(env cldf.Environment, input RMNRemoteSetEventAuthoritiesChangesetInput) error {
	if err := cldf.IsValidChainSelector(input.ChainSelector); err != nil {
		return fmt.Errorf("invalid chain selector: %d - %w", input.ChainSelector, err)
	}
	if !env.BlockChains.Exists(input.ChainSelector) {
		return fmt.Errorf("chain with selector %d does not exist", input.ChainSelector)
	}

	if err := validateMCMSInput(env.Logger, input.MCMS); err != nil {
		return fmt.Errorf("invalid MCMS configuration: %w", err)
	}

	return nil
}

func rmnSetEventAuthoritiesApply(e cldf.Environment, input RMNRemoteSetEventAuthoritiesChangesetInput) (cldf.ChangesetOutput, error) {
	// Prepare sequence input
	seqInput := sequences.RMNRemoteSetEventAuthoritiesSequenceInput{
		DataStore: e.DataStore,
		Selector:  input.ChainSelector,
	}

	report, err := cldf_ops.ExecuteSequence(e.OperationsBundle, sequences.SetRMNRemoteEventAuthorities, e.BlockChains, seqInput)
	if err != nil {
		return cldf.ChangesetOutput{}, fmt.Errorf("failed to set rmn event authorities: %w", err)
	}

	reports := make([]cldf_ops.Report[any, any], 0, len(report.ExecutionReports))
	reports = append(reports, report.ExecutionReports...)

	// Create the datastore with the addresses from the report
	ds := datastore.NewMemoryDataStore()
	for _, addr := range report.Output.Addresses {
		if err := ds.Addresses().Add(addr); err != nil {
			return cldf.ChangesetOutput{}, fmt.Errorf("failed to add address to datastore: %w", err)
		}
	}

	if err := requireMCMSForBatchOps(e.Logger, input.MCMS, report.Output.BatchOps); err != nil {
		return cldf.ChangesetOutput{}, err
	}
	return changesets.
		NewOutputBuilder(e, changesets.GetRegistry()).
		WithReports(reports).
		WithDataStore(ds).
		WithBatchOps(report.Output.BatchOps).
		Build(input.MCMS)
}

type RMNRemoteSetCurserChangesetInput struct {
	ChainSelector uint64           `json:"chainSelector"`
	Curser        solana.PublicKey `json:"curser"`
	MCMS          mcms.Input       `json:"mcms"`
}

// RMNRemoteSetCurserChangeset sets the account allowed to curse and uncurse on RMNRemote.
func RMNRemoteSetCurserChangeset() cldf.ChangeSetV2[RMNRemoteSetCurserChangesetInput] {
	return cldf.CreateChangeSet(rmnSetCurserApply, rmnSetCurserVerify)
}

func rmnSetCurserVerify(env cldf.Environment, input RMNRemoteSetCurserChangesetInput) error {
	if err := cldf.IsValidChainSelector(input.ChainSelector); err != nil {
		return fmt.Errorf("invalid chain selector: %d - %w", input.ChainSelector, err)
	}
	if !env.BlockChains.Exists(input.ChainSelector) {
		return fmt.Errorf("chain with selector %d does not exist", input.ChainSelector)
	}
	if input.Curser.IsZero() {
		return errors.New("curser public key must not be zero")
	}
	if err := validateMCMSInput(env.Logger, input.MCMS); err != nil {
		return fmt.Errorf("invalid MCMS configuration: %w", err)
	}
	return nil
}

func rmnSetCurserApply(e cldf.Environment, input RMNRemoteSetCurserChangesetInput) (cldf.ChangesetOutput, error) {
	report, err := cldf_ops.ExecuteSequence(e.OperationsBundle, sequences.SetRMNRemoteCurser, e.BlockChains, sequences.RMNRemoteSetCurserSequenceInput{
		DataStore: e.DataStore,
		Curser:    input.Curser,
		Selector:  input.ChainSelector,
	})
	if err != nil {
		return cldf.ChangesetOutput{}, fmt.Errorf("failed to set rmn curser: %w", err)
	}

	reports := make([]cldf_ops.Report[any, any], 0, len(report.ExecutionReports))
	reports = append(reports, report.ExecutionReports...)

	if err := requireMCMSForBatchOps(e.Logger, input.MCMS, report.Output.BatchOps); err != nil {
		return cldf.ChangesetOutput{}, err
	}
	return changesets.
		NewOutputBuilder(e, changesets.GetRegistry()).
		WithReports(reports).
		WithBatchOps(report.Output.BatchOps).
		Build(input.MCMS)
}
