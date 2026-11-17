package router

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/ccip_router"
)

func TestTargetDestChainConfig(t *testing.T) {
	current := ccip_router.DestChainConfigCcipV2{
		BaseExecutionGasCost: ccip_router.CrossChainGas{V0: 200_000}, MessageNetworkFee: ccip_router.UsdCents{V0: 50},
		TokenTransferNetworkFee: ccip_router.UsdCents{V0: 150},
		MaxFeePerMessage:        ccip_router.UsdCents{V0: 1000},
		TokenReceiverAllowed:    true,
	}
	gas := uint32(300_000)

	// Raising one field restates every other field unchanged.
	config, changed := targetDestChainConfig(SetDestChainConfigParams{BaseExecutionGasCost: &gas}, current)
	require.True(t, changed)
	require.Equal(t, uint32(300_000), config.BaseExecutionGasCost.V0)
	require.Equal(t, current.MessageNetworkFee, config.MessageNetworkFee)
	require.Equal(t, current.TokenTransferNetworkFee, config.TokenTransferNetworkFee)
	require.Equal(t, current.MaxFeePerMessage, config.MaxFeePerMessage)
	require.True(t, config.TokenReceiverAllowed)

	// An empty request is a skip.
	_, changed = targetDestChainConfig(SetDestChainConfigParams{}, current)
	require.False(t, changed)

	// Re-running the change is a skip.
	already := current
	already.BaseExecutionGasCost = ccip_router.CrossChainGas{V0: 300_000}
	_, changed = targetDestChainConfig(SetDestChainConfigParams{BaseExecutionGasCost: &gas}, already)
	require.False(t, changed)

	// Fee and flag changes apply on top of the restated fields.
	fee := uint16(10)
	tokenFee := uint16(25)
	maxFee := uint16(1000)
	allowed := false
	config, changed = targetDestChainConfig(SetDestChainConfigParams{
		MessageNetworkFeeUSDCents:       &fee,
		TokenTransferNetworkFeeUSDCents: &tokenFee,
		MaxFeePerMessageUSDCents:        &maxFee,
		TokenReceiverAllowed:            &allowed,
	}, current)
	require.True(t, changed)
	require.Equal(t, uint32(200_000), config.BaseExecutionGasCost.V0)
	require.Equal(t, uint32(10), config.MessageNetworkFee.V0)
	require.Equal(t, uint32(25), config.TokenTransferNetworkFee.V0)
	require.Equal(t, uint32(1000), config.MaxFeePerMessage.V0)
	require.False(t, config.TokenReceiverAllowed)
}
