# oa-emacs: Specification

<!-- generated:registry:begin (do not edit by hand; run the spec generator) -->
## Master digest
1. Purpose: The Emacs client: manage endpoints and keys, fetch and choose models, send with a key binding, stream output into its own buffer, accept answers into the project document.
2. Layout:
   - oa.el: Entry point, minor mode definition, keymap, autoloads
   - oa-config.el: Endpoint management, state persistence, auth-source resolution
   - oa-engine.el: Binary location, process invocation, NDJSON framing, cancellation
   - oa-ui.el: Output buffer, header-line updates, window display, transient menu
   - oa-doc.el: Document capture, response heading normalization, writeback, commit
   - test/: ERT test suite, fake engine script, vendored engine fixtures
   - docs/legacy-elisp-first.md: Archived concept notes from legacy design
3. Connections:
   - Consumes: oa-engine (runtime via subprocess pipes and NDJSON), oa-testkit (test)
   - Consumed by: oa-docs (docs), oa-packaging (release)
4. Contracts:
   - Owns: emacs-user-interface (v0.1.0)
   - Implements or consumes: engine-protocol (v1), document-json-shapes (schema 1), org-project-conventions (v1), api-key-convention
5. Gates:
   - make test (ERT unit tests in batch mode)
   - make compile (byte-compilation with warnings as errors)
   - make lint (checkdoc and package-lint checks)
6. Tasks:
   - L0: Skeleton, Makefile, test harness, vendored protocol fixtures
   - L1: oa-config (endpoints data model, state file, key-source resolution)
   - L2: oa-engine (binary locator, version check, process handling, NDJSON framing)
   - L3: Models cache and selection (TTL caching, oa-select-model)
   - L4: Endpoint UX (oa-add-endpoint with validation, select, edit, remove)
   - L5: Send flow and output buffer (stream rendering, confirm modes)
   - L6: Cancel and error handling (sentinels, signal dispatch)
   - L7: oa-doc (buffer capture, heading normalization, writeback, atomic commit)
   - L8: oa-mode minor mode, key bindings, transient menu, oa-doctor
   - L9: Documentation and linting compliance
   - X1: E2E suite integration with real oa engine and oa-mockrouter
   - X2: Manual acceptance test against live 9router
   - X3: Release tagging v0.1.0
7. Assumptions and pitfalls:
   - Built-in Emacs 29.1+ libraries only (org, transient, auth-source, json, seq, map, cl-lib). Zero MELPA/external dependencies.
   - API keys must never be stored in plaintext state files; resolve at runtime from environment or auth-source.
   - Acceptance and writeback must modify only the `* Agent :agent:` headline and leave all user subtrees unchanged.
   - Process filter must handle arbitrary NDJSON packet fragmentation across network/process chunks.
<!-- generated:registry:end -->

---

## 1. Scope & Vision

`oa-emacs` provides the Emacs frontend for Org Agent. It connects Org-mode buffers to the `oa` Go engine, offering an ergonomic, keyboard-driven interface for LLM-assisted document editing.

Core responsibilities:
1. Detecting Org Agent project files (`oa-mode`).
2. Managing endpoint configurations (URLs, auth sources) and model selections.
3. Spawning the `oa` binary as an asynchronous subprocess.
4. Parsing streamed NDJSON events into an interactive output buffer.
5. Re-leveling response headlines and committing accepted changes atomically into `* Agent :agent:`.
6. Updating project snapshots using send-time buffer snapshots to preserve concurrent human edits.

---

## 2. Architecture & File Organization

```
  ┌────────────────────────────────────────────────────────┐
  │                        oa-mode                         │
  │                  (Keymap: C-c a ...)                   │
  └───────────┬────────────────────────────────┬───────────┘
              │                                │
              ▼                                ▼
  ┌───────────────────────┐        ┌───────────────────────┐
  │       oa-config       │        │        oa-doc         │
  │  Endpoints / Auth /   │        │ Capture / Normalize / │
  │    State File .eld    │        │  Writeback / Commit   │
  └───────────┬───────────┘        └───────────┬───────────┘
              │                                │
              ▼                                ▼
  ┌───────────────────────┐        ┌───────────────────────┐
  │         oa-ui         │        │       oa-engine       │
  │ Output Buffer / Menu  │◄───────┤ Process Management /  │
  │    Header-Line / UX   │        │     NDJSON Framing    │
  └───────────────────────┘        └───────────┬───────────┘
                                               │
                                               ▼
                                      Subprocess: `oa`
```

Files:
- `oa.el`: Mode activation (`oa-mode`, `oa-maybe-enable`), key bindings, and package entry.
- `oa-config.el`: Endpoint data model, reading/writing `~/.emacs.d/oa/state.eld`, resolving secrets from `auth-source` or `process-environment`.
- `oa-engine.el`: Resolving the `oa` executable via `executable-find`, running `oa version -json`, spawning child processes with pipes, framing partial NDJSON lines, dispatching cancellation.
- `oa-ui.el`: Managing `*oa: <project>*` buffers, rendering streaming tokens, updating header lines, rendering the `transient` menu (`oa-menu`).
- `oa-doc.el`: Capturing buffer text at send time, calculating heading offsets, formatting entries under `* Agent :agent:`, invoking `oa snapshot`.

---

## 3. Setup & User Interface Contract

### 3.1 Setup (5 Lines)
```elisp
(add-to-list 'load-path "~/Desktop/code/eap/oa-emacs")
(require 'oa)
(add-hook 'org-mode-hook #'oa-maybe-enable)
```

`oa-maybe-enable` activates `oa-mode` if:
- The buffer contains `#+OA: t`, or
- A `.oa/` directory exists adjacent to the buffer file.

### 3.2 Key Bindings (`oa-mode`, Prefix `C-c a`)
| Key Binding | Command | Description |
|---|---|---|
| `C-c a s` | `oa-send` | Send uncommitted changes since last snapshot |
| `C-c a f` | `oa-send-focus` | Send current subtree at point as focus |
| `C-c a p` | `oa-preview` | Preview assembled prompt without sending |
| `C-c a m` | `oa-select-model` | Choose active model |
| `C-c a e` | `oa-select-endpoint`| Select active endpoint |
| `C-c a E` | `oa-add-endpoint` | Configure and validate new endpoint |
| `C-c a r` | `oa-refresh-models` | Refresh cached models from endpoint |
| `C-c a o` | `oa-show-output` | Switch to output buffer |
| `C-c a k` | `oa-cancel` | Cancel in-flight request |
| `C-c a a` | `oa-accept` | Accept output and write into document |
| `C-c a ?` | `oa-menu` | Open transient menu |

### 3.3 Output Buffer Key Bindings
Inside `*oa: <project>*`:
- `C-c C-c`: Accept response (`oa-accept`).
- `C-c C-k`: Cancel request (`oa-cancel`).
- `C-c C-l`: Clear previous responses.
- `g`: Re-send last request.
- `q`: Hide output window.

---

## 4. Configuration & State Management

### 4.1 Customization Options (`M-x customize-group RET oa`)
- `oa-engine-program` (default `"oa"`): Executable name or absolute path.
- `oa-keymap-prefix` (default `"C-c a"`): Mode key prefix.
- `oa-confirm-before-send` (default `'summary`): Confirmation style (`nil`, `'summary`, `'preview`).
- `oa-accept-mode` (default `'ask`): Auto-accept behavior (`'ask`, `'auto`, `'manual`).
- `oa-display-buffer-action`: Buffer display rules (default: right side window, 40% width).
- `oa-models-cache-ttl` (default `86400`): Cache lifetime in seconds.
- `oa-state-file`: Default `~/.emacs.d/oa/state.eld`.
- `oa-snapshot-file`: Default `".oa/snapshot.json"`.

### 4.2 State File (`state.eld`)
Contains no secrets:
```elisp
(:version 1
 :active-endpoint "9router"
 :endpoints
 ((:name "9router"
   :url "http://localhost:20128/v1"
   :key-source (:type env :name "NINEROUTER_KEY")
   :selected-model "qwen2.5-coder-32b"
   :cached-models (("qwen2.5-coder-32b" . "9router"))
   :cache-timestamp 1728892800)))
```

Keys are retrieved on-demand from `auth-source` or `process-environment`; never written to disk.

---

## 5. Endpoints & Model Management

### 5.1 Endpoint Creation (`oa-add-endpoint`)
1. Prompts for endpoint name and URL (default hint `http://localhost:20128/v1`).
2. Prompts for key source:
   - `none`: No `Authorization` header.
   - `env`: Read from specified environment variable name.
   - `auth-source`: Looked up in `~/.authinfo.gpg` by host.
3. Runs `oa models -url <URL>` in the background with `OA_API_KEY` set.
4. On success: caches model list and offers selection.
5. On failure: displays error kind (`auth`, `network`, `timeout`) and offers *edit*, *save anyway*, or *discard*.

### 5.2 Model Selection (`oa-select-model`)
- Uses `completing-read` across cached model IDs.
- Allows arbitrary free-text entry so unlisted models can be specified without blocking.

---

## 6. Process Handling & NDJSON Streaming (`oa-engine.el`)

- Locates binary using `executable-find`.
- Validates version: `oa version -json` must return `"protocol": 1`.
- Invocation:
  - `make-process` with `:connection-type 'pipe`.
  - Process environment receives `OA_API_KEY`.
  - Arguments: `chat -url <URL> -model <ID> -snapshot <PATH> -focus-line <LINE> -`.
  - Buffer contents piped via stdin.
- Stream filter:
  - Buffers incoming string chunks.
  - Splits on `\n`.
  - Deserializes complete lines via `json-parse-string`.
  - Dispatches events (`start`, `delta`, `usage`, `done`, `error`) to UI handlers.

---

## 7. Send Flow & Output Buffer Lifecycle

1. User invokes `C-c a s` (`oa-send`).
2. Verifies active endpoint and model selection.
3. If an existing request is active, prompts to cancel or abort.
4. Captures buffer string into `sent-text`.
5. Runs `oa prompt` on `sent-text`. If exit code is 2, displays "Nothing new to send" and aborts.
6. Prompts for confirmation based on `oa-confirm-before-send`.
7. Displays `*oa: <project>*` buffer in side window without stealing cursor focus.
8. Inserts metadata header:
   ```org
   * 2026-10-14 08:30 some-model @ 9router
   :PROPERTIES:
   :TOKENS_EST: 1840
   :CHANGES: 2
   :END:
   ```
9. Appends streaming delta text. Updates header line state: `idle` -> `sending` -> `streaming (N chars)` -> `done`.

---

## 8. Normalization, Writeback & Atomic Commit (`oa-doc.el`)

When user invokes `oa-accept`:
1. **Heading Normalization:**
   - Finds minimum heading depth $D$ in model response.
   - Shifts all headings so the shallowest heading becomes level 3 (`***`), nesting beneath the level 2 response entry.
   - Ignores headings inside `#+BEGIN_SRC` ... `#+END_SRC` and example blocks.
2. **Buffer Writeback:**
   - Ensures headline `* Agent :agent:` exists at document end.
   - Appends entry:
     ```org
     ** 2026-10-14 08:30 Response to: <Topic> (<model>)
     <normalized response text>
     ```
   - All human-authored subtrees remain completely byte-identical.
3. **Snapshot Update:**
   - Pipes `sent-text` (captured at send time) into `oa snapshot -out .oa/snapshot.json -`.
   - Any edits typed by the user during LLM streaming remain marked as uncommitted deltas for the subsequent send.

---

## 9. Diagnostics & Quality Gates

### 9.1 Doctor Command (`M-x oa-doctor`)
Prints diagnostic report:
- `oa` executable path and `oa version -json` output.
- Active endpoint URL reachability and latency.
- Resolved key source status (found/not found; no key printed).
- State file and snapshot file paths.

### 9.2 Test Suite (`make test`)
Run via batch Emacs:
```bash
emacs -Q --batch -L . -L test/ -l test/oa-test.el -f ert-run-tests-batch-and-exit
```
Tests replay golden fixtures using a fake engine script (`test/fake-oa`).

### 9.3 Linter Gates (`make compile`, `make lint`)
- `byte-compile` runs with `byte-compile-error-on-warn t`.
- `checkdoc` validates all docstrings.
- `package-lint` ensures package cleanliness.
