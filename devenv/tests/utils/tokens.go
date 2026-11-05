package utils

import (
	"fmt"

	"github.com/smartcontractkit/chainlink-ccv/protocol"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"

	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
)

// GetSolanaTokenAddress looks up an SPL token mint address from the CLDF datastore
// for the given chain selector and qualifier.
func GetSolanaTokenAddress(ds datastore.DataStore, chainSelector uint64, qualifier string) (protocol.UnknownAddress, error) {
	pubkey, err := solcommon.GetDataStoreAddress(ds, datastore.NewAddressRefKey(
		chainSelector, datastore.ContractType(solcommon.SPLTokens), solcommon.Version2_0_0, qualifier,
	))
	if err != nil {
		return nil, fmt.Errorf("look up SPLTokens address for chain %d qualifier %q: %w (available SPLTokens qualifiers on this chain: %v)",
			chainSelector, qualifier, err, AvailableSPLTokenQualifiers(ds, chainSelector))
	}
	return protocol.UnknownAddress(pubkey.Bytes()), nil
}

// AvailableSPLTokenQualifiers lists the qualifiers of SPLTokens refs registered for a
// chain, for diagnostics when a lookup misses.
func AvailableSPLTokenQualifiers(ds datastore.DataStore, chainSelector uint64) []string {
	refs := ds.Addresses().Filter(
		datastore.AddressRefByChainSelector(chainSelector),
		datastore.AddressRefByType(datastore.ContractType(solcommon.SPLTokens)),
	)
	qualifiers := make([]string, 0, len(refs))
	for _, ref := range refs {
		qualifiers = append(qualifiers, ref.Qualifier)
	}
	return qualifiers
}
