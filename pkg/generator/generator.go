// Package generator produces equivalent docker run CLI commands and docker-compose.yml specifications from container metadata.
//
// Objective:
//
//	Reconstruct reproducible `docker run` command strings and YAML `docker-compose.yml` service definitions
//	from inspected container metadata (ports, volumes, environment variables, restart policies).
//
// Core Components:
//   - GenerateRunCmd: Builds multiline, escaped bash CLI command string.
//   - GenerateCompose: Builds indented YAML compose service specification.
package generator

import (
	"fmt"
	"strings"
)

// MetaGetter abstracts map-like metadata access
type MetaGetter interface {
	Get(key string) string
}

// MapMeta implements MetaGetter on map[string]string
type MapMeta map[string]string

func (m MapMeta) Get(key string) string {
	return m[key]
}

// GenerateRunCmd constructs an equivalent "docker run" CLI command string from container metadata.
func GenerateRunCmd(meta MetaGetter) string {
	var sb strings.Builder
	sb.WriteString("docker run -d")

	name := meta.Get("name")
	if name != "" {
		fmt.Fprintf(&sb, " \\\n  --name %s", name)
	}

	restart := meta.Get("restartPolicy")
	if restart != "" && restart != "no" {
		fmt.Fprintf(&sb, " \\\n  --restart %s", restart)
	}

	if ports := meta.Get("ports"); ports != "" {
		for _, line := range strings.Split(ports, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if strings.Contains(line, " -> ") {
				parts := strings.Split(line, " -> ")
				fmt.Fprintf(&sb, " \\\n  -p %s:%s", parts[0], parts[1])
			} else {
				fmt.Fprintf(&sb, " \\\n  --expose %s", line)
			}
		}
	}

	if mounts := meta.Get("[MOUNTS]"); mounts != "" {
		for _, m := range strings.Split(mounts, ";;") {
			m = strings.TrimSpace(m)
			if m == "" {
				continue
			}
			parts := strings.Split(m, ":::")
			if len(parts) >= 4 {
				dest, src, mode := parts[0], parts[1], parts[3]
				fmt.Fprintf(&sb, " \\\n  -v %s:%s:%s", src, dest, mode)
			}
		}
	}

	if env := meta.Get("[ENV-VAR]"); env != "" {
		for _, e := range strings.Split(env, ";") {
			e = strings.TrimSpace(e)
			if e == "" {
				continue
			}
			fmt.Fprintf(&sb, " \\\n  -e %q", e)
		}
	}

	if mem := meta.Get("memLimit"); mem != "" {
		fmt.Fprintf(&sb, " \\\n  --memory %s", strings.ReplaceAll(mem, " ", ""))
	}
	if cpu := meta.Get("cpuLimit"); cpu != "" {
		fmt.Fprintf(&sb, " \\\n  --cpus %s", strings.Fields(cpu)[0])
	}
	if pids := meta.Get("pidsLimit"); pids != "" {
		fmt.Fprintf(&sb, " \\\n  --pids-limit %s", pids)
	}
	if meta.Get("privileged") == "true" {
		sb.WriteString(" \\\n  --privileged")
	}
	if meta.Get("readonlyRootfs") == "true" {
		sb.WriteString(" \\\n  --read-only")
	}

	image := meta.Get("image")
	if image == "" {
		image = "unknown:latest"
	}
	fmt.Fprintf(&sb, " \\\n  %s", image)

	if cmd := meta.Get("cmd"); cmd != "" {
		fmt.Fprintf(&sb, " %s", cmd)
	}

	return sb.String()
}

// GenerateCompose constructs an equivalent "docker-compose.yml" specification string from container metadata.
func GenerateCompose(meta MetaGetter) string {
	name := meta.Get("name")
	if name == "" {
		name = "app"
	}
	image := meta.Get("image")
	if image == "" {
		image = "unknown:latest"
	}

	var sb strings.Builder
	sb.WriteString("version: '3.8'\n\nservices:\n")
	fmt.Fprintf(&sb, "  %s:\n", name)
	fmt.Fprintf(&sb, "    image: %s\n", image)
	fmt.Fprintf(&sb, "    container_name: %s\n", name)

	if restart := meta.Get("restartPolicy"); restart != "" && restart != "no" {
		fmt.Fprintf(&sb, "    restart: %s\n", restart)
	}

	if ports := meta.Get("ports"); ports != "" {
		sb.WriteString("    ports:\n")
		for _, line := range strings.Split(ports, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if strings.Contains(line, " -> ") {
				parts := strings.Split(line, " -> ")
				fmt.Fprintf(&sb, "      - \"%s:%s\"\n", parts[0], parts[1])
			} else {
				fmt.Fprintf(&sb, "      - \"%s\"\n", line)
			}
		}
	}

	if mounts := meta.Get("[MOUNTS]"); mounts != "" {
		sb.WriteString("    volumes:\n")
		for _, m := range strings.Split(mounts, ";;") {
			m = strings.TrimSpace(m)
			if m == "" {
				continue
			}
			parts := strings.Split(m, ":::")
			if len(parts) >= 4 {
				dest, src, mode := parts[0], parts[1], parts[3]
				fmt.Fprintf(&sb, "      - %s:%s:%s\n", src, dest, mode)
			}
		}
	}

	if env := meta.Get("[ENV-VAR]"); env != "" {
		sb.WriteString("    environment:\n")
		for _, e := range strings.Split(env, ";") {
			e = strings.TrimSpace(e)
			if e == "" {
				continue
			}
			fmt.Fprintf(&sb, "      - %s\n", e)
		}
	}

	if meta.Get("privileged") == "true" {
		sb.WriteString("    privileged: true\n")
	}
	if meta.Get("readonlyRootfs") == "true" {
		sb.WriteString("    read_only: true\n")
	}

	if cmd := meta.Get("cmd"); cmd != "" {
		fmt.Fprintf(&sb, "    command: %s\n", cmd)
	}

	return sb.String()
}
