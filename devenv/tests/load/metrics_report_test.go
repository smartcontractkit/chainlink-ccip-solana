package loadtest

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-ccv/build/devenv/tests/e2e/metrics"
)

func TestFailedMessagesFromSummary(t *testing.T) {
	t.Parallel()

	summary := metrics.MetricsSummary{
		SentMessages: map[uint64]string{
			3: "0xccc",
			1: "0xaaa",
			2: "0xbbb",
		},
		ReceivedMessages: map[uint64]string{
			1: "0xaaa",
			3: "0xccc",
		},
	}

	failed := failedMessagesFromSummary(summary)
	require.Equal(t, []failedMessage{{SeqNo: 2, MessageID: "0xbbb"}}, failed)
}

func TestFailedMessagesFromSummaryNoneFailed(t *testing.T) {
	t.Parallel()

	summary := metrics.MetricsSummary{
		SentMessages:     map[uint64]string{1: "0xaaa"},
		ReceivedMessages: map[uint64]string{1: "0xaaa"},
	}

	require.Empty(t, failedMessagesFromSummary(summary))
}
