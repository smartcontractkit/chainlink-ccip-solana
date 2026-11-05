package sequences

import (
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/Masterminds/semver/v3"
	"github.com/gagliardetto/solana-go"

	seq_core "github.com/smartcontractkit/chainlink-ccip/deployment/utils/sequences"
	cldf_chain "github.com/smartcontractkit/chainlink-deployments-framework/chain"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	cldf_ops "github.com/smartcontractkit/chainlink-deployments-framework/operations"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/ccip_common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/ccip"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	cvops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/committeeverifier"
	routerops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/router"
	testsenderops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/testsender"
)

// V2TestSenderDestChain is a destination chain to enable on the v2 sender. The sender attaches
// Recipient and a GenericExtraArgsV3 built from the remaining fields to every message for it.
type V2TestSenderDestChain struct {
	DestChainSelector uint64 `json:"destChainSelector"`
	// Recipient is the hex receiver address on the destination chain. See
	// testsenderops.SetChainConfigParams.Recipient.
	Recipient string `json:"recipient"`
	// GasLimit is the execution gas limit on the destination chain.
	GasLimit uint32 `json:"gasLimit"`
	// Finality is the source-chain finality the message waits for. The zero value waits for
	// full finality.
	Finality ccip_common.FinalityConfig `json:"finality"`
	// CCVQualifiers are the qualifiers of the CommitteeVerifiers on this (source) chain that must
	// verify the message, resolved from the datastore. Empty uses the lane's default CCVs.
	CCVQualifiers []string `json:"ccvQualifiers"`
}

// V2TestSenderExtraArgs returns the hex GenericExtraArgsV3 blob the v2 sender stores for a
// destination chain. The executor is left zero, which the router resolves to the lane default, and
// an empty ccvs list likewise resolves to the lane's default CCVs.
func V2TestSenderExtraArgs(gasLimit uint32, finality ccip_common.FinalityConfig, ccvs []solana.PublicKey) (string, error) {
	args, err := ccip.SerializeExtraArgs(ccip_common.GenericExtraArgsV3{
		GasLimit: ccip_common.CrossChainGas{V0: gasLimit},
		Finality: finality,
		Ccvs:     ccvs,
		CcvArgs:  make([][]byte, len(ccvs)),
	}, ccip.GenericExtraArgsV3Tag)
	if err != nil {
		return "", fmt.Errorf("serialize extra args: %w", err)
	}
	return hex.EncodeToString(args), nil
}

type DeployV2TestSenderInput struct {
	ChainSelector     uint64
	ExistingAddresses []datastore.AddressRef
	SenderQualifier   string
	DestChains        []V2TestSenderDestChain
	// TokenMints are the base58 mints to create ccip_sender PDA token accounts for. Every fee
	// token (wrapped SOL for native fees included) and transferred token needs one before the
	// first send that uses it.
	TokenMints []string
	// CcipSenderLamports, when non-zero, is the balance the ccip_sender PDA is topped up to.
	CcipSenderLamports uint64
}

// DeployV2TestSender deploys and initializes the v2 ExampleSender, enables its destination
// chains, and provisions the ccip_sender PDA: the token accounts its sends pay and transfer with,
// and optionally the lamports native fees are paid from.
var DeployV2TestSender = cldf_ops.NewSequence(
	"deploy-v2-test-sender",
	testsenderops.Version,
	"Deploys, initializes, and configures a V2 TestSender on a Solana chain",
	func(b cldf_ops.Bundle, chains cldf_chain.BlockChains, input DeployV2TestSenderInput) (seq_core.OnChainOutput, error) {
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
		// Resolve everything up front so a bad qualifier or mint fails before anything is deployed.
		extraArgs := make([]string, len(input.DestChains))
		for i, dest := range input.DestChains {
			ccvs, ccvErr := cvops.ResolveCommitteeVerifiers(input.ExistingAddresses, input.ChainSelector, dest.CCVQualifiers)
			if ccvErr != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("resolve CCVs for dest chain %d: %w", dest.DestChainSelector, ccvErr)
			}
			args, argsErr := V2TestSenderExtraArgs(dest.GasLimit, dest.Finality, ccvs)
			if argsErr != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("dest chain %d: %w", dest.DestChainSelector, argsErr)
			}
			extraArgs[i] = args
		}
		mints, err := resolveTestSenderMints(b, solChain, input.TokenMints)
		if err != nil {
			return seq_core.OnChainOutput{}, err
		}

		deployReport, err := cldf_ops.ExecuteOperation(b, testsenderops.Deploy, solChain, testsenderops.DeployParams{
			ExistingAddresses: input.ExistingAddresses,
			Qualifier:         input.SenderQualifier,
		})
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("deploy sender: %w", err)
		}
		sender, err := solana.PublicKeyFromBase58(deployReport.Output.Address)
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("parse sender address %q: %w", deployReport.Output.Address, err)
		}

		if _, err = cldf_ops.ExecuteOperation(b, testsenderops.Initialize, solChain, testsenderops.Params{
			Router:        router,
			Sender:        sender,
			SenderVersion: testsenderops.Version,
		}); err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("initialize sender: %w", err)
		}
		for i, dest := range input.DestChains {
			if _, err = cldf_ops.ExecuteOperation(b, testsenderops.SetChainConfig, solChain, testsenderops.SetChainConfigParams{
				Sender:            sender,
				DestChainSelector: dest.DestChainSelector,
				Recipient:         dest.Recipient,
				ExtraArgs:         extraArgs[i],
				SenderVersion:     testsenderops.Version,
			}); err != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("set sender chain config for dest chain %d: %w", dest.DestChainSelector, err)
			}
		}
		if err = provisionTestSender(b, solChain, sender, testsenderops.Version, mints, input.CcipSenderLamports); err != nil {
			return seq_core.OnChainOutput{}, err
		}

		return seq_core.OnChainOutput{Addresses: []datastore.AddressRef{deployReport.Output}}, nil
	},
)

// resolveTestSenderMints parses the mints a sender's ccip_sender PDA needs token accounts for and
// checks each is an existing token mint, so a bad mint fails before anything is deployed.
func resolveTestSenderMints(b cldf_ops.Bundle, chain cldf_solana.Chain, tokenMints []string) ([]solana.PublicKey, error) {
	mints := make([]solana.PublicKey, len(tokenMints))
	for i, mint := range tokenMints {
		pk, err := solana.PublicKeyFromBase58(mint)
		if err != nil {
			return nil, fmt.Errorf("parse token mint %q: %w", mint, err)
		}
		if _, err = testsenderops.TokenProgramForMint(b.GetContext(), chain, pk); err != nil {
			return nil, err
		}
		mints[i] = pk
	}
	return mints, nil
}

// provisionTestSender creates the sender's ccip_sender PDA token accounts for mints and, when
// lamports is non-zero, tops the PDA up to that balance.
func provisionTestSender(
	b cldf_ops.Bundle,
	chain cldf_solana.Chain,
	sender solana.PublicKey,
	version *semver.Version,
	mints []solana.PublicKey,
	lamports uint64,
) error {
	for _, mint := range mints {
		if _, err := cldf_ops.ExecuteOperation(b, testsenderops.InitCcipSenderATA, chain, testsenderops.InitCcipSenderATAParams{
			Sender:        sender,
			Mint:          mint,
			SenderVersion: version,
		}); err != nil {
			return fmt.Errorf("init ccip_sender ATA for mint %s: %w", mint, err)
		}
	}
	if lamports == 0 {
		return nil
	}
	if _, err := cldf_ops.ExecuteOperation(b, testsenderops.FundCcipSender, chain, testsenderops.FundCcipSenderParams{
		Sender:   sender,
		Lamports: lamports,
	}); err != nil {
		return fmt.Errorf("fund ccip_sender PDA: %w", err)
	}
	return nil
}
