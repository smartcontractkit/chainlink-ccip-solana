package adapters

import (
	"fmt"

	"github.com/Masterminds/semver/v3"

	cciputils "github.com/smartcontractkit/chainlink-ccip/deployment/utils"
	ccipadapters "github.com/smartcontractkit/chainlink-ccip/deployment/v2_0_0/adapters"
	ccvadapters "github.com/smartcontractkit/chainlink-ccv/deployment/adapters"
	cldf_chain "github.com/smartcontractkit/chainlink-deployments-framework/chain"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations"

	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/committeeverifier"
	solseq "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/sequences"
)

// SolanaLaneConfigAdapter implements CCV LaneConfigAdapter (topology-free lane config,
// chainlink-ccv #1127) for Solana by delegating to ConfigureChainForLanes.
//
// Scope: local lane wiring (FeeQuoter, Router, Executor dest config, OffRamp source config).
// Two fields are intentionally omitted, mirroring how the sequence treats them as optional:
//   - Remote OnRamp/OffRamp: live on the remote chain, not in ExistingAddresses; supply
//     via a follow-up step if a remote source must be allow-listed.
//   - CV signer sets: owned by apply_verifier_config / add_nop_to_committee, not lane config.
//
// Inbound/outbound CCV qualifiers in RemoteLaneConfig are honored.
type SolanaLaneConfigAdapter struct{}

var _ ccvadapters.LaneConfigAdapter = (*SolanaLaneConfigAdapter)(nil)

func (a *SolanaLaneConfigAdapter) ConfigureLane() *operations.Sequence[ccvadapters.LaneConfigInput, ccvadapters.LaneConfigOutput, cldf_chain.BlockChains] {
	return operations.NewSequence(
		"solana-configure-lane",
		semver.MustParse("2.0.0"),
		"Configures CCIP 2.0 lanes on a Solana chain (topology-free CCV path)",
		func(b operations.Bundle, chains cldf_chain.BlockChains, input ccvadapters.LaneConfigInput) (ccvadapters.LaneConfigOutput, error) {
			fam := &solseq.SolanaAdapter{}

			// The resolver methods expect a DataStore; rebuild one from the local refs.
			ds, err := sealDataStore(input.ExistingAddresses)
			if err != nil {
				return ccvadapters.LaneConfigOutput{}, err
			}

			// Resolve local contracts.
			var routerBytes []byte
			if input.UseTestRouter {
				routerBytes, err = fam.GetTestRouter(ds, input.ChainSelector)
			} else {
				routerBytes, err = fam.GetRouterAddress(ds, input.ChainSelector)
			}
			if err != nil {
				return ccvadapters.LaneConfigOutput{}, fmt.Errorf("resolve router on chain %d: %w", input.ChainSelector, err)
			}
			onRampBytes, err := fam.GetOnRampAddress(ds, input.ChainSelector)
			if err != nil {
				return ccvadapters.LaneConfigOutput{}, fmt.Errorf("resolve onRamp on chain %d: %w", input.ChainSelector, err)
			}
			feeQuoterBytes, err := fam.GetFQAddress(ds, input.ChainSelector)
			if err != nil {
				return ccvadapters.LaneConfigOutput{}, fmt.Errorf("resolve feeQuoter on chain %d: %w", input.ChainSelector, err)
			}
			offRampBytes, err := fam.GetOffRampAddress(ds, input.ChainSelector)
			if err != nil {
				return ccvadapters.LaneConfigOutput{}, fmt.Errorf("resolve offRamp on chain %d: %w", input.ChainSelector, err)
			}

			remoteChains := make(map[uint64]ccipadapters.RemoteChainConfig[[]byte, string], len(input.RemoteChains))
			for remoteSelector, rlc := range input.RemoteChains {
				remoteCfg, rErr := buildRemoteChainConfig(fam, ds, input.ChainSelector, remoteSelector, rlc)
				if rErr != nil {
					return ccvadapters.LaneConfigOutput{}, fmt.Errorf("remote chain %d: %w", remoteSelector, rErr)
				}
				remoteChains[remoteSelector] = remoteCfg
			}

			report, err := operations.ExecuteSequence(
				b,
				fam.ConfigureChainForLanes(),
				chains,
				ccipadapters.ConfigureChainForLanesInput{
					ChainSelector:       input.ChainSelector,
					AllowOnrampOverride: input.UseTestRouter,
					Router:              routerBytes,
					OnRamp:              onRampBytes,
					FeeQuoter:           feeQuoterBytes,
					OffRamp:             offRampBytes,
					RemoteChains:        remoteChains,
				},
			)
			if err != nil {
				return ccvadapters.LaneConfigOutput{}, fmt.Errorf("execute Solana ConfigureChainForLanes: %w", err)
			}

			return ccvadapters.LaneConfigOutput{
				Addresses: report.Output.Addresses,
				BatchOps:  report.Output.BatchOps,
			}, nil
		},
	)
}

// GetOnRampAddress resolves the OnRamp address for chainSelector from the datastore, in
// Solana's native byte encoding. The cross-family lane changeset calls this on the remote
// chain's adapter to resolve a Solana chain's OnRamp before wiring the local side of a lane.
// Delegates to SolanaAdapter, the same resolver ConfigureLane uses.
func (a *SolanaLaneConfigAdapter) GetOnRampAddress(ds datastore.DataStore, chainSelector uint64) ([]byte, error) {
	return (&solseq.SolanaAdapter{}).GetOnRampAddress(ds, chainSelector)
}

// GetOffRampAddress resolves the OffRamp address for chainSelector from the datastore, in
// Solana's native byte encoding. Delegates to SolanaAdapter, the same resolver
// ConfigureLane uses.
func (a *SolanaLaneConfigAdapter) GetOffRampAddress(ds datastore.DataStore, chainSelector uint64) ([]byte, error) {
	return (&solseq.SolanaAdapter{}).GetOffRampAddress(ds, chainSelector)
}

// buildRemoteChainConfig mirrors the CCIP topology changeset's resolveRemoteChainConfig,
// driven from the sparse CCV RemoteLaneConfig instead of a topology entry. Remote-chain
// addresses (OnRamp/OffRamp) are omitted — they are not in the local ExistingAddresses and
// the sequence treats them as optional.
func buildRemoteChainConfig(
	fam *solseq.SolanaAdapter,
	ds datastore.DataStore,
	localSelector uint64,
	remoteSelector uint64,
	rlc ccvadapters.RemoteLaneConfig,
) (ccipadapters.RemoteChainConfig[[]byte, string], error) {
	executorAddr, err := fam.ResolveExecutor(ds, localSelector, rlc.ExecutorQualifier)
	if err != nil {
		return ccipadapters.RemoteChainConfig[[]byte, string]{}, fmt.Errorf("resolve executor (qualifier %q): %w", rlc.ExecutorQualifier, err)
	}

	defaultInboundCCVs, err := resolveCCVAddresses(ds, localSelector, rlc.InboundCCVQualifiers)
	if err != nil {
		return ccipadapters.RemoteChainConfig[[]byte, string]{}, fmt.Errorf("resolve inbound CCVs: %w", err)
	}
	defaultOutboundCCVs, err := resolveCCVAddresses(ds, localSelector, rlc.OutboundCCVQualifiers)
	if err != nil {
		return ccipadapters.RemoteChainConfig[[]byte, string]{}, fmt.Errorf("resolve outbound CCVs: %w", err)
	}

	fqConfig := fam.GetDefaultFeeQuoterDestChainConfig(localSelector, remoteSelector, fam.GetChainFamilySelector())

	defaults := fam.GetDefaultRemoteChainConfig(localSelector, remoteSelector)

	allowTrafficFrom := cciputils.Coalesce(rlc.AllowTrafficFrom, defaults.AllowTrafficFrom)
	tokenReceiverAllowed := cciputils.Coalesce(rlc.TokenReceiverAllowed, defaults.TokenReceiverAllowed)

	return ccipadapters.RemoteChainConfig[[]byte, string]{
		AllowTrafficFrom:          &allowTrafficFrom,
		DefaultExecutor:           executorAddr,
		DefaultInboundCCVs:        defaultInboundCCVs,
		DefaultOutboundCCVs:       defaultOutboundCCVs,
		FeeQuoterDestChainConfig:  fqConfig,
		ExecutorDestChainConfig:   defaults.ExecutorDestChainConfig,
		AddressBytesLength:        fam.GetAddressBytesLength(),
		BaseExecutionGasCost:      cciputils.Coalesce(rlc.BaseExecutionGasCost, defaults.BaseExecutionGasCost),
		TokenReceiverAllowed:      &tokenReceiverAllowed,
		MessageNetworkFeeUSDCents: cciputils.Coalesce(rlc.MessageNetworkFeeUSDCents, defaults.MessageNetworkFeeUSDCents),
		TokenNetworkFeeUSDCents:   cciputils.Coalesce(rlc.TokenNetworkFeeUSDCents, defaults.TokenNetworkFeeUSDCents),
	}, nil
}

// resolveCCVAddresses resolves committee-verifier qualifiers to local CV addresses,
// mirroring SolanaAdapter.ResolveExecutor.
func resolveCCVAddresses(ds datastore.DataStore, chainSelector uint64, qualifiers []string) ([]string, error) {
	if len(qualifiers) == 0 {
		return nil, nil
	}
	toAddress := func(ref datastore.AddressRef) (string, error) { return ref.Address, nil }
	out := make([]string, 0, len(qualifiers))
	for _, q := range qualifiers {
		addr, err := solcommon.FindAndFormatCompatibleRef(ds, datastore.AddressRef{
			Type:      datastore.ContractType(committeeverifier.ContractType),
			Qualifier: q,
			Version:   committeeverifier.Version,
		}, chainSelector, toAddress)
		if err != nil {
			return nil, fmt.Errorf("resolve committee verifier (qualifier %q) on chain %d: %w", q, chainSelector, err)
		}
		out = append(out, addr)
	}
	return out, nil
}

// sealDataStore builds a sealed in-memory DataStore from a slice of address refs so the
// SolanaAdapter resolver methods can be reused unchanged.
func sealDataStore(refs []datastore.AddressRef) (datastore.DataStore, error) {
	ms := datastore.NewMemoryDataStore()
	for _, ref := range refs {
		if err := ms.Addresses().Add(ref); err != nil {
			return nil, fmt.Errorf("add address ref (%s/%s) to datastore: %w", ref.Type, ref.Qualifier, err)
		}
	}
	return ms.Seal(), nil
}
