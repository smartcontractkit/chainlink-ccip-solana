package solcommon

import (
	"testing"

	"github.com/Masterminds/semver/v3"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
)

func TestRefVersionMatches(t *testing.T) {
	v := semver.MustParse
	require.True(t, RefVersionMatches(v("2.0.1"), v("2.0.0")))
	require.True(t, RefVersionMatches(v("2.0.0"), v("2.0.1")))
	require.False(t, RefVersionMatches(v("1.6.0"), v("2.0.0")))
	require.False(t, RefVersionMatches(v("2.0.0"), v("1.6.0")))
	require.True(t, RefVersionMatches(v("1.6.4"), v("1.6.0")), "a 1.x lookup takes any 1.x ref")
	require.False(t, RefVersionMatches(nil, v("2.0.0")))
	require.True(t, RefVersionMatches(v("1.6.0"), nil))
}

func TestFindCompatibleRef(t *testing.T) {
	const sel = uint64(16423721717087811551)
	typ := datastore.ContractType("Router")
	ref := func(version, address string) datastore.AddressRef {
		return datastore.AddressRef{ChainSelector: sel, Type: typ, Version: semver.MustParse(version), Address: address}
	}
	build := func(refs ...datastore.AddressRef) datastore.DataStore {
		ds := datastore.NewMemoryDataStore()
		for _, r := range refs {
			require.NoError(t, ds.Addresses().Add(r))
		}
		return ds.Seal()
	}
	query := datastore.AddressRef{Type: typ, Version: semver.MustParse("2.0.0")}

	t.Run("a 2.0.1-only datastore resolves a 2.0.0 lookup", func(t *testing.T) {
		got, err := FindCompatibleRef(build(ref("2.0.1", "a")), query, sel)
		require.NoError(t, err)
		require.Equal(t, "2.0.1", got.Version.String())
	})
	t.Run("2.0.0 and 2.0.1 at one address resolve to the highest", func(t *testing.T) {
		got, err := FindCompatibleRef(build(ref("2.0.0", "a"), ref("2.0.1", "a")), query, sel)
		require.NoError(t, err)
		require.Equal(t, "2.0.1", got.Version.String())
	})
	t.Run("1.6.0 never satisfies a 2.x lookup", func(t *testing.T) {
		_, err := FindCompatibleRef(build(ref("1.6.0", "a")), query, sel)
		require.ErrorIs(t, err, ErrAddressRefNotFound)
	})
	t.Run("1.6.0 and 2.0.0 at one address pick the 2.x ref", func(t *testing.T) {
		got, err := FindCompatibleRef(build(ref("1.6.0", "a"), ref("2.0.0", "a")), query, sel)
		require.NoError(t, err)
		require.Equal(t, "2.0.0", got.Version.String())
	})
	t.Run("different addresses at the highest version are ambiguous", func(t *testing.T) {
		other := ref("2.0.1", "b")
		other.Qualifier = "other"
		_, err := FindCompatibleRef(build(ref("2.0.1", "a"), other), query, sel)
		require.ErrorContains(t, err, "ambiguous")
	})
	t.Run("GetDataStoreAddress falls back to the version line", func(t *testing.T) {
		ds := build(datastore.AddressRef{ChainSelector: sel, Type: typ, Version: semver.MustParse("2.0.1"), Address: "11111111111111111111111111111111"})
		_, err := GetDataStoreAddress(ds, datastore.NewAddressRefKey(sel, typ, semver.MustParse("2.0.0"), ""))
		require.NoError(t, err)
	})
}

func TestPoolRefByQualifierAcrossVersions(t *testing.T) {
	const sel = uint64(16423721717087811551)
	add := func(ds *datastore.MemoryDataStore, version, address string) {
		require.NoError(t, ds.Addresses().Add(datastore.AddressRef{
			ChainSelector: sel, Type: "BurnMintTokenPool", Qualifier: "q", Version: semver.MustParse(version), Address: address,
		}))
	}
	same := datastore.NewMemoryDataStore()
	add(same, "2.0.0", "a")
	add(same, "2.0.1", "a")
	got, err := PoolRefByQualifier(same.Seal(), sel, "BurnMintTokenPool", "q")
	require.NoError(t, err)
	require.Equal(t, "2.0.1", got.Version.String())

	diff := datastore.NewMemoryDataStore()
	add(diff, "2.0.0", "a")
	add(diff, "2.0.1", "b")
	_, err = PoolRefByQualifier(diff.Seal(), sel, "BurnMintTokenPool", "q")
	require.ErrorContains(t, err, "ambiguous")
}
