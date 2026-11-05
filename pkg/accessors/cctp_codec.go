package accessors

import (
	"fmt"

	"github.com/mr-tron/base58"

	chainsel "github.com/smartcontractkit/chain-selectors"
	"github.com/smartcontractkit/chainlink-ccv/pkg/chainaccess"
	"github.com/smartcontractkit/chainlink-ccv/protocol"
)

// The Solana accessor is the CCTP chain codec for every Solana source chain.
var _ chainaccess.CCTPCodec = (*accessor)(nil)

// CCTPCodec returns the accessor itself.
func (a *accessor) CCTPCodec() (chainaccess.CCTPCodec, error) {
	return a, nil
}

// solanaDomains maps Solana selectors to their Circle domain. Circle assigns domain 5 to
// Solana on every network. The devenv chains use synthetic selectors; the second devenv
// Solana chain uses 106 so the devenv setup code can tell the chains apart.
var solanaDomains = map[protocol.ChainSelector]uint32{
	protocol.ChainSelector(chainsel.SOLANA_MAINNET.Selector):                                    5,
	protocol.ChainSelector(chainsel.SOLANA_DEVNET.Selector):                                     5,
	protocol.ChainSelector(chainsel.SOLANA_TESTNET.Selector):                                    5,
	protocol.ChainSelector(chainsel.TEST_22222222222222222222222222222222222222222222.Selector): 5,
	protocol.ChainSelector(chainsel.TEST_33333333333333333333333333333333333333333333.Selector): 106,
}

func (a *accessor) Domain(selector protocol.ChainSelector) (uint32, bool) {
	domain, ok := solanaDomains[selector]
	return domain, ok
}

// EncodeTxHash renders a Solana signature in base58. Circle's API keys Solana
// transactions by their base58 signature; a hex signature returns 404, which the client
// reports as "token data not ready".
func (a *accessor) EncodeTxHash(txHash protocol.ByteSlice) string {
	return base58.Encode(txHash)
}

// DecodeAddress parses a base58 Solana address, which is how Circle returns the
// messageSender of a Solana burn.
func (a *accessor) DecodeAddress(address string) (protocol.UnknownAddress, error) {
	decoded, err := base58.Decode(address)
	if err != nil {
		return nil, fmt.Errorf("invalid base58 address %q: %w", address, err)
	}
	if len(decoded) != 32 {
		return nil, fmt.Errorf("invalid Solana address %q: decoded to %d bytes, want 32", address, len(decoded))
	}
	return protocol.UnknownAddress(decoded), nil
}
