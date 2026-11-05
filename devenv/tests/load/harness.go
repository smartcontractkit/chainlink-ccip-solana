package loadtest

import (
	"context"
	"maps"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-ccv/build/devenv/cciptestinterfaces"
	"github.com/smartcontractkit/chainlink-ccv/build/devenv/tests/e2e/load"
	"github.com/smartcontractkit/chainlink-ccv/build/devenv/tests/e2e/metrics"
	"github.com/smartcontractkit/chainlink-testing-framework/wasp"
)

const (
	// Platform target for e2e latency is 1m (CCIP-9269).
	expectedP90Latency = 1 * time.Minute
	// Per-send cap for SendChainMessage; separate from overall verification timeout.
	// 80 seconds accommodates public testnets: Sepolia confirm + RPC backend lag exceeds
	defaultLoadExecTimeout = 80 * time.Second
	// Grace period after load generation ends: no new messages are sent, but
	// AssertMessagesAsync keeps verifying this long so messages sent near the
	// end of the load window can still land. Passing runs finish early.
	verifyGracePeriod = 10 * time.Minute
	// Optional buffer after wasp Wait so slow dest polling can finish.
	postTestVerificationDelay = 30 * time.Second
)

type loadTestConfig struct {
	rps      int64
	duration time.Duration
	// sendInterval > 0 selects ticker mode (fixed interval between sends, for
	// sub-1 RPS schedules wasp cannot express) and overrides rps.
	sendInterval  time.Duration
	msgSize       int64
	fireAndForget bool
}

// runLoadProfile is the shared harness for both directions: start verification before
// load, then tear down in the order AssertMessagesAsync expects (close gun channel last).
func runLoadProfile(
	ctx context.Context,
	t *testing.T,
	impls map[uint64]cciptestinterfaces.CCIP17,
	gun load.LoadGun,
	cfg loadTestConfig,
	opts ...loadProfileOption,
) {
	t.Helper()

	o := loadProfileOptions{
		execTimeout:     defaultLoadExecTimeout,
		postVerifyDelay: 0,
	}
	for _, apply := range opts {
		apply(&o)
	}

	overallTimeout := cfg.duration + verifyGracePeriod

	// guns push a message only after its source-chain send tx is confirmed
	// so tap-arrival time approximately equal to source-confirmed time. Recorded per sequence number
	// to derive a "confirmed → executed" latency alongside the standard "sent → executed" one.
	arrivals := &sync.Map{}
	relayer := make(chan load.SentMessage, sentMessageChannelBufferSize)
	go func() {
		for m := range gun.SentMessages() {
			arrivals.Store(m.SeqNo, time.Now())
			relayer <- m
		}
		close(relayer)
	}()

	vc := load.VerificationContext{Ctx: ctx, T: t, Impl: impls}
	waitForMetrics := load.AssertMessagesAsync(vc, relayer, overallTimeout)

	gunStats := runLoadGeneration(t, gun, cfg, o.execTimeout)

	if o.postVerifyDelay > 0 {
		time.Sleep(o.postVerifyDelay)
	}

	gun.CloseSentChannel()

	msgs, totals := waitForMetrics()
	summary := metrics.CalculateMetricsSummary(msgs, totals)
	failed := failedMessagesFromSummary(summary)

	passed := summary.TotalSent == summary.TotalReceived &&
		summary.TotalSent > 0 &&
		summary.P90Latency <= expectedP90Latency
	writeLoadMetricsReport(t, summary, msgs, passed, o.messageProfileMix, failed, gunStats)

	metrics.PrintMetricsSummary(t, summary)
	logFailureDetails(t, failed, gunStats)

	if gunStats.TxsFailed > 0 || gunStats.CallTimeouts > 0 {
		t.Logf("Load generation reconciliation: attempted=%d (submitted=%d + failed=%d + timeouts=%d), received=%d",
			gunStats.TxsSubmitted+gunStats.TxsFailed,
			gunStats.TxsSubmitted, gunStats.TxsFailed, gunStats.CallTimeouts, summary.TotalReceived)
	}

	require.Equal(t, summary.TotalSent, summary.TotalReceived)
	// Catches silent wasp failures (e.g. router revert) where sent==received==0 still passes Equal.
	require.Positive(t, summary.TotalSent, "load gun should send at least one message")
	// Ensure P90 latency is within expected bounds.
	require.LessOrEqual(t, summary.P90Latency, expectedP90Latency)
}

// runLoadGeneration drives the gun for the configured duration and returns send
// outcome stats. Ticker mode (cfg.sendInterval > 0) fires on a fixed interval;
// otherwise a wasp RPS schedule is used.
func runLoadGeneration(
	t *testing.T,
	gun load.LoadGun,
	cfg loadTestConfig,
	execTimeout time.Duration,
) gunCallStats {
	t.Helper()

	if cfg.sendInterval > 0 {
		t.Logf("Load generation: ticker mode, 1 send every %v for %v", cfg.sendInterval, cfg.duration)
		return runIntervalDriver(t, gun, cfg.sendInterval, cfg.duration, execTimeout)
	}

	t.Logf("Load generation: wasp RPS mode, %d RPS for %v", cfg.rps, cfg.duration)
	profile := wasp.NewProfile().Add(wasp.NewGenerator(&wasp.Config{
		T:           t,
		Gun:         gun,
		CallTimeout: execTimeout,
		LoadType:    wasp.RPS,
		Schedule:    wasp.Plain(cfg.rps, cfg.duration),
	}))
	_, err := profile.Run(true)
	require.NoError(t, err)
	profile.Wait()

	stats := gunCallStats{}
	if len(profile.Generators) > 0 {
		g := profile.Generators[0]
		s := g.Stats()
		stats = gunCallStats{
			TxsSubmitted: s.Success.Load(),
			TxsFailed:    s.Failed.Load(),
			CallTimeouts: s.CallTimeout.Load(),
			ErrCounts:    countErrStrings(g.Errors()),
		}
	}
	return stats
}

// countErrStrings aggregates wasp's per-failure error strings into error → count.
func countErrStrings(errs []string) map[string]int {
	if len(errs) == 0 {
		return nil
	}
	m := make(map[string]int, len(errs))
	for _, e := range errs {
		m[e]++
	}
	return m
}

// logFailureDetails prints gun-level send failures and the IDs of messages that were
// sent but never observed executed on the destination chain.
func logFailureDetails(t *testing.T, failed []failedMessage, gunStats gunCallStats) {
	t.Helper()
	t.Logf("Gun calls: txs_submitted=%d txs_failed=%d call_timeouts=%d", gunStats.TxsSubmitted, gunStats.TxsFailed, gunStats.CallTimeouts)
	for _, e := range slices.Sorted(maps.Keys(gunStats.ErrCounts)) {
		t.Logf("  gun call error (%dx): %s", gunStats.ErrCounts[e], e)
	}
	if len(failed) == 0 {
		return
	}
	t.Logf("Failed messages (sent but not executed): %d", len(failed))
	for _, f := range failed {
		t.Logf("  seqNo=%d messageID=%s", f.SeqNo, f.MessageID)
	}
}

type loadProfileOptions struct {
	execTimeout       time.Duration
	postVerifyDelay   time.Duration
	messageProfileMix []messageProfileMixEntry
}

type loadProfileOption func(*loadProfileOptions)

// withExecTimeout extends the per-send SendChainMessage budget. The send includes
// waiting for the source-chain tx to confirm (e.g. Sepolia block times), so EVM
// sources need more than the 30s default; destination execution is not covered
// by this timeout (that's the AssertMessagesAsync budget).
func withExecTimeout(d time.Duration) loadProfileOption {
	return func(o *loadProfileOptions) {
		o.execTimeout = d
	}
}

func withPostVerifyDelay(d time.Duration) loadProfileOption {
	return func(o *loadProfileOptions) {
		o.postVerifyDelay = d
	}
}

// withMessageProfileMix records configured payload mix in the metrics JSON / PR summary.
func withMessageProfileMix(
	profiles []load.MessageProfileConfig,
	ratios []load.MessageTestRatio,
) loadProfileOption {
	return func(o *loadProfileOptions) {
		o.messageProfileMix = expectedProfileMix(profiles, ratios)
	}
}
