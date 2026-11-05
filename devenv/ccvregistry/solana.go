// Package ccvregistry provides Solana-specific factory and configuration
// callbacks that are registered with the CCV devenv framework. Used by both the
// ccv binary (cmd/ccv/main.go) and e2e tests.
package ccvregistry

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/testcontainers/testcontainers-go"

	chainsel "github.com/smartcontractkit/chain-selectors"
	"github.com/smartcontractkit/chainlink-ccv/build/devenv/services"
	"github.com/smartcontractkit/chainlink-ccv/build/devenv/services/committeeverifier"
	"github.com/smartcontractkit/chainlink-ccv/build/devenv/services/executor"
	"github.com/smartcontractkit/chainlink-ccv/protocol"
	"github.com/smartcontractkit/chainlink-common/pkg/config"
	cldfchain "github.com/smartcontractkit/chainlink-deployments-framework/chain"
	cldfsolanaprovider "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana/provider"
	solconfig "github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
	ctfblockchain "github.com/smartcontractkit/chainlink-testing-framework/framework/components/blockchain"

	solaccessors "github.com/smartcontractkit/chainlink-ccip-solana/pkg/accessors"
	"github.com/smartcontractkit/chainlink-ccip-solana/pkg/chainselector"
	ccip "github.com/smartcontractkit/chainlink-ccip-solana/pkg/config"
	"github.com/smartcontractkit/chainlink-ccip-solana/pkg/solutil"
)

// SolanaCLDFProviderFactory creates a CLDF chain provider for a Solana blockchain entry.
// Registered via chainreg.Register as CLDFProvider.
func SolanaCLDFProviderFactory(ctx context.Context, b *ctfblockchain.Input) (cldfchain.BlockChain, uint64, error) {
	chainID := b.ChainID
	if chainID == "" {
		chainID = b.Out.ChainID
	}
	d, err := chainsel.GetChainDetailsByChainIDAndFamily(chainID, chainsel.FamilySolana)
	if err != nil {
		return nil, 0, err
	}
	keypairDir, err := os.MkdirTemp("", "solana-keypair-*")
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create temp dir for Solana keypair: %w", err)
	}
	contractsDir := b.ContractsDir
	if contractsDir == "" {
		contractsDir = os.TempDir()
	}
	if !filepath.IsAbs(contractsDir) {
		root, rootErr := findProjectRoot()
		if rootErr != nil {
			return nil, 0, fmt.Errorf("failed to find project root for relative contracts dir: %w", rootErr)
		}
		contractsDir = filepath.Join(root, contractsDir)
	}
	deployerKey, err := solutil.PrivateKeyFromEnvOrDefault(ctfblockchain.DefaultSolanaPrivateKey)
	if err != nil {
		return nil, 0, fmt.Errorf("resolve Solana deployer key: %w", err)
	}
	p, err := cldfsolanaprovider.NewRPCChainProvider(
		d.ChainSelector,
		cldfsolanaprovider.RPCChainProviderConfig{
			HTTPURL:        b.Out.Nodes[0].ExternalHTTPUrl,
			WSURL:          b.Out.Nodes[0].ExternalWSUrl,
			DeployerKeyGen: cldfsolanaprovider.PrivateKeyFromRaw(deployerKey.String()),
			ProgramsPath:   contractsDir,
			KeypairDirPath: keypairDir,
		},
	).Initialize(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to initialize Solana chain provider: %w", err)
	}
	return p, d.ChainSelector, nil
}

// SolanaVerifierModifier customises the verifier container request for Solana chains.
// Mounts NOP-owned local solana-config.toml (real RPC) at DefaultSolanaConfigPath.
// Registered via chainreg.Register as VerifierModifier.
func SolanaVerifierModifier(req testcontainers.ContainerRequest, verifierInput *committeeverifier.Input, outputs []*ctfblockchain.Output) (testcontainers.ContainerRequest, error) {
	req.Name = "solana-" + verifierInput.ContainerName
	req.Image = "solana-verifier:latest"
	return mountSolanaLocalConfig(req, outputs)
}

// SolanaExecutorModifier customize the executor container request for Solana chains.
// Mounts the same NOP-owned solana-config.toml as the verifier so destination readers
// and transmitters use real CTF internal RPC instead of job-spec placeholders.
// Registered via chainreg.Register as ExecutorModifier.
func SolanaExecutorModifier(req testcontainers.ContainerRequest, executorInput *executor.Input, outputs []*ctfblockchain.Output) (testcontainers.ContainerRequest, error) {
	req.Name = "solana-" + executorInput.ContainerName
	return mountSolanaLocalConfig(req, outputs)
}

func SolanaTokenVerifierModifier(req testcontainers.ContainerRequest, _ *services.TokenVerifierInput, outputs []*ctfblockchain.Output) (testcontainers.ContainerRequest, error) {
	return mountSolanaLocalConfig(req, outputs)
}

func mountSolanaLocalConfig(req testcontainers.ContainerRequest, outputs []*ctfblockchain.Output) (testcontainers.ContainerRequest, error) {
	cfg, err := buildSolanaLocalConfig(context.Background(), outputs)
	if err != nil {
		return req, fmt.Errorf("failed to build solana NOP config: %w", err)
	}
	configBytes, err := toml.Marshal(cfg)
	if err != nil {
		return req, fmt.Errorf("failed to marshal solana NOP config: %w", err)
	}

	req.Files = append(req.Files, testcontainers.ContainerFile{
		Reader:            bytes.NewReader(configBytes),
		ContainerFilePath: ccip.DefaultSolanaConfigPath,
		FileMode:          0o644,
	})

	return req, nil
}

// SolanaChainConfigLoader exists mainly because serviceFactory still enumerates chains from job blockchain_infos;
// This can be dropped along with pkg/accessors/info.go once chainlink-ccv is refactored.
// real RPC comes from the mounted NOP config file (see SolanaVerifierModifier / SolanaExecutorModifier).
// Registered via chainreg.Register as ChainConfigLoader.
// TODO once ccv serviceFactory is refactored to not rely on blockchain_infos for enumeration CCIP-11840, this can return empty map with no error, and remove info.go completely.
func SolanaChainConfigLoader(outputs []*ctfblockchain.Output) (map[string]any, error) {
	infos := make(map[string]any)
	for _, output := range outputs {
		if output.Family != chainsel.FamilySolana {
			continue
		}
		details, err := chainsel.GetChainDetailsByChainIDAndFamily(output.ChainID, output.Family)
		if err != nil {
			return nil, fmt.Errorf("failed to get chain details for chain %s, family %s: %w", output.ChainID, output.Family, err)
		}
		strSelector := chainselector.Format(protocol.ChainSelector(details.ChainSelector))
		// Metadata only for serviceFactory chain enumeration; RPC comes from NOP config mount.
		infos[strSelector] = solaccessors.Info{
			ChainID:         output.ChainID,
			Type:            output.Type,
			Family:          output.Family,
			UniqueChainName: output.ContainerName,
		}
	}
	if len(infos) == 0 {
		return nil, errors.New("no solana chains found in blockchain outputs")
	}
	return infos, nil
}

func buildSolanaLocalConfig(ctx context.Context, outputs []*ctfblockchain.Output) (*ccip.Config, error) {
	cfg := &ccip.Config{
		Chains: make(map[string]ccip.ChainConfig),
	}
	for _, output := range outputs {
		if output.Family != chainsel.FamilySolana {
			continue
		}
		details, err := chainsel.GetChainDetailsByChainIDAndFamily(output.ChainID, output.Family)
		if err != nil {
			return nil, fmt.Errorf("failed to get chain details for chain %s, family %s: %w", output.ChainID, output.Family, err)
		}
		strSelector := chainselector.Format(protocol.ChainSelector(details.ChainSelector))

		externalRPCURL := solanaExternalRPCURL(output.Nodes)
		if externalRPCURL == "" {
			return nil, fmt.Errorf("no external RPC URL in blockchain output for chain %s", output.ChainID)
		}
		rpcClient := rpc.New(externalRPCURL)
		chainID, err := fetchGenesisHash(ctx, rpcClient)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch genesis hash for chain %s: %w", output.ChainID, err)
		}

		nodes := make([]*solconfig.Node, 0, len(output.Nodes))
		for i, node := range output.Nodes {
			rpcURL := solanaRPCURL(node)
			if rpcURL == "" {
				return nil, fmt.Errorf("no RPC URL for node %d in blockchain output for chain %s", i, output.ChainID)
			}
			name := fmt.Sprintf("node-%d", i)
			nodes = append(nodes, &solconfig.Node{Name: &name, URL: config.MustParseURL(rpcURL)})
		}
		cfg.Chains[strSelector] = ccip.ChainConfig{
			ChainID: chainID,
			Nodes:   nodes,
		}
	}
	if len(cfg.Chains) == 0 {
		return nil, errors.New("no solana chains found in blockchain outputs")
	}
	return cfg, nil
}

func solanaRPCURL(node *ctfblockchain.Node) string {
	if node == nil {
		return ""
	}
	if node.InternalHTTPUrl != "" {
		return node.InternalHTTPUrl
	}
	return node.ExternalHTTPUrl
}

// solanaExternalRPCURL returns the external HTTP URL of the first node
func solanaExternalRPCURL(nodes []*ctfblockchain.Node) string {
	for _, node := range nodes {
		if node != nil && node.ExternalHTTPUrl != "" {
			return node.ExternalHTTPUrl
		}
	}
	return ""
}

func fetchGenesisHash(ctx context.Context, client *rpc.Client) (string, error) {
	if client == nil {
		return "", errors.New("nil RPC client")
	}
	hash, err := client.GetGenesisHash(ctx)
	if err != nil {
		return "", fmt.Errorf("fetch genesis hash: %w", err)
	}
	if hash.IsZero() {
		return "", errors.New("empty genesis hash")
	}
	return hash.String(), nil
}

// findProjectRoot walks up from the current working directory to find the
// nearest directory containing go.mod, which is the project root.
func findProjectRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found in any parent of %s", dir)
		}
		dir = parent
	}
}
