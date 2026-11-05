package solanarpc_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/config"
	solconfig "github.com/smartcontractkit/chainlink-solana/pkg/solana/config"

	"github.com/smartcontractkit/chainlink-ccip-solana/pkg/solanarpc"
)

func TestValidateNodes_RejectsOutOfRangeOrder(t *testing.T) {
	name := "n"
	base := solconfig.Node{Name: &name, URL: config.MustParseURL("http://localhost:8899")}
	for _, order := range []int32{0, -1, 101, 1000} {
		o := order
		n := base
		n.Order = &o
		require.Error(t, solanarpc.ValidateAndNormalizeNodes([]*solconfig.Node{&n}))
	}
	for _, order := range []int32{1, 50, 100} {
		o := order
		n := base
		n.Order = &o
		require.NoError(t, solanarpc.ValidateAndNormalizeNodes([]*solconfig.Node{&n}))
	}
}

func TestValidateNodes_DefaultsAndValidation(t *testing.T) {
	nodes := []*solconfig.Node{{URL: config.MustParseURL("http://localhost:8899")}}
	require.NoError(t, solanarpc.ValidateAndNormalizeNodes(nodes))
	require.Len(t, nodes, 1)

	n := nodes[0]
	require.Equal(t, "node-0", *n.Name, "unnamed node defaults to node-<i>")
	require.Equal(t, "http://localhost:8899", n.URL.String())
	require.Equal(t, int32(100), *n.Order, "unset order defaults to 100")
	require.False(t, *n.IsLoadBalancedRPC)

	emptyName := ""
	blank := []*solconfig.Node{{Name: &emptyName, URL: config.MustParseURL("http://localhost:8899")}}
	require.NoError(t, solanarpc.ValidateAndNormalizeNodes(blank))
	require.Equal(t, "node-0", *blank[0].Name, "blank name defaults to node-<i>")

	missingURL := "n1"
	require.Error(t, solanarpc.ValidateAndNormalizeNodes([]*solconfig.Node{{Name: &missingURL}}), "missing url is rejected")
}

func TestValidateNodes_RejectsDuplicates(t *testing.T) {
	explicit := "node-1"
	nodes := []*solconfig.Node{
		{Name: &explicit, URL: config.MustParseURL("http://a:8899")},
		{URL: config.MustParseURL("http://b:8899")},
	}
	require.ErrorContains(t, solanarpc.ValidateAndNormalizeNodes(nodes), `duplicate name "node-1"`)

	sameURL := []*solconfig.Node{
		{URL: config.MustParseURL("http://a:8899")},
		{URL: config.MustParseURL("http://a:8899")},
	}
	require.ErrorContains(t, solanarpc.ValidateAndNormalizeNodes(sameURL), "duplicate url")

	distinct := []*solconfig.Node{
		{URL: config.MustParseURL("http://a:8899")},
		{URL: config.MustParseURL("http://b:8899")},
	}
	require.NoError(t, solanarpc.ValidateAndNormalizeNodes(distinct))
}
