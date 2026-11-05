package adapters

import (
	"slices"
	"testing"

	"github.com/Masterminds/semver/v3"
	"github.com/stretchr/testify/require"

	chainsel "github.com/smartcontractkit/chain-selectors"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"

	cvops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/committeeverifier"
)

func TestGetCommitteeVerifierDeployedChains_nilDataStore(t *testing.T) {
	t.Parallel()
	require.Nil(t, getCommitteeVerifierDeployedChains(nil, "q"))
}

func TestGetCommitteeVerifierDeployedChains_filtersAndDedupes(t *testing.T) {
	t.Parallel()
	ds := datastore.NewMemoryDataStore()
	qual := "committee-a"
	solDev := chainsel.SOLANA_DEVNET.Selector
	solTest := chainsel.SOLANA_TESTNET.Selector
	evmMain := chainsel.ETHEREUM_MAINNET.Selector

	ref := func(chain uint64, q string, ver *semver.Version) datastore.AddressRef {
		return datastore.AddressRef{
			Address:       "So11111111111111111111111111111111111111112",
			ChainSelector: chain,
			Qualifier:     q,
			Type:          datastore.ContractType(cvops.ContractType),
			Version:       ver,
		}
	}
	require.NoError(t, ds.Addresses().Add(ref(solDev, qual, cvops.Version)))
	require.NoError(t, ds.Addresses().Add(ref(solTest, qual, cvops.Version)))
	require.NoError(t, ds.Addresses().Add(ref(solDev, "other", cvops.Version)))
	require.NoError(t, ds.Addresses().Add(ref(evmMain, qual, cvops.Version)))
	require.NoError(t, ds.Addresses().Add(ref(solTest, qual, semver.MustParse("1.0.0"))))

	got := getCommitteeVerifierDeployedChains(ds.Seal(), qual)
	slices.Sort(got)
	want := []uint64{solDev, solTest}
	slices.Sort(want)
	require.Equal(t, want, got)
}
