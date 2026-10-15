# oa-engine

The Go core engine for Org Agent (`oa`).

Zero third-party dependencies. Built exclusively using the Go standard library.

## Responsibilities

1. **Parser (`internal/org`)**: Scans Org-mode files into AST nodes with stable hierarchical keys and SHA-256 hashes.
2. **Snapshot (`internal/snapshot`)**: Saves and loads `.oa/snapshot.json` (`schema: 1`).
3. **Delta (`internal/delta`)**: Detects added, modified, and deleted nodes against snapshots; resolves focus line or key.
4. **Context Selection (`internal/ctxsel`)**: Selects `:ctx:` nodes, structural parents of changes, and recent agent history.
5. **Prompt Assembly (`internal/prompt`)**: Produces plain-text LLM prompts.
6. **Provider & CLI (`internal/provider`, `internal/cli`)**: Streams chat completions from OpenAI-compatible endpoints over NDJSON.

## Requirements

- Go 1.22+

## Building & Testing

```bash
make build   # Compiles bin/oa
make test    # Runs unit tests
make ci      # Runs gofmt check, go vet, and unit tests
make fuzz    # Runs parser fuzz tests
```

*Note: In restricted build environments, pass `GOCACHE=/tmp/oa-gocache`.*

## CLI Commands

```bash
# Parse an org document (JSON or human summary)
./bin/oa parse -file project.org [-json]

# Save snapshot of document state
./bin/oa snapshot -out .oa/snapshot.json -file project.org

# Build plain-text prompt based on snapshot delta
./bin/oa prompt -file project.org -snapshot .oa/snapshot.json [-focus KEY | -focus-line N]

# Query OpenAI-compatible models endpoint
./bin/oa models -url http://localhost:20128/v1

# Stream completion via NDJSON events
./bin/oa chat -url http://localhost:20128/v1 -model <ID> -snapshot .oa/snapshot.json project.org

# Print version and protocol compatibility
./bin/oa version [-json]
```

## Exit Codes

- `0`: Success
- `1`: Usage error, I/O error, or unknown focus
- `2`: Nothing to send (zero delta changes)
- `3`: Unsupported snapshot version
- `4`: Network error or timeout
- `5`: Authentication/authorization error
- `6`: Provider error
- `7`: Insecure HTTP endpoint refused
- `130`: Cancelled (SIGINT/SIGTERM)
