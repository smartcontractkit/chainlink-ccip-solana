package offramp

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/ccip_offramp"
)

func TestOnRampAddressRoundTrip(t *testing.T) {
	in := [][]byte{make([]byte, 32), {0xaa, 0xbb}}
	in[0][31] = 0x01

	addrs, err := ToOnRampAddresses(in)
	require.NoError(t, err)
	require.Len(t, addrs, 2)
	require.Equal(t, uint32(32), addrs[0].Len)
	require.Equal(t, uint32(2), addrs[1].Len)

	require.Equal(t, in, SourceOnRampBytes(ccip_offramp.SourceChainConfig{OnRamps: addrs}))
}

func TestToOnRampAddressesRejectsInvalidLength(t *testing.T) {
	_, err := ToOnRampAddresses([][]byte{{}})
	require.Error(t, err)
	_, err = ToOnRampAddresses([][]byte{make([]byte, 65)})
	require.Error(t, err)
}
