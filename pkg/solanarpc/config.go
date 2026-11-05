package solanarpc

import (
	"fmt"
	"net/url"

	"github.com/smartcontractkit/chainlink-common/pkg/config"
	solconfig "github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
)

// ValidateAndNormalizeNodes defaults each node's name to node-<i> when absent and runs
// chainlink-solana's per-node validation (name and URL required, order in 1..100).
// It rejects duplicate names, including a default name that matches an explicit name, and duplicate URLs.
func ValidateAndNormalizeNodes(nodes []*solconfig.Node) error {
	names, urls := config.UniqueStrings{}, config.UniqueStrings{}
	for i, n := range nodes {
		if n == nil {
			return fmt.Errorf("rpc node %d: missing node config", i)
		}
		if n.Name == nil || *n.Name == "" {
			name := fmt.Sprintf("node-%d", i)
			n.Name = &name
		}
		if n.SendOnly {
			return fmt.Errorf("rpc node %d: send-only nodes are not supported", i)
		}
		if err := n.ValidateConfig(); err != nil {
			return fmt.Errorf("rpc node %d: %w", i, err)
		}
		if names.IsDupe(n.Name) {
			return fmt.Errorf("rpc node %d: duplicate name %q", i, *n.Name)
		}
		if urls.IsDupeFmt((*url.URL)(n.URL)) {
			return fmt.Errorf("rpc node %d: duplicate url %q", i, n.URL.String())
		}
	}
	return nil
}
