package sequences

import (
	"fmt"

	"github.com/gagliardetto/solana-go"

	seq_core "github.com/smartcontractkit/chainlink-ccip/deployment/utils/sequences"
	cldf_chain "github.com/smartcontractkit/chainlink-deployments-framework/chain"
	cldf_ops "github.com/smartcontractkit/chainlink-deployments-framework/operations"

	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	tokenpoolops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/tokenpools"
)

// ProvideLiquidityInput binds the provide-liquidity op to a Solana chain.
type ProvideLiquidityInput struct {
	ChainSelector uint64
	Mint          solana.PublicKey
	PoolProgram   solana.PublicKey
	Rebalancer    solana.PublicKey
	Amount        uint64
}

// TEST/controlled environment only — see operations/tokenpools.ProvideLiquidityLockRelease.
var ProvideLiquidityLockRelease = cldf_ops.NewSequence(
	"lockrelease:provide-liquidity",
	solcommon.Version2_0_0,
	"TEST/controlled environment only: deposit deployer-held tokens into a LockRelease pool",
	func(b cldf_ops.Bundle, chains cldf_chain.BlockChains, input ProvideLiquidityInput) (seq_core.OnChainOutput, error) {
		chain, ok := chains.SolanaChains()[input.ChainSelector]
		if !ok {
			return seq_core.OnChainOutput{}, fmt.Errorf("solana chain with selector %d not found", input.ChainSelector)
		}
		out, err := cldf_ops.ExecuteOperation(b, tokenpoolops.ProvideLiquidityLockRelease, chain, tokenpoolops.ProvideLiquidityInput{
			Program:    input.PoolProgram,
			TokenMint:  input.Mint,
			Rebalancer: input.Rebalancer,
			Amount:     input.Amount,
		})
		if err != nil {
			return seq_core.OnChainOutput{}, err
		}
		return out.Output, nil
	},
)
