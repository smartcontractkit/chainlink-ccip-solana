package ccip

import (
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/ccip_common"
)

func TestSvmExecutorExtraArgs(t *testing.T) {
	t.Parallel()

	privk1, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)
	privk2, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)

	pubkey1 := privk1.PublicKey()
	pubkey2 := privk2.PublicKey()

	original := SVMExecutorArgs{
		SvmDestBlob: ccip_common.SvmDestBlob{
			UseAta:                  ccip_common.SvmTokenReceiverUsage_UseAsIs,
			AccountIsWritableBitmap: 0b101,
			Accounts:                []solana.PublicKey{pubkey1, pubkey2},
		},
	}

	t.Run("BorshEncode and BorshDecode are inverses", func(t *testing.T) {
		t.Parallel()

		encoded, err := original.BorshEncode()
		require.NoError(t, err)

		var decoded SVMExecutorArgs
		require.NoError(t, decoded.BorshDecode(encoded))
		requireEqual(t, original, decoded)
	})

	t.Run("AbiEncodePacked and AbiDecodePacked are inverses", func(t *testing.T) {
		t.Parallel()

		encoded, err := original.AbiEncodePacked()
		require.NoError(t, err)

		var decoded SVMExecutorArgs
		require.NoError(t, decoded.AbiDecodePacked(encoded))
		requireEqual(t, original, decoded)
	})
}

func requireEqual(t *testing.T, expected, actual SVMExecutorArgs) {
	t.Helper()
	require.Equal(t, expected.UseAta, actual.UseAta)
	require.Equal(t, expected.AccountIsWritableBitmap, actual.AccountIsWritableBitmap)
	require.Len(t, actual.Accounts, len(expected.Accounts))
	for i := range expected.Accounts {
		require.Equal(t, expected.Accounts[i], actual.Accounts[i])
	}
}

func TestValidateDestBlob(t *testing.T) {
	t.Parallel()

	for _, n := range []int{0, 1, 64} {
		args := SVMExecutorArgs{SvmDestBlob: ccip_common.SvmDestBlob{
			UseAta:   ccip_common.SvmTokenReceiverUsage_UseAsIs,
			Accounts: make([]solana.PublicKey, n),
		}}
		borsh, err := args.BorshEncode()
		require.NoError(t, err)
		got, err := ValidateDestBlob(borsh)
		require.NoError(t, err, "borsh, %d accounts", n)
		require.Equal(t, n, got)
		abi, err := args.AbiEncodePacked()
		require.NoError(t, err)
		got, err = ValidateDestBlob(abi)
		require.NoError(t, err, "abi, %d accounts", n)
		require.Equal(t, n, got)
	}

	valid, err := (&SVMExecutorArgs{}).BorshEncode()
	require.NoError(t, err)
	hugeCount := append([]byte(nil), valid...)
	copy(hugeCount[13:], []byte{0xff, 0xff, 0xff, 0xff})
	badUseAta := append([]byte(nil), valid...)
	badUseAta[4] = 3

	for name, blob := range map[string][]byte{
		"empty":           nil,
		"short":           valid[:13],
		"borsh short":     append(valid[:14:14], 0),
		"unknown tag":     append([]byte{0, 0, 0, 0}, valid[4:]...),
		"trailing bytes":  append(append([]byte(nil), valid...), make([]byte, 4096)...),
		"huge count":      hugeCount,
		"bad use_ata":     badUseAta,
		"abi short body":  {0x1a, 0x2b, 0x3c, 0x4d, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1},
		"abi bad use_ata": {0x1a, 0x2b, 0x3c, 0x4d, 3, 0, 0, 0, 0, 0, 0, 0, 0, 0},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := ValidateDestBlob(blob)
			require.Error(t, err)
		})
	}
}
