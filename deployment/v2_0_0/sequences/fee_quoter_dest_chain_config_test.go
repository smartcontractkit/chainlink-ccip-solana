package sequences

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-ccip/deployment/lanes"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/fee_quoter"
)

func TestReverseTranslateFQV2(t *testing.T) {
	t.Parallel()

	in := lanes.FeeQuoterDestChainConfig{
		IsEnabled:                   true,
		MaxDataBytes:                30_000,
		MaxPerMsgGasLimit:           3_000_000,
		DestGasOverhead:             300_000,
		DestGasPerPayloadByteBase:   math.MaxUint8,
		ChainFamilySelector:         0x2812d52c,
		DefaultTokenFeeUSDCents:     25,
		DefaultTokenDestGasOverhead: 90_000,
		DefaultTxGasLimit:           200_000,
		NetworkFeeUSDCents:          math.MaxUint16,
		V2Params:                    &lanes.FeeQuoterV2Params{LinkFeeMultiplierPercent: 90},
	}
	onchain := FeeQuoterDestChainConfigV2FromLanes(in)

	t.Run("round trips", func(t *testing.T) {
		t.Parallel()
		got, err := ReverseTranslateFQV2(onchain)
		require.NoError(t, err)
		require.Equal(t, in, got)
	})

	t.Run("rejects dest gas per payload byte base above uint8", func(t *testing.T) {
		t.Parallel()
		dc := onchain
		dc.DestGasPerPayloadByteBase = math.MaxUint8 + 1
		_, err := ReverseTranslateFQV2(dc)
		require.ErrorContains(t, err, "does not fit in uint8")
	})

	t.Run("rejects network fee above uint16", func(t *testing.T) {
		t.Parallel()
		dc := onchain
		dc.NetworkFee = fee_quoter.UsdCents{V0: math.MaxUint16 + 1}
		_, err := ReverseTranslateFQV2(dc)
		require.ErrorContains(t, err, "does not fit in uint16")
	})
}
