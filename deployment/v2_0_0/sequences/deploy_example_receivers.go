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

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/example_ccip_receiver_v2"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	cvops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/committeeverifier"
	routerops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/router"
	testreceiverops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/testreceiver"
)

type DeployV2TestReceiverInput struct {
	ChainSelector         uint64
	ExistingAddresses     []datastore.AddressRef
	ReceiverQualifier     string
	SourceChainSelector   uint64
	ApprovedRemoteSenders []string
	RequiredCCVQualifiers []string
	OptionalCCVQualifiers []string
	OptionalThreshold     uint8
	AllowedFinalityConfig example_ccip_receiver_v2.FinalityConfig
}

// The sequence is a bit long because it deploys, initializes, registers with the router, sets CCV config, and approves remote senders.
var DeployV2TestReceiver = cldf_ops.NewSequence(
	"deploy-v2-test-receiver",
	semver.MustParse("2.0.0"),
	"Deploys, initializes, and configures an V2 TestReceiver on a Solana chain",
	func(b cldf_ops.Bundle, chains cldf_chain.BlockChains, input DeployV2TestReceiverInput) (seq_core.OnChainOutput, error) {
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
		requiredCCVs, err := cvops.ResolveCommitteeVerifiers(input.ExistingAddresses, input.ChainSelector, input.RequiredCCVQualifiers)
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("resolve required CCVs: %w", err)
		}
		optionalCCVs, err := cvops.ResolveCommitteeVerifiers(input.ExistingAddresses, input.ChainSelector, input.OptionalCCVQualifiers)
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("resolve optional CCVs: %w", err)
		}

		deployReport, err := cldf_ops.ExecuteOperation(b, testreceiverops.Deploy, solChain, testreceiverops.DeployParams{
			ExistingAddresses: input.ExistingAddresses,
			Qualifier:         input.ReceiverQualifier,
		})
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("deploy receiver: %w", err)
		}
		receiver, err := solana.PublicKeyFromBase58(deployReport.Output.Address)
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("parse receiver address %q: %w", deployReport.Output.Address, err)
		}

		if _, err = cldf_ops.ExecuteOperation(b, testreceiverops.Initialize, solChain, testreceiverops.Params{
			Router:          router,
			Receiver:        receiver,
			ReceiverVersion: testreceiverops.Version,
		}); err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("initialize receiver: %w", err)
		}
		if _, err = cldf_ops.ExecuteOperation(b, testreceiverops.RegisterWithRouter, solChain, testreceiverops.RegisterWithRouterParams{
			Router:   router,
			Receiver: receiver,
		}); err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("register receiver with router: %w", err)
		}
		if _, err = cldf_ops.ExecuteOperation(b, testreceiverops.SetCCVConfig, solChain, testreceiverops.SetCCVConfigParams{
			Receiver:              receiver,
			SourceChainSelector:   input.SourceChainSelector,
			RequiredCCVs:          requiredCCVs,
			OptionalCCVs:          optionalCCVs,
			OptionalThreshold:     input.OptionalThreshold,
			AllowedFinalityConfig: input.AllowedFinalityConfig,
		}); err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("set receiver CCV config: %w", err)
		}
		for _, sender := range input.ApprovedRemoteSenders {
			if _, err = cldf_ops.ExecuteOperation(b, testreceiverops.ApproveSender, solChain, testreceiverops.ApproveSenderParams{
				Receiver:            receiver,
				SourceChainSelector: input.SourceChainSelector,
				RemoteSender:        sender,
				ReceiverVersion:     testreceiverops.Version,
			}); err != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("approve remote sender %q: %w", sender, err)
			}
		}

		return seq_core.OnChainOutput{Addresses: []datastore.AddressRef{deployReport.Output}}, nil
	},
)
