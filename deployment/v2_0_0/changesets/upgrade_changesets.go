package changesets

import (
	"errors"
	"fmt"

	"github.com/Masterminds/semver/v3"
	"github.com/gagliardetto/solana-go"

	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/changesets"
	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/mcms"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	cldf "github.com/smartcontractkit/chainlink-deployments-framework/deployment"
	cldf_ops "github.com/smartcontractkit/chainlink-deployments-framework/operations"

	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/sequences"
)

type UpgradeProgramChangesetInput struct {
	ChainSelector uint64 `json:"chainSelector"`
	// ProgramAddress supplies the already-deployed program ID for a first-time registration.
	// The matching private key is never part of pipeline input.
	ProgramAddress string                       `json:"programAddress,omitempty"`
	ProgramType    sequences.ProgramUpgradeType `json:"programType"`
	MCMS           mcms.Input                   `json:"mcms"`
	Version        string                       `json:"version,omitempty"`
	// Qualifier identifies the program instance being upgraded. It selects both
	// the existing v1 address ref and target v2 ref, preserving the program's
	// identity across versions. This is separate from MCMS.Qualifier, which
	// selects MCMS and timelock addresses.
	Qualifier string `json:"qualifier,omitempty"`
}

type VerifyBuildChangesetInput struct {
	ChainSelector      uint64                       `json:"chainSelector"`
	ProgramType        sequences.ProgramUpgradeType `json:"programType"`
	MCMS               mcms.Input                   `json:"mcms"`
	RemoteVerification bool                         `json:"remoteVerification"`
	Version            string                       `json:"version,omitempty"`
	// Qualifier targets a program deployed under a custom qualifier, for
	// example a staging sender. Empty uses the registry default.
	Qualifier string `json:"qualifier,omitempty"`
}

func UpgradeProgramChangeset() cldf.ChangeSetV2[UpgradeProgramChangesetInput] {
	return cldf.CreateChangeSet(upgradeProgramApply, upgradeProgramVerify)
}

func VerifyBuildChangeset() cldf.ChangeSetV2[VerifyBuildChangesetInput] {
	return cldf.CreateChangeSet(verifyBuildApply, verifyBuildVerify)
}

func verifyBuildVerify(env cldf.Environment, input VerifyBuildChangesetInput) error {
	return upgradeProgramVerify(env, UpgradeProgramChangesetInput{
		ChainSelector: input.ChainSelector,
		ProgramType:   input.ProgramType,
		MCMS:          input.MCMS,
		Version:       input.Version,
		Qualifier:     input.Qualifier,
	})
}

func upgradeProgramVerify(env cldf.Environment, input UpgradeProgramChangesetInput) error {
	if err := cldf.IsValidChainSelector(input.ChainSelector); err != nil {
		return fmt.Errorf("invalid chain selector %d: %w", input.ChainSelector, err)
	}
	if !env.BlockChains.Exists(input.ChainSelector) {
		return fmt.Errorf("chain with selector %d does not exist", input.ChainSelector)
	}
	if _, ok := env.BlockChains.SolanaChains()[input.ChainSelector]; !ok {
		return fmt.Errorf("chain with selector %d is not a Solana chain", input.ChainSelector)
	}
	if input.ProgramType == "" {
		return errors.New("programType is required")
	}
	if !sequences.IsSupportedProgramUpgradeType(input.ProgramType) {
		return fmt.Errorf("unsupported programType %q", input.ProgramType)
	}
	if input.ProgramAddress != "" {
		if _, err := solana.PublicKeyFromBase58(input.ProgramAddress); err != nil {
			return fmt.Errorf("invalid programAddress %q: %w", input.ProgramAddress, err)
		}
	}
	if input.ProgramAddress != "" {
		_, _, _, inPlace, _ := upgradeProgramRefDetails(input.ProgramType)
		if inPlace {
			return errors.New("programAddress must be omitted for in-place upgrades; the existing address is resolved from the legacy ref")
		}
	}
	if input.Version != "" && input.Version != solcommon.LocalArtifactsValue {
		if err := solcommon.ValidateReleaseTag(input.Version); err != nil {
			return fmt.Errorf("invalid version: %w", err)
		}
	}
	if input.MCMS != (mcms.Input{}) {
		mcmsInput := input.MCMS
		if err := mcmsInput.PopulateDefaults(); err != nil {
			return fmt.Errorf("failed to populate MCMS defaults: %w", err)
		}
		if err := mcmsInput.Validate(); err != nil {
			return fmt.Errorf("failed to validate MCMS input: %w", err)
		}
	}
	return nil
}

func upgradeProgramApply(e cldf.Environment, input UpgradeProgramChangesetInput) (cldf.ChangesetOutput, error) {
	addresses, err := e.DataStore.Addresses().Fetch()
	if err != nil {
		return cldf.ChangesetOutput{}, fmt.Errorf("fetch existing addresses: %w", err)
	}
	upgradedRef, addUpgradedRef, err := upgradedProgramRefForInput(addresses, input)
	if err != nil {
		return cldf.ChangesetOutput{}, err
	}
	if addUpgradedRef {
		// UpgradeProgram resolves the target release by its 2.0.0 ref. Make the
		// same-address ref available to the sequence, but only return it below
		// after direct execution or successful MCMS proposal construction.
		addresses = append(addresses, upgradedRef)
	}
	report, err := cldf_ops.ExecuteSequence(e.OperationsBundle, sequences.UpgradeProgram, e.BlockChains, sequences.UpgradeGenericInput{
		ChainSelector:     input.ChainSelector,
		ExistingAddresses: addresses,
		ProgramType:       input.ProgramType,
		MCMSQualifier:     input.MCMS.Qualifier,
		Qualifier:         input.Qualifier,
		Version:           input.Version,
	})
	if err != nil {
		return cldf.ChangesetOutput{}, fmt.Errorf("upgrade %s: %w", input.ProgramType, err)
	}
	ds := datastore.NewMemoryDataStore()
	if addUpgradedRef {
		if err := ds.Addresses().Add(upgradedRef); err != nil {
			return cldf.ChangesetOutput{Reports: report.ExecutionReports}, fmt.Errorf("add upgraded address ref %s %s: %w", upgradedRef.Type, upgradedRef.Version, err)
		}
	}
	for _, ref := range report.Output.Addresses {
		if err := ds.Addresses().Add(ref); err != nil {
			return cldf.ChangesetOutput{Reports: report.ExecutionReports}, fmt.Errorf("add upgraded address ref %s %s: %w", ref.Type, ref.Version, err)
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

func verifyBuildApply(e cldf.Environment, input VerifyBuildChangesetInput) (cldf.ChangesetOutput, error) {
	addresses, err := e.DataStore.Addresses().Fetch()
	if err != nil {
		return cldf.ChangesetOutput{}, fmt.Errorf("fetch existing addresses: %w", err)
	}
	report, err := cldf_ops.ExecuteSequence(e.OperationsBundle, sequences.VerifyBuild, e.BlockChains, sequences.UpgradeGenericInput{
		ChainSelector:      input.ChainSelector,
		ExistingAddresses:  addresses,
		ProgramType:        input.ProgramType,
		MCMSQualifier:      input.MCMS.Qualifier,
		RemoteVerification: input.RemoteVerification,
		Qualifier:          input.Qualifier,
		Version:            input.Version,
	})
	if err != nil {
		return cldf.ChangesetOutput{}, fmt.Errorf("verify build for %s: %w", input.ProgramType, err)
	}
	if err := requireMCMSForBatchOps(e.Logger, input.MCMS, report.Output.BatchOps); err != nil {
		return cldf.ChangesetOutput{}, err
	}
	return changesets.
		NewOutputBuilder(e, changesets.GetRegistry()).
		WithReports(append([]cldf_ops.Report[any, any](nil), report.ExecutionReports...)).
		WithBatchOps(report.Output.BatchOps).
		Build(input.MCMS)
}

// upgradedProgramRefForInput derives the target ref for an in-place upgrade or registers the
// supplied public key for a first deployment. The ref is available to the sequence for address
// resolution and returned after direct execution or successful MCMS proposal construction.
func upgradedProgramRefForInput(refs []datastore.AddressRef, input UpgradeProgramChangesetInput) (datastore.AddressRef, bool, error) {
	contractType, version, defaultQualifier, inPlace, supported := upgradeProgramRefDetails(input.ProgramType)
	if !supported {
		return datastore.AddressRef{}, false, nil
	}
	targetQualifier := defaultQualifier
	if input.Qualifier != "" {
		targetQualifier = input.Qualifier
	}

	var current *datastore.AddressRef
	for i := range refs {
		ref := &refs[i]
		if ref.ChainSelector == input.ChainSelector && ref.Type == contractType &&
			ref.Qualifier == targetQualifier && ref.Version != nil && ref.Version.Equal(version) {
			current = ref
			break
		}
	}
	if current != nil {
		if input.ProgramAddress != "" && current.Address != input.ProgramAddress {
			return datastore.AddressRef{}, false, fmt.Errorf(
				"%s %s ref %s differs from requested programAddress %s",
				contractType, version, current.Address, input.ProgramAddress)
		}
		if inPlace {
			legacy, err := matchingLegacyProgramRefs(refs, input.ChainSelector, contractType, targetQualifier)
			if err != nil {
				return datastore.AddressRef{}, false, err
			}
			if len(legacy) == 1 && current.Address != legacy[0].Address {
				return datastore.AddressRef{}, false, fmt.Errorf("%s %s ref %s differs from legacy ref %s",
					contractType, version, current.Address, legacy[0].Address)
			}
		}
		return datastore.AddressRef{}, false, nil
	}

	if inPlace {
		legacy, err := matchingLegacyProgramRefs(refs, input.ChainSelector, contractType, targetQualifier)
		if err != nil {
			return datastore.AddressRef{}, false, err
		}
		if len(legacy) == 0 {
			return datastore.AddressRef{}, false, fmt.Errorf(
				"no legacy %s ref on chain %d for qualifier %q to upgrade in place",
				contractType, input.ChainSelector, targetQualifier)
		}
		if len(legacy) != 1 {
			return datastore.AddressRef{}, false, fmt.Errorf(
				"multiple legacy %s refs on chain %d for qualifier %q",
				contractType, input.ChainSelector, targetQualifier)
		}
		return datastore.AddressRef{
			Address:       legacy[0].Address,
			ChainSelector: input.ChainSelector,
			Labels:        legacy[0].Labels,
			Qualifier:     targetQualifier,
			Type:          contractType,
			Version:       version,
		}, true, nil
	}

	if input.ProgramAddress == "" {
		// Existing-address upgrades still resolve from the datastore. A first deployment
		// must provide its public address so it can be registered in the changeset output.
		return datastore.AddressRef{}, false, nil
	}
	programID, err := solana.PublicKeyFromBase58(input.ProgramAddress)
	if err != nil {
		return datastore.AddressRef{}, false, fmt.Errorf("invalid programAddress %q: %w", input.ProgramAddress, err)
	}
	return datastore.AddressRef{
		Address:       programID.String(),
		ChainSelector: input.ChainSelector,
		Type:          contractType,
		Version:       version,
		Qualifier:     targetQualifier,
	}, true, nil
}

func matchingLegacyProgramRefs(
	refs []datastore.AddressRef,
	chainSelector uint64,
	contractType datastore.ContractType,
	qualifier string,
) ([]datastore.AddressRef, error) {
	var legacy []datastore.AddressRef
	for _, ref := range refs {
		if ref.ChainSelector == chainSelector && ref.Type == contractType &&
			ref.Qualifier == qualifier && ref.Version != nil && ref.Version.Major() == 1 {
			legacy = append(legacy, ref)
		}
	}
	if len(legacy) > 1 {
		return nil, fmt.Errorf("multiple legacy %s refs on chain %d for qualifier %q",
			contractType, chainSelector, qualifier)
	}
	return legacy, nil
}

func upgradeProgramRefDetails(
	programType sequences.ProgramUpgradeType,
) (datastore.ContractType, *semver.Version, string, bool, bool) {
	version := semver.MustParse("2.0.0")
	switch programType {
	case sequences.UpgradeTypeRouter:
		return "Router", version, "", true, true
	case sequences.UpgradeTypeFeeQuoter:
		return "FeeQuoter", version, "", true, true
	case sequences.UpgradeTypeRMNRemote:
		return "RMNRemote", version, "", true, true
	case sequences.UpgradeTypeBurnMintTokenPool:
		return "BurnMintTokenPool", version, solcommon.PoolProgramQualifier, true, true
	case sequences.UpgradeTypeLockReleaseTokenPool:
		return "LockReleaseTokenPool", version, solcommon.PoolProgramQualifier, true, true
	case sequences.UpgradeTypeCCTPTokenPool:
		return "CCTPTokenPool", version, solcommon.PoolProgramQualifier, true, true
	case sequences.UpgradeTypeLombardTokenPool:
		return "LombardTokenPool", version, solcommon.PoolProgramQualifier, true, true
	case sequences.UpgradeTypeOffRamp:
		return "OffRamp", version, "", false, true
	case sequences.UpgradeTypeExecutor:
		return "Executor", version, "default", false, true
	case sequences.UpgradeTypeCommitteeVerifier:
		return "CommitteeVerifier", version, "default", false, true
	case sequences.UpgradeTypeTestSenderLegacy:
		return "TestSender", semver.MustParse("1.0.0"), "default", false, true
	case sequences.UpgradeTypeTestSenderV2:
		return "TestSender", version, "default", false, true
	case sequences.UpgradeTypeTestReceiverLegacy:
		return "TestReceiver", semver.MustParse("1.0.0"), "", false, true
	case sequences.UpgradeTypeTestReceiverV2:
		return "TestReceiver", version, "", false, true
	default:
		return "", nil, "", false, false
	}
}
