# pr-sentinel — Issues Backlog

Generated from 4-agent code review (2026-04-02). Items already fixed are marked.

---

## P0 — Fix Now (correctness/security, breaks things)

### ~~1. AppleScript command injection~~ FIXED
### ~~2. Deadlock on MaxParallelReviews=0~~ FIXED
### ~~3. Repo path re-derived instead of using repo.Path~~ FIXED
### ~~4. Follow-up PRs used wrong prompt~~ FIXED
### ~~5. Status command hardcoded "not running"~~ FIXED
### ~~6. Webhook client had no HTTP timeout~~ FIXED

### 7. Claude has full shell access during reviews
**Risk:** A malicious PR that modifies `.claude/commands/` or `CLAUDE.md` in the repo can inject instructions Claude will execute. PR descriptions can contain prompt injection.
**Solution:** Pass `--allowedTools` to restrict Claude to read-only operations during reviews. Possible values: `Read`, `Grep`, `Glob`, `Bash(gh pr diff)`. Investigate if Claude CLI supports an allowlist. If not, open a feature request. Short-term mitigation: add `--append-system-prompt "Do not execute any commands other than gh pr diff. Do not modify any files."`.
**Effort:** Small (flag addition) or Medium (if needs research)

### 8. Config not validated after load
**Risk:** `PollInterval <= 0` panics `time.NewTicker`. `MaxParallelReviews = 0` was deadlock (fixed). Empty `Repos[].Name` causes silent failures.
**Solution:** Add `Config.Validate() error` method. Call after `Load()`. Check: `PollInterval > 0`, `MaxReviewsPerCycle > 0`, `MaxReviewsPerDay > 0`, `MaxParallelReviews > 0`, `ReviewTimeout > 0`, `ReposDir` not empty, each `Repos[].Name` matches `owner/repo` pattern.
**Effort:** Small — one function, one call site

### 9. `time.Parse` errors silently discarded in state store
**Risk:** Corrupted timestamps return zero-value `time.Time` with no error signal. Follow-up review detection depends on timestamps.
**Solution:** Return the parse error from `GetReview()` and `RecentReviews()` instead of discarding with `_`.
**Effort:** Small

---

## P1 — Fix Soon (reliability, data integrity)

### 10. SQLite upsert destroys review history
**Risk:** Follow-up reviews overwrite the original review. Can't compare review iterations. Dry-run files also overwritten.
**Solution A (minimal):** Add a `review_version` column, change UNIQUE to `(repo, pr_number, review_version)`. Auto-increment version per (repo, pr_number).
**Solution B (simple):** Keep current upsert for "latest" tracking but add a `review_history` table that appends every review. Append-only, never delete.
**Solution C (file naming):** Include timestamp in dry-run filenames: `owner-repo-42-2026-04-02T14-30.md`.
**Effort:** Medium

### 11. No retry logic — failed post loses completed review
**Risk:** A review that took 5+ minutes of Claude compute fails to post to GitHub due to transient 502. Work is lost, PR may never be re-reviewed (for new PRs, `HasReviewed` returns false so next cycle retries — OK. For follow-ups, the GraphQL check may no longer detect new commits if author didn't push again).
**Solution:** Add retry with exponential backoff (3 attempts, 2s/4s/8s) for `PostLiveReview` and `postJSON`. Use a simple `retry(n, fn)` helper. Don't overcomplicate.
**Effort:** Small

### 12. `gh` CLI stderr lost on GraphQL failures
**Risk:** GitHub returns helpful error messages in stderr (rate limit, auth expired, repo not found) but `cmd.Output()` discards them. Errors show as generic "gh api graphql failed: exit status 1".
**Solution:** Switch to `cmd.CombinedOutput()` or capture stderr separately (like reviewer already does). Include stderr in error message.
**Effort:** Small

### 13. Semaphore acquire doesn't check context cancellation
**Risk:** On Ctrl+C, goroutines waiting for the semaphore (`sem <- struct{}{}`) block indefinitely because no one is releasing slots.
**Solution:** Use a select:
```go
select {
case sem <- struct{}{}:
case <-ctx.Done():
    return
}
```
**Effort:** Small

### 14. File permissions too permissive
**Risk:** Config with webhook secrets is `0644` (world-readable). SQLite state DB inherits directory permissions.
**Solution:** Use `0600` for config file, `0700` for config directory, `0600` for plist. Apply in `config.Save()`, `state.Open()`, `launchd.InstallPlist()`.
**Effort:** Small

### 15. Webhook URLs leaked in error messages
**Risk:** Slack/Teams webhook URLs contain embedded auth tokens. Error messages include full URL, visible in daemon logs.
**Solution:** Create a `redactURL(url)` helper that shows only the hostname: `"webhook https://hooks.slack.com/... returned status 403"`. Use in all error messages in `webhook.go`.
**Effort:** Small

---

## P2 — Improve (operational, UX)

### 16. Config not reloaded after promote/demote
**Risk:** User runs `pr-sentinel promote repo`, expects live reviews. Daemon keeps old config snapshot. Confusing UX.
**Solution A (simple):** Reload config at the start of each poll cycle. It's just a YAML file read — cheap.
**Solution B:** Add SIGHUP handler that triggers config reload. `promote`/`demote` send SIGHUP to daemon PID.
**Solution C (minimal):** `promote`/`demote` print a warning: "Restart the daemon for changes to take effect."
**Effort:** Small (A or C), Medium (B)

### 17. No cost tracking or budget enforcement
**Risk:** Each review costs real money. No visibility into spend. No way to cap daily costs.
**Solution:** Log `CostUSD` from every review (it's already in the Claude envelope). Store cumulative daily cost in state DB. Add `max_cost_per_day` config option. Surface in `pr-sentinel status`. Alert when approaching limit.
**Effort:** Medium

### 18. No health check for daemon
**Risk:** Daemon could be stuck (hung Claude process, SQLite lock) with no way to detect.
**Solution:** Write `~/.config/pr-sentinel/health.json` at the end of each poll cycle with `{last_poll: timestamp, cycle_count: N, errors: N}`. `pr-sentinel status` reads this file and warns if last poll is older than 2x poll interval.
**Effort:** Small

### 19. No GitHub rate limit awareness
**Risk:** 5+ repos polling every 10 minutes consumes GraphQL API budget. No backoff when approaching limits. Silent 403 errors.
**Solution:** After each `gh api graphql` call, check rate limit headers. Log remaining budget at debug level. If below 20% remaining, increase poll interval temporarily. Could also use `gh api rate_limit` once per cycle.
**Effort:** Medium

### 20. `errors.Is` not used for context error comparison
**Risk:** `err == context.Canceled` is fragile if errors are wrapped in the future.
**Solution:** Replace all `== context.Canceled` and `== context.DeadlineExceeded` with `errors.Is()`. Locations: `commands/start.go:107`, `reviewer/claude.go:162,168`.
**Effort:** Small

### 21. State DB path duplicated across 3 commands
**Risk:** If filename changes, must update in 3 places.
**Solution:** Add `state.DefaultDBPath()` function (mirrors `config.DefaultConfigPath()`). Call from `start.go`, `status.go`, `logs.go`.
**Effort:** Small

### 22. `launchd.go` hardcodes config dir path
**Risk:** Diverges from `config.ConfigDir()` if it ever changes (e.g., XDG support).
**Solution:** Replace `filepath.Join(home, ".config", "pr-sentinel")` with `config.ConfigDir()` in `launchd.go:72`.
**Effort:** Small

---

## P3 — Nice to Have (architecture, future-proofing)

### 23. No test coverage on RunPollCycle / RunDaemon
**Blocker:** Functions are tightly coupled to `exec.Command` and package-level GitHub functions.
**Solution:** Define interfaces for GitHub client, reviewer, publisher, state store. Inject into `RunPollCycle`. Write integration tests with mock implementations. This is a larger refactor.
**Effort:** Large

### 24. macOS-only daemon with no Linux support
**Solution:** Add `//go:build darwin` to `launchd.go`. Create `systemd.go` with `//go:build linux`. Add runtime OS check in `start -d`.
**Effort:** Medium

### 25. Polling instead of webhooks
**Tradeoff:** Polling avoids needing a public endpoint (good for local CLI). But wastes API budget and adds latency.
**Solution:** Keep polling as default. Add optional webhook receiver mode for users who can expose an endpoint (via Cloudflare Tunnel, ngrok, etc.). This is a significant feature.
**Effort:** Large

### 26. Subprocess per review instead of API direct
**Tradeoff:** Claude CLI gives full `.claude/` context (CLAUDE.md, memory, skills). Direct API loses this.
**Solution:** Keep CLI approach but investigate `claude-agent-sdk` for Go. Could provide same context with better control over tokens, streaming, and cost.
**Effort:** Large (research + rewrite)

### 27. Dead code cleanup
- `ui.SeverityHigh/Medium/Low` — exported but unused
- `ui.SpinnerModel/SpinnerDoneMsg` — exported but unused
- `notifier_test.go:contains()` helper — reimplements `strings.Contains`
- `teams.go:statusColor` — computed then discarded
**Effort:** Small

### 28. `running` flag in RunDaemon is dead code
The synchronous select loop means `running` can never be true when the next tick fires. Either remove it or make `RunPollCycle` async with proper `atomic.Bool`.
**Effort:** Small (remove) or Medium (make async)

---

## Summary

| Priority | Count | Key Theme |
|----------|-------|-----------|
| P0 (fixed) | 6 | Security, correctness — all done |
| P0 (open) | 3 | Shell access, config validation, silent errors |
| P1 | 6 | Reliability — retries, permissions, data integrity |
| P2 | 7 | Operations — cost tracking, health, config reload |
| P3 | 6 | Architecture — testing, cross-platform, webhooks |

**Suggested next sprint (highest impact, lowest effort):**
1. #8 Config.Validate() — prevents panics/deadlocks
2. #9 Fix time.Parse errors — prevents silent data corruption
3. #11 Retry for PostLiveReview — prevents lost reviews
4. #13 Semaphore context check — prevents stuck goroutines on shutdown
5. #14 File permissions — security hardening
6. #15 Redact webhook URLs — prevents token leakage
7. #20 errors.Is — future-proof error handling
8. #21 + #22 DRY path helpers — prevents divergence
