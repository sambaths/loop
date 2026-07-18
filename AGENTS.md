# loop — Exhaustive LLM Reference

loop is a Go TUI that runs an AFK (Away From Keyboard) implement-then-test cycle over issues using `opencode` as the agent CLI. Follows the [Matt Pocock engineering skills](https://github.com/mattpocock) workflow pattern — adapted into a standalone Go binary.

## Glossary

- **loop**: The Go TUI binary that orchestrates the AFK issue pipeline.
- **Issue**: A unit of work as a local markdown file, stored under the issue directory and optionally mirrored to GitHub.
- **Issue directory**: The directory (`docs/issues/` by default) with subdirectories representing pipeline state.
- **Promise marker**: A machine-readable token in agent stdout: `COMPLETE`, `TEST_PASS`, `TEST_FAIL`, or `NO_MORE_TASKS`. Wrapped in `__LOOP_RESULT__` / `__LOOP_RESULT_END__` sentinel lines.
- **Agent**: The external CLI (`opencode run --dangerously-skip-permissions`) that loop invokes.
- **GitHub repo**: Optional remote. If configured, loop mirrors file transitions to GitHub. If not, loop runs local-only.
- **Setup wizard**: The TUI (`loop setup`) that collects config on first run.
- **State**: The pipeline stage of an issue, determined by its parent directory.
- **Role**: Either `implement` (building code) or `test` (validating), determined by the issue's pipeline state.
- **Execution mode**: Controls whether an issue can be processed autonomously: `AFK-only`, `HITL-only`, `Combo`.
- **Retry**: Counter incremented when an issue fails testing or the agent is killed by the inactivity watchdog. Max 5 retries before moving to `unable/`.
- **Checksum**: SHA-256 hash of issue file content (excluding the `Checksum:` line itself), used to detect manual edits.
- **Pipeline**: The set of all issue files across all state directories.
- **Test-ready**: State for issues that have been implemented and await validation.
- **Done**: State for issues that passed validation.
- **Quarantine**: State for duplicate files or agent failures held for manual review.
- **Unable**: State for issues that exceeded the maximum retry count.

## Quickstart

```bash
loop setup          # interactive setup wizard (creates .loop/config.json)
loop run 10         # run 10 AFK iterations
loop status         # show pipeline state
loop                # open the TUI dashboard
```

## CLI Command Reference

All commands are parsed in `cmd/loop/main.go` using Go's standard `flag` package with manual dispatch. There is no Cobra.

### Global Flags

| Flag | Type | Default | Description |
|---|---|---|---|
| `--help` / `-h` | bool | false | Show help message |
| `--version` | bool | false | Show version (`loop v<ver> <os>/<arch>`) |
| `--timeout <secs>` | int | 0 | Agent timeout in seconds (overrides `agent_timeout` in config) |
| `--repair [=true/false]` | bool | true | Repair GitHub state on each iteration (default true; use `--repair=false` to disable) |
| `--headless [=true/false]` | bool | false | Run without TUI (for scripting/CI) |

Flags can appear before or after positional arguments.

### Commands

#### `loop` (no subcommand)

Start the Bubbletea TUI dashboard. Shows 5 tabbed pages:
1. Pipeline overview (counts by state, pipeline bar, recent transitions, warnings)
2. Todo issues list
3. Test-ready issues list
4. Done issues list
5. Quarantined issues list

Keyboard navigation: `q`/`ctrl+c` to quit, `tab`/`right`/`l`/`n` for next page, `shift+tab`/`left`/`h`/`p` for prev page, `s` to save screenshot, arrows/pgup/pgdn for scrolling.

Auto-launches setup wizard if no config exists.

#### `loop setup`

Interactive TUI setup wizard. Collects:
1. Issue directory path (default: `docs/issues`)
2. GitHub repository (optional, e.g. `my-org/my-repo`; blank = local-only)
3. Default branch (default: `main`)

Validates `gh` CLI availability/authentication if GitHub repo given. Creates `.loop/config.json`. Ensures `.loop/` is in `.gitignore`. Skips if config already exists.

#### `loop run <n> [issue-number]`

Run N AFK iterations. Each iteration:
1. Scans issue directory, quarantines duplicates, runs pre-flight checks
2. Selects next issue (priority: test-ready → ready-for-agent → todo)
3. Stashes working tree, creates temp branch (`loop/<slug>`)
4. Invokes `opencode run --dangerously-skip-permissions` with issue content + embedded prompt
5. Parses agent output for promise markers
6. Transitions file between state directories
7. Syncs GitHub labels (if configured)
8. Restores original branch and stash

If `issue-number` is provided, forces that specific issue on iteration 1.

Example: `loop run 5`, `loop run 10 42`

#### `loop status`

Opens a read-only TUI status viewer with the same 5-page layout as the dashboard (but titled "loop status").

#### `loop check`

Validates pipeline state. Runs pre-flight checks:
- Duplicate filenames across state directories
- Duplicate GitHub issue numbers
- Duplicate titles
- Dead blocker references
- Self-referencing blockers
- Missing/invalid Execution mode header
- Missing required sections
- Disallowed sections present
- UAT Plan table format validation
- Acceptance criteria checkbox format
- Header field validation
- Title format validation
- Checksum verification (if enabled)

Also cleans disallowed result sections. If GitHub configured, reopens prematurely closed issues.

Exit code 1 if errors found, 0 otherwise.

#### `loop repair`

Auto-repairs pipeline state. Actions:
1. Adds missing `Status: ready-for-agent` to todo files
2. Adds missing `Execution mode: AFK-only` to files
3. Strips empty `## UAT Results` placeholders
4. Reports (but does NOT auto-promote) stuck test-ready files with populated UAT Results
5. Promotes todo files with `## Test Results` to test-ready/
6. Flags invalid Execution mode values (no auto-fix)
7. Adds/updates SHA-256 checksums on all issue files

If GitHub configured: reopens prematurely closed issues, fixes missing labels.

#### `loop restore`

Restores git context after an interrupted `loop run`. Reads `.loop/git-context.json`, switches back to original branch, pops the auto-saved stash. If no saved context: prints message and exits 0.

#### `loop upgrade`

Self-upgrades the loop binary to the latest GitHub release from `sambaths/loop`. Downloads matching OS/arch archive, backs up current binary to `loop.bak`, installs new binary with `chmod 0755`, removes backup.

#### `loop checksum verify`

Verifies SHA-256 checksums on all issue files with a `Checksum:` header. Prints `ok:` or `FAIL:` per file. Exit code 1 on any mismatch.

#### `loop completion bash`

Prints a bash completion script to stdout.

#### `loop commands`

Prints a help table of all commands and flags.

#### `loop screenshot`

Saves a text-only terminal screenshot to `loop-screenshot-<timestamp>.txt`. ANSI codes stripped.

### Exit Codes

| Code | Meaning |
|---|---|
| 0 | Success |
| 1 | General error (pre-flight failure, config missing, checksum mismatch) |
| 2 | Unknown subcommand or invalid arguments |

## Issue State Machine

### States and Directory Paths

| State | Constant | Directory | Stage Rank |
|---|---|---|---|
| todo | `StateTodo` | `docs/issues/` (root) | 1 |
| ready-for-agent | `StateReadyForAgent` | `docs/issues/ready-for-agent/` | 2 |
| test-ready | `StateTestReady` | `docs/issues/test-ready/` | 3 |
| done | `StateDone` | `docs/issues/done/` | 4 |
| .quarantine | `StateQuarantine` | `docs/issues/.quarantine/` | 0 |
| unable | `StateUnable` | `docs/issues/unable/` | 0 |

State is derived from the parent directory name via `StateFromPath()`.

### Transitions

| From State | Role | Promise / Condition | To State |
|---|---|---|---|
| todo | implement | COMPLETE | test-ready |
| ready-for-agent | implement | COMPLETE | test-ready |
| any | any | NO_MORE_TASKS | .quarantine |
| any | any | Invalid promise/role combo | .quarantine |
| test-ready | test | TEST_PASS | done |
| test-ready | test | TEST_FAIL (retry < 5) | todo |
| test-ready | test | TEST_FAIL (retry >= 5) | unable |
| test-ready | test | Inactivity kill (retry < 5) | todo (i-- retry) |
| test-ready | test | Inactivity kill (retry >= 5) | unable |
| todo | agent | Has populated `## Test Results` (repair) | test-ready |
| test-ready | agent | Has populated `## UAT Results` (stuck, reported only) | (none — manual) |

### Execution Modes

| Mode | Constant | Autonomous Implement? | Selected By |
|---|---|---|---|
| AFK-only | `ExecModeAFKOnly` | Yes | Agent-driven implement (priority 2/3) |
| HITL-only | `ExecModeHITLOnly` | No | Fallback only → NO_MORE_TASKS (priority 4) |
| Combo | `ExecModeCombo` | No | Never selected (returns ErrNoIssues) |

### Issue Selection Priority (`SelectIssue`)

1. **test-ready files** — unblocked, no populated UAT Results → role = test
2. **ready-for-agent files** — unblocked, AFK-only → role = implement
3. **todo files** — unblocked, AFK-only → role = implement
4. **ready-for-agent / todo files** — unblocked, HITL-only → role = implement (fallback, produces NO_MORE_TASKS)

If nothing available, returns `ErrNoIssues`.

### Blocker Resolution

A blocker reference is resolved if:
- The referenced GitHub# exists in any `done/` file's `GitHub:` header, OR
- Any `done/` file's basename starts with the blocker number string (e.g. `42-my-issue.md` resolves blocker `42`)
- The blocked-by text is `- None` (case-insensitive) → no blockers
- A reference with no extractable number → treated as resolved (informational)

## Issue File Format

```markdown
# NN - Feature title

GitHub: #14
Status: ready-for-agent
Execution mode: AFK-only
Branch: feature-branch
Checksum: abc123...
Type: feat
Retry: 0

## What to build

Description.

## User stories covered

...

## Acceptance criteria

- [ ] Criterion one
- [ ] Criterion two

## UAT Plan

| Step | Description | Output | Expected | Result |

## Blocked by

- None

## Test Results

(added by implementing agent)

## UAT Results

(added by testing subagent)

## Comments

(optional)
```

### Headers

All headers go between the `# Title` line and the first `## Section` heading.

| Header | Format | Required? | Stored In |
|---|---|---|---|
| Title | `# <title>` (first line, H1) | **Yes** — file invalid without it | `IssueFile.Title` |
| GitHub | `GitHub: #<number>` | Recommended (warning if missing) | `IssueFile.GitHubNum` |
| Status | `Status: ready-for-agent` or `Status: ready-for-human` | **Yes** for todo files (auto-added if missing) | (not stored in struct) |
| Execution mode | `Execution mode: AFK-only` / `HITL-only` / `Combo` | **Yes** for active pipeline files (auto-added) | `IssueFile.ExecMode` |
| Branch | `Branch: <name>` | Recommended (warning if missing) | `IssueFile.Branch` |
| Checksum | `Checksum: <sha256-hex>` | Optional (based on config) | `IssueFile.Checksum` |
| Type | `Type: <value>` | Recommended (warning if missing) | `IssueFile.Type` |
| Retry | `Retry: <number>` | Optional | `IssueFile.Retries` |

### Sections

#### Required Sections (every issue MUST contain):
1. `## What to build`
2. `## User stories covered`
3. `## Acceptance criteria`
4. `## UAT plan` (must be a markdown table with exactly 5 columns: Step, Description, Output, Expected, Result)
5. `## Blocked by`

#### Known Sections (complete set):
- `## Parent` — parent issue reference
- `## What to build` — description of the feature
- `## User stories covered` — user stories from parent PRD
- `## Acceptance criteria` — verifiable checklist (`- [ ]` / `- [x]`)
- `## UAT plan` — test plan table
- `## UAT Results` — filled by testing subagent during validation
- `## UAT Process` — process notes
- `## Defect Tracker` — defect tracking
- `## Blocked by` — blocking issue references
- `## Comments` — optional notes

#### Disallowed Sections Per State:
| State | Disallowed Sections |
|---|---|
| todo | `UAT Results` |
| test-ready | `UAT Results` (only pre-populated; empty placeholders stripped) |
| ready-for-agent | `Test Results`, `UAT Results` |
| done | (none — both are valid) |
| .quarantine | (none) |
| unable | (none) |

### Section Lifecycle

1. IMPLEMENTING AGENT writes `## Test Results` before outputting COMPLETE
2. TESTING SUBAGENT writes `## UAT Results` before outputting TEST_PASS or TEST_FAIL
3. On TEST_FAIL, loop strips both `## Test Results` and `## UAT Results` before moving back to todo/
4. On NO_MORE_TASKS, the file moves to .quarantine with existing sections preserved

## Configuration

Config file: `.loop/config.json` (project root, found by walking up for `.git` or `go.mod`)

### Schema

| Field | Type | Default | Description |
|---|---|---|---|
| `repo` | string | `""` | GitHub `owner/name`; empty = local-only |
| `issue_dir` | string | `"docs/issues"` | Path to local issue files |
| `branch_origin` | string | `"main"` | Default branch for issue branches |
| `agent_timeout` | int | `300` | Agent timeout in seconds |
| `inactivity_warn` | int | `60` | Seconds of inactivity before warning |
| `inactivity_recover` | int | `120` | Seconds of inactivity before agent kill |
| `checksums_enabled` | bool | `true` | Enable SHA-256 checksums on issue files |
| `branch_from_origin` | bool | `false` | Create temp branches from `origin/<branch>` instead of local |

### Example

```json
{
  "repo": "my-org/my-repo",
  "issue_dir": "docs/issues",
  "branch_origin": "main",
  "agent_timeout": 300,
  "inactivity_warn": 60,
  "inactivity_recover": 120,
  "checksums_enabled": true,
  "branch_from_origin": false
}
```

The `--timeout` global flag overrides `agent_timeout` at runtime.

## Agent Protocol

### Invocation

```
opencode run --dangerously-skip-permissions
```

- stdin: `## Role: <role>\n\n<issue_file_content>\n\n<system_prompt>`
- Working directory: configured issue directory
- Stdout: captured fully into a `bytes.Buffer`
- Stderr: captured separately into a `bytes.Buffer`
- For implement role: `Test Results` and `UAT Results` sections are stripped from the input
- If `role` is `implement` and `execMode` is `HITL-only`: agent is NOT invoked; returns `NO_MORE_TASKS` immediately

### Context and Timeout

- If `timeout > 0`: `context.WithTimeout` is created; otherwise `context.WithCancel`
- When context is cancelled (timeout, SIGINT), the process group is killed: `SIGKILL` on Unix (via negative PID), `Process.Kill()` on Windows

### Sentinel Protocol

Promise markers are wrapped in sentinel lines:

```
__LOOP_RESULT__
<TOKEN>
__LOOP_RESULT_END__
```

Parsing algorithm:
1. Find the LAST `__LOOP_RESULT__` in stdout (bottom-up scan)
2. Find the FIRST `__LOOP_RESULT_END__` after that position
3. Extract and trim whitespace from the text between them
4. Require exact case-sensitive match against valid tokens

### Promise Tokens

| Token | Constant | Role | Pipeline Effect |
|---|---|---|---|
| `COMPLETE` | `agent.Complete` | Implementing | Issue → test-ready |
| `TEST_PASS` | `agent.TestPass` | Testing | Issue → done |
| `TEST_FAIL` | `agent.TestFail` | Testing | Issue → todo (or unable if retries >= 5) |
| `NO_MORE_TASKS` | `agent.NoMoreTasks` | Both | Issue → .quarantine, pipeline halts |

### Commit Message Sentinels

```
__LOOP_COMMIT__
<type>: <short summary>

<detailed description of what changed and why>
__LOOP_COMMIT_END__
```

Type must be one of: `feat`, `fix`, `bug`, `enhancement`, `chore`, `test`, `docs`.

Parsing: same algorithm as promise markers (last start, first end). If missing or contains placeholder text (`<type>`, `<short summary>`, `login form`), a fallback message is generated: `{type}: {title}` where type is inferred from the issue's `Type:` header or title prefix.

### Recovery Prompt

If no valid promise marker is found in agent output:
1. Print warning with last 200 bytes of stdout
2. Re-invoke `opencode run --dangerously-skip-permissions` with only the recovery prompt (30-second timeout)
3. Recovery prompt: *"Your previous output was missing a promise marker. What was the outcome? Output exactly one of: COMPLETE, TEST_PASS, TEST_FAIL, NO_MORE_TASKS wrapped in __LOOP_RESULT__ / __LOOP_RESULT_END__"*
4. If recovery succeeds, use the recovered promise; if it fails, default to TEST_FAIL

### Inactivity Watchdog

When `inactivity_warn > 0` or `inactivity_recover > 0`:
- A goroutine monitors `lastOutputTime` at a check interval of `min(warn, recover) / 2` (floor 1s)
- If elapsed >= `inactivity_warn`: prints warning *"agent appears stalled"* (fires once)
- If elapsed >= `inactivity_recover`: kills the process group with SIGKILL
- On kill: attempts promise recovery (30s timeout)
- If recovery fails: outcome = FAIL, error = `ErrInactivityKill`
- On `ErrInactivityKill`: retries the issue (increments retry counter; if >= 5, moves to `unable/`)

### Stdout Buffering

- Full stdout is always preserved in the result (unlimited buffer)
- For promise parsing only: if stdout exceeds 1 MB (`maxOutputSize = 1,048,576` bytes), only the last N bytes are scanned (promise markers are at the end of agent output)
- `result.Truncated` indicates truncation occurred

## Pipeline Orchestration

### Iteration Flow (per iteration)

1. **Scan**: `ScanIssueDir(cfg.IssueDir)` — reads all state directories, builds `PipelineState` with cached `IssueFile` map
2. **Quarantine**: `QuarantineAll(ps)` — deduplicates files by basename, GitHub#, and title across states (keeps canonical: highest stage rank, then newest mtime)
3. **Pre-flight**: `PreFlightCheck(ps, repair, checksumsEnabled)` — errors block pipeline, warnings are reported
4. **GitHub check** (if repo configured): reopens closed issues, ensures labels
5. **Select**: `SelectIssue(ps)` or `FindIssueByNum(ps, forceIssueNum)` for forced issues
6. **Git save**: `StashChanges()`, get `CurrentBranch()`, persist context to `.loop/git-context.json`
7. **Temp branch**: `CreateTempBranch("loop/<slug>", targetBranch, branchFromOrigin)`
8. **Agent run**: `RunIterationContext(ctx, cfg, issueFile, role)` — invokes opencode, parses output, resolves promise
9. **Commit** (implement + COMPLETE): extract commit message from sentinel, run `git commit`
10. **Merge** (test + TEST_PASS): `SwitchBranch(target)`, `MergeBranch(tempBranch)`, `DeleteBranch(tempBranch)`
11. **Retry handling** (inactivity kill): increment retries; if >= `MaxRetries` (5), move to `unable/`
12. **Post-agent scan**: re-scan for duplicates (agent may create sub-issues)
13. **Transition**: `ComputeTransition(file, promise, role)` → `Move(issuesDir, issue, targetState)`
14. **GitHub sync** (if configured): `SyncLabelsForStates(repo, githubNum, fromState, toState)`
15. **Git restore**: switch back to original branch, pop stash, clear context file

### Pre-Flight Checks

Errors (block pipeline):
- Duplicate filenames across state directories
- Duplicate GitHub issue numbers
- Duplicate titles across states
- Self-blocking issue (blocker references own GitHub#)
- Blocked by non-existent GitHub issue

Warnings (reported, does not block):
- Missing `GitHub:` header
- Missing `Checksum:` header (when enabled)
- Missing `Type:` header
- Missing `Branch:` header
- Title pattern mismatch (`"N - description"` expected)
- Missing/invalid `Execution mode` header
- Missing required sections
- Disallowed sections present
- Populated `UAT Results` in test-ready (stuck files)
- Checksum mismatch (when enabled)

### Repair Pipeline (6 actions)

1. `AddMissingTodoLabels(root)` — adds `Status: ready-for-agent` to todo files missing a valid status
2. `AddMissingExecMode(root)` — adds `Execution mode: AFK-only` to files missing it
3. `StripEmptyUATPlaceholders(root)` — removes empty `## UAT Results` from test-ready and ready-for-agent
4. `FindStuckTestReadyFiles(root)` — reports (does NOT auto-promote) test-ready files with populated UAT Results
5. `PromoteTodoWithTestResults(root)` — moves todo files with `## Test Results` to test-ready/ (implemented but interrupted)
6. `FindInvalidExecModes(root)` — reports files with invalid Execution mode values (no auto-fix)
7. `AddMissingChecksums(root, checksumsEnabled)` — adds/updates SHA-256 checksum headers

### Retry System

- `MaxRetries = 5` (constant in `internal/issue/`)
- Stored in file header: `Retry: N`
- Two retry paths:
  - **Inactivity kill**: retried with `i--` (same iteration slot); at >= 5, moved to `unable/`
  - **TEST_FAIL**: sections stripped, moved to todo/ with incremented counter; at >= 5, moved to `unable/` instead of todo/

## Git Safety Layer

All operations in `internal/git/git.go`.

### Stash Flow

1. `StashChanges()` — checks `git status --porcelain`; if clean, no-op
2. If unmerged index exists: tries `git add -u`, then `git merge --abort` as fallback
3. Primary: `git stash push --include-untracked -m "loop-auto-save <timestamp>"`
4. Fallback 1: `git stash create` + `git stash store` (plumbing, bypasses index validation)
5. Fallback 2: writes binary diff to `.git/loop-autosave.patch`
6. `PopStash()` — `StashApply()` then `StashDrop()`; on conflict returns `ErrStashConflict` (stash preserved)

### Branch Management

- Temp branch naming: `loop/<slug>` (where slug is the issue filename without `.md`)
- `CreateTempBranch(name, base, fromOrigin)` — tries `origin/<base>` first if `fromOrigin=true`, then local `<base>`
- Merge strategy: `--ff-only` first → fallback to `--no-edit` merge commit → on failure, `--abort` and error
- `SwitchForIssue(branchField, defaultBranch)` — resolves branch, creates/switches if needed
- Branch field resolution: `""` → default, `"*"` → stay on current, any other value → use that branch

### Context Persistence

- File: `.loop/git-context.json`
- Format: `{"OriginalBranch": "main", "Stashed": true}`
- `SaveContext()` → returns `restore()` cleanup function that restores branch and pops stash
- `RestoreContextFromFile()` — called by `loop restore` command; reads context, restores, clears file
- `HasSavedContext()` — checks if context file exists

### Commit Operations

- `CommitAll(msg)` — `git add -A && git commit -m <msg>`; "nothing to commit" is not an error
- `CommitRaw(msg)` — same but single `-m` (accepts multi-line)
- Only performed when role=implement AND promise=COMPLETE

## GitHub Integration

All operations in `internal/github/github.go` and `internal/gh/gh.go`.

### Auth Caching

```go
var authOnce sync.Once
func CheckAuthOnce() bool
```
- Calls `gh auth status` exactly once per process lifetime
- Caches result; on failure, clears `cfg.Repo` for the session (falls back to local-only)

### Label Mapping

| Pipeline State | GitHub Label |
|---|---|
| todo | ready-for-agent |
| ready-for-agent | ready-for-agent |
| test-ready | test-ready |
| done | done |
| .quarantine | quarantine |

### Label Sync (by transition)

| Transition | Actions |
|---|---|
| todo → test-ready | Remove `ready-for-agent`/`ready-for-human`, add `test-ready` |
| test-ready → done | Remove `test-ready`, close issue |
| test-ready → todo | Remove `test-ready`, add `ready-for-agent` |
| test-ready → ready-for-agent | Remove `test-ready`, add `ready-for-agent` |
| ready-for-agent → test-ready | Remove `ready-for-agent`, add `test-ready` |
| ready-for-agent → done | Close issue (no label change) |
| todo → done | Close issue (no label change) |

### Operations

- `SyncLabelsForStates(repo, num, fromState, toState)` — updates GitHub labels based on transition
- `ReopenIfClosed(repo, num)` — reopens prematurely closed issues with auto-comment
- `RepairGitHubState(repo, ps)` — reopens all closed pending issues
- `FixMissingLabels(repo, ps)` — ensures correct labels on all issues
- `EnsureTestReadyLabels(repo, ps)` — ensures test-ready labels on test-ready issues

### Upgrade (Release Download)

- Uses `gh release download` or GitHub API (no auth required for public repos)
- Archive pattern: `<project>_<version>_<os>_<arch>.tar.gz` (or `.zip` for Windows)
- `DownloadLatestAsset(owner, name, destDir)` — fetches latest release via `api.github.com/repos/<owner>/<name>/releases/latest`
- `LatestTag(repo)` — gets the latest release tag name

## Checksum System

### How It Works

- SHA-256 hash of the file content with the `Checksum:` line excluded
- `SetChecksum(path)` — computes and writes/updates the `Checksum: <hex>` header
- `VerifyChecksum(path)` — compares stored hash against computed hash
- `VerifyChecksums(root)` — verifies all files across all states
- `AddMissingChecksums(root, enabled)` — adds/updates checksums, only if `enabled=true`

### Exclude-from-Hash Rule

Lines starting with `Checksum:` (whitespace-trimmed) are excluded from the hash computation. This allows checksum updates without changing the hash.

## TUI Components

All in `internal/tui/`. Built with [Bubbletea](https://github.com/charmbracelet/bubbletea).

### Dashboard (`internal/tui/dashboard/`)

5 tabbed pages:
1. Pipeline overview — counts by state, pipeline bar, recent transitions, warnings
2. Todo issues list
3. Test-ready issues list
4. Done issues list
5. Quarantined issues list

Keyboard: `q`/`ctrl+c` quit, `tab`/`right`/`l`/`n` next, `shift+tab`/`left`/`h`/`p` prev, `s` screenshot, arrows/pgup/pgdn scroll, `home`/`g` top, `end`/`G` bottom.

### Run Viewer (`internal/tui/run/`)

Streaming output viewer with:
- Real-time agent stdout/stderr display
- Auto-scroll
- Elapsed timer
- Iteration progress panel

### Status Viewer (`internal/tui/status/`)

Read-only dashboard view (same 5 pages, titled "loop status").

### Setup Wizard (`internal/tui/setup/`)

3-step interactive wizard: issue directory → GitHub repo → default branch.

### Screenshot Saver (`internal/tui/screenshot/`)

Saves text-only pipeline state to file, stripping ANSI codes.

## Architecture

### Internal Package Map

| Package | Path | Responsibility |
|---|---|---|
| `main` | `cmd/loop/` | CLI entry point, arg parsing, command dispatch |
| `config` | `internal/config/` | Config load/save, project root detection, `.gitignore` management |
| `agent` | `internal/agent/` | `opencode` subprocess management, promise marker parsing, inactivity watchdog, promise recovery |
| `runner` | `internal/runner/` | High-level loop orchestration: `RunLoop`, `RunLoopContext`, `RunLoopStreamed`, `RunIteration` |
| `pipeline` | `internal/pipeline/` | Legacy orchestration (older Runner/Pipeline struct, superseded by runner) |
| `issue` | `internal/issue/` | Issue file state machine: read, write, parse, transition, quarantine, pre-flight checks, repair, checksums |
| `git` | `internal/git/` | Git operations: stash, branch management, commit, merge, context save/restore |
| `github` | `internal/github/` | Higher-level GitHub operations: label sync, issue reopen, state repair, release download |
| `gh` | `internal/gh/` | Low-level `gh` CLI wrappers (repo-unaware, simpler than `internal/github/`) |
| `prompt` | `internal/prompt/` | Embedded agent system prompt for `opencode` |
| `tui/dashboard` | `internal/tui/dashboard/` | Main TUI dashboard (Bubbletea) |
| `tui/setup` | `internal/tui/setup/` | Interactive setup wizard |
| `tui/status` | `internal/tui/status/` | Pipeline status TUI |
| `tui/run` | `internal/tui/run/` | Run iteration TUI with streaming logs and progress |
| `tui/screenshot` | `internal/tui/screenshot/` | ANSI-stripped text screenshot saver |

## Development Commands

```bash
go test ./...          # run all tests
go vet ./...           # lint
make build             # build binary (requires Go 1.22+)
make install           # build + copy to $HOME/.local/bin/loop
make build-all         # cross-compile: linux/darwin/windows × amd64/arm64
make test              # go test ./...
make lint              # go vet ./...
make clean             # remove build artifacts
make completions       # generate bash completion script
make release TAG=vX.Y.Z  # create annotated tag + push (triggers GoReleaser)
```

## Release Process

- Built and published via [GoReleaser](https://goreleaser.com)
- `make release TAG=vX.Y.Z` creates an annotated git tag and pushes it
- GitHub Actions cross-compiles and publishes:
  - Linux (amd64, arm64) — `.tar.gz`
  - macOS (amd64, arm64) — `.tar.gz`
  - Windows (amd64) — `.zip`
  - `checksums.txt`
  - Auto-generated changelog
- Install script: `curl -sL https://raw.githubusercontent.com/sambaths/loop/main/install.sh | bash`

## References

- [README.md](README.md) — user-facing documentation
- [CODING_STANDARDS.md](CODING_STANDARDS.md) — coding conventions
- [CONTEXT.md](CONTEXT.md) — compact glossary
- [docs/llms.txt](docs/llms.txt) — web-accessible LLM reference (llmstxt.org format)
