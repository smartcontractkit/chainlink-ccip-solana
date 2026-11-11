package solcommon

import (
	"context"
	"fmt"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/token"
	"github.com/gagliardetto/solana-go/rpc"

	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	cldf_deployment "github.com/smartcontractkit/chainlink-deployments-framework/deployment"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	soltokens "github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/tokens"
)

// GetTokenProgramID returns the program ID for the given token program name
func GetTokenProgramID(programName cldf_deployment.ContractType) (solana.PublicKey, error) {
	tokenPrograms := map[cldf_deployment.ContractType]solana.PublicKey{
		SPLTokens:     solana.TokenProgramID,
		SPL2022Tokens: solana.Token2022ProgramID,
	}

	programID, ok := tokenPrograms[programName]
	if !ok {
		return solana.PublicKey{}, fmt.Errorf("invalid token program: %s. Must be one of: %s, %s", programName, SPLTokens, SPL2022Tokens)
	}
	return programID, nil
}

// GetTokenProgramType is the inverse of GetTokenProgramID: it returns the datastore contract type
// (SPLTokens or SPL2022Tokens) for the given token program ID.
func GetTokenProgramType(programID solana.PublicKey) (cldf_deployment.ContractType, error) {
	switch programID {
	case solana.TokenProgramID:
		return SPLTokens, nil
	case solana.Token2022ProgramID:
		return SPL2022Tokens, nil
	default:
		return "", fmt.Errorf("invalid token program ID: %s. Must be one of: %s, %s", programID, solana.TokenProgramID, solana.Token2022ProgramID)
	}
}

// FetchTokenProgramID reads a mint's owning token program from chain, checking that the account
// is a mint owned by a supported token program.
func FetchTokenProgramID(ctx context.Context, chain cldf_solana.Chain, tokenMint solana.PublicKey) (solana.PublicKey, error) {
	acct, err := chain.Client.GetAccountInfoWithOpts(ctx, tokenMint, &rpc.GetAccountInfoOpts{Commitment: cldf_solana.SolDefaultCommitment})
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("failed to get account info for token %s: %w", tokenMint, err)
	}
	if acct == nil || acct.Value == nil {
		return solana.PublicKey{}, fmt.Errorf("token mint %s not found", tokenMint)
	}
	if _, err = GetTokenProgramType(acct.Value.Owner); err != nil {
		return solana.PublicKey{}, fmt.Errorf("token %s is not owned by a supported token program: %w", tokenMint, err)
	}
	// Token accounts and multisigs are owned by the same programs, so check the layout too.
	if !IsMintAccountData(acct.Value.Data.GetBinary()) {
		return solana.PublicKey{}, fmt.Errorf("account %s is not an initialized token mint", tokenMint)
	}
	return acct.Value.Owner, nil
}

// SPL Token account layouts, shared by Token-2022 for the base state.
const (
	mintAccountLen             = 82  // spl_token::state::Mint::LEN
	tokenAccountLen            = 165 // spl_token::state::Account::LEN
	mintIsInitializedOffset    = 45  // after mint_authority (36), supply (8) and decimals (1)
	token2022MintAccountType   = 1   // spl_token_2022::extension::AccountType::Mint
	token2022AccountTypeOffset = tokenAccountLen
)

// IsMintAccountData reports whether data is an initialized SPL Token or Token-2022 mint. A token
// account (165 bytes) or multisig (355 bytes) is owned by the same program, and decoding it as a
// mint does not fail because the decoder ignores trailing bytes. A plain mint is exactly 82 bytes.
// A Token-2022 mint with extensions pads the base state with zeros up to 165 bytes, followed by
// the AccountType::Mint byte.
func IsMintAccountData(data []byte) bool {
	switch {
	case len(data) == mintAccountLen:
	case len(data) > token2022AccountTypeOffset && data[token2022AccountTypeOffset] == token2022MintAccountType:
		for _, b := range data[mintAccountLen:tokenAccountLen] {
			if b != 0 {
				return false
			}
		}
	default:
		return false
	}
	return data[mintIsInitializedOffset] == 1
}

func MintTokens(chain cldf_solana.Chain, tokenProgramID, mint solana.PublicKey, amountToAddress map[string]uint64) error {
	for toAddress, amount := range amountToAddress {
		toAddressBase58 := solana.MustPublicKeyFromBase58(toAddress)
		// get associated token account for toAddress
		ata, _, _ := soltokens.FindAssociatedTokenAddress(tokenProgramID, mint, toAddressBase58)
		mintToI, err := soltokens.MintTo(amount, tokenProgramID, mint, ata, chain.DeployerKey.PublicKey())
		if err != nil {
			return err
		}
		if err := chain.Confirm([]solana.Instruction{mintToI}); err != nil {
			return err
		}
	}
	return nil
}

func DisableFreezeAuthority(ctx context.Context, chain cldf_solana.Chain, tokenMints []solana.PublicKey) error {
	for _, mint := range tokenMints {
		ix, err := token.NewSetAuthorityInstructionBuilder().
			SetAuthorityType(token.AuthorityFreezeAccount).
			SetSubjectAccount(mint).
			SetAuthorityAccount(chain.DeployerKey.PublicKey()).
			ValidateAndBuild()
		if err != nil {
			return fmt.Errorf("failed to build set authority instruction for mint %s: %w", mint.String(), err)
		}
		if err = chain.Confirm([]solana.Instruction{
			&soltokens.TokenInstruction{Instruction: ix, Program: solana.TokenProgramID},
		}); err != nil {
			return fmt.Errorf("failed to disable freeze authority for mint %s: %w", mint.String(), err)
		}
	}
	return nil
}

func GetTokenMintAuthority(chain cldf_solana.Chain, tokenMint solana.PublicKey) (solana.PublicKey, error) {
	var mintData token.Mint
	// Token accounts are not Anchor accounts, so we can use the regular CLDF helper here without worrying about the 8-byte discriminator.
	err := common.GetAccountDataBorshInto(context.Background(), chain.Client, tokenMint, cldf_solana.SolDefaultCommitment, &mintData)
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("failed to decode token mint %s: %w", tokenMint, err)
	}
	var mintAuthority solana.PublicKey
	if mintData.MintAuthority != nil {
		mintAuthority = *mintData.MintAuthority
	}
	return mintAuthority, nil
}

// GetTokenFreezeAuthority returns the freeze authority of a mint, which is zero once it is disabled.
func GetTokenFreezeAuthority(chain cldf_solana.Chain, tokenMint solana.PublicKey) (solana.PublicKey, error) {
	var mintData token.Mint
	err := common.GetAccountDataBorshInto(context.Background(), chain.Client, tokenMint, cldf_solana.SolDefaultCommitment, &mintData)
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("failed to decode token mint %s: %w", tokenMint, err)
	}
	var freezeAuthority solana.PublicKey
	if mintData.FreezeAuthority != nil {
		freezeAuthority = *mintData.FreezeAuthority
	}
	return freezeAuthority, nil
}

func GetTokenDecimals(chain cldf_solana.Chain, tokenMint solana.PublicKey) (uint8, error) {
	var mintData token.Mint
	// Token accounts are not Anchor accounts, so we can use the regular CLDF helper here without worrying about the 8-byte discriminator.
	err := common.GetAccountDataBorshInto(context.Background(), chain.Client, tokenMint, cldf_solana.SolDefaultCommitment, &mintData)
	if err != nil {
		return 0, fmt.Errorf("failed to decode token mint %s: %w", tokenMint, err)
	}
	return mintData.Decimals, nil
}
