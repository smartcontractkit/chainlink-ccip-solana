// fee_quoter_config_test.go tests the configuration and setup of the Fee Quoter program.
// This includes initialization, price updates, token pricing configuration, destination chain config,
// and premium multiplier settings for CCIP fee calculation.
package contracts

import (
	"encoding/binary"
	"math"
	"math/big"
	"testing"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/ccip_router"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/fee_quoter"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/config"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/testutils"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/ccip"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/tokens"
)

func TestFeeQuoterConfig(t *testing.T) {
	env := testutils.NewTestEnv(t).WithTokens().Build()
	ctx := env.Ctx()
	client := env.Client
	admin := testutils.Admin
	user := testutils.User
	ccipAdmin := testutils.CcipAdmin

	initialLinkMint := testutils.Token0MintPrivK.PublicKey()
	token0Decimals := uint8(9)
	link22Decimals := uint8(9)

	initialMaxFeeJuelsPerMsg := bin.Uint128{Lo: 100000000000000000, Hi: 0, Endianness: nil}

	// ----- Initialize FeeQuoter ----- //
	t.Run("FeeQuoter is initialized", func(t *testing.T) {
		fqProgramDataAddress := testutils.GetProgramDataAddress(ctx, t, client, config.FeeQuoterProgram)

		ix, err := fee_quoter.NewInitializeInstruction(
			initialMaxFeeJuelsPerMsg,
			config.CcipRouterProgram,
			config.FqConfigPDA,
			initialLinkMint, // to be changed in the next tests
			admin.PublicKey(),
			solana.SystemProgramID,
			config.FeeQuoterProgram,
			fqProgramDataAddress,
		)
		require.NoError(t, err)

		result := testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{ix}, admin, config.DefaultCommitment)
		require.NotNil(t, result)

		configSetEvent, err := common.ParseEventFromLogs(result.Meta.LogMessages, fee_quoter.ParseEvent_ConfigSet)
		require.NoError(t, err)
		require.Equal(t, initialMaxFeeJuelsPerMsg, configSetEvent.MaxFeeJuelsPerMsg)
		require.Equal(t, initialLinkMint, configSetEvent.LinkTokenMint)
		require.Equal(t, uint8(9), configSetEvent.LinkTokenLocalDecimals)
		require.Equal(t, config.CcipRouterProgram, configSetEvent.Onramp)
		require.Equal(t, fee_quoter.CodeVersion_V1, configSetEvent.DefaultCodeVersion)

		var fqConfig fee_quoter.Config
		require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, client, config.FqConfigPDA, config.DefaultCommitment, &fqConfig))
		require.Equal(t, initialLinkMint, fqConfig.LinkTokenMint)
		require.Equal(t, token0Decimals, fqConfig.LinkTokenLocalDecimals)
		require.Equal(t, initialMaxFeeJuelsPerMsg, fqConfig.MaxFeeJuelsPerMsg)
		require.Equal(t, admin.PublicKey(), fqConfig.Owner)
		require.True(t, fqConfig.ProposedOwner.IsZero())
		require.Equal(t, config.CcipRouterProgram, fqConfig.Onramp)
	})

	// ----- Update Link Mint ----- //
	t.Run("FeeQuoter: Update link mint", func(t *testing.T) {
		t.Run("When a non-admin tries to make the update, it fails", func(t *testing.T) {
			ix, err := fee_quoter.NewSetLinkTokenMintInstruction(
				config.FqConfigPDA,
				env.Link22.Mint,
				user.PublicKey(),
			)
			require.NoError(t, err)
			testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{ix}, user, config.DefaultCommitment, []string{ccip.Unauthorized_CcipRouterError.String()})
		})

		t.Run("When an admin tries to make the update, it succeeds", func(t *testing.T) {
			var configAccount fee_quoter.Config
			require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, client, config.FqConfigPDA, config.DefaultCommitment, &configAccount))
			require.Equal(t, initialLinkMint, configAccount.LinkTokenMint)
			require.Equal(t, token0Decimals, configAccount.LinkTokenLocalDecimals)

			ix, err := fee_quoter.NewSetLinkTokenMintInstruction(
				config.FqConfigPDA,
				env.Link22.Mint,
				admin.PublicKey(),
			)
			require.NoError(t, err)

			result := testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{ix}, admin, config.DefaultCommitment)
			require.NotNil(t, result)

			configSetEvent, err := common.ParseEventFromLogs(result.Meta.LogMessages, fee_quoter.ParseEvent_ConfigSet)
			require.NoError(t, err)
			require.Equal(t, env.Link22.Mint, configSetEvent.LinkTokenMint)
			require.Equal(t, link22Decimals, configSetEvent.LinkTokenLocalDecimals)

			require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, client, config.FqConfigPDA, config.DefaultCommitment, &configAccount))
			require.Equal(t, env.Link22.Mint, configAccount.LinkTokenMint)
			require.Equal(t, link22Decimals, configAccount.LinkTokenLocalDecimals)
		})
	})

	// ----- Update Max Fee Juels Per Msg ----- //
	t.Run("FeeQuoter: Update max fee juels per msg", func(t *testing.T) {
		defaultMaxFeeJuelsPerMsg := bin.Uint128{Lo: 300000000000000000, Hi: 0, Endianness: nil}

		t.Run("When a non-admin tries to make the update, it fails", func(t *testing.T) {
			ix, err := fee_quoter.NewSetMaxFeeJuelsPerMsgInstruction(
				defaultMaxFeeJuelsPerMsg,
				config.FqConfigPDA,
				user.PublicKey(),
			)
			require.NoError(t, err)
			testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{ix}, user, config.DefaultCommitment, []string{ccip.Unauthorized_CcipRouterError.String()})
		})

		t.Run("When an admin tries to make the update, it succeeds", func(t *testing.T) {
			var fqConfig fee_quoter.Config
			require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, client, config.FqConfigPDA, config.DefaultCommitment, &fqConfig))
			require.NotEqual(t, defaultMaxFeeJuelsPerMsg, fqConfig.MaxFeeJuelsPerMsg)

			ix, err := fee_quoter.NewSetMaxFeeJuelsPerMsgInstruction(
				defaultMaxFeeJuelsPerMsg,
				config.FqConfigPDA,
				admin.PublicKey(),
			)
			require.NoError(t, err)
			result := testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{ix}, admin, config.DefaultCommitment)
			require.NotNil(t, result)

			configSetEvent, err := common.ParseEventFromLogs(result.Meta.LogMessages, fee_quoter.ParseEvent_ConfigSet)
			require.NoError(t, err)
			require.Equal(t, defaultMaxFeeJuelsPerMsg, configSetEvent.MaxFeeJuelsPerMsg)

			require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, client, config.FqConfigPDA, config.DefaultCommitment, &fqConfig))
			require.Equal(t, defaultMaxFeeJuelsPerMsg, fqConfig.MaxFeeJuelsPerMsg)
		})
	})

	// ----- Add Offramp as Price Updater ----- //
	t.Run("FeeQuoter: Add offramp as price updater", func(t *testing.T) {
		testutils.AssertClosedAccount(ctx, t, client, config.FqAllowedPriceUpdaterOfframpPDA, config.DefaultCommitment)

		ix, err := fee_quoter.NewAddPriceUpdaterInstruction(
			config.OfframpBillingSignerPDA,
			config.FqAllowedPriceUpdaterOfframpPDA,
			config.FqConfigPDA,
			admin.PublicKey(),
			solana.SystemProgramID,
		)
		require.NoError(t, err)

		testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{ix}, admin, config.DefaultCommitment)

		isClosed := common.IsClosedAccount(ctx, client, config.FqAllowedPriceUpdaterOfframpPDA, config.DefaultCommitment)
		require.False(t, isClosed)
	})

	// ----- Invalid Chain Configs ----- //
	t.Run("Fee Quoter: When admin adds a chain selector with invalid dest chain config, it fails", func(t *testing.T) {
		validFqDestChainConfig := env.ValidFqDestChainConfig

		invalidInputTests := []struct {
			Name         string
			Selector     uint64
			Conf         fee_quoter.DestChainConfig
			SkipOnUpdate bool
			Error        string
		}{
			{
				Name:     "Zero DefaultTxGasLimit",
				Selector: config.EvmChainSelector,
				Conf: fee_quoter.DestChainConfig{
					DefaultTxGasLimit:   fee_quoter.CrossChainGas{V0: 0},
					MaxPerMsgGasLimit:   validFqDestChainConfig.MaxPerMsgGasLimit,
					ChainFamilySelector: validFqDestChainConfig.ChainFamilySelector,
				},
				Error: ccip.ZeroGasLimit_FeeQuoterError.String(),
			},
			{
				Name:         "Zero DestChainSelector",
				Selector:     0,
				Conf:         validFqDestChainConfig,
				SkipOnUpdate: true,
				Error:        ccip.InvalidInputsChainSelector_FeeQuoterError.String(),
			},
			{
				Name:     "Zero ChainFamilySelector",
				Selector: config.EvmChainSelector,
				Conf: fee_quoter.DestChainConfig{
					DefaultTxGasLimit:   validFqDestChainConfig.DefaultTxGasLimit,
					MaxPerMsgGasLimit:   validFqDestChainConfig.MaxPerMsgGasLimit,
					ChainFamilySelector: [4]uint8{0, 0, 0, 0},
				},
				Error: ccip.InvalidChainFamilySelector_FeeQuoterError.String(),
			},
			{
				Name:     "DefaultTxGasLimit > MaxPerMsgGasLimit",
				Selector: config.EvmChainSelector,
				Conf: fee_quoter.DestChainConfig{
					DefaultTxGasLimit:   fee_quoter.CrossChainGas{V0: 100},
					MaxPerMsgGasLimit:   fee_quoter.CrossChainGas{V0: 1},
					ChainFamilySelector: validFqDestChainConfig.ChainFamilySelector,
				},
				Error: ccip.DefaultGasLimitExceedsMaximum_FeeQuoterError.String(),
			},
		}

		for _, test := range invalidInputTests {
			t.Run(test.Name, func(t *testing.T) {
				destChainPDA, _, derr := state.FindFqDestChainPDA(test.Selector, config.FeeQuoterProgram)
				require.NoError(t, derr)

				instruction, err := fee_quoter.NewAddDestChainInstruction(
					test.Selector,
					test.Conf,
					config.FqConfigPDA,
					destChainPDA,
					admin.PublicKey(),
					solana.SystemProgramID,
				)
				require.NoError(t, err)
				result := testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{instruction}, admin, config.DefaultCommitment, []string{"Error Code: " + test.Error})
				require.NotNil(t, result)
			})
		}
	})

	// ----- Add Chain Selectors ----- //
	t.Run("When an unauthorized user tries to add a chain selector, it fails", func(t *testing.T) {
		instruction, err := fee_quoter.NewAddDestChainInstruction(
			config.EvmChainSelector,
			env.ValidFqDestChainConfig,
			config.FqConfigPDA,
			config.FqEvmDestChainPDA,
			user.PublicKey(), // not an admin
			solana.SystemProgramID,
		)
		require.NoError(t, err)
		result := testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{instruction}, user, config.DefaultCommitment, []string{"Error Code: " + ccip.Unauthorized_FeeQuoterError.String()})
		require.NotNil(t, result)
	})

	t.Run("When admin adds EVM chain selector it's added", func(t *testing.T) {
		instruction, err := fee_quoter.NewAddDestChainInstruction(
			config.EvmChainSelector,
			env.ValidFqDestChainConfig,
			config.FqConfigPDA,
			config.FqEvmDestChainPDA,
			admin.PublicKey(),
			solana.SystemProgramID,
		)
		require.NoError(t, err)
		result := testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{instruction}, admin, config.DefaultCommitment)
		require.NotNil(t, result)

		var destChainAccount fee_quoter.DestChain
		err = common.GetAccountDataBorshIntoAnchor(ctx, client, config.FqEvmDestChainPDA, config.DefaultCommitment, &destChainAccount)
		require.NoError(t, err, "failed to get account info")
		require.Equal(t, fee_quoter.TimestampedPackedU224{}, destChainAccount.State.UsdPerUnitGas)
		require.Equal(t, env.ValidFqDestChainConfig, destChainAccount.Config)
	})

	t.Run("When admin adds SVM chain selector it's also added", func(t *testing.T) {
		instruction, err := fee_quoter.NewAddDestChainInstruction(
			config.SvmChainSelector,
			fee_quoter.DestChainConfig{
				IsEnabled: true,
				// minimal valid config
				DefaultTxGasLimit:       fee_quoter.CrossChainGas{V0: 1},
				MaxPerMsgGasLimit:       fee_quoter.CrossChainGas{V0: 100},
				ChainFamilySelector:     [4]uint8(config.SvmChainFamilySelector),
				EnforceOutOfOrder:       true,
				MaxNumberOfTokensPerMsg: 10,
				MaxDataBytes:            100,
			},
			config.FqConfigPDA,
			config.FqSvmDestChainPDA,
			admin.PublicKey(),
			solana.SystemProgramID,
		)
		require.NoError(t, err)
		result := testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{instruction}, admin, config.DefaultCommitment)
		require.NotNil(t, result)

		var destChainStateAccount fee_quoter.DestChain
		err = common.GetAccountDataBorshIntoAnchor(ctx, client, config.FqSvmDestChainPDA, config.DefaultCommitment, &destChainStateAccount)
		require.NoError(t, err, "failed to get account info")
		require.Equal(t, fee_quoter.TimestampedPackedU224{}, destChainStateAccount.State.UsdPerUnitGas)
	})

	// ----- Disable/Enable Dest Chain ----- //
	t.Run("When a non-admin tries to disable the dest chain selector, it fails", func(t *testing.T) {
		ix, err := fee_quoter.NewDisableDestChainInstruction(
			config.EvmChainSelector,
			config.FqConfigPDA,
			config.FqEvmDestChainPDA,
			user.PublicKey(),
		)
		require.NoError(t, err)
		result := testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{ix}, user, config.DefaultCommitment, []string{"Error Code: " + ccip.Unauthorized_CcipRouterError.String()})
		require.NotNil(t, result)
	})

	t.Run("When an admin disables the dest chain selector, it is no longer enabled", func(t *testing.T) {
		var initial fee_quoter.DestChain
		err := common.GetAccountDataBorshIntoAnchor(ctx, client, config.FqEvmDestChainPDA, config.DefaultCommitment, &initial)
		require.NoError(t, err, "failed to get account info")
		require.True(t, initial.Config.IsEnabled)

		ix, err := fee_quoter.NewDisableDestChainInstruction(
			config.EvmChainSelector,
			config.FqConfigPDA,
			config.FqEvmDestChainPDA,
			admin.PublicKey(),
		)
		require.NoError(t, err)
		testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{ix}, admin, config.DefaultCommitment)

		var final fee_quoter.DestChain
		err = common.GetAccountDataBorshIntoAnchor(ctx, client, config.FqEvmDestChainPDA, config.DefaultCommitment, &final)
		require.NoError(t, err, "failed to get account info")
		require.False(t, final.Config.IsEnabled)
	})

	// ----- Update Dest Chain Config ----- //
	t.Run("When an unauthorized user tries to update the dest chain config, it fails", func(t *testing.T) {
		instruction, err := fee_quoter.NewUpdateDestChainConfigInstruction(
			config.EvmChainSelector,
			env.ValidFqDestChainConfig,
			config.FqConfigPDA,
			config.FqEvmDestChainPDA,
			user.PublicKey(), // unauthorized
		)
		require.NoError(t, err)
		result := testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{instruction}, user, config.DefaultCommitment, []string{"Error Code: " + ccip.Unauthorized_CcipRouterError.String()})
		require.NotNil(t, result)
	})

	t.Run("When an admin updates the dest chain config, it is configured", func(t *testing.T) {
		var initialDest fee_quoter.DestChain
		derr := common.GetAccountDataBorshIntoAnchor(ctx, client, config.FqEvmDestChainPDA, config.DefaultCommitment, &initialDest)
		require.NoError(t, derr, "failed to get account info")

		updated := initialDest.Config
		updated.IsEnabled = true
		require.NotEqual(t, initialDest.Config, updated) // at this point, onchain is disabled and we'll re-enable it

		instruction, err := fee_quoter.NewUpdateDestChainConfigInstruction(
			config.EvmChainSelector,
			updated,
			config.FqConfigPDA,
			config.FqEvmDestChainPDA,
			admin.PublicKey(),
		)
		require.NoError(t, err)
		result := testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{instruction}, admin, config.DefaultCommitment)
		require.NotNil(t, result)

		var final fee_quoter.DestChain
		err = common.GetAccountDataBorshIntoAnchor(ctx, client, config.FqEvmDestChainPDA, config.DefaultCommitment, &final)
		require.NoError(t, err, "failed to get account info")
		require.Equal(t, updated, final.Config)
	})

	// ----- Dest Chain V2 Config ----- //
	t.Run("V2 dest chain config", func(t *testing.T) {
		validV2Config := fee_quoter.DestChainConfigV2{
			IsEnabled:                   true,
			LaneCodeVersion:             fee_quoter.CodeVersion_Default,
			MaxDataBytes:                30000,
			MaxPerMsgGasLimit:           fee_quoter.CrossChainGas{V0: 3000000},
			DestGasOverhead:             fee_quoter.CrossChainGas{V0: 100000},
			DestGasPerPayloadByteBase:   16,
			ChainFamilySelector:         [4]uint8(config.EvmChainFamilySelector),
			DefaultTokenFeeUsdCents:     50,
			DefaultTokenDestGasOverhead: fee_quoter.CrossChainGas{V0: 50000},
			DefaultTxGasLimit:           fee_quoter.CrossChainGas{V0: 200000},
			NetworkFee:                  fee_quoter.UsdCents{V0: 50},
			LinkFeeMultiplierPercent:    90,
		}

		t.Run("init: creates PDA and emits DestChainAddedV2", func(t *testing.T) {
			ix, err := fee_quoter.NewUpsertDestChainConfigV2Instruction(
				fee_quoter.UpsertDestChainConfigV2Params{
					ChainSelector:   config.EvmChainSelector,
					DestChainConfig: validV2Config,
				},
				config.FqConfigPDA,
				config.FqEvmDestChainV2PDA,
				admin.PublicKey(),
				solana.SystemProgramID,
				config.FqEventAuthorityPDA,
				config.FeeQuoterProgram,
			)
			require.NoError(t, err)

			result := testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{ix}, admin, config.DefaultCommitment)
			require.NotNil(t, result)

			// Verify event
			tx, err := result.Transaction.GetTransaction()
			require.NoError(t, err)
			addedEvent, err := common.ExtractCPIEvent(tx, result.Meta, config.FeeQuoterProgram, fee_quoter.ParseEvent_DestChainAddedV2)
			require.NoError(t, err)
			require.Equal(t, config.EvmChainSelector, addedEvent.DestChainSelector)
			require.Equal(t, validV2Config, addedEvent.DestChainConfig)

			// Verify PDA content
			var account fee_quoter.DestChainV2
			err = common.GetAccountDataBorshIntoAnchor(ctx, client, config.FqEvmDestChainV2PDA, config.DefaultCommitment, &account)
			require.NoError(t, err)
			require.Equal(t, uint8(1), account.Version)
			require.Equal(t, config.EvmChainSelector, account.ChainSelector)
			require.Equal(t, fee_quoter.TimestampedPackedU224{}, account.State.UsdPerUnitGas)
			require.Equal(t, validV2Config, account.Config)
		})

		t.Run("update: replaces config and emits DestChainConfigUpdatedV2", func(t *testing.T) {
			updatedConfig := validV2Config
			updatedConfig.MaxDataBytes = 50000
			updatedConfig.NetworkFee = fee_quoter.UsdCents{V0: 100}

			ix, err := fee_quoter.NewUpsertDestChainConfigV2Instruction(
				fee_quoter.UpsertDestChainConfigV2Params{
					ChainSelector:   config.EvmChainSelector,
					DestChainConfig: updatedConfig,
				},
				config.FqConfigPDA,
				config.FqEvmDestChainV2PDA,
				admin.PublicKey(),
				solana.SystemProgramID,
				config.FqEventAuthorityPDA,
				config.FeeQuoterProgram,
			)
			require.NoError(t, err)

			result := testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{ix}, admin, config.DefaultCommitment)
			require.NotNil(t, result)

			// Verify event
			tx, err := result.Transaction.GetTransaction()
			require.NoError(t, err)
			updatedEvent, err := common.ExtractCPIEvent(tx, result.Meta, config.FeeQuoterProgram, fee_quoter.ParseEvent_DestChainConfigUpdatedV2)
			require.NoError(t, err)
			require.Equal(t, config.EvmChainSelector, updatedEvent.DestChainSelector)
			require.Equal(t, updatedConfig, updatedEvent.DestChainConfig)

			// Verify PDA reflects the update
			var account fee_quoter.DestChainV2
			err = common.GetAccountDataBorshIntoAnchor(ctx, client, config.FqEvmDestChainV2PDA, config.DefaultCommitment, &account)
			require.NoError(t, err)
			require.Equal(t, updatedConfig, account.Config)
		})

		t.Run("disable: flips is_enabled and ignores other fields", func(t *testing.T) {
			// Read current state
			var before fee_quoter.DestChainV2
			err := common.GetAccountDataBorshIntoAnchor(ctx, client, config.FqEvmDestChainV2PDA, config.DefaultCommitment, &before)
			require.NoError(t, err)
			require.True(t, before.Config.IsEnabled)

			ix, err := fee_quoter.NewDisableDestChainV2Instruction(
				config.EvmChainSelector,
				config.FqConfigPDA,
				config.FqEvmDestChainV2PDA,
				admin.PublicKey(),
				config.FqEventAuthorityPDA,
				config.FeeQuoterProgram,
			)
			require.NoError(t, err)

			result := testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{ix}, admin, config.DefaultCommitment)
			require.NotNil(t, result)

			// Verify event
			tx, err := result.Transaction.GetTransaction()
			require.NoError(t, err)
			disabledEvent, err := common.ExtractCPIEvent(tx, result.Meta, config.FeeQuoterProgram, fee_quoter.ParseEvent_DestChainConfigDisabledV2)
			require.NoError(t, err)
			require.Equal(t, config.EvmChainSelector, disabledEvent.DestChainSelector)
			require.False(t, disabledEvent.DestChainConfig.IsEnabled)

			// Verify PDA: only is_enabled changed, everything else preserved
			var after fee_quoter.DestChainV2
			err = common.GetAccountDataBorshIntoAnchor(ctx, client, config.FqEvmDestChainV2PDA, config.DefaultCommitment, &after)
			require.NoError(t, err)
			require.False(t, after.Config.IsEnabled)

			// All other fields unchanged
			require.Equal(t, before.Config.MaxDataBytes, after.Config.MaxDataBytes)
			require.Equal(t, before.Config.MaxPerMsgGasLimit, after.Config.MaxPerMsgGasLimit)
			require.Equal(t, before.Config.DestGasOverhead, after.Config.DestGasOverhead)
			require.Equal(t, before.Config.ChainFamilySelector, after.Config.ChainFamilySelector)
			require.Equal(t, before.Config.DefaultTxGasLimit, after.Config.DefaultTxGasLimit)
			require.Equal(t, before.Config.DefaultTokenFeeUsdCents, after.Config.DefaultTokenFeeUsdCents)
			require.Equal(t, before.Config.DefaultTokenDestGasOverhead, after.Config.DefaultTokenDestGasOverhead)
			require.Equal(t, before.Config.NetworkFee, after.Config.NetworkFee)
			require.Equal(t, before.Config.LinkFeeMultiplierPercent, after.Config.LinkFeeMultiplierPercent)
			require.Equal(t, before.Config.LaneCodeVersion, after.Config.LaneCodeVersion)
			require.Equal(t, before.State, after.State)
		})

		t.Run("upsert_unauthorized", func(t *testing.T) {
			ix, err := fee_quoter.NewUpsertDestChainConfigV2Instruction(
				fee_quoter.UpsertDestChainConfigV2Params{
					ChainSelector:   config.EvmChainSelector,
					DestChainConfig: validV2Config,
				},
				config.FqConfigPDA,
				config.FqEvmDestChainV2PDA,
				user.PublicKey(), // unauthorized
				solana.SystemProgramID,
				config.FqEventAuthorityPDA,
				config.FeeQuoterProgram,
			)
			require.NoError(t, err)
			testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{ix}, user, config.DefaultCommitment, []string{"Error Code: Unauthorized"})
		})

		t.Run("upsert rejects zero LINK fee multiplier", func(t *testing.T) {
			zeroMultiplierConfig := validV2Config
			zeroMultiplierConfig.LinkFeeMultiplierPercent = 0

			ix, err := fee_quoter.NewUpsertDestChainConfigV2Instruction(
				fee_quoter.UpsertDestChainConfigV2Params{
					ChainSelector:   config.EvmChainSelector,
					DestChainConfig: zeroMultiplierConfig,
				},
				config.FqConfigPDA,
				config.FqEvmDestChainV2PDA,
				admin.PublicKey(),
				solana.SystemProgramID,
				config.FqEventAuthorityPDA,
				config.FeeQuoterProgram,
			)
			require.NoError(t, err)
			testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{ix}, admin, config.DefaultCommitment, []string{"Error Code: ZeroLinkFeeMultiplier"})
		})

		t.Run("upsert rejects dest gas per payload byte base above 255", func(t *testing.T) {
			oversizedConfig := validV2Config
			oversizedConfig.DestGasPerPayloadByteBase = 256

			ix, err := fee_quoter.NewUpsertDestChainConfigV2Instruction(
				fee_quoter.UpsertDestChainConfigV2Params{
					ChainSelector:   config.EvmChainSelector,
					DestChainConfig: oversizedConfig,
				},
				config.FqConfigPDA,
				config.FqEvmDestChainV2PDA,
				admin.PublicKey(),
				solana.SystemProgramID,
				config.FqEventAuthorityPDA,
				config.FeeQuoterProgram,
			)
			require.NoError(t, err)
			testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{ix}, admin, config.DefaultCommitment, []string{"Error Code: DestGasPerPayloadByteBaseTooLarge"})
		})

		t.Run("upsert rejects network fee above 65535 USD cents", func(t *testing.T) {
			oversizedConfig := validV2Config
			oversizedConfig.NetworkFee = fee_quoter.UsdCents{V0: math.MaxUint16 + 1}

			ix, err := fee_quoter.NewUpsertDestChainConfigV2Instruction(
				fee_quoter.UpsertDestChainConfigV2Params{
					ChainSelector:   config.EvmChainSelector,
					DestChainConfig: oversizedConfig,
				},
				config.FqConfigPDA,
				config.FqEvmDestChainV2PDA,
				admin.PublicKey(),
				solana.SystemProgramID,
				config.FqEventAuthorityPDA,
				config.FeeQuoterProgram,
			)
			require.NoError(t, err)
			testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{ix}, admin, config.DefaultCommitment, []string{"Error Code: NetworkFeeTooLarge"})
		})

		t.Run("disable_unauthorized", func(t *testing.T) {
			ix, err := fee_quoter.NewDisableDestChainV2Instruction(
				config.EvmChainSelector,
				config.FqConfigPDA,
				config.FqEvmDestChainV2PDA,
				user.PublicKey(), // unauthorized
				config.FqEventAuthorityPDA,
				config.FeeQuoterProgram,
			)
			require.NoError(t, err)
			testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{ix}, user, config.DefaultCommitment, []string{"Error Code: Unauthorized"})
		})
	})

	// ----- Ownership Transfer ----- //
	t.Run("Fee Quoter: Can transfer ownership", func(t *testing.T) {
		// Fail to transfer ownership when not owner
		instruction, err := fee_quoter.NewTransferOwnershipInstruction(
			ccipAdmin.PublicKey(),
			config.FqConfigPDA,
			user.PublicKey(),
		)
		require.NoError(t, err)
		result := testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{instruction}, user, config.DefaultCommitment, []string{"Error Code: " + ccip.Unauthorized_CcipRouterError.String()})
		require.NotNil(t, result)

		// successfully transfer ownership
		instruction, err = fee_quoter.NewTransferOwnershipInstruction(
			ccipAdmin.PublicKey(),
			config.FqConfigPDA,
			admin.PublicKey(),
		)
		require.NoError(t, err)
		result = testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{instruction}, admin, config.DefaultCommitment)
		require.NotNil(t, result)

		transferEvent, err := common.ParseEventFromLogs(result.Meta.LogMessages, fee_quoter.ParseEvent_OwnershipTransferRequested)
		require.NoError(t, err)
		require.Equal(t, admin.PublicKey(), transferEvent.From)
		require.Equal(t, ccipAdmin.PublicKey(), transferEvent.To)

		// Fail to accept ownership when not proposed_owner
		instruction, err = fee_quoter.NewAcceptOwnershipInstruction(
			config.FqConfigPDA,
			user.PublicKey(),
		)
		require.NoError(t, err)
		result = testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{instruction}, user, config.DefaultCommitment, []string{"Error Code: " + ccip.Unauthorized_CcipRouterError.String()})
		require.NotNil(t, result)

		// Successfully accept ownership - ccipAdmin becomes owner
		instruction, err = fee_quoter.NewAcceptOwnershipInstruction(
			config.FqConfigPDA,
			ccipAdmin.PublicKey(),
		)
		require.NoError(t, err)
		result = testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{instruction}, ccipAdmin, config.DefaultCommitment)
		require.NotNil(t, result)
		acceptEvent, err := common.ParseEventFromLogs(result.Meta.LogMessages, fee_quoter.ParseEvent_OwnershipTransferred)
		require.NoError(t, err)
		require.Equal(t, admin.PublicKey(), acceptEvent.From)
		require.Equal(t, ccipAdmin.PublicKey(), acceptEvent.To)

		// Current owner cannot propose self
		instruction, err = fee_quoter.NewTransferOwnershipInstruction(
			ccipAdmin.PublicKey(),
			config.FqConfigPDA,
			ccipAdmin.PublicKey(),
		)
		require.NoError(t, err)
		result = testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{instruction}, ccipAdmin, config.DefaultCommitment, []string{"Error Code: " + ccip.RedundantOwnerProposal_FeeQuoterError.String()})
		require.NotNil(t, result)

		// Validate proposed set to 0-address
		var configAccount fee_quoter.Config
		err = common.GetAccountDataBorshIntoAnchor(ctx, client, config.FqConfigPDA, config.DefaultCommitment, &configAccount)
		require.NoError(t, err, "failed to get account info")
		require.Equal(t, solana.PublicKey{}, configAccount.ProposedOwner)
	})

	// ----- Billing Token Config ----- //
	// NOTE: these tests run after ownership transfer, so ccipAdmin is now the FQ owner
	t.Run("Billing: add billing tokens", func(t *testing.T) {
		validTimestamp := int64(100)
		bigValue := [28]uint8{}
		testutils.Link22UsdPrice.FillBytes(bigValue[:])

		smallValue := [28]uint8{}
		testutils.WsolUsdPrice.FillBytes(smallValue[:])

		type testToken struct {
			billingConfig fee_quoter.BillingTokenConfig
			program       solana.PublicKey
			mint          solana.PublicKey
			billingATA    solana.PublicKey
			fqBillingPDA  solana.PublicKey
		}

		testTokens := []testToken{
			{
				billingConfig: fee_quoter.BillingTokenConfig{
					Enabled: true,
					Mint:    solana.WrappedSol,
					UsdPerToken: fee_quoter.TimestampedPackedU224{
						Value:     smallValue,
						Timestamp: validTimestamp,
					},
					// Scaled against WsolUsdPrice, which is dimensioned for 9-decimal mints: the 1.6
					// premium is (network fee * multiplier / 1e18) USD, so a multiplier this many
					// orders of magnitude below 1e18 would round the whole message fee down to zero.
					PremiumMultiplierWeiPerEth: 900_000_000_000_000_000, // 90%
				},
				program:      env.WSOL.Program,
				mint:         env.WSOL.Mint,
				billingATA:   env.WSOL.BillingATA,
				fqBillingPDA: env.WSOL.FqBillingCfgPDA,
			},
			{
				billingConfig: fee_quoter.BillingTokenConfig{
					Enabled: true,
					Mint:    env.Link22.Mint,
					UsdPerToken: fee_quoter.TimestampedPackedU224{
						Value:     bigValue,
						Timestamp: validTimestamp,
					},
					PremiumMultiplierWeiPerEth: 1_100_000_000_000_000_000, // 110%
				},
				program:      env.Link22.Program,
				mint:         env.Link22.Mint,
				billingATA:   env.Link22.BillingATA,
				fqBillingPDA: env.Link22.FqBillingCfgPDA,
			},
		}

		for _, token := range testTokens {
			ixConfig, cerr := fee_quoter.NewAddBillingTokenConfigInstruction(
				token.billingConfig,
				config.FqConfigPDA,
				token.fqBillingPDA,
				token.program,
				token.mint,
				token.billingATA,
				ccipAdmin.PublicKey(),
				config.BillingSignerPDA,
				tokens.AssociatedTokenProgramID,
				solana.SystemProgramID,
			)
			require.NoError(t, cerr)
			testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{ixConfig}, ccipAdmin, config.DefaultCommitment)
		}
	})

	t.Run("FeeQuoter: Billing Token Config", func(t *testing.T) {
		pools := []tokens.TokenPool{env.Token0, env.Token1}

		for i, token := range pools {
			t.Run("token"+string(rune('0'+i)), func(t *testing.T) {
				tokenBillingPDA := testutils.FqTokenConfigPDA(token.Mint)

				t.Run("Pre-condition: Does not support token by default", func(t *testing.T) {
					var tokenConfigAccount fee_quoter.BillingTokenConfigWrapper
					err := common.GetAccountDataBorshIntoAnchor(ctx, client, tokenBillingPDA, config.DefaultCommitment, &tokenConfigAccount)
					require.EqualError(t, err, "not found")
				})

				t.Run("When admin adds token with valid input it is configured", func(t *testing.T) {
					validTimestamp := int64(100)
					value := [28]uint8{}
					big.NewInt(3e18).FillBytes(value[:])

					tokenConfig := fee_quoter.BillingTokenConfig{
						Enabled: true,
						Mint:    token.Mint,
						UsdPerToken: fee_quoter.TimestampedPackedU224{
							Timestamp: validTimestamp,
							Value:     value,
						},
						PremiumMultiplierWeiPerEth: 1,
					}

					tokenReceiver, _, ferr := tokens.FindAssociatedTokenAddress(token.Program, token.Mint, config.BillingSignerPDA)
					require.NoError(t, ferr)

					ixConfig, cerr := fee_quoter.NewAddBillingTokenConfigInstruction(
						tokenConfig,
						config.FqConfigPDA,
						tokenBillingPDA,
						token.Program,
						token.Mint,
						tokenReceiver,
						ccipAdmin.PublicKey(),
						config.BillingSignerPDA,
						tokens.AssociatedTokenProgramID,
						solana.SystemProgramID,
					)
					require.NoError(t, cerr)

					testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{ixConfig}, ccipAdmin, config.DefaultCommitment)

					var tokenConfigAccount fee_quoter.BillingTokenConfigWrapper
					aerr := common.GetAccountDataBorshIntoAnchor(ctx, client, tokenBillingPDA, config.DefaultCommitment, &tokenConfigAccount)
					require.NoError(t, aerr)
					require.Equal(t, tokenConfig, tokenConfigAccount.Config)
				})

				t.Run("When an unauthorized user updates token with correct configuration it fails", func(t *testing.T) {
					var initial fee_quoter.BillingTokenConfigWrapper
					ierr := common.GetAccountDataBorshIntoAnchor(ctx, client, tokenBillingPDA, config.DefaultCommitment, &initial)
					require.NoError(t, ierr)

					tokenConfig := initial.Config
					tokenConfig.PremiumMultiplierWeiPerEth = initial.Config.PremiumMultiplierWeiPerEth*2 + 1

					ixConfig, cerr := fee_quoter.NewUpdateBillingTokenConfigInstruction(tokenConfig, config.FqConfigPDA, tokenBillingPDA, admin.PublicKey()) // wrong admin (no longer owner)
					require.NoError(t, cerr)
					testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{ixConfig}, admin, config.DefaultCommitment, []string{ccip.Unauthorized_CcipRouterError.String()})

					var final fee_quoter.BillingTokenConfigWrapper
					ferr := common.GetAccountDataBorshIntoAnchor(ctx, client, tokenBillingPDA, config.DefaultCommitment, &final)
					require.NoError(t, ferr)
					require.Equal(t, initial.Config, final.Config) // not updated
				})

				t.Run("When admin updates token it is updated", func(t *testing.T) {
					var initial fee_quoter.BillingTokenConfigWrapper
					ierr := common.GetAccountDataBorshIntoAnchor(ctx, client, tokenBillingPDA, config.DefaultCommitment, &initial)
					require.NoError(t, ierr)

					tokenConfig := initial.Config
					tokenConfig.PremiumMultiplierWeiPerEth = initial.Config.PremiumMultiplierWeiPerEth*2 + 1

					ixConfig, cerr := fee_quoter.NewUpdateBillingTokenConfigInstruction(tokenConfig, config.FqConfigPDA, tokenBillingPDA, ccipAdmin.PublicKey())
					require.NoError(t, cerr)
					result := testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{ixConfig}, ccipAdmin, config.DefaultCommitment)

					premiumMultiplierWeiPerEthUpdatedEvent, err := common.ParseEventFromLogs(result.Meta.LogMessages, fee_quoter.ParseEvent_PremiumMultiplierWeiPerEthUpdated)
					require.NoError(t, err)
					require.Equal(t, tokenConfig.Mint, premiumMultiplierWeiPerEthUpdatedEvent.Token)
					require.Equal(t, tokenConfig.PremiumMultiplierWeiPerEth, premiumMultiplierWeiPerEthUpdatedEvent.PremiumMultiplierWeiPerEth)

					var final fee_quoter.BillingTokenConfigWrapper
					ferr := common.GetAccountDataBorshIntoAnchor(ctx, client, tokenBillingPDA, rpc.CommitmentProcessed, &final)
					require.NoError(t, ferr)
					require.NotEqual(t, initial.Config.PremiumMultiplierWeiPerEth, final.Config.PremiumMultiplierWeiPerEth)
					require.Equal(t, tokenConfig.PremiumMultiplierWeiPerEth, final.Config.PremiumMultiplierWeiPerEth)
				})
			})
		}
	})

	t.Run("FeeQuoter: Token transfer fee config", func(t *testing.T) {
		defaultTokenTransferFeeConfig := fee_quoter.TokenTransferFeeConfig{DestBytesOverhead: 32, MinFee: fee_quoter.UsdCents{V0: 0}, MaxFee: fee_quoter.UsdCents{V0: 1}}
		testcases := []struct {
			selector   uint64
			token      tokens.TokenPool
			billingPDA solana.PublicKey
		}{
			{selector: config.EvmChainSelector, token: env.Token0, billingPDA: env.Token0.Billing[config.EvmChainSelector]},
			{selector: config.EvmChainSelector, token: env.Token1, billingPDA: env.Token1.Billing[config.EvmChainSelector]},
			{selector: config.SvmChainSelector, token: env.Token0, billingPDA: env.Token0.Billing[config.SvmChainSelector]},
			{selector: config.SvmChainSelector, token: env.Token1, billingPDA: env.Token1.Billing[config.SvmChainSelector]},
		}

		ixs := make([]solana.Instruction, 0, len(testcases))
		for _, tc := range testcases {
			ix, err := fee_quoter.NewSetTokenTransferFeeConfigInstruction(
				tc.selector,
				tc.token.Mint,
				defaultTokenTransferFeeConfig,
				config.FqConfigPDA,
				tc.billingPDA,
				ccipAdmin.PublicKey(),
				solana.SystemProgramID,
			)
			require.NoError(t, err)
			ixs = append(ixs, ix)
		}

		result := testutils.SendAndConfirm(ctx, t, client, ixs, ccipAdmin, config.DefaultCommitment)
		require.NotNil(t, result)

		parsedEvents, err := common.ParseMultipleEventsFromLogs(result.Meta.LogMessages, fee_quoter.ParseEvent_TokenTransferFeeConfigUpdated)
		require.NoError(t, err)
		require.Len(t, parsedEvents, len(testcases))

		for i, tc := range testcases {
			require.Equal(t, tc.selector, parsedEvents[i].DestChainSelector)
			require.Equal(t, tc.token.Mint, parsedEvents[i].Token)
			require.Equal(t, defaultTokenTransferFeeConfig, parsedEvents[i].TokenTransferFeeConfig)
		}

		ix, err := fee_quoter.NewSetTokenTransferFeeConfigInstruction(
			config.EvmChainSelector,
			env.Token0.Mint,
			fee_quoter.TokenTransferFeeConfig{DestBytesOverhead: 32},
			config.FqConfigPDA,
			env.Token0.Billing[config.EvmChainSelector],
			user.PublicKey(),
			solana.SystemProgramID,
		)
		require.NoError(t, err)
		testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{ix}, user, config.DefaultCommitment, []string{ccip.Unauthorized_CcipRouterError.String()})
	})

	// ----- Initialize Router (needed for getFee router CPI and Withdraw tests) ----- //
	env.InitRouter(config.SvmChainSelector, testutils.FeeAggregator.PublicKey(), env.Link22.Mint)

	{
		// Add EVM chain selector to router
		ixRouterEvm, err := ccip_router.NewAddChainSelectorInstruction(config.EvmChainSelector, ccip_router.DestChainConfig{}, config.EvmDestChainStatePDA, config.RouterConfigPDA, admin.PublicKey(), solana.SystemProgramID)
		require.NoError(t, err)
		testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{ixRouterEvm}, admin, config.DefaultCommitment)

		// Transfer router ownership to ccipAdmin
		ixTransferRouter, err := ccip_router.NewTransferOwnershipInstruction(ccipAdmin.PublicKey(), config.RouterConfigPDA, admin.PublicKey())
		require.NoError(t, err)
		testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{ixTransferRouter}, admin, config.DefaultCommitment)
		ixAcceptRouter, err := ccip_router.NewAcceptOwnershipInstruction(config.RouterConfigPDA, ccipAdmin.PublicKey())
		require.NoError(t, err)
		testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{ixAcceptRouter}, ccipAdmin, config.DefaultCommitment)
	}

	// ----- getFee ----- //
	t.Run("getFee", func(t *testing.T) {
		// A lane cannot be quoted until its gas price is first written. AnotherUser seeds it so
		// that the price updater tests below still start with Admin unregistered.
		t.Run("Setup: seed the lane gas price", func(t *testing.T) {
			lanePriceUpdater := testutils.AnotherUser
			lanePriceUpdaterPDA, _, err := state.FindFqAllowedPriceUpdaterPDA(lanePriceUpdater.PublicKey(), config.FeeQuoterProgram)
			require.NoError(t, err)
			ix, err := fee_quoter.NewAddPriceUpdaterInstruction(
				lanePriceUpdater.PublicKey(),
				lanePriceUpdaterPDA,
				config.FqConfigPDA,
				ccipAdmin.PublicKey(),
				solana.SystemProgramID,
			)
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{ix}, ccipAdmin, config.DefaultCommitment)

			env.SetLaneGasPrice(ctx, lanePriceUpdater, config.EvmChainSelector, testutils.LaneUsdPerUnitGas)
		})

		t.Run("Fee is retrieved for a correctly formatted message", func(t *testing.T) {
			message := fee_quoter.Svm2AnyMessage{
				Receiver:  env.ValidEvmAbiReceiverAddress[:],
				FeeToken:  env.WSOL.Mint,
				ExtraArgs: []byte{},
			}

			raw, err := fee_quoter.NewGetFeeInstruction(config.EvmChainSelector, message, config.FqConfigPDA, config.FqEvmDestChainPDA, env.WSOL.FqBillingCfgPDA, env.Link22.FqBillingCfgPDA)
			require.NoError(t, err)

			feeResult := testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{raw}, user, config.DefaultCommitment)
			require.NotNil(t, feeResult)
			fee, _ := common.ExtractTypedReturnValue(ctx, feeResult.Meta.LogMessages, config.FeeQuoterProgram.String(), binary.LittleEndian.Uint64)
			require.Positive(t, fee)
		})

		t.Run("Fee is retrieved for a correctly formatted message containing a nonnative token", func(t *testing.T) {
			message := fee_quoter.Svm2AnyMessage{
				Receiver:     env.ValidEvmAbiReceiverAddress[:],
				FeeToken:     env.WSOL.Mint,
				TokenAmounts: []fee_quoter.SvmTokenAmount{{Token: env.Token0.Mint, Amount: 1}},
				ExtraArgs:    []byte{},
			}

			// Set some fees that will result in some appreciable change in the message fee
			billing := fee_quoter.TokenTransferFeeConfig{
				MinFee:            fee_quoter.UsdCents{V0: 800},
				MaxFee:            fee_quoter.UsdCents{V0: 1600},
				DeciBps:           0,
				DestGasOverhead:   fee_quoter.CrossChainGas{V0: 100},
				DestBytesOverhead: 100,
				IsEnabled:         true,
			}
			token0BillingConfigPda := testutils.FqTokenConfigPDA(env.Token0.Mint)
			token0PerChainPerConfigPda := testutils.FqPerChainPerTokenConfigPDA(env.Token0.Mint)
			ix, err := fee_quoter.NewSetTokenTransferFeeConfigInstruction(config.EvmChainSelector, env.Token0.Mint, billing, config.FqConfigPDA, token0PerChainPerConfigPda, ccipAdmin.PublicKey(), solana.SystemProgramID)
			require.NoError(t, err)
			result := testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{ix}, ccipAdmin, config.DefaultCommitment)

			// check TokenTransferFeeConfigUpdated event
			tokenTransferFeeConfigUpdatedEvent, err := common.ParseEventFromLogs(result.Meta.LogMessages, fee_quoter.ParseEvent_TokenTransferFeeConfigUpdated)
			require.NoError(t, err)

			require.Equal(t, config.EvmChainSelector, tokenTransferFeeConfigUpdatedEvent.DestChainSelector)
			require.Equal(t, env.Token0.Mint, tokenTransferFeeConfigUpdatedEvent.Token)
			require.Equal(t, billing.MaxFee, tokenTransferFeeConfigUpdatedEvent.TokenTransferFeeConfig.MaxFee)

			raw, err := fee_quoter.NewGetFeeInstruction(config.EvmChainSelector, message, config.FqConfigPDA, config.FqEvmDestChainPDA, env.WSOL.FqBillingCfgPDA, env.Link22.FqBillingCfgPDA)
			genericRaw, _ := raw.(*solana.GenericInstruction)
			genericRaw.AccountValues = append(genericRaw.AccountValues, solana.Meta(token0BillingConfigPda))
			genericRaw.AccountValues = append(genericRaw.AccountValues, solana.Meta(token0PerChainPerConfigPda))
			require.NoError(t, err)

			feeResult := testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{raw}, user, config.DefaultCommitment)
			require.NotNil(t, feeResult)
			fee, _ := common.ExtractTypedReturnValue(ctx, feeResult.Meta.LogMessages, config.FeeQuoterProgram.String(), binary.LittleEndian.Uint64)
			require.Positive(t, fee)
		})

		t.Run("Fee is retrieved for a correctly formatted message containing a nonnative token with no billing config", func(t *testing.T) {
			message := fee_quoter.Svm2AnyMessage{
				Receiver:     env.ValidEvmAbiReceiverAddress[:],
				FeeToken:     env.WSOL.Mint,
				TokenAmounts: []fee_quoter.SvmTokenAmount{{Token: env.Token2.Mint, Amount: 1}},
				ExtraArgs:    []byte{},
			}

			// Token 2 is not configured with any fee overrides
			token2BillingConfigPda := testutils.FqTokenConfigPDA(env.Token2.Mint)
			token2PerChainPerConfigPda := testutils.FqPerChainPerTokenConfigPDA(env.Token2.Mint)

			raw, err := fee_quoter.NewGetFeeInstruction(config.EvmChainSelector, message, config.FqConfigPDA, config.FqEvmDestChainPDA, env.WSOL.FqBillingCfgPDA, env.Link22.FqBillingCfgPDA)
			genericRaw, _ := raw.(*solana.GenericInstruction)
			genericRaw.AccountValues = append(genericRaw.AccountValues, solana.Meta(token2BillingConfigPda))
			genericRaw.AccountValues = append(genericRaw.AccountValues, solana.Meta(token2PerChainPerConfigPda))
			require.NoError(t, err)

			feeResult := testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{raw}, user, config.DefaultCommitment)
			require.NotNil(t, feeResult)
			fee, _ := common.ExtractTypedReturnValue(ctx, feeResult.Meta.LogMessages, config.FeeQuoterProgram.String(), binary.LittleEndian.Uint64)
			require.Positive(t, fee)
		})

		t.Run("Cannot get fee for message with invalid address", func(t *testing.T) {
			// Bigger than u160
			tooBigAddress := [32]byte{}
			tooBigAddress[11] = 1

			// Falls within precompile region
			tooSmallAddress := [32]byte{}
			tooSmallAddress[31] = 1

			for _, address := range [][32]byte{tooBigAddress, tooSmallAddress} {
				message := fee_quoter.Svm2AnyMessage{
					Receiver:  address[:],
					FeeToken:  env.WSOL.Mint,
					ExtraArgs: []byte{},
				}

				raw, err := fee_quoter.NewGetFeeInstruction(config.EvmChainSelector, message, config.FqConfigPDA, config.FqEvmDestChainPDA, env.WSOL.FqBillingCfgPDA, env.Link22.FqBillingCfgPDA)
				require.NoError(t, err)

				result := testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{raw}, user, config.DefaultCommitment, []string{"Error Code: InvalidEVMAddress"})
				require.NotNil(t, result)
			}
		})

		t.Run("Can retrieve fee through router CPI", func(t *testing.T) {
			message := ccip_router.Svm2AnyMessage{
				Receiver:  env.ValidEvmAbiReceiverAddress[:],
				FeeToken:  env.WSOL.Mint,
				ExtraArgs: []byte{},
			}

			raw, err := ccip_router.NewGetFeeInstruction(config.EvmChainSelector,
				message,
				config.RouterConfigPDA,
				config.EvmDestChainStatePDA,
				config.FeeQuoterProgram,
				config.FqConfigPDA,
				config.FqEvmDestChainPDA,
				env.WSOL.FqBillingCfgPDA,
				env.Link22.FqBillingCfgPDA,
			)
			require.NoError(t, err)

			feeResult := testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{raw}, user, config.DefaultCommitment)
			require.NotNil(t, feeResult)

			// Check first that FQ returned something valid to the router
			fqFee, err := common.ExtractAnchorTypedReturnValue[fee_quoter.GetFeeResult](ctx, feeResult.Meta.LogMessages, config.FeeQuoterProgram.String())
			require.NoError(t, err)
			require.Positive(t, fqFee.Amount)
			require.Equal(t, env.WSOL.Mint, fqFee.Token)

			// Check that the router's response matches what FQ returned (with fewer fields)
			routerFee, err := common.ExtractAnchorTypedReturnValue[ccip_router.GetFeeResult](ctx, feeResult.Meta.LogMessages, config.CcipRouterProgram.String())
			require.NoError(t, err)
			require.Equal(t, fqFee.Token, routerFee.Token)
			require.Equal(t, fqFee.Amount, routerFee.Amount)
			require.Equal(t, fqFee.Juels, routerFee.Juels)
		})
	})

	// ----- V2 fee views ----- //
	// The end-to-end `get_fee_v2` router flow (CCV receipts, pool fees, fallbacks, network fee)
	// is covered in onramp_test.go, where a fully configured CCIP v2.0 lane exists. Here we cover
	// the fee quoter's V2 view instructions that the router CPIs into for that flow.
	t.Run("getValidatedTokenPriceV2", func(t *testing.T) {
		t.Run("returns the configured token price", func(t *testing.T) {
			ix, err := fee_quoter.NewGetValidatedTokenPriceV2Instruction(env.WSOL.Mint, config.FqConfigPDA, env.WSOL.FqBillingCfgPDA)
			require.NoError(t, err)

			result := testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{ix}, user, config.DefaultCommitment)
			require.NotNil(t, result)

			price, err := common.ExtractAnchorTypedReturnValue[fee_quoter.Usd18Decimals](ctx, result.Meta.LogMessages, config.FeeQuoterProgram.String())
			require.NoError(t, err)
			// The wrapped words are the raw U256 little-endian representation: [low, high].
			priceValue := new(big.Int).Add(new(big.Int).Lsh(price.V0[1].BigInt(), 128), price.V0[0].BigInt())
			require.Equal(t, testutils.WsolUsdPrice, priceValue)
		})

		t.Run("rejects a billing config account for a different mint", func(t *testing.T) {
			// The billing config PDA is derived from the mint, so pairing the LINK mint with the
			// WSOL billing config account must fail the seeds constraint.
			ix, err := fee_quoter.NewGetValidatedTokenPriceV2Instruction(env.Link22.Mint, config.FqConfigPDA, env.WSOL.FqBillingCfgPDA)
			require.NoError(t, err)
			testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{ix}, user, config.DefaultCommitment, []string{"Error Code: ConstraintSeeds"})
		})
	})

	t.Run("getFallbackTokenTransferFeeV2", func(t *testing.T) {
		// The 2.0 views read the migrated dest chain account, configured by the "V2 dest chain
		// config" subtests above; the chain defaults are read back from it rather than restated.
		var destChainV2 fee_quoter.DestChainV2
		require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, client, config.FqEvmDestChainV2PDA, config.DefaultCommitment, &destChainV2))

		t.Run("uses the token's enabled transfer fee config", func(t *testing.T) {
			// Token0's per-chain transfer fee config was enabled in the getFee tests above
			// (MinFeeUsdcents=800, DestGasOverhead=100, DestBytesOverhead=100). The legacy
			// fallback repurposes min_fee as the flat fee.
			ix, err := fee_quoter.NewGetFallbackTokenTransferFeeV2Instruction(
				fee_quoter.GetFallbackTokenTransferFeeV2Params{DestChainSelector: config.EvmChainSelector, Mint: env.Token0.Mint},
				config.FqConfigPDA,
				config.FqEvmDestChainV2PDA,
				testutils.FqPerChainPerTokenConfigPDA(env.Token0.Mint),
			)
			require.NoError(t, err)

			result := testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{ix}, user, config.DefaultCommitment)
			require.NotNil(t, result)

			fee, err := common.ExtractAnchorTypedReturnValue[fee_quoter.FallbackTokenTransferFeeV2](ctx, result.Meta.LogMessages, config.FeeQuoterProgram.String())
			require.NoError(t, err)
			require.Equal(t, fee_quoter.UsdCents{V0: 800}, fee.FlatFee)
			require.Equal(t, fee_quoter.CrossChainGas{V0: 100}, fee.DestGasLimit)
			require.Equal(t, uint32(100), fee.DestBytesOverhead)
		})

		t.Run("falls back to the chain defaults for a token without a config", func(t *testing.T) {
			// Token2 has no per-chain transfer fee config, so the dest chain defaults apply, with
			// the lock-or-burn return size as the byte overhead.
			ix, err := fee_quoter.NewGetFallbackTokenTransferFeeV2Instruction(
				fee_quoter.GetFallbackTokenTransferFeeV2Params{DestChainSelector: config.EvmChainSelector, Mint: env.Token2.Mint},
				config.FqConfigPDA,
				config.FqEvmDestChainV2PDA,
				testutils.FqPerChainPerTokenConfigPDA(env.Token2.Mint),
			)
			require.NoError(t, err)

			result := testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{ix}, user, config.DefaultCommitment)
			require.NotNil(t, result)

			fee, err := common.ExtractAnchorTypedReturnValue[fee_quoter.FallbackTokenTransferFeeV2](ctx, result.Meta.LogMessages, config.FeeQuoterProgram.String())
			require.NoError(t, err)
			require.Equal(t, fee_quoter.UsdCents{V0: uint32(destChainV2.Config.DefaultTokenFeeUsdCents)}, fee.FlatFee)
			require.Equal(t, destChainV2.Config.DefaultTokenDestGasOverhead, fee.DestGasLimit)
			require.Equal(t, uint32(32), fee.DestBytesOverhead) // CCIP_LOCK_OR_BURN_V1_RET_BYTES
		})
	})

	// ----- Withdraw Billed Funds ----- //

	// Create fee aggregator ATAs and user WSOL ATA (needed by Withdraw tests)
	ixFeeAggrWsol, _, err := tokens.CreateAssociatedTokenAccount(env.WSOL.Program, env.WSOL.Mint, testutils.FeeAggregator.PublicKey(), testutils.FeeAggregator.PublicKey())
	require.NoError(t, err)
	ixFeeAggrLink, _, err := tokens.CreateAssociatedTokenAccount(env.Link22.Program, env.Link22.Mint, testutils.FeeAggregator.PublicKey(), testutils.FeeAggregator.PublicKey())
	require.NoError(t, err)
	testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{ixFeeAggrWsol, ixFeeAggrLink}, testutils.FeeAggregator, config.DefaultCommitment)

	// Create user WSOL ATA (used as wrong source/destination in negative tests)
	ixUserWsol, _, err := tokens.CreateAssociatedTokenAccount(env.WSOL.Program, env.WSOL.Mint, user.PublicKey(), user.PublicKey())
	require.NoError(t, err)
	testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{ixUserWsol}, user, config.DefaultCommitment)

	t.Run("Withdraw billed funds", func(t *testing.T) {
		// Seed billing ATAs with funds for withdraw testing.
		// Transfer WSOL to billing ATA
		wsolAmount := solana.LAMPORTS_PER_SOL / 10
		ixTransfer, err := tokens.NativeTransfer(wsolAmount, admin.PublicKey(), env.WSOL.BillingATA)
		require.NoError(t, err)
		ixSync, err := tokens.SyncNative(env.WSOL.Program, env.WSOL.BillingATA)
		require.NoError(t, err)
		testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{ixTransfer, ixSync}, admin, config.DefaultCommitment)

		// Mint LINK22 to billing ATA
		ixMintLink, err := tokens.MintTo(1e9, env.Link22.Program, env.Link22.Mint, env.Link22.BillingATA, admin.PublicKey())
		require.NoError(t, err)
		testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{ixMintLink}, admin, config.DefaultCommitment)

		t.Run("Preconditions", func(t *testing.T) {
			require.Positive(t, env.GetBalance(env.WSOL.BillingATA))
			require.Positive(t, env.GetBalance(env.Link22.BillingATA))
		})

		t.Run("When an non-admin user tries to withdraw funds from a billing token account, it fails", func(t *testing.T) {
			ix, err := ccip_router.NewWithdrawBilledFundsInstruction(
				true,      // withdraw all
				uint64(0), // amount
				env.WSOL.Mint,
				env.WSOL.BillingATA,
				env.WSOL.FeeAggregatorATA,
				env.WSOL.Program,
				config.BillingSignerPDA,
				config.RouterConfigPDA,
				user.PublicKey(), // wrong user here
			)
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{ix}, user, config.DefaultCommitment, []string{ccip.Unauthorized_CcipRouterError.String()})
		})

		t.Run("When withdrawing funds but sending them to the token account for a wrong token, it fails", func(t *testing.T) {
			ix, err := ccip_router.NewWithdrawBilledFundsInstruction(
				true,      // withdraw all
				uint64(0), // amount
				env.WSOL.Mint,
				env.WSOL.BillingATA,
				env.Link22.FeeAggregatorATA, // wrong token account
				env.WSOL.Program,
				config.BillingSignerPDA,
				config.RouterConfigPDA,
				ccipAdmin.PublicKey(),
			)
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{ix}, ccipAdmin, config.DefaultCommitment, []string{ccip.InvalidInputsAtaAddress_CcipRouterError.String()})
		})

		t.Run("When withdrawing funds from a token account that does not belong to billing, it fails", func(t *testing.T) {
			ix, err := ccip_router.NewWithdrawBilledFundsInstruction(
				true,      // withdraw all
				uint64(0), // amount
				env.WSOL.Mint,
				env.WSOL.UserATA, // attempt to withdraw from user account instead of billingATA
				env.WSOL.FeeAggregatorATA,
				env.WSOL.Program,
				config.BillingSignerPDA,
				config.RouterConfigPDA,
				ccipAdmin.PublicKey(),
			)
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{ix}, ccipAdmin, config.DefaultCommitment, []string{"Error Code: " + common.ConstraintTokenOwner_AnchorError.String()})
		})

		t.Run("When withdrawing funds but sending them to a non-whitelisted token account, it fails", func(t *testing.T) {
			ix, err := ccip_router.NewWithdrawBilledFundsInstruction(
				true,      // withdraw all
				uint64(0), // amount
				env.WSOL.Mint,
				env.WSOL.BillingATA,
				env.WSOL.UserATA, // wrong destination, sending to user account instead of fee aggregator's
				env.WSOL.Program,
				config.BillingSignerPDA,
				config.RouterConfigPDA,
				ccipAdmin.PublicKey(),
			)
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{ix}, ccipAdmin, config.DefaultCommitment, []string{ccip.InvalidInputsAtaAddress_CcipRouterError.String()})
		})

		t.Run("When trying to withdraw more funds than what's available, it fails", func(t *testing.T) {
			ix, err := ccip_router.NewWithdrawBilledFundsInstruction(
				false,                                 // withdraw all
				env.GetBalance(env.WSOL.BillingATA)+1, // amount (more than what's available)
				env.WSOL.Mint,
				env.WSOL.BillingATA,
				env.WSOL.FeeAggregatorATA,
				env.WSOL.Program,
				config.BillingSignerPDA,
				config.RouterConfigPDA,
				ccipAdmin.PublicKey(),
			)
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{ix}, ccipAdmin, config.DefaultCommitment, []string{ccip.InsufficientFunds_CcipRouterError.String()})
		})

		t.Run("When withdrawing a specific amount of funds, it succeeds", func(t *testing.T) {
			funds := env.GetBalance(env.Link22.BillingATA)
			require.Positive(t, funds)

			initialAggrBalance := env.GetBalance(env.Link22.FeeAggregatorATA)

			amount := uint64(2)

			ix, err := ccip_router.NewWithdrawBilledFundsInstruction(
				false,  // withdraw all
				amount, // amount
				env.Link22.Mint,
				env.Link22.BillingATA,
				env.Link22.FeeAggregatorATA,
				env.Link22.Program,
				config.BillingSignerPDA,
				config.RouterConfigPDA,
				ccipAdmin.PublicKey(),
			)
			require.NoError(t, err)

			testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{ix}, ccipAdmin, config.DefaultCommitment)

			require.Equal(t, funds-amount, env.GetBalance(env.Link22.BillingATA))                    // reduced
			require.Equal(t, amount, env.GetBalance(env.Link22.FeeAggregatorATA)-initialAggrBalance) // increased by exact amount
		})

		t.Run("When withdrawing all funds, it succeeds", func(t *testing.T) {
			funds := env.GetBalance(env.WSOL.BillingATA)
			require.Positive(t, funds)

			initialAggrBalance := env.GetBalance(env.WSOL.FeeAggregatorATA)

			ix, err := ccip_router.NewWithdrawBilledFundsInstruction(
				true,      // withdraw all
				uint64(0), // amount
				env.WSOL.Mint,
				env.WSOL.BillingATA,
				env.WSOL.FeeAggregatorATA,
				env.WSOL.Program,
				config.BillingSignerPDA,
				config.RouterConfigPDA,
				ccipAdmin.PublicKey(),
			)
			require.NoError(t, err)

			testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{ix}, ccipAdmin, config.DefaultCommitment)

			require.Equal(t, uint64(0), env.GetBalance(env.WSOL.BillingATA))                      // empty
			require.Equal(t, funds, env.GetBalance(env.WSOL.FeeAggregatorATA)-initialAggrBalance) // increased by exact amount
		})

		t.Run("When withdrawing all funds but the accumulator account is already empty (no balance), it fails", func(t *testing.T) {
			funds := env.GetBalance(env.WSOL.BillingATA)
			require.Equal(t, uint64(0), funds)

			ix, err := ccip_router.NewWithdrawBilledFundsInstruction(
				true,      // withdraw all
				uint64(0), // amount
				env.WSOL.Mint,
				env.WSOL.BillingATA,
				env.WSOL.FeeAggregatorATA,
				env.WSOL.Program,
				config.BillingSignerPDA,
				config.RouterConfigPDA,
				ccipAdmin.PublicKey(),
			)
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{ix}, ccipAdmin, config.DefaultCommitment, []string{ccip.InsufficientFunds_CcipRouterError.String()})
		})
	})

	// ----- Price Updates ----- //
	t.Run("FeeQuoter price updates", func(t *testing.T) {
		testPriceUpdater := admin // reusing admin since it has funds
		testAllowedPriceUpdaterPDA, _, err := state.FindFqAllowedPriceUpdaterPDA(testPriceUpdater.PublicKey(), config.FeeQuoterProgram)
		require.NoError(t, err)

		t.Run("When a non-admin tries to add a price updater, it fails", func(t *testing.T) {
			ix, err := fee_quoter.NewAddPriceUpdaterInstruction(
				testPriceUpdater.PublicKey(),
				testAllowedPriceUpdaterPDA,
				config.FqConfigPDA,
				user.PublicKey(), // not admin
				solana.SystemProgramID,
			)
			require.NoError(t, err)
			testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{ix}, user, config.DefaultCommitment, []string{ccip.Unauthorized_FeeQuoterError.String()})
		})

		t.Run("When an admin adds a price updater, it succeeds", func(t *testing.T) {
			ix, err := fee_quoter.NewAddPriceUpdaterInstruction(
				testPriceUpdater.PublicKey(),
				testAllowedPriceUpdaterPDA,
				config.FqConfigPDA,
				ccipAdmin.PublicKey(),
				solana.SystemProgramID,
			)
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{ix}, ccipAdmin, config.DefaultCommitment)
		})

		t.Run("When the caller is not allowed to update the price, it fails", func(t *testing.T) {
			userAllowedPriceUpdaterPDA, _, err := state.FindFqAllowedPriceUpdaterPDA(user.PublicKey(), config.FeeQuoterProgram)
			require.NoError(t, err)

			ix, err := fee_quoter.NewUpdatePricesInstruction(
				[]fee_quoter.TokenPriceUpdate{},
				[]fee_quoter.GasPriceUpdate{},
				user.PublicKey(),
				userAllowedPriceUpdaterPDA,
				config.FqConfigPDA,
			)
			require.NoError(t, err)
			testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{ix}, user, config.DefaultCommitment, []string{ccip.UnauthorizedPriceUpdater_FeeQuoterError.String()})
		})

		t.Run("When the caller is not allowed to update the price but tries to trick fee quoter to check an allowed caller, it fails", func(t *testing.T) {
			ix, err := fee_quoter.NewUpdatePricesInstruction(
				[]fee_quoter.TokenPriceUpdate{},
				[]fee_quoter.GasPriceUpdate{},
				user.PublicKey(),
				testAllowedPriceUpdaterPDA,
				config.FqConfigPDA,
			)
			require.NoError(t, err)
			testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{ix}, user, config.DefaultCommitment, []string{"AnchorError caused by account: allowed_price_updater. Error Code: " + common.ConstraintSeeds_AnchorError.String()})
		})

		t.Run("When the caller is allowed to update the price, it succeeds", func(t *testing.T) {
			tokenPrice := common.To28BytesBE(123)
			gasPrice := common.To28BytesBE(321)

			var tokenPriceAccount fee_quoter.BillingTokenConfigWrapper
			require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, client, env.WSOL.FqBillingCfgPDA, config.DefaultCommitment, &tokenPriceAccount))
			require.NotEqual(t, tokenPrice, tokenPriceAccount.Config.UsdPerToken.Value)

			var gasPriceAccount fee_quoter.DestChain
			require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, client, config.FqEvmDestChainPDA, config.DefaultCommitment, &gasPriceAccount))
			require.NotEqual(t, gasPrice, gasPriceAccount.State.UsdPerUnitGas.Value)

			raw, err := fee_quoter.NewUpdatePricesInstruction(
				[]fee_quoter.TokenPriceUpdate{
					{
						SourceToken: env.WSOL.Mint,
						UsdPerToken: tokenPrice,
					},
				},
				[]fee_quoter.GasPriceUpdate{
					{
						DestChainSelector: config.EvmChainSelector,
						UsdPerUnitGas:     gasPrice,
					},
				},
				testPriceUpdater.PublicKey(),
				testAllowedPriceUpdaterPDA,
				config.FqConfigPDA,
			)
			genericRaw, _ := raw.(*solana.GenericInstruction)
			genericRaw.AccountValues = append(genericRaw.AccountValues, solana.Meta(env.WSOL.FqBillingCfgPDA).WRITE())
			genericRaw.AccountValues = append(genericRaw.AccountValues, solana.Meta(config.FqEvmDestChainPDA).WRITE())

			require.NoError(t, err)

			testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{raw}, testPriceUpdater, config.DefaultCommitment)

			require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, client, env.WSOL.FqBillingCfgPDA, config.DefaultCommitment, &tokenPriceAccount))
			require.Equal(t, tokenPrice, tokenPriceAccount.Config.UsdPerToken.Value)

			require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, client, config.FqEvmDestChainPDA, config.DefaultCommitment, &gasPriceAccount))
			require.Equal(t, gasPrice, gasPriceAccount.State.UsdPerUnitGas.Value)
		})

		t.Run("When the caller updates prices containing unregistered tokens, the call succeeds and the unregistered tokens are ignored", func(t *testing.T) {
			tokenPrice := common.To28BytesBE(123)
			anotherTokenPrice := common.To28BytesBE(234)

			var tokenPriceAccount fee_quoter.BillingTokenConfigWrapper
			require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, client, env.WSOL.FqBillingCfgPDA, config.DefaultCommitment, &tokenPriceAccount))
			require.Equal(t, tokenPrice, tokenPriceAccount.Config.UsdPerToken.Value)

			// token2 isn't registered as a billing token
			require.True(t, common.IsClosedAccount(ctx, client, env.Token2.Billing[config.EvmChainSelector], config.DefaultCommitment))

			raw, err := fee_quoter.NewUpdatePricesInstruction(
				[]fee_quoter.TokenPriceUpdate{
					{
						SourceToken: env.WSOL.Mint,
						UsdPerToken: anotherTokenPrice,
					},
					{
						SourceToken: env.Token2.Mint,
						UsdPerToken: anotherTokenPrice,
					},
				},
				[]fee_quoter.GasPriceUpdate{},
				testPriceUpdater.PublicKey(),
				testAllowedPriceUpdaterPDA,
				config.FqConfigPDA,
			)
			genericRaw, _ := raw.(*solana.GenericInstruction)
			genericRaw.AccountValues = append(genericRaw.AccountValues, solana.Meta(env.WSOL.FqBillingCfgPDA).WRITE())
			genericRaw.AccountValues = append(genericRaw.AccountValues, solana.Meta(env.Token2.FeeTokenConfig).WRITE())

			require.NoError(t, err)

			result := testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{raw}, testPriceUpdater, config.DefaultCommitment)
			require.NotNil(t, result)

			updateIgnoredEvent, err := common.ParseEventFromLogs(result.Meta.LogMessages, fee_quoter.ParseEvent_TokenPriceUpdateIgnored)
			require.NoError(t, err)
			require.Equal(t, updateIgnoredEvent.Token, env.Token2.Mint)
			require.Equal(t, updateIgnoredEvent.Value, anotherTokenPrice)

			require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, client, env.WSOL.FqBillingCfgPDA, config.DefaultCommitment, &tokenPriceAccount))
			require.Equal(t, anotherTokenPrice, tokenPriceAccount.Config.UsdPerToken.Value)

			require.True(t, common.IsClosedAccount(ctx, client, env.Token2.FeeTokenConfig, config.DefaultCommitment))
		})

		t.Run("When a non-admin tries to remove a price updater, it fails", func(t *testing.T) {
			ix, err := fee_quoter.NewRemovePriceUpdaterInstruction(
				testPriceUpdater.PublicKey(),
				testAllowedPriceUpdaterPDA,
				config.FqConfigPDA,
				user.PublicKey(), // not admin
				solana.SystemProgramID,
			)
			require.NoError(t, err)
			testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{ix}, user, config.DefaultCommitment, []string{ccip.Unauthorized_FeeQuoterError.String()})
		})

		t.Run("When an admin removes a price updater, it succeeds and the caller can no longer update prices", func(t *testing.T) {
			ix, err := fee_quoter.NewRemovePriceUpdaterInstruction(
				testPriceUpdater.PublicKey(),
				testAllowedPriceUpdaterPDA,
				config.FqConfigPDA,
				ccipAdmin.PublicKey(),
				solana.SystemProgramID,
			)
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{ix}, ccipAdmin, config.DefaultCommitment)
		})
	})
}
