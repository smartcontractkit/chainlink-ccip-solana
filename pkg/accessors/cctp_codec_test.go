package accessors

import (
	"testing"

	"github.com/stretchr/testify/require"

	chainsel "github.com/smartcontractkit/chain-selectors"
	"github.com/smartcontractkit/chainlink-ccv/protocol"
)

const (
	// Real values from a Solana -> Sepolia burn, tx x76ZETWu...
	solanaTxHashHex = "2f85428658ad2f5d8f6ef86d948bbcafc17c0d5cdadb590a7129d7fdc7a9853d02a5476eb3d1d92eb7366b069973d2058810e18aabb62a93422045c92b3a850d"
	solanaTxHashB58 = "x76ZETWu5K5C4wmemgVqv72hE2FaawNb7EFKtjiD82ugNRvnpbcu12FEP4dzrNacfHTdQTsQuU8ukVLX2qrLxqi"
	// The pool signer PDA, as Circle returns it in messageSender for a Solana burn.
	solanaSenderB58 = "2v3hCkRJGSM4N29xwjA9TePBrC3uM8BnE3r7ns2W9BjR"
	solanaSenderHex = "0x1c7283d45b49957afb150fedd003d8d5cecd20915936bee9a9b9cf085e1bdfb4"
)

func TestSolanaAccessorEncodeTxHash(t *testing.T) {
	raw, err := protocol.NewByteSliceFromHex("0x" + solanaTxHashHex)
	require.NoError(t, err)

	require.Equal(t, solanaTxHashB58, (&accessor{}).EncodeTxHash(raw))
}

func TestSolanaAccessorDecodeAddress(t *testing.T) {
	fromB58, err := (&accessor{}).DecodeAddress(solanaSenderB58)
	require.NoError(t, err)
	fromHex, err := protocol.NewUnknownAddressFromHex(solanaSenderHex)
	require.NoError(t, err)
	require.True(t, fromB58.Equal(fromHex), "base58 sender must decode to the configured verifier address")

	_, err = (&accessor{}).DecodeAddress("0xnot-base58")
	require.Error(t, err)

	// Valid base58 but wrong length: "1" decodes to a single zero byte.
	_, err = (&accessor{}).DecodeAddress("1")
	require.ErrorContains(t, err, "want 32")
}

func TestSolanaAccessorDomain(t *testing.T) {
	cases := map[string]struct {
		selector protocol.ChainSelector
		want     uint32
	}{
		"solana mainnet": {protocol.ChainSelector(chainsel.SOLANA_MAINNET.Selector), 5},
		"solana devnet":  {protocol.ChainSelector(chainsel.SOLANA_DEVNET.Selector), 5},
		"solana testnet": {protocol.ChainSelector(chainsel.SOLANA_TESTNET.Selector), 5},
		"devenv chain 1": {protocol.ChainSelector(chainsel.TEST_22222222222222222222222222222222222222222222.Selector), 5},
		"devenv chain 2": {protocol.ChainSelector(chainsel.TEST_33333333333333333333333333333333333333333333.Selector), 106},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			domain, ok := (&accessor{}).Domain(tc.selector)
			require.True(t, ok)
			require.Equal(t, tc.want, domain)
		})
	}

	_, ok := (&accessor{}).Domain(protocol.ChainSelector(chainsel.GETH_TESTNET.Selector))
	require.False(t, ok, "an EVM selector must not resolve through the Solana accessor")
}
