package feequoter

import (
	"fmt"

	"github.com/gagliardetto/solana-go"

	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/sequences"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/fee_quoter"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
)

// DestChainConfigUpdate carries the dynamic fields of an existing v2 lane's FeeQuoter config that
// the operation may change. A nil field keeps the lane's current value; a non-nil field replaces
// it. LaneCodeVersion and ChainFamilySelector are fixed by the lane and are not inputs.
type DestChainConfigUpdate struct {
	IsEnabled                   *bool   `json:"isEnabled"`
	MaxDataBytes                *uint32 `json:"maxDataBytes"`
	MaxPerMsgGasLimit           *uint32 `json:"maxPerMsgGasLimit"`
	DestGasOverhead             *uint32 `json:"destGasOverhead"`
	DestGasPerPayloadByteBase   *uint32 `json:"destGasPerPayloadByteBase"`
	DefaultTokenFeeUsdCents     *uint16 `json:"defaultTokenFeeUsdCents"`
	DefaultTokenDestGasOverhead *uint32 `json:"defaultTokenDestGasOverhead"`
	DefaultTxGasLimit           *uint32 `json:"defaultTxGasLimit"`
	NetworkFeeUSDCents          *uint16 `json:"networkFeeUSDCents"`
	LinkFeeMultiplierPercent    *uint8  `json:"linkFeeMultiplierPercent"`
}

// IsEmpty reports whether the update requests no field.
func (u DestChainConfigUpdate) IsEmpty() bool {
	return u.IsEnabled == nil &&
		u.MaxDataBytes == nil &&
		u.MaxPerMsgGasLimit == nil &&
		u.DestGasOverhead == nil &&
		u.DestGasPerPayloadByteBase == nil &&
		u.DefaultTokenFeeUsdCents == nil &&
		u.DefaultTokenDestGasOverhead == nil &&
		u.DefaultTxGasLimit == nil &&
		u.NetworkFeeUSDCents == nil &&
		u.LinkFeeMultiplierPercent == nil
}

// SetDestChainConfigParams carries the fields of an existing v2 lane's FeeQuoter config that the
// operation may change. A nil field keeps the lane's current value; a non-nil field replaces it.
type SetDestChainConfigParams struct {
	// LocalChainSelector keeps inputs of chains sharing a FeeQuoter address apart, since a bundle
	// returns the cached report for the same operation input.
	LocalChainSelector  uint64
	FeeQuoter           solana.PublicKey
	RemoteChainSelector uint64

	DestChainConfigUpdate
}

// targetDestChainConfig returns the lane's config after applying the requested changes to what
// the lane has on chain, and whether anything differs. Only the requested fields can differ, so
// the comparison covers exactly those.
func targetDestChainConfig(update DestChainConfigUpdate, current fee_quoter.DestChainConfigV2) (fee_quoter.DestChainConfigV2, bool) {
	config := current
	changed := false
	if update.IsEnabled != nil && config.IsEnabled != *update.IsEnabled {
		config.IsEnabled = *update.IsEnabled
		changed = true
	}
	if update.MaxDataBytes != nil && config.MaxDataBytes != *update.MaxDataBytes {
		config.MaxDataBytes = *update.MaxDataBytes
		changed = true
	}
	if update.MaxPerMsgGasLimit != nil && config.MaxPerMsgGasLimit.V0 != *update.MaxPerMsgGasLimit {
		config.MaxPerMsgGasLimit = fee_quoter.CrossChainGas{V0: *update.MaxPerMsgGasLimit}
		changed = true
	}
	if update.DestGasOverhead != nil && config.DestGasOverhead.V0 != *update.DestGasOverhead {
		config.DestGasOverhead = fee_quoter.CrossChainGas{V0: *update.DestGasOverhead}
		changed = true
	}
	if update.DestGasPerPayloadByteBase != nil && config.DestGasPerPayloadByteBase != *update.DestGasPerPayloadByteBase {
		config.DestGasPerPayloadByteBase = *update.DestGasPerPayloadByteBase
		changed = true
	}
	if update.DefaultTokenFeeUsdCents != nil && config.DefaultTokenFeeUsdCents != *update.DefaultTokenFeeUsdCents {
		config.DefaultTokenFeeUsdCents = *update.DefaultTokenFeeUsdCents
		changed = true
	}
	if update.DefaultTokenDestGasOverhead != nil && config.DefaultTokenDestGasOverhead.V0 != *update.DefaultTokenDestGasOverhead {
		config.DefaultTokenDestGasOverhead = fee_quoter.CrossChainGas{V0: *update.DefaultTokenDestGasOverhead}
		changed = true
	}
	if update.DefaultTxGasLimit != nil && config.DefaultTxGasLimit.V0 != *update.DefaultTxGasLimit {
		config.DefaultTxGasLimit = fee_quoter.CrossChainGas{V0: *update.DefaultTxGasLimit}
		changed = true
	}
	if update.NetworkFeeUSDCents != nil && config.NetworkFee.V0 != uint32(*update.NetworkFeeUSDCents) {
		config.NetworkFee = fee_quoter.UsdCents{V0: uint32(*update.NetworkFeeUSDCents)}
		changed = true
	}
	if update.LinkFeeMultiplierPercent != nil && config.LinkFeeMultiplierPercent != *update.LinkFeeMultiplierPercent {
		config.LinkFeeMultiplierPercent = *update.LinkFeeMultiplierPercent
		changed = true
	}
	return config, changed
}

// SetDestChainConfig changes selected dynamic fields of an existing v2 lane's FeeQuoter config,
// leaving the rest of its config as it is on chain. It skips lanes already in the requested
// state, so it can be re-run.
//
// upsert_dest_chain_config_v2 rewrites the lane's whole config, so the instruction restates what
// the operation read. The read happens when the MCMS proposal is generated but the write lands
// at execution, after the timelock delay: a lane config change in between is reverted. Do not
// run other config operations for the same lane between generating and executing the proposal.
var SetDestChainConfig = operations.NewOperation(
	"fee-quoter:set-dest-chain-config",
	Version,
	"Sets selected dynamic fields of a v2 lane's config on the FeeQuoter contract",
	func(b operations.Bundle, chain cldf_solana.Chain, input SetDestChainConfigParams) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&fee_quoter.ProgramID, fee_quoter.ParseInstructionWithoutAccounts, input.FeeQuoter)
		fqDestChainPDA, _, err := state.FindFqDestChainV2PDA(input.RemoteChainSelector, input.FeeQuoter)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive fee quoter dest chain PDA: %w", err)
		}
		destChainAccount, err := common.GetInitializedAccount(b.GetContext(), chain.Client, fqDestChainPDA, input.FeeQuoter, cldf_solana.SolDefaultCommitment, fee_quoter.ParseAccount_DestChainV2)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("read v2 lane to chain %d: %w", input.RemoteChainSelector, err)
		}
		if destChainAccount == nil {
			return sequences.OnChainOutput{}, fmt.Errorf("v2 lane to chain %d is not configured", input.RemoteChainSelector)
		}

		config, changed := targetDestChainConfig(input.DestChainConfigUpdate, destChainAccount.Config)
		if !changed {
			b.Logger.Infof("FeeQuoter v2 lane to chain %d already has the requested config, skipping", input.RemoteChainSelector)
			return sequences.OnChainOutput{}, nil
		}

		feeQuoterConfigPDA, _, err := state.FindFqConfigPDA(input.FeeQuoter)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive fee quoter config PDA: %w", err)
		}
		eventAuthority, _, err := state.FindEventAuthorityPDA(input.FeeQuoter)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive fee quoter event authority PDA: %w", err)
		}
		authority, err := GetAuthority(b.GetContext(), chain, input.FeeQuoter)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		ixn, err := fee_quoter.NewUpsertDestChainConfigV2Instruction(
			fee_quoter.UpsertDestChainConfigV2Params{
				ChainSelector:   input.RemoteChainSelector,
				DestChainConfig: config,
			},
			feeQuoterConfigPDA,
			fqDestChainPDA,
			authority,
			solana.SystemProgramID,
			eventAuthority,
			input.FeeQuoter,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to build update dest chain config instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{ixn}, input.FeeQuoter, ContractType.String())
	},
)
