package ccip

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"

	bin "github.com/gagliardetto/binary"
	"golang.org/x/crypto/sha3"

	"github.com/smartcontractkit/chainlink-common/pkg/hashutil"
	"github.com/smartcontractkit/chainlink-common/pkg/merklemulti"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/ccip_router"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/config"
)

const (
	GenericExtraArgsV2Tag = "181dcf10"
	GenericExtraArgsV3Tag = "a69dd4aa"
	SVMExtraArgsV1Tag     = "1f3b3aba"
)

// SVMExtraArgsV1 is the extra args structure for SVM (Solana) destination chains
type SVMExtraArgsV1 struct {
	ComputeUnits             uint32
	AccountIsWritableBitmap  uint64
	AllowOutOfOrderExecution bool
	TokenReceiver            [32]uint8
	Accounts                 [][32]uint8
}

var leafDomainSeparator = [32]byte{}

// merkleFrom computes the Merkle root from a slice of byte slices.
func MerkleFrom(leaves [][32]byte) ([32]byte, error) {
	tree, err := merklemulti.NewTree(hashutil.NewKeccak(), leaves)
	if err != nil {
		return [32]byte{}, err
	}
	return tree.Root(), nil
}

func HashSVMToAnyMessage(msg ccip_router.Svm2AnyRampMessage) ([]byte, error) {
	hash := sha3.NewLegacyKeccak256()

	hash.Write(leafDomainSeparator[:])
	hash.Write([]byte("SVM2AnyMessageHashV1"))

	if err := binary.Write(hash, binary.BigEndian, msg.Header.SourceChainSelector); err != nil {
		return nil, err
	}
	if err := binary.Write(hash, binary.BigEndian, msg.Header.DestChainSelector); err != nil {
		return nil, err
	}
	if _, err := hash.Write(config.CcipRouterProgram[:]); err != nil {
		return nil, err
	}
	if _, err := hash.Write(msg.Sender[:]); err != nil {
		return nil, err
	}
	if err := binary.Write(hash, binary.BigEndian, msg.Header.SequenceNumber); err != nil {
		return nil, err
	}
	if err := binary.Write(hash, binary.BigEndian, msg.Header.Nonce); err != nil {
		return nil, err
	}
	if _, err := hash.Write(msg.FeeToken[:]); err != nil {
		return nil, err
	}
	if err := binary.Write(hash, binary.BigEndian, msg.FeeTokenAmount); err != nil {
		return nil, err
	}
	if err := binary.Write(hash, binary.BigEndian, msg.FeeValueJuels); err != nil {
		return nil, err
	}
	if _, err := hash.Write([]byte{uint8(len(msg.Receiver))}); err != nil { //nolint:gosec // G115 length fits in uint8
		return nil, err
	}
	if _, err := hash.Write(msg.Receiver); err != nil {
		return nil, err
	}
	dataLen := uint16(len(msg.Data)) //nolint:gosec // max U16 larger than solana transaction size
	if err := binary.Write(hash, binary.BigEndian, dataLen); err != nil {
		return nil, err
	}
	if _, err := hash.Write(msg.Data); err != nil {
		return nil, err
	}
	tokenAmountsBytes, borshErr := bin.MarshalBorsh(msg.TokenAmounts)
	if borshErr != nil {
		return nil, borshErr
	}
	if _, err := hash.Write(tokenAmountsBytes); err != nil {
		return nil, err
	}
	extraArgsBytes, err := bin.MarshalBorsh(msg.ExtraArgs)
	if err != nil {
		return nil, err
	}
	if _, err := hash.Write(extraArgsBytes); err != nil {
		return nil, err
	}

	return hash.Sum(nil), nil
}

// GenerateBitMapForIndexes generates a bitmap for the given indexes.
func GenerateBitMapForIndexes(indexes []int) uint64 {
	var bitmap uint64

	for _, index := range indexes {
		bitmap |= 1 << index
	}

	return bitmap
}

func SerializeExtraArgs(data any, tag string) ([]byte, error) {
	tagBytes, err := hex.DecodeString(tag)
	if err != nil {
		return nil, err
	}
	v, err := bin.MarshalBorsh(data)
	return append(tagBytes, v...), err
}

func DeserializeExtraArgs(obj any, data []byte, tag string) error {
	tagBytes, err := hex.DecodeString(tag)
	if err != nil {
		return err
	}

	if len(data) < len(tagBytes) {
		return fmt.Errorf("extra args too short: got %d bytes, need at least %d for tag", len(data), len(tagBytes))
	}

	if !bytes.Equal(data[:len(tagBytes)], tagBytes) {
		return fmt.Errorf("mismatched tag: %s != %s", hex.EncodeToString(data[:len(tagBytes)]), tag)
	}

	err = bin.UnmarshalBorsh(obj, data[len(tagBytes):])
	return err
}
