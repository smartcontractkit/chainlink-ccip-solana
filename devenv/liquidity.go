package devenv

import (
	"context"
	"fmt"

	"github.com/gagliardetto/solana-go"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	cldf_ops "github.com/smartcontractkit/chainlink-deployments-framework/operations"

	lockrelease "github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/lockrelease_token_pool"
	soltokens "github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/tokens"
	tokenpoolops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/tokenpools"
)

// solanaLockReleaseLiquidity is the liquidity funded into each lock-release pool,
// in base units (100 whole tokens at SolanaDefaultDecimals).
const solanaLockReleaseLiquidity = 100_000_000

// provideLockReleaseLiquidity funds a lock-release pool during a fresh devenv
// deploy. The mint step stays local because fresh deploys have no supply and the
// deployer is the LnR mint authority (a shared mint op is deliberately not part
// of the CLD ops surface: it cannot mint BurnMint tokens, whose authority moves
// to the pool signer). Everything after the mint — rebalancer + approve +
// provide_liquidity — delegates to the shared test op
// tokenpools.ProvideLiquidityLockRelease (TEST/controlled environment only).
func provideLockReleaseLiquidity(
	ctx context.Context,
	chain cldf_solana.Chain,
	mint solana.PublicKey,
	amount uint64,
) error {
	deployer := chain.DeployerKey.PublicKey()
	tokenProgram := solana.TokenProgramID

	// Mint the liquidity source to the deployer (LnR mint authority). The
	// deployer ATA already exists on a fresh deploy: DeployToken premints to it
	// (GetTokenExpansionConfigs sets PreMint + Senders=[deployer]). Do NOT
	// existence-check + create it here
	ata, _, err := soltokens.FindAssociatedTokenAddress(tokenProgram, mint, deployer)
	if err != nil {
		return fmt.Errorf("derive deployer token account: %w", err)
	}
	mintIx, err := soltokens.MintTo(amount, tokenProgram, mint, ata, deployer)
	if err != nil {
		return fmt.Errorf("build mint instruction: %w", err)
	}
	if err := chain.Confirm([]solana.Instruction{mintIx}); err != nil {
		return fmt.Errorf("mint liquidity source: %w", err)
	}

	// Set the deployer as rebalancer (idempotent; skipped if already set) and
	// deposit liquidity through the shared CLD op.
	bundle := cldf_ops.NewBundle(func() context.Context { return ctx }, logger.Nop(), cldf_ops.NewMemoryReporter())
	if _, err := cldf_ops.ExecuteOperation(bundle, tokenpoolops.ProvideLiquidityLockRelease, chain, tokenpoolops.ProvideLiquidityInput{
		Program:    lockrelease.ProgramID,
		TokenMint:  mint,
		Rebalancer: deployer,
		Amount:     amount,
	}); err != nil {
		return fmt.Errorf("provide liquidity for mint %s: %w", mint, err)
	}
	return nil
}
