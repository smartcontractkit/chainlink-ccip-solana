package router

import (
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/ccip_router"
)

func TestRequireLatestLayoutForResolution(t *testing.T) {
	mint := solana.NewWallet().PublicKey()

	tests := []struct {
		name       string
		version    uint8
		resolution ccip_router.ResolutionType
		wantErr    bool
	}{
		// An older layout stores this mode as 2 in the byte the 1.6 OffRamp decodes as a bool.
		{"skipping extras on the v3 layout", 3, ccip_router.ResolutionType_WithoutExtendedLookupTable, true},
		{"skipping extras on the v1 layout", 1, ccip_router.ResolutionType_WithoutExtendedLookupTable, true},
		{"skipping extras on the latest layout", LatestTokenAdminRegistryVersion, ccip_router.ResolutionType_WithoutExtendedLookupTable, false},
		// Modes stored as 0 or 1 are readable by the 1.6 OffRamp on every layout.
		{"standard on the v3 layout", 3, ccip_router.ResolutionType_Standard, false},
		{"no resolution on the v1 layout", 1, ccip_router.ResolutionType_NoResolution, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := requireLatestLayoutForResolution(mint, tt.version, tt.resolution)
			if tt.wantErr {
				require.ErrorContains(t, err, "upgrade the Router program first")
				return
			}
			require.NoError(t, err)
		})
	}
}
