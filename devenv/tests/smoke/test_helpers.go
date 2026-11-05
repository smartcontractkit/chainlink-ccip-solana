package smoke

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"os"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	gethrpc "github.com/ethereum/go-ethereum/rpc"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/require"

	chainsel "github.com/smartcontractkit/chain-selectors"
	ccv "github.com/smartcontractkit/chainlink-ccv/build/devenv"
	"github.com/smartcontractkit/chainlink-ccv/build/devenv/chainreg"
	devenvcommon "github.com/smartcontractkit/chainlink-ccv/build/devenv/common"
	"github.com/smartcontractkit/chainlink-ccv/build/devenv/evm"
	e2ehelpers "github.com/smartcontractkit/chainlink-ccv/build/devenv/tests/e2e"
	"github.com/smartcontractkit/chainlink-ccv/protocol"
	cldf_evm "github.com/smartcontractkit/chainlink-deployments-framework/chain/evm"
	cldfrpc "github.com/smartcontractkit/chainlink-deployments-framework/chain/evm/provider/rpcclient"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	"github.com/smartcontractkit/chainlink-evm/gethwrappers/shared/generated/initial/erc20"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/base_token_pool"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/burnmint_token_pool"
	contractcommon "github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	soltokens "github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/tokens"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	soldevenv "github.com/smartcontractkit/chainlink-ccip-solana/devenv"
	devenvutils "github.com/smartcontractkit/chainlink-ccip-solana/devenv/tests/utils"
)

const defaultAssertionTimeout = 4 * time.Minute

// execTimeoutPerSrc is the exec delay budget per source network: finality, attestation,
// commit, and execute. Networks not listed (local chains) use defaultAssertionTimeout.
var execTimeoutPerSrc = map[uint64]time.Duration{
	chainsel.ETHEREUM_TESTNET_SEPOLIA.Selector: 35 * time.Minute,
	chainsel.SOLANA_DEVNET.Selector:            20 * time.Minute,
}

func execTimeout(sourceSelector uint64) time.Duration {
	if t, ok := execTimeoutPerSrc[sourceSelector]; ok {
		return t
	}
	return defaultAssertionTimeout
}

// fujiSendGasLimit is the explicit gas limit for ccipSend on Fuji. Fuji's
// post-'Helicon' state is unavailable under the pending tag, so estimateGas
// fails on every provider ('exceeds block gas limit'). Real usage is a few
// hundred k; 3M is a comfortable ceiling under the 15M block limit.
// per-chain estimateGas override; drop when upstream estimateGas works again.
const fujiSendGasLimit = 3_000_000

// evmSendOptions returns EVM send options for the given source chain.
func evmSendOptions(sourceSelector uint64) (evm.SendOptions, error) {
	opts := evm.SendOptions{UseTestRouter: useTestRouter()}
	if sourceSelector == chainsel.AVALANCHE_TESTNET_FUJI.Selector {
		opts.GasLimit = fujiSendGasLimit
	}
	return opts, nil
}

// useTestRouter reports whether EVM sends go through the TestRouter (USE_TEST_ROUTER=true).
func useTestRouter() bool {
	return os.Getenv("USE_TEST_ROUTER") == "true"
}

// mustEvmSendOptions is the test-call-site variant of evmSendOptions.
func mustEvmSendOptions(t *testing.T, sourceSelector uint64) evm.SendOptions {
	t.Helper()
	opts, err := evmSendOptions(sourceSelector)
	require.NoError(t, err)
	return opts
}

// Minimum transfer amounts that exercise both the full path and the pool's BPS fee:
// the 18dp(EVM) <-> 6dp(SVM) equivalent is 1e12 EVM base units == 1 SVM base unit.
// The Solana-source amount must be large enough that the 2.0.0 pool's 10 BPS fee
// (sourceAmount * TokenPoolFinalityBpsFee / 10_000, integer) is non-zero, otherwise
// the fee path is never exercised. Keeps funding tiny (0.01 whole tokens) at the
// recurring cadence; balance and pool-event assertions stay exact.
var (
	minSolanaTransferAmount = big.NewInt(10_000)                 // 1e-2 whole tokens (6dp)
	minEVMTransferAmount    = big.NewInt(10_000_000_000_000_000) // 1e-2 whole tokens (18dp) = minSolanaTransferAmount * 1e12
)

// evmRPCClient unwraps the EVM chain client to the raw RPC client used for eth_call.
func evmRPCClient(t *testing.T, evmClient any) *gethrpc.Client {
	t.Helper()

	switch c := evmClient.(type) {
	case *ethclient.Client:
		return c.Client()
	case *cldfrpc.MultiClient:
		return c.Client.Client()
	default:
		require.FailNow(t, "unsupported EVM RPC client", "client type %T", evmClient)
		return nil
	}
}

func fakeAttestationURL(t *testing.T) string {
	t.Helper()
	cfg, err := smokeConfig()
	require.NoError(t, err)
	if cfg.Fake == nil || cfg.Fake.Out == nil {
		return ""
	}
	return cfg.Fake.Out.ExternalHTTPURL
}

// solanaSendSigner returns the pubkey the tests sign Solana sends with:
// the SOLANA_PRIVATE_KEY signer when set(deployed env), otherwise the chain deployer key(local devenv).
func solanaSendSigner(chain *soldevenv.Chain) (solana.PublicKey, error) {
	opts, err := devenvutils.SolSendOptions()
	if err != nil {
		return solana.PublicKey{}, err
	}
	if opts.Signer != nil {
		return opts.Signer.PublicKey(), nil
	}
	addr, err := chain.GetSenderAddress()
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("get chain sender address: %w", err)
	}
	return solana.PublicKeyFromBytes(addr), nil
}

// solSenderTokenFundingError fails fast if the Solana sender cannot send a token
// transfer for mint: the send would otherwise fail deep in the onchain simulation
// with an opaque "invalid account data for instruction" once the sender's ATA is
// missing or empty. A missing ATA reads as a zero balance. Nil on sufficient funds.
func solSenderTokenFundingError(ctx context.Context, chain *soldevenv.Chain, mint solana.PublicKey) error {
	sender, err := solanaSendSigner(chain)
	if err != nil {
		return err
	}
	bal, err := chain.GetTokenBalance(ctx, protocol.UnknownAddress(sender.Bytes()), protocol.UnknownAddress(mint.Bytes()))
	if err != nil {
		return fmt.Errorf("get sender %s token balance for mint %s: %w", sender, mint, err)
	}
	if bal.Cmp(minSolanaTransferAmount) < 0 {
		return fmt.Errorf("sender %s holds %s base units of mint %s, needs >= %s; fund (or mint to) the sender first",
			sender, bal, mint, minSolanaTransferAmount)
	}
	return nil
}

type TokenPoolEventView struct {
	Mint                solana.PublicKey
	Recipient           solana.PublicKey
	Amount              uint64
	RemoteChainSelector *uint64
}

func NormalizeTokenPoolEvent(evt any) (TokenPoolEventView, error) {
	switch e := evt.(type) {
	case *base_token_pool.Minted:
		return TokenPoolEventView{Mint: e.Mint, Recipient: e.Recipient, Amount: e.Amount}, nil
	case *base_token_pool.Released:
		return TokenPoolEventView{Mint: e.Mint, Recipient: e.Recipient, Amount: e.Amount}, nil
	case *base_token_pool.ReleasedOrMinted:
		return TokenPoolEventView{
			Mint: e.Mint, Recipient: e.Recipient, Amount: e.Amount,
			RemoteChainSelector: &e.RemoteChainSelector,
		}, nil
	case *base_token_pool.Burned:
		return TokenPoolEventView{Mint: e.Mint, Amount: e.Amount}, nil
	case *base_token_pool.Locked:
		return TokenPoolEventView{Mint: e.Mint, Amount: e.Amount}, nil
	case *base_token_pool.LockedOrBurned:
		return TokenPoolEventView{
			Mint: e.Mint, Amount: e.Amount,
			RemoteChainSelector: &e.RemoteChainSelector,
		}, nil
	default:
		return TokenPoolEventView{}, fmt.Errorf("unsupported token pool event type %T", evt)
	}
}

// expectedPoolEventDiscriminator is the event a Solana destination pool emits on release/mint.
func expectedPoolEventDiscriminator(poolType string, poolVersion *semver.Version) ([8]byte, error) {
	if poolVersion == nil {
		return [8]byte{}, fmt.Errorf("pool version is nil for pool type %q", poolType)
	}

	if poolVersion.GreaterThanEqual(solcommon.Version2_0_0) {
		return base_token_pool.Event_ReleasedOrMinted, nil
	}

	switch poolType {
	case devenvcommon.BurnMintTokenPoolType:
		return base_token_pool.Event_Minted, nil
	case devenvcommon.LockReleaseTokenPoolType:
		return base_token_pool.Event_Released, nil
	default:
		return [8]byte{}, fmt.Errorf("unsupported pool type %q for version %s", poolType, poolVersion)
	}
}

// expectedSourcePoolEventDiscriminator is the event a Solana source pool emits on lock/burn.
func expectedSourcePoolEventDiscriminator(poolType string, poolVersion *semver.Version) ([8]byte, error) {
	if poolVersion == nil {
		return [8]byte{}, fmt.Errorf("pool version is nil for pool type %q", poolType)
	}

	// v2 pools emit a single event for both lock and burn
	if poolVersion.GreaterThanEqual(solcommon.Version2_0_0) {
		return base_token_pool.Event_LockedOrBurned, nil
	}

	// v1 pools emit separate events for lock and burn
	switch poolType {
	case devenvcommon.BurnMintTokenPoolType:
		return base_token_pool.Event_Burned, nil
	case devenvcommon.LockReleaseTokenPoolType:
		return base_token_pool.Event_Locked, nil
	default:
		return [8]byte{}, fmt.Errorf("unsupported pool type %q for version %s", poolType, poolVersion)
	}
}

// tokenPoolSpec identifies one side of a pool pair by type, version, and CCV qualifiers.
type tokenPoolSpec struct {
	poolType      string
	poolVersion   string
	ccvQualifiers []string
}

// tokenPoolTestCase pairs an EVM pool with a Solana pool by semantics. It carries no
// direction: callers pass the sides to tokenCombination in source, dest order.
// caseName names the Solana pool under test.
type tokenPoolTestCase struct {
	caseName string
	evm      tokenPoolSpec
	solana   tokenPoolSpec
}

// tokenPoolTestCases defines the pool pairs exercised by the TokenTransfer subtests
// in both directions.
var tokenPoolTestCases = []tokenPoolTestCase{
	{
		caseName: "BurnMint-1.6.1",
		// Solana pool under test (1.6.x). The EVM source pool is 2.0.0: Solana pool
		// coverage (1.6.x + 2.0.0) is required, EVM 1.6.x pool coverage is not.
		// Also makes this lane fast-finality instead of full finality on the EVM chain.
		evm: tokenPoolSpec{
			poolType: devenvcommon.BurnMintTokenPoolType, poolVersion: "2.0.0",
			ccvQualifiers: []string{devenvcommon.DefaultCommitteeVerifierQualifier},
		},
		solana: tokenPoolSpec{poolType: devenvcommon.BurnMintTokenPoolType, poolVersion: "1.6.1"},
	},
	{
		caseName: "LockRelease-1.6.1",
		evm: tokenPoolSpec{
			poolType: devenvcommon.BurnMintTokenPoolType, poolVersion: "2.0.0",
			ccvQualifiers: []string{devenvcommon.DefaultCommitteeVerifierQualifier},
		},
		solana: tokenPoolSpec{poolType: devenvcommon.LockReleaseTokenPoolType, poolVersion: "1.6.1"},
	},
	{
		caseName: "BurnMint-2.0.0",
		// Staging deployed this pairing as the catalog's "No CCV" variant
		// (empty ccvQualifiers on both sides).
		evm:    tokenPoolSpec{poolType: devenvcommon.BurnMintTokenPoolType, poolVersion: "2.0.0"},
		solana: tokenPoolSpec{poolType: devenvcommon.BurnMintTokenPoolType, poolVersion: "2.0.0"},
	},
	{
		caseName: "LockRelease-2.0.0",
		evm: tokenPoolSpec{
			poolType: devenvcommon.BurnMintTokenPoolType, poolVersion: "2.0.0",
			ccvQualifiers: []string{devenvcommon.DefaultCommitteeVerifierQualifier},
		},
		solana: tokenPoolSpec{
			poolType: devenvcommon.LockReleaseTokenPoolType, poolVersion: "2.0.0",
			ccvQualifiers: []string{devenvcommon.DefaultCommitteeVerifierQualifier},
		},
	},
}

// specMatches reports whether a pool side (type, version, CCV qualifiers) equals spec.
func specMatches(poolType, poolVersion string, ccvQualifiers []string, spec tokenPoolSpec) bool {
	return poolType == spec.poolType &&
		poolVersion == spec.poolVersion &&
		slices.Equal(ccvQualifiers, spec.ccvQualifiers)
}

// tokenCombination returns the first combo whose local pool matches src and remote pool
// matches dest. token_transfer binds a combination's local pool to the source chain, so
// callers pass their direction, e.g. tokenCombination(tc.solana, tc.evm, nil).
// Datastore-derived combos are searched first (they reflect what is actually deployed);
// their refs are matched on type+version only, since attached environments deploy their
// own CCV qualifier sets (the writer emits [default] for 2.0.0 pairs the specs list as
// []). Both legs must match on type+version, which separates pairs sharing one leg;
// duplicate full type+version pairs differing only in CCV do not exist. The curated
// catalog is the fallback for local devenv runs.
func tokenCombination(src, dest tokenPoolSpec, extra []devenvcommon.TokenCombination) (devenvcommon.TokenCombination, error) {
	for _, combo := range extra {
		local := combo.LocalPoolAddressRef()
		remote := combo.RemotePoolAddressRef()
		if string(local.Type) == src.poolType && local.Version.String() == src.poolVersion &&
			string(remote.Type) == dest.poolType && remote.Version.String() == dest.poolVersion {
			return combo, nil
		}
	}
	for _, combo := range devenvcommon.AllTokenCombinations() {
		local := combo.LocalPoolAddressRef()
		remote := combo.RemotePoolAddressRef()
		if specMatches(string(local.Type), local.Version.String(), combo.LocalPoolCCVQualifiers(), src) &&
			specMatches(string(remote.Type), remote.Version.String(), combo.RemotePoolCCVQualifiers(), dest) {
			return combo, nil
		}
	}
	return devenvcommon.TokenCombination{}, fmt.Errorf(
		"no token combination with source pool %s %s %v and dest pool %s %s %v",
		src.poolType, src.poolVersion, src.ccvQualifiers,
		dest.poolType, dest.poolVersion, dest.ccvQualifiers,
	)
}

// declaredCombos resolves token-transfer combinations from the environment's
// [[token_pool_pairs]] declarations, filtered to the requested direction.
// Environments without declarations fall back to the curated catalog,
// filtered per devenv deployment (own refs per chain, exact match).
func declaredCombos(t *testing.T, ds datastore.DataStore, srcSel, destSel uint64) []devenvcommon.TokenCombination {
	t.Helper()
	in, err := smokeConfig()
	require.NoError(t, err, "load smoke test config")
	if len(in.TokenPoolPairs) == 0 {
		return devenvcommon.FilterTokenCombinations(
			devenvcommon.AllTokenCombinations(), in.EnvironmentTopology, ds, []uint64{srcSel, destSel})
	}
	var pairs []devenvcommon.TokenPoolPair
	for _, p := range in.TokenPoolPairs {
		if p.LocalSelector == strconv.FormatUint(srcSel, 10) && p.RemoteSelector == strconv.FormatUint(destSel, 10) {
			pairs = append(pairs, p)
		}
	}
	combos, err := devenvcommon.TokenCombinationsFromPoolPairs(ds, pairs)
	require.NoError(t, err, "resolve declared token pool pairs")
	return combos
}

// smokeConfig loads the env toml once per process.
var smokeConfig = sync.OnceValues(func() (*ccv.Cfg, error) {
	return ccv.LoadOutput[ccv.Cfg](e2ehelpers.GetSmokeTestConfig())
})

// solanaMintForPoolPair returns the SPL mint for a pool pair's Solana-side pool
// qualifier. It skips the test (rather than failing) when the pair is not deployed
// in the current environment: local devenv deploys every catalog combo, but attached
// environments (staging, prod testnet) only carry whatever was actually deployed.
func solanaMintForPoolPair(t *testing.T, ds datastore.DataStore, solSelector uint64, qualifier string) solana.PublicKey {
	t.Helper()
	mintBytes, err := devenvutils.GetSolanaTokenAddress(ds, solSelector, qualifier)
	if err != nil {
		t.Skipf("pool pair not deployed in this environment: %v", err)
	}
	return solana.PublicKeyFromBytes(mintBytes)
}

// TODO: ccv approval check can be flaky, consider adding confirmation loop upstream
// ensureEVMAllowance pre-approves the EVM router for MaxUint256 on the source token
// and waits until the allowance is visible on the RPC. This prevents a race where
// the approve tx mines but estimateGas for CcipSend reads a stale block.
func ensureEVMAllowance(
	ctx context.Context,
	t *testing.T,
	evmChain cldf_evm.Chain,
	ds datastore.DataStore,
	evmSelector uint64,
	combo devenvcommon.TokenCombination,
	routerAddr common.Address,
	minAmount *big.Int,
) {
	t.Helper()

	evmReg, err := chainreg.GetRegistry().Get(chainsel.FamilyEVM)
	require.NoError(t, err)
	srcTokenBytes, err := evmReg.AddressResolver.GetToken(ds, evmSelector, combo.LocalPoolAddressRef())
	require.NoError(t, err, "resolve EVM source token address")
	tokenAddr := common.BytesToAddress(srcTokenBytes)

	tkn, err := erc20.NewERC20(tokenAddr, evmChain.Client)
	require.NoError(t, err)

	owner := evmChain.DeployerKey.From
	allowance, err := tkn.Allowance(&bind.CallOpts{Context: ctx}, owner, routerAddr)
	require.NoError(t, err)
	if allowance.Cmp(minAmount) >= 0 {
		return
	}

	// Explicit gas: Fuji providers reject the gas-0 estimate path (its
	// PendingCodeAt/estimate reads hit "state not available for pending block").
	tx, err := tkn.Approve(&bind.TransactOpts{
		Context:  ctx,
		From:     evmChain.DeployerKey.From,
		Signer:   evmChain.DeployerKey.Signer,
		GasLimit: 60000, // standard ERC20 approve
	}, routerAddr, minAmount)
	require.NoError(t, err, "approve router for token %s", tokenAddr.Hex())
	t.Logf("approve tx %s submitted for token %s, waiting for allowance visibility", tx.Hash().Hex(), tokenAddr.Hex())

	require.Eventually(t, func() bool {
		a, aErr := tkn.Allowance(&bind.CallOpts{Context: ctx}, owner, routerAddr)
		return aErr == nil && a.Cmp(minAmount) >= 0
	}, 60*time.Second, 2*time.Second, "allowance not visible after approve tx mined")
}

// getPoolFinalityBpsFee reads the v2 pool's ChainConfigV2 account onchain and
// returns its FinalityBpsFee. The value is environment-specific: devenv sets 10,
// staging may differ. Returns 0 if the account does not exist.
func getPoolFinalityBpsFee(ctx context.Context, client *rpc.Client, poolProgram solana.PublicKey, remoteSelector uint64, mint solana.PublicKey) (uint16, error) {
	pda, _, err := soltokens.TokenPoolChainConfigV2PDA(remoteSelector, mint, poolProgram)
	if err != nil {
		return 0, fmt.Errorf("derive ChainConfigV2 PDA: %w", err)
	}
	var cfg burnmint_token_pool.ChainConfigV2
	if err := contractcommon.GetAccountDataBorshIntoAnchor(ctx, client, pda, rpc.CommitmentConfirmed, &cfg); err != nil {
		if errors.Is(err, rpc.ErrNotFound) { // account missing — no fee configured
			return 0, nil
		}
		return 0, fmt.Errorf("read ChainConfigV2 %s: %w", pda, err)
	}
	return cfg.TokenTransferFeeConfig.FinalityBpsFee, nil
}
