// Package accessors provides chainaccess.AccessorFactory for Solana chains.
// The factory creates source readers wired to logpoller-based event fetchers,
// enabling the CCV pipeline to read CCIP events from Solana.
package accessors

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	chainsel "github.com/smartcontractkit/chain-selectors"
	"github.com/smartcontractkit/chainlink-ccv/pkg/chainaccess"
	"github.com/smartcontractkit/chainlink-ccv/protocol"
	commonconfig "github.com/smartcontractkit/chainlink-common/pkg/config"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/services"
	solconfig "github.com/smartcontractkit/chainlink-solana/pkg/solana/config"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/ccip_offramp"
	"github.com/smartcontractkit/chainlink-ccip-solana/pkg/chainselector"
	ccip "github.com/smartcontractkit/chainlink-ccip-solana/pkg/config"
	"github.com/smartcontractkit/chainlink-ccip-solana/pkg/destinationreader"
	"github.com/smartcontractkit/chainlink-ccip-solana/pkg/logpoller"
	"github.com/smartcontractkit/chainlink-ccip-solana/pkg/solanarpc"
	"github.com/smartcontractkit/chainlink-ccip-solana/pkg/solutil"
	"github.com/smartcontractkit/chainlink-ccip-solana/pkg/sourcereader"
	"github.com/smartcontractkit/chainlink-ccip-solana/pkg/transmitter"
)

func init() {
	chainaccess.Register(chainsel.FamilySolana, CreateAccessorFactory)
}

var (
	_ chainaccess.AccessorFactoryConstructor = CreateAccessorFactory
	_ chainaccess.AccessorFactory            = (*factory)(nil)
)

// remoteJDConfigs holds CCV addresses and destination wiring from the JD job spec.
type remoteJDConfigs struct {
	OnRampAddresses    map[string]string
	RMNRemoteAddresses map[string]string
	DestinationChain   map[string]chainaccess.DestinationChainConfig
	MaxRetryDuration   time.Duration
}

// localConfig holds NOP-owned RPC and tuning for one chain.
type localConfig struct {
	chainID           string
	nodes             []*solconfig.Node
	rpcTimeout        time.Duration
	logPoller         logpoller.Config
	destinationReader destinationreader.Config
	transmitter       transmitter.Config
}

// factory implements chainaccess.AccessorFactory for Solana.
type factory struct {
	lggr          logger.Logger
	remoteConfigs remoteJDConfigs
	localConfigs  map[protocol.ChainSelector]localConfig
}

// CreateAccessorFactory creates a Solana AccessorFactory from the given logger, local config and remote config.
func CreateAccessorFactory(
	lggr logger.Logger,
	remoteConfig chainaccess.GenericConfig,
) (chainaccess.AccessorFactory, error) {
	localSettings, err := loadLocalChainConfigs(lggr)
	if err != nil {
		return nil, err
	}
	remoteSettings := remoteJDConfigs{
		OnRampAddresses:    remoteConfig.OnRampAddresses,
		RMNRemoteAddresses: remoteConfig.RMNRemoteAddresses,
		DestinationChain:   remoteConfig.ChainConfiguration,
		MaxRetryDuration:   remoteConfig.MaxRetryDuration,
	}
	if err = validateJDChainsHaveLocalConfig(remoteSettings, localSettings); err != nil {
		return nil, err
	}
	return NewFactory(lggr, remoteSettings, localSettings), nil
}

// loadLocalChainConfigs loads Solana chain configs from the local Solana config file.
func loadLocalChainConfigs(lggr logger.Logger) (map[protocol.ChainSelector]localConfig, error) {
	nopCfg, err := ccip.LoadConfig(lggr)
	if err != nil {
		return nil, err
	}

	configs := make(map[protocol.ChainSelector]localConfig, len(nopCfg.Chains))
	for strSelector, chain := range nopCfg.Chains {
		selector, err := chainselector.Parse(strSelector)
		if err != nil {
			return nil, err
		}

		family, err := chainsel.GetSelectorFamily(uint64(selector))
		if err != nil {
			return nil, fmt.Errorf("get selector family for %d: %w", selector, err)
		}
		if family != chainsel.FamilySolana {
			continue
		}

		// Validate here for fail-fast factory construction. solanarpc validates
		// again when constructing a client.
		if err = solanarpc.ValidateAndNormalizeNodes(chain.Nodes); err != nil {
			return nil, fmt.Errorf("chain %d: %w", selector, err)
		}
		if len(chain.Nodes) == 0 {
			return nil, fmt.Errorf("chain %d: no rpc nodes configured", selector)
		}

		chainID := chain.ChainID
		if chainID == "" {
			chainID, err = chainsel.SolanaChainIdFromSelector(uint64(selector))
			if err != nil {
				return nil, fmt.Errorf("chain %d: %w", selector, err)
			}
		}

		lpConfig := chain.Reader.WithDefaults()
		if err := lpConfig.Validate(); err != nil {
			return nil, fmt.Errorf("chain %d logpoller config: %w", selector, err)
		}

		drConfig := chain.DestinationReader.WithDefaults()
		if err := drConfig.Validate(); err != nil {
			return nil, fmt.Errorf("chain %d destination reader config: %w", selector, err)
		}

		rpcTimeout := chain.RPCRequestTimeout.Duration()
		if rpcTimeout == 0 {
			rpcTimeout = ccip.DefaultRPCRequestTimeout
		}

		configs[selector] = localConfig{
			chainID:           chainID,
			nodes:             chain.Nodes,
			rpcTimeout:        rpcTimeout,
			logPoller:         lpConfig,
			destinationReader: drConfig,
			transmitter:       chain.Transmitter.WithDefaults(),
		}
	}
	if len(configs) == 0 {
		return nil, errors.New("solana local config has no chains entries")
	}
	return configs, nil
}

// validateJDChainsHaveLocalConfig ensures every Solana chain wired by the JD job spec
// (source via onRamp+RMN, or destination via chain_configuration) has a corresponding
// local config entry with a usable RPC URL. Fails fast at construction with all missing
// selectors aggregated, instead of a cryptic per-chain error later in GetAccessor.
func validateJDChainsHaveLocalConfig(
	remote remoteJDConfigs,
	local map[protocol.ChainSelector]localConfig,
) error {
	needsLocal := make(map[string]struct{})
	for strSelector := range remote.OnRampAddresses {
		if remote.OnRampAddresses[strSelector] != "" && remote.RMNRemoteAddresses[strSelector] != "" {
			needsLocal[strSelector] = struct{}{}
		}
	}
	for strSelector := range remote.DestinationChain {
		needsLocal[strSelector] = struct{}{}
	}

	var missing []string
	for strSelector := range needsLocal {
		sel, err := chainselector.Parse(strSelector)
		if err != nil {
			return fmt.Errorf("job spec chain selector %q: %w", strSelector, err)
		}
		family, err := chainsel.GetSelectorFamily(uint64(sel))
		if err != nil {
			return fmt.Errorf("get selector family for %d: %w", sel, err)
		}
		if family != chainsel.FamilySolana {
			continue
		}
		cfg, ok := local[sel]
		if !ok || len(cfg.nodes) == 0 {
			missing = append(missing, strSelector)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("solana NOP config missing RPC for JD-wired chains: %v", missing)
	}
	return nil
}

// defaultMaxRetryDuration mirrors executor.maxRetryDurationDefault. The attempt cache
// must cover the executor's retry window also when the job spec omits the duration.
const defaultMaxRetryDuration = 8 * time.Hour

// applyRetryFloor raises the attempt cache TTL to the JD MaxRetryDuration when
// that is longer. The cache must cover the executor's full retry window.
func applyRetryFloor(drConfig destinationreader.Config, retry time.Duration) destinationreader.Config {
	if retry > drConfig.AttemptCacheTTL.Duration() {
		drConfig.AttemptCacheTTL = *commonconfig.MustNewDuration(retry)
	}
	return drConfig
}

// NewFactory creates a Solana AccessorFactory.
func NewFactory(
	lggr logger.Logger,
	remoteSettings remoteJDConfigs,
	localSettings map[protocol.ChainSelector]localConfig,
) chainaccess.AccessorFactory {
	if remoteSettings.MaxRetryDuration == 0 {
		remoteSettings.MaxRetryDuration = defaultMaxRetryDuration
	}
	return &factory{
		lggr:          lggr,
		remoteConfigs: remoteSettings,
		localConfigs:  localSettings,
	}
}

// GetAccessor creates a chainaccess.Accessor for the given Solana chain.
func (f *factory) GetAccessor(ctx context.Context, chainSelector protocol.ChainSelector) (_ chainaccess.Accessor, err error) {
	family, err := chainsel.GetSelectorFamily(uint64(chainSelector))
	if err != nil {
		return nil, fmt.Errorf("failed to get selector family for %d - update chain-selectors library?: %w", chainSelector, err)
	}
	if family != chainsel.FamilySolana {
		return nil, fmt.Errorf("skipping chain, only solana is supported for chain %d, family %s", chainSelector, family)
	}

	strSelector := chainselector.Format(chainSelector)
	settings, ok := f.localConfigs[chainSelector]
	if !ok || len(settings.nodes) == 0 {
		return nil, fmt.Errorf("no RPC nodes available for chain %d", chainSelector)
	}

	// Each accessor owns a newly constructed RPC client.
	client, err := solanarpc.NewClient(ctx, solanarpc.Config{
		ChainID:    settings.chainID,
		Nodes:      settings.nodes,
		RPCTimeout: settings.rpcTimeout,
	}, f.lggr)
	if err != nil {
		return nil, fmt.Errorf("create RPC transport for chain %d: %w", chainSelector, err)
	}

	// Attach the chain selector once here. Each component adds its own name with Named.
	lggr := logger.With(f.lggr, "chainSelector", chainSelector)

	a := &accessor{
		lggr:          lggr,
		chainSelector: chainSelector,
		chainID:       settings.chainID,
		rpc:           client,
	}
	success := false
	defer func() {
		if success {
			return
		}
		if closeErr := a.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close accessor after construction failure: %w", closeErr))
		}
	}()

	if f.hasSourceConfig(strSelector) {
		sourceReader, lp, err := f.newSourceReader(lggr, chainSelector, strSelector, settings, client)
		if err != nil {
			return nil, err
		}

		a.sourceLogPoller = lp
		a.sourceReader = sourceReader
		if startErr := lp.Start(ctx); startErr != nil {
			return nil, fmt.Errorf("start logpoller: %w", startErr)
		}
	}

	if destCfg, ok := f.remoteConfigs.DestinationChain[strSelector]; ok {
		destReader, err := f.newDestinationComponents(lggr, chainSelector, settings, client, destCfg, a)
		if err != nil {
			return nil, err
		}
		a.destinationReader = destReader
	}

	success = true
	return a, nil
}

func (f *factory) hasSourceConfig(strSelector string) bool {
	return f.remoteConfigs.OnRampAddresses[strSelector] != "" && f.remoteConfigs.RMNRemoteAddresses[strSelector] != ""
}

func (f *factory) newSourceReader(
	lggr logger.Logger,
	chainSelector protocol.ChainSelector,
	strSelector string,
	settings localConfig,
	rpcClient *solanarpc.Client,
) (chainaccess.SourceReader, logpoller.Service, error) {
	onRampAddress, err := solutil.PublicKeyFromHex(f.remoteConfigs.OnRampAddresses[strSelector])
	if err != nil {
		return nil, nil, fmt.Errorf("invalid onRamp address for chain %s: %w", strSelector, err)
	}

	rmnAddress, err := solutil.PublicKeyFromHex(f.remoteConfigs.RMNRemoteAddresses[strSelector])
	if err != nil {
		return nil, nil, fmt.Errorf("invalid rmnRemote address for chain %s: %w", strSelector, err)
	}

	onrampLPConfig := settings.logPoller
	onrampLPConfig.IncludeReverted = false // reverted ccipSend txs must not reach the source reader
	lp, err := logpoller.NewService(
		rpcClient,
		onRampAddress,
		settings.chainID,
		lggr,
		onrampLPConfig,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("create logpoller service: %w", err)
	}

	reader, rerr := sourcereader.NewSolanaSourceReader(
		onRampAddress,
		rmnAddress,
		chainSelector,
		rpcClient,
		lp,
		lggr,
		sourcereader.DefaultMessageSentEventParser,
	)
	if rerr != nil {
		return nil, nil, fmt.Errorf("create source reader: %w", rerr)
	}

	return reader, lp, nil
}

func (f *factory) newDestinationComponents(
	lggr logger.Logger,
	chainSelector protocol.ChainSelector,
	settings localConfig,
	rpcClient *solanarpc.Client,
	destCfg chainaccess.DestinationChainConfig,
	a *accessor,
) (dr chainaccess.DestinationReader, err error) {
	if strings.TrimSpace(destCfg.OffRampAddress) == "" {
		return nil, errors.New("offRamp address missing from DestinationChain")
	}
	offRampProgramID, err := solutil.PublicKeyFromHex(destCfg.OffRampAddress)
	if err != nil {
		return nil, fmt.Errorf("invalid offRamp address for chain %d: %w", chainSelector, err)
	}

	rmnRemoteAddr, err := solutil.PublicKeyFromHex(destCfg.RmnAddress)
	if err != nil {
		return nil, fmt.Errorf("invalid rmnRemote address for destination chain %d: %w", chainSelector, err)
	}

	// Reconfigure gobindings to use the correct offramp program ID
	solutil.SetProgramID(&ccip_offramp.ProgramID, ccip_offramp.ParseInstructionWithoutAccounts, offRampProgramID)

	offrampLPConfig := settings.logPoller
	offrampLPConfig.IncludeReverted = true // execution-attempt poller needs failed txs for HasHonestAttempt
	lp, err := logpoller.NewService(
		rpcClient,
		offRampProgramID,
		settings.chainID,
		lggr,
		offrampLPConfig,
	)
	if err != nil {
		return nil, fmt.Errorf("offramp logpoller for chain %d: %w", chainSelector, err)
	}

	defer func() {
		if err == nil {
			return
		}
		// clean up resources on error
		if dr != nil {
			if closer, ok := dr.(services.Service); ok {
				_ = closer.Close()
			}
			return
		}
		_ = lp.Close()
	}()

	drConfig := applyRetryFloor(settings.destinationReader, f.remoteConfigs.MaxRetryDuration)

	// note: we don't start logpoller here because dest reader owns its Start/Close lifecycle
	dr, err = destinationreader.NewDestinationReader(destinationreader.Params{
		Lggr:             lggr,
		ChainSelector:    chainSelector,
		OfframpProgramID: offRampProgramID,
		RMNRemoteAddress: rmnRemoteAddr,
		RPC:              rpcClient,
		LogPoller:        lp,
		Config:           drConfig,
	})
	if err != nil {
		return nil, fmt.Errorf("destination reader for chain %d: %w", chainSelector, err)
	}

	transmitterKeyName := destCfg.TransmitterKeyName
	if transmitterKeyName == "" {
		transmitterKeyName = transmitter.DefaultKeyName
	}

	a.rpc = rpcClient
	a.keyName = transmitterKeyName
	a.offrampProgramID = offRampProgramID
	a.txConfig = settings.transmitter
	a.destConfigured = true

	return dr, nil
}
