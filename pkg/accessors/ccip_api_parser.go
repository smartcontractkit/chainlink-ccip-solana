package accessors

import "github.com/smartcontractkit/chainlink-ccv/pkg/chainaccess"

// The Solana accessor parses CCIP API addresses with the CCTP codec's base58 decoder.
var _ chainaccess.CCIPAPIParser = (*accessor)(nil)

// CCIPAPIParser returns the accessor itself.
func (a *accessor) CCIPAPIParser() (chainaccess.CCIPAPIParser, error) {
	return a, nil
}
