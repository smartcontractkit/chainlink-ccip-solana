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

	feeqops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/feequoter"
)

type UpdateFeeQuoterDestChainConfigInput struct {
	DataStore            datastore.DataStore
	ChainSelector        uint64
	RemoteChainSelectors []uint64
	// A nil field keeps each lane's current value; a non-nil field replaces it.
	DestChainConfigUpdate feeqops.DestChainConfigUpdate
}

// UpdateFeeQuoterDestChainConfig changes selected dynamic fields of the FeeQuoter config of a
// Solana chain's existing 2.0 lanes, e.g. raising dest_gas_per_payload_byte_base when destination
// executions run out of gas. Everything the input leaves out stays as it is on chain.
var UpdateFeeQuoterDestChainConfig = cldf_ops.NewSequence(
	"fee-quoter:update-dest-chain-config",
	semver.MustParse("2.0.0"),
	"Sets selected dynamic fields of a Solana chain's 2.0 lane FeeQuoter configs",
	func(b cldf_ops.Bundle, chains cldf_chain.BlockChains, input UpdateFeeQuoterDestChainConfigInput) (seq_core.OnChainOutput, error) {
		var result seq_core.OnChainOutput
		chain, ok := chains.SolanaChains()[input.ChainSelector]
		if !ok {
			return result, fmt.Errorf("solana chain with selector %d not found", input.ChainSelector)
		}
		if len(input.RemoteChainSelectors) == 0 {
			return result, errors.New("at least one remote chain selector is required")
		}
		fqAddr, err := (&SolanaAdapter{}).GetFQAddress(input.DataStore, input.ChainSelector)
		if err != nil {
			return result, fmt.Errorf("failed to get FeeQuoter address: %w", err)
		}
		feeQuoter := solana.PublicKeyFromBytes(fqAddr)

		for _, remote := range input.RemoteChainSelectors {
			if err = runOp(b, feeqops.SetDestChainConfig, chain, feeqops.SetDestChainConfigParams{
				LocalChainSelector:    input.ChainSelector,
				FeeQuoter:             feeQuoter,
				RemoteChainSelector:   remote,
				DestChainConfigUpdate: input.DestChainConfigUpdate,
			}, &result.BatchOps); err != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("update config of lane to chain %d: %w", remote, err)
			}
		}
		return result, nil
	},
)
