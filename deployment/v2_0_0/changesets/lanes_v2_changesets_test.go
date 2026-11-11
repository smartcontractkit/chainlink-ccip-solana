package changesets

import (
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/ccip_router"
)

func TestCheckCctpTokenAdminRegistryReady(t *testing.T) {
	mint := solana.NewWallet().PublicKey()
	registry := func(interfaceVersion uint8, resolutionEnabled, skipAdditionalEntries bool) ccip_router.TokenAdminRegistry {
		return ccip_router.TokenAdminRegistry{
			InterfaceVersion:                 interfaceVersion,
			AccountResolutionEnabled:         resolutionEnabled,
			SkipLookupTableAdditionalEntries: skipAdditionalEntries,
		}
	}

	// The state MigrateCctpTokenPoolToV2 leaves the registry in.
	require.NoError(t, checkCctpTokenAdminRegistryReady(mint, registry(2, true, true)))

	// Not migrated yet.
	require.ErrorContains(t, checkCctpTokenAdminRegistryReady(mint, registry(1, true, true)), "interface version 1")

	// Standard: what the 1.6 outage recovery set, and what the TAR v4 upgrade preserves.
	require.ErrorContains(t, checkCctpTokenAdminRegistryReady(mint, registry(2, true, false)), "account resolution Standard")

	// NoResolution would skip the pool's own account resolution altogether.
	require.ErrorContains(t, checkCctpTokenAdminRegistryReady(mint, registry(2, false, false)), "account resolution NoResolution")
}
