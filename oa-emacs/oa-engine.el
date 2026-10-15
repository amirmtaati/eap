;;; oa-engine.el --- Process runner and NDJSON framing for Org Agent -*- lexical-binding: t; -*-

;; Author: Org Agent Team
;; Keywords: convenience, processes, org

;;; Commentary:
;; Spawns the Go `oa' binary, frames streamed NDJSON events over standard output,
;; handles cancellation, and runs version compatibility checks.

;;; Code:

(require 'cl-lib)
(require 'json)
(require 'subr-x)
(require 'oa-config)

(defconst oa-engine-expected-protocol 1
  "Required protocol version supported by this client.")

(defun oa-engine-find-binary ()
  "Locate the `oa' executable or signal a user error with build instructions."
  (or (executable-find oa-engine-program)
      (let ((local (expand-file-name "../oa-engine/bin/oa"
                                     (file-name-directory (or load-file-name buffer-file-name "")))))
        (and (file-executable-p local) local))
      (user-error
       "Engine \"%s\" not found. Build it (cd oa-engine && make build) and customize `oa-engine-program'"
       oa-engine-program)))

(defun oa-engine-check-protocol ()
  "Verify that the engine executable supports `oa-engine-expected-protocol'."
  (let* ((bin (oa-engine-find-binary))
         (out (with-temp-buffer
                (call-process bin nil (current-buffer) nil "version" "-json")
                (buffer-string)))
         (parsed (condition-case nil
                     (json-parse-string out :object-type 'plist)
                   (error nil)))
         (proto (and parsed (plist-get parsed :protocol))))
    (unless (equal proto oa-engine-expected-protocol)
      (user-error "Engine protocol mismatch: expected %d, got %s. Please recompile oa-engine"
                  oa-engine-expected-protocol (or proto "unknown")))
    t))

(defun oa-engine-fetch-models (url &optional api-key)
  "Fetch available models from URL using API-KEY. Return list of model ID strings."
  (let* ((bin (oa-engine-find-binary))
         (process-environment (if api-key
                                  (cons (format "OA_API_KEY=%s" api-key) process-environment)
                                process-environment))
         (out (with-temp-buffer
                (let ((code (call-process bin nil (list (current-buffer) nil) nil "models" "-url" url)))
                  (unless (= code 0)
                    (error "Engine error fetching models (exit %d): %s" code (string-trim (buffer-string))))
                  (buffer-string))))
         (parsed (json-parse-string out :object-type 'plist))
         (raw-models (plist-get parsed :models)))
    (mapcar (lambda (m)
              (cons (plist-get m :id) (plist-get m :owned_by)))
            raw-models)))

(defun oa-engine-run-prompt (text snapshot-path &optional focus-line focus-key)
  "Execute `oa prompt' on TEXT with SNAPSHOT-PATH.
Returns cons (EXIT-CODE . OUTPUT-OR-ERROR)."
  (let* ((bin (oa-engine-find-binary))
         (args (list "prompt" "-snapshot" snapshot-path))
         (args (if focus-line
                   (append args (list "-focus-line" (number-to-string focus-line)))
                 args))
         (args (if (and focus-key (not (string-empty-p focus-key)))
                   (append args (list "-focus" focus-key))
                 args))
         (args (append args (list "-"))))
    (with-temp-buffer
      (let* ((code (with-temp-buffer
                     (insert text)
                     (apply #'call-process-region
                            (point-min) (point-max)
                            bin nil (list (current-buffer) t) nil
                            args))))
        (cons code (string-trim (buffer-string)))))))

(defun oa-engine-run-snapshot (text snapshot-path)
  "Save snapshot from TEXT into SNAPSHOT-PATH. Returns exit code."
  (let ((bin (oa-engine-find-binary))
        (snap-dir (file-name-directory snapshot-path)))
    (when (and snap-dir (not (file-directory-p snap-dir)))
      (make-directory snap-dir t))
    (with-temp-buffer
      (insert text)
      (call-process-region (point-min) (point-max) bin nil nil nil
                           "snapshot" "-out" snapshot-path "-"))))

(defun oa-engine--filter-ndjson (proc string)
  "Process filter for PROC framing partial STRING chunks into NDJSON lines."
  (let ((partial (or (process-get proc :partial-line) ""))
        (callbacks (process-get proc :callbacks)))
    (setq string (concat partial string))
    (let ((lines (split-string string "\n")))
      ;; Last element is either empty (ended on \n) or incomplete
      (process-put proc :partial-line (car (last lines)))
      (dolist (line (butlast lines))
        (unless (string-empty-p (string-trim line))
          (condition-case err
              (let* ((event (json-parse-string line :object-type 'plist))
                     (type (plist-get event :event)))
                (pcase type
                  ("start" (when-let* ((cb (plist-get callbacks :on-start)))
                             (funcall cb event)))
                  ("delta" (when-let* ((cb (plist-get callbacks :on-delta)))
                             (funcall cb (plist-get event :text))))
                  ("usage" (when-let* ((cb (plist-get callbacks :on-usage)))
                             (funcall cb
                                      (plist-get event :prompt_tokens)
                                      (plist-get event :completion_tokens))))
                  ("done" (when-let* ((cb (plist-get callbacks :on-done)))
                            (funcall cb (plist-get event :finish_reason))))
                  ("error" (when-let* ((cb (plist-get callbacks :on-error)))
                             (funcall cb
                                      (plist-get event :kind)
                                      (plist-get event :message))))))
            (error
             (when-let* ((cb (plist-get callbacks :on-error)))
               (funcall cb "bad_response" (format "JSON decode error: %s" err))))))))))

(defun oa-engine--sentinel (proc event)
  "Sentinel for PROC to handle unexpected termination."
  (let ((callbacks (process-get proc :callbacks)))
    (unless (process-live-p proc)
      (let ((code (process-exit-status proc)))
        (when-let* ((cb (plist-get callbacks :on-exit)))
          (funcall cb code event))))))

(defun oa-engine-start-chat (text url model snapshot-path &optional focus-line callbacks)
  "Spawn background `oa chat' streaming request on TEXT.
URL, MODEL, and SNAPSHOT-PATH specify request parameters.
FOCUS-LINE optionally targets an enclosing headline.
CALLBACKS is a plist of event handlers:
  :on-start, :on-delta, :on-usage, :on-done, :on-error, :on-exit.
Returns process object."
  (oa-engine-check-protocol)
  (let* ((bin (oa-engine-find-binary))
         (ep (oa-config-active-endpoint))
         (api-key (and ep (oa-config-resolve-key (plist-get ep :key-source) url)))
         (args (list "chat" "-url" url "-model" model "-snapshot" snapshot-path))
         (args (if focus-line
                   (append args (list "-focus-line" (number-to-string focus-line)))
                 args))
         (args (append args (list "-")))
         (proc-env (if api-key
                       (cons (format "OA_API_KEY=%s" api-key) process-environment)
                     process-environment))
         (proc (let ((process-environment proc-env))
                 (make-process
                  :name "oa-chat"
                  :command (cons bin args)
                  :connection-type 'pipe
                  :filter #'oa-engine--filter-ndjson
                  :sentinel #'oa-engine--sentinel
                  :noquery t))))
    (process-put proc :callbacks callbacks)
    (process-put proc :partial-line "")
    ;; Send captured text via stdin and close input pipe
    (process-send-string proc text)
    (process-send-eof proc)
    proc))

(defun oa-engine-cancel (proc)
  "Interrupt running PROC and terminate safely."
  (when (and proc (process-live-p proc))
    (interrupt-process proc)
    (run-with-timer 2.0 nil
                    (lambda (p)
                      (when (and p (process-live-p p))
                        (kill-process p)))
                    proc)))

(provide 'oa-engine)
;;; oa-engine.el ends here
