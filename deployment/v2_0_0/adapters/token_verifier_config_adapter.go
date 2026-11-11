package adapters

import (
	"fmt"

	ccvadapters "github.com/smartcontractkit/chainlink-ccv/deployment/adapters"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"

	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	cctpverifierops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/cctpverifier"
	lvops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/lombardverifier"
	router "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/router"
)

type SolanaTokenVerifierConfigAdapter struct{}

var _ ccvadapters.TokenVerifierConfigAdapter = (*SolanaTokenVerifierConfigAdapter)(nil)

func (a *SolanaTokenVerifierConfigAdapter) ResolveTokenVerifierAddresses(
	ds datastore.DataStore,
	chainSelector uint64,
	cctpQualifier string,
	lombardQualifier string,
) (*ccvadapters.TokenVerifierChainAddresses, error) {
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

	result := &ccvadapters.TokenVerifierChainAddresses{
		OnRampAddress:    onRampAddr,
		RMNRemoteAddress: rmnRemoteAddr,
	}

	// There's no EVM-style "LombardVerifierResolver" indirection contract on Solana: the
	// token-verifier's CCVWriter uses this value directly as the message's VerifierDestAddress
	// (see cmd/verifier/token/main.go, storage.NewCCVWriter(verifierConfig.LombardConfig.ParsedVerifierResolvers, ...)),
	// which must equal the lombard_token_pool program's own address (its get_required_ccvs
	// self-declares crate::ID as a required CCV). Looked up by lvops.ContractType/qualifier
	// (e.g. BurnMint/LockRelease-only test cases), which is fine - upstream skips empty values.
	lombardRef, err := ds.Addresses().Get(datastore.NewAddressRefKey(
		chainSelector,
		datastore.ContractType(lvops.ContractType),
		lvops.Version,
		lombardQualifier,
	))
	if err == nil {
		lombardAddr, formatErr := solcommon.RefTo0xHex(lombardRef)
		if formatErr != nil {
			return nil, fmt.Errorf("lombard verifier address for chain %d: %w", chainSelector, formatErr)
		}
		result.LombardVerifierResolverAddress = lombardAddr
	}

	// The CCTP verifier is optional: a chain without a CCTP deployment simply has no ref, and the
	// token verifier still needs the on-ramp and RMN addresses above. Circle records the pool signer
	// PDA as messageSender because it signs deposit_for_burn_with_hook. The offchain verifier matches
	// that field against CCTPVerifierAddress. Solana has no separate resolver contract, so account
	// resolution still uses the cctp_token_pool program itself. The qualifier mirrors the production
	// convention (qualifier equals the contract type) and is a pass-through, matching the EVM adapter.
	if cctpAddr, err := solcommon.FindAndFormatCompatibleRef(ds, datastore.AddressRef{
		Type:      datastore.ContractType(cctpverifierops.ContractType),
		Version:   cctpverifierops.Version,
		Qualifier: cctpQualifier,
	}, chainSelector, solcommon.RefTo0xHex); err == nil {
		messageSender, err := (&CCTPChainAdapter{}).poolSignerBytes(ds, chainSelector)
		if err != nil {
			return nil, fmt.Errorf("CCTP message sender for chain %d: %w", chainSelector, err)
		}
		result.CCTPVerifierAddress = fmt.Sprintf("0x%x", messageSender)
		result.CCTPVerifierResolverAddress = cctpAddr
	}

	return result, nil
}
