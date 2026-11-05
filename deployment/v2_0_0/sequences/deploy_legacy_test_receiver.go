package sequences

import (
	"errors"
	"fmt"

	"github.com/gagliardetto/solana-go"

	seq_core "github.com/smartcontractkit/chainlink-ccip/deployment/utils/sequences"
	cldf_chain "github.com/smartcontractkit/chainlink-deployments-framework/chain"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	cldf_ops "github.com/smartcontractkit/chainlink-deployments-framework/operations"

	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	routerops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/router"
	testreceiverops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/testreceiver"
)

type DeployLegacyTestReceiverInput struct {
	ChainSelector         uint64
	ExistingAddresses     []datastore.AddressRef
	ReceiverQualifier     string
	SourceChainSelector   uint64
	ApprovedRemoteSenders []string
}

// DeployLegacyTestReceiver deploys, initializes and approves remote senders on the legacy (v1)
// ExampleReceiver. Unlike the v2 receiver it has no router registration and no CCV config, so the
// sequence is Deploy -> Initialize -> ApproveSender. The receiver is registered in the datastore as
// TestReceiver at version 1.0.0, which is what distinguishes it from the v2 entry.
var DeployLegacyTestReceiver = cldf_ops.NewSequence(
	"deploy-legacy-test-receiver",
	testreceiverops.LegacyVersion,
	"Deploys, initializes and configures a legacy TestReceiver on a Solana chain",
	func(b cldf_ops.Bundle, chains cldf_chain.BlockChains, input DeployLegacyTestReceiverInput) (seq_core.OnChainOutput, error) {
		solChain, ok := chains.SolanaChains()[input.ChainSelector]
		if !ok {
			return seq_core.OnChainOutput{}, fmt.Errorf("solana chain with selector %d not found", input.ChainSelector)
		}
		if input.ReceiverQualifier == "" {
			return seq_core.OnChainOutput{}, errors.New("receiver qualifier is required")
		}

		router, err := solcommon.PubkeyFromRefs(input.ExistingAddresses, datastore.NewAddressRefKey(
			input.ChainSelector,
			datastore.ContractType(routerops.ContractType),
			routerops.Version,
			"",
		))
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("resolve router: %w", err)
		}

		deployReport, err := cldf_ops.ExecuteOperation(b, testreceiverops.DeployLegacy, solChain, testreceiverops.DeployParams{
			ExistingAddresses: input.ExistingAddresses,
			Qualifier:         input.ReceiverQualifier,
		})
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("deploy legacy receiver: %w", err)
		}
		receiver, err := solana.PublicKeyFromBase58(deployReport.Output.Address)
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("parse legacy receiver address %q: %w", deployReport.Output.Address, err)
		}

		if _, err = cldf_ops.ExecuteOperation(b, testreceiverops.Initialize, solChain, testreceiverops.Params{
			Router:          router,
			Receiver:        receiver,
			ReceiverVersion: testreceiverops.LegacyVersion,
		}); err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("initialize legacy receiver: %w", err)
		}

		for _, sender := range input.ApprovedRemoteSenders {
			if _, err = cldf_ops.ExecuteOperation(b, testreceiverops.ApproveSender, solChain, testreceiverops.ApproveSenderParams{
				Receiver:            receiver,
				SourceChainSelector: input.SourceChainSelector,
				RemoteSender:        sender,
				ReceiverVersion:     testreceiverops.LegacyVersion,
			}); err != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("approve remote sender %q: %w", sender, err)
			}
		}

		return seq_core.OnChainOutput{Addresses: []datastore.AddressRef{deployReport.Output}}, nil
	},
)
