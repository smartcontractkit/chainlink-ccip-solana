package sourcereader

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-ccv/protocol"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-testing-framework/framework/components/blockchain"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/ccip_router"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/test_program"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/testutils"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	"github.com/smartcontractkit/chainlink-ccip-solana/pkg/logpoller"
	"github.com/smartcontractkit/chainlink-ccip-solana/pkg/sourcereader"
)

// startLocalnetWithProgramStub spins up a local Solana validator with the test_program
// loaded via CTF's --upgradeable-program flag.
func startLocalnetWithProgramStub(ctx context.Context, t *testing.T) *rpc.Client {
	t.Helper()

	net, err := testutils.NewLocalValidator(ctx, &blockchain.Input{
		ContractsDir: filepath.Join("..", "..", "contracts", "target", "deploy"),
		PublicKey:    blockchain.DefaultSolanaPublicKey,
		SolanaPrograms: map[string]string{
			"test_program": test_program.ProgramID.String(),
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		if net.Container != nil {
			_ = net.Container.Terminate(ctx)
		}
	})

	rpcURL := net.Nodes[0].ExternalHTTPUrl
	t.Logf("Solana validator: rpc=%s", rpcURL)

	return rpc.New(rpcURL)
}

type sourceReaderRPC struct {
	*rpc.Client
}

func (c sourceReaderRPC) GetAccountInfo(
	ctx context.Context,
	addr solana.PublicKey,
	commitment rpc.CommitmentType,
) (*rpc.GetAccountInfoResult, error) {
	return c.GetAccountInfoWithOpts(ctx, addr, &rpc.GetAccountInfoOpts{
		Commitment: commitment,
		Encoding:   solana.EncodingBase64,
	})
}

// sendCcipSendV2 sends a CcipSendV2 transaction with a properly encoded protocol.Message.
func sendCcipSendV2(ctx context.Context, t *testing.T, rpcClient *rpc.Client, privateKey solana.PrivateKey) (*ccip_router.CcipMessageSentV2, *protocol.Message) {
	t.Helper()

	eventAuthority, _, err := state.FindEventAuthorityPDA(test_program.ProgramID)
	require.NoError(t, err)

	ix, err := test_program.NewEmitCcipMessageSentV2Instruction(
		privateKey.PublicKey(),
		eventAuthority,
		test_program.ProgramID,
	)
	require.NoError(t, err)

	result, err := common.SendAndConfirm(ctx, rpcClient,
		[]solana.Instruction{ix}, privateKey, rpc.CommitmentConfirmed)
	require.NoError(t, err)
	require.NotNil(t, result)
	t.Logf("Transaction confirmed in slot %d", result.Slot)

	tx, err := result.Transaction.GetTransaction()
	require.NoError(t, err)

	event, err := common.ExtractCPIEvent(tx, result.Meta, test_program.ProgramID, ccip_router.ParseEvent_CcipMessageSentV2)
	require.NoError(t, err)

	msg, err := protocol.DecodeMessage(event.EncodedMessage)
	require.NoError(t, err)

	return event, msg
}

// TestSourceReaderWithLogPoller exercises the full stack:
// localnet → ccip_send_v2 (with real encoded message) → logpoller → SolanaSourceReader.FetchMessageSentEvents
func TestSourceReaderWithLogPoller(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	rpcClient := startLocalnetWithProgramStub(ctx, t)

	privateKey, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)
	require.NoError(t, testutils.FundAccounts(ctx, []solana.PrivateKey{privateKey}, rpcClient))

	// Build a valid protocol.Message and encode it for the on-chain event
	testMessageEvent, sentMsg := sendCcipSendV2(ctx, t, rpcClient, privateKey)

	currentSlot, err := rpcClient.GetSlot(ctx, rpc.CommitmentConfirmed)
	require.NoError(t, err)

	lggr := logger.TestSugared(t)

	lp, err := logpoller.NewService(rpcClient, test_program.ProgramID, "test-chain", lggr, logpoller.DefaultConfig())
	require.NoError(t, err)
	require.NoError(t, lp.Start(ctx))
	t.Cleanup(func() { require.NoError(t, lp.Close()) })

	t.Run("source_reader_end_to_end", func(t *testing.T) {
		rmnRemote := solana.NewWallet().PublicKey()

		sr, err := sourcereader.NewSolanaSourceReader(
			test_program.ProgramID,
			rmnRemote,
			12463857294658392847, // solana test selector
			sourceReaderRPC{Client: rpcClient},
			lp,
			lggr,
			sourcereader.DefaultMessageSentEventParser,
		)
		require.NoError(t, err)

		events, err := sr.FetchMessageSentEvents(ctx, 0, currentSlot)
		require.NoError(t, err)
		require.NotEmpty(t, events, "FetchMessageSentEvents should return at least one event")

		evt := events[0]
		require.Equal(t, testMessageEvent.MessageId, [32]byte(evt.MessageID))
		require.Equal(t, testMessageEvent.DestChainSelector, uint64(evt.Message.DestChainSelector))
		require.Equal(t, uint64(sentMsg.SourceChainSelector), uint64(evt.Message.SourceChainSelector))
		require.Equal(t, sentMsg.Data, evt.Message.Data)
		require.Equal(t, uint64(sentMsg.SequenceNumber), uint64(evt.Message.SequenceNumber))

		t.Logf("End-to-end: messageId=%x src=%d dst=%d seq=%d",
			evt.MessageID, evt.Message.SourceChainSelector, evt.Message.DestChainSelector, evt.Message.SequenceNumber)
	})
}
