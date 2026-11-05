package sequences

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/gagliardetto/solana-go"

	ccip_tokens "github.com/smartcontractkit/chainlink-ccip/deployment/tokens"
	seq_core "github.com/smartcontractkit/chainlink-ccip/deployment/utils/sequences"
	ccipadapters "github.com/smartcontractkit/chainlink-ccip/deployment/v2_0_0/adapters"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	cldf_ops "github.com/smartcontractkit/chainlink-deployments-framework/operations"
	mcms_types "github.com/smartcontractkit/mcms/types"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/lombard_token_pool"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	cvops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/committeeverifier"
	fqops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/feequoter"
	lvops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/lombardverifier"
	routerops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/router"
	tokenpoolops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/tokenpools"
)

// ConfigureLombardChainForLanes implements the Solana side of adapters.LombardChain's
// per-remote-chain configuration: for each remote, it sets the token pool's remote token/pool
// config (dest_chain_id/dest_caller from the Lombard chain ID + allowed caller on the remote
// side) and the verifier's chain config, GMP routing path, and remote token adapter. Rate limits
// aren't set here - RemoteLombardChainConfig doesn't carry them; that's the generic
// SetTokenPoolRateLimits sequence's job, same as for BurnMint/LockRelease.
var ConfigureLombardChainForLanes = cldf_ops.NewSequence(
	"configure-lombard-chain-for-lanes",
	semver.MustParse("2.0.0"),
	"Configures the Lombard token pool and verifier for a set of remote chains",
	func(b cldf_ops.Bundle, deps ccipadapters.ConfigureLombardChainForLanesDeps, input ccipadapters.ConfigureLombardChainForLanesInput) (seq_core.OnChainOutput, error) {
		solChain, ok := deps.BlockChains.SolanaChains()[input.ChainSelector]
		if !ok {
			return seq_core.OnChainOutput{}, fmt.Errorf("solana chain with selector %d not found", input.ChainSelector)
		}
		if deps.DataStore == nil {
			return seq_core.OnChainOutput{}, errors.New("datastore is required")
		}
		existing := deps.DataStore.Addresses().Filter(datastore.AddressRefByChainSelector(input.ChainSelector))

		poolRef, err := solcommon.PubkeyFromRefs(existing, datastore.NewAddressRefKey(
			input.ChainSelector, datastore.ContractType(tokenpoolops.LombardTokenPool), solcommon.Version2_0_0, solcommon.PoolProgramQualifier,
		))
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("find lombard token pool in existing addresses: %w", err)
		}
		token, err := solana.PublicKeyFromBase58(input.Token)
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("parse token address %q: %w", input.Token, err)
		}

		// RouterSigner is a property of *this* chain's router, not of the remote chain, so it's
		// derived once and reused across every remote chain config - mirroring how
		// deploy_committee_verifier.go/CLAUDE.md document the ccvSignerPDA derivation.
		routerAddr, err := solcommon.PubkeyFromRefs(existing, datastore.NewAddressRefKey(
			input.ChainSelector, datastore.ContractType(routerops.ContractType), routerops.Version, "",
		))
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("find router in existing addresses: %w", err)
		}
		routerSignerPDA, _, err := state.FindCCVSignerPDA(poolRef, routerAddr)
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("derive router signer PDA: %w", err)
		}

		// Chain-level refs for the verifier lane lookup table extensions below: the Lombard
		// bridge/mailbox (registered by deploy-lombard-chains) plus the router-side ccip_send
		// accounts. These are properties of *this* chain, resolved once and reused per remote.
		bridgeAddr, err := solcommon.PubkeyFromRefs(existing, datastore.NewAddressRefKey(
			input.ChainSelector, datastore.ContractType(solcommon.LombardBridgeV2ContractType), solcommon.LombardChainContractsVersion, "",
		))
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("find lombard bridge in existing addresses: %w", err)
		}
		mailboxAddr, err := solcommon.PubkeyFromRefs(existing, datastore.NewAddressRefKey(
			input.ChainSelector, datastore.ContractType(solcommon.LombardMailboxContractType), solcommon.LombardChainContractsVersion, "",
		))
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("find lombard mailbox in existing addresses: %w", err)
		}
		feeQuoterAddr, err := solcommon.PubkeyFromRefs(existing, datastore.NewAddressRefKey(
			input.ChainSelector, datastore.ContractType(fqops.ContractType), fqops.Version, "",
		))
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("find fee quoter in existing addresses: %w", err)
		}
		cvProgramID, err := solcommon.PubkeyFromRefs(existing, datastore.NewAddressRefKey(
			input.ChainSelector, datastore.ContractType(cvops.ContractType), cvops.Version, "",
		))
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("find committee verifier in existing addresses: %w", err)
		}

		var batchOps []mcms_types.BatchOperation
		var addresses []datastore.AddressRef
		for remoteSel, remoteCfg := range input.RemoteChains {
			remoteAdapter, ok := deps.RemoteChains[remoteSel]
			if !ok {
				return seq_core.OnChainOutput{}, fmt.Errorf("no Lombard adapter registered for remote chain %d's family", remoteSel)
			}

			allowedCaller, err := remoteAdapter.AllowedCallerOnDest(deps.DataStore, deps.BlockChains, remoteSel)
			if err != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("allowed caller on remote chain %d: %w", remoteSel, err)
			}
			remoteTokenAddress, err := remoteAdapter.RemoteTokenAddress(b, deps.DataStore, deps.BlockChains, remoteSel, input.TokenQualifier)
			if err != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("remote token address on chain %d: %w", remoteSel, err)
			}
			remoteTokenPoolAddress, err := remoteAdapter.RemoteTokenPoolAddress(deps.DataStore, deps.BlockChains, remoteSel, input.TokenQualifier)
			if err != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("remote token pool address on chain %d: %w", remoteSel, err)
			}
			// RemoteLombardChainConfig (unlike the generic topology-driven RemoteChainConfig used
			// by BurnMint/LockRelease) carries no decimals field, and assuming parity with the
			// local Solana mint is wrong whenever the two chains' tokens don't share a decimals
			// convention (this devenv's EVM LBTC test token is 18 decimals, the Solana one is 9;
			// release_or_mint_tokens_v2's to_svm_token_amount scaling silently skipped scaling
			// entirely when this was mis-set to the local decimals, minting a 10^9-off amount).
			// So this queries the remote token's own decimals via the adapter instead.
			remoteDecimals, err := remoteAdapter.RemoteTokenDecimals(b, deps.DataStore, deps.BlockChains, remoteSel, input.TokenQualifier)
			if err != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("remote token decimals on chain %d: %w", remoteSel, err)
			}
			// EVM's adapter returns a raw 20-byte address (chainlink-ccip's
			// LombardChainAdapter.AddressRefToBytes), but validate_release_or_mint_v2 compares
			// this byte-for-byte against the message's source_pool_address, which EVM's onramp
			// always encodes left-padded to 32 bytes (abi.encode(address) convention - same
			// reason allowedCaller gets padded below). Without padding here, every inbound
			// EVM->Solana Lombard transfer fails with InvalidSourcePoolAddress.
			remoteTokenAddress, err = solcommon.LeftPadTo32Bytes(remoteTokenAddress)
			if err != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("left-pad remote token address on chain %d: %w", remoteSel, err)
			}
			remoteTokenPoolAddress, err = solcommon.LeftPadTo32Bytes(remoteTokenPoolAddress)
			if err != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("left-pad remote token pool address on chain %d: %w", remoteSel, err)
			}

			lChainID := paddedLombardChainID(remoteCfg.LombardChainId)
			var allowedCaller32, remoteBridgeSender32 [32]byte
			copy(allowedCaller32[32-len(allowedCaller):], allowedCaller)
			if remoteCfg.RemoteBridgeSender != "" {
				remoteBridgeSender32, err = parseLombardIdentifier(remoteCfg.RemoteBridgeSender)
				if err != nil {
					return seq_core.OnChainOutput{}, fmt.Errorf("parse remote bridge sender for chain %d: %w", remoteSel, err)
				}
			}

			if err = runOp(b, tokenpoolops.UpsertRemoteChainConfigLombard, solChain, tokenpoolops.LombardRemoteChainConfig{
				RemoteChainConfig: tokenpoolops.RemoteChainConfig{
					TokenPool:          poolRef,
					TokenMint:          token,
					RemoteSelector:     remoteSel,
					RemoteTokenAddress: remoteTokenAddress,
					RemotePoolAddress:  remoteTokenPoolAddress,
					RemoteDecimals:     remoteDecimals,
				},
				DestinationChainID: lChainID,
				DestinationCaller:  allowedCaller32,
			}, &batchOps); err != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("upsert remote chain config for chain %d: %w", remoteSel, err)
			}

			// Pool-side CCIP 2.0 fee/finality config for this remote (set_chain_config_v2).
			// Lombard's CCV discovery rides the verifier config PDA rather than the pool's
			// ccv_config, so CcvConfig stays empty here.
			if err = runOp(b, tokenpoolops.SetChainConfigV2Lombard, solChain, tokenpoolops.SetChainConfigV2LombardParams{
				TokenPool:              poolRef,
				TokenMint:              token,
				RemoteChainSelector:    remoteSel,
				AllowedFinalityConfig:  lombard_token_pool.FinalityConfig{},
				TokenTransferFeeConfig: toOnchainTransferFeeConfigLombard(remoteCfg.TokenTransferFeeConfig),
				CcvConfig:              lombard_token_pool.CcvConfig{},
			}, &batchOps); err != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("set pool v2 chain config for chain %d: %w", remoteSel, err)
			}

			if err = runOp(b, lvops.SetChainConfig, solChain, lvops.SetChainConfigParams{
				LombardVerifier:     poolRef,
				RemoteChainSelector: remoteSel,
				RouterSigner:        routerSignerPDA,
				FeeUSDCents:         remoteCfg.FeeUSDCents,
				GasForVerification:  remoteCfg.GasForVerification,
				PayloadSizeBytes:    uint32(remoteCfg.PayloadSizeBytes),
			}, &batchOps); err != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("set verifier chain config for chain %d: %w", remoteSel, err)
			}

			// This remote chain's own verifier lookup table (VerifierRemoteChainConfig.lookup_table)
			lutOut, err := cldf_ops.ExecuteOperation(b, lvops.CreateAndSetLookupTable, solChain, lvops.LookupTableParams{
				ExistingAddresses:   existing,
				LombardVerifier:     poolRef,
				RemoteChainSelector: remoteSel,
			})
			if err != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("create and set lombard verifier lookup table for chain %d: %w", remoteSel, err)
			}
			table, err := solcommon.PubkeyFromRefs(lutOut.Output.Addresses, datastore.NewAddressRefKey(solChain.Selector, datastore.ContractType(lvops.LookupTableType), lvops.Version, strconv.FormatUint(remoteSel, 10)))
			if err != nil {
				return seq_core.OnChainOutput{}, err
			}
			lvConfigPDA, _, err := state.FindLVConfigPDA(poolRef)
			if err != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("find lombard verifier config PDA: %w", err)
			}
			if err = runOp(b, lvops.ExtendLookupTable, solChain, lvops.ExtendLookupTableParams{
				LookupTable:         table,
				LombardVerifier:     poolRef,
				RemoteChainSelector: remoteSel,
				Entries:             []solana.PublicKey{poolRef, lvConfigPDA},
			}, &batchOps); err != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("extend lombard verifier lookup table with pool/verifier for chain %d: %w", remoteSel, err)
			}
			addresses = append(addresses, lutOut.Output.Addresses...)
			batchOps = append(batchOps, lutOut.Output.BatchOps...)

			if err = runOp(b, lvops.SetPath, solChain, lvops.SetPathParams{
				LombardVerifier:     poolRef,
				RemoteChainSelector: remoteSel,
				LChainID:            lChainID,
				AllowedCaller:       allowedCaller32,
				RemoteBridgeSender:  remoteBridgeSender32,
			}, &batchOps); err != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("set verifier path for chain %d: %w", remoteSel, err)
			}

			if remoteCfg.RemoteAdapter != "" {
				remoteAdapterID, perr := parseLombardIdentifier(remoteCfg.RemoteAdapter)
				if perr != nil {
					return seq_core.OnChainOutput{}, fmt.Errorf("parse remote adapter for chain %d: %w", remoteSel, perr)
				}
				if err = runOp(b, lvops.SetRemoteAdapters, solChain, lvops.SetRemoteAdaptersParams{
					LombardVerifier:     poolRef,
					RemoteChainSelector: remoteSel,
					RemoteAdapters: []lombard_token_pool.RemoteAdapter{
						{Token: token, RemoteAdapter: remoteAdapterID},
					},
				}, &batchOps); err != nil {
					return seq_core.OnChainOutput{}, fmt.Errorf("set verifier remote adapters for chain %d: %w", remoteSel, err)
				}
			}

			// Extend this remote's verifier lookup table with the full verify-path account set
			// (bridge, mailbox, configs, token authority, ccv signer, path/chain-config PDAs).
			// The addresses are derived even when their creation is still governed.
			if err = runOp(b, lvops.ExtendLookupTableForLane, solChain, lvops.ExtendLookupTableForLaneParams{
				LookupTable:         table,
				LombardVerifier:     poolRef,
				Bridge:              bridgeAddr,
				Mailbox:             mailboxAddr,
				Router:              routerAddr,
				Mint:                token,
				RemoteChainSelector: remoteSel,
				LChainID:            lChainID,
			}, &batchOps); err != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("extend lombard verifier lookup table for lane for chain %d: %w", remoteSel, err)
			}

			// Also add the router-side ccip_send accounts for this remote to the same table.
			lutEntries, err := CcipSendRemoteLutEntries(remoteSel, routerAddr, feeQuoterAddr, token, cvProgramID)
			if err != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("compute ccip_send lookup table entries for chain %d: %w", remoteSel, err)
			}
			if err = runOp(b, lvops.ExtendLookupTable, solChain, lvops.ExtendLookupTableParams{
				LookupTable:         table,
				LombardVerifier:     poolRef,
				RemoteChainSelector: remoteSel,
				Entries:             lutEntries,
			}, &batchOps); err != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("extend lombard verifier lookup table with ccip_send entries for chain %d: %w", remoteSel, err)
			}
		}

		return seq_core.OnChainOutput{Addresses: addresses, BatchOps: batchOps}, nil
	},
)

// toOnchainTransferFeeConfigLombard maps the chain-agnostic fee schedule onto the Lombard
// pool's layout; mirrors toOnchainTransferFeeConfig for the CCTP pool bindings. The on-chain
// names split by finality: default-finality fields are the standard-finality ones, and the
// custom-finality fields are the fast-finality ones.
func toOnchainTransferFeeConfigLombard(cfg ccip_tokens.TokenTransferFeeConfig) lombard_token_pool.TokenTransferFeeConfig {
	return lombard_token_pool.TokenTransferFeeConfig{
		DestGasOverhead:    lombard_token_pool.CrossChainGas{V0: cfg.DestGasOverhead},
		DestBytesOverhead:  cfg.DestBytesOverhead,
		FinalityFee:        lombard_token_pool.UsdCents{V0: cfg.DefaultFinalityFeeUSDCents},
		FastFinalityFee:    lombard_token_pool.UsdCents{V0: cfg.CustomFinalityFeeUSDCents},
		FinalityBpsFee:     cfg.DefaultFinalityTransferFeeBps,
		FastFinalityBpsFee: cfg.CustomFinalityTransferFeeBps,
		IsEnabled:          cfg.IsEnabled,
	}
}

// paddedLombardChainID matches the EVM adapter's identical helper
// (chains/evm/deployment/v2_0_0/sequences/lombard/configure_lombard_chain_for_lanes.go): a
// big-endian uint32, left-padded with zeros to 32 bytes (value occupies the low 4 bytes).
func paddedLombardChainID(lombardChainID uint32) [32]byte {
	var out [32]byte
	binary.BigEndian.PutUint32(out[28:], lombardChainID)
	return out
}

// parseLombardIdentifier accepts a hex string (with or without "0x") of up to 32 bytes and
// left-pads it to bytes32, matching how the LombardVerifier stores remote identifiers -
// mirroring the EVM adapter's identical helper. On Solana a full 32-byte value is typically
// already a Pubkey; the left-pad only matters for 20-byte EVM addresses.
func parseLombardIdentifier(raw string) ([32]byte, error) {
	decoded, err := hex.DecodeString(strings.TrimPrefix(raw, "0x"))
	if err != nil {
		return [32]byte{}, fmt.Errorf("invalid hex identifier %q: %w", raw, err)
	}
	if len(decoded) == 0 || len(decoded) > 32 {
		return [32]byte{}, fmt.Errorf("identifier %q must be 1-32 bytes, got %d", raw, len(decoded))
	}
	var out [32]byte
	copy(out[32-len(decoded):], decoded)
	return out, nil
}
