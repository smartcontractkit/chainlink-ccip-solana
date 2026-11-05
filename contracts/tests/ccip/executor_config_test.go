// executor_config_test.go tests the configuration and setup of the Executor program.
// This includes initialization, ownership transfers, global config updates, per-chain config management,
// and allowed CCV (Chain Configuration Verifier) settings.
package contracts

import (
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/executor"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/config"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/testutils"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/ccip"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/tokens"
)

func TestExecutorConfig(t *testing.T) {
	ctx := t.Context()

	admin := solana.MustPrivateKeyFromBase58("5dYf8bzhFbmwTNyq8vAoeqe19vdjCbSEKkPrN6DQoJB2RwjyuVapCmU46pEBvbw7aeg5wBncTc8HQJnKZy2LYsmF")
	user := solana.MustPrivateKeyFromBase58("ZZdVf32Npuhci4u4ir2NW9491Y3FTv2Gwk41HMpvgJoh81UM42LcNqAN8SXapHfPcr61QP7sJj7K2mKHt7qFCoV")
	newAdmin := solana.MustPrivateKeyFromBase58("5Biqymxxk6joWPiWdEdToM8nNMx7jPr1Sv7qxdyVnXERa35jqjrTtG9kfDkMDxuD25Rob4WiLpauAgHehnPBWxkj")

	remoteChainSelector := uint64(12345)

	solanaGoClient := testutils.DeployAllPrograms(t, testutils.PathToAnchorConfig, admin)

	t.Run("setup", func(t *testing.T) {
		t.Run("funding", func(t *testing.T) {
			require.NoError(t, testutils.FundAccounts(ctx, []solana.PrivateKey{admin, user, newAdmin}, solanaGoClient))
		})

		t.Run("initialize", func(t *testing.T) {
			execProgramDataAddress := testutils.GetProgramDataAddress(ctx, t, solanaGoClient, config.ExecutorProgram)

			ix, err := executor.NewInitializeInstruction(
				executor.InitializeParams{
					MaxCcvsPerMsg:         1,
					FeeAggregator:         solana.PublicKey{},
					AllowedFinalityConfig: executor.FinalityConfig{}, // empty == wait for finality
					CcvAllowlistEnabled:   false,
				},
				config.ExecConfigPDA,
				admin.PublicKey(),
				solana.SystemProgramID,
				config.ExecEventAuthorityPDA,
				config.ExecutorProgram,
				execProgramDataAddress,
			)
			require.NoError(t, err)

			result := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)
			require.NotNil(t, result)

			var cfg executor.Config
			err = common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, config.ExecConfigPDA, config.DefaultCommitment, &cfg)
			require.NoError(t, err)
			require.Equal(t, uint8(1), cfg.Version)
			require.Equal(t, admin.PublicKey(), cfg.Owner)
			require.Equal(t, solana.PublicKey{}, cfg.ProposedOwner)
		})
	})

	t.Run("ownership", func(t *testing.T) {
		t.Run("transfer_ownership fails when called by non-owner", func(t *testing.T) {
			ix, err := executor.NewTransferOwnershipInstruction(
				newAdmin.PublicKey(),
				config.ExecConfigPDA,
				user.PublicKey(),
				config.ExecEventAuthorityPDA,
				config.ExecutorProgram,
			)
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ix}, user, config.DefaultCommitment,
				[]string{"Error Code: " + "Unauthorized"})
		})

		t.Run("transfer_ownership fails with default pubkey", func(t *testing.T) {
			resolved, err := ccip.ResolveExecTransferOwnershipAccounts(ctx, solanaGoClient, config.ExecutorProgram, solana.PublicKey{}, admin.PublicKey(), logger.Test(t))
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, admin, config.DefaultCommitment,
				[]string{"Error Code: " + "DefaultOwnerProposal"})
		})

		t.Run("transfer_ownership fails when proposing current owner", func(t *testing.T) {
			resolved, err := ccip.ResolveExecTransferOwnershipAccounts(ctx, solanaGoClient, config.ExecutorProgram, admin.PublicKey(), admin.PublicKey(), logger.Test(t))
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, admin, config.DefaultCommitment,
				[]string{"Error Code: " + "RedundantOwnerProposal"})
		})

		t.Run("transfer_ownership succeeds and emits event", func(t *testing.T) {
			resolved, err := ccip.ResolveExecTransferOwnershipAccounts(ctx, solanaGoClient, config.ExecutorProgram, newAdmin.PublicKey(), admin.PublicKey(), logger.Test(t))
			require.NoError(t, err)

			result := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, admin, config.DefaultCommitment)
			require.NotNil(t, result)

			tx, err := result.Transaction.GetTransaction()
			require.NoError(t, err)
			event, err := common.ExtractCPIEvent(tx, result.Meta, config.ExecutorProgram, executor.ParseEvent_OwnershipTransferRequested)
			require.NoError(t, err)
			require.Equal(t, admin.PublicKey(), event.From)
			require.Equal(t, newAdmin.PublicKey(), event.To)

			var cfg executor.Config
			err = common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, config.ExecConfigPDA, config.DefaultCommitment, &cfg)
			require.NoError(t, err)
			require.Equal(t, admin.PublicKey(), cfg.Owner)
			require.Equal(t, newAdmin.PublicKey(), cfg.ProposedOwner)
		})

		t.Run("accept_ownership fails when called by wrong signer", func(t *testing.T) {
			ix, err := executor.NewAcceptOwnershipInstruction(
				config.ExecConfigPDA,
				user.PublicKey(),
				config.ExecEventAuthorityPDA,
				config.ExecutorProgram,
			)
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ix}, user, config.DefaultCommitment,
				[]string{"Error Code: " + "Unauthorized"})
		})

		t.Run("accept_ownership succeeds and emits event", func(t *testing.T) {
			resolved, err := ccip.ResolveExecAcceptOwnershipAccounts(ctx, solanaGoClient, config.ExecutorProgram, newAdmin.PublicKey(), logger.Test(t))
			require.NoError(t, err)

			result := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, newAdmin, config.DefaultCommitment)
			require.NotNil(t, result)

			tx, err := result.Transaction.GetTransaction()
			require.NoError(t, err)
			event, err := common.ExtractCPIEvent(tx, result.Meta, config.ExecutorProgram, executor.ParseEvent_OwnershipTransferred)
			require.NoError(t, err)
			require.Equal(t, admin.PublicKey(), event.From)
			require.Equal(t, newAdmin.PublicKey(), event.To)

			var cfg executor.Config
			err = common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, config.ExecConfigPDA, config.DefaultCommitment, &cfg)
			require.NoError(t, err)
			require.Equal(t, newAdmin.PublicKey(), cfg.Owner)
			require.Equal(t, solana.PublicKey{}, cfg.ProposedOwner)
		})

		t.Run("transfer ownership back to admin", func(t *testing.T) {
			resolved, err := ccip.ResolveExecTransferOwnershipAccounts(ctx, solanaGoClient, config.ExecutorProgram, admin.PublicKey(), newAdmin.PublicKey(), logger.Test(t))
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, newAdmin, config.DefaultCommitment)

			resolved, err = ccip.ResolveExecAcceptOwnershipAccounts(ctx, solanaGoClient, config.ExecutorProgram, admin.PublicKey(), logger.Test(t))
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, admin, config.DefaultCommitment)
		})
	})

	t.Run("set_config", func(t *testing.T) {
		t.Run("updates config", func(t *testing.T) {
			params := executor.SetConfigParams{
				FeeAggregator:         solana.PublicKey{},
				AllowedFinalityConfig: executor.FinalityConfig{BlockDepth: 50},
				CcvAllowlistEnabled:   false,
			}
			ix, err := executor.NewSetConfigInstruction(
				params,
				config.ExecConfigPDA,
				admin.PublicKey(),
				config.ExecEventAuthorityPDA,
				config.ExecutorProgram,
			)
			require.NoError(t, err)

			result := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)
			require.NotNil(t, result)

			tx, err := result.Transaction.GetTransaction()
			require.NoError(t, err)
			event, err := common.ExtractCPIEvent(tx, result.Meta, config.ExecutorProgram, executor.ParseEvent_ConfigSet)
			require.NoError(t, err)
			require.Equal(t, params.FeeAggregator, event.FeeAggregator)
			require.Equal(t, params.AllowedFinalityConfig, event.AllowedFinalityConfig)
			require.Equal(t, params.CcvAllowlistEnabled, event.CcvAllowlistEnabled)

			var cfg executor.Config
			err = common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, config.ExecConfigPDA, config.DefaultCommitment, &cfg)
			require.NoError(t, err)
			require.Equal(t, params.FeeAggregator, cfg.FeeAggregator)
			require.Equal(t, params.AllowedFinalityConfig, cfg.AllowedFinalityConfig)
			require.Equal(t, params.CcvAllowlistEnabled, cfg.CcvAllowlistEnabled)
		})

		t.Run("fails when called by non-owner", func(t *testing.T) {
			ix, err := executor.NewSetConfigInstruction(
				executor.SetConfigParams{},
				config.ExecConfigPDA,
				user.PublicKey(),
				config.ExecEventAuthorityPDA,
				config.ExecutorProgram,
			)
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ix}, user, config.DefaultCommitment,
				[]string{"Error Code: " + "Unauthorized"})
		})
	})

	t.Run("set_chain_config", func(t *testing.T) {
		t.Run("creates and populates a new chain config", func(t *testing.T) {
			resolved, err := ccip.ResolveExecSetChainConfigAccounts(ctx, solanaGoClient, config.ExecutorProgram, executor.SetChainConfigParams{
				RemoteChainSelector: remoteChainSelector,
				UsdCentsFee:         executor.UsdCents{V0: 100},
				Enabled:             false,
			}, admin.PublicKey(), logger.Test(t))
			require.NoError(t, err)

			result := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, admin, config.DefaultCommitment)
			require.NotNil(t, result)

			chainConfigPDA := resolved.AccountsToSave[2].PublicKey
			var cfg executor.ChainConfig
			err = common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, chainConfigPDA, config.DefaultCommitment, &cfg)
			require.NoError(t, err)
			require.Equal(t, uint8(1), cfg.Version)
			require.Equal(t, uint32(100), cfg.UsdCentsFee.V0)
			require.False(t, cfg.Enabled)
		})

		t.Run("overwrites an existing chain config", func(t *testing.T) {
			resolved, err := ccip.ResolveExecSetChainConfigAccounts(ctx, solanaGoClient, config.ExecutorProgram, executor.SetChainConfigParams{
				RemoteChainSelector: remoteChainSelector,
				UsdCentsFee:         executor.UsdCents{V0: 200},
				Enabled:             true,
			}, admin.PublicKey(), logger.Test(t))
			require.NoError(t, err)

			result := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, admin, config.DefaultCommitment)
			require.NotNil(t, result)

			chainConfigPDA := resolved.AccountsToSave[2].PublicKey
			var cfg executor.ChainConfig
			err = common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, chainConfigPDA, config.DefaultCommitment, &cfg)
			require.NoError(t, err)
			require.Equal(t, uint32(200), cfg.UsdCentsFee.V0)
			require.True(t, cfg.Enabled)
		})

		t.Run("fails when called by non-owner", func(t *testing.T) {
			chainConfigPDA, _, err := state.FindExecChainConfigPDA(remoteChainSelector, config.ExecutorProgram)
			require.NoError(t, err)

			ix, err := executor.NewSetChainConfigInstruction(
				executor.SetChainConfigParams{
					RemoteChainSelector: remoteChainSelector,
					UsdCentsFee:         executor.UsdCents{V0: 300},
					Enabled:             true,
				},
				config.ExecConfigPDA,
				user.PublicKey(),
				chainConfigPDA,
				solana.SystemProgramID,
				config.ExecEventAuthorityPDA,
				config.ExecutorProgram,
			)
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ix}, user, config.DefaultCommitment,
				[]string{"Error Code: " + "Unauthorized"})
		})
	})

	t.Run("set_allowed_ccvs", func(t *testing.T) {
		t.Run("updates allowed ccvs", func(t *testing.T) {
			ccvs := []solana.PublicKey{user.PublicKey()}
			allowlistEnabled := true
			ix, err := executor.NewSetAllowedCcvsInstruction(
				executor.SetAllowedCcvsParams{
					Ccvs:             ccvs,
					AllowlistEnabled: allowlistEnabled,
				},
				config.ExecConfigPDA,
				admin.PublicKey(),
				config.ExecEventAuthorityPDA,
				config.ExecutorProgram,
			)

			require.NoError(t, err)
			result := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)
			require.NotNil(t, result)

			var cfg executor.Config
			err = common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, config.ExecConfigPDA, config.DefaultCommitment, &cfg)
			require.NoError(t, err)
			require.Equal(t, ccvs, cfg.AllowedCcvs)
			require.Equal(t, allowlistEnabled, cfg.CcvAllowlistEnabled)
		})

		t.Run("fails when called by non-owner", func(t *testing.T) {
			ccvs := []solana.PublicKey{}
			ix, err := executor.NewSetAllowedCcvsInstruction(
				executor.SetAllowedCcvsParams{
					Ccvs:             ccvs,
					AllowlistEnabled: false,
				},
				config.ExecConfigPDA,
				user.PublicKey(),
				config.ExecEventAuthorityPDA,
				config.ExecutorProgram,
			)
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ix}, user, config.DefaultCommitment,
				[]string{"Error Code: " + "Unauthorized"})
		})
	})

	t.Run("get_fee", func(t *testing.T) {
		validCCV := user.PublicKey()
		feeToken := solana.PublicKey{}

		chainConfigPDA, _, err := state.FindExecChainConfigPDA(remoteChainSelector, config.ExecutorProgram)
		require.NoError(t, err)

		t.Run("invalid requested finality", func(t *testing.T) {
			ix, err := executor.NewGetFeeInstruction(
				executor.GetFeeParams{
					DestChainSelector: remoteChainSelector,
					RequestedFinality: executor.FinalityConfig{BlockDepth: 49},
					Ccvs:              []solana.PublicKey{validCCV},
					ExtraArgs:         []byte{},
					FeeToken:          feeToken,
				},
				config.ExecConfigPDA,
				chainConfigPDA,
				config.ExecEventAuthorityPDA,
				config.ExecutorProgram,
			)
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ix}, user, config.DefaultCommitment,
				[]string{"Error Code: " + "InvalidFinality"})
		})

		// A dest chain with no config account at all fails at Anchor account resolution with
		// AccountNotInitialized — this is a distinct condition from a configured-but-disabled chain
		// (see "disabled dest chain" below, which reaches the handler and returns InvalidDestChain).
		t.Run("uninitialized dest chain", func(t *testing.T) {
			invalidChain := uint64(1)
			ccPDA, _, err := state.FindExecChainConfigPDA(invalidChain, config.ExecutorProgram)
			require.NoError(t, err)
			ix, err := executor.NewGetFeeInstruction(
				executor.GetFeeParams{
					DestChainSelector: invalidChain,
					RequestedFinality: executor.FinalityConfig{}, // wait for finality
					Ccvs:              []solana.PublicKey{validCCV},
					ExtraArgs:         []byte{},
					FeeToken:          feeToken,
				},
				config.ExecConfigPDA,
				ccPDA,
				config.ExecEventAuthorityPDA,
				config.ExecutorProgram,
			)
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ix}, user, config.DefaultCommitment,
				[]string{"Error Code: " + "AccountNotInitialized"})
		})

		t.Run("disabled dest chain", func(t *testing.T) {
			disabledChain := uint64(54321)

			// Create the chain config with enabled=false so get_fee reaches the handler's
			// enabled check and returns InvalidDestChain (not AccountNotInitialized).
			resolved, err := ccip.ResolveExecSetChainConfigAccounts(ctx, solanaGoClient, config.ExecutorProgram, executor.SetChainConfigParams{
				RemoteChainSelector: disabledChain,
				UsdCentsFee:         executor.UsdCents{V0: 100},
				Enabled:             false,
			}, admin.PublicKey(), logger.Test(t))
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, admin, config.DefaultCommitment)

			ccPDA, _, err := state.FindExecChainConfigPDA(disabledChain, config.ExecutorProgram)
			require.NoError(t, err)
			ix, err := executor.NewGetFeeInstruction(
				executor.GetFeeParams{
					DestChainSelector: disabledChain,
					RequestedFinality: executor.FinalityConfig{}, // wait for finality
					Ccvs:              []solana.PublicKey{validCCV},
					ExtraArgs:         []byte{},
					FeeToken:          feeToken,
				},
				config.ExecConfigPDA,
				ccPDA,
				config.ExecEventAuthorityPDA,
				config.ExecutorProgram,
			)
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ix}, user, config.DefaultCommitment,
				[]string{"Error Code: " + "InvalidDestChain"})
		})

		t.Run("unsupported required CCV", func(t *testing.T) {
			ix, err := executor.NewGetFeeInstruction(
				executor.GetFeeParams{
					DestChainSelector: remoteChainSelector,
					RequestedFinality: executor.FinalityConfig{}, // wait for finality
					Ccvs:              []solana.PublicKey{{}},
					ExtraArgs:         []byte{},
					FeeToken:          feeToken,
				},
				config.ExecConfigPDA,
				chainConfigPDA,
				config.ExecEventAuthorityPDA,
				config.ExecutorProgram,
			)
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ix}, user, config.DefaultCommitment,
				[]string{"Error Code: " + "InvalidCCV"})
		})

		t.Run("exceeds max CCVs", func(t *testing.T) {
			ix, err := executor.NewGetFeeInstruction(
				executor.GetFeeParams{
					DestChainSelector: remoteChainSelector,
					RequestedFinality: executor.FinalityConfig{}, // wait for finality
					Ccvs:              []solana.PublicKey{validCCV, validCCV},
					ExtraArgs:         []byte{},
					FeeToken:          feeToken,
				},
				config.ExecConfigPDA,
				chainConfigPDA,
				config.ExecEventAuthorityPDA,
				config.ExecutorProgram,
			)
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ix}, user, config.DefaultCommitment,
				[]string{"Error Code: " + "ExceedsMaxCCVs"})
		})
	})

	t.Run("withdraw_fee_tokens", func(t *testing.T) {
		mint := solana.MustPrivateKeyFromBase58("4dD1x6rv1uLHKWCrYBY9WYa781YgNQGocVpqrS1EzfDQAq9TK4Vdyju6eLXicoSmjiGU9uZ9ExJHmC5GzwGoQUWD")

		tokenProgram := solana.Token2022ProgramID

		ixs, ixErr := tokens.CreateToken(ctx, tokenProgram, mint.PublicKey(), admin.PublicKey(), 9, solanaGoClient, config.DefaultCommitment)
		require.NoError(t, ixErr)

		// ccip_send_v2 pays every executor receipt fee into the ATA of the executor's ["fee_vault"]
		// PDA (see ccip_common::seed::vault_ata), so that ATA is the only account
		// withdraw_fee_tokens can sweep.
		feeVault, _, err := state.FindFeeVaultPDA(executor.ProgramID, nil)
		require.NoError(t, err)

		ixAta, feeVaultATA, ataErr := tokens.CreateAssociatedTokenAccount(tokenProgram, mint.PublicKey(), feeVault, admin.PublicKey())
		require.NoError(t, ataErr)
		expectedFeeVaultATA, err := tokens.FindFeeVaultATA(tokenProgram, mint.PublicKey(), executor.ProgramID, nil)
		require.NoError(t, err)
		require.Equal(t, expectedFeeVaultATA, feeVaultATA)

		ixAtaOwner, adminATA, ataOwnerErr := tokens.CreateAssociatedTokenAccount(tokenProgram, mint.PublicKey(), admin.PublicKey(), admin.PublicKey())
		require.NoError(t, ataOwnerErr)

		testutils.SendAndConfirm(ctx, t, solanaGoClient, append(ixs, ixAta, ixAtaOwner), admin, config.DefaultCommitment, common.AddSigners(mint))

		t.Run("fails if fee aggregator unset", func(t *testing.T) {
			ix, err := executor.NewWithdrawFeeTokensInstruction(
				config.ExecConfigPDA,
				feeVaultATA,
				feeVault,
				adminATA,
				mint.PublicKey(),
				tokenProgram,
			)
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ix}, user, config.DefaultCommitment,
				[]string{"Error Code: "}) // NOTE: we can't actually fully test this since the SDK won't even allow initializing the zero address ATA
		})

		t.Run("fails if no funds", func(t *testing.T) {
			// configure a valid fee aggregator address
			ix, err := executor.NewSetConfigInstruction(
				executor.SetConfigParams{
					FeeAggregator:         admin.PublicKey(),         // aggregate funds to admin wallet
					AllowedFinalityConfig: executor.FinalityConfig{}, // empty == wait for finality
					CcvAllowlistEnabled:   false,
				},
				config.ExecConfigPDA,
				admin.PublicKey(),
				config.ExecEventAuthorityPDA,
				config.ExecutorProgram,
			)
			require.NoError(t, err)

			result := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)
			require.NotNil(t, result)

			ix, err = executor.NewWithdrawFeeTokensInstruction(
				config.ExecConfigPDA,
				feeVaultATA,
				feeVault,
				adminATA,
				mint.PublicKey(),
				tokenProgram,
			)
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ix}, user, config.DefaultCommitment,
				[]string{"Error Code: " + "InsufficientFunds"})
		})

		t.Run("withdraws tokens", func(t *testing.T) {
			// fund the vault
			ixMintTo, mintErr := tokens.MintTo(123, tokenProgram, mint.PublicKey(), feeVaultATA, admin.PublicKey())
			require.NoError(t, mintErr)
			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ixMintTo}, admin, config.DefaultCommitment, common.AddSigners(mint))

			ix, err := executor.NewWithdrawFeeTokensInstruction(
				config.ExecConfigPDA,
				feeVaultATA,
				feeVault,
				adminATA,
				mint.PublicKey(),
				tokenProgram,
			)
			require.NoError(t, err)

			result := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)
			require.NotNil(t, result)

			_, balance, err := tokens.TokenBalance(ctx, solanaGoClient, adminATA, config.DefaultCommitment)
			require.NoError(t, err)
			require.NotZero(t, balance)
		})
	})
}
