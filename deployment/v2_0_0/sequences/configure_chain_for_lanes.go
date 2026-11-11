package sequences

import (
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/Masterminds/semver/v3"
	"github.com/ethereum/go-ethereum/common"
	"github.com/gagliardetto/solana-go"

	seq_core "github.com/smartcontractkit/chainlink-ccip/deployment/utils/sequences"
	ccvadapters "github.com/smartcontractkit/chainlink-ccip/deployment/v2_0_0/adapters"
	cldf_chain "github.com/smartcontractkit/chainlink-deployments-framework/chain"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	cldf_ops "github.com/smartcontractkit/chainlink-deployments-framework/operations"
	"github.com/smartcontractkit/mcms/types"

	solstate "github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/allowlist"
	cvops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/committeeverifier"
	executorops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/executor"
	fqops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/feequoter"
	offrampops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/offramp"
	routerops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/router"
)

// ConfigureChainForLanesSequence is the canonical sequence for configuring a Solana chain
// to participate in CCIP 2.0 lanes with multiple remote chains. For each remote chain it
// configures FeeQuoter dest chain config, Router dest chain, OffRamp source chain, and
// registers the OffRamp on the Router.
var ConfigureChainForLanesSequence = cldf_ops.NewSequence(
	"configure-chain-for-lanes",
	semver.MustParse("2.0.0"),
	"Configures a Solana chain as a source and destination for multiple remote chains",
	func(b cldf_ops.Bundle, chains cldf_chain.BlockChains, input ccvadapters.ConfigureChainForLanesInput) (seq_core.OnChainOutput, error) {
		return configureChainForLanes(b, chains, input, nil)
	},
)

// ConfigureChainForLanesStagedInput is the input of ConfigureChainForLanesStagedSequence.
type ConfigureChainForLanesStagedInput struct {
	ccvadapters.ConfigureChainForLanesInput
	// AllowedSenders are the only senders the staged lanes accept until they are promoted.
	AllowedSenders []solana.PublicKey
}

// ConfigureChainForLanesStagedSequence configures lanes like ConfigureChainForLanesSequence, but
// with each v2 lane's Router allowlist enabled for AllowedSenders only. The allowlist is part of
// the same Router write that creates the lane, so a new lane is never open to the public before
// it is promoted. This is the Solana side of staging a lane on 2.0, the equivalent of an EVM lane
// pointed at the TestRouter.
var ConfigureChainForLanesStagedSequence = cldf_ops.NewSequence(
	"configure-chain-for-lanes-staged",
	semver.MustParse("2.0.0"),
	"Configures a Solana chain's 2.0 lanes with the Router allowlist enabled for test senders only",
	func(b cldf_ops.Bundle, chains cldf_chain.BlockChains, input ConfigureChainForLanesStagedInput) (seq_core.OnChainOutput, error) {
		if len(input.AllowedSenders) == 0 {
			return seq_core.OnChainOutput{}, errors.New("staged lanes need at least one allowed sender")
		}
		enabled := true
		return configureChainForLanes(b, chains, input.ConfigureChainForLanesInput, &routerops.DestChainAllowlist{
			Enabled:             &enabled,
			AllowedSendersToAdd: input.AllowedSenders,
		})
	},
)

// configureChainForLanes writes allowlist to every v2 lane it configures on the Router when set;
// left nil, existing lanes keep their allowlist and new ones start open, unless the lane has test senders.
func configureChainForLanes(
	b cldf_ops.Bundle,
	chains cldf_chain.BlockChains,
	input ccvadapters.ConfigureChainForLanesInput,
	allowlist *routerops.DestChainAllowlist,
) (seq_core.OnChainOutput, error) {
	var result seq_core.OnChainOutput

	solChains := chains.SolanaChains()
	solChain, ok := solChains[input.ChainSelector]
	if !ok {
		return seq_core.OnChainOutput{}, fmt.Errorf("solana chain with selector %d not found", input.ChainSelector)
	}

	routerPubkey, err := solcommon.BytesToPubkey(input.Router)
	if err != nil {
		return seq_core.OnChainOutput{}, fmt.Errorf("parse router address: %w", err)
	}
	offRampPubkey, err := solcommon.BytesToPubkey(input.OffRamp)
	if err != nil {
		return seq_core.OnChainOutput{}, fmt.Errorf("parse offramp address: %w", err)
	}
	feeQuoterPubkey, err := solcommon.BytesToPubkey(input.FeeQuoter)
	if err != nil {
		return seq_core.OnChainOutput{}, fmt.Errorf("parse fee quoter address: %w", err)
	}

	// The committee verifier's allowed finality lives on its program-global Config PDA
	// (not per remote chain), so apply it once per verifier before wiring the lanes.
	// Without it the config stays at its default (full finality only) and the verifier's
	// get_fee rejects any block-depth or safe-tag request with InvalidFinality.
	for _, cv := range input.CommitteeVerifiers {
		cvProgramID, cvErr := solcommon.PubkeyFromRefs(
			cv.CommitteeVerifier,
			datastore.NewAddressRefKey(
				solChain.Selector,
				datastore.ContractType(cvops.ContractType),
				cvops.Version,
				"",
			),
		)
		if cvErr != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("extract committee verifier address: %w", cvErr)
		}
		if err = runOp(b, cvops.SetAllowedFinalityConfig, solChain, cvops.SetAllowedFinalityConfigParams{
			CommitteeVerifier: cvProgramID,
			Finality:          cvops.FinalityConfigFromCommon(cv.AllowedFinalityConfig),
		}, &result.BatchOps); err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("set cv allowed finality config: %w", err)
		}
	}

	// Sorted so the batch order is deterministic.
	for _, remoteSelector := range slices.Sorted(maps.Keys(input.RemoteChains)) {
		remoteConfig := input.RemoteChains[remoteSelector]
		b.Logger.Infof("SVM ConfigureChainForLanes: configuring chain %d for remote %d", input.ChainSelector, remoteSelector)

		// Source-side: configure FeeQuoter with dest chain config for the remote
		fqOut, err := cldf_ops.ExecuteOperation(b, fqops.ConnectChains, solChain, fqops.ConnectChainsParams{
			LocalChainSelector:  solChain.Selector,
			FeeQuoter:           feeQuoterPubkey,
			OffRamp:             offRampPubkey,
			RemoteChainSelector: remoteSelector,
			DestChainConfig:     FeeQuoterDestChainConfigV2FromChainFamily(remoteConfig.FeeQuoterDestChainConfig),
		})
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("failed to configure FeeQuoter for remote %d: %w", remoteSelector, err)
		}
		result.Addresses = append(result.Addresses, fqOut.Output.Addresses...)
		result.BatchOps = append(result.BatchOps, fqOut.Output.BatchOps...)

		// The lane's gas price is not seeded here. quote_gas_for_exec_v2 reads it from the
		// 1.6 dest_chain account (only the 1.6 price updater writes prices today), which
		// this sequence does not create; the offchain price updater fills it in.

		// The default executor is also stored on the Router's dest chain config below, so
		// parse it before wiring the Router.
		executorPubkey, err := solana.PublicKeyFromBase58(remoteConfig.DefaultExecutor)
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("parse executor address: %w", err)
		}

		// Outbound (send-side) CCVs for the Router's dest chain config. The Router requires at
		// least one default or lane-mandated CCV (EVM `_validateDefaultAndMandatedCCVs`).
		var defaultOutboundCCVs []solana.PublicKey
		for _, ccv := range remoteConfig.DefaultOutboundCCVs {
			addr, parseErr := solana.PublicKeyFromBase58(ccv)
			if parseErr != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("parse default outbound CCV: %w", parseErr)
			}
			defaultOutboundCCVs = append(defaultOutboundCCVs, addr)
		}
		var laneMandatedOutboundCCVs []solana.PublicKey
		for _, ccv := range remoteConfig.LaneMandatedOutboundCCVs {
			addr, parseErr := solana.PublicKeyFromBase58(ccv)
			if parseErr != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("parse lane-mandated outbound CCV: %w", parseErr)
			}
			laneMandatedOutboundCCVs = append(laneMandatedOutboundCCVs, addr)
		}

		// Only a lane that ordinary configuration would create needs the on-chain check.
		laneExists := true
		if len(remoteConfig.TestSenders) > 0 && allowlist == nil {
			laneExists, err = routerops.LaneExists(b.GetContext(), solChain, routerPubkey, remoteSelector)
			if err != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("check router lane to remote %d: %w", remoteSelector, err)
			}
		}
		laneAllowlist, err := laneTestSendersAllowlist(remoteConfig.TestSenders, allowlist, laneExists)
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("test senders for remote %d: %w", remoteSelector, err)
		}

		routerOut, err := cldf_ops.ExecuteOperation(b, routerops.ConnectChains, solChain, routerops.ConnectChainsParams{
			LocalChainSelector:              solChain.Selector,
			Router:                          routerPubkey,
			OffRamp:                         offRampPubkey,
			RemoteChainSelector:             remoteSelector,
			RemoteAddressBytesLength:        remoteConfig.AddressBytesLength,
			DestOffRamp:                     remoteConfig.OffRamp,
			DefaultExecutor:                 executorPubkey,
			DefaultCcvs:                     defaultOutboundCCVs,
			LaneMandatedCcvs:                laneMandatedOutboundCCVs,
			MessageNetworkFeeUSDCents:       remoteConfig.MessageNetworkFeeUSDCents,
			TokenTransferNetworkFeeUSDCents: remoteConfig.TokenNetworkFeeUSDCents,
			BaseExecutionGasCost:            remoteConfig.BaseExecutionGasCost,
			MaxFeePerMessageUSDCents:        DefaultMaxFeePerMessageUSDCents,
			TokenReceiverAllowed:            remoteConfig.TokenReceiverAllowed != nil && *remoteConfig.TokenReceiverAllowed,
			Allowlist:                       laneAllowlist,
		})
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("failed to configure Router for remote %d: %w", remoteSelector, err)
		}
		result.Addresses = append(result.Addresses, routerOut.Output.Addresses...)
		result.BatchOps = append(result.BatchOps, routerOut.Output.BatchOps...)

		// Source-side: add dest chain on default Executor
		executorOut, err := cldf_ops.ExecuteOperation(
			b,
			executorops.SetChainConfig,
			solChain,
			executorops.SetChainConfigParams{
				LocalChainSelector:  solChain.Selector,
				Executor:            executorPubkey,
				RemoteChainSelector: remoteSelector,
				UsdCentsFee:         0,
				Enabled:             true,
			},
		)
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("failed to apply dest chain updates to default executor: %w", err)
		}
		result.Addresses = append(result.Addresses, executorOut.Output.Addresses...)
		result.BatchOps = append(result.BatchOps, executorOut.Output.BatchOps...)

		// Source-side: configure committee verifiers
		for _, cv := range input.CommitteeVerifiers {
			remote, ok := cv.RemoteChains[remoteSelector]
			if !ok {
				continue
			}
			cvProgramID, cvErr := solcommon.PubkeyFromRefs(
				cv.CommitteeVerifier,
				datastore.NewAddressRefKey(
					solChain.Selector,
					datastore.ContractType(cvops.ContractType),
					cvops.Version,
					"",
				),
			)
			if cvErr != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("extract committee verifier address: %w", cvErr)
			}
			ccvSignerPDA, _, signerErr := solstate.FindCCVSignerPDA(cvProgramID, routerPubkey)
			if signerErr != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("find ccv signer PDA: %w", signerErr)
			}
			if err = runOp(b, cvops.SetChainConfig, solChain, cvops.SetChainConfigParams{
				CommitteeVerifier:   cvProgramID,
				LocalChainSelector:  solChain.Selector,
				RemoteChainSelector: remoteSelector,
				RouterSigner:        ccvSignerPDA,
				FeeUSDCents:         remote.FeeUSDCents,
				GasForVerification:  remote.GasForVerification,
				PayloadSizeBytes:    uint32(remote.PayloadSizeBytes),
			}, &result.BatchOps); err != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("set cv chain config for remote %d: %w", remoteSelector, err)
			}
			signers := make([][20]uint8, 0, len(remote.SignatureConfig.Signers))
			for _, hexAddr := range remote.SignatureConfig.Signers {
				ethAddr := common.HexToAddress(hexAddr)
				signers = append(signers, [20]uint8(ethAddr))
			}
			if err = runOp(b, cvops.SetSignerConfig, solChain, cvops.SetSignerConfigParams{
				CommitteeVerifier:   cvProgramID,
				LocalChainSelector:  solChain.Selector,
				SourceChainSelector: remoteSelector,
				Threshold:           remote.SignatureConfig.Threshold,
				Signers:             signers,
			}, &result.BatchOps); err != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("set cv signer config for remote %d: %w", remoteSelector, err)
			}

			if err = applyCVAllowlist(b, solChain, cvProgramID, remoteSelector, remote, &result.BatchOps); err != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("set cv allowlist for remote %d: %w", remoteSelector, err)
			}

			// TODO: This shouldn't be part of the LUT
			// Extend the offramp LUT with per-lane PDAs so execute transactions for any
			// configured lane can reference them via the LUT.
			allowedOfframpPDA, aErr := solstate.FindAllowedOfframpPDA(remoteSelector, offRampPubkey, routerPubkey)
			if aErr != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("find allowed offramp PDA for source selector %d: %w", remoteSelector, aErr)
			}
			cvRemoteChainPDA, _, rErr := solstate.FindCVRemoteChainConfigPDA(remoteSelector, cvProgramID)
			if rErr != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("find cv remote chain config PDA for source selector %d: %w", remoteSelector, rErr)
			}
			perLaneLUTKeys := []solana.PublicKey{allowedOfframpPDA, cvRemoteChainPDA}
			if err = solcommon.ExtendOfframpLookupTable(solChain, offRampPubkey, perLaneLUTKeys); err != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("extend offramp lookup table with per-lane keys: %w", err)
			}
		}

		// Dest-side: register OffRamp on Router for this source chain
		addOffRampOut, err := cldf_ops.ExecuteOperation(b, routerops.AddOffRamp, solChain, routerops.ConnectChainsParams{
			LocalChainSelector:  solChain.Selector,
			Router:              routerPubkey,
			OffRamp:             offRampPubkey,
			RemoteChainSelector: remoteSelector,
		})
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("failed to add OffRamp to Router for remote %d: %w", remoteSelector, err)
		}
		result.Addresses = append(result.Addresses, addOffRampOut.Output.Addresses...)
		result.BatchOps = append(result.BatchOps, addOffRampOut.Output.BatchOps...)

		// Dest-side: add source chain config on OffRamp.
		// Every OnRamp the lane config lists is allowed, not just the first: a remote chain
		// upgrading its OnRamp lists both while the old one still has inflight messages, so
		// taking only one would strand them.
		//
		// A lane with no OnRamp at all cannot be configured: the OffRamp rejects an empty set, so
		// reporting it here points at the remote chain whose config is incomplete.
		if len(remoteConfig.OnRamps) == 0 {
			return seq_core.OnChainOutput{}, fmt.Errorf("no OnRamps configured for remote %d: the OffRamp requires at least one", remoteSelector)
		}
		sourceOnRamps := make([][]byte, 0, len(remoteConfig.OnRamps))
		for _, onRamp := range remoteConfig.OnRamps {
			paddedOnRamp, padErr := solcommon.LeftPadTo32Bytes(onRamp)
			if padErr != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("pad source onramp for remote %d: %w", remoteSelector, padErr)
			}
			sourceOnRamps = append(sourceOnRamps, paddedOnRamp)
		}
		isEnabled := true
		if remoteConfig.AllowTrafficFrom != nil {
			isEnabled = *remoteConfig.AllowTrafficFrom
		}
		var defaultCCVs []solana.PublicKey
		for _, ccv := range remoteConfig.DefaultInboundCCVs {
			addr, parseErr := solana.PublicKeyFromBase58(ccv)
			if parseErr != nil {
				return seq_core.OnChainOutput{}, parseErr
			}
			defaultCCVs = append(defaultCCVs, addr)
		}
		var laneMandatedCCVs []solana.PublicKey
		for _, ccv := range remoteConfig.LaneMandatedInboundCCVs {
			addr, parseErr := solana.PublicKeyFromBase58(ccv)
			if parseErr != nil {
				return seq_core.OnChainOutput{}, parseErr
			}
			laneMandatedCCVs = append(laneMandatedCCVs, addr)
		}
		offRampOut, err := cldf_ops.ExecuteOperation(b, offrampops.ConnectChains, solChain, offrampops.ConnectChainsParams{
			LocalChainSelector:        solChain.Selector,
			RemoteChainSelector:       remoteSelector,
			OffRamp:                   offRampPubkey,
			SourceOnRamps:             sourceOnRamps,
			EnabledAsSource:           isEnabled,
			IsRMNVerificationDisabled: true,
			DefaultCCVs:               defaultCCVs,
			LaneMandatedCCVs:          laneMandatedCCVs,
			LegacyAddressLength:       remoteConfig.AddressBytesLength,
		})
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("failed to configure OffRamp for remote %d: %w", remoteSelector, err)
		}
		result.Addresses = append(result.Addresses, offRampOut.Output.Addresses...)
		result.BatchOps = append(result.BatchOps, offRampOut.Output.BatchOps...)
	}

	return result, nil
}

// laneTestSendersAllowlist returns the allowlist changes to write for a lane.
//
// A staged run (non-nil fallback) restricts every lane it configures. Without one, a lane's test
// senders stage it only as it is created: ordinary lane configuration must not re-enable a lane
// that has already been promoted, but a new lane must not start open to the public when the
// caller asked for it to be staged. laneExists is only consulted in that last case.
func laneTestSendersAllowlist(testSenders []string, fallback *routerops.DestChainAllowlist, laneExists bool) (*routerops.DestChainAllowlist, error) {
	if len(testSenders) == 0 {
		return fallback, nil
	}
	senders := make([]solana.PublicKey, 0, len(testSenders))
	for _, sender := range testSenders {
		key, err := solana.PublicKeyFromBase58(sender)
		if err != nil {
			return nil, fmt.Errorf("parse test sender %q: %w", sender, err)
		}
		senders = append(senders, key)
	}
	if fallback == nil && laneExists {
		return nil, nil
	}
	enabled := true
	return &routerops.DestChainAllowlist{Enabled: &enabled, AllowedSendersToAdd: senders}, nil
}

// CcipSendRemoteLutEntries returns the per-remote-chain accounts that ccip_send_v2's own
// resolution always reads for ANY message to remoteSelector
func CcipSendRemoteLutEntries(remoteSelector uint64, router, feeQuoter, mint, committeeVerifier solana.PublicKey) ([]solana.PublicKey, error) {
	destChainStatePDA, err := solstate.FindDestChainStatePDAV2(remoteSelector, router)
	if err != nil {
		return nil, fmt.Errorf("derive router dest chain state PDA: %w", err)
	}
	fqDestChainV2PDA, _, err := solstate.FindFqDestChainV2PDA(remoteSelector, feeQuoter)
	if err != nil {
		return nil, fmt.Errorf("derive fee quoter dest chain v2 PDA: %w", err)
	}
	fqDestChainPDA, _, err := solstate.FindFqDestChainPDA(remoteSelector, feeQuoter)
	if err != nil {
		return nil, fmt.Errorf("derive fee quoter dest chain PDA: %w", err)
	}
	entries := []solana.PublicKey{destChainStatePDA, fqDestChainV2PDA, fqDestChainPDA}

	if !mint.IsZero() {
		fqPerChainPerTokenConfigPDA, _, perTokenErr := solstate.FindFqPerChainPerTokenConfigPDA(remoteSelector, mint, feeQuoter)
		if perTokenErr != nil {
			return nil, fmt.Errorf("derive fee quoter per-chain-per-token config PDA: %w", perTokenErr)
		}
		entries = append(entries, fqPerChainPerTokenConfigPDA)
	}

	if !committeeVerifier.IsZero() {
		cvRemoteChainPDA, _, cvErr := solstate.FindCVRemoteChainConfigPDA(remoteSelector, committeeVerifier)
		if cvErr != nil {
			return nil, fmt.Errorf("derive committee verifier remote chain config PDA: %w", cvErr)
		}
		entries = append(entries, cvRemoteChainPDA)
	}

	return entries, nil
}

// applyCVAllowlist mirrors EVM: the enabled flag follows the input on every run and the senders
// to add and to remove are applied to the lane's current list.
func applyCVAllowlist(
	b cldf_ops.Bundle,
	chain cldf_solana.Chain,
	cvProgramID solana.PublicKey,
	remoteSelector uint64,
	remote ccvadapters.CommitteeVerifierRemoteChainConfig,
	batches *[]types.BatchOperation,
) error {
	toAdd, err := allowlist.ParseSenders(remote.AddedAllowlistedSenders, "added")
	if err != nil {
		return err
	}
	toRemove, err := allowlist.ParseSenders(remote.RemovedAllowlistedSenders, "removed")
	if err != nil {
		return err
	}
	return runOp(b, cvops.SetAllowlist, chain, cvops.SetAllowlistParams{
		CommitteeVerifier:      cvProgramID,
		DestChainSelector:      remoteSelector,
		Enabled:                remote.AllowlistEnabled,
		AllowedSendersToAdd:    toAdd,
		AllowedSendersToRemove: toRemove,
	}, batches)
}
