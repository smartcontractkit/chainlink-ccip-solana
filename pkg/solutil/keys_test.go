package solutil_test

import (
	"crypto/ed25519"
	"encoding/json"
	"testing"

	"github.com/gagliardetto/solana-go"
	mrtronbase58 "github.com/mr-tron/base58"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-ccip-solana/pkg/solutil"
)

func marshalKeypairJSON(b []byte) string {
	nums := make([]int, len(b))
	for i, v := range b {
		nums[i] = int(v)
	}
	raw, err := json.Marshal(nums)
	if err != nil {
		panic(err)
	}
	return string(raw)
}

func TestPrivateKeyFromRaw(t *testing.T) {
	t.Parallel()

	want, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)

	successCases := []struct {
		name   string
		encode func(solana.PrivateKey) string
	}{
		{
			name: "keypair base58",
			encode: func(pk solana.PrivateKey) string {
				return pk.String()
			},
		},
		{
			name: "seed base58",
			encode: func(pk solana.PrivateKey) string {
				return mrtronbase58.Encode([]byte(pk)[:ed25519.SeedSize])
			},
		},
		{
			name: "keypair json",
			encode: func(pk solana.PrivateKey) string {
				return marshalKeypairJSON(pk)
			},
		},
		{
			name: "seed json",
			encode: func(pk solana.PrivateKey) string {
				return marshalKeypairJSON([]byte(pk)[:ed25519.SeedSize])
			},
		},
	}

	for _, tc := range successCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := solutil.PrivateKeyFromRaw(tc.encode(want))
			require.NoError(t, err)
			require.Equal(t, want, got)
		})
	}

	errorCases := []struct {
		name    string
		raw     string
		wantErr string
	}{
		{
			name:    "empty",
			raw:     "  ",
			wantErr: "empty private key",
		},
		{
			name:    "invalid length",
			raw:     "[1,2,3]",
			wantErr: "invalid key length",
		},
	}

	for _, tc := range errorCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := solutil.PrivateKeyFromRaw(tc.raw)
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.wantErr)
		})
	}
}
