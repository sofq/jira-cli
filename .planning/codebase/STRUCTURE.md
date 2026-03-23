# Codebase Structure

**Analysis Date:** 2026-03-23

## Directory Layout

```
jira-cli-v2/
├── main.go                   # Entry point -- delegates to cmd.Execute()
├── go.mod                    # Module: github.com/sofq/jira-cli
├── go.sum
├── Makefile                  # build, test, lint, generate targets
├── CLAUDE.md                 # Agent-facing usage reference
├── README.md
├── Dockerfile
├── .goreleaser.yml           # Release builds (linux/mac/windows, npm, python)
├── .golangci.yml             # Linter config
├── e2e_test.go               # Root-level e2e tests (live Jira instance)
│
├── cmd/                      # All CLI commands (package cmd)
│   ├── root.go               # Root cobra command + PersistentPreRunE middleware
│   ├── configure.go          # jr configure
│   ├── raw.go                # jr raw <METHOD> <path>
│   ├── workflow.go           # jr workflow (transition, assign, comment, move, ...)
│   ├── watch.go              # jr watch
│   ├── diff.go               # jr diff
│   ├── batch.go              # jr batch
│   ├── pipe.go               # jr pipe
│   ├── context_cmd.go        # jr context <key>
│   ├── avatar.go             # jr avatar (extract, build, prompt, show, ...)
│   ├── avatar_act.go         # jr avatar act (autonomous loop)
│   ├── character.go          # jr character
│   ├── yolo.go               # jr yolo status/history
│   ├── template.go           # jr template
│   ├── preset.go             # jr preset list
│   ├── schema_cmd.go         # jr schema
│   ├── doctor.go             # jr doctor
│   ├── explain.go            # jr explain
│   ├── version.go            # jr version
│   ├── *_test.go             # Unit/integration tests co-located with commands
│   └── generated/            # AUTO-GENERATED -- do not edit manually
│       ├── init.go           # RegisterAll() -- adds all resource commands to root
│       ├── issue.go          # jr issue get/create/edit/delete/...
│       ├── search.go         # jr search search-and-reconsile-issues-using-jql
│       ├── project.go        # jr project search/get/...
│       ├── workflow.go       # jr workflow (generated subcommands)
│       ├── avatar.go         # jr avatar (generated subcommands -- merged with hand-written)
│       └── *.go              # One file per Jira API resource
│
├── gen/                      # Code generator (separate binary, NOT part of jr)
│   ├── main.go               # Entry point: spec -> cmd/generated/
│   ├── parser.go             # OpenAPI spec parser (github.com/pb33f/libopenapi)
│   ├── grouper.go            # Groups operations by resource name
│   ├── generator.go          # Renders Go files from templates
│   └── templates/
│       ├── init.go.tmpl      # Template for generated/init.go
│       ├── resource.go.tmpl  # Template for per-resource command files
│       └── schema_data.go.tmpl
│
├── spec/
│   ├── jira-v3.json          # Jira Cloud REST API v3 OpenAPI spec (input to gen/)
│   └── jira-v3-latest.json
│
├── internal/                 # Private packages -- not importable outside module
│   ├── client/
│   │   ├── client.go         # Client struct: Do(), Fetch(), WriteOutput(), pagination, auth
│   │   ├── oauth2_cache.go   # File-based OAuth2 token cache
│   │   └── export_test.go
│   ├── config/
│   │   └── config.go         # Config types, LoadFrom(), SaveTo(), Resolve()
│   ├── errors/
│   │   └── errors.go         # APIError, AlreadyWrittenError, exit codes 0-7
│   ├── policy/
│   │   └── policy.go         # Operation access control (glob allow/deny)
│   ├── audit/
│   │   └── audit.go          # JSONL audit logger
│   ├── cache/
│   │   └── cache.go          # Disk-based GET response cache
│   ├── jq/
│   │   └── jq.go             # jq filter runner (github.com/itchyny/gojq)
│   ├── preset/
│   │   └── preset.go         # Named output presets (agent, detail, triage, board)
│   ├── format/
│   │   └── format.go         # Table/CSV formatter for --format flag
│   ├── adf/
│   │   └── adf.go            # Plain text -> Atlassian Document Format converter
│   ├── retry/
│   │   └── retry.go          # Exponential backoff retry for 429/5xx
│   ├── duration/
│   │   └── duration.go       # Human-friendly duration parser ("2h 30m" -> seconds)
│   ├── changelog/
│   │   └── changelog.go      # Issue changelog parser/formatter
│   ├── watch/
│   │   └── watcher.go        # JQL polling loop, NDJSON event emission
│   ├── template/
│   │   ├── template.go       # Issue template engine (load, apply, validate)
│   │   └── builtin/          # Built-in YAML templates
│   │       ├── bug-report.yaml
│   │       ├── story.yaml
│   │       ├── task.yaml
│   │       ├── subtask.yaml
│   │       ├── epic.yaml
│   │       └── spike.yaml
│   ├── avatar/               # User profiling / persona building
│   │   ├── types.go          # Extraction, Profile, RawComment, etc.
│   │   ├── extract.go        # Jira activity extraction pipeline
│   │   ├── fetch.go          # Jira API fetching helpers for extraction
│   │   ├── build.go          # Profile building dispatcher (local vs LLM)
│   │   ├── build_local.go    # Local statistical profile builder
│   │   ├── build_llm.go      # LLM-based profile builder
│   │   ├── analyze_writing.go
│   │   ├── analyze_workflow.go
│   │   ├── analyze_interaction.go
│   │   ├── select_examples.go
│   │   ├── storage.go        # Save/load extraction + profile to ~/.config/jr/avatars/
│   │   ├── prompt.go         # FormatPrompt() -- renders profile for agent consumption
│   │   └── prompts/          # Prompt templates for LLM engine
│   ├── character/            # Reusable persona management
│   │   ├── types.go          # Character, StyleGuide, Reaction, ComposedCharacter
│   │   ├── storage.go        # Save/load/list character YAML files
│   │   ├── active.go         # Get/set active character
│   │   ├── compose.go        # Merge base + section overrides into ComposedCharacter
│   │   ├── convert.go        # avatar.Profile -> character.Character
│   │   ├── prompt.go         # Format character for agent consumption
│   │   ├── templates.go      # Built-in character templates (concise, formal-pm, ...)
│   │   └── validate.go       # Character validation
│   └── yolo/                 # Autonomous execution policy
│       ├── config.go         # Config, RateLimitConfig, scope tier ops
│       ├── policy.go         # Yolo policy enforcement
│       ├── events.go         # Event type definitions
│       ├── decide.go         # Decision engine (rules or LLM)
│       └── ratelimit.go      # Token bucket rate limiter
│
├── test/
│   ├── e2e/
│   │   └── e2e_test.go       # End-to-end tests (require live Jira)
│   └── integration/
│       └── integration_test.go
│
├── skill/
│   └── jira-cli/             # Agent skill definition for consuming this CLI
│
├── npm/                      # npm wrapper package for distribution
├── python/
│   └── jira_jr/              # Python wrapper package for distribution
├── website/                  # VitePress documentation site
│   ├── .vitepress/
│   ├── guide/
│   └── commands/
└── docs/
    └── superpowers/          # Internal planning docs
```

## Directory Purposes

**`cmd/`:**
- Purpose: All cobra command definitions and their `RunE` handlers
- Contains: One `.go` file per logical command group, plus co-located `_test.go` files
- Key files: `cmd/root.go` (middleware), `cmd/workflow.go` (high-level workflow helpers), `cmd/avatar_act.go` (autonomous loop)

**`cmd/generated/`:**
- Purpose: Auto-generated cobra commands from the Jira OpenAPI spec
- Contains: One `.go` file per Jira API resource (issue, project, search, etc.)
- Generated: Yes -- run `make generate` to regenerate
- Committed: Yes -- consumers get working code without running the generator

**`gen/`:**
- Purpose: Standalone Go binary that reads the OpenAPI spec and writes `cmd/generated/`
- Contains: Parser, grouper, generator, and Go text/template files
- Generated: No -- this is the generator source
- Key files: `gen/main.go`, `gen/parser.go`, `gen/generator.go`, `gen/templates/resource.go.tmpl`

**`spec/`:**
- Purpose: Jira Cloud REST API OpenAPI spec files
- Contains: `jira-v3.json` (stable), `jira-v3-latest.json` (latest)
- Generated: No -- obtained from Jira/Atlassian

**`internal/`:**
- Purpose: Private library packages used by the `cmd` layer
- Contains: One package per concern; no package imports another `cmd/` package
- Key packages: `client`, `config`, `errors`, `policy`, `avatar`, `character`, `yolo`

**`internal/avatar/`:**
- Purpose: Two-phase user profiling: extraction (Jira API -> raw stats) and build (stats -> prose profile)
- Key files: `types.go` (all struct definitions), `extract.go`, `build_local.go`, `storage.go`

**`internal/character/`:**
- Purpose: Named persona documents that carry style guidance and reaction rules for the autonomous agent
- Key files: `types.go`, `storage.go`, `active.go`, `compose.go`

**`internal/yolo/`:**
- Purpose: Autonomous execution gating -- rate limits, scope tiers, decision engine
- Key files: `config.go`, `policy.go`, `decide.go`

**`test/`:**
- Purpose: Tests that require external dependencies (live Jira or test server)
- Contains: E2e and integration test files separate from unit tests

## Key File Locations

**Entry Points:**
- `main.go`: Binary entry point
- `cmd/root.go`: Root cobra command and all middleware logic
- `cmd/generated/init.go`: `RegisterAll()` -- registers every generated command

**Configuration:**
- `internal/config/config.go`: Config types and `Resolve()` function
- `~/.config/jr/config.json`: Runtime config file (platform-specific path from `config.DefaultPath()`)
- `.golangci.yml`: Linter rules
- `.goreleaser.yml`: Release pipeline including npm/python wrapper publishing

**Core Logic:**
- `internal/client/client.go`: HTTP client with pagination, caching, retry, jq, auth
- `internal/errors/errors.go`: Error types and all exit codes
- `internal/policy/policy.go`: Operation access control

**Code Generation:**
- `gen/generator.go`: Core template rendering logic
- `gen/templates/resource.go.tmpl`: Template for each generated resource file
- `spec/jira-v3.json`: Source of truth for generated commands

**Agent/Autonomous:**
- `cmd/avatar_act.go`: Autonomous watch-and-react loop
- `internal/character/types.go`: Character struct
- `internal/yolo/config.go`: Yolo policy config and scope tier definitions
- `internal/watch/watcher.go`: JQL polling loop

**Testing:**
- `cmd/*_test.go`: Unit tests co-located with commands
- `internal/*/*_test.go`: Unit tests co-located with packages
- `test/e2e/e2e_test.go`: End-to-end tests
- `test/integration/integration_test.go`: Integration tests

## Naming Conventions

**Files:**
- Command files: match the command name (`workflow.go`, `avatar.go`, `batch.go`)
- Test files: `<name>_test.go` co-located with source
- Generated files: named after the Jira API resource (`issue.go`, `project.go`, `search.go`)
- Internal packages: single concern per package, named for the concept (`client`, `errors`, `policy`)

**Directories:**
- `internal/` packages: lowercase, single word (`adf`, `jq`, `cache`, `audit`)
- Multi-word packages: lowercase no separator (`changelog`, `duration`)

**Go symbols:**
- Exported types: PascalCase (`APIError`, `Client`, `BatchOp`, `Character`)
- Unexported types: camelCase (`paginatedPage`, `contextKey`)
- Command variables: `<name>Cmd` suffix (`workflowCmd`, `avatarCmd`, `batchCmd`)
- Run functions: `run<CommandName>` (`runAvatarExtract`, `runBatch`, `runRaw`)

## Where to Add New Code

**New hand-written command:**
- Implementation: `cmd/<command>.go` (new file)
- Register: add `rootCmd.AddCommand(<name>Cmd)` in the file's `init()`
- Tests: `cmd/<command>_test.go`

**New workflow subcommand:**
- Implementation: add `*cobra.Command` and `runXxx()` to `cmd/workflow.go`
- Register: add to `workflowCmd.AddCommand(...)` in `cmd/workflow.go`'s `init()`

**New internal utility:**
- Package: `internal/<concept>/` -- one file per concern, tests co-located
- Import as: `github.com/sofq/jira-cli/internal/<concept>`

**New generated command (from OpenAPI):**
- Do not add manually -- update `spec/jira-v3.json` and run `make generate`

**New issue template:**
- Add YAML file to `internal/template/builtin/<name>.yaml`
- File must include `name`, `description`, `variables`, and `fields` keys

**New character template:**
- Add entry to `internal/character/templates.go`

**New avatar analyzer:**
- Implement in `internal/avatar/analyze_<aspect>.go`
- Wire into `internal/avatar/extract.go`

## Special Directories

**`cmd/generated/`:**
- Purpose: Auto-generated Cobra command implementations for the full Jira API
- Generated: Yes -- `make generate` reads `spec/jira-v3.json` and rewrites this directory
- Committed: Yes -- enables builds without running the generator
- Rule: Never edit files here manually; changes will be overwritten

**`.claude/`:**
- Purpose: Claude agent worktrees and project-specific skill definitions
- Generated: Yes (worktrees created during agent sessions)
- Committed: No (worktrees are gitignored)

**`.planning/`:**
- Purpose: GSD planning documents written by `map-codebase`, consumed by `plan-phase` and `execute-phase`
- Generated: Yes (by `/gsd:map-codebase`)
- Committed: Yes

**`website/`:**
- Purpose: VitePress documentation site
- Generated: Partially (`website/.vitepress/dist/` is build output)
- Committed: Source only (not `dist/`)

**`spec/`:**
- Purpose: Jira OpenAPI spec -- source of truth for code generation
- Generated: No -- fetched from Atlassian
- Committed: Yes

---

*Structure analysis: 2026-03-23*
