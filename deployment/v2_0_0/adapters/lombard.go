package adapters

import (
	"fmt"

	"github.com/gagliardetto/solana-go"

	datastore_utils "github.com/smartcontractkit/chainlink-ccip/deployment/utils/datastore"
	seq_core "github.com/smartcontractkit/chainlink-ccip/deployment/utils/sequences"
	ccipadapters "github.com/smartcontractkit/chainlink-ccip/deployment/v2_0_0/adapters"
	cldf_chain "github.com/smartcontractkit/chainlink-deployments-framework/chain"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	cldf_deployment "github.com/smartcontractkit/chainlink-deployments-framework/deployment"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/tokens"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	tokenpoolops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/tokenpools"
	solseq "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/sequences"
)

var _ ccipadapters.LombardChain = &LombardChainAdapter{}

// LombardChainAdapter registers Solana with the upstream, Lombard-specific chain registry
// (chainlink-ccip/deployment/v2_0_0/adapters/lombard.go's LombardChainRegistry) - a separate,
// purpose-built registry from the generic per-chain-family CCV adapter registries in init.go, so
// registering here doesn't collide with the existing committee_verifier registration for Solana.
type LombardChainAdapter struct{}

// AddressRefToBytes returns the byte representation of a Solana address (a 32-byte Pubkey).
func (a *LombardChainAdapter) AddressRefToBytes(ref datastore.AddressRef) ([]byte, error) {
	pk, err := solana.PublicKeyFromBase58(ref.Address)
	if err != nil {
		return nil, fmt.Errorf("parse address %q: %w", ref.Address, err)
	}
	return pk.Bytes(), nil
}

// DeployLombardChain returns the sequence that deploys/initializes the lombard_token_pool
// program and its embedded verifier on a Solana chain.
func (a *LombardChainAdapter) DeployLombardChain() *operations.Sequence[ccipadapters.DeployLombardInput, seq_core.OnChainOutput, ccipadapters.DeployLombardChainDeps] {
	return solseq.DeployLombardChain
}

// ConfigureLombardChainForLanes returns the sequence that configures the pool/verifier for a
// set of remote chains.
func (a *LombardChainAdapter) ConfigureLombardChainForLanes() *operations.Sequence[ccipadapters.ConfigureLombardChainForLanesInput, seq_core.OnChainOutput, ccipadapters.ConfigureLombardChainForLanesDeps] {
	return solseq.ConfigureLombardChainForLanes
}

// AllowedCallerOnDest returns the address the remote verifier writes as the Lombard message's
// destination caller - on Solana that's the verifier config PDA, which the pool signs as the
// mailbox handler. It is one per program, so a single value serves every token.
func (a *LombardChainAdapter) AllowedCallerOnDest(ds datastore.DataStore, _ cldf_chain.BlockChains, selector uint64) ([]byte, error) {
	pool, err := a.tokenPool(ds, selector, "")
	if err != nil {
		return nil, fmt.Errorf("failed to get token pool: %w", err)
	}
	poolAddr, err := solcommon.ToAddress(pool)
	if err != nil {
		return nil, fmt.Errorf("failed to parse token pool address: %w", err)
	}
	verifierConfigPDA, _, err := state.FindLVConfigPDA(poolAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to derive verifier config PDA: %w", err)
	}
	return verifierConfigPDA.Bytes(), nil
}

// RemoteTokenAddress returns the SPL mint address for this chain's Lombard token, looked up by
// qualifier - Solana's DeployToken sequence registers the mint under its actual SPL program type
// (SPLTokens or SPL2022Tokens), not a generic "Token" type, unlike EVM's adapter which reads the
// token address on-chain off the pool contract instead of the datastore.
func (a *LombardChainAdapter) RemoteTokenAddress(_ operations.Bundle, ds datastore.DataStore, _ cldf_chain.BlockChains, selector uint64, tokenQualifier string) ([]byte, error) {
	for _, contractType := range []cldf_deployment.ContractType{solcommon.SPLTokens, solcommon.SPL2022Tokens} {
		ref, err := solcommon.FindLatestRef(ds, datastore.AddressRef{
			Type:      datastore.ContractType(contractType),
			Qualifier: tokenQualifier,
		}, selector)
		if err == nil {
			return a.AddressRefToBytes(ref)
		}
	}
	return nil, fmt.Errorf("failed to find SPL token with qualifier %q on chain %d", tokenQualifier, selector)
}

// RemoteTokenPoolAddress returns the address a remote chain should register as this chain's "pool" for this token: the per-token pool state PDA
func (a *LombardChainAdapter) RemoteTokenPoolAddress(ds datastore.DataStore, _ cldf_chain.BlockChains, selector uint64, tokenQualifier string) ([]byte, error) {
	pool, err := a.tokenPool(ds, selector, tokenQualifier)
	if err != nil {
		return nil, fmt.Errorf("failed to get token pool: %w", err)
	}
	poolAddr, err := solcommon.ToAddress(pool)
	if err != nil {
		return nil, fmt.Errorf("failed to parse token pool address: %w", err)
	}

	mintBytes, err := a.RemoteTokenAddress(operations.Bundle{}, ds, cldf_chain.BlockChains{}, selector, tokenQualifier)
	if err != nil {
		return nil, fmt.Errorf("failed to get token mint: %w", err)
	}
	mintAddr, err := solcommon.BytesToPubkey(mintBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse token mint address: %w", err)
	}

	poolConfigPDA, err := tokens.TokenPoolConfigAddress(mintAddr, poolAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to derive pool config PDA: %w", err)
	}
	return poolConfigPDA.Bytes(), nil
}

// RemoteTokenDecimals returns the decimals of the SPL mint for this chain's Lombard token.
func (a *LombardChainAdapter) RemoteTokenDecimals(b operations.Bundle, ds datastore.DataStore, chains cldf_chain.BlockChains, selector uint64, tokenQualifier string) (uint8, error) {
	chain, ok := chains.SolanaChains()[selector]
	if !ok {
		return 0, fmt.Errorf("solana chain with selector %d not found", selector)
	}
	mintBytes, err := a.RemoteTokenAddress(b, ds, chains, selector, tokenQualifier)
	if err != nil {
		return 0, fmt.Errorf("failed to get token mint: %w", err)
	}
	return solcommon.GetTokenDecimals(chain, solana.PublicKeyFromBytes(mintBytes))
}

func (a *LombardChainAdapter) tokenPool(ds datastore.DataStore, selector uint64, tokenQualifier string) (datastore.AddressRef, error) {
	ref := datastore.AddressRef{
		Type:      datastore.ContractType(tokenpoolops.LombardTokenPool),
		Version:   solcommon.Version2_0_0,
		Qualifier: solcommon.PoolProgramQualifier,
	}
	_ = tokenQualifier // the pool program is one shared instance per chain, not per-token
	return solcommon.FindAndFormatCompatibleRef(ds, ref, selector, datastore_utils.FullRef)
}
