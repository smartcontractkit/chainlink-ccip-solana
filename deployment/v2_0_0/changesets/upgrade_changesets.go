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
	refVersion, err := programRefVersion(input.ProgramType, input.Version, programsPath(e, input.ChainSelector))
	if err != nil {
		return cldf.ChangesetOutput{}, err
	}
	change, err := upgradedProgramRefForInput(addresses, input, refVersion)
	if err != nil {
		return cldf.ChangesetOutput{}, err
	}
	if change.add != nil {
		// The sequence resolves the program by this ref; it is only returned after a successful upgrade.
		addresses = append(addresses, *change.add)
	}
	additional := additionalRefsForUpgrade(addresses, input, refVersion)
	if e.Logger != nil {
		for _, existing := range additional.Conflicting {
			e.Logger.Warnw("Not registering an additional ref of the upgraded program: it already points at another program",
				"type", existing.Type, "qualifier", existing.Qualifier, "existingAddress", existing.Address)
		}
	}
	report, err := cldf_ops.ExecuteSequence(e.OperationsBundle, sequences.UpgradeProgram, e.BlockChains, sequences.UpgradeGenericInput{
		ChainSelector:     input.ChainSelector,
		ExistingAddresses: addresses,
		ProgramType:       input.ProgramType,
		MCMSQualifier:     input.MCMS.Qualifier,
		Qualifier:         input.Qualifier,
		Version:           input.Version,
		RefVersion:        refVersion,
	})
	if err != nil {
		return cldf.ChangesetOutput{}, fmt.Errorf("upgrade %s: %w", input.ProgramType, err)
	}
	ds := datastore.NewMemoryDataStore()
	if change.add != nil {
		if err := ds.Addresses().Add(*change.add); err != nil {
			return cldf.ChangesetOutput{Reports: report.ExecutionReports}, fmt.Errorf("add upgraded address ref %s %s: %w", change.add.Type, change.add.Version, err)
		}
	}
	// The version is part of the ref key, so replacing a ref is an add plus a delete of the old key, which
	// only exists in the environment's store (hence RemoteDelete).
	for _, old := range append(append([]datastore.AddressRef{}, change.superseded...), additional.Superseded...) {
		oldKey := datastore.NewAddressRefKey(old.ChainSelector, old.Type, old.Version, old.Qualifier)
		if err := ds.Addresses().RemoteDelete(oldKey); err != nil {
			return cldf.ChangesetOutput{Reports: report.ExecutionReports}, fmt.Errorf("delete superseded address ref %s %s: %w", old.Type, old.Version, err)
		}
	}
	for _, ref := range additional.Add {
		if err := ds.Addresses().Add(ref); err != nil {
			return cldf.ChangesetOutput{Reports: report.ExecutionReports}, fmt.Errorf("add additional address ref %s %s: %w", ref.Type, ref.Qualifier, err)
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
	refVersion, err := programRefVersion(input.ProgramType, input.Version, programsPath(e, input.ChainSelector))
	if err != nil {
		return cldf.ChangesetOutput{}, err
	}
	report, err := cldf_ops.ExecuteSequence(e.OperationsBundle, sequences.VerifyBuild, e.BlockChains, sequences.UpgradeGenericInput{
		ChainSelector:      input.ChainSelector,
		ExistingAddresses:  addresses,
		ProgramType:        input.ProgramType,
		MCMSQualifier:      input.MCMS.Qualifier,
		RemoteVerification: input.RemoteVerification,
		Qualifier:          input.Qualifier,
		Version:            input.Version,
		RefVersion:         refVersion,
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

// refChange is what an upgrade changes in the address book for the program's own ref.
type refChange struct {
	// add is the ref of the version being deployed; nil when it is already there.
	add *datastore.AddressRef
	// superseded are the same-major refs that add replaces. Refs of other majors are kept: a program
	// upgraded in place from 1.x keeps its 1.x ref, which other integrations still resolve.
	superseded []datastore.AddressRef
}

// upgradedProgramRefForInput plans the program's own ref for the version being deployed:
//   - a ref of that version exists: no change, except that stale refs of the same major are removed.
//   - a ref of the same major exists (2.0.0 when deploying 2.0.1): it is replaced, so only the new version remains.
//   - one address across majors: the ref of the other major (1.6.0) stays and the target version is added at its address.
//   - programPerVersion programs: refs of other majors are other programs and are left alone.
//   - no ref: the program is registered at input.ProgramAddress.
func upgradedProgramRefForInput(refs []datastore.AddressRef, input UpgradeProgramChangesetInput, version *semver.Version) (refChange, error) {
	info, supported := sequences.ProgramRefInfoFor(input.ProgramType)
	if !supported {
		return refChange{}, nil
	}
	contractType := info.ContractType
	programPerVersion := info.ProgramPerVersion
	targetQualifier := info.DefaultQualifier
	if input.Qualifier != "" {
		targetQualifier = input.Qualifier
	}

	var (
		current    *datastore.AddressRef
		sameMajor  []datastore.AddressRef
		otherMajor []datastore.AddressRef
	)
	for i := range refs {
		ref := &refs[i]
		if ref.ChainSelector != input.ChainSelector || ref.Type != contractType ||
			ref.Qualifier != targetQualifier || ref.Version == nil {
			continue
		}
		switch {
		case ref.Version.Equal(version):
			current = ref
		case ref.Version.Major() == version.Major():
			sameMajor = append(sameMajor, *ref)
		default:
			otherMajor = append(otherMajor, *ref)
		}
	}

	if current != nil {
		if input.ProgramAddress != "" && current.Address != input.ProgramAddress {
			return refChange{}, fmt.Errorf(
				"%s %s ref %s differs from requested programAddress %s",
				contractType, version, current.Address, input.ProgramAddress)
		}
		if !programPerVersion {
			for _, other := range append(append([]datastore.AddressRef{}, sameMajor...), otherMajor...) {
				if other.Address != current.Address {
					return refChange{}, fmt.Errorf(
						"%s %s ref %s differs from the %s ref %s of the same program",
						contractType, version, current.Address, other.Version, other.Address)
				}
			}
		}
		return refChange{superseded: sameMajor}, nil
	}

	if len(sameMajor) > 0 {
		if len(sameMajor) > 1 {
			return refChange{}, fmt.Errorf(
				"multiple %s refs of major %d on chain %d for qualifier %q: cannot tell which one is deployed",
				contractType, version.Major(), input.ChainSelector, targetQualifier)
		}
		old := sameMajor[0]
		if input.ProgramAddress != "" && old.Address != input.ProgramAddress {
			return refChange{}, fmt.Errorf(
				"%s %s ref %s differs from requested programAddress %s",
				contractType, old.Version, old.Address, input.ProgramAddress)
		}
		return refChange{add: refAt(old, input.ChainSelector, contractType, targetQualifier, version), superseded: sameMajor}, nil
	}

	if !programPerVersion && len(otherMajor) > 0 {
		source := otherMajor[0]
		for _, other := range otherMajor[1:] {
			if other.Address != source.Address {
				return refChange{}, fmt.Errorf(
					"multiple %s refs of other versions on chain %d for qualifier %q: cannot tell which one is deployed",
					contractType, input.ChainSelector, targetQualifier)
			}
			if other.Version.GreaterThan(source.Version) {
				source = other
			}
		}
		if input.ProgramAddress != "" && source.Address != input.ProgramAddress {
			return refChange{}, fmt.Errorf(
				"%s %s ref %s differs from requested programAddress %s",
				contractType, source.Version, source.Address, input.ProgramAddress)
		}
		return refChange{add: refAt(source, input.ChainSelector, contractType, targetQualifier, version)}, nil
	}

	if input.ProgramAddress == "" {
		return refChange{}, fmt.Errorf(
			"no %s %s ref on chain %d for qualifier %q: set programAddress to register the deployed program",
			contractType, version, input.ChainSelector, targetQualifier)
	}
	programID, err := solana.PublicKeyFromBase58(input.ProgramAddress)
	if err != nil {
		return refChange{}, fmt.Errorf("invalid programAddress %q: %w", input.ProgramAddress, err)
	}
	return refChange{add: &datastore.AddressRef{
		Address:       programID.String(),
		ChainSelector: input.ChainSelector,
		Type:          contractType,
		Version:       version,
		Qualifier:     targetQualifier,
	}}, nil
}

// refAt is a ref of version at the address of from, keeping its labels.
func refAt(from datastore.AddressRef, chainSelector uint64, contractType datastore.ContractType, qualifier string, version *semver.Version) *datastore.AddressRef {
	return &datastore.AddressRef{
		Address:       from.Address,
		ChainSelector: chainSelector,
		Labels:        from.Labels,
		Qualifier:     qualifier,
		Type:          contractType,
		Version:       version,
	}
}

// additionalRefsForUpgrade returns the refs saved besides the program's own. refs must hold the ref of
// version, which is where the program address comes from.
func additionalRefsForUpgrade(refs []datastore.AddressRef, input UpgradeProgramChangesetInput, version *semver.Version) sequences.AdditionalRefs {
	info, supported := sequences.ProgramRefInfoFor(input.ProgramType)
	if !supported {
		return sequences.AdditionalRefs{}
	}
	instanceQualifier := info.DefaultQualifier
	if input.Qualifier != "" {
		instanceQualifier = input.Qualifier
	}
	var address string
	for _, r := range refs {
		if r.ChainSelector == input.ChainSelector && r.Type == info.ContractType &&
			r.Qualifier == instanceQualifier && r.Version != nil && r.Version.Equal(version) {
			address = r.Address
			break
		}
	}
	return sequences.UpgradeAdditionalRefs(input.ProgramType, refs, input.ChainSelector, version, address)
}

// programsPath is where the chain's program artifacts sit, or "" when the chain is not a Solana chain.
func programsPath(e cldf.Environment, chainSelector uint64) string {
	if chain, ok := e.BlockChains.SolanaChains()[chainSelector]; ok {
		return chain.ProgramsPath
	}
	return ""
}

// programRefVersion is the ref version after the upgrade: the release's version, or for a local build
// the version its IDL declares (the registry default without an IDL).
func programRefVersion(programType sequences.ProgramUpgradeType, pipelineVersion, programsPath string) (*semver.Version, error) {
	info, supported := sequences.ProgramRefInfoFor(programType)
	if !supported {
		return nil, nil
	}
	release, local, err := solcommon.ResolveArtifactSource(pipelineVersion)
	if err != nil {
		return nil, err
	}
	if !local {
		return solcommon.ReleaseVersion(release)
	}
	if programsPath != "" {
		if artifact, inspectErr := solcommon.InspectArtifact(programsPath, info.ProgramName); inspectErr == nil && artifact.IDLPath != "" {
			idlVersion, idlErr := solcommon.IDLVersion(artifact.IDLPath)
			if idlErr != nil {
				return nil, idlErr
			}
			if !solcommon.RefVersionMatches(idlVersion, info.Version) {
				return nil, fmt.Errorf("refusing to upgrade %s: its IDL declares version %s, outside the %d.x line of this program",
					programType, idlVersion, info.Version.Major())
			}
			return idlVersion, nil
		}
	}
	return info.Version, nil
}
