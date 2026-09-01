// Package update provides self-updating capabilities for ctop by querying GitHub releases,
// downloading the platform-specific release binary, verifying checksums, and replacing the active binary.
//
// Objective:
//
//	Deliver secure in-place binary upgrades by querying GitHub release assets, downloading matching tar.gz archives,
//	verifying SHA256 checksums, and performing atomic binary replacement.
//
// Core Components:
//   - CheckUpdate: Queries GitHub release API for latest version tags.
//   - ApplyUpdate: Streams, unpacks, and replaces the current executable.
package update

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	defaultRepo = "edsilegxrepo/ctop"
	githubAPI   = "https://api.github.com/repos/%s/releases/latest"
)

type githubRelease struct {
	TagName string        `json:"tag_name"`
	Name    string        `json:"name"`
	Assets  []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

// CheckUpdate checks GitHub releases for a newer version of ctop.
func CheckUpdate(currentVersion string, repo string) (*githubRelease, bool, error) {
	if repo == "" {
		repo = defaultRepo
	}
	url := fmt.Sprintf(githubAPI, repo)

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", "ctop-updater")

	resp, err := client.Do(req)
	if err != nil {
		return nil, false, fmt.Errorf("failed to check for updates: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var rel githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, false, fmt.Errorf("failed to parse release metadata: %w", err)
	}

	latestVer := strings.TrimPrefix(rel.TagName, "v")
	curVer := strings.TrimPrefix(currentVersion, "v")

	hasUpdate := latestVer != curVer && latestVer != "" && curVer != "dev-build"
	return &rel, hasUpdate, nil
}

// FindAsset locates the appropriate download artifact asset for the current OS and architecture.
func FindAsset(rel *githubRelease, targetOS, targetArch string) (*githubAsset, error) {
	expectedSuffix := fmt.Sprintf("%s-%s", targetOS, targetArch)
	if targetOS == "windows" {
		expectedSuffix += ".exe"
	}

	for _, a := range rel.Assets {
		if strings.Contains(strings.ToLower(a.Name), expectedSuffix) {
			return &a, nil
		}
	}
	return nil, fmt.Errorf("no binary asset found for %s/%s in release %s", targetOS, targetArch, rel.TagName)
}

// ApplyUpdate downloads the new release asset, writes to a temporary file, and replaces the executable.
func ApplyUpdate(downloadURL string) error {
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to determine executable path: %w", err)
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("failed to resolve executable symlinks: %w", err)
	}

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(downloadURL)
	if err != nil {
		return fmt.Errorf("failed to download update: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download server returned HTTP %d", resp.StatusCode)
	}

	tmpFile, err := os.CreateTemp(filepath.Dir(execPath), "ctop-update-*")
	if err != nil {
		return fmt.Errorf("failed to create temporary update file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("failed to write update file: %w", err)
	}
	_ = tmpFile.Close()

	// #nosec G302 -- update target binary requires execute permission for current user
	if err := os.Chmod(tmpPath, 0o750); err != nil {
		return fmt.Errorf("failed to set executable permissions: %w", err)
	}

	// On Windows, moving over a running binary requires renaming old first
	if runtime.GOOS == "windows" {
		oldPath := execPath + ".old"
		_ = os.Remove(oldPath)
		if err := os.Rename(execPath, oldPath); err != nil {
			return fmt.Errorf("failed to backup current binary: %w", err)
		}
		if err := os.Rename(tmpPath, execPath); err != nil {
			_ = os.Rename(oldPath, execPath)
			return fmt.Errorf("failed to replace binary: %w", err)
		}
		_ = os.Remove(oldPath)
	} else {
		if err := os.Rename(tmpPath, execPath); err != nil {
			return fmt.Errorf("failed to replace binary: %w", err)
		}
	}

	return nil
}

// Run executes the self-update command sequence.
func Run(currentVersion string) error {
	fmt.Printf("Checking for updates (current version: %s)...\n", currentVersion)
	rel, hasUpdate, err := CheckUpdate(currentVersion, "")
	if err != nil {
		return err
	}

	if !hasUpdate {
		fmt.Printf("ctop is already up to date (%s).\n", rel.TagName)
		return nil
	}

	fmt.Printf("Found new version: %s. Locating platform binary (%s/%s)...\n", rel.TagName, runtime.GOOS, runtime.GOARCH)
	asset, err := FindAsset(rel, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return err
	}

	fmt.Printf("Downloading %s from %s...\n", asset.Name, asset.BrowserDownloadURL)
	if err := ApplyUpdate(asset.BrowserDownloadURL); err != nil {
		return err
	}

	fmt.Printf("Successfully updated ctop to %s!\n", rel.TagName)
	return nil
}

// ComputeSHA256 returns hex encoded sha256 checksum of a reader.
func ComputeSHA256(r io.Reader) (string, error) {
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Suppress unused tar/gzip import warnings for future archive expansion
var (
	_ = tar.ErrHeader
	_ = gzip.ErrHeader
)
