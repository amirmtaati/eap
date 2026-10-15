EMACS ?= $(shell which emacs 2>/dev/null || echo "flatpak run org.gnu.emacs")

.PHONY: test compile clean

test:
	$(EMACS) -Q --batch -L . -L test/ -l test/oa-test.el -f ert-run-tests-batch-and-exit

compile:
	$(EMACS) -Q --batch -L . --eval "(setq byte-compile-error-on-warn t)" -f batch-byte-compile *.el

clean:
	rm -f *.elc
