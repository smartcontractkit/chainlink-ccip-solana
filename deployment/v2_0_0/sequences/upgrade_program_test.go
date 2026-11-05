package sequences

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gagliardetto/solana-go"

	cldf_chain "github.com/smartcontractkit/chainlink-deployments-framework/chain"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	cldf_ops "github.com/smartcontractkit/chainlink-deployments-framework/operations"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations/optest"

	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/router"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/testsender"
)

func TestSubmitRemoteVerification(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "solana-verify")
	//nolint:gosec // G306: the fake solana-verify must keep the executable bit
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nprintf '%s\\n' \"$PWD\" \"$@\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	programID := solana.PublicKey{1}
	authority := solana.PublicKey{2}
	chain := cldf_solana.Chain{URL: "https://rpc.example", ProgramsPath: dir}
	output, err := submitRemoteVerification(context.Background(), chain, programID, authority)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Join([]string{
		dir, "remote", "submit-job", "--url", chain.URL,
		"--uploader", authority.String(), "--program-id", programID.String(), "",
	}, "\n")
	if output != want {
		t.Fatalf("command output = %q, want %q", output, want)
	}
}

// writeProgramArtifact drops a program binary that embeds the given program ID
// and declares the given SBPF version into programsPath.
func writeProgramArtifact(t *testing.T, programsPath string, programID solana.PublicKey, arch byte) {
	t.Helper()
	// Minimal ELF64 little-endian header with e_flags at 0x30, then the
	// program ID bytes the artifact check looks for.
	header := make([]byte, 0x34, 0x34+len(programID))
	copy(header, []byte{0x7f, 'E', 'L', 'F', 2, 1, 1})
	header[0x30] = arch
	header = append(header, programID[:]...)
	if err := os.WriteFile(filepath.Join(programsPath, router.ProgramName+".so"), header, 0o600); err != nil {
		t.Fatal(err)
	}
}

// The validation paths run before any chain interaction, so they are checked
// without an RPC. The authority branches past them need a live program
// account and are covered by the devenv smoke environment instead.
func TestUpgradeProgramRejectsInvalidInput(t *testing.T) {
	const selector = uint64(1)

	t.Run("unknown program type", func(t *testing.T) {
		_, err := cldf_ops.ExecuteSequence(
			optest.NewBundle(t), UpgradeProgram, cldf_chain.NewBlockChains(nil),
			UpgradeGenericInput{ProgramType: "NotAProgram", ChainSelector: selector})
		if err == nil || !strings.Contains(err.Error(), "unknown program type") {
			t.Fatalf("err = %v, want unknown program type", err)
		}
	})

	t.Run("chain not found", func(t *testing.T) {
		_, err := cldf_ops.ExecuteSequence(
			optest.NewBundle(t), UpgradeProgram, cldf_chain.NewBlockChains(nil),
			UpgradeGenericInput{ProgramType: UpgradeTypeRouter, ChainSelector: selector})
		if err == nil || !strings.Contains(err.Error(), "not found") {
			t.Fatalf("err = %v, want chain not found", err)
		}
	})

	t.Run("address ref not found", func(t *testing.T) {
		chains := cldf_chain.NewBlockChains(map[uint64]cldf_chain.BlockChain{
			selector: cldf_solana.Chain{},
		})
		_, err := cldf_ops.ExecuteSequence(
			optest.NewBundle(t), UpgradeProgram, chains,
			UpgradeGenericInput{ProgramType: UpgradeTypeRouter, ChainSelector: selector})
		if err == nil || !strings.Contains(err.Error(), "address ref not found") {
			t.Fatalf("err = %v, want address ref not found", err)
		}
	})

	t.Run("artifact source required on real networks", func(t *testing.T) {
		dir := t.TempDir()
		chains := cldf_chain.NewBlockChains(map[uint64]cldf_chain.BlockChain{
			selector: cldf_solana.Chain{URL: "https://rpc.example", ProgramsPath: dir},
		})
		_, err := cldf_ops.ExecuteSequence(
			optest.NewBundle(t), UpgradeProgram, chains,
			UpgradeGenericInput{
				ProgramType:   UpgradeTypeRouter,
				ChainSelector: selector,
				ExistingAddresses: []datastore.AddressRef{{
					ChainSelector: selector,
					Type:          datastore.ContractType(router.ContractType),
					Version:       router.Version,
					Address:       solana.NewWallet().PublicKey().String(),
				}},
			})
		if err == nil || !strings.Contains(err.Error(), solcommon.ContractsReleaseEnvVar) {
			t.Fatalf("err = %v, want %s required", err, solcommon.ContractsReleaseEnvVar)
		}
	})

	t.Run("artifact built for another program id", func(t *testing.T) {
		t.Setenv(solcommon.ContractsReleaseEnvVar, solcommon.LocalArtifactsValue)
		dir := t.TempDir()
		other, err := solana.NewRandomPrivateKey()
		if err != nil {
			t.Fatal(err)
		}
		writeProgramArtifact(t, dir, other.PublicKey(), 3)
		chains := cldf_chain.NewBlockChains(map[uint64]cldf_chain.BlockChain{
			selector: cldf_solana.Chain{ProgramsPath: dir},
		})
		expected, err := solana.NewRandomPrivateKey()
		if err != nil {
			t.Fatal(err)
		}
		_, err = cldf_ops.ExecuteSequence(
			optest.NewBundle(t), UpgradeProgram, chains,
			UpgradeGenericInput{
				ProgramType:   UpgradeTypeRouter,
				ChainSelector: selector,
				ExistingAddresses: []datastore.AddressRef{{
					ChainSelector: selector,
					Type:          datastore.ContractType(router.ContractType),
					Version:       router.Version,
					Address:       expected.PublicKey().String(),
				}},
			})
		if err == nil || !strings.Contains(err.Error(), "does not declare") {
			t.Fatalf("err = %v, want artifact program ID mismatch", err)
		}
	})

	t.Run("artifact built with wrong SBPF version", func(t *testing.T) {
		t.Setenv(solcommon.ContractsReleaseEnvVar, solcommon.LocalArtifactsValue)
		dir := t.TempDir()
		programID, err := solana.NewRandomPrivateKey()
		if err != nil {
			t.Fatal(err)
		}
		writeProgramArtifact(t, dir, programID.PublicKey(), 2)
		chains := cldf_chain.NewBlockChains(map[uint64]cldf_chain.BlockChain{
			selector: cldf_solana.Chain{ProgramsPath: dir},
		})
		_, err = cldf_ops.ExecuteSequence(
			optest.NewBundle(t), UpgradeProgram, chains,
			UpgradeGenericInput{
				ProgramType:   UpgradeTypeRouter,
				ChainSelector: selector,
				ExistingAddresses: []datastore.AddressRef{{
					ChainSelector: selector,
					Type:          datastore.ContractType(router.ContractType),
					Version:       router.Version,
					Address:       programID.PublicKey().String(),
				}},
			})
		if err == nil || !strings.Contains(err.Error(), "declares SBPF v2") {
			t.Fatalf("err = %v, want SBPF version mismatch", err)
		}
	})

	t.Run("missing keypair is not an error", func(t *testing.T) {
		t.Setenv(solcommon.ContractsReleaseEnvVar, solcommon.LocalArtifactsValue)
		dir := t.TempDir()
		programID, err := solana.NewRandomPrivateKey()
		if err != nil {
			t.Fatal(err)
		}
		writeProgramArtifact(t, dir, programID.PublicKey(), 3)
		chains := cldf_chain.NewBlockChains(map[uint64]cldf_chain.BlockChain{
			selector: cldf_solana.Chain{ProgramsPath: dir},
		})
		_, err = cldf_ops.ExecuteSequence(
			optest.NewBundle(t), UpgradeProgram, chains,
			UpgradeGenericInput{
				ProgramType:   UpgradeTypeRouter,
				ChainSelector: selector,
				ExistingAddresses: []datastore.AddressRef{{
					ChainSelector: selector,
					Type:          datastore.ContractType(router.ContractType),
					Version:       router.Version,
					Address:       programID.PublicKey().String(),
				}},
			})
		if err == nil || !strings.Contains(err.Error(), "missing Solana deployer key") {
			t.Fatalf("err = %v, want missing Solana deployer key", err)
		}
	})
}

// A program deployed under a custom qualifier (for example a staging sender)
// resolves through the input qualifier instead of the registry one.
func TestResolveUpgradeCustomQualifier(t *testing.T) {
	const selector = uint64(1)
	key, err := solana.NewRandomPrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	chains := cldf_chain.NewBlockChains(map[uint64]cldf_chain.BlockChain{
		selector: cldf_solana.Chain{},
	})
	_, _, programID, err := resolveUpgrade(chains, UpgradeGenericInput{
		ProgramType:   UpgradeTypeTestSenderV2,
		ChainSelector: selector,
		Qualifier:     "staging",
		ExistingAddresses: []datastore.AddressRef{{
			ChainSelector: selector,
			Type:          datastore.ContractType(testsender.ContractType),
			Version:       testsender.Version,
			Qualifier:     "staging",
			Address:       key.PublicKey().String(),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !programID.Equals(key.PublicKey()) {
		t.Fatalf("programID = %s, want %s", programID, key.PublicKey())
	}
}
