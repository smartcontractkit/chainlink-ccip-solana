package lut

import (
	"errors"
	"fmt"

	"github.com/Masterminds/semver/v3"
	"github.com/gagliardetto/solana-go"

	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/sequences"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations"

	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
)

var Version *semver.Version = solcommon.Version2_0_0

// MaxEntriesPerExtend is how many addresses fit in one extend transaction. A Solana transaction is
// capped at 1232 bytes and each entry costs 32, so larger batches fail at send time with a size
// error that says nothing about lookup tables. Callers chunk to this and run one op per chunk.
const MaxEntriesPerExtend = 20

type ExtendParams struct {
	LookupTable solana.PublicKey
	// Entries are recorded verbatim in the operation report, so a run shows exactly which accounts
	// each chunk added to the table.
	Entries []solana.PublicKey
}

var Extend = operations.NewOperation(
	"lut:extend",
	Version,
	"Extends an address lookup table with the given entries",
	func(b operations.Bundle, chain cldf_solana.Chain, input ExtendParams) (sequences.OnChainOutput, error) {
		if input.LookupTable.IsZero() {
			return sequences.OnChainOutput{}, errors.New("lookup table address is required")
		}
		if len(input.Entries) == 0 {
			return sequences.OnChainOutput{}, errors.New("at least one lookup table entry is required")
		}
		if len(input.Entries) > MaxEntriesPerExtend {
			return sequences.OnChainOutput{}, fmt.Errorf(
				"%d entries exceeds the per-transaction maximum of %d, chunk before calling",
				len(input.Entries), MaxEntriesPerExtend)
		}
		if err := solcommon.ExtendLookupTable(chain, input.LookupTable, input.Entries); err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("extend lookup table %s: %w", input.LookupTable, err)
		}
		return sequences.OnChainOutput{}, nil
	},
)
