package ccip

import (
	"bytes"
	"encoding/binary"
	"fmt"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/ccip_common"
)

const SvmExecutorArgsV1Tag = uint32(0x1a2b3c4d)

const FixedAbiEncodedExecutorArgsLen = 14 // tag(4) + useAta(1) + writableBitmap(8) + numAccounts(1)

type SVMExecutorArgs struct {
	ccip_common.SvmDestBlob // the executor args are the same values as the dest blob
}

func (s *SVMExecutorArgs) BorshEncode() ([]byte, error) {
	bytes, err := s.Marshal()
	if err != nil {
		return nil, err
	}

	buffer := make([]byte, 4+len(bytes))
	binary.LittleEndian.PutUint32(buffer, SvmExecutorArgsV1Tag)
	copy(buffer[4:], bytes)
	return buffer, nil
}

// BorshDecode rejects what the offramp's borsh_decode rejects: trailing bytes and an unknown UseAta.
func (s *SVMExecutorArgs) BorshDecode(data []byte) error {
	if len(data) < 4 {
		return fmt.Errorf("data too short to contain tag - length: %d", len(data))
	}

	tag := binary.LittleEndian.Uint32(data[:4])
	if tag != SvmExecutorArgsV1Tag {
		return fmt.Errorf("unexpected executor args tag for borsh-decoding: %v", data[:4])
	}

	var blob ccip_common.SvmDestBlob
	decoder := bin.NewBorshDecoder(data[4:])
	if err := blob.UnmarshalWithDecoder(decoder); err != nil {
		return fmt.Errorf("failed to unmarshal SvmDestBlob: %w", err)
	}
	if decoder.HasRemaining() {
		return fmt.Errorf("dest blob has %d trailing bytes", decoder.Remaining())
	}
	if blob.UseAta > ccip_common.SvmTokenReceiverUsage_UseAsIs {
		return fmt.Errorf("invalid UseAta value: %d", blob.UseAta)
	}

	*s = SVMExecutorArgs{blob}
	return nil
}

func (s *SVMExecutorArgs) AbiEncodePacked() ([]byte, error) {
	bufferLen := FixedAbiEncodedExecutorArgsLen + 32*len(s.Accounts)

	buffer := make([]byte, bufferLen)
	binary.BigEndian.PutUint32(buffer, SvmExecutorArgsV1Tag)
	buffer[4] = byte(s.UseAta)
	binary.BigEndian.PutUint64(buffer[5:], s.AccountIsWritableBitmap)
	l := len(s.Accounts)
	if l > 255 {
		// Solana limits the number of accounts in a single tx to 64 anyway (with plans to increase to 128),
		// so it's invalid anyway to send more than that. This is also limited by the writable bitmap being a u64,
		// but we check against 255 to ensure it fits in a single byte for the ABI encoding.
		return nil, fmt.Errorf("number of accounts exceeds maximum representable in a single byte: %d", l)
	}
	buffer[13] = byte(l)
	for i, account := range s.Accounts {
		copy(buffer[14+i*32:], account[:])
	}

	return buffer, nil
}

func (s *SVMExecutorArgs) AbiDecodePacked(data []byte) error {
	if len(data) < 14 {
		return fmt.Errorf("data too short to contain SVMExecutorArgs - length: %d", len(data))
	}
	tag := binary.BigEndian.Uint32(data[:4])
	if tag != SvmExecutorArgsV1Tag {
		return fmt.Errorf("unexpected tag for ABI-decoding SVMExecutorArgs: %v", data[:4])
	}

	useAtaByte := data[4]
	if useAtaByte > byte(ccip_common.SvmTokenReceiverUsage_UseAsIs) {
		return fmt.Errorf("invalid UseAta value: %d", useAtaByte)
	}
	useAta := ccip_common.SvmTokenReceiverUsage(useAtaByte)
	accountIsWritableBitmap := binary.BigEndian.Uint64(data[5:13])
	numAccounts := int(data[13])

	if len(data[FixedAbiEncodedExecutorArgsLen:]) != 32*numAccounts {
		return fmt.Errorf("data length does not match expected length for number of accounts - expected: %d, got: %d", 32*numAccounts, len(data[FixedAbiEncodedExecutorArgsLen:]))
	}
	for i := range numAccounts {
		var account [32]byte
		copy(account[:], data[FixedAbiEncodedExecutorArgsLen+i*32:FixedAbiEncodedExecutorArgsLen+(i+1)*32])
		s.Accounts = append(s.Accounts, solana.PublicKey(account))
	}

	*s = SVMExecutorArgs{
		SvmDestBlob: ccip_common.SvmDestBlob{
			UseAta:                  useAta,
			AccountIsWritableBitmap: accountIsWritableBitmap,
			Accounts:                s.Accounts,
		},
	}
	return nil
}

// ValidateDestBlob accepts exactly the blobs that the offramp's SVMDestBlob::from_bytes accepts,
// and returns the number of accounts in the blob.
func ValidateDestBlob(data []byte) (int, error) {
	var args SVMExecutorArgs
	decode := args.BorshDecode
	if bytes.HasPrefix(data, binary.BigEndian.AppendUint32(nil, SvmExecutorArgsV1Tag)) {
		decode = args.AbiDecodePacked
	}
	if err := decode(data); err != nil {
		return 0, err
	}
	return len(args.Accounts), nil
}
