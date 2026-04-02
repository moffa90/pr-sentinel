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
- **`internal/daemon/poller.go`** — Core orchestrator. `RunPollCycle` has 3 phases: (1) collect work items sequentially, (2) fan out reviews in parallel via semaphore, (3) process outcomes sequentially (publish, record, notify). `RunDaemon` wraps this in a ticker loop with overlap protection
- **`internal/daemon/launchd.go`** — macOS plist generation. The plist invokes `start --daemon-mode` (not `--daemon`). `--daemon` installs+loads the plist; `--daemon-mode` runs the actual loop
- **`internal/reviewer/claude.go`** — Spawns `claude -p` with `--output-format json --json-schema <schema>`. Does NOT pass the diff in the prompt — Claude fetches it itself via `gh pr diff`. Includes heartbeat goroutine (logs every 30s during review)
- **`internal/reviewer/output.go`** — Structured review types (`StructuredReview`, `Finding`, `Verdict`). Parses the claude CLI JSON envelope, extracting `structured_output` field. `FormatMarkdown()` renders findings for GitHub posting
- **`internal/github/client.go`** — GraphQL query via `gh api graphql`. `FetchOpenPRs` returns two lists: new PRs and follow-up candidates (previously reviewed PRs with new commits since last comment). Filters drafts, self-authored PRs, and already-reviewed PRs
- **`internal/state/store.go`** — SQLite at `~/.config/pr-sentinel/state.db`. Tables: `reviewed_prs` (upsert on repo+pr_number), `daily_counts`. Stores full review output for follow-up comparisons
- **`internal/publisher/publisher.go`** — Two modes: `PostLiveReview` (gh pr review) and `SaveDryRunReview` (markdown to `~/.config/pr-sentinel/reviews/`)
- **`internal/notifier/`** — `Dispatcher` fans out to multiple `Notifier` implementations. Per-repo `teams_webhook` overrides global webhook. Teams cards use Adaptive Card format

### Config

YAML at `~/.config/pr-sentinel/config.yaml`. Loaded via `config.Load()`, saved via `config.Save()`. `DefaultConfig()` provides fallback values. Per-repo settings: `mode`, `review_instructions`, `teams_webhook`.

### Review output contract

Claude CLI returns a JSON envelope with `structured_output` containing:
```json
{"verdict": "approve|comment|request-changes", "summary": "...", "findings": [{"severity": "HIGH|MEDIUM|LOW", "file": "...", "line": 42, "message": "..."}]}
```
This is enforced by `--json-schema` flag. If parsing fails, falls back to raw text.

## Conventions

- **Git:** Commits signed by Jose Moffa. Namespace `moffa90/*` uses `<moffa3@gmail.com>`. No Co-Authored-By or Claude Code footers.
- **Logging:** `log/slog` throughout. `slog.Info` for operational events, `slog.Debug` for verbose (enabled by `-v` flag), `slog.Error` for failures.
- **Error wrapping:** `fmt.Errorf("context: %w", err)` pattern.
- **Tests:** Table-driven, `*_test.go` alongside implementation. GitHub client tests use hardcoded JSON responses.
