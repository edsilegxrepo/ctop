# NGINX Reverse Proxy End-to-End Test Suite for ctop

This directory contains the permanent test artifacts and automation scripts for validating `ctop` reverse proxy deployments, SSL/TLS termination, SSE streaming, authentication, and security invariants in real environments (WSL / Linux).

---

## Files

- **`nginx.conf`**: Base NGINX configuration template defining:
  - Insecure plain HTTP reverse proxy listener on port `19080` (verifies `403 Forbidden` rejection of unencrypted remote traffic).
  - Secure SSL/TLS terminating reverse proxy listener on port `19443` with `X-Forwarded-*` header forwarding, chunked transfer encoding, and unbuffered SSE event streaming.
- **`test_nginx_e2e.sh`**: Standalone executable test runner orchestrating `ctop` and `nginx` lifecycle, running all 10 end-to-end assertion suites, and cleaning up background processes.

---

## Running the E2E Proxy Test Suite

Execute the test runner in WSL or Linux:

```bash
# From repository root:
./tests/nginx/test_nginx_e2e.sh
```

---

## Verified Test Cases

| Step | Verification Scenario | Expected Outcome |
| :---: | :--- | :--- |
| **1** | Direct Local Loopback Access on backend (`127.0.0.1:19090`) | `200 OK` (Auto-unlocked without password prompt) |
| **2** | Insecure Remote Request via HTTP Proxy (Port `19080`) | `403 Forbidden` (*"TLS encryption required for remote access"*) |
| **3** | Secure HTTPS Proxy Request without Token (Port `19443`) | `401 Unauthorized` |
| **4** | Secure HTTPS Proxy Request with Invalid Token | `401 Unauthorized` |
| **5** | Secure HTTPS Proxy Request with `Authorization: Bearer <token>` | `200 OK` (Returns container list JSON) |
| **6** | Secure HTTPS Proxy Request with Query Param (`?token=...`) | `401 Unauthorized` (Strict URL token rejection) |
| **7** | Web Dashboard SPA HTML delivery (`GET /probe/`) | `200 OK` |
| **8** | Health Endpoint (`GET /probe/api/v1/health`) via Proxy | `200 OK` |
| **9** | Metrics Endpoint (`GET /probe/api/v1/metrics`) with Bearer Token | `200 OK` |
| **10** | Schema Endpoint (`GET /probe/api/v1/schema`) | `200 OK` |
| **11** | Web Session Login via `POST /api/v1/auth/login` | `200 OK` (Issues `ctop_session` cookie) |
| **12** | Subsequent Requests via `ctop_session` Cookie | `200 OK` |
| **13** | Data Export (`GET /probe/api/v1/export`) via Session Cookie | `200 OK` |
| **14** | Live Real-Time SSE Stream (`/api/v1/stream`) via Proxy | Valid `data: {...}` event stream frames received |
| **15** | Web Session Logout via `POST /api/v1/auth/logout` | `200 OK` (Revokes session in backend store) |
| **16** | Post-Logout Request Attempt | `401 Unauthorized` |
