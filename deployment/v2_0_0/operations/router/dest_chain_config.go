package router

import (
	"fmt"

	"github.com/gagliardetto/solana-go"

	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/sequences"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/ccip_router"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
)

// SetDestChainConfigParams carries the fields of an existing v2 lane's Router config that the
// operation may change. A nil field keeps the lane's current value; a non-nil field replaces it.
type SetDestChainConfigParams struct {
	// LocalChainSelector keeps inputs of chains sharing a Router address apart, since a bundle
	// returns the cached report for the same operation input.
	LocalChainSelector  uint64
	Router              solana.PublicKey
	RemoteChainSelector uint64

	BaseExecutionGasCost            *uint32
	MessageNetworkFeeUSDCents       *uint16
	TokenTransferNetworkFeeUSDCents *uint16
	MaxFeePerMessageUSDCents        *uint16
	TokenReceiverAllowed            *bool
}

// targetDestChainConfig returns the lane's config after applying the requested changes to what
// the lane has on chain, and whether anything differs. Only the requested fields can differ, so
// the comparison covers exactly those.
func targetDestChainConfig(input SetDestChainConfigParams, current ccip_router.DestChainConfigCcipV2) (ccip_router.DestChainConfigCcipV2, bool) {
	config := current
	changed := false
	if input.BaseExecutionGasCost != nil {
		if v := (ccip_router.CrossChainGas{V0: *input.BaseExecutionGasCost}); config.BaseExecutionGasCost != v {
			config.BaseExecutionGasCost = v
			changed = true
		}
	}
	if input.MessageNetworkFeeUSDCents != nil {
		if v := (ccip_router.UsdCents{V0: uint32(*input.MessageNetworkFeeUSDCents)}); config.MessageNetworkFee != v {
			config.MessageNetworkFee = v
			changed = true
		}
	}
	if input.TokenTransferNetworkFeeUSDCents != nil {
		if v := (ccip_router.UsdCents{V0: uint32(*input.TokenTransferNetworkFeeUSDCents)}); config.TokenTransferNetworkFee != v {
			config.TokenTransferNetworkFee = v
			changed = true
		}
	}
	if input.MaxFeePerMessageUSDCents != nil {
		if v := (ccip_router.UsdCents{V0: uint32(*input.MaxFeePerMessageUSDCents)}); config.MaxFeePerMessage != v {
			config.MaxFeePerMessage = v
			changed = true
		}
	}
	if input.TokenReceiverAllowed != nil && config.TokenReceiverAllowed != *input.TokenReceiverAllowed {
		config.TokenReceiverAllowed = *input.TokenReceiverAllowed
		changed = true
	}
	return config, changed
}

// SetDestChainConfig changes selected fields of an existing v2 lane's Router config, leaving the
// rest of its config as it is on chain. It skips lanes already in the requested state, so it can
// be re-run.
//
// update_dest_chain_config_v2 rewrites the lane's whole config, so the instruction restates what
// the operation read. The read happens when the MCMS proposal is generated but the write lands
// at execution, after the timelock delay: a lane config change in between is reverted. Do not
// run other config operations for the same lane between generating and executing the proposal.
var SetDestChainConfig = operations.NewOperation(
	"router:set-dest-chain-config",
	Version,
	"Sets selected fields of a v2 lane's config on the Router contract",
	func(b operations.Bundle, chain cldf_solana.Chain, input SetDestChainConfigParams) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&ccip_router.ProgramID, ccip_router.ParseInstructionWithoutAccounts, input.Router)
		routerDestChainPDA, err := state.FindDestChainStatePDAV2(input.RemoteChainSelector, input.Router)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive dest chain state PDA: %w", err)
		}
		destChainAccount, err := common.GetInitializedAccount(b.GetContext(), chain.Client, routerDestChainPDA, input.Router, cldf_solana.SolDefaultCommitment, ccip_router.ParseAccount_DestChainCcipV2)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("read v2 lane to chain %d: %w", input.RemoteChainSelector, err)
		}
		if destChainAccount == nil {
			return sequences.OnChainOutput{}, fmt.Errorf("v2 lane to chain %d is not configured", input.RemoteChainSelector)
		}

		config, changed := targetDestChainConfig(input, destChainAccount.Config)
		if !changed {
			b.Logger.Infof("Router v2 lane to chain %d already has the requested config, skipping", input.RemoteChainSelector)
			return sequences.OnChainOutput{}, nil
		}

		routerConfigPDA, _, err := state.FindConfigPDA(input.Router)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive router config PDA: %w", err)
		}
		routerEventAuthorityPDA, _, err := state.FindEventAuthorityPDA(input.Router)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive router event authority PDA: %w", err)
		}
		authority, err := GetAuthority(b.GetContext(), chain, input.Router)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		ixn, err := ccip_router.NewUpdateDestChainConfigV2Instruction(
			ccip_router.UpdateChainSelectorV2Params{
				DestChainSelector: input.RemoteChainSelector,
				DestChainConfig:   config,
			},
			routerDestChainPDA,
			routerConfigPDA,
			authority,
			solana.SystemProgramID,
			routerEventAuthorityPDA,
			input.Router,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to build update dest chain instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{ixn}, input.Router, ContractType.String())
	},
)
