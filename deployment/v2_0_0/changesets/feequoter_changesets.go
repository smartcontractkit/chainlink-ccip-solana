package changesets

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"slices"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/changesets"
	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/mcms"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	cldf "github.com/smartcontractkit/chainlink-deployments-framework/deployment"
	cldf_ops "github.com/smartcontractkit/chainlink-deployments-framework/operations"
	"github.com/smartcontractkit/mcms/types"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/fee_quoter"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	fqops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/feequoter"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/sequences"
)

type AddLegacyDestChainChangesetInput struct {
	ChainSelector        uint64     `json:"chainSelector"`
	RemoteChainSelectors []uint64   `json:"remoteChainSelectors"`
	MCMS                 mcms.Input `json:"mcms"`
}

func AddLegacyDestChainChangeset() cldf.ChangeSetV2[AddLegacyDestChainChangesetInput] {
	return cldf.CreateChangeSet(addLegacyDestChainApply, addLegacyDestChainVerify)
}

func addLegacyDestChainVerify(env cldf.Environment, input AddLegacyDestChainChangesetInput) error {
	if err := cldf.IsValidChainSelector(input.ChainSelector); err != nil {
		return fmt.Errorf("invalid chain selector %d: %w", input.ChainSelector, err)
	}
	if !env.BlockChains.Exists(input.ChainSelector) {
		return fmt.Errorf("chain with selector %d does not exist", input.ChainSelector)
	}
	if _, ok := env.BlockChains.SolanaChains()[input.ChainSelector]; !ok {
		return fmt.Errorf("chain with selector %d is not a Solana chain", input.ChainSelector)
	}
	if len(input.RemoteChainSelectors) == 0 {
		return errors.New("at least one remote chain selector is required")
	}
	if err := validateMCMSInput(env.Logger, input.MCMS); err != nil {
		return fmt.Errorf("invalid MCMS configuration: %w", err)
	}
	return nil
}

func addLegacyDestChainApply(e cldf.Environment, input AddLegacyDestChainChangesetInput) (cldf.ChangesetOutput, error) {
	chain := e.BlockChains.SolanaChains()[input.ChainSelector]

	fqAddrBytes, err := (&sequences.SolanaAdapter{}).GetFQAddress(e.DataStore, input.ChainSelector)
	if err != nil {
		return cldf.ChangesetOutput{}, fmt.Errorf("resolve fee quoter from datastore: %w", err)
	}
	feeQuoter := solana.PublicKeyFromBytes(fqAddrBytes)

	var reports []cldf_ops.Report[any, any]
	var batchOps []types.BatchOperation
	for _, remoteSelector := range input.RemoteChainSelectors {
		v1Config, err := readV2AndProjectToV1(chain, feeQuoter, remoteSelector)
		if err != nil {
			return cldf.ChangesetOutput{}, fmt.Errorf("read v2 config for remote chain %d: %w", remoteSelector, err)
		}

		report, err := cldf_ops.ExecuteOperation(e.OperationsBundle, fqops.AddLegacyDestChain, chain, fqops.AddLegacyDestChainParams{
			FeeQuoter:           feeQuoter,
			RemoteChainSelector: remoteSelector,
			DestChainConfig:     v1Config,
		})
		if err != nil {
			return cldf.ChangesetOutput{}, fmt.Errorf("add legacy dest chain for remote %d: %w", remoteSelector, err)
		}
		reports = append(reports, report.ToGenericReport())
		// Forward the batch: with a governed (non-deployer) owner the op does
		// not send, it returns the instruction for MCMS assembly.
		batchOps = append(batchOps, report.Output.BatchOps...)
	}

	if err := requireMCMSForBatchOps(e.Logger, input.MCMS, batchOps); err != nil {
		return cldf.ChangesetOutput{}, err
	}
	return changesets.
		NewOutputBuilder(e, changesets.GetRegistry()).
		WithReports(reports).
		WithBatchOps(batchOps).
		Build(input.MCMS)
}

func readV2AndProjectToV1(chain cldf_solana.Chain, feeQuoter solana.PublicKey, remoteChainSelector uint64) (fee_quoter.DestChainConfig, error) {
	destChainV2PDA, _, err := state.FindFqDestChainV2PDA(remoteChainSelector, feeQuoter)
	if err != nil {
		return fee_quoter.DestChainConfig{}, fmt.Errorf("derive v2 dest chain PDA: %w", err)
	}

	var destChainV2 fee_quoter.DestChainV2
	if err := common.GetAccountDataBorshIntoAnchor(
		context.Background(), chain.Client, destChainV2PDA, cldf_solana.SolDefaultCommitment, &destChainV2,
	); err != nil {
		return fee_quoter.DestChainConfig{}, fmt.Errorf("read v2 dest chain account: %w", err)
	}

	return fqops.V1DestChainConfigFromV2(destChainV2.Config), nil
}

// UpdatePrices changeset

type GasPriceUpdateEntry struct {
	DestChainSelector uint64 `json:"destChainSelector"`
	UsdPerUnitGas     string `json:"usdPerUnitGas"`
}

type UpdatePricesChangesetInput struct {
	ChainSelector   uint64                `json:"chainSelector"`
	GasPriceUpdates []GasPriceUpdateEntry `json:"gasPriceUpdates"`
	MCMS            mcms.Input            `json:"mcms"`
}

func UpdatePricesChangeset() cldf.ChangeSetV2[UpdatePricesChangesetInput] {
	return cldf.CreateChangeSet(updatePricesApply, updatePricesVerify)
}

func updatePricesVerify(env cldf.Environment, input UpdatePricesChangesetInput) error {
	if err := cldf.IsValidChainSelector(input.ChainSelector); err != nil {
		return fmt.Errorf("invalid chain selector %d: %w", input.ChainSelector, err)
	}
	if !env.BlockChains.Exists(input.ChainSelector) {
		return fmt.Errorf("chain with selector %d does not exist", input.ChainSelector)
	}
	if _, ok := env.BlockChains.SolanaChains()[input.ChainSelector]; !ok {
		return fmt.Errorf("chain with selector %d is not a Solana chain", input.ChainSelector)
	}
	if len(input.GasPriceUpdates) == 0 {
		return errors.New("at least one gas price update is required")
	}
	for i, gu := range input.GasPriceUpdates {
		if _, ok := new(big.Int).SetString(gu.UsdPerUnitGas, 10); !ok {
			return fmt.Errorf("gasPriceUpdates[%d]: invalid usdPerUnitGas %q", i, gu.UsdPerUnitGas)
		}
	}
	if err := validateMCMSInput(env.Logger, input.MCMS); err != nil {
		return fmt.Errorf("invalid MCMS configuration: %w", err)
	}
	return nil
}

func updatePricesApply(e cldf.Environment, input UpdatePricesChangesetInput) (cldf.ChangesetOutput, error) {
	chain := e.BlockChains.SolanaChains()[input.ChainSelector]

	fqAddrBytes, err := (&sequences.SolanaAdapter{}).GetFQAddress(e.DataStore, input.ChainSelector)
	if err != nil {
		return cldf.ChangesetOutput{}, fmt.Errorf("resolve fee quoter from datastore: %w", err)
	}
	feeQuoter := solana.PublicKeyFromBytes(fqAddrBytes)
	authority, err := fqops.GetAuthority(e.OperationsBundle.GetContext(), chain, feeQuoter)
	if err != nil {
		return cldf.ChangesetOutput{}, err
	}

	updaterBatches, err := ensurePriceUpdater(e.OperationsBundle, chain, feeQuoter, authority)
	if err != nil {
		return cldf.ChangesetOutput{}, err
	}

	gasUpdates := make([]fqops.GasPriceUpdateInput, 0, len(input.GasPriceUpdates))
	for _, gu := range input.GasPriceUpdates {
		v, _ := new(big.Int).SetString(gu.UsdPerUnitGas, 10)
		gasUpdates = append(gasUpdates, fqops.GasPriceUpdateInput{
			DestChainSelector: gu.DestChainSelector,
			UsdPerUnitGas:     v,
		})
	}

	report, err := cldf_ops.ExecuteOperation(e.OperationsBundle, fqops.UpdatePrices, chain, fqops.UpdatePricesInput{
		FeeQuoter:       feeQuoter,
		GasPriceUpdates: gasUpdates,
	})
	if err != nil {
		return cldf.ChangesetOutput{}, fmt.Errorf("update prices: %w", err)
	}

	// Forward the batches: the price-updater registration rides ahead of the price
	// update, and with a governed (non-deployer) owner the op does not send — it
	// returns the instruction in BatchOps for MCMS assembly.
	batchOps := slices.Concat(updaterBatches, report.Output.BatchOps)
	if err := requireMCMSForBatchOps(e.Logger, input.MCMS, batchOps); err != nil {
		return cldf.ChangesetOutput{}, err
	}
	return changesets.
		NewOutputBuilder(e, changesets.GetRegistry()).
		WithReports([]cldf_ops.Report[any, any]{report.ToGenericReport()}).
		WithBatchOps(batchOps).
		Build(input.MCMS)
}

// ensurePriceUpdater registers the signer used by UpdatePrices.
func ensurePriceUpdater(b cldf_ops.Bundle, chain cldf_solana.Chain, feeQuoter, updater solana.PublicKey) ([]types.BatchOperation, error) {
	priceUpdaterPDA, _, err := state.FindFqAllowedPriceUpdaterPDA(updater, feeQuoter)
	if err != nil {
		return nil, fmt.Errorf("derive allowed price updater PDA: %w", err)
	}
	info, err := chain.Client.GetAccountInfo(b.GetContext(), priceUpdaterPDA)
	if err != nil && !errors.Is(err, rpc.ErrNotFound) {
		return nil, fmt.Errorf("read allowed price updater: %w", err)
	}
	if err == nil && info != nil && info.Value != nil {
		if info.Value.Owner != feeQuoter {
			return nil, fmt.Errorf("allowed price updater %s has unexpected owner %s", priceUpdaterPDA, info.Value.Owner)
		}
		return nil, nil
	}

	authority, err := fqops.GetAuthority(b.GetContext(), chain, feeQuoter)
	if err != nil {
		return nil, err
	}
	configPDA, _, _ := state.FindFqConfigPDA(feeQuoter)
	ixn, err := fee_quoter.NewAddPriceUpdaterInstruction(
		updater,
		priceUpdaterPDA,
		configPDA,
		authority,
		solana.SystemProgramID,
	)
	if err != nil {
		return nil, fmt.Errorf("build add price updater instruction: %w", err)
	}
	ixn, err = solcommon.WithProgramID(ixn, feeQuoter)
	if err != nil {
		return nil, err
	}
	out, err := solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{ixn}, feeQuoter, fqops.ContractType.String())
	if err != nil {
		return nil, err
	}
	return out.BatchOps, nil
}
