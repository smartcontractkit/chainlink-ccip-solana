package sequences

import (
	"errors"
	"fmt"
	"math/big"
	"slices"
	"strconv"

	"github.com/Masterminds/semver/v3"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	chain_selectors "github.com/smartcontractkit/chain-selectors"
	ccip_tokens "github.com/smartcontractkit/chainlink-ccip/deployment/tokens"
	common_utils "github.com/smartcontractkit/chainlink-ccip/deployment/utils"
	seq_core "github.com/smartcontractkit/chainlink-ccip/deployment/utils/sequences"
	ccipadapters "github.com/smartcontractkit/chainlink-ccip/deployment/v2_0_0/adapters"
	cctpconfig "github.com/smartcontractkit/chainlink-ccip/deployment/v2_0_0/config"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	cldf_ops "github.com/smartcontractkit/chainlink-deployments-framework/operations"
	mcms_types "github.com/smartcontractkit/mcms/types"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/cctp_token_pool"
	utilscctp "github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/cctp"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/tokens"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	cctpverifierops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/cctpverifier"
	cvops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/committeeverifier"
	fqops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/feequoter"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/lut"
	routerops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/router"
	tokenpoolops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/tokenpools"
)

const (
	// cctpV1Mechanism and cctpV2WithCCVMechanism are the LockOrBurnMechanism values a Solana lane
	// accepts. The string only selects which pool an EVM USDCTokenPoolProxy dispatches to; on
	// Solana it is a pure gate, because this sequence writes the pool-side (v1) config and the
	// verifier-side (v2) config unconditionally, and resolves both allowed-caller variants from
	// the remote adapter either way.
	//
	// cctpV2WithCCVMechanism is what the CCIP 2.0 / CCV path uses and what the devenv configures.
	// cctpV1Mechanism stays accepted for callers that still route through upstream's
	// DeployCCTPChains changeset, whose validation hard-rejects anything else on a Solana lane.
	cctpV1Mechanism        = "CCTP_V1"
	cctpV2WithCCVMechanism = "CCTP_V2_WITH_CCV"

	// usdcDecimals is the decimal precision of USDC on every CCTP-supported chain.
	// RemoteCCTPChainConfig carries no remote-decimals field, and CCTP only ever moves USDC.
	usdcDecimals uint8 = 6

	// evmAddressLength is the width of an EVM address. Circle stores remote token addresses
	// left-padded to 32 bytes, but the verifier's supported-token pairs hold the unpadded form.
	evmAddressLength = 20
)

// ConfigureCCTPChainForLanes wires a Solana CCTP token pool and its embedded verifier to a set of
// remote chains.
//
// Per lane the order is load-bearing:
//
//  1. SetChainConfig creates the verifier's RemoteChainConfig PDA.
//  2. The lane's address lookup table is created and populated.
//  3. SetChainConfigCctp reallocs that PDA with the Circle domain, allowed callers and the ALT.
//  4. The pool-side ChainConfig, rate limits and v2 lane config follow.
var ConfigureCCTPChainForLanes = cldf_ops.NewSequence(
	"configure-cctp-chain-for-lanes",
	semver.MustParse("2.0.0"),
	"Configures the Solana CCTP token pool and verifier for a set of remote chains",
	func(b cldf_ops.Bundle, deps ccipadapters.ConfigureCCTPChainForLanesDeps, input ccipadapters.ConfigureCCTPChainForLanesInput) (seq_core.OnChainOutput, error) {
		solChain, ok := deps.BlockChains.SolanaChains()[input.ChainSelector]
		if !ok {
			return seq_core.OnChainOutput{}, fmt.Errorf("solana chain with selector %d not found", input.ChainSelector)
		}
		if deps.DataStore == nil {
			return seq_core.OnChainOutput{}, errors.New("datastore is required")
		}
		mint, err := solana.PublicKeyFromBase58(input.USDCToken)
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("parse USDC token %q: %w", input.USDCToken, err)
		}

		existing := deps.DataStore.Addresses().Filter(datastore.AddressRefByChainSelector(input.ChainSelector))
		poolAddr, err := solcommon.PubkeyFromRefs(existing, datastore.NewAddressRefKey(
			input.ChainSelector,
			datastore.ContractType(common_utils.CCTPTokenPool),
			solcommon.Version2_0_0,
			solcommon.PoolProgramQualifier,
		))
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("find cctp token pool on chain %d: %w", input.ChainSelector, err)
		}
		router, err := refPubkey(existing, input.ChainSelector, routerops.ContractType, routerops.Version)
		if err != nil {
			return seq_core.OnChainOutput{}, err
		}
		feeQuoter, err := refPubkey(existing, input.ChainSelector, fqops.ContractType, fqops.Version)
		if err != nil {
			return seq_core.OnChainOutput{}, err
		}
		// Optional: a chain without a committee verifier simply contributes no CV entry to the
		// lane table, which only costs transaction size.
		committeeVerifier, cvErr := refPubkey(existing, input.ChainSelector, cvops.ContractType, cvops.Version)
		if cvErr != nil {
			committeeVerifier = solana.PublicKey{}
		}

		// These are properties of this chain, not of any single lane, so derive them once.
		poolSigner, err := tokens.TokenPoolSignerAddress(mint, poolAddr)
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("derive pool signer address: %w", err)
		}
		poolTokenAccount, _, err := tokens.FindAssociatedTokenAddress(solana.TokenProgramID, mint, poolSigner)
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("derive pool token account: %w", err)
		}
		verifierConfigPDA, _, err := state.FindCVConfigPDA(poolAddr)
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("derive verifier config PDA: %w", err)
		}

		var (
			addresses []datastore.AddressRef
			batchOps  []mcms_types.BatchOperation
		)
		collect := func(out seq_core.OnChainOutput) {
			addresses = append(addresses, out.Addresses...)
			batchOps = append(batchOps, out.BatchOps...)
		}

		for _, remoteSelector := range sortedSelectors(input.RemoteChains) {
			remoteCfg := input.RemoteChains[remoteSelector]
			remoteAdapter, found := deps.RemoteChains[remoteSelector]
			if !found {
				return seq_core.OnChainOutput{}, fmt.Errorf("no CCTP adapter provided for remote chain %d", remoteSelector)
			}
			// Reject an unrecognised mechanism up front so a typo fails with a clear message
			// rather than silently writing a lane the counterpart chain will route elsewhere.
			switch remoteCfg.LockOrBurnMechanism {
			case cctpV1Mechanism, cctpV2WithCCVMechanism:
			default:
				return seq_core.OnChainOutput{}, fmt.Errorf(
					"solana lanes only support %s or %s, got %q for remote chain %d",
					cctpV2WithCCVMechanism, cctpV1Mechanism, remoteCfg.LockOrBurnMechanism, remoteSelector)
			}

			remoteToken, err := remoteAdapter.TokenAddress(deps.DataStore, deps.BlockChains, remoteSelector)
			if err != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("resolve token address on remote chain %d: %w", remoteSelector, err)
			}
			remotePool, err := remoteAdapter.PoolAddress(deps.DataStore, deps.BlockChains, remoteSelector, deps.RemoteRegisteredPoolRefs[remoteSelector])
			if err != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("resolve pool address on remote chain %d: %w", remoteSelector, err)
			}
			allowedCallerReceive, err := remoteAdapter.CCTPV2AllowedCallerOnDest(deps.DataStore, deps.BlockChains, remoteSelector)
			if err != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("resolve CCTP v2 allowed caller on remote chain %d: %w", remoteSelector, err)
			}
			allowedCallerDeposit, err := remoteAdapter.AllowedCallerOnSource(deps.DataStore, deps.BlockChains, remoteSelector)
			if err != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("resolve allowed caller on source for remote chain %d: %w", remoteSelector, err)
			}
			destinationCaller, err := remoteAdapter.CCTPV2AllowedCallerOnDest(deps.DataStore, deps.BlockChains, remoteSelector)
			if err != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("resolve CCTP v2 allowed caller on remote chain %d: %w", remoteSelector, err)
			}
			mintRecipient, err := remoteAdapter.MintRecipientOnDest(deps.DataStore, deps.BlockChains, remoteSelector)
			if err != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("resolve mint recipient on remote chain %d: %w", remoteSelector, err)
			}

			// The pool stores remote addresses left-padded to 32 bytes: an EVM onramp abi-encodes
			// them that way, and an unpadded value fails the source-pool check on every inbound
			// transfer.
			remoteToken32, err := solcommon.LeftPadTo32Bytes(remoteToken)
			if err != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("pad remote token address for chain %d: %w", remoteSelector, err)
			}
			remotePool32, err := solcommon.LeftPadTo32Bytes(remotePool)
			if err != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("pad remote pool address for chain %d: %w", remoteSelector, err)
			}
			allowedCallerReceiveKey, err := solcommon.BytesToPubkey(mustPad(allowedCallerReceive))
			if err != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("parse CCTP v2 allowed caller for chain %d: %w", remoteSelector, err)
			}
			allowedCallerDepositKey, err := solcommon.BytesToPubkey(mustPad(allowedCallerDeposit))
			if err != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("parse allowed caller on source for chain %d: %w", remoteSelector, err)
			}
			destinationCallerKey, err := solcommon.BytesToPubkey(mustPad(destinationCaller))
			if err != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("parse CCTP v2 allowed caller for chain %d: %w", remoteSelector, err)
			}
			// The pool and verifier reject zero callers: anyone could consume the CCTP message.
			if allowedCallerReceiveKey.IsZero() || allowedCallerDepositKey.IsZero() || destinationCallerKey.IsZero() {
				return seq_core.OnChainOutput{}, fmt.Errorf("CCTP allowed callers for remote chain %d must not be zero", remoteSelector)
			}
			// An empty mint recipient means CCTP mints straight to the message receiver.
			var mintRecipientKey *solana.PublicKey
			if len(mintRecipient) > 0 {
				key, perr := solcommon.BytesToPubkey(mustPad(mintRecipient))
				if perr != nil {
					return seq_core.OnChainOutput{}, fmt.Errorf("parse mint recipient for chain %d: %w", remoteSelector, perr)
				}
				mintRecipientKey = &key
			}

			// Step 1: create the verifier's RemoteChainConfig before anything reallocs it.
			chainConfigRep, err := cldf_ops.ExecuteOperation(b, cctpverifierops.SetChainConfig, solChain, cctpverifierops.SetChainConfigParams{
				CctpVerifier:        poolAddr,
				RemoteChainSelector: remoteSelector,
				RouterSigner:        router,
				FeeUSDCents:         uint32(remoteCfg.FeeUSDCents),
				GasForVerification:  remoteCfg.GasForVerification,
				PayloadSizeBytes:    uint32(remoteCfg.PayloadSizeBytes),
			})
			if err != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("set verifier chain config for remote chain %d: %w", remoteSelector, err)
			}
			collect(chainConfigRep.Output)

			// Step 2: the lane's lookup table, which the verifier's account resolvers echo back
			// so the offchain caller can compress the transaction.
			sendEntries, err := CcipSendRemoteLutEntries(remoteSelector, router, feeQuoter, mint, committeeVerifier)
			if err != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("compute ccip_send lookup table entries for remote chain %d: %w", remoteSelector, err)
			}
			laneALT, created, err := ensureLaneLookupTable(b, solChain, poolAddr, mint, remoteSelector, remoteCfg.DomainIdentifier,
				remoteToken32, poolSigner, poolTokenAccount, verifierConfigPDA, sendEntries)
			if err != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("set up lookup table for remote chain %d: %w", remoteSelector, err)
			}
			if created {
				addresses = append(addresses, datastore.AddressRef{
					Address:       laneALT.String(),
					ChainSelector: input.ChainSelector,
					Type:          datastore.ContractType(cctpverifierops.LookupTableType),
					Version:       cctpverifierops.Version,
					Qualifier:     strconv.FormatUint(remoteSelector, 10),
				})
			}

			// Step 3: the CCTP half of the verifier's remote chain config.
			// SupportedTokens holds the remote token in its native width, unlike the pool's
			// RemoteConfig which stores the padded form.
			cctpRep, err := cldf_ops.ExecuteOperation(b, cctpverifierops.SetChainConfigCctp, solChain, cctpverifierops.SetChainConfigCctpParams{
				CctpVerifier:         poolAddr,
				RemoteChainSelector:  remoteSelector,
				Enabled:              true,
				DomainIdentifier:     remoteCfg.DomainIdentifier,
				AllowedCallerReceive: allowedCallerReceiveKey,
				AllowedCallerDeposit: allowedCallerDepositKey,
				MintRecipientOnDest:  mintRecipientKey,
				LookupTable:          &laneALT,
				SupportedTokens: []cctp_token_pool.RemoteLocalPair{{
					Local:  mint,
					Remote: cctp_token_pool.RemoteAddress{Address: nativeRemoteTokenAddress(remoteSelector, remoteToken32)},
				}},
			})
			if err != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("set verifier chain cctp config for remote chain %d: %w", remoteSelector, err)
			}
			collect(cctpRep.Output)

			// Step 4: the pool-side lane config.
			poolRemoteCfg := tokenpoolops.CctpRemoteChainConfig{
				RemoteChainConfig: tokenpoolops.RemoteChainConfig{
					TokenPool:                 poolAddr,
					TokenMint:                 mint,
					TokenProgramID:            solana.TokenProgramID,
					RemoteSelector:            remoteSelector,
					RemoteTokenAddress:        remoteToken32,
					RemotePoolAddress:         remotePool32,
					RemoteDecimals:            usdcDecimals,
					InboundRateLimiterConfig:  scaleRateLimit(remoteCfg.InboundRateLimiterConfig),
					OutboundRateLimiterConfig: scaleRateLimit(remoteCfg.OutboundRateLimiterConfig),
				},
				DomainIdentifier:  remoteCfg.DomainIdentifier,
				DestinationCaller: destinationCallerKey,
			}
			upsertRep, err := cldf_ops.ExecuteOperation(b, tokenpoolops.UpsertRemoteChainConfigCctp, solChain, poolRemoteCfg)
			if err != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("upsert pool remote chain config for remote chain %d: %w", remoteSelector, err)
			}
			collect(upsertRep.Output)

			rateLimitRep, err := cldf_ops.ExecuteOperation(b, tokenpoolops.UpsertRateLimitsCctp, solChain, poolRemoteCfg)
			if err != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("set pool rate limits for remote chain %d: %w", remoteSelector, err)
			}
			collect(rateLimitRep.Output)

			feeCfg, err := resolveTransferFeeConfig(b, solChain, remoteCfg.TokenTransferFeeConfig, poolAddr, mint, remoteSelector)
			if err != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("resolve transfer fee config for remote chain %d: %w", remoteSelector, err)
			}

			// This same program implements the verifier interface, so it is its own CCV on both
			// directions of the lane.
			chainV2Rep, err := cldf_ops.ExecuteOperation(b, tokenpoolops.SetChainConfigV2Cctp, solChain, tokenpoolops.CctpChainConfigV2Params{
				TokenPool:           poolAddr,
				TokenMint:           mint,
				RemoteChainSelector: remoteSelector,
				// RemoteCCTPChainConfig carries no per-lane finality policy, so this is left
				// zero-valued: WAIT_FOR_FINALITY, which permits only full-finality outbound
				// transfers. Enabling fast finality on a lane needs a separate
				// set_chain_config_v2 call until upstream's input grows a finality field.
				AllowedFinalityConfig:  cctp_token_pool.FinalityConfig{},
				TokenTransferFeeConfig: feeCfg,
				CcvConfig: cctp_token_pool.CcvConfig{
					OutboundCcvs: []solana.PublicKey{poolAddr},
					InboundCcvs:  []solana.PublicKey{poolAddr},
				},
			})
			if err != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("set pool v2 chain config for remote chain %d: %w", remoteSelector, err)
			}
			collect(chainV2Rep.Output)
		}

		return seq_core.OnChainOutput{Addresses: addresses, BatchOps: batchOps}, nil
	},
)

// ensureLaneLookupTable returns the lane's address lookup table, reusing the one already recorded
// in the verifier's RemoteChainConfig when present and creating a fresh one otherwise. Either way
// the table is extended with the accounts the verifier's resolvers need, which is idempotent.
// The boolean reports whether a new table was created.
func ensureLaneLookupTable(
	b cldf_ops.Bundle,
	solChain cldf_solana.Chain,
	poolAddr, mint solana.PublicKey,
	remoteSelector uint64,
	domain uint32,
	remoteToken32 []byte,
	poolSigner, poolTokenAccount, verifierConfigPDA solana.PublicKey,
	sendEntries []solana.PublicKey,
) (solana.PublicKey, bool, error) {
	remoteChainConfigPDA, _, err := state.FindCctpVerifierRemoteChainConfigPDA(remoteSelector, poolAddr)
	if err != nil {
		return solana.PublicKey{}, false, fmt.Errorf("derive verifier remote chain config PDA: %w", err)
	}

	messageTransmitter, err := utilscctp.GetMessageTransmitterV2PDAs()
	if err != nil {
		return solana.PublicKey{}, false, fmt.Errorf("derive CCTP v2 message transmitter PDAs: %w", err)
	}
	tokenMessenger, err := utilscctp.GetTokenMessengerMinterV2PDAs(domain, mint, remoteToken32)
	if err != nil {
		return solana.PublicKey{}, false, fmt.Errorf("derive CCTP v2 token messenger minter PDAs: %w", err)
	}
	denylist, err := utilscctp.GetDenylistV2PDA(poolSigner)
	if err != nil {
		return solana.PublicKey{}, false, fmt.Errorf("derive CCTP v2 denylist PDA: %w", err)
	}

	// The trailing sendEntries are not read by the verifier's resolvers; they ride along because
	// the program only ever hands this table back as "an ALT to be used for compressing txs"
	// (verifier/state.rs), so the same table can also shrink the ccip_send_v2 transaction.
	entries := dedupeKeys(append([]solana.PublicKey{
		// Verifier accounts.
		verifierConfigPDA,
		remoteChainConfigPDA,
		solana.SystemProgramID,
		solana.TokenProgramID,
		mint,
		poolSigner,
		poolTokenAccount,
		// MessageTransmitter accounts.
		messageTransmitter.AuthorityPda,
		messageTransmitter.EventAuthority,
		messageTransmitter.MessageTransmitter,
		messageTransmitter.Program,
		// TokenMessengerMinter accounts. Its program appears twice in the resolved account list
		// (as the program and as the message receiver); one entry covers both.
		tokenMessenger.AuthorityPda,
		tokenMessenger.CustodyTokenAccount,
		tokenMessenger.EventAuthority,
		tokenMessenger.LocalToken,
		tokenMessenger.Program,
		tokenMessenger.RemoteTokenMessenger,
		tokenMessenger.TokenMessenger,
		tokenMessenger.TokenMinter,
		tokenMessenger.TokenPair,
		denylist,
	}, sendEntries...))

	var table solana.PublicKey
	created := false
	existing, err := common.GetInitializedAccount(b.GetContext(), solChain.Client, remoteChainConfigPDA, poolAddr, cldf_solana.SolDefaultCommitment, cctp_token_pool.ParseAccount_RemoteChainConfig)
	if err != nil {
		return solana.PublicKey{}, false, err
	}
	if existing != nil && existing.Cctp.LookupTable != nil && !existing.Cctp.LookupTable.IsZero() {
		table = *existing.Cctp.LookupTable
	} else {
		if table, err = common.CreateLookupTable(b.GetContext(), solChain.Client, *solChain.DeployerKey); err != nil {
			return solana.PublicKey{}, false, fmt.Errorf("create lookup table: %w", err)
		}
		if err = common.AwaitSlotChange(b.GetContext(), solChain.Client); err != nil {
			return solana.PublicKey{}, false, fmt.Errorf("await slot change after creating lookup table: %w", err)
		}
		created = true
	}

	// A Solana transaction caps how many 32-byte entries one extend can carry, so chunk.
	for start := 0; start < len(entries); start += lut.MaxEntriesPerExtend {
		end := min(start+lut.MaxEntriesPerExtend, len(entries))
		if err = solcommon.ExtendLookupTable(solChain, table, entries[start:end]); err != nil {
			return solana.PublicKey{}, false, fmt.Errorf("extend lookup table %s: %w", table, err)
		}
	}
	return table, created, nil
}

// nativeRemoteTokenAddress converts a padded remote token address back to the width the verifier's
// supported-token pairs expect. Circle and the pool store 32-byte padded addresses, but the pair
// check compares against the remote chain's native encoding - 20 bytes on EVM.
func nativeRemoteTokenAddress(remoteSelector uint64, padded []byte) []byte {
	family, err := chain_selectors.GetSelectorFamily(remoteSelector)
	if err != nil || family != chain_selectors.FamilyEVM || len(padded) != 32 {
		return padded
	}
	return padded[32-evmAddressLength:]
}

// mustPad left-pads an address to 32 bytes, leaving already-32-byte inputs alone. Errors are left
// to BytesToPubkey, which rejects anything that isn't 32 bytes wide.
func mustPad(addr []byte) []byte {
	padded, err := solcommon.LeftPadTo32Bytes(addr)
	if err != nil {
		return addr
	}
	return padded
}

// scaleRateLimit converts the user-facing float rate limiter input into raw token units. USDC has
// the same decimals on every chain, so the local and remote scaling agree.
func scaleRateLimit(cfg ccip_tokens.RateLimiterConfigFloatInput) ccip_tokens.RateLimiterConfig {
	if !cfg.IsEnabled {
		return ccip_tokens.RateLimiterConfig{Capacity: big.NewInt(0), Rate: big.NewInt(0)}
	}
	return ccip_tokens.RateLimiterConfig{
		IsEnabled: true,
		Capacity:  ccip_tokens.ScaleFloatToBigInt(cfg.Capacity, int(usdcDecimals), 0),
		Rate:      ccip_tokens.ScaleFloatToBigInt(cfg.Rate, int(usdcDecimals), 0),
	}
}

// toOnchainTransferFeeConfig maps the chain-agnostic fee schedule onto the pool's layout. The
// on-chain names split by finality rather than by "default"/"custom": the default-finality fields
// are the standard-finality ones, and the custom-finality fields are the fast-finality ones.
func toOnchainTransferFeeConfig(cfg ccip_tokens.TokenTransferFeeConfig) cctp_token_pool.TokenTransferFeeConfig {
	return cctp_token_pool.TokenTransferFeeConfig{
		DestGasOverhead:    cctp_token_pool.CrossChainGas{V0: cfg.DestGasOverhead},
		DestBytesOverhead:  cfg.DestBytesOverhead,
		FinalityFee:        cctp_token_pool.UsdCents{V0: cfg.DefaultFinalityFeeUSDCents},
		FastFinalityFee:    cctp_token_pool.UsdCents{V0: cfg.CustomFinalityFeeUSDCents},
		FinalityBpsFee:     cfg.DefaultFinalityTransferFeeBps,
		FastFinalityBpsFee: cfg.CustomFinalityTransferFeeBps,
		IsEnabled:          cfg.IsEnabled,
	}
}

// fromOnchainTransferFeeConfig is the inverse of toOnchainTransferFeeConfig.
func fromOnchainTransferFeeConfig(cfg cctp_token_pool.TokenTransferFeeConfig) ccip_tokens.TokenTransferFeeConfig {
	return ccip_tokens.TokenTransferFeeConfig{
		DestGasOverhead:               cfg.DestGasOverhead.V0,
		DestBytesOverhead:             cfg.DestBytesOverhead,
		DefaultFinalityFeeUSDCents:    cfg.FinalityFee.V0,
		CustomFinalityFeeUSDCents:     cfg.FastFinalityFee.V0,
		DefaultFinalityTransferFeeBps: cfg.FinalityBpsFee,
		CustomFinalityTransferFeeBps:  cfg.FastFinalityBpsFee,
		IsEnabled:                     cfg.IsEnabled,
	}
}

// resolveTransferFeeConfig turns the caller's partial fee schedule into the full on-chain value,
// mirroring upstream's semantics: a nil or empty partial leaves the on-chain value untouched, and
// a partial merges over the current value. A lane that was never configured merges over the zero
// value, which is the same default the pre-pointer input produced.
func resolveTransferFeeConfig(
	b cldf_ops.Bundle,
	chain cldf_solana.Chain,
	partial *ccip_tokens.PartialTokenTransferFeeConfig,
	poolAddr, mint solana.PublicKey,
	remoteSelector uint64,
) (cctp_token_pool.TokenTransferFeeConfig, error) {
	chainConfigV2PDA, _, err := tokens.TokenPoolChainConfigV2PDA(remoteSelector, mint, poolAddr)
	if err != nil {
		return cctp_token_pool.TokenTransferFeeConfig{}, fmt.Errorf("derive chain config v2 PDA: %w", err)
	}
	var existing cctp_token_pool.ChainConfigV2
	accountExists := true
	err = common.GetAccountDataBorshIntoAnchor(b.GetContext(), chain.Client, chainConfigV2PDA, cldf_solana.SolDefaultCommitment, &existing)
	switch {
	case err == nil:
	case errors.Is(err, rpc.ErrNotFound):
		// Lane never configured: start from the native CCTP defaults instead of the zero value.
		accountExists = false
	default:
		return cctp_token_pool.TokenTransferFeeConfig{}, fmt.Errorf("read chain config v2: %w", err)
	}
	if partial == nil || partial.IsEmpty() {
		return existing.TokenTransferFeeConfig, nil
	}
	if !accountExists {
		// A new lane has no on-chain config to merge over, so unset partial fields must fall back
		// to the native CCTP fee config. Merging over the zero value would disable the lane
		// (IsEnabled=false) and zero out DestBytesOverhead.
		native := cctpconfig.NativeCCTPTokenTransferFeeConfig().MergeWith(ccip_tokens.TokenTransferFeeConfig{})
		return toOnchainTransferFeeConfig(partial.MergeWith(native)), nil
	}
	return toOnchainTransferFeeConfig(partial.MergeWith(fromOnchainTransferFeeConfig(existing.TokenTransferFeeConfig))), nil
}

func dedupeKeys(keys []solana.PublicKey) []solana.PublicKey {
	seen := make(map[solana.PublicKey]struct{}, len(keys))
	out := make([]solana.PublicKey, 0, len(keys))
	for _, key := range keys {
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	return out
}

// sortedSelectors keeps lane configuration deterministic across runs, which matters because the
// resulting MCMS batch operations are ordered.
func sortedSelectors(m map[uint64]ccipadapters.RemoteCCTPChainConfig) []uint64 {
	out := make([]uint64, 0, len(m))
	for selector := range m {
		out = append(out, selector)
	}
	slices.Sort(out)
	return out
}
