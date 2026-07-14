package git

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var ErrStashConflict = errors.New("stash conflict")

const stashPrefix = "loop-auto-save"
const GitContextFileName = "git-context.json"

var noRepoWarned bool

func warnNoRepo() {
	if !noRepoWarned {
		noRepoWarned = true
		fmt.Fprintln(os.Stderr, "warning: not a git repository — continuing without git safety")
	}
}

// ResetNoRepoWarning resets the no-repo warning flag so it can fire again.
// Used in tests.
func ResetNoRepoWarning() {
	noRepoWarned = false
}

func RunGit(args ...string) (stdout, stderr string, err error) {
	var outBuf, errBuf bytes.Buffer
	cmd := exec.Command("git", args...)
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err = cmd.Run()
	if errors.Is(err, exec.ErrNotFound) {
		return "", "", fmt.Errorf("git is not installed or not found in PATH — install it from https://git-scm.com/")
	}
	return strings.TrimSpace(outBuf.String()), strings.TrimSpace(errBuf.String()), err
}

func HasGit() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

func IsRepo() bool {
	_, _, err := RunGit("rev-parse", "--is-inside-work-tree")
	return err == nil
}

func StashChanges() (bool, error) {
	status, stderr, err := RunGit("status", "--porcelain")
	if err != nil {
		return false, fmt.Errorf("check status: %s: %w", stderr, err)
	}
	if status == "" {
		return false, nil
	}
	ts := time.Now().Format(time.RFC3339)
	_, popStderr, popErr := RunGit("stash", "push", "--include-untracked", "-m", stashPrefix+" "+ts)
	if popErr != nil {
		return false, fmt.Errorf("stash failed: %s: %w", popStderr, popErr)
	}
	return true, nil
}

func StashApply() error {
	_, stderr, err := RunGit("stash", "apply")
	if err != nil {
		if strings.Contains(stderr, "No stash entries found") {
			return nil
		}
		return err
	}
	return nil
}

func StashDrop() error {
	_, stderr, err := RunGit("stash", "drop")
	if err != nil {
		if strings.Contains(stderr, "No stash entries found") {
			return nil
		}
		return fmt.Errorf("stash drop: %s: %w", stderr, err)
	}
	return nil
}

func PopStash() error {
	if err := StashApply(); err != nil {
		return ErrStashConflict
	}
	return StashDrop()
}

func WorkingTreeClean() (bool, error) {
	status, stderr, err := RunGit("status", "--porcelain")
	if err != nil {
		return false, fmt.Errorf("check status: %s: %w", stderr, err)
	}
	return status == "", nil
}

func CommitAll(msg string) error {
	_, stderr, err := RunGit("add", "-A")
	if err != nil {
		return fmt.Errorf("git add: %s: %w", stderr, err)
	}
	stdout, stderr, err := RunGit("commit", "-m", msg)
	if err != nil {
		if strings.Contains(stderr, "nothing to commit") || strings.Contains(stdout, "nothing to commit") {
			return nil
		}
		return fmt.Errorf("git commit: %s: %w", stderr, err)
	}
	return nil
}

func CommitDetailed(subject, body string) error {
	_, stderr, err := RunGit("add", "-A")
	if err != nil {
		return fmt.Errorf("git add: %s: %w", stderr, err)
	}
	args := []string{"commit", "-m", subject}
	if body != "" {
		args = append(args, "-m", body)
	}
	stdout, stderr, err := RunGit(args...)
	if err != nil {
		if strings.Contains(stderr, "nothing to commit") || strings.Contains(stdout, "nothing to commit") {
			return nil
		}
		return fmt.Errorf("git commit: %s: %w", stderr, err)
	}
	return nil
}

func DiffStat() (string, error) {
	stdout, stderr, err := RunGit("diff", "--stat", "--cached")
	if err != nil {
		return "", fmt.Errorf("git diff --stat: %s: %w", stderr, err)
	}
	return stdout, nil
}

func TempBranchName(slug string) string {
	return "loop/" + slug
}

func CreateTempBranch(name, base string) error {
	_, _, err := RunGit("rev-parse", "--verify", "refs/heads/"+name)
	if err == nil {
		return SwitchBranch(name)
	}
	return CreateBranch(name, base)
}

func MergeFFOnly(src string) error {
	_, stderr, err := RunGit("merge", "--ff-only", src)
	if err != nil {
		return fmt.Errorf("merge --ff-only %s: %s: %w", src, stderr, err)
	}
	return nil
}

func DeleteBranch(name string) error {
	_, stderr, err := RunGit("branch", "-d", name)
	if err != nil {
		return fmt.Errorf("delete branch %s: %s: %w", name, stderr, err)
	}
	return nil
}

func CurrentBranch() (string, error) {
	stdout, stderr, err := RunGit("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", fmt.Errorf("get current branch: %s: %w", stderr, err)
	}
	return stdout, nil
}

func BranchExists(name string) bool {
	_, _, err := RunGit("rev-parse", "--verify", "refs/heads/"+name)
	return err == nil
}

func SwitchBranch(name string) error {
	_, stderr, err := RunGit("checkout", name)
	if err != nil {
		return fmt.Errorf("switch to branch %q: %s: %w", name, stderr, err)
	}
	return nil
}

func CreateBranch(name, from string) error {
	originBranch := "origin/" + from
	_, stderr, err := RunGit("checkout", "-b", name, originBranch)
	if err != nil {
		_, fbStderr, fbErr := RunGit("checkout", "-b", name, from)
		if fbErr != nil {
			return fmt.Errorf("create branch %q (tried %q then %q): %s / %s: %w",
				name, originBranch, from, stderr, fbStderr, fbErr)
		}
	}
	return nil
}

func SwitchOrCreateBranch(name, from string) error {
	if BranchExists(name) {
		return SwitchBranch(name)
	}
	return CreateBranch(name, from)
}

type Context struct {
	OriginalBranch string
	Stashed        bool
}

func contextPath() (string, error) {
	stdout, stderr, err := RunGit("rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("get repo root: %s: %w", stderr, err)
	}
	return filepath.Join(stdout, ".loop", GitContextFileName), nil
}

func persistContext(ctx *Context) error {
	path, err := contextPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create .loop dir: %w", err)
	}
	data, err := json.Marshal(ctx)
	if err != nil {
		return fmt.Errorf("marshal context: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write context: %w", err)
	}
	return nil
}

func clearContextFile() error {
	path, err := contextPath()
	if err != nil {
		return nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func HasSavedContext() bool {
	if !HasGit() || !IsRepo() {
		return false
	}
	path, err := contextPath()
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

func RestoreContextFromFile() error {
	if !HasGit() || !IsRepo() {
		return errors.New("no saved git context found")
	}
	path, err := contextPath()
	if err != nil {
		return errors.New("no saved git context found")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return errors.New("no saved git context found")
		}
		return fmt.Errorf("read git context: %w", err)
	}
	var ctx Context
	if err := json.Unmarshal(data, &ctx); err != nil {
		return fmt.Errorf("parse git context: %w", err)
	}
	if err := RestoreContext(&ctx); err != nil {
		return fmt.Errorf("restore: %w", err)
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("clear context file: %w", err)
	}
	return nil
}

func SaveContext() (func(), error) {
	if !HasGit() || !IsRepo() {
		warnNoRepo()
		return func() {}, nil
	}
	branch, err := CurrentBranch()
	if err != nil {
		return nil, fmt.Errorf("save context - current branch: %w", err)
	}
	stashed, err := StashChanges()
	if err != nil {
		return nil, fmt.Errorf("save context - stash: %w", err)
	}
	ctx := &Context{
		OriginalBranch: branch,
		Stashed:        stashed,
	}

	if err := persistContext(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not persist git context: %v\n", err)
	}

	return func() {
		if err := RestoreContext(ctx); err != nil {
			if errors.Is(err, ErrStashConflict) {
				fmt.Fprintf(os.Stderr, `Warning: git stash conflict detected while restoring your working tree.
The auto-saved changes have been dropped. Your working tree may contain
conflict markers that need manual resolution.

To resolve:
  1. Search for conflict markers (<<<<<<<, =======, >>>>>>>) in your files
  2. Edit each file to keep the desired changes
  3. Stage resolved files: git add <file>
  4. Complete the resolution: git commit -m "resolve stash conflict"

`)
			} else {
				fmt.Fprintf(os.Stderr, "restore context: %v\n", err)
			}
		}
		if err := clearContextFile(); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not clear persisted git context: %v\n", err)
		}
	}, nil
}

func ResolveBranch(branchField, defaultBranch string) string {
	switch {
	case branchField == "":
		return defaultBranch
	case branchField == "*":
		return ""
	default:
		return branchField
	}
}

func SwitchForIssue(branchField, defaultBranch string) (bool, error) {
	if !HasGit() || !IsRepo() {
		warnNoRepo()
		return false, nil
	}
	target := ResolveBranch(branchField, defaultBranch)
	if target == "" {
		return false, nil
	}
	current, err := CurrentBranch()
	if err != nil {
		return false, fmt.Errorf("switch for issue: %w", err)
	}
	if current == target {
		return false, nil
	}
	if err := SwitchOrCreateBranch(target, defaultBranch); err != nil {
		return false, err
	}
	return true, nil
}

func RestoreContext(ctx *Context) error {
	if ctx == nil {
		return nil
	}
	if !HasGit() || !IsRepo() {
		return nil
	}

	var errs []error

	if ctx.OriginalBranch != "" {
		current, err := CurrentBranch()
		if err != nil {
			errs = append(errs, fmt.Errorf("restore context - current branch: %w", err))
		} else if current != ctx.OriginalBranch {
			if err := SwitchBranch(ctx.OriginalBranch); err != nil {
				errs = append(errs, fmt.Errorf("restore context - switch back: %w", err))
			}
		}
	}

	if ctx.Stashed {
		if err := PopStash(); err != nil {
			errs = append(errs, fmt.Errorf("restore context - pop stash: %w", err))
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}
