package sequences

import (
	"fmt"

	"github.com/Masterminds/semver/v3"
	"github.com/gagliardetto/solana-go"

	deployops "github.com/smartcontractkit/chainlink-ccip/deployment/deploy"
	common_utils "github.com/smartcontractkit/chainlink-ccip/deployment/utils"
	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/datastore"
	mcms_utils "github.com/smartcontractkit/chainlink-ccip/deployment/utils/mcms"
	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/sequences"
	cldf_chain "github.com/smartcontractkit/chainlink-deployments-framework/chain"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	cldf_datastore "github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	"github.com/smartcontractkit/chainlink-deployments-framework/deployment"
	cldf_ops "github.com/smartcontractkit/chainlink-deployments-framework/operations"
	mcms_solana "github.com/smartcontractkit/mcms/sdk/solana"
	mcms_types "github.com/smartcontractkit/mcms/types"

	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	cctpverifierops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/cctpverifier"
	cvops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/committeeverifier"
	executorops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/executor"
	feequoterops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/feequoter"
	lvops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/lombardverifier"
	offrampops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/offramp"
	rmnremoteops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/rmnremote"
	routerops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/router"
	tokenpoolops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/tokenpools"
)

// GetChainMetadata resolves the MCM account for the requested timelock action and the
// access-controller accounts of its MCMS instance. Refs are read at 1.6.0: there is no
// v2 MCMS deployment, v2 reuses the shared 1.6 MCMS, so its refs are the only ones that
// exist. An empty qualifier defaults to the CLLCCIP instance; a missing or malformed ref
// is an error naming it, never a panic on an empty address.
func (a *SolanaAdapter) GetChainMetadata(e deployment.Environment, chainSelector uint64, input mcms_utils.Input) (mcms_types.ChainMetadata, error) {
	chain, ok := e.BlockChains.SolanaChains()[chainSelector]
	if !ok {
		return mcms_types.ChainMetadata{}, fmt.Errorf("chain with selector %d not found in environment", chainSelector)
	}

	refs := e.DataStore.Addresses().Filter()
	qualifier := solcommon.MCMSQualifierOrDefault(input.Qualifier)

	var mcmType deployment.ContractType
	switch input.TimelockAction {
	case mcms_types.TimelockActionSchedule:
		mcmType = common_utils.ProposerManyChainMultisig
	case mcms_types.TimelockActionCancel:
		mcmType = common_utils.CancellerManyChainMultisig
	case mcms_types.TimelockActionBypass:
		mcmType = common_utils.BypasserManyChainMultisig
	default:
		return mcms_types.ChainMetadata{}, fmt.Errorf("unsupported timelock action %s for chain %d", input.TimelockAction, chainSelector)
	}
	mcmRef, err := solcommon.FindMCMSRef(refs, chainSelector, mcmType, qualifier)
	if err != nil {
		return mcms_types.ChainMetadata{}, err
	}
	id, seed, err := mcms_solana.ParseContractAddress(mcmRef.Address)
	if err != nil {
		return mcms_types.ChainMetadata{}, fmt.Errorf("failed to parse %s address %s for chain %d: %w", mcmType, mcmRef.Address, chainSelector, err)
	}

	accessControllers := make([]solana.PublicKey, 0, 3)
	for _, accountType := range []deployment.ContractType{
		solcommon.ProposerAccessControllerAccount,
		solcommon.CancellerAccessControllerAccount,
		solcommon.BypasserAccessControllerAccount,
	} {
		ref, refErr := solcommon.FindMCMSRef(refs, chainSelector, accountType, qualifier)
		if refErr != nil {
			return mcms_types.ChainMetadata{}, refErr
		}
		account, parseErr := solana.PublicKeyFromBase58(ref.Address)
		if parseErr != nil {
			return mcms_types.ChainMetadata{}, fmt.Errorf("failed to parse %s address %s for chain %d: %w", accountType, ref.Address, chainSelector, parseErr)
		}
		accessControllers = append(accessControllers, account)
	}

	inspector := mcms_solana.NewInspector(chain.Client)
	opcount, err := inspector.GetOpCount(e.GetContext(), mcms_solana.ContractAddress(id, seed))
	if err != nil {
		return mcms_types.ChainMetadata{}, fmt.Errorf("failed to get op count for chain %d: %w", chainSelector, err)
	}
	metadata, err := mcms_solana.NewChainMetadata(
		opcount,
		id,
		seed,
		accessControllers[0],
		accessControllers[1],
		accessControllers[2])
	if err != nil {
		return mcms_types.ChainMetadata{}, fmt.Errorf("failed to create Solana MCMS chain metadata for chain %d: %w", chainSelector, err)
	}
	return metadata, nil
}

// GetTimelockRef resolves the timelock ref at 1.6.0 (there is no v2 MCMS deployment).
// It never errors on a missing ref: callers probe with it to decide whether a timelock
// exists at all (token deploy falls back to a deployer admin on an empty ref), so an
// environment without MCMS gets an empty ref and no error.
func (a *SolanaAdapter) GetTimelockRef(e deployment.Environment, chainSelector uint64, input mcms_utils.Input) (cldf_datastore.AddressRef, error) {
	return datastore.GetAddressRef(
		e.DataStore.Addresses().Filter(),
		chainSelector,
		common_utils.RBACTimelock,
		common_utils.Version_1_6_0,
		solcommon.MCMSQualifierOrDefault(input.Qualifier),
	), nil
}

// GetMCMSRef resolves the MCM program ref at 1.6.0. The program is shared by every MCMS
// instance on the chain and stored without a qualifier, so it is read with "" regardless
// of input.Qualifier. Like GetTimelockRef it returns an empty ref when missing.
func (a *SolanaAdapter) GetMCMSRef(e deployment.Environment, chainSelector uint64, input mcms_utils.Input) (cldf_datastore.AddressRef, error) {
	return datastore.GetAddressRef(
		e.DataStore.Addresses().Filter(),
		chainSelector,
		solcommon.McmProgramType,
		common_utils.Version_1_6_0,
		"",
	), nil
}

func (a *SolanaAdapter) InitializeTimelockAddress(e deployment.Environment, input mcms_utils.Input) error {
	return nil
}

// ownershipOp runs one step of a two-step ownership handover for a program.
type ownershipOp func(b cldf_ops.Bundle, chain cldf_solana.Chain, params solcommon.TransferOwnershipParams) (sequences.OnChainOutput, error)

// ownershipOps pairs the transfer and accept steps of a contract the deploy flow can hand
// to the timelock.
type ownershipOps struct {
	transfer ownershipOp
	accept   ownershipOp
}

// wrapOwnershipOp adapts an operation taking solcommon.TransferOwnershipParams to an
// ownershipOp.
func wrapOwnershipOp(
	op *cldf_ops.Operation[solcommon.TransferOwnershipParams, sequences.OnChainOutput, cldf_solana.Chain],
) ownershipOp {
	return func(b cldf_ops.Bundle, chain cldf_solana.Chain, params solcommon.TransferOwnershipParams) (sequences.OnChainOutput, error) {
		report, err := cldf_ops.ExecuteOperation(b, op, chain, params)
		return report.Output, err
	}
}

// ownablePrograms indexes the ownership operations of every contract type v2 deploys.
// Anything absent from this map is rejected with a clear error (see
// unsupportedOwnershipTypeError), so the changeset never reports success while a program
// silently stays with the deployer key. Two deliberate exclusions:
//
//   - Token pools: pool ownership moves through the dedicated UpdateAuthorities flow
//     (SolanaAdapter.UpdateAuthorities, CCTPChainAdapter.UpdateAuthorities), which also
//     handles the rate-limit admins.
//   - MCMS programs (access controller, timelock, MCM accounts): they are shared 1.6
//     infrastructure that v2 reuses, and their ownership is managed by the 1.6 transfer
//     changesets (v1_6_0 transferAllMCMS/acceptAllMCMS), not by v2.
var ownablePrograms = map[string]ownershipOps{
	routerops.ContractType.String():    {transfer: wrapOwnershipOp(routerops.TransferOwnership), accept: wrapOwnershipOp(routerops.AcceptOwnership)},
	offrampops.ContractType.String():   {transfer: wrapOwnershipOp(offrampops.TransferOwnership), accept: wrapOwnershipOp(offrampops.AcceptOwnership)},
	feequoterops.ContractType.String(): {transfer: wrapOwnershipOp(feequoterops.TransferOwnership), accept: wrapOwnershipOp(feequoterops.AcceptOwnership)},
	rmnremoteops.ContractType.String(): {transfer: wrapOwnershipOp(rmnremoteops.TransferOwnership), accept: wrapOwnershipOp(rmnremoteops.AcceptOwnership)},
	executorops.ContractType.String():  {transfer: wrapOwnershipOp(executorops.TransferOwnership), accept: wrapOwnershipOp(executorops.AcceptOwnership)},
	cvops.ContractType.String():        {transfer: wrapOwnershipOp(cvops.TransferOwnership), accept: wrapOwnershipOp(cvops.AcceptOwnership)},
	lvops.ContractType.String():        {transfer: wrapOwnershipOp(lvops.TransferOwnership), accept: wrapOwnershipOp(lvops.AcceptOwnership)},
	cctpverifierops.ContractType.String(): {
		transfer: func(b cldf_ops.Bundle, chain cldf_solana.Chain, params solcommon.TransferOwnershipParams) (sequences.OnChainOutput, error) {
			report, err := cldf_ops.ExecuteOperation(b, cctpverifierops.TransferOwnership, chain, cctpverifierops.TransferOwnershipParams{
				CctpVerifier:  params.Program,
				ProposedOwner: params.NewOwner,
			})
			return report.Output, err
		},
		accept: func(b cldf_ops.Bundle, chain cldf_solana.Chain, params solcommon.TransferOwnershipParams) (sequences.OnChainOutput, error) {
			report, err := cldf_ops.ExecuteOperation(b, cctpverifierops.AcceptOwnership, chain, cctpverifierops.AcceptOwnershipParams{
				CctpVerifier:  params.Program,
				ProposedOwner: params.NewOwner,
			})
			return report.Output, err
		},
	},
}

// unsupportedOwnershipTypeError rejects a ref the v2 flow does not transfer, pointing each
// known case at the flow that owns it.
func unsupportedOwnershipTypeError(contractRef cldf_datastore.AddressRef, chainSelector uint64) error {
	switch contractRef.Type.String() {
	case solcommon.AccessControllerProgramType.String(), common_utils.RBACTimelock.String(),
		common_utils.ProposerManyChainMultisig.String(), common_utils.CancellerManyChainMultisig.String(), common_utils.BypasserManyChainMultisig.String():
		return fmt.Errorf("cannot transfer %s on chain %d: MCMS programs are shared 1.6 infrastructure; transfer their ownership with the 1.6 transfer changesets", contractRef.Type, chainSelector)
	case common_utils.BurnMintTokenPool.String(), common_utils.LockReleaseTokenPool.String(),
		common_utils.CCTPTokenPool.String(), tokenpoolops.LombardTokenPool.String():
		return fmt.Errorf("cannot transfer %s on chain %d: token pool ownership moves through the UpdateAuthorities flow", contractRef.Type, chainSelector)
	default:
		return fmt.Errorf("unsupported contract type %s for ownership transfer on Solana chain %d: expected one of Router, OffRamp, FeeQuoter, RMNRemote, Executor, CommitteeVerifier, CCTPVerifier, LombardVerifier", contractRef.Type, chainSelector)
	}
}

// resolveTransferOwners normalizes the framework-provided owner strings into keys.
// deploy.TransferToTimelock passes ProposedOwner as the timelock's datastore address
// ("program.seed") and leaves CurrentOwner empty; the standalone changesets pass plain
// base58 keys. An empty CurrentOwner defaults to the deployer, which owns freshly
// deployed contracts.
func resolveTransferOwners(chain cldf_solana.Chain, in deployops.TransferOwnershipPerChainInput) (currentOwner, newOwner solana.PublicKey, err error) {
	newOwner, err = solcommon.ParseOwnerAddress(in.ProposedOwner)
	if err != nil {
		return solana.PublicKey{}, solana.PublicKey{}, fmt.Errorf("invalid proposed owner for chain %d: %w", in.ChainSelector, err)
	}
	currentOwner = chain.DeployerKey.PublicKey()
	if in.CurrentOwner != "" {
		currentOwner, err = solcommon.ParseOwnerAddress(in.CurrentOwner)
		if err != nil {
			return solana.PublicKey{}, solana.PublicKey{}, fmt.Errorf("invalid current owner for chain %d: %w", in.ChainSelector, err)
		}
	}
	return currentOwner, newOwner, nil
}

// runOwnershipOps executes pick (transfer or accept) for every ref in in.ContractRef.
// Refs the v2 flow does not transfer are rejected with a clear error rather than skipped,
// so the changeset cannot report success while a program stays with the deployer key.
func runOwnershipOps(
	b cldf_ops.Bundle,
	chains cldf_chain.BlockChains,
	in deployops.TransferOwnershipPerChainInput,
	pick func(ownershipOps) ownershipOp,
) (output sequences.OnChainOutput, err error) {
	chain, ok := chains.SolanaChains()[in.ChainSelector]
	if !ok {
		return sequences.OnChainOutput{}, fmt.Errorf("chain with selector %d not found in environment", in.ChainSelector)
	}
	currentOwner, newOwner, err := resolveTransferOwners(chain, in)
	if err != nil {
		return sequences.OnChainOutput{}, err
	}

	for _, contractRef := range in.ContractRef {
		ops, ok := ownablePrograms[contractRef.Type.String()]
		if !ok {
			return sequences.OnChainOutput{}, unsupportedOwnershipTypeError(contractRef, in.ChainSelector)
		}
		program, err := solana.PublicKeyFromBase58(contractRef.Address)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to parse %s address %s for chain %d: %w", contractRef.Type, contractRef.Address, in.ChainSelector, err)
		}
		out, err := pick(ops)(b, chain, solcommon.TransferOwnershipParams{
			Program:      program,
			CurrentOwner: currentOwner,
			NewOwner:     newOwner,
		})
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to run ownership step for %s on chain %d: %w", contractRef.Type, in.ChainSelector, err)
		}
		output.BatchOps = append(output.BatchOps, out.BatchOps...)
	}
	return output, nil
}

func (a *SolanaAdapter) SequenceTransferOwnershipViaMCMS() *cldf_ops.Sequence[deployops.TransferOwnershipPerChainInput, sequences.OnChainOutput, cldf_chain.BlockChains] {
	return cldf_ops.NewSequence(
		"seq-transfer-ownership-via-mcms",
		semver.MustParse("1.0.0"),
		"Transfers ownership of contracts via MCMS",
		func(b cldf_ops.Bundle, chains cldf_chain.BlockChains, in deployops.TransferOwnershipPerChainInput) (output sequences.OnChainOutput, err error) {
			return runOwnershipOps(b, chains, in, func(ops ownershipOps) ownershipOp {
				return ops.transfer
			})
		})
}

func (a *SolanaAdapter) ShouldAcceptOwnershipWithTransferOwnership(e deployment.Environment, in deployops.TransferOwnershipPerChainInput) (bool, error) {
	chain, ok := e.BlockChains.SolanaChains()[in.ChainSelector]
	if !ok {
		return false, fmt.Errorf("chain with selector %d not found in environment", in.ChainSelector)
	}
	if in.CurrentOwner == "" {
		// deploy.TransferToTimelock leaves CurrentOwner unset; contracts handed over at
		// deploy time are deployer-owned, so the accept step must run.
		return true, nil
	}
	currentOwner, err := solana.PublicKeyFromBase58(in.CurrentOwner)
	if err != nil {
		return false, fmt.Errorf("failed to parse current owner %q for chain %d: %w", in.CurrentOwner, in.ChainSelector, err)
	}
	return currentOwner == chain.DeployerKey.PublicKey(), nil
}

func (a *SolanaAdapter) SequenceAcceptOwnership() *cldf_ops.Sequence[deployops.TransferOwnershipPerChainInput, sequences.OnChainOutput, cldf_chain.BlockChains] {
	return cldf_ops.NewSequence(
		"seq-accept-ownership",
		semver.MustParse("1.0.0"),
		"Accepts ownership of contracts",
		func(b cldf_ops.Bundle, chains cldf_chain.BlockChains, in deployops.TransferOwnershipPerChainInput) (output sequences.OnChainOutput, err error) {
			return runOwnershipOps(b, chains, in, func(ops ownershipOps) ownershipOp {
				return ops.accept
			})
		})
}
