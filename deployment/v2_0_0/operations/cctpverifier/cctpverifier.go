// Package cctpverifier holds CLDF operations for CCTP's embedded verifier. Unlike
// committee_verifier, CCTP's verifier isn't a separate on-chain program - it's a second set of
// instructions/PDAs inside the cctp_token_pool program. Deploying/upgrading the program itself is
// therefore the token pool's job (tokenpools.DeployCctp/UpgradeCctp); this package only covers the
// verifier's own state (Config/RemoteChainConfig/StorageLocations).
//
// Two ordering constraints are load-bearing and easy to get wrong:
//
//  1. initialize_verifier initializes the verifier's Config at the ["config"] PDA
//     (state.FindCVConfigPDA) itself; no pre-created account is needed.
//
//  2. init_verifier_chain_config creates the per-remote RemoteChainConfig PDA.
//     set_verifier_chain_config_cctp and set_verifier_allowlist both realloc that PDA with
//     realloc::zero = true, sizing it from the *other* field's current contents. SetChainConfig
//     must therefore run before either of them, and the two reallocs must not be interleaved
//     with a stale read of each other's state.
package cctpverifier

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/Masterminds/semver/v3"
	"github.com/gagliardetto/solana-go"

	"github.com/smartcontractkit/chainlink-ccip/deployment/finality"
	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/sequences"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	cldf_deployment "github.com/smartcontractkit/chainlink-deployments-framework/deployment"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/cctp_token_pool"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/allowlist"
)

var (
	ContractType cldf_deployment.ContractType = "CCTPVerifier"
	Version      *semver.Version              = solcommon.Version2_0_0

	// LookupTableType is the datastore type under which the per-remote-chain address lookup
	// tables are registered. Unlike committee_verifier, CCTP has no verifier-global lookup
	// table: the ALT is a field of each RemoteCctpConfig, so there is one per lane.
	LookupTableType cldf_deployment.ContractType = "CCTPVerifierLookupTable"

	// MaxFastFinalityBps mirrors the on-chain BPS_DIVIDER bound enforced by
	// set_verifier_fast_finality_bps.
	MaxFastFinalityBps uint16 = 10_000
)

// Params identifies the cctp_token_pool program hosting the verifier. The verifier shares the
// program ID with the token pool, so this is the pool program's address.
type Params struct {
	CctpVerifier solana.PublicKey
}

type SetFeeAggregatorParams struct {
	CctpVerifier  solana.PublicKey
	FeeAggregator solana.PublicKey
}

type SetStorageLocationsParams struct {
	CctpVerifier     solana.PublicKey
	StorageLocations []string
}

type SetAllowedFinalityConfigParams struct {
	CctpVerifier solana.PublicKey
	Finality     cctp_token_pool.FinalityConfig
}

type SetAllowlistAdminParams struct {
	CctpVerifier   solana.PublicKey
	AllowlistAdmin solana.PublicKey
}

type SetFastFinalityBpsParams struct {
	CctpVerifier    solana.PublicKey
	FastFinalityBps uint16
}

// SetChainConfigParams populates the chain-agnostic half of a remote chain's verifier config.
// The CCTP-specific half is written separately by SetChainConfigCctp.
type SetChainConfigParams struct {
	CctpVerifier        solana.PublicKey
	RemoteChainSelector uint64
	RouterSigner        solana.PublicKey
	FeeUSDCents         uint32
	GasForVerification  uint32
	PayloadSizeBytes    uint32
}

// SetChainConfigCctpParams populates the CCTP-specific half of a remote chain's verifier config:
// the Circle domain, the callers allowed to deposit on the source and receive on the destination,
// the optional mint recipient override, the lane's address lookup table, and the local/remote
// token pairs this lane accepts.
type SetChainConfigCctpParams struct {
	CctpVerifier         solana.PublicKey
	RemoteChainSelector  uint64
	Enabled              bool
	DomainIdentifier     uint32
	AllowedCallerReceive solana.PublicKey
	AllowedCallerDeposit solana.PublicKey
	// MintRecipientOnDest is nil when CCTP should mint straight to the message receiver.
	MintRecipientOnDest *solana.PublicKey
	// LookupTable is the lane's address lookup table; nil leaves it unset.
	LookupTable     *solana.PublicKey
	SupportedTokens []cctp_token_pool.RemoteLocalPair
}

// SetAllowlistParams describes changes to a lane's sender allowlist. Enabled sets the flag, and
// the senders to add and to remove are applied to the lane's current list.
type SetAllowlistParams struct {
	CctpVerifier           solana.PublicKey
	DestChainSelector      uint64
	Enabled                bool
	AllowedSendersToAdd    []solana.PublicKey
	AllowedSendersToRemove []solana.PublicKey
}

type TransferOwnershipParams struct {
	CctpVerifier  solana.PublicKey
	ProposedOwner solana.PublicKey
}

// AcceptOwnershipParams carries the program and optionally the proposed owner. A zero
// ProposedOwner reads the pending owner from chain; pass it explicitly when the transfer
// is batched in the same proposal and has not executed yet.
type AcceptOwnershipParams struct {
	CctpVerifier  solana.PublicKey
	ProposedOwner solana.PublicKey
}

type TransferStorageLocationAdminParams struct {
	CctpVerifier                 solana.PublicKey
	ProposedStorageLocationAdmin solana.PublicKey
}

// FinalityConfigFromCommon converts the chain-agnostic finality.Config into the on-chain
// flags/block-depth split. Wire format is identical to committee_verifier's - see
// committeeverifier.FinalityConfigFromCommon's doc comment for the byte layout.
func FinalityConfigFromCommon(cfg finality.Config) cctp_token_pool.FinalityConfig {
	raw := cfg.Raw()
	return cctp_token_pool.FinalityConfig{
		Flags:      binary.BigEndian.Uint16(raw[:2]),
		BlockDepth: binary.BigEndian.Uint16(raw[2:]),
	}
}

// Initialize creates the verifier's Config at the ["config"] PDA and the storage-locations PDA.
// Requires the program upgrade authority to sign.
var Initialize = operations.NewOperation(
	"cctp-verifier:initialize",
	Version,
	"Initializes CCTP's embedded verifier state",
	func(b operations.Bundle, chain cldf_solana.Chain, input Params) (sequences.OnChainOutput, error) {
		if err := requireProgram(chain, input.CctpVerifier); err != nil {
			return sequences.OnChainOutput{}, err
		}

		storageLocationsPDA, _, err := state.FindCVStorageLocationsPDA(input.CctpVerifier)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("derive verifier storage locations PDA: %w", err)
		}
		// The storage-locations PDA is created by this instruction and by nothing else, so its
		// existence is the reliable "already initialized" signal. The config PDA is created by
		// this instruction too.
		initialized, err := common.IsAccountInitialized(b.GetContext(), chain.Client, storageLocationsPDA, input.CctpVerifier, cldf_solana.SolDefaultCommitment, cctp_token_pool.Account_StorageLocations)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		if initialized {
			b.Logger.Infow("CCTP verifier already initialized, skipping",
				"program", input.CctpVerifier.String(), "chainSelector", chain.Selector)
			return sequences.OnChainOutput{}, nil
		}

		configPDA, _, err := state.FindCVConfigPDA(input.CctpVerifier)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("derive verifier config PDA: %w", err)
		}
		programData, err := solcommon.GetSolProgramData(chain.Client, input.CctpVerifier)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("get program data: %w", err)
		}

		// initialize_verifier checks program_data.upgrade_authority_address == authority, so the
		// signer is the program's upgrade authority: confirmed directly when the deployer holds
		// it, batched into an MCMS proposal when the timelock does (e.g. a migrated 1.6 pool).
		authority, err := solcommon.GetUpgradeAuthority(chain.Client, input.CctpVerifier)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("get upgrade authority: %w", err)
		}
		if authority.IsZero() {
			return sequences.OnChainOutput{}, fmt.Errorf("program %s has no upgrade authority", input.CctpVerifier)
		}
		instruction, err := cctp_token_pool.NewInitializeVerifierInstruction(
			configPDA,
			storageLocationsPDA,
			authority,
			solana.SystemProgramID,
			input.CctpVerifier,
			programData.Address,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("build initialize_verifier instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{instruction}, input.CctpVerifier, ContractType.String())
	},
)

var SetFeeAggregator = operations.NewOperation(
	"cctp-verifier:set-fee-aggregator",
	Version,
	"Configures the CCTP verifier's fee aggregator",
	func(b operations.Bundle, chain cldf_solana.Chain, input SetFeeAggregatorParams) (sequences.OnChainOutput, error) {
		if err := requireProgram(chain, input.CctpVerifier); err != nil {
			return sequences.OnChainOutput{}, err
		}
		if input.FeeAggregator.IsZero() {
			return sequences.OnChainOutput{}, errors.New("fee aggregator is required")
		}

		cfg, err := readConfig(b.GetContext(), chain, input.CctpVerifier)
		if err == nil && cfg.FeeAggregator == input.FeeAggregator {
			b.Logger.Infow("CCTP verifier fee aggregator already set, skipping",
				"program", input.CctpVerifier.String(), "feeAggregator", input.FeeAggregator.String())
			return sequences.OnChainOutput{}, nil
		}

		accounts, err := configAccounts(b.GetContext(), chain, input.CctpVerifier, ownerOf)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		instruction, err := cctp_token_pool.NewSetVerifierFeeAggregatorInstruction(
			input.FeeAggregator,
			accounts.config, accounts.authority, accounts.eventAuthority, input.CctpVerifier,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("build set_verifier_fee_aggregator instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, accounts.authority, []solana.Instruction{instruction}, input.CctpVerifier, ContractType.String())
	},
)

var SetAllowedFinalityConfig = operations.NewOperation(
	"cctp-verifier:set-allowed-finality-config",
	Version,
	"Configures the CCTP verifier's allowed finality",
	func(b operations.Bundle, chain cldf_solana.Chain, input SetAllowedFinalityConfigParams) (sequences.OnChainOutput, error) {
		if err := requireProgram(chain, input.CctpVerifier); err != nil {
			return sequences.OnChainOutput{}, err
		}

		cfg, err := readConfig(b.GetContext(), chain, input.CctpVerifier)
		if err == nil && cfg.AllowedFinalityConfig == input.Finality {
			b.Logger.Infow("CCTP verifier allowed finality config already set, skipping",
				"program", input.CctpVerifier.String())
			return sequences.OnChainOutput{}, nil
		}

		accounts, err := configAccounts(b.GetContext(), chain, input.CctpVerifier, ownerOf)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		instruction, err := cctp_token_pool.NewSetVerifierAllowedFinalityConfigInstruction(
			input.Finality,
			accounts.config, accounts.authority, accounts.eventAuthority, input.CctpVerifier,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("build set_verifier_allowed_finality_config instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, accounts.authority, []solana.Instruction{instruction}, input.CctpVerifier, ContractType.String())
	},
)

// SetFastFinalityBps sets the basis points charged for CCTP fast finality. initialize_verifier
// seeds this to 1 (0.01%), matching Circle's Solana fast fee, so only call it when the desired
// value differs from what is already on-chain.
var SetFastFinalityBps = operations.NewOperation(
	"cctp-verifier:set-fast-finality-bps",
	Version,
	"Configures the CCTP verifier's fast finality basis points",
	func(b operations.Bundle, chain cldf_solana.Chain, input SetFastFinalityBpsParams) (sequences.OnChainOutput, error) {
		if err := requireProgram(chain, input.CctpVerifier); err != nil {
			return sequences.OnChainOutput{}, err
		}
		if input.FastFinalityBps > MaxFastFinalityBps {
			return sequences.OnChainOutput{}, fmt.Errorf("fast finality bps %d exceeds the maximum of %d", input.FastFinalityBps, MaxFastFinalityBps)
		}

		cfg, err := readConfig(b.GetContext(), chain, input.CctpVerifier)
		if err == nil && cfg.FastFinalityBps == input.FastFinalityBps {
			b.Logger.Infow("CCTP verifier fast finality bps already set, skipping",
				"program", input.CctpVerifier.String(), "bps", input.FastFinalityBps)
			return sequences.OnChainOutput{}, nil
		}

		accounts, err := configAccounts(b.GetContext(), chain, input.CctpVerifier, ownerOf)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		instruction, err := cctp_token_pool.NewSetVerifierFastFinalityBpsInstruction(
			input.FastFinalityBps,
			accounts.config, accounts.authority, accounts.eventAuthority, input.CctpVerifier,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("build set_verifier_fast_finality_bps instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, accounts.authority, []solana.Instruction{instruction}, input.CctpVerifier, ContractType.String())
	},
)

var SetAllowlistAdmin = operations.NewOperation(
	"cctp-verifier:set-allowlist-admin",
	Version,
	"Configures the CCTP verifier's allowlist admin",
	func(b operations.Bundle, chain cldf_solana.Chain, input SetAllowlistAdminParams) (sequences.OnChainOutput, error) {
		if err := requireProgram(chain, input.CctpVerifier); err != nil {
			return sequences.OnChainOutput{}, err
		}
		if input.AllowlistAdmin.IsZero() {
			return sequences.OnChainOutput{}, errors.New("allowlist admin is required")
		}

		cfg, err := readConfig(b.GetContext(), chain, input.CctpVerifier)
		if err == nil && cfg.AllowlistAdmin == input.AllowlistAdmin {
			b.Logger.Infow("CCTP verifier allowlist admin already set, skipping",
				"program", input.CctpVerifier.String(), "allowlistAdmin", input.AllowlistAdmin.String())
			return sequences.OnChainOutput{}, nil
		}

		accounts, err := configAccounts(b.GetContext(), chain, input.CctpVerifier, ownerOf)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		instruction, err := cctp_token_pool.NewSetVerifierAllowlistAdminInstruction(
			input.AllowlistAdmin,
			accounts.config, accounts.authority, accounts.eventAuthority, input.CctpVerifier,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("build set_verifier_allowlist_admin instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, accounts.authority, []solana.Instruction{instruction}, input.CctpVerifier, ContractType.String())
	},
)

// SetStorageLocations overwrites the verifier's storage locations. The on-chain account holds
// opaque ByteStrings rather than Strings (the program only stores and echoes them), so the input
// strings are wrapped rather than passed through. Authorized by config.storage_location_admin.
var SetStorageLocations = operations.NewOperation(
	"cctp-verifier:set-storage-locations",
	Version,
	"Configures the CCTP verifier's storage locations",
	func(b operations.Bundle, chain cldf_solana.Chain, input SetStorageLocationsParams) (sequences.OnChainOutput, error) {
		if err := requireProgram(chain, input.CctpVerifier); err != nil {
			return sequences.OnChainOutput{}, err
		}

		storageLocationsPDA, _, err := state.FindCVStorageLocationsPDA(input.CctpVerifier)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("derive verifier storage locations PDA: %w", err)
		}

		locations := make([]cctp_token_pool.ByteString, 0, len(input.StorageLocations))
		for _, location := range input.StorageLocations {
			locations = append(locations, cctp_token_pool.ByteString{V0: []byte(location)})
		}

		var existing cctp_token_pool.StorageLocations
		if err = common.GetAccountDataBorshIntoAnchor(b.GetContext(), chain.Client, storageLocationsPDA, cldf_solana.SolDefaultCommitment, &existing); err == nil &&
			storageLocationsEqual(existing.Locations, locations) {
			b.Logger.Infow("CCTP verifier storage locations already set, skipping",
				"program", input.CctpVerifier.String())
			return sequences.OnChainOutput{}, nil
		}

		accounts, err := configAccounts(b.GetContext(), chain, input.CctpVerifier, storageLocationAdminOf)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		instruction, err := cctp_token_pool.NewSetVerifierStorageLocationsInstruction(
			cctp_token_pool.SetVerifierStorageLocationsParams{StorageLocations: locations},
			accounts.config, accounts.authority, storageLocationsPDA, solana.SystemProgramID,
			accounts.eventAuthority, input.CctpVerifier,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("build set_verifier_storage_locations instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, accounts.authority, []solana.Instruction{instruction}, input.CctpVerifier, ContractType.String())
	},
)

// SetChainConfig writes the chain-agnostic half of a remote chain's verifier config, creating the
// RemoteChainConfig PDA through init_verifier_chain_config or updating it through
// edit_verifier_chain_config. It must run before SetChainConfigCctp and SetAllowlist, which realloc
// the same PDA.
var SetChainConfig = operations.NewOperation(
	"cctp-verifier:set-chain-config",
	Version,
	"Configures a CCTP verifier remote chain",
	func(b operations.Bundle, chain cldf_solana.Chain, input SetChainConfigParams) (sequences.OnChainOutput, error) {
		if err := requireProgram(chain, input.CctpVerifier); err != nil {
			return sequences.OnChainOutput{}, err
		}
		// The program rejects a zero router signer (ZeroRouterSigner), so fail here with a
		// message that names the input rather than surfacing an opaque on-chain error.
		if input.RouterSigner.IsZero() {
			return sequences.OnChainOutput{}, errors.New("router signer is required")
		}

		remoteChainConfigPDA, _, err := state.FindCctpVerifierRemoteChainConfigPDA(input.RemoteChainSelector, input.CctpVerifier)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("derive verifier remote chain config PDA: %w", err)
		}
		adminConfig := cctp_token_pool.RemoteChainAdminConfig{
			Router:             input.RouterSigner,
			Fee:                cctp_token_pool.UsdCents{V0: input.FeeUSDCents},
			GasForVerification: cctp_token_pool.CrossChainGas{V0: input.GasForVerification},
			PayloadSizeBytes:   input.PayloadSizeBytes,
		}

		existing, err := common.GetInitializedAccount(b.GetContext(), chain.Client, remoteChainConfigPDA, input.CctpVerifier, cldf_solana.SolDefaultCommitment, cctp_token_pool.ParseAccount_RemoteChainConfig)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		exists := existing != nil
		if exists && existing.AdminConfig == adminConfig {
			b.Logger.Infow("CCTP verifier chain config already set, skipping",
				"program", input.CctpVerifier.String(), "remoteChainSelector", input.RemoteChainSelector)
			return sequences.OnChainOutput{}, nil
		}

		accounts, err := configAccounts(b.GetContext(), chain, input.CctpVerifier, ownerOf)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		params := cctp_token_pool.SetVerifierChainConfigParams{
			RemoteChainSelector: input.RemoteChainSelector,
			ChainConfig:         adminConfig,
		}
		var instruction solana.Instruction
		if exists {
			instruction, err = cctp_token_pool.NewEditVerifierChainConfigInstruction(
				params, accounts.config, accounts.authority, remoteChainConfigPDA,
				accounts.eventAuthority, input.CctpVerifier,
			)
		} else {
			instruction, err = cctp_token_pool.NewInitVerifierChainConfigInstruction(
				params, accounts.config, accounts.authority, remoteChainConfigPDA, solana.SystemProgramID,
				accounts.eventAuthority, input.CctpVerifier,
			)
		}
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("build verifier chain config instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, accounts.authority, []solana.Instruction{instruction}, input.CctpVerifier, ContractType.String())
	},
)

// SetChainConfigCctp writes the CCTP-specific half of a remote chain's verifier config. Requires
// SetChainConfig to have created the RemoteChainConfig PDA first: this instruction reallocs it
// with realloc::zero = true, sizing from the allowlist's current contents.
var SetChainConfigCctp = operations.NewOperation(
	"cctp-verifier:set-chain-config-cctp",
	Version,
	"Configures the CCTP-specific settings of a CCTP verifier remote chain",
	func(b operations.Bundle, chain cldf_solana.Chain, input SetChainConfigCctpParams) (sequences.OnChainOutput, error) {
		if err := requireProgram(chain, input.CctpVerifier); err != nil {
			return sequences.OnChainOutput{}, err
		}
		if input.AllowedCallerReceive.IsZero() {
			return sequences.OnChainOutput{}, errors.New("allowed caller receive is required")
		}
		if input.AllowedCallerDeposit.IsZero() {
			return sequences.OnChainOutput{}, errors.New("allowed caller deposit is required")
		}

		remoteChainConfigPDA, _, err := state.FindCctpVerifierRemoteChainConfigPDA(input.RemoteChainSelector, input.CctpVerifier)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("derive verifier remote chain config PDA: %w", err)
		}
		cctpConfig := cctp_token_pool.RemoteCctpConfig{
			Enabled:              input.Enabled,
			DomainIdentifier:     input.DomainIdentifier,
			AllowedCallerReceive: input.AllowedCallerReceive,
			AllowedCallerDeposit: input.AllowedCallerDeposit,
			MintRecipientOnDest:  input.MintRecipientOnDest,
			LookupTable:          input.LookupTable,
			SupportedTokens:      input.SupportedTokens,
		}

		var existing cctp_token_pool.RemoteChainConfig
		if err = common.GetAccountDataBorshIntoAnchor(b.GetContext(), chain.Client, remoteChainConfigPDA, cldf_solana.SolDefaultCommitment, &existing); err == nil &&
			remoteCctpConfigEqual(existing.Cctp, cctpConfig) {
			b.Logger.Infow("CCTP verifier chain cctp config already set, skipping",
				"program", input.CctpVerifier.String(), "remoteChainSelector", input.RemoteChainSelector)
			return sequences.OnChainOutput{}, nil
		}

		accounts, err := configAccounts(b.GetContext(), chain, input.CctpVerifier, ownerOf)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		instruction, err := cctp_token_pool.NewSetVerifierChainConfigCctpInstruction(
			cctp_token_pool.SetVerifierChainConfigCctpParams{
				RemoteChainSelector: input.RemoteChainSelector,
				Cctp:                cctpConfig,
			},
			accounts.config, accounts.authority, remoteChainConfigPDA, solana.SystemProgramID,
			accounts.eventAuthority, input.CctpVerifier,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("build set_verifier_chain_config_cctp instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, accounts.authority, []solana.Instruction{instruction}, input.CctpVerifier, ContractType.String())
	},
)

// SetAllowlist applies sender allowlist changes to a lane. set_verifier_allowlist replaces the
// whole on-chain allowlist, so the changes are applied to the lane's current one first, and
// nothing is sent when the result matches it. See allowlist.Signer for who signs.
var SetAllowlist = operations.NewOperation(
	"cctp-verifier:set-allowlist",
	Version,
	"Sets the CCTP verifier sender allowlist for a destination chain",
	func(b operations.Bundle, chain cldf_solana.Chain, input SetAllowlistParams) (sequences.OnChainOutput, error) {
		if err := requireProgram(chain, input.CctpVerifier); err != nil {
			return sequences.OnChainOutput{}, err
		}

		current, err := readAllowlist(b.GetContext(), chain.Client, input.CctpVerifier, input.DestChainSelector)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		senders, changed, err := allowlist.Update(current, input.Enabled, input.AllowedSendersToAdd, input.AllowedSendersToRemove)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("invalid allowlist for remote chain %d: %w", input.DestChainSelector, err)
		}
		if !changed {
			b.Logger.Infof("CCTP verifier allowlist for remote chain %d is up to date", input.DestChainSelector)
			return sequences.OnChainOutput{}, nil
		}

		accounts, err := configAccounts(b.GetContext(), chain, input.CctpVerifier, allowlistAdminOf)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		owner, err := resolveAuthority(b.GetContext(), chain, input.CctpVerifier, ownerOf)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		accounts.authority, err = allowlist.Signer(current, owner, accounts.authority, chain.Deployer().PublicKey())
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("remote chain %d on CCTP verifier %s: %w", input.DestChainSelector, input.CctpVerifier, err)
		}
		remoteChainConfigPDA, _, err := state.FindCctpVerifierRemoteChainConfigPDA(input.DestChainSelector, input.CctpVerifier)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("derive verifier remote chain config PDA: %w", err)
		}
		instruction, err := cctp_token_pool.NewSetVerifierAllowlistInstruction(
			cctp_token_pool.SetVerifierAllowlistParams{
				DestChainSelector: input.DestChainSelector,
				Enabled:           input.Enabled,
				AllowedSenders:    senders,
			},
			accounts.config, accounts.authority, remoteChainConfigPDA, solana.SystemProgramID,
			accounts.eventAuthority, input.CctpVerifier,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("build set_verifier_allowlist instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, accounts.authority, []solana.Instruction{instruction}, input.CctpVerifier, ContractType.String())
	},
)

var TransferOwnership = operations.NewOperation(
	"cctp-verifier:transfer-ownership",
	Version,
	"Proposes a new owner for the CCTP verifier",
	func(b operations.Bundle, chain cldf_solana.Chain, input TransferOwnershipParams) (sequences.OnChainOutput, error) {
		if err := requireProgram(chain, input.CctpVerifier); err != nil {
			return sequences.OnChainOutput{}, err
		}
		if input.ProposedOwner.IsZero() {
			return sequences.OnChainOutput{}, errors.New("proposed owner is required")
		}

		cfg, err := readConfig(b.GetContext(), chain, input.CctpVerifier)
		if err == nil && (cfg.Owner == input.ProposedOwner || cfg.ProposedOwner == input.ProposedOwner) {
			b.Logger.Infow("CCTP verifier ownership already transferred or proposed, skipping",
				"program", input.CctpVerifier.String(), "proposedOwner", input.ProposedOwner.String())
			return sequences.OnChainOutput{}, nil
		}

		accounts, err := configAccounts(b.GetContext(), chain, input.CctpVerifier, ownerOf)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		instruction, err := cctp_token_pool.NewTransferVerifierOwnershipInstruction(
			input.ProposedOwner,
			accounts.config, accounts.authority, accounts.eventAuthority, input.CctpVerifier,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("build transfer_verifier_ownership instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, accounts.authority, []solana.Instruction{instruction}, input.CctpVerifier, ContractType.String())
	},
)

// AcceptOwnership completes the two-step handover. The signer is the proposed owner: it comes
// from the input when set (the transfer may be batched in the same proposal, so the pending
// owner is not readable on chain yet), and is read off-chain otherwise.
var AcceptOwnership = operations.NewOperation(
	"cctp-verifier:accept-ownership",
	Version,
	"Accepts a pending CCTP verifier ownership transfer",
	func(b operations.Bundle, chain cldf_solana.Chain, input AcceptOwnershipParams) (sequences.OnChainOutput, error) {
		if err := requireProgram(chain, input.CctpVerifier); err != nil {
			return sequences.OnChainOutput{}, err
		}

		authority := input.ProposedOwner
		if authority.IsZero() {
			accounts, err := configAccounts(b.GetContext(), chain, input.CctpVerifier, proposedOwnerOf)
			if err != nil {
				return sequences.OnChainOutput{}, err
			}
			if accounts.authority.IsZero() {
				b.Logger.Infow("CCTP verifier has no pending ownership transfer, skipping",
					"program", input.CctpVerifier.String())
				return sequences.OnChainOutput{}, nil
			}
			authority = accounts.authority
		}
		configPDA, _, err := state.FindCVConfigPDA(input.CctpVerifier)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("derive verifier config PDA: %w", err)
		}
		eventAuthority, _, err := state.FindEventAuthorityPDA(input.CctpVerifier)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("derive verifier event authority PDA: %w", err)
		}
		instruction, err := cctp_token_pool.NewAcceptVerifierOwnershipInstruction(
			configPDA, authority, eventAuthority, input.CctpVerifier,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("build accept_verifier_ownership instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{instruction}, input.CctpVerifier, ContractType.String())
	},
)

var TransferStorageLocationAdmin = operations.NewOperation(
	"cctp-verifier:transfer-storage-location-admin",
	Version,
	"Proposes a new storage location admin for the CCTP verifier",
	func(b operations.Bundle, chain cldf_solana.Chain, input TransferStorageLocationAdminParams) (sequences.OnChainOutput, error) {
		if err := requireProgram(chain, input.CctpVerifier); err != nil {
			return sequences.OnChainOutput{}, err
		}
		if input.ProposedStorageLocationAdmin.IsZero() {
			return sequences.OnChainOutput{}, errors.New("proposed storage location admin is required")
		}

		cfg, err := readConfig(b.GetContext(), chain, input.CctpVerifier)
		if err == nil && (cfg.StorageLocationAdmin == input.ProposedStorageLocationAdmin ||
			cfg.ProposedStorageLocationAdmin == input.ProposedStorageLocationAdmin) {
			b.Logger.Infow("CCTP verifier storage location admin already transferred or proposed, skipping",
				"program", input.CctpVerifier.String())
			return sequences.OnChainOutput{}, nil
		}

		accounts, err := configAccounts(b.GetContext(), chain, input.CctpVerifier, storageLocationAdminOf)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		instruction, err := cctp_token_pool.NewTransferVerifierStorageLocationAdminInstruction(
			input.ProposedStorageLocationAdmin,
			accounts.config, accounts.authority, accounts.eventAuthority, input.CctpVerifier,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("build transfer_verifier_storage_location_admin instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, accounts.authority, []solana.Instruction{instruction}, input.CctpVerifier, ContractType.String())
	},
)

var AcceptStorageLocationAdmin = operations.NewOperation(
	"cctp-verifier:accept-storage-location-admin",
	Version,
	"Accepts a pending CCTP verifier storage location admin transfer",
	func(b operations.Bundle, chain cldf_solana.Chain, input Params) (sequences.OnChainOutput, error) {
		if err := requireProgram(chain, input.CctpVerifier); err != nil {
			return sequences.OnChainOutput{}, err
		}

		accounts, err := configAccounts(b.GetContext(), chain, input.CctpVerifier, proposedStorageLocationAdminOf)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		if accounts.authority.IsZero() {
			b.Logger.Infow("CCTP verifier has no pending storage location admin transfer, skipping",
				"program", input.CctpVerifier.String())
			return sequences.OnChainOutput{}, nil
		}
		instruction, err := cctp_token_pool.NewAcceptVerifierStorageLocationAdminInstruction(
			accounts.config, accounts.authority, accounts.eventAuthority, input.CctpVerifier,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("build accept_verifier_storage_location_admin instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, accounts.authority, []solana.Instruction{instruction}, input.CctpVerifier, ContractType.String())
	},
)

// requireProgram performs the preamble every non-deploy operation shares: rebind the gobinding's
// compile-time ProgramID to the deployed address, and reject a missing deployer key or program.
func requireProgram(chain cldf_solana.Chain, program solana.PublicKey) error {
	if program.IsZero() {
		return errors.New("cctp verifier program is required")
	}
	solcommon.SetProgramID(&cctp_token_pool.ProgramID, cctp_token_pool.ParseInstructionWithoutAccounts, program)
	if chain.Deployer() == nil {
		return errors.New("missing Solana deployer key")
	}
	return nil
}

// verifierAccounts are the three accounts every verifier admin instruction takes beyond its own
// operands: the config PDA, the signing authority, and the program's event authority.
type verifierAccounts struct {
	config         solana.PublicKey
	authority      solana.PublicKey
	eventAuthority solana.PublicKey
}

// authorityFn selects which Config field authorizes an instruction.
type authorityFn func(cctp_token_pool.Config) solana.PublicKey

func ownerOf(c cctp_token_pool.Config) solana.PublicKey          { return c.Owner }
func proposedOwnerOf(c cctp_token_pool.Config) solana.PublicKey  { return c.ProposedOwner }
func allowlistAdminOf(c cctp_token_pool.Config) solana.PublicKey { return c.AllowlistAdmin }
func storageLocationAdminOf(c cctp_token_pool.Config) solana.PublicKey {
	return c.StorageLocationAdmin
}

func proposedStorageLocationAdminOf(c cctp_token_pool.Config) solana.PublicKey {
	return c.ProposedStorageLocationAdmin
}

func configAccounts(ctx context.Context, chain cldf_solana.Chain, program solana.PublicKey, pick authorityFn) (verifierAccounts, error) {
	configPDA, _, err := state.FindCVConfigPDA(program)
	if err != nil {
		return verifierAccounts{}, fmt.Errorf("derive verifier config PDA: %w", err)
	}
	eventAuthority, _, err := state.FindEventAuthorityPDA(program)
	if err != nil {
		return verifierAccounts{}, fmt.Errorf("derive verifier event authority PDA: %w", err)
	}
	authority, err := resolveAuthority(ctx, chain, program, pick)
	if err != nil {
		return verifierAccounts{}, err
	}
	return verifierAccounts{
		config:         configPDA,
		authority:      authority,
		eventAuthority: eventAuthority,
	}, nil
}

// resolveAuthority returns the key pick selects from the verifier config, or the deployer key
// before the verifier is initialized.
func resolveAuthority(ctx context.Context, chain cldf_solana.Chain, program solana.PublicKey, pick authorityFn) (solana.PublicKey, error) {
	configPDA, _, err := state.FindCVConfigPDA(program)
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("derive verifier config PDA: %w", err)
	}
	return solcommon.ConfigAuthority(ctx, chain, program, configPDA, cctp_token_pool.ParseAccount_Config, func(c *cctp_token_pool.Config) solana.PublicKey { return pick(*c) })
}

func readConfig(ctx context.Context, chain cldf_solana.Chain, program solana.PublicKey) (cctp_token_pool.Config, error) {
	configPDA, _, err := state.FindCVConfigPDA(program)
	if err != nil {
		return cctp_token_pool.Config{}, fmt.Errorf("derive verifier config PDA: %w", err)
	}
	var cfg cctp_token_pool.Config
	if err := common.GetAccountDataBorshIntoAnchor(ctx, chain.Client, configPDA, cldf_solana.SolDefaultCommitment, &cfg); err != nil {
		return cctp_token_pool.Config{}, fmt.Errorf("read verifier config: %w", err)
	}
	return cfg, nil
}

func storageLocationsEqual(a, b []cctp_token_pool.ByteString) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if string(a[i].V0) != string(b[i].V0) {
			return false
		}
	}
	return true
}

func remoteCctpConfigEqual(a, b cctp_token_pool.RemoteCctpConfig) bool {
	if a.Enabled != b.Enabled ||
		a.DomainIdentifier != b.DomainIdentifier ||
		a.AllowedCallerReceive != b.AllowedCallerReceive ||
		a.AllowedCallerDeposit != b.AllowedCallerDeposit ||
		!pubkeyPtrEqual(a.MintRecipientOnDest, b.MintRecipientOnDest) ||
		!pubkeyPtrEqual(a.LookupTable, b.LookupTable) ||
		len(a.SupportedTokens) != len(b.SupportedTokens) {
		return false
	}
	for i := range a.SupportedTokens {
		if a.SupportedTokens[i].Local != b.SupportedTokens[i].Local ||
			!bytesEqual(a.SupportedTokens[i].Remote.Address, b.SupportedTokens[i].Remote.Address) {
			return false
		}
	}
	return true
}

func pubkeyPtrEqual(a, b *solana.PublicKey) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
