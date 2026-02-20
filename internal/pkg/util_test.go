package pkg

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/h2non/gock"
	"github.com/stretchr/testify/assert"
)

func TestDownloadValidGithubOrg(t *testing.T) {
	defer gock.Off()

	gock.New("https://api.github.com").
		Get("/repos/pulumiverse/pulumi-unifi/contents/provider/cmd/pulumi-resource-unifi/schema.json").
		MatchParam("ref", "main").
		Reply(200).
		File("schema.json")

	spec, err := DownloadSchema(context.Background(),
		"github://api.github.com/pulumiverse", "unifi", "main")

	assert.Nil(t, err)
	assert.NotNil(t, spec)
	// Spec parsing is already tested in Pulumi codebase
	assert.Equal(t, "test", spec.Name)
}

func TestDownloadValidGithubRepo(t *testing.T) {
	defer gock.Off()

	gock.New("https://api.github.com").
		Get("/repos/pulumiverse/pulumi-unifi/contents/provider/cmd/pulumi-resource-unifi/schema.json").
		MatchParam("ref", "main").
		Reply(200).
		File("schema.json")

	spec, err := DownloadSchema(context.Background(),
		"github://api.github.com/pulumiverse/pulumi-unifi", "unifi", "main")

	assert.Nil(t, err)
	assert.NotNil(t, spec)
	// Spec parsing is already tested in Pulumi codebase
	assert.Equal(t, "test", spec.Name)
}

func TestDownloadUnknownGithubRef(t *testing.T) {
	defer gock.Off()

	gock.New("https://api.github.com").
		Get("/repos/pulumiverse/pulumi-unifi/contents/provider/cmd/pulumi-resource-unifi/schema.json").
		MatchParam("ref", "unknown").
		Reply(404)

	_, err := DownloadSchema(context.Background(),
		"github://api.github.com/pulumiverse/pulumi-unifi", "unifi", "unknown")

	assert.NotNil(t, err)
	assert.Equal(t, "404 HTTP error fetching schema from https://api.github.com/repos/pulumiverse/pulumi-unifi/contents/provider/cmd/pulumi-resource-unifi/schema.json?ref=unknown. If this is a private GitHub repository, try providing a token via the GITHUB_TOKEN environment variable. See: https://github.com/settings/tokens", err.Error())
}

func TestDownloadValidGitlabOwner(t *testing.T) {
	defer gock.Off()

	gock.New("https://gitlab.com").
		Get("/api/v4/projects/pulumiverse/pulumi-unifi/repository/files/provider/cmd/pulumi-resource-unifi/schema.json/raw").
		MatchParam("ref", "main").
		Reply(200).
		File("schema.json")

	spec, err := DownloadSchema(context.Background(), "gitlab://gitlab.com/pulumiverse", "unifi", "main")

	assert.Nil(t, err)
	assert.NotNil(t, spec)
	// Spec parsing is already tested in Pulumi codebase
	assert.Equal(t, "test", spec.Name)
}

func TestDownloadValidGitlabRepo(t *testing.T) {
	defer gock.Off()

	gock.New("https://gitlab.com").
		Get("/api/v4/projects/pulumiverse/pulumi-unifi/repository/files/provider/cmd/pulumi-resource-unifi/schema.json/raw").
		MatchParam("ref", "main").
		Reply(200).
		File("schema.json")

	spec, err := DownloadSchema(context.Background(), "gitlab://gitlab.com/pulumiverse/pulumi-unifi", "unifi", "main")

	assert.Nil(t, err)
	assert.NotNil(t, spec)
	// Spec parsing is already tested in Pulumi codebase
	assert.Equal(t, "test", spec.Name)
}

func TestDownloadUnknownGitlabRef(t *testing.T) {
	defer gock.Off()

	gock.New("https://gitlab.com").
		Get("/api/v4/projects/pulumiverse/pulumi-unifi/repository/files/provider/cmd/pulumi-resource-unifi/schema.json/raw").
		MatchParam("ref", "unknown").
		Reply(404)

	_, err := DownloadSchema(context.Background(), "gitlab://gitlab.com/pulumiverse/pulumi-unifi", "unifi", "unknown")

	assert.NotNil(t, err)
	assert.Equal(t, "404 HTTP error fetching schema from https://gitlab.com/api/v4/projects/pulumiverse%2Fpulumi-unifi/repository/files/provider%2Fcmd%2Fpulumi-resource-unifi%2Fschema.json/raw?ref=unknown", err.Error())
}

func TestDownloadSchemaFileRepositoryRoot(t *testing.T) {
	tmpDir := t.TempDir()
	schemaPath := filepath.Join(tmpDir, filepath.FromSlash(StandardSchemaPath("unifi")))
	assert.NoError(t, os.MkdirAll(filepath.Dir(schemaPath), 0o755))
	assert.NoError(t, os.WriteFile(schemaPath, []byte(`{"name":"test"}`), 0o600))

	spec, err := DownloadSchema(context.Background(), "file:"+tmpDir, "unifi", "local")

	assert.NoError(t, err)
	assert.Equal(t, "test", spec.Name)
}

func TestDownloadSchemaFileRepositorySchemaPathCompatibility(t *testing.T) {
	tmpDir := t.TempDir()
	schemaPath := filepath.Join(tmpDir, "schema.json")
	assert.NoError(t, os.WriteFile(schemaPath, []byte(`{"name":"legacy"}`), 0o600))

	spec, err := DownloadSchema(context.Background(), "file:"+schemaPath, "unifi", "local")

	assert.NoError(t, err)
	assert.Equal(t, "legacy", spec.Name)
}

func TestDownloadSchemaFileRepositorySchemaPathMissing(t *testing.T) {
	missingPath := filepath.Join(t.TempDir(), "schema.json")

	_, err := DownloadSchema(context.Background(), "file:"+missingPath, "unifi", "local")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), missingPath)
	assert.NotContains(t, err.Error(), StandardSchemaPath("unifi"))
}

func TestDownloadRepoFileMetadataGithub(t *testing.T) {
	defer gock.Off()

	gock.New("https://api.github.com").
		Get("/repos/pulumiverse/pulumi-unifi/contents/provider/cmd/pulumi-resource-unifi/bridge-metadata.json").
		MatchParam("ref", "main").
		Reply(200).
		BodyString(`{"auto-aliasing":{}}`)

	body, err := DownloadRepoFile(context.Background(),
		"github://api.github.com/pulumiverse/pulumi-unifi", "unifi", "main", StandardMetadataPath("unifi"))

	assert.NoError(t, err)
	assert.Equal(t, `{"auto-aliasing":{}}`, string(body))
}

func TestDownloadRepoFileMetadataGitlab(t *testing.T) {
	defer gock.Off()

	gock.New("https://gitlab.com").
		Get("/api/v4/projects/pulumiverse/pulumi-unifi/repository/files/provider/cmd/pulumi-resource-unifi/bridge-metadata.json/raw").
		MatchParam("ref", "main").
		Reply(200).
		BodyString(`{"auto-aliasing":{}}`)

	body, err := DownloadRepoFile(context.Background(),
		"gitlab://gitlab.com/pulumiverse/pulumi-unifi", "unifi", "main", StandardMetadataPath("unifi"))

	assert.NoError(t, err)
	assert.Equal(t, `{"auto-aliasing":{}}`, string(body))
}

func TestDownloadRepoFileMetadataNotFound(t *testing.T) {
	defer gock.Off()

	gock.New("https://api.github.com").
		Get("/repos/pulumiverse/pulumi-unifi/contents/provider/cmd/pulumi-resource-unifi/bridge-metadata.json").
		MatchParam("ref", "unknown").
		Reply(404)

	_, err := DownloadRepoFile(context.Background(),
		"github://api.github.com/pulumiverse/pulumi-unifi", "unifi", "unknown", StandardMetadataPath("unifi"))

	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrRepoFileNotFound))
	assert.Equal(t,
		"repo file not found: github://api.github.com/pulumiverse/pulumi-unifi@unknown:provider/cmd/pulumi-resource-unifi/bridge-metadata.json",
		err.Error())
}

func TestDownloadRepoFileMetadataHTTPErrorNon404(t *testing.T) {
	defer gock.Off()

	gock.New("https://api.github.com").
		Get("/repos/pulumiverse/pulumi-unifi/contents/provider/cmd/pulumi-resource-unifi/bridge-metadata.json").
		MatchParam("ref", "main").
		Reply(500)

	_, err := DownloadRepoFile(context.Background(),
		"github://api.github.com/pulumiverse/pulumi-unifi", "unifi", "main", StandardMetadataPath("unifi"))

	assert.Error(t, err)
	assert.False(t, errors.Is(err, ErrRepoFileNotFound))
	assert.Contains(t, err.Error(), "500 HTTP error fetching schema")
}

func TestEncodePathSegments(t *testing.T) {
	assert.Equal(
		t,
		"provider/cmd/pulumi-resource-unifi/bridge%20metadata%23v1.json",
		encodePathSegments("provider/cmd/pulumi-resource-unifi/bridge metadata#v1.json"),
	)
}

func TestEncodeGitLabRepositoryPath(t *testing.T) {
	assert.Equal(
		t,
		"provider%2Fcmd%2Fpulumi-resource-unifi%2Fbridge%20metadata%23v1.json",
		encodeGitLabRepositoryPath("provider/cmd/pulumi-resource-unifi/bridge metadata#v1.json"),
	)
}

func TestDownloadRepoFileMetadataFileRepository(t *testing.T) {
	tmpDir := t.TempDir()
	metadataPath := filepath.Join(tmpDir, filepath.FromSlash(StandardMetadataPath("unifi")))
	assert.NoError(t, os.MkdirAll(filepath.Dir(metadataPath), 0o755))
	assert.NoError(t, os.WriteFile(metadataPath, []byte(`{"auto-aliasing":{}}`), 0o600))

	body, err := DownloadRepoFile(
		context.Background(),
		"file:"+tmpDir,
		"unifi",
		"local",
		StandardMetadataPath("unifi"),
	)

	assert.NoError(t, err)
	assert.Equal(t, `{"auto-aliasing":{}}`, string(body))
}

func TestDownloadRepoFileMetadataFileRepositoryNotFound(t *testing.T) {
	tmpDir := t.TempDir()

	_, err := DownloadRepoFile(
		context.Background(),
		"file:"+tmpDir,
		"unifi",
		"local",
		StandardMetadataPath("unifi"),
	)

	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrRepoFileNotFound))
	assert.Equal(
		t,
		"repo file not found: file:"+tmpDir+"@local:provider/cmd/pulumi-resource-unifi/bridge-metadata.json",
		err.Error(),
	)
}

func TestReadRepoFileAllowsLargeMetadataPayload(t *testing.T) {
	payloadSize := 20 << 20
	payload := strings.Repeat("a", payloadSize)

	body, err := readRepoFile(strings.NewReader(payload))

	assert.NoError(t, err)
	assert.Len(t, body, payloadSize)
}

func TestResolveSafeRepoFilePathRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	_, err := resolveSafeRepoFilePath(root, "../secret.txt")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "traversal outside repository root")
}

func TestResolveSafeRepoFilePathRejectsAbsolutePath(t *testing.T) {
	root := t.TempDir()
	_, err := resolveSafeRepoFilePath(root, "/etc/passwd")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "absolute path not allowed")
}

func TestResolveSafeRepoFilePathRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	target := filepath.Join(outside, "secret.json")
	assert.NoError(t, os.WriteFile(target, []byte(`{"secret":true}`), 0o600))

	linkPath := filepath.Join(root, "link")
	if err := os.Symlink(outside, linkPath); err != nil {
		t.Skipf("symlink not supported in test environment: %v", err)
	}

	_, err := resolveSafeRepoFilePath(root, "link/secret.json")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "traversal outside repository root")
}
