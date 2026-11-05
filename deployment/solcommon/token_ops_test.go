package solcommon

import (
	"bytes"
	"testing"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/token"
	"github.com/stretchr/testify/require"
)

func TestIsMintAccountData(t *testing.T) {
	encode := func(v any) []byte {
		var buf bytes.Buffer
		require.NoError(t, bin.NewBinEncoder(&buf).Encode(v))
		return buf.Bytes()
	}
	authority := solana.NewWallet().PublicKey()
	mint := encode(token.Mint{MintAuthority: &authority, Supply: 1_000, Decimals: 9, IsInitialized: true})
	require.Len(t, mint, mintAccountLen)

	// Token-2022 mint with extensions: base mint, zero padding to 165 bytes, AccountType::Mint, TLV data
	extended := append(append(append([]byte{}, mint...), make([]byte, tokenAccountLen-mintAccountLen)...), token2022MintAccountType, 3, 0, 2, 0, 7, 7)

	account := encode(token.Account{Mint: solana.NewWallet().PublicKey(), Owner: authority, Amount: 5, State: token.Initialized})
	require.Len(t, account, tokenAccountLen)

	var signers [11]solana.PublicKey
	for i := range signers {
		signers[i] = solana.NewWallet().PublicKey()
	}
	multisig := encode(token.Multisig{M: 2, N: 11, IsInitialized: true, Signers: signers})

	uninitialized := encode(token.Mint{Decimals: 9})

	require.True(t, IsMintAccountData(mint), "plain mint")
	require.True(t, IsMintAccountData(extended), "token-2022 mint with extensions")
	require.False(t, IsMintAccountData(account), "token account")
	require.False(t, IsMintAccountData(multisig), "multisig")
	require.False(t, IsMintAccountData(uninitialized), "uninitialized mint")
	require.False(t, IsMintAccountData(nil), "empty account")
}
