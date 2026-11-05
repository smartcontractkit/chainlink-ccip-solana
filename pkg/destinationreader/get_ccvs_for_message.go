package destinationreader

import (
	"context"
	"fmt"

	"github.com/gagliardetto/solana-go"

	"github.com/smartcontractkit/chainlink-ccv/protocol"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/ccip_offramp"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/ccip"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
)

type ccvMessageRPC interface {
	ccip.AccountResolutionRPC
	common.AccountInfoReader
}

// getCCVsForMessage simulates the offramp get_ccvs_for_msg instruction and returns the
// onchain CCV quorum for the encoded message. Token-transfer messages require pool
// accounts resolved via the offramp account-resolution flow
func getCCVsForMessage(
	ctx context.Context,
	lggr logger.Logger,
	rpcClient ccvMessageRPC,
	offrampProgramID solana.PublicKey,
	encodedMsg []byte,
	payer solana.PublicKey,
) (*ccip_offramp.GetCcvsForMsgResponse, error) {
	resolved, err := ccip.ResolveGetCCVsForMsgAccounts(
		ctx, rpcClient, offrampProgramID, encodedMsg, payer,
		logger.Named(lggr, "ccip.ResolveGetCCVsForMsgAccounts"),
	)
	if err != nil {
		return nil, fmt.Errorf("resolve get_ccvs_for_msg accounts: %w", err)
	}

	lutMap, err := common.GetAddressLookupTableMap(ctx, rpcClient, resolved.LookupTables)
	if err != nil {
		return nil, err
	}

	tx, opts, err := common.BuildReadOnlySimulationTxWithLookupTables(payer, lutMap, resolved.Ix)
	if err != nil {
		return nil, err
	}
	res, err := rpcClient.SimulateTransactionWithOpts(ctx, tx, opts)
	if err != nil {
		return nil, err
	}
	if res.Value.Err != nil {
		// Include the program logs for onchain error details (account name)
		return nil, fmt.Errorf("get_ccvs_for_msg simulation failed: %v, logs: %q", res.Value.Err, res.Value.Logs)
	}

	resp, err := common.ExtractAnchorTypedReturnValue[ccip_offramp.GetCcvsForMsgResponse](
		ctx, res.Value.Logs, offrampProgramID.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("parse get_ccvs_for_msg return value: %w", err)
	}
	return resp, nil
}

func pubkeysToUnknownAddresses(addrs []solana.PublicKey) []protocol.UnknownAddress {
	out := make([]protocol.UnknownAddress, len(addrs))
	for i, addr := range addrs {
		out[i] = protocol.UnknownAddress(addr.Bytes())
	}
	return out
}
