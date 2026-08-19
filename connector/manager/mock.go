package manager

import "github.com/edsilegx/ctop/models"

type Mock struct{}

func NewMock() *Mock {
	return &Mock{}
}

func (m *Mock) Start() error {
	return ErrActionNotImpl
}

func (m *Mock) Stop() error {
	return ErrActionNotImpl
}

func (m *Mock) Remove() error {
	return ErrActionNotImpl
}

func (m *Mock) Pause() error {
	return ErrActionNotImpl
}

func (m *Mock) Unpause() error {
	return ErrActionNotImpl
}

func (m *Mock) Restart() error {
	return ErrActionNotImpl
}

func (m *Mock) Exec(cmd []string) error {
	return ErrActionNotImpl
}

func (m *Mock) Kill(signal string) error {
	return nil
}

func (m *Mock) Top(args string) (models.TopResult, error) {
	return models.TopResult{
		Titles: []string{"UID", "PID", "PPID", "C", "STIME", "TTY", "TIME", "CMD"},
		Processes: [][]string{
			{"root", "1234", "1", "0.0", "12:00", "?", "00:00:01", "/app/server --port=8080"},
			{"root", "1235", "1234", "0.2", "12:00", "?", "00:00:05", "worker-thread"},
		},
	}, nil
}

func (m *Mock) Changes() ([]models.Change, error) {
	return []models.Change{
		{Path: "/app/config.json", Kind: 0},
		{Path: "/tmp/app.log", Kind: 1},
		{Path: "/var/cache/old.tmp", Kind: 2},
	}, nil
}

func (m *Mock) ReadDir(dirPath string) ([]models.FileInfo, error) {
	if dirPath == "/" || dirPath == "" {
		return []models.FileInfo{
			{Name: "app", Path: "/app", IsDir: true, Size: 4096, Mode: "drwxr-xr-x", ModTime: "2026-08-18 12:00:00"},
			{Name: "etc", Path: "/etc", IsDir: true, Size: 4096, Mode: "drwxr-xr-x", ModTime: "2026-08-18 12:00:00"},
			{Name: "var", Path: "/var", IsDir: true, Size: 4096, Mode: "drwxr-xr-x", ModTime: "2026-08-18 12:00:00"},
		}, nil
	}
	if dirPath == "/app" {
		return []models.FileInfo{
			{Name: "config.json", Path: "/app/config.json", IsDir: false, Size: 154, Mode: "-rw-r--r--", ModTime: "2026-08-18 12:00:00"},
			{Name: "server", Path: "/app/server", IsDir: false, Size: 12582912, Mode: "-rwxr-xr-x", ModTime: "2026-08-18 12:00:00"},
		}, nil
	}
	return []models.FileInfo{}, nil
}

func (m *Mock) ReadFile(filePath string, maxBytes int64) (string, error) {
	if filePath == "/app/config.json" {
		return "{\n  \"port\": 8080,\n  \"env\": \"production\",\n  \"metrics\": true\n}\n", nil
	}
	return "sample file content", nil
}

func (m *Mock) Download(srcPath, dstPath string) (int64, error) {
	return 154, nil
}

func (m *Mock) Upload(srcHostPath, dstContainerPath string) error {
	return nil
}

func (m *Mock) UpdateResources(memoryMB int64, cpus float64, restartPolicy string) error {
	return nil
}


