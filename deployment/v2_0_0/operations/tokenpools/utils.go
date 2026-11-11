package tokenpools

import (
	"bytes"
	"context"
	"fmt"

	"github.com/gagliardetto/solana-go"

	token_deployments "github.com/smartcontractkit/chainlink-ccip/deployment/tokens"
	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/sequences"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/burnmint_token_pool"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/tokens"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
)

// poolDiff returns elements in newAddrs that don't exist in existingAddrs, compared by their byte address.
func poolDiff[T any](existingAddrs, newAddrs []T, getAddress func(T) []byte) []T {
	var result []T
	for _, n := range newAddrs {
		exists := false
		for _, e := range existingAddrs {
			if bytes.Equal(getAddress(e), getAddress(n)) {
				exists = true
				break
			}
		}
		if !exists {
			result = append(result, n)
		}
	}
	return result
}

type TokenPoolTransferOwnershipInput struct {
	Program   solana.PublicKey
	NewOwner  solana.PublicKey
	TokenMint solana.PublicKey
}

type TokenPoolSetRebalancerInput struct {
	Program    solana.PublicKey
	TokenMint  solana.PublicKey
	Rebalancer solana.PublicKey
}

type Params struct {
	TokenPool solana.PublicKey
	TokenMint solana.PublicKey
	// SPLToken or SPLToken2022
	TokenProgramID solana.PublicKey
	// Only used for certain ops
	RMNRemote        solana.PublicKey
	Router           solana.PublicKey
	NewMintAuthority solana.PublicKey
	OldMintAuthority solana.PublicKey
}

type RemoteChainConfig struct {
	TokenPool solana.PublicKey
	TokenMint solana.PublicKey
	// SPLToken or SPLToken2022
	TokenProgramID            solana.PublicKey
	RemoteSelector            uint64
	RemoteTokenAddress        []byte
	RemotePoolAddress         []byte
	RemoteDecimals            uint8
	ForceOverrideRemoteConfig bool
	InboundRateLimiterConfig  token_deployments.RateLimiterConfig
	OutboundRateLimiterConfig token_deployments.RateLimiterConfig
}

type initGlobalCfgParams struct {
	PoolTypeLabel string // e.g. common_utils.BurnMintTokenPool.String()
	LogName       string // e.g. "BurnMintTokenPool"
	SetProgramID  func(solana.PublicKey)
	BuildInitIx   func(configPDA solana.PublicKey, upgradeAuthority solana.PublicKey, programData solana.PublicKey) (solana.Instruction, error)
}

// initGlobalConfigTokenPool initializes the token pool global config if not initialized.
// If upgradeAuthority != deployer => produces MCMS batch op, otherwise sends tx directly.
func initGlobalConfigTokenPool(
	b operations.Bundle,
	chain cldf_solana.Chain,
	input Params,
	p initGlobalCfgParams,
) (sequences.OnChainOutput, error) {
	programData, err := solcommon.GetSolProgramData(chain.Client, input.TokenPool)
	if err != nil {
		return sequences.OnChainOutput{}, err
	}

	upgradeAuthority, err := solcommon.GetUpgradeAuthority(chain.Client, input.TokenPool)
	if err != nil {
		return sequences.OnChainOutput{}, err
	}

	configPDA, _, err := state.FindConfigPDA(input.TokenPool)
	if err != nil {
		return sequences.OnChainOutput{}, fmt.Errorf("failed to derive config PDA: %w", err)
	}

	// The PoolConfig discriminator is the same for every pool program.
	initialized, err := common.IsAccountInitialized(b.GetContext(), chain.Client, configPDA, input.TokenPool, cldf_solana.SolDefaultCommitment, burnmint_token_pool.Account_PoolConfig)
	if err != nil {
		return sequences.OnChainOutput{}, err
	}
	if initialized {
		b.Logger.Info(p.LogName+" global config already initialized for token pool:", input.TokenPool.String())
		return sequences.OnChainOutput{}, nil
	}

	ixn, err := p.BuildInitIx(configPDA, upgradeAuthority, programData.Address)
	if err != nil {
		return sequences.OnChainOutput{}, err
	}

	// If deployer isn't upgrade authority, create MCMS batch op.
	return solcommon.ConfirmOrBatch(chain, upgradeAuthority, []solana.Instruction{ixn}, input.TokenPool, p.PoolTypeLabel)
}

// readPoolState reads the pool State account for tokenMint. It returns nil, with no error, when
// the pool is not initialized for the mint. RPC failures, an account owned by another program and
// decode failures are returned as errors.
//
// The BurnMint, LockRelease, Lombard and CCTP State accounts share the discriminator and start with
// the same {version, config BaseConfig} prefix (Lombard and CCTP only add trailing fields), so the
// BurnMint binding reads the owner and rate limit admin of any of them.
func readPoolState(ctx context.Context, chain cldf_solana.Chain, program, tokenMint solana.PublicKey) (*burnmint_token_pool.State, error) {
	poolConfigPDA, err := tokens.TokenPoolConfigAddress(tokenMint, program)
	if err != nil {
		return nil, fmt.Errorf("failed to derive token pool config address: %w", err)
	}
	state, err := common.GetInitializedAccount(ctx, chain.Client, poolConfigPDA, program, cldf_solana.SolDefaultCommitment, burnmint_token_pool.ParseAccount_State)
	if err != nil {
		return nil, fmt.Errorf("failed to read pool state of %s for mint %s: %w", program, tokenMint, err)
	}
	return state, nil
}

// poolAuthority returns the key that signs owner-gated pool instructions: the pool owner, or the
// program upgrade authority before the pool is initialized for tokenMint (the initializer then
// sets the owner). A failed read is returned instead of falling back to the upgrade authority,
// which would send or batch the instructions for the wrong signer.
func poolAuthority(ctx context.Context, chain cldf_solana.Chain, program, tokenMint solana.PublicKey) (solana.PublicKey, error) {
	state, err := readPoolState(ctx, chain, program, tokenMint)
	if err != nil {
		return solana.PublicKey{}, err
	}
	return authorityFromPoolState(chain, program, state)
}

// authorityFromPoolState is poolAuthority for a state already read with readPoolState.
func authorityFromPoolState(chain cldf_solana.Chain, program solana.PublicKey, state *burnmint_token_pool.State) (solana.PublicKey, error) {
	if state != nil {
		return state.Config.Owner, nil
	}
	authority, err := solcommon.GetUpgradeAuthority(chain.Client, program)
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("failed to get upgrade authority of uninitialized pool program %s: %w", program, err)
	}
	return authority, nil
}

// getTokenPoolAuthority returns the owner of an initialized pool. A pool that is not initialized
// for tokenMint is an error.
func getTokenPoolAuthority(ctx context.Context, chain cldf_solana.Chain, program, tokenMint solana.PublicKey) (solana.PublicKey, error) {
	state, err := readPoolState(ctx, chain, program, tokenMint)
	if err != nil {
		return solana.PublicKey{}, err
	}
	if state == nil {
		return solana.PublicKey{}, fmt.Errorf("pool %s is not initialized for mint %s", program, tokenMint)
	}
	return state.Config.Owner, nil
}

type PoolInitializeOut struct {
	sequences.OnChainOutput
	Initializer solana.PublicKey
}

type SelfServedAllowedInput struct {
	TokenPool         solana.PublicKey
	SelfServedAllowed bool
}

type updateSelfServedAllowedParams struct {
	PoolTypeLabel string // e.g. common_utils.BurnMintTokenPool.String()
	LogName       string // e.g. "BurnMintTokenPool"
	BuildUpdateIx func(configPDA solana.PublicKey, upgradeAuthority solana.PublicKey, programData solana.PublicKey, selfServedAllowed bool) (solana.Instruction, error)
}

// updateSelfServedAllowedTokenPool enables or disables self-served pool initialization in the token pool global config.
// It is a no-op if the on-chain value already matches the requested one.
// If upgradeAuthority != deployer => produces MCMS batch op, otherwise sends tx directly.
func updateSelfServedAllowedTokenPool(
	b operations.Bundle,
	chain cldf_solana.Chain,
	input SelfServedAllowedInput,
	p updateSelfServedAllowedParams,
) (sequences.OnChainOutput, error) {
	configPDA, _, err := state.FindConfigPDA(input.TokenPool)
	if err != nil {
		return sequences.OnChainOutput{}, fmt.Errorf("failed to derive config PDA: %w", err)
	}

	// PoolConfig layout is identical across the BurnMint and LockRelease pool programs.
	var poolConfig burnmint_token_pool.PoolConfig

	err = common.GetAccountDataBorshIntoAnchor(b.GetContext(), chain.Client, configPDA, cldf_solana.SolDefaultCommitment, &poolConfig)
	if err != nil {
		return sequences.OnChainOutput{}, fmt.Errorf("failed to read %s global config (not initialized for token pool %s?): %w", p.LogName, input.TokenPool.String(), err)
	}

	if poolConfig.SelfServedAllowed == input.SelfServedAllowed {
		b.Logger.Info(p.LogName+" self served allowed already set to the requested value for token pool:", input.TokenPool.String())
		return sequences.OnChainOutput{}, nil
	}

	programData, err := solcommon.GetSolProgramData(chain.Client, input.TokenPool)
	if err != nil {
		return sequences.OnChainOutput{}, err
	}

	upgradeAuthority, err := solcommon.GetUpgradeAuthority(chain.Client, input.TokenPool)
	if err != nil {
		return sequences.OnChainOutput{}, err
	}
	// The program requires the upgrade authority to sign, so an immutable program can never accept this instruction.
	if upgradeAuthority.IsZero() {
		return sequences.OnChainOutput{}, fmt.Errorf("%s %s is immutable (no upgrade authority); cannot update self served allowed", p.LogName, input.TokenPool)
	}

	ixn, err := p.BuildUpdateIx(configPDA, upgradeAuthority, programData.Address, input.SelfServedAllowed)
	if err != nil {
		return sequences.OnChainOutput{}, err
	}

	// If deployer isn't upgrade authority, create MCMS batch op.
	return solcommon.ConfirmOrBatch(chain, upgradeAuthority, []solana.Instruction{ixn}, input.TokenPool, p.PoolTypeLabel)
}
