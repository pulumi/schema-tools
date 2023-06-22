package pkg

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/logging"
	"github.com/pulumi/pulumi/sdk/v3/go/common/version"
)

// GitSource deals with downloading individual files from a specific git repository over HTTPS
type GitSource interface {
	// Download fetches an io.ReadCloser for the schema file to download and also returns the size of the response (if known).
	Download(
		commit string,
		getHTTPResponse func(*http.Request) (io.ReadCloser, int64, error)) (io.ReadCloser, int64, error)
}

// gitlabSource can download a plugin from gitlab releases.
type gitlabSource struct {
	host    string
	owner   string
	project string
	name    string

	token string
}

// Creates a new GitLab source from a gitlab://<host>/<project_id> url.
// Uses the GITLAB_TOKEN environment variable for authentication if it's set.
func newGitlabSource(url *url.URL, name string) (*gitlabSource, error) {
	contract.Requiref(url.Scheme == "gitlab", "url", `scheme must be "gitlab", was %q`, url.Scheme)

	host := url.Host
	parts := strings.Split(strings.Trim(url.Path, "/"), "/")

	if host == "" {
		return nil, fmt.Errorf("gitlab:// url must have a host part, was: %s", url)
	}

	if len(parts) != 1 && len(parts) != 2 {
		return nil, fmt.Errorf(
			"gitlab:// url must have the format <host>/<owner>[/<repository>], was: %s",
			url)
	}

	owner := parts[0]
	if owner == "" {
		return nil, fmt.Errorf(
			"gitlab:// url must have the format <host>/<owner>[/<repository>], was: %s",
			url)
	}

	repository := "pulumi-" + name
	if len(parts) == 2 {
		repository = parts[1]
	}

	return &gitlabSource{
		host:    host,
		owner:   owner,
		project: repository,
		name:    name,

		token: os.Getenv("GITLAB_TOKEN"),
	}, nil
}

func (source *gitlabSource) newHTTPRequest(url, accept string) (*http.Request, error) {
	var authorization string
	if source.token != "" {
		authorization = fmt.Sprintf("Bearer %s", source.token)
	}

	req, err := buildHTTPRequest(url, authorization)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", accept)
	return req, nil
}

func (source *gitlabSource) Download(
	commit string,
	getHTTPResponse func(*http.Request) (io.ReadCloser, int64, error),
) (io.ReadCloser, int64, error) {
	assetName := url.QueryEscape(StandardSchemaPath(source.name))
	project := url.QueryEscape(fmt.Sprintf("%s/%s", source.owner, source.project))

	// Gitlab Files API: https://docs.gitlab.com/ee/api/repository_files.html
	assetURL := fmt.Sprintf(
		"https://%s/api/v4/projects/%s/repository/files/%s/raw?ref=%s",
		source.host, project, assetName, commit)
	logging.V(1).Infof("%s downloading from %s", source.name, assetURL)

	req, err := source.newHTTPRequest(assetURL, "application/octet-stream")
	if err != nil {
		return nil, -1, err
	}
	return getHTTPResponse(req)
}

// githubSource can download a plugin from github releases
type githubSource struct {
	host         string
	organization string
	repository   string
	name         string

	token string
}

// Creates a new github source adding authentication data in the environment, if it exists
func newGithubSource(url *url.URL, name string) (*githubSource, error) {
	contract.Requiref(url.Scheme == "github", "url", `scheme must be "github", was %q`, url.Scheme)

	host := url.Host
	parts := strings.Split(strings.Trim(url.Path, "/"), "/")

	if host == "" {
		return nil, fmt.Errorf("github:// url must have a host part, was: %s", url)
	}

	if len(parts) != 1 && len(parts) != 2 {
		return nil, fmt.Errorf(
			"github:// url must have the format <host>/<organization>[/<repository>], was: %s",
			url)
	}

	organization := parts[0]
	if organization == "" {
		return nil, fmt.Errorf(
			"github:// url must have the format <host>/<organization>[/<repository>], was: %s",
			url)
	}

	repository := "pulumi-" + name
	if len(parts) == 2 {
		repository = parts[1]
	}

	return &githubSource{
		host:         host,
		organization: organization,
		repository:   repository,
		name:         name,

		token: os.Getenv("GITHUB_TOKEN"),
	}, nil
}

func (source *githubSource) newHTTPRequest(url, accept string) (*http.Request, error) {
	var authorization string
	if source.token != "" {
		authorization = fmt.Sprintf("token %s", source.token)
	}

	req, err := buildHTTPRequest(url, authorization)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", accept)
	return req, nil
}

func (source *githubSource) getHTTPResponse(
	getHTTPResponse func(*http.Request) (io.ReadCloser, int64, error),
	req *http.Request,
) (io.ReadCloser, int64, error) {
	resp, length, err := getHTTPResponse(req)
	if err == nil {
		return resp, length, nil
	}

	// Wrap 403 rate limit errors with a more helpful message.
	var downErr *downloadError
	if !errors.As(err, &downErr) || downErr.code != 403 {
		return nil, -1, err
	}

	// This is a rate limiting error only if x-ratelimit-remaining is 0.
	// https://docs.github.com/en/rest/overview/resources-in-the-rest-api?apiVersion=2022-11-28#exceeding-the-rate-limit
	if downErr.header.Get("x-ratelimit-remaining") != "0" {
		return nil, -1, err
	}

	tryAgain := "."
	if reset, err := strconv.ParseInt(downErr.header.Get("x-ratelimit-reset"), 10, 64); err == nil {
		delay := time.Until(time.Unix(reset, 0).UTC())
		tryAgain = fmt.Sprintf(", try again in %s.", delay)
	}

	addAuth := ""
	if source.token == "" {
		addAuth = " You can set GITHUB_TOKEN to make an authenticated request with a higher rate limit."
	}

	logging.Errorf("GitHub rate limit exceeded for %s%s%s", req.URL, tryAgain, addAuth)
	return nil, -1, fmt.Errorf("rate limit exceeded: %w", err)
}

func (source *githubSource) Download(
	commit string,
	getHTTPResponse func(*http.Request) (io.ReadCloser, int64, error),
) (io.ReadCloser, int64, error) {
	schemaURL := fmt.Sprintf(
		"https://%s/repos/%s/%s/contents/%s?ref=%s",
		source.host, source.organization, source.repository, StandardSchemaPath(source.name), commit)
	logging.V(9).Infof("plugin GitHub schema url: %s", schemaURL)

	req, err := source.newHTTPRequest(schemaURL, "application/vnd.github.v4.raw")
	if err != nil {
		return nil, -1, err
	}
	return source.getHTTPResponse(getHTTPResponse, req)
}

func buildHTTPRequest(pluginEndpoint string, authorization string) (*http.Request, error) {
	req, err := http.NewRequest("GET", pluginEndpoint, nil)
	if err != nil {
		return nil, err
	}

	userAgent := fmt.Sprintf("pulumi-cli/1 (%s; %s)", version.Version, runtime.GOOS)
	req.Header.Set("User-Agent", userAgent)

	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}

	return req, nil
}

func getHTTPResponse(req *http.Request) (io.ReadCloser, int64, error) {
	logging.V(9).Infof("full plugin download url: %s", req.URL)
	// This logs at level 11 because it could include authentication headers, we reserve log level 11 for
	// detailed api logs that may include credentials.
	logging.V(11).Infof("plugin install request headers: %v", req.Header)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, -1, err
	}

	// As above this might include authentication information, but also to be consistent at what level headers
	// print at.
	logging.V(11).Infof("plugin install response headers: %v", resp.Header)

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		contract.IgnoreClose(resp.Body)
		return nil, -1, newDownloadError(resp.StatusCode, req.URL, resp.Header)
	}

	return resp.Body, resp.ContentLength, nil
}

// func getHTTPResponseWithRetry(req *http.Request) (io.ReadCloser, int64, error) {
// 	logging.V(9).Infof("full plugin download url: %s", req.URL)
// 	// This logs at level 11 because it could include authentication headers, we reserve log level 11 for
// 	// detailed api logs that may include credentials.
// 	logging.V(11).Infof("plugin install request headers: %v", req.Header)

// 	resp, err := httputil.DoWithRetry(req, http.DefaultClient)
// 	if err != nil {
// 		return nil, -1, err
// 	}

// 	// As above this might include authentication information, but also to be consistent at what level headers
// 	// print at.
// 	logging.V(11).Infof("plugin install response headers: %v", resp.Header)

// 	if resp.StatusCode < 200 || resp.StatusCode > 299 {
// 		contract.IgnoreClose(resp.Body)
// 		return nil, -1, newDownloadError(resp.StatusCode, req.URL, resp.Header)
// 	}

// 	return resp.Body, resp.ContentLength, nil
// }

// downloadError is an error that happened during the HTTP download of a plugin.
type downloadError struct {
	msg    string
	code   int
	header http.Header
}

func (e *downloadError) Error() string {
	return e.msg
}

// Create a new downloadError with a message that indicates GITHUB_TOKEN should be set.
func newGithubPrivateRepoError(statusCode int, url *url.URL) error {
	return &downloadError{
		code: statusCode,
		msg: fmt.Sprintf("%d HTTP error fetching schema from %s. "+
			"If this is a private GitHub repository, try "+
			"providing a token via the GITHUB_TOKEN environment variable. "+
			"See: https://github.com/settings/tokens",
			statusCode, url),
	}
}

// Create a new downloadError.
func newDownloadError(statusCode int, url *url.URL, header http.Header) error {
	if url.Host == "api.github.com" && statusCode == 404 {
		return newGithubPrivateRepoError(statusCode, url)
	}
	return &downloadError{
		code:   statusCode,
		msg:    fmt.Sprintf("%d HTTP error fetching schema from %s", statusCode, url),
		header: header,
	}
}

// StandardSchemaPath returns the standard name for the asset that contains the given plugin.
func StandardSchemaPath(provider string) string {
	return fmt.Sprintf("provider/cmd/pulumi-resource-%s/schema.json", provider)
}
