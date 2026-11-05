package tokenpools

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/gagliardetto/solana-go"
	solrpc "github.com/gagliardetto/solana-go/rpc"

	common_utils "github.com/smartcontractkit/chainlink-ccip/deployment/utils"
	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/sequences"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations"
	"github.com/smartcontractkit/mcms/types"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/lockrelease_token_pool"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/tokens"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
)

var (
	LockReleaseProgramName = "lockrelease_token_pool"
	LockReleaseVersion     = solcommon.Version2_0_0
)

var DeployLockRelease = operations.NewOperation(
	"lockrelease:deploy",
	LockReleaseVersion,
	"Deploys the LockReleaseTokenPool program",
	func(b operations.Bundle, chain cldf_solana.Chain, input []datastore.AddressRef) (datastore.AddressRef, error) {
		return solcommon.DeployContractIfNotInDatastore(
			b,
			chain,
			input,
			common_utils.LockReleaseTokenPool,
			LockReleaseVersion,
			solcommon.PoolProgramQualifier,
			LockReleaseProgramName)
	},
)

var UpgradeLockRelease = operations.NewOperation(
	"lockrelease:upgrade",
	LockReleaseVersion,
	"Upgrades the default LockReleaseTokenPool program",
	func(b operations.Bundle, chain cldf_solana.Chain, programID solana.PublicKey) ([]solana.Instruction, error) {
		return solcommon.UpgradeProgram(
			b,
			chain,
			common_utils.LockReleaseTokenPool,
			programID,
			LockReleaseProgramName,
		)
	},
)

var InitializeLockRelease = operations.NewOperation(
	"lockrelease:initialize",
	LockReleaseVersion,
	"Initializes the LockReleaseTokenPool program",
	func(b operations.Bundle, chain cldf_solana.Chain, input Params) (PoolInitializeOut, error) {
		solcommon.SetProgramID(&lockrelease_token_pool.ProgramID, lockrelease_token_pool.ParseInstructionWithoutAccounts, input.TokenPool)
		batches := make([]types.BatchOperation, 0)
		out, err := operations.ExecuteOperation(b, InitGlobalConfigLockRelease, chain, input)
		if err != nil {
			return PoolInitializeOut{}, fmt.Errorf("failed to initialize global config: %w", err)
		}
		batches = append(batches, out.Output.BatchOps...)
		programData, err := solcommon.GetSolProgramData(chain.Client, input.TokenPool)
		if err != nil {
			return PoolInitializeOut{}, err
		}
		upgradeAuthority, err := solcommon.GetUpgradeAuthority(chain.Client, input.TokenPool)
		if err != nil {
			return PoolInitializeOut{}, err
		}
		poolConfigPDA, err := tokens.TokenPoolConfigAddress(input.TokenMint, input.TokenPool)
		if err != nil {
			return PoolInitializeOut{}, fmt.Errorf("failed to derive token pool config address: %w", err)
		}
		initialized, err := common.IsAccountInitialized(b.GetContext(), chain.Client, poolConfigPDA, input.TokenPool, cldf_solana.SolDefaultCommitment, lockrelease_token_pool.Account_State)
		if err != nil {
			return PoolInitializeOut{}, err
		}
		if initialized {
			b.Logger.Info("LockReleaseTokenPool already initialized for token mint:", input.TokenMint.String())
			return PoolInitializeOut{}, nil
		}
		configPDA, _, err := state.FindConfigPDA(input.TokenPool)
		if err != nil {
			return PoolInitializeOut{}, fmt.Errorf("failed to derive config PDA: %w", err)
		}
		ixn, err := lockrelease_token_pool.NewInitializeInstruction(
			poolConfigPDA,
			input.TokenMint,
			upgradeAuthority,
			solana.SystemProgramID,
			input.TokenPool,
			programData.Address,
			configPDA,
		)
		if err != nil {
			return PoolInitializeOut{}, err
		}
		output, err := solcommon.ConfirmOrBatch(chain, upgradeAuthority, []solana.Instruction{ixn}, input.TokenPool, common_utils.LockReleaseTokenPool.String())
		if err != nil {
			return PoolInitializeOut{}, err
		}
		output.BatchOps = append(batches, output.BatchOps...)
		return PoolInitializeOut{OnChainOutput: output, Initializer: upgradeAuthority}, nil
	})

var InitGlobalConfigLockRelease = operations.NewOperation(
	"lockrelease:global_config",
	LockReleaseVersion,
	"Initializes the LockReleaseTokenPool global config",
	func(b operations.Bundle, chain cldf_solana.Chain, input Params) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&lockrelease_token_pool.ProgramID, lockrelease_token_pool.ParseInstructionWithoutAccounts, input.TokenPool)
		return initGlobalConfigTokenPool(b, chain, input, initGlobalCfgParams{
			PoolTypeLabel: common_utils.LockReleaseTokenPool.String(),
			LogName:       "LockReleaseTokenPool",
			SetProgramID:  nil,
			BuildInitIx: func(configPDA solana.PublicKey, upgradeAuthority solana.PublicKey, programData solana.PublicKey) (solana.Instruction, error) {
				return lockrelease_token_pool.NewInitGlobalConfigInstruction(
					input.Router,
					input.RMNRemote,
					configPDA,
					upgradeAuthority,
					solana.SystemProgramID,
					input.TokenPool,
					programData,
				)
			},
		})
	},
)

var UpdateSelfServedAllowedLockRelease = operations.NewOperation(
	"lockrelease:update-self-served-allowed",
	solcommon.Version2_0_0,
	"Enables or disables self-served pool initialization for the LockReleaseTokenPool global config",
	func(b operations.Bundle, chain cldf_solana.Chain, input SelfServedAllowedInput) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&lockrelease_token_pool.ProgramID, lockrelease_token_pool.ParseInstructionWithoutAccounts, input.TokenPool)
		return updateSelfServedAllowedTokenPool(b, chain, input, updateSelfServedAllowedParams{
			PoolTypeLabel: common_utils.LockReleaseTokenPool.String(),
			LogName:       "LockReleaseTokenPool",
			BuildUpdateIx: func(configPDA solana.PublicKey, upgradeAuthority solana.PublicKey, programData solana.PublicKey, selfServedAllowed bool) (solana.Instruction, error) {
				return lockrelease_token_pool.NewUpdateSelfServedAllowedInstruction(
					selfServedAllowed,
					configPDA,
					upgradeAuthority,
					input.TokenPool,
					programData,
				)
			},
		})
	},
)

var UpsertRemoteChainConfigLockRelease = operations.NewOperation(
	"lockrelease:init_chain_remote_config",
	LockReleaseVersion,
	"Initializes the LockReleaseTokenPool chain remote config",
	func(b operations.Bundle, chain cldf_solana.Chain, input RemoteChainConfig) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&lockrelease_token_pool.ProgramID, lockrelease_token_pool.ParseInstructionWithoutAccounts, input.TokenPool)
		remoteConfig := lockrelease_token_pool.RemoteConfig{
			PoolAddresses: []lockrelease_token_pool.RemoteAddress{},
			TokenAddress: lockrelease_token_pool.RemoteAddress{
				Address: input.RemoteTokenAddress,
			},
			Decimals: input.RemoteDecimals,
		}
		authority, err := poolAuthority(b.GetContext(), chain, input.TokenPool, input.TokenMint)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to resolve lock release token pool authority: %w", err)
		}
		poolConfigPDA, err := tokens.TokenPoolConfigAddress(input.TokenMint, input.TokenPool)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive token pool config address: %w", err)
		}
		// check if remote chain config already exists
		remoteChainConfigPDA, _, err := tokens.TokenPoolChainConfigPDA(input.RemoteSelector, input.TokenMint, input.TokenPool)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive remote chain config PDA: %w", err)
		}
		remoteChainConfigAccount, err := common.GetInitializedAccount(b.GetContext(), chain.Client, remoteChainConfigPDA, input.TokenPool, cldf_solana.SolDefaultCommitment, lockrelease_token_pool.ParseAccount_ChainConfig)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		isSupportedChain := remoteChainConfigAccount != nil
		existingConfig := lockrelease_token_pool.BaseChain{}
		if isSupportedChain {
			existingConfig = remoteChainConfigAccount.Base
		}
		var ixns []solana.Instruction
		var ixn solana.Instruction
		if isSupportedChain {
			remoteConfig.PoolAddresses = append(remoteConfig.PoolAddresses,
				lockrelease_token_pool.RemoteAddress{
					Address: input.RemotePoolAddress,
				})
			// if the token address has changed or if the override config flag is set, edit the remote config (just overwrite the existing remote config)
			if !bytes.Equal(existingConfig.Remote.TokenAddress.Address, input.RemoteTokenAddress) || input.ForceOverrideRemoteConfig {
				ixn, err = lockrelease_token_pool.NewEditChainRemoteConfigInstruction(
					input.RemoteSelector,
					input.TokenMint,
					remoteConfig,
					poolConfigPDA,
					remoteChainConfigPDA,
					authority,
					solana.SystemProgramID,
				)
				if err != nil {
					return sequences.OnChainOutput{}, err
				}
				ixns = append(ixns, ixn)
			} else {
				// diff between [existing remote pool addresses on solana chain] vs [what was just derived from evm chain]
				poolAddresses := existingConfig.Remote.PoolAddresses
				// translate to base
				baseAddresses := make([]lockrelease_token_pool.RemoteAddress, len(poolAddresses))
				for i, cfg := range poolAddresses {
					baseAddresses[i] = lockrelease_token_pool.RemoteAddress{
						Address: cfg.Address,
					}
				}
				diff := poolDiff(baseAddresses, remoteConfig.PoolAddresses, func(a lockrelease_token_pool.RemoteAddress) []byte { return a.Address })
				if len(diff) > 0 {
					ixn, err = lockrelease_token_pool.NewAppendRemotePoolAddressesInstruction(
						input.RemoteSelector,
						input.TokenMint,
						diff, // evm supports multiple remote pools per token
						poolConfigPDA,
						remoteChainConfigPDA,
						authority,
						solana.SystemProgramID,
					)
					if err != nil {
						return sequences.OnChainOutput{}, err
					}
					ixns = append(ixns, ixn)
				}
			}
		} else {
			ixn, err = lockrelease_token_pool.NewInitChainRemoteConfigInstruction(
				input.RemoteSelector,
				input.TokenMint,
				remoteConfig,
				poolConfigPDA,
				remoteChainConfigPDA,
				authority,
				solana.SystemProgramID,
			)
			if err != nil {
				return sequences.OnChainOutput{}, err
			}
			ixns = append(ixns, ixn)
			appendIxn, err1 := lockrelease_token_pool.NewAppendRemotePoolAddressesInstruction(
				input.RemoteSelector,
				input.TokenMint,
				[]lockrelease_token_pool.RemoteAddress{
					{
						Address: input.RemotePoolAddress,
					},
				},
				poolConfigPDA,
				remoteChainConfigPDA,
				authority,
				solana.SystemProgramID,
			)
			if err1 != nil {
				return sequences.OnChainOutput{}, err1
			}
			ixns = append(ixns, appendIxn)
			trplixn, err1 := lockrelease_token_pool.NewSetChainRateLimitInstruction(
				input.RemoteSelector,
				input.TokenMint,
				lockrelease_token_pool.RateLimitConfig{
					Enabled:  false,
					Capacity: 0,
					Rate:     0,
				},
				lockrelease_token_pool.RateLimitConfig{
					Enabled:  false,
					Capacity: 0,
					Rate:     0,
				},
				poolConfigPDA,
				remoteChainConfigPDA,
				authority,
			)
			if err1 != nil {
				return sequences.OnChainOutput{}, err1
			}
			ixns = append(ixns, trplixn)
		}
		return solcommon.ConfirmOrBatch(chain, authority, ixns, input.TokenPool, common_utils.LockReleaseTokenPool.String())
	})

var UpsertRateLimitsLockRelease = operations.NewOperation(
	"lockrelease:rate_limits",
	LockReleaseVersion,
	"Initializes the LockReleaseTokenPool rate limits for a remote chain",
	func(b operations.Bundle, chain cldf_solana.Chain, input RemoteChainConfig) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&lockrelease_token_pool.ProgramID, lockrelease_token_pool.ParseInstructionWithoutAccounts, input.TokenPool)
		if !input.InboundRateLimiterConfig.Capacity.IsUint64() || !input.InboundRateLimiterConfig.Rate.IsUint64() {
			return sequences.OnChainOutput{}, errors.New("invalid inbound capacity/inbound rate")
		}
		inboundCapacity := input.InboundRateLimiterConfig.Capacity.Uint64()
		inboundRate := input.InboundRateLimiterConfig.Rate.Uint64()
		if inboundRate > inboundCapacity {
			return sequences.OnChainOutput{}, errors.New("inbound rate > capacity")
		}
		inbound := lockrelease_token_pool.RateLimitConfig{
			Enabled:  input.InboundRateLimiterConfig.IsEnabled,
			Capacity: inboundCapacity,
			Rate:     inboundRate,
		}
		if !input.OutboundRateLimiterConfig.Capacity.IsUint64() || !input.OutboundRateLimiterConfig.Rate.IsUint64() {
			return sequences.OnChainOutput{}, errors.New("invalid outbound capacity/inbound rate")
		}
		outboundCapacity := input.OutboundRateLimiterConfig.Capacity.Uint64()
		outboundRate := input.OutboundRateLimiterConfig.Rate.Uint64()
		if outboundRate > outboundCapacity {
			return sequences.OnChainOutput{}, errors.New("outbound rate > capacity")
		}
		outbound := lockrelease_token_pool.RateLimitConfig{
			Enabled:  input.OutboundRateLimiterConfig.IsEnabled,
			Capacity: outboundCapacity,
			Rate:     outboundRate,
		}
		authority, err := poolAuthority(b.GetContext(), chain, input.TokenPool, input.TokenMint)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to resolve lock release token pool authority: %w", err)
		}
		poolConfigPDA, err := tokens.TokenPoolConfigAddress(input.TokenMint, input.TokenPool)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive token pool config address: %w", err)
		}
		// check if remote chain config already exists
		remoteChainConfigPDA, _, err := tokens.TokenPoolChainConfigPDA(input.RemoteSelector, input.TokenMint, input.TokenPool)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive remote chain config PDA: %w", err)
		}
		ixn, err := lockrelease_token_pool.NewSetChainRateLimitInstruction(
			input.RemoteSelector,
			input.TokenMint,
			inbound,
			outbound,
			poolConfigPDA,
			remoteChainConfigPDA,
			authority,
		)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{ixn}, input.TokenPool, common_utils.LockReleaseTokenPool.String())
	})

var TransferOwnershipLockRelease = operations.NewOperation(
	"lockrelease:transfer-ownership",
	LockReleaseVersion,
	"Transfers ownership of the LockReleaseTokenPool token mint PDA to a new authority",
	func(b operations.Bundle, chain cldf_solana.Chain, input TokenPoolTransferOwnershipInput) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&lockrelease_token_pool.ProgramID, lockrelease_token_pool.ParseInstructionWithoutAccounts, input.Program)
		authority, err := poolAuthority(b.GetContext(), chain, input.Program, input.TokenMint)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to resolve lock release token pool authority: %w", err)
		}
		if authority == input.NewOwner {
			b.Logger.Info("New owner is the same as the current owner for lock release token pool with token mint:", input.TokenMint.String())
			return sequences.OnChainOutput{}, nil
		}
		tokenPoolConfigPDA, err := tokens.TokenPoolConfigAddress(input.TokenMint, input.Program)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive token pool config address: %w", err)
		}
		ixn, err := lockrelease_token_pool.NewTransferOwnershipInstruction(
			input.NewOwner,
			tokenPoolConfigPDA,
			input.TokenMint,
			authority,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to build transfer ownership instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{ixn}, input.Program, common_utils.LockReleaseTokenPool.String())
	},
)

var AcceptOwnershipLockRelease = operations.NewOperation(
	"lockrelease:accept-ownership",
	LockReleaseVersion,
	"Accepts ownership of the LockReleaseTokenPool token mint PDA",
	func(b operations.Bundle, chain cldf_solana.Chain, input TokenPoolTransferOwnershipInput) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&lockrelease_token_pool.ProgramID, lockrelease_token_pool.ParseInstructionWithoutAccounts, input.Program)
		tokenPoolConfigPDA, err := tokens.TokenPoolConfigAddress(input.TokenMint, input.Program)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive token pool config address: %w", err)
		}
		authority, err := poolAuthority(b.GetContext(), chain, input.Program, input.TokenMint)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to resolve lock release token pool authority: %w", err)
		}
		if authority == input.NewOwner {
			b.Logger.Info("New owner is the same as the current owner for lock release token pool with token mint:", input.TokenMint.String())
			return sequences.OnChainOutput{}, nil
		}
		ixn, err := lockrelease_token_pool.NewAcceptOwnershipInstruction(
			tokenPoolConfigPDA,
			input.TokenMint,
			input.NewOwner,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to build accept ownership instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, input.NewOwner, []solana.Instruction{ixn}, input.Program, common_utils.LockReleaseTokenPool.String())
	},
)

var UpdateRateLimitAdminLockRelease = operations.NewOperation(
	"lockrelease:update-rate-limit-admin",
	LockReleaseVersion,
	"Updates the rate limit admin for a LockReleaseTokenPool remote chain config",
	func(b operations.Bundle, chain cldf_solana.Chain, input TokenPoolTransferOwnershipInput) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&lockrelease_token_pool.ProgramID, lockrelease_token_pool.ParseInstructionWithoutAccounts, input.Program)
		poolConfigPDA, err := tokens.TokenPoolConfigAddress(input.TokenMint, input.Program)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive token pool config address: %w", err)
		}
		state, err := readPoolState(b.GetContext(), chain, input.Program, input.TokenMint)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		if state != nil && state.Config.RateLimitAdmin == input.NewOwner {
			b.Logger.Info("New owner is the same as the current rate limit admin for lock release token pool with token mint:", input.TokenMint.String())
			return sequences.OnChainOutput{}, nil
		}
		authority, err := authorityFromPoolState(chain, input.Program, state)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to resolve lock release token pool authority: %w", err)
		}
		// Before initialization the upgrade authority signs, and the initializer sets the admin, so
		// there is nothing to update when the upgrade authority is already the requested key.
		if state == nil && authority == input.NewOwner {
			b.Logger.Info("New owner is the same as the current owner for lock release token pool with token mint:", input.TokenMint.String())
			return sequences.OnChainOutput{}, nil
		}
		ixn, err := lockrelease_token_pool.NewSetRateLimitAdminInstruction(
			input.TokenMint,
			input.NewOwner,
			poolConfigPDA,
			authority,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to build update rate limit admin instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{ixn}, input.Program, common_utils.LockReleaseTokenPool.String())
	},
)

func GetAuthorityLockRelease(ctx context.Context, chain cldf_solana.Chain, program solana.PublicKey, tokenMint solana.PublicKey) (solana.PublicKey, error) {
	return getTokenPoolAuthority(ctx, chain, program, tokenMint)
}

var SetRebalancerLockRelease = operations.NewOperation(
	"lockrelease:set-rebalancer",
	LockReleaseVersion,
	"Sets the rebalancer for a LockReleaseTokenPool",
	func(b operations.Bundle, chain cldf_solana.Chain, input TokenPoolSetRebalancerInput) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&lockrelease_token_pool.ProgramID, lockrelease_token_pool.ParseInstructionWithoutAccounts, input.Program)
		poolConfigPDA, err := tokens.TokenPoolConfigAddress(input.TokenMint, input.Program)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive token pool config address: %w", err)
		}
		var chainConfig lockrelease_token_pool.State
		err = common.GetAccountDataBorshIntoAnchor(b.GetContext(), chain.Client, poolConfigPDA, cldf_solana.SolDefaultCommitment, &chainConfig)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to read pool state (pool not initialized for mint %s?): %w", input.TokenMint.String(), err)
		}
		if chainConfig.Config.Rebalancer == input.Rebalancer {
			b.Logger.Info("Rebalancer already set for lock release token pool with token mint:", input.TokenMint.String())
			return sequences.OnChainOutput{}, nil
		}
		authority, err := GetAuthorityLockRelease(b.GetContext(), chain, input.Program, input.TokenMint)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to get authority for lock release token pool: %w", err)
		}
		ixn, err := lockrelease_token_pool.NewSetRebalancerInstruction(
			input.Rebalancer,
			poolConfigPDA,
			input.TokenMint,
			authority,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to build set rebalancer instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{ixn}, input.Program, common_utils.LockReleaseTokenPool.String())
	},
)

// ProvideLiquidityInput deposits tokens held by the deployer into a LockRelease
// pool as liquidity. Rebalancer (optional) sets the pool rebalancer first when
// it differs from the on-chain value.
type ProvideLiquidityInput struct {
	Program    solana.PublicKey
	TokenMint  solana.PublicKey
	Rebalancer solana.PublicKey
	Amount     uint64
}

// TEST/controlled environment only — operational tooling, not a protocol
// contract op (EVM parity: rebalancer-gated liquidity is only ever called raw
// from test code, never through MCMS). Liquidity is rebalancer-gated, so the
// signer (chain.DeployerKey) must BE the on-chain rebalancer; the set-rebalancer
// step is deployer-signed too and refuses the MCMS path.
var ProvideLiquidityLockRelease = operations.NewOperation(
	"lockrelease:provide-liquidity",
	LockReleaseVersion,
	"TEST/controlled environment only: deposit deployer-held tokens into a LockRelease pool",
	func(b operations.Bundle, chain cldf_solana.Chain, input ProvideLiquidityInput) (sequences.OnChainOutput, error) {
		deployer := chain.DeployerKey.PublicKey()

		if !input.Rebalancer.IsZero() {
			setOut, err := operations.ExecuteOperation(b, SetRebalancerLockRelease, chain, TokenPoolSetRebalancerInput{
				Program:    input.Program,
				TokenMint:  input.TokenMint,
				Rebalancer: input.Rebalancer,
			})
			if err != nil {
				return sequences.OnChainOutput{}, fmt.Errorf("set rebalancer: %w", err)
			}
			if len(setOut.Output.BatchOps) > 0 {
				return sequences.OnChainOutput{}, fmt.Errorf(
					"rebalancer %s is not the deployer and set-rebalancer would require MCMS; provide-liquidity is deployer-signed only: pass the deployer as the rebalancer",
					input.Rebalancer.String())
			}
		}

		solcommon.SetProgramID(&lockrelease_token_pool.ProgramID, lockrelease_token_pool.ParseInstructionWithoutAccounts, input.Program)
		poolConfigPDA, err := tokens.TokenPoolConfigAddress(input.TokenMint, input.Program)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("derive token pool config address: %w", err)
		}
		var chainConfig lockrelease_token_pool.State
		if err = common.GetAccountDataBorshIntoAnchor(b.GetContext(), chain.Client, poolConfigPDA, cldf_solana.SolDefaultCommitment, &chainConfig); err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("read pool state (pool not initialized for mint %s?): %w", input.TokenMint.String(), err)
		}
		if chainConfig.Config.Rebalancer != deployer {
			return sequences.OnChainOutput{}, fmt.Errorf(
				"on-chain rebalancer is %s but the deployer is %s; set the rebalancer to the deployer first (or pass it as input)",
				chainConfig.Config.Rebalancer.String(), deployer.String())
		}
		providerTokenAccount, _, err := tokens.FindAssociatedTokenAddress(chainConfig.Config.TokenProgram, input.TokenMint, deployer)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("derive deployer token account: %w", err)
		}
		acct, err := chain.Client.GetAccountInfoWithOpts(b.GetContext(), providerTokenAccount, &solrpc.GetAccountInfoOpts{
			Commitment: cldf_solana.SolDefaultCommitment,
		})
		if err != nil || acct == nil || acct.Value == nil {
			return sequences.OnChainOutput{}, fmt.Errorf(
				"deployer token account %s for mint %s does not exist; fund (or mint) supply to the deployer first",
				providerTokenAccount.String(), input.TokenMint.String())
		}

		approveIx, err := tokens.TokenApproveChecked(input.Amount, chainConfig.Config.Decimals, chainConfig.Config.TokenProgram, providerTokenAccount, input.TokenMint, chainConfig.Config.PoolSigner, deployer, nil)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("build approve instruction: %w", err)
		}
		provideIx, err := lockrelease_token_pool.NewProvideLiquidityInstruction(input.Amount, poolConfigPDA, chainConfig.Config.TokenProgram, input.TokenMint, chainConfig.Config.PoolSigner, chainConfig.Config.PoolTokenAccount, providerTokenAccount, deployer)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("build provide liquidity instruction: %w", err)
		}
		if err := chain.Confirm([]solana.Instruction{approveIx, provideIx}); err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("confirm provide liquidity: %w", err)
		}
		b.Logger.Infow("liquidity provided", "mint", input.TokenMint.String(), "pool", input.Program.String(), "amount", input.Amount)
		return sequences.OnChainOutput{}, nil
	},
)
