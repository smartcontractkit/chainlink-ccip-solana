package devenv

import (
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	devenvcommon "github.com/smartcontractkit/chainlink-ccv/build/devenv/common"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"

	testreceiverops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/testreceiver"
)

func TestAddressResolver_GetContractReceiver(t *testing.T) {
	const chainSelector = uint64(1)
	v1 := solana.NewWallet().PublicKey()
	v2 := solana.NewWallet().PublicKey()

	ds := datastore.NewMemoryDataStore()
	require.NoError(t, ds.AddressRefStore.Add(datastore.AddressRef{
		Address:       v1.String(),
		Type:          datastore.ContractType(testreceiverops.ContractType),
		Version:       testreceiverops.LegacyVersion,
		Qualifier:     devenvcommon.DefaultReceiverQualifier,
		ChainSelector: chainSelector,
	}))
	require.NoError(t, ds.AddressRefStore.Add(datastore.AddressRef{
		Address:       v2.String(),
		Type:          datastore.ContractType(testreceiverops.ContractType),
		Version:       testreceiverops.Version,
		Qualifier:     devenvcommon.DefaultReceiverQualifier,
		ChainSelector: chainSelector,
	}))

	resolver := NewAddressResolver()
	gotV2, err := resolver.GetContractReceiver(ds.Seal(), chainSelector, devenvcommon.DefaultReceiverQualifier)
	require.NoError(t, err)
	assert.Equal(t, v2.Bytes(), []byte(gotV2), "default qualifier selects the v2 receiver")

	gotV1, err := resolver.GetContractReceiver(ds.Seal(), chainSelector, LegacyReceiverQualifier)
	require.NoError(t, err)
	assert.Equal(t, v1.Bytes(), []byte(gotV1), "legacy qualifier selects the v1 receiver")
}
