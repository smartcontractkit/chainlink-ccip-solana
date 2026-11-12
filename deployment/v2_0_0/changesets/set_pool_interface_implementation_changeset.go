package changesets

import (
	"fmt"

	"github.com/gagliardetto/solana-go"

	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/changesets"
	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/mcms"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	cldf "github.com/smartcontractkit/chainlink-deployments-framework/deployment"
	cldf_ops "github.com/smartcontractkit/chainlink-deployments-framework/operations"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/ccip_router"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	routerops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/router"
)

// Account resolution modes accepted by SetPoolInterfaceImplementationChangesetInput.
const (
	AccountResolutionNoResolution               = "NoResolution"
	AccountResolutionStandard                   = "Standard"
	AccountResolutionWithoutExtendedLookupTable = "WithoutExtendedLookupTable"
)

// SetPoolInterfaceImplementationChangesetInput selects the Token Admin Registry entry to edit.
type SetPoolInterfaceImplementationChangesetInput struct {
	ChainSelector uint64 `json:"chainSelector"`
	// TokenMint is the base58 mint whose Token Admin Registry entry is edited.
	TokenMint string `json:"tokenMint"`
	// InterfaceVersion is the pool interface the registry records: 1 (CCIP 1.6) or 2 (CCIP 2.0+).
	InterfaceVersion uint8 `json:"interfaceVersion"`
	// AccountResolution is NoResolution, Standard or WithoutExtendedLookupTable. Each interface version
	// takes a single one, as the Router enforces: NoResolution for 1 and WithoutExtendedLookupTable for
	// 2, so Standard is always rejected. Since TAR v4 every mode keeps the registry decodable by the 1.6
	// OffRamp: WithoutExtendedLookupTable is stored as Standard in the legacy byte plus a trailing skip
	// flag.
	AccountResolution string     `json:"accountResolution"`
	MCMS              mcms.Input `json:"mcms"`
}

// SetPoolInterfaceImplementationChangeset edits a mint's Token Admin Registry interface version
// and account resolution mode with the router's set_pool_interface_implementation instruction.
//
// It was introduced as a stopgap for the 1.6 CCTP lane outage, when a TAR v3 registry holding
// WithoutExtendedLookupTable (enum value 2) broke 1.6 execution and the workaround was to set
// Standard. TAR v4 removed that reason: WithoutExtendedLookupTable is now stored as Standard in the
// legacy byte plus a trailing skip flag, so every mode is decodable by the 1.6 OffRamp. A v2 pool
// now takes only WithoutExtendedLookupTable, so this changeset can no longer set Standard. After the
// Router upgrade to TAR v4, use it (or re-run MigrateCctpTokenPoolToV2Changeset) to move a registry
// left at Standard to WithoutExtendedLookupTable.
//
// It is idempotent: a registry already at the requested values produces no instruction.
func SetPoolInterfaceImplementationChangeset() cldf.ChangeSetV2[SetPoolInterfaceImplementationChangesetInput] {
	return cldf.CreateChangeSet(setPoolInterfaceImplementationApply, setPoolInterfaceImplementationVerify)
}

func parseAccountResolution(s string) (uint8, error) {
	switch s {
	case AccountResolutionNoResolution:
		return uint8(ccip_router.ResolutionType_NoResolution), nil
	case AccountResolutionStandard:
		return uint8(ccip_router.ResolutionType_Standard), nil
	case AccountResolutionWithoutExtendedLookupTable:
		return uint8(ccip_router.ResolutionType_WithoutExtendedLookupTable), nil
	default:
		return 0, fmt.Errorf("invalid accountResolution %q: must be %s, %s or %s", s,
			AccountResolutionNoResolution, AccountResolutionStandard, AccountResolutionWithoutExtendedLookupTable)
	}
}

func setPoolInterfaceImplementationVerify(env cldf.Environment, input SetPoolInterfaceImplementationChangesetInput) error {
	if err := cldf.IsValidChainSelector(input.ChainSelector); err != nil {
		return fmt.Errorf("invalid chain selector %d: %w", input.ChainSelector, err)
	}
	if !env.BlockChains.Exists(input.ChainSelector) {
		return fmt.Errorf("chain with selector %d does not exist", input.ChainSelector)
	}
	if _, ok := env.BlockChains.SolanaChains()[input.ChainSelector]; !ok {
		return fmt.Errorf("chain with selector %d is not a Solana chain", input.ChainSelector)
	}
	if _, err := solana.PublicKeyFromBase58(input.TokenMint); err != nil {
		return fmt.Errorf("invalid token mint %q: %w", input.TokenMint, err)
	}
	if input.InterfaceVersion != 1 && input.InterfaceVersion != 2 {
		return fmt.Errorf("invalid interfaceVersion %d: must be 1 or 2", input.InterfaceVersion)
	}
	resolution, parseErr := parseAccountResolution(input.AccountResolution)
	if parseErr != nil {
		return parseErr
	}
	if err := routerops.ValidatePoolInterfaceImplementation(input.InterfaceVersion, ccip_router.ResolutionType(resolution)); err != nil {
		return err
	}
	if err := validateMCMSInput(env.Logger, input.MCMS); err != nil {
		return fmt.Errorf("failed to validate MCMS input: %w", err)
	}
	if _, err := solcommon.FindLatestRef(env.DataStore, datastore.AddressRef{Type: datastore.ContractType(routerops.ContractType)}, input.ChainSelector); err != nil {
		return fmt.Errorf("find router on chain %d: %w", input.ChainSelector, err)
	}
	return nil
}

func setPoolInterfaceImplementationApply(e cldf.Environment, input SetPoolInterfaceImplementationChangesetInput) (cldf.ChangesetOutput, error) {
	chain := e.BlockChains.SolanaChains()[input.ChainSelector]
	mint, err := solana.PublicKeyFromBase58(input.TokenMint)
	if err != nil {
		return cldf.ChangesetOutput{}, fmt.Errorf("invalid token mint %q: %w", input.TokenMint, err)
	}
	resolution, err := parseAccountResolution(input.AccountResolution)
	if err != nil {
		return cldf.ChangesetOutput{}, err
	}
	routerRef, err := solcommon.FindLatestRef(e.DataStore, datastore.AddressRef{Type: datastore.ContractType(routerops.ContractType)}, input.ChainSelector)
	if err != nil {
		return cldf.ChangesetOutput{}, fmt.Errorf("find router on chain %d: %w", input.ChainSelector, err)
	}
	router, err := solana.PublicKeyFromBase58(routerRef.Address)
	if err != nil {
		return cldf.ChangesetOutput{}, fmt.Errorf("parse router address %q: %w", routerRef.Address, err)
	}

	report, err := cldf_ops.ExecuteOperation(e.OperationsBundle, routerops.SetPoolInterfaceImplementation, chain, routerops.SetPoolInterfaceImplParams{
		Router:                      router,
		TokenMint:                   mint,
		InterfaceVersion:            input.InterfaceVersion,
		ImplementsAccountResolution: resolution,
	})
	if err != nil {
		return cldf.ChangesetOutput{}, fmt.Errorf("set pool interface implementation: %w", err)
	}
	if err := requireMCMSForBatchOps(e.Logger, input.MCMS, report.Output.BatchOps); err != nil {
		return cldf.ChangesetOutput{}, err
	}
	return changesets.
		NewOutputBuilder(e, changesets.GetRegistry()).
		WithReports([]cldf_ops.Report[any, any]{report.ToGenericReport()}).
		WithBatchOps(report.Output.BatchOps).
		Build(input.MCMS)
}
