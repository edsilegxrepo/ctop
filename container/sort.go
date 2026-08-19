// sort.go implements comparator functions, sort registries, and filtering logic for container collections.
package container

import (
	"fmt"
	"regexp"
	"sort"

	"github.com/edsilegx/ctop/config"
)

type sortMethod func(c1, c2 *Container) bool

var stateMap = map[string]int{
	"running": 3,
	"paused":  2,
	"exited":  1,
	"created": 0,
	"":        0,
}

var (
	idSorter   = func(c1, c2 *Container) bool { return c1.Id < c2.Id }
	nameSorter = func(c1, c2 *Container) bool { return c1.GetMeta("name") < c2.GetMeta("name") }
)

// Sorters maps configurable column keys (cpu, mem, io, net, pids, state, uptime) to comparator functions.
var Sorters = map[string]sortMethod{
	"id":   idSorter,
	"name": nameSorter,
	"cpu": func(c1, c2 *Container) bool {
		// Use secondary sort method if equal values
		if c1.CPUUtil == c2.CPUUtil {
			return nameSorter(c1, c2)
		}
		return c1.CPUUtil > c2.CPUUtil
	},
	"mem": func(c1, c2 *Container) bool {
		// Use secondary sort method if equal values
		if c1.MemUsage == c2.MemUsage {
			return nameSorter(c1, c2)
		}
		return c1.MemUsage > c2.MemUsage
	},
	"mem %": func(c1, c2 *Container) bool {
		// Use secondary sort method if equal values
		if c1.MemPercent == c2.MemPercent {
			return nameSorter(c1, c2)
		}
		return c1.MemPercent > c2.MemPercent
	},
	"net": func(c1, c2 *Container) bool {
		sum1 := sumNet(c1)
		sum2 := sumNet(c2)
		// Use secondary sort method if equal values
		if sum1 == sum2 {
			return nameSorter(c1, c2)
		}
		return sum1 > sum2
	},
	"pids": func(c1, c2 *Container) bool {
		// Use secondary sort method if equal values
		if c1.Pids == c2.Pids {
			return nameSorter(c1, c2)
		}
		return c1.Pids > c2.Pids
	},
	"io": func(c1, c2 *Container) bool {
		sum1 := sumIO(c1)
		sum2 := sumIO(c2)
		// Use secondary sort method if equal values
		if sum1 == sum2 {
			return nameSorter(c1, c2)
		}
		return sum1 > sum2
	},
	"state": func(c1, c2 *Container) bool {
		// Use secondary sort method if equal values
		c1state := c1.GetMeta("state")
		c2state := c2.GetMeta("state")
		if c1state == c2state {
			return nameSorter(c1, c2)
		}
		return stateMap[c1state] > stateMap[c2state]
	},
	"uptime": func(c1, c2 *Container) bool {
		// Use secondary sort method if equal values
		c1Uptime := c1.GetMeta("uptime")
		c2Uptime := c2.GetMeta("uptime")
		if c1Uptime == c2Uptime {
			return nameSorter(c1, c2)
		}
		return c1Uptime > c2Uptime
	},
	"compose": func(c1, c2 *Container) bool {
		p1 := c1.GetMeta("composeProject")
		p2 := c2.GetMeta("composeProject")
		if p1 == p2 {
			return nameSorter(c1, c2)
		}
		if p1 == "" {
			return false
		}
		if p2 == "" {
			return true
		}
		return p1 < p2
	},
}

func SortFields() (fields []string) {
	for k := range Sorters {
		fields = append(fields, k)
	}
	return fields
}

type Containers []*Container

func (a Containers) Sort()         { sort.Sort(a) }
func (a Containers) Len() int      { return len(a) }
func (a Containers) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a Containers) Less(i, j int) bool {
	if config.GetSwitchVal("groupByCompose") {
		projI := a[i].GetMeta("composeProject")
		projJ := a[j].GetMeta("composeProject")
		if projI != projJ {
			if projI == "" {
				return false
			}
			if projJ == "" {
				return true
			}
			return projI < projJ
		}
	}

	sortField := config.GetVal("sortField")
	f, ok := Sorters[sortField]
	if !ok || f == nil {
		f = Sorters["name"]
	}
	if f == nil {
		return false
	}
	if config.GetSwitchVal("sortReversed") {
		return f(a[j], a[i])
	}
	return f(a[i], a[j])
}

func isActive(state string) bool {
	return state == "running" || state == "paused" || state == "restarting"
}

func (a Containers) Filter() {
	filter := config.GetVal("filterStr")
	re := regexp.MustCompile(fmt.Sprintf(".*%s", filter))

	for _, c := range a {
		name := c.GetMeta("name")
		state := c.GetMeta("state")
		display := true
		// Apply name filter
		if re.FindAllString(name, 1) == nil {
			display = false
		}
		// Apply state filter (active only includes running, paused, and restarting)
		if !config.GetSwitchVal("allContainers") && !isActive(state) {
			display = false
		}
		c.mu.Lock()
		c.Display = display
		c.mu.Unlock()
	}
}

func sumNet(c *Container) int64 { return c.NetRx + c.NetTx }

func sumIO(c *Container) int64 { return c.IOBytesRead + c.IOBytesWrite }
