// Package cwidgets defines UI update contracts and formatting helpers for container widgets.
//
// Objective:
//
//	Standardize property passing and telemetry bindings from container models to visual TermUI widgets.
//
// Core Components:
//   - WidgetUpdater: Interface implemented by widgets consuming container metadata and telemetry metrics.
//   - NullWidgetUpdater: Null-object pattern implementation ensuring safe transition states without nil checks.
//
// Data Flow:
//
//	Container Telemetry -> WidgetUpdater.SetMetrics() / SetMeta() -> Visual TermUI State.
package cwidgets

import (
	"github.com/edsilegx/ctop/pkg/models"
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
