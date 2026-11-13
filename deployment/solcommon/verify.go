package solcommon

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"

	"github.com/gagliardetto/solana-go"
	solrpc "github.com/gagliardetto/solana-go/rpc"

	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations"
	mcms_types "github.com/smartcontractkit/mcms/types"
)

// VerifyProgramID is the OtterSec verified-builds registry. A record here is
// what lets a third party check a deployed program against its source.
//
// The instructions are built directly rather than shelling out to the
// solana-verify CLI: that keeps Docker and a CLI install off the deployment
// path, and avoids depending on the CLI's stdout format.
// Program source: https://github.com/otter-sec/otter-verify
const VerifyProgramID = "verifycLy8mB96wd9wqq3WDXQwM4oU6r42Th37Db9fC"

// verifyPDASeed is the registry's PDA prefix.
const verifyPDASeed = "otter_verify"

// DefaultRepoURL is the source a verified build is reproduced from.
const DefaultRepoURL = "https://github.com/smartcontractkit/chainlink-ccip-solana"

// DeploymentPolicy is what must hold before a program reaches a given network.
type DeploymentPolicy struct {
	Network            string
	RequireSecurityTxt bool
	RequireIDL         bool
	// RequirePublishedIDL makes the on-chain IDL upload mandatory rather than
	// best effort. It is separate from RequireIDL, which only asks that the
	// file be present in the artifact: publishing additionally needs the
	// metadata program on the cluster, which a bare local validator lacks.
	RequirePublishedIDL  bool
	RequireVerifiedBuild bool
}

// PolicyForChain decides what to enforce for a chain.
//
// A local validator builds from the working tree rather than a released commit,
// and carries no registry to publish to, so only the IDL requirement applies
// there. Every other network gets the full policy, except when the artifact
// source is the local build (ContractsReleaseEnvVar=local, the staging path):
// there is no release to verify against, so the verified-build requirement
// drops out while the rest of the real-network policy still applies.
func PolicyForChain(chain cldf_solana.Chain) DeploymentPolicy {
	if IsLocalValidator(chain.URL) {
		return DeploymentPolicy{Network: "local", RequireIDL: true}
	}
	policy := DeploymentPolicy{
		Network:              "network",
		RequireSecurityTxt:   true,
		RequireIDL:           true,
		RequirePublishedIDL:  true,
		RequireVerifiedBuild: true,
	}
	if os.Getenv(ContractsReleaseEnvVar) == LocalArtifactsValue {
		policy.Network = "staging"
		policy.RequireVerifiedBuild = false
	}
	return policy
}

// IsLocalValidator reports whether an RPC URL points at a local test validator.
//
// The host is matched exactly rather than searched for anywhere in the string.
// A substring test would read https://rpc.example/localhost as local and
// silently drop the whole real-network policy: no security.txt, no verified
// build, no published IDL.
func IsLocalValidator(rawURL string) bool {
	host := localHost(rawURL)
	if ip := net.ParseIP(host); ip != nil {
		// Covers 127.0.0.0/8 and ::1 as well as the 0.0.0.0 a validator binds to.
		return ip.IsLoopback() || ip.IsUnspecified()
	}

	return host == "localhost" || host == "host.docker.internal"
}

// localHost extracts the hostname from an RPC URL, tolerating the bare
// "host:port" and "host" forms that url.Parse does not treat as a URL.
func localHost(rawURL string) string {
	raw := strings.ToLower(strings.TrimSpace(rawURL))

	if u, err := url.Parse(raw); err == nil && u.Host != "" {
		return u.Hostname()
	}
	if h, _, err := net.SplitHostPort(raw); err == nil {
		return strings.Trim(h, "[]")
	}

	return strings.Trim(raw, "[]")
}

// BuildRecord is the provenance written to the registry for a deployed program.
type BuildRecord struct {
	ProgramID solana.PublicKey
	// Authority signs the record and owns the PDA. For an MCMS-governed program
	// this is the timelock signer, so the instruction must go through a proposal.
	Authority solana.PublicKey

	Version    string   // solana-verify version that would reproduce this
	GitURL     string   // defaults to DefaultRepoURL
	Commit     string   // the commit the artifact was built from
	Args       []string // build args, e.g. --library-name ccip_router
	DeploySlot uint64
}

func (r BuildRecord) gitURL() string {
	if r.GitURL == "" {
		return DefaultRepoURL
	}
	return strings.TrimSuffix(r.GitURL, ".git")
}

func (r BuildRecord) validate() error {
	if r.ProgramID.IsZero() {
		return errors.New("program id is required")
	}
	if r.Authority.IsZero() {
		return errors.New("authority is required")
	}
	if r.Commit == "" {
		return errors.New("commit is required: a verified build must name the source it came from")
	}
	return nil
}

// VerifyPDA derives the registry record address for an (authority, program) pair.
//
// The authority is part of the seed, so the same program verified by a different
// signer produces a different record. That is why an MCMS-governed program must
// be registered by the timelock rather than the deployer.
func VerifyPDA(authority, programID solana.PublicKey) (solana.PublicKey, error) {
	registry, err := solana.PublicKeyFromBase58(VerifyProgramID)
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("parse verify program id: %w", err)
	}
	pda, _, err := solana.FindProgramAddress(
		[][]byte{[]byte(verifyPDASeed), authority.Bytes(), programID.Bytes()},
		registry,
	)
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("derive verify pda: %w", err)
	}
	return pda, nil
}

// anchorDiscriminator returns the 8-byte prefix Anchor uses to dispatch an
// instruction: the first eight bytes of sha256("global:<name>").
func anchorDiscriminator(name string) []byte {
	sum := sha256.Sum256([]byte("global:" + name))
	return sum[:8]
}

// borshString encodes a Rust String: a u32 length followed by the bytes.
func borshString(s string) []byte {
	out := binary.LittleEndian.AppendUint32(nil, uint32(len(s))) //nolint:gosec // string lengths here are small
	return append(out, s...)
}

// encodeInputParams serialises the registry's InputParams. Field order matches
// the Rust struct declaration, which is what Borsh encodes against.
func encodeInputParams(r BuildRecord) []byte {
	data := borshString(r.Version)
	data = append(data, borshString(r.gitURL())...)
	data = append(data, borshString(r.Commit)...)

	data = binary.LittleEndian.AppendUint32(data, uint32(len(r.Args))) //nolint:gosec // arg counts are small
	for _, a := range r.Args {
		data = append(data, borshString(a)...)
	}

	return binary.LittleEndian.AppendUint64(data, r.DeploySlot)
}

// RegisterBuildInstruction builds the registry instruction recording how a
// deployed program was produced.
//
// Pass update=false the first time a program is registered and true afterwards;
// the registry uses separate initialize and update instructions because the
// record is reallocated to fit its contents.
func RegisterBuildInstruction(r BuildRecord, update bool) (*solana.GenericInstruction, error) {
	if err := r.validate(); err != nil {
		return nil, err
	}
	registry, err := solana.PublicKeyFromBase58(VerifyProgramID)
	if err != nil {
		return nil, fmt.Errorf("parse verify program id: %w", err)
	}
	pda, err := VerifyPDA(r.Authority, r.ProgramID)
	if err != nil {
		return nil, err
	}

	name := "initialize"
	if update {
		name = "update"
	}
	data := append(anchorDiscriminator(name), encodeInputParams(r)...)

	accounts := solana.AccountMetaSlice{
		solana.Meta(pda).WRITE(),
		solana.Meta(r.Authority).SIGNER().WRITE(),
		solana.Meta(r.ProgramID),
		solana.Meta(solana.SystemProgramID),
	}
	return solana.NewInstruction(registry, accounts, data), nil
}

// CloseBuildRecordInstruction removes a registry record and refunds its rent.
func CloseBuildRecordInstruction(authority, programID solana.PublicKey) (*solana.GenericInstruction, error) {
	registry, err := solana.PublicKeyFromBase58(VerifyProgramID)
	if err != nil {
		return nil, fmt.Errorf("parse verify program id: %w", err)
	}
	pda, err := VerifyPDA(authority, programID)
	if err != nil {
		return nil, err
	}
	accounts := solana.AccountMetaSlice{
		solana.Meta(pda).WRITE(),
		solana.Meta(authority).SIGNER().WRITE(),
		solana.Meta(programID),
	}
	return solana.NewInstruction(registry, accounts, anchorDiscriminator("close")), nil
}

// DeploySlot returns the slot a program was last deployed or upgraded at, read
// from its ProgramData account. The registry records it so a verifier knows
// which deployment a record refers to.
func DeploySlot(client *solrpc.Client, programID solana.PublicKey) (uint64, error) {
	programData, err := GetSolProgramData(client, programID)
	if err != nil {
		return 0, fmt.Errorf("locate program data for %s: %w", programID, err)
	}
	acct, err := client.GetAccountInfoWithOpts(context.Background(), programData.Address,
		&solrpc.GetAccountInfoOpts{Commitment: solrpc.CommitmentConfirmed})
	if err != nil {
		return 0, fmt.Errorf("read program data account: %w", err)
	}
	if acct == nil || acct.Value == nil {
		return 0, fmt.Errorf("program data account does not exist: %s", programData.Address)
	}
	raw := acct.Value.Data.GetBinary()
	if len(raw) < 12 {
		return 0, fmt.Errorf("program data account too short: %d bytes", len(raw))
	}
	// layout: u32 enum tag, u64 slot, Option<Pubkey> authority
	return binary.LittleEndian.Uint64(raw[4:12]), nil
}

// RegisterBuild records how a deployed program was built, in the public
// verified-builds registry.
//
// The registry PDA is seeded with the signing authority, so the record has to be
// written by whoever owns the program. When that is the deployer the instruction
// is sent immediately. When it is the timelock it cannot be, so the instruction
// is returned as a batch operation for a proposal, and the caller is responsible
// for including it. That is the same two-phase shape v1.6 uses.
//
// Returns a nil batch when the record was written directly.
func RegisterBuild(
	b operations.Bundle,
	chain cldf_solana.Chain,
	programID solana.PublicKey,
	programName string,
) (*mcms_types.BatchOperation, error) {
	prov, ok, err := ReadProvenance(chain.ProgramsPath)
	if err != nil {
		return nil, err
	}
	if !ok {
		// No manifest means a locally built tree: local validators and staging
		// (ContractsReleaseEnvVar=local) have no release to verify against, so
		// there is nothing to register.
		b.Logger.Debugw("Skipping verified build registration, no provenance",
			"program", programName)
		return nil, nil
	}

	authority, err := GetUpgradeAuthority(chain.Client, programID)
	if err != nil {
		return nil, fmt.Errorf("resolve upgrade authority for %s: %w", programName, err)
	}
	slot, err := DeploySlot(chain.Client, programID)
	if err != nil {
		return nil, err
	}

	record := prov.BuildRecordFor(programID, authority, programName, slot)

	// The record already exists if this program was registered by the same
	// authority before, which is the normal case for an upgrade.
	exists, err := buildRecordExists(chain, record)
	if err != nil {
		return nil, err
	}

	ix, err := RegisterBuildInstruction(record, exists)
	if err != nil {
		return nil, err
	}

	if authority.Equals(chain.Deployer().PublicKey()) {
		if err = chain.Confirm([]solana.Instruction{ix}); err != nil {
			return nil, fmt.Errorf("register verified build for %s: %w", programName, err)
		}
		b.Logger.Infow("Registered verified build",
			"program", programName, "commit", record.Commit, "release", prov.Release)
		return nil, nil
	}

	batch, err := BuildMCMSBatchOperation(chain.Selector, []solana.Instruction{ix}, VerifyProgramID, "VerifiedBuild")
	if err != nil {
		return nil, fmt.Errorf("wrap verified build registration for %s: %w", programName, err)
	}
	b.Logger.Infow("Verified build registration needs a proposal",
		"program", programName, "authority", authority, "commit", record.Commit)
	return &batch, nil
}

// anchorAccountPrefixLen is the 8-byte discriminator Anchor writes at the head
// of every account it owns. A shorter account cannot be a registry record.
const anchorAccountPrefixLen = 8

// buildRecordExists reports whether the registry already holds a record for
// this (authority, program) pair, which decides between initialize and update.
func buildRecordExists(chain cldf_solana.Chain, record BuildRecord) (bool, error) {
	pda, err := VerifyPDA(record.Authority, record.ProgramID)
	if err != nil {
		return false, err
	}
	registry, err := solana.PublicKeyFromBase58(VerifyProgramID)
	if err != nil {
		return false, fmt.Errorf("parse verify program id: %w", err)
	}

	acct, err := chain.Client.GetAccountInfoWithOpts(context.Background(), pda,
		&solrpc.GetAccountInfoOpts{Commitment: solrpc.CommitmentConfirmed})
	if err != nil {
		if errors.Is(err, solrpc.ErrNotFound) {
			return false, nil
		}
		// Only absence means "no record". Reporting a transient read failure as
		// absence would build an initialize over an existing record and bury
		// the real error.
		return false, fmt.Errorf("read verified build record %s: %w", pda, err)
	}
	if acct == nil || acct.Value == nil {
		return false, nil
	}

	// The PDA is publicly derivable, so anyone can leave a system-owned account
	// there by sending it lamports. Only an account the registry itself owns is
	// a record; treating a dusted one as existing would pick update, which the
	// registry then rejects.
	return acct.Value.Owner.Equals(registry) &&
		len(acct.Value.Data.GetBinary()) >= anchorAccountPrefixLen, nil
}
