package utils

import (
	"context"
	"fmt"

	ccv "github.com/smartcontractkit/chainlink-ccv/build/devenv"
	devenvcommon "github.com/smartcontractkit/chainlink-ccv/build/devenv/common"
	"github.com/smartcontractkit/chainlink-ccv/build/devenv/tests/e2e/tcapi"
	"github.com/smartcontractkit/chainlink-ccv/protocol"
)

// ResolveSendAddressesWithReceiverQualifier resolves the receiver, committee CCV, and executor for
// a V3 send. Solana has no EOA concept, so the receiver comes from the AddressResolver, which picks
// the v1 or v2 example receiver by qualifier. tcapi.ResolveV3SendAddresses is not used because it
// always resolves the v2 receiver and cannot select v1.
func ResolveSendAddressesWithReceiverQualifier(
	ctx context.Context, lib ccv.Lib, src, dst uint64, receiverQualifier string,
) (protocol.UnknownAddress, []protocol.CCV, protocol.UnknownAddress, error) {
	env, ok := tcapi.LoadV3Env(ctx, lib, src, dst)
	if !ok {
		return protocol.UnknownAddress{}, nil, protocol.UnknownAddress{},
			fmt.Errorf("load V3 env for src %d dst %d", src, dst)
	}

	receiver, err := env.DstResolver.GetContractReceiver(env.DS, dst, receiverQualifier)
	if err != nil {
		return protocol.UnknownAddress{}, nil, protocol.UnknownAddress{},
			fmt.Errorf("resolve contract receiver: %w", err)
	}

	cv, err := tcapi.GetCommitteeCCV(env.SrcResolver, env.DS, src, devenvcommon.DefaultCommitteeVerifierQualifier)
	if err != nil {
		return protocol.UnknownAddress{}, nil, protocol.UnknownAddress{},
			fmt.Errorf("resolve committee CCV: %w", err)
	}

	executor, err := env.SrcResolver.GetExecutor(env.DS, src, devenvcommon.DefaultExecutorQualifier)
	if err != nil {
		return protocol.UnknownAddress{}, nil, protocol.UnknownAddress{},
			fmt.Errorf("resolve executor: %w", err)
	}

	return receiver, []protocol.CCV{cv}, executor, nil
}
