package rmnremote

import (
	"context"
	"fmt"
	"slices"

	"github.com/Masterminds/semver/v3"
	"github.com/gagliardetto/solana-go"

	api "github.com/smartcontractkit/chainlink-ccip/deployment/fastcurse"
	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/sequences"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	cldf_deployment "github.com/smartcontractkit/chainlink-deployments-framework/deployment"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations"
	"github.com/smartcontractkit/mcms/types"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/rmn_remote"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
)

var (
	ContractType cldf_deployment.ContractType = "RMNRemote"
	ProgramName                               = "rmn_remote"
	Version      *semver.Version              = solcommon.Version2_0_0
)

type CurseInput struct {
	Subjects           []api.Subject
	RMNRemoteCursePDA  solana.PublicKey
	RMNRemoteConfigPDA solana.PublicKey
	RMNRemote          solana.PublicKey
}

var Deploy = operations.NewOperation(
	"rmn-remote:deploy",
	Version,
	"Deploys the RMNRemote program",
	func(b operations.Bundle, chain cldf_solana.Chain, input []datastore.AddressRef) (datastore.AddressRef, error) {
		return solcommon.DeployContractIfNotInDatastore(
			b,
			chain,
			input,
			ContractType,
			Version,
			"",
			ProgramName)
	},
)

var Upgrade = operations.NewOperation(
	"rmn-remote:upgrade",
	Version,
	"Upgrades the default RMNRemote program",
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

var Initialize = operations.NewOperation(
	"rmn-remote:initialize",
	Version,
	"Initializes the RMNRemote program",
	func(b operations.Bundle, chain cldf_solana.Chain, input Params) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&rmn_remote.ProgramID, rmn_remote.ParseInstructionWithoutAccounts, input.RMNRemote)
		programData, err := solcommon.GetSolProgramData(chain.Client, input.RMNRemote)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to get program data: %w", err)
		}
		authority, err := GetAuthority(b.GetContext(), chain, input.RMNRemote)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		rmnRemoteConfigPDA, _, _ := state.FindRMNRemoteConfigPDA(input.RMNRemote)
		rmnRemoteCursesPDA, _, _ := state.FindRMNRemoteCursesPDA(input.RMNRemote)
		instruction, err := rmn_remote.NewInitializeInstruction(
			rmnRemoteConfigPDA,
			rmnRemoteCursesPDA,
			authority,
			solana.SystemProgramID,
			input.RMNRemote,
			programData.Address,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to build initialize instruction: %w", err)
		}
		err = chain.Confirm([]solana.Instruction{instruction})
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to confirm initialization: %w", err)
		}
		return sequences.OnChainOutput{}, nil
	},
)

var TransferOwnership = operations.NewOperation(
	"rmn-remote:transfer-ownership",
	Version,
	"Transfers ownership of the RMNRemote contract to a new authority",
	func(b operations.Bundle, chain cldf_solana.Chain, input solcommon.TransferOwnershipParams) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&rmn_remote.ProgramID, rmn_remote.ParseInstructionWithoutAccounts, input.Program)
		authority, err := GetAuthority(b.GetContext(), chain, input.Program)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		if authority != input.CurrentOwner {
			return sequences.OnChainOutput{}, fmt.Errorf("current owner %s does not match on-chain authority %s", input.CurrentOwner.String(), authority.String())
		}
		configPDA, _, _ := state.FindConfigPDA(input.Program)
		cursePDA, _, _ := state.FindRMNRemoteCursesPDA(input.Program)
		ixn, err := rmn_remote.NewTransferOwnershipInstruction(
			input.NewOwner,
			configPDA,
			cursePDA,
			authority,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to build add dest chain instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{ixn}, input.Program, ContractType.String())
	},
)

var AcceptOwnership = operations.NewOperation(
	"rmn-remote:accept-ownership",
	Version,
	"Accepts ownership of the RMNRemote contract",
	func(b operations.Bundle, chain cldf_solana.Chain, input solcommon.TransferOwnershipParams) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&rmn_remote.ProgramID, rmn_remote.ParseInstructionWithoutAccounts, input.Program)
		configPDA, _, _ := state.FindConfigPDA(input.Program)
		ixn, err := rmn_remote.NewAcceptOwnershipInstruction(
			configPDA,
			input.NewOwner,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to build add dest chain instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, input.NewOwner, []solana.Instruction{ixn}, input.Program, ContractType.String())
	},
)

var Curse = operations.NewOperation(
	"rmn-remote:curse",
	Version,
	"Curses subjects with RMNRemote",
	func(b operations.Bundle, chain cldf_solana.Chain, input CurseInput) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&rmn_remote.ProgramID, rmn_remote.ParseInstructionWithoutAccounts, input.RMNRemote)
		rmnRemoteConfigPDA := input.RMNRemoteConfigPDA
		rmnRemoteCursesPDA := input.RMNRemoteCursePDA
		authority, err := GetAuthority(b.GetContext(), chain, input.RMNRemote)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		ins := make([]solana.Instruction, 0)
		for _, subject := range input.Subjects {
			curseSubject := rmn_remote.CurseSubject{
				Value: subject,
			}
			ix, err := rmn_remote.NewCurseInstruction(
				curseSubject,
				rmnRemoteConfigPDA,
				authority,
				rmnRemoteCursesPDA,
				solana.SystemProgramID,
			)
			if err != nil {
				return sequences.OnChainOutput{}, fmt.Errorf("failed to build curse instruction: %w", err)
			}
			ins = append(ins, ix)
		}
		batches := make([]types.BatchOperation, 0)
		if authority != chain.DeployerKey.PublicKey() {
			b, err := solcommon.BuildMCMSBatchOperation(
				chain.Selector,
				ins,
				input.RMNRemote.String(),
				ContractType.String(),
			)
			if err != nil {
				return sequences.OnChainOutput{}, fmt.Errorf("failed to execute or create batch: %w", err)
			}
			batches = append(batches, b)
		} else {
			for _, ixn := range ins {
				err := chain.Confirm([]solana.Instruction{ixn})
				if err != nil {
					return sequences.OnChainOutput{}, fmt.Errorf("failed to confirm curse instruction: %w", err)
				}
			}
		}
		return sequences.OnChainOutput{BatchOps: batches}, nil
	},
)

var Uncurse = operations.NewOperation(
	"rmn-remote:uncurse",
	Version,
	"Lifts curses for subjects with RMNRemote",
	func(b operations.Bundle, chain cldf_solana.Chain, input CurseInput) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&rmn_remote.ProgramID, rmn_remote.ParseInstructionWithoutAccounts, input.RMNRemote)
		rmnRemoteConfigPDA := input.RMNRemoteConfigPDA
		rmnRemoteCursesPDA := input.RMNRemoteCursePDA
		authority, err := GetAuthority(b.GetContext(), chain, input.RMNRemote)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		ins := make([]solana.Instruction, 0)
		for _, subject := range input.Subjects {
			curseSubject := rmn_remote.CurseSubject{
				Value: subject,
			}
			ix, err := rmn_remote.NewUncurseInstruction(
				curseSubject,
				rmnRemoteConfigPDA,
				authority,
				rmnRemoteCursesPDA,
				solana.SystemProgramID,
			)
			if err != nil {
				return sequences.OnChainOutput{}, fmt.Errorf("failed to build uncurse instruction: %w", err)
			}
			ins = append(ins, ix)
		}
		batches := make([]types.BatchOperation, 0)
		if authority != chain.DeployerKey.PublicKey() {
			b, err := solcommon.BuildMCMSBatchOperation(
				chain.Selector,
				ins,
				input.RMNRemote.String(),
				ContractType.String(),
			)
			if err != nil {
				return sequences.OnChainOutput{}, fmt.Errorf("failed to execute or create batch: %w", err)
			}
			batches = append(batches, b)
		} else {
			for _, ixn := range ins {
				err := chain.Confirm([]solana.Instruction{ixn})
				if err != nil {
					return sequences.OnChainOutput{}, fmt.Errorf("failed to confirm uncurse instruction: %w", err)
				}
			}
		}
		return sequences.OnChainOutput{BatchOps: batches}, nil
	},
)

type EventAuthoritiesInput struct {
	EventAuthorities   []solana.PublicKey
	RMNRemote          solana.PublicKey
	RMNRemoteConfigPDA solana.PublicKey
}

var SetEventAuthorities = operations.NewOperation(
	"rmn-remote:set-event-authorities",
	Version,
	"Sets the event authorities list on the RMNRemote contract",
	func(b operations.Bundle, chain cldf_solana.Chain, input EventAuthoritiesInput) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&rmn_remote.ProgramID, rmn_remote.ParseInstructionWithoutAccounts, input.RMNRemote)
		authority, err := GetAuthority(b.GetContext(), chain, input.RMNRemote)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}

		ixn, err := rmn_remote.NewSetEventAuthoritiesInstruction(
			input.EventAuthorities,
			input.RMNRemoteConfigPDA,
			authority,
			solana.SystemProgramID,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to build set event authorities instruction: %w", err)
		}

		return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{ixn}, input.RMNRemote, ContractType.String())
	},
)

type SetCurserInput struct {
	Curser    solana.PublicKey
	RMNRemote solana.PublicKey
}

// SetCurser sets the account allowed to curse and uncurse on the RMNRemote contract.
var SetCurser = operations.NewOperation(
	"rmn-remote:set-curser",
	Version,
	"Sets the curser on the RMNRemote contract",
	func(b operations.Bundle, chain cldf_solana.Chain, input SetCurserInput) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&rmn_remote.ProgramID, rmn_remote.ParseInstructionWithoutAccounts, input.RMNRemote)
		configPDA, _, err := state.FindRMNRemoteConfigPDA(input.RMNRemote)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to find RMNRemoteConfig PDA: %w", err)
		}
		var cfg rmn_remote.Config
		if err = common.GetAccountDataBorshIntoAnchor(b.GetContext(), chain.Client, configPDA, cldf_solana.SolDefaultCommitment, &cfg); err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to read RMNRemote config %s: %w", configPDA, err)
		}
		if cfg.Curser == input.Curser {
			b.Logger.Infow("RMNRemote curser already set, skipping", "rmnRemote", input.RMNRemote.String(), "curser", input.Curser.String())
			return sequences.OnChainOutput{}, nil
		}
		cursesPDA, _, err := state.FindRMNRemoteCursesPDA(input.RMNRemote)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to find RMNRemoteCurses PDA: %w", err)
		}

		authority := cfg.Owner
		ixn, err := rmn_remote.NewSetCurserInstruction(input.Curser, configPDA, cursesPDA, authority)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to build set curser instruction: %w", err)
		}

		return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{ixn}, input.RMNRemote, ContractType.String())
	},
)

// GetAuthority returns the owner of program, or the deployer key before the program is initialized.
func GetAuthority(ctx context.Context, chain cldf_solana.Chain, program solana.PublicKey) (solana.PublicKey, error) {
	configPDA, _, err := state.FindRMNRemoteConfigPDA(program)
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("failed to derive config PDA: %w", err)
	}
	return solcommon.ConfigAuthority(ctx, chain, program, configPDA, rmn_remote.ParseAccount_Config, func(c *rmn_remote.Config) solana.PublicKey { return c.Owner })
}

func IsSubjectCursed(chain cldf_solana.Chain, program solana.PublicKey, subject rmn_remote.CurseSubject) (bool, error) {
	rmnRemoteCursesPDA, _, err := state.FindRMNRemoteCursesPDA(program)
	if err != nil {
		return false, fmt.Errorf("failed to find RMNRemoteCurses PDA: %w", err)
	}

	var cursesAccount rmn_remote.Curses
	err = common.GetAccountDataBorshIntoAnchor(context.Background(), chain.Client, rmnRemoteCursesPDA, cldf_solana.SolDefaultCommitment, &cursesAccount)
	if err != nil {
		return false, fmt.Errorf("failed to read curses account: %w", err)
	}

	if slices.Contains(cursesAccount.CursedSubjects, subject) {
		return true, nil
	}

	return false, nil
}

type Params struct {
	RMNRemote solana.PublicKey
}
