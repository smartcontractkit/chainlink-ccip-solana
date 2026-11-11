package sequences

import (
	"testing"

	"github.com/Masterminds/semver/v3"
	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"

	chainsel "github.com/smartcontractkit/chain-selectors"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"

	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	cctpverifierops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/cctpverifier"
	lvops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/lombardverifier"
)

func TestProgramRefInfoFor(t *testing.T) {
	v1 := semver.MustParse("1.0.0")
	v2 := semver.MustParse("2.0.0")
	pool := solcommon.PoolProgramQualifier

	for programType, want := range map[ProgramUpgradeType]struct {
		contractType      datastore.ContractType
		version           *semver.Version
		qualifier         string
		programPerVersion bool
	}{
		UpgradeTypeRouter:               {"Router", v2, "", false},
		UpgradeTypeFeeQuoter:            {"FeeQuoter", v2, "", false},
		UpgradeTypeRMNRemote:            {"RMNRemote", v2, "", false},
		UpgradeTypeBurnMintTokenPool:    {"BurnMintTokenPool", v2, pool, false},
		UpgradeTypeLockReleaseTokenPool: {"LockReleaseTokenPool", v2, pool, false},
		UpgradeTypeCCTPTokenPool:        {"CCTPTokenPool", v2, pool, false},
		UpgradeTypeLombardTokenPool:     {"LombardTokenPool", v2, pool, false},
		UpgradeTypeExecutor:             {"Executor", v2, "default", false},
		UpgradeTypeCommitteeVerifier:    {"CommitteeVerifier", v2, "default", false},
		UpgradeTypeOffRamp:              {"OffRamp", v2, "", true},
		UpgradeTypeTestSenderLegacy:     {"TestSender", v1, "default", true},
		UpgradeTypeTestSenderV2:         {"TestSender", v2, "default", true},
		UpgradeTypeTestReceiverLegacy:   {"TestReceiver", v1, "", true},
		UpgradeTypeTestReceiverV2:       {"TestReceiver", v2, "", true},
	} {
		info, ok := ProgramRefInfoFor(programType)
		require.True(t, ok, programType)
		require.Equal(t, want.contractType, info.ContractType, programType)
		require.True(t, want.version.Equal(info.Version), "%s: version %s, want %s", programType, info.Version, want.version)
		require.Equal(t, want.qualifier, info.DefaultQualifier, programType)
		require.Equal(t, want.programPerVersion, info.ProgramPerVersion, programType)
	}

	_, ok := ProgramRefInfoFor("Unknown")
	require.False(t, ok)
}

func TestUpgradeAdditionalRefs(t *testing.T) {
	selector := chainsel.SOLANA_DEVNET.Selector
	v2 := semver.MustParse("2.0.0")
	address := solana.NewWallet().PublicKey().String()
	other := solana.NewWallet().PublicKey().String()

	ref := func(contractType datastore.ContractType, qualifier, addr string) datastore.AddressRef {
		return datastore.AddressRef{Address: addr, ChainSelector: selector, Type: contractType, Version: v2, Qualifier: qualifier}
	}
	bnm := datastore.ContractType(UpgradeTypeBurnMintTokenPool)
	cctp := datastore.ContractType(UpgradeTypeCCTPTokenPool)
	lombard := datastore.ContractType(UpgradeTypeLombardTokenPool)
	cctpVerifier := datastore.ContractType(cctpverifierops.ContractType)
	lombardVerifier := datastore.ContractType(lvops.ContractType)

	t.Run("BurnMint gets the TokenPoolProgram alias", func(t *testing.T) {
		got := UpgradeAdditionalRefs(UpgradeTypeBurnMintTokenPool, []datastore.AddressRef{ref(bnm, "CLLCCIP", address)}, selector, v2, address)
		require.Empty(t, got.Conflicting)
		require.Equal(t, []datastore.AddressRef{ref(bnm, solcommon.PoolProgramQualifier, address)}, got.Add)
	})

	t.Run("LockRelease gets it too", func(t *testing.T) {
		lockRelease := datastore.ContractType(UpgradeTypeLockReleaseTokenPool)
		got := UpgradeAdditionalRefs(UpgradeTypeLockReleaseTokenPool, nil, selector, v2, address)
		require.Equal(t, []datastore.AddressRef{ref(lockRelease, solcommon.PoolProgramQualifier, address)}, got.Add)
	})

	t.Run("CCTP gets the alias and the verifier", func(t *testing.T) {
		got := UpgradeAdditionalRefs(UpgradeTypeCCTPTokenPool, nil, selector, v2, address)
		require.Empty(t, got.Conflicting)
		require.ElementsMatch(t, []datastore.AddressRef{
			ref(cctp, solcommon.PoolProgramQualifier, address),
			ref(cctpVerifier, string(cctpverifierops.ContractType), address),
		}, got.Add)
	})

	t.Run("the alias follows the version of the program, the verifier keeps the version consumers look it up by", func(t *testing.T) {
		v201 := semver.MustParse("2.0.1")
		got := UpgradeAdditionalRefs(UpgradeTypeCCTPTokenPool, nil, selector, v201, address)
		alias := ref(cctp, solcommon.PoolProgramQualifier, address)
		alias.Version = v201
		require.ElementsMatch(t, []datastore.AddressRef{
			alias,
			ref(cctpVerifier, string(cctpverifierops.ContractType), address),
		}, got.Add)
	})

	t.Run("Lombard gets the verifier under both qualifiers, and the alias when the instance is not it", func(t *testing.T) {
		got := UpgradeAdditionalRefs(UpgradeTypeLombardTokenPool, nil, selector, v2, address)
		require.Empty(t, got.Conflicting)
		require.ElementsMatch(t, []datastore.AddressRef{
			ref(lombard, solcommon.PoolProgramQualifier, address),
			ref(lombardVerifier, string(lvops.ContractType), address),
			ref(lombardVerifier, lvops.LombardVerifierResolverQualifier, address),
		}, got.Add)
	})

	t.Run("Lombard under the alias qualifier already has it", func(t *testing.T) {
		refs := []datastore.AddressRef{ref(lombard, solcommon.PoolProgramQualifier, address)}
		got := UpgradeAdditionalRefs(UpgradeTypeLombardTokenPool, refs, selector, v2, address)
		require.ElementsMatch(t, []datastore.AddressRef{
			ref(lombardVerifier, string(lvops.ContractType), address),
			ref(lombardVerifier, lvops.LombardVerifierResolverQualifier, address),
		}, got.Add)
	})

	t.Run("a ref already at this address is not saved again", func(t *testing.T) {
		refs := []datastore.AddressRef{
			ref(cctp, "USDC", address),
			ref(cctp, solcommon.PoolProgramQualifier, address),
			ref(cctpVerifier, string(cctpverifierops.ContractType), address),
		}
		got := UpgradeAdditionalRefs(UpgradeTypeCCTPTokenPool, refs, selector, v2, address)
		require.Empty(t, got.Add)
		require.Empty(t, got.Conflicting)
	})

	t.Run("a ref at another program is reported, not replaced: the first pool upgraded keeps the alias", func(t *testing.T) {
		existingAlias := ref(bnm, solcommon.PoolProgramQualifier, other)
		got := UpgradeAdditionalRefs(UpgradeTypeBurnMintTokenPool, []datastore.AddressRef{existingAlias}, selector, v2, address)
		require.Empty(t, got.Add)
		require.Equal(t, []datastore.AddressRef{existingAlias}, got.Conflicting)
	})

	t.Run("only the refs that are missing are added", func(t *testing.T) {
		refs := []datastore.AddressRef{ref(cctp, solcommon.PoolProgramQualifier, address)}
		got := UpgradeAdditionalRefs(UpgradeTypeCCTPTokenPool, refs, selector, v2, address)
		require.Equal(t, []datastore.AddressRef{ref(cctpVerifier, string(cctpverifierops.ContractType), address)}, got.Add)
	})

	t.Run("no address, nothing to save", func(t *testing.T) {
		got := UpgradeAdditionalRefs(UpgradeTypeCCTPTokenPool, nil, selector, v2, "")
		require.Empty(t, got.Add)
		require.Empty(t, got.Conflicting)
	})

	t.Run("programs without additional refs get none", func(t *testing.T) {
		for _, programType := range []ProgramUpgradeType{
			UpgradeTypeRouter, UpgradeTypeFeeQuoter, UpgradeTypeRMNRemote, UpgradeTypeOffRamp,
			UpgradeTypeExecutor, UpgradeTypeCommitteeVerifier, UpgradeTypeTestSenderV2, UpgradeTypeTestReceiverV2,
		} {
			got := UpgradeAdditionalRefs(programType, nil, selector, v2, address)
			require.Empty(t, got.Add, programType)
			require.Empty(t, got.Conflicting, programType)
		}
	})

	t.Run("a different chain's refs do not count", func(t *testing.T) {
		otherChain := ref(bnm, solcommon.PoolProgramQualifier, other)
		otherChain.ChainSelector = chainsel.SOLANA_MAINNET.Selector
		got := UpgradeAdditionalRefs(UpgradeTypeBurnMintTokenPool, []datastore.AddressRef{otherChain}, selector, v2, address)
		require.Len(t, got.Add, 1)
		require.Empty(t, got.Conflicting)
	})
}
