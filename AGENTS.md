# Agent Guidelines

This is a Go service. Keep changes small, idiomatic, and easy to review.

## Go Style

- Follow Effective Go: prefer clear package names, short exported names that read well with the package qualifier, `gofmt` formatting, and simple control flow.
- Use the standard library first. Add dependencies only when they clearly reduce production risk or complexity.
- Keep interfaces small and close to their callers. Do not add abstractions for a single implementation unless they preserve an existing extension seam.
- Return errors with useful context and wrap underlying errors with `%w` when callers may need the cause.
- Keep comments focused on behavior, contracts, and non-obvious assumptions. Avoid tool-specific labels in code comments.

## Project Expectations

- Preserve the datastore extension seam: `IP2COUNTRY_DB` selects the backend, and each backend should satisfy the `geoip.Store` contract.
- Keep configuration centralized in `internal/config`; load environment values once at startup and validate eagerly.
- Keep HTTP responses aligned with the PRD: successful lookups return `{"country":"...","city":"..."}`, and errors return `{"error":"..."}` with the appropriate status code.
- Keep rate limiting explicit and testable. Do not use third-party rate-limit libraries or `golang.org/x/time/rate`.
- Treat CSV input as `from,to,City,Country` rows unless the PRD or tests are updated.

## Workflow

- Before editing, read the surrounding package and match its style.
- Prefer surgical changes over broad refactors. If a larger cleanup is needed, explain why before doing it.
- Use the Karpathy guidelines: make assumptions explicit, keep the solution simple, and verify the result.
- Run `gofmt` on changed Go files.
- Validate with `ponytail` when it is available in the environment. If it is not available, use Go-native checks instead:
  - `go test ./...`
  - `go vet ./...`
  - targeted tests for changed packages
  - e2e tests when HTTP behavior, configuration, or rate limiting changes

If Go commands fail because the default build cache is not writable, rerun them with `GOCACHE` pointing to a writable temp directory.
