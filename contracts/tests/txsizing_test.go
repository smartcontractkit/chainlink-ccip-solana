package contracts

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/ccip_router"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/config"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
)

func mustRandomPubkey() solana.PublicKey {
	k, err := solana.NewRandomPrivateKey()
	if err != nil {
		panic(err)
	}
	return k.PublicKey()
}

const MaxSolanaTxSize = 1300

type failOnExcessTxSize func(tables map[solana.PublicKey]solana.PublicKeySlice) bool

func failOnExcessAlways(_ map[solana.PublicKey]solana.PublicKeySlice) bool {
	return true
}

func failOnExcessOnlyWithTables(tables map[solana.PublicKey]solana.PublicKeySlice) bool {
	return len(tables) > 0
}

func toTableEntries(t any) solana.PublicKeySlice {
	fields := reflect.VisibleFields(reflect.TypeOf(t))
	entries := make(solana.PublicKeySlice, 0, len(fields))
	for _, field := range fields {
		if field.Type == reflect.TypeFor[solana.PublicKey]() {
			entries = append(entries, reflect.ValueOf(t).FieldByName(field.Name).Interface().(solana.PublicKey))
		}
	}
	return entries
}

type RouterTable struct {
	RouterConfig        solana.PublicKey
	SystemProgram       solana.PublicKey
	BillingTokenProgram solana.PublicKey
	RouterBillingSigner solana.PublicKey
	SysVarInstruction   solana.PublicKey
	FqBillingSinger     solana.PublicKey
	FeeQuoterProgram    solana.PublicKey
	FqConfigPDA         solana.PublicKey
	FqLinkConfig        solana.PublicKey
	RmnProgram          solana.PublicKey
	RmnCurses           solana.PublicKey
	RmnConfig           solana.PublicKey
}

type OfframpTable struct {
	Config                solana.PublicKey
	ReferenceAddresses    solana.PublicKey
	SysVarInstruction     solana.PublicKey
	SystemProgram         solana.PublicKey
	BillingSinger         solana.PublicKey
	FeeQuoterProgram      solana.PublicKey
	FqConfig              solana.PublicKey
	Offramp               solana.PublicKey
	FqAllowedPriceUpdater solana.PublicKey
	RmnProgram            solana.PublicKey
	RmnCurses             solana.PublicKey
	RmnConfig             solana.PublicKey
}

type TokenTable struct {
	TokenAdminRegistryPDA solana.PublicKey
	PoolLookupTable       solana.PublicKey
	PoolProgram           solana.PublicKey
	PoolConfig            solana.PublicKey
	PoolTokenAccount      solana.PublicKey
	PoolSigner            solana.PublicKey
	TokenProgram          solana.PublicKey
	Mint                  solana.PublicKey
	RouterSigner          solana.PublicKey
}

// NOTE: this test does not execute or validate transaction inputs, it simply builds transactions to calculate the size of each transaction with signers
func TestTransactionSizing(t *testing.T) {
	auth, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)

	// mocked router lookup table for constant accounts
	// chain specific configs are not constant but are a small set relative to the number of users
	routerTable := RouterTable{
		RouterConfig:        mustRandomPubkey(),
		SystemProgram:       solana.SystemProgramID,
		BillingTokenProgram: solana.TokenProgramID,
		RouterBillingSigner: mustRandomPubkey(),
		SysVarInstruction:   solana.SysVarInstructionsPubkey,
		FqBillingSinger:     mustRandomPubkey(),
		FeeQuoterProgram:    mustRandomPubkey(),
		FqConfigPDA:         mustRandomPubkey(),
		FqLinkConfig:        mustRandomPubkey(),
		RmnProgram:          config.RMNRemoteProgram,
		RmnCurses:           config.RMNRemoteCursesPDA,
		RmnConfig:           config.RMNRemoteConfigPDA,
	}

	// offrampTable := OfframpTable{
	// 	Config:                mustRandomPubkey(),
	// 	ReferenceAddresses:    mustRandomPubkey(),
	// 	SysVarInstruction:     solana.SysVarInstructionsPubkey,
	// 	SystemProgram:         solana.SystemProgramID,
	// 	BillingSinger:         mustRandomPubkey(),
	// 	FeeQuoterProgram:      mustRandomPubkey(),
	// 	Offramp:               config.CcipOfframpProgram,
	// 	FqAllowedPriceUpdater: mustRandomPubkey(),
	// 	FqConfig:              mustRandomPubkey(),
	// 	RmnProgram:            config.RMNRemoteProgram,
	// 	RmnCurses:             config.RMNRemoteCursesPDA,
	// 	RmnConfig:             config.RMNRemoteConfigPDA,
	// }

	tokenTable := TokenTable{
		TokenAdminRegistryPDA: mustRandomPubkey(),
		PoolLookupTable:       mustRandomPubkey(),
		PoolProgram:           config.CcipBurnMintTokenPoolProgram,
		PoolConfig:            mustRandomPubkey(),
		PoolTokenAccount:      mustRandomPubkey(),
		PoolSigner:            mustRandomPubkey(),
		TokenProgram:          config.Token2022Program,
		Mint:                  mustRandomPubkey(),
		RouterSigner:          mustRandomPubkey(),
	}

	run := func(name string, failOnExcessPredicate failOnExcessTxSize, ix solana.Instruction, tables map[solana.PublicKey]solana.PublicKeySlice, opts ...common.TxModifier) string {
		tx, err := solana.NewTransaction([]solana.Instruction{ix}, solana.Hash{1}, solana.TransactionAddressTables(tables))
		require.NoError(t, err)

		for _, o := range opts {
			require.NoError(t, o(tx, nil))
		}

		_, err = tx.Sign(func(_ solana.PublicKey) *solana.PrivateKey {
			return &auth
		})
		require.NoError(t, err)

		bz, err := tx.MarshalBinary()
		require.NoError(t, err)
		l := len(bz)
		if failOnExcessPredicate(tables) {
			require.LessOrEqual(t, l, MaxSolanaTxSize, name)
		}
		remaining := MaxSolanaTxSize - l
		var warning string
		if remaining < 0 {
			warning = "<<< WARNING!!"
		}
		return fmt.Sprintf("%-55s: %-4d - remaining: %d %s", name, l, remaining, warning)
	}

	// ccipSend test messages + instruction ---------------------------------
	sendNoTokens := ccip_router.Svm2AnyMessage{
		Receiver:     make([]byte, 20), // EVM address
		Data:         []byte{},
		TokenAmounts: []ccip_router.SvmTokenAmount{}, // no tokens
		FeeToken:     [32]byte{},                     // solana fee token
		ExtraArgs:    []byte{},                       // default options
	}
	sendSingleMinimalToken := ccip_router.Svm2AnyMessage{
		Receiver: make([]byte, 20),
		Data:     []byte{},
		TokenAmounts: []ccip_router.SvmTokenAmount{{
			Token:  [32]byte{},
			Amount: 0,
		}}, // one token
		FeeToken:  [32]byte{},
		ExtraArgs: []byte{}, // default options
	}
	ixCcipSend := func(msg ccip_router.Svm2AnyMessage, tokenIndexes []byte, addAccounts solana.PublicKeySlice) solana.Instruction {
		inst, err := ccip_router.NewCcipSendInstruction(
			1,
			msg,
			tokenIndexes,
			routerTable.RouterConfig,
			mustRandomPubkey(), // destChainConfig,
			mustRandomPubkey(), // user nonce PDA
			auth.PublicKey(),   // sender/authority
			routerTable.SystemProgram,
			routerTable.BillingTokenProgram, // fee token program
			routerTable.BillingTokenProgram, // fee token mint
			mustRandomPubkey(),              // fee token user ATA
			mustRandomPubkey(),              // fee token receiver
			routerTable.RouterBillingSigner, // fee billing signer
			routerTable.FeeQuoterProgram,    // fee quoter
			routerTable.FqConfigPDA,         // fee quoter config
			mustRandomPubkey(),              // fee quoter dest chain
			mustRandomPubkey(),              // fee quoter billing token config
			routerTable.FqLinkConfig,        // fee quoter link token config
			routerTable.RmnProgram,
			routerTable.RmnCurses,
			routerTable.RmnConfig,
		)
		require.NoError(t, err)

		// Cast to *solana.GenericInstruction to append additional accounts
		genericInst, ok := inst.(*solana.GenericInstruction)
		require.True(t, ok, "instruction must be *solana.GenericInstruction")

		for _, v := range addAccounts {
			genericInst.AccountValues = append(genericInst.AccountValues, solana.Meta(v))
		}
		return inst
	}

	// runner ---------------------------------------------------------
	params := []struct {
		name           string
		ix             solana.Instruction
		tables         map[solana.PublicKey]solana.PublicKeySlice
		allowPredicate failOnExcessTxSize
	}{
		{
			"ccipSend:noToken",
			ixCcipSend(sendNoTokens, []byte{}, nil),
			map[solana.PublicKey]solana.PublicKeySlice{
				mustRandomPubkey(): toTableEntries(routerTable),
			},
			failOnExcessAlways,
		},
		{
			"ccipSend:singleToken",
			ixCcipSend(sendSingleMinimalToken, []byte{0}, append([]solana.PublicKey{
				mustRandomPubkey(), // user ATA
				mustRandomPubkey(), // token billing config
				mustRandomPubkey(), // token pool chain config
			}, toTableEntries(tokenTable)...)),
			map[solana.PublicKey]solana.PublicKeySlice{
				mustRandomPubkey():         toTableEntries(routerTable),
				tokenTable.PoolLookupTable: toTableEntries(tokenTable),
			},
			failOnExcessOnlyWithTables, // without lookup tables, we already know it exceeds the max tx size
		},
	}

	divider := strings.Repeat("-", 78)
	outputs := make([]string, 0, 2+10*len(params))
	outputs = append(outputs, "TX SIZE ANALYSIS", divider)
	for _, p := range params {
		for _, l := range []string{"", " +lookupTable"} {
			var tables map[solana.PublicKey]solana.PublicKeySlice
			if strings.Contains(l, "+lookupTable") {
				tables = p.tables
			}

			outputs = append(outputs,
				run(p.name+l, p.allowPredicate, p.ix, tables),
				run(p.name+l+" +cuLimit", p.allowPredicate, p.ix, tables, common.AddComputeUnitLimit(0)),
				run(p.name+l+" +cuPrice", p.allowPredicate, p.ix, tables, common.AddComputeUnitPrice(0)),
				run(p.name+l+" +cuPrice +cuLimit", p.allowPredicate, p.ix, tables, common.AddComputeUnitLimit(0), common.AddComputeUnitPrice(0)),
				divider,
			)
		}
	}
	t.Logf("\n%s\n", strings.Join(outputs, "\n"))
}
