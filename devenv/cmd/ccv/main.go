package main

import (
	chainselectors "github.com/smartcontractkit/chain-selectors"
	"github.com/smartcontractkit/chainlink-ccv/build/devenv/chainreg"
	"github.com/smartcontractkit/chainlink-ccv/build/devenv/cli"
	"github.com/smartcontractkit/chainlink-ccv/build/devenv/localnetwork"

	_ "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/adapters" // triggers init() registrations
	soldevenv "github.com/smartcontractkit/chainlink-ccip-solana/devenv"
	"github.com/smartcontractkit/chainlink-ccip-solana/devenv/ccvregistry"
)

func init() {
	factory := soldevenv.NewImplFactory()
	if err := chainreg.Register(chainselectors.FamilySolana, chainreg.Registration{
		ImplFactory:              factory,
		ExecutorInfo:             factory,
		CLDFProvider:             ccvregistry.SolanaCLDFProviderFactory,
		ChainConfigLoader:        ccvregistry.SolanaChainConfigLoader,
		LocalNetworkConfigurator: localnetwork.Configurator(chainselectors.FamilySolana),
		VerifierModifier:         ccvregistry.SolanaVerifierModifier,
		ExecutorModifier:         ccvregistry.SolanaExecutorModifier,
		TokenVerifierModifier:    ccvregistry.SolanaTokenVerifierModifier,
		AddressResolver:          soldevenv.NewAddressResolver(),
	}); err != nil {
		panic("solana chainreg: " + err.Error())
	}

	// Off-chain service config adapters, core registrations, and token adapters
	// are handled by the adapters package init() (chainlink-ccip-solana/deployment/v2_0_0/adapters).
}

func main() {
	cli.RunCLI()
}
