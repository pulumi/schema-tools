package normalize

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadMetadata(t *testing.T) {
	t.Parallel()

	fixture := mustReadFixture(t, "parity-vsphere.json")
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "bridge-metadata.json")
	require.NoError(t, os.WriteFile(path, fixture, 0o600))

	metadata, err := LoadMetadata(path)
	require.NoError(t, err)
	require.NotNil(t, metadata)
	require.NotNil(t, metadata.AutoAliasing)
	require.Contains(t, metadata.AutoAliasing.Resources, "vsphere_compute_cluster")
}

func TestLoadMetadataRequired(t *testing.T) {
	t.Parallel()

	_, err := LoadMetadata("")
	require.Error(t, err)
	require.ErrorIs(t, err, ErrMetadataRequired)
}

func TestLoadMetadataMissingFilePreservesCause(t *testing.T) {
	t.Parallel()

	_, err := LoadMetadata(filepath.Join(t.TempDir(), "missing.json"))
	require.Error(t, err)
	require.ErrorIs(t, err, ErrMetadataRequired)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestParseMetadata(t *testing.T) {
	t.Parallel()

	data := mustReadFixture(t, "parity-keycloak-past.json")

	metadata, err := ParseMetadata(data)
	require.NoError(t, err)
	require.NotNil(t, metadata)
	require.NotNil(t, metadata.AutoAliasing)

	entry := metadata.AutoAliasing.Resources["keycloak_openid_audience_resolve_protocol_mapper"]
	require.NotNil(t, entry)
	require.Equal(t, "keycloak:openid/audienceResolveProtocolMapper:AudienceResolveProtocolMapper", entry.Current)
	require.Len(t, entry.Past, 1)
	require.Equal(t, "keycloak:openid/audienceResolveProtocolMappter:AudienceResolveProtocolMappter", entry.Past[0].Name)
}

func TestParseMetadataInvalidJSON(t *testing.T) {
	t.Parallel()

	_, err := ParseMetadata([]byte(`{"auto-aliasing":`))
	require.Error(t, err)
	require.ErrorIs(t, err, ErrMetadataInvalid)
}

func TestParseMetadataMissingCurrent(t *testing.T) {
	t.Parallel()

	_, err := ParseMetadata(mustReadFixture(t, "malformed-missing-current.json"))
	require.Error(t, err)
	require.ErrorIs(t, err, ErrMetadataInvalid)
}

func TestValidateMetadata(t *testing.T) {
	t.Parallel()

	metadata, err := ParseMetadata(mustReadFixture(t, "parity-vsphere.json"))
	require.NoError(t, err)

	require.NoError(t, ValidateMetadata(metadata))
}

func TestValidateMetadataRequired(t *testing.T) {
	t.Parallel()

	err := ValidateMetadata(&MetadataEnvelope{})
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrMetadataRequired))
}

func TestValidateMetadataVersionUnsupported(t *testing.T) {
	t.Parallel()

	_, err := ParseMetadata(mustReadFixture(t, "unsupported-version.json"))
	require.Error(t, err)
	require.ErrorIs(t, err, ErrMetadataVersionUnsupported)
}

func TestMetadataFixtureParity(t *testing.T) {
	t.Parallel()

	fixtures := []string{"parity-vsphere.json", "parity-keycloak-past.json"}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture, func(t *testing.T) {
			t.Parallel()
			metadata, err := ParseMetadata(mustReadFixture(t, fixture))
			require.NoError(t, err)
			require.NotNil(t, metadata.AutoAliasing)
			require.NotNil(t, metadata.AutoAliasing.Resources)
		})
	}
}

func mustReadFixture(t *testing.T, name string) []byte {
	t.Helper()

	path := filepath.Join("testdata", "metadata", name)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return data
}
