// Package commonreader provides shared read-only helpers used by both the
// Solana source reader and destination reader to query on-chain CCIP state.
package commonreader

import (
	"context"
	"encoding/binary"
	"fmt"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	"github.com/smartcontractkit/chainlink-ccv/protocol"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/rmn_remote"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	"github.com/smartcontractkit/chainlink-ccip-solana/pkg/solanarpc"
)

// globalCurseSubject is the 16 byte sentinel used by the RMN contract to denote
// a global curse (affects all lanes). It is compared as a full 16 byte value, so
// endianness normalization must not touch it.
//
// Matches CurseSubject::GLOBAL in contracts/programs/rmn-remote/src/state.rs.
var globalCurseSubject = [16]byte{0: 0x01, 15: 0x01}

// ReadRMNCursedSubjects queries an RMN Remote contract on Solana for
// cursed subjects and returns them as []protocol.Bytes16.
//
// The curses PDA is derived from the RMN Remote program address using the
// static seed "curses".
//
// Returns cursed subjects as bytes16, which can be:
// - Global curse constant (0x0100000000000000000000000000000001)
// - Chain selectors as bytes16s
//
// The Solana RMN Remote contract encodes chain selectors as little endian
// uint64 in bytes [0:8] with bytes [8:16] zero. The rest of the protocol
// (EVM contracts, CCV cursechecker) encodes them as big endian uint64 in
// bytes [8:16] with bytes [0:8] zero. This function normalizes chain selector
// subjects to the protocol wide big endian layout before returning them.
// The global curse sentinel is passed through unchanged because it is compared
// as a full 16 byte value.
func ReadRMNCursedSubjects(
	ctx context.Context,
	rpcClient solanarpc.AccountReader,
	rmnRemoteAddress solana.PublicKey,
) ([]protocol.Bytes16, error) {
	cursesPDA, _, err := state.FindRMNRemoteCursesPDA(rmnRemoteAddress)
	if err != nil {
		return nil, fmt.Errorf("derive RMN remote curses PDA: %w", err)
	}

	resp, err := rpcClient.GetAccountInfo(ctx, cursesPDA, rpc.CommitmentConfirmed)
	// RMN Remote initialize creates the curses account, and no instruction closes it.
	// A missing account means a wrong RMN Remote address or an uninitialized program, not "no curses".
	if err != nil {
		return nil, fmt.Errorf("get RMN curses account (RMN Remote %s, curses PDA %s): %w", rmnRemoteAddress, cursesPDA, err)
	}

	curses, err := rmn_remote.ParseAccount_Curses(resp.GetBinary())
	if err != nil {
		return nil, fmt.Errorf("parse RMN curses account: %w", err)
	}

	result := make([]protocol.Bytes16, len(curses.CursedSubjects))
	for i, subject := range curses.CursedSubjects {
		if subject.Value == globalCurseSubject {
			copy(result[i][:], subject.Value[:])
			continue
		}
		selector := binary.LittleEndian.Uint64(subject.Value[:8])
		binary.BigEndian.PutUint64(result[i][8:], selector)
	}

	return result, nil
}
