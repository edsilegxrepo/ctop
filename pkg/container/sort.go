// sort.go implements comparator functions, sort registries, and filtering logic for container collections.
//
// Objective:
//
//	Provide multi-field sorting algorithms (CPU, Memory, Network, I/O, PIDs, State, Uptime) and rich structured filtering.
//
// Functionality:
//   - Sorters: Map of comparator functions with state-priority tie-breaking.
//   - Filter(): Multi-field expression evaluator supporting regex, wildcard, state, name, label, and environment queries.
//   - GroupByCompose(): Organizes containers by docker-compose project/service metadata.
package container

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/edsilegx/ctop/pkg/config"
)

type sortMethod func(c1, c2 *Container) bool

func getStateScore(s string) int {
	s = strings.ToLower(strings.TrimSpace(s))
	if strings.HasPrefix(s, "up") || strings.Contains(s, "running") {
		return 4
	}
	if strings.Contains(s, "restart") {
		return 3
	}
	if strings.Contains(s, "pause") {
		return 2
	}
	if strings.Contains(s, "exit") || strings.Contains(s, "stop") || strings.Contains(s, "dead") {
		return 1
	}
	return 0
}

var (
	idSorter   = func(c1, c2 *Container) bool { return strings.ToLower(c1.Id) < strings.ToLower(c2.Id) }
	nameSorter = func(c1, c2 *Container) bool {
		return strings.ToLower(c1.GetMeta("name")) < strings.ToLower(c2.GetMeta("name"))
	}
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
		s1 := getStateScore(c1.GetMeta("state"))
		s2 := getStateScore(c2.GetMeta("state"))
		if s1 == s2 {
			name1 := strings.ToLower(c1.GetMeta("name"))
			name2 := strings.ToLower(c2.GetMeta("name"))
			if name1 == name2 {
				return strings.ToLower(c1.Id) < strings.ToLower(c2.Id)
			}
			return name1 < name2
		}
		return s1 > s2
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
		return strings.ToLower(p1) < strings.ToLower(p2)
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
	sortField := config.GetVal("sortField")
	if sortField == "" {
		sortField = "state"
	}

	// Group by compose ONLY if sortField == "compose" or if groupByCompose switch is enabled (and not sorting by state)
	if sortField == "compose" || (config.GetSwitchVal("groupByCompose") && sortField != "state") {
		projI := a[i].GetMeta("composeProject")
		projJ := a[j].GetMeta("composeProject")
		if projI != projJ {
			if projI == "" {
				return false
			}
			if projJ == "" {
				return true
			}
			return strings.ToLower(projI) < strings.ToLower(projJ)
		}
	}

	if sortField == "state" {
		// Default state sort: strictly Up first -> Pause -> Down, with alphabetical A-Z
		return Sorters["state"](a[i], a[j])
	}

	f, ok := Sorters[sortField]
	if !ok || f == nil {
		f = Sorters["name"]
	}

	if config.GetSwitchVal("sortReversed") {
		return f(a[j], a[i])
	}
	return f(a[i], a[j])
}

func isActive(state string) bool {
	return state == "running" || state == "paused" || state == "restarting"
}

func matchesStructuredFilter(c *Container, filter string) bool {
	if filter == "" {
		return true
	}

	tokens := strings.Fields(filter)
	for _, token := range tokens {
		if strings.Contains(token, "=") {
			k, v, _ := strings.Cut(token, "=")
			k = strings.ToLower(strings.TrimSpace(k))
			v = strings.ToLower(strings.TrimSpace(v))

			switch k {
			case "status", "state":
				if strings.ToLower(c.GetMeta("state")) != v {
					return false
				}
			case "health":
				if strings.ToLower(c.GetMeta("health")) != v {
					return false
				}
			case "name":
				if !strings.Contains(strings.ToLower(c.GetMeta("name")), v) {
					return false
				}
			case "id":
				if !strings.Contains(strings.ToLower(c.Id), v) {
					return false
				}
			case "image", "ancestor":
				if !strings.Contains(strings.ToLower(c.GetMeta("image")), v) {
					return false
				}
			case "compose", "project":
				if !strings.Contains(strings.ToLower(c.GetMeta("composeProject")), v) {
					return false
				}
			default:
				metaVal := strings.ToLower(c.GetMeta(k))
				labelsVal := strings.ToLower(c.GetMeta("[LABELS]"))
				if v != "" {
					labelTarget := fmt.Sprintf("%s=%s", k, v)
					if !strings.Contains(metaVal, v) && !strings.Contains(labelsVal, labelTarget) && !strings.Contains(labelsVal, v) {
						return false
					}
				} else if metaVal == "" && !strings.Contains(labelsVal, k) {
					return false
				}
			}
		} else {
			pattern := fmt.Sprintf("(?i)%s", token)
			re, err := regexp.Compile(pattern)
			if err != nil {
				re = regexp.MustCompile(fmt.Sprintf("(?i)%s", regexp.QuoteMeta(token)))
			}
			if !re.MatchString(c.GetMeta("name")) && !re.MatchString(c.Id) {
				return false
			}
		}
	}
	return true
}

func (a Containers) Filter() {
	filter := config.GetVal("filterStr")

	for _, c := range a {
		state := c.GetMeta("state")
		display := true
		// Apply structured and substring filter
		if !matchesStructuredFilter(c, filter) {
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
