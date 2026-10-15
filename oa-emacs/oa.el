;;; oa.el --- Org Agent for Emacs -*- lexical-binding: t; -*-

;; Author: Org Agent Team
;; Version: 0.1.0
;; Package-Requires: ((emacs "29.1"))
;; Keywords: convenience, tools, org
;; URL: https://github.com/amirmtaati/eap

;;; Commentary:
;; An agentic development package driven by a single Org-mode document.
;; Connects to OpenAI-compatible LLM endpoints through the Go `oa' engine.

;;; Code:

(require 'org)
(require 'subr-x)
(require 'oa-config)
(require 'oa-doc)
(require 'oa-engine)
(require 'oa-ui)

(defvar-local oa--active-process nil
  "Running background `oa chat' process for the current project buffer.")

(defvar-local oa--last-sent-text nil
  "Text captured from the project buffer at send time.")

(defvar-local oa--last-response-text nil
  "Full response text received from the last completed request.")

(defvar-local oa--last-topic nil
  "Topic string derived from the last request.")

(defvar oa-mode-map
  (let ((map (make-sparse-keymap)))
    (define-key map (kbd (concat oa-keymap-prefix " s")) #'oa-send)
    (define-key map (kbd (concat oa-keymap-prefix " f")) #'oa-send-focus)
    (define-key map (kbd (concat oa-keymap-prefix " p")) #'oa-preview)
    (define-key map (kbd (concat oa-keymap-prefix " m")) #'oa-select-model)
    (define-key map (kbd (concat oa-keymap-prefix " e")) #'oa-select-endpoint)
    (define-key map (kbd (concat oa-keymap-prefix " E")) #'oa-add-endpoint)
    (define-key map (kbd (concat oa-keymap-prefix " r")) #'oa-refresh-models)
    (define-key map (kbd (concat oa-keymap-prefix " o")) #'oa-show-output)
    (define-key map (kbd (concat oa-keymap-prefix " k")) #'oa-cancel)
    (define-key map (kbd (concat oa-keymap-prefix " a")) #'oa-accept)
    (define-key map (kbd (concat oa-keymap-prefix " ?")) #'oa-menu)
    map)
  "Keymap for `oa-mode'.")

;;;###autoload
(define-minor-mode oa-mode
  "Minor mode for Org Agent document workflow."
  :lighter " OA"
  :keymap oa-mode-map
  :group 'oa)

;;;###autoload
(defun oa-maybe-enable ()
  "Enable `oa-mode' if current buffer is marked as an Org Agent document.
Checks for `#+OA: t' keyword or an adjacent `.oa/' directory."
  (when (derived-mode-p 'org-mode)
    (save-excursion
      (goto-char (point-min))
      (if (or (re-search-forward "^#\\+OA:[ \t]*t" 1000 t)
              (and buffer-file-name
                   (file-directory-p
                    (expand-file-name ".oa" (file-name-directory buffer-file-name)))))
          (oa-mode 1)))))

(defun oa--project-name ()
  "Return active project identifier."
  (if buffer-file-name
      (file-name-base buffer-file-name)
    "default"))

(defun oa--snapshot-path ()
  "Return absolute path to snapshot file for current buffer."
  (if buffer-file-name
      (expand-file-name oa-snapshot-file (file-name-directory buffer-file-name))
    (expand-file-name "snapshot.json" temporary-file-directory)))

;;;###autoload
(defun oa-add-endpoint ()
  "Interactively add a new endpoint configuration with validation."
  (interactive)
  (let* ((name (read-string "Endpoint name: "))
         (url (string-trim-right
               (read-string "Endpoint base URL (e.g. http://localhost:20128/v1): "
                            "http://localhost:20128/v1")
               "/"))
         (auth-type (intern
                     (completing-read "Authentication source: "
                                      '("none" "env" "auth-source") nil t nil nil "none")))
         (key-source
          (pcase auth-type
            ('none (list :type 'none))
            ('env
             (let ((var (read-string "Environment variable name: " "OA_API_KEY")))
               (list :type 'env :name var)))
            ('auth-source
             (let ((host (read-string "auth-source host: " "localhost")))
               (list :type 'auth-source :host host)))))
         (api-key (oa-config-resolve-key key-source url)))
    (message "Validating endpoint '%s'..." name)
    (condition-case err
        (let ((models (oa-engine-fetch-models url api-key)))
          (message "Found %d models." (length models))
          (oa-config-update-endpoint
           name
           (lambda (ep)
             (plist-put (plist-put (plist-put (plist-put ep :url url)
                                              :key-source key-source)
                                  :cached-models models)
                        :cache-timestamp (float-time))))
          (let ((state (oa-config-get-state)))
            (setq state (plist-put state :active-endpoint name))
            (oa-config-save-state state))
          (when (and models (y-or-n-p "Select a model for this endpoint now? "))
            (oa-select-model)))
      (error
       (if (y-or-n-p (format "Endpoint validation failed (%s). Save anyway? " err))
           (progn
             (oa-config-update-endpoint
              name
              (lambda (ep)
                (plist-put (plist-put ep :url url) :key-source key-source)))
             (message "Endpoint saved without model cache."))
         (message "Endpoint discarded."))))))

;;;###autoload
(defun oa-refresh-models ()
  "Refresh models list from active endpoint."
  (interactive)
  (let ((ep (oa-config-active-endpoint)))
    (unless ep
      (user-error "No active endpoint to refresh"))
    (let* ((url (plist-get ep :url))
           (api-key (oa-config-resolve-key (plist-get ep :key-source) url)))
      (message "Refreshing models from %s..." url)
      (condition-case err
          (let ((models (oa-engine-fetch-models url api-key)))
            (oa-config-update-endpoint
             (plist-get ep :name)
             (lambda (e)
               (plist-put (plist-put e :cached-models models)
                          :cache-timestamp (float-time))))
            (message "Cached %d models from %s." (length models) (plist-get ep :name)))
        (error
         (message "Failed to refresh models: %s" err))))))

;;;###autoload
(defun oa-send (&optional focus-line)
  "Send document changes to LLM endpoint.
FOCUS-LINE optionally restricts context to enclosing headline."
  (interactive)
  (let ((ep (oa-config-active-endpoint)))
    (unless ep
      (oa-add-endpoint)
      (setq ep (oa-config-active-endpoint)))
    (unless (and ep (plist-get ep :selected-model))
      (oa-select-model)
      (setq ep (oa-config-active-endpoint)))
    (when (and oa--active-process (process-live-p oa--active-process))
      (if (y-or-n-p "A request is already running. Cancel it and restart? ")
          (oa-cancel)
        (user-error "Aborted")))
    (let* ((orig-buf (current-buffer))
           (proj-name (oa--project-name))
           (snap-path (oa--snapshot-path))
           (sent-text (oa-doc-capture-buffer orig-buf))
           (model (plist-get ep :selected-model))
           (url (plist-get ep :url)))
      (setq oa--last-sent-text sent-text)
      (setq oa--last-response-text "")
      (setq oa--last-topic
            (if focus-line
                (save-excursion
                  (goto-line focus-line)
                  (org-get-heading t t t t))
              "changes"))
      ;; Check delta with prompt call
      (let ((prompt-res (oa-engine-run-prompt sent-text snap-path focus-line)))
        (when (= (car prompt-res) 2)
          (user-error "Nothing new to send. Use C-c a f to send the subtree at point"))
        (when (/= (car prompt-res) 0)
          (user-error "Engine prompt error: %s" (cdr prompt-res)))
        ;; Confirm before send
        (pcase oa-confirm-before-send
          ('summary
           (unless (y-or-n-p (format "Send changes to %s (%s)? " (plist-get ep :name) model))
             (user-error "Cancelled")))
          ('preview
           (oa-preview focus-line)
           (unless (y-or-n-p "Send prompt? ")
             (user-error "Cancelled")))))
      ;; Dispatch chat stream
      (let* ((out-buf (oa-ui-get-output-buffer proj-name)))
        (oa-ui-show-output proj-name)
        (oa-ui-insert-header out-buf model ep 0 1)
        (setq oa--active-process
              (oa-engine-start-chat
               sent-text url model snap-path focus-line
               (list
                :on-start
                (lambda (_ev)
                  (when (buffer-live-p out-buf)
                    (with-current-buffer out-buf
                      (setq oa-ui--current-state "streaming")
                      (force-mode-line-update))))
                :on-delta
                (lambda (txt)
                  (with-current-buffer orig-buf
                    (setq oa--last-response-text
                          (concat oa--last-response-text txt)))
                  (oa-ui-append-stream out-buf txt))
                :on-done
                (lambda (_reason)
                  (when (buffer-live-p out-buf)
                    (with-current-buffer out-buf
                      (setq oa-ui--current-state "done")
                      (force-mode-line-update)))
                  (message "Response complete.")
                  (pcase oa-accept-mode
                    ('auto (with-current-buffer orig-buf (oa-accept)))
                    ('ask (when (y-or-n-p "Accept response into the document? ")
                            (with-current-buffer orig-buf (oa-accept))))))
                :on-error
                (lambda (kind msg)
                  (when (buffer-live-p out-buf)
                    (with-current-buffer out-buf
                      (setq oa-ui--current-state (format "error: %s" kind))
                      (force-mode-line-update)))
                  (message "Request failed (%s): %s" kind msg)))))))))

;;;###autoload
(defun oa-send-focus ()
  "Send current headline subtree at point as focus."
  (interactive)
  (unless (derived-mode-p 'org-mode)
    (user-error "Must be in an Org-mode buffer"))
  (oa-send (line-number-at-pos)))

;;;###autoload
(defun oa-preview (&optional focus-line)
  "Show assembled prompt in a preview buffer without sending."
  (interactive)
  (let* ((snap-path (oa--snapshot-path))
         (text (oa-doc-capture-buffer))
         (res (oa-engine-run-prompt text snap-path focus-line))
         (buf (get-buffer-create "*oa-preview*")))
    (if (= (car res) 2)
        (message "Nothing new to send.")
      (with-current-buffer buf
        (let ((inhibit-read-only t))
          (erase-buffer)
          (insert (cdr res))
          (goto-char (point-min))))
      (display-buffer buf))))

;;;###autoload
(defun oa-cancel ()
  "Cancel active request."
  (interactive)
  (if (and oa--active-process (process-live-p oa--active-process))
      (progn
        (oa-engine-cancel oa--active-process)
        (setq oa--active-process nil)
        (message "Request cancelled."))
    (message "No active request to cancel.")))

;;;###autoload
(defun oa-accept ()
  "Commit last response into document and update project snapshot."
  (interactive)
  (unless (and oa--last-response-text (not (string-empty-p oa--last-response-text)))
    (user-error "No response available to accept"))
  (oa-doc-writeback (current-buffer) oa--last-response-text
                    (let ((ep (oa-config-active-endpoint)))
                      (and ep (plist-get ep :selected-model)))
                    oa--last-topic)
  ;; Update snapshot using send-time captured text
  (when oa--last-sent-text
    (oa-engine-run-snapshot oa--last-sent-text (oa--snapshot-path)))
  (setq oa--last-response-text nil)
  (message "Accepted response into * Agent :agent: and updated snapshot."))

;;;###autoload
(defun oa-show-output ()
  "Display output buffer for current project."
  (interactive)
  (oa-ui-show-output (oa--project-name)))

(provide 'oa)
;;; oa.el ends here
