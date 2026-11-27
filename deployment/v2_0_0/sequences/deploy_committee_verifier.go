package sequences

import (
	"errors"
	"fmt"

	"github.com/Masterminds/semver/v3"
	"github.com/gagliardetto/solana-go"

	seq_core "github.com/smartcontractkit/chainlink-ccip/deployment/utils/sequences"
	cldf_chain "github.com/smartcontractkit/chainlink-deployments-framework/chain"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	cldf_ops "github.com/smartcontractkit/chainlink-deployments-framework/operations"
	mcms_types "github.com/smartcontractkit/mcms/types"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/tokens"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	cvops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/committeeverifier"
	offrampops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/offramp"
)

// DeployCommitteeVerifierInput drives the DeployCommitteeVerifier sequence. The program is
// deployed (or resolved, if already present in ExistingAddresses) and then configured;
// FeeAggregator, AllowlistAdmin and StorageLocations are optional.
type DeployCommitteeVerifierInput struct {
	ChainSelector     uint64
	ExistingAddresses []datastore.AddressRef
	FeeAggregator     string
	AllowlistAdmin    string
	StorageLocations  []string
}

// DeployCommitteeVerifier deploys and configures the CommitteeVerifier on a Solana chain:
// Deploy (or resolve existing) → Initialize (skipped if already initialized) → SetFeeAggregator →
// SetAllowlistAdmin → SetStorageLocations. It is
// separate from DeployChainContracts because committee verifiers own their own deploy (the
// 2026-06-17 protocol/committee split). Signer sets are owned by apply_verifier_config.
var DeployCommitteeVerifier = cldf_ops.NewSequence(
	"deploy-committee-verifier",
	semver.MustParse("2.0.0"),
	"Deploys and configures the CommitteeVerifier on a Solana chain",
	func(b cldf_ops.Bundle, chains cldf_chain.BlockChains, input DeployCommitteeVerifierInput) (seq_core.OnChainOutput, error) {
		var batchOps []mcms_types.BatchOperation
		solChain, ok := chains.SolanaChains()[input.ChainSelector]
		if !ok {
			return seq_core.OnChainOutput{}, fmt.Errorf("solana chain with selector %d not found", input.ChainSelector)
		}

		// Deploy (or resolve, if already registered) the committee-verifier program. On Solana
		// the program is typically pre-loaded into the validator and seeded into the datastore
		// by the chain's pre-deploy hook, so this resolves the existing ref; in a fresh env
		// (CLD) it deploys. The ref is returned below so downstream consumers find it.
		//
		// The committee-verifier program state uses global seeds, so the program holds one
		// committee under cvops.DeployQualifier. The pre-deploy seed registers it
		// under the empty qualifier, but cvops.Deploy resolves it under cvops.DeployQualifier.
		// Normalize the seed to that qualifier so the deploy short-circuits to the pre-loaded
		// program instead of deploying a fresh, declared-ID-mismatched keypair.
		existing := withCommitteeVerifierQualifier(input.ExistingAddresses, cvops.DeployQualifier)
		deployRep, err := cldf_ops.ExecuteOperation(b, cvops.Deploy, solChain, existing)
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("deploy committee verifier: %w", err)
		}
		verifierRef := deployRep.Output
		verifierAddr, err := solana.PublicKeyFromBase58(verifierRef.Address)
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("parse committee verifier address %q: %w", verifierRef.Address, err)
		}

		if _, err = cldf_ops.ExecuteOperation(b, cvops.Initialize, solChain, cvops.Params{
			CommitteeVerifier: verifierAddr,
		}); err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("initialize committee verifier: %w", err)
		}

		// Protocol deploy creates LUTs; extend them here with CV accounts after initialize.
		// configure_chain_for_lanes extends per-lane CV PDAs when lanes are wired.
		cvConfigPDA, _, _ := state.FindCVConfigPDA(verifierAddr)
		offRampPubkey, err := solcommon.PubkeyFromRefs(
			input.ExistingAddresses,
			datastore.NewAddressRefKey(
				input.ChainSelector,
				datastore.ContractType(offrampops.ContractType),
				offrampops.Version,
				"",
			),
		)
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("find offramp in existing addresses: %w", err)
		}
		if err = solcommon.ExtendOfframpLookupTable(solChain, offRampPubkey, []solana.PublicKey{cvConfigPDA}); err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("extend offramp lookup table with committee verifier: %w", err)
		}
		ccipSendLUT, lutErr := solcommon.PubkeyFromRefs(
			input.ExistingAddresses,
			datastore.NewAddressRefKey(
				input.ChainSelector,
				solcommon.CcipSendLookupTableType,
				solcommon.Version2_0_0,
				"",
			),
		)
		if lutErr == nil {
			if err = solcommon.ExtendLookupTable(solChain, ccipSendLUT, []solana.PublicKey{verifierAddr, cvConfigPDA}); err != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("extend ccipSend lookup table with committee verifier: %w", err)
			}
		} else if !errors.Is(lutErr, solcommon.ErrAddressRefNotFound) {
			return seq_core.OnChainOutput{}, fmt.Errorf("find ccipSend lookup table in existing addresses: %w", lutErr)
		}

		if input.FeeAggregator != "" {
			feeAgg, perr := solana.PublicKeyFromBase58(input.FeeAggregator)
			if perr != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("parse fee aggregator %q: %w", input.FeeAggregator, perr)
			}
			if err = runOp(b, cvops.SetFeeAggregator, solChain, cvops.SetFeeAggregatorParams{
				CommitteeVerifier: verifierAddr,
				FeeAggregator:     feeAgg,
			}, &batchOps); err != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("set fee aggregator: %w", err)
			}
		}

		if input.AllowlistAdmin != "" {
			allowlistAdmin, perr := solana.PublicKeyFromBase58(input.AllowlistAdmin)
			if perr != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("parse allowlist admin %q: %w", input.AllowlistAdmin, perr)
			}
			if err = runOp(b, cvops.SetAllowlistAdmin, solChain, cvops.SetAllowlistAdminParams{
				CommitteeVerifier: verifierAddr,
				AllowlistAdmin:    allowlistAdmin,
			}, &batchOps); err != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("set allowlist admin: %w", err)
			}
		}

		if len(input.StorageLocations) > 0 {
			if err = runOp(b, cvops.SetStorageLocations, solChain, cvops.SetStorageLocationsParams{
				CommitteeVerifier: verifierAddr,
				StorageLocations:  input.StorageLocations,
			}, &batchOps); err != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("set storage locations: %w", err)
			}
		}

		// initialize fee vault ATAs
		type ATA struct {
			ata          solana.PublicKey
			tokenProgram solana.PublicKey
			mint         solana.PublicKey
			authority    solana.PublicKey
		}
		ataFrom := func(tokenProgram, mint, authority solana.PublicKey) ATA {
			ata, _, aerr := tokens.FindAssociatedTokenAddress(tokenProgram, mint, authority)
			if aerr != nil {
				b.Logger.Warnw("failed to find associated token address", "tokenProgram", tokenProgram.String(), "mint", mint.String(), "authority", authority.String(), "error", aerr)
			}
			return ATA{
				ata:          ata,
				tokenProgram: tokenProgram,
				mint:         mint,
				authority:    authority,
			}
		}

		linkMint, err := findLinkMint(input.ExistingAddresses, solChain.Selector)
		if err != nil {
			return seq_core.OnChainOutput{}, err
		}
		fees, err := feeTokens(b.GetContext(), solChain, linkMint)
		if err != nil {
			return seq_core.OnChainOutput{}, err
		}
		atas := make([]ATA, 0, len(fees))
		for _, ft := range fees {
			vaultPda, _, aerr := state.FindFeeVaultPDA(verifierAddr, []byte{})
			if aerr != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("failed to find CCV fee vault: %w", aerr)
			}
			atas = append(atas, ataFrom(ft.program, ft.mint, vaultPda))
		}

		for _, ata := range atas {
			ixn, _, ataErr := tokens.CreateAssociatedTokenAccount(
				ata.tokenProgram,
				ata.mint,
				ata.authority,
				solChain.Deployer().PublicKey(),
			)
			if ataErr != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("failed to create the instruction to create associated token account for CCV vault ATA (mint: %s, ccv: %s, ata/vaultAta: %s): %w", ata.mint.String(), verifierAddr.String(), ata.ata.String(), ataErr)
			}
			if confirmErr := solChain.Confirm([]solana.Instruction{ixn}); confirmErr != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("failed to create associated token account for CCV vault ATA (mint: %s, ccv: %s, ata/vaultAta: %s): %w", ata.mint.String(), verifierAddr.String(), ata.ata.String(), confirmErr)
			}
		}

		return seq_core.OnChainOutput{Addresses: []datastore.AddressRef{verifierRef}, BatchOps: batchOps}, nil
	},
)

// withCommitteeVerifierQualifier ensures a CommitteeVerifier program ref tagged with qualifier
// is present in refs. The Solana committee-verifier program holds one committee, so
// when the datastore already holds the program under a different qualifier (e.g. the empty
// pre-deploy seed) we append a copy re-tagged with the requested qualifier. This lets
// cvops.Deploy resolve the pre-loaded program by qualifier instead of deploying a fresh one. If
// no CommitteeVerifier ref exists (fresh CLD env) refs is returned unchanged, so cvops.Deploy
// falls through to a real deploy.
func withCommitteeVerifierQualifier(refs []datastore.AddressRef, qualifier string) []datastore.AddressRef {
	var program *datastore.AddressRef
	for i := range refs {
		ref := refs[i]
		if ref.Type != datastore.ContractType(cvops.ContractType) || !solcommon.RefVersionMatches(ref.Version, cvops.Version) {
			continue
		}
		if ref.Qualifier == qualifier {
			return refs // already resolvable under the requested qualifier
		}
		program = &refs[i]
	}
	if program == nil {
		return refs // no pre-loaded program; let cvops.Deploy deploy fresh
	}
	tagged := *program
	tagged.Qualifier = qualifier
	return append(refs, tagged)
}
