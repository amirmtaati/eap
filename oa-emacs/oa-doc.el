;;; oa-doc.el --- Document capture, heading normalization, and writeback -*- lexical-binding: t; -*-

;; Author: Org Agent Team
;; Keywords: convenience, tools, org

;;; Commentary:
;; Normalizes LLM response headings, executes atomic document writeback under
;; `* Agent :agent:`, and triggers send-time snapshot commits.

;;; Code:

(require 'org)
(require 'subr-x)

(defun oa-doc-normalize-response (response-text target-level)
  "Normalize Org headings in RESPONSE-TEXT so shallowest level matches TARGET-LEVEL.
Headings inside block delimiters (`#+BEGIN_...' and `#+END_...') are untouched.
Deliberate simplification: single regex scan with block state tracker.
ponytail: simple line scanner; switch to org-element AST if complex nested drawers needed."
  (let* ((lines (split-string response-text "\n"))
         (in-block nil)
         (min-depth nil))
    ;; Pass 1: find minimum headline depth outside blocks
    (dolist (line lines)
      (let ((trimmed (string-trim line)))
        (cond
         ((string-match-p "\\`#\\+BEGIN_" trimmed) (setq in-block t))
         ((string-match-p "\\`#\\+END_" trimmed) (setq in-block nil))
         ((and (not in-block) (string-match "\\`\\(\\*+\\)[ \t]" line))
          (let ((d (length (match-string 1 line))))
            (setq min-depth (if min-depth (min min-depth d) d)))))))
    ;; Pass 2: shift headings
    (if (not min-depth)
        response-text
      (let ((shift (- target-level min-depth))
            (result nil))
        (setq in-block nil)
        (dolist (line lines)
          (let ((trimmed (string-trim line)))
            (cond
             ((string-match-p "\\`#\\+BEGIN_" trimmed)
              (setq in-block t)
              (push line result))
             ((string-match-p "\\`#\\+END_" trimmed)
              (setq in-block nil)
              (push line result))
             ((and (not in-block) (string-match "\\`\\(\\*+\\)\\([ \t].*\\)" line))
              (let* ((orig-len (length (match-string 1 line)))
                     (new-len (max 1 (+ orig-len shift)))
                     (stars (make-string new-len ?*)))
                (push (concat stars (match-string 2 line)) result)))
             (t
              (push line result)))))
        (string-join (nreverse result) "\n")))))

(defun oa-doc-capture-buffer (&optional buffer)
  "Return full text of BUFFER (defaults to current buffer) as string."
  (with-current-buffer (or buffer (current-buffer))
    (save-restriction
      (widen)
      (buffer-substring-no-properties (point-min) (point-max)))))

(defun oa-doc-writeback (target-buffer response-text model-id &optional topic)
  "Insert normalized RESPONSE-TEXT into TARGET-BUFFER under `* Agent :agent:'.
TOPIC is an optional string describing the prompt topic.
MODEL-ID is the identifier of the generating model.
Leaves all other nodes byte-identical."
  (with-current-buffer target-buffer
    (save-excursion
      (save-restriction
        (widen)
        ;; 1. Normalize response text so top response headings are depth 3 (***)
        (let* ((normalized (oa-doc-normalize-response response-text 3))
               (timestamp (format-time-string "%Y-%m-%d %H:%M"))
               (entry-title (format "** %s Response to: %s (%s)\n"
                                   timestamp
                                   (or topic "update")
                                   (or model-id "agent")))
               (agent-headline-pos nil))
          ;; 2. Locate `* Agent :agent:'
          (goto-char (point-min))
          (while (and (not agent-headline-pos)
                      (re-search-forward "^\\*+[ \t]+Agent[ \t]+:agent:" nil t))
            (setq agent-headline-pos (line-beginning-position)))
          ;; 3. If missing, insert at end of buffer
          (unless agent-headline-pos
            (goto-char (point-max))
            (unless (bolp) (insert "\n"))
            (insert "\n* Agent :agent:\n")
            (setq agent-headline-pos (point)))
          ;; 4. Move to end of Agent subtree and insert entry
          (goto-char agent-headline-pos)
          (org-end-of-subtree t t)
          (unless (bolp) (insert "\n"))
          (insert entry-title)
          (insert normalized)
          (unless (bolp) (insert "\n")))))))

(provide 'oa-doc)
;;; oa-doc.el ends here
