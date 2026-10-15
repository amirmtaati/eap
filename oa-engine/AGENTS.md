# oa-engine agent notes

- Read the SPEC.md master digest first: layout, wire contracts, and exit codes live there.
- Stay inside the declared task scope; never edit packages outside your assignment.
- Go standard library only: no third-party imports in `cmd/` or `internal/`.
- Every behavior change ships with table-driven tests in the same package.
- Run `make ci` before reporting anything done; paste the real output.
- Never weaken, skip, or delete existing tests to make them pass.
- All stdout output must be deterministic: sort or pre-order everything emitted.
- Keep exit codes stable: 0 ok, 1 usage/IO, 2 nothing to send, 3 unsupported snapshot.
- Never log or accept `OA_API_KEY` outside the environment.
- If the default build cache is read-only, run targets with `GOCACHE=/tmp/oa-gocache`.
