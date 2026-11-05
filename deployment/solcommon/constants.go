package solcommon

import (
	"github.com/Masterminds/semver/v3"

	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	cldf_deployment "github.com/smartcontractkit/chainlink-deployments-framework/deployment"
)

const (
	// tokens
	SPLTokens     cldf_deployment.ContractType = "SPLTokens"
	SPL2022Tokens cldf_deployment.ContractType = "SPL2022Tokens"
	// CcipSendLookupTableType is the datastore type for the router ccip_send_v2 LUT.
	CcipSendLookupTableType datastore.ContractType = "CcipSendLookupTable"
	// PoolProgramQualifier is the qualifier used when registering pool *programs*
	// in the datastore. Pool *instances* (per-token accounts) use a token-combo
	// qualifier. We need a non-empty qualifier because FindAndFormatRef treats
	// empty qualifier as a wildcard.
	PoolProgramQualifier = "TokenPoolProgram"
	// USDCTokenQualifier is the datastore qualifier under which the chain's USDC
	// mint is registered.
	USDCTokenQualifier = "USDC"
	// Lombard bridge/mailbox contract types. On staging these are Lombard's real
	// programs; devenv registers its mock bridge/mailbox under the same types so
	// the v2_0_0 configure sequence resolves them in both environments.
	LombardBridgeV2ContractType = "LombardBridgeV2"
	LombardMailboxContractType  = "LombardMailbox"
)

var (
	Version2_0_0 = semver.MustParse("2.0.0")
	// LegacyTokenPoolVersion is retained for datastore compatibility while token consumers
	// migrate from the former pool identity to the Solana 2.0.0 programs.
	LegacyTokenPoolVersion = semver.MustParse("1.6.4")

	// SVMChainFamilySelector is bytes4(keccak256("CCIP ChainFamilySelector SVM")) = 0x1e10bdc4.
	SVMChainFamilySelector = [4]byte{0x1e, 0x10, 0xbd, 0xc4}

	// LombardChainContractsVersion is the datastore version for the Lombard
	// bridge/mailbox refs.
	LombardChainContractsVersion = semver.MustParse("1.0.0")
)
