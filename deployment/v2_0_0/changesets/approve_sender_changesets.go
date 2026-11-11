package changesets

import (
	"errors"
	"fmt"

	"github.com/Masterminds/semver/v3"

	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/mcms"
	cldf "github.com/smartcontractkit/chainlink-deployments-framework/deployment"
	cldf_ops "github.com/smartcontractkit/chainlink-deployments-framework/operations"

	testreceiverops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/testreceiver"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/sequences"
)

// ApproveTestReceiverSenderChangesetInput approves remote senders on a receiver that is already
// deployed. The receiver is resolved from the datastore by qualifier, so nothing is redeployed and
// no other receiver state is touched. MCMS builds the proposal when the timelock owns the receiver.
// The receiver derives the approved-sender PDA from the exact sender bytes in the message, so the
// v2 receiver approves the 32-byte padded sender (2.0 lanes) and the legacy (v1) receiver approves
// both that and the raw 20-byte sender (1.6 lanes). Approvals that already exist are skipped, so
// re-running the changeset only creates what is missing.
type ApproveTestReceiverSenderChangesetInput struct {
	ChainSelector         uint64     `json:"chainSelector"`
	ReceiverQualifier     string     `json:"receiverQualifier"`
	SourceChainSelector   uint64     `json:"sourceChainSelector"`
	ApprovedRemoteSenders []string   `json:"approvedRemoteSenders"`
	MCMS                  mcms.Input `json:"mcms"`
}

// ApproveTestReceiverSenderChangeset approves senders on the v2 receiver.
func ApproveTestReceiverSenderChangeset() cldf.ChangeSetV2[ApproveTestReceiverSenderChangesetInput] {
	return cldf.CreateChangeSet(approveSenderApply(testreceiverops.Version), approveSenderVerify(testreceiverops.Version))
}

// ApproveLegacyTestReceiverSenderChangeset approves senders on the legacy (v1) receiver.
func ApproveLegacyTestReceiverSenderChangeset() cldf.ChangeSetV2[ApproveTestReceiverSenderChangesetInput] {
	return cldf.CreateChangeSet(approveSenderApply(testreceiverops.LegacyVersion), approveSenderVerify(testreceiverops.LegacyVersion))
}

func approveSenderVerify(receiverVersion *semver.Version) func(cldf.Environment, ApproveTestReceiverSenderChangesetInput) error {
	return func(env cldf.Environment, input ApproveTestReceiverSenderChangesetInput) error {
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
		if err := validateMCMSInput(env.Logger, input.MCMS); err != nil {
			return err
		}

		addresses, err := env.DataStore.Addresses().Fetch()
		if err != nil {
			return fmt.Errorf("fetch existing addresses: %w", err)
		}
		receiver, err := sequences.ResolveTestReceiver(addresses, input.ChainSelector, input.ReceiverQualifier, receiverVersion)
		if err != nil {
			return fmt.Errorf("resolve receiver (qualifier %q, version %s): %w", input.ReceiverQualifier, receiverVersion, err)
		}
		return senderApprovals{
			Receiver:            receiver,
			SourceChainSelector: input.SourceChainSelector,
			Senders:             input.ApprovedRemoteSenders,
			ReceiverVersion:     receiverVersion,
		}.validate()
	}
}

func approveSenderApply(receiverVersion *semver.Version) func(cldf.Environment, ApproveTestReceiverSenderChangesetInput) (cldf.ChangesetOutput, error) {
	return func(e cldf.Environment, input ApproveTestReceiverSenderChangesetInput) (cldf.ChangesetOutput, error) {
		addresses, err := e.DataStore.Addresses().Fetch()
		if err != nil {
			return cldf.ChangesetOutput{}, fmt.Errorf("fetch existing addresses: %w", err)
		}
		receiver, err := sequences.ResolveTestReceiver(addresses, input.ChainSelector, input.ReceiverQualifier, receiverVersion)
		if err != nil {
			return cldf.ChangesetOutput{}, fmt.Errorf("resolve receiver (qualifier %q, version %s): %w", input.ReceiverQualifier, receiverVersion, err)
		}
		solChain, ok := e.BlockChains.SolanaChains()[input.ChainSelector]
		if !ok {
			return cldf.ChangesetOutput{}, fmt.Errorf("chain with selector %d is not a Solana chain", input.ChainSelector)
		}
		pending, err := senderApprovals{
			Receiver:            receiver,
			SourceChainSelector: input.SourceChainSelector,
			Senders:             input.ApprovedRemoteSenders,
			ReceiverVersion:     receiverVersion,
		}.pending(e, solChain)
		if err != nil {
			return cldf.ChangesetOutput{}, err
		}
		if len(pending) == 0 {
			return cldf.ChangesetOutput{}, nil
		}
		report, err := cldf_ops.ExecuteSequence(e.OperationsBundle, sequences.ApproveTestReceiverSender, e.BlockChains, sequences.ApproveTestReceiverSenderInput{
			ChainSelector:       input.ChainSelector,
			ExistingAddresses:   addresses,
			ReceiverQualifier:   input.ReceiverQualifier,
			SourceChainSelector: input.SourceChainSelector,
			Approvals:           pending,
			ReceiverVersion:     receiverVersion,
		})
		if err != nil {
			return cldf.ChangesetOutput{}, fmt.Errorf("approve test receiver sender: %w", err)
		}
		// No addresses are produced: the receiver already exists in the datastore.
		return buildProposalOutput(e, input.MCMS, append([]cldf_ops.Report[any, any](nil), report.ExecutionReports...), report.Output.BatchOps)
	}
}
