package transmitter

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/google/uuid"

	"github.com/smartcontractkit/chainlink-ccv/executor"
	"github.com/smartcontractkit/chainlink-ccv/pkg/chainaccess"
	"github.com/smartcontractkit/chainlink-ccv/protocol"
	"github.com/smartcontractkit/chainlink-common/keystore"
	commonconfig "github.com/smartcontractkit/chainlink-common/pkg/config"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/services"
	solconfig "github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/monitor"
	soltxm "github.com/smartcontractkit/chainlink-solana/pkg/solana/txm"
	txmutils "github.com/smartcontractkit/chainlink-solana/pkg/solana/txm/utils"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/utils"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/ccip_offramp"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/ccip"
	solcommon "github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/pkg/solanarpc"
)

var _ chainaccess.ContractTransmitter = (*SolanaContractTransmitter)(nil)

// ErrTransmitterClosed is returned from ConvertAndWriteMessageToChain when the transmitter
// has already been closed. Callers should treat this as a terminal, non-retryable condition.
var ErrTransmitterClosed = errors.New("transmitter closed")

// RPCClient is the read-only RPC surface the transmitter uses for account resolution,
// lookup-table fetch and blockhash capture.
type RPCClient interface {
	ccip.AccountResolutionRPC
	solcommon.AccountInfoReader
	solanarpc.BlockhashReader
}

// computeUnitLimit derives the CU limit for an execute_v2 tx from the message's
// ExecutionGasLimit, which already includes CcipReceiveGasLimit.
func computeUnitLimit(msg protocol.Message) uint32 {
	cu := min(uint64(msg.ExecutionGasLimit), soltxm.MaxComputeUnitLimit)
	return uint32(cu)
}

// SolanaContractTransmitter submits CCIP execute_v2 transactions via chainlink-solana txm.
//
// The inline path returns nil as soon as txm accepts the transaction. txm rebroadcasts the same
// signed transaction for a bounded time and does not rebuild it with a new blockhash. A terminal
// inline failure is not reported to the executor, so this executor does not retry the message;
// other executors retry it on their own turn.
//
// The buffered path runs synchronously on the caller's goroutine (the CCV executor worker) under
// the caller's context. On failure it returns an error, and the executor retries the message via
// its heap-based retry loop.
//
// Lifecycle:
//   - NewSolanaContractTransmitter starts the underlying txm, which spawns its own background
//     goroutines for broadcast/retry/confirmation, and the fee payer balance monitor.
//   - Close closes the balance monitor and the txm and is idempotent (guarded by closeOnce).
//     After Close returns, ConvertAndWriteMessageToChain returns ErrTransmitterClosed.
type SolanaContractTransmitter struct {
	lggr             logger.SugaredLogger
	chainSelector    protocol.ChainSelector
	chainID          string
	offrampProgramID solana.PublicKey
	rpcClient        RPCClient
	feePayerPubKey   solana.PublicKey
	txm              soltxm.TxManager
	balanceMonitor   services.Service
	ccipLog          logger.Logger
	cfg              Config

	closeOnce sync.Once
	closeErr  error
	// closed is closed by Close so ConvertAndWriteMessageToChain can fast-fail new work.
	closed chan struct{}
}

// FeePayer returns the funded transmitter public key used to pay for transactions.
// It doubles as a funded fee payer for read-only instruction simulations.
func (t *SolanaContractTransmitter) FeePayer() solana.PublicKey {
	return t.feePayerPubKey
}

type Params struct {
	Lggr             logger.Logger
	ChainSelector    protocol.ChainSelector
	ChainID          string
	OfframpProgramID solana.PublicKey
	RPC              *solanarpc.Client
	Keystore         keystore.Keystore
	KeyName          string
	Config           Config
}

func NewSolanaContractTransmitter(ctx context.Context, p Params) (*SolanaContractTransmitter, error) {
	cfg := p.Config.WithDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid transmitter config: %w", err)
	}
	if p.Keystore == nil {
		return nil, errors.New("keystore is required")
	}

	if p.RPC == nil {
		return nil, errors.New("rpc is required")
	}

	if strings.TrimSpace(p.KeyName) == "" {
		return nil, errors.New("transmitter key name is required")
	}

	feePayerPubKey, ks, err := loadTransmitterKey(ctx, p.Keystore, p.KeyName)
	if err != nil {
		return nil, err
	}

	l := logger.Sugared(p.Lggr).Named("Transmitter")
	l.Infow("solana executor fee payer ready", "pubkey", feePayerPubKey.String(), "chainSelector", p.ChainSelector)

	tomlCfg := solconfig.Defaults()
	tomlCfg.SetDefaults()

	// Keep terminal status records available for finality polling.
	tomlCfg.Chain.TxRetentionTimeout = &cfg.TxRetentionTimeout
	tomlCfg.Chain.BalancePollPeriod = commonconfig.MustNewDuration(time.Minute)
	txmInst, err := soltxm.NewTxm(p.ChainID, utils.NewOnceLoader(p.RPC.Select), p.RPC.Send, &tomlCfg, ks, p.Lggr)
	if err != nil {
		return nil, fmt.Errorf("new txm: %w", err)
	}
	balanceMonitor, err := monitor.NewBalanceMonitor(p.ChainID, &tomlCfg, p.Lggr, ks,
		utils.NewOnceLoader(func(ctx context.Context) (monitor.BalanceClient, error) { return p.RPC.Select(ctx) }))
	if err != nil {
		return nil, fmt.Errorf("new balance monitor: %w", err)
	}
	if err = errors.Join(txmInst.Start(ctx), balanceMonitor.Start(ctx), ctx.Err()); err != nil {
		// Close to release resources from Start before returning.
		if cerr := errors.Join(balanceMonitor.Close(), txmInst.Close()); cerr != nil {
			p.Lggr.Warnw("failed to close txm and balance monitor after startup failed", "error", cerr)
		}
		return nil, fmt.Errorf("start txm and balance monitor: %w", err)
	}

	t := &SolanaContractTransmitter{
		lggr:             l,
		chainSelector:    p.ChainSelector,
		chainID:          p.ChainID,
		offrampProgramID: p.OfframpProgramID,
		rpcClient:        p.RPC,
		feePayerPubKey:   feePayerPubKey,
		txm:              txmInst,
		balanceMonitor:   balanceMonitor,
		ccipLog:          logger.Named(p.Lggr, "ccip.ResolveExecAccounts"),
		cfg:              cfg,
		closed:           make(chan struct{}),
	}

	return t, nil
}

// Close shuts down the balance monitor and the underlying txm and marks the transmitter closed
// so subsequent calls to ConvertAndWriteMessageToChain return ErrTransmitterClosed. Idempotent
// and safe to call from any goroutine.
//
// In-flight submissions run on the caller's goroutine and are scoped to the caller's context,
// so they are cancelled by the executor's own shutdown rather than here.
func (t *SolanaContractTransmitter) Close() error {
	t.closeOnce.Do(func() {
		close(t.closed)
		t.closeErr = errors.Join(t.balanceMonitor.Close(), t.txm.Close())
	})
	return t.closeErr
}

func (t *SolanaContractTransmitter) ConvertAndWriteMessageToChain(ctx context.Context, report protocol.AbstractAggregatedReport) error {
	// Fast-fail if the transmitter has been closed
	select {
	case <-t.closed:
		return ErrTransmitterClosed
	default:
	}

	// Terminally reject messages with a zero ExecutionGasLimit. At a zero CU
	// limit the chainlink-solana txm omits the SetComputeUnitLimit instruction entirely,
	// so the execute_v2 tx would run on Solana's default compute budget, burning
	// fee-payer funds on a message the sender priced at nothing. The executor treats
	// ErrMessageRejectedByTransmitter as non-retryable.
	if report.Message.ExecutionGasLimit == 0 {
		msgID := report.Message.MustMessageID().String() // 0x-prefixed hex
		t.lggr.Errorw("rejecting message with zero execution gas limit: refusing to submit an unexecutable tx that would burn fee-payer funds on Solana's default compute budget",
			"messageID", msgID)
		return fmt.Errorf("zero execution gas limit, message %s unexecutable: %w",
			msgID, executor.ErrMessageRejectedByTransmitter)
	}

	if err := checkDestBlob(report.Message); err != nil {
		msgID := report.Message.MustMessageID().String()
		t.lggr.Errorw("rejecting message that execute_v2 rejects on every attempt: invalid receiver or dest blob",
			"messageID", msgID,
			"sourceChainSelector", report.Message.SourceChainSelector,
			"sender", report.Message.Sender.String(),
			"reason", err.Error())
		return fmt.Errorf("message %s: %w: %w", msgID, err, executor.ErrMessageRejectedByTransmitter)
	}

	execInputs, err := t.buildExecutionInputs(report)
	if err != nil {
		return err
	}

	// Two conditions trigger the buffered fallback:
	//   1. The simulation tx itself exceeds the RPC's base64 size limit.
	//      This happens for very large payloads (e.g. 4KB+) where the exec_inputs alone exceed
	//      the limit before we can even measure the final tx. The RPC returns "too large".
	//   2. Simulation succeeds but the fully resolved tx (with all accounts + LUTs) exceeds
	//      1232 bytes. This covers borderline payloads that are small enough to simulate but
	//      too large once all resolved accounts are included.
	inlineTx, _, recent, err := t.buildInlineExecTx(ctx, execInputs)
	if err != nil {
		// ResolveExecAccounts simulates the full inline execute_v2 tx. When exec inputs
		// are large the RPC rejects the simulation before we can measure serialized size,
		// so fall through to the buffered path instead of failing the executor.
		if isOversizedTxError(err) {
			t.lggr.Infow("inline account resolution failed due to oversized tx, switching to buffered flow",
				"messageID", report.Message.MustMessageID().String(), "error", err)
			return t.submitBuffered(ctx, execInputs, report)
		}
		return err
	}

	size, err := calculateTxSizeInBytes(inlineTx)
	if err != nil {
		return fmt.Errorf("calculate inline tx size: %w", err)
	}

	if size <= solcommon.MaxSolanaTxSize {
		t.lggr.Debugw("inline tx size within limits, submitting via txm",
			"size", size, "limit", solcommon.MaxSolanaTxSize, "messageID", report.Message.MustMessageID().String())
		return t.enqueueInline(ctx, inlineTx, recent, report)
	}

	t.lggr.Infow("inline tx oversized, switching to buffered flow",
		"size", size, "limit", solcommon.MaxSolanaTxSize, "messageID", report.Message.MustMessageID().String())

	return t.submitBuffered(ctx, execInputs, report)
}

// checkDestBlob returns an error for a receiver or dest blob that execute_v2 rejects on every
// attempt, so the message is refused before any fee is spent. execute_v2 rejects any non-empty
// receiver that is not 32 bytes, before the token-only filter. execute_v2 parses the blob for a
// token transfer, or when it calls the receiver: a non-empty, non-zero receiver with data or a
// receive gas limit (is_offramp_token_only_transfer).
func checkDestBlob(m protocol.Message) error {
	if len(m.Receiver) > 0 && len(m.Receiver) != solana.PublicKeyLength {
		return fmt.Errorf("receiver is %d bytes, want %d", len(m.Receiver), solana.PublicKeyLength)
	}
	callsReceiver := len(m.Receiver) > 0 &&
		!bytes.Equal(m.Receiver, solana.PublicKey{}.Bytes()) &&
		(len(m.Data) > 0 || m.CcipReceiveGasLimit > 0)
	if m.TokenTransfer == nil && !callsReceiver {
		return nil
	}
	numAccounts, err := ccip.ValidateDestBlob(m.DestBlob)
	if err != nil {
		return fmt.Errorf("invalid dest blob: %w", err)
	}
	if callsReceiver && numAccounts > 64 {
		return fmt.Errorf("dest blob has %d messaging accounts, max 64", numAccounts)
	}
	return nil
}

func (t *SolanaContractTransmitter) buildExecutionInputs(report protocol.AbstractAggregatedReport) (ccip_offramp.ExecutionInputsV2, error) {
	encodedMsg, err := report.Message.Encode()
	if err != nil {
		return ccip_offramp.ExecutionInputsV2{}, errors.Join(executor.ErrMessageEncoding, fmt.Errorf("encode message: %w", err))
	}
	return ccip.BuildExecutionInputsV2(encodedMsg, report.CCVS, report.CCVData)
}

func (t *SolanaContractTransmitter) buildInlineExecTx(ctx context.Context, execInputs ccip_offramp.ExecutionInputsV2) (*solana.Transaction, map[solana.PublicKey]solana.PublicKeySlice, *rpc.GetLatestBlockhashResult, error) {
	resolved, err := ccip.ResolveExecAccounts(ctx, t.rpcClient, t.offrampProgramID, execInputs, t.feePayerPubKey, t.ccipLog)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("resolve execute accounts: %w", err)
	}

	lutMap, err := solcommon.GetAddressLookupTableMap(ctx, t.rpcClient, resolved.LookupTables)
	if err != nil {
		return nil, nil, nil, err
	}

	recent, err := t.rpcClient.GetLatestBlockhash(ctx)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("get latest blockhash: %w", err)
	}

	tx, err := solana.NewTransaction(
		[]solana.Instruction{resolved.Ix},
		recent.Value.Blockhash,
		solana.TransactionPayer(t.feePayerPubKey),
		solana.TransactionAddressTables(lutMap),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("new transaction: %w", err)
	}

	return tx, lutMap, recent, nil
}

func (t *SolanaContractTransmitter) enqueueInline(ctx context.Context, tx *solana.Transaction, recent *rpc.GetLatestBlockhashResult, report protocol.AbstractAggregatedReport) error {
	txID := uuid.NewString()
	lastValid := recent.Value.LastValidBlockHeight
	cuLimit := computeUnitLimit(report.Message)
	if err := t.txm.Enqueue(ctx, t.feePayerPubKey.String(), tx, &txID, lastValid,
		txmutils.SetComputeUnitLimit(cuLimit),
	); err != nil {
		return fmt.Errorf("txm enqueue: %w", err)
	}

	// Log line matches chainlink-ccv EVM contract transmitter ("submitted tx to chain") so CCV e2e
	// log asserters and future Loki-based tests do not need Solana-specific patterns.
	t.lggr.Infow("submitted tx to chain (txm)", "messageID", report.Message.MustMessageID().String(), "txID", txID)
	return nil
}

func isOversizedTxError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "too large")
}
