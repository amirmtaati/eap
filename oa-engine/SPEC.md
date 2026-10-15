# oa-engine: Specification

<!-- generated:registry:begin (do not edit by hand; run the spec generator) -->
## Master digest
1. Purpose: The Go core: parses org project documents, computes deltas, selects context, assembles prompts, calls OpenAI-compatible endpoints, streams events, and ships as the oa CLI.
2. Layout:
   - cmd/oa/: CLI entry point
   - cmd/oa-mockrouter/: Mock OpenAI-compatible server for tests
   - internal/org/: Org-mode scanner, AST, key rules, and node hashing
   - internal/snapshot/: Snapshot file persistence and map deserialization
   - internal/delta/: Node diffing against historical snapshots
   - internal/ctxsel/: Context selection (:ctx: tag, ancestor chain, focus)
   - internal/prompt/: Prompt templating and diff formatting
   - internal/provider/: OpenAI-compatible HTTP client and SSE stream reader
   - internal/events/: Event definitions and deterministic NDJSON encoder
   - internal/cli/: Subcommand logic and flag parsing
   - testdata/protocol/: Golden event streams and contract fixtures
   - docs/protocol.md: Wire protocol and contract documentation
3. Connections:
   - Consumes: none (Go standard library only)
   - Consumed by: oa-emacs (runtime via subprocess pipes, NDJSON), oa-eval (runtime), oa-testkit (test)
4. Contracts:
   - Owns: engine-protocol (v1), document-json-shapes (schema 1), org-project-conventions (v1)
   - Implements or consumes: api-key-convention (OA_API_KEY environment variable)
5. Gates:
   - make ci (gofmt -s, go vet ./..., go test -race ./...)
   - make fuzz (parser fuzz tests)
6. Tasks:
   - G0: Module skeleton (go.mod, Makefile, AGENTS.md, directory stubs)
   - G1: Org parser (internal/org scanner and AST)
   - G2: Snapshot persistence (internal/snapshot)
   - G3: Delta engine (internal/delta)
   - G4: Context selector (internal/ctxsel)
   - G5: Prompt assembler (internal/prompt)
   - G6: Provider client (internal/provider: /models and SSE chat)
   - G7: Events and NDJSON encoder (internal/events)
   - G8: CLI subcommands (internal/cli and cmd/oa)
   - G9: Mock router (cmd/oa-mockrouter)
   - G10: Golden protocol fixtures and sync
   - G11: Focus line resolution (-focus-line) and oa version -json
   - C0: Contract freeze (docs/protocol.md and testdata/protocol/)
   - E1: Spike A1 (9router real-world fixture capture)
   - E2: internal/events encoder + oa-mockrouter
   - E3: internal/provider streaming client & fault handling
   - E4: CLI integration (oa models, oa chat, exit codes, security controls)
7. Assumptions and pitfalls:
   - Shipped binaries must strictly use the Go standard library (zero third-party dependencies).
   - Single binary artifact `oa`.
   - Loopback HTTP is permitted without TLS; non-loopback plaintext HTTP must fail with exit 7 unless `-allow-insecure-http` is passed.
   - OA_API_KEY is read strictly from the environment; never accept API keys via command-line flags or output them in logs/errors.
<!-- generated:registry:end -->

---

## 1. Scope & Mission

`oa-engine` provides the core processing engine for Org Agent. It runs as a standalone command-line executable (`oa`) invoked by editor clients (such as `oa-emacs`).

Its core responsibilities are:
1. Parsing Org-mode project files into structured syntax trees.
2. Generating stable node identification keys and cryptographic hashes of node contents.
3. Managing project snapshots to detect additions, modifications, and deletions.
4. Selecting relevant context nodes based on tags (`:ctx:`), ancestor paths, and user focus.
5. Assembling self-contained prompts containing instructions, project context, and delta changes.
6. Communicating with OpenAI-compatible LLM endpoints (such as local 9router instances) over HTTP/SSE.
7. Emitting streamed events and completion responses as newline-delimited JSON (NDJSON) over standard output.
8. Exiting with deterministic, standard exit codes to signal status to calling processes.

Non-goals for v0.1:
- Tool execution / agent loop (file editing, shell execution).
- Multi-file Org document graphs.
- Proprietary LLM APIs (native Anthropic or Google Gemini wire formats).

---

## 2. Design Principles & Architecture

```
                    ┌───────────────────────────┐
                    │     Input Org Document    │
                    └─────────────┬─────────────┘
                                  ▼
                    ┌───────────────────────────┐
                    │       internal/org        │
                    │   (Scanner, AST, Hashes)  │
                    └──────┬─────────────┬──────┘
                           │             │
              Current Tree │             │
                           ▼             ▼
  ┌───────────────────────────┐       ┌───────────────────────────┐
  │     internal/snapshot     │       │      internal/delta       │
  │     (Load/Save States)    │──────►│  (Diff vs Historical Snap)│
  └───────────────────────────┘       └─────────────┬─────────────┘
                                                    │ Changes & Diffs
                                                    ▼
                                      ┌───────────────────────────┐
                                      │      internal/ctxsel      │
                                      │  (Select :ctx: & Parents) │
                                      └─────────────┬─────────────┘
                                                    │ Context + Delta
                                                    ▼
                                      ┌───────────────────────────┐
                                      │      internal/prompt      │
                                      │   (Assemble Prompt Text)  │
                                      └─────────────┬─────────────┘
                                                    │
                                                    ▼
                                      ┌───────────────────────────┐
                                      │     internal/provider     │
                                      │ (HTTP POST SSE to 9router)│
                                      └─────────────┬─────────────┘
                                                    │ Streaming Chunks
                                                    ▼
                                      ┌───────────────────────────┐
                                      │      internal/events      │
                                      │   (NDJSON Stream Encoder) │
                                      └─────────────┬─────────────┘
                                                    │
                                                    ▼
                                        stdout: NDJSON events
```

Key rules:
- **Zero Third-Party Dependencies:** Only Go standard library packages are allowed in shipped code (`cmd/oa` and `internal/*`).
- **Stateless Operation:** The engine maintains no background daemon or persistent network connection; it executes as a one-shot process per command.
- **Fail-Safe Secret Handling:** API keys are injected via `OA_API_KEY` only. They must never appear in `argv`, log files, or stdout/stderr.
- **Deterministic Output:** JSON outputs and NDJSON event streams have fixed field orderings and consistent schemas (`"schema": 1`).

---

## 3. Toolchain & Directory Layout

Go version: **1.22+**.

```
oa-engine/
├── cmd/
│   ├── oa/                 # Main CLI binary entry point
│   └── oa-mockrouter/      # Test mock OpenAI server
├── internal/
│   ├── org/                # Org parser, AST, key rules, node hashing
│   ├── snapshot/           # Snapshot serialization/deserialization
│   ├── delta/              # Document diffing against snapshots
│   ├── ctxsel/             # Context selection algorithms
│   ├── prompt/             # Prompt generation and formatting
│   ├── provider/           # OpenAI HTTP/SSE client implementation
│   ├── events/             # NDJSON event definitions and encoder
│   └── cli/                # CLI subcommands and flag parsing
├── testdata/
│   └── protocol/           # Golden NDJSON fixtures and mock responses
├── docs/
│   └── protocol.md         # Protocol documentation
├── Makefile                # Quality gates (ci, build, test, fuzz)
├── AGENTS.md               # Agent instructions for engine implementation
└── SPEC.md                 # This specification
```

---

## 4. Wire Protocol & Data Models

All JSON objects emitted or consumed by the engine must include `"schema": 1`.

### 4.1 Node Data Structure
```json
{
  "key": "roadmap/milestone-1",
  "title": "Milestone 1",
  "level": 2,
  "line": 42,
  "tags": ["ctx", "urgent"],
  "properties": {
    "CUSTOM_ID": "m1"
  },
  "content_hash": "a1b2c3d4...",
  "subtree_hash": "e5f6g7h8..."
}
```

### 4.2 Snapshot File (`.oa/snapshot.json`)
```json
{
  "schema": 1,
  "timestamp": "2026-10-14T08:30:00Z",
  "nodes": {
    "roadmap/milestone-1": {
      "content_hash": "a1b2c3d4...",
      "subtree_hash": "e5f6g7h8..."
    }
  }
}
```

### 4.3 Delta Output
```json
{
  "schema": 1,
  "added": ["roadmap/milestone-2"],
  "modified": ["roadmap/milestone-1"],
  "deleted": ["legacy-task"],
  "unchanged_count": 14
}
```

### 4.4 `oa models` Output
```json
{
  "schema": 1,
  "models": [
    {
      "id": "qwen2.5-coder-32b",
      "owned_by": "9router"
    }
  ]
}
```

### 4.5 `oa chat` Stream Events (NDJSON)
Each event is a single line terminated by `\n`:
- **Start:**
  ```json
  {"schema":1,"event":"start","model":"qwen2.5-coder-32b","tokens_estimate":1840,"context_keys":["roadmap/milestone-1"],"delta_count":2,"history_count":1,"focus_key":""}
  ```
- **Delta:**
  ```json
  {"schema":1,"event":"delta","text":"Here is the proposed update."}
  ```
- **Usage:**
  ```json
  {"schema":1,"event":"usage","prompt_tokens":1900,"completion_tokens":420}
  ```
- **Done:**
  ```json
  {"schema":1,"event":"done","finish_reason":"stop"}
  ```
- **Error:**
  ```json
  {"schema":1,"event":"error","kind":"auth","message":"401 Unauthorized from endpoint"}
  ```

Error `kind` must be one of:
`nothing_to_send`, `bad_request`, `auth`, `network`, `timeout`, `provider`, `bad_response`, `cancelled`, `insecure_endpoint`, `unsupported_snapshot`.

---

## 5. Org Mode Parser & AST (`internal/org`)

The parser scans Org documents line-by-line without external libraries.

### 5.1 Headline Parsing
- Matches lines matching `^(\*+)\s+(.*?)(?:\s+:([a-zA-Z0-9_@#%:]+):)?$`.
- Asterisk count determines node depth (`level`).
- Trailing colon-delimited tags are parsed into a string slice.

### 5.2 Property Drawers
- Matches `:PROPERTIES:` and `:END:` immediately following headlines.
- Pairs of `:KEY: Value` are extracted into `map[string]string`.

### 5.3 Key Generation Rules
1. If `:CUSTOM_ID:` or `:ID:` property exists, use its sanitized slug.
2. Otherwise, construct a hierarchical slug path from ancestor titles:
   - Example: `* Project Goals` -> `** Frontend` => `project-goals/frontend`.
3. If duplicate keys exist at the same level, append numeric suffixes (`-1`, `-2`).

### 5.4 Hash Computation
- `content_hash`: SHA-256 over headline text, tags, properties, and body content lines (excluding child headlines).
- `subtree_hash`: SHA-256 combining `content_hash` and all child `subtree_hash` values in pre-order traversal.

---

## 6. Snapshot Persistence (`internal/snapshot`)

- Serializes snapshot data to JSON with atomic write (write to temporary file, then rename).
- Reads existing `.oa/snapshot.json` files.
- Rejects snapshots with unsupported `schema` versions (exit code 3).

---

## 7. Delta Engine (`internal/delta`)

Compares the current AST against snapshot entries:
- **Added:** Key exists in current AST but not in snapshot.
- **Modified:** Key exists in both, but `content_hash` differs.
- **Deleted:** Key exists in snapshot but not in current AST.
- **Unchanged:** Key exists in both with identical `content_hash`.

If `-focus` or `-focus-line` is provided, delta evaluation restricts comparison to the targeted node and its subtree.

---

## 8. Context Selection (`internal/ctxsel`)

Context nodes provide background information to the model:
1. **Explicit Context:** Any node with the `:ctx:` tag is included.
2. **Structural Anchors:** Direct parent nodes of added or modified headlines are included.
3. **Focus Subtree:** When a focus key/line is specified, enclosing ancestors and direct siblings are prioritized.
4. **Agent History:** Up to `oa-history-limit` recent entries under `* Agent :agent:` are included for conversation continuity.

Nodes marked with `:noctx:` are explicitly excluded from context.

---

## 9. Prompt Assembly (`internal/prompt`)

Assembles the system and user messages for the LLM:
- **System Instructions:** Role description, Org syntax formatting constraints, and instruction that model output must use Org headings that fit under the agent response node.
- **Context Section:** Included `:ctx:` and ancestor nodes.
- **Changes Section:** Additions and modifications with diff representations.
- **Focus Request:** Specific user instructions or focused node text.

---

## 10. Provider Client (`internal/provider`)

OpenAI-compatible HTTP client:
- Endpoints:
  - Models: `GET {url}/models`
  - Chat: `POST {url}/chat/completions` with streaming enabled (`"stream": true`).
- Headers:
  - `Content-Type: application/json`
  - `Authorization: Bearer <OA_API_KEY>` (if `OA_API_KEY` is non-empty).
- Server-Sent Events (SSE) Parsing:
  - Handles line prefixes `data: `.
  - Parses JSON chunks containing `choices[0].delta.content`.
  - Terminates cleanly upon receiving `data: [DONE]`.
  - Tolerates keep-alive comments (`: keep-alive`) and empty lines.
- Memory & Bounds:
  - Max response body bounded to 64 MB.
  - Idle timeout triggers if no chunk is received within `-idle-timeout` (default 60s).

---

## 11. Event Framing & NDJSON (`internal/events`)

- Ensures deterministic JSON serialization (stable key ordering).
- Encodes events directly to `io.Writer` (stdout).
- Trailing newline after every JSON object.

---

## 12. CLI Interface & Exit Codes (`cmd/oa`)

### 12.1 Commands
```bash
oa models  -url URL [-timeout 10s] [-allow-insecure-http]
oa chat    -url URL -model ID [-file PATH | -] -snapshot PATH \
           [-focus KEY | -focus-line N] [-history N] [-max-tokens N] \
           [-timeout 10s] [-idle-timeout 60s] [-allow-insecure-http]
oa prompt  [-file PATH | -] -snapshot PATH [-focus KEY | -focus-line N]
oa parse   [-file PATH | -] [-json]
oa snapshot -out PATH [-file PATH | -]
oa version [-json]
```

### 12.2 Exit Codes
- `0`: Success.
- `1`: Usage error, I/O error, invalid flag, or unknown focus node.
- `2`: Nothing to send (zero delta changes detected).
- `3`: Unsupported snapshot version.
- `4`: Network failure or connect/idle timeout.
- `5`: Authentication/authorization error (HTTP 401/403).
- `6`: Provider error (HTTP 5xx, invalid SSE stream).
- `7`: Insecure HTTP rejected (attempt to send credentials over plaintext HTTP to a non-loopback host).
- `130`: Cancelled by signal (SIGINT, SIGTERM).

---

## 13. Mock Router (`cmd/oa-mockrouter`)

A lightweight HTTP server used strictly for automated testing:
- Responds to `GET /v1/models` with configurable model fixtures.
- Responds to `POST /v1/chat/completions` with streamed SSE chunks mimicking OpenAI/9router.
- Configurable failure modes: simulated delay, 401 auth failure, network drop, malformed SSE chunks.

---

## 14. Non-Functional & Security Requirements

1. **Secret Isolation:**
   - `OA_API_KEY` is retrieved from `os.Getenv`.
   - Never log or format `OA_API_KEY` in error strings or output streams.
2. **Insecure Endpoint Rejection:**
   - If URL scheme is `http://` and host is not a loopback address (`localhost`, `127.0.0.1`, `::1`), exit with code `7` unless `-allow-insecure-http` is explicitly supplied.
3. **Graceful Cancellation:**
   - Listen for `os.Interrupt` and `syscall.SIGTERM`.
   - On signal receipt, abort the active HTTP request context, emit `{"schema":1,"event":"error","kind":"cancelled","message":"request cancelled"}`, and exit with code `130` within 1 second.

---

## 15. Quality Gates & Test Strategy

Run all gates via `Makefile`:
```bash
make ci    # Runs gofmt -s, go vet, and go test -race ./...
make fuzz  # Runs 60-second fuzzing pass on internal/org parser
make build # Builds bin/oa and bin/oa-mockrouter
```

Unit testing covers:
- AST parsing and property extraction across edge-case Org syntax.
- Key collision resolution.
- Snapshot serialization and hash stability.
- SSE stream chunk reassembly and error handling.
- Deterministic NDJSON encoding.
