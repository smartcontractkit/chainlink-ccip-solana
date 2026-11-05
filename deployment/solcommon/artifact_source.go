package solcommon

import (
	"fmt"
	"os"
	"strings"

	"github.com/Masterminds/semver/v3"
)

const (
	// ContractsReleaseEnvVar selects which artifacts a deploy or upgrade uses on a
	// real network. It accepts a pinned version such as "v2.0.0" or its full
	// release tag "contracts/v2.0.0". Production always resolves to the
	// contracts-prefixed GitHub tag.
	ContractsReleaseEnvVar = "SOLANA_CONTRACTS_RELEASE"

	// LocalArtifactsValue selects the local build in ProgramsPath instead of a
	// downloaded release.
	LocalArtifactsValue = "local"

	// ContractsTagPrefix prefixes versioned contracts release tags.
	ContractsTagPrefix = "contracts/v"
)

// ValidateReleaseTag accepts either a bare semantic version (for example
// "v2.0.0") or the corresponding contracts release tag ("contracts/v2.0.0").
func ValidateReleaseTag(release string) error {
	version := release
	if after, ok := strings.CutPrefix(release, ContractsTagPrefix); ok {
		version = after
	} else if after, ok := strings.CutPrefix(version, "v"); ok {
		version = after
	} else {
		return fmt.Errorf(
			"%s must be a version like \"v2.0.0\" or a contracts release tag like \"contracts/v2.0.0\" (got %q); the rolling \"latest\" prerelease is never a deploy target",
			ContractsReleaseEnvVar, release)
	}
	if _, err := semver.StrictNewVersion(version); err != nil {
		return fmt.Errorf("%s must be a valid version like \"v2.0.0\" (got %q): %w",
			ContractsReleaseEnvVar, release, err)
	}
	return nil
}

// ResolveArtifactSource uses an explicit pipeline version when supplied,
// otherwise it reads ContractsReleaseEnvVar. A real network deploy or upgrade
// must resolve to a pinned contracts release tag or the explicit local build
// option.
func ResolveArtifactSource(explicitVersion ...string) (release string, local bool, err error) {
	version := os.Getenv(ContractsReleaseEnvVar)
	if len(explicitVersion) > 0 && explicitVersion[0] != "" {
		version = explicitVersion[0]
	}
	switch version {
	case LocalArtifactsValue:
		return "", true, nil
	case "":
		return "", false, fmt.Errorf(
			"%s must be set: a pinned version (e.g. \"v2.0.0\") for production, or %q to use the local build in ProgramsPath (staging)",
			ContractsReleaseEnvVar, LocalArtifactsValue)
	default:
		if tagErr := ValidateReleaseTag(version); tagErr != nil {
			return "", false, tagErr
		}
		if strings.HasPrefix(version, ContractsTagPrefix) {
			return version, false, nil
		}
		return ContractsTagPrefix + strings.TrimPrefix(version, "v"), false, nil
	}
}
