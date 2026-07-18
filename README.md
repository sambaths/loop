# loop

[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go)](go.mod)
[![Build Status](https://img.shields.io/github/actions/workflow/status/sambaths/loop/ci.yml?branch=main)](https://github.com/sambaths/loop/actions)
[![License](https://img.shields.io/badge/license-MIT-blue)](LICENSE)

A Go TUI that runs an AFK implement-then-test loop over issues, using `opencode` as the agent CLI.

This is my personal take on running AFK agent loops using the **skills shared by [Matt Pocock](https://github.com/mattpocock)** for breaking work into issues — adapted into a standalone Go binary instead of bash scripts.

> **Note:** This entire repo was agent-generated, with bug fixes done via more agent loops on top. I have practically no Go experience myself, so there may be rough edges I've overlooked. Issues and PRs welcome.

## Prerequisites

- **Go 1.22+** — to build from source
- **opencode** — the agent CLI loop invokes. Install from [opencode.ai](https://opencode.ai)

## How it works

You write issues as local markdown files. Loop picks one up, runs `opencode run` to implement it, then tests it in a separate pass. Issues move through directories as they change state — the filesystem **is** the state machine:

```
issues/  ──[IMPLEMENT]──▶  test-ready/  ──[TEST_PASS]──▶  done/
  ▲                              │
  └─────────[TEST_FAIL]──────────┘
```

> **For AI agents:** [`AGENTS.md`](AGENTS.md) is the exhaustive LLM reference read by `opencode` at session start. A web-accessible version is at [`docs/llms.txt`](docs/llms.txt). See also [`CONTEXT.md`](CONTEXT.md) for a compact glossary.

Two modes:

- **Local-only** — no GitHub, issues live entirely as markdown files
- **GitHub-synced** — file transitions mirror to GitHub labels/issue state

## Install

**From release:**
```bash
curl -sL https://raw.githubusercontent.com/sambaths/loop/main/install.sh | bash
# pin a version:
curl -sL https://raw.githubusercontent.com/sambaths/loop/main/install.sh | bash -s -- --version v0.1.0
```

**From source:**
```bash
go install github.com/sambaths/loop/cmd/loop@latest
```

**Build locally:**
```bash
make build
./loop --version
```

**Bash completion:**
```bash
make completions
source loop_completion.bash
```

## Quickstart

```bash
loop setup          # interactive setup wizard
loop run 10         # run 10 AFK iterations
loop run 5 42       # force a specific issue (#42) on iteration 1
loop status         # check pipeline state
loop                # open the TUI dashboard
```

Setup asks for: issue directory (default `docs/issues`), GitHub repo (optional, blank = local-only), and default branch (default `main`).

## Workflow: Matt Pocock skills + loop

Two phases:

**1. Create issues** (Matt Pocock skills, in `.agents/skills/`)
```
/setup-matt-pocock-skills    → configure repo (tracker, labels, domain docs)
/to-prd                      → turn conversation into a PRD
/to-issues                   → break PRD into vertical-slice issues
/triage                      → triage and label issues
```
These produce issue files in `docs/issues/` with the expected structure (`## What to build`, `## Acceptance criteria`, `## UAT Plan`, etc.).

**2. Run the loop**
```
loop run 10
```
Loop handles branch switching, agent invocation, file transitions, and GitHub sync automatically.

### What's different from Matt Pocock's original

- Bash scripts replaced with a single Go TUI binary
- Sentinel-wrapped promise markers (`__LOOP_RESULT__` / `__LOOP_RESULT_END__`) instead of loose text parsing
- Strict close-after-test rule enforced in the state machine
- Pre-flight checks + auto-repair for pipeline state
- Agent prompt embedded in the binary — no sync drift
- Distributed via GoReleaser + install script
- Fully local-only mode, no GitHub required

## Commands

| Command                       | Description                                                          |
| ------------------------------ | --------------------------------------------------------------------- |
| `loop`                         | Start the TUI dashboard                                              |
| `loop setup`                   | Interactive setup wizard                                              |
| `loop run <n> [issue-number]`  | Run N AFK iterations (optionally force a specific issue)             |
| `loop status`                  | Show pipeline state (todo, test-ready, done, quarantine)              |
| `loop check`                   | Validate pipeline: duplicates, missing fields, checksum mismatches    |
| `loop repair`                  | Repair pipeline: missing labels, stuck files, GitHub label drift      |
| `loop restore`                 | Restore git context (branch + stash) after an interrupted run         |
| `loop upgrade`                | Upgrade loop to the latest release                                   |
| `loop checksum verify`         | Verify issue file checksums against the `Checksum:` header            |
| `loop completion`              | Print bash completion script                                          |
| `loop --version` / `--help`    | Show version / help                                                   |

**Flags:** `--timeout <secs>` overrides the configured agent timeout; `--repair` reopens prematurely closed GitHub issues.

## Configuration

`loop setup` writes `.loop/config.json`:

```json
{
  "repo": "my-org/my-repo",
  "issue_dir": "docs/issues",
  "branch_origin": "main",
  "agent_timeout": 300,
  "checksums_enabled": true
}
```

| Field               | Description                                             |
| -------------------- | --------------------------------------------------------- |
| `repo`               | GitHub `owner/name` (empty = local-only mode)            |
| `issue_dir`          | Path to local issue files (default `docs/issues`)        |
| `branch_origin`      | Default branch for issue branches (default `main`)       |
| `agent_timeout`      | Agent timeout in seconds (default `300`)                  |
| `checksums_enabled`  | Enable SHA-256 checksums on issue files (default `true`)  |

Loop finds the project root by walking up for `.git` or `go.mod`, then reads `.loop/config.json`. `--timeout` overrides `agent_timeout` at runtime.

### Issue file format

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

### Pipeline directories

| Directory                  | Contents                                       |
| ---------------------------- | ------------------------------------------------- |
| `docs/issues/`              | Pending implementation                         |
| `docs/issues/test-ready/`   | Implemented, awaiting UAT                      |
| `docs/issues/done/`         | Tested and passed                              |
| `docs/issues/.quarantine/`  | Duplicates or agent failures held for review   |

### Promise markers

The agent signals results via stdout with sentinel-wrapped tokens:

```
__LOOP_RESULT__
COMPLETE
__LOOP_RESULT_END__
```

| Token             | Role       | Transition            |
| -------------------- | ------------ | ------------------------ |
| `COMPLETE`          | Implement  | `todo → test-ready`     |
| `TEST_PASS`         | Test       | `test-ready → done`     |
| `TEST_FAIL`         | Test       | `test-ready → todo`     |
| `NO_MORE_TASKS`     | Both       | Pipeline halts          |

## Development

```bash
go test ./...        # run all tests
go vet ./...          # lint
make build            # build binary
make install          # build + copy to $HOME/.local/bin/loop
make build-all        # cross-compile linux/windows/darwin × amd64/arm64
make clean             # remove build artifacts
```

### Project structure

```
cmd/loop/            # CLI entry point
internal/
  agent/             # opencode subprocess management
  config/            # configuration system
  gh/                # raw gh CLI wrappers
  git/               # git safety (stash, branch, context)
  github/            # higher-level GitHub operations
  issue/             # issue file state machine
  pipeline/          # legacy orchestration
  prompt/            # embedded agent prompt
  runner/            # current orchestration layer
  tui/               # Bubbletea TUI components
    dashboard/       # main TUI dashboard
    output/          # output viewer
    run/             # run command TUI
    setup/           # setup wizard
    status/          # status command TUI
```

### Architecture

- **Filesystem as state machine** — issue file location determines pipeline stage
- **Sentinel-wrapped promises** — prevents false positives when parsing agent output
- **Git-first safety** — stash/restore around every agent run
- **Optional GitHub integration** — works fully offline, syncs when configured
- **Embedded prompt** — agent system prompt baked into the binary, no sync drift
- **Graceful degradation** — falls back to local-only if `gh` isn't authenticated

### Releases

Built and published via GoReleaser:

```bash
make release TAG=v0.1.0
```

Tags the commit, pushes the tag. GitHub Actions cross-compiles and publishes a draft release with:

- Linux (amd64, arm64) — `.tar.gz`
- macOS (amd64, arm64) — `.tar.gz`
- Windows (amd64) — `.zip`
- `checksums.txt`
- Auto-generated changelog

## License

MIT