# Security Guard: TLS & Web Authentication Token Enforcement

This document details the security architecture, cryptographic specifications, runtime token management, and transport encryption mechanisms in `ctop`.

---

## 1. Security Architecture & Core Invariants

The `ctop` security model is governed by two mandatory pillars:

```
+-----------------------------------------------------------------------------------+
|  1. PLAIN HTTP (Without TLS flags)                                                |
|     --> Bound to 127.0.0.1 ONLY (MANDATORY). No external exposure allowed.        |
|                                                                                   |
|  2. TLS ENCRYPTION (Direct flags OR Upstream Reverse Proxy)                       |
|     --> REQUIRED for web-auth-token                                               |
|     --> REQUIRED for remote access                                                |
|                                                                                   |
|  => In short: REMOTE ACCESS REQUIRES BOTH: TLS + WEB-AUTH-TOKEN                   |
+-----------------------------------------------------------------------------------+
```

### Threat Mitigations

| Threat | Vulnerability / Risk | Security Defense in `ctop` |
| :--- | :--- | :--- |
| **Unencrypted Remote Exposure** | Plain HTTP exposed to local networks or public interfaces. | **Mandatory `127.0.0.1` Loopback Binding** for all plain HTTP listeners. |
| **Eavesdropping / Sniffing** | MitM interception of container telemetry and process lists. | **TLS 1.2 / 1.3 Transport Encryption** via `--web-tls-cert/key` or reverse proxy. |
| **Unauthorized Telemetry Access** | Unauthenticated network actors scraping container data. | **Mandatory Auto-Generated 32-Character Bearer Token** (`128-bit entropy`). |
| **Weak Secrets & Credential Leaks** | Weak passwords or secrets leaking into log streams. | **Strict Zero-Log Policy & `~/.config/ctop/token` (`0400`)**. |

---

## 2. Web Authentication Token Enforcement (`--web-auth-token`)

### 2.1. Pure Auto-Generation & Remote Access Model

Authentication in `ctop` operates on a zero-configuration, secure-by-default auto-generation model:

```bash
ctop --web :9090 --web-auth-token
```

- **Per-Start Generation**: Every time `ctop` starts or restarts (under `systemd` or standalone), a **fresh, unique 32-character token is generated** into `~/.config/ctop/token` with **`0400` (owner read-only)** permissions.
- **Automatic Cleanup**: When `ctop` shuts down, the token file is automatically removed from disk.
- **Plain HTTP Listener (`127.0.0.1` ONLY)**:
  - If direct TLS flags are not passed, `ctop` **strictly binds the listener to `127.0.0.1` ONLY** (e.g. `:9090` $\rightarrow$ `127.0.0.1:9090`).
  - Any attempt to bind to external IPs without TLS (e.g. `--web 192.168.1.100:9090`) is rejected at startup.
- **Remote Access Requirements**:
  - Remote access strictly requires **TLS (native or reverse proxy) + web-auth-token**.

---

### 2.2. Cryptographic Specification (`GenerateAuthToken`)

Tokens are generated using Go's cryptographically secure pseudo-random number generator (`crypto/rand`):

```go
// GenerateAuthToken generates a secure 32-character random hexadecimal authentication token (128-bit entropy).
func GenerateAuthToken() (string, error) {
	b := make([]byte, 16) // 16 bytes = 128 bits of CSPRNG entropy
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate random token: %w", err)
	}
	return hex.EncodeToString(b), nil // Exactly 32 hex characters
}
```

- **Entropy**: 128 bits ($2^{128} \approx 3.402 \times 10^{38}$ combinations).
- **Format**: 32 lowercase hexadecimal characters (e.g. `1263171892d7bc4ba3b4e632ffd8d4fb`).

---

### 2.3. Zero-Log Secret Policy & Secure Token File (`~/.config/ctop/token`, `0400`)

> [!IMPORTANT]
> **Strict Zero-Log Policy**: Authentication tokens are **NEVER** written to `log.Infof`, persistent log files, or `journald` streams.

1. **Secure Token File (`~/.config/ctop/token`, `0400`)**:
   - Written exclusively to `~/.config/ctop/token` with `0400` (read-only for owner) permissions.
   - Parent directory `~/.config/ctop` is created with `0700` (owner-accessible only).

2. **Interactive TTY vs Non-Interactive Systemd Execution**:
   - **Interactive Terminal (TTY)**: If `stdout` is connected to a human interactive terminal (`term.IsTerminal`), `ctop` prints the token and file location once for developer convenience.
   - **Non-Interactive (systemd / Daemon / Pipe)**: Standard output is **completely silenced** regarding secret tokens. Authorized clients and monitoring scripts read the token directly from `~/.config/ctop/token`.

```bash
# Example: Client reading token from ~/.config/ctop/token
TOKEN=$(cat ~/.config/ctop/token)
curl -H "Authorization: Bearer $TOKEN" http://localhost:9090/api/v1/metrics
```

---

### 2.4. Systemd Service Integration

The standard `ctop service` definition enables `--web-auth-token`:

```ini
[Unit]
Description=ctop - Container Top & Monitoring Telemetry Daemon
After=network.target docker.service
Requires=docker.service

[Service]
Type=simple
ExecStart=/usr/local/bin/ctop --headless --web :9090 --web-auth-token
Restart=always
RestartSec=5s
LimitNOFILE=65536
Environment=CTOP_DOWNLOAD_DIR=/var/log/ctop

[Install]
WantedBy=multi-user.target
```

---

## 3. Server-Side Security Middleware (`corsMiddleware`)

The authentication middleware in `pkg/web/server.go` intercepts every incoming HTTP/HTTPS request:

```mermaid
flowchart TD
    Req[Incoming HTTP / HTTPS Request] --> AuthCheck{Is Web Auth Enabled?}
    AuthCheck -- No --> Allow[Proceed to Route Handler]
    AuthCheck -- Yes --> ExtractToken[Extract Token from Header or Query Param]
    
    ExtractToken --> CheckBearer{Authorization: Bearer <token>?}
    CheckBearer -- Valid Match --> Allow
    CheckBearer -- No / Mismatch --> CheckQuery{URL ?token= or ?auth=?}
    
    CheckQuery -- Valid Match --> Allow
    CheckQuery -- No / Mismatch --> Reject[Return 401 Unauthorized + WWW-Authenticate Header]
```

### 3.1. Token Extraction Priority

1. **HTTP Authorization Header**:
   ```http
   GET /api/v1/metrics HTTP/1.1
   Host: localhost:9090
   Authorization: Bearer 1263171892d7bc4ba3b4e632ffd8d4fb
   ```
2. **URL Query Parameters (`?token=` or `?auth=`)**:
   Designed for browser `EventSource` / SSE connections that cannot supply custom HTTP headers natively:
   ```javascript
   const evtSource = new EventSource('/api/v1/stream?token=1263171892d7bc4ba3b4e632ffd8d4fb');
   ```

### 3.2. Unauthorized Response Specification

When an unauthenticated or invalidly authenticated request is received:
- **Status Code**: `401 Unauthorized`
- **Response Headers**:
  - `WWW-Authenticate: Bearer realm="ctop"`
  - `Content-Type: application/json; charset=utf-8`
- **Response Body**:
  ```json
  {
    "error": "Unauthorized - invalid or missing authentication token"
  }
  ```

---

## 4. Transport Layer Security (TLS / HTTPS)

To protect tokens and telemetry from in-flight eavesdropping or man-in-the-middle interception, `ctop` supports two standard TLS deployment architectures:

### 4.1. Direct Native TLS Encryption

When binding directly to external networks, provide server certificates via CLI flags:

| CLI Flag | Parameter | Description |
| :--- | :--- | :--- |
| `--web-tls-cert` | `<path>` | Path to server X.509 certificate PEM file (must be absolute path). |
| `--web-tls-key` | `<path>` | Path to server RSA/ECDSA private key PEM file (must be absolute path). |

```bash
ctop --web :9443 \
     --web-tls-cert /etc/ssl/certs/ctop.crt \
     --web-tls-key /etc/ssl/private/ctop.key \
     --web-auth-token
```

### 4.2. Reverse Proxy TLS Termination (NGINX, Traefik, Caddy, Cloud ALBs)

When running behind an ingress controller or edge reverse proxy (e.g. NGINX, Traefik, Envoy, AWS Application Load Balancer), TLS is terminated at the proxy edge while `ctop` listens on a local loopback port or private container network:

```
[ Client (HTTPS) ] ---> [ Reverse Proxy (TLS Termination) ] ---> [ ctop (127.0.0.1:9090) ]
```

```bash
# Example: Local backend service behind TLS reverse proxy with custom subpath prefix
ctop --headless --web 127.0.0.1:9090 --url-prefix /ctop --web-auth-token
```

#### Example NGINX Configuration:
```nginx
location /ctop/ {
    proxy_pass http://127.0.0.1:9090/ctop/;
    proxy_http_version 1.1;
    proxy_set_header Connection '';
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto https;
    
    # Required for SSE stream buffering bypass
    proxy_buffering off;
    proxy_cache off;
    proxy_read_timeout 24h;
}
```

### 4.3. Path Traversal & Absolute Path Security Defense

In accordance with strict security policies:
- All certificate and key paths (`--web-tls-cert`, `--web-tls-key`, `--tls-ca`, `--tls-cert`, `--tls-key`) are sanitized and resolved through `filepath.Abs(filepath.Clean(p))` on startup.
- If relative path traversal sequences (e.g. `../../`) or malformed path injections are detected, they are resolved against canonical filesystem boundaries.
- Container REST routes (e.g. `/api/v1/containers/{id}/top`) reject any ID containing `..`, `/`, or `\` with `400 Bad Request` before accessing engine APIs.

---

## 5. End-to-End Verification Matrix

| Test Suite | Test Function | Verified Security Invariant |
| :--- | :--- | :--- |
| **Unit (`pkg/web`)** | `TestGenerateAuthToken` | Verifies 32-character hexadecimal output and CSPRNG uniqueness. |
| **Unit (`pkg/web`)** | `TestWebServerAuthToken` | Verifies 401 on missing token, 401 on invalid token, 200 on Bearer header, 200 on URL query param. |
| **Unit (`pkg/web`)** | `TestSecureTokenFileOperations` | Verifies file creation at `0400` permissions and cleanup on removal. |
| **Unit (`main`)** | `TestWebBridgeWithOptionsAndAuth` | Verifies `--web-auth-token` creates `~/.config/ctop/token`, serves authenticated clients, and deletes token on shutdown. |
| **Integration (`docker`)** | `TestE2ELiveWebTelemetryServerAndOpenAPISchema` | Boots live HTTPS web server against real Alpine Docker containers, verifies 401 rejection and 200 success over live TLS sockets. |

---

## 6. Quick Reference Cheat Sheet

### Startup Commands

```bash
# 1. Headless Daemon with Auto-Generated Token in ~/.config/ctop/token
ctop --headless --web :9090 --web-auth-token

# 2. Encrypted HTTPS with Auto-Generated Token
ctop --web :9443 \
     --web-tls-cert /etc/ssl/certs/ctop.crt \
     --web-tls-key /etc/ssl/private/ctop.key \
     --web-auth-token
```

### Client Request Examples

```bash
# REST API Request via curl using token file
TOKEN=$(cat ~/.config/ctop/token)
curl -H "Authorization: Bearer $TOKEN" https://localhost:9443/api/v1/containers

# SSE Stream via curl
TOKEN=$(cat ~/.config/ctop/token)
curl -N -H "Authorization: Bearer $TOKEN" https://localhost:9443/api/v1/stream

# SSE Stream via Browser / JavaScript
const token = "..."; // Read from ~/.config/ctop/token
const source = new EventSource(`https://localhost:9443/api/v1/stream?token=${token}`);
source.onmessage = (event) => console.log(JSON.parse(event.data));
```
