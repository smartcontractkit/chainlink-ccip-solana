// offramp_config_test.go tests the configuration and setup of the Offramp program.
// This includes initialization, OCR3 config management, source chain configuration,
// and reference address setup for inbound CCIP message processing.
package contracts

import (
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/ccip_offramp"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/config"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/testutils"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/ccip"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
)

func TestOfframpConfig(t *testing.T) {
	t.Parallel()
	env := testutils.NewTestEnv(t).WithOCR().Build()
	ctx := env.Ctx()
	client := env.Client
	admin := testutils.Admin
	user := testutils.User
	ccipAdmin := testutils.CcipAdmin

	const invalidSVMChainSelector uint64 = 17

	// Create a minimal lookup table for offramp init
	lookupEntries := []solana.PublicKey{
		config.CcipOfframpProgram,
		config.OfframpConfigPDA,
		config.OfframpReferenceAddressesPDA,
		solana.SystemProgramID,
	}
	lookupTableAddr, err := common.SetupLookupTable(ctx, client, admin, lookupEntries)
	require.NoError(t, err)

	// ----- Initialize Offramp ----- //

	t.Run("Offramp is initialized", func(t *testing.T) {
		offrampProgramDataAddress := testutils.GetProgramDataAddress(ctx, t, client, config.CcipOfframpProgram)

		// Check that a user who isn't the upgrade authority cannot call this
		badInitIx, err := ccip_offramp.NewInitializeInstruction(
			config.OfframpReferenceAddressesPDA,
			config.CcipRouterProgram,
			config.FeeQuoterProgram,
			config.RMNRemoteProgram,
			lookupTableAddr,
			user.PublicKey(), // not the upgrade authority
			solana.SystemProgramID,
			config.CcipOfframpProgram,
			offrampProgramDataAddress,
		)
		require.NoError(t, err)

		testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{badInitIx}, user, config.DefaultCommitment, []string{ccip.Unauthorized_CcipRouterError.String()})

		badInitConfigIx, err := ccip_offramp.NewInitializeConfigInstruction(
			invalidSVMChainSelector,
			config.OfframpConfigPDA,
			ccipAdmin.PublicKey(), // not the upgrade authority
			solana.SystemProgramID,
			config.CcipOfframpProgram,
			offrampProgramDataAddress,
		)
		require.NoError(t, err)

		testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{badInitConfigIx}, ccipAdmin, config.DefaultCommitment, []string{ccip.Unauthorized_CcipRouterError.String()})

		testReferenceAddresses := ccip_offramp.ReferenceAddresses{
			Router:             [32]byte{1},
			FeeQuoter:          [32]byte{2},
			OfframpLookupTable: [32]byte{3},
			RmnRemote:          [32]byte{4},
		}

		// Now, actually initialize the offramp
		initIx, err := ccip_offramp.NewInitializeInstruction(
			config.OfframpReferenceAddressesPDA,
			testReferenceAddresses.Router,             // will be updated in later test
			testReferenceAddresses.FeeQuoter,          // will be updated in later test
			testReferenceAddresses.RmnRemote,          // will be updated in later test
			testReferenceAddresses.OfframpLookupTable, // will be updated in later test
			admin.PublicKey(),
			solana.SystemProgramID,
			config.CcipOfframpProgram,
			offrampProgramDataAddress,
		)
		require.NoError(t, err)

		initConfigIx, err := ccip_offramp.NewInitializeConfigInstruction(
			invalidSVMChainSelector,
			config.OfframpConfigPDA,
			admin.PublicKey(),
			solana.SystemProgramID,
			config.CcipOfframpProgram,
			offrampProgramDataAddress,
		)
		require.NoError(t, err)

		result := testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{initIx, initConfigIx}, admin, config.DefaultCommitment)
		require.NotNil(t, result)

		refAddrEvent, err := common.ParseEventFromLogs(result.Meta.LogMessages, ccip_offramp.ParseEvent_ReferenceAddressesSet)
		require.NoError(t, err)
		require.Equal(t, testReferenceAddresses.Router, refAddrEvent.Router)
		require.Equal(t, testReferenceAddresses.FeeQuoter, refAddrEvent.FeeQuoter)
		require.Equal(t, testReferenceAddresses.OfframpLookupTable, refAddrEvent.OfframpLookupTable)
		require.Equal(t, testReferenceAddresses.RmnRemote, refAddrEvent.RmnRemote)

		configSetEvent, err := common.ParseEventFromLogs(result.Meta.LogMessages, ccip_offramp.ParseEvent_ConfigSet)
		require.NoError(t, err)
		require.Equal(t, invalidSVMChainSelector, configSetEvent.SvmChainSelector)

		// Fetch account data
		var offrampConfig ccip_offramp.Config
		require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, client, config.OfframpConfigPDA, config.DefaultCommitment, &offrampConfig))
		require.Equal(t, invalidSVMChainSelector, offrampConfig.SvmChainSelector)
		require.Equal(t, admin.PublicKey(), offrampConfig.Owner)
		require.Equal(t, solana.PublicKey{}, offrampConfig.ProposedOwner)

		// check reference addresses
		var referenceAddresses ccip_offramp.ReferenceAddresses
		require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, client, config.OfframpReferenceAddressesPDA, config.DefaultCommitment, &referenceAddresses))
		require.Equal(t, testReferenceAddresses.FeeQuoter, referenceAddresses.FeeQuoter)
		require.Equal(t, testReferenceAddresses.OfframpLookupTable, referenceAddresses.OfframpLookupTable)
		require.Equal(t, testReferenceAddresses.Router, referenceAddresses.Router)
	})

	// ----- Update Reference Addresses ----- //

	t.Run("Offramp: Update reference addresses", func(t *testing.T) {
		t.Run("When an unauthorized user tries to make the update, it fails", func(t *testing.T) {
			ix, err := ccip_offramp.NewUpdateReferenceAddressesInstruction(
				config.CcipRouterProgram,
				config.FeeQuoterProgram,
				lookupTableAddr,
				config.RMNRemoteProgram,
				config.OfframpConfigPDA,
				config.OfframpReferenceAddressesPDA,
				user.PublicKey(), // unauthorized user here
				config.OfframpEventAuthorityPDA,
				config.CcipOfframpProgram,
			)
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{ix}, user, config.DefaultCommitment, []string{ccip.Unauthorized_CcipRouterError.String()})
		})

		t.Run("When the admin makes the update, it succeeds", func(t *testing.T) {
			ix, err := ccip_offramp.NewUpdateReferenceAddressesInstruction(
				config.CcipRouterProgram,
				config.FeeQuoterProgram,
				lookupTableAddr,
				config.RMNRemoteProgram,
				config.OfframpConfigPDA,
				config.OfframpReferenceAddressesPDA,
				admin.PublicKey(),
				config.OfframpEventAuthorityPDA,
				config.CcipOfframpProgram,
			)
			require.NoError(t, err)

			result := testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{ix}, admin, config.DefaultCommitment)
			require.NotNil(t, result)

			// check event
			txrefAddrEvent, txErr := result.Transaction.GetTransaction()
			require.NoError(t, txErr)
			refAddrEvent, err := common.ExtractCPIEvent(txrefAddrEvent, result.Meta, config.CcipOfframpProgram, ccip_offramp.ParseEvent_ReferenceAddressesSet)
			require.NoError(t, err)
			require.Equal(t, config.CcipRouterProgram, refAddrEvent.Router)
			require.Equal(t, config.FeeQuoterProgram, refAddrEvent.FeeQuoter)
			require.Equal(t, config.RMNRemoteProgram, refAddrEvent.RmnRemote)
			require.Equal(t, lookupTableAddr, refAddrEvent.OfframpLookupTable)

			// check state
			var referenceAddresses ccip_offramp.ReferenceAddresses
			require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, client, config.OfframpReferenceAddressesPDA, config.DefaultCommitment, &referenceAddresses))
			require.Equal(t, config.FeeQuoterProgram, referenceAddresses.FeeQuoter)
			require.Equal(t, lookupTableAddr, referenceAddresses.OfframpLookupTable)
			require.Equal(t, config.CcipRouterProgram, referenceAddresses.Router)
			require.Equal(t, config.RMNRemoteProgram, referenceAddresses.RmnRemote)
		})
	})

	// ----- Update SVM Chain Selector ----- //

	t.Run("When admin updates the solana chain selector it's updated", func(t *testing.T) {
		instruction, err := ccip_offramp.NewUpdateSvmChainSelectorInstruction(
			config.SvmChainSelector,
			config.OfframpConfigPDA,
			admin.PublicKey(),
			config.OfframpEventAuthorityPDA,
			config.CcipOfframpProgram,
		)
		require.NoError(t, err)
		result := testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{instruction}, admin, config.DefaultCommitment)
		require.NotNil(t, result)

		txconfigSetEvent, txErr := result.Transaction.GetTransaction()
		require.NoError(t, txErr)
		configSetEvent, err := common.ExtractCPIEvent(txconfigSetEvent, result.Meta, config.CcipOfframpProgram, ccip_offramp.ParseEvent_ConfigSet)
		require.NoError(t, err)
		require.Equal(t, config.SvmChainSelector, configSetEvent.SvmChainSelector)

		var configAccount ccip_offramp.Config
		err = common.GetAccountDataBorshIntoAnchor(ctx, client, config.OfframpConfigPDA, config.DefaultCommitment, &configAccount)
		require.NoError(t, err, "failed to get account info")
		require.Equal(t, config.SvmChainSelector, configAccount.SvmChainSelector)
	})

	// ----- Add Source Chains ----- //

	t.Run("When an unauthorized user adds a chain selector, it fails", func(t *testing.T) {
		instruction, err := ccip_offramp.NewAddSourceChainInstruction(
			ccip_offramp.AddSourceChainParams{
				NewChainSelector:  config.EvmChainSelector,
				SourceChainConfig: env.ValidSourceChainConfig,
			},
			config.OfframpEvmSourceChainPDA,
			config.OfframpConfigPDA,
			user.PublicKey(), // not an admin
			solana.SystemProgramID,
			config.OfframpEventAuthorityPDA,
			config.CcipOfframpProgram,
		)
		require.NoError(t, err)
		result := testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{instruction}, user, config.DefaultCommitment, []string{"Error Code: " + ccip.Unauthorized_CcipOfframpError.String()})
		require.NotNil(t, result)
	})

	t.Run("When admin adds EVM source chain it's added", func(t *testing.T) {
		instruction, err := ccip_offramp.NewAddSourceChainInstruction(
			ccip_offramp.AddSourceChainParams{
				NewChainSelector:  config.EvmChainSelector,
				SourceChainConfig: env.ValidSourceChainConfig,
			},
			config.OfframpEvmSourceChainPDA,
			config.OfframpConfigPDA,
			admin.PublicKey(),
			solana.SystemProgramID,
			config.OfframpEventAuthorityPDA,
			config.CcipOfframpProgram,
		)
		require.NoError(t, err)
		result := testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{instruction}, admin, config.DefaultCommitment)
		require.NotNil(t, result)

		var sourceChainStateAccount ccip_offramp.SourceChain
		err = common.GetAccountDataBorshIntoAnchor(ctx, client, config.OfframpEvmSourceChainPDA, config.DefaultCommitment, &sourceChainStateAccount)
		require.NoError(t, err, "failed to get account info")
		require.True(t, sourceChainStateAccount.Config.IsEnabled)

		onRampAddress := env.OnRampAddress
		require.Equal(t, []ccip_offramp.OnRampAddress{{Bytes: onRampAddress, Len: 32}}, sourceChainStateAccount.Config.OnRamps)
	})

	t.Run("When admin adds SVM source chain it's also added", func(t *testing.T) {
		paddedCcipRouterProgram := common.ToPadded64Bytes(config.CcipRouterProgram.Bytes())
		onRampConfig := ccip_offramp.OnRampAddress{
			Bytes: paddedCcipRouterProgram,
			Len:   32,
		}

		instruction, err := ccip_offramp.NewAddSourceChainInstruction(
			ccip_offramp.AddSourceChainParams{
				NewChainSelector: config.SvmChainSelector,
				SourceChainConfig: ccip_offramp.SourceChainConfig{
					OnRamps:             []ccip_offramp.OnRampAddress{onRampConfig},
					IsEnabled:           true,
					DefaultCcvs:         []solana.PublicKey{config.CommitteeVerifierProgram},
					LaneMandatedCcvs:    []solana.PublicKey{},
					LegacyAddressLength: 32, // svm
				},
			},
			config.OfframpSvmSourceChainPDA,
			config.OfframpConfigPDA,
			admin.PublicKey(),
			solana.SystemProgramID,
			config.OfframpEventAuthorityPDA,
			config.CcipOfframpProgram,
		)
		require.NoError(t, err)
		result := testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{instruction}, admin, config.DefaultCommitment)
		require.NotNil(t, result)

		var sourceChainStateAccount ccip_offramp.SourceChain
		err = common.GetAccountDataBorshIntoAnchor(ctx, client, config.OfframpSvmSourceChainPDA, config.DefaultCommitment, &sourceChainStateAccount)
		require.NoError(t, err, "failed to get account info")
		require.True(t, sourceChainStateAccount.Config.IsEnabled)
		require.Equal(t, []ccip_offramp.OnRampAddress{{Bytes: paddedCcipRouterProgram, Len: 32}}, sourceChainStateAccount.Config.OnRamps)
	})

	// ----- Disable/Enable Source Chain ----- //

	t.Run("When a non-admin tries to disable the source chain selector, it fails", func(t *testing.T) {
		ix, err := ccip_offramp.NewDisableSourceChainSelectorInstruction(
			config.EvmChainSelector,
			config.OfframpEvmSourceChainPDA,
			config.OfframpConfigPDA,
			user.PublicKey(),
			config.OfframpEventAuthorityPDA,
			config.CcipOfframpProgram,
		)
		require.NoError(t, err)
		result := testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{ix}, user, config.DefaultCommitment, []string{"Error Code: " + ccip.Unauthorized_CcipRouterError.String()})
		require.NotNil(t, result)
	})

	t.Run("When an admin disables the source chain selector, it is no longer enabled", func(t *testing.T) {
		var initial ccip_offramp.SourceChain
		err := common.GetAccountDataBorshIntoAnchor(ctx, client, config.OfframpEvmSourceChainPDA, config.DefaultCommitment, &initial)
		require.NoError(t, err, "failed to get account info")
		require.True(t, initial.Config.IsEnabled)

		ix, err := ccip_offramp.NewDisableSourceChainSelectorInstruction(
			config.EvmChainSelector,
			config.OfframpEvmSourceChainPDA,
			config.OfframpConfigPDA,
			admin.PublicKey(),
			config.OfframpEventAuthorityPDA,
			config.CcipOfframpProgram,
		)
		require.NoError(t, err)
		testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{ix}, admin, config.DefaultCommitment)

		var final ccip_offramp.SourceChain
		err = common.GetAccountDataBorshIntoAnchor(ctx, client, config.OfframpEvmSourceChainPDA, config.DefaultCommitment, &final)
		require.NoError(t, err, "failed to get account info")
		require.False(t, final.Config.IsEnabled)
	})

	// ----- Update Source Chain Config ----- //

	t.Run("When an unauthorized user tries to update the source chain config, it fails", func(t *testing.T) {
		instruction, err := ccip_offramp.NewUpdateSourceChainConfigInstruction(
			ccip_offramp.UpdateSourceChainParams{
				SourceChainSelector: config.EvmChainSelector,
				SourceChainConfig:   env.ValidSourceChainConfig,
			},
			config.OfframpEvmSourceChainPDA,
			config.OfframpConfigPDA,
			user.PublicKey(), // unauthorized
			solana.SystemProgramID,
			config.OfframpEventAuthorityPDA,
			config.CcipOfframpProgram,
		)
		require.NoError(t, err)
		result := testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{instruction}, user, config.DefaultCommitment, []string{"Error Code: " + ccip.Unauthorized_CcipRouterError.String()})
		require.NotNil(t, result)
	})

	t.Run("When an admin updates the source chain config, it is configured", func(t *testing.T) {
		var initialSource ccip_offramp.SourceChain
		serr := common.GetAccountDataBorshIntoAnchor(ctx, client, config.OfframpEvmSourceChainPDA, config.DefaultCommitment, &initialSource)
		require.NoError(t, serr, "failed to get account info")

		updated := initialSource.Config
		updated.IsEnabled = true
		require.NotEqual(t, initialSource.Config, updated) // at this point, onchain is disabled and we'll re-enable it

		instruction, err := ccip_offramp.NewUpdateSourceChainConfigInstruction(
			ccip_offramp.UpdateSourceChainParams{
				SourceChainSelector: config.EvmChainSelector,
				SourceChainConfig:   updated,
			},
			config.OfframpEvmSourceChainPDA,
			config.OfframpConfigPDA,
			admin.PublicKey(),
			solana.SystemProgramID,
			config.OfframpEventAuthorityPDA,
			config.CcipOfframpProgram,
		)
		require.NoError(t, err)
		result := testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{instruction}, admin, config.DefaultCommitment)
		require.NotNil(t, result)

		var final ccip_offramp.SourceChain
		err = common.GetAccountDataBorshIntoAnchor(ctx, client, config.OfframpEvmSourceChainPDA, config.DefaultCommitment, &final)
		require.NoError(t, err, "failed to get account info")
		require.Equal(t, updated, final.Config)
	})

	// ---- Delete Source Chain ---- //
	t.Run("Delete source chain", func(t *testing.T) {
		t.Run("when an unauthorized user tries to delete it, it fails", func(t *testing.T) {
			ix, err := ccip_offramp.NewDeleteSourceChainInstruction(
				config.EvmChainSelector,
				config.OfframpEvmSourceChainPDA,
				config.OfframpConfigPDA,
				user.PublicKey(), // non-admin authority
				config.OfframpEventAuthorityPDA,
				config.CcipOfframpProgram,
			)
			require.NoError(t, err)
			testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{ix}, user, config.DefaultCommitment, []string{"Error Code: " + ccip.Unauthorized_CcipRouterError.String()})
		})

		t.Run("when an admin tries to delete a non-existent source chain, it fails", func(t *testing.T) {
			chainSelector := uint64(999) // some non-existent chain selector
			pda, _, err := state.FindOfframpSourceChainPDA(chainSelector, config.CcipOfframpProgram)
			require.NoError(t, err)
			ix, err := ccip_offramp.NewDeleteSourceChainInstruction(
				chainSelector,
				pda,
				config.OfframpConfigPDA,
				admin.PublicKey(), // admin authority
				config.OfframpEventAuthorityPDA,
				config.CcipOfframpProgram,
			)
			require.NoError(t, err)
			testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{ix}, admin, config.DefaultCommitment, []string{common.ConstraintOwner_AnchorError.String()})
		})

		t.Run("when an admin tries to delete an inconsistent source chain account, it fails", func(t *testing.T) {
			chainSelector := uint64(999) // some non-existent chain selector
			ix, err := ccip_offramp.NewDeleteSourceChainInstruction(
				chainSelector,
				config.OfframpEvmSourceChainPDA, // but evm pda
				config.OfframpConfigPDA,
				admin.PublicKey(),
				config.OfframpEventAuthorityPDA,
				config.CcipOfframpProgram,
			)
			require.NoError(t, err)
			testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{ix}, admin, config.DefaultCommitment, []string{common.ConstraintSeeds_AnchorError.String()})
		})

		t.Run("when an admin deletes it, it succeeds", func(t *testing.T) {
			require.False(t, common.IsClosedAccount(ctx, client, config.OfframpEvmSourceChainPDA, config.DefaultCommitment))

			ix, err := ccip_offramp.NewDeleteSourceChainInstruction(
				config.EvmChainSelector,
				config.OfframpEvmSourceChainPDA,
				config.OfframpConfigPDA,
				admin.PublicKey(), // admin authority
				config.OfframpEventAuthorityPDA,
				config.CcipOfframpProgram,
			)
			require.NoError(t, err)
			result := testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{ix}, admin, config.DefaultCommitment)

			require.True(t, common.IsClosedAccount(ctx, client, config.OfframpEvmSourceChainPDA, config.DefaultCommitment))

			tx, err := result.Transaction.GetTransaction()
			require.NoError(t, err)
			event, err := common.ExtractCPIEvent(tx, result.Meta, config.CcipOfframpProgram, ccip_offramp.ParseEvent_SourceChainDeleted)
			require.NoError(t, err)
			require.Equal(t, config.EvmChainSelector, event.SourceChainSelector)
		})
	})

	// ---- Source Chain CCV Config Validation ---- //

	t.Run("Source chain CCV config validation", func(t *testing.T) {
		valid := env.ValidSourceChainConfig

		testCases := []struct {
			name          string
			mutate        func(*ccip_offramp.SourceChainConfig)
			expectedError string
		}{
			{
				name: "rejects empty defaults",
				mutate: func(cfg *ccip_offramp.SourceChainConfig) {
					cfg.DefaultCcvs = []solana.PublicKey{}
					cfg.LaneMandatedCcvs = []solana.PublicKey{}
				},
				expectedError: ccip.ZeroPubkeyNotAllowedInCCVConfig_CcipOfframpError.String(),
			},
			{
				name: "rejects zero default CCV",
				mutate: func(cfg *ccip_offramp.SourceChainConfig) {
					cfg.DefaultCcvs = []solana.PublicKey{{}}
				},
				expectedError: ccip.ZeroPubkeyNotAllowedInCCVConfig_CcipOfframpError.String(),
			},
			{
				name: "rejects duplicate default CCVs",
				mutate: func(cfg *ccip_offramp.SourceChainConfig) {
					cfg.DefaultCcvs = []solana.PublicKey{config.CommitteeVerifierProgram, config.CommitteeVerifierProgram}
				},
				expectedError: ccip.DuplicateCCVNotAllowed_CcipOfframpError.String(),
			},
			{
				name: "rejects duplicate lane mandated CCVs",
				mutate: func(cfg *ccip_offramp.SourceChainConfig) {
					laneMandatedCCV := solana.NewWallet().PublicKey()
					cfg.LaneMandatedCcvs = []solana.PublicKey{laneMandatedCCV, laneMandatedCCV}
				},
				expectedError: ccip.DuplicateCCVNotAllowed_CcipOfframpError.String(),
			},
			{
				name: "rejects duplicate across default and lane mandated",
				mutate: func(cfg *ccip_offramp.SourceChainConfig) {
					cfg.DefaultCcvs = []solana.PublicKey{config.CommitteeVerifierProgram}
					cfg.LaneMandatedCcvs = []solana.PublicKey{config.CommitteeVerifierProgram}
				},
				expectedError: ccip.DuplicateCCVNotAllowed_CcipOfframpError.String(),
			},
			{
				name: "rejects zero legacy address length",
				mutate: func(cfg *ccip_offramp.SourceChainConfig) {
					cfg.LegacyAddressLength = 0
				},
				expectedError: ccip.InvalidLegacyAddressLength_CcipOfframpError.String(),
			},
			{
				name: "rejects legacy address length above the OnRamp address capacity",
				mutate: func(cfg *ccip_offramp.SourceChainConfig) {
					cfg.LegacyAddressLength = 65
				},
				expectedError: ccip.InvalidLegacyAddressLength_CcipOfframpError.String(),
			},
		}

		for i, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				invalid := valid
				tc.mutate(&invalid)
				// #nosec G115 - val is validated as positive above
				chainSelector := uint64(900_000 + i)
				sourceChainPDA, _, err := state.FindOfframpSourceChainPDA(chainSelector, config.CcipOfframpProgram)
				require.NoError(t, err)

				instruction, err := ccip_offramp.NewAddSourceChainInstruction(
					ccip_offramp.AddSourceChainParams{
						NewChainSelector:  chainSelector,
						SourceChainConfig: invalid,
					},
					sourceChainPDA,
					config.OfframpConfigPDA,
					admin.PublicKey(),
					solana.SystemProgramID,
					config.OfframpEventAuthorityPDA,
					config.CcipOfframpProgram,
				)
				require.NoError(t, err)

				result := testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{instruction}, admin, config.DefaultCommitment, []string{"Error Code: " + tc.expectedError})
				require.NotNil(t, result)
			})
		}
	})

	// ----- Ownership Transfer ----- //

	t.Run("Offramp: Can transfer ownership", func(t *testing.T) {
		// Fail to transfer ownership when not owner
		instruction, err := ccip_offramp.NewTransferOwnershipInstruction(
			ccipAdmin.PublicKey(),
			config.OfframpConfigPDA,
			user.PublicKey(),
			config.OfframpEventAuthorityPDA,
			config.CcipOfframpProgram,
		)
		require.NoError(t, err)
		result := testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{instruction}, user, config.DefaultCommitment, []string{"Error Code: " + ccip.Unauthorized_CcipOfframpError.String()})
		require.NotNil(t, result)

		// successfully transfer ownership
		instruction, err = ccip_offramp.NewTransferOwnershipInstruction(
			ccipAdmin.PublicKey(),
			config.OfframpConfigPDA,
			admin.PublicKey(),
			config.OfframpEventAuthorityPDA,
			config.CcipOfframpProgram,
		)
		require.NoError(t, err)
		result = testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{instruction}, admin, config.DefaultCommitment)
		require.NotNil(t, result)

		txtransferEvent, txErr := result.Transaction.GetTransaction()
		require.NoError(t, txErr)
		transferEvent, err := common.ExtractCPIEvent(txtransferEvent, result.Meta, config.CcipOfframpProgram, ccip_offramp.ParseEvent_OwnershipTransferRequested)
		require.NoError(t, err)
		require.Equal(t, admin.PublicKey(), transferEvent.From)
		require.Equal(t, ccipAdmin.PublicKey(), transferEvent.To)

		// Fail to accept ownership when not proposed_owner
		instruction, err = ccip_offramp.NewAcceptOwnershipInstruction(
			config.OfframpConfigPDA,
			user.PublicKey(),
			config.OfframpEventAuthorityPDA,
			config.CcipOfframpProgram,
		)
		require.NoError(t, err)
		result = testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{instruction}, user, config.DefaultCommitment, []string{"Error Code: " + ccip.Unauthorized_CcipOfframpError.String()})
		require.NotNil(t, result)

		// Successfully accept ownership
		instruction, err = ccip_offramp.NewAcceptOwnershipInstruction(
			config.OfframpConfigPDA,
			ccipAdmin.PublicKey(),
			config.OfframpEventAuthorityPDA,
			config.CcipOfframpProgram,
		)
		require.NoError(t, err)
		result = testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{instruction}, ccipAdmin, config.DefaultCommitment)
		require.NotNil(t, result)
		txacceptEvent, txErr := result.Transaction.GetTransaction()
		require.NoError(t, txErr)
		acceptEvent, err := common.ExtractCPIEvent(txacceptEvent, result.Meta, config.CcipOfframpProgram, ccip_offramp.ParseEvent_OwnershipTransferred)
		require.NoError(t, err)
		require.Equal(t, admin.PublicKey(), acceptEvent.From)
		require.Equal(t, ccipAdmin.PublicKey(), acceptEvent.To)

		// Current owner cannot propose self
		instruction, err = ccip_offramp.NewTransferOwnershipInstruction(
			ccipAdmin.PublicKey(),
			config.OfframpConfigPDA,
			ccipAdmin.PublicKey(),
			config.OfframpEventAuthorityPDA,
			config.CcipOfframpProgram,
		)
		require.NoError(t, err)
		result = testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{instruction}, ccipAdmin, config.DefaultCommitment, []string{"Error Code: " + ccip.RedundantOwnerProposal_CcipOfframpError.String()})
		require.NotNil(t, result)

		// Validate proposed set to 0-address
		var configAccount ccip_offramp.Config
		err = common.GetAccountDataBorshIntoAnchor(ctx, client, config.OfframpConfigPDA, config.DefaultCommitment, &configAccount)
		if err != nil {
			require.NoError(t, err, "failed to get account info")
		}
		require.Equal(t, solana.PublicKey{}, configAccount.ProposedOwner)
	})
}
