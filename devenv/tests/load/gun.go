package loadtest

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/rs/zerolog"

	"github.com/smartcontractkit/chainlink-ccv/build/devenv/cciptestinterfaces"
	"github.com/smartcontractkit/chainlink-ccv/build/devenv/tests/e2e/load"
	"github.com/smartcontractkit/chainlink-ccv/protocol"
	"github.com/smartcontractkit/chainlink-testing-framework/wasp"

	soldevenv "github.com/smartcontractkit/chainlink-ccip-solana/devenv"
)

const sentMessageChannelBufferSize = 1000

// Lane defines a source→destination pair for the gun to fire messages on.
type Lane struct {
	Src      uint64
	Dst      uint64
	Receiver protocol.UnknownAddress
}

// SolanaTXGun implements the wasp gun contract and load.LoadGun interface
// for Solana as source CCIP load tests.
type SolanaTXGun struct {
	sources             map[uint64]cciptestinterfaces.ChainAsSource
	v3Sources           map[uint64]cciptestinterfaces.MessageV3Source
	dests               map[uint64]cciptestinterfaces.MessageV3Destination
	signerSelector      map[uint64]func() solana.PrivateKey
	lanes               []Lane
	laneIndex           atomic.Uint32
	sentMsgCh           chan load.SentMessage
	closeOnce           sync.Once
	logger              zerolog.Logger
	executorArgsParams  any // optional, passed to BuildV3ExtraArgs
	tokenReceiverParams any // optional, passed to BuildV3ExtraArgs
	tokenArgsParams     any // optional, passed to BuildV3ExtraArgs
}

var _ load.LoadGun = (*SolanaTXGun)(nil)

// SolanaTXGunOption configures a SolanaTXGun at construction time.
type SolanaTXGunOption func(*SolanaTXGun)

// WithExecutorArgsParams sets the executorArgsParams passed to BuildV3ExtraArgs.
func WithExecutorArgsParams(params any) SolanaTXGunOption {
	return func(g *SolanaTXGun) {
		g.executorArgsParams = params
	}
}

// WithTokenReceiverParams sets the tokenReceiverParams passed to BuildV3ExtraArgs.
func WithTokenReceiverParams(params any) SolanaTXGunOption {
	return func(g *SolanaTXGun) {
		g.tokenReceiverParams = params
	}
}

// WithTokenArgsParams sets the tokenArgsParams passed to BuildV3ExtraArgs.
func WithTokenArgsParams(params any) SolanaTXGunOption {
	return func(g *SolanaTXGun) {
		g.tokenArgsParams = params
	}
}

// NewSolanaTXGun creates a SolanaTXGun configured for the given lanes.
func NewSolanaTXGun(
	logger zerolog.Logger,
	impls map[uint64]cciptestinterfaces.CCIP17,
	lanes []Lane,
	opts ...SolanaTXGunOption,
) (*SolanaTXGun, error) {
	if len(lanes) == 0 {
		return nil, errors.New("at least one lane is required")
	}

	sources := make(map[uint64]cciptestinterfaces.ChainAsSource)
	v3Sources := make(map[uint64]cciptestinterfaces.MessageV3Source)
	dests := make(map[uint64]cciptestinterfaces.MessageV3Destination)
	signerSelector := make(map[uint64]func() solana.PrivateKey)

	for _, lane := range lanes {
		if len(lane.Receiver) == 0 {
			return nil, fmt.Errorf("lane %d->%d: receiver address is required", lane.Src, lane.Dst)
		}
		// Multiple lanes can share a source; register signers and ChainAsSource once per src.
		if _, exists := signerSelector[lane.Src]; exists {
			continue
		}
		impl, ok := impls[lane.Src]
		if !ok {
			return nil, fmt.Errorf("source chain %d not found in impls", lane.Src)
		}
		chainAsSource, ok := impl.(cciptestinterfaces.ChainAsSource)
		if !ok {
			return nil, fmt.Errorf("chain %d does not implement ChainAsSource", lane.Src)
		}
		v3Source, ok := impl.(cciptestinterfaces.MessageV3Source)
		if !ok {
			return nil, fmt.Errorf("chain %d does not implement MessageV3Source", lane.Src)
		}
		solOpts, ok := impl.(soldevenv.SolanaOptions)
		if !ok {
			return nil, fmt.Errorf("chain %d does not implement SolanaOptions", lane.Src)
		}
		destImpl, ok := impls[lane.Dst]
		if !ok {
			return nil, fmt.Errorf("dest chain %d not found in impls", lane.Dst)
		}
		v3Dest, ok := destImpl.(cciptestinterfaces.MessageV3Destination)
		if !ok {
			return nil, fmt.Errorf("dest chain %d does not implement MessageV3Destination", lane.Dst)
		}
		sources[lane.Src] = chainAsSource
		v3Sources[lane.Src] = v3Source
		dests[lane.Dst] = v3Dest
		// Capture the round-robin closure once; Call() invokes it per message.
		signerSelector[lane.Src] = solOpts.GetRoundRobinUser()
	}

	g := &SolanaTXGun{
		sources:        sources,
		v3Sources:      v3Sources,
		dests:          dests,
		signerSelector: signerSelector,
		lanes:          lanes,
		sentMsgCh:      make(chan load.SentMessage, sentMessageChannelBufferSize),
		logger:         logger,
	}
	for _, opt := range opts {
		if opt == nil {
			opt(g)
		}
	}
	return g, nil
}

// Call fires one CCIP message on the next round-robin lane with the next round-robin signer.
func (g *SolanaTXGun) Call(_ *wasp.Generator) *wasp.Response {
	if len(g.lanes) == 0 {
		return &wasp.Response{Error: errors.New("no lanes configured").Error(), Failed: true}
	}

	laneIdx := int(g.laneIndex.Add(1) - 1)
	lane := g.lanes[laneIdx%len(g.lanes)]
	srcChain := g.sources[lane.Src]
	ctx := context.Background()

	// Rotate through the signer pool so concurrent Call()s use different keys.
	signer := g.signerSelector[lane.Src]()

	// Build V3 extra args with defaults resolved from the source chain's datastore.
	// then call BuildV3ExtraArgs to let the destination chain fill executor/token args.
	solChain, ok := g.sources[lane.Src].(interface {
		DefaultCCVAndExecutorPubKeys() (solana.PublicKey, solana.PublicKey, error)
	})
	if !ok {
		return &wasp.Response{Error: "source chain does not expose DefaultCCVAndExecutorPubKeys", Failed: true}
	}
	defaultCCV, defaultExecutor, err := solChain.DefaultCCVAndExecutorPubKeys()
	if err != nil {
		return &wasp.Response{Error: fmt.Errorf("resolve default CCV/executor: %w", err).Error(), Failed: true}
	}

	// Resolve gas limit from destination chain defaults (like EVM gun does at gun.go:321-324).
	opts := cciptestinterfaces.MessageOptions{
		ExecutionGasLimit: 0, // let dest chain fill via V3LoadMessageOptions
		CCVs: []protocol.CCV{{
			CCVAddress: defaultCCV.Bytes(),
			Args:       []byte{},
			ArgsLen:    0,
		}},
		Executor: defaultExecutor.Bytes(),
	}

	v3Source := g.v3Sources[lane.Src]
	v3Dest := g.dests[lane.Dst]

	if opts.ExecutionGasLimit == 0 {
		if def, ok := g.dests[lane.Dst].(cciptestinterfaces.V3DestinationLoadDefaults); ok {
			opts.ExecutionGasLimit = def.V3LoadMessageOptions().ExecutionGasLimit
		}
	}

	extraArgs, err := v3Source.BuildV3ExtraArgs(opts, v3Dest, g.executorArgsParams, g.tokenReceiverParams, g.tokenArgsParams)
	if err != nil {
		return &wasp.Response{Error: fmt.Errorf("build v3 extra args: %w", err).Error(), Failed: true}
	}

	msg, err := srcChain.BuildChainMessage(ctx, cciptestinterfaces.MessageFields{
		Receiver: lane.Receiver,
		Data:     nil,
	}, extraArgs)
	if err != nil {
		return &wasp.Response{Error: err.Error(), Failed: true}
	}

	// Capture before send: SendChainMessage blocks until the tx is confirmed on-chain.
	sentTime := time.Now()

	sentEvt, _, err := srcChain.SendChainMessage(ctx, lane.Dst, msg, soldevenv.SolanaSendOptions{
		Signer: &signer,
	})
	if err != nil {
		return &wasp.Response{Error: err.Error(), Failed: true}
	}

	g.sentMsgCh <- load.SentMessage{
		SeqNo:     uint64(sentEvt.Message.SequenceNumber),
		MessageID: sentEvt.MessageID,
		SentTime:  sentTime,
		ChainPair: load.SrcDest{Src: lane.Src, Dest: lane.Dst},
	}

	g.logger.Debug().
		Str("signer", signer.PublicKey().String()).
		Uint64("seqNo", uint64(sentEvt.Message.SequenceNumber)).
		Msg("gun fired")

	return &wasp.Response{Data: "ok"}
}

// CloseSentChannel closes the sent messages channel to signal no more messages will be sent.
// Safe to call multiple times.
func (g *SolanaTXGun) CloseSentChannel() {
	g.closeOnce.Do(func() {
		close(g.sentMsgCh)
	})
}

// SentMessages returns the channel of sent messages for the verification pipeline.
func (g *SolanaTXGun) SentMessages() <-chan load.SentMessage {
	return g.sentMsgCh
}
