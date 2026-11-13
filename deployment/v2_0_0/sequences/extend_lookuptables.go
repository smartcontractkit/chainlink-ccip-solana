package sequences

import (
	"errors"
	"fmt"

	"github.com/Masterminds/semver/v3"
	"github.com/gagliardetto/solana-go"

	common_utils "github.com/smartcontractkit/chainlink-ccip/deployment/utils"
	seq_core "github.com/smartcontractkit/chainlink-ccip/deployment/utils/sequences"
	cldf_chain "github.com/smartcontractkit/chainlink-deployments-framework/chain"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	cldf_ops "github.com/smartcontractkit/chainlink-deployments-framework/operations"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	soltokens "github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/tokens"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	executorops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/executor"
	fqops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/feequoter"
	lutops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/lut"
	offrampops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/offramp"
	rmnremoteops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/rmnremote"
	routerops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/router"
	tokensops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/tokens"
)

type ExtendLutInput struct {
	ChainSelector     uint64
	ExistingAddresses []datastore.AddressRef
	// CreateIfMissing creates the table when the datastore has none, which is the case for a chain
	// whose Router was upgraded in place and never went through DeployChainContracts.
	CreateIfMissing bool
}

var ExtendCcipSendLut = cldf_ops.NewSequence(
	"extend-ccip-send-lookup-table",
	semver.MustParse("2.0.0"),
	"Fills the ccip_send_v2 lookup table, creating it first when asked and the chain has none, with the current static account set and the v2 burn/mint and lock/release pools' fee vault ATAs, and points the Router at it",
	func(b cldf_ops.Bundle, chains cldf_chain.BlockChains, input ExtendLutInput) (seq_core.OnChainOutput, error) {
		chain, ok := chains.SolanaChains()[input.ChainSelector]
		if !ok {
			return seq_core.OnChainOutput{}, fmt.Errorf("solana chain with selector %d not found", input.ChainSelector)
		}

		accounts, err := resolveCcipSendLutAccounts(input.ExistingAddresses, input.ChainSelector)
		if err != nil {
			return seq_core.OnChainOutput{}, err
		}
		router := accounts.Router
		staticEntries, err := CcipSendLutEntries(accounts)
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("failed to create ccipSend LUT entries: %w", err)
		}
		recorded, recordedErr := readAddress(input, solcommon.CcipSendLookupTableType, solcommon.Version2_0_0, "")
		onChain, err := routerops.OnrampLookupTable(b.GetContext(), chain, router)
		if err != nil {
			return seq_core.OnChainOutput{}, err
		}
		lookupTable, register, err := ccipSendLookupTable(recorded, recordedErr, onChain, input.CreateIfMissing)
		if err != nil {
			return seq_core.OnChainOutput{}, err
		}
		entries := staticEntries
		if lookupTable.IsZero() {
			lookupTable, err = solcommon.SetupLookupTable(b.GetContext(), chain, staticEntries)
			if err != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("create ccip_send lookup table: %w", err)
			}
			entries = nil
		}
		var addresses []datastore.AddressRef
		if register {
			addresses = append(addresses, datastore.AddressRef{
				Address:       lookupTable.String(),
				Type:          solcommon.CcipSendLookupTableType,
				Version:       solcommon.Version2_0_0,
				ChainSelector: input.ChainSelector,
			})
		}
		// ccip_send_v2 resolves each v2 pool's fee vault ATA on the send path. As plain keys
		// they cost 32 bytes apiece and push the transaction over the 1232-byte packet limit;
		// as LUT entries they cost one byte each.
		feeVaults, skipped, err := poolFeeVaultLutEntries(input.ExistingAddresses, input.ChainSelector)
		if err != nil {
			return seq_core.OnChainOutput{}, err
		}
		if len(skipped) > 0 {
			b.Logger.Warnw("Skipped v2 token pools with no mint ref; their fee vault ATAs are not in the ccip_send lookup table", "qualifiers", skipped)
		}
		b.Logger.Infow("Adding v2 token pool fee vault ATAs to the ccip_send lookup table", "count", len(feeVaults))
		entries = append(entries, feeVaults...)
		if err = extendLookupTableInChunks(b, chain, lookupTable, entries); err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("extend ccip_send lookup table: %w", err)
		}
		// Chains deployed before the router returned this table from account resolution still
		// need their router pointed at it. A no-op once it already is.
		setLUTReport, err := cldf_ops.ExecuteOperation(b, routerops.UpdateReferenceAddresses, chain, routerops.UpdateReferenceAddressesParams{
			Router:            router,
			OnrampLookupTable: lookupTable,
		})
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("update router reference addresses: %w", err)
		}
		return seq_core.OnChainOutput{Addresses: addresses, BatchOps: setLUTReport.Output.BatchOps}, nil
	},
)

func ccipSendLookupTable(recorded solana.PublicKey, recordedErr error, onChain solana.PublicKey, createIfMissing bool) (solana.PublicKey, bool, error) {
	notRecorded := errors.Is(recordedErr, solcommon.ErrAddressRefNotFound)
	switch {
	case recordedErr != nil && !notRecorded:
		return solana.PublicKey{}, false, fmt.Errorf("resolve ccip_send lookup table: %w", recordedErr)
	case !notRecorded && !onChain.IsZero() && recorded != onChain:
		return solana.PublicKey{}, false, fmt.Errorf("datastore records ccip_send lookup table %s, but the router points at %s", recorded, onChain)
	case !notRecorded:
		return recorded, false, nil
	case !onChain.IsZero():
		return onChain, true, nil
	case createIfMissing:
		return solana.PublicKey{}, true, nil
	default:
		return solana.PublicKey{}, false, fmt.Errorf("resolve ccip_send lookup table: %w", recordedErr)
	}
}

// resolveCcipSendLutAccounts reads from the datastore the programs whose accounts the ccip_send_v2
// lookup table holds.
func resolveCcipSendLutAccounts(refs []datastore.AddressRef, chainSelector uint64) (CcipSendLutEntriesInput, error) {
	input := ExtendLutInput{ChainSelector: chainSelector, ExistingAddresses: refs}
	var out CcipSendLutEntriesInput
	var err error
	// LINK is a shared token, not a program upgraded with the rest, so a chain deployed before 2.0 only
	// has its 1.6.0 ref. Resolve it the way the other sequences do, whatever version it was registered at.
	if out.LinkToken, err = findLinkMint(refs, chainSelector); err != nil {
		return out, fmt.Errorf("resolve LINK token: %w", err)
	}
	if out.LinkToken.IsZero() {
		return out, fmt.Errorf("resolve LINK token: %w: chain %d type %s", solcommon.ErrAddressRefNotFound, chainSelector, tokensops.LinkContractType)
	}
	if out.Router, err = readAddress(input, datastore.ContractType(routerops.ContractType), routerops.Version, ""); err != nil {
		return out, fmt.Errorf("resolve router: %w", err)
	}
	if out.FeeQuoter, err = readAddress(input, datastore.ContractType(fqops.ContractType), fqops.Version, ""); err != nil {
		return out, fmt.Errorf("resolve fee quoter: %w", err)
	}
	if out.RMNRemote, err = readAddress(input, datastore.ContractType(rmnremoteops.ContractType), rmnremoteops.Version, ""); err != nil {
		return out, fmt.Errorf("resolve RMN Remote: %w", err)
	}
	if out.BurnMintTokenPool, err = readAddress(input, datastore.ContractType(common_utils.BurnMintTokenPool), solcommon.Version2_0_0, solcommon.PoolProgramQualifier); err != nil {
		return out, fmt.Errorf("resolve burn/mint token pool: %w", err)
	}
	if out.LockReleaseTokenPool, err = readAddress(input, datastore.ContractType(common_utils.LockReleaseTokenPool), solcommon.Version2_0_0, solcommon.PoolProgramQualifier); err != nil {
		return out, fmt.Errorf("resolve lock/release token pool: %w", err)
	}
	if out.Executor, err = readAddress(input, datastore.ContractType(executorops.ContractType), executorops.Version, "default"); err != nil {
		return out, fmt.Errorf("resolve executor: %w", err)
	}
	return out, nil
}

func readAddress(input ExtendLutInput, contractType datastore.ContractType, version *semver.Version, qualifier string) (solana.PublicKey, error) {
	return solcommon.PubkeyFromRefs(input.ExistingAddresses, datastore.NewAddressRefKey(
		input.ChainSelector,
		contractType,
		version,
		qualifier,
	))
}

// poolFeeVaultLutEntries derives the fee vault ATA of every burn/mint and lock/release v2 pool
// instance registered in the refs, for the given chain. Only these two pool types resolve the ATA
// on the ccip_send_v2 path. It also returns the qualifiers of pool instances with no mint ref.
func poolFeeVaultLutEntries(refs []datastore.AddressRef, chainSelector uint64) ([]solana.PublicKey, []string, error) {
	var entries []solana.PublicKey
	var skipped []string
	for _, ref := range refs {
		if ref.ChainSelector != chainSelector || !solcommon.RefVersionMatches(ref.Version, solcommon.Version2_0_0) {
			continue
		}
		if ref.Type != datastore.ContractType(common_utils.BurnMintTokenPool) && ref.Type != datastore.ContractType(common_utils.LockReleaseTokenPool) {
			continue
		}
		// The program alias is not tied to a mint, so it has no fee vault.
		if ref.Qualifier == solcommon.PoolProgramQualifier {
			continue
		}
		mintType := solcommon.SPLTokens
		mint, err := latestMint(refs, chainSelector, datastore.ContractType(mintType), ref.Qualifier)
		if err != nil {
			mintType = solcommon.SPL2022Tokens
			mint, err = latestMint(refs, chainSelector, datastore.ContractType(mintType), ref.Qualifier)
		}
		if err != nil {
			skipped = append(skipped, ref.Qualifier)
			continue
		}
		tokenProgram, err := solcommon.GetTokenProgramID(mintType)
		if err != nil {
			return nil, nil, err
		}
		poolProgram, err := solcommon.ToAddress(ref)
		if err != nil {
			return nil, nil, fmt.Errorf("resolve %s pool %q: %w", ref.Type, ref.Qualifier, err)
		}
		feeVaultATA, err := soltokens.FindFeeVaultATA(tokenProgram, mint, poolProgram, mint.Bytes())
		if err != nil {
			return nil, nil, fmt.Errorf("derive fee vault ATA for pool %q: %w", ref.Qualifier, err)
		}
		entries = append(entries, feeVaultATA)
	}
	return entries, skipped, nil
}

// latestMint resolves a token mint from the refs at its latest version: a token is the same for every
// version, so lookups never name one.
func latestMint(refs []datastore.AddressRef, chainSelector uint64, mintType datastore.ContractType, qualifier string) (solana.PublicKey, error) {
	ref, err := solcommon.FindLatestRefInRefs(refs, chainSelector, mintType, qualifier)
	if err != nil {
		return solana.PublicKey{}, err
	}
	return solana.PublicKeyFromBase58(ref.Address)
}

func extendLookupTableInChunks(b cldf_ops.Bundle, chain cldf_solana.Chain, table solana.PublicKey, entries []solana.PublicKey) error {
	for start := 0; start < len(entries); start += lutops.MaxEntriesPerExtend {
		end := min(start+lutops.MaxEntriesPerExtend, len(entries))
		if _, err := cldf_ops.ExecuteOperation(b, lutops.Extend, chain, lutops.ExtendParams{
			LookupTable: table,
			Entries:     entries[start:end],
		}); err != nil {
			return fmt.Errorf("entries %d-%d of %d: %w", start, end-1, len(entries), err)
		}
	}
	if err := common.AwaitSlotChange(b.GetContext(), chain.Client); err != nil {
		return fmt.Errorf("await slot change after extending lookup table %s: %w", table, err)
	}
	return nil
}

type ExtendOfframpLutInput struct {
	ChainSelector     uint64
	ExistingAddresses []datastore.AddressRef
}

// ExtendOfframpLut fills the OffRamp lookup table with the protocol account set.
var ExtendOfframpLut = cldf_ops.NewSequence(
	"extend-offramp-lookup-table",
	semver.MustParse("2.0.0"),
	"Fills the OffRamp lookup table with the protocol account set",
	func(b cldf_ops.Bundle, chains cldf_chain.BlockChains, input ExtendOfframpLutInput) (seq_core.OnChainOutput, error) {
		chain, ok := chains.SolanaChains()[input.ChainSelector]
		if !ok {
			return seq_core.OnChainOutput{}, fmt.Errorf("solana chain with selector %d not found", input.ChainSelector)
		}
		accounts, err := resolveOfframpLutAccounts(input.ExistingAddresses, input.ChainSelector)
		if err != nil {
			return seq_core.OnChainOutput{}, err
		}
		entries, err := OfframpLutEntries(accounts)
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("failed to create OffRamp LUT entries: %w", err)
		}
		lookupTable, err := solcommon.OfframpLookupTable(chain, accounts.OffRamp)
		if err != nil {
			return seq_core.OnChainOutput{}, err
		}
		if lookupTable.IsZero() {
			return seq_core.OnChainOutput{}, fmt.Errorf("offramp %s has no lookup table, initialize the OffRamp first", accounts.OffRamp)
		}
		if err = extendLookupTableInChunks(b, chain, lookupTable, entries); err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("extend offramp lookup table: %w", err)
		}
		return seq_core.OnChainOutput{}, nil
	},
)

func resolveOfframpLutAccounts(refs []datastore.AddressRef, chainSelector uint64) (OfframpLutEntriesInput, error) {
	input := ExtendLutInput{ChainSelector: chainSelector, ExistingAddresses: refs}
	var out OfframpLutEntriesInput
	var err error
	if out.LinkToken, err = findLinkMint(refs, chainSelector); err != nil {
		return out, fmt.Errorf("resolve LINK token: %w", err)
	}
	if out.LinkToken.IsZero() {
		return out, fmt.Errorf("resolve LINK token: %w: chain %d type %s", solcommon.ErrAddressRefNotFound, chainSelector, tokensops.LinkContractType)
	}
	if out.OffRamp, err = readAddress(input, datastore.ContractType(offrampops.ContractType), offrampops.Version, ""); err != nil {
		return out, fmt.Errorf("resolve offramp: %w", err)
	}
	if out.Router, err = readAddress(input, datastore.ContractType(routerops.ContractType), routerops.Version, ""); err != nil {
		return out, fmt.Errorf("resolve router: %w", err)
	}
	if out.FeeQuoter, err = readAddress(input, datastore.ContractType(fqops.ContractType), fqops.Version, ""); err != nil {
		return out, fmt.Errorf("resolve fee quoter: %w", err)
	}
	if out.RMNRemote, err = readAddress(input, datastore.ContractType(rmnremoteops.ContractType), rmnremoteops.Version, ""); err != nil {
		return out, fmt.Errorf("resolve RMN Remote: %w", err)
	}
	if out.BurnMintTokenPool, err = readAddress(input, datastore.ContractType(common_utils.BurnMintTokenPool), solcommon.Version2_0_0, solcommon.PoolProgramQualifier); err != nil {
		return out, fmt.Errorf("resolve burn/mint token pool: %w", err)
	}
	if out.LockReleaseTokenPool, err = readAddress(input, datastore.ContractType(common_utils.LockReleaseTokenPool), solcommon.Version2_0_0, solcommon.PoolProgramQualifier); err != nil {
		return out, fmt.Errorf("resolve lock/release token pool: %w", err)
	}
	return out, nil
}

type ExtendLookupTableInput struct {
	ChainSelector uint64
	LookupTable   solana.PublicKey
	Entries       []solana.PublicKey
}

// ExtendLookupTable adds the given entries to a deployer-owned lookup table.
var ExtendLookupTable = cldf_ops.NewSequence(
	"extend-lookup-table",
	semver.MustParse("2.0.0"),
	"Adds the given entries to a deployer-owned address lookup table",
	func(b cldf_ops.Bundle, chains cldf_chain.BlockChains, input ExtendLookupTableInput) (seq_core.OnChainOutput, error) {
		chain, ok := chains.SolanaChains()[input.ChainSelector]
		if !ok {
			return seq_core.OnChainOutput{}, fmt.Errorf("solana chain with selector %d not found", input.ChainSelector)
		}
		if err := extendLookupTableInChunks(b, chain, input.LookupTable, input.Entries); err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("extend lookup table %s: %w", input.LookupTable, err)
		}
		return seq_core.OnChainOutput{}, nil
	},
)
