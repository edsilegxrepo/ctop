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
	"sync"
	"time"

	"github.com/edsilegx/ctop/pkg/models"
	api "github.com/fsouza/go-dockerclient"
	"github.com/pkg/errors"
)

type dirCacheEntry struct {
	entries []models.FileInfo
	exp     time.Time
}

type diffCacheEntry struct {
	changes []models.Change
	exp     time.Time
}

var (
	dirCacheLock  sync.RWMutex
	dirCache      = make(map[string]dirCacheEntry)
	diffCacheLock sync.RWMutex
	diffCache     = make(map[string]diffCacheEntry)
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
	targetArgs := args
	if targetArgs == "" {
		targetArgs = "aux"
	}
	res, err := dc.client.TopContainer(dc.id, targetArgs)
	if err != nil && targetArgs != "" {
		res, err = dc.client.TopContainer(dc.id, "")
	}
	if err != nil {
		return models.TopResult{
			Titles:    []string{"PID", "USER", "%CPU", "%MEM", "COMMAND"},
			Processes: [][]string{},
		}, err
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

	diffCacheLock.RLock()
	if entry, ok := diffCache[dc.id]; ok && time.Now().Before(entry.exp) {
		diffCacheLock.RUnlock()
		return entry.changes, nil
	}
	diffCacheLock.RUnlock()

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

	diffCacheLock.Lock()
	diffCache[dc.id] = diffCacheEntry{
		changes: out,
		exp:     time.Now().Add(10 * time.Second),
	}
	diffCacheLock.Unlock()

	return out, nil
}

func (dc *Docker) ReadDir(dirPath string) ([]models.FileInfo, error) {
	if dc.client == nil {
		return nil, fmt.Errorf("docker client is nil")
	}
	if dirPath == "" {
		dirPath = "/"
	}
	if !strings.HasPrefix(dirPath, "/") || strings.Contains(dirPath, "..") {
		return nil, fmt.Errorf("security violation: directory path must be an absolute path without relative components: %q", dirPath)
	}
	cleanPath := path.Clean(dirPath)

	cacheKey := dc.id + ":" + cleanPath
	dirCacheLock.RLock()
	if cEntry, ok := dirCache[cacheKey]; ok && time.Now().Before(cEntry.exp) {
		dirCacheLock.RUnlock()
		return cEntry.entries, nil
	}
	dirCacheLock.RUnlock()

	// 1. Fast path: non-recursive in-container ls (runs in ~10ms vs tar dump)
	if entries, err := dc.readDirExec(cleanPath); err == nil && len(entries) > 0 {
		dirCacheLock.Lock()
		dirCache[cacheKey] = dirCacheEntry{
			entries: entries,
			exp:     time.Now().Add(10 * time.Second),
		}
		dirCacheLock.Unlock()
		return entries, nil
	}

	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		_ = dc.client.DownloadFromContainer(dc.id, api.DownloadFromContainerOptions{
			Path:         cleanPath,
			OutputStream: pw,
		})
	}()

	tr := tar.NewReader(pr)
	var entries []models.FileInfo
	seen := make(map[string]bool)
	headerCount := 0

	for {
		header, err := tr.Next()
		if err == io.EOF || err != nil {
			break
		}
		headerCount++
		if headerCount > 300 && len(entries) > 0 {
			break
		}

		cleanHeaderName := path.Clean(strings.TrimPrefix(header.Name, "./"))
		cleanHeaderName = strings.TrimPrefix(cleanHeaderName, "/")
		if cleanHeaderName == "." || cleanHeaderName == "" || strings.HasPrefix(cleanHeaderName, "..") {
			continue
		}

		hClean := cleanHeaderName

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
	_ = pr.Close()

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})

	dirCacheLock.Lock()
	dirCache[cacheKey] = dirCacheEntry{
		entries: entries,
		exp:     time.Now().Add(5 * time.Second),
	}
	dirCacheLock.Unlock()

	return entries, nil
}

func (dc *Docker) readDirExec(cleanPath string) ([]models.FileInfo, error) {
	if dc.client == nil {
		return nil, fmt.Errorf("docker client is nil")
	}

	cmd := []string{"/bin/sh", "-c", fmt.Sprintf("ls -lpa %q 2>/dev/null", cleanPath)}
	execCmd, err := dc.client.CreateExec(api.CreateExecOptions{
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          cmd,
		Container:    dc.id,
	})
	if err != nil {
		return nil, err
	}

	var stdout bytes.Buffer
	err = dc.client.StartExec(execCmd.ID, api.StartExecOptions{
		OutputStream: &stdout,
		RawTerminal:  false,
	})
	if err != nil {
		return nil, err
	}

	output := stdout.String()
	if strings.TrimSpace(output) == "" {
		return nil, fmt.Errorf("empty exec output")
	}

	lines := strings.Split(output, "\n")
	var entries []models.FileInfo
	seen := make(map[string]bool)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "total ") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 8 {
			continue
		}

		mode := fields[0]
		name := strings.Join(fields[8:], " ")
		if name == "" {
			name = fields[len(fields)-1]
		}
		if idx := strings.Index(name, " -> "); idx != -1 {
			name = name[:idx]
		}

		cleanName := strings.TrimSuffix(name, "/")
		if cleanName == "." || cleanName == ".." || cleanName == "" {
			continue
		}

		fullChildPath := path.Join(cleanPath, cleanName)
		if seen[fullChildPath] {
			continue
		}
		seen[fullChildPath] = true

		isDir := strings.HasPrefix(mode, "d") || strings.HasSuffix(name, "/")
		var size int64
		if sz, err := strconv.ParseInt(fields[4], 10, 64); err == nil {
			size = sz
		}

		modTime := strings.Join(fields[5:8], " ")

		entries = append(entries, models.FileInfo{
			Name:    cleanName,
			Path:    fullChildPath,
			IsDir:   isDir,
			Size:    size,
			Mode:    mode,
			ModTime: modTime,
		})
	}

	if len(entries) == 0 {
		return nil, fmt.Errorf("no entries parsed from ls")
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

	if !strings.HasPrefix(filePath, "/") || strings.Contains(filePath, "..") {
		return "", fmt.Errorf("security violation: file path must be an absolute path without relative components: %q", filePath)
	}
	cleanFilePath := path.Clean(filePath)

	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		_ = dc.client.DownloadFromContainer(dc.id, api.DownloadFromContainerOptions{
			Path:         cleanFilePath,
			OutputStream: pw,
		})
	}()

	tr := tar.NewReader(pr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			_ = pr.Close()
			return "", err
		}

		if header.Typeflag == tar.TypeReg {
			content, err := io.ReadAll(io.LimitReader(tr, maxBytes))
			_ = pr.Close()
			if err != nil {
				return "", err
			}
			return string(content), nil
		}
	}

	_ = pr.Close()
	return "", fmt.Errorf("file not found in archive")
}

func (dc *Docker) SearchFiles(basePath, pattern string, maxResults int) ([]models.FileInfo, error) {
	if dc.client == nil {
		return nil, fmt.Errorf("docker client is nil")
	}
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return nil, fmt.Errorf("search pattern cannot be empty")
	}
	if basePath == "" {
		basePath = "/"
	}
	if !strings.HasPrefix(basePath, "/") || strings.Contains(basePath, "..") {
		return nil, fmt.Errorf("security violation: base path must be an absolute path without relative components: %q", basePath)
	}
	cleanBasePath := path.Clean(basePath)
	if maxResults <= 0 || maxResults > 200 {
		maxResults = 100
	}

	// Filter out dangerous characters from search pattern for shell execution safety
	safePattern := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
			r == '.' || r == '_' || r == '-' || r == '*' || r == '?' {
			return r
		}
		return -1
	}, pattern)

	if safePattern == "" {
		return nil, fmt.Errorf("invalid search pattern")
	}

	// Determine branch targets for multi-threaded parallel execution
	var targetBranches []string
	if cleanBasePath == "/" {
		// Parallel branch groups covering root filesystem
		targetBranches = []string{
			"/etc /opt /srv",
			"/var /tmp",
			"/app /home /root",
			"/usr/local /usr/share /usr/lib",
			"/bin /sbin /usr/bin /usr/sbin",
		}
	} else {
		targetBranches = []string{cleanBasePath}
	}

	// 1. Parallel in-container search workers
	var (
		wg         sync.WaitGroup
		mu         sync.Mutex
		seen       = make(map[string]bool)
		allEntries []models.FileInfo
	)

	for _, branchGroup := range targetBranches {
		wg.Add(1)
		go func(targets string) {
			defer wg.Done()

			// Early return if enough items have already been collected
			mu.Lock()
			if len(allEntries) >= maxResults {
				mu.Unlock()
				return
			}
			mu.Unlock()

			cmdStr := fmt.Sprintf("find %s -maxdepth 6 -iname %q -exec ls -ld {} + 2>/dev/null | head -n %d", targets, "*"+safePattern+"*", maxResults)
			cmd := []string{"sh", "-c", cmdStr}

			execCmd, err := dc.client.CreateExec(api.CreateExecOptions{
				AttachStdout: true,
				AttachStderr: false,
				Cmd:          cmd,
				Container:    dc.id,
			})
			if err != nil {
				return
			}

			var stdout bytes.Buffer
			err = dc.client.StartExec(execCmd.ID, api.StartExecOptions{
				OutputStream: &stdout,
				RawTerminal:  false,
			})
			if err != nil || strings.TrimSpace(stdout.String()) == "" {
				return
			}

			lines := strings.Split(stdout.String(), "\n")
			var localEntries []models.FileInfo

			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line == "" || strings.HasPrefix(line, "total ") {
					continue
				}
				fields := strings.Fields(line)
				if len(fields) < 8 {
					continue
				}
				mode := fields[0]
				fullPath := strings.Join(fields[8:], " ")
				if idx := strings.Index(fullPath, " -> "); idx != -1 {
					fullPath = fullPath[:idx]
				}
				fullPath = strings.TrimSpace(fullPath)
				if fullPath == "" {
					continue
				}

				var size int64
				if sz, err := strconv.ParseInt(fields[4], 10, 64); err == nil {
					size = sz
				}
				modTime := strings.Join(fields[5:8], " ")
				isDir := strings.HasPrefix(mode, "d")

				localEntries = append(localEntries, models.FileInfo{
					Name:    path.Base(fullPath),
					Path:    fullPath,
					IsDir:   isDir,
					Size:    size,
					Mode:    mode,
					ModTime: modTime,
				})
			}

			if len(localEntries) > 0 {
				mu.Lock()
				for _, ent := range localEntries {
					if !seen[ent.Path] && len(allEntries) < maxResults {
						seen[ent.Path] = true
						allEntries = append(allEntries, ent)
					}
				}
				mu.Unlock()
			}
		}(branchGroup)
	}

	wg.Wait()

	if len(allEntries) > 0 {
		sort.Slice(allEntries, func(i, j int) bool {
			if allEntries[i].IsDir != allEntries[j].IsDir {
				return allEntries[i].IsDir
			}
			return strings.ToLower(allEntries[i].Path) < strings.ToLower(allEntries[j].Path)
		})
		return allEntries, nil
	}

	// 2. Fallback: match against layer diffs (for stopped/non-shell containers)
	changes, err := dc.Changes()
	if err == nil && len(changes) > 0 {
		var entries []models.FileInfo
		lowerPat := strings.ToLower(safePattern)
		for _, ch := range changes {
			if strings.Contains(strings.ToLower(ch.Path), lowerPat) {
				entries = append(entries, models.FileInfo{
					Name:  path.Base(ch.Path),
					Path:  ch.Path,
					IsDir: false,
					Size:  0,
					Mode:  "-rw-r--r--",
				})
				if len(entries) >= maxResults {
					break
				}
			}
		}
		if len(entries) > 0 {
			return entries, nil
		}
	}

	return []models.FileInfo{}, nil
}

func (dc *Docker) Download(srcPath, dstPath string) (int64, error) {
	if dc.client == nil {
		return 0, fmt.Errorf("docker client is nil")
	}
	if srcPath == "" {
		return 0, fmt.Errorf("source path is required")
	}
	if !strings.HasPrefix(srcPath, "/") || strings.Contains(srcPath, "..") {
		return 0, fmt.Errorf("security violation: source path must be an absolute container path without relative components: %q", srcPath)
	}
	cleanSrc := path.Clean(srcPath)
	if dstPath == "" {
		dstPath = path.Base(cleanSrc)
	}

	var buf bytes.Buffer
	err := dc.client.DownloadFromContainer(dc.id, api.DownloadFromContainerOptions{
		Path:         cleanSrc,
		OutputStream: &buf,
	})
	if err != nil {
		return 0, err
	}

	tr := tar.NewReader(&buf)
	var totalBytes int64

	destBase, err := filepath.Abs(filepath.Clean(dstPath))
	if err != nil {
		return 0, fmt.Errorf("invalid destination path: %w", err)
	}

	var isDestDir bool
	if fi, err := os.Stat(destBase); err == nil && fi.IsDir() {
		isDestDir = true
	}

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return totalBytes, err
		}

		// Disallow unsafe links/symlinks escaping root
		if header.Typeflag == tar.TypeSymlink || header.Typeflag == tar.TypeLink {
			continue
		}

		cleanHeaderName := filepath.Clean(filepath.ToSlash(header.Name))
		if strings.HasPrefix(cleanHeaderName, "..") || strings.HasPrefix(cleanHeaderName, "/") {
			return totalBytes, fmt.Errorf("security violation: illegal path traversal in tar archive %q", header.Name)
		}

		var target string
		if isDestDir {
			target = filepath.Join(destBase, filepath.Base(cleanHeaderName))
		} else {
			if header.Typeflag == tar.TypeDir {
				continue
			}
			target = destBase
		}
		target = filepath.Clean(target)

		// Strict Zip Slip validation: target MUST stay within destination parent or destBase
		rel, err := filepath.Rel(filepath.Dir(destBase), target)
		if err != nil || strings.HasPrefix(rel, "..") {
			return totalBytes, fmt.Errorf("security violation: illegal archive destination escape %q", header.Name)
		}
		if isDestDir {
			relDir, err := filepath.Rel(destBase, target)
			if err != nil || strings.HasPrefix(relDir, "..") {
				return totalBytes, fmt.Errorf("security violation: illegal archive destination escape %q", header.Name)
			}
		}

		if header.Typeflag == tar.TypeDir {
			if err := os.MkdirAll(target, 0o750); err != nil {
				return totalBytes, err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			return totalBytes, err
		}

		if header.Mode < 0 || header.Mode > math.MaxUint32 {
			header.Mode = 0o600
		}
		mode := os.FileMode(uint32(header.Mode) & 0o777) // #nosec G115 -- header.Mode bounded
		if mode == 0 {
			mode = 0o600
		}

		outFile, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
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
	absSrc, err := filepath.Abs(filepath.Clean(srcHostPath))
	if err != nil {
		return fmt.Errorf("invalid source host path: %w", err)
	}
	if dstContainerPath == "" {
		dstContainerPath = "/"
	}
	cleanDst := path.Clean(dstContainerPath)
	if !strings.HasPrefix(cleanDst, "/") || strings.Contains(cleanDst, "..") {
		return fmt.Errorf("security violation: destination container path must be an absolute path without relative components: %q", dstContainerPath)
	}

	cleanSrc := absSrc
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
		if cleanDst == "/" || strings.HasSuffix(dstContainerPath, "/") {
			header.Name = srcStat.Name()
		} else {
			header.Name = path.Base(cleanDst)
			cleanDst = path.Dir(cleanDst)
		}
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
		Path:        cleanDst,
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
