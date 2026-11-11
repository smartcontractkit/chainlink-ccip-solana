// lombard_verifier_verify_test.go tests lombard_token_pool's `verify` instruction: the
// destination-side hook that consumes an already-delivered Lombard mailbox message and performs
// the real SPL mint. Ports the "basic idea" of LombardVerifier.verifyMessage.t.sol, but Solana's
// `verify` has a completely different wire format from EVM's ABI-encoded GMP envelope (see
// verify.rs's doc comment: proof verification happens earlier, via a separate mailbox instruction
// a relayer calls first), so this drives the real CPI chain directly instead of reusing any
// EVM-specific envelope-building helper: mock_lombard_mailbox.mark_delivered stands in for that
// earlier relayer step, then lombard_token_pool.verify consumes it via the real
// verify -> mailbox::handle_message -> bridge::mock_gmp_receive CPI chain.
package contracts

import (
	"math/big"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/system"
	"github.com/gagliardetto/solana-go/programs/token"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-ccv/protocol"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/arbitrary_cpi_proxy"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/lombard_token_pool"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/mock_lombard_bridge"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/mock_lombard_mailbox"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/config"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/testutils"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/tokens"
)

func TestLombardVerifierVerifyMessage(t *testing.T) {
	ctx := t.Context()

	admin, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)
	receiver, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)

	bridge := config.MockLombardBridgeProgram
	mailbox := config.MockLombardMailboxProgram
	// Stands in for the real offramp: verify() no longer trusts a pubkey stored in our own admin
	// config, it checks the router's own allowed_offramp registry for whichever program signs the
	// call - the proxy plays both roles (router *and* offramp-shaped caller) so the test doesn't
	// need the full ccip-router setup, mirroring cctp_tokenpool_test.go's identical pattern.
	mockRamp := config.ArbitraryCpiProxyProgram

	solanaGoClient := testutils.DeployAllPrograms(t, testutils.PathToAnchorConfig, admin)
	require.NoError(t, testutils.FundAccounts(ctx, []solana.PrivateKey{admin, receiver}, solanaGoClient))

	// mock_lombard_bridge's mock_gmp_receive mints signed by this PDA - the real mint's
	// mint_authority must be set to it for the inner mint_to CPI to succeed.
	tokenAuthorityPDA, _, err := solana.FindProgramAddress([][]byte{[]byte("token_authority")}, bridge)
	require.NoError(t, err)

	mintPriv, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)
	mint := mintPriv.PublicKey()

	// Built manually rather than via tokens.CreateTokenWith: that helper's CreateAccount funding
	// account is the mintAuthority parameter, which must be a transaction signer - but here the
	// real mint authority is the (unsigning) tokenAuthorityPDA, so admin funds the account
	// creation while tokenAuthorityPDA is set as the mint's actual authority.
	mintLamports, err := solanaGoClient.GetMinimumBalanceForRentExemption(ctx, token.MINT_SIZE, config.DefaultCommitment)
	require.NoError(t, err)
	createMintIx, err := system.NewCreateAccountInstruction(mintLamports, token.MINT_SIZE, solana.TokenProgramID, admin.PublicKey(), mint).ValidateAndBuild()
	require.NoError(t, err)
	initMintIx, err := token.NewInitializeMintInstruction(9, tokenAuthorityPDA, tokenAuthorityPDA, mint, solana.SysVarRentPubkey).ValidateAndBuild()
	require.NoError(t, err)
	testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{createMintIx, initMintIx}, admin, config.DefaultCommitment, common.AddSigners(mintPriv))

	receiverATAIx, receiverATA, err := tokens.CreateAssociatedTokenAccount(solana.TokenProgramID, mint, receiver.PublicKey(), admin.PublicKey())
	require.NoError(t, err)
	testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{receiverATAIx}, admin, config.DefaultCommitment)

	// Not tokens.TokenPoolGlobalConfigPDA: Lombard's PoolConfig moved off the generic "config"
	// seed to make room for VerifierConfig there (see contracts/utils/state/pda.go).
	globalConfigPDA, _, err := state.FindLombardPoolConfigPDA(config.LombardTokenPoolProgram)
	require.NoError(t, err)
	statePDA, err := tokens.TokenPoolConfigAddress(mint, config.LombardTokenPoolProgram)
	require.NoError(t, err)
	poolSignerPDA, err := tokens.TokenPoolSignerAddress(mint, config.LombardTokenPoolProgram)
	require.NoError(t, err)

	t.Run("setup", func(t *testing.T) {
		programDataAddress := testutils.GetProgramDataAddress(ctx, t, solanaGoClient, config.LombardTokenPoolProgram)

		t.Run("init_global_config", func(t *testing.T) {
			ix, err1 := lombard_token_pool.NewInitGlobalConfigInstruction(
				globalConfigPDA, admin.PublicKey(), solana.SystemProgramID, config.LombardTokenPoolProgram, programDataAddress,
			)
			require.NoError(t, err1)
			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)
		})

		t.Run("initialize pool", func(t *testing.T) {
			// router = mockRamp (not the real ccip-router): verify()'s allowed_offramp check is
			// against `state.config.router`, and mockRamp doubles as that router below.
			ix, err1 := lombard_token_pool.NewInitializeInstruction(
				mockRamp, config.RMNRemoteProgram, bridge,
				statePDA, mint, admin.PublicKey(), config.LombardTokenPoolProgram, programDataAddress, globalConfigPDA, solana.SystemProgramID,
			)
			require.NoError(t, err1)
			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)
		})

		t.Run("initialize verifier", func(t *testing.T) {
			ix, err1 := lombard_token_pool.NewInitializeVerifierInstruction(
				config.LombardVerifierConfigPDA, config.LombardVerifierStorageLocationsPDA,
				admin.PublicKey(), solana.SystemProgramID, config.LombardTokenPoolProgram, programDataAddress,
			)
			require.NoError(t, err1)
			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)
		})
	})

	var versionTag [4]uint8
	{
		var cfg lombard_token_pool.VerifierConfig
		require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, config.LombardVerifierConfigPDA, config.DefaultCommitment, &cfg))
		versionTag = cfg.VersionTag
	}

	remoteChainSelector := config.EvmChainSelector
	remoteChainConfigPDA := config.LombardVerifierEvmRemoteChainConfigPDA

	// The "good" lane: verifier path and the mock bridge's remote_bridge_config agree on
	// l_chain_id/bridge, matching verify.rs's cross-check.
	lChainID := [32]byte{}
	lChainID[31] = 7
	remoteBridgeSender := [32]byte{}
	remoteBridgeSender[31] = 9
	allowedCaller := [32]byte{}
	allowedCaller[31] = 1

	remoteBridgeConfigPDA, _, err := solana.FindProgramAddress([][]byte{[]byte("remote_bridge_config"), lChainID[:]}, bridge)
	require.NoError(t, err)

	t.Run("setup path", func(t *testing.T) {
		// set_verifier_path requires the remote_chain_config account to already exist -
		// init_verifier_chain_config creates it.
		ix, err1 := lombard_token_pool.NewInitVerifierChainConfigInstruction(
			lombard_token_pool.SetVerifierChainConfigParams{
				RemoteChainSelector: remoteChainSelector,
				ChainConfig: lombard_token_pool.VerifierRemoteChainAdminConfig{
					RouterSigner:       admin.PublicKey(), // stand-in, not exercised by verify()
					Fee:                lombard_token_pool.UsdCents{V0: 45},
					GasForVerification: lombard_token_pool.CrossChainGas{V0: 500_000},
					PayloadSizeBytes:   448,
				},
			},
			config.LombardVerifierConfigPDA, admin.PublicKey(), remoteChainConfigPDA, solana.SystemProgramID,
		)
		require.NoError(t, err1)
		testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)

		ix, err1 = lombard_token_pool.NewSetVerifierPathInstruction(
			remoteChainSelector, lChainID, allowedCaller, remoteBridgeSender,
			config.LombardVerifierConfigPDA, admin.PublicKey(), remoteChainConfigPDA,
		)
		require.NoError(t, err1)
		testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)

		ix, err = mock_lombard_bridge.NewTestSetupRemoteBridgeConfigInstruction(
			lChainID, remoteBridgeSender,
			admin.PublicKey(), remoteBridgeConfigPDA, solana.SystemProgramID,
		)
		require.NoError(t, err1)
		testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)
	})

	// A second remote_bridge_config, deliberately registered under a *different* chain_id than
	// the path above configured - used by the "wrong path" case below.
	mismatchedChainID := [32]byte{}
	mismatchedChainID[31] = 200
	mismatchedRemoteBridgeConfigPDA, _, err := solana.FindProgramAddress([][]byte{[]byte("remote_bridge_config"), mismatchedChainID[:]}, bridge)
	require.NoError(t, err)

	t.Run("setup mismatched bridge config", func(t *testing.T) {
		ix, err1 := mock_lombard_bridge.NewTestSetupRemoteBridgeConfigInstruction(
			mismatchedChainID, remoteBridgeSender,
			admin.PublicKey(), mismatchedRemoteBridgeConfigPDA, solana.SystemProgramID,
		)
		require.NoError(t, err1)
		testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)
	})

	// verify() reconciles the bridged amount against token_transfer.amount via
	// chain_config.base.remote.decimals - decimals=9 matches the local mint's own 9 decimals set
	// above, so protocol_amount_to_svm_token_amount doesn't rescale.
	chainConfigPDA, _, err := tokens.TokenPoolChainConfigPDA(remoteChainSelector, mint, config.LombardTokenPoolProgram)
	require.NoError(t, err)
	t.Run("setup chain config", func(t *testing.T) {
		ix, err1 := lombard_token_pool.NewInitChainRemoteConfigInstruction(
			remoteChainSelector, mint,
			lombard_token_pool.RemoteConfig{TokenAddress: lombard_token_pool.RemoteAddress{Address: []byte{9, 9, 9}}, Decimals: 9},
			[32]byte{31: 1}, [32]byte{31: 1}, // the bridge path is unused here, but must be non-zero
			statePDA, chainConfigPDA, admin.PublicKey(), solana.SystemProgramID,
		)
		require.NoError(t, err1)
		testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)
	})

	// allowed_offramp: verify() checks the router's (here, mockRamp's own) registry that
	// mockRamp is a currently-allowed offramp for remoteChainSelector, rather than trusting a
	// pubkey stored in our own admin config. mismatchedAllowedOfframpPDA is a real, router-owned
	// PDA too, just registered for a *different* chain selector - used by the "wrong authority"
	// case below to exercise the handler's own address check (not just an owner/existence check).
	allowedOfframpPDA, err := state.FindAllowedOfframpPDA(remoteChainSelector, mockRamp, mockRamp)
	require.NoError(t, err)
	mismatchedAllowedOfframpPDA, err := state.FindAllowedOfframpPDA(config.SvmChainSelector, mockRamp, mockRamp)
	require.NoError(t, err)
	t.Run("setup allowed offramp", func(t *testing.T) {
		ix, err1 := arbitrary_cpi_proxy.NewAddOfframpInstruction(
			remoteChainSelector, mockRamp, allowedOfframpPDA, admin.PublicKey(), solana.SystemProgramID,
		)
		require.NoError(t, err1)
		ix2, err1 := arbitrary_cpi_proxy.NewAddOfframpInstruction(
			config.SvmChainSelector, mockRamp, mismatchedAllowedOfframpPDA, admin.PublicKey(), solana.SystemProgramID,
		)
		require.NoError(t, err1)
		testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix, ix2}, admin, config.DefaultCommitment)
	})

	// authority: the offramp-derived ccv_signer PDA, signed via the arbitrary_cpi_proxy program -
	// standing in for the real offramp's `call_verify`, which signs the same PDA under its own
	// program id.
	cpiSigner, _, err := state.FindCCVSignerPDA(config.LombardTokenPoolProgram, mockRamp)
	require.NoError(t, err)

	buildEncodedMessage := func(t *testing.T, destToken solana.PublicKey) []byte {
		tokenTransfer := protocol.TokenTransfer{
			Amount:                   big.NewInt(1_000_000),
			SourcePoolAddress:        protocol.ByteSlice{0x01, 0x02},
			SourceTokenAddress:       protocol.ByteSlice{0x03, 0x04},
			DestTokenAddress:         protocol.ByteSlice(destToken.Bytes()),
			TokenReceiver:            protocol.ByteSlice(receiver.PublicKey().Bytes()),
			ExtraData:                protocol.ByteSlice{},
			Version:                  1,
			SourcePoolAddressLength:  2,
			SourceTokenAddressLength: 2,
			DestTokenAddressLength:   uint8(len(destToken.Bytes())),            //nolint:gosec // disable G115
			TokenReceiverLength:      uint8(len(receiver.PublicKey().Bytes())), //nolint:gosec // disable G115
			ExtraDataLength:          0,
		}
		tokenTransferBytes, err1 := tokenTransfer.Encode()
		require.NoError(t, err1)

		msg := testutils.MinimalMessageV1(remoteChainSelector)
		msg.TokenTransfer = &tokenTransfer
		msg.TokenTransferLength = uint16(len(tokenTransferBytes)) //nolint:gosec // disable G115

		encoded, err1 := msg.Encode()
		require.NoError(t, err1)
		return encoded
	}

	// markDelivered stands in for the real mailbox's off-chain-relayer-driven proof
	// verification: bridge-side, this is the record `verify`'s mailbox CPI later consumes.
	markDelivered := func(t *testing.T, payloadHash [32]byte, amount uint64, boundMessageID [32]byte, destinationCaller solana.PublicKey) {
		messageInfoPDA, _, err1 := solana.FindProgramAddress([][]byte{[]byte("message"), payloadHash[:]}, mailbox)
		require.NoError(t, err1)

		var binding [36]uint8
		copy(binding[0:4], versionTag[:])
		copy(binding[4:36], boundMessageID[:])

		caller := [32]uint8(destinationCaller)
		ix, err1 := mock_lombard_mailbox.NewMarkDeliveredInstruction(
			payloadHash, receiver.PublicKey(), amount, &binding, &caller,
			admin.PublicKey(), messageInfoPDA, solana.SystemProgramID,
		)
		require.NoError(t, err1)
		testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)
	}

	// Verify's account list (20 accounts) plus a 400+-byte encoded message doesn't fit a legacy
	// transaction's 1232-byte limit, so every verify() call below goes through a lookup table -
	// matching how the real deploy sequence (deploy_lombard_chain.go) extends the offramp/
	// ccipSend LUTs with the verifier's PDAs for the same reason.
	//
	// dummyN are fixed (not freshly generated per call) so they can be pre-loaded into that LUT:
	// they fill Verify's several UncheckedAccount slots the mock programs never dereference,
	// present only for account-order parity with the real bridge/mailbox programs' account lists.
	dummyMintAuthority := solana.NewWallet().PublicKey()
	dummyBridgeConfig := solana.NewWallet().PublicKey()
	dummyMailboxConfig := solana.NewWallet().PublicKey()
	dummyLocalTokenConfig := solana.NewWallet().PublicKey()
	dummyInboundMessagePath := solana.NewWallet().PublicKey()
	dummyMessageHandled := solana.NewWallet().PublicKey()

	remoteTokenConfigPDA, _, err := solana.FindProgramAddress([][]byte{[]byte("remote_token_config"), mint.Bytes(), lChainID[:]}, bridge)
	require.NoError(t, err)
	t.Run("setup remote token config", func(t *testing.T) {
		ix, err1 := mock_lombard_bridge.NewTestSetupRemoteTokenConfigInstruction(
			mint, lChainID, [32]byte{}, 0,
			admin.PublicKey(), remoteTokenConfigPDA, solana.SystemProgramID,
		)
		require.NoError(t, err1)
		testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment)
	})

	verifyWithAllowedOfframp := func(allowedOfframpAcct, remoteChainConfigAcct, remoteBridgeConfigAcct solana.PublicKey, encodedMessage []byte, messageID [32]byte, payloadHash [32]byte) (solana.Instruction, error) {
		messageInfoPDA, _, err1 := solana.FindProgramAddress([][]byte{[]byte("message"), payloadHash[:]}, mailbox)
		if err1 != nil {
			return nil, err1
		}
		return testutils.NewVerifierVerifyInstruction(
			lombard_token_pool.VerifyParams{
				MessageId:       messageID,
				EncodedMessage:  encodedMessage,
				VerifierResults: payloadHash[:],
			},
			config.LombardTokenPoolProgram,
			cpiSigner,
			config.LombardVerifierConfigPDA,
			remoteChainConfigAcct,
			mockRamp,
			allowedOfframpAcct,
			statePDA,
			solana.TokenProgramID,
			mint,
			poolSignerPDA,
			receiverATA,
			dummyMintAuthority, // unused by mock_gmp_receive
			tokenAuthorityPDA,
			bridge,
			dummyBridgeConfig, // unused
			mailbox,
			dummyMailboxConfig,    // unused
			dummyLocalTokenConfig, // unused
			solana.SystemProgramID,
			remoteBridgeConfigAcct,
			remoteTokenConfigPDA,
			chainConfigPDA,
			dummyInboundMessagePath, // unused
			messageInfoPDA,
			dummyMessageHandled, // unused
		)
	}

	// allowedOfframpPDA: registered for remoteChainSelector in "setup allowed offramp" above.
	verify := func(remoteChainConfigAcct, remoteBridgeConfigAcct solana.PublicKey, encodedMessage []byte, messageID [32]byte, payloadHash [32]byte) (solana.Instruction, error) {
		return verifyWithAllowedOfframp(allowedOfframpPDA, remoteChainConfigAcct, remoteBridgeConfigAcct, encodedMessage, messageID, payloadHash)
	}

	messageInfoPDAFor := func(payloadHash [32]byte) solana.PublicKey {
		pda, _, err1 := solana.FindProgramAddress([][]byte{[]byte("message"), payloadHash[:]}, mailbox)
		require.NoError(t, err1)
		return pda
	}

	verifyLookupTableAddr, err := common.CreateLookupTable(ctx, solanaGoClient, admin)
	require.NoError(t, err)
	verifyLookupTableEntries := solana.PublicKeySlice{
		config.LombardVerifierConfigPDA, remoteChainConfigPDA, statePDA,
		solana.TokenProgramID, mint, poolSignerPDA, receiverATA, tokenAuthorityPDA,
		bridge, mailbox, solana.SystemProgramID, remoteBridgeConfigPDA, mismatchedRemoteBridgeConfigPDA,
		dummyMintAuthority, dummyBridgeConfig, dummyMailboxConfig, dummyLocalTokenConfig,
		remoteTokenConfigPDA, dummyInboundMessagePath, dummyMessageHandled,
		messageInfoPDAFor([32]byte{0x11}), messageInfoPDAFor([32]byte{0x22}),
		messageInfoPDAFor([32]byte{0x33}), messageInfoPDAFor([32]byte{0x44}), messageInfoPDAFor([32]byte{0x55}),
		mockRamp, allowedOfframpPDA, mismatchedAllowedOfframpPDA, chainConfigPDA, cpiSigner,
	}
	require.NoError(t, common.ExtendLookupTable(ctx, solanaGoClient, verifyLookupTableAddr, admin, verifyLookupTableEntries))
	require.NoError(t, common.AwaitSlotChange(ctx, solanaGoClient))
	verifyLookupTable := map[solana.PublicKey]solana.PublicKeySlice{verifyLookupTableAddr: verifyLookupTableEntries}

	t.Run("verify", func(t *testing.T) {
		t.Run("fails with mismatched path (wrong remote bridge config) -> PathNotConfigured", func(t *testing.T) {
			messageID := [32]byte{0xaa}
			encodedMessage := buildEncodedMessage(t, mint)
			payloadHash := [32]byte{0x11}

			ix, err := verify(remoteChainConfigPDA, mismatchedRemoteBridgeConfigPDA, encodedMessage, messageID, payloadHash)
			require.NoError(t, err)

			testutils.SendAndFailWithLookupTables(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment, verifyLookupTable,
				[]string{"Error Code: " + "PathNotConfigured"})
		})

		t.Run("fails with wrong dest_token -> InvalidPath", func(t *testing.T) {
			messageID := [32]byte{0xbb}
			wrongToken := solana.NewWallet().PublicKey()
			encodedMessage := buildEncodedMessage(t, wrongToken)
			payloadHash := [32]byte{0x22}

			ix, err := verify(remoteChainConfigPDA, remoteBridgeConfigPDA, encodedMessage, messageID, payloadHash)
			require.NoError(t, err)

			testutils.SendAndFailWithLookupTables(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment, verifyLookupTable,
				[]string{"Error Code: " + "InvalidPath"})
		})

		t.Run("fails with tampered message_id -> InvalidPayload", func(t *testing.T) {
			payloadHash := [32]byte{0x33}
			boundMessageID := [32]byte{0xcc}
			markDelivered(t, payloadHash, 1_000_000, boundMessageID, config.LombardVerifierConfigPDA)

			tamperedMessageID := [32]byte{0xdd} // doesn't match what was bound above
			encodedMessage := buildEncodedMessage(t, mint)

			ix, err := verify(remoteChainConfigPDA, remoteBridgeConfigPDA, encodedMessage, tamperedMessageID, payloadHash)
			require.NoError(t, err)

			// The inner mint CPI runs before this check but is rolled back with the rest of the
			// failed instruction, so no balance change should be observable either.
			testutils.SendAndFailWithLookupTables(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment, verifyLookupTable,
				[]string{"Error Code: " + "InvalidPayload"})

			require.Equal(t, "0", testutils.GetTokenBalance(ctx, t, solanaGoClient, receiverATA))
		})

		t.Run("fails with wrong allowed_offramp (registered for a different chain) -> Unauthorized", func(t *testing.T) {
			// allowed_offramp is checked right after the path lookup, before the mailbox CPI even
			// runs (see verify.rs), so this reuses payloadHash 0x11 without ever calling markDelivered.
			// mismatchedAllowedOfframpPDA is a real, router-owned PDA (passes the struct-level
			// owner constraint) but registered for config.SvmChainSelector, not remoteChainSelector -
			// exercising the handler's own address recomputation, not just existence/ownership.
			payloadHash := [32]byte{0x11}
			messageID := [32]byte{0xff}

			encodedMessage := buildEncodedMessage(t, mint)

			ix, err1 := verifyWithAllowedOfframp(mismatchedAllowedOfframpPDA, remoteChainConfigPDA, remoteBridgeConfigPDA, encodedMessage, messageID, payloadHash)
			require.NoError(t, err1)

			testutils.SendAndFailWithLookupTables(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment, verifyLookupTable,
				[]string{"Error Code: " + "Unauthorized"})

			require.Equal(t, "0", testutils.GetTokenBalance(ctx, t, solanaGoClient, receiverATA))
		})

		t.Run("fails when destination caller is the per-mint State PDA -> InvalidDestinationCaller", func(t *testing.T) {
			// The mailbox handler is the chain-level verifier config PDA, so a message addressed
			// to the per-mint State PDA must not be consumable.
			payloadHash := [32]byte{0x55}
			messageID := [32]byte{0xab}
			markDelivered(t, payloadHash, 1_000_000, messageID, statePDA)

			encodedMessage := buildEncodedMessage(t, mint)

			ix, err := verify(remoteChainConfigPDA, remoteBridgeConfigPDA, encodedMessage, messageID, payloadHash)
			require.NoError(t, err)

			testutils.SendAndFailWithLookupTables(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment, verifyLookupTable,
				[]string{"Error Code: " + "InvalidDestinationCaller"})

			require.Equal(t, "0", testutils.GetTokenBalance(ctx, t, solanaGoClient, receiverATA))
		})

		t.Run("succeeds and mints the real SPL transfer", func(t *testing.T) {
			payloadHash := [32]byte{0x44}
			messageID := [32]byte{0xee}
			amount := uint64(1_000_000)
			markDelivered(t, payloadHash, amount, messageID, config.LombardVerifierConfigPDA)

			encodedMessage := buildEncodedMessage(t, mint)

			ix, err := verify(remoteChainConfigPDA, remoteBridgeConfigPDA, encodedMessage, messageID, payloadHash)
			require.NoError(t, err)

			result := testutils.SendAndConfirmWithLookupTables(ctx, t, solanaGoClient, []solana.Instruction{ix}, admin, config.DefaultCommitment, verifyLookupTable)
			require.NotNil(t, result)

			require.Equal(t, "1000000", testutils.GetTokenBalance(ctx, t, solanaGoClient, receiverATA))
		})
	})
}
