# loop
[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go)](go.mod)
[![Build Status](https://img.shields.io/github/actions/workflow/status/sambaths/loop/ci.yml?branch=main)](https://github.com/sambaths/loop/actions)
[![License](https://img.shields.io/badge/license-MIT-blue)](LICENSE)

A simple looping mechanism — drives an implement-then-test cycle over issues using `opencode` as the agent CLI.

## Prerequisites

- **Go 1.22+** â for building from source
- **opencode** â the agent CLI loop uses. Install from [opencode.ai](https://opencode.ai)

## What is loop

Loop is a Go TUI tool that automates the full issue lifecycle. You write issues as local markdown files, and loop picks them up one at a time, invokes `opencode run` to implement them, then tests them in a separate iteration. Issues move through filesystem directories as their state changes — the filesystem **is** the state machine.

```
issues/  ──[IMPLEMENT]──▶  test-ready/  ──[TEST_PASS]──▶  done/
  ▲                              │
  └─────────[TEST_FAIL]──────────┘
```

Loop operates in two modes:

- **Local-only** — no GitHub integration; issues live entirely in markdown files
- **GitHub-synced** — file transitions mirror to GitHub labels and issue state



## Installation



### From release

```bash
curl -sL https://raw.githubusercontent.com/sambaths/loop/main/install.sh | bash
```

To install a specific version:

```bash
curl -sL https://raw.githubusercontent.com/sambaths/loop/main/install.sh | bash -s -- --version v0.1.0
```



### From source

```bash
go install github.com/sambaths/loop/cmd/loop@latest
```



### Build locally

```bash
make build
./loop --version
```



### Bash completion

```bash
make completions
source loop_completion.bash
```



## Quickstart

```bash
# Run the setup wizard (interactive)
loop setup

# Run 10 AFK iterations
loop run 10

# Force a specific issue on iteration 1
loop run 5 42

# Check pipeline state
loop status

# Open the TUI dashboard
loop
```

The setup wizard walks you through:

1. Issue directory path (default: `docs/issues`)
2. GitHub repository (optional — leave blank for local-only mode)
3. Default branch origin (default: `main`)

After setup, place markdown issue files in `docs/issues/` and run `loop run <n>`.

## Workflow: Matt Pocock skills + loop

This project follows the [Matt Pocock engineering skills](https://github.com/mattpocock) workflow, with some changes to make it work for me. The two-phase approach:

### Phase 1: Create issues (Matt Pocock skills)

Use the agent skills in `.agents/skills/` to break work down:

```
/setup-matt-pocock-skills    → configure repo (issue tracker, labels, domain docs)
/to-prd                      → create a PRD from conversation
/to-issues                   → break PRD into vertical-slice issues
/triage                      → triage and label issues
```

These produce issue files in `docs/issues/` with the right structure (`## What to build`, `## Acceptance criteria`, `## UAT Plan`, etc.).

### Phase 2: Execute (loop)

Once issues are ready, loop processes them autonomously:

```
loop run 10                  → process up to 10 issues, alternating implement/test
```

Loop handles the rest: branch switching, agent invocation, file transitions, GitHub sync.

### Key differences from Matt Pocock's original pattern

- The bash scripts are replaced by a Go TUI binary
- Promise markers use sentinel-wrapped tokens (`__LOOP_RESULT__` / `__LOOP_RESULT_END__`)
- Implement-then-test lifecycle with close-after-test rule
- Pre-flight checks and auto-repair for pipeline state
- Agent prompt is embedded in the binary (no sync drift)
- Distributable via GoReleaser and install script
- Local-only mode — works entirely without GitHub



## Commands Reference


| Command                       | Description                                                                                       |
| ----------------------------- | ------------------------------------------------------------------------------------------------- |
| `loop`                        | Start the TUI dashboard                                                                           |
| `loop setup`                  | Interactive setup wizard                                                                          |
| `loop run <n> [issue-number]` | Run N AFK iterations (optionally force a specific issue)                                          |
| `loop status`                 | Show pipeline state (todo, test-ready, done, quarantine)                                          |
| `loop check`                  | Validate pipeline state — detect duplicates, missing fields, checksum mismatches                  |
| `loop repair`                 | Repair pipeline: add missing labels, strip empty sections, promote stuck files, fix GitHub labels |
| `loop restore`                | Restore git context (original branch + pop stash) if loop was interrupted                         |
| `loop download`               | Download the latest release from GitHub                                                           |
| `loop checksum verify`        | Verify file content checksums against the `Checksum:` header in issue files                       |
| `loop completion`             | Print bash completion script                                                                      |
| `loop --version`              | Show version (e.g. `loop v0.1.0 linux/amd64`)                                                     |
| `loop --help`                 | Show help                                                                                         |




### Flags


| Flag               | Description                                                |
| ------------------ | ---------------------------------------------------------- |
| `--timeout <secs>` | Agent timeout in seconds (overrides config default of 300) |
| `--repair`         | Repair GitHub state (reopen prematurely closed issues)     |




## Configuration

Run `loop setup` to create `.loop/config.json` in the project root:

```json
{
  "repo": "my-org/my-repo",
  "issue_dir": "docs/issues",
  "branch_origin": "main",
  "agent_timeout": 300,
  "checksums_enabled": true
}
```


| Field               | Description                                                  |
| ------------------- | ------------------------------------------------------------ |
| `repo`              | GitHub `owner/name` (empty = local-only mode)                |
| `issue_dir`         | Path to local issue files (default: `docs/issues`)           |
| `branch_origin`     | Default branch for creating issue branches (default: `main`) |
| `agent_timeout`     | Agent timeout in seconds (default: 300)                      |
| `checksums_enabled` | Enable SHA-256 checksums on issue files (default: true)      |


Loop discovers the project root by walking up the directory tree looking for `.git` or `go.mod`, then reads `.loop/config.json`. The `--timeout` flag overrides `agent_timeout` at runtime.

### Issue file format

Issue files are markdown with a structured header and sections:

```markdown
# 01 - Feature title

GitHub: #14
Status: ready-for-agent
Execution mode: AFK-only
Branch: feature-branch
Checksum: abc123...

## What to build

Description of the feature.

## Acceptance criteria

- [ ] Criterion one
- [ ] Criterion two

## UAT Plan

| Step | Description | Output | Expected | Result |
```



### Pipeline state directories


| Directory                  | Contents                                     |
| -------------------------- | -------------------------------------------- |
| `docs/issues/`             | Pending implementation                       |
| `docs/issues/test-ready/`  | Implemented, awaiting UAT                    |
| `docs/issues/done/`        | Tested and passed                            |
| `docs/issues/.quarantine/` | Duplicates or agent failures held for review |




### Promise markers

The agent signs results via stdout with sentinel-wrapped tokens:

```
__LOOP_RESULT__
COMPLETE
__LOOP_RESULT_END__
```


| Token           | Role      | Transition        |
| --------------- | --------- | ----------------- |
| `COMPLETE`      | Implement | todo → test-ready |
| `TEST_PASS`     | Test      | test-ready → done |
| `TEST_FAIL`     | Test      | test-ready → todo |
| `NO_MORE_TASKS` | Both      | Pipeline halts    |




## Development



### Prerequisites

- Go 1.22+



### Commands

```bash
go test ./...        # Run all tests
go vet ./...         # Run lint
make build           # Build binary
make install         # Build + copy to $HOME/.local/bin/loop
make build-all       # Cross-compile for linux/windows/darwin × amd64/arm64
make clean           # Remove build artifacts
```



### Project structure

```
cmd/loop/            # CLI entry point
internal/
  agent/             # opencode subprocess management
  config/            # Configuration system
  gh/                # Raw gh CLI wrappers
  git/               # Git safety (stash, branch, context)
  github/            # Higher-level GitHub operations
  issue/             # Issue file state machine
  pipeline/          # Legacy orchestration
  prompt/            # Embedded agent prompt
  runner/            # Current orchestration layer
  tui/               # Bubbletea TUI components
    dashboard/       # Main TUI dashboard
    output/          # Output viewer
    run/             # Run command TUI
    setup/           # Setup wizard
    status/          # Status command TUI
```



### Architecture

- **Filesystem as state machine** — issue file location determines pipeline stage
- **Sentinel-wrapped promises** — prevents false positives when parsing agent output
- **Git-first safety** — stash/restore around every agent run
- **Optional GitHub integration** — works fully offline, syncs to GitHub when configured
- **Embedded prompt** — agent system prompt baked into the binary, preventing sync drift
- **Graceful degradation** — if `gh` is not authenticated, loop silently falls back to local-only



### Releases

Tags are built and published via GoReleaser. To create a release:

```bash
make release TAG=v0.1.0
```

This tags the commit and pushes the tag. GitHub Actions builds cross-platform binaries and publishes a draft release with:

- Linux (amd64, arm64) — `.tar.gz`
- macOS (amd64, arm64) — `.tar.gz`
- Windows (amd64) — `.zip`
- `checksums.txt`
- Auto-generated changelog



## License

MIT