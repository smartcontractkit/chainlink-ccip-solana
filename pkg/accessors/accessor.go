package accessors

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/gagliardetto/solana-go"

	"github.com/smartcontractkit/chainlink-ccv/bootstrap"
	"github.com/smartcontractkit/chainlink-ccv/pkg/chainaccess"
	"github.com/smartcontractkit/chainlink-ccv/protocol"
	"github.com/smartcontractkit/chainlink-common/keystore"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"

	"github.com/smartcontractkit/chainlink-ccip-solana/pkg/destinationreader"
	"github.com/smartcontractkit/chainlink-ccip-solana/pkg/logpoller"
	"github.com/smartcontractkit/chainlink-ccip-solana/pkg/solanarpc"
	"github.com/smartcontractkit/chainlink-ccip-solana/pkg/transmitter"
)

var (
	_ chainaccess.Accessor     = (*accessor)(nil)
	_ bootstrap.KeystoreSetter = (*accessor)(nil)
)

// accessor wraps the concrete chain access components available for a Solana chain.
type accessor struct {
	sourceReader        chainaccess.SourceReader
	destinationReader   chainaccess.DestinationReader
	contractTransmitter chainaccess.ContractTransmitter
	sourceLogPoller     logpoller.Service

	// stored for lazy keystore transmitter construction via SetKeystore
	lggr             logger.Logger
	chainSelector    protocol.ChainSelector
	chainID          string
	offrampProgramID solana.PublicKey
	rpc              *solanarpc.Client
	keyName          string
	txConfig         transmitter.Config
	destConfigured   bool
	setupErr         error
	mu               sync.Mutex
	closed           bool
	closeOnce        sync.Once
	closeErr         error
}

func (a *accessor) SetKeystore(ctx context.Context, ks keystore.Keystore) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.closed {
		return errors.New("accessor is closed")
	}
	if a.contractTransmitter != nil {
		return errors.New("contract transmitter already set")
	}
	if !a.destConfigured {
		a.lggr.Debugw("skipping keystore contract transmitter setup for source-only chain",
			"chainSelector", a.chainSelector)
		return nil
	}
	if ks == nil || a.keyName == "" || a.rpc == nil || a.chainID == "" {
		a.lggr.Warnw("skipping keystore contract transmitter setup due to missing parameter",
			"chainSelector", a.chainSelector,
			"keyNameSet", a.keyName != "",
			"rpcSet", a.rpc != nil,
			"chainIDSet", a.chainID != "")
		return nil
	}

	ct, err := transmitter.NewSolanaContractTransmitter(ctx, transmitter.Params{
		Lggr:             a.lggr,
		ChainSelector:    a.chainSelector,
		ChainID:          a.chainID,
		OfframpProgramID: a.offrampProgramID,
		RPC:              a.rpc,
		Keystore:         ks,
		KeyName:          a.keyName,
		Config:           a.txConfig,
	})
	if err != nil {
		a.setupErr = err
		a.lggr.Errorw("Failed to create keystore contract transmitter", "chainSelector", a.chainSelector, "error", err)
		return fmt.Errorf("failed to create keystore contract transmitter for chain %d: %w", a.chainSelector, err)
	}
	a.contractTransmitter = ct

	// The destination reader simulates the offramp get_ccvs_for_msg instruction, which Solana
	// rejects with an unfunded fee payer. Reuse the funded transmitter key as that fee payer.
	if dr, ok := a.destinationReader.(*destinationreader.DestinationReader); ok {
		dr.SetSimulationFeePayer(ct.FeePayer())
	}
	return nil
}

func (a *accessor) SourceReader() (chainaccess.SourceReader, error) {
	if a.sourceReader == nil {
		return nil, errors.New("solana source reader is not configured")
	}
	return a.sourceReader, nil
}

func (a *accessor) DestinationReader() (chainaccess.DestinationReader, error) {
	if a.destinationReader == nil {
		return nil, errors.New("solana destination reader is not configured")
	}
	return a.destinationReader, nil
}

func (a *accessor) ContractTransmitter() (chainaccess.ContractTransmitter, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.setupErr != nil {
		return nil, fmt.Errorf("solana contract transmitter setup failed: %w", a.setupErr)
	}
	if a.contractTransmitter == nil {
		return nil, errors.New("solana contract transmitter is not configured")
	}
	return a.contractTransmitter, nil
}

func (a *accessor) FeeQuoterReader() (chainaccess.FeeQuoterReader, error) {
	return nil, errors.New("solana fee quoter reader is not configured")
}

func (a *accessor) FeeQuoterWriter() (chainaccess.FeeQuoterWriter, error) {
	return nil, errors.New("solana fee quoter writer is not configured")
}

func (a *accessor) GasPriceReader() (chainaccess.GasPriceReader, error) {
	return nil, errors.New("solana gas price reader is not configured")
}

func (a *accessor) Close() error {
	a.closeOnce.Do(func() {
		a.mu.Lock()
		a.closed = true
		ct := a.contractTransmitter
		a.mu.Unlock()

		var errs []error
		if a.sourceLogPoller != nil {
			if err := a.sourceLogPoller.Close(); err != nil {
				errs = append(errs, fmt.Errorf("close source logpoller: %w", err))
			}
		}
		// The Solana contract transmitter owns a chainlink-solana txm with background
		// goroutines that must be shut down. chainaccess.ContractTransmitter has no Close,
		// so close it best-effort via io.Closer.
		// TODO: speak to platform team about adding Close() to ContractTransmitter interface
		if closer, ok := ct.(io.Closer); ok {
			if err := closer.Close(); err != nil {
				errs = append(errs, fmt.Errorf("close contract transmitter: %w", err))
			}
		}

		if a.rpc != nil {
			if err := a.rpc.Close(); err != nil {
				errs = append(errs, fmt.Errorf("close solana rpc client: %w", err))
			}
		}
		a.closeErr = errors.Join(errs...)
	})
	return a.closeErr
}
