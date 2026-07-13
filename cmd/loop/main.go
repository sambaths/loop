package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sambaths/loop/internal/config"
	"github.com/sambaths/loop/internal/git"
	"github.com/sambaths/loop/internal/github"
	"github.com/sambaths/loop/internal/issue"
	"github.com/sambaths/loop/internal/runner"
	"github.com/sambaths/loop/internal/tui/dashboard"
	"github.com/sambaths/loop/internal/tui/run"
	"github.com/sambaths/loop/internal/tui/screenshot"
	"github.com/sambaths/loop/internal/tui/setup"
	"github.com/sambaths/loop/internal/tui/status"
)

var Version = "dev"
var GOOS = runtime.GOOS
var GOARCH = runtime.GOARCH

var osExit = os.Exit
var runCompletionFn = printBashCompletion
var runSetupFn = runSetup
var runTUIFn = runTUI
var runStatusFn = runStatus
var runCheckFn = runCheck
var runRestoreFn = runRestore
var runRepairFn = runRepair
var runDownloadFn = runDownload
var runChecksumVerifyFn = runChecksumVerify
var runCommandsFn = runCommands
var runScreenshotFn = runScreenshot

const usage = `loop - autonomous issue orchestrator

Usage:
  loop [command] [flags]

Commands:
  setup              Interactive setup wizard
  run <n> [issue-number]     Run N AFK iterations (optionally force a specific issue)
  status             Show pipeline state
  check              Validate pipeline state for issues
  repair             Repair pipeline: strip empty UAT Results, promote stuck files, fix GitHub labels
  restore            Restore git context (original branch + pop stash) if loop was interrupted
  screenshot         Save a terminal screenshot of the pipeline state
  download           Download the latest release from GitHub
  checksum verify    Verify file content checksums
  completion bash    Print bash completion script
  commands           Show a table of all available commands

Flags:
  -h, --help         Show this help message
  --version          Show version information
  --timeout <secs>   Agent timeout in seconds (overrides config)
  --repair           Repair GitHub state (reopen prematurely closed issues)
  --headless         Run without TUI (headless mode, for scripting/CI)
`

type command int

const (
	cmdTUI command = iota
	cmdSetup
	cmdHelp
	cmdVersion
	cmdRun
	cmdStatus
	cmdCheck
	cmdRepair
	cmdRestore
	cmdDownload
	cmdCompletion
	cmdChecksum
	cmdCommands
	cmdScreenshot
	cmdUnknown
)

var runN int        // iterations for cmdRun
var runIssueNum int // issue number to force for cmdRun (0 = normal selection)
var cliTimeout int  // agent timeout in seconds, 0 means use config default
var cliRepair bool  // repair GitHub state on each iteration
var cliHeadless bool // run without TUI (headless mode)

func parseArgs(args []string) (command, int) {
	// Reorder args so flags come before positional args.
	// Go's flag package stops parsing at the first non-flag arg,
	// so `loop run 1 --headless` would fail without this reorder.
	reordered := reorderFlags(args)

	fs := flag.NewFlagSet("loop", flag.ContinueOnError)
	helpFlag := fs.Bool("help", false, "Show this help message")
	hFlag := fs.Bool("h", false, "Show this help message (shorthand)")
	verFlag := fs.Bool("version", false, "Show version information")
	timeoutFlag := fs.Int("timeout", 0, "Agent timeout in seconds (overrides config)")
	repairFlag := fs.Bool("repair", true, "Repair GitHub state (reopen prematurely closed issues)")
	headlessFlag := fs.Bool("headless", false, "Run without TUI (headless mode)")
	fs.Usage = func() {}

	if err := fs.Parse(reordered); err != nil {
		return cmdUnknown, 2
	}

	if *helpFlag || *hFlag {
		return cmdHelp, 0
	}

	if *verFlag {
		return cmdVersion, 0
	}

	cliTimeout = *timeoutFlag
	cliRepair = *repairFlag
	cliHeadless = *headlessFlag

	if fs.NArg() > 0 {
		switch fs.Arg(0) {
		case "help":
			return cmdHelp, 0
		case "setup":
			return cmdSetup, 0
		case "run":
			if fs.NArg() < 2 {
				return cmdUnknown, 2
			}
			n, err := strconv.Atoi(fs.Arg(1))
			if err != nil || n < 1 {
				return cmdUnknown, 2
			}
			runN = n
			runIssueNum = 0
			if fs.NArg() >= 3 {
				num, err := strconv.Atoi(fs.Arg(2))
				if err != nil || num < 1 {
					return cmdUnknown, 2
				}
				runIssueNum = num
			}
			return cmdRun, 0
		case "status":
			return cmdStatus, 0
		case "check":
			return cmdCheck, 0
		case "restore":
			return cmdRestore, 0
		case "download":
			return cmdDownload, 0
		case "repair":
			return cmdRepair, 0
		case "checksum":
			if fs.NArg() < 2 || fs.Arg(1) != "verify" {
				return cmdUnknown, 2
			}
			return cmdChecksum, 0
		case "completion":
			if fs.NArg() > 2 {
				return cmdUnknown, 2
			}
			if fs.NArg() == 2 && fs.Arg(1) != "bash" {
				return cmdUnknown, 2
			}
			return cmdCompletion, 0
		case "commands":
			return cmdCommands, 0
		case "screenshot":
			return cmdScreenshot, 0
		}
		return cmdUnknown, 2
	}

	return cmdTUI, 0
}

// reorderFlags moves all flag arguments (starting with -) before positional
// arguments so that Go's flag.Parse can find them even when they appear after
// positional arguments (e.g. "loop run 1 --headless").
func reorderFlags(args []string) []string {
	var result, positional []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--timeout" || a == "-timeout" {
			result = append(result, a)
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				result = append(result, args[i+1])
				i++
			}
		} else if strings.HasPrefix(a, "-") {
			result = append(result, a)
		} else {
			positional = append(positional, a)
		}
	}
	return append(result, positional...)
}

func printUsage() {
	fmt.Fprint(os.Stderr, usage)
}

func requireConfig() {
	_, exists, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading config: %v\n", err)
		osExit(1)
	}
	if exists {
		return
	}
	runSetupFn()
	_, exists, err = config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading config: %v\n", err)
		osExit(1)
	}
	if !exists {
		fmt.Fprintln(os.Stderr, "Not configured — run 'loop setup' first")
		osExit(1)
	}
}

func runSetup() {
	_, exists, err := config.Load()
	if err == nil && exists {
		fmt.Fprintln(os.Stderr, "Config already exists — skipping setup")
		return
	}
	p := tea.NewProgram(setup.NewModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runTUI(cfg config.Config) {
	m := dashboard.NewModel(cfg)
	m.Version = Version
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runRun(n int) {
	cfg, exists, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading config: %v\n", err)
		osExit(1)
		return
	}
	if !exists {
		fmt.Fprintln(os.Stderr, "Not configured — run 'loop setup' first")
		osExit(1)
		return
	}

	if cliTimeout > 0 {
		cfg.AgentTimeout = cliTimeout
	}

	if runIssueNum <= 0 {
		ps, err := issue.ScanIssueDir(cfg.IssueDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error scanning issues: %v\n", err)
			osExit(1)
			return
		}
		if len(ps.TodoFiles) == 0 && len(ps.TestReadyFiles) == 0 {
			fmt.Fprintf(os.Stderr, "no issues found in pipeline\n")
			return
		}
	}

	if cliHeadless {
		runHeadlessRun(n, &cfg)
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)

	m := run.NewStreamingModel(cfg, n, stop, func(logChan chan<- string, iterChan chan<- run.ProgressMsg, doneChan chan<- error) {
		defer stop()

		err := runner.RunLoopStreamed(ctx, &cfg, n, runIssueNum, cliRepair, func(line string) {
			logChan <- line
		}, func(iter, total int, title, role string) {
			iterChan <- run.ProgressMsg{
				Iteration:  iter,
				Total:      total,
				IssueTitle: title,
				IssueRole:  role,
			}
		})
		doneChan <- err
		close(logChan)
	})

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		osExit(1)
	}

	m2 := m.(*run.Model)
	if m2.Err != nil {
		if errors.Is(m2.Err, context.Canceled) {
			fmt.Fprintf(os.Stderr, "interrupted\n")
			printShutdownSummary(m2.Iteration(), n, cfg.IssueDir)
			return
		}
		if errors.Is(m2.Err, issue.ErrPreFlightFailed) {
			osExit(1)
		} else {
			fmt.Fprintf(os.Stderr, "error: %v\n", m2.Err)
			osExit(1)
		}
	}
	printShutdownSummary(m2.Iteration(), n, cfg.IssueDir)
}

func runHeadlessRun(n int, cfg *config.Config) {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	err := runner.RunLoopContext(ctx, cfg, n, runIssueNum, cliRepair)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			fmt.Fprintf(os.Stderr, "interrupted\n")
		} else {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			osExit(1)
		}
	}
	printShutdownSummary(n, n, cfg.IssueDir)
}

func printShutdownSummary(iterations, total int, issuesDir string) {
	ps, err := issue.ScanIssueDir(issuesDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nerror scanning pipeline: %v\n", err)
		return
	}
	counts := ps.Counts()

	fmt.Fprintf(os.Stderr, "\nRun summary — %d/%d iterations completed\n", iterations, total)
	fmt.Fprintf(os.Stderr, "  todo: %d  test-ready: %d  done: %d  quarantined: %d\n",
		counts[issue.StateTodo],
		counts[issue.StateTestReady],
		counts[issue.StateDone],
		counts[issue.StateQuarantine])
}

func runCheck() {
	cfg, exists, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading config: %v\n", err)
		osExit(1)
		return
	}
	if !exists {
		fmt.Fprintln(os.Stderr, "Not configured — run 'loop setup' first")
		osExit(1)
		return
	}

	state, err := issue.ScanIssueDir(cfg.IssueDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error scanning issue directory: %v\n", err)
		osExit(1)
		return
	}

	issues := issue.PreFlightCheck(state, cliRepair, cfg.ChecksumsEnabled)

	if cfg.Repo != "" && github.CheckAuthOnce() {
		repo, repoErr := github.RepoFromString(cfg.Repo)
		if repoErr == nil {
			reopenedIssues, reopenErr := github.RepairGitHubState(repo, state)
			if reopenErr != nil {
				fmt.Fprintf(os.Stderr, "warning: repair GitHub state: %v\n", reopenErr)
			}
			for _, ri := range reopenedIssues {
				fmt.Fprintf(os.Stderr, "reopened prematurely closed GitHub issue #%d (%s)\n", ri.Number, ri.File)
			}
		}
	}

	hasErrors := false
	for _, i := range issues {
		fmt.Fprintf(os.Stderr, "%s: %s\n", i.Severity, i.Message)
		if i.Severity == issue.SeverityError {
			hasErrors = true
		}
	}
	if hasErrors {
		osExit(1)
		return
	}

	cleaned, err := issue.CleanTestResults(cfg.IssueDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error cleaning result sections: %v\n", err)
		osExit(1)
		return
	}
	for _, p := range cleaned {
		fmt.Printf("cleaned: removed result sections from %s\n", p)
	}
	if len(cleaned) == 0 {
		fmt.Println("No issues found — pipeline state is valid")
	}
}

func runRepair() {
	cfg, exists, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading config: %v\n", err)
		osExit(1)
		return
	}
	if !exists {
		fmt.Fprintln(os.Stderr, "Not configured — run 'loop setup' first")
		osExit(1)
		return
	}

	labelsAdded, execModesAdded, stripped, stuckCount, testResultsPromoted, invalidExecModes, checksumsAdded, err := issue.RepairPipeline(cfg.IssueDir, cfg.ChecksumsEnabled)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: repair pipeline: %v\n", err)
	} else {
		if labelsAdded > 0 {
			fmt.Fprintf(os.Stderr, "added Status: ready-for-agent to %d file(s)\n", labelsAdded)
		}
		if execModesAdded > 0 {
			fmt.Fprintf(os.Stderr, "added Execution mode: AFK-only to %d file(s)\n", execModesAdded)
		}
		if stripped > 0 {
			fmt.Fprintf(os.Stderr, "stripped empty UAT Results from %d file(s)\n", stripped)
		}
		if stuckCount > 0 {
			fmt.Fprintf(os.Stderr, "found %d test-ready file(s) with populated UAT Results — already tested but never transitioned; left in place for manual review\n", stuckCount)
		}
		if testResultsPromoted > 0 {
			fmt.Fprintf(os.Stderr, "promoted %d todo file(s) with Test Results to test-ready/\n", testResultsPromoted)
		}
		if invalidExecModes > 0 {
			fmt.Fprintf(os.Stderr, "found %d file(s) with invalid Execution mode — review and fix manually\n", invalidExecModes)
		}
		if checksumsAdded > 0 {
			fmt.Fprintf(os.Stderr, "added/updated checksums for %d file(s)\n", checksumsAdded)
		}
	}

	if cfg.Repo != "" && github.CheckAuthOnce() {
		state, scanErr := issue.ScanIssueDir(cfg.IssueDir)
		if scanErr == nil {
			repo, repoErr := github.RepoFromString(cfg.Repo)
			if repoErr == nil {
				reopened, reopenErr := github.RepairGitHubState(repo, state)
				if reopenErr != nil {
					fmt.Fprintf(os.Stderr, "warning: GitHub state repair: %v\n", reopenErr)
				}
				for _, ri := range reopened {
					fmt.Fprintf(os.Stderr, "reopened prematurely closed GitHub issue #%d (%s)\n", ri.Number, ri.File)
				}

				fixedLabels, labelErr := github.FixMissingLabels(repo, state)
				if labelErr != nil {
					fmt.Fprintf(os.Stderr, "warning: label fix: %v\n", labelErr)
				}
				for _, num := range fixedLabels {
					fmt.Fprintf(os.Stderr, "fixed missing label for GitHub issue #%d\n", num)
				}

				ensuredTestReady, trErr := github.EnsureTestReadyLabels(repo, state)
				if trErr != nil {
					fmt.Fprintf(os.Stderr, "warning: ensure test-ready labels: %v\n", trErr)
				}
				for _, num := range ensuredTestReady {
					fmt.Fprintf(os.Stderr, "ensured test-ready label for GitHub issue #%d\n", num)
				}
			}
		}
	}

	if err == nil && labelsAdded == 0 && stripped == 0 && stuckCount == 0 && testResultsPromoted == 0 {
		fmt.Println("No issues found — pipeline state is valid")
	}
}

func runChecksumVerify() {
	cfg, exists, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading config: %v\n", err)
		osExit(1)
		return
	}
	if !exists {
		fmt.Fprintln(os.Stderr, "Not configured — run 'loop setup' first")
		osExit(1)
		return
	}

	results, err := issue.VerifyChecksums(cfg.IssueDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error verifying checksums: %v\n", err)
		osExit(1)
		return
	}

	hasErrors := false
	for _, r := range results {
		if r.Err != nil {
			fmt.Fprintf(os.Stderr, "error: %s: %v\n", r.FilePath, r.Err)
			hasErrors = true
			continue
		}
		if r.Valid {
			fmt.Printf("ok: %s\n", r.FilePath)
		} else {
			fmt.Fprintf(os.Stderr, "FAIL: %s — checksum mismatch\n", r.FilePath)
			hasErrors = true
		}
	}

	if len(results) == 0 {
		fmt.Println("No files with checksums found")
		return
	}

	if hasErrors {
		osExit(1)
	}
}

func runDownload() {
	cfg, exists, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading config: %v\n", err)
		osExit(1)
		return
	}

	if !exists {
		fmt.Fprintln(os.Stderr, "Not configured — run 'loop setup' first")
		osExit(1)
		return
	}

	if cfg.Repo == "" {
		fmt.Fprintln(os.Stderr, "No GitHub repo configured — run 'loop setup' first")
		osExit(1)
		return
	}

	if !github.CheckAuthOnce() {
		osExit(1)
		return
	}

	repo, err := github.RepoFromString(cfg.Repo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing repo: %v\n", err)
		osExit(1)
		return
	}

	pattern := github.ArchivePattern("loop")
	destDir := "."

	fmt.Fprintf(os.Stderr, "==> Downloading latest release from %s ...\n", repo)
	files, err := github.DownloadLatestRelease(repo, pattern, destDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error downloading release: %v\n", err)
		osExit(1)
		return
	}

	for _, f := range files {
		fmt.Println(f)
	}
	fmt.Fprintf(os.Stderr, "==> Downloaded %d file(s) to %s\n", len(files), destDir)
}

func runRestore() {
	if err := git.RestoreContextFromFile(); err != nil {
		if err.Error() == "no saved git context found" {
			fmt.Println("no git context to restore")
			return
		}
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		osExit(1)
		return
	}
	fmt.Println("git context restored successfully")
}

func runStatus() {
	cfg, exists, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading config: %v\n", err)
		osExit(1)
		return
	}
	if !exists {
		fmt.Fprintln(os.Stderr, "Not configured — run 'loop setup' first")
		osExit(1)
		return
	}
	p := tea.NewProgram(status.NewModel(cfg), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		osExit(1)
	}
}

func runScreenshot() {
	cfg, exists, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading config: %v\n", err)
		osExit(1)
		return
	}
	if !exists {
		fmt.Fprintln(os.Stderr, "Not configured — run 'loop setup' first")
		osExit(1)
		return
	}

	state, err := issue.ScanIssueDir(cfg.IssueDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error scanning issue directory: %v\n", err)
		osExit(1)
		return
	}

	var b strings.Builder
	counts := state.Counts()
	b.WriteString("Pipeline overview\n\n")
	b.WriteString(fmt.Sprintf("  todo:       %d\n", counts[issue.StateTodo]))
	b.WriteString(fmt.Sprintf("  test-ready: %d\n", counts[issue.StateTestReady]))
	b.WriteString(fmt.Sprintf("  done:       %d\n", counts[issue.StateDone]))
	b.WriteString(fmt.Sprintf("  quarantined: %d\n", counts[issue.StateQuarantine]))

	if cfg.Repo != "" {
		b.WriteString(fmt.Sprintf("  repo:       %s\n", cfg.Repo))
	}
	b.WriteString(fmt.Sprintf("  issues dir: %s\n", cfg.IssueDir))

	name, err := screenshot.Save(b.String(), "pipeline")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error saving screenshot: %v\n", err)
		osExit(1)
	}
	fmt.Fprintf(os.Stderr, "screenshot saved: %s\n", name)
}

func runCommands() {
	fmt.Println(`Commands:
  loop                      Start the TUI dashboard
  loop setup                Interactive setup wizard
  loop run <n> [issue]      Run N AFK iterations (optionally force a specific issue)
  loop status               Show pipeline state
  loop check                Validate pipeline state for issues
  loop repair               Repair pipeline: strip empty UAT Results, promote stuck files, fix GitHub labels
  loop restore              Restore git context (original branch + pop stash) if loop was interrupted
  loop download             Download the latest release from GitHub
  loop checksum verify      Verify file content checksums
  loop screenshot           Save a terminal screenshot of the pipeline state
   loop completion bash      Print bash completion script
  loop commands             Show this table

Flags:
  --help, -h                Show help
  --version                 Show version
  --timeout <secs>          Agent timeout in seconds (overrides config)
  --repair                  Repair GitHub state (reopen prematurely closed issues)`)
}

func printBashCompletion() {
	fmt.Print(`_loop_completions()
{
	local cur prev words cword
	COMPREPLY=()
	_get_comp_words_by_ref -n : cur prev words cword

	if [[ $cword -eq 1 ]]; then
		COMPREPLY=($(compgen -W "help setup run status check repair restore download checksum screenshot completion commands --help -h --version --timeout --repair --headless" -- "${cur}"))
		return 0
	fi

	case "${prev}" in
		run)
			COMPREPLY=($(compgen -W "1 2 3 5 10 20 50" -- "${cur}"))
			return 0
			;;
		checksum)
			COMPREPLY=($(compgen -W "verify" -- "${cur}"))
			return 0
			;;
		completion)
			COMPREPLY=($(compgen -W "bash" -- "${cur}"))
			return 0
			;;
		--timeout)
			COMPREPLY=($(compgen -W "30 60 120 300 600" -- "${cur}"))
			return 0
			;;
		*)
			if [[ ${cur} == -* ]]; then
				COMPREPLY=($(compgen -W "--help -h --version --timeout --repair --headless" -- "${cur}"))
			else
				COMPREPLY=($(compgen -W "help setup run status check repair restore download checksum screenshot completion commands" -- "${cur}"))
			fi
			return 0
			;;
	esac
}

complete -F _loop_completions loop
`)
}

func main() {
	cmd, exitCode := parseArgs(os.Args[1:])
	switch cmd {
	case cmdHelp:
		printUsage()
		osExit(0)
	case cmdVersion:
		fmt.Printf("loop v%s %s/%s\n", strings.TrimPrefix(Version, "v"), GOOS, GOARCH)
		osExit(0)
	case cmdSetup:
		runSetupFn()
	case cmdRun:
		requireConfig()
		runRun(runN)
	case cmdStatus:
		requireConfig()
		runStatusFn()
	case cmdCheck:
		requireConfig()
		runCheckFn()
	case cmdRepair:
		requireConfig()
		runRepairFn()
	case cmdDownload:
		requireConfig()
		runDownloadFn()
	case cmdChecksum:
		requireConfig()
		runChecksumVerifyFn()
	case cmdRestore:
		runRestoreFn()
	case cmdCompletion:
		runCompletionFn()
	case cmdCommands:
		runCommandsFn()
	case cmdScreenshot:
		requireConfig()
		runScreenshotFn()
	case cmdUnknown:
		printUsage()
		osExit(exitCode)
	case cmdTUI:
		requireConfig()
		cfg, _, _ := config.Load()
		runTUIFn(cfg)
	}
}
