package contracts

import (
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/burnmint_token_pool"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/ccip_offramp"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/ccip_router"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/committee_verifier"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/executor"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/fee_quoter"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/lockrelease_token_pool"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/rmn_remote"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/config"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/testutils"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
)

func TestProgramTypeVersions(t *testing.T) {
	ctx := t.Context()
	admin := testutils.Admin
	client := testutils.DeployAllPrograms(t, testutils.PathToAnchorConfig, admin)
	require.NoError(t, testutils.FundAccounts(ctx, []solana.PrivateKey{admin}, client))

	routerIx, err := ccip_router.NewTypeVersionInstruction(solana.SysVarClockPubkey)
	require.NoError(t, err)
	feeQuoterIx, err := fee_quoter.NewTypeVersionInstruction(solana.SysVarClockPubkey)
	require.NoError(t, err)
	offrampIx, err := ccip_offramp.NewTypeVersionInstruction()
	require.NoError(t, err)
	rmnIx, err := rmn_remote.NewTypeVersionInstruction(solana.SysVarClockPubkey)
	require.NoError(t, err)
	committeeVerifierIx, err := committee_verifier.NewTypeVersionInstruction(solana.SysVarClockPubkey)
	require.NoError(t, err)
	executorIx, err := executor.NewTypeVersionInstruction(solana.SysVarClockPubkey)
	require.NoError(t, err)
	burnmintIx, err := burnmint_token_pool.NewTypeVersionInstruction()
	require.NoError(t, err)
	lockreleaseIx, err := lockrelease_token_pool.NewTypeVersionInstruction()
	require.NoError(t, err)

	testcases := []struct {
		contractName string
		program      solana.PublicKey
		instruction  solana.Instruction
	}{
		{"ccip-router", config.CcipRouterProgram, routerIx},
		{"fee-quoter", config.FeeQuoterProgram, feeQuoterIx},
		{"ccip-offramp", config.CcipOfframpProgram, offrampIx},
		{"rmn-remote", config.RMNRemoteProgram, rmnIx},
		{"committee-verifier", config.CommitteeVerifierProgram, committeeVerifierIx},
		{"executor", config.ExecutorProgram, executorIx},
		{"burnmint-token-pool", config.CcipBurnMintTokenPoolProgram, burnmintIx},
		{"lockrelease-token-pool", config.CcipBasePoolLockRelease, lockreleaseIx},
	}

	semverRegex := "(0|[1-9]\\d*)\\.(0|[1-9]\\d*)\\.(0|[1-9]\\d*)(?:-((?:0|[1-9]\\d*|\\d*[a-zA-Z-][0-9a-zA-Z-]*)(?:\\.(?:0|[1-9]\\d*|\\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?(?:\\+([0-9a-zA-Z-]+(?:\\.[0-9a-zA-Z-]+)*))?"
	for _, tc := range testcases {
		t.Run(tc.contractName, func(t *testing.T) {
			result := testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{tc.instruction}, admin, config.DefaultCommitment)
			require.NotNil(t, result)

			output, err := common.ExtractTypedReturnValue(ctx, result.Meta.LogMessages, tc.program.String(), func(b []byte) string {
				require.Len(t, b, int(binary.LittleEndian.Uint32(b[:4]))+4)
				return string(b[4:])
			})
			require.NoError(t, err)
			require.Regexp(t, fmt.Sprintf("^%s %s$", tc.contractName, semverRegex), output)
		})
	}
}
