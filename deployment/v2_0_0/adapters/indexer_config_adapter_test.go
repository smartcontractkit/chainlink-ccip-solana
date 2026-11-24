package adapters

import (
	"strings"
	"testing"

	"github.com/Masterminds/semver/v3"
	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"

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

func TestResolveLombardVerifierAddressesCompatibleVersion(t *testing.T) {
	t.Parallel()

	const (
		chainSelector = uint64(124615329519749607)
		qualifier     = "LombardVerifierResolver"
	)

	address := solana.NewWallet().PublicKey().String()

	ds := datastore.NewMemoryDataStore()
	require.NoError(t, ds.Addresses().Add(datastore.AddressRef{
		ChainSelector: chainSelector,
		Type:          datastore.ContractType("LombardVerifier"),
		Version:       semver.MustParse("2.1.0"),
		Qualifier:     qualifier,
		Address:       address,
	}))

	got, err := resolveLombardVerifierAddresses(ds.Seal(), chainSelector, qualifier)
	require.NoError(t, err)

	expected, err := solcommon.Base58To0xHex(address)
	require.NoError(t, err)
	require.Equal(t, []string{expected}, got)
}
