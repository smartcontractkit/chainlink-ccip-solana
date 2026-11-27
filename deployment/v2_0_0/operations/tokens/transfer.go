package tokens

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/gagliardetto/solana-go"

	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/sequences"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations"

	soltokens "github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/tokens"
)

// TransferTokensParams transfers existing SPL supply held by the deployer's
// associated token account to a recipient's ATA. It never mints new supply.
type TransferTokensParams struct {
	TokenMint solana.PublicKey
	Recipient solana.PublicKey
	Amount    uint64
}

// TEST/controlled environment only — operational test tooling, not a protocol
// contract op. It funds test senders (e.g. Sol -> EVM LnR token tests) on
// demand by moving supply the deployer already holds. Minting new supply is a
// separate concern: Solana BurnMint pools move the mint authority to the pool
// signer at pool setup, so only a CCIP destination release can create their
// SPL supply; the transfer op fails loudly if the deployer holds nothing.
var TransferTokens = operations.NewOperation(
	"tokens:transfer",
	Version,
	"TEST/controlled environment only: transfer SPL tokens from the deployer's ATA to a recipient",
	func(b operations.Bundle, chain cldf_solana.Chain, input TransferTokensParams) (sequences.OnChainOutput, error) {
		deployer := chain.Deployer().PublicKey()
		if input.Recipient == deployer {
			return sequences.OnChainOutput{}, errors.New("recipient is the deployer; nothing to transfer")
		}
		if input.Amount == 0 {
			return sequences.OnChainOutput{}, errors.New("amount must be non-zero")
		}

		mintAcct, err := chain.Client.GetAccountInfo(b.GetContext(), input.TokenMint)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("get mint account %s: %w", input.TokenMint.String(), err)
		}
		if mintAcct == nil || mintAcct.Value == nil {
			return sequences.OnChainOutput{}, fmt.Errorf("mint account %s does not exist", input.TokenMint.String())
		}
		tokenProgram := mintAcct.Value.Owner

		deployerATA, _, err := soltokens.FindAssociatedTokenAddress(tokenProgram, input.TokenMint, deployer)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("derive deployer token account: %w", err)
		}
		if err = ensureTokenAccount(chain, tokenProgram, input.TokenMint, deployer); err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("ensure deployer token account: %w", err)
		}
		balance, err := chain.Client.GetTokenAccountBalance(b.GetContext(), deployerATA, cldf_solana.SolDefaultCommitment)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("get deployer token balance: %w", err)
		}
		current, err := strconv.ParseUint(balance.Value.Amount, 10, 64)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("parse deployer balance %q: %w", balance.Value.Amount, err)
		}
		if current < input.Amount {
			return sequences.OnChainOutput{}, fmt.Errorf("deployer balance %d is less than transfer amount %d", current, input.Amount)
		}

		recipientATA, _, err := soltokens.FindAssociatedTokenAddress(tokenProgram, input.TokenMint, input.Recipient)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("derive recipient token account: %w", err)
		}
		if err = ensureTokenAccount(chain, tokenProgram, input.TokenMint, input.Recipient); err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("ensure recipient token account: %w", err)
		}

		ixn, err := soltokens.TokenTransferChecked(input.Amount, balance.Value.Decimals, tokenProgram, deployerATA, input.TokenMint, recipientATA, deployer, nil)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("build transfer instruction: %w", err)
		}
		if err := chain.Confirm([]solana.Instruction{ixn}); err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("confirm transfer: %w", err)
		}
		b.Logger.Infow("tokens transferred", "mint", input.TokenMint.String(), "recipient", input.Recipient.String(), "amount", input.Amount)
		return sequences.OnChainOutput{}, nil
	},
)

func ensureTokenAccount(chain cldf_solana.Chain, tokenProgram, mint, owner solana.PublicKey) error {
	ixn, _, err := soltokens.CreateAssociatedTokenAccount(tokenProgram, mint, owner, chain.Deployer().PublicKey())
	if err != nil {
		return err
	}
	return chain.Confirm([]solana.Instruction{ixn})
}
