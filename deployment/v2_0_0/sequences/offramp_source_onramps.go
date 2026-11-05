package sequences

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/gagliardetto/solana-go"

	ccvadapters "github.com/smartcontractkit/chainlink-ccip/deployment/v2_0_0/adapters"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	cldf "github.com/smartcontractkit/chainlink-deployments-framework/deployment"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations"
	mcms_types "github.com/smartcontractkit/mcms/types"

	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/offramp"
)

var (
	_ ccvadapters.OffRampSourceOnRampSetter = (*SolanaAdapter)(nil)
	_ ccvadapters.OffRampSourceOnRampReader = (*SolanaAdapter)(nil)
)

// GetOffRampSourceOnRamps returns the OnRamps the local OffRamp accepts from sourceChainSelector.
func (a *SolanaAdapter) GetOffRampSourceOnRamps(e cldf.Environment, localChainSelector uint64, sourceChainSelector uint64) ([][]byte, error) {
	chain, offRamp, err := a.offRampForChain(e, localChainSelector)
	if err != nil {
		return nil, err
	}
	sourceChain, err := offramp.GetSourceChain(chain, offRamp, sourceChainSelector)
	if err != nil {
		return nil, err
	}
	return offramp.SourceOnRampBytes(sourceChain.Config), nil
}

// SetOffRampSourceOnRamps replaces the OnRamps the local OffRamp accepts from the source chain.
// It returns skipped=true when the configured set already matches, or when the deployer key owns
// the OffRamp and the update was executed directly, so there is no batch operation to propose.
func (a *SolanaAdapter) SetOffRampSourceOnRamps(e cldf.Environment, update ccvadapters.OffRampSetSourceOnRampsEntry) (*mcms_types.BatchOperation, bool, error) {
	desired, err := parseSourceOnRamps(update.OnRamps)
	if err != nil {
		return nil, false, err
	}
	current, err := a.GetOffRampSourceOnRamps(e, update.LocalChainSelector, update.SourceChainSelector)
	if err != nil {
		return nil, false, err
	}
	if sameOnRampSet(current, desired) {
		return nil, true, nil
	}

	chain, offRamp, err := a.offRampForChain(e, update.LocalChainSelector)
	if err != nil {
		return nil, false, err
	}
	out, err := operations.ExecuteOperation(e.OperationsBundle, offramp.SetSourceOnRamps, chain, offramp.SetSourceOnRampsParams{
		OffRamp:             offRamp,
		RemoteChainSelector: update.SourceChainSelector,
		SourceOnRamps:       desired,
	})
	if err != nil {
		return nil, false, fmt.Errorf("failed to set source OnRamps for source chain %d on chain %d: %w", update.SourceChainSelector, update.LocalChainSelector, err)
	}
	if len(out.Output.BatchOps) == 0 {
		return nil, true, nil
	}
	return &out.Output.BatchOps[0], false, nil
}

func (a *SolanaAdapter) offRampForChain(e cldf.Environment, chainSelector uint64) (cldf_solana.Chain, solana.PublicKey, error) {
	chain, ok := e.BlockChains.SolanaChains()[chainSelector]
	if !ok {
		return cldf_solana.Chain{}, solana.PublicKey{}, fmt.Errorf("solana chain %d not found in environment", chainSelector)
	}
	addr, err := a.GetOffRampAddress(e.DataStore, chainSelector)
	if err != nil {
		return cldf_solana.Chain{}, solana.PublicKey{}, fmt.Errorf("failed to find OffRamp on chain %d: %w", chainSelector, err)
	}
	return chain, solana.PublicKeyFromBytes(addr), nil
}

// parseSourceOnRamps decodes 0x-prefixed hex OnRamps, dropping duplicates.
func parseSourceOnRamps(addrs []string) ([][]byte, error) {
	out := make([][]byte, 0, len(addrs))
	seen := make(map[string]struct{}, len(addrs))
	for i, addr := range addrs {
		trimmed := strings.TrimSpace(addr)
		if !strings.HasPrefix(trimmed, "0x") && !strings.HasPrefix(trimmed, "0X") {
			return nil, fmt.Errorf("onRamps[%d]: address %q must be 0x-prefixed hex", i, addr)
		}
		raw, err := hex.DecodeString(trimmed[2:])
		if err != nil {
			return nil, fmt.Errorf("onRamps[%d]: invalid hex address %q: %w", i, addr, err)
		}
		if len(raw) == 0 || len(raw) > math.MaxUint8 {
			return nil, fmt.Errorf("onRamps[%d]: address %q must be 1-%d bytes, got %d", i, addr, math.MaxUint8, len(raw))
		}
		if _, ok := seen[string(raw)]; ok {
			continue
		}
		seen[string(raw)] = struct{}{}
		out = append(out, raw)
	}
	if len(out) == 0 {
		return nil, errors.New("no valid onRamp addresses after parsing")
	}
	return out, nil
}

// sameOnRampSet reports whether a and b hold the same OnRamps, ignoring order.
func sameOnRampSet(a, b [][]byte) bool {
	if len(a) != len(b) {
		return false
	}
	for _, x := range a {
		found := false
		for _, y := range b {
			if bytes.Equal(x, y) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
