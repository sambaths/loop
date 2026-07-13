package gh

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHasGH(t *testing.T) {
	has := HasGH()
	t.Logf("HasGH() = %v", has)
}

func TestRunGHVersion(t *testing.T) {
	if !HasGH() {
		t.Skip("gh not installed")
	}
	out, err := RunGH("--version")
	if err != nil {
		t.Fatalf("RunGH --version failed: %v", err)
	}
	if !strings.Contains(out, "gh version") {
		t.Errorf("expected 'gh version ...', got %q", out)
	}
}

func TestRunGHNotInstalled(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir)

	_, err := RunGH("--version")
	if err == nil {
		t.Fatal("expected error when gh is not in PATH")
	}
	if !strings.Contains(err.Error(), "gh is not installed") {
		t.Errorf("expected friendly error about gh not installed, got: %v", err)
	}
}

func TestIssueExists(t *testing.T) {
	dir := t.TempDir()
	mockGh := filepath.Join(dir, "gh")
	script := `#!/bin/bash
if [ "$1" = "issue" ] && [ "$2" = "view" ] && [ "$3" = "42" ]; then
	exit 0
fi
if [ "$1" = "issue" ] && [ "$2" = "view" ] && [ "$3" = "999" ]; then
	echo "issue not found" >&2
	exit 1
fi
echo "unexpected args: $*" >&2
	exit 1
`
	if err := os.WriteFile(mockGh, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))

	t.Run("existing issue", func(t *testing.T) {
		if !IssueExists(42) {
			t.Error("expected true for existing issue")
		}
	})

	t.Run("nonexistent issue", func(t *testing.T) {
		if IssueExists(999) {
			t.Error("expected false for nonexistent issue")
		}
	})
}

func TestIssueState(t *testing.T) {
	dir := t.TempDir()
	mockGh := filepath.Join(dir, "gh")
	script := `#!/bin/bash
if [ "$1" = "issue" ] && [ "$2" = "view" ] && [ "$3" = "42" ] && [ "$4" = "--json" ] && [ "$5" = "state" ] && [ "$6" = "--jq" ] && [ "$7" = ".state" ]; then
	echo "OPEN"
	exit 0
fi
if [ "$1" = "issue" ] && [ "$2" = "view" ] && [ "$3" = "43" ] && [ "$4" = "--json" ] && [ "$5" = "state" ] && [ "$6" = "--jq" ] && [ "$7" = ".state" ]; then
	echo "CLOSED"
	exit 0
fi
if [ "$1" = "issue" ] && [ "$2" = "view" ] && [ "$3" = "999" ]; then
	echo "issue not found" >&2
	exit 1
fi
echo "unexpected args: $*" >&2
	exit 1
`
	if err := os.WriteFile(mockGh, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))

	t.Run("open issue", func(t *testing.T) {
		if got := IssueState(42); got != "OPEN" {
			t.Errorf("IssueState(42) = %q, want %q", got, "OPEN")
		}
	})

	t.Run("closed issue", func(t *testing.T) {
		if got := IssueState(43); got != "CLOSED" {
			t.Errorf("IssueState(43) = %q, want %q", got, "CLOSED")
		}
	})

	t.Run("nonexistent issue", func(t *testing.T) {
		if got := IssueState(999); got != "" {
			t.Errorf("IssueState(999) = %q, want empty string", got)
		}
	})
}

func TestAddLabel(t *testing.T) {
	dir := t.TempDir()
	mockGh := filepath.Join(dir, "gh")
	script := `#!/bin/bash
if [ "$1" = "issue" ] && [ "$2" = "edit" ] && [ "$3" = "42" ] && [ "$4" = "--add-label" ] && [ "$5" = "bug" ]; then
	exit 0
fi
if [ "$1" = "issue" ] && [ "$2" = "edit" ] && [ "$3" = "999" ]; then
	echo "issue not found" >&2
	exit 1
fi
echo "unexpected args: $*" >&2
	exit 1
`
	if err := os.WriteFile(mockGh, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))

	t.Run("success", func(t *testing.T) {
		err := AddLabel(42, "bug")
		if err != nil {
			t.Errorf("AddLabel(42, \"bug\") = %v, want nil", err)
		}
	})

	t.Run("nonexistent issue", func(t *testing.T) {
		err := AddLabel(999, "bug")
		if err == nil {
			t.Error("expected error for nonexistent issue")
		}
	})
}

func TestRemoveLabel(t *testing.T) {
	dir := t.TempDir()
	mockGh := filepath.Join(dir, "gh")
	script := `#!/bin/bash
if [ "$1" = "issue" ] && [ "$2" = "edit" ] && [ "$3" = "42" ] && [ "$4" = "--remove-label" ] && [ "$5" = "bug" ]; then
	exit 0
fi
if [ "$1" = "issue" ] && [ "$2" = "edit" ] && [ "$3" = "999" ]; then
	echo "issue not found" >&2
	exit 1
fi
echo "unexpected args: $*" >&2
	exit 1
`
	if err := os.WriteFile(mockGh, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))

	t.Run("success", func(t *testing.T) {
		err := RemoveLabel(42, "bug")
		if err != nil {
			t.Errorf("RemoveLabel(42, \"bug\") = %v, want nil", err)
		}
	})

	t.Run("nonexistent issue", func(t *testing.T) {
		err := RemoveLabel(999, "bug")
		if err == nil {
			t.Error("expected error for nonexistent issue")
		}
	})
}

func TestCloseIssue(t *testing.T) {
	dir := t.TempDir()
	mockGh := filepath.Join(dir, "gh")
	script := `#!/bin/bash
if [ "$1" = "issue" ] && [ "$2" = "close" ] && [ "$3" = "42" ] && [ $# -eq 3 ]; then
	exit 0
fi
if [ "$1" = "issue" ] && [ "$2" = "close" ] && [ "$3" = "42" ] && [ "$4" = "--comment" ] && [ "$5" = "Done" ]; then
	exit 0
fi
if [ "$1" = "issue" ] && [ "$2" = "close" ] && [ "$3" = "999" ]; then
	echo "issue not found" >&2
	exit 1
fi
echo "unexpected args: $*" >&2
	exit 1
`
	if err := os.WriteFile(mockGh, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))

	t.Run("close without comment", func(t *testing.T) {
		err := CloseIssue(42, "")
		if err != nil {
			t.Errorf("CloseIssue(42, \"\") = %v, want nil", err)
		}
	})

	t.Run("close with comment", func(t *testing.T) {
		err := CloseIssue(42, "Done")
		if err != nil {
			t.Errorf("CloseIssue(42, \"Done\") = %v, want nil", err)
		}
	})

	t.Run("nonexistent issue", func(t *testing.T) {
		err := CloseIssue(999, "")
		if err == nil {
			t.Error("expected error for nonexistent issue")
		}
	})
}

func TestReopenIssue(t *testing.T) {
	dir := t.TempDir()
	mockGh := filepath.Join(dir, "gh")
	script := `#!/bin/bash
if [ "$1" = "issue" ] && [ "$2" = "reopen" ] && [ "$3" = "42" ] && [ $# -eq 3 ]; then
	exit 0
fi
if [ "$1" = "issue" ] && [ "$2" = "reopen" ] && [ "$3" = "999" ]; then
	echo "issue not found" >&2
	exit 1
fi
echo "unexpected args: $*" >&2
	exit 1
`
	if err := os.WriteFile(mockGh, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))

	t.Run("reopen existing issue", func(t *testing.T) {
		err := ReopenIssue(42)
		if err != nil {
			t.Errorf("ReopenIssue(42) = %v, want nil", err)
		}
	})

	t.Run("nonexistent issue", func(t *testing.T) {
		err := ReopenIssue(999)
		if err == nil {
			t.Error("expected error for nonexistent issue")
		}
	})
}

func TestCommentOnIssue(t *testing.T) {
	dir := t.TempDir()
	mockGh := filepath.Join(dir, "gh")
	script := `#!/bin/bash
if [ "$1" = "issue" ] && [ "$2" = "comment" ] && [ "$3" = "42" ] && [ "$4" = "--body" ] && [ "$5" = "hello" ]; then
	exit 0
fi
if [ "$1" = "issue" ] && [ "$2" = "comment" ] && [ "$3" = "42" ] && [ "$4" = "--body" ] && [ "$5" = "multi word body" ]; then
	exit 0
fi
if [ "$1" = "issue" ] && [ "$2" = "comment" ] && [ "$3" = "999" ]; then
	echo "issue not found" >&2
	exit 1
fi
echo "unexpected args: $*" >&2
	exit 1
`
	if err := os.WriteFile(mockGh, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))

	t.Run("simple comment", func(t *testing.T) {
		err := CommentOnIssue(42, "hello")
		if err != nil {
			t.Errorf("CommentOnIssue(42, \"hello\") = %v, want nil", err)
		}
	})

	t.Run("multi-word body", func(t *testing.T) {
		err := CommentOnIssue(42, "multi word body")
		if err != nil {
			t.Errorf("CommentOnIssue(42, \"multi word body\") = %v, want nil", err)
		}
	})

	t.Run("nonexistent issue", func(t *testing.T) {
		err := CommentOnIssue(999, "body")
		if err == nil {
			t.Error("expected error for nonexistent issue")
		}
	})
}

func TestIsAuthenticated(t *testing.T) {
	if !HasGH() {
		t.Skip("gh not installed")
	}
	ok := IsAuthenticated()
	t.Logf("IsAuthenticated() = %v", ok)
}
