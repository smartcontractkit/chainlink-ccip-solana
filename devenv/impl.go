package devenv

import (
	"context"
	"fmt"
	"os"

	"github.com/rs/zerolog"

	"github.com/smartcontractkit/chainlink-ccv/build/devenv/cciptestinterfaces"
	"github.com/smartcontractkit/chainlink-ccv/build/devenv/chainreg"
	"github.com/smartcontractkit/chainlink-ccv/build/devenv/services"
	"github.com/smartcontractkit/chainlink-deployments-framework/deployment"

	"github.com/smartcontractkit/chainlink-ccip-solana/pkg/transmitter"
)

var (
	_ chainreg.ImplFactory  = &ImplFactory{}
	_ chainreg.ExecutorInfo = &ImplFactory{}
)

type ImplFactory struct{}

func NewImplFactory() *ImplFactory {
	return &ImplFactory{}
}

func (i *ImplFactory) New(ctx context.Context, lggr zerolog.Logger, env *deployment.Environment, chainSelector uint64) (cciptestinterfaces.CCIP17, error) {
	solChain, ok := env.BlockChains.SolanaChains()[chainSelector]
	if !ok {
		return nil, fmt.Errorf("no Solana chain for selector %d", chainSelector)
	}
	if solChain.DeployerKey == nil {
		return nil, fmt.Errorf("no deployer key for Solana chain selector %d", chainSelector)
	}
	return New(ctx, lggr, env, chainSelector, solChain)
}

func (i *ImplFactory) DefaultSignerKey(keys services.BootstrapKeys) string {
	return keys.ECDSAAddress
}

func (i *ImplFactory) DefaultFeeAggregator(env *deployment.Environment, chainSelector uint64) string {
	solChains := env.BlockChains.SolanaChains()
	if chain, ok := solChains[chainSelector]; ok && chain.DeployerKey != nil {
		return chain.DeployerKey.PublicKey().String()
	}
	return ""
}

func (i *ImplFactory) SupportsFunding() bool {
	return true
}

func (i *ImplFactory) ExecutorTransmitterKeyName() string {
	return transmitter.DefaultKeyName
}

func (i *ImplFactory) ExecutorTransmitterAddress(keys services.BootstrapKeys) string {
	// The Solana transmitter address is the raw Ed25519 public key, which the
	// bootstrap server already exposes as hex, no further decoding is needed.
	return keys.PublicKeyHex(transmitter.DefaultKeyName)
}

func (i *ImplFactory) NewEmpty() cciptestinterfaces.CCIP17Configuration {
	return NewEmptyChain(
		zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).
			Level(zerolog.DebugLevel).
			With().
			Timestamp().
			Str("component", "Solana").
			Logger(),
	)
}
