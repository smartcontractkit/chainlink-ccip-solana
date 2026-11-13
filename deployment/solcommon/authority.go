package solcommon

import (
	"context"
	"fmt"

	"github.com/gagliardetto/solana-go"

	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/sequences"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	"github.com/smartcontractkit/mcms/types"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
)

// ConfirmOrBatch sends ixns with the deployer key when signer is the deployer. Otherwise it
// returns them as an MCMS batch operation, for signer (typically the timelock) to execute.
// No instructions is a no-op: nothing is sent and no batch is returned, so an operation that finds
// its on-chain state already up to date can pass an empty list.
func ConfirmOrBatch(chain cldf_solana.Chain, signer solana.PublicKey, ixns []solana.Instruction, program solana.PublicKey, contractType string) (sequences.OnChainOutput, error) {
	if len(ixns) == 0 {
		return sequences.OnChainOutput{}, nil
	}
	if signer != chain.Deployer().PublicKey() {
		batch, err := BuildMCMSBatchOperation(chain.Selector, ixns, program.String(), contractType)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to build MCMS batch for %s instructions on %s: %w", contractType, program, err)
		}
		return sequences.OnChainOutput{BatchOps: []types.BatchOperation{batch}}, nil
	}
	if err := chain.Confirm(ixns); err != nil {
		return sequences.OnChainOutput{}, fmt.Errorf("failed to confirm %s instructions on %s: %w", contractType, program, err)
	}
	return sequences.OnChainOutput{}, nil
}

// InitializerAuthority returns the program's upgrade authority, the only key its initialize
// instruction accepts as signer. Pass it to ConfirmOrBatch so a timelock-owned program is
// initialized through MCMS.
func InitializerAuthority(chain cldf_solana.Chain, program solana.PublicKey) (solana.PublicKey, error) {
	authority, err := GetUpgradeAuthority(chain.Client, program)
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("resolve upgrade authority of %s: %w", program, err)
	}
	if authority.IsZero() {
		return solana.PublicKey{}, fmt.Errorf("program %s is immutable, so nobody can initialize it", program)
	}
	return authority, nil
}

// ConfigAuthority reads the config account of program at configPDA with parser and returns the
// key that pick selects from it. Before the program is initialized the deployer signs, so a
// missing config returns the deployer key. Read failures are returned as errors.
func ConfigAuthority[T any](ctx context.Context, chain cldf_solana.Chain, program, configPDA solana.PublicKey, parser func([]byte) (*T, error), pick func(*T) solana.PublicKey) (solana.PublicKey, error) {
	config, err := common.GetInitializedAccount(ctx, chain.Client, configPDA, program, cldf_solana.SolDefaultCommitment, parser)
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("failed to read config of program %s: %w", program, err)
	}
	if config == nil {
		return chain.Deployer().PublicKey(), nil
	}
	return pick(config), nil
}
