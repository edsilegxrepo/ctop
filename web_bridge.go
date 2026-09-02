// web_bridge.go bridges ctop's container registry and telemetry collectors with the read-only embedded web server and SSE broadcaster.
//
// Objective:
//
//	Adapt ctop's live internal Container and Connector abstractions to the decoupled interfaces required
//	by the pkg/web dashboard engine (ContainerProvider, LogsProvider, ExecInspectProvider, FileExplorerProvider, DiagnosticProvider).
//
// Core Components:
//   - superContainerProvider: Adapter implementing web.ContainerProvider, web.LogsProvider, web.ExecInspectProvider, web.FileExplorerProvider, web.DiagnosticProvider.
//   - StartWebBridge: Background coordinator orchestrating web server lifecycle and SSE telemetry broadcast ticker.
//
// Functionality:
//   - Converts live container metrics and metadata into JSON-safe sanitized snapshots.
//   - Tails and formats live log streams with ANSI stripping and JSON parsing.
//   - Exposes container top processes, diffs, compose templates, files, and diagnostics via REST/SSE.
//
// Data Flow:
//
//	Connector Super / Containers -> superContainerProvider -> WebServer API Handlers / Broadcaster -> HTTP / SSE Clients.
package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/edsilegx/ctop/pkg/audit"
	"github.com/edsilegx/ctop/pkg/connector"
	"github.com/edsilegx/ctop/pkg/generator"
	"github.com/edsilegx/ctop/pkg/sanitize"
	"github.com/edsilegx/ctop/pkg/web"
)

type superContainerProvider struct {
	cSuper *connector.ConnectorSuper
}

func (p *superContainerProvider) GetContainerSnapshots() []web.ContainerSnapshot {
	if p.cSuper == nil {
		return nil
	}
	cSource, err := p.cSuper.Get()
	if err != nil || cSource == nil {
		return nil
	}

	var list []web.ContainerSnapshot
	for _, c := range cSource.All() {
		c.RLock()
		state := strings.ToLower(c.Meta.Get("state"))
		uptime := c.Meta.Get("uptime")
		if state != "running" && state != "paused" {
			uptime = "-"
		}

		snap := web.ContainerSnapshot{
			ID:               c.Id,
			Name:             c.Meta.Get("name"),
			Image:            c.Meta.Get("image"),
			State:            c.Meta.Get("state"),
			Health:           c.Meta.Get("health"),
			Host:             c.HostID,
			Created:          c.Meta.Get("created"),
			Uptime:           uptime,
			CPUUtil:          nonNeg(c.CPUUtil),
			MemUsage:         nonNeg(c.MemUsage),
			MemLimit:         nonNeg(c.MemLimit),
			MemPercent:       nonNeg(c.MemPercent),
			MemRss:           nonNeg(c.MemRss),
			MemCache:         nonNeg(c.MemCache),
			NetRx:            nonNeg(c.NetRx),
			NetTx:            nonNeg(c.NetTx),
			NetRxRate:        nonNeg(c.NetRxRate),
			NetTxRate:        nonNeg(c.NetTxRate),
			IOBytesRead:      nonNeg(c.IOBytesRead),
			IOBytesWrite:     nonNeg(c.IOBytesWrite),
			IORateRead:       nonNeg(c.IORateRead),
			IORateWrite:      nonNeg(c.IORateWrite),
			Pids:             nonNeg(c.Pids),
			IPs:              c.Meta.Get("IPs"),
			Ports:            c.Meta.Get("ports"),
			WebPort:          c.Meta.Get("Web Port"),
			Command:          c.Meta.Get("cmd"),
			Entrypoint:       c.Meta.Get("entrypoint"),
			WorkDir:          c.Meta.Get("workdir"),
			User:             c.Meta.Get("user"),
			RestartPol:       c.Meta.Get("restartPolicy"),
			MemLimitStr:      c.Meta.Get("memLimit"),
			ImageID:          c.Meta.Get("imageId"),
			ImageArch:        c.Meta.Get("imageArch"),
			ImageSize:        c.Meta.Get("imageSize"),
			ImageLayers:      c.Meta.Get("imageLayers"),
			ImageAuthor:      c.Meta.Get("imageAuthor"),
			ImageCreated:     c.Meta.Get("imageCreated"),
			ImageDockerVer:   c.Meta.Get("imageDockerVersion"),
			ImageLabels:      parseLabels(c.Meta.Get("imageLabels")),
			ImageEnv:         parseEnv(c.Meta.Get("imageEnv")),
			ImageCmd:         c.Meta.Get("imageCmd"),
			ImageEntrypoint:  c.Meta.Get("imageEntrypoint"),
			ImageWorkDir:     c.Meta.Get("imageWorkDir"),
			ImageUser:        c.Meta.Get("imageUser"),
			ImageVolumes:     c.Meta.Get("imageVolumes"),
			ImagePorts:       c.Meta.Get("imagePorts"),
			GeneratedRunCmd:  generator.GenerateRunCmd(c.Meta),
			GeneratedCompose: generator.GenerateCompose(c.Meta),
			Env:              parseEnv(c.Meta.Get("[ENV-VAR]")),
			Labels:           parseLabels(c.Meta.Get("[LABELS]")),
			Mounts:           parseMounts(c.Meta.Get("[MOUNTS]")),
			Networks:         parseNetworks(c.Meta.Get("[NETWORKS]")),
			Timestamp:        time.Now().UTC(),
		}
		c.RUnlock()
		list = append(list, snap)
	}
	return list
}

func (p *superContainerProvider) GetContainerTop(id string) (web.TopResult, error) {
	if p.cSuper == nil {
		return web.TopResult{}, fmt.Errorf("connector unavailable")
	}
	cSource, err := p.cSuper.Get()
	if err != nil || cSource == nil {
		return web.TopResult{}, fmt.Errorf("connector unavailable")
	}
	for _, c := range cSource.All() {
		if c.Id == id || strings.HasPrefix(c.Id, id) || strings.EqualFold(c.Meta.Get("name"), id) {
			topRes, err := c.Top()
			if err != nil {
				return web.TopResult{}, err
			}
			return web.TopResult{
				Titles:    topRes.Titles,
				Processes: topRes.Processes,
			}, nil
		}
	}
	return web.TopResult{}, fmt.Errorf("container not found")
}

func (p *superContainerProvider) GetContainerDiff(id string) ([]web.DiffChange, error) {
	if p.cSuper == nil {
		return nil, fmt.Errorf("connector unavailable")
	}
	cSource, err := p.cSuper.Get()
	if err != nil || cSource == nil {
		return nil, fmt.Errorf("connector unavailable")
	}
	for _, c := range cSource.All() {
		if c.Id == id || strings.HasPrefix(c.Id, id) || strings.EqualFold(c.Meta.Get("name"), id) {
			changes, err := c.Changes()
			if err != nil {
				return nil, err
			}
			var res []web.DiffChange
			for _, ch := range changes {
				kindStr := "C"
				switch ch.Kind {
				case 1:
					kindStr = "A"
				case 2:
					kindStr = "D"
				}
				res = append(res, web.DiffChange{
					Path: ch.Path,
					Kind: kindStr,
				})
			}
			return res, nil
		}
	}
	return nil, fmt.Errorf("container not found")
}

func (p *superContainerProvider) ReadContainerDir(id, dirPath string) ([]web.FileEntry, error) {
	if p.cSuper == nil {
		return nil, fmt.Errorf("connector unavailable")
	}
	cSource, err := p.cSuper.Get()
	if err != nil || cSource == nil {
		return nil, fmt.Errorf("connector unavailable")
	}
	for _, c := range cSource.All() {
		if c.Id == id || strings.HasPrefix(c.Id, id) || strings.EqualFold(c.Meta.Get("name"), id) {
			entries, err := c.ReadDir(dirPath)
			if err != nil {
				return nil, err
			}
			var res []web.FileEntry
			for _, ent := range entries {
				res = append(res, web.FileEntry{
					Name:    ent.Name,
					Path:    ent.Path,
					IsDir:   ent.IsDir,
					Size:    ent.Size,
					Mode:    ent.Mode,
					ModTime: ent.ModTime,
				})
			}
			return res, nil
		}
	}
	return nil, fmt.Errorf("container not found")
}

func (p *superContainerProvider) ReadContainerFile(id, filePath string, maxBytes int64) (string, error) {
	if p.cSuper == nil {
		return "", fmt.Errorf("connector unavailable")
	}
	cSource, err := p.cSuper.Get()
	if err != nil || cSource == nil {
		return "", fmt.Errorf("connector unavailable")
	}
	for _, c := range cSource.All() {
		if c.Id == id || strings.HasPrefix(c.Id, id) || strings.EqualFold(c.Meta.Get("name"), id) {
			return c.ReadFile(filePath, maxBytes)
		}
	}
	return "", fmt.Errorf("container not found")
}

func (p *superContainerProvider) SearchContainerFiles(id, basePath, pattern string, maxResults int) ([]web.FileEntry, error) {
	if p.cSuper == nil {
		return nil, fmt.Errorf("connector unavailable")
	}
	cSource, err := p.cSuper.Get()
	if err != nil || cSource == nil {
		return nil, fmt.Errorf("connector unavailable")
	}
	for _, c := range cSource.All() {
		if c.Id == id || strings.HasPrefix(c.Id, id) || strings.EqualFold(c.Meta.Get("name"), id) {
			entries, err := c.SearchFiles(basePath, pattern, maxResults)
			if err != nil {
				return nil, err
			}
			var res []web.FileEntry
			for _, ent := range entries {
				res = append(res, web.FileEntry{
					Name:    ent.Name,
					Path:    ent.Path,
					IsDir:   ent.IsDir,
					Size:    ent.Size,
					Mode:    ent.Mode,
					ModTime: ent.ModTime,
				})
			}
			return res, nil
		}
	}
	return nil, fmt.Errorf("container not found")
}

func parseMounts(raw string) []web.MountInfo {
	if raw == "" {
		return nil
	}
	var res []web.MountInfo
	for _, part := range strings.Split(raw, ";;") {
		fields := strings.Split(part, ":::")
		if len(fields) >= 4 {
			driver := ""
			if len(fields) >= 5 {
				driver = fields[4]
			}
			res = append(res, web.MountInfo{
				Destination: fields[0],
				Source:      fields[1],
				Type:        fields[2],
				Mode:        fields[3],
				Driver:      driver,
			})
		}
	}
	return res
}

func parseNetworks(raw string) []web.NetworkInfo {
	if raw == "" {
		return nil
	}
	var res []web.NetworkInfo
	for _, part := range strings.Split(raw, ";;") {
		fields := strings.Split(part, ":::")
		if len(fields) >= 4 {
			prefix := 0
			if len(fields) >= 5 {
				if p, err := strconv.Atoi(fields[4]); err == nil {
					prefix = p
				}
			}
			res = append(res, web.NetworkInfo{
				Name:      fields[0],
				IPAddress: fields[1],
				Gateway:   fields[2],
				Mac:       fields[3],
				PrefixLen: prefix,
			})
		}
	}
	return res
}

func parseLabels(raw string) map[string]string {
	if raw == "" {
		return nil
	}
	res := make(map[string]string)
	for _, part := range strings.Split(raw, ";;") {
		idx := strings.Index(part, "=")
		if idx > 0 {
			k := part[:idx]
			v := part[idx+1:]
			if !sanitize.IsSensitiveKey(k) {
				res[k] = v
			}
		}
	}
	return res
}

func parseEnv(raw string) []string {
	if raw == "" {
		return nil
	}
	return sanitize.SanitizeEnv(strings.Split(raw, ";"))
}

func nonNeg[T ~int | ~int64](v T) T {
	if v < 0 {
		return 0
	}
	return v
}

// WebOptions configures runtime parameters for the web server bridge.
type WebOptions struct {
	URLPrefix       string
	AuthToken       bool
	PersistentToken bool
	TLSCert         string
	TLSKey          string
	AuditLog        string
}

func startWebServer(addr, version, urlPrefix string, cSuper *connector.ConnectorSuper, opts ...WebOptions) (*web.Server, func(), error) {
	var opt WebOptions
	if len(opts) > 0 {
		opt = opts[0]
	}
	if opt.URLPrefix == "" {
		opt.URLPrefix = urlPrefix
	}

	if opt.AuditLog != "" && audit.Get() == nil {
		if _, err := audit.Init(opt.AuditLog); err != nil {
			return nil, nil, fmt.Errorf("failed to initialize audit log at %s: %w", opt.AuditLog, err)
		}
	}

	isTLS := opt.TLSCert != "" && opt.TLSKey != ""

	addr = strings.TrimSpace(addr)
	if !isTLS {
		// MANDATORY SECURITY INVARIANT:
		// Without native TLS encryption, the listener is strictly bound to 127.0.0.1 loopback only.
		// No remote or 0.0.0.0 listeners are allowed without TLS.
		port := addr
		if idx := strings.LastIndex(addr, ":"); idx != -1 {
			port = addr[idx+1:]
		}
		if port == "" || port == addr {
			port = "9090"
		}
		addr = "127.0.0.1:" + port
	} else {
		// In TLS mode:
		// - If only a port number is provided (e.g. "9443"), normalize to ":9443"
		// - If ":9443" is provided, keep it as ":9443" (uses IPv6 dual-stack if supported, fallback to IPv4)
		// - If an explicit IPv4 address is provided (e.g. "0.0.0.0:9443"), keep it as "0.0.0.0:9443" (strictly IPv4)
		if !strings.Contains(addr, ":") {
			addr = ":" + addr
		}
	}

	prov := &superContainerProvider{cSuper: cSuper}
	broadcaster := web.NewBroadcaster()
	srv := web.NewServer(addr, version, prov, broadcaster, opt.URLPrefix)

	var tokenPath string
	if opt.AuthToken {
		if opt.PersistentToken {
			// If persistent token is requested, check if a valid token already exists on disk
			token, tErr := web.ReadSecureTokenFile("")
			if tErr == nil && len(token) >= web.MinAuthTokenLength {
				if err := srv.EnableAuthWithToken(token); err != nil {
					return nil, nil, fmt.Errorf("failed to load persistent authentication token: %w", err)
				}
				tokenPath = web.DefaultAuthTokenPath()
			} else {
				// Generate ONCE and write to secure file
				token, err := srv.EnableAuth()
				if err != nil {
					return nil, nil, fmt.Errorf("failed to enable web authentication token: %w", err)
				}
				var wErr error
				tokenPath, wErr = web.WriteSecureTokenFile("", token)
				if wErr != nil {
					return nil, nil, fmt.Errorf("failed to write secure token file: %w", wErr)
				}
			}
		} else {
			// Ephemeral (default): generate fresh token every startup
			token, err := srv.EnableAuth()
			if err != nil {
				return nil, nil, fmt.Errorf("failed to enable web authentication token: %w", err)
			}
			var wErr error
			tokenPath, wErr = web.WriteSecureTokenFile("", token)
			if wErr != nil {
				return nil, nil, fmt.Errorf("failed to write secure token file: %w", wErr)
			}
		}
	}

	if opt.TLSCert != "" && opt.TLSKey != "" {
		srv.SetTLS(opt.TLSCert, opt.TLSKey)
	}

	if err := srv.Start(); err != nil {
		if tokenPath != "" && !opt.PersistentToken {
			web.RemoveSecureTokenFile(tokenPath)
		}
		return nil, nil, fmt.Errorf("failed to start web dashboard server: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Background ticker broadcasting real-time telemetry events to SSE clients
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if broadcaster.SubscriberCount() > 0 {
					snapshots := prov.GetContainerSnapshots()
					sys := web.AggregateSnapshots(snapshots)
					ev := web.TelemetryEvent{
						Type:       "metrics",
						Timestamp:  time.Now().UTC().Format(time.RFC3339),
						System:     sys,
						Containers: snapshots,
					}
					broadcaster.Broadcast(ev)
				}
			}
		}
	}()

	cleanup := func() {
		cancel()
		if tokenPath != "" && !opt.PersistentToken {
			web.RemoveSecureTokenFile(tokenPath)
		}
		shutdownCtx, sCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer sCancel()
		_ = srv.Stop(shutdownCtx)
	}

	return srv, cleanup, nil
}
