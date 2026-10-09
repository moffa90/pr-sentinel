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

`scripts/patch-config.py` is a one-off Python 3.9+ migration for one specific live config (it names `Cellgain/dms-gateway` and `moffa90/pr-sentinel`), not a general tool. It checks every edit's match count before writing, is a no-op on re-run (including after `config.Save` rewrites its flow-style lists and quotes in yaml.v3's style), backs up to `config.yaml.bak-<timestamp>` at 0600 and writes atomically.

## Architecture

pr-sentinel is a CLI daemon that polls GitHub for open PRs and reviews them using Claude Code (`claude -p`) running inside each repo directory. This preserves the repo's `.claude/` context (CLAUDE.md, memory, skills).

**Data flow:** Poller (GitHub GraphQL) → Reviewer (claude CLI subprocess) → Publisher (gh pr review / local file) → Notifier (Teams/Slack/macOS/webhook)

### Key packages

- **`cmd/pr-sentinel/`** — Cobra root command, registers all subcommands, `--verbose` flag sets slog to LevelDebug
- **`internal/daemon/poller.go`** — Core orchestrator. `RunPollCycle` delegates to `RunPollCycleWith` (testable via `PRFetcher` interface). Three phases: (1) collect work items sequentially (new PRs + follow-up candidates), detect closed PRs, (2) fan out reviews in parallel via semaphore with context-aware acquire, (3) process outcomes sequentially via `ProcessReview`. Phase 1 decides new / follow-up / skip per PR in `followup.go` (`planUnreviewedOnGitHub`, `planForCandidate`) from GitHub's view plus the stored `head_oid`; see Follow-up detection. `RunDaemon` wraps this in a ticker loop with config hot-reload each cycle, schedule guard (skips cycles outside configured window), and health file writes
- **`internal/daemon/logfile.go`** — `SetupDaemonLogging` routes slog in `--daemon-mode` to `~/.config/pr-sentinel/daemon.log` via lumberjack (10 MB per file, 3 compressed backups, 0600) and restricts existing log files to 0600. launchd's `daemon.stdout.log`/`daemon.stderr.log` now only capture crash output
- **`internal/daemon/health.go`** — `HealthStatus` struct written to `~/.config/pr-sentinel/health.json` after each poll cycle (last_poll, cycle_count, last_errors, pid, schedule_active, next_window_change)
- **`internal/daemon/launchd.go`** — macOS plist generation. The plist invokes `start --daemon-mode` (not `--daemon`). `--daemon` installs+loads the plist; `--daemon-mode` runs the actual loop. Uses `config.ConfigDir()` for paths. Sets `Umask` 63 (0077) so everything the daemon creates is owner-only
- **`internal/reviewer/claude.go`** — Spawns `claude -p` with `--output-format json --json-schema <schema> --allowedTools <read-only+gh/git>`. Does NOT pass the diff in the prompt — Claude fetches it itself via `gh pr diff`. Includes heartbeat goroutine (logs every 30s during review). `BuildFollowUpPrompt` includes previous review text for re-reviews. `RunReviewWithModel`/`BuildClaudeArgsWithModel` pass `--model`/`--fallback-model` from `review.model`/`review.fallback_model` (default `opus`, the alias for the latest Opus); `RunReview`/`BuildClaudeArgs` keep the CLI default. The models the CLI actually used (envelope `modelUsage`) are logged, with a warning if they don't match the requested model
- **`internal/reviewer/output.go`** — Structured review types (`StructuredReview`, `Finding`, `Verdict`). Parses the claude CLI JSON envelope, extracting `structured_output` field (from `--json-schema`), falls back to `result`. `FormatMarkdown()` renders findings for GitHub posting. `ParseResult` includes `CostUSD` and `Models` (from `modelUsage`)
- **`internal/github/client.go`** — GraphQL query via `gh api graphql` with `rateLimit` field. `FetchOpenPRs` returns two lists: PRs pr-sentinel hasn't reviewed on GitHub, and follow-up candidates (pr-sentinel's last review was written against a commit other than `headRefOid`). Each `PullRequest` carries `HeadOID` and `CommitOIDs`. Filters drafts, self-authored PRs. Fetches PR labels for auto-merge gating. `PostReview` accepts verdict and maps to `--approve`/`--comment`/`--request-changes` flags. `PostReviewAtCommit` posts through REST with `commit_id`, pinning the review to the polled head (`gh pr review` can't). `GetPRState` confirms PR closure via `gh pr view --json state`. `GetPR` fetches a single PR's metadata (title, author, stats, labels, URL) for the `review` command. Logs rate limit at debug level, warns when <20% remaining
- **`internal/github/merge.go`** — `EnableAutoMerge` wraps `gh pr merge --auto` with configurable strategy (merge/squash/rebase) and `--delete-branch`. Uses GitHub's merge queue to defer merge until CI/branch protection passes
- **`internal/github/labels.go`** — `EnsureLabel` runs `gh label create` without `--force`, so an existing label (and a color a team chose) is left alone; "already exists" counts as success
- **`internal/github/issues.go`** — `CreateIssue` (`gh issue create`, body via stdin, returns number+URL), `CommentOnIssue` and `GetIssueState` (take an issue number or URL)
- **`internal/daemon/process.go`** — `ProcessReview` holds every post-review step shared by the daemon and the `review` command: publish with verdict-aware review via `--approve`/`--comment`/`--request-changes` (falls back to `--comment` on self-authored PRs), auto-merge on approval with finding gate, issues, record, notify per-repo + global. `ReviewBody` renders the formatted markdown body. All GitHub writes go through the `GitHubActions` interface (`GitHubCLI` in production); `ProcessReviewWith` accepts a mock for tests. Add new post-review behavior here so both paths stay identical
- **`internal/daemon/issues.go`** — `handleIssues` runs in phase 3 after publish. Filters findings by `issues.min_severity`, creates one issue per PR, or comments on the existing one for follow-up reviews. Only for `approve` verdicts. Opens a new issue if the existing one is closed. Claims the row before creating, records the URL even when the issue number can't be parsed (gh accepts either), and retries the state write, to avoid duplicates
- **`internal/schedule/schedule.go`** — Standalone package for schedule window logic. `Config` struct with `Days`, `StartTime`, `EndTime`, `Timezone`. `IsActive(t)` checks if time falls within window (supports overnight windows where end < start). `NextWindowOpen(t)` finds next window opening. `Validate()` checks days/times/timezone. Fails open on invalid timezone at runtime (Validate catches at config load)
- **`internal/state/store.go`** — SQLite at `~/.config/pr-sentinel/state.db` with `0600` permissions and `busy_timeout(5000)` so the daemon and the `review` command can write concurrently. WAL is not used: its `-wal`/`-shm` files would not get `0600`. Tables: `reviewed_prs` (always INSERT, no UNIQUE — preserves review history), `daily_counts`, `review_attempts` (PK `repo, pr_number, head_oid` — every daemon model run, see Cost guards), `created_issues` (PK `repo, pr_number` — one issue per PR). Columns include `cost_usd`, `closed_at`, `models` (comma-separated model IDs from the CLI's `modelUsage`, shown by `logs`) and `head_oid` (the commit the review was written against). Key methods: `RecordReview`, `SetHeadOID`, `BeginAttempt`/`FinishAttempt`/`GetAttempt`/`PruneAttempts`, `HasReviewed`, `GetReview` (latest by reviewed_at), `TrackedOpenPRNumbers`, `MarkPRClosed`, `DailyCost`, `RecentReviews`, `RecordIssue`, `GetIssue`, `ClaimIssue`, `ReleaseIssueClaim`, `DeleteIssue`. Migrations handle schema evolution (drop UNIQUE, add cost_usd, add closed_at, add models, add head_oid)
- **`internal/publisher/publisher.go`** — Two modes: `PostLiveReview` (gh pr review, passes verdict for proper GitHub review state) and `SaveDryRunReview` (timestamped markdown to `~/.config/pr-sentinel/reviews/`, directory 0700 and files 0600 since they hold PR content)
- **`internal/notifier/`** — `Dispatcher` fans out to multiple `Notifier` implementations. Per-repo `teams_webhook` overrides global webhook. Teams cards use Adaptive Card format with verdict display (✅ Approved / 💬 Comment / ❌ Changes Requested), review summary, and auto-merge status. `Event` struct includes `Verdict`, `Summary`, `AutoMerge`, `Issue` fields (auto-merge and issue status shown in Teams and Slack). `redactURL` helper prevents webhook token leakage in logs
- **`internal/retry/retry.go`** — Generic `Do(maxAttempts, baseDelay, desc, fn)` with exponential backoff. Used for PostLiveReview (3 attempts, 2s base)
- **`internal/config/config.go`** — `Validate()` method checks all numeric fields > 0, repo names in owner/repo format, schedule config, auto_merge strategy. `AutoMergeConfig` struct for per-repo merge settings. File permissions: `0600` for config, `0700` for directories. `os.WriteFile`'s mode only applies on create, so `Save` chmods afterwards and `Load` tightens a config readable by group or others (with a warning), since a hand- or script-edited file keeps its old mode

### Commands

`init`, `start` (with `--once`, `--daemon`/`--daemon-mode`), `stop`, `status`, `review`, `repos`, `promote`, `demote`, `logs`, `notify-test` (with `--repo` flag)

### Config

YAML at `~/.config/pr-sentinel/config.yaml`. Loaded via `config.Load()` (calls `Validate()`), saved via `config.Save()`. `DefaultConfig()` provides fallback values. Per-repo settings: `mode`, `review_instructions`, `teams_webhook`, `auto_merge`, `issues`. Global settings include `schedule` (days/start_time/end_time/timezone). Config is hot-reloaded each poll cycle.

### Schedule

Optional `schedule` section restricts when the daemon polls. Omitting it means 24/7 operation. Supports overnight windows (`end_time < start_time` crosses midnight). `start --once` and `review` commands ignore the schedule.

### Review command

`review <pr-url>` fetches PR metadata with `GetPR`, uses the same model and instructions as the daemon, and picks the prompt the same way: the follow-up prompt with the previous review when the PR was reviewed before, otherwise the full review prompt. It prints the exact body, asks for confirmation in live mode, then calls `daemon.ProcessReview`. It records state, so the daemon won't review the same PR again, but sets `SkipDailyCount` so manual reviews don't use up the daemon's `max_reviews_per_day` budget. The `status` command shows whether the schedule is active or paused.

### Model

`review.model` defaults to `opus` (the claude CLI alias for the latest Opus) and applies to existing configs that don't set it, which may cost more than the CLI's own default. Set `model: ""` to use the claude CLI default instead. `review.fallback_model` is optional and must differ from `model`.

### Auto-merge

Per-repo `auto_merge` config enables automatic merge via `gh pr merge --auto` when: verdict is `approve`, zero HIGH/MEDIUM findings, mode is `live`, and optional `require_label` matches. In live mode a missing `require_label` is created in the repo, so a human can apply it; until someone does, auto-merge stays off for that repo. Strategy can be `merge`, `squash`, or `rebase`. In dry-run mode, logs what would happen without merging.

### Issues

Per-repo `issues` config (`enabled`, `min_severity` HIGH|MEDIUM|LOW — default HIGH, `labels`). Issues track findings left on **approved** PRs: when a review's verdict is `approve` and it still has findings at or above `min_severity`, those findings won't be fixed in the PR, so pr-sentinel opens an issue in the PR's repo. Reviews with `comment` or `request-changes` never open issues; those findings are handled in the PR itself. One issue per PR, with findings as a checklist and a cross-link to the PR. Later approved reviews with qualifying findings comment on the existing issue, or open a new one if it was closed. Before creating an issue, a claim row in `created_issues` stops the daemon and the `review` command from both creating one; claims older than 10 minutes are treated as stale. Finding text is passed through `reviewer.NeutralizeMentions` (zero-width space after `@`/`#`) so it can't ping users or cross-link other issues. Dry-run mode only logs. Issues are created by pr-sentinel, not by Claude, so the reviewer stays read-only. Requires the `repo` scope on the gh token. Configured `labels` are created in the target repo if missing (`GitHubCLI.EnsureLabel`, cached per process); a label that can't be created is dropped with a warning instead of failing the issue, because `gh issue create` refuses a missing label outright. If gh still refuses a labelled issue (a cached label was deleted since), the cache entries are forgotten and the issue is retried once without labels.

### Cost guards

Each model run costs money whether or not it succeeds, so the daemon budgets runs, not successful reviews (`internal/daemon/attempts.go`):

- **Every run counts** toward `max_reviews_per_day`, recorded when it starts (phase 2), before the model runs. `ProcessReview` no longer counts; `PollOptions.SkipDailyCount` is deprecated and ignored. Manual `review` runs don't count against the daemon's budget.
- **Every run is recorded** in `review_attempts` per (repo, PR, head commit) before it starts, and its outcome after (`succeeded`, `last_error`).
- **`attemptGate`** in phase 1, after the follow-up decision: a head whose review was posted is never run again by the daemon (even if recording the review failed); a failed head backs off 10m, then 20m, measured from when the run failed; after `maxAttemptsPerHead` (3) failures it is **parked** with one warning, until the PR's head moves. A new head starts clean.
- A run interrupted by daemon shutdown stays counted in the daily budget but is reverted from the attempt count (`RevertAttempt`), so restarts can't park a PR.
- Pruning (each cycle) drops records idle for 30 days that protect nothing (failures below the park limit, or any record for a PR seen closed). Succeeded and parked records on open PRs are kept; everything goes after 180 days.

### Follow-up detection

No clock is compared. A PR needs a follow-up when its head commit differs from the commit pr-sentinel last reviewed it against; a commit authored before a review but pushed after it (a date-preserving rebase, or a slow push) still moves the head.

- **GitHub side** (`parseGraphQLResponse`): pr-sentinel's own reviews are found with `reviews(last: 20, author: $author, states: [non-PENDING])` and recognised by `github.IsSentinelReview` (a full `**Verdict: Approved|Changes Requested|Comment**` line written by `FormatMarkdown`). The same account's manual reviews and thread replies don't count: GitHub stamps them with the head at posting time, so counting them would hide a push. Manual reviews never suppress pr-sentinel.
- **State side** (`head_oid`): covers dry-run and promoted repos, which never post. The newer of the stored review and the GitHub review wins, since the stored one is written after the post (a newer GitHub review comes from another host).
- **Unknown never counts as changed.** A row from before `head_oid` existed adopts the current head as its baseline (`SetHeadOID`) and is skipped, so rollout causes no burst.
- **Pinning:** reviews post through `PostReviewAtCommit` against the polled head, so a push during a review leaves head ≠ reviewed commit and is reviewed next cycle. GitHub rejects (422, "not part of the pull request") a commit no longer in the PR; that's `github.ErrCommitNotInPR`, returned through `retry.Stop` so it isn't retried, and the PR is reviewed against the new head next cycle.
- `NewCommitCount` counts commits after the reviewed one in `CommitOIDs`; if the reviewed commit isn't there (force-push, rebase, or more than 100 commits since), `Rewritten` is set and the follow-up prompt asks for the full diff. The daemon and the `review` command handle this identically.

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
