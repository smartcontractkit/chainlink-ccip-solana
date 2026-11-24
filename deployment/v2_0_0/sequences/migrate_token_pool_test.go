package sequences

import (
	"testing"

	"github.com/Masterminds/semver/v3"
	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"

	chainsel "github.com/smartcontractkit/chain-selectors"
	common_utils "github.com/smartcontractkit/chainlink-ccip/deployment/utils"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/burnmint_token_pool"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/ccip_router"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/lockrelease_token_pool"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
)

func TestValidateMigratablePoolType(t *testing.T) {
	require.NoError(t, ValidateMigratablePoolType(common_utils.BurnMintTokenPool.String()))
	require.NoError(t, ValidateMigratablePoolType(common_utils.LockReleaseTokenPool.String()))
	require.ErrorContains(t, ValidateMigratablePoolType(common_utils.CCTPTokenPool.String()), "MigrateCctpTokenPoolToV2Changeset")
	require.ErrorContains(t, ValidateMigratablePoolType("LombardTokenPool"), "not supported")
	require.ErrorContains(t, ValidateMigratablePoolType(""), "not supported")
}

func TestResolveTokenPoolMigration(t *testing.T) {
	sel := chainsel.SOLANA_DEVNET.Selector
	other := chainsel.SOLANA_MAINNET.Selector
	bnm := common_utils.BurnMintTokenPool.String()
	lr := common_utils.LockReleaseTokenPool.String()
	v160 := semver.MustParse("1.6.0")
	v200 := semver.MustParse("2.0.0")
	v203 := semver.MustParse("2.0.3")

	// The shared BurnMint program and a separate one, registered under their 1.6 pool metadata,
	// plus the refs the in-place upgrade registers at the shared program.
	cll := solana.NewWallet().PublicKey()
	separate := solana.NewWallet().PublicKey()
	mint := solana.NewWallet().PublicKey()

	ref := func(selector uint64, typ string, version *semver.Version, qualifier string, address solana.PublicKey) datastore.AddressRef {
		return datastore.AddressRef{ChainSelector: selector, Type: datastore.ContractType(typ), Version: version, Qualifier: qualifier, Address: address.String()}
	}
	build := func(refs ...datastore.AddressRef) datastore.DataStore {
		ds := datastore.NewMemoryDataStore()
		for _, r := range refs {
			require.NoError(t, ds.Addresses().Add(r))
		}
		return ds.Seal()
	}
	legacy := []datastore.AddressRef{
		ref(sel, string(solcommon.SPL2022Tokens), v160, "CCIP-BnM", mint),
		ref(sel, bnm, v160, CLLPoolProgramQualifier, cll),
		ref(sel, bnm, v160, "pool-a", separate),
		// Another chain's refs are ignored.
		ref(other, bnm, v160, CLLPoolProgramQualifier, separate),
	}
	upgraded := append(append([]datastore.AddressRef{}, legacy...),
		ref(sel, bnm, v203, solcommon.PoolProgramQualifier, cll),
		ref(sel, bnm, v200, "pool-a-2", separate),
	)

	t.Run("CLL program by default, registered at the upgraded version under the token qualifier", func(t *testing.T) {
		for _, programQualifier := range []string{"", CLLPoolProgramQualifier} {
			target, err := ResolveTokenPoolMigration(build(upgraded...), sel, bnm, programQualifier, mint, "")
			require.NoError(t, err)
			require.Equal(t, cll, target.Program)
			require.Equal(t, "CCIP-BnM", target.Qualifier)
			require.True(t, target.Version.Equal(v203))
			require.True(t, target.RegisterRef)
		}
	})

	t.Run("CLL program registered without qualifier", func(t *testing.T) {
		refs := []datastore.AddressRef{
			ref(sel, string(solcommon.SPL2022Tokens), v160, "CCIP-BnM", mint),
			ref(sel, bnm, v160, "", cll),
			ref(sel, bnm, v200, solcommon.PoolProgramQualifier, cll),
		}
		target, err := ResolveTokenPoolMigration(build(refs...), sel, bnm, "", mint, "")
		require.NoError(t, err)
		require.Equal(t, cll, target.Program)
	})

	t.Run("separate program by its qualifier", func(t *testing.T) {
		target, err := ResolveTokenPoolMigration(build(upgraded...), sel, bnm, "pool-a", mint, "")
		require.NoError(t, err)
		require.Equal(t, separate, target.Program)
		require.True(t, target.Version.Equal(v200))
	})

	t.Run("explicit token qualifier skips the token lookup", func(t *testing.T) {
		target, err := ResolveTokenPoolMigration(build(upgraded...), sel, bnm, "", solana.NewWallet().PublicKey(), "OTHER")
		require.NoError(t, err)
		require.Equal(t, "OTHER", target.Qualifier)
	})

	t.Run("re-run does not register the ref again", func(t *testing.T) {
		rerun := append(append([]datastore.AddressRef{}, upgraded...), ref(sel, bnm, v203, "CCIP-BnM", cll))
		target, err := ResolveTokenPoolMigration(build(rerun...), sel, bnm, "", mint, "")
		require.NoError(t, err)
		require.False(t, target.RegisterRef)
	})

	t.Run("2.x instance ref at another program", func(t *testing.T) {
		refs := append(append([]datastore.AddressRef{}, upgraded...), ref(sel, bnm, v200, "CCIP-BnM", separate))
		_, err := ResolveTokenPoolMigration(build(refs...), sel, bnm, "", mint, "")
		require.ErrorContains(t, err, "not at pool program")
	})

	t.Run("program not upgraded", func(t *testing.T) {
		_, err := ResolveTokenPoolMigration(build(legacy...), sel, bnm, "", mint, "")
		require.ErrorContains(t, err, "has no 2.x ref")
		require.ErrorContains(t, err, "UpgradeProgramChangeset")
	})

	t.Run("a 2.x ref of another type at the address does not count as upgraded", func(t *testing.T) {
		refs := append(append([]datastore.AddressRef{}, legacy...), ref(sel, lr, v200, solcommon.PoolProgramQualifier, cll))
		_, err := ResolveTokenPoolMigration(build(refs...), sel, bnm, "", mint, "")
		require.ErrorContains(t, err, "has no 2.x ref")
	})

	t.Run("unknown program qualifier", func(t *testing.T) {
		_, err := ResolveTokenPoolMigration(build(upgraded...), sel, bnm, "nobody", mint, "")
		require.ErrorContains(t, err, `no BurnMintTokenPool program registered under qualifier "nobody"`)
	})

	t.Run("programs at two addresses under one qualifier are ambiguous", func(t *testing.T) {
		refs := append(append([]datastore.AddressRef{}, upgraded...), ref(sel, bnm, v200, "pool-a", cll))
		_, err := ResolveTokenPoolMigration(build(refs...), sel, bnm, "pool-a", mint, "")
		require.ErrorContains(t, err, "ambiguous")
	})

	t.Run("unsupported pool type", func(t *testing.T) {
		_, err := ResolveTokenPoolMigration(build(upgraded...), sel, common_utils.CCTPTokenPool.String(), "", mint, "")
		require.ErrorContains(t, err, "MigrateCctpTokenPoolToV2Changeset")
	})
}

func TestTokenQualifierForMint(t *testing.T) {
	sel := chainsel.SOLANA_DEVNET.Selector
	v160 := semver.MustParse("1.6.0")
	mint := solana.NewWallet().PublicKey()
	build := func(refs ...datastore.AddressRef) datastore.DataStore {
		ds := datastore.NewMemoryDataStore()
		for _, r := range refs {
			require.NoError(t, ds.Addresses().Add(r))
		}
		return ds.Seal()
	}
	token := func(typ datastore.ContractType, qualifier string) datastore.AddressRef {
		return datastore.AddressRef{ChainSelector: sel, Type: typ, Version: v160, Qualifier: qualifier, Address: mint.String()}
	}

	q, err := TokenQualifierForMint(build(token(datastore.ContractType(solcommon.SPL2022Tokens), "CCIP-BnM")), sel, mint)
	require.NoError(t, err)
	require.Equal(t, "CCIP-BnM", q)

	q, err = TokenQualifierForMint(build(
		token(datastore.ContractType(solcommon.SPLTokens), "TKN"),
		datastore.AddressRef{ChainSelector: sel, Type: datastore.ContractType(solcommon.SPLTokens), Version: semver.MustParse("2.0.0"), Qualifier: "TKN", Address: mint.String()},
	), sel, mint)
	require.NoError(t, err, "the same qualifier at several versions is one token")
	require.Equal(t, "TKN", q)

	_, err = TokenQualifierForMint(build(), sel, mint)
	require.ErrorContains(t, err, "no token ref")

	_, err = TokenQualifierForMint(build(token("SomethingElse", "X")), sel, mint)
	require.ErrorContains(t, err, "no token ref", "only token types count")

	_, err = TokenQualifierForMint(build(token(datastore.ContractType(solcommon.SPLTokens), "")), sel, mint)
	require.ErrorContains(t, err, "without qualifier")

	_, err = TokenQualifierForMint(build(
		token(datastore.ContractType(solcommon.SPLTokens), "A"),
		token(datastore.ContractType(solcommon.SPL2022Tokens), "B"),
	), sel, mint)
	require.ErrorContains(t, err, "several qualifiers")
}

func TestDeriveTokenAddressFromQualifier(t *testing.T) {
	sel := chainsel.SOLANA_DEVNET.Selector
	spl := solana.NewWallet().PublicKey().String()
	spl2022 := solana.NewWallet().PublicKey().String()
	build := func(refs ...datastore.AddressRef) datastore.DataStore {
		ds := datastore.NewMemoryDataStore()
		for _, r := range refs {
			require.NoError(t, ds.Addresses().Add(r))
		}
		return ds.Seal()
	}
	token := func(typ datastore.ContractType, version, qualifier, address string) datastore.AddressRef {
		return datastore.AddressRef{ChainSelector: sel, Type: typ, Version: semver.MustParse(version), Qualifier: qualifier, Address: address}
	}
	splType := datastore.ContractType(solcommon.SPLTokens)
	spl2022Type := datastore.ContractType(solcommon.SPL2022Tokens)

	addr, err := deriveTokenAddressFromQualifier(build(token(splType, "1.6.0", "TKN", spl)), sel, "TKN")
	require.NoError(t, err)
	require.Equal(t, spl, addr)

	addr, err = deriveTokenAddressFromQualifier(build(token(spl2022Type, "2.0.0", "CCIP-BnM", spl2022)), sel, "CCIP-BnM")
	require.NoError(t, err, "a Token-2022 mint is found too")
	require.Equal(t, spl2022, addr)

	addr, err = deriveTokenAddressFromQualifier(build(token(splType, "1.6.0", "X", spl), token(spl2022Type, "2.0.0", "X", spl)), sel, "X")
	require.NoError(t, err, "the same mint under both types is one token")
	require.Equal(t, spl, addr)

	_, err = deriveTokenAddressFromQualifier(build(token(splType, "1.6.0", "X", spl), token(spl2022Type, "2.0.0", "X", spl2022)), sel, "X")
	require.ErrorContains(t, err, "ambiguous")

	_, err = deriveTokenAddressFromQualifier(build(token(splType, "1.6.0", "OTHER", spl)), sel, "TKN")
	require.ErrorIs(t, err, solcommon.ErrAddressRefNotFound)
}

func TestTokenPoolMigrationChecksAuthority(t *testing.T) {
	sel := chainsel.SOLANA_DEVNET.Selector
	v160 := semver.MustParse("1.6.0")
	timelockProgram := solana.NewWallet().PublicKey()
	timelock := func(qualifier, seed string) datastore.AddressRef {
		return datastore.AddressRef{ChainSelector: sel, Type: datastore.ContractType(common_utils.RBACTimelock), Version: v160,
			Qualifier: qualifier, Address: timelockProgram.String() + "." + seed}
	}
	refs := []datastore.AddressRef{
		{ChainSelector: sel, Type: "Router", Version: semver.MustParse("2.1.0"), Address: solana.NewWallet().PublicKey().String()},
		{ChainSelector: sel, Type: "OffRamp", Version: semver.MustParse("2.0.0"), Address: solana.NewWallet().PublicKey().String()},
		timelock("CLLCCIP", "E4R6Nwg1K8Zvi6McLdkaGDD5ClX1KkyV"),
		timelock("RMNMCMS", "gEBh0VvLk7U6QyrQ6xOVMn25Uc0zg5ud"),
	}
	deployerKey := solana.NewWallet().PrivateKey
	chain := cldf_solana.Chain{Selector: sel, DeployerKey: &deployerKey}

	checks, err := NewTokenPoolMigrationChecks(refs, chain, sel, common_utils.BurnMintTokenPool.String(), "CLLCCIP")
	require.NoError(t, err)
	require.NotNil(t, checks.Timelock)
	var rmnSigner solana.PublicKey
	for signer, qualifier := range checks.OtherTimelocks {
		require.Equal(t, "RMNMCMS", qualifier)
		rmnSigner = signer
	}

	require.NoError(t, checks.requireControlled("pool", deployerKey.PublicKey()))
	require.NoError(t, checks.requireControlled("pool", *checks.Timelock))
	require.ErrorContains(t, checks.requireControlled("pool", rmnSigner), `set mcms.qualifier to "RMNMCMS"`)
	require.ErrorContains(t, checks.requireControlled("pool", solana.NewWallet().PublicKey()), "neither the deployer nor the selected timelock")

	noQualifier, err := NewTokenPoolMigrationChecks(refs, chain, sel, common_utils.BurnMintTokenPool.String(), "")
	require.NoError(t, err, "no MCMS qualifier is valid when the deployer controls the pool")
	require.Nil(t, noQualifier.Timelock)
	require.ErrorContains(t, noQualifier.requireControlled("pool", *checks.Timelock), `set mcms.qualifier to "CLLCCIP"`)
	require.ErrorContains(t, noQualifier.requireControlled("pool", solana.NewWallet().PublicKey()), "set mcms.qualifier if it is a timelock signer")

	_, err = NewTokenPoolMigrationChecks(refs, chain, sel, common_utils.BurnMintTokenPool.String(), "nope")
	require.ErrorContains(t, err, `no timelock for MCMS qualifier "nope"`)

	_, err = NewTokenPoolMigrationChecks(refs[1:], chain, sel, common_utils.BurnMintTokenPool.String(), "CLLCCIP")
	require.ErrorContains(t, err, "resolve 2.0 Router")
}

// The CCV proposal analyzer decodes 2.x pool and Router instructions with the gobindings'
// ParseInstruction and then populates the accounts, so each instruction a migration proposal can carry
// must round-trip through both.
func TestMigrationProposalInstructionsDecode(t *testing.T) {
	key := func() solana.PublicKey { return solana.NewWallet().PublicKey() }
	mint := key()
	params := burnmint_token_pool.SetChainConfigV2Params{
		Mint:                  mint,
		RemoteChainSelector:   chainsel.ETHEREUM_TESTNET_SEPOLIA.Selector,
		AllowedFinalityConfig: burnmint_token_pool.FinalityConfig{Flags: 1, BlockDepth: 32},
	}

	initBnM, err := burnmint_token_pool.NewInitChainConfigV2Instruction(params, key(), key(), key(), solana.SystemProgramID,
		mint, key(), key(), solana.TokenProgramID, solana.SPLAssociatedTokenAccountProgramID, key(), key())
	require.NoError(t, err)
	decoded := decodeLikeAnalyzer(t, initBnM, burnmint_token_pool.ParseInstruction)
	require.IsType(t, &burnmint_token_pool.InitChainConfigV2Instruction{}, decoded)
	require.Equal(t, params, decoded.(*burnmint_token_pool.InitChainConfigV2Instruction).Params)

	editBnM, err := burnmint_token_pool.NewEditChainConfigV2Instruction(params, key(), key(), key(), solana.SystemProgramID, key(), key())
	require.NoError(t, err)
	require.IsType(t, &burnmint_token_pool.EditChainConfigV2Instruction{}, decodeLikeAnalyzer(t, editBnM, burnmint_token_pool.ParseInstruction))

	lrParams := lockrelease_token_pool.SetChainConfigV2Params{Mint: mint, RemoteChainSelector: params.RemoteChainSelector}
	initLR, err := lockrelease_token_pool.NewInitChainConfigV2Instruction(lrParams, key(), key(), key(), solana.SystemProgramID,
		mint, key(), key(), solana.TokenProgramID, solana.SPLAssociatedTokenAccountProgramID, key(), key())
	require.NoError(t, err)
	require.IsType(t, &lockrelease_token_pool.InitChainConfigV2Instruction{}, decodeLikeAnalyzer(t, initLR, lockrelease_token_pool.ParseInstruction))

	flip, err := ccip_router.NewSetPoolInterfaceImplementationInstruction(ccip_router.SetPoolInterfaceImplementationParams{
		Mint:              mint,
		InterfaceVersion:  2,
		AccountResolution: ccip_router.ResolutionType_WithoutExtendedLookupTable,
	}, key(), key(), key(), key(), key())
	require.NoError(t, err)
	require.IsType(t, &ccip_router.SetPoolInterfaceImplementationInstruction{}, decodeLikeAnalyzer(t, flip, ccip_router.ParseInstruction))
}

func decodeLikeAnalyzer[T interface {
	PopulateFromAccountIndices([]uint8, []solana.PublicKey) error
}](t *testing.T, ix solana.Instruction, parse func([]byte, []byte, []solana.PublicKey) (T, error)) T {
	t.Helper()
	data, err := ix.Data()
	require.NoError(t, err)
	decoded, err := parse(data, nil, nil)
	require.NoError(t, err)
	accounts := ix.Accounts()
	indices := make([]uint8, len(accounts))
	keys := make([]solana.PublicKey, len(accounts))
	for i, account := range accounts {
		indices[i] = uint8(i)
		keys[i] = account.PublicKey
	}
	require.NoError(t, decoded.PopulateFromAccountIndices(indices, keys))
	return decoded
}
