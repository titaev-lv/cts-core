#!/usr/bin/env bash
set -euo pipefail

WITH_TRADER=0
for arg in "$@"; do
  case "$arg" in
    --with-trader)
      WITH_TRADER=1
      ;;
    *)
      echo "Unknown arg: $arg" >&2
      echo "Usage: $0 [--with-trader]" >&2
      exit 2
      ;;
  esac
done

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
COMPOSE_DIR="${COMPOSE_DIR:-$(cd "$SCRIPT_DIR/../../.." && pwd)}"
COMPOSE_FILE="${COMPOSE_FILE:-$COMPOSE_DIR/docker-compose.yml}"
CTS_HEALTH_URL="${CTS_HEALTH_URL:-http://localhost:8081/health}"

if ! command -v docker >/dev/null 2>&1; then
  echo "docker is required" >&2
  exit 1
fi

if [ ! -f "$COMPOSE_FILE" ]; then
  echo "compose file not found: $COMPOSE_FILE" >&2
  exit 1
fi

echo "[smoke] compose file: $COMPOSE_FILE"

SERVICES=(mysql hsm cts-core)
if [ "$WITH_TRADER" -eq 1 ]; then
  SERVICES+=(trader)
fi

echo "[smoke] starting services: ${SERVICES[*]}"
docker compose -f "$COMPOSE_FILE" up -d "${SERVICES[@]}"

echo "[smoke] waiting for cts-core health endpoint"
for i in $(seq 1 30); do
  if curl -fsS "$CTS_HEALTH_URL" >/tmp/cts-core-health.json 2>/dev/null; then
    break
  fi
  sleep 2
done

if ! [ -s /tmp/cts-core-health.json ]; then
  echo "[smoke] ERROR: health endpoint is not reachable: $CTS_HEALTH_URL" >&2
  docker compose -f "$COMPOSE_FILE" ps cts-core || true
  exit 1
fi

echo "[smoke] health endpoint reachable"
if command -v jq >/dev/null 2>&1; then
  STATUS="$(jq -r '.status' /tmp/cts-core-health.json)"
  echo "[smoke] service status: $STATUS"
  jq -r '.components.websocket, .components.scheduler' /tmp/cts-core-health.json >/dev/null || true
else
  echo "[smoke] jq not installed, raw health payload:"
  cat /tmp/cts-core-health.json
fi

echo "[smoke] checking compose state"
docker compose -f "$COMPOSE_FILE" ps cts-core

echo "[smoke] checking ws lifecycle logs (last 5m)"
LOGS="$(docker compose -f "$COMPOSE_FILE" logs --since 5m cts-core 2>/dev/null || true)"
if echo "$LOGS" | grep -Eq "ws_register|ws_heartbeat|ws_disconnect|ws_timeout"; then
  echo "[smoke] ws lifecycle events found"
else
  echo "[smoke] WARNING: ws lifecycle events not found in recent logs"
  echo "[smoke] This is expected if no trader/ws client sent register/heartbeat yet"
fi

echo "[smoke] restart check"
docker compose -f "$COMPOSE_FILE" restart cts-core >/dev/null
sleep 2
curl -fsS "$CTS_HEALTH_URL" >/dev/null

echo "[smoke] shutdown/start check"
docker compose -f "$COMPOSE_FILE" stop cts-core >/dev/null
docker compose -f "$COMPOSE_FILE" start cts-core >/dev/null
sleep 2
curl -fsS "$CTS_HEALTH_URL" >/dev/null

echo "[smoke] PASS: phase2 compose smoke checks completed"
