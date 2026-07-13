package github

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sambaths/loop/internal/issue"
)

func TestRepoFromString(t *testing.T) {
	r, err := RepoFromString("my-org/my-repo")
	if err != nil {
		t.Fatalf("RepoFromString failed: %v", err)
	}
	if r.Owner != "my-org" {
		t.Errorf("expected owner 'my-org', got %q", r.Owner)
	}
	if r.Name != "my-repo" {
		t.Errorf("expected name 'my-repo', got %q", r.Name)
	}
}

func TestRepoFromStringDotsAndHyphens(t *testing.T) {
	r, err := RepoFromString("my-org.dev/my-repo.name")
	if err != nil {
		t.Fatalf("RepoFromString failed: %v", err)
	}
	if r.Owner != "my-org.dev" {
		t.Errorf("expected owner 'my-org.dev', got %q", r.Owner)
	}
	if r.Name != "my-repo.name" {
		t.Errorf("expected name 'my-repo.name', got %q", r.Name)
	}
}

func TestRepoFromStringUppercase(t *testing.T) {
	r, err := RepoFromString("MyOrg/MyRepo")
	if err != nil {
		t.Fatalf("RepoFromString failed: %v", err)
	}
	if r.Owner != "MyOrg" {
		t.Errorf("expected owner 'MyOrg', got %q", r.Owner)
	}
	if r.Name != "MyRepo" {
		t.Errorf("expected name 'MyRepo', got %q", r.Name)
	}
}

func TestRepoFromStringInvalid(t *testing.T) {
	tests := []string{"", "no-slash", "/only-owner", "owner/", "/"}
	for _, s := range tests {
		_, err := RepoFromString(s)
		if err == nil {
			t.Errorf("expected error for %q", s)
		}
	}
}

func TestRepoString(t *testing.T) {
	r := Repo{Owner: "a", Name: "b"}
	if s := r.String(); s != "a/b" {
		t.Errorf("expected 'a/b', got %q", s)
	}
}

func TestStateLabel(t *testing.T) {
	tests := []struct {
		state issue.State
		want  string
	}{
		{issue.StateTodo, "ready-for-agent"},
		{issue.StateReadyForAgent, "ready-for-agent"},
		{issue.StateTestReady, "test-ready"},
		{issue.StateDone, "done"},
		{issue.StateQuarantine, "quarantine"},
		{issue.StateUnknown, ""},
	}
	for _, tc := range tests {
		got := stateLabel(tc.state)
		if got != tc.want {
			t.Errorf("stateLabel(%q) = %q, want %q", tc.state, got, tc.want)
		}
	}
}

func TestIsNotAvailable(t *testing.T) {
	if IsNotAvailable(nil) {
		t.Error("IsNotAvailable(nil) should be false")
	}
	if IsNotAvailable(ErrNoRepo) {
		t.Error("IsNotAvailable(ErrNoRepo) should be false")
	}
	if !IsNotAvailable(ErrNotAvailable) {
		t.Error("IsNotAvailable(ErrNotAvailable) should be true")
	}
}

func TestIsNoRepo(t *testing.T) {
	if IsNoRepo(nil) {
		t.Error("IsNoRepo(nil) should be false")
	}
	if IsNoRepo(ErrNotAvailable) {
		t.Error("IsNoRepo(ErrNotAvailable) should be false")
	}
	if !IsNoRepo(ErrNoRepo) {
		t.Error("IsNoRepo(ErrNoRepo) should be true")
	}
}

func TestCheckInstalled(t *testing.T) {
	err := CheckInstalled()
	if err != nil {
		if !IsNotAvailable(err) {
			t.Fatalf("CheckInstalled error should be ErrNotAvailable, got: %v", err)
		}
	}
}

func TestSyncLabelsNoOpForSameState(t *testing.T) {
	r := Repo{Owner: "test", Name: "repo"}
	err := SyncLabelsForStates(r, 1, issue.StateTodo, issue.StateTodo)
	if err != nil {
		t.Errorf("SyncLabelsForStates for same state should be no-op, got: %v", err)
	}
}

func TestSyncLabelsUnhandledTransition(t *testing.T) {
	r := Repo{Owner: "test", Name: "repo"}
	err := SyncLabelsForStates(r, 1, issue.StateDone, issue.StateTodo)
	if err != nil {
		t.Errorf("SyncLabelsForStates for unknown transition should be no-op, got: %v", err)
	}
}

func TestRepoExists(t *testing.T) {
	mockDir := t.TempDir()
	mockGh := filepath.Join(mockDir, "gh")
	script := `#!/bin/bash
if [ "$1" = "repo" ] && [ "$2" = "view" ] && [ "$3" = "owner/existing-repo" ]; then
	exit 0
fi
if [ "$1" = "repo" ] && [ "$2" = "view" ] && [ "$3" = "owner/nonexistent" ]; then
	echo "gh: Not found" >&2
	exit 1
fi
echo "unexpected args: $*" >&2
exit 1
`
	if err := os.WriteFile(mockGh, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))

	t.Run("existing repo", func(t *testing.T) {
		r := Repo{Owner: "owner", Name: "existing-repo"}
		if !RepoExists(r) {
			t.Error("expected true for existing repo")
		}
	})

	t.Run("nonexistent repo", func(t *testing.T) {
		r := Repo{Owner: "owner", Name: "nonexistent"}
		if RepoExists(r) {
			t.Error("expected false for nonexistent repo")
		}
	})
}

func TestIssueExists(t *testing.T) {
	mockDir := t.TempDir()
	mockGh := filepath.Join(mockDir, "gh")
	script := `#!/bin/bash
if [ "$1" = "issue" ] && [ "$2" = "view" ] && [ "$3" = "42" ] && [ "$4" = "--repo" ] && [ "$5" = "owner/repo" ]; then
	exit 0
fi
if [ "$1" = "issue" ] && [ "$2" = "view" ] && [ "$3" = "999" ] && [ "$4" = "--repo" ] && [ "$5" = "owner/repo" ]; then
	echo "issue not found" >&2
	exit 1
fi
echo "unexpected args: $*" >&2
exit 1
`
	if err := os.WriteFile(mockGh, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))

	t.Run("existing issue", func(t *testing.T) {
		r := Repo{Owner: "owner", Name: "repo"}
		if !IssueExists(r, 42) {
			t.Error("expected true for existing issue")
		}
	})

	t.Run("nonexistent issue", func(t *testing.T) {
		r := Repo{Owner: "owner", Name: "repo"}
		if IssueExists(r, 999) {
			t.Error("expected false for nonexistent issue")
		}
	})
}

func TestIssueLabels(t *testing.T) {
	mockDir := t.TempDir()
	mockGh := filepath.Join(mockDir, "gh")
	script := `#!/bin/bash
if [ "$1" = "issue" ] && [ "$2" = "view" ] && [ "$3" = "42" ] && [ "$4" = "--repo" ] && [ "$5" = "owner/repo" ] && [ "$6" = "--json" ] && [ "$7" = "labels" ] && [ "$8" = "--jq" ]; then
	echo "bug,enhancement,test-ready"
	exit 0
fi
if [ "$1" = "issue" ] && [ "$2" = "view" ] && [ "$3" = "1" ] && [ "$4" = "--repo" ] && [ "$5" = "owner/repo" ] && [ "$6" = "--json" ] && [ "$7" = "labels" ] && [ "$8" = "--jq" ]; then
	echo "done"
	exit 0
fi
if [ "$1" = "issue" ] && [ "$2" = "view" ] && [ "$3" = "2" ] && [ "$4" = "--repo" ] && [ "$5" = "owner/repo" ] && [ "$6" = "--json" ] && [ "$7" = "labels" ] && [ "$8" = "--jq" ]; then
	echo ""
	exit 0
fi
if [ "$1" = "issue" ] && [ "$2" = "view" ] && [ "$3" = "999" ] && [ "$4" = "--repo" ] && [ "$5" = "owner/repo" ] && [ "$6" = "--json" ] && [ "$7" = "labels" ] && [ "$8" = "--jq" ]; then
	echo "issue not found" >&2
	exit 1
fi
echo "unexpected args: $*" >&2
exit 1
`
	if err := os.WriteFile(mockGh, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))

	r := Repo{Owner: "owner", Name: "repo"}

	t.Run("multiple labels", func(t *testing.T) {
		labels, err := IssueLabels(r, 42)
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"bug", "enhancement", "test-ready"}
		if len(labels) != len(want) {
			t.Fatalf("got %d labels, want %d", len(labels), len(want))
		}
		for i := range want {
			if labels[i] != want[i] {
				t.Errorf("label[%d] = %q, want %q", i, labels[i], want[i])
			}
		}
	})

	t.Run("single label", func(t *testing.T) {
		labels, err := IssueLabels(r, 1)
		if err != nil {
			t.Fatal(err)
		}
		if len(labels) != 1 || labels[0] != "done" {
			t.Errorf("got %v, want [done]", labels)
		}
	})

	t.Run("no labels", func(t *testing.T) {
		labels, err := IssueLabels(r, 2)
		if err != nil {
			t.Fatal(err)
		}
		if labels != nil {
			t.Errorf("expected nil, got %v", labels)
		}
	})

	t.Run("nonexistent issue", func(t *testing.T) {
		_, err := IssueLabels(r, 999)
		if err == nil {
			t.Error("expected error for nonexistent issue")
		}
	})
}

func TestRemoveLabelIfExistsDoesNotCrash(t *testing.T) {
	if err := CheckInstalled(); err != nil {
		t.Skip("gh not available:", err)
	}
	r := Repo{Owner: "test", Name: "repo"}
	err := removeLabelIfExists(r, 999999, "nonexistent-label")
	if err == nil {
		return
	}
	_ = err
}

func TestGhReturnsErrNotAvailableWhenNotFound(t *testing.T) {
	// Create a temp dir with no gh binary and set it as the only PATH entry.
	emptyDir := t.TempDir()
	t.Setenv("PATH", emptyDir)

	_, err := gh("--version")
	if err == nil {
		t.Fatal("expected error when gh is not in PATH")
	}
	if !IsNotAvailable(err) {
		t.Errorf("expected IsNotAvailable(err) to be true, got: %v", err)
	}
}

func TestPublicFunctionsReturnErrNotAvailableWhenGhNotFound(t *testing.T) {
	emptyDir := t.TempDir()
	t.Setenv("PATH", emptyDir)
	r := Repo{Owner: "owner", Name: "repo"}

	t.Run("RepoExists", func(t *testing.T) {
		if RepoExists(r) {
			t.Error("expected false when gh is not installed")
		}
	})

	t.Run("IssueExists", func(t *testing.T) {
		if IssueExists(r, 1) {
			t.Error("expected false when gh is not installed")
		}
	})

	t.Run("IssueState", func(t *testing.T) {
		_, err := IssueState(r, 1)
		if err == nil {
			t.Fatal("expected error")
		}
		if !IsNotAvailable(err) {
			t.Errorf("expected IsNotAvailable, got: %v", err)
		}
	})

	t.Run("IssueLabels", func(t *testing.T) {
		_, err := IssueLabels(r, 1)
		if err == nil {
			t.Fatal("expected error")
		}
		if !IsNotAvailable(err) {
			t.Errorf("expected IsNotAvailable, got: %v", err)
		}
	})

	t.Run("AddLabel", func(t *testing.T) {
		err := AddLabel(r, 1, "bug")
		if err == nil {
			t.Fatal("expected error")
		}
		if !IsNotAvailable(err) {
			t.Errorf("expected IsNotAvailable, got: %v", err)
		}
	})

	t.Run("RemoveLabel", func(t *testing.T) {
		err := RemoveLabel(r, 1, "bug")
		if err == nil {
			t.Fatal("expected error")
		}
		if !IsNotAvailable(err) {
			t.Errorf("expected IsNotAvailable, got: %v", err)
		}
	})

	t.Run("CloseIssue", func(t *testing.T) {
		err := CloseIssue(r, 1, "")
		if err == nil {
			t.Fatal("expected error")
		}
		if !IsNotAvailable(err) {
			t.Errorf("expected IsNotAvailable, got: %v", err)
		}
	})

	t.Run("ReopenIssue", func(t *testing.T) {
		err := ReopenIssue(r, 1)
		if err == nil {
			t.Fatal("expected error")
		}
		if !IsNotAvailable(err) {
			t.Errorf("expected IsNotAvailable, got: %v", err)
		}
	})

	t.Run("CommentOnIssue", func(t *testing.T) {
		err := CommentOnIssue(r, 1, "test")
		if err == nil {
			t.Fatal("expected error")
		}
		if !IsNotAvailable(err) {
			t.Errorf("expected IsNotAvailable, got: %v", err)
		}
	})

	t.Run("SyncLabel", func(t *testing.T) {
		err := SyncLabel(r, 1, issue.StateTestReady)
		if err == nil {
			t.Fatal("expected error")
		}
		if !IsNotAvailable(err) {
			t.Errorf("expected IsNotAvailable, got: %v", err)
		}
	})

	t.Run("SyncLabelsForStates", func(t *testing.T) {
		err := SyncLabelsForStates(r, 1, issue.StateTodo, issue.StateTestReady)
		if err == nil {
			t.Fatal("expected error")
		}
		if !IsNotAvailable(err) {
			t.Errorf("expected IsNotAvailable, got: %v", err)
		}
	})

	t.Run("CreateIssue", func(t *testing.T) {
		_, err := CreateIssue(r, "title", "body")
		if err == nil {
			t.Fatal("expected error")
		}
		if !IsNotAvailable(err) {
			t.Errorf("expected IsNotAvailable, got: %v", err)
		}
	})
}

func TestAddLabelMock(t *testing.T) {
	mockDir := t.TempDir()
	mockGh := filepath.Join(mockDir, "gh")
	script := `#!/bin/bash
if [ "$1" = "issue" ] && [ "$2" = "edit" ] && [ "$3" = "42" ] && [ "$4" = "--repo" ] && [ "$5" = "owner/repo" ] && [ "$6" = "--add-label" ] && [ "$7" = "bug" ]; then
	exit 0
fi
if [ "$1" = "issue" ] && [ "$2" = "edit" ] && [ "$3" = "999" ] && [ "$4" = "--repo" ] && [ "$5" = "owner/repo" ] && [ "$6" = "--add-label" ]; then
	echo "issue not found" >&2
	exit 1
fi
echo "unexpected args: $*" >&2
exit 1
`
	if err := os.WriteFile(mockGh, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))
	r := Repo{Owner: "owner", Name: "repo"}

	t.Run("success", func(t *testing.T) {
		err := AddLabel(r, 42, "bug")
		if err != nil {
			t.Errorf("AddLabel = %v, want nil", err)
		}
	})

	t.Run("nonexistent issue", func(t *testing.T) {
		err := AddLabel(r, 999, "bug")
		if err == nil {
			t.Error("expected error for nonexistent issue")
		}
	})
}

func TestRemoveLabelMock(t *testing.T) {
	mockDir := t.TempDir()
	mockGh := filepath.Join(mockDir, "gh")
	script := `#!/bin/bash
if [ "$1" = "issue" ] && [ "$2" = "edit" ] && [ "$3" = "42" ] && [ "$4" = "--repo" ] && [ "$5" = "owner/repo" ] && [ "$6" = "--remove-label" ] && [ "$7" = "bug" ]; then
	exit 0
fi
if [ "$1" = "issue" ] && [ "$2" = "edit" ] && [ "$3" = "999" ] && [ "$4" = "--repo" ] && [ "$5" = "owner/repo" ] && [ "$6" = "--remove-label" ]; then
	echo "issue not found" >&2
	exit 1
fi
echo "unexpected args: $*" >&2
exit 1
`
	if err := os.WriteFile(mockGh, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))
	r := Repo{Owner: "owner", Name: "repo"}

	t.Run("success", func(t *testing.T) {
		err := RemoveLabel(r, 42, "bug")
		if err != nil {
			t.Errorf("RemoveLabel = %v, want nil", err)
		}
	})

	t.Run("nonexistent issue", func(t *testing.T) {
		err := RemoveLabel(r, 999, "bug")
		if err == nil {
			t.Error("expected error for nonexistent issue")
		}
	})
}

func TestCloseIssueMock(t *testing.T) {
	mockDir := t.TempDir()
	mockGh := filepath.Join(mockDir, "gh")
	script := `#!/bin/bash
if [ "$1" = "issue" ] && [ "$2" = "close" ] && [ "$3" = "42" ] && [ "$4" = "--repo" ] && [ "$5" = "owner/repo" ] && [ $# -eq 5 ]; then
	exit 0
fi
if [ "$1" = "issue" ] && [ "$2" = "close" ] && [ "$3" = "42" ] && [ "$4" = "--repo" ] && [ "$5" = "owner/repo" ] && [ "$6" = "--comment" ]; then
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
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))
	r := Repo{Owner: "owner", Name: "repo"}

	t.Run("close without comment", func(t *testing.T) {
		err := CloseIssue(r, 42, "")
		if err != nil {
			t.Errorf("CloseIssue = %v, want nil", err)
		}
	})

	t.Run("close with comment", func(t *testing.T) {
		err := CloseIssue(r, 42, "Done")
		if err != nil {
			t.Errorf("CloseIssue with comment = %v, want nil", err)
		}
	})

	t.Run("nonexistent issue", func(t *testing.T) {
		err := CloseIssue(r, 999, "")
		if err == nil {
			t.Error("expected error for nonexistent issue")
		}
	})
}

func TestReopenIssueMock(t *testing.T) {
	mockDir := t.TempDir()
	mockGh := filepath.Join(mockDir, "gh")
	script := `#!/bin/bash
if [ "$1" = "issue" ] && [ "$2" = "reopen" ] && [ "$3" = "42" ] && [ "$4" = "--repo" ] && [ "$5" = "owner/repo" ] && [ $# -eq 5 ]; then
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
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))
	r := Repo{Owner: "owner", Name: "repo"}

	t.Run("reopen existing issue", func(t *testing.T) {
		err := ReopenIssue(r, 42)
		if err != nil {
			t.Errorf("ReopenIssue = %v, want nil", err)
		}
	})

	t.Run("nonexistent issue", func(t *testing.T) {
		err := ReopenIssue(r, 999)
		if err == nil {
			t.Error("expected error for nonexistent issue")
		}
	})
}

func TestCommentOnIssueMock(t *testing.T) {
	mockDir := t.TempDir()
	mockGh := filepath.Join(mockDir, "gh")
	script := `#!/bin/bash
if [ "$1" = "issue" ] && [ "$2" = "comment" ] && [ "$3" = "42" ] && [ "$4" = "--repo" ] && [ "$5" = "owner/repo" ] && [ "$6" = "--body" ] && [ "$7" = "hello" ]; then
	exit 0
fi
if [ "$1" = "issue" ] && [ "$2" = "comment" ] && [ "$3" = "42" ] && [ "$4" = "--repo" ] && [ "$5" = "owner/repo" ] && [ "$6" = "--body" ]; then
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
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))
	r := Repo{Owner: "owner", Name: "repo"}

	t.Run("simple comment", func(t *testing.T) {
		err := CommentOnIssue(r, 42, "hello")
		if err != nil {
			t.Errorf("CommentOnIssue = %v, want nil", err)
		}
	})

	t.Run("multi-word body", func(t *testing.T) {
		err := CommentOnIssue(r, 42, "multi word body")
		if err != nil {
			t.Errorf("CommentOnIssue multi-word = %v, want nil", err)
		}
	})

	t.Run("nonexistent issue", func(t *testing.T) {
		err := CommentOnIssue(r, 999, "body")
		if err == nil {
			t.Error("expected error for nonexistent issue")
		}
	})
}

func TestSyncLabelMock(t *testing.T) {
	mockDir := t.TempDir()
	mockGh := filepath.Join(mockDir, "gh")
	script := `#!/bin/bash
if [ "$1" = "issue" ] && [ "$2" = "edit" ] && [ "$3" = "42" ] && [ "$4" = "--add-label" ] && [ "$5" = "test-ready" ] && [ "$6" = "--repo" ] && [ "$7" = "owner/repo" ]; then
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
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))
	r := Repo{Owner: "owner", Name: "repo"}

	t.Run("success", func(t *testing.T) {
		err := SyncLabel(r, 42, issue.StateTestReady)
		if err != nil {
			t.Errorf("SyncLabel = %v, want nil", err)
		}
	})

	t.Run("nonexistent issue", func(t *testing.T) {
		err := SyncLabel(r, 999, issue.StateTestReady)
		if err == nil {
			t.Error("expected error for nonexistent issue")
		}
	})
}

func TestAuthCheckMock(t *testing.T) {
	mockDir := t.TempDir()
	mockGh := filepath.Join(mockDir, "gh")
	script := `#!/bin/bash
if [ "$1" = "auth" ] && [ "$2" = "status" ]; then
	exit 0
fi
echo "unexpected args: $*" >&2
exit 1
`
	if err := os.WriteFile(mockGh, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))

	t.Run("authenticated", func(t *testing.T) {
		err := AuthCheck()
		if err != nil {
			t.Errorf("AuthCheck = %v, want nil", err)
		}
	})
}

func TestAuthCheckNotAuthenticated(t *testing.T) {
	mockDir := t.TempDir()
	mockGh := filepath.Join(mockDir, "gh")
	script := `#!/bin/bash
if [ "$1" = "auth" ] && [ "$2" = "status" ]; then
	echo "not logged in" >&2
	exit 1
fi
echo "unexpected args: $*" >&2
exit 1
`
	if err := os.WriteFile(mockGh, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))

	err := AuthCheck()
	if err == nil {
		t.Fatal("expected error when not authenticated")
	}
	if !IsNotAvailable(err) {
		t.Errorf("expected IsNotAvailable, got: %v", err)
	}
}

func TestRemoveLabelIfExistsMock(t *testing.T) {
	mockDir := t.TempDir()
	mockGh := filepath.Join(mockDir, "gh")
	script := `#!/bin/bash
if [ "$1" = "issue" ] && [ "$2" = "edit" ] && [ "$3" = "42" ] && [ "$4" = "--repo" ] && [ "$5" = "owner/repo" ] && [ "$6" = "--remove-label" ] && [ "$7" = "bug" ]; then
	exit 0
fi
if [ "$1" = "issue" ] && [ "$2" = "edit" ] && [ "$3" = "43" ] && [ "$4" = "--repo" ] && [ "$5" = "owner/repo" ] && [ "$6" = "--remove-label" ] && [ "$7" = "nonexistent" ]; then
	echo "label does not exist" >&2
	exit 1
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
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))
	r := Repo{Owner: "owner", Name: "repo"}

	t.Run("label exists", func(t *testing.T) {
		err := removeLabelIfExists(r, 42, "bug")
		if err != nil {
			t.Errorf("removeLabelIfExists = %v, want nil", err)
		}
	})

	t.Run("label does not exist", func(t *testing.T) {
		err := removeLabelIfExists(r, 43, "nonexistent")
		if err != nil {
			t.Errorf("removeLabelIfExists (label missing) = %v, want nil", err)
		}
	})
}

func TestReopenIfClosedClosedIssue(t *testing.T) {
	mockDir := t.TempDir()
	mockGh := filepath.Join(mockDir, "gh")
	script := `#!/bin/bash
if [ "$1" = "issue" ] && [ "$2" = "view" ] && [ "$3" = "42" ] && [ "$4" = "--repo" ] && [ "$5" = "owner/repo" ] && [ "$6" = "--json" ] && [ "$7" = "state" ] && [ "$8" = "--jq" ] && [ "$9" = ".state" ]; then
	echo "CLOSED"
	exit 0
fi
if [ "$1" = "issue" ] && [ "$2" = "reopen" ] && [ "$3" = "42" ] && [ "$4" = "--repo" ] && [ "$5" = "owner/repo" ]; then
	exit 0
fi
if [ "$1" = "issue" ] && [ "$2" = "comment" ] && [ "$3" = "42" ] && [ "$4" = "--repo" ] && [ "$5" = "owner/repo" ] && [ "$6" = "--body" ]; then
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

	reopened, err := ReopenIfClosed(r, 42)
	if err != nil {
		t.Errorf("ReopenIfClosed = %v, want nil", err)
	}
	if !reopened {
		t.Error("expected reopened=true for closed issue")
	}
}

func TestReopenIfClosedOpenIssue(t *testing.T) {
	mockDir := t.TempDir()
	mockGh := filepath.Join(mockDir, "gh")
	script := `#!/bin/bash
if [ "$1" = "issue" ] && [ "$2" = "view" ] && [ "$3" = "42" ] && [ "$4" = "--repo" ] && [ "$5" = "owner/repo" ] && [ "$6" = "--json" ] && [ "$7" = "state" ] && [ "$8" = "--jq" ] && [ "$9" = ".state" ]; then
	echo "OPEN"
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

	reopened, err := ReopenIfClosed(r, 42)
	if err != nil {
		t.Errorf("ReopenIfClosed = %v, want nil", err)
	}
	if reopened {
		t.Error("expected reopened=false for open issue")
	}
}

func TestCreateIssueMock(t *testing.T) {
	mockDir := t.TempDir()
	mockGh := filepath.Join(mockDir, "gh")
	script := `#!/bin/bash
if [ "$1" = "issue" ] && [ "$2" = "create" ] && [ "$3" = "--title" ] && [ "$5" = "--body" ] && [ "$7" = "--repo" ] && [ "$8" = "owner/repo" ] && [ "$9" = "--label" ] && [ "${10}" = "test-ready" ]; then
	echo "https://github.com/owner/repo/issues/42"
	exit 0
fi
if [ "$1" = "issue" ] && [ "$2" = "create" ] && [ "$3" = "--title" ] && [ "$5" = "--body" ] && [ "$7" = "--repo" ] && [ "$8" = "other/repo" ]; then
	echo "unexpected output format" >&2
	exit 0
fi
echo "unexpected args: $*" >&2
exit 1
`
	if err := os.WriteFile(mockGh, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))

	t.Run("success", func(t *testing.T) {
		r := Repo{Owner: "owner", Name: "repo"}
		num, err := CreateIssue(r, "Test Title", "Test Body")
		if err != nil {
			t.Fatalf("CreateIssue = %v, want nil", err)
		}
		if num != 42 {
			t.Errorf("CreateIssue returned %d, want 42", num)
		}
	})

	t.Run("unexpected output format", func(t *testing.T) {
		r := Repo{Owner: "other", Name: "repo"}
		_, err := CreateIssue(r, "Title", "Body")
		if err == nil {
			t.Error("expected error for unexpected output format")
		}
	})
}

func TestSyncLabelsTodoToTestReady(t *testing.T) {
	mockDir := t.TempDir()
	mockGh := filepath.Join(mockDir, "gh")
	script := `#!/bin/bash
if [ "$1" = "issue" ] && [ "$2" = "edit" ] && [ "$3" = "42" ] && [ "$4" = "--repo" ] && [ "$5" = "owner/repo" ] && [ "$6" = "--remove-label" ] && [ "$7" = "ready-for-agent" ]; then
	exit 0
fi
if [ "$1" = "issue" ] && [ "$2" = "edit" ] && [ "$3" = "42" ] && [ "$4" = "--repo" ] && [ "$5" = "owner/repo" ] && [ "$6" = "--remove-label" ] && [ "$7" = "ready-for-human" ]; then
	exit 0
fi
if [ "$1" = "issue" ] && [ "$2" = "edit" ] && [ "$3" = "42" ] && [ "$4" = "--repo" ] && [ "$5" = "owner/repo" ] && [ "$6" = "--add-label" ] && [ "$7" = "test-ready" ]; then
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

	err := SyncLabelsForStates(r, 42, issue.StateTodo, issue.StateTestReady)
	if err != nil {
		t.Errorf("SyncLabelsForStates Todo->TestReady = %v, want nil", err)
	}
}

func TestSyncLabelsTestReadyToDone(t *testing.T) {
	mockDir := t.TempDir()
	mockGh := filepath.Join(mockDir, "gh")
	script := `#!/bin/bash
if [ "$1" = "issue" ] && [ "$2" = "edit" ] && [ "$3" = "42" ] && [ "$4" = "--repo" ] && [ "$5" = "owner/repo" ] && [ "$6" = "--remove-label" ] && [ "$7" = "test-ready" ]; then
	exit 0
fi
if [ "$1" = "issue" ] && [ "$2" = "close" ] && [ "$3" = "42" ] && [ "$4" = "--repo" ] && [ "$5" = "owner/repo" ] && [ $# -eq 5 ]; then
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

	err := SyncLabelsForStates(r, 42, issue.StateTestReady, issue.StateDone)
	if err != nil {
		t.Errorf("SyncLabelsForStates TestReady->Done = %v, want nil", err)
	}
}

func TestSyncLabelsReadyForAgentToTestReady(t *testing.T) {
	mockDir := t.TempDir()
	mockGh := filepath.Join(mockDir, "gh")
	script := `#!/bin/bash
if [ "$1" = "issue" ] && [ "$2" = "edit" ] && [ "$3" = "42" ] && [ "$4" = "--repo" ] && [ "$5" = "owner/repo" ] && [ "$6" = "--remove-label" ] && [ "$7" = "ready-for-agent" ]; then
	exit 0
fi
if [ "$1" = "issue" ] && [ "$2" = "edit" ] && [ "$3" = "42" ] && [ "$4" = "--repo" ] && [ "$5" = "owner/repo" ] && [ "$6" = "--add-label" ] && [ "$7" = "test-ready" ]; then
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

	err := SyncLabelsForStates(r, 42, issue.StateReadyForAgent, issue.StateTestReady)
	if err != nil {
		t.Errorf("SyncLabelsForStates ReadyForAgent->TestReady = %v, want nil", err)
	}
}

func TestSyncLabelsReadyForAgentToTestReadyLabelMissing(t *testing.T) {
	mockDir := t.TempDir()
	mockGh := filepath.Join(mockDir, "gh")
	script := `#!/bin/bash
if [ "$1" = "issue" ] && [ "$2" = "edit" ] && [ "$3" = "42" ] && [ "$4" = "--repo" ] && [ "$5" = "owner/repo" ] && [ "$6" = "--remove-label" ] && [ "$7" = "ready-for-agent" ]; then
	echo "label does not exist" >&2
	exit 1
fi
if [ "$1" = "issue" ] && [ "$2" = "edit" ] && [ "$3" = "42" ] && [ "$4" = "--repo" ] && [ "$5" = "owner/repo" ] && [ "$6" = "--add-label" ] && [ "$7" = "test-ready" ]; then
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

	err := SyncLabelsForStates(r, 42, issue.StateReadyForAgent, issue.StateTestReady)
	if err != nil {
		t.Errorf("SyncLabelsForStates ReadyForAgent->TestReady with missing label = %v, want nil", err)
	}
}

func TestSyncLabelsTestReadyToTodo(t *testing.T) {
	mockDir := t.TempDir()
	mockGh := filepath.Join(mockDir, "gh")
	script := `#!/bin/bash
if [ "$1" = "issue" ] && [ "$2" = "edit" ] && [ "$3" = "42" ] && [ "$4" = "--repo" ] && [ "$5" = "owner/repo" ] && [ "$6" = "--remove-label" ] && [ "$7" = "test-ready" ]; then
	exit 0
fi
if [ "$1" = "issue" ] && [ "$2" = "edit" ] && [ "$3" = "42" ] && [ "$4" = "--repo" ] && [ "$5" = "owner/repo" ] && [ "$6" = "--add-label" ] && [ "$7" = "ready-for-agent" ]; then
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

	err := SyncLabelsForStates(r, 42, issue.StateTestReady, issue.StateTodo)
	if err != nil {
		t.Errorf("SyncLabelsForStates TestReady->Todo = %v, want nil", err)
	}
}

func TestSyncLabelsTodoToTestReadyLabelMissing(t *testing.T) {
	mockDir := t.TempDir()
	mockGh := filepath.Join(mockDir, "gh")
	script := `#!/bin/bash
if [ "$1" = "issue" ] && [ "$2" = "edit" ] && [ "$3" = "42" ] && [ "$4" = "--repo" ] && [ "$5" = "owner/repo" ] && [ "$6" = "--remove-label" ] && [ "$7" = "ready-for-agent" ]; then
	echo "label does not exist" >&2
	exit 1
fi
if [ "$1" = "issue" ] && [ "$2" = "edit" ] && [ "$3" = "42" ] && [ "$4" = "--repo" ] && [ "$5" = "owner/repo" ] && [ "$6" = "--remove-label" ] && [ "$7" = "ready-for-human" ]; then
	echo "label does not exist" >&2
	exit 1
fi
if [ "$1" = "issue" ] && [ "$2" = "edit" ] && [ "$3" = "42" ] && [ "$4" = "--repo" ] && [ "$5" = "owner/repo" ] && [ "$6" = "--add-label" ] && [ "$7" = "test-ready" ]; then
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

	err := SyncLabelsForStates(r, 42, issue.StateTodo, issue.StateTestReady)
	if err != nil {
		t.Errorf("SyncLabelsForStates Todo->TestReady with missing labels = %v, want nil", err)
	}
}

func TestDirToState(t *testing.T) {
	tests := []struct {
		dir  string
		want issue.State
	}{
		{"/path/to/issues/todo", issue.StateTodo},
		{"/path/to/issues/test-ready", issue.StateTestReady},
		{"/path/to/issues/done", issue.StateDone},
		{"/path/to/issues/.quarantine", issue.StateQuarantine},
		{"/path/to/issues", issue.StateTodo},
		{"test-ready", issue.StateTestReady},
		{"done", issue.StateDone},
		{".quarantine", issue.StateQuarantine},
		{"some-unknown", issue.StateTodo},
	}
	for _, tc := range tests {
		got := dirToState(tc.dir)
		if got != tc.want {
			t.Errorf("dirToState(%q) = %q, want %q", tc.dir, got, tc.want)
		}
	}
}

// setupSyncLabelsTest creates a temporary project root with .git and config,
// and changes the working directory to it. Returns the root path.
func setupSyncLabelsTest(t *testing.T) string {
	t.Helper()
	rootDir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(rootDir, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	configDir := filepath.Join(rootDir, ".loop")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte(`{"repo": "owner/repo"}`), 0644); err != nil {
		t.Fatal(err)
	}

	t.Chdir(rootDir)
	return rootDir
}

func setupSyncLabelsTestWithEmptyRepo(t *testing.T) string {
	t.Helper()
	rootDir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(rootDir, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	configDir := filepath.Join(rootDir, ".loop")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte(`{"repo": ""}`), 0644); err != nil {
		t.Fatal(err)
	}

	t.Chdir(rootDir)
	return rootDir
}

func TestSyncLabelsFromTransitionTodoToTestReady(t *testing.T) {
	rootDir := setupSyncLabelsTest(t)
	mockDir := t.TempDir()
	mockGh := filepath.Join(mockDir, "gh")
	script := `#!/bin/bash
if [ "$1" = "issue" ] && [ "$2" = "edit" ] && [ "$3" = "42" ] && [ "$4" = "--repo" ] && [ "$5" = "owner/repo" ] && [ "$6" = "--remove-label" ] && [ "$7" = "ready-for-agent" ]; then
	exit 0
fi
if [ "$1" = "issue" ] && [ "$2" = "edit" ] && [ "$3" = "42" ] && [ "$4" = "--repo" ] && [ "$5" = "owner/repo" ] && [ "$6" = "--remove-label" ] && [ "$7" = "ready-for-human" ]; then
	exit 0
fi
if [ "$1" = "issue" ] && [ "$2" = "edit" ] && [ "$3" = "42" ] && [ "$4" = "--repo" ] && [ "$5" = "owner/repo" ] && [ "$6" = "--add-label" ] && [ "$7" = "test-ready" ]; then
	exit 0
fi
echo "unexpected args: $*" >&2
exit 1
`
	if err := os.WriteFile(mockGh, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))

	transition := &issue.Transition{
		SourceDir: filepath.Join(rootDir, "docs/issues"),
		DestDir:   filepath.Join(rootDir, "docs/issues/test-ready"),
		Filename:  "my-issue.md",
	}
	issueFile := &issue.IssueFile{
		Title:     "My Issue",
		GitHubNum: 42,
		State:     issue.StateTodo,
		FilePath:  filepath.Join(rootDir, "docs/issues/my-issue.md"),
	}

	err := SyncLabels(transition, issueFile)
	if err != nil {
		t.Errorf("SyncLabels Todo->TestReady = %v, want nil", err)
	}
}

func TestSyncLabelsFromTransitionTestReadyToDone(t *testing.T) {
	setupSyncLabelsTest(t)
	mockDir := t.TempDir()
	mockGh := filepath.Join(mockDir, "gh")
	script := `#!/bin/bash
if [ "$1" = "issue" ] && [ "$2" = "edit" ] && [ "$3" = "42" ] && [ "$4" = "--repo" ] && [ "$5" = "owner/repo" ] && [ "$6" = "--remove-label" ] && [ "$7" = "test-ready" ]; then
	exit 0
fi
if [ "$1" = "issue" ] && [ "$2" = "close" ] && [ "$3" = "42" ] && [ "$4" = "--repo" ] && [ "$5" = "owner/repo" ] && [ $# -eq 5 ]; then
	exit 0
fi
echo "unexpected args: $*" >&2
exit 1
`
	if err := os.WriteFile(mockGh, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))

	transition := &issue.Transition{
		DestDir:  "done",
		Filename: "my-issue.md",
	}
	issueFile := &issue.IssueFile{
		Title:     "My Issue",
		GitHubNum: 42,
		State:     issue.StateTestReady,
	}

	err := SyncLabels(transition, issueFile)
	if err != nil {
		t.Errorf("SyncLabels TestReady->Done = %v, want nil", err)
	}
}

func TestSyncLabelsFromTransitionTestReadyToTodo(t *testing.T) {
	setupSyncLabelsTest(t)
	mockDir := t.TempDir()
	mockGh := filepath.Join(mockDir, "gh")
	script := `#!/bin/bash
if [ "$1" = "issue" ] && [ "$2" = "edit" ] && [ "$3" = "42" ] && [ "$4" = "--repo" ] && [ "$5" = "owner/repo" ] && [ "$6" = "--remove-label" ] && [ "$7" = "test-ready" ]; then
	exit 0
fi
if [ "$1" = "issue" ] && [ "$2" = "edit" ] && [ "$3" = "42" ] && [ "$4" = "--repo" ] && [ "$5" = "owner/repo" ] && [ "$6" = "--add-label" ] && [ "$7" = "ready-for-agent" ]; then
	exit 0
fi
echo "unexpected args: $*" >&2
exit 1
`
	if err := os.WriteFile(mockGh, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))

	transition := &issue.Transition{
		DestDir:  "todo",
		Filename: "my-issue.md",
	}
	issueFile := &issue.IssueFile{
		Title:     "My Issue",
		GitHubNum: 42,
		State:     issue.StateTestReady,
	}

	err := SyncLabels(transition, issueFile)
	if err != nil {
		t.Errorf("SyncLabels TestReady->Todo = %v, want nil", err)
	}
}

func TestSyncLabelsFromTransitionNoConfig(t *testing.T) {
	// Use a temp dir without config
	emptyDir := t.TempDir()
	os.MkdirAll(filepath.Join(emptyDir, ".git"), 0755)
	t.Chdir(emptyDir)

	transition := &issue.Transition{
		DestDir: "test-ready",
	}
	issueFile := &issue.IssueFile{
		GitHubNum: 42,
		State:     issue.StateTodo,
	}

	err := SyncLabels(transition, issueFile)
	if !errors.Is(err, ErrNoRepo) {
		t.Errorf("expected ErrNoRepo, got: %v", err)
	}
}

func TestSyncLabelsFromTransitionEmptyRepo(t *testing.T) {
	rootDir := setupSyncLabelsTestWithEmptyRepo(t)

	mockDir := t.TempDir()
	mockGh := filepath.Join(mockDir, "gh")
	ghScript := `#!/bin/bash
echo "should not be called" >&2
exit 1
`
	if err := os.WriteFile(mockGh, []byte(ghScript), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))

	transition := &issue.Transition{
		DestDir:  filepath.Join(rootDir, "docs/issues/test-ready"),
		Filename: "my-issue.md",
	}
	issueFile := &issue.IssueFile{
		Title:     "My Issue",
		GitHubNum: 42,
		State:     issue.StateTodo,
		FilePath:  filepath.Join(rootDir, "docs/issues/my-issue.md"),
	}

	err := SyncLabels(transition, issueFile)
	if err != nil {
		t.Errorf("expected nil (no-op) for empty repo, got: %v", err)
	}
}

func TestIsTransient(t *testing.T) {
	if IsTransient(nil) {
		t.Error("IsTransient(nil) should be false")
	}
	if IsTransient(ErrNotAvailable) {
		t.Error("IsTransient(ErrNotAvailable) should be false")
	}
	if !IsTransient(ErrTransient) {
		t.Error("IsTransient(ErrTransient) should be true")
	}
	if !IsTransient(fmt.Errorf("wrapped: %w", ErrTransient)) {
		t.Error("IsTransient(wrapped ErrTransient) should be true")
	}
}

func TestTransientOutputDetection(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"connection refused", true},
		{"dial tcp: lookup github.com: no such host", true},
		{"i/o timeout", true},
		{"TLS handshake timeout", true},
		{"network is unreachable", true},
		{"Client.Timeout exceeded after 30s", true},
		{"could not resolve host: github.com", true},
		{"EOF", true},
		{"issue not found", false},
		{"GraphQL error: Not Found", false},
		{"", false},
		{"success", false},
	}
	for _, tc := range tests {
		got := isTransientOutput(tc.input)
		if got != tc.want {
			t.Errorf("isTransientOutput(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestGhReturnsTransientOnNetworkError(t *testing.T) {
	mockDir := t.TempDir()
	mockGh := filepath.Join(mockDir, "gh")
	script := `#!/bin/bash
echo "dial tcp: lookup github.com: no such host" >&2
exit 1
`
	if err := os.WriteFile(mockGh, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))

	_, err := gh("issue", "view", "1")
	if err == nil {
		t.Fatal("expected error")
	}
	if !IsTransient(err) {
		t.Errorf("expected IsTransient, got: %v", err)
	}
}

func TestGhReturnsRegularErrorOnNonNetworkFailure(t *testing.T) {
	mockDir := t.TempDir()
	mockGh := filepath.Join(mockDir, "gh")
	script := `#!/bin/bash
echo "GraphQL: Not Found" >&2
exit 1
`
	if err := os.WriteFile(mockGh, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))

	_, err := gh("issue", "view", "999")
	if err == nil {
		t.Fatal("expected error")
	}
	if IsTransient(err) {
		t.Errorf("expected non-transient error, got transient: %v", err)
	}
}

func TestCheckClosedIssuesLogsTransientWarning(t *testing.T) {
	mockDir := t.TempDir()
	mockGh := filepath.Join(mockDir, "gh")
	script := `#!/bin/bash
if [ "$1" = "issue" ] && [ "$2" = "view" ] && [ "$3" = "42" ]; then
	echo "connection refused" >&2
	exit 1
fi
echo "unexpected args: $*" >&2
exit 1
`
	if err := os.WriteFile(mockGh, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))

	r := Repo{Owner: "owner", Name: "repo"}

	dir := t.TempDir()
	p := filepath.Join(dir, "task.md")
	if err := os.WriteFile(p, []byte("# Task\n\nGitHub: #42\nExecution mode: AFK-only\n\n## Parent\n\n## What to build\n\n## User stories covered\n\n## Acceptance criteria\n\n## UAT plan\n\n## Blocked by\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ps := &issue.PipelineState{
		TodoFiles: []string{p},
	}

	rPipe, wPipe, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = wPipe
	defer func() { os.Stderr = old }()

	issues := CheckClosedIssues(ps, r)
	wPipe.Close()

	var buf strings.Builder
	if _, err := io.Copy(&buf, rPipe); err != nil {
		t.Fatal(err)
	}
	stderr := buf.String()

	if len(issues) != 0 {
		t.Errorf("expected 0 issues on transient error, got %d", len(issues))
	}
	if !strings.Contains(stderr, "warning: transient error checking issue #42 state") {
		t.Errorf("expected transient warning on stderr, got: %s", stderr)
	}
}

func TestCheckClosedIssues(t *testing.T) {
	mockDir := t.TempDir()
	mockGh := filepath.Join(mockDir, "gh")
	script := `#!/bin/bash
if [ "$1" = "issue" ] && [ "$2" = "view" ] && [ "$3" = "42" ] && [ "$4" = "--repo" ] && [ "$5" = "owner/repo" ] && [ "$6" = "--json" ] && [ "$7" = "state" ] && [ "$8" = "--jq" ] && [ "$9" = ".state" ]; then
	echo "CLOSED"
	exit 0
fi
if [ "$1" = "issue" ] && [ "$2" = "view" ] && [ "$3" = "43" ] && [ "$4" = "--repo" ] && [ "$5" = "owner/repo" ] && [ "$6" = "--json" ] && [ "$7" = "state" ] && [ "$8" = "--jq" ] && [ "$9" = ".state" ]; then
	echo "OPEN"
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

	t.Run("closed issue in todo", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "task.md")
		if err := os.WriteFile(p, []byte("# Task\n\nGitHub: #42\nExecution mode: AFK-only\n\n## Parent\n\n## What to build\n\n## User stories covered\n\n## Acceptance criteria\n\n## UAT plan\n\n## Blocked by\n"), 0644); err != nil {
			t.Fatal(err)
		}

		ps := &issue.PipelineState{
			TodoFiles: []string{p},
		}
		issues := CheckClosedIssues(ps, r)
		if len(issues) != 1 {
			t.Fatalf("expected 1 issue, got %d", len(issues))
		}
		if issues[0].Severity != issue.SeverityWarning {
			t.Errorf("expected severity %q, got %q", issue.SeverityWarning, issues[0].Severity)
		}
		if !strings.Contains(issues[0].Message, "#42") {
			t.Errorf("expected message to mention #42, got %q", issues[0].Message)
		}
	})

	t.Run("closed issue in test-ready", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "test-ready")
		os.MkdirAll(p, 0755)
		p = filepath.Join(p, "task.md")
		if err := os.WriteFile(p, []byte("# Task\n\nGitHub: #42\nExecution mode: AFK-only\n\n## Parent\n\n## What to build\n\n## User stories covered\n\n## Acceptance criteria\n\n## UAT plan\n\n## Blocked by\n"), 0644); err != nil {
			t.Fatal(err)
		}

		ps := &issue.PipelineState{
			TestReadyFiles: []string{p},
		}
		issues := CheckClosedIssues(ps, r)
		if len(issues) != 1 {
			t.Fatalf("expected 1 issue, got %d", len(issues))
		}
		if !strings.Contains(issues[0].Message, "test-ready") {
			t.Errorf("expected message to mention 'test-ready', got %q", issues[0].Message)
		}
	})

	t.Run("open issue produces no warning", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "task.md")
		if err := os.WriteFile(p, []byte("# Task\n\nGitHub: #43\nExecution mode: AFK-only\n\n## Parent\n\n## What to build\n\n## User stories covered\n\n## Acceptance criteria\n\n## UAT plan\n\n## Blocked by\n"), 0644); err != nil {
			t.Fatal(err)
		}

		ps := &issue.PipelineState{
			TodoFiles: []string{p},
		}
		issues := CheckClosedIssues(ps, r)
		if len(issues) != 0 {
			t.Errorf("expected 0 issues for open issue, got %d", len(issues))
		}
	})

	t.Run("no GitHub number is skipped", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "task.md")
		if err := os.WriteFile(p, []byte("# Task\n\nExecution mode: AFK-only\n\n## Parent\n\n## What to build\n\n## User stories covered\n\n## Acceptance criteria\n\n## UAT plan\n\n## Blocked by\n"), 0644); err != nil {
			t.Fatal(err)
		}

		ps := &issue.PipelineState{
			TodoFiles: []string{p},
		}
		issues := CheckClosedIssues(ps, r)
		if len(issues) != 0 {
			t.Errorf("expected 0 issues for file with no GitHub number, got %d", len(issues))
		}
	})

	t.Run("done files are not checked", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "done")
		os.MkdirAll(p, 0755)
		p = filepath.Join(p, "task.md")
		if err := os.WriteFile(p, []byte("# Task\n\nGitHub: #42\nExecution mode: AFK-only\n\n## Parent\n\n## What to build\n\n## User stories covered\n\n## Acceptance criteria\n\n## UAT plan\n\n## Blocked by\n"), 0644); err != nil {
			t.Fatal(err)
		}

		ps := &issue.PipelineState{
			DoneFiles: []string{p},
		}
		issues := CheckClosedIssues(ps, r)
		if len(issues) != 0 {
			t.Errorf("expected 0 issues for done files, got %d", len(issues))
		}
	})

		t.Run("gh not available is silently skipped", func(t *testing.T) {
		emptyDir := t.TempDir()
		t.Setenv("PATH", emptyDir)

		dir := t.TempDir()
		p := filepath.Join(dir, "task.md")
		if err := os.WriteFile(p, []byte("# Task\n\nGitHub: #42\nExecution mode: AFK-only\n\n## Parent\n\n## What to build\n\n## User stories covered\n\n## Acceptance criteria\n\n## UAT plan\n\n## Blocked by\n"), 0644); err != nil {
			t.Fatal(err)
		}

		ps := &issue.PipelineState{
			TodoFiles: []string{p},
		}
		issues := CheckClosedIssues(ps, r)
		if len(issues) != 0 {
			t.Errorf("expected 0 issues when gh is unavailable, got %d", len(issues))
		}
	})
}

func TestFixMissingLabelsAddsMissingLabel(t *testing.T) {
	mockDir := t.TempDir()
	mockGh := filepath.Join(mockDir, "gh")
	script := `#!/bin/bash
# IssueLabels: return empty (no labels) for #42
if [ "$1" = "issue" ] && [ "$2" = "view" ] && [ "$3" = "42" ] && [ "$4" = "--repo" ] && [ "$5" = "owner/repo" ] && [ "$6" = "--json" ] && [ "$7" = "labels" ] && [ "$8" = "--jq" ]; then
	echo ""
	exit 0
fi
# AddLabel: accept the label add for #42
if [ "$1" = "issue" ] && [ "$2" = "edit" ] && [ "$3" = "42" ] && [ "$4" = "--repo" ] && [ "$5" = "owner/repo" ] && [ "$6" = "--add-label" ] && [ "$7" = "test-ready" ]; then
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
	dir := t.TempDir()
	readyDir := filepath.Join(dir, "test-ready")
	os.MkdirAll(readyDir, 0755)
	p := filepath.Join(readyDir, "task.md")
	if err := os.WriteFile(p, []byte("# Task\n\nGitHub: #42\nExecution mode: AFK-only\n\n## Parent\n\n## What to build\n\n## User stories covered\n\n## Acceptance criteria\n\n## UAT plan\n\n## Blocked by\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ps := &issue.PipelineState{
		TestReadyFiles: []string{p},
	}
	fixed, err := FixMissingLabels(r, ps)
	if err != nil {
		t.Fatalf("FixMissingLabels = %v, want nil", err)
	}
	if len(fixed) != 1 || fixed[0] != 42 {
		t.Errorf("expected [42], got %v", fixed)
	}
}

func TestFixMissingLabelsAlreadyHasLabel(t *testing.T) {
	mockDir := t.TempDir()
	mockGh := filepath.Join(mockDir, "gh")
	script := `#!/bin/bash
# IssueLabels: already has "test-ready"
if [ "$1" = "issue" ] && [ "$2" = "view" ] && [ "$3" = "42" ] && [ "$4" = "--repo" ] && [ "$5" = "owner/repo" ] && [ "$6" = "--json" ] && [ "$7" = "labels" ] && [ "$8" = "--jq" ]; then
	echo "test-ready"
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
	dir := t.TempDir()
	readyDir := filepath.Join(dir, "test-ready")
	os.MkdirAll(readyDir, 0755)
	p := filepath.Join(readyDir, "task.md")
	if err := os.WriteFile(p, []byte("# Task\n\nGitHub: #42\nExecution mode: AFK-only\n\n## Parent\n\n## What to build\n\n## User stories covered\n\n## Acceptance criteria\n\n## UAT plan\n\n## Blocked by\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ps := &issue.PipelineState{
		TestReadyFiles: []string{p},
	}
	fixed, err := FixMissingLabels(r, ps)
	if err != nil {
		t.Fatalf("FixMissingLabels = %v, want nil", err)
	}
	if len(fixed) != 0 {
		t.Errorf("expected 0 fixed (already has label), got %v", fixed)
	}
}

func TestEnsureTestReadyLabelsAddsMissingLabel(t *testing.T) {
	mockDir := t.TempDir()
	mockGh := filepath.Join(mockDir, "gh")
	script := `#!/bin/bash
# IssueLabels: return empty (no labels) for #42
if [ "$1" = "issue" ] && [ "$2" = "view" ] && [ "$3" = "42" ] && [ "$4" = "--repo" ] && [ "$5" = "owner/repo" ] && [ "$6" = "--json" ] && [ "$7" = "labels" ] && [ "$8" = "--jq" ]; then
	echo ""
	exit 0
fi
# AddLabel: accept the label add for #42
if [ "$1" = "issue" ] && [ "$2" = "edit" ] && [ "$3" = "42" ] && [ "$4" = "--repo" ] && [ "$5" = "owner/repo" ] && [ "$6" = "--add-label" ] && [ "$7" = "test-ready" ]; then
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
	dir := t.TempDir()
	readyDir := filepath.Join(dir, "test-ready")
	os.MkdirAll(readyDir, 0755)
	p := filepath.Join(readyDir, "task.md")
	if err := os.WriteFile(p, []byte("# Task\n\nGitHub: #42\nExecution mode: AFK-only\n\n## Parent\n\n## What to build\n\n## User stories covered\n\n## Acceptance criteria\n\n## UAT plan\n\n## Blocked by\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ps := &issue.PipelineState{
		TestReadyFiles: []string{p},
	}
	fixed, err := EnsureTestReadyLabels(r, ps)
	if err != nil {
		t.Fatalf("EnsureTestReadyLabels = %v, want nil", err)
	}
	if len(fixed) != 1 || fixed[0] != 42 {
		t.Errorf("expected [42], got %v", fixed)
	}
}

func TestEnsureTestReadyLabelsAlreadyHasLabel(t *testing.T) {
	mockDir := t.TempDir()
	mockGh := filepath.Join(mockDir, "gh")
	script := `#!/bin/bash
# IssueLabels: already has "test-ready"
if [ "$1" = "issue" ] && [ "$2" = "view" ] && [ "$3" = "42" ] && [ "$4" = "--repo" ] && [ "$5" = "owner/repo" ] && [ "$6" = "--json" ] && [ "$7" = "labels" ] && [ "$8" = "--jq" ]; then
	echo "test-ready"
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
	dir := t.TempDir()
	readyDir := filepath.Join(dir, "test-ready")
	os.MkdirAll(readyDir, 0755)
	p := filepath.Join(readyDir, "task.md")
	if err := os.WriteFile(p, []byte("# Task\n\nGitHub: #42\nExecution mode: AFK-only\n\n## Parent\n\n## What to build\n\n## User stories covered\n\n## Acceptance criteria\n\n## UAT plan\n\n## Blocked by\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ps := &issue.PipelineState{
		TestReadyFiles: []string{p},
	}
	fixed, err := EnsureTestReadyLabels(r, ps)
	if err != nil {
		t.Fatalf("EnsureTestReadyLabels = %v, want nil", err)
	}
	if len(fixed) != 0 {
		t.Errorf("expected 0 fixed (already has label), got %v", fixed)
	}
}

func TestEnsureTestReadyLabelsSkipsNoGitHubNum(t *testing.T) {
	r := Repo{Owner: "owner", Name: "repo"}
	dir := t.TempDir()
	readyDir := filepath.Join(dir, "test-ready")
	os.MkdirAll(readyDir, 0755)
	p := filepath.Join(readyDir, "task.md")
	if err := os.WriteFile(p, []byte("# Task\n\n## Parent\n\n## What to build\n\n## User stories covered\n\n## Acceptance criteria\n\n## UAT plan\n\n## Blocked by\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ps := &issue.PipelineState{
		TestReadyFiles: []string{p},
	}
	fixed, err := EnsureTestReadyLabels(r, ps)
	if err != nil {
		t.Fatalf("EnsureTestReadyLabels = %v, want nil", err)
	}
	if len(fixed) != 0 {
		t.Errorf("expected 0 fixed (no GitHub num), got %v", fixed)
	}
}

func TestEnsureTestReadyLabelsSkipsNonTestReady(t *testing.T) {
	mockDir := t.TempDir()
	mockGh := filepath.Join(mockDir, "gh")
	script := `#!/bin/bash
echo "should not be called" >&2
exit 1
`
	if err := os.WriteFile(mockGh, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))

	r := Repo{Owner: "owner", Name: "repo"}
	dir := t.TempDir()
	todoDir := filepath.Join(dir, "todo")
	os.MkdirAll(todoDir, 0755)
	p := filepath.Join(todoDir, "task.md")
	if err := os.WriteFile(p, []byte("# Task\n\nGitHub: #42\nExecution mode: AFK-only\n\n## Parent\n\n## What to build\n\n## User stories covered\n\n## Acceptance criteria\n\n## UAT plan\n\n## Blocked by\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Only todo file — EnsureTestReadyLabels should not touch it.
	ps := &issue.PipelineState{
		TodoFiles: []string{p},
	}
	fixed, err := EnsureTestReadyLabels(r, ps)
	if err != nil {
		t.Fatalf("EnsureTestReadyLabels = %v, want nil", err)
	}
	if len(fixed) != 0 {
		t.Errorf("expected 0 fixed (only todo file), got %v", fixed)
	}
}

func TestEnsureTestReadyLabelsTransientError(t *testing.T) {
	mockDir := t.TempDir()
	mockGh := filepath.Join(mockDir, "gh")
	script := `#!/bin/bash
if [ "$1" = "issue" ] && [ "$2" = "view" ] && [ "$3" = "42" ]; then
	echo "connection refused" >&2
	exit 1
fi
echo "unexpected args: $*" >&2
exit 1
`
	if err := os.WriteFile(mockGh, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))

	r := Repo{Owner: "owner", Name: "repo"}
	dir := t.TempDir()
	readyDir := filepath.Join(dir, "test-ready")
	os.MkdirAll(readyDir, 0755)
	p := filepath.Join(readyDir, "task.md")
	if err := os.WriteFile(p, []byte("# Task\n\nGitHub: #42\nExecution mode: AFK-only\n\n## Parent\n\n## What to build\n\n## User stories covered\n\n## Acceptance criteria\n\n## UAT plan\n\n## Blocked by\n"), 0644); err != nil {
		t.Fatal(err)
	}

	rPipe, wPipe, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = wPipe
	defer func() { os.Stderr = old }()

	ps := &issue.PipelineState{
		TestReadyFiles: []string{p},
	}
	fixed, err := EnsureTestReadyLabels(r, ps)
	wPipe.Close()
	if err != nil {
		t.Fatalf("EnsureTestReadyLabels = %v, want nil", err)
	}
	if len(fixed) != 0 {
		t.Errorf("expected 0 fixed on transient error, got %v", fixed)
	}

	var buf strings.Builder
	if _, err := io.Copy(&buf, rPipe); err != nil {
		t.Fatal(err)
	}
	stderr := buf.String()
	if !strings.Contains(stderr, "warning: transient error reading labels for #42") {
		t.Errorf("expected transient warning on stderr, got: %s", stderr)
	}
}

func TestEnsureTestReadyLabelsSkipsWhenGhNotAvailable(t *testing.T) {
	emptyDir := t.TempDir()
	t.Setenv("PATH", emptyDir)

	r := Repo{Owner: "owner", Name: "repo"}
	dir := t.TempDir()
	readyDir := filepath.Join(dir, "test-ready")
	os.MkdirAll(readyDir, 0755)
	p := filepath.Join(readyDir, "task.md")
	if err := os.WriteFile(p, []byte("# Task\n\nGitHub: #42\nExecution mode: AFK-only\n\n## Parent\n\n## What to build\n\n## User stories covered\n\n## Acceptance criteria\n\n## UAT plan\n\n## Blocked by\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ps := &issue.PipelineState{
		TestReadyFiles: []string{p},
	}
	fixed, err := EnsureTestReadyLabels(r, ps)
	if err != nil {
		t.Fatalf("EnsureTestReadyLabels = %v, want nil", err)
	}
	if len(fixed) != 0 {
		t.Errorf("expected 0 fixed when gh is unavailable, got %v", fixed)
	}
}

func TestEnsureTestReadyLabelsMultipleFiles(t *testing.T) {
	mockDir := t.TempDir()
	mockGh := filepath.Join(mockDir, "gh")
	script := `#!/bin/bash
# IssueLabels: #42 has no labels, #43 already has test-ready, #44 has other labels but not test-ready
if [ "$1" = "issue" ] && [ "$2" = "view" ] && [ "$3" = "42" ] && [ "$4" = "--repo" ] && [ "$5" = "owner/repo" ] && [ "$6" = "--json" ] && [ "$7" = "labels" ] && [ "$8" = "--jq" ]; then
	echo ""
	exit 0
fi
if [ "$1" = "issue" ] && [ "$2" = "view" ] && [ "$3" = "43" ] && [ "$4" = "--repo" ] && [ "$5" = "owner/repo" ] && [ "$6" = "--json" ] && [ "$7" = "labels" ] && [ "$8" = "--jq" ]; then
	echo "test-ready"
	exit 0
fi
if [ "$1" = "issue" ] && [ "$2" = "view" ] && [ "$3" = "44" ] && [ "$4" = "--repo" ] && [ "$5" = "owner/repo" ] && [ "$6" = "--json" ] && [ "$7" = "labels" ] && [ "$8" = "--jq" ]; then
	echo "bug,enhancement"
	exit 0
fi
# AddLabel for #42 and #44
if [ "$1" = "issue" ] && [ "$2" = "edit" ] && [ "$3" = "42" ] && [ "$4" = "--repo" ] && [ "$5" = "owner/repo" ] && [ "$6" = "--add-label" ] && [ "$7" = "test-ready" ]; then
	exit 0
fi
if [ "$1" = "issue" ] && [ "$2" = "edit" ] && [ "$3" = "44" ] && [ "$4" = "--repo" ] && [ "$5" = "owner/repo" ] && [ "$6" = "--add-label" ] && [ "$7" = "test-ready" ]; then
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
	dir := t.TempDir()
	readyDir := filepath.Join(dir, "test-ready")
	os.MkdirAll(readyDir, 0755)

	p42 := filepath.Join(readyDir, "task42.md")
	if err := os.WriteFile(p42, []byte("# Task 42\n\nGitHub: #42\nExecution mode: AFK-only\n\n## Parent\n\n## What to build\n\n## User stories covered\n\n## Acceptance criteria\n\n## UAT plan\n\n## Blocked by\n"), 0644); err != nil {
		t.Fatal(err)
	}
	p43 := filepath.Join(readyDir, "task43.md")
	if err := os.WriteFile(p43, []byte("# Task 43\n\nGitHub: #43\nExecution mode: AFK-only\n\n## Parent\n\n## What to build\n\n## User stories covered\n\n## Acceptance criteria\n\n## UAT plan\n\n## Blocked by\n"), 0644); err != nil {
		t.Fatal(err)
	}
	p44 := filepath.Join(readyDir, "task44.md")
	if err := os.WriteFile(p44, []byte("# Task 44\n\nGitHub: #44\nExecution mode: AFK-only\n\n## Parent\n\n## What to build\n\n## User stories covered\n\n## Acceptance criteria\n\n## UAT plan\n\n## Blocked by\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ps := &issue.PipelineState{
		TestReadyFiles: []string{p42, p43, p44},
	}
	fixed, err := EnsureTestReadyLabels(r, ps)
	if err != nil {
		t.Fatalf("EnsureTestReadyLabels = %v, want nil", err)
	}
	if len(fixed) != 2 {
		t.Fatalf("expected 2 fixed (42 and 44), got %v", fixed)
	}
	if fixed[0] != 42 || fixed[1] != 44 {
		t.Errorf("expected [42 44], got %v", fixed)
	}
}

func TestFixMissingLabelsSkipsNoGitHubNum(t *testing.T) {
	r := Repo{Owner: "owner", Name: "repo"}
	dir := t.TempDir()
	p := filepath.Join(dir, "task.md")
	if err := os.WriteFile(p, []byte("# Task\n\n## Parent\n\n## What to build\n\n## User stories covered\n\n## Acceptance criteria\n\n## UAT plan\n\n## Blocked by\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ps := &issue.PipelineState{
		TodoFiles: []string{p},
	}
	fixed, err := FixMissingLabels(r, ps)
	if err != nil {
		t.Fatalf("FixMissingLabels = %v, want nil", err)
	}
	if len(fixed) != 0 {
		t.Errorf("expected 0 fixed (no GitHub num), got %v", fixed)
	}
}

func TestCheckIssueExistenceAllExist(t *testing.T) {
	mockDir := t.TempDir()
	mockGh := filepath.Join(mockDir, "gh")
	script := `#!/bin/bash
if [ "$1" = "issue" ] && [ "$2" = "view" ] && [ "$3" = "42" ] && [ "$4" = "--repo" ] && [ "$5" = "owner/repo" ]; then
	exit 0
fi
if [ "$1" = "issue" ] && [ "$2" = "view" ] && [ "$3" = "43" ] && [ "$4" = "--repo" ] && [ "$5" = "owner/repo" ]; then
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
	dir := t.TempDir()
	readyDir := filepath.Join(dir, "test-ready")
	os.MkdirAll(readyDir, 0755)

	p1 := filepath.Join(dir, "task42.md")
	if err := os.WriteFile(p1, []byte("# Task 42\n\nGitHub: #42\nExecution mode: AFK-only\n\n## Parent\n\n## What to build\n\n## User stories covered\n\n## Acceptance criteria\n\n## UAT plan\n\n## Blocked by\n"), 0644); err != nil {
		t.Fatal(err)
	}
	p2 := filepath.Join(readyDir, "task43.md")
	if err := os.WriteFile(p2, []byte("# Task 43\n\nGitHub: #43\nExecution mode: AFK-only\n\n## Parent\n\n## What to build\n\n## User stories covered\n\n## Acceptance criteria\n\n## UAT plan\n\n## Blocked by\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ps := &issue.PipelineState{
		TodoFiles:      []string{p1},
		TestReadyFiles: []string{p2},
	}

	issues := CheckIssueExistence(ps, r)
	if len(issues) != 0 {
		t.Errorf("expected 0 issues (all exist), got %d", len(issues))
	}
}

func TestCheckIssueExistenceNonExistent(t *testing.T) {
	mockDir := t.TempDir()
	mockGh := filepath.Join(mockDir, "gh")
	script := `#!/bin/bash
if [ "$1" = "issue" ] && [ "$2" = "view" ] && [ "$3" = "42" ] && [ "$4" = "--repo" ] && [ "$5" = "owner/repo" ]; then
	exit 0
fi
if [ "$1" = "issue" ] && [ "$2" = "view" ] && [ "$3" = "999" ] && [ "$4" = "--repo" ] && [ "$5" = "owner/repo" ]; then
	echo "issue not found" >&2
	exit 1
fi
echo "unexpected args: $*" >&2
exit 1
`
	if err := os.WriteFile(mockGh, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))

	r := Repo{Owner: "owner", Name: "repo"}
	dir := t.TempDir()
	readyDir := filepath.Join(dir, "test-ready")
	os.MkdirAll(readyDir, 0755)

	pExisting := filepath.Join(dir, "task42.md")
	if err := os.WriteFile(pExisting, []byte("# Task 42\n\nGitHub: #42\nExecution mode: AFK-only\n\n## Parent\n\n## What to build\n\n## User stories covered\n\n## Acceptance criteria\n\n## UAT plan\n\n## Blocked by\n"), 0644); err != nil {
		t.Fatal(err)
	}
	pMissing := filepath.Join(readyDir, "task999.md")
	if err := os.WriteFile(pMissing, []byte("# Task 999\n\nGitHub: #999\nExecution mode: AFK-only\n\n## Parent\n\n## What to build\n\n## User stories covered\n\n## Acceptance criteria\n\n## UAT plan\n\n## Blocked by\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ps := &issue.PipelineState{
		TodoFiles:      []string{pExisting},
		TestReadyFiles: []string{pMissing},
	}

	issues := CheckIssueExistence(ps, r)
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue (non-existent), got %d", len(issues))
	}
	if !strings.Contains(issues[0].Message, "#999") {
		t.Errorf("expected message to mention #999, got %q", issues[0].Message)
	}
	if issues[0].Severity != issue.SeverityError {
		t.Errorf("expected severity %q, got %q", issue.SeverityError, issues[0].Severity)
	}
	if !strings.Contains(issues[0].FilePath, "task999.md") {
		t.Errorf("expected file path to contain task999.md, got %q", issues[0].FilePath)
	}
}

func TestCheckIssueExistenceSkipsNoGitHubNum(t *testing.T) {
	r := Repo{Owner: "owner", Name: "repo"}
	dir := t.TempDir()
	p := filepath.Join(dir, "task.md")
	if err := os.WriteFile(p, []byte("# Task\n\n## Parent\n\n## What to build\n\n## User stories covered\n\n## Acceptance criteria\n\n## UAT plan\n\n## Blocked by\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ps := &issue.PipelineState{
		TodoFiles: []string{p},
	}

	issues := CheckIssueExistence(ps, r)
	if len(issues) != 0 {
		t.Errorf("expected 0 issues (no GitHub num), got %d", len(issues))
	}
}

func TestCheckIssueExistenceSkipsDoneFiles(t *testing.T) {
	mockDir := t.TempDir()
	mockGh := filepath.Join(mockDir, "gh")
	script := `#!/bin/bash
echo "should not be called" >&2
exit 1
`
	if err := os.WriteFile(mockGh, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))

	r := Repo{Owner: "owner", Name: "repo"}
	dir := t.TempDir()
	doneDir := filepath.Join(dir, "done")
	os.MkdirAll(doneDir, 0755)
	p := filepath.Join(doneDir, "done.md")
	if err := os.WriteFile(p, []byte("# Done\n\nGitHub: #42\n\n## Parent\n\n## What to build\n\n## User stories covered\n\n## Acceptance criteria\n\n## UAT plan\n\n## Blocked by\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ps := &issue.PipelineState{
		DoneFiles: []string{p},
	}

	issues := CheckIssueExistence(ps, r)
	if len(issues) != 0 {
		t.Errorf("expected 0 issues (done files skipped), got %d", len(issues))
	}
}

func TestCheckIssueExistenceTransientError(t *testing.T) {
	mockDir := t.TempDir()
	mockGh := filepath.Join(mockDir, "gh")
	script := `#!/bin/bash
if [ "$1" = "issue" ] && [ "$2" = "view" ] && [ "$3" = "42" ] && [ "$4" = "--repo" ] && [ "$5" = "owner/repo" ]; then
	echo "connection refused" >&2
	exit 1
fi
echo "unexpected args: $*" >&2
exit 1
`
	if err := os.WriteFile(mockGh, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))

	r := Repo{Owner: "owner", Name: "repo"}
	dir := t.TempDir()
	p := filepath.Join(dir, "task42.md")
	if err := os.WriteFile(p, []byte("# Task 42\n\nGitHub: #42\nExecution mode: AFK-only\n\n## Parent\n\n## What to build\n\n## User stories covered\n\n## Acceptance criteria\n\n## UAT plan\n\n## Blocked by\n"), 0644); err != nil {
		t.Fatal(err)
	}

	rPipe, wPipe, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = wPipe
	defer func() { os.Stderr = old }()

	ps := &issue.PipelineState{
		TodoFiles: []string{p},
	}

	issues := CheckIssueExistence(ps, r)
	wPipe.Close()

	var stderrBuf strings.Builder
	if _, err := io.Copy(&stderrBuf, rPipe); err != nil {
		t.Fatal(err)
	}
	stderr := stderrBuf.String()

	if len(issues) != 0 {
		t.Errorf("expected 0 issues on transient error, got %d", len(issues))
	}
	if !strings.Contains(stderr, "warning: transient error checking issue #42") {
		t.Errorf("expected transient warning on stderr, got: %s", stderr)
	}
}

func TestCheckIssueExistenceSkipsMalformedFiles(t *testing.T) {
	mockDir := t.TempDir()
	mockGh := filepath.Join(mockDir, "gh")
	script := `#!/bin/bash
echo "should not be called" >&2
exit 1
`
	if err := os.WriteFile(mockGh, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))

	r := Repo{Owner: "owner", Name: "repo"}
	dir := t.TempDir()
	emptyPath := filepath.Join(dir, "empty.md")
	if err := os.WriteFile(emptyPath, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	ps := &issue.PipelineState{
		TodoFiles: []string{emptyPath},
	}

	issues := CheckIssueExistence(ps, r)
	if len(issues) != 0 {
		t.Errorf("expected 0 issues (malformed file skipped), got %d", len(issues))
	}
}

func TestCheckIssueExistenceReadyForAgentFiles(t *testing.T) {
	mockDir := t.TempDir()
	mockGh := filepath.Join(mockDir, "gh")
	script := `#!/bin/bash
if [ "$1" = "issue" ] && [ "$2" = "view" ] && [ "$3" = "100" ] && [ "$4" = "--repo" ] && [ "$5" = "owner/repo" ]; then
	exit 0
fi
if [ "$1" = "issue" ] && [ "$2" = "view" ] && [ "$3" = "999" ] && [ "$4" = "--repo" ] && [ "$5" = "owner/repo" ]; then
	echo "issue not found" >&2
	exit 1
fi
echo "unexpected args: $*" >&2
exit 1
`
	if err := os.WriteFile(mockGh, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", mockDir+":"+os.Getenv("PATH"))

	r := Repo{Owner: "owner", Name: "repo"}
	dir := t.TempDir()
	rfaDir := filepath.Join(dir, "ready-for-agent")
	os.MkdirAll(rfaDir, 0755)

	pExisting := filepath.Join(rfaDir, "task100.md")
	if err := os.WriteFile(pExisting, []byte("# Task 100\n\nGitHub: #100\nExecution mode: AFK-only\n\n## Parent\n\n## What to build\n\n## User stories covered\n\n## Acceptance criteria\n\n## UAT plan\n\n## Blocked by\n"), 0644); err != nil {
		t.Fatal(err)
	}
	pMissing := filepath.Join(rfaDir, "task999.md")
	if err := os.WriteFile(pMissing, []byte("# Task 999\n\nGitHub: #999\nExecution mode: AFK-only\n\n## Parent\n\n## What to build\n\n## User stories covered\n\n## Acceptance criteria\n\n## UAT plan\n\n## Blocked by\n"), 0644); err != nil {
		t.Fatal(err)
	}

	ps := &issue.PipelineState{
		ReadyForAgentFiles: []string{pExisting, pMissing},
	}

	issues := CheckIssueExistence(ps, r)
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue (non-existent in ready-for-agent), got %d", len(issues))
	}
	if !strings.Contains(issues[0].Message, "#999") {
		t.Errorf("expected message to mention #999, got %q", issues[0].Message)
	}
}
