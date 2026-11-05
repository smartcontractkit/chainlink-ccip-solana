package ccvregistry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"

	chainsel "github.com/smartcontractkit/chain-selectors"
	"github.com/smartcontractkit/chainlink-ccv/build/devenv/services/executor"
	"github.com/smartcontractkit/chainlink-ccv/protocol"
	ctfblockchain "github.com/smartcontractkit/chainlink-testing-framework/framework/components/blockchain"

	solaccessors "github.com/smartcontractkit/chainlink-ccip-solana/pkg/accessors"
	"github.com/smartcontractkit/chainlink-ccip-solana/pkg/chainselector"
	ccip "github.com/smartcontractkit/chainlink-ccip-solana/pkg/config"
)

// fakeGenesisHashServer returns a test HTTP server that responds to getGenesisHash
// with the given hash.
func fakeGenesisHashServer(t *testing.T, hash string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","result":"` + hash + `","id":1}`))
	}))
}

func TestSolanaChainConfigLoader_MetadataOnly(t *testing.T) {
	t.Parallel()

	outputs := []*ctfblockchain.Output{{
		ChainID:       "EtWTRABZaYq6iMfeYKouRu166VU2xqa1wcaWoxPkrZBG",
		Family:        chainsel.FamilySolana,
		Type:          "solana",
		ContainerName: "solana-0",
		Nodes: []*ctfblockchain.Node{{
			ExternalHTTPUrl: "http://real-rpc:8899",
			InternalHTTPUrl: "http://internal-rpc:8899",
		}},
	}}

	infos, err := SolanaChainConfigLoader(outputs)
	require.NoError(t, err)
	require.Len(t, infos, 1)

	for _, v := range infos {
		info, ok := v.(solaccessors.Info)
		require.True(t, ok)
		require.Equal(t, "EtWTRABZaYq6iMfeYKouRu166VU2xqa1wcaWoxPkrZBG", info.ChainID)
		require.Equal(t, chainsel.FamilySolana, info.Family)
		require.Equal(t, "solana-0", info.UniqueChainName)
	}
}

func TestBuildSolanaNOPConfig_PreservesAllRPCNodes(t *testing.T) {
	t.Parallel()

	srv := fakeGenesisHashServer(t, "EtWTRABZaYq6iMfeYKouRu166VU2xqa1wcaWoxPkrZBG")
	t.Cleanup(srv.Close)

	outputs := []*ctfblockchain.Output{{
		ChainID:       "EtWTRABZaYq6iMfeYKouRu166VU2xqa1wcaWoxPkrZBG",
		Family:        chainsel.FamilySolana,
		Type:          "solana",
		ContainerName: "solana-0",
		Nodes: []*ctfblockchain.Node{{
			ExternalHTTPUrl: srv.URL,
			InternalHTTPUrl: "http://primary-rpc:8899",
		}, {
			ExternalHTTPUrl: "http://localhost:18899",
			InternalHTTPUrl: "http://secondary-rpc:8899",
		}},
	}}

	cfg, err := buildSolanaLocalConfig(context.Background(), outputs)
	require.NoError(t, err)

	details, err := chainsel.GetChainDetailsByChainIDAndFamily(outputs[0].ChainID, outputs[0].Family)
	require.NoError(t, err)
	strSelector := chainselector.Format(protocol.ChainSelector(details.ChainSelector))

	chain := cfg.Chains[strSelector]
	require.Len(t, chain.Nodes, 2)
	require.Equal(t, "http://primary-rpc:8899", chain.Nodes[0].URL.String())
	require.Equal(t, "http://secondary-rpc:8899", chain.Nodes[1].URL.String())

	configBytes, err := toml.Marshal(cfg)
	require.NoError(t, err)

	var roundtrip ccip.Config
	_, err = toml.Decode(string(configBytes), &roundtrip)
	require.NoError(t, err)
	roundtripChain := roundtrip.Chains[strSelector]
	require.Len(t, roundtripChain.Nodes, 2)
	require.Equal(t, "http://primary-rpc:8899", roundtripChain.Nodes[0].URL.String())
	require.Equal(t, "http://secondary-rpc:8899", roundtripChain.Nodes[1].URL.String())
}

func TestSolanaExecutorModifier_MountsNOPConfig(t *testing.T) {
	t.Parallel()

	srv := fakeGenesisHashServer(t, "EtWTRABZaYq6iMfeYKouRu166VU2xqa1wcaWoxPkrZBG")
	t.Cleanup(srv.Close)

	outputs := []*ctfblockchain.Output{{
		ChainID:       "EtWTRABZaYq6iMfeYKouRu166VU2xqa1wcaWoxPkrZBG",
		Family:        chainsel.FamilySolana,
		Type:          "solana",
		ContainerName: "solana-0",
		Nodes: []*ctfblockchain.Node{{
			ExternalHTTPUrl: srv.URL,
			InternalHTTPUrl: "http://internal-rpc:8899",
		}},
	}}

	req, err := SolanaExecutorModifier(testcontainers.ContainerRequest{}, &executor.Input{
		ContainerName: "solana-executor-1",
	}, outputs)
	require.NoError(t, err)
	require.Equal(t, "solana-solana-executor-1", req.Name)
	require.Len(t, req.Files, 1)
	require.Equal(t, ccip.DefaultSolanaConfigPath, req.Files[0].ContainerFilePath)
}
