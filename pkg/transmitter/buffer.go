package transmitter

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/google/uuid"

	"github.com/smartcontractkit/chainlink-ccv/executor"
	"github.com/smartcontractkit/chainlink-ccv/protocol"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	commontypes "github.com/smartcontractkit/chainlink-common/pkg/types"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/fees"
	txmutils "github.com/smartcontractkit/chainlink-solana/pkg/solana/txm/utils"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/utils"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/ccip_offramp"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/ccip"
	solcommon "github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	localfees "github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/fees"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
)

const (
	// maxNumChunks is the maximum number of buffer chunks supported on-chain (u64 bitmap).
	// See contracts/programs/ccip-offramp/src/buffering.rs.
	maxNumChunks = 64
)

// submitBuffered runs the buffer --> resolve --> exec sequence synchronously on the caller's
// goroutine and returns an error if any step fails. The error propagates to the CCV executor,
// which retries the message via its heap-based retry loop (unlike the inline path, see
// SolanaContractTransmitter). Failure paths that have already touched the chain close the
// buffer PDA best-effort to reclaim rent before returning.
//
// The flow is scoped to the caller's context with an upper bound of BufferedFlowTimeout, so
// executor/coordinator shutdown cancels in-flight RPCs and waitForFinalized loops.
func (t *SolanaContractTransmitter) submitBuffered(ctx context.Context, execInputs ccip_offramp.ExecutionInputsV2, report protocol.AbstractAggregatedReport) error {
	// Flow steps (writes, resolve, exec) run under BufferedFlowTimeout
	flowCtx, cancel := context.WithTimeout(ctx, t.cfg.BufferedFlowTimeout.Duration())
	defer cancel()

	b, rawInputs, err := t.newBufferedExecution(execInputs, report)
	if err != nil {
		return fmt.Errorf("buffered flow setup: %w", err)
	}

	b.lggr().Debugw("buffered flow: starting",
		"rawInputsLen", len(rawInputs), "bufferPDA", b.bufferPDA, "authority", b.authority)

	// Measure overhead, split payload
	chunks, err := b.prepareChunks(rawInputs)
	if err != nil {
		// nothing on-chain yet, no cleanup needed
		return fmt.Errorf("buffered flow: prepare chunks: %w", err)
	}

	// Initial blockhash. Once acquired, every subsequent failure path goes through
	// b.fail() which closes the buffer PDA to reclaim rent. writeChunks gets a new one per chunk.
	if err = b.refreshBlockhash(flowCtx); err != nil {
		// nothing on-chain yet so no need to call fail() to clean up buffer
		return fmt.Errorf("buffered flow: initial blockhash: %w", err)
	}

	// Write to chain
	if err = b.writeChunks(flowCtx, rawInputs, chunks); err != nil {
		b.fail(ctx, "write chunks", err)
		return fmt.Errorf("buffered flow: write chunks: %w", err)
	}

	// Simulate execute_v2 and resolve LUTs
	resolved, lutMap, err := b.resolveAndFetchLUTs(flowCtx)
	if err != nil {
		b.fail(ctx, "resolve exec accounts", err)
		return fmt.Errorf("buffered flow: resolve exec accounts: %w", err)
	}

	// Refresh blockhash for the exec tx: buffer writes may have taken long enough
	// (we waited for Finalized on each chunk) that the original is stale.
	if err := b.refreshBlockhash(flowCtx); err != nil {
		b.fail(ctx, "refresh blockhash for exec", err)
		return fmt.Errorf("buffered flow: refresh blockhash for exec: %w", err)
	}

	if err := b.executeAndWait(flowCtx, resolved, lutMap); err != nil {
		b.fail(ctx, "execute", err)
		return fmt.Errorf("buffered flow: execute: %w", err)
	}

	b.lggr().Infow("submitted tx to chain", "buffered", true)

	return nil
}

// bufferedExecution holds the immutable identifiers for one submitBuffered run
// (PDAs, authority, message/buffer IDs) plus the mutable blockhash/lastValid pair
// that gets refreshed between buffer-writes and execution.
type bufferedExecution struct {
	t                 *SolanaContractTransmitter
	messageID         [32]byte
	bufferID          [32]byte
	authority         solana.PublicKey
	bufferPDA         solana.PublicKey
	configPDA         solana.PublicKey
	eventAuthorityPDA solana.PublicKey
	computeUnits      uint32

	// Mutable (blockhash, lastValid) pair, always set together by refreshBlockhash.
	// Zero-valued until the first refresh, callers must refresh before using.
	blockhash solana.Hash
	lastValid uint64

	// The txs of this run that can still land. cleanup waits for all of them before it returns.
	inFlightTxs []inFlightTx
}

// inFlightTx is an enqueued tx and the last valid block height of its blockhash.
type inFlightTx struct {
	id        string
	lastValid uint64
}

// newBufferedExecution derives all PDAs needed for the buffered flow. Returns the
// raw marshalled execution-inputs payload alongside since it's needed for chunking.
func (t *SolanaContractTransmitter) newBufferedExecution(
	execInputs ccip_offramp.ExecutionInputsV2,
	report protocol.AbstractAggregatedReport,
) (*bufferedExecution, []byte, error) {
	messageID := report.Message.MustMessageID()

	rawInputs, err := execInputs.Marshal()
	if err != nil {
		return nil, nil, fmt.Errorf("marshal execution inputs: %w", err)
	}

	// bufferID == messageID: the buffer PDA is deterministic from (messageID, transmitter).
	bufferID := messageID
	authority := t.feePayerPubKey
	bufferPDA, _, err := state.FindExecutionInputsBufferPDA(bufferID, authority, t.offrampProgramID)
	if err != nil {
		return nil, nil, fmt.Errorf("derive buffer PDA: %w", err)
	}

	// Required by the onchain BufferExecutionInputs ix to verify the offramp
	// config version is supported (see CcipOfframpError::InvalidVersion).
	configPDA, _, err := state.FindOfframpConfigPDA(t.offrampProgramID)
	if err != nil {
		return nil, nil, fmt.Errorf("derive config PDA: %w", err)
	}

	// Needed for emitting CPI events.
	eventAuthorityPDA, _, err := state.FindEventAuthorityPDA(t.offrampProgramID)
	if err != nil {
		return nil, nil, fmt.Errorf("derive event authority PDA: %w", err)
	}

	b := &bufferedExecution{
		t:                 t,
		messageID:         messageID,
		bufferID:          bufferID,
		authority:         authority,
		bufferPDA:         bufferPDA,
		configPDA:         configPDA,
		eventAuthorityPDA: eventAuthorityPDA,
		computeUnits:      computeUnitLimit(report.Message),
	}
	return b, rawInputs, nil
}

// lggr returns a logger with messageID pre-bound so individual call sites stay terse.
func (b *bufferedExecution) lggr() logger.Logger {
	return logger.With(b.t.lggr, "messageID", protocol.Bytes32(b.messageID).String())
}

// refreshBlockhash fetches a finalized blockhash and updates (blockhash, lastValid)
// atomically. Always use this instead of assigning the fields directly to keep the pair
// in sync.
func (b *bufferedExecution) refreshBlockhash(ctx context.Context) error {
	recent, err := b.t.rpcClient.GetLatestBlockhash(ctx)
	if err != nil {
		return fmt.Errorf("get latest blockhash: %w", err)
	}
	b.blockhash = recent.Value.Blockhash
	b.lastValid = recent.Value.LastValidBlockHeight
	return nil
}

// fail logs the failure and closes the buffer PDA (see cleanup).
// Use only after the first successful refreshBlockhash, otherwise the cleanup tx
// has no blockhash and will silently fail to enqueue.
func (b *bufferedExecution) fail(ctx context.Context, reason string, err error) {
	b.lggr().Errorw("buffered flow: "+reason, "error", err)
	b.cleanup(ctx, reason)
}

// enqueue sends tx through the txm and records it for cleanup. Fee bumping is off: each bump
// adds a signature, and the txm marks the logical tx finalized when one signature lands while
// the other signatures can still land.
func (b *bufferedExecution) enqueue(ctx context.Context, tx *solana.Transaction, txID string, opts ...txmutils.SetTxConfig) error {
	opts = append(opts, txmutils.SetFeeBumpPeriod(0))
	if err := b.t.txm.Enqueue(ctx, b.authority.String(), tx, &txID, b.lastValid, opts...); err != nil {
		return err
	}
	b.inFlightTxs = append(b.inFlightTxs, inFlightTx{id: txID, lastValid: b.lastValid})
	return nil
}

// cleanup closes the buffer PDA and waits until no tx of this run can still land, because a retry
// of the same message reuses the PDA. If ctx ends first, a late tx can make the next attempt fail.
func (b *bufferedExecution) cleanup(ctx context.Context, reason string) {
	var cancel context.CancelFunc
	if ctx.Err() != nil {
		ctx, cancel = context.WithTimeout(context.Background(), b.t.cfg.BufferCleanupTimeout.Duration())
	} else {
		ctx, cancel = context.WithTimeout(ctx, b.t.cfg.BufferedFlowTimeout.Duration())
	}
	defer cancel()
	b.waitUntilNoTxCanLand(ctx, reason)
	b.closeBuffer(ctx, reason)
	b.waitUntilNoTxCanLand(ctx, reason)
}

// waitUntilNoTxCanLand keeps in b.inFlight the txs that ctx ended before. It polls all txs together,
// so the landing windows of dropped txs overlap.
func (b *bufferedExecution) waitUntilNoTxCanLand(ctx context.Context, reason string) {
	ticker := time.NewTicker(b.t.cfg.TxStatusPollInterval.Duration())
	defer ticker.Stop()
	for len(b.inFlightTxs) > 0 {
		select {
		case <-ctx.Done():
			b.lggr().Warnw("buffered flow: txs are still in flight",
				"reason", reason, "txs", fmt.Sprintf("%+v", b.inFlightTxs), "error", ctx.Err())
			return
		case <-ticker.C:
		}
		b.inFlightTxs = b.t.stillInFlight(ctx, b.inFlightTxs)
	}
}

func (b *bufferedExecution) closeBuffer(ctx context.Context, reason string) {
	// The wait for in-flight txs can outlast the flow's blockhash. On failure, keep the last known pair.
	if err := b.refreshBlockhash(ctx); err != nil {
		b.lggr().Debugw("buffered flow: refresh blockhash before close failed, using stale value",
			"reason", reason, "error", err)
	}
	closeIx, err := buildCloseBufferIx(b.bufferID, b.bufferPDA, b.configPDA, b.authority, b.eventAuthorityPDA, b.t.offrampProgramID)
	if err != nil {
		b.lggr().Warnw("buffered flow: build close buffer ix", "reason", reason, "error", err)
		return
	}
	closeTx, err := solana.NewTransaction([]solana.Instruction{closeIx}, b.blockhash, solana.TransactionPayer(b.authority))
	if err != nil {
		b.lggr().Warnw("buffered flow: build close buffer tx", "reason", reason, "error", err)
		return
	}
	closeID := fmt.Sprintf("CloseBuffer-%s-%s", hex.EncodeToString(b.bufferID[:]), uuid.NewString())
	if err := b.enqueue(ctx, closeTx, closeID, txmutils.SetEstimateComputeUnitLimit(true)); err != nil {
		b.lggr().Warnw("buffered flow: enqueue close buffer tx", "reason", reason, "closeTxID", closeID, "error", err)
		return
	}
	b.lggr().Debugw("buffered flow: enqueued close buffer tx", "reason", reason, "closeTxID", closeID, "bufferPDA", b.bufferPDA)
}

// prepareChunks measures buffer-tx overhead, splits the payload, and validates that
// the chunk count fits in the on-chain bitmap.
func (b *bufferedExecution) prepareChunks(rawInputs []byte) ([][]byte, error) {
	baseBufferTxSize, err := getBaseBufferTxSize(b.t.offrampProgramID, b.bufferID, b.bufferPDA, b.configPDA, b.authority, b.eventAuthorityPDA)
	if err != nil {
		return nil, fmt.Errorf("empty buffer tx size: %w", err)
	}

	chunks, err := splitAndBuildReportChunks(rawInputs, baseBufferTxSize)
	if err != nil {
		return nil, fmt.Errorf("split chunks: %w", err)
	}
	if len(chunks) > maxNumChunks {
		return nil, fmt.Errorf("too many chunks: %d > %d: %w", len(chunks), maxNumChunks, executor.ErrMessageRejectedByTransmitter)
	}

	b.lggr().Debugw("buffered flow: chunked payload",
		"numChunks", len(chunks), "totalLen", len(rawInputs), "baseBufferTxSize", baseBufferTxSize)
	return chunks, nil
}

// writeChunks builds and enqueues one buffer-write tx per chunk, then waits for each to
// finalize. On failure the caller's cleanup waits for the writes that are still in flight.
// Buffer writes go out in parallel so total latency is bounded by the slowest chunk to
// finalize. Each chunk gets a new blockhash, so a slow enqueue of earlier chunks cannot
// expire later chunks.
func (b *bufferedExecution) writeChunks(ctx context.Context, rawInputs []byte, chunks [][]byte) error {
	totalLen := uint32(len(rawInputs)) //nolint:gosec // execution inputs length fits in uint32 for practical payloads
	numChunks := uint8(len(chunks))    //nolint:gosec // validated against maxNumChunks in prepareChunks

	for i, chunk := range chunks {
		if err := b.refreshBlockhash(ctx); err != nil {
			return fmt.Errorf("refresh blockhash for chunk %d: %w", i, err)
		}

		ix, err := buildBufferIx(b.bufferID, totalLen, chunk, uint8(i), numChunks,
			b.bufferPDA, b.configPDA, b.authority, b.eventAuthorityPDA, b.t.offrampProgramID)
		if err != nil {
			return fmt.Errorf("build buffer instruction chunk %d: %w", i, err)
		}

		tx, err := solana.NewTransaction([]solana.Instruction{ix}, b.blockhash, solana.TransactionPayer(b.authority))
		if err != nil {
			return fmt.Errorf("build buffer transaction chunk %d: %w", i, err)
		}

		if err := b.enqueue(ctx, tx, uuid.NewString(), txmutils.SetEstimateComputeUnitLimit(true)); err != nil {
			return fmt.Errorf("enqueue buffer transaction chunk %d: %w", i, err)
		}
	}

	b.lggr().Infow("buffered flow: enqueued and waiting for buffer txs to finalize", "txCount", len(b.inFlightTxs), "bufferPDA", b.bufferPDA)

	// b.inFlight holds only the chunk writes here, because writeChunks is the first step that enqueues.
	for _, tx := range b.inFlightTxs {
		if err := b.t.waitForFinalized(ctx, tx.id); err != nil {
			return fmt.Errorf("wait for finalized buffer tx %s: %w", tx.id, err)
		}
	}
	return nil
}

// resolveAndFetchLUTs simulates the on-chain execute_v2 to discover account metas
// and lookup tables, then fetches each LUT's contents for the eventual exec tx.
func (b *bufferedExecution) resolveAndFetchLUTs(ctx context.Context) (
	ccip.ResolveAccountsResult,
	map[solana.PublicKey]solana.PublicKeySlice,
	error,
) {
	resolved, err := ccip.ResolveExecAccountsWithBuffer(ctx, b.t.rpcClient, b.t.offrampProgramID, b.t.feePayerPubKey, b.bufferPDA, b.t.ccipLog)
	if err != nil {
		return ccip.ResolveAccountsResult{}, nil, fmt.Errorf("resolve exec accounts: %w", err)
	}
	b.lggr().Debugw("buffered flow: resolved execute accounts", "numLookupTables", len(resolved.LookupTables))

	lutMap, err := solcommon.GetAddressLookupTableMap(ctx, b.t.rpcClient, resolved.LookupTables)
	if err != nil {
		return ccip.ResolveAccountsResult{}, nil, fmt.Errorf("fetch LUTs: %w", err)
	}
	return resolved, lutMap, nil
}

// executeAndWait builds the exec_v2 tx (with extended heap), enqueues it, and waits
// for it to reach Finalized. Returns an error if the tx still exceeds the Solana
// size limit after buffering. At that point buffering is not enough and we cannot
// land the message. The error wraps executor.ErrMessageRejectedByTransmitter, so the
// executor does not retry.
func (b *bufferedExecution) executeAndWait(
	ctx context.Context,
	resolved ccip.ResolveAccountsResult,
	lutMap map[solana.PublicKey]solana.PublicKeySlice,
) error {
	tx, err := solana.NewTransaction(
		[]solana.Instruction{resolved.Ix},
		b.blockhash,
		solana.TransactionPayer(b.authority),
		solana.TransactionAddressTables(lutMap),
	)
	if err != nil {
		return fmt.Errorf("build exec tx: %w", err)
	}

	// Request extended heap (256 KB) for the on-chain program to deserialize the large buffer.
	// The OffRamp's custom allocator uses the whole frame; programs it CPIs into that keep the
	// SDK's default allocator are still limited to its fixed 32 KB.
	if err = localfees.SetHeapFrame(tx, 256*1024); err != nil {
		return fmt.Errorf("set heap frame: %w", err)
	}

	// The heap-frame ix added above is already part of tx, so calculateTxSizeInBytes counts it
	// when it marshals the tx. (It separately adds the CU price/limit ixs itself, since those are
	// applied by txm at enqueue time rather than being on the tx here.)
	size, err := calculateTxSizeInBytes(tx)
	if err != nil {
		return fmt.Errorf("calculate exec tx size: %w", err)
	}
	b.lggr().Debugw("buffered flow: exec tx built", "execTxSize", size, "limit", solcommon.MaxSolanaTxSize)
	if size > solcommon.MaxSolanaTxSize {
		return fmt.Errorf("exec tx still oversized after buffering: %d > %d: %w", size, solcommon.MaxSolanaTxSize, executor.ErrMessageRejectedByTransmitter)
	}

	txID := uuid.NewString()
	if err := b.enqueue(ctx, tx, txID, txmutils.SetComputeUnitLimit(b.computeUnits)); err != nil {
		return fmt.Errorf("enqueue exec tx: %w", err)
	}
	b.lggr().Infow("buffered flow: enqueued exec tx", "txID", txID)
	b.lggr().Debugw("buffered flow: waiting for exec tx to finalize", "txID", txID)

	if err := b.t.waitForFinalized(ctx, txID); err != nil {
		return fmt.Errorf("exec tx %s: %w", txID, err)
	}
	return nil
}

// calculateTxSizeInBytes returns the serialized size of a transaction after txm adds compute-budget
// instructions and a signature placeholder.
func calculateTxSizeInBytes(tx *solana.Transaction) (int, error) {
	if tx == nil {
		return 0, errors.New("tx is nil")
	}
	copyTx := utils.DeepCopyTx(*tx)

	if err := fees.SetComputeUnitPrice(&copyTx, 0); err != nil {
		return 0, fmt.Errorf("set compute unit price: %w", err)
	}
	if err := fees.SetComputeUnitLimit(&copyTx, 0); err != nil {
		return 0, fmt.Errorf("set compute unit limit: %w", err)
	}
	copyTx.Signatures = append(copyTx.Signatures, solana.Signature{})

	txBytes, err := copyTx.MarshalBinary()
	if err != nil {
		return 0, fmt.Errorf("marshal transaction: %w", err)
	}
	return len(txBytes), nil
}

// splitAndBuildReportChunks splits raw payload bytes into chunks that fit in a single Solana transaction.
// Adapted from chainlink-solana/pkg/solana/chainwriter/buffer_payload.go (V2 uses BufferExecutionInputs overhead).
func splitAndBuildReportChunks(rawReport []byte, emptyBufferTxSize int) ([][]byte, error) {
	chunkSize := solcommon.MaxSolanaTxSize - emptyBufferTxSize - 1
	if chunkSize <= 0 {
		return nil, fmt.Errorf("empty buffer tx size %d leaves no room for chunk data", emptyBufferTxSize)
	}

	// Extra chunk for any remainder
	chunkCount := len(rawReport) / chunkSize
	if len(rawReport)%chunkSize != 0 {
		chunkCount++
	}

	chunks := make([][]byte, 0, chunkCount)
	for i := range chunkCount {
		start := i * chunkSize
		end := min((i+1)*chunkSize, len(rawReport))
		chunks = append(chunks, rawReport[start:end])
	}
	return chunks, nil
}

func buildBufferIx(
	bufferID [32]byte,
	totalLen uint32,
	chunk []byte,
	chunkIndex uint8,
	numChunks uint8,
	bufferPDA, configPDA, authority, eventAuthority, programID solana.PublicKey,
) (solana.Instruction, error) {
	ix, err := ccip_offramp.NewBufferExecutionInputsInstruction(
		ccip_offramp.BufferExecutionInputsParams{
			BufferId:    bufferID,
			TotalLength: totalLen,
			Chunk:       chunk,
			ChunkIndex:  chunkIndex,
			NumChunks:   numChunks,
		},
		bufferPDA,
		configPDA,
		authority,
		solana.SystemProgramID,
		eventAuthority,
		programID,
	)
	if err != nil {
		return nil, err
	}
	return solcommon.InstructionWithProgramID(ix, programID)
}

func buildCloseBufferIx(
	bufferID [32]byte,
	bufferPDA, configPDA, authority, eventAuthority, programID solana.PublicKey,
) (solana.Instruction, error) {
	ix, err := ccip_offramp.NewCloseExecutionInputsBufferInstruction(
		ccip_offramp.CloseExecutionInputsBufferParams{BufferId: bufferID},
		bufferPDA,
		configPDA,
		authority,
		solana.SystemProgramID,
		eventAuthority,
		programID,
	)
	if err != nil {
		return nil, err
	}
	return solcommon.InstructionWithProgramID(ix, programID)
}

// getBaseBufferTxSize builds a buffer transaction with zero-length chunk data and measures its
// serialized size. This is effectively overhead and is subtracted from solcommon.MaxSolanaTxSize to determine the
// maximum chunk payload that can fit in each buffer transaction (see splitAndBuildReportChunks).
func getBaseBufferTxSize(
	offrampProgramID solana.PublicKey,
	bufferID [32]byte,
	bufferPDA, configPDA, authority, eventAuthority solana.PublicKey,
) (int, error) {
	emptyIx, err := buildBufferIx(bufferID, 0, []byte{}, 0, 1, bufferPDA, configPDA, authority, eventAuthority, offrampProgramID)
	if err != nil {
		return 0, err
	}
	emptyTx, err := solana.NewTransaction(
		[]solana.Instruction{emptyIx},
		solana.Hash{},
		solana.TransactionPayer(authority),
	)
	if err != nil {
		return 0, fmt.Errorf("build empty buffer tx: %w", err)
	}
	return calculateTxSizeInBytes(emptyTx)
}

// waitForFinalized polls txm until the transaction reaches Finalized or a terminal failure.
//
// We wait for Finalized rather than Unconfirmed to match the 1.6 flow. The trade-off is higher latency, but
// it guarantees the buffer is observable from the resolver simulation.
func (t *SolanaContractTransmitter) waitForFinalized(ctx context.Context, txID string) error {
	ticker := time.NewTicker(t.cfg.TxStatusPollInterval.Duration())
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			status, err := t.txm.GetTransactionStatus(ctx, txID)
			if err != nil {
				return fmt.Errorf("get transaction status for %s: %w", txID, err)
			}
			switch status {
			case commontypes.Finalized:
				return nil
			case commontypes.Failed, commontypes.Fatal:
				return fmt.Errorf("transaction %s failed with status %v", txID, status)
			case commontypes.Unknown:
				return fmt.Errorf("transaction %s has unknown status", txID)
			case commontypes.Pending, commontypes.Unconfirmed:
				// keep polling
			default:
				return fmt.Errorf("transaction %s has unexpected status %v", txID, status)
			}
		}
	}
}

// stillInFlight returns the txs that can still land. A finalized tx can no longer land, because
// Solana finalization is final. A tx that the txm still reports as pending or unconfirmed can land
// even after the last valid height recorded at enqueue, because a reorg rebroadcasts it with a new
// blockhash and a later last valid height. A failed status is not enough, because the txm can mark
// a tx failed while it can still land.
func (t *SolanaContractTransmitter) stillInFlight(ctx context.Context, txs []inFlightTx) []inFlightTx {
	height, err := t.rpcClient.GetLatestBlockHeight(ctx)
	if err != nil {
		height = 0 // only finalized txs leave inFlight on this poll
	}
	return slices.DeleteFunc(txs, func(tx inFlightTx) bool {
		status, err := t.txm.GetTransactionStatus(ctx, tx.id)
		if err == nil {
			switch status {
			case commontypes.Finalized:
				return true
			case commontypes.Pending, commontypes.Unconfirmed:
				return false
			case commontypes.Failed, commontypes.Fatal, commontypes.Unknown:
			}
		}
		return height > tx.lastValid
	})
}
