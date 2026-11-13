package sequences

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	seq_core "github.com/smartcontractkit/chainlink-ccip/deployment/utils/sequences"
	ccipadapters "github.com/smartcontractkit/chainlink-ccip/deployment/v2_0_0/adapters"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	cldf_ops "github.com/smartcontractkit/chainlink-deployments-framework/operations"
	mcms_types "github.com/smartcontractkit/mcms/types"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/ccip_router"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/lombard_token_pool"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/tokens"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	fqops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/feequoter"
	lvops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/lombardverifier"
	offrampops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/offramp"
	rmnremoteops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/rmnremote"
	routerops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/router"
	tokenpoolops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/tokenpools"
)

// DeployLombardChain implements the Solana side of the upstream adapters.LombardChain interface
// (chainlink-ccip/deployment/v2_0_0/adapters/lombard.go): deploy/resolve the lombard_token_pool
// program, initialize the pool, initialize its embedded verifier (same program, separate
// PDAs - see lombardverifier package docs), then wire the verifier config PDA into the
// offramp/ccipSend lookup tables the same way DeployCommitteeVerifier does for committee_verifier.
var DeployLombardChain = cldf_ops.NewSequence(
	"deploy-lombard-chain",
	semver.MustParse("2.0.0"),
	"Deploys and configures the Lombard token pool and its embedded verifier on a Solana chain",
	func(b cldf_ops.Bundle, deps ccipadapters.DeployLombardChainDeps, input ccipadapters.DeployLombardInput) (seq_core.OnChainOutput, error) {
		var batchOps []mcms_types.BatchOperation
		solChain, ok := deps.BlockChains.SolanaChains()[input.ChainSelector]
		if !ok {
			return seq_core.OnChainOutput{}, fmt.Errorf("solana chain with selector %d not found", input.ChainSelector)
		}
		if deps.DataStore == nil {
			return seq_core.OnChainOutput{}, errors.New("datastore is required")
		}
		existing := deps.DataStore.Addresses().Filter(datastore.AddressRefByChainSelector(input.ChainSelector))

		bridge, err := solana.PublicKeyFromBase58(input.Bridge)
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("parse bridge address %q: %w", input.Bridge, err)
		}
		token, err := solana.PublicKeyFromBase58(input.Token)
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("parse token address %q: %w", input.Token, err)
		}

		// The lookup table ref that router:set-pool records below is qualified by token symbol,
		// kept semantic rather than derived from the mint so the datastore key is not anchored
		// on an address.
		//
		// The caller declares it as TokenQualifier. Only when that is unset do we fall back to
		// the token's own datastore ref, which covers a token deployed earlier in the same run
		// that already carries its symbol as a qualifier. A token supplied by address alone --
		// as devenv does for a pre-existing mint -- has neither, and is the case the fallback
		// cannot serve: it is reported as a missing input rather than a missing ref, because
		// setting TokenQualifier is what fixes it.
		//
		// Resolved here, before the first deployment operation, rather than next to its only use
		// further down: every path out of this block is a config error, and a config error must
		// not land after DeployLombard/InitializeLombard/RegisterTokenAdminRegistry have already
		// left the chain half-configured.
		tokenSymbol := input.TokenQualifier
		if tokenSymbol == "" {
			// One mint can carry several refs -- devenv aliases the Lombard mint under both
			// "LBTC" and the EVM-side "Lombard_LBTC" (devenv/lombard.go) -- and the datastore
			// filter returns no guaranteed order, so taking the first match would pick a
			// qualifier arbitrarily. Make the caller disambiguate instead.
			var candidates []string
			for _, ref := range existing {
				if ref.Address == input.Token && ref.Qualifier != "" && !slices.Contains(candidates, ref.Qualifier) {
					candidates = append(candidates, ref.Qualifier)
				}
			}
			if len(candidates) > 1 {
				slices.Sort(candidates)
				return seq_core.OnChainOutput{}, fmt.Errorf(
					"token %s on chain %d is registered under %d qualifiers (%s); set TokenQualifier "+
						"on the Lombard chain config to select the one the pool lookup table is keyed by",
					input.Token, input.ChainSelector, len(candidates), strings.Join(candidates, ", "))
			}
			if len(candidates) == 1 {
				tokenSymbol = candidates[0]
			}
		}
		if tokenSymbol == "" {
			return seq_core.OnChainOutput{}, fmt.Errorf(
				"TokenQualifier is not set for token %s on chain %d and the token has no "+
					"datastore ref to take a symbol from; set TokenQualifier on the Lombard "+
					"chain config so the pool lookup table gets a semantic qualifier",
				input.Token, input.ChainSelector)
		}

		deployRep, err := cldf_ops.ExecuteOperation(b, tokenpoolops.DeployLombard, solChain, existing)
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("deploy lombard token pool: %w", err)
		}
		poolRef := deployRep.Output
		poolAddr, err := solana.PublicKeyFromBase58(poolRef.Address)
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("parse lombard token pool address %q: %w", poolRef.Address, err)
		}

		routerAddr, err := solcommon.PubkeyFromRefs(existing, datastore.NewAddressRefKey(
			input.ChainSelector, datastore.ContractType(routerops.ContractType), routerops.Version, "",
		))
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("find router in existing addresses: %w", err)
		}
		rmnRemoteAddr, err := solcommon.PubkeyFromRefs(existing, datastore.NewAddressRefKey(
			input.ChainSelector, datastore.ContractType(rmnremoteops.ContractType), rmnremoteops.Version, "",
		))
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("find rmn remote in existing addresses: %w", err)
		}
		fqAddr, err := solcommon.PubkeyFromRefs(existing, datastore.NewAddressRefKey(
			input.ChainSelector, datastore.ContractType(fqops.ContractType), fqops.Version, "",
		))
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("find fee quoter in existing addresses: %w", err)
		}

		// The SPL token program that owns the mint (classic Token or Token-2022) - the pool
		// needs this for every subsequent instruction, but DeployLombardInput doesn't carry it
		// explicitly, so read it straight off the mint account. Plain GetAccountInfo defaults to
		// "finalized" commitment, but the mint was only just confirmed at solcommon's default
		// "confirmed" level (see tokens.go's DeploySolanaToken)
		mintInfo, err := solChain.Client.GetAccountInfoWithOpts(b.GetContext(), token, &rpc.GetAccountInfoOpts{
			Commitment: rpc.CommitmentConfirmed,
		})
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("get mint account info for %s: %w", token.String(), err)
		}
		tokenProgramID := mintInfo.Value.Owner

		initOut, err := cldf_ops.ExecuteOperation(b, tokenpoolops.InitializeLombard, solChain, tokenpoolops.LombardParams{
			Params: tokenpoolops.Params{
				TokenPool:      poolAddr,
				TokenMint:      token,
				TokenProgramID: tokenProgramID,
				RMNRemote:      rmnRemoteAddr,
				Router:         routerAddr,
			},
			Bridge: bridge,
		})
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("initialize lombard token pool: %w", err)
		}
		batchOps = append(batchOps, initOut.Output.BatchOps...)

		// Create the pool signer's ATA. Lombard's own V2 mint/burn CPI never touches this
		// account's balance (the bridge mints directly to the receiver inside verify()), but the
		// offramp's generic V2 token-transfer account layout
		// (ccip_common::v2::CcipV2OfframpOnrampCommonTokenAccounts) requires a real, initialized
		// pool_token_account for every pool type - mirroring DeployTokenPoolForToken
		// (sequences/tokens.go) does for BurnMint/LockRelease. Without it, ExecuteV2 fails with
		// AccountNotInitialized on pool_token_account.
		poolSigner, err := tokens.TokenPoolSignerAddress(token, poolAddr)
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("derive lombard token pool signer address: %w", err)
		}
		ataIxn, _, err := tokens.CreateAssociatedTokenAccount(tokenProgramID, token, poolSigner, solChain.Deployer().PublicKey())
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("build create ATA instruction for lombard pool token account: %w", err)
		}
		if err = solChain.Confirm([]solana.Instruction{ataIxn}); err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("create lombard pool token account: %w", err)
		}

		// Register the pool with the router's TokenAdminRegistry and build its token lookup table
		// (pool_program@index2, mint@index7) - ccip-offramp's get_ccvs_for_msg account resolution
		// (resolve_accounts.rs) reads exactly this LUT to find a pool at all, the same way
		// BurnMint/LockRelease pools get it via ConfigureTokenForTransfersSequence
		// (sequences/tokens.go). Nothing previously called this for Lombard, so the offramp could
		// never resolve accounts for any Lombard message - only caught by an actual execute
		// (AccountNotInitialized inside ResolveAccountsGetCcvsForMsgStage).
		rtarOut, err := cldf_ops.ExecuteOperation(b, routerops.RegisterTokenAdminRegistry, solChain, routerops.TokenAdminRegistryParams{
			Router:            routerAddr,
			TokenMint:         token,
			ExistingAddresses: existing,
		})
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("register token admin registry: %w", err)
		}
		batchOps = append(batchOps, rtarOut.Output.BatchOps...)
		if !rtarOut.Output.PendingSigner.IsZero() {
			if err = runOp(b, routerops.AcceptTokenAdminRegistry, solChain, routerops.TokenAdminRegistryParams{
				Router:            routerAddr,
				TokenMint:         token,
				Admin:             rtarOut.Output.PendingSigner,
				ExistingAddresses: existing,
			}, &batchOps); err != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("accept token admin registry: %w", err)
			}
		}

		setPoolOut, err := cldf_ops.ExecuteOperation(b, routerops.SetPool, solChain, routerops.PoolParams{
			ExistingAddresses: existing,
			Router:            routerAddr,
			FeeQuoter:         fqAddr,
			TokenMint:         token,
			TokenPool:         poolAddr,
			TokenProgramID:    tokenProgramID,
			TokenPoolType:     string(tokenpoolops.LombardTokenPool),
			TokenSymbol:       tokenSymbol,
			// pool_signer (index 5) must be writable here too: verify() needs the same PDA
			// mutable to pay message_handled's rent, and execute_v2 bundles both CPIs into one
			// transaction, so Solana unions the writable flag across the whole tx.
			WritableIndexes: []uint8{3, 4, 5, 7},
		})
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("set pool on router: %w", err)
		}
		batchOps = append(batchOps, setPoolOut.Output.BatchOps...)
		if err = runOp(b, routerops.SetPoolInterfaceImplementation, solChain, routerops.SetPoolInterfaceImplParams{
			Router:           routerAddr,
			TokenMint:        token,
			InterfaceVersion: 2,
			// Lombard's own lock_or_burn_v2/release_or_mint_v2 CPI is bookkeeping-only
			ImplementsAccountResolution: uint8(ccip_router.ResolutionType_WithoutExtendedLookupTable),
		}, &batchOps); err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("set pool interface implementation: %w", err)
		}

		if _, err = cldf_ops.ExecuteOperation(b, lvops.Initialize, solChain, lvops.Params{
			LombardVerifier: poolAddr,
		}); err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("initialize lombard verifier: %w", err)
		}

		// start_verification_at_source rejects sends of any mint missing from the verifier's
		// supported_tokens list (UnsupportedToken), so onboarding the mint is part of deploy -
		// mirroring the EVM sequence's UpdateSupportedTokens call and devenv's deployLombardChain.
		if err = runOp(b, lvops.SetSupportedTokens, solChain, lvops.SetSupportedTokensParams{
			LombardVerifier: poolAddr,
			TokensToSet: []lombard_token_pool.SupportedToken{
				{LocalToken: token},
			},
		}, &batchOps); err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("set lombard verifier supported tokens: %w", err)
		}

		// Wire the verifier's config PDA into the offramp/ccipSend lookup tables, mirroring
		// DeployCommitteeVerifier: the pool address itself also needs to be resolvable via the
		// ccipSend LUT the same way BurnMint/LockRelease pool addresses are (deploy_chain_contracts.go).
		lvConfigPDA, _, err := state.FindLVConfigPDA(poolAddr)
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("find lombard verifier config PDA: %w", err)
		}
		offRampPubkey, err := solcommon.PubkeyFromRefs(existing, datastore.NewAddressRefKey(
			input.ChainSelector, datastore.ContractType(offrampops.ContractType), offrampops.Version, "",
		))
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("find offramp in existing addresses: %w", err)
		}
		if err = solcommon.ExtendOfframpLookupTable(solChain, offRampPubkey, []solana.PublicKey{lvConfigPDA}); err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("extend offramp lookup table with lombard verifier: %w", err)
		}

		if input.FeeAggregator != "" {
			feeAgg, perr := solana.PublicKeyFromBase58(input.FeeAggregator)
			if perr != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("parse fee aggregator %q: %w", input.FeeAggregator, perr)
			}
			if err = runOp(b, lvops.SetFeeAggregator, solChain, lvops.SetFeeAggregatorParams{
				LombardVerifier: poolAddr,
				FeeAggregator:   feeAgg,
			}, &batchOps); err != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("set fee aggregator: %w", err)
			}
		}

		if len(input.StorageLocations) > 0 {
			if err = runOp(b, lvops.SetStorageLocations, solChain, lvops.SetStorageLocationsParams{
				LombardVerifier:  poolAddr,
				StorageLocations: input.StorageLocations,
			}, &batchOps); err != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("set storage locations: %w", err)
			}
		}

		if input.RateLimitAdmin != "" {
			rateLimitAdmin, perr := solana.PublicKeyFromBase58(input.RateLimitAdmin)
			if perr != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("parse rate limit admin %q: %w", input.RateLimitAdmin, perr)
			}
			if err = runOp(b, tokenpoolops.UpdateRateLimitAdminLombard, solChain, tokenpoolops.TokenPoolTransferOwnershipInput{
				Program:   poolAddr,
				TokenMint: token,
				NewOwner:  rateLimitAdmin,
			}, &batchOps); err != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("update rate limit admin: %w", err)
			}
		}

		// Fee vaults: the verifier's own, and the token pool's for this mint.
		linkMint, err := findLinkMint(existing, input.ChainSelector)
		if err != nil {
			return seq_core.OnChainOutput{}, err
		}
		fees, err := feeTokens(b.GetContext(), solChain, linkMint)
		if err != nil {
			return seq_core.OnChainOutput{}, err
		}
		if err = ensureFeeVaults(b.GetContext(), solChain, poolAddr, []byte{}, fees); err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("lombard verifier: %w", err)
		}
		if err = ensureFeeVaults(b.GetContext(), solChain, poolAddr, token.Bytes(), fees); err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("lombard token pool: %w", err)
		}

		// Pool and verifier are the same program, but registered under both contract types since
		// consumers look each up by their own distinct type
		verifierRef := poolRef
		verifierRef.Type = datastore.ContractType(lvops.ContractType)
		verifierRef.Qualifier = lvops.ContractType.String()

		// Dual-register under the EVM-canonical resolver qualifier so one qualifier resolves the
		// verifier on both families (no separate resolver contract exists on Solana).
		verifierResolverRef := verifierRef
		verifierResolverRef.Qualifier = lvops.LombardVerifierResolverQualifier

		// setPoolOut.Output.Addresses carries the TokenPoolLookupTableType ref for this mint. Each
		// remote chain's own LombardVerifierLookupTable ref is registered later, per lane, by
		// ConfigureLombardChainForLanes (see configure_lombard_chain_for_lanes.go).
		addresses := append([]datastore.AddressRef{poolRef, verifierRef, verifierResolverRef}, setPoolOut.Output.Addresses...)
		return seq_core.OnChainOutput{Addresses: addresses, BatchOps: batchOps}, nil
	},
)
