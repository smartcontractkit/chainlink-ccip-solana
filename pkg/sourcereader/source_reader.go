// Package sourcereader implements the chainaccess.SourceReader interface for Solana.
// It reads CCIP message events from the Solana blockchain by:
//  1. Using a logpoller.Service to discover and extract events from slots
//  2. Parsing raw event data into protocol.MessageSentEvent structs
//
// The source reader has two chain dependencies:
//   - logpoller.Service for event discovery (slot polling + emit_cpi! extraction)
//   - RPCClient for slot queries and block header fetching (LatestAndFinalizedBlock, GetBlocksHeaders)
//
// The accessor factory supplies a logpoller.Service and starts it at GetAccessor;
// accessor.Close stops it after CCV bootstrap runs factory.Stop and CloseAll.
// SolanaSourceReader only reads via the injected logpoller (FetchEvents); it does
// not implement services.Service. Destination-side logpoller lifecycle is owned by
// DestinationReader and the CCV executor coordinator.
package sourcereader

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	"github.com/smartcontractkit/chainlink-ccv/pkg/chainaccess"
	"github.com/smartcontractkit/chainlink-ccv/protocol"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	solclient "github.com/smartcontractkit/chainlink-solana/pkg/solana/client"

	"github.com/smartcontractkit/chainlink-ccip-solana/pkg/commonreader"
	"github.com/smartcontractkit/chainlink-ccip-solana/pkg/logpoller"
	"github.com/smartcontractkit/chainlink-ccip-solana/pkg/solanarpc"
)

// Compile-time checks for SolanaSourceReader.
var (
	_ chainaccess.SourceReader = (*SolanaSourceReader)(nil)
)

// RPCClient is the Solana JSON-RPC subset this package reads through. It covers the
// block and slot queries made here plus the account read that commonreader performs.
type RPCClient interface {
	solanarpc.BlockReader
	solanarpc.AccountReader
}

// SolanaSourceReader reads CCIP events and block headers from Solana.
// It implements chainaccess.SourceReader for use by the CCV pipeline.
type SolanaSourceReader struct {
	onRampAddress    solana.PublicKey
	rmnRemoteAddress solana.PublicKey
	chainSelector    protocol.ChainSelector
	lggr             logger.SugaredLogger
	rpcClient        RPCClient
	logPoller        logpoller.Service
	eventParser      MessageSentEventParser
}

// ReaderConfig is the configuration required to create a source reader.
// logpoller.Config is embedded anonymously so its fields flatten into the
// [chains.<selector>.reader] TOML table (e.g. slots_batch_size).
type ReaderConfig struct {
	logpoller.Config
}

func NewSolanaSourceReader(
	onRampAddress solana.PublicKey,
	rmnRemoteAddress solana.PublicKey,
	chainSelector protocol.ChainSelector,
	rpcClient RPCClient,
	logPoller logpoller.Service,
	lggr logger.Logger,
	eventParser MessageSentEventParser,
) (chainaccess.SourceReader, error) {
	var errs []error

	if onRampAddress.IsZero() {
		errs = append(errs, errors.New("onRampAddress is not set"))
	}
	if rmnRemoteAddress.IsZero() {
		errs = append(errs, errors.New("rmnRemoteAddress is not set"))
	}
	if chainSelector == 0 {
		errs = append(errs, errors.New("chainSelector is not set"))
	}
	if rpcClient == nil {
		errs = append(errs, errors.New("rpcClient is not set"))
	}
	if logPoller == nil {
		errs = append(errs, errors.New("logPoller is not set"))
	}
	if eventParser == nil {
		errs = append(errs, errors.New("eventParser is not set"))
	}

	if len(errs) != 0 {
		return nil, errors.Join(errs...)
	}

	reader := &SolanaSourceReader{
		onRampAddress:    onRampAddress,
		rmnRemoteAddress: rmnRemoteAddress,
		chainSelector:    chainSelector,
		lggr:             logger.Sugared(lggr).Named("SourceReader"),
		rpcClient:        rpcClient,
		logPoller:        logPoller,
		eventParser:      eventParser,
	}

	return reader, nil
}

// GetBlocksHeaders returns block headers for the requested chainaccess block
// numbers. For Solana, those block numbers are interpreted as slots.
// It omits a slot when getBlock returns a null result (nil block or
// rpc.ErrNotConfirmed). It returns every other RPC error, including error
// -32007, which a node returns for a skipped slot or for a slot that is not in
// its ledger.
func (r *SolanaSourceReader) GetBlocksHeaders(ctx context.Context, blockNumbers []uint64) (map[uint64]protocol.BlockHeader, error) {
	if r.rpcClient == nil {
		return nil, errors.New("rpc client not configured")
	}

	headers := make(map[uint64]protocol.BlockHeader, len(blockNumbers))

	for _, slot := range blockNumbers {
		block, err := r.rpcClient.GetBlockWithOpts(ctx, slot, solclient.HeadMetadataGetBlockOpts(rpc.CommitmentFinalized))
		if err != nil {
			if errors.Is(err, rpc.ErrNotConfirmed) {
				continue
			}
			return nil, fmt.Errorf("get block header for slot %d: %w", slot, err)
		}
		if block == nil {
			continue
		}

		var hash, parentHash protocol.Bytes32
		copy(hash[:], block.Blockhash[:])
		if block.PreviousBlockhash != (solana.Hash{}) {
			copy(parentHash[:], block.PreviousBlockhash[:])
		}

		var ts time.Time
		if block.BlockTime != nil {
			ts = block.BlockTime.Time()
		}

		headers[slot] = protocol.BlockHeader{
			Number:     slot,
			Hash:       hash,
			ParentHash: parentHash,
			Timestamp:  ts,
		}
	}

	return headers, nil
}

// FetchMessageSentEvents returns CCIPMessageSentV2 events in the requested
// chainaccess block-number range. For Solana, those block numbers are slots.
// It delegates slot discovery and event extraction to the logpoller, then
// parses each raw event into protocol.MessageSentEvent.
//
// The logpoller filter applies to the router program, so the raw events include every
// emit_cpi! event from that program. This function skips the events of other types and
// returns an error if it cannot decode a message event.
//
// Use the passed ctx directly. The upstream CCV coordinator bounds each poll cycle with
// pollTimeout and maxBlockRange, so no inner deadline is necessary.
func (r *SolanaSourceReader) FetchMessageSentEvents(
	ctx context.Context,
	fromBlock, toBlock uint64,
) ([]protocol.MessageSentEvent, error) {
	fromSlot, toSlot, err := r.resolveFetchSlotRange(ctx, fromBlock, toBlock)
	if err != nil {
		return nil, err
	}

	r.lggr.Infow("FetchMessageSentEvents",
		"onRampAddress", r.onRampAddress.String(),
		"fromSlot", fromSlot,
		"toSlot", toSlot,
	)

	rawEvents, err := r.logPoller.FetchEvents(ctx, fromSlot, toSlot)
	if err != nil {
		return nil, fmt.Errorf("fetch events: %w", err)
	}

	r.lggr.Infow("FetchMessageSentEvents result",
		"rawEventCount", len(rawEvents),
		"fromSlot", fromSlot,
		"toSlot", toSlot,
	)

	results := make([]protocol.MessageSentEvent, 0, len(rawEvents))
	for _, raw := range rawEvents {
		evt, isMessageSentEvent, parseErr := r.eventParser(raw.Data)
		if !isMessageSentEvent {
			// Sibling event from the same program-scoped filter: safe to skip.
			r.lggr.Debugw("Skipping non-message router CPI event",
				"slot", raw.SlotNumber,
				"txSignature", raw.TxSignature.String(),
				"dataLen", len(raw.Data),
			)
			continue
		}
		if parseErr != nil {
			// Critical: the deployed program and the pinned bindings disagree.
			r.lggr.Criticalw("Undecodable CcipMessageSentV2: holding slot cursor to avoid message loss",
				"error", parseErr,
				"slot", raw.SlotNumber,
				"txSignature", raw.TxSignature.String(),
				"dataLen", len(raw.Data),
			)
			return nil, fmt.Errorf("decode CcipMessageSentV2 at slot %d tx %s: %w",
				raw.SlotNumber, raw.TxSignature.String(), parseErr)
		}
		evt.BlockNumber = raw.SlotNumber
		evt.TxHash = protocol.ByteSlice(raw.TxSignature[:])
		results = append(results, evt)
	}

	return results, nil
}

// resolveFetchSlotRange converts chainaccess fromBlock/toBlock arguments into
// an inclusive Solana slot range. The argument names say "block" because they
// come from the chainaccess interface; in this Solana implementation their
// numeric values are slots. A toBlock of 0 means "through the latest finalized
// slot" because chainlink-solana's logpoller discovers signatures with
// CommitmentFinalized.
func (r *SolanaSourceReader) resolveFetchSlotRange(
	ctx context.Context,
	fromBlock, toBlock uint64,
) (uint64, uint64, error) {
	fromSlot := fromBlock
	toSlot := toBlock
	if toSlot == 0 {
		finalizedSlot, slotErr := r.rpcClient.GetSlot(ctx, rpc.CommitmentFinalized)
		if slotErr != nil {
			return 0, 0, fmt.Errorf("resolve zero toBlock to finalized slot: %w", slotErr)
		}
		toSlot = finalizedSlot
	}

	if toSlot < fromSlot {
		return 0, 0, fmt.Errorf("invalid slot range: toSlot (%d) < fromSlot (%d)", toSlot, fromSlot)
	}
	return fromSlot, toSlot, nil
}

// LatestAndFinalizedBlock queries the Solana RPC for the latest (processed)
// and finalized slot numbers. Solana has no parent hash concept, so hashes
// are left as zero values.
func (r *SolanaSourceReader) LatestAndFinalizedBlock(
	ctx context.Context,
) (latest, finalized *protocol.BlockHeader, err error) {
	if r.rpcClient == nil {
		return nil, nil, errors.New("rpc client not configured")
	}

	// TODO(logpoller): chainlink-solana's logpoller hardcodes CommitmentFinalized in
	// getSlotsForAddressJob, so we must use finalized-compatible slots here.
	// CCIP 2.0 expects faster-than-finality message passing — the logpoller needs
	// configurable commitment levels (CommitmentConfirmed or CommitmentProcessed)
	// to reduce latency from the ~32 slot finality window.
	latestSlot, err := r.rpcClient.GetSlot(ctx, rpc.CommitmentConfirmed)
	if err != nil {
		return nil, nil, fmt.Errorf("get latest slot: %w", err)
	}

	finalizedSlot, err := r.rpcClient.GetSlot(ctx, rpc.CommitmentFinalized)
	if err != nil {
		return nil, nil, fmt.Errorf("get finalized slot: %w", err)
	}

	now := time.Now()

	latestHeader := &protocol.BlockHeader{
		Number:    latestSlot,
		Timestamp: now,
	}

	finalizedHeader := &protocol.BlockHeader{
		Number:    finalizedSlot,
		Timestamp: now,
	}

	return latestHeader, finalizedHeader, nil
}

// TODO: is there a "safe block" equivalent on Solana? Maybe GetLatestBlockhash(ctx, rpc.CommitmentFinalized)?
func (r *SolanaSourceReader) LatestSafeBlock(ctx context.Context) (*protocol.BlockHeader, error) {
	_ = ctx
	return nil, nil
}

// GetRMNCursedSubjects queries this source chain's RMN Remote contract for cursed subjects.
func (r *SolanaSourceReader) GetRMNCursedSubjects(
	ctx context.Context,
) ([]protocol.Bytes16, error) {
	return commonreader.ReadRMNCursedSubjects(ctx, r.rpcClient, r.rmnRemoteAddress)
}
