// Goroutine leak tests use go.uber.org/goleak (via chainlink-common/pkg/utils/tests.VerifyNoLeaks).
//
// Do NOT add t.Parallel() to any test in this file. goleak checks goroutines for the
// currently running test; t.Parallel() in the same package can cause false positives
// (other tests' goroutines) or false negatives (timing). See goleak.VerifyNone docs.
package transmitter

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/smartcontractkit/chainlink-ccv/protocol"
	"github.com/smartcontractkit/chainlink-common/pkg/config"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	commontests "github.com/smartcontractkit/chainlink-common/pkg/utils/tests"
	solconfig "github.com/smartcontractkit/chainlink-solana/pkg/solana/config"

	"github.com/smartcontractkit/chainlink-ccip-solana/pkg/solanarpc"
)

// newLeakCheckTransmitter constructs an RPC client and transmitter against an
// unreachable RPC URL. Both start background goroutines that the test must close.
func newLeakCheckTransmitter(t *testing.T) *SolanaContractTransmitter {
	t.Helper()
	rpcURL := "http://127.0.0.1:9"
	name := "primary"
	rpcClient, err := solanarpc.NewClient(t.Context(), solanarpc.Config{
		ChainID: "localnet",
		Nodes: []*solconfig.Node{{
			Name: &name,
			URL:  config.MustParseURL(rpcURL),
		}},
		RPCTimeout: time.Second,
	}, logger.Nop())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, rpcClient.Close()) })

	tr, err := NewSolanaContractTransmitter(t.Context(), Params{
		Lggr:          logger.Nop(),
		ChainSelector: protocol.ChainSelector(1),
		ChainID:       "localnet",
		RPC:           rpcClient,
		Keystore:      newTestTransmitterKeystore(t),
		KeyName:       DefaultKeyName,
	})
	require.NoError(t, err)
	return tr
}

// TestGoleak_detectsUnclosedTransmitterRuntime is a sanity check that goleak.Find
// reports extra goroutines while the client and transmitter runtime is open.
func TestGoleak_detectsUnclosedTransmitterRuntime(t *testing.T) {
	// Do not add t.Parallel(): see file comment.
	baseline := goleak.IgnoreCurrent()

	tr := newLeakCheckTransmitter(t)
	t.Cleanup(func() { require.NoError(t, tr.Close()) })

	err := goleak.Find(baseline)
	require.Error(t, err, "goleak should detect goroutines left by the open transmitter runtime")
	require.Contains(t, err.Error(), "found unexpected goroutines")
}

// TestVerifyNoLeaks_passesWithClosedTransmitter asserts tests.VerifyNoLeaks passes
// when the transmitter is shut down before the test ends (success path).
//
// VerifyNoLeaks is registered first so that, with t.Cleanup running LIFO, Close
// executes before the goleak assertion.
func TestVerifyNoLeaks_passesWithClosedTransmitter(t *testing.T) {
	// Do not add t.Parallel(): see file comment.
	commontests.VerifyNoLeaks(t)

	tr := newLeakCheckTransmitter(t)

	t.Cleanup(func() { require.NoError(t, tr.Close()) })
}

// TestClose_idempotent asserts Close can be called multiple times without panicking
// (sync.Once guard) and returns the same error on each call.
func TestClose_idempotent(t *testing.T) {
	// Do not add t.Parallel(): see file comment.
	tr := newLeakCheckTransmitter(t)

	err1 := tr.Close()
	err2 := tr.Close()
	require.Equal(t, err1, err2, "Close should be idempotent")
}

// TestConvertAndWriteMessageToChain_rejectsAfterClose asserts that no new submission
// is accepted once Close has been called.
func TestConvertAndWriteMessageToChain_rejectsAfterClose(t *testing.T) {
	// Do not add t.Parallel(): see file comment.
	tr := newLeakCheckTransmitter(t)
	require.NoError(t, tr.Close())

	err := tr.ConvertAndWriteMessageToChain(t.Context(), protocol.AbstractAggregatedReport{})
	require.ErrorIs(t, err, ErrTransmitterClosed)
}
