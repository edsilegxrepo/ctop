// Package container models active container entities, their telemetry channels, lifecycle controls, and visual row widgets.
// Objective: Bridge raw metrics collectors and managers with UI widget updaters and filtering/sorting logic.
// Data Flow: Collector Stream -> Container.Read() -> Container.Metrics -> CompactRow / SingleView Updaters.
package container

import (
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
