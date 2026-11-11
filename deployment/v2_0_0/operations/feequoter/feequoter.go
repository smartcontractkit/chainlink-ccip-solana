package feequoter

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/Masterminds/semver/v3"
	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"

	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/sequences"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	cldf_deployment "github.com/smartcontractkit/chainlink-deployments-framework/deployment"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/fee_quoter"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/tokens"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
)

var (
	ContractType cldf_deployment.ContractType = "FeeQuoter"
	ProgramName                               = "fee_quoter"
	Version      *semver.Version              = solcommon.Version2_0_0
)

type ConnectChainsParams struct {
	LocalChainSelector  uint64
	FeeQuoter           solana.PublicKey
	OffRamp             solana.PublicKey
	RemoteChainSelector uint64
	DestChainConfig     fee_quoter.DestChainConfigV2
}

type Params struct {
	MaxFeeJuelsPerMsg bin.Uint128
	FeeQuoter         solana.PublicKey
	Router            solana.PublicKey
	OffRamp           solana.PublicKey
	LinkToken         solana.PublicKey
}

type SetTokenTransferFeeConfigInput struct {
	SrcSelector uint64
	DstSelector uint64
	FeeQuoter   solana.PublicKey
	Config      fee_quoter.TokenTransferFeeConfig
	Token       solana.PublicKey
}

var Deploy = operations.NewOperation(
	"fee-quoter:deploy",
	Version,
	"Deploys the FeeQuoter program",
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
	"fee-quoter:upgrade",
	Version,
	"Upgrades the default FeeQuoter program",
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
	"fee-quoter:initialize",
	Version,
	"Initializes the FeeQuoter contract",
	func(b operations.Bundle, chain cldf_solana.Chain, input Params) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&fee_quoter.ProgramID, fee_quoter.ParseInstructionWithoutAccounts, input.FeeQuoter)
		programData, err := solcommon.GetSolProgramData(chain.Client, input.FeeQuoter)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to get program data: %w", err)
		}
		authority, err := GetAuthority(b.GetContext(), chain, input.FeeQuoter)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		feeQuoterConfigPDA, _, _ := state.FindFqConfigPDA(input.FeeQuoter)
		instruction, err := fee_quoter.NewInitializeInstruction(
			input.MaxFeeJuelsPerMsg,
			input.Router,
			feeQuoterConfigPDA,
			input.LinkToken,
			authority,
			solana.SystemProgramID,
			input.FeeQuoter,
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

var AddPriceUpdater = operations.NewOperation(
	"fee-quoter:add-price-updater",
	Version,
	"Adds a price updater to the FeeQuoter contract",
	func(b operations.Bundle, chain cldf_solana.Chain, input Params) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&fee_quoter.ProgramID, fee_quoter.ParseInstructionWithoutAccounts, input.FeeQuoter)
		authority, err := GetAuthority(b.GetContext(), chain, input.FeeQuoter)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		feeQuoterConfigPDA, _, _ := state.FindFqConfigPDA(input.FeeQuoter)
		offRampBillingSignerPDA, _, _ := state.FindOfframpBillingSignerPDA(input.OffRamp)
		fqAllowedPriceUpdaterOfframpPDA, _, _ := state.FindFqAllowedPriceUpdaterPDA(offRampBillingSignerPDA, input.FeeQuoter)
		instruction, err := fee_quoter.NewAddPriceUpdaterInstruction(
			offRampBillingSignerPDA,
			fqAllowedPriceUpdaterOfframpPDA,
			feeQuoterConfigPDA,
			authority,
			solana.SystemProgramID,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to build add price updater instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{instruction}, input.FeeQuoter, ContractType.String())
	},
)

type AddBillingTokenConfigInput struct {
	FeeQuoter    solana.PublicKey
	Router       solana.PublicKey
	Mint         solana.PublicKey
	TokenProgram solana.PublicKey
	// UsdPerToken is the token price as 18-decimals USD for 1e18 smallest token
	// denominations (e.g. $150/SOL with 9 decimals -> 150e9 * 1e18 = 1.5e29).
	UsdPerToken                *big.Int
	PremiumMultiplierWeiPerEth uint64
	// Timestamp is the price time in unix seconds. Set it for MCMS proposals so they are
	// deterministic; zero means now.
	Timestamp int64
}

// AddBillingTokenConfig registers a fee (billing) token on the FeeQuoter with an initial USD
// price. Since fee accounting became real in the 2.0 fee quoter, `ccip_send_v2`/`get_fee_v2`
// CPI into the FeeQuoter for a validated fee token price, so every fee token needs this
// config (with a non-zero price and timestamp) before any message can be sent.
var AddBillingTokenConfig = operations.NewOperation(
	"fee-quoter:add-billing-token-config",
	Version,
	"Registers a billing (fee) token with its USD price on the FeeQuoter contract",
	func(b operations.Bundle, chain cldf_solana.Chain, input AddBillingTokenConfigInput) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&fee_quoter.ProgramID, fee_quoter.ParseInstructionWithoutAccounts, input.FeeQuoter)
		authority, err := GetAuthority(b.GetContext(), chain, input.FeeQuoter)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		feeQuoterConfigPDA, _, _ := state.FindFqConfigPDA(input.FeeQuoter)
		billingTokenConfigPDA, _, err := state.FindFqBillingTokenConfigPDA(input.Mint, input.FeeQuoter)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive billing token config PDA for %s: %w", input.Mint, err)
		}
		feeBillingSignerPDA, _, err := state.FindFeeBillingSignerPDA(input.Router)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive fee billing signer PDA: %w", err)
		}
		// The instruction creates the fee receiver as the billing signer's ATA for the mint.
		feeTokenReceiverATA, _, err := tokens.FindAssociatedTokenAddress(input.TokenProgram, input.Mint, feeBillingSignerPDA)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive fee token receiver ATA for %s: %w", input.Mint, err)
		}
		usdPerToken, err := bigIntTo28Bytes(input.UsdPerToken)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("billing token %s price: %w", input.Mint, err)
		}
		timestamp := input.Timestamp
		if timestamp == 0 {
			timestamp = time.Now().Unix()
		}
		instruction, err := fee_quoter.NewAddBillingTokenConfigInstruction(
			fee_quoter.BillingTokenConfig{
				Enabled: true,
				Mint:    input.Mint,
				UsdPerToken: fee_quoter.TimestampedPackedU224{
					Value:     usdPerToken,
					Timestamp: timestamp,
				},
				PremiumMultiplierWeiPerEth: input.PremiumMultiplierWeiPerEth,
			},
			feeQuoterConfigPDA,
			billingTokenConfigPDA,
			input.TokenProgram,
			input.Mint,
			feeTokenReceiverATA,
			authority,
			feeBillingSignerPDA,
			tokens.AssociatedTokenProgramID,
			solana.SystemProgramID,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to build add billing token config instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{instruction}, input.FeeQuoter, ContractType.String())
	},
)

// BillingTokenExists reports whether mint already has a billing token config on feeQuoter.
func BillingTokenExists(ctx context.Context, chain cldf_solana.Chain, feeQuoter, mint solana.PublicKey) (bool, error) {
	pda, _, err := state.FindFqBillingTokenConfigPDA(mint, feeQuoter)
	if err != nil {
		return false, fmt.Errorf("failed to derive billing token config PDA for %s: %w", mint, err)
	}
	cfg, err := common.GetInitializedAccount(ctx, chain.Client, pda, feeQuoter, cldf_solana.SolDefaultCommitment, fee_quoter.ParseAccount_BillingTokenConfigWrapper)
	if err != nil {
		return false, fmt.Errorf("read billing token config for %s: %w", mint, err)
	}
	return cfg != nil, nil
}

// UpdateBillingTokenConfig updates the price and premium of a billing token that is already
// registered on the FeeQuoter, and enables it.
var UpdateBillingTokenConfig = operations.NewOperation(
	"fee-quoter:update-billing-token-config",
	Version,
	"Updates a registered billing (fee) token's price and premium on the FeeQuoter contract",
	func(b operations.Bundle, chain cldf_solana.Chain, input AddBillingTokenConfigInput) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&fee_quoter.ProgramID, fee_quoter.ParseInstructionWithoutAccounts, input.FeeQuoter)
		authority, err := GetAuthority(b.GetContext(), chain, input.FeeQuoter)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		feeQuoterConfigPDA, _, _ := state.FindFqConfigPDA(input.FeeQuoter)
		billingTokenConfigPDA, _, err := state.FindFqBillingTokenConfigPDA(input.Mint, input.FeeQuoter)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive billing token config PDA for %s: %w", input.Mint, err)
		}
		usdPerToken, err := bigIntTo28Bytes(input.UsdPerToken)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("billing token %s price: %w", input.Mint, err)
		}
		timestamp := input.Timestamp
		if timestamp == 0 {
			timestamp = time.Now().Unix()
		}
		instruction, err := fee_quoter.NewUpdateBillingTokenConfigInstruction(
			fee_quoter.BillingTokenConfig{
				Enabled: true,
				Mint:    input.Mint,
				UsdPerToken: fee_quoter.TimestampedPackedU224{
					Value:     usdPerToken,
					Timestamp: timestamp,
				},
				PremiumMultiplierWeiPerEth: input.PremiumMultiplierWeiPerEth,
			},
			feeQuoterConfigPDA,
			billingTokenConfigPDA,
			authority,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to build update billing token config instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{instruction}, input.FeeQuoter, ContractType.String())
	},
)

var ConnectChains = operations.NewOperation(
	"fee-quoter:connect-chains",
	Version,
	"Connects the FeeQuoter contract to other chains",
	func(b operations.Bundle, chain cldf_solana.Chain, input ConnectChainsParams) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&fee_quoter.ProgramID, fee_quoter.ParseInstructionWithoutAccounts, input.FeeQuoter)
		authority, err := GetAuthority(b.GetContext(), chain, input.FeeQuoter)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		feeQuoterConfigPDA, _, _ := state.FindFqConfigPDA(input.FeeQuoter)
		fqRemoteChainPDA, _, _ := state.FindFqDestChainV2PDA(input.RemoteChainSelector, input.FeeQuoter)
		eventAuthority, _, _ := state.FindEventAuthorityPDA(input.FeeQuoter)
		ixn, err := fee_quoter.NewUpsertDestChainConfigV2Instruction(
			fee_quoter.UpsertDestChainConfigV2Params{
				ChainSelector:   input.RemoteChainSelector,
				DestChainConfig: input.DestChainConfig,
			},
			feeQuoterConfigPDA,
			fqRemoteChainPDA,
			authority,
			solana.SystemProgramID,
			eventAuthority,
			input.FeeQuoter,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to build dest chain config v2 instruction: %w", err)
		}
		err = solcommon.ExtendOfframpLookupTable(chain, input.OffRamp, []solana.PublicKey{fqRemoteChainPDA})
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to extend OffRamp lookup table: %w", err)
		}
		out, err := solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{ixn}, input.FeeQuoter, ContractType.String())
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		return out, nil
	},
)

type DisableDestChainParams struct {
	FeeQuoter           solana.PublicKey
	RemoteChainSelector uint64
}

var DisableDestChain = operations.NewOperation(
	"fee-quoter:disable-dest-chain",
	Version,
	"Disables a destination chain on the FeeQuoter, preventing sending to that chain",
	func(b operations.Bundle, chain cldf_solana.Chain, input DisableDestChainParams) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&fee_quoter.ProgramID, fee_quoter.ParseInstructionWithoutAccounts, input.FeeQuoter)
		authority, err := GetAuthority(b.GetContext(), chain, input.FeeQuoter)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		feeQuoterConfigPDA, _, _ := state.FindFqConfigPDA(input.FeeQuoter)
		fqDestChainPDA, _, _ := state.FindFqDestChainV2PDA(input.RemoteChainSelector, input.FeeQuoter)
		eventAuthority, _, _ := state.FindEventAuthorityPDA(input.FeeQuoter)

		instruction, err := fee_quoter.NewDisableDestChainV2Instruction(
			input.RemoteChainSelector,
			feeQuoterConfigPDA,
			fqDestChainPDA,
			authority,
			eventAuthority,
			input.FeeQuoter,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to build update dest chain config v2 to disable chain instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{instruction}, input.FeeQuoter, ContractType.String())
	},
)

var TransferOwnership = operations.NewOperation(
	"fee-quoter:transfer-ownership",
	Version,
	"Transfers ownership of the FeeQuoter contract to a new authority",
	func(b operations.Bundle, chain cldf_solana.Chain, input solcommon.TransferOwnershipParams) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&fee_quoter.ProgramID, fee_quoter.ParseInstructionWithoutAccounts, input.Program)
		authority, err := GetAuthority(b.GetContext(), chain, input.Program)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		if authority != input.CurrentOwner {
			return sequences.OnChainOutput{}, fmt.Errorf("current owner %s does not match on-chain authority %s", input.CurrentOwner.String(), authority.String())
		}
		configPDA, _, _ := state.FindConfigPDA(input.Program)
		ixn, err := fee_quoter.NewTransferOwnershipInstruction(
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
	"fee-quoter:accept-ownership",
	Version,
	"Accepts ownership of the FeeQuoter contract",
	func(b operations.Bundle, chain cldf_solana.Chain, input solcommon.TransferOwnershipParams) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&fee_quoter.ProgramID, fee_quoter.ParseInstructionWithoutAccounts, input.Program)
		configPDA, _, _ := state.FindConfigPDA(input.Program)
		ixn, err := fee_quoter.NewAcceptOwnershipInstruction(
			configPDA,
			input.NewOwner,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to build accept ownership instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, input.NewOwner, []solana.Instruction{ixn}, input.Program, ContractType.String())
	},
)

type UpdatePricesInput struct {
	FeeQuoter         solana.PublicKey
	GasPriceUpdates   []GasPriceUpdateInput
	TokenPriceUpdates []TokenPriceUpdateWithMint
}

// GasPriceUpdateInput holds a gas price update expressed as a *big.Int (18 decimals, USD).
type GasPriceUpdateInput struct {
	DestChainSelector uint64
	UsdPerUnitGas     *big.Int
}

// TokenPriceUpdateWithMint bundles a token price update with the mint public key
// used to derive the billing token config PDA that must be appended as an additional account.
type TokenPriceUpdateWithMint struct {
	Mint        solana.PublicKey
	UsdPerToken *big.Int
}

var UpdatePrices = operations.NewOperation(
	"fee-quoter:update-prices",
	Version,
	"Updates gas and token prices on the FeeQuoter contract",
	func(b operations.Bundle, chain cldf_solana.Chain, input UpdatePricesInput) (sequences.OnChainOutput, error) {
		authority, err := GetAuthority(b.GetContext(), chain, input.FeeQuoter)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		feeQuoterConfigPDA, _, _ := state.FindFqConfigPDA(input.FeeQuoter)
		allowedPriceUpdaterPDA, _, _ := state.FindFqAllowedPriceUpdaterPDA(authority, input.FeeQuoter)

		// Convert input types to the fee_quoter binding types.
		gasUpdates := make([]fee_quoter.GasPriceUpdate, 0, len(input.GasPriceUpdates))
		for _, gu := range input.GasPriceUpdates {
			usdPerUnitGas, convErr := bigIntTo28Bytes(gu.UsdPerUnitGas)
			if convErr != nil {
				return sequences.OnChainOutput{}, fmt.Errorf("gas price for dest chain %d: %w", gu.DestChainSelector, convErr)
			}
			gasUpdates = append(gasUpdates, fee_quoter.GasPriceUpdate{
				DestChainSelector: gu.DestChainSelector,
				UsdPerUnitGas:     usdPerUnitGas,
			})
		}
		tokenUpdates := make([]fee_quoter.TokenPriceUpdate, 0, len(input.TokenPriceUpdates))
		for _, tu := range input.TokenPriceUpdates {
			usdPerToken, convErr := bigIntTo28Bytes(tu.UsdPerToken)
			if convErr != nil {
				return sequences.OnChainOutput{}, fmt.Errorf("token price for %s: %w", tu.Mint, convErr)
			}
			tokenUpdates = append(tokenUpdates, fee_quoter.TokenPriceUpdate{
				SourceToken: tu.Mint,
				UsdPerToken: usdPerToken,
			})
		}

		raw, err := fee_quoter.NewUpdatePricesInstruction(
			tokenUpdates,
			gasUpdates,
			authority,
			allowedPriceUpdaterPDA,
			feeQuoterConfigPDA,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to build update prices instruction: %w", err)
		}

		// Append billing token config PDAs (one per token update, writable)
		genericRaw, ok := raw.(*solana.GenericInstruction)
		if !ok {
			return sequences.OnChainOutput{}, fmt.Errorf("unexpected instruction type: %T", raw)
		}
		for _, tu := range input.TokenPriceUpdates {
			billingPDA, _, pdaErr := state.FindFqBillingTokenConfigPDA(tu.Mint, input.FeeQuoter)
			if pdaErr != nil {
				return sequences.OnChainOutput{}, fmt.Errorf("failed to derive billing token config PDA for %s: %w", tu.Mint.String(), pdaErr)
			}
			genericRaw.AccountValues = append(genericRaw.AccountValues, solana.Meta(billingPDA).WRITE())
		}

		// Append dest chain PDAs (one per gas update, writable). update_prices writes gas prices
		// to the 1.6 dest_chain account, which is also where quote_gas_for_exec_v2 reads them
		// from until a 2.0 price updater exists.
		for _, gu := range input.GasPriceUpdates {
			destChainPDA, _, pdaErr := state.FindFqDestChainPDA(gu.DestChainSelector, input.FeeQuoter)
			if pdaErr != nil {
				return sequences.OnChainOutput{}, fmt.Errorf("failed to derive dest chain PDA for selector %d: %w", gu.DestChainSelector, pdaErr)
			}
			genericRaw.AccountValues = append(genericRaw.AccountValues, solana.Meta(destChainPDA).WRITE())
		}

		genericRaw.ProgID = input.FeeQuoter
		return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{raw}, input.FeeQuoter, ContractType.String())
	},
)

// bigIntTo28Bytes converts a *big.Int to a 28-byte big-endian array as expected by FeeQuoter.
// Invalid inputs now fail fast instead of being silently zeroed or truncated.
func bigIntTo28Bytes(v *big.Int) ([28]uint8, error) {
	var result [28]uint8
	if v == nil {
		return result, errors.New("invalid 28-byte integer: nil value")
	}
	if v.Sign() < 0 {
		return result, fmt.Errorf("invalid 28-byte integer: negative value %s", v)
	}
	if v.BitLen() > 224 {
		return result, fmt.Errorf("invalid 28-byte integer: bit length %d exceeds 224", v.BitLen())
	}

	b := v.Bytes() // big-endian
	copy(result[28-len(b):], b)
	return result, nil
}

var SetTokenTransferFeeConfig = operations.NewOperation(
	"fee-quoter:set-token-transfer-fee-config",
	Version,
	"Sets token transfer fee config on the FeeQuoter contract",
	func(b operations.Bundle, chain cldf_solana.Chain, input SetTokenTransferFeeConfigInput) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&fee_quoter.ProgramID, fee_quoter.ParseInstructionWithoutAccounts, input.FeeQuoter)
		authority, err := GetAuthority(b.GetContext(), chain, input.FeeQuoter)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		feeQuoterConfigPDA, _, _ := state.FindFqConfigPDA(input.FeeQuoter)
		remoteBillingPDA, _, _ := state.FindFqPerChainPerTokenConfigPDA(input.DstSelector, input.Token, input.FeeQuoter)
		instruction, err := fee_quoter.NewSetTokenTransferFeeConfigInstruction(
			input.DstSelector,
			input.Token,
			input.Config,
			feeQuoterConfigPDA,
			remoteBillingPDA,
			authority,
			solana.SystemProgramID,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to build set token transfer fee config instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{instruction}, input.FeeQuoter, ContractType.String())
	},
)

type AddLegacyDestChainParams struct {
	FeeQuoter           solana.PublicKey
	RemoteChainSelector uint64
	DestChainConfig     fee_quoter.DestChainConfig
}

var AddLegacyDestChain = operations.NewOperation(
	"fee-quoter:add-legacy-dest-chain",
	Version,
	"Creates the 1.6 legacy DestChain PDA that QuoteGasForExecV2 reads gas prices from",
	func(b operations.Bundle, chain cldf_solana.Chain, input AddLegacyDestChainParams) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&fee_quoter.ProgramID, fee_quoter.ParseInstructionWithoutAccounts, input.FeeQuoter)
		authority, err := GetAuthority(b.GetContext(), chain, input.FeeQuoter)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		feeQuoterConfigPDA, _, _ := state.FindFqConfigPDA(input.FeeQuoter)
		destChainPDA, _, _ := state.FindFqDestChainPDA(input.RemoteChainSelector, input.FeeQuoter)

		ixn, err := fee_quoter.NewAddDestChainInstruction(
			input.RemoteChainSelector,
			input.DestChainConfig,
			feeQuoterConfigPDA,
			destChainPDA,
			authority,
			solana.SystemProgramID,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to build add legacy dest chain instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{ixn}, input.FeeQuoter, ContractType.String())
	},
)

// V1DestChainConfigFromV2 projects a v2 dest chain config onto the v1 shape.
// The v2 flow reads gas prices from the v1 account, so the v1 PDA must exist
// with a valid config for each lane the v2 flow quotes.
func V1DestChainConfigFromV2(cfg fee_quoter.DestChainConfigV2) fee_quoter.DestChainConfig {
	return fee_quoter.DestChainConfig{
		IsEnabled:                   cfg.IsEnabled,
		LaneCodeVersion:             fee_quoter.CodeVersion_V1,
		MaxNumberOfTokensPerMsg:     1,
		MaxDataBytes:                cfg.MaxDataBytes,
		MaxPerMsgGasLimit:           cfg.MaxPerMsgGasLimit,
		DestGasOverhead:             cfg.DestGasOverhead,
		DestGasPerPayloadByteBase:   fee_quoter.CrossChainGas{V0: cfg.DestGasPerPayloadByteBase},
		DefaultTokenFeeUsdcents:     cfg.DefaultTokenFeeUsdCents,
		DefaultTokenDestGasOverhead: cfg.DefaultTokenDestGasOverhead,
		DefaultTxGasLimit:           cfg.DefaultTxGasLimit,
		NetworkFee:                  cfg.NetworkFee,
		ChainFamilySelector:         cfg.ChainFamilySelector,
	}
}

// GetAuthority returns the owner of program, or the deployer key before the program is initialized.
func GetAuthority(ctx context.Context, chain cldf_solana.Chain, program solana.PublicKey) (solana.PublicKey, error) {
	configPDA, _, err := state.FindFqConfigPDA(program)
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("failed to derive config PDA: %w", err)
	}
	return solcommon.ConfigAuthority(ctx, chain, program, configPDA, fee_quoter.ParseAccount_Config, func(c *fee_quoter.Config) solana.PublicKey { return c.Owner })
}
