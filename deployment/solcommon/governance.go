package solcommon

import (
	"fmt"
	"math/big"

	"github.com/gagliardetto/solana-go"

	"github.com/smartcontractkit/chainlink-ccip/chains/solana/utils/state"
	common_utils "github.com/smartcontractkit/chainlink-ccip/deployment/utils"
	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/datastore"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	cldf_datastore "github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	cldf_deployment "github.com/smartcontractkit/chainlink-deployments-framework/deployment"
	mcms_solana "github.com/smartcontractkit/mcms/sdk/solana"
	"github.com/smartcontractkit/mcms/types"
)

const (
	TimelockProgramType         cldf_deployment.ContractType = "RBACTimelockProgram"
	McmProgramType              cldf_deployment.ContractType = "ManyChainMultiSigProgram"
	AccessControllerProgramType cldf_deployment.ContractType = "AccessControllerProgram"
	// special type for Solana that encodes PDA seed usage
	ProposerSeed     cldf_deployment.ContractType = "ProposerSeed"
	CancellerSeed    cldf_deployment.ContractType = "CancellerSeed"
	BypasserSeed     cldf_deployment.ContractType = "BypasserSeed"
	RBACTimelockSeed cldf_deployment.ContractType = "RBACTimelockSeed"
	// access controller accounts
	ProposerAccessControllerAccount  cldf_deployment.ContractType = "ProposerAccessControllerAccount"
	ExecutorAccessControllerAccount  cldf_deployment.ContractType = "ExecutorAccessControllerAccount"
	CancellerAccessControllerAccount cldf_deployment.ContractType = "CancellerAccessControllerAccount"
	BypasserAccessControllerAccount  cldf_deployment.ContractType = "BypasserAccessControllerAccount"
)

// TransferOwnershipParams contains common parameters for transferring ownership of a program.
type TransferOwnershipParams struct {
	Program      solana.PublicKey
	CurrentOwner solana.PublicKey
	NewOwner     solana.PublicKey
}

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
	txns := make([]types.Transaction, 0, len(ixns))
	for _, ixn := range ixns {
		data, err := ixn.Data()
		if err != nil {
			return types.BatchOperation{}, fmt.Errorf("failed to extract data: %w", err)
		}
		for _, account := range ixn.Accounts() {
			if account.IsSigner {
				account.IsSigner = false
			}
		}
		tx, err := mcms_solana.NewTransaction(
			programID,
			data,
			big.NewInt(0),  // e.g. value
			ixn.Accounts(), // pass along needed accounts
			contractType,   // some string identifying the target
			[]string{},     // any relevant metadata
		)
		if err != nil {
			return types.BatchOperation{}, fmt.Errorf("failed to create transaction: %w", err)
		}
		txns = append(txns, tx)
	}
	return types.BatchOperation{
		ChainSelector: types.ChainSelector(chainSelector),
		Transactions:  txns,
	}, nil
}

func GetAllMCMS(
	chain cldf_solana.Chain,
	qualifier string,
	existingAddresses []cldf_datastore.AddressRef) []cldf_datastore.AddressRef {
	accessControllerProgram := datastore.GetAddressRef(
		existingAddresses,
		chain.ChainSelector(),
		AccessControllerProgramType,
		common_utils.Version_1_6_0,
		"",
	)
	proposerAccount := datastore.GetAddressRef(
		existingAddresses,
		chain.ChainSelector(),
		ProposerAccessControllerAccount,
		common_utils.Version_1_6_0,
		qualifier,
	)
	executorAccount := datastore.GetAddressRef(
		existingAddresses,
		chain.ChainSelector(),
		ExecutorAccessControllerAccount,
		common_utils.Version_1_6_0,
		qualifier,
	)
	cancellerAccount := datastore.GetAddressRef(
		existingAddresses,
		chain.ChainSelector(),
		CancellerAccessControllerAccount,
		common_utils.Version_1_6_0,
		qualifier,
	)
	bypasserAccount := datastore.GetAddressRef(
		existingAddresses,
		chain.ChainSelector(),
		BypasserAccessControllerAccount,
		common_utils.Version_1_6_0,
		qualifier,
	)
	timelockProgram := datastore.GetAddressRef(
		existingAddresses,
		chain.ChainSelector(),
		common_utils.RBACTimelock,
		common_utils.Version_1_6_0,
		qualifier,
	)
	proposerMCMSAccount := datastore.GetAddressRef(
		existingAddresses,
		chain.ChainSelector(),
		common_utils.ProposerManyChainMultisig,
		common_utils.Version_1_6_0,
		qualifier,
	)
	cancellerMCMSAccount := datastore.GetAddressRef(
		existingAddresses,
		chain.ChainSelector(),
		common_utils.CancellerManyChainMultisig,
		common_utils.Version_1_6_0,
		qualifier,
	)
	bypasserMCMSAccount := datastore.GetAddressRef(
		existingAddresses,
		chain.ChainSelector(),
		common_utils.BypasserManyChainMultisig,
		common_utils.Version_1_6_0,
		qualifier,
	)
	return []cldf_datastore.AddressRef{
		accessControllerProgram,
		proposerAccount,
		executorAccount,
		cancellerAccount,
		bypasserAccount,
		timelockProgram,
		proposerMCMSAccount,
		cancellerMCMSAccount,
		bypasserMCMSAccount,
	}
}
