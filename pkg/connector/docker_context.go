// Package connector implements container runtime connectors (Docker, runC, Mock).
package connector

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

type dockerConfigFile struct {
	CurrentContext string `json:"currentContext"`
}

type contextMetaFile struct {
	Name      string `json:"Name"`
	Endpoints struct {
		Docker struct {
			Host string `json:"Host"`
		} `json:"docker"`
	} `json:"Endpoints"`
}

// ResolveDockerEndpoint determines the Docker daemon host endpoint following Docker CLI context resolution:
// 1. DOCKER_HOST env var
// 2. DOCKER_CONTEXT env var
// 3. ~/.docker/config.json currentContext
// 4. Rootless Docker socket discovery ($XDG_RUNTIME_DIR/docker.sock or /run/user/<uid>/docker.sock)
// 5. Fallback to default client
func ResolveDockerEndpoint() string {
	if host := os.Getenv("DOCKER_HOST"); strings.TrimSpace(host) != "" {
		return host
	}

	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return resolveDefaultSocket()
	}

	contextName := strings.TrimSpace(os.Getenv("DOCKER_CONTEXT"))
	if contextName == "" {
		cfgPath := filepath.Clean(filepath.Join(home, ".docker", "config.json"))
		if data, err := os.ReadFile(cfgPath); err == nil {
			var cfg dockerConfigFile
			if err := json.Unmarshal(data, &cfg); err == nil {
				contextName = strings.TrimSpace(cfg.CurrentContext)
			}
		}
	}

	if contextName == "" || contextName == "default" {
		return resolveDefaultSocket()
	}

	hash := sha256.Sum256([]byte(contextName))
	dirName := hex.EncodeToString(hash[:])

	metaPath := filepath.Clean(filepath.Join(home, ".docker", "contexts", "meta", dirName, "meta.json"))
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return resolveDefaultSocket()
	}

	var meta contextMetaFile
	if err := json.Unmarshal(data, &meta); err != nil {
		return resolveDefaultSocket()
	}

	if ep := strings.TrimSpace(meta.Endpoints.Docker.Host); ep != "" {
		return ep
	}

	return resolveDefaultSocket()
}

// resolveDefaultSocket checks if standard /var/run/docker.sock exists;
// if not on Linux, it checks for rootless Docker sockets ($XDG_RUNTIME_DIR/docker.sock or /run/user/<uid>/docker.sock).
func resolveDefaultSocket() string {
	if runtime.GOOS != "linux" {
		return ""
	}

	// 1. If standard /var/run/docker.sock exists, let client use standard default
	if _, err := os.Stat("/var/run/docker.sock"); err == nil {
		return ""
	}

	// 2. Check XDG_RUNTIME_DIR rootless socket (e.g. /run/user/1000/docker.sock)
	if runtimeDir := os.Getenv("XDG_RUNTIME_DIR"); strings.TrimSpace(runtimeDir) != "" {
		runtimeDir = filepath.Clean(strings.TrimSpace(runtimeDir))
		if filepath.IsAbs(runtimeDir) {
			sock := filepath.Join(runtimeDir, "docker.sock")
			// #nosec G304,G703 -- sock is validated clean absolute path to rootless socket
			if _, err := os.Stat(sock); err == nil {
				return "unix://" + sock
			}
		}
	}

	// 3. Check /run/user/<uid>/docker.sock if XDG_RUNTIME_DIR is not exported
	uid := os.Getuid()
	if uid > 0 {
		sock := filepath.Join("/run", "user", strconv.Itoa(uid), "docker.sock")
		if _, err := os.Stat(sock); err == nil {
			return "unix://" + filepath.Clean(sock)
		}
	}

	return ""
}
