# External Integrations

**Analysis Date:** 2026-03-23

## APIs & External Services

**Atlassian Jira Cloud (primary):**
- Service: Jira Cloud REST API v3
- Spec source: `https://dac-static.atlassian.com/cloud/jira/platform/swagger-v3.v3.json`
- Local spec: `spec/jira-v3.json` and `spec/jira-v3-latest.json`
- Client: custom HTTP client in `internal/client/client.go` (stdlib `net/http`, no SDK)
- All generated commands in `cmd/generated/` are auto-generated from this spec via `gen/`
- Base URL config: `JR_BASE_URL` env var or `base_url` in config profile
- Auth: `JR_AUTH_TOKEN` / `JR_AUTH_USER` (see Auth section below)

**External LLM (optional, avatar feature):**
- Service: any LLM CLI tool configured by the user
- Integration: shell-out via `os/exec` in `internal/avatar/build_llm.go`
- Config: `avatar.llm_cmd` in profile (e.g. `"claude -p"`, `"ollama run llama3"`)
- Protocol: extraction JSON piped to stdin; expects YAML profile on stdout
- Timeout: 5 minutes (`llmTimeout` in `internal/avatar/build_llm.go`)
- Not required — `jr avatar build` falls back to local heuristics if no LLM configured

## Data Storage

**Databases:**
- None — no database dependency

**File Storage (local filesystem only):**
- Config file: OS-specific path under user config dir (see `internal/config/config.go`)
- HTTP response cache: `os.UserCacheDir()/jr/` — SHA256-keyed files (`internal/cache/cache.go`)
- OAuth2 token cache: `os.UserCacheDir()/jr/oauth2/` — keyed by tokenURL+clientID hash (`internal/client/oauth2_cache.go`)
- Audit log: `os.UserConfigDir()/jr/audit.log` — JSONL format (`internal/audit/audit.go`)
- Avatar extraction: `os.UserConfigDir()/jr/avatars/<user-hash>/extraction.json`
- Avatar profile: `os.UserConfigDir()/jr/avatars/<user-hash>/profile.yaml`
- Character definitions: `os.UserConfigDir()/jr/characters/<name>.yaml` (`internal/character/storage.go`)
- Yolo state/history: `os.UserConfigDir()/jr/yolo/` (`internal/yolo/`)
- Template definitions: user config dir (see `internal/template/`)

**Caching:**
- File-based HTTP response cache with TTL (`internal/cache/cache.go`)
- Per-request TTL via `--cache <duration>` flag
- Automatic metadata TTL for known low-churn Jira endpoints (e.g. `/rest/api/3/project`)
- Cache keys: SHA256(method + URL + auth context) — auth-scoped to prevent cross-profile leakage

## Authentication & Identity

**Auth Providers (all Jira-targeted):**

**Basic Auth (default):**
- Jira API token as password
- Config: `auth.type = "basic"`, `auth.username`, `auth.token`
- Env: `JR_AUTH_TYPE=basic`, `JR_AUTH_USER`, `JR_AUTH_TOKEN`
- `req.SetBasicAuth(username, token)` in `internal/client/client.go`

**Bearer Token:**
- Config: `auth.type = "bearer"`, `auth.token`
- Sets `Authorization: Bearer <token>` header

**OAuth2 Client Credentials:**
- Config: `auth.type = "oauth2"`, `auth.client_id`, `auth.client_secret`, `auth.token_url`, `auth.scopes`
- Grant type: `client_credentials`
- Token fetched in `internal/client/client.go` (`fetchOAuth2Token`)
- Tokens cached in file-based OAuth2 cache to avoid repeated token requests

## Monitoring & Observability

**Error Tracking:**
- None — no external error tracking service

**Logs:**
- Verbose HTTP logging: `--verbose` flag writes request/response JSON to stderr
- Audit logging: JSONL file at `os.UserConfigDir()/jr/audit.log` — enabled per-profile (`audit_log: true`) or per-invocation (`--audit`, `--audit-file`)
- Audit entry structure defined in `internal/audit/audit.go`

**Coverage:**
- Codecov — CI uploads `coverage.out` via `codecov/codecov-action` in `.github/workflows/ci.yml`
- Token: `CODECOV_TOKEN` GitHub secret

## CI/CD & Deployment

**Hosting:**
- GitHub: https://github.com/sofq/jira-cli
- Container registry: GitHub Container Registry (`ghcr.io`) for Docker images
- Docs: GitHub Pages (`sofq.github.io/jira-cli`) via VitePress build

**CI Pipeline (GitHub Actions):**
- `ci.yml` — runs on push/PR to main: build, unit tests, lint (golangci-lint), npm/PyPI smoke tests, docs build, integration tests (main only)
- `release.yml` — triggered on `v*` tags: GoReleaser cross-platform builds, npm publish, PyPI publish
- `docs.yml` — deploys documentation site
- `security.yml` — security scanning
- `spec-drift.yml` — detects drift between local and upstream Jira OpenAPI spec
- `dependabot.yml` — automated dependency updates

**Package Distribution:**
- npm: `jira-jr` on npmjs.com — binary wrapper that downloads platform binary on install (`npm/install.js`)
- PyPI: `jira-jr` on PyPI — binary wrapper (`python/`)
- Homebrew: `sofq/homebrew-tap` — formula auto-published via GoReleaser
- Scoop: `sofq/scoop-bucket` — manifest auto-published via GoReleaser
- GitHub Releases: platform tarballs with checksums via GoReleaser

## Environment Configuration

**Required env vars (or config file equivalent):**
- `JR_BASE_URL` — Jira instance base URL (e.g. `https://yoursite.atlassian.net`)
- `JR_AUTH_TOKEN` — Jira API token
- `JR_AUTH_USER` — Jira username/email (basic auth)

**Optional env vars:**
- `JR_AUTH_TYPE` — auth type (`basic`, `bearer`, `oauth2`); default `basic`
- `JR_CONFIG_PATH` — override config file path

**CI secrets (GitHub Actions):**
- `CODECOV_TOKEN` — Codecov upload token
- `JR_BASE_URL`, `JR_AUTH_USER`, `JR_AUTH_TOKEN` — live Jira instance for integration tests
- `GITHUB_TOKEN` — GoReleaser release and GHCR push
- `HOMEBREW_TAP_TOKEN` — push to Homebrew tap repo

## Webhooks & Callbacks

**Incoming:**
- None — `jr` is a CLI tool, not a server; no HTTP listener

**Outgoing:**
- All outbound HTTP calls go to the configured Jira instance (`base_url`)
- OAuth2 token endpoint calls go to the configured `token_url`
- Avatar LLM calls are local shell-outs, not HTTP

## Atlassian Document Format (ADF)

The `internal/adf/` package provides a local ADF builder — no external Atlassian SDK is used. Plain-text inputs (e.g. `--text` for comments) are auto-converted to ADF JSON via `adf.FromText()` before being sent to Jira's API.

---

*Integration audit: 2026-03-23*
