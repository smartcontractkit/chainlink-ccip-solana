// router_config_test.go tests the configuration and setup of the CCIP Router program.
// This includes initialization, chain selector settings, LINK token configuration,
// fee aggregator management, and destination chain setup.
package contracts

import (
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/ccip_router"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/config"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/testutils"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/ccip"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
)

func TestRouterConfig(t *testing.T) {
	t.Parallel()
	env := testutils.NewTestEnv(t).Build()
	ctx := env.Ctx()
	client := env.Client
	admin := testutils.Admin
	user := testutils.User

	const invalidSVMChainSelector uint64 = 17

	// Use token0 mint pubkey as initial link token (just needs to be different from link22)
	initialLinkMint := testutils.Token0MintPrivK.PublicKey()

	// ----- Initialize Router ----- //
	t.Run("Router is initialized", func(t *testing.T) {
		routerProgramDataAddress := testutils.GetProgramDataAddress(ctx, t, client, config.CcipRouterProgram)

		tempFeeAggregator := testutils.AnotherUser.PublicKey()
		instruction, err := ccip_router.NewInitializeInstruction(
			invalidSVMChainSelector,
			tempFeeAggregator,
			config.FeeQuoterProgram,
			initialLinkMint, // to be changed in the next tests
			config.RMNRemoteProgram,
			config.RouterConfigPDA,
			admin.PublicKey(),
			solana.SystemProgramID,
			config.CcipRouterProgram,
			routerProgramDataAddress,
		)
		require.NoError(t, err)

		result := testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{instruction}, admin, config.DefaultCommitment)
		require.NotNil(t, result)

		configSetEvent, err := common.ParseEventFromLogs(result.Meta.LogMessages, ccip_router.ParseEvent_ConfigSet)
		require.NoError(t, err)
		require.Equal(t, invalidSVMChainSelector, configSetEvent.SvmChainSelector)
		require.Equal(t, config.FeeQuoterProgram, configSetEvent.FeeQuoter)
		require.Equal(t, config.RMNRemoteProgram, configSetEvent.RmnRemote)
		require.Equal(t, initialLinkMint, configSetEvent.LinkTokenMint)
		require.Equal(t, tempFeeAggregator, configSetEvent.FeeAggregator)

		var configAccount ccip_router.Config
		err = common.GetAccountDataBorshIntoAnchor(ctx, client, config.RouterConfigPDA, config.DefaultCommitment, &configAccount)
		require.NoError(t, err, "failed to get account info")
		require.Equal(t, uint64(17), configAccount.SvmChainSelector)
		require.Equal(t, config.FeeQuoterProgram, configAccount.FeeQuoter)
	})

	// ----- Update Link Mint ----- //
	t.Run("Router: Update link mint", func(t *testing.T) {
		t.Run("When a non-admin tries to make the update, it fails", func(t *testing.T) {
			ix, err := ccip_router.NewSetLinkTokenMintInstruction(
				env.Link22.Mint,
				config.RouterConfigPDA,
				user.PublicKey(),
				solana.SystemProgramID,
			)
			require.NoError(t, err)
			testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{ix}, user, config.DefaultCommitment, []string{ccip.Unauthorized_CcipRouterError.String()})
		})

		t.Run("When an admin tries to make the update, it succeeds", func(t *testing.T) {
			var configAccount ccip_router.Config
			require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, client, config.RouterConfigPDA, config.DefaultCommitment, &configAccount))
			require.Equal(t, initialLinkMint, configAccount.LinkTokenMint) // initially set to token0 mint

			ix, err := ccip_router.NewSetLinkTokenMintInstruction(
				env.Link22.Mint,
				config.RouterConfigPDA,
				admin.PublicKey(),
				solana.SystemProgramID,
			)
			require.NoError(t, err)

			result := testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{ix}, admin, config.DefaultCommitment)
			require.NotNil(t, result)

			configSetEvent, err := common.ParseEventFromLogs(result.Meta.LogMessages, ccip_router.ParseEvent_ConfigSet)
			require.NoError(t, err)
			require.Equal(t, env.Link22.Mint, configSetEvent.LinkTokenMint)

			require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, client, config.RouterConfigPDA, config.DefaultCommitment, &configAccount))
			require.Equal(t, env.Link22.Mint, configAccount.LinkTokenMint)
		})
	})

	// ----- Update SVM Chain Selector ----- //
	t.Run("When admin updates the solana chain selector it's updated", func(t *testing.T) {
		instruction, err := ccip_router.NewUpdateSvmChainSelectorInstruction(
			config.SvmChainSelector,
			config.RouterConfigPDA,
			admin.PublicKey(),
			solana.SystemProgramID,
		)
		require.NoError(t, err)
		result := testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{instruction}, admin, config.DefaultCommitment)
		require.NotNil(t, result)

		configSetEvent, err := common.ParseEventFromLogs(result.Meta.LogMessages, ccip_router.ParseEvent_ConfigSet)
		require.NoError(t, err)
		require.Equal(t, config.SvmChainSelector, configSetEvent.SvmChainSelector)

		var configAccount ccip_router.Config
		err = common.GetAccountDataBorshIntoAnchor(ctx, client, config.RouterConfigPDA, config.DefaultCommitment, &configAccount)
		require.NoError(t, err, "failed to get account info")
		require.Equal(t, config.SvmChainSelector, configAccount.SvmChainSelector)
	})

	// ----- Add Chain Selectors ----- //
	t.Run("When an unauthorized user tries to add a chain selector, it fails", func(t *testing.T) {
		instruction, err := ccip_router.NewAddChainSelectorInstruction(
			config.EvmChainSelector,
			ccip_router.DestChainConfig{},
			config.EvmDestChainStatePDA,
			config.RouterConfigPDA,
			user.PublicKey(), // not an admin
			solana.SystemProgramID,
		)
		require.NoError(t, err)
		result := testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{instruction}, user, config.DefaultCommitment, []string{"Error Code: " + ccip.Unauthorized_CcipRouterError.String()})
		require.NotNil(t, result)
	})

	t.Run("When admin adds EVM chain selector it's added on the list", func(t *testing.T) {
		instruction, err := ccip_router.NewAddChainSelectorInstruction(
			config.EvmChainSelector,
			ccip_router.DestChainConfig{},
			config.EvmDestChainStatePDA,
			config.RouterConfigPDA,
			admin.PublicKey(),
			solana.SystemProgramID,
		)
		require.NoError(t, err)
		result := testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{instruction}, admin, config.DefaultCommitment)
		require.NotNil(t, result)

		var destChainStateAccount ccip_router.DestChain
		err = common.GetAccountDataBorshIntoAnchor(ctx, client, config.EvmDestChainStatePDA, config.DefaultCommitment, &destChainStateAccount)
		require.NoError(t, err, "failed to get account info")
		require.Equal(t, uint64(0), destChainStateAccount.State.MessageNumber)
		require.Equal(t, ccip_router.DestChainConfig{}, destChainStateAccount.Config)
	})

	t.Run("When admin adds SVM chain selector it's also added on the list", func(t *testing.T) {
		instruction, err := ccip_router.NewAddChainSelectorInstruction(
			config.SvmChainSelector,
			ccip_router.DestChainConfig{},
			config.SvmDestChainStatePDA,
			config.RouterConfigPDA,
			admin.PublicKey(),
			solana.SystemProgramID,
		)
		require.NoError(t, err)
		result := testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{instruction}, admin, config.DefaultCommitment)
		require.NotNil(t, result)

		var destChainStateAccount ccip_router.DestChain
		err = common.GetAccountDataBorshIntoAnchor(ctx, client, config.SvmDestChainStatePDA, config.DefaultCommitment, &destChainStateAccount)
		require.NoError(t, err, "failed to get account info")
		require.Equal(t, uint64(0), destChainStateAccount.State.MessageNumber)
	})

	// ----- Onramp Lane CCIP Version ----- //
	t.Run("Onramp lane ccip version", func(t *testing.T) {
		var initial ccip_router.DestChain
		require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, client, config.SvmDestChainStatePDA, config.DefaultCommitment, &initial))
		require.Equal(t, ccip_router.RestoreOnAction_None, initial.State.RestoreOnAction)

		t.Run("When a non-admin tries to bump the onramp ccip version for a dest chain, it fails", func(t *testing.T) {
			ix, err := ccip_router.NewBumpCcipVersionForDestChainInstruction(
				config.SvmChainSelector,
				config.SvmDestChainStatePDA,
				config.RouterConfigPDA,
				user.PublicKey(),
			)
			require.NoError(t, err)
			testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{ix}, user, config.DefaultCommitment, []string{"Error Code: " + ccip.Unauthorized_CcipRouterError.String()})
		})

		t.Run("When an admin bumps the onramp ccip version for a dest chain, it succeeds", func(t *testing.T) {
			ix, err := ccip_router.NewBumpCcipVersionForDestChainInstruction(
				config.SvmChainSelector,
				config.SvmDestChainStatePDA,
				config.RouterConfigPDA,
				admin.PublicKey(),
			)
			require.NoError(t, err)
			result := testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{ix}, admin, config.DefaultCommitment)
			require.NotNil(t, result)

			var routerDestChain ccip_router.DestChain
			require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, client, config.SvmDestChainStatePDA, config.DefaultCommitment, &routerDestChain))
			require.Equal(t, ccip_router.RestoreOnAction_Rollback, routerDestChain.State.RestoreOnAction)
		})

		t.Run("When a non-admin tries to rollback the onramp ccip version for a dest chain, it fails", func(t *testing.T) {
			ix, err := ccip_router.NewRollbackCcipVersionForDestChainInstruction(
				config.SvmChainSelector,
				config.SvmDestChainStatePDA,
				config.RouterConfigPDA,
				user.PublicKey(),
			)
			require.NoError(t, err)
			testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{ix}, user, config.DefaultCommitment, []string{"Error Code: " + ccip.Unauthorized_CcipRouterError.String()})
		})

		t.Run("When an admin rollbacks the onramp ccip version for a dest chain, it succeeds", func(t *testing.T) {
			ix, err := ccip_router.NewRollbackCcipVersionForDestChainInstruction(
				config.SvmChainSelector,
				config.SvmDestChainStatePDA,
				config.RouterConfigPDA,
				admin.PublicKey(),
			)
			require.NoError(t, err)
			result := testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{ix}, admin, config.DefaultCommitment)
			require.NotNil(t, result)

			var routerDestChain ccip_router.DestChain
			require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, client, config.SvmDestChainStatePDA, config.DefaultCommitment, &routerDestChain))
			require.Equal(t, ccip_router.RestoreOnAction_Upgrade, routerDestChain.State.RestoreOnAction)
		})

		t.Run("When an admin tries to rollback again the onramp ccip version for a dest chain, it fails", func(t *testing.T) {
			ix, err := ccip_router.NewRollbackCcipVersionForDestChainInstruction(
				config.SvmChainSelector,
				config.SvmDestChainStatePDA,
				config.RouterConfigPDA,
				admin.PublicKey(),
			)
			require.NoError(t, err)
			testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{ix}, admin, config.DefaultCommitment, []string{"Error Code: " + ccip.InvalidCcipVersionRollback_CcipRouterError.String()})
		})
	})

	// ----- Update Fee Aggregator ----- //
	t.Run("When an unauthorized user tries to update the fee aggregator, it fails", func(t *testing.T) {
		instruction, err := ccip_router.NewUpdateFeeAggregatorInstruction(
			user.PublicKey(),
			config.RouterConfigPDA,
			user.PublicKey(),
			solana.SystemProgramID,
		)
		require.NoError(t, err)
		result := testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{instruction}, user, config.DefaultCommitment, []string{"Error Code: " + ccip.Unauthorized_CcipRouterError.String()})
		require.NotNil(t, result)
	})

	t.Run("When an authorized user updates the fee aggregator, it succeeds", func(t *testing.T) {
		feeAggregator := testutils.FeeAggregator

		var configAccount ccip_router.Config
		err := common.GetAccountDataBorshIntoAnchor(ctx, client, config.RouterConfigPDA, config.DefaultCommitment, &configAccount)
		require.NoError(t, err, "failed to get account info")
		require.NotEqual(t, feeAggregator.PublicKey(), configAccount.FeeAggregator)

		instruction, err := ccip_router.NewUpdateFeeAggregatorInstruction(
			feeAggregator.PublicKey(),
			config.RouterConfigPDA,
			admin.PublicKey(),
			solana.SystemProgramID,
		)
		require.NoError(t, err)
		result := testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{instruction}, admin, config.DefaultCommitment)
		require.NotNil(t, result)

		configSetEvent, err := common.ParseEventFromLogs(result.Meta.LogMessages, ccip_router.ParseEvent_ConfigSet)
		require.NoError(t, err)
		require.Equal(t, feeAggregator.PublicKey(), configSetEvent.FeeAggregator)

		err = common.GetAccountDataBorshIntoAnchor(ctx, client, config.RouterConfigPDA, config.DefaultCommitment, &configAccount)
		require.NoError(t, err, "failed to get account info")
		require.Equal(t, feeAggregator.PublicKey(), configAccount.FeeAggregator)
	})

	// ----- Offramp Permissions ----- //
	t.Run("Offramp permissions", func(t *testing.T) {
		instruction, err := ccip_router.NewAddOfframpInstruction(
			config.EvmChainSelector, config.CcipOfframpProgram, config.AllowedOfframpEvmPDA, config.RouterConfigPDA, admin.PublicKey(), solana.SystemProgramID,
		)
		require.NoError(t, err)
		testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{instruction}, admin, config.DefaultCommitment)
	})

	// ----- Ownership Transfer ----- //
	t.Run("Can transfer ownership", func(t *testing.T) {
		ccipAdmin := testutils.CcipAdmin

		// Fail to transfer ownership when not owner
		instruction, err := ccip_router.NewTransferOwnershipInstruction(
			ccipAdmin.PublicKey(),
			config.RouterConfigPDA,
			user.PublicKey(),
		)
		require.NoError(t, err)
		result := testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{instruction}, user, config.DefaultCommitment, []string{"Error Code: " + ccip.Unauthorized_CcipRouterError.String()})
		require.NotNil(t, result)

		// successfully transfer ownership
		instruction, err = ccip_router.NewTransferOwnershipInstruction(
			ccipAdmin.PublicKey(),
			config.RouterConfigPDA,
			admin.PublicKey(),
		)
		require.NoError(t, err)
		result = testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{instruction}, admin, config.DefaultCommitment)
		require.NotNil(t, result)

		transferEvent, err := common.ParseEventFromLogs(result.Meta.LogMessages, ccip_router.ParseEvent_OwnershipTransferRequested)
		require.NoError(t, err)
		require.Equal(t, admin.PublicKey(), transferEvent.From)
		require.Equal(t, ccipAdmin.PublicKey(), transferEvent.To)

		// Fail to accept ownership when not proposed_owner
		instruction, err = ccip_router.NewAcceptOwnershipInstruction(
			config.RouterConfigPDA,
			user.PublicKey(),
		)
		require.NoError(t, err)
		result = testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{instruction}, user, config.DefaultCommitment, []string{"Error Code: " + ccip.Unauthorized_CcipRouterError.String()})
		require.NotNil(t, result)

		// Successfully accept ownership
		instruction, err = ccip_router.NewAcceptOwnershipInstruction(
			config.RouterConfigPDA,
			ccipAdmin.PublicKey(),
		)
		require.NoError(t, err)
		result = testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{instruction}, ccipAdmin, config.DefaultCommitment)
		require.NotNil(t, result)
		acceptEvent, err := common.ParseEventFromLogs(result.Meta.LogMessages, ccip_router.ParseEvent_OwnershipTransferred)
		require.NoError(t, err)
		require.Equal(t, admin.PublicKey(), acceptEvent.From)
		require.Equal(t, ccipAdmin.PublicKey(), acceptEvent.To)

		// Current owner cannot propose self
		instruction, err = ccip_router.NewTransferOwnershipInstruction(
			ccipAdmin.PublicKey(),
			config.RouterConfigPDA,
			ccipAdmin.PublicKey(),
		)
		require.NoError(t, err)
		result = testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{instruction}, ccipAdmin, config.DefaultCommitment, []string{"Error Code: " + ccip.RedundantOwnerProposal_CcipRouterError.String()})
		require.NotNil(t, result)

		// Validate proposed set to 0-address
		var configAccount ccip_router.Config
		err = common.GetAccountDataBorshIntoAnchor(ctx, client, config.RouterConfigPDA, config.DefaultCommitment, &configAccount)
		require.NoError(t, err, "failed to get account info")
		require.Equal(t, solana.PublicKey{}, configAccount.ProposedOwner)
	})
}
