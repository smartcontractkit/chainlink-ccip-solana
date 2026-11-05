//go:build devnet
// +build devnet

package contracts

import (
	"crypto/ecdsa"
	_ "embed"
	"math/big"
	"strings"
	"testing"

	ethcommon "github.com/ethereum/go-ethereum/common"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

type SolanaValues struct {
	User                   solana.PrivateKey
	USDC                   solana.PublicKey
	MessageTransmitterV2   solana.PublicKey
	TokenMessengerMinterV2 solana.PublicKey
	Domain                 uint32 // source domain (5 = Solana devnet)
	Client                 *rpc.Client
}

type EVMValues struct {
	UserKey              *ecdsa.PrivateKey
	UserAddress          ethcommon.Address
	USDC                 ethcommon.Address
	MessageTransmitterV2 ethcommon.Address
	TokenMessengerV2     ethcommon.Address
	Domain               uint32 // destination domain (0 = Ethereum Sepolia)
	ChainID              *big.Int
	Client               *ethclient.Client
}

type DevnetValues struct {
	Solana     SolanaValues
	EVM        EVMValues
	IrisAPIURL string
}

func getDevnetValues(t *testing.T) DevnetValues {
	devnetInfo, err := getDevnetInfo()
	require.NoError(t, err)

	solUser := solana.PrivateKey(devnetInfo.PrivateKeys.Solana)
	require.True(t, solUser.IsValid(), "fill private_keys.solana in devnet.config.yaml (64-byte keypair array)")

	require.NotEmpty(t, devnetInfo.PrivateKeys.Sepolia, "fill private_keys.sepolia in devnet.config.yaml (hex string)")
	evmKey, err := ethcrypto.HexToECDSA(strings.TrimPrefix(devnetInfo.PrivateKeys.Sepolia, "0x"))
	require.NoError(t, err)

	ethClient, err := ethclient.Dial(devnetInfo.CCTP.Sepolia.RPC)
	require.NoError(t, err)

	return DevnetValues{
		Solana: SolanaValues{
			User:                   solUser,
			USDC:                   solana.MustPublicKeyFromBase58(devnetInfo.CCTP.UsdcMint),
			MessageTransmitterV2:   solana.MustPublicKeyFromBase58(devnetInfo.CCTP.MessageTransmitterV2),
			TokenMessengerMinterV2: solana.MustPublicKeyFromBase58(devnetInfo.CCTP.TokenMessengerMinterV2),
			Domain:                 devnetInfo.CCTP.SolanaDomain,
			Client:                 rpc.New(devnetInfo.RPC),
		},
		EVM: EVMValues{
			UserKey:              evmKey,
			UserAddress:          ethcrypto.PubkeyToAddress(evmKey.PublicKey),
			USDC:                 ethcommon.HexToAddress(devnetInfo.CCTP.Sepolia.USDC),
			MessageTransmitterV2: ethcommon.HexToAddress(devnetInfo.CCTP.Sepolia.MessageTransmitterV2),
			TokenMessengerV2:     ethcommon.HexToAddress(devnetInfo.CCTP.Sepolia.TokenMessengerV2),
			Domain:               devnetInfo.CCTP.Sepolia.Domain,
			ChainID:              big.NewInt(devnetInfo.CCTP.Sepolia.ChainID),
			Client:               ethClient,
		},
		IrisAPIURL: devnetInfo.CCTP.IrisAPIURL,
	}
}

//go:embed devnet.config.yaml
var devnetInfoBuffer []byte

type DevnetInfo struct {
	PrivateKeys struct {
		Solana  []byte `yaml:"solana"`
		Sepolia string `yaml:"sepolia"`
	} `yaml:"private_keys"`
	CCTP struct {
		UsdcMint               string `yaml:"usdc_mint"`
		MessageTransmitterV2   string `yaml:"message_transmitter_v2"`
		TokenMessengerMinterV2 string `yaml:"token_messenger_minter_v2"`
		SolanaDomain           uint32 `yaml:"solana_domain"`
		IrisAPIURL             string `yaml:"iris_api_url"`
		Sepolia                struct {
			Domain               uint32 `yaml:"domain"`
			RPC                  string `yaml:"rpc"`
			ChainID              int64  `yaml:"chain_id"`
			USDC                 string `yaml:"usdc"`
			MessageTransmitterV2 string `yaml:"message_transmitter_v2"`
			TokenMessengerV2     string `yaml:"token_messenger_v2"`
			RecipientBase58      string `yaml:"recipient_base58"`
			ReceiverAddress      string `yaml:"receiver_address"`
			AllowedCaller        string `yaml:"allowed_caller"`
		} `yaml:"sepolia"`
	} `yaml:"cctp"`
	RPC string `yaml:"rpc"`
}

func getDevnetInfo() (DevnetInfo, error) {
	var devnetInfo DevnetInfo
	if err := yaml.Unmarshal(devnetInfoBuffer, &devnetInfo); err != nil {
		return DevnetInfo{}, err
	}
	return devnetInfo, nil
}

type offrampPDAs struct {
	config             solana.PublicKey
	referenceAddresses solana.PublicKey
	billingSigner      solana.PublicKey
}

func getOfframpPDAs(offrampAddress solana.PublicKey) (offrampPDAs, error) {
	offrampReferenceAddressesPDA, _, err := state.FindOfframpReferenceAddressesPDA(offrampAddress)
	if err != nil {
		return offrampPDAs{}, err
	}

	offrampConfigPDA, _, err := state.FindOfframpConfigPDA(offrampAddress)
	if err != nil {
		return offrampPDAs{}, err
	}

	offrampBillingSignerPDA, _, err := state.FindOfframpBillingSignerPDA(offrampAddress)
	if err != nil {
		return offrampPDAs{}, err
	}

	return offrampPDAs{
		config:             offrampConfigPDA,
		referenceAddresses: offrampReferenceAddressesPDA,
		billingSigner:      offrampBillingSignerPDA,
	}, nil
}

type senderPDAs struct {
	program solana.PublicKey
	state   solana.PublicKey
}

func (s *senderPDAs) FindChainConfig(t *testing.T, chainSelector uint64) solana.PublicKey {
	chainSelectorLE := common.Uint64ToLE(chainSelector)
	pda, _, err := solana.FindProgramAddress([][]byte{[]byte("remote_chain_config"), chainSelectorLE}, s.program)
	require.NoError(t, err)
	return pda
}
