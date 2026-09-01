// docker.go implements the Connector interface for Docker daemons via go-dockerclient.
package connector

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/hako/durafmt"
	"github.com/op/go-logging"

	"github.com/edsilegx/ctop/pkg/connector/collector"
	"github.com/edsilegx/ctop/pkg/connector/manager"
	"github.com/edsilegx/ctop/pkg/container"
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
	hostID       string
	containers   map[string]*container.Container
	needsRefresh chan string // container IDs requiring refresh
	statuses     chan StatusUpdate
	closed       chan struct{}
	closeOnce    sync.Once
	lock         sync.RWMutex
}

func (cm *Docker) Close() {
	cm.closeOnce.Do(func() {
		close(cm.closed)
	})
}

// TLSConfig holds paths to TLS certificates and keys for mTLS daemon authentication.
type TLSConfig struct {
	Verify bool
	CA     string
	Cert   string
	Key    string
}

var (
	globalTLSLock   sync.RWMutex
	globalTLSConfig TLSConfig
)

// SetGlobalTLSConfig sets the global TLS configuration used when creating Docker clients.
func SetGlobalTLSConfig(cfg TLSConfig) {
	globalTLSLock.Lock()
	defer globalTLSLock.Unlock()
	globalTLSConfig = cfg
}

func getGlobalTLSConfig() TLSConfig {
	globalTLSLock.RLock()
	defer globalTLSLock.RUnlock()
	return globalTLSConfig
}

// newDockerClient initializes a docker API client with TLS/mTLS certificate support or env fallback.
func newDockerClient(endpoint string) (*api.Client, error) {
	cfg := getGlobalTLSConfig()
	if cfg.Cert != "" || cfg.Key != "" || cfg.CA != "" {
		return api.NewTLSClient(endpoint, cfg.Cert, cfg.Key, cfg.CA)
	}
	if certPath := os.Getenv("DOCKER_CERT_PATH"); certPath != "" {
		ca := filepath.Join(certPath, "ca.pem")
		cert := filepath.Join(certPath, "cert.pem")
		key := filepath.Join(certPath, "key.pem")
		return api.NewTLSClient(endpoint, cert, key, ca)
	}
	if cfg.Verify || os.Getenv("DOCKER_TLS_VERIFY") == "1" || strings.HasPrefix(endpoint, "https://") {
		return api.NewClientFromEnv()
	}
	return api.NewClient(endpoint)
}

func NewDocker() (Connector, error) {
	// init docker client with context endpoint resolution
	var (
		client *api.Client
		err    error
	)
	if endpoint := ResolveDockerEndpoint(); endpoint != "" {
		client, err = newDockerClient(endpoint)
	} else {
		cfg := getGlobalTLSConfig()
		if cfg.Cert != "" || cfg.Key != "" || cfg.CA != "" {
			client, err = api.NewTLSClient(os.Getenv("DOCKER_HOST"), cfg.Cert, cfg.Key, cfg.CA)
		} else {
			client, err = api.NewClientFromEnv()
		}
	}
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

// NewDockerWithEndpoint creates a Docker connector for a specific remote endpoint and host identifier.
func NewDockerWithEndpoint(endpoint, hostID string) (Connector, error) {
	client, err := newDockerClient(endpoint)
	if err != nil {
		return nil, err
	}
	cm := &Docker{
		client:       client,
		hostID:       hostID,
		containers:   make(map[string]*container.Container),
		needsRefresh: make(chan string, 60),
		statuses:     make(chan StatusUpdate, 60),
		closed:       make(chan struct{}),
		lock:         sync.RWMutex{},
	}
	if err := client.Ping(); err != nil {
		return nil, err
	}
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
	cm.Close()
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
	if cm.hostID != "" {
		c.SetHost(cm.hostID)
	}
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
	if insp.State.Status != "" {
		c.SetState(insp.State.Status)
	} else if insp.State.Running {
		c.SetState("running")
	} else {
		c.SetState("exited")
	}
	c.SetMeta("[ENV-VAR]", strings.Join(insp.Config.Env, ";"))
	c.SetMeta("[MOUNTS]", mountsFormat(insp.Mounts))
	c.SetMeta("[LABELS]", labelsFormat(insp.Config.Labels))
	c.SetMeta("[NETWORKS]", networksFormat(insp.NetworkSettings.Networks))
	if insp.Config.Labels != nil {
		for k, v := range insp.Config.Labels {
			c.SetMeta(k, v)
		}
		if proj := insp.Config.Labels["com.docker.compose.project"]; proj != "" {
			c.SetMeta("composeProject", proj)
		}
		if svc := insp.Config.Labels["com.docker.compose.service"]; svc != "" {
			c.SetMeta("composeService", svc)
		}
		if num := insp.Config.Labels["com.docker.compose.container-number"]; num != "" {
			c.SetMeta("composeNumber", num)
		}
		if oneoff := insp.Config.Labels["com.docker.compose.oneoff"]; oneoff != "" {
			c.SetMeta("composeOneoff", oneoff)
		}
		if ver := insp.Config.Labels["com.docker.compose.version"]; ver != "" {
			c.SetMeta("composeVersion", ver)
		}
	}

	if len(insp.Config.Entrypoint) > 0 {
		c.SetMeta("entrypoint", strings.Join(insp.Config.Entrypoint, " "))
	}
	if len(insp.Config.Cmd) > 0 {
		c.SetMeta("cmd", strings.Join(insp.Config.Cmd, " "))
	}
	if insp.Config.WorkingDir != "" {
		c.SetMeta("workdir", insp.Config.WorkingDir)
	}
	userVal := insp.Config.User
	if userVal == "" {
		userVal = "root (default UID 0)"
	}
	c.SetMeta("user", userVal)

	if insp.HostConfig != nil {
		if insp.HostConfig.RestartPolicy.Name != "" {
			c.SetMeta("restartPolicy", insp.HostConfig.RestartPolicy.Name)
		}
		if insp.HostConfig.Memory > 0 {
			c.SetMeta("memLimit", fmt.Sprintf("%d MB", insp.HostConfig.Memory/(1024*1024)))
		} else {
			c.SetMeta("memLimit", "unlimited")
		}
		if insp.HostConfig.NanoCPUs > 0 {
			c.SetMeta("cpuLimit", fmt.Sprintf("%.2f CPUs", float64(insp.HostConfig.NanoCPUs)/1e9))
		} else {
			c.SetMeta("cpuLimit", "unlimited")
		}
		if insp.HostConfig.PidsLimit != nil && *insp.HostConfig.PidsLimit > 0 {
			c.SetMeta("pidsLimit", fmt.Sprintf("%d", *insp.HostConfig.PidsLimit))
		} else {
			c.SetMeta("pidsLimit", "unlimited")
		}
		if len(insp.HostConfig.CapAdd) > 0 {
			c.SetMeta("capAdd", strings.Join(insp.HostConfig.CapAdd, ", "))
		} else {
			c.SetMeta("capAdd", "default (none added)")
		}
		if len(insp.HostConfig.CapDrop) > 0 {
			c.SetMeta("capDrop", strings.Join(insp.HostConfig.CapDrop, ", "))
		} else {
			c.SetMeta("capDrop", "none (standard set)")
		}
		if len(insp.HostConfig.SecurityOpt) > 0 {
			c.SetMeta("securityOpt", strings.Join(insp.HostConfig.SecurityOpt, ", "))
		} else {
			c.SetMeta("securityOpt", "default (seccomp:default)")
		}
		c.SetMeta("privileged", fmt.Sprintf("%t", insp.HostConfig.Privileged))
		c.SetMeta("readonlyRootfs", fmt.Sprintf("%t", insp.HostConfig.ReadonlyRootfs))
	}
	if insp.State.Health.Status != "" {
		c.SetMeta("healthStatus", insp.State.Health.Status)
		c.SetMeta("failingStreak", fmt.Sprintf("%d", insp.State.Health.FailingStreak))
		var hLogs []string
		for _, hLog := range insp.State.Health.Log {
			out := strings.TrimSpace(hLog.Output)
			if len(out) > 60 {
				out = out[:60] + "..."
			}
			out = strings.ReplaceAll(out, "\n", " ")
			out = strings.ReplaceAll(out, ":::", " ")
			hLogs = append(hLogs, fmt.Sprintf("%d:::%s:::%s", hLog.ExitCode, hLog.Start.Format("15:04:05"), out))
		}
		if len(hLogs) > 0 {
			c.SetMeta("[HEALTH-LOG]", strings.Join(hLogs, ";;"))
		}
	} else {
		c.SetMeta("healthStatus", "none configured")
	}
	if insp.Config.Healthcheck != nil {
		if len(insp.Config.Healthcheck.Test) > 0 {
			c.SetMeta("healthTest", strings.Join(insp.Config.Healthcheck.Test, " "))
		}
		if insp.Config.Healthcheck.Interval > 0 {
			c.SetMeta("healthInterval", insp.Config.Healthcheck.Interval.String())
		}
		if insp.Config.Healthcheck.Timeout > 0 {
			c.SetMeta("healthTimeout", insp.Config.Healthcheck.Timeout.String())
		}
		if insp.Config.Healthcheck.Retries > 0 {
			c.SetMeta("healthRetries", fmt.Sprintf("%d", insp.Config.Healthcheck.Retries))
		}
	}
	if insp.State.Status != "running" {
		c.SetMeta("exitCode", fmt.Sprintf("%d", insp.State.ExitCode))
	}
	c.SetMeta("oomKilled", fmt.Sprintf("%t", insp.State.OOMKilled))

	if insp.Image != "" {
		c.SetMeta("imageId", insp.Image)
		img, err := cm.client.InspectImage(insp.Image)
		if err != nil && insp.Config.Image != "" {
			img, err = cm.client.InspectImage(insp.Config.Image)
		}
		if err == nil && img != nil {
			if len(img.RepoTags) > 0 {
				c.SetMeta("imageRepoTags", strings.Join(img.RepoTags, ", "))
			}
			if len(img.RepoDigests) > 0 {
				c.SetMeta("imageRepoDigests", strings.Join(img.RepoDigests, "\n"))
			}
			if img.Architecture != "" {
				archStr := img.Architecture
				if img.OS != "" {
					archStr = fmt.Sprintf("%s/%s", img.OS, img.Architecture)
				}
				c.SetMeta("imageArch", archStr)
			}
			if img.Author != "" {
				c.SetMeta("imageAuthor", img.Author)
			}
			if !img.Created.IsZero() {
				c.SetMeta("imageCreated", img.Created.Format("Mon Jan 02 15:04:05 2006"))
			}
			if img.DockerVersion != "" {
				c.SetMeta("imageDockerVersion", img.DockerVersion)
			}
			if img.Size > 0 {
				c.SetMeta("imageSize", fmt.Sprintf("%.2f MB (%d bytes)", float64(img.Size)/(1024*1024), img.Size))
			}
			if len(img.RootFS.Layers) > 0 {
				c.SetMeta("imageLayers", fmt.Sprintf("%d layers", len(img.RootFS.Layers)))
				c.SetMeta("imageLayerList", strings.Join(img.RootFS.Layers, "\n"))
			}
			if img.Config != nil {
				if len(img.Config.Entrypoint) > 0 {
					c.SetMeta("imageEntrypoint", strings.Join(img.Config.Entrypoint, " "))
				}
				if len(img.Config.Cmd) > 0 {
					c.SetMeta("imageCmd", strings.Join(img.Config.Cmd, " "))
				}
				if img.Config.WorkingDir != "" {
					c.SetMeta("imageWorkdir", img.Config.WorkingDir)
				}
				if img.Config.User != "" {
					c.SetMeta("imageUser", img.Config.User)
				}
				if len(img.Config.Env) > 0 {
					c.SetMeta("imageEnv", strings.Join(img.Config.Env, ";;"))
				}
				if len(img.Config.Labels) > 0 {
					c.SetMeta("imageLabels", labelsFormat(img.Config.Labels))
				}
				var expPorts []string
				for p := range img.Config.ExposedPorts {
					expPorts = append(expPorts, string(p))
				}
				if len(expPorts) > 0 {
					c.SetMeta("imageExposedPorts", strings.Join(expPorts, ", "))
				}
				var vols []string
				for v := range img.Config.Volumes {
					vols = append(vols, v)
				}
				if len(vols) > 0 {
					c.SetMeta("imageVolumes", strings.Join(vols, ", "))
				}
			}
		}
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
	if !insp.State.Running && !insp.State.Paused {
		return "-"
	}
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
		if len(i.Labels) > 0 {
			c.SetMeta("[LABELS]", labelsFormat(i.Labels))
			for k, v := range i.Labels {
				c.SetMeta(k, v)
			}
			if proj := i.Labels["com.docker.compose.project"]; proj != "" {
				c.SetMeta("composeProject", proj)
			}
			if svc := i.Labels["com.docker.compose.service"]; svc != "" {
				c.SetMeta("composeService", svc)
			}
			if num := i.Labels["com.docker.compose.container-number"]; num != "" {
				c.SetMeta("composeNumber", num)
			}
			if oneoff := i.Labels["com.docker.compose.oneoff"]; oneoff != "" {
				c.SetMeta("composeOneoff", oneoff)
			}
			if ver := i.Labels["com.docker.compose.version"]; ver != "" {
				c.SetMeta("composeVersion", ver)
			}
		}
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
