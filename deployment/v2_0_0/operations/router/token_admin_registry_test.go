package router

import (
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/ccip_router"
)

func TestValidatePoolInterfaceImplementation(t *testing.T) {
	tests := []struct {
		name             string
		interfaceVersion uint8
		resolution       ccip_router.ResolutionType
		wantErr          string
	}{
		{"v1 without resolution", 1, ccip_router.ResolutionType_NoResolution, ""},
		{"v1 with standard resolution", 1, ccip_router.ResolutionType_Standard, "requires account resolution NoResolution"},
		{"v1 skipping extras", 1, ccip_router.ResolutionType_WithoutExtendedLookupTable, "requires account resolution NoResolution"},
		{"v2 skipping extras", 2, ccip_router.ResolutionType_WithoutExtendedLookupTable, ""},
		{"v2 without resolution", 2, ccip_router.ResolutionType_NoResolution, "requires account resolution WithoutExtendedLookupTable"},
		{"v2 with standard resolution", 2, ccip_router.ResolutionType_Standard, "requires account resolution WithoutExtendedLookupTable"},
		{"unknown version", 3, ccip_router.ResolutionType_WithoutExtendedLookupTable, "invalid pool interface version 3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePoolInterfaceImplementation(tt.interfaceVersion, tt.resolution)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

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
