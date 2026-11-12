package router

import (
	"context"
	"fmt"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/ccip_router"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
)

// LatestTokenAdminRegistryVersion is the TokenAdminRegistry layout version that Router writes require.
const LatestTokenAdminRegistryVersion = 4

// latestTokenAdminRegistrySize is the account size of the latest layout: discriminator, version, administrator,
// pending administrator, lookup table, writable indexes, mint, account_resolution_enabled, bump, interface version
// and skip_lookup_table_additional_entries.
const latestTokenAdminRegistrySize = 8 + 1 + 32 + 32 + 32 + 32 + 32 + 1 + 1 + 1 + 1

// firstTokenAdminRegistryVersionWithInterfaceVersion is the layout version that added interface_version.
const firstTokenAdminRegistryVersionWithInterfaceVersion = 3

// defaultTokenAdminRegistryInterfaceVersion mirrors TokenAdminRegistry::DEFAULT_INTERFACE_VERSION on-chain: the
// value the loader and the upgrade give registries whose layout predates interface_version.
const defaultTokenAdminRegistryInterfaceVersion = 1

// ReadTokenAdminRegistry parses a TokenAdminRegistry of any layout version and returns the stored version.
// Fields an older layout lacks get the same values the on-chain loader (load_token_admin_registry_checked)
// gives them: interface version 1 for v1 and v2, which only served pools implementing the v1 interface, and
// zero for everything else (the bump reads as unset; the on-chain upgrade then writes the canonical one).
//
// A v3 account may still hold 2 (WithoutExtendedLookupTable) in its resolution byte, which the latest
// layout decodes as a bool and rejects. ReadTokenAdminRegistry maps it onto the two latest-layout fields,
// as the on-chain loader does.
func ReadTokenAdminRegistry(ctx context.Context, client *rpc.Client, pda solana.PublicKey) (ccip_router.TokenAdminRegistry, uint8, error) {
	resp, err := client.GetAccountInfoWithOpts(ctx, pda, &rpc.GetAccountInfoOpts{Commitment: cldf_solana.SolDefaultCommitment})
	if err != nil {
		return ccip_router.TokenAdminRegistry{}, 0, err
	}
	if resp.Value.Owner == solana.SystemProgramID {
		return ccip_router.TokenAdminRegistry{}, 0, rpc.ErrNotFound
	}
	raw := resp.Value.Data.GetBinary()
	if len(raw) < 9 {
		return ccip_router.TokenAdminRegistry{}, 0, fmt.Errorf("token admin registry %s is too short: %d bytes", pda, len(raw))
	}
	version := raw[8]
	skipLookupTableAdditionalEntries := false
	if version < LatestTokenAdminRegistryVersion {
		raw = append([]byte(nil), raw...) // don't modify the RPC response buffer
		if version == 3 && len(raw) == latestTokenAdminRegistrySize-1 {
			resolutionOffset := len(raw) - 3 // before bump and interface version
			if raw[resolutionOffset] == uint8(ccip_router.ResolutionType_WithoutExtendedLookupTable) {
				raw[resolutionOffset] = 1
				skipLookupTableAdditionalEntries = true
			}
		}
		if len(raw) < latestTokenAdminRegistrySize {
			raw = append(raw, make([]byte, latestTokenAdminRegistrySize-len(raw))...)
		}
	}
	tar, err := ccip_router.ParseAccount_TokenAdminRegistry(raw)
	if err != nil {
		return ccip_router.TokenAdminRegistry{}, 0, fmt.Errorf("failed to parse token admin registry %s: %w", pda, err)
	}
	if skipLookupTableAdditionalEntries {
		tar.SkipLookupTableAdditionalEntries = true
	}
	if version < firstTokenAdminRegistryVersionWithInterfaceVersion {
		tar.InterfaceVersion = defaultTokenAdminRegistryInterfaceVersion
	}
	return *tar, version, nil
}

// TokenAdminRegistryResolutionType returns the resolution mode encoded by the registry's two stored fields,
// mirroring TokenAdminRegistry::account_resolution on-chain.
func TokenAdminRegistryResolutionType(tar ccip_router.TokenAdminRegistry) ccip_router.ResolutionType {
	switch {
	case !tar.AccountResolutionEnabled:
		return ccip_router.ResolutionType_NoResolution
	case tar.SkipLookupTableAdditionalEntries:
		return ccip_router.ResolutionType_WithoutExtendedLookupTable
	default:
		return ccip_router.ResolutionType_Standard
	}
}

// loadTokenAdminRegistry reads the TokenAdminRegistry of a mint, first trying to upgrade an older account to the
// latest layout, and returns the registry with the layout version it ended up at.
//
// A Router that supports the latest layout requires it for every write to the registry, and the upgrade is
// permissionless and always correct to run, so the deployer key signs it directly, also when the caller builds an
// MCMS proposal. The upgrade is best-effort: a Router that predates the latest layout rejects it, and the writes such
// a Router understands are still safe to make. The one that isn't is guarded by requireLatestLayoutForResolution.
func loadTokenAdminRegistry(b operations.Bundle, chain cldf_solana.Chain, router, mint solana.PublicKey) (ccip_router.TokenAdminRegistry, uint8, error) {
	pda, _, err := state.FindTokenAdminRegistryPDA(mint, router)
	if err != nil {
		return ccip_router.TokenAdminRegistry{}, 0, fmt.Errorf("failed to derive token admin registry PDA: %w", err)
	}
	tar, version, err := ReadTokenAdminRegistry(b.GetContext(), chain.Client, pda)
	if err != nil || version >= LatestTokenAdminRegistryVersion {
		return tar, version, err
	}
	configPDA, _, err := state.FindConfigPDA(router)
	if err != nil {
		return ccip_router.TokenAdminRegistry{}, 0, fmt.Errorf("failed to derive router config PDA: %w", err)
	}
	raw, err := ccip_router.NewUpgradeTokenAdminRegistryInstruction(configPDA, pda, mint, chain.DeployerKey.PublicKey(), solana.SystemProgramID)
	if err != nil {
		return ccip_router.TokenAdminRegistry{}, 0, fmt.Errorf("failed to build upgrade token admin registry instruction: %w", err)
	}
	ixData, err := raw.Data()
	if err != nil {
		return ccip_router.TokenAdminRegistry{}, 0, fmt.Errorf("failed to extract data payload from upgrade token admin registry instruction: %w", err)
	}
	if err = chain.Confirm([]solana.Instruction{solana.NewInstruction(router, raw.Accounts(), ixData)}); err != nil {
		b.Logger.Warnw("Could not upgrade token admin registry to the latest layout, continuing with the layout it has",
			"pda", pda, "mint", mint, "version", version, "latestVersion", LatestTokenAdminRegistryVersion, "error", err)
	} else {
		b.Logger.Infow("Upgraded token admin registry", "pda", pda, "mint", mint, "fromVersion", version)
	}
	// Read it back either way: a confirmation error doesn't prove the upgrade didn't land.
	return ReadTokenAdminRegistry(b.GetContext(), chain.Client, pda)
}

// requireLatestLayoutForResolution refuses to write WithoutExtendedLookupTable to a registry that is still on an
// older layout. Only the latest layout stores that mode as the 0/1 resolution byte the 1.6 OffRamp decodes as a bool,
// plus a trailing flag. An older layout stores it as 2 in that byte, which makes the registry unreadable to the 1.6
// OffRamp and breaks every 1.6 lane of the token. A registry still on an older layout after loadTokenAdminRegistry
// most likely means the Router program predates the latest layout.
func requireLatestLayoutForResolution(mint solana.PublicKey, version uint8, resolution ccip_router.ResolutionType) error {
	if resolution != ccip_router.ResolutionType_WithoutExtendedLookupTable || version >= LatestTokenAdminRegistryVersion {
		return nil
	}
	return fmt.Errorf("token admin registry for mint %s is at layout v%d and could not be upgraded to v%d, most likely because the Router "+
		"program predates it; setting %s on it would break the token's 1.6 lanes, so upgrade the Router program first",
		mint, version, LatestTokenAdminRegistryVersion, resolution)
}
