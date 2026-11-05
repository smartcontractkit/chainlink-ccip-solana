package adapters

import (
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"

	mcms_solana "github.com/smartcontractkit/mcms/sdk/solana"
)

func TestSolanaAddressNormalizer(t *testing.T) {
	n := SolanaAddressNormalizer{}
	key := solana.NewWallet().PublicKey()

	got, err := n.NormalizeAddress(key.String())
	require.NoError(t, err)
	require.Equal(t, key.String(), got)

	mcmsAddr := mcms_solana.ContractAddress(key, mcms_solana.PDASeed([32]byte{'s', 'e', 'e', 'd'}))
	got, err = n.NormalizeAddress(mcmsAddr)
	require.NoError(t, err)
	require.Equal(t, mcmsAddr, got, "programID.seed addresses keep their form")

	_, err = n.NormalizeAddress("not-base58-0OIl")
	require.Error(t, err)

	raw, err := n.StringToBytes(key.String())
	require.NoError(t, err)
	str, err := n.BytesToString(raw)
	require.NoError(t, err)
	require.Equal(t, key.String(), str)

	_, err = n.BytesToString([]byte{1, 2, 3})
	require.Error(t, err)
}
