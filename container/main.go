// Package container models active container entities, their telemetry channels, lifecycle controls, and visual row widgets.
// Objective: Bridge raw metrics collectors and managers with UI widget updaters and filtering/sorting logic.
// Data Flow: Collector Stream -> Container.Read() -> Container.Metrics -> CompactRow / SingleView Updaters.
package container

import (
	"fmt"
	"strings"
	"sync"

	"github.com/edsilegx/ctop/connector/collector"
	"github.com/edsilegx/ctop/connector/manager"
	"github.com/edsilegx/ctop/cwidgets"
	"github.com/edsilegx/ctop/cwidgets/compact"
	"github.com/edsilegx/ctop/logging"
	"github.com/edsilegx/ctop/models"
)

var log = logging.Init()

const (
	running = "running"
)

// Container encapsulates metadata, live telemetry metrics, lifecycle control methods, and visual widgets for a container.
type Container struct {
	models.Metrics
	Id        string
	Meta      models.Meta
	Widgets   *compact.CompactRow
	Display   bool // display this container in compact view
	updater   cwidgets.WidgetUpdater
	collector collector.Collector
	manager   manager.Manager
	mu        sync.RWMutex
}

func New(id string, collector collector.Collector, manager manager.Manager) *Container {
	widgets := compact.NewCompactRow()
	shortID := id
	if len(shortID) > 12 {
		shortID = shortID[0:12]
	}
	return &Container{
		Metrics:   models.NewMetrics(),
		Id:        id,
		Meta:      models.NewMeta("id", shortID),
		Widgets:   widgets,
		updater:   widgets,
		collector: collector,
		manager:   manager,
	}
}

func (c *Container) RecreateWidgets() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.updater = cwidgets.NullWidgetUpdater{}
	c.Widgets = compact.NewCompactRow()
	c.updater = c.Widgets
	c.updater.SetMeta(c.Meta)
	c.updater.SetMetrics(c.Metrics)
}

func (c *Container) SetUpdater(u cwidgets.WidgetUpdater) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.updater = u
	c.updater.SetMeta(c.Meta)
	c.updater.SetMetrics(c.Metrics)
}

func (c *Container) SetMeta(k, v string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Meta[k] = v
	if c.updater != nil {
		c.updater.SetMeta(c.Meta)
	}
}

func (c *Container) GetMeta(k string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Meta.Get(k)
}

func (c *Container) SetState(s string) {
	c.SetMeta("state", s)
	// start collector, if needed
	if s == running && !c.collector.Running() {
		c.collector.Start()
		c.Read(c.collector.Stream())
	}
	// stop collector, if needed
	if s != running && c.collector.Running() {
		c.collector.Stop()
	}
}

// Logs returns container log collector
func (c *Container) Logs() collector.LogCollector {
	return c.collector.Logs()
}

// Read metric stream, updating widgets
func (c *Container) Read(stream chan models.Metrics) {
	go func() {
		for metrics := range stream {
			c.mu.Lock()
			c.Metrics = metrics
			if c.updater != nil {
				c.updater.SetMetrics(metrics)
			}
			c.mu.Unlock()
		}
		log.Infof("reader stopped for container: %s", c.Id)
		c.mu.Lock()
		c.Metrics = models.NewMetrics()
		if c.Widgets != nil {
			c.Widgets.Reset()
		}
		c.mu.Unlock()
	}()
	log.Infof("reader started for container: %s", c.Id)
}

func (c *Container) Start() {
	if c.GetMeta("state") != running {
		if err := c.manager.Start(); err != nil {
			log.Warningf("container %s: %v", c.Id, err)
			log.StatusErr(err)
			return
		}
		c.SetState(running)
	}
}

func (c *Container) Stop() {
	if c.GetMeta("state") == running {
		if err := c.manager.Stop(); err != nil {
			log.Warningf("container %s: %v", c.Id, err)
			log.StatusErr(err)
			return
		}
		c.SetState("exited")
	}
}

func (c *Container) Remove() {
	if err := c.manager.Remove(); err != nil {
		log.Warningf("container %s: %v", c.Id, err)
		log.StatusErr(err)
	}
}

func (c *Container) Pause() {
	if c.GetMeta("state") == running {
		if err := c.manager.Pause(); err != nil {
			log.Warningf("container %s: %v", c.Id, err)
			log.StatusErr(err)
			return
		}
		c.SetState("paused")
	}
}

func (c *Container) Unpause() {
	if c.GetMeta("state") == "paused" {
		if err := c.manager.Unpause(); err != nil {
			log.Warningf("container %s: %v", c.Id, err)
			log.StatusErr(err)
			return
		}
		c.SetState(running)
	}
}

func (c *Container) Restart() {
	if c.GetMeta("state") == running {
		if err := c.manager.Restart(); err != nil {
			log.Warningf("container %s: %v", c.Id, err)
			log.StatusErr(err)
			return
		}
	}
}

func (c *Container) Exec(cmd []string) error {
	return c.manager.Exec(cmd)
}

func (c *Container) Signal(sig string) error {
	return c.manager.Kill(sig)
}

func (c *Container) Top() (models.TopResult, error) {
	return c.manager.Top("aux")
}

func (c *Container) Changes() ([]models.Change, error) {
	return c.manager.Changes()
}

func (c *Container) ReadDir(path string) ([]models.FileInfo, error) {
	return c.manager.ReadDir(path)
}

func (c *Container) ReadFile(path string, maxBytes int64) (string, error) {
	return c.manager.ReadFile(path, maxBytes)
}

func (c *Container) Download(srcPath, dstPath string) (int64, error) {
	return c.manager.Download(srcPath, dstPath)
}

func (c *Container) Upload(srcHostPath, dstContainerPath string) error {
	return c.manager.Upload(srcHostPath, dstContainerPath)
}

func (c *Container) UpdateResources(memoryMB int64, cpus float64, restartPolicy string) error {
	return c.manager.UpdateResources(memoryMB, cpus, restartPolicy)
}



func (c *Container) GenerateRunCmd() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var sb strings.Builder
	sb.WriteString("docker run -d")

	name := c.Meta.Get("name")
	if name != "" {
		sb.WriteString(fmt.Sprintf(" \\\n  --name %s", name))
	}

	restart := c.Meta.Get("restartPolicy")
	if restart != "" && restart != "no" {
		sb.WriteString(fmt.Sprintf(" \\\n  --restart %s", restart))
	}

	if ports := c.Meta.Get("ports"); ports != "" {
		for _, line := range strings.Split(ports, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if strings.Contains(line, " -> ") {
				parts := strings.Split(line, " -> ")
				sb.WriteString(fmt.Sprintf(" \\\n  -p %s:%s", parts[0], parts[1]))
			} else {
				sb.WriteString(fmt.Sprintf(" \\\n  --expose %s", line))
			}
		}
	}

	if mounts := c.Meta.Get("[MOUNTS]"); mounts != "" {
		for _, m := range strings.Split(mounts, ";;") {
			m = strings.TrimSpace(m)
			if m == "" {
				continue
			}
			parts := strings.Split(m, ":::")
			if len(parts) >= 4 {
				dest, src, mode := parts[0], parts[1], parts[3]
				sb.WriteString(fmt.Sprintf(" \\\n  -v %s:%s:%s", src, dest, mode))
			}
		}
	}

	if env := c.Meta.Get("[ENV-VAR]"); env != "" {
		for _, e := range strings.Split(env, ";") {
			e = strings.TrimSpace(e)
			if e == "" {
				continue
			}
			sb.WriteString(fmt.Sprintf(" \\\n  -e %q", e))
		}
	}

	if mem := c.Meta.Get("memLimit"); mem != "" {
		sb.WriteString(fmt.Sprintf(" \\\n  --memory %s", strings.ReplaceAll(mem, " ", "")))
	}
	if cpu := c.Meta.Get("cpuLimit"); cpu != "" {
		sb.WriteString(fmt.Sprintf(" \\\n  --cpus %s", strings.Fields(cpu)[0]))
	}
	if pids := c.Meta.Get("pidsLimit"); pids != "" {
		sb.WriteString(fmt.Sprintf(" \\\n  --pids-limit %s", pids))
	}
	if c.Meta.Get("privileged") == "true" {
		sb.WriteString(" \\\n  --privileged")
	}
	if c.Meta.Get("readonlyRootfs") == "true" {
		sb.WriteString(" \\\n  --read-only")
	}

	image := c.Meta.Get("image")
	if image == "" {
		image = "unknown:latest"
	}
	sb.WriteString(fmt.Sprintf(" \\\n  %s", image))

	if cmd := c.Meta.Get("cmd"); cmd != "" {
		sb.WriteString(fmt.Sprintf(" %s", cmd))
	}

	return sb.String()
}

func (c *Container) GenerateCompose() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	name := c.Meta.Get("name")
	if name == "" {
		name = "app"
	}
	image := c.Meta.Get("image")
	if image == "" {
		image = "unknown:latest"
	}

	var sb strings.Builder
	sb.WriteString("version: '3.8'\n\nservices:\n")
	sb.WriteString(fmt.Sprintf("  %s:\n", name))
	sb.WriteString(fmt.Sprintf("    image: %s\n", image))
	sb.WriteString(fmt.Sprintf("    container_name: %s\n", name))

	if restart := c.Meta.Get("restartPolicy"); restart != "" && restart != "no" {
		sb.WriteString(fmt.Sprintf("    restart: %s\n", restart))
	}

	if ports := c.Meta.Get("ports"); ports != "" {
		sb.WriteString("    ports:\n")
		for _, line := range strings.Split(ports, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if strings.Contains(line, " -> ") {
				parts := strings.Split(line, " -> ")
				sb.WriteString(fmt.Sprintf("      - \"%s:%s\"\n", parts[0], parts[1]))
			} else {
				sb.WriteString(fmt.Sprintf("      - \"%s\"\n", line))
			}
		}
	}

	if mounts := c.Meta.Get("[MOUNTS]"); mounts != "" {
		sb.WriteString("    volumes:\n")
		for _, m := range strings.Split(mounts, ";;") {
			m = strings.TrimSpace(m)
			if m == "" {
				continue
			}
			parts := strings.Split(m, ":::")
			if len(parts) >= 4 {
				dest, src, mode := parts[0], parts[1], parts[3]
				sb.WriteString(fmt.Sprintf("      - %s:%s:%s\n", src, dest, mode))
			}
		}
	}

	if env := c.Meta.Get("[ENV-VAR]"); env != "" {
		sb.WriteString("    environment:\n")
		for _, e := range strings.Split(env, ";") {
			e = strings.TrimSpace(e)
			if e == "" {
				continue
			}
			sb.WriteString(fmt.Sprintf("      - %s\n", e))
		}
	}

	if c.Meta.Get("privileged") == "true" {
		sb.WriteString("    privileged: true\n")
	}
	if c.Meta.Get("readonlyRootfs") == "true" {
		sb.WriteString("    read_only: true\n")
	}

	if cmd := c.Meta.Get("cmd"); cmd != "" {
		sb.WriteString(fmt.Sprintf("    command: %s\n", cmd))
	}

	return sb.String()
}

