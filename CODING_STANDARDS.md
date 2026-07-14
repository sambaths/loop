# Coding Standards

Standards for Go code in the `loop` repository. These codify patterns already in use and set expectations for new contributions.

---

## 1. Project Layout

```
cmd/loop/           # Single binary entry point — thin main(), imports from internal/
internal/
  agent/            # opencode subprocess management
  config/           # Configuration (JSON read/write)
  gh/               # Thin gh CLI wrappers
  git/              # Git safety (stash, branch, context)
  github/           # Higher-level GitHub operations
  issue/            # Issue file state machine
  pipeline/         # Legacy orchestration
  prompt/           # Embedded agent prompt
  runner/           # Current orchestration layer
  tui/              # Bubbletea components (dashboard, output, run, setup, status, screenshot)
```

- **One package per directory.** No `internal/pkg/` or versioned dirs.
- **Platform-specific files** use build tags: `agent_unix.go` (`//go:build !windows`), `agent_windows.go` (`//go:build windows`). Same package, same types, different implementations.
- **`cmd/loop/main.go`** is `package main` with `var Version = "dev"` overridden via `-ldflags` at build time.
- **`testdata/`** at repo root for shared test fixtures (e.g., mock shell scripts).
- **Shared test helpers** go in a `testhelper` subpackage under the relevant package (e.g., `internal/git/testhelper/`).

---

## 2. Naming Conventions

### Types, Functions, Variables

| Scope | Convention | Examples |
|---|---|---|
| Exported types | PascalCase | `PipelineState`, `IssueFile`, `FileMoveError`, `TransitionEvent` |
| Unexported types | camelCase | `page`, `step`, `model` |
| Exported constants | PascalCase | `StateTodo`, `StateTestReady`, `RoleImplement`, `RoleTest` |
| Unexported constants | camelCase | `maxMoveRetries`, `stashPrefix`, `sentinelStart` |
| Functions | Verb or verb phrase | `ScanIssueDir`, `ParseIssueFile`, `RunIteration`, `Move` |
| Method receivers | 1-2 letter abbreviation | `m` for model, `r` for runner, `c` for config |

### Acronyms and Initialisms

All-caps in PascalCase names:

```go
GitHubNum     // not GithubNum
ExecMode      // not Execmode
AFKOnly       // not AfkOnly
HITLOnly      // not HitlOnly
HTTP          // from http.Get
JSON          // from json.Marshal
ID            // not Id
URL           // not Url
```

### Files

- **Source files**: lowercase with underscores for multi-word: `agent_unix.go`, `agent_windows.go`.
- **Test files**: `*_test.go` next to source: `config_test.go`, `agent_test.go`.
- **Internal tests**: use `package foo` (not `package foo_test`).

---

## 3. Code Style & Formatting

### Formatting

- All Go code **must** pass `gofmt` / `gofumpt`. Format on save.
- Imports are organized in **three groups** separated by blank lines:
  1. Standard library (alphabetical)
  2. Third-party (alphabetical)
  3. Internal (`github.com/sambaths/loop/...`) (alphabetical)

```go
import (
    "context"
    "errors"
    "fmt"
    "os"

    tea "github.com/charmbracelet/bubbletea"
    "github.com/charmbracelet/lipgloss"

    "github.com/sambaths/loop/internal/config"
    "github.com/sambaths/loop/internal/issue"
)
```

- Aliased imports: `tea "github.com/charmbracelet/bubbletea"` (alias to `tea`).
- No blank imports.

### Pointers and Receivers

- **Pointer receivers** for methods that mutate state: `RunIteration`, `Update`, `View`.
- **Value receivers** for read-only methods on small structs: `Init()`, `headerView()`.

```go
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd)  // mutating
func (m Model) Init() tea.Cmd                               // const
```

### Zero Values

Design types so the zero value is usable or a safe default:

```go
var buf strings.Builder
var f IssueFile
```

### String Building

Use `strings.Builder` for concatenation in hot paths. Use `bytes.Buffer` for stdout/stderr accumulation.

### Paths

Always use `filepath.Join` (never string concatenation for paths).

```go
path := filepath.Join(root, ConfigDirName, ConfigFileName)
```

### File I/O

Prefer `os.ReadFile` / `os.WriteFile` (Go 1.16+) over older `ioutil` equivalents.

File permissions: `0644` for files, `0755` for directories and scripts.

### JSON

Use `json.MarshalIndent(cfg, "", "  ")` for config files, `json.NewEncoder(f).Encode(event)` for log files.

### Enums

Use `iota` for typed enums with a default zero value for "unknown":

```go
type page int

const (
    pageOverview page = iota
    pageTodo
    pageTestReady
    pageDone
    pageQuarantine
    pageCount
)

type State string

const (
    StateUnknown       State = ""
    StateTodo          State = "todo"
    StateTestReady     State = "test-ready"
    StateDone          State = "done"
)
```

---

## 4. Error Handling

### Sentinel Errors

Define at package level with `errors.New`:

```go
var ErrOpencodeNotFound = errors.New("opencode binary not found in PATH")
var ErrNoIssues         = fmt.Errorf("no issues available")
var ErrPreFlightFailed  = fmt.Errorf("pre-flight checks failed")
```

Check with `errors.Is`, never `==`:

```go
if errors.Is(err, issue.ErrNoIssues) { ... }
```

### Error Wrapping

Wrap errors with `fmt.Errorf("context: %w", err)` (use `%w`, never `%v` or `%s`):

```go
return nil, fmt.Errorf("read config: %w", err)
return nil, fmt.Errorf("parse config: %w", err)
```

### Error Strings

- **Internal/programming errors**: lowercase, no trailing punctuation — `"scan issue dir: %w"`.
- **User-facing messages**: start uppercase — `"No issues found in pipeline"`.

### Custom Error Types

Implement `Error() string` and `Unwrap()` for structured errors:

```go
type FileMoveError struct {
    Src        string
    Dst        string
    Err        error
    Suggestion string
}

func (e *FileMoveError) Error() string { return e.Err.Error() }
func (e *FileMoveError) Unwrap() error  { return e.Err }
```

Check with `errors.As`:

```go
var linkErr *os.LinkError
if errors.As(err, &linkErr) && errors.Is(linkErr.Err, syscall.EXDEV) { ... }
```

### Error Accumulation

Use `errors.Join` to collect multiple non-fatal errors:

```go
var errs []error
errs = append(errs, ...)
return errors.Join(errs...)
```

### Retry Logic

Retry transient errors with backoff. Use a typed helper function for retryable checks:

```go
const maxMoveRetries = 3

func moveRetryDelay(attempt int) time.Duration {
    return time.Duration(100*(attempt+1)) * time.Millisecond
}

func isRetryableError(err error) bool {
    if errors.Is(err, os.ErrPermission) { return true }
    if errors.Is(err, syscall.EACCES) || errors.Is(err, syscall.EPERM) { return true }
    return errors.Is(err, syscall.EBUSY) || errors.Is(err, syscall.ETXTBSY)
}
```

### Indentation

Handle errors first, return early. Normal path stays at minimal indentation:

```go
data, err := os.ReadFile(path)
if err != nil {
    return nil, fmt.Errorf("read config: %w", err)
}
// happy path continues here, un-nested
```

---

## 5. Testing

### Framework

Use the standard `testing` package only. No external test frameworks (no testify, no gomega, no assert).

### Table-Driven Tests

The primary pattern:

```go
func TestParseOutputComplete(t *testing.T) {
    tests := []struct {
        name    string
        content string
        want    Outcome
    }{
        {"complete", "some output\n__LOOP_RESULT__\nCOMPLETE\n__LOOP_RESULT_END__", OutcomeComplete},
        {"not found", "no markers here", ""},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, found := ParseOutput(tt.content)
            if tt.want == "" && found {
                t.Errorf("expected not found, got %q", got)
            }
            if tt.want != "" && !found {
                t.Errorf("expected %q, not found", tt.want)
            }
            if tt.want != "" && got != tt.want {
                t.Errorf("ParseOutput() = %q, want %q", got, tt.want)
            }
        })
    }
}
```

### Test Helpers

- Call `t.Helper()` at the top of every shared test helper.
- Use `t.TempDir()` for temporary directories (no manual cleanup).
- Use `t.Setenv()` for environment variable mocking.

### Mocking

Use **function variable substitution** (save/restore pattern):

```go
// Production code:
var execCommand = exec.Command
var execCommandContext = exec.CommandContext

// Test:
func TestSomething(t *testing.T) {
    saved := execCommandContext
    defer func() { execCommandContext = saved }()

    execCommandContext = func(ctx context.Context, name string, arg ...string) *exec.Cmd {
        return exec.CommandContext(ctx, "echo", "mocked output")
    }
    // ... test logic ...
}
```

Also used for `osExit`, `maxOutputSize`, and other function boundaries:

```go
var osExit = os.Exit
var runCompletionFn = printBashCompletion
var runSetupFn = runSetup
var runTUIFn = runTUI
```

### Failure Messages

Format: `t.Errorf("Foo(%v) = %v, want %v", in, got, want)` — got before want.

Prefer `t.Error` (continue) over `t.Fatal` (stop) to report all failures in one run. Use `t.Fatal` only when the remainder of the test cannot proceed safely.

### Error Export Tests

Verify sentinel errors are accessible and non-nil:

```go
func TestRunnerExportErrors(t *testing.T) {
    if ErrNoIssues == nil {
        t.Error("ErrNoIssues must be non-nil")
    }
}
```

### Golden Files

For complex output verification, use `testdata/` files at repo root.

---

## 6. Bubbletea TUI Patterns

All TUI components follow the standard Elm Architecture contract:

```go
type Model struct { /* state */ }

func (m Model) Init() tea.Cmd
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd)
func (m Model) View() string
```

### Structure

- **Model struct**: exported fields for external access, unexported fields for internal state.
- **Constructor**: `NewModel(cfg config.Config) tea.Model` — returns `tea.Model` interface.
- **Custom message types**: exported structs in the TUI package:

```go
// Messages
type ProgressMsg struct { ... }
type CompletionMsg struct { ... }
type LineMsg struct{ Line string }

// Unexported
type tickMsg time.Time
type doneWaitingMsg struct{}
```

### Channel-Based Streaming

The pattern for piping work from a goroutine into the Bubbletea event loop:

```go
func NewStreamingModel(cfg config.Config, n int, stop context.CancelFunc,
    runFn func(logChan chan<- string, iterChan chan<- ProgressMsg, doneChan chan<- error)) Model {

    m := Model{
        cfg:      cfg,
        lineChan: make(chan string, 100),
        iterChan: make(chan ProgressMsg, 10),
        doneChan: make(chan error, 1),
    }
    m.runFn = runFn
    return m
}

func (m Model) Init() tea.Cmd {
    go m.runFn(m.lineChan, m.iterChan, m.doneChan)
    return tea.Batch(m.listen(), m.listenIter())
}

func (m Model) listen() tea.Cmd {
    return func() tea.Msg {
        line, ok := <-m.lineChan
        if !ok { return nil }
        return LineMsg{Line: line}
    }
}
```

### Lipgloss Styling

Define styles as package-level `var` blocks:

```go
var (
    titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00FF00"))
    headerStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00FF00"))
    countStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFF00"))
    helpStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
    todoStyle    = lipgloss.NewStyle().Background(lipgloss.Color("#6A5ACD"))
)
```

### Key Handling

Handle `tea.KeyMsg` in `Update` with a type switch, then a string switch:

```go
case tea.KeyMsg:
    switch msg.String() {
    case "q", "ctrl+c":
        m.quit = true
        return m, tea.Quit
    case "tab", "right", "l", "n":
        if m.page < pageCount-1 { m.page++ }
    }
```

### Quit

Set `m.quit = true` and return `tea.Quit`. The `View()` returns `""` when quitting. Some models signal cancellation first via `context.CancelFunc`.

---

## 7. Git & GitHub Integration

### Shell-Out Pattern

Wrapping `git` and `gh` CLIs (no Go git libraries):

```go
func RunGit(args ...string) (stdout, stderr string, err error) {
    cmd := exec.Command("git", args...)
    // ...
}
```

### Authentication

GitHub auth is checked once per process via `sync.Once`:

```go
var authOnce sync.Once

func CheckAuthOnce() bool {
    authOnce.Do(func() {
        _, err := RunGH("auth", "status")
        // cache result
    })
    return cached
}

func ResetAuthCheck() { authOnce = sync.Once{} } // for tests
```

### Context Save/Restore

Save git context (current branch + stash dirty changes) before agent runs, restore after:

```go
func SaveContext() (restore func(), err error) { ... }
func RestoreContextFromFile() error { ... }
```

Context is persisted as JSON in `.loop/git-context.json`.

### Repo Type

```go
type Repo struct {
    Owner string
    Name  string
}

func RepoFromString(s string) (Repo, error) { ... }
func (r Repo) String() string { return fmt.Sprintf("%s/%s", r.Owner, r.Name) }
```

### Transient Error Detection

```go
func isTransientOutput(errStr string) bool {
    patterns := []string{"connection refused", "i/o timeout", "dial tcp"}
    for _, p := range patterns {
        if strings.Contains(errStr, p) { return true }
    }
    return false
}
```

---

## 8. Documentation

### Doc Comments

Every exported name must have a doc comment:

```go
// PipelineState holds the lists of issue files grouped by pipeline stage
// and a parsed-files cache keyed by path.
type PipelineState struct { ... }

// RunAgentContextStreamed is like RunAgentContext but streams stdout lines
// to lineFn as they are produced, while still buffering for promise parsing.
func RunAgentContextStreamed(...) { ... }
```

Doc comments begin with the declared name and end with a period.

### Package Comments

First line of every package file: `// Package agent manages opencode subprocess execution.`

### Inline Comments

Explain **why**, not **what**:

```go
// Reorder args so flags come before positional args.
// Go's flag package stops parsing at the first non-flag arg,
// so `loop run 1 --headless` would fail without this reorder.

// Populate Files cache after initial scan so parsing errors in individual
// files don't block the scan itself.
```

### Section Comments in Tests

Use `// --- RunLoop tests ---` style separators in long test files.

---

## 9. Concurrency

### Context as First Param

`context.Context` is passed as the first parameter for cancellation throughout:

```go
func RunAgentContext(ctx context.Context, ...) (*Result, error)
func RunLoopContext(ctx context.Context, ...) error
func runContent(ctx context.Context, ...) (*Result, error)
```

Never store `context.Context` in a struct.

### Goroutine Lifecycle

Document when/if goroutines exit. Use `sync.WaitGroup` for coordination:

```go
var wg sync.WaitGroup
wg.Add(2)
go func() {
    defer wg.Done()
    // ...
}()
wg.Wait()
```

### Channel Naming

Suffix channels with their role: `lineChan`, `iterChan`, `doneChan`.

### Signal Handling

```go
ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
defer stop()
```

---

## 10. Linting & Tooling

### Current Setup

- `go vet ./...` in the Makefile (no third-party linter configured).
- No `.golangci.yml` or `.editorconfig` yet.

### Recommended `.golangci.yml`

```yaml
version: "2"
linters:
  enable:
    - bodyclose
    - exhaustive
    - goconst
    - godot
    - gosec
    - misspell
    - nakedret
    - nilerr
    - noctx
    - revive
    - unconvert
    - unparam
    - whitespace
    - wrapcheck
    - prealloc
formatters:
  enable:
    - gofumpt
    - goimports
```

### CI Gate

Every PR should pass: `gofumpt` → `go vet` → `go test ./...`.

---

## 11. Dependencies

- Only the Charm ecosystem: `bubbletea`, `bubbles`, `lipgloss`.
- No external dependencies beyond Charm.
- No test dependencies — all tests use stdlib only.
- Keep the dependency tree minimal.
