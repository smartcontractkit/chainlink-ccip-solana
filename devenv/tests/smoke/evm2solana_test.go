package smoke

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/Masterminds/semver/v3"
	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	chainsel "github.com/smartcontractkit/chain-selectors"
	evmrouterops "github.com/smartcontractkit/chainlink-ccip/chains/evm/deployment/v1_2_0/operations/router"
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
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/ccip_common"
	token_messenger_minter_v2 "github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/cctp_token_messenger_minter_v2"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/example_ccip_receiver"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/example_ccip_receiver_v2"
	ccipsolutils "github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/ccip"
	contractcommon "github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	soltokens "github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/tokens"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	testreceiverops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/testreceiver"
	tokenpoolops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/tokenpools"
	soldevenv "github.com/smartcontractkit/chainlink-ccip-solana/devenv"
	"github.com/smartcontractkit/chainlink-ccip-solana/devenv/ccvregistry"
	devenvutils "github.com/smartcontractkit/chainlink-ccip-solana/devenv/tests/utils"
)

func requireSolanaReceiverEvent(
	ctx context.Context,
	t *testing.T,
	solanaImpl *soldevenv.Chain,
	txID protocol.ByteSlice,
	receiver solana.PublicKey,
	isV2Receiver bool,
	messageID protocol.Bytes32,
	sourceChainSelector uint64,
	sender []byte,
) {
	t.Helper()

	maxTxVersion := uint64(0)
	txResult, err := solanaImpl.RPCClient().GetTransaction(ctx, solana.SignatureFromBytes(txID), &rpc.GetTransactionOpts{
		Commitment:                     rpc.CommitmentFinalized,
		MaxSupportedTransactionVersion: &maxTxVersion,
	})
	require.NoError(t, err)
	require.NotNil(t, txResult)
	require.NotNil(t, txResult.Meta)
	require.NotNil(t, txResult.Transaction)
	tx, err := txResult.Transaction.GetTransaction()
	require.NoError(t, err)

	if isV2Receiver {
		evt, eventErr := contractcommon.ExtractCPIEvent(
			tx, txResult.Meta, receiver, example_ccip_receiver_v2.ParseEvent_MessageReceived,
		)
		require.NoError(t, eventErr, "extract v2 receiver MessageReceived event")
		require.Equal(t, [32]byte(messageID), evt.MessageId)
		require.Equal(t, sourceChainSelector, evt.SourceChainSelector)
		require.Equal(t, sender, evt.Sender)
		return
	}

	evt, err := contractcommon.ParseEventFromLogs(
		txResult.Meta.LogMessages, example_ccip_receiver.ParseEvent_MessageReceived,
	)
	require.NoError(t, err, "parse v1 receiver MessageReceived event")
	require.Equal(t, [32]byte(messageID), evt.MessageId)
}

// TODO: temp food for thoughts
// - Only arb message
//   - V1 receiver
//   - V2 receiver
//
// - Only token transfer
//   - V1 LnR
//   - V1 BnM
//   - V2 LnR
//   - V2 BnM
//
// - Token transfer + Arbitrary Message
//   - V1 LnR + V1 receiver
//   - V1 BnM + V1 receiver
//   - V2 LnR + V2 receiver
//   - V2 BnM + V2 receiver
//
// TestEVM2Solana sends CCIP messages from the EVM chain to the Solana chain
// and verifies they are aggregated and executed on-chain.
//
// Subtests:
//   - TokenTransfer: tcapi send/assert/confirm with receipt-based pool event assertions.
//   - ArbMessaging: tcapi send/assert/confirm against v1 and v2 Solana receiver programs.
//   - ArbMessagingLargePayload: hand-built buffered path with an 8KB payload.
//
// Expects the devenv to already be running:
//
//	make start-devenv
//
// Local output: env-phased-out.toml. Staging attach: env-private-staging.deployed.toml.
func TestEVM2Solana(t *testing.T) {
	factory := soldevenv.NewImplFactory()
	regErr := chainreg.Register(chainsel.FamilySolana, chainreg.Registration{
		ImplFactory:     factory,
		ExecutorInfo:    factory,
		CLDFProvider:    ccvregistry.SolanaCLDFProviderFactory,
		AddressResolver: soldevenv.NewAddressResolver(),
	})
	require.NoError(t, regErr)

	ctx := ccv.Plog.WithContext(t.Context())
	lib, envErr := ccv.NewLibFromCCVEnv(zerolog.Ctx(ctx), e2ehelpers.GetSmokeTestConfig())
	require.NoError(t, envErr)

	ds, dataErr := lib.DataStore()
	require.NoError(t, dataErr)

	chains, chainErr := lib.ChainsMap(ctx)
	require.NoError(t, chainErr)

	cfg, cfgErr := smokeConfig()
	require.NoError(t, cfgErr)
	solSelector, evmSelector, laneErr := devenvutils.ResolveSolanaEVMLanes(cfg.Blockchains)
	require.NoError(t, laneErr)

	solanaChain, ok := chains[solSelector]
	require.True(t, ok, "Solana chain (selector %d) not found in output config", solSelector)
	solanaImpl, ok := solanaChain.(*soldevenv.Chain)
	require.True(t, ok, "Solana chain should be backed by *soldevenv.Chain")
	t.Cleanup(func() {
		if closer, canClose := solanaChain.(interface{ Close() error }); canClose {
			_ = closer.Close()
		}
	})

	executorArgs := ccipsolutils.SVMExecutorArgs{
		SvmDestBlob: ccip_common.SvmDestBlob{
			UseAta:                  0, // using atomic ATA creation (DeriveAtaAndCreate)
			AccountIsWritableBitmap: 0,
			Accounts:                []solana.PublicKey{},
		},
	}

	// Resolve both receivers from the datastore rather than the gobinding declare_id!, which only
	// matches in devenv.
	receiverV1ProgramID, recvErr := soldevenv.ResolveTestReceiverAddress(ds, solSelector, testreceiverops.LegacyVersion)
	require.NoError(t, recvErr)
	receiverV2ProgramID, recvErr := soldevenv.ResolveTestReceiverAddress(ds, solSelector, testreceiverops.Version)
	require.NoError(t, recvErr)
	v1ReceiverStatePDA, _, stateErr := state.FindStateReceiverPDA(receiverV1ProgramID)
	require.NoError(t, stateErr)
	// get client
	cldfEnv, envErr := lib.CLDFEnvironment()
	require.NoError(t, envErr)
	solChainCLDF, ok := cldfEnv.BlockChains.SolanaChains()[solSelector]
	require.True(t, ok, "Solana chain (selector %d) not found in CLDF environment", solSelector)
	require.NotNil(t, solChainCLDF.DeployerKey, "Solana deployer key not set")

	evmImpl, ok := chains[evmSelector]
	require.True(t, ok, "EVM chain (selector %d) not found in output config", evmSelector)
	evmCLDFChain, ok := cldfEnv.BlockChains.EVMChains()[evmSelector]
	require.True(t, ok, "EVM chain (selector %d) not found in CLDF environment", evmSelector)
	evmRPC := evmRPCClient(t, evmCLDFChain.Client)
	evmSender, senderErr := evmImpl.GetSenderAddress()
	require.NoError(t, senderErr)
	evmName, nameErr := chainsel.GetChainNameFromSelector(evmSelector)
	require.NoError(t, nameErr)
	t.Logf("EVM chain: selector %d (%s)", evmSelector, evmName)
	paddedSender := make([]byte, 32)
	copy(paddedSender[32-len(evmSender):], evmSender)
	solanaV3Dest, ok := solanaChain.(cciptestinterfaces.MessageV3Destination)
	require.True(t, ok, "Solana chain should implement MessageV3Destination")

	v1ApprovedSenderPDA, approvedSenderErr := state.FindApprovedSender(evmSelector, paddedSender, receiverV1ProgramID)
	require.NoError(t, approvedSenderErr)

	arbExecutorArgsV1 := ccipsolutils.SVMExecutorArgs{
		SvmDestBlob: ccip_common.SvmDestBlob{
			UseAta:                  0,
			AccountIsWritableBitmap: 0,
			Accounts: []solana.PublicKey{
				v1ApprovedSenderPDA,
				v1ReceiverStatePDA,
			},
		},
	}
	arbExecutorArgsV2, err := devenvutils.BuildV2ReceiverExecutorArgs(evmSelector, evmSender, receiverV2ProgramID)
	require.NoError(t, err)

	routerType := evmrouterops.ContractType
	if useTestRouter() {
		routerType = evmrouterops.TestRouterContractType
	}
	routerVersion := semver.MustParse(evmrouterops.Deploy.Version())
	routerRefs := ds.Addresses().Filter(
		datastore.AddressRefByChainSelector(evmSelector),
		datastore.AddressRefByType(datastore.ContractType(routerType)),
		datastore.AddressRefByVersion(routerVersion),
	)
	require.Len(t, routerRefs, 1, "expected exactly one EVM %s", routerType)
	evmRouterAddr := ethcommon.HexToAddress(routerRefs[0].Address)

	// target token transfer amounts: minimum that exercises the decimals path
	sourceAmount := new(big.Int).Set(minEVMTransferAmount)
	destinationAmount := new(big.Int).Set(minSolanaTransferAmount)

	for _, tc := range tokenPoolTestCases {
		t.Run("TokenTransfer/"+tc.caseName, func(t *testing.T) {
			caseName := tc.caseName
			solTokenReceiver := solana.NewWallet().PublicKey()
			combo, combErr := tokenCombination(tc.evm, tc.solana, declaredCombos(t, ds, evmSelector, solSelector))
			require.NoError(t, combErr, "pool pair %q not deployed in this environment", caseName)

			// get mint address
			remotePool := combo.RemotePoolAddressRef()
			mint := solanaMintForPoolPair(t, ds, solSelector, remotePool.Qualifier)

			// define the token transfer test case; per-combo finality: v1 EVM source
			// pools request default (full) finality — fast finality reverts with
			// FTFNotSupportedOnPoolV1 — v2 pools request fast finality.
			tc := token_transfer.TokenTransfer(
				lib,
				evmSelector,
				solSelector,
				combo,
				combo.FinalityConfig(),
				true, // useEOAReceiver
				tc.caseName,
				token_transfer.Args{
					Send: tcapi.SendArgs{
						ExecutionGasLimit:   devenvutils.MaxComputeUnits,
						ExtraArgsParams:     executorArgs,
						TokenReceiverParams: protocol.UnknownAddress(solTokenReceiver.Bytes()),
						SendOption:          mustEvmSendOptions(t, evmSelector),
					},
					Run: tcapi.RunConfig{
						ConfirmSentTimeout: defaultAssertionTimeout,
						ConfirmExecTimeout: execTimeout(evmSelector),
					},
					TransferAmount:      sourceAmount,
					DestBalanceIncrease: destinationAmount,
				},
			)

			if !tc.HavePrerequisites(ctx) {
				t.Skipf("pool pair %q prerequisites not met in this environment", caseName)
			}

			ensureEVMAllowance(ctx, t, evmCLDFChain, ds, evmSelector, combo, evmRouterAddr, sourceAmount)

			result, testErr := tc.Run(ctx)
			require.NoError(t, testErr)
			require.Len(t, result.Src.TxID, 32, "source receipt should contain an EVM transaction hash")
			require.Len(t, result.Dest.TxID, solana.SignatureLength, "destination receipt should contain a Solana signature")

			// RunWithResult already fails unless execution succeeded; assert only the fields it
			// does not check: message identity, source selector, message number, and that a
			// token-only transfer carries no receiver return data.
			execEvt := result.Dest.Event
			require.EqualValues(t, result.Src.Event.MessageID, execEvt.MessageID, "execution event should match the sent message ID")
			require.Equal(t, protocol.ChainSelector(evmSelector), execEvt.SourceChainSelector,
				"execution event should retain the source chain selector")
			require.NotZero(t, execEvt.MessageNumber, "execution event should expose a non-zero message number")
			require.Empty(t, execEvt.ReturnData, "token-only transfer should not return receiver data")

			// get program address and version
			poolProgram, poolVersion, tokenErr := solcommon.GetSolanaTokenPoolProgramAddress(
				ds,
				solSelector,
				string(remotePool.Type),
				remotePool.Qualifier,
			)
			require.NoError(t, tokenErr)
			require.NotNil(t, poolVersion, "resolved Solana pool version is nil")
			require.Equal(t, remotePool.Version.String(), poolVersion.String(), "resolved Solana pool version mismatch")

			// retrieve tokenpool event from the Solana tx
			discriminator, poolErr := expectedPoolEventDiscriminator(string(remotePool.Type), poolVersion)
			require.NoError(t, poolErr)
			poolEvt, eventErr := solanaImpl.ConfirmTokenPoolEventInTx(
				ctx,
				solana.SignatureFromBytes(result.Dest.TxID),
				poolProgram,
				discriminator,
			)
			require.NoError(t, eventErr)

			// normalize the event and assert its fields
			normalized, normErr := NormalizeTokenPoolEvent(poolEvt)
			require.NoError(t, normErr)
			require.Equal(t, mint, normalized.Mint, "token pool event mint mismatch")
			require.Equal(t, solTokenReceiver, normalized.Recipient, "token pool event recipient mismatch")
			require.Equal(t, destinationAmount.Uint64(), normalized.Amount, "token pool event amount mismatch")
			if poolVersion.GreaterThanEqual(solcommon.Version2_0_0) {
				require.NotNil(t, normalized.RemoteChainSelector, "v2 pool event should expose the remote chain selector")
				require.Equal(t, evmSelector, *normalized.RemoteChainSelector,
					"token pool event should retain the source chain selector")
			}
		})
	}

	// TokenTransfer/Lombard exercises the Lombard custom-CCV token pool: a plain EVM->Solana
	// token transfer of the Lombard-wrapped test asset. Unlike the pool pairings above, this can't go through
	// tokenCombination/token_transfer.TokenTransfer: devenvcommon.AllTokenCombinations() has no
	// Lombard entry, so the message is built directly with tcapi.
	//
	// No Lombard-specific CCV needs to be requested by the sender the token pools unconditionally inject the
	// Lombard verifier as a required CCV for any message carrying this token.
	//
	// On a deployed env Lombard's real attestation infrastructure serves the verifier: nothing
	// registers an attestation here, and default finality on the EVM chain needs a long exec window.
	// The 48-vs-36 optional-message skew only affects the Solana->EVM direction: the EVM outbound
	// sends exactly 36 bytes and the Solana bridge's inbound parse is length-tolerant.
	t.Run("TokenTransfer/Lombard", func(t *testing.T) {
		fakeURL := fakeAttestationURL(t)

		// The receiver is the Solana sender: the env key (test sender) holds funded
		// LBTC/BTCb ATAs, and the verifier's end_receiver_token_account has no
		// init_if_needed, so execution would revert on a fresh wallet.
		solReceiver, recvErr := solanaImpl.GetSenderAddress()
		require.NoError(t, recvErr)
		solReceiverPubkey := solana.PublicKeyFromBytes(solReceiver)

		lombardTokens := []lombardToken{
			{mintType: string(solcommon.SPLTokens), mintQualifier: "Lombard_LBTC", evmPoolQualifier: "Lombard_LBTC",
				sendAmount: lombardTransferAmount, destAmount: lombardTransferAmount},
			{mintType: string(solcommon.SPL2022Tokens), mintQualifier: "Lombard_BTCb", evmPoolQualifier: "Lombard_BTCb",
				sendAmount: lombardTransferAmount, destAmount: lombardTransferAmount},
		}
		if fakeURL != "" {
			// Not the shared amounts: the devenv mock mints are calibrated for
			// EVM 18dp / soldevenv.LombardSolanaMintDecimals (9dp), and only the LBTC
			// token pair is deployed.
			lombardTokens = []lombardToken{
				{mintType: string(solcommon.SPLTokens), mintQualifier: "LBTC", evmPoolQualifier: "Lombard_LBTC",
					sendAmount: tokens.ScaleTokenAmount(big.NewInt(1), 18),
					destAmount: tokens.ScaleTokenAmount(big.NewInt(1), soldevenv.LombardSolanaMintDecimals)},
			}
		}

		v3Src, srcErr := lib.V3Source(ctx, evmSelector)
		require.NoError(t, srcErr)
		v3Dst, dstErr := lib.V3Destination(ctx, solSelector)
		require.NoError(t, dstErr)

		srcBal, ok := v3Src.(cciptestinterfaces.TokenBalanceReader)
		require.True(t, ok, "EVM source should implement TokenBalanceReader")

		_, ccvs, executor, addrErr := tcapi.ResolveV3SendAddresses(ctx, lib, evmSelector, solSelector)
		require.NoError(t, addrErr)

		for _, lt := range lombardTokens {
			t.Run(lt.mintQualifier, func(t *testing.T) {
				mintRef, err := solcommon.PoolRefByQualifier(ds, solSelector, lt.mintType, lt.mintQualifier)
				require.NoError(t, err, "resolve Solana lombard mint")
				solMint, err := solana.PublicKeyFromBase58(mintRef.Address)
				require.NoError(t, err)

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
				evmPool := ethcommon.HexToAddress(evmPoolRefs[0].Address)
				// The EVM side holds the ERC-20 wrapper: the pool's getToken() is the
				// source token the pool burns.
				srcToken := resolveLombardEVMPoolToken(ctx, t, evmRPC, evmPool)
				srcTokenAddr := protocol.UnknownAddress(srcToken.Bytes())
				t.Logf("EVM pool %s token %s  Solana mint %s", evmPool, srcToken, solMint)

				// Fail fast if the receiver's ATA is missing (it must pre-exist).
				dstStart := lombardSolanaTokenBalance(ctx, t, solanaImpl.RPCClient(), solReceiverPubkey, solMint)

				srcStart, balErr := srcBal.GetTokenBalance(ctx, evmSender, srcTokenAddr)
				require.NoError(t, balErr)
				require.GreaterOrEqual(t, srcStart.Cmp(lt.sendAmount), 0,
					"EVM sender %x holds %s of %s base units, needs %s",
					evmSender, srcStart, lt.mintQualifier, lt.sendAmount)

				sentEvt, _, sendErr := tcapi.SendV3Message(ctx, v3Src, v3Dst,
					cciptestinterfaces.MessageFields{
						Receiver: make([]byte, 32), // pure token transfer, no receiver logic
						TokenAmount: cciptestinterfaces.TokenAmount{
							Amount:       lt.sendAmount,
							TokenAddress: srcTokenAddr,
						},
					},
					cciptestinterfaces.MessageOptions{
						// The Lombard EVM pools allow only default finality — requesting fast
						// finality reverts in the pool's fee check (InvalidRequestedFinality(1,0)).
						FinalityConfig: protocol.Finality(0),
						Executor:       executor,
						CCVs:           ccvs,
					},
					tcapi.SendArgs{
						ExecutionGasLimit:   devenvutils.MaxComputeUnits,
						ExtraArgsParams:     executorArgs,
						TokenReceiverParams: protocol.UnknownAddress(solReceiverPubkey.Bytes()),
						SendOption:          mustEvmSendOptions(t, evmSelector),
					},
				)
				require.NoError(t, sendErr)
				t.Logf("burn messageID: %x", [32]byte(sentEvt.MessageID))
				for i, blob := range sentEvt.VerifierBlobs {
					t.Logf("verifier blob %d: %x", i, blob)
				}

				messageKey := cciptestinterfaces.MessageEventKey{MessageID: sentEvt.MessageID}
				_, confErr := v3Src.ConfirmSendOnSource(ctx, solSelector, messageKey, defaultAssertionTimeout)
				require.NoError(t, confErr)

				if fakeURL != "" {
					deliverLombardMessage(ctx, t, fakeURL, ds, cldfEnv, solChainCLDF, evmSelector, solSelector,
						sentEvt.MessageID, solReceiverPubkey, lt.destAmount.Uint64())

					aggregatorClient, indexerMonitor, idxErr := tcapi.SetupOffchainClients(lib, "")
					require.NoError(t, idxErr)
					testCtx, cleanup := tcapi.NewTestingContext(ctx, aggregatorClient, indexerMonitor)
					defer cleanup()
					_, idxErr = testCtx.AssertMessage(sentEvt.MessageID, tcapi.AssertMessageOptions{
						TickInterval:            time.Second,
						Timeout:                 defaultAssertionTimeout,
						ExpectedVerifierResults: 2, // default committee CCV + Lombard's own (because of non-empty extra args gas)
					})
					require.NoError(t, idxErr)
				}

				execEnv, execErr := v3Dst.ConfirmExecOnDest(ctx, evmSelector, messageKey, execTimeout(evmSelector))
				require.NoError(t, execErr)
				require.NotNil(t, execEnv.Event)
				require.Equalf(t, cciptestinterfaces.ExecutionStateSuccess, execEnv.Event.State,
					"unexpected execution state, return data: %x", execEnv.Event.ReturnData)

				srcEnd, balErr := srcBal.GetTokenBalance(ctx, evmSender, srcTokenAddr)
				require.NoError(t, balErr)
				require.Equal(t, new(big.Int).Sub(srcStart, lt.sendAmount), srcEnd,
					"EVM source balance should drop by exactly the burned amount")

				dstEnd := lombardSolanaTokenBalance(ctx, t, solanaImpl.RPCClient(), solReceiverPubkey, solMint)
				minted := new(big.Int).Sub(dstEnd, dstStart)
				if fakeURL != "" {
					require.Equal(t, new(big.Int).Add(dstStart, lt.destAmount), dstEnd,
						"Solana token receiver ATA should gain the destination amount")
				} else {
					// The bps fee is charged by the source-side EVM pool (its TTFC), so the
					// destination mint is amount minus fee; assert a range.
					t.Logf("minted on Solana: %s of %s burned", minted, lt.sendAmount)
					require.Positive(t, minted.Sign(), "Solana receiver did not receive any tokens")
					require.LessOrEqual(t, minted.Cmp(lt.sendAmount), 0,
						"destination minted more than was burned")
				}

				poolEvt, evtErr := solanaImpl.ConfirmTokenPoolEventInTx(
					ctx,
					solana.SignatureFromBytes(execEnv.TxID),
					poolProgram,
					base_token_pool.Event_ReleasedOrMinted,
				)
				require.NoError(t, evtErr)
				normalized, normErr := NormalizeTokenPoolEvent(poolEvt)
				require.NoError(t, normErr)
				require.Equal(t, solMint, normalized.Mint, "pool event mint mismatch")
				require.Equal(t, solReceiverPubkey, normalized.Recipient, "pool event recipient should be the token receiver")
				require.NotNil(t, normalized.RemoteChainSelector, "lombard pool event should expose the remote chain selector")
				require.Equal(t, evmSelector, *normalized.RemoteChainSelector,
					"pool event should retain the source chain selector")
				require.Equal(t, minted.Uint64(), normalized.Amount, "pool event amount mismatch")
			})
		}
	})

	// EVM -> Solana over CCTP: against the local fake attestation service on devenv, against a
	// deployed env otherwise. On a deployed env Circle attests for real, so there is no fake
	// service and no mock transmitter. Uses fast finality on a deployed env: standard finality
	// on an EVM source is ~15-19 min, fast is ~20s.
	t.Run("TokenTransfer/CCTP", func(t *testing.T) {
		fakeURL := fakeAttestationURL(t)

		// USDC is 6dp on both chains, so no decimal scaling.
		amount := new(big.Int).Set(minSolanaTransferAmount)

		solCCTP := resolveSolanaCCTP(t, ds, solSelector)
		solMintAddr := protocol.UnknownAddress(solCCTP.USDCMint.Bytes())
		evmUSDC := resolveEVMUSDC(t, ds, evmCLDFChain.Client, evmSelector)
		evmUSDCAddr := protocol.UnknownAddress(evmUSDC.Bytes())

		// The receiver's ATA must already exist: verify's end_receiver_token_account is a plain
		// InterfaceAccount with no init_if_needed, so a fresh wallet makes execution revert.
		// The Solana sender always holds USDC, so reuse it as the token receiver.
		// TODO: confirm the Solana sender is the right CCTP receiver
		solReceiver, recvErr := solanaImpl.GetSenderAddress()
		require.NoError(t, recvErr)
		solReceiverPubkey := solana.PublicKeyFromBytes(solReceiver)

		var (
			evmVerifier  ethcommon.Address
			evmMessenger ethcommon.Address
			poolATA      solana.PublicKey
		)
		if fakeURL != "" {
			evmVerifier = resolveEVMCCTPVerifier(t, ds, evmSelector)
			var regErr error
			evmMessenger, regErr = soldevenv.MockUSDCTokenMessengerAddress(ds, evmSelector)
			require.NoError(t, regErr)
			// Circle's receive_message mints to the pool's own proxy ATA, not straight to the end
			// receiver: cctp-token-pool's verify processor requires the attested mint_recipient to be
			// pool_token_account, then CCIP's regular release_or_mint forwards the funds on.
			poolATA, _, regErr = soltokens.FindAssociatedTokenAddress(
				solana.TokenProgramID, solCCTP.USDCMint, solCCTP.MessageSender,
			)
			require.NoError(t, regErr)
		}

		v3Src, srcErr := lib.V3Source(ctx, evmSelector)
		require.NoError(t, srcErr)
		v3Dst, dstErr := lib.V3Destination(ctx, solSelector)
		require.NoError(t, dstErr)

		srcBal, ok := v3Src.(cciptestinterfaces.TokenBalanceReader)
		require.True(t, ok, "EVM source should implement TokenBalanceReader")
		dstBal, ok := v3Dst.(cciptestinterfaces.TokenBalanceReader)
		require.True(t, ok, "Solana dest should implement TokenBalanceReader")

		srcStart, balErr := srcBal.GetTokenBalance(ctx, evmSender, evmUSDCAddr)
		require.NoError(t, balErr)
		require.GreaterOrEqual(t, srcStart.Cmp(amount), 0,
			"EVM sender %x holds %s USDC base units, needs %s", evmSender, srcStart, amount)
		dstStart, balErr := dstBal.GetTokenBalance(ctx, solReceiver, solMintAddr)
		require.NoError(t, balErr, "token receiver %s has no USDC ATA", solReceiverPubkey)

		_, ccvs, executor, addrErr := tcapi.ResolveV3SendAddresses(ctx, lib, evmSelector, solSelector)
		require.NoError(t, addrErr)

		// Fast finality (block depth 1). The EVM chain's verifier and CCV pool both allow it
		// (getAllowedFinalityConfig = 0x00000001), and with no verifierArgs the verifier
		// derives maxFee from its own fastFinalityBps (50) rather than requiring a quote.
		// Circle attests at the confirmed level: ~20s instead of ~15-19 min.
		finality := protocol.Finality(1)
		if fakeURL != "" {
			// Local devenv hasn't configured fast finality for this lane
			finality = protocol.Finality(0)
		}

		sentEvt, _, sendErr := tcapi.SendV3Message(ctx, v3Src, v3Dst,
			cciptestinterfaces.MessageFields{
				Receiver: make([]byte, 32), // pure token transfer, no receiver logic
				TokenAmount: cciptestinterfaces.TokenAmount{
					Amount:       amount,
					TokenAddress: evmUSDCAddr,
				},
			},
			cciptestinterfaces.MessageOptions{
				FinalityConfig: finality,
				Executor:       executor,
				CCVs:           ccvs,
			},
			tcapi.SendArgs{
				ExecutionGasLimit:   devenvutils.MaxComputeUnits,
				ExtraArgsParams:     executorArgs,
				TokenReceiverParams: protocol.UnknownAddress(solReceiverPubkey.Bytes()),
			},
		)
		require.NoError(t, sendErr)
		t.Logf("burn messageID: %x", [32]byte(sentEvt.MessageID))

		messageKey := cciptestinterfaces.MessageEventKey{MessageID: sentEvt.MessageID}
		_, confErr := v3Src.ConfirmSendOnSource(ctx, solSelector, messageKey, defaultAssertionTimeout)
		require.NoError(t, confErr)

		if fakeURL != "" {
			registerCCTPAttestation(
				t,
				fakeURL,
				sentEvt.MessageID,
				protocol.UnknownAddress(evmVerifier.Bytes()),
				protocol.UnknownAddress(poolATA.Bytes()),
				evmSelector, solSelector,
				evmUSDCAddr,
				protocol.UnknownAddress(token_messenger_minter_v2.ProgramID.Bytes()),
				&cctpInbound{
					sourceTokenMessenger: protocol.UnknownAddress(evmMessenger.Bytes()),
					amount:               amount,
				},
			)
		}

		execEnv, execErr := v3Dst.ConfirmExecOnDest(ctx, evmSelector, messageKey, execTimeout(evmSelector))
		require.NoError(t, execErr)
		require.NotNil(t, execEnv.Event)
		require.Equalf(t, cciptestinterfaces.ExecutionStateSuccess, execEnv.Event.State,
			"unexpected execution state, return data: %x", execEnv.Event.ReturnData)

		srcEnd, balErr := srcBal.GetTokenBalance(ctx, evmSender, evmUSDCAddr)
		require.NoError(t, balErr)
		require.Equal(t, new(big.Int).Sub(srcStart, amount), srcEnd,
			"EVM source balance should drop by exactly the burned amount")

		// Circle's fee is deducted on the destination, so assert a range, not equality.
		dstEnd, balErr := dstBal.GetTokenBalance(ctx, solReceiver, solMintAddr)
		require.NoError(t, balErr)
		minted := new(big.Int).Sub(dstEnd, dstStart)
		require.Positive(t, minted.Sign(), "Solana receiver did not receive any USDC")
		require.LessOrEqual(t, minted.Cmp(amount), 0, "destination minted more than was burned")

		poolEvt, evtErr := solanaImpl.ConfirmTokenPoolEventInTx(
			ctx,
			solana.SignatureFromBytes(execEnv.TxID),
			solCCTP.VerifierProgram,
			base_token_pool.Event_ReleasedOrMinted,
		)
		require.NoError(t, evtErr)
		normalized, evtErr := NormalizeTokenPoolEvent(poolEvt)
		require.NoError(t, evtErr)
		require.Equal(t, solCCTP.USDCMint, normalized.Mint, "pool event mint mismatch")
		require.Equal(t, solReceiverPubkey, normalized.Recipient, "pool event recipient should be the token receiver")
		require.NotNil(t, normalized.RemoteChainSelector, "CCTP pool event should expose the remote chain selector")
		require.Equal(t, evmSelector, *normalized.RemoteChainSelector,
			"pool event should retain the source chain selector")
		if fakeURL != "" {
			require.Equal(t, amount.Uint64(), normalized.Amount, "pool event amount mismatch")
		} else {
			// Circle's fee is deducted on the destination, so the event amount is below the burned amount.
			require.LessOrEqual(t, normalized.Amount, amount.Uint64(), "pool event amount exceeds the burned amount")
		}
	})

	// Both receivers are initialized and sender-approved in PostDeployContractsForSelector; the v2
	// one is also registered with the router and CCV-configured there.

	t.Run("ArbMessaging/V1Receiver", func(t *testing.T) {
		// The default receiver is v2; this case checks that v1 is still supported.
		v1Receiver, ccvs, executor, err := devenvutils.ResolveSendAddressesWithReceiverQualifier(
			ctx, lib, evmSelector, solSelector, soldevenv.LegacyReceiverQualifier,
		)
		require.NoError(t, err)

		evmV3Source, ok := chains[evmSelector].(cciptestinterfaces.MessageV3Source)
		require.True(t, ok)

		extraArgs, err := evmV3Source.BuildV3ExtraArgs(cciptestinterfaces.MessageOptions{
			// Must be full finality: a v1 receiver has no CCV/finality interface, so the offramp
			// treats it as allowing only WAIT_FOR_FINALITY (INV-FIN-DST-2 / INV-FIN-OPTIN-2) and
			// rejects any faster-than-finality request. FTF against a v1 receiver is covered as a
			// negative case in the offramp integration tests.
			FinalityConfig:    protocol.Finality(0),
			ExecutionGasLimit: devenvutils.MaxComputeUnits,
			Executor:          executor,
			CCVs:              ccvs,
		}, solanaV3Dest, arbExecutorArgsV1, nil, nil)
		require.NoError(t, err)

		evmSource, ok := chains[evmSelector].(cciptestinterfaces.ChainAsSource)
		require.True(t, ok)

		msg, err := evmSource.BuildChainMessage(ctx, cciptestinterfaces.MessageFields{
			Receiver: v1Receiver,
			Data:     []byte("v1 receiver test"),
		}, extraArgs)
		require.NoError(t, err)

		sentEvt, _, err := evmSource.SendChainMessage(ctx, solSelector, msg, mustEvmSendOptions(t, evmSelector))
		require.NoError(t, err)

		_, err = solanaChain.ConfirmExecOnDest(ctx, evmSelector,
			cciptestinterfaces.MessageEventKey{MessageID: sentEvt.MessageID}, execTimeout(evmSelector))
		require.NoError(t, err)
	})

	t.Run("ArbMessaging/V2Receiver", func(t *testing.T) {
		// basic.EOAReceiverDefaultVerifier resolves the receiver through GetEOAReceiverAddress, so the
		// send is built manually like the v1 case to resolve the receiver through the AddressResolver.
		v2Receiver, ccvs, executor, err := devenvutils.ResolveSendAddressesWithReceiverQualifier(
			ctx, lib, evmSelector, solSelector, devenvcommon.DefaultReceiverQualifier,
		)
		require.NoError(t, err)

		evmV3Source, ok := chains[evmSelector].(cciptestinterfaces.MessageV3Source)
		require.True(t, ok)

		extraArgs, err := evmV3Source.BuildV3ExtraArgs(cciptestinterfaces.MessageOptions{
			FinalityConfig:    protocol.Finality(1),
			ExecutionGasLimit: devenvutils.MaxComputeUnits,
			Executor:          executor,
			CCVs:              ccvs,
		}, solanaV3Dest, arbExecutorArgsV2, nil, nil)
		require.NoError(t, err)

		evmSource, ok := chains[evmSelector].(cciptestinterfaces.ChainAsSource)
		require.True(t, ok)

		msg, err := evmSource.BuildChainMessage(ctx, cciptestinterfaces.MessageFields{
			Receiver: v2Receiver,
			Data:     []byte("v2 receiver test"),
		}, extraArgs)
		require.NoError(t, err)

		sentEvt, _, err := evmSource.SendChainMessage(ctx, solSelector, msg, mustEvmSendOptions(t, evmSelector))
		require.NoError(t, err)

		execEnv, err := solanaChain.ConfirmExecOnDest(ctx, evmSelector,
			cciptestinterfaces.MessageEventKey{MessageID: sentEvt.MessageID}, execTimeout(evmSelector))
		require.NoError(t, err)
		require.Equal(t, cciptestinterfaces.ExecutionStateSuccess, execEnv.Event.State)
	})

	t.Run("ArbMessaging/LargePayload", func(t *testing.T) {
		// Sends the offramp's known-good max data size (see Chain.GetMaxDataBytes), exercising the
		// buffered execute flow end-to-end. With the custom 256KB heap allocator in the offramp and
		// the test receiver, the cap is 8KB; the compute budget, not the heap, is now the limit.
		tc := basic.MaxDataSize(lib, evmSelector, solSelector, basic.Args{
			Send: tcapi.SendArgs{
				ExecutionGasLimit: devenvutils.MaxComputeUnits,
				ExtraArgsParams:   arbExecutorArgsV2,
				SendOption:        mustEvmSendOptions(t, evmSelector),
			},
			Run: tcapi.RunConfig{
				ConfirmSentTimeout: defaultAssertionTimeout,
				ConfirmExecTimeout: defaultAssertionTimeout,
			},
		})
		_, err := tc.Run(ctx)
		require.NoError(t, err)
	})

	// Programmable token transfers: a token transfer with arbitrary data and a distinct
	// token receiver in a single message. Receiver and pool versions are independent, so
	// every pool case is exercised against both receiver APIs.
	type receiverTestCase struct {
		name         string
		programID    solana.PublicKey
		executorArgs ccipsolutils.SVMExecutorArgs
		isV2         bool
	}
	receiverTestCases := []receiverTestCase{
		{name: "ReceiverV1", programID: receiverV1ProgramID, executorArgs: arbExecutorArgsV1},
		{name: "ReceiverV2", programID: receiverV2ProgramID, executorArgs: arbExecutorArgsV2, isV2: true},
	}
	type pttTestCase struct {
		poolTC     tokenPoolTestCase
		receiverTC receiverTestCase
	}
	pttTestCases := make([]pttTestCase, 0, len(tokenPoolTestCases)*len(receiverTestCases))
	for _, poolTC := range tokenPoolTestCases {
		for _, receiverTC := range receiverTestCases {
			pttTestCases = append(pttTestCases, pttTestCase{poolTC: poolTC, receiverTC: receiverTC})
		}
	}
	for _, tc := range pttTestCases {
		t.Run("PTT/"+tc.poolTC.caseName+"/"+tc.receiverTC.name, func(t *testing.T) {
			combo, err := tokenCombination(tc.poolTC.evm, tc.poolTC.solana, declaredCombos(t, ds, evmSelector, solSelector))
			require.NoError(t, err, "pool pair %q not deployed in this environment", tc.poolTC.caseName)

			remotePool := combo.RemotePoolAddressRef()
			receiver := tc.receiverTC.programID

			solTokenReceiver, _, err := state.FindReceiverTokenAdminPDA(receiver)
			require.NoError(t, err)

			v3Src, err := lib.V3Source(ctx, evmSelector)
			require.NoError(t, err)
			v3Dest, err := lib.V3Destination(ctx, solSelector)
			require.NoError(t, err)

			srcBalReader, ok := v3Src.(cciptestinterfaces.TokenBalanceReader)
			require.True(t, ok, "EVM source should implement TokenBalanceReader")
			dstBalReader, ok := v3Dest.(cciptestinterfaces.TokenBalanceReader)
			require.True(t, ok, "Solana dest should implement TokenBalanceReader")

			evmReg, err := chainreg.GetRegistry().Get(chainsel.FamilyEVM)
			require.NoError(t, err)
			solReg, err := chainreg.GetRegistry().Get(chainsel.FamilySolana)
			require.NoError(t, err)
			srcToken, err := evmReg.AddressResolver.GetToken(ds, evmSelector, combo.LocalPoolAddressRef())
			require.NoError(t, err, "resolve EVM source token")
			destToken, err := solReg.AddressResolver.GetToken(ds, solSelector, remotePool)
			require.NoError(t, err, "resolve Solana dest token")

			srcStart, err := srcBalReader.GetTokenBalance(ctx, evmSender, srcToken)
			require.NoError(t, err)
			dstStart, err := dstBalReader.GetTokenBalance(ctx, protocol.UnknownAddress(solTokenReceiver.Bytes()), destToken)
			require.NoError(t, err)

			_, ccvs, executor, err := devenvutils.ResolveSendAddressesWithReceiverQualifier(ctx, lib, evmSelector, solSelector, devenvcommon.DefaultReceiverQualifier)
			require.NoError(t, err)

			ensureEVMAllowance(ctx, t, evmCLDFChain, ds, evmSelector, combo, evmRouterAddr, sourceAmount)

			// solana receiver v1 targeting message doesn't support finality config
			fin := combo.FinalityConfig()
			if !tc.receiverTC.isV2 {
				fin = protocol.Finality(0)
			}

			sentEvt, _, err := tcapi.SendV3Message(ctx, v3Src, v3Dest,
				cciptestinterfaces.MessageFields{
					Receiver: receiver.Bytes(),
					Data:     []byte("ptt evm2sol"),
					TokenAmount: cciptestinterfaces.TokenAmount{
						Amount:       sourceAmount,
						TokenAddress: srcToken,
					},
				},
				cciptestinterfaces.MessageOptions{
					FinalityConfig: fin,
					Executor:       executor,
					CCVs:           ccvs,
				},
				tcapi.SendArgs{
					ExecutionGasLimit:   devenvutils.MaxComputeUnits,
					ExtraArgsParams:     tc.receiverTC.executorArgs,
					TokenReceiverParams: protocol.UnknownAddress(solTokenReceiver.Bytes()),
					SendOption:          mustEvmSendOptions(t, evmSelector),
				},
			)
			require.NoError(t, err)

			messageKey := cciptestinterfaces.MessageEventKey{MessageID: sentEvt.MessageID}
			_, err = v3Src.ConfirmSendOnSource(ctx, solSelector, messageKey, defaultAssertionTimeout)
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

			execEnv, err := v3Dest.ConfirmExecOnDest(ctx, evmSelector, messageKey, execTimeout(evmSelector))
			require.NoError(t, err)
			require.Equal(t, cciptestinterfaces.ExecutionStateSuccess, execEnv.Event.State, "exec should succeed end-to-end")
			require.Empty(t, execEnv.Event.ReturnData, "offramp emits empty return data by design")
			requireSolanaReceiverEvent(
				ctx, t, solanaImpl, execEnv.TxID, receiver, tc.receiverTC.isV2,
				sentEvt.MessageID, evmSelector, paddedSender,
			)

			srcEnd, err := srcBalReader.GetTokenBalance(ctx, evmSender, srcToken)
			require.NoError(t, err)
			require.Equal(t, new(big.Int).Sub(srcStart, sourceAmount), srcEnd,
				"EVM source balance should drop by the source amount")
			dstEnd, err := dstBalReader.GetTokenBalance(ctx, protocol.UnknownAddress(solTokenReceiver.Bytes()), destToken)
			require.NoError(t, err)
			require.Equal(t, new(big.Int).Add(dstStart, destinationAmount), dstEnd,
				"Solana token receiver ATA should gain the destination amount")

			poolProgram, poolVersion, err := solcommon.GetSolanaTokenPoolProgramAddress(
				ds, solSelector, string(remotePool.Type), remotePool.Qualifier,
			)
			require.NoError(t, err)
			require.NotNil(t, poolVersion, "resolved Solana pool version is nil")
			disc, err := expectedPoolEventDiscriminator(string(remotePool.Type), poolVersion)
			require.NoError(t, err)
			poolEvt, err := solanaImpl.ConfirmTokenPoolEventInTx(
				ctx,
				solana.SignatureFromBytes(execEnv.TxID),
				poolProgram,
				disc,
			)
			require.NoError(t, err)
			normalized, err := NormalizeTokenPoolEvent(poolEvt)
			require.NoError(t, err)
			require.Equal(t, solana.PublicKeyFromBytes(destToken), normalized.Mint, "pool event mint mismatch")
			require.Equal(t, solTokenReceiver, normalized.Recipient, "pool event recipient should be the token receiver")
			require.Equal(t, destinationAmount.Uint64(), normalized.Amount, "pool event amount mismatch")
			if poolVersion.GreaterThanEqual(solcommon.Version2_0_0) {
				require.NotNil(t, normalized.RemoteChainSelector, "v2 pool event should expose the remote chain selector")
				require.Equal(t, evmSelector, *normalized.RemoteChainSelector,
					"pool event should retain the source chain selector")
			}
		})
	}
}
