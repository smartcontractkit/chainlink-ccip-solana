package testutils

import (
	"encoding/binary"
	"encoding/hex"
	"math/big"
	"slices"
	"strconv"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
	"github.com/gagliardetto/solana-go"

	"github.com/smartcontractkit/chainlink-ccv/protocol"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/ccip_common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/config"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/testutils/eth"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/ccip"
)

var committeeVerifierVersionTag, _ = hex.DecodeString("e9a05a20")

func MinimalMessageV1(sourceChainSelector uint64) protocol.Message {
	senderAddress, err := protocol.NewByteSliceFromHex(
		"0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaac",
	)
	if err != nil {
		panic("MinimalMessageV1 sender address error: " + err.Error())
	}

	// The receiver is `test_ccip_receiver`, whose `ccip_receive` expects (after the
	// offramp-prepended [external_execution_signer, offramp, allowed_offramp]) its own
	// [external_execution_config, counter, system_program] accounts. external_execution_config
	// and counter are writable (bits 0 and 1); system_program is read-only.
	executorArgs := ccip.SVMExecutorArgs{
		SvmDestBlob: ccip_common.SvmDestBlob{
			UseAta:                  ccip_common.SvmTokenReceiverUsage_DeriveAtaDontCreate,
			AccountIsWritableBitmap: 0b011,
			Accounts: []solana.PublicKey{
				config.ReceiverExternalExecutionConfigPDA,
				config.ReceiverTargetAccountPDA,
				solana.SystemProgramID,
			},
		},
	}
	executorArgsBytes, err := executorArgs.AbiEncodePacked() // could also be borsh-encoded here, both are supported
	if err != nil {
		panic("MinimalMessageV1 executor args encoding error: " + err.Error())
	}

	executorArgsLength := len(executorArgsBytes)
	if executorArgsLength > 0xffff { // uint16 max
		panic("MinimalMessageV1: executor args too long: " + strconv.Itoa(executorArgsLength))
	}

	return protocol.Message{
		Sender: protocol.UnknownAddress(senderAddress),
		Data:   []byte{0xde, 0xad, 0xbe, 0xef},
		OnRampAddress: []byte{
			0xaa, 0xbb, 0xcc, 0xdd,
			0xee, 0xff, 0x00, 0x11,
			0x22, 0x33, 0x44, 0x55,
			0x66, 0x77, 0x88, 0x99,
			0xaa, 0xbb, 0xcc, 0xdd,
			0xee, 0xff, 0x00, 0x11,
			0x22, 0x33, 0x44, 0x55,
			0x66, 0x77, 0x88, 0x99,
		},
		TokenTransfer:  nil,
		OffRampAddress: config.CcipOfframpProgram[:],
		DestBlobLength: uint16(executorArgsLength),
		DestBlob:       executorArgsBytes,
		// Real deployed receiver so the offramp's ccip_receive CPI targets an executable program.
		Receiver:            config.CcipLogicReceiver.Bytes(),
		SourceChainSelector: protocol.ChainSelector(sourceChainSelector),
		DestChainSelector:   protocol.ChainSelector(config.SvmChainSelector),
		SequenceNumber:      1,
		ExecutionGasLimit:   500_000,
		CcipReceiveGasLimit: 300_000,
		Finality:            0,
		CcvAndExecutorHash: [32]byte{
			0xd5, 0x64, 0x76, 0x82,
			0x7a, 0xb6, 0xfc, 0x2d,
			0x55, 0x98, 0xb5, 0x7e,
			0x90, 0x2b, 0xe9, 0xcc,
			0xd6, 0xf7, 0x8e, 0x6f,
			0x25, 0x56, 0x18, 0xf4,
			0x61, 0xc4, 0xfb, 0x96,
			0xdc, 0xfb, 0x97, 0xdf,
		},
		TokenTransferLength:  0,
		DataLength:           4,
		ReceiverLength:       32,
		SenderLength:         32,
		Version:              1,
		OffRampAddressLength: 32,
		OnRampAddressLength:  32,
	}
}

// EncodeMinimalMessageV1 builds a minimal valid MessageV1 wire-format byte blob
// that can be parsed by the on-chain MessageV1::from_bytes.
func EncodeMinimalMessageV1(sourceChainSelector uint64) []byte {
	msg := MinimalMessageV1(sourceChainSelector)
	encoded, err := msg.Encode()
	if err != nil {
		panic("EncodeMinimalMessageV1: " + err.Error())
	}
	return encoded
}

// SignCommitteeVerifierMessage builds the verifier_results wire format for the
// committee verifier's verify instruction. It signs keccak256(VERSION_TAG || messageID) with the
// first `threshold` signers, normalizing all signatures to V=27 (recovery_id=0) via ECDSA
// malleability. This matches the on-chain `compute_signed_hash` and the off-chain committee signer
// (chainlink-ccv `committee/common/hash.go`).
func SignCommitteeVerifierMessage(messageID [32]byte, signers []eth.Signer, threshold int) ([]byte, error) {
	hashInput := slices.Concat(committeeVerifierVersionTag, messageID[:])
	signedHash := eth.Keccak256(hashInput)

	signatures := make([]byte, 0, threshold*64)
	for i := range threshold {
		privKey := secp256k1.PrivKeyFromBytes(signers[i].PrivateKey)
		baseSig := ecdsa.SignCompact(privKey, signedHash, false)

		v := baseSig[0] // 27 or 28
		r := baseSig[1:33]
		s := baseSig[33:65]

		if v == 28 {
			// Flip S to N - S to normalize to V=27 (recovery_id=0)
			sBig := new(big.Int).SetBytes(s)
			sBig.Sub(eth.Secp256k1N, sBig)
			sBytes := sBig.Bytes()
			s = make([]byte, 32)
			copy(s[32-len(sBytes):], sBytes) // left-pad to 32 bytes
		}

		signatures = append(signatures, r...)
		signatures = append(signatures, s...)
	}

	sigByteLen := len(signatures)
	result := make([]byte, 0, 4+2+sigByteLen)
	result = append(result, committeeVerifierVersionTag...)
	result = binary.BigEndian.AppendUint16(result, uint16(sigByteLen)) //nolint:gosec // sigByteLen = threshold*64, fits uint16 for any realistic committee size
	result = append(result, signatures...)
	return result, nil
}
