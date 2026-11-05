package changesets

import (
	"errors"
	"fmt"

	"github.com/gagliardetto/solana-go"

	cldf "github.com/smartcontractkit/chainlink-deployments-framework/deployment"
	cldf_ops "github.com/smartcontractkit/chainlink-deployments-framework/operations"

	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/sequences"
)

type TransferTokensChangesetInput struct {
	ChainSelector uint64 `json:"chainSelector"`
	TokenMint     string `json:"tokenMint"`
	Recipient     string `json:"recipient"`
	Amount        uint64 `json:"amount"`
}

// TEST/controlled environment only — on-demand token funding tooling, not a
// protocol contract. Moves existing SPL supply from the deployer's ATA to a
// test sender; it never mints new supply (see operations/tokens.TransferTokens).
func TransferTokensChangeset() cldf.ChangeSetV2[TransferTokensChangesetInput] {
	return cldf.CreateChangeSet(transferTokensApply, transferTokensVerify)
}

func transferTokensVerify(env cldf.Environment, input TransferTokensChangesetInput) error {
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

func transferTokensApply(e cldf.Environment, input TransferTokensChangesetInput) (cldf.ChangesetOutput, error) {
	report, err := cldf_ops.ExecuteSequence(e.OperationsBundle, sequences.TransferTokens, e.BlockChains, sequences.TransferTokensInput{
		ChainSelector: input.ChainSelector,
		Mint:          solana.MustPublicKeyFromBase58(input.TokenMint),
		Recipient:     solana.MustPublicKeyFromBase58(input.Recipient),
		Amount:        input.Amount,
	})
	if err != nil {
		return cldf.ChangesetOutput{}, fmt.Errorf("transfer tokens: %w", err)
	}
	return cldf.ChangesetOutput{
		Reports: append([]cldf_ops.Report[any, any](nil), report.ExecutionReports...),
	}, nil
}
