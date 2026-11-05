// cctp_tokenpool_test.go tests the CCTP token pool programs' core functionality, integrated with the CCTP contracts.
package contracts

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"maps"
	"testing"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/arbitrary_cpi_proxy"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/base_token_pool"
	tp "github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/burnmint_token_pool"
	cctp_message_transmitter "github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/cctp_message_transmitter"
	cctp_message_transmitter_v2 "github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/cctp_message_transmitter_v2"
	cctp_token_messenger_minter "github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/cctp_token_messenger_minter"
	cctp_token_messenger_minter_v2 "github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/cctp_token_messenger_minter_v2"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/cctp_token_pool"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/rmn_remote"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/cctp"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/config"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/testutils"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/tokens"
)

func TestCctpTokenPool(t *testing.T) {
	mockRamp := config.ArbitraryCpiProxyProgram

	admin := solana.MustPrivateKeyFromBase58("4whgxZhpxcArYWzM1iTmokruAzws9YVi2f9M7pWwchQniaFXBr1WGSGXgadeqHtiRooxNiPosdLj2g2ohbtkWtu5")
	anotherAdmin := solana.MustPrivateKeyFromBase58("3T3TwgX851KixNcZ2nJDftvXb5gMckHg8zzonKXpSthdDQEtttP4iY5VwetMRmoJkMDUPqSbrypFHVV2aC9FWHKE")
	user := solana.MustPrivateKeyFromBase58("iVu6VhFc44zTZx6LceYup18rQfxqFvKWFWzJtrNpFcWHCNnbmDfSMYFbDrj9R7RZon4t6YHHmFU8Fh6461PBvfC")
	feeRecipient := solana.MustPrivateKeyFromBase58("55F8j6inTsGz9rQjC2ZJTRAGFUVxSqGCt2uPbHjutFyfetCGST1oo2eV3XJVrjg5jKdkfAo6rTZz3pc515YCZCmw")

	ctx := t.Context()

	// Only the SVM & EVM lanes is registered: CCTP is the sole remaining mock-driven flow and it loops a
	// message back to this chain (v1) or svm<>evm (v2).
	allowedOfframpSvmPDA, err := state.FindAllowedOfframpPDA(config.SvmChainSelector, mockRamp, mockRamp)
	require.NoError(t, err)
	// Only the SVM lane is registered: CCTP is the sole remaining mock-driven flow and it loops a
	// message back to this chain.
	allowedOfframpEvmPDA, err := state.FindAllowedOfframpPDA(config.EvmChainSelector, mockRamp, mockRamp)
	require.NoError(t, err)

	evmPoolAddress, err := hex.DecodeString("A55F731e602d0DCF6bB332fb611Dc424184982D8")
	require.NoError(t, err)

	solanaGoClient := testutils.DeployAllPrograms(t, testutils.PathToAnchorConfig, admin)

	var offrampLookupTable map[solana.PublicKey]solana.PublicKeySlice

	// Common setup — executed directly (not in t.Run) so it always runs regardless of -run filters.
	require.NoError(t, testutils.FundAccounts(ctx, []solana.PrivateKey{admin, anotherAdmin, user, feeRecipient}, solanaGoClient))

	{
		svmIx, err := arbitrary_cpi_proxy.NewAddOfframpInstruction(
			config.SvmChainSelector,
			mockRamp,
			allowedOfframpSvmPDA,
			admin.PublicKey(),
			solana.SystemProgramID,
		)
		require.NoError(t, err)
		evmIx, err := arbitrary_cpi_proxy.NewAddOfframpInstruction(
			config.EvmChainSelector,
			mockRamp,
			allowedOfframpEvmPDA,
			admin.PublicKey(),
			solana.SystemProgramID,
		)
		require.NoError(t, err)

		testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{svmIx, evmIx}, admin, config.DefaultCommitment)
	}

	t.Run("setup: mock offramp lookup table", func(t *testing.T) {
		offrampLookupTableAddr, kErr := common.CreateLookupTable(ctx, solanaGoClient, admin)
		require.NoError(t, kErr)

		// Derive CCTP pool signer PDA for inclusion in the lookup table to reduce tx size
		cctpPoolSigner, _, kErr := state.FindExternalTokenPoolsSignerPDA(config.CctpTokenPoolProgram, mockRamp)
		require.NoError(t, kErr)

		entries := solana.PublicKeySlice{
			mockRamp,
			config.RMNRemoteProgram,
			config.RMNRemoteConfigPDA,
			config.RMNRemoteCursesPDA,
			solana.SystemProgramID,
			// hack just for the test to have it here - it shouldn't be in the table, but our tests are also
			// cursing in the same tx (to avoid concurrency side-effects) and thus need the extra space
			allowedOfframpSvmPDA,
			allowedOfframpEvmPDA,
			// proxy-related entries to keep CCTP transactions under the 1232-byte limit
			config.CctpTokenPoolProgram,
			cctpPoolSigner,
		}

		offrampLookupTable = map[solana.PublicKey]solana.PublicKeySlice{
			offrampLookupTableAddr: entries,
		}

		require.NoError(t, common.ExtendLookupTable(ctx, solanaGoClient, offrampLookupTableAddr, admin, entries))
		require.NoError(t, common.AwaitSlotChange(ctx, solanaGoClient))
	})

	{
		programDataAddr := testutils.GetProgramDataAddress(ctx, t, solanaGoClient, config.RMNRemoteProgram)

		ix, err := rmn_remote.NewInitializeInstruction(
			config.RMNRemoteConfigPDA,
			config.RMNRemoteCursesPDA,
			admin.PublicKey(),
			solana.SystemProgramID,
			config.RMNRemoteProgram,
			programDataAddr,
		)

		require.NoError(t, err)
		result := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)
		require.NotNil(t, result)
	}

	t.Run("CCTP token pool", func(t *testing.T) {
		// do not run in parallel, as it checks the balances of the same users that are also used in the other suites

		usdcMintPriv, kErr := solana.NewRandomPrivateKey()
		require.NoError(t, kErr)
		usdcMint := usdcMintPriv.PublicKey()
		usdcEvm := config.EVMUsdcAddressBytesTrim
		usdcDecimals := uint8(6)

		// Example domain, used as both local and remote domain in v1, to test something similar to a
		// Solana <> Solana transfer. It has to be a valid domain due to used_nonces PDA derivation.
		// As of May 2025, valid domains are 0 to 10 (https://developers.circle.com/stablecoins/supported-domains)
		domain := uint32(5)
		remoteDomainV2 := uint32(1) // what would be AVAX
		require.NotEqual(t, domain, remoteDomainV2, "SourceDomain and DestDomain must be different for CCTPv2 core contracts")

		type v1PDAs struct {
			MessageTransmitter   cctp.MessageTransmitterPDAs
			TokenMessengerMinter cctp.TokenMessengerMinterPDAs
		}
		type v2PDAs struct {
			MessageTransmitter   cctp.MessageTransmitterV2PDAs
			TokenMessengerMinter cctp.TokenMessengerMinterV2PDAs
		}
		pdas := struct {
			v1 v1PDAs
			v2 v2PDAs
		}{
			v1: v1PDAs{
				MessageTransmitter:   cctp.GetMessageTransmitterPDAs(t),
				TokenMessengerMinter: cctp.GetTokenMessengerMinterPDAs(t, domain, usdcMint, usdcMint[:]),
			},
			v2: v2PDAs{
				MessageTransmitter:   cctp.GetMessageTransmitterV2PDAs(t),
				TokenMessengerMinter: cctp.GetTokenMessengerMinterV2PDAs(t, remoteDomainV2, usdcMint, usdcEvm),
			},
		}

		cctpPool := cctp.GetTokenPoolPDAs(t, usdcMint)

		attesters, otherKeys, _ := testutils.GenerateSignersAndTransmitters(t, 1)
		attester := attesters[0]
		attesterSolana := solana.PublicKey(common.ToLeftPadded32Bytes(attester.Address[:]))
		tokenController := otherKeys[0]

		var userATA solana.PublicKey
		var adminATA solana.PublicKey
		var feeRecipientATA solana.PublicKey

		var tpLookupTable map[solana.PublicKey]solana.PublicKeySlice
		var tpLookupTableAddr solana.PublicKey

		t.Run("Setup CCTP", func(t *testing.T) {
			t.Run("CCTP token", func(t *testing.T) {
				// create USDC token
				instructions, err := tokens.CreateToken(ctx, solana.TokenProgramID, usdcMint, admin.PublicKey(), usdcDecimals, solanaGoClient, config.DefaultCommitment)
				require.NoError(t, err)

				// Admin: create associated token accounts & mint
				createAdminI, adminTokenAccount, err := tokens.CreateAssociatedTokenAccount(solana.TokenProgramID, usdcMint, admin.PublicKey(), admin.PublicKey())
				require.NoError(t, err)
				adminATA = adminTokenAccount
				mintToAdminI, err := tokens.MintTo(1000*1e6, solana.TokenProgramID, usdcMint, adminTokenAccount, admin.PublicKey())
				require.NoError(t, err)
				instructions = append(instructions, createAdminI, mintToAdminI)

				// User: create associated token account & mint
				createUserI, userTokenAccount, err := tokens.CreateAssociatedTokenAccount(solana.TokenProgramID, usdcMint, user.PublicKey(), admin.PublicKey())
				require.NoError(t, err)
				userATA = userTokenAccount
				mintToUserI, err := tokens.MintTo(100*1e6, solana.TokenProgramID, usdcMint, userTokenAccount, admin.PublicKey())
				require.NoError(t, err)
				instructions = append(instructions, createUserI, mintToUserI)

				// Fee recipient (dest-side fee collector for fast transfers): create associated token account
				createFeeI, feeRecipientTokenAccount, err := tokens.CreateAssociatedTokenAccount(solana.TokenProgramID, usdcMint, feeRecipient.PublicKey(), admin.PublicKey())
				require.NoError(t, err)
				feeRecipientATA = feeRecipientTokenAccount
				instructions = append(instructions, createFeeI)

				testutils.SendAndConfirm(ctx, t, solanaGoClient, instructions, admin, config.DefaultCommitment, common.AddSigners(usdcMintPriv))

				// validate admin for example
				outDec, outVal, err := tokens.TokenBalance(ctx, solanaGoClient, adminTokenAccount, config.DefaultCommitment)
				require.NoError(t, err)
				require.Equal(t, int(1_000*1e6), outVal)
				require.Equal(t, usdcDecimals, outDec)
			})

			t.Run("CCTP token pool program", func(t *testing.T) {
				t.Run("Create lookup table", func(t *testing.T) {
					tpLookupTableAddr, kErr = common.CreateLookupTable(ctx, solanaGoClient, admin)
					require.NoError(t, kErr)

					entries := solana.PublicKeySlice{
						tpLookupTableAddr,
						solana.SystemProgramID, // placeholder for the token admin registry, as there is none in the mock ramp
						cctpPool.Program,
						cctpPool.State,
						cctpPool.TokenAccount,
						cctpPool.Signer,
						solana.TokenProgramID,
						usdcMint,
						solana.SystemProgramID, // placeholder for the fee token config, as there is none in the mock ramp
						solana.SystemProgramID, // placeholder for the router signer, as there is none in the mock ramp
						// -- CCTP custom entries --
						pdas.v1.MessageTransmitter.MessageTransmitter,
						pdas.v1.TokenMessengerMinter.Program,
						solana.SystemProgramID,
						pdas.v1.MessageTransmitter.Program,
						pdas.v1.TokenMessengerMinter.TokenMessenger,
						pdas.v1.TokenMessengerMinter.TokenMinter,
						pdas.v1.TokenMessengerMinter.LocalToken,
						pdas.v1.TokenMessengerMinter.EventAuthority,
					}

					tpLookupTable = map[solana.PublicKey]solana.PublicKeySlice{
						tpLookupTableAddr: entries,
					}

					require.NoError(t, common.ExtendLookupTable(ctx, solanaGoClient, tpLookupTableAddr, admin, entries))

					// Extend with CCTP chain config to keep proxy transactions under the 1232-byte limit
					extraEntries := solana.PublicKeySlice{cctpPool.Svm.ChainConfig}
					require.NoError(t, common.ExtendLookupTable(ctx, solanaGoClient, tpLookupTableAddr, admin, extraEntries))
					tpLookupTable[tpLookupTableAddr] = append(tpLookupTable[tpLookupTableAddr], extraEntries...)

					require.NoError(t, common.AwaitSlotChange(ctx, solanaGoClient))
				})

				t.Run("Initialize", func(t *testing.T) {
					programDataAddr := testutils.GetProgramDataAddress(ctx, t, solanaGoClient, config.CctpTokenPoolProgram)

					instructions := []solana.Instruction{} //nolint:prealloc // not perf-sensitive, and good to keep the size dynamic as we add/remove ixs

					poolInitI, err := cctp_token_pool.NewInitializeInstruction(
						mockRamp,
						config.RMNRemoteProgram,
						cctpPool.State,
						usdcMint,
						admin.PublicKey(),
						solana.SystemProgramID,
						cctpPool.Program,
						programDataAddr,
						cctpPool.GlobalConfig,
					)
					require.NoError(t, err)
					instructions = append(instructions, poolInitI)

					// set pool config
					ixConfigureSvm, err := cctp_token_pool.NewInitChainRemoteConfigInstruction(
						config.SvmChainSelector,
						usdcMint,
						cctp_token_pool.RemoteConfig{
							TokenAddress: cctp_token_pool.RemoteAddress{
								Address: usdcMint.Bytes(),
							},
							Decimals:      usdcDecimals,
							PoolAddresses: []cctp_token_pool.RemoteAddress{},
						},
						cctpPool.State,
						cctpPool.Svm.ChainConfig,
						admin.PublicKey(),
						solana.SystemProgramID,
					)
					require.NoError(t, err)
					instructions = append(instructions, ixConfigureSvm)

					ixCctpConfigureSvm, err := cctp_token_pool.NewEditChainRemoteConfigCctpInstruction(
						config.SvmChainSelector,
						usdcMint,
						cctp_token_pool.CctpChain{
							DomainId:          domain,
							DestinationCaller: cctpPool.Signer, // as it is svm<->svm, the caller is the pool signer
						},
						cctpPool.State,
						cctpPool.Svm.ChainConfig,
						admin.PublicKey(),
					)
					require.NoError(t, err)
					instructions = append(instructions, ixCctpConfigureSvm)

					ixAppendSvm, err := cctp_token_pool.NewAppendRemotePoolAddressesInstruction(
						config.SvmChainSelector,
						usdcMint,
						[]cctp_token_pool.RemoteAddress{{Address: cctpPool.Signer.Bytes()}}, // when the source is Solana, the pool is identified by its signer
						cctpPool.State,
						cctpPool.Svm.ChainConfig,
						admin.PublicKey(),
						solana.SystemProgramID,
					)
					require.NoError(t, err)
					instructions = append(instructions, ixAppendSvm)

					// set rate limit
					ixRates, err := cctp_token_pool.NewSetChainRateLimitInstruction(
						config.SvmChainSelector,
						usdcMint,
						cctp_token_pool.RateLimitConfig{
							Enabled:  true,
							Capacity: 1e5, // 0.1 USDC, drains in one offramp
							Rate:     1,   // slow refill
						},
						cctp_token_pool.RateLimitConfig{
							Enabled:  true,
							Capacity: 1e5, // 0.1 USDC, drains in one onramp
							Rate:     1,   // slow refill
						},
						cctpPool.State,
						cctpPool.Svm.ChainConfig,
						admin.PublicKey(),
					)
					require.NoError(t, err)
					instructions = append(instructions, ixRates)

					// create pool token account
					createP, poolTokenAccount, err := tokens.CreateAssociatedTokenAccount(solana.TokenProgramID, usdcMint, cctpPool.Signer, admin.PublicKey())
					require.NoError(t, err)
					require.Equal(t, poolTokenAccount, cctpPool.TokenAccount)
					instructions = append(instructions, createP)

					// submit tx with all instructions
					res := testutils.SendAndConfirm(ctx, t, solanaGoClient, instructions, admin, config.DefaultCommitment)
					require.NotNil(t, res)

					// A zero destination caller would let anyone consume the CCTP message on the destination.
					ixZeroCaller, err := cctp_token_pool.NewEditChainRemoteConfigCctpInstruction(
						config.SvmChainSelector,
						usdcMint,
						cctp_token_pool.CctpChain{DomainId: domain},
						cctpPool.State,
						cctpPool.Svm.ChainConfig,
						admin.PublicKey(),
					)
					require.NoError(t, err)
					testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ixZeroCaller}, admin, config.DefaultCommitment, []string{"Error Code: CctpAllowedCallersInvalidZero"})

					// validate state
					var configAccount cctp_token_pool.State
					require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, cctpPool.State, config.DefaultCommitment, &configAccount))
					require.Equal(t, cctpPool.TokenAccount, configAccount.Config.PoolTokenAccount)

					// validate events
					eventConfigured, err := common.ParseEventFromLogs(res.Meta.LogMessages, base_token_pool.ParseEvent_RemoteChainConfigured)
					require.NoError(t, err)
					require.Equal(t, config.SvmChainSelector, eventConfigured.ChainSelector)
					require.Empty(t, eventConfigured.PoolAddresses)
					require.Empty(t, eventConfigured.PreviousPoolAddresses)
					require.Equal(t, usdcMint.Bytes(), eventConfigured.Token.Address)
					require.Empty(t, eventConfigured.PreviousToken.Address)
					require.Equal(t, usdcMint, eventConfigured.Mint)

					eventAppended, err := common.ParseEventFromLogs(res.Meta.LogMessages, base_token_pool.ParseEvent_RemotePoolsAppended)
					require.NoError(t, err)
					require.Equal(t, config.SvmChainSelector, eventAppended.ChainSelector)
					require.Len(t, eventAppended.PoolAddresses, 1)
					require.Equal(t, cctpPool.Signer.Bytes(), eventAppended.PoolAddresses[0].Address)
					require.Empty(t, eventAppended.PreviousPoolAddresses)
					require.Equal(t, usdcMint, eventAppended.Mint)

					eventCctpEdit, err := common.ParseEventFromLogs(res.Meta.LogMessages, cctp_token_pool.ParseEvent_RemoteChainCctpConfigChanged)
					require.NoError(t, err)
					require.Equal(t, domain, eventCctpEdit.Config.DomainId)
					require.Equal(t, cctpPool.Signer, eventCctpEdit.Config.DestinationCaller)

					eventRateLimit, err := common.ParseEventFromLogs(res.Meta.LogMessages, base_token_pool.ParseEvent_RateLimitConfigured)
					require.NoError(t, err)
					require.Equal(t, config.SvmChainSelector, eventRateLimit.ChainSelector)
					require.True(t, eventRateLimit.InboundRateLimit.Enabled)
					require.Equal(t, uint64(1e5), eventRateLimit.InboundRateLimit.Capacity)
					require.Equal(t, uint64(1), eventRateLimit.InboundRateLimit.Rate)
					require.True(t, eventRateLimit.OutboundRateLimit.Enabled)
					require.Equal(t, uint64(1e5), eventRateLimit.OutboundRateLimit.Capacity)
					require.Equal(t, uint64(1), eventRateLimit.OutboundRateLimit.Rate)
					require.Equal(t, usdcMint, eventRateLimit.Mint)

					// Configure the EVM lane for v2
					instructions = []solana.Instruction{} //nolint:prealloc // not perf-sensitive, and good to keep the size dynamic as we add/remove ixs

					ixConfigureEvm, err := cctp_token_pool.NewInitChainRemoteConfigInstruction(
						config.EvmChainSelector,
						usdcMint,
						cctp_token_pool.RemoteConfig{
							TokenAddress: cctp_token_pool.RemoteAddress{
								Address: usdcEvm,
							},
							Decimals:      usdcDecimals,
							PoolAddresses: []cctp_token_pool.RemoteAddress{},
						},
						cctpPool.State,
						cctpPool.Evm.ChainConfig,
						admin.PublicKey(),
						solana.SystemProgramID,
					)
					require.NoError(t, err)
					instructions = append(instructions, ixConfigureEvm)

					ixCctpConfigureEvm, err := cctp_token_pool.NewEditChainRemoteConfigCctpInstruction(
						config.EvmChainSelector,
						usdcMint,
						cctp_token_pool.CctpChain{
							DomainId:          remoteDomainV2,
							DestinationCaller: cctpPool.Signer, // as it is svm<->svm, the caller is the pool signer
						},
						cctpPool.State,
						cctpPool.Evm.ChainConfig,
						admin.PublicKey(),
					)
					require.NoError(t, err)
					instructions = append(instructions, ixCctpConfigureEvm)

					ixAppendEvm, err := cctp_token_pool.NewAppendRemotePoolAddressesInstruction(
						config.EvmChainSelector,
						usdcMint,
						[]cctp_token_pool.RemoteAddress{{Address: evmPoolAddress}},
						cctpPool.State,
						cctpPool.Evm.ChainConfig,
						admin.PublicKey(),
						solana.SystemProgramID,
					)
					require.NoError(t, err)
					instructions = append(instructions, ixAppendEvm)

					ixRates, err = cctp_token_pool.NewSetChainRateLimitInstruction(
						config.EvmChainSelector,
						usdcMint,
						cctp_token_pool.RateLimitConfig{
							Enabled:  true,
							Capacity: 1e5, // 0.1 USDC, drains in one offramp
							Rate:     1,   // slow refill
						},
						cctp_token_pool.RateLimitConfig{
							Enabled:  true,
							Capacity: 1e5, // 0.1 USDC, drains in one onramp
							Rate:     1,   // slow refill
						},
						cctpPool.State,
						cctpPool.Evm.ChainConfig,
						admin.PublicKey(),
					)
					require.NoError(t, err)
					instructions = append(instructions, ixRates)

					// submit tx with all instructions
					res = testutils.SendAndConfirm(ctx, t, solanaGoClient, instructions, admin, config.DefaultCommitment)
					require.NotNil(t, res)
				})
			})

			t.Run("CCTPv1", func(t *testing.T) {
				t.Run("CCTP Message Transmitter program", func(t *testing.T) {
					programDataAddr := testutils.GetProgramDataAddress(ctx, t, solanaGoClient, pdas.v1.MessageTransmitter.Program)

					ix, err := cctp_message_transmitter.NewInitializeInstruction(
						cctp_message_transmitter.InitializeParams{
							LocalDomain:        domain,
							Attester:           attesterSolana,
							MaxMessageBodySize: 1234,
							Version:            0,
						},
						admin.PublicKey(),
						admin.PublicKey(),
						pdas.v1.MessageTransmitter.MessageTransmitter,
						programDataAddr,
						pdas.v1.MessageTransmitter.Program,
						solana.SystemProgramID,
						pdas.v1.MessageTransmitter.EventAuthority,
						pdas.v1.MessageTransmitter.Program,
					)
					require.NoError(t, err)

					testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)
				})

				t.Run("CCTP Token Messenger Minter program", func(t *testing.T) {
					t.Run("Initialize", func(t *testing.T) {
						programDataAddr := testutils.GetProgramDataAddress(ctx, t, solanaGoClient, pdas.v1.TokenMessengerMinter.Program)

						ix, err := cctp_token_messenger_minter.NewInitializeInstruction(
							cctp_token_messenger_minter.InitializeParams{
								TokenController:         tokenController.PublicKey(),
								LocalMessageTransmitter: pdas.v1.MessageTransmitter.Program,
								MessageBodyVersion:      0,
							},
							admin.PublicKey(),
							admin.PublicKey(),
							pdas.v1.TokenMessengerMinter.AuthorityPda,
							pdas.v1.TokenMessengerMinter.TokenMessenger,
							pdas.v1.TokenMessengerMinter.TokenMinter,
							programDataAddr,
							pdas.v1.TokenMessengerMinter.Program,
							solana.SystemProgramID,
							pdas.v1.TokenMessengerMinter.EventAuthority,
							pdas.v1.TokenMessengerMinter.Program,
						)
						require.NoError(t, err)

						testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)
					})

					t.Run("Add remote token messenger", func(t *testing.T) {
						ix, err := cctp_token_messenger_minter.NewAddRemoteTokenMessengerInstruction(
							cctp_token_messenger_minter.AddRemoteTokenMessengerParams{
								Domain:         domain,
								TokenMessenger: pdas.v1.TokenMessengerMinter.Program,
							},
							admin.PublicKey(),
							admin.PublicKey(),
							pdas.v1.TokenMessengerMinter.TokenMessenger,
							pdas.v1.TokenMessengerMinter.RemoteTokenMessenger,
							solana.SystemProgramID,
							pdas.v1.TokenMessengerMinter.EventAuthority,
							pdas.v1.TokenMessengerMinter.Program,
						)
						require.NoError(t, err)

						testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)
					})

					t.Run("Add local token", func(t *testing.T) {
						ix, err := cctp_token_messenger_minter.NewAddLocalTokenInstruction(
							cctp_token_messenger_minter.AddLocalTokenParams{},
							admin.PublicKey(),
							tokenController.PublicKey(),
							pdas.v1.TokenMessengerMinter.TokenMinter,
							pdas.v1.TokenMessengerMinter.LocalToken,
							pdas.v1.TokenMessengerMinter.CustodyTokenAccount,
							usdcMint,
							solana.TokenProgramID,
							solana.SystemProgramID,
							pdas.v1.TokenMessengerMinter.EventAuthority,
							pdas.v1.TokenMessengerMinter.Program,
						)
						require.NoError(t, err)

						testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment, common.AddSigners(tokenController))
					})

					t.Run("Link token pair", func(t *testing.T) {
						ix, err := cctp_token_messenger_minter.NewLinkTokenPairInstruction(
							cctp_token_messenger_minter.LinkTokenPairParams{
								LocalToken:   pdas.v1.TokenMessengerMinter.LocalToken,
								RemoteDomain: domain,
								RemoteToken:  usdcMint, // using Solana as remote domain, so using USDC mint as remote token
							},
							admin.PublicKey(),
							tokenController.PublicKey(),
							pdas.v1.TokenMessengerMinter.TokenMinter,
							pdas.v1.TokenMessengerMinter.TokenPair,
							solana.SystemProgramID,
							pdas.v1.TokenMessengerMinter.EventAuthority,
							pdas.v1.TokenMessengerMinter.Program,
						)
						require.NoError(t, err)

						testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment, common.AddSigners(tokenController))
					})

					t.Run("Set max burn amount per message", func(t *testing.T) {
						ix, err := cctp_token_messenger_minter.NewSetMaxBurnAmountPerMessageInstruction(
							cctp_token_messenger_minter.SetMaxBurnAmountPerMessageParams{
								BurnLimitPerMessage: 2 * 1e6, // 2 USDC, as it uses 6 decimals
							},
							tokenController.PublicKey(),
							pdas.v1.TokenMessengerMinter.TokenMinter,
							pdas.v1.TokenMessengerMinter.LocalToken,
							pdas.v1.TokenMessengerMinter.EventAuthority,
							pdas.v1.TokenMessengerMinter.Program,
						)
						require.NoError(t, err)

						testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment, common.AddSigners(tokenController))
					})
				})

				t.Run("CCTPv1 sanity checks", func(t *testing.T) {
					messageAmount := uint64(1123456) // 1.123456 USDC, as it uses 6 decimals

					var messageSent cctp_message_transmitter.MessageSent
					messageSentEventKeypair, err := solana.NewRandomPrivateKey()
					require.NoError(t, err)

					var attestation []byte

					t.Run("DepositForBurnWithCaller", func(t *testing.T) {
						ix, err := cctp_token_messenger_minter.NewDepositForBurnWithCallerInstruction(
							cctp_token_messenger_minter.DepositForBurnWithCallerParams{
								Amount:            messageAmount,
								DestinationDomain: domain,
								MintRecipient:     adminATA,
								DestinationCaller: admin.PublicKey(),
							},
							user.PublicKey(),
							user.PublicKey(),
							pdas.v1.TokenMessengerMinter.AuthorityPda,
							userATA,
							pdas.v1.MessageTransmitter.MessageTransmitter,
							pdas.v1.TokenMessengerMinter.TokenMessenger,
							pdas.v1.TokenMessengerMinter.RemoteTokenMessenger,
							pdas.v1.TokenMessengerMinter.TokenMinter,
							pdas.v1.TokenMessengerMinter.LocalToken,
							usdcMint,
							messageSentEventKeypair.PublicKey(),
							pdas.v1.MessageTransmitter.Program,
							pdas.v1.TokenMessengerMinter.Program,
							solana.TokenProgramID,
							solana.SystemProgramID,
							pdas.v1.TokenMessengerMinter.EventAuthority,
							pdas.v1.TokenMessengerMinter.Program,
						)
						require.NoError(t, err)

						result := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, user, config.DefaultCommitment, common.AddSigners(messageSentEventKeypair))

						returnedNonce, err := common.ExtractTypedReturnValue(ctx, result.Meta.LogMessages, pdas.v1.TokenMessengerMinter.Program.String(), binary.LittleEndian.Uint64)
						require.NoError(t, err)
						t.Log("Nonce:", returnedNonce)

						require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, messageSentEventKeypair.PublicKey(), config.DefaultCommitment, &messageSent))
						t.Log("Message Sent Event Account Data:", messageSent)
						t.Log("Message Sent Event Bytes (hex):", hex.EncodeToString(messageSent.Message))
					})

					t.Run("ReceiveMessage", func(t *testing.T) {
						t.Run("Fund custody token account", func(t *testing.T) {
							ix, err := tokens.MintTo(messageAmount, solana.TokenProgramID, usdcMint, pdas.v1.TokenMessengerMinter.CustodyTokenAccount, admin.PublicKey())
							require.NoError(t, err)
							testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)

							decimals, amount, err := tokens.TokenBalance(ctx, solanaGoClient, pdas.v1.TokenMessengerMinter.CustodyTokenAccount, config.DefaultCommitment)
							require.NoError(t, err)
							require.Equal(t, messageAmount, uint64(amount)) //nolint:gosec // G115
							require.Equal(t, usdcDecimals, decimals)
						})

						t.Run("Actually receive message", func(t *testing.T) {
							_, initial, err := tokens.TokenBalance(ctx, solanaGoClient, adminATA, config.DefaultCommitment)
							require.NoError(t, err)

							usedNoncesPDA := cctp.GetUsedNoncesPDA(t, messageSent.Message)

							attestation, err = testutils.AttestCCTP(messageSent.Message, attesters)
							t.Log("Attestation:", hex.EncodeToString(attestation))
							require.NoError(t, err)

							raw, err := cctp_message_transmitter.NewReceiveMessageInstruction(
								cctp_message_transmitter.ReceiveMessageParams{
									Message:     messageSent.Message,
									Attestation: attestation,
								},
								admin.PublicKey(), // payer
								admin.PublicKey(), // caller
								pdas.v1.MessageTransmitter.AuthorityPda,
								pdas.v1.MessageTransmitter.MessageTransmitter,
								usedNoncesPDA,
								pdas.v1.TokenMessengerMinter.Program, // receiver
								solana.SystemProgramID,
								pdas.v1.MessageTransmitter.EventAuthority,
								pdas.v1.MessageTransmitter.Program,
							)
							require.NoError(t, err)
							genericRaw, _ := raw.(*solana.GenericInstruction)

							genericRaw.AccountValues = append(genericRaw.AccountValues,
								solana.Meta(pdas.v1.TokenMessengerMinter.TokenMessenger),
								solana.Meta(pdas.v1.TokenMessengerMinter.RemoteTokenMessenger),
								solana.Meta(pdas.v1.TokenMessengerMinter.TokenMinter).WRITE(),
								solana.Meta(pdas.v1.TokenMessengerMinter.LocalToken).WRITE(),
								solana.Meta(pdas.v1.TokenMessengerMinter.TokenPair),
								solana.Meta(adminATA).WRITE(), // user receiving the USDC
								solana.Meta(pdas.v1.TokenMessengerMinter.CustodyTokenAccount).WRITE(),
								solana.Meta(solana.TokenProgramID),
								solana.Meta(pdas.v1.TokenMessengerMinter.EventAuthority),
								solana.Meta(pdas.v1.TokenMessengerMinter.Program))

							testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{raw}, admin, config.DefaultCommitment)

							_, final, err := tokens.TokenBalance(ctx, solanaGoClient, adminATA, config.DefaultCommitment)
							require.NoError(t, err)

							require.Equal(t, uint64(initial)+messageAmount, uint64(final), "Admin should have received the USDC after receiving the message") //nolint:gosec // G115
							t.Log("Final balance of admin's USDC ATA:", final)
						})
					})

					t.Run("Reclaim event account rent", func(t *testing.T) {
						initial, err := solanaGoClient.GetAccountInfoWithOpts(ctx, user.PublicKey(), &rpc.GetAccountInfoOpts{Commitment: config.DefaultCommitment})
						require.NoError(t, err)

						messageEventAccount, err := solanaGoClient.GetAccountInfoWithOpts(ctx, messageSentEventKeypair.PublicKey(), &rpc.GetAccountInfoOpts{Commitment: config.DefaultCommitment})
						require.NoError(t, err)

						reclaimIx, err := cctp_message_transmitter.NewReclaimEventAccountInstruction(
							cctp_message_transmitter.ReclaimEventAccountParams{
								Attestation: attestation,
							},
							user.PublicKey(),
							pdas.v1.MessageTransmitter.MessageTransmitter,
							messageSentEventKeypair.PublicKey(),
						)
						require.NoError(t, err)

						result := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{reclaimIx}, user, config.DefaultCommitment)
						require.NotNil(t, result)

						final, err := solanaGoClient.GetAccountInfoWithOpts(ctx, user.PublicKey(), &rpc.GetAccountInfoOpts{Commitment: config.DefaultCommitment})
						require.NoError(t, err)

						require.Equal(t, initial.Value.Lamports-result.Meta.Fee+messageEventAccount.Value.Lamports, final.Value.Lamports)
					})
				})
			})

			t.Run("CCTPv2", func(t *testing.T) {
				t.Run("CCTP Message Transmitter V2 program", func(t *testing.T) {
					programDataAddr := testutils.GetProgramDataAddress(ctx, t, solanaGoClient, pdas.v2.MessageTransmitter.Program)

					ix, err := cctp_message_transmitter_v2.NewInitializeInstruction(
						// Values based on https://explorer.solana.com/address/W1k5ijkaSTo5iA5zChNpfzcy796fLhkBxfmJuR8W8HU/anchor-account
						cctp_message_transmitter_v2.InitializeParams{
							LocalDomain:        domain,
							Attester:           attesterSolana, // this is just for theses tests
							MaxMessageBodySize: 8192,
							Version:            1,
						},
						admin.PublicKey(),
						admin.PublicKey(),
						pdas.v2.MessageTransmitter.MessageTransmitter,
						programDataAddr,
						pdas.v2.MessageTransmitter.Program,
						solana.SystemProgramID,
						pdas.v2.MessageTransmitter.EventAuthority,
						pdas.v2.MessageTransmitter.Program,
					)
					require.NoError(t, err)

					testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)
				})

				t.Run("CCTP Token Messenger Minter V2 program", func(t *testing.T) {
					t.Run("Initialize", func(t *testing.T) {
						programDataAddr := testutils.GetProgramDataAddress(ctx, t, solanaGoClient, pdas.v2.TokenMessengerMinter.Program)

						ix, err := cctp_token_messenger_minter_v2.NewInitializeInstruction(
							// Values based on https://explorer.solana.com/address/AawthJCGRmggpfv9MMWV6Jmo9cue4gL9wUZgRBShg58W/anchor-account
							cctp_token_messenger_minter_v2.InitializeParams{
								TokenController:    tokenController.PublicKey(),
								MinFee:             0,
								MessageBodyVersion: 1,
								FeeRecipient:       feeRecipient.PublicKey(),
								// the following addresses are set to a stub value in this test we don't care about it
								// but they can't be zero, as the program checks for that
								Denylister:       solana.SysVarClockPubkey,
								MinFeeController: solana.SysVarClockPubkey,
							},
							admin.PublicKey(),
							admin.PublicKey(),
							pdas.v2.TokenMessengerMinter.AuthorityPda,
							pdas.v2.TokenMessengerMinter.TokenMessenger,
							pdas.v2.TokenMessengerMinter.TokenMinter,
							programDataAddr,
							pdas.v2.TokenMessengerMinter.Program,
							solana.SystemProgramID,
							pdas.v2.TokenMessengerMinter.EventAuthority,
							pdas.v2.TokenMessengerMinter.Program,
						)
						require.NoError(t, err)

						testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)
					})

					t.Run("Add remote token messenger", func(t *testing.T) {
						ix, err := cctp_token_messenger_minter_v2.NewAddRemoteTokenMessengerInstruction(
							cctp_token_messenger_minter_v2.AddRemoteTokenMessengerParams{
								Domain:         remoteDomainV2,
								TokenMessenger: pdas.v2.TokenMessengerMinter.Program,
							},
							admin.PublicKey(),
							admin.PublicKey(),
							pdas.v2.TokenMessengerMinter.TokenMessenger,
							pdas.v2.TokenMessengerMinter.RemoteTokenMessenger,
							solana.SystemProgramID,
							pdas.v2.TokenMessengerMinter.EventAuthority,
							pdas.v2.TokenMessengerMinter.Program,
						)
						require.NoError(t, err)

						testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)
					})

					t.Run("Add local token", func(t *testing.T) {
						ix, err := cctp_token_messenger_minter_v2.NewAddLocalTokenInstruction(
							cctp_token_messenger_minter_v2.AddLocalTokenParams{},
							admin.PublicKey(),
							tokenController.PublicKey(),
							pdas.v2.TokenMessengerMinter.TokenMinter,
							pdas.v2.TokenMessengerMinter.LocalToken,
							pdas.v2.TokenMessengerMinter.CustodyTokenAccount,
							usdcMint,
							solana.TokenProgramID,
							solana.SystemProgramID,
							pdas.v2.TokenMessengerMinter.EventAuthority,
							pdas.v2.TokenMessengerMinter.Program,
						)
						require.NoError(t, err)

						testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment, common.AddSigners(tokenController))
					})

					t.Run("Link token pair", func(t *testing.T) {
						ix, err := cctp_token_messenger_minter_v2.NewLinkTokenPairInstruction(
							cctp_token_messenger_minter_v2.LinkTokenPairParams{
								LocalToken:   pdas.v2.TokenMessengerMinter.LocalToken,
								RemoteDomain: remoteDomainV2,
								RemoteToken:  solana.PublicKey(common.ToLeftPadded32Bytes(usdcEvm)),
							},
							admin.PublicKey(),
							tokenController.PublicKey(),
							pdas.v2.TokenMessengerMinter.TokenMinter,
							pdas.v2.TokenMessengerMinter.TokenPair,
							solana.SystemProgramID,
							pdas.v2.TokenMessengerMinter.EventAuthority,
							pdas.v2.TokenMessengerMinter.Program,
						)
						require.NoError(t, err)

						testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment, common.AddSigners(tokenController))
					})

					t.Run("Set max burn amount per message", func(t *testing.T) {
						ix, err := cctp_token_messenger_minter_v2.NewSetMaxBurnAmountPerMessageInstruction(
							cctp_token_messenger_minter_v2.SetMaxBurnAmountPerMessageParams{
								BurnLimitPerMessage: 2 * 1e6, // 2 USDC, as it uses 6 decimals
							},
							tokenController.PublicKey(),
							pdas.v2.TokenMessengerMinter.TokenMinter,
							pdas.v2.TokenMessengerMinter.LocalToken,
							pdas.v2.TokenMessengerMinter.EventAuthority,
							pdas.v2.TokenMessengerMinter.Program,
						)
						require.NoError(t, err)

						testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment, common.AddSigners(tokenController))
					})
				})

				t.Run("CCTPv2 sanity checks", func(t *testing.T) {
					messageAmount := uint64(1123456)     // 1.123456 USDC, as it uses 6 decimals
					maxFee := uint64(0)                  // 0 for standard transfer, as it has no destination fees
					minFinalityThreshold := uint32(2000) // standard transfer
					hookData := []byte("something")      // arbitrary hook data for testing

					u64ToBytes32 := func(n uint64) [32]byte {
						var b [32]byte
						binary.BigEndian.PutUint64(b[24:], n)
						return b
					}

					// test nonce value copied from
					// https://github.com/circlefin/solana-cctp-contracts/blob/ec16e95d28ee47f7832df4203ae07b5981d146fc/programs/v2/tests/token_messenger_minter/tests.ts#L45
					nonce, err := hex.DecodeString("a87ca1348591db5efaafe71d20f08bc97f4ca01c2dec23a94736a0bb9f21c3f0")
					require.NoError(t, err)

					cctpMessage := cctp.Message{
						Version:                   1,
						SourceDomain:              remoteDomainV2,
						DestinationDomain:         domain,
						Nonce:                     [32]byte(nonce),
						Sender:                    pdas.v2.TokenMessengerMinter.Program,
						Recipient:                 pdas.v2.TokenMessengerMinter.Program,
						DestinationCaller:         admin.PublicKey(),
						MinFinalityThreshold:      minFinalityThreshold,
						FinalityThresholdExecuted: minFinalityThreshold + 1,
						Body: cctp.MessageBody{
							Version:         1,
							BurnToken:       common.ToLeftPadded32Bytes(usdcEvm),
							MintRecipient:   adminATA,
							Amount:          u64ToBytes32(messageAmount),
							MessageSender:   user.PublicKey(),
							MaxFee:          u64ToBytes32(maxFee),
							FeeExecuted:     u64ToBytes32(maxFee),
							ExpirationBlock: u64ToBytes32(0),
							HookData:        hookData,
						},
					}
					messageBytes := cctpMessage.Serialize()

					var burnTxSig string

					var messageSent cctp_message_transmitter_v2.MessageSent
					messageSentEventKeypair, err := solana.NewRandomPrivateKey()
					require.NoError(t, err)

					var attestation []byte

					t.Run("DepositForBurnWithHook", func(t *testing.T) {
						ix, err := cctp_token_messenger_minter_v2.NewDepositForBurnWithHookInstruction(
							cctp_token_messenger_minter_v2.DepositForBurnWithHookParams{
								Amount:               messageAmount,
								DestinationDomain:    remoteDomainV2,
								MintRecipient:        adminATA,
								DestinationCaller:    admin.PublicKey(),
								MaxFee:               maxFee,
								MinFinalityThreshold: minFinalityThreshold,
								HookData:             hookData,
							},
							user.PublicKey(),
							user.PublicKey(),
							pdas.v2.TokenMessengerMinter.AuthorityPda,
							userATA,
							cctp.GetDenylistV2PDA(t, user.PublicKey()),
							pdas.v2.MessageTransmitter.MessageTransmitter,
							pdas.v2.TokenMessengerMinter.TokenMessenger,
							pdas.v2.TokenMessengerMinter.RemoteTokenMessenger,
							pdas.v2.TokenMessengerMinter.TokenMinter,
							pdas.v2.TokenMessengerMinter.LocalToken,
							usdcMint,
							messageSentEventKeypair.PublicKey(),
							pdas.v2.MessageTransmitter.Program,
							pdas.v2.TokenMessengerMinter.Program,
							solana.TokenProgramID,
							solana.SystemProgramID,
							pdas.v2.TokenMessengerMinter.EventAuthority,
							pdas.v2.TokenMessengerMinter.Program,
						)
						require.NoError(t, err)
						// deposit_for_burn_with_hook CPIs into the message transmitter's send_message,
						// whose CPI event needs the transmitter's event authority + program appended.
						genericIx, ok := ix.(*solana.GenericInstruction)
						require.True(t, ok)
						genericIx.AccountValues = append(genericIx.AccountValues,
							solana.Meta(pdas.v2.MessageTransmitter.EventAuthority),
							solana.Meta(pdas.v2.MessageTransmitter.Program),
						)

						result := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{genericIx}, user, config.DefaultCommitment, common.AddSigners(messageSentEventKeypair))
						require.NotNil(t, result)
						tx, err := result.Transaction.GetTransaction()
						require.NoError(t, err)
						burnTxSig = tx.Signatures[0].String()
						t.Log("Burn transaction signature:", burnTxSig)

						require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient,
							messageSentEventKeypair.PublicKey(), config.DefaultCommitment, &messageSent))
						t.Logf("MessageSent: rentPayer=%s createdAt=%d message=%s",
							messageSent.RentPayer, messageSent.CreatedAt, hex.EncodeToString(messageSent.Message))

						require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, messageSentEventKeypair.PublicKey(), config.DefaultCommitment, &messageSent))
						t.Log("Message Sent Event Account Data:", messageSent)
						t.Log("Message Sent Event Bytes (hex):", hex.EncodeToString(messageSent.Message))
					})

					t.Run("ReceiveMessage", func(t *testing.T) {
						t.Run("Fund custody token account", func(t *testing.T) {
							ix, err := tokens.MintTo(messageAmount, solana.TokenProgramID, usdcMint, pdas.v2.TokenMessengerMinter.CustodyTokenAccount, admin.PublicKey())
							require.NoError(t, err)
							testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)

							decimals, amount, err := tokens.TokenBalance(ctx, solanaGoClient, pdas.v2.TokenMessengerMinter.CustodyTokenAccount, config.DefaultCommitment)
							require.NoError(t, err)
							require.Equal(t, messageAmount, uint64(amount)) //nolint:gosec // G115
							require.Equal(t, usdcDecimals, decimals)
						})

						t.Run("Actually receive message", func(t *testing.T) {
							_, initial, err := tokens.TokenBalance(ctx, solanaGoClient, adminATA, config.DefaultCommitment)
							require.NoError(t, err)

							attestation, err = testutils.AttestCCTP(messageBytes, attesters)
							t.Log("Attestation:", hex.EncodeToString(attestation))
							require.NoError(t, err)

							raw, err := cctp_message_transmitter_v2.NewReceiveMessageInstruction(
								cctp_message_transmitter_v2.ReceiveMessageParams{
									Message:     messageBytes,
									Attestation: attestation,
								},
								admin.PublicKey(), // payer
								admin.PublicKey(), // caller
								pdas.v2.MessageTransmitter.AuthorityPda,
								pdas.v2.MessageTransmitter.MessageTransmitter,
								cctp.GetUsedNonceV2PDA(t, nonce),
								pdas.v2.TokenMessengerMinter.Program, // receiver
								solana.SystemProgramID,
								pdas.v2.MessageTransmitter.EventAuthority,
								pdas.v2.MessageTransmitter.Program,
							)
							require.NoError(t, err)
							genericRaw, _ := raw.(*solana.GenericInstruction)

							genericRaw.AccountValues = append(genericRaw.AccountValues,
								solana.Meta(pdas.v2.TokenMessengerMinter.TokenMessenger),
								solana.Meta(pdas.v2.TokenMessengerMinter.RemoteTokenMessenger),
								solana.Meta(pdas.v2.TokenMessengerMinter.TokenMinter),
								solana.Meta(pdas.v2.TokenMessengerMinter.LocalToken).WRITE(),
								solana.Meta(pdas.v2.TokenMessengerMinter.TokenPair),
								solana.Meta(feeRecipientATA).WRITE(),
								solana.Meta(adminATA).WRITE(), // user receiving the USDC
								solana.Meta(pdas.v2.TokenMessengerMinter.CustodyTokenAccount).WRITE(),
								solana.Meta(solana.TokenProgramID),
								solana.Meta(pdas.v2.TokenMessengerMinter.EventAuthority),
								solana.Meta(pdas.v2.TokenMessengerMinter.Program))

							testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{raw}, admin, config.DefaultCommitment)

							_, final, err := tokens.TokenBalance(ctx, solanaGoClient, adminATA, config.DefaultCommitment)
							require.NoError(t, err)

							require.Equal(t, uint64(initial)+messageAmount, uint64(final), "Admin should have received the USDC after receiving the message") //nolint:gosec // G115
							t.Log("Final balance of admin's USDC ATA:", final)
						})
					})

					t.Run("Reclaim event account rent", func(t *testing.T) {
						// CCTPv2 hardcodes a timewindow of 5 days before being able to reclaim the event account.
						// So, we skip this test, and just leave the code here for reference, in case we want to test
						// it in the future or we want to automate the reclaiming process elsewhere.
						// See https://github.com/circlefin/solana-cctp-contracts/blob/ec16e95d28ee47f7832df4203ae07b5981d146fc/programs/v2/message-transmitter-v2/src/state.rs#L56
						t.Skip()

						initial, err := solanaGoClient.GetAccountInfoWithOpts(ctx, user.PublicKey(), &rpc.GetAccountInfoOpts{Commitment: config.DefaultCommitment})
						require.NoError(t, err)

						messageEventAccount, err := solanaGoClient.GetAccountInfoWithOpts(ctx, messageSentEventKeypair.PublicKey(), &rpc.GetAccountInfoOpts{Commitment: config.DefaultCommitment})
						require.NoError(t, err)

						reclaimIx, err := cctp_message_transmitter_v2.NewReclaimEventAccountInstruction(
							cctp_message_transmitter_v2.ReclaimEventAccountParams{
								Attestation:        attestation,
								DestinationMessage: messageBytes,
							},
							user.PublicKey(),
							pdas.v2.MessageTransmitter.MessageTransmitter,
							messageSentEventKeypair.PublicKey(),
						)
						require.NoError(t, err)

						result := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{reclaimIx}, user, config.DefaultCommitment)
						require.NotNil(t, result)

						final, err := solanaGoClient.GetAccountInfoWithOpts(ctx, user.PublicKey(), &rpc.GetAccountInfoOpts{Commitment: config.DefaultCommitment})
						require.NoError(t, err)

						require.Equal(t, initial.Value.Lamports-result.Meta.Fee+messageEventAccount.Value.Lamports, final.Value.Lamports)
					})
				})
			})
		})

		t.Run("admin:setRouter", func(t *testing.T) {
			programDataAddr := testutils.GetProgramDataAddress(ctx, t, solanaGoClient, cctpPool.Program)

			// Change router to CcipRouterProgram (from mockRamp) — admin is upgrade authority
			ixSetRaw, err := cctp_token_pool.NewSetRouterInstruction(config.CcipRouterProgram, cctpPool.State, usdcMint, admin.PublicKey(), cctpPool.Program, programDataAddr)
			require.NoError(t, err)
			res := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ixSetRaw}, admin, config.DefaultCommitment)
			require.NotNil(t, res)

			eventRouter, err := common.ParseEventFromLogs(res.Meta.LogMessages, base_token_pool.ParseEvent_RouterUpdated)
			require.NoError(t, err)
			require.Equal(t, config.CcipRouterProgram, eventRouter.NewRouter)
			require.Equal(t, mockRamp, eventRouter.OldRouter)
			require.Equal(t, usdcMint, eventRouter.Mint)

			// Restore router to mockRamp so subsequent tests remain authorized
			ixRestoreRaw, err := cctp_token_pool.NewSetRouterInstruction(mockRamp, cctpPool.State, usdcMint, admin.PublicKey(), cctpPool.Program, programDataAddr)
			require.NoError(t, err)
			res = testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ixRestoreRaw}, admin, config.DefaultCommitment)
			require.NotNil(t, res)

			eventRouter, err = common.ParseEventFromLogs(res.Meta.LogMessages, base_token_pool.ParseEvent_RouterUpdated)
			require.NoError(t, err)
			require.Equal(t, mockRamp, eventRouter.NewRouter)
			require.Equal(t, config.CcipRouterProgram, eventRouter.OldRouter)
			require.Equal(t, usdcMint, eventRouter.Mint)
		})

		t.Run("admin:setRmn", func(t *testing.T) {
			programDataAddr := testutils.GetProgramDataAddress(ctx, t, solanaGoClient, cctpPool.Program)

			// Change RMN to CcipRouterProgram (from RMNRemoteProgram) — admin is upgrade authority
			ixSetRaw, err := cctp_token_pool.NewSetRmnInstruction(config.CcipRouterProgram, cctpPool.State, usdcMint, admin.PublicKey(), cctpPool.Program, programDataAddr)
			require.NoError(t, err)
			res := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ixSetRaw}, admin, config.DefaultCommitment)
			require.NotNil(t, res)

			eventRmn, err := common.ParseEventFromLogs(res.Meta.LogMessages, base_token_pool.ParseEvent_RmnRemoteUpdated)
			require.NoError(t, err)
			require.Equal(t, config.CcipRouterProgram, eventRmn.NewRmnRemote)
			require.Equal(t, config.RMNRemoteProgram, eventRmn.OldRmnRemote)
			require.Equal(t, usdcMint, eventRmn.Mint)

			// Restore RMN to RMNRemoteProgram so subsequent tests remain authorized
			ixRestoreRaw, err := cctp_token_pool.NewSetRmnInstruction(config.RMNRemoteProgram, cctpPool.State, usdcMint, admin.PublicKey(), cctpPool.Program, programDataAddr)
			require.NoError(t, err)
			res = testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ixRestoreRaw}, admin, config.DefaultCommitment)
			require.NotNil(t, res)

			eventRmn, err = common.ParseEventFromLogs(res.Meta.LogMessages, base_token_pool.ParseEvent_RmnRemoteUpdated)
			require.NoError(t, err)
			require.Equal(t, config.RMNRemoteProgram, eventRmn.NewRmnRemote)
			require.Equal(t, config.CcipRouterProgram, eventRmn.OldRmnRemote)
			require.Equal(t, usdcMint, eventRmn.Mint)
		})

		t.Run("TypeVersion", func(t *testing.T) {
			t.Parallel()

			ix, err := cctp_token_pool.NewTypeVersionInstruction()
			require.NoError(t, err)
			result := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)
			require.NotNil(t, result)

			output, err := common.ExtractTypedReturnValue(ctx, result.Meta.LogMessages, config.CctpTokenPoolProgram.String(), func(b []byte) string {
				require.Len(t, b, int(binary.LittleEndian.Uint32(b[:4]))+4) // the first 4 bytes just encodes the length
				return string(b[4:])
			})
			require.NoError(t, err)
			// regex from https://semver.org/#is-there-a-suggested-regular-expression-regex-to-check-a-semver-string
			semverRegex := "(0|[1-9]\\d*)\\.(0|[1-9]\\d*)\\.(0|[1-9]\\d*)(?:-((?:0|[1-9]\\d*|\\d*[a-zA-Z-][0-9a-zA-Z-]*)(?:\\.(?:0|[1-9]\\d*|\\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?(?:\\+([0-9a-zA-Z-]+(?:\\.[0-9a-zA-Z-]+)*))?"
			require.Regexp(t, fmt.Sprintf("^%s %s$", "cctp-token-pool", semverRegex), output)
			t.Logf("Type Version: %s", output)
		})

		t.Run("query resolvers return the full account list", func(t *testing.T) {
			resolve := func(inputs []byte) (solana.Instruction, error) {
				return cctp_token_pool.NewNestedResolveAccountsStartInstruction(
					cctp_token_pool.NestedResolveAccountsParams{
						Caller:              user.PublicKey(),
						CallerProgram:       config.CcipRouterProgram,
						CpiResolutionInputs: inputs,
					},
					cctpPool.State,
				)
			}
			ccvsParams := cctp_token_pool.GetRequiredCcVsIn{
				RemoteChainSelector: config.EvmChainSelector,
				LocalToken:          usdcMint,
				Direction:           cctp_token_pool.MessageDirection_Outbound,
			}
			requireQueriesResolveFullAccountList(ctx, t, solanaGoClient, user, cctpPool.Program, resolve,
				[]solana.PublicKey{cctpPool.State, cctpPool.Evm.ChainConfig, cctpPool.Evm.ChainConfigV2},
				[]poolQuery{
					{"get_required_ccvs", resolutionInputsGetRequiredCcv, &ccvsParams, func(a []solana.PublicKey) (solana.Instruction, error) {
						return cctp_token_pool.NewGetRequiredCcvsInstruction(ccvsParams, a[0], a[1], a[2])
					}},
				},
			)
		})

		mockRampCctpSigner, _, derr := state.FindExternalTokenPoolsSignerPDA(config.CctpTokenPoolProgram, mockRamp)
		require.NoError(t, derr)

		t.Run("E2E v1 through pool (svm user -> svm admin)", func(t *testing.T) {
			remoteChainSelector := config.SvmChainSelector
			fakeCcipTotalNonce := uint64(1234567890)

			messageSentEventAddress, _, err := solana.FindProgramAddress([][]byte{
				[]byte("ccip_cctp_message_sent_event"),
				user.PublicKey().Bytes(),
				common.Uint64ToLE(remoteChainSelector),
				common.Uint64ToLE(fakeCcipTotalNonce),
			}, cctpPool.Program)
			require.NoError(t, err)

			messageAmount := uint64(1e5) // 0.1 USDC, as it uses 6 decimals
			var messageSentEventData cctp_message_transmitter.MessageSent
			var attestation []byte

			t.Run("Basic onramp", func(t *testing.T) {
				// fund pool signer with SOL so it can pay for the rent of the message sent event account
				fundPoolSignerIx, err := tokens.NativeTransfer(1*solana.LAMPORTS_PER_SOL, admin.PublicKey(), cctpPool.Signer)
				require.NoError(t, err)

				// fund pool signer with USDC, as the onramp would normally do it but we're using a mock ramp here
				transferI, err := tokens.TokenTransferChecked(messageAmount, usdcDecimals, solana.TokenProgramID, adminATA, usdcMint, cctpPool.TokenAccount, admin.PublicKey(), solana.PublicKeySlice{})
				require.NoError(t, err)

				lockOrBurnIn := cctp_token_pool.LockOrBurnInV1{
					LocalToken:          usdcMint,
					Amount:              messageAmount,
					RemoteChainSelector: config.SvmChainSelector,
					Receiver:            cctpPool.TokenAccount.Bytes(),
					OriginalSender:      user.PublicKey(),
					MsgTotalNonce:       fakeCcipTotalNonce,
				}

				dynamicAdditionalAccountMetas := []*solana.AccountMeta{
					solana.Meta(pdas.v1.TokenMessengerMinter.AuthorityPda),
					solana.Meta(pdas.v1.TokenMessengerMinter.RemoteTokenMessenger),
					solana.Meta(messageSentEventAddress).WRITE(),
				}

				additionalAccountMetas := make([]*solana.AccountMeta, 0, 8+len(dynamicAdditionalAccountMetas))
				additionalAccountMetas = append(additionalAccountMetas,
					// static ones, present in LUT
					solana.Meta(pdas.v1.MessageTransmitter.MessageTransmitter).WRITE(),
					solana.Meta(pdas.v1.TokenMessengerMinter.Program),
					solana.Meta(solana.SystemProgramID),
					solana.Meta(pdas.v1.MessageTransmitter.Program),
					solana.Meta(pdas.v1.TokenMessengerMinter.TokenMessenger),
					solana.Meta(pdas.v1.TokenMessengerMinter.TokenMinter),
					solana.Meta(pdas.v1.TokenMessengerMinter.LocalToken).WRITE(),
					solana.Meta(pdas.v1.TokenMessengerMinter.EventAuthority),
				)
				additionalAccountMetas = append(additionalAccountMetas, dynamicAdditionalAccountMetas...)

				t.Run("Accounts derivation", func(t *testing.T) {
					accounts, tables := deriveCctpIxAccounts(ctx, t, solanaGoClient, admin, func(stage string, askWith []*solana.AccountMeta) solana.Instruction {
						raw, err1 := cctp_token_pool.NewDeriveAccountsLockOrBurnTokensInstruction(
							stage,
							lockOrBurnIn,
						)
						require.NoError(t, err1)
						genericRaw, _ := raw.(*solana.GenericInstruction)
						genericRaw.AccountValues = append(genericRaw.AccountValues, askWith...)
						return raw
					})
					require.Equal(t, []solana.PublicKey{}, tables)
					require.Equal(t, dynamicAdditionalAccountMetas, accounts)
				})

				ix, err := testutils.NewPoolProxyLockOrBurnInstruction(
					tp.LockOrBurnInV1(lockOrBurnIn),
					cctpPool.Program,
					mockRampCctpSigner,
					cctpPool.State,
					solana.TokenProgramID,
					usdcMint,
					cctpPool.Signer,
					cctpPool.TokenAccount,
					config.RMNRemoteProgram,
					config.RMNRemoteCursesPDA,
					config.RMNRemoteConfigPDA,
					cctpPool.Svm.ChainConfig,
				)
				require.NoError(t, err)
				genericIx, _ := ix.(*solana.GenericInstruction)
				genericIx.AccountValues = append(genericIx.AccountValues, additionalAccountMetas...)

				t.Run("When there is no curse, it succeeds", func(t *testing.T) {
					res := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{transferI, fundPoolSignerIx, ix}, admin, config.DefaultCommitment)
					require.NotNil(t, res)

					output, err := common.ExtractAnchorTypedReturnValue[cctp_token_pool.LockOrBurnOutV1](ctx, res.Meta.LogMessages, cctpPool.Program.String())
					require.NoError(t, err)
					outputSourceDomain := binary.BigEndian.Uint32(output.DestPoolData[60:64])
					require.Equal(t, outputSourceDomain, domain) // the source domain is Solana in this test

					require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, messageSentEventAddress, config.DefaultCommitment, &messageSentEventData))
					t.Log("Message Sent Event Data:", messageSentEventData)
					require.Equal(t, cctpPool.Signer, messageSentEventData.RentPayer)

					ccipCctpMessageSentEvent, err := common.ParseEventFromLogs(res.Meta.LogMessages, cctp_token_pool.ParseEvent_CcipCctpMessageSentEvent)
					require.NoError(t, err)
					require.Equal(t, user.PublicKey(), ccipCctpMessageSentEvent.OriginalSender)
					require.Equal(t, config.SvmChainSelector, ccipCctpMessageSentEvent.RemoteChainSelector)
					require.Equal(t, fakeCcipTotalNonce, ccipCctpMessageSentEvent.MsgTotalNonce)
					require.Equal(t, messageSentEventAddress, ccipCctpMessageSentEvent.EventAddress)
					require.Equal(t, messageSentEventData.Message, ccipCctpMessageSentEvent.MessageSentBytes)
					require.Equal(t, domain, ccipCctpMessageSentEvent.SourceDomain)

					// Check CCTP nonce in all ways it appears
					outputNonce := binary.BigEndian.Uint64(output.DestPoolData[24:32])                      // in pool returned dest data
					require.Equal(t, outputNonce, cctp.GetNonce(ccipCctpMessageSentEvent.MessageSentBytes)) // in event message bytes
					require.Equal(t, outputNonce, cctp.GetNonce(messageSentEventData.Message))              // in event account data
					require.Equal(t, outputNonce, ccipCctpMessageSentEvent.CctpNonce)                       // in the event field
				})

				t.Run("Rate limit persists after onramp", func(t *testing.T) {
					var cfg cctp_token_pool.ChainConfig
					require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, cctpPool.Svm.ChainConfig, config.DefaultCommitment, &cfg))

					require.Less(t, cfg.Base.OutboundRateLimit.Tokens, cfg.Base.OutboundRateLimit.Cfg.Capacity,
						"rate limit tokens should decrease after onramp")
				})

				t.Run("Rate limit exceeded on second onramp", func(t *testing.T) {
					// The outbound rate limit bucket is depleted from the first onramp,
					// so a second onramp of the same amount should fail.
					// Use a different nonce so the CCTP event account PDA is fresh
					// (the previous one is already initialized from the first onramp).
					secondNonce := fakeCcipTotalNonce + 1
					secondEventAddress, _, err := solana.FindProgramAddress([][]byte{
						[]byte("ccip_cctp_message_sent_event"),
						user.PublicKey().Bytes(),
						common.Uint64ToLE(remoteChainSelector),
						common.Uint64ToLE(secondNonce),
					}, cctpPool.Program)
					require.NoError(t, err)

					secondLockOrBurnIn := cctp_token_pool.LockOrBurnInV1{
						LocalToken:          usdcMint,
						Amount:              messageAmount,
						RemoteChainSelector: config.SvmChainSelector,
						Receiver:            cctpPool.TokenAccount.Bytes(),
						OriginalSender:      user.PublicKey(),
						MsgTotalNonce:       secondNonce,
					}

					secondDynamicMetas := []*solana.AccountMeta{
						solana.Meta(pdas.v1.TokenMessengerMinter.AuthorityPda),
						solana.Meta(pdas.v1.TokenMessengerMinter.RemoteTokenMessenger),
						solana.Meta(secondEventAddress).WRITE(),
					}

					secondAdditionalMetas := make([]*solana.AccountMeta, 0, 8+len(secondDynamicMetas))
					secondAdditionalMetas = append(secondAdditionalMetas,
						solana.Meta(pdas.v1.MessageTransmitter.MessageTransmitter).WRITE(),
						solana.Meta(pdas.v1.TokenMessengerMinter.Program),
						solana.Meta(solana.SystemProgramID),
						solana.Meta(pdas.v1.MessageTransmitter.Program),
						solana.Meta(pdas.v1.TokenMessengerMinter.TokenMessenger),
						solana.Meta(pdas.v1.TokenMessengerMinter.TokenMinter),
						solana.Meta(pdas.v1.TokenMessengerMinter.LocalToken).WRITE(),
						solana.Meta(pdas.v1.TokenMessengerMinter.EventAuthority),
					)
					secondAdditionalMetas = append(secondAdditionalMetas, secondDynamicMetas...)

					secondIx, err := testutils.NewPoolProxyLockOrBurnInstruction(
						tp.LockOrBurnInV1(secondLockOrBurnIn),
						cctpPool.Program,
						mockRampCctpSigner,
						cctpPool.State,
						solana.TokenProgramID,
						usdcMint,
						cctpPool.Signer,
						cctpPool.TokenAccount,
						config.RMNRemoteProgram,
						config.RMNRemoteCursesPDA,
						config.RMNRemoteConfigPDA,
						cctpPool.Svm.ChainConfig,
					)
					require.NoError(t, err)
					secondGenericIx, _ := secondIx.(*solana.GenericInstruction)
					secondGenericIx.AccountValues = append(secondGenericIx.AccountValues, secondAdditionalMetas...)

					testutils.SendAndFailWith(ctx, t, solanaGoClient,
						[]solana.Instruction{transferI, fundPoolSignerIx, secondIx},
						admin, config.DefaultCommitment, []string{"rate limit reached"})
				})
			})

			t.Run("Basic offramp", func(t *testing.T) {
				fundCustodyIx, err := tokens.MintTo(messageAmount, solana.TokenProgramID, usdcMint, pdas.v1.TokenMessengerMinter.CustodyTokenAccount, admin.PublicKey())
				require.NoError(t, err)
				testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{fundCustodyIx}, admin, config.DefaultCommitment)

				decimals, amount, err := tokens.TokenBalance(ctx, solanaGoClient, pdas.v1.TokenMessengerMinter.CustodyTokenAccount, config.DefaultCommitment)
				require.NoError(t, err)
				require.Equal(t, messageAmount, uint64(amount)) //nolint:gosec // G115
				require.Equal(t, usdcDecimals, decimals)

				_, initial, err := tokens.TokenBalance(ctx, solanaGoClient, adminATA, config.DefaultCommitment)
				require.NoError(t, err)

				attestation, err = testutils.AttestCCTP(messageSentEventData.Message, attesters)
				require.NoError(t, err)
				offchainTokenDataBuffer := new(bytes.Buffer)
				offchainEncoder := bin.NewBorshEncoder(offchainTokenDataBuffer)
				require.NoError(t, offchainEncoder.Encode(messageSentEventData.Message))
				require.NoError(t, offchainEncoder.Encode(attestation))

				sourcePoolData := make([]byte, 64)
				binary.BigEndian.PutUint64(sourcePoolData[24:32], cctp.GetNonce(messageSentEventData.Message))
				binary.BigEndian.PutUint32(sourcePoolData[60:64], domain)

				releaseOrMintIn := cctp_token_pool.ReleaseOrMintInV1{
					OriginalSender:      cctp_token_pool.RemoteAddress{Address: user.PublicKey().Bytes()},
					RemoteChainSelector: config.SvmChainSelector,
					Receiver:            admin.PublicKey(),
					Amount:              tokens.ToLittleEndianU256(messageAmount),
					LocalToken:          usdcMint,
					SourcePoolAddress:   cctp_token_pool.RemoteAddress{Address: cctpPool.Signer.Bytes()}, // when the source is Solana, the pool is identified by its signer
					SourcePoolData:      sourcePoolData,
					OffchainTokenData:   offchainTokenDataBuffer.Bytes(),
				}

				dynamicAdditionalAccountMetas := []*solana.AccountMeta{
					// Accounts not in LUT
					solana.Meta(pdas.v1.MessageTransmitter.AuthorityPda),
					solana.Meta(pdas.v1.MessageTransmitter.EventAuthority),
					solana.Meta(pdas.v1.TokenMessengerMinter.CustodyTokenAccount).WRITE(),
					solana.Meta(pdas.v1.TokenMessengerMinter.RemoteTokenMessenger),
					solana.Meta(pdas.v1.TokenMessengerMinter.TokenPair),
					solana.Meta(cctp.GetUsedNoncesPDA(t, messageSentEventData.Message)).WRITE(),
				}

				additionalAccountMetas := make([]*solana.AccountMeta, 0, 8+len(dynamicAdditionalAccountMetas))
				additionalAccountMetas = append(additionalAccountMetas,
					// Accounts present in LUT
					solana.Meta(pdas.v1.MessageTransmitter.MessageTransmitter),
					solana.Meta(pdas.v1.TokenMessengerMinter.Program),
					solana.Meta(solana.SystemProgramID),
					solana.Meta(pdas.v1.MessageTransmitter.Program),
					solana.Meta(pdas.v1.TokenMessengerMinter.TokenMessenger),
					solana.Meta(pdas.v1.TokenMessengerMinter.TokenMinter).WRITE(),
					solana.Meta(pdas.v1.TokenMessengerMinter.LocalToken).WRITE(),
					solana.Meta(pdas.v1.TokenMessengerMinter.EventAuthority),
				)
				additionalAccountMetas = append(additionalAccountMetas, dynamicAdditionalAccountMetas...)

				t.Run("Accounts derivation", func(t *testing.T) {
					accounts, tables := deriveCctpIxAccounts(ctx, t, solanaGoClient, admin, func(stage string, askWith []*solana.AccountMeta) solana.Instruction {
						raw, err1 := cctp_token_pool.NewDeriveAccountsReleaseOrMintTokensInstruction(
							stage,
							releaseOrMintIn,
						)
						require.NoError(t, err1)
						genericRaw, _ := raw.(*solana.GenericInstruction)
						genericRaw.AccountValues = append(genericRaw.AccountValues, askWith...)
						return raw
					})
					require.Equal(t, []solana.PublicKey{}, tables)
					require.Equal(t, dynamicAdditionalAccountMetas, accounts)
				})

				ix, err := testutils.NewPoolProxyReleaseOrMintInstruction(
					tp.ReleaseOrMintInV1{
						OriginalSender:      tp.RemoteAddress{Address: releaseOrMintIn.OriginalSender.Address},
						RemoteChainSelector: releaseOrMintIn.RemoteChainSelector,
						Receiver:            releaseOrMintIn.Receiver,
						Amount:              releaseOrMintIn.Amount,
						LocalToken:          releaseOrMintIn.LocalToken,
						SourcePoolAddress:   tp.RemoteAddress{Address: releaseOrMintIn.SourcePoolAddress.Address},
						SourcePoolData:      releaseOrMintIn.SourcePoolData,
						OffchainTokenData:   releaseOrMintIn.OffchainTokenData,
					},
					cctpPool.Program,
					mockRampCctpSigner,
					mockRamp,
					allowedOfframpSvmPDA,
					cctpPool.State,
					solana.TokenProgramID,
					usdcMint,
					cctpPool.Signer,
					cctpPool.TokenAccount,
					cctpPool.Svm.ChainConfig,
					config.RMNRemoteProgram,
					config.RMNRemoteCursesPDA,
					config.RMNRemoteConfigPDA,
					adminATA,
				)
				require.NoError(t, err)
				genericIx, _ := ix.(*solana.GenericInstruction)
				genericIx.AccountValues[7].IsWritable = true // CCTP requires this, though other pools don't (which is why the bindings don't do it by default)

				genericIx.AccountValues = append(genericIx.AccountValues, additionalAccountMetas...)

				// merge lookup tables into single map
				lookupTables := map[solana.PublicKey]solana.PublicKeySlice{}
				maps.Copy(lookupTables, offrampLookupTable)
				maps.Copy(lookupTables, tpLookupTable)

				t.Run("When there is no curse, it succeeds", func(t *testing.T) {
					res := testutils.SendAndConfirmWithLookupTables(ctx, t, solanaGoClient, []solana.Instruction{fundCustodyIx, ix},
						admin, config.DefaultCommitment, lookupTables)
					require.NotNil(t, res)

					_, final, err := tokens.TokenBalance(ctx, solanaGoClient, adminATA, config.DefaultCommitment)
					require.NoError(t, err)

					require.Equal(t, uint64(initial)+messageAmount, uint64(final), "Admin should have received the USDC after receiving the message") //nolint:gosec // G115
				})

				t.Run("Rate limit persists after offramp", func(t *testing.T) {
					var cfg cctp_token_pool.ChainConfig
					require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, cctpPool.Svm.ChainConfig, config.DefaultCommitment, &cfg))

					require.Less(t, cfg.Base.InboundRateLimit.Tokens, cfg.Base.InboundRateLimit.Cfg.Capacity,
						"rate limit tokens should decrease after offramp")
				})

				t.Run("Rate limit exceeded on second offramp", func(t *testing.T) {
					// The inbound rate limit bucket is depleted from the first offramp,
					// so a second offramp of the same amount should fail.
					testutils.SendAndFailWithLookupTables(ctx, t, solanaGoClient,
						[]solana.Instruction{fundCustodyIx, ix},
						admin, config.DefaultCommitment, lookupTables, []string{"rate limit reached"})
				})
			})

			t.Run("Reclaim event account rent", func(t *testing.T) {
				poolSignerInfo, err := solanaGoClient.GetAccountInfoWithOpts(ctx, cctpPool.Signer, &rpc.GetAccountInfoOpts{Commitment: config.DefaultCommitment})
				require.NoError(t, err)
				initialBalance := poolSignerInfo.Value.Lamports

				messageSentAccount, err := solanaGoClient.GetAccountInfoWithOpts(ctx, messageSentEventAddress, &rpc.GetAccountInfoOpts{Commitment: config.DefaultCommitment})
				require.NoError(t, err)
				messageRent := messageSentAccount.Value.Lamports

				ix, err := cctp_token_pool.NewReclaimEventAccountInstruction(
					usdcMint,
					user.PublicKey(),
					remoteChainSelector,
					fakeCcipTotalNonce,
					attestation,
					cctpPool.State,
					cctpPool.Signer,
					messageSentEventAddress,
					pdas.v1.MessageTransmitter.MessageTransmitter,
					pdas.v1.MessageTransmitter.Program,
					admin.PublicKey(),
					solana.SystemProgramID,
				)
				require.NoError(t, err)

				// There's no fund manager, so funds cannot be reclaimed
				testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment, []string{"Fund Manager is invalid"})

				fundManagerIx, err := cctp_token_pool.NewSetFundManagerInstruction(admin.PublicKey(), cctpPool.State, usdcMint, admin.PublicKey())
				require.NoError(t, err)

				testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{fundManagerIx, ix}, admin, config.DefaultCommitment)

				poolSignerInfoAfter, err := solanaGoClient.GetAccountInfoWithOpts(ctx, cctpPool.Signer, &rpc.GetAccountInfoOpts{Commitment: config.DefaultCommitment})
				require.NoError(t, err)
				finalBalance := poolSignerInfoAfter.Value.Lamports

				require.Equal(t, finalBalance, initialBalance+messageRent)
			})

			t.Run("Reclaim funds from signer account", func(t *testing.T) {
				poolSignerInfoInitial, err := solanaGoClient.GetAccountInfoWithOpts(ctx, cctpPool.Signer, &rpc.GetAccountInfoOpts{Commitment: config.DefaultCommitment})
				require.NoError(t, err)
				startingBalance := poolSignerInfoInitial.Value.Lamports
				require.LessOrEqual(t, startingBalance, 1*solana.LAMPORTS_PER_SOL)

				userAtaBalanceInitial, err := solanaGoClient.GetAccountInfoWithOpts(ctx, userATA, &rpc.GetAccountInfoOpts{Commitment: config.DefaultCommitment})
				require.NoError(t, err)
				startingAtaBalance := userAtaBalanceInitial.Value.Lamports

				// Fund pool signer to exactly 1 SOL
				fundPoolSignerIx, err := tokens.NativeTransfer(1*solana.LAMPORTS_PER_SOL-startingBalance, admin.PublicKey(), cctpPool.Signer)
				require.NoError(t, err)
				testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{fundPoolSignerIx}, admin, config.DefaultCommitment)

				// Can't reclaim funds to invalid (non-configured) receiver
				reclaimToInvalidIx, err := cctp_token_pool.NewReclaimFundsInstruction(solana.LAMPORTS_PER_SOL/5, cctpPool.State, usdcMint, cctpPool.Signer, userATA, admin.PublicKey(), solana.SystemProgramID)
				require.NoError(t, err)
				testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{reclaimToInvalidIx}, admin, config.DefaultCommitment, []string{"Invalid destination for funds reclaim"})

				// We set reclaim destination to user ATA
				reclaimDestinationConfigIx, err := cctp_token_pool.NewSetFundReclaimDestinationInstruction(userATA, cctpPool.State, usdcMint, admin.PublicKey())
				require.NoError(t, err)
				testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{reclaimDestinationConfigIx}, admin, config.DefaultCommitment)

				// Can't reclaim invalid values
				reclaimZeroIx, err := cctp_token_pool.NewReclaimFundsInstruction(0, cctpPool.State, usdcMint, cctpPool.Signer, userATA, admin.PublicKey(), solana.SystemProgramID)
				require.NoError(t, err)
				testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{reclaimZeroIx}, admin, config.DefaultCommitment, []string{"Invalid SOL amount"})

				reclaimExcessiveFundsIx, err := cctp_token_pool.NewReclaimFundsInstruction(2*solana.LAMPORTS_PER_SOL, cctpPool.State, usdcMint, cctpPool.Signer, userATA, admin.PublicKey(), solana.SystemProgramID)
				require.NoError(t, err)
				testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{reclaimExcessiveFundsIx}, admin, config.DefaultCommitment, []string{"Insufficient funds"})

				// reclaim limits are enforced
				setLimitIx, err := cctp_token_pool.NewSetMinimumSignerFundsInstruction(solana.LAMPORTS_PER_SOL/2, cctpPool.State, usdcMint, admin.PublicKey())
				require.NoError(t, err)
				testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{setLimitIx}, admin, config.DefaultCommitment)
				reclaimFundsOverLimitIx, err := cctp_token_pool.NewReclaimFundsInstruction(3*solana.LAMPORTS_PER_SOL/4, cctpPool.State, usdcMint, cctpPool.Signer, userATA, admin.PublicKey(), solana.SystemProgramID)
				require.NoError(t, err)
				testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{reclaimFundsOverLimitIx}, admin, config.DefaultCommitment, []string{"Insufficient funds"})

				// Otherwise, reclaiming succeeds
				validReclaimAmount := solana.LAMPORTS_PER_SOL / 4
				validReclaimIx, err := cctp_token_pool.NewReclaimFundsInstruction(validReclaimAmount, cctpPool.State, usdcMint, cctpPool.Signer, userATA, admin.PublicKey(), solana.SystemProgramID)
				require.NoError(t, err)
				testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{validReclaimIx}, admin, config.DefaultCommitment)

				poolSignerInfoAfter, err := solanaGoClient.GetAccountInfoWithOpts(ctx, cctpPool.Signer, &rpc.GetAccountInfoOpts{Commitment: config.DefaultCommitment})
				require.NoError(t, err)
				finalBalance := poolSignerInfoAfter.Value.Lamports

				userAtaBalanceAfter, err := solanaGoClient.GetAccountInfoWithOpts(ctx, userATA, &rpc.GetAccountInfoOpts{Commitment: config.DefaultCommitment})
				require.NoError(t, err)
				finalAtaBalance := userAtaBalanceAfter.Value.Lamports

				require.Equal(t, finalBalance, startingBalance-validReclaimAmount)
				require.Equal(t, finalAtaBalance, startingAtaBalance+validReclaimAmount)
			})
		})

		t.Run("E2E v2 through pool", func(t *testing.T) {
			t.Run("Onramp: svm to evm", func(t *testing.T) {
				messageAmount := uint64(1e5) // 0.1 USDC, as it uses 6 decimals

				t.Run("Basic onramp", func(t *testing.T) {
					// fund pool signer with SOL so it can pay for the rent of the message sent event account
					fundPoolSignerIx, err := tokens.NativeTransfer(1*solana.LAMPORTS_PER_SOL, admin.PublicKey(), cctpPool.Signer)
					require.NoError(t, err)

					// fund pool signer with USDC, as the onramp would normally do it but we're using a mock ramp here
					transferI, err := tokens.TokenTransferChecked(messageAmount, usdcDecimals, solana.TokenProgramID, adminATA, usdcMint, cctpPool.TokenAccount, admin.PublicKey(), solana.PublicKeySlice{})
					require.NoError(t, err)

					lockOrBurnIn := cctp_token_pool.LockOrBurnInV2{
						LocalToken:              usdcMint,
						Amount:                  messageAmount,
						RemoteChainSelector:     config.EvmChainSelector,
						Receiver:                cctpPool.TokenAccount.Bytes(),
						OriginalSender:          user.PublicKey(),
						RequestedFinalityConfig: cctp_token_pool.FinalityConfig{},
						TokenArgs:               []byte{},
					}

					ix, err := testutils.NewPoolProxyLockOrBurnV2Instruction(
						lockOrBurnIn,
						cctpPool.Program,
						mockRampCctpSigner,
						cctpPool.State,
						solana.TokenProgramID,
						usdcMint,
						cctpPool.Signer,
						cctpPool.TokenAccount,
						cctpPool.Evm.ChainConfig,
						cctpPool.Evm.ChainConfigV2,
						cctpPool.Evm.ChainConfigOverrides,
						// CCTP charges no in-token BPS fee, so its onramp context has no fee
						// vault account.
						solana.PublicKey{},
						cctpPool.EventAuthority,
					)
					require.NoError(t, err)

					t.Run("it succeeds", func(t *testing.T) {
						res := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{transferI, fundPoolSignerIx, ix}, admin, config.DefaultCommitment)
						require.NotNil(t, res)

						// Validate returned value from pool
						output, err := common.ExtractAnchorTypedReturnValue[cctp_token_pool.LockOrBurnOutV2](ctx, res.Meta.LogMessages, cctpPool.Program.String())
						require.NoError(t, err)
						require.Equal(t, usdcEvm, output.DestTokenAddress.Address)

						require.Equal(t, []byte{0x30, 0x47, 0x58, 0x7c}, output.DestPoolData, "dest pool data should contain the CCTP_VERSION_2_CCV_TAG")
						require.Equal(t, messageAmount, output.DestTokenAmount)

						// Validate emitted event
						tx, err := res.Transaction.GetTransaction()
						require.NoError(t, err)
						event, err := common.ExtractCPIEvent(tx, res.Meta, cctpPool.Program, base_token_pool.ParseEvent_LockedOrBurned)
						require.NoError(t, err)
						require.Equal(t, usdcMint, event.Mint)
						require.Equal(t, mockRampCctpSigner, event.Sender)
						require.Equal(t, config.EvmChainSelector, event.RemoteChainSelector)
						require.Equal(t, messageAmount, event.Amount)
					})

					t.Run("Rate limit persists after onramp", func(t *testing.T) {
						var cfg cctp_token_pool.ChainConfig
						require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, cctpPool.Evm.ChainConfig, config.DefaultCommitment, &cfg))

						require.Less(t, cfg.Base.OutboundRateLimit.Tokens, cfg.Base.OutboundRateLimit.Cfg.Capacity,
							"rate limit tokens should decrease after onramp")
						require.GreaterOrEqual(t, cfg.Base.OutboundRateLimit.Cfg.Capacity-cfg.Base.OutboundRateLimit.Tokens, messageAmount,
							"rate limit tokens should decrease after onramp")
					})

					t.Run("Rate limit exceeded on second onramp", func(t *testing.T) {
						// The outbound rate limit bucket is depleted from the first onramp,
						// so a second onramp of the same amount should fail.
						testutils.SendAndFailWith(ctx, t, solanaGoClient,
							[]solana.Instruction{transferI, fundPoolSignerIx, ix},
							admin, config.DefaultCommitment, []string{"rate limit reached"})
					})
				})
			})

			t.Run("Offramp: evm to svm", func(t *testing.T) {
				// The receiving user is the "admin" in this test, and the receiving token account is the admin's USDC ATA.

				evmSenderAddress, err := hex.DecodeString("e0b53A9a0991bb2e2A28D9606f266306232119E3")
				require.NoError(t, err)

				messageAmount := uint64(1e5) // 0.1 USDC, as it uses 6 decimals

				_, initial, err := tokens.TokenBalance(ctx, solanaGoClient, adminATA, config.DefaultCommitment)
				require.NoError(t, err)

				t.Run("Basic offramp", func(t *testing.T) {
					// Source pool data comes from
					// https://github.com/smartcontractkit/chainlink-ccip/blob/d146894e564d4ccb9e290b63809ad948e2e03339/chains/evm/contracts/pools/USDC/CCTPThroughCCVTokenPool.sol#L65 and
					// https://github.com/smartcontractkit/chainlink-ccip/blob/d146894e564d4ccb9e290b63809ad948e2e03339/chains/evm/contracts/libraries/USDCSourcePoolDataCodec.sol#L19
					sourcePoolData := make([]byte, 4)
					binary.BigEndian.PutUint32(sourcePoolData[0:4], 0x3047587c)

					releaseOrMintIn := cctp_token_pool.ReleaseOrMintInV2{
						OriginalSender:          cctp_token_pool.RemoteAddress{Address: evmSenderAddress},
						RemoteChainSelector:     config.EvmChainSelector,
						Receiver:                admin.PublicKey(),
						Amount:                  cctp_token_pool.ProtocolAmount{BeBytes: tokens.ToBigEndianU256(messageAmount)},
						LocalToken:              usdcMint,
						SourcePoolAddress:       cctp_token_pool.RemoteAddress{Address: evmPoolAddress},
						SourcePoolData:          sourcePoolData,
						RequestedFinalityConfig: cctp_token_pool.FinalityConfig{}, // this means WAIT_FOR_FINALITY
						ReceiverUsage:           0,
					}

					ix, err := testutils.NewPoolProxyReleaseOrMintV2Instruction(
						releaseOrMintIn,
						cctpPool.Program,
						mockRampCctpSigner,
						mockRamp,
						allowedOfframpEvmPDA,
						cctpPool.State,
						solana.TokenProgramID,
						usdcMint,
						cctpPool.Signer,
						cctpPool.TokenAccount,
						cctpPool.Evm.ChainConfig,
						cctpPool.Evm.ChainConfigOverrides,
						adminATA,
						cctpPool.EventAuthority,
					)
					require.NoError(t, err)

					// merge lookup tables into single map
					lookupTables := map[solana.PublicKey]solana.PublicKeySlice{}
					maps.Copy(lookupTables, offrampLookupTable)
					maps.Copy(lookupTables, tpLookupTable)

					t.Run("it succeeds", func(t *testing.T) {
						res := testutils.SendAndConfirmWithLookupTables(ctx, t, solanaGoClient, []solana.Instruction{ix},
							admin, config.DefaultCommitment, lookupTables)
						require.NotNil(t, res)

						_, afterPool, err := tokens.TokenBalance(ctx, solanaGoClient, adminATA, config.DefaultCommitment)
						require.NoError(t, err)

						require.Equal(t, initial, afterPool, "Admin should not have received the USDC yet (the verifier will mint it)")
					})

					t.Run("Rate limit persists after offramp", func(t *testing.T) {
						var cfg cctp_token_pool.ChainConfig
						require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, cctpPool.Evm.ChainConfig, config.DefaultCommitment, &cfg))

						require.Less(t, cfg.Base.InboundRateLimit.Tokens, cfg.Base.InboundRateLimit.Cfg.Capacity,
							"rate limit tokens should decrease after offramp")
					})

					t.Run("Rate limit exceeded on second offramp", func(t *testing.T) {
						// The inbound rate limit bucket is depleted from the first offramp,
						// so a second offramp of the same amount should fail.
						testutils.SendAndFailWithLookupTables(ctx, t, solanaGoClient,
							[]solana.Instruction{ix},
							admin, config.DefaultCommitment, lookupTables, []string{"rate limit reached"})
					})
				})
			})
		})
	})
}

func deriveCctpIxAccounts(
	ctx context.Context,
	t *testing.T,
	solanaGoClient *rpc.Client,
	signer solana.PrivateKey,
	createRawIx func(stage string, askWith []*solana.AccountMeta) solana.Instruction,
) (derivedAccounts []*solana.AccountMeta, lookUpTables []solana.PublicKey) {
	t.Helper()

	derivedAccounts = make([]*solana.AccountMeta, 0)
	lookUpTables = make([]solana.PublicKey, 0)

	askWith := []*solana.AccountMeta{}
	stage := "Start"
	for {
		derive := createRawIx(stage, askWith)
		tx := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{derive}, signer, config.DefaultCommitment)
		derivation, err := common.ExtractAnchorTypedReturnValue[cctp_token_pool.DeriveAccountsResponse](ctx, tx.Meta.LogMessages, config.CctpTokenPoolProgram.String())
		require.NoError(t, err)

		for _, meta := range derivation.AccountsToSave {
			derivedAccounts = append(derivedAccounts, &solana.AccountMeta{
				PublicKey:  meta.Pubkey,
				IsWritable: meta.IsWritable,
				IsSigner:   meta.IsSigner,
			})
		}

		askWith = make([]*solana.AccountMeta, 0, len(derivation.AskAgainWith))
		for _, meta := range derivation.AskAgainWith {
			askWith = append(askWith, &solana.AccountMeta{
				PublicKey:  meta.Pubkey,
				IsWritable: meta.IsWritable,
				IsSigner:   meta.IsSigner,
			})
		}

		if len(derivation.LookUpTablesToSave) > 0 {
			lookUpTables = append(lookUpTables, derivation.LookUpTablesToSave...)
		}

		if len(derivation.NextStage) == 0 {
			return derivedAccounts, lookUpTables
		}
		stage = derivation.NextStage
	}
}
