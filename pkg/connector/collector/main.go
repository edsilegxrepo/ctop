// Package collector implements continuous telemetry and log streaming for container runtimes.
//
// Objective:
//
//	Stream real-time CPU, memory, network, and I/O metrics along with timestamped log entries from underlying container daemons and cgroup pseudo-filesystems.
//
// Core Components:
//   - Collector: Interface providing metric streaming channels, log collector access, and lifecycle control.
//   - LogCollector: Interface providing multiplexed container log streams.
//
// Data Flow:
//
//	Daemon Stats / Cgroups -> Collector -> models.Metrics Channel -> Container Model.
package collector

import (
	"math"

	"github.com/edsilegx/ctop/pkg/logging"
	"github.com/edsilegx/ctop/pkg/models"
)

var log = logging.Init()

// LogCollector streams live container log lines.
type LogCollector interface {
	Stream() chan models.Log
	Stop()
}

// Collector continuously collects and streams container resource utilization metrics.
type Collector interface {
	Stream() chan models.Metrics
	Logs() LogCollector
	Running() bool
	Start()
	Stop()
}

func round(num float64) int {
	return int(num + math.Copysign(0.5, num))
}

// return rounded percentage
func percent(val float64, total float64) int {
	if total <= 0 {
		return 0
	}
	return round((val / total) * 100)
}
