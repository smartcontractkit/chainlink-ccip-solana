package main

import (
	"context"

	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/sequences"
)

func main() {
	_, err := sequences.GenerateKeys(context.Background(), "contracts/target/deploy")
	if err != nil {
		panic(err)
	}
}
