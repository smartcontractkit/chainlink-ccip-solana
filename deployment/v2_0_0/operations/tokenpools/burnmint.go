package tokenpools

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/gagliardetto/solana-go"

	common_utils "github.com/smartcontractkit/chainlink-ccip/deployment/utils"
	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/sequences"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations"
	"github.com/smartcontractkit/mcms/types"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/burnmint_token_pool"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/tokens"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
)

var (
	BurnMintProgramName = "burnmint_token_pool"
	BurnMintVersion     = solcommon.Version2_0_0
)

var DeployBurnMint = operations.NewOperation(
	"burnmint:deploy",
	BurnMintVersion,
	"Deploys the BurnMintTokenPool program",
	func(b operations.Bundle, chain cldf_solana.Chain, input []datastore.AddressRef) (datastore.AddressRef, error) {
		return solcommon.DeployContractIfNotInDatastore(
			b,
			chain,
			input,
			common_utils.BurnMintTokenPool,
			BurnMintVersion,
			solcommon.PoolProgramQualifier,
			BurnMintProgramName)
	},
)

var UpgradeBurnMint = operations.NewOperation(
	"burnmint:upgrade",
	BurnMintVersion,
	"Upgrades the default BurnMintTokenPool program",
	func(b operations.Bundle, chain cldf_solana.Chain, programID solana.PublicKey) ([]solana.Instruction, error) {
		return solcommon.UpgradeProgram(
			b,
			chain,
			common_utils.BurnMintTokenPool,
			programID,
			BurnMintProgramName,
		)
	},
)

var InitializeBurnMint = operations.NewOperation(
	"burnmint:initialize",
	BurnMintVersion,
	"Initializes the BurnMintTokenPool program",
	func(b operations.Bundle, chain cldf_solana.Chain, input Params) (PoolInitializeOut, error) {
		solcommon.SetProgramID(&burnmint_token_pool.ProgramID, burnmint_token_pool.ParseInstructionWithoutAccounts, input.TokenPool)
		batches := make([]types.BatchOperation, 0)
		out, err := operations.ExecuteOperation(b, InitGlobalConfigBurnMint, chain, input)
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
		initialized, err := common.IsAccountInitialized(b.GetContext(), chain.Client, poolConfigPDA, input.TokenPool, cldf_solana.SolDefaultCommitment, burnmint_token_pool.Account_State)
		if err != nil {
			return PoolInitializeOut{}, err
		}
		if initialized {
			b.Logger.Info("BurnMintTokenPool already initialized for token mint:", input.TokenMint.String())
			return PoolInitializeOut{}, nil
		}
		configPDA, _, err := state.FindConfigPDA(input.TokenPool)
		if err != nil {
			return PoolInitializeOut{}, fmt.Errorf("failed to derive config PDA: %w", err)
		}
		ixn, err := burnmint_token_pool.NewInitializeInstruction(
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
		output, err := solcommon.ConfirmOrBatch(chain, upgradeAuthority, []solana.Instruction{ixn}, input.TokenPool, common_utils.BurnMintTokenPool.String())
		if err != nil {
			return PoolInitializeOut{}, err
		}
		output.BatchOps = append(batches, output.BatchOps...)
		return PoolInitializeOut{OnChainOutput: output, Initializer: upgradeAuthority}, nil
	})

var InitGlobalConfigBurnMint = operations.NewOperation(
	"burnmint:global_config",
	BurnMintVersion,
	"Initializes the BurnMintTokenPool global config",
	func(b operations.Bundle, chain cldf_solana.Chain, input Params) (sequences.OnChainOutput, error) {
		return initGlobalConfigTokenPool(b, chain, input, initGlobalCfgParams{
			PoolTypeLabel: common_utils.BurnMintTokenPool.String(),
			LogName:       "BurnMintTokenPool",
			SetProgramID:  nil,
			BuildInitIx: func(configPDA solana.PublicKey, upgradeAuthority solana.PublicKey, programData solana.PublicKey) (solana.Instruction, error) {
				solcommon.SetProgramID(&burnmint_token_pool.ProgramID, burnmint_token_pool.ParseInstructionWithoutAccounts, input.TokenPool)
				return burnmint_token_pool.NewInitGlobalConfigInstruction(
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

var UpdateSelfServedAllowedBurnMint = operations.NewOperation(
	"burnmint:update-self-served-allowed",
	solcommon.Version2_0_0,
	"Enables or disables self-served pool initialization for the BurnMintTokenPool global config",
	func(b operations.Bundle, chain cldf_solana.Chain, input SelfServedAllowedInput) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&burnmint_token_pool.ProgramID, burnmint_token_pool.ParseInstructionWithoutAccounts, input.TokenPool)
		return updateSelfServedAllowedTokenPool(b, chain, input, updateSelfServedAllowedParams{
			PoolTypeLabel: common_utils.BurnMintTokenPool.String(),
			LogName:       "BurnMintTokenPool",
			BuildUpdateIx: func(configPDA solana.PublicKey, upgradeAuthority solana.PublicKey, programData solana.PublicKey, selfServedAllowed bool) (solana.Instruction, error) {
				return burnmint_token_pool.NewUpdateSelfServedAllowedInstruction(
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

var TransferMintAuthorityBurnMint = operations.NewOperation(
	"burnmint:transfer_mint_authority",
	BurnMintVersion,
	"Transfers the mint authority of the token pool's mint",
	func(b operations.Bundle, chain cldf_solana.Chain, input Params) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&burnmint_token_pool.ProgramID, burnmint_token_pool.ParseInstructionWithoutAccounts, input.TokenPool)
		programData, err := solcommon.GetSolProgramData(chain.Client, input.TokenPool)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		// The program checks the signer against the program's upgrade authority, not the pool
		// owner, because moving the mint authority is a critical operation.
		authority, err := solcommon.GetUpgradeAuthority(chain.Client, input.TokenPool)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to get upgrade authority for burn mint token pool: %w", err)
		}
		if authority.IsZero() {
			return sequences.OnChainOutput{}, fmt.Errorf("burn mint token pool %s is immutable (no upgrade authority); cannot transfer mint authority", input.TokenPool)
		}
		poolConfigPDA, err := tokens.TokenPoolConfigAddress(input.TokenMint, input.TokenPool)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive token pool config address: %w", err)
		}
		poolSignerPDA, err := tokens.TokenPoolSignerAddress(input.TokenMint, input.TokenPool)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive token pool signer address: %w", err)
		}
		ixn, err := burnmint_token_pool.NewTransferMintAuthorityToMultisigInstruction(
			poolConfigPDA,
			input.TokenMint,
			input.TokenProgramID,
			poolSignerPDA,
			authority,
			input.NewMintAuthority,
			input.TokenPool,
			programData.Address,
		)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{ixn}, input.TokenPool, common_utils.BurnMintTokenPool.String())
	})

var UpsertRemoteChainConfigBurnMint = operations.NewOperation(
	"burnmint:init_chain_remote_config",
	BurnMintVersion,
	"Initializes the BurnMintTokenPool chain remote config",
	func(b operations.Bundle, chain cldf_solana.Chain, input RemoteChainConfig) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&burnmint_token_pool.ProgramID, burnmint_token_pool.ParseInstructionWithoutAccounts, input.TokenPool)
		remoteConfig := burnmint_token_pool.RemoteConfig{
			PoolAddresses: []burnmint_token_pool.RemoteAddress{},
			TokenAddress: burnmint_token_pool.RemoteAddress{
				Address: input.RemoteTokenAddress,
			},
			Decimals: input.RemoteDecimals,
		}
		authority, err := poolAuthority(b.GetContext(), chain, input.TokenPool, input.TokenMint)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to resolve burn mint token pool authority: %w", err)
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
		remoteChainConfigAccount, err := common.GetInitializedAccount(b.GetContext(), chain.Client, remoteChainConfigPDA, input.TokenPool, cldf_solana.SolDefaultCommitment, burnmint_token_pool.ParseAccount_ChainConfig)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		isSupportedChain := remoteChainConfigAccount != nil
		existingConfig := burnmint_token_pool.BaseChain{}
		if isSupportedChain {
			existingConfig = remoteChainConfigAccount.Base
		}
		var ixns []solana.Instruction
		if isSupportedChain {
			remoteConfig.PoolAddresses = append(remoteConfig.PoolAddresses,
				burnmint_token_pool.RemoteAddress{
					Address: input.RemotePoolAddress,
				})
			// if the token address has changed or if the override config flag is set, edit the remote config (just overwrite the existing remote config)
			if !bytes.Equal(existingConfig.Remote.TokenAddress.Address, input.RemoteTokenAddress) || input.ForceOverrideRemoteConfig {
				var ixn solana.Instruction
				ixn, err = burnmint_token_pool.NewEditChainRemoteConfigInstruction(
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
				baseAddresses := make([]burnmint_token_pool.RemoteAddress, len(poolAddresses))
				for i, cfg := range poolAddresses {
					baseAddresses[i] = burnmint_token_pool.RemoteAddress{
						Address: cfg.Address,
					}
				}
				diff := poolDiff(baseAddresses, remoteConfig.PoolAddresses, func(a burnmint_token_pool.RemoteAddress) []byte { return a.Address })
				if len(diff) > 0 {
					var ixn solana.Instruction
					ixn, err = burnmint_token_pool.NewAppendRemotePoolAddressesInstruction(
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
			var ixn solana.Instruction
			ixn, err = burnmint_token_pool.NewInitChainRemoteConfigInstruction(
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
			var appendIxn solana.Instruction
			appendIxn, err = burnmint_token_pool.NewAppendRemotePoolAddressesInstruction(
				input.RemoteSelector,
				input.TokenMint,
				[]burnmint_token_pool.RemoteAddress{
					{
						Address: input.RemotePoolAddress,
					},
				},
				poolConfigPDA,
				remoteChainConfigPDA,
				authority,
				solana.SystemProgramID,
			)
			if err != nil {
				return sequences.OnChainOutput{}, err
			}
			ixns = append(ixns, appendIxn)
			trplixn, err1 := burnmint_token_pool.NewSetChainRateLimitInstruction(
				input.RemoteSelector,
				input.TokenMint,
				burnmint_token_pool.RateLimitConfig{
					Enabled:  false,
					Capacity: 0,
					Rate:     0,
				},
				burnmint_token_pool.RateLimitConfig{
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
		return solcommon.ConfirmOrBatch(chain, authority, ixns, input.TokenPool, common_utils.BurnMintTokenPool.String())
	})

var UpsertRateLimitsBurnMint = operations.NewOperation(
	"burnmint:rate_limits",
	BurnMintVersion,
	"Initializes the BurnMintTokenPool rate limits for a remote chain",
	func(b operations.Bundle, chain cldf_solana.Chain, input RemoteChainConfig) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&burnmint_token_pool.ProgramID, burnmint_token_pool.ParseInstructionWithoutAccounts, input.TokenPool)
		if !input.InboundRateLimiterConfig.Capacity.IsUint64() || !input.InboundRateLimiterConfig.Rate.IsUint64() {
			return sequences.OnChainOutput{}, errors.New("invalid inbound capacity/inbound rate")
		}
		inboundCapacity := input.InboundRateLimiterConfig.Capacity.Uint64()
		inboundRate := input.InboundRateLimiterConfig.Rate.Uint64()
		if inboundRate > inboundCapacity {
			return sequences.OnChainOutput{}, errors.New("inbound rate > capacity")
		}
		inbound := burnmint_token_pool.RateLimitConfig{
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
		outbound := burnmint_token_pool.RateLimitConfig{
			Enabled:  input.OutboundRateLimiterConfig.IsEnabled,
			Capacity: outboundCapacity,
			Rate:     outboundRate,
		}
		authority, err := poolAuthority(b.GetContext(), chain, input.TokenPool, input.TokenMint)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to resolve burn mint token pool authority: %w", err)
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
		// Setting a rate limit refills the bucket, so skip it when the limits are already set.
		existing, err := common.GetInitializedAccount(b.GetContext(), chain.Client, remoteChainConfigPDA, input.TokenPool, cldf_solana.SolDefaultCommitment, burnmint_token_pool.ParseAccount_ChainConfig)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		if existing != nil && existing.Base.InboundRateLimit.Cfg == inbound && existing.Base.OutboundRateLimit.Cfg == outbound {
			b.Logger.Info("BurnMintTokenPool rate limits already set for remote chain:", input.RemoteSelector)
			return sequences.OnChainOutput{}, nil
		}
		ixn, err := burnmint_token_pool.NewSetChainRateLimitInstruction(
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
		return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{ixn}, input.TokenPool, common_utils.BurnMintTokenPool.String())
	})

var TransferOwnershipBurnMint = operations.NewOperation(
	"burnmint:transfer-ownership",
	BurnMintVersion,
	"Transfers ownership of the BurnMintTokenPool token mint PDA to a new authority",
	func(b operations.Bundle, chain cldf_solana.Chain, input TokenPoolTransferOwnershipInput) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&burnmint_token_pool.ProgramID, burnmint_token_pool.ParseInstructionWithoutAccounts, input.Program)
		authority, err := poolAuthority(b.GetContext(), chain, input.Program, input.TokenMint)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to resolve burn mint token pool authority: %w", err)
		}
		if authority == input.NewOwner {
			b.Logger.Info("New owner is the same as the current owner for burn mint token pool with token mint:", input.TokenMint.String())
			return sequences.OnChainOutput{}, nil
		}
		tokenPoolConfigPDA, err := tokens.TokenPoolConfigAddress(input.TokenMint, input.Program)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive token pool config address: %w", err)
		}
		ixn, err := burnmint_token_pool.NewTransferOwnershipInstruction(
			input.NewOwner,
			tokenPoolConfigPDA,
			input.TokenMint,
			authority,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to build transfer ownership instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{ixn}, input.Program, common_utils.BurnMintTokenPool.String())
	},
)

var AcceptOwnershipBurnMint = operations.NewOperation(
	"burnmint:accept-ownership",
	BurnMintVersion,
	"Accepts ownership of the BurnMintTokenPool token mint PDA",
	func(b operations.Bundle, chain cldf_solana.Chain, input TokenPoolTransferOwnershipInput) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&burnmint_token_pool.ProgramID, burnmint_token_pool.ParseInstructionWithoutAccounts, input.Program)
		tokenPoolConfigPDA, err := tokens.TokenPoolConfigAddress(input.TokenMint, input.Program)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive token pool config address: %w", err)
		}
		authority, err := poolAuthority(b.GetContext(), chain, input.Program, input.TokenMint)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to resolve burn mint token pool authority: %w", err)
		}
		if authority == input.NewOwner {
			b.Logger.Info("New owner is the same as the current owner for burn mint token pool with token mint:", input.TokenMint.String())
			return sequences.OnChainOutput{}, nil
		}
		ixn, err := burnmint_token_pool.NewAcceptOwnershipInstruction(
			tokenPoolConfigPDA,
			input.TokenMint,
			input.NewOwner,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to build accept ownership instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, input.NewOwner, []solana.Instruction{ixn}, input.Program, common_utils.BurnMintTokenPool.String())
	},
)

var UpdateRateLimitAdminBurnMint = operations.NewOperation(
	"burnmint:update-rate-limit-admin",
	BurnMintVersion,
	"Updates the rate limit admin for a BurnMintTokenPool remote chain config",
	func(b operations.Bundle, chain cldf_solana.Chain, input TokenPoolTransferOwnershipInput) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&burnmint_token_pool.ProgramID, burnmint_token_pool.ParseInstructionWithoutAccounts, input.Program)
		poolConfigPDA, err := tokens.TokenPoolConfigAddress(input.TokenMint, input.Program)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive token pool config address: %w", err)
		}
		state, err := readPoolState(b.GetContext(), chain, input.Program, input.TokenMint)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		if state != nil && state.Config.RateLimitAdmin == input.NewOwner {
			b.Logger.Info("New owner is the same as the current rate limit admin for burn mint token pool with token mint:", input.TokenMint.String())
			return sequences.OnChainOutput{}, nil
		}
		authority, err := authorityFromPoolState(chain, input.Program, state)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to resolve burn mint token pool authority: %w", err)
		}
		// Before initialization the upgrade authority signs, and the initializer sets the admin, so
		// there is nothing to update when the upgrade authority is already the requested key.
		if state == nil && authority == input.NewOwner {
			b.Logger.Info("New owner is the same as the current owner for burn mint token pool with token mint:", input.TokenMint.String())
			return sequences.OnChainOutput{}, nil
		}
		ixn, err := burnmint_token_pool.NewSetRateLimitAdminInstruction(
			input.TokenMint,
			input.NewOwner,
			poolConfigPDA,
			authority,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to build update rate limit admin instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{ixn}, input.Program, common_utils.BurnMintTokenPool.String())
	},
)

func GetAuthorityBurnMint(ctx context.Context, chain cldf_solana.Chain, program solana.PublicKey, tokenMint solana.PublicKey) (solana.PublicKey, error) {
	return getTokenPoolAuthority(ctx, chain, program, tokenMint)
}
