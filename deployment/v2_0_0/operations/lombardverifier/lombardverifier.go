// Package lombardverifier holds CLDF operations for Lombard's embedded verifier. Unlike
// committee_verifier, Lombard's verifier isn't a separate on-chain program - it's a second set of
// instructions/PDAs inside the lombard_token_pool program (see the plan's decision to keep pool
// and verifier in one program). Deploying/upgrading the program itself is therefore the token
// pool's job (tokenpools.DeployLombard/UpgradeLombard); this package only covers the verifier's
// own state (VerifierConfig/VerifierRemoteChainConfig/VerifierStorageLocations).
package lombardverifier

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"strconv"

	"github.com/Masterminds/semver/v3"
	"github.com/gagliardetto/solana-go"

	"github.com/smartcontractkit/chainlink-ccip/deployment/finality"
	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/sequences"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	cldf_deployment "github.com/smartcontractkit/chainlink-deployments-framework/deployment"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/lombard_token_pool"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/tokens"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/allowlist"
)

var (
	ContractType    cldf_deployment.ContractType = "LombardVerifier"
	Version         *semver.Version              = solcommon.Version2_0_0
	LookupTableType cldf_deployment.ContractType = "LombardVerifierLookupTable"
	// LombardVerifierResolverQualifier mirrors the EVM convention
	// (chains/evm versioned_verifier_resolver.LombardVerifierResolverType): the pool program is
	// dual-registered under it so cross-family consumers (token-verifier, indexer) resolve one
	// qualifier on both chains.
	LombardVerifierResolverQualifier = "LombardVerifierResolver"
)

type Params struct {
	LombardVerifier solana.PublicKey
}

type LookupTableParams struct {
	ExistingAddresses   []datastore.AddressRef
	LombardVerifier     solana.PublicKey
	RemoteChainSelector uint64
}

type ExtendLookupTableParams struct {
	LookupTable         solana.PublicKey
	LombardVerifier     solana.PublicKey
	RemoteChainSelector uint64
	Entries             []solana.PublicKey
}

type SetFeeAggregatorParams struct {
	LombardVerifier solana.PublicKey
	FeeAggregator   solana.PublicKey
}

type SetStorageLocationsParams struct {
	LombardVerifier  solana.PublicKey
	StorageLocations []string
}

type SetAllowedFinalityConfigParams struct {
	LombardVerifier solana.PublicKey
	Finality        lombard_token_pool.FinalityConfig
}

type SetAllowlistAdminParams struct {
	LombardVerifier solana.PublicKey
	AllowlistAdmin  solana.PublicKey
}

// SetChainConfigParams mirrors committeeverifier.SetChainConfigParams.
type SetChainConfigParams struct {
	LombardVerifier     solana.PublicKey
	RemoteChainSelector uint64
	RouterSigner        solana.PublicKey
	FeeUSDCents         uint16
	GasForVerification  uint32
	PayloadSizeBytes    uint32
}

// SetPathParams sets Lombard's own GMP-routing path for a remote chain (l_chain_id/
// allowed_caller/remote_bridge_sender) - no equivalent on committee_verifier.
type SetPathParams struct {
	LombardVerifier     solana.PublicKey
	RemoteChainSelector uint64
	LChainID            [32]byte
	AllowedCaller       [32]byte
	RemoteBridgeSender  [32]byte
}

type RemovePathParams struct {
	LombardVerifier     solana.PublicKey
	RemoteChainSelector uint64
}

// SetAllowlistParams describes changes to a lane's sender allowlist. Enabled sets the flag, and
// the senders to add and to remove are applied to the lane's current list.
type SetAllowlistParams struct {
	LombardVerifier        solana.PublicKey
	DestChainSelector      uint64
	Enabled                bool
	AllowedSendersToAdd    []solana.PublicKey
	AllowedSendersToRemove []solana.PublicKey
}

type SetSupportedTokensParams struct {
	LombardVerifier solana.PublicKey
	TokensToRemove  []solana.PublicKey
	TokensToSet     []lombard_token_pool.SupportedToken
}

type SetRemoteAdaptersParams struct {
	LombardVerifier     solana.PublicKey
	RemoteChainSelector uint64
	RemoteAdapters      []lombard_token_pool.RemoteAdapter
}

// FinalityConfigFromCommon converts the chain-agnostic finality.Config into the on-chain
// flags/block-depth split. Wire format is identical to committee_verifier's - see
// committeeverifier.FinalityConfigFromCommon's doc comment for the byte layout.
func FinalityConfigFromCommon(cfg finality.Config) lombard_token_pool.FinalityConfig {
	raw := cfg.Raw()
	return lombard_token_pool.FinalityConfig{
		Flags:      binary.BigEndian.Uint16(raw[:2]),
		BlockDepth: binary.BigEndian.Uint16(raw[2:]),
	}
}

var Initialize = operations.NewOperation(
	"lombard-verifier:initialize",
	Version,
	"Initializes Lombard's embedded verifier state",
	func(b operations.Bundle, chain cldf_solana.Chain, input Params) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&lombard_token_pool.ProgramID, lombard_token_pool.ParseInstructionWithoutAccounts, input.LombardVerifier)
		if chain.Deployer() == nil {
			return sequences.OnChainOutput{}, errors.New("missing Solana deployer key")
		}
		if input.LombardVerifier.IsZero() {
			return sequences.OnChainOutput{}, errors.New("lombard verifier program is required")
		}

		configPDA, _, err := state.FindLVConfigPDA(input.LombardVerifier)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive verifier config PDA: %w", err)
		}
		initialized, err := common.IsAccountInitialized(b.GetContext(), chain.Client, configPDA, input.LombardVerifier, cldf_solana.SolDefaultCommitment, lombard_token_pool.Account_VerifierConfig)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		if initialized {
			b.Logger.Info("Lombard verifier already initialized for program:", input.LombardVerifier.String())
			return sequences.OnChainOutput{}, nil
		}

		programData, err := solcommon.GetSolProgramData(chain.Client, input.LombardVerifier)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to get program data: %w", err)
		}
		storageLocationsPDA, _, err := state.FindLVStorageLocationsPDA(input.LombardVerifier)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive verifier storage locations PDA: %w", err)
		}

		authority := chain.Deployer().PublicKey()
		instruction, err := lombard_token_pool.NewInitializeVerifierInstruction(
			configPDA,
			storageLocationsPDA,
			authority,
			solana.SystemProgramID,
			input.LombardVerifier,
			programData.Address,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to build initialize_verifier instruction: %w", err)
		}
		if err := chain.Confirm([]solana.Instruction{instruction}); err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to confirm initialize_verifier: %w", err)
		}
		return sequences.OnChainOutput{}, nil
	},
)

// TransferOwnership proposes a new owner for the Lombard verifier. The verifier lives inside the
// lombard_token_pool program; this transfers the verifier authority, not the pool ownership (pool
// ownership moves through UpdateAuthorities).
var TransferOwnership = operations.NewOperation(
	"lombard-verifier:transfer-ownership",
	Version,
	"Transfers the Lombard verifier authority to a new owner",
	func(b operations.Bundle, chain cldf_solana.Chain, input solcommon.TransferOwnershipParams) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&lombard_token_pool.ProgramID, lombard_token_pool.ParseInstructionWithoutAccounts, input.Program)
		authority, err := getOwner(b.GetContext(), chain, input.Program)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		if authority != input.CurrentOwner {
			return sequences.OnChainOutput{}, fmt.Errorf("current owner %s does not match on-chain authority %s", input.CurrentOwner.String(), authority.String())
		}
		configPDA, _, err := state.FindLVConfigPDA(input.Program)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive verifier config PDA: %w", err)
		}
		ixn, err := lombard_token_pool.NewTransferVerifierOwnershipInstruction(
			input.NewOwner,
			configPDA,
			authority,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to build transfer verifier ownership instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{ixn}, input.Program, ContractType.String())
	},
)

// AcceptOwnership completes the verifier handover, signed by the proposed owner.
var AcceptOwnership = operations.NewOperation(
	"lombard-verifier:accept-ownership",
	Version,
	"Accepts a pending Lombard verifier ownership transfer",
	func(b operations.Bundle, chain cldf_solana.Chain, input solcommon.TransferOwnershipParams) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&lombard_token_pool.ProgramID, lombard_token_pool.ParseInstructionWithoutAccounts, input.Program)
		configPDA, _, err := state.FindLVConfigPDA(input.Program)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive verifier config PDA: %w", err)
		}
		ixn, err := lombard_token_pool.NewAcceptVerifierOwnershipInstruction(
			configPDA,
			input.NewOwner,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to build accept verifier ownership instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, input.NewOwner, []solana.Instruction{ixn}, input.Program, ContractType.String())
	},
)

// CreateAndSetLookupTable reuses a prepared table when verifier ownership is governed.
var CreateAndSetLookupTable = operations.NewOperation("lombard-verifier:create-and-set-lookup-table",
	Version,
	"Creates and registers a Lombard verifier remote chain's own address lookup table",
	func(b operations.Bundle, chain cldf_solana.Chain, input LookupTableParams) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&lombard_token_pool.ProgramID, lombard_token_pool.ParseInstructionWithoutAccounts, input.LombardVerifier)
		if chain.Deployer() == nil {
			return sequences.OnChainOutput{}, errors.New("missing Solana deployer key")
		}

		configPDA, _, err := state.FindLVConfigPDA(input.LombardVerifier)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive verifier config PDA: %w", err)
		}
		remoteChainConfigPDA, _, err := state.FindLVRemoteChainConfigPDA(input.RemoteChainSelector, input.LombardVerifier)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive verifier remote chain config PDA: %w", err)
		}
		table, err := getLookupTable(b.GetContext(), chain, input.LombardVerifier, input.RemoteChainSelector)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		ref := datastore.AddressRef{ChainSelector: chain.Selector, Type: datastore.ContractType(LookupTableType), Version: Version, Qualifier: strconv.FormatUint(input.RemoteChainSelector, 10)}
		if !table.IsZero() {
			ref.Address = table.String()
			return sequences.OnChainOutput{Addresses: []datastore.AddressRef{ref}}, nil
		}
		authority, err := getOwner(b.GetContext(), chain, input.LombardVerifier)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		table, err = solcommon.PubkeyFromRefs(input.ExistingAddresses, datastore.NewAddressRefKey(chain.Selector, ref.Type, Version, ref.Qualifier))
		if err != nil && !errors.Is(err, solcommon.ErrAddressRefNotFound) {
			return sequences.OnChainOutput{}, err
		}
		table, err = solcommon.PreparedLookupTable(b.GetContext(), chain, table)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		instruction, err := lombard_token_pool.NewSetVerifierLookupTableInstruction(
			input.RemoteChainSelector,
			&table,
			configPDA,
			authority,
			remoteChainConfigPDA,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to build set_verifier_lookup_table instruction: %w", err)
		}
		out, err := solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{instruction}, input.LombardVerifier, ContractType.String())
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		out.Addresses = append(out.Addresses, datastore.AddressRef{
			Address:       table.String(),
			ChainSelector: chain.Selector,
			Type:          datastore.ContractType(LookupTableType),
			Version:       Version,
			Qualifier:     strconv.FormatUint(input.RemoteChainSelector, 10),
		})
		return out, nil
	},
)

func extendLookupTable(ctx context.Context, chain cldf_solana.Chain, verifier solana.PublicKey, remote uint64, table solana.PublicKey, entries []solana.PublicKey) (sequences.OnChainOutput, error) {
	if chain.Deployer() == nil {
		return sequences.OnChainOutput{}, errors.New("missing Solana deployer key")
	}
	if table.IsZero() {
		var err error
		table, err = getLookupTable(ctx, chain, verifier, remote)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
	}
	if table.IsZero() {
		return sequences.OnChainOutput{}, fmt.Errorf("lombard verifier %s remote chain %d has no lookup table set or supplied", verifier, remote)
	}
	return sequences.OnChainOutput{}, solcommon.ExtendLookupTable(chain, table, entries)
}

// ExtendLookupTable extends one remote chain's own address lookup table with caller-supplied
// entries, e.g. the pool/verifier-config addresses configure_lombard_chain_for_lanes.go wires in
// right after CreateAndSetLookupTable.
var ExtendLookupTable = operations.NewOperation(
	"lombard-verifier:extend-lookup-table",
	Version,
	"Extends a Lombard verifier remote chain's own address lookup table with new entries",
	func(b operations.Bundle, chain cldf_solana.Chain, input ExtendLookupTableParams) (sequences.OnChainOutput, error) {
		return extendLookupTable(b.GetContext(), chain, input.LombardVerifier, input.RemoteChainSelector, input.LookupTable, input.Entries)
	},
)

type ExtendLookupTableForLaneParams struct {
	LookupTable         solana.PublicKey
	LombardVerifier     solana.PublicKey
	Bridge              solana.PublicKey
	Mailbox             solana.PublicKey
	Router              solana.PublicKey
	Mint                solana.PublicKey
	RemoteChainSelector uint64
	LChainID            [32]byte
}

var ExtendLookupTableForLane = operations.NewOperation(
	"lombard-verifier:extend-lookup-table-for-lane",
	Version,
	"Extends a Lombard verifier remote chain's own lookup table with accounts needed by that lane's resolution",
	func(b operations.Bundle, chain cldf_solana.Chain, input ExtendLookupTableForLaneParams) (sequences.OnChainOutput, error) {
		tokenAuthorityPDA, _, err := solana.FindProgramAddress([][]byte{[]byte("token_authority")}, input.Bridge)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("derive bridge token authority PDA: %w", err)
		}
		bridgeConfigPDA, _, err := solana.FindProgramAddress([][]byte{[]byte("bridge_config")}, input.Bridge)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("derive bridge config PDA: %w", err)
		}
		mailboxConfigPDA, _, err := solana.FindProgramAddress([][]byte{[]byte("mailbox_config")}, input.Mailbox)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("derive mailbox config PDA: %w", err)
		}
		ccvSignerPDA, _, err := state.FindCCVSignerPDA(input.LombardVerifier, input.Router)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("derive ccv_signer PDA: %w", err)
		}
		eventAuthorityPDA, _, err := state.FindEventAuthorityPDA(input.LombardVerifier)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("derive event authority PDA: %w", err)
		}
		poolSignerPDA, _, err := solana.FindProgramAddress(
			[][]byte{[]byte("ccip_tokenpool_signer"), input.Mint.Bytes()}, input.LombardVerifier,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("derive pool signer PDA: %w", err)
		}
		bridgeSenderConfigPDA, _, err := solana.FindProgramAddress(
			[][]byte{[]byte("sender_config"), poolSignerPDA.Bytes()}, input.Bridge,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("derive bridge sender config PDA: %w", err)
		}
		mailboxSenderConfigPDA, _, err := solana.FindProgramAddress(
			[][]byte{[]byte("sender_config"), bridgeConfigPDA.Bytes()}, input.Mailbox,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("derive mailbox sender config PDA: %w", err)
		}
		localTokenConfigPDA, _, err := solana.FindProgramAddress(
			[][]byte{[]byte("local_token_config"), input.Mint.Bytes()}, input.Bridge,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("derive local token config PDA: %w", err)
		}

		remoteBridgeConfigPDA, _, err := solana.FindProgramAddress(
			[][]byte{[]byte("remote_bridge_config"), input.LChainID[:]}, input.Bridge,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("derive remote bridge config PDA: %w", err)
		}
		remoteChainConfigPDA, _, err := state.FindLVRemoteChainConfigPDA(input.RemoteChainSelector, input.LombardVerifier)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("derive verifier remote chain config PDA: %w", err)
		}
		// inbound_message_path (verify_stage2) and outbound_message_path
		// (start_verification_at_source stage2) are both keyed only by the remote chain's Lombard
		// chain ID, not by mint - global across every token this verifier might support for this
		// remote, but derived here alongside the other per-remote entries since both derive from
		// the remote chain's LChainID, not from the mint.
		inboundMessagePathPDA, _, err := solana.FindProgramAddress(
			[][]byte{[]byte("inbound_message_path"), input.LChainID[:]}, input.Mailbox,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("derive inbound message path PDA: %w", err)
		}
		outboundMessagePathPDA, _, err := solana.FindProgramAddress(
			[][]byte{[]byte("outbound_message_path"), input.LChainID[:]}, input.Mailbox,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("derive outbound message path PDA: %w", err)
		}
		remoteTokenConfigPDA, _, err := solana.FindProgramAddress(
			[][]byte{[]byte("remote_token_config"), input.Mint.Bytes(), input.LChainID[:]}, input.Bridge,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("derive remote token config PDA: %w", err)
		}
		// remote_token_config, chain_config, chain_config_v2 and chain_config_override are all
		// keyed by mint AND remote chain selector, but they must NOT go in the token's own TAR
		// lookup table (router:set-pool's TokenPoolLookupTableType): both ccip-router's onramp
		// resolution and ccip-offramp's execute resolution deserialize that LUT on-chain and
		// replay its *entire* contents verbatim into their own accounts_to_save, for BOTH
		// ccip_send and execute_v2 alike - it's not a passive byte-savings aid, its contents are
		// the authoritative per-pool CPI account list for both directions. Putting a
		// Solana2EVM-only account there also force-feeds it into every EVM2Solana execute_v2 call
		// for this mint, and chain_config/chain_config_v2/chain_config_override are independently
		// re-derived via find() by BOTH resolvers before that replay, so adding them there would
		// additionally duplicate them in accounts_to_save.
		chainConfigPDA, _, err := tokens.TokenPoolChainConfigPDA(input.RemoteChainSelector, input.Mint, input.LombardVerifier)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("derive pool chain config PDA: %w", err)
		}
		chainConfigV2PDA, _, err := tokens.TokenPoolChainConfigV2PDA(input.RemoteChainSelector, input.Mint, input.LombardVerifier)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("derive pool chain config v2 PDA: %w", err)
		}
		chainConfigOverridePDA, _, err := tokens.TokenPoolChainConfigOverridePDA(input.RemoteChainSelector, input.Mint, input.LombardVerifier, []byte("ftf"))
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("derive pool chain config override PDA: %w", err)
		}

		return extendLookupTable(b.GetContext(), chain, input.LombardVerifier, input.RemoteChainSelector, input.LookupTable, []solana.PublicKey{
			input.Bridge, input.Mailbox, bridgeConfigPDA, mailboxConfigPDA, tokenAuthorityPDA,
			ccvSignerPDA, mailboxSenderConfigPDA, eventAuthorityPDA,
			localTokenConfigPDA, bridgeSenderConfigPDA, solana.SystemProgramID,
			remoteBridgeConfigPDA, remoteChainConfigPDA, inboundMessagePathPDA, outboundMessagePathPDA,
			remoteTokenConfigPDA, chainConfigPDA, chainConfigV2PDA, chainConfigOverridePDA,
		})
	},
)

var SetFeeAggregator = operations.NewOperation(
	"lombard-verifier:set-fee-aggregator",
	Version,
	"Configures Lombard verifier's fee aggregator",
	func(b operations.Bundle, chain cldf_solana.Chain, input SetFeeAggregatorParams) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&lombard_token_pool.ProgramID, lombard_token_pool.ParseInstructionWithoutAccounts, input.LombardVerifier)
		if chain.Deployer() == nil {
			return sequences.OnChainOutput{}, errors.New("missing Solana deployer key")
		}
		if input.FeeAggregator.IsZero() {
			return sequences.OnChainOutput{}, errors.New("fee aggregator is required")
		}

		configPDA, _, err := state.FindLVConfigPDA(input.LombardVerifier)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive verifier config PDA: %w", err)
		}
		authority, err := getOwner(b.GetContext(), chain, input.LombardVerifier)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		instruction, err := lombard_token_pool.NewSetVerifierFeeAggregatorInstruction(
			input.FeeAggregator,
			configPDA,
			authority,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to build set_verifier_fee_aggregator instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{instruction}, input.LombardVerifier, ContractType.String())
	},
)

var SetAllowedFinalityConfig = operations.NewOperation(
	"lombard-verifier:set-allowed-finality-config",
	Version,
	"Configures Lombard verifier's allowed finality",
	func(b operations.Bundle, chain cldf_solana.Chain, input SetAllowedFinalityConfigParams) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&lombard_token_pool.ProgramID, lombard_token_pool.ParseInstructionWithoutAccounts, input.LombardVerifier)
		if chain.Deployer() == nil {
			return sequences.OnChainOutput{}, errors.New("missing Solana deployer key")
		}

		configPDA, _, err := state.FindLVConfigPDA(input.LombardVerifier)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive verifier config PDA: %w", err)
		}
		authority, err := getOwner(b.GetContext(), chain, input.LombardVerifier)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		instruction, err := lombard_token_pool.NewSetVerifierAllowedFinalityConfigInstruction(
			input.Finality,
			configPDA,
			authority,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to build set_verifier_allowed_finality_config instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{instruction}, input.LombardVerifier, ContractType.String())
	},
)

var SetAllowlistAdmin = operations.NewOperation(
	"lombard-verifier:set-allowlist-admin",
	Version,
	"Configures Lombard verifier's allowlist admin",
	func(b operations.Bundle, chain cldf_solana.Chain, input SetAllowlistAdminParams) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&lombard_token_pool.ProgramID, lombard_token_pool.ParseInstructionWithoutAccounts, input.LombardVerifier)
		if chain.Deployer() == nil {
			return sequences.OnChainOutput{}, errors.New("missing Solana deployer key")
		}
		if input.AllowlistAdmin.IsZero() {
			return sequences.OnChainOutput{}, errors.New("allowlist admin is required")
		}

		configPDA, _, err := state.FindLVConfigPDA(input.LombardVerifier)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive verifier config PDA: %w", err)
		}
		var config lombard_token_pool.VerifierConfig
		if readErr := common.GetAccountDataBorshIntoAnchor(b.GetContext(), chain.Client, configPDA, cldf_solana.SolDefaultCommitment, &config); readErr == nil && config.AllowlistAdmin == input.AllowlistAdmin {
			b.Logger.Infof("Lombard verifier %s allowlist admin is already %s", input.LombardVerifier, input.AllowlistAdmin)
			return sequences.OnChainOutput{}, nil
		}
		authority, err := getOwner(b.GetContext(), chain, input.LombardVerifier)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		instruction, err := lombard_token_pool.NewSetVerifierAllowlistAdminInstruction(
			input.AllowlistAdmin,
			configPDA,
			authority,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to build set_verifier_allowlist_admin instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{instruction}, input.LombardVerifier, ContractType.String())
	},
)

var SetStorageLocations = operations.NewOperation(
	"lombard-verifier:set-storage-locations",
	Version,
	"Configures Lombard verifier's storage locations",
	func(b operations.Bundle, chain cldf_solana.Chain, input SetStorageLocationsParams) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&lombard_token_pool.ProgramID, lombard_token_pool.ParseInstructionWithoutAccounts, input.LombardVerifier)
		if chain.Deployer() == nil {
			return sequences.OnChainOutput{}, errors.New("missing Solana deployer key")
		}

		configPDA, _, err := state.FindLVConfigPDA(input.LombardVerifier)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive verifier config PDA: %w", err)
		}
		storageLocationsPDA, _, err := state.FindLVStorageLocationsPDA(input.LombardVerifier)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive verifier storage locations PDA: %w", err)
		}
		authority, err := getStorageLocationAdmin(b.GetContext(), chain, input.LombardVerifier)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		storageLocations := make([]lombard_token_pool.ByteString, len(input.StorageLocations))
		for i, loc := range input.StorageLocations {
			storageLocations[i] = lombard_token_pool.ByteString{V0: []byte(loc)}
		}
		params := lombard_token_pool.SetVerifierStorageLocationsParams{
			StorageLocations: storageLocations,
		}
		instruction, err := lombard_token_pool.NewSetVerifierStorageLocationsInstruction(
			params,
			configPDA,
			authority,
			storageLocationsPDA,
			solana.SystemProgramID,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to build set_verifier_storage_locations instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{instruction}, input.LombardVerifier, ContractType.String())
	},
)

var SetChainConfig = operations.NewOperation(
	"lombard-verifier:set-chain-config",
	Version,
	"Configures a Lombard verifier remote chain",
	func(b operations.Bundle, chain cldf_solana.Chain, input SetChainConfigParams) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&lombard_token_pool.ProgramID, lombard_token_pool.ParseInstructionWithoutAccounts, input.LombardVerifier)
		if chain.Deployer() == nil {
			return sequences.OnChainOutput{}, errors.New("missing Solana deployer key")
		}

		configPDA, _, err := state.FindLVConfigPDA(input.LombardVerifier)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive verifier config PDA: %w", err)
		}
		remoteChainConfigPDA, _, err := state.FindLVRemoteChainConfigPDA(input.RemoteChainSelector, input.LombardVerifier)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive verifier remote chain config PDA: %w", err)
		}
		authority, err := getOwner(b.GetContext(), chain, input.LombardVerifier)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		params := lombard_token_pool.SetVerifierChainConfigParams{
			RemoteChainSelector: input.RemoteChainSelector,
			ChainConfig: lombard_token_pool.VerifierRemoteChainAdminConfig{
				RouterSigner:       input.RouterSigner,
				Fee:                lombard_token_pool.UsdCents{V0: uint32(input.FeeUSDCents)},
				GasForVerification: lombard_token_pool.CrossChainGas{V0: input.GasForVerification},
				PayloadSizeBytes:   input.PayloadSizeBytes,
			},
		}
		exists, err := common.IsAccountInitialized(b.GetContext(), chain.Client, remoteChainConfigPDA, input.LombardVerifier, cldf_solana.SolDefaultCommitment, lombard_token_pool.Account_VerifierRemoteChainConfig)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		var instruction solana.Instruction
		if exists {
			instruction, err = lombard_token_pool.NewEditVerifierChainConfigInstruction(
				params,
				configPDA,
				authority,
				remoteChainConfigPDA,
			)
		} else {
			instruction, err = lombard_token_pool.NewInitVerifierChainConfigInstruction(
				params,
				configPDA,
				authority,
				remoteChainConfigPDA,
				solana.SystemProgramID,
			)
		}
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to build verifier chain config instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{instruction}, input.LombardVerifier, ContractType.String())
	},
)

// SetPath sets Lombard's l_chain_id/allowed_caller/remote_bridge_sender routing path for a
// remote chain. Requires remote_chain_config to already exist (created by SetChainConfig).
var SetPath = operations.NewOperation(
	"lombard-verifier:set-path",
	Version,
	"Configures Lombard verifier's GMP routing path for a remote chain",
	func(b operations.Bundle, chain cldf_solana.Chain, input SetPathParams) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&lombard_token_pool.ProgramID, lombard_token_pool.ParseInstructionWithoutAccounts, input.LombardVerifier)
		if chain.Deployer() == nil {
			return sequences.OnChainOutput{}, errors.New("missing Solana deployer key")
		}

		configPDA, _, err := state.FindLVConfigPDA(input.LombardVerifier)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive verifier config PDA: %w", err)
		}
		remoteChainConfigPDA, _, err := state.FindLVRemoteChainConfigPDA(input.RemoteChainSelector, input.LombardVerifier)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive verifier remote chain config PDA: %w", err)
		}
		authority, err := getOwner(b.GetContext(), chain, input.LombardVerifier)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		instruction, err := lombard_token_pool.NewSetVerifierPathInstruction(
			input.RemoteChainSelector,
			input.LChainID,
			input.AllowedCaller,
			input.RemoteBridgeSender,
			configPDA,
			authority,
			remoteChainConfigPDA,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to build set_verifier_path instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{instruction}, input.LombardVerifier, ContractType.String())
	},
)

var RemovePath = operations.NewOperation(
	"lombard-verifier:remove-path",
	Version,
	"Clears Lombard verifier's GMP routing path for a remote chain",
	func(b operations.Bundle, chain cldf_solana.Chain, input RemovePathParams) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&lombard_token_pool.ProgramID, lombard_token_pool.ParseInstructionWithoutAccounts, input.LombardVerifier)
		if chain.Deployer() == nil {
			return sequences.OnChainOutput{}, errors.New("missing Solana deployer key")
		}

		configPDA, _, err := state.FindLVConfigPDA(input.LombardVerifier)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive verifier config PDA: %w", err)
		}
		remoteChainConfigPDA, _, err := state.FindLVRemoteChainConfigPDA(input.RemoteChainSelector, input.LombardVerifier)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive verifier remote chain config PDA: %w", err)
		}
		authority, err := getOwner(b.GetContext(), chain, input.LombardVerifier)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		instruction, err := lombard_token_pool.NewRemoveVerifierPathInstruction(
			input.RemoteChainSelector,
			configPDA,
			authority,
			remoteChainConfigPDA,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to build remove_verifier_path instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{instruction}, input.LombardVerifier, ContractType.String())
	},
)

// SetAllowlist applies sender allowlist changes to a lane. set_verifier_allowlist replaces the
// whole on-chain allowlist, so the changes are applied to the lane's current one first, and
// nothing is sent when the result matches it. See allowlist.Signer for who signs.
var SetAllowlist = operations.NewOperation(
	"lombard-verifier:set-allowlist",
	Version,
	"Sets the Lombard verifier sender allowlist for a destination chain",
	func(b operations.Bundle, chain cldf_solana.Chain, input SetAllowlistParams) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&lombard_token_pool.ProgramID, lombard_token_pool.ParseInstructionWithoutAccounts, input.LombardVerifier)
		if chain.Deployer() == nil {
			return sequences.OnChainOutput{}, errors.New("missing Solana deployer key")
		}

		current, err := readAllowlist(b.GetContext(), chain.Client, input.LombardVerifier, input.DestChainSelector)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		senders, changed, err := allowlist.Update(current, input.Enabled, input.AllowedSendersToAdd, input.AllowedSendersToRemove)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("invalid allowlist for remote chain %d: %w", input.DestChainSelector, err)
		}
		if !changed {
			b.Logger.Infof("Lombard verifier allowlist for remote chain %d is up to date", input.DestChainSelector)
			return sequences.OnChainOutput{}, nil
		}
		owner, err := getOwner(b.GetContext(), chain, input.LombardVerifier)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		admin, err := getAllowlistAdmin(b.GetContext(), chain, input.LombardVerifier)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		authority, err := allowlist.Signer(current, owner, admin, chain.Deployer().PublicKey())
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("remote chain %d on Lombard verifier %s: %w", input.DestChainSelector, input.LombardVerifier, err)
		}

		configPDA, _, err := state.FindLVConfigPDA(input.LombardVerifier)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive verifier config PDA: %w", err)
		}
		remoteChainConfigPDA, _, err := state.FindLVRemoteChainConfigPDA(input.DestChainSelector, input.LombardVerifier)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive verifier remote chain config PDA: %w", err)
		}
		instruction, err := lombard_token_pool.NewSetVerifierAllowlistInstruction(
			lombard_token_pool.SetVerifierAllowlistParams{
				DestChainSelector: input.DestChainSelector,
				Enabled:           input.Enabled,
				AllowedSenders:    senders,
			},
			configPDA,
			authority,
			remoteChainConfigPDA,
			solana.SystemProgramID,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to build set_verifier_allowlist instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{instruction}, input.LombardVerifier, ContractType.String())
	},
)

var SetSupportedTokens = operations.NewOperation(
	"lombard-verifier:set-supported-tokens",
	Version,
	"Configures Lombard verifier's supported local tokens",
	func(b operations.Bundle, chain cldf_solana.Chain, input SetSupportedTokensParams) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&lombard_token_pool.ProgramID, lombard_token_pool.ParseInstructionWithoutAccounts, input.LombardVerifier)
		if chain.Deployer() == nil {
			return sequences.OnChainOutput{}, errors.New("missing Solana deployer key")
		}

		configPDA, _, err := state.FindLVConfigPDA(input.LombardVerifier)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive verifier config PDA: %w", err)
		}
		authority, err := getOwner(b.GetContext(), chain, input.LombardVerifier)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		instruction, err := lombard_token_pool.NewSetVerifierSupportedTokensInstruction(
			input.TokensToRemove,
			input.TokensToSet,
			configPDA,
			authority,
			solana.SystemProgramID,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to build set_verifier_supported_tokens instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{instruction}, input.LombardVerifier, ContractType.String())
	},
)

var SetRemoteAdapters = operations.NewOperation(
	"lombard-verifier:set-remote-adapters",
	Version,
	"Configures Lombard verifier's remote token adapters for a remote chain",
	func(b operations.Bundle, chain cldf_solana.Chain, input SetRemoteAdaptersParams) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&lombard_token_pool.ProgramID, lombard_token_pool.ParseInstructionWithoutAccounts, input.LombardVerifier)
		if chain.Deployer() == nil {
			return sequences.OnChainOutput{}, errors.New("missing Solana deployer key")
		}

		configPDA, _, err := state.FindLVConfigPDA(input.LombardVerifier)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive verifier config PDA: %w", err)
		}
		remoteChainConfigPDA, _, err := state.FindLVRemoteChainConfigPDA(input.RemoteChainSelector, input.LombardVerifier)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive verifier remote chain config PDA: %w", err)
		}
		authority, err := getOwner(b.GetContext(), chain, input.LombardVerifier)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		instruction, err := lombard_token_pool.NewSetVerifierRemoteAdaptersInstruction(
			input.RemoteChainSelector,
			input.RemoteAdapters,
			configPDA,
			authority,
			remoteChainConfigPDA,
			solana.SystemProgramID,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to build set_verifier_remote_adapters instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{instruction}, input.LombardVerifier, ContractType.String())
	},
)

// getOwner returns the owner of program, or the deployer key before the program is initialized.
func getOwner(ctx context.Context, chain cldf_solana.Chain, program solana.PublicKey) (solana.PublicKey, error) {
	configPDA, _, err := state.FindLVConfigPDA(program)
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("failed to derive config PDA: %w", err)
	}
	return solcommon.ConfigAuthority(ctx, chain, program, configPDA, lombard_token_pool.ParseAccount_VerifierConfig, func(c *lombard_token_pool.VerifierConfig) solana.PublicKey { return c.Owner })
}

// getAllowlistAdmin returns the allowlist admin of program, or the deployer key before the program is initialized.
func getAllowlistAdmin(ctx context.Context, chain cldf_solana.Chain, program solana.PublicKey) (solana.PublicKey, error) {
	configPDA, _, err := state.FindLVConfigPDA(program)
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("failed to derive config PDA: %w", err)
	}
	return solcommon.ConfigAuthority(ctx, chain, program, configPDA, lombard_token_pool.ParseAccount_VerifierConfig, func(c *lombard_token_pool.VerifierConfig) solana.PublicKey { return c.AllowlistAdmin })
}

// getStorageLocationAdmin returns the storage location admin of program, or the deployer key before the program is initialized.
func getStorageLocationAdmin(ctx context.Context, chain cldf_solana.Chain, program solana.PublicKey) (solana.PublicKey, error) {
	configPDA, _, err := state.FindLVConfigPDA(program)
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("failed to derive config PDA: %w", err)
	}
	return solcommon.ConfigAuthority(ctx, chain, program, configPDA, lombard_token_pool.ParseAccount_VerifierConfig, func(c *lombard_token_pool.VerifierConfig) solana.PublicKey { return c.StorageLocationAdmin })
}

// getLookupTable returns the zero PublicKey if the remote chain's VerifierRemoteChainConfig
// doesn't exist or its lookup_table hasn't been set yet.
func getLookupTable(ctx context.Context, chain cldf_solana.Chain, program solana.PublicKey, remoteChainSelector uint64) (solana.PublicKey, error) {
	remoteChainConfigPDA, _, err := state.FindLVRemoteChainConfigPDA(remoteChainSelector, program)
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("failed to derive verifier remote chain config PDA: %w", err)
	}
	cfg, err := common.GetInitializedAccount(ctx, chain.Client, remoteChainConfigPDA, program, cldf_solana.SolDefaultCommitment, lombard_token_pool.ParseAccount_VerifierRemoteChainConfig)
	if err != nil || cfg == nil || cfg.LookupTable == nil {
		return solana.PublicKey{}, err
	}
	return *cfg.LookupTable, nil
}
