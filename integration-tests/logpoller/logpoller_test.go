// Package logpoller contains integration tests for pkg/logpoller against a real local
// Solana validator. This is shared infrastructure (used by source and destination readers),
// so it lives in its own suite—not under sourcereader or destreader.
package logpoller

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

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/ccip_router"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/test_program"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/testutils"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	"github.com/smartcontractkit/chainlink-ccip-solana/pkg/logpoller"
)

// startLocalnetWithProgramStub matches integration-tests/sourcereader: only test_program.so.
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
	t.Logf("Solana validator: rpc=%s", net.Nodes[0].ExternalHTTPUrl)
	return rpc.New(net.Nodes[0].ExternalHTTPUrl)
}

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
	tx, err := result.Transaction.GetTransaction()
	require.NoError(t, err)
	event, err := common.ExtractCPIEvent(tx, result.Meta, test_program.ProgramID, ccip_router.ParseEvent_CcipMessageSentV2)
	require.NoError(t, err)
	msg, err := protocol.DecodeMessage(event.EncodedMessage)
	require.NoError(t, err)
	return event, msg
}

// TestLogPoller_ccip_message_sent_v2_via_test_program emits CcipMessageSentV2 from test_program
// and asserts logpoller discovers and parses the CPI event (no product offramp/router deploy).
//
// Leak checks: pkg/logpoller/leak_test.go (not here — testcontainers pollute goleak).
func TestLogPoller_ccip_message_sent_v2_via_test_program(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	rpcClient := startLocalnetWithProgramStub(ctx, t)
	priv, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)
	require.NoError(t, testutils.FundAccounts(ctx, []solana.PrivateKey{priv}, rpcClient))

	testEvent, _ := sendCcipSendV2(ctx, t, rpcClient, priv)
	currentSlot, err := rpcClient.GetSlot(ctx, rpc.CommitmentConfirmed)
	require.NoError(t, err)

	lp, err := logpoller.NewService(rpcClient, test_program.ProgramID, "logpoller-integration", logger.TestSugared(t), logpoller.DefaultConfig())
	require.NoError(t, err)
	require.NoError(t, lp.Start(ctx))
	t.Cleanup(func() { require.NoError(t, lp.Close()) })
	require.NoError(t, lp.Ready())

	cpiEvents, err := lp.FetchEvents(ctx, 0, currentSlot)
	require.NoError(t, err)
	require.NotEmpty(t, cpiEvents, "logpoller should discover at least one CPI event")

	var found *ccip_router.CcipMessageSentV2
	for _, raw := range cpiEvents {
		parsed, parseErr := ccip_router.ParseEvent_CcipMessageSentV2(raw.Data)
		if parseErr != nil {
			continue
		}
		if parsed.MessageId == testEvent.MessageId {
			found = parsed
			break
		}
	}
	require.NotNil(t, found, "CcipMessageSentV2 not found for message ID %x", testEvent.MessageId)
	require.Equal(t, testEvent.EncodedMessage, found.EncodedMessage)
}
