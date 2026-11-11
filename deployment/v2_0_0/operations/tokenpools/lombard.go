package tokenpools

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/gagliardetto/solana-go"

	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/sequences"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	cldf_deployment "github.com/smartcontractkit/chainlink-deployments-framework/deployment"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations"
	"github.com/smartcontractkit/mcms/types"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/lombard_token_pool"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/tokens"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
)

var LombardProgramName = "lombard_token_pool"

// LombardTokenPool has no upstream chainlink-ccip constant (Lombard is Solana-only, with no
// EVM/legacy-1.6 analog like BurnMintTokenPool/LockReleaseTokenPool have). datastore.ContractType/
// cldf_deployment.ContractType is a plain string type and every dispatch in this repo compares by
// .String() value, so a local constant is enough for the Solana-side switch statements to work.
var LombardTokenPool cldf_deployment.ContractType = "LombardTokenPool"

var DeployLombard = operations.NewOperation(
	"lombard:deploy",
	solcommon.Version2_0_0,
	"Deploys the LombardTokenPool program",
	func(b operations.Bundle, chain cldf_solana.Chain, input []datastore.AddressRef) (datastore.AddressRef, error) {
		return solcommon.DeployContractIfNotInDatastore(
			b,
			chain,
			input,
			LombardTokenPool,
			solcommon.Version2_0_0,
			solcommon.PoolProgramQualifier,
			LombardProgramName)
	},
)

var UpgradeLombard = operations.NewOperation(
	"lombard:upgrade",
	solcommon.Version2_0_0,
	"Upgrades the default LombardTokenPool program",
	func(b operations.Bundle, chain cldf_solana.Chain, programID solana.PublicKey) ([]solana.Instruction, error) {
		return solcommon.UpgradeProgram(
			b,
			chain,
			LombardTokenPool,
			programID,
			LombardProgramName,
		)
	},
)

// LombardParams extends Params with the bridge program address that Lombard's initialize()
// records on State. Unlike BurnMint/LockRelease, Lombard's pool CPIs into a Lombard-specific
// bridge program for outbound transfers, so it needs to know that address at init time.
type LombardParams struct {
	Params
	Bridge solana.PublicKey
}

var InitializeLombard = operations.NewOperation(
	"lombard:initialize",
	solcommon.Version2_0_0,
	"Initializes the LombardTokenPool program",
	func(b operations.Bundle, chain cldf_solana.Chain, input LombardParams) (PoolInitializeOut, error) {
		solcommon.SetProgramID(&lombard_token_pool.ProgramID, lombard_token_pool.ParseInstructionWithoutAccounts, input.TokenPool)
		batches := make([]types.BatchOperation, 0)
		out, err := operations.ExecuteOperation(b, InitGlobalConfigLombard, chain, input.Params)
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
		initialized, err := common.IsAccountInitialized(b.GetContext(), chain.Client, poolConfigPDA, input.TokenPool, cldf_solana.SolDefaultCommitment, lombard_token_pool.Account_State)
		if err != nil {
			return PoolInitializeOut{}, err
		}
		if initialized {
			b.Logger.Info("LombardTokenPool already initialized for token mint:", input.TokenMint.String())
			return PoolInitializeOut{}, nil
		}
		configPDA, _, err := state.FindLombardPoolConfigPDA(input.TokenPool)
		if err != nil {
			return PoolInitializeOut{}, fmt.Errorf("failed to derive config PDA: %w", err)
		}
		ixn, err := lombard_token_pool.NewInitializeInstruction(
			input.Router,
			input.RMNRemote,
			input.Bridge,
			poolConfigPDA,
			input.TokenMint,
			upgradeAuthority,
			input.TokenPool,
			programData.Address,
			configPDA,
			solana.SystemProgramID,
		)
		if err != nil {
			return PoolInitializeOut{}, err
		}
		output, err := solcommon.ConfirmOrBatch(chain, upgradeAuthority, []solana.Instruction{ixn}, input.TokenPool, LombardTokenPool.String())
		if err != nil {
			return PoolInitializeOut{}, err
		}
		output.BatchOps = append(batches, output.BatchOps...)
		return PoolInitializeOut{OnChainOutput: output, Initializer: upgradeAuthority}, nil
	})

var InitGlobalConfigLombard = operations.NewOperation(
	"lombard:global_config",
	solcommon.Version2_0_0,
	"Initializes the LombardTokenPool global config",
	func(b operations.Bundle, chain cldf_solana.Chain, input Params) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&lombard_token_pool.ProgramID, lombard_token_pool.ParseInstructionWithoutAccounts, input.TokenPool)
		programData, err := solcommon.GetSolProgramData(chain.Client, input.TokenPool)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		upgradeAuthority, err := solcommon.GetUpgradeAuthority(chain.Client, input.TokenPool)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		configPDA, _, err := state.FindLombardPoolConfigPDA(input.TokenPool)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive config PDA: %w", err)
		}

		// Lombard's global config (PoolConfig{version}) isn't built on BaseConfig the way
		// BurnMint/LockRelease's is - it's a single version byte - so the shared
		// initGlobalConfigTokenPool helper's idempotency check (which reads
		// BaseConfig.TokenProgram) doesn't apply here; check PoolConfig.Version directly.
		poolConfig, err := common.GetInitializedAccount(b.GetContext(), chain.Client, configPDA, input.TokenPool, cldf_solana.SolDefaultCommitment, lombard_token_pool.ParseAccount_PoolConfig)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		if poolConfig != nil && poolConfig.Version != 0 {
			b.Logger.Info("LombardTokenPool global config already initialized for token pool:", input.TokenPool.String())
			return sequences.OnChainOutput{}, nil
		}

		ixn, err := lombard_token_pool.NewInitGlobalConfigInstruction(
			configPDA,
			upgradeAuthority,
			solana.SystemProgramID,
			input.TokenPool,
			programData.Address,
		)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		return solcommon.ConfirmOrBatch(chain, upgradeAuthority, []solana.Instruction{ixn}, input.TokenPool, LombardTokenPool.String())
	},
)

// LombardRemoteChainConfig extends RemoteChainConfig with the two Lombard-specific fields
// (destination_chain_id, destination_caller) carried by the on-chain LombardChain struct.
// BurnMint/LockRelease have no equivalent - only Lombard's bridge needs a per-remote-chain
// routing target on top of the generic CCIP remote-pool/token config.
type LombardRemoteChainConfig struct {
	RemoteChainConfig
	DestinationChainID [32]byte
	DestinationCaller  [32]byte
}

var UpsertRemoteChainConfigLombard = operations.NewOperation(
	"lombard:init_chain_remote_config",
	solcommon.Version2_0_0,
	"Initializes the LombardTokenPool chain remote config",
	func(b operations.Bundle, chain cldf_solana.Chain, input LombardRemoteChainConfig) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&lombard_token_pool.ProgramID, lombard_token_pool.ParseInstructionWithoutAccounts, input.TokenPool)
		remoteConfig := lombard_token_pool.RemoteConfig{
			PoolAddresses: []lombard_token_pool.RemoteAddress{},
			TokenAddress: lombard_token_pool.RemoteAddress{
				Address: input.RemoteTokenAddress,
			},
			Decimals: input.RemoteDecimals,
		}
		authority, err := poolAuthority(b.GetContext(), chain, input.TokenPool, input.TokenMint)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to resolve lombard token pool authority: %w", err)
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
		existing, err := common.GetInitializedAccount(b.GetContext(), chain.Client, remoteChainConfigPDA, input.TokenPool, cldf_solana.SolDefaultCommitment, lombard_token_pool.ParseAccount_ChainConfig)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		isSupportedChain := existing != nil
		var existingConfig lombard_token_pool.ChainConfig
		if isSupportedChain {
			existingConfig = *existing
		}
		var ixns []solana.Instruction
		if isSupportedChain {
			remoteConfig.PoolAddresses = append(remoteConfig.PoolAddresses,
				lombard_token_pool.RemoteAddress{
					Address: input.RemotePoolAddress,
				})
			// if the token address has changed or if the override config flag is set, edit the remote config (just overwrite the existing remote config)
			if !bytes.Equal(existingConfig.Base.Remote.TokenAddress.Address, input.RemoteTokenAddress) || input.ForceOverrideRemoteConfig {
				var ixn solana.Instruction
				ixn, err = lombard_token_pool.NewEditChainRemoteConfigInstruction(
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
				poolAddresses := existingConfig.Base.Remote.PoolAddresses
				// translate to base
				baseAddresses := make([]lombard_token_pool.RemoteAddress, len(poolAddresses))
				for i, cfg := range poolAddresses {
					baseAddresses[i] = lombard_token_pool.RemoteAddress{
						Address: cfg.Address,
					}
				}
				diff := poolDiff(baseAddresses, remoteConfig.PoolAddresses, func(a lombard_token_pool.RemoteAddress) []byte { return a.Address })
				if len(diff) > 0 {
					var ixn solana.Instruction
					ixn, err = lombard_token_pool.NewAppendRemotePoolAddressesInstruction(
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
			// Lombard's own dest_chain_id/dest_caller fields live outside the generic CCIP
			// RemoteConfig and are edited via a dedicated instruction.
			if existingConfig.Bridge.DestinationChainId != input.DestinationChainID ||
				existingConfig.Bridge.DestinationCaller != input.DestinationCaller {
				var ixn solana.Instruction
				ixn, err = lombard_token_pool.NewEditChainRemoteConfigLombardInstruction(
					input.RemoteSelector,
					input.TokenMint,
					lombard_token_pool.LombardChain{
						DestinationChainId: input.DestinationChainID,
						DestinationCaller:  input.DestinationCaller,
					},
					poolConfigPDA,
					remoteChainConfigPDA,
					authority,
				)
				if err != nil {
					return sequences.OnChainOutput{}, err
				}
				ixns = append(ixns, ixn)
			}
		} else {
			var ixn solana.Instruction
			ixn, err = lombard_token_pool.NewInitChainRemoteConfigInstruction(
				input.RemoteSelector,
				input.TokenMint,
				remoteConfig,
				input.DestinationChainID,
				input.DestinationCaller,
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
			appendIxn, err = lombard_token_pool.NewAppendRemotePoolAddressesInstruction(
				input.RemoteSelector,
				input.TokenMint,
				[]lombard_token_pool.RemoteAddress{
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
			rlIxn, err1 := lombard_token_pool.NewSetChainRateLimitInstruction(
				input.RemoteSelector,
				input.TokenMint,
				lombard_token_pool.RateLimitConfig{
					Enabled:  false,
					Capacity: 0,
					Rate:     0,
				},
				lombard_token_pool.RateLimitConfig{
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
			ixns = append(ixns, rlIxn)
		}
		if len(ixns) == 0 {
			b.Logger.Infow("LombardTokenPool remote chain config already up to date, skipping",
				"tokenPool", input.TokenPool.String(), "remoteChainSelector", input.RemoteSelector)
			return sequences.OnChainOutput{}, nil
		}
		return solcommon.ConfirmOrBatch(chain, authority, ixns, input.TokenPool, LombardTokenPool.String())
	})

var UpsertRateLimitsLombard = operations.NewOperation(
	"lombard:rate_limits",
	solcommon.Version2_0_0,
	"Initializes the LombardTokenPool rate limits for a remote chain",
	func(b operations.Bundle, chain cldf_solana.Chain, input RemoteChainConfig) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&lombard_token_pool.ProgramID, lombard_token_pool.ParseInstructionWithoutAccounts, input.TokenPool)
		if !input.InboundRateLimiterConfig.Capacity.IsUint64() || !input.InboundRateLimiterConfig.Rate.IsUint64() {
			return sequences.OnChainOutput{}, errors.New("invalid inbound capacity/inbound rate")
		}
		inboundCapacity := input.InboundRateLimiterConfig.Capacity.Uint64()
		inboundRate := input.InboundRateLimiterConfig.Rate.Uint64()
		if inboundRate > inboundCapacity {
			return sequences.OnChainOutput{}, errors.New("inbound rate > capacity")
		}
		inbound := lombard_token_pool.RateLimitConfig{
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
		outbound := lombard_token_pool.RateLimitConfig{
			Enabled:  input.OutboundRateLimiterConfig.IsEnabled,
			Capacity: outboundCapacity,
			Rate:     outboundRate,
		}
		authority, err := poolAuthority(b.GetContext(), chain, input.TokenPool, input.TokenMint)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to resolve lombard token pool authority: %w", err)
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
		ixn, err := lombard_token_pool.NewSetChainRateLimitInstruction(
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
		return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{ixn}, input.TokenPool, LombardTokenPool.String())
	})

// SetChainConfigV2LombardParams are the parameters for SetChainConfigV2Lombard.
type SetChainConfigV2LombardParams struct {
	TokenPool              solana.PublicKey
	TokenMint              solana.PublicKey
	RemoteChainSelector    uint64
	AllowedFinalityConfig  lombard_token_pool.FinalityConfig
	TokenTransferFeeConfig lombard_token_pool.TokenTransferFeeConfig
	CcvConfig              lombard_token_pool.CcvConfig
}

// SetChainConfigV2Lombard writes the CCIP 2.0 lane config on the pool: finality policy,
// transfer fee schedule, and CCV config. The on-chain account is init_if_needed, so this both
// creates and updates it. Mirrors SetChainConfigV2Cctp.
var SetChainConfigV2Lombard = operations.NewOperation(
	"lombard:set_chain_config_v2",
	solcommon.Version2_0_0,
	"Sets the LombardTokenPool v2 chain config for a remote chain",
	func(b operations.Bundle, chain cldf_solana.Chain, input SetChainConfigV2LombardParams) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&lombard_token_pool.ProgramID, lombard_token_pool.ParseInstructionWithoutAccounts, input.TokenPool)
		authority, err := GetAuthorityLombard(b.GetContext(), chain, input.TokenPool, input.TokenMint)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to get lombard pool authority: %w", err)
		}
		poolConfigPDA, err := tokens.TokenPoolConfigAddress(input.TokenMint, input.TokenPool)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("derive token pool config address: %w", err)
		}
		chainConfigV2PDA, _, err := tokens.TokenPoolChainConfigV2PDA(input.RemoteChainSelector, input.TokenMint, input.TokenPool)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("derive chain config v2 PDA: %w", err)
		}
		eventAuthority, _, err := state.FindEventAuthorityPDA(input.TokenPool)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("derive event authority PDA: %w", err)
		}

		params := lombard_token_pool.SetChainConfigV2Params{
			Mint:                   input.TokenMint,
			RemoteChainSelector:    input.RemoteChainSelector,
			AllowedFinalityConfig:  input.AllowedFinalityConfig,
			TokenTransferFeeConfig: input.TokenTransferFeeConfig,
			CcvConfig:              input.CcvConfig,
		}

		existing, err := common.GetInitializedAccount(b.GetContext(), chain.Client, chainConfigV2PDA, input.TokenPool, cldf_solana.SolDefaultCommitment, lombard_token_pool.ParseAccount_ChainConfigV2)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		if existing != nil &&
			existing.AllowedFinalityConfig == params.AllowedFinalityConfig &&
			existing.TokenTransferFeeConfig == params.TokenTransferFeeConfig &&
			lombardCcvConfigEqual(existing.CcvConfig, params.CcvConfig) {
			b.Logger.Infow("LombardTokenPool v2 chain config already set, skipping",
				"tokenPool", input.TokenPool.String(), "remoteChainSelector", input.RemoteChainSelector)
			return sequences.OnChainOutput{}, nil
		}

		var ixn solana.Instruction
		if existing != nil {
			ixn, err = lombard_token_pool.NewEditChainConfigV2Instruction(
				params, poolConfigPDA, chainConfigV2PDA, authority, solana.SystemProgramID,
				eventAuthority, input.TokenPool,
			)
		} else {
			ixn, err = lombard_token_pool.NewInitChainConfigV2Instruction(
				params, poolConfigPDA, chainConfigV2PDA, authority, solana.SystemProgramID,
				eventAuthority, input.TokenPool,
			)
		}
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("build set_chain_config_v2 instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{ixn}, input.TokenPool, LombardTokenPool.String())
	},
)

// lombardCcvConfigEqual mirrors ccvConfigEqual for the Lombard pool bindings.
func lombardCcvConfigEqual(a, b lombard_token_pool.CcvConfig) bool {
	return a.ThresholdAmount == b.ThresholdAmount &&
		pubkeysEqual(a.OutboundCcvs, b.OutboundCcvs) &&
		pubkeysEqual(a.ThresholdOutboundCcvs, b.ThresholdOutboundCcvs) &&
		pubkeysEqual(a.InboundCcvs, b.InboundCcvs) &&
		pubkeysEqual(a.ThresholdInboundCcvs, b.ThresholdInboundCcvs)
}

var TransferOwnershipLombard = operations.NewOperation(
	"lombard:transfer-ownership",
	solcommon.Version2_0_0,
	"Transfers ownership of the LombardTokenPool token mint PDA to a new authority",
	func(b operations.Bundle, chain cldf_solana.Chain, input TokenPoolTransferOwnershipInput) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&lombard_token_pool.ProgramID, lombard_token_pool.ParseInstructionWithoutAccounts, input.Program)
		authority, err := poolAuthority(b.GetContext(), chain, input.Program, input.TokenMint)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to resolve lombard token pool authority: %w", err)
		}
		if authority == input.NewOwner {
			b.Logger.Info("New owner is the same as the current owner for lombard token pool with token mint:", input.TokenMint.String())
			return sequences.OnChainOutput{}, nil
		}
		tokenPoolConfigPDA, err := tokens.TokenPoolConfigAddress(input.TokenMint, input.Program)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive token pool config address: %w", err)
		}
		ixn, err := lombard_token_pool.NewTransferOwnershipInstruction(
			input.NewOwner,
			tokenPoolConfigPDA,
			input.TokenMint,
			authority,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to build transfer ownership instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{ixn}, input.Program, LombardTokenPool.String())
	},
)

var AcceptOwnershipLombard = operations.NewOperation(
	"lombard:accept-ownership",
	solcommon.Version2_0_0,
	"Accepts ownership of the LombardTokenPool token mint PDA",
	func(b operations.Bundle, chain cldf_solana.Chain, input TokenPoolTransferOwnershipInput) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&lombard_token_pool.ProgramID, lombard_token_pool.ParseInstructionWithoutAccounts, input.Program)
		tokenPoolConfigPDA, err := tokens.TokenPoolConfigAddress(input.TokenMint, input.Program)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive token pool config address: %w", err)
		}
		authority, err := poolAuthority(b.GetContext(), chain, input.Program, input.TokenMint)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to resolve lombard token pool authority: %w", err)
		}
		if authority == input.NewOwner {
			b.Logger.Info("New owner is the same as the current owner for lombard token pool with token mint:", input.TokenMint.String())
			return sequences.OnChainOutput{}, nil
		}
		ixn, err := lombard_token_pool.NewAcceptOwnershipInstruction(
			tokenPoolConfigPDA,
			input.TokenMint,
			input.NewOwner,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to build accept ownership instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, input.NewOwner, []solana.Instruction{ixn}, input.Program, LombardTokenPool.String())
	},
)

var UpdateRateLimitAdminLombard = operations.NewOperation(
	"lombard:update-rate-limit-admin",
	solcommon.Version2_0_0,
	"Updates the rate limit admin for a LombardTokenPool remote chain config",
	func(b operations.Bundle, chain cldf_solana.Chain, input TokenPoolTransferOwnershipInput) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&lombard_token_pool.ProgramID, lombard_token_pool.ParseInstructionWithoutAccounts, input.Program)
		poolConfigPDA, err := tokens.TokenPoolConfigAddress(input.TokenMint, input.Program)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive token pool config address: %w", err)
		}
		state, err := readPoolState(b.GetContext(), chain, input.Program, input.TokenMint)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		if state != nil && state.Config.RateLimitAdmin == input.NewOwner {
			b.Logger.Info("New owner is the same as the current rate limit admin for lombard token pool with token mint:", input.TokenMint.String())
			return sequences.OnChainOutput{}, nil
		}
		authority, err := authorityFromPoolState(chain, input.Program, state)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to resolve lombard token pool authority: %w", err)
		}
		// Before initialization the upgrade authority signs, and the initializer sets the admin, so
		// there is nothing to update when the upgrade authority is already the requested key.
		if state == nil && authority == input.NewOwner {
			b.Logger.Info("New owner is the same as the current owner for lombard token pool with token mint:", input.TokenMint.String())
			return sequences.OnChainOutput{}, nil
		}
		ixn, err := lombard_token_pool.NewSetRateLimitAdminInstruction(
			input.TokenMint,
			input.NewOwner,
			poolConfigPDA,
			authority,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to build update rate limit admin instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{ixn}, input.Program, LombardTokenPool.String())
	},
)

// GetAuthorityLombard reuses the shared getTokenPoolAuthority helper: Lombard's State account
// starts with the same {version, config BaseConfig} prefix as BurnMint/LockRelease's (Bridge/Alt
// are trailing fields Borsh never reads unless asked to), so decoding into burnmint_token_pool.State
// still yields the correct Config.Owner.
func GetAuthorityLombard(ctx context.Context, chain cldf_solana.Chain, program solana.PublicKey, tokenMint solana.PublicKey) (solana.PublicKey, error) {
	return getTokenPoolAuthority(ctx, chain, program, tokenMint)
}
