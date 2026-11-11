package changesets

import (
	"fmt"

	"github.com/gagliardetto/solana-go"

	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/changesets"
	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/mcms"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	cldf "github.com/smartcontractkit/chainlink-deployments-framework/deployment"
	cldf_ops "github.com/smartcontractkit/chainlink-deployments-framework/operations"

	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/sequences"
)

// MigrateCctpTokenPoolToV2ChangesetInput selects the chain and USDC mint whose 1.6 CCTP pool
// migrates to 2.0.
type MigrateCctpTokenPoolToV2ChangesetInput struct {
	ChainSelector uint64 `json:"chainSelector"`
	// USDCToken is the base58 mint the CCTP pool manages.
	USDCToken string     `json:"usdcToken"`
	MCMS      mcms.Input `json:"mcms"`
}

// MigrateCctpTokenPoolToV2Changeset makes an existing 1.6 CCTP token pool usable by 2.0 lanes:
// it initializes the pool's embedded verifier, funds the pool signer, flips the Token Admin
// Registry to interface version 2 with WithoutExtendedLookupTable resolution (account resolution
// stays on; only the lookup table's additional entries are skipped), and registers the pool
// program's 2.0.0 ref at its existing address. See sequences.MigrateCctpTokenPoolToV2 for the steps.
//
// Prerequisites, in order:
//  1. Upgrade Router, FeeQuoter and RMNRemote to 2.0 with UpgradeProgramChangeset. Each successful
//     upgrade returns a 2.0.0 address ref at the existing program address.
//  2. Upgrade the CCTP pool in place with UpgradeProgramChangeset and programType CCTPTokenPool;
//     initialize_verifier exists only in the 2.0 binary. The upgrade also returns its 2.0.0 ref.
//  3. Run this changeset, then ConfigureCCTPChainForLanes for the affected remotes. Promote the
//     2.0 lanes only after those steps; promotion checks the supplied CCTP mints on chain.
//
// The 1.6 lanes keep serving this token throughout: the legacy ccip_send path never reads the
// registry's interface version, so the flip does not disturb them (TAR v4 keeps the resolution
// byte the 1.6 OffRamp decodes at 0 or 1), and the pool picks the CCTP
// version from the lane version of each message. The changeset therefore does not need to share
// an MCMS batch with the lane promotion. It is idempotent and can be re-run.
func MigrateCctpTokenPoolToV2Changeset() cldf.ChangeSetV2[MigrateCctpTokenPoolToV2ChangesetInput] {
	return cldf.CreateChangeSet(migrateCctpTokenPoolToV2Apply, migrateCctpTokenPoolToV2Verify)
}

func migrateCctpTokenPoolToV2Verify(env cldf.Environment, input MigrateCctpTokenPoolToV2ChangesetInput) error {
	if err := cldf.IsValidChainSelector(input.ChainSelector); err != nil {
		return fmt.Errorf("invalid chain selector %d: %w", input.ChainSelector, err)
	}
	if !env.BlockChains.Exists(input.ChainSelector) {
		return fmt.Errorf("chain with selector %d does not exist", input.ChainSelector)
	}
	if _, ok := env.BlockChains.SolanaChains()[input.ChainSelector]; !ok {
		return fmt.Errorf("chain with selector %d is not a Solana chain", input.ChainSelector)
	}
	if _, err := solana.PublicKeyFromBase58(input.USDCToken); err != nil {
		return fmt.Errorf("invalid USDC token %q: %w", input.USDCToken, err)
	}
	if err := validateMCMSInput(env.Logger, input.MCMS); err != nil {
		return fmt.Errorf("failed to validate MCMS input: %w", err)
	}
	if _, _, err := sequences.ResolveCctpPoolForMigration(env.DataStore, input.ChainSelector); err != nil {
		return err
	}
	return nil
}

func migrateCctpTokenPoolToV2Apply(e cldf.Environment, input MigrateCctpTokenPoolToV2ChangesetInput) (cldf.ChangesetOutput, error) {
	mint, err := solana.PublicKeyFromBase58(input.USDCToken)
	if err != nil {
		return cldf.ChangesetOutput{}, fmt.Errorf("invalid USDC token %q: %w", input.USDCToken, err)
	}
	report, err := cldf_ops.ExecuteSequence(e.OperationsBundle, sequences.MigrateCctpTokenPoolToV2, e.BlockChains, sequences.MigrateCctpTokenPoolToV2Input{
		DataStore:     e.DataStore,
		ChainSelector: input.ChainSelector,
		USDCToken:     mint,
	})
	if err != nil {
		return cldf.ChangesetOutput{}, fmt.Errorf("migrate CCTP token pool to 2.0: %w", err)
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
