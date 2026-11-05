package transmitter

import (
	"context"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/keystore"
)

func newTestTransmitterKeystore(t *testing.T) keystore.Keystore {
	t.Helper()
	ctx := context.Background()
	ks, err := keystore.LoadKeystore(ctx, keystore.NewMemoryStorage(), "test-password", keystore.WithScryptParams(keystore.FastScryptParams))
	require.NoError(t, err)
	_, err = ks.CreateKeys(ctx, keystore.CreateKeysRequest{
		Keys: []keystore.CreateKeyRequest{{
			KeyName: DefaultKeyName,
			KeyType: keystore.Ed25519,
		}},
	})
	require.NoError(t, err)
	return ks
}

func TestLoadTransmitterKey(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ks := newTestTransmitterKeystore(t)

	pubkey, coreKs, err := loadTransmitterKey(ctx, ks, DefaultKeyName)
	require.NoError(t, err)
	require.False(t, pubkey.IsZero())

	accounts, err := coreKs.Accounts(ctx)
	require.NoError(t, err)
	require.Len(t, accounts, 1)
	require.Equal(t, pubkey.String(), accounts[0])

	data := []byte("test message to sign")
	coreSig, err := coreKs.Sign(ctx, pubkey.String(), data)
	require.NoError(t, err)
	require.True(t, pubkey.Verify(data, solana.SignatureFromBytes(coreSig)))
}

func TestLoadTransmitterKey_unknownKeyName(t *testing.T) {
	t.Parallel()
	_, _, err := loadTransmitterKey(context.Background(), newTestTransmitterKeystore(t), "missing/key")
	require.Error(t, err)
}
