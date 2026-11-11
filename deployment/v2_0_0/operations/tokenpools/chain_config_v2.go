package tokenpools

import (
	"context"
	"errors"
	"fmt"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	common_utils "github.com/smartcontractkit/chainlink-ccip/deployment/utils"
	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/sequences"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/burnmint_token_pool"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/lockrelease_token_pool"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/tokens"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
)

// SetChainConfigV2Input updates a lane's CCIP 2.0 pool config. Nil fields keep their current
// on-chain value; the lane's CCV config is always kept, since it is owned by the lane flow.
type SetChainConfigV2Input struct {
	TokenPool              solana.PublicKey
	TokenMint              solana.PublicKey
	RemoteChainSelector    uint64
	AllowedFinalityConfig  *burnmint_token_pool.FinalityConfig
	TokenTransferFeeConfig *burnmint_token_pool.TokenTransferFeeConfig
}

// GetChainConfigV2 reads a pool's CCIP 2.0 lane config. BurnMint and LockRelease share the layout.
// found is false when the lane has no v2 config yet.
func GetChainConfigV2(b operations.Bundle, chain cldf_solana.Chain, tokenPool, tokenMint solana.PublicKey, remoteSelector uint64) (cfg burnmint_token_pool.ChainConfigV2, found bool, err error) {
	pda, _, err := tokens.TokenPoolChainConfigV2PDA(remoteSelector, tokenMint, tokenPool)
	if err != nil {
		return cfg, false, fmt.Errorf("failed to derive chain config v2 PDA: %w", err)
	}
	err = common.GetAccountDataBorshIntoAnchor(b.GetContext(), chain.Client, pda, cldf_solana.SolDefaultCommitment, &cfg)
	if errors.Is(err, rpc.ErrNotFound) {
		return cfg, false, nil
	}
	if err != nil {
		return cfg, false, fmt.Errorf("failed to decode chain config v2 at PDA %s for remote %d: %w", pda, remoteSelector, err)
	}
	return cfg, true, nil
}

type chainConfigV2Accounts struct {
	poolConfig, chainConfigV2, authority, mint, feeVault, feeVaultTokenAccount, tokenProgram, eventAuthority, program solana.PublicKey
}

func setChainConfigV2(
	b operations.Bundle,
	chain cldf_solana.Chain,
	input SetChainConfigV2Input,
	poolType string,
	getAuthority func(context.Context, cldf_solana.Chain, solana.PublicKey, solana.PublicKey) (solana.PublicKey, error),
	buildInit func(burnmint_token_pool.SetChainConfigV2Params, chainConfigV2Accounts) (solana.Instruction, error),
	buildEdit func(burnmint_token_pool.SetChainConfigV2Params, chainConfigV2Accounts) (solana.Instruction, error),
) (sequences.OnChainOutput, error) {
	current, found, err := GetChainConfigV2(b, chain, input.TokenPool, input.TokenMint, input.RemoteChainSelector)
	if err != nil {
		return sequences.OnChainOutput{}, err
	}
	params := burnmint_token_pool.SetChainConfigV2Params{
		Mint:                   input.TokenMint,
		RemoteChainSelector:    input.RemoteChainSelector,
		AllowedFinalityConfig:  current.AllowedFinalityConfig,
		TokenTransferFeeConfig: current.TokenTransferFeeConfig,
		CcvConfig:              current.CcvConfig,
	}
	if input.AllowedFinalityConfig != nil {
		params.AllowedFinalityConfig = *input.AllowedFinalityConfig
	}
	if input.TokenTransferFeeConfig != nil {
		params.TokenTransferFeeConfig = *input.TokenTransferFeeConfig
	}
	if found && params.AllowedFinalityConfig == current.AllowedFinalityConfig && params.TokenTransferFeeConfig == current.TokenTransferFeeConfig {
		b.Logger.Infow("Token pool v2 chain config already set, skipping", "tokenPool", input.TokenPool.String(), "remoteChainSelector", input.RemoteChainSelector)
		return sequences.OnChainOutput{}, nil
	}

	var poolState burnmint_token_pool.State
	accts := chainConfigV2Accounts{mint: input.TokenMint, program: input.TokenPool}
	if accts.poolConfig, err = tokens.TokenPoolConfigAddress(input.TokenMint, input.TokenPool); err != nil {
		return sequences.OnChainOutput{}, fmt.Errorf("failed to derive token pool config address: %w", err)
	}
	if err = common.GetAccountDataBorshIntoAnchor(b.GetContext(), chain.Client, accts.poolConfig, cldf_solana.SolDefaultCommitment, &poolState); err != nil {
		return sequences.OnChainOutput{}, fmt.Errorf("failed to read pool config %s: %w", accts.poolConfig, err)
	}
	accts.tokenProgram = poolState.Config.TokenProgram
	if accts.chainConfigV2, _, err = tokens.TokenPoolChainConfigV2PDA(input.RemoteChainSelector, input.TokenMint, input.TokenPool); err != nil {
		return sequences.OnChainOutput{}, fmt.Errorf("failed to derive chain config v2 PDA: %w", err)
	}
	if accts.feeVault, _, err = state.FindFeeVaultPDA(input.TokenPool, input.TokenMint.Bytes()); err != nil {
		return sequences.OnChainOutput{}, fmt.Errorf("failed to derive fee vault PDA: %w", err)
	}
	if accts.feeVaultTokenAccount, _, err = tokens.FindAssociatedTokenAddress(accts.tokenProgram, input.TokenMint, accts.feeVault); err != nil {
		return sequences.OnChainOutput{}, fmt.Errorf("failed to derive fee vault token account: %w", err)
	}
	if accts.eventAuthority, _, err = state.FindEventAuthorityPDA(input.TokenPool); err != nil {
		return sequences.OnChainOutput{}, fmt.Errorf("failed to derive event authority PDA: %w", err)
	}
	if accts.authority, err = getAuthority(b.GetContext(), chain, input.TokenPool, input.TokenMint); err != nil {
		return sequences.OnChainOutput{}, fmt.Errorf("failed to get token pool owner: %w", err)
	}

	var ixn solana.Instruction
	if found {
		ixn, err = buildEdit(params, accts)
	} else {
		// init also creates the fee vault ATA, paid by the pool owner.
		ixn, err = buildInit(params, accts)
	}
	if err != nil {
		return sequences.OnChainOutput{}, fmt.Errorf("failed to build chain config v2 instruction: %w", err)
	}
	return solcommon.ConfirmOrBatch(chain, accts.authority, []solana.Instruction{ixn}, input.TokenPool, poolType)
}

var SetChainConfigV2BurnMint = operations.NewOperation(
	"burnmint:set-chain-config-v2",
	solcommon.Version2_0_0,
	"Sets the allowed finality and token transfer fee of a BurnMintTokenPool lane, keeping its CCV config",
	func(b operations.Bundle, chain cldf_solana.Chain, input SetChainConfigV2Input) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&burnmint_token_pool.ProgramID, burnmint_token_pool.ParseInstructionWithoutAccounts, input.TokenPool)
		return setChainConfigV2(b, chain, input, common_utils.BurnMintTokenPool.String(), GetAuthorityBurnMint,
			func(p burnmint_token_pool.SetChainConfigV2Params, a chainConfigV2Accounts) (solana.Instruction, error) {
				return burnmint_token_pool.NewInitChainConfigV2Instruction(p, a.poolConfig, a.chainConfigV2, a.authority, solana.SystemProgramID,
					a.mint, a.feeVault, a.feeVaultTokenAccount, a.tokenProgram, solana.SPLAssociatedTokenAccountProgramID, a.eventAuthority, a.program)
			},
			func(p burnmint_token_pool.SetChainConfigV2Params, a chainConfigV2Accounts) (solana.Instruction, error) {
				return burnmint_token_pool.NewEditChainConfigV2Instruction(p, a.poolConfig, a.chainConfigV2, a.authority, solana.SystemProgramID, a.eventAuthority, a.program)
			},
		)
	},
)

var SetChainConfigV2LockRelease = operations.NewOperation(
	"lockrelease:set-chain-config-v2",
	solcommon.Version2_0_0,
	"Sets the allowed finality and token transfer fee of a LockReleaseTokenPool lane, keeping its CCV config",
	func(b operations.Bundle, chain cldf_solana.Chain, input SetChainConfigV2Input) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&lockrelease_token_pool.ProgramID, lockrelease_token_pool.ParseInstructionWithoutAccounts, input.TokenPool)
		return setChainConfigV2(b, chain, input, common_utils.LockReleaseTokenPool.String(), GetAuthorityLockRelease,
			func(p burnmint_token_pool.SetChainConfigV2Params, a chainConfigV2Accounts) (solana.Instruction, error) {
				return lockrelease_token_pool.NewInitChainConfigV2Instruction(toLockReleaseChainConfigV2Params(p), a.poolConfig, a.chainConfigV2, a.authority, solana.SystemProgramID,
					a.mint, a.feeVault, a.feeVaultTokenAccount, a.tokenProgram, solana.SPLAssociatedTokenAccountProgramID, a.eventAuthority, a.program)
			},
			func(p burnmint_token_pool.SetChainConfigV2Params, a chainConfigV2Accounts) (solana.Instruction, error) {
				return lockrelease_token_pool.NewEditChainConfigV2Instruction(toLockReleaseChainConfigV2Params(p), a.poolConfig, a.chainConfigV2, a.authority, solana.SystemProgramID, a.eventAuthority, a.program)
			},
		)
	},
)

func toLockReleaseChainConfigV2Params(p burnmint_token_pool.SetChainConfigV2Params) lockrelease_token_pool.SetChainConfigV2Params {
	fee := p.TokenTransferFeeConfig
	return lockrelease_token_pool.SetChainConfigV2Params{
		Mint:                  p.Mint,
		RemoteChainSelector:   p.RemoteChainSelector,
		AllowedFinalityConfig: lockrelease_token_pool.FinalityConfig(p.AllowedFinalityConfig),
		TokenTransferFeeConfig: lockrelease_token_pool.TokenTransferFeeConfig{
			DestGasOverhead:    lockrelease_token_pool.CrossChainGas(fee.DestGasOverhead),
			DestBytesOverhead:  fee.DestBytesOverhead,
			FinalityFee:        lockrelease_token_pool.UsdCents(fee.FinalityFee),
			FastFinalityFee:    lockrelease_token_pool.UsdCents(fee.FastFinalityFee),
			FinalityBpsFee:     fee.FinalityBpsFee,
			FastFinalityBpsFee: fee.FastFinalityBpsFee,
			IsEnabled:          fee.IsEnabled,
		},
		CcvConfig: lockrelease_token_pool.CcvConfig(p.CcvConfig),
	}
}
