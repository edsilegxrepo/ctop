// Package config provides thread-safe runtime configuration, default options, parameter toggling, and column layout management.
// Objective: Manage in-memory configuration state with TOML persistence and environment variable overrides.
// Data Flow: Default Parameters -> TOML File / CLI Overrides -> GlobalParams/GlobalSwitches -> UI Consumers.
package config

import (
	"fmt"
	"sync"

	"github.com/edsilegx/ctop/logging"
)

var (
	GlobalParams   []*Param     // String/integer configuration parameters (e.g. filterStr, sortField)
	GlobalSwitches []*Switch    // Boolean configuration switches (e.g. allContainers, sortReversed)
	GlobalColumns  []*Column    // Column layout definitions and visibility states
	lock           sync.RWMutex // Mutex protecting global configuration access
	log            = logging.Init()
)

// Init resets and initializes global parameters, switches, and columns to default values.
func Init() {
	lock.Lock()
	defer lock.Unlock()

	GlobalParams = nil
	GlobalSwitches = nil
	GlobalColumns = nil

	for _, p := range defaultParams {
		pm := *p
		GlobalParams = append(GlobalParams, &pm)
		log.Infof("loaded default config param [%s]: %s", quote(pm.Key), quote(pm.Val))
	}
	for _, s := range defaultSwitches {
		sw := *s
		GlobalSwitches = append(GlobalSwitches, &sw)
		log.Infof("loaded default config switch [%s]: %t", quote(sw.Key), sw.Val)
	}
	for _, c := range defaultColumns {
		x := c
		GlobalColumns = append(GlobalColumns, &x)
		log.Infof("loaded default widget config [%s]: %t", quote(x.Name), x.Enabled)
	}
}

func quote(s string) string {
	return fmt.Sprintf("\"%s\"", s)
}
