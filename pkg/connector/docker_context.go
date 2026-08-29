// Package connector implements container runtime connectors (Docker, runC, Mock).
package connector

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
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
// 4. Fallback to default client
func ResolveDockerEndpoint() string {
	if host := os.Getenv("DOCKER_HOST"); strings.TrimSpace(host) != "" {
		return host
	}

	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
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
		return ""
	}

	hash := sha256.Sum256([]byte(contextName))
	dirName := hex.EncodeToString(hash[:])

	metaPath := filepath.Clean(filepath.Join(home, ".docker", "contexts", "meta", dirName, "meta.json"))
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return ""
	}

	var meta contextMetaFile
	if err := json.Unmarshal(data, &meta); err != nil {
		return ""
	}

	return strings.TrimSpace(meta.Endpoints.Docker.Host)
}
