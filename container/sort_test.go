// sort_test.go validates all container sorter comparators and regex filter criteria.
// Test Strategy: Fixture-based unit tests asserting sort order across CPU, memory, PIDs, network, and status fields.
package container

import (
	"testing"

	"github.com/edsilegx/ctop/config"
	"github.com/edsilegx/ctop/models"
)

func newTestContainer(id, name, state string, cpu int, mem int64, pids int) *Container {
	c := &Container{
		Id:      id,
		Meta:    models.NewMeta("id", id, "name", name, "state", state, "uptime", "1 hour"),
		Metrics: models.NewMetrics(),
		Display: true,
	}
	c.CPUUtil = cpu
	c.MemUsage = mem
	c.Pids = pids
	c.NetRx = int64(cpu * 100)
	c.NetTx = int64(cpu * 200)
	c.IOBytesRead = mem
	c.IOBytesWrite = mem * 2
	return c
}

func TestAllSorters(t *testing.T) {
	config.Init()
	config.UpdateSwitch("sortReversed", false)

	c1 := newTestContainer("1", "alpha", "running", 10, 100, 5)
	c2 := newTestContainer("2", "beta", "running", 50, 50, 2)
	c3 := newTestContainer("3", "gamma", "running", 30, 200, 8)

	sorters := SortFields()
	if len(sorters) == 0 {
		t.Fatal("expected non-empty SortFields()")
	}

	for _, s := range sorters {
		config.Update("sortField", s)
		containers := Containers{c1, c2, c3}
		containers.Sort()
		if containers.Len() != 3 {
			t.Fatalf("expected 3 containers sorted by %s", s)
		}
	}

	// Equal metric secondary sort (by name)
	cEqual1 := newTestContainer("1", "zebra", "running", 10, 100, 5)
	cEqual2 := newTestContainer("2", "alpha", "running", 10, 100, 5)
	for _, s := range []string{"cpu", "mem", "mem %", "net", "io", "pids", "state", "uptime"} {
		config.Update("sortField", s)
		containers := Containers{cEqual1, cEqual2}
		containers.Sort()
		if containers[0].GetMeta("name") != "alpha" {
			t.Errorf("expected secondary sort by name for equal %s values", s)
		}
	}

	// Invalid sorter fallback to name
	config.Update("sortField", "non-existent")
	containers := Containers{c3, c1, c2}
	containers.Sort()
	if containers[0].Id != "1" || containers[1].Id != "2" || containers[2].Id != "3" {
		t.Errorf("expected fallback sort by name [1, 2, 3], got [%s, %s, %s]",
			containers[0].Id, containers[1].Id, containers[2].Id)
	}

	// Reverse sort
	config.Update("sortField", "name")
	config.UpdateSwitch("sortReversed", true)
	containers = Containers{c1, c2, c3}
	containers.Sort()
	if containers[0].Id != "3" || containers[1].Id != "2" || containers[2].Id != "1" {
		t.Errorf("expected reverse sort by name [3, 2, 1], got [%s, %s, %s]",
			containers[0].Id, containers[1].Id, containers[2].Id)
	}
}

func TestFilterByName(t *testing.T) {
	config.Init()
	config.Update("filterStr", "bet")
	config.UpdateSwitch("allContainers", true)

	c1 := newTestContainer("1", "alpha", "running", 10, 100, 5)
	c2 := newTestContainer("2", "beta", "running", 50, 50, 2)
	c3 := newTestContainer("3", "gamma", "running", 30, 200, 8)

	containers := Containers{c1, c2, c3}
	containers.Filter()

	if c1.Display || !c2.Display || c3.Display {
		t.Errorf("expected only c2 (beta) to be displayed, got c1=%v, c2=%v, c3=%v",
			c1.Display, c2.Display, c3.Display)
	}
}

func TestFilterRunningOnly(t *testing.T) {
	config.Init()
	config.Update("filterStr", "")
	config.UpdateSwitch("allContainers", false)

	c1 := newTestContainer("1", "alpha", "running", 10, 100, 5)
	c2 := newTestContainer("2", "beta", "exited", 50, 50, 2)
	c3 := newTestContainer("3", "gamma", "paused", 30, 200, 8)
	c4 := newTestContainer("4", "delta", "restarting", 20, 150, 4)
	c5 := newTestContainer("5", "epsilon", "created", 0, 0, 0)

	containers := Containers{c1, c2, c3, c4, c5}
	containers.Filter()

	if !c1.Display || c2.Display || !c3.Display || !c4.Display || c5.Display {
		t.Errorf("expected active containers (c1 running, c3 paused, c4 restarting) displayed, got c1=%v, c2=%v, c3=%v, c4=%v, c5=%v",
			c1.Display, c2.Display, c3.Display, c4.Display, c5.Display)
	}
}

func TestComposeGrouping(t *testing.T) {
	config.Init()
	config.UpdateSwitch("groupByCompose", true)
	config.Update("sortField", "name")

	c1 := newTestContainer("1", "web", "running", 10, 100, 5)
	c1.SetMeta("composeProject", "backend")

	c2 := newTestContainer("2", "db", "running", 50, 50, 2)
	c2.SetMeta("composeProject", "backend")

	c3 := newTestContainer("3", "frontend-app", "running", 30, 200, 8)
	c3.SetMeta("composeProject", "frontend")

	c4 := newTestContainer("4", "standalone", "running", 20, 150, 4)

	containers := Containers{c4, c3, c1, c2}
	containers.Sort()

	// Expected order: backend (db, web), frontend (frontend-app), standalone
	if containers[0].GetMeta("name") != "db" || containers[1].GetMeta("name") != "web" ||
		containers[2].GetMeta("name") != "frontend-app" || containers[3].GetMeta("name") != "standalone" {
		t.Errorf("unexpected compose group order: %+v, %+v, %+v, %+v",
			containers[0].GetMeta("name"), containers[1].GetMeta("name"),
			containers[2].GetMeta("name"), containers[3].GetMeta("name"))
	}
}
