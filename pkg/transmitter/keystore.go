package transmitter

import (
	"context"
	"fmt"

	"github.com/gagliardetto/solana-go"

	"github.com/smartcontractkit/chainlink-common/keystore"
	"github.com/smartcontractkit/chainlink-common/pkg/types/core"
	solkeys "github.com/smartcontractkit/chainlink-solana/pkg/solana/keys"
)

// DefaultKeyName is the full keystore path of the Ed25519 key the executor uses to
// sign and submit OffRamp transactions on Solana chains. The solana/tx/ prefix follows
// the solkeys convention so that TxKey can use it directly with WithNoPrefix(). The
// bootstrap framework creates or loads it on startup, devenv uses it to look up the
// funded on-chain address after the container is running.
//
// Accessors default to this key name unless a per-chain transmitter_key_name override
// is configured.
const DefaultKeyName = "solana/tx/executor_solana_transmitter_key"

// loadTransmitterKey resolves the executor transmitter key from the bootstrap keystore and
// returns the fee-payer public key and txm core.Keystore adapter.
func loadTransmitterKey(ctx context.Context, ks keystore.Keystore, keyName string) (solana.PublicKey, core.Keystore, error) {
	txKeys, err := solkeys.GetTxKeys(ctx, ks, []string{keyName}, solkeys.WithNoPrefix())
	if err != nil {
		return solana.PublicKey{}, nil, fmt.Errorf("get transmitter tx keys: %w", err)
	}
	if len(txKeys) != 1 {
		return solana.PublicKey{}, nil, fmt.Errorf("expected 1 transmitter key, got %d", len(txKeys))
	}

	txKey := txKeys[0]
	keyPath := txKey.KeyPath().String()
	pubkey := txKey.Address()
	coreKs := solkeys.NewTxKeyCoreKeystore(ks, solkeys.WithAllowedKeyNames([]string{keyPath}))
	return pubkey, coreKs, nil
}
