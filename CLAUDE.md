# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Test

```bash
go build -o bin/pr-sentinel ./cmd/pr-sentinel   # build
go test ./...                                     # all tests
go test ./internal/reviewer/ -v                   # single package
go test ./internal/github/ -run TestParseFoo -v   # single test
```

No Makefile, no linter configured. Standard Go toolchain only.

## Architecture

pr-sentinel is a CLI daemon that polls GitHub for open PRs and reviews them using Claude Code (`claude -p`) running inside each repo directory. This preserves the repo's `.claude/` context (CLAUDE.md, memory, skills).

**Data flow:** Poller (GitHub GraphQL) → Reviewer (claude CLI subprocess) → Publisher (gh pr review / local file) → Notifier (Teams/Slack/macOS/webhook)

### Key packages

- **`cmd/pr-sentinel/`** — Cobra root command, registers all subcommands, `--verbose` flag sets slog to LevelDebug
- **`internal/daemon/poller.go`** — Core orchestrator. `RunPollCycle` delegates to `RunPollCycleWith` (testable via `PRFetcher` interface). Three phases: (1) collect work items sequentially (new PRs + follow-up candidates), detect closed PRs, (2) fan out reviews in parallel via semaphore with context-aware acquire, (3) process outcomes sequentially (publish with verdict-aware review via `--approve`/`--comment`/`--request-changes`, auto-merge on approval with finding gate, record, notify per-repo + global). `RunDaemon` wraps this in a ticker loop with config hot-reload each cycle, schedule guard (skips cycles outside configured window), and health file writes
- **`internal/daemon/health.go`** — `HealthStatus` struct written to `~/.config/pr-sentinel/health.json` after each poll cycle (last_poll, cycle_count, last_errors, pid, schedule_active, next_window_change)
- **`internal/daemon/launchd.go`** — macOS plist generation. The plist invokes `start --daemon-mode` (not `--daemon`). `--daemon` installs+loads the plist; `--daemon-mode` runs the actual loop. Uses `config.ConfigDir()` for paths
- **`internal/reviewer/claude.go`** — Spawns `claude -p` with `--output-format json --json-schema <schema> --allowedTools <read-only+gh/git>`. Does NOT pass the diff in the prompt — Claude fetches it itself via `gh pr diff`. Includes heartbeat goroutine (logs every 30s during review). `BuildFollowUpPrompt` includes previous review text for re-reviews
- **`internal/reviewer/output.go`** — Structured review types (`StructuredReview`, `Finding`, `Verdict`). Parses the claude CLI JSON envelope, extracting `structured_output` field (from `--json-schema`), falls back to `result`. `FormatMarkdown()` renders findings for GitHub posting. `ParseResult` includes `CostUSD`
- **`internal/github/client.go`** — GraphQL query via `gh api graphql` with `rateLimit` field. `FetchOpenPRs` returns two lists: new PRs and follow-up candidates (previously reviewed PRs with new commits since last comment). Filters drafts, self-authored PRs. Fetches PR labels for auto-merge gating. `PostReview` accepts verdict and maps to `--approve`/`--comment`/`--request-changes` flags. `GetPRState` confirms PR closure via `gh pr view --json state`. Logs rate limit at debug level, warns when <20% remaining
- **`internal/github/merge.go`** — `EnableAutoMerge` wraps `gh pr merge --auto` with configurable strategy (merge/squash/rebase) and `--delete-branch`. Uses GitHub's merge queue to defer merge until CI/branch protection passes
- **`internal/schedule/schedule.go`** — Standalone package for schedule window logic. `Config` struct with `Days`, `StartTime`, `EndTime`, `Timezone`. `IsActive(t)` checks if time falls within window (supports overnight windows where end < start). `NextWindowOpen(t)` finds next window opening. `Validate()` checks days/times/timezone. Fails open on invalid timezone at runtime (Validate catches at config load)
- **`internal/state/store.go`** — SQLite at `~/.config/pr-sentinel/state.db` with `0600` permissions. Tables: `reviewed_prs` (always INSERT, no UNIQUE — preserves review history), `daily_counts`. Columns include `cost_usd` and `closed_at`. Key methods: `RecordReview`, `HasReviewed`, `GetReview` (latest by reviewed_at), `TrackedOpenPRNumbers`, `MarkPRClosed`, `DailyCost`, `RecentReviews`. Migrations handle schema evolution (drop UNIQUE, add cost_usd, add closed_at)
- **`internal/publisher/publisher.go`** — Two modes: `PostLiveReview` (gh pr review, passes verdict for proper GitHub review state) and `SaveDryRunReview` (timestamped markdown to `~/.config/pr-sentinel/reviews/`)
- **`internal/notifier/`** — `Dispatcher` fans out to multiple `Notifier` implementations. Per-repo `teams_webhook` overrides global webhook. Teams cards use Adaptive Card format with verdict display (✅ Approved / 💬 Comment / ❌ Changes Requested), review summary, and auto-merge status. `Event` struct includes `Verdict`, `Summary`, `AutoMerge` fields. `redactURL` helper prevents webhook token leakage in logs
- **`internal/retry/retry.go`** — Generic `Do(maxAttempts, baseDelay, desc, fn)` with exponential backoff. Used for PostLiveReview (3 attempts, 2s base)
- **`internal/config/config.go`** — `Validate()` method checks all numeric fields > 0, repo names in owner/repo format, schedule config, auto_merge strategy. `AutoMergeConfig` struct for per-repo merge settings. File permissions: `0600` for config, `0700` for directories

### Commands

`init`, `start` (with `--once`, `--daemon`/`--daemon-mode`), `stop`, `status`, `review`, `repos`, `promote`, `demote`, `logs`, `notify-test` (with `--repo` flag)

### Config

YAML at `~/.config/pr-sentinel/config.yaml`. Loaded via `config.Load()` (calls `Validate()`), saved via `config.Save()`. `DefaultConfig()` provides fallback values. Per-repo settings: `mode`, `review_instructions`, `teams_webhook`, `auto_merge`. Global settings include `schedule` (days/start_time/end_time/timezone). Config is hot-reloaded each poll cycle.

### Schedule

Optional `schedule` section restricts when the daemon polls. Omitting it means 24/7 operation. Supports overnight windows (`end_time < start_time` crosses midnight). `start --once` and `review` commands ignore the schedule. The `status` command shows whether the schedule is active or paused.

### Auto-merge

Per-repo `auto_merge` config enables automatic merge via `gh pr merge --auto` when: verdict is `approve`, zero HIGH/MEDIUM findings, mode is `live`, and optional `require_label` matches. Strategy can be `merge`, `squash`, or `rebase`. In dry-run mode, logs what would happen without merging.

### Review output contract

Claude CLI returns a JSON envelope with `structured_output` containing:
```json
{"verdict": "approve|comment|request-changes", "summary": "...", "findings": [{"severity": "HIGH|MEDIUM|LOW", "file": "...", "line": 42, "message": "..."}]}
```
This is enforced by `--json-schema` flag. If parsing fails, falls back to raw text.

## Conventions

- **Git:** Commits signed by Jose Moffa. Namespace `moffa90/*` uses `<moffa3@gmail.com>`. No Co-Authored-By or Claude Code footers.
- **Logging:** `log/slog` throughout. `slog.Info` for operational events, `slog.Debug` for verbose (enabled by `-v` flag), `slog.Error` for failures, `slog.Warn` for degraded state (e.g., low rate limit).
- **Error wrapping:** `fmt.Errorf("context: %w", err)` pattern. Use `errors.Is()` for context error checks.
- **Tests:** Table-driven, `*_test.go` alongside implementation. GitHub client tests use hardcoded JSON responses. `poll_cycle_test.go` uses `mockFetcher` implementing `PRFetcher` interface.
- **Security:** `--allowedTools` restricts Claude to read-only during reviews. File permissions `0600`/`0700`. Webhook URLs redacted in error messages. AppleScript inputs escaped.
