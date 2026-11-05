package adapters

import (
	"fmt"

	ccvadapters "github.com/smartcontractkit/chainlink-ccip/deployment/v2_0_0/adapters"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
)

type SolanaCommitteeVerifierContractAdapter struct{}

var _ ccvadapters.CommitteeVerifierContractAdapter = (*SolanaCommitteeVerifierContractAdapter)(nil)

// ResolveCommitteeVerifierContracts is called by PopulateCommitteeConfig() in cl-ccip.
func (a *SolanaCommitteeVerifierContractAdapter) ResolveCommitteeVerifierContracts(
	ds datastore.DataStore,
	chainSelector uint64,
	qualifier string,
) ([]datastore.AddressRef, error) {
	verifier, err := committeeVerifierRef(ds, chainSelector, qualifier)
	if err != nil {
		return nil, fmt.Errorf("resolve committee verifier contracts: %w", err)
	}

	return []datastore.AddressRef{verifier}, nil
}

// NOTE: There are no committee verifier resolvers in Solana. This method returns
// the verifier address
func (a *SolanaCommitteeVerifierContractAdapter) GetCommitteeVerifierResolver(
	ds datastore.DataStore,
	chainSelector uint64,
	qualifier string,
) ([]datastore.AddressRef, error) {
	return a.ResolveCommitteeVerifierContracts(ds, chainSelector, qualifier)
}
