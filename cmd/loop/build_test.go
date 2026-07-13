package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type buildTarget struct {
	os   string
	arch string
	ext  string
}

func TestCrossCompileBuilds(t *testing.T) {
	root, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Fatalf("cannot find project root: %v", err)
	}
	root = []byte(strings.TrimSpace(string(root)))

	platforms := []buildTarget{
		{"linux", "amd64", ""},
		{"linux", "arm64", ""},
		{"darwin", "amd64", ""},
		{"darwin", "arm64", ""},
		{"windows", "amd64", ".exe"},
	}

	for _, p := range platforms {
		t.Run(p.os+"/"+p.arch, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "loop-"+p.os+"-"+p.arch+p.ext)
			cmd := exec.Command("go", "build",
				"-ldflags=-X main.Version=test",
				"-o", out,
				"./cmd/loop",
			)
			cmd.Dir = string(root)
			cmd.Env = append(os.Environ(),
				"CGO_ENABLED=0",
				"GOOS="+p.os,
				"GOARCH="+p.arch,
			)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("cross-compile %s/%s failed: %v\n%s", p.os, p.arch, err, output)
			}
			info, err := os.Stat(out)
			if err != nil {
				t.Fatalf("binary not found after build: %v", err)
			}
			if info.Size() == 0 {
				t.Error("binary is empty")
			}
		})
	}
}

func TestNativeBuild(t *testing.T) {
	root, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Fatalf("cannot find project root: %v", err)
	}
	root = []byte(strings.TrimSpace(string(root)))

	out := filepath.Join(t.TempDir(), "loop")
	cmd := exec.Command("go", "build",
		"-ldflags=-X main.Version=test",
		"-o", out,
		"./cmd/loop",
	)
	cmd.Dir = string(root)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("native build failed: %v\n%s", err, output)
	}
	info, err := os.Stat(out)
	if err != nil {
		t.Fatalf("binary not found: %v", err)
	}
	if info.Size() == 0 {
		t.Error("binary is empty")
	}
}

func TestPlatformList(t *testing.T) {
	expected := []string{
		"linux/amd64",
		"linux/arm64",
		"darwin/amd64",
		"darwin/arm64",
		"windows/amd64",
	}
	for _, plat := range expected {
		parts := strings.SplitN(plat, "/", 2)
		if len(parts) != 2 {
			t.Fatalf("invalid platform: %s", plat)
		}
		_, err := exec.Command("go", "tool", "dist", "list").Output()
		if err != nil {
			t.Fatalf("go tool dist list failed: %v", err)
		}
	}
}

func TestBuildVersionLdFlag(t *testing.T) {
	root, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Fatalf("cannot find project root: %v", err)
	}
	root = []byte(strings.TrimSpace(string(root)))

	out := filepath.Join(t.TempDir(), "loop"+exeSuffix())
	cmd := exec.Command("go", "build",
		"-ldflags=-X main.Version=test-version -X main.GOOS=linux -X main.GOARCH=arm64",
		"-o", out,
		"./cmd/loop",
	)
	cmd.Dir = string(root)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build with version failed: %v\n%s", err, output)
	}
	info, err := os.Stat(out)
	if err != nil {
		t.Fatalf("binary not found: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("binary is empty")
	}

	got, err := exec.Command(out, "--version").CombinedOutput()
	if err != nil {
		t.Fatalf("running binary with --version failed: %v", err)
	}
	gotStr := string(got)
	if !strings.Contains(gotStr, "test-version") {
		t.Errorf("expected version output to contain 'test-version', got %q", gotStr)
	}
	if !strings.Contains(gotStr, "linux/arm64") {
		t.Errorf("expected OS/arch 'linux/arm64' in version output, got %q", gotStr)
	}
}

func TestGoReleaserConfigExists(t *testing.T) {
	root, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Fatalf("cannot find project root: %v", err)
	}
	root = []byte(strings.TrimSpace(string(root)))

	path := filepath.Join(string(root), ".goreleaser.yaml")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf(".goreleaser.yaml not found: %v", err)
	}
}

func TestReleaseWorkflowExists(t *testing.T) {
	root, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Fatalf("cannot find project root: %v", err)
	}
	root = []byte(strings.TrimSpace(string(root)))

	path := filepath.Join(string(root), ".github", "workflows", "release.yml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("release.yml not found: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, `tags:`) || !strings.Contains(content, `"v*"`) {
		t.Error("release.yml should trigger on tags matching v*")
	}
	if !strings.Contains(content, `goreleaser/goreleaser-action@v6`) {
		t.Error("release.yml should use goreleaser-action@v6")
	}
}

func TestReleaseWorkflowTriggersOnTags(t *testing.T) {
	root, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Fatalf("cannot find project root: %v", err)
	}
	root = []byte(strings.TrimSpace(string(root)))

	path := filepath.Join(string(root), ".github", "workflows", "release.yml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("release.yml not found: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "on:") {
		t.Error("release.yml missing 'on:' trigger")
	}
	if !strings.Contains(content, "push:") {
		t.Error("release.yml missing 'push:' trigger")
	}
}

func TestMakefileReleaseTarget(t *testing.T) {
	root, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Fatalf("cannot find project root: %v", err)
	}
	root = []byte(strings.TrimSpace(string(root)))

	path := filepath.Join(string(root), "Makefile")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Makefile not found: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "release:") {
		t.Error("Makefile missing 'release:' target")
	}
	if !strings.Contains(content, "make release TAG=") {
		t.Error("Makefile release target should document TAG=v0.1.0 usage")
	}
	if !strings.Contains(content, "git tag -a \"$(TAG)\"") {
		t.Error("Makefile release target should tag with git tag -a")
	}
	if !strings.Contains(content, "git push origin \"$(TAG)\"") {
		t.Error("Makefile release target should push tag to origin")
	}
}

func TestGoReleaserConfigValidate(t *testing.T) {
	if _, err := exec.LookPath("goreleaser"); err != nil {
		t.Skip("goreleaser not installed, skipping validation")
	}

	root, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Fatalf("cannot find project root: %v", err)
	}
	root = []byte(strings.TrimSpace(string(root)))

	cmd := exec.Command("goreleaser", "check", filepath.Join(string(root), ".goreleaser.yaml"))
	cmd.Env = append(os.Environ(), "GITHUB_OWNER=test", "GITHUB_REPO=test")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("goreleaser check failed: %v\n%s", err, output)
	}
}

func TestGoReleaserArchiveNameTemplate(t *testing.T) {
	root, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Fatalf("cannot find project root: %v", err)
	}
	root = []byte(strings.TrimSpace(string(root)))

	data, err := os.ReadFile(filepath.Join(string(root), ".goreleaser.yaml"))
	if err != nil {
		t.Fatalf("cannot read .goreleaser.yaml: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "loop_{{ .Version }}_{{ .Os }}_{{ .Arch }}") {
		t.Error("expected archive name template with Version, Os, and Arch placeholders")
	}
	if !strings.Contains(content, "formats:") {
		t.Error("expected 'formats:' in archives section (goreleaser v2)")
	}
	if !strings.Contains(content, "tar.gz") {
		t.Error("expected tar.gz archive format")
	}
	if !strings.Contains(content, "format_overrides:") {
		t.Error("expected format_overrides for Windows zip")
	}
	if !strings.Contains(content, "checksums.txt") {
		t.Error("expected checksums.txt name template")
	}
}

func TestCIWorkflowExists(t *testing.T) {
	root, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Fatalf("cannot find project root: %v", err)
	}
	root = []byte(strings.TrimSpace(string(root)))

	path := filepath.Join(string(root), ".github", "workflows", "ci.yml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ci.yml not found: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "go test ./...") {
		t.Error("ci.yml should run go test")
	}
	if !strings.Contains(content, "go vet ./...") {
		t.Error("ci.yml should run go vet")
	}
	if !strings.Contains(content, "actions/setup-go@v5") {
		t.Error("ci.yml should use setup-go@v5")
	}
	if !strings.Contains(content, "goreleaser check") {
		t.Error("ci.yml should run goreleaser check")
	}
}

func TestInstallScriptExists(t *testing.T) {
	root, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Fatalf("cannot find project root: %v", err)
	}
	root = []byte(strings.TrimSpace(string(root)))

	path := filepath.Join(string(root), "install.sh")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("install.sh not found: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "#!/usr/bin/env bash") {
		t.Error("install.sh should start with shebang")
	}
	if !strings.Contains(content, "curl") {
		t.Error("install.sh should download from GitHub")
	}
	if !strings.Contains(content, "--version") {
		t.Error("install.sh should support --version flag")
	}
	if !strings.Contains(content, "PREFIX") {
		t.Error("install.sh should support PREFIX")
	}
	if !strings.Contains(content, "BIN_DIR") {
		t.Error("install.sh should support BIN_DIR")
	}
}

func TestCompletionCommand(t *testing.T) {
	root, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Fatalf("cannot find project root: %v", err)
	}
	root = []byte(strings.TrimSpace(string(root)))

	data, err := os.ReadFile(filepath.Join(string(root), "cmd", "loop", "main.go"))
	if err != nil {
		t.Fatalf("cannot read main.go: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "cmdCompletion") {
		t.Error("main.go should define cmdCompletion constant")
	}
	if !strings.Contains(content, "\"completion\"") {
		t.Error("parseArgs should handle 'completion' subcommand")
	}
	if !strings.Contains(content, "runCompletionFn") {
		t.Error("main should invoke runCompletionFn for completion")
	}
	if !strings.Contains(content, "complete -F") {
		t.Error("completion output should contain bash complete directive")
	}
}

func TestMakefileCompletionsTarget(t *testing.T) {
	root, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Fatalf("cannot find project root: %v", err)
	}
	root = []byte(strings.TrimSpace(string(root)))

	data, err := os.ReadFile(filepath.Join(string(root), "Makefile"))
	if err != nil {
		t.Fatalf("Makefile not found: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "completions:") {
		t.Error("Makefile should have completions target")
	}
	if !strings.Contains(content, "loop completion") {
		t.Error("completions target should invoke loop completion")
	}

	cmd := exec.Command("make", "completions")
	cmd.Dir = string(root)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("make completions failed: %v\n%s", err, output)
	}

	outPath := filepath.Join(string(root), "loop_completion.bash")
	completionData, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("cannot read generated completion file: %v", err)
	}

	completionContent := string(completionData)
	if !strings.Contains(completionContent, "_loop_completions") {
		t.Error("completion script should contain the completion function")
	}
	if !strings.Contains(completionContent, "complete -F") {
		t.Error("completion script should contain the complete directive")
	}
	if !strings.Contains(completionContent, "setup run status check repair restore download checksum screenshot completion commands") {
		t.Error("completion script should contain the command list")
	}

	os.Remove(outPath)
}

func TestBuildVersionSemverTag(t *testing.T) {
	root, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Fatalf("cannot find project root: %v", err)
	}
	root = []byte(strings.TrimSpace(string(root)))

	out := filepath.Join(t.TempDir(), "loop"+exeSuffix())
	cmd := exec.Command("go", "build",
		"-ldflags=-X main.Version=v0.1.0 -X main.GOOS=linux -X main.GOARCH=amd64",
		"-o", out,
		"./cmd/loop",
	)
	cmd.Dir = string(root)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build with semver tag failed: %v\n%s", err, output)
	}

	got, err := exec.Command(out, "--version").CombinedOutput()
	if err != nil {
		t.Fatalf("running binary with --version failed: %v", err)
	}
	gotStr := string(got)

	if strings.Contains(gotStr, "vv") {
		t.Errorf("version output should not contain double 'vv', got %q", gotStr)
	}
	if !strings.Contains(gotStr, "v0.1.0") {
		t.Errorf("expected 'v0.1.0' in version output, got %q", gotStr)
	}
	if !strings.Contains(gotStr, "linux/amd64") {
		t.Errorf("expected OS/arch in version output, got %q", gotStr)
	}
}

func TestGoReleaserBuildsMatchPlatforms(t *testing.T) {
	root, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Fatalf("cannot find project root: %v", err)
	}
	root = []byte(strings.TrimSpace(string(root)))

	goreleaserData, err := os.ReadFile(filepath.Join(string(root), ".goreleaser.yaml"))
	if err != nil {
		t.Fatalf("cannot read .goreleaser.yaml: %v", err)
	}

	makefileData, err := os.ReadFile(filepath.Join(string(root), "Makefile"))
	if err != nil {
		t.Fatalf("cannot read Makefile: %v", err)
	}

	goreleaserContent := string(goreleaserData)
	makefileContent := string(makefileData)

	platforms := []struct {
		goos  string
		goarch string
	}{
		{"linux", "amd64"},
		{"linux", "arm64"},
		{"darwin", "amd64"},
		{"darwin", "arm64"},
		{"windows", "amd64"},
	}

	for _, p := range platforms {
		if !strings.Contains(goreleaserContent, "- "+p.goos) {
			t.Errorf(".goreleaser.yaml missing GOOS: %s", p.goos)
		}
		if !strings.Contains(goreleaserContent, "- "+p.goarch) {
			t.Errorf(".goreleaser.yaml missing GOARCH: %s", p.goarch)
		}
		if !strings.Contains(makefileContent, p.goos+"/"+p.goarch) {
			t.Errorf("Makefile BUILD_PLATFORMS missing %s/%s", p.goos, p.goarch)
		}
	}

	if !strings.Contains(goreleaserContent, `CGO_ENABLED=0`) {
		t.Error("expected CGO_ENABLED=0 in goreleaser builds")
	}
}

func TestGoReleaserEntryPoint(t *testing.T) {
	root, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Fatalf("cannot find project root: %v", err)
	}
	root = []byte(strings.TrimSpace(string(root)))

	data, err := os.ReadFile(filepath.Join(string(root), ".goreleaser.yaml"))
	if err != nil {
		t.Fatalf("cannot read .goreleaser.yaml: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "./cmd/loop") {
		t.Error("expected main entrypoint ./cmd/loop in goreleaser config")
	}
	if !strings.Contains(content, "binary: loop") {
		t.Error("expected binary name 'loop' in goreleaser config")
	}
	if !strings.Contains(content, "main.Version") {
		t.Error("expected main.Version ldflag in goreleaser config")
	}
}

func TestLicenseFileExists(t *testing.T) {
	root, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Fatalf("cannot find project root: %v", err)
	}
	root = []byte(strings.TrimSpace(string(root)))

	path := filepath.Join(string(root), "LICENSE")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("LICENSE not found: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "MIT License") {
		t.Error("LICENSE should be MIT")
	}
	if !strings.Contains(content, "Permission is hereby granted") {
		t.Error("LICENSE should contain grant text")
	}
}

func TestReadmeBadges(t *testing.T) {
	root, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Fatalf("cannot find project root: %v", err)
	}
	root = []byte(strings.TrimSpace(string(root)))

	path := filepath.Join(string(root), "README.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("README.md not found: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "img.shields.io/badge/Go-") {
		t.Error("README.md should have Go version badge")
	}
	if !strings.Contains(content, "img.shields.io/github/actions/workflow/status/") {
		t.Error("README.md should have build status badge")
	}
	if !strings.Contains(content, "img.shields.io/badge/license-MIT") {
		t.Error("README.md should have license badge")
	}
}

func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}
