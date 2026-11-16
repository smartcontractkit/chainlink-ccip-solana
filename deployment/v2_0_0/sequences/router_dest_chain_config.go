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

type UpdateDestChainConfigInput struct {
	DataStore            datastore.DataStore
	ChainSelector        uint64
	RemoteChainSelectors []uint64
	// A nil field keeps each lane's current value; a non-nil field replaces it.
	BaseExecutionGasCost            *uint32
	MessageNetworkFeeUSDCents       *uint16
	TokenTransferNetworkFeeUSDCents *uint16
	MaxFeePerMessageUSDCents        *uint16
	TokenReceiverAllowed            *bool
}

// UpdateDestChainConfig changes selected fields of the Router config of a Solana chain's
// existing 2.0 lanes, e.g. raising base_execution_gas_cost when executions run out of gas.
// Everything the input leaves out stays as it is on chain.
var UpdateDestChainConfig = cldf_ops.NewSequence(
	"router:update-dest-chain-config",
	semver.MustParse("2.0.0"),
	"Sets selected fields of a Solana chain's 2.0 lane configs on the Router",
	func(b cldf_ops.Bundle, chains cldf_chain.BlockChains, input UpdateDestChainConfigInput) (seq_core.OnChainOutput, error) {
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
			if err = runOp(b, routerops.SetDestChainConfig, chain, routerops.SetDestChainConfigParams{
				LocalChainSelector:              input.ChainSelector,
				Router:                          router,
				RemoteChainSelector:             remote,
				BaseExecutionGasCost:            input.BaseExecutionGasCost,
				MessageNetworkFeeUSDCents:       input.MessageNetworkFeeUSDCents,
				TokenTransferNetworkFeeUSDCents: input.TokenTransferNetworkFeeUSDCents,
				MaxFeePerMessageUSDCents:        input.MaxFeePerMessageUSDCents,
				TokenReceiverAllowed:            input.TokenReceiverAllowed,
			}, &result.BatchOps); err != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("update config of lane to chain %d: %w", remote, err)
			}
		}
		return result, nil
	},
)
