package adapters

import (
	"fmt"

	chainsel "github.com/smartcontractkit/chain-selectors"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"

	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	cvops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/committeeverifier"
)

func committeeVerifierRef(ds datastore.DataStore, chainSelector uint64, qualifier string) (datastore.AddressRef, error) {
	ref, err := ds.Addresses().Get(datastore.NewAddressRefKey(
		chainSelector,
		datastore.ContractType(cvops.ContractType),
		cvops.Version,
		qualifier,
	))
	if err != nil {
		return datastore.AddressRef{}, fmt.Errorf("committee verifier not found for chain %d qualifier %q: %w", chainSelector, qualifier, err)
	}
	if ref.Address == "" {
		return datastore.AddressRef{}, fmt.Errorf("committee verifier ref for chain %d qualifier %q has empty address", chainSelector, qualifier)
	}
	return ref, nil
}

func committeeVerifier0xHex(ds datastore.DataStore, chainSelector uint64, qualifier string) (string, error) {
	ref, err := committeeVerifierRef(ds, chainSelector, qualifier)
	if err != nil {
		return "", err
	}
	hexAddr, err := solcommon.Base58To0xHex(ref.Address)
	if err != nil {
		return "", fmt.Errorf("committee verifier ref for chain %d qualifier %q: %w", chainSelector, qualifier, err)
	}
	return hexAddr, nil
}

// getCommitteeVerifierDeployedChains returns every Solana chain selector that has a
// CommitteeVerifier address ref in the datastore for the given qualifier (same key
// shape as committeeVerifierRef).
func getCommitteeVerifierDeployedChains(ds datastore.DataStore, qualifier string) []uint64 {
	if ds == nil {
		return nil
	}
	selectors := solcommon.GetDeployedChainSelectors(ds,
		datastore.AddressRefByQualifier(qualifier),
		datastore.AddressRefByType(datastore.ContractType(cvops.ContractType)),
		datastore.AddressRefByVersion(cvops.Version),
	)
	chains := make([]uint64, 0, len(selectors))
	for _, sel := range selectors {
		family, err := chainsel.GetSelectorFamily(sel)
		if err != nil || family != chainsel.FamilySolana {
			continue
		}
		chains = append(chains, sel)
	}
	return chains
}
