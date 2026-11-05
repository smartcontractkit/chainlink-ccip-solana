package accessors

import "fmt"

// Info is chain-enumeration metadata for Solana chains. It is emitted by devenv's
// SolanaChainConfigLoader into the job spec's blockchain_infos solely so CCV's
// serviceFactory can enumerate chains. RPC URLs and tuning are NOP-owned and live
// in the mounted solana-config.toml (see pkg.Config), not here.
//
// This type can be dropped once chainlink-ccv no longer enumerates chains from job
// blockchain_infos (CCIP-11840).
type Info struct {
	ChainID         string `json:"chain_id"`
	Type            string `json:"type"`
	Family          string `json:"family"`
	UniqueChainName string `json:"unique_chain_name"`
}

func (bi Info) Empty() bool {
	return bi.ChainID == "" && bi.Type == "" && bi.Family == "" && bi.UniqueChainName == ""
}

func (bi Info) String() string {
	return fmt.Sprintf("Chain ID: %s, Type: %s, Family: %s, ChainName: %s",
		bi.ChainID, bi.Type, bi.Family, bi.UniqueChainName)
}
