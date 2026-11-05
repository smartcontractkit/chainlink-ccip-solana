// Package solanarpc owns Solana multinode construction, lifecycle, and the RPC
// interfaces exposed to CCIP-Solana components.
package solanarpc

import (
	"context"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	solconfig "github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
)

type Config struct {
	ChainID    string
	Nodes      []*solconfig.Node
	RPCTimeout time.Duration
}

// AccountReader fixes account encoding to base64 while preserving the caller's
// commitment choice.
type AccountReader interface {
	GetAccountInfo(ctx context.Context, addr solana.PublicKey, commitment rpc.CommitmentType) (*rpc.GetAccountInfoResult, error)
}

// TxReader omits opts because Client is configured for finalized reads and the
// underlying chainlink-solana client fixes encoding and transaction version.
type TxReader interface {
	GetTransaction(ctx context.Context, signature solana.Signature) (*rpc.GetTransactionResult, error)
}

type SlotReader interface {
	GetSlot(ctx context.Context, commitment rpc.CommitmentType) (uint64, error)
}

type BlockReader interface {
	SlotReader
	GetBlockWithOpts(ctx context.Context, slot uint64, opts *rpc.GetBlockOpts) (*rpc.GetBlockResult, error)
}

type LogPollerReader interface {
	GetFirstAvailableBlock(ctx context.Context) (uint64, error)
	GetBlockWithOpts(ctx context.Context, slot uint64, opts *rpc.GetBlockOpts) (*rpc.GetBlockResult, error)
	GetSignaturesForAddressWithOpts(ctx context.Context, account solana.PublicKey, opts *rpc.GetSignaturesForAddressOpts) ([]*rpc.TransactionSignature, error)
	GetSlot(ctx context.Context, commitment rpc.CommitmentType) (uint64, error)
}

type Simulator interface {
	SimulateTransactionWithOpts(ctx context.Context, tx *solana.Transaction, opts *rpc.SimulateTransactionOpts) (*rpc.SimulateTransactionResponse, error)
}

type BlockhashReader interface {
	GetLatestBlockhash(ctx context.Context) (*rpc.GetLatestBlockhashResult, error)
	GetLatestBlockHeight(ctx context.Context) (uint64, error)
}
