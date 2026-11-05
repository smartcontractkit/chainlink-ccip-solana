package devenv

import (
	"fmt"

	"github.com/Masterminds/semver/v3"

	"github.com/smartcontractkit/chainlink-ccv/build/devenv/chainreg"
	"github.com/smartcontractkit/chainlink-ccv/protocol"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"

	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	cvops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/committeeverifier"
	executorops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/executor"
	testreceiverops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/testreceiver"
)

// AddressResolver implements [chainreg.AddressResolver] for Solana devenv deployments.
type AddressResolver struct{}

// NewAddressResolver returns a Solana [chainreg.AddressResolver].
func NewAddressResolver() chainreg.AddressResolver {
	return AddressResolver{}
}

// LegacyReceiverQualifier selects the v1 example receiver. Every other qualifier, including the
// tcapi default, selects the v2 receiver.
const LegacyReceiverQualifier = "legacy"

// GetContractReceiver implements [chainreg.AddressResolver].
func (AddressResolver) GetContractReceiver(ds datastore.DataStore, chainSelector uint64, qualifier string) (protocol.UnknownAddress, error) {
	// On Solana an arbitrary message triggers a ccip_receive CPI into a program, so the "contract
	// receiver" is an example receiver program. The v1 and v2 receivers share a contract type and
	// differ by version, so the qualifier picks the version.
	version := testreceiverops.Version
	if qualifier == LegacyReceiverQualifier {
		version = testreceiverops.LegacyVersion
	}
	receiver, err := ResolveTestReceiverAddress(ds, chainSelector, version)
	if err != nil {
		return nil, fmt.Errorf("resolve contract receiver: %w", err)
	}
	return receiver.Bytes(), nil
}

// GetExecutor implements [chainreg.AddressResolver].
func (r AddressResolver) GetExecutor(ds datastore.DataStore, chainSelector uint64, qualifier string) (protocol.UnknownAddress, error) {
	return r.lookupProgramAddress(ds, chainSelector, executorops.ProxyContractType, executorops.Version, qualifier, "executor")
}

// GetCommitteeCCV implements [chainreg.AddressResolver].
func (r AddressResolver) GetCommitteeCCV(ds datastore.DataStore, chainSelector uint64, qualifier string) (protocol.UnknownAddress, error) {
	addr, err := r.lookupProgramAddress(ds, chainSelector, datastore.ContractType(cvops.ContractType), cvops.Version, qualifier, "committee verifier")
	if err != nil && qualifier != "" {
		addr, err = r.lookupProgramAddress(ds, chainSelector, datastore.ContractType(cvops.ContractType), cvops.Version, "", "committee verifier")
	}
	return addr, err
}

// GetToken implements [chainreg.AddressResolver].
func (AddressResolver) GetToken(ds datastore.DataStore, chainSelector uint64, poolRef datastore.AddressRef) (protocol.UnknownAddress, error) {
	tokenRef := TokenRefForPool(poolRef)
	pubkey, err := solcommon.GetDataStoreAddress(ds, datastore.NewAddressRefKey(
		chainSelector, tokenRef.Type, tokenRef.Version, tokenRef.Qualifier,
	))
	if err != nil {
		return protocol.UnknownAddress{}, fmt.Errorf("look up SPL token for chain %d pool %s %s: %w", chainSelector, poolRef.Type, poolRef.Version, err)
	}
	return protocol.UnknownAddress(pubkey.Bytes()), nil
}

func (AddressResolver) lookupProgramAddress(
	ds datastore.DataStore,
	chainSelector uint64,
	contractType datastore.ContractType,
	version *semver.Version,
	qualifier, contractName string,
) (protocol.UnknownAddress, error) {
	pubkey, err := solcommon.GetDataStoreAddress(ds, datastore.NewAddressRefKey(
		chainSelector, contractType, version, qualifier,
	))
	if err != nil {
		return protocol.UnknownAddress{}, fmt.Errorf("look up %s for chain %d qualifier %q: %w", contractName, chainSelector, qualifier, err)
	}
	return protocol.UnknownAddress(pubkey.Bytes()), nil
}
