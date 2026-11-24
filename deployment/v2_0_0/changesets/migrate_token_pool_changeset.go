package changesets

import (
	"context"
	"errors"
	"fmt"

	"github.com/gagliardetto/solana-go"

	"github.com/smartcontractkit/chainlink-ccip/deployment/finality"
	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/changesets"
	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/mcms"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	cldf "github.com/smartcontractkit/chainlink-deployments-framework/deployment"
	cldf_ops "github.com/smartcontractkit/chainlink-deployments-framework/operations"

	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/sequences"
)

// MigrateTokenPoolToV2ChangesetInput selects the token pool instance to migrate from 1.6 to 2.0.
type MigrateTokenPoolToV2ChangesetInput struct {
	ChainSelector uint64 `json:"chainSelector"`
	// PoolType is BurnMintTokenPool or LockReleaseTokenPool.
	PoolType string `json:"poolType"`
	// TokenMint is the base58 mint whose pool instance migrates.
	TokenMint string `json:"tokenMint"`
	// PoolProgramQualifier selects the pool program: empty or CLLCCIP is the shared program, any
	// other value is the program registered under that qualifier.
	PoolProgramQualifier string `json:"poolProgramQualifier,omitempty"`
	// TokenPoolQualifier is the qualifier the migrated instance is registered under. Empty derives it
	// from the mint's token ref.
	TokenPoolQualifier string `json:"tokenPoolQualifier,omitempty"`
	// RemoteChainSelectors are the 2.0 lanes the pool is migrated for.
	RemoteChainSelectors []uint64 `json:"remoteChainSelectors"`
	// AllowedFinalityConfig, when set, is the allowed finality written for every remote above.
	AllowedFinalityConfig *finality.Config `json:"allowedFinalityConfig,omitempty"`
	MCMS                  mcms.Input       `json:"mcms"`
}

// MigrateTokenPoolToV2Changeset makes an existing 1.6 BurnMint or LockRelease token pool instance
// usable by 2.0 lanes. See sequences.MigrateTokenPoolToV2 for the steps.
//
// Prerequisites, checked in verify before anything is sent:
//  1. The pool program is upgraded in place to 2.0 with UpgradeProgramChangeset (programType
//     BurnMintTokenPool or LockReleaseTokenPool), which registers a 2.x ref at its address.
//  2. Every remote in RemoteChainSelectors is a 2.0 lane on the Router and the 2.0 OffRamp, and the
//     pool instance is already configured for it.
//  3. The deployer or the timelock of MCMS.Qualifier controls the pool program, the pool and the
//     Token Admin Registry entry.
//
// Outputs: the pool signer and fee vault token accounts, sent directly by the deployer and listed in
// the EnsurePoolTokenAccounts report; the ChainConfigV2 of each remote and the Token Admin Registry
// flip, sent directly or as an MCMS proposal depending on who controls them; and the instance's 2.x
// ref under the token qualifier, unless the datastore already has it.
//
// Rollback: SetPoolInterfaceImplementationChangeset with interfaceVersion 1 and NoResolution routes
// the token back to the 1.6 interface. The token accounts and ChainConfigV2 are unused on 1.6 and can
// stay. The 2.x instance ref is removed from the datastore with a RemoteDelete of its key.
//
// CCTP pools migrate with MigrateCctpTokenPoolToV2Changeset. The 1.6 lanes keep serving the token
// throughout, as the 1.6 ccip_send path never reads the registry's interface version.
func MigrateTokenPoolToV2Changeset() cldf.ChangeSetV2[MigrateTokenPoolToV2ChangesetInput] {
	return cldf.CreateChangeSet(migrateTokenPoolToV2Apply, migrateTokenPoolToV2Verify)
}

func migrateTokenPoolToV2Verify(env cldf.Environment, input MigrateTokenPoolToV2ChangesetInput) error {
	mint, err := validateMigrateTokenPoolInput(env, input)
	if err != nil {
		return err
	}
	if env.DataStore == nil {
		return errors.New("datastore is required to migrate a token pool")
	}
	target, err := sequences.ResolveTokenPoolMigration(env.DataStore, input.ChainSelector, input.PoolType, input.PoolProgramQualifier, mint, input.TokenPoolQualifier)
	if err != nil {
		return err
	}
	chain := env.BlockChains.SolanaChains()[input.ChainSelector]
	refs := env.DataStore.Addresses().Filter(datastore.AddressRefByChainSelector(input.ChainSelector))
	checks, err := sequences.NewTokenPoolMigrationChecks(refs, chain, input.ChainSelector, input.PoolType, input.MCMS.Qualifier)
	if err != nil {
		return err
	}
	ctx := env.GetContext()
	if ctx == nil {
		ctx = context.Background()
	}
	return checks.Run(ctx, chain, target.Program, mint, input.RemoteChainSelectors)
}

// validateMigrateTokenPoolInput checks the input without reading the datastore or the chain.
func validateMigrateTokenPoolInput(env cldf.Environment, input MigrateTokenPoolToV2ChangesetInput) (solana.PublicKey, error) {
	if err := cldf.IsValidChainSelector(input.ChainSelector); err != nil {
		return solana.PublicKey{}, fmt.Errorf("invalid chain selector %d: %w", input.ChainSelector, err)
	}
	if !env.BlockChains.Exists(input.ChainSelector) {
		return solana.PublicKey{}, fmt.Errorf("chain with selector %d does not exist", input.ChainSelector)
	}
	if _, ok := env.BlockChains.SolanaChains()[input.ChainSelector]; !ok {
		return solana.PublicKey{}, fmt.Errorf("chain with selector %d is not a Solana chain", input.ChainSelector)
	}
	if err := sequences.ValidateMigratablePoolType(input.PoolType); err != nil {
		return solana.PublicKey{}, err
	}
	mint, err := solana.PublicKeyFromBase58(input.TokenMint)
	if err != nil || mint.IsZero() {
		return solana.PublicKey{}, fmt.Errorf("invalid token mint %q", input.TokenMint)
	}
	if len(input.RemoteChainSelectors) == 0 {
		return solana.PublicKey{}, errors.New("at least one remote chain selector is required")
	}
	seen := make(map[uint64]struct{}, len(input.RemoteChainSelectors))
	for _, remote := range input.RemoteChainSelectors {
		if remote == input.ChainSelector {
			return solana.PublicKey{}, fmt.Errorf("remote chain selector %d is the chain itself", remote)
		}
		if err = cldf.IsValidChainSelector(remote); err != nil {
			return solana.PublicKey{}, fmt.Errorf("invalid remote chain selector %d: %w", remote, err)
		}
		if _, dup := seen[remote]; dup {
			return solana.PublicKey{}, fmt.Errorf("duplicate remote chain selector %d", remote)
		}
		seen[remote] = struct{}{}
	}
	if err = validateMCMSInput(env.Logger, input.MCMS); err != nil {
		return solana.PublicKey{}, fmt.Errorf("failed to validate MCMS input: %w", err)
	}
	return mint, nil
}

func migrateTokenPoolToV2Apply(e cldf.Environment, input MigrateTokenPoolToV2ChangesetInput) (cldf.ChangesetOutput, error) {
	mint, err := solana.PublicKeyFromBase58(input.TokenMint)
	if err != nil {
		return cldf.ChangesetOutput{}, fmt.Errorf("invalid token mint %q: %w", input.TokenMint, err)
	}
	report, err := cldf_ops.ExecuteSequence(e.OperationsBundle, sequences.MigrateTokenPoolToV2, e.BlockChains, sequences.MigrateTokenPoolToV2Input{
		DataStore:             e.DataStore,
		ChainSelector:         input.ChainSelector,
		PoolType:              input.PoolType,
		TokenMint:             mint,
		PoolProgramQualifier:  input.PoolProgramQualifier,
		TokenPoolQualifier:    input.TokenPoolQualifier,
		MCMSQualifier:         input.MCMS.Qualifier,
		RemoteChainSelectors:  input.RemoteChainSelectors,
		AllowedFinalityConfig: input.AllowedFinalityConfig,
	})
	if err != nil {
		return cldf.ChangesetOutput{}, fmt.Errorf("migrate token pool to 2.0: %w", err)
	}
	ds := datastore.NewMemoryDataStore()
	for _, addr := range report.Output.Addresses {
		if err := ds.Addresses().Add(addr); err != nil {
			return cldf.ChangesetOutput{}, fmt.Errorf("add address to datastore: %w", err)
		}
	}
	if err := requireMCMSForBatchOps(e.Logger, input.MCMS, report.Output.BatchOps); err != nil {
		return cldf.ChangesetOutput{}, err
	}
	return changesets.
		NewOutputBuilder(e, changesets.GetRegistry()).
		WithReports(append([]cldf_ops.Report[any, any](nil), report.ExecutionReports...)).
		WithDataStore(ds).
		WithBatchOps(report.Output.BatchOps).
		Build(input.MCMS)
}
