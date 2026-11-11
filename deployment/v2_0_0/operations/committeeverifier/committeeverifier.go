package committeeverifier

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"

	"github.com/Masterminds/semver/v3"
	"github.com/gagliardetto/solana-go"

	"github.com/smartcontractkit/chainlink-ccip/deployment/finality"
	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/sequences"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	cldf_deployment "github.com/smartcontractkit/chainlink-deployments-framework/deployment"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/committee_verifier"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/ccip"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/allowlist"
)

var (
	ContractType cldf_deployment.ContractType = "CommitteeVerifier"
	ProgramName                               = "committee_verifier"
	Version      *semver.Version              = solcommon.Version2_0_0
)

// DeployQualifier is the datastore qualifier under which Deploy resolves/registers the
// committee-verifier program. The program state uses global seeds, so one program holds
// exactly one committee.
const DeployQualifier = "default"

type Params struct {
	CommitteeVerifier solana.PublicKey
}

type SetChainConfigParams struct {
	CommitteeVerifier   solana.PublicKey
	LocalChainSelector  uint64
	RemoteChainSelector uint64
	RouterSigner        solana.PublicKey
	FeeUSDCents         uint16
	GasForVerification  uint32
	PayloadSizeBytes    uint32
}

type SetSignerConfigParams struct {
	CommitteeVerifier   solana.PublicKey
	LocalChainSelector  uint64
	SourceChainSelector uint64
	Threshold           uint8
	Signers             [][20]uint8
}

type RemoveSignerConfigParams struct {
	CommitteeVerifier   solana.PublicKey
	SourceChainSelector uint64
}

// SetAllowlistParams describes changes to a lane's sender allowlist. Enabled sets the flag, and
// the senders to add and to remove are applied to the lane's current list.
type SetAllowlistParams struct {
	CommitteeVerifier      solana.PublicKey
	DestChainSelector      uint64
	Enabled                bool
	AllowedSendersToAdd    []solana.PublicKey
	AllowedSendersToRemove []solana.PublicKey
}

type SetStorageLocationsParams struct {
	CommitteeVerifier solana.PublicKey
	StorageLocations  []string
}

type SetAllowlistAdminParams struct {
	CommitteeVerifier solana.PublicKey
	AllowlistAdmin    solana.PublicKey
}

type SetFeeAggregatorParams struct {
	CommitteeVerifier solana.PublicKey
	FeeAggregator     solana.PublicKey
}

type SetAllowedFinalityConfigParams struct {
	CommitteeVerifier solana.PublicKey
	Finality          committee_verifier.FinalityConfig
}

// FinalityConfigFromCommon converts the chain-agnostic finality.Config into the on-chain
// flags/block-depth split. The wire format is a big-endian bytes4 (flags in the upper 16
// bits, block depth in the lower 16); WaitForFinality is the zero-value sentinel and has
// no bit of its own.
func FinalityConfigFromCommon(cfg finality.Config) committee_verifier.FinalityConfig {
	raw := cfg.Raw()
	return committee_verifier.FinalityConfig{
		Flags:      binary.BigEndian.Uint16(raw[:2]),
		BlockDepth: binary.BigEndian.Uint16(raw[2:]),
	}
}

var Deploy = operations.NewOperation(
	"committee-verifier:deploy",
	Version,
	"Deploys the ComitteeVerifier program",
	func(b operations.Bundle, chain cldf_solana.Chain, input []datastore.AddressRef) (datastore.AddressRef, error) {
		return solcommon.DeployContractIfNotInDatastore(
			b,
			chain,
			input,
			ContractType,
			Version,
			DeployQualifier,
			"committee_verifier")
	},
)

var Upgrade = operations.NewOperation(
	"committee-verifier:upgrade",
	Version,
	"Upgrades the default CommitteeVerifier program",
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
	"committee-verifier:initialize",
	Version,
	"Initializes the CommitteeVerifier contract",
	func(b operations.Bundle, chain cldf_solana.Chain, input Params) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&committee_verifier.ProgramID, committee_verifier.ParseInstructionWithoutAccounts, input.CommitteeVerifier)
		if chain.DeployerKey == nil {
			return sequences.OnChainOutput{}, errors.New("missing Solana deployer key")
		}
		if input.CommitteeVerifier.IsZero() {
			return sequences.OnChainOutput{}, errors.New("committee verifier program is required")
		}

		programData, err := solcommon.GetSolProgramData(chain.Client, input.CommitteeVerifier)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to get program data: %w", err)
		}
		configPDA, _, err := state.FindConfigPDA(input.CommitteeVerifier)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive config PDA: %w", err)
		}
		existing, err := common.GetInitializedAccount(b.GetContext(), chain.Client, configPDA, input.CommitteeVerifier, cldf_solana.SolDefaultCommitment, committee_verifier.ParseAccount_Config)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to read config account: %w", err)
		}
		if existing != nil {
			b.Logger.Infof("CommitteeVerifier %s is already initialized", input.CommitteeVerifier)
			return sequences.OnChainOutput{}, nil
		}
		storageLocationsPDA, _, err := state.FindCVStorageLocationsPDA(input.CommitteeVerifier)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive storage locations PDA: %w", err)
		}

		// Only the program's upgrade authority may initialize it.
		authority, err := solcommon.InitializerAuthority(chain, input.CommitteeVerifier)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		instruction, err := committee_verifier.NewInitializeInstruction(
			configPDA,
			storageLocationsPDA,
			authority,
			solana.SystemProgramID,
			input.CommitteeVerifier,
			programData.Address,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to build initialize instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{instruction}, input.CommitteeVerifier, ContractType.String())
	},
)

var SetFeeAggregator = operations.NewOperation(
	"committee-verifier:set-fee-aggregator",
	Version,
	"Configures CommitteeVerifier fee aggregator",
	func(b operations.Bundle, chain cldf_solana.Chain, input SetFeeAggregatorParams) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&committee_verifier.ProgramID, committee_verifier.ParseInstructionWithoutAccounts, input.CommitteeVerifier)
		if chain.DeployerKey == nil {
			return sequences.OnChainOutput{}, errors.New("missing Solana deployer key")
		}
		if input.FeeAggregator.IsZero() {
			return sequences.OnChainOutput{}, errors.New("fee aggregator is required")
		}

		configPDA, eventAuthorityPDA, config, err := readConfig(b.GetContext(), chain, input.CommitteeVerifier)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		if config.FeeAggregator == input.FeeAggregator {
			b.Logger.Infof("CommitteeVerifier %s fee aggregator is already %s", input.CommitteeVerifier, input.FeeAggregator)
			return sequences.OnChainOutput{}, nil
		}
		instruction, err := committee_verifier.NewSetFeeAggregatorInstruction(
			input.FeeAggregator,
			configPDA,
			config.Owner,
			eventAuthorityPDA,
			input.CommitteeVerifier,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to build set_fee_aggregator instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, config.Owner, []solana.Instruction{instruction}, input.CommitteeVerifier, ContractType.String())
	},
)

var SetAllowlistAdmin = operations.NewOperation(
	"committee-verifier:set-allowlist-admin",
	Version,
	"Configures CommitteeVerifier allowlist admin",
	func(b operations.Bundle, chain cldf_solana.Chain, input SetAllowlistAdminParams) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&committee_verifier.ProgramID, committee_verifier.ParseInstructionWithoutAccounts, input.CommitteeVerifier)
		if chain.DeployerKey == nil {
			return sequences.OnChainOutput{}, errors.New("missing Solana deployer key")
		}
		if input.AllowlistAdmin.IsZero() {
			return sequences.OnChainOutput{}, errors.New("allowlist admin is required")
		}
		configPDA, eventAuthorityPDA, config, err := readConfig(b.GetContext(), chain, input.CommitteeVerifier)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		if config.AllowlistAdmin == input.AllowlistAdmin {
			b.Logger.Infof("CommitteeVerifier %s allowlist admin is already %s", input.CommitteeVerifier, input.AllowlistAdmin)
			return sequences.OnChainOutput{}, nil
		}
		instruction, err := committee_verifier.NewSetAllowlistAdminInstruction(
			input.AllowlistAdmin,
			configPDA,
			config.Owner,
			eventAuthorityPDA,
			input.CommitteeVerifier,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to build set_allowlist_admin instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, config.Owner, []solana.Instruction{instruction}, input.CommitteeVerifier, ContractType.String())
	},
)

var SetAllowedFinalityConfig = operations.NewOperation(
	"committee-verifier:set-allowed-finality-config",
	Version,
	"Configures CommitteeVerifier allowed finality",
	func(b operations.Bundle, chain cldf_solana.Chain, input SetAllowedFinalityConfigParams) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&committee_verifier.ProgramID, committee_verifier.ParseInstructionWithoutAccounts, input.CommitteeVerifier)
		if chain.DeployerKey == nil {
			return sequences.OnChainOutput{}, errors.New("missing Solana deployer key")
		}

		configPDA, _, err := state.FindConfigPDA(input.CommitteeVerifier)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive config PDA: %w", err)
		}
		eventAuthorityPDA, _, err := state.FindEventAuthorityPDA(input.CommitteeVerifier)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive event authority PDA: %w", err)
		}

		authority, err := getAuthority(b.GetContext(), chain, input.CommitteeVerifier)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		instruction, err := committee_verifier.NewSetAllowedFinalityConfigInstruction(
			input.Finality,
			configPDA,
			authority,
			eventAuthorityPDA,
			input.CommitteeVerifier,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to build set_allowed_finality_config instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{instruction}, input.CommitteeVerifier, ContractType.String())
	},
)

var SetStorageLocations = operations.NewOperation(
	"committee-verifier:set-storage-locations",
	Version,
	"Configures CommitteeVerifier storage locations",
	func(b operations.Bundle, chain cldf_solana.Chain, input SetStorageLocationsParams) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&committee_verifier.ProgramID, committee_verifier.ParseInstructionWithoutAccounts, input.CommitteeVerifier)
		if chain.DeployerKey == nil {
			return sequences.OnChainOutput{}, errors.New("missing Solana deployer key")
		}

		configPDA, eventAuthorityPDA, config, err := readConfig(b.GetContext(), chain, input.CommitteeVerifier)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		// This is the string/bytes boundary. On-chain the locations are opaque bytes — the program
		// stores and echoes them without interpreting them — while CLDF configs and env.toml supply
		// them as URLs, so the UTF-8 encoding happens here and the CLDF-facing API stays []string.
		storageLocations := make([]committee_verifier.ByteString, len(input.StorageLocations))
		for i, location := range input.StorageLocations {
			storageLocations[i] = committee_verifier.ByteString{V0: []byte(location)}
		}
		storageLocationsPDA, _, err := state.FindCVStorageLocationsPDA(input.CommitteeVerifier)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive storage locations PDA: %w", err)
		}
		var current committee_verifier.StorageLocations
		if err = common.GetAccountDataBorshIntoAnchor(b.GetContext(), chain.Client, storageLocationsPDA, cldf_solana.SolDefaultCommitment, &current); err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to read storage locations account %s: %w", storageLocationsPDA, err)
		}
		if slices.EqualFunc(current.Locations, input.StorageLocations, func(a committee_verifier.ByteString, s string) bool { return string(a.V0) == s }) {
			b.Logger.Infof("CommitteeVerifier %s storage locations are already %v", input.CommitteeVerifier, input.StorageLocations)
			return sequences.OnChainOutput{}, nil
		}
		instruction, err := committee_verifier.NewSetStorageLocationsInstruction(
			committee_verifier.SetStorageLocationsParams{StorageLocations: storageLocations},
			configPDA,
			config.StorageLocationAdmin,
			storageLocationsPDA,
			solana.SystemProgramID,
			eventAuthorityPDA,
			input.CommitteeVerifier,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to build set_storage_locations instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, config.StorageLocationAdmin, []solana.Instruction{instruction}, input.CommitteeVerifier, ContractType.String())
	},
)

var SetChainConfig = operations.NewOperation(
	"committee-verifier:set-chain-config",
	Version,
	"Configures a CommitteeVerifier remote chain",
	func(b operations.Bundle, chain cldf_solana.Chain, input SetChainConfigParams) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&committee_verifier.ProgramID, committee_verifier.ParseInstructionWithoutAccounts, input.CommitteeVerifier)
		if chain.DeployerKey == nil {
			return sequences.OnChainOutput{}, errors.New("missing Solana deployer key")
		}

		authority, err := getAuthority(b.GetContext(), chain, input.CommitteeVerifier)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		params := committee_verifier.SetChainConfigParams{
			RemoteChainSelector: input.RemoteChainSelector,
			ChainConfig: committee_verifier.RemoteChainAdminConfig{
				RouterSigner:       input.RouterSigner,
				Fee:                committee_verifier.UsdCents{V0: uint32(input.FeeUSDCents)},
				GasForVerification: committee_verifier.CrossChainGas{V0: input.GasForVerification},
				PayloadSizeBytes:   input.PayloadSizeBytes,
			},
		}
		remoteChainConfigPDA, _, err := state.FindCVRemoteChainConfigPDA(input.RemoteChainSelector, input.CommitteeVerifier)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive remote chain config PDA: %w", err)
		}
		exists, err := common.IsAccountInitialized(b.GetContext(), chain.Client, remoteChainConfigPDA, input.CommitteeVerifier, cldf_solana.SolDefaultCommitment, committee_verifier.Account_RemoteChainConfig)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		// The account grows once signers or allowed senders are stored, so an existing remote chain
		// is updated through edit_chain_config, which leaves those untouched.
		resolve := ccip.ResolveCVInitChainConfigAccounts
		if exists {
			resolve = ccip.ResolveCVEditChainConfigAccounts
		}
		resolved, err := resolve(
			b.GetContext(),
			chain.Client,
			input.CommitteeVerifier,
			params,
			authority,
			b.Logger,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to resolve chain config accounts: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{resolved.Ix}, input.CommitteeVerifier, ContractType.String())
	},
)

var SetSignerConfig = operations.NewOperation(
	"committee-verifier:set-signer-config",
	Version,
	"Configures a CommitteeVerifier remote chain signer set",
	func(b operations.Bundle, chain cldf_solana.Chain, input SetSignerConfigParams) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&committee_verifier.ProgramID, committee_verifier.ParseInstructionWithoutAccounts, input.CommitteeVerifier)
		if chain.DeployerKey == nil {
			return sequences.OnChainOutput{}, errors.New("missing Solana deployer key")
		}

		authority, err := getAuthority(b.GetContext(), chain, input.CommitteeVerifier)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		params := committee_verifier.SetSignerConfigParams{
			SourceChainSelector: input.SourceChainSelector,
			Threshold:           input.Threshold,
			Signers:             input.Signers,
		}
		resolved, err := ccip.ResolveCVSetSignerConfigAccounts(
			b.GetContext(),
			chain.Client,
			input.CommitteeVerifier,
			params,
			authority,
			b.Logger,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to resolve set_signer_config accounts: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{resolved.Ix}, input.CommitteeVerifier, ContractType.String())
	},
)

var RemoveSignerConfig = operations.NewOperation(
	"committee-verifier:remove-signer-config",
	Version,
	"Clears a CommitteeVerifier remote chain signer set",
	func(b operations.Bundle, chain cldf_solana.Chain, input RemoveSignerConfigParams) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&committee_verifier.ProgramID, committee_verifier.ParseInstructionWithoutAccounts, input.CommitteeVerifier)
		if chain.DeployerKey == nil {
			return sequences.OnChainOutput{}, errors.New("missing Solana deployer key")
		}

		authority, err := getAuthority(b.GetContext(), chain, input.CommitteeVerifier)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		params := committee_verifier.RemoveSignerConfigParams{
			SourceChainSelector: input.SourceChainSelector,
		}
		resolved, err := ccip.ResolveCVRemoveSignerConfigAccounts(
			b.GetContext(),
			chain.Client,
			input.CommitteeVerifier,
			params,
			authority,
			b.Logger,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to resolve remove_signer_config accounts: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{resolved.Ix}, input.CommitteeVerifier, ContractType.String())
	},
)

// SetAllowlist applies sender allowlist changes to a lane. set_allowlist replaces the whole
// on-chain allowlist, so the changes are applied to the lane's current one first, and nothing is
// sent when the result matches it. See allowlist.Signer for who signs.
var SetAllowlist = operations.NewOperation(
	"committee-verifier:set-allowlist",
	Version,
	"Sets the CommitteeVerifier sender allowlist for a destination chain",
	func(b operations.Bundle, chain cldf_solana.Chain, input SetAllowlistParams) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&committee_verifier.ProgramID, committee_verifier.ParseInstructionWithoutAccounts, input.CommitteeVerifier)
		if chain.DeployerKey == nil {
			return sequences.OnChainOutput{}, errors.New("missing Solana deployer key")
		}

		current, err := readAllowlist(b.GetContext(), chain.Client, input.CommitteeVerifier, input.DestChainSelector)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		senders, changed, err := allowlist.Update(current, input.Enabled, input.AllowedSendersToAdd, input.AllowedSendersToRemove)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("invalid allowlist for remote chain %d: %w", input.DestChainSelector, err)
		}
		if !changed {
			b.Logger.Infof("CommitteeVerifier allowlist for remote chain %d is up to date", input.DestChainSelector)
			return sequences.OnChainOutput{}, nil
		}
		owner, err := getAuthority(b.GetContext(), chain, input.CommitteeVerifier)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		admin, err := getAllowlistAdmin(b.GetContext(), chain, input.CommitteeVerifier)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		authority, err := allowlist.Signer(current, owner, admin, chain.DeployerKey.PublicKey())
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("remote chain %d on CommitteeVerifier %s: %w", input.DestChainSelector, input.CommitteeVerifier, err)
		}
		params := committee_verifier.SetAllowlistParams{
			DestChainSelector: input.DestChainSelector,
			Enabled:           input.Enabled,
			AllowedSenders:    senders,
		}
		resolved, err := ccip.ResolveCCVSetAllowlistAccounts(
			b.GetContext(),
			chain.Client,
			input.CommitteeVerifier,
			params,
			authority,
			b.Logger,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to resolve set_allowlist accounts: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{resolved.Ix}, input.CommitteeVerifier, ContractType.String())
	},
)

var TransferOwnership = operations.NewOperation(
	"committee-verifier:transfer-ownership",
	Version,
	"Transfers ownership of the CommitteeVerifier contract to a new authority",
	func(b operations.Bundle, chain cldf_solana.Chain, input solcommon.TransferOwnershipParams) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&committee_verifier.ProgramID, committee_verifier.ParseInstructionWithoutAccounts, input.Program)
		authority, err := getAuthority(b.GetContext(), chain, input.Program)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		if authority != input.CurrentOwner {
			return sequences.OnChainOutput{}, fmt.Errorf("current owner %s does not match on-chain authority %s", input.CurrentOwner.String(), authority.String())
		}
		configPDA, _, err := state.FindConfigPDA(input.Program)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive config PDA: %w", err)
		}
		eventAuthorityPDA, _, err := state.FindEventAuthorityPDA(input.Program)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive event authority PDA: %w", err)
		}
		ixn, err := committee_verifier.NewTransferOwnershipInstruction(
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
	"committee-verifier:accept-ownership",
	Version,
	"Accepts ownership of the CommitteeVerifier contract",
	func(b operations.Bundle, chain cldf_solana.Chain, input solcommon.TransferOwnershipParams) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&committee_verifier.ProgramID, committee_verifier.ParseInstructionWithoutAccounts, input.Program)
		configPDA, _, err := state.FindConfigPDA(input.Program)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive config PDA: %w", err)
		}
		eventAuthorityPDA, _, err := state.FindEventAuthorityPDA(input.Program)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive event authority PDA: %w", err)
		}
		ixn, err := committee_verifier.NewAcceptOwnershipInstruction(
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

// readConfig returns the Config PDA, the event authority PDA and the decoded Config.
func readConfig(ctx context.Context, chain cldf_solana.Chain, program solana.PublicKey) (solana.PublicKey, solana.PublicKey, committee_verifier.Config, error) {
	var config committee_verifier.Config
	configPDA, _, err := state.FindConfigPDA(program)
	if err != nil {
		return solana.PublicKey{}, solana.PublicKey{}, config, fmt.Errorf("failed to derive config PDA: %w", err)
	}
	eventAuthorityPDA, _, err := state.FindEventAuthorityPDA(program)
	if err != nil {
		return solana.PublicKey{}, solana.PublicKey{}, config, fmt.Errorf("failed to derive event authority PDA: %w", err)
	}
	if err = common.GetAccountDataBorshIntoAnchor(ctx, chain.Client, configPDA, cldf_solana.SolDefaultCommitment, &config); err != nil {
		return solana.PublicKey{}, solana.PublicKey{}, config, fmt.Errorf("failed to read config account %s: %w", configPDA, err)
	}
	return configPDA, eventAuthorityPDA, config, nil
}

// getAuthority returns the owner of program, or the deployer key before the program is initialized.
func getAuthority(ctx context.Context, chain cldf_solana.Chain, program solana.PublicKey) (solana.PublicKey, error) {
	configPDA, _, err := state.FindConfigPDA(program)
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("failed to derive config PDA: %w", err)
	}
	return solcommon.ConfigAuthority(ctx, chain, program, configPDA, committee_verifier.ParseAccount_Config, func(c *committee_verifier.Config) solana.PublicKey { return c.Owner })
}

// getAllowlistAdmin returns the allowlist admin of program, or the deployer key before the program is initialized.
func getAllowlistAdmin(ctx context.Context, chain cldf_solana.Chain, program solana.PublicKey) (solana.PublicKey, error) {
	configPDA, _, err := state.FindConfigPDA(program)
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("failed to derive config PDA: %w", err)
	}
	return solcommon.ConfigAuthority(ctx, chain, program, configPDA, committee_verifier.ParseAccount_Config, func(c *committee_verifier.Config) solana.PublicKey { return c.AllowlistAdmin })
}

// ResolveCommitteeVerifiers resolves the CommitteeVerifier program pubkeys for the given
// qualifiers on one chain from datastore address refs.
func ResolveCommitteeVerifiers(refs []datastore.AddressRef, chainSelector uint64, qualifiers []string) ([]solana.PublicKey, error) {
	out := make([]solana.PublicKey, 0, len(qualifiers))
	for _, qualifier := range qualifiers {
		ccv, err := solcommon.PubkeyFromRefs(refs, datastore.NewAddressRefKey(
			chainSelector,
			datastore.ContractType(ContractType),
			Version,
			qualifier,
		))
		if err != nil {
			return nil, fmt.Errorf("qualifier %q: %w", qualifier, err)
		}
		out = append(out, ccv)
	}
	return out, nil
}
