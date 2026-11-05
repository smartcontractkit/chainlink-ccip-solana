package changesets

import (
	"errors"
	"fmt"

	"github.com/gagliardetto/solana-go"

	cldf "github.com/smartcontractkit/chainlink-deployments-framework/deployment"
	cldf_ops "github.com/smartcontractkit/chainlink-deployments-framework/operations"

	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/sequences"
)

type MintTokensChangesetInput struct {
	ChainSelector uint64 `json:"chainSelector"`
	TokenMint     string `json:"tokenMint"`
	Recipient     string `json:"recipient"`
	Amount        uint64 `json:"amount"`
}

// TEST/controlled environment only — on-demand token funding tooling, not a
// protocol contract. Mints fresh SPL supply when the deployer is the on-chain
// mint authority (LnR); fails loudly for BurnMint, whose authority is the pool
// signer (see operations/tokens.MintTokens).
func MintTokensChangeset() cldf.ChangeSetV2[MintTokensChangesetInput] {
	return cldf.CreateChangeSet(mintTokensApply, mintTokensVerify)
}

func mintTokensVerify(env cldf.Environment, input MintTokensChangesetInput) error {
	if err := cldf.IsValidChainSelector(input.ChainSelector); err != nil {
		return fmt.Errorf("invalid chain selector %d: %w", input.ChainSelector, err)
	}
	if !env.BlockChains.Exists(input.ChainSelector) {
		return fmt.Errorf("chain with selector %d does not exist", input.ChainSelector)
	}
	if _, ok := env.BlockChains.SolanaChains()[input.ChainSelector]; !ok {
		return fmt.Errorf("chain with selector %d is not a Solana chain", input.ChainSelector)
	}
	if input.Amount == 0 {
		return errors.New("amount must be non-zero")
	}
	if _, err := solana.PublicKeyFromBase58(input.TokenMint); err != nil {
		return fmt.Errorf("invalid tokenMint %q: %w", input.TokenMint, err)
	}
	if _, err := solana.PublicKeyFromBase58(input.Recipient); err != nil {
		return fmt.Errorf("invalid recipient %q: %w", input.Recipient, err)
	}
	return nil
}

func mintTokensApply(e cldf.Environment, input MintTokensChangesetInput) (cldf.ChangesetOutput, error) {
	report, err := cldf_ops.ExecuteSequence(e.OperationsBundle, sequences.MintTokens, e.BlockChains, sequences.MintTokensInput{
		ChainSelector: input.ChainSelector,
		Mint:          solana.MustPublicKeyFromBase58(input.TokenMint),
		Recipient:     solana.MustPublicKeyFromBase58(input.Recipient),
		Amount:        input.Amount,
	})
	if err != nil {
		return cldf.ChangesetOutput{}, fmt.Errorf("mint tokens: %w", err)
	}
	return cldf.ChangesetOutput{
		Reports: append([]cldf_ops.Report[any, any](nil), report.ExecutionReports...),
	}, nil
}
