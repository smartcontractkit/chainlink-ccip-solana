package sequences

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/Masterminds/semver/v3"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	"github.com/smartcontractkit/chainlink-ccip/deployment/finality"
	common_utils "github.com/smartcontractkit/chainlink-ccip/deployment/utils"
	seq_core "github.com/smartcontractkit/chainlink-ccip/deployment/utils/sequences"
	cldf_chain "github.com/smartcontractkit/chainlink-deployments-framework/chain"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	cldf_ops "github.com/smartcontractkit/chainlink-deployments-framework/operations"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/burnmint_token_pool"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/ccip_router"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/tokens"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	offrampops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/offramp"
	routerops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/router"
	tokenpoolops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/tokenpools"
)

// MigrateTokenPoolToV2Input selects the token pool instance (pool program + mint) that migrates
// from the 1.6 interface to 2.0, and the 2.0 lanes it is checked against.
type MigrateTokenPoolToV2Input struct {
	DataStore     datastore.DataStore
	ChainSelector uint64
	// PoolType is BurnMintTokenPool or LockReleaseTokenPool.
	PoolType  string
	TokenMint solana.PublicKey
	// PoolProgramQualifier selects the pool program: empty or CLLCCIP is CLL's shared program, any
	// other value is a white-glove program registered under that qualifier (the 1.6 pool metadata).
	PoolProgramQualifier string
	// TokenPoolQualifier is the qualifier the migrated instance is registered under. Empty derives it
	// from the token's datastore ref, as 2.0 tooling looks pool instances up by the token qualifier.
	TokenPoolQualifier string
	// MCMSQualifier selects the timelock that, besides the deployer, counts as ours when checking
	// who controls the pool.
	MCMSQualifier string
	// RemoteChainSelectors are the lanes the pool is migrated for. Each must be a 2.0 lane on the
	// Router and already configured on the pool.
	RemoteChainSelectors []uint64
	// AllowedFinalityConfig, when set, is written to the pool's ChainConfigV2 of every remote in
	// RemoteChainSelectors. Nil keeps an existing config's finality and initializes a missing one
	// at the default, WAIT_FOR_FINALITY.
	AllowedFinalityConfig *finality.Config
}

// TokenPoolMigrationTarget is the pool instance a migration acts on, resolved from the datastore.
type TokenPoolMigrationTarget struct {
	// Program is the pool program, shared by every mint of the pool type deployed on it.
	Program solana.PublicKey
	// Qualifier is the pool instance qualifier (the token's qualifier).
	Qualifier string
	// Version is the program's 2.x ref version, the version the migrated instance is registered at.
	Version *semver.Version
	// RegisterRef reports whether the instance still lacks its 2.x ref (false on a re-run).
	RegisterRef bool
}

// MigrateTokenPoolToV2 makes an existing 1.6 BurnMint or LockRelease pool instance usable by 2.0
// lanes. The pool program is shared by every mint on it, so the program upgrade is a separate,
// earlier step (UpgradeProgramChangeset); this sequence migrates one mint's instance:
//
//  1. The pool signer and fee vault token accounts (EnsurePoolTokenAccounts), which a 1.6 instance
//     lacks.
//  2. The pool's ChainConfigV2 of each migrated lane, so every 2.0 lane has its config account. A
//     missing one is initialized with the default config (WAIT_FOR_FINALITY, no pool CCVs, fee
//     disabled), or with AllowedFinalityConfig when given; an existing one is left as is apart from
//     that finality. It runs before the registry flip so the config is in place when 2.0 lanes
//     start using the pool. It is signed by the pool owner, so it is batched into the MCMS
//     proposal when the timelock owns the pool.
//  3. The Token Admin Registry flip to interface version 2 with WithoutExtendedLookupTable
//     resolution, the same state ConfigureTokenForTransfers leaves a fresh 2.0 pool in. The 1.6
//     ccip_send path never reads the registry version, so live 1.6 lanes are unaffected.
//  4. The instance's 2.x datastore ref at the program address, under the instance qualifier, so
//     2.0 tooling (PoolRefByQualifier) resolves the pool at the upgraded version.
//
// The pool's lookup table needs no change: 1.6 and 2.0 use the same layout. Every step is
// idempotent, so the sequence can be re-run.
var MigrateTokenPoolToV2 = cldf_ops.NewSequence(
	"migrate-token-pool-to-v2",
	semver.MustParse("2.0.0"),
	"Migrates an existing 1.6 BurnMint or LockRelease token pool instance so 2.0 lanes can use it",
	func(b cldf_ops.Bundle, chains cldf_chain.BlockChains, input MigrateTokenPoolToV2Input) (seq_core.OnChainOutput, error) {
		var result seq_core.OnChainOutput
		chain, ok := chains.SolanaChains()[input.ChainSelector]
		if !ok {
			return result, fmt.Errorf("solana chain with selector %d not found", input.ChainSelector)
		}
		if input.DataStore == nil {
			return result, errors.New("datastore is required")
		}
		if input.TokenMint.IsZero() {
			return result, errors.New("token mint is required")
		}

		target, err := ResolveTokenPoolMigration(input.DataStore, input.ChainSelector, input.PoolType, input.PoolProgramQualifier, input.TokenMint, input.TokenPoolQualifier)
		if err != nil {
			return result, err
		}
		existing := input.DataStore.Addresses().Filter(datastore.AddressRefByChainSelector(input.ChainSelector))
		checks, err := NewTokenPoolMigrationChecks(existing, chain, input.ChainSelector, input.PoolType, input.MCMSQualifier)
		if err != nil {
			return result, err
		}
		if err = checks.Run(b.GetContext(), chain, target.Program, input.TokenMint, input.RemoteChainSelectors); err != nil {
			return result, err
		}
		router := checks.Router
		tokenProgram, err := solcommon.FetchTokenProgramID(b.GetContext(), chain, input.TokenMint)
		if err != nil {
			return result, err
		}

		if err = ensurePoolTokenAccounts(b, chain, existing, target.Program, input.TokenMint, tokenProgram); err != nil {
			return result, err
		}

		// Step 2: ChainConfigV2 of every migrated lane.
		op, err := chainConfigV2Op(input.PoolType)
		if err != nil {
			return result, err
		}
		var finalityCfg *burnmint_token_pool.FinalityConfig
		if input.AllowedFinalityConfig != nil {
			cfg := toSolanaFinalityConfig(*input.AllowedFinalityConfig)
			finalityCfg = &cfg
		}
		for _, remote := range input.RemoteChainSelectors {
			if err = runOp(b, op, chain, tokenpoolops.SetChainConfigV2Input{
				TokenPool:             target.Program,
				TokenMint:             input.TokenMint,
				RemoteChainSelector:   remote,
				AllowedFinalityConfig: finalityCfg,
			}, &result.BatchOps); err != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("set chain config v2 for remote chain %d: %w", remote, err)
			}
		}

		// Step 3: Token Admin Registry flip.
		if err = runOp(b, routerops.SetPoolInterfaceImplementation, chain, routerops.SetPoolInterfaceImplParams{
			Router:                      router,
			TokenMint:                   input.TokenMint,
			InterfaceVersion:            2,
			ImplementsAccountResolution: uint8(ccip_router.ResolutionType_WithoutExtendedLookupTable),
		}, &result.BatchOps); err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("set pool interface implementation: %w", err)
		}

		// Step 4: the instance's 2.x ref.
		if target.RegisterRef {
			result.Addresses = append(result.Addresses, datastore.AddressRef{
				Address:       target.Program.String(),
				ChainSelector: input.ChainSelector,
				Type:          datastore.ContractType(input.PoolType),
				Version:       target.Version,
				Qualifier:     target.Qualifier,
			})
		}
		return result, nil
	},
)

// ValidateMigratablePoolType rejects pool types this migration does not cover.
func ValidateMigratablePoolType(poolType string) error {
	switch poolType {
	case common_utils.BurnMintTokenPool.String(), common_utils.LockReleaseTokenPool.String():
		return nil
	case common_utils.CCTPTokenPool.String():
		return fmt.Errorf("token pool type %q migrates with MigrateCctpTokenPoolToV2Changeset", poolType)
	default:
		return fmt.Errorf("token pool type %q is not supported; expected %s or %s",
			poolType, common_utils.BurnMintTokenPool, common_utils.LockReleaseTokenPool)
	}
}

// CLLPoolProgramQualifier is the 1.6 pool metadata of CLL's shared pool programs. An empty
// qualifier means the same; any other qualifier is a white-glove program.
const CLLPoolProgramQualifier = "CLLCCIP"

// ResolveTokenPoolMigration finds the pool program selected by programQualifier in the datastore,
// checks it was upgraded to 2.x, and returns the instance ref the migration registers for mint.
// tokenQualifier, when empty, is taken from the mint's token ref.
//
// The program is selected by its qualifier (the 1.6 pool metadata), never by address alone: CLL's
// shared program and each white-glove program are registered under their own qualifier.
func ResolveTokenPoolMigration(ds datastore.DataStore, chainSelector uint64, poolType, programQualifier string, mint solana.PublicKey, tokenQualifier string) (TokenPoolMigrationTarget, error) {
	if err := ValidateMigratablePoolType(poolType); err != nil {
		return TokenPoolMigrationTarget{}, err
	}
	program, err := resolvePoolProgram(ds, chainSelector, poolType, programQualifier)
	if err != nil {
		return TokenPoolMigrationTarget{}, err
	}
	address := program.String()

	// The upgrade registers a 2.x ref at the program address (under its own qualifier and the
	// TokenPoolProgram alias); any of them shows this program runs the 2.0 binary.
	programRefs := ds.Addresses().Filter(
		datastore.AddressRefByChainSelector(chainSelector),
		datastore.AddressRefByType(datastore.ContractType(poolType)),
		datastore.AddressRefByAddress(address),
	)
	var upgraded *semver.Version
	for _, ref := range programRefs {
		if solcommon.RefVersionMatches(ref.Version, solcommon.Version2_0_0) && (upgraded == nil || ref.Version.GreaterThan(upgraded)) {
			upgraded = ref.Version
		}
	}
	if upgraded == nil {
		return TokenPoolMigrationTarget{}, fmt.Errorf("pool program %s (%s) has no 2.x ref on chain %d; upgrade it in place with UpgradeProgramChangeset first",
			address, poolType, chainSelector)
	}

	if tokenQualifier == "" {
		if tokenQualifier, err = TokenQualifierForMint(ds, chainSelector, mint); err != nil {
			return TokenPoolMigrationTarget{}, err
		}
	}
	registerRef := true
	for _, ref := range ds.Addresses().Filter(
		datastore.AddressRefByChainSelector(chainSelector),
		datastore.AddressRefByType(datastore.ContractType(poolType)),
		datastore.AddressRefByQualifier(tokenQualifier),
	) {
		if !solcommon.RefVersionMatches(ref.Version, solcommon.Version2_0_0) {
			continue
		}
		if ref.Address != address {
			return TokenPoolMigrationTarget{}, fmt.Errorf("%s ref %s with qualifier %q on chain %d is at %s, not at pool program %s",
				poolType, ref.Version, tokenQualifier, chainSelector, ref.Address, address)
		}
		registerRef = false
	}
	return TokenPoolMigrationTarget{
		Program:     program,
		Qualifier:   tokenQualifier,
		Version:     upgraded,
		RegisterRef: registerRef,
	}, nil
}

// resolvePoolProgram returns the address of the poolType program registered under programQualifier.
func resolvePoolProgram(ds datastore.DataStore, chainSelector uint64, poolType, programQualifier string) (solana.PublicKey, error) {
	qualifiers := []string{programQualifier}
	if programQualifier == "" || programQualifier == CLLPoolProgramQualifier {
		qualifiers = []string{"", CLLPoolProgramQualifier}
	}
	var address string
	for _, ref := range ds.Addresses().Filter(
		datastore.AddressRefByChainSelector(chainSelector),
		datastore.AddressRefByType(datastore.ContractType(poolType)),
	) {
		if !slices.Contains(qualifiers, ref.Qualifier) {
			continue
		}
		if address != "" && ref.Address != address {
			return solana.PublicKey{}, fmt.Errorf("ambiguous %s programs under qualifier %q on chain %d: %s and %s",
				poolType, programQualifier, chainSelector, address, ref.Address)
		}
		address = ref.Address
	}
	if address == "" {
		return solana.PublicKey{}, fmt.Errorf("no %s program registered under qualifier %q on chain %d", poolType, programQualifier, chainSelector)
	}
	program, err := solana.PublicKeyFromBase58(address)
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("parse %s program address %q: %w", poolType, address, err)
	}
	return program, nil
}

// TokenQualifierForMint returns the qualifier of mint's token ref, which its pool instance shares.
func TokenQualifierForMint(ds datastore.DataStore, chainSelector uint64, mint solana.PublicKey) (string, error) {
	refs := ds.Addresses().Filter(
		datastore.AddressRefByChainSelector(chainSelector),
		datastore.AddressRefByAddress(mint.String()),
	)
	var qualifiers []string
	for _, ref := range refs {
		if ref.Type != datastore.ContractType(solcommon.SPLTokens) && ref.Type != datastore.ContractType(solcommon.SPL2022Tokens) {
			continue
		}
		if !slices.Contains(qualifiers, ref.Qualifier) {
			qualifiers = append(qualifiers, ref.Qualifier)
		}
	}
	switch len(qualifiers) {
	case 0:
		return "", fmt.Errorf("token mint %s has no token ref on chain %d; set tokenPoolQualifier", mint, chainSelector)
	case 1:
		if qualifiers[0] == "" {
			return "", fmt.Errorf("token mint %s has a token ref without qualifier on chain %d; set tokenPoolQualifier", mint, chainSelector)
		}
		return qualifiers[0], nil
	default:
		return "", fmt.Errorf("token mint %s has token refs under several qualifiers %v on chain %d; set tokenPoolQualifier", mint, qualifiers, chainSelector)
	}
}

// TokenPoolMigrationChecks holds what the on-chain preconditions of a migration are checked against.
type TokenPoolMigrationChecks struct {
	Router   solana.PublicKey
	OffRamp  solana.PublicKey
	PoolType string
	// Deployer and Timelock are the keys that may control the pool. Timelock is the signer of the
	// timelock selected by MCMSQualifier, nil when the chain has none.
	Deployer      *solana.PublicKey
	Timelock      *solana.PublicKey
	MCMSQualifier string
	// OtherTimelocks maps the signers of the chain's other timelocks to their MCMS qualifier.
	OtherTimelocks map[solana.PublicKey]string
}

// NewTokenPoolMigrationChecks resolves the 2.0 Router and OffRamp and the keys that may control the
// pool. A named MCMS qualifier without a timelock is an error.
func NewTokenPoolMigrationChecks(refs []datastore.AddressRef, chain cldf_solana.Chain, chainSelector uint64, poolType, mcmsQualifier string) (TokenPoolMigrationChecks, error) {
	router, err := refPubkey(refs, chainSelector, routerops.ContractType, routerops.Version)
	if err != nil {
		return TokenPoolMigrationChecks{}, fmt.Errorf("resolve 2.0 Router (upgrade it first): %w", err)
	}
	offRamp, err := refPubkey(refs, chainSelector, offrampops.ContractType, offrampops.Version)
	if err != nil {
		return TokenPoolMigrationChecks{}, fmt.Errorf("resolve 2.0 OffRamp (deploy it first): %w", err)
	}
	checks := TokenPoolMigrationChecks{
		Router:         router,
		OffRamp:        offRamp,
		PoolType:       poolType,
		MCMSQualifier:  mcmsQualifier,
		OtherTimelocks: map[solana.PublicKey]string{},
	}
	if chain.DeployerKey != nil {
		deployer := chain.DeployerKey.PublicKey()
		checks.Deployer = &deployer
	}
	for _, ref := range refs {
		if ref.ChainSelector != chainSelector || ref.Type != datastore.ContractType(common_utils.RBACTimelock) {
			continue
		}
		signer, found, err := solcommon.FindTimelockSignerPDA(refs, chainSelector, ref.Qualifier)
		if err != nil {
			return TokenPoolMigrationChecks{}, fmt.Errorf("resolve signer of timelock %q: %w", ref.Qualifier, err)
		}
		switch {
		case !found:
		case ref.Qualifier == mcmsQualifier:
			checks.Timelock = &signer
		default:
			checks.OtherTimelocks[signer] = ref.Qualifier
		}
	}
	if mcmsQualifier != "" && checks.Timelock == nil {
		return TokenPoolMigrationChecks{}, fmt.Errorf("no timelock for MCMS qualifier %q on chain %d", mcmsQualifier, chainSelector)
	}
	return checks, nil
}

// requireControlled fails unless key, which controls what, is the deployer or the selected timelock.
func (c TokenPoolMigrationChecks) requireControlled(what string, key solana.PublicKey) error {
	if (c.Deployer != nil && key.Equals(*c.Deployer)) || (c.Timelock != nil && key.Equals(*c.Timelock)) {
		return nil
	}
	if qualifier, ok := c.OtherTimelocks[key]; ok {
		return fmt.Errorf("%s is controlled by the timelock of MCMS qualifier %q, not %q; set mcms.qualifier to %q",
			what, qualifier, c.MCMSQualifier, qualifier)
	}
	if c.Timelock == nil {
		return fmt.Errorf("%s is controlled by %s, which is neither the deployer nor a known timelock: set mcms.qualifier if it is a timelock signer; otherwise CLL does not control it and a self-served pool is migrated by its owner",
			what, key)
	}
	return fmt.Errorf("%s is controlled by %s, which is not CLL's: a self-served pool is migrated by its owner", what, key)
}

// Run checks that CLL controls the pool (so it is CLL's or white-glove, not self-served), that the
// Token Admin Registry routes mint to program, and that every remote is a 2.0 lane in both
// directions that the pool is already configured for.
func (c TokenPoolMigrationChecks) Run(ctx context.Context, chain cldf_solana.Chain, program, mint solana.PublicKey, remotes []uint64) error {
	if len(remotes) == 0 {
		return errors.New("at least one remote chain selector is required")
	}

	authority, err := solcommon.GetUpgradeAuthority(chain.Client, program)
	if err != nil {
		return fmt.Errorf("read upgrade authority of pool program %s: %w", program, err)
	}
	if err = c.requireControlled(fmt.Sprintf("pool program %s", program), authority); err != nil {
		return err
	}
	poolConfig, err := tokens.TokenPoolConfigAddress(mint, program)
	if err != nil {
		return fmt.Errorf("derive pool config for mint %s: %w", mint, err)
	}
	if err = requireProgramAccount(ctx, chain, poolConfig, program); err != nil {
		return fmt.Errorf("pool for mint %s is not initialized on program %s: %w", mint, program, err)
	}
	getOwner := tokenpoolops.GetAuthorityBurnMint
	if c.PoolType == common_utils.LockReleaseTokenPool.String() {
		getOwner = tokenpoolops.GetAuthorityLockRelease
	}
	owner, err := getOwner(ctx, chain, program, mint)
	if err != nil {
		return fmt.Errorf("read owner of the pool for mint %s: %w", mint, err)
	}
	if err = c.requireControlled(fmt.Sprintf("the pool for mint %s", mint), owner); err != nil {
		return err
	}

	tarPDA, _, err := state.FindTokenAdminRegistryPDA(mint, c.Router)
	if err != nil {
		return fmt.Errorf("derive token admin registry for mint %s: %w", mint, err)
	}
	tar, _, err := routerops.ReadTokenAdminRegistry(ctx, chain.Client, tarPDA)
	if err != nil {
		return fmt.Errorf("read token admin registry for mint %s (was the pool registered on 1.6?): %w", mint, err)
	}
	if !tar.Administrator.IsZero() {
		if err = c.requireControlled(fmt.Sprintf("the token admin registry of mint %s", mint), tar.Administrator); err != nil {
			return err
		}
	}
	table, err := solcommon.ReadLookupTable(ctx, chain, tar.LookupTable)
	if err != nil {
		return fmt.Errorf("read pool lookup table of mint %s: %w", mint, err)
	}
	// SetPool writes the pool program at index 2 of the token's lookup table.
	const poolProgramIndex = 2
	if len(table.Addresses) <= poolProgramIndex || !table.Addresses[poolProgramIndex].Equals(program) {
		return fmt.Errorf("token admin registry of mint %s does not route to pool program %s", mint, program)
	}

	for _, remote := range remotes {
		laneUpgraded, laneErr := routerops.LaneExists(ctx, chain, c.Router, remote)
		if laneErr != nil {
			return laneErr
		}
		if !laneUpgraded {
			return fmt.Errorf("remote chain %d has no 2.0 lane on Router %s; upgrade the lane before migrating the pool", remote, c.Router)
		}
		// The 2.0 OffRamp is its own program (the 1.6 one is not upgraded in place), so its source
		// chain account exists only once the inbound 2.0 lane is configured.
		sourceChain, _, srcErr := state.FindOfframpSourceChainPDA(remote, c.OffRamp)
		if srcErr != nil {
			return fmt.Errorf("derive OffRamp source chain for remote chain %d: %w", remote, srcErr)
		}
		if err = requireProgramAccount(ctx, chain, sourceChain, c.OffRamp); err != nil {
			return fmt.Errorf("remote chain %d has no 2.0 inbound lane on OffRamp %s; upgrade the lane before migrating the pool: %w", remote, c.OffRamp, err)
		}
		// The pool's base ChainConfig (remote pools, remote token, rate limits) is a required account of
		// both the 1.6 and the 2.0 burn/mint instructions; ChainConfigV2 only adds to it. This reads
		// Solana-side state only, whatever version the remote chain's pool runs.
		chainConfig, _, cfgErr := tokens.TokenPoolChainConfigPDA(remote, mint, program)
		if cfgErr != nil {
			return fmt.Errorf("derive pool chain config for remote chain %d: %w", remote, cfgErr)
		}
		if err = requireProgramAccount(ctx, chain, chainConfig, program); err != nil {
			return fmt.Errorf("pool for mint %s is not configured for remote chain %d: %w", mint, remote, err)
		}
	}
	return nil
}

// requireProgramAccount fails unless account exists and is owned by program.
func requireProgramAccount(ctx context.Context, chain cldf_solana.Chain, account, program solana.PublicKey) error {
	info, err := chain.Client.GetAccountInfoWithOpts(ctx, account, &rpc.GetAccountInfoOpts{Commitment: cldf_solana.SolDefaultCommitment})
	if errors.Is(err, rpc.ErrNotFound) {
		return fmt.Errorf("account %s not found", account)
	}
	if err != nil {
		return fmt.Errorf("read account %s: %w", account, err)
	}
	if info == nil || info.Value == nil {
		return fmt.Errorf("account %s not found", account)
	}
	if !info.Value.Owner.Equals(program) {
		return fmt.Errorf("account %s is owned by %s, not %s", account, info.Value.Owner, program)
	}
	return nil
}
