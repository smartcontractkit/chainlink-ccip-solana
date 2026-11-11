package sequences

import (
	"testing"

	"github.com/Masterminds/semver/v3"
	"github.com/stretchr/testify/require"

	tokenapi "github.com/smartcontractkit/chainlink-ccip/deployment/tokens"
	cldf_chain "github.com/smartcontractkit/chainlink-deployments-framework/chain"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	cldf_ops "github.com/smartcontractkit/chainlink-deployments-framework/operations"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations/optest"
)

// A re-run of a token expansion finds the mint in the datastore and must not deploy a second one.
// It still has to return the existing ref, otherwise the caller has no token ref to build the pool
// from when the input does not set one.
func TestDeployTokenReturnsExistingRef(t *testing.T) {
	const selector = uint64(16423721717087811551)
	existing := datastore.AddressRef{
		ChainSelector: selector,
		Type:          "SPLTokens",
		Version:       semver.MustParse("2.0.0"),
		Qualifier:     "TESTTR",
		Address:       "9Jmp2rhfJFJyZTXzvPEgnFMgvTnkDD6atDZNHHNSzaJV",
	}
	ds := datastore.NewMemoryDataStore()
	require.NoError(t, ds.Addresses().Add(existing))

	// No Solana chains are provided, so any attempt to deploy a mint would fail.
	report, err := cldf_ops.ExecuteSequence(
		optest.NewBundle(t), (&SolanaAdapter{}).DeployToken(), cldf_chain.NewBlockChains(nil),
		tokenapi.DeployTokenInput{
			ChainSelector:     selector,
			Type:              "SPLTokens",
			Symbol:            "TESTTR",
			ExistingDataStore: ds.Seal(),
		})
	require.NoError(t, err)
	require.Equal(t, []datastore.AddressRef{existing}, report.Output.Addresses)
	require.Empty(t, report.Output.BatchOps)
}
