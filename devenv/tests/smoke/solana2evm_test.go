package smoke

import (
	"bytes"
	"context"
	"math/big"
	"testing"
	"time"

	semver "github.com/Masterminds/semver/v3"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	gethrpc "github.com/ethereum/go-ethereum/rpc"
	"github.com/gagliardetto/solana-go"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	chainsel "github.com/smartcontractkit/chain-selectors"
	"github.com/smartcontractkit/chainlink-ccip/deployment/tokens"
	ccv "github.com/smartcontractkit/chainlink-ccv/build/devenv"
	"github.com/smartcontractkit/chainlink-ccv/build/devenv/cciptestinterfaces"
	"github.com/smartcontractkit/chainlink-ccv/build/devenv/chainreg"
	devenvcommon "github.com/smartcontractkit/chainlink-ccv/build/devenv/common"
	_ "github.com/smartcontractkit/chainlink-ccv/build/devenv/evm" // register EVM
	e2ehelpers "github.com/smartcontractkit/chainlink-ccv/build/devenv/tests/e2e"
	"github.com/smartcontractkit/chainlink-ccv/build/devenv/tests/e2e/tcapi"
	"github.com/smartcontractkit/chainlink-ccv/build/devenv/tests/e2e/tcapi/basic"
	"github.com/smartcontractkit/chainlink-ccv/build/devenv/tests/e2e/tcapi/token_transfer"
	"github.com/smartcontractkit/chainlink-ccv/protocol"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/base_token_pool"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/example_ccip_sender_v2"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	soltokens "github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/tokens"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	testsenderops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/testsender"
	tokenpoolops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/tokenpools"
	soldevenv "github.com/smartcontractkit/chainlink-ccip-solana/devenv"
	"github.com/smartcontractkit/chainlink-ccip-solana/devenv/ccvregistry"
	devenvutils "github.com/smartcontractkit/chainlink-ccip-solana/devenv/tests/utils"
)

// exampleSenderMaxFee is the native fee bound, in lamports, the example sender cases allow the
// router to bill. Well above a plain message's fee and well below the payer's balance.
const exampleSenderMaxFee = 100_000_000

// tokenSendPaths are the two ways the Lombard case leaves Solana: straight into the router, and
// through the example sender v2. The sender puts one more CPI level in front of the router
// (sender -> router -> pool -> ...), which is what can exceed Solana's CPI depth limit on deep
// token paths, so Lombard runs both ways. CCTP does not fit through the sender at all; see the
// CCTP case.
var tokenSendPaths = []struct {
	name      string
	viaSender bool
}{
	{"Router", false},
	{"ExampleSenderV2", true},
}

// sendTokenViaExampleSenderV2 sends a token-only message through the example sender v2, to the
// recipient and with the extra args the environment configured for dest.
func sendTokenViaExampleSenderV2(
	ctx context.Context,
	t *testing.T,
	chain *soldevenv.Chain,
	dest uint64,
	mint solana.PublicKey,
	amount *big.Int,
) (cciptestinterfaces.MessageSentEvent, protocol.ByteSlice) {
	t.Helper()
	require.True(t, amount.IsUint64(), "transfer amount %s does not fit in a u64", amount)
	sentEvt, sig, err := chain.SendViaTestSenderV2(ctx, dest, nil,
		&example_ccip_sender_v2.SvmTokenAmount{Token: mint, Amount: amount.Uint64()}, exampleSenderMaxFee)
	require.NoError(t, err)
	return sentEvt, sig[:]
}

type evmCallTrace struct {
	To    *common.Address `json:"to"`
	Input hexutil.Bytes   `json:"input"`
	Calls []evmCallTrace  `json:"calls"`
}

func findEVMCall(trace *evmCallTrace, receiver common.Address, selector []byte) *evmCallTrace {
	if trace.To != nil && *trace.To == receiver && bytes.HasPrefix(trace.Input, selector) {
		return trace
	}
	for i := range trace.Calls {
		if call := findEVMCall(&trace.Calls[i], receiver, selector); call != nil {
			return call
		}
	}
	return nil
}

func requireEVMReceiverCall(
	ctx context.Context,
	t *testing.T,
	rpcClient *gethrpc.Client,
	txID protocol.ByteSlice,
	receiver protocol.UnknownAddress,
) {
	t.Helper()

	var trace evmCallTrace
	err := rpcClient.CallContext(ctx, &trace, "debug_traceTransaction", common.BytesToHash(txID), map[string]any{
		"tracer": "callTracer",
	})
	require.NoError(t, err, "trace EVM execution transaction")

	ccipReceiveSelector := crypto.Keccak256(
		[]byte("ccipReceive((bytes32,uint64,bytes,bytes,(address,uint256)[]))"),
	)[:4]
	receiverCall := findEVMCall(&trace, common.BytesToAddress(receiver), ccipReceiveSelector)
	require.NotNil(t, receiverCall,
		"execution transaction should invoke ccipReceive on the configured receiver")
}

// TestSolana2EVM sends CCIP messages from the Solana chain to the EVM chain
// and verifies they are aggregated, indexed, and executed on the EVM offramp.
//
// Subtests:
//   - ArbMessaging: tcapi basic.EOAReceiverDefaultVerifier from Solana to EVM.
//   - TokenTransfer: tcapi send/assert/confirm with source-side pool event assertions.
//
// Expects the devenv to already be running:
//
//	make start-devenv
//
// Local output: env-phased-out.toml. Staging attach: env-private-staging.deployed.toml.
func TestSolana2EVM(t *testing.T) {
	factory := soldevenv.NewImplFactory()
	err := chainreg.Register(chainsel.FamilySolana, chainreg.Registration{
		ImplFactory:     factory,
		ExecutorInfo:    factory,
		CLDFProvider:    ccvregistry.SolanaCLDFProviderFactory,
		AddressResolver: soldevenv.NewAddressResolver(),
	})
	require.NoError(t, err)

	ctx := ccv.Plog.WithContext(t.Context())
	lib, err := ccv.NewLibFromCCVEnv(zerolog.Ctx(ctx), e2ehelpers.GetSmokeTestConfig())
	require.NoError(t, err)

	ds, err := lib.DataStore()
	require.NoError(t, err)

	chains, err := lib.ChainsMap(ctx)
	require.NoError(t, err)
	cldfEnv, err := lib.CLDFEnvironment()
	require.NoError(t, err)

	cfg, cfgErr := smokeConfig()
	require.NoError(t, cfgErr)
	solSelector, evmSelector, laneErr := devenvutils.ResolveSolanaEVMLanes(cfg.Blockchains)
	require.NoError(t, laneErr)
	evmImpl, ok := chains[evmSelector]
	require.True(t, ok, "EVM chain (selector %d) not found in output config", evmSelector)
	evmCLDFChain, ok := cldfEnv.BlockChains.EVMChains()[evmSelector]
	require.True(t, ok, "EVM chain (selector %d) not found in CLDF environment", evmSelector)
	evmName, nameErr := chainsel.GetChainNameFromSelector(evmSelector)
	require.NoError(t, nameErr)
	t.Logf("EVM chain: selector %d (%s)", evmSelector, evmName)
	evmRPC := evmRPCClient(t, evmCLDFChain.Client)

	solanaChain, ok := chains[solSelector]
	require.True(t, ok, "Solana chain (selector %d) not found in output config", solSelector)
	solanaImpl, ok := solanaChain.(*soldevenv.Chain)
	require.True(t, ok, "Solana chain should be backed by *soldevenv.Chain")
	t.Cleanup(func() {
		if closer, canClose := solanaChain.(interface{ Close() error }); canClose {
			_ = closer.Close()
		}
	})

	// target token transfer amounts: minimum that exercises the decimals path
	sourceAmount := new(big.Int).Set(minSolanaTransferAmount)
	destinationAmount := new(big.Int).Set(minEVMTransferAmount)

	payerAddr, err := solanaImpl.GetSenderAddress()
	require.NoError(t, err, "resolve default Solana sender/payer address")
	ccvProgramID, _, err := solanaImpl.DefaultCCVAndExecutorPubKeys()
	require.NoError(t, err)
	ccvVaultPDA, _, err := state.FindFeeVaultPDA(ccvProgramID, nil)
	require.NoError(t, err)
	ccvVaultOwnerAddr := protocol.UnknownAddress(ccvVaultPDA.Bytes())
	feeMintAddr := protocol.UnknownAddress(solana.WrappedSol.Bytes())

	t.Run("ArbMessaging", func(t *testing.T) {
		payerBefore, err := solanaImpl.NativeBalance(ctx, payerAddr)
		require.NoError(t, err)
		ccvVaultBefore, err := solanaImpl.GetTokenBalance(ctx, ccvVaultOwnerAddr, feeMintAddr)
		require.NoError(t, err)

		tc := basic.EOAReceiverDefaultVerifier(lib, solSelector, evmSelector, basic.Args{
			Send: tcapi.SendArgs{
				SendOption: devenvutils.MustSolSendOptions(t),
			},
			Run: tcapi.RunConfig{
				ConfirmSentTimeout: defaultAssertionTimeout,
				ConfirmExecTimeout: execTimeout(solSelector),
			},
		})
		result, err := tc.Run(ctx)
		require.NoError(t, err)

		payerAfter, err := solanaImpl.NativeBalance(ctx, payerAddr)
		require.NoError(t, err)
		require.Equal(t, -1, payerAfter.Cmp(payerBefore), "arb message send should charge a non-zero fee")

		sendEvt, err := solanaImpl.ConfirmSendEventInTx(ctx, solana.SignatureFromBytes(result.Src.TxID))
		require.NoError(t, err)
		var ccvReceiptFee uint64
		for _, r := range sendEvt.Receipts {
			if r.Issuer == ccvProgramID {
				ccvReceiptFee = r.FeeTokenAmount
			}
		}

		ccvVaultAfter, err := solanaImpl.GetTokenBalance(ctx, ccvVaultOwnerAddr, feeMintAddr)
		require.NoError(t, err)
		wantCCVVaultAfter := new(big.Int).Add(ccvVaultBefore, new(big.Int).SetUint64(ccvReceiptFee))
		require.Equal(t, wantCCVVaultAfter, ccvVaultAfter,
			"committee verifier's fee vault should increase by exactly its receipt's fee")
	})

	// The message is sent by the example sender v2 program rather than the payer: it CPIs into the
	// router's ccip_send_v2 with its ccip_sender PDA as the CCIP sender, and the payer funds the
	// native SOL fee through that PDA. The sender's EVM config is re-pointed at the mock receiver
	// contract so execution must call ccipReceive.
	t.Run("ExampleSenderV2", func(t *testing.T) {
		evmReg, err := chainreg.GetRegistry().Get(chainsel.FamilyEVM)
		require.NoError(t, err)
		receiver, err := evmReg.AddressResolver.GetContractReceiver(ds, evmSelector, devenvcommon.DefaultReceiverQualifier)
		require.NoError(t, err)

		sender, err := solanaImpl.TestSenderV2Address()
		require.NoError(t, err)
		ccipSenderPDA, err := testsenderops.FindCcipSenderPDA(sender)
		require.NoError(t, err)
		pdaBefore, err := solanaImpl.NativeBalance(ctx, protocol.UnknownAddress(ccipSenderPDA.Bytes()))
		require.NoError(t, err)

		sentEvt, _, err := solanaImpl.SendViaTestSenderV2(ctx, evmSelector, []byte("example sender v2 sol2evm"), nil, exampleSenderMaxFee)
		require.NoError(t, err)
		require.Equal(t, protocol.UnknownAddress(ccipSenderPDA.Bytes()), sentEvt.Sender,
			"the CCIP sender should be the example sender's ccip_sender PDA, not the payer")

		pdaAfter, err := solanaImpl.NativeBalance(ctx, protocol.UnknownAddress(ccipSenderPDA.Bytes()))
		require.NoError(t, err)
		require.Zero(t, pdaBefore.Cmp(pdaAfter), "the native fee must not come out of the ccip_sender PDA's own balance")

		messageKey := cciptestinterfaces.MessageEventKey{MessageID: sentEvt.MessageID}
		v3Src, err := lib.V3Source(ctx, solSelector)
		require.NoError(t, err)
		v3Dest, err := lib.V3Destination(ctx, evmSelector)
		require.NoError(t, err)
		_, err = v3Src.ConfirmSendOnSource(ctx, evmSelector, messageKey, defaultAssertionTimeout)
		require.NoError(t, err)

		aggregatorClient, indexerMonitor, err := tcapi.SetupOffchainClients(lib, "")
		require.NoError(t, err)
		testCtx, cleanup := tcapi.NewTestingContext(ctx, aggregatorClient, indexerMonitor)
		defer cleanup()
		_, err = testCtx.AssertMessage(sentEvt.MessageID, tcapi.AssertMessageOptions{
			TickInterval:            time.Second,
			Timeout:                 defaultAssertionTimeout,
			ExpectedVerifierResults: 1,
		})
		require.NoError(t, err)

		execEnv, err := v3Dest.ConfirmExecOnDest(ctx, solSelector, messageKey, execTimeout(solSelector))
		require.NoError(t, err)
		require.Equal(t, cciptestinterfaces.ExecutionStateSuccess, execEnv.Event.State, "exec should succeed end-to-end")
		requireEVMReceiverCall(ctx, t, evmRPC, execEnv.TxID, receiver)
	})

	// The v1 example sender wraps the router's 1.6 ccip_send. The CCIP 2.0 offchain stack does not
	// deliver legacy messages, so this only covers the source side: the send goes through and the
	// router records the sender's ccip_sender PDA as the CCIP sender.
	//
	// TODO: extend into end-to-end coverage once devenv can test lane upgrades from 1.6 to 2.0, which
	// needs a 1.6 offchain stack delivering legacy messages alongside the 2.0 one.
	t.Run("ExampleSenderV1", func(t *testing.T) {
		laneExists, err := solanaImpl.LegacyDestChainExists(ctx, evmSelector)
		require.NoError(t, err)
		if !laneExists {
			t.Skipf("router has no 1.6 lane to chain %d in this environment", evmSelector)
		}

		sender, err := solanaImpl.TestSenderV1Address()
		require.NoError(t, err)
		ccipSenderPDA, err := testsenderops.FindCcipSenderPDA(sender)
		require.NoError(t, err)

		sentEvt, _, err := solanaImpl.SendViaTestSenderV1(ctx, evmSelector, []byte("example sender v1 sol2evm"))
		require.NoError(t, err)
		require.Equal(t, evmSelector, sentEvt.DestChainSelector)
		require.Equal(t, ccipSenderPDA, sentEvt.Message.Sender,
			"the CCIP sender should be the example sender's ccip_sender PDA, not the payer")
		require.NotEqual(t, [32]uint8{}, sentEvt.Message.Header.MessageId)
	})

	for _, tc := range tokenPoolTestCases {
		t.Run("TokenTransfer/"+tc.caseName, func(t *testing.T) {
			caseName := tc.caseName
			combo, err := tokenCombination(tc.solana, tc.evm, declaredCombos(t, ds, solSelector, evmSelector))
			require.NoError(t, err, "pool pair %q not deployed in this environment", caseName)

			payerBefore, err := solanaImpl.NativeBalance(ctx, payerAddr)
			require.NoError(t, err)
			ccvVaultBefore, err := solanaImpl.GetTokenBalance(ctx, ccvVaultOwnerAddr, feeMintAddr)
			require.NoError(t, err)

			// get mint address of the Solana source pool
			sourcePool := combo.LocalPoolAddressRef()
			mintBytes, err := devenvutils.GetSolanaTokenAddress(ds, solSelector, sourcePool.Qualifier)
			require.NoError(t, err, "Solana token address not found for qualifier %q", sourcePool.Qualifier)
			mint := solana.PublicKeyFromBytes(mintBytes)
			mintAddr := protocol.UnknownAddress(mint.Bytes())

			poolProgram, poolVersion, err := solcommon.GetSolanaTokenPoolProgramAddress(
				ds,
				solSelector,
				string(sourcePool.Type),
				sourcePool.Qualifier,
			)
			require.NoError(t, err)
			require.NotNil(t, poolVersion, "resolved Solana pool version is nil")
			require.Equal(t, sourcePool.Version.String(), poolVersion.String(), "resolved Solana pool version mismatch")
			isV2Pool := poolVersion.GreaterThanEqual(solcommon.Version2_0_0)

			transferAmount := sourceAmount
			destBalanceIncrease := destinationAmount
			var bpsFeeAmount uint64
			var poolVaultOwnerAddr, poolTokenOwnerAddr protocol.UnknownAddress
			var poolVaultBefore, poolInTokenVaultBefore, poolTokenBefore *big.Int
			if isV2Pool {
				finalityBps, bpsErr := getPoolFinalityBpsFee(ctx, solanaImpl.RPCClient(), poolProgram, evmSelector, mint)
				require.NoError(t, bpsErr)
				bpsFeeAmount = sourceAmount.Uint64() * uint64(finalityBps) / 10_000
				// the BPS fee is charged on the source SVM amount, integer-truncated; the EVM
				// destination receives the post-fee SVM amount, re-scaled to EVM base units
				postFeeSVM := new(big.Int).Sub(sourceAmount, new(big.Int).SetUint64(bpsFeeAmount))
				destBalanceIncrease = new(big.Int).Div(
					new(big.Int).Mul(postFeeSVM, destinationAmount),
					sourceAmount,
				)

				poolVaultPDA, _, pdaErr := state.FindFeeVaultPDA(poolProgram, mint.Bytes())
				require.NoError(t, pdaErr)
				poolVaultOwnerAddr = protocol.UnknownAddress(poolVaultPDA.Bytes())
				poolVaultBefore, err = solanaImpl.GetTokenBalance(ctx, poolVaultOwnerAddr, feeMintAddr)
				require.NoError(t, err)

				// The same fee vault also holds the in-token BPS fee, in its ATA for the pool's
				// own mint rather than the fee token's.
				poolInTokenVaultBefore, err = solanaImpl.GetTokenBalance(ctx, poolVaultOwnerAddr, mintAddr)
				require.NoError(t, err)

				poolSignerPDA, signerErr := soltokens.TokenPoolSignerAddress(mint, poolProgram)
				require.NoError(t, signerErr)
				poolTokenOwnerAddr = protocol.UnknownAddress(poolSignerPDA.Bytes())
				poolTokenBefore, err = solanaImpl.GetTokenBalance(ctx, poolTokenOwnerAddr, mintAddr)
				require.NoError(t, err)
			}

			require.NoError(t, solSenderTokenFundingError(ctx, solanaImpl, mint))

			// define the token transfer test case; finality must stay 0 until devenv
			// configures the pools' allowed finality, v2 Solana source pools reject fast
			// finality in GetFee with InvalidRequestedFinality (6029)
			tc := token_transfer.TokenTransfer(
				lib,
				solSelector,
				evmSelector,
				combo,
				protocol.Finality(0),
				true, // useEOAReceiver
				tc.caseName,
				token_transfer.Args{
					Send: tcapi.SendArgs{
						SendOption: devenvutils.MustSolSendOptions(t),
					},
					Run: tcapi.RunConfig{
						ConfirmSentTimeout: defaultAssertionTimeout,
						ConfirmExecTimeout: execTimeout(solSelector),
					},
					TransferAmount:      transferAmount,
					DestBalanceIncrease: destBalanceIncrease,
				},
			)

			if !tc.HavePrerequisites(ctx) {
				t.Skipf("pool pair %q prerequisites not met in this environment", caseName)
			}

			// run the test case and get the result for additional assertions
			result, err := tc.Run(ctx)
			require.NoError(t, err)
			require.Len(t, result.Src.TxID, solana.SignatureLength, "source receipt should contain a Solana signature")
			require.Len(t, result.Dest.TxID, 32, "destination receipt should contain an EVM transaction hash")

			// Check fees
			srcSig := solana.SignatureFromBytes(result.Src.TxID)
			sendEvt, err := solanaImpl.ConfirmSendEventInTx(ctx, srcSig)
			require.NoError(t, err)
			require.Equal(t, solana.WrappedSol, sendEvt.FeeToken, "fee should be charged in wrapped SOL")
			gotTokenAmount := new(big.Int).SetBytes(sendEvt.TokenAmountBeforeTokenPoolFees.BeBytes[:])
			require.Zero(t, gotTokenAmount.Cmp(sourceAmount),
				"send event should record the transferred token amount, got %s want %s", gotTokenAmount, sourceAmount)

			var totalFee uint64
			var ccvReceiptFee uint64
			var foundCCVReceipt bool
			for _, r := range sendEvt.Receipts {
				totalFee += r.FeeTokenAmount
				if r.Issuer == ccvProgramID {
					ccvReceiptFee = r.FeeTokenAmount
					foundCCVReceipt = true
				}
			}
			require.True(t, foundCCVReceipt, "send event should include a receipt issued by the default committee verifier")
			require.Positive(t, totalFee, "token transfer send should incur a non-zero fee")

			// The payer pays totalFee (in lamports, 1:1 with wrapped SOL) plus its own tx/gas cost,
			// so its balance must drop by at least totalFee.
			payerAfter, err := solanaImpl.NativeBalance(ctx, payerAddr)
			require.NoError(t, err)
			paidByPayer := new(big.Int).Sub(payerBefore, payerAfter)
			require.GreaterOrEqual(t, paidByPayer.Uint64(), totalFee,
				"payer's native SOL balance should drop by at least the fee charged in the send event")

			// The committee verifier's fee vault is paid out exactly its own receipt's fee.
			ccvVaultAfter, err := solanaImpl.GetTokenBalance(ctx, ccvVaultOwnerAddr, feeMintAddr)
			require.NoError(t, err)
			wantCCVVaultAfter := new(big.Int).Add(ccvVaultBefore, new(big.Int).SetUint64(ccvReceiptFee))
			require.Equal(t, wantCCVVaultAfter, ccvVaultAfter,
				"committee verifier's fee vault should increase by exactly its receipt's fee")

			if isV2Pool {
				var poolReceiptFee uint64
				var foundPoolReceipt bool
				for _, r := range sendEvt.Receipts {
					if r.Issuer == poolProgram {
						poolReceiptFee = r.FeeTokenAmount
						foundPoolReceipt = true
					}
				}
				require.True(t, foundPoolReceipt, "send event should include a receipt issued by the token pool")

				poolVaultAfter, err2 := solanaImpl.GetTokenBalance(ctx, poolVaultOwnerAddr, feeMintAddr)
				require.NoError(t, err2)
				wantPoolVaultAfter := new(big.Int).Add(poolVaultBefore, new(big.Int).SetUint64(poolReceiptFee))
				require.Equal(t, wantPoolVaultAfter, poolVaultAfter,
					"token pool's fee vault should increase by exactly its receipt's flat fee")

				// BPS fee: deducted in-token at lock/burn time, so it never appears in any
				// receipt. The pool moves it into its fee vault's ATA for its own mint (the only
				// account withdraw_fee_tokens can reach), so it lands there for either pool type.
				poolInTokenVaultAfter, errV := solanaImpl.GetTokenBalance(ctx, poolVaultOwnerAddr, mintAddr)
				require.NoError(t, errV)
				wantInTokenVaultAfter := new(big.Int).Add(poolInTokenVaultBefore, new(big.Int).SetUint64(bpsFeeAmount))
				require.Equal(t, wantInTokenVaultAfter, poolInTokenVaultAfter,
					"token pool's fee vault should increase by exactly the in-token BPS fee")

				// What stays in the pool's own token account is only what it custodies: nothing
				// for BurnMint (the post-fee principal is burned), the post-fee principal for
				// LockRelease (it has no burn step, and the fee has been moved to the vault).
				poolTokenIncrease := uint64(0)
				if string(sourcePool.Type) == devenvcommon.LockReleaseTokenPoolType {
					poolTokenIncrease = sourceAmount.Uint64() - bpsFeeAmount
				}
				poolTokenAfter, err3 := solanaImpl.GetTokenBalance(ctx, poolTokenOwnerAddr, mintAddr)
				require.NoError(t, err3)
				wantPoolTokenAfter := new(big.Int).Add(poolTokenBefore, new(big.Int).SetUint64(poolTokenIncrease))
				require.Equal(t, wantPoolTokenAfter, poolTokenAfter,
					"token pool should retain exactly the principal it custodies for this transfer")
			}

			// Run already fails unless execution succeeded; assert only the fields it
			// does not check: message identity, source selector, message number, and that a
			// token-only transfer carries no receiver return data.
			execEvt := result.Dest.Event
			require.EqualValues(t, result.Src.Event.MessageID, execEvt.MessageID, "execution event should match the sent message ID")
			require.Equal(t, protocol.ChainSelector(solSelector), execEvt.SourceChainSelector,
				"execution event should retain the source chain selector")
			require.NotZero(t, execEvt.MessageNumber, "execution event should expose a non-zero message number")
			require.Empty(t, execEvt.ReturnData, "token-only transfer should not return receiver data")

			// retrieve the lock/burn event from the Solana send tx
			discriminator, err := expectedSourcePoolEventDiscriminator(string(sourcePool.Type), poolVersion)
			require.NoError(t, err)
			poolEvt, err := solanaImpl.ConfirmTokenPoolEventInTx(
				ctx,
				solana.SignatureFromBytes(result.Src.TxID),
				poolProgram,
				discriminator,
			)
			require.NoError(t, err)

			// normalize the event and assert its fields
			normalized, err := NormalizeTokenPoolEvent(poolEvt)
			require.NoError(t, err)
			require.Equal(t, mint, normalized.Mint, "source pool event mint mismatch")
			// v2 pools lock/burn only the post-BPS-fee amount; 1.6 pools always transfer 1:1.
			wantLockedAmount := sourceAmount.Uint64() - bpsFeeAmount
			require.Equal(t, wantLockedAmount, normalized.Amount, "source pool event amount mismatch")
			if isV2Pool {
				require.NotNil(t, normalized.RemoteChainSelector, "v2 pool event should expose the remote chain selector")
				require.Equal(t, evmSelector, *normalized.RemoteChainSelector,
					"source pool event should retain the destination chain selector")
			}
		})
	}

	// TokenTransfer/Lombard exercises the Lombard custom-CCV token pool in the Solana->EVM direction.
	//
	// On devenv, EVM's MockLombardMailbox never actually mints on receipt (see chainlink-ccv's own
	// runLombardTestCase FIXME in smoke_token_verification_test.go), so this test only asserts
	// the source-side balance decrease and lock/burn event, not a destination balance increase.
	// On a deployed env Lombard's real attestation infrastructure serves the verifier (gastald
	// attests, the token-verifier verifies, the EVM offramp executes), and nothing registers
	// an attestation here.
	t.Run("TokenTransfer/Lombard", func(t *testing.T) {
		fakeURL := fakeAttestationURL(t)

		// TODO: find out why the receiver is pinned; keep or remove
		// Pinned receiver: the lane may stall before Lombard configures the EVM side;
		// stuck dust must land in a key we hold.
		receiver := common.HexToAddress("0xe26641a49ff4f841b327714f5ec28cc5f62c6522")
		evmReg, err := chainreg.GetRegistry().Get(chainsel.FamilyEVM)
		require.NoError(t, err)
		senderRecipient, err := evmReg.AddressResolver.GetContractReceiver(ds, evmSelector, devenvcommon.DefaultReceiverQualifier)
		require.NoError(t, err)

		_, _, executor, err := tcapi.ResolveV3SendAddresses(ctx, lib, solSelector, evmSelector)
		require.NoError(t, err)

		v3Src, err := lib.V3Source(ctx, solSelector)
		require.NoError(t, err)
		v3Dest, err := lib.V3Destination(ctx, evmSelector)
		require.NoError(t, err)

		lombardTokens := []lombardToken{
			{mintType: string(solcommon.SPLTokens), mintQualifier: "Lombard_LBTC", evmPoolQualifier: "Lombard_LBTC", sendAmount: lombardTransferAmount},
			{mintType: string(solcommon.SPL2022Tokens), mintQualifier: "Lombard_BTCb", evmPoolQualifier: "Lombard_BTCb", sendAmount: lombardTransferAmount},
		}
		if fakeURL != "" {
			// Not the shared amount: the devenv mock mint uses
			// soldevenv.LombardSolanaMintDecimals (9), not the generic
			// soldevenv.SolanaDefaultDecimals (6), and only the LBTC token pair is deployed.
			lombardTokens = []lombardToken{
				{mintType: string(solcommon.SPLTokens), mintQualifier: "LBTC", evmPoolQualifier: "Lombard_LBTC",
					sendAmount: tokens.ScaleTokenAmount(big.NewInt(1), soldevenv.LombardSolanaMintDecimals)},
			}
		}

		for _, lt := range lombardTokens {
			t.Run(lt.mintQualifier, func(t *testing.T) {
				for _, path := range tokenSendPaths {
					t.Run(path.name, func(t *testing.T) {
						receiver := receiver
						if path.viaSender {
							receiver = common.BytesToAddress(senderRecipient)
						}
						mintRef, err := solcommon.PoolRefByQualifier(ds, solSelector, lt.mintType, lt.mintQualifier)
						require.NoError(t, err, "resolve Solana lombard mint")
						mint, err := solana.PublicKeyFromBase58(mintRef.Address)
						require.NoError(t, err)
						mintAddr := protocol.UnknownAddress(mint.Bytes())

						poolRef, err := ds.Addresses().Get(datastore.NewAddressRefKey(
							solSelector, datastore.ContractType(tokenpoolops.LombardTokenPool),
							solcommon.Version2_0_0, solcommon.PoolProgramQualifier,
						))
						require.NoError(t, err, "resolve Solana lombard pool program")
						poolProgram, err := solana.PublicKeyFromBase58(poolRef.Address)
						require.NoError(t, err)

						evmPoolRefs := ds.Addresses().Filter(
							datastore.AddressRefByChainSelector(evmSelector),
							datastore.AddressRefByType(datastore.ContractType(tokenpoolops.LombardTokenPool)),
							datastore.AddressRefByQualifier(lt.evmPoolQualifier),
						)
						require.Lenf(t, evmPoolRefs, 1, "expected one EVM lombard pool for qualifier %s", lt.evmPoolQualifier)
						evmPool := common.HexToAddress(evmPoolRefs[0].Address)
						destToken := protocol.UnknownAddress(resolveLombardEVMPoolToken(ctx, t, evmRPC, evmPool).Bytes())
						t.Logf("EVM pool %s token %s", evmPool, common.BytesToAddress(destToken.Bytes()))

						// BTCb is a Token-2022 mint; the shared TokenBalanceReader derives only
						// classic ATAs, so read the balance directly under the mint's owner program.
						payerKey := solana.PublicKeyFromBytes(payerAddr.Bytes())
						sourceStartBalance := lombardSolanaTokenBalance(ctx, t, solanaImpl.RPCClient(), payerKey, mint)
						require.GreaterOrEqual(t, sourceStartBalance.Cmp(lt.sendAmount), 0,
							"sender %s holds %s of %s base units, needs %s", payerAddr, sourceStartBalance, lt.mintQualifier, lt.sendAmount)

						var destStartBalance *big.Int
						if fakeURL == "" {
							destStartBalance, err = evmImpl.GetTokenBalance(ctx, protocol.UnknownAddress(receiver.Bytes()), destToken)
							require.NoError(t, err)
						}

						// The Lombard mailbox attestation verify needs more gas than the tcapi default; 1M
						// covers the full EVM execution path with headroom.
						const lombardExecGasLimit = 1_000_000
						var sentEvt cciptestinterfaces.MessageSentEvent
						var srcTxHash protocol.ByteSlice
						if path.viaSender {
							sentEvt, srcTxHash = sendTokenViaExampleSenderV2(ctx, t, solanaImpl, evmSelector, mint, lt.sendAmount)
						} else {
							sentEvt, srcTxHash, err = tcapi.SendV3Message(ctx, v3Src, v3Dest,
								cciptestinterfaces.MessageFields{
									Receiver: protocol.UnknownAddress(receiver.Bytes()),
									TokenAmount: cciptestinterfaces.TokenAmount{
										TokenAddress: mintAddr,
										Amount:       lt.sendAmount,
									},
								},
								cciptestinterfaces.MessageOptions{
									FinalityConfig: protocol.Finality(0),
									Executor:       executor,
								},
								tcapi.SendArgs{
									ExecutionGasLimit: lombardExecGasLimit,
									SendOption:        devenvutils.MustSolSendOptions(t),
								},
							)
							require.NoError(t, err)
						}
						require.Len(t, srcTxHash, solana.SignatureLength, "source receipt should contain a Solana signature")
						t.Logf("burn signature: %s  messageID: %x", solana.SignatureFromBytes(srcTxHash), [32]byte(sentEvt.MessageID))

						var messageHash protocol.ByteSlice
						issuedByPool := false
						for i, issuer := range sentEvt.ReceiptIssuers {
							// ReceiptIssuers (per fee payee) and VerifierBlobs (per CCV) are not parallel.
							if issuer.Equal(protocol.UnknownAddress(poolProgram.Bytes())) {
								issuedByPool = true
								if i < len(sentEvt.VerifierBlobs) {
									messageHash = sentEvt.VerifierBlobs[i]
								}
								break
							}
						}
						require.True(t, issuedByPool, "lombard pool %s not found in ReceiptIssuers", poolProgram)

						messageKey := cciptestinterfaces.MessageEventKey{MessageID: sentEvt.MessageID}
						_, err = v3Src.ConfirmSendOnSource(ctx, evmSelector, messageKey, defaultAssertionTimeout)
						require.NoError(t, err)

						require.NotNil(t, sentEvt.Message, "sent event should decode the sent message")
						require.NotNil(t, sentEvt.Message.TokenTransfer, "sent message should carry a token transfer")
						onWireAmount := sentEvt.Message.TokenTransfer.Amount

						if fakeURL != "" {
							require.NotEmpty(t, messageHash, "Lombard verifier blob not found in ReceiptIssuers/VerifierBlobs")
							e2ehelpers.SetLombardMailboxBridgedMessageIfSupported(ctx, t, evmImpl, sentEvt.MessageID)

							solBridgeRef, bridgeErr := ds.Addresses().Get(datastore.NewAddressRefKey(
								solSelector, datastore.ContractType(solcommon.LombardBridgeV2ContractType),
								solcommon.LombardChainContractsVersion, "",
							))
							require.NoError(t, bridgeErr, "resolve solana lombard bridge")
							solBridge, bridgeErr := solana.PublicKeyFromBase58(solBridgeRef.Address)
							require.NoError(t, bridgeErr)

							evmBridgeRef, bridgeErr := ds.Addresses().Get(datastore.NewAddressRefKey(
								evmSelector, datastore.ContractType(soldevenv.MockLombardBridgeContractType),
								solcommon.Version2_0_0, devenvcommon.LombardContractsQualifier,
							))
							require.NoError(t, bridgeErr, "resolve EVM lombard bridge")

							attestation := buildLombardAttestationForEVM(lombardEVMAttestationArgs{
								Sender:       sentEvt.Sender.Bytes(),
								DestToken:    destToken.Bytes(),
								Receiver:     receiver.Bytes(),
								Amount:       onWireAmount,
								MessageID:    sentEvt.MessageID,
								SourceBridge: solBridge.Bytes(),
								DestBridge:   common.HexToAddress(evmBridgeRef.Address).Bytes(),
							})
							registerLombardAttestation(t, fakeURL, messageHash, attestation)
						}

						execEnv, err := v3Dest.ConfirmExecOnDest(ctx, solSelector, messageKey, execTimeout(solSelector))
						require.NoError(t, err)
						require.NotNil(t, execEnv.Event)
						require.Equalf(t, cciptestinterfaces.ExecutionStateSuccess, execEnv.Event.State,
							"unexpected execution state, return data: %x", execEnv.Event.ReturnData)

						sourceEndBalance := lombardSolanaTokenBalance(ctx, t, solanaImpl.RPCClient(), payerKey, mint)
						require.Equal(t, new(big.Int).Sub(sourceStartBalance, lt.sendAmount), sourceEndBalance,
							"Solana source balance should drop by the transferred amount")

						if fakeURL == "" {
							// The pool's BPS fee is retained on the source; both mints are 8dp on both
							// sides, so the minted amount is exact.
							bpsFee, feeErr := getPoolFinalityBpsFee(ctx, solanaImpl.RPCClient(), poolProgram, evmSelector, mint)
							require.NoError(t, feeErr)
							expectedFee := new(big.Int).Div(new(big.Int).Mul(lt.sendAmount, big.NewInt(int64(bpsFee))), big.NewInt(10_000))
							expectedMinted := new(big.Int).Sub(lt.sendAmount, expectedFee)

							destEndBalance, destErr := evmImpl.GetTokenBalance(ctx, protocol.UnknownAddress(receiver.Bytes()), destToken)
							require.NoError(t, destErr)
							minted := new(big.Int).Sub(destEndBalance, destStartBalance)
							t.Logf("minted on destination: %s (bps fee %d) of %s burned", minted, bpsFee, lt.sendAmount)
							require.Equal(t, expectedMinted, minted, "destination minted amount should be the burn minus the source-side BPS fee")
						}

						poolEvt, evtErr := solanaImpl.ConfirmTokenPoolEventInTx(
							ctx,
							solana.SignatureFromBytes(srcTxHash),
							poolProgram,
							base_token_pool.Event_LockedOrBurned,
						)
						require.NoError(t, evtErr)
						normalized, normErr := NormalizeTokenPoolEvent(poolEvt)
						require.NoError(t, normErr)
						require.Equal(t, mint, normalized.Mint, "source pool event mint mismatch")
						require.NotNil(t, normalized.RemoteChainSelector, "lombard pool event should expose the remote chain selector")
						require.Equal(t, evmSelector, *normalized.RemoteChainSelector,
							"source pool event should retain the destination chain selector")
						require.Equal(t, onWireAmount.Uint64(), normalized.Amount, "source pool event amount mismatch")
					})
				}
			})
		}
	})

	// Solana -> EVM over CCTP: against the local fake attestation service on devenv, against a
	// deployed env otherwise. On a deployed env Circle attests for real, so there is no fake
	// service to POST to and no mock transmitter.
	//
	// Unlike Lombard, CCTP only runs straight into the router: it fails through the example sender
	// (or any program that CPIs into the router), for two separate reasons:
	//
	//  A. Transaction size. The sender's resolver returns no lookup table of its own, so its
	//     accounts are static keys and the send is ~1347 bytes, over Solana's 1232-byte limit.
	//     Fixable on our side: the sender would return a lookup table of its fixed accounts during
	//     resolution, created and extended by the deployment ops that add its chains and mints (see
	//     the note in the sender's resolve_accounts/start.rs). Callers would need no setup of their
	//     own.
	//  B. CPI depth. Solana caps the invocation stack at 5 levels, and CCTP already uses them all
	//     from the router down (router -> CCTP pool -> token messenger -> message transmitter -> its
	//     own CPI). The sender adds a level on top, so the message transmitter's CPI lands on level
	//     6 and fails with CallDepth ("Cross-program invocation call depth too deep"). Not fixable
	//     from the sender: it needs the runtime limit raised or fewer nested CPIs in CCTP.
	//
	// A fails first today; with A fixed the send fails on B instead. Integrators have to send CCTP
	// transfers to the router directly.
	t.Run("TokenTransfer/CCTP", func(t *testing.T) {
		fakeURL := fakeAttestationURL(t)
		sourceUsdcAmount := new(big.Int).Set(minSolanaTransferAmount)

		solCCTP := resolveSolanaCCTP(t, ds, solSelector)
		usdcMintAddr := protocol.UnknownAddress(solCCTP.USDCMint.Bytes())

		sourceStartBalance, err := solanaImpl.GetTokenBalance(ctx, payerAddr, usdcMintAddr)
		require.NoError(t, err)
		require.GreaterOrEqual(t, sourceStartBalance.Cmp(sourceUsdcAmount), 0,
			"sender %s holds %s USDC base units, needs %s", payerAddr, sourceStartBalance, sourceUsdcAmount)

		destUSDC := resolveEVMUSDC(t, ds, evmCLDFChain.Client, evmSelector)
		destUSDCToken := protocol.UnknownAddress(destUSDC.Bytes())

		receiver, _, executor, err := tcapi.ResolveV3SendAddresses(ctx, lib, solSelector, evmSelector)
		require.NoError(t, err)
		destStartBalance, err := evmImpl.GetTokenBalance(ctx, receiver, destUSDCToken)
		require.NoError(t, err)

		v3Src, err := lib.V3Source(ctx, solSelector)
		require.NoError(t, err)
		v3Dest, err := lib.V3Destination(ctx, evmSelector)
		require.NoError(t, err)

		sentEvt, srcTxHash, err := tcapi.SendV3Message(ctx, v3Src, v3Dest,
			cciptestinterfaces.MessageFields{
				Receiver: receiver,
				TokenAmount: cciptestinterfaces.TokenAmount{
					TokenAddress: usdcMintAddr,
					Amount:       sourceUsdcAmount,
				},
			},
			cciptestinterfaces.MessageOptions{
				// Lane is WAIT-only; a fast-finality request would revert.
				FinalityConfig: protocol.Finality(0),
				Executor:       executor,
			},
			tcapi.SendArgs{SendOption: devenvutils.MustSolSendOptions(t)},
		)
		require.NoError(t, err)
		require.Len(t, srcTxHash, solana.SignatureLength, "source receipt should contain a Solana signature")
		t.Logf("burn signature: %s  messageID: %x", solana.SignatureFromBytes(srcTxHash), [32]byte(sentEvt.MessageID))

		found := false
		committeeAttached := false
		for _, issuer := range sentEvt.ReceiptIssuers {
			if issuer.Equal(protocol.UnknownAddress(solCCTP.VerifierProgram.Bytes())) {
				found = true
			}
			if issuer.Equal(protocol.UnknownAddress(ccvProgramID.Bytes())) {
				committeeAttached = true
			}
		}
		require.True(t, found, "CCTP verifier not found in ReceiptIssuers")
		if committeeAttached {
			// A post-#427 router skips default CCVs on a token-only send. Harmless: the
			// committee just has to attest too.
			t.Log("NOTE: CommitteeVerifier attached to a token-only transfer - router predates #427")
		}

		require.NotEmpty(t, sentEvt.VerifierBlobs, "message carries no CCVs")

		messageKey := cciptestinterfaces.MessageEventKey{MessageID: sentEvt.MessageID}
		_, err = v3Src.ConfirmSendOnSource(ctx, evmSelector, messageKey, defaultAssertionTimeout)
		require.NoError(t, err)

		if fakeURL != "" {
			destTokenMessengerRef, refErr := ds.Addresses().Get(datastore.NewAddressRefKey(
				evmSelector, datastore.ContractType("MockE2EUSDCTokenMessenger"),
				semver.MustParse("1.0.0"), "",
			))
			require.NoError(t, refErr, "resolve EVM CCTP token messenger")
			registerCCTPAttestation(
				t,
				fakeURL,
				sentEvt.MessageID,
				protocol.UnknownAddress(solCCTP.MessageSender.Bytes()),
				receiver,
				solSelector,
				evmSelector,
				usdcMintAddr,
				protocol.UnknownAddress(common.HexToAddress(destTokenMessengerRef.Address).Bytes()),
				nil,
			)
		}

		execEnv, err := v3Dest.ConfirmExecOnDest(ctx, solSelector, messageKey, execTimeout(solSelector))
		require.NoError(t, err)
		require.NotNil(t, execEnv.Event)
		require.Equalf(t, cciptestinterfaces.ExecutionStateSuccess, execEnv.Event.State,
			"unexpected execution state, return data: %x", execEnv.Event.ReturnData)

		sourceEndBalance, err := solanaImpl.GetTokenBalance(ctx, payerAddr, usdcMintAddr)
		require.NoError(t, err)
		require.Equal(t, new(big.Int).Sub(sourceStartBalance, sourceUsdcAmount), sourceEndBalance,
			"Solana source balance should drop by exactly the burned amount")

		// Circle's fee is deducted on the destination, so assert a range, not equality.
		destEndBalance, err := evmImpl.GetTokenBalance(ctx, receiver, destUSDCToken)
		require.NoError(t, err)
		minted := new(big.Int).Sub(destEndBalance, destStartBalance)
		require.Positive(t, minted.Sign(), "destination receiver did not receive any USDC")
		require.LessOrEqual(t, minted.Cmp(sourceUsdcAmount), 0, "destination minted more than was burned")
	})

	// Programmable token transfer: a token transfer with arbitrary data in one message,
	// landing on the EVM mock receiver contract.
	for _, tc := range tokenPoolTestCases {
		t.Run("PTT/"+tc.caseName, func(t *testing.T) {
			combo, err := tokenCombination(tc.solana, tc.evm, declaredCombos(t, ds, solSelector, evmSelector))
			require.NoError(t, err, "pool pair %q not deployed in this environment", tc.caseName)

			evmReg, err := chainreg.GetRegistry().Get(chainsel.FamilyEVM)
			require.NoError(t, err)
			// data must hit a contract, not the EOA, so the messages lands on the mock receiver
			receiver, err := evmReg.AddressResolver.GetContractReceiver(ds, evmSelector, devenvcommon.DefaultReceiverQualifier)
			require.NoError(t, err)

			v3Src, err := lib.V3Source(ctx, solSelector)
			require.NoError(t, err)
			v3Dest, err := lib.V3Destination(ctx, evmSelector)
			require.NoError(t, err)

			srcBalReader, ok := v3Src.(cciptestinterfaces.TokenBalanceReader)
			require.True(t, ok, "Solana source should implement TokenBalanceReader")
			dstBalReader, ok := v3Dest.(cciptestinterfaces.TokenBalanceReader)
			require.True(t, ok, "EVM dest should implement TokenBalanceReader")
			senderProvider, ok := v3Src.(cciptestinterfaces.SenderAddressProvider)
			require.True(t, ok, "Solana source should implement SenderAddressProvider")
			sender, err := senderProvider.GetSenderAddress()
			require.NoError(t, err)

			localPool := combo.LocalPoolAddressRef()
			mintBytes, err := devenvutils.GetSolanaTokenAddress(ds, solSelector, localPool.Qualifier)
			require.NoError(t, err, "Solana token address not found for qualifier %q", localPool.Qualifier)
			mint := solana.PublicKeyFromBytes(mintBytes)
			srcToken := mintBytes
			destToken, err := evmReg.AddressResolver.GetToken(ds, evmSelector, combo.RemotePoolAddressRef())
			require.NoError(t, err, "resolve EVM dest token")

			require.NoError(t, solSenderTokenFundingError(ctx, solanaImpl, mint))

			srcStart, err := srcBalReader.GetTokenBalance(ctx, sender, srcToken)
			require.NoError(t, err)
			dstStart, err := dstBalReader.GetTokenBalance(ctx, receiver, destToken)
			require.NoError(t, err)

			_, ccvs, executor, err := tcapi.ResolveV3SendAddresses(ctx, lib, solSelector, evmSelector)
			require.NoError(t, err)

			sentEvt, sentTxHash, err := tcapi.SendV3Message(ctx, v3Src, v3Dest,
				cciptestinterfaces.MessageFields{
					Receiver: receiver,
					Data:     []byte("ptt sol2evm"),
					TokenAmount: cciptestinterfaces.TokenAmount{
						Amount:       sourceAmount,
						TokenAddress: srcToken,
					},
				},
				cciptestinterfaces.MessageOptions{
					// v2 Solana source pools reject fast finality in GetFee (InvalidRequestedFinality)
					FinalityConfig: protocol.Finality(0),
					Executor:       executor,
					CCVs:           ccvs,
				},
				tcapi.SendArgs{
					ExecutionGasLimit: devenvutils.MaxComputeUnits,
					SendOption:        devenvutils.MustSolSendOptions(t),
				},
			)
			require.NoError(t, err)
			require.Len(t, sentTxHash, solana.SignatureLength, "source receipt should contain a Solana signature")

			messageKey := cciptestinterfaces.MessageEventKey{MessageID: sentEvt.MessageID}
			_, err = v3Src.ConfirmSendOnSource(ctx, evmSelector, messageKey, defaultAssertionTimeout)
			require.NoError(t, err)

			aggregatorClient, indexerMonitor, err := tcapi.SetupOffchainClients(lib, "")
			require.NoError(t, err)
			testCtx, cleanup := tcapi.NewTestingContext(ctx, aggregatorClient, indexerMonitor)
			defer cleanup()
			_, err = testCtx.AssertMessage(sentEvt.MessageID, tcapi.AssertMessageOptions{
				TickInterval:            time.Second,
				Timeout:                 defaultAssertionTimeout,
				ExpectedVerifierResults: 1,
			})
			require.NoError(t, err)

			execEnv, err := v3Dest.ConfirmExecOnDest(ctx, solSelector, messageKey, execTimeout(solSelector))
			require.NoError(t, err)
			require.Equal(t, cciptestinterfaces.ExecutionStateSuccess, execEnv.Event.State, "exec should succeed end-to-end")
			requireEVMReceiverCall(ctx, t, evmRPC, execEnv.TxID, receiver)

			srcEnd, err := srcBalReader.GetTokenBalance(ctx, sender, srcToken)
			require.NoError(t, err)
			require.Equal(t, new(big.Int).Sub(srcStart, sourceAmount), srcEnd,
				"Solana source balance should drop by the source amount")
			expectedDestIncrease := destinationAmount
			isV2Pool := localPool.Version.GreaterThanEqual(solcommon.Version2_0_0)
			if isV2Pool {
				poolProgram, _, poolErr := solcommon.GetSolanaTokenPoolProgramAddress(
					ds, solSelector, string(localPool.Type), localPool.Qualifier,
				)
				require.NoError(t, poolErr)
				finalityBps, bpsErr := getPoolFinalityBpsFee(ctx, solanaImpl.RPCClient(), poolProgram, evmSelector, mint)
				require.NoError(t, bpsErr)
				bpsFeeAmount := sourceAmount.Uint64() * uint64(finalityBps) / 10_000
				postFeeSVM := new(big.Int).Sub(sourceAmount, new(big.Int).SetUint64(bpsFeeAmount))
				expectedDestIncrease = new(big.Int).Div(new(big.Int).Mul(postFeeSVM, destinationAmount), sourceAmount)
			}
			dstEnd, err := dstBalReader.GetTokenBalance(ctx, receiver, destToken)
			require.NoError(t, err)
			require.Equal(t, new(big.Int).Add(dstStart, expectedDestIncrease), dstEnd,
				"EVM mock receiver balance should gain the destination amount")

			poolProgram, poolVersion, err := solcommon.GetSolanaTokenPoolProgramAddress(
				ds, solSelector, string(localPool.Type), localPool.Qualifier,
			)
			require.NoError(t, err)
			require.NotNil(t, poolVersion, "resolved Solana pool version is nil")
			disc, err := expectedSourcePoolEventDiscriminator(string(localPool.Type), poolVersion)
			require.NoError(t, err)
			poolEvt, err := solanaImpl.ConfirmTokenPoolEventInTx(
				ctx,
				solana.SignatureFromBytes(sentTxHash),
				poolProgram,
				disc,
			)
			require.NoError(t, err)
			normalized, err := NormalizeTokenPoolEvent(poolEvt)
			require.NoError(t, err)
			require.Equal(t, mint, normalized.Mint, "source pool event mint mismatch")
			expectedPoolEventAmount := sourceAmount.Uint64()
			if isV2Pool {
				finalityBps2, bpsErr2 := getPoolFinalityBpsFee(ctx, solanaImpl.RPCClient(), poolProgram, evmSelector, mint)
				require.NoError(t, bpsErr2)
				expectedPoolEventAmount -= sourceAmount.Uint64() * uint64(finalityBps2) / 10_000
			}
			require.Equal(t, expectedPoolEventAmount, normalized.Amount, "source pool event amount mismatch")
			if isV2Pool {
				require.NotNil(t, normalized.RemoteChainSelector, "v2 pool event should expose the remote chain selector")
				require.Equal(t, evmSelector, *normalized.RemoteChainSelector,
					"source pool event should retain the destination chain selector")
			}
		})
	}
}
