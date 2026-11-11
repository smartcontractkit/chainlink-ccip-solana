package adapters

import (
	"errors"
	"fmt"

	"github.com/Masterminds/semver/v3"
	"github.com/gagliardetto/solana-go"

	common_utils "github.com/smartcontractkit/chainlink-ccip/deployment/utils"
	datastore_utils "github.com/smartcontractkit/chainlink-ccip/deployment/utils/datastore"
	seq_core "github.com/smartcontractkit/chainlink-ccip/deployment/utils/sequences"
	ccipadapters "github.com/smartcontractkit/chainlink-ccip/deployment/v2_0_0/adapters"
	cldf_chain "github.com/smartcontractkit/chainlink-deployments-framework/chain"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	cldf_deployment "github.com/smartcontractkit/chainlink-deployments-framework/deployment"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/tokens"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	cctpverifierops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/cctpverifier"
	tokenpoolops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/tokenpools"
	solseq "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/sequences"
)

var _ ccipadapters.CCTPChain = &CCTPChainAdapter{}

// USDCTokenQualifier is the datastore qualifier under which the chain's USDC mint is registered.
// The upstream RemoteCCTPChain interface passes no qualifier to TokenAddress, so the adapter
// looks the mint up by this well-known name before falling back to a wildcard match.
const USDCTokenQualifier = solcommon.USDCTokenQualifier

// CCTPChainAdapter registers Solana with the upstream CCTPChainRegistry
// (chainlink-ccip/deployment/v2_0_0/adapters/cctp.go). That registry is keyed by chain family and
// USDC type, and has no global singleton getter, so registration happens at the call site rather
// than in this package's init.go - see how devenv wires the Lombard equivalent.
type CCTPChainAdapter struct{}

// USDCType reports that Solana's USDC is CCTP-native rather than backed by locked USDC elsewhere.
func (a *CCTPChainAdapter) USDCType() ccipadapters.USDCType {
	return ccipadapters.Canonical
}

// DeployCCTPChain returns the sequence that deploys and initializes the cctp_token_pool program
// and its embedded verifier on a Solana chain.
func (a *CCTPChainAdapter) DeployCCTPChain() *operations.Sequence[ccipadapters.DeployCCTPInput, seq_core.OnChainOutput, ccipadapters.DeployCCTPChainDeps] {
	return solseq.DeployCCTPChain
}

// ConfigureCCTPChainForLanes returns the sequence that configures the pool and verifier for a set
// of remote chains.
func (a *CCTPChainAdapter) ConfigureCCTPChainForLanes() *operations.Sequence[ccipadapters.ConfigureCCTPChainForLanesInput, seq_core.OnChainOutput, ccipadapters.ConfigureCCTPChainForLanesDeps] {
	return solseq.ConfigureCCTPChainForLanes
}

// UpdateAuthorities transfers the pool and verifier authority to the MCMS timelock signer PDA.
// The pool and verifier share one program, so a single pool program address drives both halves.
// Every op is idempotent, and each one batches into the MCMS proposal when its signer is not the
// deployer key, so the returned BatchOps feed the upstream DeployCCTPChains proposal.
func (a *CCTPChainAdapter) UpdateAuthorities() *operations.Sequence[ccipadapters.UpdateAuthoritiesInput, seq_core.OnChainOutput, *cldf_deployment.Environment] {
	return operations.NewSequence(
		"solana-cctp-update-authorities",
		semver.MustParse("2.0.0"),
		"Transfer CCTP pool and verifier authorities to the MCMS timelock signer",
		func(b operations.Bundle, e *cldf_deployment.Environment, input ccipadapters.UpdateAuthoritiesInput) (seq_core.OnChainOutput, error) {
			chain, ok := e.BlockChains.SolanaChains()[input.ChainSelector]
			if !ok {
				return seq_core.OnChainOutput{}, fmt.Errorf("chain with selector %d not defined", input.ChainSelector)
			}
			timelockSigner, err := solcommon.GetTimelockSignerPDA(
				e.DataStore.Addresses().Filter(),
				input.ChainSelector,
				common_utils.CLLQualifier,
			)
			if err != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("resolve timelock signer PDA: %w", err)
			}
			mint, err := a.usdcMint(e.DataStore, input.ChainSelector)
			if err != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("resolve USDC mint: %w", err)
			}
			var pool solana.PublicKey
			for _, ref := range input.ContractRefs {
				if ref.Type.String() == common_utils.CCTPTokenPool.String() {
					pool, err = solana.PublicKeyFromBase58(ref.Address)
					if err != nil {
						return seq_core.OnChainOutput{}, fmt.Errorf("parse cctp token pool address %q: %w", ref.Address, err)
					}
					break
				}
			}
			if pool.IsZero() {
				return seq_core.OnChainOutput{}, fmt.Errorf("no %s ref in ContractRefs for chain %d", common_utils.CCTPTokenPool, input.ChainSelector)
			}

			var result seq_core.OnChainOutput
			merge := func(src seq_core.OnChainOutput) {
				result.Addresses = append(result.Addresses, src.Addresses...)
				result.BatchOps = append(result.BatchOps, src.BatchOps...)
			}

			b.Logger.Infow("Updating rate-limit admin of CCTP pool to timelock signer",
				"pool", pool.String(), "timelockSigner", timelockSigner.String())
			rlAdminOut, err := operations.ExecuteOperation(b, tokenpoolops.UpdateRateLimitAdminCctp, chain, tokenpoolops.TokenPoolTransferOwnershipInput{
				Program:   pool,
				TokenMint: mint,
				NewOwner:  timelockSigner,
			})
			if err != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("update pool rate-limit admin: %w", err)
			}
			merge(rlAdminOut.Output)

			b.Logger.Infow("Transferring ownership of CCTP pool to timelock signer",
				"pool", pool.String(), "timelockSigner", timelockSigner.String())
			transferOut, err := operations.ExecuteOperation(b, tokenpoolops.TransferOwnershipCctp, chain, tokenpoolops.TokenPoolTransferOwnershipInput{
				Program:   pool,
				TokenMint: mint,
				NewOwner:  timelockSigner,
			})
			if err != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("transfer pool ownership: %w", err)
			}
			merge(transferOut.Output)

			b.Logger.Infow("Accepting ownership of CCTP pool by timelock signer",
				"pool", pool.String(), "timelockSigner", timelockSigner.String())
			acceptOut, err := operations.ExecuteOperation(b, tokenpoolops.AcceptOwnershipCctp, chain, tokenpoolops.TokenPoolTransferOwnershipInput{
				Program:   pool,
				TokenMint: mint,
				NewOwner:  timelockSigner,
			})
			if err != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("accept pool ownership: %w", err)
			}
			merge(acceptOut.Output)

			b.Logger.Infow("Transferring verifier ownership to timelock signer",
				"program", pool.String(), "timelockSigner", timelockSigner.String())
			verifierTransferOut, err := operations.ExecuteOperation(b, cctpverifierops.TransferOwnership, chain, cctpverifierops.TransferOwnershipParams{
				CctpVerifier:  pool,
				ProposedOwner: timelockSigner,
			})
			if err != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("transfer verifier ownership: %w", err)
			}
			merge(verifierTransferOut.Output)

			b.Logger.Infow("Accepting verifier ownership by timelock signer",
				"program", pool.String(), "timelockSigner", timelockSigner.String())
			verifierAcceptOut, err := operations.ExecuteOperation(b, cctpverifierops.AcceptOwnership, chain, cctpverifierops.AcceptOwnershipParams{
				CctpVerifier: pool,
			})
			if err != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("accept verifier ownership: %w", err)
			}
			merge(verifierAcceptOut.Output)

			b.Logger.Infow("Transferring verifier storage-location admin to timelock signer",
				"program", pool.String(), "timelockSigner", timelockSigner.String())
			storageAdminTransferOut, err := operations.ExecuteOperation(b, cctpverifierops.TransferStorageLocationAdmin, chain, cctpverifierops.TransferStorageLocationAdminParams{
				CctpVerifier:                 pool,
				ProposedStorageLocationAdmin: timelockSigner,
			})
			if err != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("transfer verifier storage-location admin: %w", err)
			}
			merge(storageAdminTransferOut.Output)

			b.Logger.Infow("Accepting verifier storage-location admin by timelock signer",
				"program", pool.String(), "timelockSigner", timelockSigner.String())
			storageAdminAcceptOut, err := operations.ExecuteOperation(b, cctpverifierops.AcceptStorageLocationAdmin, chain, cctpverifierops.Params{
				CctpVerifier: pool,
			})
			if err != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("accept verifier storage-location admin: %w", err)
			}
			merge(storageAdminAcceptOut.Output)

			return result, nil
		})
}

// MigrateHybridLockReleaseLiquidity satisfies the CCTPChain interface but always fails: migrating
// from a HybridLockReleaseUSDCTokenPool into siloed lockboxes is an EVM-only concern, and neither
// pool type exists on Solana. Failing loudly beats silently reporting success for a migration
// that never happened.
func (a *CCTPChainAdapter) MigrateHybridLockReleaseLiquidity() *operations.Sequence[ccipadapters.MigrateHybridLockReleaseLiquidityInput, seq_core.OnChainOutput, ccipadapters.MigrateHybridLockReleaseLiquidityDeps] {
	return operations.NewSequence(
		"solana-migrate-hybrid-lock-release-liquidity",
		semver.MustParse("2.0.0"),
		"Unsupported on Solana",
		func(_ operations.Bundle, _ ccipadapters.MigrateHybridLockReleaseLiquidityDeps, _ ccipadapters.MigrateHybridLockReleaseLiquidityInput) (seq_core.OnChainOutput, error) {
			return seq_core.OnChainOutput{}, errors.New(
				"MigrateHybridLockReleaseLiquidity is not supported on Solana: there is no HybridLockRelease USDC pool")
		},
	)
}

// TokenAddress returns the chain's USDC mint. Solana's DeployToken registers mints under their
// actual SPL program type (SPLTokens or SPL2022Tokens) rather than a generic "Token" type, so
// both are tried.
func (a *CCTPChainAdapter) TokenAddress(ds datastore.DataStore, _ cldf_chain.BlockChains, chainSelector uint64) ([]byte, error) {
	mint, err := a.usdcMint(ds, chainSelector)
	if err != nil {
		return nil, err
	}
	return mint.Bytes(), nil
}

// TokenDecimals resolves a token's decimals from its SPL mint account, rather than trusting a
// configured value.
func (a *CCTPChainAdapter) TokenDecimals(_ operations.Bundle, _ datastore.DataStore, chains cldf_chain.BlockChains, chainSelector uint64, token string) (uint8, error) {
	solChain, ok := chains.SolanaChains()[chainSelector]
	if !ok {
		return 0, fmt.Errorf("solana chain with selector %d not found", chainSelector)
	}
	mint, err := solana.PublicKeyFromBase58(token)
	if err != nil {
		return 0, fmt.Errorf("parse token address %q: %w", token, err)
	}
	return solcommon.GetTokenDecimals(solChain, mint)
}

// PoolAddress returns the address a remote chain should register as this chain's pool for USDC:
// the per-mint pool state PDA, not the shared pool program.
func (a *CCTPChainAdapter) PoolAddress(ds datastore.DataStore, _ cldf_chain.BlockChains, chainSelector uint64, _ datastore.AddressRef) ([]byte, error) {
	poolProgram, mint, err := a.poolAndMint(ds, chainSelector)
	if err != nil {
		return nil, err
	}
	poolStatePDA, err := tokens.TokenPoolConfigAddress(mint, poolProgram)
	if err != nil {
		return nil, fmt.Errorf("derive pool state PDA on chain %d: %w", chainSelector, err)
	}
	return poolStatePDA.Bytes(), nil
}

// CCTPV1AllowedCallerOnDest returns the account allowed to trigger CCTP v1 message reception on
// Solana: the pool signer PDA, which is the CPI signer for release_or_mint_tokens.
func (a *CCTPChainAdapter) CCTPV1AllowedCallerOnDest(ds datastore.DataStore, _ cldf_chain.BlockChains, chainSelector uint64) ([]byte, error) {
	return a.poolSignerBytes(ds, chainSelector)
}

// CCTPV2AllowedCallerOnDest returns the account allowed to trigger CCTP v2 message reception on
// Solana. The verifier deliberately reuses the pool signer PDA as its CPI signer so that the SOL
// staked for CCTP event-account rent lives in one account, so this is the same address as the v1
// caller.
func (a *CCTPChainAdapter) CCTPV2AllowedCallerOnDest(ds datastore.DataStore, _ cldf_chain.BlockChains, chainSelector uint64) ([]byte, error) {
	return a.poolSignerBytes(ds, chainSelector)
}

// AllowedCallerOnSource returns the account that deposits tokens for burn on Solana: the pool
// signer PDA signs deposit_for_burn_with_hook, so it is the messageSender a remote chain must
// accept on an inbound transfer.
func (a *CCTPChainAdapter) AllowedCallerOnSource(ds datastore.DataStore, _ cldf_chain.BlockChains, chainSelector uint64) ([]byte, error) {
	return a.poolSignerBytes(ds, chainSelector)
}

// MintRecipientOnDest returns the pool's associated token account. Solana cannot have CCTP mint
// straight to the CCIP receiver: the verifier snapshots the pool ATA's balance around the
// receive_message CPI and forwards exactly the delta, so it asserts that CCTP minted into that
// account.
func (a *CCTPChainAdapter) MintRecipientOnDest(ds datastore.DataStore, _ cldf_chain.BlockChains, chainSelector uint64) ([]byte, error) {
	poolProgram, mint, err := a.poolAndMint(ds, chainSelector)
	if err != nil {
		return nil, err
	}
	poolSigner, err := tokens.TokenPoolSignerAddress(mint, poolProgram)
	if err != nil {
		return nil, fmt.Errorf("derive pool signer on chain %d: %w", chainSelector, err)
	}
	// The verifier's account resolvers hardcode the SPL Token program when deriving this ATA.
	poolTokenAccount, _, err := tokens.FindAssociatedTokenAddress(solana.TokenProgramID, mint, poolSigner)
	if err != nil {
		return nil, fmt.Errorf("derive pool token account on chain %d: %w", chainSelector, err)
	}
	return poolTokenAccount.Bytes(), nil
}

func (a *CCTPChainAdapter) poolSignerBytes(ds datastore.DataStore, chainSelector uint64) ([]byte, error) {
	poolProgram, mint, err := a.poolAndMint(ds, chainSelector)
	if err != nil {
		return nil, err
	}
	poolSigner, err := tokens.TokenPoolSignerAddress(mint, poolProgram)
	if err != nil {
		return nil, fmt.Errorf("derive pool signer on chain %d: %w", chainSelector, err)
	}
	return poolSigner.Bytes(), nil
}

func (a *CCTPChainAdapter) poolAndMint(ds datastore.DataStore, chainSelector uint64) (poolProgram, mint solana.PublicKey, err error) {
	if poolProgram, err = a.poolProgram(ds, chainSelector); err != nil {
		return solana.PublicKey{}, solana.PublicKey{}, err
	}
	if mint, err = a.usdcMint(ds, chainSelector); err != nil {
		return solana.PublicKey{}, solana.PublicKey{}, err
	}
	return poolProgram, mint, nil
}

// poolProgram resolves the shared cctp_token_pool program. There is one instance per chain, not
// one per token, hence the fixed pool-program qualifier.
func (a *CCTPChainAdapter) poolProgram(ds datastore.DataStore, chainSelector uint64) (solana.PublicKey, error) {
	ref, err := solcommon.FindAndFormatCompatibleRef(ds, datastore.AddressRef{
		Type:      datastore.ContractType(common_utils.CCTPTokenPool),
		Version:   solcommon.Version2_0_0,
		Qualifier: solcommon.PoolProgramQualifier,
	}, chainSelector, datastore_utils.FullRef)
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("find CCTP token pool on chain %d: %w", chainSelector, err)
	}
	return solcommon.ToAddress(ref)
}

// usdcMint resolves the chain's USDC mint, preferring the USDC qualifier and falling back to a
// wildcard (empty qualifier) lookup for datastores that registered the mint unqualified.
func (a *CCTPChainAdapter) usdcMint(ds datastore.DataStore, chainSelector uint64) (solana.PublicKey, error) {
	splTypes := []cldf_deployment.ContractType{solcommon.SPLTokens, solcommon.SPL2022Tokens}
	for _, qualifier := range []string{USDCTokenQualifier, ""} {
		for _, contractType := range splTypes {
			ref, err := solcommon.FindLatestRef(ds, datastore.AddressRef{
				Type:      datastore.ContractType(contractType),
				Qualifier: qualifier,
			}, chainSelector)
			if err == nil {
				return solcommon.ToAddress(ref)
			}
		}
	}
	return solana.PublicKey{}, fmt.Errorf("find USDC mint on chain %d (looked for %s/%s with qualifier %q then unqualified)",
		chainSelector, solcommon.SPLTokens, solcommon.SPL2022Tokens, USDCTokenQualifier)
}
