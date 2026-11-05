package solcommon

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateReleaseTag(t *testing.T) {
	for _, tag := range []string{
		"v2.0.0", "v0.1.2", "v2.1.0-rc.1",
		"contracts/v2.0.0", "contracts/v0.1.2", "contracts/v2.1.0-rc.1",
	} {
		require.NoError(t, ValidateReleaseTag(tag), tag)
	}
	for _, tag := range []string{"latest", "main", "2.0.0", "v2.0", "contracts/2.0.0", "contracts/v2.0", "b1241a47", ""} {
		require.Error(t, ValidateReleaseTag(tag), tag)
	}
}

func TestResolveArtifactSource(t *testing.T) {
	t.Run("unset fails", func(t *testing.T) {
		t.Setenv(ContractsReleaseEnvVar, "")
		_, _, err := ResolveArtifactSource()
		require.ErrorContains(t, err, ContractsReleaseEnvVar)
	})

	t.Run("local", func(t *testing.T) {
		t.Setenv(ContractsReleaseEnvVar, LocalArtifactsValue)
		_, local, err := ResolveArtifactSource()
		require.NoError(t, err)
		require.True(t, local)
	})

	t.Run("pinned contracts tag", func(t *testing.T) {
		t.Setenv(ContractsReleaseEnvVar, "contracts/v2.0.0")
		release, local, err := ResolveArtifactSource()
		require.NoError(t, err)
		require.False(t, local)
		require.Equal(t, "contracts/v2.0.0", release)
	})

	t.Run("bare version becomes contracts tag", func(t *testing.T) {
		t.Setenv(ContractsReleaseEnvVar, "v2.0.0")
		release, local, err := ResolveArtifactSource()
		require.NoError(t, err)
		require.False(t, local)
		require.Equal(t, "contracts/v2.0.0", release)
	})

	t.Run("explicit pipeline version overrides environment", func(t *testing.T) {
		t.Setenv(ContractsReleaseEnvVar, "v1.6.0")
		release, local, err := ResolveArtifactSource("v2.0.0")
		require.NoError(t, err)
		require.False(t, local)
		require.Equal(t, "contracts/v2.0.0", release)
	})

	t.Run("non-semver fails", func(t *testing.T) {
		t.Setenv(ContractsReleaseEnvVar, "latest")
		_, _, err := ResolveArtifactSource()
		require.Error(t, err)
	})
}
