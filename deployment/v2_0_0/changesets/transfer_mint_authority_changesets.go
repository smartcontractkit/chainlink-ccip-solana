package changesets

import (
	"errors"
	"fmt"

	"github.com/gagliardetto/solana-go"

	common_utils "github.com/smartcontractkit/chainlink-ccip/deployment/utils"
	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/changesets"
	datastore_utils "github.com/smartcontractkit/chainlink-ccip/deployment/utils/datastore"
	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/mcms"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	cldf "github.com/smartcontractkit/chainlink-deployments-framework/deployment"
	cldf_ops "github.com/smartcontractkit/chainlink-deployments-framework/operations"
	mcms_types "github.com/smartcontractkit/mcms/types"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/tokens"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	tokenpoolops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/tokenpools"
)

// TokenMintAuthorityUpdate changes the mint authority of a single token to NewMintAuthority
// (typically a token multisig). The transfer is signed by the BurnMint pool program's upgrade
// authority, the only account allowed to call the on-chain transfer instruction.
type TokenMintAuthorityUpdate struct {
	TokenPoolRef     datastore.AddressRef `json:"tokenPoolRef"`
	TokenMint        solana.PublicKey     `json:"tokenMint"`
	NewMintAuthority solana.PublicKey     `json:"newMintAuthority"`
}

// TransferMintAuthoritiesChangesetInput updates the mint authority for a batch of tokens in a
// single proposal.
type TransferMintAuthoritiesChangesetInput struct {
	ChainSelector uint64                     `json:"chainSelector"`
	Updates       []TokenMintAuthorityUpdate `json:"updates"`
	MCMS          mcms.Input                 `json:"mcms"`
}

// TransferMintAuthoritiesChangeset moves the mint authority of BurnMint pool tokens from the pool
// signer PDA to a new authority. Mints that already have the new authority are skipped.
func TransferMintAuthoritiesChangeset() cldf.ChangeSetV2[TransferMintAuthoritiesChangesetInput] {
	return cldf.CreateChangeSet(transferMintAuthoritiesApply, transferMintAuthoritiesVerify)
}

func transferMintAuthoritiesVerify(env cldf.Environment, input TransferMintAuthoritiesChangesetInput) error {
	if err := cldf.IsValidChainSelector(input.ChainSelector); err != nil {
		return fmt.Errorf("invalid chain selector: %d - %w", input.ChainSelector, err)
	}
	if !env.BlockChains.Exists(input.ChainSelector) {
		return fmt.Errorf("chain with selector %d does not exist", input.ChainSelector)
	}
	if err := validateMintAuthorityUpdates(input.Updates); err != nil {
		return err
	}
	for i, u := range input.Updates {
		fullRef, err := datastore_utils.FindAndFormatRef(env.DataStore, u.TokenPoolRef, input.ChainSelector, datastore_utils.FullRef)
		if err != nil {
			return fmt.Errorf("update[%d]: failed to resolve token pool ref: %w", i, err)
		}
		if fullRef.Type.String() != common_utils.BurnMintTokenPool.String() {
			return fmt.Errorf("update[%d]: mint authority transfer is only supported for BurnMint token pools, but the pool ref has type '%s'", i, fullRef.Type)
		}
	}
	if err := validateMCMSInput(env.Logger, input.MCMS); err != nil {
		return fmt.Errorf("invalid MCMS configuration: %w", err)
	}
	return nil
}

// validateMintAuthorityUpdates checks the updates without touching the datastore or the chain.
func validateMintAuthorityUpdates(updates []TokenMintAuthorityUpdate) error {
	if len(updates) == 0 {
		return errors.New("at least one update is required")
	}
	seen := make(map[solana.PublicKey]struct{}, len(updates))
	for i, u := range updates {
		if datastore_utils.IsAddressRefEmpty(u.TokenPoolRef) {
			return fmt.Errorf("update[%d]: tokenPoolRef must not be empty", i)
		}
		if u.TokenMint.IsZero() {
			return fmt.Errorf("update[%d]: tokenMint must not be zero", i)
		}
		if u.NewMintAuthority.IsZero() {
			return fmt.Errorf("update[%d]: newMintAuthority must not be zero", i)
		}
		if _, ok := seen[u.TokenMint]; ok {
			return fmt.Errorf("update[%d]: duplicate tokenMint %s", i, u.TokenMint)
		}
		seen[u.TokenMint] = struct{}{}
	}
	return nil
}

func transferMintAuthoritiesApply(e cldf.Environment, input TransferMintAuthoritiesChangesetInput) (cldf.ChangesetOutput, error) {
	chain, ok := e.BlockChains.SolanaChains()[input.ChainSelector]
	if !ok {
		return cldf.ChangesetOutput{}, fmt.Errorf("chain with selector %d not found", input.ChainSelector)
	}

	// Check every update before executing any of them, so the deployer-authority path (which
	// confirms each transfer immediately) can't partially apply a batch before a later one fails.
	type pending struct {
		pool         solana.PublicKey
		tokenProgram solana.PublicKey
		update       TokenMintAuthorityUpdate
	}
	todo := make([]pending, 0, len(input.Updates))
	for i, u := range input.Updates {
		pool, err := datastore_utils.FindAndFormatRef(e.DataStore, u.TokenPoolRef, input.ChainSelector, solcommon.ToAddress)
		if err != nil {
			return cldf.ChangesetOutput{}, fmt.Errorf("update[%d]: failed to resolve token pool ref: %w", i, err)
		}
		current, err := solcommon.GetTokenMintAuthority(chain, u.TokenMint)
		if err != nil {
			return cldf.ChangesetOutput{}, fmt.Errorf("update[%d]: %w", i, err)
		}
		if current == u.NewMintAuthority {
			e.Logger.Infof("update[%d]: mint %s already has mint authority %s, skipping", i, u.TokenMint, u.NewMintAuthority)
			continue
		}
		poolSigner, err := tokens.TokenPoolSignerAddress(u.TokenMint, pool)
		if err != nil {
			return cldf.ChangesetOutput{}, fmt.Errorf("update[%d]: failed to derive token pool signer: %w", i, err)
		}
		if current != poolSigner {
			return cldf.ChangesetOutput{}, fmt.Errorf("update[%d]: current mint authority %s is neither the pool signer PDA %s nor the target %s", i, current, poolSigner, u.NewMintAuthority)
		}
		authority, err := solcommon.GetUpgradeAuthority(chain.Client, pool)
		if err != nil {
			return cldf.ChangesetOutput{}, fmt.Errorf("update[%d]: failed to get upgrade authority for token pool: %w", i, err)
		}
		if authority.IsZero() {
			return cldf.ChangesetOutput{}, fmt.Errorf("update[%d]: pool is immutable (no upgrade authority); cannot transfer mint authority", i)
		}
		tokenProgram, err := solcommon.FetchTokenProgramID(e.GetContext(), chain, u.TokenMint)
		if err != nil {
			return cldf.ChangesetOutput{}, fmt.Errorf("update[%d]: %w", i, err)
		}
		todo = append(todo, pending{pool: pool, tokenProgram: tokenProgram, update: u})
	}

	batchOps := make([]mcms_types.BatchOperation, 0, len(todo))
	reports := make([]cldf_ops.Report[any, any], 0, len(todo))
	for _, p := range todo {
		report, err := cldf_ops.ExecuteOperation(e.OperationsBundle, tokenpoolops.TransferMintAuthorityBurnMint, chain, tokenpoolops.Params{
			TokenPool:        p.pool,
			TokenMint:        p.update.TokenMint,
			TokenProgramID:   p.tokenProgram,
			NewMintAuthority: p.update.NewMintAuthority,
		})
		if err != nil {
			return cldf.ChangesetOutput{}, fmt.Errorf("failed to transfer mint authority of %s: %w", p.update.TokenMint, err)
		}
		batchOps = append(batchOps, report.Output.BatchOps...)
		reports = append(reports, report.ToGenericReport())
	}

	if err := requireMCMSForBatchOps(e.Logger, input.MCMS, batchOps); err != nil {
		return cldf.ChangesetOutput{}, err
	}
	return changesets.NewOutputBuilder(e, changesets.GetRegistry()).
		WithReports(reports).
		WithSingleBatchOpPerChain(batchOps).
		Build(input.MCMS)
}
