package accessors

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	chainsel "github.com/smartcontractkit/chain-selectors"
	"github.com/smartcontractkit/chainlink-ccv/pkg/chainaccess"
	"github.com/smartcontractkit/chainlink-ccv/protocol"
	"github.com/smartcontractkit/chainlink-common/pkg/config"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	solconfig "github.com/smartcontractkit/chainlink-solana/pkg/solana/config"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/ccip_offramp"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/committee_verifier"
	"github.com/smartcontractkit/chainlink-ccip-solana/pkg/chainselector"
	ccip "github.com/smartcontractkit/chainlink-ccip-solana/pkg/config"
	"github.com/smartcontractkit/chainlink-ccip-solana/pkg/destinationreader"
	"github.com/smartcontractkit/chainlink-ccip-solana/pkg/solanarpc"
	"github.com/smartcontractkit/chainlink-ccip-solana/pkg/solutil"
)

func testSolSelector() protocol.ChainSelector {
	return protocol.ChainSelector(chainsel.TEST_22222222222222222222222222222222222222222222.Selector)
}

func mustNodes(t *testing.T, urls ...string) []*solconfig.Node {
	t.Helper()
	nodes := make([]*solconfig.Node, 0, len(urls))
	for _, u := range urls {
		name := "primary"
		nodes = append(nodes, &solconfig.Node{Name: &name, URL: config.MustParseURL(u)})
	}
	require.NoError(t, solanarpc.ValidateAndNormalizeNodes(nodes))
	return nodes
}

func testSolLocalSetting(t *testing.T) (protocol.ChainSelector, string, localConfig) {
	t.Helper()
	sel := testSolSelector()
	return sel, chainselector.Format(sel), localConfig{
		chainID:    "ccip-solana-factory-test",
		nodes:      mustNodes(t, "http://127.0.0.1:9"),
		rpcTimeout: ccip.DefaultRPCRequestTimeout,
	}
}

// accessorFixture holds inputs to NewFactory plus the selector passed to GetAccessor.
type accessorFixture struct {
	sel           protocol.ChainSelector
	remoteConfigs remoteJDConfigs
	localSettings map[protocol.ChainSelector]localConfig
}

func TestGetAccessor(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	evmSel := protocol.ChainSelector(chainsel.ETHEREUM_MAINNET.Selector)
	evmRPCURL := "http://127.0.0.1:9"

	tests := []struct {
		name         string
		fixture      func(t *testing.T) accessorFixture
		wantErr      bool
		errSubstring string
		assertAcc    func(t *testing.T, acc chainaccess.Accessor)
	}{
		{
			name: "non_solana_selector_rejected",
			fixture: func(t *testing.T) accessorFixture {
				t.Helper()
				return accessorFixture{
					sel: evmSel,
					localSettings: map[protocol.ChainSelector]localConfig{
						evmSel: {chainID: "1", nodes: mustNodes(t, evmRPCURL)},
					},
				}
			},
			wantErr:      true,
			errSubstring: "only solana is supported",
		},
		{
			name: "missing_rpc_url",
			fixture: func(t *testing.T) accessorFixture {
				t.Helper()
				sel, strSel, settings := testSolLocalSetting(t)
				settings.nodes = nil
				return accessorFixture{
					sel: sel,
					remoteConfigs: remoteJDConfigs{
						OnRampAddresses: map[string]string{
							strSel: solutil.PublicKeyToHex(ccip_offramp.ProgramID),
						},
						RMNRemoteAddresses: map[string]string{
							strSel: solutil.PublicKeyToHex(committee_verifier.ProgramID),
						},
					},
					localSettings: map[protocol.ChainSelector]localConfig{sel: settings},
				}
			},
			wantErr:      true,
			errSubstring: "no RPC nodes available",
		},
		{
			name: "missing_chain_settings",
			fixture: func(t *testing.T) accessorFixture {
				t.Helper()
				sel, strSel, _ := testSolLocalSetting(t)
				return accessorFixture{
					sel: sel,
					remoteConfigs: remoteJDConfigs{
						OnRampAddresses: map[string]string{
							strSel: solutil.PublicKeyToHex(ccip_offramp.ProgramID),
						},
						RMNRemoteAddresses: map[string]string{
							strSel: solutil.PublicKeyToHex(committee_verifier.ProgramID),
						},
					},
					localSettings: map[protocol.ChainSelector]localConfig{},
				}
			},
			wantErr:      true,
			errSubstring: "no RPC nodes available",
		},
		{
			name: "invalid_onRamp_address",
			fixture: func(t *testing.T) accessorFixture {
				t.Helper()
				sel, strSel, settings := testSolLocalSetting(t)
				return accessorFixture{
					sel: sel,
					remoteConfigs: remoteJDConfigs{
						OnRampAddresses: map[string]string{
							strSel: "not-valid-hex",
						},
						RMNRemoteAddresses: map[string]string{
							strSel: solutil.PublicKeyToHex(committee_verifier.ProgramID),
						},
					},
					localSettings: map[protocol.ChainSelector]localConfig{sel: settings},
				}
			},
			wantErr:      true,
			errSubstring: "invalid onRamp address",
		},
		{
			name: "invalid_rmnRemote_address",
			fixture: func(t *testing.T) accessorFixture {
				t.Helper()
				sel, strSel, settings := testSolLocalSetting(t)
				return accessorFixture{
					sel: sel,
					remoteConfigs: remoteJDConfigs{
						OnRampAddresses: map[string]string{
							strSel: solutil.PublicKeyToHex(ccip_offramp.ProgramID),
						},
						RMNRemoteAddresses: map[string]string{
							strSel: "0xdead",
						},
					},
					localSettings: map[protocol.ChainSelector]localConfig{sel: settings},
				}
			},
			wantErr:      true,
			errSubstring: "invalid rmnRemote address",
		},
		{
			name: "destination_invalid_offRamp_hex",
			fixture: func(t *testing.T) accessorFixture {
				t.Helper()
				sel, strSel, settings := testSolLocalSetting(t)
				return accessorFixture{
					sel: sel,
					remoteConfigs: remoteJDConfigs{
						DestinationChain: map[string]chainaccess.DestinationChainConfig{
							strSel: {OffRampAddress: "not-32-bytes"},
						},
					},
					localSettings: map[protocol.ChainSelector]localConfig{sel: settings},
				}
			},
			wantErr:      true,
			errSubstring: "invalid offRamp address",
		},
		{
			name: "destination_no_rpc_url_on_chain_info",
			fixture: func(t *testing.T) accessorFixture {
				t.Helper()
				sel, strSel, _ := testSolLocalSetting(t)
				settingsNoURL := localConfig{
					chainID: "ccip-solana-factory-test-no-url",
				}
				return accessorFixture{
					sel: sel,
					remoteConfigs: remoteJDConfigs{
						DestinationChain: map[string]chainaccess.DestinationChainConfig{
							strSel: {},
						},
					},
					localSettings: map[protocol.ChainSelector]localConfig{sel: settingsNoURL},
				}
			},
			wantErr:      true,
			errSubstring: "no RPC nodes available",
		},
		{
			name: "source_only_wires_source_reader",
			fixture: func(t *testing.T) accessorFixture {
				t.Helper()
				sel, strSel, settings := testSolLocalSetting(t)
				return accessorFixture{
					sel: sel,
					remoteConfigs: remoteJDConfigs{
						OnRampAddresses: map[string]string{
							strSel: solutil.PublicKeyToHex(ccip_offramp.ProgramID),
						},
						RMNRemoteAddresses: map[string]string{
							strSel: solutil.PublicKeyToHex(committee_verifier.ProgramID),
						},
					},
					localSettings: map[protocol.ChainSelector]localConfig{sel: settings},
				}
			},
			wantErr: false,
			assertAcc: func(t *testing.T, acc chainaccess.Accessor) {
				t.Helper()
				t.Cleanup(func() { require.NoError(t, acc.Close()) })

				sr, sErr := acc.SourceReader()
				require.NoError(t, sErr)
				require.NotNil(t, sr)

				_, dErr := acc.DestinationReader()
				require.Error(t, dErr)
				require.Contains(t, dErr.Error(), "not configured")

				_, cErr := acc.ContractTransmitter()
				require.Error(t, cErr)
				require.Contains(t, cErr.Error(), "not configured")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fx := tc.fixture(t)
			f := NewFactory(logger.Nop(), fx.remoteConfigs, fx.localSettings)
			acc, err := f.GetAccessor(ctx, fx.sel)
			if tc.wantErr {
				require.Error(t, err)
				if tc.errSubstring != "" {
					require.Contains(t, err.Error(), tc.errSubstring)
				}
				require.Nil(t, acc)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, acc)
			if tc.assertAcc != nil {
				tc.assertAcc(t, acc)
			}
		})
	}
}

func TestCreateAccessorFactory_loadsLocalConfig(t *testing.T) {
	ctx := context.Background()

	sel, strSel, _ := testSolLocalSetting(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "solana-config.toml")
	require.NoError(t, os.WriteFile(path, []byte(`
[chains."`+strSel+`"]
[[chains."`+strSel+`".nodes]]
name = "primary"
url = "http://127.0.0.1:9"
`), 0o600))
	t.Setenv(ccip.SolanaConfigPathEnv, path)

	fac, err := CreateAccessorFactory(logger.Nop(), chainaccess.GenericConfig{
		CommitteeConfig: chainaccess.CommitteeConfig{
			OnRampAddresses: map[string]string{
				strSel: solutil.PublicKeyToHex(ccip_offramp.ProgramID),
			},
			RMNRemoteAddresses: map[string]string{
				strSel: solutil.PublicKeyToHex(committee_verifier.ProgramID),
			},
		},
	})
	require.NoError(t, err)

	acc, err := fac.GetAccessor(ctx, sel)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, acc.Close()) })

	sr, err := acc.SourceReader()
	require.NoError(t, err)
	require.NotNil(t, sr)
}

func TestGetAccessorCreatesIndependentClients(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	sel, strSel, settings := testSolLocalSetting(t)
	f := NewFactory(
		logger.Nop(),
		remoteJDConfigs{
			OnRampAddresses: map[string]string{
				strSel: solutil.PublicKeyToHex(ccip_offramp.ProgramID),
			},
			RMNRemoteAddresses: map[string]string{
				strSel: solutil.PublicKeyToHex(committee_verifier.ProgramID),
			},
		},
		map[protocol.ChainSelector]localConfig{sel: settings},
	)

	first, err := f.GetAccessor(ctx, sel)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, first.Close()) })

	second, err := f.GetAccessor(ctx, sel)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, second.Close()) })

	firstAccessor := first.(*accessor)
	secondAccessor := second.(*accessor)
	require.NotSame(t, firstAccessor.rpc, secondAccessor.rpc)
	require.NoError(t, first.Close())
	require.NoError(t, second.Close())
}

// TestNewFactory_DefaultsMaxRetryDuration pins the factory boundary default: an omitted
// job spec duration must give the executor's default retry window, so the attempt cache
// does not shrink to the poller lookback. The pattern mirrors the pinned EVM factory.
func TestNewFactory_DefaultsMaxRetryDuration(t *testing.T) {
	t.Parallel()

	sel, strSel, settings := testSolLocalSetting(t)
	remote := remoteJDConfigs{
		OnRampAddresses: map[string]string{
			strSel: solutil.PublicKeyToHex(ccip_offramp.ProgramID),
		},
		RMNRemoteAddresses: map[string]string{
			strSel: solutil.PublicKeyToHex(committee_verifier.ProgramID),
		},
	}

	t.Run("omitted_duration_gets_executor_default", func(t *testing.T) {
		t.Parallel()
		f := NewFactory(logger.Test(t), remote, map[protocol.ChainSelector]localConfig{sel: settings})
		require.Equal(t, defaultMaxRetryDuration, f.(*factory).remoteConfigs.MaxRetryDuration)
	})

	t.Run("explicit_duration_is_kept", func(t *testing.T) {
		t.Parallel()
		explicit := remote
		explicit.MaxRetryDuration = 2 * time.Hour
		f := NewFactory(logger.Test(t), explicit, map[protocol.ChainSelector]localConfig{sel: settings})
		require.Equal(t, 2*time.Hour, f.(*factory).remoteConfigs.MaxRetryDuration)
	})
}

// TestApplyRetryFloor pins the retry floor: the attempt cache TTL must cover the
// JD MaxRetryDuration, and a longer configured TTL must survive.
func TestApplyRetryFloor(t *testing.T) {
	t.Parallel()

	base := destinationreader.DefaultConfig() // TTL = 15m lookback

	t.Run("longer_retry_raises_ttl", func(t *testing.T) {
		t.Parallel()
		got := applyRetryFloor(base, 8*time.Hour)
		require.Equal(t, 8*time.Hour, got.AttemptCacheTTL.Duration())
	})

	t.Run("shorter_retry_keeps_configured_ttl", func(t *testing.T) {
		t.Parallel()
		cfg := base
		cfg.AttemptCacheTTL = *config.MustNewDuration(24 * time.Hour)
		got := applyRetryFloor(cfg, 8*time.Hour)
		require.Equal(t, 24*time.Hour, got.AttemptCacheTTL.Duration())
	})

	t.Run("zero_retry_keeps_configured_ttl", func(t *testing.T) {
		t.Parallel()
		got := applyRetryFloor(base, 0)
		require.Equal(t, base.AttemptCacheTTL, got.AttemptCacheTTL)
	})
}

func TestValidateJDChainsHaveLocalConfig(t *testing.T) {
	t.Parallel()

	sel, strSel, settings := testSolLocalSetting(t)
	sel333 := protocol.ChainSelector(chainsel.TEST_33333333333333333333333333333333333333333333.Selector)
	strSel333 := chainselector.Format(sel333)
	evmSel := protocol.ChainSelector(chainsel.ETHEREUM_MAINNET.Selector)

	tests := []struct {
		name        string
		remote      remoteJDConfigs
		local       map[protocol.ChainSelector]localConfig
		wantErr     bool
		errContains string
	}{
		{
			name: "ignores_partial_source_config",
			remote: remoteJDConfigs{
				OnRampAddresses: map[string]string{strSel: solutil.PublicKeyToHex(ccip_offramp.ProgramID)},
			},
			local: map[protocol.ChainSelector]localConfig{},
		},
		{
			name: "missing_source_config_when_both_addresses_set",
			remote: remoteJDConfigs{
				OnRampAddresses: map[string]string{
					strSel: solutil.PublicKeyToHex(ccip_offramp.ProgramID),
				},
				RMNRemoteAddresses: map[string]string{
					strSel: solutil.PublicKeyToHex(committee_verifier.ProgramID),
				},
			},
			local:       map[protocol.ChainSelector]localConfig{},
			wantErr:     true,
			errContains: "missing RPC for JD-wired chains",
		},
		{
			name: "missing_destination_config",
			remote: remoteJDConfigs{
				DestinationChain: map[string]chainaccess.DestinationChainConfig{
					strSel: {OffRampAddress: solutil.PublicKeyToHex(ccip_offramp.ProgramID)},
				},
			},
			local:       map[protocol.ChainSelector]localConfig{},
			wantErr:     true,
			errContains: "missing RPC for JD-wired chains",
		},
		{
			name: "matching_local_config",
			remote: remoteJDConfigs{
				DestinationChain: map[string]chainaccess.DestinationChainConfig{
					strSel: {OffRampAddress: solutil.PublicKeyToHex(ccip_offramp.ProgramID)},
				},
			},
			local: map[protocol.ChainSelector]localConfig{sel: settings},
		},
		{
			name: "skips_non_solana_selectors",
			remote: remoteJDConfigs{
				DestinationChain: map[string]chainaccess.DestinationChainConfig{
					chainselector.Format(evmSel): {},
				},
			},
			local: map[protocol.ChainSelector]localConfig{},
		},
		{
			name: "non_canonical_job_selector",
			remote: remoteJDConfigs{
				DestinationChain: map[string]chainaccess.DestinationChainConfig{"0" + strSel: {}},
			},
			local:       map[protocol.ChainSelector]localConfig{sel: settings},
			wantErr:     true,
			errContains: "non-canonical decimal string",
		},
		{
			name: "invalid_job_selector",
			remote: remoteJDConfigs{
				DestinationChain: map[string]chainaccess.DestinationChainConfig{
					"not-a-selector": {},
				},
			},
			local:       map[protocol.ChainSelector]localConfig{},
			wantErr:     true,
			errContains: "job spec chain selector",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateJDChainsHaveLocalConfig(tc.remote, tc.local)
			if tc.wantErr {
				require.Error(t, err)
				if tc.errContains != "" {
					require.Contains(t, err.Error(), tc.errContains)
				}
				return
			}
			require.NoError(t, err)
		})
	}

	t.Run("lists_all_missing_selectors", func(t *testing.T) {
		t.Parallel()
		err := validateJDChainsHaveLocalConfig(remoteJDConfigs{
			OnRampAddresses: map[string]string{
				strSel:    solutil.PublicKeyToHex(ccip_offramp.ProgramID),
				strSel333: solutil.PublicKeyToHex(ccip_offramp.ProgramID),
			},
			RMNRemoteAddresses: map[string]string{
				strSel:    solutil.PublicKeyToHex(committee_verifier.ProgramID),
				strSel333: solutil.PublicKeyToHex(committee_verifier.ProgramID),
			},
		}, map[protocol.ChainSelector]localConfig{})
		require.Error(t, err)
		require.Contains(t, err.Error(), strSel)
		require.Contains(t, err.Error(), strSel333)
	})
}

func TestCreateAccessorFactoryRejectsNonCanonicalLocalSelector(t *testing.T) {
	_, strSel, _ := testSolLocalSetting(t)
	path := filepath.Join(t.TempDir(), "solana-config.toml")
	require.NoError(t, os.WriteFile(path, []byte(`
[chains."`+strSel+`"]
[[chains."`+strSel+`".nodes]]
name = "primary"
url = "http://127.0.0.1:9"
[chains."0`+strSel+`"]
[[chains."0`+strSel+`".nodes]]
name = "other"
url = "http://127.0.0.1:10"
`), 0o600))
	t.Setenv(ccip.SolanaConfigPathEnv, path)
	_, err := CreateAccessorFactory(logger.Nop(), chainaccess.GenericConfig{})
	require.ErrorContains(t, err, "non-canonical decimal string")
}
