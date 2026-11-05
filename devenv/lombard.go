package devenv

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"

	"github.com/Masterminds/semver/v3"
	"github.com/gagliardetto/solana-go"

	chainsel "github.com/smartcontractkit/chain-selectors"
	tokenapi "github.com/smartcontractkit/chainlink-ccip/deployment/tokens"
	changesetscore "github.com/smartcontractkit/chainlink-ccip/deployment/utils/changesets"
	ccipadapters "github.com/smartcontractkit/chainlink-ccip/deployment/v2_0_0/adapters"
	ccipchangesets "github.com/smartcontractkit/chainlink-ccip/deployment/v2_0_0/changesets"
	devenvcommon "github.com/smartcontractkit/chainlink-ccv/build/devenv/common"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	"github.com/smartcontractkit/chainlink-deployments-framework/deployment"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/mock_lombard_bridge"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/mock_lombard_mailbox"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/tokens"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	solanaadapters "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/adapters"
	fqops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/feequoter"
	lvops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/lombardverifier"
	tokenpoolops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/tokenpools"
	solseq "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/sequences"
)

func init() {
	devenvcommon.GlobalLombardRegistry.RegisterLombardChain(chainsel.FamilySolana, &solanaadapters.LombardChainAdapter{})
}

// evmLombardTokenPoolQualifier mirrors chainlink-ccip/chains/evm/deployment/v2_0_0/sequences/
// lombard's tokenPoolQualifier helper (ContractQualifier + "_" + tokenQualifier, i.e.
// "Lombard_LBTC"): the qualifier the EVM-side LombardTokenPool/AdvancedPoolHooks are actually
// registered under, distinct from the plain LombardTokenQualifier ("LBTC") Solana uses.
const evmLombardTokenPoolQualifier = "Lombard_" + LombardTokenQualifier

// devenv-only placeholder values
const (
	lombardLaneFeeUSDCents        = 45
	lombardLaneGasForVerification = 7_500*6 + 350_000
	lombardLanePayloadSizeBytes   = 6*64 + 2*32
)

// configureLombardForTransfer configures the Solana side of the Lombard lane for all supported
// remote chains (called from PostConnect, after every chain's deploy phase has run).
func configureLombardForTransfer(env *deployment.Environment, selector uint64, remoteSelectors []uint64) error {
	remoteSelectors = devenvcommon.FilterLombardSupportedSelectors(remoteSelectors)
	if len(remoteSelectors) == 0 {
		return nil
	}

	tokenRef, err := solcommon.PoolRefByQualifier(env.DataStore, selector, string(solcommon.SPLTokens), LombardTokenQualifier)
	if err != nil {
		return fmt.Errorf("resolve lombard token on chain %d: %w", selector, err)
	}

	sealedDS := env.DataStore

	remoteChainAdapters := make(map[uint64]ccipadapters.RemoteLombardChain)
	remoteChains := make(map[uint64]ccipadapters.RemoteLombardChainConfig)
	for _, rs := range remoteSelectors {
		var family string
		family, err = chainsel.GetSelectorFamily(rs)
		if err != nil {
			return fmt.Errorf("get selector family for remote %d: %w", rs, err)
		}
		remoteAdapter, ok := devenvcommon.GlobalLombardRegistry.GetLombardChain(family)
		if !ok {
			return fmt.Errorf("no Lombard adapter for remote chain family %q (selector %d)", family, rs)
		}
		remoteChainAdapters[rs] = remoteAdapter

		var remoteBridgeRef datastore.AddressRef
		remoteBridgeRef, err = env.DataStore.Addresses().Get(datastore.NewAddressRefKey(
			rs, datastore.ContractType(MockLombardBridgeContractType), semver.MustParse("2.0.0"), devenvcommon.LombardContractsQualifier,
		))
		if err != nil {
			return fmt.Errorf("resolve lombard bridge on remote chain %d: %w", rs, err)
		}
		var bridgeBytes []byte
		bridgeBytes, err = remoteAdapter.AddressRefToBytes(remoteBridgeRef)
		if err != nil {
			return fmt.Errorf("encode remote bridge address for chain %d: %w", rs, err)
		}

		remoteChains[rs] = ccipadapters.RemoteLombardChainConfig{
			// The canonical ConfigureLombardChainForLanes sequence owns the pool ChainConfigV2
			// (SetChainConfigV2Lombard), so the fee schedule travels here instead of being written
			// separately by the devenv mock-bridge helper.
			TokenTransferFeeConfig: tokenapi.TokenTransferFeeConfig{
				DestGasOverhead:               tokenPoolV2DestGasOverhead,
				DestBytesOverhead:             tokenPoolV2DestBytesOverhead,
				DefaultFinalityFeeUSDCents:    tokenPoolV2FinalityFeeUSDCents,
				CustomFinalityFeeUSDCents:     tokenPoolV2FastFinalityFeeUSDCents,
				DefaultFinalityTransferFeeBps: 0,
				CustomFinalityTransferFeeBps:  0,
				IsEnabled:                     true,
			},
			FeeUSDCents:        lombardLaneFeeUSDCents,
			GasForVerification: lombardLaneGasForVerification,
			PayloadSizeBytes:   lombardLanePayloadSizeBytes,
			LombardChainId:     uint32(rs), //nolint:gosec // deliberate low-32-bit truncation of the uint64 selector
			RemoteBridgeSender: "0x" + hex.EncodeToString(bridgeBytes),
		}
	}

	solAdapter := &solanaadapters.LombardChainAdapter{}
	if _, err = operations.ExecuteSequence(env.OperationsBundle, solAdapter.ConfigureLombardChainForLanes(), ccipadapters.ConfigureLombardChainForLanesDeps{
		BlockChains:  env.BlockChains,
		DataStore:    sealedDS,
		RemoteChains: remoteChainAdapters,
	}, ccipadapters.ConfigureLombardChainForLanesInput{
		ChainSelector:  selector,
		Token:          tokenRef.Address,
		TokenQualifier: evmLombardTokenPoolQualifier,
		RemoteChains:   remoteChains,
	}); err != nil {
		return fmt.Errorf("configure lombard lanes on solana chain %d: %w", selector, err)
	}

	for _, rs := range remoteSelectors {
		if err = configureLombardMockBridgeForRemote(env, selector, rs); err != nil {
			return fmt.Errorf("configure mock bridge for remote %d: %w", rs, err)
		}
	}

	return nil
}

// configureLombardMockBridgeForRemote creates the Solana mock bridge's remote_bridge_config and
// remote_token_config PDAs for one remote chain (the test_setup_* instructions exist only on the
// mock bridge). The canonical ConfigureLombardChainForLanes sequence already owns the pool
// ChainConfigV2 and this remote chain's verifier lookup table, so this helper does not re-init
// them; the lookup table extensions below are idempotent top-ups for the mock bridge's accounts.
func configureLombardMockBridgeForRemote(env *deployment.Environment, selector uint64, remoteSelector uint64) error {
	solChain, ok := env.BlockChains.SolanaChains()[selector]
	if !ok {
		return fmt.Errorf("solana chain not found for selector %d", selector)
	}
	solBridgeRef, err := env.DataStore.Addresses().Get(datastore.NewAddressRefKey(
		selector, datastore.ContractType(solcommon.LombardBridgeV2ContractType), solcommon.LombardChainContractsVersion, "",
	))
	if err != nil {
		return fmt.Errorf("resolve mock-lombard-bridge on chain %d: %w", selector, err)
	}
	solBridgeAddr, err := solana.PublicKeyFromBase58(solBridgeRef.Address)
	if err != nil {
		return fmt.Errorf("parse solana bridge address %q: %w", solBridgeRef.Address, err)
	}

	family, err := chainsel.GetSelectorFamily(remoteSelector)
	if err != nil {
		return fmt.Errorf("get selector family for remote %d: %w", remoteSelector, err)
	}
	remoteAdapter, ok := devenvcommon.GlobalLombardRegistry.GetLombardChain(family)
	if !ok {
		return fmt.Errorf("no Lombard adapter for remote chain family %q (selector %d)", family, remoteSelector)
	}

	remoteBridgeRef, err := env.DataStore.Addresses().Get(datastore.NewAddressRefKey(
		remoteSelector, datastore.ContractType(MockLombardBridgeContractType), semver.MustParse("2.0.0"), devenvcommon.LombardContractsQualifier,
	))
	if err != nil {
		return fmt.Errorf("resolve lombard bridge on remote chain %d: %w", remoteSelector, err)
	}
	remoteBridgeBytes, err := remoteAdapter.AddressRefToBytes(remoteBridgeRef)
	if err != nil {
		return fmt.Errorf("encode remote bridge address for chain %d: %w", remoteSelector, err)
	}
	remoteBridgePadded, err := solcommon.LeftPadTo32Bytes(remoteBridgeBytes)
	if err != nil {
		return fmt.Errorf("remote bridge address for chain %d: %w", remoteSelector, err)
	}
	remoteBridgeBytes32 := [32]byte(remoteBridgePadded)

	remoteTokenBytes, err := remoteAdapter.RemoteTokenAddress(
		operations.NewBundle(func() context.Context { return env.OperationsBundle.GetContext() }, env.Logger, operations.NewMemoryReporter()),
		env.DataStore, env.BlockChains, remoteSelector, evmLombardTokenPoolQualifier,
	)
	if err != nil {
		return fmt.Errorf("resolve remote token address for chain %d: %w", remoteSelector, err)
	}
	remoteTokenPadded, err := solcommon.LeftPadTo32Bytes(remoteTokenBytes)
	if err != nil {
		return fmt.Errorf("remote token address for chain %d: %w", remoteSelector, err)
	}
	remoteTokenBytes32 := [32]byte(remoteTokenPadded)

	tokenRef, err := solcommon.PoolRefByQualifier(env.DataStore, selector, string(solcommon.SPLTokens), LombardTokenQualifier)
	if err != nil {
		return fmt.Errorf("resolve lombard token on chain %d: %w", selector, err)
	}
	solMintPubkey, err := solana.PublicKeyFromBase58(tokenRef.Address)
	if err != nil {
		return fmt.Errorf("parse solana mint address %q: %w", tokenRef.Address, err)
	}

	remoteLChainID := paddedLombardChainID(uint32(remoteSelector)) //nolint:gosec // deliberate low-32-bit truncation of the uint64 selector
	remoteBridgeConfigPDA, _, err := solana.FindProgramAddress(
		[][]byte{[]byte("remote_bridge_config"), remoteLChainID[:]}, solBridgeAddr,
	)
	if err != nil {
		return fmt.Errorf("derive remote bridge config PDA: %w", err)
	}
	setupBridgeConfigIx, err := mock_lombard_bridge.NewTestSetupRemoteBridgeConfigInstruction(
		remoteLChainID, remoteBridgeBytes32,
		solChain.DeployerKey.PublicKey(), remoteBridgeConfigPDA, solana.SystemProgramID,
	)
	if err != nil {
		return fmt.Errorf("build test_setup_remote_bridge_config instruction: %w", err)
	}
	if err = solChain.Confirm([]solana.Instruction{setupBridgeConfigIx}); err != nil {
		return fmt.Errorf("confirm test_setup_remote_bridge_config: %w", err)
	}

	remoteTokenConfigPDA, _, err := solana.FindProgramAddress(
		[][]byte{[]byte("remote_token_config"), solMintPubkey.Bytes(), remoteLChainID[:]}, solBridgeAddr,
	)
	if err != nil {
		return fmt.Errorf("derive remote token config PDA: %w", err)
	}
	const outboundDirection = 1 // matches the real bridge's OUTBOUND_DIRECTION; unenforced by the mock
	setupTokenConfigIx, err := mock_lombard_bridge.NewTestSetupRemoteTokenConfigInstruction(
		solMintPubkey, remoteLChainID, remoteTokenBytes32, outboundDirection,
		solChain.DeployerKey.PublicKey(), remoteTokenConfigPDA, solana.SystemProgramID,
	)
	if err != nil {
		return fmt.Errorf("build test_setup_remote_token_config instruction: %w", err)
	}
	if err = solChain.Confirm([]solana.Instruction{setupTokenConfigIx}); err != nil {
		return fmt.Errorf("confirm test_setup_remote_token_config: %w", err)
	}

	// Addresses needed for extending this remote chain's own verifier lookup table below, once
	// ConfigureLombardChainForLanes has created it.
	poolRef, err := env.DataStore.Addresses().Get(datastore.NewAddressRefKey(
		selector, datastore.ContractType(tokenpoolops.LombardTokenPool), solcommon.Version2_0_0, solcommon.PoolProgramQualifier,
	))
	if err != nil {
		return fmt.Errorf("resolve lombard token pool on chain %d: %w", selector, err)
	}
	poolAddr, err := solana.PublicKeyFromBase58(poolRef.Address)
	if err != nil {
		return fmt.Errorf("parse lombard token pool address %q: %w", poolRef.Address, err)
	}
	mailboxRef, err := env.DataStore.Addresses().Get(datastore.NewAddressRefKey(
		selector, datastore.ContractType(solcommon.LombardMailboxContractType), solcommon.LombardChainContractsVersion, "",
	))
	if err != nil {
		return fmt.Errorf("resolve mock-lombard-mailbox on chain %d: %w", selector, err)
	}
	mailboxAddr, err := solana.PublicKeyFromBase58(mailboxRef.Address)
	if err != nil {
		return fmt.Errorf("parse mock-lombard-mailbox address %q: %w", mailboxRef.Address, err)
	}

	// Not Lombard-specific but several per remote chain accounts that will be used in Lombard ccipSends.
	routerAddr, rErr := routerPubkeyFromDataStore(env.DataStore, selector, "")
	if rErr != nil {
		return fmt.Errorf("resolve router on chain %d: %w", selector, rErr)
	}

	feeQuoterAddr, fqErr := solcommon.PubkeyFromRefs(env.DataStore.Addresses().Filter(datastore.AddressRefByChainSelector(selector)),
		datastore.NewAddressRefKey(selector, datastore.ContractType(fqops.ContractType), fqops.Version, ""),
	)
	if fqErr != nil {
		return fmt.Errorf("resolve fee quoter on chain %d: %w", selector, fqErr)
	}
	cvProgramID, _ := committeeVerifierPubkeyFromDataStore(env.DataStore, selector, "")
	remoteLUTEntries, err := solseq.CcipSendRemoteLutEntries(remoteSelector, routerAddr, feeQuoterAddr, solMintPubkey, cvProgramID)
	if err != nil {
		return fmt.Errorf("compute ccip_send lookup table entries for remote %d: %w", remoteSelector, err)
	}
	if _, err = operations.ExecuteOperation(env.OperationsBundle, lvops.ExtendLookupTableForLane, solChain, lvops.ExtendLookupTableForLaneParams{
		LombardVerifier:     poolAddr,
		Bridge:              solBridgeAddr,
		Mailbox:             mailboxAddr,
		Router:              routerAddr,
		Mint:                solMintPubkey,
		RemoteChainSelector: remoteSelector,
		LChainID:            remoteLChainID,
	}); err != nil {
		return fmt.Errorf("extend lombard verifier lookup table with remote bridge/token config: %w", err)
	}

	if _, err = operations.ExecuteOperation(env.OperationsBundle, lvops.ExtendLookupTable, solChain, lvops.ExtendLookupTableParams{
		LombardVerifier:     poolAddr,
		RemoteChainSelector: remoteSelector,
		Entries:             remoteLUTEntries,
	}); err != nil {
		return fmt.Errorf("extend lombard verifier lookup table with generic per-remote-chain accounts: %w", err)
	}

	return nil
}

// paddedLombardChainID matches the identical helper in chainlink-ccv/build/devenv/evm/lombard.go
// and this repo's own deployment/v2_0_0/sequences/configure_lombard_chain_for_lanes.go: a
// big-endian uint32, left-padded with zeros to 32 bytes (value occupies the low 4 bytes).
func paddedLombardChainID(lombardChainID uint32) [32]byte {
	var out [32]byte
	binary.BigEndian.PutUint32(out[28:], lombardChainID)
	return out
}

const LombardTokenQualifier = "LBTC"

// LombardSolanaMintDecimals is the decimals of the Solana-side Lombard test mint deployed below
// (deliberately different from the generic devenv SolanaDefaultDecimals=6, and from the EVM
// LBTC test token's 18 decimals, matching a real cross-chain wrapped asset where the two chains'
// tokens don't share a decimals convention).
const LombardSolanaMintDecimals = 9

// MockLombardBridgeContractType is the datastore type of the EVM MockLombardBridge deployed by
// chainlink-ccv. It is resolved only for remote (EVM) chains; the Solana mock bridge is
// registered under solcommon.LombardBridgeV2ContractType.
const MockLombardBridgeContractType = "MockLombardBridge"

// deployLombardTokenAndPool is the Solana counterpart to EVM's deployLombardTokenAndPool
// (chainlink-ccv/build/devenv/evm/lombard.go): create a devenv-only test mint, then deploy and
// initialize the Lombard token pool + embedded verifier via the upstream Lombard changeset
func (c *Chain) deployLombardTokenAndPool(ctx context.Context, env *deployment.Environment, ds *datastore.MemoryDataStore, selector uint64) error {
	solChain, ok := env.BlockChains.SolanaChains()[selector]
	if !ok {
		return fmt.Errorf("solana chain not found for selector %d", selector)
	}

	bundle := operations.NewBundle(func() context.Context { return ctx }, env.Logger, operations.NewMemoryReporter())

	// devenv-only Lombard-wrapped test mint, pre-minted to the deployer so later burn/mint e2e
	// steps have a funded source account.
	preMint := uint64(1_000_000) * 1_000_000_000 // 1,000,000 tokens @ 9 decimals
	deployTokenOut, err := operations.ExecuteSequence(bundle, (&solseq.SolanaAdapter{}).DeployToken(), env.BlockChains, tokenapi.DeployTokenInput{
		Name:              "Lombard Staked Bitcoin",
		Symbol:            LombardTokenQualifier,
		Decimals:          LombardSolanaMintDecimals,
		PreMint:           &preMint,
		Senders:           []string{solChain.DeployerKey.PublicKey().String()},
		Type:              solcommon.SPLTokens,
		ChainSelector:     selector,
		ExistingDataStore: ds.Seal(),
		// The devenv deploys no MCMS, so there is no timelock to hold the freeze authority
		// (see the TODO in tokens.go GetTokenExpansionConfigs); revoke it instead of asking
		// DeploySolanaToken for a timelock signer PDA that this environment never writes.
		DisableFreezeAuthority: true,
	})
	if err != nil {
		return fmt.Errorf("failed to deploy lombard test token on chain %d: %w", selector, err)
	}
	for _, addr := range deployTokenOut.Output.Addresses {
		if err = ds.Addresses().Add(addr); err != nil {
			return fmt.Errorf("failed to store lombard token address on chain %d: %w", selector, err)
		}
	}
	tokenRef, err := solcommon.PoolRefByQualifier(ds.Seal(), selector, string(solcommon.SPLTokens), LombardTokenQualifier)
	if err != nil {
		return fmt.Errorf("failed to resolve lombard token on chain %d: %w", selector, err)
	}
	if err = ds.Addresses().Add(datastore.AddressRef{
		ChainSelector: selector,
		Type:          tokenRef.Type,
		Version:       tokenRef.Version,
		Address:       tokenRef.Address,
		Qualifier:     evmLombardTokenPoolQualifier,
	}); err != nil {
		return fmt.Errorf("failed to alias lombard token for evm qualifier lookups on chain %d: %w", selector, err)
	}

	bridgeRef, err := ds.Seal().Addresses().Get(datastore.NewAddressRefKey(
		selector, datastore.ContractType(solcommon.LombardBridgeV2ContractType), solcommon.LombardChainContractsVersion, "",
	))
	if err != nil {
		return fmt.Errorf("failed to resolve mock-lombard-bridge on chain %d: %w", selector, err)
	}

	// mock-lombard-bridge's mock_gmp_receive mints via a PDA it signs for itself
	// (TOKEN_AUTHORITY_SEED = "token_authority", see mock-lombard-bridge/src/lib.rs) - the mint's
	// real SPL mint_authority must already be that PDA for the mint_to CPI inside verify()'s
	// mailbox chain to succeed. DeployToken above left it on the deployer key (its own default),
	// so it must be handed over here before any message can actually be verified/minted.
	bridgeAddr, err := solana.PublicKeyFromBase58(bridgeRef.Address)
	if err != nil {
		return fmt.Errorf("parse mock-lombard-bridge address %q: %w", bridgeRef.Address, err)
	}
	tokenAuthorityPDA, _, err := solana.FindProgramAddress([][]byte{[]byte("token_authority")}, bridgeAddr)
	if err != nil {
		return fmt.Errorf("derive mock-lombard-bridge token authority PDA: %w", err)
	}
	mintAddr, err := solana.PublicKeyFromBase58(tokenRef.Address)
	if err != nil {
		return fmt.Errorf("parse lombard token address %q: %w", tokenRef.Address, err)
	}
	setAuthorityIx, err := tokens.SetTokenMintAuthority(
		solana.TokenProgramID, tokenAuthorityPDA, mintAddr, solChain.DeployerKey.PublicKey(),
	)
	if err != nil {
		return fmt.Errorf("build set mint authority instruction: %w", err)
	}
	if err = solChain.Confirm([]solana.Instruction{setAuthorityIx}); err != nil {
		return fmt.Errorf("transfer lombard mint authority to bridge token authority PDA: %w", err)
	}

	mcmsRegistry := changesetscore.GetRegistry()
	out, err := ccipchangesets.DeployLombardChains(devenvcommon.GlobalLombardRegistry, mcmsRegistry).Apply(*env, ccipchangesets.DeployLombardChainsConfig{
		Chains: map[uint64]ccipchangesets.LombardChainConfig{
			selector: {
				Bridge:           bridgeRef.Address,
				Token:            tokenRef.Address,
				TokenQualifier:   LombardTokenQualifier,
				StorageLocations: []string{"https://test.chain.link.fake"},
				RateLimitAdmin:   solChain.DeployerKey.PublicKey().String(),
				FeeAggregator:    solChain.DeployerKey.PublicKey().String(),
			},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to deploy lombard chain on chain %d: %w", selector, err)
	}
	if err = ds.Merge(out.DataStore.Seal()); err != nil {
		return fmt.Errorf("failed to merge lombard deploy output on chain %d: %w", selector, err)
	}

	// mock-lombard-mailbox's global Config account (global_nonce, treasury, etc.) - read by
	// start_verification_at_source's resolution (stage 2 -> 3, see
	// lombard-token-pool/src/instructions/verifier/resolution.rs) to derive the outbound_message
	// PDA from the current nonce. Unlike the bridge's remote_bridge_config/remote_token_config
	// (per-remote-chain, set up in configureLombardMockBridgeForRemote), this is a single chain-global
	// PDA, so it belongs here rather than being repeated per remote lane. test_setup_config is the
	// only instruction that creates it; nothing else does.
	mailboxRef, err := ds.Seal().Addresses().Get(datastore.NewAddressRefKey(
		selector, datastore.ContractType(solcommon.LombardMailboxContractType), solcommon.LombardChainContractsVersion, "",
	))
	if err != nil {
		return fmt.Errorf("failed to resolve mock-lombard-mailbox on chain %d: %w", selector, err)
	}
	mailboxAddr, err := solana.PublicKeyFromBase58(mailboxRef.Address)
	if err != nil {
		return fmt.Errorf("parse mock-lombard-mailbox address %q: %w", mailboxRef.Address, err)
	}
	mailboxConfigPDA, _, err := solana.FindProgramAddress([][]byte{[]byte("mailbox_config")}, mailboxAddr)
	if err != nil {
		return fmt.Errorf("derive mock-lombard-mailbox config PDA: %w", err)
	}
	treasury := solana.NewWallet().PublicKey()
	setupMailboxConfigIx, err := mock_lombard_mailbox.NewTestSetupConfigInstruction(
		0, treasury, solChain.DeployerKey.PublicKey(), mailboxConfigPDA, solana.SystemProgramID,
	)
	if err != nil {
		return fmt.Errorf("build test_setup_config instruction: %w", err)
	}
	if err = solChain.Confirm([]solana.Instruction{setupMailboxConfigIx}); err != nil {
		return fmt.Errorf("confirm test_setup_config: %w", err)
	}

	return nil
}
