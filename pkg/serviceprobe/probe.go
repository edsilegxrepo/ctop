// Package serviceprobe provides bounded HTTP probing and status capture for container endpoints.
//
// Objective:
//
//	Execute fast, non-blocking, and bounded HTTP/HTTPS probes against container web ports,
//	capturing status codes, response headers, content types, latency, and bounded body content.
//
// Core Components:
//   - HTTPProbeResult: Comprehensive probe telemetry including timing, response headers, and content classification.
//   - MaxProbeResponseBodyLimit: Strict 2MB ceiling preventing memory exhaustion on large responses.
//   - ProbeHTTP: Context-bounded HTTP client with 3-redirect cap and self-signed TLS support.
//
// Functionality:
//   - Automated protocol detection (HTML, JSON, plain text).
//   - Bounded response reads via io.LimitReader.
//   - Timing measurement and deterministic header extraction.
//
// Data Flow:
//
//	Target URL -> ProbeHTTP(ctx, url, timeout) -> Bounded HTTP GET -> HTTPProbeResult.
package serviceprobe

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// HTTPProbeResult contains execution telemetry and payload from an HTTP probe.
type HTTPProbeResult struct {
	TargetURL   string            `json:"target_url"`
	StatusCode  int               `json:"status_code"`
	StatusText  string            `json:"status_text"`
	Duration    time.Duration     `json:"duration"`
	DurationMS  float64           `json:"duration_ms"`
	Proto       string            `json:"proto"`
	Headers     map[string]string `json:"headers"`
	Body        string            `json:"body"`
	BodySize    int64             `json:"body_size"`
	ContentType string            `json:"content_type"`
	IsHTML      bool              `json:"is_html"`
	IsJSON      bool              `json:"is_json"`
	Error       string            `json:"error,omitempty"`
}

// MaxProbeResponseBodyLimit limits read response size to 2 MB.
const MaxProbeResponseBodyLimit = 2 * 1024 * 1024

// isDisallowedProbeTarget checks if a target hostname is a prohibited cloud metadata or link-local address.
func isDisallowedProbeTarget(host string) bool {
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "" {
		return true
	}
	if host == "metadata.google.internal" || host == "metadata" || strings.HasSuffix(host, ".metadata.google.internal") {
		return true
	}
	h, _, err := net.SplitHostPort(host)
	if err == nil {
		host = h
	}
	ip := net.ParseIP(host)
	if ip != nil {
		if ip4 := ip.To4(); ip4 != nil {
			if ip4[0] == 169 && ip4[1] == 254 {
				return true
			}
			if ip.IsUnspecified() {
				return true
			}
		} else {
			if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
				return true
			}
			if ip.String() == "fd00:ec2::254" {
				return true
			}
		}
	}
	return false
}

// ProbeHTTP executes a fast, bounded HTTP request to the target URL.
func ProbeHTTP(ctx context.Context, targetURL string, timeout time.Duration) *HTTPProbeResult {
	if timeout <= 0 {
		timeout = 3 * time.Second
	}

	result := &HTTPProbeResult{
		TargetURL: targetURL,
		Headers:   make(map[string]string),
	}

	// Normalize URL
	if !strings.HasPrefix(targetURL, "http://") && !strings.HasPrefix(targetURL, "https://") {
		targetURL = "http://" + targetURL
		result.TargetURL = targetURL
	}

	parsedURL, parseErr := url.Parse(targetURL)
	if parseErr != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		result.Error = fmt.Sprintf("invalid or unsupported request URL: %v", parseErr)
		return result
	}
	if parsedURL.Hostname() == "" {
		result.Error = "invalid request URL: missing host"
		return result
	}
	if isDisallowedProbeTarget(parsedURL.Hostname()) {
		result.Error = fmt.Sprintf("request blocked by SSRF security policy: %s", parsedURL.Hostname())
		return result
	}

	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// #nosec G107 -- Bounded internal probe client for container endpoint inspection; target is validated and constrained to HTTP/HTTPS
	// codeql[go/request-forgery] In-engine container inspector queries validated container endpoints
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, targetURL, nil)
	if err != nil {
		result.Error = fmt.Sprintf("invalid request URL: %v", err)
		return result
	}

	req.Header.Set("User-Agent", "ctop/0.9 (embedded-inspector; pure-go)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/json,text/plain,*/*")

	// Custom transport allowing internal self-signed TLS certs
	tr := &http.Transport{
		// #nosec G402 -- In-engine service probe requires InsecureSkipVerify to probe local/in-container endpoints with self-signed TLS certificates
		// CodeQL [go/disabled-certificate-check] Required for inspecting containers with self-signed TLS certs
		// nosemgrep: problem-based-packs.insecure-transport.go-stdlib.bypass-tls-verification.bypass-tls-verification -- In-engine probe requires InsecureSkipVerify for self-signed container endpoints
		TLSClientConfig:       &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12}, // nosemgrep
		DisableKeepAlives:     true,
		ResponseHeaderTimeout: timeout,
	}

	client := &http.Client{
		Transport: tr,
		Timeout:   timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return http.ErrUseLastResponse
			}
			if req.URL != nil && isDisallowedProbeTarget(req.URL.Hostname()) {
				return fmt.Errorf("redirect blocked by SSRF security policy to host: %s", req.URL.Hostname())
			}
			return nil
		},
	}

	start := time.Now()
	// #nosec G107 -- Bounded internal probe client for container endpoint inspection; target is validated and constrained to HTTP/HTTPS
	// codeql[go/request-forgery] In-engine container inspector purposefully queries validated container endpoints
	resp, err := client.Do(req)
	result.Duration = time.Since(start)
	result.DurationMS = float64(result.Duration.Microseconds()) / 1000.0

	if err != nil {
		result.Error = fmt.Sprintf("connection failed: %v", err)
		return result
	}
	defer func() { _ = resp.Body.Close() }()

	result.StatusCode = resp.StatusCode
	result.StatusText = http.StatusText(resp.StatusCode)
	if result.StatusText == "" {
		result.StatusText = resp.Status
	}
	result.Proto = resp.Proto

	// Capture response headers
	for k, vals := range resp.Header {
		if len(vals) > 0 {
			result.Headers[k] = strings.Join(vals, ", ")
		}
	}

	contentType := resp.Header.Get("Content-Type")
	result.ContentType = contentType
	lowerCT := strings.ToLower(contentType)
	result.IsHTML = strings.Contains(lowerCT, "html")
	result.IsJSON = strings.Contains(lowerCT, "json")

	// Read body up to MaxProbeResponseBodyLimit
	lr := io.LimitReader(resp.Body, MaxProbeResponseBodyLimit)
	bodyBytes, err := io.ReadAll(lr)
	if err != nil {
		result.Error = fmt.Sprintf("failed to read response body: %v", err)
		return result
	}

	result.Body = string(bodyBytes)
	result.BodySize = int64(len(bodyBytes))

	return result
}
