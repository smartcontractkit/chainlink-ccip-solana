package solcommon

import (
	"encoding/json"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/system"
	"github.com/stretchr/testify/require"
)

func TestBuildMCMSBatchOperationGolden(t *testing.T) {
	t.Parallel()
	from := solana.MustPublicKeyFromBase58("DoajfR5tK24xVw51fWcawUZWhAXD8yrBJVacc13neVQA")
	to := solana.MustPublicKeyFromBase58("11111111111111111111111111111112")
	ix, err := system.NewTransferInstruction(5, from, to).ValidateAndBuild()
	require.NoError(t, err)

	op, err := BuildMCMSBatchOperation(
		124615329519749607,
		[]solana.Instruction{ix, ix},
		"11111111111111111111111111111111",
		"Router",
	)
	require.NoError(t, err)
	got, err := json.Marshal(op)
	require.NoError(t, err)
	require.JSONEq(t, goldenBatchOperation, string(got))

	empty, err := BuildMCMSBatchOperation(1, nil, "x", "y")
	require.NoError(t, err)
	require.Equal(t, uint64(1), uint64(empty.ChainSelector))
	require.NotNil(t, empty.Transactions)
	require.Empty(t, empty.Transactions)

	_, err = BuildMCMSBatchOperation(1, []solana.Instruction{ix}, "not-a-key", "Router")
	require.ErrorContains(t, err, "failed to create transaction")
}

// The strings are written to datastores and must not change.
func TestGovernanceContractTypesGolden(t *testing.T) {
	t.Parallel()
	require.Equal(t, "RBACTimelockProgram", string(TimelockProgramType))
	require.Equal(t, "ManyChainMultiSigProgram", string(McmProgramType))
	require.Equal(t, "AccessControllerProgram", string(AccessControllerProgramType))
	require.Equal(t, "ProposerSeed", string(ProposerSeed))
	require.Equal(t, "CancellerSeed", string(CancellerSeed))
	require.Equal(t, "BypasserSeed", string(BypasserSeed))
	require.Equal(t, "RBACTimelockSeed", string(RBACTimelockSeed))
	require.Equal(t, "ProposerAccessControllerAccount", string(ProposerAccessControllerAccount))
	require.Equal(t, "ExecutorAccessControllerAccount", string(ExecutorAccessControllerAccount))
	require.Equal(t, "CancellerAccessControllerAccount", string(CancellerAccessControllerAccount))
	require.Equal(t, "BypasserAccessControllerAccount", string(BypasserAccessControllerAccount))
}

const goldenBatchOperation = `{"chainSelector":124615329519749607,"transactions":[{"contractType":"Router","tags":[],"to":"11111111111111111111111111111111","data":"AgAAAAUAAAAAAAAA","additionalFields":{"accounts":[{"PublicKey":"DoajfR5tK24xVw51fWcawUZWhAXD8yrBJVacc13neVQA","IsWritable":true,"IsSigner":false},{"PublicKey":"11111111111111111111111111111112","IsWritable":true,"IsSigner":false}],"value":0}},{"contractType":"Router","tags":[],"to":"11111111111111111111111111111111","data":"AgAAAAUAAAAAAAAA","additionalFields":{"accounts":[{"PublicKey":"DoajfR5tK24xVw51fWcawUZWhAXD8yrBJVacc13neVQA","IsWritable":true,"IsSigner":false},{"PublicKey":"11111111111111111111111111111112","IsWritable":true,"IsSigner":false}],"value":0}}]}`
