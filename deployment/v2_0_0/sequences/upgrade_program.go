package sequences

import (
	"context"
	"errors"
	"fmt"

	"github.com/Masterminds/semver/v3"
	"github.com/gagliardetto/solana-go"

	common_utils "github.com/smartcontractkit/chainlink-ccip/deployment/utils"
	seq_core "github.com/smartcontractkit/chainlink-ccip/deployment/utils/sequences"
	cldf_chain "github.com/smartcontractkit/chainlink-deployments-framework/chain"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	cldf_ops "github.com/smartcontractkit/chainlink-deployments-framework/operations"
	"github.com/smartcontractkit/mcms/types"

	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	cctpverifierops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/cctpverifier"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/committeeverifier"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/executor"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/feequoter"
	lvops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/lombardverifier"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/offramp"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/rmnremote"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/router"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/testreceiver"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/testsender"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/tokenpools"
)

type ProgramUpgradeType string

// IsSupportedProgramUpgradeType reports whether the sequence has an upgrade
// implementation registered for the given program type.
func IsSupportedProgramUpgradeType(programType ProgramUpgradeType) bool {
	_, ok := programRegistry[programType]
	return ok
}

const (
	UpgradeTypeOffRamp              ProgramUpgradeType = "OffRamp"
	UpgradeTypeRouter               ProgramUpgradeType = "Router"
	UpgradeTypeFeeQuoter            ProgramUpgradeType = "FeeQuoter"
	UpgradeTypeExecutor             ProgramUpgradeType = "Executor"
	UpgradeTypeCommitteeVerifier    ProgramUpgradeType = "CommitteeVerifier"
	UpgradeTypeRMNRemote            ProgramUpgradeType = "RMNRemote"
	UpgradeTypeBurnMintTokenPool    ProgramUpgradeType = "BurnMintTokenPool"
	UpgradeTypeLockReleaseTokenPool ProgramUpgradeType = "LockReleaseTokenPool"
	UpgradeTypeCCTPTokenPool        ProgramUpgradeType = "CCTPTokenPool"
	UpgradeTypeLombardTokenPool     ProgramUpgradeType = "LombardTokenPool"
	UpgradeTypeTestReceiverV2       ProgramUpgradeType = "TestReceiverV2"
	UpgradeTypeTestReceiverLegacy   ProgramUpgradeType = "TestReceiverLegacy"
	UpgradeTypeTestSenderV2         ProgramUpgradeType = "TestSenderV2"
	UpgradeTypeTestSenderLegacy     ProgramUpgradeType = "TestSenderLegacy"
)

type programEntry struct {
	contractType datastore.ContractType
	version      *semver.Version
	programName  string
	qualifier    string
	upgrade      *cldf_ops.Operation[solana.PublicKey, []solana.Instruction, cldf_solana.Chain]
	// additionalRefs are registered by an in-place upgrade, as a fresh deploy does.
	additionalRefs []additionalRef
	// programPerVersion marks programs where each version is its own program with its own ref (OffRamp,
	// test sender/receiver). Others keep one address across versions, so an upgrade updates the ref version.
	programPerVersion bool
}

// ProgramRefInfo is how the datastore refs of a program type are keyed.
type ProgramRefInfo struct {
	ProgramName       string
	ContractType      datastore.ContractType
	Version           *semver.Version
	DefaultQualifier  string
	ProgramPerVersion bool
}

// ProgramRefInfoFor returns false for a type without an upgrade implementation.
func ProgramRefInfoFor(programType ProgramUpgradeType) (ProgramRefInfo, bool) {
	entry, ok := programRegistry[programType]
	if !ok {
		return ProgramRefInfo{}, false
	}
	return ProgramRefInfo{
		ProgramName:       entry.programName,
		ContractType:      entry.contractType,
		Version:           entry.version,
		DefaultQualifier:  entry.qualifier,
		ProgramPerVersion: entry.programPerVersion,
	}, true
}

var programRegistry = map[ProgramUpgradeType]programEntry{
	UpgradeTypeOffRamp: {
		contractType:      datastore.ContractType(offramp.ContractType),
		version:           offramp.Version,
		programName:       offramp.ProgramName,
		upgrade:           offramp.Upgrade,
		programPerVersion: true,
	},
	UpgradeTypeRouter: {
		contractType: datastore.ContractType(router.ContractType),
		version:      router.Version,
		programName:  router.ProgramName,
		upgrade:      router.Upgrade,
	},
	UpgradeTypeFeeQuoter: {
		contractType: datastore.ContractType(feequoter.ContractType),
		version:      feequoter.Version,
		programName:  feequoter.ProgramName,
		upgrade:      feequoter.Upgrade,
	},
	UpgradeTypeExecutor: {
		contractType: datastore.ContractType(executor.ContractType),
		version:      executor.Version,
		programName:  executor.ProgramName,
		qualifier:    "default",
		upgrade:      executor.Upgrade,
	},
	UpgradeTypeCommitteeVerifier: {
		contractType: datastore.ContractType(committeeverifier.ContractType),
		version:      committeeverifier.Version,
		programName:  committeeverifier.ProgramName,
		qualifier:    committeeverifier.DeployQualifier,
		upgrade:      committeeverifier.Upgrade,
	},
	UpgradeTypeRMNRemote: {
		contractType: datastore.ContractType(rmnremote.ContractType),
		version:      rmnremote.Version,
		programName:  rmnremote.ProgramName,
		upgrade:      rmnremote.Upgrade,
	},
	UpgradeTypeBurnMintTokenPool: {
		contractType:   datastore.ContractType(common_utils.BurnMintTokenPool),
		version:        solcommon.Version2_0_0,
		programName:    tokenpools.BurnMintProgramName,
		qualifier:      solcommon.PoolProgramQualifier,
		upgrade:        tokenpools.UpgradeBurnMint,
		additionalRefs: []additionalRef{poolProgramAlias},
	},
	UpgradeTypeLockReleaseTokenPool: {
		contractType:   datastore.ContractType(common_utils.LockReleaseTokenPool),
		version:        solcommon.Version2_0_0,
		programName:    tokenpools.LockReleaseProgramName,
		qualifier:      solcommon.PoolProgramQualifier,
		upgrade:        tokenpools.UpgradeLockRelease,
		additionalRefs: []additionalRef{poolProgramAlias},
	},
	UpgradeTypeCCTPTokenPool: {
		contractType: datastore.ContractType(common_utils.CCTPTokenPool),
		version:      solcommon.Version2_0_0,
		programName:  tokenpools.CctpProgramName,
		qualifier:    solcommon.PoolProgramQualifier,
		upgrade:      tokenpools.UpgradeCctp,
		// The verifier is embedded in the pool program but looked up under its own type.
		additionalRefs: []additionalRef{
			poolProgramAlias,
			{contractType: datastore.ContractType(cctpverifierops.ContractType), version: cctpverifierops.Version, qualifier: string(cctpverifierops.ContractType)},
		},
	},
	UpgradeTypeLombardTokenPool: {
		contractType: datastore.ContractType(tokenpools.LombardTokenPool),
		version:      solcommon.Version2_0_0,
		programName:  tokenpools.LombardProgramName,
		qualifier:    solcommon.PoolProgramQualifier,
		upgrade:      tokenpools.UpgradeLombard,
		// The verifier is also registered under the EVM-canonical resolver qualifier.
		additionalRefs: []additionalRef{
			poolProgramAlias,
			{contractType: datastore.ContractType(lvops.ContractType), version: lvops.Version, qualifier: string(lvops.ContractType)},
			{contractType: datastore.ContractType(lvops.ContractType), version: lvops.Version, qualifier: lvops.LombardVerifierResolverQualifier},
		},
	},
	UpgradeTypeTestReceiverV2: {
		contractType:      datastore.ContractType(testreceiver.ContractType),
		version:           testreceiver.Version,
		programName:       testreceiver.ProgramName,
		upgrade:           testreceiver.Upgrade,
		programPerVersion: true,
	},
	UpgradeTypeTestReceiverLegacy: {
		contractType:      datastore.ContractType(testreceiver.ContractType),
		version:           testreceiver.LegacyVersion,
		programName:       testreceiver.LegacyProgramName,
		upgrade:           testreceiver.UpgradeLegacy,
		programPerVersion: true,
	},
	UpgradeTypeTestSenderV2: {
		contractType:      datastore.ContractType(testsender.ContractType),
		version:           testsender.Version,
		qualifier:         "default",
		programName:       testsender.ProgramName,
		upgrade:           testsender.Upgrade,
		programPerVersion: true,
	},
	UpgradeTypeTestSenderLegacy: {
		contractType:      datastore.ContractType(testsender.ContractType),
		version:           testsender.LegacyVersion,
		qualifier:         "default",
		programName:       testsender.LegacyProgramName,
		upgrade:           testsender.UpgradeLegacy,
		programPerVersion: true,
	},
}

type UpgradeGenericInput struct {
	ChainSelector      uint64
	ExistingAddresses  []datastore.AddressRef
	ProgramType        ProgramUpgradeType
	MCMSQualifier      string
	RemoteVerification bool
	Version            string
	// Qualifier overrides the registry qualifier when the target program was
	// deployed under a custom one, for example a staging sender.
	Qualifier string
	// RefVersion is the datastore version of the program ref. Nil uses the registry version.
	RefVersion *semver.Version
}

func resolveUpgrade(chains cldf_chain.BlockChains, input UpgradeGenericInput) (cldf_solana.Chain, programEntry, solana.PublicKey, error) {
	entry, ok := programRegistry[input.ProgramType]
	if !ok {
		return cldf_solana.Chain{}, programEntry{}, solana.PublicKey{}, fmt.Errorf("unknown program type: %s", input.ProgramType)
	}
	chain, ok := chains.SolanaChains()[input.ChainSelector]
	if !ok {
		return cldf_solana.Chain{}, programEntry{}, solana.PublicKey{}, fmt.Errorf("solana chain with selector %d not found", input.ChainSelector)
	}
	qualifier := entry.qualifier
	if input.Qualifier != "" {
		qualifier = input.Qualifier
	}
	refVersion := entry.version
	if input.RefVersion != nil {
		refVersion = input.RefVersion
	}
	programID, err := solcommon.PubkeyFromRefs(input.ExistingAddresses, datastore.NewAddressRefKey(
		input.ChainSelector, entry.contractType, refVersion, qualifier,
	))
	if err != nil {
		return cldf_solana.Chain{}, programEntry{}, solana.PublicKey{}, fmt.Errorf("resolve %s: %w", input.ProgramType, err)
	}
	return chain, entry, programID, nil
}

// verifyArtifact checks that the compiled program and its IDL were built for
// the program ID in the datastore. This catches an artifact built for another
// address or another SBPF version before spending SOL on a buffer deploy.
//
// The program keypair is not required: the buffer deploy uses the deployer key
// and the upgrade uses the upgrade authority. Partner-deployed programs such as
// the Lombard pool therefore upgrade without their keypair.
func verifyArtifact(chain cldf_solana.Chain, programName string, expectedID solana.PublicKey) error {
	info, err := solcommon.InspectArtifact(chain.ProgramsPath, programName)
	if err != nil {
		return err
	}
	if !solcommon.IsLocalValidator(chain.URL) && info.SBPFVersion != solcommon.ExpectedSBPFVersion {
		return fmt.Errorf("artifact for %s declares SBPF v%d, want v%d: rebuild with --arch v%d",
			programName, info.SBPFVersion, solcommon.ExpectedSBPFVersion, solcommon.ExpectedSBPFVersion)
	}
	return solcommon.RequireArtifactForID(chain.ProgramsPath, programName, expectedID)
}

var UpgradeProgram = cldf_ops.NewSequence(
	"upgrade-program",
	semver.MustParse("2.0.0"),
	"Upgrades a deployed Solana program by type",
	func(b cldf_ops.Bundle, chains cldf_chain.BlockChains, input UpgradeGenericInput) (seq_core.OnChainOutput, error) {
		chain, entry, programID, err := resolveUpgrade(chains, input)
		if err != nil {
			return seq_core.OnChainOutput{}, err
		}

		// An upgrade needs only the upgrade authority, never the program keypair.
		// On real networks the artifacts being upgraded to come from the explicit
		// artifact source; local validators run the local build.
		if err = stageUpgradeArtifacts(b.GetContext(), chain, entry.programName, input.Version); err != nil {
			return seq_core.OnChainOutput{}, err
		}
		if err = verifyArtifact(chain, entry.programName, programID); err != nil {
			return seq_core.OnChainOutput{}, err
		}

		if chain.DeployerKey == nil {
			return seq_core.OnChainOutput{}, errors.New("missing Solana deployer key")
		}

		authority, err := solcommon.GetUpgradeAuthority(chain.Client, programID)
		if err != nil {
			return seq_core.OnChainOutput{}, err
		}
		if authority.IsZero() {
			return seq_core.OnChainOutput{}, fmt.Errorf("program %s has no upgrade authority", programID)
		}
		timelock, timelockFound, err := solcommon.FindTimelockSignerPDA(input.ExistingAddresses, input.ChainSelector, input.MCMSQualifier)
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("resolve timelock signer for upgrade authority handoff: %w", err)
		}
		deployer := chain.DeployerKey.PublicKey()
		if err = checkUpgradeAuthority(authority, deployer, timelock, timelockFound, chainHasTimelock(input.ExistingAddresses, input.ChainSelector), input.MCMSQualifier); err != nil {
			return seq_core.OnChainOutput{}, err
		}

		report, err := cldf_ops.ExecuteOperation(b, entry.upgrade, chain, programID)
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("upgrade %s: %w", input.ProgramType, err)
		}
		// If the deployer holds the upgrade authority, execute the upgrade and publish
		// the IDL with that key, then hand the program upgrade authority to MCMS.
		if authority.Equals(deployer) {
			err = chain.Confirm(report.Output)
			if err != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("confirm upgrade %s: %w", input.ProgramType, err)
			}
			err = solcommon.VerifyUpgradedProgram(chain, programID, entry.programName)
			if err != nil {
				return seq_core.OnChainOutput{}, err
			}
			var idl []types.BatchOperation
			idl, err = solcommon.PublishIDL(b, chain, programID, entry.programName)
			if err != nil {
				return seq_core.OnChainOutput{}, err
			}
			if len(idl) != 0 {
				return seq_core.OnChainOutput{}, fmt.Errorf("upgrade authority changed during IDL publish for %s", programID)
			}
			// Without a timelock (an environment deployed without MCMS) the deployer stays the authority.
			if timelockFound && !timelock.Equals(deployer) {
				handoff := solcommon.SetUpgradeAuthority(programID, deployer, timelock, false)
				if err = chain.Confirm([]solana.Instruction{handoff}); err != nil {
					return seq_core.OnChainOutput{}, fmt.Errorf("transfer upgrade authority for %s to MCMS: %w", programID, err)
				}
				transferredAuthority, authorityErr := solcommon.GetUpgradeAuthority(chain.Client, programID)
				if authorityErr != nil {
					return seq_core.OnChainOutput{}, fmt.Errorf("verify upgrade authority transfer for %s: %w", programID, authorityErr)
				}
				if !transferredAuthority.Equals(timelock) {
					return seq_core.OnChainOutput{}, fmt.Errorf("upgrade authority transfer for %s left authority at %s, expected MCMS timelock %s", programID, transferredAuthority, timelock)
				}
			}
			return seq_core.OnChainOutput{}, nil
		}
		// timelock controlled upgrade: build and submit the batch operation to the timelock
		batch, err := solcommon.BuildMCMSBatchOperation(
			chain.Selector, report.Output, solana.BPFLoaderUpgradeableProgramID.String(), string(entry.contractType))
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("build upgrade batch: %w", err)
		}
		out := seq_core.OnChainOutput{BatchOps: []types.BatchOperation{batch}}
		idl, err := solcommon.PublishIDL(b, chain, programID, entry.programName)
		if err != nil {
			return seq_core.OnChainOutput{}, err
		}
		out.BatchOps = append(out.BatchOps, idl...)
		return out, nil
	},
)

var VerifyBuild = cldf_ops.NewSequence(
	"verify-build",
	semver.MustParse("2.0.0"),
	"Checks the upgrade and records its verified build",
	func(b cldf_ops.Bundle, chains cldf_chain.BlockChains, input UpgradeGenericInput) (seq_core.OnChainOutput, error) {
		chain, entry, programID, err := resolveUpgrade(chains, input)
		if err != nil {
			return seq_core.OnChainOutput{}, err
		}
		if input.RemoteVerification {
			var authority solana.PublicKey
			authority, err = solcommon.GetUpgradeAuthority(chain.Client, programID)
			if err != nil {
				return seq_core.OnChainOutput{}, err
			}
			var output string
			output, err = submitRemoteVerification(b.GetContext(), chain, programID, authority)
			if err != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("submit remote verification for %s: %s: %w", programID, output, err)
			}
			b.Logger.Infow("Remote verification job submitted", "program", programID, "output", output)
			return seq_core.OnChainOutput{}, nil
		}

		// The hash check and the build record both read the local artifact set, so
		// real networks stage the same source the upgrade used.
		if err = stageUpgradeArtifacts(b.GetContext(), chain, entry.programName, input.Version); err != nil {
			return seq_core.OnChainOutput{}, err
		}

		if chain.DeployerKey == nil {
			return seq_core.OnChainOutput{}, errors.New("missing Solana deployer key")
		}
		err = solcommon.VerifyUpgradedProgram(chain, programID, entry.programName)
		if err != nil {
			return seq_core.OnChainOutput{}, err
		}
		build, err := solcommon.RegisterBuild(b, chain, programID, entry.programName)
		if err != nil {
			return seq_core.OnChainOutput{}, err
		}
		if build != nil {
			timelock, err := solcommon.GetTimelockSignerPDA(input.ExistingAddresses, input.ChainSelector, input.MCMSQualifier)
			if err != nil {
				return seq_core.OnChainOutput{}, err
			}
			authority, err := solcommon.GetUpgradeAuthority(chain.Client, programID)
			if err != nil {
				return seq_core.OnChainOutput{}, err
			}
			if !authority.Equals(timelock) {
				return seq_core.OnChainOutput{}, fmt.Errorf("upgrade authority %s is not the timelock", authority)
			}
			return seq_core.OnChainOutput{BatchOps: []types.BatchOperation{*build}}, nil
		}
		return seq_core.OnChainOutput{}, nil
	},
)

func submitRemoteVerification(ctx context.Context, chain cldf_solana.Chain, programID, authority solana.PublicKey) (string, error) {
	return runCommand(ctx, "solana-verify", []string{
		"remote", "submit-job",
		"--url", chain.URL,
		"--uploader", authority.String(),
		"--program-id", programID.String(),
	}, chain.ProgramsPath)
}

// checkUpgradeAuthority rejects an upgrade whose authority this run cannot sign for, before anything is
// sent. The deployer signs directly, and the timelock signs through an MCMS proposal. The timelock is
// resolved from the MCMS qualifier, so an environment that has a timelock but did not get a qualifier that
// resolves it is rejected rather than treated as having no MCMS. An environment without any timelock only
// supports a deployer-owned program.
func checkUpgradeAuthority(authority, deployer, timelock solana.PublicKey, timelockFound, chainHasTimelock bool, qualifier string) error {
	switch {
	case authority.Equals(deployer):
		if !timelockFound && chainHasTimelock {
			return fmt.Errorf("no timelock found for MCMS qualifier %q: pass mcms.qualifier so the upgrade authority can be handed to it", qualifier)
		}
		return nil
	case timelockFound && authority.Equals(timelock):
		return nil
	case !timelockFound && chainHasTimelock:
		return fmt.Errorf("upgrade authority %s is not the deployer, and no timelock was found for MCMS qualifier %q: pass mcms.qualifier if the timelock owns the program", authority, qualifier)
	case !timelockFound:
		return fmt.Errorf("upgrade authority %s is not the deployer, and this environment has no MCMS", authority)
	default:
		return fmt.Errorf("upgrade authority %s is not the deployer or the timelock", authority)
	}
}

// chainHasTimelock reports whether the datastore has any timelock for the chain, whatever its qualifier.
func chainHasTimelock(refs []datastore.AddressRef, chainSelector uint64) bool {
	for _, ref := range refs {
		if ref.ChainSelector == chainSelector && ref.Type == datastore.ContractType(common_utils.RBACTimelock) {
			return true
		}
	}
	return false
}

// stageUpgradeArtifacts stages the release artifacts of a real-network upgrade. The pipeline version
// must equal the version the program's IDL declares.
func stageUpgradeArtifacts(ctx context.Context, chain cldf_solana.Chain, programName, pipelineVersion string) error {
	if solcommon.IsLocalValidator(chain.URL) {
		return nil
	}
	release, local, err := solcommon.ResolveArtifactSource(pipelineVersion)
	if err != nil {
		return err
	}
	if local {
		return nil
	}
	if err = stageReleaseArtifacts(ctx, chain.ProgramsPath, release); err != nil {
		return err
	}
	version, err := solcommon.ReleaseVersion(release)
	if err != nil {
		return err
	}
	return solcommon.RequireIDLVersionEqual(chain.ProgramsPath, programName, version)
}
