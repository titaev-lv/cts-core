# Phase 2 Smoke Runbook

Updated: 2026-03-14
Scope: CTS-Core Phase 2 runtime checks (WS lifecycle + restart/shutdown behavior)

## 1. Goal

Validate that CTS-Core is operational in docker compose and that the base lifecycle `connect -> register -> heartbeat -> disconnect` is observable.

## 2. Prerequisites

- Docker + Docker Compose available.
- Workspace root contains `docker-compose.yml`.
- Services are built (or pull/build is allowed on first run).

## 3. Quick Smoke (Automated)

From `services/cts-core`:

```bash
./scripts/smoke_phase2_ws_lifecycle.sh
```

Optional mode with trader service startup:

```bash
./scripts/smoke_phase2_ws_lifecycle.sh --with-trader
```

What the script verifies:
- compose services `mysql`, `hsm`, `cts-core` are up
- `/health` is reachable from host (`http://localhost:8081/health`)
- CTS-Core process is present in compose status
- WS lifecycle logs are present if trader traffic exists (`ws_register`, `ws_heartbeat`, `ws_disconnect|ws_timeout`)

## 4. Manual Lifecycle Check

1. Start core stack:

```bash
docker compose up -d mysql hsm cts-core
```

2. Verify health endpoint:

```bash
curl -sS http://localhost:8081/health
```

Expected:
- response JSON contains `status` (`ok` or `degraded` depending on external dependencies)
- `components.websocket`
- `components.scheduler`

3. Observe WS lifecycle logs in CTS-Core:

```bash
docker compose logs --since 5m cts-core | grep -E "ws_register|ws_heartbeat|ws_disconnect|ws_timeout"
```

Expected events in normal flow:
- `ws_register`
- `ws_heartbeat`
- `ws_disconnect` (graceful) OR `ws_timeout`

## 5. Restart Behavior Check

1. Restart CTS-Core while stack is running:

```bash
docker compose restart cts-core
```

2. Validate service comes back and health is reachable:

```bash
docker compose ps cts-core
curl -sS http://localhost:8081/health
```

3. Validate state continues to update (`state.updated_at` moves forward):

```bash
sleep 2
curl -sS http://localhost:8081/health
```

## 6. Shutdown Behavior Check

1. Trigger graceful stop:

```bash
docker compose stop cts-core
```

2. Validate container state:

```bash
docker compose ps cts-core
```

3. Start again and verify health:

```bash
docker compose start cts-core
curl -sS http://localhost:8081/health
```

## 7. Troubleshooting

- No `/health` response:
  - check `docker compose ps cts-core`
  - inspect logs: `docker compose logs --since 5m cts-core`
- No WS lifecycle logs:
  - ensure trader (or any WS client) actually sends `trader.register` + `trader.heartbeat`
- DB/HSM degraded:
  - check dependency containers and logs (`mysql`, `hsm`)

## 8. Exit Criteria Mapping

This runbook supports Sprint 6 acceptance checks:
- E2E baseline lifecycle visibility
- restart/shutdown validation
- operational smoke path in compose environment
