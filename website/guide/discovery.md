# Discovering Commands

`jr` has over 600 commands, all auto-generated from the official Jira OpenAPI v3 spec. Rather than memorizing them, use `jr schema` to explore what is available.

## Four discovery modes

**1. Resource-to-verb mapping** (default, best starting point):
```bash
jr schema
# Shows every resource and its available verbs
```

**2. List all resource names:**
```bash
jr schema --list
# issue, project, search, workflow, board, sprint, ...
```

**3. All operations for a resource:**
```bash
jr schema issue
# Lists every operation under the "issue" resource, with flags
```

**4. Full schema for a single operation:**
```bash
jr schema issue get
# Shows all available flags, types, and descriptions for "issue get"
```

::: tip
Start with `jr schema` or `jr schema --list` to orient yourself, then drill into a specific resource and operation. This is especially useful for AI agents that need to discover commands at runtime.
:::
