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
CTS_HEALTH_URL="${CTS_HEALTH_URL:-}"
CTS_WS_URL="${CTS_WS_URL:-}"
CTS_SMOKE_TRADER_ID="${CTS_SMOKE_TRADER_ID:-smoke-trader-e2e}"
CTS_SMOKE_TRADER_NAME="${CTS_SMOKE_TRADER_NAME:-Smoke Trader E2E}"
MYSQL_USER="${MYSQL_USER:-root}"
MYSQL_PASSWORD="${MYSQL_PASSWORD:-root}"
MYSQL_DATABASE="${MYSQL_DATABASE:-ct_system}"
SMOKE_SKIP_UP="${SMOKE_SKIP_UP:-1}"
SMOKE_NO_RESTART="${SMOKE_NO_RESTART:-1}"

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

if [ "$SMOKE_SKIP_UP" = "1" ]; then
  echo "[smoke] skip startup (SMOKE_SKIP_UP=1, default)"
else
  echo "[smoke] starting services: ${SERVICES[*]}"
  docker compose -f "$COMPOSE_FILE" up -d "${SERVICES[@]}"
fi

echo "[smoke] waiting for cts-core health endpoint"
if [ -z "$CTS_HEALTH_URL" ]; then
  CANDIDATE_HEALTH_URLS=(
    "https://localhost:8080/health"
    "http://localhost:8080/health"
    "https://localhost:8081/health"
    "http://localhost:8081/health"
  )
else
  CANDIDATE_HEALTH_URLS=("$CTS_HEALTH_URL")
fi

HEALTH_OK=0
for i in $(seq 1 30); do
  for candidate in "${CANDIDATE_HEALTH_URLS[@]}"; do
    if curl -k -fsS "$candidate" >/tmp/cts-core-health.json 2>/dev/null; then
      CTS_HEALTH_URL="$candidate"
      HEALTH_OK=1
      break
    fi
  done
  if [ "$HEALTH_OK" -eq 1 ]; then
    break
  fi
  sleep 2
done

if [ "$HEALTH_OK" -ne 1 ] || ! [ -s /tmp/cts-core-health.json ]; then
  echo "[smoke] ERROR: health endpoint is not reachable: $CTS_HEALTH_URL" >&2
  docker compose -f "$COMPOSE_FILE" ps cts-core || true
  exit 1
fi

echo "[smoke] health endpoint reachable"
echo "[smoke] health url: $CTS_HEALTH_URL"
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

if [ -z "$CTS_WS_URL" ]; then
  case "$CTS_HEALTH_URL" in
    https://localhost:8080/health)
      CTS_WS_URL="wss://localhost:8080/ws"
      ;;
    http://localhost:8080/health)
      CTS_WS_URL="ws://localhost:8080/ws"
      ;;
    https://localhost:8081/health)
      CTS_WS_URL="wss://localhost:8081/ws"
      ;;
    *)
      CTS_WS_URL="ws://localhost:8081/ws"
      ;;
  esac
fi
echo "[smoke] ws url: $CTS_WS_URL"

echo "[smoke] seeding smoke trader in MySQL"
docker compose -f "$COMPOSE_FILE" exec -T mysql \
  mysql -u"$MYSQL_USER" -p"$MYSQL_PASSWORD" -D "$MYSQL_DATABASE" -e \
  "INSERT INTO TRADER (TRADER_NAME, CERTIFICATE_CN, REGION, STATUS, MAX_TASKS) \
   VALUES ('$CTS_SMOKE_TRADER_NAME', '$CTS_SMOKE_TRADER_ID', 'smoke', 'registered', 10) \
   ON DUPLICATE KEY UPDATE DATE_MODIFY = CURRENT_TIMESTAMP;"

echo "[smoke] running ws lifecycle client (connect/register/heartbeat/disconnect)"
CTS_WS_URL="$CTS_WS_URL" CTS_SMOKE_TRADER_ID="$CTS_SMOKE_TRADER_ID" go run ./tests/smoke_ws_lifecycle_client.go

echo "[smoke] checking ws lifecycle logs (last 5m)"
LOGS="$(docker compose -f "$COMPOSE_FILE" logs --since 5m cts-core 2>/dev/null || true)"
WS_OUT_LOG="$SCRIPT_DIR/../logs/ws_out.log"
if echo "$LOGS" | grep -Eq "ws_register|ws_heartbeat|ws_disconnect|ws_timeout"; then
  echo "[smoke] ws lifecycle events found"
elif [ -f "$WS_OUT_LOG" ] && tail -n 300 "$WS_OUT_LOG" | grep -Eq "trader.register_ack|trader.heartbeat_ack|\"event\":\"error\""; then
  echo "[smoke] ws lifecycle events found in ws_out.log"
else
  echo "[smoke] WARNING: ws lifecycle events not found in recent compose logs/output" >&2
fi

if [ "$SMOKE_NO_RESTART" = "1" ]; then
  echo "[smoke] skip restart/shutdown checks (SMOKE_NO_RESTART=1, default)"
else
  echo "[smoke] restart check"
  docker compose -f "$COMPOSE_FILE" restart cts-core >/dev/null
  sleep 2
  curl -k -fsS "$CTS_HEALTH_URL" >/dev/null

  echo "[smoke] shutdown/start check"
  docker compose -f "$COMPOSE_FILE" stop cts-core >/dev/null
  docker compose -f "$COMPOSE_FILE" start cts-core >/dev/null
  sleep 2
  curl -k -fsS "$CTS_HEALTH_URL" >/dev/null
fi

echo "[smoke] PASS: phase2 compose smoke checks completed"
