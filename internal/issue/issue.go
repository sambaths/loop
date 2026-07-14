package issue

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode"

	"github.com/sambaths/loop/internal/agent"
)

const MaxRetries = 5

// stageRank returns the priority of a pipeline stage for canonical file
// selection. Higher rank = later stage = preferred.
func stageRank(s State) int {
	switch s {
	case StateDone:
		return 4
	case StateTestReady:
		return 3
	case StateReadyForAgent:
		return 2
	case StateTodo:
		return 1
	default:
		return 0
	}
}

// PickCanonical selects the canonical file from a set of files sharing the same
// basename across pipeline stages. It prefers later pipeline stages
// (done > test-ready > todo), then newer modification time within the same
// stage. Returns the path of the canonical file.
func PickCanonical(paths []string) (string, error) {
	if len(paths) == 0 {
		return "", fmt.Errorf("cannot pick canonical from empty list")
	}
	if len(paths) == 1 {
		return paths[0], nil
	}

	sort.Slice(paths, func(i, j int) bool {
		si := stageRank(StateFromPath(paths[i]))
		sj := stageRank(StateFromPath(paths[j]))
		if si != sj {
			return si > sj
		}
		fi, err := os.Stat(paths[i])
		if err != nil {
			return false
		}
		fj, err := os.Stat(paths[j])
		if err != nil {
			return false
		}
		return fi.ModTime().After(fj.ModTime())
	})
	return paths[0], nil
}

// errMissingHeading is returned by Read when a file's first line is not a level-1 heading.
var errMissingHeading = fmt.Errorf("missing title heading: first line must start with '# '")

// sectionName extracts the section name from a markdown heading line (any level).
// Returns empty string if the line is not a heading.
// "## What to build" → "What to build"
// "# What to build" → "What to build"
// "### What to build" → "What to build"
func sectionName(line string) string {
	trimmed := strings.TrimLeftFunc(line, unicode.IsSpace)
	count := 0
	for count < len(trimmed) && trimmed[count] == '#' {
		count++
	}
	if count == 0 || count > 6 || count >= len(trimmed) || trimmed[count] != ' ' {
		return ""
	}
	return strings.TrimSpace(trimmed[count+1:])
}

type State string

const (
	StateUnknown       State = ""
	StateTodo          State = "todo"
	StateTestReady     State = "test-ready"
	StateReadyForAgent State = "ready-for-agent"
	StateDone          State = "done"
	StateQuarantine    State = ".quarantine"
	StateUnable        State = "unable"
)

type Issue struct {
	FilePath  string
	Title     string
	Body      string
	State     State
	GitHubNum int
}

type IssueHeader struct {
	Title     string
	GitHubNum int
	ExecMode  string
	Branch    string
	Type      string
}

type IssueFile struct {
	Title     string
	GitHubNum int
	ExecMode  string
	Branch    string
	Checksum  string
	Type      string
	Retries   int
	FilePath  string
	State     State
}

type IssueBody struct {
	Parent             string
	WhatToBuild        string
	UserStoriesCovered string
	AcceptanceCriteria string
	UATPlan            string
	BlockedBy          string
	Comments           string
}

// RequiredSections lists the six body sections that every issue file MUST
// contain for proper agent processing.
var RequiredSections = []string{
	"What to build",
	"User stories covered",
	"Acceptance criteria",
	"UAT plan",
	"Blocked by",
}

// DisallowedSections maps issue states to section names that must not appear
// in files in that state. UAT Results must not be pre-populated in todo or
// test-ready files — it is added only by the testing subagent during
// validation.
var DisallowedSections = map[State][]string{
	StateTodo:          {"UAT Results"},
	StateTestReady:     {"UAT Results"},
	StateReadyForAgent: {"Test Results", "UAT Results"},
}

// KnownSections is the complete set of all recognized body sections in an issue file.
var KnownSections = []string{
	"Parent",
	"What to build",
	"User stories covered",
	"Acceptance criteria",
	"UAT plan",
	"UAT Results",
	"UAT Process",
	"Defect Tracker",
	"Blocked by",
	"Comments",
}

type PipelineState struct {
	TodoFiles          []string
	TestReadyFiles     []string
	ReadyForAgentFiles []string
	DoneFiles          []string
	QuarantineFiles    []string
	UnableFiles        []string
	Files              map[string]*IssueFile
}

func (ps *PipelineState) Counts() map[State]int {
	return map[State]int{
		StateTodo:          len(ps.TodoFiles),
		StateTestReady:     len(ps.TestReadyFiles),
		StateReadyForAgent: len(ps.ReadyForAgentFiles),
		StateDone:          len(ps.DoneFiles),
		StateQuarantine:    len(ps.QuarantineFiles),
		StateUnable:        len(ps.UnableFiles),
	}
}

func ScanIssueDir(root string) (*PipelineState, error) {
	todo, err := scanDirFiles(root)
	if err != nil {
		return nil, fmt.Errorf("scan todo: %w", err)
	}
	testReady, err := scanDirFiles(filepath.Join(root, string(StateTestReady)))
	if err != nil {
		return nil, fmt.Errorf("scan test-ready: %w", err)
	}
	readyForAgent, err := scanDirFiles(filepath.Join(root, string(StateReadyForAgent)))
	if err != nil {
		return nil, fmt.Errorf("scan ready-for-agent: %w", err)
	}
	done, err := scanDirFiles(filepath.Join(root, string(StateDone)))
	if err != nil {
		return nil, fmt.Errorf("scan done: %w", err)
	}
	quarantine, err := scanDirFiles(filepath.Join(root, string(StateQuarantine)))
	if err != nil {
		return nil, fmt.Errorf("scan quarantine: %w", err)
	}
	unable, err := scanDirFiles(filepath.Join(root, string(StateUnable)))
	if err != nil {
		return nil, fmt.Errorf("scan unable: %w", err)
	}
	ps := &PipelineState{
		TodoFiles:          todo,
		TestReadyFiles:     testReady,
		ReadyForAgentFiles: readyForAgent,
		DoneFiles:          done,
		QuarantineFiles:    quarantine,
		UnableFiles:        unable,
	}

	// Populate Files cache after initial scan so parsing errors in individual
	// files don't block the scan itself.
	allPaths := ps.TodoFiles
	allPaths = append(allPaths, ps.TestReadyFiles...)
	allPaths = append(allPaths, ps.ReadyForAgentFiles...)
	allPaths = append(allPaths, ps.DoneFiles...)
	allPaths = append(allPaths, ps.QuarantineFiles...)
	allPaths = append(allPaths, ps.UnableFiles...)

	ps.Files = make(map[string]*IssueFile, len(allPaths))
	for _, p := range allPaths {
		if f, err := ParseIssueFile(p); err == nil {
			ps.Files[p] = f
		}
	}

	return ps, nil
}

func scanDirFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read dir %s: %w", dir, err)
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		files = append(files, filepath.Join(dir, e.Name()))
	}
	return files, nil
}

func stateDir(issuesDir string, state State) string {
	if state == StateTodo {
		return issuesDir
	}
	return filepath.Join(issuesDir, string(state))
}

func List(issuesDir string, state State) ([]Issue, error) {
	dir := stateDir(issuesDir, state)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read dir %s: %w", dir, err)
	}
	var issues []Issue
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		issue, err := Read(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: skipping malformed issue file %s: %v\n", path, err)
			continue
		}
		issues = append(issues, *issue)
	}
	return issues, nil
}

// FileMoveError describes a file move failure and provides a user-facing
// suggestion for resolving the issue.
type FileMoveError struct {
	Src        string
	Dst        string
	Err        error
	Suggestion string
}

func (e *FileMoveError) Error() string {
	return e.Err.Error()
}

func (e *FileMoveError) Unwrap() error {
	return e.Err
}

const maxMoveRetries = 3

func moveRetryDelay(attempt int) time.Duration {
	return time.Duration(100*(attempt+1)) * time.Millisecond
}

func isRetryableError(err error) bool {
	if errors.Is(err, os.ErrPermission) {
		return true
	}
	if errors.Is(err, syscall.EACCES) || errors.Is(err, syscall.EPERM) {
		return true
	}
	return errors.Is(err, syscall.EBUSY) || errors.Is(err, syscall.ETXTBSY)
}

func suggestionForErr(err error) string {
	switch {
	case errors.Is(err, os.ErrPermission) || errors.Is(err, syscall.EACCES) || errors.Is(err, syscall.EPERM):
		return "The file could not be accessed due to insufficient permissions. Check that the file is not read-only and that you have write access to both the source and destination directories."
	case errors.Is(err, syscall.EBUSY) || errors.Is(err, syscall.ETXTBSY):
		return "The file appears to be in use by another process. Close any programs (editor, file manager, antivirus) that may be using the file and retry."
	default:
		return "The file could not be moved. Verify that the destination directory exists and is writable, and that the disk is not full."
	}
}

// moveFile moves a file from src to dst. It retries transient errors
// (permission denied, file locked) up to 3 times with backoff. For
// cross-device link errors (EXDEV) it falls back to copy+delete.
func moveFile(src, dst string) error {
	var err error
	for i := range maxMoveRetries + 1 {
		if i > 0 {
			time.Sleep(moveRetryDelay(i))
		}
		err = moveFileOnce(src, dst)
		if err == nil {
			return nil
		}
		if !isRetryableError(err) {
			break
		}
	}
	if err == nil {
		return nil
	}
	return &FileMoveError{
		Src:        src,
		Dst:        dst,
		Err:        err,
		Suggestion: suggestionForErr(err),
	}
}

func moveFileOnce(src, dst string) error {
	err := os.Rename(src, dst)
	if err == nil {
		return nil
	}
	var linkErr *os.LinkError
	if errors.As(err, &linkErr) && errors.Is(linkErr.Err, syscall.EXDEV) {
		return copyAndDelete(src, dst)
	}
	return err
}

// copyAndDelete copies src to dst then removes src. Used as a fallback
// when os.Rename fails with EXDEV (cross-device link).
func copyAndDelete(src, dst string) error {
	if err := copyFile(src, dst); err != nil {
		return fmt.Errorf("copy %s to %s: %w", src, dst, err)
	}
	if err := os.Remove(src); err != nil {
		os.Remove(dst)
		return fmt.Errorf("remove %s after copy: %w", src, err)
	}
	return nil
}

// copyFile copies a file from src to dst, preserving the source's mode.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create: %w", err)
	}

	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return fmt.Errorf("copy data: %w", err)
	}

	if err := out.Close(); err != nil {
		return fmt.Errorf("close destination: %w", err)
	}

	// Preserve source file mode.
	fi, err := os.Stat(src)
	if err == nil {
		os.Chmod(dst, fi.Mode())
	}

	return nil
}

func Move(issuesDir string, issue Issue, to State) error {
	if issue.State == to {
		return nil
	}
	srcDir := stateDir(issuesDir, issue.State)
	dstDir := stateDir(issuesDir, to)

	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return fmt.Errorf("create destination: %w", err)
	}

	src := filepath.Join(srcDir, filepath.Base(issue.FilePath))
	dst := filepath.Join(dstDir, filepath.Base(issue.FilePath))

	if err := moveFile(src, dst); err != nil {
		return fmt.Errorf("move %s to %s: %w", src, dst, err)
	}

	fmt.Fprintf(os.Stderr, "  ✓ %q moved: %s → %s\n", issue.Title, issue.State, to)
	AppendTransitionLog(issuesDir, TransitionEvent{
		Title: issue.Title,
		From:  issue.State,
		To:    to,
	})

	return nil
}

func Read(path string) (*Issue, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	body := string(data)

	lines := strings.SplitN(body, "\n", 2)
	if len(lines) == 0 || strings.TrimSpace(lines[0]) == "" {
		return nil, fmt.Errorf("%w: file is empty", errMissingHeading)
	}

	firstLine := strings.TrimSpace(lines[0])
	if !strings.HasPrefix(firstLine, "# ") {
		return nil, fmt.Errorf("%w: got %q", errMissingHeading, firstLine)
	}

	title := strings.TrimPrefix(firstLine, "# ")
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, fmt.Errorf("%w: heading is empty", errMissingHeading)
	}

	state := StateFromPath(path)
	githubNum := parseGitHubNum(body)
	return &Issue{
		FilePath:  path,
		Title:     title,
		Body:      body,
		State:     state,
		GitHubNum: githubNum,
	}, nil
}

func parseGitHubNum(body string) int {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "GitHub: #") {
			num, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "GitHub: #")))
			if err == nil {
				return num
			}
		}
	}
	return 0
}

func StateFromPath(path string) State {
	dir := filepath.Base(filepath.Dir(path))
	switch dir {
	case "test-ready":
		return StateTestReady
	case "ready-for-agent":
		return StateReadyForAgent
	case "done":
		return StateDone
	case ".quarantine":
		return StateQuarantine
	case "unable":
		return StateUnable
	default:
		return StateTodo
	}
}

func ParseIssueFile(path string) (*IssueFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	f := &IssueFile{FilePath: path, State: StateFromPath(path)}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "# "):
			if f.Title == "" {
				f.Title = strings.TrimSpace(strings.TrimPrefix(line, "# "))
			}
		case strings.HasPrefix(line, "GitHub: #"):
			num, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "GitHub: #")))
			if err == nil {
				f.GitHubNum = num
			}
		case strings.HasPrefix(line, "Execution mode:"):
			f.ExecMode = strings.TrimSpace(strings.TrimPrefix(line, "Execution mode:"))
		case strings.HasPrefix(line, "Branch:"):
			f.Branch = strings.TrimSpace(strings.TrimPrefix(line, "Branch:"))
		case strings.HasPrefix(line, "Checksum:"):
			f.Checksum = strings.TrimSpace(strings.TrimPrefix(line, "Checksum:"))
		case strings.HasPrefix(line, "Type:"):
			f.Type = strings.TrimSpace(strings.TrimPrefix(line, "Type:"))
		case strings.HasPrefix(line, "Retry:"):
			trimmed := strings.TrimSpace(strings.TrimPrefix(line, "Retry:"))
			if n, err := strconv.Atoi(trimmed); err == nil {
				f.Retries = n
			}
		}
	}

	return f, nil
}

func computeChecksumFromContent(content string) string {
	lines := strings.Split(content, "\n")
	var filtered []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "Checksum:") {
			continue
		}
		filtered = append(filtered, line)
	}
	h := sha256.Sum256([]byte(strings.Join(filtered, "\n")))
	return hex.EncodeToString(h[:])
}

func SetChecksum(path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}
	checksum := computeChecksumFromContent(string(content))
	lines := strings.Split(string(content), "\n")

	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "Checksum:") {
			lines[i] = "Checksum: " + checksum
			return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644)
		}
	}

	updated := insertChecksumLine(string(content), checksum)
	return os.WriteFile(path, []byte(updated), 0644)
}

func SetRetryCount(path string, retries int) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read file for retry update: %w", err)
	}
	content := string(data)
	retryLine := fmt.Sprintf("Retry: %d", retries)

	lines := strings.SplitN(content, "\n", 2)
	if len(lines) < 2 {
		return fmt.Errorf("file too short for retry update")
	}

	header := lines[0]
	rest := lines[1]

	// Check if Retry: already exists in the header area
	headerLines := strings.SplitN(content, "\n## ", 2)
	headerSection := headerLines[0]

	if strings.Contains(headerSection, "\nRetry:") || strings.HasPrefix(headerSection[len(lines[0]):], "Retry:") {
		// Replace existing Retry: line
		var newLines []string
		for _, line := range strings.Split(content, "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "Retry:") {
				newLines = append(newLines, retryLine)
			} else {
				newLines = append(newLines, line)
			}
		}
		content = strings.Join(newLines, "\n")
	} else {
		// Add Retry: after the title line
		content = header + "\n" + retryLine + "\n" + rest
	}

	return os.WriteFile(path, []byte(content), 0644)
}

func insertChecksumLine(content string, checksum string) string {
	line := "Checksum: " + checksum
	lines := strings.Split(content, "\n")

	for i, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "Branch:") {
			return insertAtLine(lines, i+1, line)
		}
	}
	for i, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "Execution mode:") {
			return insertAtLine(lines, i+1, line)
		}
	}
	for i, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "Status:") {
			return insertAtLine(lines, i+1, line)
		}
	}
	for i, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "GitHub: #") {
			return insertAtLine(lines, i+1, line)
		}
	}
	for i, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "# ") {
			return insertAtLine(lines, i+1, line)
		}
	}
	return content + "\n" + line
}

func VerifyChecksum(path string) (bool, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("read file: %w", err)
	}

	var storedChecksum string
	for _, line := range strings.Split(string(content), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "Checksum:") {
			storedChecksum = strings.TrimSpace(strings.TrimPrefix(trimmed, "Checksum:"))
			break
		}
	}

	if storedChecksum == "" {
		return false, nil
	}

	expected := computeChecksumFromContent(string(content))
	return storedChecksum == expected, nil
}

// ChecksumResult describes the verification result for a single file.
type ChecksumResult struct {
	FilePath string
	Valid    bool
	Err      error
}

// VerifyChecksums scans all issue files and verifies each file's checksum
// against its content. Returns a list of results, one per file that has a
// Checksum header. Files without a Checksum header are skipped.
func VerifyChecksums(root string) ([]ChecksumResult, error) {
	ps, err := ScanIssueDir(root)
	if err != nil {
		return nil, fmt.Errorf("scan issue dir: %w", err)
	}

	allPaths := ps.TodoFiles
	allPaths = append(allPaths, ps.TestReadyFiles...)
	allPaths = append(allPaths, ps.ReadyForAgentFiles...)
	allPaths = append(allPaths, ps.DoneFiles...)
	allPaths = append(allPaths, ps.QuarantineFiles...)

	var results []ChecksumResult
	for _, p := range allPaths {
		f, err := ParseIssueFile(p)
		if err != nil {
			continue
		}
		if f.Checksum == "" {
			continue
		}
		valid, err := VerifyChecksum(p)
		results = append(results, ChecksumResult{
			FilePath: p,
			Valid:    valid,
			Err:      err,
		})
	}
	return results, nil
}

func ParseIssueHeader(content string) (*IssueHeader, error) {
	h := &IssueHeader{}
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "# "):
			if h.Title == "" {
				h.Title = strings.TrimSpace(strings.TrimPrefix(line, "# "))
			}
		case strings.HasPrefix(line, "GitHub: #"):
			num, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "GitHub: #")))
			if err == nil {
				h.GitHubNum = num
			}
		case strings.HasPrefix(line, "Execution mode:"):
			h.ExecMode = strings.TrimSpace(strings.TrimPrefix(line, "Execution mode:"))
		case strings.HasPrefix(line, "Branch:"):
			h.Branch = strings.TrimSpace(strings.TrimPrefix(line, "Branch:"))
		case strings.HasPrefix(line, "Type:"):
			h.Type = strings.TrimSpace(strings.TrimPrefix(line, "Type:"))
		}
	}
	return h, nil
}

func ScanParsed(root string) ([]*IssueFile, error) {
	ps, err := ScanIssueDir(root)
	if err != nil {
		return nil, fmt.Errorf("scan issue dir: %w", err)
	}

	allPaths := ps.TodoFiles
	allPaths = append(allPaths, ps.TestReadyFiles...)
	allPaths = append(allPaths, ps.ReadyForAgentFiles...)
	allPaths = append(allPaths, ps.DoneFiles...)
	allPaths = append(allPaths, ps.QuarantineFiles...)

	var result []*IssueFile
	for _, p := range allPaths {
		f, err := ParseIssueFile(p)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: skipping malformed issue file %s: %v\n", p, err)
			continue
		}
		result = append(result, f)
	}
	return result, nil
}

type Role string

const (
	RoleImplement Role = "implement"
	RoleTest      Role = "test"
)

// Execution mode constants.
const (
	ExecModeAFKOnly  = "AFK-only"
	ExecModeHITLOnly = "HITL-only"
	ExecModeCombo    = "Combo"
)

// ValidExecModes returns all valid execution mode values.
func ValidExecModes() []string {
	return []string{ExecModeAFKOnly, ExecModeHITLOnly, ExecModeCombo}
}

// IsValidExecMode reports whether the given execution mode is one of the
// recognised values: AFK-only, HITL-only, or Combo.
func IsValidExecMode(mode string) bool {
	return mode == ExecModeAFKOnly || mode == ExecModeHITLOnly || mode == ExecModeCombo
}

// ExecModeAllowsImplement reports whether the given execution mode allows
// autonomous implementation. Only AFK-only is eligible; HITL-only and Combo
// require human involvement and must be skipped.
func ExecModeAllowsImplement(mode string) bool {
	return mode == ExecModeAFKOnly
}

var (
	ErrNoIssues         = fmt.Errorf("no issues available")
	ErrPreFlightFailed  = fmt.Errorf("pre-flight checks failed")
	ErrIssueAlreadyDone = fmt.Errorf("Issue already done")
	ErrIssueNonAFK      = fmt.Errorf("issue execution mode does not allow autonomous implementation")
)

func isBlockerResolved(ref string, doneBases []string) bool {
	ref = strings.TrimSpace(ref)
	ref = strings.TrimSuffix(ref, ".md")
	parts := strings.Fields(ref)
	for _, p := range parts {
		p = strings.TrimLeft(p, "#( ")
		if p == "" {
			continue
		}
		if _, err := strconv.Atoi(p); err == nil {
			for _, db := range doneBases {
				if strings.HasPrefix(db, p) {
					return true
				}
			}
		}
	}
	return false
}

func ParseBlockedBy(content string) []string {
	var refs []string
	inBlockedBy := false
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimLeftFunc(line, unicode.IsSpace)
		if name := sectionName(trimmed); name != "" && strings.EqualFold(name, "Blocked by") {
			inBlockedBy = true
			continue
		}
		if !inBlockedBy {
			continue
		}
		if sectionName(trimmed) != "" {
			break
		}
		if strings.HasPrefix(trimmed, "- ") {
			ref := strings.TrimPrefix(trimmed, "- ")
			if strings.HasPrefix(strings.ToLower(ref), "none") {
				return nil
			}
			refs = append(refs, ref)
		}
	}
	return refs
}

func ParseIssueSections(content string) (map[string]string, error) {
	sections := make(map[string]string)
	if content == "" {
		return sections, nil
	}

	var currentSection string
	var buf strings.Builder

	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimLeftFunc(line, unicode.IsSpace)
		if name := sectionName(trimmed); name != "" {
			if currentSection != "" {
				sections[currentSection] = strings.TrimSpace(buf.String())
			}
			currentSection = strings.ToLower(name)
			buf.Reset()
			continue
		}
		if currentSection != "" {
			if buf.Len() > 0 {
				buf.WriteString("\n")
			}
			buf.WriteString(line)
		}
	}

	if currentSection != "" {
		sections[currentSection] = strings.TrimSpace(buf.String())
	}

	return sections, nil
}

func extractSection(content, section string) string {
	var buf strings.Builder
	inSection := false
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimLeftFunc(line, unicode.IsSpace)
		if name := sectionName(trimmed); name != "" {
			if strings.EqualFold(name, section) {
				inSection = true
				continue
			}
			if inSection {
				break
			}
		}
		if inSection {
			if buf.Len() > 0 {
				buf.WriteString("\n")
			}
			buf.WriteString(line)
		}
	}
	return strings.TrimSpace(buf.String())
}

func ParseIssueBody(content string) *IssueBody {
	return &IssueBody{
		Parent:             extractSection(content, "Parent"),
		WhatToBuild:        extractSection(content, "What to build"),
		UserStoriesCovered: extractSection(content, "User stories covered"),
		AcceptanceCriteria: extractSection(content, "Acceptance criteria"),
		UATPlan:            extractSection(content, "UAT plan"),
		BlockedBy:          extractSection(content, "Blocked by"),
		Comments:           extractSection(content, "Comments"),
	}
}

func doneBasenames(doneFiles []string) []string {
	bases := make([]string, len(doneFiles))
	for i, f := range doneFiles {
		n := filepath.Base(f)
		n = strings.TrimSuffix(n, ".md")
		bases[i] = n
	}
	return bases
}

func allBlockersResolved(blockers []string, doneBases []string) bool {
	for _, b := range blockers {
		if !isBlockerResolved(b, doneBases) {
			return false
		}
	}
	return true
}

// SelectIssue picks the next issue to process.
// Priority 1: unblocked test-ready items (role = test; any execution mode).
// Priority 2: first unblocked item from ready-for-agent whose execution mode
// allows autonomous implementation (role = implement).
// Priority 3: first unblocked item from todo whose execution mode allows
// autonomous implementation (role = implement; see ExecModeAllowsImplement).
func SelectIssue(state *PipelineState) (selected *IssueFile, role Role, err error) {
	dones := doneBasenames(state.DoneFiles)

	// Priority 1: exhaust all unblocked test-ready items first.
	// Skip files that already have populated UAT Results — they were tested
	// but the transition was interrupted; leave them for manual triage.
	for _, f := range state.TestReadyFiles {
		issueFile, err := getCachedIssueFile(state, f)
		if err != nil {
			return nil, "", fmt.Errorf("parse test-ready file: %w", err)
		}
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, "", fmt.Errorf("read test-ready file: %w", err)
		}
		if sectionHasContent(string(data), "UAT Results") {
			continue
		}
		blockers := ParseBlockedBy(string(data))
		if len(blockers) > 0 && !allBlockersResolved(blockers, dones) {
			continue
		}
		return issueFile, RoleTest, nil
	}
	// All test-ready items are either stuck or blocked.
	// Fall through to ready-for-agent and todo.

	// Priority 2: pick first unblocked AFK-only issue from ready-for-agent/.
	for _, f := range state.ReadyForAgentFiles {
		issueFile, err := getCachedIssueFile(state, f)
		if err != nil {
			return nil, "", fmt.Errorf("parse ready-for-agent file: %w", err)
		}
		if !ExecModeAllowsImplement(issueFile.ExecMode) {
			continue
		}
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, "", fmt.Errorf("read ready-for-agent file: %w", err)
		}
		blockers := ParseBlockedBy(string(data))
		if len(blockers) > 0 && !allBlockersResolved(blockers, dones) {
			continue
		}
		return issueFile, RoleImplement, nil
	}

	// Priority 3: pick first unblocked AFK-only issue from issues/.
	for _, f := range state.TodoFiles {
		issueFile, err := getCachedIssueFile(state, f)
		if err != nil {
			return nil, "", fmt.Errorf("parse todo file: %w", err)
		}
		if !ExecModeAllowsImplement(issueFile.ExecMode) {
			continue
		}
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, "", fmt.Errorf("read todo file: %w", err)
		}
		blockers := ParseBlockedBy(string(data))
		if len(blockers) > 0 && !allBlockersResolved(blockers, dones) {
			continue
		}
		return issueFile, RoleImplement, nil
	}

	// Priority 4 (fallback): pick first unblocked HITL-only issue. These are
	// handled specially by the iteration code (no agent run, NO_MORE_TASKS).
	for _, f := range state.ReadyForAgentFiles {
		issueFile, err := getCachedIssueFile(state, f)
		if err != nil {
			return nil, "", fmt.Errorf("parse ready-for-agent file: %w", err)
		}
		if issueFile.ExecMode != ExecModeHITLOnly {
			continue
		}
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, "", fmt.Errorf("read ready-for-agent file: %w", err)
		}
		blockers := ParseBlockedBy(string(data))
		if len(blockers) > 0 && !allBlockersResolved(blockers, dones) {
			continue
		}
		return issueFile, RoleImplement, nil
	}
	for _, f := range state.TodoFiles {
		issueFile, err := getCachedIssueFile(state, f)
		if err != nil {
			return nil, "", fmt.Errorf("parse todo file: %w", err)
		}
		if issueFile.ExecMode != ExecModeHITLOnly {
			continue
		}
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, "", fmt.Errorf("read todo file: %w", err)
		}
		blockers := ParseBlockedBy(string(data))
		if len(blockers) > 0 && !allBlockersResolved(blockers, dones) {
			continue
		}
		return issueFile, RoleImplement, nil
	}

	return nil, "", ErrNoIssues
}

// FindIssueByNum finds an issue by GitHub issue number in the pipeline state.
// It checks done/ first (returns ErrIssueAlreadyDone if found), then todo/,
// ready-for-agent/, and test-ready/. For todo and ready-for-agent, it also
// checks that the execution mode allows autonomous implementation.
func FindIssueByNum(state *PipelineState, num int) (*IssueFile, Role, error) {
	for _, path := range state.DoneFiles {
		f, err := getCachedIssueFile(state, path)
		if err != nil {
			continue
		}
		if f.GitHubNum == num {
			return nil, "", ErrIssueAlreadyDone
		}
	}
	for _, path := range state.TodoFiles {
		f, err := getCachedIssueFile(state, path)
		if err != nil {
			continue
		}
		if f.GitHubNum == num {
			if !ExecModeAllowsImplement(f.ExecMode) {
				return nil, "", fmt.Errorf("%w: issue #%d (%q) has execution mode %q", ErrIssueNonAFK, num, f.Title, f.ExecMode)
			}
			return f, RoleImplement, nil
		}
	}
	for _, path := range state.ReadyForAgentFiles {
		f, err := getCachedIssueFile(state, path)
		if err != nil {
			continue
		}
		if f.GitHubNum == num {
			if !ExecModeAllowsImplement(f.ExecMode) {
				return nil, "", fmt.Errorf("%w: issue #%d (%q) has execution mode %q", ErrIssueNonAFK, num, f.Title, f.ExecMode)
			}
			return f, RoleImplement, nil
		}
	}
	for _, path := range state.TestReadyFiles {
		f, err := getCachedIssueFile(state, path)
		if err != nil {
			continue
		}
		if f.GitHubNum == num {
			return f, RoleTest, nil
		}
	}
	return nil, "", nil
}

// ValidStatusLabels lists the allowed Status: values for todo issue files.
var ValidStatusLabels = []string{"ready-for-agent", "ready-for-human"}

// AddMissingTodoLabels scans files in the issues root directory (todo state)
// and adds "Status: ready-for-agent" to any file that has neither
// "Status: ready-for-agent" nor "Status: ready-for-human". Returns the count
// of files modified.
func AddMissingTodoLabels(root string) (int, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("read issues dir: %w", err)
	}

	var modified int
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		path := filepath.Join(root, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: read %s: %v\n", path, err)
			continue
		}
		content := string(data)

		if hasValidStatus(content) {
			continue
		}

		updated := insertStatusLine(content, "ready-for-agent")
		if updated == content {
			continue
		}
		if err := os.WriteFile(path, []byte(updated), 0644); err != nil {
			return modified, fmt.Errorf("write %s: %w", path, err)
		}
		modified++
	}
	return modified, nil
}

// hasValidStatus reports whether content contains any of the valid Status: lines.
func hasValidStatus(content string) bool {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		for _, label := range ValidStatusLabels {
			if trimmed == "Status: "+label {
				return true
			}
		}
	}
	return false
}

// insertStatusLine inserts "Status: <value>" into the header block of the
// issue file content. It places it after the "GitHub:" line if present,
// otherwise after the title line, before the first blank line or "##" section.
func insertStatusLine(content, value string) string {
	line := "Status: " + value
	lines := strings.Split(content, "\n")

	for i, l := range lines {
		trimmed := strings.TrimSpace(l)
		if strings.HasPrefix(trimmed, "GitHub: #") {
			insertAt := i + 1
			if insertAt >= len(lines) {
				return content + "\n" + line
			}
			result := make([]string, 0, len(lines)+1)
			result = append(result, lines[:insertAt]...)
			result = append(result, line)
			result = append(result, lines[insertAt:]...)
			return strings.Join(result, "\n")
		}
	}

	titleIdx := -1
	for i, l := range lines {
		trimmed := strings.TrimSpace(l)
		if strings.HasPrefix(trimmed, "# ") {
			titleIdx = i
			break
		}
	}

	if titleIdx >= 0 {
		insertAt := titleIdx + 1
		if insertAt < len(lines) && strings.TrimSpace(lines[insertAt]) == "" {
			insertAt++
			if insertAt < len(lines) {
				result := make([]string, 0, len(lines)+1)
				result = append(result, lines[:insertAt]...)
				result = append(result, line)
				result = append(result, lines[insertAt:]...)
				return strings.Join(result, "\n")
			}
		}
		result := make([]string, 0, len(lines)+1)
		result = append(result, lines[:insertAt]...)
		result = append(result, line)
		result = append(result, lines[insertAt:]...)
		return strings.Join(result, "\n")
	}

	return content + "\n" + line
}

// AddMissingExecMode scans todo files and adds "Execution mode: AFK-only" to
// any file that does not have an Execution mode header field.
func AddMissingExecMode(root string) (int, error) {
	ps, err := ScanIssueDir(root)
	if err != nil {
		return 0, fmt.Errorf("scan issue dir: %w", err)
	}

	allPaths := ps.TodoFiles
	allPaths = append(allPaths, ps.TestReadyFiles...)
	allPaths = append(allPaths, ps.ReadyForAgentFiles...)

	var modified int
	for _, p := range allPaths {
		data, err := os.ReadFile(p)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: read %s: %v\n", p, err)
			continue
		}
		content := string(data)

		if hasExecMode(content) {
			continue
		}

		updated := insertExecModeLine(content)
		if updated == content {
			continue
		}
		if err := os.WriteFile(p, []byte(updated), 0644); err != nil {
			return modified, fmt.Errorf("write %s: %w", p, err)
		}
		modified++
	}
	return modified, nil
}

// hasExecMode reports whether content contains an "Execution mode:" header line.
func hasExecMode(content string) bool {
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "Execution mode:") {
			return true
		}
	}
	return false
}

// insertExecModeLine inserts "Execution mode: AFK-only" into the header block
// of the issue file content. It places it after "Status:" if present,
// otherwise after "GitHub:" if present, otherwise after the title line.
func insertExecModeLine(content string) string {
	line := "Execution mode: AFK-only"
	lines := strings.Split(content, "\n")

	// Insert after "Status:" line if present.
	for i, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "Status:") {
			return insertAtLine(lines, i+1, line)
		}
	}

	// Insert after "GitHub: #" line if present.
	for i, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "GitHub: #") {
			return insertAtLine(lines, i+1, line)
		}
	}

	// Insert after title line.
	for i, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "# ") {
			return insertAtLine(lines, i+1, line)
		}
	}

	return content + "\n" + line
}

func insertAtLine(lines []string, idx int, line string) string {
	if idx >= len(lines) {
		return strings.Join(lines, "\n") + "\n" + line
	}
	result := make([]string, 0, len(lines)+1)
	result = append(result, lines[:idx]...)
	result = append(result, line)
	result = append(result, lines[idx:]...)
	return strings.Join(result, "\n")
}

// UnparseableFile represents a file that could not be parsed as an issue.
type UnparseableFile struct {
	Path string
	Err  error
}

// Severity represents the severity level of a PreFlightIssue.
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
)

// PreFlightIssue describes a problem found during a pre-flight check.
type PreFlightIssue struct {
	FilePath string
	Severity Severity
	Message  string
}

// Transition represents a file moving from SourceDir to DestDir.
type Transition struct {
	SourceDir string
	DestDir   string
	Filename  string
}

// TransitionEvent records a single issue state transition for logging.
type TransitionEvent struct {
	Time  time.Time `json:"time"`
	Title string    `json:"title"`
	From  State     `json:"from"`
	To    State     `json:"to"`
}

const transitionLogFile = "transitions.log"

func logDir(issuesDir string) string {
	return filepath.Join(issuesDir, ".loop")
}

func transitionLogPath(issuesDir string) string {
	return filepath.Join(logDir(issuesDir), transitionLogFile)
}

// AppendTransitionLog appends a transition event to the transition log file.
func AppendTransitionLog(issuesDir string, event TransitionEvent) error {
	if err := os.MkdirAll(logDir(issuesDir), 0755); err != nil {
		return fmt.Errorf("create log dir: %w", err)
	}
	f, err := os.OpenFile(transitionLogPath(issuesDir), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open transitions log: %w", err)
	}
	defer f.Close()
	event.Time = time.Now()
	if err := json.NewEncoder(f).Encode(event); err != nil {
		return fmt.Errorf("encode transition event: %w", err)
	}
	return nil
}

// ReadTransitionLog reads the last n transition events from the log file.
// If n <= 0, all events are returned. Returns nil if the log does not exist.
func ReadTransitionLog(issuesDir string, n int) ([]TransitionEvent, error) {
	f, err := os.Open(transitionLogPath(issuesDir))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open transitions log: %w", err)
	}
	defer f.Close()

	var events []TransitionEvent
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if len(scanner.Bytes()) == 0 {
			continue
		}
		var ev TransitionEvent
		if err := json.Unmarshal(scanner.Bytes(), &ev); err != nil {
			continue
		}
		events = append(events, ev)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read transitions log: %w", err)
	}
	if n > 0 && len(events) > n {
		events = events[len(events)-n:]
	}
	return events, nil
}

// ComputeTransition determines where an issue file should move based on the
// current file state, the promise from the agent, and the role that ran.
// NO_MORE_TASKS always moves the issue to quarantine.
func ComputeTransition(file *IssueFile, promise agent.Promise, role Role) (*Transition, error) {
	if promise == agent.NoMoreTasks {
		root := issuesRoot(file.FilePath, file.State)
		return &Transition{
			SourceDir: stateDir(root, file.State),
			DestDir:   stateDir(root, StateQuarantine),
			Filename:  filepath.Base(file.FilePath),
		}, nil
	}

	switch role {
	case RoleImplement:
		if file.State != StateTodo && file.State != StateReadyForAgent {
			return nil, fmt.Errorf("cannot implement issue in state %q", file.State)
		}
	case RoleTest:
		if file.State != StateTestReady {
			return nil, fmt.Errorf("cannot test issue in state %q", file.State)
		}
	default:
		return nil, fmt.Errorf("unknown role %q", role)
	}

	if err := validateRolePromise(role, promise); err != nil {
		return nil, err
	}

	var target State
	switch role {
	case RoleImplement:
		switch promise {
		case agent.Complete:
			target = StateTestReady
		default:
			return nil, fmt.Errorf("unknown promise %q", promise)
		}
	case RoleTest:
		switch promise {
		case agent.TestPass:
			target = StateDone
		case agent.TestFail:
			target = StateTodo
		default:
			return nil, fmt.Errorf("unknown promise %q", promise)
		}
	}

	root := issuesRoot(file.FilePath, file.State)
	return &Transition{
		SourceDir: stateDir(root, file.State),
		DestDir:   stateDir(root, target),
		Filename:  filepath.Base(file.FilePath),
	}, nil
}

// validateRolePromise checks that the promise is valid for the given role.
// IMPLEMENTING cannot emit TEST_PASS or TEST_FAIL; TESTING cannot emit COMPLETE.
func validateRolePromise(role Role, promise agent.Promise) error {
	switch role {
	case RoleImplement:
		switch promise {
		case agent.TestPass, agent.TestFail:
			return fmt.Errorf("role %q cannot emit promise %q", role, promise)
		}
	case RoleTest:
		switch promise {
		case agent.Complete:
			return fmt.Errorf("role %q cannot emit promise %q", role, promise)
		}
	}
	return nil
}

// CleanTestResults scans all issue files and removes result sections that are
// not valid in the file's current state per lifecycle rules:
//   - ## Test Results only valid in test-ready/ or done/
//   - ## UAT Results only valid in done/ (after PASS)
//
// Returns the list of files that were cleaned.
func CleanTestResults(root string) ([]string, error) {
	ps, err := ScanIssueDir(root)
	if err != nil {
		return nil, fmt.Errorf("scan issue dir: %w", err)
	}

	var cleaned []string

	// todo/: strip sections disallowed in the todo state.
	for _, p := range ps.TodoFiles {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		disallowed := append([]string{"Test Results"}, DisallowedSections[StateTodo]...)
		stripped := StripIssueSections(string(data), disallowed)
		if stripped != string(data) {
			if err := os.WriteFile(p, []byte(stripped), 0644); err != nil {
				return cleaned, fmt.Errorf("write %s: %w", p, err)
			}
			cleaned = append(cleaned, p)
		}
	}

	// test-ready/: Only empty UAT Results sections are stripped. Filled UAT
	// Results are preserved so PromoteStuckTestReady can promote them to done/.
	for _, p := range ps.TestReadyFiles {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		stripped := stripEmptySection(string(data), "UAT Results")
		if stripped != string(data) {
			if err := os.WriteFile(p, []byte(stripped), 0644); err != nil {
				return cleaned, fmt.Errorf("write %s: %w", p, err)
			}
			cleaned = append(cleaned, p)
		}
	}

	// ready-for-agent/: strip Test Results and UAT Results (same as todo).
	for _, p := range ps.ReadyForAgentFiles {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		stripped := StripIssueSections(string(data), DisallowedSections[StateReadyForAgent])
		if stripped != string(data) {
			if err := os.WriteFile(p, []byte(stripped), 0644); err != nil {
				return cleaned, fmt.Errorf("write %s: %w", p, err)
			}
			cleaned = append(cleaned, p)
		}
	}

	// done/ and quarantine/: both sections are valid, keep as-is.

	return cleaned, nil
}

// StripSectionsFromFile removes the named ## sections from a markdown file,
// writing the result back to the same path.
func StripSectionsFromFile(path string, sections []string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}
	stripped := StripIssueSections(string(data), sections)
	return os.WriteFile(path, []byte(stripped), 0644)
}

// StripEmptyUATPlaceholders scans test-ready issue files and removes empty
// "## UAT Results" placeholder sections. A section is considered empty if it
// contains no meaningful content (only whitespace) between the heading and
// the next "##" heading or EOF. Unlike CleanTestResults, this only targets
// empty placeholders in test-ready files rather than stripping all sections.
func StripEmptyUATPlaceholders(root string) ([]string, error) {
	ps, err := ScanIssueDir(root)
	if err != nil {
		return nil, fmt.Errorf("scan issue dir: %w", err)
	}

	var cleaned []string
	for _, p := range ps.TestReadyFiles {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		content := string(data)
		modified := stripEmptySection(content, "UAT Results")
		if modified != content {
			if err := os.WriteFile(p, []byte(modified), 0644); err != nil {
				fmt.Fprintf(os.Stderr, "warning: failed to write %s: %v\n", p, err)
				continue
			}
			cleaned = append(cleaned, p)
		}
	}
	for _, p := range ps.ReadyForAgentFiles {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		content := string(data)
		modified := stripEmptySection(content, "UAT Results")
		if modified != content {
			if err := os.WriteFile(p, []byte(modified), 0644); err != nil {
				fmt.Fprintf(os.Stderr, "warning: failed to write %s: %v\n", p, err)
				continue
			}
			cleaned = append(cleaned, p)
		}
	}
	return cleaned, nil
}

// sectionHasContent reports whether the named ## section contains any
// non-whitespace content between its heading and the next ## heading or EOF.
func sectionHasContent(content string, section string) bool {
	lines := strings.Split(content, "\n")
	for i := 0; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if name := sectionName(trimmed); name != "" {
			if strings.EqualFold(name, section) {
				j := i + 1
				for j < len(lines) {
					nextTrimmed := strings.TrimSpace(lines[j])
					if sectionName(nextTrimmed) != "" {
						break
					}
					if nextTrimmed != "" {
						return true
					}
					j++
				}
				return false
			}
		}
	}
	return false
}

// FindInvalidExecModes returns file paths that have an "Execution mode:"
// header with an invalid value (not one of: AFK-only, HITL-only, Combo).
// Unlike AddMissingExecMode, this function does NOT modify the files — it
// only reports them so the caller can flag the user.
func FindInvalidExecModes(root string) ([]string, error) {
	parsed, err := ScanParsed(root)
	if err != nil {
		return nil, fmt.Errorf("scan parsed: %w", err)
	}

	var invalid []string
	for _, f := range parsed {
		if f.ExecMode != "" && !IsValidExecMode(f.ExecMode) {
			invalid = append(invalid, f.FilePath)
		}
	}
	return invalid, nil
}

// FindStuckTestReadyFiles returns test-ready file paths that have non-empty
// ## UAT Results sections. Such files were tested by the agent (which wrote
// results) but never transitioned out of test-ready. Unlike
// PromoteStuckTestReady, this function does NOT move the files — it only
// reports them so the caller can flag the user.
func FindStuckTestReadyFiles(root string) ([]string, error) {
	ps, err := ScanIssueDir(root)
	if err != nil {
		return nil, fmt.Errorf("scan issue dir: %w", err)
	}

	var stuck []string
	for _, p := range ps.TestReadyFiles {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if sectionHasContent(string(data), "UAT Results") {
			stuck = append(stuck, p)
		}
	}
	return stuck, nil
}

// PromoteStuckTestReady promotes test-ready files that have non-empty
// ## UAT Results sections to done/. Such files were tested by the agent
// (which wrote results) but never transitioned out of test-ready.
// Returns the list of files promoted.
func PromoteStuckTestReady(root string) ([]string, error) {
	ps, err := ScanIssueDir(root)
	if err != nil {
		return nil, fmt.Errorf("scan issue dir: %w", err)
	}

	var promoted []string
	for _, p := range ps.TestReadyFiles {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if !sectionHasContent(string(data), "UAT Results") {
			continue
		}
		issueFile, err := Read(p)
		if err != nil {
			continue
		}
		if err := Move(root, *issueFile, StateDone); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to promote %s: %v\n", p, err)
			continue
		}
		promoted = append(promoted, p)
	}
	return promoted, nil
}

// PromoteTodoWithTestResults promotes todo files that have non-empty
// ## Test Results sections to test-ready/. Such files were implemented by
// the agent (which wrote results) but never transitioned out of the todo
// state. Returns the list of files promoted.
func PromoteTodoWithTestResults(root string) ([]string, error) {
	ps, err := ScanIssueDir(root)
	if err != nil {
		return nil, fmt.Errorf("scan issue dir: %w", err)
	}

	var promoted []string
	for _, p := range ps.TodoFiles {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if !sectionHasContent(string(data), "Test Results") {
			continue
		}
		issueFile, err := Read(p)
		if err != nil {
			continue
		}
		if err := Move(root, *issueFile, StateTestReady); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to promote %s: %v\n", p, err)
			continue
		}
		promoted = append(promoted, p)
	}
	return promoted, nil
}

// AddMissingChecksums scans all issue files and adds or updates Checksum header
// fields. Returns the count of files modified.
func AddMissingChecksums(root string, checksumsEnabled bool) (int, error) {
	if !checksumsEnabled {
		return 0, nil
	}
	ps, err := ScanIssueDir(root)
	if err != nil {
		return 0, fmt.Errorf("scan issue dir: %w", err)
	}

	allPaths := ps.TodoFiles
	allPaths = append(allPaths, ps.TestReadyFiles...)
	allPaths = append(allPaths, ps.ReadyForAgentFiles...)
	allPaths = append(allPaths, ps.DoneFiles...)
	allPaths = append(allPaths, ps.QuarantineFiles...)

	var modified int
	for _, p := range allPaths {
		f, err := getCachedIssueFile(ps, p)
		if err != nil {
			continue
		}
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		expected := computeChecksumFromContent(string(data))
		if f.Checksum == expected {
			continue
		}
		if err := SetChecksum(p); err != nil {
			fmt.Fprintf(os.Stderr, "warning: set checksum %s: %v\n", p, err)
			continue
		}
		modified++
	}
	return modified, nil
}

// RepairPipeline performs five repair actions on the pipeline:
//  1. Add missing Status: ready-for-agent to todo files without a valid label.
//  2. Strip empty ## UAT Results placeholder sections from test-ready and
//     ready-for-agent files.
//  3. Detect test-ready files that have populated ## UAT Results (stuck files
//     that were tested but never transitioned) and report them — does NOT
//     auto-promote, leaves them for the user to resolve.
//  4. Promote todo files that have populated ## Test Results to test-ready/
//     (they were implemented but the transition was interrupted).
//  5. Flag files with invalid Execution mode values (no auto-fix).
//  6. Add or update Checksum header fields on all issue files (if
//     checksumsEnabled is true).
//  7. Return the count of files affected by each action.
func RepairPipeline(root string, checksumsEnabled bool) (labelsAdded int, execModesAdded int, stripped int, stuckCount int, testResultsPromoted int, invalidExecModes int, checksumsAdded int, err error) {
	added, labelErr := AddMissingTodoLabels(root)
	if labelErr != nil {
		fmt.Fprintf(os.Stderr, "warning: add missing todo labels: %v\n", labelErr)
	}
	labelsAdded = added

	execAdded, execErr := AddMissingExecMode(root)
	if execErr != nil {
		fmt.Fprintf(os.Stderr, "warning: add missing exec mode: %v\n", execErr)
	}
	execModesAdded = execAdded

	cleaned, stripErr := StripEmptyUATPlaceholders(root)
	if stripErr != nil {
		fmt.Fprintf(os.Stderr, "warning: strip empty UAT placeholders: %v\n", stripErr)
	}
	stripped = len(cleaned)

	stuckFiles, stuckErr := FindStuckTestReadyFiles(root)
	if stuckErr != nil {
		fmt.Fprintf(os.Stderr, "warning: find stuck test-ready files: %v\n", stuckErr)
	}
	stuckCount = len(stuckFiles)

	trFiles, trErr := PromoteTodoWithTestResults(root)
	if trErr != nil {
		fmt.Fprintf(os.Stderr, "warning: promote todo files with test results: %v\n", trErr)
	}
	testResultsPromoted = len(trFiles)

	invalidFiles, invalidErr := FindInvalidExecModes(root)
	if invalidErr != nil {
		fmt.Fprintf(os.Stderr, "warning: find invalid exec modes: %v\n", invalidErr)
	}
	invalidExecModes = len(invalidFiles)

	checksumsAdded, csErr := AddMissingChecksums(root, checksumsEnabled)
	if csErr != nil {
		fmt.Fprintf(os.Stderr, "warning: add missing checksums: %v\n", csErr)
	}

	if labelErr != nil || execErr != nil || stripErr != nil || stuckErr != nil || trErr != nil || invalidErr != nil || csErr != nil {
		return labelsAdded, execModesAdded, stripped, stuckCount, testResultsPromoted, invalidExecModes, checksumsAdded, fmt.Errorf("repair pipeline completed with errors")
	}
	return labelsAdded, execModesAdded, stripped, stuckCount, testResultsPromoted, invalidExecModes, checksumsAdded, nil
}

// stripEmptySection removes a named ## section from content if the section
// has no non-whitespace content between the heading and the next ## heading
// or end of file.
func stripEmptySection(content string, section string) string {
	lines := strings.Split(content, "\n")
	var result []string
	i := 0
	for i < len(lines) {
		trimmed := strings.TrimSpace(lines[i])
		if name := sectionName(trimmed); name != "" {
			if strings.EqualFold(name, section) {
				j := i + 1
				hasContent := false
				for j < len(lines) {
					nextTrimmed := strings.TrimSpace(lines[j])
					if sectionName(nextTrimmed) != "" {
						break
					}
					if nextTrimmed != "" {
						hasContent = true
						break
					}
					j++
				}
				if !hasContent {
					i = j
					continue
				}
			}
		}
		result = append(result, lines[i])
		i++
	}
	return strings.Join(result, "\n")
}

// StripIssueSections removes the named ## sections from a markdown string.
// Each section is removed along with its content up to the next ## heading.
func StripIssueSections(content string, sections []string) string {
	if len(sections) == 0 || content == "" {
		return content
	}

	stripSet := make(map[string]bool, len(sections))
	for _, s := range sections {
		stripSet[strings.ToLower(s)] = true
	}

	lines := strings.Split(content, "\n")
	var result []string
	skip := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if name := sectionName(trimmed); name != "" {
			if stripSet[strings.ToLower(name)] {
				skip = true
				continue
			}
			skip = false
		} else if skip {
			continue
		}
		result = append(result, line)
	}

	return strings.Join(result, "\n")
}

func issuesRoot(filePath string, state State) string {
	if state == StateTodo {
		return filepath.Dir(filePath)
	}
	return filepath.Dir(filepath.Dir(filePath))
}

// ScanUnparseable scans all state directories for .md files that fail to parse.
// It returns the file paths together with the parse error for each.
func ScanUnparseable(root string) ([]UnparseableFile, error) {
	ps, err := ScanIssueDir(root)
	if err != nil {
		return nil, fmt.Errorf("scan issue dir: %w", err)
	}

	allPaths := ps.TodoFiles
	allPaths = append(allPaths, ps.TestReadyFiles...)
	allPaths = append(allPaths, ps.ReadyForAgentFiles...)
	allPaths = append(allPaths, ps.DoneFiles...)
	allPaths = append(allPaths, ps.QuarantineFiles...)

	var result []UnparseableFile
	for _, p := range allPaths {
		_, err := Read(p)
		if err != nil {
			result = append(result, UnparseableFile{Path: p, Err: err})
		}
	}
	return result, nil
}

// extractBlockerNum extracts a numeric issue reference from a "Blocked by" line.
// It mirrors the number-extraction logic in isBlockerResolved.
func extractBlockerNum(ref string) (int, bool) {
	ref = strings.TrimSpace(ref)
	ref = strings.TrimPrefix(ref, "#")
	ref = strings.TrimSuffix(ref, ".md")
	ref = strings.TrimSpace(ref)
	for _, p := range strings.Fields(ref) {
		if n, err := strconv.Atoi(p); err == nil {
			return n, true
		}
	}
	return 0, false
}

// ValidateSections checks the content of an issue file against RequiredSections
// and DisallowedSections for the given state. Returns PreFlightIssue entries for
// any missing required sections or present disallowed sections.
func ValidateSections(content string, state State) []PreFlightIssue {
	sections, err := ParseIssueSections(content)
	if err != nil {
		return nil
	}

	var issues []PreFlightIssue

	for _, req := range RequiredSections {
		if _, ok := sections[strings.ToLower(req)]; !ok {
			issues = append(issues, PreFlightIssue{
				Severity: SeverityWarning,
				Message:  fmt.Sprintf("missing required section %q", req),
			})
		}
	}

	if disallowed, ok := DisallowedSections[state]; ok {
		for _, dis := range disallowed {
			if _, found := sections[strings.ToLower(dis)]; found {
				issues = append(issues, PreFlightIssue{
					Severity: SeverityWarning,
					Message:  fmt.Sprintf("section %q is not allowed in state %q (pre-populated disallowed section)", dis, state),
				})
			}
		}
	}

	return issues
}

// ValidateIssueFormat checks issue content for format compliance.
// Returns a list of human-readable messages describing any issues found:
// missing required sections or extra unrecognized sections.
func ValidateIssueFormat(content string) []string {
	sections, err := ParseIssueSections(content)
	if err != nil {
		return nil
	}

	var issues []string

	for _, req := range RequiredSections {
		if _, ok := sections[strings.ToLower(req)]; !ok {
			issues = append(issues, fmt.Sprintf("missing required section %q", req))
		}
	}

	known := make(map[string]bool, len(KnownSections))
	for _, s := range KnownSections {
		known[strings.ToLower(s)] = true
	}
	for name := range sections {
		if !known[name] {
			issues = append(issues, fmt.Sprintf("extra unrecognized section %q", name))
		}
	}

	return issues
}

var uatPlanColumns = []string{"Step", "Description", "Output", "Expected", "Result"}

// ValidateUATPlan checks that the UAT Plan section contains a markdown table
// with the correct column format: Step | Description | Output | Expected | Result.
func ValidateUATPlan(content string) error {
	plan := extractSection(content, "UAT plan")
	if plan == "" {
		return fmt.Errorf("UAT Plan section is missing or empty")
	}

	var headerLine, sepLine string
	for _, line := range strings.Split(plan, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "|") {
			continue
		}
		if headerLine == "" {
			headerLine = trimmed
		} else if sepLine == "" && strings.Contains(trimmed, "---") {
			sepLine = trimmed
		}
		if headerLine != "" && sepLine != "" {
			break
		}
	}

	if headerLine == "" {
		return fmt.Errorf("UAT Plan does not contain a markdown table header")
	}

	cols := parseTableRow(headerLine)
	if len(cols) != len(uatPlanColumns) {
		return fmt.Errorf("UAT Plan table has %d columns, expected %d", len(cols), len(uatPlanColumns))
	}
	for i, expected := range uatPlanColumns {
		if !strings.EqualFold(strings.TrimSpace(cols[i]), expected) {
			return fmt.Errorf("UAT Plan table column %d: expected %q, got %q", i+1, expected, cols[i])
		}
	}

	if sepLine == "" {
		return fmt.Errorf("UAT Plan table is missing the separator row after the header")
	}
	sepCols := parseTableRow(sepLine)
	if len(sepCols) != len(uatPlanColumns) {
		return fmt.Errorf("UAT Plan table separator row has %d columns, expected %d", len(sepCols), len(uatPlanColumns))
	}

	return nil
}

// ValidateAcceptanceCriteria checks that the Acceptance Criteria section
// contains only markdown checkbox items (- [ ] or - [x]).
func ValidateAcceptanceCriteria(content string) error {
	ac := extractSection(content, "Acceptance criteria")
	if ac == "" {
		return fmt.Errorf("Acceptance Criteria section is missing or empty")
	}

	for _, line := range strings.Split(ac, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if !strings.HasPrefix(trimmed, "- [ ]") && !strings.HasPrefix(trimmed, "- [x]") && !strings.HasPrefix(trimmed, "- [X]") {
			return fmt.Errorf("Acceptance criteria item does not use markdown checkbox format: %q", trimmed)
		}
	}

	return nil
}

func parseTableRow(line string) []string {
	var cols []string
	for _, p := range strings.Split(line, "|") {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			cols = append(cols, trimmed)
		}
	}
	return cols
}

// DetectDuplicateFilenames returns PreFlightIssue entries for files that share
// a basename across different non-quarantine state directories. Quarantine
// files are excluded since they are intentionally duplicates.
func DetectDuplicateFilenames(state *PipelineState) []PreFlightIssue {
	allPaths := state.TodoFiles
	allPaths = append(allPaths, state.TestReadyFiles...)
	allPaths = append(allPaths, state.ReadyForAgentFiles...)
	allPaths = append(allPaths, state.DoneFiles...)

	byBasename := make(map[string][]string)
	for _, p := range allPaths {
		base := filepath.Base(p)
		byBasename[base] = append(byBasename[base], p)
	}

	var issues []PreFlightIssue
	for base, paths := range byBasename {
		if len(paths) > 1 {
			issues = append(issues, PreFlightIssue{
				FilePath: paths[0],
				Severity: SeverityError,
				Message:  fmt.Sprintf("duplicate filename %q appears in multiple states: %s", base, strings.Join(paths, ", ")),
			})
		}
	}
	return issues
}

// DetectDuplicateGitHubNums returns PreFlightIssue entries for GitHub issue
// numbers that appear in more than one non-quarantine file. Quarantine files
// are excluded since they are intentionally duplicates.
func DetectDuplicateGitHubNums(state *PipelineState) []PreFlightIssue {
	allPaths := state.TodoFiles
	allPaths = append(allPaths, state.TestReadyFiles...)
	allPaths = append(allPaths, state.ReadyForAgentFiles...)
	allPaths = append(allPaths, state.DoneFiles...)

	ghNums := make(map[int][]string)
	for _, p := range allPaths {
		f, err := getCachedIssueFile(state, p)
		if err != nil {
			continue
		}
		if f.GitHubNum > 0 {
			ghNums[f.GitHubNum] = append(ghNums[f.GitHubNum], p)
		}
	}

	var issues []PreFlightIssue
	for num, paths := range ghNums {
		if len(paths) > 1 {
			issues = append(issues, PreFlightIssue{
				FilePath: paths[0],
				Severity: SeverityError,
				Message:  fmt.Sprintf("GitHub issue #%d appears in multiple files: %s", num, strings.Join(paths, ", ")),
			})
		}
	}
	return issues
}

// DetectDuplicateTitles returns PreFlightIssue entries for files that share
// the same issue title across different non-quarantine state directories or
// within the same state directory. Quarantine files are excluded since they
// are intentionally duplicates. This catches duplicates created by git
// operations (restore, merge, branch switch) where the same issue appears
// under different filenames.
func DetectDuplicateTitles(state *PipelineState) []PreFlightIssue {
	allPaths := state.TodoFiles
	allPaths = append(allPaths, state.TestReadyFiles...)
	allPaths = append(allPaths, state.ReadyForAgentFiles...)
	allPaths = append(allPaths, state.DoneFiles...)

	byTitle := make(map[string][]string)
	for _, p := range allPaths {
		f, err := getCachedIssueFile(state, p)
		if err != nil {
			continue
		}
		if f.Title == "" {
			continue
		}
		byTitle[f.Title] = append(byTitle[f.Title], p)
	}

	var issues []PreFlightIssue
	for title, paths := range byTitle {
		if len(paths) > 1 {
			issues = append(issues, PreFlightIssue{
				FilePath: paths[0],
				Severity: SeverityError,
				Message:  fmt.Sprintf("duplicate title %q appears in multiple files: %s", title, strings.Join(paths, ", ")),
			})
		}
	}
	return issues
}

// PreFlightCheck inspects the pipeline state for issues that would prevent
// normal operation: dead blocker references (blocked-by pointing to non-existent
// issues), self-referencing blockers, duplicate GitHub issue numbers, and
// missing Execution mode field.
// When repair is true, it attempts to auto-fix issues such as duplicate filenames
// by quarantining duplicates. When checksumsEnabled is true, it verifies checksums
// on files that have them.
func PreFlightCheck(state *PipelineState, repair bool, checksumsEnabled bool) []PreFlightIssue {
	var issues []PreFlightIssue
	if repair {
		_, qIssues := QuarantineDuplicates(state)
		issues = append(issues, qIssues...)
	}
	allPaths := state.TodoFiles
	allPaths = append(allPaths, state.TestReadyFiles...)
	allPaths = append(allPaths, state.ReadyForAgentFiles...)
	allPaths = append(allPaths, state.DoneFiles...)
	allPaths = append(allPaths, state.QuarantineFiles...)

	ghNums := make(map[int][]string)

	for _, p := range allPaths {
		if f, err := getCachedIssueFile(state, p); err == nil {
			if f.GitHubNum > 0 {
				ghNums[f.GitHubNum] = append(ghNums[f.GitHubNum], p)
			}
		}
	}

	issues = append(issues, DetectDuplicateFilenames(state)...)
	issues = append(issues, DetectDuplicateGitHubNums(state)...)
	issues = append(issues, DetectDuplicateTitles(state)...)

	allNums := make(map[int]bool)
	for num := range ghNums {
		allNums[num] = true
	}
	allBasenamePrefixes := make(map[string]bool)
	for _, p := range allPaths {
		base := strings.TrimSuffix(filepath.Base(p), ".md")
		parts := strings.SplitN(base, "-", 2)
		if len(parts) > 0 {
			if n, err := strconv.Atoi(parts[0]); err == nil {
				allBasenamePrefixes[strconv.Itoa(n)] = true
			}
		}
	}

	for _, p := range allPaths {
		st := StateFromPath(p)
		if st == StateDone || st == StateQuarantine || st == StateUnable {
			continue
		}
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		blockers := ParseBlockedBy(string(data))
		f, _ := getCachedIssueFile(state, p)
		for _, b := range blockers {
			num, ok := extractBlockerNum(b)
			if !ok {
				continue
			}
			if f != nil && f.GitHubNum == num {
				issues = append(issues, PreFlightIssue{
					FilePath: p,
					Severity: SeverityError,
					Message:  fmt.Sprintf("issue blocks itself (blocker references its own GitHub #%d)", num),
				})
				continue
			}
			if !allNums[num] {
				numStr := strconv.Itoa(num)
				if !allBasenamePrefixes[numStr] {
					issues = append(issues, PreFlightIssue{
						FilePath: p,
						Severity: SeverityError,
						Message:  fmt.Sprintf("blocked by non-existent GitHub issue #%d", num),
					})
				}
			}
		}
	}

	for _, p := range allPaths {
		f, err := getCachedIssueFile(state, p)
		if err != nil {
			continue
		}
		if f.ExecMode == "" {
			issues = append(issues, PreFlightIssue{
				FilePath: p,
				Severity: SeverityWarning,
				Message:  "missing Execution mode field",
			})
		} else if !IsValidExecMode(f.ExecMode) {
			issues = append(issues, PreFlightIssue{
				FilePath: p,
				Severity: SeverityWarning,
				Message:  fmt.Sprintf("invalid Execution mode value %q (must be AFK-only, HITL-only, or Combo)", f.ExecMode),
			})
		}
	}

	for _, p := range allPaths {
		f, err := getCachedIssueFile(state, p)
		if err != nil {
			continue
		}
		if f.State == StateDone || f.State == StateQuarantine || f.State == StateUnable {
			continue
		}
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		secIssues := ValidateSections(string(data), f.State)
		issues = append(issues, secIssues...)
	}

	for _, p := range state.TestReadyFiles {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if sectionHasContent(string(data), "UAT Results") {
			issues = append(issues, PreFlightIssue{
				FilePath: p,
				Severity: SeverityWarning,
				Message:  "file in test-ready/ has populated UAT Results section — already tested but never transitioned; file belongs in done/ or should be re-triaged manually",
			})
		}
	}

	if checksumsEnabled {
		for _, p := range allPaths {
			f, err := getCachedIssueFile(state, p)
			if err != nil {
				continue
			}
			if f.Checksum == "" {
				continue
			}
			valid, err := VerifyChecksum(p)
			if err != nil {
				continue
			}
			if !valid {
				issues = append(issues, PreFlightIssue{
					FilePath: p,
					Severity: SeverityWarning,
					Message:  "checksum mismatch — file content has been modified",
				})
			}
		}
	}

	return issues
}

func ParseIssueTitle(content string) string {
	lines := strings.SplitN(content, "\n", 2)
	if len(lines) == 0 {
		return ""
	}
	firstLine := strings.TrimSpace(lines[0])
	if !strings.HasPrefix(firstLine, "# ") {
		return ""
	}
	title := strings.TrimSpace(strings.TrimPrefix(firstLine, "# "))
	for _, sep := range []string{" - ", " — "} {
		parts := strings.SplitN(title, sep, 2)
		if len(parts) == 2 {
			if _, err := strconv.Atoi(strings.TrimSpace(parts[0])); err == nil {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return title
}

func Create(issuesDir string, state State, title, body string) (*Issue, error) {
	dir := stateDir(issuesDir, state)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create dir: %w", err)
	}

	filename := strings.ReplaceAll(strings.ToLower(title), " ", "-") + ".md"
	path := filepath.Join(dir, filename)

	fullBody := body
	if !strings.HasPrefix(body, "# ") {
		fullBody = "# " + title + "\n\n" + body
	}

	if err := os.WriteFile(path, []byte(fullBody), 0644); err != nil {
		return nil, fmt.Errorf("write file: %w", err)
	}

	return &Issue{
		FilePath: path,
		Title:    title,
		Body:     fullBody,
		State:    state,
	}, nil
}

func deriveIssuesDir(filePath string, state State) string {
	if state == StateTodo {
		return filepath.Dir(filePath)
	}
	return filepath.Dir(filepath.Dir(filePath))
}

// QuarantineDuplicates moves files with duplicate basenames across non-quarantine
// states (todo, test-ready, done) into .quarantine, keeping only the canonical
// copy (preferring later pipeline stages then newer files). It updates the
// PipelineState in-place to reflect the moves and returns the count of files
// moved plus PreFlightIssue entries describing each duplicate group.
func QuarantineDuplicates(state *PipelineState) (quarantined int, issues []PreFlightIssue) {
	allPaths := state.TodoFiles
	allPaths = append(allPaths, state.TestReadyFiles...)
	allPaths = append(allPaths, state.ReadyForAgentFiles...)
	allPaths = append(allPaths, state.DoneFiles...)

	byBasename := make(map[string][]string)
	for _, p := range allPaths {
		base := filepath.Base(p)
		byBasename[base] = append(byBasename[base], p)
	}

	quarantineSet := make(map[string]bool)
	for base, paths := range byBasename {
		if len(paths) <= 1 {
			continue
		}
		// Pick the canonical file (latest stage, then newest mtime).
		canonical, err := PickCanonical(paths)
		if err != nil {
			continue
		}
		issues = append(issues, PreFlightIssue{
			FilePath: canonical,
			Severity: SeverityWarning,
			Message:  fmt.Sprintf("quarantined duplicate filename %q (keeping canonical %s, appeared in: %s)", base, filepath.Base(canonical), strings.Join(paths, ", ")),
		})
		for _, p := range paths {
			if p != canonical {
				quarantineSet[p] = true
			}
		}
	}

	if len(quarantineSet) == 0 {
		return 0, nil
	}

	var firstPath string
	for p := range quarantineSet {
		firstPath = p
		break
	}
	st := StateFromPath(firstPath)
	rootDir := deriveIssuesDir(firstPath, st)
	quarantineDir := stateDir(rootDir, StateQuarantine)

	if err := os.MkdirAll(quarantineDir, 0755); err != nil {
		issues = append(issues, PreFlightIssue{
			FilePath: quarantineDir,
			Severity: SeverityError,
			Message:  fmt.Sprintf("cannot create quarantine directory: %v", err),
		})
		return 0, issues
	}

	var remainingTodo, remainingTestReady, remainingReadyForAgent, remainingDone []string
	for _, p := range state.TodoFiles {
		if !quarantineSet[p] {
			remainingTodo = append(remainingTodo, p)
		}
	}
	state.TodoFiles = remainingTodo

	for _, p := range state.TestReadyFiles {
		if !quarantineSet[p] {
			remainingTestReady = append(remainingTestReady, p)
		}
	}
	state.TestReadyFiles = remainingTestReady

	for _, p := range state.ReadyForAgentFiles {
		if !quarantineSet[p] {
			remainingReadyForAgent = append(remainingReadyForAgent, p)
		}
	}
	state.ReadyForAgentFiles = remainingReadyForAgent

	for _, p := range state.DoneFiles {
		if !quarantineSet[p] {
			remainingDone = append(remainingDone, p)
		}
	}
	state.DoneFiles = remainingDone

	ts := time.Now().Unix()
	seq := 0
	for p := range quarantineSet {
		seq++
		dst := filepath.Join(quarantineDir, fmt.Sprintf("%d_%d_%s", ts, seq, filepath.Base(p)))
		if err := moveFile(p, dst); err != nil {
			issues = append(issues, PreFlightIssue{
				FilePath: p,
				Severity: SeverityError,
				Message:  fmt.Sprintf("failed to quarantine: %v", err),
			})
			continue
		}
		state.QuarantineFiles = append(state.QuarantineFiles, dst)
		quarantined++
	}

	return quarantined, issues
}

// QuarantineDuplicateGitHubNums moves files that share the same GitHub issue
// number across non-quarantine states into .quarantine, keeping only the
// canonical copy (preferring later pipeline stages then newer files). It
// updates the PipelineState in-place and returns the count of files moved
// plus PreFlightIssue entries describing each duplicate group.
func QuarantineDuplicateGitHubNums(state *PipelineState) (quarantined int, issues []PreFlightIssue) {
	allPaths := state.TodoFiles
	allPaths = append(allPaths, state.TestReadyFiles...)
	allPaths = append(allPaths, state.ReadyForAgentFiles...)
	allPaths = append(allPaths, state.DoneFiles...)

	byNum := make(map[int][]string)
	for _, p := range allPaths {
		f, err := ParseIssueFile(p)
		if err != nil {
			continue
		}
		if f.GitHubNum > 0 {
			byNum[f.GitHubNum] = append(byNum[f.GitHubNum], p)
		}
	}

	quarantineSet := make(map[string]bool)
	for num, paths := range byNum {
		if len(paths) <= 1 {
			continue
		}
		canonical, err := PickCanonical(paths)
		if err != nil {
			continue
		}
		issues = append(issues, PreFlightIssue{
			FilePath: canonical,
			Severity: SeverityWarning,
			Message:  fmt.Sprintf("quarantined duplicate GitHub #%d (keeping canonical %s, appeared in: %s)", num, filepath.Base(canonical), strings.Join(paths, ", ")),
		})
		for _, p := range paths {
			if p != canonical {
				quarantineSet[p] = true
			}
		}
	}

	return quarantineNonCanonical(state, quarantineSet, issues)
}

// QuarantineDuplicateTitles moves files that share the same issue title across
// non-quarantine states into .quarantine, keeping only the canonical copy
// (preferring later pipeline stages then newer files). It updates the
// PipelineState in-place and returns the count of files moved plus
// PreFlightIssue entries describing each duplicate group.
func QuarantineDuplicateTitles(state *PipelineState) (quarantined int, issues []PreFlightIssue) {
	allPaths := state.TodoFiles
	allPaths = append(allPaths, state.TestReadyFiles...)
	allPaths = append(allPaths, state.ReadyForAgentFiles...)
	allPaths = append(allPaths, state.DoneFiles...)

	byTitle := make(map[string][]string)
	for _, p := range allPaths {
		f, err := ParseIssueFile(p)
		if err != nil {
			continue
		}
		if f.Title == "" {
			continue
		}
		byTitle[f.Title] = append(byTitle[f.Title], p)
	}

	quarantineSet := make(map[string]bool)
	for title, paths := range byTitle {
		if len(paths) <= 1 {
			continue
		}
		canonical, err := PickCanonical(paths)
		if err != nil {
			continue
		}
		issues = append(issues, PreFlightIssue{
			FilePath: canonical,
			Severity: SeverityWarning,
			Message:  fmt.Sprintf("quarantined duplicate title %q (keeping canonical %s, appeared in: %s)", title, filepath.Base(canonical), strings.Join(paths, ", ")),
		})
		for _, p := range paths {
			if p != canonical {
				quarantineSet[p] = true
			}
		}
	}

	return quarantineNonCanonical(state, quarantineSet, issues)
}

// quarantineNonCanonical is a shared helper that moves files in quarantineSet
// to the .quarantine directory with a timestamp prefix and updates the
// PipelineState in-place. It returns the count of files moved plus any errors
// appended to the provided PreFlightIssue slice.
func quarantineNonCanonical(state *PipelineState, quarantineSet map[string]bool, issues []PreFlightIssue) (int, []PreFlightIssue) {
	if len(quarantineSet) == 0 {
		return 0, issues
	}

	var firstPath string
	for p := range quarantineSet {
		firstPath = p
		break
	}
	st := StateFromPath(firstPath)
	rootDir := deriveIssuesDir(firstPath, st)
	quarantineDir := stateDir(rootDir, StateQuarantine)

	if err := os.MkdirAll(quarantineDir, 0755); err != nil {
		issues = append(issues, PreFlightIssue{
			FilePath: quarantineDir,
			Severity: SeverityError,
			Message:  fmt.Sprintf("cannot create quarantine directory: %v", err),
		})
		return 0, issues
	}

	var remainingTodo, remainingTestReady, remainingReadyForAgent, remainingDone []string
	for _, p := range state.TodoFiles {
		if !quarantineSet[p] {
			remainingTodo = append(remainingTodo, p)
		}
	}
	state.TodoFiles = remainingTodo

	for _, p := range state.TestReadyFiles {
		if !quarantineSet[p] {
			remainingTestReady = append(remainingTestReady, p)
		}
	}
	state.TestReadyFiles = remainingTestReady

	for _, p := range state.ReadyForAgentFiles {
		if !quarantineSet[p] {
			remainingReadyForAgent = append(remainingReadyForAgent, p)
		}
	}
	state.ReadyForAgentFiles = remainingReadyForAgent

	for _, p := range state.DoneFiles {
		if !quarantineSet[p] {
			remainingDone = append(remainingDone, p)
		}
	}
	state.DoneFiles = remainingDone

	ts := time.Now().Unix()
	seq := 0
	quarantined := 0
	for p := range quarantineSet {
		seq++
		dst := filepath.Join(quarantineDir, fmt.Sprintf("%d_%d_%s", ts, seq, filepath.Base(p)))
		if err := moveFile(p, dst); err != nil {
			issues = append(issues, PreFlightIssue{
				FilePath: p,
				Severity: SeverityError,
				Message:  fmt.Sprintf("failed to quarantine: %v", err),
			})
			continue
		}
		state.QuarantineFiles = append(state.QuarantineFiles, dst)
		quarantined++
	}

	return quarantined, issues
}

// getCachedIssueFile returns the parsed IssueFile for path, using the cached
// Files map in state if available, otherwise falling back to ParseIssueFile.
// This allows functions to benefit from the cache when populated via ScanIssueDir
// while still working with manually-constructed PipelineState (e.g. in tests).
func getCachedIssueFile(state *PipelineState, path string) (*IssueFile, error) {
	if state.Files != nil {
		if f, ok := state.Files[path]; ok {
			return f, nil
		}
	}
	return ParseIssueFile(path)
}

// QuarantineAll detects AND quarantines duplicates across all three dimensions
// (filename, GitHub number, title) in a single pass. It builds a combined
// quarantine set, then calls quarantineNonCanonical once to move all
// non-canonical files to .quarantine. Returns the total count of files moved
// and all PreFlightIssue entries describing each duplicate group.
func QuarantineAll(state *PipelineState) (quarantined int, issues []PreFlightIssue) {
	allPaths := state.TodoFiles
	allPaths = append(allPaths, state.TestReadyFiles...)
	allPaths = append(allPaths, state.ReadyForAgentFiles...)
	allPaths = append(allPaths, state.DoneFiles...)

	quarantineSet := make(map[string]bool)

	// 1. Filename collisions: same basename in different state dirs.
	byBasename := make(map[string][]string)
	for _, p := range allPaths {
		base := filepath.Base(p)
		byBasename[base] = append(byBasename[base], p)
	}
	for base, paths := range byBasename {
		if len(paths) <= 1 {
			continue
		}
		canonical, err := PickCanonical(paths)
		if err != nil {
			continue
		}
		issues = append(issues, PreFlightIssue{
			FilePath: canonical,
			Severity: SeverityWarning,
			Message:  fmt.Sprintf("quarantined duplicate filename %q (keeping canonical %s, appeared in: %s)", base, filepath.Base(canonical), strings.Join(paths, ", ")),
		})
		for _, p := range paths {
			if p != canonical {
				quarantineSet[p] = true
			}
		}
	}

	// 2. GitHub num collisions (using ps.Files cache).
	byNum := make(map[int][]string)
	for _, p := range allPaths {
		if quarantineSet[p] {
			continue
		}
		f, err := getCachedIssueFile(state, p)
		if err != nil {
			continue
		}
		if f.GitHubNum > 0 {
			byNum[f.GitHubNum] = append(byNum[f.GitHubNum], p)
		}
	}
	for num, paths := range byNum {
		if len(paths) <= 1 {
			continue
		}
		canonical, err := PickCanonical(paths)
		if err != nil {
			continue
		}
		issues = append(issues, PreFlightIssue{
			FilePath: canonical,
			Severity: SeverityWarning,
			Message:  fmt.Sprintf("quarantined duplicate GitHub #%d (keeping canonical %s, appeared in: %s)", num, filepath.Base(canonical), strings.Join(paths, ", ")),
		})
		for _, p := range paths {
			if p != canonical {
				quarantineSet[p] = true
			}
		}
	}

	// 3. Title collisions (using ps.Files cache).
	byTitle := make(map[string][]string)
	for _, p := range allPaths {
		if quarantineSet[p] {
			continue
		}
		f, err := getCachedIssueFile(state, p)
		if err != nil {
			continue
		}
		if f.Title == "" {
			continue
		}
		byTitle[f.Title] = append(byTitle[f.Title], p)
	}
	for title, paths := range byTitle {
		if len(paths) <= 1 {
			continue
		}
		canonical, err := PickCanonical(paths)
		if err != nil {
			continue
		}
		issues = append(issues, PreFlightIssue{
			FilePath: canonical,
			Severity: SeverityWarning,
			Message:  fmt.Sprintf("quarantined duplicate title %q (keeping canonical %s, appeared in: %s)", title, filepath.Base(canonical), strings.Join(paths, ", ")),
		})
		for _, p := range paths {
			if p != canonical {
				quarantineSet[p] = true
			}
		}
	}

	if len(quarantineSet) == 0 {
		return 0, issues
	}

	n, qIssues := quarantineNonCanonical(state, quarantineSet, issues)
	return n, qIssues
}
