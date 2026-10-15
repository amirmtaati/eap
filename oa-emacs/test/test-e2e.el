;;; test-e2e.el --- End-to-end automated integration tests for oa-emacs -*- lexical-binding: t; -*-

(require 'ert)
(require 'oa)

(defconst test-mock-url "http://127.0.0.1:20130/v1")
(defconst test-mock-model "mock/qwen2.5-coder-32b")

(ert-deftest test-oa-doctor ()
  "Test that oa-doctor runs and verifies system compatibility."
  (oa-doctor)
  (let ((buf (get-buffer "*oa-doctor*")))
    (should (buffer-live-p buf))
    (with-current-buffer buf
      (should (string-match-p "Engine binary:" (buffer-string)))
      (should (string-match-p "Protocol match (v1): YES" (buffer-string))))))

(ert-deftest test-oa-endpoint-and-models ()
  "Test configuring an endpoint and fetching models from mockrouter."
  (let ((models (oa-engine-fetch-models test-mock-url nil)))
    (should (> (length models) 0))
    (should (assoc "mock/qwen2.5-coder-32b" models))
    ;; Update endpoint state
    (oa-config-update-endpoint
     "test-mock"
     (lambda (ep)
       (plist-put (plist-put (plist-put ep :url test-mock-url)
                             :key-source '(:type none))
                  :cached-models models)))
    ;; Set active endpoint and model
    (let ((state (oa-config-get-state)))
      (setq state (plist-put state :active-endpoint "test-mock"))
      (oa-config-save-state state))
    (oa-config-update-endpoint
     "test-mock"
     (lambda (ep) (plist-put ep :selected-model test-mock-model)))
    ;; Verify active endpoint
    (let ((active (oa-config-active-endpoint)))
      (should (equal (plist-get active :name) "test-mock"))
      (should (equal (plist-get active :selected-model) test-mock-model)))))

(ert-deftest test-oa-workflow-in-org-file ()
  "Test full org-mode workflow: open doc, delta detection, stream, output buffer, accept."
  (let* ((temp-dir (make-temp-file "oa-test-" t))
         (org-file (expand-file-name "test-project.org" temp-dir))
         (snap-file (expand-file-name ".oa/snapshot.json" temp-dir)))
    (unwind-protect
        (progn
          ;; 1. Create org file with #+OA: t
          (with-temp-file org-file
            (insert "#+TITLE: Test Project\n#+OA: t\n\n* Architecture :ctx:\nCore system overview.\n\n* Tasks\n** TODO First Task\nInitial work item.\n"))

          ;; 2. Visit file and verify oa-mode activates
          (let ((buf (find-file-noselect org-file)))
            (with-current-buffer buf
              (should (derived-mode-p 'org-mode))
              (oa-maybe-enable)
              (should (bound-and-true-p oa-mode))

              ;; 3. Take initial snapshot
              (make-directory (file-name-directory snap-file) t)
              (oa-engine-run-snapshot (buffer-string) snap-file)
              (should (file-exists-p snap-file))

              ;; 4. Test zero delta (nothing new to send)
              (let ((prompt-res (oa-engine-run-prompt (buffer-string) snap-file)))
                (should (= (car prompt-res) 2))
                (should (string-match-p "nothing to send" (cdr prompt-res))))

              ;; 5. Edit document (add new task)
              (goto-char (point-max))
              (insert "\n** TODO Second Task :urgent:\nAdded in testing.\n")
              (save-buffer)

              ;; 6. Preview prompt
              (let ((prompt-res (oa-engine-run-prompt (buffer-string) snap-file)))
                (should (= (car prompt-res) 0))
                (should (string-match-p "Added: tasks/todo-second-task" (cdr prompt-res))))

              ;; 7. Test sending stream to mockrouter
              (setq oa-confirm-before-send nil) ;; do not prompt in batch mode
              (setq oa-accept-mode 'manual)
              (oa-send)

              ;; 8. Wait for background chat process to complete
              (let ((proc oa--active-process)
                    (timeout 10.0)
                    (start-time (float-time)))
                (should (processp proc))
                (while (and (process-live-p proc)
                            (< (- (float-time) start-time) timeout))
                  (accept-process-output proc 0.1))
                (should-not (process-live-p proc))
                (should (= (process-exit-status proc) 0)))

              ;; 9. Inspect *oa: test-project* output buffer
              (let ((out-buf (get-buffer "*oa: test-project*")))
                (should (buffer-live-p out-buf))
                (with-current-buffer out-buf
                  (should (string-match-p "Proposed Solution" (buffer-string)))
                  (should (string-match-p "Updated implementation plan" (buffer-string)))
                  (should (string-match-p "done" (or oa-ui--current-state "")))))

              ;; 10. Accept response into document
              (oa-accept)
              (should (string-match-p "\\* Agent :agent:" (buffer-string)))
              (should (string-match-p "Response to: changes" (buffer-string)))
              ;; Verify heading re-leveling: ** Proposed Solution -> *** Proposed Solution
              (should (string-match-p "\\*\\*\\* Proposed Solution" (buffer-string)))

              ;; 11. Verify subsequent send reports nothing to send
              (let ((prompt-res (oa-engine-run-prompt (buffer-string) snap-file)))
                (should (= (car prompt-res) 2)))

              (kill-buffer buf))))
      ;; Cleanup temp directory
      (delete-directory temp-dir t))))
