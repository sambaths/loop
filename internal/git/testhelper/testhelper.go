package testhelper

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func Init(t *testing.T, dir string) {
	t.Helper()
	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git init failed: %v\n%s", err, out)
	}
	for _, kv := range []string{"user.name test", "user.email test@test.com"} {
		parts := strings.SplitN(kv, " ", 2)
		cmd := exec.Command("git", "config", parts[0], parts[1])
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git config %s failed: %v\n%s", kv, err, out)
		}
	}
}

func Commit(t *testing.T, dir, msg string) {
	t.Helper()
	cmd := exec.Command("git", "commit", "--allow-empty", "-m", msg)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git commit failed: %v\n%s", err, out)
	}
}

func InitRepo(t *testing.T, dir string) {
	t.Helper()
	Init(t, dir)
	Commit(t, dir, "initial")
}

func WriteFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}
}
