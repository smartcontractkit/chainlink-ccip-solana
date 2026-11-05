//revive:disable:var-naming // legacy package name
package common

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/gagliardetto/solana-go"
	addresslookuptable "github.com/gagliardetto/solana-go/programs/address-lookup-table"
	"github.com/gagliardetto/solana-go/rpc"
)

var (
	AddressLookupTableProgram = solana.MustPublicKeyFromBase58("AddressLookupTab1e1111111111111111111111111")
)

// https://github.com/anza-xyz/agave/blob/master/programs/address-lookup-table/src/processor.rs
// https://github.com/anza-xyz/agave/blob/489f483e1d7b30ef114e0123994818b2accfa389/sdk/program/src/address_lookup_table/instruction.rs
const (
	InstructionCreateLookupTable uint32 = iota
	InstructionFreezeLookupTable
	InstructionExtendLookupTable
	InstructionDeactivateLookupTable
	InstructionCloseLookupTable
)

func NewCreateLookupTableInstruction(
	authority,
	funder solana.PublicKey,
	slot uint64,
) (solana.PublicKey, solana.Instruction, error) {
	account, bumpSeed, err := FindLookupTablePDA(authority, slot)
	if err != nil {
		return solana.PublicKey{}, nil, err
	}

	data := binary.LittleEndian.AppendUint32([]byte{}, InstructionCreateLookupTable)
	data = binary.LittleEndian.AppendUint64(data, slot)
	data = append(data, bumpSeed)
	return account, solana.NewInstruction(
		AddressLookupTableProgram,
		solana.AccountMetaSlice{
			solana.Meta(account).WRITE(),
			solana.Meta(authority).SIGNER(),
			solana.Meta(funder).SIGNER().WRITE(),
			solana.Meta(solana.SystemProgramID),
		},
		data,
	), nil
}

func NewExtendLookupTableInstruction(
	table, authority, funder solana.PublicKey,
	accounts []solana.PublicKey,
) solana.Instruction {
	// https://github.com/solana-labs/solana-web3.js/blob/c1c98715b0c7900ce37c59bffd2056fa0037213d/src/programs/address-lookup-table/index.ts#L113

	data := binary.LittleEndian.AppendUint32([]byte{}, InstructionExtendLookupTable)
	data = binary.LittleEndian.AppendUint64(data, uint64(len(accounts))) // note: this is usually u32 + 8 byte buffer
	for _, a := range accounts {
		data = append(data, a.Bytes()...)
	}

	return solana.NewInstruction(
		AddressLookupTableProgram,
		solana.AccountMetaSlice{
			solana.Meta(table).WRITE(),
			solana.Meta(authority).SIGNER(),
			solana.Meta(funder).SIGNER().WRITE(),
			solana.Meta(solana.SystemProgramID),
		},
		data,
	)
}

func CreateLookupTable(ctx context.Context, client *rpc.Client, admin solana.PrivateKey) (solana.PublicKey, error) {
	slot, serr := client.GetSlot(ctx, rpc.CommitmentFinalized)
	if serr != nil {
		return solana.PublicKey{}, serr
	}

	table, instruction, ierr := NewCreateLookupTableInstruction(
		admin.PublicKey(),
		admin.PublicKey(),
		slot-1, // Using the most recent slot sometimes results in errors when submitting the transaction.
	)
	if ierr != nil {
		return solana.PublicKey{}, ierr
	}

	_, err := SendAndConfirmWithLookupTablesAndRetries(ctx, client, []solana.Instruction{instruction}, admin, rpc.CommitmentConfirmed, map[solana.PublicKey]solana.PublicKeySlice{})
	return table, err
}

// A single extend instruction must fit in one 1232-byte transaction and each address costs 32
// bytes of instruction data, so larger entry lists are extended in chunks (in order, so the
// resulting table mirrors the input slice).
const maxExtendAddressesPerTx = 20

func ExtendLookupTable(ctx context.Context, client *rpc.Client, table solana.PublicKey, admin solana.PrivateKey, entries []solana.PublicKey) error {
	for chunk := range slices.Chunk(entries, maxExtendAddressesPerTx) {
		_, err := SendAndConfirmWithLookupTablesAndRetries(ctx, client, []solana.Instruction{
			NewExtendLookupTableInstruction(
				table,
				admin.PublicKey(),
				admin.PublicKey(),
				chunk,
			),
		}, admin, rpc.CommitmentConfirmed, map[solana.PublicKey]solana.PublicKeySlice{})
		if err != nil {
			return err
		}
	}
	return nil
}

func AwaitSlotChange(ctx context.Context, client *rpc.Client) error {
	originalSlot, err := client.GetSlot(ctx, rpc.CommitmentConfirmed)
	if err != nil {
		return err
	}
	newSlot := originalSlot
	for newSlot == originalSlot {
		newSlot, err = client.GetSlot(ctx, rpc.CommitmentConfirmed)
		if err != nil {
			return err
		}
		time.Sleep(10 * time.Millisecond)
	}
	return nil
}

func SetupLookupTable(ctx context.Context, client *rpc.Client, admin solana.PrivateKey, entries []solana.PublicKey) (solana.PublicKey, error) {
	table, err := CreateLookupTable(ctx, client, admin)
	if err != nil {
		return solana.PublicKey{}, err
	}

	err = ExtendLookupTable(ctx, client, table, admin, entries)
	if err != nil {
		return solana.PublicKey{}, err
	}

	// Address lookup tables have to "warm up" for at least 1 slot before they can be used.
	// So, we wait for a new slot to be produced before returning the table, so it's available
	// and warmed up as soon as this method returns it.
	err = AwaitSlotChange(ctx, client)
	if err != nil {
		return solana.PublicKey{}, err
	}

	return table, nil
}

// AccountInfoReader is the read-only RPC surface lookup-table fetching needs.
type AccountInfoReader interface {
	GetAccountInfoWithOpts(ctx context.Context, addr solana.PublicKey, opts *rpc.GetAccountInfoOpts) (*rpc.GetAccountInfoResult, error)
}

// GetAddressLookupTable fetches and decodes the on-chain entries of a lookup table.
func GetAddressLookupTable(ctx context.Context, client AccountInfoReader, lookupTablePublicKey solana.PublicKey) ([]solana.PublicKey, error) {
	account, err := client.GetAccountInfoWithOpts(ctx, lookupTablePublicKey, &rpc.GetAccountInfoOpts{
		Commitment: rpc.CommitmentConfirmed,
	})
	if err != nil {
		return []solana.PublicKey{}, err
	}
	if account == nil || account.Value == nil {
		return []solana.PublicKey{}, errors.New("account not found")
	}
	lookupTableState, err := addresslookuptable.DecodeAddressLookupTableState(account.GetBinary())
	if err != nil {
		return []solana.PublicKey{}, err
	}

	return lookupTableState.Addresses, nil
}

// GetAddressLookupTableMap fetches on-chain entries for each lookup table pubkey.
func GetAddressLookupTableMap(
	ctx context.Context,
	client AccountInfoReader,
	lookupTables []solana.PublicKey,
) (map[solana.PublicKey]solana.PublicKeySlice, error) {
	if len(lookupTables) == 0 {
		return nil, nil
	}
	lutMap := make(map[solana.PublicKey]solana.PublicKeySlice, len(lookupTables))
	for _, lut := range lookupTables {
		entries, err := GetAddressLookupTable(ctx, client, lut)
		if err != nil {
			return nil, fmt.Errorf("fetch lookup table %s: %w", lut.String(), err)
		}
		lutMap[lut] = entries
	}
	return lutMap, nil
}

// https://github.com/solana-labs/solana-web3.js/blob/c1c98715b0c7900ce37c59bffd2056fa0037213d/src/programs/address-lookup-table/index.ts#L274
func FindLookupTablePDA(authority solana.PublicKey, slot uint64) (solana.PublicKey, uint8, error) {
	slotLE := make([]byte, 8)
	binary.LittleEndian.PutUint64(slotLE, slot)
	return solana.FindProgramAddress([][]byte{authority.Bytes(), slotLE}, AddressLookupTableProgram)
}
