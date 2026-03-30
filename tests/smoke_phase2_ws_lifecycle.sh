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
CTS_METRICS_URL="${CTS_METRICS_URL:-}"
CTS_WS_URL="${CTS_WS_URL:-}"
CTS_WS_CLIENT_CA_PATH="${CTS_WS_CLIENT_CA_PATH:-$COMPOSE_DIR/volumes/pki/ca/ca.crt}"
CTS_WS_CLIENT_CERT_PATH="${CTS_WS_CLIENT_CERT_PATH:-$COMPOSE_DIR/volumes/pki/cts-core/clients/trader-1/trader-1-cts.crt}"
CTS_WS_CLIENT_KEY_PATH="${CTS_WS_CLIENT_KEY_PATH:-$COMPOSE_DIR/volumes/pki/cts-core/clients/trader-1/trader-1-cts.key}"
CTS_WS_SERVER_NAME="${CTS_WS_SERVER_NAME:-localhost}"
CTS_SMOKE_CERTIFICATE_CN="${CTS_SMOKE_CERTIFICATE_CN:-}"
CTS_SMOKE_RESET_TRADER="${CTS_SMOKE_RESET_TRADER:-1}"
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

for f in "$CTS_WS_CLIENT_CA_PATH" "$CTS_WS_CLIENT_CERT_PATH" "$CTS_WS_CLIENT_KEY_PATH"; do
  if [ ! -f "$f" ]; then
    echo "required file not found: $f" >&2
    exit 1
  fi
done

if [ -z "$CTS_SMOKE_CERTIFICATE_CN" ]; then
  CTS_SMOKE_CERTIFICATE_CN="$(openssl x509 -in "$CTS_WS_CLIENT_CERT_PATH" -noout -subject | sed -n 's/.*CN *= *\([^,]*\).*/\1/p')"
fi
if [ -z "$CTS_SMOKE_CERTIFICATE_CN" ]; then
  echo "failed to resolve trader CN from certificate: $CTS_WS_CLIENT_CERT_PATH" >&2
  exit 1
fi

echo "[smoke] compose file: $COMPOSE_FILE"
echo "[smoke] trader identity CN: $CTS_SMOKE_CERTIFICATE_CN"

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
    "https://localhost:8081/health"
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
    https://localhost:8081/health)
      CTS_WS_URL="wss://localhost:8081/ws"
      ;;
    *)
      CTS_WS_URL="wss://localhost:8080/ws"
      ;;
  esac
fi
echo "[smoke] ws url: $CTS_WS_URL"
if [[ "$CTS_WS_URL" != wss://* ]]; then
  echo "[smoke] ERROR: CTS_WS_URL must use wss:// in hard-cutover mode" >&2
  exit 1
fi

if [ -z "$CTS_METRICS_URL" ]; then
  CTS_METRICS_URL="${CTS_HEALTH_URL%/health}/metrics"
fi
echo "[smoke] metrics url: $CTS_METRICS_URL"

echo "[smoke] checking metrics endpoint"
METRICS_PAYLOAD="$(curl -k -fsS "$CTS_METRICS_URL" || true)"
if ! echo "$METRICS_PAYLOAD" | grep -Eq "go_goroutines|cts_core_ws_active_connections"; then
  echo "[smoke] ERROR: metrics endpoint missing expected prometheus series: $CTS_METRICS_URL" >&2
  exit 1
fi

if [ "$CTS_SMOKE_RESET_TRADER" = "1" ]; then
  echo "[smoke] resetting trader row for deterministic auto-create"
  TRADER_ID_TO_RESET="$(docker compose -f "$COMPOSE_FILE" exec -T mysql \
    mysql -N -s -u"$MYSQL_USER" -p"$MYSQL_PASSWORD" -D "$MYSQL_DATABASE" -e \
    "SELECT ID FROM TRADER WHERE CERTIFICATE_CN = '$CTS_SMOKE_CERTIFICATE_CN' LIMIT 1;" 2>/dev/null || true)"

  if [ -n "$TRADER_ID_TO_RESET" ]; then
    TRADER_RES_TABLE_EXISTS="$(docker compose -f "$COMPOSE_FILE" exec -T mysql \
      mysql -N -s -u"$MYSQL_USER" -p"$MYSQL_PASSWORD" -D "$MYSQL_DATABASE" -e \
      "SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'TRADER_EXCHANGE_RESOURCE';" 2>/dev/null || echo "0")"
    if [ "$TRADER_RES_TABLE_EXISTS" = "1" ]; then
      docker compose -f "$COMPOSE_FILE" exec -T mysql \
        mysql -u"$MYSQL_USER" -p"$MYSQL_PASSWORD" -D "$MYSQL_DATABASE" -e \
        "DELETE FROM TRADER_EXCHANGE_RESOURCE WHERE TRADER_ID = $TRADER_ID_TO_RESET;"
    fi

    TRADER_SESSION_TABLE_EXISTS="$(docker compose -f "$COMPOSE_FILE" exec -T mysql \
      mysql -N -s -u"$MYSQL_USER" -p"$MYSQL_PASSWORD" -D "$MYSQL_DATABASE" -e \
      "SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'TRADER_SESSION';" 2>/dev/null || echo "0")"
    if [ "$TRADER_SESSION_TABLE_EXISTS" = "1" ]; then
      docker compose -f "$COMPOSE_FILE" exec -T mysql \
        mysql -u"$MYSQL_USER" -p"$MYSQL_PASSWORD" -D "$MYSQL_DATABASE" -e \
        "DELETE FROM TRADER_SESSION WHERE TRADER_ID = $TRADER_ID_TO_RESET;"
    fi

    MONITORING_TABLE_EXISTS="$(docker compose -f "$COMPOSE_FILE" exec -T mysql \
      mysql -N -s -u"$MYSQL_USER" -p"$MYSQL_PASSWORD" -D "$MYSQL_DATABASE" -e \
      "SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'MONITORING';" 2>/dev/null || echo "0")"
    if [ "$MONITORING_TABLE_EXISTS" = "1" ]; then
      docker compose -f "$COMPOSE_FILE" exec -T mysql \
        mysql -u"$MYSQL_USER" -p"$MYSQL_PASSWORD" -D "$MYSQL_DATABASE" -e \
        "UPDATE MONITORING SET ASSIGNED_TRADER_ID = NULL WHERE ASSIGNED_TRADER_ID = $TRADER_ID_TO_RESET;
         UPDATE MONITORING SET BACKUP_TRADER_ID = NULL WHERE BACKUP_TRADER_ID = $TRADER_ID_TO_RESET;"
    fi

    TRADE_TABLE_EXISTS="$(docker compose -f "$COMPOSE_FILE" exec -T mysql \
      mysql -N -s -u"$MYSQL_USER" -p"$MYSQL_PASSWORD" -D "$MYSQL_DATABASE" -e \
      "SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'TRADE';" 2>/dev/null || echo "0")"
    if [ "$TRADE_TABLE_EXISTS" = "1" ]; then
      docker compose -f "$COMPOSE_FILE" exec -T mysql \
        mysql -u"$MYSQL_USER" -p"$MYSQL_PASSWORD" -D "$MYSQL_DATABASE" -e \
        "UPDATE TRADE SET TRADER_ID = NULL WHERE TRADER_ID = $TRADER_ID_TO_RESET;"
    fi

    docker compose -f "$COMPOSE_FILE" exec -T mysql \
      mysql -u"$MYSQL_USER" -p"$MYSQL_PASSWORD" -D "$MYSQL_DATABASE" -e \
      "DELETE FROM TRADER WHERE ID = $TRADER_ID_TO_RESET;"
  fi
fi

echo "[smoke] running ws lifecycle client (connect/register/heartbeat/disconnect)"
CTS_WS_URL="$CTS_WS_URL" \
CTS_WS_CLIENT_CA_PATH="$CTS_WS_CLIENT_CA_PATH" \
CTS_WS_CLIENT_CERT_PATH="$CTS_WS_CLIENT_CERT_PATH" \
CTS_WS_CLIENT_KEY_PATH="$CTS_WS_CLIENT_KEY_PATH" \
CTS_WS_SERVER_NAME="$CTS_WS_SERVER_NAME" \
CTS_SMOKE_CERTIFICATE_CN="$CTS_SMOKE_CERTIFICATE_CN" \
go run ./tests/smoke_ws_lifecycle_client.go

echo "[smoke] running duplicate CN conflict check"
DUP_PRIMARY_LOG="$(mktemp)"
DUP_SECONDARY_LOG="$(mktemp)"

cleanup_dup_logs() {
  rm -f "$DUP_PRIMARY_LOG" "$DUP_SECONDARY_LOG"
}
trap cleanup_dup_logs EXIT

CTS_WS_URL="$CTS_WS_URL" \
CTS_WS_CLIENT_CA_PATH="$CTS_WS_CLIENT_CA_PATH" \
CTS_WS_CLIENT_CERT_PATH="$CTS_WS_CLIENT_CERT_PATH" \
CTS_WS_CLIENT_KEY_PATH="$CTS_WS_CLIENT_KEY_PATH" \
CTS_WS_SERVER_NAME="$CTS_WS_SERVER_NAME" \
CTS_SMOKE_CERTIFICATE_CN="$CTS_SMOKE_CERTIFICATE_CN" \
CTS_SMOKE_HOLD_SEC="8" \
go run ./tests/smoke_ws_lifecycle_client.go >"$DUP_PRIMARY_LOG" 2>&1 &
DUP_PRIMARY_PID=$!

PRIMARY_READY=0
for _ in $(seq 1 60); do
  PRIMARY_ACTIVE_COUNT="$(docker compose -f "$COMPOSE_FILE" exec -T mysql \
    mysql -N -s -u"$MYSQL_USER" -p"$MYSQL_PASSWORD" -D "$MYSQL_DATABASE" -e \
    "SELECT COUNT(*) FROM TRADER_SESSION s JOIN TRADER t ON t.ID = s.TRADER_ID WHERE t.CERTIFICATE_CN = '$CTS_SMOKE_CERTIFICATE_CN' AND s.ENDED_AT IS NULL;" 2>/dev/null || echo "0")"
  if [ "$PRIMARY_ACTIVE_COUNT" -ge 1 ] 2>/dev/null; then
    PRIMARY_READY=1
    break
  fi
  if ! kill -0 "$DUP_PRIMARY_PID" 2>/dev/null; then
    break
  fi
  sleep 0.5
done

if [ "$PRIMARY_READY" != "1" ]; then
  echo "[smoke] ERROR: primary ws client did not establish active session" >&2
  if [ -f "$DUP_PRIMARY_LOG" ]; then
    cat "$DUP_PRIMARY_LOG" >&2
  fi
  wait "$DUP_PRIMARY_PID" 2>/dev/null || true
  exit 1
fi

if ! CTS_WS_URL="$CTS_WS_URL" \
  CTS_WS_CLIENT_CA_PATH="$CTS_WS_CLIENT_CA_PATH" \
  CTS_WS_CLIENT_CERT_PATH="$CTS_WS_CLIENT_CERT_PATH" \
  CTS_WS_CLIENT_KEY_PATH="$CTS_WS_CLIENT_KEY_PATH" \
  CTS_WS_SERVER_NAME="$CTS_WS_SERVER_NAME" \
  CTS_SMOKE_CERTIFICATE_CN="$CTS_SMOKE_CERTIFICATE_CN" \
  CTS_SMOKE_EXPECT_REGISTER_ERROR_CODE="DUPLICATE_CONNECTION" \
  go run ./tests/smoke_ws_lifecycle_client.go >"$DUP_SECONDARY_LOG" 2>&1; then
  echo "[smoke] ERROR: duplicate CN secondary client did not receive expected DUPLICATE_CONNECTION" >&2
  cat "$DUP_SECONDARY_LOG" >&2
  wait "$DUP_PRIMARY_PID" 2>/dev/null || true
  exit 1
fi

if ! wait "$DUP_PRIMARY_PID"; then
  echo "[smoke] ERROR: primary ws client failed during duplicate CN scenario" >&2
  cat "$DUP_PRIMARY_LOG" >&2
  exit 1
fi

DUP_VIOLATIONS="$(docker compose -f "$COMPOSE_FILE" exec -T mysql \
  mysql -N -s -u"$MYSQL_USER" -p"$MYSQL_PASSWORD" -D "$MYSQL_DATABASE" -e \
  "SELECT COUNT(*) FROM (SELECT TRADER_ID FROM TRADER_SESSION WHERE ENDED_AT IS NULL GROUP BY TRADER_ID HAVING COUNT(*) > 1) x;" 2>/dev/null || echo "-1")"

if [ "$DUP_VIOLATIONS" != "0" ]; then
  echo "[smoke] ERROR: duplicate CN invariant violated, traders with >1 active session: $DUP_VIOLATIONS" >&2
  exit 1
fi

echo "[smoke] duplicate CN conflict check passed"

cleanup_dup_logs
trap - EXIT

echo "[smoke] validating trader auto-create status"
TRADER_ROW="$(docker compose -f "$COMPOSE_FILE" exec -T mysql \
  mysql -N -s -u"$MYSQL_USER" -p"$MYSQL_PASSWORD" -D "$MYSQL_DATABASE" -e \
  "SELECT ID, STATUS FROM TRADER WHERE CERTIFICATE_CN = '$CTS_SMOKE_CERTIFICATE_CN' LIMIT 1;" 2>/dev/null || true)"

if [ -z "$TRADER_ROW" ]; then
  echo "[smoke] ERROR: trader row was not created for CN '$CTS_SMOKE_CERTIFICATE_CN'" >&2
  exit 1
fi

TRADER_DB_ID="$(echo "$TRADER_ROW" | awk '{print $1}')"
TRADER_STATUS="$(echo "$TRADER_ROW" | awk '{print $2}')"
if [ "$TRADER_STATUS" != "pending" ]; then
  echo "[smoke] ERROR: expected trader status 'pending', got '$TRADER_STATUS' (TRADER.ID=$TRADER_DB_ID)" >&2
  exit 1
fi
echo "[smoke] trader is pending as expected (id=$TRADER_DB_ID)"

echo "[smoke] validating scheduler gate (pending trader must not receive assignments)"
MONITORING_TABLE_EXISTS="$(docker compose -f "$COMPOSE_FILE" exec -T mysql \
  mysql -N -s -u"$MYSQL_USER" -p"$MYSQL_PASSWORD" -D "$MYSQL_DATABASE" -e \
  "SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'MONITORING';" 2>/dev/null || echo "0")"
if [ "$MONITORING_TABLE_EXISTS" = "1" ]; then
  MONITORING_COUNT="$(docker compose -f "$COMPOSE_FILE" exec -T mysql \
    mysql -N -s -u"$MYSQL_USER" -p"$MYSQL_PASSWORD" -D "$MYSQL_DATABASE" -e \
    "SELECT COUNT(*) FROM MONITORING WHERE ASSIGNED_TRADER_ID = $TRADER_DB_ID OR BACKUP_TRADER_ID = $TRADER_DB_ID;" 2>/dev/null || echo "0")"
  if [ "$MONITORING_COUNT" != "0" ]; then
    echo "[smoke] ERROR: pending trader has MONITORING assignment rows: $MONITORING_COUNT" >&2
    exit 1
  fi
fi

TRADE_TABLE_EXISTS="$(docker compose -f "$COMPOSE_FILE" exec -T mysql \
  mysql -N -s -u"$MYSQL_USER" -p"$MYSQL_PASSWORD" -D "$MYSQL_DATABASE" -e \
  "SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'TRADE';" 2>/dev/null || echo "0")"
if [ "$TRADE_TABLE_EXISTS" = "1" ]; then
  TRADE_COUNT="$(docker compose -f "$COMPOSE_FILE" exec -T mysql \
    mysql -N -s -u"$MYSQL_USER" -p"$MYSQL_PASSWORD" -D "$MYSQL_DATABASE" -e \
    "SELECT COUNT(*) FROM TRADE WHERE TRADER_ID = $TRADER_DB_ID;" 2>/dev/null || echo "0")"
  if [ "$TRADE_COUNT" != "0" ]; then
    echo "[smoke] ERROR: pending trader has TRADE assignments: $TRADE_COUNT" >&2
    exit 1
  fi
fi

echo "[smoke] scheduler gate check passed"

AUDIT_LOG_PATH="${CTS_AUDIT_LOG_PATH:-$COMPOSE_DIR/services/cts-core/logs/audit.log}"
if [ -f "$AUDIT_LOG_PATH" ]; then
  AUDIT_CREATE_COUNT="$(tail -n 500 "$AUDIT_LOG_PATH" | awk -v cn="$CTS_SMOKE_CERTIFICATE_CN" 'index($0,"\"action\":\"TRADER_CREATE\"") && index($0,"\"certificate_cn\":\"" cn "\"") {c++} END {print c+0}')"
  if [ "$AUDIT_CREATE_COUNT" = "0" ]; then
    echo "[smoke] WARNING: TRADER_CREATE audit event not found in $AUDIT_LOG_PATH for CN '$CTS_SMOKE_CERTIFICATE_CN'" >&2
  else
    echo "[smoke] TRADER_CREATE audit events found in file: $AUDIT_CREATE_COUNT"
  fi
else
  echo "[smoke] WARNING: audit log file not found: $AUDIT_LOG_PATH" >&2
fi

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

echo "[smoke] PASS: hard-cutover ws lifecycle smoke checks completed"
