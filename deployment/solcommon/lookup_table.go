package solcommon

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/gagliardetto/solana-go"
	addresslookuptable "github.com/gagliardetto/solana-go/programs/address-lookup-table"
	"github.com/gagliardetto/solana-go/rpc"

	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/ccip_offramp"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
)

// ExtendOfframpLookupTable resolves the offramp's on-chain lookup table and idempotently
// extends it with any entries that are not already present.
func ExtendOfframpLookupTable(chain cldf_solana.Chain, offRampID solana.PublicKey, lookUpTableEntries []solana.PublicKey) error {
	table, err := OfframpLookupTable(chain, offRampID)
	if err != nil {
		return err
	}
	return ExtendLookupTable(chain, table, lookUpTableEntries)
}

func OfframpLookupTable(chain cldf_solana.Chain, offRampID solana.PublicKey) (solana.PublicKey, error) {
	var referenceAddressesAccount ccip_offramp.ReferenceAddresses
	offRampReferenceAddressesPDA, _, _ := state.FindOfframpReferenceAddressesPDA(offRampID)
	// Anchor stores an 8-byte discriminator before the Borsh payload.
	// The CLDF helper doesn't seem to strip it.
	err := common.GetAccountDataBorshIntoAnchor(
		context.Background(),
		chain.Client,
		offRampReferenceAddressesPDA,
		cldf_solana.SolDefaultCommitment,
		&referenceAddressesAccount,
	)
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("failed to get offramp reference addresses: %w", err)
	}
	return referenceAddressesAccount.OfframpLookupTable, nil
}

// ReadLookupTable validates the account before exposing its authority and entries.
func ReadLookupTable(ctx context.Context, chain cldf_solana.Chain, table solana.PublicKey) (*addresslookuptable.AddressLookupTableState, error) {
	account, err := chain.Client.GetAccountInfoWithOpts(ctx, table, &rpc.GetAccountInfoOpts{Commitment: rpc.CommitmentConfirmed})
	if err != nil {
		return nil, fmt.Errorf("read lookup table %s: %w", table, err)
	}
	if account == nil || account.Value == nil || account.Value.Owner != common.AddressLookupTableProgram {
		return nil, fmt.Errorf("%s is not an address lookup table", table)
	}
	state, err := addresslookuptable.DecodeAddressLookupTableState(account.GetBinary())
	if err != nil {
		return nil, fmt.Errorf("decode lookup table %s: %w", table, err)
	}
	if state.TypeIndex != 1 {
		return nil, fmt.Errorf("lookup table %s is uninitialized", table)
	}
	if state.DeactivationSlot != math.MaxUint64 {
		return nil, fmt.Errorf("lookup table %s is deactivated", table)
	}
	return state, nil
}

// ExtendLookupTable idempotently extends a deployer-owned lookup table.
func ExtendLookupTable(chain cldf_solana.Chain, table solana.PublicKey, entries []solana.PublicKey) error {
	if chain.DeployerKey == nil {
		return errors.New("missing Solana deployer key")
	}
	state, err := ReadLookupTable(context.Background(), chain, table)
	if err != nil {
		return err
	}
	seen := make(map[solana.PublicKey]bool, len(state.Addresses))
	for _, key := range state.Addresses {
		seen[key] = true
	}
	var missing []solana.PublicKey
	for _, key := range entries {
		if !seen[key] {
			missing = append(missing, key)
			seen[key] = true
		}
	}
	if len(missing) == 0 {
		return nil
	}
	if len(state.Addresses)+len(missing) > 256 {
		return fmt.Errorf("lookup table %s exceeds 256 entries", table)
	}
	if state.Authority == nil || *state.Authority != chain.DeployerKey.PublicKey() {
		return fmt.Errorf("lookup table %s is not owned by the deployer", table)
	}
	for start := 0; start < len(missing); start += 20 {
		ix := common.NewExtendLookupTableInstruction(table, chain.DeployerKey.PublicKey(), chain.DeployerKey.PublicKey(), missing[start:min(start+20, len(missing))])
		if err := chain.Confirm([]solana.Instruction{ix}); err != nil {
			return fmt.Errorf("extend lookup table %s: %w", table, err)
		}
	}
	return nil
}

// PreparedLookupTable reuses a recorded table or creates one with the deployer.
func PreparedLookupTable(ctx context.Context, chain cldf_solana.Chain, table solana.PublicKey) (solana.PublicKey, error) {
	if chain.DeployerKey == nil {
		return solana.PublicKey{}, errors.New("missing Solana deployer key")
	}
	if !table.IsZero() {
		state, err := ReadLookupTable(ctx, chain, table)
		if err != nil {
			return solana.PublicKey{}, err
		}
		if state.Authority == nil || *state.Authority != chain.DeployerKey.PublicKey() {
			return solana.PublicKey{}, fmt.Errorf("lookup table %s is not owned by the deployer", table)
		}
		return table, nil
	}
	return common.CreateLookupTable(ctx, chain.Client, *chain.DeployerKey)
}
