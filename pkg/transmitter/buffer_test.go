package transmitter

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-ccv/executor"
	"github.com/smartcontractkit/chainlink-common/pkg/config"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	commontypes "github.com/smartcontractkit/chainlink-common/pkg/types"
	txmmocks "github.com/smartcontractkit/chainlink-solana/pkg/solana/txm/mocks"
	txmutils "github.com/smartcontractkit/chainlink-solana/pkg/solana/txm/utils"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/ccip_offramp"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/ccip"
	solcommon "github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
)

func TestExtractChunks(t *testing.T) {
	t.Parallel()

	emptyTxSize := 200
	chunkSize := solcommon.MaxSolanaTxSize - emptyTxSize - 1

	t.Run("exact multiple", func(t *testing.T) {
		t.Parallel()
		raw := make([]byte, chunkSize*3)
		chunks, err := splitAndBuildReportChunks(raw, emptyTxSize)
		require.NoError(t, err)
		require.Len(t, chunks, 3)
		for _, c := range chunks {
			require.Len(t, c, chunkSize)
		}
	})

	t.Run("remainder chunk", func(t *testing.T) {
		t.Parallel()
		raw := make([]byte, chunkSize*2+50)
		chunks, err := splitAndBuildReportChunks(raw, emptyTxSize)
		require.NoError(t, err)
		require.Len(t, chunks, 3)
		require.Len(t, chunks[0], chunkSize)
		require.Len(t, chunks[1], chunkSize)
		require.Len(t, chunks[2], 50)
	})

	t.Run("rejects empty tx overhead", func(t *testing.T) {
		t.Parallel()
		_, err := splitAndBuildReportChunks([]byte{1}, solcommon.MaxSolanaTxSize)
		require.Error(t, err)
	})
}

func TestExtractChunks_MaxChunks(t *testing.T) {
	t.Parallel()
	emptyTxSize := 200
	chunkSize := solcommon.MaxSolanaTxSize - emptyTxSize - 1
	raw := make([]byte, chunkSize*maxNumChunks+1)
	chunks, err := splitAndBuildReportChunks(raw, emptyTxSize)
	require.NoError(t, err)
	require.Greater(t, len(chunks), maxNumChunks)
}

func TestFindExecutionInputsBufferPDA(t *testing.T) {
	t.Parallel()
	var bufferID [32]byte
	copy(bufferID[:], []byte("test-buffer-id-fixed-32-bytes!!"))
	authority := solana.NewWallet().PublicKey()

	pda, _, err := state.FindExecutionInputsBufferPDA(bufferID, authority, ccip_offramp.ProgramID)
	require.NoError(t, err)

	expected, _, err := solana.FindProgramAddress(
		[][]byte{[]byte("execution_inputs_buffer"), bufferID[:], authority.Bytes()},
		ccip_offramp.ProgramID,
	)
	require.NoError(t, err)
	require.Equal(t, expected, pda)
}

func TestCalculateTxSize_NilTx(t *testing.T) {
	t.Parallel()
	_, err := calculateTxSizeInBytes(nil)
	require.Error(t, err)
}

func TestCalculateTxSize_SimpleTx(t *testing.T) {
	t.Parallel()
	authority := solana.NewWallet().PublicKey()
	ix := solana.NewInstruction(
		solana.SystemProgramID,
		solana.AccountMetaSlice{
			solana.Meta(authority).WRITE().SIGNER(),
			solana.Meta(solana.MustPublicKeyFromBase58("11111111111111111111111111111111")),
		},
		[]byte{2, 0, 0, 0, 0, 0, 0, 0, 0},
	)
	tx, err := solana.NewTransaction(
		[]solana.Instruction{ix},
		solana.Hash{},
		solana.TransactionPayer(authority),
	)
	require.NoError(t, err)

	size, err := calculateTxSizeInBytes(tx)
	require.NoError(t, err)
	require.Positive(t, size)
	require.LessOrEqual(t, size, solcommon.MaxSolanaTxSize)
}

// blockhashRPC returns a new (blockhash, lastValid) pair on each call and fails on call failOn (0 = never).
type blockhashRPC struct {
	RPCClient
	calls  byte
	failOn byte
}

func (f *blockhashRPC) GetLatestBlockhash(context.Context) (*rpc.GetLatestBlockhashResult, error) {
	f.calls++
	if f.calls == f.failOn {
		return nil, errors.New("rpc down")
	}
	return &rpc.GetLatestBlockhashResult{Value: &rpc.LatestBlockhashResult{
		Blockhash:            solana.Hash{f.calls},
		LastValidBlockHeight: 100 + uint64(f.calls),
	}}, nil
}

func TestWriteChunks_NewBlockhashPerChunk(t *testing.T) {
	t.Parallel()
	chunks := [][]byte{{1}, {2}, {3}}

	type enqueued struct {
		blockhash solana.Hash
		lastValid uint64
	}
	run := func(t *testing.T, failOn byte) ([]enqueued, error) {
		fakeRPC := &blockhashRPC{failOn: failOn}
		txm := txmmocks.NewTxManager(t)
		var got []enqueued
		txm.EXPECT().Enqueue(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			RunAndReturn(func(_ context.Context, _ string, tx *solana.Transaction, _ *string, lastValid uint64, _ ...txmutils.SetTxConfig) error {
				got = append(got, enqueued{tx.Message.RecentBlockhash, lastValid})
				return nil
			})
		txm.EXPECT().GetTransactionStatus(mock.Anything, mock.Anything).Return(commontypes.Finalized, nil).Maybe()

		tr := &SolanaContractTransmitter{
			lggr:      logger.Sugared(logger.Nop()),
			rpcClient: fakeRPC,
			txm:       txm,
			cfg:       Config{BufferedConfig: BufferedConfig{TxStatusPollInterval: *config.MustNewDuration(time.Millisecond)}},
		}
		b := &bufferedExecution{t: tr}
		return got, b.writeChunks(t.Context(), []byte{1, 2, 3}, chunks)
	}

	t.Run("each chunk gets its own blockhash", func(t *testing.T) {
		t.Parallel()
		got, err := run(t, 0)
		require.NoError(t, err)
		require.Equal(t, []enqueued{
			{solana.Hash{1}, 101},
			{solana.Hash{2}, 102},
			{solana.Hash{3}, 103},
		}, got)
	})

	t.Run("refresh failure on a later chunk stops enqueueing", func(t *testing.T) {
		t.Parallel()
		got, err := run(t, 2)
		require.ErrorContains(t, err, "refresh blockhash for chunk 1")
		require.Equal(t, []enqueued{{solana.Hash{1}, 101}}, got)
	})
}

func TestPrepareChunksLimit(t *testing.T) {
	t.Parallel()
	b := &bufferedExecution{t: &SolanaContractTransmitter{lggr: logger.Sugared(logger.Nop())}}
	overhead, err := getBaseBufferTxSize(b.t.offrampProgramID, b.bufferID, b.bufferPDA, b.configPDA, b.authority, b.eventAuthorityPDA)
	require.NoError(t, err)
	payload := make([]byte, (solcommon.MaxSolanaTxSize-overhead-1)*maxNumChunks)
	chunks, err := b.prepareChunks(payload)
	require.NoError(t, err)
	require.Len(t, chunks, maxNumChunks)
	_, err = b.prepareChunks(append(payload, 0))
	require.ErrorIs(t, err, executor.ErrMessageRejectedByTransmitter)
}

func TestBufferIxsTargetTransmitterOfframp(t *testing.T) {
	t.Parallel()
	programID := solana.NewWallet().PublicKey()
	var bufferID [32]byte

	bufferIx, err := buildBufferIx(bufferID, 3, []byte{1, 2, 3}, 0, 1, programID, programID, programID, programID, programID)
	require.NoError(t, err)
	closeIx, err := buildCloseBufferIx(bufferID, programID, programID, programID, programID, programID)
	require.NoError(t, err)

	require.Equal(t, programID, bufferIx.ProgramID())
	require.Equal(t, programID, closeIx.ProgramID())
}

func TestExecuteAndWaitOversized(t *testing.T) {
	t.Parallel()
	b := &bufferedExecution{t: &SolanaContractTransmitter{lggr: logger.Sugared(logger.Nop())}}
	ix := solana.NewInstruction(ccip_offramp.ProgramID, solana.AccountMetaSlice{solana.Meta(solana.NewWallet().PublicKey()).SIGNER()}, make([]byte, solcommon.MaxSolanaTxSize))
	err := b.executeAndWait(t.Context(), ccip.ResolveAccountsResult{Ix: ix}, nil)
	require.ErrorIs(t, err, executor.ErrMessageRejectedByTransmitter)
}

// TestExecuteAndWaitFailedTx pins that a program error is sent and comes back as a retryable error.
func TestExecuteAndWaitFailedTx(t *testing.T) {
	t.Parallel()
	txm := txmmocks.NewTxManager(t)
	txm.EXPECT().Enqueue(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	txm.EXPECT().GetTransactionStatus(mock.Anything, mock.Anything).Return(commontypes.Fatal, nil)
	b := &bufferedExecution{
		t: &SolanaContractTransmitter{
			lggr: logger.Sugared(logger.Nop()),
			txm:  txm,
			cfg:  Config{BufferedConfig: BufferedConfig{TxStatusPollInterval: *config.MustNewDuration(time.Millisecond)}},
		},
		authority:    solana.NewWallet().PublicKey(),
		computeUnits: 300_000,
	}
	ix := solana.NewInstruction(ccip_offramp.ProgramID, solana.AccountMetaSlice{solana.Meta(b.authority).SIGNER()}, []byte{1})

	err := b.executeAndWait(t.Context(), ccip.ResolveAccountsResult{Ix: ix}, nil)
	require.ErrorContains(t, err, "failed with status")
	require.NotErrorIs(t, err, executor.ErrMessageRejectedByTransmitter)
	require.Len(t, b.inFlightTxs, 1, "the failed exec tx must stay tracked for cleanup")
}

// chainRPC is a fake chain whose block height goes up by one on each read.
type chainRPC struct {
	RPCClient
	height uint64
}

func (f *chainRPC) GetLatestBlockHeight(context.Context) (uint64, error) {
	f.height++
	return f.height, nil
}

func (f *chainRPC) GetLatestBlockhash(context.Context) (*rpc.GetLatestBlockhashResult, error) {
	return &rpc.GetLatestBlockhashResult{Value: &rpc.LatestBlockhashResult{LastValidBlockHeight: f.height + 5}}, nil
}

func newChainTransmitter(fake *chainRPC, txm *txmmocks.TxManager) *SolanaContractTransmitter {
	return &SolanaContractTransmitter{
		lggr:      logger.Sugared(logger.Nop()),
		rpcClient: fake,
		txm:       txm,
		cfg: Config{BufferedConfig: BufferedConfig{
			TxStatusPollInterval: *config.MustNewDuration(time.Millisecond),
			BufferedFlowTimeout:  *config.MustNewDuration(time.Minute),
		}},
	}
}

func TestWaitUntilNoTxCanLand(t *testing.T) {
	t.Parallel()
	const lastValid = 10
	wait := func(ctx context.Context, fake *chainRPC, txm *txmmocks.TxManager) []inFlightTx {
		b := &bufferedExecution{t: newChainTransmitter(fake, txm), inFlightTxs: []inFlightTx{{id: "a", lastValid: lastValid}}}
		b.waitUntilNoTxCanLand(ctx, "test")
		return b.inFlightTxs
	}

	t.Run("finalized returns at once", func(t *testing.T) {
		t.Parallel()
		txm := txmmocks.NewTxManager(t)
		txm.EXPECT().GetTransactionStatus(mock.Anything, "a").Return(commontypes.Finalized, nil).Once()
		fake := &chainRPC{}
		require.Empty(t, wait(t.Context(), fake, txm))
		require.Equal(t, uint64(1), fake.height, "done on the first poll")
	})

	type result struct {
		status commontypes.TransactionStatus
		err    error
	}
	for name, status := range map[string]result{
		"failed":       {commontypes.Failed, nil},
		"fatal":        {commontypes.Fatal, nil},
		"unknown":      {commontypes.Unknown, nil},
		"status error": {commontypes.Unknown, errors.New("not found")},
	} {
		t.Run(name+" waits past the last valid height", func(t *testing.T) {
			t.Parallel()
			txm := txmmocks.NewTxManager(t)
			txm.EXPECT().GetTransactionStatus(mock.Anything, "a").Return(status.status, status.err)
			fake := &chainRPC{}
			require.Empty(t, wait(t.Context(), fake, txm))
			require.Equal(t, uint64(lastValid+1), fake.height)
		})
	}

	for name, status := range map[string]commontypes.TransactionStatus{
		"pending":     commontypes.Pending,
		"unconfirmed": commontypes.Unconfirmed,
	} {
		t.Run(name+" is kept past the last valid height", func(t *testing.T) {
			t.Parallel()
			txm := txmmocks.NewTxManager(t)
			txm.EXPECT().GetTransactionStatus(mock.Anything, "a").Return(status, nil)
			ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
			defer cancel()
			fake := &chainRPC{}
			b := &bufferedExecution{t: newChainTransmitter(fake, txm), inFlightTxs: []inFlightTx{{id: "a", lastValid: lastValid}}}
			b.waitUntilNoTxCanLand(ctx, "test")
			require.Len(t, b.inFlightTxs, 1)
			require.Greater(t, fake.height, uint64(lastValid))
		})
	}

	t.Run("deadline keeps the tx", func(t *testing.T) {
		t.Parallel()
		txm := txmmocks.NewTxManager(t)
		txm.EXPECT().GetTransactionStatus(mock.Anything, "a").Return(commontypes.Pending, nil)
		ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
		defer cancel()
		b := &bufferedExecution{t: newChainTransmitter(&chainRPC{}, txm), inFlightTxs: []inFlightTx{{id: "a", lastValid: math.MaxUint64}}}
		b.waitUntilNoTxCanLand(ctx, "test")
		require.Len(t, b.inFlightTxs, 1)
	})
}

func TestEnqueueDisablesFeeBump(t *testing.T) {
	t.Parallel()
	txm := txmmocks.NewTxManager(t)
	got := txmutils.TxConfig{FeeBumpPeriod: time.Second}
	txm.EXPECT().Enqueue(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, _ string, _ *solana.Transaction, _ *string, _ uint64, opts ...txmutils.SetTxConfig) error {
			for _, opt := range opts {
				opt(&got)
			}
			return nil
		})
	b := &bufferedExecution{t: newChainTransmitter(&chainRPC{}, txm)}
	require.NoError(t, b.enqueue(t.Context(), nil, "a"))
	require.Zero(t, got.FeeBumpPeriod)
	require.Len(t, b.inFlightTxs, 1)
}

// TestCleanupOrder checks that the close is enqueued only after every write can no longer land,
// and that cleanup returns only after the close can no longer land.
func TestCleanupOrder(t *testing.T) {
	t.Parallel()
	fake := &chainRPC{}
	txm := txmmocks.NewTxManager(t)
	// The txm dropped both writes (for example on its confirm timeout), and later the close.
	txm.EXPECT().GetTransactionStatus(mock.Anything, mock.Anything).Return(commontypes.Failed, nil)
	var closeHeight, closeLastValid uint64
	txm.EXPECT().Enqueue(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, _ string, _ *solana.Transaction, _ *string, lastValid uint64, _ ...txmutils.SetTxConfig) error {
			closeHeight, closeLastValid = fake.height, lastValid
			return nil
		}).Once()

	b := &bufferedExecution{
		t:           newChainTransmitter(fake, txm),
		authority:   solana.NewWallet().PublicKey(),
		inFlightTxs: []inFlightTx{{id: "w0", lastValid: 10}, {id: "w1", lastValid: 20}},
	}
	b.fail(t.Context(), "write chunks", errors.New("dropped"))

	require.Equal(t, uint64(20+1), closeHeight, "the close went out before the last write could no longer land")
	require.Greater(t, fake.height, closeLastValid, "cleanup returned while the close could still land")
	require.Empty(t, b.inFlightTxs)
}
