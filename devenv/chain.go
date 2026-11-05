package devenv

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"sync"

	"github.com/Masterminds/semver/v3"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/system"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	chainsel "github.com/smartcontractkit/chain-selectors"
	"github.com/smartcontractkit/chainlink-ccv/build/devenv/cciptestinterfaces"
	"github.com/smartcontractkit/chainlink-ccv/protocol"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	"github.com/smartcontractkit/chainlink-deployments-framework/deployment"
	"github.com/smartcontractkit/chainlink-testing-framework/framework/components/blockchain"
	nodeset "github.com/smartcontractkit/chainlink-testing-framework/framework/components/simple_node_set"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/testutils"
	solcommon "github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	soltokens "github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/tokens"
	deploysolcommon "github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	testreceiverops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/testreceiver"
	"github.com/smartcontractkit/chainlink-ccip-solana/pkg/logpoller"
)

var errChainClosed = errors.New("chain closed")

// defaultPoolFundingSOL is the amount of SOL funded to each signer pool key
// during InitSignerPool.
const defaultPoolFundingSOL = 1

var (
	_ cciptestinterfaces.CCIP17               = &Chain{}
	_ cciptestinterfaces.CCIP17Configuration  = &Chain{}
	_ cciptestinterfaces.TokenConfigProvider  = &Chain{}
	_ cciptestinterfaces.ChainAsSource        = &Chain{}
	_ cciptestinterfaces.ChainAsDestination   = &Chain{}
	_ cciptestinterfaces.MessageV3Destination = &Chain{}
	_ SolanaOptions                           = &Chain{}
)

// Chain implements both cciptestinterfaces.CCIP17 and cciptestinterfaces.CCIP17Configuration
type Chain struct {
	e            *deployment.Environment
	logger       zerolog.Logger
	chainDetails chainsel.ChainDetails
	chain        cldf_solana.Chain
	signerPool   *SignerPool

	// Handed to the CCIP account-resolution helpers; see ccipResolutionLogger.
	ccipLggr logger.Logger

	routerPoller  logpoller.Service // lazy init for ConfirmSendOnSource
	offrampPoller logpoller.Service // lazy init for ConfirmExecOnDest
	cancelPollers context.CancelFunc
	pollerOnce    sync.Once
	pollerErr     error
}

func New(_ context.Context, logger zerolog.Logger, e *deployment.Environment, chainSelector uint64, chain cldf_solana.Chain) (*Chain, error) {
	chainDetails, err := chainsel.GetChainDetails(chainSelector)
	if err != nil {
		return nil, fmt.Errorf("get chain details for chain %d: %w", chainSelector, err)
	}

	return &Chain{
		e:            e,
		chainDetails: chainDetails,
		logger:       logger,
		chain:        chain,
		ccipLggr:     ccipResolutionLogger(),
	}, nil
}

func NewEmptyChain(logger zerolog.Logger) *Chain {
	return &Chain{
		logger:   logger,
		ccipLggr: ccipResolutionLogger(),
	}
}

// ccipResolutionLogger builds the logger passed to the account-resolution helpers.
//
// Those helpers emit the per-round simulation output at Debug, and that output is a primary devenv
// debugging aid, so this is pinned to Debug rather than inheriting the ambient level (which
// logger.New() leaves at Info, silently dropping it). It is a chainlink-common logger rather than
// the chain's zerolog one because that is the interface the helpers take; keeping a bridge between
// the two would cost ~30 delegating methods for no benefit here.
//
// Falls back to a no-op logger instead of failing: logging setup should not break message sending.
func ccipResolutionLogger() logger.Logger {
	cfg := logger.Config{Level: zapcore.DebugLevel}
	lggr, err := cfg.New()
	if err != nil {
		return logger.Nop()
	}
	return lggr
}

func (c *Chain) ChainFamily() string {
	return chainsel.FamilySolana
}

func (c *Chain) ChainSelector() uint64 {
	return c.chainDetails.ChainSelector
}

func (c *Chain) RPCClient() *rpc.Client {
	return c.chain.Client
}

// GetEOAReceiverAddress returns the v2 example receiver program. On Solana there is no EOA concept:
// an arbitrary message triggers a `ccip_receive` CPI, so the receiver must be an executable program.
//
// Deprecated: use AddressResolver.GetContractReceiver, which also selects the v1 receiver. This
// method stays as a fallback for callers that resolve the receiver for every destination, such as
// the ccv load gun and tcapi's ResolveV3SendAddresses.
func (c *Chain) GetEOAReceiverAddress() (protocol.UnknownAddress, error) {
	receiver, err := ResolveTestReceiverAddress(c.e.DataStore, c.chainDetails.ChainSelector, testreceiverops.Version)
	if err != nil {
		return nil, fmt.Errorf("resolve v2 test receiver: %w", err)
	}
	return receiver.Bytes(), nil
}

// ResolveTestReceiverAddress returns the example receiver program deployed on chainSelector at the
// given version.
//
// It matches on chain, type and version but not qualifier: devenv registers both the v1 and v2
// receivers under the default qualifier, like staging, and DeployExampleReceiver also uses a
// non-empty one. The version already distinguishes the v1 and v2 receivers, so exactly one match is
// expected either way, and anything else is a misconfiguration worth failing on. Use this rather
// than a gobinding ProgramID, which only matches in devenv.
func ResolveTestReceiverAddress(ds datastore.DataStore, chainSelector uint64, version *semver.Version) (solana.PublicKey, error) {
	refs := ds.Addresses().Filter(
		datastore.AddressRefByChainSelector(chainSelector),
		datastore.AddressRefByType(datastore.ContractType(testreceiverops.ContractType)),
		datastore.AddressRefByVersion(version),
	)
	if len(refs) == 0 {
		return solana.PublicKey{}, fmt.Errorf("resolve TestReceiver from datastore: no %s %s registered on chain %d",
			testreceiverops.ContractType, version, chainSelector)
	}
	if len(refs) > 1 {
		qualifiers := make([]string, 0, len(refs))
		for _, ref := range refs {
			qualifiers = append(qualifiers, ref.Qualifier)
		}
		return solana.PublicKey{}, fmt.Errorf("resolve TestReceiver from datastore: %d %s %s refs on chain %d, qualifiers %v",
			len(refs), testreceiverops.ContractType, version, chainSelector, qualifiers)
	}

	receiver, err := deploysolcommon.ToAddress(refs[0])
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("resolve TestReceiver from datastore: %w", err)
	}
	return receiver, nil
}

func (c *Chain) GetSenderAddress() (protocol.UnknownAddress, error) {
	return c.chain.DeployerKey.PublicKey().Bytes(), nil
}

// ---------------------------------------------------------------------------
// OffChainConfigurable
// ---------------------------------------------------------------------------

func (c *Chain) DeployLocalNetwork(ctx context.Context, bcs *blockchain.Input) (*blockchain.Output, error) {
	c.logger.Info().Str("chainID", bcs.ChainID).Msg("Deploying local Solana network")
	return testutils.NewLocalValidator(ctx, bcs)
}

func (c *Chain) ConfigureNodes(_ context.Context, _ *blockchain.Input) (string, error) {
	return "", nil // TODO: implement
}

func (c *Chain) FundNodes(_ context.Context, _ []*nodeset.Input, _ *blockchain.Input, _, _ *big.Int) error {
	return nil // TODO: implement
}

// FundAddresses sends nativeAmount SOL to each address from the deployer key.
// nativeAmount is denominated in whole SOL to align with chainlink-ccv's funding path
// and converted to lamports internally.
func (c *Chain) FundAddresses(ctx context.Context, bc *blockchain.Input, addresses []protocol.UnknownAddress, nativeAmount *big.Int) error {
	if len(addresses) == 0 {
		return nil
	}

	// Two connectivity paths:
	//   - bc non-nil: CCV framework callers use a NewEmpty() chain with no RPC state,
	//     so rpcClient and deployerKey are derived from bc.
	//
	//   - bc nil: internal callers use an already initialized Chain,
	//     so we fall back to the struct's own rpcClient and deployerKey.
	rpcClient := c.chain.Client
	deployerKey := c.chain.DeployerKey
	if bc != nil && bc.Out != nil && len(bc.Out.Nodes) > 0 {
		rpcClient = rpc.New(bc.Out.Nodes[0].ExternalHTTPUrl)
		deployerKey = new(solana.MustPrivateKeyFromBase58(blockchain.DefaultSolanaPrivateKey))
	}
	if rpcClient == nil {
		return errors.New("RPC client not configured and no blockchain input provided")
	}
	if deployerKey == nil {
		return errors.New("deployer key not configured and no blockchain input provided")
	}

	// Filter addresses that aren't valid on-curve Solana public keys.
	// Rejects: wrong-length addresses (e.g. 20-byte EVM addresses from CCV framework),
	// PDAs (off-curve by design, cannot sign transactions), and random garbage.
	valid := make([]protocol.UnknownAddress, 0, len(addresses))
	for _, addr := range addresses {
		if !solana.IsOnCurve(addr) {
			c.logger.Warn().
				Int("len", len(addr)).
				Hex("addr", addr).
				Msg("FundAddresses: skipping invalid Solana address (not an on-curve Ed25519 public key)")
			continue
		}
		valid = append(valid, addr)
	}
	if len(valid) == 0 {
		c.logger.Warn().
			Int("total", len(addresses)).
			Msg("FundAddresses: all addresses were invalid, nothing to fund")
		return nil
	}

	if nativeAmount == nil || nativeAmount.Sign() <= 0 {
		return fmt.Errorf("invalid nativeAmount: must be a positive value, got %v", nativeAmount)
	}

	// nativeAmount is denominated in whole SOL, convert to lamports for the transfer.
	lamportsBig := new(big.Int).Mul(nativeAmount, new(big.Int).SetUint64(solana.LAMPORTS_PER_SOL))
	if !lamportsBig.IsUint64() {
		return fmt.Errorf("nativeAmount too large: %v SOL overflows uint64 lamports", nativeAmount)
	}
	lamports := lamportsBig.Uint64()
	from := deployerKey.PublicKey()

	// Solana tx limit: 1232 bytes. Fixed overhead ~170 bytes.
	// Each additional transfer adds ~50 bytes.
	// At N=20: 170 + 20*50 = 1170 bytes, within the limit.
	const maxIxPerTx = 20

	for start := 0; start < len(valid); start += maxIxPerTx {
		end := min(start+maxIxPerTx, len(valid))
		batch := valid[start:end]

		ixs := make([]solana.Instruction, 0, len(batch))
		for _, addr := range batch {
			to := solana.PublicKeyFromBytes(addr)
			ix, err := system.NewTransferInstruction(lamports, from, to).ValidateAndBuild()
			if err != nil {
				return fmt.Errorf("build transfer to %s: %w", to, err)
			}
			ixs = append(ixs, ix)
		}

		_, err := solcommon.SendAndConfirm(ctx, rpcClient, ixs, *deployerKey, rpc.CommitmentConfirmed)
		if err != nil {
			return fmt.Errorf("fund addresses batch [%d:%d]: %w", start, end, err)
		}
	}

	return nil
}

// InitSignerPool generates n random keypairs and funds each with SOL from the deployer key.
func (c *Chain) InitSignerPool(ctx context.Context, n int) error {
	if c.signerPool != nil {
		return fmt.Errorf("signer pool already initialized (%d keys)", len(c.signerPool.keys))
	}

	pool, err := NewSignerPool(n)
	if err != nil {
		return fmt.Errorf("create signer pool: %w", err)
	}

	pubkeys := pool.PublicKeys()
	addrs := make([]protocol.UnknownAddress, len(pubkeys))
	for i, pk := range pubkeys {
		addrs[i] = pk.Bytes()
	}

	fundAmount := big.NewInt(defaultPoolFundingSOL)
	if err := c.FundAddresses(ctx, nil, addrs, fundAmount); err != nil {
		return fmt.Errorf("fund signer pool: %w", err)
	}

	c.signerPool = pool
	c.logger.Info().
		Int("poolSize", n).
		Int("solPerKey", defaultPoolFundingSOL).
		Msg("Initialized signer pool with funded keypairs")

	return nil
}

// DrainSignerPool transfers remaining SOL from each pool key back to the deployer key.
// Should be called in t.Cleanup to avoid stranding funds on devnet/staging.
// No-op if the pool was never initialized.
func (c *Chain) DrainSignerPool(ctx context.Context) error {
	if c.signerPool == nil {
		return nil
	}
	if c.chain.Client == nil || c.chain.DeployerKey == nil {
		return errors.New("RPC client or deployer key not configured")
	}

	deployerPubkey := c.chain.DeployerKey.PublicKey()
	keys := c.signerPool.PrivateKeys()
	var totalReturned uint64

	for _, key := range keys {
		pubkey := key.PublicKey()
		balance, err := c.chain.Client.GetBalance(ctx, pubkey, rpc.CommitmentConfirmed)
		if err != nil {
			c.logger.Warn().Str("key", pubkey.String()).Err(err).Msg("DrainSignerPool: failed to get balance, skipping")
			continue
		}
		if balance.Value == 0 {
			continue
		}

		ix, err := system.NewTransferInstruction(balance.Value, pubkey, deployerPubkey).ValidateAndBuild()
		if err != nil {
			c.logger.Warn().Str("key", pubkey.String()).Err(err).Msg("DrainSignerPool: failed to build transfer, skipping")
			continue
		}

		_, err = solcommon.SendAndConfirm(ctx, c.chain.Client, []solana.Instruction{ix}, *c.chain.DeployerKey, rpc.CommitmentConfirmed, solcommon.AddSigners(key))
		if err != nil {
			c.logger.Warn().Str("key", pubkey.String()).Err(err).Msg("DrainSignerPool: failed to transfer, skipping")
			continue
		}
		totalReturned += balance.Value
	}

	c.logger.Info().
		Uint64("totalLamports", totalReturned).
		Float64("totalSOL", float64(totalReturned)/float64(solana.LAMPORTS_PER_SOL)).
		Int("poolSize", len(keys)).
		Msg("Drained signer pool funds back to deployer")

	return nil
}

// SignerPool returns the signer pool, or nil if not initialized.
func (c *Chain) SignerPool() *SignerPool {
	return c.signerPool
}

// NativeBalance returns the SOL balance (in lamports) for the given address.
func (c *Chain) NativeBalance(ctx context.Context, address protocol.UnknownAddress) (*big.Int, error) {
	if len(address) != solana.PublicKeyLength {
		return nil, fmt.Errorf("invalid address length: expected %d, got %d", solana.PublicKeyLength, len(address))
	}
	pubkey := solana.PublicKeyFromBytes(address)
	result, err := c.chain.Client.GetBalance(ctx, pubkey, rpc.CommitmentConfirmed)
	if err != nil {
		return nil, fmt.Errorf("get balance for %s: %w", pubkey, err)
	}
	return new(big.Int).SetUint64(result.Value), nil
}

// resolveFromKey maps a public address to the corresponding private key held by this Chain.
// Supports the deployer key and pool keys if initialized.
func (c *Chain) resolveFromKey(from protocol.UnknownAddress) (solana.PrivateKey, error) {
	if len(from) != solana.PublicKeyLength {
		return nil, fmt.Errorf("invalid from address length: expected %d, got %d", solana.PublicKeyLength, len(from))
	}
	fromPubkey := solana.PublicKeyFromBytes(from)
	if fromPubkey == c.chain.DeployerKey.PublicKey() {
		return *c.chain.DeployerKey, nil
	}
	if c.signerPool != nil {
		if key, ok := c.signerPool.FindKey(fromPubkey); ok {
			return key, nil
		}
	}
	return nil, fmt.Errorf("no private key available for address %s", fromPubkey)
}

// TransferNative moves SOL between accounts. The `from` address must be either
// the deployer key or a key in the signer pool.
func (c *Chain) TransferNative(ctx context.Context, from, to protocol.UnknownAddress, amount *big.Int) error {
	if len(to) != solana.PublicKeyLength {
		// Length-only check and not IsOnCurve as destinations can be PDAs (off-curve),
		// which legitimately hold SOL. Only the `from` side needs to be a signer.
		return fmt.Errorf("invalid destination address length: expected %d, got %d", solana.PublicKeyLength, len(to))
	}
	if amount == nil || amount.Sign() <= 0 || !amount.IsUint64() {
		return fmt.Errorf("invalid amount: must be a positive value that fits in uint64, got %v", amount)
	}
	signer, err := c.resolveFromKey(from)
	if err != nil {
		return fmt.Errorf("TransferNative: %w", err)
	}
	fromPubkey := signer.PublicKey()
	toPubkey := solana.PublicKeyFromBytes(to)

	ix, err := system.NewTransferInstruction(amount.Uint64(), fromPubkey, toPubkey).ValidateAndBuild()
	if err != nil {
		return fmt.Errorf("build transfer instruction: %w", err)
	}

	_, err = solcommon.SendAndConfirm(ctx, c.chain.Client, []solana.Instruction{ix}, signer, rpc.CommitmentConfirmed)
	if err != nil {
		return fmt.Errorf("send and confirm transfer: %w", err)
	}
	return nil
}

func (c *Chain) GetTokenBalance(ctx context.Context, owner, tokenAddr protocol.UnknownAddress) (*big.Int, error) {
	if len(owner) == 0 || len(tokenAddr) == 0 {
		return nil, errors.New("owner and token address are required")
	}

	ownerPubkey := solana.PublicKeyFromBytes(owner)
	mintPubkey := solana.PublicKeyFromBytes(tokenAddr)

	// Derive the Associated Token Account for the owner + mint.
	ata, _, err := soltokens.FindAssociatedTokenAddress(solana.TokenProgramID, mintPubkey, ownerPubkey)
	if err != nil {
		return nil, fmt.Errorf("derive ATA for owner %s mint %s: %w", ownerPubkey, mintPubkey, err)
	}

	_, balance, err := soltokens.TokenBalance(ctx, c.chain.Client, ata, rpc.CommitmentConfirmed)
	if err != nil {
		// ATA doesn't exist yet — balance is 0.
		if strings.Contains(err.Error(), "could not find account") {
			return big.NewInt(0), nil
		}
		return nil, fmt.Errorf("get token balance for ATA %s: %w", ata, err)
	}

	if balance < 0 {
		return nil, fmt.Errorf("unexpected negative token balance %d for ATA %s", balance, ata)
	}
	return new(big.Int).SetUint64(uint64(balance)), nil
}

// initPollers lazily initializes the shared router and offramp logpoller services.
// Pollers are created once and reused across all ConfirmSendOnSource/ConfirmExecOnDest calls
// to avoid per-call recreation overhead in load test scenarios.
// TODO: consider dropping logpoller entirely in favor of raw RPC + manual CPI parsing if proven more efficient.
func (c *Chain) initPollers() error {
	if errors.Is(c.pollerErr, errChainClosed) {
		return c.pollerErr
	}
	c.pollerOnce.Do(func() {
		// Human-readable poller logs: console layout instead of JSON, with
		// ISO8601 timestamps instead of zap's default Unix epoch.
		lggr, err := logger.NewWith(func(cfg *zap.Config) {
			cfg.Encoding = "console"
			cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
		})
		if err != nil {
			c.pollerErr = fmt.Errorf("create logger: %w", err)
			return
		}
		routerProgramID, err := routerPubkeyFromDataStore(c.e.DataStore, c.chainDetails.ChainSelector, "")
		if err != nil {
			c.pollerErr = fmt.Errorf("data store: %w", err)
			return
		}
		offrampProgramID, err := offrampPubkeyFromDataStore(c.e.DataStore, c.chainDetails.ChainSelector, "")
		if err != nil {
			c.pollerErr = fmt.Errorf("data store: %w", err)
			return
		}

		c.routerPoller, err = logpoller.NewService(c.chain.Client, routerProgramID, c.chainDetails.ChainName, lggr, logpoller.DefaultConfig())
		if err != nil {
			c.pollerErr = fmt.Errorf("create router logpoller: %w", err)
			return
		}
		c.offrampPoller, err = logpoller.NewService(c.chain.Client, offrampProgramID, c.chainDetails.ChainName, lggr, logpoller.DefaultConfig())
		if err != nil {
			_ = c.routerPoller.Close()
			c.routerPoller = nil
			c.pollerErr = fmt.Errorf("create offramp logpoller: %w", err)
			return
		}

		ctx, cancel := context.WithCancel(context.Background())
		c.cancelPollers = cancel

		// failStart tears down all pollers so we don't leave it half initialized on partial failures.
		failStart := func(msg string, startErr error) {
			cancel()
			c.cancelPollers = nil
			_ = c.routerPoller.Close()
			_ = c.offrampPoller.Close()
			c.routerPoller = nil
			c.offrampPoller = nil
			c.pollerErr = fmt.Errorf("%s: %w", msg, startErr)
		}

		// Start pollers; on failure close everything to prevent resource leaks.
		if err = c.routerPoller.Start(ctx); err != nil {
			failStart("start router logpoller", err)
			return
		}
		if err = c.offrampPoller.Start(ctx); err != nil {
			failStart("start offramp logpoller", err)
			return
		}
	})
	return c.pollerErr
}

// Close shuts down the shared logpoller services. Safe to call even if pollers were never initialized.
// Idempotent: subsequent calls are no-ops. After Close, initPollers returns an error.
func (c *Chain) Close() error {
	if c.cancelPollers != nil {
		c.cancelPollers()
		c.cancelPollers = nil
	}
	var errs []error
	if c.routerPoller != nil {
		errs = append(errs, c.routerPoller.Close())
		c.routerPoller = nil
	}
	if c.offrampPoller != nil {
		errs = append(errs, c.offrampPoller.Close())
		c.offrampPoller = nil
	}
	c.pollerErr = errChainClosed
	return errors.Join(errs...)
}

// ---------------------------------------------------------------------------
// Observable
// ---------------------------------------------------------------------------

func (c *Chain) ExposeMetrics(_ context.Context, _, _ uint64) ([]string, *prometheus.Registry, error) {
	return nil, nil, nil // TODO: implement
}
