package tokenpools

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMissingRemotePoolAddresses(t *testing.T) {
	evm := bytes.Repeat([]byte{0xab}, 20)
	other := bytes.Repeat([]byte{0xcd}, 20)
	padded := append(make([]byte, 12), evm...)

	missing, err := MissingRemotePoolAddresses([][]byte{evm, other}, 20)
	require.NoError(t, err)
	require.Empty(t, missing, "already at the width")

	missing, err = MissingRemotePoolAddresses([][]byte{padded, other}, 20)
	require.NoError(t, err)
	require.Equal(t, [][]byte{evm}, missing)

	missing, err = MissingRemotePoolAddresses([][]byte{padded, evm}, 20)
	require.NoError(t, err)
	require.Empty(t, missing, "the padded entry already has its native twin")

	missing, err = MissingRemotePoolAddresses([][]byte{padded, padded}, 20)
	require.NoError(t, err)
	require.Equal(t, [][]byte{evm}, missing, "appended once")

	missing, err = MissingRemotePoolAddresses([][]byte{padded}, 32)
	require.NoError(t, err)
	require.Empty(t, missing, "a 32-byte remote family keeps 32-byte addresses")

	_, err = MissingRemotePoolAddresses([][]byte{append([]byte{1}, evm...)}, 20)
	require.ErrorContains(t, err, "does not fit in 20 bytes")
	_, err = MissingRemotePoolAddresses([][]byte{evm[:19]}, 20)
	require.ErrorContains(t, err, "shorter than 20 bytes")
	_, err = MissingRemotePoolAddresses(nil, 20)
	require.ErrorContains(t, err, "no remote pool addresses")
	_, err = MissingRemotePoolAddresses([][]byte{evm}, 0)
	require.ErrorContains(t, err, "width 0")
}
