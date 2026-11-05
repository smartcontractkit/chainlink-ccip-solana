package utils

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"

	chainsel "github.com/smartcontractkit/chain-selectors"
	"github.com/smartcontractkit/chainlink-testing-framework/framework/components/blockchain"
)

// ResolveSolanaEVMLanes returns the single Solana selector and the EVM selector
// for this run. TARGET_EVM_SELECTOR picks the EVM chain when the config has more
// than one; without it, the config must have exactly one EVM chain.
func ResolveSolanaEVMLanes(blockchains []*blockchain.Input) (solSel, evmSel uint64, err error) {
	var solSels, evmSels []uint64
	for _, in := range blockchains {
		family, famErr := blockchain.TypeToFamily(in.Type)
		if famErr != nil {
			return 0, 0, famErr
		}
		details, detErr := chainsel.GetChainDetailsByChainIDAndFamily(in.ChainID, string(family))
		if detErr != nil {
			return 0, 0, fmt.Errorf("resolve %s chain %s: %w", family, in.ChainID, detErr)
		}
		switch family {
		case chainsel.FamilySolana:
			solSels = append(solSels, details.ChainSelector)
		case chainsel.FamilyEVM:
			evmSels = append(evmSels, details.ChainSelector)
		}
	}
	if len(solSels) != 1 {
		return 0, 0, fmt.Errorf("need exactly one Solana chain in config, got %v", solSels)
	}
	if len(evmSels) == 0 {
		return 0, 0, errors.New("need at least one EVM chain in config, got none")
	}
	if v := os.Getenv("TARGET_EVM_SELECTOR"); v != "" {
		sel, parseErr := strconv.ParseUint(v, 10, 64)
		if parseErr != nil {
			return 0, 0, fmt.Errorf("TARGET_EVM_SELECTOR %q is not a valid chain selector: %w; manifest EVM selectors: %v", v, parseErr, evmSels)
		}
		if !slices.Contains(evmSels, sel) {
			return 0, 0, fmt.Errorf("TARGET_EVM_SELECTOR %d is not in the manifest EVM selectors %v", sel, evmSels)
		}
		return solSels[0], sel, nil
	}
	if len(evmSels) > 1 {
		return 0, 0, fmt.Errorf("the config has several EVM chains %v; set TARGET_EVM_SELECTOR", evmSels)
	}
	return solSels[0], evmSels[0], nil
}
