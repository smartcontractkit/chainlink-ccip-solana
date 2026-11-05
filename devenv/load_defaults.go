package devenv

import (
	"github.com/smartcontractkit/chainlink-ccv/build/devenv/cciptestinterfaces"
)

// DefaultLoadV3ComputeUnits is the execution budget for V3 messages arriving on Solana
// in devenv load tests. Owned here so CCV's EVMTXGun does not hardcode Solana-specific limits.
const DefaultLoadV3ComputeUnits uint32 = 300_000

var _ cciptestinterfaces.V3DestinationLoadDefaults = (*Chain)(nil)

// V3LoadMessageOptions implements cciptestinterfaces.V3DestinationLoadDefaults.
func (c *Chain) V3LoadMessageOptions() cciptestinterfaces.MessageOptions {
	return cciptestinterfaces.MessageOptions{
		ExecutionGasLimit: DefaultLoadV3ComputeUnits,
	}
}
