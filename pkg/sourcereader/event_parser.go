package sourcereader

import (
	"bytes"
	"fmt"
	"math/big"

	"github.com/smartcontractkit/chainlink-ccv/protocol"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/ccip_router"
)

// MessageSentEventParser parses a raw CPI event payload into a chain-agnostic
// message event. Block metadata is attached by the source reader after parsing.
//
// The logpoller filter is scoped to the router program, not to one event type.
// Every emit_cpi! event from the router reaches this parser. The isMessageEvent
// return value separates a non-message event from an unreadable message event:
//
//	(_, false, nil)   not a message event: skip it and continue
//	(_, true, err)    a message event that failed to decode or convert: return the error
//	(evt, true, nil)  success
type MessageSentEventParser func(eventData []byte) (evt protocol.MessageSentEvent, isMessageEvent bool, err error)

// DefaultMessageSentEventParser is the production parser for onramp
// CCIPMessageSentV2 events.
func DefaultMessageSentEventParser(eventData []byte) (evt protocol.MessageSentEvent, isMessageEvent bool, err error) {
	// Check the Anchor discriminator before parsing. ParseEvent_CcipMessageSentV2
	// also rejects a mismatched discriminator, but with the same error return as a
	// corrupt payload. The prefix check is the only stable way to tell the two apart.
	if !bytes.HasPrefix(eventData, ccip_router.Event_CcipMessageSentV2[:]) {
		return protocol.MessageSentEvent{}, false, nil
	}

	parsed, err := ccip_router.ParseEvent_CcipMessageSentV2(eventData)
	if err != nil {
		return protocol.MessageSentEvent{}, true, err
	}

	msg, err := convertCcipMessageSentV2(parsed)
	return msg, true, err
}

// convertCcipMessageSentV2 converts a Solana CcipMessageSentV2 event into
// the chain-agnostic protocol.MessageSentEvent used by the CCV pipeline.
//
// Structural invariants enforced here (see protocol.ParseReceiptStructure
// for the canonical CCIP receipt layout):
//   - Receipts are ordered [CCV_0..CCV_N-1, (token)?, executor, network_fee].
//     A well-formed event therefore carries at least 2 receipts: executor
//     plus network fee.
//   - VerifierBlobs is a parallel list: index i corresponds to Receipts[i].
//     Trailing receipts that don't carry a blob (token / executor / network
//     fee) sit at empty slots. An event with more blobs than receipts cannot
//     be zipped and is malformed.
//   - Every receipt must have a non-zero issuer. A zero issuer indicates a
//     default-initialized (uninitialized) receipt slot.
//
// These are cheap fail-fast checks: they don't replace the downstream
// protocol.ValidateCCVAndExecutorHash pass, but they catch obvious garbage
// at the parse boundary so it doesn't propagate as opaque hash mismatches
// three layers deep.
func convertCcipMessageSentV2(evt *ccip_router.CcipMessageSentV2) (protocol.MessageSentEvent, error) {
	msg, err := protocol.DecodeMessage(evt.EncodedMessage)
	if err != nil {
		return protocol.MessageSentEvent{}, fmt.Errorf("decode encoded message: %w", err)
	}

	if len(evt.Receipts) < 2 {
		return protocol.MessageSentEvent{}, fmt.Errorf(
			"insufficient receipts: got %d, need at least 2 (executor + network fee)",
			len(evt.Receipts),
		)
	}

	if len(evt.VerifierBlobs) > len(evt.Receipts) {
		return protocol.MessageSentEvent{}, fmt.Errorf(
			"verifier blob count %d exceeds receipt count %d",
			len(evt.VerifierBlobs), len(evt.Receipts),
		)
	}

	for i, r := range evt.Receipts {
		if r.Issuer.IsZero() {
			return protocol.MessageSentEvent{}, fmt.Errorf("receipt[%d] has zero issuer", i)
		}
	}

	receipts := make([]protocol.ReceiptWithBlob, len(evt.Receipts))
	for i, r := range evt.Receipts {
		receipts[i] = protocol.ReceiptWithBlob{
			Issuer:            protocol.UnknownAddress(r.Issuer.Bytes()),
			ExtraArgs:         protocol.ByteSlice(r.ExtraArgs),
			DestGasLimit:      uint64(r.DestGasLimit.V0),
			DestBytesOverhead: r.DestBytesOverhead,
			FeeTokenAmount:    new(big.Int).SetUint64(r.FeeTokenAmount),
		}
	}

	// VerifierBlobs is parallel to Receipts: zipping by index is safe because
	// we've already enforced len(VerifierBlobs) <= len(Receipts).
	for i, blob := range evt.VerifierBlobs {
		receipts[i].Blob = protocol.ByteSlice(blob)
	}

	return protocol.MessageSentEvent{
		MessageID: protocol.Bytes32(evt.MessageId),
		Message:   *msg,
		Receipts:  receipts,
	}, nil
}
