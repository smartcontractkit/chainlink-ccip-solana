package solcommon

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/gagliardetto/solana-go"

	datastore_utils "github.com/smartcontractkit/chainlink-ccip/deployment/utils/datastore"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	cldf_deployment "github.com/smartcontractkit/chainlink-deployments-framework/deployment"
)

// ErrAddressRefNotFound is returned by PubkeyFromRefs when no matching ref exists.
var ErrAddressRefNotFound = errors.New("address ref not found")

// ToByteArray formats a datastore.AddressRef into a byte slice.
func ToByteArray(ref datastore.AddressRef) (bytes []byte, err error) {
	if ref.Address == "" {
		return nil, fmt.Errorf("address is empty in ref: %s", datastore_utils.SprintRef(ref))
	}
	addr, err := ToAddress(ref)
	if err != nil {
		return nil, err
	}
	return addr.Bytes(), nil
}

// ToAddress formats a datastore.AddressRef into a solana.PublicKey.
func ToAddress(ref datastore.AddressRef) (commonAddress solana.PublicKey, err error) {
	if ref.Address == "" {
		return solana.PublicKey{}, fmt.Errorf("address is empty in ref: %s", datastore_utils.SprintRef(ref))
	}
	out, err := solana.PublicKeyFromBase58(ref.Address)
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("address is not a valid base58 address in ref: %s", datastore_utils.SprintRef(ref))
	}
	return out, nil
}

// Base58ToHex converts a Solana base58 address to a hex string.
// Use at adapter boundaries where chain-agnostic CCV consumers expect hex.
func Base58ToHex(address string) (string, error) {
	pk, err := solana.PublicKeyFromBase58(address)
	if err != nil {
		return "", fmt.Errorf("invalid base58 address %q: %w", address, err)
	}
	return hex.EncodeToString(pk.Bytes()), nil
}

// Base58To0xHex converts a Solana base58 address to 0x-prefixed hex (64 hex chars for a pubkey).
// Use where consumers expect Ethereum-style 0x + hex (e.g. executor config, indexer issuer addresses).
func Base58To0xHex(address string) (string, error) {
	h, err := Base58ToHex(address)
	if err != nil {
		return "", err
	}
	return "0x" + h, nil
}

// RefTo0xHex formats a datastore.AddressRef's base58 address as 0x-prefixed hex.
// It satisfies datastore_utils.FormatFn[string] so it can be passed directly to FindAndFormatRef.
func RefTo0xHex(ref datastore.AddressRef) (string, error) {
	return Base58To0xHex(ref.Address)
}

// LeftPadTo32Bytes left-pads a raw remote-chain address to 32 bytes so it matches the
// abi.encode(address) representation carried in a CCV-signed MessageV1's on_ramp_address /
// off_ramp_address fields. Solana address fields are 32 bytes already; EVM addresses arrive
// here as 20 bytes and must be left-padded with zeros.
//
// See _vendor/chainlink-ccip/chains/evm/contracts/onRamp/OnRamp.sol (`abi.encode(address(this))`)
// and the ENCODING_INVARIANTS.md note that source-side addresses are padded to 32 bytes for EVM.
func LeftPadTo32Bytes(b []byte) ([]byte, error) {
	if len(b) > 32 {
		return nil, fmt.Errorf("address longer than 32 bytes: %d", len(b))
	}
	if len(b) == 32 {
		return b, nil
	}
	out := make([]byte, 32)
	copy(out[32-len(b):], b)
	return out, nil
}

// DecodeRemoteSender parses a hex address, with or without the 0x prefix, and left-pads it to the
// 32 bytes a receiver stores remote senders under.
func DecodeRemoteSender(sender string) ([]byte, error) {
	raw, err := hex.DecodeString(strings.TrimPrefix(sender, "0x"))
	if err != nil {
		return nil, fmt.Errorf("decode remote sender %q: %w", sender, err)
	}
	padded, err := LeftPadTo32Bytes(raw)
	if err != nil {
		return nil, fmt.Errorf("normalize remote sender %q: %w", sender, err)
	}
	return padded, nil
}

// BytesToPubkey parses raw bytes to Solana address
func BytesToPubkey(b []byte) (solana.PublicKey, error) {
	if len(b) != 32 {
		return solana.PublicKey{}, fmt.Errorf("invalid solana address %v, expected 32 bytes, got %d", b, len(b))
	}
	return solana.PublicKeyFromBytes(b), nil
}

// GetDataStoreAddress looks up a contract address from the datastore by key and returns it as a solana.PublicKey.
func GetDataStoreAddress(ds datastore.DataStore, key datastore.AddressRefKey) (solana.PublicKey, error) {
	ref, err := ds.Addresses().Get(key)
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("address not found: %w", err)
	}
	return ToAddress(ref)
}

// PubkeyFromRefs looks up a Solana pubkey in a ref slice (e.g. ExistingAddresses
// passed to a sequence). key selects chain, contract type, and version.
func PubkeyFromRefs(refs []datastore.AddressRef, key datastore.AddressRefKey) (solana.PublicKey, error) {
	for _, ref := range refs {
		if ref.ChainSelector != key.ChainSelector() || ref.Type != key.Type() {
			continue
		}
		if key.Version() != nil && !ref.Version.Equal(key.Version()) {
			continue
		}
		if q := key.Qualifier(); q != "" && ref.Qualifier != q {
			continue
		}
		pubkey, err := solana.PublicKeyFromBase58(ref.Address)
		if err != nil {
			return solana.PublicKey{}, fmt.Errorf("parse address %q for type %s: %w", ref.Address, key.Type(), err)
		}
		return pubkey, nil
	}
	versionLabel := "any"
	if key.Version() != nil {
		versionLabel = key.Version().String()
	}
	return solana.PublicKey{}, fmt.Errorf(
		"%w: chain %d type %s version %s",
		ErrAddressRefNotFound,
		key.ChainSelector(),
		key.Type(),
		versionLabel,
	)
}

// LookupAddressHex looks up a contract address from the datastore and converts it to hex.
// Returns fallback if ds is nil or the address is not found.
func LookupAddressHex(ds datastore.DataStore, key datastore.AddressRefKey, fallback string) (string, error) {
	if ds == nil {
		return fallback, nil
	}
	ref, err := ds.Addresses().Get(key)
	if err != nil {
		if errors.Is(err, datastore.ErrAddressRefNotFound) {
			return fallback, nil
		}
		return "", fmt.Errorf("look up address for type %s: %w", key.Type(), err)
	}
	if ref.Address == "" {
		return fallback, nil
	}
	return Base58ToHex(ref.Address)
}

// GetDeployedChainSelectors returns deduplicated chain selectors for all address refs
// matching the given filters. Useful for discovering which chains have a particular
// contract type deployed.
func GetDeployedChainSelectors(ds datastore.DataStore, filters ...datastore.FilterFunc[datastore.AddressRefKey, datastore.AddressRef]) []uint64 {
	if ds == nil {
		return nil
	}
	refs := ds.Addresses().Filter(filters...)
	seen := make(map[uint64]struct{}, len(refs))
	chains := make([]uint64, 0, len(refs))
	for _, ref := range refs {
		if _, exists := seen[ref.ChainSelector]; exists {
			continue
		}
		seen[ref.ChainSelector] = struct{}{}
		chains = append(chains, ref.ChainSelector)
	}
	return chains
}

// GetTokenMintAndTokenProgram resolves a token reference from the datastore and returns
// the full AddressRef and the corresponding SPL token program ID (SPL or SPL2022).
func GetTokenMintAndTokenProgram(store datastore.DataStore, tokenRef datastore.AddressRef, chain cldf_solana.Chain) (datastore.AddressRef, solana.PublicKey, error) {
	tokenAddr, err := FindLatestRef(store, tokenRef, chain.Selector)
	if err != nil {
		return datastore.AddressRef{}, solana.PublicKey{}, fmt.Errorf("failed to find token address for ref '%+v': %w", tokenRef, err)
	}
	tokenProgramID, err := GetTokenProgramID(cldf_deployment.ContractType(tokenAddr.Type))
	if err != nil {
		return datastore.AddressRef{}, solana.PublicKey{}, fmt.Errorf("failed to get token program ID for token type '%s': %w", tokenAddr.Type, err)
	}
	return tokenAddr, tokenProgramID, nil
}

// PoolRefByQualifier returns the unique pool address ref registered for the given
// pool type and qualifier. The ref's Version identifies which pool program serves
// the instance; unlike an address lookup this stays unambiguous when the pool
// program is registered under multiple versions (the deploy_chain_contracts
// double-registration HACK).
func PoolRefByQualifier(ds datastore.DataStore, chainSelector uint64, poolType, qualifier string) (datastore.AddressRef, error) {
	refs := ds.Addresses().Filter(
		datastore.AddressRefByChainSelector(chainSelector),
		datastore.AddressRefByType(datastore.ContractType(poolType)),
		datastore.AddressRefByQualifier(qualifier),
	)
	if len(refs) == 0 {
		return datastore.AddressRef{}, fmt.Errorf("%w: chain %d type %s qualifier %q", ErrAddressRefNotFound, chainSelector, poolType, qualifier)
	}
	if len(refs) > 1 {
		return datastore.AddressRef{}, fmt.Errorf("ambiguous pool refs for chain %d type %s qualifier %q: %d matches", chainSelector, poolType, qualifier, len(refs))
	}
	if refs[0].Version == nil {
		return datastore.AddressRef{}, fmt.Errorf("pool ref for chain %d type %s qualifier %q has no version", chainSelector, poolType, qualifier)
	}
	return refs[0], nil
}

// GetSolanaTokenPoolProgramAddress resolves the Solana token pool program address
func GetSolanaTokenPoolProgramAddress(
	ds datastore.DataStore,
	chainSelector uint64,
	poolType string,
	solanaTokenQualifier string,
) (solana.PublicKey, *semver.Version, error) {
	poolRef, err := PoolRefByQualifier(ds, chainSelector, poolType, solanaTokenQualifier)
	if err != nil {
		return solana.PublicKey{}, nil, err
	}
	pubkey, err := ToAddress(poolRef)
	if err != nil {
		return solana.PublicKey{}, nil, err
	}
	return pubkey, poolRef.Version, nil
}

// FindLatestRef resolves a token or token multisig ref without a version. These refs are the same for every
// CCIP version, so a lookup never names one: it matches on the chain, type and qualifier, ignores the version
// of ref, and returns the match with the highest version. Several matches at that version must share an
// address, otherwise the lookup is ambiguous.
func FindLatestRef(ds datastore.DataStore, ref datastore.AddressRef, chainSelector uint64) (datastore.AddressRef, error) {
	ref.ChainSelector = chainSelector
	ref.Version = nil
	return latestRef(ds.Addresses().Filter(datastore_utils.AddressRefToFilters(ref)...), ref)
}

// FindLatestRefInRefs is FindLatestRef over a slice of refs. An empty qualifier matches any.
func FindLatestRefInRefs(refs []datastore.AddressRef, chainSelector uint64, contractType datastore.ContractType, qualifier string) (datastore.AddressRef, error) {
	var matches []datastore.AddressRef
	for _, r := range refs {
		if r.ChainSelector != chainSelector || r.Type != contractType {
			continue
		}
		if qualifier != "" && r.Qualifier != qualifier {
			continue
		}
		matches = append(matches, r)
	}
	return latestRef(matches, datastore.AddressRef{ChainSelector: chainSelector, Type: contractType, Qualifier: qualifier})
}

func latestRef(matches []datastore.AddressRef, criteria datastore.AddressRef) (datastore.AddressRef, error) {
	if len(matches) == 0 {
		return datastore.AddressRef{}, fmt.Errorf("%w: %s", ErrAddressRefNotFound, datastore_utils.SprintRef(criteria))
	}
	latest := matches[0]
	for _, m := range matches[1:] {
		if m.Version != nil && (latest.Version == nil || m.Version.GreaterThan(latest.Version)) {
			latest = m
		}
	}
	for _, m := range matches {
		sameVersion := (m.Version == nil && latest.Version == nil) || (m.Version != nil && latest.Version != nil && m.Version.Equal(latest.Version))
		if sameVersion && m.Address != latest.Address {
			return datastore.AddressRef{}, fmt.Errorf("ambiguous ref %s: %s and %s are both at the latest version", datastore_utils.SprintRef(criteria), latest.Address, m.Address)
		}
	}
	return latest, nil
}
