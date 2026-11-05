package logpoller

import (
	"github.com/gagliardetto/solana-go"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	sollogpoller "github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller"
	lptypes "github.com/smartcontractkit/chainlink-solana/pkg/solana/logpoller/types"
)

func newAnchorCPIEventExtractor(programID solana.PublicKey, lggr logger.Logger) *sollogpoller.CPIEventExtractor {
	extractor := sollogpoller.NewCPIEventExtractor(logger.Sugared(lggr))
	extractor.AddFilter(anchorSelfCPIEventFilter(lptypes.PublicKey(programID)))
	return extractor
}

func anchorSelfCPIEventFilter(programID lptypes.PublicKey) lptypes.Filter {
	return lptypes.Filter{
		Address: programID,
		ExtraFilterConfig: lptypes.ExtraFilterConfig{
			DestProgram:     programID,
			MethodSignature: lptypes.AnchorCPIEventDiscriminator(),
		},
	}
}
