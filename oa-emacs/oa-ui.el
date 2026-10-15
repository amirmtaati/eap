;;; oa-ui.el --- Output buffer, transient menu, and UI widgets for Org Agent -*- lexical-binding: t; -*-

;; Author: Org Agent Team
;; Keywords: convenience, faces, org

;;; Commentary:
;; Renders streaming output buffers with interactive header lines, provides transient
;; command menus, and implements the diagnostic `oa-doctor'.

;;; Code:

(require 'org)
(require 'transient)
(require 'subr-x)
(require 'oa-config)
(require 'oa-engine)

(declare-function oa-send "oa" (&optional focus-line))
(declare-function oa-send-focus "oa" ())
(declare-function oa-preview "oa" (&optional focus-line))
(declare-function oa-accept "oa" ())
(declare-function oa-cancel "oa" ())
(declare-function oa-add-endpoint "oa" ())
(declare-function oa-set-key "oa" (&optional key))
(declare-function oa-refresh-models "oa" ())

(defvar-local oa-ui--current-state "idle"
  "Current request state string for header line.")

(defvar-local oa-ui--stream-chars 0
  "Number of characters received in the current streaming response.")

(defvar-local oa-ui--last-endpoint nil
  "Endpoint plist of the last request in this buffer.")

(defvar-local oa-ui--last-model nil
  "Model string of the last request in this buffer.")

(defvar oa-output-mode-map
  (let ((map (make-sparse-keymap)))
    (define-key map (kbd "C-c C-c") #'oa-accept)
    (define-key map (kbd "C-c C-k") #'oa-cancel)
    (define-key map (kbd "C-c C-l") #'oa-ui-clear-output)
    (define-key map (kbd "g") #'oa-send)
    (define-key map (kbd "q") #'quit-window)
    map)
  "Keymap for `oa-output-mode'.")

(define-derived-mode oa-output-mode org-mode "OA-Output"
  "Major mode for Org Agent output buffers."
  (setq buffer-read-only t)
  (setq header-line-format
        '(:eval (oa-ui--format-header-line))))

(defun oa-ui--format-header-line ()
  "Format the header line showing active endpoint, model, and status."
  (let* ((ep (or oa-ui--last-endpoint (oa-config-active-endpoint)))
         (ep-name (if ep (plist-get ep :name) "no-endpoint"))
         (model (or oa-ui--last-model (and ep (plist-get ep :selected-model)) "no-model"))
         (status oa-ui--current-state))
    (format " [%s · %s]  State: %s"
            (propertize ep-name 'face 'bold)
            (propertize model 'face 'font-lock-keyword-face)
            (propertize status 'face 'font-lock-string-face))))

(defun oa-ui-get-output-buffer (project-name)
  "Return or create the output buffer for PROJECT-NAME."
  (let* ((buf-name (format "*oa: %s*" project-name))
         (buf (get-buffer-create buf-name)))
    (with-current-buffer buf
      (unless (derived-mode-p 'oa-output-mode)
        (oa-output-mode)))
    buf))

(defun oa-ui-show-output (&optional project-name)
  "Display the output buffer for PROJECT-NAME in a side window."
  (interactive)
  (let* ((name (or project-name
                   (and (buffer-file-name) (file-name-base (buffer-file-name)))
                   "default"))
         (buf (oa-ui-get-output-buffer name)))
    (display-buffer buf oa-display-buffer-action)))

(defun oa-ui-clear-output ()
  "Clear output text from current buffer."
  (interactive)
  (let ((inhibit-read-only t))
    (erase-buffer)
    (message "Output buffer cleared")))

(defun oa-ui-append-stream (buf text)
  "Append streaming TEXT into BUF respecting point position."
  (when (buffer-live-p buf)
    (with-current-buffer buf
      (let ((inhibit-read-only t)
            (moving (= (point) (point-max))))
        (save-excursion
          (goto-char (point-max))
          (insert text))
        (when moving
          (goto-char (point-max)))
        (setq oa-ui--stream-chars (+ oa-ui--stream-chars (length text)))
        (setq oa-ui--current-state (format "streaming (%d chars)" oa-ui--stream-chars))
        (force-mode-line-update)))))

(defun oa-ui-insert-header (buf model endpoint tokens-est delta-count)
  "Insert Org heading block for a new response into BUF."
  (with-current-buffer buf
    (let ((inhibit-read-only t)
          (ts (format-time-string "%Y-%m-%d %H:%M")))
      (setq oa-ui--stream-chars 0)
      (setq oa-ui--last-endpoint endpoint)
      (setq oa-ui--last-model model)
      (setq oa-ui--current-state "sending")
      (goto-char (point-max))
      (unless (bobp) (insert "\n\n"))
      (insert (format "* %s %s @ %s\n" ts model (plist-get endpoint :name)))
      (insert ":PROPERTIES:\n")
      (insert (format ":TOKENS_EST: %d\n" (or tokens-est 0)))
      (insert (format ":CHANGES: %d\n" (or delta-count 0)))
      (insert ":END:\n\n")
      (force-mode-line-update))))

;;; Transient Menu

;;;###autoload (autoload 'oa-menu "oa-ui" nil t)
(transient-define-prefix oa-menu ()
  "Org Agent command menu."
  [:description
   (lambda ()
     (let* ((ep (oa-config-active-endpoint))
            (ep-name (if ep (plist-get ep :name) "None"))
            (model (if ep (or (plist-get ep :selected-model) "None") "None")))
       (format "Org Agent (Endpoint: %s | Model: %s)" ep-name model)))
   ["Send"
    ("s" "Send changes" oa-send)
    ("f" "Send focus" oa-send-focus)
    ("p" "Preview prompt" oa-preview)]
   ["Model"
    ("m" "Select model" oa-select-model)
    ("r" "Refresh cache" oa-refresh-models)]
   ["Endpoint"
    ("e" "Select endpoint" oa-select-endpoint)
    ("E" "Add endpoint" oa-add-endpoint)
    ("K" "Paste API key" oa-set-key)]
   ["Output & Actions"
    ("o" "Show output" oa-show-output)
    ("a" "Accept response" oa-accept)
    ("k" "Cancel request" oa-cancel)
    ("d" "Run doctor" oa-doctor)]])

;;; Doctor Diagnostics

;;;###autoload
(defun oa-doctor ()
  "Inspect system configuration and print diagnostic report."
  (interactive)
  (let* ((buf (get-buffer-create "*oa-doctor*"))
         (bin (condition-case nil (oa-engine-find-binary) (error nil)))
         (proto-ok (and bin (condition-case nil (oa-engine-check-protocol) (error nil))))
         (ep (oa-config-active-endpoint))
         (url (and ep (plist-get ep :url)))
         (key-src (and ep (plist-get ep :key-source)))
         (key-found (and key-src (not (null (oa-config-resolve-key key-src url))))))
    (with-current-buffer buf
      (let ((inhibit-read-only t))
        (erase-buffer)
        (insert "=== Org Agent Diagnostic Report ===\n\n")
        (insert (format "Engine binary:       %s\n" (or bin "NOT FOUND")))
        (insert (format "Protocol match (v1): %s\n" (if proto-ok "YES" "NO / MISMATCH")))
        (insert (format "Active Endpoint:     %s\n" (if ep (plist-get ep :name) "None configured")))
        (insert (format "Endpoint URL:        %s\n" (or url "N/A")))
        (insert (format "Model selected:      %s\n" (if ep (or (plist-get ep :selected-model) "None") "N/A")))
        (insert (format "Key source resolved: %s\n"
                        (cond
                         ((null ep) "N/A")
                         ((equal (plist-get key-src :type) 'none) "None required")
                         (key-found "Key located (redacted)")
                         (t "MISSING / UNRESOLVED"))))
        (insert (format "Snapshot target:     %s\n\n" oa-snapshot-file))
        (insert "Ready status: ")
        (if (and bin proto-ok ep)
            (insert (propertize "READY FOR AGENT WORK" 'face 'font-lock-keyword-face))
          (insert (propertize "ATTENTION REQUIRED" 'face 'font-lock-warning-face)))
        (insert "\n")))
    (display-buffer buf)))

(provide 'oa-ui)
;;; oa-ui.el ends here
