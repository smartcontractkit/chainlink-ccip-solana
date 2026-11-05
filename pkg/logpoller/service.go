package logpoller

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/services"
	sollogpoller "github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller"
	lptypes "github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller/types"

	"github.com/smartcontractkit/chainlink-ccip-solana/pkg/solanarpc"
)

// Service provides event fetching with managed lifecycle.
// The factory creates and starts it; consumers call FetchEvents.
type Service interface {
	services.Service
	FetchEvents(ctx context.Context, fromSlot, toSlot uint64) ([]Event, error)
}

// Compile-time check: service satisfies Service.
var _ Service = (*service)(nil)

// Event is a CPI event extracted from a transaction's inner instructions.
type Event struct {
	// Program is the program that emitted the event (the self-CPI target for emit_cpi!).
	Program solana.PublicKey
	// Data is the raw Anchor event payload. Format: [event_discriminator (8)] [borsh_payload].
	Data []byte
	// SlotNumber is the slot containing this event.
	SlotNumber uint64
	// TxSignature is the transaction signature.
	TxSignature solana.Signature
}

// LogCollector is the narrow dependency Service uses to backfill blocks and run lifecycle
// as services.Service. In production this is *sollogpoller.EncodedLogCollector from NewService;
// tests may inject a fake via NewServiceWithLogCollector.
type LogCollector interface {
	services.Service
	BackfillForAddresses(ctx context.Context, addresses []lptypes.PublicKey, fromSlot, toSlot uint64) (<-chan lptypes.Block, func(), error)
}

var _ LogCollector = (*sollogpoller.EncodedLogCollector)(nil)

type service struct {
	LogCollector    // Start, Close, Ready, HealthReport, Name promoted from LogCollector
	programID       solana.PublicKey
	addr            lptypes.PublicKey
	lggr            logger.SugaredLogger
	includeReverted bool
}

// RPCClient is the Solana JSON-RPC subset the encoded log collector reads through,
// spelled the way *rpc.Client spells it.
type RPCClient interface {
	solanarpc.LogPollerReader
}

// logCollectorClient renames GetSlot to the SlotHeightWithCommitment spelling that
// sollogpoller.RPCClient requires.
type logCollectorClient struct {
	RPCClient
}

func (c logCollectorClient) SlotHeightWithCommitment(ctx context.Context, commitment rpc.CommitmentType) (uint64, error) {
	return c.GetSlot(ctx, commitment)
}

var _ sollogpoller.RPCClient = logCollectorClient{}

// NewService creates a logpoller Service wired to the given RPC client and program.
// It registers a CPI filter for Anchor emit_cpi! events where programID invokes
// itself (source == dest == programID, method discriminator == anchor:event).
func NewService(
	rpcClient RPCClient,
	programID solana.PublicKey,
	chainID string,
	lggr logger.Logger,
	cfg Config,
) (Service, error) {
	metrics, err := sollogpoller.NewSolLpMetrics(chainID)
	if err != nil {
		return nil, fmt.Errorf("failed to init logpoller metrics: %w", err)
	}
	dependencyLogger := newChainSolanaLogPollerLogger(lggr)

	collector := sollogpoller.NewEncodedLogCollector(
		logCollectorClient{rpcClient},
		dependencyLogger,
		chainID,
		metrics,
		newAnchorCPIEventExtractor(programID, dependencyLogger),
		cfg.SlotsBatchSize,
	)

	return NewServiceWithLogCollector(collector, programID, lggr, WithIncludeReverted(cfg.IncludeReverted))
}

// ServiceOption configures optional Service behavior.
type ServiceOption func(*service)

// WithIncludeReverted makes FetchEvents return events from reverted transactions.
func WithIncludeReverted(v bool) ServiceOption {
	return func(s *service) { s.includeReverted = v }
}

// NewServiceWithLogCollector returns a Service that uses the given log collector
// (typically *sollogpoller.EncodedLogCollector from NewService). Use this constructor
// in tests to inject a fake LogCollector without touching RPC.
func NewServiceWithLogCollector(collector LogCollector, programID solana.PublicKey, lggr logger.Logger, opts ...ServiceOption) (Service, error) {
	if collector == nil {
		return nil, errors.New("log collector is required")
	}
	if lggr == nil {
		return nil, errors.New("logger is required")
	}
	var zero solana.PublicKey
	if programID.Equals(zero) {
		return nil, errors.New("program ID is required")
	}
	s := &service{
		LogCollector: collector,
		programID:    programID,
		addr:         lptypes.PublicKey(programID),
		lggr:         logger.Sugared(lggr).Named("LogPoller"),
	}
	for _, o := range opts {
		o(s)
	}
	return s, nil
}

// FetchEvents discovers blocks in [fromSlot, toSlot] containing transactions
// for the configured program and returns the Anchor emit_cpi! events parsed by
// chainlink-solana's CPIEventExtractor.
func (s *service) FetchEvents(
	ctx context.Context,
	fromSlot, toSlot uint64,
) ([]Event, error) {
	if toSlot < fromSlot {
		return nil, fmt.Errorf("invalid slot range: toSlot (%d) < fromSlot (%d)", toSlot, fromSlot)
	}
	addrs := []lptypes.PublicKey{s.addr}
	blocks, cleanup, err := s.BackfillForAddresses(ctx, addrs, fromSlot, toSlot)
	if err != nil {
		return nil, fmt.Errorf("backfill failed: %w", err)
	}
	defer cleanup()

	// NOTE: The sorter does not send aborted blocks on this channel. A slot stopped by
	// the worker pool is not visible here. Use the solana_log_poller_blocks_skipped
	// metric to find such slots.
	//
	// The producer does not close this channel after a failure. A range loop on this
	// channel can wait forever. Each caller must supply a context with a timeout.
	var events []Event
collect:
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case block, ok := <-blocks:
			if !ok {
				break collect
			}
			for _, evt := range block.Events {
				// CCIP 2.0 only utilize CPI event. skip otherwise
				if !evt.IsCPI || evt.Program != s.programID.String() {
					continue
				}
				// skip events from reverted transactions unless the consumer explicitly requested them
				if evt.Error != nil && !s.includeReverted {
					continue
				}
				data, decodeErr := base64.StdEncoding.DecodeString(evt.Data)
				if decodeErr != nil {
					// NOTE: This case does not occur in production: the same process
					// encodes the data. Skip the event; do not fail the full fetch.
					s.lggr.Criticalw("FetchEvents: skipping undecodable CPI event",
						"error", decodeErr,
						"slot", evt.SlotNumber,
						"txSignature", evt.TransactionHash.String(),
					)
					continue
				}
				events = append(events, Event{
					Program:     s.programID,
					Data:        data,
					SlotNumber:  evt.SlotNumber,
					TxSignature: evt.TransactionHash,
				})
			}
		}
	}
	if len(events) > 0 {
		s.lggr.Infow("FetchEvents: found events", "count", len(events), "fromSlot", fromSlot, "toSlot", toSlot)
	}
	return events, nil
}
