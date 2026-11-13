package adapters

import (
	"context"
	"errors"
	"fmt"

	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	"github.com/smartcontractkit/chainlink-ccip/deployment/finality"
	ccvadapters "github.com/smartcontractkit/chainlink-ccv/deployment/adapters"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	"github.com/smartcontractkit/chainlink-deployments-framework/deployment"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations"
	mcmstypes "github.com/smartcontractkit/mcms/types"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/committee_verifier"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/allowlist"
	cvops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/committeeverifier"
	"github.com/smartcontractkit/chainlink-ccip-solana/pkg/solutil"
)

// SolanaCommitteeVerifierOnchainAdapter implements the CCV CommitteeVerifierOnchainAdapter.
type SolanaCommitteeVerifierOnchainAdapter struct{}

var _ ccvadapters.CommitteeVerifierOnchainAdapter = (*SolanaCommitteeVerifierOnchainAdapter)(nil)

// ScanCommitteeStates lists RemoteChainConfig PDAs via getProgramAccounts (pubkeys
// only) then fetches them via batched getMultipleAccounts. Two-step to stay under RPC
// response-size limits as source-chain count and signer count grow; sequential batches
// to avoid RPC rate limits. Assumes no concurrent committee config changesets against
// the same destination chain. Skips accounts with threshold == 0 (cleared by
// remove_signer_config).
func (a *SolanaCommitteeVerifierOnchainAdapter) ScanCommitteeStates(
	ctx context.Context,
	env deployment.Environment,
	chainSelector uint64,
) ([]*ccvadapters.CommitteeState, error) {
	if env.DataStore == nil {
		return nil, errors.New("datastore is required")
	}

	committeeRefs := env.DataStore.Addresses().Filter(
		datastore.AddressRefByType(datastore.ContractType(cvops.ContractType)),
		datastore.AddressRefByChainSelector(chainSelector),
		datastore.AddressRefByVersion(cvops.Version),
	)
	if len(committeeRefs) == 0 {
		return nil, nil
	}

	chain, ok := env.BlockChains.SolanaChains()[chainSelector]
	if !ok {
		return nil, fmt.Errorf("solana chain %d not found in environment", chainSelector)
	}

	states := make([]*ccvadapters.CommitteeState, 0, len(committeeRefs))
	for _, ref := range committeeRefs {
		if ref.Qualifier == "" {
			continue
		}
		programID, err := solana.PublicKeyFromBase58(ref.Address)
		if err != nil {
			return nil, fmt.Errorf("invalid committee verifier program ID %q for qualifier %q: %w", ref.Address, ref.Qualifier, err)
		}
		sigConfigs, err := scanRemoteChainConfigs(ctx, chain.Client, programID)
		if err != nil {
			return nil, fmt.Errorf("scan committee %q on chain %d: %w", ref.Qualifier, chainSelector, err)
		}
		states = append(states, &ccvadapters.CommitteeState{
			Qualifier:        ref.Qualifier,
			ChainSelector:    chainSelector,
			Address:          ref.Address,
			SignatureConfigs: sigConfigs,
		})
	}
	return states, nil
}

// scanRemoteChainConfigs lists then fetches RemoteChainConfig PDAs owned by the given
// committee_verifier program deployment. See ScanCommitteeStates for rationale.
func scanRemoteChainConfigs(
	ctx context.Context,
	client *rpc.Client,
	programID solana.PublicKey,
) ([]ccvadapters.SignatureConfig, error) {
	scanned, err := solutil.ScanProgramAccountsByDiscriminator(
		ctx,
		client,
		programID,
		committee_verifier.Account_RemoteChainConfig,
		0, // default batch size
	)
	if err != nil {
		return nil, err
	}
	sigConfigs := make([]ccvadapters.SignatureConfig, 0, len(scanned))
	for _, s := range scanned {
		if s.Account == nil {
			continue
		}
		sigCfg, err := parseRemoteChainConfig(s.Account.Data.GetBinary())
		if err != nil {
			return nil, fmt.Errorf("parse RemoteChainConfig at %s: %w", s.Pubkey, err)
		}
		if sigCfg == nil {
			continue
		}
		sigConfigs = append(sigConfigs, *sigCfg)
	}
	return sigConfigs, nil
}

// ApplySignatureConfigs processes removals before updates so that a remove-and-re-add for
// the same source selector lands on the new config — same ordering as the EVM contract.
// On Solana there is no bulk apply instruction, so each program instruction is mined directly or returned as a batch operation.
//
// Removal of a source chain selector that is not configured on-chain returns the underlying
// Anchor account-constraint error; callers should pre-filter against a fresh view of
// on-chain state. EVM-style silent no-op semantics will return when `ScanCommitteeStates`
// reads on-chain.
func (a *SolanaCommitteeVerifierOnchainAdapter) ApplySignatureConfigs(
	_ context.Context,
	env deployment.Environment,
	destChainSelector uint64,
	qualifier string,
	change ccvadapters.SignatureConfigChange,
) ([]mcmstypes.BatchOperation, error) {
	if env.DataStore == nil {
		return nil, errors.New("datastore is required")
	}

	ref, err := committeeVerifierRef(env.DataStore, destChainSelector, qualifier)
	if err != nil {
		return nil, err
	}
	cvProgramID, err := solana.PublicKeyFromBase58(ref.Address)
	if err != nil {
		return nil, fmt.Errorf("parse committee verifier address: %w", err)
	}

	chain, ok := env.BlockChains.SolanaChains()[destChainSelector]
	if !ok {
		return nil, fmt.Errorf("solana chain %d not found in environment", destChainSelector)
	}
	if chain.Deployer() == nil {
		return nil, fmt.Errorf("missing Solana deployer key for chain %d", destChainSelector)
	}

	var batchOps []mcmstypes.BatchOperation
	for _, sel := range change.RemovedSourceChainSelectors {
		report, err := operations.ExecuteOperation(env.OperationsBundle, cvops.RemoveSignerConfig, chain, cvops.RemoveSignerConfigParams{
			CommitteeVerifier:   cvProgramID,
			SourceChainSelector: sel,
		})
		if err != nil {
			return nil, fmt.Errorf("remove_signer_config failed on chain %d source %d: %w", destChainSelector, sel, err)
		}
		batchOps = append(batchOps, report.Output.BatchOps...)
	}

	for _, cfg := range change.NewConfigs {
		signers, err := convertEVMSignerHexes(cfg.Signers)
		if err != nil {
			return nil, fmt.Errorf("invalid signers for chain %d source %d: %w", destChainSelector, cfg.SourceChainSelector, err)
		}
		report, err := operations.ExecuteOperation(env.OperationsBundle, cvops.SetSignerConfig, chain, cvops.SetSignerConfigParams{
			CommitteeVerifier:   cvProgramID,
			LocalChainSelector:  chain.Selector,
			SourceChainSelector: cfg.SourceChainSelector,
			Threshold:           cfg.Threshold,
			Signers:             signers,
		})
		if err != nil {
			return nil, fmt.Errorf("set_signer_config failed on chain %d source %d: %w", destChainSelector, cfg.SourceChainSelector, err)
		}
		batchOps = append(batchOps, report.Output.BatchOps...)
	}

	return batchOps, nil
}

// SetAllowedFinalityConfig sets the allowed-finality config on the committee verifier's
// program-global Config PDA. The chainSelector is the chain the verifier lives on; the
// config is not per remote chain on Solana.
func (a *SolanaCommitteeVerifierOnchainAdapter) SetAllowedFinalityConfig(
	_ context.Context,
	env deployment.Environment,
	chainSelector uint64,
	qualifier string,
	waitForFinality bool,
	waitForSafe bool,
	blockDepth uint16,
) ([]mcmstypes.BatchOperation, error) {
	if env.DataStore == nil {
		return nil, errors.New("datastore is required")
	}

	ref, err := committeeVerifierRef(env.DataStore, chainSelector, qualifier)
	if err != nil {
		return nil, err
	}
	cvProgramID, err := solana.PublicKeyFromBase58(ref.Address)
	if err != nil {
		return nil, fmt.Errorf("parse committee verifier address: %w", err)
	}

	chain, ok := env.BlockChains.SolanaChains()[chainSelector]
	if !ok {
		return nil, fmt.Errorf("solana chain %d not found in environment", chainSelector)
	}
	if chain.Deployer() == nil {
		return nil, fmt.Errorf("missing Solana deployer key for chain %d", chainSelector)
	}

	report, err := operations.ExecuteOperation(env.OperationsBundle, cvops.SetAllowedFinalityConfig, chain, cvops.SetAllowedFinalityConfigParams{
		CommitteeVerifier: cvProgramID,
		Finality: cvops.FinalityConfigFromCommon(finality.Config{
			WaitForFinality: waitForFinality,
			WaitForSafe:     waitForSafe,
			BlockDepth:      blockDepth,
		}),
	})
	if err != nil {
		return nil, fmt.Errorf("set_allowed_finality_config failed on chain %d: %w", chainSelector, err)
	}

	return report.Output.BatchOps, nil
}

// ApplyAllowlistUpdates updates the sender allowlist on the destination chain's
// RemoteChainConfig PDA. The on-chain set_allowlist instruction replaces the full sender
// list, so we merge against the current on-chain state: preserve existing order, drop
// any address in removedSenders, then append addedSenders (skipping duplicates). Sender
// strings are family-native base58 Solana pubkeys.
//
// Removal of a sender that is not currently on-chain is a no-op, matching EVM semantics.
// The RemoteChainConfig PDA must already exist (created by init_chain_config); otherwise
// the underlying Anchor constraint returns the error unchanged.
func (a *SolanaCommitteeVerifierOnchainAdapter) ApplyAllowlistUpdates(
	ctx context.Context,
	env deployment.Environment,
	chainSelector uint64,
	qualifier string,
	destChainSelector uint64,
	allowlistEnabled bool,
	addedSenders []string,
	removedSenders []string,
) ([]mcmstypes.BatchOperation, error) {
	if env.DataStore == nil {
		return nil, errors.New("datastore is required")
	}

	ref, err := committeeVerifierRef(env.DataStore, chainSelector, qualifier)
	if err != nil {
		return nil, err
	}
	cvProgramID, err := solana.PublicKeyFromBase58(ref.Address)
	if err != nil {
		return nil, fmt.Errorf("parse committee verifier address: %w", err)
	}

	chain, ok := env.BlockChains.SolanaChains()[chainSelector]
	if !ok {
		return nil, fmt.Errorf("solana chain %d not found in environment", chainSelector)
	}
	if chain.Deployer() == nil {
		return nil, fmt.Errorf("missing Solana deployer key for chain %d", chainSelector)
	}

	toAdd, err := allowlist.ParseSenders(addedSenders, "added")
	if err != nil {
		return nil, fmt.Errorf("chain %d dest %d: %w", chainSelector, destChainSelector, err)
	}
	toRemove, err := allowlist.ParseSenders(removedSenders, "removed")
	if err != nil {
		return nil, fmt.Errorf("chain %d dest %d: %w", chainSelector, destChainSelector, err)
	}

	report, err := operations.ExecuteOperation(env.OperationsBundle, cvops.SetAllowlist, chain, cvops.SetAllowlistParams{
		CommitteeVerifier:      cvProgramID,
		DestChainSelector:      destChainSelector,
		Enabled:                allowlistEnabled,
		AllowedSendersToAdd:    toAdd,
		AllowedSendersToRemove: toRemove,
	})
	if err != nil {
		return nil, fmt.Errorf("set_allowlist failed on chain %d dest %d: %w", chainSelector, destChainSelector, err)
	}
	return report.Output.BatchOps, nil
}

// convertEVMSignerHexes converts the canonical EVM-style hex addresses carried in the
// adapter contract (with or without 0x prefix) to the [20]byte signer encoding used by the
// committee_verifier program.
func convertEVMSignerHexes(signers []string) ([][20]uint8, error) {
	out := make([][20]uint8, 0, len(signers))
	for _, s := range signers {
		if !ethcommon.IsHexAddress(s) {
			return nil, fmt.Errorf("signer %q is not a valid EVM address", s)
		}
		out = append(out, ethcommon.HexToAddress(s))
	}
	return out, nil
}

// parseRemoteChainConfig decodes a RemoteChainConfig payload. Returns (nil, nil) when
// the signer config has been cleared (threshold == 0).
func parseRemoteChainConfig(data []byte) (*ccvadapters.SignatureConfig, error) {
	cfg, err := committee_verifier.ParseAccount_RemoteChainConfig(data)
	if err != nil {
		return nil, err
	}
	if cfg.SignerConfig.Threshold == 0 {
		return nil, nil
	}
	signers := make([]string, 0, len(cfg.SignerConfig.Signers))
	for _, s := range cfg.SignerConfig.Signers {
		signers = append(signers, ethcommon.BytesToAddress(s[:]).Hex())
	}
	return &ccvadapters.SignatureConfig{
		SourceChainSelector: cfg.RemoteChainSelector,
		Signers:             signers,
		Threshold:           cfg.SignerConfig.Threshold,
	}, nil
}
