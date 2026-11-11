package tokenpools

import (
	"context"
	"fmt"

	"github.com/gagliardetto/solana-go"

	common_utils "github.com/smartcontractkit/chainlink-ccip/deployment/utils"
	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/sequences"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/burnmint_token_pool"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/cctp_token_pool"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/lockrelease_token_pool"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/lombard_token_pool"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/tokens"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
)

type SetPoolRouterInput struct {
	TokenPool solana.PublicKey
	TokenMint solana.PublicKey
	Router    solana.PublicKey
}

// setPoolRouter repoints a pool at a router. The program only accepts set_router from a signer that
// is both the pool owner and the pool program's upgrade authority, so the two must match: the
// deployer key (executed directly) or one MCMS signer (batched).
func setPoolRouter(
	b operations.Bundle,
	chain cldf_solana.Chain,
	input SetPoolRouterInput,
	poolType string,
	getAuthority func(context.Context, cldf_solana.Chain, solana.PublicKey, solana.PublicKey) (solana.PublicKey, error),
	buildIx func(poolConfigPDA, authority, programData solana.PublicKey) (solana.Instruction, error),
) (sequences.OnChainOutput, error) {
	poolConfigPDA, err := tokens.TokenPoolConfigAddress(input.TokenMint, input.TokenPool)
	if err != nil {
		return sequences.OnChainOutput{}, fmt.Errorf("failed to derive token pool config address: %w", err)
	}
	// Every token pool program's State starts with version and BaseConfig, so the BurnMint binding
	// reads the router of any pool type.
	var state burnmint_token_pool.State
	if err = common.GetAccountDataBorshIntoAnchor(b.GetContext(), chain.Client, poolConfigPDA, cldf_solana.SolDefaultCommitment, &state); err != nil {
		return sequences.OnChainOutput{}, fmt.Errorf("failed to read pool config %s: %w", poolConfigPDA, err)
	}
	if state.Config.Router == input.Router {
		b.Logger.Infow("Token pool router already set, skipping", "tokenPool", input.TokenPool.String(), "mint", input.TokenMint.String())
		return sequences.OnChainOutput{}, nil
	}

	owner, err := getAuthority(b.GetContext(), chain, input.TokenPool, input.TokenMint)
	if err != nil {
		return sequences.OnChainOutput{}, fmt.Errorf("failed to get token pool owner: %w", err)
	}
	upgradeAuthority, err := solcommon.GetUpgradeAuthority(chain.Client, input.TokenPool)
	if err != nil {
		return sequences.OnChainOutput{}, fmt.Errorf("failed to get upgrade authority of token pool program %s: %w", input.TokenPool, err)
	}
	if owner != upgradeAuthority {
		return sequences.OnChainOutput{}, fmt.Errorf("set_router on pool %s requires the pool owner (%s) to be the program upgrade authority (%s)", input.TokenPool, owner, upgradeAuthority)
	}

	programData, _, err := solana.FindProgramAddress([][]byte{input.TokenPool.Bytes()}, solana.BPFLoaderUpgradeableProgramID)
	if err != nil {
		return sequences.OnChainOutput{}, fmt.Errorf("failed to derive program data address: %w", err)
	}
	ixn, err := buildIx(poolConfigPDA, owner, programData)
	if err != nil {
		return sequences.OnChainOutput{}, fmt.Errorf("failed to build set_router instruction: %w", err)
	}
	return solcommon.ConfirmOrBatch(chain, owner, []solana.Instruction{ixn}, input.TokenPool, poolType)
}

var SetRouterBurnMint = operations.NewOperation(
	"burnmint:set-router",
	solcommon.Version2_0_0,
	"Sets the router of a BurnMintTokenPool",
	func(b operations.Bundle, chain cldf_solana.Chain, input SetPoolRouterInput) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&burnmint_token_pool.ProgramID, burnmint_token_pool.ParseInstructionWithoutAccounts, input.TokenPool)
		return setPoolRouter(b, chain, input, common_utils.BurnMintTokenPool.String(), GetAuthorityBurnMint,
			func(poolConfigPDA, authority, programData solana.PublicKey) (solana.Instruction, error) {
				return burnmint_token_pool.NewSetRouterInstruction(input.Router, poolConfigPDA, input.TokenMint, authority, input.TokenPool, programData)
			},
		)
	},
)

var SetRouterLockRelease = operations.NewOperation(
	"lockrelease:set-router",
	solcommon.Version2_0_0,
	"Sets the router of a LockReleaseTokenPool",
	func(b operations.Bundle, chain cldf_solana.Chain, input SetPoolRouterInput) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&lockrelease_token_pool.ProgramID, lockrelease_token_pool.ParseInstructionWithoutAccounts, input.TokenPool)
		return setPoolRouter(b, chain, input, common_utils.LockReleaseTokenPool.String(), GetAuthorityLockRelease,
			func(poolConfigPDA, authority, programData solana.PublicKey) (solana.Instruction, error) {
				return lockrelease_token_pool.NewSetRouterInstruction(input.Router, poolConfigPDA, input.TokenMint, authority, input.TokenPool, programData)
			},
		)
	},
)

var SetRouterCctp = operations.NewOperation(
	"cctp:set-router",
	solcommon.Version2_0_0,
	"Sets the router of a CCTPTokenPool",
	func(b operations.Bundle, chain cldf_solana.Chain, input SetPoolRouterInput) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&cctp_token_pool.ProgramID, cctp_token_pool.ParseInstructionWithoutAccounts, input.TokenPool)
		return setPoolRouter(b, chain, input, common_utils.CCTPTokenPool.String(), GetAuthorityCctp,
			func(poolConfigPDA, authority, programData solana.PublicKey) (solana.Instruction, error) {
				return cctp_token_pool.NewSetRouterInstruction(input.Router, poolConfigPDA, input.TokenMint, authority, input.TokenPool, programData)
			},
		)
	},
)

var SetRouterLombard = operations.NewOperation(
	"lombard:set-router",
	solcommon.Version2_0_0,
	"Sets the router of a LombardTokenPool",
	func(b operations.Bundle, chain cldf_solana.Chain, input SetPoolRouterInput) (sequences.OnChainOutput, error) {
		solcommon.SetProgramID(&lombard_token_pool.ProgramID, lombard_token_pool.ParseInstructionWithoutAccounts, input.TokenPool)
		return setPoolRouter(b, chain, input, LombardTokenPool.String(), GetAuthorityLombard,
			func(poolConfigPDA, authority, programData solana.PublicKey) (solana.Instruction, error) {
				return lombard_token_pool.NewSetRouterInstruction(input.Router, poolConfigPDA, input.TokenMint, authority, input.TokenPool, programData)
			},
		)
	},
)
