package changesets

import (
	"context"
	"errors"
	"fmt"

	"github.com/Masterminds/semver/v3"
	"github.com/gagliardetto/solana-go"

	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/changesets"
	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/mcms"
	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/sequences"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	cldf "github.com/smartcontractkit/chainlink-deployments-framework/deployment"
	cldf_ops "github.com/smartcontractkit/chainlink-deployments-framework/operations"
	mcmstypes "github.com/smartcontractkit/mcms/types"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/ccip_offramp"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/example_ccip_receiver"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/example_ccip_receiver_v2"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/executor"
	contractcommon "github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	cvops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/committeeverifier"
	executorops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/executor"
	fqops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/feequoter"
	offrampops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/offramp"
	rmnremoteops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/rmnremote"
	routerops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/router"
	testreceiverops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/testreceiver"
	testsenderops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/testsender"
	sequences_v2 "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/sequences"
)

// These changesets initialize programs whose binaries have already been deployed and registered
// in the datastore. They deliberately do not call a Deploy operation: the upgrade changeset (or
// an external first deployment) is responsible for making the program address available.
// Initialization assigns the program's initial on-chain owner to its upgrade authority. When that
// is the timelock rather than the deployer, the changesets return an MCMS proposal built from
// their MCMS input instead of sending the transaction.

type InitializeCommitteeVerifierChangesetInput struct {
	ChainSelector uint64     `json:"chainSelector"`
	MCMS          mcms.Input `json:"mcms"`
}

func InitializeCommitteeVerifierChangeset() cldf.ChangeSetV2[InitializeCommitteeVerifierChangesetInput] {
	return cldf.CreateChangeSet(initializeCommitteeVerifierApply, initializeCommitteeVerifierVerify)
}

func initializeCommitteeVerifierVerify(env cldf.Environment, input InitializeCommitteeVerifierChangesetInput) error {
	if err := validateMCMSInput(env.Logger, input.MCMS); err != nil {
		return err
	}
	_, _, err := resolveInitializerProgram(env, input.ChainSelector, datastore.ContractType(cvops.ContractType), cvops.Version, cvops.DeployQualifier)
	return err
}

func initializeCommitteeVerifierApply(e cldf.Environment, input InitializeCommitteeVerifierChangesetInput) (cldf.ChangesetOutput, error) {
	chain, program, err := resolveInitializerProgram(e, input.ChainSelector, datastore.ContractType(cvops.ContractType), cvops.Version, cvops.DeployQualifier)
	if err != nil {
		return cldf.ChangesetOutput{}, err
	}
	if err = ensureIssuerFeeVaults(e, chain, program); err != nil {
		return cldf.ChangesetOutput{}, err
	}
	report, err := cldf_ops.ExecuteOperation(e.OperationsBundle, cvops.Initialize, chain, cvops.Params{CommitteeVerifier: program})
	if err != nil {
		return cldf.ChangesetOutput{}, fmt.Errorf("initialize Committee Verifier: %w", err)
	}
	return initializerOutput(e, input.MCMS, report)
}

type InitializeExecutorChangesetInput struct {
	ChainSelector         uint64                   `json:"chainSelector"`
	MaxCcvsPerMsg         uint8                    `json:"maxCcvsPerMsg"`
	FeeAggregator         string                   `json:"feeAggregator,omitempty"`
	AllowedFinalityConfig *executor.FinalityConfig `json:"allowedFinalityConfig,omitempty"`
	CcvAllowlistEnabled   bool                     `json:"ccvAllowlistEnabled"`
	MCMS                  mcms.Input               `json:"mcms"`
}

func InitializeExecutorChangeset() cldf.ChangeSetV2[InitializeExecutorChangesetInput] {
	return cldf.CreateChangeSet(initializeExecutorApply, initializeExecutorVerify)
}

func initializeExecutorVerify(env cldf.Environment, input InitializeExecutorChangesetInput) error {
	if input.MaxCcvsPerMsg == 0 {
		return errors.New("maxCcvsPerMsg must be greater than zero")
	}
	if err := validateMCMSInput(env.Logger, input.MCMS); err != nil {
		return err
	}
	if input.FeeAggregator != "" {
		if _, err := solana.PublicKeyFromBase58(input.FeeAggregator); err != nil {
			return fmt.Errorf("invalid feeAggregator %q: %w", input.FeeAggregator, err)
		}
	}
	_, _, err := resolveInitializerProgram(env, input.ChainSelector, datastore.ContractType(executorops.ContractType), executorops.Version, "default")
	return err
}

func initializeExecutorApply(e cldf.Environment, input InitializeExecutorChangesetInput) (cldf.ChangesetOutput, error) {
	chain, program, err := resolveInitializerProgram(e, input.ChainSelector, datastore.ContractType(executorops.ContractType), executorops.Version, "default")
	if err != nil {
		return cldf.ChangesetOutput{}, err
	}
	if err = ensureIssuerFeeVaults(e, chain, program); err != nil {
		return cldf.ChangesetOutput{}, err
	}
	configPDA, _, err := state.FindConfigPDA(program)
	if err != nil {
		return cldf.ChangesetOutput{}, fmt.Errorf("derive Executor config PDA: %w", err)
	}
	initialized, err := contractcommon.IsAccountInitialized(context.Background(), chain.Client, configPDA, program, cldf_solana.SolDefaultCommitment, executor.Account_Config)
	if err != nil {
		return cldf.ChangesetOutput{}, fmt.Errorf("check Executor initialization: %w", err)
	}
	if initialized {
		return cldf.ChangesetOutput{}, nil
	}
	feeAggregator := chain.Deployer().PublicKey()
	if input.FeeAggregator != "" {
		feeAggregator, err = solana.PublicKeyFromBase58(input.FeeAggregator)
		if err != nil {
			return cldf.ChangesetOutput{}, fmt.Errorf("invalid feeAggregator %q: %w", input.FeeAggregator, err)
		}
	}
	finality := executor.FinalityConfig{Flags: 0, BlockDepth: 1}
	if input.AllowedFinalityConfig != nil {
		finality = *input.AllowedFinalityConfig
	}
	report, err := cldf_ops.ExecuteOperation(e.OperationsBundle, executorops.Initialize, chain, executorops.Params{
		Executor:              program,
		MaxCcvsPerMsg:         input.MaxCcvsPerMsg,
		FeeAggregator:         feeAggregator,
		AllowedFinalityConfig: finality,
		CcvAllowlistEnabled:   input.CcvAllowlistEnabled,
	})
	if err != nil {
		return cldf.ChangesetOutput{}, fmt.Errorf("initialize Executor: %w", err)
	}
	return initializerOutput(e, input.MCMS, report)
}

// ensureIssuerFeeVaults creates the issuer program's fee vault token accounts. It is idempotent and
// does not depend on the program's initialization state, so reruns repair already-initialized programs.
// The fee tokens are read from the FeeQuoter on-chain; nothing is added to the datastore.
func ensureIssuerFeeVaults(e cldf.Environment, chain cldf_solana.Chain, issuer solana.PublicKey) error {
	feeQuoter, err := resolveInitializerRef(e, chain.Selector, datastore.ContractType(fqops.ContractType), fqops.Version, "")
	if err != nil {
		return err
	}
	if err = sequences_v2.EnsureIssuerFeeVaults(context.Background(), chain, feeQuoter, issuer); err != nil {
		return fmt.Errorf("ensure fee vaults of %s: %w", issuer, err)
	}
	return nil
}

type InitializeOffRampChangesetInput struct {
	ChainSelector uint64     `json:"chainSelector"`
	MCMS          mcms.Input `json:"mcms"`
}

func InitializeOffRampChangeset() cldf.ChangeSetV2[InitializeOffRampChangesetInput] {
	return cldf.CreateChangeSet(initializeOffRampApply, initializeOffRampVerify)
}

func initializeOffRampVerify(env cldf.Environment, input InitializeOffRampChangesetInput) error {
	if err := validateMCMSInput(env.Logger, input.MCMS); err != nil {
		return err
	}
	for _, target := range []struct {
		typ       datastore.ContractType
		version   *semver.Version
		qualifier string
	}{
		{datastore.ContractType(offrampops.ContractType), offrampops.Version, ""},
		{datastore.ContractType(routerops.ContractType), routerops.Version, ""},
		{datastore.ContractType(fqops.ContractType), fqops.Version, ""},
		{datastore.ContractType(rmnremoteops.ContractType), rmnremoteops.Version, ""},
	} {
		if _, _, err := resolveInitializerProgram(env, input.ChainSelector, target.typ, target.version, target.qualifier); err != nil {
			return err
		}
	}
	return nil
}

func initializeOffRampApply(e cldf.Environment, input InitializeOffRampChangesetInput) (cldf.ChangesetOutput, error) {
	chain, offrampProgram, err := resolveInitializerProgram(e, input.ChainSelector, datastore.ContractType(offrampops.ContractType), offrampops.Version, "")
	if err != nil {
		return cldf.ChangesetOutput{}, err
	}
	refs, err := e.DataStore.Addresses().Fetch()
	if err != nil {
		return cldf.ChangesetOutput{}, fmt.Errorf("fetch address refs: %w", err)
	}
	resolve := func(typ datastore.ContractType, version *semver.Version) (solana.PublicKey, error) {
		return solcommon.PubkeyFromRefs(refs, datastore.NewAddressRefKey(input.ChainSelector, typ, version, ""))
	}
	router, err := resolve(datastore.ContractType(routerops.ContractType), routerops.Version)
	if err != nil {
		return cldf.ChangesetOutput{}, fmt.Errorf("resolve Router: %w", err)
	}
	feeQuoter, err := resolve(datastore.ContractType(fqops.ContractType), fqops.Version)
	if err != nil {
		return cldf.ChangesetOutput{}, fmt.Errorf("resolve FeeQuoter: %w", err)
	}
	rmnRemote, err := resolve(datastore.ContractType(rmnremoteops.ContractType), rmnremoteops.Version)
	if err != nil {
		return cldf.ChangesetOutput{}, fmt.Errorf("resolve RMN Remote: %w", err)
	}
	refAddressesPDA, _, err := state.FindOfframpReferenceAddressesPDA(offrampProgram)
	if err != nil {
		return cldf.ChangesetOutput{}, fmt.Errorf("derive OffRamp reference addresses PDA: %w", err)
	}
	refAddressesInitialized, err := contractcommon.IsAccountInitialized(context.Background(), chain.Client, refAddressesPDA, offrampProgram, cldf_solana.SolDefaultCommitment, ccip_offramp.Account_ReferenceAddresses)
	if err != nil {
		return cldf.ChangesetOutput{}, fmt.Errorf("check OffRamp reference addresses initialization: %w", err)
	}
	configPDA, _, err := state.FindOfframpConfigPDA(offrampProgram)
	if err != nil {
		return cldf.ChangesetOutput{}, fmt.Errorf("derive OffRamp config PDA: %w", err)
	}
	configInitialized, err := contractcommon.IsAccountInitialized(context.Background(), chain.Client, configPDA, offrampProgram, cldf_solana.SolDefaultCommitment, ccip_offramp.Account_Config)
	if err != nil {
		return cldf.ChangesetOutput{}, fmt.Errorf("check OffRamp config initialization: %w", err)
	}
	if configInitialized && !refAddressesInitialized {
		return cldf.ChangesetOutput{}, errors.New("offramp config exists but reference addresses are not initialized")
	}
	var reports []cldf_ops.Report[any, any]
	var batchOps []mcmstypes.BatchOperation
	if !refAddressesInitialized {
		report, opErr := cldf_ops.ExecuteOperation(e.OperationsBundle, offrampops.Initialize, chain, offrampops.Params{
			FeeQuoter: feeQuoter,
			Router:    router,
			OffRamp:   offrampProgram,
			RMNRemote: rmnRemote,
		})
		if opErr != nil {
			return cldf.ChangesetOutput{}, fmt.Errorf("initialize OffRamp: %w", opErr)
		}
		reports = append(reports, report.ToGenericReport())
		batchOps = append(batchOps, report.Output.BatchOps...)
	}
	if !configInitialized {
		report, opErr := cldf_ops.ExecuteOperation(e.OperationsBundle, offrampops.InitializeConfig, chain, offrampops.Params{OffRamp: offrampProgram})
		if opErr != nil {
			return cldf.ChangesetOutput{}, fmt.Errorf("initialize OffRamp config: %w", opErr)
		}
		reports = append(reports, report.ToGenericReport())
		batchOps = append(batchOps, report.Output.BatchOps...)
	}
	return buildProposalOutput(e, input.MCMS, reports, batchOps)
}

type InitializeV2TestSenderChangesetInput struct {
	ChainSelector   uint64     `json:"chainSelector"`
	SenderQualifier string     `json:"senderQualifier"`
	MCMS            mcms.Input `json:"mcms"`
}

func InitializeV2TestSenderChangeset() cldf.ChangeSetV2[InitializeV2TestSenderChangesetInput] {
	return cldf.CreateChangeSet(initializeV2TestSenderApply, initializeV2TestSenderVerify)
}

func initializeV2TestSenderVerify(env cldf.Environment, input InitializeV2TestSenderChangesetInput) error {
	return verifyTestSenderInitializer(env, input.ChainSelector, input.SenderQualifier, testsenderops.Version, input.MCMS)
}

func initializeV2TestSenderApply(e cldf.Environment, input InitializeV2TestSenderChangesetInput) (cldf.ChangesetOutput, error) {
	return initializeTestSender(e, input.ChainSelector, input.SenderQualifier, testsenderops.Version, input.MCMS)
}

type InitializeLegacyTestSenderChangesetInput struct {
	ChainSelector   uint64     `json:"chainSelector"`
	SenderQualifier string     `json:"senderQualifier"`
	MCMS            mcms.Input `json:"mcms"`
}

func InitializeLegacyTestSenderChangeset() cldf.ChangeSetV2[InitializeLegacyTestSenderChangesetInput] {
	return cldf.CreateChangeSet(initializeLegacyTestSenderApply, initializeLegacyTestSenderVerify)
}

func initializeLegacyTestSenderVerify(env cldf.Environment, input InitializeLegacyTestSenderChangesetInput) error {
	return verifyTestSenderInitializer(env, input.ChainSelector, input.SenderQualifier, testsenderops.LegacyVersion, input.MCMS)
}

func initializeLegacyTestSenderApply(e cldf.Environment, input InitializeLegacyTestSenderChangesetInput) (cldf.ChangesetOutput, error) {
	return initializeTestSender(e, input.ChainSelector, input.SenderQualifier, testsenderops.LegacyVersion, input.MCMS)
}

func verifyTestSenderInitializer(env cldf.Environment, chainSelector uint64, qualifier string, version *semver.Version, mcmsInput mcms.Input) error {
	if qualifier == "" {
		return errors.New("senderQualifier is required")
	}
	if err := validateMCMSInput(env.Logger, mcmsInput); err != nil {
		return err
	}
	_, _, err := resolveInitializerProgram(env, chainSelector, datastore.ContractType(testsenderops.ContractType), version, qualifier)
	return err
}

func initializeTestSender(e cldf.Environment, chainSelector uint64, qualifier string, version *semver.Version, mcmsInput mcms.Input) (cldf.ChangesetOutput, error) {
	chain, sender, err := resolveInitializerProgram(e, chainSelector, datastore.ContractType(testsenderops.ContractType), version, qualifier)
	if err != nil {
		return cldf.ChangesetOutput{}, err
	}
	router, err := resolveInitializerRef(e, chainSelector, datastore.ContractType(routerops.ContractType), routerops.Version, "")
	if err != nil {
		return cldf.ChangesetOutput{}, fmt.Errorf("resolve Router: %w", err)
	}
	report, err := cldf_ops.ExecuteOperation(e.OperationsBundle, testsenderops.Initialize, chain, testsenderops.Params{
		Router:        router,
		Sender:        sender,
		SenderVersion: version,
	})
	if err != nil {
		return cldf.ChangesetOutput{}, fmt.Errorf("initialize Test Sender: %w", err)
	}
	return initializerOutput(e, mcmsInput, report)
}

type InitializeV2TestReceiverChangesetInput struct {
	ChainSelector     uint64     `json:"chainSelector"`
	ReceiverQualifier string     `json:"receiverQualifier"`
	MCMS              mcms.Input `json:"mcms"`
}

func InitializeV2TestReceiverChangeset() cldf.ChangeSetV2[InitializeV2TestReceiverChangesetInput] {
	return cldf.CreateChangeSet(initializeV2TestReceiverApply, initializeV2TestReceiverVerify)
}

func initializeV2TestReceiverVerify(env cldf.Environment, input InitializeV2TestReceiverChangesetInput) error {
	return verifyTestReceiverInitializer(env, input.ChainSelector, input.ReceiverQualifier, testreceiverops.Version, input.MCMS)
}

func initializeV2TestReceiverApply(e cldf.Environment, input InitializeV2TestReceiverChangesetInput) (cldf.ChangesetOutput, error) {
	return initializeTestReceiver(e, input.ChainSelector, input.ReceiverQualifier, testreceiverops.Version, input.MCMS)
}

type InitializeLegacyTestReceiverChangesetInput struct {
	ChainSelector     uint64     `json:"chainSelector"`
	ReceiverQualifier string     `json:"receiverQualifier"`
	MCMS              mcms.Input `json:"mcms"`
}

func InitializeLegacyTestReceiverChangeset() cldf.ChangeSetV2[InitializeLegacyTestReceiverChangesetInput] {
	return cldf.CreateChangeSet(initializeLegacyTestReceiverApply, initializeLegacyTestReceiverVerify)
}

func initializeLegacyTestReceiverVerify(env cldf.Environment, input InitializeLegacyTestReceiverChangesetInput) error {
	return verifyTestReceiverInitializer(env, input.ChainSelector, input.ReceiverQualifier, testreceiverops.LegacyVersion, input.MCMS)
}

func initializeLegacyTestReceiverApply(e cldf.Environment, input InitializeLegacyTestReceiverChangesetInput) (cldf.ChangesetOutput, error) {
	return initializeTestReceiver(e, input.ChainSelector, input.ReceiverQualifier, testreceiverops.LegacyVersion, input.MCMS)
}

func verifyTestReceiverInitializer(env cldf.Environment, chainSelector uint64, qualifier string, version *semver.Version, mcmsInput mcms.Input) error {
	if qualifier == "" {
		return errors.New("receiverQualifier is required")
	}
	if err := validateMCMSInput(env.Logger, mcmsInput); err != nil {
		return err
	}
	_, _, err := resolveInitializerProgram(env, chainSelector, datastore.ContractType(testreceiverops.ContractType), version, qualifier)
	return err
}

func initializeTestReceiver(e cldf.Environment, chainSelector uint64, qualifier string, version *semver.Version, mcmsInput mcms.Input) (cldf.ChangesetOutput, error) {
	chain, receiver, err := resolveInitializerProgram(e, chainSelector, datastore.ContractType(testreceiverops.ContractType), version, qualifier)
	if err != nil {
		return cldf.ChangesetOutput{}, err
	}
	router, err := resolveInitializerRef(e, chainSelector, datastore.ContractType(routerops.ContractType), routerops.Version, "")
	if err != nil {
		return cldf.ChangesetOutput{}, fmt.Errorf("resolve Router: %w", err)
	}
	statePDA, discriminator, err := testReceiverInitState(receiver, version)
	if err != nil {
		return cldf.ChangesetOutput{}, err
	}
	initialized, err := contractcommon.IsAccountInitialized(context.Background(), chain.Client, statePDA, receiver, cldf_solana.SolDefaultCommitment, discriminator)
	if err != nil {
		return cldf.ChangesetOutput{}, fmt.Errorf("check Test Receiver initialization: %w", err)
	}
	if initialized {
		return cldf.ChangesetOutput{}, nil
	}
	report, err := cldf_ops.ExecuteOperation(e.OperationsBundle, testreceiverops.Initialize, chain, testreceiverops.Params{
		Router:          router,
		Receiver:        receiver,
		ReceiverVersion: version,
	})
	if err != nil {
		return cldf.ChangesetOutput{}, fmt.Errorf("initialize Test Receiver: %w", err)
	}
	return initializerOutput(e, mcmsInput, report)
}

func testReceiverInitState(receiver solana.PublicKey, version *semver.Version) (solana.PublicKey, [8]byte, error) {
	if version.Major() == testreceiverops.LegacyVersion.Major() {
		pda, _, err := solana.FindProgramAddress([][]byte{[]byte("state")}, receiver)
		return pda, example_ccip_receiver.Account_BaseState, err
	}
	pda, _, err := solana.FindProgramAddress([][]byte{[]byte("config")}, receiver)
	return pda, example_ccip_receiver_v2.Account_Config, err
}

func resolveInitializerProgram(env cldf.Environment, chainSelector uint64, contractType datastore.ContractType, version *semver.Version, qualifier string) (cldf_solana.Chain, solana.PublicKey, error) {
	if err := cldf.IsValidChainSelector(chainSelector); err != nil {
		return cldf_solana.Chain{}, solana.PublicKey{}, fmt.Errorf("invalid chain selector %d: %w", chainSelector, err)
	}
	chain, exists := env.BlockChains.SolanaChains()[chainSelector]
	if !exists {
		return cldf_solana.Chain{}, solana.PublicKey{}, fmt.Errorf("solana chain %d does not exist", chainSelector)
	}
	if chain.Deployer() == nil {
		return cldf_solana.Chain{}, solana.PublicKey{}, fmt.Errorf("solana deployer key is required for chain %d", chainSelector)
	}
	address, err := resolveInitializerRef(env, chainSelector, contractType, version, qualifier)
	if err != nil {
		return cldf_solana.Chain{}, solana.PublicKey{}, err
	}
	return chain, address, nil
}

func resolveInitializerRef(env cldf.Environment, chainSelector uint64, contractType datastore.ContractType, version *semver.Version, qualifier string) (solana.PublicKey, error) {
	refs, err := env.DataStore.Addresses().Fetch()
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("fetch address refs: %w", err)
	}
	address, err := solcommon.PubkeyFromRefs(refs, datastore.NewAddressRefKey(chainSelector, contractType, version, qualifier))
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("resolve %s ref version %s qualifier %q on chain %d: %w", contractType, version, qualifier, chainSelector, err)
	}
	return address, nil
}

func initializerOutput[IN any](e cldf.Environment, input mcms.Input, report cldf_ops.Report[IN, sequences.OnChainOutput]) (cldf.ChangesetOutput, error) {
	return buildProposalOutput(e, input, []cldf_ops.Report[any, any]{report.ToGenericReport()}, report.Output.BatchOps)
}

// buildProposalOutput returns the reports, plus an MCMS proposal when the operations resulted in
// batch operations because the timelock owns the program.
func buildProposalOutput(e cldf.Environment, input mcms.Input, reports []cldf_ops.Report[any, any], batchOps []mcmstypes.BatchOperation) (cldf.ChangesetOutput, error) {
	if err := requireMCMSForBatchOps(e.Logger, input, batchOps); err != nil {
		return cldf.ChangesetOutput{}, err
	}
	return changesets.NewOutputBuilder(e, changesets.GetRegistry()).
		WithReports(reports).
		WithBatchOps(batchOps).
		Build(input)
}

// appendOutput adds an operation's report and batch operations to the accumulators.
func appendOutput[IN any](reports *[]cldf_ops.Report[any, any], batchOps *[]mcmstypes.BatchOperation, report cldf_ops.Report[IN, sequences.OnChainOutput]) {
	*reports = append(*reports, report.ToGenericReport())
	*batchOps = append(*batchOps, report.Output.BatchOps...)
}
