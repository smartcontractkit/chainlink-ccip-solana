package loadtest

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-ccv/build/devenv/tests/e2e/load"
	"github.com/smartcontractkit/chainlink-testing-framework/wasp"
)

// fakeGun counts Call invocations and returns the configured outcome.
type fakeGun struct {
	calls atomic.Int64
	fail  bool
	delay time.Duration
}

func (g *fakeGun) Call(_ *wasp.Generator) *wasp.Response {
	g.calls.Add(1)
	if g.delay > 0 {
		time.Sleep(g.delay)
	}
	if g.fail {
		return &wasp.Response{Error: "boom", Failed: true}
	}
	return &wasp.Response{Data: "ok"}
}

func (g *fakeGun) SentMessages() <-chan load.SentMessage { return nil }
func (g *fakeGun) CloseSentChannel()                     {}

func TestIntervalDriverFiresOnCadence(t *testing.T) {
	t.Parallel()

	gun := &fakeGun{}
	// Ticks at 50ms boundaries only: 10 sends over 525ms.
	stats := runIntervalDriver(t, gun, 50*time.Millisecond, 525*time.Millisecond, time.Minute)

	require.InDelta(t, 10, gun.calls.Load(), 1)
	require.Equal(t, gun.calls.Load(), stats.TxsSubmitted)
	require.Zero(t, stats.TxsFailed)
	require.Zero(t, stats.CallTimeouts)
}

func TestIntervalDriverCountsFailures(t *testing.T) {
	t.Parallel()

	gun := &fakeGun{fail: true}
	stats := runIntervalDriver(t, gun, 50*time.Millisecond, 175*time.Millisecond, time.Minute)

	require.InDelta(t, 3, gun.calls.Load(), 1)
	require.Equal(t, gun.calls.Load(), stats.TxsFailed)
	require.Zero(t, stats.TxsSubmitted)
}

func TestIntervalDriverCountsSlowSendsPastDuration(t *testing.T) {
	t.Parallel()

	// Sends take 100ms but the timeout is 10ms: every send counts as timed out.
	// The driver awaits in-flight sends, so slow calls are still counted.
	gun := &fakeGun{delay: 100 * time.Millisecond}
	stats := runIntervalDriver(t, gun, 50*time.Millisecond, 175*time.Millisecond, 10*time.Millisecond)

	require.InDelta(t, 3, gun.calls.Load(), 1)
	require.Equal(t, gun.calls.Load(), stats.TxsSubmitted)
	require.Equal(t, gun.calls.Load(), stats.CallTimeouts)
}
