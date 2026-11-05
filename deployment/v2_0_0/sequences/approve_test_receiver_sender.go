package sequences

import (
	"errors"
	"fmt"

	"github.com/Masterminds/semver/v3"
	"github.com/gagliardetto/solana-go"

	seq_core "github.com/smartcontractkit/chainlink-ccip/deployment/utils/sequences"
	cldf_chain "github.com/smartcontractkit/chainlink-deployments-framework/chain"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	cldf_ops "github.com/smartcontractkit/chainlink-deployments-framework/operations"
	mcmstypes "github.com/smartcontractkit/mcms/types"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	testreceiverops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/testreceiver"
)

type ApproveTestReceiverSenderInput struct {
	ChainSelector         uint64
	ExistingAddresses     []datastore.AddressRef
	ReceiverQualifier     string
	SourceChainSelector   uint64
	ApprovedRemoteSenders []string
	// ReceiverVersion selects which receiver to target. Both register as TestReceiver and are told
	// apart by version, so this picks the ref to resolve and the interface to call. Nil means the
	// current version.
	ReceiverVersion *semver.Version
}

// ResolveTestReceiver returns the receiver program address registered for the given qualifier and
// version. The v1 and v2 receivers share a contract type and differ by version.
func ResolveTestReceiver(refs []datastore.AddressRef, chainSelector uint64, qualifier string, receiverVersion *semver.Version) (solana.PublicKey, error) {
	if receiverVersion == nil {
		receiverVersion = testreceiverops.Version
	}
	return solcommon.PubkeyFromRefs(refs, datastore.NewAddressRefKey(
		chainSelector,
		datastore.ContractType(testreceiverops.ContractType),
		receiverVersion,
		qualifier,
	))
}

// ApprovedSenderPDA derives the approved-sender PDA for a remote sender, decoding the sender with
// the same padding the receiver stores it under. Exposed so callers can check whether an approval
// already exists before attempting one.
func ApprovedSenderPDA(receiver solana.PublicKey, sourceChainSelector uint64, sender string, receiverVersion *semver.Version) (solana.PublicKey, error) {
	remoteSender, err := solcommon.DecodeRemoteSender(sender)
	if err != nil {
		return solana.PublicKey{}, err
	}
	legacy, err := testreceiverops.IsLegacy(receiverVersion)
	if err != nil {
		return solana.PublicKey{}, err
	}
	if legacy {
		return state.FindApprovedSender(sourceChainSelector, remoteSender, receiver)
	}
	return state.FindApprovedSenderV2(sourceChainSelector, remoteSender, receiver)
}

// ApproveTestReceiverSender approves remote senders on an already-deployed TestReceiver, without
// touching its deployment, initialization or CCV config. Use it to add senders after the fact.
var ApproveTestReceiverSender = cldf_ops.NewSequence(
	"approve-test-receiver-sender",
	testreceiverops.Version,
	"Approves remote senders on an already-deployed TestReceiver on a Solana chain",
	func(b cldf_ops.Bundle, chains cldf_chain.BlockChains, input ApproveTestReceiverSenderInput) (seq_core.OnChainOutput, error) {
		solChain, ok := chains.SolanaChains()[input.ChainSelector]
		if !ok {
			return seq_core.OnChainOutput{}, fmt.Errorf("solana chain with selector %d not found", input.ChainSelector)
		}
		if input.ReceiverQualifier == "" {
			return seq_core.OnChainOutput{}, errors.New("receiver qualifier is required")
		}
		if len(input.ApprovedRemoteSenders) == 0 {
			return seq_core.OnChainOutput{}, errors.New("at least one approved remote sender is required")
		}

		receiver, err := ResolveTestReceiver(input.ExistingAddresses, input.ChainSelector, input.ReceiverQualifier, input.ReceiverVersion)
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("resolve receiver (qualifier %q, version %s): %w", input.ReceiverQualifier, input.ReceiverVersion, err)
		}

		var batchOps []mcmstypes.BatchOperation
		for _, sender := range input.ApprovedRemoteSenders {
			report, opErr := cldf_ops.ExecuteOperation(b, testreceiverops.ApproveSender, solChain, testreceiverops.ApproveSenderParams{
				Receiver:            receiver,
				SourceChainSelector: input.SourceChainSelector,
				RemoteSender:        sender,
				ReceiverVersion:     input.ReceiverVersion,
			})
			if opErr != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("approve remote sender %q: %w", sender, opErr)
			}
			batchOps = append(batchOps, report.Output.BatchOps...)
		}

		return seq_core.OnChainOutput{BatchOps: batchOps}, nil
	},
)
