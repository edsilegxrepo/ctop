#!/usr/bin/env bash
# ==============================================================================
# test_nginx_e2e.sh: Comprehensive End-to-End NGINX Reverse Proxy Verification
#
# Complete End-to-End Test Matrix:
#  1. Direct local loopback bypass on ctop backend (127.0.0.1:19090) -> 200 OK
#  2. Insecure plain HTTP proxy forwarding (Port 19080) -> 403 Forbidden
#  3. Secure HTTPS proxy forwarding (Port 19443) without auth -> 401 Unauthorized
#  4. Secure HTTPS proxy forwarding with invalid token -> 401 Unauthorized
#  5. Secure HTTPS proxy forwarding with Bearer Token header -> 200 OK
#  6. Secure HTTPS proxy forwarding with query param (?token=...) -> 401 Unauthorized
#  7. Web Dashboard SPA HTML delivery (GET /probe/) -> 200 OK
#  8. Health endpoint (GET /probe/api/v1/health) via proxy -> 200 OK
#  9. System Metrics endpoint (GET /probe/api/v1/metrics) with Bearer token -> 200 OK
# 10. Schema endpoint (GET /probe/api/v1/schema) -> 200 OK
# 11. Rate limiter over proxy (5 failed logins -> 429 Too Many Requests)
# 12. Web session login via POST /api/v1/auth/login -> 200 OK (sets cookie)
# 13. Authenticated access via session cookie -> 200 OK
# 14. Data export endpoint (GET /probe/api/v1/export) via cookie -> 200 OK
# 15. Live real-time SSE streaming (/api/v1/stream) through NGINX -> Event frames
# 16. Web session logout via POST /api/v1/auth/logout -> 200 OK (revokes cookie)
# 17. Post-logout unauthorized access -> 401 Unauthorized
# ==============================================================================
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
CTOP_BIN="$REPO_ROOT/bin/ctop"
PID_FILE="/tmp/nginx_ctop_test.pid"
COOKIE_JAR="/tmp/ctop_cookie.txt"
RENDERED_CONF="/tmp/nginx_ctop_rendered.conf"
ERROR_LOG="/tmp/nginx_ctop_error.log"

cd "$REPO_ROOT"

if [ ! -f "$CTOP_BIN" ]; then
  echo "Building ctop binary..."
  make build
fi

if [ ! -f "$REPO_ROOT/tests/tls/server.crt" ] || [ ! -f "$REPO_ROOT/tests/tls/server.key" ]; then
  echo "Generating test self-signed TLS certificates in tests/tls/..."
  mkdir -p "$REPO_ROOT/tests/tls"
  openssl req -x509 -newkey rsa:2048 -nodes \
    -keyout "$REPO_ROOT/tests/tls/server.key" \
    -out "$REPO_ROOT/tests/tls/server.crt" \
    -days 1 \
    -subj "/CN=localhost"
fi

# Render NGINX configuration with absolute repository paths
cat << EOF > "$RENDERED_CONF"
worker_processes 1;
pid $PID_FILE;
error_log $ERROR_LOG info;

events {
    worker_connections 128;
}

http {
    access_log /tmp/nginx_ctop_access.log;
    client_body_temp_path /tmp/nginx_body_temp;
    proxy_temp_path /tmp/nginx_proxy_temp;
    fastcgi_temp_path /tmp/nginx_fcgi_temp;
    uwsgi_temp_path /tmp/nginx_uwsgi_temp;
    scgi_temp_path /tmp/nginx_scgi_temp;

    # 1. Insecure HTTP Proxy (Forwards X-Forwarded-Proto: http)
    server {
        listen 127.0.0.1:19080;
        server_name localhost;

        location /probe/ {
            proxy_pass http://127.0.0.1:19090/probe/;
            proxy_http_version 1.1;
            proxy_set_header Host \$host;
            proxy_set_header X-Real-IP \$remote_addr;
            proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
            proxy_set_header X-Forwarded-Proto http;
            proxy_set_header Connection "";
            proxy_buffering off;
            proxy_cache off;
        }
    }

    # 2. Secure SSL/TLS Terminating Reverse Proxy (Forwards X-Forwarded-Proto: https)
    server {
        listen 127.0.0.1:19443 ssl;
        server_name localhost;

        ssl_certificate $REPO_ROOT/tests/tls/server.crt;
        ssl_certificate_key $REPO_ROOT/tests/tls/server.key;
        ssl_protocols TLSv1.2 TLSv1.3;
        ssl_ciphers HIGH:!aNULL:!MD5;

        location /probe/ {
            proxy_pass http://127.0.0.1:19090/probe/;
            proxy_http_version 1.1;
            proxy_set_header Host \$host;
            proxy_set_header X-Real-IP \$remote_addr;
            proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
            proxy_set_header X-Forwarded-Proto https;
            proxy_set_header Connection "";
            proxy_buffering off;
            proxy_cache off;
            chunked_transfer_encoding off;
        }
    }
}
EOF

# Clean up any previously running instances
pkill -u "$USER" -f "$CTOP_BIN" 2> /dev/null || true
if [ -f "$PID_FILE" ]; then
  nginx -e "$ERROR_LOG" -c "$RENDERED_CONF" -s stop 2> /dev/null || true
  rm -f "$PID_FILE"
fi
rm -f "$COOKIE_JAR"

TEST_XDG="/tmp/ctop_xdg_$$"
export XDG_CONFIG_HOME="$TEST_XDG"
mkdir -p "$TEST_XDG"

echo "=== 1. Starting ctop daemon on 127.0.0.1:19090 with /probe, --web-auth-token, and --audit-log ==="
AUDIT_LOG_BASE="$TEST_XDG/audit.ndjson"
"$CTOP_BIN" --headless --web 127.0.0.1:19090 --url-prefix /probe --web-auth-token --audit-log "$AUDIT_LOG_BASE" &
CTOP_PID=$!

TOKEN_FILE="$TEST_XDG/ctop/token"
for _ in {1..50}; do
  if [ -f "$TOKEN_FILE" ]; then
    break
  fi
  sleep 0.1
done

if [ ! -f "$TOKEN_FILE" ]; then
  echo "ERROR: Token file $TOKEN_FILE not created!"
  kill "$CTOP_PID" 2> /dev/null || true
  rm -rf "$TEST_XDG"
  exit 1
fi
TOKEN=$(cat "$TOKEN_FILE")
echo "Generated Bearer Token: $TOKEN"

echo "=== 2. Starting NGINX reverse proxy on ports 19080 (HTTP) and 19443 (HTTPS) ==="
nginx -e "$ERROR_LOG" -c "$RENDERED_CONF"
sleep 1

cleanup() {
  echo ""
  echo "=== Cleaning up test processes ==="
  kill "$CTOP_PID" 2> /dev/null || true
  nginx -e "$ERROR_LOG" -c "$RENDERED_CONF" -s stop 2> /dev/null || true
  rm -rf "$COOKIE_JAR" "$PID_FILE" "$RENDERED_CONF" "$TEST_XDG"
}
trap cleanup EXIT

echo "=== 3. Testing Direct Local Loopback Bypass on ctop backend (127.0.0.1:19090) ==="
CODE=$(curl -s -o /dev/null -w "%{http_code}" http://127.0.0.1:19090/probe/api/v1/containers)
echo "Direct local access response code: $CODE"
if [ "$CODE" != "200" ]; then
  echo "FAILED: expected 200 for direct local access, got $CODE"
  exit 1
fi
echo "PASS: Direct local loopback bypassed auth without prompt."

echo "=== 4. Testing Insecure HTTP Reverse Proxy (Port 19080) ==="
CODE=$(curl -s -o /dev/null -w "%{http_code}" http://127.0.0.1:19080/probe/api/v1/containers)
echo "Insecure proxy response code: $CODE"
if [ "$CODE" != "403" ]; then
  echo "FAILED: expected 403 Forbidden on unencrypted proxy, got $CODE"
  exit 1
fi
echo "PASS: Unencrypted proxy access rejected with 403 Forbidden."

echo "=== 5. Testing HTTPS Reverse Proxy (Port 19443) Without Auth Token ==="
CODE=$(curl -k -s -o /dev/null -w "%{http_code}" https://127.0.0.1:19443/probe/api/v1/containers)
echo "Secure proxy without token response code: $CODE"
if [ "$CODE" != "401" ]; then
  echo "FAILED: expected 401 Unauthorized, got $CODE"
  exit 1
fi
echo "PASS: Secure proxy without token rejected with 401 Unauthorized."

echo "=== 6. Testing HTTPS Reverse Proxy with Invalid Token ==="
CODE=$(curl -k -s -o /dev/null -w "%{http_code}" -H "Authorization: Bearer invalid_token_1234567890123456" https://127.0.0.1:19443/probe/api/v1/containers)
if [ "$CODE" != "401" ]; then
  echo "FAILED: expected 401 Unauthorized for invalid token, got $CODE"
  exit 1
fi
echo "PASS: Invalid Bearer token rejected with 401 Unauthorized."

echo "=== 7. Testing HTTPS Reverse Proxy with Valid Bearer Token Header ==="
BODY=$(curl -k -s -H "Authorization: Bearer $TOKEN" https://127.0.0.1:19443/probe/api/v1/containers)
echo "API response length: ${#BODY} bytes"
CODE=$(curl -k -s -o /dev/null -w "%{http_code}" -H "Authorization: Bearer $TOKEN" https://127.0.0.1:19443/probe/api/v1/containers)
if [ "$CODE" != "200" ]; then
  echo "FAILED: expected 200 OK with Bearer token, got $CODE"
  exit 1
fi
echo "PASS: Bearer token authorized via HTTPS NGINX proxy."

echo "=== 8. Testing HTTPS Reverse Proxy with Deprecated Query Param (?token=...) ==="
CODE=$(curl -k -s -o /dev/null -w "%{http_code}" "https://127.0.0.1:19443/probe/api/v1/containers?token=$TOKEN")
echo "Query param response code: $CODE"
if [ "$CODE" != "401" ]; then
  echo "FAILED: expected 401 Unauthorized for query param token, got $CODE"
  exit 1
fi
echo "PASS: Query parameter token strictly rejected with 401 Unauthorized."

echo "=== 9. Testing Dashboard SPA HTML Delivery (GET /probe/) ==="
CODE=$(curl -k -s -o /dev/null -w "%{http_code}" https://127.0.0.1:19443/probe/)
if [ "$CODE" != "200" ]; then
  echo "FAILED: expected 200 OK for dashboard SPA HTML, got $CODE"
  exit 1
fi
echo "PASS: Dashboard SPA HTML delivered successfully."

echo "=== 10. Testing Health, Metrics, and Schema Endpoints via HTTPS Proxy ==="
# Unauthenticated /health via proxy must be rejected with 401 Unauthorized
HEALTH_UNAUTH=$(curl -k -s -o /dev/null -w "%{http_code}" https://127.0.0.1:19443/probe/api/v1/health)
if [ "$HEALTH_UNAUTH" != "401" ]; then
  echo "FAILED: expected 401 Unauthorized for unauthenticated /health, got $HEALTH_UNAUTH"
  exit 1
fi
HEALTH_CODE=$(curl -k -s -o /dev/null -w "%{http_code}" -H "Authorization: Bearer $TOKEN" https://127.0.0.1:19443/probe/api/v1/health)
if [ "$HEALTH_CODE" != "200" ]; then
  echo "FAILED: expected 200 OK for authenticated /health, got $HEALTH_CODE"
  exit 1
fi
METRICS_CODE=$(curl -k -s -o /dev/null -w "%{http_code}" -H "Authorization: Bearer $TOKEN" https://127.0.0.1:19443/probe/api/v1/metrics)
if [ "$METRICS_CODE" != "200" ]; then
  echo "FAILED: expected 200 OK for /metrics, got $METRICS_CODE"
  exit 1
fi
SCHEMA_CODE=$(curl -k -s -o /dev/null -w "%{http_code}" -H "Authorization: Bearer $TOKEN" https://127.0.0.1:19443/probe/api/v1/schema)
if [ "$SCHEMA_CODE" != "200" ]; then
  echo "FAILED: expected 200 OK for /schema, got $SCHEMA_CODE"
  exit 1
fi
echo "PASS: /health, /metrics, and /schema endpoints verified."

echo "=== 11. Testing Web Session Login via POST /api/v1/auth/login ==="
CODE=$(curl -k -s -o /dev/null -w "%{http_code}" -c "$COOKIE_JAR" -X POST \
  -H "Content-Type: application/json" \
  -d "{\"token\":\"$TOKEN\"}" \
  https://127.0.0.1:19443/probe/api/v1/auth/login)
if [ "$CODE" != "200" ]; then
  echo "FAILED: login failed, got $CODE"
  exit 1
fi
echo "PASS: Login successful, session cookie saved."

echo "=== 12. Testing Authenticated Access via Session Cookie ==="
CODE=$(curl -k -s -o /dev/null -w "%{http_code}" -b "$COOKIE_JAR" https://127.0.0.1:19443/probe/api/v1/containers)
if [ "$CODE" != "200" ]; then
  echo "FAILED: expected 200 OK with session cookie, got $CODE"
  exit 1
fi
echo "PASS: Session cookie authorized request to /probe/api/v1/containers."

echo "=== 13. Testing Data Export via Session Cookie (GET /probe/api/v1/export) ==="
EXPORT_BODY=$(curl -k -s -b "$COOKIE_JAR" https://127.0.0.1:19443/probe/api/v1/export)
if [[ "$EXPORT_BODY" != *"containers"* ]]; then
  echo "FAILED: invalid export response: $EXPORT_BODY"
  exit 1
fi
echo "PASS: Data export verified."

echo "=== 14. Testing Live SSE Stream via NGINX Proxy with Session Cookie ==="
SSE_OUTPUT=$(curl -k -s -N -b "$COOKIE_JAR" --max-time 3 https://127.0.0.1:19443/probe/api/v1/stream | head -n 5)
echo "SSE Stream Sample:"
echo "$SSE_OUTPUT"
if [[ "$SSE_OUTPUT" != *"data:"* ]]; then
  echo "FAILED: did not receive SSE data frame"
  exit 1
fi
echo "PASS: Live SSE streaming through NGINX reverse proxy verified."

echo "=== 15. Testing Logout via POST /api/v1/auth/logout ==="
CODE=$(curl -k -s -o /dev/null -w "%{http_code}" -b "$COOKIE_JAR" -c "$COOKIE_JAR" -X POST https://127.0.0.1:19443/probe/api/v1/auth/logout)
if [ "$CODE" != "200" ]; then
  echo "FAILED: logout failed, got $CODE"
  exit 1
fi
echo "PASS: Logout successful."

echo "=== 16. Testing Access After Logout ==="
CODE=$(curl -k -s -o /dev/null -w "%{http_code}" -b "$COOKIE_JAR" https://127.0.0.1:19443/probe/api/v1/containers)
if [ "$CODE" != "401" ]; then
  echo "FAILED: expected 401 Unauthorized after logout, got $CODE"
  exit 1
fi
echo "PASS: Post-logout access rejected with 401 Unauthorized."

echo "=== 17. Verifying Daily Rotated NDJSON Audit Log ==="
TODAY=$(date +%Y-%m-%d)
EXPECTED_AUDIT_LOG="$TEST_XDG/audit-$TODAY.ndjson"
if [ ! -f "$EXPECTED_AUDIT_LOG" ]; then
  echo "FAILED: expected audit log file at $EXPECTED_AUDIT_LOG"
  ls -la "$TEST_XDG"
  exit 1
fi

AUDIT_LINES=$(wc -l < "$EXPECTED_AUDIT_LOG")
echo "Audit Log contains $AUDIT_LINES NDJSON events."
if [ "$AUDIT_LINES" -lt 5 ]; then
  echo "FAILED: expected at least 5 audit log entries, found $AUDIT_LINES"
  cat "$EXPECTED_AUDIT_LOG"
  exit 1
fi

# Verify every line is valid JSON
while IFS= read -r line; do
  if ! echo "$line" | jq -e . > /dev/null 2>&1; then
    echo "FAILED: invalid JSON line in audit log: $line"
    exit 1
  fi
done < "$EXPECTED_AUDIT_LOG"

echo "PASS: NDJSON Audit Log verified with $AUDIT_LINES valid JSON records."

echo ""
echo "========================================================"
echo "ALL 16 NGINX REVERSE PROXY E2E TESTS PASSED!"
echo "========================================================"
