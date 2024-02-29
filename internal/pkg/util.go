package pkg

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
)

func DownloadSchema(ctx context.Context, repositoryUrl string,
	provider string, commit string) (schema.PackageSpec, error) {
	var gitSource GitSource
	// Support schematised URLS if the URL has a "schema" part we recognize
	url, err := url.Parse(repositoryUrl)
	if err != nil {
		return schema.PackageSpec{}, err
	}

	switch url.Scheme {
	case "file":
		return LoadLocalPackageSpec(strings.TrimPrefix(repositoryUrl, "file:"))
	case "github":
		gitSource, err = newGithubSource(url, provider)
	case "gitlab":
		gitSource, err = newGitlabSource(url, provider)
	default:
		return schema.PackageSpec{}, fmt.Errorf("unknown schema source scheme: %s", url.Scheme)
	}
	if err != nil {
		return schema.PackageSpec{}, err
	}

	resp, _, err := gitSource.Download(ctx, commit, getHTTPResponse)
	if err != nil {
		return schema.PackageSpec{}, err
	}
	defer resp.Close()

	body, err := io.ReadAll(resp)
	if err != nil {
		return schema.PackageSpec{}, err
	}

	var sch schema.PackageSpec
	if err = json.Unmarshal(body, &sch); err != nil {
		return schema.PackageSpec{}, err
	}

	return sch, nil
}

func LoadLocalPackageSpec(filePath string) (schema.PackageSpec, error) {
	body, err := os.ReadFile(filePath)
	if err != nil {
		return schema.PackageSpec{}, err
	}

	var sch schema.PackageSpec
	if err = json.Unmarshal(body, &sch); err != nil {
		return schema.PackageSpec{}, err
	}

	return sch, nil
}
