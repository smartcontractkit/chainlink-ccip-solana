package router

import (
	"context"
	"fmt"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/ccip_router"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
)

// ReadTokenAdminRegistry parses a TokenAdminRegistry of any layout version and returns the stored version.
// A v1 or v2 account is shorter than v3, so it is padded with zeros: the fields added in v3 read as zero.
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
	if version < 3 {
		raw = append(raw, 0, 0, 0)
	}
	tar, err := ccip_router.ParseAccount_TokenAdminRegistry(raw)
	if err != nil {
		return ccip_router.TokenAdminRegistry{}, 0, fmt.Errorf("failed to parse token admin registry %s: %w", pda, err)
	}
	return *tar, version, nil
}

// loadTokenAdminRegistry reads the TokenAdminRegistry of a mint and upgrades a v1 or v2 account to v3 first,
// because every Router instruction that writes the registry requires v3.
// The upgrade is permissionless, so the deployer key signs it directly, also when the caller builds an MCMS proposal.
func loadTokenAdminRegistry(b operations.Bundle, chain cldf_solana.Chain, router, mint solana.PublicKey) (ccip_router.TokenAdminRegistry, error) {
	pda, _, err := state.FindTokenAdminRegistryPDA(mint, router)
	if err != nil {
		return ccip_router.TokenAdminRegistry{}, fmt.Errorf("failed to derive token admin registry PDA: %w", err)
	}
	tar, version, err := ReadTokenAdminRegistry(b.GetContext(), chain.Client, pda)
	if err != nil || version >= 3 {
		return tar, err
	}
	configPDA, _, err := state.FindConfigPDA(router)
	if err != nil {
		return ccip_router.TokenAdminRegistry{}, fmt.Errorf("failed to derive router config PDA: %w", err)
	}
	raw, err := ccip_router.NewUpgradeTokenAdminRegistryInstruction(configPDA, pda, mint, chain.DeployerKey.PublicKey(), solana.SystemProgramID)
	if err != nil {
		return ccip_router.TokenAdminRegistry{}, fmt.Errorf("failed to build upgrade token admin registry instruction: %w", err)
	}
	ixData, err := raw.Data()
	if err != nil {
		return ccip_router.TokenAdminRegistry{}, fmt.Errorf("failed to extract data payload from upgrade token admin registry instruction: %w", err)
	}
	if err = chain.Confirm([]solana.Instruction{solana.NewInstruction(router, raw.Accounts(), ixData)}); err != nil {
		return ccip_router.TokenAdminRegistry{}, fmt.Errorf("failed to upgrade token admin registry %s from version %d: %w", pda, version, err)
	}
	b.Logger.Infow("Upgraded token admin registry", "pda", pda, "mint", mint, "fromVersion", version)
	tar, _, err = ReadTokenAdminRegistry(b.GetContext(), chain.Client, pda)
	return tar, err
}
