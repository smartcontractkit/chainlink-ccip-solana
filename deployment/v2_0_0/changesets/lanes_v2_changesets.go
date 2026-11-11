package changesets

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/gagliardetto/solana-go"

	common_utils "github.com/smartcontractkit/chainlink-ccip/deployment/utils"
	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/changesets"
	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/mcms"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	cldf "github.com/smartcontractkit/chainlink-deployments-framework/deployment"
	cldf_ops "github.com/smartcontractkit/chainlink-deployments-framework/operations"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/ccip_router"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/cctp_token_pool"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/tokens"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/allowlist"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/offramp"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/router"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/sequences"
)

// These changesets manage who may send on a Solana chain's 2.0 lanes while they run in parallel
// with 1.6. A staged lane only accepts the test wallet; promoting opens it to everyone, and
// rolling back restricts it to the test wallet again. The Router keeps the list while the lane is
// promoted, so a rollback does not need to restate it.
//
// Each produces Router instructions, batched into an MCMS proposal once the timelock owns the
// Router. Promotion and rollback are meant to be merged with the remote chains' lane migration
// into one proposal, ordered remote chains first.
//
// The allowlist write restates the lane's whole config as read when the proposal is generated,
// but it lands at execution, after the timelock delay: any lane config change in between is
// reverted. Do not run other lane config changes for the same lanes between generating the
// proposal and executing it.

// SetSolanaLanesV2AllowlistChangesetInput applies sender allowlist changes to the given lanes.
type SetSolanaLanesV2AllowlistChangesetInput struct {
	ChainSelector        uint64   `json:"chainSelector"`
	RemoteChainSelectors []uint64 `json:"remoteChainSelectors"`
	// Enabled sets whether each lane's allowlist is enforced; a nil Enabled keeps its setting.
	Enabled *bool `json:"enabled"`
	// AllowedSendersToAdd and AllowedSendersToRemove are base58 addresses applied to each lane's
	// current list.
	AllowedSendersToAdd    []string   `json:"allowedSendersToAdd"`
	AllowedSendersToRemove []string   `json:"allowedSendersToRemove"`
	MCMS                   mcms.Input `json:"mcms"`
}

// SetSolanaLanesV2AllowlistChangeset applies allowlist changes to the Router allowlist of
// existing 2.0 lanes, e.g. adding the test wallet to a lane's list or rotating it.
func SetSolanaLanesV2AllowlistChangeset() cldf.ChangeSetV2[SetSolanaLanesV2AllowlistChangesetInput] {
	return cldf.CreateChangeSet(setSolanaLanesV2AllowlistApply, setSolanaLanesV2AllowlistVerify)
}

// LanesV2ChangesetInput selects the 2.0 lanes to promote or roll back.
type LanesV2ChangesetInput struct {
	ChainSelector        uint64   `json:"chainSelector"`
	RemoteChainSelectors []uint64 `json:"remoteChainSelectors"`
	// CCTPTokenMintsByRemote lists the local CCTP mints used on each remote lane. Omit a remote
	// entry when that lane does not use CCTP. Promotion verifies the pool migration and lane config.
	CCTPTokenMintsByRemote map[uint64][]string `json:"cctpTokenMintsByRemote"`
	MCMS                   mcms.Input          `json:"mcms"`
}

// PromoteSolanaLanesV2Changeset opens the given 2.0 lanes to every sender by disabling their
// Router allowlist. The list itself is kept for a rollback. CCTPTokenMintsByRemote must be
// provided, with an entry for each remote lane using CCTP; promotion checks that each pool
// migration and lane's verifier and pool configuration have landed.
func PromoteSolanaLanesV2Changeset() cldf.ChangeSetV2[LanesV2ChangesetInput] {
	return cldf.CreateChangeSet(lanesV2Apply(false), promoteLanesV2Verify)
}

// RollbackSolanaLanesV2Changeset restricts the given 2.0 lanes to their allowlisted senders again.
func RollbackSolanaLanesV2Changeset() cldf.ChangeSetV2[LanesV2ChangesetInput] {
	return cldf.CreateChangeSet(lanesV2Apply(true), lanesV2Verify)
}

func setSolanaLanesV2AllowlistVerify(env cldf.Environment, input SetSolanaLanesV2AllowlistChangesetInput) error {
	if err := verifyLanesV2(env, input.ChainSelector, input.RemoteChainSelectors, input.MCMS); err != nil {
		return err
	}
	toAdd, err := parseAllowedSenders(input.AllowedSendersToAdd)
	if err != nil {
		return err
	}
	toRemove, err := parseAllowedSenders(input.AllowedSendersToRemove)
	if err != nil {
		return err
	}
	for _, sender := range toRemove {
		if slices.Contains(toAdd, sender) {
			return fmt.Errorf("allowed sender %s is in both the senders to add and the senders to remove", sender)
		}
	}
	return nil
}

func setSolanaLanesV2AllowlistApply(e cldf.Environment, input SetSolanaLanesV2AllowlistChangesetInput) (cldf.ChangesetOutput, error) {
	toAdd, err := parseAllowedSenders(input.AllowedSendersToAdd)
	if err != nil {
		return cldf.ChangesetOutput{}, err
	}
	toRemove, err := parseAllowedSenders(input.AllowedSendersToRemove)
	if err != nil {
		return cldf.ChangesetOutput{}, err
	}
	return runSetLanesV2Allowlist(e, input.MCMS, sequences.SetLanesV2AllowlistInput{
		DataStore:              e.DataStore,
		ChainSelector:          input.ChainSelector,
		RemoteChainSelectors:   input.RemoteChainSelectors,
		Enabled:                input.Enabled,
		AllowedSendersToAdd:    toAdd,
		AllowedSendersToRemove: toRemove,
	})
}

func lanesV2Verify(env cldf.Environment, input LanesV2ChangesetInput) error {
	return verifyLanesV2(env, input.ChainSelector, input.RemoteChainSelectors, input.MCMS)
}

func promoteLanesV2Verify(env cldf.Environment, input LanesV2ChangesetInput) error {
	if err := verifyLanesV2(env, input.ChainSelector, input.RemoteChainSelectors, input.MCMS); err != nil {
		return err
	}
	if input.CCTPTokenMintsByRemote == nil {
		return errors.New("cctpTokenMintsByRemote must be provided; use an empty map when the lanes do not use CCTP")
	}
	selectedRemotes := make(map[uint64]struct{}, len(input.RemoteChainSelectors))
	for _, remote := range input.RemoteChainSelectors {
		selectedRemotes[remote] = struct{}{}
	}
	for remote, mintValues := range input.CCTPTokenMintsByRemote {
		if _, ok := selectedRemotes[remote]; !ok {
			return fmt.Errorf("CCTP readiness entry references unselected remote chain %d", remote)
		}
		seen := make(map[solana.PublicKey]struct{}, len(mintValues))
		for _, value := range mintValues {
			mint, err := solana.PublicKeyFromBase58(value)
			if err != nil || mint.IsZero() {
				return fmt.Errorf("invalid CCTP token mint %q for remote chain %d", value, remote)
			}
			if _, ok := seen[mint]; ok {
				return fmt.Errorf("duplicate CCTP token mint %s for remote chain %d", mint, remote)
			}
			seen[mint] = struct{}{}
			if err = verifyCctpLaneReadiness(env, input.ChainSelector, remote, mint); err != nil {
				return err
			}
		}
	}
	return nil
}

// checkCctpTokenAdminRegistryReady checks that a CCTP mint's registry is in the state
// MigrateCctpTokenPoolToV2 leaves it in: interface version 2 with WithoutExtendedLookupTable
// resolution. Standard would also pass an interface version check, but it makes 2.0 resolution
// append the pool lookup table's additional entries, which on a legacy CCTP table are the
// CCTPv1-specific accounts. The TAR v3 to v4 upgrade preserves the resolution mode, so a registry
// set to Standard (e.g. by the 1.6 outage recovery) stays Standard until it is set explicitly.
func checkCctpTokenAdminRegistryReady(mint solana.PublicKey, tar ccip_router.TokenAdminRegistry) error {
	if tar.InterfaceVersion != 2 {
		return fmt.Errorf("CCTP mint %s is registered at interface version %d; migrate it to version 2 before promotion", mint, tar.InterfaceVersion)
	}
	if resolution := router.TokenAdminRegistryResolutionType(tar); resolution != ccip_router.ResolutionType_WithoutExtendedLookupTable {
		return fmt.Errorf("CCTP mint %s has account resolution %s; set it to %s (re-run the CCTP pool migration) before promotion",
			mint, resolution, ccip_router.ResolutionType_WithoutExtendedLookupTable)
	}
	return nil
}

func verifyCctpLaneReadiness(env cldf.Environment, chainSelector, remote uint64, mint solana.PublicKey) error {
	if env.DataStore == nil {
		return errors.New("datastore is required to verify CCTP readiness")
	}
	chain, ok := env.BlockChains.SolanaChains()[chainSelector]
	if !ok {
		return fmt.Errorf("solana chain with selector %d not found", chainSelector)
	}
	refs := env.DataStore.Addresses().Filter(datastore.AddressRefByChainSelector(chainSelector))
	pool, err := solcommon.PubkeyFromRefs(refs, datastore.NewAddressRefKey(
		chainSelector, datastore.ContractType(common_utils.CCTPTokenPool), solcommon.Version2_0_0, solcommon.PoolProgramQualifier))
	if err != nil {
		return fmt.Errorf("resolve migrated CCTP token pool for mint %s: %w", mint, err)
	}
	routerProgram, err := solcommon.PubkeyFromRefs(refs, datastore.NewAddressRefKey(
		chainSelector, datastore.ContractType(router.ContractType), router.Version, ""))
	if err != nil {
		return fmt.Errorf("resolve Router for CCTP readiness: %w", err)
	}
	offRampProgram, err := solcommon.PubkeyFromRefs(refs, datastore.NewAddressRefKey(
		chainSelector, datastore.ContractType(offramp.ContractType), offramp.Version, ""))
	if err != nil {
		return fmt.Errorf("resolve OffRamp for CCTP readiness: %w", err)
	}
	ctx := context.Background()

	// The registry flip is what allows ccip_send_v2 to resolve this legacy CCTP pool.
	tarPDA, _, err := state.FindTokenAdminRegistryPDA(mint, routerProgram)
	if err != nil {
		return fmt.Errorf("derive token admin registry for mint %s: %w", mint, err)
	}
	tar, _, err := router.ReadTokenAdminRegistry(ctx, chain.Client, tarPDA)
	if err != nil {
		return fmt.Errorf("read token admin registry for CCTP mint %s: %w", mint, err)
	}
	if err = checkCctpTokenAdminRegistryReady(mint, tar); err != nil {
		return err
	}

	configPDA, _, err := state.FindCVConfigPDA(pool)
	if err != nil {
		return fmt.Errorf("derive CCTP verifier config: %w", err)
	}
	config, err := common.GetInitializedAccount(ctx, chain.Client, configPDA, pool, cldf_solana.SolDefaultCommitment, cctp_token_pool.ParseAccount_Config)
	if err != nil {
		return fmt.Errorf("read CCTP verifier config: %w", err)
	}
	if config == nil {
		return fmt.Errorf("CCTP verifier for mint %s is not initialized; migrate the pool before promotion", mint)
	}

	// Migration installs the verifier config in the OffRamp table for inbound execution.
	offRampLUT, err := solcommon.OfframpLookupTable(chain, offRampProgram)
	if err != nil {
		return fmt.Errorf("resolve OffRamp lookup table for CCTP: %w", err)
	}
	offRampEntries, err := common.GetAddressLookupTable(ctx, chain.Client, offRampLUT)
	if err != nil {
		return fmt.Errorf("read OffRamp lookup table for CCTP: %w", err)
	}
	if !slices.Contains(offRampEntries, configPDA) {
		return fmt.Errorf("CCTP verifier config %s is missing from the OffRamp lookup table; migrate the pool before promotion", configPDA)
	}

	verifierConfigPDA, _, err := state.FindCctpVerifierRemoteChainConfigPDA(remote, pool)
	if err != nil {
		return fmt.Errorf("derive CCTP verifier config for remote chain %d: %w", remote, err)
	}
	verifierConfig, err := common.GetInitializedAccount(ctx, chain.Client, verifierConfigPDA, pool, cldf_solana.SolDefaultCommitment, cctp_token_pool.ParseAccount_RemoteChainConfig)
	if err != nil {
		return fmt.Errorf("read CCTP verifier config for remote chain %d: %w", remote, err)
	}
	if verifierConfig == nil || !verifierConfig.Cctp.Enabled || verifierConfig.Cctp.LookupTable == nil || verifierConfig.Cctp.LookupTable.IsZero() {
		return fmt.Errorf("CCTP verifier is not configured for remote chain %d; configure its CCTP lane before promotion", remote)
	}
	if !slices.ContainsFunc(verifierConfig.Cctp.SupportedTokens, func(pair cctp_token_pool.RemoteLocalPair) bool { return pair.Local == mint }) {
		return fmt.Errorf("CCTP mint %s is not enabled for remote chain %d", mint, remote)
	}
	laneEntries, err := common.GetAddressLookupTable(ctx, chain.Client, *verifierConfig.Cctp.LookupTable)
	if err != nil {
		return fmt.Errorf("read CCTP lane lookup table for remote chain %d: %w", remote, err)
	}
	if !slices.Contains(laneEntries, configPDA) || !slices.Contains(laneEntries, verifierConfigPDA) {
		return fmt.Errorf("CCTP lane lookup table is missing verifier accounts for remote chain %d", remote)
	}

	poolConfigPDA, _, err := tokens.TokenPoolChainConfigPDA(remote, mint, pool)
	if err != nil {
		return fmt.Errorf("derive CCTP pool config for remote chain %d: %w", remote, err)
	}
	poolConfig, err := common.GetInitializedAccount(ctx, chain.Client, poolConfigPDA, pool, cldf_solana.SolDefaultCommitment, cctp_token_pool.ParseAccount_ChainConfig)
	if err != nil {
		return fmt.Errorf("read CCTP pool config for remote chain %d: %w", remote, err)
	}
	if poolConfig == nil {
		return fmt.Errorf("CCTP pool config is missing for remote chain %d", remote)
	}
	v2ConfigPDA, _, err := tokens.TokenPoolChainConfigV2PDA(remote, mint, pool)
	if err != nil {
		return fmt.Errorf("derive CCTP v2 pool config for remote chain %d: %w", remote, err)
	}
	v2Config, err := common.GetInitializedAccount(ctx, chain.Client, v2ConfigPDA, pool, cldf_solana.SolDefaultCommitment, cctp_token_pool.ParseAccount_ChainConfigV2)
	if err != nil {
		return fmt.Errorf("read CCTP v2 pool config for remote chain %d: %w", remote, err)
	}
	if v2Config == nil || !slices.Contains(v2Config.CcvConfig.InboundCcvs, pool) || !slices.Contains(v2Config.CcvConfig.OutboundCcvs, pool) {
		return fmt.Errorf("CCTP v2 pool config is incomplete for remote chain %d", remote)
	}
	return nil
}

func lanesV2Apply(enabled bool) func(cldf.Environment, LanesV2ChangesetInput) (cldf.ChangesetOutput, error) {
	return func(e cldf.Environment, input LanesV2ChangesetInput) (cldf.ChangesetOutput, error) {
		return runSetLanesV2Allowlist(e, input.MCMS, sequences.SetLanesV2AllowlistInput{
			DataStore:            e.DataStore,
			ChainSelector:        input.ChainSelector,
			RemoteChainSelectors: input.RemoteChainSelectors,
			Enabled:              &enabled,
		})
	}
}

func runSetLanesV2Allowlist(e cldf.Environment, mcmsInput mcms.Input, seqInput sequences.SetLanesV2AllowlistInput) (cldf.ChangesetOutput, error) {
	report, err := cldf_ops.ExecuteSequence(e.OperationsBundle, sequences.SetLanesV2Allowlist, e.BlockChains, seqInput)
	if err != nil {
		return cldf.ChangesetOutput{}, fmt.Errorf("failed to set 2.0 lanes allowlist: %w", err)
	}
	if err := requireMCMSForBatchOps(e.Logger, mcmsInput, report.Output.BatchOps); err != nil {
		return cldf.ChangesetOutput{}, err
	}
	return changesets.
		NewOutputBuilder(e, changesets.GetRegistry()).
		WithReports(append([]cldf_ops.Report[any, any](nil), report.ExecutionReports...)).
		WithBatchOps(report.Output.BatchOps).
		Build(mcmsInput)
}

func verifyLanesV2(env cldf.Environment, chainSelector uint64, remotes []uint64, mcmsInput mcms.Input) error {
	if err := cldf.IsValidChainSelector(chainSelector); err != nil {
		return fmt.Errorf("invalid chain selector: %d - %w", chainSelector, err)
	}
	if !env.BlockChains.Exists(chainSelector) {
		return fmt.Errorf("chain with selector %d does not exist", chainSelector)
	}
	if _, ok := env.BlockChains.SolanaChains()[chainSelector]; !ok {
		return fmt.Errorf("chain with selector %d is not a Solana chain", chainSelector)
	}
	if len(remotes) == 0 {
		return errors.New("at least one remote chain selector is required")
	}
	seen := make(map[uint64]bool, len(remotes))
	for _, remote := range remotes {
		if err := cldf.IsValidChainSelector(remote); err != nil {
			return fmt.Errorf("invalid remote chain selector: %d - %w", remote, err)
		}
		if remote == chainSelector {
			return fmt.Errorf("remote chain selector %d is the chain itself", remote)
		}
		if seen[remote] {
			return fmt.Errorf("duplicate remote chain selector %d", remote)
		}
		seen[remote] = true
	}
	if err := validateMCMSInput(env.Logger, mcmsInput); err != nil {
		return fmt.Errorf("invalid MCMS configuration: %w", err)
	}
	return nil
}

// parseAllowedSenders parses base58 sender addresses, rejecting duplicates and the zero key,
// which ApplyUpdates only catches at apply time.
func parseAllowedSenders(senders []string) ([]solana.PublicKey, error) {
	keys, err := allowlist.ParseSenders(senders, "allowed")
	if err != nil {
		return nil, err
	}
	seen := make(map[solana.PublicKey]bool, len(keys))
	for _, key := range keys {
		if key.IsZero() {
			return nil, errors.New("the zero key cannot be an allowed sender")
		}
		if seen[key] {
			return nil, fmt.Errorf("duplicate allowed sender %s", key)
		}
		seen[key] = true
	}
	return keys, nil
}
