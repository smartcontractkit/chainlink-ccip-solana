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

func verifySolanaChain(env cldf.Environment, chainSelector uint64) error {
	if err := cldf.IsValidChainSelector(chainSelector); err != nil {
		return fmt.Errorf("invalid chain selector %d: %w", chainSelector, err)
	}
	if !env.BlockChains.Exists(chainSelector) {
		return fmt.Errorf("chain with selector %d does not exist", chainSelector)
	}
	if _, ok := env.BlockChains.SolanaChains()[chainSelector]; !ok {
		return fmt.Errorf("chain with selector %d is not a Solana chain", chainSelector)
	}
	return nil
}

func extendCcipSendLookupTableVerify(env cldf.Environment, input ExtendCcipSendLookupTableChangesetInput) error {
	if err := verifySolanaChain(env, input.ChainSelector); err != nil {
		return err
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

type ExtendOfframpLookupTableChangesetInput struct {
	ChainSelector uint64 `json:"chainSelector"`
}

// ExtendOfframpLookupTableChangeset fills the OffRamp lookup table with the protocol account set.
func ExtendOfframpLookupTableChangeset() cldf.ChangeSetV2[ExtendOfframpLookupTableChangesetInput] {
	return cldf.CreateChangeSet(extendOfframpLookupTableApply, extendOfframpLookupTableVerify)
}

func extendOfframpLookupTableVerify(env cldf.Environment, input ExtendOfframpLookupTableChangesetInput) error {
	return verifySolanaChain(env, input.ChainSelector)
}

func extendOfframpLookupTableApply(e cldf.Environment, input ExtendOfframpLookupTableChangesetInput) (cldf.ChangesetOutput, error) {
	addresses, err := e.DataStore.Addresses().Fetch()
	if err != nil {
		return cldf.ChangesetOutput{}, fmt.Errorf("fetch existing addresses: %w", err)
	}
	report, err := cldf_ops.ExecuteSequence(e.OperationsBundle, sequences.ExtendOfframpLut, e.BlockChains, sequences.ExtendOfframpLutInput{
		ChainSelector:     input.ChainSelector,
		ExistingAddresses: addresses,
	})
	if err != nil {
		return cldf.ChangesetOutput{Reports: report.ExecutionReports}, fmt.Errorf("extend offramp lookup table: %w", err)
	}
	return cldf.ChangesetOutput{Reports: report.ExecutionReports}, nil
}

type ExtendLookupTableChangesetInput struct {
	ChainSelector uint64   `json:"chainSelector"`
	LookupTable   string   `json:"lookupTable"`
	Entries       []string `json:"entries"`
}

// ExtendLookupTableChangeset adds the given entries to a deployer-owned address lookup table.
func ExtendLookupTableChangeset() cldf.ChangeSetV2[ExtendLookupTableChangesetInput] {
	return cldf.CreateChangeSet(extendLookupTableApply, extendLookupTableVerify)
}

func extendLookupTableVerify(env cldf.Environment, input ExtendLookupTableChangesetInput) error {
	if err := verifySolanaChain(env, input.ChainSelector); err != nil {
		return err
	}
	_, err := parseExtendLookupTableInput(input)
	return err
}

func parseExtendLookupTableInput(input ExtendLookupTableChangesetInput) (sequences.ExtendLookupTableInput, error) {
	table, err := solana.PublicKeyFromBase58(input.LookupTable)
	if err != nil {
		return sequences.ExtendLookupTableInput{}, fmt.Errorf("invalid lookup table %q: %w", input.LookupTable, err)
	}
	if table.IsZero() {
		return sequences.ExtendLookupTableInput{}, errors.New("lookup table must not be the zero key")
	}
	if len(input.Entries) == 0 {
		return sequences.ExtendLookupTableInput{}, errors.New("at least one entry is required")
	}
	if len(input.Entries) > 256 {
		return sequences.ExtendLookupTableInput{}, fmt.Errorf("%d entries exceed the 256 a lookup table holds", len(input.Entries))
	}
	seen := make(map[solana.PublicKey]bool, len(input.Entries))
	entries := make([]solana.PublicKey, 0, len(input.Entries))
	for _, raw := range input.Entries {
		key, err := solana.PublicKeyFromBase58(raw)
		if err != nil {
			return sequences.ExtendLookupTableInput{}, fmt.Errorf("invalid entry %q: %w", raw, err)
		}
		if seen[key] {
			return sequences.ExtendLookupTableInput{}, fmt.Errorf("duplicate entry %s", key)
		}
		seen[key] = true
		entries = append(entries, key)
	}
	return sequences.ExtendLookupTableInput{ChainSelector: input.ChainSelector, LookupTable: table, Entries: entries}, nil
}

func extendLookupTableApply(e cldf.Environment, input ExtendLookupTableChangesetInput) (cldf.ChangesetOutput, error) {
	seqInput, err := parseExtendLookupTableInput(input)
	if err != nil {
		return cldf.ChangesetOutput{}, err
	}
	report, err := cldf_ops.ExecuteSequence(e.OperationsBundle, sequences.ExtendLookupTable, e.BlockChains, seqInput)
	if err != nil {
		return cldf.ChangesetOutput{Reports: report.ExecutionReports}, fmt.Errorf("extend lookup table: %w", err)
	}
	return cldf.ChangesetOutput{Reports: report.ExecutionReports}, nil
}
