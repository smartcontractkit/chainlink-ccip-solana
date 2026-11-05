// Goroutine leak tests use go.uber.org/goleak (via chainlink-common/pkg/utils/tests.VerifyNoLeaks).
//
// Do NOT add t.Parallel() to any test in this file. goleak checks goroutines for the
// currently running test; t.Parallel() in the same package can cause false positives
// (other tests' goroutines) or false negatives (timing). See goleak.VerifyNone docs.
package logpoller_test

import (
	"context"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	commontests "github.com/smartcontractkit/chainlink-common/pkg/utils/tests"

	"github.com/smartcontractkit/chainlink-ccip-solana/pkg/logpoller"
)

func newLeakCheckLogpoller(t *testing.T) logpoller.Service {
	t.Helper()
	lp, err := logpoller.NewService(
		rpc.New("http://127.0.0.1:9"),
		solana.PublicKey{0x01},
		"goleak-check",
		logger.Nop(),
		logpoller.DefaultConfig(),
	)
	require.NoError(t, err)
	require.NoError(t, lp.Start(context.Background()))
	// Ensure workers are running before leak assertions (avoids racing Start).
	require.NoError(t, lp.Ready())
	return lp
}

func closeLogpoller(t *testing.T, lp logpoller.Service) {
	t.Helper()
	require.NoError(t, lp.Close())
}

// TestGoleak_detectsUnclosedLogpollerService asserts goleak.Find reports extra goroutines
// while the service is still running without Close (failure-path detector sanity check).
func TestGoleak_detectsUnclosedLogpollerService(t *testing.T) {
	// Do not add t.Parallel(): see file comment.
	baseline := goleak.IgnoreCurrent()

	lp := newLeakCheckLogpoller(t)
	t.Cleanup(func() { closeLogpoller(t, lp) })

	require.NoError(t, lp.Ready(), "logpoller should still be running before leak check")

	err := goleak.Find(baseline)
	require.Error(t, err, "goleak should detect goroutines left by unclosed logpoller")
	require.Contains(t, err.Error(), "found unexpected goroutines")
}

// TestVerifyNoLeaks_passesWithClosedLogpollerService asserts tests.VerifyNoLeaks passes
// when the logpoller is shut down before the test ends (success path).
func TestVerifyNoLeaks_passesWithClosedLogpollerService(t *testing.T) {
	// Do not add t.Parallel(): see file comment.
	commontests.VerifyNoLeaks(t)

	lp := newLeakCheckLogpoller(t)

	// runs before VerifyNoLeaks cleanup to reduce flake from async worker exit
	t.Cleanup(func() { closeLogpoller(t, lp) })
}
