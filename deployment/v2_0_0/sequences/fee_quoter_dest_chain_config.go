package sequences

import (
	"encoding/binary"
	"fmt"
	"math"
	"math/big"

	"github.com/smartcontractkit/chainlink-ccip/deployment/lanes"
	ccipdeploymentv2adapters "github.com/smartcontractkit/chainlink-ccip/deployment/v2_0_0/adapters"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/fee_quoter"
)

func coalesce[T any](p *T, def T) T {
	if p == nil {
		return def
	}
	return *p
}

// FeeQuoterDestChainConfigV2FromChainFamily maps chainlink-ccip's ConfigureChainForLanes
// FeeQuoterDestChainConfigOverrides (deployment/v2_0_0/adapters) to on-chain fee_quoter.DestChainConfigV2.
func FeeQuoterDestChainConfigV2FromChainFamily(fqc ccipdeploymentv2adapters.FeeQuoterDestChainConfigOverrides) fee_quoter.DestChainConfigV2 {
	return fee_quoter.DestChainConfigV2{
		IsEnabled:                   coalesce(fqc.IsEnabled, true),
		LaneCodeVersion:             fee_quoter.CodeVersion_V1,
		MaxDataBytes:                coalesce(fqc.MaxDataBytes, 0),
		MaxPerMsgGasLimit:           fee_quoter.CrossChainGas{V0: coalesce(fqc.MaxPerMsgGasLimit, 0)},
		DestGasOverhead:             fee_quoter.CrossChainGas{V0: coalesce(fqc.DestGasOverhead, 0)},
		DestGasPerPayloadByteBase:   uint32(coalesce(fqc.DestGasPerPayloadByteBase, 0)),
		ChainFamilySelector:         fqc.ChainFamilySelector,
		DefaultTokenDestGasOverhead: fee_quoter.CrossChainGas{V0: coalesce(fqc.DefaultTokenDestGasOverhead, 0)},
		DefaultTokenFeeUsdCents:     coalesce(fqc.DefaultTokenFeeUSDCents, 0),
		DefaultTxGasLimit:           fee_quoter.CrossChainGas{V0: coalesce(fqc.DefaultTxGasLimit, 0)},
		NetworkFee:                  fee_quoter.UsdCents{V0: uint32(coalesce(fqc.NetworkFeeUSDCents, 0))},
		LinkFeeMultiplierPercent:    coalesce(fqc.LinkFeeMultiplierPercent, 0),
	}
}

// FeeQuoterDestChainConfigV2FromLanes maps lanes.FeeQuoterDestChainConfig (update_lanes /
// fees product types) through the chain-family struct above so lane setup and fee adapters
// share one on-chain encoding path with ConfigureChainForLanes.
func FeeQuoterDestChainConfigV2FromLanes(fqc lanes.FeeQuoterDestChainConfig) fee_quoter.DestChainConfigV2 {
	return FeeQuoterDestChainConfigV2FromChainFamily(feeQuoterDestChainConfigFromLanes(fqc))
}

func feeQuoterDestChainConfigFromLanes(fqc lanes.FeeQuoterDestChainConfig) ccipdeploymentv2adapters.FeeQuoterDestChainConfigOverrides {
	var linkPct *uint8
	var usdPerUnit *big.Int
	if fqc.V2Params != nil {
		linkPct = new(fqc.V2Params.LinkFeeMultiplierPercent)
		usdPerUnit = fqc.V2Params.USDPerUnitGas
	}
	return ccipdeploymentv2adapters.FeeQuoterDestChainConfigOverrides{
		OverrideExistingConfig:      fqc.OverrideExistingConfig,
		IsEnabled:                   new(fqc.IsEnabled),
		MaxDataBytes:                new(fqc.MaxDataBytes),
		MaxPerMsgGasLimit:           new(fqc.MaxPerMsgGasLimit),
		DestGasOverhead:             new(fqc.DestGasOverhead),
		DestGasPerPayloadByteBase:   new(fqc.DestGasPerPayloadByteBase),
		ChainFamilySelector:         chainFamilySelectorFromUint32(fqc.ChainFamilySelector),
		DefaultTokenFeeUSDCents:     new(fqc.DefaultTokenFeeUSDCents),
		DefaultTokenDestGasOverhead: new(fqc.DefaultTokenDestGasOverhead),
		DefaultTxGasLimit:           new(fqc.DefaultTxGasLimit),
		NetworkFeeUSDCents:          new(fqc.NetworkFeeUSDCents),
		LinkFeeMultiplierPercent:    linkPct,
		USDPerUnitGas:               usdPerUnit,
	}
}

func chainFamilySelectorFromUint32(sel uint32) [4]byte {
	var out [4]byte
	binary.BigEndian.PutUint32(out[:], sel)
	return out
}

// ReverseTranslateFQV2 maps the on-chain fee_quoter.DestChainConfigV2 back to
// the product-level lanes.FeeQuoterDestChainConfig type (with V2Params populated). It fails
// when a stored value does not fit the narrower product-level field, so a read-modify-write
// cannot write a truncated value back.
func ReverseTranslateFQV2(dc fee_quoter.DestChainConfigV2) (lanes.FeeQuoterDestChainConfig, error) {
	if dc.DestGasPerPayloadByteBase > math.MaxUint8 {
		return lanes.FeeQuoterDestChainConfig{}, fmt.Errorf("dest gas per payload byte base %d does not fit in uint8", dc.DestGasPerPayloadByteBase)
	}
	if dc.NetworkFee.V0 > math.MaxUint16 {
		return lanes.FeeQuoterDestChainConfig{}, fmt.Errorf("network fee %d USD cents does not fit in uint16", dc.NetworkFee.V0)
	}
	return lanes.FeeQuoterDestChainConfig{
		IsEnabled:                   dc.IsEnabled,
		MaxDataBytes:                dc.MaxDataBytes,
		MaxPerMsgGasLimit:           dc.MaxPerMsgGasLimit.V0,
		DestGasOverhead:             dc.DestGasOverhead.V0,
		DestGasPerPayloadByteBase:   uint8(dc.DestGasPerPayloadByteBase),
		ChainFamilySelector:         binary.BigEndian.Uint32(dc.ChainFamilySelector[:]),
		DefaultTokenFeeUSDCents:     dc.DefaultTokenFeeUsdCents,
		DefaultTokenDestGasOverhead: dc.DefaultTokenDestGasOverhead.V0,
		DefaultTxGasLimit:           dc.DefaultTxGasLimit.V0,
		NetworkFeeUSDCents:          uint16(dc.NetworkFee.V0),
		V2Params: &lanes.FeeQuoterV2Params{
			LinkFeeMultiplierPercent: dc.LinkFeeMultiplierPercent,
		},
	}, nil
}
