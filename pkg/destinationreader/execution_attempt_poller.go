package destinationreader

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"math/big"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/hashicorp/golang-lru/v2/expirable"

	"github.com/smartcontractkit/chainlink-ccv/protocol"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/services"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/ccip_offramp"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/committee_verifier"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/fees"
	"github.com/smartcontractkit/chainlink-ccip-solana/pkg/logpoller"
)

const (
	// coverageTargetUnset means the coverage target is not set.
	coverageTargetUnset uint64 = ^uint64(0)
	// pendingSlotUnset means no slot has partial progress.
	pendingSlotUnset uint64 = ^uint64(0)
	// minSolanaSlotTime is the lowest SIMD-0525 target slot time, so the slot range covers at least the lookback window.
	minSolanaSlotTime = 200 * time.Millisecond
	// warnCachedMessageIDs logs when the caches hold unusually many messageIDs.
	// maxCachedMessageIDs is the LRU size of each cache, so spam from permissionless
	// execute_v2 txs cannot grow memory without limit.
	warnCachedMessageIDs = 1000
	maxCachedMessageIDs  = 2000
	// warnAttemptsPerMessage logs when one messageID accumulates unusually many attempts
	// (re-ingestion from a held cursor). maxAttemptsPerMessage hard-caps the slice,
	// newest entries win so memory stays bounded under sustained transient failures.
	// Failed attempts only trigger a simulation, so a small cap is enough for them.
	warnAttemptsPerMessage      = 10
	maxAttemptsPerMessage       = 50
	maxFailedAttemptsPerMessage = 5
)

// errTransientIngest marks per-event ingest failures that are expected to resolve on
// retry: RPC availability (GetTransaction timeout/429/5xx, or a finalized tx not yet
// served by the node) or tx meta missing inner instructions (an RPC node that doesn't
// record them leaves buffered executions undecodable, so the cursor holds rather than
// silently skipping every buffered attempt).
// pollOnce holds lastPolledSlot only for these. Permanent decode failures (ExecuteV2
// not found, undecodable payloads, messageID mismatch, a buffered execution whose tx
// contains no verify CPIs) are logged and skipped so a single undecodable historical
// event cannot wedge the cursor.
var errTransientIngest = errors.New("transient ingest failure")

// ErrBackfillInProgress reports that the startup backfill has not covered the
// initial lookback window yet.
var ErrBackfillInProgress = errors.New("execution attempt poller backfill in progress")

// errNoNewSlots signals that the finalized slot has not advanced past the cursor.
// It is an expected state, not a failure.
var errNoNewSlots = errors.New("no new slots to poll")

// ExecutionAttemptPollerParams configures the ExecutionAttemptPoller.
type ExecutionAttemptPollerParams struct {
	Lggr logger.Logger
	// LogPoller provides ExecutionStateChangedV2 events from the offramp program.
	// Caller owns lifecycle (Start/Close), the poller only reads from it.
	LogPoller logpoller.Service
	// RPC is used to fetch finalized slot and to resolve ExecuteV2 txs per event.
	RPC RPCClient
	// OfframpProgramID is the deployed ccip-offramp program; required (no default).
	// pkg/accessors calls solutil.SetProgramID before constructing destination components.
	OfframpProgramID solana.PublicKey
	// Config tunes polling. Zero values use defaults.
	Config Config
}

// ExecutionAttemptPoller caches execute_v2 attempts keyed by messageID by ingesting
// ExecutionStateChangedV2(InProgress) events from the offramp.
//
// Why this exists: before the executor spends fees submitting an execute_v2, it asks
// "has someone already executed this message with valid verifier data?" That question
// is answered from this cache, DestinationReader.HasHonestAttempt reads the attempts
// and compares their CCVData against the verifier results it was about to submit.
// A hit means skip; a miss means proceed. Under-reporting therefore costs a duplicate
// submission, never a lost message, which is why per-event failures are logged and
// skipped rather than wedging the loop.
//
// End-to-end flow:
//
//	logpoller events  →  ingest        filter to ExecutionStateChangedV2(InProgress)
//	                  →  fetchAttempt GetTransaction(sig) at finalized commitment
//	                  →                find the ExecuteV2 ix, Borsh-decode ExecuteParams
//	                  →                inline: read ExecInputs from the ix data
//	                  →                buffered (ExecInputs nil): reconstruct from the
//	                  →                verify-CPI inner instructions in the tx meta
//	                  →                decode the message, assert its ID matches the event
//	                  →  cacheAttempt  append to attemptCache or failedAttemptCache
//	                  →                [messageID] by tx outcome (TTL = cacheTTL)
//
// InProgress fires before the receiver CPI, so its log survives a CPI revert.
// That makes (InProgress observed, tx.Meta.Err) the full outcome signal,
// Success/Failure events are redundant and we skip them. Successful transactions
// are terminal evidence of an attempt; reverted ones are only simulation triggers
// for HasHonestAttempt.
//
// Report.CCVS carries the verifier program IDs (inline: from ExecuteParams; buffered:
// from the verify CPIs), positionally aligned with Report.CCVData, slot i of CCVS is
// the verifier that produced blob i. That alignment is what lets a consumer attribute
// a blob to a specific CCV rather than only asking whether it appears somewhere in the set.
type ExecutionAttemptPoller struct {
	// services.StateMachine handles Start/Stop-once and startup health semantics;
	// Ready additionally gates on backfill coverage.
	services.StateMachine

	lggr                   logger.SugaredLogger
	logPoller              logpoller.Service
	rpcClient              RPCClient
	offrampProgramID       solana.PublicKey
	pollInterval           time.Duration
	lookbackWindow         time.Duration
	startupBackfillTimeout time.Duration
	pollFetchTimeout       time.Duration
	// maxEventsPerPoll bounds the number of events ingested in one poll.
	maxEventsPerPoll int
	// pendingSlot holds a slot with partial progress. pendingSlotUnset means none.
	pendingSlot uint64
	// pendingSlotProcessed is the number of events already handled in pendingSlot.
	pendingSlotProcessed int

	// attemptCache holds attempts from successful transactions; failedAttemptCache holds
	// reverted ones, which HasHonestAttempt uses only as simulation triggers.
	// expirable.LRU is internally synchronized. Both are keyed by messageID.
	attemptCache       *expirable.LRU[[32]byte, []protocol.ExecutionAttempt]
	failedAttemptCache *expirable.LRU[[32]byte, []protocol.ExecutionAttempt]
	// lastPolledSlot is the highest slot that has been fully ingested.
	// pollOnce reads from lastPolledSlot+1 and advances it after a successful
	// range fetch. It never moves backward. It is read+written by pollOnce (and by
	// Start before the ticker spawns). Atomic because the `go p.runLoop` hand-off
	// establishes happens-before, but a later refactor introducing a second poller
	// would otherwise race silently.
	lastPolledSlot atomic.Uint64

	// coverageTarget is the finalized slot that startup must reach before Ready.
	// coverageTargetUnset means the target is not set.
	coverageTarget atomic.Uint64

	// lookbackCovered flips once the cursor reaches coverageTarget, either by the
	// startup backfill or by a later pollOnce that advances lastPolledSlot.
	// Ready() gates on this so a cache miss before coverage is distinguishable from
	// "there really was no attempt."
	lookbackCovered atomic.Bool

	cancelFunc context.CancelFunc
	wg         sync.WaitGroup
}

// NewExecutionAttemptPoller builds a poller. Call Start before GetAttempts.
func NewExecutionAttemptPoller(params ExecutionAttemptPollerParams) (*ExecutionAttemptPoller, error) {
	var errs []error
	if params.Lggr == nil {
		errs = append(errs, errors.New("logger is required"))
	}
	if params.LogPoller == nil {
		errs = append(errs, errors.New("log poller is required"))
	}
	if params.RPC == nil {
		errs = append(errs, errors.New("rpc client is required"))
	}
	var zeroKey solana.PublicKey
	if params.OfframpProgramID.Equals(zeroKey) {
		errs = append(errs, errors.New("offramp program ID is required"))
	}
	if len(errs) != 0 {
		return nil, errors.Join(errs...)
	}
	cfg := params.Config.WithDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("execution attempt poller config: %w", err)
	}
	lookback := cfg.LookbackWindow.Duration()
	cacheTTL := max(cfg.AttemptCacheTTL.Duration(), lookback)
	p := &ExecutionAttemptPoller{
		lggr:                   logger.Sugared(params.Lggr).Named("ExecutionAttemptPoller"),
		logPoller:              params.LogPoller,
		rpcClient:              params.RPC,
		offrampProgramID:       params.OfframpProgramID,
		pollInterval:           cfg.PollInterval.Duration(),
		lookbackWindow:         lookback,
		startupBackfillTimeout: cfg.StartupBackfillTimeout.Duration(),
		pollFetchTimeout:       cfg.PollFetchTimeout.Duration(),
		maxEventsPerPoll:       cfg.MaxEventsPerPoll,
		pendingSlot:            pendingSlotUnset,
		// Limited by maxCachedMessageIDs and by cacheTTL.
		attemptCache:       expirable.NewLRU[[32]byte, []protocol.ExecutionAttempt](maxCachedMessageIDs, nil, cacheTTL),
		failedAttemptCache: expirable.NewLRU[[32]byte, []protocol.ExecutionAttempt](maxCachedMessageIDs, nil, cacheTTL),
	}
	p.coverageTarget.Store(coverageTargetUnset)
	return p, nil
}

// Start launches the startup backfill and the ticker loop in the background.
// Start returns immediately. Ready stays false until the initial lookback window
// is covered. Startup RPC failures are logged and retried by the ticker loop.
// The supplied context controls the poller lifecycle. Close cancels it and waits
// for the background goroutine.
func (p *ExecutionAttemptPoller) Start(ctx context.Context) error {
	return p.StartOnce("ExecutionAttemptPoller", func() error {
		runCtx, cancel := context.WithCancel(ctx)
		p.cancelFunc = cancel
		p.wg.Go(func() {
			p.runStartupSequence(runCtx)
			if runCtx.Err() != nil {
				return
			}
			p.runLoop(runCtx)
		})
		return nil
	})
}

// runStartupSequence does the backfill: GetSlot + FetchEvents bounded by
// startupBackfillTimeout, then per-event ingest. Startup RPC failures are
// non-fatal: they are logged and lookbackCovered stays unset so the ticker
// retries the range.
func (p *ExecutionAttemptPoller) runStartupSequence(ctx context.Context) {
	// Narrow the startup deadline to the two fast RPC calls (GetSlot + FetchEvents);
	// the ingestion loop below fetches one tx per event and must not share that budget
	// or later events would silently fail with a context deadline.
	fetchCtx, cancelFetch := context.WithTimeout(ctx, p.startupBackfillTimeout)
	defer cancelFetch()

	latestSlot, err := p.rpcClient.GetSlot(fetchCtx, rpc.CommitmentFinalized)
	if err != nil {
		if ctx.Err() != nil {
			p.lggr.Infow("poller start: startup sequence cancelled", "err", err)
			return
		}
		p.lggr.Warnw("poller start: get latest slot failed, retrying", "err", err)
		return
	}
	p.coverageTarget.Store(latestSlot)

	//nolint:gosec // G115: lookbackWindow is validated > 0 by the constructor; minSolanaSlotTime > 0
	lookbackSlots := uint64(p.lookbackWindow / minSolanaSlotTime)
	var startSlot uint64
	if lookbackSlots < latestSlot {
		startSlot = latestSlot - lookbackSlots
	}
	if startSlot >= latestSlot {
		// Nothing to backfill, so the lookback window is already covered.
		p.advanceLastPolledSlot(latestSlot)
		p.updateCoverage()
		return
	}
	p.advanceLastPolledSlot(slotBefore(startSlot))

	events, bErr := p.logPoller.FetchEvents(fetchCtx, startSlot, latestSlot)
	if bErr != nil {
		p.lggr.Warnw("poller start: backfill failed, ticker will retry",
			"startSlot", startSlot, "latestSlot", latestSlot, "err", bErr)
		return
	}

	// Ingestion uses the lifecycle context without an additional deadline.
	processedThrough, ingestErr := p.ingestBatch(ctx, events, latestSlot)
	p.advanceLastPolledSlot(processedThrough)
	if ingestErr != nil {
		p.updateCoverage()
		p.lggr.Warnw("poller start: ingest batch failed, resuming from last fully ingested slot",
			"startSlot", startSlot, "latestSlot", latestSlot,
			"lastFullyIngestedSlot", processedThrough, "err", ingestErr)
		return
	}
	if processedThrough < latestSlot {
		p.updateCoverage()
		p.lggr.Warnw("poller start: ingest batch truncated at event cap, resuming from last fully ingested slot",
			"startSlot", startSlot, "latestSlot", latestSlot,
			"lastFullyIngestedSlot", processedThrough, "maxEventsPerPoll", p.maxEventsPerPoll)
		return
	}
	p.updateCoverage()
}

// Close cancels the ticker loop and waits for it to return.
func (p *ExecutionAttemptPoller) Close() error {
	return p.StopOnce("ExecutionAttemptPoller", func() error {
		if p.cancelFunc != nil {
			p.cancelFunc()
		}
		p.wg.Wait()
		return nil
	})
}

// Ready composes the StateMachine state with backfill coverage. A non-nil error
// makes the executor defer and retry the message. Returns nil only when the poller
// has been started and the initial lookback window has been covered.
func (p *ExecutionAttemptPoller) Ready() error {
	if err := p.StateMachine.Ready(); err != nil {
		return err
	}
	if !p.lookbackCovered.Load() {
		return ErrBackfillInProgress
	}
	return nil
}

// Healthy reports lifecycle failures only. Backfill, ingest, and transient polling
// failures never fail Healthy; they are retried from the held cursor and must not
// make the executor classify a valid message as invalid.
func (p *ExecutionAttemptPoller) Healthy() error {
	return p.StateMachine.Healthy()
}

func (p *ExecutionAttemptPoller) Name() string {
	return "ExecutionAttemptPoller"
}

// HealthReport reports Ready so backfill progress is visible in component health.
func (p *ExecutionAttemptPoller) HealthReport() map[string]error {
	report := make(map[string]error)
	report[p.Name()] = p.Ready()
	return report
}

// GetAttempts returns cached attempts for the message. Cache miss is (nil, nil).
// Returns a copy of the cached slice so callers can mutate freely.
func (p *ExecutionAttemptPoller) GetAttempts(message protocol.Message) ([]protocol.ExecutionAttempt, error) {
	msgID, err := message.MessageID()
	if err != nil {
		return nil, fmt.Errorf("compute message ID: %w", err)
	}
	successful, failed := p.getAttemptsByOutcome(msgID)
	return append(successful, failed...), nil
}

// getAttemptsByOutcome returns the cached attempts for the message ID split by their
// on-chain outcome. HasHonestAttempt consumes the split (successful entries are skip
// evidence, failed entries are simulation triggers); GetAttempts is the merged view.
func (p *ExecutionAttemptPoller) getAttemptsByOutcome(msgID [32]byte) (successful, failed []protocol.ExecutionAttempt) {
	if cached, ok := p.attemptCache.Get(msgID); ok {
		successful = slices.Clone(cached)
	}
	if cached, ok := p.failedAttemptCache.Get(msgID); ok {
		failed = slices.Clone(cached)
	}
	return successful, failed
}

// runLoop is the single writer to lastPolledSlot and the cache after Start returns.
// A failed tick is logged and retried on the next one. The cursor advances only to
// the last fully ingested slot, so a transient RPC outage retries from the first
// uncovered slot rather than skipping it.
//
// This guarantee covers only the errors that FetchEvents returns. The log collector tries
// a block 46 times over approximately 8 hours before the worker pool stops the job. Aborted
// blocks do not reach FetchEvents: the cursor moves past the slot and its events are lost.
//
// NOTE: This package cannot find that loss. Monitor the solana_log_poller_blocks_skipped metric.
func (p *ExecutionAttemptPoller) runLoop(ctx context.Context) {
	ticker := time.NewTicker(p.pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := p.pollOnce(ctx); err != nil {
				p.lggr.Warnw("poll attempts failed", "err", err)
			}
		}
	}
}

// pollOnce ingests (lastPolledSlot, finalizedSlot]. Polling at finalized commitment means
// attempts become visible one finality behind the chain tip; that lag is acceptable because
// a not-yet-finalized attempt could still be dropped by a fork and is not worth trusting.
func (p *ExecutionAttemptPoller) pollOnce(ctx context.Context) error {
	// Note: we're setting up the tx baseline as finalized, so we don't need to worry about (any rare) reorgs.
	// However, this can be revisited if we want to support a more aggressive approach with confirmed commitment,
	// worst case scenario / symptom to monitor: multiple execution attempts for the same message, a lot of already-executed no-op offramp executions
	// Limit the poll deadline to the two fast RPC calls (GetSlot + FetchEvents).
	// The ingestion below fetches one transaction for each event and must not share
	// that budget.
	fetchCtx, cancelFetch := context.WithTimeout(ctx, p.pollFetchTimeout)
	defer cancelFetch()

	// get the latest finalized slot
	toSlot, err := p.rpcClient.GetSlot(fetchCtx, rpc.CommitmentFinalized)
	if err != nil {
		return fmt.Errorf("get slot: %w", err)
	}
	// Use this slot as the coverage target if startup did not set it.
	if p.coverageTarget.Load() == coverageTargetUnset {
		p.coverageTarget.Store(toSlot)
	}
	// resolve the range to fetch, capped at the lookback window
	fromSlot, err := p.resolveSlotRange(toSlot)
	if err != nil {
		if errors.Is(err, errNoNewSlots) {
			return nil
		}
		return fmt.Errorf("resolve slot range: %w", err)
	}
	// fetch events and ingest them; lastPolledSlot is advanced to the last fully
	// ingested slot, or to toSlot when the whole range is complete.
	events, err := p.logPoller.FetchEvents(fetchCtx, fromSlot, toSlot)
	if err != nil {
		return fmt.Errorf("fetch events: %w", err)
	}
	processedThrough, ingestErr := p.ingestBatch(ctx, events, toSlot)
	p.advanceLastPolledSlot(processedThrough)
	p.updateCoverage()
	if ingestErr != nil {
		return fmt.Errorf("ingest batch: %w", ingestErr)
	}
	if processedThrough < toSlot {
		p.lggr.Debugw("ingest batch truncated at event cap, resuming from last fully ingested slot",
			"lastFullyIngestedSlot", processedThrough,
			"toSlot", toSlot,
			"maxEventsPerPoll", p.maxEventsPerPoll)
	}
	return nil
}

func (p *ExecutionAttemptPoller) setPendingSlot(slot uint64, processed int) {
	p.pendingSlot = slot
	p.pendingSlotProcessed = processed
}

func (p *ExecutionAttemptPoller) clearPendingSlot() {
	p.pendingSlot = pendingSlotUnset
	p.pendingSlotProcessed = 0
}

// advanceLastPolledSlot advances the cursor to slot. The cursor never moves backward.
// The poller has a single writer, so a load and a conditional store are sufficient.
func (p *ExecutionAttemptPoller) advanceLastPolledSlot(slot uint64) {
	if slot > p.lastPolledSlot.Load() {
		p.lastPolledSlot.Store(slot)
	}
}

// updateCoverage marks the lookback window covered after the cursor reaches the
// startup target. A target of coverageTargetUnset means the target is not set.
func (p *ExecutionAttemptPoller) updateCoverage() {
	target := p.coverageTarget.Load()
	if target == coverageTargetUnset {
		return
	}
	if p.lastPolledSlot.Load() >= target {
		p.lookbackCovered.Store(true)
	}
}

// resolveSlotRange computes the (lastPolledSlot, toSlot] range to fetch, capped at the
// lookback window. Returns errNoNewSlots when the finalized slot has not advanced past
// the cursor. A cursor that lagged past the window holds a backlog older than the cache
// TTL, so those attempts would expire anyway. Drop it and fetch only the window,
// mirroring the startup backfill. Without this, a range that outgrows pollFetchTimeout
// retries ever-larger and never recovers.
// TODO: record poll duration and backlog size in metrics, so we can alert on a poll
// that is consistently slow or lagging.
func (p *ExecutionAttemptPoller) resolveSlotRange(toSlot uint64) (fromSlot uint64, err error) {
	lastPolledSlot := p.lastPolledSlot.Load()
	if toSlot <= lastPolledSlot {
		return 0, errNoNewSlots
	}
	fromSlot = slotAfter(lastPolledSlot)
	//nolint:gosec // G115: lookbackWindow is validated > 0 by the constructor; minSolanaSlotTime > 0
	lookbackSlots := uint64(p.lookbackWindow / minSolanaSlotTime)
	if toSlot-lastPolledSlot > lookbackSlots {
		windowStart := toSlot - lookbackSlots
		p.lggr.Warnw("poller cursor lagged past lookback window; dropping backlog",
			"lastPolledSlot", lastPolledSlot,
			"droppedSlots", windowStart-fromSlot,
			"fromSlot", windowStart,
			"toSlot", toSlot)
		fromSlot = windowStart
	}
	return fromSlot, nil
}

func slotBefore(slot uint64) uint64 {
	if slot == 0 {
		return 0
	}
	return slot - 1
}

func slotAfter(slot uint64) uint64 {
	if slot == ^uint64(0) {
		return slot
	}
	return slot + 1
}

// decodedExecutionAttempt pairs an attempt with its cache key. The messageID is the one
// decoded from the tx payload (already asserted equal to the event's), not the event's,
// so the key and the cached message can never disagree.
type decodedExecutionAttempt struct {
	messageID [32]byte
	attempt   protocol.ExecutionAttempt
	failed    bool
}

// resumePendingSlot finds the first event not handled by an earlier poll.
// The events must have the same sort order on each poll.
// The second result counts the events already handled in the slot at the returned index.
// The caller saves or clears the stored progress after the event loop.
func (p *ExecutionAttemptPoller) resumePendingSlot(events []logpoller.Event) (int, int) {
	// Start at the first event when no slot has saved progress.
	if p.pendingSlot == pendingSlotUnset {
		return 0, 0
	}

	// Earlier slots are complete, so find the saved slot in the sorted events.
	pending := p.pendingSlot
	i := 0
	for i < len(events) && events[i].SlotNumber < pending {
		i++
	}
	// The lookback window can advance past the saved slot, so its count no longer applies.
	if i >= len(events) || events[i].SlotNumber != pending {
		return i, 0
	}

	// Skip completed events in this slot to avoid repeated transaction reads.
	pendingStart := i
	skip := p.pendingSlotProcessed
	for i < len(events) && events[i].SlotNumber == pending && skip > 0 {
		i++
		skip--
	}
	// Restart the slot if the saved count exceeds the number of returned events.
	if skip > 0 {
		p.lggr.Warnw("ingest: pending slot has fewer events than expected; reprocessing slot",
			"slot", pending, "expectedProcessed", p.pendingSlotProcessed)
		return pendingStart, 0
	}

	// Keep the saved count while the next event belongs to the same slot.
	if i < len(events) && events[i].SlotNumber == pending {
		return i, p.pendingSlotProcessed
	}
	// The saved slot is complete, so the next slot starts with a zero count.
	return i, 0
}

// ingestBatch ingests events up to toSlot and returns the last fully ingested slot.
// It enforces maxEventsPerPoll inside a slot. The cursor stays before a slot with
// partial progress. pendingSlot and pendingSlotProcessed carry that progress to the
// next poll, so completed events in the slot are not read again. Permanent errors are
// logged and skipped. On success it returns toSlot, including when no event follows
// the last ingested slot.
func (p *ExecutionAttemptPoller) ingestBatch(ctx context.Context, events []logpoller.Event, toSlot uint64) (uint64, error) {
	// FetchEvents does not promise slot order. Sort by slot, then by transaction
	// signature and data, so the pending count identifies the same events on retry.
	slices.SortStableFunc(events, func(a, b logpoller.Event) int {
		if c := cmp.Compare(a.SlotNumber, b.SlotNumber); c != 0 {
			return c
		}
		if c := bytes.Compare(a.TxSignature[:], b.TxSignature[:]); c != 0 {
			return c
		}
		return bytes.Compare(a.Data, b.Data)
	})
	defer p.warnIfCacheLarge()

	i, slotProcessed := p.resumePendingSlot(events)
	if i == len(events) {
		p.clearPendingSlot()
		return toSlot, nil
	}
	slot := events[i].SlotNumber
	processed := 0
	for ; i < len(events); i++ {
		e := events[i]
		if e.SlotNumber != slot {
			slot = e.SlotNumber
			slotProcessed = 0
		}
		if err := ctx.Err(); err != nil {
			p.setPendingSlot(slot, slotProcessed)
			return slotBefore(e.SlotNumber), err
		}
		if processed >= p.maxEventsPerPoll {
			p.setPendingSlot(slot, slotProcessed)
			return slotBefore(e.SlotNumber), nil
		}
		if err := p.ingestEvent(ctx, e); err != nil {
			if errors.Is(err, errTransientIngest) {
				p.setPendingSlot(slot, slotProcessed)
				return slotBefore(e.SlotNumber), fmt.Errorf("ingest event at slot %d: %w", e.SlotNumber, err)
			}
			p.lggr.Warnw("ingest: skipping execution attempt event",
				"err", err,
				"slot", e.SlotNumber,
				"tx", e.TxSignature,
			)
		}
		processed++
		slotProcessed++
	}
	p.clearPendingSlot()
	return toSlot, nil
}

func (p *ExecutionAttemptPoller) warnIfCacheLarge() {
	if n := p.attemptCache.Len() + p.failedAttemptCache.Len(); n > warnCachedMessageIDs {
		p.lggr.Warnw("execution attempt cache has high number of entries",
			"cachedMessageIDs", n)
	}
}

// ingestEvent caches finalized InProgress events.
func (p *ExecutionAttemptPoller) ingestEvent(ctx context.Context, e logpoller.Event) error {
	attempt, ok, err := p.decodeExecutionAttemptEvent(ctx, e)
	if err != nil {
		p.lggr.Warnw("ingest: decode execution attempt failed",
			"err", err, "tx", e.TxSignature)
		return err
	}
	if ok {
		p.cacheAttempt(attempt)
	}
	return nil
}

func (p *ExecutionAttemptPoller) decodeExecutionAttemptEvent(
	ctx context.Context,
	e logpoller.Event,
) (decodedExecutionAttempt, bool, error) {
	// Check the discriminator before parsing.
	if !bytes.HasPrefix(e.Data, ccip_offramp.Event_ExecutionStateChangedV2[:]) {
		// HasPrefix also filters shorter data (noise) and other events from the same program.
		return decodedExecutionAttempt{}, false, nil
	}
	ev, err := ccip_offramp.ParseEvent_ExecutionStateChangedV2(e.Data)
	if err != nil {
		// Permanent error: do not wrap errTransientIngest. ingestBatch logs it and the
		// cursor advances. A retry cannot fix a binding mismatch.
		return decodedExecutionAttempt{}, false, fmt.Errorf(
			"decode ExecutionStateChangedV2 at slot %d tx %s: %w",
			e.SlotNumber, e.TxSignature.String(), err)
	}
	// InProgress identifies the transaction that supplied the verifier inputs.
	if ev.State != ccip_offramp.MessageExecutionState_InProgress {
		return decodedExecutionAttempt{}, false, nil
	}

	attempt, failed, err := p.fetchAttempt(ctx, e.TxSignature, *ev)
	if err != nil {
		return decodedExecutionAttempt{}, false, fmt.Errorf("messageID %x: %w", ev.MessageId, err)
	}

	return decodedExecutionAttempt{
		messageID: ev.MessageId,
		attempt:   attempt,
		failed:    failed,
	}, true, nil
}

// cacheAttempt appends to the cache for the attempt's outcome with a hard cap that drops
// the oldest entries, so the slice stays bounded under repeated re-ingestion from a held
// cursor. Newest attempts are the most relevant for HasHonestAttempt. Fresh slice, never
// mutate the cached backing array.
func (p *ExecutionAttemptPoller) cacheAttempt(attempt decodedExecutionAttempt) {
	cache, maxAttempts := p.attemptCache, maxAttemptsPerMessage
	if attempt.failed {
		cache, maxAttempts = p.failedAttemptCache, maxFailedAttemptsPerMessage
	}
	existing, _ := cache.Get(attempt.messageID)
	if len(existing) > warnAttemptsPerMessage {
		p.lggr.Warnw("execution attempt cache has high number of attempts for messageID",
			"messageID", fmt.Sprintf("%x", attempt.messageID),
			"attemptCount", len(existing))
	}
	if len(existing) >= maxAttempts {
		kept := make([]protocol.ExecutionAttempt, 0, maxAttempts)
		kept = append(kept, existing[len(existing)-maxAttempts+1:]...)
		existing = kept
	}
	cache.Add(attempt.messageID, append(slices.Clone(existing), attempt.attempt))
}

// fetchAttempt loads a finalized transaction and decodes its ExecuteV2 input. The second
// return reports whether the transaction reverted on-chain; the attempt is cached in
// failedAttemptCache instead of attemptCache when it did.
func (p *ExecutionAttemptPoller) fetchAttempt(
	ctx context.Context,
	sig solana.Signature,
	ev ccip_offramp.ExecutionStateChangedV2,
) (protocol.ExecutionAttempt, bool, error) {
	txRes, err := p.rpcClient.GetTransaction(ctx, sig)
	if err != nil {
		return protocol.ExecutionAttempt{}, false, fmt.Errorf("%w: get transaction: %w", errTransientIngest, err)
	}
	if txRes == nil || txRes.Transaction == nil {
		return protocol.ExecutionAttempt{}, false, fmt.Errorf("%w: empty transaction result", errTransientIngest)
	}
	if txRes.Meta == nil {
		return protocol.ExecutionAttempt{}, false, fmt.Errorf("%w: missing transaction meta", errTransientIngest)
	}
	failed := txRes.Meta.Err != nil
	if failed {
		p.lggr.Debugw("fetchAttempt: caching failed transaction",
			"messageID", fmt.Sprintf("%x", ev.MessageId), "tx", sig, "err", txRes.Meta.Err)
	}
	tx, err := txRes.Transaction.GetTransaction()
	if err != nil {
		return protocol.ExecutionAttempt{}, false, fmt.Errorf("extract transaction: %w", err)
	}

	execIxIndices := findExecuteV2Instructions(tx, p.offrampProgramID)
	if len(execIxIndices) == 0 {
		return protocol.ExecutionAttempt{}, false, errors.New("ExecuteV2 instruction not found in transaction")
	}
	// A tx can batch several ExecuteV2 ixs; use the one for the event's messageID.
	var errs []error
	for _, execIxIndex := range execIxIndices {
		attempt, dErr := p.decodeExecuteV2Instruction(tx, txRes, execIxIndex, sig, ev)
		if dErr == nil {
			return attempt, failed, nil
		}
		errs = append(errs, dErr)
	}
	return protocol.ExecutionAttempt{}, false, errors.Join(errs...)
}

// decodeExecuteV2Instruction decodes the ExecuteV2 ix at execIxIndex and returns an error
// when its messageID does not match the event's.
func (p *ExecutionAttemptPoller) decodeExecuteV2Instruction(
	tx *solana.Transaction,
	txRes *rpc.GetTransactionResult,
	execIxIndex int,
	sig solana.Signature,
	ev ccip_offramp.ExecutionStateChangedV2,
) (protocol.ExecutionAttempt, error) {
	executeIx, err := ccip_offramp.UnmarshalExecuteV2Instruction(tx.Message.Instructions[execIxIndex].Data)
	if err != nil {
		return protocol.ExecutionAttempt{}, fmt.Errorf("decode ExecuteV2 instruction: %w", err)
	}
	params := executeIx.Params
	if params.ExecInputs == nil {
		return p.decodeBufferedAttempt(tx, txRes, execIxIndex, sig, ev)
	}

	msg, err := protocol.DecodeMessage(params.ExecInputs.EncodedMessage)
	if err != nil {
		return protocol.ExecutionAttempt{}, fmt.Errorf("decode message: %w", err)
	}
	decodedID, err := msg.MessageID()
	if err != nil {
		return protocol.ExecutionAttempt{}, fmt.Errorf("compute message id: %w", err)
	}
	if decodedID != ev.MessageId {
		return protocol.ExecutionAttempt{}, fmt.Errorf(
			"event messageID %x does not match decoded messageID %x", ev.MessageId, decodedID)
	}

	// Keep verifier addresses and results positionally aligned.
	if len(params.ExecInputs.VerifierResults) != len(params.ExecInputs.Ccvs) {
		return protocol.ExecutionAttempt{}, fmt.Errorf(
			"ExecInputs positional mismatch: %d verifier results vs %d ccvs",
			len(params.ExecInputs.VerifierResults), len(params.ExecInputs.Ccvs))
	}

	// CCVS: the verifier addresses passed to execute, positionally aligned with the verifier results.
	// On Solana these are program IDs (32-byte pubkeys).
	ccvs := make([]protocol.UnknownAddress, len(params.ExecInputs.Ccvs))
	for i, pk := range params.ExecInputs.Ccvs {
		ccvs[i] = protocol.UnknownAddress(pk.Bytes())
	}

	return protocol.ExecutionAttempt{
		Report: protocol.AbstractAggregatedReport{
			Message: *msg,
			CCVS:    ccvs,
			CCVData: params.ExecInputs.VerifierResults,
		},
		TransactionGasLimit: extractComputeUnitLimit(tx),
	}, nil
}

// findExecuteV2Instructions returns the indices of the compiled instructions whose program is
// offrampProgramID and whose data starts with the ExecuteV2 discriminator, in tx order. The
// index also locates the ix's inner-instruction group in the tx meta (needed by the buffered path).
func findExecuteV2Instructions(tx *solana.Transaction, offrampProgramID solana.PublicKey) []int {
	discLen := len(ccip_offramp.Instruction_ExecuteV2)
	var indices []int
	for i := range tx.Message.Instructions {
		ix := &tx.Message.Instructions[i]
		progIdx := int(ix.ProgramIDIndex)
		if progIdx >= len(tx.Message.AccountKeys) {
			continue
		}
		if !tx.Message.AccountKeys[progIdx].Equals(offrampProgramID) {
			continue
		}
		if len(ix.Data) < discLen {
			continue
		}
		if !bytes.Equal(ix.Data[:discLen], ccip_offramp.Instruction_ExecuteV2[:]) {
			continue
		}
		indices = append(indices, i)
	}
	return indices
}

// decodeBufferedAttempt reconstructs an ExecutionAttempt for the buffered ExecuteV2 form
// (ExecInputs == nil, inputs carried via a buffer account) from the verify CPIs recorded in
// the transaction's inner instructions. The buffer account itself is NOT a viable source:
// execute_v2 closes it in the same transaction on success, so by finalized commitment it is
// empty. Verify CPIs are immutable tx meta, present on both successful and failed txs (a CPI
// that failed is still recorded), and carry everything needed: VerifyParams holds the encoded
// message, the messageID, and that CCV's verifier-result blob, the CPI's program ID is the CCV.
//
// An inner ix counts as a verify CPI for this attempt only if it (1) originated from the
// ExecuteV2 top-level ix, (2) starts with the committee-verifier Verify discriminator, uniform
// across all CCVs because the on-chain call_verify hardcodes that ix format for every verifier,
// and (3) decodes to VerifyParams whose messageID equals the event's.
func (p *ExecutionAttemptPoller) decodeBufferedAttempt(
	tx *solana.Transaction,
	txRes *rpc.GetTransactionResult,
	execIxIndex int,
	sig solana.Signature,
	ev ccip_offramp.ExecutionStateChangedV2,
) (protocol.ExecutionAttempt, error) {
	// Inner instructions are optional in the RPC meta contract: a node that doesn't record
	// them leaves us no way to reconstruct buffered attempts. Transient, so the cursor holds
	// and Healthy() surfaces the blindness rather than silently skipping every buffered attempt.
	if txRes.Meta == nil || txRes.Meta.InnerInstructions == nil {
		return protocol.ExecutionAttempt{}, fmt.Errorf("%w: transaction meta has no inner instructions", errTransientIngest)
	}

	var (
		encodedMessage []byte
		ccvs           []protocol.UnknownAddress
		ccvData        [][]byte
	)
	for _, group := range txRes.Meta.InnerInstructions {
		if int(group.Index) != execIxIndex {
			continue
		}
		for i := range group.Instructions {
			inner := &group.Instructions[i]
			discLen := len(committee_verifier.Instruction_Verify)
			if len(inner.Data) < discLen || !bytes.Equal(inner.Data[:discLen], committee_verifier.Instruction_Verify[:]) {
				continue
			}
			var vp committee_verifier.VerifyParams
			if uErr := vp.UnmarshalWithDecoder(bin.NewBorshDecoder(inner.Data[discLen:])); uErr != nil {
				continue
			}
			if vp.MessageId != ev.MessageId {
				continue
			}
			ccv, rErr := resolveProgramID(tx, txRes.Meta, inner.ProgramIDIndex)
			if rErr != nil {
				return protocol.ExecutionAttempt{}, fmt.Errorf("resolve verify CPI program ID: %w", rErr)
			}
			if encodedMessage == nil {
				encodedMessage = vp.EncodedMessage
			}
			ccvs = append(ccvs, protocol.UnknownAddress(ccv.Bytes()))
			ccvData = append(ccvData, vp.VerifierResults)
		}
	}
	// Meta was present but no verify CPI matched: the tx failed at/before the first verify,
	// so no CCV data exists anywhere in it. Permanent skip, nothing cacheable exists.
	if len(ccvs) == 0 {
		return protocol.ExecutionAttempt{}, errors.New("buffered execution attempt had no decodable verify CPIs")
	}

	msg, err := protocol.DecodeMessage(encodedMessage)
	if err != nil {
		return protocol.ExecutionAttempt{}, fmt.Errorf("decode message from verify CPI: %w", err)
	}
	decodedID, err := msg.MessageID()
	if err != nil {
		return protocol.ExecutionAttempt{}, fmt.Errorf("compute message id: %w", err)
	}
	if decodedID != ev.MessageId {
		return protocol.ExecutionAttempt{}, fmt.Errorf(
			"event messageID %x does not match decoded messageID %x", ev.MessageId, decodedID)
	}

	// The failed-tx case was already logged by fetchAttempt right after GetTransaction.
	p.lggr.Debugw("reconstructed buffered execution attempt from verify CPIs",
		"messageID", fmt.Sprintf("%x", ev.MessageId), "tx", sig, "ccvCount", len(ccvs))

	return protocol.ExecutionAttempt{
		Report: protocol.AbstractAggregatedReport{
			Message: *msg,
			CCVS:    ccvs,
			CCVData: ccvData,
		},
		TransactionGasLimit: extractComputeUnitLimit(tx),
	}, nil
}

// resolveProgramID maps a compiled instruction's program ID index to a public key, accounting
// for addresses loaded via lookup tables: static account keys first, then loaded writable,
// then loaded readonly. CCV programs may be ALT-loaded because account resolution returns
// lookup tables to the submitter.
func resolveProgramID(tx *solana.Transaction, meta *rpc.TransactionMeta, idx uint16) (solana.PublicKey, error) {
	i := int(idx)
	if i < len(tx.Message.AccountKeys) {
		return tx.Message.AccountKeys[i], nil
	}
	i -= len(tx.Message.AccountKeys)
	if i < len(meta.LoadedAddresses.Writable) {
		return meta.LoadedAddresses.Writable[i], nil
	}
	i -= len(meta.LoadedAddresses.Writable)
	if i < len(meta.LoadedAddresses.ReadOnly) {
		return meta.LoadedAddresses.ReadOnly[i], nil
	}
	return solana.PublicKey{}, fmt.Errorf("program ID index %d out of range (static=%d, loadedWritable=%d, loadedReadOnly=%d)",
		idx, len(tx.Message.AccountKeys), len(meta.LoadedAddresses.Writable), len(meta.LoadedAddresses.ReadOnly))
}

// extractComputeUnitLimit scans the ComputeBudget program instructions for a
// SetComputeUnitLimit entry and returns its u32 value as a *big.Int. Returns nil
// when no explicit limit is set, callers should treat nil as "runtime default."
func extractComputeUnitLimit(tx *solana.Transaction) *big.Int {
	for i := range tx.Message.Instructions {
		ix := &tx.Message.Instructions[i]
		progIdx := int(ix.ProgramIDIndex)
		if progIdx >= len(tx.Message.AccountKeys) {
			continue
		}
		if !tx.Message.AccountKeys[progIdx].Equals(fees.ComputeBudgetProgram) {
			continue
		}
		cuLimit, err := fees.ParseComputeUnitLimit(ix.Data)
		if err != nil {
			continue // ComputeBudget but not SetComputeUnitLimit (Price/HeapFrame).
		}
		return new(big.Int).SetUint64(uint64(cuLimit))
	}
	return nil
}
