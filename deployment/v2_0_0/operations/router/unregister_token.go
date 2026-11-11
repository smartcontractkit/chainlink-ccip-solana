package router

import (
	"errors"
	"fmt"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/sequences"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/ccip_router"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
)

type UnregisterTokenParams struct {
	Router    solana.PublicKey
	TokenMint solana.PublicKey
}

// UnregisterToken clears a token's pool in the TokenAdminRegistry by setting its pool lookup table
// to the zero pubkey. It signs as the token's registry administrator, or the router authority when
// no administrator is set. A token that is not registered, or has no pool, is a no-op.
var UnregisterToken = operations.NewOperation(
	"router:unregister-token",
	Version,
	"Unregisters a token from the Router TokenAdminRegistry",
	func(b operations.Bundle, chain cldf_solana.Chain, input UnregisterTokenParams) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&ccip_router.ProgramID, ccip_router.ParseInstructionWithoutAccounts, input.Router)
		tarPDA, _, err := state.FindTokenAdminRegistryPDA(input.TokenMint, input.Router)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive token admin registry PDA: %w", err)
		}
		tar, err := loadTokenAdminRegistry(b, chain, input.Router, input.TokenMint)
		if errors.Is(err, rpc.ErrNotFound) {
			b.Logger.Infow("Token not registered, nothing to unregister", "mint", input.TokenMint.String())
			return sequences.OnChainOutput{}, nil
		}
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to read token admin registry for mint %s: %w", input.TokenMint, err)
		}
		if tar.LookupTable.IsZero() {
			b.Logger.Infow("Token has no pool, nothing to unregister", "mint", input.TokenMint.String())
			return sequences.OnChainOutput{}, nil
		}

		currentAdmin, err := GetAuthority(b.GetContext(), chain, input.Router)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		if !tar.Administrator.IsZero() {
			currentAdmin = tar.Administrator
		}
		routerConfigPDA, _, err := state.FindConfigPDA(input.Router)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to derive router config PDA: %w", err)
		}
		raw, err := ccip_router.NewSetPoolInstruction([]byte{}, routerConfigPDA, tarPDA, input.TokenMint, solana.PublicKey{}, currentAdmin)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to build router set pool instruction: %w", err)
		}
		ixData, err := raw.Data()
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to extract data payload from router set pool instruction: %w", err)
		}
		instruction := solana.NewInstruction(input.Router, raw.Accounts(), ixData)

		return solcommon.ConfirmOrBatch(chain, currentAdmin, []solana.Instruction{instruction}, input.Router, ContractType.String())
	},
)
