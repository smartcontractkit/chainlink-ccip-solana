package devenv

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/gagliardetto/solana-go"
	solrpc "github.com/gagliardetto/solana-go/rpc"

	chainsel "github.com/smartcontractkit/chain-selectors"
	"github.com/smartcontractkit/chainlink-ccip/deployment/finality"
	"github.com/smartcontractkit/chainlink-ccip/deployment/lanes"
	common_utils "github.com/smartcontractkit/chainlink-ccip/deployment/utils"
	v20adapters "github.com/smartcontractkit/chainlink-ccip/deployment/v2_0_0/adapters"
	ccipchangesets "github.com/smartcontractkit/chainlink-ccip/deployment/v2_0_0/changesets"
	"github.com/smartcontractkit/chainlink-ccv/build/devenv/chainreg"
	devenvcommon "github.com/smartcontractkit/chainlink-ccv/build/devenv/common"
	ccvdeployment "github.com/smartcontractkit/chainlink-ccv/deployment"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	"github.com/smartcontractkit/chainlink-deployments-framework/deployment"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/burnmint_token_pool"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/ccip_common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/ccip_offramp"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/ccip_router"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/cctp_token_pool"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/committee_verifier"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/example_ccip_receiver"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/example_ccip_receiver_v2"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/example_ccip_sender"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/example_ccip_sender_v2"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/executor"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/fee_quoter"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/lockrelease_token_pool"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/lombard_token_pool"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/mock_lombard_bridge"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/mock_lombard_mailbox"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/rmn_remote"
	contractscommon "github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	solstate "github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	soltokens "github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/tokens"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	cvops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/committeeverifier"
	executorops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/executor"
	fqops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/feequoter"
	offrampops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/offramp"
	rmnremoteops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/rmnremote"
	routerops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/router"
	testreceiverops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/testreceiver"
	testsenderops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/testsender"
	tokenpoolops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/tokenpools"
	tokensops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/tokens"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/sequences"
)

func (c *Chain) PreDeployContractsForSelector(ctx context.Context, env *deployment.Environment, selector uint64, topology *ccvdeployment.EnvironmentTopology) (datastore.DataStore, error) {
	// Note: with a fresh test-validator CommitmentFinalized can return 0, causing slot-1 to wrap to u64::MAX which the ALT program rejects.
	// Wait for at least 2 finalized slots before deploying. CreateLookupTable uses slot-1 as the recent slot;
	solChain, ok := env.BlockChains.SolanaChains()[selector]
	if ok {
		for {
			slot, err := solChain.Client.GetSlot(ctx, solrpc.CommitmentFinalized)
			if err != nil {
				return nil, fmt.Errorf("wait for finalized slot: %w", err)
			}
			if slot >= 2 {
				break
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(200 * time.Millisecond):
			}
		}
	}

	ds := datastore.NewMemoryDataStore()

	// LINK is a shared token the deploy sequence resolves from the datastore rather
	// than deploying, so devenv deploys its mock here and registers it like any other
	// environment-provided address.
	if ok {
		if solChain.DeployerKey == nil {
			return nil, fmt.Errorf("no Solana deployer key found for selector %d", selector)
		}
		tokenPrivKey, err := solana.NewRandomPrivateKey()
		if err != nil {
			return nil, fmt.Errorf("generate LINK mock key: %w", err)
		}
		bundle := operations.NewBundle(
			func() context.Context { return ctx },
			env.Logger,
			operations.NewMemoryReporter(),
		)
		linkRef, err := operations.ExecuteOperation(bundle, tokensops.DeployLINK, solChain, tokensops.Params{
			TokenPrivKey:  tokenPrivKey,
			TokenDecimals: 9,
		})
		if err != nil {
			return nil, fmt.Errorf("deploy LINK mock: %w", err)
		}
		if err := ds.AddressRefStore.Add(linkRef.Output); err != nil {
			return nil, fmt.Errorf("failed to add LINK: %w", err)
		}
	}

	// TODO: remove and test, since we're actually deploying programs
	// insert address refs so DeployChainContracts uses preloaded programs
	rmnRemotePubkey := rmn_remote.ProgramID
	routerPubkey := ccip_router.ProgramID
	executorPubkey := executor.ProgramID
	committeeVerifierPubkey := committee_verifier.ProgramID
	offRampPubkey := ccip_offramp.ProgramID
	feeQuoterPubkey := fee_quoter.ProgramID
	testReceiverV2Pubkey := example_ccip_receiver_v2.ProgramID
	testReceiverV1Pubkey := example_ccip_receiver.ProgramID

	addRef := func(ctype, version, qualifier string, addr solana.PublicKey) error {
		return ds.AddressRefStore.Add(datastore.AddressRef{
			Address:       addr.String(),
			Type:          datastore.ContractType(ctype),
			Version:       semver.MustParse(version),
			Qualifier:     qualifier,
			ChainSelector: selector,
		})
	}

	if err := addRef("Router", "2.0.0", "", routerPubkey); err != nil {
		return nil, fmt.Errorf("failed to add Router: %w", err)
	}
	if err := addRef("OffRamp", offrampops.Version.String(), "", offRampPubkey); err != nil {
		return nil, fmt.Errorf("failed to add OffRamp: %w", err)
	}
	// The message receiver used by the arbitrary-messaging e2e cases. Like the token pools it is
	// pre-loaded into the validator (via env.toml), so we only register its address here and
	// initialize and configure it in PostDeployContractsForSelector.
	if err := addRef(string(testreceiverops.ContractType), testreceiverops.Version.String(), devenvcommon.DefaultReceiverQualifier, testReceiverV2Pubkey); err != nil {
		return nil, fmt.Errorf("failed to add TestReceiver: %w", err)
	}
	// The legacy receiver is preloaded the same way. Registering it means every consumer resolves
	// both receivers from the datastore, instead of one from the datastore and one from a
	// gobinding declare_id! that only happens to be correct here. It shares the default qualifier
	// with v2, like staging, and the version tells them apart.
	if err := addRef(string(testreceiverops.ContractType), testreceiverops.LegacyVersion.String(), devenvcommon.DefaultReceiverQualifier, testReceiverV1Pubkey); err != nil {
		return nil, fmt.Errorf("failed to add legacy TestReceiver: %w", err)
	}
	// The example senders used by the Solana→EVM sender cases
	if err := addRef(string(testsenderops.ContractType), testsenderops.Version.String(), DefaultSenderQualifier, example_ccip_sender_v2.ProgramID); err != nil {
		return nil, fmt.Errorf("failed to add TestSender: %w", err)
	}
	if err := addRef(string(testsenderops.ContractType), testsenderops.LegacyVersion.String(), DefaultSenderQualifier, example_ccip_sender.ProgramID); err != nil {
		return nil, fmt.Errorf("failed to add legacy TestSender: %w", err)
	}
	if err := addRef("FeeQuoter", fqops.Version.String(), "", feeQuoterPubkey); err != nil {
		return nil, fmt.Errorf("failed to add FeeQuoter: %w", err)
	}
	if err := addRef(string(cvops.ContractType), cvops.Version.String(), "", committeeVerifierPubkey); err != nil {
		return nil, fmt.Errorf("failed to add CommitteeVerifier: %w", err)
	}
	defaultExecutorQualifier := devenvcommon.DefaultExecutorQualifier
	if err := addRef(string(executorops.ContractType), executorops.Version.String(), defaultExecutorQualifier, executorPubkey); err != nil {
		return nil, fmt.Errorf("failed to add Executor: %w", err)
	}
	// Register the executor directly as executorproxy as well, proxy abstractions leaks from EVM and all code interacts with the "proxy"
	if err := addRef(string(executorops.ProxyContractType), executorops.Version.String(), defaultExecutorQualifier, executorPubkey); err != nil {
		return nil, fmt.Errorf("failed to add Executor as ExecutorProxy: %w", err)
	}
	// Register RMNRemote at 1.6.0: the executor config adapter (executor_config_adapter.go)
	// and ccv expect this version. Other deployment operations look it up at
	// rmnremoteops.Version; register that too, but only when it differs — rmnremoteops.Version
	// is currently also 1.6.0, and adding the same (selector, type, version, qualifier) key
	// twice fails with "address ref already exists".
	const rmnRemoteCCVVersion = "1.6.0"
	if err := addRef(string(rmnremoteops.ContractType), rmnRemoteCCVVersion, "", rmnRemotePubkey); err != nil {
		return nil, fmt.Errorf("failed to add RMNRemote %s: %w", rmnRemoteCCVVersion, err)
	}
	if rmnremoteops.Version.String() != rmnRemoteCCVVersion {
		if err := addRef(string(rmnremoteops.ContractType), rmnremoteops.Version.String(), "", rmnRemotePubkey); err != nil {
			return nil, fmt.Errorf("failed to add RMNRemote %s: %w", rmnremoteops.Version.String(), err)
		}
	}

	// Register token pool programs in the datastore. On Solana, pool programs are
	// pre-deployed and loaded into the validator (via env.toml), not deployed through
	// CLDF operations. Register them here so downstream sequences like
	// DeployTokenPoolForToken can look them up from the datastore instead of
	// hardcoding Go binding ProgramIDs. Uses PoolProgramQualifier (not empty) to
	// distinguish program entries from per-token instance entries.
	for _, pool := range c.GetSupportedPools() {
		var programAddr solana.PublicKey
		switch pool.PoolType {
		case devenvcommon.BurnMintTokenPoolType:
			programAddr = burnmint_token_pool.ProgramID
		case devenvcommon.LockReleaseTokenPoolType:
			programAddr = lockrelease_token_pool.ProgramID
		default:
			return nil, fmt.Errorf("unsupported pool type %q", pool.PoolType)
		}
		if err := addRef(pool.PoolType, pool.PoolVersion.String(), solcommon.PoolProgramQualifier, programAddr); err != nil {
			return nil, fmt.Errorf("failed to add %s %s program: %w", pool.PoolType, pool.PoolVersion, err)
		}
	}

	// Lombard's pool+verifier program and its mock bridge/mailbox are pre-loaded the same way
	// register them so deployLombardTokenAndPool resolves them from the datastore.
	if err := addRef(string(tokenpoolops.LombardTokenPool), solcommon.Version2_0_0.String(), solcommon.PoolProgramQualifier, lombard_token_pool.ProgramID); err != nil {
		return nil, fmt.Errorf("failed to add Lombard token pool program: %w", err)
	}
	// Register the Solana mock bridge/mailbox programs under Lombard's canonical types. The
	// v2_0_0 configure sequence resolves them as LombardBridgeV2/LombardMailbox, the same types
	// the staging datastore uses for Lombard's real programs. The EVM mock bridge lives in
	// chainlink-ccv and keeps its own "MockLombardBridge" ref.
	if err := addRef(solcommon.LombardBridgeV2ContractType, solcommon.LombardChainContractsVersion.String(), "", mock_lombard_bridge.ProgramID); err != nil {
		return nil, fmt.Errorf("failed to add lombard bridge program: %w", err)
	}
	// The pinned chainlink-ccv devenv (build/devenv/evm/lombard.go, configureLombardForTransfer)
	// resolves the REMOTE Lombard bridge with the hard-coded key (remoteSelector, MockLombardBridge,
	// 2.0.0, "Lombard"). Register the same Solana mock bridge program under that key too, so the
	// pinned ccv can resolve it. Both keys point at the same program; remove this once ccv resolves
	// the remote bridge through the remote family adapter.
	if err := addRef(MockLombardBridgeContractType, "2.0.0", devenvcommon.LombardContractsQualifier, mock_lombard_bridge.ProgramID); err != nil {
		return nil, fmt.Errorf("failed to add mock lombard bridge program: %w", err)
	}
	if err := addRef(solcommon.LombardMailboxContractType, solcommon.LombardChainContractsVersion.String(), "", mock_lombard_mailbox.ProgramID); err != nil {
		return nil, fmt.Errorf("failed to add lombard mailbox program: %w", err)
	}
	// The CCTP pool+verifier program is pre-loaded the same way, but it is deliberately not part
	// of GetSupportedPools: CCTP is not one of the generic pool types the token-combination
	// machinery expands, so it is registered directly here for deployCCTPTokenAndPool to resolve.
	if err := addRef(string(common_utils.CCTPTokenPool), solcommon.Version2_0_0.String(), solcommon.PoolProgramQualifier, cctp_token_pool.ProgramID); err != nil {
		return nil, fmt.Errorf("failed to add CCTP token pool program: %w", err)
	}

	return ds.Seal(), nil
}

// receiverSourceChain returns the EVM chain the Solana receiver accepts messages from, plus that
// chain's sender address as hex. devenv wires a single Solana<>EVM lane pair; when more than one
// EVM chain is present the lowest selector is used so the choice stays deterministic.
func receiverSourceChain(env *deployment.Environment) (uint64, string, error) {
	evmChains := env.BlockChains.EVMChains()
	var evmSelector uint64
	for sel := range evmChains {
		if evmSelector == 0 || sel < evmSelector {
			evmSelector = sel
		}
	}
	if evmSelector == 0 {
		return 0, "", errors.New("no EVM chain found to configure the Solana test receiver")
	}
	evmChain := evmChains[evmSelector]
	if evmChain.DeployerKey == nil {
		return 0, "", fmt.Errorf("no EVM deployer key for selector %d", evmSelector)
	}
	return evmSelector, evmChain.DeployerKey.From.Hex(), nil
}

func (c *Chain) PostDeployContractsForSelector(ctx context.Context, env *deployment.Environment, selector uint64, topology *ccvdeployment.EnvironmentTopology) (datastore.DataStore, error) {
	c.logger.Info().Uint64("selector", selector).Msg("Solana: PostDeployContractsForSelector")

	ds := datastore.NewMemoryDataStore()
	if err := ds.Merge(env.DataStore); err != nil {
		return nil, fmt.Errorf("failed to seed post-deploy DS: %w", err)
	}

	solChains := env.BlockChains.SolanaChains()
	solChain, ok := solChains[selector]
	if !ok {
		return nil, fmt.Errorf("no Solana chain found for selector %d", selector)
	}
	if solChain.DeployerKey == nil {
		return nil, fmt.Errorf("no Solana deployer key found for selector %d", selector)
	}
	localFamily, err := chainsel.GetSelectorFamily(selector)
	if err != nil {
		return nil, fmt.Errorf("get selector family for selector %d: %w", selector, err)
	}
	if localFamily != chainsel.FamilySolana {
		return nil, fmt.Errorf("solana devenv CommitteeVerifier expected local selector %d to be family %q, got %q", selector, chainsel.FamilySolana, localFamily)
	}

	// Initialize the message receivers (creates their counter + external_execution_config PDAs and
	// records the router they trust). Required for the arbitrary-messaging e2e cases: the offramp
	// CPIs into the receiver's `ccip_receive`, and its allowed_offramp check needs `state.router`
	// to match the deployed router. Pure token transfers (zero receiver) do not touch it.
	bundle := operations.NewBundle(
		func() context.Context { return ctx },
		env.Logger,
		operations.NewMemoryReporter(),
	)

	router, err := solcommon.GetDataStoreAddress(env.DataStore, datastore.NewAddressRefKey(
		selector, datastore.ContractType("Router"), semver.MustParse("2.0.0"), "",
	))
	if err != nil {
		return nil, fmt.Errorf("resolve router for test receiver: %w", err)
	}
	receiverV2, err := solcommon.GetDataStoreAddress(env.DataStore, datastore.NewAddressRefKey(
		selector, datastore.ContractType(testreceiverops.ContractType), testreceiverops.Version, devenvcommon.DefaultReceiverQualifier,
	))
	if err != nil {
		return nil, fmt.Errorf("resolve test receiver v2: %w", err)
	}
	receiverV1, err := solcommon.GetDataStoreAddress(env.DataStore, datastore.NewAddressRefKey(
		selector, datastore.ContractType(testreceiverops.ContractType), testreceiverops.LegacyVersion, devenvcommon.DefaultReceiverQualifier,
	))
	if err != nil {
		return nil, fmt.Errorf("resolve test receiver v1: %w", err)
	}

	// The v1 receiver is legacy; initialize it with the legacy version.
	if _, err = operations.ExecuteOperation(bundle, testreceiverops.Initialize, solChain, testreceiverops.Params{
		Router:          router,
		Receiver:        receiverV1,
		ReceiverVersion: testreceiverops.LegacyVersion,
	}); err != nil {
		return nil, fmt.Errorf("initialize test receiver v1: %w", err)
	}
	if _, err = operations.ExecuteOperation(bundle, testreceiverops.Initialize, solChain, testreceiverops.Params{
		Router:   router,
		Receiver: receiverV2,
	}); err != nil {
		return nil, fmt.Errorf("initialize test receiver v2: %w", err)
	}

	// Register the v2 receiver with the router, set its CCV config and approve the remote sender,
	// so the arbitrary-messaging cases find it ready. These are the same ops DeployExampleReceiver
	// runs after Initialize; devenv skips its Deploy step because the program is preloaded and its
	// address is already seeded. Each op binds its gobinding ProgramID from Receiver, so none of
	// this depends on declare_id!.
	committeeVerifier, err := solcommon.GetDataStoreAddress(env.DataStore, datastore.NewAddressRefKey(
		selector, datastore.ContractType(cvops.ContractType), cvops.Version, "",
	))
	if err != nil {
		return nil, fmt.Errorf("resolve committee verifier for test receiver: %w", err)
	}
	evmSelector, evmSender, err := receiverSourceChain(env)
	if err != nil {
		return nil, err
	}
	if _, err = operations.ExecuteOperation(bundle, testreceiverops.RegisterWithRouter, solChain, testreceiverops.RegisterWithRouterParams{
		Router:   router,
		Receiver: receiverV2,
	}); err != nil {
		return nil, fmt.Errorf("register test receiver v2 with router: %w", err)
	}
	if _, err = operations.ExecuteOperation(bundle, testreceiverops.SetCCVConfig, solChain, testreceiverops.SetCCVConfigParams{
		Receiver:              receiverV2,
		SourceChainSelector:   evmSelector,
		RequiredCCVs:          []solana.PublicKey{committeeVerifier},
		OptionalCCVs:          []solana.PublicKey{},
		OptionalThreshold:     0,
		AllowedFinalityConfig: example_ccip_receiver_v2.FinalityConfig{BlockDepth: 1},
	}); err != nil {
		return nil, fmt.Errorf("set test receiver v2 CCV config: %w", err)
	}
	if _, err = operations.ExecuteOperation(bundle, testreceiverops.ApproveSender, solChain, testreceiverops.ApproveSenderParams{
		Receiver:            receiverV2,
		SourceChainSelector: evmSelector,
		RemoteSender:        evmSender,
	}); err != nil {
		return nil, fmt.Errorf("approve remote sender on test receiver v2: %w", err)
	}
	// The v1 receiver only needs its sender approved; it has no router registration or CCV config.
	if _, err = operations.ExecuteOperation(bundle, testreceiverops.ApproveSender, solChain, testreceiverops.ApproveSenderParams{
		Receiver:            receiverV1,
		SourceChainSelector: evmSelector,
		RemoteSender:        evmSender,
		ReceiverVersion:     testreceiverops.LegacyVersion,
	}); err != nil {
		return nil, fmt.Errorf("approve remote sender on test receiver v1: %w", err)
	}

	if err = configureTestSenders(bundle, env, solChain, selector, router, evmSelector, evmSender); err != nil {
		return nil, err
	}

	if err = c.deployLombardTokenAndPool(ctx, env, ds, selector); err != nil {
		return nil, fmt.Errorf("failed to deploy lombard token and pool: %w", err)
	}
	if err = c.deployCCTPTokenAndPool(ctx, env, ds, selector); err != nil {
		return nil, fmt.Errorf("failed to deploy cctp token and pool: %w", err)
	}

	return onlyNewAddresses(ds, env.DataStore)
}

// DefaultSenderQualifier is the qualifier both example senders are registered under in devenv.
const DefaultSenderQualifier = "default"

// testSenderGasLimit is the EVM execution gas limit the v2 sender requests for every message. It
// covers the Lombard mailbox attestation verify, the most expensive path sent through the sender.
const testSenderGasLimit = 1_000_000

// testSenderFunding is the lamport balance the v1 sender's ccip_sender PDA is topped up to. That
// PDA fronts the router's native SOL fee and nonce rent, which the caller then pays back.
const testSenderFunding = 5 * solana.LAMPORTS_PER_SOL

// configureTestSenders initializes both preloaded example senders and enables the EVM chain as a
// destination for the v1 sender. These are the same ops DeployV2TestSender/DeployLegacyTestSender
// run after Deploy. The v2 sender's destinations are set in PostConnect by
// configureTestSenderV2Destinations.
//
// The v1 recipient is the EVM deployer EOA: the v1 case only covers the source side.
func configureTestSenders(
	bundle operations.Bundle,
	env *deployment.Environment,
	solChain cldf_solana.Chain,
	selector uint64,
	router solana.PublicKey,
	evmSelector uint64,
	evmRecipient string,
) error {
	senderV2, err := solcommon.GetDataStoreAddress(env.DataStore, datastore.NewAddressRefKey(
		selector, datastore.ContractType(testsenderops.ContractType), testsenderops.Version, DefaultSenderQualifier,
	))
	if err != nil {
		return fmt.Errorf("resolve test sender v2: %w", err)
	}
	senderV1, err := solcommon.GetDataStoreAddress(env.DataStore, datastore.NewAddressRefKey(
		selector, datastore.ContractType(testsenderops.ContractType), testsenderops.LegacyVersion, DefaultSenderQualifier,
	))
	if err != nil {
		return fmt.Errorf("resolve test sender v1: %w", err)
	}

	for _, s := range []struct {
		sender  solana.PublicKey
		version *semver.Version
	}{
		{senderV1, testsenderops.LegacyVersion},
		{senderV2, testsenderops.Version},
	} {
		if _, err = operations.ExecuteOperation(bundle, testsenderops.Initialize, solChain, testsenderops.Params{
			Router:        router,
			Sender:        s.sender,
			SenderVersion: s.version,
		}); err != nil {
			return fmt.Errorf("initialize test sender %s: %w", s.version, err)
		}
	}

	// The legacy sender sends through the router's 1.6 ccip_send, which needs its own lane to the
	// destination and applies its own defaults when the extra args are empty. That send also
	// re-emits its event through RMN Remote, signed by the router's fee billing signer, which must
	// be one of RMN Remote's event authorities.
	if _, err = operations.ExecuteOperation(bundle, routerops.AddLegacyDestChain, solChain, routerops.AddLegacyDestChainParams{
		Router:              router,
		RemoteChainSelector: evmSelector,
	}); err != nil {
		return fmt.Errorf("add legacy router lane to chain %d: %w", evmSelector, err)
	}
	if _, err = operations.ExecuteSequence(bundle, sequences.SetRMNRemoteEventAuthorities, env.BlockChains, sequences.RMNRemoteSetEventAuthoritiesSequenceInput{
		DataStore: env.DataStore,
		Selector:  selector,
	}); err != nil {
		return fmt.Errorf("set RMN remote event authorities: %w", err)
	}
	if _, err = operations.ExecuteOperation(bundle, testsenderops.SetChainConfig, solChain, testsenderops.SetChainConfigParams{
		Sender:            senderV1,
		DestChainSelector: evmSelector,
		Recipient:         evmRecipient,
		SenderVersion:     testsenderops.LegacyVersion,
	}); err != nil {
		return fmt.Errorf("set test sender v1 chain config: %w", err)
	}

	// Both senders need their ccip_sender PDA's token account for the fee mint up front: fees are
	// quoted against wrapped SOL, and that account is checked (v2: by the router, v1: by the
	// sender's own ccip_send constraints) before any work.
	for _, s := range []struct {
		sender  solana.PublicKey
		version *semver.Version
	}{
		{senderV1, testsenderops.LegacyVersion},
		{senderV2, testsenderops.Version},
	} {
		if _, err = operations.ExecuteOperation(bundle, testsenderops.InitCcipSenderATA, solChain, testsenderops.InitCcipSenderATAParams{
			Sender:        s.sender,
			Mint:          solana.WrappedSol,
			SenderVersion: s.version,
		}); err != nil {
			return fmt.Errorf("init test sender %s wSOL ATA: %w", s.version, err)
		}
	}

	// Only v1 needs a funded ccip_sender PDA, to front the send; v2 takes the fee from the caller up
	// front.
	if _, err = operations.ExecuteOperation(bundle, testsenderops.FundCcipSender, solChain, testsenderops.FundCcipSenderParams{
		Sender:   senderV1,
		Lamports: testSenderFunding,
	}); err != nil {
		return fmt.Errorf("fund test sender %s ccip_sender PDA: %w", testsenderops.LegacyVersion, err)
	}
	return nil
}

// onlyNewAddresses returns a sealed DataStore containing only the AddressRefs
// present in full that are not already in base. This prevents the seeded
// env.DataStore content from leaking into the caller's accumulator.
func onlyNewAddresses(full *datastore.MemoryDataStore, base datastore.DataStore) (datastore.DataStore, error) {
	baseAddrs, err := base.Addresses().Fetch()
	if err != nil {
		return nil, fmt.Errorf("fetch base addresses: %w", err)
	}
	// Keyed on (ref key, address): some devenv-only components are redeployed fresh on every run
	// under the same ref key, since the underlying account can't survive a validator reset.
	// Comparing by key alone would treat that as "already known" and silently drop the new address
	// from the merged datastore, leaving consumers resolving a stale, no-longer-existent address
	// from an earlier devenv session.
	baseKeys := make(map[string]struct{}, len(baseAddrs))
	for _, a := range baseAddrs {
		baseKeys[a.Key().String()+"|"+a.Address] = struct{}{}
	}

	allAddrs, err := full.Seal().Addresses().Fetch()
	if err != nil {
		return nil, fmt.Errorf("fetch full addresses: %w", err)
	}

	result := datastore.NewMemoryDataStore()
	for _, a := range allAddrs {
		if _, exists := baseKeys[a.Key().String()+"|"+a.Address]; !exists {
			if err := result.Addresses().Add(a); err != nil {
				return nil, fmt.Errorf("add new address to result: %w", err)
			}
		}
	}
	return result.Seal(), nil
}

func (c *Chain) GetDeployChainContractsCfg(_ *deployment.Environment, _ uint64, _ *ccvdeployment.EnvironmentTopology) (ccipchangesets.DeployChainContractsPerChainCfg, error) {
	fqVersion := semver.MustParse("2.0.0")
	execVersion := semver.MustParse("2.0.0")
	executors := []v20adapters.ExecutorDeployParams{
		{
			Version:       execVersion,
			MaxCCVsPerMsg: 10,
			DynamicConfig: v20adapters.ExecutorDynamicDeployConfig{
				FeeAggregator:         "",
				CcvAllowlistEnabled:   false,
				AllowedFinalityConfig: finality.Config{BlockDepth: 1, WaitForSafe: false},
			},
			Qualifier: devenvcommon.DefaultExecutorQualifier,
		},
	}
	return ccipchangesets.DeployChainContractsPerChainCfg{
		DeployerContract: new("devenv"),
		DeployerKeyOwned: true,
		ContractParams: &v20adapters.DeployContractParamsOverrides{
			FeeQuoter: &v20adapters.FeeQuoterDeployParamsOverrides{
				Version:           fqVersion,
				MaxFeeJuelsPerMsg: big.NewInt(0).Mul(big.NewInt(2e2), big.NewInt(1e9)),
			},
			Executors: &executors,
		},
	}, nil
}

// PostTokenDeploy funds the lock-release token pools deployed on this Solana chain
// so they can release tokens as a destination. It only runs as part of a fresh deploy;
// attached persistent environments never execute this pipeline step.
func (c *Chain) PostTokenDeploy(env *deployment.Environment, selector uint64, deployedRefs []datastore.AddressRef) error {
	chain, ok := env.BlockChains.SolanaChains()[selector]
	if !ok {
		return fmt.Errorf("solana chain not found for selector %d", selector)
	}
	if chain.DeployerKey == nil {
		return errors.New("solana deployer key not set")
	}

	for _, ref := range deployedRefs {
		if ref.Type != datastore.ContractType(devenvcommon.LockReleaseTokenPoolType) {
			continue
		}
		mint, err := solcommon.GetDataStoreAddress(env.DataStore, datastore.NewAddressRefKey(
			selector, datastore.ContractType(solcommon.SPLTokens), solcommon.Version2_0_0, ref.Qualifier,
		))
		if err != nil {
			return fmt.Errorf("resolve mint for lock-release pool %q: %w", ref.Qualifier, err)
		}
		if err := provideLockReleaseLiquidity(context.Background(), chain, mint, solanaLockReleaseLiquidity); err != nil {
			return fmt.Errorf("fund lock-release pool %q: %w", ref.Qualifier, err)
		}
	}
	return nil
}

// GetConnectionProfile implements cciptestinterfaces.OnChainConfigurable. Solana devenv only
// wires lanes via ConfigureChainsForLanesFromTopology; this method always errors.
func (c *Chain) GetConnectionProfile(_ *deployment.Environment, _ uint64) (lanes.ChainDefinition, lanes.CommitteeVerifierRemoteChainInput, error) {
	return lanes.ChainDefinition{}, lanes.CommitteeVerifierRemoteChainInput{},
		errors.New("solana devenv: use ConfigureChainsForLanesFromTopology (not connection-profile lane wiring)")
}

func (c *Chain) GetChainLaneProfile(_ *deployment.Environment, selector uint64) (ccipchangesets.ChainOverrides, error) {
	baseExecGas := uint32(150_000)
	tokenReceiverAllowed := false
	defaultExecutorQualifier := devenvcommon.DefaultExecutorQualifier
	return ccipchangesets.ChainOverrides{
		RemoteChainCfg: ccipchangesets.PartialRemoteChainConfig{
			BaseExecutionGasCost:     &baseExecGas,
			TokenReceiverAllowed:     &tokenReceiverAllowed,
			DefaultExecutorQualifier: &defaultExecutorQualifier,
			FeeQuoterDestChainConfig: v20adapters.FeeQuoterDestChainConfigOverrides{
				USDPerUnitGas: devenvUSDPerUnitGas,
			},
			ExecutorDestChainConfig: &v20adapters.ExecutorDestChainConfig{
				Enabled: true,
			},
			DefaultInboundCCVs: []datastore.AddressRef{
				{
					Type:          datastore.ContractType(cvops.ContractType),
					Version:       semver.MustParse("2.0.0"),
					ChainSelector: selector,
					Qualifier:     devenvcommon.DefaultCommitteeVerifierQualifier,
				},
			},
			DefaultOutboundCCVs: []datastore.AddressRef{
				{
					Type:          datastore.ContractType(cvops.ContractType),
					Version:       semver.MustParse("2.0.0"),
					ChainSelector: selector,
					Qualifier:     devenvcommon.DefaultCommitteeVerifierQualifier,
				},
			},
		},
	}, nil
}

func (c *Chain) PostConnect(env *deployment.Environment, selector uint64, remoteSelectors []uint64) error {
	// TODO: Code for custom executor support once we add some definitions
	// solChains := env.BlockChains.SolanaChains()
	// solChain, ok := solChains[selector]
	// if !ok {
	// 	return fmt.Errorf("no Solana chain found for selector %d", selector)
	// }
	// customExecutor, err := executorPubkeyFromDataStore(env.DataStore, selector, devenvcommon.CustomExecutorQualifier)
	// if err != nil {
	// 	return fmt.Errorf("failed to get custom executor address: %w", err)
	// }
	// for _, remoteSelector := range remoteSelectors {
	// 	// TODO: batch setConfig
	// 	_, err := operations.ExecuteOperation(
	// 		env.OperationsBundle,
	// 		executorops.SetChainConfig,
	// 		solChain,
	// 		executorops.SetChainConfigParams{
	// 			Executor:            customExecutor,
	// 			RemoteChainSelector: remoteSelector,
	// 			UsdCentsFee:         0,
	// 			Enabled:             true,
	// 		},
	// 	)
	// 	if err != nil {
	// 		return fmt.Errorf("failed to apply dest chain updates to custom executor: %w", err)
	// 	}
	// }
	if err := ensureV1FeeQuoterDestChains(env, selector, remoteSelectors); err != nil {
		return err
	}
	if err := configureLombardForTransfer(env, selector, remoteSelectors); err != nil {
		return err
	}
	if err := configureCCTPLaneForRemote(env, selector, remoteSelectors); err != nil {
		return err
	}
	if err := configureTestSenderV2Destinations(env, selector, remoteSelectors); err != nil {
		return err
	}
	return ensureV2PoolTokenTransferFeeConfigs(env, selector, remoteSelectors)
}

// configureTestSenderV2Destinations points the v2 example sender at each remote EVM chain's mock
// receiver, with the CCVs left to the router.
func configureTestSenderV2Destinations(env *deployment.Environment, selector uint64, remoteSelectors []uint64) error {
	solChain, ok := env.BlockChains.SolanaChains()[selector]
	if !ok {
		return fmt.Errorf("no Solana chain found for selector %d", selector)
	}
	sender, err := solcommon.GetDataStoreAddress(env.DataStore, datastore.NewAddressRefKey(
		selector, datastore.ContractType(testsenderops.ContractType), testsenderops.Version, DefaultSenderQualifier,
	))
	if err != nil {
		return fmt.Errorf("resolve test sender v2: %w", err)
	}
	evmReg, err := chainreg.GetRegistry().Get(chainsel.FamilyEVM)
	if err != nil {
		return fmt.Errorf("get EVM chain registry: %w", err)
	}
	extraArgs, err := sequences.V2TestSenderExtraArgs(testSenderGasLimit, ccip_common.FinalityConfig{}, nil)
	if err != nil {
		return fmt.Errorf("build test sender v2 extra args: %w", err)
	}
	for _, rs := range remoteSelectors {
		family, err := chainsel.GetSelectorFamily(rs)
		if err != nil {
			return fmt.Errorf("get selector family for remote %d: %w", rs, err)
		}
		if family != chainsel.FamilyEVM {
			continue
		}
		receiver, err := evmReg.AddressResolver.GetContractReceiver(env.DataStore, rs, devenvcommon.DefaultReceiverQualifier)
		if err != nil {
			return fmt.Errorf("resolve EVM mock receiver on chain %d: %w", rs, err)
		}
		if _, err = operations.ExecuteOperation(env.OperationsBundle, testsenderops.SetChainConfig, solChain, testsenderops.SetChainConfigParams{
			Sender:            sender,
			DestChainSelector: rs,
			Recipient:         hex.EncodeToString(receiver),
			ExtraArgs:         extraArgs,
			SenderVersion:     testsenderops.Version,
		}); err != nil {
			return fmt.Errorf("set test sender v2 chain config for chain %d: %w", rs, err)
		}
	}
	return nil
}

// devenvUSDPerUnitGas is the lane gas price this devenv seeds, as 18-decimals USD per single
// unit of destination gas. It lives on the fee quoter's v1 [dest_chain] account, which is what
// quote_gas_for_exec_v2 reads (see seedV1FeeQuoterGasPrice).
var devenvUSDPerUnitGas = big.NewInt(1e6)

// tokenPoolV2FinalityFeeUSDCents, tokenPoolV2FastFinalityFeeUSDCents, tokenPoolV2DestGasOverhead,
// tokenPoolV2DestBytesOverhead and tokenPoolV2FastFinalityBpsFee mirror the reference config used
// in contracts/tests/ccip/tokenpool_test.go: small enough to not distort execution cost, using the
// same DestGasOverhead/DestBytesOverhead the fee-quoter fallback already uses so destination
// execution isn't affected by switching a pool from the fallback to its own fee quote.
const (
	tokenPoolV2FinalityFeeUSDCents     = 50
	tokenPoolV2FastFinalityFeeUSDCents = 75
	tokenPoolV2DestGasOverhead         = 90_000
	tokenPoolV2DestBytesOverhead       = 32
	tokenPoolV2FastFinalityBpsFee      = 20

	// TokenPoolFinalityBpsFee is the BPS fee (out of 10_000) devenv configures on every 2.0.0
	// token pool. Exported so e2e tests can compute the exact post-fee transfer amount
	// (amount - amount*TokenPoolFinalityBpsFee/10_000) without duplicating pool fee logic.
	TokenPoolFinalityBpsFee = 10
)

// ensureV2PoolTokenTransferFeeConfigs configures a small non-zero flat + BPS token-transfer fee
// on every 2.0.0 token pool deployed on this chain, for each remote lane. Nothing in deployment/
// or devenv/ otherwise calls init_chain_config_v2, so every pool's own get_fee reports
// is_enabled=false and every transfer falls back to the fee-quoter's default fee with a zero BPS
// fee -- this is purely so e2e tests can exercise the pool's own flat+BPS fee path.
//
// AllowedFinalityConfig and CcvConfig are left at their zero value, which is exactly equivalent
// to today's "account absent" behavior (finality=0 is unconditionally allowed regardless of
// allowed_finality_config; empty CCV lists fall back to the lane's default CCVs via
// pool_ccvs_with_default_fallback) -- only the fee config actually changes for existing lanes.
func ensureV2PoolTokenTransferFeeConfigs(env *deployment.Environment, selector uint64, remoteSelectors []uint64) error {
	solChain, ok := env.BlockChains.SolanaChains()[selector]
	if !ok {
		return fmt.Errorf("no Solana chain found for selector %d", selector)
	}
	authority := solChain.DeployerKey.PublicKey()

	// Every combo qualifier of a given (chain, pool type, version) resolves to the same deployed
	// program on Solana (one program instance handles every mint via distinct PDAs), so dedupe by
	// (program, mint) to avoid redundant repeat calls for what is really the same pool+token pair.
	seen := make(map[[2]solana.PublicKey]bool)

	for _, poolType := range []string{devenvcommon.BurnMintTokenPoolType, devenvcommon.LockReleaseTokenPoolType} {
		refs := env.DataStore.Addresses().Filter(
			datastore.AddressRefByChainSelector(selector),
			datastore.AddressRefByType(datastore.ContractType(poolType)),
		)
		for _, ref := range refs {
			if !solcommon.RefVersionMatches(ref.Version, solcommon.Version2_0_0) {
				continue
			}
			// PoolProgramQualifier is a plain alias to the deployed program, not tied to any
			// mint; skip it here (TokenRefForPool would fail to resolve a mint for it).
			if ref.Qualifier == solcommon.PoolProgramQualifier {
				continue
			}
			poolProgram, err := solcommon.ToAddress(ref)
			if err != nil {
				return fmt.Errorf("resolve pool program for %s qualifier %q: %w", poolType, ref.Qualifier, err)
			}
			mintRef := TokenRefForPool(ref)
			mint, err := solcommon.GetDataStoreAddress(env.DataStore, datastore.NewAddressRefKey(
				selector, mintRef.Type, mintRef.Version, mintRef.Qualifier,
			))
			if err != nil {
				// Not every hand-curated combo qualifier is actually deployed in this
				// environment (see devenvcommon.AllTokenCombinations' doc comment); skip
				// whatever doesn't resolve to a real mint instead of failing devenv bootup.
				continue
			}
			key := [2]solana.PublicKey{poolProgram, mint}
			if seen[key] {
				continue
			}
			seen[key] = true

			statePDA, err := soltokens.TokenPoolConfigAddress(mint, poolProgram)
			if err != nil {
				return fmt.Errorf("derive pool config PDA for %s qualifier %q: %w", poolType, ref.Qualifier, err)
			}
			eventAuthority, _, err := solstate.FindEventAuthorityPDA(poolProgram)
			if err != nil {
				return fmt.Errorf("derive event authority for %s qualifier %q: %w", poolType, ref.Qualifier, err)
			}

			// set_chain_config_v2 creates the fee vault ATA for the pool's own mint, which is
			// where lock_or_burn_tokens_v2 puts the in-token BPS fee configured below. The mint's
			// owner is its token program (these combos span both SPL Token and Token-2022).
			mintInfo, err := solChain.Client.GetAccountInfo(context.Background(), mint)
			if err != nil {
				return fmt.Errorf("fetch mint for %s qualifier %q: %w", poolType, ref.Qualifier, err)
			}
			if mintInfo == nil || mintInfo.Value == nil {
				return fmt.Errorf("mint %s for %s qualifier %q not found", mint, poolType, ref.Qualifier)
			}
			tokenProgram := mintInfo.Value.Owner
			feeVault, _, err := solstate.FindFeeVaultPDA(poolProgram, mint.Bytes())
			if err != nil {
				return fmt.Errorf("derive fee vault for %s qualifier %q: %w", poolType, ref.Qualifier, err)
			}
			feeVaultATA, err := soltokens.FindFeeVaultATA(tokenProgram, mint, poolProgram, mint.Bytes())
			if err != nil {
				return fmt.Errorf("derive fee vault ATA for %s qualifier %q: %w", poolType, ref.Qualifier, err)
			}

			for _, remoteSelector := range remoteSelectors {
				chainConfigV2PDA, _, err := soltokens.TokenPoolChainConfigV2PDA(remoteSelector, mint, poolProgram)
				if err != nil {
					return fmt.Errorf("derive chain config v2 PDA for %s qualifier %q remote %d: %w", poolType, ref.Qualifier, remoteSelector, err)
				}

				var ixn solana.Instruction
				switch poolType {
				case devenvcommon.BurnMintTokenPoolType:
					ixn, err = burnmint_token_pool.NewInitChainConfigV2Instruction(
						burnmint_token_pool.SetChainConfigV2Params{
							Mint:                  mint,
							RemoteChainSelector:   remoteSelector,
							AllowedFinalityConfig: burnmint_token_pool.FinalityConfig{},
							CcvConfig:             burnmint_token_pool.CcvConfig{},
							TokenTransferFeeConfig: burnmint_token_pool.TokenTransferFeeConfig{
								DestGasOverhead:    burnmint_token_pool.CrossChainGas{V0: tokenPoolV2DestGasOverhead},
								DestBytesOverhead:  tokenPoolV2DestBytesOverhead,
								FinalityFee:        burnmint_token_pool.UsdCents{V0: tokenPoolV2FinalityFeeUSDCents},
								FastFinalityFee:    burnmint_token_pool.UsdCents{V0: tokenPoolV2FastFinalityFeeUSDCents},
								FinalityBpsFee:     TokenPoolFinalityBpsFee,
								FastFinalityBpsFee: tokenPoolV2FastFinalityBpsFee,
								IsEnabled:          true,
							},
						},
						statePDA, chainConfigV2PDA, authority, solana.SystemProgramID, mint, feeVault, feeVaultATA,
						tokenProgram, solana.SPLAssociatedTokenAccountProgramID, eventAuthority, poolProgram,
					)
				case devenvcommon.LockReleaseTokenPoolType:
					ixn, err = lockrelease_token_pool.NewInitChainConfigV2Instruction(
						lockrelease_token_pool.SetChainConfigV2Params{
							Mint:                  mint,
							RemoteChainSelector:   remoteSelector,
							AllowedFinalityConfig: lockrelease_token_pool.FinalityConfig{},
							CcvConfig:             lockrelease_token_pool.CcvConfig{},
							TokenTransferFeeConfig: lockrelease_token_pool.TokenTransferFeeConfig{
								DestGasOverhead:    lockrelease_token_pool.CrossChainGas{V0: tokenPoolV2DestGasOverhead},
								DestBytesOverhead:  tokenPoolV2DestBytesOverhead,
								FinalityFee:        lockrelease_token_pool.UsdCents{V0: tokenPoolV2FinalityFeeUSDCents},
								FastFinalityFee:    lockrelease_token_pool.UsdCents{V0: tokenPoolV2FastFinalityFeeUSDCents},
								FinalityBpsFee:     TokenPoolFinalityBpsFee,
								FastFinalityBpsFee: tokenPoolV2FastFinalityBpsFee,
								IsEnabled:          true,
							},
						},
						statePDA, chainConfigV2PDA, authority, solana.SystemProgramID, mint, feeVault, feeVaultATA,
						tokenProgram, solana.SPLAssociatedTokenAccountProgramID, eventAuthority, poolProgram,
					)
				default:
					continue
				}
				if err != nil {
					return fmt.Errorf("build init_chain_config_v2 for %s qualifier %q remote %d: %w", poolType, ref.Qualifier, remoteSelector, err)
				}
				if err = solChain.Confirm([]solana.Instruction{ixn}); err != nil {
					return fmt.Errorf("init_chain_config_v2 for %s qualifier %q remote %d: %w", poolType, ref.Qualifier, remoteSelector, err)
				}
			}
		}
	}

	refs, err := env.DataStore.Addresses().Fetch()
	if err != nil {
		return fmt.Errorf("fetch datastore addresses: %w", err)
	}
	report, err := operations.ExecuteSequence(env.OperationsBundle, sequences.ExtendCcipSendLut, env.BlockChains, sequences.ExtendLutInput{
		ChainSelector:     selector,
		ExistingAddresses: refs,
	})
	if err != nil {
		return fmt.Errorf("extend ccipSend lookup table: %w", err)
	}
	if len(report.Output.BatchOps) > 0 {
		return errors.New("extend ccipSend lookup table: router update needs an MCMS proposal, devenv expects a deployer-owned router")
	}
	return nil
}

// ensureV1FeeQuoterDestChains creates the fee quoter's v1 [dest_chain] account for each
// remote, and seeds its gas price. The 2.0 send path prices PoolV1 token transfers from the
// v1 dest chain config (GetFallbackTokenTransferFeeV2) and reads the lane's gas price from
// the v1 dest chain state (quote_gas_for_exec_v2), both of which real environments already
// have because their parallel 1.6 lanes keep them configured and their 1.6 price updater
// keeps the price fresh. This devenv only ever deploys 2.0, so nothing else creates the
// account (Solana-source token transfers fail with AccountNotInitialized) and no price
// updater writes to it (2.0 fee quotes fail with PriceNeverSet).
func ensureV1FeeQuoterDestChains(env *deployment.Environment, selector uint64, remoteSelectors []uint64) error {
	solChain, ok := env.BlockChains.SolanaChains()[selector]
	if !ok {
		return fmt.Errorf("no Solana chain found for selector %d", selector)
	}
	feeQuoter, err := solcommon.GetDataStoreAddress(env.DataStore, datastore.NewAddressRefKey(
		selector, datastore.ContractType(fqops.ContractType), fqops.Version, "",
	))
	if err != nil {
		return fmt.Errorf("get fee quoter address: %w", err)
	}
	solcommon.SetProgramID(&fee_quoter.ProgramID, fee_quoter.ParseInstructionWithoutAccounts, feeQuoter)
	configPDA, _, err := solstate.FindFqConfigPDA(feeQuoter)
	if err != nil {
		return fmt.Errorf("derive fee quoter config PDA: %w", err)
	}

	for _, remoteSelector := range remoteSelectors {
		selectorLE := make([]byte, 8)
		binary.LittleEndian.PutUint64(selectorLE, remoteSelector)
		destChainPDA, _, err := solana.FindProgramAddress([][]byte{[]byte("dest_chain"), selectorLE}, feeQuoter)
		if err != nil {
			return fmt.Errorf("derive v1 dest chain PDA for remote %d: %w", remoteSelector, err)
		}
		// add_dest_chain is init-only, so skip creating remotes already configured. The gas
		// price still has to be seeded, since it is separate mutable state.
		if info, infoErr := solChain.Client.GetAccountInfo(context.Background(), destChainPDA); infoErr == nil && info != nil && info.Value != nil {
			if err = seedV1FeeQuoterGasPrice(solChain, feeQuoter, configPDA, remoteSelector, destChainPDA); err != nil {
				return err
			}
			continue
		}

		// Mirror the v2 config that ConfigureChainForLanes just wrote, so both versions
		// describe the same lane and no values are duplicated here.
		destChainV2PDA, _, err := solstate.FindFqDestChainV2PDA(remoteSelector, feeQuoter)
		if err != nil {
			return fmt.Errorf("derive v2 dest chain PDA for remote %d: %w", remoteSelector, err)
		}
		var destChainV2 fee_quoter.DestChainV2
		if err = contractscommon.GetAccountDataBorshIntoAnchor(
			context.Background(), solChain.Client, destChainV2PDA, solrpc.CommitmentConfirmed, &destChainV2,
		); err != nil {
			return fmt.Errorf("read v2 dest chain config for remote %d: %w", remoteSelector, err)
		}
		// add_dest_chain rejects a zero default tx gas limit.
		if destChainV2.Config.DefaultTxGasLimit.V0 == 0 {
			return fmt.Errorf("v2 dest chain config for remote %d has no default tx gas limit", remoteSelector)
		}

		ixn, err := fee_quoter.NewAddDestChainInstruction(
			remoteSelector,
			v1DestChainConfigFromV2(destChainV2.Config),
			configPDA,
			destChainPDA,
			solChain.DeployerKey.PublicKey(),
			solana.SystemProgramID,
		)
		if err != nil {
			return fmt.Errorf("build add dest chain instruction for remote %d: %w", remoteSelector, err)
		}
		if err = solChain.Confirm([]solana.Instruction{ixn}); err != nil {
			return fmt.Errorf("add v1 fee quoter dest chain for remote %d: %w", remoteSelector, err)
		}
		if err = seedV1FeeQuoterGasPrice(solChain, feeQuoter, configPDA, remoteSelector, destChainPDA); err != nil {
			return err
		}
	}
	return nil
}

// seedV1FeeQuoterGasPrice writes devenvUSDPerUnitGas to the v1 [dest_chain] account through
// update_prices, the instruction the offchain price updater uses. It is a no-op once a price
// is present, so a later real price update is never clobbered. The deployer is registered as
// an allowed price updater on first use: in this devenv it owns the fee quoter, and there is
// no offchain 1.6 price pipeline to write the price instead.
func seedV1FeeQuoterGasPrice(
	solChain cldf_solana.Chain,
	feeQuoter, configPDA solana.PublicKey,
	remoteSelector uint64,
	destChainPDA solana.PublicKey,
) error {
	var destChain fee_quoter.DestChain
	if err := contractscommon.GetAccountDataBorshIntoAnchor(
		context.Background(), solChain.Client, destChainPDA, solrpc.CommitmentConfirmed, &destChain,
	); err != nil {
		return fmt.Errorf("read v1 dest chain state for remote %d: %w", remoteSelector, err)
	}
	if destChain.State.UsdPerUnitGas.Timestamp != 0 {
		return nil
	}

	deployer := solChain.DeployerKey.PublicKey()
	priceUpdaterPDA, _, err := solstate.FindFqAllowedPriceUpdaterPDA(deployer, feeQuoter)
	if err != nil {
		return fmt.Errorf("derive allowed price updater PDA: %w", err)
	}
	if info, infoErr := solChain.Client.GetAccountInfo(context.Background(), priceUpdaterPDA); infoErr != nil || info == nil || info.Value == nil {
		ixn, addErr := fee_quoter.NewAddPriceUpdaterInstruction(
			deployer, priceUpdaterPDA, configPDA, deployer, solana.SystemProgramID,
		)
		if addErr != nil {
			return fmt.Errorf("build add price updater instruction: %w", addErr)
		}
		if addErr = solChain.Confirm([]solana.Instruction{ixn}); addErr != nil {
			return fmt.Errorf("add deployer as fee quoter price updater: %w", addErr)
		}
	}

	var packedPrice [28]uint8
	devenvUSDPerUnitGas.FillBytes(packedPrice[:])
	raw, err := fee_quoter.NewUpdatePricesInstruction(
		[]fee_quoter.TokenPriceUpdate{},
		[]fee_quoter.GasPriceUpdate{{DestChainSelector: remoteSelector, UsdPerUnitGas: packedPrice}},
		deployer,
		priceUpdaterPDA,
		configPDA,
	)
	if err != nil {
		return fmt.Errorf("build update prices instruction for remote %d: %w", remoteSelector, err)
	}
	// update_prices takes the accounts it writes as remaining accounts, in update order.
	generic, ok := raw.(*solana.GenericInstruction)
	if !ok {
		return fmt.Errorf("unexpected instruction type: %T", raw)
	}
	generic.AccountValues = append(generic.AccountValues, solana.Meta(destChainPDA).WRITE())

	if err = solChain.Confirm([]solana.Instruction{raw}); err != nil {
		return fmt.Errorf("seed v1 fee quoter gas price for remote %d: %w", remoteSelector, err)
	}
	return nil
}

// v1DestChainConfigFromV2 projects a v2 dest chain config onto the v1 shape. Only the
// default token fee fields are read by the 2.0 fallback fee path; the rest carry over so
// the v1 account describes the same lane and satisfies add_dest_chain's validation.
func v1DestChainConfigFromV2(cfg fee_quoter.DestChainConfigV2) fee_quoter.DestChainConfig {
	return fee_quoter.DestChainConfig{
		IsEnabled:                   cfg.IsEnabled,
		LaneCodeVersion:             fee_quoter.CodeVersion_V1,
		MaxNumberOfTokensPerMsg:     1,
		MaxDataBytes:                cfg.MaxDataBytes,
		MaxPerMsgGasLimit:           cfg.MaxPerMsgGasLimit,
		DestGasOverhead:             cfg.DestGasOverhead,
		DestGasPerPayloadByteBase:   fee_quoter.CrossChainGas{V0: cfg.DestGasPerPayloadByteBase},
		DefaultTokenFeeUsdcents:     cfg.DefaultTokenFeeUsdCents,
		DefaultTokenDestGasOverhead: cfg.DefaultTokenDestGasOverhead,
		DefaultTxGasLimit:           cfg.DefaultTxGasLimit,
		NetworkFee:                  cfg.NetworkFee,
		ChainFamilySelector:         cfg.ChainFamilySelector,
	}
}
