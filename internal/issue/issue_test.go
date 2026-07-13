package issue

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/sambaths/loop/internal/agent"
)

func TestCreateAndRead(t *testing.T) {
	dir := t.TempDir()
	issue, err := Create(dir, StateTestReady, "Test Issue", "This is a test body")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if issue.Title != "Test Issue" {
		t.Errorf("expected title 'Test Issue', got %q", issue.Title)
	}
	if issue.State != StateTestReady {
		t.Errorf("expected state test-ready, got %q", issue.State)
	}
	if _, err := os.Stat(issue.FilePath); os.IsNotExist(err) {
		t.Fatal("expected file to exist")
	}
}

func TestRead(t *testing.T) {
	dir := t.TempDir()
	created, err := Create(dir, StateTestReady, "My Issue", "Body text")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	read, err := Read(created.FilePath)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if read.Title != "My Issue" {
		t.Errorf("expected title 'My Issue', got %q", read.Title)
	}
	if read.State != StateTestReady {
		t.Errorf("expected state test-ready, got %q", read.State)
	}
}

func TestList(t *testing.T) {
	dir := t.TempDir()

	_, err := Create(dir, StateTestReady, "Issue One", "Body one")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	_, err = Create(dir, StateTestReady, "Issue Two", "Body two")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	issues, err := List(dir, StateTestReady)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(issues) != 2 {
		t.Fatalf("expected 2 issues, got %d", len(issues))
	}
}

func TestListEmptyDirectory(t *testing.T) {
	dir := t.TempDir()

	issues, err := List(dir, StateTestReady)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(issues) != 0 {
		t.Errorf("expected 0 issues, got %d", len(issues))
	}
}

func TestListNonExistentDirectory(t *testing.T) {
	dir := t.TempDir()

	issues, err := List(dir, StateDone)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(issues) != 0 {
		t.Errorf("expected 0 issues, got %d", len(issues))
	}
}

func TestListNonExistentRoot(t *testing.T) {
	for _, state := range []State{StateTodo, StateTestReady, StateDone, StateQuarantine} {
		issues, err := List("/nonexistent/path", state)
		if err != nil {
			t.Fatalf("List(%q) failed: %v", state, err)
		}
		if len(issues) != 0 {
			t.Errorf("List(%q) expected 0 issues, got %d", state, len(issues))
		}
	}
}

func TestMove(t *testing.T) {
	dir := t.TempDir()

	issue, err := Create(dir, StateTestReady, "Moving Issue", "Body")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if err := Move(dir, *issue, StateDone); err != nil {
		t.Fatalf("Move failed: %v", err)
	}

	doneDir := filepath.Join(dir, "done")
	entries, err := os.ReadDir(doneDir)
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 file in done, got %d", len(entries))
	}
}

func TestMoveSameState(t *testing.T) {
	dir := t.TempDir()

	issue, err := Create(dir, StateTestReady, "Same State", "Body")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if err := Move(dir, *issue, StateTestReady); err != nil {
		t.Fatalf("Move to same state failed: %v", err)
	}
}

func TestStateFromPath(t *testing.T) {
	tests := []struct {
		path  string
		state State
	}{
		{"/issues/test-ready/foo.md", StateTestReady},
		{"/issues/done/bar.md", StateDone},
		{"/issues/.quarantine/baz.md", StateQuarantine},
		{"/issues/ready/foo.md", StateTodo},
		{"/issues/foo.md", StateTodo},
		{"/issues/sub/foo.md", StateTodo},
	}
	for _, tc := range tests {
		s := StateFromPath(tc.path)
		if s != tc.state {
			t.Errorf("StateFromPath(%q) = %q, want %q", tc.path, s, tc.state)
		}
	}
}

func TestIssueFileFields(t *testing.T) {
	f := IssueFile{
		Title:     "Implement login",
		GitHubNum: 42,
		ExecMode:  "implement",
		Branch:    "feat/login",
		FilePath:  "/tmp/issues/test-ready/implement-login.md",
	}
	if f.Title != "Implement login" {
		t.Errorf("Title = %q, want %q", f.Title, "Implement login")
	}
	if f.GitHubNum != 42 {
		t.Errorf("GitHubNum = %d, want %d", f.GitHubNum, 42)
	}
	if f.ExecMode != "implement" {
		t.Errorf("ExecMode = %q, want %q", f.ExecMode, "implement")
	}
	if f.Branch != "feat/login" {
		t.Errorf("Branch = %q, want %q", f.Branch, "feat/login")
	}
	if f.FilePath != "/tmp/issues/test-ready/implement-login.md" {
		t.Errorf("FilePath = %q, want %q", f.FilePath, "/tmp/issues/test-ready/implement-login.md")
	}
}

func TestIssueFileZeroValues(t *testing.T) {
	var f IssueFile
	if f.GitHubNum != 0 {
		t.Errorf("GitHubNum zero value = %d, want 0", f.GitHubNum)
	}
	if f.Title != "" {
		t.Errorf("Title zero value = %q, want empty", f.Title)
	}
}

func TestParseIssueFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test-ready", "test-issue.md")
	os.MkdirAll(filepath.Join(dir, "test-ready"), 0755)

	content := `# 02 - Equity curve drawdown calculation

GitHub: #14
Status: ready-for-agent
Execution mode: AFK-only
Branch: main

Some body content.
`
	os.WriteFile(path, []byte(content), 0644)

	f, err := ParseIssueFile(path)
	if err != nil {
		t.Fatalf("ParseIssueFile failed: %v", err)
	}

	if f.Title != "02 - Equity curve drawdown calculation" {
		t.Errorf("Title = %q, want %q", f.Title, "02 - Equity curve drawdown calculation")
	}
	if f.GitHubNum != 14 {
		t.Errorf("GitHubNum = %d, want %d", f.GitHubNum, 14)
	}
	if f.ExecMode != "AFK-only" {
		t.Errorf("ExecMode = %q, want %q", f.ExecMode, "AFK-only")
	}
	if f.Branch != "main" {
		t.Errorf("Branch = %q, want %q", f.Branch, "main")
	}
	if f.FilePath != path {
		t.Errorf("FilePath = %q, want %q", f.FilePath, path)
	}
	if f.State != StateTestReady {
		t.Errorf("State = %q, want %q", f.State, StateTestReady)
	}
}

func TestParseIssueFileMissingFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test-ready", "minimal.md")
	os.MkdirAll(filepath.Join(dir, "test-ready"), 0755)

	content := "# Just a title\n\nSome body.\n"
	os.WriteFile(path, []byte(content), 0644)

	f, err := ParseIssueFile(path)
	if err != nil {
		t.Fatalf("ParseIssueFile failed: %v", err)
	}

	if f.Title != "Just a title" {
		t.Errorf("Title = %q, want %q", f.Title, "Just a title")
	}
	if f.GitHubNum != 0 {
		t.Errorf("GitHubNum = %d, want 0", f.GitHubNum)
	}
	if f.ExecMode != "" {
		t.Errorf("ExecMode = %q, want empty", f.ExecMode)
	}
	if f.Branch != "" {
		t.Errorf("Branch = %q, want empty", f.Branch)
	}
	if f.FilePath != path {
		t.Errorf("FilePath = %q, want %q", f.FilePath, path)
	}
}

func TestParseIssueFileInvalidPath(t *testing.T) {
	_, err := ParseIssueFile("/nonexistent/path.md")
	if err == nil {
		t.Fatal("expected error for invalid path")
	}
}

func TestParseIssueFileInvalidGitHubNum(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test-issue.md")
	content := "# Test\n\nGitHub: #abc\n"
	os.WriteFile(path, []byte(content), 0644)

	f, err := ParseIssueFile(path)
	if err != nil {
		t.Fatalf("ParseIssueFile failed: %v", err)
	}
	if f.GitHubNum != 0 {
		t.Errorf("GitHubNum = %d, want 0", f.GitHubNum)
	}
}

func TestParseIssueFileFirstTitleOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")
	content := "# First title\n\nSome text\n\n# Second title\n"
	os.WriteFile(path, []byte(content), 0644)

	f, err := ParseIssueFile(path)
	if err != nil {
		t.Fatalf("ParseIssueFile failed: %v", err)
	}
	if f.Title != "First title" {
		t.Errorf("Title = %q, want %q", f.Title, "First title")
	}
}

func TestListOnlyMarkdownFiles(t *testing.T) {
	dir := t.TempDir()
	readyDir := filepath.Join(dir, "test-ready")
	os.MkdirAll(readyDir, 0755)

	os.WriteFile(filepath.Join(readyDir, "note.txt"), []byte("not an issue"), 0644)
	os.WriteFile(filepath.Join(readyDir, "issue.md"), []byte("# Real Issue\n\nBody"), 0644)

	issues, err := List(dir, StateTestReady)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(issues) != 1 {
		t.Errorf("expected 1 issue, got %d", len(issues))
	}
}

func TestReadGitHubNum(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test-ready", "gh-issue.md")
	os.MkdirAll(filepath.Join(dir, "test-ready"), 0755)
	content := "# GitHub Issue\n\nGitHub: #42\n\nSome body content.\n"
	os.WriteFile(path, []byte(content), 0644)

	issue, err := Read(path)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if issue.GitHubNum != 42 {
		t.Errorf("expected GitHubNum 42, got %d", issue.GitHubNum)
	}
}

func TestReadGitHubNumMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test-ready", "no-gh.md")
	os.MkdirAll(filepath.Join(dir, "test-ready"), 0755)
	content := "# No GitHub Number\n\nJust body.\n"
	os.WriteFile(path, []byte(content), 0644)

	issue, err := Read(path)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if issue.GitHubNum != 0 {
		t.Errorf("expected GitHubNum 0, got %d", issue.GitHubNum)
	}
}

func TestReadEmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.md")
	os.WriteFile(path, []byte{}, 0644)

	_, err := Read(path)
	if err == nil {
		t.Fatal("expected error for empty file")
	}
}

func TestReadMissingTitleHeading(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "no-heading.md")

	tests := []struct {
		name    string
		content string
	}{
		{"plain text", "This is a plain text file without a heading.\n"},
		{"level-2 heading only", "## Not a level-1 heading\n\nBody text.\n"},
		{"whitespace only", "   \n\n"},
		{"no heading prefix", "Not a heading\n\nBody.\n"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			os.WriteFile(path, []byte(tc.content), 0644)
			_, err := Read(path)
			if err == nil {
				t.Error("expected error for file without # heading")
			}
		})
	}
}

func TestReadEmptyHeading(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty-heading.md")
	os.WriteFile(path, []byte("# \n\nBody.\n"), 0644)

	_, err := Read(path)
	if err == nil {
		t.Fatal("expected error for empty heading")
	}
}

func TestParseIssueTitle(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{"number with hyphen", "# 02 - Equity curve drawdown calculation", "Equity curve drawdown calculation"},
		{"number with em dash", "# 14 — Implement login", "Implement login"},
		{"no number prefix", "# Just a title", "Just a title"},
		{"no heading prefix", "Plain text without heading", ""},
		{"empty content", "", ""},
		{"whitespace only", "   \n\n", ""},
		{"multi-digit number", "# 142 — Fix rare race condition", "Fix rare race condition"},
		{"body after heading", "# 07 - Cache invalidation\n\nBody content here.\n", "Cache invalidation"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseIssueTitle(tc.content)
			if got != tc.want {
				t.Errorf("ParseIssueTitle(%q) = %q, want %q", tc.content, got, tc.want)
			}
		})
	}
}

func TestParseIssueTitleHyphenOnlySeparatorInTitle(t *testing.T) {
	content := "# 05 - From here to there"
	got := ParseIssueTitle(content)
	if got != "From here to there" {
		t.Errorf("ParseIssueTitle(%q) = %q, want %q", content, got, "From here to there")
	}
}

func TestParseIssueTitleNumberWithoutSeparator(t *testing.T) {
	content := "# 42"
	got := ParseIssueTitle(content)
	if got != "42" {
		t.Errorf("ParseIssueTitle(%q) = %q, want %q", content, got, "42")
	}
}

func TestParseIssueTitleMultipleHyphens(t *testing.T) {
	content := "# 05 - Title - Subtitle"
	got := ParseIssueTitle(content)
	if got != "Title - Subtitle" {
		t.Errorf("ParseIssueTitle(%q) = %q, want %q", content, got, "Title - Subtitle")
	}
}

func TestParseIssueTitleNonNumericPrefixAfterSeparator(t *testing.T) {
	content := "# Title - something"
	got := ParseIssueTitle(content)
	if got != "Title - something" {
		t.Errorf("ParseIssueTitle(%q) = %q, want %q", content, got, "Title - something")
	}
}

func TestParseIssueTitleHeadingWithColon(t *testing.T) {
	content := "# 05 - Fix: login bug"
	got := ParseIssueTitle(content)
	if got != "Fix: login bug" {
		t.Errorf("ParseIssueTitle(%q) = %q, want %q", content, got, "Fix: login bug")
	}
}

func TestParseIssueTitleEmDashAfterNumber(t *testing.T) {
	content := "# 05 — Implement login"
	got := ParseIssueTitle(content)
	if got != "Implement login" {
		t.Errorf("ParseIssueTitle(%q) = %q, want %q", content, got, "Implement login")
	}
}

func TestParseIssueTitleOnlyWhitespaceHeading(t *testing.T) {
	content := "#   "
	got := ParseIssueTitle(content)
	if got != "" {
		t.Errorf("ParseIssueTitle(%q) = %q, want empty", content, got)
	}
}

func TestParseIssueTitleNumberTooLarge(t *testing.T) {
	content := "# 99999999999999999999999 - Overflow Test"
	got := ParseIssueTitle(content)
	if got != "99999999999999999999999 - Overflow Test" {
		t.Errorf("ParseIssueTitle(%q) = %q, want %q", content, got, "99999999999999999999999 - Overflow Test")
	}
}

func TestParseIssueFileEmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.md")
	os.WriteFile(path, []byte(""), 0644)

	f, err := ParseIssueFile(path)
	if err != nil {
		t.Fatalf("ParseIssueFile failed on empty file: %v", err)
	}
	if f.Title != "" {
		t.Errorf("expected empty title, got %q", f.Title)
	}
	if f.FilePath != path {
		t.Errorf("FilePath = %q, want %q", f.FilePath, path)
	}
}

func TestParseIssueFileOnlyWhitespaceLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "whitespace.md")
	os.WriteFile(path, []byte("  \n\n\t\n"), 0644)

	f, err := ParseIssueFile(path)
	if err != nil {
		t.Fatalf("ParseIssueFile failed: %v", err)
	}
	if f.Title != "" {
		t.Errorf("expected empty title for whitespace-only file, got %q", f.Title)
	}
}

func TestParseIssueFileExtraWhitespaceInFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "whitespace-fields.md")
	os.MkdirAll(filepath.Join(dir), 0755)
	content := "#  Title with space  \n\nGitHub: #  42  \nStatus:   ready  \n"
	os.WriteFile(path, []byte(content), 0644)

	f, err := ParseIssueFile(path)
	if err != nil {
		t.Fatalf("ParseIssueFile failed: %v", err)
	}
	if f.Title != "Title with space" {
		t.Errorf("Title = %q, want %q", f.Title, "Title with space")
	}
	if f.GitHubNum != 42 {
		t.Errorf("GitHubNum = %d, want 42", f.GitHubNum)
	}
}

func TestParseIssueFileLowcasePrefix(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lowcase.md")
	content := "# Test\n\ngithub: #14\nstatus: ready\n"
	os.WriteFile(path, []byte(content), 0644)

	f, err := ParseIssueFile(path)
	if err != nil {
		t.Fatalf("ParseIssueFile failed: %v", err)
	}
	if f.GitHubNum != 0 {
		t.Errorf("expected GitHubNum 0 (lowercase prefix), got %d", f.GitHubNum)
	}
}

func TestParseIssueFileGitHubHashWithoutNumber(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "no-number.md")
	content := "# Test\n\nGitHub: #\n"
	os.WriteFile(path, []byte(content), 0644)

	f, err := ParseIssueFile(path)
	if err != nil {
		t.Fatalf("ParseIssueFile failed: %v", err)
	}
	if f.GitHubNum != 0 {
		t.Errorf("expected GitHubNum 0 (empty number), got %d", f.GitHubNum)
	}
}

func TestParseIssueFileMultipleValues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "multiple.md")
	content := "# Test\n\nStatus: first\nStatus: second\nGitHub: #1\nGitHub: #99\n"
	os.WriteFile(path, []byte(content), 0644)

	f, err := ParseIssueFile(path)
	if err != nil {
		t.Fatalf("ParseIssueFile failed: %v", err)
	}
	if f.GitHubNum != 99 {
		t.Errorf("GitHubNum = %d, want 99 (last value wins)", f.GitHubNum)
	}
}

func TestListSkipsMalformedFiles(t *testing.T) {
	dir := t.TempDir()
	readyDir := filepath.Join(dir, "test-ready")
	os.MkdirAll(readyDir, 0755)

	os.WriteFile(filepath.Join(readyDir, "good.md"), []byte("# Good Issue\n\nBody"), 0644)
	os.WriteFile(filepath.Join(readyDir, "empty.md"), []byte{}, 0644)
	os.WriteFile(filepath.Join(readyDir, "no-heading.md"), []byte("No heading here\n"), 0644)
	os.WriteFile(filepath.Join(readyDir, "whitespace.md"), []byte("   \n\n"), 0644)

	issues, err := List(dir, StateTestReady)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(issues) != 1 {
		t.Errorf("expected 1 issue (only the valid one), got %d", len(issues))
	}
	if len(issues) > 0 && issues[0].Title != "Good Issue" {
		t.Errorf("expected title 'Good Issue', got %q", issues[0].Title)
	}
}

func TestCountsEmpty(t *testing.T) {
	ps := &PipelineState{}
	counts := ps.Counts()
	for _, s := range []State{StateTodo, StateTestReady, StateDone, StateQuarantine} {
		if counts[s] != 0 {
			t.Errorf("Counts()[%q] = %d, want 0", s, counts[s])
		}
	}
}

func TestCountsPopulated(t *testing.T) {
	dir := t.TempDir()

	_, err := Create(dir, StateTodo, "Todo A", "Body")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	_, err = Create(dir, StateTestReady, "Ready B", "Body")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	_, err = Create(dir, StateTestReady, "Ready C", "Body")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	_, err = Create(dir, StateDone, "Done D", "Body")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	_, err = Create(dir, StateDone, "Done E", "Body")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	_, err = Create(dir, StateDone, "Done F", "Body")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	_, err = Create(dir, StateQuarantine, "Quar G", "Body")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	ps, err := ScanIssueDir(dir)
	if err != nil {
		t.Fatalf("ScanIssueDir failed: %v", err)
	}

	counts := ps.Counts()
	if counts[StateTodo] != 1 {
		t.Errorf("Counts()[%q] = %d, want 1", StateTodo, counts[StateTodo])
	}
	if counts[StateTestReady] != 2 {
		t.Errorf("Counts()[%q] = %d, want 2", StateTestReady, counts[StateTestReady])
	}
	if counts[StateDone] != 3 {
		t.Errorf("Counts()[%q] = %d, want 3", StateDone, counts[StateDone])
	}
	if counts[StateQuarantine] != 1 {
		t.Errorf("Counts()[%q] = %d, want 1", StateQuarantine, counts[StateQuarantine])
	}
}

func TestCountsAfterMove(t *testing.T) {
	dir := t.TempDir()

	issue, err := Create(dir, StateTestReady, "Moving Item", "Body")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	ps, err := ScanIssueDir(dir)
	if err != nil {
		t.Fatalf("ScanIssueDir failed: %v", err)
	}
	counts := ps.Counts()
	if counts[StateTestReady] != 1 {
		t.Errorf("before move: Counts()[%q] = %d, want 1", StateTestReady, counts[StateTestReady])
	}
	if counts[StateDone] != 0 {
		t.Errorf("before move: Counts()[%q] = %d, want 0", StateDone, counts[StateDone])
	}

	if err := Move(dir, *issue, StateDone); err != nil {
		t.Fatalf("Move failed: %v", err)
	}

	ps, err = ScanIssueDir(dir)
	if err != nil {
		t.Fatalf("ScanIssueDir failed: %v", err)
	}
	counts = ps.Counts()
	if counts[StateTestReady] != 0 {
		t.Errorf("after move: Counts()[%q] = %d, want 0", StateTestReady, counts[StateTestReady])
	}
	if counts[StateDone] != 1 {
		t.Errorf("after move: Counts()[%q] = %d, want 1", StateDone, counts[StateDone])
	}
}

func TestListCountsAcrossAllStates(t *testing.T) {
	dir := t.TempDir()

	_, err := Create(dir, StateTodo, "Backlog A", "Body")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	_, err = Create(dir, StateTodo, "Backlog B", "Body")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	_, err = Create(dir, StateTestReady, "Ready C", "Body")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	_, err = Create(dir, StateTestReady, "Ready D", "Body")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	_, err = Create(dir, StateTestReady, "Ready E", "Body")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	_, err = Create(dir, StateDone, "Done F", "Body")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	_, err = Create(dir, StateQuarantine, "Quar G", "Body")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	_, err = Create(dir, StateQuarantine, "Quar H", "Body")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	todo, err := List(dir, StateTodo)
	if err != nil {
		t.Fatalf("List(todo) failed: %v", err)
	}
	testReady, err := List(dir, StateTestReady)
	if err != nil {
		t.Fatalf("List(test-ready) failed: %v", err)
	}
	done, err := List(dir, StateDone)
	if err != nil {
		t.Fatalf("List(done) failed: %v", err)
	}
	quar, err := List(dir, StateQuarantine)
	if err != nil {
		t.Fatalf("List(quarantine) failed: %v", err)
	}

	if len(todo) != 2 {
		t.Errorf("List(todo) = %d, want 2", len(todo))
	}
	if len(testReady) != 3 {
		t.Errorf("List(test-ready) = %d, want 3", len(testReady))
	}
	if len(done) != 1 {
		t.Errorf("List(done) = %d, want 1", len(done))
	}
	if len(quar) != 2 {
		t.Errorf("List(quarantine) = %d, want 2", len(quar))
	}
}

func TestScanIssueDirPopulated(t *testing.T) {
	dir := t.TempDir()

	_, err := Create(dir, StateTestReady, "Feature A", "Body")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	_, err = Create(dir, StateTestReady, "Feature B", "Body")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	_, err = Create(dir, StateDone, "Bugfix C", "Body")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	_, err = Create(dir, StateQuarantine, "Duplicate D", "Body")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	// Create a todo file directly in the issue root directory.
	todoPath := filepath.Join(dir, "todo-issue.md")
	os.WriteFile(todoPath, []byte("# Todo Issue\n\nBody"), 0644)

	ps, err := ScanIssueDir(dir)
	if err != nil {
		t.Fatalf("ScanIssueDir failed: %v", err)
	}
	if len(ps.TodoFiles) != 1 {
		t.Errorf("TodoFiles = %d, want 1", len(ps.TodoFiles))
	}
	if ps.TodoFiles[0] != todoPath {
		t.Errorf("TodoFiles[0] = %q, want %q", ps.TodoFiles[0], todoPath)
	}
	if len(ps.TestReadyFiles) != 2 {
		t.Errorf("TestReadyFiles = %d, want 2", len(ps.TestReadyFiles))
	}
	if len(ps.DoneFiles) != 1 {
		t.Errorf("DoneFiles = %d, want 1", len(ps.DoneFiles))
	}
	if len(ps.QuarantineFiles) != 1 {
		t.Errorf("QuarantineFiles = %d, want 1", len(ps.QuarantineFiles))
	}
}

func TestScanIssueDirEmpty(t *testing.T) {
	dir := t.TempDir()

	ps, err := ScanIssueDir(dir)
	if err != nil {
		t.Fatalf("ScanIssueDir failed: %v", err)
	}
	if len(ps.TodoFiles) != 0 {
		t.Errorf("TodoFiles = %d, want 0", len(ps.TodoFiles))
	}
	if len(ps.TestReadyFiles) != 0 {
		t.Errorf("TestReadyFiles = %d, want 0", len(ps.TestReadyFiles))
	}
	if len(ps.DoneFiles) != 0 {
		t.Errorf("DoneFiles = %d, want 0", len(ps.DoneFiles))
	}
	if len(ps.QuarantineFiles) != 0 {
		t.Errorf("QuarantineFiles = %d, want 0", len(ps.QuarantineFiles))
	}
}

func TestScanIssueDirEmptySubdirs(t *testing.T) {
	dir := t.TempDir()
	for _, sub := range []string{"test-ready", "done", ".quarantine"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0755); err != nil {
			t.Fatal(err)
		}
	}

	ps, err := ScanIssueDir(dir)
	if err != nil {
		t.Fatalf("ScanIssueDir failed: %v", err)
	}
	if len(ps.TodoFiles) != 0 {
		t.Errorf("TodoFiles = %d, want 0", len(ps.TodoFiles))
	}
	if len(ps.TestReadyFiles) != 0 {
		t.Errorf("TestReadyFiles = %d, want 0", len(ps.TestReadyFiles))
	}
	if len(ps.DoneFiles) != 0 {
		t.Errorf("DoneFiles = %d, want 0", len(ps.DoneFiles))
	}
	if len(ps.QuarantineFiles) != 0 {
		t.Errorf("QuarantineFiles = %d, want 0", len(ps.QuarantineFiles))
	}
}

func TestScanIssueDirNonExistent(t *testing.T) {
	ps, err := ScanIssueDir("/nonexistent/path")
	if err != nil {
		t.Fatalf("ScanIssueDir failed: %v", err)
	}
	if len(ps.TodoFiles) != 0 {
		t.Errorf("TodoFiles = %d, want 0", len(ps.TodoFiles))
	}
	if len(ps.TestReadyFiles) != 0 {
		t.Errorf("TestReadyFiles = %d, want 0", len(ps.TestReadyFiles))
	}
	if len(ps.DoneFiles) != 0 {
		t.Errorf("DoneFiles = %d, want 0", len(ps.DoneFiles))
	}
	if len(ps.QuarantineFiles) != 0 {
		t.Errorf("QuarantineFiles = %d, want 0", len(ps.QuarantineFiles))
	}
}

func TestListTodo(t *testing.T) {
	dir := t.TempDir()

	todoPath := filepath.Join(dir, "todo-issue.md")
	os.WriteFile(todoPath, []byte("# Todo Issue\n\nBody"), 0644)

	issues, err := List(dir, StateTodo)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
	if issues[0].Title != "Todo Issue" {
		t.Errorf("expected title 'Todo Issue', got %q", issues[0].Title)
	}
	if issues[0].State != StateTodo {
		t.Errorf("expected state 'todo', got %q", issues[0].State)
	}
}

func TestCreateTodo(t *testing.T) {
	dir := t.TempDir()
	issue, err := Create(dir, StateTodo, "Backlog Item", "Needs triage")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if issue.Title != "Backlog Item" {
		t.Errorf("expected title 'Backlog Item', got %q", issue.Title)
	}
	if issue.State != StateTodo {
		t.Errorf("expected state todo, got %q", issue.State)
	}
	if _, err := os.Stat(issue.FilePath); os.IsNotExist(err) {
		t.Fatal("expected file to exist")
	}
	if filepath.Dir(issue.FilePath) != dir {
		t.Errorf("expected file in root dir %q, got %q", dir, filepath.Dir(issue.FilePath))
	}
}

func TestMoveFromTodoToTestReady(t *testing.T) {
	dir := t.TempDir()
	issue, err := Create(dir, StateTodo, "Move Me", "Body")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if err := Move(dir, *issue, StateTestReady); err != nil {
		t.Fatalf("Move failed: %v", err)
	}
	readyDir := filepath.Join(dir, "test-ready")
	entries, err := os.ReadDir(readyDir)
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 file in test-ready, got %d", len(entries))
	}
}

func TestMoveToTodo(t *testing.T) {
	dir := t.TempDir()
	issue, err := Create(dir, StateTestReady, "Move Back", "Body")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if err := Move(dir, *issue, StateTodo); err != nil {
		t.Fatalf("Move to todo failed: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}
	var mdFiles []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".md" {
			mdFiles = append(mdFiles, e.Name())
		}
	}
	if len(mdFiles) != 1 {
		t.Errorf("expected 1 md file in root dir, got %d", len(mdFiles))
	}
}

func TestMoveTodoSameState(t *testing.T) {
	dir := t.TempDir()
	issue, err := Create(dir, StateTodo, "Same State", "Body")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if err := Move(dir, *issue, StateTodo); err != nil {
		t.Fatalf("Move to same state failed: %v", err)
	}
}

func TestReadTodoFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "root-issue.md")
	content := "# Root Issue\n\nBody content"
	os.WriteFile(path, []byte(content), 0644)

	issue, err := Read(path)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if issue.Title != "Root Issue" {
		t.Errorf("expected title 'Root Issue', got %q", issue.Title)
	}
	if issue.State != StateTodo {
		t.Errorf("expected state todo, got %q", issue.State)
	}
}

func TestScanIssueDirSkipsNonMdFiles(t *testing.T) {
	dir := t.TempDir()

	_, err := Create(dir, StateTestReady, "Real Issue", "Body")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	readyDir := filepath.Join(dir, "test-ready")
	os.WriteFile(filepath.Join(readyDir, "note.txt"), []byte("not an issue"), 0644)
	doneDir := filepath.Join(dir, "done")
	os.MkdirAll(doneDir, 0755)
	os.WriteFile(filepath.Join(doneDir, "readme.txt"), []byte("nope"), 0644)
	// Non-.md file in root should not appear in TodoFiles.
	os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("nope"), 0644)

	ps, err := ScanIssueDir(dir)
	if err != nil {
		t.Fatalf("ScanIssueDir failed: %v", err)
	}
	if len(ps.TodoFiles) != 0 {
		t.Errorf("TodoFiles = %d, want 0", len(ps.TodoFiles))
	}
	if len(ps.TestReadyFiles) != 1 {
		t.Errorf("TestReadyFiles = %d, want 1", len(ps.TestReadyFiles))
	}
	if len(ps.DoneFiles) != 0 {
		t.Errorf("DoneFiles = %d, want 0", len(ps.DoneFiles))
	}
}

func TestScanParsedEmpty(t *testing.T) {
	dir := t.TempDir()
	files, err := ScanParsed(dir)
	if err != nil {
		t.Fatalf("ScanParsed failed: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected 0 files, got %d", len(files))
	}
}

func TestScanParsedNonExistentRoot(t *testing.T) {
	files, err := ScanParsed("/nonexistent/path")
	if err != nil {
		t.Fatalf("ScanParsed failed: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected 0 files, got %d", len(files))
	}
}

func TestScanParsedAllStates(t *testing.T) {
	dir := t.TempDir()

	_, err := Create(dir, StateTodo, "Todo Item", "Body")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	_, err = Create(dir, StateTestReady, "Test Ready Item", "Body")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	_, err = Create(dir, StateDone, "Done Item", "Body")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	_, err = Create(dir, StateQuarantine, "Quarantined Item", "Body")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	files, err := ScanParsed(dir)
	if err != nil {
		t.Fatalf("ScanParsed failed: %v", err)
	}
	if len(files) != 4 {
		t.Fatalf("expected 4 files, got %d", len(files))
	}

	states := map[State]int{}
	for _, f := range files {
		states[f.State]++
	}
	if states[StateTodo] != 1 {
		t.Errorf("expected 1 todo, got %d", states[StateTodo])
	}
	if states[StateTestReady] != 1 {
		t.Errorf("expected 1 test-ready, got %d", states[StateTestReady])
	}
	if states[StateDone] != 1 {
		t.Errorf("expected 1 done, got %d", states[StateDone])
	}
	if states[StateQuarantine] != 1 {
		t.Errorf("expected 1 quarantined, got %d", states[StateQuarantine])
	}
}

func TestScanParsedEmptyFile(t *testing.T) {
	dir := t.TempDir()
	readyDir := filepath.Join(dir, "test-ready")
	os.MkdirAll(readyDir, 0755)

	os.WriteFile(filepath.Join(readyDir, "empty.md"), []byte{}, 0644)

	files, err := ScanParsed(dir)
	if err != nil {
		t.Fatalf("ScanParsed failed: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if files[0].Title != "" {
		t.Errorf("expected empty title for empty file, got %q", files[0].Title)
	}
}

func TestSelectIssuePrefersTestReady(t *testing.T) {
	dir := t.TempDir()

	todoPath := filepath.Join(dir, "todo-issue.md")
	os.WriteFile(todoPath, []byte("# Todo Issue\n\nBody"), 0644)

	readyDir := filepath.Join(dir, "test-ready")
	os.MkdirAll(readyDir, 0755)
	readyPath := filepath.Join(readyDir, "ready-issue.md")
	os.WriteFile(readyPath, []byte("# Ready Issue\n\nBody"), 0644)

	state := &PipelineState{
		TodoFiles:      []string{todoPath},
		TestReadyFiles: []string{readyPath},
	}

	selected, role, err := SelectIssue(state)
	if err != nil {
		t.Fatalf("SelectIssue failed: %v", err)
	}
	if selected.Title != "Ready Issue" {
		t.Errorf("expected title 'Ready Issue', got %q", selected.Title)
	}
	if role != RoleTest {
		t.Errorf("expected role test, got %q", role)
	}
}

func TestSelectIssueFallsBackToTodo(t *testing.T) {
	dir := t.TempDir()

	todoPath := filepath.Join(dir, "todo-issue.md")
	os.WriteFile(todoPath, []byte("# Todo Issue\n\nExecution mode: AFK-only\n\nBody"), 0644)

	state := &PipelineState{
		TodoFiles: []string{todoPath},
	}

	selected, role, err := SelectIssue(state)
	if err != nil {
		t.Fatalf("SelectIssue failed: %v", err)
	}
	if selected.Title != "Todo Issue" {
		t.Errorf("expected title 'Todo Issue', got %q", selected.Title)
	}
	if role != RoleImplement {
		t.Errorf("expected role implement, got %q", role)
	}
}

func TestSelectIssueNoIssues(t *testing.T) {
	state := &PipelineState{}
	_, _, err := SelectIssue(state)
	if err == nil {
		t.Fatal("expected error for empty pipeline state")
	}
}

func TestSelectIssuePrefersTestReadyOverTodo(t *testing.T) {
	dir := t.TempDir()

	todoPath := filepath.Join(dir, "todo-issue.md")
	os.WriteFile(todoPath, []byte("# Todo Issue\n\nBody"), 0644)

	readyDir := filepath.Join(dir, "test-ready")
	os.MkdirAll(readyDir, 0755)
	readyPath := filepath.Join(readyDir, "ready-issue.md")
	os.WriteFile(readyPath, []byte("# Ready Issue\n\nBody"), 0644)

	state := &PipelineState{
		TodoFiles:      []string{todoPath},
		TestReadyFiles: []string{readyPath},
	}

	selected, role, err := SelectIssue(state)
	if err != nil {
		t.Fatalf("SelectIssue failed: %v", err)
	}
	if selected.Title != "Ready Issue" {
		t.Errorf("expected title 'Ready Issue', got %q", selected.Title)
	}
	if role != RoleTest {
		t.Errorf("expected role test, got %q", role)
	}
}

func TestSelectIssueNonExistentTestReady(t *testing.T) {
	state := &PipelineState{
		TestReadyFiles: []string{"/nonexistent/test-ready/issue.md"},
	}

	_, _, err := SelectIssue(state)
	if err == nil {
		t.Fatal("expected error for non-existent test-ready file")
	}
}

func TestSelectIssueNonExistentTodo(t *testing.T) {
	state := &PipelineState{
		TodoFiles: []string{"/nonexistent/todo.md"},
	}

	_, _, err := SelectIssue(state)
	if err == nil {
		t.Fatal("expected error for non-existent todo file")
	}
}

func TestSelectIssueMultipleTestReady(t *testing.T) {
	dir := t.TempDir()

	readyDir := filepath.Join(dir, "test-ready")
	os.MkdirAll(readyDir, 0755)
	alphaPath := filepath.Join(readyDir, "alpha.md")
	os.WriteFile(alphaPath, []byte("# Alpha\n\nBody"), 0644)
	betaPath := filepath.Join(readyDir, "beta.md")
	os.WriteFile(betaPath, []byte("# Beta\n\nBody"), 0644)

	state := &PipelineState{
		TestReadyFiles: []string{alphaPath, betaPath},
	}

	selected, role, err := SelectIssue(state)
	if err != nil {
		t.Fatalf("SelectIssue failed: %v", err)
	}
	if selected.Title != "Alpha" {
		t.Errorf("expected first issue 'Alpha', got %q", selected.Title)
	}
	if role != RoleTest {
		t.Errorf("expected role test, got %q", role)
	}
}

func TestSelectIssueSkipsStuckTestReady(t *testing.T) {
	dir := t.TempDir()

	readyDir := filepath.Join(dir, "test-ready")
	os.MkdirAll(readyDir, 0755)

	// Stuck file — has populated UAT Results (should be skipped)
	stuckPath := filepath.Join(readyDir, "stuck.md")
	os.WriteFile(stuckPath, []byte("# Stuck\n\n## UAT Results\n\n| Step | Result |\n| --- | --- |\n| Check | Pass |\n"), 0644)

	// Fresh test-ready file — no UAT Results (should be selected)
	freshPath := filepath.Join(readyDir, "fresh.md")
	os.WriteFile(freshPath, []byte("# Fresh\n\nBody"), 0644)

	todoPath := filepath.Join(dir, "todo-issue.md")
	os.WriteFile(todoPath, []byte("# Todo Issue\n\nExecution mode: AFK-only\n\nBody"), 0644)

	state := &PipelineState{
		TodoFiles:      []string{todoPath},
		TestReadyFiles: []string{stuckPath, freshPath},
	}

	selected, role, err := SelectIssue(state)
	if err != nil {
		t.Fatalf("SelectIssue failed: %v", err)
	}
	if selected.Title != "Fresh" {
		t.Errorf("expected 'Fresh' (non-stuck test-ready file), got %q", selected.Title)
	}
	if role != RoleTest {
		t.Errorf("expected role test, got %q", role)
	}
}

func TestSelectIssueAllTestReadyStuckFallsBack(t *testing.T) {
	dir := t.TempDir()

	readyDir := filepath.Join(dir, "test-ready")
	os.MkdirAll(readyDir, 0755)

	// Both files stuck — should fall back to todo
	stuckPath := filepath.Join(readyDir, "stuck1.md")
	os.WriteFile(stuckPath, []byte("# Stuck One\n\n## UAT Results\n\nContent\n"), 0644)
	stuck2Path := filepath.Join(readyDir, "stuck2.md")
	os.WriteFile(stuck2Path, []byte("# Stuck Two\n\n## UAT Results\n\nContent\n"), 0644)

	todoPath := filepath.Join(dir, "todo-issue.md")
	os.WriteFile(todoPath, []byte("# Todo Issue\n\nExecution mode: AFK-only\n\nBody"), 0644)

	state := &PipelineState{
		TodoFiles:      []string{todoPath},
		TestReadyFiles: []string{stuckPath, stuck2Path},
	}

	selected, role, err := SelectIssue(state)
	if err != nil {
		t.Fatalf("SelectIssue failed: %v", err)
	}
	if selected.Title != "Todo Issue" {
		t.Errorf("expected 'Todo Issue' (all test-ready stuck, fallback), got %q", selected.Title)
	}
	if role != RoleImplement {
		t.Errorf("expected role implement, got %q", role)
	}
}

func TestParseBlockedByNone(t *testing.T) {
	refs := ParseBlockedBy("## Blocked by\n\nNone\n")
	if refs != nil {
		t.Errorf("expected nil for 'None', got %v", refs)
	}
}

func TestParseBlockedByNoneWithDash(t *testing.T) {
	refs := ParseBlockedBy("## Blocked by\n\n- None\n")
	if refs != nil {
		t.Errorf("expected nil for '- None', got %v", refs)
	}
}

func TestParseBlockedByReferences(t *testing.T) {
	content := "## Blocked by\n\n- 02 — Status pipeline\n- 03 — Core iteration\n"
	refs := ParseBlockedBy(content)
	if len(refs) != 2 {
		t.Fatalf("expected 2 refs, got %d: %v", len(refs), refs)
	}
	if refs[0] != "02 — Status pipeline" {
		t.Errorf("refs[0] = %q, want %q", refs[0], "02 — Status pipeline")
	}
	if refs[1] != "03 — Core iteration" {
		t.Errorf("refs[1] = %q, want %q", refs[1], "03 — Core iteration")
	}
}

func TestParseBlockedByStopsAtNextHeading(t *testing.T) {
	content := "## Blocked by\n\n- 02 — Status\n\n## Comments\n\nNothing.\n"
	refs := ParseBlockedBy(content)
	if len(refs) != 1 {
		t.Fatalf("expected 1 ref, got %d: %v", len(refs), refs)
	}
}

func TestParseBlockedByCanStartImmediately(t *testing.T) {
	refs := ParseBlockedBy("## Blocked by\n\nNone — can start immediately\n")
	if refs != nil {
		t.Errorf("expected nil for 'None — can start immediately', got %v", refs)
	}
}

func TestParseBlockedByNoSection(t *testing.T) {
	refs := ParseBlockedBy("# Title\n\nSome body text\n")
	if len(refs) != 0 {
		t.Errorf("expected 0 refs for no blocked-by section, got %v", refs)
	}
}

func TestParseIssueSections(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    map[string]string
	}{
		{
			name:    "empty content",
			content: "",
			want:    map[string]string{},
		},
		{
			name:    "single section",
			content: "## What to build\nBuild a thing\n",
			want: map[string]string{
				"what to build": "Build a thing",
			},
		},
		{
			name:    "multiple sections",
			content: "## Section One\nContent one\n## Section Two\nContent two\n",
			want: map[string]string{
				"section one": "Content one",
				"section two": "Content two",
			},
		},
		{
			name:    "content before first section is ignored",
			content: "Preamble\n## Section One\nContent one\n",
			want: map[string]string{
				"section one": "Content one",
			},
		},
		{
			name:    "multiline section content",
			content: "## Section\nLine 1\nLine 2\n",
			want: map[string]string{
				"section": "Line 1\nLine 2",
			},
		},
		{
			name:    "no sections",
			content: "# Title\n\nSome body text\n",
			want: map[string]string{
				"title": "Some body text",
			},
		},
		{
			name:    "section with leading whitespace in heading",
			content: "##   Spaced Name   \nContent\n",
			want: map[string]string{
				"spaced name": "Content",
			},
		},
		{
			name:    "only headings no content",
			content: "## One\n## Two\n",
			want: map[string]string{
				"one": "",
				"two": "",
			},
		},
		{
			name:    "content after last section is included",
			content: "## Section\nContent\nTrailing\n",
			want: map[string]string{
				"section": "Content\nTrailing",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseIssueSections(tt.content)
			if err != nil {
				t.Fatalf("ParseIssueSections() unexpected error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("ParseIssueSections() got %d sections, want %d\n  got:  %v\n  want: %v", len(got), len(tt.want), got, tt.want)
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("ParseIssueSections()[%q] = %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

func TestSelectIssueSkipsBlockedTodo(t *testing.T) {
	dir := t.TempDir()

	doneDir := filepath.Join(dir, "done")
	os.MkdirAll(doneDir, 0755)
	donePath := filepath.Join(doneDir, "01-done-issue.md")
	os.WriteFile(donePath, []byte("# 01 — Done Issue\n\nBody"), 0644)

	blockedPath := filepath.Join(dir, "blocked-issue.md")
	content := "# 02 — Blocked\n\nExecution mode: AFK-only\n\n## Blocked by\n\n- 99 — Missing issue\n"
	os.WriteFile(blockedPath, []byte(content), 0644)

	unblockedPath := filepath.Join(dir, "unblocked-issue.md")
	content2 := "# 03 — Unblocked\n\nExecution mode: AFK-only\n\n## Blocked by\n\nNone\n"
	os.WriteFile(unblockedPath, []byte(content2), 0644)

	state := &PipelineState{
		TodoFiles: []string{blockedPath, unblockedPath},
		DoneFiles: []string{donePath},
	}

	selected, role, err := SelectIssue(state)
	if err != nil {
		t.Fatalf("SelectIssue failed: %v", err)
	}
	if selected.Title != "03 — Unblocked" {
		t.Errorf("expected '03 — Unblocked', got %q", selected.Title)
	}
	if role != RoleImplement {
		t.Errorf("expected role implement, got %q", role)
	}
}

func TestSelectIssueSkipsHITLOnly(t *testing.T) {
	dir := t.TempDir()

	hitlPath := filepath.Join(dir, "hitl-issue.md")
	content := "# 01 — HITL\n\nExecution mode: HITL-only\n"
	os.WriteFile(hitlPath, []byte(content), 0644)

	afkPath := filepath.Join(dir, "afk-issue.md")
	content2 := "# 02 — AFK\n\nExecution mode: AFK-only\n"
	os.WriteFile(afkPath, []byte(content2), 0644)

	state := &PipelineState{
		TodoFiles: []string{hitlPath, afkPath},
	}

	selected, role, err := SelectIssue(state)
	if err != nil {
		t.Fatalf("SelectIssue failed: %v", err)
	}
	if selected.Title != "02 — AFK" {
		t.Errorf("expected '02 — AFK', got %q", selected.Title)
	}
	if role != RoleImplement {
		t.Errorf("expected role implement, got %q", role)
	}
}

func TestSelectIssueSkipsCombo(t *testing.T) {
	dir := t.TempDir()

	comboPath := filepath.Join(dir, "combo-issue.md")
	content := "# 01 — Combo\n\nExecution mode: Combo\n"
	os.WriteFile(comboPath, []byte(content), 0644)

	state := &PipelineState{
		TodoFiles: []string{comboPath},
	}

	_, _, err := SelectIssue(state)
	if err != ErrNoIssues {
		t.Errorf("expected ErrNoIssues for only Combo issues, got %v", err)
	}
}

func TestSelectIssueFallsBackToHITLOnlyInTodo(t *testing.T) {
	dir := t.TempDir()

	hitlPath := filepath.Join(dir, "hitl-issue.md")
	content := "# 01 — HITL\n\nExecution mode: HITL-only\n"
	os.WriteFile(hitlPath, []byte(content), 0644)

	state := &PipelineState{
		TodoFiles: []string{hitlPath},
	}

	selected, role, err := SelectIssue(state)
	if err != nil {
		t.Fatalf("SelectIssue failed: %v", err)
	}
	if selected.Title != "01 — HITL" {
		t.Errorf("expected '01 — HITL', got %q", selected.Title)
	}
	if role != RoleImplement {
		t.Errorf("expected role implement, got %q", role)
	}
}

func TestSelectIssueFallsBackToHITLOnlyInReadyForAgent(t *testing.T) {
	dir := t.TempDir()

	readyDir := filepath.Join(dir, "ready-for-agent")
	os.MkdirAll(readyDir, 0755)
	hitlPath := filepath.Join(readyDir, "hitl-issue.md")
	content := "# 01 — HITL\n\nExecution mode: HITL-only\n"
	os.WriteFile(hitlPath, []byte(content), 0644)

	state := &PipelineState{
		ReadyForAgentFiles: []string{hitlPath},
	}

	selected, role, err := SelectIssue(state)
	if err != nil {
		t.Fatalf("SelectIssue failed: %v", err)
	}
	if selected.Title != "01 — HITL" {
		t.Errorf("expected '01 — HITL', got %q", selected.Title)
	}
	if role != RoleImplement {
		t.Errorf("expected role implement, got %q", role)
	}
}

func TestSelectIssuePrefersAFKOverHITLOnly(t *testing.T) {
	dir := t.TempDir()

	hitlPath := filepath.Join(dir, "hitl-issue.md")
	content := "# 01 — HITL\n\nExecution mode: HITL-only\n"
	os.WriteFile(hitlPath, []byte(content), 0644)

	afkPath := filepath.Join(dir, "afk-issue.md")
	content2 := "# 02 — AFK\n\nExecution mode: AFK-only\n"
	os.WriteFile(afkPath, []byte(content2), 0644)

	state := &PipelineState{
		TodoFiles: []string{hitlPath, afkPath},
	}

	selected, role, err := SelectIssue(state)
	if err != nil {
		t.Fatalf("SelectIssue failed: %v", err)
	}
	if selected.Title != "02 — AFK" {
		t.Errorf("expected '02 — AFK', got %q", selected.Title)
	}
	if role != RoleImplement {
		t.Errorf("expected role implement, got %q", role)
	}
}

func TestSelectIssueComboOnlyStillReturnsErrNoIssues(t *testing.T) {
	dir := t.TempDir()

	comboPath := filepath.Join(dir, "combo-issue.md")
	content := "# 01 — Combo\n\nExecution mode: Combo\n"
	os.WriteFile(comboPath, []byte(content), 0644)

	state := &PipelineState{
		TodoFiles: []string{comboPath},
	}

	_, _, err := SelectIssue(state)
	if err != ErrNoIssues {
		t.Errorf("expected ErrNoIssues for only Combo issues, got %v", err)
	}
}

func TestSelectIssueAllTodoBlocked(t *testing.T) {
	dir := t.TempDir()

	blockedPath := filepath.Join(dir, "blocked.md")
	content := "# 01 — Blocked\n\n## Blocked by\n\n- 99 — Missing\n"
	os.WriteFile(blockedPath, []byte(content), 0644)

	state := &PipelineState{
		TodoFiles: []string{blockedPath},
	}

	_, _, err := SelectIssue(state)
	if err != ErrNoIssues {
		t.Errorf("expected ErrNoIssues, got %v", err)
	}
}

func TestSelectIssueBlockerResolved(t *testing.T) {
	dir := t.TempDir()

	doneDir := filepath.Join(dir, "done")
	os.MkdirAll(doneDir, 0755)
	donePath := filepath.Join(doneDir, "02-status-pipeline.md")
	os.WriteFile(donePath, []byte("# 02 — Status pipeline\n\nBody"), 0644)

	todoPath := filepath.Join(dir, "todo.md")
	content := "# 03 — Core iteration\n\nExecution mode: AFK-only\n\n## Blocked by\n\n- 02 — Status pipeline\n"
	os.WriteFile(todoPath, []byte(content), 0644)

	state := &PipelineState{
		TodoFiles: []string{todoPath},
		DoneFiles: []string{donePath},
	}

	selected, role, err := SelectIssue(state)
	if err != nil {
		t.Fatalf("SelectIssue failed: %v", err)
	}
	if selected.Title != "03 — Core iteration" {
		t.Errorf("expected '03 — Core iteration', got %q", selected.Title)
	}
	if role != RoleImplement {
		t.Errorf("expected role implement, got %q", role)
	}
}

func TestSelectIssueTestReadyBlocked(t *testing.T) {
	dir := t.TempDir()

	readyDir := filepath.Join(dir, "test-ready")
	os.MkdirAll(readyDir, 0755)
	readyPath := filepath.Join(readyDir, "ready.md")
	content := "# 01 — Ready\n\n## Blocked by\n\n- 99 — Missing\n"
	os.WriteFile(readyPath, []byte(content), 0644)

	state := &PipelineState{
		TestReadyFiles: []string{readyPath},
	}

	_, _, err := SelectIssue(state)
	if err != ErrNoIssues {
		t.Errorf("expected ErrNoIssues (all blocked, nothing else), got %v", err)
	}
}

func TestSelectIssueEmptyExecModeAndEmptyTodo(t *testing.T) {
	dir := t.TempDir()

	emptyModePath := filepath.Join(dir, "no-mode.md")
	content := "# 01 — No mode\n"
	os.WriteFile(emptyModePath, []byte(content), 0644)

	state := &PipelineState{
		TodoFiles: []string{emptyModePath},
	}

	_, _, err := SelectIssue(state)
	if err != ErrNoIssues {
		t.Errorf("expected ErrNoIssues for empty exec mode (no AFK-only), got %v", err)
	}
}

func TestSelectIssueHandlesDoneAndQuarantine(t *testing.T) {
	dir := t.TempDir()

	todoPath := filepath.Join(dir, "todo-issue.md")
	os.WriteFile(todoPath, []byte("# Todo Issue\n\nExecution mode: AFK-only\n\nBody"), 0644)

	state := &PipelineState{
		DoneFiles:       []string{filepath.Join(dir, "done", "done.md")},
		QuarantineFiles: []string{filepath.Join(dir, ".quarantine", "bad.md")},
		TestReadyFiles:  []string{},
		TodoFiles:       []string{todoPath},
	}

	selected, role, err := SelectIssue(state)
	if err != nil {
		t.Fatalf("SelectIssue failed: %v", err)
	}
	if selected.Title != "Todo Issue" {
		t.Errorf("expected title 'Todo Issue', got %q", selected.Title)
	}
	if role != RoleImplement {
		t.Errorf("expected role implement, got %q", role)
	}
}

func TestSelectIssueParsesIssueFileFields(t *testing.T) {
	dir := t.TempDir()

	readyDir := filepath.Join(dir, "test-ready")
	os.MkdirAll(readyDir, 0755)
	path := filepath.Join(readyDir, "login.md")
	content := "# 05 - Implement login\n\nGitHub: #42\nStatus: ready-for-agent\nExecution mode: AFK-only\nBranch: feat/login\n"
	os.WriteFile(path, []byte(content), 0644)

	state := &PipelineState{
		TestReadyFiles: []string{path},
	}
	selected, role, err := SelectIssue(state)
	if err != nil {
		t.Fatalf("SelectIssue failed: %v", err)
	}
	if selected.GitHubNum != 42 {
		t.Errorf("GitHubNum = %d, want 42", selected.GitHubNum)
	}
	if selected.ExecMode != "AFK-only" {
		t.Errorf("ExecMode = %q, want %q", selected.ExecMode, "AFK-only")
	}
	if selected.Branch != "feat/login" {
		t.Errorf("Branch = %q, want %q", selected.Branch, "feat/login")
	}
	if selected.State != StateTestReady {
		t.Errorf("State = %q, want %q", selected.State, StateTestReady)
	}
	if role != RoleTest {
		t.Errorf("role = %q, want %q", role, RoleTest)
	}
}

// Mixed states

func TestSelectIssueMixedSomeTestReadyBlocked(t *testing.T) {
	dir := t.TempDir()

	readyDir := filepath.Join(dir, "test-ready")
	os.MkdirAll(readyDir, 0755)

	blockedPath := filepath.Join(readyDir, "blocked.md")
	os.WriteFile(blockedPath, []byte("# 01 — Blocked\n\n## Blocked by\n\n- 99 — Missing\n"), 0644)

	unblockedPath := filepath.Join(readyDir, "unblocked.md")
	os.WriteFile(unblockedPath, []byte("# 02 — Unblocked\n\nBody"), 0644)

	state := &PipelineState{
		TestReadyFiles: []string{blockedPath, unblockedPath},
	}

	selected, role, err := SelectIssue(state)
	if err != nil {
		t.Fatalf("SelectIssue failed: %v", err)
	}
	if selected.Title != "02 — Unblocked" {
		t.Errorf("expected '02 — Unblocked', got %q", selected.Title)
	}
	if role != RoleTest {
		t.Errorf("expected role test, got %q", role)
	}
}

func TestSelectIssueMixedAllBlocked(t *testing.T) {
	dir := t.TempDir()

	readyDir := filepath.Join(dir, "test-ready")
	os.MkdirAll(readyDir, 0755)
	readyPath := filepath.Join(readyDir, "ready.md")
	os.WriteFile(readyPath, []byte("# 01 — Ready\n\n## Blocked by\n\n- 99 — Missing\n"), 0644)

	todoPath := filepath.Join(dir, "todo.md")
	os.WriteFile(todoPath, []byte("# 02 — Todo\n\nExecution mode: AFK-only\n\n## Blocked by\n\n- 98 — Missing\n"), 0644)

	state := &PipelineState{
		TestReadyFiles: []string{readyPath},
		TodoFiles:      []string{todoPath},
	}

	_, _, err := SelectIssue(state)
	if err != ErrNoIssues {
		t.Errorf("expected ErrNoIssues (all items blocked), got %v", err)
	}
}

// All blocked

func TestSelectIssueAllTestReadyBlockedWithTodo(t *testing.T) {
	dir := t.TempDir()

	readyDir := filepath.Join(dir, "test-ready")
	os.MkdirAll(readyDir, 0755)
	readyPath := filepath.Join(readyDir, "ready.md")
	os.WriteFile(readyPath, []byte("# 01 — Ready\n\n## Blocked by\n\n- 99 — Missing\n"), 0644)

	todoPath := filepath.Join(dir, "todo.md")
	os.WriteFile(todoPath, []byte("# 02 — Todo\n\nExecution mode: AFK-only\n\nBody"), 0644)

	state := &PipelineState{
		TestReadyFiles: []string{readyPath},
		TodoFiles:      []string{todoPath},
	}

	selected, role, err := SelectIssue(state)
	if err != nil {
		t.Fatalf("SelectIssue failed: %v", err)
	}
	if selected.Title != "02 — Todo" {
		t.Errorf("expected todo issue to be selected (test-ready all blocked), got %q", selected.Title)
	}
	if role != RoleImplement {
		t.Errorf("expected role implement, got %q", role)
	}
}

// Empty queues

func TestSelectIssueEmptyTestReadyQueue(t *testing.T) {
	dir := t.TempDir()

	todoPath := filepath.Join(dir, "todo.md")
	os.WriteFile(todoPath, []byte("# 01 — Todo\n\nExecution mode: AFK-only\n\nBody"), 0644)

	state := &PipelineState{
		TestReadyFiles: []string{},
		TodoFiles:      []string{todoPath},
	}

	selected, role, err := SelectIssue(state)
	if err != nil {
		t.Fatalf("SelectIssue failed: %v", err)
	}
	if selected.Title != "01 — Todo" {
		t.Errorf("expected '01 — Todo', got %q", selected.Title)
	}
	if role != RoleImplement {
		t.Errorf("expected role implement, got %q", role)
	}
}

func TestSelectIssueEmptyTodoQueue(t *testing.T) {
	dir := t.TempDir()

	readyDir := filepath.Join(dir, "test-ready")
	os.MkdirAll(readyDir, 0755)
	readyPath := filepath.Join(readyDir, "ready.md")
	os.WriteFile(readyPath, []byte("# 01 — Ready\n\nBody"), 0644)

	state := &PipelineState{
		TestReadyFiles: []string{readyPath},
		TodoFiles:      []string{},
	}

	selected, role, err := SelectIssue(state)
	if err != nil {
		t.Fatalf("SelectIssue failed: %v", err)
	}
	if selected.Title != "01 — Ready" {
		t.Errorf("expected '01 — Ready', got %q", selected.Title)
	}
	if role != RoleTest {
		t.Errorf("expected role test, got %q", role)
	}
}

func TestStateDir(t *testing.T) {
	dir := t.TempDir()
	tests := []struct {
		state State
		want  string
	}{
		{StateTodo, dir},
		{StateTestReady, dir + "/test-ready"},
		{StateDone, dir + "/done"},
		{StateQuarantine, dir + "/.quarantine"},
	}
	for _, tc := range tests {
		got := stateDir(dir, tc.state)
		if got != tc.want {
			t.Errorf("stateDir(%q, %q) = %q, want %q", dir, tc.state, got, tc.want)
		}
	}
}

func TestScanDirFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.md"), []byte("# A\n"), 0644)
	os.WriteFile(filepath.Join(dir, "b.md"), []byte("# B\n"), 0644)
	os.WriteFile(filepath.Join(dir, "note.txt"), []byte("not md"), 0644)
	os.MkdirAll(filepath.Join(dir, "sub"), 0755)
	os.WriteFile(filepath.Join(dir, "sub", "c.md"), []byte("# C\n"), 0644)

	files, err := scanDirFiles(dir)
	if err != nil {
		t.Fatalf("scanDirFiles failed: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 .md files, got %d: %v", len(files), files)
	}
}

func TestCreateWithHeadingBody(t *testing.T) {
	dir := t.TempDir()

	body := "# My Heading\n\nThis body already has a heading."
	issue, err := Create(dir, StateTestReady, "Ignored Title", body)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if issue.Title != "Ignored Title" {
		t.Errorf("expected title 'Ignored Title', got %q", issue.Title)
	}
	if issue.Body != body {
		t.Errorf("expected body unchanged, got %q", issue.Body)
	}
}

func TestCreateAndReadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	created, err := Create(dir, StateTodo, "Round Trip", "Some body")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	read, err := Read(created.FilePath)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if read.Title != "Round Trip" {
		t.Errorf("expected title 'Round Trip', got %q", read.Title)
	}
	if read.State != StateTodo {
		t.Errorf("expected state todo, got %q", read.State)
	}
}

func TestMoveSourceNotExist(t *testing.T) {
	dir := t.TempDir()

	issue := Issue{
		FilePath: filepath.Join(dir, "nonexistent.md"),
		Title:    "Missing",
		State:    StateTodo,
	}

	err := Move(dir, issue, StateTestReady)
	if err == nil {
		t.Fatal("expected error when moving non-existent file")
	}
}

func TestScanIssueDirSkipsSubdirectories(t *testing.T) {
	dir := t.TempDir()

	os.MkdirAll(filepath.Join(dir, "subdir"), 0755)
	subFile := filepath.Join(dir, "subdir", "issue.md")
	os.WriteFile(subFile, []byte("# In Subdir\n"), 0644)

	ps, err := ScanIssueDir(dir)
	if err != nil {
		t.Fatalf("ScanIssueDir failed: %v", err)
	}
	if len(ps.TodoFiles) != 0 {
		t.Errorf("expected 0 todo files (subdirectory), got %d", len(ps.TodoFiles))
	}
}

func TestScanUnparseableEmpty(t *testing.T) {
	dir := t.TempDir()
	files, err := ScanUnparseable(dir)
	if err != nil {
		t.Fatalf("ScanUnparseable failed: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected 0 unparseable files, got %d", len(files))
	}
}

func TestScanUnparseableFindsMalformedFiles(t *testing.T) {
	dir := t.TempDir()
	readyDir := filepath.Join(dir, "test-ready")
	os.MkdirAll(readyDir, 0755)

	os.WriteFile(filepath.Join(readyDir, "good.md"), []byte("# Good Issue\n\nBody"), 0644)
	os.WriteFile(filepath.Join(readyDir, "empty.md"), []byte{}, 0644)
	os.WriteFile(filepath.Join(readyDir, "no-heading.md"), []byte("No heading here\n"), 0644)
	os.WriteFile(filepath.Join(readyDir, "whitespace.md"), []byte("   \n\n"), 0644)

	files, err := ScanUnparseable(dir)
	if err != nil {
		t.Fatalf("ScanUnparseable failed: %v", err)
	}
	if len(files) != 3 {
		t.Fatalf("expected 3 unparseable files, got %d", len(files))
	}
	for _, f := range files {
		if f.Err == nil {
			t.Errorf("expected non-nil error for unparseable file %s", f.Path)
		}
	}
}

func TestScanUnparseableNonExistentRoot(t *testing.T) {
	files, err := ScanUnparseable("/nonexistent/path")
	if err != nil {
		t.Fatalf("ScanUnparseable failed: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected 0 files for non-existent root, got %d", len(files))
	}
}

func TestScanUnparseableAllStates(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "test-ready"), 0755)
	os.MkdirAll(filepath.Join(dir, "done"), 0755)
	os.MkdirAll(filepath.Join(dir, ".quarantine"), 0755)

	// Valid files
	os.WriteFile(filepath.Join(dir, "todo.md"), []byte("# Todo\n\nBody"), 0644)
	os.WriteFile(filepath.Join(dir, "test-ready", "ready.md"), []byte("# Ready\n\nBody"), 0644)

	// Malformed files in each state
	os.WriteFile(filepath.Join(dir, "bad-todo.md"), []byte("No heading"), 0644)
	os.WriteFile(filepath.Join(dir, "test-ready", "bad-ready.md"), []byte{}, 0644)
	os.WriteFile(filepath.Join(dir, "done", "bad-done.md"), []byte("   "), 0644)
	os.WriteFile(filepath.Join(dir, ".quarantine", "bad-quar.md"), []byte{}, 0644)

	files, err := ScanUnparseable(dir)
	if err != nil {
		t.Fatalf("ScanUnparseable failed: %v", err)
	}
	if len(files) != 4 {
		t.Fatalf("expected 4 unparseable files across all states, got %d", len(files))
	}
}

func TestScanUnparseableSkipsNonMdFiles(t *testing.T) {
	dir := t.TempDir()
	readyDir := filepath.Join(dir, "test-ready")
	os.MkdirAll(readyDir, 0755)

	os.WriteFile(filepath.Join(readyDir, "note.txt"), []byte("not an issue"), 0644)
	os.WriteFile(filepath.Join(readyDir, "data.json"), []byte(`{}`), 0644)

	files, err := ScanUnparseable(dir)
	if err != nil {
		t.Fatalf("ScanUnparseable failed: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected 0 unparseable files (non-md skipped), got %d", len(files))
	}
}

func TestScanParsedPopulatedFields(t *testing.T) {
	dir := t.TempDir()
	readyDir := filepath.Join(dir, "test-ready")
	os.MkdirAll(readyDir, 0755)

	content := "# 05 - Implement login\n\nGitHub: #42\nStatus: ready-for-agent\nExecution mode: AFK-only\nBranch: feat/login\n"
	os.WriteFile(filepath.Join(readyDir, "login.md"), []byte(content), 0644)

	files, err := ScanParsed(dir)
	if err != nil {
		t.Fatalf("ScanParsed failed: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}

	f := files[0]
	if f.Title != "05 - Implement login" {
		t.Errorf("Title = %q, want %q", f.Title, "05 - Implement login")
	}
	if f.GitHubNum != 42 {
		t.Errorf("GitHubNum = %d, want 42", f.GitHubNum)
	}
	if f.ExecMode != "AFK-only" {
		t.Errorf("ExecMode = %q, want %q", f.ExecMode, "AFK-only")
	}
	if f.Branch != "feat/login" {
		t.Errorf("Branch = %q, want %q", f.Branch, "feat/login")
	}
	if f.State != StateTestReady {
		t.Errorf("State = %q, want %q", f.State, StateTestReady)
	}
}

func setupIssueFile(t *testing.T, dir, stateDir, name, content string) string {
	t.Helper()
	sd := filepath.Join(dir, stateDir)
	os.MkdirAll(sd, 0755)
	p := filepath.Join(sd, name)
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatalf("write issue file: %v", err)
	}
	return p
}

func TestPreFlightCheckEmpty(t *testing.T) {
	state := &PipelineState{}
	issues := PreFlightCheck(state, false, true)
	if len(issues) != 0 {
		t.Errorf("expected 0 issues, got %d", len(issues))
	}
}

func TestPreFlightCheckDeadBlocker(t *testing.T) {
	dir := t.TempDir()
	setupIssueFile(t, dir, "test-ready", "task.md", "# Task\n\nGitHub: #42\nExecution mode: AFK-only\n\n## Parent\n\n## What to build\n\n## User stories covered\n\n## Acceptance criteria\n\n## UAT plan\n\n## Blocked by\n- 99 — Missing\n")

	state := &PipelineState{
		TestReadyFiles: []string{filepath.Join(dir, "test-ready", "task.md")},
	}

	issues := PreFlightCheck(state, false, true)
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
	if issues[0].Severity != SeverityError {
		t.Errorf("expected severity %q, got %q", SeverityError, issues[0].Severity)
	}
	if !strings.Contains(issues[0].Message, "#99") {
		t.Errorf("expected message to mention #99, got %q", issues[0].Message)
	}
}

func TestPreFlightCheckSelfBlocking(t *testing.T) {
	dir := t.TempDir()
	setupIssueFile(t, dir, "test-ready", "task.md", "# Task\n\nGitHub: #42\nExecution mode: AFK-only\n\n## Parent\n\n## What to build\n\n## User stories covered\n\n## Acceptance criteria\n\n## UAT plan\n\n## Blocked by\n- 42\n")

	state := &PipelineState{
		TestReadyFiles: []string{filepath.Join(dir, "test-ready", "task.md")},
	}

	issues := PreFlightCheck(state, false, true)
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
	if !strings.Contains(issues[0].Message, "blocks itself") {
		t.Errorf("expected self-blocking message, got %q", issues[0].Message)
	}
}

func TestPreFlightCheckDuplicateGitHubNum(t *testing.T) {
	dir := t.TempDir()
	setupIssueFile(t, dir, "done", "task1.md", "# Task One\n\nGitHub: #42\nExecution mode: AFK-only\n\n## Parent\n\n## What to build\n\n## User stories covered\n\n## Acceptance criteria\n\n## UAT plan\n\n## Blocked by\n")
	setupIssueFile(t, dir, "test-ready", "task2.md", "# Task Two\n\nGitHub: #42\nExecution mode: AFK-only\n\n## Parent\n\n## What to build\n\n## User stories covered\n\n## Acceptance criteria\n\n## UAT plan\n\n## Blocked by\n")

	state := &PipelineState{
		DoneFiles:      []string{filepath.Join(dir, "done", "task1.md")},
		TestReadyFiles: []string{filepath.Join(dir, "test-ready", "task2.md")},
	}

	issues := PreFlightCheck(state, false, true)
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
	if !strings.Contains(issues[0].Message, "#42") {
		t.Errorf("expected message to mention #42, got %q", issues[0].Message)
	}
	if !strings.Contains(issues[0].Message, "multiple") {
		t.Errorf("expected message to mention 'multiple', got %q", issues[0].Message)
	}
}

func TestPreFlightCheckBlockerResolvedByDone(t *testing.T) {
	dir := t.TempDir()
	setupIssueFile(t, dir, "done", "task1-42.md", "# Task One\n\nGitHub: #42\nExecution mode: AFK-only\n\n## Parent\n\n## What to build\n\n## User stories covered\n\n## Acceptance criteria\n\n## UAT plan\n\n## Blocked by\n")
	setupIssueFile(t, dir, "test-ready", "task2.md", "# Task Two\n\nGitHub: #43\nExecution mode: AFK-only\n\n## Parent\n\n## What to build\n\n## User stories covered\n\n## Acceptance criteria\n\n## UAT plan\n\n## Blocked by\n- 42\n")

	state := &PipelineState{
		DoneFiles:      []string{filepath.Join(dir, "done", "task1-42.md")},
		TestReadyFiles: []string{filepath.Join(dir, "test-ready", "task2.md")},
	}

	issues := PreFlightCheck(state, false, true)
	if len(issues) != 0 {
		t.Errorf("expected 0 issues (blocker resolved by done), got %d", len(issues))
	}
}

func TestPreFlightCheckAllStates(t *testing.T) {
	dir := t.TempDir()
	setupIssueFile(t, dir, "", "todo.md", "# Todo\n\nGitHub: #10\nExecution mode: AFK-only\n\n## Parent\n\n## What to build\n\n## User stories covered\n\n## Acceptance criteria\n\n## UAT plan\n\n## Blocked by\n")
	setupIssueFile(t, dir, "test-ready", "ready.md", "# Ready\n\nGitHub: #20\nExecution mode: AFK-only\n\n## Parent\n\n## What to build\n\n## User stories covered\n\n## Acceptance criteria\n\n## UAT plan\n\n## Blocked by\n- 99\n")
	setupIssueFile(t, dir, "done", "done.md", "# Done\n\nGitHub: #30\nExecution mode: AFK-only\n\n## Parent\n\n## What to build\n\n## User stories covered\n\n## Acceptance criteria\n\n## UAT plan\n\n## Blocked by\n")
	setupIssueFile(t, dir, ".quarantine", "quar.md", "# Quar\n\nGitHub: #40\nExecution mode: AFK-only\n\n## Parent\n\n## What to build\n\n## User stories covered\n\n## Acceptance criteria\n\n## UAT plan\n\n## Blocked by\n")

	state := &PipelineState{
		TodoFiles:       []string{filepath.Join(dir, "todo.md")},
		TestReadyFiles:  []string{filepath.Join(dir, "test-ready", "ready.md")},
		DoneFiles:       []string{filepath.Join(dir, "done", "done.md")},
		QuarantineFiles: []string{filepath.Join(dir, ".quarantine", "quar.md")},
	}

	issues := PreFlightCheck(state, false, true)
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue (dead blocker #99), got %d", len(issues))
	}
	if !strings.Contains(issues[0].Message, "#99") {
		t.Errorf("expected message to mention #99, got %q", issues[0].Message)
	}
}

func TestPreFlightCheckSkipsMalformedFiles(t *testing.T) {
	dir := t.TempDir()
	setupIssueFile(t, dir, "test-ready", "empty.md", "")
	setupIssueFile(t, dir, "todo", "good.md", "# Good\n\nGitHub: #1\nExecution mode: AFK-only\n\n## Parent\n\n## What to build\n\n## User stories covered\n\n## Acceptance criteria\n\n## UAT plan\n\n## Blocked by\n")

	state := &PipelineState{
		TodoFiles:      []string{filepath.Join(dir, "todo", "good.md")},
		TestReadyFiles: []string{filepath.Join(dir, "test-ready", "empty.md")},
	}

	issues := PreFlightCheck(state, false, true)
	var execIssues []PreFlightIssue
	for _, iss := range issues {
		if strings.Contains(iss.Message, "Execution mode") {
			execIssues = append(execIssues, iss)
		}
	}
	if len(execIssues) != 1 {
		t.Fatalf("expected 1 Execution mode issue, got %d: %v", len(execIssues), execIssues)
	}
}

func TestPreFlightCheckBlockerInTodo(t *testing.T) {
	dir := t.TempDir()
	setupIssueFile(t, dir, "", "todo-blocker.md", "# Blocker\n\nGitHub: #5\nExecution mode: AFK-only\n\n## Parent\n\n## What to build\n\n## User stories covered\n\n## Acceptance criteria\n\n## UAT plan\n\n## Blocked by\n")
	setupIssueFile(t, dir, "test-ready", "task.md", "# Task\n\nGitHub: #10\nExecution mode: AFK-only\n\n## Parent\n\n## What to build\n\n## User stories covered\n\n## Acceptance criteria\n\n## UAT plan\n\n## Blocked by\n- 5\n")

	state := &PipelineState{
		TodoFiles:      []string{filepath.Join(dir, "todo-blocker.md")},
		TestReadyFiles: []string{filepath.Join(dir, "test-ready", "task.md")},
	}

	issues := PreFlightCheck(state, false, true)
	if len(issues) != 0 {
		t.Errorf("expected 0 issues (blocker in todo is valid), got %d", len(issues))
	}
}

func TestPreFlightCheckDuplicateFilenamesAcrossStates(t *testing.T) {
	dir := t.TempDir()
	setupIssueFile(t, dir, "", "same-name.md", "# Todo\n\nGitHub: #10\n")
	setupIssueFile(t, dir, "test-ready", "same-name.md", "# Ready\n\nGitHub: #20\n")

	state := &PipelineState{
		TodoFiles:      []string{filepath.Join(dir, "same-name.md")},
		TestReadyFiles: []string{filepath.Join(dir, "test-ready", "same-name.md")},
	}

	issues := PreFlightCheck(state, false, true)
	var dupIssues []PreFlightIssue
	for _, i := range issues {
		if strings.Contains(i.Message, "duplicate filename") {
			dupIssues = append(dupIssues, i)
		}
	}
	if len(dupIssues) != 1 {
		t.Fatalf("expected 1 duplicate filename issue, got %d", len(dupIssues))
	}
	if !strings.Contains(dupIssues[0].Message, "same-name.md") {
		t.Errorf("expected message to mention 'same-name.md', got %q", dupIssues[0].Message)
	}
	if dupIssues[0].Severity != SeverityError {
		t.Errorf("expected severity %q, got %q", SeverityError, dupIssues[0].Severity)
	}
}

func TestPreFlightCheckDuplicateFilenamesThreeStates(t *testing.T) {
	dir := t.TempDir()
	setupIssueFile(t, dir, "", "dup.md", "# Todo\n\nGitHub: #10\n")
	setupIssueFile(t, dir, "test-ready", "dup.md", "# Ready\n\nGitHub: #20\n")
	setupIssueFile(t, dir, "done", "dup.md", "# Done\n\nGitHub: #30\n")

	state := &PipelineState{
		TodoFiles:      []string{filepath.Join(dir, "dup.md")},
		TestReadyFiles: []string{filepath.Join(dir, "test-ready", "dup.md")},
		DoneFiles:      []string{filepath.Join(dir, "done", "dup.md")},
	}

	issues := PreFlightCheck(state, false, true)
	var dupIssues []PreFlightIssue
	for _, i := range issues {
		if strings.Contains(i.Message, "duplicate filename") {
			dupIssues = append(dupIssues, i)
		}
	}
	if len(dupIssues) != 1 {
		t.Fatalf("expected 1 duplicate filename issue, got %d", len(dupIssues))
	}
	if !strings.Contains(dupIssues[0].Message, "dup.md") {
		t.Errorf("expected message to mention 'dup.md', got %q", dupIssues[0].Message)
	}
}

func TestPreFlightCheckUniqueFilenames(t *testing.T) {
	dir := t.TempDir()
	setupIssueFile(t, dir, "", "todo.md", "# Todo\n\nGitHub: #10\nExecution mode: AFK-only\n")
	setupIssueFile(t, dir, "test-ready", "ready.md", "# Ready\n\nGitHub: #20\nExecution mode: AFK-only\n")
	setupIssueFile(t, dir, "done", "done.md", "# Done\n\nGitHub: #30\nExecution mode: AFK-only\n")

	state := &PipelineState{
		TodoFiles:      []string{filepath.Join(dir, "todo.md")},
		TestReadyFiles: []string{filepath.Join(dir, "test-ready", "ready.md")},
		DoneFiles:      []string{filepath.Join(dir, "done", "done.md")},
	}

	issues := PreFlightCheck(state, false, true)
	for _, i := range issues {
		if strings.Contains(i.Message, "duplicate filename") {
			t.Errorf("unexpected duplicate filename issue: %s", i.Message)
		}
	}
}

func TestDetectDuplicateFilenamesEmpty(t *testing.T) {
	state := &PipelineState{}
	issues := DetectDuplicateFilenames(state)
	if len(issues) != 0 {
		t.Errorf("expected 0 issues, got %d", len(issues))
	}
}

func TestDetectDuplicateFilenamesNoDuplicates(t *testing.T) {
	dir := t.TempDir()
	setupIssueFile(t, dir, "", "todo.md", "# Todo\n\nGitHub: #10\n")
	setupIssueFile(t, dir, "test-ready", "ready.md", "# Ready\n\nGitHub: #20\n")
	setupIssueFile(t, dir, "done", "done.md", "# Done\n\nGitHub: #30\n")

	state := &PipelineState{
		TodoFiles:      []string{filepath.Join(dir, "todo.md")},
		TestReadyFiles: []string{filepath.Join(dir, "test-ready", "ready.md")},
		DoneFiles:      []string{filepath.Join(dir, "done", "done.md")},
	}

	issues := DetectDuplicateFilenames(state)
	for _, i := range issues {
		if strings.Contains(i.Message, "duplicate filename") {
			t.Errorf("unexpected duplicate filename issue: %s", i.Message)
		}
	}
	if len(issues) != 0 {
		t.Errorf("expected 0 issues, got %d", len(issues))
	}
}

func TestDetectDuplicateFilenamesTwoStates(t *testing.T) {
	dir := t.TempDir()
	todoPath := setupIssueFile(t, dir, "", "dup.md", "# Todo\n\nGitHub: #10\n")
	readyPath := setupIssueFile(t, dir, "test-ready", "dup.md", "# Ready\n\nGitHub: #20\n")

	state := &PipelineState{
		TodoFiles:      []string{todoPath},
		TestReadyFiles: []string{readyPath},
	}

	issues := DetectDuplicateFilenames(state)
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
	if !strings.Contains(issues[0].Message, `duplicate filename "dup.md"`) {
		t.Errorf("expected message mentioning dup.md, got %q", issues[0].Message)
	}
	if issues[0].Severity != SeverityError {
		t.Errorf("expected severity %q, got %q", SeverityError, issues[0].Severity)
	}
}

func TestDetectDuplicateFilenamesThreeStates(t *testing.T) {
	dir := t.TempDir()
	setupIssueFile(t, dir, "", "dup.md", "# Todo\n\nGitHub: #10\n")
	setupIssueFile(t, dir, "test-ready", "dup.md", "# Ready\n\nGitHub: #20\n")
	setupIssueFile(t, dir, "done", "dup.md", "# Done\n\nGitHub: #30\n")

	state := &PipelineState{
		TodoFiles:      []string{filepath.Join(dir, "dup.md")},
		TestReadyFiles: []string{filepath.Join(dir, "test-ready", "dup.md")},
		DoneFiles:      []string{filepath.Join(dir, "done", "dup.md")},
	}

	issues := DetectDuplicateFilenames(state)
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
}

func TestDetectDuplicateFilenamesMultipleGroups(t *testing.T) {
	dir := t.TempDir()
	setupIssueFile(t, dir, "", "a.md", "# A\n\nGitHub: #10\n")
	setupIssueFile(t, dir, "test-ready", "a.md", "# A\n\nGitHub: #20\n")
	setupIssueFile(t, dir, "", "b.md", "# B\n\nGitHub: #30\n")
	setupIssueFile(t, dir, "done", "b.md", "# B\n\nGitHub: #40\n")

	state := &PipelineState{
		TodoFiles:      []string{filepath.Join(dir, "a.md"), filepath.Join(dir, "b.md")},
		TestReadyFiles: []string{filepath.Join(dir, "test-ready", "a.md")},
		DoneFiles:      []string{filepath.Join(dir, "done", "b.md")},
	}

	issues := DetectDuplicateFilenames(state)
	if len(issues) != 2 {
		t.Fatalf("expected 2 issues, got %d", len(issues))
	}
}

func TestDetectDuplicateFilenamesIgnoresQuarantine(t *testing.T) {
	dir := t.TempDir()
	todoPath := setupIssueFile(t, dir, "", "dup.md", "# Todo\n\nGitHub: #10\n")
	quarPath := setupIssueFile(t, dir, ".quarantine", "dup.md", "# Quar\n\nGitHub: #20\n")

	state := &PipelineState{
		TodoFiles:       []string{todoPath},
		QuarantineFiles: []string{quarPath},
	}

	issues := DetectDuplicateFilenames(state)
	if len(issues) != 0 {
		t.Errorf("expected 0 issues (quarantine excluded), got %d: %v", len(issues), issues)
	}
}

func TestDetectDuplicateGitHubNumsEmpty(t *testing.T) {
	state := &PipelineState{}
	issues := DetectDuplicateGitHubNums(state)
	if len(issues) != 0 {
		t.Errorf("expected 0 issues, got %d", len(issues))
	}
}

func TestDetectDuplicateGitHubNumsNoDuplicates(t *testing.T) {
	dir := t.TempDir()
	setupIssueFile(t, dir, "", "one.md", "# One\n\nGitHub: #10\n")
	setupIssueFile(t, dir, "test-ready", "two.md", "# Two\n\nGitHub: #20\n")
	setupIssueFile(t, dir, "done", "three.md", "# Three\n\nGitHub: #30\n")

	state := &PipelineState{
		TodoFiles:      []string{filepath.Join(dir, "one.md")},
		TestReadyFiles: []string{filepath.Join(dir, "test-ready", "two.md")},
		DoneFiles:      []string{filepath.Join(dir, "done", "three.md")},
	}

	issues := DetectDuplicateGitHubNums(state)
	if len(issues) != 0 {
		t.Errorf("expected 0 issues, got %d", len(issues))
	}
}

func TestDetectDuplicateGitHubNumsSameNumberAcrossStates(t *testing.T) {
	dir := t.TempDir()
	donePath := setupIssueFile(t, dir, "done", "task1.md", "# Task One\n\nGitHub: #42\n")
	readyPath := setupIssueFile(t, dir, "test-ready", "task2.md", "# Task Two\n\nGitHub: #42\n")

	state := &PipelineState{
		DoneFiles:      []string{donePath},
		TestReadyFiles: []string{readyPath},
	}

	issues := DetectDuplicateGitHubNums(state)
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
	if !strings.Contains(issues[0].Message, "#42") {
		t.Errorf("expected message to mention #42, got %q", issues[0].Message)
	}
	if !strings.Contains(issues[0].Message, "multiple") {
		t.Errorf("expected message to mention 'multiple', got %q", issues[0].Message)
	}
	if issues[0].Severity != SeverityError {
		t.Errorf("expected severity %q, got %q", SeverityError, issues[0].Severity)
	}
}

func TestDetectDuplicateGitHubNumsSameState(t *testing.T) {
	dir := t.TempDir()
	aPath := setupIssueFile(t, dir, "", "a.md", "# A\n\nGitHub: #42\n")
	bPath := setupIssueFile(t, dir, "", "b.md", "# B\n\nGitHub: #42\n")

	state := &PipelineState{
		TodoFiles: []string{aPath, bPath},
	}

	issues := DetectDuplicateGitHubNums(state)
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
	if !strings.Contains(issues[0].Message, "#42") {
		t.Errorf("expected message to mention #42, got %q", issues[0].Message)
	}
}

func TestDetectDuplicateGitHubNumsThreeStates(t *testing.T) {
	dir := t.TempDir()
	setupIssueFile(t, dir, "", "a.md", "# A\n\nGitHub: #7\n")
	setupIssueFile(t, dir, "test-ready", "b.md", "# B\n\nGitHub: #7\n")
	setupIssueFile(t, dir, "done", "c.md", "# C\n\nGitHub: #7\n")

	state := &PipelineState{
		TodoFiles:      []string{filepath.Join(dir, "a.md")},
		TestReadyFiles: []string{filepath.Join(dir, "test-ready", "b.md")},
		DoneFiles:      []string{filepath.Join(dir, "done", "c.md")},
	}

	issues := DetectDuplicateGitHubNums(state)
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
}

func TestDetectDuplicateGitHubNumsMultipleGroups(t *testing.T) {
	dir := t.TempDir()
	setupIssueFile(t, dir, "", "a.md", "# A\n\nGitHub: #10\n")
	setupIssueFile(t, dir, "test-ready", "a2.md", "# A2\n\nGitHub: #10\n")
	setupIssueFile(t, dir, "", "b.md", "# B\n\nGitHub: #20\n")
	setupIssueFile(t, dir, "done", "b2.md", "# B2\n\nGitHub: #20\n")

	state := &PipelineState{
		TodoFiles:      []string{filepath.Join(dir, "a.md"), filepath.Join(dir, "b.md")},
		TestReadyFiles: []string{filepath.Join(dir, "test-ready", "a2.md")},
		DoneFiles:      []string{filepath.Join(dir, "done", "b2.md")},
	}

	issues := DetectDuplicateGitHubNums(state)
	if len(issues) != 2 {
		t.Fatalf("expected 2 issues, got %d", len(issues))
	}
}

func TestDetectDuplicateGitHubNumsIgnoresQuarantine(t *testing.T) {
	dir := t.TempDir()
	todoPath := setupIssueFile(t, dir, "", "todo.md", "# Todo\n\nGitHub: #42\n")
	quarPath := setupIssueFile(t, dir, ".quarantine", "quar.md", "# Quar\n\nGitHub: #42\n")

	state := &PipelineState{
		TodoFiles:       []string{todoPath},
		QuarantineFiles: []string{quarPath},
	}

	issues := DetectDuplicateGitHubNums(state)
	if len(issues) != 0 {
		t.Errorf("expected 0 issues (quarantine excluded), got %d: %v", len(issues), issues)
	}
}

func TestDetectDuplicateGitHubNumsIgnoresZero(t *testing.T) {
	dir := t.TempDir()
	setupIssueFile(t, dir, "", "a.md", "# A\n")
	setupIssueFile(t, dir, "done", "b.md", "# B\n")

	state := &PipelineState{
		TodoFiles: []string{filepath.Join(dir, "a.md")},
		DoneFiles: []string{filepath.Join(dir, "done", "b.md")},
	}

	issues := DetectDuplicateGitHubNums(state)
	if len(issues) != 0 {
		t.Errorf("expected 0 issues for files without GitHub number, got %d", len(issues))
	}
}

func TestDetectDuplicateGitHubNumsSkipsMalformedFiles(t *testing.T) {
	dir := t.TempDir()
	setupIssueFile(t, dir, "", "good.md", "# Good\n\nGitHub: #42\n")
	badPath := filepath.Join(dir, "bad.md")
	os.WriteFile(badPath, []byte(""), 0644)

	state := &PipelineState{
		TodoFiles: []string{filepath.Join(dir, "good.md"), badPath},
	}

	issues := DetectDuplicateGitHubNums(state)
	if len(issues) != 0 {
		t.Errorf("expected 0 issues (single GH number), got %d", len(issues))
	}
}

func TestDetectDuplicateTitlesEmpty(t *testing.T) {
	state := &PipelineState{}
	issues := DetectDuplicateTitles(state)
	if len(issues) != 0 {
		t.Errorf("expected 0 issues, got %d", len(issues))
	}
}

func TestDetectDuplicateTitlesNoDuplicates(t *testing.T) {
	dir := t.TempDir()
	setupIssueFile(t, dir, "", "one.md", "# One\n\nGitHub: #10\n")
	setupIssueFile(t, dir, "test-ready", "two.md", "# Two\n\nGitHub: #20\n")
	setupIssueFile(t, dir, "done", "three.md", "# Three\n\nGitHub: #30\n")

	state := &PipelineState{
		TodoFiles:      []string{filepath.Join(dir, "one.md")},
		TestReadyFiles: []string{filepath.Join(dir, "test-ready", "two.md")},
		DoneFiles:      []string{filepath.Join(dir, "done", "three.md")},
	}

	issues := DetectDuplicateTitles(state)
	if len(issues) != 0 {
		t.Errorf("expected 0 issues, got %d", len(issues))
	}
}

func TestDetectDuplicateTitlesAcrossStates(t *testing.T) {
	dir := t.TempDir()
	setupIssueFile(t, dir, "", "login-impl.md", "# Implement login\n\nGitHub: #10\n")
	setupIssueFile(t, dir, "test-ready", "login-test.md", "# Implement login\n\nGitHub: #20\n")

	state := &PipelineState{
		TodoFiles:      []string{filepath.Join(dir, "login-impl.md")},
		TestReadyFiles: []string{filepath.Join(dir, "test-ready", "login-test.md")},
	}

	issues := DetectDuplicateTitles(state)
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
	if !strings.Contains(issues[0].Message, `"Implement login"`) {
		t.Errorf("expected message to mention title, got %q", issues[0].Message)
	}
	if issues[0].Severity != SeverityError {
		t.Errorf("expected severity %q, got %q", SeverityError, issues[0].Severity)
	}
}

func TestDetectDuplicateTitlesSameState(t *testing.T) {
	dir := t.TempDir()
	setupIssueFile(t, dir, "", "login-a.md", "# Implement login\n\nGitHub: #10\n")
	setupIssueFile(t, dir, "", "login-b.md", "# Implement login\n\nGitHub: #20\n")

	state := &PipelineState{
		TodoFiles: []string{filepath.Join(dir, "login-a.md"), filepath.Join(dir, "login-b.md")},
	}

	issues := DetectDuplicateTitles(state)
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
	if !strings.Contains(issues[0].Message, `"Implement login"`) {
		t.Errorf("expected message to mention title, got %q", issues[0].Message)
	}
}

func TestDetectDuplicateTitlesThreeStates(t *testing.T) {
	dir := t.TempDir()
	setupIssueFile(t, dir, "", "a.md", "# Common title\n\nGitHub: #10\n")
	setupIssueFile(t, dir, "test-ready", "b.md", "# Common title\n\nGitHub: #20\n")
	setupIssueFile(t, dir, "done", "c.md", "# Common title\n\nGitHub: #30\n")

	state := &PipelineState{
		TodoFiles:      []string{filepath.Join(dir, "a.md")},
		TestReadyFiles: []string{filepath.Join(dir, "test-ready", "b.md")},
		DoneFiles:      []string{filepath.Join(dir, "done", "c.md")},
	}

	issues := DetectDuplicateTitles(state)
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
}

func TestDetectDuplicateTitlesMultipleGroups(t *testing.T) {
	dir := t.TempDir()
	setupIssueFile(t, dir, "", "login-a.md", "# Implement login\n\nGitHub: #10\n")
	setupIssueFile(t, dir, "test-ready", "login-b.md", "# Implement login\n\nGitHub: #20\n")
	setupIssueFile(t, dir, "", "logout-a.md", "# Add logout\n\nGitHub: #30\n")
	setupIssueFile(t, dir, "done", "logout-b.md", "# Add logout\n\nGitHub: #40\n")

	state := &PipelineState{
		TodoFiles:      []string{filepath.Join(dir, "login-a.md"), filepath.Join(dir, "logout-a.md")},
		TestReadyFiles: []string{filepath.Join(dir, "test-ready", "login-b.md")},
		DoneFiles:      []string{filepath.Join(dir, "done", "logout-b.md")},
	}

	issues := DetectDuplicateTitles(state)
	if len(issues) != 2 {
		t.Fatalf("expected 2 issues, got %d", len(issues))
	}
}

func TestDetectDuplicateTitlesIgnoresQuarantine(t *testing.T) {
	dir := t.TempDir()
	todoPath := setupIssueFile(t, dir, "", "todo.md", "# Implement login\n\nGitHub: #42\n")
	quarPath := setupIssueFile(t, dir, ".quarantine", "quar.md", "# Implement login\n\nGitHub: #99\n")

	state := &PipelineState{
		TodoFiles:       []string{todoPath},
		QuarantineFiles: []string{quarPath},
	}

	issues := DetectDuplicateTitles(state)
	if len(issues) != 0 {
		t.Errorf("expected 0 issues (quarantine excluded), got %d: %v", len(issues), issues)
	}
}

func TestDetectDuplicateTitlesSkipsMalformedFiles(t *testing.T) {
	dir := t.TempDir()
	goodPath := setupIssueFile(t, dir, "", "good.md", "# Good\n\nGitHub: #42\n")
	badPath := filepath.Join(dir, "bad.md")
	os.WriteFile(badPath, []byte(""), 0644)

	state := &PipelineState{
		TodoFiles: []string{goodPath, badPath},
	}

	issues := DetectDuplicateTitles(state)
	if len(issues) != 0 {
		t.Errorf("expected 0 issues (single valid title), got %d", len(issues))
	}
}

func TestDetectDuplicateTitlesPreFlightCheckIntegration(t *testing.T) {
	dir := t.TempDir()
	setupIssueFile(t, dir, "", "login-impl.md", "# Implement login\n\nGitHub: #10\nExecution mode: AFK-only\n\n## Parent\n\n## What to build\n\n## User stories covered\n\n## Acceptance criteria\n\n## UAT plan\n\n## Blocked by\n")
	setupIssueFile(t, dir, "test-ready", "login-test.md", "# Implement login\n\nGitHub: #20\nExecution mode: AFK-only\n\n## Parent\n\n## What to build\n\n## User stories covered\n\n## Acceptance criteria\n\n## UAT plan\n\n## Blocked by\n")

	state := &PipelineState{
		TodoFiles:      []string{filepath.Join(dir, "login-impl.md")},
		TestReadyFiles: []string{filepath.Join(dir, "test-ready", "login-test.md")},
	}

	issues := PreFlightCheck(state, false, true)
	var titleDupIssues []PreFlightIssue
	for _, i := range issues {
		if strings.Contains(i.Message, "duplicate title") {
			titleDupIssues = append(titleDupIssues, i)
		}
	}
	if len(titleDupIssues) != 1 {
		t.Fatalf("expected 1 duplicate title issue from PreFlightCheck, got %d", len(titleDupIssues))
	}
	if !strings.Contains(titleDupIssues[0].Message, `"Implement login"`) {
		t.Errorf("expected message to mention title, got %q", titleDupIssues[0].Message)
	}
}

func TestExtractBlockerNum(t *testing.T) {
	tests := []struct {
		ref     string
		wantNum int
		wantOK  bool
	}{
		{"42", 42, true},
		{"99 — Missing issue", 99, true},
		{"02 — Status", 2, true},
		{"#42", 42, true},
		{"42.md", 42, true},
		{"text without number", 0, false},
		{"", 0, false},
	}
	for _, tc := range tests {
		n, ok := extractBlockerNum(tc.ref)
		if n != tc.wantNum || ok != tc.wantOK {
			t.Errorf("extractBlockerNum(%q) = (%d, %t), want (%d, %t)", tc.ref, n, ok, tc.wantNum, tc.wantOK)
		}
	}
}

func TestExecModeAllowsImplement(t *testing.T) {
	tests := []struct {
		mode string
		want bool
	}{
		{ExecModeAFKOnly, true},
		{ExecModeHITLOnly, false},
		{ExecModeCombo, false},
		{"", false},
		{"unknown", false},
	}
	for _, tc := range tests {
		got := ExecModeAllowsImplement(tc.mode)
		if got != tc.want {
			t.Errorf("ExecModeAllowsImplement(%q) = %v, want %v", tc.mode, got, tc.want)
		}
	}
}

func TestTransitionAllFields(t *testing.T) {
	tr := Transition{
		SourceDir: "/issues/todo",
		DestDir:   "/issues/done",
		Filename:  "fix-bug.md",
	}
	if tr.SourceDir != "/issues/todo" {
		t.Errorf("SourceDir = %q, want %q", tr.SourceDir, "/issues/todo")
	}
	if tr.DestDir != "/issues/done" {
		t.Errorf("DestDir = %q, want %q", tr.DestDir, "/issues/done")
	}
	if tr.Filename != "fix-bug.md" {
		t.Errorf("Filename = %q, want %q", tr.Filename, "fix-bug.md")
	}
}

func TestTransitionZeroValue(t *testing.T) {
	var tr Transition
	if tr.SourceDir != "" {
		t.Errorf("SourceDir zero value = %q, want empty", tr.SourceDir)
	}
	if tr.DestDir != "" {
		t.Errorf("DestDir zero value = %q, want empty", tr.DestDir)
	}
	if tr.Filename != "" {
		t.Errorf("Filename zero value = %q, want empty", tr.Filename)
	}
}

func TestComputeTransitionImplementComplete(t *testing.T) {
	file := &IssueFile{
		FilePath: "/tmp/issues/my-issue.md",
		State:    StateTodo,
	}
	tr, err := ComputeTransition(file, agent.Complete, RoleImplement)
	if err != nil {
		t.Fatalf("ComputeTransition failed: %v", err)
	}
	if tr.SourceDir != "/tmp/issues" {
		t.Errorf("SourceDir = %q, want %q", tr.SourceDir, "/tmp/issues")
	}
	wantDest := filepath.Join("/tmp/issues", string(StateTestReady))
	if tr.DestDir != wantDest {
		t.Errorf("DestDir = %q, want %q", tr.DestDir, wantDest)
	}
	if tr.Filename != "my-issue.md" {
		t.Errorf("Filename = %q, want %q", tr.Filename, "my-issue.md")
	}
}

func TestComputeTransitionImplementTestPassRejected(t *testing.T) {
	file := &IssueFile{
		FilePath: "/tmp/issues/my-issue.md",
		State:    StateTodo,
	}
	_, err := ComputeTransition(file, agent.TestPass, RoleImplement)
	if err == nil {
		t.Fatal("expected error: IMPLEMENTING cannot emit TEST_PASS")
	}
}

func TestComputeTransitionImplementTestFailRejected(t *testing.T) {
	file := &IssueFile{
		FilePath: "/tmp/issues/my-issue.md",
		State:    StateTodo,
	}
	_, err := ComputeTransition(file, agent.TestFail, RoleImplement)
	if err == nil {
		t.Fatal("expected error: IMPLEMENTING cannot emit TEST_FAIL")
	}
}

func TestComputeTransitionImplementNoMoreTasks(t *testing.T) {
	file := &IssueFile{
		FilePath: "/tmp/issues/my-issue.md",
		State:    StateTodo,
	}
	tr, err := ComputeTransition(file, agent.NoMoreTasks, RoleImplement)
	if err != nil {
		t.Fatalf("ComputeTransition failed: %v", err)
	}
	if tr == nil {
		t.Fatal("expected quarantine transition for NO_MORE_TASKS, got nil")
	}
	if !strings.HasSuffix(tr.DestDir, ".quarantine") {
		t.Errorf("expected destination .quarantine, got %q", tr.DestDir)
	}
}

func TestComputeTransitionTestCompleteRejected(t *testing.T) {
	file := &IssueFile{
		FilePath: "/tmp/issues/test-ready/my-issue.md",
		State:    StateTestReady,
	}
	_, err := ComputeTransition(file, agent.Complete, RoleTest)
	if err == nil {
		t.Fatal("expected error: TESTING cannot emit COMPLETE")
	}
}

func TestComputeTransitionTestTestPass(t *testing.T) {
	file := &IssueFile{
		FilePath: "/tmp/issues/test-ready/my-issue.md",
		State:    StateTestReady,
	}
	tr, err := ComputeTransition(file, agent.TestPass, RoleTest)
	if err != nil {
		t.Fatalf("ComputeTransition failed: %v", err)
	}
	wantDest := filepath.Join("/tmp/issues", string(StateDone))
	if tr.DestDir != wantDest {
		t.Errorf("DestDir = %q, want %q", tr.DestDir, wantDest)
	}
}

func TestComputeTransitionTestTestFail(t *testing.T) {
	file := &IssueFile{
		FilePath: "/tmp/issues/test-ready/my-issue.md",
		State:    StateTestReady,
	}
	tr, err := ComputeTransition(file, agent.TestFail, RoleTest)
	if err != nil {
		t.Fatalf("ComputeTransition failed: %v", err)
	}
	wantDest := "/tmp/issues"
	if tr.DestDir != wantDest {
		t.Errorf("DestDir = %q, want %q", tr.DestDir, wantDest)
	}
}

func TestComputeTransitionTestNoMoreTasks(t *testing.T) {
	file := &IssueFile{
		FilePath: "/tmp/issues/test-ready/my-issue.md",
		State:    StateTestReady,
	}
	tr, err := ComputeTransition(file, agent.NoMoreTasks, RoleTest)
	if err != nil {
		t.Fatalf("ComputeTransition failed: %v", err)
	}
	if tr == nil {
		t.Fatal("expected quarantine transition for NO_MORE_TASKS, got nil")
	}
	if !strings.HasSuffix(tr.DestDir, ".quarantine") {
		t.Errorf("expected destination .quarantine, got %q", tr.DestDir)
	}
}

func TestComputeTransitionErrorImplementWithTestReadyState(t *testing.T) {
	file := &IssueFile{
		FilePath: "/tmp/issues/test-ready/my-issue.md",
		State:    StateTestReady,
	}
	_, err := ComputeTransition(file, agent.Complete, RoleImplement)
	if err == nil {
		t.Fatal("expected error for implement role with test-ready file")
	}
}

func TestComputeTransitionErrorTestWithTodoState(t *testing.T) {
	file := &IssueFile{
		FilePath: "/tmp/issues/my-issue.md",
		State:    StateTodo,
	}
	_, err := ComputeTransition(file, agent.TestPass, RoleTest)
	if err == nil {
		t.Fatal("expected error for test role with todo file")
	}
}

func TestComputeTransitionErrorUnknownRole(t *testing.T) {
	file := &IssueFile{
		FilePath: "/tmp/issues/my-issue.md",
		State:    StateTodo,
	}
	_, err := ComputeTransition(file, agent.Complete, Role("invalid"))
	if err == nil {
		t.Fatal("expected error for unknown role")
	}
}

func TestComputeTransitionErrorImplementWithDoneState(t *testing.T) {
	file := &IssueFile{
		FilePath: "/tmp/issues/done/my-issue.md",
		State:    StateDone,
	}
	_, err := ComputeTransition(file, agent.Complete, RoleImplement)
	if err == nil {
		t.Fatal("expected error for implement role with done file")
	}
}

func TestComputeTransitionErrorImplementWithQuarantineState(t *testing.T) {
	file := &IssueFile{
		FilePath: "/tmp/issues/.quarantine/my-issue.md",
		State:    StateQuarantine,
	}
	_, err := ComputeTransition(file, agent.Complete, RoleImplement)
	if err == nil {
		t.Fatal("expected error for implement role with quarantine file")
	}
}

func TestComputeTransitionErrorImplementWithUnknownState(t *testing.T) {
	file := &IssueFile{
		FilePath: "/tmp/issues/other/my-issue.md",
		State:    StateUnknown,
	}
	_, err := ComputeTransition(file, agent.Complete, RoleImplement)
	if err == nil {
		t.Fatal("expected error for implement role with unknown state")
	}
}

func TestComputeTransitionErrorTestWithDoneState(t *testing.T) {
	file := &IssueFile{
		FilePath: "/tmp/issues/done/my-issue.md",
		State:    StateDone,
	}
	_, err := ComputeTransition(file, agent.TestFail, RoleTest)
	if err == nil {
		t.Fatal("expected error for test role with done file")
	}
}

func TestComputeTransitionErrorTestWithQuarantineState(t *testing.T) {
	file := &IssueFile{
		FilePath: "/tmp/issues/.quarantine/my-issue.md",
		State:    StateQuarantine,
	}
	_, err := ComputeTransition(file, agent.TestPass, RoleTest)
	if err == nil {
		t.Fatal("expected error for test role with quarantine file")
	}
}

func TestComputeTransitionErrorTestWithUnknownState(t *testing.T) {
	file := &IssueFile{
		FilePath: "/tmp/issues/other/my-issue.md",
		State:    StateUnknown,
	}
	_, err := ComputeTransition(file, agent.TestPass, RoleTest)
	if err == nil {
		t.Fatal("expected error for test role with unknown state")
	}
}

func TestIssuesRootTodo(t *testing.T) {
	root := issuesRoot("/tmp/issues/my-issue.md", StateTodo)
	if root != "/tmp/issues" {
		t.Errorf("issuesRoot = %q, want %q", root, "/tmp/issues")
	}
}

func TestIssuesRootTestReady(t *testing.T) {
	root := issuesRoot("/tmp/issues/test-ready/my-issue.md", StateTestReady)
	if root != "/tmp/issues" {
		t.Errorf("issuesRoot = %q, want %q", root, "/tmp/issues")
	}
}

func TestIssuesRootDone(t *testing.T) {
	root := issuesRoot("/tmp/issues/done/my-issue.md", StateDone)
	if root != "/tmp/issues" {
		t.Errorf("issuesRoot = %q, want %q", root, "/tmp/issues")
	}
}

func TestIssuesRootQuarantine(t *testing.T) {
	root := issuesRoot("/tmp/issues/.quarantine/my-issue.md", StateQuarantine)
	if root != "/tmp/issues" {
		t.Errorf("issuesRoot = %q, want %q", root, "/tmp/issues")
	}
}

func TestComputeTransitionErrorUnknownPromise(t *testing.T) {
	file := &IssueFile{
		FilePath: "/tmp/issues/my-issue.md",
		State:    StateTodo,
	}
	_, err := ComputeTransition(file, agent.Promise("UNKNOWN"), RoleImplement)
	if err == nil {
		t.Fatal("expected error for unknown promise")
	}
}

func TestStripIssueSectionsRemovesOneSection(t *testing.T) {
	content := "# Title\n\n## Test Results\n\nSome test output\n\nMore output\n\n## Other\n\nRemains\n"
	got := StripIssueSections(content, []string{"Test Results"})
	want := "# Title\n\n## Other\n\nRemains\n"
	if got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestStripIssueSectionsRemovesMultipleSections(t *testing.T) {
	content := "# Title\n\n## Test Results\n\nTest output\n\n## UAT Results\n\nUAT output\n\n## Other\n\nRemains\n"
	got := StripIssueSections(content, []string{"Test Results", "UAT Results"})
	want := "# Title\n\n## Other\n\nRemains\n"
	if got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestStripIssueSectionsEmptySectionsList(t *testing.T) {
	content := "# Title\n\n## Test Results\n\nContent\n"
	got := StripIssueSections(content, nil)
	if got != content {
		t.Errorf("got:\n%q\nwant:\n%q", got, content)
	}
}

func TestStripIssueSectionsSectionNotPresent(t *testing.T) {
	content := "# Title\n\n## Other\n\nContent\n"
	got := StripIssueSections(content, []string{"Missing"})
	if got != content {
		t.Errorf("got:\n%q\nwant:\n%q", got, content)
	}
}

func TestStripIssueSectionsEmptyContent(t *testing.T) {
	got := StripIssueSections("", []string{"Test"})
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestStripIssueSectionsRemovesContentUntilNextHeading(t *testing.T) {
	content := "# Title\n\n## Test Results\n\nLine1\nLine2\nLine3\n\n## Other\n\nRemains\n"
	got := StripIssueSections(content, []string{"Test Results"})
	want := "# Title\n\n## Other\n\nRemains\n"
	if got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestStripIssueSectionsRemovesAtEnd(t *testing.T) {
	content := "# Title\n\n## Other\n\nContent\n\n## Test Results\n\nFinal section\n"
	got := StripIssueSections(content, []string{"Test Results"})
	want := "# Title\n\n## Other\n\nContent\n"
	if got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestStripIssueSectionsCaseInsensitive(t *testing.T) {
	content := "# Title\n\n## test results\n\nSome content\n\n## Other\n\nRemains\n"
	got := StripIssueSections(content, []string{"Test Results"})
	want := "# Title\n\n## Other\n\nRemains\n"
	if got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestStripIssueSectionsCommonTestFailScenario(t *testing.T) {
	content := "# 05 - Implement login\n\nGitHub: #42\n\n## What to build\n\nLogin page\n\n## Test Results\n\nAll tests pass\n\n## UAT Results\n\nUAT verified\n\n## Comments\n\nDone\n"
	got := StripIssueSections(content, []string{"Test Results", "UAT Results"})
	want := "# 05 - Implement login\n\nGitHub: #42\n\n## What to build\n\nLogin page\n\n## Comments\n\nDone\n"
	if got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestStripSectionsFromFile(t *testing.T) {
	dir := t.TempDir()
	readyDir := filepath.Join(dir, "test-ready")
	os.MkdirAll(readyDir, 0755)

	path := filepath.Join(readyDir, "issue.md")
	content := "# 05 - Implement login\n\nGitHub: #42\n\n## What to build\n\nLogin page\n\n## Test Results\n\nAll tests pass\n\n## UAT Results\n\nUAT verified\n\n## Comments\n\nDone\n"
	os.WriteFile(path, []byte(content), 0644)

	if err := StripSectionsFromFile(path, []string{"Test Results", "UAT Results"}); err != nil {
		t.Fatalf("StripSectionsFromFile failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	want := "# 05 - Implement login\n\nGitHub: #42\n\n## What to build\n\nLogin page\n\n## Comments\n\nDone\n"
	if string(data) != want {
		t.Errorf("got:\n%q\nwant:\n%q", string(data), want)
	}
}

func TestStripSectionsFromFileNonExistent(t *testing.T) {
	err := StripSectionsFromFile("/nonexistent/path.md", []string{"Test Results"})
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
}

func TestStripSectionsFromFileNoSectionsToStrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "issue.md")
	content := "# Title\n\n## Other\n\nContent\n"
	os.WriteFile(path, []byte(content), 0644)

	if err := StripSectionsFromFile(path, []string{"Test Results"}); err != nil {
		t.Fatalf("StripSectionsFromFile failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(data) != content {
		t.Errorf("expected unchanged content, got:\n%q", string(data))
	}
}

func TestCleanTestResultsStripsSections(t *testing.T) {
	dir := t.TempDir()
	content := "# 01 - Implement login\n\n## What to build\n\nLogin page\n\n## Test Results\n\nAll tests pass\n\n## UAT Results\n\nUAT verified\n"
	path := filepath.Join(dir, "issue-login.md")
	os.WriteFile(path, []byte(content), 0644)

	cleaned, err := CleanTestResults(dir)
	if err != nil {
		t.Fatalf("CleanTestResults failed: %v", err)
	}
	if len(cleaned) != 1 {
		t.Fatalf("expected 1 cleaned file, got %d", len(cleaned))
	}
	if cleaned[0] != path {
		t.Errorf("expected cleaned path %q, got %q", path, cleaned[0])
	}

	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "## Test Results") {
		t.Errorf("Test Results section was not removed")
	}
	if strings.Contains(string(data), "## UAT Results") {
		t.Errorf("UAT Results section was not removed")
	}
}

func TestCleanTestResultsPreservesFilledUATInTestReady(t *testing.T) {
	dir := t.TempDir()
	readyDir := filepath.Join(dir, "test-ready")
	os.MkdirAll(readyDir, 0755)

	content := "# 01 - Test issue\n\n## What to build\n\nSomething\n\n## UAT Results\n\nUAT verified\n"
	path := filepath.Join(readyDir, "issue.md")
	os.WriteFile(path, []byte(content), 0644)

	cleaned, err := CleanTestResults(dir)
	if err != nil {
		t.Fatalf("CleanTestResults failed: %v", err)
	}
	if len(cleaned) != 0 {
		t.Fatalf("expected 0 cleaned files (filled UAT preserved), got %d", len(cleaned))
	}

	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "## UAT Results") {
		t.Errorf("filled UAT Results section was incorrectly removed from test-ready file")
	}
}

func TestCleanTestResultsStripsEmptyUATFromTestReady(t *testing.T) {
	dir := t.TempDir()
	readyDir := filepath.Join(dir, "test-ready")
	os.MkdirAll(readyDir, 0755)

	content := "# 01 - Test issue\n\n## What to build\n\nSomething\n\n## UAT Results\n"
	path := filepath.Join(readyDir, "issue.md")
	os.WriteFile(path, []byte(content), 0644)

	cleaned, err := CleanTestResults(dir)
	if err != nil {
		t.Fatalf("CleanTestResults failed: %v", err)
	}
	if len(cleaned) != 1 {
		t.Fatalf("expected 1 cleaned file, got %d", len(cleaned))
	}
	if cleaned[0] != path {
		t.Errorf("expected cleaned path %q, got %q", path, cleaned[0])
	}

	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "## UAT Results") {
		t.Errorf("empty UAT Results section was not removed from test-ready file")
	}
}

func TestCleanTestResultsKeepsTestResultsInTestReady(t *testing.T) {
	dir := t.TempDir()
	readyDir := filepath.Join(dir, "test-ready")
	os.MkdirAll(readyDir, 0755)

	content := "# 01 - Implement login\n\n## What to build\n\nLogin page\n\n## Test Results\n\nAll tests pass\n"
	path := filepath.Join(readyDir, "issue.md")
	os.WriteFile(path, []byte(content), 0644)

	cleaned, err := CleanTestResults(dir)
	if err != nil {
		t.Fatalf("CleanTestResults failed: %v", err)
	}
	if len(cleaned) != 0 {
		t.Fatalf("expected 0 cleaned files (Test Results valid in test-ready), got %d", len(cleaned))
	}

	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "## Test Results") {
		t.Errorf("Test Results section was incorrectly removed from test-ready file")
	}
}

func TestCleanTestResultsPreservesDone(t *testing.T) {
	dir := t.TempDir()
	doneDir := filepath.Join(dir, "done")
	os.MkdirAll(doneDir, 0755)

	content := "# 01 - Completed feature\n\n## What to build\n\nSomething\n\n## Test Results\n\nAll tests pass\n\n## UAT Results\n\nUAT verified\n"
	path := filepath.Join(doneDir, "issue.md")
	os.WriteFile(path, []byte(content), 0644)

	cleaned, err := CleanTestResults(dir)
	if err != nil {
		t.Fatalf("CleanTestResults failed: %v", err)
	}
	if len(cleaned) != 0 {
		t.Fatalf("expected 0 cleaned files (both sections valid in done), got %d", len(cleaned))
	}

	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "## Test Results") {
		t.Errorf("Test Results section was incorrectly removed from done file")
	}
	if !strings.Contains(string(data), "## UAT Results") {
		t.Errorf("UAT Results section was incorrectly removed from done file")
	}
}

func TestCleanTestResultsStripsUATResults(t *testing.T) {
	dir := t.TempDir()
	content := "# 01 - UAT issue\n\n## What to build\n\nSomething\n\n## UAT Results\n\nUAT verified\n"
	path := filepath.Join(dir, "uat-issue.md")
	os.WriteFile(path, []byte(content), 0644)

	cleaned, err := CleanTestResults(dir)
	if err != nil {
		t.Fatalf("CleanTestResults failed: %v", err)
	}
	if len(cleaned) != 1 {
		t.Fatalf("expected 1 cleaned file, got %d", len(cleaned))
	}
	if cleaned[0] != path {
		t.Errorf("expected cleaned path %q, got %q", path, cleaned[0])
	}

	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "## UAT Results") {
		t.Errorf("UAT Results section was not removed")
	}
}

func TestCleanTestResultsNoFiles(t *testing.T) {
	dir := t.TempDir()
	content := "# 01 - Implement login\n\n## What to build\n\nLogin page\n"
	path := filepath.Join(dir, "issue-login.md")
	os.WriteFile(path, []byte(content), 0644)

	cleaned, err := CleanTestResults(dir)
	if err != nil {
		t.Fatalf("CleanTestResults failed: %v", err)
	}
	if len(cleaned) != 0 {
		t.Errorf("expected 0 cleaned files, got %d", len(cleaned))
	}
}

func TestCleanTestResultsMultipleStates(t *testing.T) {
	dir := t.TempDir()
	testReadyDir := filepath.Join(dir, "test-ready")
	os.MkdirAll(testReadyDir, 0755)

	// File in todo (root) with Test Results
	todoPath := filepath.Join(dir, "issue-a.md")
	os.WriteFile(todoPath, []byte("# A\n\n## Test Results\n\nData\n"), 0644)

	// File in test-ready without Test Results
	readyPath := filepath.Join(testReadyDir, "issue-b.md")
	os.WriteFile(readyPath, []byte("# B\n\n## UAT Plan\n\nSteps\n"), 0644)

	cleaned, err := CleanTestResults(dir)
	if err != nil {
		t.Fatalf("CleanTestResults failed: %v", err)
	}
	if len(cleaned) != 1 {
		t.Fatalf("expected 1 cleaned file, got %d", len(cleaned))
	}
	if cleaned[0] != todoPath {
		t.Errorf("expected cleaned path %q, got %q", todoPath, cleaned[0])
	}
}

func TestCleanTestResultsEmptyDir(t *testing.T) {
	dir := t.TempDir()
	cleaned, err := CleanTestResults(dir)
	if err != nil {
		t.Fatalf("CleanTestResults failed: %v", err)
	}
	if len(cleaned) != 0 {
		t.Errorf("expected 0 cleaned files, got %d", len(cleaned))
	}
}

func TestCleanTestResultsNonExistentDir(t *testing.T) {
	_, err := CleanTestResults("/nonexistent/path")
	if err != nil {
		t.Fatalf("CleanTestResults failed: %v", err)
	}
}

func TestStripEmptySection(t *testing.T) {
	tests := []struct {
		name    string
		content string
		section string
		want    string
	}{
		{
			name:    "empty section at end",
			content: "# Title\n\n## UAT Results\n",
			section: "UAT Results",
			want:    "# Title\n",
		},
		{
			name:    "empty section between other sections",
			content: "# Title\n\n## Plan\n\nDo stuff\n\n## UAT Results\n\n## Comments\n\nDone\n",
			section: "UAT Results",
			want:    "# Title\n\n## Plan\n\nDo stuff\n\n## Comments\n\nDone\n",
		},
		{
			name:    "section with content preserved",
			content: "# Title\n\n## UAT Results\n\nStep 1: pass\n\n## Comments\n\nDone\n",
			section: "UAT Results",
			want:    "# Title\n\n## UAT Results\n\nStep 1: pass\n\n## Comments\n\nDone\n",
		},
		{
			name:    "section with only whitespace stripped",
			content: "# Title\n\n## UAT Results\n  \n\t\n\n## Other\n\nContent\n",
			section: "UAT Results",
			want:    "# Title\n\n## Other\n\nContent\n",
		},
		{
			name:    "non-matching section left alone",
			content: "# Title\n\n## Test Results\n\nData\n",
			section: "UAT Results",
			want:    "# Title\n\n## Test Results\n\nData\n",
		},
		{
			name:    "empty section at very end with trailing newline",
			content: "# Title\n\n## A\n\nContent\n\n## UAT Results\n",
			section: "UAT Results",
			want:    "# Title\n\n## A\n\nContent\n",
		},
		{
			name:    "no section present",
			content: "# Title\n\n## Other\n\nContent\n",
			section: "UAT Results",
			want:    "# Title\n\n## Other\n\nContent\n",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := stripEmptySection(tc.content, tc.section)
			if got != tc.want {
				t.Errorf("stripEmptySection:\ngot:  %q\nwant: %q", got, tc.want)
			}
		})
	}
}

func TestStripEmptyUATPlaceholdersStripsEmpty(t *testing.T) {
	dir := t.TempDir()
	readyDir := filepath.Join(dir, "test-ready")
	os.MkdirAll(readyDir, 0755)

	content := "# 01 - Test issue\n\n## What to build\n\nSomething\n\n## UAT Results\n"
	path := filepath.Join(readyDir, "issue.md")
	os.WriteFile(path, []byte(content), 0644)

	cleaned, err := StripEmptyUATPlaceholders(dir)
	if err != nil {
		t.Fatalf("StripEmptyUATPlaceholders failed: %v", err)
	}
	if len(cleaned) != 1 {
		t.Fatalf("expected 1 cleaned file, got %d", len(cleaned))
	}
	if cleaned[0] != path {
		t.Errorf("expected cleaned path %q, got %q", path, cleaned[0])
	}

	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "UAT Results") {
		t.Errorf("UAT Results section was not removed")
	}
}

func TestStripEmptyUATPlaceholdersPreservesFilled(t *testing.T) {
	dir := t.TempDir()
	readyDir := filepath.Join(dir, "test-ready")
	os.MkdirAll(readyDir, 0755)

	content := "# 01 - Test issue\n\n## UAT Results\n\n| Step | Result |\n| --- | --- |\n| Login | Pass |\n"
	path := filepath.Join(readyDir, "issue.md")
	os.WriteFile(path, []byte(content), 0644)

	cleaned, err := StripEmptyUATPlaceholders(dir)
	if err != nil {
		t.Fatalf("StripEmptyUATPlaceholders failed: %v", err)
	}
	if len(cleaned) != 0 {
		t.Errorf("expected 0 cleaned files (filled section), got %d", len(cleaned))
	}
}

func TestStripEmptyUATPlaceholdersSkipsNonTestReady(t *testing.T) {
	dir := t.TempDir()

	// File in todo (root) with empty UAT Results
	content := "# 01 - Todo issue\n\n## UAT Results\n"
	path := filepath.Join(dir, "todo-issue.md")
	os.WriteFile(path, []byte(content), 0644)

	cleaned, err := StripEmptyUATPlaceholders(dir)
	if err != nil {
		t.Fatalf("StripEmptyUATPlaceholders failed: %v", err)
	}
	if len(cleaned) != 0 {
		t.Errorf("expected 0 cleaned files (not in test-ready), got %d", len(cleaned))
	}
}

func TestStripEmptyUATPlaceholdersEmptyDir(t *testing.T) {
	dir := t.TempDir()
	cleaned, err := StripEmptyUATPlaceholders(dir)
	if err != nil {
		t.Fatalf("StripEmptyUATPlaceholders failed: %v", err)
	}
	if len(cleaned) != 0 {
		t.Errorf("expected 0 cleaned files, got %d", len(cleaned))
	}
}

func TestStripEmptyUATPlaceholdersNonExistentDir(t *testing.T) {
	cleaned, err := StripEmptyUATPlaceholders("/nonexistent/path")
	if err != nil {
		t.Fatalf("StripEmptyUATPlaceholders failed: %v", err)
	}
	if len(cleaned) != 0 {
		t.Errorf("expected 0 cleaned files, got %d", len(cleaned))
	}
}

func TestPreFlightCheckMissingExecMode(t *testing.T) {
	dir := t.TempDir()
	setupIssueFile(t, dir, "", "task.md", "# Task\n\nGitHub: #1\n")

	state := &PipelineState{
		TodoFiles: []string{filepath.Join(dir, "task.md")},
	}

	issues := PreFlightCheck(state, false, true)
	var modeIssues []PreFlightIssue
	for _, i := range issues {
		if strings.Contains(i.Message, "Execution mode") {
			modeIssues = append(modeIssues, i)
		}
	}
	if len(modeIssues) != 1 {
		t.Fatalf("expected 1 Execution mode issue, got %d", len(modeIssues))
	}
	if modeIssues[0].Severity != SeverityWarning {
		t.Errorf("expected severity %q, got %q", SeverityWarning, modeIssues[0].Severity)
	}
}

func TestPreFlightCheckMissingExecModeMultipleStates(t *testing.T) {
	dir := t.TempDir()
	setupIssueFile(t, dir, "", "todo.md", "# Todo\n\nGitHub: #1\n")
	setupIssueFile(t, dir, "test-ready", "ready.md", "# Ready\n\nGitHub: #2\n")
	setupIssueFile(t, dir, "done", "done.md", "# Done\n\nGitHub: #3\n")

	state := &PipelineState{
		TodoFiles:      []string{filepath.Join(dir, "todo.md")},
		TestReadyFiles: []string{filepath.Join(dir, "test-ready", "ready.md")},
		DoneFiles:      []string{filepath.Join(dir, "done", "done.md")},
	}

	issues := PreFlightCheck(state, false, true)
	var modeIssues []PreFlightIssue
	for _, i := range issues {
		if strings.Contains(i.Message, "Execution mode") {
			modeIssues = append(modeIssues, i)
		}
	}
	if len(modeIssues) != 3 {
		t.Fatalf("expected 3 Execution mode issues, got %d", len(modeIssues))
	}
}

func TestPreFlightCheckExecModePresent(t *testing.T) {
	dir := t.TempDir()
	setupIssueFile(t, dir, "", "todo.md", "# Todo\n\nGitHub: #1\nExecution mode: AFK-only\n")

	state := &PipelineState{
		TodoFiles: []string{filepath.Join(dir, "todo.md")},
	}

	issues := PreFlightCheck(state, false, true)
	for _, i := range issues {
		if strings.Contains(i.Message, "Execution mode") {
			t.Errorf("unexpected Execution mode issue: %s", i.Message)
		}
	}
}

func TestPreFlightCheckInvalidExecMode(t *testing.T) {
	dir := t.TempDir()
	setupIssueFile(t, dir, "", "task.md", "# Task\n\nGitHub: #1\nExecution mode: garbage\n")

	state := &PipelineState{
		TodoFiles: []string{filepath.Join(dir, "task.md")},
	}

	issues := PreFlightCheck(state, false, true)
	var modeIssues []PreFlightIssue
	for _, i := range issues {
		if strings.Contains(i.Message, "Execution mode") {
			modeIssues = append(modeIssues, i)
		}
	}
	if len(modeIssues) != 1 {
		t.Fatalf("expected 1 Execution mode issue, got %d", len(modeIssues))
	}
	if !strings.Contains(modeIssues[0].Message, "invalid") {
		t.Errorf("expected 'invalid' in message, got %q", modeIssues[0].Message)
	}
	if !strings.Contains(modeIssues[0].Message, "garbage") {
		t.Errorf("expected message to mention value %q, got %q", "garbage", modeIssues[0].Message)
	}
	if modeIssues[0].Severity != SeverityWarning {
		t.Errorf("expected severity %q, got %q", SeverityWarning, modeIssues[0].Severity)
	}
}

func TestPreFlightCheckInvalidExecModeMultipleFiles(t *testing.T) {
	dir := t.TempDir()
	setupIssueFile(t, dir, "", "bad1.md", "# Bad One\n\nGitHub: #1\nExecution mode: wat\n")
	setupIssueFile(t, dir, "test-ready", "bad2.md", "# Bad Two\n\nGitHub: #2\nExecution mode: unknown\n")

	state := &PipelineState{
		TodoFiles:      []string{filepath.Join(dir, "bad1.md")},
		TestReadyFiles: []string{filepath.Join(dir, "test-ready", "bad2.md")},
	}

	issues := PreFlightCheck(state, false, true)
	var modeIssues []PreFlightIssue
	for _, i := range issues {
		if strings.Contains(i.Message, "Execution mode") {
			modeIssues = append(modeIssues, i)
		}
	}
	if len(modeIssues) != 2 {
		t.Fatalf("expected 2 Execution mode issues, got %d", len(modeIssues))
	}
	for _, mi := range modeIssues {
		if !strings.Contains(mi.Message, "invalid") {
			t.Errorf("expected 'invalid' in message, got %q", mi.Message)
		}
	}
}

func TestPreFlightCheckValidExecModes(t *testing.T) {
	dir := t.TempDir()
	setupIssueFile(t, dir, "", "afk.md", "# AFK\n\nGitHub: #1\nExecution mode: AFK-only\n")
	setupIssueFile(t, dir, "test-ready", "hitl.md", "# HITL\n\nGitHub: #2\nExecution mode: HITL-only\n")
	setupIssueFile(t, dir, "done", "combo.md", "# Combo\n\nGitHub: #3\nExecution mode: Combo\n")

	state := &PipelineState{
		TodoFiles:      []string{filepath.Join(dir, "afk.md")},
		TestReadyFiles: []string{filepath.Join(dir, "test-ready", "hitl.md")},
		DoneFiles:      []string{filepath.Join(dir, "done", "combo.md")},
	}

	issues := PreFlightCheck(state, false, true)
	for _, i := range issues {
		if strings.Contains(i.Message, "Execution mode") {
			t.Errorf("unexpected Execution mode issue for valid value: %s", i.Message)
		}
	}
}

func TestIsValidExecMode(t *testing.T) {
	tests := []struct {
		mode string
		want bool
	}{
		{ExecModeAFKOnly, true},
		{ExecModeHITLOnly, true},
		{ExecModeCombo, true},
		{"", false},
		{"garbage", false},
		{"AFK", false},
		{"auto", false},
	}
	for _, tc := range tests {
		got := IsValidExecMode(tc.mode)
		if got != tc.want {
			t.Errorf("IsValidExecMode(%q) = %v, want %v", tc.mode, got, tc.want)
		}
	}
}

func TestValidExecModes(t *testing.T) {
	modes := ValidExecModes()
	if len(modes) != 3 {
		t.Fatalf("expected 3 valid modes, got %d: %v", len(modes), modes)
	}
	seen := make(map[string]bool)
	for _, m := range modes {
		if seen[m] {
			t.Errorf("duplicate mode %q", m)
		}
		seen[m] = true
		if !IsValidExecMode(m) {
			t.Errorf("ValidExecModes() includes %q but IsValidExecMode returns false", m)
		}
	}
}

func TestPreFlightCheckMissingExecModeSomeFiles(t *testing.T) {
	dir := t.TempDir()
	setupIssueFile(t, dir, "", "with-mode.md", "# With Mode\n\nGitHub: #1\nExecution mode: AFK-only\n")
	setupIssueFile(t, dir, "test-ready", "without-mode.md", "# Without Mode\n\nGitHub: #2\n")

	state := &PipelineState{
		TodoFiles:      []string{filepath.Join(dir, "with-mode.md")},
		TestReadyFiles: []string{filepath.Join(dir, "test-ready", "without-mode.md")},
	}

	issues := PreFlightCheck(state, false, true)
	var modeIssues []PreFlightIssue
	for _, i := range issues {
		if strings.Contains(i.Message, "Execution mode") {
			modeIssues = append(modeIssues, i)
		}
	}
	if len(modeIssues) != 1 {
		t.Fatalf("expected 1 Execution mode issue, got %d", len(modeIssues))
	}
	if !strings.Contains(modeIssues[0].FilePath, "without-mode.md") {
		t.Errorf("expected issue for 'without-mode.md', got %s", modeIssues[0].FilePath)
	}
}

func TestValidateRolePromise(t *testing.T) {
	tests := []struct {
		name    string
		role    Role
		promise agent.Promise
		wantErr bool
	}{
		{"implement complete", RoleImplement, agent.Complete, false},
		{"implement no more tasks", RoleImplement, agent.NoMoreTasks, false},
		{"implement test pass rejected", RoleImplement, agent.TestPass, true},
		{"implement test fail rejected", RoleImplement, agent.TestFail, true},
		{"test test pass", RoleTest, agent.TestPass, false},
		{"test test fail", RoleTest, agent.TestFail, false},
		{"test no more tasks", RoleTest, agent.NoMoreTasks, false},
		{"test complete rejected", RoleTest, agent.Complete, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateRolePromise(tc.role, tc.promise)
			if tc.wantErr && err == nil {
				t.Errorf("validateRolePromise(%q, %q) expected error", tc.role, tc.promise)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("validateRolePromise(%q, %q) unexpected error: %v", tc.role, tc.promise, err)
			}
		})
	}
}

func TestIsBlockerResolved(t *testing.T) {
	tests := []struct {
		name      string
		ref       string
		doneBases []string
		want      bool
	}{
		{"number prefix match", "42", []string{"42-feature", "43-bugfix"}, true},
		{"number with description", "42 — Feature name", []string{"42-feature"}, true},
		{"with md extension", "42.md", []string{"42"}, true},
		{"hash prefix not parsed", "#42", []string{"42-feature"}, false},
		{"unresolved number", "99 — Missing", []string{"42-feature"}, false},
		{"no number in ref", "text without number", []string{"42-feature"}, false},
		{"empty ref", "", []string{"42-feature"}, false},
		{"number not starting match", "42", []string{"142-other"}, false},
		{"empty done bases", "42", []string{}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isBlockerResolved(tc.ref, tc.doneBases)
			if got != tc.want {
				t.Errorf("isBlockerResolved(%q, %v) = %v, want %v", tc.ref, tc.doneBases, got, tc.want)
			}
		})
	}
}

func TestAllBlockersResolved(t *testing.T) {
	doneBases := []string{"01-done", "02-done", "03-done"}
	tests := []struct {
		name     string
		blockers []string
		want     bool
	}{
		{"all resolved", []string{"01", "02"}, true},
		{"single resolved", []string{"03"}, true},
		{"empty blockers", []string{}, true},
		{"nil blockers", nil, true},
		{"one unresolved", []string{"01", "99"}, false},
		{"all unresolved", []string{"99", "100"}, false},
		{"mix resolved and unresolved", []string{"02", "99", "03"}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := allBlockersResolved(tc.blockers, doneBases)
			if got != tc.want {
				t.Errorf("allBlockersResolved(%v, %v) = %v, want %v", tc.blockers, doneBases, got, tc.want)
			}
		})
	}
}

func TestIssueHeaderFields(t *testing.T) {
	h := IssueHeader{
		Title:     "Add login feature",
		GitHubNum: 42,
		ExecMode:  "AFK-only",
		Branch:    "feat/login",
	}
	if h.Title != "Add login feature" {
		t.Errorf("Title = %q, want %q", h.Title, "Add login feature")
	}
	if h.GitHubNum != 42 {
		t.Errorf("GitHubNum = %d, want %d", h.GitHubNum, 42)
	}
	if h.ExecMode != "AFK-only" {
		t.Errorf("ExecMode = %q, want %q", h.ExecMode, "AFK-only")
	}
	if h.Branch != "feat/login" {
		t.Errorf("Branch = %q, want %q", h.Branch, "feat/login")
	}
}

func TestIssueHeaderZeroValues(t *testing.T) {
	var h IssueHeader
	if h.Title != "" {
		t.Errorf("expected empty Title, got %q", h.Title)
	}
	if h.GitHubNum != 0 {
		t.Errorf("expected 0 GitHubNum, got %d", h.GitHubNum)
	}
	if h.ExecMode != "" {
		t.Errorf("expected empty ExecMode, got %q", h.ExecMode)
	}
	if h.Branch != "" {
		t.Errorf("expected empty Branch, got %q", h.Branch)
	}
}

func TestParseIssueHeader(t *testing.T) {
	content := "# 02 - Equity curve drawdown calculation\n\nGitHub: #14\nStatus: ready-for-agent\nExecution mode: AFK-only\nBranch: main\n\nSome body content.\n"
	h, err := ParseIssueHeader(content)
	if err != nil {
		t.Fatalf("ParseIssueHeader() returned error: %v", err)
	}
	if h.Title != "02 - Equity curve drawdown calculation" {
		t.Errorf("Title = %q, want %q", h.Title, "02 - Equity curve drawdown calculation")
	}
	if h.GitHubNum != 14 {
		t.Errorf("GitHubNum = %d, want %d", h.GitHubNum, 14)
	}
	if h.ExecMode != "AFK-only" {
		t.Errorf("ExecMode = %q, want %q", h.ExecMode, "AFK-only")
	}
	if h.Branch != "main" {
		t.Errorf("Branch = %q, want %q", h.Branch, "main")
	}
}

func TestParseIssueHeaderOnlyTitle(t *testing.T) {
	content := "# Just a title\n\nSome body.\n"
	h, err := ParseIssueHeader(content)
	if err != nil {
		t.Fatalf("ParseIssueHeader() returned error: %v", err)
	}
	if h.Title != "Just a title" {
		t.Errorf("Title = %q, want %q", h.Title, "Just a title")
	}
	if h.GitHubNum != 0 {
		t.Errorf("GitHubNum = %d, want 0", h.GitHubNum)
	}
	if h.ExecMode != "" {
		t.Errorf("ExecMode = %q, want empty", h.ExecMode)
	}
	if h.Branch != "" {
		t.Errorf("Branch = %q, want empty", h.Branch)
	}
}

func TestParseIssueHeaderEmptyContent(t *testing.T) {
	h, err := ParseIssueHeader("")
	if err != nil {
		t.Fatalf("ParseIssueHeader() returned error: %v", err)
	}
	if h.Title != "" {
		t.Errorf("Title = %q, want empty", h.Title)
	}
}

func TestParseIssueHeaderExtraWhitespace(t *testing.T) {
	content := "#  Title with space  \n\nGitHub: #  42  \nStatus:   ready  \n"
	h, err := ParseIssueHeader(content)
	if err != nil {
		t.Fatalf("ParseIssueHeader() returned error: %v", err)
	}
	if h.Title != "Title with space" {
		t.Errorf("Title = %q, want %q", h.Title, "Title with space")
	}
	if h.GitHubNum != 42 {
		t.Errorf("GitHubNum = %d, want %d", h.GitHubNum, 42)
	}
}

func TestParseIssueHeaderLowcasePrefix(t *testing.T) {
	content := "# Test\n\ngithub: #14\nstatus: ready\n"
	h, err := ParseIssueHeader(content)
	if err != nil {
		t.Fatalf("ParseIssueHeader() returned error: %v", err)
	}
	if h.GitHubNum != 0 {
		t.Errorf("GitHubNum = %d, want 0", h.GitHubNum)
	}
}

func TestParseIssueHeaderGitHubHashWithoutNumber(t *testing.T) {
	content := "# Test\n\nGitHub: #\n"
	h, err := ParseIssueHeader(content)
	if err != nil {
		t.Fatalf("ParseIssueHeader() returned error: %v", err)
	}
	if h.GitHubNum != 0 {
		t.Errorf("GitHubNum = %d, want 0", h.GitHubNum)
	}
}

func TestParseIssueHeaderMultipleValues(t *testing.T) {
	content := "# Test\n\nStatus: first\nStatus: second\nGitHub: #1\nGitHub: #99\n"
	h, err := ParseIssueHeader(content)
	if err != nil {
		t.Fatalf("ParseIssueHeader() returned error: %v", err)
	}
	if h.GitHubNum != 99 {
		t.Errorf("GitHubNum = %d, want %d", h.GitHubNum, 99)
	}
}

func TestParseIssueHeaderFirstTitleOnly(t *testing.T) {
	content := "# First title\n\nSome text\n\n# Second title\n"
	h, err := ParseIssueHeader(content)
	if err != nil {
		t.Fatalf("ParseIssueHeader() returned error: %v", err)
	}
	if h.Title != "First title" {
		t.Errorf("Title = %q, want %q", h.Title, "First title")
	}
}

func TestParseIssueHeaderFieldsInAnyOrder(t *testing.T) {
	content := "# Test\n\nBranch: develop\nExecution mode: HITL-only\nStatus: blocked\nGitHub: #7\n"
	h, err := ParseIssueHeader(content)
	if err != nil {
		t.Fatalf("ParseIssueHeader() returned error: %v", err)
	}
	if h.GitHubNum != 7 {
		t.Errorf("GitHubNum = %d, want %d", h.GitHubNum, 7)
	}
	if h.ExecMode != "HITL-only" {
		t.Errorf("ExecMode = %q, want %q", h.ExecMode, "HITL-only")
	}
	if h.Branch != "develop" {
		t.Errorf("Branch = %q, want %q", h.Branch, "develop")
	}
}

func TestExtractSection(t *testing.T) {
	tests := []struct {
		name    string
		content string
		section string
		want    string
	}{
		{
			name:    "section present with content",
			content: "## Comments\n\nThis is a comment.\n",
			section: "Comments",
			want:    "This is a comment.",
		},
		{
			name:    "section not present",
			content: "## Other\n\nContent\n",
			section: "Comments",
			want:    "",
		},
		{
			name:    "stops at next heading",
			content: "## What to build\n\nFeature X\n\n## Comments\n\nNothing\n",
			section: "What to build",
			want:    "Feature X",
		},
		{
			name:    "empty content",
			content: "",
			section: "Comments",
			want:    "",
		},
		{
			name:    "case insensitive match",
			content: "## WHAT TO BUILD\n\nFeature Y\n",
			section: "What to build",
			want:    "Feature Y",
		},
		{
			name:    "multi-line content preserved",
			content: "## UAT plan\n- Step 1\n- Step 2\n- Step 3\n",
			section: "UAT plan",
			want:    "- Step 1\n- Step 2\n- Step 3",
		},
		{
			name:    "section with blank lines preserved",
			content: "## Parent\n\nRef: #42\n\nContext here.\n",
			section: "Parent",
			want:    "Ref: #42\n\nContext here.",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := extractSection(tc.content, tc.section)
			if got != tc.want {
				t.Errorf("extractSection:\ngot:  %q\nwant: %q", got, tc.want)
			}
		})
	}
}

func TestParseIssueBody(t *testing.T) {
	content := "# Issue Title\n\n" +
		"## Parent\n\nRef: Epic #42\n\n" +
		"## What to build\n\nA new feature\n\n" +
		"## User stories covered\n\n- Story 1\n- Story 2\n\n" +
		"## Acceptance criteria\n\n- AC 1\n- AC 2\n\n" +
		"## UAT plan\n\n- Step 1\n- Step 2\n\n" +
		"## Blocked by\n\n- 03 — Core iteration\n\n" +
		"## Comments\n\nSome notes\n"

	body := ParseIssueBody(content)
	if body == nil {
		t.Fatal("ParseIssueBody returned nil")
	}

	want := IssueBody{
		Parent:             "Ref: Epic #42",
		WhatToBuild:        "A new feature",
		UserStoriesCovered: "- Story 1\n- Story 2",
		AcceptanceCriteria: "- AC 1\n- AC 2",
		UATPlan:            "- Step 1\n- Step 2",
		BlockedBy:          "- 03 — Core iteration",
		Comments:           "Some notes",
	}
	if *body != want {
		t.Errorf("ParseIssueBody:\ngot:  %+v\nwant: %+v", *body, want)
	}
}

func TestParseIssueBodyPartialSections(t *testing.T) {
	content := "# Title\n\n" +
		"## What to build\n\nFeature X\n\n" +
		"## Comments\n\nLooks good\n"

	body := ParseIssueBody(content)
	if body == nil {
		t.Fatal("ParseIssueBody returned nil")
	}

	if body.WhatToBuild != "Feature X" {
		t.Errorf("WhatToBuild = %q, want %q", body.WhatToBuild, "Feature X")
	}
	if body.Comments != "Looks good" {
		t.Errorf("Comments = %q, want %q", body.Comments, "Looks good")
	}
	if body.Parent != "" {
		t.Errorf("Parent = %q, want empty", body.Parent)
	}
	if body.UATPlan != "" {
		t.Errorf("UATPlan = %q, want empty", body.UATPlan)
	}
}

func TestParseIssueBodyEmptyContent(t *testing.T) {
	body := ParseIssueBody("")
	if body == nil {
		t.Fatal("ParseIssueBody returned nil")
	}
	if *body != (IssueBody{}) {
		t.Errorf("ParseIssueBody(\"\") = %+v, want zero value", *body)
	}
}

func TestParseIssueBodyNoSections(t *testing.T) {
	body := ParseIssueBody("# Just a title\n\nSome body text with no sections.\n")
	if body == nil {
		t.Fatal("ParseIssueBody returned nil")
	}
	if *body != (IssueBody{}) {
		t.Errorf("ParseIssueBody = %+v, want zero value", *body)
	}
}

func TestRequiredSectionsHaveExpectedItems(t *testing.T) {
	if len(RequiredSections) != 5 {
		t.Fatalf("RequiredSections length = %d, want 5", len(RequiredSections))
	}

	want := []string{
		"What to build",
		"User stories covered",
		"Acceptance criteria",
		"UAT plan",
		"Blocked by",
	}
	if len(RequiredSections) != len(want) {
		t.Fatalf("RequiredSections = %v, want %v", RequiredSections, want)
	}
	for i := range want {
		if RequiredSections[i] != want[i] {
			t.Errorf("RequiredSections[%d] = %q, want %q", i, RequiredSections[i], want[i])
		}
	}
}

func TestDisallowedSectionsHaveExpectedEntries(t *testing.T) {
	todo, ok := DisallowedSections[StateTodo]
	if !ok {
		t.Fatal("DisallowedSections missing entry for StateTodo")
	}
	if len(todo) != 1 || todo[0] != "UAT Results" {
		t.Errorf("DisallowedSections[StateTodo] = %v, want [\"UAT Results\"]", todo)
	}

	testReady, ok := DisallowedSections[StateTestReady]
	if !ok {
		t.Fatal("DisallowedSections missing entry for StateTestReady")
	}
	if len(testReady) != 1 || testReady[0] != "UAT Results" {
		t.Errorf("DisallowedSections[StateTestReady] = %v, want [\"UAT Results\"]", testReady)
	}

	if _, ok := DisallowedSections[StateDone]; ok {
		t.Errorf("DisallowedSections should not have entry for StateDone")
	}
	if _, ok := DisallowedSections[StateQuarantine]; ok {
		t.Errorf("DisallowedSections should not have entry for StateQuarantine")
	}
}

func TestValidateSectionsAllPresent(t *testing.T) {
	content := `## Parent

Ref: Epic #1

## What to build

A feature

## User stories covered

US1, US2

## Acceptance criteria

- AC1

## UAT plan

| Step | Description | Output | Expected | Result |
|------|-------------|--------|----------|--------|

## Blocked by

None
`
	issues := ValidateSections(content, StateTodo)
	if len(issues) != 0 {
		t.Errorf("expected 0 issues for complete content, got %d: %v", len(issues), issues)
	}
}

func TestValidateSectionsMissingRequired(t *testing.T) {
	content := `## What to build

A feature

## Comments

Some notes
`
	issues := ValidateSections(content, StateTodo)
	if len(issues) == 0 {
		t.Fatal("expected issues for missing required sections")
	}
	found := make(map[string]bool)
	for _, iss := range issues {
		if iss.Severity != SeverityWarning {
			t.Errorf("expected severity %q, got %q", SeverityWarning, iss.Severity)
		}
		if iss.FilePath != "" {
			t.Errorf("expected empty FilePath, got %q", iss.FilePath)
		}
		found[iss.Message] = true
	}
	for _, req := range RequiredSections {
		if req == "What to build" {
			continue
		}
		if !found[fmt.Sprintf("missing required section %q", req)] {
			t.Errorf("expected issue for missing section %q", req)
		}
	}
	if found[fmt.Sprintf("missing required section %q", "What to build")] {
		t.Errorf("should not report 'What to build' as missing when it is present")
	}
}

func TestValidateSectionsDisallowedUATResults(t *testing.T) {
	content := `## Parent

Ref: Epic #1

## What to build

A feature

## User stories covered

US1

## Acceptance criteria

- AC1

## UAT plan

| Step | Description |

## UAT Results

| Step | Result |
| ---- | ------ |

## Blocked by

None
`
	issues := ValidateSections(content, StateTodo)
	var disallowedIssues []PreFlightIssue
	for _, iss := range issues {
		if strings.Contains(iss.Message, "not allowed in state") {
			disallowedIssues = append(disallowedIssues, iss)
		}
	}
	if len(disallowedIssues) != 1 {
		t.Fatalf("expected 1 disallowed section issue, got %d: %v", len(disallowedIssues), issues)
	}
	if !strings.Contains(disallowedIssues[0].Message, "UAT Results") {
		t.Errorf("expected message about UAT Results, got %q", disallowedIssues[0].Message)
	}
	if !strings.Contains(disallowedIssues[0].Message, "todo") {
		t.Errorf("expected message to mention state, got %q", disallowedIssues[0].Message)
	}
}

func TestValidateSectionsAllowsUATResultsInDone(t *testing.T) {
	content := `## Parent

Ref: Epic #1

## What to build

A feature

## User stories covered

US1

## Acceptance criteria

- AC1

## UAT plan

| Step | Description |

## UAT Results

PASS

## Blocked by

None
`
	issues := ValidateSections(content, StateDone)
	var disallowedIssues []PreFlightIssue
	for _, iss := range issues {
		if strings.Contains(iss.Message, "not allowed in state") {
			disallowedIssues = append(disallowedIssues, iss)
		}
	}
	if len(disallowedIssues) != 0 {
		t.Errorf("expected 0 disallowed issues in done state, got %d: %v", len(disallowedIssues), disallowedIssues)
	}
}

func TestValidateSectionsEmptyContent(t *testing.T) {
	issues := ValidateSections("", StateTodo)
	if len(issues) != len(RequiredSections) {
		t.Errorf("expected %d issues for empty content (all required missing), got %d", len(RequiredSections), len(issues))
	}
}

func TestValidateSectionsDisallowedInTestReady(t *testing.T) {
	content := `## Parent

Ref: Epic #1

## What to build

A feature

## User stories covered

US1

## Acceptance criteria

- AC1

## UAT plan

| Step | Description |

## UAT Results

| Step | Result |
| ---- | ------ |

## Blocked by

None
`
	issues := ValidateSections(content, StateTestReady)
	var disallowedIssues []PreFlightIssue
	for _, iss := range issues {
		if strings.Contains(iss.Message, "not allowed in state") {
			disallowedIssues = append(disallowedIssues, iss)
		}
	}
	if len(disallowedIssues) != 1 {
		t.Fatalf("expected 1 disallowed section issue, got %d: %v", len(disallowedIssues), issues)
	}
	if !strings.Contains(disallowedIssues[0].Message, "UAT Results") {
		t.Errorf("expected message about UAT Results, got %q", disallowedIssues[0].Message)
	}
	if !strings.Contains(disallowedIssues[0].Message, "test-ready") {
		t.Errorf("expected message to mention state, got %q", disallowedIssues[0].Message)
	}
}

func TestPreFlightCheckSectionIssues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "issue.md")
	content := "# Test Issue\n\n## UAT Results\n\nPre-populated\n"
	os.WriteFile(path, []byte(content), 0644)

	state := &PipelineState{
		TodoFiles: []string{path},
	}

	issues := PreFlightCheck(state, false, true)
	var secIssues []PreFlightIssue
	for _, iss := range issues {
		if strings.Contains(iss.Message, "section") || strings.Contains(iss.Message, "required section") {
			secIssues = append(secIssues, iss)
		}
	}
	if len(secIssues) == 0 {
		t.Fatal("expected section validation issues in PreFlightCheck")
	}
}

func TestPreFlightCheckValidIssueSections(t *testing.T) {
	dir := t.TempDir()
	content := "# Valid Issue\n\nGitHub: #1\nExecution mode: AFK-only\n\n## Parent\n\nRef: Epic #1\n\n## What to build\n\nA feature\n\n## User stories covered\n\nUS1\n\n## Acceptance criteria\n\n- [ ] AC1\n- [x] AC2\n\n## UAT plan\n\n| Step | Description | Output | Expected | Result |\n|------|-------------|--------|----------|--------|\n| 1 | Test | Done | Works | PASS |\n\n## Blocked by\n\nNone\n"
	path := setupIssueFile(t, dir, "todo", "valid.md", content)

	state := &PipelineState{
		TodoFiles: []string{path},
	}

	issues := PreFlightCheck(state, false, true)
	for _, iss := range issues {
		if strings.Contains(iss.Message, "required section") || strings.Contains(iss.Message, "not allowed in state") {
			t.Errorf("unexpected section issue for valid file: %s", iss.Message)
		}
	}
}

func TestPreFlightCheckMissingSections(t *testing.T) {
	dir := t.TempDir()
	content := "# Missing Sections\n\nGitHub: #1\nExecution mode: AFK-only\n\n## What to build\n\nA feature\n\n## Acceptance criteria\n\n- [ ] AC1\n"
	path := setupIssueFile(t, dir, "todo", "missing.md", content)

	state := &PipelineState{
		TodoFiles: []string{path},
	}

	issues := PreFlightCheck(state, false, true)
	var secIssues []PreFlightIssue
	for _, iss := range issues {
		if strings.Contains(iss.Message, "required section") {
			secIssues = append(secIssues, iss)
		}
	}
	if len(secIssues) == 0 {
		t.Fatal("expected missing section issues in PreFlightCheck")
	}
	missing := make(map[string]bool)
	for _, iss := range secIssues {
		missing[iss.Message] = true
	}
	for _, req := range RequiredSections {
		if req == "What to build" || req == "Acceptance criteria" {
			continue
		}
		if !missing[fmt.Sprintf("missing required section %q", req)] {
			t.Errorf("expected missing section warning for %q", req)
		}
	}
}

func TestPreFlightCheckDisallowedSectionsTodo(t *testing.T) {
	dir := t.TempDir()
	content := "# Disallowed\n\nGitHub: #1\nExecution mode: AFK-only\n\n## Parent\n\nRef\n\n## What to build\n\nFeature\n\n## User stories covered\n\nUS1\n\n## Acceptance criteria\n\n- [ ] AC1\n\n## UAT plan\n\n| Step | Description | Output | Expected | Result |\n|------|-------------|--------|----------|--------|\n\n## UAT Results\n\nPASS\n\n## Blocked by\n\nNone\n"
	path := setupIssueFile(t, dir, "todo", "disallowed.md", content)

	state := &PipelineState{
		TodoFiles: []string{path},
	}

	issues := PreFlightCheck(state, false, true)
	var disallowedIssues []PreFlightIssue
	for _, iss := range issues {
		if strings.Contains(iss.Message, "not allowed in state") {
			disallowedIssues = append(disallowedIssues, iss)
		}
	}
	if len(disallowedIssues) != 1 {
		t.Fatalf("expected 1 disallowed section issue, got %d: %v", len(disallowedIssues), issues)
	}
	if !strings.Contains(disallowedIssues[0].Message, "UAT Results") {
		t.Errorf("expected message about UAT Results, got %q", disallowedIssues[0].Message)
	}
}

func TestDoneBasenames(t *testing.T) {
	tests := []struct {
		name  string
		files []string
		want  []string
	}{
		{"multiple files", []string{"/issues/done/42-feature.md", "/issues/done/43-bugfix.md"}, []string{"42-feature", "43-bugfix"}},
		{"single file", []string{"/issues/done/01-login.md"}, []string{"01-login"}},
		{"empty list", []string{}, []string{}},
		{"nil list", nil, []string{}},
		{"no extension", []string{"/issues/done/README"}, []string{"README"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := doneBasenames(tc.files)
			if len(got) != len(tc.want) {
				t.Fatalf("doneBasenames(%v) = %v (len %d), want %v (len %d)", tc.files, got, len(got), tc.want, len(tc.want))
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("doneBasenames(%v)[%d] = %q, want %q", tc.files, i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestValidateIssueFormatAllPresent(t *testing.T) {
	content := `## Parent

Ref: Epic #1

## What to build

A feature

## User stories covered

US1, US2

## Acceptance criteria

- AC1

## UAT plan

| Step | Description |

## Blocked by

None
`
	issues := ValidateIssueFormat(content)
	if len(issues) != 0 {
		t.Errorf("expected 0 issues for complete content, got %d: %v", len(issues), issues)
	}
}

func TestValidateIssueFormatMissingRequired(t *testing.T) {
	content := `## What to build

A feature

## Comments

Some notes
`
	issues := ValidateIssueFormat(content)
	if len(issues) == 0 {
		t.Fatal("expected issues for missing required sections")
	}
	found := make(map[string]bool)
	for _, msg := range issues {
		found[msg] = true
	}
	for _, req := range RequiredSections {
		if req == "What to build" {
			continue
		}
		if !found[fmt.Sprintf("missing required section %q", req)] {
			t.Errorf("expected issue for missing section %q", req)
		}
	}
	if found[fmt.Sprintf("missing required section %q", "What to build")] {
		t.Errorf("should not report 'What to build' as missing when it is present")
	}
}

func TestValidateIssueFormatExtraSection(t *testing.T) {
	content := `## Parent

Ref: Epic #1

## What to build

A feature

## User stories covered

US1

## Acceptance criteria

- AC1

## UAT plan

| Step | Description |

## Blocked by

None

## Spurious Section

This section should not be here
`
	issues := ValidateIssueFormat(content)
	var extra []string
	for _, msg := range issues {
		if strings.Contains(msg, "extra unrecognized") {
			extra = append(extra, msg)
		}
	}
	if len(extra) != 1 {
		t.Fatalf("expected 1 extra section issue, got %d: %v", len(extra), issues)
	}
	if !strings.Contains(extra[0], "spurious section") {
		t.Errorf("expected message about spurious section, got %q", extra[0])
	}
}

func TestValidateIssueFormatKnownOptionalSections(t *testing.T) {
	content := `## Parent

Ref

## What to build

Feature

## User stories covered

US1

## Acceptance criteria

- AC1

## UAT plan

Plan

## UAT Results

PASS

## UAT Process

How to report

## Defect Tracker

Link

## Blocked by

None

## Comments

Some notes
`
	issues := ValidateIssueFormat(content)
	var extra []string
	for _, msg := range issues {
		if strings.Contains(msg, "extra unrecognized") {
			extra = append(extra, msg)
		}
	}
	if len(extra) != 0 {
		t.Errorf("expected 0 extra section issues for known optional sections, got %d: %v", len(extra), issues)
	}
}

func TestValidateIssueFormatEmptyContent(t *testing.T) {
	issues := ValidateIssueFormat("")
	if len(issues) != len(RequiredSections) {
		t.Errorf("expected %d issues for empty content (all required missing), got %d", len(RequiredSections), len(issues))
	}
	for _, msg := range issues {
		if !strings.HasPrefix(msg, "missing required section") {
			t.Errorf("expected all issues to be about missing sections, got %q", msg)
		}
	}
}

func TestValidateIssueFormatMissingAndExtra(t *testing.T) {
	content := `## Parent

Ref

## Wildcard

Unexpected
`
	issues := ValidateIssueFormat(content)
	var missing, extra int
	for _, msg := range issues {
		if strings.Contains(msg, "missing required section") {
			missing++
		}
		if strings.Contains(msg, "extra unrecognized section") {
			extra++
		}
	}
	if missing != len(RequiredSections) {
		t.Errorf("expected %d missing required sections (all required), got %d", len(RequiredSections), missing)
	}
	if extra != 1 {
		t.Errorf("expected 1 extra section, got %d", extra)
	}
}

func TestValidateUATPlanValid(t *testing.T) {
	content := `## UAT plan

| Step | Description | Output | Expected | Result |
|------|-------------|--------|----------|--------|
| 1 | Run tests | 44/44 pass | All pass | PASS |
`
	if err := ValidateUATPlan(content); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestValidateUATPlanNoSection(t *testing.T) {
	content := `## What to build

A feature
`
	if err := ValidateUATPlan(content); err == nil {
		t.Fatal("expected error for missing section")
	}
}

func TestValidateUATPlanEmptySection(t *testing.T) {
	content := `## UAT plan
`
	if err := ValidateUATPlan(content); err == nil {
		t.Fatal("expected error for empty section")
	}
}

func TestValidateUATPlanNoTable(t *testing.T) {
	content := `## UAT plan

Some text without a table
`
	if err := ValidateUATPlan(content); err == nil {
		t.Fatal("expected error for no table")
	}
}

func TestValidateUATPlanWrongColumns(t *testing.T) {
	content := `## UAT plan

| Step | Description | Expected | Result |
|------|-------------|----------|--------|
| 1 | Test | Works | PASS |
`
	if err := ValidateUATPlan(content); err == nil {
		t.Fatal("expected error for wrong column count")
	}
}

func TestValidateUATPlanWrongColumnOrder(t *testing.T) {
	content := `## UAT plan

| Step | Description | Expected | Output | Result |
|------|-------------|----------|--------|--------|
| 1 | Test | Works | Done | PASS |
`
	if err := ValidateUATPlan(content); err == nil {
		t.Fatal("expected error for wrong column order")
	}
}

func TestValidateUATPlanMissingSeparator(t *testing.T) {
	content := `## UAT plan

| Step | Description | Output | Expected | Result |
| 1 | Test | Output | Expected | PASS |
`
	if err := ValidateUATPlan(content); err == nil {
		t.Fatal("expected error for missing separator row")
	}
}

func TestValidateUATPlanWrongSeparatorCols(t *testing.T) {
	content := `## UAT plan

| Step | Description | Output | Expected | Result |
|------|-------------|--------|----------|
| 1 | Test | Output | Expected | PASS |
`
	if err := ValidateUATPlan(content); err == nil {
		t.Fatal("expected error for wrong separator column count")
	}
}

func TestValidateUATPlanCaseInsensitive(t *testing.T) {
	content := `## UAT plan

| step | description | output | expected | result |
|------|-------------|--------|----------|--------|
| 1 | Run tests | Output | Expected | PASS |
`
	if err := ValidateUATPlan(content); err != nil {
		t.Errorf("expected no error for case-insensitive columns, got: %v", err)
	}
}

func TestValidateUATPlanExtraWhitespace(t *testing.T) {
	content := `## UAT plan

|  Step  |  Description  |  Output  |  Expected  |  Result  |
|--------|---------------|----------|------------|----------|
| 1 | Test | Done | Works | PASS |
`
	if err := ValidateUATPlan(content); err != nil {
		t.Errorf("expected no error with extra whitespace, got: %v", err)
	}
}

func TestValidateUATPlanWithLeadingText(t *testing.T) {
	content := `## UAT plan

Define test steps using the standard UAT table format:

| Step | Description | Output | Expected | Result |
|------|-------------|--------|----------|--------|
| 1 | Run tests | 44/44 pass | All pass | PASS |
`
	if err := ValidateUATPlan(content); err != nil {
		t.Errorf("expected no error with leading text, got: %v", err)
	}
}

func TestValidateUATPlanEmptyContent(t *testing.T) {
	if err := ValidateUATPlan(""); err == nil {
		t.Fatal("expected error for empty content")
	}
}

func TestValidateUATPlanWrongColumnNames(t *testing.T) {
	content := `## UAT plan

| Step | Foo | Bar | Expected | Result |
|------|-----|-----|----------|--------|
| 1 | Test | Done | Works | PASS |
`
	if err := ValidateUATPlan(content); err == nil {
		t.Fatal("expected error for wrong column names")
	}
}

func TestValidateAcceptanceCriteriaValid(t *testing.T) {
	content := `## Acceptance criteria

- [ ] Criterion 1
- [ ] Criterion 2
- [ ] Criterion 3
`
	if err := ValidateAcceptanceCriteria(content); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestValidateAcceptanceCriteriaWithChecked(t *testing.T) {
	content := `## Acceptance criteria

- [ ] Criterion 1
- [x] Completed criterion 2
- [ ] Criterion 3
`
	if err := ValidateAcceptanceCriteria(content); err != nil {
		t.Errorf("expected no error for mix of checked/unchecked, got: %v", err)
	}
}

func TestValidateAcceptanceCriteriaUppercaseX(t *testing.T) {
	content := `## Acceptance criteria

- [ ] Criterion 1
- [X] Completed criterion 2
`
	if err := ValidateAcceptanceCriteria(content); err != nil {
		t.Errorf("expected no error for uppercase [X], got: %v", err)
	}
}

func TestValidateAcceptanceCriteriaNoSection(t *testing.T) {
	content := `## What to build

A feature
`
	if err := ValidateAcceptanceCriteria(content); err == nil {
		t.Fatal("expected error for missing section")
	}
}

func TestValidateAcceptanceCriteriaEmptySection(t *testing.T) {
	content := `## Acceptance criteria
`
	if err := ValidateAcceptanceCriteria(content); err == nil {
		t.Fatal("expected error for empty section")
	}
}

func TestValidateAcceptanceCriteriaPlainBullets(t *testing.T) {
	content := `## Acceptance criteria

- Criterion 1
- Criterion 2
`
	if err := ValidateAcceptanceCriteria(content); err == nil {
		t.Fatal("expected error for plain bullet points")
	}
}

func TestValidateAcceptanceCriteriaNumberedList(t *testing.T) {
	content := `## Acceptance criteria

1. Criterion 1
2. Criterion 2
`
	if err := ValidateAcceptanceCriteria(content); err == nil {
		t.Fatal("expected error for numbered list")
	}
}

func TestValidateAcceptanceCriteriaBlankLines(t *testing.T) {
	content := `## Acceptance criteria

- [ ] Criterion 1

- [ ] Criterion 2

- [x] Criterion 3
`
	if err := ValidateAcceptanceCriteria(content); err != nil {
		t.Errorf("expected no error with blank lines, got: %v", err)
	}
}

func TestValidateAcceptanceCriteriaEmptyContent(t *testing.T) {
	if err := ValidateAcceptanceCriteria(""); err == nil {
		t.Fatal("expected error for empty content")
	}
}

func TestValidateAcceptanceCriteriaExtraSpaces(t *testing.T) {
	content := `## Acceptance criteria

-  [ ] Criterion 1
-  [x] Criterion 2
`
	if err := ValidateAcceptanceCriteria(content); err == nil {
		t.Fatal("expected error for extra spaces before bracket")
	}
}

func TestStageRank(t *testing.T) {
	tests := []struct {
		state State
		want  int
	}{
		{StateDone, 4},
		{StateTestReady, 3},
		{StateReadyForAgent, 2},
		{StateTodo, 1},
		{StateQuarantine, 0},
		{StateUnknown, 0},
	}
	for _, tc := range tests {
		got := stageRank(tc.state)
		if got != tc.want {
			t.Errorf("stageRank(%q) = %d, want %d", tc.state, got, tc.want)
		}
	}
}

func TestPickCanonicalSingleFile(t *testing.T) {
	dir := t.TempDir()
	path := setupIssueFile(t, dir, "", "issue.md", "# Single\n")
	result, err := PickCanonical([]string{path})
	if err != nil {
		t.Fatalf("PickCanonical failed: %v", err)
	}
	if result != path {
		t.Errorf("PickCanonical = %q, want %q", result, path)
	}
}

func TestPickCanonicalEmptyList(t *testing.T) {
	_, err := PickCanonical([]string{})
	if err == nil {
		t.Fatal("expected error for empty list")
	}
}

func TestPickCanonicalPrefersLaterStage(t *testing.T) {
	dir := t.TempDir()
	todoPath := setupIssueFile(t, dir, "", "dup.md", "# Todo\n")
	donePath := setupIssueFile(t, dir, "done", "dup.md", "# Done\n")

	result, err := PickCanonical([]string{todoPath, donePath})
	if err != nil {
		t.Fatalf("PickCanonical failed: %v", err)
	}
	if result != donePath {
		t.Errorf("PickCanonical = %q, want %q (done preferred over todo)", result, donePath)
	}
}

func TestPickCanonicalPrefersTestReadyOverTodo(t *testing.T) {
	dir := t.TempDir()
	todoPath := setupIssueFile(t, dir, "", "dup.md", "# Todo\n")
	readyPath := setupIssueFile(t, dir, "test-ready", "dup.md", "# Ready\n")

	result, err := PickCanonical([]string{todoPath, readyPath})
	if err != nil {
		t.Fatalf("PickCanonical failed: %v", err)
	}
	if result != readyPath {
		t.Errorf("PickCanonical = %q, want %q (test-ready preferred over todo)", result, readyPath)
	}
}

func TestPickCanonicalPrefersDoneOverTestReady(t *testing.T) {
	dir := t.TempDir()
	readyPath := setupIssueFile(t, dir, "test-ready", "dup.md", "# Ready\n")
	donePath := setupIssueFile(t, dir, "done", "dup.md", "# Done\n")

	result, err := PickCanonical([]string{readyPath, donePath})
	if err != nil {
		t.Fatalf("PickCanonical failed: %v", err)
	}
	if result != donePath {
		t.Errorf("PickCanonical = %q, want %q (done preferred over test-ready)", result, donePath)
	}
}

func TestPickCanonicalNewerWithinSameStage(t *testing.T) {
	dir := t.TempDir()

	oldPath := filepath.Join(dir, "old.md")
	os.WriteFile(oldPath, []byte("# Old\n"), 0644)
	// Set old mtime to the past.
	past := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	os.Chtimes(oldPath, past, past)

	newerPath := filepath.Join(dir, "newer.md")
	os.WriteFile(newerPath, []byte("# Newer\n"), 0644)

	// Both in todo stage, newer should win.
	result, err := PickCanonical([]string{oldPath, newerPath})
	if err != nil {
		t.Fatalf("PickCanonical failed: %v", err)
	}
	if result != newerPath {
		t.Errorf("PickCanonical = %q, want %q (newer file preferred within same stage)", result, newerPath)
	}
}

func TestPickCanonicalThreeStates(t *testing.T) {
	dir := t.TempDir()
	todoPath := setupIssueFile(t, dir, "", "dup.md", "# Todo\n")
	readyPath := setupIssueFile(t, dir, "test-ready", "dup.md", "# Ready\n")
	donePath := setupIssueFile(t, dir, "done", "dup.md", "# Done\n")

	result, err := PickCanonical([]string{todoPath, readyPath, donePath})
	if err != nil {
		t.Fatalf("PickCanonical failed: %v", err)
	}
	if result != donePath {
		t.Errorf("PickCanonical = %q, want %q (done preferred over test-ready and todo)", result, donePath)
	}
}

func TestPickCanonicalWithNonExistentFile(t *testing.T) {
	dir := t.TempDir()
	realPath := setupIssueFile(t, dir, "done", "dup.md", "# Done\n")
	bogusPath := filepath.Join(dir, "nonexistent", "dup.md")

	// Even though one path doesn't exist, the function should still return
	// the real file (stat error on bogus path means it sorts after real).
	result, err := PickCanonical([]string{bogusPath, realPath})
	if err != nil {
		t.Fatalf("PickCanonical failed: %v", err)
	}
	if result != realPath {
		t.Errorf("PickCanonical = %q, want %q (valid file preferred over non-existent)", result, realPath)
	}
}

func TestPickCanonicalMixedInputOrder(t *testing.T) {
	dir := t.TempDir()
	todoPath := setupIssueFile(t, dir, "", "dup.md", "# Todo\n")
	donePath := setupIssueFile(t, dir, "done", "dup.md", "# Done\n")
	readyPath := setupIssueFile(t, dir, "test-ready", "dup.md", "# Ready\n")

	// Input order: done, todo, test-ready — result should still be done (highest stage).
	result, err := PickCanonical([]string{donePath, todoPath, readyPath})
	if err != nil {
		t.Fatalf("PickCanonical failed: %v", err)
	}
	if result != donePath {
		t.Errorf("PickCanonical = %q, want %q (done preferred regardless of input order)", result, donePath)
	}
}

func TestQuarantineDuplicatesNoDuplicates(t *testing.T) {
	dir := t.TempDir()
	for _, sub := range []string{"test-ready", "done", ".quarantine"} {
		os.MkdirAll(filepath.Join(dir, sub), 0755)
	}

	state := &PipelineState{
		TodoFiles:       []string{filepath.Join(dir, "todo.md")},
		TestReadyFiles:  []string{filepath.Join(dir, "test-ready", "ready.md")},
		DoneFiles:       []string{filepath.Join(dir, "done", "done.md")},
		QuarantineFiles: []string{filepath.Join(dir, ".quarantine", "quar.md")},
	}

	quarantined, issues := QuarantineDuplicates(state)
	if quarantined != 0 {
		t.Errorf("expected 0 quarantined, got %d", quarantined)
	}
	if len(issues) != 0 {
		t.Errorf("expected 0 issues, got %d", len(issues))
	}
}

func TestQuarantineDuplicatesTwoStates(t *testing.T) {
	dir := t.TempDir()
	setupIssueFile(t, dir, "", "dup.md", "# Todo\n\nGitHub: #10\n")
	setupIssueFile(t, dir, "test-ready", "dup.md", "# Ready\n\nGitHub: #20\n")

	state := &PipelineState{
		TodoFiles:      []string{filepath.Join(dir, "dup.md")},
		TestReadyFiles: []string{filepath.Join(dir, "test-ready", "dup.md")},
	}

	quarantined, issues := QuarantineDuplicates(state)
	if quarantined != 1 {
		t.Errorf("expected 1 quarantined (todo copy), got %d", quarantined)
	}
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
	if !strings.Contains(issues[0].Message, "dup.md") {
		t.Errorf("expected message to mention 'dup.md', got %q", issues[0].Message)
	}
	if issues[0].Severity != SeverityWarning {
		t.Errorf("expected severity %q, got %q", SeverityWarning, issues[0].Severity)
	}

	// Verify the lower-stage (todo) file was removed, canonical (test-ready) kept
	if _, err := os.Stat(filepath.Join(dir, "dup.md")); !os.IsNotExist(err) {
		t.Errorf("expected todo file to be removed, still exists")
	}
	if _, err := os.Stat(filepath.Join(dir, "test-ready", "dup.md")); os.IsNotExist(err) {
		t.Errorf("expected test-ready canonical file to remain, was removed")
	}

	// Verify file now exists in quarantine with timestamp prefix
	entries, err := os.ReadDir(filepath.Join(dir, ".quarantine"))
	if err != nil {
		t.Fatalf("failed to list quarantine dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry in quarantine dir, got %d", len(entries))
	}
	if !strings.HasSuffix(entries[0].Name(), "_dup.md") {
		t.Errorf("expected quarantine entry matching *_dup.md, got %q", entries[0].Name())
	}

	// Verify PipelineState updated
	if len(state.TodoFiles) != 0 {
		t.Errorf("expected 0 todo files, got %d", len(state.TodoFiles))
	}
	if len(state.TestReadyFiles) != 1 {
		t.Errorf("expected 1 test-ready file (canonical kept), got %d", len(state.TestReadyFiles))
	}
	if len(state.QuarantineFiles) != 1 {
		t.Errorf("expected 1 quarantine file, got %d", len(state.QuarantineFiles))
	}
}

func TestQuarantineDuplicatesThreeStates(t *testing.T) {
	dir := t.TempDir()
	setupIssueFile(t, dir, "", "dup.md", "# Todo\n\nGitHub: #10\n")
	setupIssueFile(t, dir, "test-ready", "dup.md", "# Ready\n\nGitHub: #20\n")
	setupIssueFile(t, dir, "done", "dup.md", "# Done\n\nGitHub: #30\n")

	state := &PipelineState{
		TodoFiles:      []string{filepath.Join(dir, "dup.md")},
		TestReadyFiles: []string{filepath.Join(dir, "test-ready", "dup.md")},
		DoneFiles:      []string{filepath.Join(dir, "done", "dup.md")},
	}

	quarantined, issues := QuarantineDuplicates(state)
	if quarantined != 2 {
		t.Errorf("expected 2 quarantined (todo + test-ready), got %d", quarantined)
	}
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}

	// Low-stage files removed, canonical (done) kept
	if _, err := os.Stat(filepath.Join(dir, "dup.md")); !os.IsNotExist(err) {
		t.Errorf("expected todo file to be removed")
	}
	if _, err := os.Stat(filepath.Join(dir, "test-ready", "dup.md")); !os.IsNotExist(err) {
		t.Errorf("expected test-ready file to be removed")
	}
	if _, err := os.Stat(filepath.Join(dir, "done", "dup.md")); os.IsNotExist(err) {
		t.Errorf("expected done canonical file to remain, was removed")
	}

	// Files exist in quarantine with timestamp prefix
	entries, err := os.ReadDir(filepath.Join(dir, ".quarantine"))
	if err != nil {
		t.Fatalf("failed to list quarantine dir: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries in quarantine dir, got %d", len(entries))
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), "_dup.md") {
			t.Errorf("expected quarantine entry matching *_dup.md, got %q", e.Name())
		}
	}

	if len(state.TodoFiles) != 0 {
		t.Errorf("expected 0 todo files, got %d", len(state.TodoFiles))
	}
	if len(state.TestReadyFiles) != 0 {
		t.Errorf("expected 0 test-ready files, got %d", len(state.TestReadyFiles))
	}
	if len(state.DoneFiles) != 1 {
		t.Errorf("expected 1 done file (canonical kept), got %d", len(state.DoneFiles))
	}
	if len(state.QuarantineFiles) != 2 {
		t.Errorf("expected 2 quarantine files, got %d", len(state.QuarantineFiles))
	}
}

func TestQuarantineDuplicatesMultipleDuplicateGroups(t *testing.T) {
	dir := t.TempDir()
	setupIssueFile(t, dir, "", "a.md", "# A\n\nGitHub: #10\n")
	setupIssueFile(t, dir, "test-ready", "a.md", "# A\n\nGitHub: #20\n")
	setupIssueFile(t, dir, "", "b.md", "# B\n\nGitHub: #30\n")
	setupIssueFile(t, dir, "test-ready", "b.md", "# B\n\nGitHub: #40\n")

	state := &PipelineState{
		TodoFiles:      []string{filepath.Join(dir, "a.md"), filepath.Join(dir, "b.md")},
		TestReadyFiles: []string{filepath.Join(dir, "test-ready", "a.md"), filepath.Join(dir, "test-ready", "b.md")},
	}

	quarantined, issues := QuarantineDuplicates(state)
	if quarantined != 2 {
		t.Errorf("expected 2 quarantined (todo copies), got %d", quarantined)
	}
	if len(issues) != 2 {
		t.Fatalf("expected 2 issues, got %d", len(issues))
	}

	if len(state.TodoFiles) != 0 {
		t.Errorf("expected 0 todo files, got %d", len(state.TodoFiles))
	}
	if len(state.TestReadyFiles) != 2 {
		t.Errorf("expected 2 test-ready files (canonical kept), got %d", len(state.TestReadyFiles))
	}
	if len(state.QuarantineFiles) != 2 {
		t.Errorf("expected 2 quarantine files, got %d", len(state.QuarantineFiles))
	}
}

func TestQuarantineDuplicatesOnlyQuarantineFilesUnchanged(t *testing.T) {
	dir := t.TempDir()
	setupIssueFile(t, dir, "test-ready", "unique.md", "# Unique\n\nGitHub: #10\n")

	state := &PipelineState{
		TestReadyFiles:  []string{filepath.Join(dir, "test-ready", "unique.md")},
		QuarantineFiles: []string{filepath.Join(dir, ".quarantine", "unique.md")},
	}

	quarantined, issues := QuarantineDuplicates(state)
	if quarantined != 0 {
		t.Errorf("expected 0 quarantined, got %d", quarantined)
	}
	if len(issues) != 0 {
		t.Errorf("expected 0 issues, got %d", len(issues))
	}

	// Verify test-ready file still exists
	if _, err := os.Stat(filepath.Join(dir, "test-ready", "unique.md")); os.IsNotExist(err) {
		t.Errorf("expected test-ready file to remain, it was removed")
	}

	if len(state.TestReadyFiles) != 1 {
		t.Errorf("expected 1 test-ready file, got %d", len(state.TestReadyFiles))
	}
	if len(state.QuarantineFiles) != 1 {
		t.Errorf("expected 1 quarantine file, got %d", len(state.QuarantineFiles))
	}
}

func TestMoveFileSameDevice(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.txt")
	dst := filepath.Join(dir, "dest.txt")
	if err := os.WriteFile(src, []byte("hello"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	if err := moveFile(src, dst); err != nil {
		t.Fatalf("moveFile failed: %v", err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Error("expected source file to be removed")
	}
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("expected 'hello', got %q", string(data))
	}
}

func TestCopyFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.txt")
	dst := filepath.Join(dir, "b.txt")
	if err := os.WriteFile(src, []byte("copy test"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile failed: %v", err)
	}
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(data) != "copy test" {
		t.Errorf("expected 'copy test', got %q", string(data))
	}
}

func TestCopyFileNonExistentSource(t *testing.T) {
	dir := t.TempDir()
	err := copyFile(filepath.Join(dir, "missing.txt"), filepath.Join(dir, "dest.txt"))
	if err == nil {
		t.Fatal("expected error for non-existent source")
	}
}

func TestCopyAndDelete(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "original.txt")
	dst := filepath.Join(dir, "copied.txt")
	if err := os.WriteFile(src, []byte("to delete"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	if err := copyAndDelete(src, dst); err != nil {
		t.Fatalf("copyAndDelete failed: %v", err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Error("expected source to be deleted")
	}
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(data) != "to delete" {
		t.Errorf("expected 'to delete', got %q", string(data))
	}
}

func TestCopyAndDeleteSourceNotExist(t *testing.T) {
	dir := t.TempDir()
	err := copyAndDelete(filepath.Join(dir, "missing.txt"), filepath.Join(dir, "dest.txt"))
	if err == nil {
		t.Fatal("expected error for non-existent source")
	}
}

func TestTransitionEventFields(t *testing.T) {
	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	ev := TransitionEvent{
		Time:  now,
		Title: "Test Issue",
		From:  StateTodo,
		To:    StateTestReady,
	}
	if ev.Time != now {
		t.Errorf("Time = %v, want %v", ev.Time, now)
	}
	if ev.Title != "Test Issue" {
		t.Errorf("Title = %q, want %q", ev.Title, "Test Issue")
	}
	if ev.From != StateTodo {
		t.Errorf("From = %q, want %q", ev.From, StateTodo)
	}
	if ev.To != StateTestReady {
		t.Errorf("To = %q, want %q", ev.To, StateTestReady)
	}
}

func TestAppendAndReadTransitionLog(t *testing.T) {
	dir := t.TempDir()

	ev1 := TransitionEvent{Title: "Issue One", From: StateTodo, To: StateTestReady}
	if err := AppendTransitionLog(dir, ev1); err != nil {
		t.Fatalf("AppendTransitionLog failed: %v", err)
	}

	ev2 := TransitionEvent{Title: "Issue Two", From: StateTestReady, To: StateDone}
	if err := AppendTransitionLog(dir, ev2); err != nil {
		t.Fatalf("AppendTransitionLog failed: %v", err)
	}

	events, err := ReadTransitionLog(dir, 0)
	if err != nil {
		t.Fatalf("ReadTransitionLog failed: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Title != "Issue One" || events[0].From != StateTodo || events[0].To != StateTestReady {
		t.Errorf("events[0] = %+v, want {Issue One, todo, test-ready}", events[0])
	}
	if events[1].Title != "Issue Two" || events[1].From != StateTestReady || events[1].To != StateDone {
		t.Errorf("events[1] = %+v, want {Issue Two, test-ready, done}", events[1])
	}

	// Verify timestamps are set
	if events[0].Time.IsZero() {
		t.Error("expected non-zero timestamp on events[0]")
	}
	if events[1].Time.IsZero() {
		t.Error("expected non-zero timestamp on events[1]")
	}
	if events[1].Time.Before(events[0].Time) {
		t.Error("events[1] should be after events[0]")
	}
}

func TestReadTransitionLogLimit(t *testing.T) {
	dir := t.TempDir()

	for i := 0; i < 10; i++ {
		AppendTransitionLog(dir, TransitionEvent{
			Title: fmt.Sprintf("Issue %d", i),
			From:  StateTodo,
			To:    StateTestReady,
		})
	}

	events, err := ReadTransitionLog(dir, 3)
	if err != nil {
		t.Fatalf("ReadTransitionLog failed: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 events (limited from 10), got %d", len(events))
	}
	if events[0].Title != "Issue 7" {
		t.Errorf("expected first limited event 'Issue 7', got %q", events[0].Title)
	}
	if events[2].Title != "Issue 9" {
		t.Errorf("expected last limited event 'Issue 9', got %q", events[2].Title)
	}
}

func TestReadTransitionLogNonExistent(t *testing.T) {
	dir := t.TempDir()
	events, err := ReadTransitionLog(dir, 5)
	if err != nil {
		t.Fatalf("ReadTransitionLog for non-existent log: %v", err)
	}
	if events != nil {
		t.Errorf("expected nil for non-existent log, got %v", events)
	}
}

func TestReadTransitionLogEmpty(t *testing.T) {
	dir := t.TempDir()
	// Create the .loop dir but no log file
	os.MkdirAll(filepath.Join(dir, ".loop"), 0755)
	events, err := ReadTransitionLog(dir, 5)
	if err != nil {
		t.Fatalf("ReadTransitionLog for empty dir: %v", err)
	}
	if events != nil {
		t.Errorf("expected nil for empty log dir, got %v", events)
	}
}

func TestReadTransitionLogZeroLimit(t *testing.T) {
	dir := t.TempDir()
	AppendTransitionLog(dir, TransitionEvent{Title: "A", From: StateTodo, To: StateTestReady})
	AppendTransitionLog(dir, TransitionEvent{Title: "B", From: StateTestReady, To: StateDone})

	events, err := ReadTransitionLog(dir, 0)
	if err != nil {
		t.Fatalf("ReadTransitionLog with limit 0: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events (no limit), got %d", len(events))
	}
}

func TestReadTransitionLogNegativeLimit(t *testing.T) {
	dir := t.TempDir()
	AppendTransitionLog(dir, TransitionEvent{Title: "A", From: StateTodo, To: StateTestReady})
	AppendTransitionLog(dir, TransitionEvent{Title: "B", From: StateTestReady, To: StateDone})

	events, err := ReadTransitionLog(dir, -1)
	if err != nil {
		t.Fatalf("ReadTransitionLog with limit -1: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events (negative limit = all), got %d", len(events))
	}
}

func TestTransitionEventJSON(t *testing.T) {
	ev := TransitionEvent{
		Time:  time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC),
		Title: "Login Feature",
		From:  StateTestReady,
		To:    StateDone,
	}
	data, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	var decoded TransitionEvent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if decoded.Title != ev.Title {
		t.Errorf("Title = %q, want %q", decoded.Title, ev.Title)
	}
	if decoded.From != ev.From {
		t.Errorf("From = %q, want %q", decoded.From, ev.From)
	}
	if decoded.To != ev.To {
		t.Errorf("To = %q, want %q", decoded.To, ev.To)
	}
}

func TestLogDir(t *testing.T) {
	dir := logDir("/tmp/issues")
	if dir != "/tmp/issues/.loop" {
		t.Errorf("logDir = %q, want %q", dir, "/tmp/issues/.loop")
	}
}

func TestTransitionLogPath(t *testing.T) {
	path := transitionLogPath("/tmp/issues")
	if path != "/tmp/issues/.loop/transitions.log" {
		t.Errorf("transitionLogPath = %q, want %q", path, "/tmp/issues/.loop/transitions.log")
	}
}

func TestMoveLogsTransition(t *testing.T) {
	dir := t.TempDir()
	iss, err := Create(dir, StateTodo, "Logging Test", "## Blocked by\n\n- None\n\nExecution mode: AFK-only")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if err := Move(dir, *iss, StateTestReady); err != nil {
		t.Fatalf("Move failed: %v", err)
	}

	events, err := ReadTransitionLog(dir, 0)
	if err != nil {
		t.Fatalf("ReadTransitionLog failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 transition event, got %d", len(events))
	}
	if events[0].Title != "Logging Test" {
		t.Errorf("Title = %q, want %q", events[0].Title, "Logging Test")
	}
	if events[0].From != StateTodo {
		t.Errorf("From = %q, want %q", events[0].From, StateTodo)
	}
	if events[0].To != StateTestReady {
		t.Errorf("To = %q, want %q", events[0].To, StateTestReady)
	}
}

func TestMoveLogsQuarantineTransition(t *testing.T) {
	dir := t.TempDir()
	iss, err := Create(dir, StateTestReady, "Quarantine Test", "## Blocked by\n\n- None")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if err := Move(dir, *iss, StateQuarantine); err != nil {
		t.Fatalf("Move to quarantine failed: %v", err)
	}

	events, err := ReadTransitionLog(dir, 0)
	if err != nil {
		t.Fatalf("ReadTransitionLog failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 transition event, got %d", len(events))
	}
	if events[0].Title != "Quarantine Test" {
		t.Errorf("Title = %q, want %q", events[0].Title, "Quarantine Test")
	}
	if events[0].From != StateTestReady {
		t.Errorf("From = %q, want %q", events[0].From, StateTestReady)
	}
	if events[0].To != StateQuarantine {
		t.Errorf("To = %q, want %q", events[0].To, StateQuarantine)
	}
}

func TestMoveDoesNotLogSameState(t *testing.T) {
	dir := t.TempDir()
	iss, err := Create(dir, StateTodo, "No Move", "Body")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if err := Move(dir, *iss, StateTodo); err != nil {
		t.Fatalf("Move to same state failed: %v", err)
	}

	events, err := ReadTransitionLog(dir, 0)
	if err != nil {
		t.Fatalf("ReadTransitionLog failed: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected 0 events for same-state move, got %d", len(events))
	}
}

func TestPreFlightCheckRepairQuarantinesDuplicates(t *testing.T) {
	dir := t.TempDir()
	setupIssueFile(t, dir, "", "dup.md", "# Todo\n\nGitHub: #10\nExecution mode: AFK-only\n\n## Parent\n\n## What to build\n\n## User stories covered\n\n## Acceptance criteria\n\n## UAT plan\n\n## Blocked by\n")
	setupIssueFile(t, dir, "test-ready", "dup.md", "# Ready\n\nGitHub: #20\nExecution mode: AFK-only\n\n## Parent\n\n## What to build\n\n## User stories covered\n\n## Acceptance criteria\n\n## UAT plan\n\n## Blocked by\n")

	state := &PipelineState{
		TodoFiles:      []string{filepath.Join(dir, "dup.md")},
		TestReadyFiles: []string{filepath.Join(dir, "test-ready", "dup.md")},
	}

	issues := PreFlightCheck(state, true, true)

	// With repair=true, quarantine warnings are returned so the user sees
	// what was repaired. Verify there are no error-level duplicate issues.
	var errIssues []PreFlightIssue
	var warnQuarantine int
	for _, i := range issues {
		if i.Severity == SeverityError && strings.Contains(i.Message, "duplicate filename") {
			errIssues = append(errIssues, i)
		}
		if i.Severity == SeverityWarning && strings.Contains(i.Message, "quarantined") {
			warnQuarantine++
		}
	}
	if len(errIssues) > 0 {
		t.Errorf("expected 0 error-level duplicate filename issues with repair=true (should quarantine), got %d", len(errIssues))
	}
	if warnQuarantine == 0 {
		t.Error("expected at least one quarantine warning to be returned")
	}

	if len(state.QuarantineFiles) == 0 {
		t.Error("expected files to be quarantined when repair=true")
	}
}

func TestPreFlightCheckRepairOnlyOneFileState(t *testing.T) {
	dir := t.TempDir()
	setupIssueFile(t, dir, "", "unique.md", "# Todo\n\nGitHub: #1\nExecution mode: AFK-only\n\n## Parent\n\n## What to build\n\n## User stories covered\n\n## Acceptance criteria\n\n## UAT plan\n\n## Blocked by\n")

	state := &PipelineState{
		TodoFiles: []string{filepath.Join(dir, "unique.md")},
	}

	issues := PreFlightCheck(state, true, true)

	if len(issues) != 0 {
		t.Errorf("expected 0 issues with repair=true (no duplicates), got %d: %v", len(issues), issues)
	}
}

func TestPreFlightCheckRepairFalseStillReportsDupes(t *testing.T) {
	dir := t.TempDir()
	setupIssueFile(t, dir, "", "dup.md", "# Todo\n\nGitHub: #10\nExecution mode: AFK-only\n\n## Parent\n\n## What to build\n\n## User stories covered\n\n## Acceptance criteria\n\n## UAT plan\n\n## Blocked by\n")
	setupIssueFile(t, dir, "test-ready", "dup.md", "# Ready\n\nGitHub: #20\nExecution mode: AFK-only\n\n## Parent\n\n## What to build\n\n## User stories covered\n\n## Acceptance criteria\n\n## UAT plan\n\n## Blocked by\n")

	state := &PipelineState{
		TodoFiles:      []string{filepath.Join(dir, "dup.md")},
		TestReadyFiles: []string{filepath.Join(dir, "test-ready", "dup.md")},
	}

	issues := PreFlightCheck(state, false, true)

	var dupIssues []PreFlightIssue
	for _, i := range issues {
		if strings.Contains(i.Message, "duplicate filename") {
			dupIssues = append(dupIssues, i)
		}
	}
	if len(dupIssues) != 1 {
		t.Errorf("expected 1 duplicate filename issue with repair=false, got %d", len(dupIssues))
	}
}

func TestSectionHasContent(t *testing.T) {
	tests := []struct {
		name    string
		content string
		section string
		want    bool
	}{
		{
			name:    "section with content at end",
			content: "# Title\n\n## UAT Results\n\nStep 1: pass\n",
			section: "UAT Results",
			want:    true,
		},
		{
			name:    "empty section",
			content: "# Title\n\n## UAT Results\n",
			section: "UAT Results",
			want:    false,
		},
		{
			name:    "section with only whitespace",
			content: "# Title\n\n## UAT Results\n  \n\t\n",
			section: "UAT Results",
			want:    false,
		},
		{
			name:    "section not present",
			content: "# Title\n\n## Other\n\nContent\n",
			section: "UAT Results",
			want:    false,
		},
		{
			name:    "section with content between others",
			content: "# Title\n\n## Plan\n\nDo stuff\n\n## UAT Results\n\nStep 1: pass\n\n## Comments\nDone\n",
			section: "UAT Results",
			want:    true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := sectionHasContent(tc.content, tc.section)
			if got != tc.want {
				t.Errorf("sectionHasContent(%q) = %v, want %v", tc.section, got, tc.want)
			}
		})
	}
}

func TestPromoteStuckTestReadyPromotesFilled(t *testing.T) {
	dir := t.TempDir()
	readyDir := filepath.Join(dir, "test-ready")
	os.MkdirAll(readyDir, 0755)

	content := "# 01 - Test issue\n\n## What to build\n\nSomething\n\n## UAT Results\n\n| Step | Result |\n| --- | --- |\n| Login | Pass |\n"
	path := filepath.Join(readyDir, "issue.md")
	os.WriteFile(path, []byte(content), 0644)

	promoted, err := PromoteStuckTestReady(dir)
	if err != nil {
		t.Fatalf("PromoteStuckTestReady failed: %v", err)
	}
	if len(promoted) != 1 {
		t.Fatalf("expected 1 promoted file, got %d", len(promoted))
	}

	doneDir := filepath.Join(dir, "done")
	donePath := filepath.Join(doneDir, "issue.md")
	if _, err := os.Stat(donePath); os.IsNotExist(err) {
		t.Error("expected file to exist in done/ directory")
	}
}

func TestPromoteStuckTestReadySkipsEmpty(t *testing.T) {
	dir := t.TempDir()
	readyDir := filepath.Join(dir, "test-ready")
	os.MkdirAll(readyDir, 0755)

	content := "# 01 - Test issue\n\n## What to build\n\nSomething\n\n## UAT Results\n"
	path := filepath.Join(readyDir, "issue.md")
	os.WriteFile(path, []byte(content), 0644)

	promoted, err := PromoteStuckTestReady(dir)
	if err != nil {
		t.Fatalf("PromoteStuckTestReady failed: %v", err)
	}
	if len(promoted) != 0 {
		t.Errorf("expected 0 promoted files (empty UAT Results), got %d", len(promoted))
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("expected file to remain in place")
	}
}

func TestPromoteStuckTestReadyNoUAT(t *testing.T) {
	dir := t.TempDir()
	readyDir := filepath.Join(dir, "test-ready")
	os.MkdirAll(readyDir, 0755)

	content := "# 01 - Test issue\n\n## What to build\n\nSomething\n"
	path := filepath.Join(readyDir, "issue.md")
	os.WriteFile(path, []byte(content), 0644)

	promoted, err := PromoteStuckTestReady(dir)
	if err != nil {
		t.Fatalf("PromoteStuckTestReady failed: %v", err)
	}
	if len(promoted) != 0 {
		t.Errorf("expected 0 promoted files (no UAT Results), got %d", len(promoted))
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("expected file to remain in place")
	}
}

func TestPromoteStuckTestReadyEmptyDir(t *testing.T) {
	dir := t.TempDir()
	promoted, err := PromoteStuckTestReady(dir)
	if err != nil {
		t.Fatalf("PromoteStuckTestReady failed: %v", err)
	}
	if len(promoted) != 0 {
		t.Errorf("expected 0 promoted files, got %d", len(promoted))
	}
}

func TestFindStuckTestReadyFilesFilled(t *testing.T) {
	dir := t.TempDir()
	readyDir := filepath.Join(dir, "test-ready")
	os.MkdirAll(readyDir, 0755)

	content := "# 01 - Test issue\n\n## What to build\n\nSomething\n\n## UAT Results\n\n| Step | Result |\n| --- | --- |\n| Login | Pass |\n"
	path := filepath.Join(readyDir, "issue.md")
	os.WriteFile(path, []byte(content), 0644)

	stuck, err := FindStuckTestReadyFiles(dir)
	if err != nil {
		t.Fatalf("FindStuckTestReadyFiles failed: %v", err)
	}
	if len(stuck) != 1 {
		t.Fatalf("expected 1 stuck file, got %d", len(stuck))
	}
	if stuck[0] != path {
		t.Errorf("expected %q, got %q", path, stuck[0])
	}

	// Verify file was NOT moved
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("expected file to remain in test-ready/")
	}
}

func TestFindStuckTestReadyFilesSkipsEmpty(t *testing.T) {
	dir := t.TempDir()
	readyDir := filepath.Join(dir, "test-ready")
	os.MkdirAll(readyDir, 0755)

	content := "# 01 - Test issue\n\n## What to build\n\nSomething\n\n## UAT Results\n"
	path := filepath.Join(readyDir, "issue.md")
	os.WriteFile(path, []byte(content), 0644)

	stuck, err := FindStuckTestReadyFiles(dir)
	if err != nil {
		t.Fatalf("FindStuckTestReadyFiles failed: %v", err)
	}
	if len(stuck) != 0 {
		t.Errorf("expected 0 stuck files (empty UAT Results), got %d", len(stuck))
	}
}

func TestFindStuckTestReadyFilesNoUAT(t *testing.T) {
	dir := t.TempDir()
	readyDir := filepath.Join(dir, "test-ready")
	os.MkdirAll(readyDir, 0755)

	content := "# 01 - Test issue\n\n## What to build\n\nSomething\n"
	path := filepath.Join(readyDir, "issue.md")
	os.WriteFile(path, []byte(content), 0644)

	stuck, err := FindStuckTestReadyFiles(dir)
	if err != nil {
		t.Fatalf("FindStuckTestReadyFiles failed: %v", err)
	}
	if len(stuck) != 0 {
		t.Errorf("expected 0 stuck files (no UAT Results), got %d", len(stuck))
	}
}

func TestFindStuckTestReadyFilesEmptyDir(t *testing.T) {
	dir := t.TempDir()
	stuck, err := FindStuckTestReadyFiles(dir)
	if err != nil {
		t.Fatalf("FindStuckTestReadyFiles failed: %v", err)
	}
	if len(stuck) != 0 {
		t.Errorf("expected 0 stuck files, got %d", len(stuck))
	}
}

func TestFindInvalidExecModesDetectsInvalid(t *testing.T) {
	dir := t.TempDir()

	os.MkdirAll(filepath.Join(dir, "test-ready"), 0755)

	os.WriteFile(filepath.Join(dir, "typo.md"), []byte("# Typo\n\nExecution mode: AFK_onLy\n\nBody\n"), 0644)
	os.WriteFile(filepath.Join(dir, "test-ready", "bad.md"), []byte("# Bad\n\nExecution mode: combo\n\nBody\n"), 0644)

	invalid, err := FindInvalidExecModes(dir)
	if err != nil {
		t.Fatalf("FindInvalidExecModes failed: %v", err)
	}
	if len(invalid) != 2 {
		t.Errorf("expected 2 invalid exec mode files, got %d", len(invalid))
	}
}

func TestFindInvalidExecModesSkipsMissing(t *testing.T) {
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "no-mode.md"), []byte("# No Mode\n\nBody\n"), 0644)

	invalid, err := FindInvalidExecModes(dir)
	if err != nil {
		t.Fatalf("FindInvalidExecModes failed: %v", err)
	}
	if len(invalid) != 0 {
		t.Errorf("expected 0 invalid exec mode files, got %d", len(invalid))
	}
}

func TestFindInvalidExecModesSkipsValid(t *testing.T) {
	dir := t.TempDir()

	os.MkdirAll(filepath.Join(dir, "done"), 0755)

	os.WriteFile(filepath.Join(dir, "afk.md"), []byte("# AFK\n\nExecution mode: AFK-only\n\nBody\n"), 0644)
	os.WriteFile(filepath.Join(dir, "done", "hitl.md"), []byte("# HITL\n\nExecution mode: HITL-only\n\nBody\n"), 0644)

	invalid, err := FindInvalidExecModes(dir)
	if err != nil {
		t.Fatalf("FindInvalidExecModes failed: %v", err)
	}
	if len(invalid) != 0 {
		t.Errorf("expected 0 invalid exec mode files, got %d", len(invalid))
	}
}

func TestFindInvalidExecModesEmptyDir(t *testing.T) {
	dir := t.TempDir()
	invalid, err := FindInvalidExecModes(dir)
	if err != nil {
		t.Fatalf("FindInvalidExecModes failed: %v", err)
	}
	if len(invalid) != 0 {
		t.Errorf("expected 0 invalid exec mode files, got %d", len(invalid))
	}
}

func TestRepairPipelineStripsAndReportsStuck(t *testing.T) {
	dir := t.TempDir()
	readyDir := filepath.Join(dir, "test-ready")
	os.MkdirAll(readyDir, 0755)

	// File with filled UAT Results (should be reported as stuck, NOT promoted)
	filled := "# 01 - Tested\n\nExecution mode: AFK-only\n\n## What to build\n\nStuff\n\n## UAT Results\n\n| Step | Result |\n| --- | --- |\n| Check | Pass |\n"
	filledPath := filepath.Join(readyDir, "filled.md")
	os.WriteFile(filledPath, []byte(filled), 0644)

	// File with empty UAT Results (should be stripped)
	empty := "# 02 - Empty\n\nExecution mode: AFK-only\n\n## What to build\n\nStuff\n\n## UAT Results\n"
	os.WriteFile(filepath.Join(readyDir, "empty.md"), []byte(empty), 0644)

	labelsAdded, execModesAdded, stripped, stuckCount, testResultsPromoted, invalidExecModes, checksumsAdded, err := RepairPipeline(dir, true)
	if err != nil {
		t.Fatalf("RepairPipeline failed: %v", err)
	}
	if labelsAdded != 0 {
		t.Errorf("expected 0 labels added, got %d", labelsAdded)
	}
	if execModesAdded != 0 {
		t.Errorf("expected 0 exec modes added, got %d", execModesAdded)
	}
	if stripped != 1 {
		t.Errorf("expected 1 stripped file, got %d", stripped)
	}
	if checksumsAdded != 2 {
		t.Errorf("expected 2 checksums added, got %d", checksumsAdded)
	}
	if stuckCount != 1 {
		t.Errorf("expected 1 stuck file reported, got %d", stuckCount)
	}
	if testResultsPromoted != 0 {
		t.Errorf("expected 0 test results promoted files, got %d", testResultsPromoted)
	}
	if invalidExecModes != 0 {
		t.Errorf("expected 0 invalid exec modes, got %d", invalidExecModes)
	}

	// Verify file with filled UAT Results is LEFT in test-ready/ (NOT promoted)
	if _, err := os.Stat(filledPath); os.IsNotExist(err) {
		t.Error("expected filled.md to remain in test-ready/")
	}
	donePath := filepath.Join(dir, "done", "filled.md")
	if _, err := os.Stat(donePath); !os.IsNotExist(err) {
		t.Error("expected filled.md to NOT be promoted to done/")
	}

	// Verify empty file had its UAT Results stripped
	emptyPath := filepath.Join(readyDir, "empty.md")
	data, _ := os.ReadFile(emptyPath)
	if strings.Contains(string(data), "UAT Results") {
		t.Error("expected UAT Results to be stripped from empty.md")
	}
}

func TestRepairPipelineEmptyDir(t *testing.T) {
	dir := t.TempDir()
	labelsAdded, execModesAdded, stripped, stuckCount, testResultsPromoted, invalidExecModes, checksumsAdded, err := RepairPipeline(dir, true)
	if err != nil {
		t.Fatalf("RepairPipeline failed: %v", err)
	}
	if labelsAdded != 0 {
		t.Errorf("expected 0 labels added, got %d", labelsAdded)
	}
	if execModesAdded != 0 {
		t.Errorf("expected 0 exec modes added, got %d", execModesAdded)
	}
	if stripped != 0 {
		t.Errorf("expected 0 stripped, got %d", stripped)
	}
	if stuckCount != 0 {
		t.Errorf("expected 0 stuck count, got %d", stuckCount)
	}
	if testResultsPromoted != 0 {
		t.Errorf("expected 0 test results promoted, got %d", testResultsPromoted)
	}
	if invalidExecModes != 0 {
		t.Errorf("expected 0 invalid exec modes, got %d", invalidExecModes)
	}
	if checksumsAdded != 0 {
		t.Errorf("expected 0 checksums added, got %d", checksumsAdded)
	}
}

func TestPromoteTodoWithTestResultsPromotesFilled(t *testing.T) {
	dir := t.TempDir()

	content := "# 01 - Implemented issue\n\n## What to build\n\nSomething\n\n## Test Results\n\nAll tests pass\n"
	path := filepath.Join(dir, "issue.md")
	os.WriteFile(path, []byte(content), 0644)

	promoted, err := PromoteTodoWithTestResults(dir)
	if err != nil {
		t.Fatalf("PromoteTodoWithTestResults failed: %v", err)
	}
	if len(promoted) != 1 {
		t.Fatalf("expected 1 promoted file, got %d", len(promoted))
	}

	readyDir := filepath.Join(dir, "test-ready")
	readyPath := filepath.Join(readyDir, "issue.md")
	if _, err := os.Stat(readyPath); os.IsNotExist(err) {
		t.Error("expected file to exist in test-ready/ directory")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("expected original file to be removed")
	}
}

func TestPromoteTodoWithTestResultsSkipsEmpty(t *testing.T) {
	dir := t.TempDir()

	content := "# 01 - Issue\n\n## What to build\n\nSomething\n\n## Test Results\n"
	path := filepath.Join(dir, "issue.md")
	os.WriteFile(path, []byte(content), 0644)

	promoted, err := PromoteTodoWithTestResults(dir)
	if err != nil {
		t.Fatalf("PromoteTodoWithTestResults failed: %v", err)
	}
	if len(promoted) != 0 {
		t.Errorf("expected 0 promoted files (empty Test Results), got %d", len(promoted))
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("expected file to remain in place")
	}
}

func TestPromoteTodoWithTestResultsNoTestResults(t *testing.T) {
	dir := t.TempDir()

	content := "# 01 - Issue\n\n## What to build\n\nSomething\n"
	path := filepath.Join(dir, "issue.md")
	os.WriteFile(path, []byte(content), 0644)

	promoted, err := PromoteTodoWithTestResults(dir)
	if err != nil {
		t.Fatalf("PromoteTodoWithTestResults failed: %v", err)
	}
	if len(promoted) != 0 {
		t.Errorf("expected 0 promoted files (no Test Results), got %d", len(promoted))
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("expected file to remain in place")
	}
}

func TestPromoteTodoWithTestResultsEmptyDir(t *testing.T) {
	dir := t.TempDir()
	promoted, err := PromoteTodoWithTestResults(dir)
	if err != nil {
		t.Fatalf("PromoteTodoWithTestResults failed: %v", err)
	}
	if len(promoted) != 0 {
		t.Errorf("expected 0 promoted files, got %d", len(promoted))
	}
}

func TestRepairPipelinePromotesTodoWithTestResults(t *testing.T) {
	dir := t.TempDir()
	readyDir := filepath.Join(dir, "test-ready")
	os.MkdirAll(readyDir, 0755)

	// File in todo with Test Results (should be promoted to test-ready)
	content := "# 01 - Implemented\n\nExecution mode: AFK-only\n\n## What to build\n\nStuff\n\n## Test Results\n\nAll tests pass\n"
	todoPath := filepath.Join(dir, "implemented.md")
	os.WriteFile(todoPath, []byte(content), 0644)

	// File in test-ready with filled UAT Results (should be reported as stuck, NOT promoted)
	filled := "# 02 - Tested\n\nExecution mode: AFK-only\n\n## What to build\n\nStuff\n\n## UAT Results\n\n| Step | Result |\n| --- | --- |\n| Check | Pass |\n"
	filledPath := filepath.Join(readyDir, "tested.md")
	os.WriteFile(filledPath, []byte(filled), 0644)

	labelsAdded, execModesAdded, stripped, stuckCount, testResultsPromoted, invalidExecModes, checksumsAdded, err := RepairPipeline(dir, true)
	if err != nil {
		t.Fatalf("RepairPipeline failed: %v", err)
	}
	if labelsAdded != 1 {
		t.Errorf("expected 1 label added, got %d", labelsAdded)
	}
	if execModesAdded != 0 {
		t.Errorf("expected 0 exec modes added, got %d", execModesAdded)
	}
	if stripped != 0 {
		t.Errorf("expected 0 stripped, got %d", stripped)
	}
	if stuckCount != 1 {
		t.Errorf("expected 1 stuck file reported, got %d", stuckCount)
	}
	if testResultsPromoted != 1 {
		t.Errorf("expected 1 test results promoted, got %d", testResultsPromoted)
	}
	if invalidExecModes != 0 {
		t.Errorf("expected 0 invalid exec modes, got %d", invalidExecModes)
	}
	if checksumsAdded != 2 {
		t.Errorf("expected 2 checksums added, got %d", checksumsAdded)
	}

	// Verify todo file promoted to test-ready/
	readyPath := filepath.Join(readyDir, "implemented.md")
	if _, err := os.Stat(readyPath); os.IsNotExist(err) {
		t.Error("expected implemented.md to be promoted to test-ready/")
	}

	// Verify test-ready file with UAT Results is LEFT in test-ready/ (NOT promoted to done/)
	if _, err := os.Stat(filledPath); os.IsNotExist(err) {
		t.Error("expected tested.md to remain in test-ready/")
	}
	doneDir := filepath.Join(dir, "done")
	donePath := filepath.Join(doneDir, "tested.md")
	if _, err := os.Stat(donePath); !os.IsNotExist(err) {
		t.Error("expected tested.md to NOT be promoted to done/")
	}
}

func TestMoveFilePreservesMode(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "exec.sh")
	dst := filepath.Join(dir, "moved.sh")
	if err := os.WriteFile(src, []byte("#!/bin/sh\necho hi\n"), 0755); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	if err := moveFile(src, dst); err != nil {
		t.Fatalf("moveFile failed: %v", err)
	}
	fi, err := os.Stat(dst)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	if fi.Mode()&0111 == 0 {
		t.Error("expected destination to preserve executable mode")
	}
}

func TestFileMoveError(t *testing.T) {
	inner := fmt.Errorf("permission denied")
	e := &FileMoveError{
		Src:        "/a",
		Dst:        "/b",
		Err:        inner,
		Suggestion: "check permissions",
	}
	if e.Error() != "permission denied" {
		t.Errorf("Error() = %q, want %q", e.Error(), "permission denied")
	}
	if !errors.Is(e, inner) {
		t.Error("errors.Is should match the wrapped error")
	}
	var got *FileMoveError
	if !errors.As(e, &got) {
		t.Error("errors.As should find FileMoveError")
	}
	if got.Suggestion != "check permissions" {
		t.Errorf("Suggestion = %q, want %q", got.Suggestion, "check permissions")
	}
}

func TestIsRetryableError(t *testing.T) {
	tests := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{fmt.Errorf("random error"), false},
		{os.ErrPermission, true},
		{&os.PathError{Op: "open", Path: "/x", Err: syscall.EACCES}, true},
		{&os.LinkError{Op: "rename", Old: "/x", New: "/y", Err: syscall.EACCES}, true},
		{&os.LinkError{Op: "rename", Old: "/x", New: "/y", Err: syscall.EPERM}, true},
		{syscall.EBUSY, true},
		{syscall.ETXTBSY, true},
		{fmt.Errorf("wrapped: %w", syscall.EACCES), true},
	}
	for _, tc := range tests {
		got := isRetryableError(tc.err)
		if got != tc.want {
			t.Errorf("isRetryableError(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}
}

func TestSuggestionForErr(t *testing.T) {
	perm := suggestionForErr(os.ErrPermission)
	if !strings.Contains(perm, "permissions") {
		t.Errorf("permission suggestion should mention 'permissions': %q", perm)
	}

	busy := suggestionForErr(syscall.EBUSY)
	if !strings.Contains(busy, "in use") {
		t.Errorf("busy suggestion should mention 'in use': %q", busy)
	}

	other := suggestionForErr(fmt.Errorf("some error"))
	if !strings.Contains(other, "destination directory") {
		t.Errorf("default suggestion should mention 'destination directory': %q", other)
	}
}

func TestMoveFileNonExistentSource(t *testing.T) {
	dir := t.TempDir()
	err := moveFile(filepath.Join(dir, "nonexistent.txt"), filepath.Join(dir, "dest.txt"))
	if err == nil {
		t.Fatal("expected error for non-existent source")
	}
	var moveErr *FileMoveError
	if !errors.As(err, &moveErr) {
		t.Fatalf("expected *FileMoveError to be returned, got %T", err)
	}
	if moveErr.Suggestion == "" {
		t.Error("expected non-empty Suggestion field")
	}
	if moveErr.Src == "" || moveErr.Dst == "" {
		t.Error("expected Src and Dst to be set")
	}
}

func TestMoveFilePermissionDenied(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "readonly.txt")
	dst := filepath.Join(dir, "dest.txt")
	if err := os.WriteFile(src, []byte("data"), 0444); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	if err := os.Chmod(dir, 0500); err != nil {
		t.Fatalf("Chmod failed: %v", err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0755) })

	err := moveFile(src, dst)
	if err == nil {
		t.Fatal("expected error when moving from read-only directory")
	}
	var moveErr *FileMoveError
	if !errors.As(err, &moveErr) {
		t.Fatalf("expected *FileMoveError, got %T", err)
	}
	if moveErr.Suggestion == "" {
		t.Error("expected non-empty Suggestion field")
	}
}

func TestAddMissingTodoLabelsAddsMissingStatus(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "no-status.md")
	content := "# 01 - No status\n\n## What to build\n\nThing\n"
	os.WriteFile(path, []byte(content), 0644)

	count, err := AddMissingTodoLabels(dir)
	if err != nil {
		t.Fatalf("AddMissingTodoLabels failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 file modified, got %d", count)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	updated := string(data)
	if !strings.Contains(updated, "Status: ready-for-agent") {
		t.Errorf("expected file to contain 'Status: ready-for-agent', got:\n%s", updated)
	}
}

func TestAddMissingTodoLabelsSkipsFilesWithStatus(t *testing.T) {
	dir := t.TempDir()

	// File with ready-for-agent status.
	path := filepath.Join(dir, "has-agent.md")
	os.WriteFile(path, []byte("# 01 - Has agent\n\nGitHub: #1\nStatus: ready-for-agent\n\n## What to build\n"), 0644)

	// File with ready-for-human status.
	path2 := filepath.Join(dir, "has-human.md")
	os.WriteFile(path2, []byte("# 02 - Has human\n\nStatus: ready-for-human\n\n## What to build\n"), 0644)

	count, err := AddMissingTodoLabels(dir)
	if err != nil {
		t.Fatalf("AddMissingTodoLabels failed: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 files modified, got %d", count)
	}
}

func TestAddMissingTodoLabelsSkipsNonMdFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "note.txt"), []byte("not an issue"), 0644)
	os.WriteFile(filepath.Join(dir, "data.json"), []byte("{}"), 0644)

	count, err := AddMissingTodoLabels(dir)
	if err != nil {
		t.Fatalf("AddMissingTodoLabels failed: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 files modified, got %d", count)
	}
}

func TestAddMissingTodoLabelsSkipsSubdirs(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "test-ready"), 0755)
	os.WriteFile(filepath.Join(dir, "test-ready", "nested.md"), []byte("# Nested\n\nBody"), 0644)

	count, err := AddMissingTodoLabels(dir)
	if err != nil {
		t.Fatalf("AddMissingTodoLabels failed: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 files modified (subdirs skipped), got %d", count)
	}
}

func TestAddMissingTodoLabelsIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "todo.md")
	os.WriteFile(path, []byte("# 01 - Idempotent\n\n## What to build\n\nThing\n"), 0644)

	count, err := AddMissingTodoLabels(dir)
	if err != nil {
		t.Fatalf("first call failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 on first call, got %d", count)
	}

	count, err = AddMissingTodoLabels(dir)
	if err != nil {
		t.Fatalf("second call failed: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 on second call, got %d", count)
	}
}

func TestAddMissingTodoLabelsNonExistentDir(t *testing.T) {
	count, err := AddMissingTodoLabels("/nonexistent/path")
	if err != nil {
		t.Fatalf("AddMissingTodoLabels on non-existent dir: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 for non-existent dir, got %d", count)
	}
}

func TestAddMissingTodoLabelsMultipleFiles(t *testing.T) {
	dir := t.TempDir()

	// File with GitHub line but no Status.
	os.WriteFile(filepath.Join(dir, "a.md"), []byte("# 01 - A\n\nGitHub: #1\n\n## What to build\n\nBody\n"), 0644)

	// File with no header info at all.
	os.WriteFile(filepath.Join(dir, "b.md"), []byte("# 02 - B\n\n## Parent\n\nBody\n"), 0644)

	// File that already has the label.
	os.WriteFile(filepath.Join(dir, "c.md"), []byte("# 03 - C\n\nGitHub: #3\nStatus: ready-for-agent\n\n## Parent\n\nBody\n"), 0644)

	count, err := AddMissingTodoLabels(dir)
	if err != nil {
		t.Fatalf("AddMissingTodoLabels failed: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 files modified, got %d", count)
	}

	// Verify a.md got the Status line after GitHub.
	data, err := os.ReadFile(filepath.Join(dir, "a.md"))
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "GitHub: #1\nStatus: ready-for-agent") {
		t.Errorf("expected Status line after GitHub, got:\n%s", content)
	}

	// Verify b.md got the Status line.
	data, err = os.ReadFile(filepath.Join(dir, "b.md"))
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if !strings.Contains(string(data), "Status: ready-for-agent") {
		t.Errorf("expected Status line in b.md, got:\n%s", string(data))
	}
}

func TestHasValidStatus(t *testing.T) {
	tests := []struct {
		content string
		want    bool
	}{
		{"Status: ready-for-agent", true},
		{"Status: ready-for-human", true},
		{"Status: something-else", false},
		{"", false},
		{"GitHub: #1", false},
		{"Status: ready-for-agent\nGitHub: #1", true},
		{"Status: ready-for-human\nGitHub: #1", true},
	}
	for _, tc := range tests {
		got := hasValidStatus(tc.content)
		if got != tc.want {
			t.Errorf("hasValidStatus(%q) = %v, want %v", tc.content, got, tc.want)
		}
	}
}

func TestInsertStatusLine(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "after GitHub line",
			content: "# Title\n\nGitHub: #1\n\n## Section\n",
			want:    "# Title\n\nGitHub: #1\nStatus: ready-for-agent\n\n## Section\n",
		},
		{
			name:    "no GitHub, after title",
			content: "# Title\n\n## Section\n",
			want:    "# Title\n\nStatus: ready-for-agent\n## Section\n",
		},
		{
			name:    "no blank line after title",
			content: "# Title\n## Section\n",
			want:    "# Title\nStatus: ready-for-agent\n## Section\n",
		},
		{
			name:    "empty content",
			content: "",
			want:    "\nStatus: ready-for-agent",
		},
		{
			name:    "no title at all",
			content: "Body text\n",
			want:    "Body text\n\nStatus: ready-for-agent",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := insertStatusLine(tc.content, "ready-for-agent")
			if got != tc.want {
				t.Errorf("insertStatusLine() =\n%q\nwant:\n%q", got, tc.want)
			}
		})
	}
}

func TestHasExecMode(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{"AFK-only", "# Title\n\nExecution mode: AFK-only\n", true},
		{"HITL-only", "# Title\n\nExecution mode: HITL-only\n", true},
		{"Combo", "# Title\n\nExecution mode: Combo\n", true},
		{"missing", "# Title\n\nBody\n", false},
		{"in body section", "# Title\n\n## Comments\nExecution mode: Combo\n", true},
		{"empty", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := hasExecMode(tc.content)
			if got != tc.want {
				t.Errorf("hasExecMode() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestInsertExecModeLine(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "after status line",
			content: "# Title\n\nGitHub: #1\nStatus: ready-for-agent\nBranch: main\n\n## Section\n",
			want:    "# Title\n\nGitHub: #1\nStatus: ready-for-agent\nExecution mode: AFK-only\nBranch: main\n\n## Section\n",
		},
		{
			name:    "after github line no status",
			content: "# Title\n\nGitHub: #1\nBranch: main\n\n## Section\n",
			want:    "# Title\n\nGitHub: #1\nExecution mode: AFK-only\nBranch: main\n\n## Section\n",
		},
		{
			name:    "after title with no header fields",
			content: "# Title\n\n## Section\n",
			want:    "# Title\nExecution mode: AFK-only\n\n## Section\n",
		},
		{
			name:    "with blank line after title",
			content: "# Title\n\n## Section\n",
			want:    "# Title\nExecution mode: AFK-only\n\n## Section\n",
		},
		{
			name:    "empty content",
			content: "",
			want:    "\nExecution mode: AFK-only",
		},
		{
			name:    "no title",
			content: "Body text\n",
			want:    "Body text\n\nExecution mode: AFK-only",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := insertExecModeLine(tc.content)
			if got != tc.want {
				t.Errorf("insertExecModeLine() =\n%q\nwant:\n%q", got, tc.want)
			}
		})
	}
}

func TestAddMissingExecMode(t *testing.T) {
	dir := t.TempDir()

	// File without Execution mode
	missingPath := filepath.Join(dir, "missing.md")
	os.WriteFile(missingPath, []byte("# Missing\n\nBody\n"), 0644)

	// File with Execution mode
	presentPath := filepath.Join(dir, "present.md")
	os.WriteFile(presentPath, []byte("# Present\n\nExecution mode: AFK-only\n\nBody\n"), 0644)

	count, err := AddMissingExecMode(dir)
	if err != nil {
		t.Fatalf("AddMissingExecMode failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 file modified, got %d", count)
	}

	data, _ := os.ReadFile(missingPath)
	if !strings.Contains(string(data), "Execution mode: AFK-only") {
		t.Error("expected Execution mode to be added to missing.md")
	}

	data, _ = os.ReadFile(presentPath)
	if !strings.Contains(string(data), "Execution mode: AFK-only") {
		t.Error("expected present.md to still have Execution mode")
	}
}

func TestAddMissingExecModeAllStates(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "test-ready"), 0755)
	os.MkdirAll(filepath.Join(dir, "ready-for-agent"), 0755)

	os.WriteFile(filepath.Join(dir, "todo.md"), []byte("# Todo\n\nBody\n"), 0644)
	os.WriteFile(filepath.Join(dir, "test-ready", "ready.md"), []byte("# Ready\n\nBody\n"), 0644)
	os.WriteFile(filepath.Join(dir, "ready-for-agent", "rfa.md"), []byte("# RFA\n\nBody\n"), 0644)

	count, err := AddMissingExecMode(dir)
	if err != nil {
		t.Fatalf("AddMissingExecMode failed: %v", err)
	}
	if count != 3 {
		t.Fatalf("expected 3 files modified (one per state), got %d", count)
	}
}

func TestAddMissingExecModeSkipsExecModePresent(t *testing.T) {
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "has-mode.md"), []byte("# Has\n\nExecution mode: AFK-only\n\nBody\n"), 0644)
	os.WriteFile(filepath.Join(dir, "has-combo.md"), []byte("# Combo\n\nExecution mode: Combo\n\nBody\n"), 0644)

	count, err := AddMissingExecMode(dir)
	if err != nil {
		t.Fatalf("AddMissingExecMode failed: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 files modified, got %d", count)
	}
}

func TestAddMissingExecModeEmptyDir(t *testing.T) {
	dir := t.TempDir()
	count, err := AddMissingExecMode(dir)
	if err != nil {
		t.Fatalf("AddMissingExecMode failed: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 files modified, got %d", count)
	}
}

func TestAddMissingExecModeNonExistentDir(t *testing.T) {
	count, err := AddMissingExecMode("/nonexistent/path")
	if err != nil {
		t.Fatalf("AddMissingExecMode failed: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 files modified, got %d", count)
	}
}

func TestRepairPipelineAddsMissingExecMode(t *testing.T) {
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "no-mode.md"), []byte("# No Mode\n\nBody\n"), 0644)

	labelsAdded, execModesAdded, stripped, stuckCount, testResultsPromoted, invalidExecModes, checksumsAdded, err := RepairPipeline(dir, true)
	if err != nil {
		t.Fatalf("RepairPipeline failed: %v", err)
	}
	if labelsAdded != 1 {
		t.Errorf("expected 1 label added, got %d", labelsAdded)
	}
	if execModesAdded != 1 {
		t.Errorf("expected 1 exec mode added, got %d", execModesAdded)
	}
	if stripped != 0 {
		t.Errorf("expected 0 stripped, got %d", stripped)
	}
	if stuckCount != 0 {
		t.Errorf("expected 0 stuck count, got %d", stuckCount)
	}
	if checksumsAdded != 1 {
		t.Errorf("expected 1 checksum added, got %d", checksumsAdded)
	}
	if testResultsPromoted != 0 {
		t.Errorf("expected 0 promoted, got %d", testResultsPromoted)
	}
	if invalidExecModes != 0 {
		t.Errorf("expected 0 invalid exec modes, got %d", invalidExecModes)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "no-mode.md"))
	if !strings.Contains(string(data), "Execution mode: AFK-only") {
		t.Error("expected Execution mode: AFK-only to be added")
	}
	if !strings.Contains(string(data), "Status: ready-for-agent") {
		t.Error("expected Status: ready-for-agent to be added")
	}
	if !strings.Contains(string(data), "Checksum:") {
		t.Error("expected Checksum line to be added")
	}
}

func TestFindIssueByNumTodoHITLOnly(t *testing.T) {
	dir := t.TempDir()

	path := filepath.Join(dir, "hitl.md")
	content := "# HITL Issue\n\nGitHub: #42\nExecution mode: HITL-only\n"
	os.WriteFile(path, []byte(content), 0644)

	state := &PipelineState{
		TodoFiles: []string{path},
	}

	_, _, err := FindIssueByNum(state, 42)
	if err == nil {
		t.Fatal("expected error for HITL-only issue")
	}
	if !errors.Is(err, ErrIssueNonAFK) {
		t.Errorf("expected ErrIssueNonAFK, got: %v", err)
	}
}

func TestFindIssueByNumTodoCombo(t *testing.T) {
	dir := t.TempDir()

	path := filepath.Join(dir, "combo.md")
	content := "# Combo Issue\n\nGitHub: #42\nExecution mode: Combo\n"
	os.WriteFile(path, []byte(content), 0644)

	state := &PipelineState{
		TodoFiles: []string{path},
	}

	_, _, err := FindIssueByNum(state, 42)
	if err == nil {
		t.Fatal("expected error for Combo issue")
	}
	if !errors.Is(err, ErrIssueNonAFK) {
		t.Errorf("expected ErrIssueNonAFK, got: %v", err)
	}
}

func TestFindIssueByNumReadyForAgentHITLOnly(t *testing.T) {
	dir := t.TempDir()

	path := filepath.Join(dir, "hitl.md")
	content := "# HITL Issue\n\nGitHub: #42\nExecution mode: HITL-only\n"
	os.WriteFile(path, []byte(content), 0644)

	state := &PipelineState{
		ReadyForAgentFiles: []string{path},
	}

	_, _, err := FindIssueByNum(state, 42)
	if err == nil {
		t.Fatal("expected error for HITL-only issue in ready-for-agent")
	}
	if !errors.Is(err, ErrIssueNonAFK) {
		t.Errorf("expected ErrIssueNonAFK, got: %v", err)
	}
}

func TestFindIssueByNumReadyForAgentCombo(t *testing.T) {
	dir := t.TempDir()

	path := filepath.Join(dir, "combo.md")
	content := "# Combo Issue\n\nGitHub: #42\nExecution mode: Combo\n"
	os.WriteFile(path, []byte(content), 0644)

	state := &PipelineState{
		ReadyForAgentFiles: []string{path},
	}

	_, _, err := FindIssueByNum(state, 42)
	if err == nil {
		t.Fatal("expected error for Combo issue in ready-for-agent")
	}
	if !errors.Is(err, ErrIssueNonAFK) {
		t.Errorf("expected ErrIssueNonAFK, got: %v", err)
	}
}

func TestFindIssueByNumTestReadyNonAFK(t *testing.T) {
	dir := t.TempDir()

	path := filepath.Join(dir, "hitl.md")
	content := "# HITL Issue\n\nGitHub: #42\nExecution mode: HITL-only\n"
	os.WriteFile(path, []byte(content), 0644)

	state := &PipelineState{
		TestReadyFiles: []string{path},
	}

	file, role, err := FindIssueByNum(state, 42)
	if err != nil {
		t.Fatalf("unexpected error for HITL-only issue in test-ready: %v", err)
	}
	if file == nil {
		t.Fatal("expected issue file for test-ready HITL-only issue")
	}
	if role != RoleTest {
		t.Errorf("expected RoleTest, got %q", role)
	}
}

func TestFindIssueByNumTodoAFKOnly(t *testing.T) {
	dir := t.TempDir()

	path := filepath.Join(dir, "afk.md")
	content := "# AFK Issue\n\nGitHub: #42\nExecution mode: AFK-only\n"
	os.WriteFile(path, []byte(content), 0644)

	state := &PipelineState{
		TodoFiles: []string{path},
	}

	file, role, err := FindIssueByNum(state, 42)
	if err != nil {
		t.Fatalf("unexpected error for AFK-only issue: %v", err)
	}
	if file == nil {
		t.Fatal("expected issue file for AFK-only issue")
	}
	if role != RoleImplement {
		t.Errorf("expected RoleImplement, got %q", role)
	}
}

func TestFindIssueByNumReadyForAgentAFKOnly(t *testing.T) {
	dir := t.TempDir()

	path := filepath.Join(dir, "afk.md")
	content := "# AFK Issue\n\nGitHub: #42\nExecution mode: AFK-only\n"
	os.WriteFile(path, []byte(content), 0644)

	state := &PipelineState{
		ReadyForAgentFiles: []string{path},
	}

	file, role, err := FindIssueByNum(state, 42)
	if err != nil {
		t.Fatalf("unexpected error for AFK-only issue in ready-for-agent: %v", err)
	}
	if file == nil {
		t.Fatal("expected issue file for AFK-only issue")
	}
	if role != RoleImplement {
		t.Errorf("expected RoleImplement, got %q", role)
	}
}

func TestRepairPipelineFlagsInvalidExecModes(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "test-ready"), 0755)

	os.WriteFile(filepath.Join(dir, "bad.md"), []byte("# Bad\n\nStatus: ready-for-agent\nExecution mode: AFK_onLy\n\nBody\n"), 0644)
	os.WriteFile(filepath.Join(dir, "test-ready", "also-bad.md"), []byte("# Also Bad\n\nStatus: ready-for-agent\nExecution mode: Combo!\n\nBody\n"), 0644)
	os.WriteFile(filepath.Join(dir, "good.md"), []byte("# Good\n\nStatus: ready-for-agent\nExecution mode: AFK-only\n\nBody\n"), 0644)

	labelsAdded, execModesAdded, stripped, stuckCount, testResultsPromoted, invalidExecModes, checksumsAdded, err := RepairPipeline(dir, true)
	if err != nil {
		t.Fatalf("RepairPipeline failed: %v", err)
	}
	if labelsAdded != 0 {
		t.Errorf("expected 0 labels added, got %d", labelsAdded)
	}
	if execModesAdded != 0 {
		t.Errorf("expected 0 exec modes added, got %d", execModesAdded)
	}
	if stripped != 0 {
		t.Errorf("expected 0 stripped, got %d", stripped)
	}
	if stuckCount != 0 {
		t.Errorf("expected 0 stuck count, got %d", stuckCount)
	}
	if testResultsPromoted != 0 {
		t.Errorf("expected 0 promoted, got %d", testResultsPromoted)
	}
	if invalidExecModes != 2 {
		t.Errorf("expected 2 invalid exec modes, got %d", invalidExecModes)
	}
	if checksumsAdded != 3 {
		t.Errorf("expected 3 checksums added, got %d", checksumsAdded)
	}

	// Verify files were NOT modified (no auto-fix)
	data, _ := os.ReadFile(filepath.Join(dir, "bad.md"))
	if !strings.Contains(string(data), "Execution mode: AFK_onLy") {
		t.Error("expected bad.md to retain its original invalid value (no auto-fix)")
	}
	data, _ = os.ReadFile(filepath.Join(dir, "test-ready", "also-bad.md"))
	if !strings.Contains(string(data), "Execution mode: Combo!") {
		t.Error("expected also-bad.md to retain its original invalid value (no auto-fix)")
	}
}

func TestQuarantineDuplicateGitHubNumsEmpty(t *testing.T) {
	state := &PipelineState{}
	quarantined, issues := QuarantineDuplicateGitHubNums(state)
	if quarantined != 0 {
		t.Errorf("expected 0 quarantined, got %d", quarantined)
	}
	if len(issues) != 0 {
		t.Errorf("expected 0 issues, got %d", len(issues))
	}
}

func TestQuarantineDuplicateGitHubNumsNoDuplicates(t *testing.T) {
	dir := t.TempDir()
	setupIssueFile(t, dir, "", "one.md", "# One\n\nGitHub: #10\n")
	setupIssueFile(t, dir, "test-ready", "two.md", "# Two\n\nGitHub: #20\n")
	setupIssueFile(t, dir, "done", "three.md", "# Three\n\nGitHub: #30\n")

	state := &PipelineState{
		TodoFiles:      []string{filepath.Join(dir, "one.md")},
		TestReadyFiles: []string{filepath.Join(dir, "test-ready", "two.md")},
		DoneFiles:      []string{filepath.Join(dir, "done", "three.md")},
	}

	quarantined, issues := QuarantineDuplicateGitHubNums(state)
	if quarantined != 0 {
		t.Errorf("expected 0 quarantined, got %d", quarantined)
	}
	if len(issues) != 0 {
		t.Errorf("expected 0 issues, got %d", len(issues))
	}
}

func TestQuarantineDuplicateGitHubNumsTwoStates(t *testing.T) {
	dir := t.TempDir()
	todoPath := setupIssueFile(t, dir, "", "dup1.md", "# Dup Alpha\n\nGitHub: #42\n")
	readyPath := setupIssueFile(t, dir, "test-ready", "dup2.md", "# Dup Beta\n\nGitHub: #42\n")

	state := &PipelineState{
		TodoFiles:      []string{todoPath},
		TestReadyFiles: []string{readyPath},
	}

	quarantined, issues := QuarantineDuplicateGitHubNums(state)
	if quarantined != 1 {
		t.Errorf("expected 1 quarantined, got %d", quarantined)
	}
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
	if !strings.Contains(issues[0].Message, "#42") {
		t.Errorf("expected message to mention #42, got %q", issues[0].Message)
	}
	if issues[0].Severity != SeverityWarning {
		t.Errorf("expected severity %q, got %q", SeverityWarning, issues[0].Severity)
	}

	// Lower-stage (todo) file removed, higher-stage (test-ready) kept
	if _, err := os.Stat(todoPath); !os.IsNotExist(err) {
		t.Errorf("expected todo file to be removed, still exists")
	}
	if _, err := os.Stat(readyPath); os.IsNotExist(err) {
		t.Errorf("expected test-ready canonical file to remain, was removed")
	}

	entries, err := os.ReadDir(filepath.Join(dir, ".quarantine"))
	if err != nil {
		t.Fatalf("failed to list quarantine dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry in quarantine dir, got %d", len(entries))
	}
	if !strings.HasSuffix(entries[0].Name(), "_dup1.md") {
		t.Errorf("expected quarantine entry matching *_dup1.md, got %q", entries[0].Name())
	}

	// PipelineState updated
	if len(state.TodoFiles) != 0 {
		t.Errorf("expected 0 todo files, got %d", len(state.TodoFiles))
	}
	if len(state.TestReadyFiles) != 1 {
		t.Errorf("expected 1 test-ready file, got %d", len(state.TestReadyFiles))
	}
	if len(state.QuarantineFiles) != 1 {
		t.Errorf("expected 1 quarantine file, got %d", len(state.QuarantineFiles))
	}
}

func TestQuarantineDuplicateGitHubNumsSameState(t *testing.T) {
	dir := t.TempDir()
	aPath := setupIssueFile(t, dir, "", "a.md", "# A\n\nGitHub: #42\n")
	bPath := setupIssueFile(t, dir, "", "b.md", "# B\n\nGitHub: #42\n")

	state := &PipelineState{
		TodoFiles: []string{aPath, bPath},
	}

	quarantined, issues := QuarantineDuplicateGitHubNums(state)
	if quarantined != 1 {
		t.Errorf("expected 1 quarantined, got %d", quarantined)
	}
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
	if !strings.Contains(issues[0].Message, "#42") {
		t.Errorf("expected message to mention #42, got %q", issues[0].Message)
	}

	// One of the two files should be in quarantine
	entries, err := os.ReadDir(filepath.Join(dir, ".quarantine"))
	if err != nil {
		t.Fatalf("failed to list quarantine dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry in quarantine, got %d", len(entries))
	}

	// PipelineState: one todo remaining, one quarantined
	if len(state.TodoFiles) != 1 {
		t.Errorf("expected 1 todo file remaining, got %d", len(state.TodoFiles))
	}
	if len(state.QuarantineFiles) != 1 {
		t.Errorf("expected 1 quarantine file, got %d", len(state.QuarantineFiles))
	}
}

func TestQuarantineDuplicateGitHubNumsThreeStates(t *testing.T) {
	dir := t.TempDir()
	todoPath := setupIssueFile(t, dir, "", "a.md", "# A\n\nGitHub: #7\n")
	readyPath := setupIssueFile(t, dir, "test-ready", "b.md", "# B\n\nGitHub: #7\n")
	donePath := setupIssueFile(t, dir, "done", "c.md", "# C\n\nGitHub: #7\n")

	state := &PipelineState{
		TodoFiles:      []string{todoPath},
		TestReadyFiles: []string{readyPath},
		DoneFiles:      []string{donePath},
	}

	quarantined, issues := QuarantineDuplicateGitHubNums(state)
	if quarantined != 2 {
		t.Errorf("expected 2 quarantined (todo + test-ready), got %d", quarantined)
	}
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}

	// Lower-stage files removed, canonical (done) kept
	if _, err := os.Stat(todoPath); !os.IsNotExist(err) {
		t.Errorf("expected todo file to be removed")
	}
	if _, err := os.Stat(readyPath); !os.IsNotExist(err) {
		t.Errorf("expected test-ready file to be removed")
	}
	if _, err := os.Stat(donePath); os.IsNotExist(err) {
		t.Errorf("expected done file to remain")
	}

	// PipelineState updated
	if len(state.TodoFiles) != 0 {
		t.Errorf("expected 0 todo files, got %d", len(state.TodoFiles))
	}
	if len(state.TestReadyFiles) != 0 {
		t.Errorf("expected 0 test-ready files, got %d", len(state.TestReadyFiles))
	}
	if len(state.DoneFiles) != 1 {
		t.Errorf("expected 1 done file, got %d", len(state.DoneFiles))
	}
	if len(state.QuarantineFiles) != 2 {
		t.Errorf("expected 2 quarantine files, got %d", len(state.QuarantineFiles))
	}
}

func TestQuarantineDuplicateGitHubNumsMultipleGroups(t *testing.T) {
	dir := t.TempDir()
	aPath := setupIssueFile(t, dir, "", "a.md", "# A\n\nGitHub: #10\n")
	a2Path := setupIssueFile(t, dir, "test-ready", "a2.md", "# A2\n\nGitHub: #10\n")
	bPath := setupIssueFile(t, dir, "", "b.md", "# B\n\nGitHub: #20\n")
	b2Path := setupIssueFile(t, dir, "done", "b2.md", "# B2\n\nGitHub: #20\n")

	state := &PipelineState{
		TodoFiles:      []string{aPath, bPath},
		TestReadyFiles: []string{a2Path},
		DoneFiles:      []string{b2Path},
	}

	quarantined, issues := QuarantineDuplicateGitHubNums(state)
	if quarantined != 2 {
		t.Errorf("expected 2 quarantined, got %d", quarantined)
	}
	if len(issues) != 2 {
		t.Fatalf("expected 2 issues, got %d", len(issues))
	}

	// Group #10: todo copy quarantined, test-ready kept
	if _, err := os.Stat(aPath); !os.IsNotExist(err) {
		t.Errorf("expected first todo file to be removed")
	}
	if _, err := os.Stat(a2Path); os.IsNotExist(err) {
		t.Errorf("expected test-ready file for #10 to remain")
	}

	// Group #20: todo copy quarantined, done kept
	if _, err := os.Stat(bPath); !os.IsNotExist(err) {
		t.Errorf("expected second todo file to be removed")
	}
	if _, err := os.Stat(b2Path); os.IsNotExist(err) {
		t.Errorf("expected done file for #20 to remain")
	}

	if len(state.QuarantineFiles) != 2 {
		t.Errorf("expected 2 quarantine files, got %d", len(state.QuarantineFiles))
	}
}

func TestQuarantineDuplicateGitHubNumsIgnoresZero(t *testing.T) {
	dir := t.TempDir()
	setupIssueFile(t, dir, "", "a.md", "# A\n\n")
	setupIssueFile(t, dir, "test-ready", "b.md", "# B\n\n")

	state := &PipelineState{
		TodoFiles:      []string{filepath.Join(dir, "a.md")},
		TestReadyFiles: []string{filepath.Join(dir, "test-ready", "b.md")},
	}

	quarantined, issues := QuarantineDuplicateGitHubNums(state)
	if quarantined != 0 {
		t.Errorf("expected 0 quarantined, got %d", quarantined)
	}
	if len(issues) != 0 {
		t.Errorf("expected 0 issues, got %d", len(issues))
	}
}

func TestQuarantineDuplicateGitHubNumsSkipsMalformedFiles(t *testing.T) {
	dir := t.TempDir()
	setupIssueFile(t, dir, "", "a.md", "not-a-header\n")
	setupIssueFile(t, dir, "test-ready", "b.md", "still-no-header\n")

	state := &PipelineState{
		TodoFiles:      []string{filepath.Join(dir, "a.md")},
		TestReadyFiles: []string{filepath.Join(dir, "test-ready", "b.md")},
	}

	quarantined, issues := QuarantineDuplicateGitHubNums(state)
	if quarantined != 0 {
		t.Errorf("expected 0 quarantined, got %d", quarantined)
	}
	if len(issues) != 0 {
		t.Errorf("expected 0 issues, got %d", len(issues))
	}
}

func TestQuarantineDuplicateTitlesEmpty(t *testing.T) {
	state := &PipelineState{}
	quarantined, issues := QuarantineDuplicateTitles(state)
	if quarantined != 0 {
		t.Errorf("expected 0 quarantined, got %d", quarantined)
	}
	if len(issues) != 0 {
		t.Errorf("expected 0 issues, got %d", len(issues))
	}
}

func TestQuarantineDuplicateTitlesNoDuplicates(t *testing.T) {
	dir := t.TempDir()
	setupIssueFile(t, dir, "", "one.md", "# One\n")
	setupIssueFile(t, dir, "test-ready", "two.md", "# Two\n")
	setupIssueFile(t, dir, "done", "three.md", "# Three\n")

	state := &PipelineState{
		TodoFiles:      []string{filepath.Join(dir, "one.md")},
		TestReadyFiles: []string{filepath.Join(dir, "test-ready", "two.md")},
		DoneFiles:      []string{filepath.Join(dir, "done", "three.md")},
	}

	quarantined, issues := QuarantineDuplicateTitles(state)
	if quarantined != 0 {
		t.Errorf("expected 0 quarantined, got %d", quarantined)
	}
	if len(issues) != 0 {
		t.Errorf("expected 0 issues, got %d", len(issues))
	}
}

func TestQuarantineDuplicateTitlesTwoStates(t *testing.T) {
	dir := t.TempDir()
	todoPath := setupIssueFile(t, dir, "", "dup1.md", "# Duplicate Title\n")
	readyPath := setupIssueFile(t, dir, "test-ready", "dup2.md", "# Duplicate Title\n")

	state := &PipelineState{
		TodoFiles:      []string{todoPath},
		TestReadyFiles: []string{readyPath},
	}

	quarantined, issues := QuarantineDuplicateTitles(state)
	if quarantined != 1 {
		t.Errorf("expected 1 quarantined, got %d", quarantined)
	}
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
	if !strings.Contains(issues[0].Message, "Duplicate Title") {
		t.Errorf("expected message to mention title, got %q", issues[0].Message)
	}
	if issues[0].Severity != SeverityWarning {
		t.Errorf("expected severity %q, got %q", SeverityWarning, issues[0].Severity)
	}

	// Lower-stage (todo) file removed
	if _, err := os.Stat(todoPath); !os.IsNotExist(err) {
		t.Errorf("expected todo file to be removed, still exists")
	}
	if _, err := os.Stat(readyPath); os.IsNotExist(err) {
		t.Errorf("expected test-ready canonical file to remain, was removed")
	}

	entries, err := os.ReadDir(filepath.Join(dir, ".quarantine"))
	if err != nil {
		t.Fatalf("failed to list quarantine dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry in quarantine dir, got %d", len(entries))
	}

	// PipelineState updated
	if len(state.TodoFiles) != 0 {
		t.Errorf("expected 0 todo files, got %d", len(state.TodoFiles))
	}
	if len(state.TestReadyFiles) != 1 {
		t.Errorf("expected 1 test-ready file, got %d", len(state.TestReadyFiles))
	}
	if len(state.QuarantineFiles) != 1 {
		t.Errorf("expected 1 quarantine file, got %d", len(state.QuarantineFiles))
	}
}

func TestQuarantineDuplicateTitlesSameState(t *testing.T) {
	dir := t.TempDir()
	aPath := setupIssueFile(t, dir, "", "a.md", "# Same Title\n")
	bPath := setupIssueFile(t, dir, "", "b.md", "# Same Title\n")

	state := &PipelineState{
		TodoFiles: []string{aPath, bPath},
	}

	quarantined, issues := QuarantineDuplicateTitles(state)
	if quarantined != 1 {
		t.Errorf("expected 1 quarantined, got %d", quarantined)
	}
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}

	entries, err := os.ReadDir(filepath.Join(dir, ".quarantine"))
	if err != nil {
		t.Fatalf("failed to list quarantine dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry in quarantine, got %d", len(entries))
	}

	if len(state.TodoFiles) != 1 {
		t.Errorf("expected 1 todo file remaining, got %d", len(state.TodoFiles))
	}
	if len(state.QuarantineFiles) != 1 {
		t.Errorf("expected 1 quarantine file, got %d", len(state.QuarantineFiles))
	}
}

func TestQuarantineDuplicateTitlesThreeStates(t *testing.T) {
	dir := t.TempDir()
	todoPath := setupIssueFile(t, dir, "", "a.md", "# Common Title\n")
	readyPath := setupIssueFile(t, dir, "test-ready", "b.md", "# Common Title\n")
	donePath := setupIssueFile(t, dir, "done", "c.md", "# Common Title\n")

	state := &PipelineState{
		TodoFiles:      []string{todoPath},
		TestReadyFiles: []string{readyPath},
		DoneFiles:      []string{donePath},
	}

	quarantined, issues := QuarantineDuplicateTitles(state)
	if quarantined != 2 {
		t.Errorf("expected 2 quarantined, got %d", quarantined)
	}
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}

	// Lower-stage files removed, canonical (done) kept
	if _, err := os.Stat(todoPath); !os.IsNotExist(err) {
		t.Errorf("expected todo file to be removed")
	}
	if _, err := os.Stat(readyPath); !os.IsNotExist(err) {
		t.Errorf("expected test-ready file to be removed")
	}
	if _, err := os.Stat(donePath); os.IsNotExist(err) {
		t.Errorf("expected done file to remain")
	}

	if len(state.TodoFiles) != 0 {
		t.Errorf("expected 0 todo files, got %d", len(state.TodoFiles))
	}
	if len(state.TestReadyFiles) != 0 {
		t.Errorf("expected 0 test-ready files, got %d", len(state.TestReadyFiles))
	}
	if len(state.DoneFiles) != 1 {
		t.Errorf("expected 1 done file, got %d", len(state.DoneFiles))
	}
	if len(state.QuarantineFiles) != 2 {
		t.Errorf("expected 2 quarantine files, got %d", len(state.QuarantineFiles))
	}
}

func TestQuarantineDuplicateTitlesMultipleGroups(t *testing.T) {
	dir := t.TempDir()
	aPath := setupIssueFile(t, dir, "", "a.md", "# Alpha\n")
	a2Path := setupIssueFile(t, dir, "test-ready", "a2.md", "# Alpha\n")
	bPath := setupIssueFile(t, dir, "", "b.md", "# Beta\n")
	b2Path := setupIssueFile(t, dir, "done", "b2.md", "# Beta\n")

	state := &PipelineState{
		TodoFiles:      []string{aPath, bPath},
		TestReadyFiles: []string{a2Path},
		DoneFiles:      []string{b2Path},
	}

	quarantined, issues := QuarantineDuplicateTitles(state)
	if quarantined != 2 {
		t.Errorf("expected 2 quarantined, got %d", quarantined)
	}
	if len(issues) != 2 {
		t.Fatalf("expected 2 issues, got %d", len(issues))
	}

	// Group "Alpha": todo quarantined, test-ready kept
	if _, err := os.Stat(aPath); !os.IsNotExist(err) {
		t.Errorf("expected first todo file to be removed")
	}
	if _, err := os.Stat(a2Path); os.IsNotExist(err) {
		t.Errorf("expected test-ready file for Alpha to remain")
	}

	// Group "Beta": todo quarantined, done kept
	if _, err := os.Stat(bPath); !os.IsNotExist(err) {
		t.Errorf("expected second todo file to be removed")
	}
	if _, err := os.Stat(b2Path); os.IsNotExist(err) {
		t.Errorf("expected done file for Beta to remain")
	}

	if len(state.QuarantineFiles) != 2 {
		t.Errorf("expected 2 quarantine files, got %d", len(state.QuarantineFiles))
	}
}

func TestQuarantineDuplicateTitlesIgnoresEmptyTitle(t *testing.T) {
	dir := t.TempDir()
	setupIssueFile(t, dir, "", "a.md", "no title here\n")
	setupIssueFile(t, dir, "test-ready", "b.md", "still no title\n")

	state := &PipelineState{
		TodoFiles:      []string{filepath.Join(dir, "a.md")},
		TestReadyFiles: []string{filepath.Join(dir, "test-ready", "b.md")},
	}

	quarantined, issues := QuarantineDuplicateTitles(state)
	if quarantined != 0 {
		t.Errorf("expected 0 quarantined, got %d", quarantined)
	}
	if len(issues) != 0 {
		t.Errorf("expected 0 issues, got %d", len(issues))
	}
}

func TestQuarantineDuplicateTitlesSkipsMalformedFiles(t *testing.T) {
	dir := t.TempDir()
	setupIssueFile(t, dir, "", "a.md", "garbage\n")
	setupIssueFile(t, dir, "test-ready", "b.md", "more garbage\n")

	state := &PipelineState{
		TodoFiles:      []string{filepath.Join(dir, "a.md")},
		TestReadyFiles: []string{filepath.Join(dir, "test-ready", "b.md")},
	}

	quarantined, issues := QuarantineDuplicateTitles(state)
	if quarantined != 0 {
		t.Errorf("expected 0 quarantined, got %d", quarantined)
	}
	if len(issues) != 0 {
		t.Errorf("expected 0 issues, got %d", len(issues))
	}
}

func TestComputeChecksum(t *testing.T) {
	content := "hello world"
	cs := computeChecksumFromContent(content)
	if len(cs) != 64 {
		t.Errorf("expected 64-char hex checksum, got %d chars", len(cs))
	}
	if cs == "" {
		t.Error("expected non-empty checksum")
	}
}

func TestComputeChecksumExcludesItself(t *testing.T) {
	content := "# Title\nGitHub: #42\nChecksum: abc\n\nBody\n"
	cs := computeChecksumFromContent(content)

	content2 := "# Title\nGitHub: #42\nChecksum: xyz\n\nBody\n"
	cs2 := computeChecksumFromContent(content2)

	if cs != cs2 {
		t.Error("checksums should be equal when only Checksum header differs")
	}
}

func TestComputeChecksumChangesWithContent(t *testing.T) {
	a := computeChecksumFromContent("# Title\n\nBody A\n")
	b := computeChecksumFromContent("# Title\n\nBody B\n")
	if a == b {
		t.Error("checksums should differ when content differs")
	}
}

func TestSetAndVerifyChecksum(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")
	content := "# Test Issue\nGitHub: #42\nStatus: ready-for-agent\nExecution mode: AFK-only\n\n## What to build\n\nStuff\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	if err := SetChecksum(path); err != nil {
		t.Fatalf("SetChecksum failed: %v", err)
	}

	valid, err := VerifyChecksum(path)
	if err != nil {
		t.Fatalf("VerifyChecksum failed: %v", err)
	}
	if !valid {
		t.Error("expected valid checksum after SetChecksum")
	}

	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "Checksum:") {
		t.Error("expected Checksum header line in file")
	}

	f, err := ParseIssueFile(path)
	if err != nil {
		t.Fatalf("ParseIssueFile failed: %v", err)
	}
	if f.Checksum == "" {
		t.Error("expected parsed Checksum field to be non-empty")
	}
	if len(f.Checksum) != 64 {
		t.Errorf("expected 64-char hex checksum, got %d chars", len(f.Checksum))
	}
}

func TestVerifyChecksumDetectsTampering(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")
	content := "# Test Issue\n\nBody\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	if err := SetChecksum(path); err != nil {
		t.Fatalf("SetChecksum failed: %v", err)
	}

	data, _ := os.ReadFile(path)
	tampered := strings.ReplaceAll(string(data), "Body", "Tampered")
	if err := os.WriteFile(path, []byte(tampered), 0644); err != nil {
		t.Fatalf("write tampered failed: %v", err)
	}

	valid, err := VerifyChecksum(path)
	if err != nil {
		t.Fatalf("VerifyChecksum failed: %v", err)
	}
	if valid {
		t.Error("expected invalid checksum after tampering")
	}
}

func TestVerifyChecksumNoChecksumField(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")
	content := "# Test Issue\n\nBody\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	valid, err := VerifyChecksum(path)
	if err != nil {
		t.Fatalf("VerifyChecksum failed: %v", err)
	}
	if valid {
		t.Error("expected false when no Checksum field present")
	}
}

func TestSetChecksumReplacesExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")
	content := "# Test Issue\nChecksum: oldhash\n\nBody\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	if err := SetChecksum(path); err != nil {
		t.Fatalf("SetChecksum failed: %v", err)
	}

	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "oldhash") {
		t.Error("expected old checksum to be replaced")
	}

	if !strings.Contains(string(data), "Checksum:") {
		t.Error("expected Checksum line to exist after SetChecksum")
	}
}

func TestAddMissingChecksums(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "test-ready"), 0755)

	os.WriteFile(filepath.Join(dir, "todo.md"), []byte("# Todo\n\nBody\n"), 0644)
	os.WriteFile(filepath.Join(dir, "test-ready", "ready.md"), []byte("# Ready\n\nBody\n"), 0644)

	n, err := AddMissingChecksums(dir, true)
	if err != nil {
		t.Fatalf("AddMissingChecksums failed: %v", err)
	}
	if n != 2 {
		t.Errorf("expected 2 checksums added, got %d", n)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "todo.md"))
	if !strings.Contains(string(data), "Checksum:") {
		t.Error("expected Checksum in todo.md")
	}
}

func TestAddMissingChecksumsDisabled(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "test-ready"), 0755)

	os.WriteFile(filepath.Join(dir, "todo.md"), []byte("# Todo\n\nBody\n"), 0644)
	os.WriteFile(filepath.Join(dir, "test-ready", "ready.md"), []byte("# Ready\n\nBody\n"), 0644)

	n, err := AddMissingChecksums(dir, false)
	if err != nil {
		t.Fatalf("AddMissingChecksums failed: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 checksums added when disabled, got %d", n)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "todo.md"))
	if strings.Contains(string(data), "Checksum:") {
		t.Error("expected no Checksum in todo.md when disabled")
	}
}

func TestPreFlightCheckChecksumsDisabled(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "issue.md")
	content := "# Test Issue\nChecksum: abc\n\nBody\n"
	os.WriteFile(path, []byte(content), 0644)

	state := &PipelineState{
		TodoFiles: []string{path},
	}

	issues := PreFlightCheck(state, false, false)
	for _, i := range issues {
		if strings.Contains(i.Message, "checksum") {
			t.Errorf("expected no checksum issues when disabled, got: %s", i.Message)
		}
	}
}

func TestPreFlightCheckChecksumsEnabledDetectsMismatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "issue.md")
	content := "# Test Issue\nChecksum: abc\n\nBody\n"
	os.WriteFile(path, []byte(content), 0644)

	state := &PipelineState{
		TodoFiles: []string{path},
	}

	issues := PreFlightCheck(state, false, true)
	found := false
	for _, i := range issues {
		if strings.Contains(i.Message, "checksum") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected checksum mismatch issue when enabled")
	}
}

func TestVerifyChecksumsAllValid(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "test-ready"), 0755)

	os.WriteFile(filepath.Join(dir, "a.md"), []byte("# A\n\nBody\n"), 0644)
	os.WriteFile(filepath.Join(dir, "test-ready", "b.md"), []byte("# B\n\nBody\n"), 0644)

	// Add checksums
	if _, err := AddMissingChecksums(dir, true); err != nil {
		t.Fatalf("AddMissingChecksums: %v", err)
	}

	results, err := VerifyChecksums(dir)
	if err != nil {
		t.Fatalf("VerifyChecksums: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for _, r := range results {
		if r.Err != nil {
			t.Errorf("unexpected error for %s: %v", r.FilePath, r.Err)
		}
		if !r.Valid {
			t.Errorf("expected valid checksum for %s", r.FilePath)
		}
	}
}

func TestVerifyChecksumsDetectsTampering(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.md")
	content := "# A\n\nBody\n"
	os.WriteFile(path, []byte(content), 0644)

	if _, err := AddMissingChecksums(dir, true); err != nil {
		t.Fatalf("AddMissingChecksums: %v", err)
	}

	data, _ := os.ReadFile(path)
	tampered := strings.ReplaceAll(string(data), "Body", "Tampered")
	os.WriteFile(path, []byte(tampered), 0644)

	results, err := VerifyChecksums(dir)
	if err != nil {
		t.Fatalf("VerifyChecksums: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Valid {
		t.Error("expected invalid checksum after tampering")
	}
}

func TestVerifyChecksumsSkipsFilesWithoutChecksum(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.md"), []byte("# A\n\nBody\n"), 0644)

	results, err := VerifyChecksums(dir)
	if err != nil {
		t.Fatalf("VerifyChecksums: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for files without checksums, got %d", len(results))
	}
}

func TestVerifyChecksumsEmptyDir(t *testing.T) {
	dir := t.TempDir()
	results, err := VerifyChecksums(dir)
	if err != nil {
		t.Fatalf("VerifyChecksums: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for empty dir, got %d", len(results))
	}
}
