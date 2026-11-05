package sequences

import (
	"testing"

	"github.com/Masterminds/semver/v3"
	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"

	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
)

func TestResolveCctpPoolForMigration(t *testing.T) {
	sel := uint64(124615329519749607) // solana-mainnet
	pool := solana.NewWallet().PublicKey().String()
	otherPool := solana.NewWallet().PublicKey().String()
	v160 := semver.MustParse("1.6.0")
	v164 := semver.MustParse("1.6.4")

	dsWith := func(refs ...datastore.AddressRef) datastore.DataStore {
		ds := datastore.NewMemoryDataStore()
		for _, ref := range refs {
			require.NoError(t, ds.Addresses().Add(ref))
		}
		return ds.Seal()
	}
	ref := func(version *semver.Version, qualifier, address string) datastore.AddressRef {
		return datastore.AddressRef{
			ChainSelector: sel,
			Type:          "CCTPTokenPool",
			Version:       version,
			Qualifier:     qualifier,
			Address:       address,
		}
	}

	// A 1.6 pool migrates and gets its 2.0.0 ref registered.
	addr, needsRef, err := ResolveCctpPoolForMigration(dsWith(ref(v160, "USDC", pool)), sel)
	require.NoError(t, err)
	require.Equal(t, pool, addr.String())
	require.True(t, needsRef)

	// Any 1.x ref counts as legacy.
	_, needsRef, err = ResolveCctpPoolForMigration(dsWith(ref(v164, "USDC", pool)), sel)
	require.NoError(t, err)
	require.True(t, needsRef)

	// A re-run finds the registered 2.0.0 ref and registers nothing.
	addr, needsRef, err = ResolveCctpPoolForMigration(dsWith(
		ref(v160, "USDC", pool),
		ref(solcommon.Version2_0_0, solcommon.PoolProgramQualifier, pool),
	), sel)
	require.NoError(t, err)
	require.Equal(t, pool, addr.String())
	require.False(t, needsRef)

	// Other chains' and other types' refs are ignored.
	_, _, err = ResolveCctpPoolForMigration(dsWith(
		ref(v160, "USDC", pool),
		datastore.AddressRef{ChainSelector: sel, Type: "BurnMintTokenPool", Version: v160, Qualifier: "BnM", Address: otherPool},
	), 1)
	require.ErrorContains(t, err, "no 1.x CCTPTokenPool ref")
	_, _, err = ResolveCctpPoolForMigration(dsWith(
		datastore.AddressRef{ChainSelector: sel, Type: "BurnMintTokenPool", Version: v160, Qualifier: "BnM", Address: otherPool},
	), sel)
	require.ErrorContains(t, err, "no 1.x CCTPTokenPool ref")

	// Ambiguous legacy refs are an error.
	_, _, err = ResolveCctpPoolForMigration(dsWith(
		ref(v160, "USDC", pool),
		ref(v160, "USDC2", otherPool),
	), sel)
	require.ErrorContains(t, err, "multiple 1.x CCTPTokenPool refs")
}
