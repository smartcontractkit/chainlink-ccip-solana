package adapters

import (
	"fmt"

	chainsel "github.com/smartcontractkit/chain-selectors"
	ccvadapters "github.com/smartcontractkit/chainlink-ccv/deployment/adapters"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"

	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	router "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/router"
)

type SolanaVerifierConfigAdapter struct{}

var _ ccvadapters.VerifierConfigAdapter = (*SolanaVerifierConfigAdapter)(nil)
var _ ccvadapters.VerifierNodeChainJDSupport = (*SolanaVerifierConfigAdapter)(nil)

func (a *SolanaVerifierConfigAdapter) ResolveVerifierContractAddresses(
	ds datastore.DataStore,
	chainSelector uint64,
	committeeQualifier string,
	executorQualifier string,
) (*ccvadapters.VerifierContractAddresses, error) {
	committeeVerifierAddr, err := committeeVerifier0xHex(ds, chainSelector, committeeQualifier)
	if err != nil {
		return nil, err
	}

	onRampAddr, err := solcommon.FindAndFormatCompatibleRef(ds, datastore.AddressRef{
		Type:    datastore.ContractType(router.ContractType),
		Version: router.Version,
	}, chainSelector, solcommon.RefTo0xHex)
	if err != nil {
		return nil, fmt.Errorf("on ramp address for chain %d: %w", chainSelector, err)
	}

	rmnRemoteAddr, err := solcommon.FindAndFormatCompatibleRef(ds, datastore.AddressRef{
		Type:    rmnRemoteContractType,
		Version: rmnRemoteSemver,
	}, chainSelector, solcommon.RefTo0xHex)
	if err != nil {
		return nil, fmt.Errorf("rmn remote address for chain %d: %w", chainSelector, err)
	}

	return &ccvadapters.VerifierContractAddresses{
		CommitteeVerifierAddress: committeeVerifierAddr,
		OnRampAddress:            onRampAddr,
		RMNRemoteAddress:         rmnRemoteAddr,
	}, nil
}

func (a *SolanaVerifierConfigAdapter) GetSignerAddressFamily() string {
	return chainsel.FamilySolana
}

func (a *SolanaVerifierConfigAdapter) RequiresNodeChainSupportInJD() bool {
	return false
}
