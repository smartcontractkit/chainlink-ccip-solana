package adapters

import (
	"strings"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
)

func TestBase58To0xHex(t *testing.T) {
	t.Parallel()

	var b [32]byte
	for i := range b {
		b[i] = 0x22
	}
	pk := solana.PublicKeyFromBytes(b[:])
	got, err := solcommon.Base58To0xHex(pk.String())
	require.NoError(t, err)
	require.Equal(t, "0x"+strings.Repeat("22", 32), got)

	_, err = solcommon.Base58To0xHex("not-base58!!!")
	require.Error(t, err)
}
