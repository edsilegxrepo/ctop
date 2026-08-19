package manager

import "github.com/edsilegx/ctop/models"

type Runc struct{}

func NewRunc() *Runc {
	return &Runc{}
}

func (rc *Runc) Start() error {
	return ErrActionNotImpl
}

func (rc *Runc) Stop() error {
	return ErrActionNotImpl
}

func (rc *Runc) Remove() error {
	return ErrActionNotImpl
}

func (rc *Runc) Pause() error {
	return ErrActionNotImpl
}

func (rc *Runc) Unpause() error {
	return ErrActionNotImpl
}

func (rc *Runc) Restart() error {
	return ErrActionNotImpl
}

func (rc *Runc) Exec(cmd []string) error {
	return ErrActionNotImpl
}

func (rc *Runc) Kill(signal string) error {
	return ErrActionNotImpl
}

func (rc *Runc) Top(args string) (models.TopResult, error) {
	return models.TopResult{}, ErrActionNotImpl
}

func (rc *Runc) Changes() ([]models.Change, error) {
	return nil, ErrActionNotImpl
}

func (rc *Runc) ReadDir(path string) ([]models.FileInfo, error) {
	return nil, ErrActionNotImpl
}

func (rc *Runc) ReadFile(path string, maxBytes int64) (string, error) {
	return "", ErrActionNotImpl
}

func (rc *Runc) Download(srcPath, dstPath string) (int64, error) {
	return 0, ErrActionNotImpl
}

func (rc *Runc) Upload(srcHostPath, dstContainerPath string) error {
	return ErrActionNotImpl
}

func (rc *Runc) UpdateResources(memoryMB int64, cpus float64, restartPolicy string) error {
	return ErrActionNotImpl
}



