package tokenpools

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/gagliardetto/solana-go"

	common_utils "github.com/smartcontractkit/chainlink-ccip/deployment/utils"
	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/sequences"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/burnmint_token_pool"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/lockrelease_token_pool"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/tokens"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
)

// AddNativeRemotePoolsInput selects a pool's remote chain config and the address width its remote
// pools must have.
type AddNativeRemotePoolsInput struct {
	TokenPool      solana.PublicKey
	TokenMint      solana.PublicKey
	RemoteSelector uint64
	// Width is the source chain's legacy_address_length on the 2.0 OffRamp.
	Width uint8
}

// MissingRemotePoolAddresses returns, for each remote pool address stored zero-padded beyond width,
// its width-byte form when the pool does not already hold it. The 2.0 OffRamp trims the message's
// source pool address to width before release_or_mint and the pool matches it byte for byte, so a
// padded entry alone rejects every inbound transfer. An address that cannot be trimmed to width
// without dropping a non-zero byte, or is shorter than width, is an error.
func MissingRemotePoolAddresses(pools [][]byte, width uint8) ([][]byte, error) {
	if width == 0 {
		return nil, errors.New("invalid remote pool address width 0")
	}
	if len(pools) == 0 {
		return nil, errors.New("no remote pool addresses configured")
	}
	w := int(width)
	held := func(addr []byte, in [][]byte) bool {
		for _, p := range in {
			if bytes.Equal(p, addr) {
				return true
			}
		}
		return false
	}
	var missing [][]byte
	for _, pool := range pools {
		if len(pool) < w {
			return nil, fmt.Errorf("remote pool address %x is shorter than %d bytes", pool, w)
		}
		if len(pool) == w {
			continue
		}
		padding := len(pool) - w
		if !bytes.Equal(pool[:padding], make([]byte, padding)) {
			return nil, fmt.Errorf("remote pool address %x does not fit in %d bytes", pool, w)
		}
		native := pool[padding:]
		if !held(native, pools) && !held(native, missing) {
			missing = append(missing, native)
		}
	}
	return missing, nil
}

// AddNativeRemotePoolsBurnMint appends the width-byte form of a BurnMint pool's zero-padded remote
// pool addresses. Existing entries are left as they are.
var AddNativeRemotePoolsBurnMint = operations.NewOperation(
	"burnmint:add-native-remote-pools",
	BurnMintVersion,
	"Appends BurnMintTokenPool remote pool addresses at the width the 2.0 OffRamp passes",
	func(b operations.Bundle, chain cldf_solana.Chain, input AddNativeRemotePoolsInput) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&burnmint_token_pool.ProgramID, burnmint_token_pool.ParseInstructionWithoutAccounts, input.TokenPool)
		pdas, err := remoteChainConfigPDAs(input)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		account, err := common.GetInitializedAccount(b.GetContext(), chain.Client, pdas.ChainConfig, input.TokenPool, cldf_solana.SolDefaultCommitment, burnmint_token_pool.ParseAccount_ChainConfig)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		if account == nil {
			return sequences.OnChainOutput{}, fmt.Errorf("pool for mint %s is not configured for remote chain %d", input.TokenMint, input.RemoteSelector)
		}
		pools := make([][]byte, len(account.Base.Remote.PoolAddresses))
		for i, p := range account.Base.Remote.PoolAddresses {
			pools[i] = p.Address
		}
		missing, err := MissingRemotePoolAddresses(pools, input.Width)
		if err != nil || len(missing) == 0 {
			return sequences.OnChainOutput{}, err
		}
		addresses := make([]burnmint_token_pool.RemoteAddress, len(missing))
		for i, m := range missing {
			addresses[i] = burnmint_token_pool.RemoteAddress{Address: m}
		}
		authority, err := poolAuthority(b.GetContext(), chain, input.TokenPool, input.TokenMint)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		ixn, err := burnmint_token_pool.NewAppendRemotePoolAddressesInstruction(input.RemoteSelector, input.TokenMint, addresses, pdas.PoolConfig, pdas.ChainConfig, authority, solana.SystemProgramID)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{ixn}, input.TokenPool, common_utils.BurnMintTokenPool.String())
	})

// AddNativeRemotePoolsLockRelease is AddNativeRemotePoolsBurnMint for LockRelease pools.
var AddNativeRemotePoolsLockRelease = operations.NewOperation(
	"lockrelease:add-native-remote-pools",
	LockReleaseVersion,
	"Appends LockReleaseTokenPool remote pool addresses at the width the 2.0 OffRamp passes",
	func(b operations.Bundle, chain cldf_solana.Chain, input AddNativeRemotePoolsInput) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&lockrelease_token_pool.ProgramID, lockrelease_token_pool.ParseInstructionWithoutAccounts, input.TokenPool)
		pdas, err := remoteChainConfigPDAs(input)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		account, err := common.GetInitializedAccount(b.GetContext(), chain.Client, pdas.ChainConfig, input.TokenPool, cldf_solana.SolDefaultCommitment, lockrelease_token_pool.ParseAccount_ChainConfig)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		if account == nil {
			return sequences.OnChainOutput{}, fmt.Errorf("pool for mint %s is not configured for remote chain %d", input.TokenMint, input.RemoteSelector)
		}
		pools := make([][]byte, len(account.Base.Remote.PoolAddresses))
		for i, p := range account.Base.Remote.PoolAddresses {
			pools[i] = p.Address
		}
		missing, err := MissingRemotePoolAddresses(pools, input.Width)
		if err != nil || len(missing) == 0 {
			return sequences.OnChainOutput{}, err
		}
		addresses := make([]lockrelease_token_pool.RemoteAddress, len(missing))
		for i, m := range missing {
			addresses[i] = lockrelease_token_pool.RemoteAddress{Address: m}
		}
		authority, err := poolAuthority(b.GetContext(), chain, input.TokenPool, input.TokenMint)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		ixn, err := lockrelease_token_pool.NewAppendRemotePoolAddressesInstruction(input.RemoteSelector, input.TokenMint, addresses, pdas.PoolConfig, pdas.ChainConfig, authority, solana.SystemProgramID)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{ixn}, input.TokenPool, common_utils.LockReleaseTokenPool.String())
	})

type remoteChainConfigAccounts struct {
	PoolConfig  solana.PublicKey
	ChainConfig solana.PublicKey
}

func remoteChainConfigPDAs(input AddNativeRemotePoolsInput) (remoteChainConfigAccounts, error) {
	poolConfig, err := tokens.TokenPoolConfigAddress(input.TokenMint, input.TokenPool)
	if err != nil {
		return remoteChainConfigAccounts{}, fmt.Errorf("derive pool config: %w", err)
	}
	chainConfig, _, err := tokens.TokenPoolChainConfigPDA(input.RemoteSelector, input.TokenMint, input.TokenPool)
	if err != nil {
		return remoteChainConfigAccounts{}, fmt.Errorf("derive chain config for remote chain %d: %w", input.RemoteSelector, err)
	}
	return remoteChainConfigAccounts{PoolConfig: poolConfig, ChainConfig: chainConfig}, nil
}
