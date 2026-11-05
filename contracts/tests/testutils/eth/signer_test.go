package eth

import (
	"encoding/hex"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
)

// Secp256k1N used to be a hard-coded literal. It now comes from secp256k1.Params(), so pin the value
// down: signature normalization (S -> N-S) depends on it, and a wrong order would produce signatures
// the verifier silently rejects.
func TestSecp256k1NIsTheCurveOrder(t *testing.T) {
	t.Parallel()

	expected, ok := new(big.Int).SetString("fffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364141", 16)
	require.True(t, ok)
	require.Zero(t, Secp256k1N.Cmp(expected))
}

// GenerateEthPrivateKeys now delegates to secp256k1.GeneratePrivateKey instead of hand-rolled
// rejection sampling plus manual zero-padding, so check the properties that replaced: exactly 32
// bytes each, in range, and distinct.
func TestGenerateEthPrivateKeys(t *testing.T) {
	t.Parallel()

	keys, err := GenerateEthPrivateKeys(8)
	require.NoError(t, err)
	require.Len(t, keys, 8)

	seen := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		require.Len(t, key, 64, "expected a zero-padded 32-byte hex key")

		raw, decodeErr := hex.DecodeString(key)
		require.NoError(t, decodeErr)
		require.Len(t, raw, 32)

		scalar := new(big.Int).SetBytes(raw)
		require.Positive(t, scalar.Sign(), "private key must be non-zero")
		require.Negative(t, scalar.Cmp(Secp256k1N), "private key must be below the curve order")

		_, ok := seen[key]
		require.False(t, ok, "keys must be distinct")
		seen[key] = struct{}{}
	}
}

func TestGetEvmSignersSortsByAddress(t *testing.T) {
	t.Parallel()

	keys, err := GenerateEthPrivateKeys(5)
	require.NoError(t, err)

	signers, err := GetEvmSigners(keys)
	require.NoError(t, err)
	require.Len(t, signers, 5)

	// set_signer_config requires ascending addresses; callers rely on this helper for that ordering.
	for i := 1; i < len(signers); i++ {
		require.LessOrEqual(t, string(signers[i-1].Address[:]), string(signers[i].Address[:]))
	}
}

func TestKeccak256MatchesKnownVector(t *testing.T) {
	t.Parallel()

	// keccak256("") — distinguishes Keccak-256 from SHA3-256, which the legacy hasher choice matters for.
	require.Equal(t,
		"c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a470",
		hex.EncodeToString(Keccak256(nil)),
	)
}
