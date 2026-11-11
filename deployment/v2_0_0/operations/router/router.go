package router

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/Masterminds/semver/v3"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	common_utils "github.com/smartcontractkit/chainlink-ccip/deployment/utils"
	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/sequences"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	cldf_deployment "github.com/smartcontractkit/chainlink-deployments-framework/deployment"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations"
	"github.com/smartcontractkit/mcms/types"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/ccip_router"
	cctputils "github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/cctp"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/tokens"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
)

var (
	ContractType             cldf_deployment.ContractType = "Router"
	DestChainType            cldf_deployment.ContractType = "RemoteDest"
	TokenPoolLookupTableType cldf_deployment.ContractType = "TokenPoolLookupTable"
	ProgramName                                           = "ccip_router"
	Version                  *semver.Version              = solcommon.Version2_0_0
)

type ConnectChainsParams struct {
	LocalChainSelector       uint64
	Router                   solana.PublicKey
	OffRamp                  solana.PublicKey
	TokenReceiverAllowed     bool
	RemoteChainSelector      uint64
	RemoteAddressBytesLength uint8
	// Allowlist changes for the lane, applied to what it has on chain; see DestChainAllowlist.
	// Left nil, an existing lane keeps its allowlist and a new lane starts open.
	Allowlist       *DestChainAllowlist
	DestOffRamp     []byte
	DefaultExecutor solana.PublicKey
	// Outbound (send-side) CCVs for this lane's dest chain config. The router enforces at least one
	// default or lane-mandated CCV (EVM `_validateDefaultAndMandatedCCVs`).
	DefaultCcvs      []solana.PublicKey
	LaneMandatedCcvs []solana.PublicKey
	// Fee accounting fields of the lane. The network fee is charged per message (message vs
	// token-transfer rate), and the base execution gas cost is added to the executor receipt's
	// gas accounting on every send.
	MessageNetworkFeeUSDCents       uint16
	TokenTransferNetworkFeeUSDCents uint16
	BaseExecutionGasCost            uint32
	// Per-message fee cap of the lane (the per-lane equivalent of EVM's global
	// maxFeeJuelsPerMsg). The router rejects any message whose summed receipt fees exceed it, so
	// a lane left at zero rejects every send.
	MaxFeePerMessageUSDCents uint16
}

var Deploy = operations.NewOperation(
	"router:deploy",
	Version,
	"Deploys the Router program",
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
	"router:upgrade",
	Version,
	"Upgrades the default Router program",
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
	"router:initialize",
	Version,
	"Initializes the Router contract",
	func(b operations.Bundle, chain cldf_solana.Chain, input Params) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&ccip_router.ProgramID, ccip_router.ParseInstructionWithoutAccounts, input.Router)
		programData, err := solcommon.GetSolProgramData(chain.Client, input.Router)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to get program data: %w", err)
		}
		authority, err := GetAuthority(b.GetContext(), chain, input.Router)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		routerConfigPDA, _, err := state.FindConfigPDA(input.Router)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive router config PDA: %w", err)
		}
		instruction, err := ccip_router.NewInitializeInstruction(
			chain.Selector,
			solana.PublicKey{},
			input.FeeQuoter,
			input.LinkToken,
			input.RMNRemote,
			routerConfigPDA,
			authority,
			solana.SystemProgramID,
			input.Router,
			programData.Address,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to build initialize instruction: %w", err)
		}
		err = chain.Confirm([]solana.Instruction{instruction})
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to confirm router initialization: %w", err)
		}
		return sequences.OnChainOutput{}, nil
	},
)

// LaneExists reports whether the Router already has a 2.0 lane to the remote chain, as ConnectChains
// sees it: only an account the Router owns counts, so lamports sent to the lane address do not.
func LaneExists(ctx context.Context, chain cldf_solana.Chain, router solana.PublicKey, remoteChainSelector uint64) (bool, error) {
	laneState, err := state.FindDestChainStatePDAV2(remoteChainSelector, router)
	if err != nil {
		return false, fmt.Errorf("failed to derive dest chain state PDA: %w", err)
	}
	existing, err := common.GetInitializedAccount(ctx, chain.Client, laneState, router, cldf_solana.SolDefaultCommitment, ccip_router.ParseAccount_DestChainCcipV2)
	if err != nil {
		return false, fmt.Errorf("failed to read dest chain state account for remote chain %d: %w", remoteChainSelector, err)
	}
	return existing != nil, nil
}

var ConnectChains = operations.NewOperation(
	"router:connect-chains",
	Version,
	"Connects the Router contract to other chains",
	func(b operations.Bundle, chain cldf_solana.Chain, input ConnectChainsParams) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&ccip_router.ProgramID, ccip_router.ParseInstructionWithoutAccounts, input.Router)
		authority, err := GetAuthority(b.GetContext(), chain, input.Router)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		routerConfigPDA, _, err := state.FindConfigPDA(input.Router)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive router config PDA: %w", err)
		}
		// v2.0 lanes use the DestChainCcipV2 account under the DEST_CHAIN_STATE_V2 seed, so it can
		// coexist with a v1 lane on the same selector.
		routerDestChainPDA, err := state.FindDestChainStatePDAV2(input.RemoteChainSelector, input.Router)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive dest chain state PDA: %w", err)
		}
		routerEventAuthorityPDA, _, err := state.FindEventAuthorityPDA(input.Router)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive router event authority PDA: %w", err)
		}
		// CCIP v2.0 lanes use the DestChainCcipV2 account layout (ccip_send_v2 reads it directly).
		existing, err := common.GetInitializedAccount(b.GetContext(), chain.Client, routerDestChainPDA, input.Router, cldf_solana.SolDefaultCommitment, ccip_router.ParseAccount_DestChainCcipV2)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to read dest chain state account for remote chain %d: %w", input.RemoteChainSelector, err)
		}
		var destChainAccount ccip_router.DestChainCcipV2
		var currentConfig *ccip_router.DestChainConfigCcipV2
		isUpdate := existing != nil
		if isUpdate {
			destChainAccount = *existing
			b.Logger.Infof("Remote chain state account found: %+v", destChainAccount)
			currentConfig = &destChainAccount.Config
		}
		laneAllowlistEnabled, laneAllowedSenders, err := resolveAllowlist(input.Allowlist, currentConfig)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("resolve allowlist for remote chain %d: %w", input.RemoteChainSelector, err)
		}
		destChainConfig := ccip_router.DestChainConfigCcipV2{
			AllowedSenders:          laneAllowedSenders,
			AllowListEnabled:        laneAllowlistEnabled,
			DefaultExecutor:         input.DefaultExecutor,
			Offramp:                 input.DestOffRamp,
			DefaultCcvs:             input.DefaultCcvs,
			LaneMandatedCcvs:        input.LaneMandatedCcvs,
			AddressBytesLength:      input.RemoteAddressBytesLength,
			MessageNetworkFee:       ccip_router.UsdCents{V0: uint32(input.MessageNetworkFeeUSDCents)},
			TokenTransferNetworkFee: ccip_router.UsdCents{V0: uint32(input.TokenTransferNetworkFeeUSDCents)},
			BaseExecutionGasCost:    ccip_router.CrossChainGas{V0: input.BaseExecutionGasCost},
			MaxFeePerMessage:        ccip_router.UsdCents{V0: uint32(input.MaxFeePerMessageUSDCents)},
			TokenReceiverAllowed:    input.TokenReceiverAllowed,
		}
		var ixn solana.Instruction
		var addressRefs []datastore.AddressRef
		if isUpdate {
			ixn, err = ccip_router.NewUpdateDestChainConfigV2Instruction(
				ccip_router.UpdateChainSelectorV2Params{
					DestChainSelector: input.RemoteChainSelector,
					DestChainConfig:   destChainConfig,
				},
				routerDestChainPDA,
				routerConfigPDA,
				authority,
				solana.SystemProgramID,
				routerEventAuthorityPDA,
				input.Router,
			)
			if err != nil {
				return sequences.OnChainOutput{}, fmt.Errorf("failed to build update dest chain instruction: %w", err)
			}
		} else {
			ixn, err = ccip_router.NewAddDestChainV2Instruction(
				ccip_router.AddDestChainV2Params{
					NewChainSelector: input.RemoteChainSelector,
					DestChainConfig:  destChainConfig,
				},
				routerDestChainPDA,
				routerConfigPDA,
				authority,
				solana.SystemProgramID,
				routerEventAuthorityPDA,
				input.Router,
			)
			if err != nil {
				return sequences.OnChainOutput{}, fmt.Errorf("failed to build add dest chain instruction: %w", err)
			}
			err = solcommon.ExtendOfframpLookupTable(chain, input.OffRamp, []solana.PublicKey{routerDestChainPDA})
			if err != nil {
				return sequences.OnChainOutput{}, fmt.Errorf("failed to extend OffRamp lookup table: %w", err)
			}
			addressRefs = append(addressRefs, datastore.AddressRef{
				Address:       routerDestChainPDA.String(),
				ChainSelector: chain.Selector,
				Type:          datastore.ContractType(DestChainType),
				Version:       Version,
				Qualifier:     strconv.FormatUint(input.RemoteChainSelector, 10),
			})
		}
		output, err := solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{ixn}, input.Router, ContractType.String())
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		output.Addresses = addressRefs
		return output, nil
	},
)

type AddLegacyDestChainParams struct {
	Router              solana.PublicKey
	RemoteChainSelector uint64
}

// AddLegacyDestChain enables the router's 1.6 `ccip_send` towards a destination, which programs
// still built against that interface (such as the v1 example sender) send through. It is separate
// from the v2.0 lane ConnectChains creates and lives under its own seed, so both can coexist on
// the same selector. Messages sent this way are only delivered where 1.6 offchain infrastructure
// serves the lane.
var AddLegacyDestChain = operations.NewOperation(
	"router:add-legacy-dest-chain",
	Version,
	"Adds a 1.6 destination chain to the Router contract",
	func(b operations.Bundle, chain cldf_solana.Chain, input AddLegacyDestChainParams) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&ccip_router.ProgramID, ccip_router.ParseInstructionWithoutAccounts, input.Router)
		destChainPDA, err := state.FindDestChainStatePDA(input.RemoteChainSelector, input.Router)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive legacy dest chain state PDA: %w", err)
		}
		destChainAccount, err := common.GetInitializedAccount(b.GetContext(), chain.Client, destChainPDA, input.Router, cldf_solana.SolDefaultCommitment, ccip_router.ParseAccount_DestChain)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to read legacy dest chain state account %s for remote chain %d: %w", destChainPDA, input.RemoteChainSelector, err)
		}
		if destChainAccount != nil {
			b.Logger.Infof("Legacy dest chain state account found: %+v", *destChainAccount)
			return sequences.OnChainOutput{}, nil
		}
		routerConfigPDA, _, err := state.FindConfigPDA(input.Router)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive router config PDA: %w", err)
		}
		authority, err := GetAuthority(b.GetContext(), chain, input.Router)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		ixn, err := ccip_router.NewAddChainSelectorInstruction(
			input.RemoteChainSelector,
			ccip_router.DestChainConfig{},
			destChainPDA,
			routerConfigPDA,
			authority,
			solana.SystemProgramID,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to build add chain selector instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{ixn}, input.Router, ContractType.String())
	},
)

var AddOffRamp = operations.NewOperation(
	"router:add-off-ramp",
	Version,
	"Adds an OffRamp to the Router contract for a given chain",
	func(b operations.Bundle, chain cldf_solana.Chain, input ConnectChainsParams) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&ccip_router.ProgramID, ccip_router.ParseInstructionWithoutAccounts, input.Router)
		authority, err := GetAuthority(b.GetContext(), chain, input.Router)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		routerConfigPDA, _, err := state.FindConfigPDA(input.Router)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive router config PDA: %w", err)
		}
		allowedOffRampRemotePDA, err := state.FindAllowedOfframpPDA(input.RemoteChainSelector, input.OffRamp, input.Router)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive allowed offramp PDA: %w", err)
		}
		allowed, err := common.IsAccountInitialized(b.GetContext(), chain.Client, allowedOffRampRemotePDA, input.Router, cldf_solana.SolDefaultCommitment, ccip_router.Account_AllowedOfframp)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to read allowed offramp account: %w", err)
		}
		if allowed {
			b.Logger.Infof("OffRamp %s is already allowed on Router %s for remote chain %d", input.OffRamp, input.Router, input.RemoteChainSelector)
			return sequences.OnChainOutput{}, nil
		}
		ixn, err := ccip_router.NewAddOfframpInstruction(
			input.RemoteChainSelector,
			input.OffRamp,
			allowedOffRampRemotePDA,
			routerConfigPDA,
			authority,
			solana.SystemProgramID,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to build add off ramp instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{ixn}, input.Router, ContractType.String())
	},
)

type UpdateReferenceAddressesParams struct {
	Router            solana.PublicKey
	OnrampLookupTable solana.PublicKey
}

var UpdateReferenceAddresses = operations.NewOperation(
	"router:update-reference-addresses",
	Version,
	"Updates the Router reference addresses, including the lookup table ccip_send_v2 / get_fee_v2 account resolution returns",
	func(b operations.Bundle, chain cldf_solana.Chain, input UpdateReferenceAddressesParams) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&ccip_router.ProgramID, ccip_router.ParseInstructionWithoutAccounts, input.Router)
		authority, err := GetAuthority(b.GetContext(), chain, input.Router)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		referenceAddressesPDA, _, err := state.FindRouterReferenceAddressesPDA(input.Router)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive router reference addresses PDA: %w", err)
		}
		var current ccip_router.ReferenceAddresses
		err = common.GetAccountDataBorshIntoAnchor(b.GetContext(), chain.Client, referenceAddressesPDA, cldf_solana.SolDefaultCommitment, &current)
		if err == nil && current.OnrampLookupTable == input.OnrampLookupTable {
			b.Logger.Infof("Router reference addresses already up to date")
			return sequences.OnChainOutput{}, nil
		}
		routerConfigPDA, _, err := state.FindConfigPDA(input.Router)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive router config PDA: %w", err)
		}
		routerEventAuthorityPDA, _, err := state.FindEventAuthorityPDA(input.Router)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive router event authority PDA: %w", err)
		}
		ixn, err := ccip_router.NewUpdateReferenceAddressesInstruction(
			input.OnrampLookupTable,
			referenceAddressesPDA,
			routerConfigPDA,
			authority,
			solana.SystemProgramID,
			routerEventAuthorityPDA,
			input.Router,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to build update reference addresses instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{ixn}, input.Router, ContractType.String())
	},
)

// OnrampLookupTable returns the zero key when the Router has no reference addresses account.
func OnrampLookupTable(ctx context.Context, chain cldf_solana.Chain, router solana.PublicKey) (solana.PublicKey, error) {
	referenceAddressesPDA, _, err := state.FindRouterReferenceAddressesPDA(router)
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("failed to derive router reference addresses PDA: %w", err)
	}
	var current ccip_router.ReferenceAddresses
	err = common.GetAccountDataBorshIntoAnchor(ctx, chain.Client, referenceAddressesPDA, cldf_solana.SolDefaultCommitment, &current)
	if errors.Is(err, rpc.ErrNotFound) {
		return solana.PublicKey{}, nil
	}
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("read router reference addresses: %w", err)
	}
	return current.OnrampLookupTable, nil
}

var TransferOwnership = operations.NewOperation(
	"router:transfer-ownership",
	Version,
	"Transfers ownership of the Router contract to a new authority",
	func(b operations.Bundle, chain cldf_solana.Chain, input solcommon.TransferOwnershipParams) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&ccip_router.ProgramID, ccip_router.ParseInstructionWithoutAccounts, input.Program)
		authority, err := GetAuthority(b.GetContext(), chain, input.Program)
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
		ixn, err := ccip_router.NewTransferOwnershipInstruction(
			input.NewOwner,
			configPDA,
			authority,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to build transfer ownership instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{ixn}, input.Program, ContractType.String())
	},
)

var AcceptOwnership = operations.NewOperation(
	"router:accept-ownership",
	Version,
	"Accepts ownership of the Router contract",
	func(b operations.Bundle, chain cldf_solana.Chain, input solcommon.TransferOwnershipParams) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&ccip_router.ProgramID, ccip_router.ParseInstructionWithoutAccounts, input.Program)
		configPDA, _, err := state.FindConfigPDA(input.Program)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive config PDA: %w", err)
		}
		ixn, err := ccip_router.NewAcceptOwnershipInstruction(
			configPDA,
			input.NewOwner,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to build accept ownership instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, input.NewOwner, []solana.Instruction{ixn}, input.Program, ContractType.String())
	},
)

var SetPool = operations.NewOperation(
	"router:set-pool",
	Version,
	"Sets the pool of the Router contract",
	func(b operations.Bundle, chain cldf_solana.Chain, input PoolParams) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&ccip_router.ProgramID, ccip_router.ParseInstructionWithoutAccounts, input.Router)
		// Checked before any on-chain work: the lookup table this operation creates is
		// recorded under TokenSymbol, and a blank qualifier would collide with every other
		// blank-qualified lookup table on the chain.
		if input.TokenSymbol == "" {
			return sequences.OnChainOutput{}, errors.New("TokenSymbol is required to qualify the token pool lookup table")
		}
		addresses := make([]datastore.AddressRef, 0, 1)
		// we can only sign as either the deployer or the token admin
		// if there is no admin set, we assume the router authority is timelock
		tokenAdminRegistryPDA, _, err := state.FindTokenAdminRegistryPDA(input.TokenMint, input.Router)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive token admin registry PDA: %w", err)
		}
		currentAdmin, err := GetAuthority(b.GetContext(), chain, input.Router)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		var tokenAdminRegistryAccount ccip_router.TokenAdminRegistry
		tokenAdminRegistryAccount, err = loadTokenAdminRegistry(b, chain, input.Router, input.TokenMint)
		if err != nil && !errors.Is(err, rpc.ErrNotFound) {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to read token admin registry for mint %s: %w", input.TokenMint, err)
		}
		if err == nil {
			if !tokenAdminRegistryAccount.LookupTable.IsZero() {
				b.Logger.Info("Token pool lookup table already set for this mint with the given admin:", tokenAdminRegistryAccount)
				// Emitted so a datastore that lost the ref gets it back.
				return sequences.OnChainOutput{
					Addresses: []datastore.AddressRef{{
						Address:       tokenAdminRegistryAccount.LookupTable.String(),
						ChainSelector: chain.Selector,
						Labels:        datastore.NewLabelSet(input.TokenPool.String(), input.TokenMint.String()),
						Type:          datastore.ContractType(TokenPoolLookupTableType),
						Version:       Version,
						Qualifier:     input.TokenSymbol,
					}},
				}, nil
			}
			// If the admin is zero, it means we're performing a propose + accept + set sequence,
			// so we should sign with the timelock (current authority)
			if !tokenAdminRegistryAccount.Administrator.IsZero() {
				currentAdmin = tokenAdminRegistryAccount.Administrator
			}
		}
		// Only works for BnM and LnR pools for now
		tokenPoolConfigPDA, err := tokens.TokenPoolConfigAddress(input.TokenMint, input.TokenPool)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive token pool config address: %w", err)
		}
		tokenPoolSigner, err := tokens.TokenPoolSignerAddress(input.TokenMint, input.TokenPool)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive token pool signer address: %w", err)
		}
		poolTokenAccount, _, err := tokens.FindAssociatedTokenAddress(input.TokenProgramID, input.TokenMint, tokenPoolSigner)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive pool token account: %w", err)
		}
		feeTokenConfigPDA, _, err := state.FindFqBillingTokenConfigPDA(input.TokenMint, input.FeeQuoter)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive fee token config PDA: %w", err)
		}
		routerPoolSignerPDA, _, err := state.FindExternalTokenPoolsSignerPDA(input.TokenPool, input.Router)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive router pool signer PDA: %w", err)
		}
		table, err := solcommon.PubkeyFromRefs(input.ExistingAddresses, datastore.NewAddressRefKey(chain.Selector, datastore.ContractType(TokenPoolLookupTableType), Version, input.TokenSymbol))
		if err != nil && !errors.Is(err, solcommon.ErrAddressRefNotFound) {
			return sequences.OnChainOutput{}, err
		}
		table, err = solcommon.PreparedLookupTable(b.GetContext(), chain, table)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to create lookup table: %w", err)
		}
		// link the token + token pool lookup table + token mint
		labels := datastore.NewLabelSet(input.TokenPool.String(), input.TokenMint.String())
		addresses = append(addresses, datastore.AddressRef{
			Address:       table.String(),
			ChainSelector: chain.Selector,
			Labels:        labels,
			Type:          datastore.ContractType(TokenPoolLookupTableType),
			Version:       Version,
			Qualifier:     input.TokenSymbol,
		})
		writableIndexes := input.WritableIndexes
		if writableIndexes == nil {
			writableIndexes = []uint8{3, 4, 7}
		}
		list := solana.PublicKeySlice{
			table,                 // 0
			tokenAdminRegistryPDA, // 1
			input.TokenPool,       // 2
			tokenPoolConfigPDA,    // 3 - writable
			poolTokenAccount,      // 4 - writable
			tokenPoolSigner,       // 5 - only writable if CCTP
			input.TokenProgramID,  // 6
			input.TokenMint,       // 7 - writable
			feeTokenConfigPDA,     // 8
			routerPoolSignerPDA,   // 9
		}
		if input.TokenPoolType == string(common_utils.CCTPTokenPool) {
			// hack: we only use values here that are for all chains/domain, so we can safely use a bogus domain & token
			// and just pick which entries we put in the LUT
			bogusDomain := uint32(255)
			bogusToken := []byte{1, 2, 3}
			tokenMessengerMinterPDAs, nerr := cctputils.GetTokenMessengerMinterPDAs(bogusDomain, input.TokenMint, bogusToken)
			if nerr != nil {
				return sequences.OnChainOutput{}, fmt.Errorf("failed to get CCTPv1 token messenger minter PDAs: %w", nerr)
			}
			messageTransmitterPDAs, nerr := cctputils.GetMessageTransmitterPDAs()
			if nerr != nil {
				return sequences.OnChainOutput{}, fmt.Errorf("failed to get CCTPv1 message transmitter PDAs: %w", nerr)
			}

			// these are all CCTPv1 entries, since the pool-part of the program only interacts with CCTPv1 on 1.6
			// (on 2.0, it's the verifier that interacts with CCTPv2, and that's set up separately)
			list = append(list,
				messageTransmitterPDAs.MessageTransmitter, // 10 - writable
				tokenMessengerMinterPDAs.Program,          // 11
				solana.SystemProgramID,                    // 12
				messageTransmitterPDAs.Program,            // 13
				tokenMessengerMinterPDAs.TokenMessenger,   // 14
				tokenMessengerMinterPDAs.TokenMinter,      // 15
				tokenMessengerMinterPDAs.LocalToken,       // 16 - writable
				tokenMessengerMinterPDAs.EventAuthority,   // 17
			)
			writableIndexes = append(writableIndexes, 5, 10, 16)
		}
		tableState, err := solcommon.ReadLookupTable(b.GetContext(), chain, table)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		// The router interprets these entries by index, so reuse must preserve the prefix.
		for i, entry := range tableState.Addresses {
			if i < len(list) && entry != list[i] {
				return sequences.OnChainOutput{}, fmt.Errorf("lookup table %s has an incompatible pool entry at index %d", table, i)
			}
		}
		err = solcommon.ExtendLookupTable(chain, table, list)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to extend lookup table for token pool (mint: %s): %w", input.TokenMint.String(), err)
		}
		if awaitErr := common.AwaitSlotChange(b.GetContext(), chain.Client); awaitErr != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to await slot change while extending lookup table: %w", awaitErr)
		}

		routerConfigPDA, _, err := state.FindConfigPDA(input.Router)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive router config PDA: %w", err)
		}
		raw, err := ccip_router.NewSetPoolInstruction(
			writableIndexes,
			routerConfigPDA,
			tokenAdminRegistryPDA,
			input.TokenMint,
			table,
			currentAdmin,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to build router set pool instruction: %w", err)
		}
		genericRaw, ok := raw.(*solana.GenericInstruction)
		if !ok {
			return sequences.OnChainOutput{}, fmt.Errorf("unexpected instruction type from NewSetPoolInstruction: %T", raw)
		}
		genericRaw.AccountValues = append(genericRaw.AccountValues, solana.Meta(table))
		ixData, err := raw.Data()
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to extract data payload from router set pool instruction: %w", err)
		}
		instruction := solana.NewInstruction(input.Router, raw.Accounts(), ixData)
		output, err := solcommon.ConfirmOrBatch(chain, currentAdmin, []solana.Instruction{instruction}, input.Router, ContractType.String())
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		output.Addresses = addresses
		return output, nil
	},
)

var SetPoolInterfaceImplementation = operations.NewOperation(
	"router:set-pool-interface-implementation",
	Version,
	"Sets the pool interface version and account resolution flag on the Token Admin Registry",
	func(b operations.Bundle, chain cldf_solana.Chain, input SetPoolInterfaceImplParams) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&ccip_router.ProgramID, ccip_router.ParseInstructionWithoutAccounts, input.Router)
		routerConfigPDA, _, err := state.FindConfigPDA(input.Router)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive router config PDA: %w", err)
		}
		tokenAdminRegistryPDA, _, err := state.FindTokenAdminRegistryPDA(input.TokenMint, input.Router)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive token admin registry PDA: %w", err)
		}

		// Read the current TAR to determine the administrator for signing.
		currentAdmin, err := GetAuthority(b.GetContext(), chain, input.Router)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		var tokenAdminRegistryAccount ccip_router.TokenAdminRegistry
		resolutionType, err := toResolutionType(input.ImplementsAccountResolution)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("invalid resolution type: %w", err)
		}
		tokenAdminRegistryAccount, err = loadTokenAdminRegistry(b, chain, input.Router, input.TokenMint)
		if err != nil && !errors.Is(err, rpc.ErrNotFound) {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to read token admin registry for mint %s: %w", input.TokenMint, err)
		}
		if err == nil {
			if tokenAdminRegistryAccount.InterfaceVersion == input.InterfaceVersion &&
				TokenAdminRegistryResolutionType(tokenAdminRegistryAccount) == *resolutionType {
				b.Logger.Info("Pool interface implementation already set for this mint", "mint", input.TokenMint,
					"interfaceVersion", input.InterfaceVersion, "implementsAccountResolution", input.ImplementsAccountResolution)
				return sequences.OnChainOutput{}, nil
			}
			if !tokenAdminRegistryAccount.Administrator.IsZero() {
				currentAdmin = tokenAdminRegistryAccount.Administrator
			}
		}

		eventAuthorityPDA, _, err := state.FindEventAuthorityPDA(input.Router)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive event authority PDA: %w", err)
		}

		raw, err := ccip_router.NewSetPoolInterfaceImplementationInstruction(
			ccip_router.SetPoolInterfaceImplementationParams{
				Mint:              input.TokenMint,
				InterfaceVersion:  input.InterfaceVersion,
				AccountResolution: *resolutionType,
			},
			routerConfigPDA,
			tokenAdminRegistryPDA,
			currentAdmin,
			eventAuthorityPDA,
			input.Router,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to build set pool interface implementation instruction: %w", err)
		}

		ixData, err := raw.Data()
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to extract data payload from set pool interface implementation instruction: %w", err)
		}
		instruction := solana.NewInstruction(input.Router, raw.Accounts(), ixData)

		return solcommon.ConfirmOrBatch(chain, currentAdmin, []solana.Instruction{instruction}, input.Router, ContractType.String())
	},
)

var RegisterTokenAdminRegistry = operations.NewOperation(
	"router:register-token-admin-registry",
	Version,
	"Registers a Token Admin Registry with the Router contract",
	func(b operations.Bundle, chain cldf_solana.Chain, input TokenAdminRegistryParams) (TokenAdminRegistryOut, error) {
		solcommon.SetProgramID(&ccip_router.ProgramID, ccip_router.ParseInstructionWithoutAccounts, input.Router)
		routerConfigPDA, _, err := state.FindConfigPDA(input.Router)
		if err != nil {
			return TokenAdminRegistryOut{}, fmt.Errorf("failed to derive router config PDA: %w", err)
		}
		tokenAdminRegistryPDA, _, err := state.FindTokenAdminRegistryPDA(input.TokenMint, input.Router)
		if err != nil {
			return TokenAdminRegistryOut{}, fmt.Errorf("failed to derive token admin registry PDA: %w", err)
		}
		tokenMintAuthority, err := solcommon.GetTokenMintAuthority(chain, input.TokenMint)
		if err != nil {
			return TokenAdminRegistryOut{}, fmt.Errorf("failed to get token mint authority: %w", err)
		}
		// we can only sign as either the deployer or timelock
		// ccip admin should be timelock
		ccipAdmin, err := GetAuthority(b.GetContext(), chain, input.Router)
		if err != nil {
			return TokenAdminRegistryOut{}, err
		}
		// if no admin provided, use ccip admin
		if input.Admin.IsZero() {
			input.Admin = ccipAdmin
		}
		var pendingAdmin solana.PublicKey
		var tokenAdminRegistryAccount ccip_router.TokenAdminRegistry
		tokenAdminRegistryAccount, err = loadTokenAdminRegistry(b, chain, input.Router, input.TokenMint)
		if err != nil && !errors.Is(err, rpc.ErrNotFound) {
			return TokenAdminRegistryOut{}, fmt.Errorf("failed to read token admin registry for mint %s: %w", input.TokenMint, err)
		}
		if err == nil {
			if input.Admin == tokenAdminRegistryAccount.Administrator {
				b.Logger.Info("Token admin registry already registered with the given admin:", tokenAdminRegistryAccount)
				return TokenAdminRegistryOut{}, nil
			}
			pendingAdmin = tokenAdminRegistryAccount.PendingAdministrator
			// there is already an admin registered, we need to transfer
			if !tokenAdminRegistryAccount.Administrator.IsZero() {
				out, transferErr := operations.ExecuteOperation(b, TransferTokenAdminRegistry, chain, input)
				if transferErr != nil {
					return TokenAdminRegistryOut{}, fmt.Errorf("failed to transfer token admin registry: %w", transferErr)
				}
				b.Logger.Infof("Token admin registry transfer initiated. Pending admin: %s", out.Output.PendingSigner.String())
				return out.Output, nil
			}
		}
		// this is the key that will need to accept the admin registration
		pendingSigner := input.Admin
		var ixn solana.Instruction
		var needProposal bool
		// we need to override the admin if there is a pending admin
		if !pendingAdmin.IsZero() {
			// we need to register via ccip admin if the mint authority is not the deployer
			if tokenMintAuthority != chain.DeployerKey.PublicKey() {
				tmp, ixErr := ccip_router.NewCcipAdminOverridePendingAdministratorInstruction(
					input.Admin, // admin of the tokenAdminRegistry PDA
					routerConfigPDA,
					tokenAdminRegistryPDA, // this gets created
					input.TokenMint,
					ccipAdmin,
					solana.SystemProgramID,
				)
				if ixErr != nil {
					return TokenAdminRegistryOut{}, fmt.Errorf("failed to build override pending admin instruction: %w", ixErr)
				}
				ixData, ixErr := tmp.Data()
				if ixErr != nil {
					return TokenAdminRegistryOut{}, fmt.Errorf("failed to extract data payload from override pending admin instruction: %w", ixErr)
				}
				ixn = solana.NewInstruction(input.Router, tmp.Accounts(), ixData)
				if ccipAdmin != chain.DeployerKey.PublicKey() {
					needProposal = true
				}
			} else {
				// the token mint authority can register directly
				tmp, ixErr := ccip_router.NewOwnerOverridePendingAdministratorInstruction(
					input.Admin, // admin of the tokenAdminRegistry PDA
					routerConfigPDA,
					tokenAdminRegistryPDA, // this gets created
					input.TokenMint,
					tokenMintAuthority,
					solana.SystemProgramID,
				)
				if ixErr != nil {
					return TokenAdminRegistryOut{}, fmt.Errorf("failed to build override pending admin instruction: %w", ixErr)
				}
				ixData, ixErr := tmp.Data()
				if ixErr != nil {
					return TokenAdminRegistryOut{}, fmt.Errorf("failed to extract data payload from override pending admin instruction: %w", ixErr)
				}
				ixn = solana.NewInstruction(input.Router, tmp.Accounts(), ixData)
				if tokenMintAuthority != chain.DeployerKey.PublicKey() {
					needProposal = true
				}
			}
		} else {
			// no pending admin, normal registration
			// we need to register via ccip admin if the mint authority is not the deployer
			if tokenMintAuthority != chain.DeployerKey.PublicKey() {
				tmp, ixErr := ccip_router.NewCcipAdminProposeAdministratorInstruction(
					input.Admin, // admin of the tokenAdminRegistry PDA
					routerConfigPDA,
					tokenAdminRegistryPDA, // this gets created
					input.TokenMint,
					ccipAdmin,
					solana.SystemProgramID,
				)
				if ixErr != nil {
					return TokenAdminRegistryOut{}, fmt.Errorf("failed to build propose administrator instruction: %w", ixErr)
				}
				ixData, ixErr := tmp.Data()
				if ixErr != nil {
					return TokenAdminRegistryOut{}, fmt.Errorf("failed to extract data payload from propose administrator instruction: %w", ixErr)
				}
				ixn = solana.NewInstruction(input.Router, tmp.Accounts(), ixData)
				if ccipAdmin != chain.DeployerKey.PublicKey() {
					needProposal = true
				}
			} else {
				// the token mint authority can register directly
				tmp, ixErr := ccip_router.NewOwnerProposeAdministratorInstruction(
					input.Admin, // admin of the tokenAdminRegistry PDA
					routerConfigPDA,
					tokenAdminRegistryPDA, // this gets created
					input.TokenMint,
					tokenMintAuthority,
					solana.SystemProgramID,
				)
				if ixErr != nil {
					return TokenAdminRegistryOut{}, fmt.Errorf("failed to build propose administrator instruction: %w", ixErr)
				}
				ixData, ixErr := tmp.Data()
				if ixErr != nil {
					return TokenAdminRegistryOut{}, fmt.Errorf("failed to extract data payload from propose administrator instruction: %w", ixErr)
				}
				ixn = solana.NewInstruction(input.Router, tmp.Accounts(), ixData)
				if tokenMintAuthority != chain.DeployerKey.PublicKey() {
					needProposal = true
				}
			}
		}
		if needProposal {
			batches, batchErr := solcommon.BuildMCMSBatchOperation(
				chain.Selector,
				[]solana.Instruction{ixn},
				input.Router.String(),
				ContractType.String(),
			)
			if batchErr != nil {
				return TokenAdminRegistryOut{}, fmt.Errorf("failed to execute or create batch: %w", batchErr)
			}
			return TokenAdminRegistryOut{
				PendingSigner: pendingSigner,
				OnChainOutput: sequences.OnChainOutput{
					BatchOps: []types.BatchOperation{batches},
				},
			}, nil
		}

		err = chain.Confirm([]solana.Instruction{ixn})
		if err != nil {
			return TokenAdminRegistryOut{}, fmt.Errorf("failed to confirm register token admin registry: %w", err)
		}
		return TokenAdminRegistryOut{
			PendingSigner: pendingSigner,
		}, nil
	},
)

var AcceptTokenAdminRegistry = operations.NewOperation(
	"router:accept-token-admin-registry",
	Version,
	"Accepts a Token Admin Registry with the Router contract",
	func(b operations.Bundle, chain cldf_solana.Chain, input TokenAdminRegistryParams) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&ccip_router.ProgramID, ccip_router.ParseInstructionWithoutAccounts, input.Router)
		routerConfigPDA, _, err := state.FindConfigPDA(input.Router)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive router config PDA: %w", err)
		}
		tokenAdminRegistryPDA, _, err := state.FindTokenAdminRegistryPDA(input.TokenMint, input.Router)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive token admin registry PDA: %w", err)
		}
		var pendingAdmin solana.PublicKey
		var currentAdmin solana.PublicKey
		var tokenAdminRegistryAccount ccip_router.TokenAdminRegistry
		tokenAdminRegistryAccount, err = loadTokenAdminRegistry(b, chain, input.Router, input.TokenMint)
		if err != nil && !errors.Is(err, rpc.ErrNotFound) {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to read token admin registry for mint %s: %w", input.TokenMint, err)
		}
		if err == nil {
			// NOTE: only set pendingAdmin if the account exists / fetch succeeds
			pendingAdmin = tokenAdminRegistryAccount.PendingAdministrator
			currentAdmin = tokenAdminRegistryAccount.Administrator
		}
		// A chain without MCMS (the devenv) has no timelock to sign as, which only matters if
		// the registry is actually held by one - hence the tolerant lookup.
		timelockSigner, hasTimelock, err := solcommon.FindTimelockSignerPDA(
			input.ExistingAddresses,
			chain.Selector,
			common_utils.CLLQualifier,
		)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		isTimelock := func(key solana.PublicKey) bool { return hasTimelock && key == timelockSigner }
		timelockDesc := "<none deployed>"
		if hasTimelock {
			timelockDesc = timelockSigner.String()
		}
		if pendingAdmin.IsZero() {
			// if there is no pending admin, we assume the authority is timelock
			// but we need to confirm that timelock is indeed the authority
			if isTimelock(currentAdmin) {
				b.Logger.Info("No pending admin found, but timelock is already the current admin, skipping accept admin role for token admin registry")
				return sequences.OnChainOutput{}, nil
			}
			if !isTimelock(input.Admin) {
				return sequences.OnChainOutput{}, fmt.Errorf("no pending admin found for token admin registry, expected timelock signer %s but got %s", timelockDesc, input.Admin.String())
			}
			pendingAdmin = input.Admin
		} else if !isTimelock(pendingAdmin) && pendingAdmin != chain.DeployerKey.PublicKey() {
			// we can only sign as either the deployer or timelock
			return sequences.OnChainOutput{}, fmt.Errorf("pending admin %s does not match timelock signer %s or deployer %s", pendingAdmin.String(), timelockDesc, chain.DeployerKey.PublicKey().String())
		}
		// sign as the pending admin to accept
		// when there is no pending admin, we assume the authority is timelock
		tempIx, err := ccip_router.NewAcceptAdminRoleTokenAdminRegistryInstruction(
			routerConfigPDA,
			tokenAdminRegistryPDA,
			input.TokenMint,
			pendingAdmin,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to generate instructions: %w", err)
		}
		ixData, err := tempIx.Data()
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to extract data payload router accept admin role token admin registry instruction: %w", err)
		}
		ixn := solana.NewInstruction(input.Router, tempIx.Accounts(), ixData)
		// pending admin unset = we're proposing and accepting in the same batch, so timelock must be the signer
		// pending admin set = we need a proposal if the admin is not the deployer
		if pendingAdmin.IsZero() || pendingAdmin != chain.DeployerKey.PublicKey() {
			var batches types.BatchOperation
			batches, err = solcommon.BuildMCMSBatchOperation(
				chain.Selector,
				[]solana.Instruction{ixn},
				input.Router.String(),
				ContractType.String(),
			)
			if err != nil {
				return sequences.OnChainOutput{}, fmt.Errorf("failed to execute or create batch: %w", err)
			}
			return sequences.OnChainOutput{
				BatchOps: []types.BatchOperation{batches},
			}, nil
		}

		err = chain.Confirm([]solana.Instruction{ixn})
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to confirm accept token admin registry: %w", err)
		}
		return sequences.OnChainOutput{}, nil
	},
)

var TransferTokenAdminRegistry = operations.NewOperation(
	"router:transfer-token-admin-registry",
	Version,
	"Transfers a Token Admin Registry with the Router contract",
	func(b operations.Bundle, chain cldf_solana.Chain, input TokenAdminRegistryParams) (TokenAdminRegistryOut, error) {
		solcommon.SetProgramID(&ccip_router.ProgramID, ccip_router.ParseInstructionWithoutAccounts, input.Router)
		routerConfigPDA, _, err := state.FindConfigPDA(input.Router)
		if err != nil {
			return TokenAdminRegistryOut{}, fmt.Errorf("failed to derive router config PDA: %w", err)
		}
		tokenAdminRegistryPDA, _, err := state.FindTokenAdminRegistryPDA(input.TokenMint, input.Router)
		if err != nil {
			return TokenAdminRegistryOut{}, fmt.Errorf("failed to derive token admin registry PDA: %w", err)
		}
		var currentAdmin solana.PublicKey
		var tokenAdminRegistryAccount ccip_router.TokenAdminRegistry
		if tokenAdminRegistryAccount, err = loadTokenAdminRegistry(b, chain, input.Router, input.TokenMint); err != nil {
			return TokenAdminRegistryOut{}, fmt.Errorf("failed to fetch token admin registry account: %w", err)
		}
		currentAdmin = tokenAdminRegistryAccount.Administrator
		if currentAdmin.IsZero() {
			return TokenAdminRegistryOut{}, errors.New("no admin found for token admin registry")
		}
		b.Logger.Infof("Current token admin registry account: %+v", tokenAdminRegistryAccount)
		// we can only sign as either the deployer or timelock
		// ccip admin should be timelock
		ccipAdmin, err := GetAuthority(b.GetContext(), chain, input.Router)
		if err != nil {
			return TokenAdminRegistryOut{}, err
		}
		// if no admin provided, use ccip admin
		if input.Admin.IsZero() {
			input.Admin = ccipAdmin
		}
		if currentAdmin == input.Admin {
			b.Logger.Info("Token admin registry already registered with the given admin:", tokenAdminRegistryAccount)
			return TokenAdminRegistryOut{}, nil
		}
		// sign as the current admin to transfer
		tempIx, err := ccip_router.NewTransferAdminRoleTokenAdminRegistryInstruction(
			input.Admin,
			routerConfigPDA,
			tokenAdminRegistryPDA,
			input.TokenMint,
			currentAdmin,
		)
		if err != nil {
			return TokenAdminRegistryOut{}, fmt.Errorf("failed to generate instructions: %w", err)
		}
		ixData, err := tempIx.Data()
		if err != nil {
			return TokenAdminRegistryOut{}, fmt.Errorf("failed to extract data payload from transfer admin role token admin registry instruction: %w", err)
		}
		ixn := solana.NewInstruction(input.Router, tempIx.Accounts(), ixData)
		// now we need a proposal if the admin is not the deployer
		output, err := solcommon.ConfirmOrBatch(chain, currentAdmin, []solana.Instruction{ixn}, input.Router, ContractType.String())
		if err != nil {
			return TokenAdminRegistryOut{}, err
		}
		return TokenAdminRegistryOut{PendingSigner: input.Admin, OnChainOutput: output}, nil
	},
)

// GetAuthority returns the owner of program, or the deployer key before the program is initialized.
func GetAuthority(ctx context.Context, chain cldf_solana.Chain, program solana.PublicKey) (solana.PublicKey, error) {
	configPDA, _, err := state.FindConfigPDA(program)
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("failed to derive config PDA: %w", err)
	}
	return solcommon.ConfigAuthority(ctx, chain, program, configPDA, ccip_router.ParseAccount_Config, func(c *ccip_router.Config) solana.PublicKey { return c.Owner })
}

func toResolutionType(resolution uint8) (*ccip_router.ResolutionType, error) {
	if resolution > 2 {
		return nil, fmt.Errorf("invalid resolution type: %d", resolution)
	}
	rt := ccip_router.ResolutionType(resolution)
	return &rt, nil
}

type Params struct {
	FeeQuoter solana.PublicKey
	Router    solana.PublicKey
	LinkToken solana.PublicKey
	RMNRemote solana.PublicKey
}

type PoolParams struct {
	ExistingAddresses []datastore.AddressRef
	Router            solana.PublicKey
	FeeQuoter         solana.PublicKey
	TokenMint         solana.PublicKey
	TokenProgramID    solana.PublicKey
	TokenPool         solana.PublicKey
	TokenPoolType     string
	// TokenSymbol qualifies the TokenPoolLookupTable ref this operation records. A token has
	// exactly one pool per environment (enforced by the router's TokenAdminRegistry PDA), so
	// the symbol alone identifies the table.
	//
	// It is caller-declared rather than derived: the AddressBook-to-Datastore migration
	// requires semantic qualifiers, and deriving one from the mint would anchor the datastore
	// key on an address.
	TokenSymbol string
	// WritableIndexes overrides the default {3, 4, 7} (pool_config, pool_token_account, mint)
	// writable set recorded in the TokenAdminRegistry for this pool's lookup table.
	WritableIndexes []uint8
}

type TokenAdminRegistryParams struct {
	Router            solana.PublicKey
	TokenMint         solana.PublicKey
	Admin             solana.PublicKey
	ExistingAddresses []datastore.AddressRef
}

type SetPoolInterfaceImplParams struct {
	Router                      solana.PublicKey
	TokenMint                   solana.PublicKey
	InterfaceVersion            uint8
	ImplementsAccountResolution uint8
}

type TokenAdminRegistryOut struct {
	sequences.OnChainOutput
	PendingSigner solana.PublicKey
}
