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
	"net/http"
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

	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, targetURL, nil)
	if err != nil {
		result.Error = fmt.Sprintf("invalid request URL: %v", err)
		return result
	}

	req.Header.Set("User-Agent", "ctop/0.9.2 (embedded-inspector; pure-go)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/json,text/plain,*/*")

	// Custom transport allowing internal self-signed TLS certs
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true, // #nosec G402 -- InsecureSkipVerify is required for internal container diagnostics against self-signed certs
		},
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
			return nil
		},
	}

	start := time.Now()
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
