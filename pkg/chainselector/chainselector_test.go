package chainselector_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-ccv/protocol"

	"github.com/smartcontractkit/chainlink-ccip-solana/pkg/chainselector"
)

func TestParse(t *testing.T) {
	t.Parallel()

	sel, err := chainselector.Parse("16423721717087811551")
	require.NoError(t, err)
	require.Equal(t, protocol.ChainSelector(16423721717087811551), sel)
}

func TestParse_invalid(t *testing.T) {
	t.Parallel()

	_, err := chainselector.Parse("not-a-number")
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid chain selector")
}

func TestFormat(t *testing.T) {
	t.Parallel()

	require.Equal(t, "16423721717087811551", chainselector.Format(protocol.ChainSelector(16423721717087811551)))
}

func TestFormatParseRoundtrip(t *testing.T) {
	t.Parallel()

	const raw = "16423721717087811551"
	sel, err := chainselector.Parse(raw)
	require.NoError(t, err)
	require.Equal(t, raw, chainselector.Format(sel))
}

func TestParseCanonicalDecimal(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{"0", "7", "16423721717087811551", "18446744073709551615"} {
		t.Run(raw, func(t *testing.T) {
			t.Parallel()
			sel, err := chainselector.Parse(raw)
			require.NoError(t, err)
			require.Equal(t, raw, chainselector.Format(sel))
		})
	}
	for _, raw := range []string{"00", "007", "016423721717087811551", "18446744073709551616"} {
		t.Run(raw, func(t *testing.T) {
			t.Parallel()
			_, err := chainselector.Parse(raw)
			require.ErrorContains(t, err, "invalid chain selector")
		})
	}
}
