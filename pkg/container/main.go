// Package container models active container entities, their telemetry channels, lifecycle controls, and visual row widgets.
// Objective: Bridge raw metrics collectors and managers with UI widget updaters and filtering/sorting logic.
// Data Flow: Collector Stream -> Container.Read() -> Container.Metrics -> CompactRow / SingleView Updaters.
package container

import (
	"sync"

	"github.com/edsilegx/ctop/internal/cwidgets"
	"github.com/edsilegx/ctop/internal/cwidgets/compact"
	"github.com/edsilegx/ctop/pkg/connector/collector"
	"github.com/edsilegx/ctop/pkg/connector/manager"
	"github.com/edsilegx/ctop/pkg/generator"
	"github.com/edsilegx/ctop/pkg/logging"
	"github.com/edsilegx/ctop/pkg/models"
)

var log = logging.Init()

const (
	running = "running"
)

// Container encapsulates metadata, live telemetry metrics, lifecycle control methods, and visual widgets for a container.
type Container struct {
	models.Metrics
	Id        string
	HostID    string
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
	if k == "host" {
		c.HostID = v
	}
	if c.updater != nil {
		c.updater.SetMeta(c.Meta)
	}
}

func (c *Container) SetHost(h string) {
	c.SetMeta("host", h)
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

func (c *Container) SearchFiles(basePath, pattern string, maxResults int) ([]models.FileInfo, error) {
	return c.manager.SearchFiles(basePath, pattern, maxResults)
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
	return generator.GenerateRunCmd(c.Meta)
}

func (c *Container) GenerateCompose() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return generator.GenerateCompose(c.Meta)
}

// RLock acquires the read lock on the container.
func (c *Container) RLock() {
	c.mu.RLock()
}

// RUnlock releases the read lock on the container.
func (c *Container) RUnlock() {
	c.mu.RUnlock()
}
