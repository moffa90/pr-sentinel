# pr-sentinel

Automated PR review powered by Claude Code. Watches your GitHub repos for new pull requests and runs reviews locally, preserving your full `.claude/` context.

## Why

Your CLAUDE.md files, memory, and skills define how you review code. pr-sentinel runs `claude -p` inside each repo directory so your full context is preserved — not a generic AI review, but *your* AI review.

## Features

- **Auto-detect repos** — Scans a directory, finds GitHub repos, lets you pick which to track
- **Local Claude Code reviews** — Runs inside each repo for full `.claude/` context
- **Dry-run by default** — New repos start in dry-run mode; reviews are saved locally until you promote to live
- **Post to GitHub** — Live mode posts reviews as PR comments, tags the author
- **Configurable notifications** — macOS, Slack, Teams, generic webhooks
- **Rate limiting** — Per-cycle and daily review caps
- **launchd daemon** — Background service on macOS with auto-start

## Prerequisites

- [Claude Code](https://docs.anthropic.com/en/docs/claude-code/overview) — installed and authenticated
- [GitHub CLI](https://cli.github.com/) — installed and authenticated (`gh auth login`)

## Quick Start

```bash
# Install
go install github.com/moffa90/pr-sentinel/cmd/pr-sentinel@latest

# Initialize — scans ~/Git for repos, generates config
pr-sentinel init ~/Git

# Start watching (foreground)
pr-sentinel start

# Or run as a background daemon
pr-sentinel start -d
```

## Commands

| Command | Description |
|---------|-------------|
| `init [path]` | Scan directory for repos and generate config |
| `start` | Start polling (foreground) |
| `start -d` | Start as background daemon (launchd) |
| `stop` | Stop daemon |
| `status` | Show tracked repos and review stats |
| `review <pr-url>` | One-off review of a specific PR |
| `repos` | List tracked repositories |
| `promote <repo>` | Move repo to live mode (posts reviews) |
| `demote <repo>` | Move repo to dry-run mode (saves locally) |
| `logs` | Show recent review activity |
| `version` | Show version |

## Configuration

Config lives at `~/.config/pr-sentinel/config.yaml`:

```yaml
poll_interval: 10m
max_reviews_per_cycle: 5
max_reviews_per_day: 20
repos_dir: ~/Git
review_timeout: 5m

github_user: ""  # Auto-detected from gh auth status

review:
  instructions: ""       # Extra review instructions for Claude
  ai_disclosure: true    # Prefix reviews with AI disclaimer
  disclosure_text: "> AI-assisted review by [pr-sentinel](https://github.com/moffa90/pr-sentinel)"

notifications:
  macos: true
  log: true
  slack:
    enabled: false
    webhook_url: ""
  teams:
    enabled: false
    webhook_url: ""
  webhook:
    enabled: false
    url: ""

repos:
  - name: owner/repo
    path: ~/Git/repo
    mode: dry-run         # dry-run | live
    review_instructions: ""  # Per-repo override
```

## Review Context (Layered)

pr-sentinel runs `claude -p` inside each repo directory, so context is layered automatically:

1. **Repo's `CLAUDE.md`** — auto-loaded by Claude Code
2. **Your `~/.claude/CLAUDE.md`** — auto-loaded by Claude Code
3. **Your `.claude/` memory + skills** — auto-loaded by Claude Code
4. **`review.instructions`** from config — appended via `--append-system-prompt`
5. **Per-repo `review_instructions`** — appended after global

Users with a rich `.claude/` setup get full context automatically. Users without `.claude/` can still configure review behavior via the config file.

## How It Works

```
┌─────────────────────────────────────────────────┐
│                   pr-sentinel                    │
│                                                  │
│  ┌───────────┐  ┌────────────┐  ┌────────────┐ │
│  │  Poller   │→ │  Reviewer  │→ │ Publisher  │ │
│  │ (gh API)  │  │ (claude -p)│  │ (gh pr)   │ │
│  └───────────┘  └────────────┘  └────────────┘ │
│       ↕               ↕              ↕         │
│  ┌─────────────────────────────────────────────┐│
│  │            State Store (SQLite)             ││
│  └─────────────────────────────────────────────┘│
│       ↕                                         │
│  ┌─────────────────────────────────────────────┐│
│  │     Notifier (macOS, Slack, Teams, hook)    ││
│  └─────────────────────────────────────────────┘│
└─────────────────────────────────────────────────┘
```

1. **Poller** — queries GitHub GraphQL API for open, non-draft PRs you haven't reviewed
2. **Reviewer** — `cd`s into the repo and runs `claude -p` with your full `.claude/` context
3. **Publisher** — posts to GitHub (live) or saves locally (dry-run)
4. **Notifier** — alerts you via macOS, Slack, Teams, or webhook

## Daemon Management

```bash
# Start as background service (survives terminal close)
pr-sentinel start -d

# Check status
pr-sentinel status

# Stop
pr-sentinel stop

# View logs
pr-sentinel logs
```

The daemon uses macOS launchd (`~/Library/LaunchAgents/com.moffa90.pr-sentinel.plist`) and auto-starts on login.

## License

MIT
