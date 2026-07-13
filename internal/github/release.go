package github

import (
	"fmt"
	"os"
	"path/filepath"
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
// from repo into destDir. Returns the names of downloaded files.
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
	// Fallback: match any version (same OS/arch).
	fallback := fmt.Sprintf("%s_*_%s_%s.", project, goos, goarch)
	for _, f := range files {
		matched, err := filepath.Match(fallback+"*", f)
		if err == nil && matched {
			return f
		}
	}
	return ""
}
