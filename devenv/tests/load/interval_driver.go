package loadtest

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/smartcontractkit/chainlink-ccv/build/devenv/tests/e2e/load"
)

// runIntervalDriver fires gun.Call on a fixed interval until duration elapses.
// It exists for sub-1 RPS schedules (e.g. one message every 30s) that wasp's
// integer-RPS schedules cannot express. Each send runs in its own goroutine so
// a slow send does not shift the cadence; overlapping sends are safe (the guns
// round-robin signers / manage nonces for concurrent use).
//
// The first send happens after one interval, so exactly duration/interval sends
// are fired. In-flight sends are awaited before returning so their outcomes are
// counted even when they finish after duration.
func runIntervalDriver(t *testing.T, gun load.LoadGun, interval, duration, callTimeout time.Duration) gunCallStats {
	t.Helper()

	var success, failed, timedOut atomic.Int64
	var wg sync.WaitGroup
	var mu sync.Mutex
	errCounts := map[string]int{}
	recordErr := func(msg string) {
		mu.Lock()
		errCounts[msg]++
		mu.Unlock()
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	timer := time.NewTimer(duration)
	defer timer.Stop()

	for {
		select {
		case <-timer.C:
			wg.Wait()
			if len(errCounts) == 0 {
				errCounts = nil
			}
			return gunCallStats{
				TxsSubmitted: success.Load(),
				TxsFailed:    failed.Load(),
				CallTimeouts: timedOut.Load(),
				ErrCounts:    errCounts,
			}
		case <-ticker.C:
			wg.Add(1)
			//nolint:modernize // WaitGroup.Go needs Go 1.25+; keep Add/Done for older toolchains
			go func() {
				defer wg.Done()
				start := time.Now()
				resp := gun.Call(nil)
				switch {
				case resp == nil:
					failed.Add(1)
					recordErr("send returned nil response")
					t.Logf("interval driver: send returned nil response")
				case resp.Failed || resp.Error != "":
					failed.Add(1)
					recordErr(resp.Error)
					t.Logf("interval driver: send failed: %s", resp.Error)
				default:
					success.Add(1)
				}
				if elapsed := time.Since(start); elapsed > callTimeout {
					timedOut.Add(1)
					t.Logf("interval driver: send exceeded call timeout (%v > %v)", elapsed, callTimeout)
				}
			}()
		}
	}
}
