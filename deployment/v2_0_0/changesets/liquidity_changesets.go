package changesets

import (
	"errors"
	"fmt"

	"github.com/gagliardetto/solana-go"

	cldf "github.com/smartcontractkit/chainlink-deployments-framework/deployment"
	cldf_ops "github.com/smartcontractkit/chainlink-deployments-framework/operations"

	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/sequences"
)

type ProvideLockReleaseLiquidityChangesetInput struct {
	ChainSelector uint64 `json:"chainSelector"`
	TokenMint     string `json:"tokenMint"`
	PoolProgram   string `json:"poolProgram"`
	Rebalancer    string `json:"rebalancer,omitempty"`
	Amount        uint64 `json:"amount"`
}

// TEST/controlled environment only — operational tooling, not a protocol
// contract. Deposits deployer-held tokens into a LockRelease pool so destination
// releases succeed. Rebalancer-gated: the on-chain rebalancer must be the CLD
// deployer (see operations/tokenpools.ProvideLiquidityLockRelease).
func ProvideLockReleaseLiquidityChangeset() cldf.ChangeSetV2[ProvideLockReleaseLiquidityChangesetInput] {
	return cldf.CreateChangeSet(provideLockReleaseLiquidityApply, provideLockReleaseLiquidityVerify)
}

func provideLockReleaseLiquidityVerify(env cldf.Environment, input ProvideLockReleaseLiquidityChangesetInput) error {
	if err := cldf.IsValidChainSelector(input.ChainSelector); err != nil {
		return fmt.Errorf("invalid chain selector %d: %w", input.ChainSelector, err)
	}
	if !env.BlockChains.Exists(input.ChainSelector) {
		return fmt.Errorf("chain with selector %d does not exist", input.ChainSelector)
	}
	if _, ok := env.BlockChains.SolanaChains()[input.ChainSelector]; !ok {
		return fmt.Errorf("chain with selector %d is not a Solana chain", input.ChainSelector)
	}
	if input.Amount == 0 {
		return errors.New("amount must be non-zero")
	}
	if _, err := solana.PublicKeyFromBase58(input.TokenMint); err != nil {
		return fmt.Errorf("invalid tokenMint %q: %w", input.TokenMint, err)
	}
	if input.PoolProgram != "" {
		if _, err := solana.PublicKeyFromBase58(input.PoolProgram); err != nil {
			return fmt.Errorf("invalid poolProgram %q: %w", input.PoolProgram, err)
		}
	}
	if input.Rebalancer != "" {
		if _, err := solana.PublicKeyFromBase58(input.Rebalancer); err != nil {
			return fmt.Errorf("invalid rebalancer %q: %w", input.Rebalancer, err)
		}
	}
	return nil
}

func provideLockReleaseLiquidityApply(e cldf.Environment, input ProvideLockReleaseLiquidityChangesetInput) (cldf.ChangesetOutput, error) {
	rebalancer := solana.PublicKey{}
	if input.Rebalancer != "" {
		rebalancer = solana.MustPublicKeyFromBase58(input.Rebalancer)
	}
	report, err := cldf_ops.ExecuteSequence(e.OperationsBundle, sequences.ProvideLiquidityLockRelease, e.BlockChains, sequences.ProvideLiquidityInput{
		ChainSelector: input.ChainSelector,
		Mint:          solana.MustPublicKeyFromBase58(input.TokenMint),
		PoolProgram:   solana.MustPublicKeyFromBase58(input.PoolProgram),
		Rebalancer:    rebalancer,
		Amount:        input.Amount,
	})
	if err != nil {
		return cldf.ChangesetOutput{}, fmt.Errorf("provide lock-release liquidity: %w", err)
	}
	return cldf.ChangesetOutput{
		Reports: append([]cldf_ops.Report[any, any](nil), report.ExecutionReports...),
	}, nil
}
