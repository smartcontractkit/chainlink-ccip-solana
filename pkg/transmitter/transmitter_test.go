package transmitter

import (
	"context"
	"errors"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-ccv/executor"
	"github.com/smartcontractkit/chainlink-ccv/protocol"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	commontests "github.com/smartcontractkit/chainlink-common/pkg/utils/tests"
	soltxm "github.com/smartcontractkit/chainlink-solana/pkg/solana/txm"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/ccip_common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/ccip"
	"github.com/smartcontractkit/chainlink-ccip-solana/pkg/solanarpc"
)

func TestNewSolanaContractTransmitter_canceledContext(t *testing.T) {
	commontests.VerifyNoLeaks(t)
	ks := newTestTransmitterKeystore(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	tr, err := NewSolanaContractTransmitter(ctx, Params{
		Lggr:     logger.Nop(),
		ChainID:  "localnet",
		RPC:      &solanarpc.Client{},
		Keystore: ks,
		KeyName:  DefaultKeyName,
	})
	if tr != nil {
		t.Cleanup(func() { require.NoError(t, tr.Close()) })
	}
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, tr)
}

// TestConvertAndWriteMessageToChain_EncodeError covers the first failure branch:
// invalid canonical message encoding before any RPC or txm use.
func TestConvertAndWriteMessageToChain_EncodeError(t *testing.T) {
	// Do not add t.Parallel(): this test constructs a real transmitter (with txm
	// background goroutines) and could race goleak-based tests in this package.
	ctx := context.Background()
	tr := newLeakCheckTransmitter(t)
	t.Cleanup(func() { require.NoError(t, tr.Close()) })

	msg := protocol.Message{
		SourceChainSelector:  1,
		DestChainSelector:    2,
		SequenceNumber:       1,
		Version:              1,
		ExecutionGasLimit:    1, // non-zero so the message passes the zero-gas guard and reaches encoding
		SenderLength:         32,
		Sender:               make([]byte, 32),
		ReceiverLength:       32,
		Receiver:             make([]byte, 32),
		OffRampAddressLength: 32,
		OffRampAddress:       make([]byte, 32),
		OnRampAddressLength:  32,
		OnRampAddress:        make([]byte, 32),
		DataLength:           99,
		Data:                 []byte{1, 2, 3},
	}

	err := tr.ConvertAndWriteMessageToChain(ctx, protocol.AbstractAggregatedReport{Message: msg})
	require.Error(t, err)
	require.ErrorIs(t, err, executor.ErrMessageEncoding)
}

// TestConvertAndWriteMessageToChain_ZeroGasRejected covers the terminal rejection branch:
// a zero execution gas limit is refused before any RPC or txm use.
func TestConvertAndWriteMessageToChain_ZeroGasRejected(t *testing.T) {
	// Do not add t.Parallel(): this test constructs a real transmitter (with txm
	// background goroutines) and could race goleak-based tests in this package.
	ctx := context.Background()
	tr := newLeakCheckTransmitter(t)
	t.Cleanup(func() { require.NoError(t, tr.Close()) })

	msg := protocol.Message{
		SourceChainSelector:  1,
		DestChainSelector:    2,
		SequenceNumber:       1,
		Version:              1,
		ExecutionGasLimit:    0,
		CcipReceiveGasLimit:  100_000,
		SenderLength:         32,
		Sender:               make([]byte, 32),
		ReceiverLength:       32,
		Receiver:             make([]byte, 32),
		OffRampAddressLength: 32,
		OffRampAddress:       make([]byte, 32),
		OnRampAddressLength:  32,
		OnRampAddress:        make([]byte, 32),
		DataLength:           3,
		Data:                 []byte{1, 2, 3},
	}

	err := tr.ConvertAndWriteMessageToChain(ctx, protocol.AbstractAggregatedReport{Message: msg})
	require.Error(t, err)
	require.ErrorIs(t, err, executor.ErrMessageRejectedByTransmitter)
}

func TestIsOversizedTxError(t *testing.T) {
	t.Parallel()

	require.True(t, isOversizedTxError(errors.New(
		`resolve execute accounts: failed to simulate transaction: VersionedTransaction too large: 6408 bytes (max: 1644/1232)`,
	)))
	require.False(t, isOversizedTxError(errors.New("resolve execute accounts: simulation error: custom program error")))
	require.False(t, isOversizedTxError(nil))
}

func TestComputeUnitLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		executionGasLimit   uint32
		ccipReceiveGasLimit uint32
		want                uint32
	}{
		{
			name:                "receiver gas is ignored",
			executionGasLimit:   200_000,
			ccipReceiveGasLimit: 100_000,
			want:                200_000,
		},
		{
			name:                "low limit uses message value not default",
			executionGasLimit:   1_000,
			ccipReceiveGasLimit: 2_000,
			want:                1_000,
		},
		{
			name:                "zero yields zero (zero-gas messages are rejected upstream in ConvertAndWriteMessageToChain)",
			executionGasLimit:   0,
			ccipReceiveGasLimit: 0,
			want:                0,
		},
		{
			name:                "capped at Solana max",
			executionGasLimit:   1_500_000,
			ccipReceiveGasLimit: 500_000,
			want:                soltxm.MaxComputeUnitLimit,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			msg := protocol.Message{
				ExecutionGasLimit:   tt.executionGasLimit,
				CcipReceiveGasLimit: tt.ccipReceiveGasLimit,
			}
			require.Equal(t, tt.want, computeUnitLimit(msg))
		})
	}
}

// TestConvertAndWriteMessageToChain_InvalidDestBlobRejected covers a malformed dest blob large
// enough to fill the program log. It is refused before any RPC or txm use.
func TestConvertAndWriteMessageToChain_InvalidDestBlobRejected(t *testing.T) {
	// Do not add t.Parallel(): this test constructs a real transmitter (with txm
	// background goroutines) and could race goleak-based tests in this package.
	ctx := context.Background()
	tr := newLeakCheckTransmitter(t)
	t.Cleanup(func() { require.NoError(t, tr.Close()) })

	receiver := make([]byte, 32)
	receiver[0] = 1
	blob := make([]byte, 4096)
	msg := protocol.Message{
		SourceChainSelector:  1,
		DestChainSelector:    2,
		SequenceNumber:       1,
		Version:              1,
		ExecutionGasLimit:    1,
		CcipReceiveGasLimit:  1,
		SenderLength:         32,
		Sender:               make([]byte, 32),
		ReceiverLength:       32,
		Receiver:             receiver,
		OffRampAddressLength: 32,
		OffRampAddress:       make([]byte, 32),
		OnRampAddressLength:  32,
		OnRampAddress:        make([]byte, 32),
		DestBlobLength:       4096,
		DestBlob:             blob,
	}

	err := tr.ConvertAndWriteMessageToChain(ctx, protocol.AbstractAggregatedReport{Message: msg})
	require.ErrorIs(t, err, executor.ErrMessageRejectedByTransmitter)
	require.ErrorContains(t, err, "invalid dest blob")
}

func TestCheckDestBlob(t *testing.T) {
	t.Parallel()
	receiver := make([]byte, 32)
	receiver[0] = 1
	blob := func(n int) []byte {
		b, err := (&ccip.SVMExecutorArgs{SvmDestBlob: ccip_common.SvmDestBlob{Accounts: make([]solana.PublicKey, n)}}).BorshEncode()
		require.NoError(t, err)
		return b
	}
	bad := []byte{1}

	for name, tc := range map[string]struct {
		msg     protocol.Message
		wantErr string
	}{
		"data":                             {protocol.Message{Receiver: receiver, Data: []byte{1}, DestBlob: bad}, "invalid dest blob"},
		"receive gas":                      {protocol.Message{Receiver: receiver, CcipReceiveGasLimit: 1, DestBlob: bad}, "invalid dest blob"},
		"token transfer":                   {protocol.Message{TokenTransfer: &protocol.TokenTransfer{}, DestBlob: bad}, "invalid dest blob"},
		"no callback":                      {protocol.Message{Receiver: receiver, DestBlob: bad}, ""},
		"zero receiver":                    {protocol.Message{Receiver: make([]byte, 32), Data: []byte{1}, DestBlob: bad}, ""},
		"short receiver":                   {protocol.Message{Receiver: receiver[:20], Data: []byte{1}, DestBlob: blob(0)}, "receiver is 20 bytes"},
		"short receiver, no callback":      {protocol.Message{Receiver: receiver[:20], DestBlob: blob(0)}, "receiver is 20 bytes"},
		"short receiver, token transfer":   {protocol.Message{Receiver: receiver[:20], TokenTransfer: &protocol.TokenTransfer{}, DestBlob: blob(0)}, "receiver is 20 bytes"},
		"long receiver":                    {protocol.Message{Receiver: append(receiver, 0), DestBlob: blob(0)}, "receiver is 33 bytes"},
		"empty receiver":                   {protocol.Message{Data: []byte{1}, DestBlob: bad}, ""},
		"64 messaging accounts":            {protocol.Message{Receiver: receiver, Data: []byte{1}, DestBlob: blob(64)}, ""},
		"65 messaging accounts":            {protocol.Message{Receiver: receiver, Data: []byte{1}, DestBlob: blob(65)}, "65 messaging accounts"},
		"65 accounts, token transfer only": {protocol.Message{TokenTransfer: &protocol.TokenTransfer{}, DestBlob: blob(65)}, ""},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			err := checkDestBlob(tc.msg)
			if tc.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.wantErr)
			}
		})
	}
}
