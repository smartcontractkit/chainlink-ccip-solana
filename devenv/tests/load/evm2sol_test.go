package loadtest

import (
	"strconv"
	"testing"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"

	devenvcommon "github.com/smartcontractkit/chainlink-ccv/build/devenv/common"
	ccve2e "github.com/smartcontractkit/chainlink-ccv/build/devenv/tests/e2e"
	"github.com/smartcontractkit/chainlink-ccv/build/devenv/tests/e2e/load"

	soldevenv "github.com/smartcontractkit/chainlink-ccip-solana/devenv"
	devenvutils "github.com/smartcontractkit/chainlink-ccip-solana/devenv/tests/utils"
)

// TestEVM2SolanaLoad fires sustained EVM→Solana arbitrary messages with the
// chainlink-ccv EVMTransactionGun, with an optional fire-and-forget mode (true by default):
// if enabled EVM txs are submitted without waiting for confirmation (SentTime = block timestamp of the block
// containing the tx), avoiding RPC-throttling timeouts on public testnets under load.
func TestEVM2SolanaLoad(t *testing.T) {
	cfg := loadTestConfigFromEnv()
	runEVMLoadTest(t, cfg, cfg.fireAndForget)
}

func runEVMLoadTest(t *testing.T, cfg loadTestConfig, useFireAndForget bool) {
	t.Helper()

	ctx, env := setupCrossFamilyLoadEnv(t)

	evmChains := env.deployEnv.BlockChains.EVMChains()
	srcChain, ok := evmChains[env.evmSelector]
	require.True(t, ok, "EVM chain (selector %d) not found in CLDF environment", env.evmSelector)

	solReceiver, err := soldevenv.NewAddressResolver().GetContractReceiver(
		env.deployEnv.DataStore, env.solSelector, devenvcommon.DefaultReceiverQualifier,
	)
	require.NoError(t, err)

	receiverProgramID := solana.PublicKeyFromBytes(solReceiver)
	evmSender, err := env.impls[env.evmSelector].GetSenderAddress()
	require.NoError(t, err)
	arbExecutorArgs, err := devenvutils.BuildV2ReceiverExecutorArgs(env.evmSelector, evmSender, receiverProgramID)
	require.NoError(t, err)

	// EVMTXGun pays fees in WETH.
	ccve2e.EnsureWETHBalanceAndApproval(ctx, t, env.logger, env.deployEnv, srcChain)

	// When a non-zero payload size is configured, select a data profile so the
	// EVMTXGun exercises the destination's buffered execution path for large messages.
	messageProfileName := "small"
	messageProfiles := make([]load.MessageProfileConfig, 0)
	if cfg.msgSize > 0 {
		messageProfileName = "large"
		messageProfiles = append(messageProfiles, load.MessageProfileConfig{
			Name:          messageProfileName,
			Finality:      1,
			HasData:       true,
			DataSizeBytes: int(cfg.msgSize),
		})
	} else {
		messageProfiles = append(messageProfiles, load.MessageProfileConfig{
			Name:     messageProfileName,
			Finality: 1,
		})
	}
	testProfile := &load.TestProfileConfig{
		ChainsAsSource: []load.ChainProfileConfig{{
			Selector: strconv.FormatUint(env.evmSelector, 10),
			Ratio:    1,
		}},
		ChainsAsDest: []load.ChainProfileConfig{{
			Selector: strconv.FormatUint(env.solSelector, 10),
			Ratio:    1,
		}},
		Messages: []load.MessageTestRatio{
			{Ratio: 1, MessageProfile: messageProfileName},
		},
	}

	opts := []ccve2e.EVMTXGunOption{ccve2e.WithExecutorArgsParams(arbExecutorArgs)}
	if useFireAndForget {
		opts = append(opts, ccve2e.WithFireAndForget())
	}

	gun, err := ccve2e.NewEVMTransactionGunFromTestConfig(
		env.cfg, testProfile, messageProfiles, env.deployEnv, env.impls, opts...,
	)
	require.NoError(t, err)

	runLoadProfile(ctx, t, env.impls, gun, cfg,
		withExecTimeout(60*time.Second),
		// EVMTXGun picks profiles at random per send so actual counts may differ slightly
		withMessageProfileMix(messageProfiles, testProfile.Messages),
	)
}
