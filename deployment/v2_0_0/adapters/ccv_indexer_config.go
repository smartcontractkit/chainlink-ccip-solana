package adapters

import (
	"fmt"

	dsutil "github.com/smartcontractkit/chainlink-ccip/deployment/utils/datastore"
	ccvadapters "github.com/smartcontractkit/chainlink-ccv/deployment/adapters"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"

	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	cctpverifierops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/cctpverifier"
	lvops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/lombardverifier"
)

// SolanaCCVIndexerConfigAdapter implements the chainlink-ccv offchain
// IndexerConfigAdapter for devenv and CCV-specific changesets.
type SolanaCCVIndexerConfigAdapter struct{}

var _ ccvadapters.IndexerConfigAdapter = (*SolanaCCVIndexerConfigAdapter)(nil)

func (a *SolanaCCVIndexerConfigAdapter) ResolveVerifierAddresses(
	ds datastore.DataStore,
	chainSelector uint64,
	qualifier string,
	kind ccvadapters.VerifierKind,
) ([]string, error) {
	switch kind {
	case ccvadapters.CommitteeVerifierKind:
		return resolveCommitteeVerifierAddresses(ds, chainSelector, qualifier)
	case ccvadapters.LombardVerifierKind:
		return resolveLombardVerifierAddresses(ds, chainSelector, qualifier)
	case ccvadapters.CCTPVerifierKind:
		return resolveCCTPVerifierAddresses(ds, chainSelector, qualifier)
	default:
		return nil, fmt.Errorf("unknown verifier kind %q", kind)
	}
}

func resolveCommitteeVerifierAddresses(ds datastore.DataStore, chainSelector uint64, qualifier string) ([]string, error) {
	ref, err := committeeVerifierRef(ds, chainSelector, qualifier)
	if err != nil {
		return nil, &ccvadapters.MissingIndexerVerifierAddressesError{
			Kind:          ccvadapters.CommitteeVerifierKind,
			ChainSelector: chainSelector,
			Qualifier:     qualifier,
		}
	}
	hexAddr, err := solcommon.Base58To0xHex(ref.Address)
	if err != nil {
		return nil, fmt.Errorf("committee verifier ref for chain %d qualifier %q: %w", chainSelector, qualifier, err)
	}
	return []string{hexAddr}, nil
}

func resolveLombardVerifierAddresses(ds datastore.DataStore, chainSelector uint64, qualifier string) ([]string, error) {
	ref, err := ds.Addresses().Get(datastore.NewAddressRefKey(
		chainSelector,
		datastore.ContractType(lvops.ContractType),
		lvops.Version,
		qualifier,
	))
	if err != nil {
		return nil, &ccvadapters.MissingIndexerVerifierAddressesError{
			Kind:          ccvadapters.LombardVerifierKind,
			ChainSelector: chainSelector,
			Qualifier:     qualifier,
		}
	}
	hexAddr, err := solcommon.Base58To0xHex(ref.Address)
	if err != nil {
		return nil, fmt.Errorf("lombard token pool ref for chain %d qualifier %q: %w", chainSelector, qualifier, err)
	}
	return []string{hexAddr}, nil
}

// resolveCCTPVerifierAddresses returns the address the indexer should watch for CCTP-verified
// messages. On Solana the verifier is not its own program: it is a second set of instructions
// inside cctp_token_pool, registered in the datastore under its own contract type.
func resolveCCTPVerifierAddresses(ds datastore.DataStore, chainSelector uint64, qualifier string) ([]string, error) {
	ref, err := solcommon.FindAndFormatCompatibleRef(ds, datastore.AddressRef{
		Type:      datastore.ContractType(cctpverifierops.ContractType),
		Version:   cctpverifierops.Version,
		Qualifier: qualifier,
	}, chainSelector, dsutil.FullRef)
	if err != nil {
		return nil, &ccvadapters.MissingIndexerVerifierAddressesError{
			Kind:          ccvadapters.CCTPVerifierKind,
			ChainSelector: chainSelector,
			Qualifier:     qualifier,
		}
	}
	hexAddr, err := solcommon.Base58To0xHex(ref.Address)
	if err != nil {
		return nil, fmt.Errorf("cctp verifier ref for chain %d qualifier %q: %w", chainSelector, qualifier, err)
	}
	return []string{hexAddr}, nil
}
