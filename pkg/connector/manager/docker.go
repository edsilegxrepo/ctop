package manager

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"math"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/edsilegx/ctop/pkg/models"
	api "github.com/fsouza/go-dockerclient"
	"github.com/pkg/errors"
)

type Docker struct {
	id     string
	client *api.Client
}

func NewDocker(client *api.Client, id string) *Docker {
	return &Docker{
		id:     id,
		client: client,
	}
}

// Do not allow to close reader (i.e. /dev/stdin which docker client tries to close after command execution)
type noClosableReader struct {
	io.Reader
}

func (w *noClosableReader) Read(p []byte) (n int, err error) {
	return w.Reader.Read(p)
}

const (
	STDIN  = 0
	STDOUT = 1
	STDERR = 2
)

var wrongFrameFormat = errors.New("Wrong frame format")

// A frame has a Header and a Payload
// Header: [8]byte{STREAM_TYPE, 0, 0, 0, SIZE1, SIZE2, SIZE3, SIZE4}
// STREAM_TYPE can be:
//
//	0: stdin (is written on stdout)
//	1: stdout
//	2: stderr
//
// SIZE1, SIZE2, SIZE3, SIZE4 are the four bytes of the uint32 size encoded as big endian.
// But we don't use size, because we don't need to find the end of frame.
type frameWriter struct {
	stdout io.Writer
	stderr io.Writer
	stdin  io.Writer
}

func (w *frameWriter) Write(p []byte) (n int, err error) {
	// drop initial empty frames
	if len(p) == 0 {
		return 0, nil
	}

	if len(p) > 8 {
		var targetWriter io.Writer
		switch p[0] {
		case STDIN:
			targetWriter = w.stdin
		case STDOUT:
			targetWriter = w.stdout
		case STDERR:
			targetWriter = w.stderr
		default:
			return 0, wrongFrameFormat
		}

		n, err := targetWriter.Write(p[8:])
		return n + 8, err
	}

	return 0, wrongFrameFormat
}

func (dc *Docker) Exec(cmd []string) error {
	if dc.client == nil {
		return fmt.Errorf("docker client is nil")
	}
	execCmd, err := dc.client.CreateExec(api.CreateExecOptions{
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          cmd,
		Container:    dc.id,
		Tty:          true,
	})
	if err != nil {
		return err
	}

	return dc.client.StartExec(execCmd.ID, api.StartExecOptions{
		InputStream:  &noClosableReader{os.Stdin},
		OutputStream: &frameWriter{os.Stdout, os.Stderr, os.Stdin},
		ErrorStream:  os.Stderr,
		RawTerminal:  true,
	})
}

func (dc *Docker) Start() error {
	if dc.client == nil {
		return fmt.Errorf("docker client is nil")
	}
	opts := api.InspectContainerOptions{ID: dc.id}
	c, err := dc.client.InspectContainerWithOptions(opts)
	if err != nil {
		return fmt.Errorf("cannot inspect container: %v", err)
	}

	if err := dc.client.StartContainer(c.ID, c.HostConfig); err != nil {
		return fmt.Errorf("cannot start container: %v", err)
	}
	return nil
}

func (dc *Docker) Stop() error {
	if dc.client == nil {
		return fmt.Errorf("docker client is nil")
	}
	if err := dc.client.StopContainer(dc.id, 3); err != nil {
		return fmt.Errorf("cannot stop container: %v", err)
	}
	return nil
}

func (dc *Docker) Remove() error {
	if dc.client == nil {
		return fmt.Errorf("docker client is nil")
	}
	if err := dc.client.RemoveContainer(api.RemoveContainerOptions{ID: dc.id}); err != nil {
		return fmt.Errorf("cannot remove container: %v", err)
	}
	return nil
}

func (dc *Docker) Pause() error {
	if dc.client == nil {
		return fmt.Errorf("docker client is nil")
	}
	if err := dc.client.PauseContainer(dc.id); err != nil {
		return fmt.Errorf("cannot pause container: %v", err)
	}
	return nil
}

func (dc *Docker) Unpause() error {
	if dc.client == nil {
		return fmt.Errorf("docker client is nil")
	}
	if err := dc.client.UnpauseContainer(dc.id); err != nil {
		return fmt.Errorf("cannot unpause container: %v", err)
	}
	return nil
}

func (dc *Docker) Restart() error {
	if dc.client == nil {
		return fmt.Errorf("docker client is nil")
	}
	if err := dc.client.RestartContainer(dc.id, 3); err != nil {
		return fmt.Errorf("cannot restart container: %v", err)
	}
	return nil
}

var signalMap = map[string]api.Signal{
	"SIGHUP":   api.Signal(1),
	"SIGINT":   api.Signal(2),
	"SIGQUIT":  api.Signal(3),
	"SIGKILL":  api.Signal(9),
	"SIGUSR1":  api.Signal(10),
	"SIGUSR2":  api.Signal(12),
	"SIGTERM":  api.Signal(15),
	"SIGSTOP":  api.Signal(19),
	"SIGWINCH": api.Signal(28),
}

func (dc *Docker) Kill(signal string) error {
	if dc.client == nil {
		return fmt.Errorf("docker client is nil")
	}
	sigUpper := strings.ToUpper(strings.TrimSpace(signal))
	sig, ok := signalMap[sigUpper]
	if !ok {
		if !strings.HasPrefix(sigUpper, "SIG") {
			sig, ok = signalMap["SIG"+sigUpper]
		}
		if !ok {
			if num, err := strconv.Atoi(sigUpper); err == nil {
				sig = api.Signal(num)
			} else {
				sig = api.SIGKILL
			}
		}
	}
	return dc.client.KillContainer(api.KillContainerOptions{
		ID:     dc.id,
		Signal: sig,
	})
}

func (dc *Docker) Top(args string) (models.TopResult, error) {
	if dc.client == nil {
		return models.TopResult{}, fmt.Errorf("docker client is nil")
	}
	if args == "" {
		args = "aux"
	}
	res, err := dc.client.TopContainer(dc.id, args)
	if err != nil {
		return models.TopResult{}, err
	}
	return models.TopResult{
		Titles:    res.Titles,
		Processes: res.Processes,
	}, nil
}

func (dc *Docker) Changes() ([]models.Change, error) {
	if dc.client == nil {
		return nil, fmt.Errorf("docker client is nil")
	}
	changes, err := dc.client.ContainerChanges(dc.id)
	if err != nil {
		return nil, err
	}
	out := make([]models.Change, 0, len(changes))
	for _, ch := range changes {
		out = append(out, models.Change{
			Path: ch.Path,
			Kind: int(ch.Kind),
		})
	}
	return out, nil
}

func (dc *Docker) ReadDir(dirPath string) ([]models.FileInfo, error) {
	if dc.client == nil {
		return nil, fmt.Errorf("docker client is nil")
	}
	if dirPath == "" {
		dirPath = "/"
	}
	cleanPath := path.Clean(dirPath)
	if !strings.HasPrefix(cleanPath, "/") {
		cleanPath = "/" + cleanPath
	}

	var buf bytes.Buffer
	err := dc.client.DownloadFromContainer(dc.id, api.DownloadFromContainerOptions{
		Path:         cleanPath,
		OutputStream: &buf,
	})
	if err != nil {
		return nil, err
	}

	tr := tar.NewReader(&buf)
	var entries []models.FileInfo
	seen := make(map[string]bool)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		hName := strings.TrimPrefix(header.Name, "./")
		hName = strings.TrimPrefix(hName, "/")
		hClean := path.Clean(hName)
		if hClean == "." || hClean == "" {
			continue
		}

		baseFolder := strings.TrimPrefix(cleanPath, "/")
		rel := hClean
		if baseFolder != "" && baseFolder != "." {
			if hClean == baseFolder {
				continue
			}
			if strings.HasPrefix(hClean, baseFolder+"/") {
				rel = strings.TrimPrefix(hClean, baseFolder+"/")
			}
		}

		if strings.Contains(rel, "/") {
			parts := strings.Split(rel, "/")
			childDir := parts[0]
			fullChildPath := path.Join(cleanPath, childDir)
			if !seen[fullChildPath] {
				seen[fullChildPath] = true
				entries = append(entries, models.FileInfo{
					Name:    childDir,
					Path:    fullChildPath,
					IsDir:   true,
					Size:    4096,
					Mode:    "drwxr-xr-x",
					ModTime: header.ModTime.Format("2006-01-02 15:04:05"),
				})
			}
			continue
		}

		fullChildPath := path.Join(cleanPath, rel)
		if seen[fullChildPath] {
			continue
		}
		seen[fullChildPath] = true

		isDir := header.Typeflag == tar.TypeDir
		if header.Mode < 0 || header.Mode > math.MaxUint32 {
			header.Mode = 0o644
		}
		modeStr := os.FileMode(uint32(header.Mode) & 0o7777).String() // #nosec G115 -- header.Mode bounded
		if isDir && !strings.HasPrefix(modeStr, "d") {
			modeStr = "d" + modeStr
		}

		entries = append(entries, models.FileInfo{
			Name:    rel,
			Path:    fullChildPath,
			IsDir:   isDir,
			Size:    header.Size,
			Mode:    modeStr,
			ModTime: header.ModTime.Format("2006-01-02 15:04:05"),
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})

	return entries, nil
}

func (dc *Docker) ReadFile(filePath string, maxBytes int64) (string, error) {
	if dc.client == nil {
		return "", fmt.Errorf("docker client is nil")
	}
	if maxBytes <= 0 {
		maxBytes = 256 * 1024
	}

	var buf bytes.Buffer
	err := dc.client.DownloadFromContainer(dc.id, api.DownloadFromContainerOptions{
		Path:         filePath,
		OutputStream: &buf,
	})
	if err != nil {
		return "", err
	}

	tr := tar.NewReader(&buf)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}

		if header.Typeflag == tar.TypeReg {
			content, err := io.ReadAll(io.LimitReader(tr, maxBytes))
			if err != nil {
				return "", err
			}
			return string(content), nil
		}
	}

	return "", fmt.Errorf("file not found in archive")
}

func (dc *Docker) Download(srcPath, dstPath string) (int64, error) {
	if dc.client == nil {
		return 0, fmt.Errorf("docker client is nil")
	}
	if srcPath == "" {
		return 0, fmt.Errorf("source path is required")
	}
	if dstPath == "" {
		dstPath = path.Base(srcPath)
	}

	var buf bytes.Buffer
	err := dc.client.DownloadFromContainer(dc.id, api.DownloadFromContainerOptions{
		Path:         srcPath,
		OutputStream: &buf,
	})
	if err != nil {
		return 0, err
	}

	tr := tar.NewReader(&buf)
	var totalBytes int64

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return totalBytes, err
		}

		target := dstPath
		if header.Typeflag == tar.TypeDir {
			if err := os.MkdirAll(filepath.Clean(target), 0o750); err != nil {
				return totalBytes, err
			}
			continue
		}

		if fi, err := os.Stat(dstPath); err == nil && fi.IsDir() {
			target = path.Join(dstPath, path.Base(header.Name))
		}

		if err := os.MkdirAll(filepath.Clean(path.Dir(target)), 0o750); err != nil {
			return totalBytes, err
		}

		if header.Mode < 0 || header.Mode > math.MaxUint32 {
			header.Mode = 0o600
		}
		mode := os.FileMode(uint32(header.Mode) & 0o777) // #nosec G115 -- header.Mode bounded
		if mode == 0 {
			mode = 0o600
		}

		outFile, err := os.OpenFile(filepath.Clean(target), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
		if err != nil {
			return totalBytes, err
		}

		n, err := io.CopyN(outFile, tr, 1024*1024*256)
		_ = outFile.Close()
		if err != nil && err != io.EOF {
			return totalBytes, err
		}
		totalBytes += n
	}

	return totalBytes, nil
}

func (dc *Docker) Upload(srcHostPath, dstContainerPath string) error {
	if dc.client == nil {
		return fmt.Errorf("docker client is nil")
	}
	if srcHostPath == "" {
		return fmt.Errorf("source host path is required")
	}
	if dstContainerPath == "" {
		dstContainerPath = "/"
	}

	cleanSrc := filepath.Clean(srcHostPath)
	srcFile, err := os.Open(cleanSrc)
	if err != nil {
		return err
	}
	defer func() { _ = srcFile.Close() }()

	srcStat, err := srcFile.Stat()
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	if srcStat.IsDir() {
		baseDir := filepath.Dir(cleanSrc)
		err = filepath.Walk(cleanSrc, func(curPath string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			relPath, err := filepath.Rel(baseDir, curPath)
			if err != nil {
				return err
			}
			header, err := tar.FileInfoHeader(info, info.Name())
			if err != nil {
				return err
			}
			header.Name = filepath.ToSlash(relPath)
			if err := tw.WriteHeader(header); err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			f, err := os.Open(filepath.Clean(curPath)) // #nosec G122,G304 -- curPath is validated path from filepath.Walk
			if err != nil {
				return err
			}
			defer func() { _ = f.Close() }()
			_, err = io.Copy(tw, f)
			return err
		})
		if err != nil {
			return err
		}
	} else {
		header, err := tar.FileInfoHeader(srcStat, srcStat.Name())
		if err != nil {
			return err
		}
		header.Name = srcStat.Name()
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if _, err := io.Copy(tw, srcFile); err != nil {
			return err
		}
	}

	if err := tw.Close(); err != nil {
		return err
	}

	return dc.client.UploadToContainer(dc.id, api.UploadToContainerOptions{
		Path:        dstContainerPath,
		InputStream: &buf,
	})
}

func (dc *Docker) UpdateResources(memoryMB int64, cpus float64, restartPolicy string) error {
	if dc.client == nil {
		return fmt.Errorf("docker client is nil")
	}
	opts := api.UpdateContainerOptions{}
	if memoryMB > 0 {
		opts.Memory = int(memoryMB * 1024 * 1024)
	}
	if cpus > 0 {
		opts.CPUPeriod = 100000
		opts.CPUQuota = int(cpus * 100000)
	}
	if restartPolicy != "" {
		opts.RestartPolicy = api.RestartPolicy{Name: restartPolicy}
	}
	return dc.client.UpdateContainer(dc.id, opts)
}
