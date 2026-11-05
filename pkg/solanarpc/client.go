package solanarpc

import (
	"context"
	"errors"
	"fmt"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	chainsel "github.com/smartcontractkit/chain-selectors"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-framework/metrics"
	mn "github.com/smartcontractkit/chainlink-framework/multinode"
	solclient "github.com/smartcontractkit/chainlink-solana/pkg/solana/client"
	solconfig "github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
)

// Client is one chain's multinode RPC and transaction transport.
type Client struct {
	multiNode *mn.MultiNode[mn.StringID, *solclient.MultiNodeClient]
	txSender  *mn.TransactionSender[*solana.Transaction, solana.Signature, mn.StringID, *solclient.MultiNodeClient]
}

var (
	_ AccountReader   = (*Client)(nil)
	_ TxReader        = (*Client)(nil)
	_ BlockReader     = (*Client)(nil)
	_ LogPollerReader = (*Client)(nil)
	_ Simulator       = (*Client)(nil)
	_ BlockhashReader = (*Client)(nil)
)

// NewClient constructs and starts a client. The caller owns its lifecycle.
func NewClient(ctx context.Context, cfg Config, lggr logger.Logger) (*Client, error) {
	tomlCfg := solconfig.Defaults()
	tomlCfg.ChainID = &cfg.ChainID
	tomlCfg.SetDefaults()

	finalized := string(rpc.CommitmentFinalized)
	tomlCfg.Chain.Commitment = &finalized
	mnCfg := &tomlCfg.MultiNode

	multiNodeMetrics, err := metrics.NewGenericMultiNodeMetrics("solana", cfg.ChainID)
	if err != nil {
		return nil, fmt.Errorf("create multinode metrics: %w", err)
	}

	if err = ValidateAndNormalizeNodes(cfg.Nodes); err != nil {
		return nil, fmt.Errorf("validate nodes: %w", err)
	}

	var (
		nodes          []mn.Node[mn.StringID, *solclient.MultiNodeClient]
		createdClients []*solclient.MultiNodeClient
	)

	closeCreated := func() {
		for _, c := range createdClients {
			c.Close()
		}
	}

	for i, nodeCfg := range cfg.Nodes {
		// assume here ValidateAndNormalizeNodes checked all nodeCfg nil-ptr
		name := *nodeCfg.Name
		var rpcClient *solclient.MultiNodeClient
		rpcClient, err = solclient.NewMultiNodeClient(nodeCfg.URL.String(), &tomlCfg, cfg.RPCTimeout, logger.Named(lggr, "Client."+name))
		if err != nil {
			closeCreated()
			return nil, fmt.Errorf("create multinode client for %s: %w", name, err)
		}
		createdClients = append(createdClients, rpcClient)
		node := mn.NewNode[mn.StringID, *solclient.Head, *solclient.MultiNodeClient](
			mnCfg, mnCfg, lggr, multiNodeMetrics, nil, nodeCfg.URL.URL(), name,
			i, mn.StringID(cfg.ChainID), *nodeCfg.Order, rpcClient, chainsel.FamilySolana, *nodeCfg.IsLoadBalancedRPC,
		)
		nodes = append(nodes, node)
	}

	multiNode := mn.NewMultiNode[mn.StringID, *solclient.MultiNodeClient](
		lggr, multiNodeMetrics, mnCfg.SelectionMode(), mnCfg.LeaseDuration(),
		nodes, nil, mn.StringID(cfg.ChainID), "solana", mnCfg.DeathDeclarationDelay(),
	)
	txSender := mn.NewTransactionSender[*solana.Transaction, solana.Signature, mn.StringID, *solclient.MultiNodeClient](
		lggr,
		mn.StringID(cfg.ChainID),
		"solana",
		multiNode,
		multiNodeMetrics,
		func(err error) mn.SendTxReturnCode { return solclient.ClassifySendError(nil, err) },
		0,
	)

	client := &Client{multiNode: multiNode, txSender: txSender}
	if err = client.start(ctx); err != nil {
		if closeErr := client.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close transport after start failure: %w", closeErr))
		}
		return nil, err
	}

	return client, nil
}

func (c *Client) start(ctx context.Context) error {
	if err := c.multiNode.Start(ctx); err != nil {
		return fmt.Errorf("start multinode: %w", err)
	}
	if err := c.txSender.Start(ctx); err != nil {
		return fmt.Errorf("start transaction sender: %w", err)
	}
	return nil
}

func (c *Client) Close() error {
	return errors.Join(c.txSender.Close(), c.multiNode.Close())
}

func (c *Client) Select(ctx context.Context) (solclient.ReaderWriter, error) {
	return c.multiNode.SelectRPC(ctx)
}

// Send broadcasts a transaction via the multinode TransactionSender.
// Node selection/broadcast is handled internally; no need to call Select here.
func (c *Client) Send(ctx context.Context, tx *solana.Transaction) (solana.Signature, error) {
	sig, _, err := c.txSender.SendTransaction(ctx, tx)
	return sig, err
}

func (c *Client) GetBlockWithOpts(ctx context.Context, slot uint64, opts *rpc.GetBlockOpts) (*rpc.GetBlockResult, error) {
	r, err := c.Select(ctx)
	if err != nil {
		return nil, err
	}
	return r.GetBlockWithOpts(ctx, slot, opts)
}

func (c *Client) GetSlot(ctx context.Context, commitment rpc.CommitmentType) (uint64, error) {
	r, err := c.Select(ctx)
	if err != nil {
		return 0, err
	}
	return r.SlotHeightWithCommitment(ctx, commitment)
}

func (c *Client) GetTransaction(ctx context.Context, sig solana.Signature) (*rpc.GetTransactionResult, error) {
	r, err := c.Select(ctx)
	if err != nil {
		return nil, err
	}
	return r.GetTransaction(ctx, sig)
}

func (c *Client) SimulateTransactionWithOpts(ctx context.Context, tx *solana.Transaction, opts *rpc.SimulateTransactionOpts) (*rpc.SimulateTransactionResponse, error) {
	r, err := c.Select(ctx)
	if err != nil {
		return nil, err
	}
	res, err := r.SimulateTx(ctx, tx, opts)
	if err != nil {
		return nil, err
	}
	return &rpc.SimulateTransactionResponse{Value: res}, nil
}

func (c *Client) GetFirstAvailableBlock(ctx context.Context) (uint64, error) {
	r, err := c.Select(ctx)
	if err != nil {
		return 0, err
	}
	return r.GetFirstAvailableBlock(ctx)
}

func (c *Client) GetSignaturesForAddressWithOpts(ctx context.Context, account solana.PublicKey, opts *rpc.GetSignaturesForAddressOpts) ([]*rpc.TransactionSignature, error) {
	r, err := c.Select(ctx)
	if err != nil {
		return nil, err
	}
	return r.GetSignaturesForAddressWithOpts(ctx, account, opts)
}

func (c *Client) GetAccountInfo(ctx context.Context, addr solana.PublicKey, commitment rpc.CommitmentType) (*rpc.GetAccountInfoResult, error) {
	return c.GetAccountInfoWithOpts(ctx, addr, &rpc.GetAccountInfoOpts{
		Commitment: commitment,
		Encoding:   solana.EncodingBase64,
	})
}

// GetAccountInfoWithOpts is retained for contract lookup-table helpers that
// also accept a raw *rpc.Client. Runtime readers use GetAccountInfo instead.
func (c *Client) GetAccountInfoWithOpts(ctx context.Context, addr solana.PublicKey, opts *rpc.GetAccountInfoOpts) (*rpc.GetAccountInfoResult, error) {
	r, err := c.Select(ctx)
	if err != nil {
		return nil, err
	}
	return r.GetAccountInfoWithOpts(ctx, addr, opts)
}

func (c *Client) GetLatestBlockHeight(ctx context.Context) (uint64, error) {
	r, err := c.Select(ctx)
	if err != nil {
		return 0, err
	}
	return r.GetLatestBlockHeight(ctx)
}

func (c *Client) GetLatestBlockhash(ctx context.Context) (*rpc.GetLatestBlockhashResult, error) {
	r, err := c.Select(ctx)
	if err != nil {
		return nil, err
	}
	return r.LatestBlockhash(ctx)
}
