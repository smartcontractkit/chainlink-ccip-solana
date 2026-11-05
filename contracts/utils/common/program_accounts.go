package common

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
)

// GetInitializedAccount fetches the account at address and parses it with parser, typically a
// generated ParseAccount_* function, which also checks the account discriminator. It returns nil
// when the account is not initialized: missing, or owned by the System program, because anyone
// can send lamports to a PDA before the program's init takes it over. RPC failures, accounts
// owned by a program other than owner, and data that does not parse are returned as errors.
func GetInitializedAccount[T any](ctx context.Context, client *rpc.Client, address, owner solana.PublicKey, commitment rpc.CommitmentType, parser func([]byte) (*T, error)) (*T, error) {
	data, initialized, err := getProgramAccountData(ctx, client, address, owner, commitment)
	if err != nil || !initialized {
		return nil, err
	}
	account, err := parser(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse account %s: %w", address, err)
	}
	return account, nil
}

// IsAccountInitialized reports whether the account at address is initialized as the account type
// identified by discriminator. Missing and System-owned accounts are not initialized; the error
// cases match GetInitializedAccount.
func IsAccountInitialized(ctx context.Context, client *rpc.Client, address, owner solana.PublicKey, commitment rpc.CommitmentType, discriminator [8]byte) (bool, error) {
	data, initialized, err := getProgramAccountData(ctx, client, address, owner, commitment)
	if err != nil || !initialized {
		return false, err
	}
	if !bytes.HasPrefix(data, discriminator[:]) {
		return false, fmt.Errorf("account %s is owned by %s but its discriminator does not match the expected account type", address, owner)
	}
	return true, nil
}

// getProgramAccountData fetches the account at address. initialized is false, with no error, when
// the account is missing or owned by the System program; data is then nil.
func getProgramAccountData(ctx context.Context, client *rpc.Client, address, owner solana.PublicKey, commitment rpc.CommitmentType) (data []byte, initialized bool, err error) {
	info, err := client.GetAccountInfoWithOpts(ctx, address, &rpc.GetAccountInfoOpts{Commitment: commitment})
	switch {
	case errors.Is(err, rpc.ErrNotFound):
		return nil, false, nil
	case err != nil:
		return nil, false, fmt.Errorf("failed to fetch account %s: %w", address, err)
	case info.Value.Owner.Equals(solana.SystemProgramID):
		return nil, false, nil
	case !info.Value.Owner.Equals(owner):
		return nil, false, fmt.Errorf("account %s is owned by %s, not %s", address, info.Value.Owner, owner)
	}
	return info.Value.Data.GetBinary(), true, nil
}
