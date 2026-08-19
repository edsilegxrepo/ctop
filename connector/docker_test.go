// docker_test.go validates Docker connector initialization, container map concurrency, and format parsing.
// Test Strategy: Combines synthetic in-memory mocks with httptest HTTP mock daemon endpoints to verify REST JSON handling and event dispatching.
package connector

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/edsilegx/ctop/connector/collector"
	"github.com/edsilegx/ctop/container"
	"github.com/edsilegx/ctop/models"
	api "github.com/fsouza/go-dockerclient"
)

type dummyCollector struct {
	running bool
}

func (d *dummyCollector) Start()                       { d.running = true }
func (d *dummyCollector) Stop()                        { d.running = false }
func (d *dummyCollector) Running() bool                { return d.running }
func (d *dummyCollector) Stream() chan models.Metrics  { return make(chan models.Metrics) }
func (d *dummyCollector) Logs() collector.LogCollector { return nil }

type dummyManager struct{}

func (d *dummyManager) Start() error            { return nil }
func (d *dummyManager) Stop() error             { return nil }
func (d *dummyManager) Remove() error           { return nil }
func (d *dummyManager) Pause() error            { return nil }
func (d *dummyManager) Unpause() error          { return nil }
func (d *dummyManager) Restart() error                        { return nil }
func (d *dummyManager) Exec(cmd []string) error               { return nil }
func (d *dummyManager) Kill(sig string) error                 { return nil }
func (d *dummyManager) Top(args string) (models.TopResult, error) {
	return models.TopResult{}, nil
}
func (d *dummyManager) Changes() ([]models.Change, error) { return nil, nil }
func (d *dummyManager) ReadDir(path string) ([]models.FileInfo, error) {
	return []models.FileInfo{
		{Name: "app", Path: "/app", IsDir: true, Mode: "drwxr-xr-x"},
	}, nil
}
func (d *dummyManager) ReadFile(path string, maxBytes int64) (string, error) {
	return "dummy file content", nil
}
func (d *dummyManager) Download(srcPath, dstPath string) (int64, error) {
	return 1024, nil
}
func (d *dummyManager) Upload(srcPath, dstPath string) error {
	return nil
}
func (d *dummyManager) UpdateResources(memoryMB int64, cpus float64, restartPolicy string) error {
	return nil
}

func TestDockerMustGetConcurrent(t *testing.T) {
	cm := &Docker{
		containers:   make(map[string]*container.Container),
		needsRefresh: make(chan string, 60),
		statuses:     make(chan StatusUpdate, 60),
		closed:       make(chan struct{}),
	}

	const id = "test-concurrent-cid-123"
	const goroutines = 20

	var wg sync.WaitGroup
	containers := make([]*container.Container, goroutines)

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			containers[idx] = cm.MustGet(id)
		}(i)
	}

	wg.Wait()

	first := containers[0]
	if first == nil {
		t.Fatal("expected container pointer, got nil")
	}

	for i := 1; i < goroutines; i++ {
		if containers[i] != first {
			t.Fatalf("goroutine %d got different container instance than goroutine 0", i)
		}
	}

	if len(cm.containers) != 1 {
		t.Fatalf("expected 1 container in map, got %d", len(cm.containers))
	}
}

func TestDockerDelByID(t *testing.T) {
	cm := &Docker{
		containers:   make(map[string]*container.Container),
		needsRefresh: make(chan string, 60),
		statuses:     make(chan StatusUpdate, 60),
		closed:       make(chan struct{}),
	}

	dummyCol := &dummyCollector{}
	c := container.New("test-del-cid", dummyCol, &dummyManager{})
	cm.lock.Lock()
	cm.containers["test-del-cid"] = c
	cm.lock.Unlock()

	c.SetState("running")
	if !dummyCol.Running() {
		t.Fatal("expected dummy collector to be running")
	}

	if _, ok := cm.Get("test-del-cid"); !ok {
		t.Fatal("expected container to exist")
	}

	cm.delByID("test-del-cid")

	if _, ok := cm.Get("test-del-cid"); ok {
		t.Fatal("expected container to be deleted")
	}

	if state := c.GetMeta("state"); state != "exited" {
		t.Fatalf("expected deleted container state to be exited, got %s", state)
	}

	if dummyCol.Running() {
		t.Fatal("expected collector to be stopped when deleted")
	}
}

func TestDockerConcurrentReads(t *testing.T) {
	cm := &Docker{
		containers:   make(map[string]*container.Container),
		needsRefresh: make(chan string, 60),
		statuses:     make(chan StatusUpdate, 60),
		closed:       make(chan struct{}),
	}

	for i := 0; i < 10; i++ {
		cid := string(rune('a' + i))
		cm.containers[cid] = container.New(cid, &dummyCollector{}, &dummyManager{})
		cm.containers[cid].SetMeta("name", "container-"+cid)
		cm.containers[cid].Display = true
	}

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_ = cm.All()
			}
		}()
		go func(idx int) {
			defer wg.Done()
			cid := string(rune('a' + (idx % 10)))
			for j := 0; j < 50; j++ {
				_, _ = cm.Get(cid)
			}
		}(i)
	}

	wg.Wait()
}

func TestConnectorRegistryAndSuper(t *testing.T) {
	enabledList := Enabled()
	if len(enabledList) == 0 {
		t.Fatal("expected at least 1 enabled connector")
	}

	// ByName mock
	super, err := ByName("mock")
	if err != nil {
		t.Fatalf("expected ByName('mock') to succeed, got %v", err)
	}
	if super == nil {
		t.Fatal("expected non-nil ConnectorSuper")
	}

	var conn Connector
	for i := 0; i < 50; i++ {
		conn, err = super.Get()
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil || conn == nil {
		t.Fatalf("expected super.Get() to return mock connector, err: %v", err)
	}

	// Calling Get() second time should return cached conn
	conn2, err := super.Get()
	if err != nil || conn2 != conn {
		t.Fatalf("expected cached connector from super.Get()")
	}

	// ByName non-existent
	_, err = ByName("invalid-connector-name")
	if err == nil {
		t.Fatal("expected error for invalid connector name")
	}

	// ConnectorSuper with failing connFn
	failCount := 0
	failingSuper := NewConnectorSuper(func() (Connector, error) {
		failCount++
		if failCount == 1 {
			return nil, errors.New("initial connection failed")
		}
		return &Mock{}, nil
	})

	_, _ = failingSuper.Get()
	time.Sleep(50 * time.Millisecond)
	_, _ = failingSuper.Get()
}

func TestConnectorHelpers(t *testing.T) {
	// shortName
	if short := shortName("/my-container"); short != "my-container" {
		t.Fatalf("expected 'my-container', got '%s'", short)
	}
	if short := shortName("standalone"); short != "standalone" {
		t.Fatalf("expected 'standalone', got '%s'", short)
	}

	// portsFormat and webPort
	ports := map[api.Port][]api.PortBinding{
		"80/tcp": {
			{HostIP: "0.0.0.0", HostPort: "8080"},
		},
		"443/tcp": {},
	}
	formatted := portsFormat(ports)
	if formatted == "" {
		t.Fatal("expected non-empty portsFormat")
	}

	web := webPort(ports)
	if web != "localhost:8080" {
		t.Fatalf("expected 'localhost:8080', got '%s'", web)
	}

	// ipsFormat
	networks := map[string]api.ContainerNetwork{
		"bridge": {IPAddress: "172.17.0.2"},
	}
	ips := ipsFormat(networks)
	if ips != "bridge:172.17.0.2" {
		t.Fatalf("expected 'bridge:172.17.0.2', got '%s'", ips)
	}

	// calcUptime
	insp := &api.Container{
		State: api.State{
			Running:   true,
			StartedAt: time.Now().Add(-2 * time.Hour),
		},
	}
	uptime := calcUptime(insp)
	if uptime == "" || uptime == "-" {
		t.Fatalf("expected non-empty uptime, got '%s'", uptime)
	}

	// portsFormat & webPort edge cases
	ports = map[api.Port][]api.PortBinding{
		"80/tcp":  {{HostIP: "0.0.0.0", HostPort: "8080"}},
		"443/tcp": {},
	}
	pf := portsFormat(ports)
	if !strings.Contains(pf, "443/tcp") || !strings.Contains(pf, "80/tcp") {
		t.Fatalf("expected ports formatted with exposed and published, got '%s'", pf)
	}
	wp := webPort(ports)
	if wp != "localhost:8080" {
		t.Fatalf("expected webPort 'localhost:8080', got '%s'", wp)
	}
	if emptyWp := webPort(map[api.Port][]api.PortBinding{"80/tcp": {}}); emptyWp != "" {
		t.Fatalf("expected empty webPort, got '%s'", emptyWp)
	}
	if customWp := webPort(map[api.Port][]api.PortBinding{"8080/tcp": {{HostIP: "192.168.1.100", HostPort: "8080"}}}); customWp != "192.168.1.100:8080" {
		t.Fatalf("expected '192.168.1.100:8080', got '%s'", customWp)
	}

	// ipsFormat multi-network
	multiIps := ipsFormat(map[string]api.ContainerNetwork{
		"net1": {IPAddress: "10.0.0.1"},
		"net2": {IPAddress: "10.0.0.2"},
	})
	if !strings.Contains(multiIps, "net1:10.0.0.1") || !strings.Contains(multiIps, "net2:10.0.0.2") {
		t.Fatalf("expected multi ips, got '%s'", multiIps)
	}

	// calcUptime stopped container
	stoppedInsp := &api.Container{
		State: api.State{Running: false},
	}
	if u := calcUptime(stoppedInsp); u != "-" {
		t.Fatalf("expected '-' for stopped container uptime, got '%s'", u)
	}
}

func TestDockerMockServerLifecycle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/info":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ID":"mock-docker","Containers":1}`))
		case r.URL.Path == "/containers/json":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[{"Id":"c123","Names":["/test-container"],"State":"running"}]`))
		case strings.HasPrefix(r.URL.Path, "/containers/c123/json"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"Id": "c123",
				"Name": "/test-container",
				"Created": "2026-08-18T10:00:00Z",
				"Config": {"Image": "alpine:latest", "Env": ["KEY=VAL"]},
				"State": {"Status": "running", "Running": true, "StartedAt": "2026-08-18T10:00:00Z", "Health": {"Status": "healthy"}},
				"NetworkSettings": {"Ports": {"80/tcp": [{"HostIP": "0.0.0.0", "HostPort": "8080"}]}, "Networks": {"bridge": {"IPAddress": "172.17.0.2"}}}
			}`))
		case strings.HasPrefix(r.URL.Path, "/containers/nonexistent/json"):
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"No such container"}`))
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer server.Close()

	client, err := api.NewClient(server.URL)
	if err != nil {
		t.Fatalf("failed to create client for mock server: %v", err)
	}

	cm := &Docker{
		client:       client,
		containers:   make(map[string]*container.Container),
		needsRefresh: make(chan string, 60),
		statuses:     make(chan StatusUpdate, 60),
		closed:       make(chan struct{}),
		lock:         sync.RWMutex{},
	}

	// Test refreshAll
	cm.refreshAll()
	if len(cm.containers) != 1 {
		t.Fatalf("expected 1 container after refreshAll, got %d", len(cm.containers))
	}

	// Test refresh
	c := cm.MustGet("c123")
	cm.refresh(c)
	if c.GetMeta("name") != "test-container" {
		t.Fatalf("expected container name 'test-container', got '%s'", c.GetMeta("name"))
	}
	if c.GetMeta("image") != "alpine:latest" {
		t.Fatalf("expected container image 'alpine:latest', got '%s'", c.GetMeta("image"))
	}

	// Test inspect with not found container
	nonExist := container.New("nonexistent", &dummyCollector{}, &dummyManager{})
	cm.containers["nonexistent"] = nonExist
	cm.refresh(nonExist)
	if _, exists := cm.Get("nonexistent"); exists {
		t.Fatal("expected nonexistent container to be removed after refresh")
	}

	// Test Loop and LoopStatuses in goroutines
	go cm.Loop()
	go cm.LoopStatuses()

	cm.needsRefresh <- "c123"
	cm.statuses <- StatusUpdate{Cid: "c123", Field: "status", Status: "paused"}
	cm.statuses <- StatusUpdate{Cid: "c123", Field: "health", Status: "healthy"}

	time.Sleep(100 * time.Millisecond)

	if c.GetMeta("health") != "healthy" {
		t.Fatalf("expected health 'healthy', got '%s'", c.GetMeta("health"))
	}

	// Close loops
	cm.Close()
}

func TestNewDockerFromMockServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/info":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ID":"mock-docker","Driver":"overlay2","Images":3,"Name":"test-node","ServerVersion":"20.10.0"}`))
		case r.URL.Path == "/containers/json":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[{"Id":"c123","Names":["/test-container"],"State":"running"}]`))
		case strings.HasPrefix(r.URL.Path, "/containers/c123/json"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"Id": "c123",
				"Name": "/test-container",
				"Created": "2026-08-18T10:00:00Z",
				"Config": {"Image": "alpine:latest", "Env": ["KEY=VAL"]},
				"State": {"Status": "running", "Running": true, "StartedAt": "2026-08-18T10:00:00Z", "Health": {"Status": "healthy"}},
				"NetworkSettings": {"Ports": {"80/tcp": [{"HostIP": "0.0.0.0", "HostPort": "8080"}]}, "Networks": {"bridge": {"IPAddress": "172.17.0.2"}}}
			}`))
		case r.URL.Path == "/events":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"start","id":"c123","from":"alpine","time":1600000000}` + "\n"))
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer server.Close()

	t.Setenv("DOCKER_HOST", server.URL)
	conn, err := NewDocker()
	if err != nil {
		t.Fatalf("unexpected error from NewDocker: %v", err)
	}
	if dm, ok := conn.(*Docker); ok {
		defer dm.Close()
	}

	time.Sleep(50 * time.Millisecond)

	all := conn.All()
	if len(all) == 0 {
		t.Fatal("expected non-empty containers from NewDocker()")
	}

	if c, ok := conn.Get("c123"); !ok || c == nil {
		t.Fatal("expected to find container c123")
	}

	if dm, ok := conn.(*Docker); ok {
		cMust := dm.MustGet("c123")
		if cMust == nil {
			t.Fatal("expected non-nil from MustGet")
		}
		dm.delByID("c123")
		if _, exists := dm.Get("c123"); exists {
			t.Fatal("expected c123 to be removed after delByID")
		}
		go func() {
			time.Sleep(10 * time.Millisecond)
			dm.Close()
		}()
		dm.Wait()
	}
}

func TestMockConnectorOperations(t *testing.T) {
	conn, err := NewMock()
	if err != nil {
		t.Fatalf("unexpected error from NewMock: %v", err)
	}

	all := conn.All()
	if len(all) == 0 {
		t.Fatal("expected non-empty containers from NewMock()")
	}

	first := all[0]
	if found, ok := conn.Get(first.Id); !ok || found == nil {
		t.Fatalf("expected to find container %s", first.Id)
	}

	if _, ok := conn.Get("nonexistent-id"); ok {
		t.Fatal("expected nonexistent-id to return ok=false")
	}
}

func TestConnectorFormattingHelpers(t *testing.T) {
	// 1. mountsFormat
	mounts := []api.Mount{
		{Source: "/var/lib/docker/volumes/myvol/_data", Destination: "/data", RW: true, Driver: "local"},
		{Source: "/host/path", Destination: "/container/path", RW: false, Mode: "ro", Driver: ""},
	}
	mStr := mountsFormat(mounts)
	if !strings.Contains(mStr, "/data:::") || !strings.Contains(mStr, "bind") {
		t.Errorf("unexpected mountsFormat result: %s", mStr)
	}

	// 2. labelsFormat
	labels := map[string]string{
		"com.docker.compose.service": "web",
		"version":                    "1.0",
	}
	lStr := labelsFormat(labels)
	if !strings.Contains(lStr, "com.docker.compose.service=web") {
		t.Errorf("unexpected labelsFormat result: %s", lStr)
	}

	// 3. networksFormat
	nets := map[string]api.ContainerNetwork{
		"bridge": {IPAddress: "172.17.0.2", Gateway: "172.17.0.1", MacAddress: "02:42:ac:11:00:02", IPPrefixLen: 16},
	}
	nStr := networksFormat(nets)
	if !strings.Contains(nStr, "bridge:::172.17.0.2:::172.17.0.1") {
		t.Errorf("unexpected networksFormat result: %s", nStr)
	}
}

func TestRuncConnectorStructures(t *testing.T) {
	tempRuncRoot := t.TempDir()
	t.Setenv("RUNC_ROOT", tempRuncRoot)
	t.Setenv("RUNC_SYSTEMD_CGROUP", "1")

	opts, err := NewRuncOpts()
	if err != nil {
		t.Fatalf("unexpected error from NewRuncOpts: %v", err)
	}
	if !opts.systemdCgroups || opts.root != tempRuncRoot {
		t.Errorf("unexpected opts: %+v", opts)
	}

	runcConn, err := NewRunc()
	if err != nil {
		t.Fatalf("unexpected error from NewRunc: %v", err)
	}
	if rc, ok := runcConn.(*Runc); ok {
		defer close(rc.closed)
		if rc.GetLibc("nonexistent") != nil {
			t.Error("expected nil for nonexistent libc")
		}
		rc.refreshAll()
		all := rc.All()
		if len(all) != 0 {
			t.Errorf("expected 0 containers in empty runc root, got %d", len(all))
		}
		rc.delByID("test-id")
	}
}

func TestDockerConnectorFullRefresh(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/containers/json":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[{"Id":"c_full","Names":["/full-container"],"State":"exited"}]`))
		case strings.HasPrefix(r.URL.Path, "/containers/c_full/json"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"Id": "c_full",
				"Name": "/full-container",
				"Created": "2026-08-18T10:00:00Z",
				"Config": {
					"Image": "redis:alpine",
					"Env": ["PORT=6379", "SECRET=test"],
					"Labels": {"com.docker.compose.project": "myproject", "com.docker.compose.service": "redis"},
					"Entrypoint": ["docker-entrypoint.sh"],
					"Cmd": ["redis-server"],
					"WorkingDir": "/data",
					"User": "1000:1000",
					"Healthcheck": {"Test": ["CMD-SHELL", "redis-cli ping"], "Interval": 30000000000, "Timeout": 5000000000, "Retries": 3}
				},
				"State": {
					"Status": "exited",
					"Running": false,
					"ExitCode": 137,
					"OOMKilled": true,
					"StartedAt": "2026-08-18T10:00:00Z",
					"Health": {
						"Status": "unhealthy",
						"FailingStreak": 5,
						"Log": [{"Start": "2026-08-18T10:05:00Z", "ExitCode": 1, "Output": "Connection refused"}]
					}
				},
				"HostConfig": {
					"RestartPolicy": {"Name": "unless-stopped"},
					"Memory": 1073741824,
					"NanoCPUs": 2000000000,
					"CapAdd": ["NET_ADMIN"],
					"CapDrop": ["MKNOD"],
					"SecurityOpt": ["apparmor=docker-default"],
					"Privileged": true,
					"ReadonlyRootfs": false
				},
				"NetworkSettings": {
					"Ports": {"6379/tcp": [{"HostIP": "0.0.0.0", "HostPort": "6379"}]},
					"Networks": {"custom-net": {"IPAddress": "10.0.0.5", "Gateway": "10.0.0.1"}}
				},
				"Mounts": [
					{"Source": "/var/lib/docker/volumes/redis-data/_data", "Destination": "/data", "RW": true, "Driver": "local"}
				]
			}`))
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer server.Close()

	client, err := api.NewClient(server.URL)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	cm := &Docker{
		client:       client,
		containers:   make(map[string]*container.Container),
		needsRefresh: make(chan string, 10),
		statuses:     make(chan StatusUpdate, 10),
		closed:       make(chan struct{}),
	}
	defer cm.Close()

	c := cm.MustGet("c_full")
	cm.refresh(c)

	if c.GetMeta("name") != "full-container" {
		t.Errorf("unexpected name: %s", c.GetMeta("name"))
	}
	if c.GetMeta("composeProject") != "myproject" || c.GetMeta("composeService") != "redis" {
		t.Errorf("unexpected compose meta: project=%s service=%s", c.GetMeta("composeProject"), c.GetMeta("composeService"))
	}
	if c.GetMeta("exitCode") != "137" || c.GetMeta("oomKilled") != "true" {
		t.Errorf("unexpected state meta: exitCode=%s oom=%s", c.GetMeta("exitCode"), c.GetMeta("oomKilled"))
	}
	if c.GetMeta("memLimit") != "1024 MB" || c.GetMeta("cpuLimit") != "2.00 CPUs" {
		t.Errorf("unexpected limits: mem=%s cpu=%s", c.GetMeta("memLimit"), c.GetMeta("cpuLimit"))
	}
	if c.GetMeta("healthStatus") != "unhealthy" {
		t.Errorf("unexpected health: %s", c.GetMeta("healthStatus"))
	}

	cm.refreshAll()
}
