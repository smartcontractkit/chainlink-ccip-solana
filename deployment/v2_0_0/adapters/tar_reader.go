package adapters

import (
	"errors"
	"fmt"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	tokensapi "github.com/smartcontractkit/chainlink-ccip/deployment/tokens"
	datastore_utils "github.com/smartcontractkit/chainlink-ccip/deployment/utils/datastore"
	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/sequences"
	cldf_chain "github.com/smartcontractkit/chainlink-deployments-framework/chain"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	"github.com/smartcontractkit/chainlink-deployments-framework/deployment"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	routerops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/router"
)

var _ tokensapi.TokenAdminRegistryManager = (*SolanaAdminRegistryManager)(nil)

// tarPoolLookupTableIndex is the position of the pool program in a token's TAR lookup table.
const tarPoolLookupTableIndex = 2

// SolanaAdminRegistryManager reads and clears a token's pool in the Router's TokenAdminRegistry PDA.
type SolanaAdminRegistryManager struct{}

// UnregisterToken clears the token's pool lookup table. The caller checks that the active pool is
// the one being removed before running it.
func (r *SolanaAdminRegistryManager) UnregisterToken() *operations.Sequence[tokensapi.UnregisterTokenSequenceInput, sequences.OnChainOutput, cldf_chain.BlockChains] {
	return operations.NewSequence(
		"solana-admin-registry:unregister-token",
		solcommon.Version2_0_0,
		"Unregisters a token from the Router TokenAdminRegistry by setting its pool lookup table to zero",
		func(b operations.Bundle, chains cldf_chain.BlockChains, input tokensapi.UnregisterTokenSequenceInput) (sequences.OnChainOutput, error) {
			chain, ok := chains.SolanaChains()[input.Selector]
			if !ok {
				return sequences.OnChainOutput{}, fmt.Errorf("solana chain with selector %d not defined", input.Selector)
			}
			env := deployment.Environment{DataStore: input.ExistingDataStore, BlockChains: chains}
			router, err := r.resolveRouter(env, input.Selector, []datastore.AddressRef{input.RegistryRef})
			if err != nil {
				return sequences.OnChainOutput{}, err
			}
			mint, err := solana.PublicKeyFromBase58(input.TokenRef.Address)
			if err != nil {
				return sequences.OnChainOutput{}, fmt.Errorf("invalid token mint address %q on chain %d: %w", input.TokenRef.Address, input.Selector, err)
			}
			out, err := operations.ExecuteOperation(b, routerops.UnregisterToken, chain, routerops.UnregisterTokenParams{
				Router:    router,
				TokenMint: mint,
			})
			if err != nil {
				return sequences.OnChainOutput{}, fmt.Errorf("failed to unregister token %s on chain %d: %w", mint, input.Selector, err)
			}
			return out.Output, nil
		},
	)
}

func (r *SolanaAdminRegistryManager) GetActivePool(e deployment.Environment, chainSelector uint64, tokenRef datastore.AddressRef, overrides ...datastore.AddressRef) ([]byte, error) {
	chain, ok := e.BlockChains.SolanaChains()[chainSelector]
	if !ok {
		return nil, fmt.Errorf("chain with selector %d not found", chainSelector)
	}

	tokenAddress := tokenRef.Address
	if tokenAddress == "" {
		ref, err := solcommon.FindLatestRef(e.DataStore, tokenRef, chainSelector)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve token ref for chain %d: %w", chainSelector, err)
		}
		tokenAddress = ref.Address
	}
	mint, err := solana.PublicKeyFromBase58(tokenAddress)
	if err != nil {
		return nil, fmt.Errorf("invalid token mint address %q: %w", tokenAddress, err)
	}

	router, err := r.resolveRouter(e, chainSelector, overrides)
	if err != nil {
		return nil, err
	}
	tarPDA, _, err := state.FindTokenAdminRegistryPDA(mint, router)
	if err != nil {
		return nil, fmt.Errorf("failed to derive TAR PDA: %w", err)
	}

	// A missing TAR account means the token was never registered (no active pool). Any other
	// failure must surface as an error, since callers act on the "no pool" answer.
	ctx := e.GetContext()
	tarAccount, _, err := routerops.ReadTokenAdminRegistry(ctx, chain.Client, tarPDA)
	if err != nil {
		if errors.Is(err, rpc.ErrNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read token admin registry %s for mint %s on chain %d: %w", tarPDA, mint, chainSelector, err)
	}
	if tarAccount.LookupTable.IsZero() {
		return nil, nil
	}

	// A non-zero lookup table means a pool is registered: the router's set_pool only accepts an
	// existing table holding the pool at index 2. A missing or short table is a broken
	// registration, so report it instead of answering "no pool".
	entries, err := common.GetAddressLookupTable(ctx, chain.Client, tarAccount.LookupTable)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch lookup table %s registered for mint %s on chain %d: %w", tarAccount.LookupTable, mint, chainSelector, err)
	}
	if len(entries) <= tarPoolLookupTableIndex {
		return nil, fmt.Errorf("lookup table %s registered for mint %s on chain %d has %d entries; expected the pool at index %d",
			tarAccount.LookupTable, mint, chainSelector, len(entries), tarPoolLookupTableIndex)
	}
	return entries[tarPoolLookupTableIndex].Bytes(), nil
}

// GetTokenAdminRegistryRef returns the Router ref: on Solana the TokenAdminRegistry lives in Router PDAs.
func (r *SolanaAdminRegistryManager) GetTokenAdminRegistryRef(e deployment.Environment, chainSelector uint64) (datastore.AddressRef, error) {
	if _, ok := e.BlockChains.SolanaChains()[chainSelector]; !ok {
		return datastore.AddressRef{}, fmt.Errorf("chain with selector %d not found", chainSelector)
	}
	refs := e.DataStore.Addresses().Filter(
		datastore.AddressRefByChainSelector(chainSelector),
		datastore.AddressRefByType(datastore.ContractType(routerops.ContractType)),
		datastore.AddressRefByVersion(routerops.Version),
	)
	if len(refs) == 0 {
		return datastore.AddressRef{}, fmt.Errorf("router not found in datastore for chain %d", chainSelector)
	}
	return refs[0], nil
}

// resolveRouter returns the first override that resolves from the datastore, or the default Router.
func (r *SolanaAdminRegistryManager) resolveRouter(e deployment.Environment, chainSelector uint64, overrides []datastore.AddressRef) (solana.PublicKey, error) {
	for _, override := range overrides {
		if datastore_utils.IsAddressRefEmpty(override) {
			continue
		}
		ref, err := solcommon.FindAndFormatCompatibleRef(e.DataStore, override, chainSelector, datastore_utils.FullRef)
		if err != nil || ref.Address == "" {
			continue
		}
		router, err := solana.PublicKeyFromBase58(ref.Address)
		if err != nil {
			return solana.PublicKey{}, fmt.Errorf("invalid router address %q: %w", ref.Address, err)
		}
		return router, nil
	}
	routerRef, err := r.GetTokenAdminRegistryRef(e, chainSelector)
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("failed to resolve Router on chain %d: %w", chainSelector, err)
	}
	router, err := solana.PublicKeyFromBase58(routerRef.Address)
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("invalid router address %q: %w", routerRef.Address, err)
	}
	return router, nil
}
