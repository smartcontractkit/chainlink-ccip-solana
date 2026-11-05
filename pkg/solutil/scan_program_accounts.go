package solutil

import (
	"context"
	"fmt"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
)

// DefaultScanProgramAccountsBatchSize bounds each getMultipleAccounts payload; well
// under maximum of 100 accounts cap.
const DefaultScanProgramAccountsBatchSize = 10

// ScanProgramAccountsByDiscriminator lists PDAs owned by programID matching the given
// Anchor 8-byte discriminator at offset 0 via getProgramAccounts (pubkeys only), then
// fetches account data via batched getMultipleAccounts. Two-step to stay under RPC
// response-size limits as account count and per-account size grow; sequential batches
// to avoid RPC rate limits. If batchSize <= 0, DefaultScanProgramAccountsBatchSize is used.
func ScanProgramAccountsByDiscriminator(
	ctx context.Context,
	client *rpc.Client,
	programID solana.PublicKey,
	discriminator [8]byte,
	batchSize int,
) ([]rpc.KeyedAccount, error) {
	if batchSize <= 0 {
		batchSize = DefaultScanProgramAccountsBatchSize
	}

	zero := uint64(0)
	listed, err := client.GetProgramAccountsWithOpts(
		ctx,
		programID,
		&rpc.GetProgramAccountsOpts{
			Commitment: rpc.CommitmentConfirmed,
			Filters: []rpc.RPCFilter{{
				Memcmp: &rpc.RPCFilterMemcmp{Offset: 0, Bytes: discriminator[:]},
			}},
			DataSlice: &rpc.DataSlice{Offset: &zero, Length: &zero},
		},
	)
	if err != nil {
		return nil, fmt.Errorf("list program accounts for %s: %w", programID, err)
	}
	if len(listed) == 0 {
		return []rpc.KeyedAccount{}, nil
	}

	pubkeys := make([]solana.PublicKey, len(listed))
	for i, a := range listed {
		pubkeys[i] = a.Pubkey
	}

	out := make([]rpc.KeyedAccount, 0, len(pubkeys))
	for start := 0; start < len(pubkeys); start += batchSize {
		end := min(start+batchSize, len(pubkeys))
		batch := pubkeys[start:end]
		resp, err := client.GetMultipleAccountsWithOpts(ctx, batch, &rpc.GetMultipleAccountsOpts{
			Commitment: rpc.CommitmentConfirmed,
			Encoding:   solana.EncodingBase64,
		})
		if err != nil {
			return nil, fmt.Errorf("get accounts batch [%d:%d] for %s: %w", start, end, programID, err)
		}
		if len(resp.Value) != len(batch) {
			return nil, fmt.Errorf("get accounts batch [%d:%d] for %s: expected %d, got %d",
				start, end, programID, len(batch), len(resp.Value))
		}
		for i, acct := range resp.Value {
			out = append(out, rpc.KeyedAccount{Pubkey: batch[i], Account: acct})
		}
	}
	return out, nil
}
