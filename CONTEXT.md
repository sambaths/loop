# loop

A simple looping mechanism — drives an implement-then-test cycle over issues using `opencode` as the agent CLI.

Follows the [Matt Pocock engineering skills](https://github.com/mattpocock) workflow pattern, with changes to make it work for me. The key changes are documented in the README.

## Glossary

**loop**:
The Go TUI that orchestrates the AFK issue pipeline.

**Issue**:
A unit of work as a local markdown file under `docs/issues/`, optionally mirrored to GitHub.

**Issue directory**:
The directory (`docs/issues/` by default) with subdirectories `test-ready/`, `done/`, and `.quarantine/` representing pipeline state.

**Promise marker**:
A machine-readable token in agent stdout: `COMPLETE`, `TEST_PASS`, `TEST_FAIL`, or `NO_MORE_TASKS`. Wrapped in `__LOOP_RESULT__` / `__LOOP_RESULT_END__` sentinel lines.

**Agent**:
The external CLI (`opencode run`) that loop invokes.

**GitHub repo**:
Optional remote. If configured, loop mirrors file transitions to GitHub. If not, loop runs local-only.

**Setup wizard**:
The TUI (`loop setup`) that collects config on first run.
