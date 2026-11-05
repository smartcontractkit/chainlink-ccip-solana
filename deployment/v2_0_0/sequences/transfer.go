package sequences

import (
	"fmt"

	"github.com/gagliardetto/solana-go"

	seq_core "github.com/smartcontractkit/chainlink-ccip/deployment/utils/sequences"
	cldf_chain "github.com/smartcontractkit/chainlink-deployments-framework/chain"
	cldf_ops "github.com/smartcontractkit/chainlink-deployments-framework/operations"

	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	tokensops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/tokens"
)

// TransferTokensInput binds the transfer op to a Solana chain.
type TransferTokensInput struct {
	ChainSelector uint64
	Mint          solana.PublicKey
	Recipient     solana.PublicKey
	Amount        uint64
}

// TEST/controlled environment only — see operations/tokens.TransferTokens.
var TransferTokens = cldf_ops.NewSequence(
	"tokens:transfer",
	solcommon.Version2_0_0,
	"TEST/controlled environment only: transfer SPL tokens from the deployer's ATA to a recipient",
	func(b cldf_ops.Bundle, chains cldf_chain.BlockChains, input TransferTokensInput) (seq_core.OnChainOutput, error) {
		chain, ok := chains.SolanaChains()[input.ChainSelector]
		if !ok {
			return seq_core.OnChainOutput{}, fmt.Errorf("solana chain with selector %d not found", input.ChainSelector)
		}
		out, err := cldf_ops.ExecuteOperation(b, tokensops.TransferTokens, chain, tokensops.TransferTokensParams{
			TokenMint: input.Mint,
			Recipient: input.Recipient,
			Amount:    input.Amount,
		})
		if err != nil {
			return seq_core.OnChainOutput{}, err
		}
		return out.Output, nil
	},
)

// MintTokensInput binds the mint op to a Solana chain.
type MintTokensInput struct {
	ChainSelector uint64
	Mint          solana.PublicKey
	Recipient     solana.PublicKey
	Amount        uint64
}

// TEST/controlled environment only — see operations/tokens.MintTokens.
var MintTokens = cldf_ops.NewSequence(
	"tokens:mint",
	solcommon.Version2_0_0,
	"TEST/controlled environment only: mint SPL supply to a recipient ATA when the deployer is the mint authority",
	func(b cldf_ops.Bundle, chains cldf_chain.BlockChains, input MintTokensInput) (seq_core.OnChainOutput, error) {
		chain, ok := chains.SolanaChains()[input.ChainSelector]
		if !ok {
			return seq_core.OnChainOutput{}, fmt.Errorf("solana chain with selector %d not found", input.ChainSelector)
		}
		out, err := cldf_ops.ExecuteOperation(b, tokensops.MintTokens, chain, tokensops.MintTokensParams{
			TokenMint: input.Mint,
			Recipient: input.Recipient,
			Amount:    input.Amount,
		})
		if err != nil {
			return seq_core.OnChainOutput{}, err
		}
		return out.Output, nil
	},
)
