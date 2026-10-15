;;; oa-config.el --- Endpoint and configuration management for Org Agent -*- lexical-binding: t; -*-

;; Author: Org Agent Team
;; Keywords: convenience, tools, org

;;; Commentary:
;; Manages endpoints, model caching, state file persistence, and API key resolution
;; without storing any secrets in plain text.

;;; Code:

(require 'cl-lib)
(require 'seq)
(require 'auth-source)
(require 'subr-x)
(require 'url-parse)

(defgroup oa nil
  "Org Agent interface."
  :prefix "oa-"
  :group 'tools)

(defcustom oa-engine-program "oa"
  "Path or command name for the Org Agent Go engine executable."
  :type 'string
  :group 'oa)

(defcustom oa-keymap-prefix "C-c a"
  "Prefix key for `oa-mode` commands."
  :type 'string
  :group 'oa)

(defcustom oa-confirm-before-send 'summary
  "Confirmation style before dispatching an LLM request.
Possible values are `summary' (one-line y/n prompt), `preview'
(show prompt buffer), or nil (send immediately)."
  :type '(choice (const :tag "One-line summary" summary)
                 (const :tag "Preview prompt" preview)
                 (const :tag "Send without confirmation" nil))
  :group 'oa)

(defcustom oa-accept-mode 'ask
  "Action on receiving a complete LLM response.
Possible values are `ask' (prompt user), `auto' (accept immediately),
or `manual' (wait for explicit `oa-accept')."
  :type '(choice (const :tag "Ask user" ask)
                 (const :tag "Auto accept" auto)
                 (const :tag "Manual" manual))
  :group 'oa)

(defcustom oa-display-buffer-action
  '((display-buffer-in-side-window)
    (side . right)
    (window-width . 0.4))
  "Action alist passed to `display-buffer' when showing output buffer."
  :type 'sexp
  :group 'oa)

(defcustom oa-models-cache-ttl 86400
  "Seconds before cached endpoint models are considered stale."
  :type 'integer
  :group 'oa)

(defcustom oa-state-file
  (locate-user-emacs-file "oa/state.eld")
  "File path for storing endpoints and model selection."
  :type 'file
  :group 'oa)

(defcustom oa-snapshot-file ".oa/snapshot.json"
  "Snapshot file path relative to the project document."
  :type 'string
  :group 'oa)

(defcustom oa-history-limit 3
  "Maximum number of recent agent entries included in context."
  :type 'integer
  :group 'oa)

(defcustom oa-max-tokens nil
  "Optional maximum token generation cap."
  :type '(choice (integer :tag "Limit") (const :tag "None" nil))
  :group 'oa)

(defcustom oa-connect-timeout 10
  "Connection timeout in seconds."
  :type 'integer
  :group 'oa)

(defcustom oa-idle-timeout 60
  "Stream idle timeout in seconds."
  :type 'integer
  :group 'oa)

(defvar oa-config--state-cache nil
  "In-memory cached state read from `oa-state-file'.")

(defun oa-config--read-state ()
  "Read the state structure from `oa-state-file'."
  (if (file-readable-p oa-state-file)
      (with-temp-buffer
        (insert-file-contents oa-state-file)
        (goto-char (point-min))
        (condition-case nil
            (read (current-buffer))
          (error nil)))
    nil))

(defun oa-config-get-state ()
  "Return active state structure, loading lazily if necessary."
  (or oa-config--state-cache
      (setq oa-config--state-cache
            (or (oa-config--read-state)
                (list :version 1 :active-endpoint nil :endpoints nil)))))

(defun oa-config-save-state (state)
  "Atomically save STATE to `oa-state-file'."
  (setq oa-config--state-cache state)
  (let* ((dir (file-name-directory oa-state-file))
         (temp-file (make-temp-name (expand-file-name "state-" dir))))
    (unless (file-directory-p dir)
      (make-directory dir t))
    (with-temp-file temp-file
      (let ((print-length nil)
            (print-level nil))
        (prin1 state (current-buffer))))
    (rename-file temp-file oa-state-file t)))

(defun oa-config-active-endpoint ()
  "Return plist of active endpoint, or nil if none."
  (let* ((state (oa-config-get-state))
         (active-name (plist-get state :active-endpoint))
         (endpoints (plist-get state :endpoints)))
    (if active-name
        (seq-find (lambda (ep) (string-equal (plist-get ep :name) active-name))
                  endpoints)
      (car endpoints))))

(defun oa-config-resolve-key (key-source &optional url)
  "Resolve API key string from KEY-SOURCE plist, or return nil."
  (pcase (plist-get key-source :type)
    ('none nil)
    ('env (let ((var (plist-get key-source :name)))
            (and var (getenv var))))
    ('auth-source
     (let* ((host (or (plist-get key-source :host)
                      (and url (url-host (url-generic-parse-url url)))
                      "localhost"))
            (auth (car (auth-source-search :host host :require '(:secret)))))
       (when auth
         (let ((sec (plist-get auth :secret)))
           (if (functionp sec) (funcall sec) sec)))))
    (_ nil)))

(defun oa-config-update-endpoint (name update-fn)
  "Update endpoint named NAME with UPDATE-FN and persist."
  (let* ((state (oa-config-get-state))
         (endpoints (plist-get state :endpoints))
         (found nil)
         (new-endpoints
          (mapcar (lambda (ep)
                    (if (string-equal (plist-get ep :name) name)
                        (progn (setq found t) (funcall update-fn ep))
                      ep))
                  endpoints)))
    (unless found
      (setq new-endpoints (append endpoints (list (funcall update-fn (list :name name))))))
    (setq state (plist-put state :endpoints new-endpoints))
    (oa-config-save-state state)))

;;;###autoload
(defun oa-select-endpoint ()
  "Interactively select active endpoint."
  (interactive)
  (let* ((state (oa-config-get-state))
         (endpoints (plist-get state :endpoints))
         (names (mapcar (lambda (ep) (plist-get ep :name)) endpoints)))
    (unless names
      (user-error "No endpoints configured. Use `oa-add-endpoint'"))
    (let* ((current (plist-get state :active-endpoint))
           (choices (if current
                        (cons current (delete current (copy-sequence names)))
                      names))
           (chosen (completing-read "Select endpoint: " choices nil t)))
      (setq state (plist-put state :active-endpoint chosen))
      (oa-config-save-state state)
      (message "Active endpoint: %s" chosen))))

;;;###autoload
(defun oa-select-model ()
  "Interactively select model for active endpoint with free-text fallback."
  (interactive)
  (let ((ep (oa-config-active-endpoint)))
    (unless ep
      (user-error "No active endpoint. Configure one with `oa-add-endpoint'"))
    (let* ((cached (plist-get ep :cached-models))
           (current (plist-get ep :selected-model))
           (candidates (mapcar (lambda (item)
                                 (if (consp item) (car item) item))
                               cached))
           (choices (if current
                        (cons current (delete current (copy-sequence candidates)))
                      candidates))
           (chosen (completing-read
                    (format "Select model (current: %s): " (or current "none"))
                    choices nil nil nil nil current)))
      (when (and chosen (not (string-empty-p chosen)))
        (oa-config-update-endpoint
         (plist-get ep :name)
         (lambda (e) (plist-put e :selected-model chosen)))
        (message "Selected model: %s" chosen)))))

(provide 'oa-config)
;;; oa-config.el ends here
