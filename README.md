# eap (Org Agent)

Agentic LLM-assisted development directly from Emacs, driven by a single `org-mode` project document.

## Overview

- **Core idea**: Human edits an org document. The Go engine (`oa`) detects deltas since last snapshot, selects relevant context (`:ctx:`, ancestor chains, focus), builds plain-text prompts, and streams LLM answers back into Emacs.
- **Client**: `oa-emacs` provides a clean keyboard-driven interface (`oa-mode`), output buffer, and atomic writeback into `* Agent :agent:`.
- **Primary endpoint**: 9router or any OpenAI-compatible gateway (`/v1/chat/completions`).

## Monorepo Layout

```
.
├── MASTER-SPEC.md     # System-wide architecture and cross-component contracts
├── AGENTS.md          # Rules for development agents
├── .master/           # Planning ledger (PLAN.org) and agent role definitions
├── oa-engine/         # Go engine CLI (parser, snapshot, delta, prompt, provider)
└── oa-emacs/          # Emacs Lisp package (oa-mode, UI, process runner)
```

## Quick Start

### 1. Build Engine

```bash
cd oa-engine
make build
# Output: bin/oa
```

### 2. Configure Emacs

Add to your Emacs configuration:

```elisp
(add-to-list 'load-path "~/path/to/eap/oa-emacs")
(require 'oa)
(add-hook 'org-mode-hook #'oa-maybe-enable)
```

Inside an org buffer with `#+OA: t`:
- `M-x oa-add-endpoint` (e.g. `http://localhost:20128/v1`).
- `C-c a m` to select a model.
- `C-c a s` to send uncommitted changes.
- `C-c a a` (or `C-c C-c` in output buffer) to accept response into `* Agent :agent:`.

## License

MIT
