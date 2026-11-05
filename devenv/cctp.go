package devenv

import (
	"context"
	"fmt"

	"github.com/Masterminds/semver/v3"
	"github.com/ethereum/go-ethereum/common"
	"github.com/gagliardetto/solana-go"

	chainsel "github.com/smartcontractkit/chain-selectors"
	routerops "github.com/smartcontractkit/chainlink-ccip/chains/evm/deployment/v1_2_0/operations/router"
	burnminterc677ops "github.com/smartcontractkit/chainlink-ccip/chains/evm/deployment/v1_5_0/operations/burn_mint_erc20_with_drip"
	tokenadminregistryops "github.com/smartcontractkit/chainlink-ccip/chains/evm/deployment/v1_5_0/operations/token_admin_registry"
	evmadapters "github.com/smartcontractkit/chainlink-ccip/chains/evm/deployment/v2_0_0/adapters"
	cctpthroughccvops "github.com/smartcontractkit/chainlink-ccip/chains/evm/deployment/v2_0_0/operations/cctp_through_ccv_token_pool"
	"github.com/smartcontractkit/chainlink-ccip/chains/evm/deployment/v2_0_0/operations/usdc_token_pool_proxy"
	evmtokensequences "github.com/smartcontractkit/chainlink-ccip/chains/evm/deployment/v2_0_0/sequences/tokens"
	cctpverifierops "github.com/smartcontractkit/chainlink-ccip/chains/evm/deployment/v2_1_0/operations/cctp_verifier"
	"github.com/smartcontractkit/chainlink-ccip/chains/evm/gobindings/generated/latest/mock_token_minter"
	"github.com/smartcontractkit/chainlink-ccip/chains/evm/gobindings/generated/latest/mock_usdc_token_messenger"
	tokenapi "github.com/smartcontractkit/chainlink-ccip/deployment/tokens"
	common_utils "github.com/smartcontractkit/chainlink-ccip/deployment/utils"
	changesetscore "github.com/smartcontractkit/chainlink-ccip/deployment/utils/changesets"
	datastoreutils "github.com/smartcontractkit/chainlink-ccip/deployment/utils/datastore"
	ccipadapters "github.com/smartcontractkit/chainlink-ccip/deployment/v2_0_0/adapters"
	ccipchangesets "github.com/smartcontractkit/chainlink-ccip/deployment/v2_0_0/changesets"
	evmcontract "github.com/smartcontractkit/chainlink-deployments-framework/chain/evm/operations/contract"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	"github.com/smartcontractkit/chainlink-deployments-framework/deployment"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations"

	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	solanaadapters "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/adapters"
	solseq "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/sequences"
)

// USDCTokenQualifier is the datastore qualifier the devenv registers its USDC mint under. It has
// to match what CCTPChainAdapter.usdcMint looks for first.
const USDCTokenQualifier = solanaadapters.USDCTokenQualifier

// USDCDecimals is USDC's decimal precision on every CCTP-supported chain, Solana included.
const USDCDecimals = 6

// usdcPreMint is how much USDC the devenv pre-mints to the deployer, in raw units (1,000,000 USDC
// at 6 decimals), matching what ccv's EVM side mints in configureCircleContracts.
const usdcPreMint uint64 = 1_000_000 * 1e6

// cctpLaneMechanism is the LockOrBurnMechanism the devenv configures on the Solana <-> EVM lane.
// This is the CCIP 2.0 path: on EVM it makes USDCTokenPoolProxy dispatch to CCTPThroughCCVTokenPool
// via IPoolV2, and on Solana the cctp_token_pool acts as its own CCV.
const cctpLaneMechanism = "CCTP_V2_WITH_CCV"

// devenv-only placeholder lane values, mirroring the constants ccv's EVM side hardcodes in
// build/devenv/evm/cctp.go's configureUSDCForTransfer.
const (
	cctpLaneFeeUSDCents        = 10
	cctpLaneGasForVerification = 100_000
	cctpLanePayloadSizeBytes   = 1000
)

// devenvCCTPDomains holds the Circle domain of every local devenv chain. Each chain family owns
// its domain table: the Solana accessor in pkg/accessors owns the Solana table, and ccv's EVM
// accessor owns the EVM catalog. The devenv restates the local values so the lane setup reads one
// table.
// Circle's Solana programs hardcode domain 5, so a second simulated Solana chain cannot
// participate in a real CCTP transfer with a distinct domain.
var devenvCCTPDomains = map[uint64]uint32{
	chainsel.TEST_22222222222222222222222222222222222222222222.Selector: 5,   // devenv Solana chain 1 (chain id 2222...)
	chainsel.TEST_33333333333333333333333333333333333333333333.Selector: 106, // devenv Solana chain 2 (chain id 3333...). Solana contracts hardcode 5, but setting a different value here allows the setup code to work (even if the contracts on this entry won't actually work correctly)
	chainsel.GETH_TESTNET.Selector:                                      100, // devenv EVM chain (GETH_TESTNET, chain id 1337)
}

// The devenv-only Circle mocks chainlink-ccv's deployCircleContracts registers on the EVM chain.
// They are stored under a plain string type, so there is no ops package constant to reference.
var (
	mockUSDCTokenMessengerType    = datastore.ContractType("MockE2EUSDCTokenMessenger")
	mockUSDCTokenMessengerVersion = semver.MustParse("1.0.0")
)

// CCTPDomainFor resolves a chain's Circle domain from the devenv-local table.
func CCTPDomainFor(selector uint64) (uint32, error) {
	if domain, ok := devenvCCTPDomains[selector]; ok {
		return domain, nil
	}
	return 0, fmt.Errorf("no CCTP domain mapping found for chain selector %d", selector)
}

// deployCCTPTokenAndPool is the Solana counterpart to EVM's deployUSDCTokenAndPool
// (chainlink-ccv/build/devenv/evm/cctp.go): create a devenv-only USDC mint, then deploy and
// initialize the CCTP token pool + its embedded verifier via the upstream CCTP changeset.
//
// Circle's own programs (TokenMessengerMinter / MessageTransmitter, v1 and v2) are deliberately
// NOT deployed here. Their addresses are compile-time constants in the pool's Rust and every PDA
// this path touches is derivable without them, so deployment and configuration succeed; only an
// actual transfer would fail at the Circle CPI.
func (c *Chain) deployCCTPTokenAndPool(ctx context.Context, env *deployment.Environment, ds *datastore.MemoryDataStore, selector uint64) error {
	solChain, ok := env.BlockChains.SolanaChains()[selector]
	if !ok {
		return fmt.Errorf("solana chain not found for selector %d", selector)
	}

	bundle := operations.NewBundle(func() context.Context { return ctx }, env.Logger, operations.NewMemoryReporter())

	// The devenv USDC mint, pre-minted to the deployer so a send has a funded source account.
	// It must be an SPL Token (not Token-2022) mint: the CCTP verifier's account resolvers
	// hardcode the SPL Token program when deriving the pool and receiver ATAs, and
	// DeployCCTPChain rejects anything else.
	preMint := usdcPreMint
	deployTokenOut, err := operations.ExecuteSequence(bundle, (&solseq.SolanaAdapter{}).DeployToken(), env.BlockChains, tokenapi.DeployTokenInput{
		Name:              "USD Coin",
		Symbol:            USDCTokenQualifier,
		Decimals:          USDCDecimals,
		PreMint:           &preMint,
		Senders:           []string{solChain.DeployerKey.PublicKey().String()},
		Type:              solcommon.SPLTokens,
		ChainSelector:     selector,
		ExistingDataStore: ds.Seal(),
		// The devenv has no MCMS timelock, so the token deploy cannot resolve the
		// CLL timelock signer PDA as freeze authority; opt out explicitly, like the
		// devenv's generic token and Lombard LBTC deploys do.
		DisableFreezeAuthority: true,
	})
	if err != nil {
		return fmt.Errorf("failed to deploy USDC token on chain %d: %w", selector, err)
	}
	for _, addr := range deployTokenOut.Output.Addresses {
		if err = ds.Addresses().Add(addr); err != nil {
			return fmt.Errorf("failed to store USDC token address on chain %d: %w", selector, err)
		}
	}
	tokenRef, err := solcommon.PoolRefByQualifier(ds.Seal(), selector, string(solcommon.SPLTokens), USDCTokenQualifier)
	if err != nil {
		return fmt.Errorf("failed to resolve USDC token on chain %d: %w", selector, err)
	}

	cctpChainRegistry := ccipadapters.NewCCTPChainRegistry()
	cctpChainRegistry.RegisterCCTPChain(chainsel.FamilySolana, &solanaadapters.CCTPChainAdapter{})

	// A single-chain map with no RemoteChains: the changeset's reciprocity check and its
	// "solana lanes only support CCTP_V1" check both live in the remote loop, so neither is
	// reached here. Lanes are configured later, in configureCCTPLaneForRemote.
	out, err := ccipchangesets.DeployCCTPChains(cctpChainRegistry, changesetscore.GetRegistry()).Apply(*env, ccipchangesets.DeployCCTPChainsConfig{
		Chains: map[uint64]ccipchangesets.CCTPChainConfig{
			selector: {
				USDCType:         ccipadapters.Canonical,
				TokenDecimals:    USDCDecimals,
				USDCToken:        tokenRef.Address,
				StorageLocations: []string{"https://test.chain.link.fake"},
				FeeAggregator:    solChain.DeployerKey.PublicKey().String(),
				FastFinalityBps:  100,
			},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to deploy cctp chain on chain %d: %w", selector, err)
	}
	if err = ds.Merge(out.DataStore.Seal()); err != nil {
		return fmt.Errorf("failed to merge cctp deploy output on chain %d: %w", selector, err)
	}

	return nil
}

// configureCCTPLaneForRemote wires the EVM <-> Solana CCTP lane in both directions, once both
// sides' contracts are deployed (called from PostConnect, after every chain's deploy phase).
//
// Like the Lombard equivalent this cannot go through ccipchangesets.DeployCCTPChains: that
// changeset requires every chain referenced by a RemoteChains entry to appear as its own key in
// the same config map, each pointing back at the other, and it rejects any mechanism other than
// CCTP_V1 the moment a Solana selector is involved. This devenv's Solana deploy and chainlink-ccv's
// own EVM deploy each call it independently with single-chain maps, so neither can satisfy that.
// Instead both adapters' ConfigureCCTPChainForLanes sequences are invoked directly.
//
// TODO(upstream): chainlink-ccv's configureUSDCForTransfer (build/devenv/evm/cctp.go) drops every
// non-EVM remote via filterOnlySupportedSelectors, and chainlink-ccip's EVM sequence
// (chains/evm/deployment/v2_0_0/sequences/cctp/configure_cctp_chain_for_lanes.go) carries three
// isEVMRemote guards that skip a Solana remote in the CCV-pool preconfiguration, in
// buildVerifierResolverOutboundArgs and in buildCCTPVerifierArgs. Until those are lifted, the EVM
// CCTPVerifier and its resolver have no entry for the Solana lane, so an EVM->Solana send reverts
// with ChainNotSupportedByVerifier inside USDCTokenPoolProxy's IPoolV2 path. The same filtering
// skips the CCTP-through-CCV pool's remote-chain config and the CCTPVerifier's own remote-chain
// and domain entries, both of which are added explicitly below for the Solana->EVM direction.
func configureCCTPLaneForRemote(env *deployment.Environment, selector uint64, remoteSelectors []uint64) error {
	var evmSelector uint64
	for _, rs := range remoteSelectors {
		family, err := chainsel.GetSelectorFamily(rs)
		if err != nil {
			return fmt.Errorf("get selector family for remote %d: %w", rs, err)
		}
		if family == chainsel.FamilyEVM {
			evmSelector = rs
			break
		}
	}
	if evmSelector == 0 {
		// No EVM remote to wire a CCTP lane to (e.g. a Solana-only devenv).
		return nil
	}

	solDomain, err := CCTPDomainFor(selector)
	if err != nil {
		return err
	}
	evmDomain, err := CCTPDomainFor(evmSelector)
	if err != nil {
		return err
	}

	solAdapter := &solanaadapters.CCTPChainAdapter{}
	evmAdapter := &evmadapters.CCTPChainAdapter{}

	solTokenRef, err := solcommon.PoolRefByQualifier(env.DataStore, selector, string(solcommon.SPLTokens), USDCTokenQualifier)
	if err != nil {
		return fmt.Errorf("resolve USDC token on chain %d: %w", selector, err)
	}
	// The EVM USDC that chainlink-ccv's deployCircleContracts registered for its CCTP stack.
	evmTokenRef, err := env.DataStore.Addresses().Get(datastore.NewAddressRefKey(
		evmSelector, datastore.ContractType(burnminterc677ops.ContractType), burnminterc677ops.Version, "",
	))
	if err != nil {
		return fmt.Errorf("resolve USDC token on remote chain %d: %w", evmSelector, err)
	}

	solPoolRef, err := env.DataStore.Addresses().Get(datastore.NewAddressRefKey(
		selector, datastore.ContractType(common_utils.CCTPTokenPool), solcommon.Version2_0_0, solcommon.PoolProgramQualifier,
	))
	if err != nil {
		return fmt.Errorf("resolve cctp token pool on chain %d: %w", selector, err)
	}
	// EVM registers USDCTokenPoolProxy with the TokenAdminRegistry, not the CCTP pool itself.
	evmPoolRef, err := env.DataStore.Addresses().Get(datastore.NewAddressRefKey(
		evmSelector, datastore.ContractType(usdc_token_pool_proxy.ContractType), usdc_token_pool_proxy.Version, "",
	))
	if err != nil {
		return fmt.Errorf("resolve USDC token pool proxy on chain %d: %w", evmSelector, err)
	}

	remoteChainCfg := func(domain uint32) ccipadapters.RemoteCCTPChainConfig {
		return ccipadapters.RemoteCCTPChainConfig{
			FeeUSDCents:         cctpLaneFeeUSDCents,
			GasForVerification:  cctpLaneGasForVerification,
			PayloadSizeBytes:    cctpLanePayloadSizeBytes,
			LockOrBurnMechanism: cctpLaneMechanism,
			DomainIdentifier:    domain,
		}
	}

	if _, err = operations.ExecuteSequence(env.OperationsBundle, solAdapter.ConfigureCCTPChainForLanes(), ccipadapters.ConfigureCCTPChainForLanesDeps{
		BlockChains:              env.BlockChains,
		DataStore:                env.DataStore,
		RemoteChains:             map[uint64]ccipadapters.RemoteCCTPChain{evmSelector: evmAdapter},
		RegisteredPoolRef:        solPoolRef,
		RemoteRegisteredPoolRefs: map[uint64]datastore.AddressRef{evmSelector: evmPoolRef},
	}, ccipadapters.ConfigureCCTPChainForLanesInput{
		ChainSelector: selector,
		USDCToken:     solTokenRef.Address,
		RemoteChains:  map[uint64]ccipadapters.RemoteCCTPChainConfig{evmSelector: remoteChainCfg(evmDomain)},
	}); err != nil {
		return fmt.Errorf("configure cctp lane on solana chain %d for remote %d: %w", selector, evmSelector, err)
	}

	if _, err = operations.ExecuteSequence(env.OperationsBundle, evmAdapter.ConfigureCCTPChainForLanes(), ccipadapters.ConfigureCCTPChainForLanesDeps{
		BlockChains:              env.BlockChains,
		DataStore:                env.DataStore,
		RemoteChains:             map[uint64]ccipadapters.RemoteCCTPChain{selector: solAdapter},
		RegisteredPoolRef:        evmPoolRef,
		RemoteRegisteredPoolRefs: map[uint64]datastore.AddressRef{selector: solPoolRef},
	}, ccipadapters.ConfigureCCTPChainForLanesInput{
		ChainSelector: evmSelector,
		USDCToken:     evmTokenRef.Address,
		RemoteChains:  map[uint64]ccipadapters.RemoteCCTPChainConfig{selector: remoteChainCfg(solDomain)},
	}); err != nil {
		return fmt.Errorf("configure cctp lane on evm chain %d for remote %d: %w", evmSelector, selector, err)
	}

	// The pinned EVM CCTP sequence filters non-EVM selectors out of the CCTP-through-CCV pool
	// configuration. The proxy still routes this lane to that pool, so configure its Solana remote
	// explicitly or releaseOrMint reverts with RemoteChainNotSupported.
	cctpThroughCCVRef, err := env.DataStore.Addresses().Get(datastore.NewAddressRefKey(
		evmSelector, datastore.ContractType(cctpthroughccvops.ContractType), cctpthroughccvops.Version, "",
	))
	if err != nil {
		return fmt.Errorf("resolve CCTP-through-CCV pool on chain %d: %w", evmSelector, err)
	}
	tokenAdminRegistryRef, err := env.DataStore.Addresses().Get(datastore.NewAddressRefKey(
		evmSelector, datastore.ContractType(tokenadminregistryops.ContractType), tokenadminregistryops.Version, "",
	))
	if err != nil {
		return fmt.Errorf("resolve token admin registry on chain %d: %w", evmSelector, err)
	}
	solRemoteToken, err := solAdapter.TokenAddress(env.DataStore, env.BlockChains, selector)
	if err != nil {
		return fmt.Errorf("resolve Solana USDC token on chain %d: %w", selector, err)
	}
	solRemotePool, err := solAdapter.PoolAddress(env.DataStore, env.BlockChains, selector, solPoolRef)
	if err != nil {
		return fmt.Errorf("resolve Solana CCTP pool on chain %d: %w", selector, err)
	}
	if _, err = operations.ExecuteSequence(env.OperationsBundle, evmtokensequences.ConfigureTokenForTransfers, env.BlockChains, tokenapi.ConfigureTokenForTransfersInput{
		ChainSelector:            evmSelector,
		TokenAddress:             evmTokenRef.Address,
		TokenPoolAddress:         cctpThroughCCVRef.Address,
		RegistryTokenPoolAddress: evmPoolRef.Address,
		RegistryAddress:          tokenAdminRegistryRef.Address,
		RemoteChains: map[uint64]tokenapi.RemoteChainConfig[[]byte, string]{
			selector: {
				RemoteToken:    solRemoteToken,
				RemotePool:     solRemotePool,
				RemoteDecimals: USDCDecimals,
			},
		},
		SkipActivePoolSupportedChainsCheck: true,
		ExistingDataStore:                  env.DataStore,
	}); err != nil {
		return fmt.Errorf("configure CCTP-through-CCV pool on chain %d for Solana remote %d: %w", evmSelector, selector, err)
	}

	// The same filtering leaves the EVM CCTPVerifier itself unconfigured for the Solana lane, so
	// an inbound message reverts in BaseVerifier._onlyOffRamp before any CCTP work happens.
	// Configure its remote chain and domain entries explicitly.
	if err = configureEVMCCTPVerifierForRemote(env, evmSelector, selector, solAdapter, remoteChainCfg(solDomain)); err != nil {
		return err
	}

	if err = registerSolanaBurnTokenOnEVMMockMinter(env, evmSelector, solDomain, solRemoteToken, evmTokenRef.Address); err != nil {
		return err
	}

	evmMessenger, err := MockUSDCTokenMessengerAddress(env.DataStore, evmSelector)
	if err != nil {
		return err
	}
	usdcMint, err := solana.PublicKeyFromBase58(solTokenRef.Address)
	if err != nil {
		return fmt.Errorf("parse USDC mint on chain %d: %w", selector, err)
	}
	return setupSolanaCircleCCTPV2(env, selector, solDomain, evmDomain, usdcMint, evmMessenger, common.HexToAddress(evmTokenRef.Address))
}

// MockUSDCTokenMessengerAddress returns the EVM devenv's MockE2EUSDCTokenMessenger: the CCTP
// token messenger that Solana's Circle programs trust for the EVM domain.
func MockUSDCTokenMessengerAddress(ds datastore.DataStore, evmSelector uint64) (common.Address, error) {
	ref, err := ds.Addresses().Get(datastore.NewAddressRefKey(
		evmSelector, mockUSDCTokenMessengerType, mockUSDCTokenMessengerVersion, "",
	))
	if err != nil {
		return common.Address{}, fmt.Errorf("resolve mock USDC token messenger on chain %d: %w", evmSelector, err)
	}
	return common.HexToAddress(ref.Address), nil
}

// registerSolanaBurnTokenOnEVMMockMinter teaches the EVM devenv's MockTokenMinter that the Solana
// (domain, USDC mint) pair maps to the EVM USDC. CCTPVerifier.verifyMessage resolves the attested
// burn token through i_tokenMessenger.localMinter().getLocalToken(...) and reverts with
// InvalidBurnToken unless the result is the local USDC.
//
// chainlink-ccv's registerCCTPLocalTokens (build/devenv/evm/cctp.go) writes these entries only for
// EVM remotes: it looks the remote token up as a BurnMintERC20WithDrip address ref and runs it
// through HexToAddress, neither of which fits a base58 SPL mint. So the Solana entry is written
// here instead.
//
// Devenv-only: MockE2EUSDCTokenMessenger and MockTokenMinter stand in for Circle's contracts and
// have no counterpart on a real chain.
func registerSolanaBurnTokenOnEVMMockMinter(
	env *deployment.Environment,
	evmSelector uint64,
	remoteDomain uint32,
	remoteBurnToken []byte,
	localUSDC string,
) error {
	evmChain, ok := env.BlockChains.EVMChains()[evmSelector]
	if !ok {
		return fmt.Errorf("evm chain not found for selector %d", evmSelector)
	}

	messengerAddr, err := MockUSDCTokenMessengerAddress(env.DataStore, evmSelector)
	if err != nil {
		return err
	}
	messenger, err := mock_usdc_token_messenger.NewMockE2EUSDCTokenMessenger(messengerAddr, evmChain.Client)
	if err != nil {
		return fmt.Errorf("bind mock USDC token messenger on chain %d: %w", evmSelector, err)
	}
	// The messenger creates its MockTokenMinter in its constructor and only exposes it, so the
	// address has to be read back rather than configured.
	minterAddr, err := messenger.ITokenMinter(nil)
	if err != nil {
		return fmt.Errorf("read local minter on chain %d: %w", evmSelector, err)
	}
	minter, err := mock_token_minter.NewMockTokenMinter(minterAddr, evmChain.Client)
	if err != nil {
		return fmt.Errorf("bind mock token minter on chain %d: %w", evmSelector, err)
	}

	// The Solana mint is already 32 bytes; the pad keeps this correct for a 20-byte remote too.
	var burnToken [32]byte
	copy(burnToken[:], common.LeftPadBytes(remoteBurnToken, 32))
	localUSDCAddr := common.HexToAddress(localUSDC)

	existing, err := minter.GetLocalToken(nil, remoteDomain, burnToken)
	if err != nil {
		return fmt.Errorf("read local token for domain %d on chain %d: %w", remoteDomain, evmSelector, err)
	}
	if existing == localUSDCAddr {
		return nil
	}

	tx, err := minter.SetLocalToken(evmChain.DeployerKey, remoteDomain, burnToken, localUSDCAddr)
	if err != nil {
		return fmt.Errorf("set local token for domain %d on chain %d: %w", remoteDomain, evmSelector, err)
	}
	if _, err = evmChain.Confirm(tx); err != nil {
		return fmt.Errorf("confirm setLocalToken for domain %d on chain %d: %w", remoteDomain, evmSelector, err)
	}

	return nil
}

// configureEVMCCTPVerifierForRemote applies the CCTPVerifier remote-chain config and domain entry
// that the pinned EVM sequence's buildCCTPVerifierArgs skips for non-EVM remotes.
//
// Without the remote-chain config the verifier's router lookup for the Solana source chain is the
// zero address, so verifyMessage reverts with RemoteChainNotSupported(solanaSelector). Without the
// domain entry it would then revert with UnknownDomain. Both mirror what buildCCTPVerifierArgs
// would have produced had the isEVMRemote guard not dropped the Solana remote.
func configureEVMCCTPVerifierForRemote(
	env *deployment.Environment,
	evmSelector, remoteSelector uint64,
	remoteAdapter ccipadapters.RemoteCCTPChain,
	remoteChain ccipadapters.RemoteCCTPChainConfig,
) error {
	evmChain, ok := env.BlockChains.EVMChains()[evmSelector]
	if !ok {
		return fmt.Errorf("evm chain not found for selector %d", evmSelector)
	}

	verifierRef, err := datastoreutils.FindAndFormatRef(env.DataStore, datastore.AddressRef{
		Type:    datastore.ContractType(cctpverifierops.ContractType),
		Version: cctpverifierops.Version,
	}, evmSelector, datastoreutils.FullRef)
	if err != nil {
		return fmt.Errorf("resolve CCTPVerifier on chain %d: %w", evmSelector, err)
	}
	routerRef, err := datastoreutils.FindAndFormatRef(env.DataStore, datastore.AddressRef{
		Type:    datastore.ContractType(routerops.ContractType),
		Version: routerops.Version,
	}, evmSelector, datastoreutils.FullRef)
	if err != nil {
		return fmt.Errorf("resolve Router on chain %d: %w", evmSelector, err)
	}
	verifierAddr := common.HexToAddress(verifierRef.Address)

	if _, err = operations.ExecuteOperation(env.OperationsBundle, cctpverifierops.ApplyRemoteChainConfigUpdates, evmChain,
		evmcontract.FunctionInput[[]cctpverifierops.RemoteChainConfigArgs]{
			ChainSelector: evmSelector,
			Address:       verifierAddr,
			Args: []cctpverifierops.RemoteChainConfigArgs{{
				Router:              common.HexToAddress(routerRef.Address),
				RemoteChainSelector: remoteSelector,
				FeeUSDCents:         remoteChain.FeeUSDCents,
				GasForVerification:  remoteChain.GasForVerification,
				PayloadSizeBytes:    remoteChain.PayloadSizeBytes,
			}},
		}); err != nil {
		return fmt.Errorf("apply CCTPVerifier remote chain config on chain %d for remote %d: %w", evmSelector, remoteSelector, err)
	}

	allowedCallerOnDest, err := remoteAdapter.CCTPV2AllowedCallerOnDest(env.DataStore, env.BlockChains, remoteSelector)
	if err != nil {
		return fmt.Errorf("resolve CCTP v2 allowed caller on remote %d: %w", remoteSelector, err)
	}
	allowedCallerOnSource, err := remoteAdapter.AllowedCallerOnSource(env.DataStore, env.BlockChains, remoteSelector)
	if err != nil {
		return fmt.Errorf("resolve CCTP allowed caller on source %d: %w", remoteSelector, err)
	}
	mintRecipientOnDest, err := remoteAdapter.MintRecipientOnDest(env.DataStore, env.BlockChains, remoteSelector)
	if err != nil {
		return fmt.Errorf("resolve CCTP mint recipient on remote %d: %w", remoteSelector, err)
	}

	if _, err = operations.ExecuteOperation(env.OperationsBundle, cctpverifierops.SetDomains, evmChain,
		evmcontract.FunctionInput[[]cctpverifierops.SetDomainArgs]{
			ChainSelector: evmSelector,
			Address:       verifierAddr,
			Args: []cctpverifierops.SetDomainArgs{{
				AllowedCallerOnDest:   toBytes32(allowedCallerOnDest),
				AllowedCallerOnSource: toBytes32(allowedCallerOnSource),
				MintRecipientOnDest:   toBytes32(mintRecipientOnDest),
				ChainSelector:         remoteSelector,
				DomainIdentifier:      remoteChain.DomainIdentifier,
				Enabled:               true,
			}},
		}); err != nil {
		return fmt.Errorf("set CCTPVerifier domain on chain %d for remote %d: %w", evmSelector, remoteSelector, err)
	}

	return nil
}

// toBytes32 left-pads an address to the bytes32 form the CCTPVerifier stores. Solana addresses are
// already 32 bytes; EVM addresses are 20.
func toBytes32(address []byte) [32]byte {
	var out [32]byte
	padded := common.LeftPadBytes(address, 32)
	copy(out[:], padded[len(padded)-32:])
	return out
}
