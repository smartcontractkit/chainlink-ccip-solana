package config

import (
	"encoding/hex"
	"fmt"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/arbitrary_cpi_proxy"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/burnmint_token_pool"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/ccip_offramp"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/ccip_router"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/cctp_token_pool"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/committee_verifier"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/example_ccip_receiver_v2"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/example_ccip_sender_v2"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/executor"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/fee_quoter"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/rmn_remote"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/test_ccip_invalid_receiver"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/test_ccip_receiver"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
)

var (
	PrintEvents       = true
	DefaultCommitment = rpc.CommitmentConfirmed

	// program ids
	CcipRouterProgram            = GetProgramID("ccip_router")
	CcipLogicReceiver            = GetProgramID("test_ccip_receiver")
	CcipBaseReceiverV2           = GetProgramID("example_ccip_receiver_v2")
	CcipBaseSenderV2             = GetProgramID("example_ccip_sender_v2")
	CcipTokenReceiver            = solana.MustPublicKeyFromBase58("DS2tt4BX7YwCw7yrDNwbAdnYrxjeCPeGJbHmZEYC8RTb")
	CcipInvalidReceiverProgram   = GetProgramID("test_ccip_invalid_receiver")
	ArbitraryCpiProxyProgram     = GetProgramID("arbitrary_cpi_proxy")
	CcipBurnMintTokenPoolProgram = GetProgramID("burnmint_token_pool")
	Token2022Program             = solana.Token2022ProgramID
	SPLTokenProgram              = solana.TokenProgramID
	FeeQuoterProgram             = GetProgramID("fee_quoter")
	CcipOfframpProgram           = GetProgramID("ccip_offramp")
	RMNRemoteProgram             = GetProgramID("rmn_remote")
	CommitteeVerifierProgram     = GetProgramID("committee_verifier")
	ExecutorProgram              = GetProgramID("executor")
	PingPongProgram              = GetProgramID("ping_pong_demo")
	CctpMessageTransmitter       = GetProgramID("cctp_message_transmitter")
	CctpMessageTransmitterV2     = GetProgramID("cctp_message_transmitter_v2")
	CctpTokenMessengerMinter     = GetProgramID("cctp_token_messenger_minter")
	CctpTokenMessengerMinterV2   = GetProgramID("cctp_token_messenger_minter_v2")
	CctpTokenPoolProgram         = GetProgramID("cctp_token_pool")

	// test values
	OnRampAddress                   = []byte{1, 2, 3}
	MaxOracles                      = 16
	OcrF                      uint8 = 5
	ConfigDigest                    = common.MakeRandom32ByteArray()
	Empty24Byte                     = [24]byte{}
	MaxSignersAndTransmitters       = 16

	// valid evm addresses(over precompiled space) - for remote token addresses
	EVMToken0AddressBytes          = []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 4, 0} // 1024
	EVMToken1AddressBytes          = []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}
	EVMToken2AddressBytes          = []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 20, 19, 18, 17, 16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}
	EVMToken3AddressBytes          = []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 21, 19, 18, 17, 16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}
	EVMUsdcAddressBytes            = []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 22, 19, 18, 17, 16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}
	EVMUsdcAddressBytesTrim        = EVMUsdcAddressBytes[12:32]
	EVMUsdcPoolAddressBytes        = []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 23, 19, 18, 17, 16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}
	EVMCctpMessageTransmitterProxy = []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 24, 19, 18, 17, 16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}
	EVMCctpTokenMessenger          = []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 25, 19, 18, 17, 16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}

	// chain selectors
	SvmChainSelector uint64 = 15
	EvmChainSelector uint64 = 21
	EvmChainLE              = common.Uint64ToLE(EvmChainSelector)
	// bytes4(keccak256("CCIP EVMExtraArgsV2"));
	EvmChainFamilySelector, _ = hex.DecodeString("2812d52c")
	// bytes4(keccak256("CCIP SVMExtraArgsV1"));
	SvmChainFamilySelector, _ = hex.DecodeString("1e10bdc4")

	// example programs
	CcipBaseSender          = GetProgramID("example_ccip_sender")
	CcipBaseReceiver        = GetProgramID("example_ccip_receiver")
	CcipBasePoolBurnMint    = GetProgramID("burnmint_token_pool")
	CcipBasePoolLockRelease = GetProgramID("lockrelease_token_pool")

	// router/onramp PDAs
	RouterConfigPDA, _, _                    = state.FindConfigPDA(CcipRouterProgram)
	ReceiverTargetAccountPDA, _, _           = solana.FindProgramAddress([][]byte{[]byte("counter")}, CcipLogicReceiver)
	ReceiverExternalExecutionConfigPDA, _, _ = solana.FindProgramAddress([][]byte{[]byte("external_execution_config")}, CcipLogicReceiver)
	BillingSignerPDA, _, _                   = state.FindFeeBillingSignerPDA(CcipRouterProgram)
	CCVSignerPDA, _, _                       = state.FindCCVSignerPDA(CommitteeVerifierProgram, CcipRouterProgram)
	SvmDestChainStatePDA, _                  = state.FindDestChainStatePDA(SvmChainSelector, CcipRouterProgram)
	EvmDestChainStatePDA, _                  = state.FindDestChainStatePDA(EvmChainSelector, CcipRouterProgram)
	EvmDestChainStatePDAV2, _                = state.FindDestChainStatePDAV2(EvmChainSelector, CcipRouterProgram)
	AllowedOfframpEvmPDA, _                  = state.FindAllowedOfframpPDA(EvmChainSelector, CcipOfframpProgram, CcipRouterProgram)
	AllowedOfframpSvmPDA, _                  = state.FindAllowedOfframpPDA(SvmChainSelector, CcipOfframpProgram, CcipRouterProgram)
	RouterEventAuthorityPDA, _, _            = state.FindEventAuthorityPDA(CcipRouterProgram)
	RouterReferenceAddressesPDA, _, _        = state.FindRouterReferenceAddressesPDA(CcipRouterProgram)

	// Offramp PDAs
	OfframpConfigPDA, _, _               = state.FindOfframpConfigPDA(CcipOfframpProgram)
	OfframpReferenceAddressesPDA, _, _   = state.FindOfframpReferenceAddressesPDA(CcipOfframpProgram)
	OfframpEvmSourceChainPDA, _, _       = state.FindOfframpSourceChainPDA(EvmChainSelector, CcipOfframpProgram)
	OfframpSvmSourceChainPDA, _, _       = state.FindOfframpSourceChainPDA(SvmChainSelector, CcipOfframpProgram)
	OfframpBillingSignerPDA, _, _        = state.FindOfframpBillingSignerPDA(CcipOfframpProgram)
	OfframpReceiverExternalExecPDA, _, _ = state.FindExternalExecutionConfigPDA(CcipLogicReceiver, CcipOfframpProgram)
	OfframpEventAuthorityPDA, _, _       = state.FindEventAuthorityPDA(CcipOfframpProgram)

	// fee quoter PDAs
	FqConfigPDA, _, _                     = state.FindFqConfigPDA(FeeQuoterProgram)
	FqEvmDestChainPDA, _, _               = state.FindFqDestChainPDA(EvmChainSelector, FeeQuoterProgram)
	FqSvmDestChainPDA, _, _               = state.FindFqDestChainPDA(SvmChainSelector, FeeQuoterProgram)
	FqEvmDestChainV2PDA, _, _             = state.FindFqDestChainV2PDA(EvmChainSelector, FeeQuoterProgram)
	FqSvmDestChainV2PDA, _, _             = state.FindFqDestChainV2PDA(SvmChainSelector, FeeQuoterProgram)
	FqAllowedPriceUpdaterOfframpPDA, _, _ = state.FindFqAllowedPriceUpdaterPDA(OfframpBillingSignerPDA, FeeQuoterProgram)
	FqEventAuthorityPDA, _, _             = state.FindEventAuthorityPDA(FeeQuoterProgram)

	// RMN Remote PDAs
	RMNRemoteConfigPDA, _, _         = state.FindRMNRemoteConfigPDA(RMNRemoteProgram)
	RMNRemoteCursesPDA, _, _         = state.FindRMNRemoteCursesPDA(RMNRemoteProgram)
	RMNRemoteEventAuthorityPDA, _, _ = state.FindEventAuthorityPDA(RMNRemoteProgram)

	// Committee Verifier PDAs
	CVConfigPDA, _, _               = state.FindConfigPDA(CommitteeVerifierProgram)
	CVStorageLocationsPDA, _, _     = state.FindCVStorageLocationsPDA(CommitteeVerifierProgram)
	CVEventAuthorityPDA, _, _       = state.FindEventAuthorityPDA(CommitteeVerifierProgram)
	CVEvmRemoteChainConfigPDA, _, _ = state.FindCVRemoteChainConfigPDA(EvmChainSelector, CommitteeVerifierProgram)

	// Cctp Verifier PDAs
	CctpVerifierConfigPDA, _, _               = state.FindCVConfigPDA(CctpTokenPoolProgram)
	CctpVerifierEvmRemoteChainConfigPDA, _, _ = state.FindCctpVerifierRemoteChainConfigPDA(EvmChainSelector, CctpTokenPoolProgram)
	CctpVerifierStorageLocationsPDA, _, _     = state.FindCVStorageLocationsPDA(CctpTokenPoolProgram)
	CctpVerifierRouterSignerPDA, _, _         = state.FindCCVSignerPDA(CctpTokenPoolProgram, CcipRouterProgram)
	CctpEventAuthorityPDA, _, _               = state.FindEventAuthorityPDA(CctpTokenPoolProgram)

	// Executor PDAs
	ExecConfigPDA, _, _         = state.FindConfigPDA(ExecutorProgram)
	ExecEventAuthorityPDA, _, _ = state.FindEventAuthorityPDA(ExecutorProgram)
	ExecEvmChainConfigPDA, _, _ = state.FindExecChainConfigPDA(EvmChainSelector, ExecutorProgram)
	ExecSvmChainConfigPDA, _, _ = state.FindExecChainConfigPDA(SvmChainSelector, ExecutorProgram)
)

func init() {
	type entry struct {
		programID solana.PublicKey
		decoder   func(instructionAccounts []*solana.AccountMeta, data []byte) (any, error)
		name      string
	}
	entries := []entry{
		{CcipRouterProgram, decoder(ccip_router.ParseInstructionWithAccounts), "ccip_router"},
		{CcipLogicReceiver, decoder(test_ccip_receiver.ParseInstructionWithAccounts), "test_ccip_receiver"},
		{CcipBaseReceiverV2, decoder(example_ccip_receiver_v2.ParseInstructionWithAccounts), "example_ccip_receiver_v2"},
		{CcipBaseSenderV2, decoder(example_ccip_sender_v2.ParseInstructionWithAccounts), "example_ccip_sender_v2"},
		{CcipInvalidReceiverProgram, decoder(test_ccip_invalid_receiver.ParseInstructionWithAccounts), "test_ccip_invalid_receiver"},
		{ArbitraryCpiProxyProgram, decoder(arbitrary_cpi_proxy.ParseInstructionWithAccounts), "arbitrary_cpi_proxy"},
		{CcipBurnMintTokenPoolProgram, decoder(burnmint_token_pool.ParseInstructionWithAccounts), "burnmint_token_pool"},
		{FeeQuoterProgram, decoder(fee_quoter.ParseInstructionWithAccounts), "fee_quoter"},
		{CcipOfframpProgram, decoder(ccip_offramp.ParseInstructionWithAccounts), "ccip_offramp"},
		{RMNRemoteProgram, decoder(rmn_remote.ParseInstructionWithAccounts), "rmn_remote"},
		{CommitteeVerifierProgram, decoder(committee_verifier.ParseInstructionWithAccounts), "committee_verifier"},
		{ExecutorProgram, decoder(executor.ParseInstructionWithAccounts), "executor"},
		{CctpTokenPoolProgram, decoder(cctp_token_pool.ParseInstructionWithAccounts), "cctp_token_pool"},
	}
	for _, e := range entries {
		if err := solana.RegisterInstructionDecoder(e.programID, e.decoder); err != nil {
			fmt.Printf("warn: failed to register instruction decoder for %s: %s. Depending on the context this may be safe to ignore.\n", e.name, err.Error())
		}
	}
}

func decoder[T any](parseInstructionWithAccounts func(instructionData []byte, accountIndicesData []byte, accountKeys []solana.PublicKey) (T, error)) func(instructionAccounts []*solana.AccountMeta, data []byte) (any, error) {
	return func(instructionAccounts []*solana.AccountMeta, data []byte) (any, error) {
		accountIndicesData := make([]byte, len(instructionAccounts))
		accountKeys := make([]solana.PublicKey, len(instructionAccounts))
		for i, account := range instructionAccounts {
			accountIndicesData[i] = byte(i)
			accountKeys[i] = account.PublicKey
		}
		return parseInstructionWithAccounts(data, accountIndicesData, accountKeys)
	}
}
