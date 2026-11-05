package changesets

import (
	"fmt"

	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/mcms"
	cldf "github.com/smartcontractkit/chainlink-deployments-framework/deployment"
	cldf_ops "github.com/smartcontractkit/chainlink-deployments-framework/operations"
	mcmstypes "github.com/smartcontractkit/mcms/types"

	testreceiverops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/testreceiver"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/sequences"
)

// ConfigureLegacyTestReceiverSourceChangesetInput's MCMS builds the proposal when the timelock owns
// the receiver.
type ConfigureLegacyTestReceiverSourceChangesetInput struct {
	ChainSelector         uint64     `json:"chainSelector"`
	ReceiverQualifier     string     `json:"receiverQualifier"`
	SourceChainSelector   uint64     `json:"sourceChainSelector"`
	ApprovedRemoteSenders []string   `json:"approvedRemoteSenders"`
	MCMS                  mcms.Input `json:"mcms"`
}

// ConfigureLegacyTestReceiverSourceChangeset approves remote senders on an existing legacy (v1)
// TestReceiver for a new source chain. Legacy receivers have no CCV/finality configuration.
func ConfigureLegacyTestReceiverSourceChangeset() cldf.ChangeSetV2[ConfigureLegacyTestReceiverSourceChangesetInput] {
	return cldf.CreateChangeSet(configureLegacySourceApply, configureLegacySourceVerify)
}

func configureLegacySourceVerify(env cldf.Environment, input ConfigureLegacyTestReceiverSourceChangesetInput) error {
	if err := validateConfigureSourceCommon(input.ChainSelector, input.SourceChainSelector, input.ReceiverQualifier, input.ApprovedRemoteSenders); err != nil {
		return err
	}
	if err := validateMCMSInput(env.Logger, input.MCMS); err != nil {
		return err
	}
	solChain, addresses, err := fetchSolanaChain(env, input.ChainSelector, testreceiverops.LegacyVersion, input.ReceiverQualifier)
	if err != nil {
		return err
	}

	receiverV1, err := sequences.ResolveTestReceiver(addresses, input.ChainSelector, input.ReceiverQualifier, testreceiverops.LegacyVersion)
	if err != nil {
		return fmt.Errorf("resolve v1 receiver (%q): %w", input.ReceiverQualifier, err)
	}
	for _, sender := range input.ApprovedRemoteSenders {
		pda, pdaErr := sequences.ApprovedSenderPDA(receiverV1, input.SourceChainSelector, sender, testreceiverops.LegacyVersion)
		if pdaErr != nil {
			return fmt.Errorf("derive approved sender PDA for %q: %w", sender, pdaErr)
		}
		if err := assertPDAAbsent(env, solChain, pda, fmt.Sprintf("approved sender %s on receiver %s", sender, receiverV1)); err != nil {
			return err
		}
	}
	return nil
}

func configureLegacySourceApply(e cldf.Environment, input ConfigureLegacyTestReceiverSourceChangesetInput) (cldf.ChangesetOutput, error) {
	addresses, err := e.DataStore.Addresses().Fetch()
	if err != nil {
		return cldf.ChangesetOutput{}, fmt.Errorf("fetch existing addresses: %w", err)
	}
	solChain := e.BlockChains.SolanaChains()[input.ChainSelector]

	receiverV1, err := sequences.ResolveTestReceiver(addresses, input.ChainSelector, input.ReceiverQualifier, testreceiverops.LegacyVersion)
	if err != nil {
		return cldf.ChangesetOutput{}, fmt.Errorf("resolve v1 receiver: %w", err)
	}

	var reports []cldf_ops.Report[any, any]
	var batchOps []mcmstypes.BatchOperation
	for _, sender := range input.ApprovedRemoteSenders {
		report, err := cldf_ops.ExecuteOperation(e.OperationsBundle, testreceiverops.ApproveSender, solChain, testreceiverops.ApproveSenderParams{
			Receiver:            receiverV1,
			SourceChainSelector: input.SourceChainSelector,
			RemoteSender:        sender,
			ReceiverVersion:     testreceiverops.LegacyVersion,
		})
		if err != nil {
			return cldf.ChangesetOutput{}, fmt.Errorf("approve remote sender %q: %w", sender, err)
		}
		reports = append(reports, report.ToGenericReport())
		batchOps = append(batchOps, report.Output.BatchOps...)
	}

	return buildProposalOutput(e, input.MCMS, reports, batchOps)
}
