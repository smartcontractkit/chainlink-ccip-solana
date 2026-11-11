package sequences

import (
	"testing"

	"github.com/Masterminds/semver/v3"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"

	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
)

func TestUpgradeAdditionalRefsReplacesTheSameMajor(t *testing.T) {
	const selector = uint64(1)
	pool := func(version, address string) datastore.AddressRef {
		return datastore.AddressRef{
			ChainSelector: selector, Type: datastore.ContractType("BurnMintTokenPool"),
			Qualifier: solcommon.PoolProgramQualifier, Version: semver.MustParse(version), Address: address,
		}
	}
	v201 := semver.MustParse("2.0.1")

	// The program alias saved at 2.0.0 is replaced by 2.0.1; the 1.6.0 alias stays.
	got := UpgradeAdditionalRefs(UpgradeTypeBurnMintTokenPool, []datastore.AddressRef{pool("1.6.0", "a"), pool("2.0.0", "a")}, selector, v201, "a")
	require.Len(t, got.Add, 1)
	require.Equal(t, "2.0.1", got.Add[0].Version.String())
	require.Len(t, got.Superseded, 1)
	require.Equal(t, "2.0.0", got.Superseded[0].Version.String())

	// A same-major alias at another address is reported, not overwritten.
	got = UpgradeAdditionalRefs(UpgradeTypeBurnMintTokenPool, []datastore.AddressRef{pool("2.0.0", "other")}, selector, v201, "a")
	require.Empty(t, got.Add)
	require.Empty(t, got.Superseded)
	require.Len(t, got.Conflicting, 1)
}
