# Codebase Concerns

**Analysis Date:** 2026-03-23

## Tech Debt

**`jr yolo history` is an unimplemented stub:**
- Issue: `runYoloHistory` returns `[]` unconditionally; comment says "TODO: filter audit log entries where yolo=true"
- Files: `cmd/yolo.go:78`
- Impact: `jr yolo history` always returns empty, making the command useless as a review tool
- Fix approach: Add `yolo` field to `audit.Entry`, filter `audit.log` JSONL on that field, return matching entries

**`jr avatar act` outputs `"status":"executed"` but never executes actions:**
- Issue: After deciding an action via `DecideRuleBased`, the code sets `record.Status = "executed"` but there is no Jira write. The comment on line 213 reads: "Execution dispatch is a future task; output the decided action only."
- Files: `cmd/avatar_act.go:213-215`
- Impact: The command description promises autonomous reactions; the `"status":"executed"` field in output is misleading — no Jira writes occur
- Fix approach: Dispatch decided actions by calling relevant `workflow` commands (comment, transition, assign) via the client before setting `status = "executed"`

**`jr yolo status` ignores persisted rate-limiter state:**
- Issue: `NewRateLimiter` is called with `stateFile = ""` (empty string), so the rate limiter always starts fresh with full burst tokens. `jr yolo status` always shows `remaining = Burst` regardless of prior activity.
- Files: `cmd/yolo.go:53`
- Impact: `jr yolo status` rate budget is inaccurate for monitoring
- Fix approach: Pass a real `stateFile` path (e.g. from `os.UserConfigDir()`) to `NewRateLimiter`

**Yolo `Config` not stored in the profile struct:**
- Issue: Comment in `runYoloStatus` says "the profile struct does not yet carry a yolo section; a future task will add that field." `DefaultConfig()` values are always used regardless of profile configuration.
- Files: `cmd/yolo.go:41`, `internal/config/config.go`
- Impact: Users cannot configure yolo per profile; all profiles use the same hardcoded defaults
- Fix approach: Add `Yolo *YoloConfig` field to `config.Profile`, load it in `cmd/yolo.go`

**`AnalyzeFieldPreferences` always receives zero-value input:**
- Issue: `Extract()` creates `issueFields` as a slice of zero-value `IssueFields` structs because `CreatedIssue` doesn't carry priority/labels/components. The comment acknowledges this: "AnalyzeFieldPreferences gracefully handles this."
- Files: `internal/avatar/extract.go:166-169`, `internal/avatar/analyze_workflow.go:148`
- Impact: Field preference data in the extracted profile is always empty/zeroed, reducing profile quality
- Fix approach: Extend `FetchUserIssues` in `internal/avatar/fetch.go` to request `priority`, `labels`, `components` fields and populate `IssueFields` accordingly

**`IssueOwner` in `CommentRecord` uses the issue key as an approximation:**
- Issue: `CommentRecord.IssueOwner` is set to the issue key (not the actual issue owner account ID). Comment says "approximate: owner = issue key".
- Files: `internal/avatar/extract.go:175`
- Impact: Response pattern analysis may be inaccurate for interaction profiling
- Fix approach: Fetch the reporter/assignee of each issue in `FetchUserComments` when populating `CommentRecord.IssueOwner`

**Avatar data extraction is capped at 50 issues per query without pagination:**
- Issue: All three `Fetch*` functions in `internal/avatar/fetch.go` hardcode `"maxResults": 50` and do not follow pagination. Users with heavy Jira activity will get truncated extractions regardless of the `min-comments`/`min-updates` targets.
- Files: `internal/avatar/fetch.go:70`, `internal/avatar/fetch.go:126`, `internal/avatar/fetch.go:174`
- Impact: `jr avatar extract` may silently under-collect data, producing weak profiles
- Fix approach: Add pagination loop using `nextPageToken` from the new `/rest/api/3/search/jql` response, consistent with how `doTokenPagination` works in `internal/client/client.go`

**`client.Fetch` does not support retry or auto-pagination:**
- Issue: `client.Fetch` (used by avatar extraction, workflow commands, and batch operations) is a single-shot HTTP request with no retry on 429/5xx and no pagination support — unlike `client.Do` which has both.
- Files: `internal/client/client.go:721`
- Impact: Avatar extraction and workflow commands are not resilient to transient Jira API errors
- Fix approach: Either route `Fetch` callers through `Do`, or add retry logic to `Fetch`

**`Retry-After` header parsing assumes integer seconds, breaks on HTTP-date format:**
- Issue: The code appends `"s"` to the raw `Retry-After` header value and calls `time.ParseDuration`. This works for integer values (e.g. `"30"`) but silently produces zero duration for HTTP-date values (e.g. `"Wed, 21 Oct 2025 07:28:00 GMT"`), which Jira can return per RFC 7231.
- Files: `internal/client/client.go:309`
- Impact: When Jira returns an HTTP-date `Retry-After`, the retry waits zero seconds, potentially burning retries instantly
- Fix approach: Try `strconv.Atoi` first; fall back to `time.Parse(http.TimeFormat, ...)` and compute delta from `time.Now()`

## Known Bugs

**`cmd/avatar_act.go` command help text contradicts actual behavior:**
- Symptoms: The Long description says "Actions are not yet executed" but also says "Use --dry-run to make this explicit without any side effects" — implying non-dry-run mode has side effects. With `record.Status = "executed"` in output, consumers may believe Jira was updated.
- Files: `cmd/avatar_act.go:29-37`
- Trigger: Run `jr avatar act --jql "..."` without `--dry-run`; observe `"status":"executed"` in output with no Jira change
- Workaround: Always use `--dry-run` until execution is implemented

## Security Considerations

**API tokens stored as plaintext in config file:**
- Risk: `config.json` stores `token`, `client_secret`, and `client_id` in plaintext JSON at `~/.config/jr/config.json` (Linux) or `~/Library/Application Support/jr/config.json` (macOS)
- Files: `internal/config/config.go:22-26`, `internal/config/config.go:123-129`
- Current mitigation: File is written with `0o600` permissions
- Recommendations: Document credential manager alternatives; consider integrating with OS keychain (macOS Keychain, Linux Secret Service, Windows DPAPI) for token storage

**`$EDITOR` command executed without validation in two places:**
- Risk: If `$EDITOR` is set to a malicious value, arbitrary code executes when `jr avatar edit` or `jr character edit` is invoked. Both are annotated with `#nosec G204`.
- Files: `cmd/avatar.go:390`, `cmd/character.go:241`
- Current mitigation: `$EDITOR` is a standard Unix convention; `#nosec` annotation is appropriate for the pattern
- Recommendations: No change needed; document that `$EDITOR` should be trusted

**`llm_cmd` shell-splits and executes arbitrary user-configured command:**
- Risk: The `build_llm.go` `BuildLLM` function shell-splits `llmCmd` and executes it with extraction JSON on stdin. This is annotated with `#nosec G204`. If the config file is writable by others, this is a privilege escalation vector.
- Files: `internal/avatar/build_llm.go:45`
- Current mitigation: Config file is `0o600`; `#nosec G204` annotation present
- Recommendations: Ensure config file path is validated to be owner-accessible only; no further action needed for single-user CLI

**OAuth2 `client_secret` is not excluded from env-var override path:**
- Risk: Env vars (`JR_BASE_URL`, `JR_AUTH_TYPE`, `JR_AUTH_USER`, `JR_AUTH_TOKEN`) are supported, but `JR_CLIENT_SECRET` is not, so OAuth2 secrets must live in the config file. There is no env-var escape hatch for CI/CD use of OAuth2.
- Files: `internal/config/config.go:194-235`
- Current mitigation: Not applicable
- Recommendations: Add `JR_CLIENT_SECRET`, `JR_CLIENT_ID`, `JR_TOKEN_URL` env var support for OAuth2 profiles in CI environments

## Performance Bottlenecks

**`cmd/generated/schema_data.go` is a 16,400-line committed file:**
- Problem: A 16,400-line Go source file is compiled on every `go build`. It contains the entire OpenAPI schema embedded as Go data structures.
- Files: `cmd/generated/schema_data.go`
- Cause: Generated from the OpenAPI spec; committed directly into the repo
- Improvement path: Embed the raw OpenAPI YAML/JSON using `go:embed` instead; parse at runtime; or split into smaller generated files per resource group

**HTTP response cache has no size limit or eviction policy:**
- Problem: `cache.Set` writes files to `~/.cache/jr/` without any size enforcement or TTL-based cleanup beyond single-entry stale removal. Heavy usage (many endpoints, many profiles) can grow this unbounded.
- Files: `internal/cache/cache.go`
- Cause: `Dir()` creates the directory once; `Set` writes files without checking total size
- Improvement path: Add periodic sweep of files older than max TTL; or enforce a maximum number of entries

**OAuth2 token cache file is read and written on every request for different profiles:**
- Problem: `getToken` and `setToken` both do a full read + full write of `oauth2_tokens.json` under a mutex. With many concurrent invocations or multiple profiles sharing the file, this is a bottleneck.
- Files: `internal/client/oauth2_cache.go:96-133`
- Cause: No in-process cache layer; every token fetch hits disk
- Improvement path: Add a simple in-memory map keyed by `oauth2CacheKey`; fall back to disk only on miss

## Fragile Areas

**`resolveAvatarDirFromDisk` silently picks the most-recently-modified profile when multiple exist:**
- Files: `cmd/avatar.go:481-530`
- Why fragile: If a user has multiple Jira profiles with avatars, commands like `jr avatar show`, `jr avatar prompt`, and `jr avatar status` that use this function will silently select whichever profile was last modified. There is no warning or flag to select a specific user.
- Safe modification: Always pass `--user` to `jr avatar extract`/`build` to use the `resolveAvatarDir` path instead; add a `--user` flag to local-only subcommands
- Test coverage: The multi-user resolution path is tested in `cmd/avatar_test.go`

**`client.Fetch` body is consumed and not buffered for retries:**
- Files: `internal/client/client.go:721-775`
- Why fragile: `Fetch` reads the passed `body io.Reader` once. If the caller passes a one-shot reader (e.g. from a `strings.NewReader` in a loop), the body is exhausted after the first call. This is fine in current usage but would silently send empty bodies on any retry if retry were ever added.
- Safe modification: Buffer the body bytes before the request loop (as `doOnce` does at line 241-255)
- Test coverage: Not tested for this edge case

**`avatar act` event classification maps all non-`created`/`initial` events to `field_change`:**
- Files: `cmd/avatar_act.go:240-248`
- Why fragile: Any new watch event type (`removed`, future types) will be classified as `field_change`, potentially triggering incorrect reactions in character rules
- Safe modification: Add explicit `case "updated":` and a default `"unknown"` fallback; update `ValidEventType` in `internal/yolo/events.go`
- Test coverage: `TestAvatarActCmd_InvalidReactTo` in `cmd/avatar_act_test.go` covers the filter but not the classification

## Scaling Limits

**Avatar extraction adaptive window loops are bounded by `6m` max and `50` result cap:**
- Current capacity: Collects at most 50 comments, 50 issues, and 50 changelog entries regardless of how large the window gets
- Limit: Users with dense Jira history (>50 items in 6 months) get truncated extractions; users with sparse history (<10 comments across 6 months) get an error
- Scaling path: Implement pagination in `FetchUserComments`, `FetchUserIssues`, `FetchUserChangelog` (see tech debt item above)

## Dependencies at Risk

**`github.com/pb33f/libopenapi v0.34.3` — OpenAPI parsing dependency:**
- Risk: Large, indirect dependency primarily used in `cmd/gendocs` for schema generation. Any breaking changes require regenerating all files in `cmd/generated/`.
- Impact: `make generate` would break; all generated commands could break if the API contract changes
- Migration plan: The generated commands can be regenerated with `make generate`; no migration risk if pinned

**`gopkg.in/yaml.v3` — YAML parsing for avatar and character profiles:**
- Risk: Used for reading/writing user-facing profile files; any deserialization changes could corrupt existing profile files
- Impact: `jr avatar show`, `jr character show` could fail on files created with older schema versions
- Migration plan: Profile struct uses `omitempty` tags; add explicit versioned migration if schema evolves

## Missing Critical Features

**No `jr yolo history` implementation:**
- Problem: `jr yolo history` returns `[]` always. There is no way to review what autonomous actions were taken in the past.
- Blocks: Operational review of `jr avatar act` usage; audit trail for yolo-mode actions

**`jr avatar act` does not execute decided actions:**
- Problem: The autonomous loop only decides and logs what actions would be taken. No Jira writes happen.
- Blocks: The core value proposition of `jr avatar act` — autonomous Jira management — is not delivered

**No `JR_CLIENT_SECRET` / `JR_CLIENT_ID` / `JR_TOKEN_URL` env var support:**
- Problem: OAuth2 credentials cannot be injected via environment variables; they must be in the config file. This blocks CI/CD use of the `oauth2` auth type.
- Blocks: Secure OAuth2 usage in automated pipelines where secrets management injects values via env

## Test Coverage Gaps

**`cmd/avatar_act.go` — actual watch-and-react loop not tested end-to-end:**
- What's not tested: The `watch.Run` integration with the handler callback; the NDJSON stream output under real polling conditions; `--max-actions` stopping behavior with real events
- Files: `cmd/avatar_act_test.go`, `cmd/avatar_act.go`
- Risk: The actual action-dispatch loop (when implemented) could break without detection
- Priority: High (core autonomous feature)

**`internal/avatar/fetch.go` — no pagination test cases:**
- What's not tested: Behavior when Jira returns exactly 50 results (the cap); behavior when fewer than `hardMinComments` are found across the max window
- Files: `internal/avatar/fetch.go`, `internal/avatar/extract.go`
- Risk: Silent data truncation
- Priority: Medium

**Stray coverage artifacts and test binaries not gitignored:**
- What's not tested: N/A — this is a repository hygiene issue
- Files: `coverage_v2.out`, `coverage_v3.out`, `coverage_v4.out`, `coverage_v5.out`, `coverage_v6.out`, `coverage_new.out`, `coverage_new2.out`, `coverage_final.out`, `coverage_final2.out`, `coverage_final3.out`, `coverage_after.out`, `coverage_check.out`, `coverage_current.out`, `coverage_pr70.out`, `cov_all.out`, `cov_final.out`, `cov_now.out`, `cmd_cover.out`, `cmd_cover2.out`, `avatar.test`, `cmd.test`, `gendocs` (binary), `project_schedule.xlsx`
- Risk: Committed accidentally; `.gitignore` only covers `coverage.out` (singular)
- Priority: Low — add `*.out`, `*.test`, `coverage_*.out`, `cov_*.out` patterns to `.gitignore`

---

*Concerns audit: 2026-03-23*
