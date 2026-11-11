package testutils

import (
	"context"
	"encoding/binary"
	"regexp"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/ccip_router"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/rmn_remote"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
)

func DeriveSendAccounts(
	ctx context.Context,
	t *testing.T,
	authority solana.PrivateKey,
	message ccip_router.Svm2AnyMessage,
	destChainSelector uint64,
	solanaGoClient *rpc.Client,
	router solana.PublicKey,
) (accounts []*solana.AccountMeta, lookUpTables []solana.PublicKey, tokenIndices []uint8) {
	t.Helper()

	derivedAccounts := []*solana.AccountMeta{}
	askWith := []*solana.AccountMeta{}
	stage := "Start"
	tokenIndex := byte(0)
	routerConfigPDA, _, err := state.FindConfigPDA(router)
	require.NoError(t, err)
	re := regexp.MustCompile(`^TokenTransferStaticAccounts/\d+/0$`)
	for {
		params := ccip_router.DeriveAccountsCcipSendParams{
			DestChainSelector: destChainSelector,
			CcipSendCaller:    authority.PublicKey(),
			Message:           message,
		}

		deriveInst, err := ccip_router.NewDeriveAccountsCcipSendInstruction(
			params,
			stage,
			routerConfigPDA,
		)
		require.NoError(t, err)

		// Cast to *solana.GenericInstruction to append additional accounts
		genericInst, ok := deriveInst.(*solana.GenericInstruction)
		require.True(t, ok, "instruction must be *solana.GenericInstruction")
		genericInst.AccountValues = append(genericInst.AccountValues, askWith...)

		tx := SimulateTransaction(ctx, t, solanaGoClient, []solana.Instruction{genericInst}, authority.PublicKey())
		derivation, err := common.ExtractAnchorTypedReturnValue[ccip_router.DeriveAccountsResponse](ctx, tx.Value.Logs, router.String())
		if err != nil {
			t.Logf("Error deriving accounts for stage %s: %v", stage, err)
			for _, line := range tx.Value.Logs {
				t.Log(line)
			}
		}
		require.NoError(t, err)
		t.Logf("Derive stage: %s. Len: %d", derivation.CurrentStage, len(derivation.AccountsToSave))

		isStartOfToken := re.MatchString(derivation.CurrentStage)
		if isStartOfToken {
			// NewCcipSendInstruction has 18 required accounts
			tokenIndices = append(tokenIndices, tokenIndex-byte(18))
		}
		require.LessOrEqual(
			t,
			len(derivation.AccountsToSave),
			int(^byte(0))-int(tokenIndex),
			"too many derived accounts to fit in tokenIndex byte accumulator",
		)
		tokenIndex += byte(len(derivation.AccountsToSave)) //nolint:gosec // length validated by LessOrEqual above

		for _, meta := range derivation.AccountsToSave {
			derivedAccounts = append(derivedAccounts, &solana.AccountMeta{
				PublicKey:  meta.Pubkey,
				IsWritable: meta.IsWritable,
				IsSigner:   meta.IsSigner,
			})
		}
		askWith = make([]*solana.AccountMeta, 0, len(derivation.AskAgainWith))
		for _, meta := range derivation.AskAgainWith {
			askWith = append(askWith, &solana.AccountMeta{
				PublicKey:  meta.Pubkey,
				IsWritable: meta.IsWritable,
				IsSigner:   meta.IsSigner,
			})
		}
		lookUpTables = append(lookUpTables, derivation.LookUpTablesToSave...)

		if len(derivation.NextStage) == 0 {
			return derivedAccounts, lookUpTables, tokenIndices
		}
		stage = derivation.NextStage
	}
}

// BuildCcipSendInstruction creates a CcipSend instruction with the given parameters
// and replaces the generated accounts with derivedAccounts.
func BuildCcipSendInstruction(
	t *testing.T,
	destChainSelector uint64,
	message ccip_router.Svm2AnyMessage,
	tokenIndexes []byte,
	derivedAccounts []*solana.AccountMeta,
) solana.Instruction {
	t.Helper()
	var dummy solana.PublicKey
	ix, err := ccip_router.NewCcipSendInstruction(
		destChainSelector, message, tokenIndexes,
		dummy, dummy, dummy, dummy, dummy, dummy,
		dummy, dummy, dummy, dummy, dummy, dummy,
		dummy, dummy, dummy, dummy, dummy, dummy,
	)
	require.NoError(t, err)
	generic, ok := ix.(*solana.GenericInstruction)
	require.True(t, ok, "instruction must be *solana.GenericInstruction")
	generic.AccountValues = derivedAccounts
	return ix
}

// GlobalCurseSubject returns the RMN curse subject used for global curses.
func GlobalCurseSubject() rmn_remote.CurseSubject {
	return rmn_remote.CurseSubject{
		Value: [16]uint8{0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01},
	}
}

// ChainCurseSubject returns the RMN curse subject for a specific chain selector.
func ChainCurseSubject(chainSelector uint64) rmn_remote.CurseSubject {
	s := rmn_remote.CurseSubject{}
	binary.LittleEndian.PutUint64(s.Value[:], chainSelector)
	return s
}
