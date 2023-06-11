package pkg

import (
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

	spec, err := DownloadSchema("github://api.github.com/pulumiverse", "unifi", "main")

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

	spec, err := DownloadSchema("github://api.github.com/pulumiverse/pulumi-unifi", "unifi", "main")

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

	_, err := DownloadSchema("github://api.github.com/pulumiverse/pulumi-unifi", "unifi", "unknown")

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

	spec, err := DownloadSchema("gitlab://gitlab.com/pulumiverse", "unifi", "main")

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

	spec, err := DownloadSchema("gitlab://gitlab.com/pulumiverse/pulumi-unifi", "unifi", "main")

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

	_, err := DownloadSchema("gitlab://gitlab.com/pulumiverse/pulumi-unifi", "unifi", "unknown")

	assert.NotNil(t, err)
	assert.Equal(t, "404 HTTP error fetching schema from https://gitlab.com/api/v4/projects/pulumiverse%2Fpulumi-unifi/repository/files/provider%2Fcmd%2Fpulumi-resource-unifi%2Fschema.json/raw?ref=unknown", err.Error())
}
