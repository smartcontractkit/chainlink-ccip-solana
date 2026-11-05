package state

import (
	"github.com/gagliardetto/solana-go"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
)

//////////////////////
// CCIP Router PDAs //
//////////////////////

func FindConfigPDA(ccipRouterProgram solana.PublicKey) (solana.PublicKey, uint8, error) {
	return solana.FindProgramAddress([][]byte{[]byte("config")}, ccipRouterProgram)
}

func FindFeeBillingSignerPDA(ccipRouterProgram solana.PublicKey) (solana.PublicKey, uint8, error) {
	return solana.FindProgramAddress([][]byte{[]byte("fee_billing_signer")}, ccipRouterProgram)
}

func FindCCVSignerPDA(ccvProgram solana.PublicKey, ccipRouterProgram solana.PublicKey) (solana.PublicKey, uint8, error) {
	return solana.FindProgramAddress([][]byte{[]byte("ccv_signer"), ccvProgram.Bytes()}, ccipRouterProgram)
}

func FindTokenAdminRegistryPDA(mint solana.PublicKey, ccipRouterProgram solana.PublicKey) (solana.PublicKey, uint8, error) {
	return solana.FindProgramAddress([][]byte{[]byte("token_admin_registry"), mint.Bytes()}, ccipRouterProgram)
}

func FindDestChainStatePDA(chainSelector uint64, ccipRouterProgram solana.PublicKey) (solana.PublicKey, error) {
	chainSelectorLE := common.Uint64ToLE(chainSelector)
	p, _, err := solana.FindProgramAddress([][]byte{[]byte("dest_chain_state"), chainSelectorLE}, ccipRouterProgram)
	return p, err
}

// FindDestChainStatePDAV2 derives the v2.0 dest chain state PDA. It uses a distinct seed from the
// v1 helper so that a v1 and a v2 lane can coexist on the same chain selector (parallel lanes).
func FindDestChainStatePDAV2(chainSelector uint64, ccipRouterProgram solana.PublicKey) (solana.PublicKey, error) {
	chainSelectorLE := common.Uint64ToLE(chainSelector)
	p, _, err := solana.FindProgramAddress([][]byte{[]byte("dest_chain_state_v2"), chainSelectorLE}, ccipRouterProgram)
	return p, err
}

func FindNoncePDA(chainSelector uint64, user solana.PublicKey, ccipRouterProgram solana.PublicKey) (solana.PublicKey, error) {
	chainSelectorLE := common.Uint64ToLE(chainSelector)
	p, _, err := solana.FindProgramAddress([][]byte{[]byte("nonce"), chainSelectorLE, user.Bytes()}, ccipRouterProgram)
	return p, err
}

func FindStateReceiverPDA(receiverProgram solana.PublicKey) (solana.PublicKey, uint8, error) {
	return solana.FindProgramAddress([][]byte{[]byte("state")}, receiverProgram)
}

func FindReceiverTokenAdminPDA(receiverProgram solana.PublicKey) (solana.PublicKey, uint8, error) {
	return solana.FindProgramAddress([][]byte{[]byte("receiver_token_admin")}, receiverProgram)
}

func FindApprovedSender(chainSelector uint64, sourceSender []byte, receiverProgram solana.PublicKey) (solana.PublicKey, error) {
	chainSelectorLE := common.Uint64ToLE(chainSelector)
	p, _, err := solana.FindProgramAddress([][]byte{[]byte("approved_ccip_sender"), chainSelectorLE, {uint8(len(sourceSender))}, sourceSender}, receiverProgram) //nolint:gosec // G115 length fits in uint8
	return p, err
}

func FindApprovedSenderV2(chainSelector uint64, sourceSender []byte, receiverProgram solana.PublicKey) (solana.PublicKey, error) {
	chainSelectorLE := common.Uint64ToLE(chainSelector)
	p, _, err := solana.FindProgramAddress([][]byte{[]byte("approved_ccip_sender"), chainSelectorLE, sourceSender}, receiverProgram)
	return p, err
}

func FindConfigReceiverPDA(receiverProgram solana.PublicKey) (solana.PublicKey, uint8, error) {
	return solana.FindProgramAddress([][]byte{[]byte("config")}, receiverProgram)
}

func FindAllowedOfframpPDA(chainSelector uint64, offramp solana.PublicKey, ccipRouterProgram solana.PublicKey) (solana.PublicKey, error) {
	chainSelectorLE := common.Uint64ToLE(chainSelector)
	p, _, err := solana.FindProgramAddress([][]byte{[]byte("allowed_offramp"), chainSelectorLE, offramp.Bytes()}, ccipRouterProgram)
	return p, err
}

// FindReceiverRegistryPDA derives the router-owned per-receiver interface-version registry PDA
// If Absent: the receiver uses the v1 flow; if present: its `receiver_version` decides.
func FindReceiverRegistryPDA(receiverProgram solana.PublicKey, ccipRouterProgram solana.PublicKey) (solana.PublicKey, uint8, error) {
	return solana.FindProgramAddress([][]byte{[]byte("receiver_registry"), receiverProgram.Bytes()}, ccipRouterProgram)
}

// FindRouterReferenceAddressesPDA derives the router-owned reference addresses PDA, which holds
// the lookup table that `ccip_send_v2` / `get_fee_v2` account resolution returns.
func FindRouterReferenceAddressesPDA(ccipRouterProgram solana.PublicKey) (solana.PublicKey, uint8, error) {
	return solana.FindProgramAddress([][]byte{[]byte("reference_addresses")}, ccipRouterProgram)
}

// FindReceiverRegistrationSignerPDA derives the signer PDA (under a receiver program) that must
// sign the `register_receiver` CPI into the router. Only the receiver program can
// sign for it, which is how the router proves the receiver authorized its own registration.
func FindReceiverRegistrationSignerPDA(receiverProgram solana.PublicKey, ccipRouterProgram solana.PublicKey) (solana.PublicKey, uint8, error) {
	return solana.FindProgramAddress([][]byte{[]byte("ccip_receiver_registration"), ccipRouterProgram.Bytes()}, receiverProgram)
}

func FindReceiverCCVConfigPDA(chainSelector uint64, receiverProgram solana.PublicKey) (solana.PublicKey, uint8, error) {
	chainSelectorLE := common.Uint64ToLE(chainSelector)
	return solana.FindProgramAddress([][]byte{[]byte("ccv_config"), chainSelectorLE}, receiverProgram)
}

//////////////////////////////////////////
// PDAs with same seeds across programs //
//////////////////////////////////////////

func FindExternalTokenPoolsSignerPDA(poolProgram solana.PublicKey, program solana.PublicKey) (solana.PublicKey, uint8, error) {
	return solana.FindProgramAddress([][]byte{[]byte("external_token_pools_signer"), poolProgram.Bytes()}, program)
}

func FindExternalExecutionConfigPDA(logicReceiver solana.PublicKey, program solana.PublicKey) (solana.PublicKey, uint8, error) {
	return solana.FindProgramAddress([][]byte{[]byte("external_execution_config"), logicReceiver.Bytes()}, program)
}

func FindEventAuthorityPDA(program solana.PublicKey) (solana.PublicKey, uint8, error) {
	return solana.FindProgramAddress([][]byte{[]byte("__event_authority")}, program)
}

/////////////////////
// Fee Quoter PDAs //
/////////////////////

func FindFqConfigPDA(feeQuoterProgram solana.PublicKey) (solana.PublicKey, uint8, error) {
	return solana.FindProgramAddress([][]byte{[]byte("config")}, feeQuoterProgram)
}

func FindFqDestChainPDA(chainSelector uint64, feeQuoterProgram solana.PublicKey) (solana.PublicKey, uint8, error) {
	chainSelectorLE := common.Uint64ToLE(chainSelector)
	return solana.FindProgramAddress([][]byte{[]byte("dest_chain"), chainSelectorLE}, feeQuoterProgram)
}

func FindFqDestChainV2PDA(chainSelector uint64, feeQuoterProgram solana.PublicKey) (solana.PublicKey, uint8, error) {
	chainSelectorLE := common.Uint64ToLE(chainSelector)
	return solana.FindProgramAddress([][]byte{[]byte("dest_chain_v2"), chainSelectorLE}, feeQuoterProgram)
}

func FindFqBillingTokenConfigPDA(mint solana.PublicKey, feeQuoterProgram solana.PublicKey) (solana.PublicKey, uint8, error) {
	return solana.FindProgramAddress([][]byte{[]byte("fee_billing_token_config"), mint.Bytes()}, feeQuoterProgram)
}

func FindFqPerChainPerTokenConfigPDA(chainSelector uint64, mint solana.PublicKey, feeQuoterProgram solana.PublicKey) (solana.PublicKey, uint8, error) {
	chainSelectorLE := common.Uint64ToLE(chainSelector)
	return solana.FindProgramAddress([][]byte{[]byte("per_chain_per_token_config"), chainSelectorLE, mint.Bytes()}, feeQuoterProgram)
}

func FindFqAllowedPriceUpdaterPDA(priceUpdater solana.PublicKey, feeQuoterProgram solana.PublicKey) (solana.PublicKey, uint8, error) {
	return solana.FindProgramAddress([][]byte{[]byte("allowed_price_updater"), priceUpdater.Bytes()}, feeQuoterProgram)
}

//////////////////
// Offramp PDAs //
//////////////////

func FindOfframpConfigPDA(offrampProgram solana.PublicKey) (solana.PublicKey, uint8, error) {
	return solana.FindProgramAddress([][]byte{[]byte("config")}, offrampProgram)
}

func FindOfframpReferenceAddressesPDA(offrampProgram solana.PublicKey) (solana.PublicKey, uint8, error) {
	return solana.FindProgramAddress([][]byte{[]byte("reference_addresses")}, offrampProgram)
}

func FindOfframpSourceChainPDA(chainSelector uint64, offrampProgram solana.PublicKey) (solana.PublicKey, uint8, error) {
	chainSelectorLE := common.Uint64ToLE(chainSelector)
	return solana.FindProgramAddress([][]byte{[]byte("source_chain_state"), chainSelectorLE}, offrampProgram)
}

func FindOfframpBillingSignerPDA(offrampProgram solana.PublicKey) (solana.PublicKey, uint8, error) {
	return solana.FindProgramAddress([][]byte{[]byte("fee_billing_signer")}, offrampProgram)
}

func FindOfframpCommitReportPDA(chainSelector uint64, root [32]byte, offrampProgram solana.PublicKey) (solana.PublicKey, error) {
	chainSelectorLE := common.Uint64ToLE(chainSelector)
	p, _, err := solana.FindProgramAddress([][]byte{[]byte("commit_report"), chainSelectorLE, root[:]}, offrampProgram)
	return p, err
}

func FindOfframpMessageStatePDA(messageID [32]byte, offrampProgram solana.PublicKey) (solana.PublicKey, uint8, error) {
	return solana.FindProgramAddress([][]byte{[]byte("message_exec_state"), messageID[:]}, offrampProgram)
}

func FindExecutionInputsBufferPDA(bufferID [32]byte, authority solana.PublicKey, offrampProgram solana.PublicKey) (solana.PublicKey, uint8, error) {
	return solana.FindProgramAddress([][]byte{[]byte("execution_inputs_buffer"), bufferID[:], authority.Bytes()}, offrampProgram)
}

//////////////////////////////
// Committee Verifier PDAs //
//////////////////////////////

func FindCVConfigPDA(cvProgram solana.PublicKey) (solana.PublicKey, uint8, error) {
	return solana.FindProgramAddress([][]byte{[]byte("config")}, cvProgram)
}

func FindCVRemoteChainConfigPDA(chainSelector uint64, cvProgram solana.PublicKey) (solana.PublicKey, uint8, error) {
	chainSelectorLE := common.Uint64ToLE(chainSelector)
	return solana.FindProgramAddress([][]byte{[]byte("remote_chain"), chainSelectorLE}, cvProgram)
}

func FindCVStorageLocationsPDA(cvProgram solana.PublicKey) (solana.PublicKey, uint8, error) {
	return solana.FindProgramAddress([][]byte{[]byte("storage_locations")}, cvProgram)
}

func FindCVChainsWithConfiguredSignersListPDA(cvProgram solana.PublicKey) (solana.PublicKey, uint8, error) {
	return solana.FindProgramAddress([][]byte{[]byte("chains_with_signers")}, cvProgram)
}

////////////////////////
// Lombard Verifier PDAs //
////////////////////////

// Lombard's verifier lives in the same program as the Lombard token pool
// (lombard_token_pool). Its config PDA deliberately reuses committee_verifier's generic
// "config" seed (ccip_common::seed::CONFIG) rather than a Lombard-specific one: the
// offramp's generic CCV account resolution (ccip-offramp's ccv.rs) hardcodes "config" as
// the seed for any CCV's own config PDA under its own program ID, so this program's
// pool-side global config (PoolConfig) was moved to POOL_CONFIG_SEED instead - see
// constants.rs.

func FindLVConfigPDA(lombardProgram solana.PublicKey) (solana.PublicKey, uint8, error) {
	return solana.FindProgramAddress([][]byte{[]byte("config")}, lombardProgram)
}

// FindLombardPoolConfigPDA finds the pool-side global config (PoolConfig) PDA - the
// generic "config" slot on this program is reserved for FindLVConfigPDA (the verifier's
// own config), so the pool's version gate lives under POOL_CONFIG_SEED instead.
func FindLombardPoolConfigPDA(lombardProgram solana.PublicKey) (solana.PublicKey, uint8, error) {
	return solana.FindProgramAddress([][]byte{[]byte("lombard_pool_config")}, lombardProgram)
}

func FindLVRemoteChainConfigPDA(chainSelector uint64, lombardProgram solana.PublicKey) (solana.PublicKey, uint8, error) {
	chainSelectorLE := common.Uint64ToLE(chainSelector)
	return solana.FindProgramAddress([][]byte{[]byte("lombard_verifier_remote_chain"), chainSelectorLE}, lombardProgram)
}

func FindLVStorageLocationsPDA(lombardProgram solana.PublicKey) (solana.PublicKey, uint8, error) {
	return solana.FindProgramAddress([][]byte{[]byte("lombard_verifier_storage_locs")}, lombardProgram)
}

// //////////////////////
// CCTP Verifier PDAs //
// //////////////////////
func FindCctpVerifierRemoteChainConfigPDA(chainSelector uint64, cctpPoolProgram solana.PublicKey) (solana.PublicKey, uint8, error) {
	chainSelectorLE := common.Uint64ToLE(chainSelector)
	return solana.FindProgramAddress([][]byte{[]byte("verifier_remote_chain"), chainSelectorLE}, cctpPoolProgram)
}

func FindCctpVerifierNoncePDA(chainSelector uint64, user solana.PublicKey, mint solana.PublicKey, cctpPoolProgram solana.PublicKey) (solana.PublicKey, uint8, error) {
	chainSelectorLE := common.Uint64ToLE(chainSelector)
	return solana.FindProgramAddress([][]byte{[]byte("ccip_cctpv2_nonce"), user.Bytes(), chainSelectorLE, mint.Bytes()}, cctpPoolProgram)
}

func FindCctpVerifierEventPDA(chainSelector uint64, user solana.PublicKey, mint solana.PublicKey, nonce uint64, cctpPoolProgram solana.PublicKey) (solana.PublicKey, uint8, error) {
	chainSelectorLE := common.Uint64ToLE(chainSelector)
	nonceLE := common.Uint64ToLE(nonce)
	return solana.FindProgramAddress([][]byte{[]byte("ccip_cctpv2_event"), user.Bytes(), chainSelectorLE, mint.Bytes(), nonceLE}, cctpPoolProgram)
}

////////////////////
// Executor PDAs //
////////////////////

func FindExecChainConfigPDA(chainSelector uint64, executorProgram solana.PublicKey) (solana.PublicKey, uint8, error) {
	chainSelectorLE := common.Uint64ToLE(chainSelector)
	return solana.FindProgramAddress([][]byte{[]byte("remote_chain"), chainSelectorLE}, executorProgram)
}

/////////////////////
// RMN Remote PDAs //
/////////////////////

func FindRMNRemoteConfigPDA(rmnRemoteProgram solana.PublicKey) (solana.PublicKey, uint8, error) {
	return solana.FindProgramAddress([][]byte{[]byte("config")}, rmnRemoteProgram)
}

func FindRMNRemoteCursesPDA(rmnRemoteProgram solana.PublicKey) (solana.PublicKey, uint8, error) {
	return solana.FindProgramAddress([][]byte{[]byte("curses")}, rmnRemoteProgram)
}

/////////////////
// Shared PDAs //
/////////////////

func FindNameAndVersionPDA(program solana.PublicKey) (solana.PublicKey, uint8, error) {
	return solana.FindProgramAddress([][]byte{[]byte("name_version")}, program)
}

// FindFeeVaultPDA derives the fee vault PDA of a receipt issuer program, matching
// ccip_common::seed::vault_ata's ["fee_vault"] seeds. The account that actually
// holds fees for a given mint is this PDA's associated token account for that mint.
// The optional qualifier (nil for none) is an extra seed that lets a single issuer
// program keep separate, individually billable fee vaults per resource it manages:
// token pools qualify the vault with the mint of the token they manage.
func FindFeeVaultPDA(program solana.PublicKey, qualifier []byte) (solana.PublicKey, uint8, error) {
	seeds := [][]byte{[]byte("fee_vault")}
	if qualifier != nil {
		seeds = append(seeds, qualifier)
	}
	return solana.FindProgramAddress(seeds, program)
}
