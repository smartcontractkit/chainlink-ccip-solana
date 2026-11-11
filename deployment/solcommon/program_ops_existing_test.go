package solcommon

import (
	"context"
	"testing"

	"github.com/Masterminds/semver/v3"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations"
)

// A 2.0.1-only datastore must reuse the ref; the zero-value chain would fail if a deploy were attempted.
func TestDeployContractIfNotInDatastoreReusesSameMajorRef(t *testing.T) {
	b := operations.NewBundle(context.Background, logger.Test(t), operations.NewMemoryReporter())
	existing := datastore.AddressRef{Type: "Router", Version: semver.MustParse("2.0.1"), Address: "addr"}

	got, err := DeployContractIfNotInDatastore(b, cldf_solana.Chain{}, []datastore.AddressRef{existing}, "Router", Version2_0_0, "", "ccip_router")
	require.NoError(t, err)
	require.Equal(t, existing, got)

	// A ref without a version is ignored.
	require.NotPanics(t, func() {
		_, _ = DeployContractIfNotInDatastore(b, cldf_solana.Chain{}, []datastore.AddressRef{{Type: "Router", Address: "x"}}, "Router", Version2_0_0, "", "ccip_router")
	})
}

// A ref for another chain must never be reused, even if type and version match.
func TestDeployContractIfNotInDatastoreIgnoresOtherChains(t *testing.T) {
	b := operations.NewBundle(context.Background, logger.Test(t), operations.NewMemoryReporter())
	other := datastore.AddressRef{ChainSelector: 42, Type: "Router", Version: semver.MustParse("2.0.0"), Address: "other-chain"}

	// The zero-value chain cannot deploy, so reaching the deploy path returns an error rather
	// than the other chain's address.
	got, err := DeployContractIfNotInDatastore(b, cldf_solana.Chain{}, []datastore.AddressRef{other}, "Router", Version2_0_0, "", "ccip_router")
	require.Error(t, err)
	require.NotEqual(t, "other-chain", got.Address)
}

func TestDecodeUpgradeableLoaderStateRejectsShortProgramData(t *testing.T) {
	for n := 4; n < 13; n++ {
		data := make([]byte, n)
		data[0] = 3 // ProgramData
		require.NotPanics(t, func() {
			_, err := decodeUpgradeableLoaderState(data)
			require.Error(t, err, "len %d", n)
		})
	}
}
