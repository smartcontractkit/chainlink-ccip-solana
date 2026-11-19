package solcommon

import (
	"fmt"

	"github.com/gagliardetto/solana-go"

	"github.com/smartcontractkit/chainlink-ccip/chains/solana/governance"
	"github.com/smartcontractkit/chainlink-ccip/chains/solana/utils/state"
	common_utils "github.com/smartcontractkit/chainlink-ccip/deployment/utils"
	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/datastore"
	cldf_datastore "github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	cldf_deployment "github.com/smartcontractkit/chainlink-deployments-framework/deployment"
	mcms_solana "github.com/smartcontractkit/mcms/sdk/solana"
	"github.com/smartcontractkit/mcms/types"
)

const (
	TimelockProgramType         cldf_deployment.ContractType = cldf_deployment.ContractType(governance.TimelockProgramType)
	McmProgramType              cldf_deployment.ContractType = cldf_deployment.ContractType(governance.McmProgramType)
	AccessControllerProgramType cldf_deployment.ContractType = cldf_deployment.ContractType(governance.AccessControllerProgramType)
	// special type for Solana that encodes PDA seed usage
	ProposerSeed     cldf_deployment.ContractType = cldf_deployment.ContractType(governance.ProposerSeed)
	CancellerSeed    cldf_deployment.ContractType = cldf_deployment.ContractType(governance.CancellerSeed)
	BypasserSeed     cldf_deployment.ContractType = cldf_deployment.ContractType(governance.BypasserSeed)
	RBACTimelockSeed cldf_deployment.ContractType = cldf_deployment.ContractType(governance.RBACTimelockSeed)
	// access controller accounts
	ProposerAccessControllerAccount  cldf_deployment.ContractType = cldf_deployment.ContractType(governance.ProposerAccessControllerAccount)
	ExecutorAccessControllerAccount  cldf_deployment.ContractType = cldf_deployment.ContractType(governance.ExecutorAccessControllerAccount)
	CancellerAccessControllerAccount cldf_deployment.ContractType = cldf_deployment.ContractType(governance.CancellerAccessControllerAccount)
	BypasserAccessControllerAccount  cldf_deployment.ContractType = cldf_deployment.ContractType(governance.BypasserAccessControllerAccount)
)

// TransferOwnershipParams contains common parameters for transferring ownership of a program.
type TransferOwnershipParams = governance.TransferOwnershipParams

// MCMSQualifierOrDefault defaults an empty qualifier to the CLLCCIP MCMS
// instance. An empty qualifier matches every instance in GetAddressRef, which
// would mix refs of different MCMS deployments, so readers must never pass
// one through.
func MCMSQualifierOrDefault(qualifier string) string {
	if qualifier == "" {
		return common_utils.CLLQualifier
	}
	return qualifier
}

// FindMCMSRef resolves an MCMS infrastructure ref at 1.6.0. There is no v2 MCMS
// deployment: v2 reuses the shared 1.6 MCMS, so its refs are the only ones that
// exist. An unresolved ref is an error that names it, so proposal building never
// degenerates into a zero address or a panic downstream.
func FindMCMSRef(
	existingAddresses []cldf_datastore.AddressRef,
	chainSelector uint64,
	contractType cldf_deployment.ContractType,
	qualifier string) (cldf_datastore.AddressRef, error) {
	ref := datastore.GetAddressRef(existingAddresses, chainSelector, contractType, common_utils.Version_1_6_0, qualifier)
	if ref.Address == "" {
		return cldf_datastore.AddressRef{}, fmt.Errorf(
			"no %s address for chain %d at version %s with qualifier %q",
			contractType, chainSelector, common_utils.Version_1_6_0, qualifier)
	}
	return ref, nil
}

// GetTimelockSignerPDA resolves the timelock signer PDA for a chain from the
// datastore. The qualifier identifies which timelock instance to use.
//
// The ref is read at 1.6.0: there is no v2 MCMS deployment, v2 reuses the
// shared 1.6 MCMS, so its refs are the only ones that exist.
//
// An unresolved ref is an error rather than a zero PDA. Deriving from an empty
// address yields a well-formed but meaningless key, and the callers use it as
// a token freeze authority or route admin, so a silent fallback would hand
// those to an address nobody holds.
func GetTimelockSignerPDA(
	existingAddresses []cldf_datastore.AddressRef,
	chainSelector uint64,
	qualifier string) (solana.PublicKey, error) {
	signer, found, err := FindTimelockSignerPDA(existingAddresses, chainSelector, qualifier)
	if err != nil {
		return solana.PublicKey{}, err
	}
	if !found {
		return solana.PublicKey{}, fmt.Errorf(
			"no %s address for chain %d with qualifier %q", common_utils.RBACTimelock, chainSelector, qualifier)
	}

	return signer, nil
}

// FindTimelockSignerPDA is GetTimelockSignerPDA for callers that can still do
// something useful without a timelock - an environment deployed without MCMS
// (the devenv, for one) has no ref at all, and reports found=false here rather
// than failing. A ref that exists but does not parse is still an error: that is
// a broken environment, not one without governance.
func FindTimelockSignerPDA(
	existingAddresses []cldf_datastore.AddressRef,
	chainSelector uint64,
	qualifier string) (solana.PublicKey, bool, error) {
	// timelock seeds are stored under a separate program type; there is no v2 MCMS
	// deployment, so the shared 1.6 ref is the only one that exists
	timelock := datastore.GetAddressRef(
		existingAddresses,
		chainSelector,
		common_utils.RBACTimelock,
		common_utils.Version_1_6_0,
		qualifier,
	)
	if timelock.Address == "" {
		return solana.PublicKey{}, false, nil
	}
	id, seed, err := mcms_solana.ParseContractAddress(timelock.Address)
	if err != nil {
		return solana.PublicKey{}, false, fmt.Errorf(
			"parse timelock address %q for chain %d: %w", timelock.Address, chainSelector, err)
	}

	return state.GetTimelockSignerPDA(id, state.PDASeed(seed[:])), true, nil
}

// ParseOwnerAddress parses an owner as a plain base58 public key, or as an MCMS
// contract address ("program.seed") whose timelock signer PDA is the owner. The
// deploy framework hands the timelock's datastore address over as the proposed
// owner (deploy.TransferToTimelock), while program configs are owned by its
// signer PDA.
func ParseOwnerAddress(owner string) (solana.PublicKey, error) {
	key, err := solana.PublicKeyFromBase58(owner)
	if err == nil {
		return key, nil
	}
	id, seed, perr := mcms_solana.ParseContractAddress(owner)
	if perr != nil {
		return solana.PublicKey{}, fmt.Errorf(
			"owner %q is neither a base58 public key nor an MCMS contract address: %w", owner, err)
	}
	return state.GetTimelockSignerPDA(id, state.PDASeed(seed[:])), nil
}

func BuildMCMSBatchOperation(
	chainSelector uint64,
	ixns []solana.Instruction,
	programID string,
	contractType string) (types.BatchOperation, error) {
	return governance.BuildMCMSBatchOperation(chainSelector, ixns, programID, contractType)
}
