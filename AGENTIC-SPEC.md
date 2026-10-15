# Org Agent: Agentic Architecture & Execution Specification (AGENTIC-SPEC.md)

> **Status:** Draft v1.0  
> **Precedence:** Extends `MASTER-SPEC.md` §1.3, §12 (Milestone M5+), and §17.5 (`oa-agent`).  
> **Source Inspiration:** Reverse-engineered and adapted from the DeepSeek Harness (`/home/amir/Desktop/code/deepseek-harness`).  
> **Scope:** Architecture, Turn/Step execution loops, Tool execution pipeline, Subagent orchestration, Sandbox/Approval policy, and Emacs UI/UX integration.

---

## 1. Executive Summary & Architectural Vision

### 1.1 The Convergence of Two Paradigms
Current `eap` (Org Agent v0.1) operates as an interactive, single-turn assistant:
1. The human edits an Org document.
2. The Go engine calculates document deltas, gathers context (`:ctx:`, ancestor chains, focus), and builds a prompt.
3. The LLM streams a response via SSE/NDJSON back into Emacs.
4. The human accepts the response into `* Agent :agent:` with heading normalization.

While ergonomic, single-turn generation cannot autonomously fix failing tests, explore large codebases, execute refactorings, or verify deliverables. 

**DeepSeek Harness (`dsh`)** represents an industrial-grade agent harness featuring:
- An **event-sourced session log** as the single source of truth.
- A **ReactLoop driver** enforcing strict turn/step boundaries, request waterfalls, and tool barriers.
- A **guarded tool execution pipeline** supporting concurrent and barrier execution, monotonic permissions, and approval hooks.
- **Subagent delegation**, **dynamic workflows**, **same-session goals**, and **summary compaction**.

**The Vision for `eap` v2 (Agentic `eap`):**  
Merge the conversational ergonomics of an Emacs Org-mode document with the execution power of the DeepSeek Harness loop. In `eap`, the Org document serves both as the **human project canvas** and the **high-level event ledger**, while the Go engine executes an autonomous, multi-step tool loop with human-in-the-loop approvals.

```
┌────────────────────────────────────────────────────────────────────────┐
│                          Emacs Client (oa-emacs)                       │
│  - Org project canvas (#+OA: t, task trees, :ctx:, :agent:)            │
│  - Interactive *oa: <project>* buffer (streaming tokens & tool cards)  │
│  - Human-in-the-loop approval dialogs (transient / minibuffer)         │
│  - Atomic AST writeback & diff reviews                                 │
└───────────────────────────────────┬────────────────────────────────────┘
                                    │ stdin/stdout: NDJSON Protocol v2
                                    ▼
┌────────────────────────────────────────────────────────────────────────┐
│                           Go Engine (oa-engine)                        │
│ ┌──────────────────────┐  ┌────────────────────┐  ┌──────────────────┐ │
│ │  Org & Delta Engine  │  │   Agent Turn Loop  │  │  Tool Pipeline   │ │
│ │ (AST, Snaps, CtxSel) │  │ (Turns, Steps, SSE)│  │ (FS, Bash, Guard)│ │
│ └──────────────────────┘  └────────────────────┘  └──────────────────┘ │
└───────────────────────────────────┬────────────────────────────────────┘
                                    │ HTTP / SSE (OpenAI wire format)
                                    ▼
                     9router / OpenAI-compatible LLM
```

---

## 2. Deconstruction of DeepSeek Harness Architecture

DeepSeek Harness achieves state-of-the-art stability, auditability, and safety through several foundational subsystems:

### 2.1 The Cordis Framework & Inversion of Control
Every capability in `dsh` is a plugin mounted onto a shared Cordis `Context`. Plugins provide:
- **Services:** Singleton capabilities accessible via `ctx.<name>` (e.g., `ctx.sessions`, `ctx.tools`, `ctx.llm`).
- **Events & Waterfalls:** Typed asynchronous extension points. Waterfalls allow listeners to inspect, modify, or veto payloads sequentially before reaching the terminal provider.
- **Reversible Lifecycles:** When a plugin or agent is disposed, all registered tools, hooks, and listeners unwind cleanly.

### 2.2 The Event-Sourced Session Log
In `dsh`, the session log (`SessionEvent`) is the sole source of truth:
- **Durable Reconstructability:** Every model request can be completely reconstructed by reading the chronological event stream (`turn/start`, `step/start`, `system/message`, `user/message`, `assistant/message`, `tool/call`, `tool/result`, `step/end`, `turn/end`).
- **Pure Projections:** Model history is derived via pure projection functions (`deriveMessages()`). No ad-hoc in-memory arrays persist outside the log.
- **Attempt Tracking:** Failed, aborted, or retried attempts are recorded as `assistant/attempt` without polluting model context, preserving full observability.

### 2.3 The ReactLoopAgent Turn & Step Lifecycle
A request is structured into **Turns** and **Steps**:
1. **Turn (`turn/start` ... `turn/end`):** Begins when new user input or waking input is claimed from the inbox. A turn encompasses multiple autonomous steps until the objective settles or human guidance is needed.
2. **Step (`step/start` ... `step/end`):** One LLM generation call plus the execution of any tools requested in that call.
   - **`agent/pre-step`:** Waterfall to inspect claimed messages, inject runtime context, and assemble prompt sections.
   - **`agent/request`:** Resolves provider routes, configures model capabilities, and reconciles system prompt updates.
   - **LLM Streaming:** Streams tokens over `llm/stream` into `assistant/message`.
   - **Tool Execution:** Parses tool calls, resolves permissions, and dispatches tool execution.
   - **Continuation Check:** If tools were called, results are appended, and the step loop repeats immediately. If no tools were called, `agent/turn-stopping` runs before closing the turn.

### 2.4 Guarded Tool Execution Pipeline
Tools are executed through a multi-stage guarded pipeline:
1. **`tools/pre-execute`:** Runs hooks, checks monotonic permissions, and queries `ctx.approval`.
2. **Monotonic Guards:** Immutable policy invariants that cannot be bypassed by retries or overrides.
3. **Execution Modes:**
   - **`parallel`:** Read-only / concurrency-safe tools run in a bounded rolling worker pool.
   - **`barrier`:** State-mutating tools (e.g. bash commands, file writes) act as barriers; all prior calls must finish, and the barrier runs exclusively.
4. **Execution Body:** Runs the tool logic with cancellation signal propagation (`exec.signal`).
5. **`tools/post-execute` & Normalization:** Validates output against the tool's JSON schema, projects model-facing text, and enforces `finalizeContent`.

### 2.5 Subagent Delegation & Orchestration
`dsh` separates orchestration into distinct models:
- **Local Child Subagents:** Full session-backed agents spawned under a parent (`ctx.agents.create({ parentAgent })`). Delivery can be `parent` (notifies the calling model on completion) or `caller` (returns a programmatic promise).
- **Dynamic Workflows:** Sandboxed JavaScript scripts running in an isolated process that coordinate subagents via `agent()`, `pipeline()`, and `parallel()`.
- **Iterative Ralph Loops:** Fresh-agent execution where each iteration starts with no conversational history; long-term memory is preserved solely via the shared filesystem workspace.

### 2.6 Same-Session Goals & Plan Mode
- **Goals (`ctx.goals`):** Maintains a durable objective across autonomous turns with compare-and-set revisions (`GoalRef`) and states (`active`, `paused`, `blocked`, `complete`).
- **Plan Mode (`ctx.planMode`):** Soft-guidance prompt injection that requires the agent to draft a plan and call `exit_plan_mode` for human approval before taking state-mutating actions.

---

## 3. Adapting Harness Architecture to `eap`

We translate these TypeScript/Cordis abstractions into a clean, stdlib-first Go engine and an Emacs Lisp interface.

### 3.1 The Org Document as the Living Canvas & Audit Log
In `eap`, we avoid opaque binary logs or raw JSONL files as the primary user interface. **The Org file itself is the human-facing canvas:**

1. **Context Nodes (`:ctx:`):** Persistent system prompts and architectural background.
2. **Task Tree (`TODO` / `DONE`):** Action items, goals, and sub-objectives.
3. **Agent Ledger (`* Agent :agent:`):** The structured record of all agent interventions, tool executions, and diff commits.
4. **Tool Invocations as Org Drawers / Blocks:**
   ```org
   * Agent :agent:
   ** 2026-10-15 20:30 Implement User Authentication (gpt-5)
   :PROPERTIES:
   :RUN_ID: run-88f12a
   :TURNS: 2
   :STEPS: 4
   :TOOLS: fs_read, fs_edit, bash_exec
   :END:

   *** Step 1: Inspection
   #+BEGIN_TOOL_CALL: fs_read
   {"path": "internal/auth/auth.go"}
   #+END_TOOL_CALL
   #+BEGIN_TOOL_RESULT: fs_read
   package auth
   // missing token verification
   #+END_TOOL_RESULT

   *** Step 2: Patch & Verification
   #+BEGIN_TOOL_CALL: fs_edit
   {"path": "internal/auth/auth.go", "old_string": "...", "new_string": "..."}
   #+END_TOOL_CALL
   #+BEGIN_TOOL_RESULT: fs_edit
   File internal/auth/auth.go updated successfully.
   #+END_TOOL_RESULT
   ```

### 3.2 Protocol Evolution: Wire Protocol v2 (NDJSON Stream)
To support multi-step tool loops and interactive human approvals, the communication between `oa` and `oa-emacs` is extended to Protocol v2.

#### Event Stream Specification:
| Event | Direction | Payload Schema | Description |
|---|---|---|---|
| `start` | Engine → Client | `{"schema":2,"event":"start","model":ID,"tokens_est":N,"run_id":STR}` | Stream initiation |
| `step_start` | Engine → Client | `{"schema":2,"event":"step_start","step":N,"turn":M}` | Beginning of an autonomous step |
| `delta` | Engine → Client | `{"schema":2,"event":"delta","text":STR}` | Partial assistant token stream |
| `tool_call` | Engine → Client | `{"schema":2,"event":"tool_call","call_id":ID,"tool":NAME,"args":OBJ}` | Model invoked a tool |
| `approval_req`| Engine → Client | `{"schema":2,"event":"approval_req","req_id":ID,"tool":NAME,"action":DESC,"risk":"high"|"med"|"low"}` | Engine requests human authorization |
| `approval_res`| Client → Engine | `{"schema":2,"event":"approval_res","req_id":ID,"approved":BOOL,"reason":STR}` | Sent on engine stdin |
| `tool_result` | Engine → Client | `{"schema":2,"event":"tool_result","call_id":ID,"output":OBJ,"is_error":BOOL}` | Tool execution settled |
| `step_end` | Engine → Client | `{"schema":2,"event":"step_end","step":N,"finish_reason":STR}` | Step completed |
| `usage` | Engine → Client | `{"schema":2,"event":"usage","prompt_tokens":N,"completion_tokens":M}` | Token accounting |
| `done` | Engine → Client | `{"schema":2,"event":"done","finish_reason":"stop"|"max_steps"|"blocked"}` | Request finished |
| `error` | Engine → Client | `{"schema":2,"event":"error","kind":STR,"message":STR}` | Fatal error terminating loop |

---

## 4. Go Engine: Architecture & Tool Pipeline (`oa-engine`)

The agent loop in `oa-engine` will reside under `internal/agent/` and `internal/tools/`.

```
oa-engine/
├── cmd/
│   ├── oa/                 # CLI entry point (new commands: agent, run)
│   └── oa-mockrouter/      # Mock server supporting tool-call responses
├── internal/
│   ├── org/                # Org parser & AST
│   ├── snapshot/           # Document snapshotting
│   ├── delta/              # Document diffing
│   ├── ctxsel/             # Context node selection
│   ├── prompt/             # Prompt assembly
│   ├── provider/           # OpenAI client (extended for tool schemas & function calling)
│   ├── events/             # Protocol v2 NDJSON event definitions
│   ├── tools/              # Tool definitions, schemas, and execution engine
│   │   ├── registry.go     # Tool registry and concurrency scheduler
│   │   ├── builtins.go     # Standard tools (fs_read, fs_write, fs_edit, bash, etc.)
│   │   ├── guard.go        # Path boundary and security sandboxing
│   │   └── approval.go     # Interactive approval manager
│   └── agent/              # Autonomous Turn/Step ReactLoop
│       ├── loop.go         # Step loop, continuation condition, step limits
│       ├── state.go        # In-memory execution state and message history
│       └── session.go      # Replay log and snapshot updater
```

### 4.1 Tool Definition Interface (`internal/tools`)

```go
package tools

import (
	"context"
	"encoding/json"
)

// ExecutionMode controls concurrency scheduling.
type ExecutionMode int
const (
	ModeParallel ExecutionMode = iota // Read-only, safe to execute concurrently
	ModeBarrier                       // Mutating (bash, file edit); requires barrier drain
)

// RiskLevel determines whether human confirmation is required.
type RiskLevel int
const (
	RiskLow RiskLevel = iota // Read-only fs operations
	RiskMedium              // File writes/edits in project workspace
	RiskHigh                // Arbitrary bash execution, network calls
)

type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"` // JSON Schema (Draft 7/2020-12)
	Mode        ExecutionMode   `json:"-"`
	Risk        RiskLevel       `json:"-"`
	
	// Execute runs the tool. Must respect context cancellation.
	Execute func(ctx context.Context, args json.RawMessage) (any, error)
}
```

### 4.2 Built-in Core Tools ("The Fab Six")

1. **`fs_read`**:
   - Parameters: `{"path": "string", "offset": "int?", "limit": "int?"}`
   - Behavior: Reads text files within the workspace root. Rejects binary/unsupported encodings.
   - Mode: `ModeParallel`, `RiskLow`.

2. **`fs_write`**:
   - Parameters: `{"path": "string", "content": "string"}`
   - Behavior: Writes or completely overwrites a file atomically (tmpfile + rename).
   - Mode: `ModeBarrier`, `RiskMedium`.

3. **`fs_edit`**:
   - Parameters: `{"path": "string", "old_string": "string", "new_string": "string", "replace_all": "bool?"}`
   - Behavior: Exact literal text replacement. Verifies that `old_string` matches exactly once (unless `replace_all` is true). Fails fast if file changed.
   - Mode: `ModeBarrier`, `RiskMedium`.

4. **`glob`**:
   - Parameters: `{"path": "string?", "pattern": "string"}`
   - Behavior: Discovers files matching glob pattern. Ignores `.git/` and binaries.
   - Mode: `ModeParallel`, `RiskLow`.

5. **`grep`**:
   - Parameters: `{"path": "string?", "pattern": "string", "include": "string?"}`
   - Behavior: Fast regex search over text files. Returns file, line number, and matching text.
   - Mode: `ModeParallel`, `RiskLow`.

6. **`bash_exec`**:
   - Parameters: `{"command": "string", "timeout_ms": "int?"}`
   - Behavior: Spawns command in workspace directory under `/bin/bash -c`. Limits output size (capped to 64KB tail), monitors timeout, terminates child process trees on cancellation.
   - Mode: `ModeBarrier`, `RiskHigh`.

### 4.3 The Agent Turn/Step Loop (`internal/agent/loop.go`)

The loop coordinates multi-step autonomy:

```go
type AgentConfig struct {
	MaxSteps       int           // e.g. 25 steps per turn ceiling
	StepTimeout    time.Duration // e.g. 120s per step
	RequireApprove bool          // whether high-risk tools require client approval
}

func (l *AgentLoop) Run(ctx context.Context, doc *org.Document, prompt string) error {
    turn := 1
    step := 1
    
    // 1. Initial Prompt Assembly & Model History
    messages := []provider.ChatMessage{
        {Role: "system", Content: l.SystemPrompt},
        {Role: "user", Content: prompt},
    }
    
    for step <= l.Config.MaxSteps {
        // Emit step_start event
        l.events.EmitStepStart(step, turn)
        
        // Call Provider with Tools Schema enabled
        resp, err := l.provider.StreamChatWithTools(ctx, l.Model, messages, l.tools.Schemas())
        if err != nil {
            return l.handleError(err)
        }
        
        // If assistant emitted content, record in history
        messages = append(messages, resp.Message)
        
        // If no tool calls were requested, objective reached!
        if len(resp.ToolCalls) == 0 {
            l.events.EmitDone("stop")
            return nil
        }
        
        // Execute tool calls (handling barriers and approvals)
        for _, call := range resp.ToolCalls {
            tool := l.tools.Lookup(call.Name)
            if tool.Risk == RiskHigh && l.Config.RequireApprove {
                approved := l.requestClientApproval(ctx, call)
                if !approved {
                    messages = append(messages, toolResult(call.ID, "Action denied by user.", true))
                    continue
                }
            }
            
            result, isErr := tool.Execute(ctx, call.Args)
            l.events.EmitToolResult(call.ID, result, isErr)
            messages = append(messages, toolResult(call.ID, result, isErr))
        }
        
        l.events.EmitStepEnd(step)
        step++
    }
    
    l.events.EmitDone("max_steps_exceeded")
    return nil
}
```

---

## 5. Emacs Client: UI/UX & Human-in-the-Loop Cockpit (`oa-emacs`)

The client provides full visibility and control over autonomous agent runs.

### 5.1 Interactive Output Buffer (`*oa: <project>*`)
The output buffer is transformed from a passive text sink into an interactive execution monitor:
- **Streaming Tokens:** Rendered directly into the active buffer as text.
- **Collapsible Tool Call Cards:**
  When a `tool_call` event arrives:
  ```org
  #+BEGIN_TOOL: bash_exec [RUNNING]
  Command: go test ./...
  #+END_TOOL
  ```
  Upon `tool_result`, updates in-place to:
  ```org
  #+BEGIN_TOOL: bash_exec [SUCCESS] (0.42s)
  PASS: ok  oa-engine/internal/agent 0.005s
  #+END_TOOL
  ```
  Foldable via `TAB` (`org-cycle`).

### 5.2 Real-Time Approval Dialogs
When the engine emits `approval_req`:
1. Emacs displays a non-blocking prompt or transient pop-up:
   ```
   [Org Agent] Tool Approval Required:
   Action: bash_exec: "rm -rf bin/ && make build"
   Risk: HIGH
   [y] Approve once   [a] Approve all this session   [d] Deny   [c] Cancel turn
   ```
2. User keystroke (`y`/`d`) sends `approval_res` over engine standard input.
3. The engine immediately resumes or aborts without crashing the process pipe.

### 5.3 Diff Review Buffer
Before executing `fs_edit` or `fs_write`:
- Client can optionally pop up a side-by-side diff (`ediff` or `diff-mode` buffer).
- Pressing `C-c C-c` approves the modification; `C-c C-k` denies it with human feedback forwarded to the agent.

### 5.4 Multi-Step Writeback Semantics
When an autonomous turn concludes:
1. `oa-accept` writes an entry under `* Agent :agent:` containing:
   - Summary of actions taken.
   - List of modified files with diff links.
   - Closed TODO items updated in the project outline.
2. The send-time snapshot is updated to reflect all files altered by tool operations during the turn.

---

## 6. Advanced Workflows: Goals & Plan Mode

### 6.1 Same-Session Goals in Org-Mode
`eap` maps the Harness Goal subsystem directly to Org headlines:
```org
* Goals :goal:
** GOAL Refactor provider client to support tool-use streaming
:PROPERTIES:
:ROUNDS: 3
:MAX_ROUNDS: 10
:STATUS: active
:END:
Objective: Add function calling schemas to /chat/completions payload.
```
- The engine checks `:goal:` subtrees.
- If a goal is active, the engine autonomously arms subsequent turns after each step sequence until the goal criteria are verified and marked `DONE`.

### 6.2 Plan Mode Guidance
When starting complex refactoring tasks:
- `M-x oa-plan-mode` activates plan mode.
- System prompt instructs the model to refrain from mutating tools (`fs_edit`, `fs_write`, `bash_exec`) and emit an initial plan.
- The model calls `exit_plan_mode` presenting its proposed plan.
- The user reviews the plan in Emacs (`C-c a a`). Upon approval, plan mode deactivates and the agent executes the approved steps.

---

## 7. Implementation Roadmap & Milestones

| Milestone | Deliverables | Target Area |
|---|---|---|
| **M5.1: Tool Foundation** | Built-in developer tools (`fs_read`, `fs_write`, `fs_edit`, `glob`, `grep`, `bash_exec`); path sandbox guards; unit tests. | `oa-engine/internal/tools` |
| **M5.2: Protocol v2 & Approvals** | NDJSON Protocol v2 (`step_start`, `tool_call`, `approval_req`, `tool_result`); interactive stdin/stdout approval channel. | `oa-engine/internal/events`, `provider` |
| **M5.3: Autonomous React Loop** | Multi-step agent loop (`internal/agent`); step counters; tool call history assembly; convergence detectors. | `oa-engine/internal/agent`, `cmd/oa` |
| **M5.4: Emacs Tool UI** | Foldable `#+BEGIN_TOOL` blocks in `*oa: <project>*`; transient approval pop-up; diff preview integration. | `oa-emacs/oa-ui.el`, `oa-engine.el` |
| **M5.5: Goal Tracking & Writeback**| `:goal:` headline tracking; multi-step session summaries in `* Agent :agent:`; snapshot synchronization. | `oa-emacs/oa-doc.el`, `oa-engine` |

---

## 8. Definition of Done for Agentic `eap`

An agentic task is complete when a developer can:
1. Open a `project.org` document in Emacs with a failing test noted under `* Tasks`.
2. Press `C-c a s` (or `oa-send`).
3. Observe the model autonomously:
   - Run `glob` and `grep` to locate the source code.
   - Run `fs_read` to inspect the code.
   - Prompt the user in Emacs to approve a `fs_edit` patch.
   - Run `bash_exec: make test` to verify the fix passes.
4. Review the structured result in the `*oa-output*` buffer and press `C-c C-c` to accept the fix and ledger summary into the Org document.
