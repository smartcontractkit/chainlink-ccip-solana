package loadtest

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-ccv/build/devenv/tests/e2e/load"
)

func newTestSolanaTXGun() *SolanaTXGun {
	return &SolanaTXGun{
		sentMsgCh:  make(chan load.SentMessage, sentMessageChannelBufferSize),
		shutdownCh: make(chan struct{}),
	}
}

func TestSolanaTXGun_SendBeforeClose(t *testing.T) {
	g := newTestSolanaTXGun()
	g.sendSent(load.SentMessage{SeqNo: 1})
	require.Len(t, g.sentMsgCh, 1)

	g.CloseSentChannel()
	msg, ok := <-g.SentMessages()
	require.True(t, ok)
	require.Equal(t, uint64(1), msg.SeqNo)
	_, ok = <-g.SentMessages()
	require.False(t, ok)
}

func TestSolanaTXGun_FullBufferShutdown(t *testing.T) {
	// Regression: the staging mesh test pattern runs the gun with no consumer; teardown must not hang.
	g := &SolanaTXGun{
		sentMsgCh:  make(chan load.SentMessage, 1),
		shutdownCh: make(chan struct{}),
	}
	g.sendSent(load.SentMessage{SeqNo: 1})
	var senders sync.WaitGroup
	for range 8 {
		senders.Go(func() {
			g.sendSent(load.SentMessage{SeqNo: 2})
		})
	}
	// Give blocked senders time to park on the full channel.
	time.Sleep(100 * time.Millisecond)

	finished := make(chan struct{})
	go func() {
		defer close(finished)
		g.CloseSentChannel()
	}()
	select {
	case <-finished:
	case <-time.After(10 * time.Second):
		t.Fatal("CloseSentChannel hung with a full buffer and no consumer")
	}
	senders.Wait()

	msg, ok := <-g.SentMessages()
	require.True(t, ok)
	require.Equal(t, uint64(1), msg.SeqNo)
	_, ok = <-g.SentMessages()
	require.False(t, ok)
}

func TestSolanaTXGun_SendAfterClose(t *testing.T) {
	g := newTestSolanaTXGun()
	g.CloseSentChannel()
	// Straggler sends after close must be dropped, not panic (CI regression).
	g.sendSent(load.SentMessage{SeqNo: 1})
	require.Len(t, g.sentMsgCh, 0)

	g.CloseSentChannel() // idempotent
}

func TestSolanaTXGun_ConcurrentSendAndClose(t *testing.T) {
	g := newTestSolanaTXGun()
	var wg sync.WaitGroup
	go g.CloseSentChannel() // close first: maximize the race with senders
	for range 200 {
		wg.Go(func() {
			g.sendSent(load.SentMessage{SeqNo: 2})
		})
	}
	wg.Wait()
	g.CloseSentChannel()
}
