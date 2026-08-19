// docker.go implements the Connector interface for Docker daemons via go-dockerclient.
package connector

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/hako/durafmt"
	"github.com/op/go-logging"

	"github.com/edsilegx/ctop/connector/collector"
	"github.com/edsilegx/ctop/connector/manager"
	"github.com/edsilegx/ctop/container"
	api "github.com/fsouza/go-dockerclient"
)

func init() { enabled["docker"] = NewDocker }

var actionToStatus = map[string]string{
	"start":   "running",
	"die":     "exited",
	"stop":    "exited",
	"pause":   "paused",
	"unpause": "running",
}

// StatusUpdate carries asynchronous container lifecycle or health updates from the Docker event stream.
type StatusUpdate struct {
	Cid    string
	Field  string // "status" or "health"
	Status string
}

// Docker implements Connector for Docker engines, caching active containers and streaming events.
type Docker struct {
	client       *api.Client
	containers   map[string]*container.Container
	needsRefresh chan string // container IDs requiring refresh
	statuses     chan StatusUpdate
	closed       chan struct{}
	lock         sync.RWMutex
}

func NewDocker() (Connector, error) {
	// init docker client
	client, err := api.NewClientFromEnv()
	if err != nil {
		return nil, err
	}
	cm := &Docker{
		client:       client,
		containers:   make(map[string]*container.Container),
		needsRefresh: make(chan string, 60),
		statuses:     make(chan StatusUpdate, 60),
		closed:       make(chan struct{}),
		lock:         sync.RWMutex{},
	}

	// query info as pre-flight healthcheck
	info, err := client.Info()
	if err != nil {
		return nil, err
	}

	log.Debugf("docker-connector ID: %s", info.ID)
	log.Debugf("docker-connector Driver: %s", info.Driver)
	log.Debugf("docker-connector Images: %d", info.Images)
	log.Debugf("docker-connector Name: %s", info.Name)
	log.Debugf("docker-connector ServerVersion: %s", info.ServerVersion)

	go cm.Loop()
	go cm.LoopStatuses()
	cm.refreshAll()
	go cm.watchEvents()
	return cm, nil
}

// Docker implements Connector
func (cm *Docker) Wait() struct{} { return <-cm.closed }

// Docker events watcher
func (cm *Docker) watchEvents() {
	log.Info("docker event listener starting")
	events := make(chan *api.APIEvents)
	opts := api.EventsOptions{
		Filters: map[string][]string{
			"type":  {"container"},
			"event": {"create", "start", "health_status", "pause", "unpause", "stop", "die", "destroy"},
		},
	}
	if err := cm.client.AddEventListenerWithOptions(opts, events); err != nil {
		log.Errorf("failed to add docker event listener: %s", err)
	}

	for e := range events {
		actionName := e.Action
		switch actionName {
		// most frequent event is a health checks
		case "health_status: healthy", "health_status: unhealthy":
			sepIdx := strings.Index(actionName, ": ")
			healthStatus := e.Action[sepIdx+2:]
			if log.IsEnabledFor(logging.DEBUG) {
				log.Debugf("handling docker event: action=health_status id=%s %s", e.ID, healthStatus)
			}
			cm.statuses <- StatusUpdate{e.ID, "health", healthStatus}
		case "create":
			if log.IsEnabledFor(logging.DEBUG) {
				log.Debugf("handling docker event: action=create id=%s", e.ID)
			}
			cm.needsRefresh <- e.ID
		case "destroy":
			if log.IsEnabledFor(logging.DEBUG) {
				log.Debugf("handling docker event: action=destroy id=%s", e.ID)
			}
			cm.delByID(e.ID)
		default:
			// check if this action changes status e.g. start -> running
			status := actionToStatus[actionName]
			if status != "" {
				if log.IsEnabledFor(logging.DEBUG) {
					log.Debugf("handling docker event: action=%s id=%s %s", actionName, e.ID, status)
				}
				cm.statuses <- StatusUpdate{e.ID, "status", status}
			}
		}
	}
	log.Info("docker event listener exited")
	close(cm.closed)
}

func portsFormat(ports map[api.Port][]api.PortBinding) string {
	var exposed []string
	var published []string

	for k, v := range ports {
		if len(v) == 0 {
			exposed = append(exposed, string(k))
			continue
		}
		for _, binding := range v {
			s := fmt.Sprintf("%s:%s -> %s", binding.HostIP, binding.HostPort, k)
			published = append(published, s)
		}
	}

	return strings.Join(append(exposed, published...), "\n")
}

func webPort(ports map[api.Port][]api.PortBinding) string {
	for _, v := range ports {
		if len(v) == 0 {
			continue
		}
		binding := v[0]
		publishedIp := binding.HostIP
		if publishedIp == "0.0.0.0" {
			publishedIp = "localhost"
		}
		return fmt.Sprintf("%s:%s", publishedIp, binding.HostPort)
	}
	return ""
}

func ipsFormat(networks map[string]api.ContainerNetwork) string {
	var ips []string

	for k, v := range networks {
		s := fmt.Sprintf("%s:%s", k, v.IPAddress)
		ips = append(ips, s)
	}

	return strings.Join(ips, "\n")
}

func mountsFormat(mounts []api.Mount) string {
	var ms []string
	for _, m := range mounts {
		mode := "rw"
		if !m.RW || m.Mode == "ro" {
			mode = "ro"
		}
		mType := "volume"
		if m.Driver == "" && strings.HasPrefix(m.Source, "/") {
			mType = "bind"
		}
		ms = append(ms, fmt.Sprintf("%s:::%s:::%s:::%s:::%s", m.Destination, m.Source, mType, mode, m.Driver))
	}
	return strings.Join(ms, ";;")
}

func labelsFormat(labels map[string]string) string {
	var ls []string
	for k, v := range labels {
		ls = append(ls, fmt.Sprintf("%s=%s", k, v))
	}
	return strings.Join(ls, ";;")
}

func networksFormat(networks map[string]api.ContainerNetwork) string {
	var ns []string
	for name, net := range networks {
		ns = append(ns, fmt.Sprintf("%s:::%s:::%s:::%s:::%d", name, net.IPAddress, net.Gateway, net.MacAddress, net.IPPrefixLen))
	}
	return strings.Join(ns, ";;")
}

func (cm *Docker) refresh(c *container.Container) {
	insp, found, failed := cm.inspect(c.Id)
	if failed {
		return
	}
	// remove container if no longer exists
	if !found {
		cm.delByID(c.Id)
		return
	}
	c.SetMeta("name", shortName(insp.Name))
	c.SetMeta("image", insp.Config.Image)
	c.SetMeta("IPs", ipsFormat(insp.NetworkSettings.Networks))
	c.SetMeta("ports", portsFormat(insp.NetworkSettings.Ports))
	webPort := webPort(insp.NetworkSettings.Ports)
	if webPort != "" {
		c.SetMeta("Web Port", webPort)
	}
	c.SetMeta("created", insp.Created.Format("Mon Jan 02 15:04:05 2006"))
	c.SetMeta("uptime", calcUptime(insp))
	c.SetMeta("health", insp.State.Health.Status)
	c.SetMeta("[ENV-VAR]", strings.Join(insp.Config.Env, ";"))
	c.SetMeta("[MOUNTS]", mountsFormat(insp.Mounts))
	c.SetMeta("[LABELS]", labelsFormat(insp.Config.Labels))
	c.SetMeta("[NETWORKS]", networksFormat(insp.NetworkSettings.Networks))

	if len(insp.Config.Entrypoint) > 0 {
		c.SetMeta("entrypoint", strings.Join(insp.Config.Entrypoint, " "))
	}
	if len(insp.Config.Cmd) > 0 {
		c.SetMeta("cmd", strings.Join(insp.Config.Cmd, " "))
	}
	if insp.Config.WorkingDir != "" {
		c.SetMeta("workdir", insp.Config.WorkingDir)
	}
	if insp.Config.User != "" {
		c.SetMeta("user", insp.Config.User)
	}
	if insp.HostConfig != nil {
		if insp.HostConfig.RestartPolicy.Name != "" {
			c.SetMeta("restartPolicy", insp.HostConfig.RestartPolicy.Name)
		}
		if insp.HostConfig.Memory > 0 {
			c.SetMeta("memLimit", fmt.Sprintf("%d MB", insp.HostConfig.Memory/(1024*1024)))
		}
		if insp.HostConfig.NanoCPUs > 0 {
			c.SetMeta("cpuLimit", fmt.Sprintf("%.2f CPUs", float64(insp.HostConfig.NanoCPUs)/1e9))
		}
		if insp.HostConfig.PidsLimit != nil && *insp.HostConfig.PidsLimit > 0 {
			c.SetMeta("pidsLimit", fmt.Sprintf("%d", *insp.HostConfig.PidsLimit))
		}
		c.SetMeta("privileged", fmt.Sprintf("%t", insp.HostConfig.Privileged))
		c.SetMeta("readonlyRootfs", fmt.Sprintf("%t", insp.HostConfig.ReadonlyRootfs))
	}
	if insp.State.Status != "running" {
		c.SetMeta("exitCode", fmt.Sprintf("%d", insp.State.ExitCode))
		c.SetMeta("oomKilled", fmt.Sprintf("%t", insp.State.OOMKilled))
	}
	c.SetState(insp.State.Status)
}

func (cm *Docker) inspect(id string) (insp *api.Container, found bool, failed bool) {
	opts := api.InspectContainerOptions{ID: id}
	c, err := cm.client.InspectContainerWithOptions(opts)
	if err != nil {
		if _, notFound := err.(*api.NoSuchContainer); notFound {
			return c, false, false
		}
		// other error e.g. connection failed
		log.Errorf("%s (%T)", err.Error(), err)
		return c, false, true
	}
	return c, true, false
}

func calcUptime(insp *api.Container) string {
	if insp.State.StartedAt.IsZero() {
		return "-"
	}
	endTime := insp.State.FinishedAt
	if endTime.IsZero() || insp.State.Running {
		endTime = time.Now()
	}
	uptime := endTime.Sub(insp.State.StartedAt)
	return durafmt.Parse(uptime).LimitFirstN(1).String()
}

// Mark all container IDs for refresh
func (cm *Docker) refreshAll() {
	opts := api.ListContainersOptions{All: true}
	allContainers, err := cm.client.ListContainers(opts)
	if err != nil {
		log.Errorf("%s (%T)", err.Error(), err)
		return
	}

	for _, i := range allContainers {
		c := cm.MustGet(i.ID)
		if len(i.Names) > 0 {
			c.SetMeta("name", shortName(i.Names[0]))
		}
		state := i.State
		if state == "" {
			statusLower := strings.ToLower(i.Status)
			if strings.Contains(statusLower, "paused") {
				state = "paused"
			} else if strings.Contains(statusLower, "restarting") {
				state = "restarting"
			} else if strings.HasPrefix(statusLower, "up") {
				state = "running"
			} else if strings.HasPrefix(statusLower, "exited") {
				state = "exited"
			} else if strings.HasPrefix(statusLower, "created") {
				state = "created"
			}
		}
		c.SetState(state)
		cm.needsRefresh <- c.Id
	}
}

func (cm *Docker) Loop() {
	for {
		select {
		case id := <-cm.needsRefresh:
			c := cm.MustGet(id)
			cm.refresh(c)
		case <-cm.closed:
			return
		}
	}
}

func (cm *Docker) LoopStatuses() {
	for {
		select {
		case statusUpdate := <-cm.statuses:
			c, _ := cm.Get(statusUpdate.Cid)
			if c != nil {
				if statusUpdate.Field == "health" {
					c.SetMeta("health", statusUpdate.Status)
				} else {
					c.SetState(statusUpdate.Status)
				}
			}
		case <-cm.closed:
			return
		}
	}
}

// MustGet gets a single container, creating one anew if not existing
func (cm *Docker) MustGet(id string) *container.Container {
	c, ok := cm.Get(id)
	if ok {
		return c
	}
	// append container struct for new containers
	// create collector
	collector := collector.NewDocker(cm.client, id)
	// create manager
	manager := manager.NewDocker(cm.client, id)
	// create container
	c = container.New(id, collector, manager)
	cm.lock.Lock()
	if existing, exists := cm.containers[id]; exists {
		cm.lock.Unlock()
		return existing
	}
	cm.containers[id] = c
	cm.lock.Unlock()
	return c
}

// Docker implements Connector
func (cm *Docker) Get(id string) (*container.Container, bool) {
	cm.lock.RLock()
	c, ok := cm.containers[id]
	cm.lock.RUnlock()
	return c, ok
}

// Remove containers by ID
func (cm *Docker) delByID(id string) {
	cm.lock.Lock()
	c, ok := cm.containers[id]
	if ok {
		delete(cm.containers, id)
	}
	cm.lock.Unlock()
	if ok && c != nil {
		c.SetState("exited")
	}
	log.Infof("removed dead container: %s", id)
}

// Docker implements Connector
func (cm *Docker) All() (containers container.Containers) {
	cm.lock.RLock()
	for _, c := range cm.containers {
		containers = append(containers, c)
	}
	cm.lock.RUnlock()

	containers.Sort()
	containers.Filter()
	return containers
}

// use primary container name
func shortName(name string) string {
	return strings.TrimPrefix(name, "/")
}
