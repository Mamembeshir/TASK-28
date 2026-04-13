# Delivery Acceptance Issue Re-review (Static) — Fresh Report

Date: 2026-04-13  
Method: Static-only verification (no runtime/test execution in this pass)

## Overall Verdict

- **Pass for the previously reported issue set**
- All five previously tracked findings are now resolved in code/docs/tests, based on static evidence.

## Re-check Results

### 1) High: Startup seed path storing non-encrypted password hashes
- Status: **Fixed**
- Evidence:
  - Seed requires valid encryption key length: `cmd/server/main.go:189`.
  - Seed password flow hashes then encrypts before persistence: `cmd/server/main.go:194`, `cmd/server/main.go:199`.
  - Encrypted value is written to `users.password_hash`: `cmd/server/main.go:231`, `cmd/server/main.go:239`.

### 2) Medium: API spec mismatch for `/` auth behavior
- Status: **Fixed**
- Evidence:
  - API spec now marks home route authenticated: `docs/api-spec.md:29`.
  - Router protects `/` behind auth middleware group: `internal/app/router.go:167`, `internal/app/router.go:172`.

### 3) Medium: Like-ring cron semantics (one-way falsely treated as mutual)
- Status: **Fixed**
- Evidence:
  - Detection query enforces reciprocal directional pair matching: `internal/cron/jobs.go:137`, `internal/cron/jobs.go:148`.
  - Pair deduplication present: `internal/cron/jobs.go:149`.

### 4) Medium: Coverage gaps (retry queue and anomaly assertions)
- Status: **Fixed**
- Evidence:
  - Retry backoff schedule values now asserted with timing tolerance: `tests/integration/messaging_test.go:382`, `tests/integration/messaging_test.go:436`.
  - Retry terminal behavior asserted (`attempts=5`, `FAILED`): `tests/integration/messaging_test.go:464`, `tests/integration/messaging_test.go:465`.
  - Positive anomaly path asserted: `tests/integration/workflow_test.go:763`, `tests/integration/moderation_test.go:511`.
  - Negative anomaly path added (one-way votes must produce zero flags): `tests/integration/moderation_test.go:515`, `tests/integration/moderation_test.go:569`.

### 5) Low: Quick-start secret requirements clarity
- Status: **Fixed**
- Evidence:
  - Quick Start explicitly instructs required secrets: `README.md:14`, `README.md:18`, `README.md:19`, `README.md:22`.

## Notes

- This report only re-validates the previously reported issues list; it is not a full new end-to-end architecture audit.
- Runtime behavior (cron firing, SSE delivery, containerized test DB wiring) still requires manual execution verification outside static review.
