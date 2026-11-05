package tokens

import (
	"errors"
	"fmt"

	"github.com/gagliardetto/solana-go"

	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/sequences"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations"

	soltokens "github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/tokens"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
)

// MintTokensParams mints new SPL supply to a recipient ATA.
type MintTokensParams struct {
	TokenMint solana.PublicKey
	Recipient solana.PublicKey
	Amount    uint64
}

// TEST/controlled environment only — operational tooling, not a protocol
// contract. Mints SPL supply when the CLD deployer is the on-chain mint
// authority, which is the LockRelease case (authority stays with the deployer;
// the finite preMint gets consumed by provide-liquidity, so on-demand minting
// is how test senders get funded). Solana BurnMint pools move the authority to
// the pool signer at pool setup, so this op fails loudly for them — their
// supply can only be created by a CCIP destination release.
var MintTokens = operations.NewOperation(
	"tokens:mint",
	Version,
	"TEST/controlled environment only: mint SPL supply to a recipient ATA when the deployer is the mint authority",
	func(b operations.Bundle, chain cldf_solana.Chain, input MintTokensParams) (sequences.OnChainOutput, error) {
		deployer := chain.DeployerKey.PublicKey()
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

		authority, err := solcommon.GetTokenMintAuthority(chain, input.TokenMint)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("get mint authority: %w", err)
		}
		if authority.IsZero() || authority != deployer {
			return sequences.OnChainOutput{}, fmt.Errorf(
				"mint authority is %s, not the deployer %s; this TEST op only mints deployer-authority tokens (LnR). BurnMint authority moved to the pool signer — create its supply via a CCIP destination release",
				authority.String(), deployer.String())
		}

		ata, _, err := soltokens.FindAssociatedTokenAddress(tokenProgram, input.TokenMint, input.Recipient)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("derive recipient token account: %w", err)
		}
		if err = ensureTokenAccount(chain, tokenProgram, input.TokenMint, input.Recipient); err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("ensure recipient token account: %w", err)
		}
		ixn, err := soltokens.MintTo(input.Amount, tokenProgram, input.TokenMint, ata, deployer)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("build mint instruction: %w", err)
		}
		if err := chain.Confirm([]solana.Instruction{ixn}); err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("confirm mint: %w", err)
		}
		b.Logger.Infow("tokens minted", "mint", input.TokenMint.String(), "recipient", input.Recipient.String(), "amount", input.Amount)
		return sequences.OnChainOutput{}, nil
	},
)
