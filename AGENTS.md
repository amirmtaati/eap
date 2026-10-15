# AGENTS.md — eap workspace rules

The specs are the contract: `MASTER-SPEC.md` > `<dir>/SPEC.md`. Never change a contract silently.

1. Read `MASTER-SPEC.md` and the relevant `SPEC.md` before any work. Cite sections (`path §n`), never paste.
2. Stay inside your declared directory and task scope. Need out-of-scope access? Stop and report why.
3. Trust nothing unverified: run gates yourself, paste real output. Never accept a "tests pass" claim.
4. Separation of duties: implementer, tester, reviewer are different runs. Reviewer never reviews own code.
5. Stdlib-first: Go standard library only in `oa-engine/`; Emacs built-ins only in `oa-emacs/`. No new dependency without human approval.
6. Minimal working diff. No speculative abstractions. Mark deliberate simplifications `ponytail:`.
7. Secrets: `OA_API_KEY` via environment only. Never in argv, logs, fixtures, or committed files.
8. Local commits only; the human pushes to remotes.
9. When in doubt, ask. A short question is cheaper than a wrong merge.

Role definitions: `.master/agents/*.md`. Task ledger: `.master/PLAN.org`.