package router

import (
	"errors"
	"fmt"
	"slices"

	"github.com/gagliardetto/solana-go"

	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/sequences"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/ccip_router"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/allowlist"
)

// DestChainAllowlist changes a v2 lane's sender allowlist: Enabled sets whether the list is
// enforced, a nil Enabled keeping the lane's current setting, and the senders to add and to
// remove apply to the lane's current list. While enabled, ccip_send_v2 rejects any sender not
// in the list with SenderNotAllowed. A staged lane is enabled with the test wallet only;
// promoting it disables the list, and rolling back enables it again.
type DestChainAllowlist struct {
	Enabled                *bool
	AllowedSendersToAdd    []solana.PublicKey
	AllowedSendersToRemove []solana.PublicKey
}

// resolveAllowlist returns the allowlist to write for a lane after applying the requested
// changes to what the lane has on chain; a nil request keeps it as is. update_dest_chain_config_v2
// overwrites the whole config, allowlist included, so an update without changes restates the
// lane's allowlist rather than opening it. A new lane (current nil) without a request starts
// open.
func resolveAllowlist(requested *DestChainAllowlist, current *ccip_router.DestChainConfigCcipV2) (bool, []solana.PublicKey, error) {
	enabled := current != nil && current.AllowListEnabled
	var senders []solana.PublicKey
	if current != nil {
		senders = current.AllowedSenders
	}
	if requested == nil {
		return enabled, senders, nil
	}
	if requested.Enabled != nil {
		enabled = *requested.Enabled
	}
	senders, err := allowlist.ApplyUpdates(senders, requested.AllowedSendersToAdd, requested.AllowedSendersToRemove, enabled)
	if err != nil {
		return false, nil, err
	}
	if enabled && len(senders) == 0 {
		return false, nil, errors.New("an enabled allowlist needs at least one allowed sender")
	}
	return enabled, senders, nil
}

type SetDestChainAllowlistParams struct {
	// LocalChainSelector keeps inputs of chains sharing a Router address apart, since a bundle
	// returns the cached report for the same operation input.
	LocalChainSelector  uint64
	Router              solana.PublicKey
	RemoteChainSelector uint64
	// Allowlist changes applied to the lane's current allowlist; see DestChainAllowlist.
	Enabled                *bool
	AllowedSendersToAdd    []solana.PublicKey
	AllowedSendersToRemove []solana.PublicKey
}

// targetAllowlist returns the allowlist the lane should end up with after applying the changes,
// and whether it differs from what it has. An enabled list must not be empty, since
// ccip_send_v2 would then reject everyone; this also covers a rollback that enables a list kept
// from chain.
func targetAllowlist(input SetDestChainAllowlistParams, current ccip_router.DestChainConfigCcipV2) (bool, []solana.PublicKey, bool, error) {
	requested := &DestChainAllowlist{
		Enabled:                input.Enabled,
		AllowedSendersToAdd:    input.AllowedSendersToAdd,
		AllowedSendersToRemove: input.AllowedSendersToRemove,
	}
	enabled, senders, err := resolveAllowlist(requested, &current)
	if err != nil {
		return false, nil, false, err
	}
	changed := current.AllowListEnabled != enabled || !slices.Equal(current.AllowedSenders, senders)
	return enabled, senders, changed, nil
}

// SetDestChainAllowlist changes the sender allowlist of an existing v2 lane, leaving the rest
// of its config as it is on chain. It skips lanes already in the requested state, so it can be
// re-run.
//
// update_dest_chain_config_v2 rewrites the lane's whole config, so the instruction restates what
// the operation read. The read happens when the MCMS proposal is generated but the write lands
// at execution, after the timelock delay: a lane config change in between is reverted. Do not
// run other config operations for the same lane between generating and executing the proposal.
var SetDestChainAllowlist = operations.NewOperation(
	"router:set-dest-chain-allowlist",
	Version,
	"Sets the sender allowlist of a v2 lane on the Router 2.0.0-dev contract",
	func(b operations.Bundle, chain cldf_solana.Chain, input SetDestChainAllowlistParams) (sequences.OnChainOutput, error) {
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

		targetEnabled, targetSenders, changed, err := targetAllowlist(input, destChainAccount.Config)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("lane to chain %d: %w", input.RemoteChainSelector, err)
		}
		if !changed {
			b.Logger.Infof("Router v2 lane to chain %d already has allowlist enabled=%t with %d senders, skipping",
				input.RemoteChainSelector, targetEnabled, len(targetSenders))
			return sequences.OnChainOutput{}, nil
		}

		config := destChainAccount.Config
		config.AllowListEnabled = targetEnabled
		config.AllowedSenders = targetSenders

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
