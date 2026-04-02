# Changelog

## v0.0.1 - 2026-04-02

### Features
- Deliver Phase 2 WebSocket runtime lifecycle for Trader channel (`trader.register`, `trader.register_ack`, `trader.heartbeat`) with runtime session persistence in `TRADER_SESSION`.
- Add scheduler runtime loop over active WS sessions plus latency sweep dispatch path for connectivity profiling.
- Expose operational endpoints and telemetry surfaces: `/health`, `/ready`, `/live`, `/metrics`, `/api/v1/version`.
- Persist Trader release metadata from WS register payload into `TRADER.RELEASE_VERSION` for fleet visibility and drift control.

### Security
- Enforce strict Trader WS identity policy: canonical `trader_id` is derived from mTLS certificate CN only.
- Restrict Trader WS admission to client certificates with `OU=Trading`.
- Harden WS ingress with protocol version checks, inbound rate limiting, request dedup window, payload size bounds, and unknown-action flood guard.

### Reliability
- Add startup log rotation so every process start writes into fresh log files.
- Merge WS inbound/outbound streams into a single runtime file `ws.log`.
- Align session liveness defaults for runtime traffic (`heartbeat_interval=60s`, `heartbeat_timeout=180s`, configurable `session.write_timeout`).

### Build & Release
- Add startup build metadata log line: `INIT START cts-core` with `release`, `commit`, `build_time`.
- Align Docker release identity policy with Trader:
  - exact tag on `HEAD` => release build,
  - commits after last tag => `${last_tag}-dev.${commits_since_tag}+${utc_timestamp}.${short_sha}`,
  - no tags in repository => build fails.
- Keep service semantic version (`main.version`) sourced from `VERSION`.

### Database & Migrations
- Add migration `004_trader_release_version.sql` for `TRADER.RELEASE_VERSION`.
- Make migration scripts DBeaver-friendly for line-by-line execution flow.
- Sync bootstrap SQL init path with `RELEASE_VERSION` initialization logic.

### Tests
- Extend logger tests for startup rotation, text formatting behavior, and merged WS stream output.
- Update config tests for single WS log path contract (`logging.ws_path`).
- Targeted scheduler/logger/config/cmd test suites pass for release state.

### Documentation
- Refresh WS transport and API docs to `release` register payload and `protocol_version=1` contract.
- Update runtime timing references to `60s/180s` heartbeat model.
- Update architecture logging notes for unified `ws.log` stream.
