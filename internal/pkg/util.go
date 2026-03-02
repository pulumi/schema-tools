package pkg

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
)

var ErrRepoFileNotFound = errors.New("repo file not found")

type RepoFileNotFoundError struct {
	RepositoryURL  string
	Commit         string
	RepositoryPath string
	Err            error
}

func (e *RepoFileNotFoundError) Error() string {
	return fmt.Sprintf("%s: %s@%s:%s", ErrRepoFileNotFound, e.RepositoryURL, e.Commit, e.RepositoryPath)
}

func (e *RepoFileNotFoundError) Unwrap() error {
	return e.Err
}

func (e *RepoFileNotFoundError) Is(target error) bool {
	return target == ErrRepoFileNotFound
}

func DownloadSchema(ctx context.Context, repositoryUrl string,
	provider string, commit string) (schema.PackageSpec, error) {
	if filePath, isFileURL, err := parseFileRepositoryPath(repositoryUrl); err != nil {
		return schema.PackageSpec{}, err
	} else if isFileURL {
		fileInfo, statErr := os.Stat(filePath)
		switch {
		case statErr == nil && !fileInfo.IsDir():
			// Backward compatibility: historically file: accepted a direct schema.json
			// path. Keep this behavior while also supporting repo-root mode.
			return LoadLocalPackageSpec(filePath)
		case statErr == nil && fileInfo.IsDir():
			// Directory paths use repository-root semantics below.
		case errors.Is(statErr, os.ErrNotExist) && looksLikeSchemaFilePath(filePath):
			// Preserve direct-file error behavior for likely schema file inputs.
			return LoadLocalPackageSpec(filePath)
		case statErr != nil && !errors.Is(statErr, os.ErrNotExist):
			return schema.PackageSpec{}, statErr
		}
	}

	body, err := DownloadRepoFile(ctx, repositoryUrl, provider, commit, StandardSchemaPath(provider))
	if err != nil {
		// Preserve existing schema caller behavior for missing schema files by
		// returning the underlying transport/file error instead of repo-file wrapper.
		var notFoundErr *RepoFileNotFoundError
		if errors.As(err, &notFoundErr) {
			return schema.PackageSpec{}, notFoundErr.Err
		}
		return schema.PackageSpec{}, err
	}

	var sch schema.PackageSpec
	if err = json.Unmarshal(body, &sch); err != nil {
		return schema.PackageSpec{}, err
	}

	return sch, nil
}

func DownloadRepoFile(ctx context.Context, repositoryURL, provider, commit, repositoryPath string) ([]byte, error) {
	return downloadRepositoryFile(
		ctx,
		repositoryURL,
		provider,
		commit,
		repositoryPath,
		true,
	)
}

func downloadRepositoryFile(
	ctx context.Context,
	repositoryURL, provider, commit, repositoryPath string,
	wrapNotFound bool,
) ([]byte, error) {
	root, isFileURL, err := parseFileRepositoryPath(repositoryURL)
	if err != nil {
		return nil, err
	}
	if isFileURL {
		localPath, err := resolveSafeRepoFilePath(root, repositoryPath)
		if err != nil {
			return nil, err
		}
		file, err := os.Open(localPath)
		if err != nil {
			if wrapNotFound && errors.Is(err, os.ErrNotExist) {
				return nil, &RepoFileNotFoundError{
					RepositoryURL:  repositoryURL,
					Commit:         commit,
					RepositoryPath: repositoryPath,
					Err:            err,
				}
			}
			return nil, err
		}
		defer file.Close()
		body, err := readRepoFile(file)
		if err != nil {
			return nil, err
		}
		return body, nil
	}

	gitSource, err := newGitSource(repositoryURL, provider)
	if err != nil {
		return nil, err
	}

	resp, _, err := gitSource.DownloadFile(ctx, commit, repositoryPath, getHTTPResponse)
	if err != nil {
		var downErr *downloadError
		if wrapNotFound && errors.As(err, &downErr) && downErr.code == 404 {
			return nil, &RepoFileNotFoundError{
				RepositoryURL:  repositoryURL,
				Commit:         commit,
				RepositoryPath: repositoryPath,
				Err:            err,
			}
		}
		return nil, err
	}
	defer resp.Close()

	body, err := readRepoFile(resp)
	if err != nil {
		return nil, err
	}

	return body, nil
}

func parseFileRepositoryPath(repositoryURL string) (string, bool, error) {
	repoURL, err := url.Parse(repositoryURL)
	if err != nil {
		return "", false, err
	}
	if repoURL.Scheme != "file" {
		return "", false, nil
	}
	root := repoURL.Path
	if root == "" {
		root = strings.TrimPrefix(repositoryURL, "file:")
	}
	return root, true, nil
}

func looksLikeSchemaFilePath(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".json")
}

func resolveSafeRepoFilePath(root, repositoryPath string) (string, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}

	relPath := filepath.FromSlash(repositoryPath)
	if filepath.IsAbs(relPath) {
		return "", fmt.Errorf("invalid repository path: absolute path not allowed: %q", repositoryPath)
	}
	cleanRel := filepath.Clean(relPath)
	if cleanRel == "." {
		return "", fmt.Errorf("invalid repository path: empty path")
	}

	fullPath := filepath.Join(absRoot, cleanRel)
	withinRoot, err := pathIsWithinRoot(absRoot, fullPath)
	if err != nil {
		return "", err
	}
	if !withinRoot {
		return "", fmt.Errorf("invalid repository path: traversal outside repository root: %q", repositoryPath)
	}

	resolvedRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		// Fall back to lexical root checks when the root cannot be resolved.
		resolvedRoot = absRoot
	}
	resolvedPath, err := filepath.EvalSymlinks(fullPath)
	if err == nil {
		withinResolvedRoot, relErr := pathIsWithinRoot(resolvedRoot, resolvedPath)
		if relErr != nil {
			return "", relErr
		}
		if !withinResolvedRoot {
			return "", fmt.Errorf("invalid repository path: traversal outside repository root: %q", repositoryPath)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	return fullPath, nil
}

func pathIsWithinRoot(root, candidate string) (bool, error) {
	relToRoot, err := filepath.Rel(root, candidate)
	if err != nil {
		return false, err
	}
	if relToRoot == ".." || strings.HasPrefix(relToRoot, ".."+string(filepath.Separator)) {
		return false, nil
	}
	return true, nil
}

func readRepoFile(r io.Reader) ([]byte, error) {
	return io.ReadAll(r)
}

func newGitSource(repositoryURL, provider string) (GitSource, error) {
	repoURL, err := url.Parse(repositoryURL)
	if err != nil {
		return nil, err
	}

	switch repoURL.Scheme {
	case "github":
		return newGithubSource(repoURL, provider)
	case "gitlab":
		return newGitlabSource(repoURL, provider)
	default:
		return nil, fmt.Errorf("unknown schema source scheme: %s", repoURL.Scheme)
	}
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
