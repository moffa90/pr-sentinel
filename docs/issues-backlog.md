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
### ~~7. Claude has full shell access during reviews~~ FIXED — `--allowedTools` restricts to read-only + gh/git commands
### ~~8. Config not validated after load~~ FIXED — `Config.Validate()` method added
### ~~9. `time.Parse` errors silently discarded in state store~~ FIXED — errors properly returned

---

## P1 — Fix Soon (reliability, data integrity)

### ~~10. SQLite upsert destroys review history~~ FIXED — UNIQUE constraint dropped, always INSERT
### ~~11. No retry logic — failed post loses completed review~~ FIXED — `retry.Do(3, 2s)` for PostLiveReview
### ~~12. `gh` CLI stderr lost on GraphQL failures~~ FIXED — stderr captured in all gh commands
### ~~13. Semaphore acquire doesn't check context cancellation~~ FIXED — `select` with `ctx.Done()`
### ~~14. File permissions too permissive~~ FIXED — config `0600`, directories `0700`, plist `0600`, state.db `0600`
### ~~15. Webhook URLs leaked in error messages~~ FIXED — `redactURL` helper

---

## P2 — Improve (operational, UX)

### ~~16. Config not reloaded after promote/demote~~ FIXED — hot-reload each poll cycle
### ~~17. No cost tracking or budget enforcement~~ FIXED — `cost_usd` column, `DailyCost()`, shown in status
### ~~18. No health check for daemon~~ FIXED — `health.json` written each cycle
### ~~19. No GitHub rate limit awareness~~ FIXED — `rateLimit` in GraphQL query, debug logging, warn at <20%
### ~~20. `errors.Is` not used for context error comparison~~ FIXED
### ~~21. State DB path duplicated across 3 commands~~ FIXED — `state.DefaultDBPath()`
### ~~22. `launchd.go` hardcodes config dir path~~ FIXED — uses `config.ConfigDir()`

---

## P3 — Nice to Have (architecture, future-proofing)

### ~~23. No test coverage on RunPollCycle / RunDaemon~~ FIXED — `poll_cycle_test.go` with 6+ tests

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

### ~~27. Dead code cleanup~~ FIXED
### ~~28. `running` flag in RunDaemon is dead code~~ FIXED — removed

---

## Summary

| Priority | Total | Fixed | Open |
|----------|-------|-------|------|
| P0       | 9     | 9     | 0    |
| P1       | 6     | 6     | 0    |
| P2       | 7     | 7     | 0    |
| P3       | 6     | 4     | 2    |

All P0, P1, and P2 items resolved. Remaining P3 items (#24, #25, #26) are architectural changes for future consideration.
