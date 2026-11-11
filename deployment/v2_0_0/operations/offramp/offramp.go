package offramp

import (
	"context"
	"fmt"
	"strconv"

	"github.com/Masterminds/semver/v3"
	"github.com/gagliardetto/solana-go"

	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/sequences"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	cldf_deployment "github.com/smartcontractkit/chainlink-deployments-framework/deployment"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/ccip_offramp"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
)

var (
	ContractType    cldf_deployment.ContractType = "OffRamp"
	SourceChainType cldf_deployment.ContractType = "RemoteSource"
	ProgramName                                  = "ccip_offramp"
	Version         *semver.Version              = solcommon.Version2_0_0
)

type Params struct {
	FeeQuoter solana.PublicKey
	Router    solana.PublicKey
	OffRamp   solana.PublicKey
	RMNRemote solana.PublicKey
}

type ConnectChainsParams struct {
	LocalChainSelector  uint64
	OffRamp             solana.PublicKey
	RemoteChainSelector uint64
	// SourceOnRamps is the complete set of OnRamps allowed to send messages from the remote chain,
	// not a delta: whatever is passed here replaces the currently configured set (matching the EVM
	// OffRamp, whose applySourceChainConfigUpdates clears and repopulates the allowed OnRamps).
	//
	// More than one entry lets the remote chain upgrade its OnRamp without downtime: configure
	// [old, new] while the old OnRamp still has inflight messages, then [new] once it has drained.
	// Dropping an OnRamp that still has inflight messages leaves those messages unexecutable while
	// it is missing; re-adding it makes them executable again, since the OffRamp checks the allowed
	// set at execution time and a rejected execution records nothing.
	//
	// Must be non-empty: the OffRamp rejects a source chain config without OnRamps.
	SourceOnRamps             [][]byte
	EnabledAsSource           bool
	IsRMNVerificationDisabled bool
	DefaultCCVs               []solana.PublicKey
	LaneMandatedCCVs          []solana.PublicKey
	// LegacyAddressLength is the width the OffRamp trims source pool addresses to before CPI'ing
	// into pools (20 for EVM remotes, 32 otherwise), so they match what pools store.
	LegacyAddressLength uint8
}

var Deploy = operations.NewOperation(
	"off-ramp:deploy",
	Version,
	"Deploys the OffRamp program",
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
	"off-ramp:upgrade",
	Version,
	"Upgrades the default OffRamp program",
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
	"off-ramp:initialize",
	Version,
	"Initializes the OffRamp contract",
	func(b operations.Bundle, chain cldf_solana.Chain, input Params) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&ccip_offramp.ProgramID, ccip_offramp.ParseInstructionWithoutAccounts, input.OffRamp)
		programData, err := solcommon.GetSolProgramData(chain.Client, input.OffRamp)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to get program data: %w", err)
		}
		// Only the program's upgrade authority may initialize it.
		authority, err := solcommon.InitializerAuthority(chain, input.OffRamp)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		offRampReferenceAddressesPDA, _, _ := state.FindOfframpReferenceAddressesPDA(input.OffRamp)
		offRampConfigPDA, _, _ := state.FindOfframpConfigPDA(input.OffRamp)
		offRampEventAuthorityPDA, _, _ := state.FindEventAuthorityPDA(input.OffRamp)
		rmnRemoteConfigPDA, _, _ := state.FindRMNRemoteConfigPDA(input.RMNRemote)
		rmnRemoteCursesPDA, _, _ := state.FindRMNRemoteCursesPDA(input.RMNRemote)
		// Already initialized: creating another lookup table here would orphan the one the
		// offramp references and cost rent on every re-run.
		existing, err := common.GetInitializedAccount(b.GetContext(), chain.Client, offRampReferenceAddressesPDA, input.OffRamp, cldf_solana.SolDefaultCommitment, ccip_offramp.ParseAccount_ReferenceAddresses)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("check offramp initialization: %w", err)
		}
		if existing != nil {
			b.Logger.Infof("OffRamp %s is already initialized, skipping", input.OffRamp)
			return sequences.OnChainOutput{}, nil
		}
		table, err := common.SetupLookupTable(
			b.GetContext(),
			chain.Client,
			*chain.DeployerKey,
			[]solana.PublicKey{
				// system
				solana.SystemProgramID,
				solana.ComputeBudget,
				solana.SysVarInstructionsPubkey,
				// token
				solana.Token2022ProgramID,
				solana.TokenProgramID,
				solana.SPLAssociatedTokenAccountProgramID,
				// constant per-offramp
				input.OffRamp,
				offRampReferenceAddressesPDA,
				offRampConfigPDA,
				offRampEventAuthorityPDA,
				input.RMNRemote,
				rmnRemoteConfigPDA,
				rmnRemoteCursesPDA,
			})
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to setup lookup table: %w", err)
		}
		instruction, err := ccip_offramp.NewInitializeInstruction(
			offRampReferenceAddressesPDA,
			input.Router,
			input.FeeQuoter,
			input.RMNRemote,
			table,
			authority,
			solana.SystemProgramID,
			input.OffRamp,
			programData.Address,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to build initialize instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{instruction}, input.OffRamp, ContractType.String())
	},
)

var InitializeConfig = operations.NewOperation(
	"off-ramp:initialize-config",
	Version,
	"Initializes the config of the OffRamp contract",
	func(b operations.Bundle, chain cldf_solana.Chain, input Params) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&ccip_offramp.ProgramID, ccip_offramp.ParseInstructionWithoutAccounts, input.OffRamp)
		programData, err := solcommon.GetSolProgramData(chain.Client, input.OffRamp)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to get program data: %w", err)
		}
		authority, err := solcommon.InitializerAuthority(chain, input.OffRamp)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		offRampConfigPDA, _, _ := state.FindOfframpConfigPDA(input.OffRamp)
		instruction, err := ccip_offramp.NewInitializeConfigInstruction(
			chain.Selector,
			offRampConfigPDA,
			authority,
			solana.SystemProgramID,
			input.OffRamp,
			programData.Address,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to build initialize instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{instruction}, input.OffRamp, ContractType.String())
	},
)

var ConnectChains = operations.NewOperation(
	"off-ramp:connect-chains",
	Version,
	"Connects the OffRamp contract to other chains",
	func(b operations.Bundle, chain cldf_solana.Chain, input ConnectChainsParams) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&ccip_offramp.ProgramID, ccip_offramp.ParseInstructionWithoutAccounts, input.OffRamp)
		b.Logger.Infof("Connecting OffRamp to remote chain %+v", input)
		// The contract rejects an empty OnRamp set, so an instruction built from one is guaranteed to
		// fail on-chain with InvalidOnrampAddress. Refusing it here says which lane is misconfigured.
		if len(input.SourceOnRamps) == 0 {
			return sequences.OnChainOutput{}, fmt.Errorf("no source OnRamps given for remote chain %d: the OffRamp requires at least one", input.RemoteChainSelector)
		}
		isUpdate := false
		authority, err := GetAuthority(b.GetContext(), chain, input.OffRamp)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		offRampConfigPDA, _, _ := state.FindOfframpConfigPDA(input.OffRamp)
		offRampSourceChainPDA, _, _ := state.FindOfframpSourceChainPDA(input.RemoteChainSelector, input.OffRamp)
		// Whether the lane already exists decides between add_source_chain and
		// update_source_chain_config. An account that cannot be read as a SourceChain likely holds an
		// outdated schema version, and add_source_chain would fail on it, so it is reported instead.
		sourceChainAccount, err := common.GetInitializedAccount(b.GetContext(), chain.Client, offRampSourceChainPDA, input.OffRamp, cldf_solana.SolDefaultCommitment, ccip_offramp.ParseAccount_SourceChain)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to read source chain account %s for remote chain %d: %w", offRampSourceChainPDA, input.RemoteChainSelector, err)
		}
		if sourceChainAccount != nil {
			b.Logger.Infof("Remote chain state account found: %+v", *sourceChainAccount)
			isUpdate = true
		} else {
			b.Logger.Infof("Remote chain state account %s not found, adding source chain %d", offRampSourceChainPDA, input.RemoteChainSelector)
		}
		// The full set of OnRamps allowed to send from this source chain. Both the add and the update
		// instruction replace it wholesale, so the input must always carry every OnRamp to keep — see
		// SourceOnRamps.
		onRampAddresses := make([]ccip_offramp.OnRampAddress, 0, len(input.SourceOnRamps))
		for _, sourceOnRamp := range input.SourceOnRamps {
			var onRampAddress ccip_offramp.OnRampAddress
			addressBytesLen := len(sourceOnRamp)
			if addressBytesLen == 0 || addressBytesLen > len(onRampAddress.Bytes) {
				return sequences.OnChainOutput{}, fmt.Errorf("invalid on ramp address length: %d", addressBytesLen)
			}
			copy(onRampAddress.Bytes[:], sourceOnRamp)
			onRampAddress.Len = uint32(addressBytesLen)
			onRampAddresses = append(onRampAddresses, onRampAddress)
		}
		validSourceChainConfig := ccip_offramp.SourceChainConfig{
			OnRamps:             onRampAddresses,
			IsEnabled:           input.EnabledAsSource,
			DefaultCcvs:         input.DefaultCCVs,
			LaneMandatedCcvs:    input.LaneMandatedCCVs,
			LegacyAddressLength: input.LegacyAddressLength,
		}
		b.Logger.Infof("Source chain config to be set: %+v", validSourceChainConfig)
		var ixn solana.Instruction
		var addressRefs []datastore.AddressRef
		offRampEventAuthorityPDA, _, _ := state.FindEventAuthorityPDA(input.OffRamp)
		if isUpdate {
			ixn, err = ccip_offramp.NewUpdateSourceChainConfigInstruction(
				ccip_offramp.UpdateSourceChainParams{
					SourceChainSelector: input.RemoteChainSelector,
					SourceChainConfig:   validSourceChainConfig,
				},
				offRampSourceChainPDA,
				offRampConfigPDA,
				authority,
				solana.SystemProgramID,
				offRampEventAuthorityPDA,
				input.OffRamp,
			)
			if err != nil {
				return sequences.OnChainOutput{}, fmt.Errorf("failed to build update dest chain instruction: %w", err)
			}
		} else {
			ixn, err = ccip_offramp.NewAddSourceChainInstruction(
				ccip_offramp.AddSourceChainParams{
					NewChainSelector:  input.RemoteChainSelector,
					SourceChainConfig: validSourceChainConfig,
				},
				offRampSourceChainPDA,
				offRampConfigPDA,
				authority,
				solana.SystemProgramID,
				offRampEventAuthorityPDA,
				input.OffRamp,
			)
			if err != nil {
				return sequences.OnChainOutput{}, fmt.Errorf("failed to build add source chain instruction: %w", err)
			}
			err = solcommon.ExtendOfframpLookupTable(chain, input.OffRamp, []solana.PublicKey{offRampSourceChainPDA})
			if err != nil {
				return sequences.OnChainOutput{}, fmt.Errorf("failed to extend OffRamp lookup table: %w", err)
			}
			addressRefs = append(addressRefs, datastore.AddressRef{
				Address:       offRampSourceChainPDA.String(),
				ChainSelector: chain.Selector,
				Type:          datastore.ContractType(SourceChainType),
				Version:       Version,
				Qualifier:     strconv.FormatUint(input.RemoteChainSelector, 10),
			})
		}
		output, err := solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{ixn}, input.OffRamp, ContractType.String())
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		output.Addresses = addressRefs
		return output, nil
	},
)

type DisableSourceChainParams struct {
	OffRamp             solana.PublicKey
	RemoteChainSelector uint64
}

var DisableSourceChain = operations.NewOperation(
	"off-ramp:disable-source-chain",
	Version,
	"Disables a source chain on the OffRamp, preventing receiving from that chain",
	func(b operations.Bundle, chain cldf_solana.Chain, input DisableSourceChainParams) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&ccip_offramp.ProgramID, ccip_offramp.ParseInstructionWithoutAccounts, input.OffRamp)
		authority, err := GetAuthority(b.GetContext(), chain, input.OffRamp)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		offRampConfigPDA, _, _ := state.FindOfframpConfigPDA(input.OffRamp)
		offRampSourceChainPDA, _, _ := state.FindOfframpSourceChainPDA(input.RemoteChainSelector, input.OffRamp)

		offRampEventAuthorityPDA, _, _ := state.FindEventAuthorityPDA(input.OffRamp)
		instruction, err := ccip_offramp.NewDisableSourceChainSelectorInstruction(
			input.RemoteChainSelector,
			offRampSourceChainPDA,
			offRampConfigPDA,
			authority,
			offRampEventAuthorityPDA,
			input.OffRamp,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to build disable source chain instruction: %w", err)
		}

		return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{instruction}, input.OffRamp, ContractType.String())
	},
)

var TransferOwnership = operations.NewOperation(
	"off-ramp:transfer-ownership",
	Version,
	"Transfers ownership of the OffRamp contract to a new authority",
	func(b operations.Bundle, chain cldf_solana.Chain, input solcommon.TransferOwnershipParams) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&ccip_offramp.ProgramID, ccip_offramp.ParseInstructionWithoutAccounts, input.Program)
		authority, err := GetAuthority(b.GetContext(), chain, input.Program)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		if authority != input.CurrentOwner {
			return sequences.OnChainOutput{}, fmt.Errorf("current owner %s does not match on-chain authority %s", input.CurrentOwner.String(), authority.String())
		}
		configPDA, _, _ := state.FindConfigPDA(input.Program)
		eventAuthorityPDA, _, _ := state.FindEventAuthorityPDA(input.Program)
		ixn, err := ccip_offramp.NewTransferOwnershipInstruction(
			input.NewOwner,
			configPDA,
			authority,
			eventAuthorityPDA,
			input.Program,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to build add dest chain instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{ixn}, input.Program, ContractType.String())
	},
)

var AcceptOwnership = operations.NewOperation(
	"off-ramp:accept-ownership",
	Version,
	"Accepts ownership of the OffRamp contract",
	func(b operations.Bundle, chain cldf_solana.Chain, input solcommon.TransferOwnershipParams) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&ccip_offramp.ProgramID, ccip_offramp.ParseInstructionWithoutAccounts, input.Program)
		configPDA, _, _ := state.FindConfigPDA(input.Program)
		eventAuthorityPDA, _, _ := state.FindEventAuthorityPDA(input.Program)
		ixn, err := ccip_offramp.NewAcceptOwnershipInstruction(
			configPDA,
			input.NewOwner,
			eventAuthorityPDA,
			input.Program,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to build add dest chain instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, input.NewOwner, []solana.Instruction{ixn}, input.Program, ContractType.String())
	},
)

// GetAuthority returns the owner of program, or the deployer key before the program is initialized.
func GetAuthority(ctx context.Context, chain cldf_solana.Chain, program solana.PublicKey) (solana.PublicKey, error) {
	configPDA, _, err := state.FindOfframpConfigPDA(program)
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("failed to derive config PDA: %w", err)
	}
	return solcommon.ConfigAuthority(ctx, chain, program, configPDA, ccip_offramp.ParseAccount_Config, func(c *ccip_offramp.Config) solana.PublicKey { return c.Owner })
}
