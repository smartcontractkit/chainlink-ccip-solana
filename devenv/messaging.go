package devenv

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gagliardetto/solana-go"
	computebudget "github.com/gagliardetto/solana-go/programs/compute-budget"
	"github.com/gagliardetto/solana-go/rpc"

	"github.com/smartcontractkit/chainlink-ccv/build/devenv/cciptestinterfaces"
	devenvcommon "github.com/smartcontractkit/chainlink-ccv/build/devenv/common"
	"github.com/smartcontractkit/chainlink-ccv/protocol"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/base_token_pool"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/ccip_common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/ccip_offramp"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/ccip_router"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/ccip"
	solcommon "github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	solstate "github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	soltokens "github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/tokens"
	deploysolcommon "github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	cvops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/committeeverifier"
	executorops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/executor"
	offrampops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/offramp"
	routerops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/router"
)

// sendComputeUnitLimit is the compute budget requested for ccip_send_v2. The 200k default
// is not enough once the token pool implements the v2 interface.
const sendComputeUnitLimit = 600_000

// ConfirmSendOnSource waits for the source Solana chain to send a CCIP message.
// Message is identified by message ID or sequence number.
// Under the hood, it uses a logpoller to scan RouterOnRamp CcipMessageSentV2 events,
// and polls every tick until a match is found or the timeout expires.
func (c *Chain) ConfirmSendOnSource(ctx context.Context, to uint64, key cciptestinterfaces.MessageEventKey, timeout time.Duration) (cciptestinterfaces.MessageSentEvent, error) {
	if key.MessageID == ([32]byte{}) && key.SeqNum == 0 {
		return cciptestinterfaces.MessageSentEvent{}, errors.New("message key must include message ID or sequence number")
	}

	if err := c.initPollers(); err != nil {
		return cciptestinterfaces.MessageSentEvent{}, fmt.Errorf("init pollers: %w", err)
	}

	// Start scanning from slightly before now so we don't miss events emitted between the caller's send and this call.
	currentSlot, err := c.chain.Client.GetSlot(ctx, rpc.CommitmentFinalized)
	if err != nil {
		return cciptestinterfaces.MessageSentEvent{}, fmt.Errorf("get current slot: %w", err)
	}
	fromSlot := currentSlot - min(currentSlot, 100)

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return cciptestinterfaces.MessageSentEvent{}, ctx.Err()
		case <-timer.C:
			return cciptestinterfaces.MessageSentEvent{}, fmt.Errorf(
				"ConfirmSendOnSource timed out after %s waiting for send event (to=%d, key=%+v)", timeout, to, key)
		case <-ticker.C:
			toSlot, slotErr := c.chain.Client.GetSlot(ctx, rpc.CommitmentFinalized)
			if slotErr != nil {
				c.logger.Warn().Err(slotErr).Msg("ConfirmSendOnSource: GetSlot failed, retrying")
				continue
			}
			events, fetchErr := c.routerPoller.FetchEvents(ctx, fromSlot, toSlot)
			if fetchErr != nil {
				c.logger.Warn().Err(fetchErr).Msg("ConfirmSendOnSource: FetchEvents failed, retrying")
				continue
			}
			for _, evt := range events {
				parsed, parseErr := ccip_router.ParseEvent_CcipMessageSentV2(evt.Data)
				if parseErr != nil {
					continue
				}
				if parsed.DestChainSelector != to {
					continue
				}

				// Check MessageID before decoding to avoid unnecessary decode on non-matching events.
				if key.MessageID != ([32]byte{}) && parsed.MessageId != key.MessageID {
					continue
				}

				// Decode the message to get return value and SeqNum
				// V2 events don't expose SeqNum directly, it's inside EncodedMessage.
				decodedMsg, decErr := protocol.DecodeMessage(parsed.EncodedMessage)
				if decErr != nil {
					c.logger.Warn().Err(decErr).Msg("ConfirmSendOnSource: DecodeMessage failed, skipping event")
					continue
				}

				if key.MessageID == ([32]byte{}) {
					// MessageID is zero, so SeqNum must be set.
					if uint64(decodedMsg.SequenceNumber) != key.SeqNum {
						continue
					}
				}

				c.logger.Info().
					Hex("messageID", parsed.MessageId[:]).
					Uint64("seqNum", uint64(decodedMsg.SequenceNumber)).
					Uint64("destChain", to).
					Msg("ConfirmSendOnSource: CcipMessageSentV2 found")

				issuers := make([]protocol.UnknownAddress, len(parsed.Receipts))
				for i, r := range parsed.Receipts {
					issuers[i] = protocol.UnknownAddress(r.Issuer.Bytes())
				}

				return cciptestinterfaces.MessageSentEvent{
					MessageID:      parsed.MessageId,
					Sender:         protocol.UnknownAddress(parsed.Sender.Bytes()),
					Message:        decodedMsg,
					ReceiptIssuers: issuers,
					VerifierBlobs:  parsed.VerifierBlobs,
				}, nil
			}
			fromSlot = toSlot + 1
		}
	}
}

// ConfirmExecOnDest waits for the destination Solana chain to execute a CCIP message.
// Message is identified by message ID or sequence number.
// Under the hood, it uses a logpoller to scan offramp ExecutionStateChangedV2 events
// and polls every tick until a match with terminal state is found or the timeout expires.
func (c *Chain) ConfirmExecOnDest(ctx context.Context, from uint64, key cciptestinterfaces.MessageEventKey, timeout time.Duration) (cciptestinterfaces.ExecEnvelope, error) {
	if key.MessageID == ([32]byte{}) && key.SeqNum == 0 {
		return cciptestinterfaces.ExecEnvelope{}, errors.New("message key must include message ID or sequence number")
	}

	if err := c.initPollers(); err != nil {
		return cciptestinterfaces.ExecEnvelope{}, fmt.Errorf("init pollers: %w", err)
	}

	// Start scanning from slightly before now so we don't miss events emitted between the caller's send and this call.
	currentSlot, err := c.chain.Client.GetSlot(ctx, rpc.CommitmentFinalized)
	if err != nil {
		return cciptestinterfaces.ExecEnvelope{}, fmt.Errorf("get current slot: %w", err)
	}
	fromSlot := currentSlot - min(currentSlot, 100)

	// CCV load verification passes timeout=0 and relies on ctx; EVM ignores the param the same way.
	// A zero-duration timer fires immediately and may falsely fail before Solana exec is observed.
	var timerC <-chan time.Time
	if timeout > 0 {
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		timerC = timer.C
	}

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return cciptestinterfaces.ExecEnvelope{}, ctx.Err()
		case <-timerC:
			return cciptestinterfaces.ExecEnvelope{}, fmt.Errorf(
				"ConfirmExecOnDest timed out after %s waiting for execution event (from=%d, key=%+v)", timeout, from, key)
		case <-ticker.C:
			// Use Finalized commitment to avoid RPC -32016 errors where confirmed slots race ahead of the RPC's min-context-slot.
			toSlot, slotErr := c.chain.Client.GetSlot(ctx, rpc.CommitmentFinalized)
			if slotErr != nil {
				c.logger.Warn().Err(slotErr).Msg("ConfirmExecOnDest: GetSlot failed, retrying")
				continue
			}
			events, fetchErr := c.offrampPoller.FetchEvents(ctx, fromSlot, toSlot)
			if fetchErr != nil {
				c.logger.Warn().Err(fetchErr).Msg("ConfirmExecOnDest: FetchEvents failed, retrying")
				continue
			}
			for _, evt := range events {
				parsed, parseErr := ccip_offramp.ParseEvent_ExecutionStateChangedV2(evt.Data)
				if parseErr != nil {
					continue
				}
				if parsed.SourceChainSelector != from {
					continue
				}
				// Match by MessageID or SeqNum as fallback
				if key.MessageID != ([32]byte{}) {
					if parsed.MessageId != key.MessageID {
						continue
					}
				} else {
					// MessageID is zero, so SeqNum must be set.
					if parsed.MessageNumber != key.SeqNum {
						continue
					}
				}

				state := cciptestinterfaces.MessageExecutionState(uint8(parsed.State))

				c.logger.Info().
					Hex("messageID", parsed.MessageId[:]).
					Str("solana signature", evt.TxSignature.String()).
					Uint64("seqNum", parsed.MessageNumber).
					Str("state", parsed.State.String()).
					Msg("ConfirmExecOnDest: ExecutionStateChangedV2 found")

				// Skip non-terminal states and keep polling for a final outcome.
				if state != cciptestinterfaces.ExecutionStateSuccess && state != cciptestinterfaces.ExecutionStateFailure {
					continue
				}

				return cciptestinterfaces.ExecEnvelope{
					Event: cciptestinterfaces.ExecutionStateChangedEvent{
						SourceChainSelector: protocol.ChainSelector(parsed.SourceChainSelector),
						MessageID:           parsed.MessageId,
						MessageNumber:       parsed.MessageNumber,
						State:               state,
						ReturnData:          parsed.ReturnData,
					},
					TxID: protocol.ByteSlice(evt.TxSignature[:]),
				}, nil
			}
			fromSlot = toSlot + 1
		}
	}
}

func (c *Chain) ConfirmTokenPoolEventInTx(
	ctx context.Context,
	txSig solana.Signature,
	poolProgram solana.PublicKey,
	expectedDiscriminator [8]byte,
) (any, error) {
	tx, meta, err := c.fetchConfirmedTxAndMeta(ctx, txSig)
	if err != nil {
		return nil, err
	}

	switch expectedDiscriminator {
	case base_token_pool.Event_Minted:
		evt, parseErr := solcommon.ParseEventFromLogs(meta.LogMessages, base_token_pool.ParseEvent_Minted)
		if parseErr != nil {
			return nil, fmt.Errorf("parse Minted event from execution logs: %w", parseErr)
		}
		return evt, nil
	case base_token_pool.Event_Released:
		evt, parseErr := solcommon.ParseEventFromLogs(meta.LogMessages, base_token_pool.ParseEvent_Released)
		if parseErr != nil {
			return nil, fmt.Errorf("parse Released event from execution logs: %w", parseErr)
		}
		return evt, nil
	case base_token_pool.Event_ReleasedOrMinted:
		evt, parseErr := solcommon.ExtractCPIEvent(tx, meta, poolProgram, base_token_pool.ParseEvent_ReleasedOrMinted)
		if parseErr != nil {
			logEvt, logErr := solcommon.ParseEventFromLogs(meta.LogMessages, base_token_pool.ParseEvent_ReleasedOrMinted)
			if logErr == nil {
				return logEvt, nil
			}

			// Some deployments still emit Minted/Released semantics on v2 lanes.
			if mintedEvt, mintedErr := solcommon.ParseEventFromLogs(meta.LogMessages, base_token_pool.ParseEvent_Minted); mintedErr == nil {
				return mintedEvt, nil
			}
			if releasedEvt, releasedErr := solcommon.ParseEventFromLogs(meta.LogMessages, base_token_pool.ParseEvent_Released); releasedErr == nil {
				return releasedEvt, nil
			}

			return nil, fmt.Errorf("extract ReleasedOrMinted event (cpi=%w, logs=%s)", parseErr, logErr.Error())
		}
		return evt, nil
	case base_token_pool.Event_Burned:
		evt, parseErr := solcommon.ParseEventFromLogs(meta.LogMessages, base_token_pool.ParseEvent_Burned)
		if parseErr != nil {
			return nil, fmt.Errorf("parse Burned event from send logs: %w", parseErr)
		}
		return evt, nil
	case base_token_pool.Event_Locked:
		evt, parseErr := solcommon.ParseEventFromLogs(meta.LogMessages, base_token_pool.ParseEvent_Locked)
		if parseErr != nil {
			return nil, fmt.Errorf("parse Locked event from send logs: %w", parseErr)
		}
		return evt, nil
	case base_token_pool.Event_LockedOrBurned:
		evt, parseErr := solcommon.ExtractCPIEvent(tx, meta, poolProgram, base_token_pool.ParseEvent_LockedOrBurned)
		if parseErr != nil {
			logEvt, logErr := solcommon.ParseEventFromLogs(meta.LogMessages, base_token_pool.ParseEvent_LockedOrBurned)
			if logErr == nil {
				return logEvt, nil
			}

			// Some deployments still emit Burned/Locked semantics on v2 lanes.
			if burnedEvt, burnedErr := solcommon.ParseEventFromLogs(meta.LogMessages, base_token_pool.ParseEvent_Burned); burnedErr == nil {
				return burnedEvt, nil
			}
			if lockedEvt, lockedErr := solcommon.ParseEventFromLogs(meta.LogMessages, base_token_pool.ParseEvent_Locked); lockedErr == nil {
				return lockedEvt, nil
			}

			return nil, fmt.Errorf("extract LockedOrBurned event (cpi=%w, logs=%s)", parseErr, logErr.Error())
		}
		return evt, nil
	default:
		return nil, fmt.Errorf("unsupported token pool event discriminator %v", expectedDiscriminator)
	}
}

func (c *Chain) ConfirmSendEventInTx(ctx context.Context, txSig solana.Signature) (*ccip_router.CcipMessageSentV2, error) {
	routerProgramID, err := routerPubkeyFromDataStore(c.e.DataStore, c.chainDetails.ChainSelector, "")
	if err != nil {
		return nil, fmt.Errorf("resolve router program: %w", err)
	}
	tx, meta, err := c.fetchConfirmedTxAndMeta(ctx, txSig)
	if err != nil {
		return nil, err
	}
	evt, err := solcommon.ExtractCPIEvent(tx, meta, routerProgramID, ccip_router.ParseEvent_CcipMessageSentV2)
	if err != nil {
		return nil, fmt.Errorf("extract CcipMessageSentV2 event: %w", err)
	}
	return evt, nil
}

func (c *Chain) fetchConfirmedTxAndMeta(ctx context.Context, txSig solana.Signature) (*solana.Transaction, *rpc.TransactionMeta, error) {
	txRes, err := c.getTransactionBySignature(ctx, txSig)
	if err != nil {
		return nil, nil, err
	}
	if txRes.Meta == nil {
		return nil, nil, errors.New("transaction meta is required to inspect CPI events")
	}
	tx, err := txRes.Transaction.GetTransaction()
	if err != nil {
		return nil, nil, fmt.Errorf("extract transaction: %w", err)
	}
	return tx, txRes.Meta, nil
}

func (c *Chain) getTransactionBySignature(ctx context.Context, sig solana.Signature) (*rpc.GetTransactionResult, error) {
	maxTxVersion := uint64(0)
	txRes, err := c.chain.Client.GetTransaction(ctx, sig, &rpc.GetTransactionOpts{
		Commitment:                     rpc.CommitmentFinalized,
		MaxSupportedTransactionVersion: &maxTxVersion,
	})
	if err != nil {
		return nil, fmt.Errorf("get transaction %s: %w", sig.String(), err)
	}
	if txRes == nil || txRes.Transaction == nil {
		return nil, fmt.Errorf("empty transaction result for %s", sig.String())
	}
	return txRes, nil
}

// ConfirmBufferInitializedOnDest waits for the offramp to emit a BufferInitialized CPI event
// for the given messageID. This proves the buffered submission flow in
// pkg/transmitter/buffer.go was exercised (the inline path never writes a buffer PDA).
//
// fromSlot must be captured before the message is sent so we don't miss the event.
func (c *Chain) ConfirmBufferInitializedOnDest(
	ctx context.Context,
	messageID [32]byte,
	fromSlot uint64,
	timeout time.Duration,
) (ccip_offramp.BufferInitialized, solana.Signature, error) {
	if c.chain.Client == nil {
		return ccip_offramp.BufferInitialized{}, solana.Signature{}, errors.New("RPC client not configured")
	}
	if err := c.initPollers(); err != nil {
		return ccip_offramp.BufferInitialized{}, solana.Signature{}, fmt.Errorf("init pollers: %w", err)
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ccip_offramp.BufferInitialized{}, solana.Signature{}, ctx.Err()
		case <-timer.C:
			return ccip_offramp.BufferInitialized{}, solana.Signature{},
				fmt.Errorf("timed out after %s waiting for BufferInitialized for messageID 0x%x", timeout, messageID)
		case <-ticker.C:
			toSlot, slotErr := c.chain.Client.GetSlot(ctx, rpc.CommitmentFinalized)
			if slotErr != nil {
				c.logger.Warn().Err(slotErr).Msg("ConfirmBufferInitializedOnDest: GetSlot failed, retrying")
				continue
			}
			events, fetchErr := c.offrampPoller.FetchEvents(ctx, fromSlot, toSlot)
			if fetchErr != nil {
				c.logger.Warn().Err(fetchErr).Msg("ConfirmBufferInitializedOnDest: FetchEvents failed, retrying")
				continue
			}
			for _, evt := range events {
				parsed, parseErr := ccip_offramp.ParseEvent_BufferInitialized(evt.Data)
				if parseErr != nil {
					continue
				}
				if parsed.BufferId != messageID {
					continue
				}

				c.logger.Info().
					Hex("messageID", messageID[:]).
					Str("authority", parsed.Authority.String()).
					Str("txSignature", evt.TxSignature.String()).
					Msg("ConfirmBufferInitializedOnDest: BufferInitialized found")

				return *parsed, evt.TxSignature, nil
			}
		}
	}
}

// ConfirmBufferClosedOnDest waits for the offramp to emit a BufferClosed CPI event
// for the given messageID. This proves the buffered submission flow in
// pkg/transmitter/buffer.go was exercised (the inline path never writes a buffer PDA).
//
// fromSlot must be captured before the message is sent so we don't miss the event.
func (c *Chain) ConfirmBufferClosedOnDest(
	ctx context.Context,
	messageID [32]byte,
	fromSlot uint64,
	timeout time.Duration,
) (ccip_offramp.BufferClosed, solana.Signature, error) {
	if c.chain.Client == nil {
		return ccip_offramp.BufferClosed{}, solana.Signature{}, errors.New("RPC client not configured")
	}
	if err := c.initPollers(); err != nil {
		return ccip_offramp.BufferClosed{}, solana.Signature{}, fmt.Errorf("init pollers: %w", err)
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ccip_offramp.BufferClosed{}, solana.Signature{}, ctx.Err()
		case <-timer.C:
			return ccip_offramp.BufferClosed{}, solana.Signature{},
				fmt.Errorf("timed out after %s waiting for BufferClosed for messageID 0x%x", timeout, messageID)
		case <-ticker.C:
			toSlot, slotErr := c.chain.Client.GetSlot(ctx, rpc.CommitmentFinalized)
			if slotErr != nil {
				c.logger.Warn().Err(slotErr).Msg("ConfirmBufferClosedOnDest: GetSlot failed, retrying")
				continue
			}
			events, fetchErr := c.offrampPoller.FetchEvents(ctx, fromSlot, toSlot)
			if fetchErr != nil {
				c.logger.Warn().Err(fetchErr).Msg("ConfirmBufferClosedOnDest: FetchEvents failed, retrying")
				continue
			}
			for _, evt := range events {
				parsed, parseErr := ccip_offramp.ParseEvent_BufferClosed(evt.Data)
				if parseErr != nil {
					continue
				}
				if parsed.BufferId != messageID {
					continue
				}

				c.logger.Info().
					Hex("messageID", messageID[:]).
					Str("authority", parsed.Authority.String()).
					Str("txSignature", evt.TxSignature.String()).
					Msg("ConfirmBufferClosedOnDest: BufferClosed found")

				return *parsed, evt.TxSignature, nil
			}
		}
	}
}

// GetExecutorArgs encodes SVM executor args for V3 messages.
// A nil opts is allowed so CCV's EVMTXGun can call BuildV3ExtraArgs without importing
// Solana-specific types; we supply the same empty blob used in evm2solana e2e tests.
func (c *Chain) GetExecutorArgs(opts any) (cciptestinterfaces.MessageV3ExecutorArgs, error) {
	var args ccip.SVMExecutorArgs
	if opts == nil {
		args = ccip.SVMExecutorArgs{SvmDestBlob: ccip_common.SvmDestBlob{
			UseAta: 0, AccountIsWritableBitmap: 0, Accounts: []solana.PublicKey{},
		}}
	} else {
		var ok bool
		args, ok = opts.(ccip.SVMExecutorArgs)
		if !ok {
			return nil, fmt.Errorf("expected ccip.SVMExecutorArgs, got %T", opts)
		}
	}
	bytes, err := args.BorshEncode()
	if err != nil {
		return nil, fmt.Errorf("encode executor args: %w", err)
	}
	return bytes, nil
}

// GetTokenArgs is noop for Solana, currently only SVMExecutorArgs is needed, could be extended in the future token pool
func (c *Chain) GetTokenArgs(_ any) (cciptestinterfaces.MessageV3TokenArgs, error) {
	return []byte{}, nil
}

func (c *Chain) GetTokenReceiver(receiver any) (cciptestinterfaces.MessageV3TokenReceiver, error) {
	if receiver == nil {
		return nil, nil // no token receiver specified, this is allowed for arbitrary message passing
	}
	switch v := receiver.(type) {
	case string:
		pk, err := solana.PublicKeyFromBase58(v)
		if err != nil {
			return nil, fmt.Errorf("failed to decode SVM address '%s': %w", v, err)
		}
		return pk.Bytes(), nil
	case protocol.UnknownAddress:
		if len(v) != solana.PublicKeyLength {
			return nil, fmt.Errorf("invalid SVM address length, expected %v, got %d", solana.PublicKeyLength, len(v))
		}
		return cciptestinterfaces.MessageV3TokenReceiver(v), nil
	default:
		return nil, fmt.Errorf("unsupported token receiver type %T, expected string or protocol.UnknownAddress", receiver)
	}
}

// SendMessage delegates to BuildChainMessage + SendChainMessage for backward compat.
func (c *Chain) SendMessage(ctx context.Context, dest uint64, fields cciptestinterfaces.MessageFields, extraArgsProvider cciptestinterfaces.ExtraArgsDataProvider, _ uint8) (cciptestinterfaces.MessageSentEvent, error) {
	var extraArgs cciptestinterfaces.GenericExtraArgs
	// Go interfaces can hold either a value or a pointer to the concrete type;
	// normalize to a value so the subsequent type assertion handles both.
	if p, ok := extraArgsProvider.(*cciptestinterfaces.MessageOptions); ok && p != nil {
		extraArgsProvider = *p
	}
	if opts, ok := extraArgsProvider.(cciptestinterfaces.MessageOptions); ok {
		if len(opts.Executor) == 0 || len(opts.CCVs) == 0 {
			defaultCCV, defaultExecutor, err := c.DefaultCCVAndExecutorPubKeys()
			if err != nil {
				return cciptestinterfaces.MessageSentEvent{}, fmt.Errorf("resolve default CCV and executor: %w", err)
			}
			if len(opts.Executor) == 0 {
				opts.Executor = defaultExecutor.Bytes()
			}
			if len(opts.CCVs) == 0 {
				opts.CCVs = []protocol.CCV{{CCVAddress: defaultCCV.Bytes(), Args: []byte{}, ArgsLen: 0}}
			}
		}
		var err error
		extraArgs, err = SerializeMessageV3ExtraArgs(opts)
		if err != nil {
			return cciptestinterfaces.MessageSentEvent{}, fmt.Errorf("serialize extra args: %w", err)
		}
	}
	msg, err := c.BuildChainMessage(ctx, fields, extraArgs)
	if err != nil {
		return cciptestinterfaces.MessageSentEvent{}, fmt.Errorf("build chain message: %w", err)
	}
	sent, _, err := c.SendChainMessage(ctx, dest, msg, SolanaSendOptions{})
	if err != nil {
		return cciptestinterfaces.MessageSentEvent{}, fmt.Errorf("send chain message: %w", err)
	}
	return sent, nil
}

// BuildChainMessage builds an Svm2AnyMessage.
func (c *Chain) BuildChainMessage(_ context.Context, fields cciptestinterfaces.MessageFields, extraArgs cciptestinterfaces.GenericExtraArgs) (cciptestinterfaces.GenericChainMessage, error) {
	var tokenAmounts []ccip_router.SvmTokenAmount
	if fields.TokenAmount.Amount != nil && fields.TokenAmount.Amount.Sign() > 0 {
		tokenPubkey := solana.PublicKeyFromBytes(fields.TokenAmount.TokenAddress)
		tokenAmounts = append(tokenAmounts, ccip_router.SvmTokenAmount{
			Token:  tokenPubkey,
			Amount: fields.TokenAmount.Amount.Uint64(),
		})
	}

	return ccip_router.Svm2AnyMessage{
		Receiver:     fields.Receiver,
		Data:         fields.Data,
		TokenAmounts: tokenAmounts,
		FeeToken:     solana.PublicKey{}, // native SOL
		ExtraArgs:    extraArgs,
	}, nil
}

// SendChainMessage submits an Svm2AnyMessage via ccip_send_v2 and returns the event
// plus the tx signature (Solana's tx identifier, analogous to EVM's tx hash).
func (c *Chain) SendChainMessage(ctx context.Context, dest uint64, message cciptestinterfaces.GenericChainMessage, sendOption cciptestinterfaces.ChainSendOption) (cciptestinterfaces.MessageSentEvent, protocol.ByteSlice, error) {
	inputMsg, ok := message.(ccip_router.Svm2AnyMessage)
	if !ok {
		return cciptestinterfaces.MessageSentEvent{}, nil, fmt.Errorf("expected ccip_router.Svm2AnyMessage, got %T", message)
	}

	signer := c.chain.DeployerKey
	// Go interfaces can hold either a value or a pointer to the concrete type;
	// normalize to a value so the subsequent type assertion handles both.
	if p, ok := sendOption.(*SolanaSendOptions); ok && p != nil {
		sendOption = *p
	}
	if solOpts, ok := sendOption.(SolanaSendOptions); ok && solOpts.Signer != nil {
		signer = solOpts.Signer
	}
	if signer == nil {
		return cciptestinterfaces.MessageSentEvent{}, nil, errors.New("private key not set — use BuildTestChains to create funded chains for sending")
	}
	routerProgramID, err := routerPubkeyFromDataStore(c.e.DataStore, c.chainDetails.ChainSelector, "")
	if err != nil {
		return cciptestinterfaces.MessageSentEvent{}, nil, err
	}

	// The caller (BuildV3ExtraArgs -> SerializeMessageV3ExtraArgs) already resolved
	// Executor/CCVs/ExecutorArgs into inputMsg.ExtraArgs; decode rather than rebuild them.
	var decodedExtraArgs ccip_common.GenericExtraArgsV3
	if err1 := ccip.DeserializeExtraArgs(&decodedExtraArgs, inputMsg.ExtraArgs, ccip.GenericExtraArgsV3Tag); err1 != nil {
		return cciptestinterfaces.MessageSentEvent{}, nil, fmt.Errorf("decode v3 extra args: %w", err1)
	}

	c.logger.Info().
		Str("router_program_id", routerProgramID.String()).
		Msg("resolving ccip_send_v2 accounts")

	resolved, err := ccip.ResolveCcipSendV2Accounts(
		ctx,
		c.chain.Client,
		routerProgramID,
		dest,
		inputMsg,
		signer.PublicKey(),
		c.ccipLggr,
	)
	if err != nil {
		return cciptestinterfaces.MessageSentEvent{}, nil, fmt.Errorf("build CcipSendV2 instruction: %w", err)
	}

	lutMap, err := c.sendLookupTables(ctx, resolved.LookupTables)
	if err != nil {
		return cciptestinterfaces.MessageSentEvent{}, nil, err
	}

	// ccip_send_v2 exceeds the 200k default when the token pool implements the v2 interface
	// (extra GetFee and GetRequiredCcvs CPIs), so request a larger budget explicitly.
	instructions := []solana.Instruction{
		computebudget.NewSetComputeUnitLimitInstruction(sendComputeUnitLimit).Build(),
	}
	// The router moves the sender's tokens into the pool with its fee billing signer PDA as
	// the authority, so the sender has to delegate the amount first. Real senders do this
	// through the SDK; here it is part of acting as the sender.
	for _, tokenAmount := range inputMsg.TokenAmounts {
		// Decimals only: a mint's total supply can exceed math.MaxInt64 (Circle's devnet USDC
		// does), which TokenSupply cannot parse.
		decimals, decimalsErr := soltokens.TokenDecimals(ctx, c.chain.Client, tokenAmount.Token, rpc.CommitmentConfirmed)
		if decimalsErr != nil {
			return cciptestinterfaces.MessageSentEvent{}, nil, decimalsErr
		}
		// The mint's owner program decides which token program holds the sender's ATA
		// (classic SPL vs Token-2022); hardcoding the classic program broke T22 sends.
		mintInfo, mintInfoErr := c.chain.Client.GetAccountInfo(ctx, tokenAmount.Token)
		if mintInfoErr != nil {
			return cciptestinterfaces.MessageSentEvent{}, nil, fmt.Errorf("get mint %s: %w", tokenAmount.Token, mintInfoErr)
		}
		approveIx, approveErr := approveFeeBillingSigner(routerProgramID, signer.PublicKey(), tokenAmount, decimals, mintInfo.Value.Owner)
		if approveErr != nil {
			return cciptestinterfaces.MessageSentEvent{}, nil, approveErr
		}
		instructions = append(instructions, approveIx)
	}
	instructions = append(instructions, resolved.Ix)

	result, err := solcommon.SendAndConfirmWithLookupTablesAndRetries(ctx, c.chain.Client,
		instructions, *signer, rpc.CommitmentConfirmed, lutMap)
	if err != nil {
		return cciptestinterfaces.MessageSentEvent{}, nil, fmt.Errorf("send and confirm: %w", err)
	}
	c.logger.Println("Got result from SendAndConfirm, extracting event. Onchain logs:", result.Meta.LogMessages)

	tx, err := result.Transaction.GetTransaction()
	if err != nil {
		return cciptestinterfaces.MessageSentEvent{}, nil, fmt.Errorf("get transaction: %w", err)
	}
	txSig := tx.Signatures[0]
	c.logger.Println("Got transaction", txSig)

	event, err := solcommon.ExtractCPIEvent(tx, result.Meta, routerProgramID, ccip_router.ParseEvent_CcipMessageSentV2)
	if err != nil {
		return cciptestinterfaces.MessageSentEvent{}, nil, fmt.Errorf("extract CPI event: %w", err)
	}
	c.logger.Println("Got event", event, "with receipts", event.Receipts)

	c.logger.Info().
		Uint64("dest", dest).
		Hex("messageID", event.MessageId[:]).
		Str("txSignature", txSig.String()).
		Msg("Solana: CCIPMessageSentV2 emitted via ccip_send_v2")

	sent, err := messageSentEvent(event)
	if err != nil {
		return cciptestinterfaces.MessageSentEvent{}, nil, err
	}
	return sent, txSig[:], nil
}

// messageSentEvent converts the router's CcipMessageSentV2 event into the test interface's form.
func messageSentEvent(event *ccip_router.CcipMessageSentV2) (cciptestinterfaces.MessageSentEvent, error) {
	decodedMsg, err := protocol.DecodeMessage(event.EncodedMessage)
	if err != nil {
		return cciptestinterfaces.MessageSentEvent{}, fmt.Errorf("decode message: %w", err)
	}

	receiptIssuers := make([]protocol.UnknownAddress, len(event.Receipts))
	for i, r := range event.Receipts {
		receiptIssuers[i] = protocol.UnknownAddress(r.Issuer.Bytes())
	}

	return cciptestinterfaces.MessageSentEvent{
		MessageID:      event.MessageId,
		Sender:         event.Sender.Bytes(),
		Message:        decodedMsg,
		ReceiptIssuers: receiptIssuers,
		VerifierBlobs:  event.VerifierBlobs,
	}, nil
}

// sendLookupTables fetches the lookup tables a resolved send asked for. They include the router's
// onramp lookup table, which keeps the versioned transaction within the 1232-byte limit.
func (c *Chain) sendLookupTables(ctx context.Context, resolved []solana.PublicKey) (map[solana.PublicKey]solana.PublicKeySlice, error) {
	lutMap := make(map[solana.PublicKey]solana.PublicKeySlice, len(resolved))
	for _, lut := range resolved {
		entries, err := solcommon.GetAddressLookupTable(ctx, c.chain.Client, lut)
		if err != nil {
			return nil, fmt.Errorf("fetch lookup table %s: %w", lut.String(), err)
		}
		lutMap[lut] = entries
	}
	return lutMap, nil
}

// approveFeeBillingSigner delegates a token amount from the sender's associated token account
// to the router's fee billing signer PDA, the authority the onramp transfers with.
// tokenProgram must be the mint's owner program (classic SPL or Token-2022): the sender's
// ATA lives under that program and the approve CPI must target it.
func approveFeeBillingSigner(routerProgramID, owner solana.PublicKey, tokenAmount ccip_router.SvmTokenAmount, decimals uint8, tokenProgram solana.PublicKey) (solana.Instruction, error) {
	feeBillingSigner, _, err := solstate.FindFeeBillingSignerPDA(routerProgramID)
	if err != nil {
		return nil, fmt.Errorf("derive fee billing signer PDA: %w", err)
	}
	ownerTokenAccount, _, err := soltokens.FindAssociatedTokenAddress(tokenProgram, tokenAmount.Token, owner)
	if err != nil {
		return nil, fmt.Errorf("derive sender token account for mint %s: %w", tokenAmount.Token, err)
	}
	approveIx, err := soltokens.TokenApproveChecked(
		tokenAmount.Amount,
		decimals,
		tokenProgram,
		ownerTokenAccount,
		tokenAmount.Token,
		feeBillingSigner,
		owner,
		solana.PublicKeySlice{},
	)
	if err != nil {
		return nil, fmt.Errorf("build approve instruction for mint %s: %w", tokenAmount.Token, err)
	}
	return approveIx, nil
}

func (c *Chain) GetExpectedNextSequenceNumber(ctx context.Context, to uint64) (uint64, error) {
	routerProgramID, err := routerPubkeyFromDataStore(c.e.DataStore, c.chainDetails.ChainSelector, "")
	if err != nil {
		return 0, err
	}
	pda, err := solstate.FindDestChainStatePDAV2(to, routerProgramID)
	if err != nil {
		return 0, fmt.Errorf("find dest chain state PDA: %w", err)
	}
	var destChain ccip_router.DestChainCcipV2
	if err = solcommon.GetAccountDataBorshIntoAnchor(ctx, c.chain.Client, pda, rpc.CommitmentConfirmed, &destChain); err != nil {
		return 0, fmt.Errorf("get dest chain state account: %w", err)
	}
	return destChain.State.MessageNumber + 1, nil
}

func (c *Chain) BuildV3ExtraArgs(
	opts cciptestinterfaces.MessageOptions,
	destChain cciptestinterfaces.MessageV3Destination,
	executorArgsParams any,
	tokenReceiverParam any,
	tokenArgsParams any,
) (cciptestinterfaces.GenericExtraArgs, error) {
	execArgs, err := destChain.GetExecutorArgs(executorArgsParams)
	if err != nil {
		return nil, fmt.Errorf("failed to get executor args: %w", err)
	}
	tokenArgs, err := destChain.GetTokenArgs(tokenArgsParams)
	if err != nil {
		return nil, fmt.Errorf("failed to get token args: %w", err)
	}
	tokenReceiver, err := destChain.GetTokenReceiver(tokenReceiverParam)
	if err != nil {
		return nil, fmt.Errorf("failed to get token receiver: %w", err)
	}
	return SerializeMessageV3ExtraArgs(cciptestinterfaces.MessageOptions{
		FinalityConfig:      opts.FinalityConfig,
		ExecutionGasLimit:   opts.ExecutionGasLimit,
		Executor:            opts.Executor,
		ExecutorArgs:        execArgs,
		TokenReceiver:       tokenReceiver,
		TokenArgs:           tokenArgs,
		CCVs:                opts.CCVs,
		OutOfOrderExecution: opts.OutOfOrderExecution,
	})
}

// SerializeMessageV3ExtraArgs maps MessageOptions into a Solana GenericExtraArgsV3
// (converting CCV/executor addresses to pubkeys and splitting finality into flags/block
// depth) and returns the borsh-encoded, tag-prefixed bytes for the message's ExtraArgs.
func SerializeMessageV3ExtraArgs(opts cciptestinterfaces.MessageOptions) ([]byte, error) {
	ccvs := make([]solana.PublicKey, 0, len(opts.CCVs))
	ccvArgs := make([][]byte, 0, len(opts.CCVs))
	for _, ccv := range opts.CCVs {
		if len(ccv.CCVAddress) != solana.PublicKeyLength {
			return nil, fmt.Errorf("invalid CCV address length, expected %d, got %d", solana.PublicKeyLength, len(ccv.CCVAddress))
		}
		ccvs = append(ccvs, solana.PublicKeyFromBytes(ccv.CCVAddress))
		args := ccv.Args
		if args == nil {
			args = []byte{}
		}
		ccvArgs = append(ccvArgs, args)
	}

	if len(ccvs) != len(ccvArgs) {
		return nil, fmt.Errorf("mismatched CCV addresses and args lengths: ccvs len: %d vs. ccvArgs len: %d", len(ccvs), len(ccvArgs))
	}

	executor := solana.PublicKeyFromBytes(opts.Executor)
	if len(opts.Executor) == 0 {
		executor = solana.PublicKey{}
	} else if len(opts.Executor) != solana.PublicKeyLength {
		return nil, fmt.Errorf("invalid executor address length, expected %d, got %d", solana.PublicKeyLength, len(opts.Executor))
	}

	f := uint32(opts.FinalityConfig)
	v3Args := ccip_common.GenericExtraArgsV3{
		GasLimit: ccip_common.CrossChainGas{V0: opts.ExecutionGasLimit},
		Finality: ccip_common.FinalityConfig{
			Flags:      uint16(f >> 16),
			BlockDepth: uint16(f & 0xFFFF),
		},
		Ccvs:          ccvs,
		CcvArgs:       ccvArgs,
		Executor:      executor,
		ExecutorArgs:  opts.ExecutorArgs,
		TokenReceiver: opts.TokenReceiver,
		TokenArgs:     opts.TokenArgs,
	}
	return ccip.SerializeExtraArgs(v3Args, ccip.GenericExtraArgsV3Tag)
}

func (c *Chain) GetMaxDataBytes(_ context.Context, remoteChainSelector uint64) (uint32, error) {
	if remoteChainSelector != c.ChainSelector() {
		return 0, fmt.Errorf("GetMaxDataBytes: chain %d not found in environment chains %v", remoteChainSelector, c.ChainSelector())
	}
	// Hardcoded known-good ceiling. The offramp and the test receiver now request a 256KB heap via
	// the custom bump allocator, so buffered payloads are no longer bounded by the default 32KB
	// heap; the compute budget is the next ceiling.
	// TODO(NONEVM-2544): read the real per-lane limit from the fee quoter dest chain config instead
	// (DestChainConfigV2.max_data_bytes, defaulted to 30_000 in
	// deployment/v2_0_0/sequences/adapter.go) once that value is confirmed executable end-to-end.
	return 8192, nil
}

func (c *Chain) ManuallyExecuteMessage(_ context.Context, _ protocol.Message, _ uint64, _ []protocol.UnknownAddress, _ [][]byte) (cciptestinterfaces.ExecutionStateChangedEvent, error) {
	return cciptestinterfaces.ExecutionStateChangedEvent{}, errors.New("ManuallyExecuteMessage not yet implemented for Solana")
}

// DefaultCCVAndExecutorPubKeys resolves the default committee verifier and executor pubkeys from the datastore.
func (c *Chain) DefaultCCVAndExecutorPubKeys() (ccv solana.PublicKey, executor solana.PublicKey, err error) {
	ccv, err = committeeVerifierPubkeyFromDataStore(c.e.DataStore, c.chainDetails.ChainSelector, devenvcommon.DefaultCommitteeVerifierQualifier)
	if err != nil {
		return solana.PublicKey{}, solana.PublicKey{}, err
	}
	executor, err = executorPubkeyFromDataStore(c.e.DataStore, c.chainDetails.ChainSelector, devenvcommon.DefaultExecutorQualifier)
	if err != nil {
		return solana.PublicKey{}, solana.PublicKey{}, err
	}
	return ccv, executor, nil
}

func routerPubkeyFromDataStore(ds datastore.DataStore, chainSelector uint64, qualifier string) (solana.PublicKey, error) {
	pk, err := deploysolcommon.GetDataStoreAddress(ds, datastore.NewAddressRefKey(
		chainSelector,
		datastore.ContractType(routerops.ContractType),
		routerops.Version,
		qualifier,
	))
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("router ref not found for chain %d qualifier %q: %w", chainSelector, qualifier, err)
	}
	return pk, nil
}

func offrampPubkeyFromDataStore(ds datastore.DataStore, chainSelector uint64, qualifier string) (solana.PublicKey, error) {
	pk, err := deploysolcommon.GetDataStoreAddress(ds, datastore.NewAddressRefKey(
		chainSelector,
		datastore.ContractType(offrampops.ContractType),
		offrampops.Version,
		qualifier,
	))
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("offramp ref not found for chain %d qualifier %q: %w", chainSelector, qualifier, err)
	}
	return pk, nil
}

func committeeVerifierPubkeyFromDataStore(ds datastore.DataStore, chainSelector uint64, qualifier string) (solana.PublicKey, error) {
	pk, err := deploysolcommon.GetDataStoreAddress(ds, datastore.NewAddressRefKey(
		chainSelector,
		datastore.ContractType(cvops.ContractType),
		cvops.Version,
		qualifier,
	))
	if err != nil && qualifier != "" {
		pk, err = deploysolcommon.GetDataStoreAddress(ds, datastore.NewAddressRefKey(
			chainSelector,
			datastore.ContractType(cvops.ContractType),
			cvops.Version,
			"",
		))
	}
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("committee verifier ref not found for chain %d qualifier %q: %w", chainSelector, qualifier, err)
	}
	return pk, nil
}

func executorPubkeyFromDataStore(ds datastore.DataStore, chainSelector uint64, qualifier string) (solana.PublicKey, error) {
	pk, err := deploysolcommon.GetDataStoreAddress(ds, datastore.NewAddressRefKey(
		chainSelector,
		executorops.ProxyContractType,
		executorops.Version,
		qualifier,
	))
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("executor ref not found for chain %d qualifier %q: %w", chainSelector, qualifier, err)
	}
	return pk, nil
}
