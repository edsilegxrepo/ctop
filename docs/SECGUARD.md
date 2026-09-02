# Security Guard: TLS & Web Authentication Token Enforcement

This document details the security architecture, cryptographic specifications, runtime token management, and transport encryption mechanisms in `ctop`.

---

## 1. Security Architecture & Core Invariants

The `ctop` security model is governed by two mandatory pillars:

```
+-----------------------------------------------------------------------------------+
|  1. LOCAL LOOPBACK ACCESS (Direct 127.0.0.1 / localhost ONLY)                     |
|     --> Plain HTTP bound to 127.0.0.1 ONLY (MANDATORY).                           |
|     --> Unlocked direct access: No password/token prompt in browser for local UI. |
|     --> MUST NOT contain reverse proxy forwarding headers (X-Forwarded-*).        |
|                                                                                   |
|  2. REMOTE ACCESS (Direct TLS flags OR Upstream Reverse Proxy)                    |
|     --> REQUIRED: TLS Transport Encryption (HTTPS).                               |
|     --> REQUIRED: Web Authentication Token (--web-auth-token).                    |
|     --> Browser Web Dashboard: Conditional Password Input / Session Cookie.       |
|     --> REST / SSE APIs: Strict 'Authorization: Bearer <token>' Header.           |
|                                                                                   |
|  => In short: REMOTE ACCESS REQUIRES BOTH: TLS + WEB-AUTH-TOKEN                   |
+-----------------------------------------------------------------------------------+
```

### Threat Mitigations

| Threat | Vulnerability / Risk | Security Defense in `ctop` |
| :--- | :--- | :--- |
| **Unencrypted Remote Exposure** | Plain HTTP exposed to local networks or public interfaces. | **Mandatory `127.0.0.1` Loopback Binding** for all plain HTTP listeners. |
| **Eavesdropping / Sniffing** | MitM interception of container telemetry and process lists. | **TLS 1.2 / 1.3 Transport Encryption** via `--web-tls-cert/key` or reverse proxy. |
| **Unauthorized Telemetry Access** | Unauthenticated network actors scraping container data. | **Mandatory Auto-Generated 64-Character Bearer Token** (`~381-bit entropy`). |
| **Weak Secrets & Credential Leaks** | Weak passwords or secrets leaking into log streams. | **Strict Zero-Log Policy & `~/.config/ctop/token` (`0400`)**. |
| **URL Query Parameter Leaks** | Tokens leaking into proxy access logs, browser history, or referrers. | **Strict Zero-Leak Model**: Complete rejection of `?token=` / `?auth=`. Dedicated `Authorization: Bearer` and `HttpOnly; SameSite=Strict; Secure` session cookies. |
| **Reverse Proxy Loopback Spoofing** | Remote requests proxied via local NGINX (`127.0.0.1`) bypassing auth. | **Strict `isDirectLocalAccess` Guard**: Remote classification if any `X-Forwarded-*` / `X-Real-IP` headers are present, even if `RemoteAddr` is `127.0.0.1`. |
| **Timing Side-Channel Attacks** | Byte-by-byte secret character matching via execution time variances. | **Constant-Time Verification**: Enforce `crypto/subtle.ConstantTimeCompare` across all secret validations. |
| **Session Memory Exhaustion (DoS)** | Unbounded active session growth consuming daemon RAM. | **Bounded In-Memory Session Store**: Strict capacity limit (max 100 concurrent sessions), LRU eviction, and 24h absolute / 2h idle TTL. |
| **Login Brute-Force Flooding** | Attacker flooding `/api/v1/auth/login` to exhaust CPU/socket buffers. | **In-Memory Rate Limiting**: Sliding-window rate limiter enforcing max 5 failed login attempts per IP per minute (`429 Too Many Requests`). |

---

## 2. Web Authentication Token Enforcement (`--web-auth-token`)

### 2.1. Pure Auto-Generation & Local vs. Remote Access Model

Authentication in `ctop` operates on a zero-configuration, secure-by-default auto-generation model:

```bash
# Ephemeral token (default: generated fresh every run, deleted on exit)
ctop --web :9090 --web-auth-token

# Persistent token (generated once in ~/.config/ctop/token and reused across restarts)
ctop --web :9090 --web-auth-token --persistent-token
```

- **Per-Start Generation vs Persistence**:
  - By default (`--web-auth-token`), every time `ctop` starts, a **fresh, unique 64-character alphanumeric token is generated** into `~/.config/ctop/token` with **`0400` (owner read-only)** permissions, and deleted upon shutdown.
  - When `--persistent-token` is passed, `ctop` generates the token **once only** on initial run, reuses the existing token across restarts, and retains the token file on shutdown.
- **Local Loopback Access (`isDirectLocalAccess`)**:
  - If direct TLS flags are not passed, `ctop` **strictly binds the listener to `127.0.0.1` ONLY** (e.g. `:9090` $\rightarrow$ `127.0.0.1:9090`).
  - **Local UI Auto-Unlock Invariant**: Accessing the web dashboard from `127.0.0.1` / `localhost` bypasses authentication prompts **ONLY** when no proxy forwarding headers are present:
    ```go
    func isDirectLocalAccess(r *http.Request) bool {
        isLoopback := isLoopbackAddr(r.RemoteAddr)
        hasProxyHeaders := r.Header.Get("X-Forwarded-For") != "" ||
                           r.Header.Get("X-Real-IP") != "" ||
                           r.Header.Get("X-Forwarded-Proto") != "" ||
                           r.Header.Get("X-Forwarded-Host") != "" ||
                           r.Header.Get("X-Forwarded-Scheme") != "" ||
                           r.Header.Get("Front-End-Https") != ""
        return isLoopback && !hasProxyHeaders
    }
    ```
- **Remote Access Requirements**:
  - Remote access strictly requires **TLS (native or reverse proxy) + web-auth-token**.
  - For remote browser users, the dashboard displays an in-browser password unlock dialog and authenticates via an `HttpOnly` session cookie (`POST /api/v1/auth/login`).
  - For programmatic clients, requests must supply the standard `Authorization: Bearer <token>` header.

---

### 2.2. Cryptographic Specifications

#### 1. Bearer Authentication Token (`GenerateAuthToken`)
Tokens are generated using Go's cryptographically secure pseudo-random number generator (`crypto/rand`) mapped across a 62-character alphanumeric keyspace:

```go
const base62Chars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// GenerateAuthToken generates a secure 64-character random alphanumeric authentication token (~381-bit entropy).
func GenerateAuthToken() (string, error) {
	bytes := make([]byte, 64)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random token: %w", err)
	}
	result := make([]byte, 64)
	for i, b := range bytes {
		result[i] = base62Chars[b%byte(len(base62Chars))]
	}
	return string(result), nil
}
```

- **Entropy**: ~381 bits ($62^{64} \approx 5.16 \times 10^{114}$ combinations).
- **Format**: 64 mixed-case alphanumeric characters `[a-zA-Z0-9]` (e.g. `9kL2xP8vB1mN7qR4tY6wZ3aC5eG8hJ0kL2xP8vB1mN7qR4tY6wZ3aC5eG8hJ0kL2`).

#### 2. Ephemeral Session Identifiers (`GenerateSessionID`)
Browser session IDs for the `ctop_session` cookie are generated with 256 bits of CSPRNG entropy:

```go
// GenerateSessionID generates a secure 64-character random hexadecimal session ID (256-bit entropy).
func GenerateSessionID() (string, error) {
	b := make([]byte, 32) // 32 bytes = 256 bits of CSPRNG entropy
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate random session ID: %w", err)
	}
	return hex.EncodeToString(b), nil // Exactly 64 hex characters
}
```

#### 3. Constant-Time Verification (`crypto/subtle`)
All bearer token and session token comparisons are evaluated in constant time to eliminate timing side-channel attacks:

```go
func secureCompare(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
```

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

## 3. Server-Side Security Middleware (`corsMiddleware`) & Zero-Leak Web Authentication

The authentication middleware in `pkg/web/server.go` intercepts every incoming HTTP/HTTPS request, implementing a **Dual-Channel Zero-Leak Security Model**:

```mermaid
flowchart TD
    Req[Incoming HTTP / HTTPS Request] --> AuthCheck{Is Web Auth Enabled?}
    AuthCheck -- No --> Allow[Proceed to Route Handler]
    AuthCheck -- Yes --> DirectLocalCheck{isDirectLocalAccess?<br/>RemoteAddr is 127.0.0.1<br/>AND NO X-Forwarded-* Headers}
    
    DirectLocalCheck -- Yes (Direct Loopback) --> Allow
    DirectLocalCheck -- No (Remote / Proxied) --> TLSCheck{Is TLS Active or Secure Proxy?}
    
    TLSCheck -- No --> Reject403[Return 403 Forbidden - TLS Required]
    TLSCheck -- Yes --> ExtractAuth[Extract Authentication]
    
    ExtractAuth --> CheckBearer{Authorization: Bearer <token>?}
    CheckBearer -- Valid (Constant-Time) --> Allow
    CheckBearer -- No / Mismatch --> CheckCookie{Cookie: ctop_session=<valid>?}
    
    CheckCookie -- Valid (Unexpired Session) --> Allow
    CheckCookie -- No / Invalid --> CheckQuery{URL ?token= or ?auth=?}
    
    CheckQuery -- Present --> RejectQuery[Return 401 - Query Param Tokens Discontinued]
    CheckQuery -- Not Present --> Reject401[Return 401 Unauthorized + WWW-Authenticate Header]
```

### 3.1. Dual-Channel Authentication Channels

1. **Channel 1: Programmatic REST & SSE APIs (`Authorization: Bearer <token>`)**:
   Standard RFC 6750 Bearer authentication for scripts, monitoring agents, cURL, and reverse proxies:
   ```http
   GET /api/v1/metrics HTTP/1.1
   Host: localhost:9090
   Authorization: Bearer 1263171892d7bc4ba3b4e632ffd8d4fb
   ```

2. **Channel 2: Web Dashboard UI (`HttpOnly; SameSite=Strict; Secure` Session Cookie)**:
   - **Local Access (`127.0.0.1` / `localhost` without Proxy Headers)**: Unlocked directly without password prompt.
   - **Remote Access (External IP / Reverse Proxy / TLS)**: The browser dashboard presents an "Unlock Dashboard" modal. On submitting the token, the client performs `POST /api/v1/auth/login` to establish a secure, ephemeral session cookie:
     ```http
     Set-Cookie: ctop_session=4b3d7a8f...; Path=/; HttpOnly; SameSite=Strict; Secure; Max-Age=86400
     ```
   - The browser automatically includes `ctop_session` in subsequent API requests and `EventSource` (`/api/v1/stream`) connections.

3. **Strict Discontinuation of URL Query Parameter Tokens (`?token=` / `?auth=`)**:
   - In alignment with **RFC 6750** and **OWASP** recommendations, query parameter authentication is **completely rejected with `401 Unauthorized`**.
   - This prevents tokens from leaking into web server access logs (`access.log`), browser history, address bar autocomplete, screen shares, and outbound `Referer` headers.

### 3.2. Bounded In-Memory Session Store Specification

To prevent unbounded memory growth and session fixation attacks, sessions are managed by a thread-safe, bounded in-memory store:

| Parameter | Specification | Purpose |
| :--- | :--- | :--- |
| **Identifier Format** | 64 hex characters (256-bit CSPRNG) | Guarantees collision resistance and unguessability. |
| **Max Capacity** | 100 concurrent active sessions | Prevents memory exhaustion attacks via session flooding. |
| **Eviction Policy** | Least Recently Used (LRU) | Oldest inactive sessions are purged when capacity is reached. |
| **Absolute TTL** | 24 Hours (`Max-Age=86400`) | Enforces periodic re-authentication for long-lived browsers. |
| **Idle Timeout** | 2 Hours | Inactivates abandoned sessions automatically. |
| **Process Lifecycle** | Ephemeral (Zero disk persistence) | All sessions are immediately invalidated on daemon restart. |

### 3.3. In-Memory Login Rate Limiting & Brute-Force Guard

To protect the `POST /api/v1/auth/login` endpoint against automated credential guessing and CPU/socket exhaustion:
- **Rate Limit Window**: Sliding window tracking failed login attempts per client IP.
- **Threshold**: Maximum **5 failed attempts per IP per minute**.
- **Violation Policy**: Responds with `429 Too Many Requests` and a `Retry-After: 60` HTTP header.
- **Success Reset**: Successful authentication clears the failure counter for the client IP.

### 3.4. Authentication Endpoints

| Endpoint | Method | Payload | Purpose |
| :--- | :--- | :--- | :--- |
| `/api/v1/auth/login` | `POST` | `{"token": "..."}` | Validates token (rate-limited, constant-time), creates session, and issues `ctop_session` cookie. |
| `/api/v1/auth/logout` | `POST` | Empty | Destroys active session from in-memory store and expires `ctop_session` cookie. |
| `/api/v1/auth/status` | `GET` | None | Returns current authentication state (`{"authenticated": true/false}`). |

### 3.5. Security Response Specifications

#### 1. Unauthorized Response (`401 Unauthorized`)
```json
{
  "error": "Unauthorized - invalid or missing authentication token"
}
```
*Headers*: `WWW-Authenticate: Bearer realm="ctop"`, `Content-Type: application/json; charset=utf-8`

#### 2. Rate Limit Exceeded Response (`429 Too Many Requests`)
```json
{
  "error": "Too many failed login attempts. Please retry after 60 seconds."
}
```
*Headers*: `Retry-After: 60`, `Content-Type: application/json; charset=utf-8`

#### 3. Forbidden Response (`403 Forbidden`)
```json
{
  "error": "Forbidden - TLS encryption required (direct TLS or X-Forwarded-Proto: https)"
}
```

---

## 4. Transport Layer Security (TLS / HTTPS)

To protect tokens and telemetry from in-flight eavesdropping or man-in-the-middle interception, `ctop` supports two standard TLS deployment architectures:

### 4.1. Minimum Protocol & Cipher Suite Enforcement

When direct TLS encryption is enabled, `ctop` enforces strict modern cryptographic standards in Go's `tls.Config`:

- **Minimum TLS Protocol**: Enforce `tls.VersionTLS12` (TLS 1.0, TLS 1.1, and SSLv3 are strictly disabled).
- **Supported Cryptographic Protocols**: TLS 1.2 and TLS 1.3 only.
- **Recommended Elliptic Curves**: `X25519`, `CurveP256`.
- **Enforced Secure Cipher Suites**:
  - `TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256`
  - `TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256`
  - `TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384`
  - `TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384`
  - `TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305`
  - `TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305`

### 4.2. Direct Native TLS Encryption

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

### 4.3. Reverse Proxy TLS Termination (NGINX, Traefik, Caddy, Cloud ALBs)

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

### 4.4. Path Traversal & Absolute Path Security Defense

In accordance with strict security policies:
- All certificate and key paths (`--web-tls-cert`, `--web-tls-key`, `--tls-ca`, `--tls-cert`, `--tls-key`) are sanitized and resolved through `filepath.Abs(filepath.Clean(p))` on startup.
- If relative path traversal sequences (e.g. `../../`) or malformed path injections are detected, they are resolved against canonical filesystem boundaries.
- Container REST routes (e.g. `/api/v1/containers/{id}/top`) reject any ID containing `..`, `/`, or `\` with `400 Bad Request` before accessing engine APIs.

---

## 5. End-to-End Verification Matrix

| Test Suite | Test Function | Verified Security Invariant |
| :--- | :--- | :--- |
| **Unit (`pkg/web`)** | `TestGenerateAuthToken` | Verifies 32-character hexadecimal output and CSPRNG uniqueness. |
| **Unit (`pkg/web`)** | `TestWebServerAuthToken` | Verifies 401 on missing token, 401 on invalid token, 200 on Bearer header, 401 on deprecated query param. |
| **Unit (`pkg/web`)** | `TestWebServerSessionCookie` | Verifies session creation via `POST /api/v1/auth/login`, cookie validation, and session logout revocation. |
| **Unit (`pkg/web`)** | `TestWebServerDirectLocalAccess` | Verifies direct `127.0.0.1` bypasses password modal, while requests with `X-Forwarded-For` or remote IPs strictly enforce authentication. |
| **Unit (`pkg/web`)** | `TestWebServerSessionCapacityAndTTL` | Verifies LRU eviction at capacity (100 sessions) and TTL expiration after timeout. |
| **Unit (`pkg/web`)** | `TestWebServerLoginRateLimiting` | Verifies `429 Too Many Requests` is returned after 5 failed login attempts from a single IP. |
| **Unit (`pkg/web`)** | `TestSecureTokenFileOperations` | Verifies file creation at `0400` permissions and cleanup on removal. |
| **Unit (`pkg/web`)** | `TestTLSVersionEnforcement` | Verifies TLS 1.0/1.1 rejection and TLS 1.2/1.3 success. |
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

### Client-Side Authentication Usage Guide

#### 1. Web Browser (Dashboard UI Access)

Navigate directly using standard, clean URLs (no query string secrets):

```text
# Local plain HTTP (auto-unlocked on loopback)
http://localhost:9090/

# Remote HTTPS (prompts for 64-char token once in browser modal)
https://monitoring.example.com:9443/

# Behind a Reverse Proxy with subpath prefix
https://proxy.example.com/ctop/
```

- **Local Access (`127.0.0.1` / `localhost`)**: Dashboard loads immediately without asking for a password.
- **Remote Access**: An in-browser modal prompts for the 64-character token once, establishing an `HttpOnly` session cookie (`ctop_session`). Tokens never appear in browser history or address bars.

#### 2. cURL / CLI HTTP Clients

**REST API Request via HTTP Bearer Header**
```bash
TOKEN=$(cat ~/.config/ctop/token)
curl -H "Authorization: Bearer $TOKEN" https://localhost:9443/api/v1/containers
```

**Live Real-Time SSE Stream (`-N` disables stream buffering)**
```bash
TOKEN=$(cat ~/.config/ctop/token)
curl -N -H "Authorization: Bearer $TOKEN" https://localhost:9443/api/v1/stream
```

#### 3. JavaScript / Web Clients (Fetch & SSE)

**Fetch API Request (Bearer Token)**:
```javascript
const token = "9kL2xP8vB1mN7qR4tY6wZ3aC5eG8hJ0kL2xP8vB1mN7qR4tY6wZ3aC5eG8hJ0kL2";

async function fetchContainers() {
  const response = await fetch("https://localhost:9443/api/v1/containers", {
    headers: {
      "Authorization": `Bearer ${token}`
    }
  });
  if (!response.ok) {
    throw new Error(`HTTP ${response.status}: ${response.statusText}`);
  }
  return await response.json();
}
```

**Browser Server-Sent Events (`EventSource`) with Session Cookie**:
When logged into the dashboard, `EventSource` automatically passes the `ctop_session` cookie:
```javascript
// Connects seamlessly using active HttpOnly session cookie
const source = new EventSource(`${BASE_PATH}/api/v1/stream`);

source.onmessage = (event) => {
  const telemetry = JSON.parse(event.data);
  console.log("Telemetry received:", telemetry);
};

source.onerror = (err) => {
  console.error("SSE stream disconnected or unauthorized", err);
};
```

#### 4. Python Scripts & Automation
```python
import os
import requests

# Read token from ~/.config/ctop/token
token_path = os.path.expanduser("~/.config/ctop/token")
with open(token_path, "r") as f:
    token = f.read().strip()

# Query REST API with Bearer token header
headers = {"Authorization": f"Bearer {token}"}
response = requests.get("https://localhost:9443/api/v1/containers", headers=headers)

if response.status_code == 200:
    containers = response.json()
    print(f"Discovered {len(containers)} containers")
else:
    print(f"Auth failed: {response.status_code} - {response.text}")
```
