package testutils

import (
	"bytes"
	"context"
	"math/big"
	"sort"
	"testing"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"

	burnmint_tokenpool "github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/burnmint_token_pool"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/ccip_offramp"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/ccip_router"
	cctp_message_transmitter_v2 "github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/cctp_message_transmitter_v2"
	cctp_token_messenger_minter_v2 "github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/cctp_token_messenger_minter_v2"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/cctp_token_pool"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/committee_verifier"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/executor"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/fee_quoter"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/rmn_remote"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/test_ccip_receiver"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/cctp"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/config"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/testutils/eth"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/ccip"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/tokens"
)

// ----- Accounts ----- //

// Shared test accounts. Deterministic keys so PDAs derived from them are stable across tests.
var (
	User           = solana.MustPrivateKeyFromBase58("ZZdVf32Npuhci4u4ir2NW9491Y3FTv2Gwk41HMpvgJoh81UM42LcNqAN8SXapHfPcr61QP7sJj7K2mKHt7qFCoV")
	AnotherUser    = solana.MustPrivateKeyFromBase58("i9btAVgpmReUv9jH52xpPoYvtsv6XQSJrRGnLpTU4ArSP6E3Xa9aunyeT7n83QhMeLZMmRPnwY41xr7jFrTXAPR")
	TokenlessUser  = solana.MustPrivateKeyFromBase58("4g8xCc96ox2ksCcv5VggqaenkSsnkUEe8WZBazLyTCMFnB2r2bJtdNK2QW9E6mojMmMGpGcSGuKFeDQbGLiCqM3n")
	Admin          = solana.MustPrivateKeyFromBase58("5dYf8bzhFbmwTNyq8vAoeqe19vdjCbSEKkPrN6DQoJB2RwjyuVapCmU46pEBvbw7aeg5wBncTc8HQJnKZy2LYsmF")
	CcipAdmin      = solana.MustPrivateKeyFromBase58("AmNCLrssMCEGUM5RWDao2v4J6ydB48FZBDye6x1o5Z1KR7QkZvuMgX185fZoESBotBtBZ2SiQVkoNwjmUD8ysge")
	Token0Admin    = solana.MustPrivateKeyFromBase58("2NqkEEvMWf5Y8aUTSZvGCfyqPi4KjBKJShXWH4BrTWVyfxBzJm22S1K4gtkgzHcAhStseHypRV7mKPsx5nVa9h2e")
	Token1Admin    = solana.MustPrivateKeyFromBase58("rAJULkVqwXHED22STrDdUPxfGqSi6gkHfSpgaYSTzp3X9MCqsYegEcWMJVZ5yQFw5H3mNdsBAJpR9xfdYRGfb7J")
	Token2Admin    = solana.MustPrivateKeyFromBase58("3UUqZ5xa3xv9fX1UJQyHsJtovE2gzmJUJjybizjrAvxh7NUmVyVQHUkJVkQwBKtVr5vLVCp1DWAeAzv46WzLoEmS")
	UsdcPoolAdmin  = solana.MustPrivateKeyFromBase58("2Gjx3b1ZY7P3A63EnPRSVWQsKth8BXXjQQNLjzAsiLFrDxbBQwZ62yDG8BiN5RjG8VYveMdZFy7gLjzsdKboPM7N")
	FeeAggregator  = solana.MustPrivateKeyFromBase58("NPchsbT3bkkziJPBUxto3eVTVKHcV4tou33NjRY8inArmzi7EXKBf5cC7MX47xqYMwkZGbkw7t55jCciCStSwNs")
	Token0Multisig = solana.MustPrivateKeyFromBase58("5BayUa1C1nfiSptuV521hYPKZZsjHJM5YDfWZJkyrDgnEhzvZS4x5Nuqn6n4E6anuqAc7dJpAr1faNUwyK99cf3C")

	CctpAttester = mustGetEthSignerFromPk("124a3c092690091303c28d3b387274396f8c9382949eeaf2f81fdacfff7051df") // Address 0xF035B5356C3ECAd540F34992762CfBAf57641e03

	// Deterministic mint keypairs for test tokens.
	Token0MintPrivK = solana.MustPrivateKeyFromBase58("42uJJqZk4gFz6Q6ghMiaYrFdDapXhbufQdTCGJDMeyv2wN6wNBbXkBBPibF7xQQZemzRaDH66ouJmjfvWhPJKtQC")
	Token1MintPrivK = solana.MustPrivateKeyFromBase58("5uBhsiup2KiXPNznVSZaFeuegvLXCtmVmuW4u7kfBbP3xSmT1BZgmapKYiCCwi8GUgwdSQniAah3rJaUJdhPprcB")
	Token2MintPrivK = solana.MustPrivateKeyFromBase58("2b4zgrXRBDkAuhFMEUEaMJHXwPfPX5REmy3gA3Vxhj7efTLkZEBuq49mDSdQyCJFyG3KPRGt2PVoGF8VqhGUTo9")
	LinkMintPrivK   = solana.MustPrivateKeyFromBase58("32YVeJArcWWWV96fztfkRQhohyFz5Hwno93AeGVrN4g2LuFyvwznrNd9A6tbvaTU6BuyBsynwJEMLre8vSy3CrVU")
	UsdcMintPrivK   = solana.MustPrivateKeyFromBase58("3NnpbE8mrtqhC99YLxhZU4xQTBQBha6tKjfTa123DtR6bmGH1NQS59QELnNarvgHzzvRjAdDTqsfGYjyWHf6mwtA")
)

func mustGetEthSignerFromPk(pk string) eth.Signer {
	signer, err := eth.GetSignerFromPk(pk)
	if err != nil {
		panic("failed to get eth signer from pk: " + err.Error())
	}
	return signer
}

// ----- V2 lane defaults ----- //

func pow10(n int64) *big.Int { return new(big.Int).Exp(big.NewInt(10), big.NewInt(n), nil) }

// Baseline configuration of the CCIP v2.0 EVM lane. Fee-related tests assert exact receipt
// values derived from these, so keep them in sync with any assertion helpers.
var (
	// WSOL and LINK22 billing prices, as 18-decimals USD for 1e18 smallest token denominations
	// (see setupBillingTokenConfigs). For a token with `d` decimals priced at $P per whole token
	// that is `P * 1e(36-d)`; both test mints have 9 decimals, so ~$19.82/WSOL is 1.981668e28.
	//
	// Getting the scale wrong is not caught by the 1.6 assertions, which only compare a send
	// against the fee quoter's own get_fee, but the 2.0 flat fees are charged straight off this
	// price: a value dimensioned for an 18-decimal token makes a $0.50 network fee cost 2.5e17
	// lamports. Keep it paired with PremiumMultiplierWeiPerEth below, which scales the 1.6 fee.
	WsolUsdPrice   = new(big.Int).Mul(big.NewInt(1981668), pow10(22)) // ~$19.82 per WSOL
	Link22UsdPrice = new(big.Int).Mul(big.NewInt(1981668), pow10(23)) // ~$198.17 per LINK22

	// An EVM-style 20-byte off-ramp address. The router requires a non-empty off-ramp on the
	// lane: its length determines the destination address width used for fee accounting.
	V2LaneOfframpAddress = []byte{
		0xde, 0xad, 0xbe, 0xef, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06,
		0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
	}
	V2LaneMessageNetworkFeeUsdCents       = uint16(50)  // $0.50 per plain message
	V2LaneTokenTransferNetworkFeeUsdCents = uint16(150) // $1.50 per token transfer
	V2LaneBaseExecutionGasCost            = uint32(100_000)
	// Per-message fee cap (EVM's maxFeeJuelsPerMsg equivalent, configured per lane here). The
	// router rejects any message whose total receipt sum exceeds it, so the baseline must be
	// above every fee the tests quote.
	V2LaneMaxFeePerMessageUsdCents = uint16(1000) // $10.00 per message

	// Gas price of the EVM and SVM lanes, as 18-decimals USD per single unit of destination gas. It lives
	// on the 1.6 dest chain state, which is what both the 1.6 fee path and quote_gas_for_exec_v2
	// read. quote_gas_for_exec_v2 rejects a lane whose price was never set (a zero timestamp), so
	// the baseline has to write one before any 2.0 fee can be quoted; see SetLaneGasPrice.
	//
	// 1e10 is $1e-8 per gas unit (an L2-like ~0.5 gwei at $20 per gas token), chosen so that the
	// execution cost rounds up to exactly V2LaneExecutionGasCostUsdCents for every message the
	// 2.0 suite quotes: the cost is rounded up to the cent, and the largest of those messages
	// stays well under the 1e6 total gas where a second cent would start being charged.
	LaneUsdPerUnitGas = pow10(10)
	// Execution cost of every 2.0 message in the suite, in cents; see LaneUsdPerUnitGas. The
	// fee quoter bills it on top of the executor's own fee, so it lands in the executor receipt.
	V2LaneExecutionGasCostUsdCents = uint16(1)
)

// DefaultV2DestChainConfig returns the baseline CCIP v2.0 lane config used for the EVM
// selector. Tests that override the lane config should start from this value and defer a
// restore to it, so later subtests keep seeing the default lane.
func DefaultV2DestChainConfig() ccip_router.DestChainConfigCcipV2 {
	return ccip_router.DestChainConfigCcipV2{
		// The router enforces (like EVM `_validateDefaultAndMandatedCCVs`) that every lane has
		// at least one default or lane-mandated CCV.
		DefaultCcvs:             []solana.PublicKey{config.CommitteeVerifierProgram},
		Offramp:                 V2LaneOfframpAddress, // this is 20 bytes
		AddressBytesLength:      20,                   // so setting this to 20 bytes
		MessageNetworkFee:       ccip_router.UsdCents{V0: uint32(V2LaneMessageNetworkFeeUsdCents)},
		TokenTransferNetworkFee: ccip_router.UsdCents{V0: uint32(V2LaneTokenTransferNetworkFeeUsdCents)},
		BaseExecutionGasCost:    ccip_router.CrossChainGas{V0: V2LaneBaseExecutionGasCost},
		MaxFeePerMessage:        ccip_router.UsdCents{V0: uint32(V2LaneMaxFeePerMessageUsdCents)},
		// The onramp CPIs the default executor's get_fee on every send that doesn't request a
		// specific executor, so the default must be the real executor program (see setupExecutor).
		DefaultExecutor:      config.ExecutorProgram,
		TokenReceiverAllowed: true, // In SVM is always true
	}
}

// ----- Billing Token State ----- //

// BillingTokenState holds all accounts for a billing token (WSOL or LINK22).
type BillingTokenState struct {
	Name             string
	Program          solana.PublicKey
	Mint             solana.PublicKey
	BillingATA       solana.PublicKey
	UserATA          solana.PublicKey
	AnotherUserATA   solana.PublicKey
	TokenlessUserATA solana.PublicKey
	FeeAggregatorATA solana.PublicKey
	FqBillingCfgPDA  solana.PublicKey
	FqEvmCfgPDA      solana.PublicKey

	// 2.0 fee-vault ATAs of the receipt issuers, where ccip_send_v2 pays out each issuer's fee
	// share. Keyed by (issuer program, fee token mint), so a single burn-mint pool vault ATA
	// covers every pool running on that program regardless of the transferred token. The
	// onramp's own vault (where network and legacy-pool fees remain) is BillingATA.
	ExecutorVaultATA     solana.PublicKey
	CVVaultATA           solana.PublicKey
	BurnMintPoolVaultATA solana.PublicKey
	UsdcPoolVaultATA     solana.PublicKey
	CctpVerifierVaultATA solana.PublicKey
}

// ----- TestEnv ----- //

// TestEnv is the environment produced by the builder. Every field that is
// populated depends on which With*() options were selected.
type TestEnv struct {
	T      *testing.T
	Ctx    func() context.Context // t.Context() - use Ctx() to get it
	Client *rpc.Client
	RPCURL string // the endpoint of Client

	// Billing tokens (always populated after Build)
	WSOL   BillingTokenState
	Link22 BillingTokenState

	// Populated by WithTokens()
	Token0   tokens.TokenPool
	Token1   tokens.TokenPool
	Token2   tokens.TokenPool
	LinkPool tokens.TokenPool
	UsdcPool tokens.TokenPool

	// Populated by WithOCR()
	Signers            []eth.Signer
	Transmitters       []solana.PrivateKey
	GetTransmitter     func() solana.PrivateKey
	SignerAddresses    [][20]byte
	TransmitterPubKeys []solana.PublicKey

	// Populated by WithLookupTables()
	CcipSendLookupTable     map[solana.PublicKey]solana.PublicKeySlice
	CcipSendLookupTableAddr solana.PublicKey
	OfframpLookupTable      map[solana.PublicKey]solana.PublicKeySlice
	OfframpLookupTableAddr  solana.PublicKey

	// Nonce PDAs (always populated)
	NonceEvmPDA solana.PublicKey
	NonceSvmPDA solana.PublicKey

	// Valid defaults for tests
	OnRampAddress              [64]byte
	ValidSourceChainConfig     ccip_offramp.SourceChainConfig
	ValidFqDestChainConfig     fee_quoter.DestChainConfig
	ValidFqDestChainConfigV2   fee_quoter.DestChainConfigV2
	ValidEvmRawReceiverAddress [20]byte
	ValidEvmAbiReceiverAddress [32]byte
	ValidSvmReceiverAddress    [32]byte

	// CCTP (populated by WithTokens())
	CctpSolDomain          uint32
	CctpEvmDomain          uint32
	MessageTransmitter     cctp.MessageTransmitterPDAs
	TokenMessengerMinter   cctp.TokenMessengerMinterPDAs
	MessageTransmitterV2   cctp.MessageTransmitterV2PDAs
	TokenMessengerMinterV2 cctp.TokenMessengerMinterV2PDAs

	// CCTPv2, (populated by WithCCTPv2())
	CctpAttesters []eth.Signer
}

// ----- Builder ----- //

type TestEnvBuilder struct {
	t *testing.T

	withTokens        bool
	withPools         bool
	withOCR           bool
	withBilling       bool
	withLookupTables  bool
	withRMN           bool
	withCV            bool
	withReceiver      bool
	withTrafficConfig bool
	withCCTPv2        bool
}

func NewTestEnv(t *testing.T) *TestEnvBuilder {
	return &TestEnvBuilder{t: t}
}

func (b *TestEnvBuilder) WithTokens() *TestEnvBuilder {
	b.withTokens = true
	return b
}

func (b *TestEnvBuilder) WithPools() *TestEnvBuilder {
	b.withPools = true
	b.withTokens = true // pools imply tokens
	return b
}

func (b *TestEnvBuilder) WithOCR() *TestEnvBuilder {
	b.withOCR = true
	return b
}

func (b *TestEnvBuilder) WithBilling() *TestEnvBuilder {
	b.withBilling = true
	return b
}

func (b *TestEnvBuilder) WithLookupTables() *TestEnvBuilder {
	b.withLookupTables = true
	return b
}

func (b *TestEnvBuilder) WithRMN() *TestEnvBuilder {
	b.withRMN = true
	return b
}

func (b *TestEnvBuilder) WithCV() *TestEnvBuilder {
	b.withCV = true
	return b
}

func (b *TestEnvBuilder) WithReceiver() *TestEnvBuilder {
	b.withReceiver = true
	return b
}

func (b *TestEnvBuilder) WithCCTPv2() *TestEnvBuilder {
	b.withCCTPv2 = true
	return b
}

// WithTrafficConfig enables the full traffic configuration:
// Router + FeeQuoter + Offramp initialization, chain selectors, ownership transfer,
// billing token configs, token admin registry, per-chain pool config, and OCR config.
// Implies WithPools, WithOCR, WithBilling, WithLookupTables, WithRMN, WithCV, WithReceiver.
func (b *TestEnvBuilder) WithTrafficConfig() *TestEnvBuilder {
	b.withTrafficConfig = true
	b.withPools = true
	b.withTokens = true
	b.withOCR = true
	b.withBilling = true
	b.withLookupTables = true
	b.withRMN = true
	b.withCV = true
	b.withReceiver = true
	return b
}

func (b *TestEnvBuilder) Build() *TestEnv {
	t := b.t
	t.Helper()
	ctx := t.Context()

	// Deploy all programs and create RPC client
	rpcURL := startValidator(t, PathToAnchorConfig, Admin)
	client := rpc.New(rpcURL)

	nonceEvmPDA, err := state.FindNoncePDA(config.EvmChainSelector, User.PublicKey(), config.CcipRouterProgram)
	require.NoError(t, err)
	nonceSvmPDA, err := state.FindNoncePDA(config.SvmChainSelector, User.PublicKey(), config.CcipRouterProgram)
	require.NoError(t, err)

	env := &TestEnv{
		T:      t,
		Ctx:    t.Context,
		Client: client,
		RPCURL: rpcURL,

		NonceEvmPDA: nonceEvmPDA,
		NonceSvmPDA: nonceSvmPDA,
	}

	// Valid defaults
	env.OnRampAddress = [64]byte{ // matches what was used in MinimalMessageV1
		0xaa, 0xbb, 0xcc, 0xdd,
		0xee, 0xff, 0x00, 0x11,
		0x22, 0x33, 0x44, 0x55,
		0x66, 0x77, 0x88, 0x99,
		0xaa, 0xbb, 0xcc, 0xdd,
		0xee, 0xff, 0x00, 0x11,
		0x22, 0x33, 0x44, 0x55,
		0x66, 0x77, 0x88, 0x99,
	}
	env.ValidSourceChainConfig = ccip_offramp.SourceChainConfig{
		OnRamps: []ccip_offramp.OnRampAddress{{
			Bytes: env.OnRampAddress,
			Len:   32,
		}},
		IsEnabled:           true,
		DefaultCcvs:         []solana.PublicKey{config.CommitteeVerifierProgram},
		LaneMandatedCcvs:    []solana.PublicKey{},
		LegacyAddressLength: 20, // default value for EVM chains - compatibility with 1.6 pools that work across lane versions
	}
	env.ValidFqDestChainConfig = fee_quoter.DestChainConfig{
		IsEnabled:                   true,
		LaneCodeVersion:             fee_quoter.CodeVersion_Default,
		DefaultTxGasLimit:           fee_quoter.CrossChainGas{V0: 200000},
		MaxPerMsgGasLimit:           fee_quoter.CrossChainGas{V0: 3000000},
		MaxDataBytes:                30000,
		MaxNumberOfTokensPerMsg:     5,
		DefaultTokenDestGasOverhead: fee_quoter.CrossChainGas{V0: 50000},
		ChainFamilySelector:         [4]uint8(config.EvmChainFamilySelector),
		DefaultTokenFeeUsdcents:     50,
		NetworkFee:                  fee_quoter.UsdCents{V0: 50},
	}
	// CCIP 2.0 instructions read the migrated `dest_chain_v2` account, not the 1.6 one. The
	// token-fee defaults must mirror ValidFqDestChainConfig above: the 2.0 fallback token fee
	// view (get_fallback_token_transfer_fee_v2) serves them for tokens without a per-chain
	// override, and the onramp fee assertions are written against those same values.
	env.ValidFqDestChainConfigV2 = fee_quoter.DestChainConfigV2{
		IsEnabled:                   true,
		LaneCodeVersion:             fee_quoter.CodeVersion_Default,
		DefaultTxGasLimit:           fee_quoter.CrossChainGas{V0: 200000},
		MaxPerMsgGasLimit:           fee_quoter.CrossChainGas{V0: 3000000},
		MaxDataBytes:                30000,
		DestGasOverhead:             fee_quoter.CrossChainGas{V0: 100000},
		DestGasPerPayloadByteBase:   16,
		DefaultTokenDestGasOverhead: fee_quoter.CrossChainGas{V0: 50000},
		ChainFamilySelector:         [4]uint8(config.EvmChainFamilySelector),
		DefaultTokenFeeUsdCents:     50,
		NetworkFee:                  fee_quoter.UsdCents{V0: 50},
		LinkFeeMultiplierPercent:    90,
	}
	env.ValidEvmRawReceiverAddress = [20]byte{}
	env.ValidEvmRawReceiverAddress[0] = 1

	env.ValidEvmAbiReceiverAddress = [32]byte{}
	env.ValidEvmAbiReceiverAddress[12] = 1

	env.ValidSvmReceiverAddress = [32]byte{}
	env.ValidSvmReceiverAddress[0] = 1

	// Fund accounts
	allAccounts := []solana.PrivateKey{
		User, AnotherUser, TokenlessUser,
		Admin, CcipAdmin,
		Token0Admin, Token1Admin, Token2Admin,
		UsdcPoolAdmin, FeeAggregator,
	}

	if b.withOCR {
		signers, transmitters, getTransmitter := GenerateSignersAndTransmitters(t, config.MaxOracles)
		env.Signers = signers
		env.Transmitters = transmitters
		env.GetTransmitter = getTransmitter

		signerAddresses := make([][20]byte, 0, len(signers))
		transmitterPubKeys := make([]solana.PublicKey, 0, len(transmitters))
		for _, v := range signers {
			signerAddresses = append(signerAddresses, v.Address)
		}
		for _, v := range transmitters {
			transmitterPubKeys = append(transmitterPubKeys, v.PublicKey())
		}
		sort.SliceStable(signerAddresses, func(i, j int) bool {
			return bytes.Compare(signerAddresses[i][:], signerAddresses[j][:]) == 1
		})
		sort.SliceStable(transmitterPubKeys, func(i, j int) bool {
			return bytes.Compare(transmitterPubKeys[i].Bytes(), transmitterPubKeys[j].Bytes()) == 1
		})
		env.SignerAddresses = signerAddresses
		env.TransmitterPubKeys = transmitterPubKeys

		allAccounts = append(allAccounts, transmitters...)
	}

	require.NoError(t, FundAccounts(ctx, allAccounts, client))

	// Setup billing tokens (always needed)
	env.setupBillingTokens(ctx)

	if b.withTokens {
		env.setupTokens(ctx)
	}

	if b.withPools {
		env.setupPools(ctx)
	}

	if b.withReceiver {
		env.setupReceiver(ctx)
	}

	if b.withBilling {
		env.setupBillingFundingAndApprovals(ctx)
	}

	if b.withLookupTables {
		env.setupLookupTables(ctx)
	}

	if b.withRMN {
		env.setupRMN(ctx)
	}

	if b.withCV {
		env.setupCV(ctx)
	}

	if b.withCCTPv2 {
		env.setupCCTPv2(ctx)
		env.setupCCTPPool(ctx)
		env.setupCCTPVerifier(ctx)
	}

	if b.withTrafficConfig {
		env.setupTrafficConfig(ctx, b.withCCTPv2)
	}

	return env
}

// ----- Setup helpers ----- //

func (env *TestEnv) setupBillingTokens(ctx context.Context) {
	t := env.T
	t.Helper()

	// WSOL
	wsolPDA, _, err := state.FindFqBillingTokenConfigPDA(solana.WrappedSol, config.FeeQuoterProgram)
	require.NoError(t, err)
	wsolReceiver, _, err := tokens.FindAssociatedTokenAddress(config.SPLTokenProgram, solana.WrappedSol, config.BillingSignerPDA)
	require.NoError(t, err)
	wsolEvmCfg, _, err := state.FindFqPerChainPerTokenConfigPDA(config.EvmChainSelector, solana.WrappedSol, config.FeeQuoterProgram)
	require.NoError(t, err)
	wsolUserATA, _, err := tokens.FindAssociatedTokenAddress(config.SPLTokenProgram, solana.WrappedSol, User.PublicKey())
	require.NoError(t, err)
	wsolAnotherUserATA, _, err := tokens.FindAssociatedTokenAddress(config.SPLTokenProgram, solana.WrappedSol, AnotherUser.PublicKey())
	require.NoError(t, err)
	wsolTokenlessUserATA, _, err := tokens.FindAssociatedTokenAddress(config.SPLTokenProgram, solana.WrappedSol, TokenlessUser.PublicKey())
	require.NoError(t, err)
	wsolFeeAggregatorATA, _, err := tokens.FindAssociatedTokenAddress(config.SPLTokenProgram, solana.WrappedSol, FeeAggregator.PublicKey())
	require.NoError(t, err)

	// 2.0
	wsolExecutorVaultATA, err := tokens.FindFeeVaultATA(config.SPLTokenProgram, solana.WrappedSol, config.ExecutorProgram, nil)
	require.NoError(t, err)
	wsolCVVaultATA, err := tokens.FindFeeVaultATA(config.SPLTokenProgram, solana.WrappedSol, config.CommitteeVerifierProgram, nil)
	require.NoError(t, err)
	// Pool fee vaults are qualified by the mint of the managed token; Token1 is the token the
	// onramp suite transfers through the burn-mint pool.
	wsolBurnMintPoolVaultATA, err := tokens.FindFeeVaultATA(config.SPLTokenProgram, solana.WrappedSol, config.CcipBurnMintTokenPoolProgram, Token1MintPrivK.PublicKey().Bytes())
	require.NoError(t, err)
	wsolUsdcPoolVaultATA, err := tokens.FindFeeVaultATA(config.SPLTokenProgram, solana.WrappedSol, config.CctpTokenPoolProgram, UsdcMintPrivK.PublicKey().Bytes())
	require.NoError(t, err)
	wsolCctpVerifierVaultATA, err := tokens.FindFeeVaultATA(config.SPLTokenProgram, solana.WrappedSol, config.CctpTokenPoolProgram, nil)
	require.NoError(t, err)

	env.WSOL = BillingTokenState{
		Name:             "WSOL (pre-2022)",
		Program:          config.SPLTokenProgram,
		Mint:             solana.WrappedSol,
		FqBillingCfgPDA:  wsolPDA,
		BillingATA:       wsolReceiver,
		FqEvmCfgPDA:      wsolEvmCfg,
		UserATA:          wsolUserATA,
		AnotherUserATA:   wsolAnotherUserATA,
		TokenlessUserATA: wsolTokenlessUserATA,
		FeeAggregatorATA: wsolFeeAggregatorATA,

		// 2.0
		ExecutorVaultATA:     wsolExecutorVaultATA,
		CVVaultATA:           wsolCVVaultATA,
		BurnMintPoolVaultATA: wsolBurnMintPoolVaultATA,
		UsdcPoolVaultATA:     wsolUsdcPoolVaultATA,
		CctpVerifierVaultATA: wsolCctpVerifierVaultATA,
	}

	// LINK22
	linkMint := LinkMintPrivK.PublicKey()
	link22Decimals := uint8(9)
	ixToken, err := tokens.CreateToken(ctx, config.Token2022Program, linkMint, Admin.PublicKey(), link22Decimals, env.Client, config.DefaultCommitment)
	require.NoError(t, err)
	SendAndConfirm(ctx, t, env.Client, ixToken, Admin, config.DefaultCommitment, common.AddSigners(LinkMintPrivK))

	link22PDA, _, err := state.FindFqBillingTokenConfigPDA(linkMint, config.FeeQuoterProgram)
	require.NoError(t, err)
	link22EvmCfg, _, err := state.FindFqPerChainPerTokenConfigPDA(config.EvmChainSelector, linkMint, config.FeeQuoterProgram)
	require.NoError(t, err)
	link22Receiver, _, err := tokens.FindAssociatedTokenAddress(config.Token2022Program, linkMint, config.BillingSignerPDA)
	require.NoError(t, err)
	link22UserATA, _, err := tokens.FindAssociatedTokenAddress(config.Token2022Program, linkMint, User.PublicKey())
	require.NoError(t, err)
	link22AnotherUserATA, _, err := tokens.FindAssociatedTokenAddress(config.Token2022Program, linkMint, AnotherUser.PublicKey())
	require.NoError(t, err)
	link22TokenlessUserATA, _, err := tokens.FindAssociatedTokenAddress(config.Token2022Program, linkMint, TokenlessUser.PublicKey())
	require.NoError(t, err)
	link22FeeAggregatorATA, _, err := tokens.FindAssociatedTokenAddress(config.Token2022Program, linkMint, FeeAggregator.PublicKey())
	require.NoError(t, err)

	// 2.0
	link22ExecutorVaultATA, err := tokens.FindFeeVaultATA(config.Token2022Program, linkMint, config.ExecutorProgram, nil)
	require.NoError(t, err)
	link22CVVaultATA, err := tokens.FindFeeVaultATA(config.Token2022Program, linkMint, config.CommitteeVerifierProgram, nil)
	require.NoError(t, err)
	link22BurnMintPoolVaultATA, err := tokens.FindFeeVaultATA(config.Token2022Program, linkMint, config.CcipBurnMintTokenPoolProgram, Token1MintPrivK.PublicKey().Bytes())
	require.NoError(t, err)
	link22UsdcPoolVaultATA, err := tokens.FindFeeVaultATA(config.Token2022Program, linkMint, config.CctpTokenPoolProgram, UsdcMintPrivK.PublicKey().Bytes())
	require.NoError(t, err)
	link22CctpVerifierVaultATA, err := tokens.FindFeeVaultATA(config.Token2022Program, linkMint, config.CctpTokenPoolProgram, nil)
	require.NoError(t, err)

	env.Link22 = BillingTokenState{
		Name:             "LINK sample token (2022)",
		Program:          config.Token2022Program,
		Mint:             linkMint,
		FqBillingCfgPDA:  link22PDA,
		BillingATA:       link22Receiver,
		FqEvmCfgPDA:      link22EvmCfg,
		UserATA:          link22UserATA,
		AnotherUserATA:   link22AnotherUserATA,
		TokenlessUserATA: link22TokenlessUserATA,
		FeeAggregatorATA: link22FeeAggregatorATA,

		// 2.0
		ExecutorVaultATA:     link22ExecutorVaultATA,
		CVVaultATA:           link22CVVaultATA,
		BurnMintPoolVaultATA: link22BurnMintPoolVaultATA,
		UsdcPoolVaultATA:     link22UsdcPoolVaultATA,
		CctpVerifierVaultATA: link22CctpVerifierVaultATA,
	}
}

func (env *TestEnv) setupTokens(ctx context.Context) {
	t := env.T
	t.Helper()

	token0Decimals := uint8(9)
	token1Decimals := uint8(18)
	token2Decimals := uint8(9)
	usdcDecimals := uint8(6)

	var err error
	var tp tokens.TokenPool
	tp, err = tokens.NewTokenPool(config.SPLTokenProgram, config.CcipBurnMintTokenPoolProgram, Token0MintPrivK.PublicKey())
	require.NoError(t, err)
	env.Token0 = tp
	tp, err = tokens.NewTokenPool(config.Token2022Program, config.CcipBurnMintTokenPoolProgram, Token1MintPrivK.PublicKey())
	require.NoError(t, err)
	env.Token1 = tp
	env.Token1.WithTokenExtensions = true
	tp, err = tokens.NewTokenPool(config.Token2022Program, config.CcipBurnMintTokenPoolProgram, Token2MintPrivK.PublicKey())
	require.NoError(t, err)
	env.Token2 = tp
	tp, err = tokens.NewTokenPool(config.Token2022Program, config.CcipBurnMintTokenPoolProgram, LinkMintPrivK.PublicKey())
	require.NoError(t, err)
	env.LinkPool = tp
	tp, err = tokens.NewTokenPool(config.SPLTokenProgram, config.CctpTokenPoolProgram, UsdcMintPrivK.PublicKey())
	require.NoError(t, err)
	env.UsdcPool = tp

	// CCTP config
	env.CctpSolDomain = uint32(5)
	env.CctpEvmDomain = uint32(1)
	env.MessageTransmitter = cctp.GetMessageTransmitterPDAs(t)
	env.TokenMessengerMinter = cctp.GetTokenMessengerMinterPDAs(t, env.CctpEvmDomain, UsdcMintPrivK.PublicKey(), config.EVMUsdcAddressBytes) // tests with CCTPv1 use SOL<>SOL
	env.MessageTransmitterV2 = cctp.GetMessageTransmitterV2PDAs(t)
	env.TokenMessengerMinterV2 = cctp.GetTokenMessengerMinterV2PDAs(t, env.CctpEvmDomain, UsdcMintPrivK.PublicKey(), config.EVMUsdcAddressBytes) // tests with CCTPv2 use SOL<>EVM, because Circle added a check forbidding same domain as source & dest

	// Create token0 (SPL) + multisig
	ix0, err := tokens.CreateTokenFrom(ctx, env.Token0, Token0Admin.PublicKey(), token0Decimals, env.Client, config.DefaultCommitment)
	require.NoError(t, err)
	ixMsig, err := tokens.CreateMultisig(ctx, User.PublicKey(), config.SPLTokenProgram, Token0Multisig.PublicKey(), 1, []solana.PublicKey{Token0Admin.PublicKey(), env.Token0.PoolSigner}, env.Client, config.DefaultCommitment)
	require.NoError(t, err)
	SendAndConfirm(ctx, t, env.Client, ixMsig, Token0Admin, config.DefaultCommitment, common.AddSigners(Token0Multisig, User))

	// Create token1 (Token2022 with extensions)
	ix1, err := tokens.CreateTokenFrom(ctx, env.Token1, Token1Admin.PublicKey(), token1Decimals, env.Client, config.DefaultCommitment)
	require.NoError(t, err)
	// Create token2 (Token2022)
	ix2, err := tokens.CreateTokenFrom(ctx, env.Token2, Token2Admin.PublicKey(), token2Decimals, env.Client, config.DefaultCommitment)
	require.NoError(t, err)
	// Create USDC token
	ixUsdc, err := tokens.CreateToken(ctx, env.UsdcPool.Program, env.UsdcPool.Mint, UsdcPoolAdmin.PublicKey(), usdcDecimals, env.Client, config.DefaultCommitment)
	require.NoError(t, err)

	// Mint to user + create receiver ATAs
	ixAta0, addr0, err := tokens.CreateAssociatedTokenAccount(env.Token0.Program, env.Token0.Mint, User.PublicKey(), Token0Admin.PublicKey())
	require.NoError(t, err)
	ixMintTo0, err := tokens.MintTo(10000000, env.Token0.Program, env.Token0.Mint, addr0, Token0Admin.PublicKey())
	require.NoError(t, err)
	ixAta1, addr1, err := tokens.CreateAssociatedTokenAccount(env.Token1.Program, env.Token1.Mint, User.PublicKey(), Token1Admin.PublicKey())
	require.NoError(t, err)
	ixMintTo1, err := tokens.MintTo(10000000, env.Token1.Program, env.Token1.Mint, addr1, Token1Admin.PublicKey())
	require.NoError(t, err)
	ixAta2, addr2, err := tokens.CreateAssociatedTokenAccount(env.Token2.Program, env.Token2.Mint, User.PublicKey(), Token2Admin.PublicKey())
	require.NoError(t, err)
	ixMintTo2, err := tokens.MintTo(10000000, env.Token2.Program, env.Token2.Mint, addr2, Token2Admin.PublicKey())
	require.NoError(t, err)
	ixAtaUsdc, usdcAddr, err := tokens.CreateAssociatedTokenAccount(env.UsdcPool.Program, env.UsdcPool.Mint, User.PublicKey(), UsdcPoolAdmin.PublicKey())
	require.NoError(t, err)
	ixMintToUsdc, err := tokens.MintTo(10000000, env.UsdcPool.Program, env.UsdcPool.Mint, usdcAddr, UsdcPoolAdmin.PublicKey())
	require.NoError(t, err)

	// Receiver ATAs
	ixAtaReceiver0, recAddr0, err := tokens.CreateAssociatedTokenAccount(env.Token0.Program, env.Token0.Mint, config.ReceiverExternalExecutionConfigPDA, Token0Admin.PublicKey())
	require.NoError(t, err)
	ixAtaReceiver1, recAddr1, err := tokens.CreateAssociatedTokenAccount(env.Token1.Program, env.Token1.Mint, config.ReceiverExternalExecutionConfigPDA, Token1Admin.PublicKey())
	require.NoError(t, err)
	ixAtaReceiver2, recAddr2, err := tokens.CreateAssociatedTokenAccount(env.Token2.Program, env.Token2.Mint, config.ReceiverExternalExecutionConfigPDA, Token2Admin.PublicKey())
	require.NoError(t, err)
	ixAtaReceiverLink, recAddrLink, err := tokens.CreateAssociatedTokenAccount(env.Link22.Program, env.Link22.Mint, config.ReceiverExternalExecutionConfigPDA, Admin.PublicKey())
	require.NoError(t, err)
	ixAtaReceiverUsdc, recAddrUsdc, err := tokens.CreateAssociatedTokenAccount(env.UsdcPool.Program, env.UsdcPool.Mint, config.ReceiverExternalExecutionConfigPDA, UsdcPoolAdmin.PublicKey())
	require.NoError(t, err)

	// Map user→ATA
	env.Token0.User[User.PublicKey()] = addr0
	env.Token0.User[config.ReceiverExternalExecutionConfigPDA] = recAddr0
	env.Token1.User[User.PublicKey()] = addr1
	env.Token1.User[config.ReceiverExternalExecutionConfigPDA] = recAddr1
	env.Token2.User[User.PublicKey()] = addr2
	env.Token2.User[config.ReceiverExternalExecutionConfigPDA] = recAddr2
	env.LinkPool.User[User.PublicKey()] = env.Link22.UserATA
	env.LinkPool.User[config.ReceiverExternalExecutionConfigPDA] = recAddrLink
	env.UsdcPool.User[User.PublicKey()] = usdcAddr
	env.UsdcPool.User[config.ReceiverExternalExecutionConfigPDA] = recAddrUsdc

	// Send token creation + minting txs
	ix0 = append(ix0, ixAta0, ixMintTo0, ixAtaReceiver0)
	SendAndConfirm(ctx, t, env.Client, ix0, Token0Admin, config.DefaultCommitment, common.AddSigners(Token0MintPrivK))
	ix1 = append(ix1, ixAta1, ixMintTo1, ixAtaReceiver1)
	SendAndConfirm(ctx, t, env.Client, ix1, Token1Admin, config.DefaultCommitment, common.AddSigners(Token1MintPrivK))
	ix2 = append(ix2, ixAta2, ixMintTo2, ixAtaReceiver2)
	SendAndConfirm(ctx, t, env.Client, ix2, Token2Admin, config.DefaultCommitment, common.AddSigners(Token2MintPrivK))
	ixUsdc = append(ixUsdc, ixAtaUsdc, ixMintToUsdc, ixAtaReceiverUsdc)
	SendAndConfirm(ctx, t, env.Client, ixUsdc, UsdcPoolAdmin, config.DefaultCommitment, common.AddSigners(UsdcMintPrivK))
	SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ixAtaReceiverLink}, Admin, config.DefaultCommitment)
}

func (env *TestEnv) setupPools(ctx context.Context) {
	t := env.T
	t.Helper()

	env.Token0.AdditionalAccounts = append(env.Token0.AdditionalAccounts, Token0Multisig.PublicKey())

	programDataAddress := GetProgramDataAddress(ctx, t, env.Client, config.CcipBurnMintTokenPoolProgram)
	globalConfigPDA, err := tokens.TokenPoolGlobalConfigPDA(config.CcipBurnMintTokenPoolProgram)
	require.NoError(t, err)

	// Init global config
	ixGlobalConfig, err := burnmint_tokenpool.NewInitGlobalConfigInstruction(
		config.CcipRouterProgram,
		config.RMNRemoteProgram,
		globalConfigPDA,
		Admin.PublicKey(),
		solana.SystemProgramID,
		config.CcipBurnMintTokenPoolProgram,
		programDataAddress,
	)
	require.NoError(t, err)
	SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ixGlobalConfig}, Admin, config.DefaultCommitment)

	// Init pools
	ixInit0, err := burnmint_tokenpool.NewInitializeInstruction(env.Token0.PoolConfig, env.Token0.Mint, Admin.PublicKey(), solana.SystemProgramID, config.CcipBurnMintTokenPoolProgram, programDataAddress, globalConfigPDA)
	require.NoError(t, err)
	ixInit1, err := burnmint_tokenpool.NewInitializeInstruction(env.Token1.PoolConfig, env.Token1.Mint, Admin.PublicKey(), solana.SystemProgramID, config.CcipBurnMintTokenPoolProgram, programDataAddress, globalConfigPDA)
	require.NoError(t, err)
	ixInit2, err := burnmint_tokenpool.NewInitializeInstruction(env.Token2.PoolConfig, env.Token2.Mint, Admin.PublicKey(), solana.SystemProgramID, config.CcipBurnMintTokenPoolProgram, programDataAddress, globalConfigPDA)
	require.NoError(t, err)
	ixInitLink, err := burnmint_tokenpool.NewInitializeInstruction(env.LinkPool.PoolConfig, env.LinkPool.Mint, Admin.PublicKey(), solana.SystemProgramID, config.CcipBurnMintTokenPoolProgram, programDataAddress, globalConfigPDA)
	require.NoError(t, err)

	// Transfer ownership to individual pool admins
	ixTransfer0, err := burnmint_tokenpool.NewTransferOwnershipInstruction(Token0Admin.PublicKey(), env.Token0.PoolConfig, env.Token0.Mint, Admin.PublicKey())
	require.NoError(t, err)
	ixTransfer1, err := burnmint_tokenpool.NewTransferOwnershipInstruction(Token1Admin.PublicKey(), env.Token1.PoolConfig, env.Token1.Mint, Admin.PublicKey())
	require.NoError(t, err)
	ixTransfer2, err := burnmint_tokenpool.NewTransferOwnershipInstruction(Token2Admin.PublicKey(), env.Token2.PoolConfig, env.Token2.Mint, Admin.PublicKey())
	require.NoError(t, err)

	ixAccept0, err := burnmint_tokenpool.NewAcceptOwnershipInstruction(env.Token0.PoolConfig, env.Token0.Mint, Token0Admin.PublicKey())
	require.NoError(t, err)
	ixAccept1, err := burnmint_tokenpool.NewAcceptOwnershipInstruction(env.Token1.PoolConfig, env.Token1.Mint, Token1Admin.PublicKey())
	require.NoError(t, err)
	ixAccept2, err := burnmint_tokenpool.NewAcceptOwnershipInstruction(env.Token2.PoolConfig, env.Token2.Mint, Token2Admin.PublicKey())
	require.NoError(t, err)

	// Pool token accounts (ATAs for pool signers)
	ixAta0, addr0, err := tokens.CreateAssociatedTokenAccount(env.Token0.Program, env.Token0.Mint, env.Token0.PoolSigner, Token0Admin.PublicKey())
	require.NoError(t, err)
	env.Token0.PoolTokenAccount = addr0
	env.Token0.User[env.Token0.PoolSigner] = addr0
	ixAta1, addr1, err := tokens.CreateAssociatedTokenAccount(env.Token1.Program, env.Token1.Mint, env.Token1.PoolSigner, Token0Admin.PublicKey())
	require.NoError(t, err)
	env.Token1.PoolTokenAccount = addr1
	env.Token1.User[env.Token1.PoolSigner] = addr1
	ixAta2, addr2, err := tokens.CreateAssociatedTokenAccount(env.Token2.Program, env.Token2.Mint, env.Token2.PoolSigner, Token0Admin.PublicKey())
	require.NoError(t, err)
	env.Token2.PoolTokenAccount = addr2
	env.Token2.User[env.Token2.PoolSigner] = addr2
	ixAtaLink, addrLink, err := tokens.CreateAssociatedTokenAccount(env.LinkPool.Program, env.LinkPool.Mint, env.LinkPool.PoolSigner, Admin.PublicKey())
	require.NoError(t, err)
	env.LinkPool.PoolTokenAccount = addrLink
	env.LinkPool.User[env.LinkPool.PoolSigner] = addrLink
	ixAuth, err := tokens.SetTokenMintAuthority(env.Token0.Program, Token0Multisig.PublicKey(), env.Token0.Mint, Token0Admin.PublicKey())
	require.NoError(t, err)

	SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ixInit0, ixInit1, ixInit2, ixInitLink}, Admin, config.DefaultCommitment)
	SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ixTransfer0, ixTransfer1, ixTransfer2, ixAccept0, ixAccept1, ixAccept2}, Admin, config.DefaultCommitment, common.AddSigners(Token0Admin, Token1Admin, Token2Admin))
	SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ixAta0, ixAta1, ixAuth, ixAta2, ixAtaLink}, Token0Admin, config.DefaultCommitment, common.AddSigners(Token1Admin, Token2Admin, Admin))

	// Setup lookup tables for each pool
	require.NoError(t, env.Token0.SetupLookupTable(ctx, env.Client, Token0Admin))
	require.NoError(t, env.Token1.SetupLookupTable(ctx, env.Client, Token1Admin))
	require.NoError(t, env.Token2.SetupLookupTable(ctx, env.Client, Token2Admin))
	require.NoError(t, env.LinkPool.SetupLookupTable(ctx, env.Client, Admin))
	env.assertTokenPoolLookupTable(ctx, env.Token0)
	env.assertTokenPoolLookupTable(ctx, env.Token1)
	env.assertTokenPoolLookupTable(ctx, env.Token2)
	env.assertTokenPoolLookupTable(ctx, env.LinkPool)
}

func (env *TestEnv) setupCCTPPool(ctx context.Context) {
	t := env.T
	t.Helper()

	cctpProgramDataAddress := GetProgramDataAddress(ctx, t, env.Client, config.CctpTokenPoolProgram)

	ixInit, err := cctp_token_pool.NewInitializeInstruction(
		config.CcipRouterProgram,
		config.RMNRemoteProgram,
		env.UsdcPool.PoolConfig,
		env.UsdcPool.Mint,
		Admin.PublicKey(),
		solana.SystemProgramID,
		config.CctpTokenPoolProgram,
		cctpProgramDataAddress,
		env.UsdcPool.GlobalConfig,
	)
	require.NoError(t, err)

	ixTransfer, err := cctp_token_pool.NewTransferOwnershipInstruction(
		UsdcPoolAdmin.PublicKey(),
		env.UsdcPool.PoolConfig,
		env.UsdcPool.Mint,
		Admin.PublicKey(),
	)
	require.NoError(t, err)

	SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ixInit, ixTransfer}, Admin, config.DefaultCommitment)

	ixAccept, err := cctp_token_pool.NewAcceptOwnershipInstruction(
		env.UsdcPool.PoolConfig,
		env.UsdcPool.Mint,
		UsdcPoolAdmin.PublicKey(),
	)
	require.NoError(t, err)

	ixAta, addr, err := tokens.CreateAssociatedTokenAccount(env.UsdcPool.Program, env.UsdcPool.Mint, env.UsdcPool.PoolSigner, UsdcPoolAdmin.PublicKey())
	require.NoError(t, err)
	env.UsdcPool.PoolTokenAccount = addr
	env.UsdcPool.User[env.UsdcPool.PoolSigner] = addr

	ixFunding, err := tokens.NativeTransfer(10*solana.LAMPORTS_PER_SOL, UsdcPoolAdmin.PublicKey(), env.UsdcPool.PoolSigner) // the signer PDA funds the rent of the CCTP event PDAs
	require.NoError(t, err)

	SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ixAccept, ixAta, ixFunding}, UsdcPoolAdmin, config.DefaultCommitment)

	env.UsdcPool.AdditionalAccounts = append(env.UsdcPool.AdditionalAccounts,
		env.MessageTransmitter.MessageTransmitter,
		config.CctpTokenMessengerMinter,
		solana.SystemProgramID,
		config.CctpMessageTransmitter,
		env.TokenMessengerMinter.TokenMessenger,
		env.TokenMessengerMinter.TokenMinter,
		env.TokenMessengerMinter.LocalToken,
		env.TokenMessengerMinter.EventAuthority,
	)
	env.UsdcPool.WritableIndexes = append(env.UsdcPool.WritableIndexes,
		5,      // it requires the cpi signer to be writable, to stake rent for event PDAs
		10, 16) // these are specific to CCTPv1

	require.NoError(t, env.UsdcPool.SetupLookupTable(ctx, env.Client, UsdcPoolAdmin))
	env.assertTokenPoolLookupTable(ctx, env.UsdcPool)
}

func (env *TestEnv) setupCCTPVerifier(ctx context.Context) {
	t := env.T
	t.Helper()

	usdcEvmAltEntries := []solana.PublicKey{
		// Verifier accounts
		config.CctpVerifierConfigPDA,
		config.CctpVerifierEvmRemoteChainConfigPDA,
		solana.SystemProgramID,
		solana.TokenProgramID,
		env.UsdcPool.Mint,
		env.UsdcPool.PoolSigner,
		env.UsdcPool.PoolTokenAccount,

		// Message Transmitter accounts
		env.MessageTransmitterV2.AuthorityPda,
		env.MessageTransmitterV2.EventAuthority,
		env.MessageTransmitterV2.MessageTransmitter,
		env.MessageTransmitterV2.Program,

		// Token Messenger Minter accounts
		env.TokenMessengerMinterV2.AuthorityPda,
		env.TokenMessengerMinterV2.CustodyTokenAccount,
		env.TokenMessengerMinterV2.EventAuthority,
		env.TokenMessengerMinterV2.LocalToken,
		env.TokenMessengerMinterV2.Program,
		env.TokenMessengerMinterV2.Program, // receiver
		env.TokenMessengerMinterV2.RemoteTokenMessenger,
		env.TokenMessengerMinterV2.TokenMessenger,
		env.TokenMessengerMinterV2.TokenMinter,
		env.TokenMessengerMinterV2.TokenPair,
		cctp.GetDenylistV2PDA(t, env.UsdcPool.PoolSigner),
	}
	usdcEvmAlt, err := common.SetupLookupTable(ctx, env.Client, Admin, usdcEvmAltEntries)
	require.NoError(t, err)

	programDataAddr := GetProgramDataAddress(ctx, t, env.Client, config.CctpTokenPoolProgram)

	ixs := make([]solana.Instruction, 0) //nolint:prealloc // no need to optimize, this is a test

	ix, err := cctp_token_pool.NewInitializeVerifierInstruction(
		config.CctpVerifierConfigPDA,
		config.CctpVerifierStorageLocationsPDA,
		Admin.PublicKey(),
		solana.SystemProgramID,
		config.CctpTokenPoolProgram,
		programDataAddr,
	)
	require.NoError(t, err)
	ixs = append(ixs, ix)

	ix, err = cctp_token_pool.NewSetVerifierFeeAggregatorInstruction(
		FeeAggregator.PublicKey(),
		config.CctpVerifierConfigPDA,
		Admin.PublicKey(),
		config.CctpEventAuthorityPDA,
		config.CctpTokenPoolProgram,
	)
	require.NoError(t, err)
	ixs = append(ixs, ix)

	ix, err = cctp_token_pool.NewInitVerifierChainConfigInstruction(
		cctp_token_pool.SetVerifierChainConfigParams{
			RemoteChainSelector: config.EvmChainSelector,
			ChainConfig: cctp_token_pool.RemoteChainAdminConfig{
				Router:             config.CcipRouterProgram,
				Fee:                cctp_token_pool.UsdCents{V0: 5},
				GasForVerification: cctp_token_pool.CrossChainGas{},
				PayloadSizeBytes:   0,
			},
		},
		config.CctpVerifierConfigPDA,
		Admin.PublicKey(),
		config.CctpVerifierEvmRemoteChainConfigPDA,
		solana.SystemProgramID,
		config.CctpEventAuthorityPDA,
		config.CctpTokenPoolProgram,
	)
	require.NoError(t, err)
	ixs = append(ixs, ix)

	ix, err = cctp_token_pool.NewSetVerifierChainConfigCctpInstruction(
		cctp_token_pool.SetVerifierChainConfigCctpParams{
			RemoteChainSelector: config.EvmChainSelector,
			Cctp: cctp_token_pool.RemoteCctpConfig{
				Enabled:              true,
				DomainIdentifier:     env.CctpEvmDomain,
				LookupTable:          &usdcEvmAlt,
				AllowedCallerReceive: solana.PublicKey(config.EVMCctpMessageTransmitterProxy),
				AllowedCallerDeposit: solana.PublicKey(config.EVMUsdcPoolAddressBytes),
				MintRecipientOnDest:  nil,
				SupportedTokens: []cctp_token_pool.RemoteLocalPair{
					{Local: env.UsdcPool.Mint, Remote: cctp_token_pool.RemoteAddress{Address: config.EVMUsdcAddressBytesTrim}},
				},
			},
		},
		config.CctpVerifierConfigPDA,
		Admin.PublicKey(),
		config.CctpVerifierEvmRemoteChainConfigPDA,
		solana.SystemProgramID,
		config.CctpEventAuthorityPDA,
		config.CctpTokenPoolProgram,
	)
	require.NoError(t, err)
	ixs = append(ixs, ix)

	SendAndConfirm(ctx, t, env.Client, ixs, Admin, config.DefaultCommitment)
}

func (env *TestEnv) setupCCTPv2(ctx context.Context) {
	t := env.T
	t.Helper()

	// Exports
	env.CctpAttesters = []eth.Signer{CctpAttester}

	// Actually do the setup
	cctpAdmin := Admin // this is the upgrade authority of all programs when we do the genesis inclusion of the contracts
	tokenController := UsdcPoolAdmin
	feeRecipient := FeeAggregator

	// CCTP Message Transmitter V2 program
	{
		programDataAddr := GetProgramDataAddress(ctx, t, env.Client, env.MessageTransmitterV2.Program)

		ix, err := cctp_message_transmitter_v2.NewInitializeInstruction(
			// Values based on https://explorer.solana.com/address/W1k5ijkaSTo5iA5zChNpfzcy796fLhkBxfmJuR8W8HU/anchor-account
			cctp_message_transmitter_v2.InitializeParams{
				LocalDomain:        env.CctpSolDomain,
				Attester:           solana.PublicKey(common.ToLeftPadded32Bytes(CctpAttester.Address[:])),
				MaxMessageBodySize: 8192,
				Version:            1,
			},
			cctpAdmin.PublicKey(),
			cctpAdmin.PublicKey(),
			env.MessageTransmitterV2.MessageTransmitter,
			programDataAddr,
			env.MessageTransmitterV2.Program,
			solana.SystemProgramID,
			env.MessageTransmitterV2.EventAuthority,
			env.MessageTransmitterV2.Program,
		)
		require.NoError(t, err)

		SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ix}, cctpAdmin, config.DefaultCommitment)
	}

	// CCTP Token Messenger Minter V2 program
	{
		programDataAddr := GetProgramDataAddress(ctx, t, env.Client, env.TokenMessengerMinterV2.Program)

		ix, err := cctp_token_messenger_minter_v2.NewInitializeInstruction(
			// Values based on https://explorer.solana.com/address/AawthJCGRmggpfv9MMWV6Jmo9cue4gL9wUZgRBShg58W/anchor-account
			cctp_token_messenger_minter_v2.InitializeParams{
				TokenController:    tokenController.PublicKey(),
				MinFee:             0,
				MessageBodyVersion: 1,
				FeeRecipient:       feeRecipient.PublicKey(),
				// the following addresses are set to a stub value in this test we don't care about it
				// but they can't be zero, as the program checks for that
				Denylister:       solana.SysVarClockPubkey,
				MinFeeController: solana.SysVarClockPubkey,
			},
			cctpAdmin.PublicKey(),
			cctpAdmin.PublicKey(),
			env.TokenMessengerMinterV2.AuthorityPda,
			env.TokenMessengerMinterV2.TokenMessenger,
			env.TokenMessengerMinterV2.TokenMinter,
			programDataAddr,
			env.TokenMessengerMinterV2.Program,
			solana.SystemProgramID,
			env.TokenMessengerMinterV2.EventAuthority,
			env.TokenMessengerMinterV2.Program,
		)
		require.NoError(t, err)

		feeAtaIx, _, err := tokens.CreateAssociatedTokenAccount(env.UsdcPool.Program, env.UsdcPool.Mint, feeRecipient.PublicKey(), cctpAdmin.PublicKey())
		require.NoError(t, err)

		SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ix, feeAtaIx}, cctpAdmin, config.DefaultCommitment)

		ix, err = cctp_token_messenger_minter_v2.NewAddRemoteTokenMessengerInstruction(
			cctp_token_messenger_minter_v2.AddRemoteTokenMessengerParams{
				Domain:         env.CctpEvmDomain,
				TokenMessenger: solana.PublicKey(config.EVMCctpTokenMessenger),
			},
			cctpAdmin.PublicKey(),
			cctpAdmin.PublicKey(),
			env.TokenMessengerMinterV2.TokenMessenger,
			env.TokenMessengerMinterV2.RemoteTokenMessenger,
			solana.SystemProgramID,
			env.TokenMessengerMinterV2.EventAuthority,
			env.TokenMessengerMinterV2.Program,
		)
		require.NoError(t, err)

		SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ix}, cctpAdmin, config.DefaultCommitment)

		ix, err = cctp_token_messenger_minter_v2.NewAddLocalTokenInstruction(
			cctp_token_messenger_minter_v2.AddLocalTokenParams{},
			cctpAdmin.PublicKey(),
			tokenController.PublicKey(),
			env.TokenMessengerMinterV2.TokenMinter,
			env.TokenMessengerMinterV2.LocalToken,
			env.TokenMessengerMinterV2.CustodyTokenAccount,
			env.UsdcPool.Mint,
			solana.TokenProgramID,
			solana.SystemProgramID,
			env.TokenMessengerMinterV2.EventAuthority,
			env.TokenMessengerMinterV2.Program,
		)
		require.NoError(t, err)

		SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ix}, cctpAdmin, config.DefaultCommitment, common.AddSigners(tokenController))

		ixMintToCustody, err := tokens.MintTo(10000000, env.UsdcPool.Program, env.UsdcPool.Mint, env.TokenMessengerMinterV2.CustodyTokenAccount, UsdcPoolAdmin.PublicKey())
		require.NoError(t, err)
		SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ixMintToCustody}, UsdcPoolAdmin, config.DefaultCommitment, common.AddSigners(UsdcMintPrivK))

		ix, err = cctp_token_messenger_minter_v2.NewLinkTokenPairInstruction(
			cctp_token_messenger_minter_v2.LinkTokenPairParams{
				LocalToken:   env.TokenMessengerMinterV2.LocalToken,
				RemoteDomain: env.CctpEvmDomain,
				RemoteToken:  solana.PublicKey(config.EVMUsdcAddressBytes),
			},
			cctpAdmin.PublicKey(),
			tokenController.PublicKey(),
			env.TokenMessengerMinterV2.TokenMinter,
			env.TokenMessengerMinterV2.TokenPair,
			solana.SystemProgramID,
			env.TokenMessengerMinterV2.EventAuthority,
			env.TokenMessengerMinterV2.Program,
		)
		require.NoError(t, err)

		SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ix}, cctpAdmin, config.DefaultCommitment, common.AddSigners(tokenController))

		ix, err = cctp_token_messenger_minter_v2.NewSetMaxBurnAmountPerMessageInstruction(
			cctp_token_messenger_minter_v2.SetMaxBurnAmountPerMessageParams{
				BurnLimitPerMessage: 2 * 1e6, // 2 USDC, as it uses 6 decimals
			},
			tokenController.PublicKey(),
			env.TokenMessengerMinterV2.TokenMinter,
			env.TokenMessengerMinterV2.LocalToken,
			env.TokenMessengerMinterV2.EventAuthority,
			env.TokenMessengerMinterV2.Program,
		)
		require.NoError(t, err)

		SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ix}, cctpAdmin, config.DefaultCommitment, common.AddSigners(tokenController))
	}
}

func (env *TestEnv) setupReceiver(ctx context.Context) {
	t := env.T
	t.Helper()

	ix, err := test_ccip_receiver.NewInitializeInstruction(
		config.CcipRouterProgram,
		config.ReceiverTargetAccountPDA,
		config.ReceiverExternalExecutionConfigPDA,
		User.PublicKey(),
		solana.SystemProgramID,
	)
	require.NoError(t, err)
	SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ix}, User, config.DefaultCommitment)
}

func (env *TestEnv) setupBillingFundingAndApprovals(ctx context.Context) {
	t := env.T
	t.Helper()

	billingTokens := []*BillingTokenState{&env.WSOL, &env.Link22}

	type Item struct {
		name       string
		user       solana.PrivateKey
		getATA     func(bt *BillingTokenState) solana.PublicKey
		shouldFund bool
	}
	list := []Item{
		{name: "user", user: User, getATA: func(bt *BillingTokenState) solana.PublicKey { return bt.UserATA }, shouldFund: true},
		{name: "anotherUser", user: AnotherUser, getATA: func(bt *BillingTokenState) solana.PublicKey { return bt.AnotherUserATA }, shouldFund: true},
		{name: "tokenlessUser", user: TokenlessUser, getATA: func(bt *BillingTokenState) solana.PublicKey { return bt.TokenlessUserATA }, shouldFund: false},
	}

	feeAggrAtaIx := make([]solana.Instruction, len(billingTokens))
	for i, bt := range billingTokens {
		ixAtaFeeAggr, addrFeeAggr, err := tokens.CreateAssociatedTokenAccount(bt.Program, bt.Mint, FeeAggregator.PublicKey(), FeeAggregator.PublicKey())
		require.NoError(t, err)
		require.Equal(t, bt.FeeAggregatorATA, addrFeeAggr)
		feeAggrAtaIx[i] = ixAtaFeeAggr
	}
	SendAndConfirm(ctx, t, env.Client, feeAggrAtaIx, FeeAggregator, config.DefaultCommitment)

	// 2.0: fee-vault ATAs of the receipt issuers (executor, committee verifier, burn-mint pool
	// program). ccip_send_v2 pays each receipt into the ATA of the issuer's ["fee_vault"] PDA
	// and fails with MissingAtaForReceiptIssuer if it doesn't exist, so they're created up
	// front for both billing tokens. The ATA owner is the vault PDA; Admin just pays the rent.
	vaultAtaIxs := make([]solana.Instruction, 0, len(billingTokens)*3)
	for _, bt := range billingTokens {
		issuerVaults := []struct {
			issuer    solana.PublicKey
			qualifier []byte
			vaultATA  solana.PublicKey
		}{
			{config.ExecutorProgram, nil, bt.ExecutorVaultATA},
			{config.CommitteeVerifierProgram, nil, bt.CVVaultATA},
			// The pool's fee vault is qualified by the mint of the managed token (Token1 for
			// the onramp suite), so the same pool program is billed separately per token.
			{config.CcipBurnMintTokenPoolProgram, Token1MintPrivK.PublicKey().Bytes(), bt.BurnMintPoolVaultATA},
			{config.CctpTokenPoolProgram, nil, bt.CctpVerifierVaultATA},
			{config.CctpTokenPoolProgram, UsdcMintPrivK.PublicKey().Bytes(), bt.UsdcPoolVaultATA},
		}
		for _, iv := range issuerVaults {
			vaultPDA, _, err := state.FindFeeVaultPDA(iv.issuer, iv.qualifier)
			require.NoError(t, err)
			ixVaultAta, addrVaultATA, err := tokens.CreateAssociatedTokenAccount(bt.Program, bt.Mint, vaultPDA, Admin.PublicKey())
			require.NoError(t, err)
			require.Equal(t, iv.vaultATA, addrVaultATA)
			vaultAtaIxs = append(vaultAtaIxs, ixVaultAta)
		}
	}
	SendAndConfirm(ctx, t, env.Client, vaultAtaIxs, Admin, config.DefaultCommitment)

	for _, it := range list {
		for _, bt := range billingTokens {
			ixAtaUser, addrUser, err := tokens.CreateAssociatedTokenAccount(bt.Program, bt.Mint, it.user.PublicKey(), it.user.PublicKey())
			require.NoError(t, err)
			require.Equal(t, it.getATA(bt), addrUser)

			// The allowance must cover every fee-paying send in the suite: ccip_send_v2 pulls the
			// full quoted fee through this delegation, and at the test WSOL price a single
			// token-transfer send costs ~1 SOL in fees.
			ixApprove, err := tokens.TokenApproveChecked(100e9, 9, bt.Program, it.getATA(bt), bt.Mint, config.BillingSignerPDA, it.user.PublicKey(), []solana.PublicKey{})
			require.NoError(t, err)

			SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ixAtaUser, ixApprove}, it.user, config.DefaultCommitment)
		}

		if it.shouldFund {
			ixMint, err := tokens.MintTo(1e9, env.Link22.Program, env.Link22.Mint, it.getATA(&env.Link22), Admin.PublicKey())
			require.NoError(t, err)
			SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ixMint}, Admin, config.DefaultCommitment)

			// Wrap enough SOL to pay the v2 flat fees across the whole suite (~1 SOL per
			// token-transfer send at the test WSOL price); accounts are airdropped 1000 SOL.
			transferAmount := 50.0 * solana.LAMPORTS_PER_SOL
			ixTransfer, err := tokens.NativeTransfer(transferAmount, it.user.PublicKey(), it.getATA(&env.WSOL))
			require.NoError(t, err)
			ixSync, err := tokens.SyncNative(env.WSOL.Program, it.getATA(&env.WSOL))
			require.NoError(t, err)
			SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ixTransfer, ixSync}, it.user, config.DefaultCommitment)
		}
	}
}

func (env *TestEnv) setupLookupTables(ctx context.Context) {
	t := env.T
	t.Helper()

	// CcipSend lookup table
	sendEntries := []solana.PublicKey{
		config.RouterConfigPDA,
		env.NonceEvmPDA,
		env.NonceSvmPDA,
		config.EvmDestChainStatePDA,
		config.SvmDestChainStatePDA,
		solana.SystemProgramID,
		config.SPLTokenProgram,
		config.Token2022Program,
		config.FeeQuoterProgram,
		config.FqConfigPDA,
		config.FqEvmDestChainPDA,
		config.FqSvmDestChainPDA,
		env.WSOL.FqBillingCfgPDA,
		env.WSOL.Mint,
		env.WSOL.UserATA,
		env.WSOL.BillingATA,
		env.WSOL.AnotherUserATA,
		env.Link22.FqBillingCfgPDA,
		config.RouterEventAuthorityPDA,
		env.Link22.Mint,
		env.Link22.UserATA,
		env.Link22.BillingATA,
		config.ExecutorProgram,
		config.ExecConfigPDA,
		config.ExecEvmChainConfigPDA,
		config.ExecSvmChainConfigPDA,
		config.ExecEventAuthorityPDA,
		// 2.0 lane accounts
		config.EvmDestChainStatePDAV2,
		config.FqEvmDestChainV2PDA,
		config.BillingSignerPDA,
		config.RMNRemoteProgram,
		config.RMNRemoteConfigPDA,
		config.RMNRemoteCursesPDA,
		config.CommitteeVerifierProgram,
		config.CVConfigPDA,
		config.CVEventAuthorityPDA,
		config.CVEvmRemoteChainConfigPDA,
		config.CCVSignerPDA,
		// 2.0 fee-vault ATAs of the receipt issuers
		env.WSOL.ExecutorVaultATA,
		env.WSOL.CVVaultATA,
		env.WSOL.BurnMintPoolVaultATA,
		env.Link22.ExecutorVaultATA,
		env.Link22.CVVaultATA,
		env.Link22.BurnMintPoolVaultATA,
	}
	var err error
	env.CcipSendLookupTableAddr, err = common.SetupLookupTable(ctx, env.Client, Admin, sendEntries)
	require.NoError(t, err)
	env.CcipSendLookupTable = map[solana.PublicKey]solana.PublicKeySlice{
		env.CcipSendLookupTableAddr: sendEntries,
	}

	// Offramp lookup table
	offrampEntries := []solana.PublicKey{
		config.CcipOfframpProgram,
		config.OfframpConfigPDA,
		config.OfframpReferenceAddressesPDA,
		config.OfframpEventAuthorityPDA,
		config.OfframpEvmSourceChainPDA,
		solana.SystemProgramID,
		solana.SysVarInstructionsPubkey,
		solana.SPLAssociatedTokenAccountProgramID,
		config.OfframpBillingSignerPDA,
		config.FeeQuoterProgram,
		config.FqConfigPDA,
		config.RMNRemoteProgram,
		config.RMNRemoteConfigPDA,
		config.RMNRemoteCursesPDA,
		config.FqEvmDestChainPDA,
		config.FqSvmDestChainPDA,
		env.WSOL.FqBillingCfgPDA,
		env.WSOL.FqEvmCfgPDA,
		env.Link22.FqBillingCfgPDA,
		env.Link22.FqEvmCfgPDA,
	}
	env.OfframpLookupTableAddr, err = common.SetupLookupTable(ctx, env.Client, Admin, offrampEntries)
	require.NoError(t, err)
	env.OfframpLookupTable = map[solana.PublicKey]solana.PublicKeySlice{
		env.OfframpLookupTableAddr: offrampEntries,
	}
}

func (env *TestEnv) setupRMN(ctx context.Context) {
	t := env.T
	t.Helper()

	rmnProgramDataAddress := GetProgramDataAddress(ctx, t, env.Client, config.RMNRemoteProgram)

	ix, err := rmn_remote.NewInitializeInstruction(
		config.RMNRemoteConfigPDA,
		config.RMNRemoteCursesPDA,
		Admin.PublicKey(),
		solana.SystemProgramID,
		config.RMNRemoteProgram,
		rmnProgramDataAddress,
	)
	require.NoError(t, err)

	ix2, err := rmn_remote.NewSetEventAuthoritiesInstruction(
		[]solana.PublicKey{config.BillingSignerPDA},
		config.RMNRemoteConfigPDA,
		Admin.PublicKey(),
		solana.SystemProgramID,
	)
	require.NoError(t, err)

	result := SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ix, ix2}, Admin, config.DefaultCommitment)
	require.NotNil(t, result)
}

func (env *TestEnv) setupCV(ctx context.Context) {
	t := env.T
	t.Helper()

	cvProgramDataAddress := GetProgramDataAddress(ctx, t, env.Client, config.CommitteeVerifierProgram)

	ix, err := committee_verifier.NewInitializeInstruction(
		config.CVConfigPDA,
		config.CVStorageLocationsPDA,
		Admin.PublicKey(),
		solana.SystemProgramID,
		config.CommitteeVerifierProgram,
		cvProgramDataAddress,
	)
	require.NoError(t, err)

	result := SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ix}, Admin, config.DefaultCommitment)
	require.NotNil(t, result)

	// The committee verifier's get_fee gates the requested finality against the config's
	// allowed_finality_config, which is initialized to WAIT_FOR_FINALITY (full finality only).
	// Relax it to permit the flag-only fast finality (Flags:1) that the onramp send tests
	// exercise; wait-for-finality requests remain allowed regardless.
	ixFinality, err := committee_verifier.NewSetAllowedFinalityConfigInstruction(
		committee_verifier.FinalityConfig{Flags: 1},
		config.CVConfigPDA,
		Admin.PublicKey(),
		config.CVEventAuthorityPDA,
		config.CommitteeVerifierProgram,
	)
	require.NoError(t, err)
	SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ixFinality}, Admin, config.DefaultCommitment)

	// Remote-chain config for the v2.0 EVM lane, so the committee verifier can be used as a CCV on
	// that lane (its get_fee / start_verification CPIs load the remote_chain_config and require the
	// router's CCV signer PDA as authority).
	resolvedCVConfig, err := ccip.ResolveCVInitChainConfigAccounts(ctx, env.Client, committee_verifier.ProgramID, committee_verifier.SetChainConfigParams{
		RemoteChainSelector: config.EvmChainSelector,
		ChainConfig: committee_verifier.RemoteChainAdminConfig{
			RouterSigner: config.CCVSignerPDA,
		},
	}, Admin.PublicKey(), logger.Test(t))
	require.NoError(t, err)
	SendAndConfirm(ctx, t, env.Client, []solana.Instruction{resolvedCVConfig.Ix}, Admin, config.DefaultCommitment)
}

// ----- Convenience helpers ----- //

// GetBalance returns the token balance of the given ATA.
func (env *TestEnv) GetBalance(tokenAccount solana.PublicKey) uint64 {
	env.T.Helper()
	_, amount, err := tokens.TokenBalance(env.Ctx(), env.Client, tokenAccount, config.DefaultCommitment)
	require.NoError(env.T, err)
	return uint64(amount) //nolint:gosec // G115
}

func (env *TestEnv) assertTokenPoolLookupTable(ctx context.Context, token tokens.TokenPool) {
	env.T.Helper()
	expectedEntries := token.ToTokenPoolEntries()
	lookupTableEntries, err := common.GetAddressLookupTable(ctx, env.Client, token.PoolLookupTable)
	require.NoError(env.T, err)
	require.Len(env.T, lookupTableEntries, len(expectedEntries))
	require.Equal(env.T, expectedEntries, lookupTableEntries)
}

// FqTokenConfigPDA returns the fee quoter billing token config PDA for a mint.
func FqTokenConfigPDA(mint solana.PublicKey) solana.PublicKey {
	pda, _, _ := state.FindFqBillingTokenConfigPDA(mint, config.FeeQuoterProgram)
	return pda
}

// FqPerChainPerTokenConfigPDA returns the fee quoter per-chain per-token billing PDA for the EVM chain.
func FqPerChainPerTokenConfigPDA(mint solana.PublicKey) solana.PublicKey {
	pda, _, _ := state.FindFqPerChainPerTokenConfigPDA(config.EvmChainSelector, mint, config.FeeQuoterProgram)
	return pda
}

// InitRouter initializes the router with standard test defaults and returns the result.
func (env *TestEnv) InitRouter(svmChainSelector uint64, feeAggregator solana.PublicKey, linkMint solana.PublicKey) *rpc.GetTransactionResult {
	t := env.T
	t.Helper()
	ctx := env.Ctx()

	routerProgramDataAddress := GetProgramDataAddress(ctx, t, env.Client, config.CcipRouterProgram)
	ix, err := ccip_router.NewInitializeInstruction(
		svmChainSelector,
		feeAggregator,
		config.FeeQuoterProgram,
		linkMint,
		config.RMNRemoteProgram,
		config.RouterConfigPDA,
		Admin.PublicKey(),
		solana.SystemProgramID,
		config.CcipRouterProgram,
		routerProgramDataAddress,
	)
	require.NoError(t, err)

	return SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ix}, Admin, config.DefaultCommitment)
}

// UpdateRouterReferenceAddressesInstruction builds the router instruction that points
// ccip_send_v2 / get_fee_v2 account resolution at onrampLookupTable, signed by authority.
func UpdateRouterReferenceAddressesInstruction(t *testing.T, onrampLookupTable solana.PublicKey, authority solana.PublicKey) solana.Instruction {
	t.Helper()
	ix, err := ccip_router.NewUpdateReferenceAddressesInstruction(
		onrampLookupTable,
		config.RouterReferenceAddressesPDA,
		config.RouterConfigPDA,
		authority,
		solana.SystemProgramID,
		config.RouterEventAuthorityPDA,
		config.CcipRouterProgram,
	)
	require.NoError(t, err)
	return ix
}

// SetOnrampLookupTable points the router's ccip_send_v2 / get_fee_v2 account resolution at the
// suite's CcipSend lookup table, so resolved sends carry it without the test attaching it.
func (env *TestEnv) SetOnrampLookupTable() {
	t := env.T
	t.Helper()
	ix := UpdateRouterReferenceAddressesInstruction(t, env.CcipSendLookupTableAddr, CcipAdmin.PublicKey())
	SendAndConfirm(env.Ctx(), t, env.Client, []solana.Instruction{ix}, CcipAdmin, config.DefaultCommitment)
}

// InitFeeQuoter initializes the fee quoter with standard test defaults and returns the result.
func (env *TestEnv) InitFeeQuoter(maxFeeJuelsPerMsg bin.Uint128, linkMint solana.PublicKey) *rpc.GetTransactionResult {
	t := env.T
	t.Helper()
	ctx := env.Ctx()

	fqProgramDataAddress := GetProgramDataAddress(ctx, t, env.Client, config.FeeQuoterProgram)
	ix, err := fee_quoter.NewInitializeInstruction(
		maxFeeJuelsPerMsg,
		config.CcipRouterProgram,
		config.FqConfigPDA,
		linkMint,
		Admin.PublicKey(),
		solana.SystemProgramID,
		config.FeeQuoterProgram,
		fqProgramDataAddress,
	)
	require.NoError(t, err)

	return SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ix}, Admin, config.DefaultCommitment)
}

// InitOfframp initializes the offramp with standard test defaults.
func (env *TestEnv) InitOfframp(svmChainSelector uint64) {
	t := env.T
	t.Helper()
	ctx := env.Ctx()

	offrampProgramDataAddress := GetProgramDataAddress(ctx, t, env.Client, config.CcipOfframpProgram)

	var lookupTableAddr solana.PublicKey
	if env.OfframpLookupTable != nil {
		for k := range env.OfframpLookupTable {
			lookupTableAddr = k
		}
	}

	ixInit, err := ccip_offramp.NewInitializeInstruction(
		config.OfframpReferenceAddressesPDA,
		config.CcipRouterProgram,
		config.FeeQuoterProgram,
		config.RMNRemoteProgram,
		lookupTableAddr,
		Admin.PublicKey(),
		solana.SystemProgramID,
		config.CcipOfframpProgram,
		offrampProgramDataAddress,
	)
	require.NoError(t, err)

	ixConfig, err := ccip_offramp.NewInitializeConfigInstruction(
		svmChainSelector,
		config.OfframpConfigPDA,
		Admin.PublicKey(),
		solana.SystemProgramID,
		config.CcipOfframpProgram,
		offrampProgramDataAddress,
	)
	require.NoError(t, err)

	SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ixInit, ixConfig}, Admin, config.DefaultCommitment)
}

// setupTrafficConfig initializes Router, FeeQuoter, Offramp with chain selectors,
// transfers ownership to CcipAdmin, adds billing token configs, registers token admin
// registries for all pools, configures per-chain remote tokens and billing, and sets OCR config.
func (env *TestEnv) setupTrafficConfig(ctx context.Context, withCCTPv2 bool) {
	t := env.T
	t.Helper()

	defaultMaxFeeJuelsPerMsg := bin.Uint128{Lo: 300000000000000000, Hi: 0, Endianness: nil}

	// ---- Initialize Router ---- //
	env.InitRouter(config.SvmChainSelector, FeeAggregator.PublicKey(), env.Link22.Mint)

	// ---- Initialize FeeQuoter ---- //
	env.InitFeeQuoter(defaultMaxFeeJuelsPerMsg, env.Link22.Mint)

	// ---- Initialize Offramp ---- //
	env.InitOfframp(config.SvmChainSelector)

	// ---- Add offramp as FQ price updater ---- //
	ixPriceUpdater, err := fee_quoter.NewAddPriceUpdaterInstruction(
		config.OfframpBillingSignerPDA,
		config.FqAllowedPriceUpdaterOfframpPDA,
		config.FqConfigPDA,
		Admin.PublicKey(),
		solana.SystemProgramID,
	)
	require.NoError(t, err)

	// ---- Add Admin as FQ price updater ---- //
	// Stands in for the offchain price updater so the baseline can seed lane gas prices
	// directly; see SetLaneGasPrice.
	adminPriceUpdaterPDA, _, err := state.FindFqAllowedPriceUpdaterPDA(Admin.PublicKey(), config.FeeQuoterProgram)
	require.NoError(t, err)
	ixAdminPriceUpdater, err := fee_quoter.NewAddPriceUpdaterInstruction(
		Admin.PublicKey(),
		adminPriceUpdaterPDA,
		config.FqConfigPDA,
		Admin.PublicKey(),
		solana.SystemProgramID,
	)
	require.NoError(t, err)
	SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ixPriceUpdater, ixAdminPriceUpdater}, Admin, config.DefaultCommitment)

	// ---- Add offramp permission on Router ---- //
	ixOfframp, err := ccip_router.NewAddOfframpInstruction(
		config.EvmChainSelector, config.CcipOfframpProgram, config.AllowedOfframpEvmPDA, config.RouterConfigPDA, Admin.PublicKey(), solana.SystemProgramID,
	)
	require.NoError(t, err)
	SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ixOfframp}, Admin, config.DefaultCommitment)

	// ---- Add chain selectors ---- //
	// Router: EVM + SVM dest chains
	ixRouterEvm, err := ccip_router.NewAddChainSelectorInstruction(config.EvmChainSelector, ccip_router.DestChainConfig{}, config.EvmDestChainStatePDA, config.RouterConfigPDA, Admin.PublicKey(), solana.SystemProgramID)
	require.NoError(t, err)
	ixRouterSvm, err := ccip_router.NewAddChainSelectorInstruction(config.SvmChainSelector, ccip_router.DestChainConfig{}, config.SvmDestChainStatePDA, config.RouterConfigPDA, Admin.PublicKey(), solana.SystemProgramID)
	require.NoError(t, err)
	// CCIP v2.0 lane: a DestChainCcipV2 account that ccip_send_v2 targets. The baseline carries a
	// single default CCV (the committee verifier), a non-empty off-ramp address, and the network
	// fees / base execution gas that the fee receipts are computed from. Individual tests override
	// the lane via update_dest_chain_config_v2.
	ixRouterEvmV2, err := ccip_router.NewAddDestChainV2Instruction(
		ccip_router.AddDestChainV2Params{
			NewChainSelector: config.EvmChainSelector,
			DestChainConfig:  DefaultV2DestChainConfig(),
		},
		config.EvmDestChainStatePDAV2,
		config.RouterConfigPDA, Admin.PublicKey(), solana.SystemProgramID,
		config.RouterEventAuthorityPDA, config.CcipRouterProgram,
	)
	require.NoError(t, err)
	SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ixRouterEvm, ixRouterSvm, ixRouterEvmV2}, Admin, config.DefaultCommitment)

	// FQ: EVM + SVM dest chains
	ixFqEvm, err := fee_quoter.NewAddDestChainInstruction(config.EvmChainSelector, env.ValidFqDestChainConfig, config.FqConfigPDA, config.FqEvmDestChainPDA, Admin.PublicKey(), solana.SystemProgramID)
	require.NoError(t, err)
	ixFqSvm, err := fee_quoter.NewAddDestChainInstruction(config.SvmChainSelector, fee_quoter.DestChainConfig{
		IsEnabled: true, DefaultTxGasLimit: fee_quoter.CrossChainGas{V0: 1}, MaxPerMsgGasLimit: fee_quoter.CrossChainGas{V0: 100},
		ChainFamilySelector: [4]uint8(config.SvmChainFamilySelector), EnforceOutOfOrder: true,
		// 500, not 100: the fee quoter now charges the expanded SVM payload (accounts + per-token
		// static overhead) against MaxDataBytes, not just msg.data.
		MaxNumberOfTokensPerMsg: 10, MaxDataBytes: 500,
	}, config.FqConfigPDA, config.FqSvmDestChainPDA, Admin.PublicKey(), solana.SystemProgramID)
	require.NoError(t, err)
	// FQ CCIP 2.0 lane: the migrated dest chain account that every 2.0 fee view is derived from
	// (the 1.6 `dest_chain` account above stays in place for the legacy flows).
	ixFqEvmV2, err := fee_quoter.NewUpsertDestChainConfigV2Instruction(
		fee_quoter.UpsertDestChainConfigV2Params{
			ChainSelector:   config.EvmChainSelector,
			DestChainConfig: env.ValidFqDestChainConfigV2,
		},
		config.FqConfigPDA,
		config.FqEvmDestChainV2PDA,
		Admin.PublicKey(),
		solana.SystemProgramID,
		config.FqEventAuthorityPDA,
		config.FeeQuoterProgram,
	)
	require.NoError(t, err)
	SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ixFqEvm, ixFqSvm, ixFqEvmV2}, Admin, config.DefaultCommitment)

	// Lane gas prices: the 1.6 chain state starts with an unset (zero-timestamp) gas price, and
	// both the 1.6 fee path and quote_gas_for_exec_v2 read their price from there and refuse to
	// quote against an unset one, so every fee path to a lane fails until a price is written.
	env.SetLaneGasPrice(ctx, Admin, config.EvmChainSelector, LaneUsdPerUnitGas)
	env.SetLaneGasPrice(ctx, Admin, config.SvmChainSelector, LaneUsdPerUnitGas)

	// Offramp: EVM + SVM source chains
	ixOfframpEvm, err := ccip_offramp.NewAddSourceChainInstruction(
		ccip_offramp.AddSourceChainParams{
			NewChainSelector:  config.EvmChainSelector,
			SourceChainConfig: env.ValidSourceChainConfig,
		},
		config.OfframpEvmSourceChainPDA,
		config.OfframpConfigPDA,
		Admin.PublicKey(),
		solana.SystemProgramID,
		config.OfframpEventAuthorityPDA,
		config.CcipOfframpProgram,
	)
	require.NoError(t, err)
	paddedCcipRouterProgram := common.ToPadded64Bytes(config.CcipRouterProgram.Bytes())
	ixOfframpSvm, err := ccip_offramp.NewAddSourceChainInstruction(
		ccip_offramp.AddSourceChainParams{
			NewChainSelector: config.SvmChainSelector,
			SourceChainConfig: ccip_offramp.SourceChainConfig{
				OnRamps:             []ccip_offramp.OnRampAddress{{Bytes: paddedCcipRouterProgram, Len: 32}},
				IsEnabled:           true,
				DefaultCcvs:         []solana.PublicKey{config.CommitteeVerifierProgram},
				LaneMandatedCcvs:    []solana.PublicKey{},
				LegacyAddressLength: 32, // solana-to-solana uses 32 bytes always
			},
		},
		config.OfframpSvmSourceChainPDA,
		config.OfframpConfigPDA,
		Admin.PublicKey(),
		solana.SystemProgramID,
		config.OfframpEventAuthorityPDA,
		config.CcipOfframpProgram,
	)
	require.NoError(t, err)
	SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ixOfframpEvm, ixOfframpSvm}, Admin, config.DefaultCommitment)

	// ---- Transfer ownership to ccipAdmin ---- //
	env.transferAllOwnership(ctx)

	// ---- Add billing token configs (wsol + link22) ---- //
	env.setupBillingTokenConfigs(ctx)

	// ---- Token admin registry for all pools ---- //
	env.setupTokenAdminRegistries(ctx)

	// ---- Per-chain pool configuration ---- //
	env.setupPerChainPoolConfig(ctx, withCCTPv2)

	// ---- Executor (the v2 lane's default executor) ---- //
	env.setupExecutor(ctx)
}

func (env *TestEnv) setupExecutor(ctx context.Context) {
	t := env.T
	t.Helper()

	execProgramDataAddress := GetProgramDataAddress(ctx, t, env.Client, config.ExecutorProgram)

	// Like the committee verifier (see setupCV), the executor's get_fee gates the requested
	// finality against the config's allowed_finality_config, so permit the flag-only fast
	// finality (Flags:1) the onramp send tests exercise; wait-for-finality requests remain
	// allowed regardless.
	ixInit, err := executor.NewInitializeInstruction(
		executor.InitializeParams{
			MaxCcvsPerMsg:         5,
			FeeAggregator:         FeeAggregator.PublicKey(),
			AllowedFinalityConfig: executor.FinalityConfig{Flags: 1},
			CcvAllowlistEnabled:   false,
		},
		config.ExecConfigPDA,
		Admin.PublicKey(),
		solana.SystemProgramID,
		config.ExecEventAuthorityPDA,
		config.ExecutorProgram,
		execProgramDataAddress,
	)
	require.NoError(t, err)
	SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ixInit}, Admin, config.DefaultCommitment)

	// Chain config for the v2.0 EVM lane: the onramp's executor get_fee CPI loads it and
	// requires it to be enabled. The baseline fee is zero so the fee tests' receipt sums stay
	// easy to derive; the non-zero fee path is covered by a dedicated onramp subtest.
	resolvedChainConfig, err := ccip.ResolveExecSetChainConfigAccounts(ctx, env.Client, config.ExecutorProgram, executor.SetChainConfigParams{
		RemoteChainSelector: config.EvmChainSelector,
		UsdCentsFee:         executor.UsdCents{V0: 0},
		Enabled:             true,
	}, Admin.PublicKey(), logger.Test(t))
	require.NoError(t, err)
	SendAndConfirm(ctx, t, env.Client, []solana.Instruction{resolvedChainConfig.Ix}, Admin, config.DefaultCommitment)
}

func (env *TestEnv) transferAllOwnership(ctx context.Context) {
	t := env.T
	t.Helper()

	// Router
	ixTransfer, err := ccip_router.NewTransferOwnershipInstruction(CcipAdmin.PublicKey(), config.RouterConfigPDA, Admin.PublicKey())
	require.NoError(t, err)
	SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ixTransfer}, Admin, config.DefaultCommitment)
	ixAccept, err := ccip_router.NewAcceptOwnershipInstruction(config.RouterConfigPDA, CcipAdmin.PublicKey())
	require.NoError(t, err)
	SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ixAccept}, CcipAdmin, config.DefaultCommitment)

	// FeeQuoter
	ixTransfer, err = fee_quoter.NewTransferOwnershipInstruction(CcipAdmin.PublicKey(), config.FqConfigPDA, Admin.PublicKey())
	require.NoError(t, err)
	SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ixTransfer}, Admin, config.DefaultCommitment)
	ixAccept, err = fee_quoter.NewAcceptOwnershipInstruction(config.FqConfigPDA, CcipAdmin.PublicKey())
	require.NoError(t, err)
	SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ixAccept}, CcipAdmin, config.DefaultCommitment)

	// Offramp
	ixTransfer2, err := ccip_offramp.NewTransferOwnershipInstruction(CcipAdmin.PublicKey(), config.OfframpConfigPDA, Admin.PublicKey(), config.OfframpEventAuthorityPDA, config.CcipOfframpProgram)
	require.NoError(t, err)
	SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ixTransfer2}, Admin, config.DefaultCommitment)
	ixAccept2, err := ccip_offramp.NewAcceptOwnershipInstruction(config.OfframpConfigPDA, CcipAdmin.PublicKey(), config.OfframpEventAuthorityPDA, config.CcipOfframpProgram)
	require.NoError(t, err)
	SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ixAccept2}, CcipAdmin, config.DefaultCommitment)

	// RMN
	ixTransfer3, err := rmn_remote.NewTransferOwnershipInstruction(CcipAdmin.PublicKey(), config.RMNRemoteConfigPDA, config.RMNRemoteCursesPDA, Admin.PublicKey())
	require.NoError(t, err)
	SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ixTransfer3}, Admin, config.DefaultCommitment)
	ixAccept3, err := rmn_remote.NewAcceptOwnershipInstruction(config.RMNRemoteConfigPDA, CcipAdmin.PublicKey())
	require.NoError(t, err)
	SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ixAccept3}, CcipAdmin, config.DefaultCommitment)
}

// SetLaneGasPrice writes the destination gas price of a lane through the 1.6 `update_prices`
// instruction, which is what the offchain price updater calls. The price lands on the 1.6
// `dest_chain` account, and both the 1.6 fee path and quote_gas_for_exec_v2 read it from there
// (the 2.0 chain state has no price of its own until a 2.0 price updater exists). The price is
// 18-decimals USD per single unit of destination gas; quote_gas_for_exec_v2 multiplies it by
// the corrected total gas of the message to get the execution cost.
//
// `priceUpdater` must be registered as an allowed price updater; `setupTrafficConfig` registers
// `Admin` for exactly this.
func (env *TestEnv) SetLaneGasPrice(ctx context.Context, priceUpdater solana.PrivateKey, chainSelector uint64, usdPerUnitGas *big.Int) {
	t := env.T
	t.Helper()

	destChainPDA, _, err := state.FindFqDestChainPDA(chainSelector, config.FeeQuoterProgram)
	require.NoError(t, err)
	allowedPriceUpdaterPDA, _, err := state.FindFqAllowedPriceUpdaterPDA(priceUpdater.PublicKey(), config.FeeQuoterProgram)
	require.NoError(t, err)

	packedPrice := [28]uint8{}
	usdPerUnitGas.FillBytes(packedPrice[:])

	raw, err := fee_quoter.NewUpdatePricesInstruction(
		[]fee_quoter.TokenPriceUpdate{},
		[]fee_quoter.GasPriceUpdate{
			{
				DestChainSelector: chainSelector,
				UsdPerUnitGas:     packedPrice,
			},
		},
		priceUpdater.PublicKey(),
		allowedPriceUpdaterPDA,
		config.FqConfigPDA,
	)
	require.NoError(t, err)

	// update_prices takes the accounts it writes as remaining accounts, in update order.
	generic, ok := raw.(*solana.GenericInstruction)
	require.True(t, ok)
	generic.AccountValues = append(generic.AccountValues, solana.Meta(destChainPDA).WRITE())

	SendAndConfirm(ctx, t, env.Client, []solana.Instruction{raw}, priceUpdater, config.DefaultCommitment)
}

func (env *TestEnv) setupBillingTokenConfigs(ctx context.Context) {
	t := env.T
	t.Helper()

	validTimestamp := int64(100)

	bigValue := [28]uint8{}
	Link22UsdPrice.FillBytes(bigValue[:])

	smallValue := [28]uint8{}
	WsolUsdPrice.FillBytes(smallValue[:])

	ixWsol, err := fee_quoter.NewAddBillingTokenConfigInstruction(
		fee_quoter.BillingTokenConfig{
			Enabled: true, Mint: solana.WrappedSol,
			UsdPerToken: fee_quoter.TimestampedPackedU224{Value: smallValue, Timestamp: validTimestamp},
			// 1e18 == 100%, matching the fee quoter's own sample_billing_config fixture (which
			// documents its values as "derived from inspecting the Ethereum Mainnet -> Avalanche
			// lane", where LINK's premiumMultiplierWeiPerEth is exactly this 9e17) — so this is
			// the scale production configs use. The 1.6 fee path multiplies its USD premium by
			// this, so it has to be scaled against WsolUsdPrice; the 2.0 flat fees don't use it
			// at all.
			PremiumMultiplierWeiPerEth: 900_000_000_000_000_000, // 90%
		},
		config.FqConfigPDA, env.WSOL.FqBillingCfgPDA, env.WSOL.Program, env.WSOL.Mint, env.WSOL.BillingATA,
		CcipAdmin.PublicKey(), config.BillingSignerPDA, tokens.AssociatedTokenProgramID, solana.SystemProgramID,
	)
	require.NoError(t, err)

	ixLink, err := fee_quoter.NewAddBillingTokenConfigInstruction(
		fee_quoter.BillingTokenConfig{
			Enabled: true, Mint: env.Link22.Mint,
			UsdPerToken:                fee_quoter.TimestampedPackedU224{Value: bigValue, Timestamp: validTimestamp},
			PremiumMultiplierWeiPerEth: 1_100_000_000_000_000_000, // 110%
		},
		config.FqConfigPDA, env.Link22.FqBillingCfgPDA, env.Link22.Program, env.Link22.Mint, env.Link22.BillingATA,
		CcipAdmin.PublicKey(), config.BillingSignerPDA, tokens.AssociatedTokenProgramID, solana.SystemProgramID,
	)
	require.NoError(t, err)

	SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ixWsol}, CcipAdmin, config.DefaultCommitment)
	SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ixLink}, CcipAdmin, config.DefaultCommitment)
}

func (env *TestEnv) setupTokenAdminRegistries(ctx context.Context) {
	t := env.T
	t.Helper()

	type poolReg struct {
		pool      tokens.TokenPool
		poolAdmin solana.PrivateKey
	}
	// token2 deliberately omitted: no admin registry in original test
	pools := []poolReg{
		{env.Token0, Token0Admin},
		{env.Token1, Token1Admin},
		{env.LinkPool, Admin},
		{env.UsdcPool, UsdcPoolAdmin},
	}

	for _, pr := range pools {
		// Propose admin
		ixPropose, err := ccip_router.NewCcipAdminProposeAdministratorInstruction(
			pr.poolAdmin.PublicKey(), config.RouterConfigPDA, pr.pool.AdminRegistryPDA, pr.pool.Mint,
			CcipAdmin.PublicKey(), solana.SystemProgramID,
		)
		require.NoError(t, err)
		SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ixPropose}, CcipAdmin, config.DefaultCommitment)

		// Accept admin
		ixAccept, err := ccip_router.NewAcceptAdminRoleTokenAdminRegistryInstruction(
			config.RouterConfigPDA, pr.pool.AdminRegistryPDA, pr.pool.Mint, pr.poolAdmin.PublicKey(),
		)
		require.NoError(t, err)
		SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ixAccept}, pr.poolAdmin, config.DefaultCommitment)

		// Set pool lookup table
		ixSetPool, err := ccip_router.NewSetPoolInstruction(
			pr.pool.WritableIndexes, config.RouterConfigPDA, pr.pool.AdminRegistryPDA, pr.pool.Mint,
			pr.pool.PoolLookupTable, pr.poolAdmin.PublicKey(),
		)
		require.NoError(t, err)
		SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ixSetPool}, pr.poolAdmin, config.DefaultCommitment)

		// Set pool interface (account resolution). The router only takes WithoutExtendedLookupTable
		// for a v2 pool.
		ixInterface, err := ccip_router.NewSetPoolInterfaceImplementationInstruction(
			ccip_router.SetPoolInterfaceImplementationParams{
				Mint: pr.pool.Mint, InterfaceVersion: 2, AccountResolution: ccip_router.ResolutionType_WithoutExtendedLookupTable,
			},
			config.RouterConfigPDA, pr.pool.AdminRegistryPDA, pr.poolAdmin.PublicKey(),
			config.RouterEventAuthorityPDA, config.CcipRouterProgram,
		)
		require.NoError(t, err)
		SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ixInterface}, pr.poolAdmin, config.DefaultCommitment)
	}

	// Transfer mint authorities for token1 and linkPool to their pool signers
	ixAuth1, err := tokens.SetTokenMintAuthority(env.Token1.Program, env.Token1.PoolSigner, env.Token1.Mint, Token1Admin.PublicKey())
	require.NoError(t, err)
	SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ixAuth1}, Token1Admin, config.DefaultCommitment)

	ixAuthLink, err := tokens.SetTokenMintAuthority(env.LinkPool.Program, env.LinkPool.PoolSigner, env.LinkPool.Mint, Admin.PublicKey())
	require.NoError(t, err)
	SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ixAuthLink}, Admin, config.DefaultCommitment)
}

func (env *TestEnv) setupPerChainPoolConfig(ctx context.Context, withCCTPv2 bool) {
	t := env.T
	t.Helper()

	evmTokenDecimals := uint8(18)

	// Remote chain configs for burnmint pools (EVM + SVM). The v2.0 EVM lane reuses the v1 EVM
	// selector, so its per-chain pool configs are the ones set up here for config.EvmChainSelector.
	for _, selector := range []uint64{config.EvmChainSelector, config.SvmChainSelector} {
		ix0, err := burnmint_tokenpool.NewInitChainRemoteConfigInstruction(selector, env.Token0.Mint, burnmint_tokenpool.RemoteConfig{
			TokenAddress: burnmint_tokenpool.RemoteAddress{Address: config.EVMToken0AddressBytes}, Decimals: evmTokenDecimals,
		}, env.Token0.PoolConfig, env.Token0.Chain[selector], Token0Admin.PublicKey(), solana.SystemProgramID)
		require.NoError(t, err)
		ix1, err := burnmint_tokenpool.NewInitChainRemoteConfigInstruction(selector, env.Token1.Mint, burnmint_tokenpool.RemoteConfig{
			PoolAddresses: []burnmint_tokenpool.RemoteAddress{}, TokenAddress: burnmint_tokenpool.RemoteAddress{Address: config.EVMToken1AddressBytes}, Decimals: evmTokenDecimals,
		}, env.Token1.PoolConfig, env.Token1.Chain[selector], Token1Admin.PublicKey(), solana.SystemProgramID)
		require.NoError(t, err)
		ix2, err := burnmint_tokenpool.NewInitChainRemoteConfigInstruction(selector, env.Token2.Mint, burnmint_tokenpool.RemoteConfig{
			PoolAddresses: []burnmint_tokenpool.RemoteAddress{}, TokenAddress: burnmint_tokenpool.RemoteAddress{Address: config.EVMToken2AddressBytes}, Decimals: evmTokenDecimals,
		}, env.Token2.PoolConfig, env.Token2.Chain[selector], Token2Admin.PublicKey(), solana.SystemProgramID)
		require.NoError(t, err)
		ixLink, err := burnmint_tokenpool.NewInitChainRemoteConfigInstruction(selector, env.LinkPool.Mint, burnmint_tokenpool.RemoteConfig{
			PoolAddresses: []burnmint_tokenpool.RemoteAddress{}, TokenAddress: burnmint_tokenpool.RemoteAddress{Address: config.EVMToken3AddressBytes}, Decimals: evmTokenDecimals,
		}, env.LinkPool.PoolConfig, env.LinkPool.Chain[selector], Admin.PublicKey(), solana.SystemProgramID)
		require.NoError(t, err)

		poolAddresses := []burnmint_tokenpool.RemoteAddress{{Address: config.EVMRemotePoolAddressBytesTrim}}
		ix3, err := burnmint_tokenpool.NewAppendRemotePoolAddressesInstruction(selector, env.Token1.Mint, poolAddresses, env.Token1.PoolConfig, env.Token1.Chain[selector], Token1Admin.PublicKey(), solana.SystemProgramID)
		require.NoError(t, err)
		ix4, err := burnmint_tokenpool.NewAppendRemotePoolAddressesInstruction(selector, env.Token2.Mint, poolAddresses, env.Token2.PoolConfig, env.Token2.Chain[selector], Token2Admin.PublicKey(), solana.SystemProgramID)
		require.NoError(t, err)

		SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ix0, ix1, ix2}, Token0Admin, config.DefaultCommitment, common.AddSigners(Token1Admin, Token2Admin, Admin))
		SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ix3, ix4, ixLink}, Token0Admin, config.DefaultCommitment, common.AddSigners(Token1Admin, Token2Admin, Admin))
	}

	// Append remote pool addresses for token0
	ixEvm, err := burnmint_tokenpool.NewAppendRemotePoolAddressesInstruction(config.EvmChainSelector, env.Token0.Mint, []burnmint_tokenpool.RemoteAddress{{Address: config.EVMRemotePoolAddressBytesTrim}},
		env.Token0.PoolConfig, env.Token0.Chain[config.EvmChainSelector], Token0Admin.PublicKey(), solana.SystemProgramID)
	require.NoError(t, err)
	ixSvm, err := burnmint_tokenpool.NewAppendRemotePoolAddressesInstruction(config.SvmChainSelector, env.Token0.Mint, []burnmint_tokenpool.RemoteAddress{{Address: []byte{1, 2, 3}}},
		env.Token0.PoolConfig, env.Token0.Chain[config.SvmChainSelector], Token0Admin.PublicKey(), solana.SystemProgramID)
	require.NoError(t, err)
	SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ixEvm, ixSvm}, Token0Admin, config.DefaultCommitment)

	// Rate limits
	ix0, err := burnmint_tokenpool.NewSetChainRateLimitInstruction(config.EvmChainSelector, env.Token0.Mint, burnmint_tokenpool.RateLimitConfig{}, burnmint_tokenpool.RateLimitConfig{}, env.Token0.PoolConfig, env.Token0.Chain[config.EvmChainSelector], Token0Admin.PublicKey())
	require.NoError(t, err)
	ix1, err := burnmint_tokenpool.NewSetChainRateLimitInstruction(config.EvmChainSelector, env.Token1.Mint, burnmint_tokenpool.RateLimitConfig{}, burnmint_tokenpool.RateLimitConfig{}, env.Token1.PoolConfig, env.Token1.Chain[config.EvmChainSelector], Token1Admin.PublicKey())
	require.NoError(t, err)
	ix2, err := burnmint_tokenpool.NewSetChainRateLimitInstruction(config.EvmChainSelector, env.Token2.Mint, burnmint_tokenpool.RateLimitConfig{}, burnmint_tokenpool.RateLimitConfig{}, env.Token2.PoolConfig, env.Token2.Chain[config.EvmChainSelector], Token2Admin.PublicKey())
	require.NoError(t, err)
	SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ix0, ix1, ix2}, Token0Admin, config.DefaultCommitment, common.AddSigners(Token1Admin, Token2Admin))

	// The v2.0 EVM lane reuses the v1 EVM selector, so it shares the rate limits set above; no
	// separate v2 rate-limit setup is needed.

	// Per-chain-per-token billing in FQ
	defaultFeeConfig := fee_quoter.TokenTransferFeeConfig{DestBytesOverhead: 32, MinFee: fee_quoter.UsdCents{V0: 0}, MaxFee: fee_quoter.UsdCents{V0: 1}}
	ixBilling0, err := fee_quoter.NewSetTokenTransferFeeConfigInstruction(config.EvmChainSelector, env.Token0.Mint, defaultFeeConfig, config.FqConfigPDA, env.Token0.Billing[config.EvmChainSelector], CcipAdmin.PublicKey(), solana.SystemProgramID)
	require.NoError(t, err)
	ixBilling1, err := fee_quoter.NewSetTokenTransferFeeConfigInstruction(config.EvmChainSelector, env.Token1.Mint, defaultFeeConfig, config.FqConfigPDA, env.Token1.Billing[config.EvmChainSelector], CcipAdmin.PublicKey(), solana.SystemProgramID)
	require.NoError(t, err)
	ixBilling2, err := fee_quoter.NewSetTokenTransferFeeConfigInstruction(config.SvmChainSelector, env.Token0.Mint, defaultFeeConfig, config.FqConfigPDA, env.Token0.Billing[config.SvmChainSelector], CcipAdmin.PublicKey(), solana.SystemProgramID)
	require.NoError(t, err)
	ixBilling3, err := fee_quoter.NewSetTokenTransferFeeConfigInstruction(config.SvmChainSelector, env.Token1.Mint, defaultFeeConfig, config.FqConfigPDA, env.Token1.Billing[config.SvmChainSelector], CcipAdmin.PublicKey(), solana.SystemProgramID)
	require.NoError(t, err)
	SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ixBilling0, ixBilling1, ixBilling2, ixBilling3}, CcipAdmin, config.DefaultCommitment)

	// Update token0 billing with DestGasOverhead=100 (matching original test state before ccipSend)
	token0BillingUpdate := fee_quoter.TokenTransferFeeConfig{
		MinFee: fee_quoter.UsdCents{V0: 800}, MaxFee: fee_quoter.UsdCents{V0: 1600}, DeciBps: 0,
		DestGasOverhead: fee_quoter.CrossChainGas{V0: 100}, DestBytesOverhead: 100, IsEnabled: true,
	}
	ixBillingUpdate, err := fee_quoter.NewSetTokenTransferFeeConfigInstruction(config.EvmChainSelector, env.Token0.Mint, token0BillingUpdate, config.FqConfigPDA, env.Token0.Billing[config.EvmChainSelector], CcipAdmin.PublicKey(), solana.SystemProgramID)
	require.NoError(t, err)
	SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ixBillingUpdate}, CcipAdmin, config.DefaultCommitment)

	if withCCTPv2 {
		// CCTP pool per-chain config
		usdcDecimals := uint8(6)
		initUsdcIx, err := cctp_token_pool.NewInitChainRemoteConfigInstruction(config.EvmChainSelector, env.UsdcPool.Mint, cctp_token_pool.RemoteConfig{
			TokenAddress: cctp_token_pool.RemoteAddress{Address: config.EVMUsdcAddressBytes}, Decimals: usdcDecimals,
		}, env.UsdcPool.PoolConfig, env.UsdcPool.Chain[config.EvmChainSelector], UsdcPoolAdmin.PublicKey(), solana.SystemProgramID)
		require.NoError(t, err)

		editUsdcIx, err := cctp_token_pool.NewEditChainRemoteConfigCctpInstruction(
			config.EvmChainSelector, env.UsdcPool.Mint,
			cctp_token_pool.CctpChain{DomainId: env.CctpEvmDomain, DestinationCaller: solana.PublicKey(config.EVMCctpMessageTransmitterProxy)},
			env.UsdcPool.PoolConfig, env.UsdcPool.Chain[config.EvmChainSelector], UsdcPoolAdmin.PublicKey(),
		)
		require.NoError(t, err)

		rlUsdcIx, err := cctp_token_pool.NewSetChainRateLimitInstruction(config.EvmChainSelector, env.UsdcPool.Mint, cctp_token_pool.RateLimitConfig{}, cctp_token_pool.RateLimitConfig{}, env.UsdcPool.PoolConfig, env.UsdcPool.Chain[config.EvmChainSelector], UsdcPoolAdmin.PublicKey())
		require.NoError(t, err)

		appendUsdcRemotePoolIx, err := cctp_token_pool.NewAppendRemotePoolAddressesInstruction(config.EvmChainSelector, env.UsdcPool.Mint, []cctp_token_pool.RemoteAddress{{Address: config.EVMUsdcPoolAddressBytesTrim}}, env.UsdcPool.PoolConfig, env.UsdcPool.Chain[config.EvmChainSelector], UsdcPoolAdmin.PublicKey(), solana.SystemProgramID)
		require.NoError(t, err)

		chainV2Ix, err := cctp_token_pool.NewInitChainConfigV2Instruction(
			cctp_token_pool.SetChainConfigV2Params{
				Mint:                  env.UsdcPool.Mint,
				RemoteChainSelector:   config.EvmChainSelector,
				AllowedFinalityConfig: cctp_token_pool.FinalityConfig{BlockDepth: 3},
				TokenTransferFeeConfig: cctp_token_pool.TokenTransferFeeConfig{
					FinalityFee:       cctp_token_pool.UsdCents{V0: 1},
					FastFinalityFee:   cctp_token_pool.UsdCents{V0: 2},
					DestBytesOverhead: 32,
				},
				CcvConfig: cctp_token_pool.CcvConfig{
					// this same pool program implements the verifier interface for CCTPv2 integration
					OutboundCcvs: []solana.PublicKey{config.CctpTokenPoolProgram},
					InboundCcvs:  []solana.PublicKey{config.CctpTokenPoolProgram},
				},
			},
			env.UsdcPool.PoolConfig, env.UsdcPool.ChainConfigV2[config.EvmChainSelector], UsdcPoolAdmin.PublicKey(),
			solana.SystemProgramID, env.UsdcPool.EventAuthority, env.UsdcPool.PoolProgram,
		)
		require.NoError(t, err)

		SendAndConfirm(ctx, t, env.Client,
			[]solana.Instruction{initUsdcIx, editUsdcIx, rlUsdcIx, appendUsdcRemotePoolIx, chainV2Ix},
			UsdcPoolAdmin, config.DefaultCommitment)
	}
}
