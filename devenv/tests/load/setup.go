package loadtest

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	chainsel "github.com/smartcontractkit/chain-selectors"
	ccv "github.com/smartcontractkit/chainlink-ccv/build/devenv"
	"github.com/smartcontractkit/chainlink-ccv/build/devenv/cciptestinterfaces"
	"github.com/smartcontractkit/chainlink-ccv/build/devenv/chainreg"
	ccldf "github.com/smartcontractkit/chainlink-ccv/build/devenv/cldf"
	"github.com/smartcontractkit/chainlink-deployments-framework/deployment"

	soldevenv "github.com/smartcontractkit/chainlink-ccip-solana/devenv"
	"github.com/smartcontractkit/chainlink-ccip-solana/devenv/ccvregistry"
	devenvutils "github.com/smartcontractkit/chainlink-ccip-solana/devenv/tests/utils"
)

const (
	defaultLoadEnvConfigPath = "../../env-phased-out.toml"
	defaultLoadTestRPS       = 2
	defaultLoadTestDuration  = 5 * time.Second
	defaultLoadTestPoolSize  = 2
	defaultLoadTestMsgSize   = 0 // use empty message by default
	// Disabled by default: RPS scheduling is used unless an interval is set, this is used only if we want to send messages at a fixed interval (e.g. 1 message every 30s).
	defaultLoadTestSendInterval time.Duration = 0
	configPathEnvVar                          = "LOAD_TEST_OUT_FILE"
	rpsEnvVar                                 = "LOAD_TEST_RPS"
	durationEnvVar                            = "LOAD_TEST_DURATION"
	poolSizeEnvVar                            = "LOAD_TEST_POOL_SIZE"
	sendIntervalEnvVar                        = "LOAD_TEST_SEND_INTERVAL"
	messageSizeEnvVar                         = "LOAD_TEST_MSG_SIZE"
	fireAndForgetEnvVar                       = "LOAD_TEST_FIRE_AND_FORGET"
)

func loadEnvConfigPath() string {
	if p := os.Getenv(configPathEnvVar); p != "" {
		return p
	}
	return defaultLoadEnvConfigPath
}

func loadTestRPS() int64 {
	if v := os.Getenv(rpsEnvVar); v != "" {
		rps, err := strconv.ParseInt(v, 10, 64)
		if err != nil || rps <= 0 {
			return defaultLoadTestRPS
		}
		return rps
	}
	return defaultLoadTestRPS
}

func loadMessageSize() int64 {
	if v := os.Getenv(messageSizeEnvVar); v != "" {
		ms, err := strconv.ParseInt(v, 10, 64)
		if err != nil || ms <= 0 {
			return 0
		}
		return ms
	}
	return defaultLoadTestMsgSize
}

// loadTestFireAndForget reports whether EVM sends should use fire-and-forget mode
// (submit without waiting for confirmation). Defaults to enabled; set "false" to
// use synchronous send+confirm.
func loadTestFireAndForget() bool {
	if v := os.Getenv(fireAndForgetEnvVar); v != "" {
		b, err := strconv.ParseBool(v)
		if err == nil {
			return b
		}
	}
	return true
}

func loadTestDuration() time.Duration {
	if v := os.Getenv(durationEnvVar); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			return defaultLoadTestDuration
		}
		return d
	}
	return defaultLoadTestDuration
}

func loadTestPoolSize() int {
	if v := os.Getenv(poolSizeEnvVar); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return defaultLoadTestPoolSize
		}
		return n
	}
	return defaultLoadTestPoolSize
}

// loadTestSendInterval returns the fixed interval between sends. When > 0 the
// harness uses a ticker driver instead of a wasp RPS schedule (for sub-1 RPS
// profiles such as one message every 30s).
func loadTestSendInterval() time.Duration {
	if v := os.Getenv(sendIntervalEnvVar); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			return defaultLoadTestSendInterval
		}
		return d
	}
	return defaultLoadTestSendInterval
}

// loadTestConfigFromEnv builds the load config from LOAD_TEST_* env vars.
func loadTestConfigFromEnv() loadTestConfig {
	return loadTestConfig{
		rps:           loadTestRPS(),
		duration:      loadTestDuration(),
		sendInterval:  loadTestSendInterval(),
		msgSize:       loadMessageSize(),
		fireAndForget: loadTestFireAndForget(),
	}
}

// crossFamilyLoadEnv holds chain impls shared by Sol→EVM and EVM→Sol load tests.
type crossFamilyLoadEnv struct {
	logger      zerolog.Logger
	evmSelector uint64
	solSelector uint64
	impls       map[uint64]cciptestinterfaces.CCIP17
	cfg         *ccv.Cfg
	deployEnv   *deployment.Environment
	selectors   []uint64
}

// setupCrossFamilyLoadEnv registers Solana, loads env-out.toml, and returns both chain impls.
// The returned context is detached so wasp and AssertMessagesAsync are not tied to t.Context().
func setupCrossFamilyLoadEnv(t *testing.T) (context.Context, *crossFamilyLoadEnv) {
	t.Helper()

	_ = chainreg.Register(chainsel.FamilySolana, chainreg.Registration{
		ImplFactory:  soldevenv.NewImplFactory(),
		CLDFProvider: ccvregistry.SolanaCLDFProviderFactory,
	})

	ctx := ccv.Plog.WithContext(context.Background())
	lggr := zerolog.Ctx(ctx)

	configPath := loadEnvConfigPath()
	in, err := ccv.LoadOutput[ccv.Cfg](configPath)
	require.NoError(t, err)

	// CLDF env is required for EVMTXGun (datastore + EVM chain users); Sol→EVM ignores deployEnv.
	selectors, deployEnv, err := ccldf.NewCLDFOperationsEnvironment(in.Blockchains, in.CLDF.DataStore)
	require.NoError(t, err)
	deployEnv.OperationsBundle = ccldf.NewDefaultCLDFBundle(deployEnv)

	lib, err := ccv.NewLibFromCCVEnv(lggr, configPath)
	require.NoError(t, err)

	impls, err := lib.ChainsMap(ctx)
	require.NoError(t, err)

	solSelector, evmSelector, laneErr := devenvutils.ResolveSolanaEVMLanes(in.Blockchains)
	require.NoError(t, laneErr)

	require.Contains(t, impls, evmSelector, "EVM chain missing from env-out")
	solanaChain, ok := impls[solSelector]
	require.True(t, ok, "Solana chain (selector %d) missing from env-out", solSelector)
	t.Cleanup(func() {
		if closer, canClose := solanaChain.(interface{ Close() error }); canClose {
			_ = closer.Close()
		}
	})

	return ctx, &crossFamilyLoadEnv{
		logger:      lggr.With().Str("test", t.Name()).Logger(),
		evmSelector: evmSelector,
		solSelector: solSelector,
		impls:       impls,
		cfg:         in,
		deployEnv:   deployEnv,
		selectors:   selectors,
	}
}
