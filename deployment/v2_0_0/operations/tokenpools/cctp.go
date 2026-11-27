package tokenpools

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	token_deployments "github.com/smartcontractkit/chainlink-ccip/deployment/tokens"
	common_utils "github.com/smartcontractkit/chainlink-ccip/deployment/utils"
	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/sequences"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/cctp_token_pool"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/tokens"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
)

var CctpProgramName = "cctp_token_pool"

// DefaultPoolSignerLamports is the balance FundPoolSignerCctp tops the pool signer PDA up to.
// The PDA pays rent for the CCTP MessageSent event accounts created on every outbound transfer
// and for the lazily-created per-sender Nonce PDAs, so it must hold SOL before the first send.
const DefaultPoolSignerLamports = 10 * solana.LAMPORTS_PER_SOL

// CctpRemoteChainConfig extends the shared RemoteChainConfig with the pool-side CCTP settings
// carried by ChainConfig.cctp: the remote's Circle domain and the caller allowed to trigger
// message reception there. This is the CCTP v1 (CCIP 1.6) half; the verifier's own per-remote
// CCTP config is written separately by cctpverifier.SetChainConfigCctp.
type CctpRemoteChainConfig struct {
	RemoteChainConfig
	// DomainIdentifier is the remote chain's Circle CCTP domain.
	DomainIdentifier uint32
	// DestinationCaller is the address allowed to call receiveMessage on the remote domain.
	DestinationCaller solana.PublicKey
}

// CctpChainConfigV2Params configures a lane's CCIP 2.0 pool settings: the finality policy, the
// token transfer fee schedule, and which CCVs guard the lane.
type CctpChainConfigV2Params struct {
	TokenPool              solana.PublicKey
	TokenMint              solana.PublicKey
	RemoteChainSelector    uint64
	AllowedFinalityConfig  cctp_token_pool.FinalityConfig
	TokenTransferFeeConfig cctp_token_pool.TokenTransferFeeConfig
	CcvConfig              cctp_token_pool.CcvConfig
}

// CctpFundingParams sets one of the three fields of the pool's FundingConfig. Only the field
// matching the operation is read.
type CctpFundingParams struct {
	TokenPool solana.PublicKey
	TokenMint solana.PublicKey
	// FundManager may call reclaim_funds and reclaim_event_account.
	FundManager solana.PublicKey
	// ReclaimDestination is where reclaim_funds sends SOL; reclaim_funds fails while it is unset.
	ReclaimDestination solana.PublicKey
	// MinimumSignerFunds is the lamport floor the pool signer must retain across a reclaim.
	MinimumSignerFunds uint64
}

// FundPoolSignerParams tops the pool signer PDA up to TargetLamports.
type FundPoolSignerParams struct {
	TokenPool solana.PublicKey
	TokenMint solana.PublicKey
	// TargetLamports defaults to DefaultPoolSignerLamports when zero.
	TargetLamports uint64
}

var DeployCctp = operations.NewOperation(
	"cctp:deploy",
	solcommon.Version2_0_0,
	"Deploys the CCTPTokenPool program",
	func(b operations.Bundle, chain cldf_solana.Chain, input []datastore.AddressRef) (datastore.AddressRef, error) {
		return solcommon.DeployContractIfNotInDatastore(
			b,
			chain,
			input,
			common_utils.CCTPTokenPool,
			solcommon.Version2_0_0,
			solcommon.PoolProgramQualifier,
			CctpProgramName)
	},
)

var UpgradeCctp = operations.NewOperation(
	"cctp:upgrade",
	solcommon.Version2_0_0,
	"Upgrades the default CCTPTokenPool program",
	func(b operations.Bundle, chain cldf_solana.Chain, programID solana.PublicKey) ([]solana.Instruction, error) {
		return solcommon.UpgradeProgram(
			b,
			chain,
			common_utils.CCTPTokenPool,
			programID,
			CctpProgramName,
		)
	},
)

// InitializeCctp creates the per-mint pool State. The program enforces that the signer is the
// program upgrade authority, so unlike BurnMint there is no self-serve path.
var InitializeCctp = operations.NewOperation(
	"cctp:initialize",
	solcommon.Version2_0_0,
	"Initializes the CCTPTokenPool program for a token mint",
	func(b operations.Bundle, chain cldf_solana.Chain, input Params) (PoolInitializeOut, error) {
		if err := requireProgramCctp(chain, input.TokenPool); err != nil {
			return PoolInitializeOut{}, err
		}
		if input.TokenMint.IsZero() {
			return PoolInitializeOut{}, errors.New("token mint is required")
		}

		poolConfigPDA, err := tokens.TokenPoolConfigAddress(input.TokenMint, input.TokenPool)
		if err != nil {
			return PoolInitializeOut{}, fmt.Errorf("derive token pool config address: %w", err)
		}
		initialized, err := common.IsAccountInitialized(b.GetContext(), chain.Client, poolConfigPDA, input.TokenPool, cldf_solana.SolDefaultCommitment, cctp_token_pool.Account_State)
		if err != nil {
			return PoolInitializeOut{}, err
		}
		if initialized {
			b.Logger.Infow("CCTPTokenPool already initialized for token mint, skipping",
				"tokenMint", input.TokenMint.String(), "tokenPool", input.TokenPool.String())
			return PoolInitializeOut{}, nil
		}

		programData, err := solcommon.GetSolProgramData(chain.Client, input.TokenPool)
		if err != nil {
			return PoolInitializeOut{}, fmt.Errorf("get program data: %w", err)
		}
		upgradeAuthority, err := solcommon.GetUpgradeAuthority(chain.Client, input.TokenPool)
		if err != nil {
			return PoolInitializeOut{}, fmt.Errorf("get upgrade authority: %w", err)
		}
		configPDA, err := tokens.TokenPoolGlobalConfigPDA(input.TokenPool)
		if err != nil {
			return PoolInitializeOut{}, fmt.Errorf("derive global config PDA: %w", err)
		}

		ixn, err := cctp_token_pool.NewInitializeInstruction(
			input.Router,
			input.RMNRemote,
			poolConfigPDA,
			input.TokenMint,
			upgradeAuthority,
			solana.SystemProgramID,
			input.TokenPool,
			programData.Address,
			configPDA,
		)
		if err != nil {
			return PoolInitializeOut{}, fmt.Errorf("build initialize instruction: %w", err)
		}

		out, err := solcommon.ConfirmOrBatch(chain, upgradeAuthority, []solana.Instruction{ixn}, input.TokenPool, common_utils.CCTPTokenPool.String())
		if err != nil {
			return PoolInitializeOut{}, err
		}
		return PoolInitializeOut{OnChainOutput: out, Initializer: upgradeAuthority}, nil
	},
)

// UpsertRemoteChainConfigCctp brings a lane's pool-side ChainConfig to the desired state. It
// mirrors UpsertRemoteChainConfigBurnMint, plus the CCTP-only edit_chain_remote_config_cctp
// instruction that writes the remote's Circle domain and destination caller.
var UpsertRemoteChainConfigCctp = operations.NewOperation(
	"cctp:upsert_remote_chain_config",
	solcommon.Version2_0_0,
	"Creates or updates the CCTPTokenPool chain remote config",
	func(b operations.Bundle, chain cldf_solana.Chain, input CctpRemoteChainConfig) (sequences.OnChainOutput, error) {
		if err := requireProgramCctp(chain, input.TokenPool); err != nil {
			return sequences.OnChainOutput{}, err
		}

		authority, err := poolAuthorityCctp(b.GetContext(), chain, input.TokenPool, input.TokenMint)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		poolConfigPDA, err := tokens.TokenPoolConfigAddress(input.TokenMint, input.TokenPool)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("derive token pool config address: %w", err)
		}
		remoteChainConfigPDA, _, err := tokens.TokenPoolChainConfigPDA(input.RemoteSelector, input.TokenMint, input.TokenPool)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("derive remote chain config PDA: %w", err)
		}

		remoteConfig := cctp_token_pool.RemoteConfig{
			PoolAddresses: []cctp_token_pool.RemoteAddress{},
			TokenAddress:  cctp_token_pool.RemoteAddress{Address: input.RemoteTokenAddress},
			Decimals:      input.RemoteDecimals,
		}
		desiredCctp := cctp_token_pool.CctpChain{
			DomainId:          input.DomainIdentifier,
			DestinationCaller: input.DestinationCaller,
		}

		existing, err := common.GetInitializedAccount(b.GetContext(), chain.Client, remoteChainConfigPDA, input.TokenPool, cldf_solana.SolDefaultCommitment, cctp_token_pool.ParseAccount_ChainConfig)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		isSupportedChain := existing != nil

		var ixns []solana.Instruction
		if isSupportedChain {
			remoteConfig.PoolAddresses = append(remoteConfig.PoolAddresses,
				cctp_token_pool.RemoteAddress{Address: input.RemotePoolAddress})

			// A changed remote token means the whole remote config is stale, so overwrite it
			// rather than appending to it.
			if !bytes.Equal(existing.Base.Remote.TokenAddress.Address, input.RemoteTokenAddress) || input.ForceOverrideRemoteConfig {
				ixn, ierr := cctp_token_pool.NewEditChainRemoteConfigInstruction(
					input.RemoteSelector, input.TokenMint, remoteConfig,
					poolConfigPDA, remoteChainConfigPDA, authority, solana.SystemProgramID,
				)
				if ierr != nil {
					return sequences.OnChainOutput{}, fmt.Errorf("build edit_chain_remote_config instruction: %w", ierr)
				}
				ixns = append(ixns, ixn)
			} else {
				baseAddresses := make([]cctp_token_pool.RemoteAddress, len(existing.Base.Remote.PoolAddresses))
				copy(baseAddresses, existing.Base.Remote.PoolAddresses)
				diff := poolDiff(baseAddresses, remoteConfig.PoolAddresses,
					func(a cctp_token_pool.RemoteAddress) []byte { return a.Address })
				if len(diff) > 0 {
					ixn, ierr := cctp_token_pool.NewAppendRemotePoolAddressesInstruction(
						input.RemoteSelector, input.TokenMint, diff,
						poolConfigPDA, remoteChainConfigPDA, authority, solana.SystemProgramID,
					)
					if ierr != nil {
						return sequences.OnChainOutput{}, fmt.Errorf("build append_remote_pool_addresses instruction: %w", ierr)
					}
					ixns = append(ixns, ixn)
				}
			}

			if existing.Cctp != desiredCctp {
				ixn, ierr := cctp_token_pool.NewEditChainRemoteConfigCctpInstruction(
					input.RemoteSelector, input.TokenMint, desiredCctp,
					poolConfigPDA, remoteChainConfigPDA, authority,
				)
				if ierr != nil {
					return sequences.OnChainOutput{}, fmt.Errorf("build edit_chain_remote_config_cctp instruction: %w", ierr)
				}
				ixns = append(ixns, ixn)
			}
		} else {
			// init_chain_remote_config rejects a non-empty pool address list, so the addresses
			// are appended in a follow-up instruction.
			initIxn, ierr := cctp_token_pool.NewInitChainRemoteConfigInstruction(
				input.RemoteSelector, input.TokenMint, remoteConfig,
				poolConfigPDA, remoteChainConfigPDA, authority, solana.SystemProgramID,
			)
			if ierr != nil {
				return sequences.OnChainOutput{}, fmt.Errorf("build init_chain_remote_config instruction: %w", ierr)
			}
			appendIxn, ierr := cctp_token_pool.NewAppendRemotePoolAddressesInstruction(
				input.RemoteSelector, input.TokenMint,
				[]cctp_token_pool.RemoteAddress{{Address: input.RemotePoolAddress}},
				poolConfigPDA, remoteChainConfigPDA, authority, solana.SystemProgramID,
			)
			if ierr != nil {
				return sequences.OnChainOutput{}, fmt.Errorf("build append_remote_pool_addresses instruction: %w", ierr)
			}
			cctpIxn, ierr := cctp_token_pool.NewEditChainRemoteConfigCctpInstruction(
				input.RemoteSelector, input.TokenMint, desiredCctp,
				poolConfigPDA, remoteChainConfigPDA, authority,
			)
			if ierr != nil {
				return sequences.OnChainOutput{}, fmt.Errorf("build edit_chain_remote_config_cctp instruction: %w", ierr)
			}
			// Seed the rate limits disabled; UpsertRateLimitsCctp sets the real values.
			disabled := cctp_token_pool.RateLimitConfig{}
			rateLimitIxn, ierr := cctp_token_pool.NewSetChainRateLimitInstruction(
				input.RemoteSelector, input.TokenMint, disabled, disabled,
				poolConfigPDA, remoteChainConfigPDA, authority,
			)
			if ierr != nil {
				return sequences.OnChainOutput{}, fmt.Errorf("build set_chain_rate_limit instruction: %w", ierr)
			}
			// Same order the program-level integration tests use
			// (contracts/tests/testutils/setup.go): create, then CCTP settings, then rate
			// limits, then the pool addresses that realloc the account.
			ixns = append(ixns, initIxn, cctpIxn, rateLimitIxn, appendIxn)
		}

		if len(ixns) == 0 {
			b.Logger.Infow("CCTPTokenPool remote chain config already up to date, skipping",
				"tokenPool", input.TokenPool.String(), "remoteChainSelector", input.RemoteSelector)
			return sequences.OnChainOutput{}, nil
		}
		return solcommon.ConfirmOrBatch(chain, authority, ixns, input.TokenPool, common_utils.CCTPTokenPool.String())
	},
)

var UpsertRateLimitsCctp = operations.NewOperation(
	"cctp:rate_limits",
	solcommon.Version2_0_0,
	"Sets the CCTPTokenPool rate limits for a remote chain",
	func(b operations.Bundle, chain cldf_solana.Chain, input CctpRemoteChainConfig) (sequences.OnChainOutput, error) {
		if err := requireProgramCctp(chain, input.TokenPool); err != nil {
			return sequences.OnChainOutput{}, err
		}

		inbound, err := toRateLimitConfig("inbound", input.InboundRateLimiterConfig)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		outbound, err := toRateLimitConfig("outbound", input.OutboundRateLimiterConfig)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}

		poolConfigPDA, err := tokens.TokenPoolConfigAddress(input.TokenMint, input.TokenPool)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("derive token pool config address: %w", err)
		}
		remoteChainConfigPDA, _, err := tokens.TokenPoolChainConfigPDA(input.RemoteSelector, input.TokenMint, input.TokenPool)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("derive remote chain config PDA: %w", err)
		}

		// The rate limit admin may differ from the owner, and either is accepted on-chain; prefer
		// the admin when one is configured so the deployer keeps a direct-send path.
		authority, err := rateLimitAuthorityCctp(b.GetContext(), chain, input.TokenPool, input.TokenMint)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		ixn, err := cctp_token_pool.NewSetChainRateLimitInstruction(
			input.RemoteSelector, input.TokenMint, inbound, outbound,
			poolConfigPDA, remoteChainConfigPDA, authority,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("build set_chain_rate_limit instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{ixn}, input.TokenPool, common_utils.CCTPTokenPool.String())
	},
)

// GetChainConfigV2Cctp reads a CCTPTokenPool's CCIP 2.0 lane config.
// found is false when the lane has no v2 config yet.
func GetChainConfigV2Cctp(b operations.Bundle, chain cldf_solana.Chain, tokenPool, tokenMint solana.PublicKey, remoteSelector uint64) (cfg cctp_token_pool.ChainConfigV2, found bool, err error) {
	pda, _, err := tokens.TokenPoolChainConfigV2PDA(remoteSelector, tokenMint, tokenPool)
	if err != nil {
		return cfg, false, fmt.Errorf("derive chain config v2 PDA: %w", err)
	}
	err = common.GetAccountDataBorshIntoAnchor(b.GetContext(), chain.Client, pda, cldf_solana.SolDefaultCommitment, &cfg)
	if errors.Is(err, rpc.ErrNotFound) {
		return cfg, false, nil
	}
	if err != nil {
		return cfg, false, fmt.Errorf("decode chain config v2 at PDA %s for remote %d: %w", pda, remoteSelector, err)
	}
	return cfg, true, nil
}

// SetChainConfigV2Cctp writes the CCIP 2.0 lane config: finality policy, transfer fee schedule,
// and the CCVs guarding the lane. The on-chain account is init_if_needed, so this both creates
// and updates it.
var SetChainConfigV2Cctp = operations.NewOperation(
	"cctp:set_chain_config_v2",
	solcommon.Version2_0_0,
	"Sets the CCTPTokenPool v2 chain config for a remote chain",
	func(b operations.Bundle, chain cldf_solana.Chain, input CctpChainConfigV2Params) (sequences.OnChainOutput, error) {
		if err := requireProgramCctp(chain, input.TokenPool); err != nil {
			return sequences.OnChainOutput{}, err
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

		params := cctp_token_pool.SetChainConfigV2Params{
			Mint:                   input.TokenMint,
			RemoteChainSelector:    input.RemoteChainSelector,
			AllowedFinalityConfig:  input.AllowedFinalityConfig,
			TokenTransferFeeConfig: input.TokenTransferFeeConfig,
			CcvConfig:              input.CcvConfig,
		}

		existing, err := common.GetInitializedAccount(b.GetContext(), chain.Client, chainConfigV2PDA, input.TokenPool, cldf_solana.SolDefaultCommitment, cctp_token_pool.ParseAccount_ChainConfigV2)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		if existing != nil &&
			existing.AllowedFinalityConfig == params.AllowedFinalityConfig &&
			existing.TokenTransferFeeConfig == params.TokenTransferFeeConfig &&
			ccvConfigEqual(existing.CcvConfig, params.CcvConfig) {
			b.Logger.Infow("CCTPTokenPool v2 chain config already set, skipping",
				"tokenPool", input.TokenPool.String(), "remoteChainSelector", input.RemoteChainSelector)
			return sequences.OnChainOutput{}, nil
		}

		authority, err := poolAuthorityCctp(b.GetContext(), chain, input.TokenPool, input.TokenMint)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		var ixn solana.Instruction
		if existing != nil {
			ixn, err = cctp_token_pool.NewEditChainConfigV2Instruction(
				params, poolConfigPDA, chainConfigV2PDA, authority, solana.SystemProgramID,
				eventAuthority, input.TokenPool,
			)
		} else {
			ixn, err = cctp_token_pool.NewInitChainConfigV2Instruction(
				params, poolConfigPDA, chainConfigV2PDA, authority, solana.SystemProgramID,
				eventAuthority, input.TokenPool,
			)
		}
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("build set_chain_config_v2 instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{ixn}, input.TokenPool, common_utils.CCTPTokenPool.String())
	},
)

var TransferOwnershipCctp = operations.NewOperation(
	"cctp:transfer-ownership",
	solcommon.Version2_0_0,
	"Proposes a new owner for the CCTPTokenPool",
	func(b operations.Bundle, chain cldf_solana.Chain, input TokenPoolTransferOwnershipInput) (sequences.OnChainOutput, error) {
		if err := requireProgramCctp(chain, input.Program); err != nil {
			return sequences.OnChainOutput{}, err
		}

		authority, err := poolAuthorityCctp(b.GetContext(), chain, input.Program, input.TokenMint)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		if authority == input.NewOwner {
			b.Logger.Infow("CCTPTokenPool owner unchanged, skipping",
				"tokenMint", input.TokenMint.String(), "owner", input.NewOwner.String())
			return sequences.OnChainOutput{}, nil
		}
		poolConfigPDA, err := tokens.TokenPoolConfigAddress(input.TokenMint, input.Program)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("derive token pool config address: %w", err)
		}
		ixn, err := cctp_token_pool.NewTransferOwnershipInstruction(
			input.NewOwner, poolConfigPDA, input.TokenMint, authority,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("build transfer_ownership instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{ixn}, input.Program, common_utils.CCTPTokenPool.String())
	},
)

var AcceptOwnershipCctp = operations.NewOperation(
	"cctp:accept-ownership",
	solcommon.Version2_0_0,
	"Accepts a pending CCTPTokenPool ownership transfer",
	func(b operations.Bundle, chain cldf_solana.Chain, input TokenPoolTransferOwnershipInput) (sequences.OnChainOutput, error) {
		if err := requireProgramCctp(chain, input.Program); err != nil {
			return sequences.OnChainOutput{}, err
		}

		authority, err := poolAuthorityCctp(b.GetContext(), chain, input.Program, input.TokenMint)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		if authority == input.NewOwner {
			b.Logger.Infow("CCTPTokenPool ownership already accepted, skipping",
				"tokenMint", input.TokenMint.String(), "owner", input.NewOwner.String())
			return sequences.OnChainOutput{}, nil
		}
		poolConfigPDA, err := tokens.TokenPoolConfigAddress(input.TokenMint, input.Program)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("derive token pool config address: %w", err)
		}
		ixn, err := cctp_token_pool.NewAcceptOwnershipInstruction(
			poolConfigPDA, input.TokenMint, input.NewOwner,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("build accept_ownership instruction: %w", err)
		}
		// accept_ownership is signed by the proposed owner, not the current one.
		return solcommon.ConfirmOrBatch(chain, input.NewOwner, []solana.Instruction{ixn}, input.Program, common_utils.CCTPTokenPool.String())
	},
)

var UpdateRateLimitAdminCctp = operations.NewOperation(
	"cctp:update-rate-limit-admin",
	solcommon.Version2_0_0,
	"Updates the rate limit admin for the CCTPTokenPool",
	func(b operations.Bundle, chain cldf_solana.Chain, input TokenPoolTransferOwnershipInput) (sequences.OnChainOutput, error) {
		if err := requireProgramCctp(chain, input.Program); err != nil {
			return sequences.OnChainOutput{}, err
		}

		poolConfigPDA, err := tokens.TokenPoolConfigAddress(input.TokenMint, input.Program)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("derive token pool config address: %w", err)
		}
		poolState, err := common.GetInitializedAccount(b.GetContext(), chain.Client, poolConfigPDA, input.Program, cldf_solana.SolDefaultCommitment, cctp_token_pool.ParseAccount_State)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		if poolState == nil {
			// The pool isn't initialized yet, so there is no admin to update; initialization will
			// set it. Skip rather than emitting an instruction that is guaranteed to fail.
			b.Logger.Infow("CCTPTokenPool not initialized, skipping rate limit admin update",
				"tokenMint", input.TokenMint.String())
			return sequences.OnChainOutput{}, nil
		}
		if poolState.Config.RateLimitAdmin == input.NewOwner {
			b.Logger.Infow("CCTPTokenPool rate limit admin unchanged, skipping",
				"tokenMint", input.TokenMint.String(), "rateLimitAdmin", input.NewOwner.String())
			return sequences.OnChainOutput{}, nil
		}

		authority, err := poolAuthorityCctp(b.GetContext(), chain, input.Program, input.TokenMint)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		ixn, err := cctp_token_pool.NewSetRateLimitAdminInstruction(
			input.TokenMint, input.NewOwner, poolConfigPDA, authority,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("build set_rate_limit_admin instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{ixn}, input.Program, common_utils.CCTPTokenPool.String())
	},
)

var SetFundManagerCctp = operations.NewOperation(
	"cctp:set-fund-manager",
	solcommon.Version2_0_0,
	"Sets the CCTPTokenPool fund manager",
	func(b operations.Bundle, chain cldf_solana.Chain, input CctpFundingParams) (sequences.OnChainOutput, error) {
		return setFundingConfigCctp(b, chain, input, "fund manager",
			func(f cctp_token_pool.FundingConfig) bool { return f.Manager == input.FundManager },
			func(poolConfigPDA, authority solana.PublicKey) (solana.Instruction, error) {
				return cctp_token_pool.NewSetFundManagerInstruction(input.FundManager, poolConfigPDA, input.TokenMint, authority)
			})
	},
)

var SetFundReclaimDestinationCctp = operations.NewOperation(
	"cctp:set-fund-reclaim-destination",
	solcommon.Version2_0_0,
	"Sets the CCTPTokenPool fund reclaim destination",
	func(b operations.Bundle, chain cldf_solana.Chain, input CctpFundingParams) (sequences.OnChainOutput, error) {
		return setFundingConfigCctp(b, chain, input, "fund reclaim destination",
			func(f cctp_token_pool.FundingConfig) bool { return f.ReclaimDestination == input.ReclaimDestination },
			func(poolConfigPDA, authority solana.PublicKey) (solana.Instruction, error) {
				return cctp_token_pool.NewSetFundReclaimDestinationInstruction(input.ReclaimDestination, poolConfigPDA, input.TokenMint, authority)
			})
	},
)

var SetMinimumSignerFundsCctp = operations.NewOperation(
	"cctp:set-minimum-signer-funds",
	solcommon.Version2_0_0,
	"Sets the CCTPTokenPool minimum signer funds",
	func(b operations.Bundle, chain cldf_solana.Chain, input CctpFundingParams) (sequences.OnChainOutput, error) {
		return setFundingConfigCctp(b, chain, input, "minimum signer funds",
			func(f cctp_token_pool.FundingConfig) bool { return f.MinimumSignerFunds == input.MinimumSignerFunds },
			func(poolConfigPDA, authority solana.PublicKey) (solana.Instruction, error) {
				return cctp_token_pool.NewSetMinimumSignerFundsInstruction(input.MinimumSignerFunds, poolConfigPDA, input.TokenMint, authority)
			})
	},
)

// FundPoolSignerCctp tops the pool signer PDA up to the target balance. The PDA stakes rent for
// the CCTP MessageSent event account created on every outbound transfer and for the lazily
// created per-sender Nonce PDAs, so an unfunded signer fails every send.
var FundPoolSignerCctp = operations.NewOperation(
	"cctp:fund-pool-signer",
	solcommon.Version2_0_0,
	"Tops up the CCTPTokenPool signer PDA with SOL for CCTP event account rent",
	func(b operations.Bundle, chain cldf_solana.Chain, input FundPoolSignerParams) (sequences.OnChainOutput, error) {
		if err := requireProgramCctp(chain, input.TokenPool); err != nil {
			return sequences.OnChainOutput{}, err
		}

		target := input.TargetLamports
		if target == 0 {
			target = DefaultPoolSignerLamports
		}
		poolSigner, err := tokens.TokenPoolSignerAddress(input.TokenMint, input.TokenPool)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("derive pool signer address: %w", err)
		}

		var balance uint64
		if res, berr := chain.Client.GetBalance(b.GetContext(), poolSigner, cldf_solana.SolDefaultCommitment); berr == nil && res != nil {
			balance = res.Value
		}
		if balance >= target {
			b.Logger.Infow("CCTPTokenPool signer already funded, skipping",
				"poolSigner", poolSigner.String(), "balance", balance, "target", target)
			return sequences.OnChainOutput{}, nil
		}

		// Funding is a plain system transfer from the deployer, never an MCMS batch: the timelock
		// holds no SOL to send.
		ixns, err := solcommon.FundFromAddressIxs(chain, chain.Deployer().PublicKey(), []solana.PublicKey{poolSigner}, target-balance)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("build pool signer funding instructions: %w", err)
		}
		if err = chain.Confirm(ixns); err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("confirm pool signer funding: %w", err)
		}
		return sequences.OnChainOutput{}, nil
	},
)

// GetAuthorityCctp reads the pool owner from the on-chain State. CCTP's State shares the
// {version, config BaseConfig} prefix with BurnMint's, so the shared decoder applies.
func GetAuthorityCctp(ctx context.Context, chain cldf_solana.Chain, program, tokenMint solana.PublicKey) (solana.PublicKey, error) {
	return getTokenPoolAuthority(ctx, chain, program, tokenMint)
}

func requireProgramCctp(chain cldf_solana.Chain, program solana.PublicKey) error {
	if program.IsZero() {
		return errors.New("cctp token pool program is required")
	}
	solcommon.SetProgramID(&cctp_token_pool.ProgramID, cctp_token_pool.ParseInstructionWithoutAccounts, program)
	if chain.Deployer() == nil {
		return errors.New("missing Solana deployer key")
	}
	return nil
}

// readPoolStateCctp returns the pool State of tokenMint, or nil before the pool is initialized.
func readPoolStateCctp(ctx context.Context, chain cldf_solana.Chain, program, tokenMint solana.PublicKey) (*cctp_token_pool.State, error) {
	poolConfigPDA, err := tokens.TokenPoolConfigAddress(tokenMint, program)
	if err != nil {
		return nil, fmt.Errorf("derive token pool config address: %w", err)
	}
	return common.GetInitializedAccount(ctx, chain.Client, poolConfigPDA, program, cldf_solana.SolDefaultCommitment, cctp_token_pool.ParseAccount_State)
}

// poolAuthorityCctp returns the pool owner, or the program upgrade authority before the pool is
// initialized.
func poolAuthorityCctp(ctx context.Context, chain cldf_solana.Chain, program, tokenMint solana.PublicKey) (solana.PublicKey, error) {
	poolState, err := readPoolStateCctp(ctx, chain, program, tokenMint)
	if err != nil {
		return solana.PublicKey{}, err
	}
	if poolState != nil {
		return poolState.Config.Owner, nil
	}
	return solcommon.GetUpgradeAuthority(chain.Client, program)
}

// rateLimitAuthorityCctp prefers the configured rate limit admin, which the program accepts
// alongside the owner for set_chain_rate_limit.
func rateLimitAuthorityCctp(ctx context.Context, chain cldf_solana.Chain, program, tokenMint solana.PublicKey) (solana.PublicKey, error) {
	poolState, err := readPoolStateCctp(ctx, chain, program, tokenMint)
	if err != nil {
		return solana.PublicKey{}, err
	}
	if poolState != nil && !poolState.Config.RateLimitAdmin.IsZero() {
		return poolState.Config.RateLimitAdmin, nil
	}
	return poolAuthorityCctp(ctx, chain, program, tokenMint)
}

func setFundingConfigCctp(
	b operations.Bundle,
	chain cldf_solana.Chain,
	input CctpFundingParams,
	field string,
	upToDate func(cctp_token_pool.FundingConfig) bool,
	build func(poolConfigPDA, authority solana.PublicKey) (solana.Instruction, error),
) (sequences.OnChainOutput, error) {
	if err := requireProgramCctp(chain, input.TokenPool); err != nil {
		return sequences.OnChainOutput{}, err
	}

	poolConfigPDA, err := tokens.TokenPoolConfigAddress(input.TokenMint, input.TokenPool)
	if err != nil {
		return sequences.OnChainOutput{}, fmt.Errorf("derive token pool config address: %w", err)
	}
	var poolState cctp_token_pool.State
	if err = common.GetAccountDataBorshIntoAnchor(b.GetContext(), chain.Client, poolConfigPDA, cldf_solana.SolDefaultCommitment, &poolState); err == nil && upToDate(poolState.Funding) {
		b.Logger.Infow("CCTPTokenPool "+field+" already set, skipping",
			"tokenPool", input.TokenPool.String(), "tokenMint", input.TokenMint.String())
		return sequences.OnChainOutput{}, nil
	}

	authority, err := poolAuthorityCctp(b.GetContext(), chain, input.TokenPool, input.TokenMint)
	if err != nil {
		return sequences.OnChainOutput{}, err
	}
	ixn, err := build(poolConfigPDA, authority)
	if err != nil {
		return sequences.OnChainOutput{}, fmt.Errorf("build set %s instruction: %w", field, err)
	}
	return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{ixn}, input.TokenPool, common_utils.CCTPTokenPool.String())
}

func toRateLimitConfig(direction string, cfg token_deployments.RateLimiterConfig) (cctp_token_pool.RateLimitConfig, error) {
	if cfg.Capacity == nil || cfg.Rate == nil {
		if cfg.IsEnabled {
			return cctp_token_pool.RateLimitConfig{}, fmt.Errorf("%s rate limiter is enabled but capacity or rate is unset", direction)
		}
		return cctp_token_pool.RateLimitConfig{}, nil
	}
	if !cfg.Capacity.IsUint64() || !cfg.Rate.IsUint64() {
		return cctp_token_pool.RateLimitConfig{}, fmt.Errorf("%s rate limiter capacity or rate does not fit in uint64", direction)
	}
	capacity := cfg.Capacity.Uint64()
	rate := cfg.Rate.Uint64()
	if rate > capacity {
		return cctp_token_pool.RateLimitConfig{}, fmt.Errorf("%s rate limiter rate %d exceeds capacity %d", direction, rate, capacity)
	}
	return cctp_token_pool.RateLimitConfig{
		Enabled:  cfg.IsEnabled,
		Capacity: capacity,
		Rate:     rate,
	}, nil
}

func ccvConfigEqual(a, b cctp_token_pool.CcvConfig) bool {
	return a.ThresholdAmount == b.ThresholdAmount &&
		pubkeysEqual(a.OutboundCcvs, b.OutboundCcvs) &&
		pubkeysEqual(a.ThresholdOutboundCcvs, b.ThresholdOutboundCcvs) &&
		pubkeysEqual(a.InboundCcvs, b.InboundCcvs) &&
		pubkeysEqual(a.ThresholdInboundCcvs, b.ThresholdInboundCcvs)
}

func pubkeysEqual(a, b []solana.PublicKey) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
