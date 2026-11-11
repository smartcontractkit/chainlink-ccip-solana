package sequences

import (
	"errors"
	"fmt"

	"github.com/Masterminds/semver/v3"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	common_utils "github.com/smartcontractkit/chainlink-ccip/deployment/utils"
	seq_core "github.com/smartcontractkit/chainlink-ccip/deployment/utils/sequences"
	ccipadapters "github.com/smartcontractkit/chainlink-ccip/deployment/v2_0_0/adapters"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	cldf_ops "github.com/smartcontractkit/chainlink-deployments-framework/operations"
	mcms_types "github.com/smartcontractkit/mcms/types"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/ccip_router"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/tokens"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	cctpverifierops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/cctpverifier"
	fqops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/feequoter"
	offrampops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/offramp"
	rmnremoteops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/rmnremote"
	routerops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/router"
	tokenpoolops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/tokenpools"
)

// DeployCCTPChain deploys and configures the cctp_token_pool program - both its token-pool half
// and its embedded CCTP verifier - on a Solana chain.
//
// Several DeployCCTPInput fields are EVM-only and deliberately ignored here:
//
//   - TokenMessengerV1 / TokenMessengerV2: Circle's program IDs are compile-time constants in the
//     Rust (verifier/cctp_constants.rs and pool/instructions/mod.rs), not configuration. The
//     Circle programs are assumed to already be deployed at their canonical addresses.
//   - DeployerContract: there is no CREATE2-style deployer on Solana.
//
// The sequence does NOT register the pool with the router's TokenAdminRegistry. Until that is
// wired up separately the pool is deployed and configured but not reachable via CCIP.
var DeployCCTPChain = cldf_ops.NewSequence(
	"deploy-cctp-chain",
	semver.MustParse("2.0.0"),
	"Deploys and configures the CCTP token pool and its embedded verifier on a Solana chain",
	func(b cldf_ops.Bundle, deps ccipadapters.DeployCCTPChainDeps, input ccipadapters.DeployCCTPInput) (seq_core.OnChainOutput, error) {
		solChain, ok := deps.BlockChains.SolanaChains()[input.ChainSelector]
		if !ok {
			return seq_core.OnChainOutput{}, fmt.Errorf("solana chain with selector %d not found", input.ChainSelector)
		}
		if deps.DataStore == nil {
			return seq_core.OnChainOutput{}, errors.New("datastore is required")
		}
		if input.USDCToken == "" {
			return seq_core.OnChainOutput{}, errors.New("USDCToken is required")
		}
		mint, err := solana.PublicKeyFromBase58(input.USDCToken)
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("parse USDC token %q: %w", input.USDCToken, err)
		}

		existing := deps.DataStore.Addresses().Filter(datastore.AddressRefByChainSelector(input.ChainSelector))

		var (
			addresses []datastore.AddressRef
			batchOps  []mcms_types.BatchOperation
		)
		collect := func(out seq_core.OnChainOutput) {
			addresses = append(addresses, out.Addresses...)
			batchOps = append(batchOps, out.BatchOps...)
		}

		deployRep, err := cldf_ops.ExecuteOperation(b, tokenpoolops.DeployCctp, solChain, existing)
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("deploy cctp token pool: %w", err)
		}
		poolRef := deployRep.Output
		poolAddr, err := solana.PublicKeyFromBase58(poolRef.Address)
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("parse cctp token pool address %q: %w", poolRef.Address, err)
		}

		router, err := refPubkey(existing, input.ChainSelector, routerops.ContractType, routerops.Version)
		if err != nil {
			return seq_core.OnChainOutput{}, err
		}
		rmnRemote, err := refPubkey(existing, input.ChainSelector, rmnremoteops.ContractType, rmnremoteops.Version)
		if err != nil {
			return seq_core.OnChainOutput{}, err
		}

		// The mint's owner program tells us SPL Token vs Token-2022. Read at Confirmed rather
		// than the default commitment: in a fresh deploy the mint may have only just landed.
		mintInfo, err := solChain.Client.GetAccountInfoWithOpts(b.GetContext(), mint, &rpc.GetAccountInfoOpts{
			Commitment: rpc.CommitmentConfirmed,
		})
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("read USDC mint %s: %w", mint, err)
		}
		tokenProgramID := mintInfo.Value.Owner
		// The verifier's account resolvers hardcode the SPL Token program when deriving the pool
		// and receiver ATAs, so a Token-2022 mint would resolve to addresses the program rejects.
		if !tokenProgramID.Equals(solana.TokenProgramID) {
			return seq_core.OnChainOutput{}, fmt.Errorf(
				"USDC mint %s is owned by %s, but the CCTP verifier only supports the SPL Token program (%s)",
				mint, tokenProgramID, solana.TokenProgramID)
		}

		poolParams := tokenpoolops.Params{
			TokenPool:      poolAddr,
			TokenMint:      mint,
			TokenProgramID: tokenProgramID,
			RMNRemote:      rmnRemote,
			Router:         router,
		}

		// initialize_verifier initializes the verifier's ["config"] PDA itself; no separate
		// global-config init exists or is needed.
		initRep, err := cldf_ops.ExecuteOperation(b, tokenpoolops.InitializeCctp, solChain, poolParams)
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("initialize cctp token pool: %w", err)
		}
		collect(initRep.Output.OnChainOutput)

		// The pool's token account must exist before any transfer: the offramp's generic v2
		// account layout requires an initialized pool_token_account, and CCTP mints into it.
		poolSigner, err := tokens.TokenPoolSignerAddress(mint, poolAddr)
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("derive pool signer address: %w", err)
		}
		if err = ensureAssociatedTokenAccount(b, solChain, tokenProgramID, mint, poolSigner); err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("create cctp pool token account: %w", err)
		}

		// The pool signer stakes rent for the CCTP MessageSent event accounts and Nonce PDAs
		// created on every outbound transfer, so it needs SOL before the first send.
		if _, err = cldf_ops.ExecuteOperation(b, tokenpoolops.FundPoolSignerCctp, solChain, tokenpoolops.FundPoolSignerParams{
			TokenPool: poolAddr,
			TokenMint: mint,
		}); err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("fund cctp pool signer: %w", err)
		}

		// Register the pool with the router's TokenAdminRegistry and build its token lookup table
		// (pool_program@2, mint@7). The offramp's get_ccvs_for_msg account resolution reads exactly
		// this LUT to find a pool at all - without it no CCTP message can ever execute, the same
		// way BurnMint/LockRelease pools get it via ConfigureTokenForTransfersSequence.
		feeQuoter, err := refPubkey(existing, input.ChainSelector, fqops.ContractType, fqops.Version)
		if err != nil {
			return seq_core.OnChainOutput{}, err
		}
		registryRep, err := cldf_ops.ExecuteOperation(b, routerops.RegisterTokenAdminRegistry, solChain, routerops.TokenAdminRegistryParams{
			Router:            router,
			TokenMint:         mint,
			ExistingAddresses: existing,
		})
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("register token admin registry: %w", err)
		}
		collect(registryRep.Output.OnChainOutput)
		if !registryRep.Output.PendingSigner.IsZero() {
			acceptRep, aerr := cldf_ops.ExecuteOperation(b, routerops.AcceptTokenAdminRegistry, solChain, routerops.TokenAdminRegistryParams{
				Router:            router,
				TokenMint:         mint,
				Admin:             registryRep.Output.PendingSigner,
				ExistingAddresses: existing,
			})
			if aerr != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("accept token admin registry: %w", aerr)
			}
			collect(acceptRep.Output)
		}
		// SetPool builds the stock 10-entry pool LUT with writable indexes {3, 4, 7}. The
		// program-level tests additionally mark index 5 (the pool signer, which stakes rent for
		// the CCTP event PDAs) and the CCTPv1-specific indexes {10, 16} writable, and append
		// Circle's accounts to the table - see contracts/tests/testutils/setup.go's setupCCTPPool.
		// That is deliberately not replicated here: it is only needed for the CCIP 1.5 pool path,
		// whereas the 2.0 CCV path this devenv exercises resolves its accounts dynamically.
		setPoolRep, err := cldf_ops.ExecuteOperation(b, routerops.SetPool, solChain, routerops.PoolParams{
			ExistingAddresses: existing,
			Router:            router,
			FeeQuoter:         feeQuoter,
			TokenMint:         mint,
			TokenPool:         poolAddr,
			TokenProgramID:    tokenProgramID,
			TokenPoolType:     string(common_utils.CCTPTokenPool),
			TokenSymbol:       solcommon.USDCTokenQualifier,
		})
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("set pool on router: %w", err)
		}
		collect(setPoolRep.Output)
		// WithoutExtendedLookupTable matches how the program-level tests register this pool
		// (contracts/tests/testutils/setup.go): cctp_token_pool resolves its own accounts, so the
		// router does not need an extended lookup table for it. TAR v4 stores this as Standard in the
		// legacy byte plus a trailing skip flag, so a 1.6 lane can still decode the registry.
		interfaceRep, err := cldf_ops.ExecuteOperation(b, routerops.SetPoolInterfaceImplementation, solChain, routerops.SetPoolInterfaceImplParams{
			Router:                      router,
			TokenMint:                   mint,
			InterfaceVersion:            2,
			ImplementsAccountResolution: uint8(ccip_router.ResolutionType_WithoutExtendedLookupTable),
		})
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("set pool interface implementation: %w", err)
		}
		collect(interfaceRep.Output)

		verifierParams := cctpverifierops.Params{CctpVerifier: poolAddr}
		verifierRep, err := cldf_ops.ExecuteOperation(b, cctpverifierops.Initialize, solChain, verifierParams)
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("initialize cctp verifier: %w", err)
		}
		// A timelock-held upgrade authority routes initialize_verifier through MCMS.
		collect(verifierRep.Output)

		// The offramp needs the verifier's config PDA in its lookup table to execute inbound
		// messages routed through this CCV.
		cvConfigPDA, _, err := state.FindCVConfigPDA(poolAddr)
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("derive cctp verifier config PDA: %w", err)
		}
		offRamp, err := refPubkey(existing, input.ChainSelector, offrampops.ContractType, offrampops.Version)
		if err != nil {
			return seq_core.OnChainOutput{}, err
		}
		if err = solcommon.ExtendOfframpLookupTable(solChain, offRamp, []solana.PublicKey{cvConfigPDA}); err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("extend offramp lookup table with cctp verifier: %w", err)
		}

		if input.FeeAggregator != "" {
			feeAggregator, perr := solana.PublicKeyFromBase58(input.FeeAggregator)
			if perr != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("parse fee aggregator %q: %w", input.FeeAggregator, perr)
			}
			rep, ferr := cldf_ops.ExecuteOperation(b, cctpverifierops.SetFeeAggregator, solChain, cctpverifierops.SetFeeAggregatorParams{
				CctpVerifier:  poolAddr,
				FeeAggregator: feeAggregator,
			})
			if ferr != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("set cctp verifier fee aggregator: %w", ferr)
			}
			collect(rep.Output)
		}

		if len(input.StorageLocations) > 0 {
			rep, serr := cldf_ops.ExecuteOperation(b, cctpverifierops.SetStorageLocations, solChain, cctpverifierops.SetStorageLocationsParams{
				CctpVerifier:     poolAddr,
				StorageLocations: input.StorageLocations,
			})
			if serr != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("set cctp verifier storage locations: %w", serr)
			}
			collect(rep.Output)
		}

		// initialize_verifier seeds fast_finality_bps to Circle's Solana default, so only
		// override it when the caller asked for something else. The operation is a no-op when
		// the on-chain value already matches.
		if input.FastFinalityBps != 0 {
			rep, ferr := cldf_ops.ExecuteOperation(b, cctpverifierops.SetFastFinalityBps, solChain, cctpverifierops.SetFastFinalityBpsParams{
				CctpVerifier:    poolAddr,
				FastFinalityBps: input.FastFinalityBps,
			})
			if ferr != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("set cctp verifier fast finality bps: %w", ferr)
			}
			collect(rep.Output)
		}

		// Two fee vaults collect receipts: the verifier's (empty qualifier) and the pool's
		// (qualified by mint). A missing ATA on either surfaces as MissingAtaForReceiptIssuer at
		// transfer time, so create both up front, for every fee token.
		linkMint, lerr := findLinkMint(existing, input.ChainSelector)
		if lerr != nil {
			return seq_core.OnChainOutput{}, lerr
		}
		fees, lerr := feeTokens(b.GetContext(), solChain, linkMint)
		if lerr != nil {
			return seq_core.OnChainOutput{}, lerr
		}
		for _, qualifier := range [][]byte{{}, mint.Bytes()} {
			if verr := ensureFeeVaults(b.GetContext(), solChain, poolAddr, qualifier, fees); verr != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("cctp: %w", verr)
			}
		}

		// The pool and the verifier are the same program, but each is registered under its own
		// contract type because consumers look them up independently. The qualifier mirrors the
		// production convention (qualifier equals the contract type), matching the
		// CCTPVerifierResolver refs on EVM chains.
		verifierRef := poolRef
		verifierRef.Type = datastore.ContractType(cctpverifierops.ContractType)
		verifierRef.Version = cctpverifierops.Version
		verifierRef.Qualifier = string(cctpverifierops.ContractType)

		// Addresses collected along the way - notably the per-mint TokenPoolLookupTable that
		// routerops.SetPool creates - are returned alongside the pool and verifier refs.
		return seq_core.OnChainOutput{
			Addresses: append([]datastore.AddressRef{poolRef, verifierRef}, addresses...),
			BatchOps:  batchOps,
		}, nil
	},
)

// refPubkey resolves a single contract address from a chain's existing address refs.
func refPubkey(refs []datastore.AddressRef, chainSelector uint64, contractType interface{ String() string }, version *semver.Version) (solana.PublicKey, error) {
	key := datastore.NewAddressRefKey(chainSelector, datastore.ContractType(contractType.String()), version, "")
	addr, err := solcommon.PubkeyFromRefs(refs, key)
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("find %s on chain %d: %w", contractType.String(), chainSelector, err)
	}
	return addr, nil
}

// ensureAssociatedTokenAccount creates an ATA if it doesn't already exist.
func ensureAssociatedTokenAccount(b cldf_ops.Bundle, solChain cldf_solana.Chain, tokenProgram, mint, owner solana.PublicKey) error {
	ata, _, err := tokens.FindAssociatedTokenAddress(tokenProgram, mint, owner)
	if err != nil {
		return fmt.Errorf("derive associated token address (mint %s, owner %s): %w", mint, owner, err)
	}
	exists, err := accountExists(b.GetContext(), solChain, ata)
	if err != nil {
		return fmt.Errorf("fetch associated token account (mint %s, owner %s, ata %s): %w", mint, owner, ata, err)
	}
	if exists {
		b.Logger.Infow("Associated token account already exists, skipping", "mint", mint, "owner", owner, "ata", ata)
		return nil
	}
	ixn, _, err := tokens.CreateAssociatedTokenAccount(tokenProgram, mint, owner, solChain.DeployerKey.PublicKey())
	if err != nil {
		return fmt.Errorf("build create associated token account instruction (mint %s, owner %s, ata %s): %w", mint, owner, ata, err)
	}
	if err = solChain.Confirm([]solana.Instruction{ixn}); err != nil {
		return fmt.Errorf("confirm create associated token account (mint %s, owner %s, ata %s): %w", mint, owner, ata, err)
	}
	return nil
}
