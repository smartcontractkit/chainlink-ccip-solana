package chainselector

import (
	"fmt"
	"strconv"

	"github.com/smartcontractkit/chainlink-ccv/protocol"
)

// Parse parses a canonical decimal chain selector string.
func Parse(s string) (protocol.ChainSelector, error) {
	u, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid chain selector %q: %w", s, err)
	}
	if Format(protocol.ChainSelector(u)) != s {
		return 0, fmt.Errorf("invalid chain selector %q: non-canonical decimal string", s)
	}
	return protocol.ChainSelector(u), nil
}

// Format returns the decimal string form of a chain selector.
func Format(sel protocol.ChainSelector) string {
	return strconv.FormatUint(uint64(sel), 10)
}
