package ccip

import (
	"encoding/hex"
	"testing"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/ccip_common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/ccip_router"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/tokens"
)

func TestMessageHashing(t *testing.T) {
	t.Parallel()

	sender := make([]byte, 32)
	copy(sender, []byte{1, 2, 3})
	tokenAmount := [32]uint8{}
	for i := range tokenAmount {
		tokenAmount[i] = 1
	}

	t.Run("SVMToAny", func(t *testing.T) {
		t.Parallel()

		extraArgs, err := SerializeExtraArgs(ccip_common.GenericExtraArgsV2{
			GasLimit:                 bin.Uint128{Lo: 1},
			AllowOutOfOrderExecution: true,
		}, GenericExtraArgsV2Tag)
		require.NoError(t, err)

		h, err := HashSVMToAnyMessage(ccip_router.Svm2AnyRampMessage{
			Header: ccip_router.RampMessageHeader{
				MessageId:           [32]uint8{},
				SourceChainSelector: 10,
				DestChainSelector:   20,
				SequenceNumber:      30,
				Nonce:               40,
			},
			Sender:         solana.MustPublicKeyFromBase58("DS2tt4BX7YwCw7yrDNwbAdnYrxjeCPeGJbHmZEYC8RTa"),
			Data:           []byte{4, 5, 6},
			Receiver:       sender,
			ExtraArgs:      extraArgs,
			FeeToken:       solana.MustPublicKeyFromBase58("DS2tt4BX7YwCw7yrDNwbAdnYrxjeCPeGJbHmZEYC8RTb"),
			FeeTokenAmount: ccip_router.CrossChainAmount{LeBytes: tokens.ToLittleEndianU256(50)},
			FeeValueJuels:  ccip_router.CrossChainAmount{LeBytes: tokens.ToLittleEndianU256(500)},
			TokenAmounts: []ccip_router.Svm2AnyTokenTransfer{
				{
					SourcePoolAddress: solana.MustPublicKeyFromBase58("DS2tt4BX7YwCw7yrDNwbAdnYrxjeCPeGJbHmZEYC8RTc"),
					DestTokenAddress:  []byte{0, 1, 2, 3},
					ExtraData:         []byte{4, 5, 6},
					Amount:            ccip_router.CrossChainAmount{LeBytes: tokenAmount},
					DestExecData:      []byte{4, 5, 6},
				},
			},
		})
		require.NoError(t, err)
		require.Equal(t, "46abd733e950b0b0e05b1b4b040cf2df6c38899af71aad058ff08bfa96d4b532", hex.EncodeToString(h))
	})
}
