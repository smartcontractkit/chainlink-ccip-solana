package testutils

import (
	"fmt"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/testutils/eth"
)

// AttestCCTP produces a Circle-style attestation over a CCTP message: the concatenated 65-byte
// signatures of each attester, in R || S || V order.
//
// Test-only. In production the attestation comes from Circle's attestation service, never from
// locally-held attester keys — which is why this lives here rather than in contracts/utils/ccip.
// Based on https://github.com/circlefin/solana-cctp-contracts/blob/ec16e95d28ee47f7832df4203ae07b5981d146fc/programs/v2/tests/token_messenger_minter/test_client.ts#L255-L291
func AttestCCTP(message []byte, attesters []eth.Signer) ([]byte, error) {
	for i := range len(attesters) - 1 {
		if ethAddressSorter(attesters[i].Address, attesters[i+1].Address) >= 0 {
			return nil, fmt.Errorf("attesters are not sorted: %x >= %x", attesters[i].Address, attesters[i+1].Address)
		}
	}
	messageHash := eth.Keccak256(message)

	// Each signature is 65 bytes (32 bytes for R, 32 bytes for S, 1 byte for V)
	const signatureLength = 65
	attestation := make([]byte, len(attesters)*signatureLength)
	writeOffset := 0

	for _, attester := range attesters {
		if len(attester.PrivateKey) == 0 {
			return nil, fmt.Errorf("attester private key is empty for address %x", attester.Address)
		}
		privKey := secp256k1.PrivKeyFromBytes(attester.PrivateKey)

		// SignCompact returns a 65-byte compact signature: [V (1 byte) || R (32 bytes) || S (32 bytes)]
		// V is recoveryID + 27.
		compactSig := ecdsa.SignCompact(privKey, messageHash, false) // isCompressedKey = false

		// We need to store it as R || S || V in the attestation buffer (the V byte moved to the end)
		v := compactSig[0]
		rs := compactSig[1:]

		copy(attestation[writeOffset:], rs)
		writeOffset += signatureLength - 1 // 64 bytes for R and S
		attestation[writeOffset] = v
		writeOffset++
	}
	return attestation, nil
}

func ethAddressSorter(a, b [20]byte) int {
	for i := range 20 {
		if a[i] < b[i] {
			return -1
		} else if a[i] > b[i] {
			return 1
		}
	}
	return 0
}
