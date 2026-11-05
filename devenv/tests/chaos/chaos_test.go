package chaos

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	chainsel "github.com/smartcontractkit/chain-selectors"
	ccv "github.com/smartcontractkit/chainlink-ccv/build/devenv"
	"github.com/smartcontractkit/chainlink-ccv/build/devenv/cciptestinterfaces"
	"github.com/smartcontractkit/chainlink-ccv/build/devenv/chainreg"
	devenvcommon "github.com/smartcontractkit/chainlink-ccv/build/devenv/common"
	"github.com/smartcontractkit/chainlink-ccv/build/devenv/localnetwork"
	e2ehelpers "github.com/smartcontractkit/chainlink-ccv/build/devenv/tests/e2e"
	"github.com/smartcontractkit/chainlink-ccv/build/devenv/tests/e2e/tcapi"
	"github.com/smartcontractkit/chainlink-ccv/build/devenv/tests/e2e/tcapi/basic"
	"github.com/smartcontractkit/chainlink-ccv/build/devenv/tests/e2e/tcapi/chaos"

	testreceiverops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/testreceiver"
	soldevenv "github.com/smartcontractkit/chainlink-ccip-solana/devenv"
	"github.com/smartcontractkit/chainlink-ccip-solana/devenv/ccvregistry"
	devenvutils "github.com/smartcontractkit/chainlink-ccip-solana/devenv/tests/utils"
)

const (
	chaosExecTimeout = 6 * time.Minute
	chaosSentTimeout = 3 * time.Minute
)

func TestChaos_SolanaRPCFailover(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}
	ctx, lib, in, evmSel, solSel := setupChaosSession(t)

	proxies, err := chaos.RPCFailoverChainFor(in, solSel)
	require.NoError(t, err, "test requires the Solana local-network failover profile")
	requireDirectTestRPC(t, in, solSel, proxies)

	cases := []struct {
		name string
		run  func(ctx context.Context, t *testing.T) error
	}{
		{
			name: "source reader switches RPC",
			run: func(ctx context.Context, t *testing.T) error {
				_, err := basic.EOAReceiverDefaultVerifier(lib, solSel, evmSel, basic.Args{
					Send: tcapi.SendArgs{SendOption: devenvutils.MustSolSendOptions(t)},
					Run: tcapi.RunConfig{
						ConfirmSentTimeout: chaosSentTimeout,
						ConfirmExecTimeout: chaosExecTimeout,
					},
				}).Run(ctx)
				return err
			},
		},
		{
			name: "destination transmitter switches RPC",
			run: func(ctx context.Context, t *testing.T) error {
				v3Src, err := lib.V3Source(ctx, evmSel)
				if err != nil {
					return err
				}
				v3Dst, err := lib.V3Destination(ctx, solSel)
				if err != nil {
					return err
				}

				receiver, ccvs, executor, err := devenvutils.ResolveSendAddressesWithReceiverQualifier(
					ctx, lib, evmSel, solSel, devenvcommon.DefaultReceiverQualifier,
				)
				if err != nil {
					return err
				}

				sentEvt, _, err := tcapi.SendV3Message(ctx, v3Src, v3Dst,
					cciptestinterfaces.MessageFields{
						Receiver: receiver,
						Data:     []byte{0xde, 0xad, 0xbe, 0xef},
					},
					cciptestinterfaces.MessageOptions{
						FinalityConfig: 1,
						Executor:       executor,
						CCVs:           ccvs,
					},
					solanaDestinationSendArgs(ctx, t, lib, evmSel, solSel),
				)
				if err != nil {
					return err
				}

				messageKey := cciptestinterfaces.MessageEventKey{MessageID: sentEvt.MessageID}
				_, err = v3Src.ConfirmSendOnSource(ctx, solSel, messageKey, chaosSentTimeout)
				if err != nil {
					return err
				}

				execEnv, err := v3Dst.ConfirmExecOnDest(ctx, evmSel, messageKey, chaosExecTimeout)
				if err != nil {
					return err
				}
				if execEnv.Event.State != cciptestinterfaces.ExecutionStateSuccess {
					return fmt.Errorf("execution on Solana failed: state %d", execEnv.Event.State)
				}
				return nil
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Cleanup(func() { chaos.RestoreRPCProxies(ctx, proxies) })

			toggleProxy := func(start, stop string) {
				if e := chaos.SetRPCProxyRunning(ctx, start, true); e != nil {
					t.Error(e)
				}
				if e := chaos.SetRPCProxyRunning(ctx, stop, false); e != nil {
					t.Error(e)
				}
			}

			// Stop the primary proxy to force the client to fail over to the secondary.
			go toggleProxy(proxies.SecondaryContainerName, proxies.PrimaryContainerName)
			require.NoError(t, tc.run(ctx, t))

			// Stop the secondary proxy to force the client to fail over to the primary.
			go toggleProxy(proxies.PrimaryContainerName, proxies.SecondaryContainerName)
			require.NoError(t, tc.run(ctx, t))
		})
	}
}

func requireDirectTestRPC(t *testing.T, in *ccv.Cfg, selector uint64, proxies *localnetwork.FailoverChainOutput) {
	t.Helper()
	directNode, err := chaos.DirectRPCNode(in, selector)
	require.NoError(t, err)
	require.NotNil(t, proxies.PrimaryNode)
	require.NotNil(t, proxies.SecondaryNode)
	require.NotEqual(t, proxies.PrimaryNode.InternalHTTPUrl, directNode.InternalHTTPUrl)
	require.NotEqual(t, proxies.SecondaryNode.InternalHTTPUrl, directNode.InternalHTTPUrl)
}

// TestChaos_AggregatorOutageRecovery stops the default aggregator's nginx proxy
// and verifies that an EVM→Solana V3 message survives the aggregator outage.
func TestChaos_AggregatorOutageRecovery(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}
	ctx, lib, in, evmSel, solSel := setupChaosSession(t)

	aggregator, err := chaos.DefaultAggregatorNginx(in, devenvcommon.DefaultCommitteeVerifierQualifier)
	require.NoError(t, err)

	runChaosScenario(ctx, t, lib, evmSel, solSel, &chaos.OutageSpec{
		Duration: chaos.DefaultOutageDuration,
		Targets:  []string{aggregator},
	}, nil)
}

// TestChaos_ExecutorOutageRecovery stops executors serving the Solana
// destination chain and verifies message execution after the executor restarts.
func TestChaos_ExecutorOutageRecovery(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}
	ctx, lib, in, evmSel, solSel := setupChaosSession(t)

	executors, err := chaos.ExecutorContainersForDest(in, solSel, devenvcommon.DefaultExecutorQualifier)
	require.NoError(t, err)

	runChaosScenario(ctx, t, lib, evmSel, solSel, &chaos.OutageSpec{
		Duration: chaos.DefaultOutageDuration,
		Targets:  executors,
	}, nil)
}

// TestChaos_RPCLatency injects network latency on both EVM and Solana blockchain
// RPC containers for 1 minute, then sends an EVM→Solana V3 message and confirms
// execution succeeds despite the added latency.
func TestChaos_RPCLatency(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}
	ctx, lib, in, evmSel, solSel := setupChaosSession(t)

	evmRPC, err := chaos.BlockchainContainerForSelector(in, evmSel)
	require.NoError(t, err)
	solRPC, err := chaos.BlockchainContainerForSelector(in, solSel)
	require.NoError(t, err)

	runChaosScenario(ctx, t, lib, evmSel, solSel, nil, &chaos.LatencySpec{
		Duration: 1 * time.Minute,
		Delay:    400,
		Targets:  []string{evmRPC, solRPC},
	})
}

// setupChaosSession registers Solana, loads the devenv, and resolves the
// EVM (source) and Solana (destination) chain selectors.
func setupChaosSession(t *testing.T) (context.Context, ccv.Lib, *ccv.Cfg, uint64, uint64) {
	t.Helper()

	factory := soldevenv.NewImplFactory()
	require.NoError(t, chainreg.Register(chainsel.FamilySolana, chainreg.Registration{
		ImplFactory:              factory,
		ExecutorInfo:             factory,
		CLDFProvider:             ccvregistry.SolanaCLDFProviderFactory,
		LocalNetworkConfigurator: localnetwork.Configurator(chainsel.FamilySolana),
		AddressResolver:          soldevenv.NewAddressResolver(),
	}))

	ctx := ccv.Plog.WithContext(t.Context())
	configPath := e2ehelpers.GetSmokeTestConfig()

	in, err := ccv.LoadOutput[ccv.Cfg](configPath)
	require.NoError(t, err)

	lib, err := ccv.NewLibFromCCVEnv(zerolog.Ctx(ctx), configPath)
	require.NoError(t, err)

	chains, err := lib.Chains(ctx)
	require.NoError(t, err)

	var evmSel, solSel uint64
	var evmChains, solChains []ccv.ChainImpl
	for _, c := range chains {
		family, _ := chainsel.GetSelectorFamily(c.Details.ChainSelector)
		switch family {
		case chainsel.FamilyEVM:
			evmChains = append(evmChains, c)
		case chainsel.FamilySolana:
			solChains = append(solChains, c)
		}
	}
	sort.Slice(evmChains, func(i, j int) bool { return evmChains[i].Details.ChainSelector < evmChains[j].Details.ChainSelector })
	sort.Slice(solChains, func(i, j int) bool { return solChains[i].Details.ChainSelector < solChains[j].Details.ChainSelector })
	require.NotEmpty(t, evmChains, "no EVM chain found")
	require.NotEmpty(t, solChains, "no Solana chain found")
	evmSel = evmChains[0].Details.ChainSelector
	solSel = solChains[0].Details.ChainSelector
	t.Cleanup(func() {
		if closer, canClose := solChains[0].CCIP17.(interface{ Close() error }); canClose {
			_ = closer.Close()
		}
	})

	return ctx, lib, in, evmSel, solSel
}

// runChaosScenario builds an EVM→Solana V3 message and runs the chaos scenario.
// Exactly one of outage or latency must be non-nil.
func runChaosScenario(ctx context.Context, t *testing.T, lib ccv.Lib, src, dst uint64, outage *chaos.OutageSpec, latency *chaos.LatencySpec) {
	t.Helper()
	spec := solanaDestinationScenarioSpec(ctx, t, lib, src, dst)
	spec.Outage = outage
	spec.Latency = latency
	require.NoError(t, chaos.RunScenario(t, ctx, spec))
}

func solanaDestinationScenarioSpec(ctx context.Context, t *testing.T, lib ccv.Lib, src, dst uint64) chaos.ScenarioSpec {
	t.Helper()

	receiver, ccvs, executor, err := devenvutils.ResolveSendAddressesWithReceiverQualifier(
		ctx, lib, src, dst, devenvcommon.DefaultReceiverQualifier,
	)
	require.NoError(t, err)

	return chaos.ScenarioSpec{
		Lib: lib,
		Src: src,
		Dst: dst,
		Fields: cciptestinterfaces.MessageFields{
			Receiver: receiver,
			Data:     []byte{0xde, 0xad, 0xbe, 0xef},
		},
		Opts: cciptestinterfaces.MessageOptions{
			FinalityConfig: 1,
			Executor:       executor,
			CCVs:           ccvs,
		},
		SendArgs: solanaDestinationSendArgs(ctx, t, lib, src, dst),
		Assert: tcapi.AssertMessageOptions{
			TickInterval:            5 * time.Second,
			Timeout:                 chaosExecTimeout,
			ExpectedVerifierResults: 1,
		},
		Run: tcapi.RunConfig{
			ConfirmSentTimeout: chaosSentTimeout,
			ConfirmExecTimeout: chaosExecTimeout,
		},
		ConfirmExecOnDest: true,
	}
}

func solanaDestinationSendArgs(ctx context.Context, t *testing.T, lib ccv.Lib, src, dst uint64) tcapi.SendArgs {
	t.Helper()

	chains, err := lib.ChainsMap(ctx)
	require.NoError(t, err)
	srcChain, ok := chains[src]
	require.True(t, ok, "source chain (selector %d) not found", src)

	srcSender, err := srcChain.GetSenderAddress()
	require.NoError(t, err)

	// Resolve the receiver from the datastore rather than the gobinding declare_id!, which only
	// matches in devenv.
	ds, dsErr := lib.DataStore()
	require.NoError(t, dsErr)
	receiverV2ProgramID, recvErr := soldevenv.ResolveTestReceiverAddress(ds, dst, testreceiverops.Version)
	require.NoError(t, recvErr)
	executorArgs, err := devenvutils.BuildV2ReceiverExecutorArgs(src, srcSender, receiverV2ProgramID)
	require.NoError(t, err)

	return tcapi.SendArgs{
		ExecutionGasLimit: devenvutils.MaxComputeUnits,
		ExtraArgsParams:   executorArgs,
	}
}
