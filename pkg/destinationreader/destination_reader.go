package destinationreader

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	"github.com/smartcontractkit/chainlink-ccv/executor/pkg/monitoring"
	"github.com/smartcontractkit/chainlink-ccv/pkg/chainaccess"
	"github.com/smartcontractkit/chainlink-ccv/protocol"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/services"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/txm"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/ccip_offramp"
	solcommon "github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	"github.com/smartcontractkit/chainlink-ccip-solana/pkg/commonreader"
	"github.com/smartcontractkit/chainlink-ccip-solana/pkg/logpoller"
	"github.com/smartcontractkit/chainlink-ccip-solana/pkg/solanarpc"
)

const solanaDestinationReaderServiceName = "solana.destinationreader.Service"

var (
	_ chainaccess.DestinationReader        = (*DestinationReader)(nil)
	_ chainaccess.ExecutorMonitoringSetter = (*DestinationReader)(nil)
	_ services.Service                     = (*DestinationReader)(nil)
)

// RPCClient is the Solana JSON-RPC subset this package reads through. It covers the
// slot, transaction and simulation calls made here plus the account read that
// commonreader performs.
type RPCClient interface {
	solanarpc.SlotReader
	solanarpc.TxReader
	solanarpc.Simulator
	solanarpc.AccountReader
	solcommon.AccountInfoReader
}

// Params configures the Solana DestinationReader.
type Params struct {
	Lggr logger.Logger
	// ChainSelector is the destination chain this reader serves (must match message.DestChainSelector).
	ChainSelector protocol.ChainSelector
	// RPC is the Solana JSON-RPC client; required. It is an adapter over the selected
	// MultiNode node for reads and simulation.
	RPC RPCClient
	// OfframpProgramID is the deployed ccip-offramp program; required (no default).
	// pkg/accessors calls solutil.SetProgramID before constructing this reader.
	OfframpProgramID solana.PublicKey
	// RMNRemoteAddress is the deployed RMN Remote program ID; required (no default).
	RMNRemoteAddress solana.PublicKey
	// LogPoller provides ExecutionStateChangedV2 events to the internal ExecutionAttemptPoller.
	// DR owns its Start/Close lifecycle (see DestinationReader.Start/Close).
	LogPoller logpoller.Service
	// Config tunes the internal execution attempt poller. Zero values use defaults.
	Config Config
}

// DestinationReader is the Solana implementation of chainaccess.DestinationReader,
// started/closed by chainlink-ccv's ChainlinkExecutor like the EVM reader.
type DestinationReader struct {
	services.StateMachine
	lggr                   logger.Logger
	chainSelector          protocol.ChainSelector
	offrampProgramID       solana.PublicKey
	rmnRemoteAddress       solana.PublicKey
	rpc                    RPCClient
	logPoller              logpoller.Service
	executionAttemptPoller *ExecutionAttemptPoller
	monitoring             monitoring.Monitoring

	// simFeePayerMu guards simFeePayer, which is set after construction once the
	// executor keystore is wired (see SetSimulationFeePayer).
	simFeePayerMu sync.RWMutex
	simFeePayer   solana.PublicKey
}

// NewDestinationReader builds a reader that reads message execution state and CCV quorum from chain.
// The internal ExecutionAttemptPoller is constructed here; lifecycle is wired through DR's Start/Close.
func NewDestinationReader(params Params) (*DestinationReader, error) {
	var errs []error
	if params.Lggr == nil {
		errs = append(errs, errors.New("logger is required"))
	}
	if params.ChainSelector == 0 {
		errs = append(errs, errors.New("chain selector is required"))
	}
	if params.RPC == nil {
		errs = append(errs, errors.New("rpc client is required"))
	}
	if params.LogPoller == nil {
		errs = append(errs, errors.New("log poller is required"))
	}
	var zeroKey solana.PublicKey
	if params.OfframpProgramID.Equals(zeroKey) {
		errs = append(errs, errors.New("offramp program ID is required"))
	}
	if params.RMNRemoteAddress.Equals(zeroKey) {
		errs = append(errs, errors.New("rmn remote address is required"))
	}
	if len(errs) != 0 {
		return nil, errors.Join(errs...)
	}

	lggr := logger.Named(params.Lggr, "DestinationReader")

	poller, err := NewExecutionAttemptPoller(ExecutionAttemptPollerParams{
		Lggr:             lggr,
		LogPoller:        params.LogPoller,
		RPC:              params.RPC,
		OfframpProgramID: params.OfframpProgramID,
		Config:           params.Config,
	})
	if err != nil {
		return nil, fmt.Errorf("construct execution attempt poller: %w", err)
	}

	return &DestinationReader{
		lggr:                   lggr,
		chainSelector:          params.ChainSelector,
		offrampProgramID:       params.OfframpProgramID,
		rmnRemoteAddress:       params.RMNRemoteAddress,
		rpc:                    params.RPC,
		logPoller:              params.LogPoller,
		executionAttemptPoller: poller,
		monitoring:             monitoring.NewNoopExecutorMonitoring(),
	}, nil
}

// Start brings up the logpoller and execution-attempt poller in order. The poller
// starts its backfill in the background; startup RPC errors are retried and Ready
// stays false until coverage. If the poller cannot start, the logpoller is closed
// before returning so we do not leak its underlying log collector.
func (d *DestinationReader) Start(ctx context.Context) error {
	return d.StartOnce(solanaDestinationReaderServiceName, func() error {
		if err := d.logPoller.Start(ctx); err != nil {
			d.logPoller.Close()
			return fmt.Errorf("start logpoller: %w", err)
		}
		if err := d.executionAttemptPoller.Start(ctx); err != nil {
			if cErr := d.logPoller.Close(); cErr != nil {
				d.lggr.Warnw("failed to close logpoller after poller start failed", "err", cErr)
			}
			return fmt.Errorf("start execution attempt poller: %w", err)
		}
		return nil
	})
}

// Close stops the execution-attempt poller and logpoller in reverse order. Both Close
// errors are joined so a failure in one doesn't hide the other.
func (d *DestinationReader) Close() error {
	return d.StopOnce(solanaDestinationReaderServiceName, func() error {
		var errs []error
		if err := d.executionAttemptPoller.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close execution attempt poller: %w", err))
		}
		if err := d.logPoller.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close logpoller: %w", err))
		}
		return errors.Join(errs...)
	})
}

// Name returns the service name used for health reporting.
func (d *DestinationReader) Name() string {
	return strings.Join([]string{d.chainSelector.String(), solanaDestinationReaderServiceName}, ".")
}

func (d *DestinationReader) HealthReport() map[string]error {
	report := make(map[string]error)
	report[d.Name()] = d.Ready()
	report[d.executionAttemptPollerHealthName()] = d.executionAttemptPoller.Ready()
	return report
}

func (d *DestinationReader) executionAttemptPollerHealthName() string {
	return strings.Join([]string{d.chainSelector.String(), "ExecutionAttemptPoller"}, ".")
}

// SetSimulationFeePayer sets the funded fee payer for the get_ccvs_for_msg simulation.
// Solana rejects an unfunded payer even with SigVerify=false, so the executor wires its
// transmitter key after construction.
func (d *DestinationReader) SetSimulationFeePayer(payer solana.PublicKey) {
	d.simFeePayerMu.Lock()
	defer d.simFeePayerMu.Unlock()
	d.simFeePayer = payer
}

// SetExecutorMonitoring implements chainaccess.ExecutorMonitoringSetter. The accessor
// builds the reader with no-op monitoring because the executor's process-level monitoring
// does not exist yet; the executor swaps in the real one here before the coordinator starts.
func (d *DestinationReader) SetExecutorMonitoring(m monitoring.Monitoring) {
	if m == nil {
		return
	}
	d.monitoring = m
}

// GetRMNCursedSubjects queries the destination chain's RMN Remote contract for cursed subjects.
func (d *DestinationReader) GetRMNCursedSubjects(ctx context.Context) ([]protocol.Bytes16, error) {
	return commonreader.ReadRMNCursedSubjects(ctx, d.rpc, d.rmnRemoteAddress)
}

// GetMessageSuccess checks the destination chain to verify if a message has been executed successfully.
func (d *DestinationReader) GetMessageSuccess(ctx context.Context, message protocol.Message) (bool, error) {
	if message.DestChainSelector != d.chainSelector {
		d.lggr.Warnw("GetMessageSuccess: dest chain mismatch",
			"messageDest", message.DestChainSelector, "readerChain", d.chainSelector)
		return false, fmt.Errorf(
			"message dest chain %d does not match destination reader chain %d",
			message.DestChainSelector, d.chainSelector,
		)
	}
	msgID, err := message.MessageID()
	if err != nil {
		return false, fmt.Errorf("compute message ID: %w", err)
	}
	pda, _, err := state.FindOfframpMessageStatePDA(msgID, d.offrampProgramID)
	if err != nil {
		return false, fmt.Errorf("derive message state PDA: %w", err)
	}
	// Finalized by design: the terminal "already executed?" gate can afford the
	// finality window; racing and dedup are HasHonestAttempt's job.
	resp, err := d.rpc.GetAccountInfo(ctx, pda, rpc.CommitmentFinalized)
	if err != nil {
		if errors.Is(err, rpc.ErrNotFound) {
			d.lggr.Debugw("GetMessageSuccess: message state account not found", "messageID", msgID)
			return false, nil
		}
		return false, fmt.Errorf("get message state: %w", err)
	}
	// A prefunded PDA (no data) can exist on-chain but is not a MessageState.
	// Treat it as "not executed". The offramp overwrites the account during execute.
	msgStateData := resp.GetBinary()
	if len(msgStateData) == 0 {
		d.lggr.Debugw("GetMessageSuccess: message state account exists but has no data (prefunded?)", "messageID", msgID)
		return false, nil
	}
	msgState, err := ccip_offramp.ParseAccount_MessageState(msgStateData)
	if err != nil {
		d.lggr.Debugw("GetMessageSuccess: message state account is not a MessageState", "messageID", msgID, "err", err)
		return false, nil
	}
	return msgState.ExecutionState == ccip_offramp.MessageExecutionState_Success, nil
}

// GetCCVSForMessage queries the destination offramp's get_ccvs_for_msg instruction for the
// required CCV quorum of the given message.
func (d *DestinationReader) GetCCVSForMessage(ctx context.Context, message protocol.Message) (protocol.CCVAddressInfo, error) {
	if message.DestChainSelector != d.chainSelector {
		d.lggr.Warnw("GetCCVSForMessage: dest chain mismatch",
			"messageDest", message.DestChainSelector, "readerChain", d.chainSelector)
		return protocol.CCVAddressInfo{}, fmt.Errorf(
			"message dest chain %d does not match destination reader chain %d",
			message.DestChainSelector, d.chainSelector,
		)
	}

	d.simFeePayerMu.RLock()
	feePayer := d.simFeePayer
	d.simFeePayerMu.RUnlock()
	// Solana rejects an unfunded payer even with SigVerify=false. This static
	// configuration check runs before message encoding and hashing.
	if feePayer == (solana.PublicKey{}) {
		return protocol.CCVAddressInfo{}, errors.New("simulation fee payer not configured")
	}

	encodedMsg, err := message.Encode()
	if err != nil {
		return protocol.CCVAddressInfo{}, fmt.Errorf("encode message: %w", err)
	}
	msgID, err := message.MessageID()
	if err != nil {
		return protocol.CCVAddressInfo{}, fmt.Errorf("compute message ID: %w", err)
	}

	start := time.Now()
	resp, err := getCCVsForMessage(
		ctx,
		d.lggr,
		d.rpc,
		d.offrampProgramID,
		encodedMsg,
		feePayer,
	)
	if err != nil {
		d.monitoring.Metrics().IncrementOfframpGetCCVsForMessageFailure(ctx, d.chainSelector)
		d.lggr.Warnw("GetCCVSForMessage: failed to resolve onchain CCV quorum",
			"messageID", msgID,
			"sourceChain", message.SourceChainSelector,
			"destChain", d.chainSelector,
			"offrampProgramID", d.offrampProgramID,
			"feePayer", feePayer,
			"error", err,
		)
		return protocol.CCVAddressInfo{}, fmt.Errorf("get CCVs for message: %w", err)
	}
	// record only successful calls for clean distribution
	d.monitoring.Metrics().RecordOfframpGetCCVsForMessageLatency(ctx, time.Since(start), d.chainSelector)

	ccvInfo := protocol.CCVAddressInfo{
		RequiredCCVs:      pubkeysToUnknownAddresses(resp.RequiredCcvs),
		OptionalCCVs:      pubkeysToUnknownAddresses(resp.OptionalCcvs),
		OptionalThreshold: resp.OptionalThreshold,
	}
	d.lggr.Infow("GetCCVSForMessage: resolved onchain CCV quorum",
		"messageID", msgID,
		"sourceChain", message.SourceChainSelector,
		"destChain", d.chainSelector,
		"requiredCCVs", len(ccvInfo.RequiredCCVs),
		"optionalCCVs", len(ccvInfo.OptionalCCVs),
		"optionalThreshold", ccvInfo.OptionalThreshold,
	)
	return ccvInfo, nil
}

// GetExecutionAttempts returns cached prior execution attempts for the message
// (cache miss: nil, nil). See ExecutionAttemptPoller doc for the Solana-specific
// observability model (InProgress events + tx.Meta.Err).
func (d *DestinationReader) GetExecutionAttempts(_ context.Context, message protocol.Message) ([]protocol.ExecutionAttempt, error) {
	return d.executionAttemptPoller.GetAttempts(message)
}

// IsReady gates on the poller having covered its initial lookback window, before
// that, a cache miss is indistinguishable from "we haven't looked yet".
func (d *DestinationReader) IsReady(_ protocol.ChainSelector) bool {
	if err := d.Ready(); err != nil {
		return false
	}
	return d.executionAttemptPoller.Ready() == nil
}

// CheckHealth surfaces lifecycle/startup failures. Runtime polling errors remain
// retryable inside the poller; the upstream executor treats a non-nil health error
// as a critical failure and drops the incoming message.
func (d *DestinationReader) CheckHealth(_ protocol.ChainSelector) error {
	if err := d.Healthy(); err != nil {
		return err
	}
	return d.executionAttemptPoller.Healthy()
}

// HasHonestAttempt reports whether a prior attempt used the same verifier data.
// The result distinguishes honest attempts, non-honest attempts, and no cached attempts.
// Required CCVs must all be present with valid blobs; optional CCVs follow the
// on-chain selection order, the first `threshold` configured entries present in
// the attempt are selected, and their slots' blobs must all be valid.
//
// A successful match is skip evidence. A failed match proves nothing (InProgress fires
// before verification) and only triggers a simulation of this executor's own submission.
// The simulation classifies a failure by the position of the failing instruction in the
// transaction. An instruction error at the execute_v2 position is skip evidence and the
// message needs manual execution; a revert there matches an on-chain FAILURE on EVM, and
// a compute budget shortfall at that position counts too, even if InProgress never fired.
// An error at another position, or a transaction level error, returns an error so the
// executor retries. A simulation that cannot run fails open and does not hold the message.
func (d *DestinationReader) HasHonestAttempt(ctx context.Context, message protocol.Message, verifierResults []protocol.VerifierResult, ccvAddressInfo protocol.CCVAddressInfo) (chainaccess.HonestAttemptCheck, error) {
	msgID, err := message.MessageID()
	if err != nil {
		return chainaccess.HonestAttemptCheck{}, fmt.Errorf("compute message ID: %w", err)
	}

	successful, failed := d.executionAttemptPoller.getAttemptsByOutcome(msgID)
	if len(successful) == 0 && len(failed) == 0 {
		return chainaccess.HonestAttemptCheck{}, nil
	}

	// An empty CCV quorum cannot establish honesty, so any cached attempt is non-honest.
	if len(ccvAddressInfo.RequiredCCVs) == 0 && ccvAddressInfo.OptionalThreshold == 0 {
		return chainaccess.HonestAttemptCheck{NotHonestAttemptExists: true}, nil
	}

	// Group verifier results by CCV before comparing each positional value.
	ccvToResults := make(map[string][]protocol.VerifierResult, len(verifierResults))
	for _, r := range verifierResults {
		key := r.VerifierDestAddress.String()
		ccvToResults[key] = append(ccvToResults[key], r)
	}

	match := func(attempt protocol.ExecutionAttempt) bool {
		// The poller caches an attempt under the ID of its decoded message, so the
		// bucket key already equals msgID. Do not recompute the hash per attempt.
		// Too few compute units can never execute, not a terminal attempt.
		if !hasHonestGasLimit(message, attempt) {
			return false
		}
		return honestCCVData(attempt, ccvAddressInfo.RequiredCCVs, len(ccvAddressInfo.RequiredCCVs), ccvToResults) &&
			honestCCVData(attempt, ccvAddressInfo.OptionalCCVs, int(ccvAddressInfo.OptionalThreshold), ccvToResults)
	}

	if slices.ContainsFunc(successful, match) {
		return chainaccess.HonestAttemptCheck{Honest: true}, nil
	}

	if slices.ContainsFunc(failed, match) {
		if simErr := d.simulateExecutionAttempt(ctx, message, verifierResults, ccvAddressInfo); simErr != nil {
			if errors.Is(simErr, errSimulatedExecutionReverted) {
				d.lggr.Warnw("execute_v2 simulation reverted, message needs manual execution",
					"messageID", msgID, "err", simErr)
				return chainaccess.HonestAttemptCheck{Honest: true}, nil
			}
			if errors.Is(simErr, errSimulatedExecutionFailed) {
				return chainaccess.HonestAttemptCheck{}, fmt.Errorf("execution attempt simulation: %w", simErr)
			}
			d.lggr.Warnw("execution attempt simulation could not run, proceeding with submission",
				"messageID", msgID, "err", simErr)
		}
	}
	return chainaccess.HonestAttemptCheck{NotHonestAttemptExists: true}, nil
}

// hasHonestGasLimit reports whether the attempt supplied the CU budget an honest
// executor would use (ExecutionGasLimit, capped at the tx max).
// A cheap tx with valid CCV data but too little compute cannot execute.
func hasHonestGasLimit(message protocol.Message, attempt protocol.ExecutionAttempt) bool {
	if attempt.TransactionGasLimit == nil {
		// A nil limit (default budget) may indicate a 3rd-party attempt, but rejecting
		// is safe: under-reporting never loses a message (terminal GetMessageSuccess gate).
		return false
	}
	return attempt.TransactionGasLimit.Uint64() >= honestComputeUnitLimit(message)
}

// honestComputeUnitLimit is the CU budget an honest executor attaches: min between
// ExecutionGasLimit and MaxComputeUnitLimit
func honestComputeUnitLimit(message protocol.Message) uint64 {
	return min(uint64(message.ExecutionGasLimit), uint64(txm.MaxComputeUnitLimit))
}

// honestCCVData mirrors the on-chain selection in ensure_ccv_quorum_is_reached: walk the
// configured `ccvs` list in order, select the first `threshold` entries present in the
// attempt's Report.CCVS (first slot wins), and require every selected slot's blob to match
// a known verifier result. A selected slot with an invalid blob aborts on-chain, so it reads
// as dishonest here too, skipping it and counting a later entry would let a planted attempt
// look honest. Note: this intentionally does not match the EVM
// executionchecker.honestCCVs count-any-threshold semantics, which diverge from the
// Solana on-chain selection.
func honestCCVData(attempt protocol.ExecutionAttempt, ccvs []protocol.UnknownAddress, threshold int, ccvToResults map[string][]protocol.VerifierResult) bool {
	if threshold == 0 {
		return true
	}
	selected := 0
	for _, addr := range ccvs {
		idx := slices.IndexFunc(attempt.Report.CCVS, func(a protocol.UnknownAddress) bool {
			return a.String() == addr.String()
		})
		if idx == -1 {
			continue
		}
		if idx >= len(attempt.Report.CCVData) ||
			!ccvDataMatchesAny(attempt.Report.CCVData[idx], ccvToResults[addr.String()]) {
			return false
		}
		selected++
		if selected == threshold {
			return true
		}
	}
	return false
}

func ccvDataMatchesAny(blob []byte, knownResults []protocol.VerifierResult) bool {
	for _, r := range knownResults {
		if bytes.Equal(r.CCVData, blob) {
			return true
		}
	}
	return false
}
