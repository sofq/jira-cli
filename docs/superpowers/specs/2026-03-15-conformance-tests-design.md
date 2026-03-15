# Schema Conformance Tests Design Spec

**Date:** 2026-03-15
**Status:** Approved

## Overview

A Go test suite (`gen/conformance_test.go`) that parses the Jira OpenAPI v3 spec and compares it against the generated CLI commands to detect drift. Runs as part of `go test ./...` and exits non-zero on any mismatch, making it suitable as a CI gate.

## Goals

1. Verify every operation in the OpenAPI spec has a corresponding generated CLI command
2. Verify no extra operations exist in generated code that aren't in the spec
3. Verify each operation's flags (path params, query params, body) match the spec
4. Verify required/optional status of flags matches the spec
5. Exit non-zero on any mismatch (CI-compatible)
6. Produce clear, actionable failure messages

## Non-Goals

- Live API calls or response validation
- Request body schema validation (only checks presence of `--body` flag)
- Performance testing
- Testing CLI behavior (covered by e2e_test.go)

## Architecture

The test lives in `gen/conformance_test.go` (package main), reusing the existing parser and grouper:

```
gen/conformance_test.go
    │
    ├── ParseSpec("../spec/jira-v3.json")     # existing parser
    ├── GroupOperations(ops)                    # existing grouper
    ├── deduplicateVerbs(ops, resource)         # existing dedup from generator
    ├── DeriveVerb(...)                         # existing verb derivation
    │
    └── Compare against actual generated output
        by parsing cmd/generated/schema_data.go as Go source
```

### Approach: Spec-to-Expected vs Actual

The test builds two data sets:

1. **Expected** — parsed from `spec/jira-v3.json` using the same pipeline as the generator (ParseSpec → GroupOperations → deduplicateVerbs → DeriveVerb). This produces the canonical set of resources, verbs, methods, paths, and flags.

2. **Actual** — parsed from `cmd/generated/schema_data.go` by loading it as a Go source file and extracting the `AllSchemaOps()` return data. Alternatively, since the test is in `package main` alongside the generator, it can re-run the generation logic into a temp dir and diff against the committed `cmd/generated/` files.

**Recommended:** Compare by re-deriving expected operations from the spec and comparing to the committed schema_data.go parsed as structured data. This catches both generator bugs and stale generated code.

## Checks

### 1. Operation Coverage

| Check | Detection |
|-------|-----------|
| Missing operation | Spec has path+method, no matching (resource, verb) in actual |
| Extra operation | Actual has (resource, verb) not derivable from spec |
| Wrong HTTP method | Same (resource, verb) but method differs |
| Wrong path | Same (resource, verb) but path template differs |

### 2. Flag Conformance (per operation)

| Check | Detection |
|-------|-----------|
| Missing path param | Spec has `{param}` in path, no flag with In="path" in actual |
| Missing query param | Spec has query parameter, no flag with In="query" in actual |
| Extra flag | Actual has a flag not in spec (excluding "body") |
| Wrong required | Spec says required, actual says optional (or vice versa) |
| Missing body flag | Spec has requestBody, actual has no flag with In="body" |
| Extra body flag | Spec has no requestBody, actual has body flag |

## Test Functions

```go
// TestConformance_OperationCoverage checks that every spec operation
// has a corresponding generated command and vice versa.
func TestConformance_OperationCoverage(t *testing.T)

// TestConformance_FlagsMatch checks that each operation's flags
// (path params, query params, body) match the spec exactly.
func TestConformance_FlagsMatch(t *testing.T)
```

Both tests use `t.Run()` subtests per resource group for granular failure reporting.

## Data Structures

```go
// expectedOp represents what the spec says an operation should look like.
type expectedOp struct {
    Resource    string
    Verb        string
    Method      string
    Path        string
    PathParams  []expectedFlag
    QueryParams []expectedFlag
    HasBody     bool
}

type expectedFlag struct {
    Name     string
    Required bool
    Type     string
}
```

## Failure Output Format

Clear, actionable messages with enough context to fix the issue:

```
--- FAIL: TestConformance_OperationCoverage/issue
    conformance_test.go:52: missing operations (spec has, generated doesn't):
        GET /rest/api/3/issue/{issueIdOrKey}/properties → expected verb "get-properties"
    conformance_test.go:58: extra operations (generated has, spec doesn't):
        GET /rest/api/3/issue/old-endpoint → verb "get-old-endpoint"

--- FAIL: TestConformance_FlagsMatch/issue/get
    conformance_test.go:85: missing query params: [properties, updateHistory]
    conformance_test.go:89: flag "issueIdOrKey": expected required=true, got required=false
```

## Loading Actual Data

Since `gen/` is `package main` and cannot import `cmd/generated`, the test loads actual data by:

1. Reading `cmd/generated/schema_data.go` as a text file
2. Parsing it with `go/ast` to extract the `AllSchemaOps()` return slice
3. Building a map of `(resource, verb) → SchemaOp` for comparison

This avoids import cycles and works with the existing package structure.

Alternative: use `go/parser` + `go/ast` to extract the struct literals, or use a simpler regex-based extraction of the key fields. The AST approach is more robust.

## File Structure

```
gen/
├── conformance_test.go    # Test file (new)
├── conformance_helpers.go # AST parser for schema_data.go + comparison logic (new)
├── parser.go              # Existing — reused
├── grouper.go             # Existing — reused
├── generator.go           # Existing — deduplicateVerbs reused
└── ...
```

## Integration

- Runs with `go test ./gen/...` or `make test`
- No special setup needed — uses committed spec and generated files
- Fails CI if generated code is stale after spec update
- Developer workflow: `make spec-update && make generate && make test`

## Edge Cases

- **Deprecated endpoints**: included in spec, should still have generated commands
- **operationId collisions**: handled by deduplicateVerbs, test uses same logic
- **Parameter name = Go keyword** (e.g., `type`): test compares spec param names, not Go identifiers
- **Path-level vs operation-level params**: test merges both (same as parser)
