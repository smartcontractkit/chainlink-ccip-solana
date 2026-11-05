package tokens

import (
	"testing"

	"github.com/gagliardetto/solana-go"
	ata "github.com/gagliardetto/solana-go/programs/associated-token-account"
	"github.com/stretchr/testify/require"
)

func legacyCreateAssociatedTokenAccount(tokenProgram, mint, address, payer solana.PublicKey) (solana.Instruction, solana.PublicKey, error) {
	base := ata.NewCreateInstruction(payer, address, mint)
	ix, err := base.ValidateAndBuild()
	if err != nil {
		return nil, solana.PublicKey{}, err
	}

	associatedTokenAddress, _, err := FindAssociatedTokenAddress(tokenProgram, mint, address)
	if err != nil {
		return nil, solana.PublicKey{}, err
	}

	accounts := ix.Accounts()
	accounts[5].PublicKey = tokenProgram
	accounts[1].PublicKey = associatedTokenAddress

	base.AccountMetaSlice = accounts
	ix.Impl = base
	return ix, associatedTokenAddress, nil
}

func TestCreateAssociatedTokenAccountMatchesLegacy(t *testing.T) {
	mint := solana.NewWallet().PublicKey()
	owner := solana.NewWallet().PublicKey()
	payer := solana.NewWallet().PublicKey()

	for name, tokenProgram := range map[string]solana.PublicKey{
		"token":      solana.TokenProgramID,
		"token-2022": solana.Token2022ProgramID,
	} {
		t.Run(name, func(t *testing.T) {
			legacyIx, legacyAddr, err := legacyCreateAssociatedTokenAccount(tokenProgram, mint, owner, payer)
			require.NoError(t, err)
			ix, addr, err := CreateAssociatedTokenAccount(tokenProgram, mint, owner, payer)
			require.NoError(t, err)

			require.Equal(t, legacyAddr, addr)
			require.Equal(t, legacyIx.ProgramID(), ix.ProgramID())
			require.Equal(t, legacyIx.Accounts(), ix.Accounts())
			require.Len(t, ix.Accounts(), 6)
			require.Equal(t, addr, ix.Accounts()[1].PublicKey)
			require.Equal(t, tokenProgram, ix.Accounts()[5].PublicKey)

			legacyData, err := legacyIx.Data()
			require.NoError(t, err)
			data, err := ix.Data()
			require.NoError(t, err)
			require.Equal(t, []byte{ata.Instruction_Create}, legacyData)
			require.Equal(t, []byte{ata.Instruction_CreateIdempotent}, data)
		})
	}
}
