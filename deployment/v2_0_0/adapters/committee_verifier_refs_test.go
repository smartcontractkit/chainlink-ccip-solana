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
	solMain := chainsel.SOLANA_MAINNET.Selector
	require.NoError(t, ds.Addresses().Add(ref(solDev, qual, cvops.Version)))
	require.NoError(t, ds.Addresses().Add(ref(solTest, qual, cvops.Version)))
	require.NoError(t, ds.Addresses().Add(ref(solDev, "other", cvops.Version)))
	require.NoError(t, ds.Addresses().Add(ref(evmMain, qual, cvops.Version)))
	require.NoError(t, ds.Addresses().Add(ref(solTest, qual, semver.MustParse("1.0.0"))))
	require.NoError(t, ds.Addresses().Add(ref(solMain, qual, semver.MustParse("2.1.0")))) // upgraded in place

	got := getCommitteeVerifierDeployedChains(ds.Seal(), qual)
	slices.Sort(got)
	want := []uint64{solDev, solTest, solMain}
	slices.Sort(want)
	require.Equal(t, want, got)
}

// cvRef is a CommitteeVerifier ref; address only needs to be non-empty here.
func cvRef(chain uint64, qualifier, version, address string) datastore.AddressRef {
	return datastore.AddressRef{
		Address:       address,
		ChainSelector: chain,
		Qualifier:     qualifier,
		Type:          datastore.ContractType(cvops.ContractType),
		Version:       semver.MustParse(version),
	}
}

func TestDeployedCommitteeVerifierChains_anyQualifier(t *testing.T) {
	t.Parallel()
	ds := datastore.NewMemoryDataStore()
	solMain := chainsel.SOLANA_MAINNET.Selector
	solDev := chainsel.SOLANA_DEVNET.Selector
	for _, r := range []datastore.AddressRef{
		cvRef(solMain, "default", "2.0.0", "program-a"),
		cvRef(solMain, "secondary", "2.1.0", "program-b"),
		cvRef(solDev, "", "2.0.0", "program-c"),
		cvRef(chainsel.SOLANA_TESTNET.Selector, "default", "1.0.0", "program-d"),   // other major
		cvRef(chainsel.ETHEREUM_MAINNET.Selector, "default", "2.0.0", "program-e"), // other family
	} {
		require.NoError(t, ds.Addresses().Add(r))
	}

	adapter := &SolanaCommitteeVerifierOnchainAdapter{}
	got := adapter.DeployedCommitteeVerifierChains(ds.Seal())
	want := []uint64{solMain, solDev}
	slices.Sort(got)
	slices.Sort(want)
	require.Equal(t, want, got, "every qualifier counts, other majors and families do not")
	require.Empty(t, adapter.DeployedCommitteeVerifierChains(nil))
}

func TestCommitteeVerifierRef_findsInPlaceUpgrade(t *testing.T) {
	t.Parallel()
	solMain := chainsel.SOLANA_MAINNET.Selector
	ds := datastore.NewMemoryDataStore()
	require.NoError(t, ds.Addresses().Add(cvRef(solMain, "default", "2.1.0", "program-a")))
	require.NoError(t, ds.Addresses().Add(cvRef(solMain, "default", "3.0.0", "program-b")))

	ref, err := committeeVerifierRef(ds.Seal(), solMain, "default")
	require.NoError(t, err)
	require.Equal(t, "2.1.0", ref.Version.String(), "the 2.x lookup takes the upgraded ref and skips other majors")
	require.Equal(t, "program-a", ref.Address)

	_, err = committeeVerifierRef(ds.Seal(), solMain, "")
	require.Error(t, err, "an empty qualifier only matches refs without one")
}

func TestLatestCommitteeVerifierRefs_oncePerQualifier(t *testing.T) {
	t.Parallel()
	solMain := chainsel.SOLANA_MAINNET.Selector
	ds := datastore.NewMemoryDataStore()
	for _, r := range []datastore.AddressRef{
		cvRef(solMain, "default", "2.0.0", "program-a"), // left behind by the in-place upgrade
		cvRef(solMain, "default", "2.1.0", "program-a"),
		cvRef(solMain, "secondary", "2.0.0", "program-b"),
		cvRef(solMain, "", "2.0.0", "program-c"),
		cvRef(chainsel.SOLANA_DEVNET.Selector, "default", "2.0.0", "program-d"),
	} {
		require.NoError(t, ds.Addresses().Add(r))
	}

	refs, err := latestCommitteeVerifierRefs(ds.Seal(), solMain)
	require.NoError(t, err)
	require.Len(t, refs, 2)
	require.Equal(t, "default", refs[0].Qualifier)
	require.Equal(t, "2.1.0", refs[0].Version.String())
	require.Equal(t, "secondary", refs[1].Qualifier)
}
