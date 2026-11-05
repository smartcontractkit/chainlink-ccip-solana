package changesets

import (
	"errors"
	"fmt"

	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	cldf "github.com/smartcontractkit/chainlink-deployments-framework/deployment"
	cldf_ops "github.com/smartcontractkit/chainlink-deployments-framework/operations"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/example_ccip_receiver_v2"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/sequences"
)

type DeployV2TestReceiverChangesetInput struct {
	ChainSelector         uint64                                  `json:"chainSelector"`
	ReceiverQualifier     string                                  `json:"receiverQualifier"`
	SourceChainSelector   uint64                                  `json:"sourceChainSelector"`
	ApprovedRemoteSenders []string                                `json:"approvedRemoteSenders"`
	RequiredCCVQualifiers []string                                `json:"requiredCCVQualifiers"`
	OptionalCCVQualifiers []string                                `json:"optionalCCVQualifiers"`
	OptionalThreshold     uint8                                   `json:"optionalThreshold"`
	AllowedFinalityConfig example_ccip_receiver_v2.FinalityConfig `json:"allowedFinalityConfig"`
}

func DeployV2TestReceiverChangeset() cldf.ChangeSetV2[DeployV2TestReceiverChangesetInput] {
	return cldf.CreateChangeSet(deployV2TestReceiverApply, deployV2TestReceiverVerify)
}

func deployV2TestReceiverVerify(env cldf.Environment, input DeployV2TestReceiverChangesetInput) error {
	if err := cldf.IsValidChainSelector(input.ChainSelector); err != nil {
		return fmt.Errorf("invalid chain selector %d: %w", input.ChainSelector, err)
	}
	if !env.BlockChains.Exists(input.ChainSelector) {
		return fmt.Errorf("chain with selector %d does not exist", input.ChainSelector)
	}
	if _, ok := env.BlockChains.SolanaChains()[input.ChainSelector]; !ok {
		return fmt.Errorf("chain with selector %d is not a Solana chain", input.ChainSelector)
	}
	if err := cldf.IsValidChainSelector(input.SourceChainSelector); err != nil {
		return fmt.Errorf("invalid source chain selector %d: %w", input.SourceChainSelector, err)
	}
	if input.ReceiverQualifier == "" {
		return errors.New("receiver qualifier is required")
	}
	if len(input.ApprovedRemoteSenders) == 0 {
		return errors.New("at least one approved remote sender is required")
	}
	if len(input.RequiredCCVQualifiers) == 0 {
		return errors.New("at least one required CCV qualifier is required")
	}
	return nil
}
func deployV2TestReceiverApply(e cldf.Environment, input DeployV2TestReceiverChangesetInput) (cldf.ChangesetOutput, error) {
	addresses, err := e.DataStore.Addresses().Fetch()
	if err != nil {
		return cldf.ChangesetOutput{}, fmt.Errorf("fetch existing addresses: %w", err)
	}
	report, err := cldf_ops.ExecuteSequence(e.OperationsBundle, sequences.DeployV2TestReceiver, e.BlockChains, sequences.DeployV2TestReceiverInput{
		ChainSelector:         input.ChainSelector,
		ExistingAddresses:     addresses,
		ReceiverQualifier:     input.ReceiverQualifier,
		SourceChainSelector:   input.SourceChainSelector,
		ApprovedRemoteSenders: input.ApprovedRemoteSenders,
		RequiredCCVQualifiers: input.RequiredCCVQualifiers,
		OptionalCCVQualifiers: input.OptionalCCVQualifiers,
		OptionalThreshold:     input.OptionalThreshold,
		AllowedFinalityConfig: input.AllowedFinalityConfig,
	})
	if err != nil {
		return cldf.ChangesetOutput{}, fmt.Errorf("deploy example receiver: %w", err)
	}
	ds := datastore.NewMemoryDataStore()
	for _, address := range report.Output.Addresses {
		if err := ds.Addresses().Add(address); err != nil {
			return cldf.ChangesetOutput{}, fmt.Errorf("add receiver address to datastore: %w", err)
		}
	}
	return cldf.ChangesetOutput{
		Reports:   append([]cldf_ops.Report[any, any](nil), report.ExecutionReports...),
		DataStore: ds,
	}, nil
}

type DeployLegacyTestReceiverChangesetInput struct {
	ChainSelector         uint64   `json:"chainSelector"`
	ReceiverQualifier     string   `json:"receiverQualifier"`
	SourceChainSelector   uint64   `json:"sourceChainSelector"`
	ApprovedRemoteSenders []string `json:"approvedRemoteSenders"`
}

func DeployLegacyTestReceiverChangeset() cldf.ChangeSetV2[DeployLegacyTestReceiverChangesetInput] {
	return cldf.CreateChangeSet(deployLegacyTestReceiverApply, deployLegacyTestReceiverVerify)
}

func deployLegacyTestReceiverVerify(env cldf.Environment, input DeployLegacyTestReceiverChangesetInput) error {
	if err := cldf.IsValidChainSelector(input.ChainSelector); err != nil {
		return fmt.Errorf("invalid chain selector %d: %w", input.ChainSelector, err)
	}
	if !env.BlockChains.Exists(input.ChainSelector) {
		return fmt.Errorf("chain with selector %d does not exist", input.ChainSelector)
	}
	if _, ok := env.BlockChains.SolanaChains()[input.ChainSelector]; !ok {
		return fmt.Errorf("chain with selector %d is not a Solana chain", input.ChainSelector)
	}
	if err := cldf.IsValidChainSelector(input.SourceChainSelector); err != nil {
		return fmt.Errorf("invalid source chain selector %d: %w", input.SourceChainSelector, err)
	}
	if input.ReceiverQualifier == "" {
		return errors.New("receiver qualifier is required")
	}
	if len(input.ApprovedRemoteSenders) == 0 {
		return errors.New("at least one approved remote sender is required")
	}
	return nil
}

func deployLegacyTestReceiverApply(e cldf.Environment, input DeployLegacyTestReceiverChangesetInput) (cldf.ChangesetOutput, error) {
	addresses, err := e.DataStore.Addresses().Fetch()
	if err != nil {
		return cldf.ChangesetOutput{}, fmt.Errorf("fetch existing addresses: %w", err)
	}
	report, err := cldf_ops.ExecuteSequence(e.OperationsBundle, sequences.DeployLegacyTestReceiver, e.BlockChains, sequences.DeployLegacyTestReceiverInput{
		ChainSelector:         input.ChainSelector,
		ExistingAddresses:     addresses,
		ReceiverQualifier:     input.ReceiverQualifier,
		SourceChainSelector:   input.SourceChainSelector,
		ApprovedRemoteSenders: input.ApprovedRemoteSenders,
	})
	if err != nil {
		return cldf.ChangesetOutput{}, fmt.Errorf("deploy legacy test receiver: %w", err)
	}
	ds := datastore.NewMemoryDataStore()
	for _, address := range report.Output.Addresses {
		if err := ds.Addresses().Add(address); err != nil {
			return cldf.ChangesetOutput{}, fmt.Errorf("add receiver address to datastore: %w", err)
		}
	}
	return cldf.ChangesetOutput{
		Reports:   append([]cldf_ops.Report[any, any](nil), report.ExecutionReports...),
		DataStore: ds,
	}, nil
}
