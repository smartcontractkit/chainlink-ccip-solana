package executor

import (
	"context"
	"errors"
	"fmt"

	"github.com/gagliardetto/solana-go"

	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/sequences"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	"github.com/smartcontractkit/chainlink-deployments-framework/deployment"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/executor"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
)

var (
	// ProxyContractType is the datastore contract type for Solana executor.
	ContractType deployment.ContractType = "Executor"

	// ProxyContractType is the datastore contract type for Solana executor proxy (devenv placeholders, adapters).
	ProxyContractType datastore.ContractType = "ExecutorProxy"

	// Version is the datastore AddressRef semver for ExecutorProxy.
	Version     = solcommon.Version2_0_0
	ProgramName = "executor"
)

var Deploy = operations.NewOperation(
	"executor:deploy",
	Version,
	"Deploys the default Executor program",
	func(b operations.Bundle, chain cldf_solana.Chain, input []datastore.AddressRef) (datastore.AddressRef, error) {
		return solcommon.DeployContractIfNotInDatastore(
			b,
			chain,
			input,
			ContractType,
			Version,
			"default",
			"executor")
	},
)

var Upgrade = operations.NewOperation(
	"executor:upgrade",
	Version,
	"Upgrades the default Executor program",
	func(b operations.Bundle, chain cldf_solana.Chain, programID solana.PublicKey) ([]solana.Instruction, error) {
		return solcommon.UpgradeProgram(
			b,
			chain,
			ContractType,
			programID,
			"executor",
		)
	},
)

type Params struct {
	Executor              solana.PublicKey
	MaxCcvsPerMsg         uint8
	FeeAggregator         solana.PublicKey
	AllowedFinalityConfig executor.FinalityConfig
	CcvAllowlistEnabled   bool
}

var Initialize = operations.NewOperation(
	"executor:initialize",
	Version,
	"Initializes the Executor contract",
	func(b operations.Bundle, chain cldf_solana.Chain, input Params) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&executor.ProgramID, executor.ParseInstructionWithoutAccounts, input.Executor)
		if chain.DeployerKey == nil {
			return sequences.OnChainOutput{}, errors.New("missing Solana deployer key")
		}
		if input.Executor.IsZero() {
			return sequences.OnChainOutput{}, errors.New("executor program is required")
		}

		programData, err := solcommon.GetSolProgramData(chain.Client, input.Executor)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to get program data: %w", err)
		}
		// Only the program's upgrade authority may initialize it.
		authority, err := solcommon.InitializerAuthority(chain, input.Executor)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		statePDA, _, _ := state.FindConfigPDA(input.Executor)
		eventAuthority, _, _ := state.FindEventAuthorityPDA(input.Executor)

		instruction, err := executor.NewInitializeInstruction(
			executor.InitializeParams{
				MaxCcvsPerMsg:         input.MaxCcvsPerMsg,
				FeeAggregator:         input.FeeAggregator,
				AllowedFinalityConfig: input.AllowedFinalityConfig,
				CcvAllowlistEnabled:   input.CcvAllowlistEnabled,
			},
			statePDA,
			authority,
			solana.SystemProgramID,
			eventAuthority,
			input.Executor,
			programData.Address,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to build initialize instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{instruction}, input.Executor, ContractType.String())
	},
)

var TransferOwnership = operations.NewOperation(
	"executor:transfer-ownership",
	Version,
	"Transfers ownership of the Executor contract to a new authority",
	func(b operations.Bundle, chain cldf_solana.Chain, input solcommon.TransferOwnershipParams) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&executor.ProgramID, executor.ParseInstructionWithoutAccounts, input.Program)
		authority, err := GetAuthority(b.GetContext(), chain, input.Program)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		if authority != input.CurrentOwner {
			return sequences.OnChainOutput{}, fmt.Errorf("current owner %s does not match on-chain authority %s", input.CurrentOwner.String(), authority.String())
		}
		configPDA, _, _ := state.FindConfigPDA(input.Program)
		eventAuthorityPDA, _, _ := state.FindEventAuthorityPDA(input.Program)
		ixn, err := executor.NewTransferOwnershipInstruction(
			input.NewOwner,
			configPDA,
			authority,
			eventAuthorityPDA,
			input.Program,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to build transfer ownership instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{ixn}, input.Program, ContractType.String())
	},
)

var AcceptOwnership = operations.NewOperation(
	"executor:accept-ownership",
	Version,
	"Accepts ownership of the Executor contract",
	func(b operations.Bundle, chain cldf_solana.Chain, input solcommon.TransferOwnershipParams) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&executor.ProgramID, executor.ParseInstructionWithoutAccounts, input.Program)
		configPDA, _, _ := state.FindConfigPDA(input.Program)
		eventAuthorityPDA, _, _ := state.FindEventAuthorityPDA(input.Program)
		ixn, err := executor.NewAcceptOwnershipInstruction(
			configPDA,
			input.NewOwner,
			eventAuthorityPDA,
			input.Program,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to build accept ownership instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, input.NewOwner, []solana.Instruction{ixn}, input.Program, ContractType.String())
	},
)

type SetChainConfigParams struct {
	LocalChainSelector  uint64
	Executor            solana.PublicKey
	RemoteChainSelector uint64
	UsdCentsFee         uint16
	Enabled             bool
}

var SetChainConfig = operations.NewOperation(
	"executor:set_chain_config",
	Version,
	"Configures the Executor for remote chain",
	func(b operations.Bundle, chain cldf_solana.Chain, input SetChainConfigParams) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&executor.ProgramID, executor.ParseInstructionWithoutAccounts, input.Executor)
		if chain.DeployerKey == nil {
			return sequences.OnChainOutput{}, errors.New("missing Solana deployer key")
		}
		if input.Executor.IsZero() {
			return sequences.OnChainOutput{}, errors.New("executor program is required")
		}

		authority, err := GetAuthority(b.GetContext(), chain, input.Executor)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		statePDA, _, _ := state.FindConfigPDA(input.Executor)
		eventAuthority, _, _ := state.FindEventAuthorityPDA(input.Executor)
		chainConfigPDA, _, _ := state.FindExecChainConfigPDA(input.RemoteChainSelector, input.Executor)

		instruction, err := executor.NewSetChainConfigInstruction(
			executor.SetChainConfigParams{
				RemoteChainSelector: input.RemoteChainSelector,
				UsdCentsFee:         executor.UsdCents{V0: uint32(input.UsdCentsFee)},
				Enabled:             input.Enabled,
			},
			statePDA,
			authority,
			chainConfigPDA,
			solana.SystemProgramID,
			eventAuthority,
			input.Executor,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to build executor.setChainConfig instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{instruction}, input.Executor, ContractType.String())
	},
)

// GetAuthority returns the owner of program, or the deployer key before the program is initialized.
func GetAuthority(ctx context.Context, chain cldf_solana.Chain, program solana.PublicKey) (solana.PublicKey, error) {
	configPDA, _, err := state.FindConfigPDA(program)
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("failed to derive config PDA: %w", err)
	}
	return solcommon.ConfigAuthority(ctx, chain, program, configPDA, executor.ParseAccount_Config, func(c *executor.Config) solana.PublicKey { return c.Owner })
}
