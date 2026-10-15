;;; oa-test.el --- Tests for oa-emacs -*- lexical-binding: t; -*-

(require 'ert)
(require 'oa-config)
(require 'oa-doc)
(require 'oa-engine)

(ert-deftest oa-test-doc-normalize-headings ()
  "Test heading level normalization under Agent responses."
  (let* ((input (concat "* Top Heading\n"
                        "Some body\n"
                        "** Sub Heading\n"
                        "#+BEGIN_SRC org\n"
                        "* Preserved inside block\n"
                        "#+END_SRC\n"))
         (output (oa-doc-normalize-response input 3)))
    (should (string-match-p "^\\*\\*\\* Top Heading" output))
    (should (string-match-p "^\\*\\*\\*\\* Sub Heading" output))
    (should (string-match-p "\\* Preserved inside block" output))))

(ert-deftest oa-test-config-key-resolution ()
  "Test resolving API keys from env and none."
  (should (null (oa-config-resolve-key '(:type none))))
  (setenv "OA_TEST_SECRET_KEY" "secret123")
  (should (equal "secret123"
                 (oa-config-resolve-key '(:type env :name "OA_TEST_SECRET_KEY")))))

(ert-deftest oa-test-protocol-constant ()
  "Test protocol constant equals 1."
  (should (= oa-engine-expected-protocol 1)))

(provide 'oa-test)
;;; oa-test.el ends here
