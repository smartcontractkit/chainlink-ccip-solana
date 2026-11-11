package sourcereader

import (
	"bytes"
	"math/big"
	"testing"

	binary "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-ccv/protocol"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/ccip_router"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/tokens"
)

// These tests intentionally cover a minimal, high-confidence surface.
// The parser has semantic gaps vs the EVM source reader (empty-receipts,
// zip asymmetries, etc.) that should be resolved at the parser level,
// not pinned by assertions here. Add cases only for behavior the parser
// is contractually obligated to get right — and that a reader can verify
// without guessing whether the current behavior is correct.

// buildEncodedMessage returns a valid protocol-encoded message the parser's
// inner protocol.DecodeMessage call can round-trip.
func buildEncodedMessage(t *testing.T) []byte {
	t.Helper()
	msg, err := protocol.NewMessage(
		protocol.ChainSelector(1),      // source
		protocol.ChainSelector(2),      // dest
		protocol.SequenceNumber(42),    // sequence
		bytes.Repeat([]byte{0xA1}, 32), // onRamp
		bytes.Repeat([]byte{0xA2}, 32), // offRamp
		protocol.FinalityWaitForFinality,
		uint32(100000),                 // executionGasLimit
		uint32(50000),                  // ccipReceiveGasLimit
		protocol.Bytes32{0x99},         // ccvAndExecutorHash
		bytes.Repeat([]byte{0xB1}, 32), // sender
		bytes.Repeat([]byte{0xB2}, 32), // receiver
		[]byte{},                       // destBlob
		[]byte{0xDA, 0x7A},             // data
		nil,                            // tokenTransfer
	)
	require.NoError(t, err)

	raw, err := msg.Encode()
	require.NoError(t, err)
	return raw
}

// encodeEvent prefixes the Borsh-marshaled event body with the Anchor event
// discriminator, producing the byte payload that ParseEvent_CcipMessageSentV2
// expects.
func encodeEvent(t *testing.T, evt ccip_router.CcipMessageSentV2) []byte {
	t.Helper()
	buf := new(bytes.Buffer)
	buf.Write(ccip_router.Event_CcipMessageSentV2[:])
	err := evt.MarshalWithEncoder(binary.NewBorshEncoder(buf))
	require.NoError(t, err)
	return buf.Bytes()
}

func TestDefaultMessageSentEventParser(t *testing.T) {
	t.Parallel()
	validEncodedMsg := buildEncodedMessage(t)
	blob := []byte{0xB1, 0xB1}

	// Canonical CCIP receipt layout: [CCV_0, executor, network_fee].
	// One CCV receipt (with blob) + the two trailing fee receipts is the
	// minimum happy-path shape that satisfies the parser's structural
	// invariants.
	ccvReceipt := ccip_router.Receipt{
		Issuer:            solana.PublicKey{0x33},
		DestGasLimit:      ccip_router.CrossChainGas{V0: 111},
		DestBytesOverhead: 222,
		FeeTokenAmount:    1_000,
		ExtraArgs:         []byte{0xEA, 0x01},
	}
	executorReceipt := ccip_router.Receipt{
		Issuer:       solana.PublicKey{0x44},
		DestGasLimit: ccip_router.CrossChainGas{V0: 222},
	}
	networkFeeReceipt := ccip_router.Receipt{
		Issuer:       solana.PublicKey{0x55},
		DestGasLimit: ccip_router.CrossChainGas{V0: 333},
	}
	base := ccip_router.CcipMessageSentV2{
		DestChainSelector:              2,
		Sender:                         solana.PublicKey{0x11},
		MessageId:                      [32]uint8{0xCA, 0xFE},
		FeeToken:                       solana.PublicKey{0x22},
		TokenAmountBeforeTokenPoolFees: ccip_router.ProtocolAmount{BeBytes: tokens.ToBigEndianU256(0)},
		EncodedMessage:                 validEncodedMsg,
		Receipts:                       []ccip_router.Receipt{ccvReceipt, executorReceipt, networkFeeReceipt},
		VerifierBlobs:                  [][]byte{blob},
	}

	tests := []struct {
		name  string
		build func(t *testing.T) []byte
		// wantSkip expects (_, false, nil): not a message event. The source reader
		// skips it and continues the range.
		wantSkip bool
		// wantErr expects (_, true, err): a message event that failed to decode.
		// The source reader must hold the cursor, not drop the event.
		wantErr string
		assert  func(t *testing.T, got protocol.MessageSentEvent)
	}{
		{
			// Core contract: the transformation produces the fields the caller
			// cares about, and blobs line up with the CCV-receipt prefix.
			name:  "happy path: 1 CCV + executor + network_fee with one blob",
			build: func(t *testing.T) []byte { return encodeEvent(t, base) },
			assert: func(t *testing.T, got protocol.MessageSentEvent) {
				require.Equal(t, protocol.Bytes32{0xCA, 0xFE}, got.MessageID)
				require.Equal(t, protocol.ChainSelector(1), got.Message.SourceChainSelector)
				require.Equal(t, protocol.ChainSelector(2), got.Message.DestChainSelector)
				require.Equal(t, protocol.SequenceNumber(42), got.Message.SequenceNumber)
				require.Equal(t, []byte{0xDA, 0x7A}, []byte(got.Message.Data))
				require.Len(t, got.Receipts, 3)
				require.Equal(t, uint64(111), got.Receipts[0].DestGasLimit)
				require.Equal(t, protocol.ByteSlice(blob), got.Receipts[0].Blob)
				require.Equal(t, new(big.Int).SetUint64(1000), got.Receipts[0].FeeTokenAmount)
				// Trailing executor + network_fee receipts have no blob.
				require.Empty(t, got.Receipts[1].Blob)
				require.Empty(t, got.Receipts[2].Blob)
			},
		},
		{
			// Not an error: the logpoller filter is program-scoped, so every router
			// emit_cpi! event arrives here. The parser filters the events of other types.
			name: "unknown discriminator is filtered, not failed",
			build: func(t *testing.T) []byte {
				raw := encodeEvent(t, base)
				raw[0] ^= 0xFF
				return raw
			},
			wantSkip: true,
		},
		{
			// register_receiver is self-service: any third party can emit it through
			// the same filter. A failure here would stall every lane permanently.
			name: "real sibling router event (ReceiverRegistered) is filtered",
			build: func(t *testing.T) []byte {
				buf := new(bytes.Buffer)
				buf.Write(ccip_router.Event_ReceiverRegistered[:])
				buf.Write(bytes.Repeat([]byte{0xAB}, 40))
				return buf.Bytes()
			},
			wantSkip: true,
		},
		{
			name:     "empty payload is filtered without panicking",
			build:    func(t *testing.T) []byte { return nil },
			wantSkip: true,
		},
		{
			// logpoller Event.Data has no minimum-length guarantee. Classify a
			// sub-discriminator buffer; do not crash.
			name:     "payload shorter than the discriminator is filtered without panicking",
			build:    func(t *testing.T) []byte { return ccip_router.Event_CcipMessageSentV2[:7] },
			wantSkip: true,
		},
		{
			// Correct event type with an unreadable body: the binding/program
			// skew case that must not be dropped.
			name: "correct discriminator with garbage body is rejected",
			build: func(t *testing.T) []byte {
				buf := new(bytes.Buffer)
				buf.Write(ccip_router.Event_CcipMessageSentV2[:])
				buf.Write([]byte{0xFF, 0xFF, 0xFF})
				return buf.Bytes()
			},
			wantErr: "CcipMessageSentV2",
		},
		{
			name: "malformed EncodedMessage is rejected",
			build: func(t *testing.T) []byte {
				broken := base
				broken.EncodedMessage = []byte{0x00, 0x01, 0x02}
				return encodeEvent(t, broken)
			},
			wantErr: "decode encoded message",
		},
		{
			name: "fewer than 2 receipts is rejected",
			build: func(t *testing.T) []byte {
				tooFew := base
				tooFew.Receipts = []ccip_router.Receipt{executorReceipt}
				tooFew.VerifierBlobs = nil
				return encodeEvent(t, tooFew)
			},
			wantErr: "insufficient receipts",
		},
		{
			name: "zero-issuer receipt is rejected",
			build: func(t *testing.T) []byte {
				bad := base
				bad.Receipts = []ccip_router.Receipt{
					ccvReceipt,
					{ /* zero issuer */ },
					networkFeeReceipt,
				}
				return encodeEvent(t, bad)
			},
			wantErr: "zero issuer",
		},
		{
			// Matches devenv/impl.go's parallel-blob encoding: one blob slot
			// per receipt, trailing slots empty. The parser must accept this
			// shape because the E2E test uses it.
			name: "parallel blob encoding (one slot per receipt) is accepted",
			build: func(t *testing.T) []byte {
				parallel := base
				parallel.VerifierBlobs = [][]byte{blob, {}, {}}
				return encodeEvent(t, parallel)
			},
			assert: func(t *testing.T, got protocol.MessageSentEvent) {
				require.Len(t, got.Receipts, 3)
				require.Equal(t, protocol.ByteSlice(blob), got.Receipts[0].Blob)
				require.Empty(t, got.Receipts[1].Blob)
				require.Empty(t, got.Receipts[2].Blob)
			},
		},
		{
			name: "more verifier blobs than receipts is rejected",
			build: func(t *testing.T) []byte {
				tooMany := base
				tooMany.VerifierBlobs = [][]byte{blob, blob, blob, blob}
				return encodeEvent(t, tooMany)
			},
			wantErr: "verifier blob count",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, isMessageEvent, err := DefaultMessageSentEventParser(tc.build(t))
			if tc.wantSkip {
				require.False(t, isMessageEvent)
				require.NoError(t, err, "a non-message event must be filtered, not failed")
				return
			}
			if tc.wantErr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.wantErr)
				// The bool makes the source reader hold the cursor instead of
				// skipping. Assert it on every failure case.
				require.True(t, isMessageEvent,
					"a CcipMessageSentV2 that fails to decode must still report as a message event")
				return
			}
			require.NoError(t, err)
			require.True(t, isMessageEvent)
			tc.assert(t, got)
		})
	}
}
