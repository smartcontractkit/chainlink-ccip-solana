package feequoter

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/fee_quoter"
)

func TestTargetDestChainConfig(t *testing.T) {
	current := fee_quoter.DestChainConfigV2{
		IsEnabled:                   true,
		LaneCodeVersion:             fee_quoter.CodeVersion_V1,
		MaxDataBytes:                32000,
		MaxPerMsgGasLimit:           fee_quoter.CrossChainGas{V0: 15000000},
		DestGasOverhead:             fee_quoter.CrossChainGas{V0: 100000},
		DestGasPerPayloadByteBase:   16,
		ChainFamilySelector:         [4]uint8{0x28, 0x12, 0xd5, 0x2c},
		DefaultTokenFeeUsdCents:     50,
		DefaultTokenDestGasOverhead: fee_quoter.CrossChainGas{V0: 90000},
		DefaultTxGasLimit:           fee_quoter.CrossChainGas{V0: 200000},
		NetworkFee:                  fee_quoter.UsdCents{V0: 50},
		LinkFeeMultiplierPercent:    90,
	}
	enabled := false
	maxDataBytes := uint32(64000)
	maxPerMsgGasLimit := uint32(2000000)
	destGasOverhead := uint32(110000)
	destGasPerPayloadByteBase := uint32(64)
	defaultTokenFeeUsdCents := uint16(75)
	defaultTokenDestGasOverhead := uint32(100000)
	defaultTxGasLimit := uint32(250000)
	networkFeeUSDCents := uint16(60)
	linkFeeMultiplierPercent := uint8(80)
	update := DestChainConfigUpdate{
		IsEnabled:                   &enabled,
		MaxDataBytes:                &maxDataBytes,
		MaxPerMsgGasLimit:           &maxPerMsgGasLimit,
		DestGasOverhead:             &destGasOverhead,
		DestGasPerPayloadByteBase:   &destGasPerPayloadByteBase,
		DefaultTokenFeeUsdCents:     &defaultTokenFeeUsdCents,
		DefaultTokenDestGasOverhead: &defaultTokenDestGasOverhead,
		DefaultTxGasLimit:           &defaultTxGasLimit,
		NetworkFeeUSDCents:          &networkFeeUSDCents,
		LinkFeeMultiplierPercent:    &linkFeeMultiplierPercent,
	}

	require.False(t, update.IsEmpty())
	require.True(t, DestChainConfigUpdate{}.IsEmpty())

	config, changed := targetDestChainConfig(update, current)
	require.True(t, changed)
	require.Equal(t, enabled, config.IsEnabled)
	require.Equal(t, maxDataBytes, config.MaxDataBytes)
	require.Equal(t, fee_quoter.CrossChainGas{V0: maxPerMsgGasLimit}, config.MaxPerMsgGasLimit)
	require.Equal(t, fee_quoter.CrossChainGas{V0: destGasOverhead}, config.DestGasOverhead)
	require.Equal(t, destGasPerPayloadByteBase, config.DestGasPerPayloadByteBase)
	require.Equal(t, defaultTokenFeeUsdCents, config.DefaultTokenFeeUsdCents)
	require.Equal(t, fee_quoter.CrossChainGas{V0: defaultTokenDestGasOverhead}, config.DefaultTokenDestGasOverhead)
	require.Equal(t, fee_quoter.CrossChainGas{V0: defaultTxGasLimit}, config.DefaultTxGasLimit)
	require.Equal(t, fee_quoter.UsdCents{V0: uint32(networkFeeUSDCents)}, config.NetworkFee)
	require.Equal(t, linkFeeMultiplierPercent, config.LinkFeeMultiplierPercent)
	require.Equal(t, current.LaneCodeVersion, config.LaneCodeVersion)
	require.Equal(t, current.ChainFamilySelector, config.ChainFamilySelector)

	_, changed = targetDestChainConfig(DestChainConfigUpdate{}, current)
	require.False(t, changed)

	_, changed = targetDestChainConfig(update, config)
	require.False(t, changed)
}
