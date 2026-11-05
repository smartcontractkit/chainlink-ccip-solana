package adapters

import (
	ccvadapters "github.com/smartcontractkit/chainlink-ccv/deployment/adapters"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
)

// SolanaCCVAggregatorConfigAdapter implements the chainlink-ccv offchain
// AggregatorConfigAdapter for devenv and CCV-specific changesets.
type SolanaCCVAggregatorConfigAdapter struct{}

var _ ccvadapters.AggregatorConfigAdapter = (*SolanaCCVAggregatorConfigAdapter)(nil)

func (a *SolanaCCVAggregatorConfigAdapter) ResolveSourceVerifierAddress(
	ds datastore.DataStore,
	chainSelector uint64,
	qualifier string,
) (string, error) {
	return committeeVerifier0xHex(ds, chainSelector, qualifier)
}

func (a *SolanaCCVAggregatorConfigAdapter) ResolveDestinationVerifierAddress(
	ds datastore.DataStore,
	chainSelector uint64,
	qualifier string,
) (string, error) {
	return committeeVerifier0xHex(ds, chainSelector, qualifier)
}

func (a *SolanaCCVAggregatorConfigAdapter) GetDeployedChains(
	ds datastore.DataStore,
	qualifier string,
) []uint64 {
	return getCommitteeVerifierDeployedChains(ds, qualifier)
}
