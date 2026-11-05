package sequences

import (
	"context"
	"fmt"
	"maps"
	"math/big"
	"slices"
	"strings"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	chainsel "github.com/smartcontractkit/chain-selectors"
	deployapi "github.com/smartcontractkit/chainlink-ccip/deployment/deploy"
	tokenapi "github.com/smartcontractkit/chainlink-ccip/deployment/tokens"
	common_utils "github.com/smartcontractkit/chainlink-ccip/deployment/utils"
	datastore_utils "github.com/smartcontractkit/chainlink-ccip/deployment/utils/datastore"
	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/sequences"
	cldf_chain "github.com/smartcontractkit/chainlink-deployments-framework/chain"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	"github.com/smartcontractkit/chainlink-deployments-framework/deployment"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/burnmint_token_pool"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/tokens"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	tokenpoolops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/tokenpools"
	tokensops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/tokens"
)

var (
	_ tokenapi.RemotePoolRemover             = (*SolanaAdapter)(nil)
	_ tokenapi.TokenPoolDynamicConfigAdapter = (*SolanaAdapter)(nil)
	_ tokenapi.RateLimitReaderAdapter        = (*SolanaAdapter)(nil)
	_ tokenapi.TokenRefResolver              = (*SolanaAdapter)(nil)
	_ tokenapi.TokenPoolMigrator             = (*SolanaAdapter)(nil)
)

// RemoveRemotePools removes remote pool entries from a Solana token pool. The pool address is a
// program ID shared across mints, so the mint comes from input.TokenRef.
func (a *SolanaAdapter) RemoveRemotePools() *operations.Sequence[tokenapi.RemoveRemotePoolsSequenceInput, sequences.OnChainOutput, cldf_chain.BlockChains] {
	return operations.NewSequence(
		"solana-adapter:remove-remote-pools",
		solcommon.Version2_0_0,
		"Removes remote pool entries from a Solana token pool",
		func(b operations.Bundle, chains cldf_chain.BlockChains, input tokenapi.RemoveRemotePoolsSequenceInput) (sequences.OnChainOutput, error) {
			chain, ok := chains.SolanaChains()[input.Selector]
			if !ok {
				return sequences.OnChainOutput{}, fmt.Errorf("solana chain with selector %d not defined", input.Selector)
			}
			var op *operations.Operation[tokenpoolops.RemoveRemotePoolInput, sequences.OnChainOutput, cldf_solana.Chain]
			switch input.TokenPoolRef.Type.String() {
			case common_utils.BurnMintTokenPool.String():
				op = tokenpoolops.RemoveRemotePoolBurnMint
			case common_utils.LockReleaseTokenPool.String():
				op = tokenpoolops.RemoveRemotePoolLockRelease
			case common_utils.CCTPTokenPool.String():
				op = tokenpoolops.RemoveRemotePoolCctp
			case tokenpoolops.LombardTokenPool.String():
				op = tokenpoolops.RemoveRemotePoolLombard
			default:
				return sequences.OnChainOutput{}, unsupportedTokenPoolTypeError(input.TokenPoolRef.Type.String())
			}
			tokenPool, tokenMint, err := poolAndMint(input.Selector, input.TokenPoolRef, input.TokenRef)
			if err != nil {
				return sequences.OnChainOutput{}, err
			}

			// Each removal rewrites the remote chain's whole pool list, so all removals for one
			// remote chain must be a single rewrite.
			addressesBySelector := make(map[uint64][][]byte)
			for _, remote := range input.RemotePoolsToRemove {
				raw, err := remoteAddressToBytes(remote.Selector, remote.Remote.Address)
				if err != nil {
					return sequences.OnChainOutput{}, err
				}
				addressesBySelector[remote.Selector] = append(addressesBySelector[remote.Selector], raw)
			}

			var result sequences.OnChainOutput
			for _, remoteSelector := range slices.Sorted(maps.Keys(addressesBySelector)) {
				out, err := operations.ExecuteOperation(b, op, chain, tokenpoolops.RemoveRemotePoolInput{
					TokenPool:           tokenPool,
					TokenMint:           tokenMint,
					RemoteSelector:      remoteSelector,
					RemotePoolAddresses: addressesBySelector[remoteSelector],
				})
				if err != nil {
					return sequences.OnChainOutput{}, fmt.Errorf("failed to remove remote pools for remote chain %d from pool %s on chain %d: %w", remoteSelector, tokenPool, input.Selector, err)
				}
				result.BatchOps = append(result.BatchOps, out.Output.BatchOps...)
			}
			return result, nil
		},
	)
}

// SetTokenPoolDynamicConfig sets the router and rate limit admin of a Solana token pool. Solana
// pools have no fee admin (fees are managed by the pool owner), so a non-nil FeeAdmin is rejected.
// set_router needs a signer that is both the pool owner and the pool program's upgrade authority;
// the operation checks that and errors otherwise. Each update is a no-op when the value matches.
func (a *SolanaAdapter) SetTokenPoolDynamicConfig() *operations.Sequence[tokenapi.SetTokenPoolDynamicConfigSequenceInput, sequences.OnChainOutput, cldf_chain.BlockChains] {
	return operations.NewSequence(
		"solana-adapter:set-token-pool-dynamic-config",
		solcommon.Version2_0_0,
		"Sets the router and rate limit admin on a Solana token pool",
		func(b operations.Bundle, chains cldf_chain.BlockChains, input tokenapi.SetTokenPoolDynamicConfigSequenceInput) (sequences.OnChainOutput, error) {
			if input.FeeAdmin != nil {
				return sequences.OnChainOutput{}, fmt.Errorf("fee admin is not supported on Solana token pools (pool %s on chain %d)", input.TokenPoolRef.Address, input.Selector)
			}
			if input.RateLimitAdmin == nil && input.Router == nil {
				return sequences.OnChainOutput{}, nil
			}
			chain, ok := chains.SolanaChains()[input.Selector]
			if !ok {
				return sequences.OnChainOutput{}, fmt.Errorf("solana chain with selector %d not defined", input.Selector)
			}
			var (
				rlAdminOp *operations.Operation[tokenpoolops.TokenPoolTransferOwnershipInput, sequences.OnChainOutput, cldf_solana.Chain]
				routerOp  *operations.Operation[tokenpoolops.SetPoolRouterInput, sequences.OnChainOutput, cldf_solana.Chain]
			)
			switch input.TokenPoolRef.Type.String() {
			case common_utils.BurnMintTokenPool.String():
				rlAdminOp, routerOp = tokenpoolops.UpdateRateLimitAdminBurnMint, tokenpoolops.SetRouterBurnMint
			case common_utils.LockReleaseTokenPool.String():
				rlAdminOp, routerOp = tokenpoolops.UpdateRateLimitAdminLockRelease, tokenpoolops.SetRouterLockRelease
			case common_utils.CCTPTokenPool.String():
				rlAdminOp, routerOp = tokenpoolops.UpdateRateLimitAdminCctp, tokenpoolops.SetRouterCctp
			case tokenpoolops.LombardTokenPool.String():
				rlAdminOp, routerOp = tokenpoolops.UpdateRateLimitAdminLombard, tokenpoolops.SetRouterLombard
			default:
				return sequences.OnChainOutput{}, unsupportedTokenPoolTypeError(input.TokenPoolRef.Type.String())
			}
			tokenPool, tokenMint, err := poolAndMint(input.Selector, input.TokenPoolRef, input.TokenRef)
			if err != nil {
				return sequences.OnChainOutput{}, err
			}

			var result sequences.OnChainOutput
			if input.Router != nil {
				router, err := solana.PublicKeyFromBase58(*input.Router)
				if err != nil {
					return sequences.OnChainOutput{}, fmt.Errorf("invalid router address for chain %d: %s: %w", input.Selector, *input.Router, err)
				}
				out, err := operations.ExecuteOperation(b, routerOp, chain, tokenpoolops.SetPoolRouterInput{
					TokenPool: tokenPool,
					TokenMint: tokenMint,
					Router:    router,
				})
				if err != nil {
					return sequences.OnChainOutput{}, fmt.Errorf("failed to set router on token pool %s on chain %d: %w", tokenPool, input.Selector, err)
				}
				result.BatchOps = append(result.BatchOps, out.Output.BatchOps...)
			}
			if input.RateLimitAdmin != nil {
				rlAdmin, err := solana.PublicKeyFromBase58(*input.RateLimitAdmin)
				if err != nil {
					return sequences.OnChainOutput{}, fmt.Errorf("invalid rate limit admin address for chain %d: %s: %w", input.Selector, *input.RateLimitAdmin, err)
				}
				out, err := operations.ExecuteOperation(b, rlAdminOp, chain, tokenpoolops.TokenPoolTransferOwnershipInput{
					Program:   tokenPool,
					TokenMint: tokenMint,
					NewOwner:  rlAdmin,
				})
				if err != nil {
					return sequences.OnChainOutput{}, fmt.Errorf("failed to set rate limit admin on token pool %s on chain %d: %w", tokenPool, input.Selector, err)
				}
				result.BatchOps = append(result.BatchOps, out.Output.BatchOps...)
			}
			return result, nil
		},
	)
}

// GetSupportedChains returns the remote chains the pool is configured for. Solana pools keep each
// remote chain config in a PDA keyed by (selector, mint, program) with no on-chain list, and the
// account does not store its selector, so candidates are probed: every selector in the
// chain-selectors registry (all families), plus any chain in the environment or the datastore.
// That way a lane to a chain that is no longer in the environment or the datastore is still found.
func (a *SolanaAdapter) GetSupportedChains(e deployment.Environment, chainSelector uint64, poolAddr, tokenAddr []byte) ([]uint64, error) {
	chain, ok := e.BlockChains.SolanaChains()[chainSelector]
	if !ok {
		return nil, fmt.Errorf("chain with selector %d not defined", chainSelector)
	}
	poolProgramID, mint, err := a.resolvePoolProgram(e, chainSelector, poolAddr, tokenAddr)
	if err != nil {
		return nil, err
	}

	candidates := make(map[uint64]struct{})
	for _, sel := range chainsel.AllSelectors() {
		candidates[sel] = struct{}{}
	}
	delete(candidates, chainSelector)
	for _, sel := range e.BlockChains.ListChainSelectors(cldf_chain.WithChainSelectorsExclusion([]uint64{chainSelector})) {
		candidates[sel] = struct{}{}
	}
	if e.DataStore != nil {
		for _, ref := range e.DataStore.Addresses().Filter() {
			if ref.ChainSelector != chainSelector {
				candidates[ref.ChainSelector] = struct{}{}
			}
		}
	}
	sels := slices.Sorted(maps.Keys(candidates))
	pdas := make([]solana.PublicKey, len(sels))
	for i, sel := range sels {
		if pdas[i], _, err = tokens.TokenPoolChainConfigPDA(sel, mint, poolProgramID); err != nil {
			return nil, fmt.Errorf("failed to derive remote chain config PDA for chain %d: %w", sel, err)
		}
	}

	// A chain is supported when its config account exists and is owned by the pool program. The
	// account is not decoded, so a layout change cannot hide a configured chain; the owner check
	// rules out a system account created by sending lamports to the PDA.
	const maxAccountsPerCall = 100
	supported := []uint64{}
	for start := 0; start < len(pdas); start += maxAccountsPerCall {
		end := min(start+maxAccountsPerCall, len(pdas))
		resp, err := chain.Client.GetMultipleAccountsWithOpts(e.GetContext(), pdas[start:end], &rpc.GetMultipleAccountsOpts{Commitment: cldf_solana.SolDefaultCommitment})
		if err != nil {
			return nil, fmt.Errorf("failed to check remote chain configs for pool %s on chain %d: %w", poolProgramID, chainSelector, err)
		}
		for i, acct := range resp.Value {
			if acct != nil && acct.Owner.Equals(poolProgramID) {
				supported = append(supported, sels[start+i])
			}
		}
	}
	return supported, nil
}

// GetRemoteToken returns the remote token the pool uses for remoteSelector.
func (a *SolanaAdapter) GetRemoteToken(e deployment.Environment, chainSelector uint64, poolAddr, tokenAddr []byte, remoteSelector uint64) ([]byte, error) {
	cfg, found, err := a.remoteChainConfig(e, chainSelector, poolAddr, tokenAddr, remoteSelector)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("remote chain %d is not configured on the pool on chain %d", remoteSelector, chainSelector)
	}
	return cfg.Remote.TokenAddress.Address, nil
}

// GetRemotePools returns the remote pools the pool is linked to for remoteSelector. A pool may have
// more than one during a remote-side upgrade; an unconfigured remote chain returns none.
func (a *SolanaAdapter) GetRemotePools(e deployment.Environment, chainSelector uint64, poolAddr, tokenAddr []byte, remoteSelector uint64) ([][]byte, error) {
	cfg, found, err := a.remoteChainConfig(e, chainSelector, poolAddr, tokenAddr, remoteSelector)
	if err != nil {
		return nil, err
	}
	pools := make([][]byte, 0, len(cfg.Remote.PoolAddresses))
	if !found {
		return pools, nil
	}
	for _, addr := range cfg.Remote.PoolAddresses {
		pools = append(pools, addr.Address)
	}
	return pools, nil
}

func (a *SolanaAdapter) remoteChainConfig(e deployment.Environment, chainSelector uint64, poolAddr, tokenAddr []byte, remoteSelector uint64) (burnmint_token_pool.BaseChain, bool, error) {
	chain, ok := e.BlockChains.SolanaChains()[chainSelector]
	if !ok {
		return burnmint_token_pool.BaseChain{}, false, fmt.Errorf("chain with selector %d not defined", chainSelector)
	}
	poolProgramID, mint, err := a.resolvePoolProgram(e, chainSelector, poolAddr, tokenAddr)
	if err != nil {
		return burnmint_token_pool.BaseChain{}, false, err
	}
	return tokenpoolops.GetRemoteChainConfig(e.OperationsBundle, chain, poolTypeOf(e.DataStore, chainSelector, poolProgramID), poolProgramID, mint, remoteSelector)
}

// poolTypeOf returns the datastore type of a pool program, which selects the chain config layout to
// decode. A program missing from the datastore is decoded as a BurnMint or LockRelease pool, which
// share a layout; CCTP and Lombard pools are always deployed through the tooling, so they have a ref.
func poolTypeOf(ds datastore.DataStore, chainSelector uint64, program solana.PublicKey) string {
	if ds == nil {
		return ""
	}
	ref, err := latestRefByAddress(ds, chainSelector, program.String())
	if err != nil {
		return ""
	}
	return ref.Type.String()
}

// resolvePoolProgram resolves poolAddr to the pool program ID, which the chain config PDAs are
// derived from. The chain-agnostic TokenPoolMigrator callers may pass either form of a Solana pool
// address:
//   - the pool program ID (the datastore and TokenAdminRegistry form), returned as is;
//   - a pool config PDA, whose program is read from chain. Its mint must match tokenAddr.
//
// The mint (tokenAddr) is returned too, because the chain config PDAs are also keyed by mint.
func (a *SolanaAdapter) resolvePoolProgram(e deployment.Environment, chainSelector uint64, poolAddr, tokenAddr []byte) (solana.PublicKey, solana.PublicKey, error) {
	if len(poolAddr) != solana.PublicKeyLength || len(tokenAddr) != solana.PublicKeyLength {
		return solana.PublicKey{}, solana.PublicKey{}, fmt.Errorf("pool and token addresses must be %d bytes on chain %d", solana.PublicKeyLength, chainSelector)
	}
	chain, ok := e.BlockChains.SolanaChains()[chainSelector]
	if !ok {
		return solana.PublicKey{}, solana.PublicKey{}, fmt.Errorf("chain with selector %d not defined", chainSelector)
	}
	pool, mint := solana.PublicKeyFromBytes(poolAddr), solana.PublicKeyFromBytes(tokenAddr)
	if mint.IsZero() {
		return solana.PublicKey{}, solana.PublicKey{}, fmt.Errorf("invalid token mint address bytes for chain %d", chainSelector)
	}
	acct, err := chain.Client.GetAccountInfoWithOpts(e.GetContext(), pool, &rpc.GetAccountInfoOpts{Commitment: cldf_solana.SolDefaultCommitment})
	if err != nil {
		return solana.PublicKey{}, solana.PublicKey{}, fmt.Errorf("failed to read pool account %s: %w", pool, err)
	}
	if acct == nil || acct.Value == nil {
		return solana.PublicKey{}, solana.PublicKey{}, fmt.Errorf("pool account %s not found", pool)
	}
	if acct.Value.Executable {
		return pool, mint, nil
	}
	cfg, err := poolFromConfigPDA(e.GetContext(), chain, pool.String())
	if err != nil {
		return solana.PublicKey{}, solana.PublicKey{}, err
	}
	if cfg.mint != mint {
		return solana.PublicKey{}, solana.PublicKey{}, fmt.Errorf("pool config %s is for mint %s, not %s", cfg.configPDA, cfg.mint, mint)
	}
	return cfg.program, cfg.mint, nil
}

// GetOnchainRateLimits reads the rate limits of a pool's lane. The mint is needed to derive the
// remote chain config PDA. With fastFinality, the buckets come from the lane's FTF override
// account instead of the base chain config.
func (a *SolanaAdapter) GetOnchainRateLimits(
	b operations.Bundle,
	chains cldf_chain.BlockChains,
	ds datastore.DataStore,
	chainSelector uint64,
	poolRef datastore.AddressRef,
	tokenRef datastore.AddressRef,
	remoteSelector uint64,
	fastFinality bool,
) (tokenapi.OnchainRateLimits, error) {
	chain, ok := chains.SolanaChains()[chainSelector]
	if !ok {
		return tokenapi.OnchainRateLimits{}, fmt.Errorf("chain with selector %d not defined", chainSelector)
	}
	fullTokenRef, err := a.resolveTokenRefWithFallback(b, chains, ds, chainSelector, tokenRef)
	if err != nil {
		return tokenapi.OnchainRateLimits{}, err
	}
	fullPoolRef, err := datastore_utils.FindAndFormatRef(ds, poolRef, chainSelector, datastore_utils.FullRef)
	if err != nil {
		return tokenapi.OnchainRateLimits{}, fmt.Errorf("failed to resolve pool ref (%s): %w", datastore_utils.SprintRef(poolRef), err)
	}
	tokenPool, tokenMint, err := poolAndMint(chainSelector, fullPoolRef, fullTokenRef)
	if err != nil {
		return tokenapi.OnchainRateLimits{}, err
	}
	var outbound, inbound burnmint_token_pool.RateLimitTokenBucket
	var found bool
	if fastFinality {
		var cfg burnmint_token_pool.ChainConfigOverride
		cfg, found, err = tokenpoolops.GetRemoteChainConfigOverride(b, chain, tokenPool, tokenMint, remoteSelector)
		outbound, inbound = cfg.Base.OutboundRateLimit, cfg.Base.InboundRateLimit
	} else {
		var cfg burnmint_token_pool.BaseChain
		cfg, found, err = tokenpoolops.GetRemoteChainConfig(b, chain, fullPoolRef.Type.String(), tokenPool, tokenMint, remoteSelector)
		outbound, inbound = cfg.OutboundRateLimit, cfg.InboundRateLimit
	}
	if err != nil {
		return tokenapi.OnchainRateLimits{}, err
	}
	if !found {
		// Lane (or its FTF override) not configured yet: zero configs, as RateLimitReaderAdapter specifies.
		disabled := tokenapi.RateLimiterConfig{IsEnabled: false, Capacity: big.NewInt(0), Rate: big.NewInt(0)}
		return tokenapi.OnchainRateLimits{Outbound: disabled, Inbound: disabled}, nil
	}
	return tokenapi.OnchainRateLimits{
		Outbound: toRateLimiterConfig(outbound.Cfg),
		Inbound:  toRateLimiterConfig(inbound.Cfg),
	}, nil
}

// ResolveTokenPoolRef resolves a pool address that is either the pool program ID or the pool
// config PDA. For a PDA, the PDA is kept in an artificial label so DeriveTokenAddress can read the
// mint from it.
func (a *SolanaAdapter) ResolveTokenPoolRef(b operations.Bundle, chains cldf_chain.BlockChains, ds datastore.DataStore, chainSelector uint64, address string) (datastore.AddressRef, error) {
	chain, ok := chains.SolanaChains()[chainSelector]
	if !ok {
		return datastore.AddressRef{}, fmt.Errorf("chain with selector %d not defined", chainSelector)
	}
	pubKey, err := solana.PublicKeyFromBase58(address)
	if err != nil {
		return datastore.AddressRef{}, fmt.Errorf("invalid pool address %q: %w", address, err)
	}
	acct, err := chain.Client.GetAccountInfoWithOpts(b.GetContext(), pubKey, &rpc.GetAccountInfoOpts{Commitment: cldf_solana.SolDefaultCommitment})
	if err != nil {
		return datastore.AddressRef{}, fmt.Errorf("get account info for %q: %w", address, err)
	}
	if acct == nil || acct.Value == nil {
		return datastore.AddressRef{}, fmt.Errorf("account %q not found", address)
	}
	poolProgramID, poolPDA := pubKey, (*solana.PublicKey)(nil)
	if !acct.Value.Executable {
		poolProgramID, poolPDA = acct.Value.Owner, &pubKey
	}

	poolRef, err := latestRefByAddress(ds, chainSelector, poolProgramID.String())
	if err != nil {
		return datastore.AddressRef{}, fmt.Errorf("failed to find token pool program ref for program id %s: %w", poolProgramID, err)
	}
	if poolPDA != nil {
		poolRef.Labels = datastore.NewLabelSet(append(poolRef.Labels.List(), encodeArtificialPoolAddressRefLabel(*poolPDA))...)
	}
	return poolRef, nil
}

// ResolveTokenRef builds a token ref from the on-chain mint. The qualifier is the Metaplex symbol
// when available, otherwise "<mint>-<token program type>".
func (a *SolanaAdapter) ResolveTokenRef(b operations.Bundle, chains cldf_chain.BlockChains, _ datastore.DataStore, chainSelector uint64, address string) (datastore.AddressRef, error) {
	chain, ok := chains.SolanaChains()[chainSelector]
	if !ok {
		return datastore.AddressRef{}, fmt.Errorf("chain with selector %d not defined", chainSelector)
	}
	mint, err := solana.PublicKeyFromBase58(address)
	if err != nil {
		return datastore.AddressRef{}, fmt.Errorf("invalid mint address %q: %w", address, err)
	}
	// FetchTokenProgramID also checks the account is a mint, not a token account or multisig.
	tokenProgram, err := solcommon.FetchTokenProgramID(b.GetContext(), chain, mint)
	if err != nil {
		return datastore.AddressRef{}, fmt.Errorf("mint %s: %w", address, err)
	}
	programType, err := tokenProgramType(tokenProgram)
	if err != nil {
		return datastore.AddressRef{}, fmt.Errorf("mint %s: %w", address, err)
	}

	// The symbol is best effort: not every mint has Metaplex metadata.
	qualifier := fmt.Sprintf("%s-%s", address, programType)
	if meta, _, err := tokens.GetTokenMetadata(b.GetContext(), chain.Client, mint); err != nil {
		b.Logger.Warnf("failed to get Metaplex token metadata for mint %s, using qualifier %s: %v", address, qualifier, err)
	} else if symbol := strings.TrimSpace(tokens.GetTokenDataV2(meta).Symbol); symbol != "" {
		qualifier = symbol
	}

	return datastore.AddressRef{
		ChainSelector: chainSelector,
		Type:          datastore.ContractType(programType),
		Version:       tokensops.Version,
		Qualifier:     qualifier,
		Address:       mint.String(),
	}, nil
}

// derivePoolMintFromPDA reads the mint from a pool config PDA, given in the pool ref's artificial
// label or in its Address field. ok is false when the ref carries no pool PDA. Only a PDA given
// in the label is required to decode; an Address that is not a pool PDA is ignored.
func (a *SolanaAdapter) derivePoolMintFromPDA(ctx context.Context, chain cldf_solana.Chain, poolRef datastore.AddressRef) (solana.PublicKey, bool, error) {
	for _, label := range poolRef.Labels.List() {
		if pda, found := decodeArtificialPoolAddressRef(label); found {
			mint, err := readPoolMint(ctx, chain, pda)
			if err != nil {
				return solana.PublicKey{}, false, fmt.Errorf("failed to decode pool config PDA %s: %w", pda, err)
			}
			return mint, true, nil
		}
	}
	mint, ok := poolMintFromAddress(ctx, chain, poolRef.Address)
	return mint, ok, nil
}

// readPoolMint reads the mint from a pool config PDA of any pool type. BurnMint, LockRelease, CCTP
// and Lombard pools all name their config account State, so they share the account discriminator,
// and all start it with version and BaseConfig. CCTP and Lombard append extra fields, which the
// BurnMint decoder ignores.
func readPoolMint(ctx context.Context, chain cldf_solana.Chain, pda solana.PublicKey) (solana.PublicKey, error) {
	state, err := common.GetParsedAccountData(ctx, chain.Client, pda, cldf_solana.SolDefaultCommitment, burnmint_token_pool.ParseAccount_State)
	if err != nil {
		return solana.PublicKey{}, err
	}
	return state.Config.Mint, nil
}

// poolMintFromAddress returns the mint when address is a pool config PDA. ok is false for
// anything else (empty, not base58, missing, a program, or not a pool state account).
func poolMintFromAddress(ctx context.Context, chain cldf_solana.Chain, address string) (solana.PublicKey, bool) {
	if address == "" {
		return solana.PublicKey{}, false
	}
	addr, err := solana.PublicKeyFromBase58(address)
	if err != nil {
		return solana.PublicKey{}, false
	}
	acct, err := chain.Client.GetAccountInfoWithOpts(ctx, addr, &rpc.GetAccountInfoOpts{Commitment: cldf_solana.SolDefaultCommitment})
	if err != nil || acct == nil || acct.Value == nil || acct.Value.Executable {
		return solana.PublicKey{}, false
	}
	mint, err := readPoolMint(ctx, chain, addr)
	if err != nil || mint.IsZero() {
		return solana.PublicKey{}, false
	}
	return mint, true
}

func (a *SolanaAdapter) resolveTokenRefWithFallback(b operations.Bundle, chains cldf_chain.BlockChains, ds datastore.DataStore, chainSelector uint64, tokenRef datastore.AddressRef) (datastore.AddressRef, error) {
	ref, err := solcommon.FindLatestRef(ds, tokenRef, chainSelector)
	if err == nil {
		return ref, nil
	}
	if tokenRef.Address == "" {
		return datastore.AddressRef{}, fmt.Errorf("token ref %s is not in the datastore and has no address to resolve on-chain: %w", datastore_utils.SprintRef(tokenRef), err)
	}
	return a.ResolveTokenRef(b, chains, ds, chainSelector, tokenRef.Address)
}

// latestRefByAddress returns the datastore ref for address. A pool program can be registered under
// several versions; the highest one wins.
func latestRefByAddress(ds datastore.DataStore, chainSelector uint64, address string) (datastore.AddressRef, error) {
	refs := ds.Addresses().Filter(
		datastore.AddressRefByChainSelector(chainSelector),
		datastore.AddressRefByAddress(address),
	)
	if len(refs) == 0 {
		return datastore.AddressRef{}, fmt.Errorf("no ref found for address %s on chain %d", address, chainSelector)
	}
	best := refs[0]
	for _, ref := range refs[1:] {
		if ref.Version != nil && (best.Version == nil || ref.Version.GreaterThan(best.Version)) {
			best = ref
		}
	}
	return best, nil
}

func poolAndMint(chainSelector uint64, poolRef, tokenRef datastore.AddressRef) (solana.PublicKey, solana.PublicKey, error) {
	tokenPool, err := solana.PublicKeyFromBase58(poolRef.Address)
	if err != nil {
		return solana.PublicKey{}, solana.PublicKey{}, fmt.Errorf("invalid token pool address for chain %d: %s: %w", chainSelector, poolRef.Address, err)
	}
	tokenMint, err := solana.PublicKeyFromBase58(tokenRef.Address)
	if err != nil {
		return solana.PublicKey{}, solana.PublicKey{}, fmt.Errorf("invalid token mint address for chain %d: %s: %w", chainSelector, tokenRef.Address, err)
	}
	return tokenPool, tokenMint, nil
}

func remoteAddressToBytes(remoteSelector uint64, address string) ([]byte, error) {
	family, err := chainsel.GetSelectorFamily(remoteSelector)
	if err != nil {
		return nil, fmt.Errorf("invalid remote chain selector %d: %w", remoteSelector, err)
	}
	normalizer, ok := deployapi.GetAddressNormalizerRegistry().GetAddressNormalizer(family)
	if !ok {
		return nil, fmt.Errorf("no address normalizer registered for chain family %s (remote chain %d)", family, remoteSelector)
	}
	raw, err := normalizer.StringToBytes(address)
	if err != nil {
		return nil, fmt.Errorf("invalid remote pool address for chain %d: %s: %w", remoteSelector, address, err)
	}
	return raw, nil
}

func tokenProgramType(owner solana.PublicKey) (string, error) {
	switch owner {
	case solana.TokenProgramID:
		return solcommon.SPLTokens.String(), nil
	case solana.Token2022ProgramID:
		return solcommon.SPL2022Tokens.String(), nil
	default:
		return "", fmt.Errorf("account is owned by %s, which is not an SPL token program", owner)
	}
}

func toRateLimiterConfig(cfg burnmint_token_pool.RateLimitConfig) tokenapi.RateLimiterConfig {
	return tokenapi.RateLimiterConfig{
		IsEnabled: cfg.Enabled,
		Capacity:  new(big.Int).SetUint64(cfg.Capacity),
		Rate:      new(big.Int).SetUint64(cfg.Rate),
	}
}

func decodeArtificialPoolAddressRef(label string) (solana.PublicKey, bool) {
	prefix, value, found := strings.Cut(label, ":")
	if !found || prefix != tokenapi.ArtificialAddressRefLabel {
		return solana.PublicKey{}, false
	}
	pubKey, err := solana.PublicKeyFromBase58(value)
	if err != nil {
		return solana.PublicKey{}, false
	}
	return pubKey, true
}

// encodeArtificialPoolAddressRefLabel builds the "ArtificialAddressRef:<pool config PDA>" label. It
// is unrelated to the "<mint>-<token program type>" token qualifier built by ResolveTokenRef.
func encodeArtificialPoolAddressRefLabel(poolPDA solana.PublicKey) string {
	return tokenapi.ArtificialAddressRefLabel + ":" + poolPDA.String()
}
