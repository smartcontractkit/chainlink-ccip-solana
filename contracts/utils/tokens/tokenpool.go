package tokens

import (
	"context"
	"fmt"
	"strings"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/ccip_common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/config"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
)

type TokenPool struct {
	// token details
	Program               solana.PublicKey
	Mint                  solana.PublicKey
	FeeTokenConfig        solana.PublicKey
	MintAuthorityMultisig solana.PublicKey

	// admin registry PDA
	AdminRegistryPDA solana.PublicKey

	// pool details
	PoolProgram, PoolConfig, PoolSigner, PoolTokenAccount solana.PublicKey
	PoolLookupTable                                       solana.PublicKey
	WritableIndexes                                       []uint8

	GlobalConfig solana.PublicKey // Global Config PDA of the Token Pool

	EventAuthority solana.PublicKey

	AdditionalAccounts solana.PublicKeySlice

	// AssociatedTokenAddress Lookups
	User map[solana.PublicKey]solana.PublicKey

	// remote chain config lookup (ChainConfig)
	Chain map[uint64]solana.PublicKey

	// remote chain config lookup (ChainConfig V2)
	ChainConfigV2 map[uint64]solana.PublicKey

	// remote chain config override lookup (e.g. FTF rate limits)
	ChainOverride map[uint64]solana.PublicKey

	// per-remote-chain token transfer fee config lookup
	TokenTransferFeeConfig map[uint64]solana.PublicKey

	// billing config lookup
	Billing map[uint64]solana.PublicKey

	// CCIP CPI signers
	RouterSigner  solana.PublicKey
	OfframpSigner solana.PublicKey

	// Token Extensions
	WithTokenExtensions bool
}

func (tp TokenPool) ToTokenPoolEntries() []solana.PublicKey {
	list := make(solana.PublicKeySlice, 0, 10+len(tp.AdditionalAccounts))
	list = append(list,
		tp.PoolLookupTable,  // 0
		tp.AdminRegistryPDA, // 1
		tp.PoolProgram,      // 2
		tp.PoolConfig,       // 3
		tp.PoolTokenAccount, // 4 - writable
		tp.PoolSigner,       // 5
		tp.Program,          // 6
		tp.Mint,             // 7 - writable
		tp.FeeTokenConfig,   // 8
		tp.RouterSigner,     // 9
	)
	return append(list, tp.AdditionalAccounts...)
}

// NewTokenPool returns token + pool addresses. however, the token still needs to be deployed
func NewTokenPool(tokenProgram solana.PublicKey, poolProgram solana.PublicKey, mint solana.PublicKey) (TokenPool, error) {
	tokenAdminRegistryPDA, _, err := state.FindTokenAdminRegistryPDA(mint, config.CcipRouterProgram)
	if err != nil {
		return TokenPool{}, err
	}
	// preload with defined config.EvmChainSelector
	evmChainConfigPDA, _, err := TokenPoolChainConfigPDA(config.EvmChainSelector, mint, poolProgram)
	if err != nil {
		return TokenPool{}, err
	}
	evmChainConfigV2PDA, _, err := TokenPoolChainConfigV2PDA(config.EvmChainSelector, mint, poolProgram)
	if err != nil {
		return TokenPool{}, err
	}
	svmChainConfigPDA, _, err := TokenPoolChainConfigPDA(config.SvmChainSelector, mint, poolProgram)
	if err != nil {
		return TokenPool{}, err
	}
	svmChainConfigV2PDA, _, err := TokenPoolChainConfigV2PDA(config.SvmChainSelector, mint, poolProgram)
	if err != nil {
		return TokenPool{}, err
	}
	evmChainOverridePDA, _, err := TokenPoolChainConfigOverridePDA(config.EvmChainSelector, mint, poolProgram, []byte("ftf"))
	if err != nil {
		return TokenPool{}, err
	}
	svmChainOverridePDA, _, err := TokenPoolChainConfigOverridePDA(config.SvmChainSelector, mint, poolProgram, []byte("ftf"))
	if err != nil {
		return TokenPool{}, err
	}
	evmTokenTransferFeeConfigPDA, _, err := TokenPoolTokenTransferFeeConfigPDA(config.EvmChainSelector, mint, poolProgram)
	if err != nil {
		return TokenPool{}, err
	}
	svmTokenTransferFeeConfigPDA, _, err := TokenPoolTokenTransferFeeConfigPDA(config.SvmChainSelector, mint, poolProgram)
	if err != nil {
		return TokenPool{}, err
	}
	evmBillingPDA, _, err := state.FindFqPerChainPerTokenConfigPDA(config.EvmChainSelector, mint, config.FeeQuoterProgram)
	if err != nil {
		return TokenPool{}, err
	}
	svmBillingPDA, _, err := state.FindFqPerChainPerTokenConfigPDA(config.SvmChainSelector, mint, config.FeeQuoterProgram)
	if err != nil {
		return TokenPool{}, err
	}
	tokenConfigPda, _, err := state.FindFqBillingTokenConfigPDA(mint, config.FeeQuoterProgram)
	if err != nil {
		return TokenPool{}, err
	}
	routerSignerPDA, _, err := state.FindExternalTokenPoolsSignerPDA(poolProgram, config.CcipRouterProgram)
	if err != nil {
		return TokenPool{}, err
	}
	offrampSignerPDA, _, err := state.FindExternalTokenPoolsSignerPDA(poolProgram, config.CcipOfframpProgram)
	if err != nil {
		return TokenPool{}, err
	}
	globalConfigPDA, err := TokenPoolGlobalConfigPDA(poolProgram)
	if err != nil {
		return TokenPool{}, err
	}
	eventAuthority, _, err := state.FindEventAuthorityPDA(poolProgram)
	if err != nil {
		return TokenPool{}, err
	}

	p := TokenPool{
		Program:                tokenProgram,
		Mint:                   mint,
		MintAuthorityMultisig:  solana.PublicKey{}, // this will be set later, if needed
		FeeTokenConfig:         tokenConfigPda,
		AdminRegistryPDA:       tokenAdminRegistryPDA,
		PoolProgram:            poolProgram,
		PoolLookupTable:        solana.PublicKey{},
		WritableIndexes:        []uint8{4, 7}, // see ToTokenPoolEntries for writable indexes.
		User:                   map[solana.PublicKey]solana.PublicKey{},
		Chain:                  map[uint64]solana.PublicKey{},
		ChainConfigV2:          map[uint64]solana.PublicKey{},
		ChainOverride:          map[uint64]solana.PublicKey{},
		TokenTransferFeeConfig: map[uint64]solana.PublicKey{},
		Billing:                map[uint64]solana.PublicKey{},
		RouterSigner:           routerSignerPDA,
		OfframpSigner:          offrampSignerPDA,
		WithTokenExtensions:    false, // default to false, can be set later if needed
		GlobalConfig:           globalConfigPDA,
		EventAuthority:         eventAuthority,
	}
	// The v2.0 EVM lane reuses config.EvmChainSelector, so these entries also serve the v2 lane.
	p.Chain[config.EvmChainSelector] = evmChainConfigPDA
	p.Chain[config.SvmChainSelector] = svmChainConfigPDA
	p.ChainConfigV2[config.EvmChainSelector] = evmChainConfigV2PDA
	p.ChainConfigV2[config.SvmChainSelector] = svmChainConfigV2PDA
	p.ChainOverride[config.EvmChainSelector] = evmChainOverridePDA
	p.ChainOverride[config.SvmChainSelector] = svmChainOverridePDA
	p.TokenTransferFeeConfig[config.EvmChainSelector] = evmTokenTransferFeeConfigPDA
	p.TokenTransferFeeConfig[config.SvmChainSelector] = svmTokenTransferFeeConfigPDA
	p.Billing[config.EvmChainSelector] = evmBillingPDA
	p.Billing[config.SvmChainSelector] = svmBillingPDA
	p.PoolConfig, err = TokenPoolConfigAddress(p.Mint, poolProgram)
	if err != nil {
		return TokenPool{}, err
	}
	p.PoolSigner, err = TokenPoolSignerAddress(p.Mint, poolProgram)
	if err != nil {
		return TokenPool{}, err
	}
	return p, nil
}

func (tp *TokenPool) SetupLookupTable(ctx context.Context, client *rpc.Client, admin solana.PrivateKey) error {
	table, err := common.CreateLookupTable(ctx, client, admin)
	if err != nil {
		return err
	}
	tp.PoolLookupTable = table // the LUT entries will include this, so set it before adding the addresses to the LUT
	if err = common.ExtendLookupTable(ctx, client, table, admin, tp.ToTokenPoolEntries()); err != nil {
		return err
	}
	return common.AwaitSlotChange(ctx, client)
}

func TokenPoolConfigAddress(tokenMint, programID solana.PublicKey) (solana.PublicKey, error) {
	addr, _, err := solana.FindProgramAddress([][]byte{[]byte("ccip_tokenpool_config"), tokenMint.Bytes()}, programID)
	return addr, err
}

func TokenPoolSignerAddress(token, programID solana.PublicKey) (solana.PublicKey, error) {
	addr, _, err := solana.FindProgramAddress([][]byte{[]byte("ccip_tokenpool_signer"), token.Bytes()}, programID)
	return addr, err
}

func TokenPoolChainConfigPDA(chainSelector uint64, mint, programID solana.PublicKey) (solana.PublicKey, uint8, error) {
	chainSelectorLE := common.Uint64ToLE(chainSelector)
	return solana.FindProgramAddress([][]byte{[]byte("ccip_tokenpool_chainconfig"), chainSelectorLE, mint.Bytes()}, programID)
}

func TokenPoolChainConfigV2PDA(chainSelector uint64, mint, programID solana.PublicKey) (solana.PublicKey, uint8, error) {
	chainSelectorLE := common.Uint64ToLE(chainSelector)
	return solana.FindProgramAddress([][]byte{[]byte("ccip_tokenpool_chainconfig_2_0"), chainSelectorLE, mint.Bytes()}, programID)
}

func TokenPoolChainConfigOverridePDA(chainSelector uint64, mint, programID solana.PublicKey, overrideType []byte) (solana.PublicKey, uint8, error) {
	chainSelectorLE := common.Uint64ToLE(chainSelector)
	return solana.FindProgramAddress([][]byte{[]byte("chainconfig_override"), chainSelectorLE, mint.Bytes(), overrideType}, programID)
}

func TokenPoolTokenTransferFeeConfigPDA(chainSelector uint64, mint, programID solana.PublicKey) (solana.PublicKey, uint8, error) {
	chainSelectorLE := common.Uint64ToLE(chainSelector)
	return solana.FindProgramAddress([][]byte{[]byte("token_transfer_fee_config"), chainSelectorLE, mint.Bytes()}, programID)
}

func TokenPoolGlobalConfigPDA(programID solana.PublicKey) (solana.PublicKey, error) {
	addr, _, err := solana.FindProgramAddress([][]byte{[]byte("config")}, programID)
	return addr, err
}

func ParseTokenLookupTable(ctx context.Context, client *rpc.Client, token TokenPool, userTokenAccount solana.PublicKey) (solana.AccountMetaSlice, map[solana.PublicKey]solana.PublicKeySlice, error) {
	return ParseTokenLookupTableWithChain(ctx, client, token, userTokenAccount, config.EvmChainSelector)
}

func ParseTokenLookupTableWithChain(ctx context.Context, client *rpc.Client, token TokenPool, userTokenAccount solana.PublicKey, chainSelector uint64) (solana.AccountMetaSlice, map[solana.PublicKey]solana.PublicKeySlice, error) {
	tokenBillingConfig := token.Billing[chainSelector]
	poolChainConfig := token.Chain[chainSelector]

	tokenAdminRegistry := ccip_common.TokenAdminRegistry{}
	err := common.GetAccountDataBorshIntoAnchor(ctx, client, token.AdminRegistryPDA, config.DefaultCommitment, &tokenAdminRegistry)
	if err != nil {
		return nil, nil, err
	}

	lookupTableEntries, err := common.GetAddressLookupTable(ctx, client, token.PoolLookupTable)
	if err != nil {
		return nil, nil, err
	}

	writableBytes := append(tokenAdminRegistry.WritableIndexes[0].Bytes(), tokenAdminRegistry.WritableIndexes[1].Bytes()...)
	var writableBits strings.Builder
	for _, b := range writableBytes {
		fmt.Fprintf(&writableBits, "%08b", b)
	}

	lookupTableMeta := []*solana.AccountMeta{}
	for i := range lookupTableEntries {
		meta := solana.Meta(lookupTableEntries[i])

		if string(writableBits.String()[i]) == "1" {
			meta = meta.WRITE()
		}
		lookupTableMeta = append(lookupTableMeta, meta)
	}

	list := make([]*solana.AccountMeta, 0, 3+len(lookupTableMeta))
	list = append(list,
		solana.Meta(userTokenAccount).WRITE(),
		solana.Meta(tokenBillingConfig),
		solana.Meta(poolChainConfig).WRITE(),
	)
	list = append(list, lookupTableMeta...)

	addressTables := make(map[solana.PublicKey]solana.PublicKeySlice)
	addressTables[token.PoolLookupTable] = lookupTableEntries

	return list, addressTables, nil
}
