# Phase 1.5: REST and WS Foundation

Status: partial
Updated: 2026-03-10

## Goal

Complete the base runtime API surface for operations and monitoring.

## Already Implemented

- HTTP server wiring integrated in `cmd/cts-core/main.go`.
- Health endpoints available:
  - `/health`
  - `/ready`
  - `/live`
- Core middleware foundation is present.
- WS handler stub is present for future protocol layer.

## Not Completed Yet

- `/metrics` endpoint.
- Prometheus exporter wiring.
- REST and WS integration tests for health/runtime paths.
- Full WS runtime protocol layer:
  - `trader.register`
  - `trader.heartbeat`
  - protocol ack/error flow

## Scope to Close Phase 1.5

1. Add `/metrics` endpoint.
2. Register core runtime metrics (HTTP, DB, HSM, WS basic gauges/counters).
3. Add integration tests:
   - health endpoints
   - startup/shutdown behavior
   - metrics endpoint availability
4. Keep docs synced in same PR:
   - `README.md`
   - `DEVELOPMENT_PLAN.md`
   - `API_SPECIFICATION.md`

## Acceptance Criteria

- `/health`, `/ready`, `/live`, `/metrics` respond successfully in runtime.
- Metrics are scrape-ready for Prometheus.
- Integration tests cover healthy path and basic failure path.

## Code References

- `cmd/cts-core/main.go`
- `internal/api/`
- `internal/middleware/`
- `internal/ws/`

## Quick Verification

```bash
# tests
go test ./...

# local runtime checks
curl -sS http://127.0.0.1:8080/health
curl -sS http://127.0.0.1:8080/ready
curl -sS http://127.0.0.1:8080/live
curl -sS http://127.0.0.1:8080/metrics
```

## Notes

- This guide intentionally replaces older generated step-by-step instructions that referenced outdated paths and file names.
- Treat this file as the current closure checklist for Phase 1.5.
