// web_bridge.go bridges ctop's container registry and telemetry collectors with the read-only embedded web server and SSE broadcaster.
package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/edsilegx/ctop/pkg/sanitize"
	"github.com/edsilegx/ctop/pkg/web"
)

type cursorContainerProvider struct {
	cursor *GridCursor
}

func (p *cursorContainerProvider) GetContainerSnapshots() []web.ContainerSnapshot {
	if p.cursor == nil || p.cursor.cSuper == nil {
		return nil
	}
	cSource, err := p.cursor.cSuper.Get()
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
			ID:           c.Id,
			Name:         c.Meta.Get("name"),
			Image:        c.Meta.Get("image"),
			State:        c.Meta.Get("state"),
			Health:       c.Meta.Get("health"),
			Host:         c.HostID,
			Created:      c.Meta.Get("created"),
			Uptime:       uptime,
			CPUUtil:      nonNeg(c.CPUUtil),
			MemUsage:     nonNeg(c.MemUsage),
			MemLimit:     nonNeg(c.MemLimit),
			MemPercent:   nonNeg(c.MemPercent),
			MemRss:       nonNeg(c.MemRss),
			MemCache:     nonNeg(c.MemCache),
			NetRx:        nonNeg(c.NetRx),
			NetTx:        nonNeg(c.NetTx),
			NetRxRate:    nonNeg(c.NetRxRate),
			NetTxRate:    nonNeg(c.NetTxRate),
			IOBytesRead:  nonNeg(c.IOBytesRead),
			IOBytesWrite: nonNeg(c.IOBytesWrite),
			IORateRead:   nonNeg(c.IORateRead),
			IORateWrite:  nonNeg(c.IORateWrite),
			Pids:         nonNeg(c.Pids),
			IPs:          c.Meta.Get("IPs"),
			Ports:        c.Meta.Get("ports"),
			WebPort:      c.Meta.Get("Web Port"),
			Command:      c.Meta.Get("cmd"),
			Entrypoint:   c.Meta.Get("entrypoint"),
			WorkDir:      c.Meta.Get("workdir"),
			User:         c.Meta.Get("user"),
			RestartPol:   c.Meta.Get("restartPolicy"),
			MemLimitStr:  c.Meta.Get("memLimit"),
			Env:          parseEnv(c.Meta.Get("[ENV-VAR]")),
			Labels:       parseLabels(c.Meta.Get("[LABELS]")),
			Mounts:       parseMounts(c.Meta.Get("[MOUNTS]")),
			Networks:     parseNetworks(c.Meta.Get("[NETWORKS]")),
			Timestamp:    time.Now().UTC(),
		}
		c.RUnlock()
		list = append(list, snap)
	}
	return list
}

func (p *cursorContainerProvider) GetContainerTop(id string) (web.TopResult, error) {
	if p.cursor == nil || p.cursor.cSuper == nil {
		return web.TopResult{}, fmt.Errorf("connector unavailable")
	}
	cSource, err := p.cursor.cSuper.Get()
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
				_, _ = fmt.Sscanf(fields[4], "%d", &prefix)
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

func startWebServer(addr string, version string, urlPrefix string, cursor *GridCursor) (*web.Server, func(), error) {
	if !strings.Contains(addr, ":") {
		addr = ":" + addr
	}

	prov := &cursorContainerProvider{cursor: cursor}
	broadcaster := web.NewBroadcaster()
	srv := web.NewServer(addr, version, prov, broadcaster, urlPrefix)

	if err := srv.Start(); err != nil {
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
		shutdownCtx, sCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer sCancel()
		_ = srv.Stop(shutdownCtx)
	}

	return srv, cleanup, nil
}
