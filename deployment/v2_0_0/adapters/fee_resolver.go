package adapters

import (
	"fmt"

	"github.com/smartcontractkit/chainlink-ccip/deployment/fees"
	datastore_utils "github.com/smartcontractkit/chainlink-ccip/deployment/utils/datastore"
	cldf_chain "github.com/smartcontractkit/chainlink-deployments-framework/chain"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations"

	routerops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/router"
)

var _ fees.FeeResolver = (*SolanaFeeResolver)(nil)

// SolanaFeeResolver resolves the OnRamp of a Solana source chain. On Solana the Router is the OnRamp.
type SolanaFeeResolver struct{}

func (r *SolanaFeeResolver) GetOnRampRef(_ operations.Bundle, chains cldf_chain.BlockChains, ds datastore.DataStore, src uint64, dst uint64) (datastore.AddressRef, error) {
	if _, ok := chains.SolanaChains()[src]; !ok {
		return datastore.AddressRef{}, fmt.Errorf("solana chain not found for selector %d", src)
	}
	routerRef, err := datastore_utils.FindAndFormatRef(ds, datastore.AddressRef{
		Type:          datastore.ContractType(routerops.ContractType),
		Version:       routerops.Version,
		ChainSelector: src,
	}, src, datastore_utils.FullRef)
	if err != nil {
		return datastore.AddressRef{}, fmt.Errorf("failed to get Router address ref for src %d and dst %d: %w", src, dst, err)
	}
	return routerRef, nil
}
