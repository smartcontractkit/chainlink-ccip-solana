package changesets

import (
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"

	testreceiverops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/testreceiver"
)

func TestSenderApprovalsValidate(t *testing.T) {
	t.Parallel()

	receiver := solana.MustPublicKeyFromBase58("22441JJX6T5G5TiPGjfkFPEuUqdtfbMD77zSuzf8x5Di")
	valid := "0x54081602645704EE2B76FEe30E8B4d4F2D82d4E0"

	for _, version := range []*senderApprovals{
		{Receiver: receiver, SourceChainSelector: 16015286601757825753, Senders: []string{valid}, ReceiverVersion: testreceiverops.LegacyVersion},
		{Receiver: receiver, SourceChainSelector: 16015286601757825753, Senders: []string{valid}, ReceiverVersion: testreceiverops.Version},
	} {
		require.NoError(t, version.validate())

		bad := *version
		bad.Senders = []string{"not-hex"}
		require.Error(t, bad.validate())
	}
}
