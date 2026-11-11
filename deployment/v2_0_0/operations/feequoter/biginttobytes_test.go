package feequoter

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBigIntTo28Bytes(t *testing.T) {
	t.Run("encodes big-endian", func(t *testing.T) {
		got, err := bigIntTo28Bytes(big.NewInt(0x0102))
		require.NoError(t, err)
		require.Equal(t, uint8(1), got[26])
		require.Equal(t, uint8(2), got[27])
	})

	t.Run("accepts the maximum 224-bit value", func(t *testing.T) {
		max := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 224), big.NewInt(1))
		_, err := bigIntTo28Bytes(max)
		require.NoError(t, err)
	})

	t.Run("returns errors instead of panicking", func(t *testing.T) {
		for name, v := range map[string]*big.Int{
			"nil":       nil,
			"negative":  big.NewInt(-1),
			"oversized": new(big.Int).Lsh(big.NewInt(1), 224),
		} {
			require.NotPanics(t, func() {
				_, err := bigIntTo28Bytes(v)
				require.Error(t, err, name)
			}, name)
		}
	})
}
