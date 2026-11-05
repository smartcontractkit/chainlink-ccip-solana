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
	testsenderops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/testsender"
)

// LegacyTestSenderDestChain is a destination chain to enable on the legacy sender.
type LegacyTestSenderDestChain struct {
	DestChainSelector uint64 `json:"destChainSelector"`
	// Recipient is the hex receiver address on the destination chain. See
	// testsenderops.SetChainConfigParams.Recipient.
	Recipient string `json:"recipient"`
	// ExtraArgs is the hex, tag-prefixed extra args blob attached to every message for this
	// chain, passed through as-is. Empty is valid: the router applies its defaults.
	ExtraArgs string `json:"extraArgs"`
}

type DeployLegacyTestSenderInput struct {
	ChainSelector     uint64
	ExistingAddresses []datastore.AddressRef
	SenderQualifier   string
	DestChains        []LegacyTestSenderDestChain
	// TokenMints are the base58 mints to create ccip_sender PDA token accounts for. The legacy
	// ccip_send requires the PDA's account for the fee mint (wrapped SOL for native fees) to exist,
	// and does not create it.
	TokenMints []string
	// CcipSenderLamports, when non-zero, is the balance the ccip_sender PDA is topped up to. Native
	// SOL fees are paid straight out of that PDA.
	CcipSenderLamports uint64
}

// DeployLegacyTestSender deploys, initializes and enables destination chains on the legacy (v1)
// ExampleSender, which sends through the router's ccip_send, then provisions its ccip_sender PDA
// the same way as the v2 sequence. The legacy ccip_send only reaches the v1 onramp, so the sender
// can only send on lanes still running 1.6 code. It is registered in the datastore as TestSender
// at version 1.0.0, which is what distinguishes it from the v2 entry.
var DeployLegacyTestSender = cldf_ops.NewSequence(
	"deploy-legacy-test-sender",
	testsenderops.LegacyVersion,
	"Deploys, initializes and configures a legacy TestSender on a Solana chain",
	func(b cldf_ops.Bundle, chains cldf_chain.BlockChains, input DeployLegacyTestSenderInput) (seq_core.OnChainOutput, error) {
		solChain, ok := chains.SolanaChains()[input.ChainSelector]
		if !ok {
			return seq_core.OnChainOutput{}, fmt.Errorf("solana chain with selector %d not found", input.ChainSelector)
		}
		if input.SenderQualifier == "" {
			return seq_core.OnChainOutput{}, errors.New("sender qualifier is required")
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

		mints, err := resolveTestSenderMints(b, solChain, input.TokenMints)
		if err != nil {
			return seq_core.OnChainOutput{}, err
		}

		deployReport, err := cldf_ops.ExecuteOperation(b, testsenderops.DeployLegacy, solChain, testsenderops.DeployParams{
			ExistingAddresses: input.ExistingAddresses,
			Qualifier:         input.SenderQualifier,
		})
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("deploy legacy sender: %w", err)
		}
		sender, err := solana.PublicKeyFromBase58(deployReport.Output.Address)
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("parse legacy sender address %q: %w", deployReport.Output.Address, err)
		}

		if _, err = cldf_ops.ExecuteOperation(b, testsenderops.Initialize, solChain, testsenderops.Params{
			Router:        router,
			Sender:        sender,
			SenderVersion: testsenderops.LegacyVersion,
		}); err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("initialize legacy sender: %w", err)
		}
		for _, dest := range input.DestChains {
			if _, err = cldf_ops.ExecuteOperation(b, testsenderops.SetChainConfig, solChain, testsenderops.SetChainConfigParams{
				Sender:            sender,
				DestChainSelector: dest.DestChainSelector,
				Recipient:         dest.Recipient,
				ExtraArgs:         dest.ExtraArgs,
				SenderVersion:     testsenderops.LegacyVersion,
			}); err != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("set legacy sender chain config for dest chain %d: %w", dest.DestChainSelector, err)
			}
		}
		if err = provisionTestSender(b, solChain, sender, testsenderops.LegacyVersion, mints, input.CcipSenderLamports); err != nil {
			return seq_core.OnChainOutput{}, err
		}

		return seq_core.OnChainOutput{Addresses: []datastore.AddressRef{deployReport.Output}}, nil
	},
)
