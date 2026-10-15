# oa-emacs

Emacs client package for Org Agent (`oa`).

Provides an ergonomic, keyboard-driven interface over the `oa` Go engine.

## Requirements

- Emacs 29.1+
- Built-in libraries only (`org`, `transient`, `auth-source`, `json`, `seq`, `map`, `cl-lib`, `subr-x`). Zero external MELPA dependencies.
- Compiled `oa` binary in `PATH` or configured via `oa-engine-program`.

## Setup

```elisp
(add-to-list 'load-path "~/path/to/eap/oa-emacs")
(require 'oa)
(add-hook 'org-mode-hook #'oa-maybe-enable)
```

`oa-maybe-enable` turns on `oa-mode` if `#+OA: t` is in the buffer or a `.oa/` directory exists next to the file.

## Key Bindings (`oa-mode`, Prefix: `C-c a`)

| Key Binding | Command | Action |
|---|---|---|
| `C-c a s` | `oa-send` | Send uncommitted changes since last snapshot |
| `C-c a f` | `oa-send-focus` | Send subtree at point as focus |
| `C-c a p` | `oa-preview` | Preview prompt without sending |
| `C-c a m` | `oa-select-model` | Select model from cache or free-text |
| `C-c a e` | `oa-select-endpoint` | Switch active endpoint |
| `C-c a E` | `oa-add-endpoint` | Add and validate new endpoint |
| `C-c a r` | `oa-refresh-models` | Refresh endpoint model list |
| `C-c a o` | `oa-show-output` | Display output buffer |
| `C-c a k` | `oa-cancel` | Cancel active request |
| `C-c a a` | `oa-accept` | Accept response and write into document |
| `C-c a ?` | `oa-menu` | Open transient menu |

### Output Buffer Keys

In `*oa: <project>*`:
- `C-c C-c`: Accept response (`oa-accept`).
- `C-c C-k`: Cancel request (`oa-cancel`).
- `C-c C-l`: Clear previous outputs.
- `g`: Re-send request.
- `q`: Hide output window.

## Customization (`M-x customize-group RET oa`)

- `oa-engine-program`: Path to `oa` binary (default `"oa"`).
- `oa-keymap-prefix`: Keymap prefix (default `"C-c a"`).
- `oa-confirm-before-send`: Confirmation mode (`'summary`, `'preview`, or `nil`).
- `oa-accept-mode`: Writeback prompt behavior (`'ask`, `'auto`, `'manual`).
- `oa-state-file`: State storage path (default `~/.emacs.d/oa/state.eld`, contains no secrets).

## Secrets Management

API keys are never stored in state files. Keys are retrieved at runtime from `process-environment` (`OA_API_KEY`) or `auth-source` (`~/.authinfo.gpg`).
