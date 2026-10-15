# Org Agent: Master Specification

> Scope: the **whole project** (Go engine + Emacs client) and the **master agent** that builds it.
> Working names (placeholders): project `oa`, engine repo `oa-engine`, Emacs repo `oa-emacs`, Elisp prefix `oa-`.
> Status: draft v0.1.
>
> **Document precedence** (highest first): `MASTER-SPEC.md` (this file) > each repo's own `SPEC.md` (internals of that repo, see 2.3) > the legacy Elisp-first design (concepts and data shapes only). Where this file amends a repo spec, section 4 lists the amendments, and this file wins.
>
> **Naming note:** until task B0 moves them, the engine spec exists as `SPEC-GO.md` and the legacy Elisp-first spec as `SPEC.md` at the workspace root. Every reference to `SPEC-GO.md` in this file means `oa-engine/SPEC.md` once B0 is done.
>
> **Repository map:** section 17 is the registry of every repo the project will ever need, with status, connections, and contract ownership. It is the source from which the Master digest of each repo spec is generated. (It is appended after the appendix so that all earlier section numbers stay stable.)

---

## 1. Vision and scope

### 1.1 What we are building

A way to do agentic development with **any LLM, from inside Emacs, driven by a single org-mode project document**.

- The human writes and edits one org document (the project's goals, conventions, tasks, notes).
- A Go **engine** works out what changed since the last send, selects stable context, builds the prompt, calls the model through an OpenAI-compatible endpoint, and streams the answer back.
- An Emacs **client** is a thin, clean interface over the engine: manage endpoints, fetch and choose models, send with a key binding, watch the streamed output in its own buffer, then accept the answer into the document.

### 1.2 First-class target: 9router

The primary endpoint is a self-hosted **9router**, an OpenAI-compatible gateway, by default at `http://localhost:20128/v1` with chat completions at `/v1/chat/completions`. The system speaks the **OpenAI-compatible protocol**, not a 9router-specific one, so any compatible endpoint works.

**Assumption A1 (verify in task E1):** `GET {base}/models` returns the OpenAI-style list (`{"data":[{"id":"…","owned_by":"…"}, …]}`). The human supplies one real, secret-free sample response; it becomes a test fixture.

### 1.3 Non-goals for v0.1

Agent tool loop (read/grep/patch tools), sub-agents inside the product, multi-file document graphs, other editors, GUI outside Emacs, non-OpenAI wire formats (`/v1/responses`, native Anthropic or Gemini APIs), MELPA packaging.

---

## 2. Workspace layout

```
oa-workspace/                 # root: optionally its own git repo (specs + plan only)
├── MASTER-SPEC.md            # the only spec at the root (cross-repo contracts + master agent)
├── AGENTS.md                 # workspace rules for the master agent (see 6.6)
├── .gitignore                # ignores oa-engine/ and oa-emacs/ (they are separate repos)
├── .master/
│   ├── PLAN.org              # task ledger (the project's own org document)
│   ├── reports/              # one report file per sub-agent run
│   └── agents/               # role definitions: architect.md, implementer-go.md, ...
├── oa-engine/                # git repo #1: Go engine + CLI + mock router; has its own SPEC.md
└── oa-emacs/                 # git repo #2: Elisp package; has its own SPEC.md
```

Rules:

- **One directory = one git repo** with its own history, `main` branch, CI targets, `AGENTS.md`, `README`, and `CHANGELOG`.
- Repos are **not** submodules in v1. The workspace root tracks only the specs and `.master/`.
- Only the **active** repos of section 17 exist as directories. Planned and candidate repos are created when their trigger fires (17.8), never earlier.
- The master agent operates from the workspace root. Sub-agents are confined to **one repo directory** (or one git worktree of it) per run; the only exception is the integrator (see 7.6).
- The **protocol** (section 4) is owned by `oa-engine`. `oa-emacs` vendors fixtures under `test/fixtures/engine/` using `make sync-fixtures` (copies from `../oa-engine/testdata/protocol/`); the integrator verifies they are in sync at every integration gate.

### 2.1 Engine repo layout (`oa-engine`)

As in `SPEC-GO.md` section 3, plus:

```
internal/provider/     # OpenAI-compatible client: models list, streaming chat
internal/events/       # NDJSON event types and encoder
cmd/oa/                # the CLI
cmd/oa-mockrouter/     # test-only mock OpenAI-compatible server (models + streaming chat)
testdata/protocol/     # golden NDJSON event streams, models responses (the contract fixtures)
docs/protocol.md       # the engine <-> client contract (generated from section 4 of this file, then maintained here)
```

### 2.2 Emacs repo layout (`oa-emacs`)

```
oa.el               # entry point, oa-mode, keymap, autoloads
oa-config.el        # endpoints, model cache, state file, auth-source keys
oa-engine.el        # locate binary, spawn process, NDJSON framing, cancel
oa-ui.el            # output buffer, header line, window display, transient menu
oa-doc.el           # capture buffer, response normalization, writeback, commit
test/               # ERT tests, fixtures/engine/ (vendored), fake engine script
Makefile  README.org  CHANGELOG.md  AGENTS.md
```

Dependencies: **Emacs 29.1+, built-ins only** (`org`, `transient`, `auth-source`, `json`, `seq`, `map`, `cl-lib`, `subr-x`). No MELPA dependencies, so installation is a clone plus `load-path`.

### 2.3 Repo spec files (the master's map of each repo)

Every repo has a `SPEC.md` at its root: the authoritative specification of that repo's internals.

- `oa-engine/SPEC.md`: the Go engine spec (today's `SPEC-GO.md`, with the section 4 amendments folded in).
- `oa-emacs/SPEC.md`: the Emacs client spec (extracted from section 5 of this file). The legacy Elisp-first spec is archived at `oa-emacs/docs/legacy-elisp-first.md` for its concepts only.

**Ownership.** Cross-repo contracts (the section 4 protocol, the user-facing names in 5.2 to 5.4, and the register in section 13) stay in this file. Repo specs mirror them and never redefine them. If a repo spec contradicts this file, this file wins and the master raises the conflict.

**Required structure.** Every repo spec starts with a **Master digest** of at most about 40 lines, so the master can understand a repo in one skim. Items 1 to 4 and 6 are generated from the registry (17.9) between marker comments and must not be edited by hand:

1. Purpose (two lines).
2. Layout: package or file to responsibility.
3. Public interfaces and frozen items, each with a section reference.
4. Gates: the exact commands.
5. Task list: ID, dependencies, and the spec section each task implements.
6. Known assumptions and pitfalls.

After the digest come numbered sections with **stable numbers**, so briefs can cite `path §n.m`. Renumbering is a contract-level change: new material is appended, not inserted.

**How the master uses repo specs.**
- At startup (6.3) it skims each digest, then reads in full only the sections relevant to the tasks it is about to brief.
- Briefs (8.2) **cite** sections by path and number and never paste spec text, so a spec change cannot leave stale copies behind.
- A sub-agent reads its own repo's digest plus the cited sections. Each repo's `AGENTS.md` begins with: "Read the digest in SPEC.md, then the sections your brief cites."
- The reviewer checks the diff against the cited sections.
- When a repo spec changes, the master re-checks the briefs of tasks in flight that cite the changed sections and flags them in `PLAN.org`.

---

## 3. System architecture

```
 ┌──────────────────────────── Emacs (oa-emacs) ─────────────────────────────┐
 │ oa-mode in project.org                                                      │
 │   oa-config  ── endpoints / models / key lookup (auth-source)               │
 │   oa-doc     ── capture buffer text, writeback, commit                      │
 │   oa-engine  ── spawn `oa`, NDJSON in, cancel                               │
 │   oa-ui      ── *oa-output* buffer, header line, menu, keys                 │
 └──────────────┬──────────────────────────────────────────▲──────────────────┘
                │ stdin: org text; flags; env OA_API_KEY     │ stdout: JSON / NDJSON events
                ▼                                            │ stderr: warnings, logs
 ┌──────────────────────────── oa (oa-engine) ───────────────┴────────────────┐
 │ org → delta → ctxsel → prompt ──► provider (HTTP, SSE) ──► events           │
 └──────────────┬──────────────────────────────────────────────────────────────┘
                ▼ GET /models, POST /chat/completions (stream)
        9router  (or any OpenAI-compatible endpoint)
```

Responsibilities:

| Concern | Owner |
|---|---|
| Org parsing, delta, context, prompt assembly | Engine |
| HTTP to the endpoint, SSE parsing, cancellation | Engine |
| Which endpoints exist, which model is selected, API keys | Emacs (engine is stateless about these) |
| Buffer capture, writeback into the document, commit | Emacs |
| Streaming display, key bindings, menus | Emacs |

The engine receives the endpoint URL and model as flags and the API key via the **environment variable `OA_API_KEY`**, never on the command line.

---

## 4. Engine amendments (contract)

These extend `SPEC-GO.md`. All JSON carries `"schema": 1`; all stdout JSON is one object per line (NDJSON) when streaming.

### 4.1 New and changed commands

```
oa models  -url URL [-timeout 10s]
oa chat    -url URL -model ID [-file PATH | -] -snapshot PATH
           [-focus KEY | -focus-line N] [-history N] [-max-tokens N]
           [-timeout 10s] [-idle-timeout 60s]
oa prompt  ... (unchanged, plus -focus-line N)
oa version [-json]
```

- `-focus-line N` resolves to the node whose headline is the nearest at or before line N (the "enclosing" node). This lets the editor pass the cursor line instead of reimplementing key rules. Mutually exclusive with `-focus`.
- `oa version -json` prints `{"schema":1,"version":"0.1.0","protocol":1}`. The client refuses to run on a different `protocol`.
- `oa chat` builds the prompt exactly as `oa prompt` does, then streams the completion.

### 4.2 `oa models` output

```json
{"schema":1,"models":[{"id":"some/model","owned_by":"provider"}]}
```

Sorted by `id`. Extra provider fields are ignored. An empty list is success.

### 4.3 `oa chat` event stream (stdout, one JSON object per line)

```json
{"schema":1,"event":"start","model":"ID","tokens_estimate":1840,"context_keys":["Context/Goal"],"delta_count":2,"history_count":1,"focus_key":""}
{"schema":1,"event":"delta","text":"partial text"}
{"schema":1,"event":"usage","prompt_tokens":1900,"completion_tokens":420}
{"schema":1,"event":"done","finish_reason":"stop"}
```

Error form (always the last line, followed by a non-zero exit):

```json
{"schema":1,"event":"error","kind":"auth","message":"401 from endpoint"}
```

`kind` is one of `nothing_to_send`, `bad_request`, `auth`, `network`, `timeout`, `provider`, `bad_response`, `cancelled`, `insecure_endpoint`, `unsupported_snapshot`.

Rules:
- `start` is emitted before the HTTP request, so the client can show the token estimate immediately.
- `delta.text` is a verbatim UTF-8 fragment, never split inside a rune.
- `usage` is emitted only if the endpoint reports it.
- Exactly one of `done` or `error` ends the stream.
- On `SIGINT` or `SIGTERM`, the engine cancels the HTTP request, emits `error` with `kind":"cancelled"`, and exits within 1 s.

### 4.4 Exit codes (extends `SPEC-GO.md` 9.3)

| Code | Meaning |
|---|---|
| 0 | Success |
| 1 | Usage or I/O error, unknown focus |
| 2 | Nothing to send |
| 3 | Unsupported snapshot version |
| 4 | Network error or timeout |
| 5 | Authentication or authorization error (401/403) |
| 6 | Provider error or malformed response |
| 7 | Refused: insecure endpoint |
| 130 | Cancelled |

For `oa chat`, the error event is written to stdout in addition to the exit code, so the client has one parsing path. For other commands, the `SPEC-GO.md` rule holds (empty stdout, one stderr line).

### 4.5 Security behavior

- The key is read from `OA_API_KEY` only. It is never logged, never echoed in errors, and never part of any output.
- Sending a key over plain `http://` to a **non-loopback host** is refused (exit 7, `insecure_endpoint`) unless `-allow-insecure-http` is passed. Loopback (`localhost`, `127.0.0.0/8`, `::1`) is allowed, which covers a local 9router.
- The engine follows no redirects to a different host.
- Response bodies are bounded (default 64 MB total, configurable) to prevent runaway memory use.

### 4.6 Provider behavior

- Models: `GET {url}/models`, header `Authorization: Bearer $OA_API_KEY` if the variable is set.
- Chat: `POST {url}/chat/completions` with `{"model":…,"messages":[…],"stream":true}` plus `max_tokens` if given. Parse `text/event-stream`, taking `choices[0].delta.content` as text, stopping at `[DONE]`. Tolerate comment lines, keep-alives, empty deltas, and a final chunk containing only `usage`.
- `url` is used as given with a trailing slash trimmed; the engine does not add or remove `/v1`.
- A non-2xx response is mapped to `auth` (401/403), `provider` (other), including the endpoint's error message if it is short and JSON-parseable.

### 4.7 Other amendments

- **Response structure:** the default instruction in `SPEC-GO.md` 8.7 stays, but the **client re-levels headlines** in the response before display and writeback (see 5.8), because a model's `*` headline would otherwise become a top-level node of the project document.
- **`SPEC-GO.md` 1.3** (out of scope: calling an LLM) is **lifted** by this document: the engine now includes `internal/provider` and `oa chat`.
- **Package `internal/events`** defines the event types and a deterministic encoder (stable field order), with golden files in `testdata/protocol/`.

---

## 5. Emacs client specification (user-facing contract)

The interface must be **clean, predictable, and fully usable from the keyboard**. Every user action is an interactive command with a docstring; nothing requires editing Elisp to get started.

### 5.1 Setup in five lines (the target experience)

```elisp
(add-to-list 'load-path "~/src/oa-workspace/oa-emacs")
(require 'oa)
(add-hook 'org-mode-hook #'oa-maybe-enable)   ; or: M-x oa-mode in a project buffer
;; then: M-x oa-add-endpoint, M-x oa-select-model, C-c a s
```

`oa-maybe-enable` turns on `oa-mode` in an org buffer only if the file is marked as an oa project (`#+OA: t` keyword or a `.oa/` directory next to it).

### 5.2 Key bindings (`oa-mode`, prefix configurable via `oa-keymap-prefix`, default `C-c a`)

| Keys | Command | Action |
|---|---|---|
| `C-c a s` | `oa-send` | Send changes since last send (see 5.5) |
| `C-c a f` | `oa-send-focus` | Send the subtree at point as focus (works with nothing new) |
| `C-c a p` | `oa-preview` | Show the exact prompt without sending |
| `C-c a m` | `oa-select-model` | Choose the model |
| `C-c a e` | `oa-select-endpoint` | Choose the endpoint |
| `C-c a E` | `oa-add-endpoint` | Add an endpoint |
| `C-c a r` | `oa-refresh-models` | Fetch models from the current endpoint |
| `C-c a o` | `oa-show-output` | Show the output buffer |
| `C-c a k` | `oa-cancel` | Cancel the running request |
| `C-c a a` | `oa-accept` | Accept the last response into the document |
| `C-c a ?` | `oa-menu` | Transient menu with all of the above and the current state |

Rationale: `C-c <letter>` is reserved for users by Emacs convention, so the package binds it only inside an explicitly enabled minor mode, and the prefix is a `defcustom`. In the output buffer: `C-c C-c` accepts, `C-c C-k` cancels, `q` hides the window, `g` re-sends the last request.

### 5.3 Customization (`M-x customize-group RET oa`)

| Variable | Default | Meaning |
|---|---|---|
| `oa-engine-program` | `"oa"` | Engine executable (resolved with `executable-find`) |
| `oa-keymap-prefix` | `"C-c a"` | Prefix key |
| `oa-confirm-before-send` | `summary` | `nil` (just send), `summary` (one-line y/n with counts and endpoint host), `preview` (show full prompt first) |
| `oa-accept-mode` | `ask` | After a response: `ask`, `auto`, or `manual` (only on `oa-accept`) |
| `oa-display-buffer-action` | side window, right, 40% | Passed to `display-buffer` for the output buffer |
| `oa-models-cache-ttl` | `86400` | Seconds before cached models are considered stale |
| `oa-state-file` | `<user-emacs-directory>/oa/state.eld` | Endpoints, selection, model cache (never secrets) |
| `oa-snapshot-file` | `".oa/snapshot.json"` | Relative to the document |
| `oa-ctx-tag`, `oa-agent-tag` | `"ctx"`, `"agent"` | Tag names |
| `oa-history-limit` | `3` | Agent entries sent back |
| `oa-max-tokens` | `nil` | Optional completion cap |
| `oa-connect-timeout`, `oa-idle-timeout` | `10`, `60` | Seconds |

### 5.4 Endpoints, keys, models

**Data model** (stored in the state file; no secrets):

```elisp
(:name "9router" :url "http://localhost:20128/v1" :key-source (:type env :name "NINEROUTER_KEY"))
;; :key-source is one of: (:type none) | (:type env :name VAR) | (:type auth-source :host "localhost")
```

**`oa-add-endpoint`** (interactive flow):
1. Prompt for a name, and for the URL with the hint `e.g. http://localhost:20128/v1` pre-filled as the initial input. Trailing slashes are trimmed. Reject anything that is not `http(s)://…` with a clear message.
2. Prompt for how the key is provided: *none*, *environment variable* (ask for the name), or *auth-source* (host defaults to the URL's host; the key is looked up in `~/.authinfo(.gpg)`, never stored by this package).
3. **Validate** by running `oa models`. On success, report `N models found`, cache them, and offer to select one now. On failure, show the reason by error kind (auth, network, timeout) and offer *edit*, *save anyway*, or *discard*.
4. If a key is required and missing, say exactly where it was expected (the variable name or the auth-source host).

**`oa-select-endpoint`**: `completing-read` among endpoints (current one first). Switching clears the selected model if it is not in the new endpoint's cache, and refreshes asynchronously.

**`oa-refresh-models`**: async; echo-area progress; updates the cache with a timestamp; on failure keeps the old cache and reports the error. First send after cache expiry triggers a background refresh but does not block the send.

**`oa-select-model`**: `completing-read` over cached model ids, annotated with `owned_by`, last-used first, then alphabetical. If the cache is empty it refreshes first. **Free-text entry is allowed**, so a model not listed (or a failed fetch) never blocks the user. The choice persists in the state file and shows in the header line and menu.

**`oa-remove-endpoint`** and **`oa-edit-endpoint`** exist for completeness.

All state reads happen lazily on first use, not at load time; writes are atomic (temp file plus rename).

### 5.5 The send flow (precise)

`oa-send`:
1. If no endpoint or model is selected, run the corresponding command, then continue. Never fail with a bare error.
2. If a request is already running for this project, ask: *cancel it and start a new one?*
3. **Capture** the buffer text (including unsaved edits) into a variable `sent-text`. This exact text is used for both the request and the later commit.
4. Run `oa prompt` with the captured text. If it exits 2, message `Nothing new to send. Use C-c a f to send the subtree at point.` and stop.
5. Apply `oa-confirm-before-send`: `summary` shows e.g. `Send 2 changes (~1.8k tokens) to 9router/some-model?`; `preview` shows the prompt buffer with `C-c C-c` to send and `q` to abort.
6. Spawn `oa chat …` with `sent-text` on stdin, `OA_API_KEY` in the process environment (resolved from the key source, never printed), and display the output buffer immediately.
7. Stream events into the output buffer (5.6). The user can keep editing the document meanwhile.
8. On `done`: per `oa-accept-mode`, ask `Accept response into the document?`, auto-accept, or wait.

### 5.6 Output buffer

- One buffer per project: `*oa: <project-name>*`, major mode `oa-output-mode` (derived from `org-mode`), read-only to the user (the package inserts with `inhibit-read-only`).
- Each request appends a section:

```org
* 2026-10-14 08:30 some-model @ 9router
:PROPERTIES:
:TOKENS_EST: 1840
:CHANGES: 2
:END:
…streamed text…
```

- Streaming is appended at the end; if point is at the end it follows the stream, otherwise point is left alone.
- **Header line:** `endpoint · model · state` where state is `idle`, `sending`, `streaming (N chars)`, `done`, `cancelled`, or `error: <kind>`.
- The window is shown with `display-buffer` and `oa-display-buffer-action` without stealing focus from the document, unless the user invoked `oa-show-output`.
- Errors are shown in the buffer as a final line plus an echo-area message; engine stderr goes to `*oa-log*` (secrets redacted).
- Previous responses remain in the buffer until the user clears them (`C-c C-l` in the output buffer).
- Process sentinel handles unexpected exit, partial last line, and bad JSON by reporting `error: bad_response` without crashing.

### 5.7 Cancel

`oa-cancel` interrupts the engine process (`interrupt-process`); the output shows `[cancelled]`; no commit happens; the delta remains pending. If the process does not exit within 2 s it is killed.

### 5.8 Accept, writeback, commit

`oa-accept` (and `C-c C-c` in the output buffer) performs one **atomic exchange commit**:

1. Normalize the response: shift all headlines so the shallowest becomes (entry level + 1); lines inside `#+BEGIN_…`/`#+END_…` blocks are never altered.
2. Insert under the `* Agent :agent:` headline (created at the end of the document if missing):

```org
** 2026-10-14 08:30 Response to: <first changed title or "update"> (some-model)
…normalized response…
```

   using buffer edits only, and never touching any node outside the agent subtree.
3. Run `oa snapshot -out <snapshot> -` with **`sent-text`** (the text captured at send time) on stdin, so edits the user made while the model was answering are not marked as delivered.

If the user never accepts, no snapshot is written and the same delta is sent next time. Accepting twice is a no-op with a message.

### 5.9 Menu and discoverability

`oa-menu` is a `transient` prefix showing: current endpoint, model, cache age, and project state (pending changes count when known), with the commands of 5.2 grouped as *Send*, *Model*, *Endpoint*, *Output*. `M-x oa-doctor` prints a diagnostic report: engine path and version, protocol compatibility, endpoint reachability, model count, key source resolution (found or not found, never the key), snapshot path.

### 5.10 Process and engine management

- Locate the engine with `executable-find`; if missing, a clear error: `Engine "oa" not found. Build it (cd oa-engine && make build) and set oa-engine-program.`
- On first use per session run `oa version -json`; require `protocol` equal to the client's expected value.
- `make-process` with `:connection-type 'pipe`, UTF-8 coding, separate stderr buffer, a filter that frames NDJSON by newline and tolerates partial lines.
- One in-flight request per project; the state machine is `idle → sending → streaming → done|error|cancelled → idle`.

### 5.11 Emacs acceptance criteria

All tests run in `emacs -Q --batch` using a **fake engine** (a script that replays fixtures from `test/fixtures/engine/`) unless marked E2E.

1. `oa-add-endpoint` with a fake engine returning two models stores the endpoint, caches the models, and writes a state file that contains **no key material**.
2. Validation failures per error kind (`auth`, `network`, `timeout`) produce distinct messages and the *edit / save anyway / discard* choice.
3. `oa-select-model` accepts a listed id and a free-text id; the choice persists across a simulated restart (re-reading the state file).
4. `oa-send` with a stream of three deltas yields the exact concatenated text in the output buffer, with the correct header-line state transitions.
5. Exit code 2 produces the "Nothing new" message and never opens a request.
6. A partial NDJSON line split across two process-filter calls is reassembled correctly; invalid JSON yields `error: bad_response`.
7. `oa-cancel` mid-stream results in `[cancelled]`, no snapshot written, and a pending delta on the next send.
8. `oa-accept` inserts a normalized entry under `* Agent :agent:`, leaves every other node byte-identical, and writes the snapshot from the send-time text (a test edits the buffer between send and accept and verifies that edit is still a delta afterwards).
9. Response with `* Heading` and `** Sub` inside the model text lands as `***`/`****` under a `**` entry; a headline-looking line inside a source block is untouched.
10. The key never appears in `ps`-visible arguments (the test inspects the spawned command line) and never in `*oa-log*`.
11. `byte-compile` with warnings as errors, `checkdoc`, and `package-lint` are clean.
12. **E2E** (integrator): real `oa` binary + `oa-mockrouter`: add endpoint, fetch models, send, accept, then "Nothing new to send".

---

## 6. The master agent

### 6.1 Mission

Own the whole project end to end: understand the specs, turn them into a task plan, delegate every piece of work to sub-agents with precise briefs, verify results independently, integrate across repos, and keep the human informed and in control of irreversible and contract-level decisions.

### 6.2 What the master does and does not do

**Does:**
- Maintain `.master/PLAN.org` as the single source of truth for task state.
- Write task briefs (8.2), spawn sub-agents, collect structured reports (8.3).
- Run the quality gates **itself** (never trusting a sub-agent's claim of "tests pass").
- Decide task order, parallelism, and when to escalate.
- Merge task branches into `main` after the gates pass.
- Keep the specs and the frozen-contract register (13) consistent.

**Does not:**
- Write production code or tests itself (it may edit `.master/` files and the specs, with human approval for spec changes).
- Change a frozen contract without human approval.
- Add dependencies, delete files wholesale, push to remotes, or use real credentials without human approval.
- Let a task's implementer also review it.

### 6.3 Startup procedure (every session)

1. Read `MASTER-SPEC.md` and `AGENTS.md`, load the repository registry (section 17) and run the registry check (17.10), then skim the Master digest of each active repo's `SPEC.md` (2.3). Read further sections only when a task needs them.
2. Read `.master/PLAN.org` and the latest files in `.master/reports/`.
3. In each repo: `git status`, `git log -5 --oneline`, current branches/worktrees.
4. Reconcile the ledger with reality (a task marked DOING with no branch is reset to TODO). Report the discrepancy list to the human.
5. Select the next ready tasks (dependencies satisfied, file scopes not overlapping with running tasks).

### 6.4 Planning rules

- **Contract first.** Before any cross-repo work, the protocol (section 4) is finalized and its fixtures exist (task C0). Emacs work proceeds against fixtures and the fake engine; it never waits for the real engine.
- **Smallest vertical slice.** Each task has acceptance criteria that can be verified mechanically and fits in one sub-agent session (roughly under 400 changed lines).
- **Parallelism.** Tasks in different repos with satisfied dependencies run concurrently. Tasks in the same repo run concurrently only in separate worktrees (`task/<id>`) and only if their declared file scopes do not overlap.
- **Risk first.** Unverified assumptions (A1, SSE behavior of 9router, `org-element` behavior if used) get an early spike task whose output is a fixture or a note.

### 6.5 Escalation and stop conditions

The master stops and asks the human (with a concrete question and options) when:
- a frozen contract would have to change;
- a review/fix loop exceeds **3 rounds** or a debugging attempt fails **2 times** on the same failure;
- a spec is ambiguous or self-contradictory (the architect agent proposes options first);
- a task needs a new dependency, a credential, network access beyond package download, or a destructive operation;
- two sub-agents' results conflict;
- the budget (time or tokens) the human set for the session is 80% used.

### 6.6 Workspace `AGENTS.md` (summary of the rules the master must carry)

1. The specs are the contract. Never "improve" a contract silently.
2. Briefs are self-contained; sub-agents do not share your memory.
3. Trust nothing unverified: run gates yourself, read diffs.
4. Separation of duties: implementer, tester, reviewer are different sub-agent runs.
5. Record every decision and every escalation in `PLAN.org`.
6. Local commits only; the human pushes.
7. When in doubt, ask. A short question is cheaper than a wrong merge.

### 6.7 Anti-patterns (the master must avoid)

Vague briefs ("implement the CLI"); accepting a green report without re-running; implementing "just this small part" itself; merging with unresolved review findings; weakening or deleting a failing test to get green; letting scope creep into a task; retrying the same prompt a third time instead of fixing the spec or the brief.

---

## 7. Sub-agent roles

Each role has a definition file in `.master/agents/<role>.md` (also mirrored into the harness's native subagent location, e.g. `.claude/agents/`). Tool permissions are enforced by the harness where possible and stated in the brief otherwise. Suggested model tier: strongest for architect, reviewer, and debugger; a mid-tier for implementers and testers, escalated if a task fails twice.

| Role | Writes code? | Tools | Output |
|---|---|---|---|
| `architect` | no (read-only) | read, search, run read-only commands | Design note or spec-ambiguity proposal with options |
| `implementer-go` | yes, in `oa-engine` only | read, edit, shell (build/test) | Code, tests, report |
| `implementer-elisp` | yes, in `oa-emacs` only | read, edit, shell (`emacs --batch`, make) | Code, tests, report |
| `tester` | **tests only**, never production code | read, edit under `test*`/`*_test.go`/`testdata`, shell | Added tests, fuzz cases, findings |
| `reviewer` | no (read-only) | read, search, run tests | Structured review verdict |
| `debugger` | yes, minimal fixes | read, edit, shell | Root cause, regression test, fix |
| `integrator` | cross-repo harness code only | read, shell across repos | E2E results, contract-sync result |

### 7.1 `architect`

Invoked for ambiguity, contract proposals, and spikes. Reads specs and code, returns **options with trade-offs and a recommendation**, never edits. Output template in 16.3.

### 7.2 `implementer-go` / `implementer-elisp`

Implements exactly one task per the brief. Obeys the repo's `AGENTS.md`, adds tests with the code, runs the repo gates, and reports. Must not touch files outside the task's declared scope; if it believes it must, it stops and says why.

### 7.3 `tester`

Works in two modes:
- **Test-first** (for contract-critical tasks: parser, delta, events, NDJSON framing, writeback): writes failing tests from the spec's acceptance criteria *before* the implementer starts. The implementer must make them pass **without editing them**; if a test is wrong, the implementer reports it instead.
- **Adversarial** (after implementation): reads the diff and the spec, writes extra tests for edge cases, error paths, fuzz seeds, and Unicode (Persian and CJK text, emoji); reports any failure as a finding with a minimal reproduction. The tester does not fix production code.

### 7.4 `reviewer`

Read-only, fresh context, never the implementer of that task. Reviews the diff against the brief, the specs, and the checklist (9.3). Returns a verdict: `APPROVE`, `REQUEST_CHANGES` (with blocking findings), or `ESCALATE` (spec problem). Findings are classified `blocking`, `should-fix`, `nit`. Approval requires zero blocking findings.

### 7.5 `debugger`

Given a failing gate, an e2e failure, or a reviewer-reported defect: reproduce, write the **smallest failing test**, find the root cause, apply the **minimal fix**, run all gates. Must explain the root cause in the report. Must not weaken tests or add workarounds that hide the failure.

### 7.6 `integrator`

Runs cross-repo checks no single-repo agent can: fixture sync (`oa-emacs/test/fixtures/engine` equals `oa-engine/testdata/protocol`), building the real engine and running the Emacs E2E suite against `oa-mockrouter`, version/protocol compatibility, and README accuracy (the commands documented actually work). May write only harness code (e.g. an `e2e/` script area in `oa-emacs/test/e2e/`) and reports.

---

## 8. Task lifecycle, briefs, reports

### 8.1 Task states

`TODO → DOING → TESTING → REVIEW → (FIX → REVIEW)* → DONE`, with `BLOCKED` and `CANCELLED` as side states.

```
#+TODO: TODO(t) DOING(d) TESTING(e) REVIEW(r) FIX(f) | DONE(x) BLOCKED(b) CANCELLED(c)
```

Standard path for a task:

1. **Brief** written (master).
2. *(Contract-critical only)* `tester` test-first.
3. `implementer-*` implements → report.
4. **Master runs the repo gates** itself (9.1). Fail → `debugger` or back to the implementer (counts toward the loop limit).
5. `tester` adversarial pass → findings fixed (implementer or debugger).
6. `reviewer` → verdict. `REQUEST_CHANGES` → `FIX` and back to step 3/5 (max 3 rounds).
7. Master merges `task/<id>` into `main` (fast-forward or squash), re-runs gates on `main`.
8. Integration gate (9.2) when the task touches a contract or completes a milestone.
9. Ledger updated: state `DONE`, report links, commit hash.

### 8.2 Task brief template (self-contained)

```markdown
# Task <ID>: <title>
Repo: <oa-engine|oa-emacs>   Branch/worktree: task/<ID>   Role: <role>
## Goal
<one paragraph: what and why>
## Read first
<repo spec sections by path and number, e.g. oa-engine/SPEC.md §5.4-5.7, MASTER-SPEC.md §4.3. Cite, never paste spec text.>
## Scope (files you may create or change)
<paths or globs; everything else is off limits>
## Out of scope
<what NOT to do>
## Frozen (do not change)
<contracts, types, JSON fields, CLI flags, keymap, defcustoms relevant here>
## Acceptance criteria
<numbered, mechanically checkable, copied from the spec>
## Gates to run before reporting
<exact commands>
## Report format
Use the template in MASTER-SPEC.md 16.2.
```

### 8.3 Report template

See 16.2. Every report states: what changed (files), tests added, gate output (**pasted**, not summarized), deviations from the brief, spec ambiguities found, and open questions. Reports are saved to `.master/reports/<ID>-<role>-<n>.md`.

### 8.4 Ledger (`.master/PLAN.org`)

Org headlines with properties; the master edits it after every state change. Seed in 16.4. This file doubles as the first real test input for the engine.

---

## 9. Quality gates and testing strategy

### 9.1 Repo gates (run by the master, per task)

| Repo | Commands (all must pass) |
|---|---|
| `oa-engine` | `make ci` = `gofmt -s` check, `go vet ./...`, `go test -race ./...` (golden files unchanged unless the brief says otherwise); `make fuzz` (short) for parser tasks |
| `oa-emacs` | `make test` (ERT in batch), `make compile` (byte-compile, warnings are errors), `make lint` (`checkdoc`, `package-lint`) |

Additionally for every task: the diff touches only files in the declared scope; no new dependencies; no TODO/FIXME left without a ledger entry; goldens changed only when the brief authorizes it, and then each changed golden is listed in the report with the reason.

### 9.2 Integration gate (integrator)

1. Fixture sync check passes.
2. Engine built from `main`; `oa version -json` protocol matches the Emacs client's expectation.
3. Emacs E2E suite against `oa-mockrouter` passes (add endpoint, models, send, stream, cancel, accept, nothing-new).
4. Contract diff: any change in `testdata/protocol/` since the last integration is either backward compatible or accompanied by a schema/protocol bump approved by the human.

### 9.3 Reviewer checklist

- Satisfies every acceptance criterion; no scope creep; frozen items untouched.
- Tests are meaningful (not tautological, not asserting implementation details), cover error paths, and fail without the change.
- No global mutable state, no panics reachable from input (Go); no side effects at load time, no global keybindings outside the mode (Elisp).
- Secrets: key only via env, never in argv, logs, state file, or fixtures.
- Determinism: map order, timestamps, locale do not leak into outputs.
- Concurrency and cancellation: goroutine leaks, context cancellation (Go); process sentinels, filter reentrancy (Elisp).
- Docs: docstrings, package comments, README commands match behavior.
- Style: `gofmt`, idiomatic errors (`%w`); Elisp conventions (`oa--` private prefix, `lexical-binding`).

### 9.4 Test strategy summary

| Layer | Technique |
|---|---|
| Engine units | Table-driven tests, golden files (`-update` is human-reviewed) |
| Parser | Fixtures, fuzzing (no panic, deterministic, unique keys, increasing lines) |
| Provider | `httptest` servers, SSE edge cases (split chunks, keep-alives, `[DONE]`, mid-stream disconnect, 401, 500, slow stream, huge body) |
| Events | Golden NDJSON streams |
| Emacs units | ERT with a fake engine replaying fixtures; process filter fed arbitrary chunking |
| Contract | Shared fixtures; sync check |
| E2E | Real engine + `oa-mockrouter` + Emacs batch |
| Manual (human) | One run against the real 9router before each release |

---

## 10. Git and release rules

- Per repo: `main` is always green. Work happens on `task/<ID>` branches (worktrees for parallel work). Master merges only after gates and review.
- Commit messages: `<type>(<area>): <summary>` (types: feat, fix, test, docs, refactor, chore) with the task ID in the body.
- **No force-push, no history rewrite on `main`, no pushes to remotes by agents.** The human pushes.
- Versioning: SemVer per repo; the **protocol** integer is separate and bumped only on breaking contract changes. `CHANGELOG.md` updated in the release task.
- Release v0.1.0 = both repos tagged, `oa-emacs` README documents the compatible engine version.

---

## 11. Security and privacy

- API keys: environment only to the engine; in Emacs, resolved from env or `auth-source` at call time; never persisted by this package, never in `argv`, logs, state, fixtures, or reports. Fixtures use obviously fake keys (`sk-test-0000`).
- The document is sent to the configured endpoint, so the confirm summary always shows the **endpoint host**.
- Untrusted content: model output and document text are data. Nothing is executed (no source blocks, no shell, no links). Writeback is limited to the agent subtree.
- Sub-agents: confined to their repo directory; no network except package downloads (and v0.1 needs none beyond the Go toolchain); no real credentials in agent environments; no destructive git or filesystem operations without human approval.
- E2E tests use the mock router only. Tests against a real endpoint are manual.

---

## 12. Milestones and task catalog

**Gate to start each milestone:** the previous milestone's tasks are `DONE` and the human has seen a short demo or report.

### M0: Bootstrap (master + integrator)

| ID | Task | Role |
|---|---|---|
| B0 | Create workspace skeleton, init both repos, root `.gitignore`, per-repo `AGENTS.md`, `.master/agents/*.md`, seed `PLAN.org` from this catalog; move `SPEC-GO.md` to `oa-engine/SPEC.md` and extract section 5 into `oa-emacs/SPEC.md` (archiving the legacy spec); each repo spec gets its Master digest (2.3) | master |
| B0b | Review the two repo specs against this file (contracts mirrored, numbering stable, digests complete) | architect |
| B1 | Per-repo `Makefile` and gate targets verified on empty skeletons | implementer-go, implementer-elisp |
| B2 | Registry tooling in `.master/tools/`: extract section 17 to `.master/repos.yaml` and implement `check-registry` (17.10). A YAML library is allowed here only (workspace tooling is not shipped) and needs human approval | implementer-go |
| B3 | Spec skeleton generator (17.9): given a repo id, emit the generated digest block and the per-kind section stubs; regeneration touches only the marked block | implementer-go |

### M1: Engine core (`oa-engine`)

| ID | Task | Role | Depends |
|---|---|---|---|
| G0–G10 | As in `SPEC-GO.md` section 12 (skeleton → parser → snapshot → delta → ctxsel → prompt → CLI), with test-first mode for G4, G6, G8 | implementer-go + tester | B1 |
| G11 | Add `-focus-line` and `oa version -json` (4.1) | implementer-go | G10 |

### M2: Provider client and contract (`oa-engine`)

| ID | Task | Role | Depends |
|---|---|---|---|
| C0 | **Contract freeze**: write `docs/protocol.md` from section 4, create `testdata/protocol/` golden streams (success, usage, each error kind, cancel, partial split chunks) | architect then implementer-go | B1 |
| E1 | Spike A1: capture a real `/models` response from 9router (human supplies, secrets removed) and SSE sample; save as fixtures; record findings | human + architect | C0 |
| E2 | `internal/events` encoder + `oa-mockrouter` (models and streaming chat, configurable faults) | implementer-go (test-first) | C0 |
| E3 | `internal/provider`: models list, streaming chat, error mapping, bounded bodies, redirect policy | implementer-go (test-first) | E2 |
| E4 | `oa models` and `oa chat` commands, cancellation, exit codes, insecure-http refusal | implementer-go | E3, G11 |

### M3: Emacs client (`oa-emacs`)

Starts after C0 (works against fixtures and a fake engine); real-engine integration happens at X1.

| ID | Task | Role | Depends |
|---|---|---|---|
| L0 | Skeleton, Makefile, fake engine script replaying fixtures, vendored fixtures + `sync-fixtures` | implementer-elisp | C0 |
| L1 | `oa-config`: endpoints data model, state file (atomic, lazy), key-source resolution | implementer-elisp (test-first) | L0 |
| L2 | `oa-engine`: locate binary, version check, process spawn, NDJSON framing, cancel | implementer-elisp (test-first) | L0 |
| L3 | Models: refresh, cache with TTL, `oa-select-model` with free-text fallback | implementer-elisp | L1, L2 |
| L4 | Endpoint UX: add (with validation), select, edit, remove | implementer-elisp | L3 |
| L5 | Send flow and output buffer (5.5, 5.6), confirm modes | implementer-elisp | L2, L3 |
| L6 | Cancel and error handling (5.7, sentinel edge cases) | implementer-elisp | L5 |
| L7 | `oa-doc`: capture, response normalization, writeback, atomic commit (5.8) | implementer-elisp (test-first) | L2 |
| L8 | `oa-mode`, keymap, `oa-maybe-enable`, `oa-menu` (transient), `oa-doctor` | implementer-elisp | L4, L5, L7 |
| L9 | README.org (setup in five lines, keys, troubleshooting), checkdoc/package-lint clean | implementer-elisp | L8 |

### M4: Integration and release

| ID | Task | Role | Depends |
|---|---|---|---|
| X1 | E2E suite: real engine + mock router + Emacs batch (acceptance 12) | integrator | E4, L8 |
| X2 | Manual acceptance run against the real 9router (human), fixes via debugger | human + debugger | X1 |
| X3 | CHANGELOGs, version tags, compatibility table in READMEs | master + implementer | X2 |

### M5+ (future, not in v0.1)

Agent tool loop (read/grep/apply-patch with diff review), multi-file loader and link roles, token budgeting, MELPA packaging.

Other repos from section 17 come online when their trigger fires (17.8). The expected order is: `oa-templates`, `oa-docs`, and `oa-eval` (after the agent loop exists); then the extractions `oa-protocol` and `oa-testkit`; then any further clients, the `oa-agent` split, and `oa-packaging`. Each activation is its own task set, started by an architect note and approved by the human.

---

## 13. Frozen contracts register

Changing anything here requires: architect proposal → human approval → schema/protocol bump → fixtures and both repos updated in the same milestone.

| Contract | Defined in |
|---|---|
| Node, Document, Snapshot, Delta, Prompt JSON shapes and `schema: 1` | `SPEC-GO.md` 5.2, 6.1, 7.1, 8.1 |
| Key rules and hash rules | `SPEC-GO.md` 5.6, 5.7 |
| Prompt structure and tags | `SPEC-GO.md` 8.2–8.6 |
| CLI commands, flags, exit codes | `SPEC-GO.md` 9, this file 4.1, 4.4 |
| Event stream, error kinds, `protocol: 1` | this file 4.3 |
| `OA_API_KEY` env var convention | this file 4.5 |
| Emacs command names, default keymap, `defcustom` names, state file shape | this file 5.2–5.4 |
| Commit semantics (accept = writeback + snapshot from send-time text) | this file 5.8 |
| Registry schema, connection rules, contract ownership (who owns which contract) | this file 17.4, 17.6, 17.5 |

---

## 14. Definition of done for v0.1.0

The human can, on a clean Emacs:

1. Build the engine (`make build`), `(require 'oa)`, run `M-x oa-add-endpoint` with the 9router URL and see `N models found`.
2. `M-x oa-select-model` (or `C-c a m`) and choose a model; the header line shows it.
3. In `project.org`, write a note and press `C-c a s`; confirm; see the answer stream into a side window.
4. Press `C-c C-c` in the output buffer: the answer appears under `* Agent :agent:` with sane heading levels; pressing `C-c a s` again says there is nothing new.
5. Cancel a long answer with `C-c a k`; the same delta is offered again afterwards.
6. Restart Emacs: endpoint and model are remembered; no key exists in any file this package wrote.
7. All gates in 9 are green on both repos, and E2E passes.

---

## 15. Open decisions for the human

| Question | Default |
|---|---|
| Names (`oa`, `oa-engine`, `oa-emacs`, prefix `oa-`) | as written |
| Workspace root as its own repo | yes (specs + `.master/` only) |
| Default prefix key | `C-c a` (opt-in minor mode) |
| `oa-confirm-before-send` default | `summary` (a relaxation of the "preview always" principle; `C-c a p` shows the full prompt) |
| Accept mode default | `ask` |
| Commit on accept (not on send) | yes, so a discarded answer leaves the delta pending |
| Engine keeps no endpoint config; Emacs owns it | yes |
| API keys via env / auth-source only | yes |
| Sub-agent model tiers | strongest for architect/reviewer/debugger |
| Who pushes to GitHub | the human |
| Licence | MIT (suggested) |
| Registry format | YAML block in section 17 (readable by humans and agents); workspace tools may use a YAML library |
| Agent runtime location | inside `oa-engine` (`internal/agent`, `internal/tools`) until the `oa-agent` split trigger fires |
| When to extract `oa-protocol` and `oa-testkit` | at the first of: a second client is approved, or the agent runtime is split |
| Candidate repos (`oa-nvim`, `oa-vscode`, `oa-agent`, `oa-packaging`) | none are created until you approve their activation |

---

## 16. Appendix: templates

### 16.1 Role definition file (skeleton, `.master/agents/<role>.md`)

```markdown
---
name: <role>
description: <when the master should use this role>
tools: <allowed tools>
---
You are the <role> for the oa project.
Rules:
1. Read the brief completely, then your repo's SPEC.md digest and the sections the brief cites. Do nothing else first.
2. Stay inside the declared scope. If you need something outside it, stop and report why.
3. <role-specific rules from section 7>
4. Finish with a report in the required template. Paste real command output.
```

### 16.2 Report template

```markdown
# Report <task-id> / <role> / run <n>
## Outcome
DONE | PARTIAL | BLOCKED  (one sentence)
## Changes
- <file>: <what and why>
## Tests
- Added: <list>   Result: <pass/fail counts>
## Gate output
<pasted output of every gate command>
## Deviations from the brief
<none | list with reasons>
## Spec ambiguities / risks found
<none | list>
## Open questions
<none | list>
```

### 16.3 Architect note template

```markdown
# Design note <topic>
## Problem
## Options (each: description, pros, cons, cost)
## Recommendation (and what would change it)
## Contract impact (frozen items touched? yes/no, which)
## Questions for the human
```

### 16.3b Review verdict template

```markdown
# Review <task-id>
Verdict: APPROVE | REQUEST_CHANGES | ESCALATE
## Blocking findings
1. <file:line> <problem> <why it matters> <suggested direction>
## Should-fix
## Nits
## Checklist (9.3): <item: ok / issue ref>
```

### 16.4 `PLAN.org` seed

```org
#+TITLE: oa master plan
#+TODO: TODO(t) DOING(d) TESTING(e) REVIEW(r) FIX(f) | DONE(x) BLOCKED(b) CANCELLED(c)

* Context :ctx:
** Specs
MASTER-SPEC.md > oa-engine/SPEC.md and oa-emacs/SPEC.md (cite, never paste). Frozen contracts: MASTER-SPEC.md section 13.
** Rules
Local commits only. Master runs gates itself. Reviewer is never the implementer.

* Milestones
** M0 Bootstrap
*** TODO B0 Workspace skeleton and repos
:PROPERTIES:
:REPO: workspace
:ROLE: master
:DEPENDS:
:SCOPE: .master/** AGENTS.md .gitignore
:END:
*** TODO B1 Per-repo Makefiles and gates
:PROPERTIES:
:REPO: both
:ROLE: implementer-go, implementer-elisp
:DEPENDS: B0
:END:
** M1 Engine core
*** TODO G0 Module skeleton
:PROPERTIES:
:REPO: oa-engine
:ROLE: implementer-go
:DEPENDS: B1
:SCOPE: go.mod Makefile AGENTS.md cmd/** internal/** testdata/**
:END:
(… one headline per task in section 12 …)

* Agent :agent:
```

### 16.5 First master prompt

> You are the master agent for the oa project. Read `MASTER-SPEC.md`. The engine spec (`SPEC-GO.md`) and the legacy Elisp-first spec (`SPEC.md`) are still at the workspace root until you move them in B0. Run the startup procedure (6.3), adapting step 1 to that. Execute **B0**: create the workspace skeleton from section 2, initialize both repos, write the per-repo `AGENTS.md` files and the role definition files from section 7 and 16.1, seed `.master/PLAN.org` from the task catalog in section 12, and extract the repository registry (section 17) into `.master/repos.yaml`. Do not write product code. When done, report what exists, list any ambiguities you found in the specs, and propose the briefs for **B1** and **C0**. Wait for my approval before spawning any sub-agent.

---

## 17. Repository registry

> Appended after the appendix so earlier section numbers stay stable. This section is the **source of truth** for which repos exist (or will ever exist), what each one is for, how they connect, and who owns which contract.

### 17.1 Purpose and uses

1. **Whole-project understanding.** The master loads the registry at startup (6.3) and always knows every repo, its status, its neighbors, and the contracts between them.
2. **Impact analysis.** From the registry the master computes who is affected by a change (17.7) and orders the work accordingly.
3. **Spec generation.** The Master digest of each repo spec, and the skeleton of its numbered sections, are generated from this data (17.9), so repo specs start consistent with the whole.
4. **Validation.** Workspace tooling checks the registry for consistency (17.10) before the master spawns work.

### 17.2 The big picture (illustrative; the registry in 17.5 wins on any difference)

```
                           oa-workspace   (master agent, plan, registry, specs)
                                 │  coordinates every repo below
   ┌─────────────────────────────┼──────────────────────────────────────────────┐
   │                             │                                              │
 CLIENTS                       CORE                                    SUPPORT
 oa-emacs  ── runtime ──►   oa-engine  ◄── runtime ── oa-eval      oa-protocol  (contracts, schemas, fixtures)
 oa-nvim   ── runtime ──►   (parser, delta,            (evaluation)  oa-testkit   (mock router, fake engine, e2e)
 oa-vscode ── runtime ──►    prompt, provider,                        oa-templates (starter org documents)
     │                       CLI, events)                             oa-docs      (manual, tutorials)
     │ optional runtime          ▲                                    oa-packaging (distro packages, releases)
     ▼                           │ build
 oa-agent (split-off agent runtime) ─┘

 contracts flow from their owner to consumers: engine-protocol, document-json-shapes,
 org-project-conventions, emacs-user-interface, api-key-convention (see 17.5)
```

Reading guide: **clients** talk to the engine only through the runtime contract; **support** repos serve the others (test tools, fixtures, content, docs, packaging); nothing depends on a client except docs and packaging.

### 17.3 Status lifecycle and kinds

| Status | Meaning |
|---|---|
| `active` | Exists as a directory and git repo, has a spec, gates, and tasks in `PLAN.org` |
| `planned` | Known to be needed; has a `trigger`; no directory yet |
| `candidate` | Might be needed; has a `trigger`; needs explicit human approval to become `planned` |
| `archived` | No longer developed; kept for history |

Transitions: `candidate → planned → active → archived`. Each transition is a human-approved change to this section.

Kinds: `meta` (coordination), `engine` (core logic and CLI), `client` (editor integration), `contract` (schemas and fixtures), `testing` (shared test tools), `content` (templates and examples), `docs`, `eval`, `packaging`.

### 17.4 Registry schema

**Repo entry fields**

| Field | Required | Meaning |
|---|---|---|
| `id` | yes | Unique identifier, equal to the directory name |
| `dir` | yes | Directory relative to the workspace root (`"."` for the workspace itself) |
| `status`, `kind` | yes | As in 17.3 |
| `languages` | yes | Implementation languages |
| `introduced` | yes | Milestone that creates it (`M0`…`M6`, or a text condition for later ones) |
| `purpose` | yes | One or two sentences |
| `owns` | yes | Responsibilities the repo is the only home of |
| `layout` | active: yes | Top-level files and directories with their role |
| `exposes` | yes | Binaries, packages, contracts, and content it provides |
| `roadmap_scope` | no | Planned future content of the repo, so the master does not create a repo for it early |
| `spec` | yes | Path of its spec file |
| `gates` | active: yes | Exact commands that must pass |
| `release` | yes | Versioning scheme, artifact, compatibility rule |
| `agents` | yes | Sub-agent roles normally assigned to its tasks |
| `trigger` | planned/candidate: yes | The condition that activates it |
| `notes` | no | Rationale, pitfalls |

**Edge fields** (`edges` list): `from` depends on `to`; `type` is one of `runtime` (a process or network boundary), `build` (compile-time import), `test` (used only by tests), `docs` (documented or verified by docs), `release` (packaged or shipped by); `via` describes the mechanism; `optional: true` marks edges that may be absent at runtime.

**Contract fields** (`contracts` list): `id`, `owner` (exactly one repo), `transfers_to` (optional future owner), `version_field` and `current`, `consumers`, `defined_in`.

**Derived, not stored:** `consumes` and `consumed_by` per repo, contract edges (consumer → owner for every contract), the impact set of a change (17.7), and whether an edge is live (both ends `active`).

### 17.5 The registry (canonical)

<!-- registry:begin -->
```yaml
registry_version: 1
project: oa
workspace_root: oa-workspace

repos:
  - id: oa-workspace
    dir: "."
    status: active
    kind: meta
    languages: [markdown, org, yaml]
    introduced: M0
    purpose: "Coordination home: master spec, plan ledger, role definitions, registry, workspace tools. Holds no product code."
    owns: ["MASTER-SPEC.md", ".master/PLAN.org", ".master/agents/", ".master/tools/", ".master/repos.yaml (generated copy of this registry)"]
    layout: ["MASTER-SPEC.md: master spec", "AGENTS.md: rules for the master", ".master/: plan, reports, agent roles, tools"]
    exposes: ["registry", "task ledger", "repo spec generator"]
    spec: "MASTER-SPEC.md"
    gates: [".master/tools/check-registry"]
    release: {scheme: none}
    agents: [master, architect, integrator]
    notes: "May be its own git repo that tracks only specs and .master/; the other repos are git-ignored, not submodules."

  - id: oa-engine
    dir: oa-engine
    status: active
    kind: engine
    languages: [go]
    introduced: M1
    purpose: "The Go core: parses org project documents, computes deltas, selects context, assembles prompts, calls OpenAI-compatible endpoints, streams events, and ships as the oa CLI."
    owns: ["org parsing", "snapshot", "delta", "context selection", "prompt assembly", "OpenAI-compatible provider client", "event stream", "the oa CLI"]
    layout: ["cmd/oa/: CLI entry", "cmd/oa-mockrouter/: test mock router (until oa-testkit exists)", "internal/org/", "internal/snapshot/", "internal/delta/", "internal/ctxsel/", "internal/prompt/", "internal/provider/", "internal/events/", "internal/cli/", "testdata/: fixtures, goldens, protocol streams", "docs/protocol.md"]
    exposes: ["binary: oa", "contract: engine-protocol", "contract: document-json-shapes", "contract: org-project-conventions"]
    roadmap_scope: ["internal/loader (multi-file documents, link roles)", "internal/agent and internal/tools (tool loop, permissions)", "token budgeting", "optional MCP client"]
    spec: "oa-engine/SPEC.md"
    gates: ["make ci", "make fuzz (parser tasks)"]
    release: {scheme: semver, artifact: "oa binary (go install, GitHub release)", compat: "reports protocol N via oa version -json"}
    agents: [implementer-go, tester, reviewer, debugger]
    notes: "Standard library only in shipped code. Single binary by design; the agent runtime lives here until the oa-agent trigger fires."

  - id: oa-emacs
    dir: oa-emacs
    status: active
    kind: client
    languages: [elisp]
    introduced: M3
    purpose: "The Emacs client: manage endpoints and keys, fetch and choose models, send with a key binding, stream output into its own buffer, accept answers into the project document."
    owns: ["endpoint, key-source and model management", "send, stream and cancel flow", "output buffer", "response normalization, writeback and atomic commit", "key bindings, menu, doctor"]
    layout: ["oa.el: entry point and oa-mode", "oa-config.el: endpoints, state, auth", "oa-engine.el: process and NDJSON", "oa-ui.el: output buffer and menu", "oa-doc.el: capture, writeback, commit", "test/: ERT, fake engine, vendored fixtures", "docs/legacy-elisp-first.md"]
    exposes: ["package: oa", "contract: emacs-user-interface"]
    roadmap_scope: ["agent-mode UI (permissions, diff review)", "multi-file project support", "MELPA recipe"]
    spec: "oa-emacs/SPEC.md"
    gates: ["make test", "make compile", "make lint"]
    release: {scheme: semver, artifact: "Elisp package (git clone; MELPA later)", compat: "requires engine protocol N"}
    agents: [implementer-elisp, tester, reviewer, debugger]
    notes: "Built-in Emacs libraries only (29.1+). Works without oa-agent; agent features appear only when it is present."

  - id: oa-protocol
    dir: oa-protocol
    status: planned
    kind: contract
    languages: [markdown, json]
    introduced: M6
    purpose: "Language-neutral home of the engine-client contract: protocol documentation, JSON Schemas, golden fixtures."
    owns: ["protocol documentation", "JSON Schemas", "golden fixtures"]
    layout: ["docs/protocol.md", "schemas/", "fixtures/"]
    exposes: ["contract: engine-protocol", "contract: document-json-shapes"]
    spec: "oa-protocol/SPEC.md"
    gates: ["every fixture validates against its schema", "schemas are valid JSON Schema"]
    release: {scheme: "semver; a major version equals a protocol version", artifact: "tagged schemas and fixtures"}
    agents: [architect, tester, reviewer]
    trigger: "A second client repo is approved, or the agent runtime is split out of oa-engine, whichever comes first. Until then the contract lives in oa-engine (docs/protocol.md, testdata/protocol/)."
    notes: "On activation, ownership of the two contracts above transfers from oa-engine. It has no dependency on any other repo; conformance runs live in the consumers (engine produces the fixtures, clients handle them)."

  - id: oa-testkit
    dir: oa-testkit
    status: planned
    kind: testing
    languages: [go]
    introduced: M6
    purpose: "Shared test tooling: an OpenAI-compatible mock router with fault injection, a fake engine that replays protocol fixtures, and an end-to-end runner."
    owns: ["oa-mockrouter", "oa-fake-engine", "e2e harness"]
    layout: ["cmd/oa-mockrouter/", "cmd/oa-fake-engine/", "e2e/"]
    exposes: ["binary: oa-mockrouter", "binary: oa-fake-engine", "e2e harness"]
    spec: "oa-testkit/SPEC.md"
    gates: ["make ci"]
    release: {scheme: semver, artifact: "binaries for CI use"}
    agents: [implementer-go, tester, reviewer]
    trigger: "A second repo needs the mock router or the fake engine (a second client, oa-eval, or the agent split). Until then they live in oa-engine (cmd/oa-mockrouter) and oa-emacs (test/ fake engine)."

  - id: oa-templates
    dir: oa-templates
    status: planned
    kind: content
    languages: [org]
    introduced: M5
    purpose: "Starter project documents and conventions: ready-made project.org files by stack, with ctx and agent sections and example task trees."
    owns: ["project.org templates (Go, Elisp, Python, embedded C)", "ctx and agent section conventions", "example task trees"]
    layout: ["templates/", "examples/"]
    exposes: ["content: org templates"]
    spec: "oa-templates/SPEC.md"
    gates: ["every template parses with oa parse without warnings", "oa prompt succeeds on every template with a sample snapshot"]
    release: {scheme: semver, artifact: "template set"}
    agents: [architect, reviewer]
    trigger: "After v0.1.0, when real use shows which structures work. The workspace's own PLAN.org seeds the first template."

  - id: oa-docs
    dir: oa-docs
    status: planned
    kind: docs
    languages: [markdown, org]
    introduced: M5
    purpose: "User manual, tutorials, architecture overview, troubleshooting, rendered protocol reference."
    owns: ["manual", "tutorials", "reference pages"]
    layout: ["manual/", "tutorials/", "reference/"]
    exposes: ["static documentation site"]
    spec: "oa-docs/SPEC.md"
    gates: ["link check", "every documented command is executed by the integrator", "no secret-looking strings"]
    release: {scheme: continuous, artifact: "static site"}
    agents: [architect, integrator, reviewer]
    trigger: "After v0.1.0. Until then each repo's README is the minimum documentation."

  - id: oa-eval
    dir: oa-eval
    status: planned
    kind: eval
    languages: [go]
    introduced: M5
    purpose: "Reproducible evaluation harness: golden project tasks run against mock or real models to measure context-selection strategies, prompt formats, and agent success rates."
    owns: ["task suite format", "metrics and reports", "the oa-eval binary"]
    layout: ["cmd/oa-eval/", "tasks/", "reports/"]
    exposes: ["binary: oa-eval", "task suite format"]
    spec: "oa-eval/SPEC.md"
    gates: ["make ci", "deterministic mock mode passes", "no real keys in the repo"]
    release: {scheme: semver, artifact: "oa-eval binary"}
    agents: [implementer-go, tester, reviewer]
    trigger: "The agent loop exists (M5), so there is behavior worth measuring. Before that, golden tests in oa-engine suffice."

  - id: oa-nvim
    dir: oa-nvim
    status: candidate
    kind: client
    languages: [lua]
    introduced: "after v1.0"
    purpose: "Neovim client with the same capabilities as oa-emacs, over the same engine protocol."
    owns: ["Neovim UI and keymaps", "endpoint and model management for Neovim"]
    exposes: ["Neovim plugin"]
    spec: "oa-nvim/SPEC.md"
    release: {scheme: semver, artifact: "Neovim plugin", compat: "requires engine protocol N"}
    agents: [implementer-lua, tester, reviewer]
    trigger: "oa-protocol is active, v1.0 has shipped, and the human wants Neovim support."
    notes: "The implementer-lua role file does not exist yet; it is written (from the 16.1 skeleton) as part of the activation tasks."

  - id: oa-vscode
    dir: oa-vscode
    status: candidate
    kind: client
    languages: [typescript]
    introduced: "after v1.0"
    purpose: "VS Code client with the same capabilities as oa-emacs, over the same engine protocol."
    owns: ["VS Code UI and commands", "endpoint and model management for VS Code"]
    exposes: ["VS Code extension"]
    spec: "oa-vscode/SPEC.md"
    release: {scheme: semver, artifact: "VS Code extension", compat: "requires engine protocol N"}
    agents: [implementer-ts, tester, reviewer]
    trigger: "oa-protocol is active, v1.0 has shipped, and the human wants VS Code support."
    notes: "The implementer-ts role file does not exist yet; it is written (from the 16.1 skeleton) as part of the activation tasks."

  - id: oa-agent
    dir: oa-agent
    status: candidate
    kind: engine
    languages: [go]
    introduced: "when triggered"
    purpose: "Agent runtime split out of oa-engine: tool loop, tools, permissions, diff-review protocol. Ships its own binary, oa-agent, built on oa-engine's public packages."
    owns: ["tool loop", "tools", "permission model", "agent events"]
    exposes: ["binary: oa-agent", "contract extension: agent events"]
    spec: "oa-agent/SPEC.md"
    release: {scheme: semver, artifact: "oa-agent binary", compat: "requires oa-engine library version N and protocol N"}
    agents: [implementer-go, tester, reviewer, debugger]
    trigger: "The agent code needs third-party dependencies, or its release cadence diverges from the engine, or internal/agent grows beyond roughly 15k lines. Requires oa-engine to promote stable packages from internal/ to pkg/ under SemVer."
    notes: "Until the trigger fires, develop it as internal/agent and internal/tools inside oa-engine. The oa binary then stays prompt and provider only."

  - id: oa-packaging
    dir: oa-packaging
    status: candidate
    kind: packaging
    languages: [shell, yaml, nix]
    introduced: "when triggered"
    purpose: "Distribution: Debian package, Nix flake, Homebrew tap, AUR, release automation."
    owns: ["packaging recipes", "release automation"]
    exposes: ["distribution packages"]
    spec: "oa-packaging/SPEC.md"
    release: {scheme: semver, artifact: "distribution packages"}
    agents: [implementer-go, reviewer]
    trigger: "The first release meant for users beyond go install and git clone."

edges:
  - {from: oa-emacs,     to: oa-engine,   type: runtime, via: "subprocess: org text on stdin, flags, env OA_API_KEY; JSON or NDJSON on stdout"}
  - {from: oa-emacs,     to: oa-testkit,  type: test,    via: "fake engine and mock router for tests and E2E"}
  - {from: oa-emacs,     to: oa-agent,    type: runtime, via: "optional agent mode over extended protocol events", optional: true}
  - {from: oa-engine,    to: oa-testkit,  type: test,    via: "mock router in provider tests"}
  - {from: oa-eval,      to: oa-engine,   type: runtime, via: "black-box use of the oa binary"}
  - {from: oa-eval,      to: oa-testkit,  type: test,    via: "mock router for deterministic runs"}
  - {from: oa-templates, to: oa-engine,   type: test,    via: "templates validated with oa parse and oa prompt"}
  - {from: oa-docs,      to: oa-engine,   type: docs,    via: "CLI reference; commands executed by the integrator"}
  - {from: oa-docs,      to: oa-emacs,    type: docs,    via: "user interface reference"}
  - {from: oa-docs,      to: oa-protocol, type: docs,    via: "rendered protocol reference"}
  - {from: oa-docs,      to: oa-templates, type: docs,   via: "template catalog"}
  - {from: oa-nvim,      to: oa-engine,   type: runtime, via: "same subprocess protocol as oa-emacs"}
  - {from: oa-nvim,      to: oa-testkit,  type: test,    via: "fake engine and mock router"}
  - {from: oa-vscode,    to: oa-engine,   type: runtime, via: "same subprocess protocol as oa-emacs"}
  - {from: oa-vscode,    to: oa-testkit,  type: test,    via: "fake engine and mock router"}
  - {from: oa-agent,     to: oa-engine,   type: build,   via: "Go import of promoted public packages"}
  - {from: oa-packaging, to: oa-engine,   type: release, via: "packages the oa binary"}
  - {from: oa-packaging, to: oa-emacs,    type: release, via: "packages the Elisp package"}
  - {from: oa-packaging, to: oa-agent,    type: release, via: "packages oa-agent", optional: true}

contracts:
  - id: engine-protocol
    owner: oa-engine
    transfers_to: oa-protocol
    version_field: "protocol (oa version -json)"
    current: 1
    consumers: [oa-emacs, oa-testkit, oa-eval, oa-nvim, oa-vscode, oa-agent]
    defined_in: "MASTER-SPEC.md section 4; oa-engine/docs/protocol.md"
  - id: document-json-shapes
    owner: oa-engine
    transfers_to: oa-protocol
    version_field: "schema (top-level field in all JSON)"
    current: 1
    consumers: [oa-emacs, oa-eval, oa-nvim, oa-vscode]
    defined_in: "oa-engine/SPEC.md sections 5.2, 6.1, 7.1, 8.1"
  - id: org-project-conventions
    owner: oa-engine
    version_field: "none (changes follow the document-json-shapes version)"
    current: 1
    consumers: [oa-emacs, oa-templates, oa-docs]
    defined_in: "oa-engine/SPEC.md sections 5 and 7; MASTER-SPEC.md section 5.8"
  - id: emacs-user-interface
    owner: oa-emacs
    version_field: "package version"
    current: "0.1.0 (planned)"
    consumers: [oa-docs]
    defined_in: "MASTER-SPEC.md sections 5.2 to 5.4; oa-emacs/SPEC.md"
  - id: api-key-convention
    owner: oa-workspace
    version_field: "none"
    current: 1
    consumers: [oa-engine, oa-emacs, oa-eval, oa-nvim, oa-vscode, oa-agent]
    defined_in: "MASTER-SPEC.md sections 4.5 and 11"
```
<!-- registry:end -->

### 17.6 Connection rules

1. **Acyclic.** The graph made of `edges` plus the derived contract edges (consumer → owner) has no cycle. Test and docs edges are included in the check.
2. **Contracts are the only coupling.** Repos meet only through the contracts in 17.5. A repo never reads another repo's files, internals, or private packages, and never imports another repo's `internal/`.
3. **Build coupling is versioned.** Go imports between repos use public, SemVer-tagged packages pinned by version. No `replace` directives in committed code.
4. **One owner per contract.** Consumers vendor fixtures with a sync script and never edit the vendored copy. The integrator verifies sync at every integration gate (9.2).
5. **Test tools are pinned.** A consumer pins `oa-testkit` by tag.
6. **Active repos never hard-depend on inactive ones.** Optional edges are allowed, and the code behind them must degrade gracefully (for example, Emacs works fully without `oa-agent`).
7. **Independent build.** Each repo builds and passes its gates with only its declared dependencies available.
8. **Compatibility is explicit.** Each client declares the protocol version it needs; the engine reports the version it implements; the compatibility table is kept in each README and checked by the integrator.
9. **Language policy.** Shipped repos are standard-library-first (Go) or built-ins-only (Elisp). Only workspace tooling (`.master/tools/`) may use approved third-party libraries.

### 17.7 Change propagation and impact analysis

When a repo or a contract changes, the master:

1. Identifies the changed node (a repo, or a contract id).
2. Computes the **impact set**: all consumers reachable over `runtime`, `build`, `release` edges and derived contract edges. `test` and `docs` neighbors get *verify* tasks only.
3. Orders the work by topological order: owner first, then consumers.
4. For a contract change: runs the architect proposal and human approval (13), bumps the version field, regenerates fixtures in the owner, then creates one update task per consumer (each syncing fixtures and adapting code), and finishes with the integration gate.
5. Updates compatibility tables and the registry's `current` values.

A change that does not touch a contract or a version field does not propagate beyond its repo.

### 17.8 Adding, activating, splitting, and archiving repos

**Add or activate (candidate → planned → active):**
1. Architect note: proposed registry entry, edges, contract changes, and why the trigger has fired.
2. Human approves the registry change (this section is edited with approval).
3. Master creates the directory, runs `git init`, generates `SPEC.md` (17.9), writes `AGENTS.md` from the repo's kind, adds a `Makefile` with the declared gates, adds the `.gitignore` entry at the workspace root, and adds the repo's tasks to `PLAN.org`.
4. `check-registry` passes; the repo's first task is its skeleton task (like G0 or L0).

**Split or extract (for example `oa-protocol`, `oa-testkit`, `oa-agent`):**
1. Architect note with the exact files that move and the new contract ownership.
2. Move the code with history (`git subtree split` or `git filter-repo` in a scratch clone), keep tags where meaningful.
3. Owner first, consumers next (17.7). Remove the old location only after every consumer is green against the new one.
4. Update `transfers_to` to `owner` in the contract entries and regenerate affected digests.

**Archive:** set `archived`, create no further tasks, and remove the directory only with explicit human approval.

### 17.9 Generating a repo spec from the registry

**Inputs:** the repo's registry entry, its edges (both directions), the contracts it owns or consumes, its tasks from `PLAN.org`, and the per-kind template below.

**Output:** `<repo>/SPEC.md` containing

```markdown
# <id>: Specification

<!-- generated:registry:begin (do not edit by hand; run the spec generator) -->
## Master digest
1. Purpose: <purpose>
2. Layout: <layout lines>
3. Connections: consumes <edges out, with type and via>; consumed by <edges in>
4. Contracts: owns <...>; implements or consumes <...>, each with its defined_in reference
5. Gates: <gates>          (generated)
6. Tasks: <ID, depends, section> for this repo, from PLAN.org
7. Assumptions and pitfalls: <notes, hand-written, kept outside the markers if edited>
<!-- generated:registry:end -->

## 1. Scope          (hand-written from here on, numbered, numbers stable)
…
```

Rules: regeneration rewrites **only** the marked block. Everything after it is hand-written (by an architect task, reviewed, human-approved). The checker (17.10) regenerates the block in memory and fails if the committed one differs.

**Per-kind section stubs** (the generator emits headings with `TODO(architect)` bodies):

| Kind | Required numbered sections |
|---|---|
| `engine` | Scope; design decisions; layout and toolchain; one section per package with API and acceptance criteria; CLI and exit codes; non-functional requirements; implementation tasks; rules for coding agents; open decisions; risks |
| `client` | Scope; user interface contract (commands, keys, customization); configuration and state; process and protocol handling; flows; acceptance criteria; implementation tasks; rules for coding agents; open decisions; risks |
| `contract` | Purpose; versioning policy; schemas; fixtures; consumer and owner obligations; change process |
| `testing` | Tools provided; fault catalog; how consumers use them; versioning |
| `content` | Catalog; authoring rules; validation rules; contribution process |
| `docs` | Structure; build; verification (links, executed commands); ownership of each page |
| `eval` | Task suite format; metrics; reproducibility rules; reporting; cost controls |
| `packaging` | Targets; release process; signing; compatibility matrix |
| `meta` | (this file) |

**Workflow:** the master assigns an architect task `SPECGEN-<repo>`: run the generator, write the hand-written sections from this file and the registry (never inventing contracts), then a reviewer checks the result against this file, and the human approves. The engine and Emacs specs written before the registry existed are brought under this scheme by `B0`/`B0b`.

### 17.10 Registry validation (`check-registry`)

The check runs at master startup (6.3), in CI of the workspace, and before any merge that touches this section or a repo spec. A failure blocks the master from spawning new work until fixed.

1. `id` and `dir` are unique; `id` equals the directory name; every `spec` path is unique.
2. Every `active` repo has an existing directory that is a git repo, an existing `spec` file with a current generated digest, non-empty `gates`, and a `layout`.
3. Every `planned` or `candidate` repo has a `trigger`, and no directory exists for it.
4. Every edge references known repo ids; `type` is a known type.
5. The dependency graph (edges plus derived contract edges) is acyclic.
6. No `active` repo has a non-optional edge, or a contract it consumes, whose provider is not `active` (planned providers are allowed only where 17.5 says the contract still lives in the owner, such as `engine-protocol`).
7. Every contract has exactly one existing `owner`; every consumer is a known repo; every `transfers_to` is a known repo that is not `active`.
8. Every repo with a `runtime` or `build` edge to another repo states in `release.compat` which version of that dependency (protocol or library) it requires.
9. Registry copies agree: `.master/repos.yaml` equals the YAML in this section.
