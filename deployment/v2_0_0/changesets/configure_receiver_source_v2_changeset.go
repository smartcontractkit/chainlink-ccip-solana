package changesets

import (
	"fmt"

	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/mcms"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	cldf "github.com/smartcontractkit/chainlink-deployments-framework/deployment"
	cldf_ops "github.com/smartcontractkit/chainlink-deployments-framework/operations"
	mcmstypes "github.com/smartcontractkit/mcms/types"

	example_ccip_receiver_v2 "github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/example_ccip_receiver_v2"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	cvops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/committeeverifier"
	routerops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/router"
	testreceiverops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/testreceiver"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/sequences"
)

// ConfigureV2TestReceiverSourceChangesetInput configures an already-deployed v2 TestReceiver for a
// new source chain without redeploying or re-initializing it: it carries the receiver's CCV/finality
// config for the source and the remote senders to approve. The receiver is also registered with the
// router, which is skipped when it already is. The source CCV config and each sender are likewise
// skipped when they already exist, so re-running it is a no-op. The v2 receiver only accepts the
// 32-byte sender format. MCMS builds the proposal when the timelock owns the receiver.
type ConfigureV2TestReceiverSourceChangesetInput struct {
	ChainSelector         uint64                                  `json:"chainSelector"`
	ReceiverQualifier     string                                  `json:"receiverQualifier"`
	SourceChainSelector   uint64                                  `json:"sourceChainSelector"`
	ApprovedRemoteSenders []string                                `json:"approvedRemoteSenders"`
	RequiredCCVQualifiers []string                                `json:"requiredCCVQualifiers"`
	OptionalCCVQualifiers []string                                `json:"optionalCCVQualifiers"`
	OptionalThreshold     uint8                                   `json:"optionalThreshold"`
	AllowedFinalityConfig example_ccip_receiver_v2.FinalityConfig `json:"allowedFinalityConfig"`
	MCMS                  mcms.Input                              `json:"mcms"`
}

// ConfigureV2TestReceiverSourceChangeset sets the CCV/finality config for a new source chain on an
// already-deployed v2 TestReceiver and approves the corresponding remote senders on it.
func ConfigureV2TestReceiverSourceChangeset() cldf.ChangeSetV2[ConfigureV2TestReceiverSourceChangesetInput] {
	return cldf.CreateChangeSet(configureV2SourceApply, configureV2SourceVerify)
}

func configureV2SourceVerify(env cldf.Environment, input ConfigureV2TestReceiverSourceChangesetInput) error {
	if err := validateConfigureSourceCommon(input.ChainSelector, input.SourceChainSelector, input.ReceiverQualifier, input.ApprovedRemoteSenders); err != nil {
		return err
	}
	if err := validateMCMSInput(env.Logger, input.MCMS); err != nil {
		return err
	}
	_, addresses, err := fetchSolanaChain(env, input.ChainSelector, testreceiverops.Version, input.ReceiverQualifier)
	if err != nil {
		return err
	}
	if _, initErr := resolveInitializerRef(env, input.ChainSelector, datastore.ContractType(routerops.ContractType), routerops.Version, ""); initErr != nil {
		return fmt.Errorf("resolve Router: %w", initErr)
	}

	receiverV2, err := sequences.ResolveTestReceiver(addresses, input.ChainSelector, input.ReceiverQualifier, testreceiverops.Version)
	if err != nil {
		return fmt.Errorf("resolve v2 receiver (%q): %w", input.ReceiverQualifier, err)
	}
	if _, _, err := state.FindReceiverCCVConfigPDA(input.SourceChainSelector, receiverV2); err != nil {
		return fmt.Errorf("derive source CCV config PDA: %w", err)
	}
	return senderApprovals{
		Receiver:            receiverV2,
		SourceChainSelector: input.SourceChainSelector,
		Senders:             input.ApprovedRemoteSenders,
		ReceiverVersion:     testreceiverops.Version,
	}.validate()
}

func configureV2SourceApply(e cldf.Environment, input ConfigureV2TestReceiverSourceChangesetInput) (cldf.ChangesetOutput, error) {
	addresses, err := e.DataStore.Addresses().Fetch()
	if err != nil {
		return cldf.ChangesetOutput{}, fmt.Errorf("fetch existing addresses: %w", err)
	}
	solChain := e.BlockChains.SolanaChains()[input.ChainSelector]

	receiverV2, err := sequences.ResolveTestReceiver(addresses, input.ChainSelector, input.ReceiverQualifier, testreceiverops.Version)
	if err != nil {
		return cldf.ChangesetOutput{}, fmt.Errorf("resolve v2 receiver: %w", err)
	}

	requiredCCVs, err := cvops.ResolveCommitteeVerifiers(addresses, input.ChainSelector, input.RequiredCCVQualifiers)
	if err != nil {
		return cldf.ChangesetOutput{}, fmt.Errorf("resolve required CCVs: %w", err)
	}
	optionalCCVs, err := cvops.ResolveCommitteeVerifiers(addresses, input.ChainSelector, input.OptionalCCVQualifiers)
	if err != nil {
		return cldf.ChangesetOutput{}, fmt.Errorf("resolve optional CCVs: %w", err)
	}

	router, err := resolveInitializerRef(e, input.ChainSelector, datastore.ContractType(routerops.ContractType), routerops.Version, "")
	if err != nil {
		return cldf.ChangesetOutput{}, fmt.Errorf("resolve Router: %w", err)
	}

	var reports []cldf_ops.Report[any, any]
	var batchOps []mcmstypes.BatchOperation

	registerReport, err := cldf_ops.ExecuteOperation(e.OperationsBundle, testreceiverops.RegisterWithRouter, solChain, testreceiverops.RegisterWithRouterParams{
		Router:   router,
		Receiver: receiverV2,
	})
	if err != nil {
		return cldf.ChangesetOutput{}, fmt.Errorf("register receiver with router: %w", err)
	}
	appendOutput(&reports, &batchOps, registerReport)

	ccvPDA, _, err := state.FindReceiverCCVConfigPDA(input.SourceChainSelector, receiverV2)
	if err != nil {
		return cldf.ChangesetOutput{}, fmt.Errorf("derive source CCV config PDA: %w", err)
	}
	ccvConfigured, err := accountExists(e, solChain, ccvPDA)
	if err != nil {
		return cldf.ChangesetOutput{}, fmt.Errorf("source CCV config: %w", err)
	}
	if ccvConfigured {
		e.Logger.Infof("source CCV config for selector %d on receiver %s already exists (PDA %s), skipping", input.SourceChainSelector, receiverV2, ccvPDA)
	} else {
		ccvReport, ccvErr := cldf_ops.ExecuteOperation(e.OperationsBundle, testreceiverops.SetCCVConfig, solChain, testreceiverops.SetCCVConfigParams{
			Receiver:              receiverV2,
			SourceChainSelector:   input.SourceChainSelector,
			RequiredCCVs:          requiredCCVs,
			OptionalCCVs:          optionalCCVs,
			OptionalThreshold:     input.OptionalThreshold,
			AllowedFinalityConfig: input.AllowedFinalityConfig,
		})
		if ccvErr != nil {
			return cldf.ChangesetOutput{}, fmt.Errorf("set CCV config: %w", ccvErr)
		}
		appendOutput(&reports, &batchOps, ccvReport)
	}

	pending, err := senderApprovals{
		Receiver:            receiverV2,
		SourceChainSelector: input.SourceChainSelector,
		Senders:             input.ApprovedRemoteSenders,
		ReceiverVersion:     testreceiverops.Version,
	}.pending(e, solChain)
	if err != nil {
		return cldf.ChangesetOutput{}, err
	}
	for _, approval := range pending {
		approveReport, err := cldf_ops.ExecuteOperation(e.OperationsBundle, testreceiverops.ApproveSender, solChain, testreceiverops.ApproveSenderParams{
			Receiver:            receiverV2,
			SourceChainSelector: input.SourceChainSelector,
			RemoteSender:        approval.Sender,
			ReceiverVersion:     testreceiverops.Version,
		})
		if err != nil {
			return cldf.ChangesetOutput{}, fmt.Errorf("approve remote sender %q: %w", approval.Sender, err)
		}
		appendOutput(&reports, &batchOps, approveReport)
	}

	return buildProposalOutput(e, input.MCMS, reports, batchOps)
}
