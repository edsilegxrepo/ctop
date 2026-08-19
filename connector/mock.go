//go:build !release

package connector

import (
	// nosemgrep: go.lang.security.audit.crypto.math_random.math-random-used - pseudo-random generator is intended for non-security mock containers simulation
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/edsilegx/ctop/connector/collector"
	"github.com/edsilegx/ctop/connector/manager"
	"github.com/edsilegx/ctop/container"
	"github.com/jgautheron/codename-generator"
	"github.com/nu7hatch/gouuid"
)

func init() { enabled["mock"] = NewMock }

type Mock struct {
	containers container.Containers
	lock       sync.RWMutex
}

func NewMock() (Connector, error) {
	cs := &Mock{}
	cs.Init()
	go cs.Loop()
	return cs, nil
}

// Create Mock containers
func (cs *Mock) Init() {
	// #nosec G404 - weak pseudo-random generator is sufficient for non-security mock containers simulation
	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	for i := 0; i < 4; i++ {
		cs.makeContainer(r, 3, true)
	}

	for i := 0; i < 16; i++ {
		cs.makeContainer(r, 1, false)
	}
}

func (cs *Mock) Wait() struct{} {
	ch := make(chan struct{})
	go func() {
		time.Sleep(30 * time.Second)
		close(ch)
	}()
	return <-ch
}

var healthStates = []string{"starting", "healthy", "unhealthy"}

func (cs *Mock) makeContainer(r *rand.Rand, aggression int64, health bool) {
	collector := collector.NewMock(aggression)
	manager := manager.NewMock()
	c := container.New(makeID(), collector, manager)
	c.SetMeta("name", makeName())
	c.SetState(makeState(r))
	if health {
		var i int
		c.SetMeta("health", healthStates[i])
		go func() {
			for {
				i++
				if i >= len(healthStates) {
					i = 0
				}
				c.SetMeta("health", healthStates[i])
				time.Sleep(12 * time.Second)
			}
		}()
	}
	cs.lock.Lock()
	cs.containers = append(cs.containers, c)
	cs.lock.Unlock()
}

func (cs *Mock) Loop() {
	// #nosec G404 - weak pseudo-random generator is sufficient for non-security mock containers simulation
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	iter := 0
	for {
		// Change state for random container
		cs.lock.RLock()
		nContainers := len(cs.containers)
		var randC *container.Container
		if iter%5 == 0 && nContainers > 0 {
			randC = cs.containers[r.Intn(nContainers)]
		}
		cs.lock.RUnlock()

		if randC != nil {
			randC.SetState(makeState(r))
		}
		iter++
		time.Sleep(3 * time.Second)
	}
}

// Get a single container, by ID
func (cs *Mock) Get(id string) (*container.Container, bool) {
	cs.lock.RLock()
	defer cs.lock.RUnlock()
	for _, c := range cs.containers {
		if c.Id == id {
			return c, true
		}
	}
	return nil, false
}

// All returns array of all containers, sorted by field
func (cs *Mock) All() container.Containers {
	cs.lock.RLock()
	containers := make(container.Containers, len(cs.containers))
	copy(containers, cs.containers)
	cs.lock.RUnlock()

	containers.Sort()
	containers.Filter()
	return containers
}

func makeID() string {
	u, err := uuid.NewV4()
	if err != nil {
		panic(err)
	}
	return strings.ReplaceAll(u.String(), "-", "")[:12]
}

func makeName() string {
	n, err := codename.Get(codename.Sanitized)
	nsp := strings.Split(n, "-")
	if len(nsp) > 2 {
		n = strings.Join(nsp[:2], "-")
	}
	if err != nil {
		panic(err)
	}
	return strings.ReplaceAll(n, "-", "_")
}

func makeState(r *rand.Rand) string {
	switch r.Intn(10) {
	case 0, 1, 2:
		return "exited"
	case 3:
		return "paused"
	}
	return "running"
}
