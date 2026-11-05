package testutils

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/pelletier/go-toml/v2"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-testing-framework/framework/components/blockchain"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/config"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
)

var PathToAnchorConfig = filepath.Join(ProjectRoot, "Anchor.toml")

// PreloadedAccount is an account that the validator holds at start.
// Use it for state that the programs can no longer create, such as an account with a legacy layout.
type PreloadedAccount struct {
	Address solana.PublicKey
	Owner   solana.PublicKey
	Data    []byte
}

// validatorAccountFile is the JSON format that `solana-test-validator --account` reads.
type validatorAccountFile struct {
	Pubkey  string `json:"pubkey"`
	Account struct {
		Lamports   uint64    `json:"lamports"`
		Data       [2]string `json:"data"` // [content, encoding]
		Owner      string    `json:"owner"`
		Executable bool      `json:"executable"`
		RentEpoch  uint64    `json:"rentEpoch"`
	} `json:"account"`
}

// preloadAccountFlags writes each account to dir and returns the validator flags that load them.
// CTF mounts dir at /programs in the validator container.
func preloadAccountFlags(t *testing.T, dir string, accounts []PreloadedAccount) []string {
	t.Helper()
	flags := make([]string, 0, 3*len(accounts))
	for _, a := range accounts {
		var f validatorAccountFile
		f.Pubkey = a.Address.String()
		f.Account.Lamports = solana.LAMPORTS_PER_SOL // rent-exempt for any account a test preloads
		f.Account.Data = [2]string{base64.StdEncoding.EncodeToString(a.Data), "base64"}
		f.Account.Owner = a.Owner.String()
		file, err := json.Marshal(f)
		require.NoError(t, err)
		name := f.Pubkey + ".json"
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), file, 0o600))
		flags = append(flags, "--account", f.Pubkey, "/programs/"+name)
	}
	return flags
}

// DeployAllPrograms starts a local Solana validator via CTF Docker with all
// programs from the Anchor config and the given accounts pre-loaded, and returns an RPC client.
func DeployAllPrograms(t *testing.T, pathToAnchorConfig string, admin solana.PrivateKey, accounts ...PreloadedAccount) *rpc.Client {
	t.Helper()
	return rpc.New(startValidator(t, pathToAnchorConfig, admin, accounts...))
}

// startValidator starts the validator for DeployAllPrograms and returns its RPC URL.
func startValidator(t *testing.T, pathToAnchorConfig string, admin solana.PrivateKey, accounts ...PreloadedAccount) string {
	t.Helper()
	ctx := t.Context()

	programs, contractsDir := resolveAnchorPrograms(t, pathToAnchorConfig)

	net, err := NewLocalValidator(ctx, &blockchain.Input{
		ContractsDir:             contractsDir,
		PublicKey:                admin.PublicKey().String(),
		SolanaPrograms:           programs,
		DockerCmdParamsOverrides: preloadAccountFlags(t, contractsDir, accounts),
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		if net.Container != nil {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := net.Container.Terminate(cleanupCtx); err != nil {
				t.Logf("warning: failed to terminate container: %v", err)
			}
		}
	})

	return net.Nodes[0].ExternalHTTPUrl
}

func FundAccounts(ctx context.Context, accounts []solana.PrivateKey, solanaGoClient *rpc.Client) error {
	return fundAccounts(ctx, accounts, solanaGoClient, waitAndRetryOpts{
		RemainingAttempts: 5,
		Timeout:           30 * time.Second,
		Timestep:          500 * time.Millisecond,
	})
}

type waitAndRetryOpts struct {
	RemainingAttempts uint
	Timeout           time.Duration
	Timestep          time.Duration
}

func (o waitAndRetryOpts) WithDecreasedAttempts() waitAndRetryOpts {
	return waitAndRetryOpts{
		RemainingAttempts: o.RemainingAttempts - 1,
		Timeout:           o.Timeout,
		Timestep:          o.Timestep,
	}
}

func fundAccounts(ctx context.Context, accounts []solana.PrivateKey, solanaGoClient *rpc.Client, opts waitAndRetryOpts) error {
	sigs := make([]solana.Signature, 0, len(accounts))
	for _, v := range accounts {
		sig, err := solanaGoClient.RequestAirdrop(ctx, v.PublicKey(), 1000*solana.LAMPORTS_PER_SOL, rpc.CommitmentFinalized)
		if err != nil {
			return fmt.Errorf("airdrop to %s: %w", v.PublicKey(), err)
		}
		sigs = append(sigs, sig)
	}

	// wait for confirmation so later transactions don't fail
	remaining := accounts
	initTime := time.Now()
	for elapsed := time.Since(initTime); elapsed < opts.Timeout; elapsed = time.Since(initTime) {
		time.Sleep(opts.Timestep)

		statusRes, sigErr := solanaGoClient.GetSignatureStatuses(ctx, true, sigs...)
		if sigErr != nil {
			return fmt.Errorf("get signature statuses: %w", sigErr)
		}
		if statusRes == nil || statusRes.Value == nil {
			return errors.New("nil signature status response")
		}

		accountsWithNonFinalizedFunding := []solana.PrivateKey{}
		for i, res := range statusRes.Value {
			if res == nil || res.ConfirmationStatus == rpc.ConfirmationStatusProcessed || res.ConfirmationStatus == rpc.ConfirmationStatusConfirmed {
				accountsWithNonFinalizedFunding = append(accountsWithNonFinalizedFunding, accounts[i])
			}
		}
		remaining = accountsWithNonFinalizedFunding

		if len(remaining) == 0 {
			return nil // all done!
		}
	}

	decreasedOpts := opts.WithDecreasedAttempts()
	if decreasedOpts.RemainingAttempts == 0 {
		return errors.New("unable to confirm airdrop transactions after all attempts")
	}
	return fundAccounts(ctx, remaining, solanaGoClient, decreasedOpts) // recursive call with only remaining & with fewer attempts
}

// resolveAnchorPrograms parses Anchor.toml and collects all program binaries
// into a single temporary directory suitable for CTF's ContractsDir mount.
// Returns the SolanaPrograms map (name→programID) and the temp directory path.
func resolveAnchorPrograms(t *testing.T, pathToAnchorConfig string) (map[string]string, string) {
	t.Helper()

	anchorData := struct {
		Programs struct {
			Localnet map[string]string
		}
	}{}

	anchorBytes, err := os.ReadFile(pathToAnchorConfig)
	require.NoError(t, err)
	require.NoError(t, toml.Unmarshal(anchorBytes, &anchorData))

	// CTF mounts a single directory as /programs. Collect all .so files
	// (from both target/deploy and target/vendor) into one temp directory.
	tmpDir := t.TempDir()
	programs := make(map[string]string, len(anchorData.Programs.Localnet))

	for name, id := range anchorData.Programs.Localnet {
		programs[name] = id
		filename := name + ".so"

		src := filepath.Join(ContractsDir, filename)
		if _, err := os.Stat(src); os.IsNotExist(err) {
			src = filepath.Join(VendorContractsDir, filename)
			if _, err := os.Stat(src); os.IsNotExist(err) {
				require.FailNowf(t, "Program file not found", "Could not find program file %s in either %s or %s", filename, ContractsDir, VendorContractsDir)
			}
		}

		data, err := os.ReadFile(src)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, filename), data, 0o600)) //nolint:gosec // tmpDir is from t.TempDir(), filename is from trusted program list
	}

	return programs, tmpDir
}

func WaitForTheNextBlock(client *rpc.Client, timeout time.Duration, commitment rpc.CommitmentType) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	return WaitForNewBlock(ctx, client, 1, commitment)
}

func WaitForNewBlock(ctx context.Context, client *rpc.Client, height uint64, commitment rpc.CommitmentType) error {
	initialHeight, err := client.GetBlockHeight(ctx, commitment)
	if err != nil {
		return fmt.Errorf("failed to get initial block height: %w", err)
	}

	targetFinalHeight := initialHeight + height

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			currentHeight, err := client.GetBlockHeight(ctx, commitment)
			if err != nil {
				return fmt.Errorf("failed to get current block height: %w", err)
			}

			if currentHeight >= targetFinalHeight {
				return nil
			}
		}
	}
}

func GetProgramID(tx *solana.Transaction, meta *rpc.TransactionMeta, programIDIndex uint16) (solana.PublicKey, error) {
	allKeys := common.GetAllKeys(tx, meta)

	if int(programIDIndex) >= len(allKeys) {
		return solana.PublicKey{}, fmt.Errorf("programID index %d out of bounds (total keys: %d)", programIDIndex, len(allKeys))
	}

	return allKeys[programIDIndex], nil
}

// programData is the Borsh-encoded layout of a Solana BPF upgradeable program account.
type programData struct {
	DataType uint32
	Address  solana.PublicKey
}

// GetProgramDataAddress fetches and decodes a program account to return its program data address.
func GetProgramDataAddress(ctx context.Context, t *testing.T, client *rpc.Client, programID solana.PublicKey) solana.PublicKey {
	t.Helper()
	data, err := client.GetAccountInfoWithOpts(ctx, programID, &rpc.GetAccountInfoOpts{
		Commitment: config.DefaultCommitment,
	})
	require.NoError(t, err)
	var pd programData
	require.NoError(t, bin.UnmarshalBorsh(&pd, data.Bytes()))
	return pd.Address
}

// GetTokenBalance returns the string balance of a token account.
func GetTokenBalance(ctx context.Context, t *testing.T, client *rpc.Client, account solana.PublicKey) string {
	t.Helper()
	balanceRes, err := client.GetTokenAccountBalance(ctx, account, config.DefaultCommitment)
	require.NoError(t, err)
	return balanceRes.Value.Amount
}
