package solcommon

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"errors"
	"fmt"

	"github.com/gagliardetto/solana-go"
	solrpc "github.com/gagliardetto/solana-go/rpc"

	solCommonUtil "github.com/smartcontractkit/chainlink-ccip/chains/solana/utils/common"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations"
	mcms_types "github.com/smartcontractkit/mcms/types"

	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/metadata"
)

// PublishIDL uploads a program's IDL to its canonical Program Metadata account,
// so that an explorer or a client can read the interface straight off the
// chain instead of trusting a file someone handed them.
//
// It mirrors RegisterBuild: the bulk of the work runs on the deployer key, and
// only the instructions the upgrade authority must sign come back as batch
// operations for MCMS. Their number depends on how far the metadata account
// has to grow, not on how many chunks the upload took.
func PublishIDL(
	b operations.Bundle,
	chain cldf_solana.Chain,
	programID solana.PublicKey,
	programName string,
) ([]mcms_types.BatchOperation, error) {
	policy := PolicyForChain(chain)

	info, err := InspectArtifact(chain.ProgramsPath, programName)
	if err != nil {
		return nil, err
	}
	if info.IDLPath == "" {
		// RequireDeployable already refuses to deploy without an IDL where one
		// is mandatory, so reaching here means it is genuinely optional.
		b.Logger.Debugw("Skipping IDL publish, no IDL in artifact", "program", programName)
		return nil, nil
	}

	// The metadata program is on every public cluster but not on a bare local
	// validator, and creating an account through it needs a system instruction
	// that only recent validators have. Neither is worth failing a devenv run
	// over, so locally this degrades to a skip.
	available, err := programMetadataAvailable(chain)
	if err != nil {
		return nil, err
	}
	if !available {
		if policy.RequirePublishedIDL {
			return nil, fmt.Errorf(
				"cannot publish the IDL for %s: the Program Metadata program (%s) is not deployed on %s",
				programName, metadata.ProgramID, chain.String())
		}
		b.Logger.Infow("Skipping IDL publish, metadata program not on this cluster",
			"program", programName, "metadataProgram", metadata.ProgramID)

		return nil, nil
	}

	document, err := metadata.LoadJSON(info.IDLPath)
	if err != nil {
		return nil, err
	}

	programData, _, err := solana.FindProgramAddress(
		[][]byte{programID.Bytes()}, solana.BPFLoaderUpgradeableProgramID)
	if err != nil {
		return nil, fmt.Errorf("derive program data for %s: %w", programName, err)
	}

	authority, err := GetUpgradeAuthority(chain.Client, programID)
	if err != nil {
		return nil, fmt.Errorf("resolve upgrade authority for %s: %w", programName, err)
	}
	if authority.IsZero() {
		// An immutable program has no canonical authority, so nobody can write
		// its metadata.
		b.Logger.Warnw("Skipping IDL publish, program is immutable", "program", programName)

		return nil, nil
	}

	account, err := metadata.CanonicalPDA(programID, metadata.SeedIDL)
	if err != nil {
		return nil, err
	}
	current, err := readMetadataAccount(chain.Client, account)
	if err != nil {
		return nil, err
	}

	buffer := idlBufferKey(chain, programID)

	plan, err := metadata.PlanPublish(metadata.PublishConfig{
		Program:     programID,
		ProgramData: programData,
		Authority:   authority,
		Payer:       chain.DeployerKey.PublicKey(),
		Buffer:      buffer.PublicKey(),
		RentExemptFor: func(space int) (uint64, error) {
			return chain.Client.GetMinimumBalanceForRentExemption(
				context.Background(), uint64(space), solrpc.CommitmentConfirmed) //nolint:gosec // a positive account size
		},
		Seed:            metadata.SeedIDL,
		Format:          metadata.FormatJSON,
		Document:        document,
		Exists:          current.Initialized,
		CurrentSize:     current.Size,
		CurrentLamports: current.Lamports,
	})
	if err != nil {
		return nil, fmt.Errorf("plan IDL publish for %s: %w", programName, err)
	}

	if err = uploadToBuffer(b, chain, plan, buffer, programName); err != nil {
		return nil, err
	}

	governed := plan.Governed()
	if len(governed) == 0 {
		return nil, nil
	}

	if authority.Equals(chain.DeployerKey.PublicKey()) {
		// One instruction per transaction: Initialize can carry the whole
		// document, which leaves no room to batch.
		for _, ix := range governed {
			if cErr := chain.Confirm([]solana.Instruction{ix}); cErr != nil {
				return nil, fmt.Errorf("publish IDL for %s: %w", programName, cErr)
			}
		}
		if plan.CloseBuffer != nil {
			if cErr := chain.Confirm([]solana.Instruction{plan.CloseBuffer},
				solCommonUtil.AddSigners(buffer)); cErr != nil {
				return nil, fmt.Errorf("close IDL buffer for %s: %w", programName, cErr)
			}
		}
		b.Logger.Infow("Published IDL",
			"program", programName, "account", account, "bytes", plan.PayloadLen)

		return nil, nil
	}

	// A batch runs as one timelock ExecuteBatch, and the runtime caps how much
	// accounts may grow across all its inner instructions. The plan packs the
	// instructions into runs that each stay under that cap, so a document that
	// needs Extends is spread over several operations.
	groups := plan.GovernedGroups()
	batches := make([]mcms_types.BatchOperation, 0, len(groups))
	for _, group := range groups {
		batch, bErr := BuildMCMSBatchOperation(
			chain.Selector, group, metadata.ProgramID.String(), "PublishIDL")
		if bErr != nil {
			return nil, fmt.Errorf("wrap IDL publish for %s: %w", programName, bErr)
		}
		batches = append(batches, batch)
	}

	// The buffer has to outlive the proposal, so its rent stays locked up until
	// the proposal executes. idlBufferKey is derived rather than random so that
	// a later run can still reach it.
	b.Logger.Infow("IDL publish needs a proposal",
		"program", programName, "account", account, "authority", authority,
		"bytes", plan.PayloadLen, "buffer", buffer.PublicKey())

	return batches, nil
}

// uploadToBuffer runs the half of the plan the deployer key can sign.
func uploadToBuffer(
	b operations.Bundle,
	chain cldf_solana.Chain,
	plan metadata.PublishPlan,
	buffer solana.PrivateKey,
	programName string,
) error {
	if len(plan.Prepare) == 0 {
		return nil
	}

	prepare := plan.Prepare

	// A buffer left behind by an interrupted run would make the allocate fail
	// as already initialized, so it is reclaimed first. The key is derived, so
	// this also returns the stranded rent.
	stale, err := readMetadataAccount(chain.Client, buffer.PublicKey())
	if err != nil {
		return err
	}
	if stale.Initialized {
		b.Logger.Infow("Reclaiming a leftover IDL buffer",
			"program", programName, "buffer", buffer.PublicKey())
		prepare = append([]solana.Instruction{
			metadata.CloseInstruction(buffer.PublicKey(), buffer.PublicKey(),
				solana.PublicKey{}, solana.PublicKey{}, chain.DeployerKey.PublicKey()),
		}, prepare...)
	}

	if err := chain.Confirm(prepare, solCommonUtil.AddSigners(buffer)); err != nil {
		return fmt.Errorf("create the IDL buffer for %s: %w", programName, err)
	}

	// Writes append, so they go one transaction at a time, in order.
	for i, w := range plan.Writes {
		if err := chain.Confirm([]solana.Instruction{w}, solCommonUtil.AddSigners(buffer)); err != nil {
			return fmt.Errorf("write IDL chunk %d/%d for %s: %w", i+1, len(plan.Writes), programName, err)
		}
	}
	b.Logger.Debugw("Filled the IDL buffer",
		"program", programName, "chunks", len(plan.Writes), "bytes", plan.PayloadLen)

	return nil
}

// idlBufferKey derives the scratch keypair for a program's IDL upload.
//
// It is derived rather than random so that a buffer is always reachable again:
// an interrupted run can reclaim it, and a buffer held open while a proposal
// waits for its timelock is not a permanent loss of rent. The deployer key is
// in the seed so two deployers never collide on the same buffer.
func idlBufferKey(chain cldf_solana.Chain, programID solana.PublicKey) solana.PrivateKey {
	// The seed is the deployer's secret key, not its public key. Seeded from
	// public values this whole keypair is recomputable by anyone, who could
	// then sign Write or Close against the buffer in the window between the
	// deployer filling it and the proposal's SetDataFromBuffer running, and
	// have governance commit an IDL of their choosing. Using the secret keeps
	// the address reachable by the same deployer on a later run, which is what
	// makes an interrupted publish recoverable, without making it guessable.
	seed := sha256.Sum256(fmt.Appendf(nil, "ccip-idl-buffer/%x/%s/%s",
		*chain.DeployerKey, programID, metadata.SeedIDL))

	return solana.PrivateKey(ed25519.NewKeyFromSeed(seed[:]))
}

// metadataAccountState is what a derived metadata address currently holds.
type metadataAccountState struct {
	// Initialized is true only when the metadata program owns the account.
	// These addresses are derived from public values, so anyone can leave a
	// system-owned account at one by sending it lamports. Such an account
	// carries no metadata header: treating it as initialized would pick
	// SetData over Initialize, or Close over Allocate, and the metadata
	// program rejects both, wedging every later publish for this program.
	Initialized bool
	Size        int
	Lamports    uint64
}

// readMetadataAccount reports what sits at a metadata address.
func readMetadataAccount(client *solrpc.Client, address solana.PublicKey) (metadataAccountState, error) {
	out, err := client.GetAccountInfoWithOpts(context.Background(), address,
		&solrpc.GetAccountInfoOpts{Commitment: solrpc.CommitmentConfirmed})
	if err != nil {
		if errors.Is(err, solrpc.ErrNotFound) {
			return metadataAccountState{}, nil
		}

		return metadataAccountState{}, fmt.Errorf("read account %s: %w", address, err)
	}
	if out.Value == nil {
		return metadataAccountState{}, nil
	}

	return metadataAccountState{
		Initialized: out.Value.Owner.Equals(metadata.ProgramID),
		Size:        len(out.Value.Data.GetBinary()),
		Lamports:    out.Value.Lamports,
	}, nil
}

// programMetadataAvailable reports whether the metadata program can be used on
// this cluster.
func programMetadataAvailable(chain cldf_solana.Chain) (bool, error) {
	// Only the account's existence matters, so ask for none of its bytes.
	var none uint64
	out, err := chain.Client.GetAccountInfoWithOpts(context.Background(), metadata.ProgramID,
		&solrpc.GetAccountInfoOpts{
			Commitment: solrpc.CommitmentConfirmed,
			DataSlice:  &solrpc.DataSlice{Offset: &none, Length: &none},
		})
	if err != nil {
		if errors.Is(err, solrpc.ErrNotFound) {
			return false, nil
		}

		return false, fmt.Errorf("look up the program metadata program: %w", err)
	}

	return out.Value != nil && out.Value.Executable, nil
}

// RecordDeployment publishes everything a freshly deployed or upgraded program
// owes the outside world: its verified-build record, so the binary can be
// checked against its source, and its IDL, so its interface can be read off the
// chain. Both are required on a real network.
//
// Whatever the upgrade authority has to sign comes back as batch operations for
// the caller to carry in its proposal; anything the deployer key can sign has
// already been sent by the time this returns.
func RecordDeployment(
	b operations.Bundle,
	chain cldf_solana.Chain,
	programID solana.PublicKey,
	programName string,
) ([]mcms_types.BatchOperation, error) {
	var batches []mcms_types.BatchOperation

	build, err := RegisterBuild(b, chain, programID, programName)
	if err != nil {
		return nil, err
	}
	if build != nil {
		batches = append(batches, *build)
	}

	idl, err := PublishIDL(b, chain, programID, programName)
	if err != nil {
		return nil, err
	}
	batches = append(batches, idl...)

	return batches, nil
}
