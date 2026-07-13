package github

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// ArchivePattern returns the release asset glob pattern for the current
// platform based on the given project name.
func ArchivePattern(project string) string {
	if len(project) == 0 {
		project = "loop"
	}
	return fmt.Sprintf("%s_*_*_*", project)
}

// DownloadLatestRelease downloads the latest release assets matching pattern
// from repo into destDir using the gh CLI. Returns the names of downloaded files.
func DownloadLatestRelease(repo Repo, pattern, destDir string) ([]string, error) {
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return nil, fmt.Errorf("create destination directory: %w", err)
	}

	_, err := gh("release", "download",
		"--repo", repo.String(),
		"--pattern", pattern,
		"--dir", destDir,
	)
	if err != nil {
		return nil, fmt.Errorf("download release: %w", err)
	}

	entries, err := os.ReadDir(destDir)
	if err != nil {
		return nil, fmt.Errorf("read destination: %w", err)
	}

	var files []string
	for _, e := range entries {
		if !e.IsDir() {
			files = append(files, e.Name())
		}
	}
	return files, nil
}

type releaseResponse struct {
	TagName string          `json:"tag_name"`
	Assets  []assetResponse `json:"assets"`
}

type assetResponse struct {
	Name       string `json:"name"`
	BrowserURL string `json:"browser_download_url"`
}

// DownloadLatestAsset downloads the latest release asset for the current
// platform from the given GitHub repo using the public GitHub API.
// No gh CLI or authentication required. Returns the names of downloaded files.
func DownloadLatestAsset(owner, name, destDir string) ([]string, error) {
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return nil, fmt.Errorf("create destination directory: %w", err)
	}

	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", owner, name)
	resp, err := http.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("fetch release info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var release releaseResponse
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("parse release info: %w", err)
	}

	ext := "tar.gz"
	if runtime.GOOS == "windows" {
		ext = "zip"
	}

	v := strings.TrimPrefix(release.TagName, "v")
	wantPrefix := fmt.Sprintf("%s_%s_%s_%s.", name, v, runtime.GOOS, runtime.GOARCH)

	var matchURL, matchName string
	for _, asset := range release.Assets {
		if strings.HasPrefix(asset.Name, wantPrefix) && strings.HasSuffix(asset.Name, "."+ext) {
			matchURL = asset.BrowserURL
			matchName = asset.Name
			break
		}
	}

	if matchURL == "" {
		return nil, fmt.Errorf("no release asset found for %s/%s (%s/%s)", owner, name, runtime.GOOS, runtime.GOARCH)
	}

	destPath := filepath.Join(destDir, matchName)
	out, err := os.Create(destPath)
	if err != nil {
		return nil, fmt.Errorf("create file: %w", err)
	}
	defer out.Close()

	assetResp, err := http.Get(matchURL)
	if err != nil {
		return nil, fmt.Errorf("download asset: %w", err)
	}
	defer assetResp.Body.Close()

	if _, err := io.Copy(out, assetResp.Body); err != nil {
		return nil, fmt.Errorf("write asset: %w", err)
	}

	return []string{matchName}, nil
}

// LatestTag returns the tag name of the latest release for the given repo.
func LatestTag(repo Repo) (string, error) {
	out, err := gh("release", "view",
		"--repo", repo.String(),
		"--json", "tagName",
		"--jq", ".tagName",
	)
	if err != nil {
		return "", fmt.Errorf("view latest release: %w", err)
	}
	return strings.TrimSpace(out), nil
}

// PlatformArchive builds the expected archive filename for the given project,
// version, OS, and architecture.
func PlatformArchive(project, version, goos, goarch string) string {
	ext := "tar.gz"
	if goos == "windows" {
		ext = "zip"
	}
	v := strings.TrimPrefix(version, "v")
	return fmt.Sprintf("%s_%s_%s_%s.%s", project, v, goos, goarch, ext)
}

// FindMatchingArchive returns the first downloaded file that matches the
// expected archive name pattern. Tries exact version match first, then
// any-version fallback.
func FindMatchingArchive(files []string, project, version, goos, goarch string) string {
	prefix := fmt.Sprintf("%s_%s_%s_%s.", project, version, goos, goarch)
	for _, f := range files {
		if strings.HasPrefix(f, prefix) {
			return f
		}
	}
	fallback := fmt.Sprintf("%s_*_%s_%s.", project, goos, goarch)
	for _, f := range files {
		matched, err := filepath.Match(fallback+"*", f)
		if err == nil && matched {
			return f
		}
	}
	return ""
}
