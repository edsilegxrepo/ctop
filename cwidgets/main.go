// Package cwidgets defines UI update contracts and formatting helpers for container widgets.
// Objective: Standardize property passing from container telemetry models to visual TermUI widgets.
package cwidgets

import (
	"github.com/edsilegx/ctop/models"
)

// WidgetUpdater defines the interface for UI components that consume container metadata and metrics.
type WidgetUpdater interface {
	SetMeta(models.Meta)
	SetMetrics(models.Metrics)
}

// NullWidgetUpdater provides a no-op implementation of WidgetUpdater used during transitions.
type NullWidgetUpdater struct{}

// NullWidgetUpdater implements WidgetUpdater
func (wu NullWidgetUpdater) SetMeta(models.Meta) {}

// NullWidgetUpdater implements WidgetUpdater
func (wu NullWidgetUpdater) SetMetrics(models.Metrics) {}
