// example_sender_v2_test.go exercises the example-ccip-sender-v2 program, which wraps the router's
// ccip_send_v2 via CPI. It is the only on-chain caller of that instruction in the repo, so it also
// covers the CPI-specific mechanics: resolving the router's account list for a program PDA rather
// than for the payer, forwarding that list verbatim, and paying fees through the PDA.
package contracts

import (
	"fmt"
	"testing"

	"github.com/gagliardetto/solana-go"
	computebudget "github.com/gagliardetto/solana-go/programs/compute-budget"
	"github.com/gagliardetto/solana-go/programs/system"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/ccip_common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/ccip_router"
	sender "github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/example_ccip_sender_v2"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/fee_quoter"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/config"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/testutils"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/ccip"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/tokens"
)

// senderPDAFunding is the balance the sender PDA is given to check that a send never spends it.
const senderPDAFunding = 1_000_000_000

func TestExampleSenderV2(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	env := testutils.NewTestEnv(t).WithTrafficConfig().Build()

	program := config.CcipBaseSenderV2
	user := testutils.User

	// Token1: a plain burn/mint token whose pool signer is the mint authority.
	const token1Decimals = 18

	// The fee bound every send in this file states. Comfortably above the quoted fee for both fee
	// tokens (a $0.50 network fee plus execution gas, and a token transfer fee when bridging,
	// priced at ~$19.82/SOL and ~$198.17/LINK22), and below both the caller's SOL balance and its
	// LINK22 balance and delegation. Whatever the router does not bill is returned to the caller by
	// the send itself.
	const maxFee = 2e8

	senderState, _, err := solana.FindProgramAddress([][]byte{[]byte("state")}, program)
	require.NoError(t, err)
	senderPDA, _, err := solana.FindProgramAddress([][]byte{[]byte("ccip_sender")}, program)
	require.NoError(t, err)
	chainConfigPDA := func(selector uint64) solana.PublicKey {
		pda, _, cErr := solana.FindProgramAddress(
			[][]byte{[]byte("remote_chain_config"), common.Uint64ToLE(selector)}, program)
		require.NoError(t, cErr)
		return pda
	}
	senderEvmChainConfig := chainConfigPDA(config.EvmChainSelector)
	senderSvmChainConfig := chainConfigPDA(config.SvmChainSelector)

	// The same extra args the program stores per chain and replays on every message. The committee
	// verifier is the CCV registered for the v2 lanes in the shared setup.
	extraArgs := testutils.MustSerializeExtraArgs(t, ccip_common.GenericExtraArgsV3{
		GasLimit: ccip_common.CrossChainGas{V0: 5000},
		Ccvs:     []solana.PublicKey{config.CommitteeVerifierProgram},
		CcvArgs:  [][]byte{{}},
	}, ccip.GenericExtraArgsV3Tag)

	// sendLookupTables fetches the tables the resolver asked for: the router's onramp table, which
	// the sender's resolution passes through from the router's, and e.g. a token pool's. A
	// resolved v2 account list does not fit in a LUT-less transaction.
	lamportBalance := func(account solana.PublicKey) uint64 {
		balance, bErr := env.Client.GetBalance(ctx, account, config.DefaultCommitment)
		require.NoError(t, bErr)
		return balance.Value
	}

	sendLookupTables := func(resolved ccip.ResolveAccountsResult) map[solana.PublicKey]solana.PublicKeySlice {
		lookupTables := make(map[solana.PublicKey]solana.PublicKeySlice, len(resolved.LookupTables))
		for _, table := range resolved.LookupTables {
			entries, lutErr := common.GetAddressLookupTable(ctx, env.Client, table)
			require.NoError(t, lutErr)
			lookupTables[table] = entries
		}
		return lookupTables
	}

	receiptsTotal := func(receipts []ccip_router.Receipt) uint64 {
		var sum uint64
		for _, r := range receipts {
			sum += r.FeeTokenAmount
		}
		return sum
	}

	t.Run("setup", func(t *testing.T) {
		// The router returns this table from resolution, and the sender's resolution passes it on.
		env.SetOnrampLookupTable()

		programData := testutils.GetProgramDataAddress(ctx, t, env.Client, program)

		// Only the program's upgrade authority may initialize it.
		userInitIx, iErr := sender.NewInitializeInstruction(
			config.CcipRouterProgram, senderState, senderPDA, user.PublicKey(), solana.SystemProgramID, program, programData)
		require.NoError(t, iErr)
		testutils.SendAndFailWith(ctx, t, env.Client, []solana.Instruction{userInitIx}, user, config.DefaultCommitment, []string{"Error Code: Unauthorized"})

		// The upgrade authority initializes it and hands ownership to the user, who drives it below.
		initIx, iErr := sender.NewInitializeInstruction(
			config.CcipRouterProgram, senderState, senderPDA, testutils.Admin.PublicKey(), solana.SystemProgramID, program, programData)
		require.NoError(t, iErr)
		transferOwnershipIx, tErr := sender.NewTransferOwnershipInstruction(user.PublicKey(), senderState, testutils.Admin.PublicKey())
		require.NoError(t, tErr)
		testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{initIx, transferOwnershipIx}, testutils.Admin, config.DefaultCommitment)
		acceptOwnershipIx, aErr := sender.NewAcceptOwnershipInstruction(senderState, user.PublicKey())
		require.NoError(t, aErr)

		evmIx, eErr := sender.NewInitChainConfigInstruction(
			sender.ChainConfigParams{
				ChainSelector:  config.EvmChainSelector,
				Recipient:      env.ValidEvmAbiReceiverAddress[:],
				ExtraArgsBytes: extraArgs,
			},
			senderState, senderEvmChainConfig, user.PublicKey(), solana.SystemProgramID)
		require.NoError(t, eErr)

		svmIx, sErr := sender.NewInitChainConfigInstruction(
			sender.ChainConfigParams{
				ChainSelector:  config.SvmChainSelector,
				Recipient:      env.ValidSvmReceiverAddress[:],
				ExtraArgsBytes: extraArgs,
			},
			senderState, senderSvmChainConfig, user.PublicKey(), solana.SystemProgramID)
		require.NoError(t, sErr)

		// The sender PDA starts unfunded, as deployments leave it: native fees come out of the
		// caller's own deposit.
		testutils.SendAndConfirm(ctx, t, env.Client,
			[]solana.Instruction{acceptOwnershipIx, evmIx, svmIx}, user, config.DefaultCommitment)

		// The PDA schema version and both bumps must actually land on-chain: ccip_send reads
		// state.ccip_sender_bump to sign its CPI, so a defaulted bump would break every send.
		var stateAccount sender.BaseState
		require.NoError(t, common.GetAccountDataBorshIntoAnchor(
			ctx, env.Client, senderState, config.DefaultCommitment, &stateAccount))
		require.Equal(t, uint8(1), stateAccount.Version)
		require.NotZero(t, stateAccount.Bump)
		require.NotZero(t, stateAccount.CcipSenderBump)
		require.Equal(t, config.CcipRouterProgram, stateAccount.Router)
		require.Equal(t, user.PublicKey(), stateAccount.Owner)

		// The router validates the sender PDA's token accounts before doing any work, so they must
		// exist up front. init_ccip_sender_ata is the program's own idempotent creator.
		for _, mint := range []struct {
			mint    solana.PublicKey
			program solana.PublicKey
		}{
			{solana.WrappedSol, config.SPLTokenProgram},
			{env.Link22.Mint, config.Token2022Program},
			{env.Token1.Mint, env.Token1.Program},
		} {
			ata, _, aErr := tokens.FindAssociatedTokenAddress(mint.program, mint.mint, senderPDA)
			require.NoError(t, aErr)
			ix, ixErr := sender.NewInitCcipSenderAtaInstruction(
				senderState, ata, mint.mint, senderPDA, user.PublicKey(), mint.program,
				solana.SPLAssociatedTokenAccountProgramID, solana.SystemProgramID)
			require.NoError(t, ixErr)
			testutils.SendAndConfirm(ctx, t, env.Client,
				[]solana.Instruction{ix}, user, config.DefaultCommitment)
		}
	})

	// buildSend runs the sender's own account resolution, which returns the complete ccip_send
	// instruction: its named accounts, its transfer-token block when the message carries a
	// transfer, and the router's resolved list, which the sender's resolver forwards from its CPI
	// into the router's own stages.
	buildSend := func(
		t *testing.T,
		selector uint64,
		feeToken solana.PublicKey,
		feeTokenMint solana.PublicKey,
		tokenAmount *sender.SvmTokenAmount,
		transferTokenMint solana.PublicKey,
	) (solana.Instruction, ccip.ResolveAccountsResult) {
		t.Helper()

		resolved, rErr := ccip.ResolveExampleSenderV2SendAccounts(
			ctx, env.Client, program, config.CcipRouterProgram,
			sender.CcipSendParams{
				DestChainSelector: selector,
				TokenAmount:       tokenAmount,
				Data:              []byte{1, 2, 3},
				FeeToken:          feeToken,
				MaxFee:            maxFee,
			},
			feeTokenMint, transferTokenMint, user.PublicKey(), logger.Test(t))
		require.NoError(t, rErr)

		return resolved.Ix, resolved
	}

	feeConfigs := []struct {
		name string
		// The zero address pays in native SOL, but fees are still quoted against wrapped SOL,
		// which is the mint the resolver derives the caller's fee account from.
		feeToken     solana.PublicKey
		feeTokenMint solana.PublicKey
		userFeeATA   solana.PublicKey
	}{
		{"sol", solana.PublicKey{}, solana.WrappedSol, env.WSOL.UserATA},
		{"link", env.Link22.Mint, env.Link22.Mint, env.Link22.UserATA},
	}

	// Only the EVM lane is provisioned as a CCIP 2.0 lane in the shared env (the router's
	// DestChainCcipV2 and the fee quoter's v2 dest chain config are added for it alone), so it is
	// the only destination ccip_send_v2 can target here.
	chainConfigs := []struct {
		name     string
		selector uint64
	}{
		{"EVM", config.EvmChainSelector},
	}

	for _, cc := range chainConfigs {
		t.Run("SVM->"+cc.name, func(t *testing.T) {
			for _, fc := range feeConfigs {
				t.Run(fmt.Sprintf("billing-%s/message_only", fc.name), func(t *testing.T) {
					ix, resolved := buildSend(t, cc.selector, fc.feeToken, fc.feeTokenMint,
						nil, solana.PublicKey{})

					// The user delegates the fee to the sender PDA, which then re-delegates to the
					// router's billing signer as part of the send.
					var ixs []solana.Instruction
					var callerFeeBalanceBefore uint64
					if fc.feeToken.IsZero() {
						callerFeeBalanceBefore = lamportBalance(user.PublicKey())
					} else {
						approve, aErr := tokens.TokenApproveChecked(1e9, 9, env.Link22.Program,
							env.Link22.UserATA, env.Link22.Mint, senderPDA, user.PublicKey(), nil)
						require.NoError(t, aErr)
						ixs = append(ixs, approve)
						callerFeeBalanceBefore = env.GetBalance(fc.userFeeATA)
					}
					ixs = append(ixs, ix)

					result := testutils.SendAndConfirmWithLookupTables(ctx, t, env.Client, ixs, user,
						config.DefaultCommitment, sendLookupTables(resolved),
						common.AddComputeUnitLimit(computebudget.MAX_COMPUTE_UNIT_LIMIT))
					require.NotNil(t, result)

					logUsage(t, cc.name+"/"+fc.name+" message_only", result, ix)

					tx, txErr := result.Transaction.GetTransaction()
					require.NoError(t, txErr)

					routerEvent, evErr := common.ExtractCPIEvent(tx, result.Meta,
						config.CcipRouterProgram, ccip_router.ParseEvent_CcipMessageSentV2)
					require.NoError(t, evErr)
					require.Equal(t, cc.selector, routerEvent.DestChainSelector)
					require.Equal(t, senderPDA, routerEvent.Sender)
					require.NotEqual(t, [32]uint8{}, routerEvent.MessageId)

					senderEvent, sevErr := common.ExtractCPIEvent(tx, result.Meta, program,
						sender.ParseEvent_MessageSent)
					require.NoError(t, sevErr)
					require.Equal(t, cc.selector, senderEvent.DestChainSelector)
					require.Equal(t, fc.feeToken, senderEvent.FeeToken)
					// The id comes back as ccip_send_v2's return value, so the sender program
					// reports the same one the router recorded.
					require.Equal(t, routerEvent.MessageId, senderEvent.MessageId)
					// The program reports what the router actually billed - the sum of the
					// receipts it issued - not the max_fee it was allowed to spend.
					billed := receiptsTotal(routerEvent.Receipts)
					require.Equal(t, billed, senderEvent.FeeAmount)
					require.Less(t, billed, uint64(maxFee), "max_fee must exceed the real fee for the refund path to run")

					if fc.feeToken.IsZero() {
						// The caller deposited max_fee, got the unbilled part back, and also paid
						// the transaction fee; the PDA ends as empty as it started.
						require.Equal(t, callerFeeBalanceBefore-billed-result.Meta.Fee,
							lamportBalance(user.PublicKey()))
						require.Zero(t, lamportBalance(senderPDA))
					} else {
						// max_fee left the caller's account and only the billed part stayed gone:
						// the unspent remainder is returned before the instruction ends, rather
						// than settling in the sender PDA's own token account.
						require.Equal(t, callerFeeBalanceBefore-billed, env.GetBalance(fc.userFeeATA))
						senderFeeATA, _, sErr := tokens.FindAssociatedTokenAddress(
							env.Link22.Program, env.Link22.Mint, senderPDA)
						require.NoError(t, sErr)
						require.Zero(t, env.GetBalance(senderFeeATA))
					}
				})
			}

			for _, fc := range feeConfigs {
				t.Run(fmt.Sprintf("billing-%s/with_tokens", fc.name), func(t *testing.T) {
					userTokenATA := env.Token1.User[user.PublicKey()]
					tokenAmount := &sender.SvmTokenAmount{Token: env.Token1.Mint, Amount: 1}

					ix, resolved := buildSend(t, cc.selector, fc.feeToken, fc.feeTokenMint,
						tokenAmount, env.Token1.Mint)

					approveToken, aErr := tokens.TokenApproveChecked(1, token1Decimals, env.Token1.Program,
						userTokenATA, env.Token1.Mint, senderPDA, user.PublicKey(), nil)
					require.NoError(t, aErr)
					ixs := []solana.Instruction{approveToken}
					var callerFeeBalanceBefore uint64
					if fc.feeToken.IsZero() {
						callerFeeBalanceBefore = lamportBalance(user.PublicKey())
					} else {
						approveFee, fErr := tokens.TokenApproveChecked(1e9, 9, env.Link22.Program,
							env.Link22.UserATA, env.Link22.Mint, senderPDA, user.PublicKey(), nil)
						require.NoError(t, fErr)
						ixs = append(ixs, approveFee)
						callerFeeBalanceBefore = env.GetBalance(fc.userFeeATA)
					}
					ixs = append(ixs, ix)

					result := testutils.SendAndConfirmWithLookupTables(ctx, t, env.Client, ixs, user,
						config.DefaultCommitment, sendLookupTables(resolved),
						common.AddComputeUnitLimit(computebudget.MAX_COMPUTE_UNIT_LIMIT))
					require.NotNil(t, result)
					logUsage(t, cc.name+"/"+fc.name+" with_tokens", result, ix)

					tx, txErr := result.Transaction.GetTransaction()
					require.NoError(t, txErr)
					routerEvent, evErr := common.ExtractCPIEvent(tx, result.Meta,
						config.CcipRouterProgram, ccip_router.ParseEvent_CcipMessageSentV2)
					require.NoError(t, evErr)
					require.Equal(t, senderPDA, routerEvent.Sender)
					require.NotEqual(t, [32]uint8{}, routerEvent.MessageId)

					// The refunds and the delegation revoke run after the router CPI, so this
					// also covers the id surviving those intervening CPIs' return data.
					senderEvent, sevErr := common.ExtractCPIEvent(tx, result.Meta, program,
						sender.ParseEvent_MessageSent)
					require.NoError(t, sevErr)
					require.Equal(t, routerEvent.MessageId, senderEvent.MessageId)

					// The token transfer fee makes this the most expensive send, so the refund of
					// the unspent max_fee is checked here too.
					billed := receiptsTotal(routerEvent.Receipts)
					require.Equal(t, billed, senderEvent.FeeAmount)
					require.Less(t, billed, uint64(maxFee), "max_fee must exceed the real fee for the refund path to run")

					if fc.feeToken.IsZero() {
						require.Equal(t, callerFeeBalanceBefore-billed-result.Meta.Fee,
							lamportBalance(user.PublicKey()))
						require.Zero(t, lamportBalance(senderPDA))
					} else {
						require.Equal(t, callerFeeBalanceBefore-billed, env.GetBalance(fc.userFeeATA))
						senderFeeATA, _, sErr := tokens.FindAssociatedTokenAddress(
							env.Link22.Program, env.Link22.Mint, senderPDA)
						require.NoError(t, sErr)
						require.Zero(t, env.GetBalance(senderFeeATA))
					}
				})
			}

			// Bridging the token that also pays the fees is the one case where the sender's
			// unspent-fee accounting shares an account with the transfer: both legs move through
			// the sender PDA's single LINK22 ATA, and both are covered by one approval of
			// `amount + max_fee` (approve_checked replaces the delegated amount rather than adding
			// to it, so the legs cannot be approved separately).
			t.Run("billing-link/with_tokens_same_mint", func(t *testing.T) {
				// LINK22 is a billing token in the shared setup and has a pool, but is never
				// bridged there, so its per-chain transfer fee config has to be added before the
				// fee quoter will price it as a transferred token.
				feeCfgIx, bErr := fee_quoter.NewSetTokenTransferFeeConfigInstruction(
					cc.selector, env.Link22.Mint,
					fee_quoter.TokenTransferFeeConfig{
						DestBytesOverhead: 32,
						MinFee:            fee_quoter.UsdCents{V0: 0},
						MaxFee:            fee_quoter.UsdCents{V0: 1},
					},
					config.FqConfigPDA, env.LinkPool.Billing[cc.selector],
					testutils.CcipAdmin.PublicKey(), solana.SystemProgramID)
				require.NoError(t, bErr)
				testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{feeCfgIx},
					testutils.CcipAdmin, config.DefaultCommitment)

				// The pool's fee vault is qualified by the mint of the managed token, so bridging
				// LINK22 bills a different vault than the shared setup's (which is qualified by
				// Token1's mint). ccip_send_v2 fails with MissingAtaForReceiptIssuer unless it
				// exists.
				poolVaultPDA, _, pErr := state.FindFeeVaultPDA(
					config.CcipBurnMintTokenPoolProgram, env.Link22.Mint.Bytes())
				require.NoError(t, pErr)
				vaultAtaIx, _, vErr := tokens.CreateAssociatedTokenAccount(
					env.Link22.Program, env.Link22.Mint, poolVaultPDA, testutils.Admin.PublicKey())
				require.NoError(t, vErr)
				testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{vaultAtaIx},
					testutils.Admin, config.DefaultCommitment)

				const bridged = 1
				tokenAmount := &sender.SvmTokenAmount{Token: env.Link22.Mint, Amount: bridged}
				ix, resolved := buildSend(t, cc.selector, env.Link22.Mint, env.Link22.Mint,
					tokenAmount, env.Link22.Mint)

				// One approval for both legs, which is what the program's single deferred
				// transfer_to_self_and_approve expects.
				approve, aErr := tokens.TokenApproveChecked(bridged+maxFee, 9, env.Link22.Program,
					env.Link22.UserATA, env.Link22.Mint, senderPDA, user.PublicKey(), nil)
				require.NoError(t, aErr)
				callerBalanceBefore := env.GetBalance(env.Link22.UserATA)

				result := testutils.SendAndConfirmWithLookupTables(ctx, t, env.Client,
					[]solana.Instruction{approve, ix}, user, config.DefaultCommitment,
					sendLookupTables(resolved),
					common.AddComputeUnitLimit(computebudget.MAX_COMPUTE_UNIT_LIMIT))
				require.NotNil(t, result)
				logUsage(t, cc.name+"/link with_tokens_same_mint", result, ix)

				tx, txErr := result.Transaction.GetTransaction()
				require.NoError(t, txErr)
				routerEvent, evErr := common.ExtractCPIEvent(tx, result.Meta,
					config.CcipRouterProgram, ccip_router.ParseEvent_CcipMessageSentV2)
				require.NoError(t, evErr)
				require.Equal(t, senderPDA, routerEvent.Sender)

				senderEvent, sevErr := common.ExtractCPIEvent(tx, result.Meta, program,
					sender.ParseEvent_MessageSent)
				require.NoError(t, sevErr)
				require.Equal(t, routerEvent.MessageId, senderEvent.MessageId)

				// The reported fee is the billed part alone: the transferred amount moving through
				// the same account must not be counted as fee spend.
				billed := receiptsTotal(routerEvent.Receipts)
				require.Equal(t, billed, senderEvent.FeeAmount)
				require.Less(t, billed, uint64(maxFee), "max_fee must exceed the real fee for the refund path to run")

				// The caller is out exactly the bridged amount plus what the router billed - the
				// unspent part of max_fee comes back even though it shared an account with the
				// transfer - and nothing is stranded in the sender PDA's ATA.
				require.Equal(t, callerBalanceBefore-billed-bridged, env.GetBalance(env.Link22.UserATA))
				senderFeeATA, _, fErr := tokens.FindAssociatedTokenAddress(
					env.Link22.Program, env.Link22.Mint, senderPDA)
				require.NoError(t, fErr)
				require.Zero(t, env.GetBalance(senderFeeATA))
			})
		})
	}

	t.Run("rejects a source token account that is not the caller's", func(t *testing.T) {
		// The transfer out of the source account is signed by the sender PDA, which users grant a
		// standing delegation to, so an unpinned source account would let anyone drain a third
		// party's balance. The resolver derives the caller's own account, so this substitutes a
		// third party's the way a hand-built account list could.
		victim := testutils.Admin
		victimATA, _, aErr := tokens.FindAssociatedTokenAddress(
			env.Token1.Program, env.Token1.Mint, victim.PublicKey())
		require.NoError(t, aErr)

		ix, resolved := buildSend(t, config.EvmChainSelector, solana.PublicKey{}, solana.WrappedSol,
			&sender.SvmTokenAmount{Token: env.Token1.Mint, Amount: 1}, env.Token1.Mint)

		generic, ok := ix.(*solana.GenericInstruction)
		require.True(t, ok)
		// The source token account is the first of ccip_send's remaining accounts, i.e. straight
		// after the accounts its context declares.
		generic.AccountValues[senderNamedAccounts] = solana.Meta(victimATA).WRITE()

		testutils.SendAndFailWithLookupTables(ctx, t, env.Client, []solana.Instruction{ix}, user,
			config.DefaultCommitment, sendLookupTables(resolved),
			[]string{"InvalidSourceTokenAccount"},
			common.AddComputeUnitLimit(computebudget.MAX_COMPUTE_UNIT_LIMIT))
	})

	// The sender PDA's own token accounts and the router's billing signer are pinned by derivation,
	// so an account list that swaps any of them fails in the sender rather than in the router's CPI.
	t.Run("rejects a token account that is not the sender PDA's", func(t *testing.T) {
		ix, resolved := buildSend(t, config.EvmChainSelector, solana.PublicKey{}, solana.WrappedSol,
			&sender.SvmTokenAmount{Token: env.Token1.Mint, Amount: 1}, env.Token1.Mint)

		userATA, _, aErr := tokens.FindAssociatedTokenAddress(env.Token1.Program, env.Token1.Mint, user.PublicKey())
		require.NoError(t, aErr)

		generic, ok := ix.(*solana.GenericInstruction)
		require.True(t, ok)
		// The sender PDA's account for the transferred token follows the source account.
		generic.AccountValues[senderNamedAccounts+1] = solana.Meta(userATA).WRITE()

		testutils.SendAndFailWithLookupTables(ctx, t, env.Client, []solana.Instruction{ix}, user,
			config.DefaultCommitment, sendLookupTables(resolved),
			[]string{"InvalidCcipSenderAta"},
			common.AddComputeUnitLimit(computebudget.MAX_COMPUTE_UNIT_LIMIT))
	})

	t.Run("rejects a fee token account that is not the sender PDA's", func(t *testing.T) {
		ix, resolved := buildSend(t, config.EvmChainSelector, env.Link22.Mint, env.Link22.Mint, nil, solana.PublicKey{})

		generic, ok := ix.(*solana.GenericInstruction)
		require.True(t, ok)
		generic.AccountValues[senderFeeTokenAccount] = solana.Meta(env.Link22.UserATA)

		testutils.SendAndFailWithLookupTables(ctx, t, env.Client, []solana.Instruction{ix}, user,
			config.DefaultCommitment, sendLookupTables(resolved),
			[]string{"InvalidCcipSenderAta"},
			common.AddComputeUnitLimit(computebudget.MAX_COMPUTE_UNIT_LIMIT))
	})

	t.Run("rejects a fee billing signer that is not the router's", func(t *testing.T) {
		ix, resolved := buildSend(t, config.EvmChainSelector, solana.PublicKey{}, solana.WrappedSol,
			nil, solana.PublicKey{})

		generic, ok := ix.(*solana.GenericInstruction)
		require.True(t, ok)
		generic.AccountValues[senderFeeBillingSignerAccount] = solana.Meta(solana.NewWallet().PublicKey())

		testutils.SendAndFailWithLookupTables(ctx, t, env.Client, []solana.Instruction{ix}, user,
			config.DefaultCommitment, sendLookupTables(resolved),
			[]string{"Error Code: " + common.ConstraintSeeds_AnchorError.String()},
			common.AddComputeUnitLimit(computebudget.MAX_COMPUTE_UNIT_LIMIT))
	})

	// The router bills the quoted fee regardless of max_fee, so a native send whose max_fee does not
	// cover it can only be billed out of the PDA's own balance.
	resolveNativeSendWithoutDeposit := func(t *testing.T) ccip.ResolveAccountsResult {
		resolved, rErr := ccip.ResolveExampleSenderV2SendAccounts(
			ctx, env.Client, program, config.CcipRouterProgram,
			sender.CcipSendParams{
				DestChainSelector: config.EvmChainSelector,
				Data:              []byte{1, 2, 3},
				FeeToken:          solana.PublicKey{},
				MaxFee:            0,
			},
			solana.WrappedSol, solana.PublicKey{}, user.PublicKey(), logger.Test(t))
		require.NoError(t, rErr)
		return resolved
	}

	t.Run("rejects a native fee that max_fee does not cover", func(t *testing.T) {
		// With the PDA unfunded, the router's own transfer fails before the sender's check runs.
		resolved := resolveNativeSendWithoutDeposit(t)
		testutils.SendAndFailWithLookupTables(ctx, t, env.Client, []solana.Instruction{resolved.Ix}, user,
			config.DefaultCommitment, sendLookupTables(resolved), []string{"insufficient lamports"},
			common.AddComputeUnitLimit(computebudget.MAX_COMPUTE_UNIT_LIMIT))
		require.Zero(t, lamportBalance(senderPDA))
	})

	t.Run("rejects a native fee above max_fee without spending the PDA's own balance", func(t *testing.T) {
		fundIx, fErr := system.NewTransferInstruction(
			senderPDAFunding, user.PublicKey(), senderPDA).ValidateAndBuild()
		require.NoError(t, fErr)
		testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{fundIx}, user, config.DefaultCommitment)

		resolved := resolveNativeSendWithoutDeposit(t)
		testutils.SendAndFailWithLookupTables(ctx, t, env.Client, []solana.Instruction{resolved.Ix}, user,
			config.DefaultCommitment, sendLookupTables(resolved), []string{"MaxFeeExceeded"},
			common.AddComputeUnitLimit(computebudget.MAX_COMPUTE_UNIT_LIMIT))
		require.Equal(t, uint64(senderPDAFunding), lamportBalance(senderPDA))
	})

	t.Run("rejects a token fee that max_fee does not cover", func(t *testing.T) {
		// Only max_fee is deposited into the sender PDA's fee account and delegated to the router,
		// so its pull of the full fee fails in the token program.
		resolved, rErr := ccip.ResolveExampleSenderV2SendAccounts(
			ctx, env.Client, program, config.CcipRouterProgram,
			sender.CcipSendParams{
				DestChainSelector: config.EvmChainSelector,
				Data:              []byte{1, 2, 3},
				FeeToken:          env.Link22.Mint,
				MaxFee:            1,
			},
			env.Link22.Mint, solana.PublicKey{}, user.PublicKey(), logger.Test(t))
		require.NoError(t, rErr)

		approve, aErr := tokens.TokenApproveChecked(1, 9, env.Link22.Program,
			env.Link22.UserATA, env.Link22.Mint, senderPDA, user.PublicKey(), nil)
		require.NoError(t, aErr)
		callerBalanceBefore := env.GetBalance(env.Link22.UserATA)

		testutils.SendAndFailWithLookupTables(ctx, t, env.Client, []solana.Instruction{approve, resolved.Ix}, user,
			config.DefaultCommitment, sendLookupTables(resolved), []string{"insufficient funds"},
			common.AddComputeUnitLimit(computebudget.MAX_COMPUTE_UNIT_LIMIT))
		require.Equal(t, callerBalanceBefore, env.GetBalance(env.Link22.UserATA))
	})

	t.Run("rejects a send pointing at a different router", func(t *testing.T) {
		ix, resolved := buildSend(t, config.EvmChainSelector, solana.PublicKey{}, solana.WrappedSol,
			nil, solana.PublicKey{})

		generic, ok := ix.(*solana.GenericInstruction)
		require.True(t, ok)
		// ccip_send names the router it CPIs into, rather than picking it out of the forwarded
		// list, and pins it to the one in its state account.
		require.Equal(t, config.CcipRouterProgram, generic.AccountValues[senderRouterAccount].PublicKey)
		generic.AccountValues[senderRouterAccount] = solana.Meta(config.CcipOfframpProgram)

		testutils.SendAndFailWithLookupTables(ctx, t, env.Client, []solana.Instruction{ix}, user,
			config.DefaultCommitment, sendLookupTables(resolved), []string{"InvalidRouter"},
			common.AddComputeUnitLimit(computebudget.MAX_COMPUTE_UNIT_LIMIT))
	})
}

// senderNamedAccounts is how many accounts ccip_send's own context declares - see CcipSend in the
// program's context.rs - so it is the index at which its remaining accounts begin.
const senderNamedAccounts = 13

// senderRouterAccount is the position of the router within those named accounts.
const senderRouterAccount = 5

// senderFeeTokenAccount and senderFeeBillingSignerAccount are the positions of the sender PDA's
// fee token account and the router's billing signer within those named accounts.
const (
	senderFeeTokenAccount         = 8
	senderFeeBillingSignerAccount = 9
)

// logUsage reports the compute units and account-lock count of a send, the two budgets a CPI
// wrapper around ccip_send_v2 has the least headroom on.
//
// Locks are distinct keys, not metas: ccip_send names several accounts that also appear in the
// router list it forwards, and the runtime merges those duplicates into a single lock.
func logUsage(t *testing.T, name string, result *rpc.GetTransactionResult, ix solana.Instruction) {
	t.Helper()
	var cu uint64
	if result.Meta != nil && result.Meta.ComputeUnitsConsumed != nil {
		cu = *result.Meta.ComputeUnitsConsumed
	}
	locked := make(map[solana.PublicKey]struct{}, len(ix.Accounts()))
	for _, meta := range ix.Accounts() {
		locked[meta.PublicKey] = struct{}{}
	}
	t.Logf("%s: %d CU, %d accounts locked (%d metas)", name, cu, len(locked), len(ix.Accounts()))
}
