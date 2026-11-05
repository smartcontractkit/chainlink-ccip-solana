package sequences

import (
	"errors"
	"fmt"

	"github.com/Masterminds/semver/v3"
	"github.com/gagliardetto/solana-go"

	seq_core "github.com/smartcontractkit/chainlink-ccip/deployment/utils/sequences"
	cldf_chain "github.com/smartcontractkit/chainlink-deployments-framework/chain"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	cldf_ops "github.com/smartcontractkit/chainlink-deployments-framework/operations"

	routerops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/router"
)

type SetLanesV2AllowlistInput struct {
	DataStore            datastore.DataStore
	ChainSelector        uint64
	RemoteChainSelectors []uint64
	// Enabled sets whether each lane's allowlist is enforced; a nil Enabled keeps its setting.
	Enabled *bool
	// AllowedSendersToAdd and AllowedSendersToRemove apply to each lane's current list.
	AllowedSendersToAdd    []solana.PublicKey
	AllowedSendersToRemove []solana.PublicKey
}

// SetLanesV2Allowlist sets the Router sender allowlist of existing v2 lanes from a Solana chain.
// It covers the allowlist side of a lane's lifecycle on 2.0: restricting it to the test wallet,
// promoting it by disabling the list, and rolling it back by enabling the list again. Lanes
// already in the requested state are skipped.
var SetLanesV2Allowlist = cldf_ops.NewSequence(
	"router:set-lanes-v2-allowlist",
	semver.MustParse("2.0.0"),
	"Sets the Router sender allowlist of a Solana chain's 2.0 lanes",
	func(b cldf_ops.Bundle, chains cldf_chain.BlockChains, input SetLanesV2AllowlistInput) (seq_core.OnChainOutput, error) {
		var result seq_core.OnChainOutput
		chain, ok := chains.SolanaChains()[input.ChainSelector]
		if !ok {
			return result, fmt.Errorf("solana chain with selector %d not found", input.ChainSelector)
		}
		if len(input.RemoteChainSelectors) == 0 {
			return result, errors.New("at least one remote chain selector is required")
		}
		routerAddr, err := (&SolanaAdapter{}).GetRouterAddress(input.DataStore, input.ChainSelector)
		if err != nil {
			return result, fmt.Errorf("failed to get Router address: %w", err)
		}
		router := solana.PublicKeyFromBytes(routerAddr)

		for _, remote := range input.RemoteChainSelectors {
			if err = runOp(b, routerops.SetDestChainAllowlist, chain, routerops.SetDestChainAllowlistParams{
				LocalChainSelector:     input.ChainSelector,
				Router:                 router,
				RemoteChainSelector:    remote,
				Enabled:                input.Enabled,
				AllowedSendersToAdd:    input.AllowedSendersToAdd,
				AllowedSendersToRemove: input.AllowedSendersToRemove,
			}, &result.BatchOps); err != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("set allowlist of lane to chain %d: %w", remote, err)
			}
		}
		return result, nil
	},
)
