package github

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestArchivePattern(t *testing.T) {
	p := ArchivePattern("loop")
	if !strings.Contains(p, "loop") {
		t.Errorf("ArchivePattern should contain project name, got %q", p)
	}
}

func TestPlatformArchive(t *testing.T) {
	tests := []struct {
		project, version, goos, goarch, want string
	}{
		{"loop", "v0.1.0", "linux", "amd64", "loop_0.1.0_linux_amd64.tar.gz"},
		{"loop", "v0.1.0", "darwin", "arm64", "loop_0.1.0_darwin_arm64.tar.gz"},
		{"loop", "v0.1.0", "windows", "amd64", "loop_0.1.0_windows_amd64.zip"},
		{"loop", "0.1.0", "linux", "amd64", "loop_0.1.0_linux_amd64.tar.gz"},
	}
	for _, tc := range tests {
		got := PlatformArchive(tc.project, tc.version, tc.goos, tc.goarch)
		if got != tc.want {
			t.Errorf("PlatformArchive(%q, %q, %q, %q) = %q, want %q", tc.project, tc.version, tc.goos, tc.goarch, got, tc.want)
		}
	}
}

func TestPlatformArchiveNoVersionPrefix(t *testing.T) {
	got := PlatformArchive("loop", "0.1.0", "linux", "amd64")
	want := "loop_0.1.0_linux_amd64.tar.gz"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestFindMatchingArchive(t *testing.T) {
	files := []string{
		"loop_0.1.0_linux_amd64.tar.gz",
		"loop_0.1.0_darwin_arm64.tar.gz",
		"checksums.txt",
	}
	got := FindMatchingArchive(files, "loop", "0.1.0", "linux", "amd64")
	want := "loop_0.1.0_linux_amd64.tar.gz"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestFindMatchingArchiveNoMatch(t *testing.T) {
	files := []string{
		"loop_0.1.0_darwin_arm64.tar.gz",
		"checksums.txt",
	}
	got := FindMatchingArchive(files, "loop", "0.1.0", "linux", "amd64")
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestLatestTag(t *testing.T) {
	mockDir := t.TempDir()
	mockGh := filepath.Join(mockDir, "gh")
	script := `#!/bin/bash
if [ "$1" = "release" ] && [ "$2" = "view" ] && [ "$3" = "--repo" ] && [ "$4" = "owner/repo" ]; then
	echo "v0.1.0"
	exit 0
fi
echo "unexpected args: $*" >&2
exit 1
`
	if err := os.WriteFile(mockGh, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))

	r := Repo{Owner: "owner", Name: "repo"}
	tag, err := LatestTag(r)
	if err != nil {
		t.Fatalf("LatestTag failed: %v", err)
	}
	if tag != "v0.1.0" {
		t.Errorf("expected v0.1.0, got %q", tag)
	}
}

func TestLatestTagTransientError(t *testing.T) {
	mockDir := t.TempDir()
	mockGh := filepath.Join(mockDir, "gh")
	script := `#!/bin/bash
echo "connection refused" >&2
exit 1
`
	if err := os.WriteFile(mockGh, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))

	r := Repo{Owner: "owner", Name: "repo"}
	_, err := LatestTag(r)
	if err == nil {
		t.Fatal("expected error for transient failure")
	}
	if !IsTransient(err) {
		t.Errorf("expected transient error, got %v", err)
	}
}

func TestDownloadLatestRelease(t *testing.T) {
	mockDir := t.TempDir()
	mockGh := filepath.Join(mockDir, "gh")
	script := `#!/bin/bash
# Simulate gh release download: find --dir argument and write dummy file
while [ $# -gt 0 ]; do
	if [ "$1" = "--dir" ]; then
		touch "$2/loop_0.1.0_linux_amd64.tar.gz"
		break
	fi
	shift
done
echo "downloaded"
exit 0
`
	if err := os.WriteFile(mockGh, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))

	destDir := t.TempDir()
	r := Repo{Owner: "owner", Name: "repo"}

	files, err := DownloadLatestRelease(r, "loop_*", destDir)
	if err != nil {
		t.Fatalf("DownloadLatestRelease failed: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("expected at least one downloaded file")
	}

	found := false
	for _, f := range files {
		if strings.Contains(f, "loop_") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected loop archive in downloaded files, got %v", files)
	}
}

func TestDownloadLatestReleaseFailure(t *testing.T) {
	mockDir := t.TempDir()
	mockGh := filepath.Join(mockDir, "gh")
	script := `#!/bin/bash
echo "release not found" >&2
exit 1
`
	if err := os.WriteFile(mockGh, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))

	r := Repo{Owner: "owner", Name: "nonexistent-repo"}
	_, err := DownloadLatestRelease(r, "loop_*", t.TempDir())
	if err == nil {
		t.Fatal("expected error for failed download")
	}
}

func TestPlatformArchiveWindows(t *testing.T) {
	got := PlatformArchive("loop", "v0.2.0", "windows", "amd64")
	want := "loop_0.2.0_windows_amd64.zip"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestFindMatchingArchiveExact(t *testing.T) {
	files := []string{
		"loop_0.1.0_linux_amd64.tar.gz",
		"checksums.txt",
	}
	got := FindMatchingArchive(files, "loop", "0.1.0", "linux", "amd64")
	if got != "loop_0.1.0_linux_amd64.tar.gz" {
		t.Errorf("expected %q, got %q", "loop_0.1.0_linux_amd64.tar.gz", got)
	}
}

func TestFindMatchingArchiveFallback(t *testing.T) {
	// Fallback: when no version prefix match, try glob without version
	files := []string{
		"loop_1.0.0_linux_amd64.tar.gz",
	}
	got := FindMatchingArchive(files, "loop", "2.0.0", "linux", "amd64")
	if got != "loop_1.0.0_linux_amd64.tar.gz" {
		t.Errorf("expected fallback match %q, got %q", "loop_1.0.0_linux_amd64.tar.gz", got)
	}
}

func TestFindMatchingArchiveNotFound(t *testing.T) {
	files := []string{
		"loop_0.1.0_linux_amd64.tar.gz",
	}
	got := FindMatchingArchive(files, "loop", "0.1.0", "windows", "amd64")
	if got != "" {
		t.Errorf("expected no match, got %q", got)
	}
}
