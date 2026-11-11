package sequences

import (
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	testreceiverops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/testreceiver"
)

const (
	sepoliaSelector = uint64(16015286601757825753)
	evmSender       = "0x54081602645704EE2B76FEe30E8B4d4F2D82d4E0"
)

var testReceiver = solana.MustPublicKeyFromBase58("22441JJX6T5G5TiPGjfkFPEuUqdtfbMD77zSuzf8x5Di")

func TestApprovedSenderPDAWithPadding(t *testing.T) {
	t.Parallel()

	padded, err := ApprovedSenderPDA(testReceiver, sepoliaSelector, evmSender, testreceiverops.LegacyVersion)
	require.NoError(t, err)
	require.Equal(t, solana.MustPublicKeyFromBase58("8s45w5VUcHoHUPJP9xijXPiA18ozt8rUVDXY3EQhDHGn"), padded)

	unpadded, err := ApprovedSenderPDAWithPadding(testReceiver, sepoliaSelector, evmSender, testreceiverops.LegacyVersion, true)
	require.NoError(t, err)
	require.Equal(t, solana.MustPublicKeyFromBase58("3daDngNP8wFWES87CrisY3Vg7HAEu2gfC4S25wSBmAgA"), unpadded)
}

func TestSenderApprovals(t *testing.T) {
	t.Parallel()

	t.Run("legacy receiver approves the padded and the unpadded sender", func(t *testing.T) {
		t.Parallel()
		approvals, err := SenderApprovals(testReceiver, sepoliaSelector, evmSender, testreceiverops.LegacyVersion)
		require.NoError(t, err)
		require.Len(t, approvals, 2)
		require.False(t, approvals[0].Unpadded)
		require.True(t, approvals[1].Unpadded)
		require.NotEqual(t, approvals[0].PDA, approvals[1].PDA)
	})

	t.Run("v2 receiver approves only the padded sender", func(t *testing.T) {
		t.Parallel()
		approvals, err := SenderApprovals(testReceiver, sepoliaSelector, evmSender, testreceiverops.Version)
		require.NoError(t, err)
		require.Len(t, approvals, 1)
		require.False(t, approvals[0].Unpadded)
	})

	t.Run("a sender that is already 32 bytes yields a single approval", func(t *testing.T) {
		t.Parallel()
		approvals, err := SenderApprovals(testReceiver, sepoliaSelector, "0x000000000000000000000000"+evmSender[2:], testreceiverops.LegacyVersion)
		require.NoError(t, err)
		require.Len(t, approvals, 1)
	})
}

func TestDecodeRemoteSenderUnpadded(t *testing.T) {
	t.Parallel()

	raw, err := solcommon.DecodeRemoteSenderUnpadded(evmSender)
	require.NoError(t, err)
	require.Len(t, raw, 20)

	padded, err := solcommon.DecodeRemoteSender(evmSender)
	require.NoError(t, err)
	require.Len(t, padded, 32)
	require.Equal(t, raw, padded[12:])

	_, err = solcommon.DecodeRemoteSenderUnpadded("0x")
	require.Error(t, err)
	_, err = solcommon.DecodeRemoteSenderUnpadded("zz")
	require.Error(t, err)
}
