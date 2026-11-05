package solcommon

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gagliardetto/solana-go"
)

// ProvenanceFile is the manifest that travels with a set of built artifacts.
//
// It sits next to the .so and IDL files in ProgramsPath rather than being
// threaded through sequence inputs. That keeps the deploy and upgrade
// signatures unchanged, and means anything holding a ProgramsPath can answer
// "where did these binaries come from" without being told.
const ProvenanceFile = "provenance.json"

// Provenance records which source produced a set of artifacts. The build writes
// it; the deploy reads it. Nothing infers it, because a guessed commit in a
// verified-build record is worse than no record at all.
type Provenance struct {
	// Release is the tag the artifacts were published under, e.g. contracts/v2.0.0.
	Release string `json:"release"`
	// Commit is the full SHA the artifacts were built from.
	Commit string `json:"commit"`
	// RepoURL is the source repository. Defaults to DefaultRepoURL when empty.
	RepoURL string `json:"repoUrl,omitempty"`
	// BuildArgs are the solana-verify arguments needed to reproduce the build.
	BuildArgs []string `json:"buildArgs,omitempty"`
	// VerifierVersion is the solana-verify version used to build, recorded so a
	// verifier can reproduce with the same toolchain.
	VerifierVersion string `json:"verifierVersion,omitempty"`
}

func (p Provenance) Repo() string {
	if p.RepoURL == "" {
		return DefaultRepoURL
	}
	return p.RepoURL
}

// Validate reports whether the manifest names a source precisely enough to
// register a verified build.
func (p Provenance) Validate() error {
	if p.Commit == "" {
		return errors.New("provenance has no commit")
	}
	if len(p.Commit) < 7 {
		return fmt.Errorf("provenance commit %q is too short to identify a revision", p.Commit)
	}
	return nil
}

// WriteProvenance records the manifest alongside a set of artifacts. Called
// after artifacts are downloaded or built.
func WriteProvenance(programsPath string, p Provenance) error {
	if err := p.Validate(); err != nil {
		return fmt.Errorf("refusing to write incomplete provenance: %w", err)
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("encode provenance: %w", err)
	}
	path := filepath.Join(programsPath, ProvenanceFile)
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write provenance to %s: %w", path, err)
	}
	return nil
}

// ReadProvenance loads the manifest for a set of artifacts. The second return
// is false when no manifest is present, which is the normal case for a locally
// built tree.
func ReadProvenance(programsPath string) (Provenance, bool, error) {
	path := filepath.Join(programsPath, ProvenanceFile)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Provenance{}, false, nil
	}
	if err != nil {
		return Provenance{}, false, fmt.Errorf("read provenance %s: %w", path, err)
	}

	var p Provenance
	if err := json.Unmarshal(data, &p); err != nil {
		return Provenance{}, false, fmt.Errorf("parse provenance %s: %w", path, err)
	}
	if err := p.Validate(); err != nil {
		return Provenance{}, false, fmt.Errorf("provenance %s is unusable: %w", path, err)
	}
	return p, true, nil
}

// BuildRecordFor turns provenance into the registry record for one program.
func (p Provenance) BuildRecordFor(programID, authority solana.PublicKey, programName string, deploySlot uint64) BuildRecord {
	args := append([]string{"--library-name", programName}, p.BuildArgs...)
	return BuildRecord{
		ProgramID:  programID,
		Authority:  authority,
		Version:    p.VerifierVersion,
		GitURL:     p.Repo(),
		Commit:     p.Commit,
		Args:       args,
		DeploySlot: deploySlot,
	}
}
