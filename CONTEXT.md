# loop

A Go TUI that runs an AFK implement-then-test cycle over issues using `opencode` as the agent CLI. Follows the [Matt Pocock engineering skills](https://github.com/mattpocock) workflow pattern.

## Glossary

- **loop**: The Go TUI binary that orchestrates the AFK issue pipeline.
- **Issue**: A unit of work as a local markdown file under `docs/issues/`, optionally mirrored to GitHub.
- **Issue directory**: The directory (`docs/issues/` by default) with subdirectories for pipeline states.
- **Promise marker**: Machine-readable token in agent stdout: `COMPLETE`, `TEST_PASS`, `TEST_FAIL`, or `NO_MORE_TASKS`. Wrapped in `__LOOP_RESULT__` / `__LOOP_RESULT_END__`.
- **Agent**: The external CLI (`opencode run --dangerously-skip-permissions`) that loop invokes.
- **State**: Pipeline stage determined by parent directory: todo, ready-for-agent, test-ready, done, .quarantine, unable.
- **Role**: Either `implement` (building code) or `test` (validating), determined by pipeline state.
- **Execution mode**: Controls autonomy: `AFK-only` (autonomous), `HITL-only` (human), `Combo` (never selected).
- **Retry**: Counter (max 5) incremented on TEST_FAIL or inactivity kill; at 5, issue moves to `unable/`.
- **Checksum**: SHA-256 hash of issue content (excluding the `Checksum:` line).

For the full exhaustive reference, see [AGENTS.md](AGENTS.md). A web-accessible version is at [docs/llms.txt](docs/llms.txt).
