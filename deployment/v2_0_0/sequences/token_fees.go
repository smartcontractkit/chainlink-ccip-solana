package sequences

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	"github.com/smartcontractkit/chainlink-ccip/deployment/finality"
	tokenapi "github.com/smartcontractkit/chainlink-ccip/deployment/tokens"
	common_utils "github.com/smartcontractkit/chainlink-ccip/deployment/utils"
	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/sequences"
	cldf_chain "github.com/smartcontractkit/chainlink-deployments-framework/chain"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	"github.com/smartcontractkit/chainlink-deployments-framework/deployment"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/burnmint_token_pool"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	tokenpoolops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/tokenpools"
)

var _ tokenapi.TokenFeeAdapter = (*SolanaAdapter)(nil)

// Solana token pools keep the allowed finality and the token transfer fee per lane, in the pool's
// ChainConfigV2 PDA. A pool program serves many mints, so every pool address taken by these methods
// is the pool config PDA (see DeriveTokenPoolCounterpart): its owner is the pool program and its
// state holds the mint.

// SetAllowedFinalityConfig sets a pool's allowed finality. EVM keeps it pool-wide; Solana keeps it
// per lane, so it is written to every remote chain the pool is configured for.
func (a *SolanaAdapter) SetAllowedFinalityConfig(e *deployment.Environment) *operations.Sequence[tokenapi.SetAllowedFinalityConfigSequenceInput, sequences.OnChainOutput, cldf_chain.BlockChains] {
	return operations.NewSequence(
		"solana-adapter:set-allowed-finality-config",
		solcommon.Version2_0_0,
		"Sets the allowed finality of Solana token pools on every configured lane",
		func(b operations.Bundle, chains cldf_chain.BlockChains, input tokenapi.SetAllowedFinalityConfigSequenceInput) (sequences.OnChainOutput, error) {
			chain, ok := chains.SolanaChains()[input.Selector]
			if !ok {
				return sequences.OnChainOutput{}, fmt.Errorf("solana chain with selector %d not defined", input.Selector)
			}
			var result sequences.OnChainOutput
			for _, poolAddress := range slices.Sorted(maps.Keys(input.Settings)) {
				pool, err := poolFromConfigPDA(b.GetContext(), chain, poolAddress)
				if err != nil {
					return sequences.OnChainOutput{}, err
				}
				op, err := chainConfigV2Op(pool.poolType(e, input.Selector))
				if err != nil {
					return sequences.OnChainOutput{}, err
				}
				remotes, err := a.GetSupportedChains(*e, input.Selector, pool.configPDA.Bytes(), pool.mint.Bytes())
				if err != nil {
					return sequences.OnChainOutput{}, fmt.Errorf("failed to list lanes of pool %s: %w", poolAddress, err)
				}
				cfg := toSolanaFinalityConfig(input.Settings[poolAddress])
				for _, remote := range remotes {
					out, err := operations.ExecuteOperation(b, op, chain, tokenpoolops.SetChainConfigV2Input{
						TokenPool:             pool.program,
						TokenMint:             pool.mint,
						RemoteChainSelector:   remote,
						AllowedFinalityConfig: &cfg,
					})
					if err != nil {
						return sequences.OnChainOutput{}, fmt.Errorf("failed to set allowed finality on pool %s for remote chain %d: %w", poolAddress, remote, err)
					}
					result.BatchOps = append(result.BatchOps, out.Output.BatchOps...)
				}
			}
			return result, nil
		},
	)
}

// SetTokenTransferFee sets a pool's per-lane token transfer fee. A nil config disables the fee.
func (a *SolanaAdapter) SetTokenTransferFee(e *deployment.Environment) *operations.Sequence[tokenapi.SetTokenTransferFeeSequenceInput, sequences.OnChainOutput, cldf_chain.BlockChains] {
	return operations.NewSequence(
		"solana-adapter:set-token-pool-transfer-fee",
		solcommon.Version2_0_0,
		"Sets the per-lane token transfer fee of Solana token pools",
		func(b operations.Bundle, chains cldf_chain.BlockChains, input tokenapi.SetTokenTransferFeeSequenceInput) (sequences.OnChainOutput, error) {
			chain, ok := chains.SolanaChains()[input.Selector]
			if !ok {
				return sequences.OnChainOutput{}, fmt.Errorf("solana chain with selector %d not defined", input.Selector)
			}
			var result sequences.OnChainOutput
			for _, poolAddress := range slices.Sorted(maps.Keys(input.Settings)) {
				pool, err := poolFromConfigPDA(b.GetContext(), chain, poolAddress)
				if err != nil {
					return sequences.OnChainOutput{}, err
				}
				op, err := chainConfigV2Op(pool.poolType(e, input.Selector))
				if err != nil {
					return sequences.OnChainOutput{}, err
				}
				dests := input.Settings[poolAddress]
				for _, dst := range slices.Sorted(maps.Keys(dests)) {
					fee := toSolanaTokenTransferFeeConfig(dests[dst])
					out, err := operations.ExecuteOperation(b, op, chain, tokenpoolops.SetChainConfigV2Input{
						TokenPool:              pool.program,
						TokenMint:              pool.mint,
						RemoteChainSelector:    dst,
						TokenTransferFeeConfig: &fee,
					})
					if err != nil {
						return sequences.OnChainOutput{}, fmt.Errorf("failed to set token transfer fee on pool %s for remote chain %d: %w", poolAddress, dst, err)
					}
					result.BatchOps = append(result.BatchOps, out.Output.BatchOps...)
				}
			}
			return result, nil
		},
	)
}

// GetOnchainTokenTransferFeeConfig reads a lane's token transfer fee. A lane without a v2 config
// returns the zero (disabled) config.
func (a *SolanaAdapter) GetOnchainTokenTransferFeeConfig(e deployment.Environment, poolAddress string, src uint64, dst uint64) (tokenapi.TokenTransferFeeConfig, error) {
	chain, ok := e.BlockChains.SolanaChains()[src]
	if !ok {
		return tokenapi.TokenTransferFeeConfig{}, fmt.Errorf("solana chain with selector %d not defined", src)
	}
	pool, err := poolFromConfigPDA(e.GetContext(), chain, poolAddress)
	if err != nil {
		return tokenapi.TokenTransferFeeConfig{}, err
	}
	cfg, found, err := tokenpoolops.GetChainConfigV2(e.OperationsBundle, chain, pool.program, pool.mint, dst)
	if err != nil || !found {
		return tokenapi.TokenTransferFeeConfig{}, err
	}
	fee := cfg.TokenTransferFeeConfig
	return tokenapi.TokenTransferFeeConfig{
		DefaultFinalityTransferFeeBps: fee.FinalityBpsFee,
		CustomFinalityTransferFeeBps:  fee.FastFinalityBpsFee,
		DefaultFinalityFeeUSDCents:    fee.FinalityFee.V0,
		CustomFinalityFeeUSDCents:     fee.FastFinalityFee.V0,
		DestBytesOverhead:             fee.DestBytesOverhead,
		DestGasOverhead:               fee.DestGasOverhead.V0,
		IsEnabled:                     fee.IsEnabled,
	}, nil
}

// GetDefaultTokenTransferFeeConfig returns the chain-agnostic default, the same one EVM pools use.
func (a *SolanaAdapter) GetDefaultTokenTransferFeeConfig(src uint64, dst uint64) tokenapi.TokenTransferFeeConfig {
	return tokenapi.GetDefaultChainAgnosticTokenTransferFeeConfig(src, dst)
}

type poolConfig struct {
	configPDA, program, mint solana.PublicKey
}

// poolFromConfigPDA resolves a pool config PDA into its pool program (the account owner) and mint.
func poolFromConfigPDA(ctx context.Context, chain cldf_solana.Chain, address string) (poolConfig, error) {
	pda, err := solana.PublicKeyFromBase58(address)
	if err != nil {
		return poolConfig{}, fmt.Errorf("invalid pool config address %q: %w", address, err)
	}
	acct, err := chain.Client.GetAccountInfoWithOpts(ctx, pda, &rpc.GetAccountInfoOpts{Commitment: cldf_solana.SolDefaultCommitment})
	if err != nil {
		return poolConfig{}, fmt.Errorf("failed to read pool config %s: %w", pda, err)
	}
	if acct == nil || acct.Value == nil {
		return poolConfig{}, fmt.Errorf("pool config %s not found", pda)
	}
	if acct.Value.Executable {
		return poolConfig{}, fmt.Errorf("%s is a program, expected the pool config PDA (derive it with DeriveTokenPoolCounterpart)", pda)
	}
	var st burnmint_token_pool.State
	if err := common.GetAccountDataBorshIntoAnchor(ctx, chain.Client, pda, cldf_solana.SolDefaultCommitment, &st); err != nil {
		return poolConfig{}, fmt.Errorf("failed to decode pool config %s: %w", pda, err)
	}
	return poolConfig{configPDA: pda, program: acct.Value.Owner, mint: st.Config.Mint}, nil
}

// poolType returns the datastore type of the pool program, or "" when it is not in the datastore.
func (p poolConfig) poolType(e *deployment.Environment, chainSelector uint64) string {
	if e == nil || e.DataStore == nil {
		return ""
	}
	ref, err := latestRefByAddress(e.DataStore, chainSelector, p.program.String())
	if err != nil {
		return ""
	}
	return ref.Type.String()
}

func chainConfigV2Op(poolType string) (*operations.Operation[tokenpoolops.SetChainConfigV2Input, sequences.OnChainOutput, cldf_solana.Chain], error) {
	switch poolType {
	case common_utils.BurnMintTokenPool.String():
		return tokenpoolops.SetChainConfigV2BurnMint, nil
	case common_utils.LockReleaseTokenPool.String():
		return tokenpoolops.SetChainConfigV2LockRelease, nil
	default:
		return nil, fmt.Errorf("unsupported token pool type '%s' for Solana token transfer fees", poolType)
	}
}

func toSolanaFinalityConfig(cfg finality.Config) burnmint_token_pool.FinalityConfig {
	raw := cfg.Raw()
	return burnmint_token_pool.FinalityConfig{
		Flags:      uint16(raw[0])<<8 | uint16(raw[1]),
		BlockDepth: uint16(raw[2])<<8 | uint16(raw[3]),
	}
}

// toSolanaTokenTransferFeeConfig maps the chain-agnostic fee; nil is the zero (disabled) config.
func toSolanaTokenTransferFeeConfig(cfg *tokenapi.TokenTransferFeeConfig) burnmint_token_pool.TokenTransferFeeConfig {
	if cfg == nil {
		return burnmint_token_pool.TokenTransferFeeConfig{}
	}
	return burnmint_token_pool.TokenTransferFeeConfig{
		DestGasOverhead:    burnmint_token_pool.CrossChainGas{V0: cfg.DestGasOverhead},
		DestBytesOverhead:  cfg.DestBytesOverhead,
		FinalityFee:        burnmint_token_pool.UsdCents{V0: cfg.DefaultFinalityFeeUSDCents},
		FastFinalityFee:    burnmint_token_pool.UsdCents{V0: cfg.CustomFinalityFeeUSDCents},
		FinalityBpsFee:     cfg.DefaultFinalityTransferFeeBps,
		FastFinalityBpsFee: cfg.CustomFinalityTransferFeeBps,
		IsEnabled:          cfg.IsEnabled,
	}
}
