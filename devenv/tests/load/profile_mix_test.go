package loadtest

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-ccv/build/devenv/tests/e2e/load"
)

func TestExpectedProfileMix(t *testing.T) {
	t.Parallel()

	profiles := []load.MessageProfileConfig{
		{Name: "small"},
		{Name: "large", HasData: true, DataSizeBytes: 1500},
	}
	ratios := []load.MessageTestRatio{
		{Ratio: 1, MessageProfile: "small"},
		{Ratio: 1, MessageProfile: "large"},
	}

	mix := expectedProfileMix(profiles, ratios)
	require.Len(t, mix, 2)
	require.Equal(t, "small", mix[0].Name)
	require.InEpsilon(t, 50.0, mix[0].ExpectedPct, 0.000001)
	require.Equal(t, 0, mix[0].DataBytes)
	require.Equal(t, "large", mix[1].Name)
	require.InEpsilon(t, 50.0, mix[1].ExpectedPct, 0.000001)
	require.Equal(t, 1500, mix[1].DataBytes)
}
