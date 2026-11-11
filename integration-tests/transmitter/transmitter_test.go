package transmitter

import (
	"context"
	"testing"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/system"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-ccv/executor"
	"github.com/smartcontractkit/chainlink-ccv/protocol"
	"github.com/smartcontractkit/chainlink-common/keystore"
	commoncfg "github.com/smartcontractkit/chainlink-common/pkg/config"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	solconfig "github.com/smartcontractkit/chainlink-solana/pkg/solana/config"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/ccip_common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/ccip_offramp"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/committee_verifier"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/rmn_remote"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/test_ccip_receiver"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/config"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/testutils"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/testutils/eth"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/ccip"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	"github.com/smartcontractkit/chainlink-ccip-solana/pkg/destinationreader"
	"github.com/smartcontractkit/chainlink-ccip-solana/pkg/logpoller"
	"github.com/smartcontractkit/chainlink-ccip-solana/pkg/solanarpc"
	"github.com/smartcontractkit/chainlink-ccip-solana/pkg/transmitter"
)

const committeeThreshold = 3

// TestBufferedFlow runs the real transmitter against a local validator with payloads that are
// too big to execute inline, so every message takes the buffered flow.
func TestBufferedFlow(t *testing.T) {
	env := testutils.NewTestEnv(t).WithTrafficConfig().Build()
	ctx := env.Ctx()
	signers := configureCommitteeVerifier(ctx, t, env.Client)
	tr, ks, rpcClient := newTransmitter(ctx, t, env)

	t.Run("executes a valid message", func(t *testing.T) {
		report := newReport(t, signers, 1, validBlob(t), 1500)
		require.NoError(t, tr.ConvertAndWriteMessageToChain(ctx, report))
	})

	t.Run("more than 64 messaging accounts is terminal", func(t *testing.T) {
		args := ccip.SVMExecutorArgs{SvmDestBlob: ccip_common.SvmDestBlob{
			UseAta:   ccip_common.SvmTokenReceiverUsage_DeriveAtaDontCreate,
			Accounts: newKeys(65),
		}}
		blob, err := args.AbiEncodePacked()
		require.NoError(t, err)
		requireTerminal(ctx, t, env, tr, newReport(t, signers, 2, blob, 1500))
	})

	t.Run("malformed dest blob is terminal", func(t *testing.T) {
		requireTerminal(ctx, t, env, tr, newReport(t, signers, 3, []byte{0xff, 0xff, 0xff, 0xff, 0xff}, 1500))
	})

	t.Run("exec tx still oversized after buffering is terminal and closes the buffer", func(t *testing.T) {
		// 40 messaging accounts that are in no lookup table need 1280 bytes of account keys alone.
		args := ccip.SVMExecutorArgs{SvmDestBlob: ccip_common.SvmDestBlob{
			UseAta:   ccip_common.SvmTokenReceiverUsage_DeriveAtaDontCreate,
			Accounts: newKeys(40),
		}}
		blob, err := args.AbiEncodePacked()
		require.NoError(t, err)
		report := newReport(t, signers, 4, blob, 1500)
		err = tr.ConvertAndWriteMessageToChain(ctx, report)
		require.ErrorIs(t, err, executor.ErrMessageRejectedByTransmitter)
		require.ErrorContains(t, err, "still oversized after buffering")
		requireNoBuffer(ctx, t, env, tr, report)
	})

	// A message that the program rejects after the buffer writes: the exec tx is sent and fails, and the
	// transmitter closes the buffer before it returns, so a retry of the same message can reuse the
	// buffer PDA. After the cause is gone, the retry executes.
	t.Run("rejected attempt closes the buffer, then the same message executes", func(t *testing.T) {
		setCurse(ctx, t, env, true)
		cursed := true
		t.Cleanup(func() {
			if cursed {
				setCurse(context.Background(), t, env, false)
			}
		})
		report := newReport(t, signers, 5, validBlob(t), 1500)

		err := tr.ConvertAndWriteMessageToChain(ctx, report)
		require.ErrorContains(t, err, "failed with status")
		require.NotErrorIs(t, err, executor.ErrMessageRejectedByTransmitter)
		requireNoBuffer(ctx, t, env, tr, report)

		err = tr.ConvertAndWriteMessageToChain(ctx, report)
		require.ErrorContains(t, err, "failed with status")
		requireNoBuffer(ctx, t, env, tr, report)

		setCurse(ctx, t, env, false)
		cursed = false
		require.NoError(t, tr.ConvertAndWriteMessageToChain(ctx, report))
		requireNoBuffer(ctx, t, env, tr, report)
	})

	// A receiver that rejects the message makes the exec tx land failed after InProgress, which
	// leaves the on-chain mark for manual execution. The reader caches the attempt but cannot
	// simulate an oversized message, so the attempt does not stop the message.
	t.Run("receiver revert lands a failed attempt that does not stop the message", func(t *testing.T) {
		dr := newDestinationReader(ctx, t, rpcClient, tr.FeePayer())
		setRejectAll(ctx, t, env, true)
		rejecting := true
		t.Cleanup(func() {
			if rejecting {
				setRejectAll(context.Background(), t, env, false)
			}
		})
		report := newReport(t, signers, 8, validBlob(t), 1500)

		err := tr.ConvertAndWriteMessageToChain(ctx, report)
		require.ErrorContains(t, err, "buffered flow: execute")
		require.NotErrorIs(t, err, executor.ErrMessageRejectedByTransmitter)
		requireNoBuffer(ctx, t, env, tr, report)

		require.Eventually(t, func() bool {
			attempts, attemptsErr := dr.GetExecutionAttempts(ctx, report.Message)
			return attemptsErr == nil && len(attempts) == 1
		}, 30*time.Second, 500*time.Millisecond, "the failed attempt must be cached")
		attempts, err := dr.GetExecutionAttempts(ctx, report.Message)
		require.NoError(t, err)
		require.Equal(t, report.CCVData, attempts[0].Report.CCVData, "the cached attempt carries the transmitted CCV data")
		require.Equal(t, uint64(report.Message.ExecutionGasLimit), attempts[0].TransactionGasLimit.Uint64(), "the cached attempt carries the honest gas limit")
		ccv := protocol.UnknownAddress(config.CommitteeVerifierProgram.Bytes())
		check, err := dr.HasHonestAttempt(ctx, report.Message,
			[]protocol.VerifierResult{{VerifierDestAddress: ccv, CCVData: report.CCVData[0]}},
			protocol.CCVAddressInfo{RequiredCCVs: []protocol.UnknownAddress{ccv}})
		require.NoError(t, err)
		require.False(t, check.Honest)
		require.True(t, check.NotHonestAttemptExists)

		setRejectAll(ctx, t, env, false)
		rejecting = false
		require.NoError(t, tr.ConvertAndWriteMessageToChain(ctx, report))
		requireNoBuffer(ctx, t, env, tr, report)
	})

	// A buffer left by an earlier run (cleanup timeout or restart) makes one attempt fail. Its
	// cleanup closes the buffer, so the next attempt executes.
	t.Run("buffer left by an earlier run costs one failed attempt", func(t *testing.T) {
		report := newReport(t, signers, 6, validBlob(t), 1500)
		writeLeftoverBuffer(ctx, t, env, tr, ks, report)

		err := tr.ConvertAndWriteMessageToChain(ctx, report)
		require.ErrorContains(t, err, "write chunks")
		require.NotErrorIs(t, err, executor.ErrMessageRejectedByTransmitter)
		requireNoBuffer(ctx, t, env, tr, report)

		require.NoError(t, tr.ConvertAndWriteMessageToChain(ctx, report))
		requireNoBuffer(ctx, t, env, tr, report)
	})

	// execute_v2 does not read the dest blob of a message without tokens and without a receiver
	// call, so a malformed blob must not be rejected.
	t.Run("malformed dest blob that is never read executes", func(t *testing.T) {
		report := newReport(t, signers, 7, []byte{0xff, 0xff, 0xff, 0xff, 0xff}, 1500, func(m *protocol.Message) {
			m.Receiver = make([]byte, solana.PublicKeyLength)
		})
		require.NoError(t, tr.ConvertAndWriteMessageToChain(ctx, report))
		requireNoBuffer(ctx, t, env, tr, report)
	})
}

// writeLeftoverBuffer writes one chunk of other data to the buffer PDA of the message, signed by the
// transmitter's fee payer, as an earlier run of the transmitter could leave it.
func writeLeftoverBuffer(ctx context.Context, t *testing.T, env *testutils.TestEnv, tr *transmitter.SolanaContractTransmitter, ks keystore.Keystore, report protocol.AbstractAggregatedReport) {
	t.Helper()
	msgID := report.Message.MustMessageID()
	bufferPDA, _, err := state.FindExecutionInputsBufferPDA(msgID, tr.FeePayer(), config.CcipOfframpProgram)
	require.NoError(t, err)
	configPDA, _, err := state.FindOfframpConfigPDA(config.CcipOfframpProgram)
	require.NoError(t, err)
	eventAuthorityPDA, _, err := state.FindEventAuthorityPDA(config.CcipOfframpProgram)
	require.NoError(t, err)
	ix, err := ccip_offramp.NewBufferExecutionInputsInstruction(
		ccip_offramp.BufferExecutionInputsParams{BufferId: msgID, TotalLength: 100, Chunk: make([]byte, 100), ChunkIndex: 0, NumChunks: 1},
		bufferPDA, configPDA, tr.FeePayer(), solana.SystemProgramID, eventAuthorityPDA, config.CcipOfframpProgram,
	)
	require.NoError(t, err)

	recent, err := env.Client.GetLatestBlockhash(ctx, config.DefaultCommitment)
	require.NoError(t, err)
	tx, err := solana.NewTransaction([]solana.Instruction{ix}, recent.Value.Blockhash, solana.TransactionPayer(tr.FeePayer()))
	require.NoError(t, err)
	msg, err := tx.Message.MarshalBinary()
	require.NoError(t, err)
	signed, err := ks.Sign(ctx, keystore.SignRequest{KeyName: transmitter.DefaultKeyName, Data: msg})
	require.NoError(t, err)
	tx.Signatures = []solana.Signature{solana.SignatureFromBytes(signed.Signature)}
	_, err = env.Client.SendTransactionWithOpts(ctx, tx, rpc.TransactionOpts{PreflightCommitment: config.DefaultCommitment})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		_, getErr := env.Client.GetAccountInfoWithOpts(ctx, bufferPDA, &rpc.GetAccountInfoOpts{Commitment: config.DefaultCommitment})
		return getErr == nil
	}, 30*time.Second, 500*time.Millisecond)
}

// requireNoBuffer asserts the buffer PDA of the message does not exist.
func requireNoBuffer(ctx context.Context, t *testing.T, env *testutils.TestEnv, tr *transmitter.SolanaContractTransmitter, report protocol.AbstractAggregatedReport) {
	t.Helper()
	bufferPDA, _, err := state.FindExecutionInputsBufferPDA(report.Message.MustMessageID(), tr.FeePayer(), config.CcipOfframpProgram)
	require.NoError(t, err)
	_, err = env.Client.GetAccountInfoWithOpts(ctx, bufferPDA, &rpc.GetAccountInfoOpts{Commitment: config.DefaultCommitment})
	require.ErrorIs(t, err, rpc.ErrNotFound, "no buffer must exist")
}

// setCurse curses or uncurses the EVM source chain on RMN Remote. CcipAdmin owns RMN Remote after setup.
func setCurse(ctx context.Context, t *testing.T, env *testutils.TestEnv, cursed bool) {
	t.Helper()
	subject := testutils.ChainCurseSubject(config.EvmChainSelector)
	var ix solana.Instruction
	var err error
	if cursed {
		ix, err = rmn_remote.NewCurseInstruction(subject, config.RMNRemoteConfigPDA, testutils.CcipAdmin.PublicKey(), config.RMNRemoteCursesPDA, solana.SystemProgramID)
	} else {
		ix, err = rmn_remote.NewUncurseInstruction(subject, config.RMNRemoteConfigPDA, testutils.CcipAdmin.PublicKey(), config.RMNRemoteCursesPDA, solana.SystemProgramID)
	}
	require.NoError(t, err)
	testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ix}, testutils.CcipAdmin, config.DefaultCommitment)
}

// setRejectAll makes the test receiver reject or accept every message. testutils.User owns the receiver.
func setRejectAll(ctx context.Context, t *testing.T, env *testutils.TestEnv, reject bool) {
	t.Helper()
	ix, err := test_ccip_receiver.NewSetRejectAllInstruction(reject, config.ReceiverTargetAccountPDA, testutils.User.PublicKey())
	require.NoError(t, err)
	testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ix}, testutils.User, config.DefaultCommitment)
}

// newDestinationReader wires the reader as the executor factory does: its log poller includes reverted txs.
func newDestinationReader(ctx context.Context, t *testing.T, rpcClient *solanarpc.Client, simFeePayer solana.PublicKey) *destinationreader.DestinationReader {
	t.Helper()
	lpCfg := logpoller.DefaultConfig()
	lpCfg.IncludeReverted = true
	lp, err := logpoller.NewService(rpcClient, config.CcipOfframpProgram, "transmitter-integration", logger.TestSugared(t), lpCfg)
	require.NoError(t, err)
	dr, err := destinationreader.NewDestinationReader(destinationreader.Params{
		Lggr:             logger.Test(t),
		ChainSelector:    protocol.ChainSelector(config.SvmChainSelector),
		RPC:              rpcClient,
		OfframpProgramID: config.CcipOfframpProgram,
		RMNRemoteAddress: config.RMNRemoteProgram,
		LogPoller:        lp,
	})
	require.NoError(t, err)
	dr.SetSimulationFeePayer(simFeePayer)
	require.NoError(t, dr.Start(ctx))
	t.Cleanup(func() { require.NoError(t, dr.Close()) })
	return dr
}

// requireTerminal asserts the transmitter rejects the message for good before it writes a buffer.
func requireTerminal(ctx context.Context, t *testing.T, env *testutils.TestEnv, tr *transmitter.SolanaContractTransmitter, report protocol.AbstractAggregatedReport) {
	t.Helper()
	err := tr.ConvertAndWriteMessageToChain(ctx, report)
	require.ErrorIs(t, err, executor.ErrMessageRejectedByTransmitter)
	requireNoBuffer(ctx, t, env, tr, report)
}

func newTransmitter(ctx context.Context, t *testing.T, env *testutils.TestEnv) (*transmitter.SolanaContractTransmitter, keystore.Keystore, *solanarpc.Client) {
	t.Helper()
	// multinode expects the genesis hash as the chain ID, and a local validator gets a new one per run
	genesis, err := env.Client.GetGenesisHash(ctx)
	require.NoError(t, err)
	chainID := genesis.String()

	name := "primary"
	rpcClient, err := solanarpc.NewClient(ctx, solanarpc.Config{
		ChainID:    chainID,
		Nodes:      []*solconfig.Node{{Name: &name, URL: commoncfg.MustParseURL(env.RPCURL)}},
		RPCTimeout: 10 * time.Second,
	}, logger.Test(t))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, rpcClient.Close()) })
	// multinode marks the node live asynchronously after start
	require.Eventually(t, func() bool {
		_, blockhashErr := rpcClient.GetLatestBlockhash(ctx)
		return blockhashErr == nil
	}, 30*time.Second, 200*time.Millisecond)

	ks, err := keystore.LoadKeystore(ctx, keystore.NewMemoryStorage(), "test-password", keystore.WithScryptParams(keystore.FastScryptParams))
	require.NoError(t, err)
	_, err = ks.CreateKeys(ctx, keystore.CreateKeysRequest{Keys: []keystore.CreateKeyRequest{{KeyName: transmitter.DefaultKeyName, KeyType: keystore.Ed25519}}})
	require.NoError(t, err)

	tr, err := transmitter.NewSolanaContractTransmitter(ctx, transmitter.Params{
		Lggr:             logger.Test(t),
		ChainSelector:    protocol.ChainSelector(config.SvmChainSelector),
		ChainID:          chainID,
		OfframpProgramID: config.CcipOfframpProgram,
		RPC:              rpcClient,
		Keystore:         ks,
		KeyName:          transmitter.DefaultKeyName,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, tr.Close()) })

	fund := system.NewTransferInstruction(100*solana.LAMPORTS_PER_SOL, testutils.User.PublicKey(), tr.FeePayer()).Build()
	testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{fund}, testutils.User, config.DefaultCommitment)
	return tr, ks, rpcClient
}

// configureCommitteeVerifier sets a threshold-of-5 signer set for the EVM lane on the committee verifier.
func configureCommitteeVerifier(ctx context.Context, t *testing.T, client *rpc.Client) []eth.Signer {
	t.Helper()
	privateKeys, err := eth.GenerateEthPrivateKeys(5)
	require.NoError(t, err)
	signers, err := eth.GetEvmSigners(privateKeys)
	require.NoError(t, err)
	addresses := make([][20]uint8, len(signers))
	for i, s := range signers {
		addresses[i] = s.Address
	}

	admin := testutils.Admin
	lggr := logger.Test(t)
	chainConfig, err := ccip.ResolveCVEditChainConfigAccounts(ctx, client, config.CommitteeVerifierProgram, committee_verifier.SetChainConfigParams{
		RemoteChainSelector: config.EvmChainSelector,
		ChainConfig:         committee_verifier.RemoteChainAdminConfig{RouterSigner: config.CCVSignerPDA},
	}, admin.PublicKey(), lggr)
	require.NoError(t, err)
	testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{chainConfig.Ix}, admin, config.DefaultCommitment)

	signerConfig, err := ccip.ResolveCVSetSignerConfigAccounts(ctx, client, config.CommitteeVerifierProgram, committee_verifier.SetSignerConfigParams{
		SourceChainSelector: config.EvmChainSelector,
		Threshold:           committeeThreshold,
		Signers:             addresses,
	}, admin.PublicKey(), lggr)
	require.NoError(t, err)
	testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{signerConfig.Ix}, admin, config.DefaultCommitment)
	return signers
}

// validBlob is the dest blob of testutils.MinimalMessageV1.
func validBlob(t *testing.T) []byte {
	t.Helper()
	return testutils.MinimalMessageV1(config.EvmChainSelector).DestBlob
}

// newReport builds a signed report whose data is dataLen bytes, which pushes the exec tx over the
// size limit. edits change the message before it is signed.
func newReport(t *testing.T, signers []eth.Signer, seq uint64, blob []byte, dataLen int, edits ...func(*protocol.Message)) protocol.AbstractAggregatedReport {
	t.Helper()
	msg := testutils.MinimalMessageV1(config.EvmChainSelector)
	msg.SequenceNumber = protocol.SequenceNumber(seq)
	msg.DestBlob = blob
	msg.DestBlobLength = uint16(len(blob)) //nolint:gosec // test blobs are small
	msg.Data = make([]byte, dataLen)
	msg.DataLength = uint16(dataLen) //nolint:gosec // test data is small
	for _, edit := range edits {
		edit(&msg)
	}

	msgID, err := msg.MessageID()
	require.NoError(t, err)
	results, err := testutils.SignCommitteeVerifierMessage([32]byte(msgID), signers, committeeThreshold)
	require.NoError(t, err)
	return protocol.AbstractAggregatedReport{
		CCVS:    []protocol.UnknownAddress{config.CommitteeVerifierProgram.Bytes()},
		CCVData: [][]byte{results},
		Message: msg,
	}
}

func newKeys(n int) []solana.PublicKey {
	keys := make([]solana.PublicKey, n)
	for i := range keys {
		keys[i] = solana.NewWallet().PublicKey()
	}
	return keys
}
