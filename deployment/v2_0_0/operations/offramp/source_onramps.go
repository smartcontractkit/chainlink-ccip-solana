package offramp

import (
	"context"
	"fmt"

	"github.com/gagliardetto/solana-go"

	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/sequences"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/ccip_offramp"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
)

type SetSourceOnRampsParams struct {
	OffRamp             solana.PublicKey
	RemoteChainSelector uint64
	// SourceOnRamps is the complete set of OnRamps allowed for the remote chain. It replaces the
	// configured set; every other field of the source chain config is kept as is.
	SourceOnRamps [][]byte
}

// GetSourceChain reads the OffRamp source chain account for remoteChainSelector.
func GetSourceChain(chain cldf_solana.Chain, offRamp solana.PublicKey, remoteChainSelector uint64) (ccip_offramp.SourceChain, error) {
	var sourceChain ccip_offramp.SourceChain
	sourceChainPDA, _, err := state.FindOfframpSourceChainPDA(remoteChainSelector, offRamp)
	if err != nil {
		return sourceChain, fmt.Errorf("failed to derive source chain PDA for remote chain %d: %w", remoteChainSelector, err)
	}
	if err := common.GetAccountDataBorshIntoAnchor(context.Background(), chain.Client, sourceChainPDA, cldf_solana.SolDefaultCommitment, &sourceChain); err != nil {
		return sourceChain, fmt.Errorf("failed to read source chain account %s for remote chain %d: %w", sourceChainPDA, remoteChainSelector, err)
	}
	return sourceChain, nil
}

// SourceOnRampBytes returns the configured OnRamps of a source chain config as raw bytes.
func SourceOnRampBytes(cfg ccip_offramp.SourceChainConfig) [][]byte {
	out := make([][]byte, 0, len(cfg.OnRamps))
	for _, onRamp := range cfg.OnRamps {
		n := min(int(onRamp.Len), len(onRamp.Bytes))
		out = append(out, append([]byte(nil), onRamp.Bytes[:n]...))
	}
	return out
}

// ToOnRampAddresses converts raw OnRamp bytes into the OffRamp's fixed-size representation.
func ToOnRampAddresses(onRamps [][]byte) ([]ccip_offramp.OnRampAddress, error) {
	out := make([]ccip_offramp.OnRampAddress, 0, len(onRamps))
	for _, raw := range onRamps {
		var addr ccip_offramp.OnRampAddress
		if len(raw) == 0 || len(raw) > len(addr.Bytes) {
			return nil, fmt.Errorf("invalid on ramp address length: %d", len(raw))
		}
		copy(addr.Bytes[:], raw)
		addr.Len = uint32(len(raw)) //nolint:gosec // bounded by len(addr.Bytes) above
		out = append(out, addr)
	}
	return out, nil
}

var SetSourceOnRamps = operations.NewOperation(
	"off-ramp:set-source-onramps",
	Version,
	"Replaces the allowed source OnRamps of an existing source chain on the OffRamp contract",
	func(b operations.Bundle, chain cldf_solana.Chain, input SetSourceOnRampsParams) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&ccip_offramp.ProgramID, ccip_offramp.ParseInstructionWithoutAccounts, input.OffRamp)
		if len(input.SourceOnRamps) == 0 {
			return sequences.OnChainOutput{}, fmt.Errorf("no source OnRamps given for remote chain %d: the OffRamp requires at least one", input.RemoteChainSelector)
		}
		current, err := GetSourceChain(chain, input.OffRamp, input.RemoteChainSelector)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("source chain %d must be connected before its OnRamps can be set: %w", input.RemoteChainSelector, err)
		}
		onRamps, err := ToOnRampAddresses(input.SourceOnRamps)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		cfg := current.Config
		cfg.OnRamps = onRamps

		authority, err := GetAuthority(b.GetContext(), chain, input.OffRamp)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		configPDA, _, _ := state.FindOfframpConfigPDA(input.OffRamp)
		sourceChainPDA, _, _ := state.FindOfframpSourceChainPDA(input.RemoteChainSelector, input.OffRamp)
		eventAuthorityPDA, _, _ := state.FindEventAuthorityPDA(input.OffRamp)
		ixn, err := ccip_offramp.NewUpdateSourceChainConfigInstruction(
			ccip_offramp.UpdateSourceChainParams{
				SourceChainSelector: input.RemoteChainSelector,
				SourceChainConfig:   cfg,
			},
			sourceChainPDA,
			configPDA,
			authority,
			solana.SystemProgramID,
			eventAuthorityPDA,
			input.OffRamp,
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to build update source chain config instruction: %w", err)
		}

		return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{ixn}, input.OffRamp, ContractType.String())
	},
)
