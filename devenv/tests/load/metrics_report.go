package loadtest

import (
	"cmp"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/smartcontractkit/chainlink-ccv/build/devenv/tests/e2e/metrics"
)

type messageDetail struct {
	SeqNo        uint64    `json:"seq_no"`
	MessageID    string    `json:"message_id"`
	SourceChain  uint64    `json:"source_chain"`
	DestChain    uint64    `json:"dest_chain"`
	SentTime     time.Time `json:"sent_time"`
	ExecutedTime time.Time `json:"executed_time"`
	LatencySec   float64   `json:"latency_sec"`
}

type loadMetricsReport struct {
	Test              string                   `json:"test"`
	Direction         string                   `json:"direction"`
	TotalAttempted    int                      `json:"total_attempted"`
	TotalSent         int                      `json:"total_sent"`
	TotalReceived     int                      `json:"total_received"`
	SuccessRatePct    float64                  `json:"success_rate_pct"`
	MinLatencySec     float64                  `json:"min_latency_sec"`
	P90LatencySec     float64                  `json:"p90_latency_sec"`
	P95LatencySec     float64                  `json:"p95_latency_sec"`
	P99LatencySec     float64                  `json:"p99_latency_sec"`
	MaxLatencySec     float64                  `json:"max_latency_sec"`
	Passed            bool                     `json:"passed"`
	Timestamp         time.Time                `json:"timestamp"`
	MessageProfileMix []messageProfileMixEntry `json:"message_profile_mix,omitempty"`
	FailedMessages    []failedMessage          `json:"failed_messages,omitempty"`
	GunStats          gunCallStats             `json:"gun_stats"`
	MessageDetails    []messageDetail          `json:"message_details,omitempty"`
}

// failedMessage identifies a message that was sent on the source chain but never
// observed executed on the destination chain.
type failedMessage struct {
	SeqNo     uint64 `json:"seq_no"`
	MessageID string `json:"message_id"`
}

// gunCallStats holds wasp generator send outcomes. Failed/timeout sends never reach
// the sent channel, so they are invisible in TotalSent/TotalReceived. ErrCounts
// aggregates the failure error strings (error text → occurrences).
type gunCallStats struct {
	TxsSubmitted int64          `json:"txs_submitted"`
	TxsFailed    int64          `json:"txs_failed"`
	CallTimeouts int64          `json:"call_timeouts"`
	ErrCounts    map[string]int `json:"errors,omitempty"`
}

// failedMessagesFromSummary returns the messages that were sent but never received,
// sorted by sequence number for deterministic output.
func failedMessagesFromSummary(summary metrics.MetricsSummary) []failedMessage {
	failed := make([]failedMessage, 0)
	for seqNo, msgID := range summary.SentMessages {
		if _, ok := summary.ReceivedMessages[seqNo]; !ok {
			failed = append(failed, failedMessage{SeqNo: seqNo, MessageID: msgID})
		}
	}
	slices.SortFunc(failed, func(a, b failedMessage) int {
		return cmp.Compare(a.SeqNo, b.SeqNo)
	})
	return failed
}

func directionLabel(testName string) string {
	switch testName {
	case "TestEVM2SolanaLoad":
		return "EVM→Sol"
	case "TestSolana2EVMLoad":
		return "Sol→EVM"
	default:
		return testName
	}
}

func loadMetricsDir() string {
	if d := os.Getenv("LOAD_TEST_METRICS_DIR"); d != "" {
		return d
	}
	return "../../"
}

func writeLoadMetricsReport(t *testing.T, summary metrics.MetricsSummary, msgs []metrics.MessageMetrics, passed bool, profileMix []messageProfileMixEntry, failed []failedMessage, gunStats gunCallStats) {
	t.Helper()

	var successRate float64
	if summary.TotalSent > 0 {
		successRate = float64(summary.TotalReceived) / float64(summary.TotalSent) * 100
	}

	// randomly generate some message details to include in the report, up to 10 messages
	details := make([]messageDetail, 0, len(msgs))
	for _, m := range msgs {
		details = append(details, messageDetail{
			SeqNo:        m.SeqNo,
			MessageID:    m.MessageID,
			SourceChain:  m.SourceChain,
			DestChain:    m.DestChain,
			SentTime:     m.SentTime,
			ExecutedTime: m.ExecutedTime,
			LatencySec:   m.LatencyDuration.Seconds(),
		})
	}
	if len(details) > 10 {
		rand.Shuffle(len(details), func(i, j int) {
			details[i], details[j] = details[j], details[i]
		})
		details = details[:10]
	}
	slices.SortFunc(details, func(a, b messageDetail) int {
		return cmp.Compare(a.SeqNo, b.SeqNo)
	})

	report := loadMetricsReport{
		Test:              t.Name(),
		Direction:         directionLabel(t.Name()),
		TotalAttempted:    int(gunStats.TxsSubmitted + gunStats.TxsFailed),
		TotalSent:         summary.TotalSent,
		TotalReceived:     summary.TotalReceived,
		SuccessRatePct:    successRate,
		MinLatencySec:     summary.MinLatency.Seconds(),
		P90LatencySec:     summary.P90Latency.Seconds(),
		P95LatencySec:     summary.P95Latency.Seconds(),
		P99LatencySec:     summary.P99Latency.Seconds(),
		MaxLatencySec:     summary.MaxLatency.Seconds(),
		Passed:            passed,
		Timestamp:         time.Now().UTC(),
		MessageProfileMix: profileMix,
		FailedMessages:    failed,
		GunStats:          gunStats,
		MessageDetails:    details,
	}

	dir := loadMetricsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create metrics dir %s: %v", dir, err)
	}

	path := filepath.Join(dir, fmt.Sprintf("load-metrics-%s.json", t.Name()))
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatalf("marshal load metrics: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write load metrics to %s: %v", path, err)
	}

	t.Logf("LOAD_METRICS_JSON=%s", string(data))
}
