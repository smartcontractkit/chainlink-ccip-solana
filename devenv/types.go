package devenv

import (
	"github.com/gagliardetto/solana-go"

	"github.com/smartcontractkit/chainlink-ccv/build/devenv/cciptestinterfaces"
)

var _ cciptestinterfaces.ChainSendOption = SolanaSendOptions{}

// SolanaSendOptions is the Solana ChainSendOption.
type SolanaSendOptions struct {
	Signer *solana.PrivateKey
}

func (SolanaSendOptions) IsSendOption() {}
