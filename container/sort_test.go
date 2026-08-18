package container

import (
	"testing"

	"github.com/bcicen/ctop/config"
	"github.com/bcicen/ctop/models"
)

func newTestContainer(id, name, state string, cpu int, mem int64, pids int) *Container {
	c := &Container{
		Id:      id,
		Meta:    models.NewMeta("id", id, "name", name, "state", state),
		Metrics: models.NewMetrics(),
		Display: true,
	}
	c.CPUUtil = cpu
	c.MemUsage = mem
	c.Pids = pids
	return c
}

func TestSortByCPU(t *testing.T) {
	config.Init()
	config.Update("sortField", "cpu")
	config.UpdateSwitch("sortReversed", false)

	c1 := newTestContainer("1", "alpha", "running", 10, 100, 5)
	c2 := newTestContainer("2", "beta", "running", 50, 50, 2)
	c3 := newTestContainer("3", "gamma", "running", 30, 200, 8)

	containers := Containers{c1, c2, c3}
	containers.Sort()

	if containers[0].Id != "2" || containers[1].Id != "3" || containers[2].Id != "1" {
		t.Errorf("expected CPU sort order [2, 3, 1], got [%s, %s, %s]",
			containers[0].Id, containers[1].Id, containers[2].Id)
	}
}

func TestSortByMem(t *testing.T) {
	config.Init()
	config.Update("sortField", "mem")
	config.UpdateSwitch("sortReversed", false)

	c1 := newTestContainer("1", "alpha", "running", 10, 100, 5)
	c2 := newTestContainer("2", "beta", "running", 50, 50, 2)
	c3 := newTestContainer("3", "gamma", "running", 30, 200, 8)

	containers := Containers{c1, c2, c3}
	containers.Sort()

	if containers[0].Id != "3" || containers[1].Id != "1" || containers[2].Id != "2" {
		t.Errorf("expected Mem sort order [3, 1, 2], got [%s, %s, %s]",
			containers[0].Id, containers[1].Id, containers[2].Id)
	}
}

func TestSortReversed(t *testing.T) {
	config.Init()
	config.Update("sortField", "cpu")
	config.UpdateSwitch("sortReversed", true)

	c1 := newTestContainer("1", "alpha", "running", 10, 100, 5)
	c2 := newTestContainer("2", "beta", "running", 50, 50, 2)
	c3 := newTestContainer("3", "gamma", "running", 30, 200, 8)

	containers := Containers{c1, c2, c3}
	containers.Sort()

	if containers[0].Id != "1" || containers[1].Id != "3" || containers[2].Id != "2" {
		t.Errorf("expected reversed CPU sort order [1, 3, 2], got [%s, %s, %s]",
			containers[0].Id, containers[1].Id, containers[2].Id)
	}
}

func TestFilterByName(t *testing.T) {
	config.Init()
	config.Update("filterStr", "bet")
	config.UpdateSwitch("allContainers", true)

	c1 := newTestContainer("1", "alpha", "running", 10, 100, 5)
	c2 := newTestContainer("2", "beta", "running", 50, 50, 2)

	containers := Containers{c1, c2}
	containers.Filter()

	if c1.Display {
		t.Errorf("expected c1 ('alpha') to not be displayed with filter 'bet'")
	}
	if !c2.Display {
		t.Errorf("expected c2 ('beta') to be displayed with filter 'bet'")
	}
}

func TestFilterRunningOnly(t *testing.T) {
	config.Init()
	config.Update("filterStr", "")
	config.UpdateSwitch("allContainers", false) // only running

	c1 := newTestContainer("1", "alpha", "running", 10, 100, 5)
	c2 := newTestContainer("2", "beta", "exited", 50, 50, 2)

	containers := Containers{c1, c2}
	containers.Filter()

	if !c1.Display {
		t.Errorf("expected c1 ('running') to be displayed")
	}
	if c2.Display {
		t.Errorf("expected c2 ('exited') to not be displayed when allContainers=false")
	}
}
