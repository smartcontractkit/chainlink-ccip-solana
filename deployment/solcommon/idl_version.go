package solcommon

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/Masterminds/semver/v3"
)

// IDLVersion returns the program version an Anchor IDL declares in metadata.version.
func IDLVersion(idlPath string) (*semver.Version, error) {
	raw, err := os.ReadFile(idlPath)
	if err != nil {
		return nil, fmt.Errorf("read IDL %s: %w", idlPath, err)
	}
	var idl struct {
		Metadata struct {
			Version string `json:"version"`
		} `json:"metadata"`
	}
	if err = json.Unmarshal(raw, &idl); err != nil {
		return nil, fmt.Errorf("parse IDL %s: %w", idlPath, err)
	}
	version, err := semver.StrictNewVersion(idl.Metadata.Version)
	if err != nil {
		return nil, fmt.Errorf("IDL %s has no valid metadata.version (%q): %w", idlPath, idl.Metadata.Version, err)
	}
	return version, nil
}

// RequireIDLVersionEqual fails unless the program's IDL declares exactly the wanted version.
// A program without an IDL passes.
func RequireIDLVersionEqual(programsPath, programName string, want *semver.Version) error {
	info, err := InspectArtifact(programsPath, programName)
	if err != nil {
		return err
	}
	if info.IDLPath == "" {
		return nil
	}
	declared, err := IDLVersion(info.IDLPath)
	if err != nil {
		return err
	}
	if !declared.Equal(want) {
		return fmt.Errorf("%s: the requested version is %s but its IDL declares %s", programName, want, declared)
	}
	return nil
}
