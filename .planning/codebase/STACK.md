# Technology Stack

**Analysis Date:** 2026-03-23

## Languages

**Primary:**
- Go 1.25.8 - All CLI logic, HTTP client, code generation, internal packages

**Secondary:**
- JavaScript (Node.js) - npm distribution wrapper (`npm/install.js`, `npm/package.json`)
- Python 3.8+ - PyPI distribution wrapper (`python/pyproject.toml`, `python/jira_jr/`)

## Runtime

**Environment:**
- Go runtime (compiled binary, no runtime dependency)
- Binary name: `jr`

**Package Manager:**
- Go modules (`go.mod` / `go.sum`) — lockfile present
- npm (`npm/package.json`) — for npm distribution only
- pip / setuptools (`python/pyproject.toml`) — for PyPI distribution only

## Frameworks

**Core:**
- `github.com/spf13/cobra v1.10.2` — CLI command framework; all commands defined with `cobra.Command` in `cmd/`
- `github.com/spf13/pflag v1.0.9` — flag parsing (cobra dependency)

**Testing:**
- Go standard `testing` package — no third-party test framework
- `go test ./...` — runs all unit and integration tests

**Build/Dev:**
- `make build` — runs `go generate ./...` then `go build -ldflags "-s -w -X ..." -o jr .`
- `make generate` — regenerates `cmd/generated/*.go` from OpenAPI spec via `go run ./gen/...`
- `make lint` — `golangci-lint run`
- GoReleaser v2 — cross-platform release builds (`.goreleaser.yml`)
- VitePress `^1.6.4` — documentation site (`website/`)

## Key Dependencies

**Critical:**
- `github.com/itchyny/gojq v0.12.18` — pure-Go jq implementation; powers `--jq` flag for all output filtering (`internal/jq/`)
- `github.com/pb33f/libopenapi v0.34.3` — OpenAPI spec parsing; used in code generator (`gen/`) to produce `cmd/generated/*.go` from Jira's OpenAPI spec
- `github.com/spf13/cobra v1.10.2` — entire CLI structure depends on this
- `github.com/tidwall/pretty v1.2.1` — JSON pretty-printing for `--pretty` flag

**Infrastructure:**
- `golang.org/x/sync v0.20.0` — used in batch/parallel execution (`cmd/batch.go`)
- `gopkg.in/yaml.v3 v3.0.1` — YAML marshaling for avatar profiles and character configs
- `go.yaml.in/yaml/v4 v4.0.0-rc.4` — indirect, via libopenapi

## Configuration

**Runtime config file:**
- JSON config stored at OS-specific path:
  - macOS: `~/Library/Application Support/jr/config.json`
  - Linux: `~/.config/jr/config.json`
  - Windows: `%APPDATA%\jr\config.json`
- Override with `JR_CONFIG_PATH` env var
- Config structure defined in `internal/config/config.go`

**Environment variables (override config file):**
- `JR_BASE_URL` — Jira instance base URL
- `JR_AUTH_TYPE` — auth type: `basic`, `bearer`, or `oauth2`
- `JR_AUTH_USER` — username for basic auth
- `JR_AUTH_TOKEN` — API token or password
- `JR_CONFIG_PATH` — path to config file

**Build config:**
- `Makefile` — all build targets
- `.goreleaser.yml` — release configuration (cross-platform binaries, Homebrew tap, Scoop bucket)
- `Dockerfile` and `Dockerfile.goreleaser` — distroless container image builds
- `go.mod` — module path `github.com/sofq/jira-cli`

## Platform Requirements

**Development:**
- Go 1.25.8+
- `golangci-lint` for linting
- Node.js 24 (npm smoke tests / docs site only)
- Python 3.12 (PyPI smoke tests only)

**Production:**
- Single static binary — no runtime dependencies
- Supports: linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64, windows/arm64
- Docker: `gcr.io/distroless/static:nonroot` base image

---

*Stack analysis: 2026-03-23*
