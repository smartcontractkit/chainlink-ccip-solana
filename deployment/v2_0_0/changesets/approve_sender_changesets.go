package changesets

import (
	"errors"
	"fmt"

	"github.com/Masterminds/semver/v3"
	"github.com/gagliardetto/solana-go/rpc"

	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/mcms"
	cldf "github.com/smartcontractkit/chainlink-deployments-framework/deployment"
	cldf_ops "github.com/smartcontractkit/chainlink-deployments-framework/operations"

	testreceiverops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/testreceiver"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/sequences"
)

// ApproveTestReceiverSenderChangesetInput approves remote senders on a receiver that is already
// deployed. The receiver is resolved from the datastore by qualifier, so nothing is redeployed and
// no other receiver state is touched. MCMS builds the proposal when the timelock owns the receiver.
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
		solChain, ok := env.BlockChains.SolanaChains()[input.ChainSelector]
		if !ok {
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

		// The receiver must already exist, and no sender may already be approved. ApproveSender
		// creates the approved-sender PDA, so re-approving fails mid-apply with an opaque
		// account-already-in-use; catching it here keeps the run from sending any transaction.
		addresses, err := env.DataStore.Addresses().Fetch()
		if err != nil {
			return fmt.Errorf("fetch existing addresses: %w", err)
		}
		receiver, err := sequences.ResolveTestReceiver(addresses, input.ChainSelector, input.ReceiverQualifier, receiverVersion)
		if err != nil {
			return fmt.Errorf("resolve receiver (qualifier %q, version %s): %w", input.ReceiverQualifier, receiverVersion, err)
		}
		for _, sender := range input.ApprovedRemoteSenders {
			pda, pdaErr := sequences.ApprovedSenderPDA(receiver, input.SourceChainSelector, sender, receiverVersion)
			if pdaErr != nil {
				return fmt.Errorf("derive approved sender PDA for %q: %w", sender, pdaErr)
			}
			acc, accErr := solChain.Client.GetAccountInfoWithOpts(
				env.GetContext(),
				pda,
				&rpc.GetAccountInfoOpts{Commitment: rpc.CommitmentConfirmed},
			)
			// Not found just means the approval does not exist yet, which is the expected case.
			if accErr != nil && !errors.Is(accErr, rpc.ErrNotFound) {
				return fmt.Errorf("read approved sender PDA %s for %q: %w", pda, sender, accErr)
			}
			if acc != nil && acc.Value != nil {
				return fmt.Errorf("remote sender %s is already approved on receiver %s (PDA %s)", sender, receiver, pda)
			}
		}
		return nil
	}
}

func approveSenderApply(receiverVersion *semver.Version) func(cldf.Environment, ApproveTestReceiverSenderChangesetInput) (cldf.ChangesetOutput, error) {
	return func(e cldf.Environment, input ApproveTestReceiverSenderChangesetInput) (cldf.ChangesetOutput, error) {
		addresses, err := e.DataStore.Addresses().Fetch()
		if err != nil {
			return cldf.ChangesetOutput{}, fmt.Errorf("fetch existing addresses: %w", err)
		}
		report, err := cldf_ops.ExecuteSequence(e.OperationsBundle, sequences.ApproveTestReceiverSender, e.BlockChains, sequences.ApproveTestReceiverSenderInput{
			ChainSelector:         input.ChainSelector,
			ExistingAddresses:     addresses,
			ReceiverQualifier:     input.ReceiverQualifier,
			SourceChainSelector:   input.SourceChainSelector,
			ApprovedRemoteSenders: input.ApprovedRemoteSenders,
			ReceiverVersion:       receiverVersion,
		})
		if err != nil {
			return cldf.ChangesetOutput{}, fmt.Errorf("approve test receiver sender: %w", err)
		}
		// No addresses are produced: the receiver already exists in the datastore.
		return buildProposalOutput(e, input.MCMS, append([]cldf_ops.Report[any, any](nil), report.ExecutionReports...), report.Output.BatchOps)
	}
}
