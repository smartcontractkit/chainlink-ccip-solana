package testreceiver

import (
	"context"
	"errors"
	"fmt"

	"github.com/Masterminds/semver/v3"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/sequences"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	cldf_deployment "github.com/smartcontractkit/chainlink-deployments-framework/deployment"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/example_ccip_receiver"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/example_ccip_receiver_v2"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
)

var ContractType cldf_deployment.ContractType = "TestReceiver"
var Version *semver.Version = solcommon.Version2_0_0
var ProgramName = "example_ccip_receiver_v2"

// Legacy (v1) receiver identity. Same contract type, distinguished by version, so both receivers
// can be registered for the same chain and qualifier.
var LegacyVersion *semver.Version = semver.MustParse("1.0.0")
var LegacyProgramName = "example_ccip_receiver"

type DeployParams struct {
	ExistingAddresses []datastore.AddressRef
	Qualifier         string
}

var Deploy = operations.NewOperation(
	"receiver:deploy",
	Version,
	"Deploys the Receiver program",
	func(b operations.Bundle, chain cldf_solana.Chain, input DeployParams) (datastore.AddressRef, error) {
		return solcommon.DeployContractIfNotInDatastore(
			b,
			chain,
			input.ExistingAddresses,
			ContractType,
			Version,
			input.Qualifier,
			ProgramName)
	},
)

var DeployLegacy = operations.NewOperation(
	"receiver:deploy-legacy",
	LegacyVersion,
	"Deploys the legacy (v1) Receiver program",
	func(b operations.Bundle, chain cldf_solana.Chain, input DeployParams) (datastore.AddressRef, error) {
		return solcommon.DeployContractIfNotInDatastore(
			b,
			chain,
			input.ExistingAddresses,
			ContractType,
			LegacyVersion,
			input.Qualifier,
			LegacyProgramName)
	},
)

var Upgrade = operations.NewOperation(
	"receiver:upgrade",
	Version,
	"Upgrades the default Receiver program",
	func(b operations.Bundle, chain cldf_solana.Chain, programID solana.PublicKey) ([]solana.Instruction, error) {
		return solcommon.UpgradeProgram(
			b,
			chain,
			ContractType,
			programID,
			ProgramName,
		)
	},
)

var UpgradeLegacy = operations.NewOperation(
	"receiver:upgrade-legacy",
	LegacyVersion,
	"Upgrades the legacy (v1) Receiver program",
	func(b operations.Bundle, chain cldf_solana.Chain, programID solana.PublicKey) ([]solana.Instruction, error) {
		return solcommon.UpgradeProgram(
			b,
			chain,
			ContractType,
			programID,
			LegacyProgramName,
		)
	},
)

var Initialize = operations.NewOperation(
	"receiver:initialize",
	Version,
	"Initializes the Receiver contract (v2, or v1 when Legacy is set)",
	func(b operations.Bundle, chain cldf_solana.Chain, input Params) (sequences.OnChainOutput, error) {
		legacy, err := IsLegacy(input.ReceiverVersion)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		// Only the program's upgrade authority may initialize it.
		programData, err := solcommon.GetSolProgramData(chain.Client, input.Receiver)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to get program data: %w", err)
		}
		authority, err := solcommon.InitializerAuthority(chain, input.Receiver)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		// legacy receiver
		if legacy {
			solcommon.SetProgramID(&example_ccip_receiver.ProgramID, example_ccip_receiver.ParseInstructionWithoutAccounts, input.Receiver)
			v1ReceiverStatePDA, _, initErr := solana.FindProgramAddress([][]byte{[]byte("state")}, input.Receiver)
			if initErr != nil {
				return sequences.OnChainOutput{}, fmt.Errorf("derive receiver state PDA: %w", initErr)
			}
			v1ReceiverTokenAdminPDA, _, initErr := solana.FindProgramAddress([][]byte{[]byte("receiver_token_admin")}, input.Receiver)
			if initErr != nil {
				return sequences.OnChainOutput{}, fmt.Errorf("derive receiver token admin PDA: %w", initErr)
			}
			initIx, initErr := example_ccip_receiver.NewInitializeInstruction(
				input.Router,
				v1ReceiverStatePDA,
				v1ReceiverTokenAdminPDA,
				authority,
				solana.SystemProgramID,
				input.Receiver,
				programData.Address,
			)
			if initErr != nil {
				return sequences.OnChainOutput{}, fmt.Errorf("failed to build initialize instruction: %w", initErr)
			}
			return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{initIx}, input.Receiver, ContractType.String())
		}
		// receiver v2
		solcommon.SetProgramID(&example_ccip_receiver_v2.ProgramID, example_ccip_receiver_v2.ParseInstructionWithoutAccounts, input.Receiver)
		configPDA, _, initErr := solana.FindProgramAddress([][]byte{[]byte("config")}, input.Receiver)
		if initErr != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("derive receiver config PDA: %w", initErr)
		}
		initIx, initErr := example_ccip_receiver_v2.NewInitializeInstruction(
			input.Router,
			configPDA,
			authority,
			solana.SystemProgramID,
			input.Receiver,
			programData.Address,
		)
		if initErr != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to build initialize instruction: %w", initErr)
		}
		return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{initIx}, input.Receiver, ContractType.String())
	},
)

type Params struct {
	Router   solana.PublicKey
	Receiver solana.PublicKey
	// ReceiverVersion selects the interface to call, and matches the version the receiver is
	// registered under in the datastore. It cannot be inferred from Receiver, because a deployed v1
	// receiver does not sit at the gobinding's declare_id! address outside devenv. Nil means the
	// current version.
	ReceiverVersion *semver.Version
}

type ApproveSenderParams struct {
	Receiver            solana.PublicKey
	SourceChainSelector uint64
	// RemoteSender is a hex address, with or without the 0x prefix. Kept as a string so the
	// execution report shows the address rather than base64-encoded bytes; it is padded to 32
	// bytes here, the form the receiver stores it under.
	RemoteSender string
	// Unpadded keeps RemoteSender at its own length instead of padding it to 32 bytes. It must match
	// the sender bytes in the message: raw 20 bytes on 1.6 lanes, already 32 on 2.0 lanes.
	Unpadded bool
	// ReceiverVersion selects the interface to call. See Params.ReceiverVersion.
	ReceiverVersion *semver.Version
}

// IsLegacy reports whether a receiver version uses the v1 interface. Nil means the current version.
// Only the major component is compared, so patch releases of either line resolve correctly.
func IsLegacy(receiverVersion *semver.Version) (bool, error) {
	if receiverVersion == nil {
		return false, nil
	}
	switch receiverVersion.Major() {
	case LegacyVersion.Major():
		return true, nil
	case Version.Major():
		return false, nil
	default:
		return false, fmt.Errorf("unsupported receiver version %s, expected %s or %s", receiverVersion, LegacyVersion, Version)
	}
}

var ApproveSender = operations.NewOperation(
	"receiver:approve-sender",
	Version,
	"Approves a remote sender on the Receiver contract (v2, or v1 when Legacy is set)",
	func(b operations.Bundle, chain cldf_solana.Chain, input ApproveSenderParams) (sequences.OnChainOutput, error) {
		legacy, legacyErr := IsLegacy(input.ReceiverVersion)
		if legacyErr != nil {
			return sequences.OnChainOutput{}, legacyErr
		}
		decode := solcommon.DecodeRemoteSender
		if input.Unpadded {
			decode = solcommon.DecodeRemoteSenderUnpadded
		}
		remoteSender, legacyErr := decode(input.RemoteSender)
		if legacyErr != nil {
			return sequences.OnChainOutput{}, legacyErr
		}
		authority, err := receiverOwner(b.GetContext(), chain, input.Receiver, legacy)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		// legacy receiver
		if legacy {
			solcommon.SetProgramID(&example_ccip_receiver.ProgramID, example_ccip_receiver.ParseInstructionWithoutAccounts, input.Receiver)
			v1StatePDA, _, v1Err := state.FindStateReceiverPDA(input.Receiver)
			if v1Err != nil {
				return sequences.OnChainOutput{}, fmt.Errorf("derive receiver state PDA: %w", v1Err)
			}
			v1ApprovedSenderPDA, v1Err := state.FindApprovedSender(input.SourceChainSelector, remoteSender, input.Receiver)
			if v1Err != nil {
				return sequences.OnChainOutput{}, fmt.Errorf("derive approved sender PDA: %w", v1Err)
			}
			v1Ix, v1Err := example_ccip_receiver.NewApproveSenderInstruction(
				input.SourceChainSelector,
				remoteSender,
				v1StatePDA,
				v1ApprovedSenderPDA,
				authority,
				solana.SystemProgramID,
			)
			if v1Err != nil {
				return sequences.OnChainOutput{}, fmt.Errorf("build approve sender instruction: %w", v1Err)
			}
			return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{v1Ix}, input.Receiver, ContractType.String())
		}
		// receiver v2
		solcommon.SetProgramID(&example_ccip_receiver_v2.ProgramID, example_ccip_receiver_v2.ParseInstructionWithoutAccounts, input.Receiver)
		configPDA, _, err := state.FindConfigReceiverPDA(input.Receiver)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("derive receiver config PDA: %w", err)
		}
		approvedSenderPDA, err := state.FindApprovedSenderV2(input.SourceChainSelector, remoteSender, input.Receiver)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("derive approved sender PDA: %w", err)
		}
		ix, err := example_ccip_receiver_v2.NewApproveSenderInstruction(
			example_ccip_receiver_v2.SenderParams{
				ChainSelector: input.SourceChainSelector,
				RemoteSender:  remoteSender,
			},
			configPDA,
			approvedSenderPDA,
			authority,
			solana.SystemProgramID,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("build approve sender instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{ix}, input.Receiver, ContractType.String())
	},
)

// receiverOwner returns the owner of the receiver's state, the only key that may change its
// config. It is the deployer until the state exists.
func receiverOwner(ctx context.Context, chain cldf_solana.Chain, receiver solana.PublicKey, legacy bool) (solana.PublicKey, error) {
	if legacy {
		statePDA, _, err := state.FindStateReceiverPDA(receiver)
		if err != nil {
			return solana.PublicKey{}, fmt.Errorf("derive receiver state PDA: %w", err)
		}
		return solcommon.ConfigAuthority(ctx, chain, receiver, statePDA, example_ccip_receiver.ParseAccount_BaseState,
			func(s *example_ccip_receiver.BaseState) solana.PublicKey { return s.Owner })
	}
	configPDA, _, err := state.FindConfigReceiverPDA(receiver)
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("derive receiver config PDA: %w", err)
	}
	return solcommon.ConfigAuthority(ctx, chain, receiver, configPDA, example_ccip_receiver_v2.ParseAccount_Config,
		func(c *example_ccip_receiver_v2.Config) solana.PublicKey { return c.Owner })
}

type RegisterWithRouterParams struct {
	Router   solana.PublicKey
	Receiver solana.PublicKey
}

var RegisterWithRouter = operations.NewOperation(
	"receiver:register-with-router",
	Version,
	"Registers the Receiver contract with its configured Router",
	func(b operations.Bundle, chain cldf_solana.Chain, input RegisterWithRouterParams) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&example_ccip_receiver_v2.ProgramID, example_ccip_receiver_v2.ParseInstructionWithoutAccounts, input.Receiver)
		configPDA, _, err := state.FindConfigReceiverPDA(input.Receiver)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("derive receiver config PDA: %w", err)
		}
		registrationSignerPDA, _, err := state.FindReceiverRegistrationSignerPDA(input.Receiver, input.Router)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("derive receiver registration signer PDA: %w", err)
		}
		receiverRegistryPDA, _, err := state.FindReceiverRegistryPDA(input.Receiver, input.Router)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("derive receiver registry PDA: %w", err)
		}
		routerConfigPDA, _, err := state.FindConfigPDA(input.Router)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("derive router config PDA: %w", err)
		}
		routerEventAuthorityPDA, _, err := state.FindEventAuthorityPDA(input.Router)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("derive router event authority PDA: %w", err)
		}
		_, regErr := chain.Client.GetAccountInfoWithOpts(b.GetContext(), receiverRegistryPDA, &rpc.GetAccountInfoOpts{
			Commitment: cldf_solana.SolDefaultCommitment,
		})
		switch {
		case regErr == nil:
			b.Logger.Infof("Receiver %s is already registered with router %s, skipping", input.Receiver, input.Router)
			return sequences.OnChainOutput{}, nil
		case !errors.Is(regErr, rpc.ErrNotFound):
			return sequences.OnChainOutput{}, fmt.Errorf("read receiver registry %s: %w", receiverRegistryPDA, regErr)
		}
		// The owner signs and pays the router's registry rent.
		authority, err := receiverOwner(b.GetContext(), chain, input.Receiver, false)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		ix, err := example_ccip_receiver_v2.NewRegisterWithRouterInstruction(
			configPDA,
			authority,
			registrationSignerPDA,
			input.Router,
			receiverRegistryPDA,
			routerConfigPDA,
			routerEventAuthorityPDA,
			solana.SystemProgramID,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("build register with router instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{ix}, input.Receiver, ContractType.String())
	},
)

type SetCCVConfigParams struct {
	Receiver              solana.PublicKey
	SourceChainSelector   uint64
	RequiredCCVs          []solana.PublicKey
	OptionalCCVs          []solana.PublicKey
	OptionalThreshold     uint8
	AllowedFinalityConfig example_ccip_receiver_v2.FinalityConfig
}

var SetCCVConfig = operations.NewOperation(
	"receiver:set-ccv-config",
	Version,
	"Configures CCV and finality requirements for a Receiver source chain",
	func(b operations.Bundle, chain cldf_solana.Chain, input SetCCVConfigParams) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&example_ccip_receiver_v2.ProgramID, example_ccip_receiver_v2.ParseInstructionWithoutAccounts, input.Receiver)
		configPDA, _, err := state.FindConfigReceiverPDA(input.Receiver)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("derive receiver config PDA: %w", err)
		}
		sourceChainCCVConfigPDA, _, err := state.FindReceiverCCVConfigPDA(input.SourceChainSelector, input.Receiver)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("derive receiver CCV config PDA: %w", err)
		}
		eventAuthorityPDA, _, err := state.FindEventAuthorityPDA(input.Receiver)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("derive receiver event authority PDA: %w", err)
		}
		authority, err := receiverOwner(b.GetContext(), chain, input.Receiver, false)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		ix, err := example_ccip_receiver_v2.NewSetCcvConfigInstruction(
			example_ccip_receiver_v2.SetCcvConfigParams{
				SourceChainSelector:   input.SourceChainSelector,
				RequiredCcvs:          input.RequiredCCVs,
				OptionalCcvs:          input.OptionalCCVs,
				OptionalThreshold:     input.OptionalThreshold,
				AllowedFinalityConfig: input.AllowedFinalityConfig,
			},
			configPDA,
			sourceChainCCVConfigPDA,
			authority,
			solana.SystemProgramID,
			eventAuthorityPDA,
			input.Receiver,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("build set CCV config instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{ix}, input.Receiver, ContractType.String())
	},
)
