package sequences

import (
	"errors"
	"fmt"

	"github.com/Masterminds/semver/v3"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	common_utils "github.com/smartcontractkit/chainlink-ccip/deployment/utils"
	seq_core "github.com/smartcontractkit/chainlink-ccip/deployment/utils/sequences"
	cldf_chain "github.com/smartcontractkit/chainlink-deployments-framework/chain"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	cldf_ops "github.com/smartcontractkit/chainlink-deployments-framework/operations"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/ccip_router"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/tokens"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	cctpverifierops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/cctpverifier"
	offrampops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/offramp"
	routerops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/router"
	tokenpoolops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/tokenpools"
)

// MigrateCctpTokenPoolToV2Input selects the chain and the USDC mint whose 1.6 CCTP pool migrates.
type MigrateCctpTokenPoolToV2Input struct {
	DataStore     datastore.DataStore
	ChainSelector uint64
	// USDCToken is the mint the CCTP pool manages.
	USDCToken solana.PublicKey
}

// MigrateCctpTokenPoolToV2 makes an existing 1.6 CCTP token pool usable by 2.0 lanes. Unlike a
// fresh DeployCCTPChain, the pool program and its router registration already exist, so this is
// the delta between them:
//
//  1. initialize_verifier creates the embedded verifier's config PDAs (the 1.6 binary has no
//     verifier half). It requires the pool program's upgrade authority, so it is batched into an
//     MCMS proposal when the timelock holds it.
//  2. The pool signer's token account and SOL balance, which stake rent for the v2 event accounts.
//  3. The Token Admin Registry flip to interface version 2 with WithoutExtendedLookupTable
//     resolution (account resolution stays on; only the lookup table's additional entries are
//     skipped, since cctp_token_pool resolves its own accounts), lifting the
//     UnsupportedPoolV1ResolutionOnLaneV2 gate ccip_send_v2 applies to 1.6 CCTP registries. The
//     1.6 ccip_send path never reads the registry version, so live 1.6 lanes are unaffected and
//     this does not need to share an MCMS batch with the lane promotion.
//  4. The verifier config PDA joins the OffRamp lookup table, which inbound v2 execution reads.
//
// The pool program itself is upgraded separately (UpgradeProgram with UpgradeTypeCCTPTokenPool)
// and the per-lane verifier and pool configs come from ConfigureCCTPChainForLanes; both must land
// before the first 2.0 message for this token. Every step is idempotent, so the sequence can be
// re-run.
var MigrateCctpTokenPoolToV2 = cldf_ops.NewSequence(
	"migrate-cctp-token-pool-to-v2",
	semver.MustParse("2.0.0"),
	"Migrates an existing 1.6 CCTP token pool deployment so 2.0 lanes can use it",
	func(b cldf_ops.Bundle, chains cldf_chain.BlockChains, input MigrateCctpTokenPoolToV2Input) (seq_core.OnChainOutput, error) {
		var result seq_core.OnChainOutput
		chain, ok := chains.SolanaChains()[input.ChainSelector]
		if !ok {
			return result, fmt.Errorf("solana chain with selector %d not found", input.ChainSelector)
		}
		if input.USDCToken.IsZero() {
			return result, errors.New("USDC token mint is required")
		}
		if input.DataStore == nil {
			return result, errors.New("datastore is required")
		}

		poolAddr, registerRef, err := ResolveCctpPoolForMigration(input.DataStore, input.ChainSelector)
		if err != nil {
			return result, err
		}
		existing := input.DataStore.Addresses().Filter(datastore.AddressRefByChainSelector(input.ChainSelector))
		router, err := refPubkey(existing, input.ChainSelector, routerops.ContractType, routerops.Version)
		if err != nil {
			return result, err
		}
		offRamp, err := refPubkey(existing, input.ChainSelector, offrampops.ContractType, offrampops.Version)
		if err != nil {
			return result, err
		}
		// The verifier's account resolvers hardcode the SPL Token program when deriving ATAs, so a
		// Token-2022 mint would resolve to addresses the program rejects (same check as DeployCCTPChain).
		mintInfo, err := chain.Client.GetAccountInfoWithOpts(b.GetContext(), input.USDCToken, &rpc.GetAccountInfoOpts{
			Commitment: rpc.CommitmentConfirmed,
		})
		if err != nil {
			return result, fmt.Errorf("read USDC mint %s: %w", input.USDCToken, err)
		}
		if !mintInfo.Value.Owner.Equals(solana.TokenProgramID) {
			return result, fmt.Errorf(
				"USDC mint %s is owned by %s, but the CCTP verifier only supports the SPL Token program (%s)",
				input.USDCToken, mintInfo.Value.Owner, solana.TokenProgramID)
		}

		// Step 1: the embedded verifier's config PDAs.
		if err = runOp(b, cctpverifierops.Initialize, chain, cctpverifierops.Params{CctpVerifier: poolAddr}, &result.BatchOps); err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("initialize cctp verifier: %w", err)
		}

		// Step 2: the pool signer's token account and SOL balance.
		poolSigner, err := tokens.TokenPoolSignerAddress(input.USDCToken, poolAddr)
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("derive pool signer address: %w", err)
		}
		if err = ensureAssociatedTokenAccount(b, chain, solana.TokenProgramID, input.USDCToken, poolSigner); err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("create cctp pool token account: %w", err)
		}
		if err = runOp(b, tokenpoolops.FundPoolSignerCctp, chain, tokenpoolops.FundPoolSignerParams{
			TokenPool: poolAddr,
			TokenMint: input.USDCToken,
		}, &result.BatchOps); err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("fund cctp pool signer: %w", err)
		}

		// Step 3: the Token Admin Registry flip. WithoutExtendedLookupTable matches fresh 2.0
		// deploys: the pool resolves its own accounts, so the router needs no extended table.
		// TAR v4 keeps the legacy resolution byte at 1 and stores the skip in a trailing field,
		// so the 1.6 OffRamp can still decode the registry.
		// The registry exists from the pool's 1.6 registration and the flip edits it in place;
		// a missing one means the pool was never registered.
		tarPDA, _, err := state.FindTokenAdminRegistryPDA(input.USDCToken, router)
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("derive token admin registry PDA: %w", err)
		}
		if _, _, err = routerops.ReadTokenAdminRegistry(b.GetContext(), chain.Client, tarPDA); err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("read token admin registry for mint %s (was the pool registered on 1.6?): %w", input.USDCToken, err)
		}
		if err = runOp(b, routerops.SetPoolInterfaceImplementation, chain, routerops.SetPoolInterfaceImplParams{
			Router:                      router,
			TokenMint:                   input.USDCToken,
			InterfaceVersion:            2,
			ImplementsAccountResolution: uint8(ccip_router.ResolutionType_WithoutExtendedLookupTable),
		}, &result.BatchOps); err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("set pool interface implementation: %w", err)
		}

		// Step 4: the OffRamp lookup table learns the verifier config PDA for inbound execution.
		cvConfigPDA, _, err := state.FindCVConfigPDA(poolAddr)
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("derive cctp verifier config PDA: %w", err)
		}
		if err = solcommon.ExtendOfframpLookupTable(chain, offRamp, []solana.PublicKey{cvConfigPDA}); err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("extend offramp lookup table with cctp verifier: %w", err)
		}

		// The 2.0 tooling resolves the pool program by its 2.0.0 ref (ConfigureCCTPChainForLanes
		// included), so the in-place-upgraded program gets one at its existing address.
		if registerRef {
			result.Addresses = append(result.Addresses, datastore.AddressRef{
				Address:       poolAddr.String(),
				ChainSelector: input.ChainSelector,
				Type:          datastore.ContractType(common_utils.CCTPTokenPool),
				Version:       solcommon.Version2_0_0,
				Qualifier:     solcommon.PoolProgramQualifier,
			})
		}
		return result, nil
	},
)

// ResolveCctpPoolForMigration returns the chain's CCTP pool program address, preferring an
// already-registered 2.0.0 ref (a re-run) over the 1.6 one. The bool reports whether the 2.0.0
// ref still needs registering.
func ResolveCctpPoolForMigration(ds datastore.DataStore, chainSelector uint64) (solana.PublicKey, bool, error) {
	refs := ds.Addresses().Filter(
		datastore.AddressRefByChainSelector(chainSelector),
		datastore.AddressRefByType(datastore.ContractType(common_utils.CCTPTokenPool)),
	)
	var upgraded, legacy []datastore.AddressRef
	for _, ref := range refs {
		if ref.Version == nil {
			continue
		}
		switch {
		case solcommon.RefVersionMatches(ref.Version, solcommon.Version2_0_0) && ref.Qualifier == solcommon.PoolProgramQualifier:
			upgraded = append(upgraded, ref)
		case ref.Version.Major() == 1:
			legacy = append(legacy, ref)
		}
	}
	if len(upgraded) > 0 {
		addr, err := solana.PublicKeyFromBase58(upgraded[0].Address)
		if err != nil {
			return solana.PublicKey{}, false, fmt.Errorf("parse CCTP token pool address %q: %w", upgraded[0].Address, err)
		}
		return addr, false, nil
	}
	switch len(legacy) {
	case 0:
		return solana.PublicKey{}, false, fmt.Errorf("no 1.x CCTPTokenPool ref on chain %d to migrate", chainSelector)
	case 1:
		addr, err := solana.PublicKeyFromBase58(legacy[0].Address)
		if err != nil {
			return solana.PublicKey{}, false, fmt.Errorf("parse CCTP token pool address %q: %w", legacy[0].Address, err)
		}
		return addr, true, nil
	default:
		return solana.PublicKey{}, false, fmt.Errorf("multiple 1.x CCTPTokenPool refs on chain %d; expected one", chainSelector)
	}
}
