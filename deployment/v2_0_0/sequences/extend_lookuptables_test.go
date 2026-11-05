package sequences

import (
	"testing"

	"github.com/Masterminds/semver/v3"
	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	common_utils "github.com/smartcontractkit/chainlink-ccip/deployment/utils"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"

	soltokens "github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/tokens"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	executorops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/executor"
	fqops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/feequoter"
	rmnremoteops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/rmnremote"
	routerops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/router"
	tokenpoolops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/tokenpools"
	tokensops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/tokens"
)

const testSelector = uint64(5526668148211293847)

func randomPubkey(t *testing.T) string {
	t.Helper()
	privKey, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)
	return privKey.PublicKey().String()
}

func poolRef(t *testing.T, chainSelector uint64, poolType string, qualifier string) datastore.AddressRef {
	return datastore.AddressRef{
		ChainSelector: chainSelector,
		Address:       randomPubkey(t),
		Type:          datastore.ContractType(poolType),
		Version:       semver.MustParse("2.0.0"),
		Qualifier:     qualifier,
	}
}

func mintRef(t *testing.T, chainSelector uint64, tokenProgram datastore.ContractType, qualifier string) datastore.AddressRef {
	return datastore.AddressRef{
		ChainSelector: chainSelector,
		Address:       randomPubkey(t),
		Type:          tokenProgram,
		Version:       solcommon.Version2_0_0,
		Qualifier:     qualifier,
	}
}

func TestPoolFeeVaultLutEntries(t *testing.T) {
	burnPool := poolRef(t, testSelector, common_utils.BurnMintTokenPool.String(), "TOK")
	lockPool := poolRef(t, testSelector, common_utils.LockReleaseTokenPool.String(), "LOK")
	tokMint := mintRef(t, testSelector, datastore.ContractType(solcommon.SPLTokens), "TOK")
	lokMint := mintRef(t, testSelector, datastore.ContractType(solcommon.SPL2022Tokens), "LOK")
	usdcMint := mintRef(t, testSelector, datastore.ContractType(solcommon.SPLTokens), "USDC")

	tokMintKey, err := solcommon.ToAddress(tokMint)
	require.NoError(t, err)
	lokMintKey, err := solcommon.ToAddress(lokMint)
	require.NoError(t, err)
	tokPoolKey, err := solcommon.ToAddress(burnPool)
	require.NoError(t, err)
	lokPoolKey, err := solcommon.ToAddress(lockPool)
	require.NoError(t, err)

	tokVault, err := soltokens.FindFeeVaultATA(solana.TokenProgramID, tokMintKey, tokPoolKey, tokMintKey.Bytes())
	require.NoError(t, err)
	lokVault, err := soltokens.FindFeeVaultATA(solana.Token2022ProgramID, lokMintKey, lokPoolKey, lokMintKey.Bytes())
	require.NoError(t, err)

	refs := []datastore.AddressRef{
		burnPool, lockPool, tokMint, lokMint, usdcMint,
		// CCTP and Lombard pools do not resolve the fee vault ATA on the send path.
		poolRef(t, testSelector, common_utils.CCTPTokenPool.String(), "USDC"),
		poolRef(t, testSelector, tokenpoolops.LombardTokenPool.String(), "USDC"),
		// The plain pool program alias has no mint and must be skipped.
		poolRef(t, testSelector, common_utils.BurnMintTokenPool.String(), solcommon.PoolProgramQualifier),
		// A pool instance without a deployed mint ref is skipped and reported.
		poolRef(t, testSelector, common_utils.BurnMintTokenPool.String(), "ORPHAN"),
		// Wrong version and wrong chain are skipped.
		func() datastore.AddressRef {
			r := poolRef(t, testSelector, common_utils.BurnMintTokenPool.String(), "V1")
			r.Version = semver.MustParse("1.6.1")
			return r
		}(),
		poolRef(t, 1, common_utils.BurnMintTokenPool.String(), "REMOTE"),
		mintRef(t, 1, datastore.ContractType(solcommon.SPLTokens), "REMOTE"),
	}

	entries, skipped, err := poolFeeVaultLutEntries(refs, testSelector)
	require.NoError(t, err)
	assert.Equal(t, []solana.PublicKey{tokVault, lokVault}, entries)
	assert.Equal(t, []string{"ORPHAN"}, skipped)
}

func TestResolveCcipSendLutAccountsLink(t *testing.T) {
	ref := func(contractType datastore.ContractType, version *semver.Version, qualifier string) datastore.AddressRef {
		return datastore.AddressRef{
			ChainSelector: testSelector,
			Address:       randomPubkey(t),
			Type:          contractType,
			Version:       version,
			Qualifier:     qualifier,
		}
	}
	program := func(extra ...datastore.AddressRef) []datastore.AddressRef {
		refs := make([]datastore.AddressRef, 0, 6+len(extra))
		refs = append(refs,
			ref(datastore.ContractType(routerops.ContractType), routerops.Version, ""),
			ref(datastore.ContractType(fqops.ContractType), fqops.Version, ""),
			ref(datastore.ContractType(rmnremoteops.ContractType), rmnremoteops.Version, ""),
			ref(datastore.ContractType(common_utils.BurnMintTokenPool), solcommon.Version2_0_0, solcommon.PoolProgramQualifier),
			ref(datastore.ContractType(common_utils.LockReleaseTokenPool), solcommon.Version2_0_0, solcommon.PoolProgramQualifier),
			ref(datastore.ContractType(executorops.ContractType), executorops.Version, "default"),
		)
		return append(refs, extra...)
	}
	link := datastore.ContractType(tokensops.LinkContractType)

	// A chain deployed before 2.0 only has its LINK ref at 1.6.0.
	legacy := ref(link, semver.MustParse("1.6.0"), "LINK")
	got, err := resolveCcipSendLutAccounts(program(legacy), testSelector)
	require.NoError(t, err)
	require.Equal(t, legacy.Address, got.LinkToken.String())

	current := ref(link, tokensops.Version, "LINK")
	got, err = resolveCcipSendLutAccounts(program(legacy, current), testSelector)
	require.NoError(t, err)
	require.Equal(t, current.Address, got.LinkToken.String(), "the 2.0.0 ref wins when both are registered")

	_, err = resolveCcipSendLutAccounts(program(), testSelector)
	require.ErrorIs(t, err, solcommon.ErrAddressRefNotFound)
}
