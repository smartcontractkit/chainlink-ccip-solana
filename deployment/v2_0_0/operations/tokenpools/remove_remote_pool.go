package tokenpools

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	common_utils "github.com/smartcontractkit/chainlink-ccip/deployment/utils"
	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/sequences"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/burnmint_token_pool"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/cctp_token_pool"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/lockrelease_token_pool"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/lombard_token_pool"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/tokens"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
)

// RemoveRemotePoolInput removes remote pool addresses from a token pool's config for one remote
// chain. Each address is the remote pool as raw bytes in the remote family's native encoding.
type RemoveRemotePoolInput struct {
	TokenPool           solana.PublicKey
	TokenMint           solana.PublicKey
	RemoteSelector      uint64
	RemotePoolAddresses [][]byte
}

// GetRemoteChainConfig reads a token pool's config for remoteSelector, as the BurnMint binding type.
// BurnMint and LockRelease pools store the BaseChain directly. CCTP and Lombard pools store a
// version byte before it and pool-specific fields after it, so they are decoded with their own
// binding and the BaseChain is converted. found is false when the pool was never configured for
// the remote chain.
func GetRemoteChainConfig(b operations.Bundle, chain cldf_solana.Chain, poolType string, tokenPool, tokenMint solana.PublicKey, remoteSelector uint64) (cfg burnmint_token_pool.BaseChain, found bool, err error) {
	remoteChainConfigPDA, _, err := tokens.TokenPoolChainConfigPDA(remoteSelector, tokenMint, tokenPool)
	if err != nil {
		return cfg, false, fmt.Errorf("failed to derive remote chain config PDA: %w", err)
	}
	switch poolType {
	case common_utils.CCTPTokenPool.String():
		var cctpCfg cctp_token_pool.ChainConfig
		err = common.GetAccountDataBorshIntoAnchor(b.GetContext(), chain.Client, remoteChainConfigPDA, cldf_solana.SolDefaultCommitment, &cctpCfg)
		if err == nil {
			err = reencodeBorsh(cctpCfg.Base, &cfg)
		}
	case LombardTokenPool.String():
		var lombardCfg lombard_token_pool.ChainConfig
		err = common.GetAccountDataBorshIntoAnchor(b.GetContext(), chain.Client, remoteChainConfigPDA, cldf_solana.SolDefaultCommitment, &lombardCfg)
		if err == nil {
			err = reencodeBorsh(lombardCfg.Base, &cfg)
		}
	default:
		err = common.GetAccountDataBorshIntoAnchor(b.GetContext(), chain.Client, remoteChainConfigPDA, cldf_solana.SolDefaultCommitment, &cfg)
	}
	if errors.Is(err, rpc.ErrNotFound) {
		return cfg, false, nil
	}
	if err != nil {
		return cfg, false, fmt.Errorf("failed to decode remote chain config at PDA %s on chain %d for remote %d: %w", remoteChainConfigPDA, chain.Selector, remoteSelector, err)
	}
	return cfg, true, nil
}

// reencodeBorsh converts between binding types that share a Borsh layout, such as the BaseChain of
// each token pool program.
func reencodeBorsh(src, dst any) error {
	var buf bytes.Buffer
	if err := bin.NewBorshEncoder(&buf).Encode(src); err != nil {
		return err
	}
	return bin.NewBorshDecoder(buf.Bytes()).Decode(dst)
}

// GetRemoteChainConfigOverride reads the fast transfer finality (FTF) override of a lane, which
// holds the rate limit buckets used for fast-finality transfers. Every token pool program uses the
// same override layout, so the BurnMint binding decodes any of them. found is false when the
// override was never initialized.
func GetRemoteChainConfigOverride(b operations.Bundle, chain cldf_solana.Chain, tokenPool, tokenMint solana.PublicKey, remoteSelector uint64) (cfg burnmint_token_pool.ChainConfigOverride, found bool, err error) {
	overridePDA, _, err := tokens.TokenPoolChainConfigOverridePDA(remoteSelector, tokenMint, tokenPool, []byte("ftf"))
	if err != nil {
		return cfg, false, fmt.Errorf("failed to derive remote chain config override PDA: %w", err)
	}
	err = common.GetAccountDataBorshIntoAnchor(b.GetContext(), chain.Client, overridePDA, cldf_solana.SolDefaultCommitment, &cfg)
	if errors.Is(err, rpc.ErrNotFound) {
		return cfg, false, nil
	}
	if err != nil {
		return cfg, false, fmt.Errorf("failed to decode remote chain config override at PDA %s on chain %d for remote %d: %w", overridePDA, chain.Selector, remoteSelector, err)
	}
	return cfg, true, nil
}

// removeRemotePool rewrites the remote chain config without the target pools. The rewrite replaces
// the whole pool list, so all removals for one remote chain must go through a single call: under
// MCMS each call reads the state before the proposal executes, and a second call would restore the
// addresses the first one removed. Targets that are not configured are rejected.
func removeRemotePool(
	b operations.Bundle,
	chain cldf_solana.Chain,
	input RemoveRemotePoolInput,
	poolType string,
	getAuthority func(context.Context, cldf_solana.Chain, solana.PublicKey, solana.PublicKey) (solana.PublicKey, error),
	buildEditIx func(cfg burnmint_token_pool.RemoteConfig, poolConfigPDA, remoteChainConfigPDA, authority solana.PublicKey) (solana.Instruction, error),
) (sequences.OnChainOutput, error) {
	current, found, err := GetRemoteChainConfig(b, chain, poolType, input.TokenPool, input.TokenMint, input.RemoteSelector)
	if err != nil {
		return sequences.OnChainOutput{}, err
	}
	if !found {
		return sequences.OnChainOutput{}, fmt.Errorf("remote chain %d is not configured on pool %s for mint %s", input.RemoteSelector, input.TokenPool, input.TokenMint)
	}
	existing := current.Remote.PoolAddresses
	for _, target := range input.RemotePoolAddresses {
		if !slices.ContainsFunc(existing, func(addr burnmint_token_pool.RemoteAddress) bool { return bytes.Equal(addr.Address, target) }) {
			return sequences.OnChainOutput{}, fmt.Errorf("remote pool %x is not configured for remote chain %d on pool %s", target, input.RemoteSelector, input.TokenPool)
		}
	}
	retained := make([]burnmint_token_pool.RemoteAddress, 0, len(existing))
	for _, addr := range existing {
		if !slices.ContainsFunc(input.RemotePoolAddresses, func(target []byte) bool { return bytes.Equal(addr.Address, target) }) {
			retained = append(retained, addr)
		}
	}

	authority, err := getAuthority(b.GetContext(), chain, input.TokenPool, input.TokenMint)
	if err != nil {
		return sequences.OnChainOutput{}, fmt.Errorf("failed to get authority for token pool: %w", err)
	}
	poolConfigPDA, err := tokens.TokenPoolConfigAddress(input.TokenMint, input.TokenPool)
	if err != nil {
		return sequences.OnChainOutput{}, fmt.Errorf("failed to derive token pool config address: %w", err)
	}
	remoteChainConfigPDA, _, err := tokens.TokenPoolChainConfigPDA(input.RemoteSelector, input.TokenMint, input.TokenPool)
	if err != nil {
		return sequences.OnChainOutput{}, fmt.Errorf("failed to derive remote chain config PDA: %w", err)
	}
	ixn, err := buildEditIx(burnmint_token_pool.RemoteConfig{
		PoolAddresses: retained,
		TokenAddress:  current.Remote.TokenAddress,
		Decimals:      current.Remote.Decimals,
	}, poolConfigPDA, remoteChainConfigPDA, authority)
	if err != nil {
		return sequences.OnChainOutput{}, fmt.Errorf("failed to build edit chain remote config instruction: %w", err)
	}

	return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{ixn}, input.TokenPool, poolType)
}

var RemoveRemotePoolBurnMint = operations.NewOperation(
	"burnmint:remove-remote-pool",
	solcommon.Version2_0_0,
	"Removes remote pool addresses from a BurnMintTokenPool remote chain config",
	func(b operations.Bundle, chain cldf_solana.Chain, input RemoveRemotePoolInput) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&burnmint_token_pool.ProgramID, burnmint_token_pool.ParseInstructionWithoutAccounts, input.TokenPool)
		return removeRemotePool(b, chain, input, common_utils.BurnMintTokenPool.String(), GetAuthorityBurnMint,
			func(cfg burnmint_token_pool.RemoteConfig, poolConfigPDA, remoteChainConfigPDA, authority solana.PublicKey) (solana.Instruction, error) {
				return burnmint_token_pool.NewEditChainRemoteConfigInstruction(
					input.RemoteSelector, input.TokenMint, cfg, poolConfigPDA, remoteChainConfigPDA, authority, solana.SystemProgramID,
				)
			},
		)
	},
)

var RemoveRemotePoolLockRelease = operations.NewOperation(
	"lockrelease:remove-remote-pool",
	solcommon.Version2_0_0,
	"Removes remote pool addresses from a LockReleaseTokenPool remote chain config",
	func(b operations.Bundle, chain cldf_solana.Chain, input RemoveRemotePoolInput) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&lockrelease_token_pool.ProgramID, lockrelease_token_pool.ParseInstructionWithoutAccounts, input.TokenPool)
		return removeRemotePool(b, chain, input, common_utils.LockReleaseTokenPool.String(), GetAuthorityLockRelease,
			func(cfg burnmint_token_pool.RemoteConfig, poolConfigPDA, remoteChainConfigPDA, authority solana.PublicKey) (solana.Instruction, error) {
				return lockrelease_token_pool.NewEditChainRemoteConfigInstruction(
					input.RemoteSelector, input.TokenMint, toLockReleaseRemoteConfig(cfg), poolConfigPDA, remoteChainConfigPDA, authority, solana.SystemProgramID,
				)
			},
		)
	},
)

var RemoveRemotePoolCctp = operations.NewOperation(
	"cctp:remove-remote-pool",
	solcommon.Version2_0_0,
	"Removes remote pool addresses from a CCTPTokenPool remote chain config",
	func(b operations.Bundle, chain cldf_solana.Chain, input RemoveRemotePoolInput) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&cctp_token_pool.ProgramID, cctp_token_pool.ParseInstructionWithoutAccounts, input.TokenPool)
		return removeRemotePool(b, chain, input, common_utils.CCTPTokenPool.String(), GetAuthorityCctp,
			func(cfg burnmint_token_pool.RemoteConfig, poolConfigPDA, remoteChainConfigPDA, authority solana.PublicKey) (solana.Instruction, error) {
				var cctpCfg cctp_token_pool.RemoteConfig
				if err := reencodeBorsh(cfg, &cctpCfg); err != nil {
					return nil, err
				}
				return cctp_token_pool.NewEditChainRemoteConfigInstruction(
					input.RemoteSelector, input.TokenMint, cctpCfg, poolConfigPDA, remoteChainConfigPDA, authority, solana.SystemProgramID,
				)
			},
		)
	},
)

var RemoveRemotePoolLombard = operations.NewOperation(
	"lombard:remove-remote-pool",
	solcommon.Version2_0_0,
	"Removes remote pool addresses from a LombardTokenPool remote chain config",
	func(b operations.Bundle, chain cldf_solana.Chain, input RemoveRemotePoolInput) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&lombard_token_pool.ProgramID, lombard_token_pool.ParseInstructionWithoutAccounts, input.TokenPool)
		return removeRemotePool(b, chain, input, LombardTokenPool.String(), GetAuthorityLombard,
			func(cfg burnmint_token_pool.RemoteConfig, poolConfigPDA, remoteChainConfigPDA, authority solana.PublicKey) (solana.Instruction, error) {
				var lombardCfg lombard_token_pool.RemoteConfig
				if err := reencodeBorsh(cfg, &lombardCfg); err != nil {
					return nil, err
				}
				return lombard_token_pool.NewEditChainRemoteConfigInstruction(
					input.RemoteSelector, input.TokenMint, lombardCfg, poolConfigPDA, remoteChainConfigPDA, authority, solana.SystemProgramID,
				)
			},
		)
	},
)

func toLockReleaseRemoteConfig(cfg burnmint_token_pool.RemoteConfig) lockrelease_token_pool.RemoteConfig {
	pools := make([]lockrelease_token_pool.RemoteAddress, 0, len(cfg.PoolAddresses))
	for _, p := range cfg.PoolAddresses {
		pools = append(pools, lockrelease_token_pool.RemoteAddress{Address: p.Address})
	}
	return lockrelease_token_pool.RemoteConfig{
		PoolAddresses: pools,
		TokenAddress:  lockrelease_token_pool.RemoteAddress{Address: cfg.TokenAddress.Address},
		Decimals:      cfg.Decimals,
	}
}
