package solcommon

import (
	"testing"

	"github.com/Masterminds/semver/v3"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
)

func TestFindLatestRef(t *testing.T) {
	const sel, other = uint64(16423721717087811551), uint64(1)
	spl, spl2022 := datastore.ContractType(SPLTokens), datastore.ContractType(SPL2022Tokens)
	ref := func(chain uint64, contractType datastore.ContractType, version, qualifier, address string) datastore.AddressRef {
		r := datastore.AddressRef{ChainSelector: chain, Type: contractType, Qualifier: qualifier, Address: address}
		if version != "" {
			r.Version = semver.MustParse(version)
		}
		return r
	}
	build := func(refs ...datastore.AddressRef) datastore.DataStore {
		ds := datastore.NewMemoryDataStore()
		for _, r := range refs {
			require.NoError(t, ds.Addresses().Add(r))
		}
		return ds.Seal()
	}
	query := datastore.AddressRef{Type: spl, Qualifier: "USDC"}

	t.Run("a token registered at several versions resolves to the highest, whatever version is asked for", func(t *testing.T) {
		ds := build(
			ref(sel, spl, "1.6.0", "USDC", "mint"),
			ref(sel, spl, "2.0.0", "USDC", "mint"),
		)
		got, err := FindLatestRef(ds, query, sel)
		require.NoError(t, err)
		require.Equal(t, "2.0.0", got.Version.String())

		asked := query
		asked.Version = semver.MustParse("1.6.0")
		got, err = FindLatestRef(ds, asked, sel)
		require.NoError(t, err)
		require.Equal(t, "2.0.0", got.Version.String(), "the version in the query is ignored")
	})

	t.Run("a single version that is not 2.0.0 is found", func(t *testing.T) {
		ds := build(ref(sel, spl, "1.6.0", "USDC", "mint"))
		got, err := FindLatestRef(ds, query, sel)
		require.NoError(t, err)
		require.Equal(t, "mint", got.Address)
	})

	t.Run("another chain, type or qualifier does not match", func(t *testing.T) {
		ds := build(
			ref(other, spl, "2.0.0", "USDC", "mint"),
			ref(sel, spl2022, "2.0.0", "USDC", "mint"),
			ref(sel, spl, "2.0.0", "OTHER", "mint"),
		)
		_, err := FindLatestRef(ds, query, sel)
		require.ErrorIs(t, err, ErrAddressRefNotFound)
	})

	t.Run("different addresses at the latest version are ambiguous", func(t *testing.T) {
		ds := build(
			ref(sel, spl, "2.0.0", "A", "mintA"),
			ref(sel, spl, "2.0.0", "B", "mintB"),
		)
		_, err := FindLatestRef(ds, datastore.AddressRef{Type: spl}, sel)
		require.ErrorContains(t, err, "ambiguous")
	})
}

func TestFindLatestRefInRefs(t *testing.T) {
	const sel = uint64(16423721717087811551)
	refs := []datastore.AddressRef{
		{ChainSelector: sel, Type: "LinkToken", Version: semver.MustParse("1.6.0"), Qualifier: "LINK", Address: "link"},
		{ChainSelector: sel, Type: "LinkToken", Version: semver.MustParse("2.0.0"), Qualifier: "LINK", Address: "link"},
		{ChainSelector: sel, Type: "LinkToken", Qualifier: "OLD", Address: "old"},
	}
	got, err := FindLatestRefInRefs(refs, sel, "LinkToken", "LINK")
	require.NoError(t, err)
	require.Equal(t, "2.0.0", got.Version.String())

	got, err = FindLatestRefInRefs(refs[2:], sel, "LinkToken", "")
	require.NoError(t, err, "a ref without a version is still found")
	require.Equal(t, "old", got.Address)

	_, err = FindLatestRefInRefs(refs, sel, "LinkToken", "NONE")
	require.ErrorIs(t, err, ErrAddressRefNotFound)
}
