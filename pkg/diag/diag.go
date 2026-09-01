// Package diag provides introspection, container state dumping, reflection inspection, and JSON/Text diagnostic report exporting.
//
// Objective:
//
//	Generate structured point-in-time diagnostic reports (sanitized metadata, metrics, mounts, networks,
//	top processes, generated compose/run commands) and serialize them to disk in JSON or text formats.
//
// Core Components:
//   - ContainerReport: Comprehensive data structure holding full container runtime topology and state.
//   - BuildReport: Aggregates raw container metadata and metrics into a ContainerReport.
//   - SaveReport: Writes formatted report artifacts (JSON, Text, or Both) to the configured download directory.
//
// Data Flow:
//
//	Container Snapshots -> diag.BuildReport() -> diag.SaveReport() -> Disk File (.json / .txt).
package diag

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/edsilegx/ctop/pkg/models"
	"github.com/edsilegx/ctop/pkg/sanitize"
)

// MountInfo represents container volume and bind mount configuration.
type MountInfo struct {
	Destination string `json:"destination"`
	Source      string `json:"source"`
	Type        string `json:"type"`
	Mode        string `json:"mode"`
	Driver      string `json:"driver,omitempty"`
}

// NetworkInfo represents container network interface configuration.
type NetworkInfo struct {
	Name      string `json:"name"`
	IPAddress string `json:"ip_address"`
	Gateway   string `json:"gateway"`
	Mac       string `json:"mac"`
	PrefixLen int    `json:"prefix_len"`
}

// ContainerReport represents a complete, structured point-in-time diagnostic snapshot of a container.
type ContainerReport struct {
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	Image            string            `json:"image"`
	ImageID          string            `json:"image_id,omitempty"`
	State            string            `json:"state"`
	Health           string            `json:"health,omitempty"`
	Host             string            `json:"host,omitempty"`
	Created          string            `json:"created,omitempty"`
	Uptime           string            `json:"uptime,omitempty"`
	CPUUtil          int               `json:"cpu_util"`
	MemUsage         int64             `json:"mem_usage"`
	MemLimit         int64             `json:"mem_limit"`
	MemPercent       int               `json:"mem_percent"`
	NetRx            int64             `json:"net_rx"`
	NetTx            int64             `json:"net_tx"`
	NetRxRate        int64             `json:"net_rx_rate"`
	NetTxRate        int64             `json:"net_tx_rate"`
	IOBytesRead      int64             `json:"io_bytes_read"`
	IOBytesWrite     int64             `json:"io_bytes_write"`
	IORateRead       int64             `json:"io_rate_read"`
	IORateWrite      int64             `json:"io_rate_write"`
	Pids             int               `json:"pids"`
	IPs              string            `json:"ips,omitempty"`
	Ports            string            `json:"ports,omitempty"`
	WebPort          string            `json:"web_port,omitempty"`
	Command          string            `json:"command,omitempty"`
	Entrypoint       string            `json:"entrypoint,omitempty"`
	WorkDir          string            `json:"workdir,omitempty"`
	User             string            `json:"user,omitempty"`
	RestartPolicy    string            `json:"restart_policy,omitempty"`
	MemLimitStr      string            `json:"mem_limit_str,omitempty"`
	CPULimit         string            `json:"cpu_limit,omitempty"`
	PidsLimit        string            `json:"pids_limit,omitempty"`
	CapAdd           string            `json:"cap_add,omitempty"`
	CapDrop          string            `json:"cap_drop,omitempty"`
	Privileged       string            `json:"privileged,omitempty"`
	ReadonlyRootfs   string            `json:"readonly_rootfs,omitempty"`
	Platform         string            `json:"platform,omitempty"`
	ImageSize        string            `json:"image_size,omitempty"`
	ImageLayers      string            `json:"image_layers,omitempty"`
	ImageAuthor      string            `json:"image_author,omitempty"`
	ImageCreated     string            `json:"image_created,omitempty"`
	DockerVersion    string            `json:"docker_version,omitempty"`
	ImageLabels      map[string]string `json:"image_labels,omitempty"`
	ImageEnv         []string          `json:"image_env,omitempty"`
	ImageVolumes     string            `json:"image_volumes,omitempty"`
	ImagePorts       string            `json:"image_ports,omitempty"`
	Env              []string          `json:"env,omitempty"`
	Labels           map[string]string `json:"labels,omitempty"`
	Mounts           []MountInfo       `json:"mounts,omitempty"`
	Networks         []NetworkInfo     `json:"networks,omitempty"`
	GeneratedRunCmd  string            `json:"generated_run_cmd,omitempty"`
	GeneratedCompose string            `json:"generated_compose,omitempty"`
	Timestamp        time.Time         `json:"timestamp"`
}

// ContainerSnapshot legacy struct for compatibility.
type ContainerSnapshot struct {
	ID      string            `json:"id"`
	Meta    map[string]string `json:"meta"`
	Metrics any               `json:"metrics"`
}

// BuildReport constructs a ContainerReport from container state, metadata, and telemetry.
func BuildReport(id string, meta map[string]string, m *models.Metrics, hostID string, runCmd string, compose string) *ContainerReport {
	if meta == nil {
		meta = make(map[string]string)
	}

	report := &ContainerReport{
		ID:               id,
		Name:             meta["name"],
		Image:            meta["image"],
		ImageID:          meta["imageId"],
		State:            meta["state"],
		Health:           meta["health"],
		Host:             hostID,
		Created:          meta["created"],
		Uptime:           meta["uptime"],
		IPs:              meta["IPs"],
		Ports:            meta["ports"],
		WebPort:          meta["Web Port"],
		Command:          meta["cmd"],
		Entrypoint:       meta["entrypoint"],
		WorkDir:          meta["workdir"],
		User:             meta["user"],
		RestartPolicy:    meta["restartPolicy"],
		MemLimitStr:      meta["memLimit"],
		CPULimit:         meta["cpuLimit"],
		PidsLimit:        meta["pidsLimit"],
		CapAdd:           meta["capAdd"],
		CapDrop:          meta["capDrop"],
		Privileged:       meta["privileged"],
		ReadonlyRootfs:   meta["readonlyRootfs"],
		Platform:         meta["imageArch"],
		ImageSize:        meta["imageSize"],
		ImageLayers:      meta["imageLayers"],
		ImageAuthor:      meta["imageAuthor"],
		ImageCreated:     meta["imageCreated"],
		DockerVersion:    meta["imageDockerVersion"],
		ImageLabels:      parseLabels(meta["imageLabels"]),
		ImageEnv:         parseEnv(meta["imageEnv"]),
		ImageVolumes:     meta["imageVolumes"],
		ImagePorts:       meta["imagePorts"],
		Env:              parseEnv(meta["[ENV-VAR]"]),
		Labels:           parseLabels(meta["[LABELS]"]),
		Mounts:           parseMounts(meta["[MOUNTS]"]),
		Networks:         parseNetworks(meta["[NETWORKS]"]),
		GeneratedRunCmd:  runCmd,
		GeneratedCompose: compose,
		Timestamp:        time.Now().UTC(),
	}

	if m != nil {
		report.CPUUtil = nonNeg(m.CPUUtil)
		report.MemUsage = nonNeg(m.MemUsage)
		report.MemLimit = nonNeg(m.MemLimit)
		report.MemPercent = nonNeg(m.MemPercent)
		report.NetRx = nonNeg(m.NetRx)
		report.NetTx = nonNeg(m.NetTx)
		report.NetRxRate = nonNeg(m.NetRxRate)
		report.NetTxRate = nonNeg(m.NetTxRate)
		report.IOBytesRead = nonNeg(m.IOBytesRead)
		report.IOBytesWrite = nonNeg(m.IOBytesWrite)
		report.IORateRead = nonNeg(m.IORateRead)
		report.IORateWrite = nonNeg(m.IORateWrite)
		report.Pids = nonNeg(m.Pids)
	}

	return report
}

// FormatJSON serializes a ContainerReport into pretty-printed JSON.
func FormatJSON(report *ContainerReport) ([]byte, error) {
	return json.MarshalIndent(report, "", "  ")
}

// FormatTextReport generates a clean, readable ASCII diagnostic report.
func FormatTextReport(r *ContainerReport) string {
	var b strings.Builder
	line := strings.Repeat("=", 80)
	divider := strings.Repeat("-", 80)

	b.WriteString(line + "\n")
	fmt.Fprintf(&b, "  CONTAINER DIAGNOSTIC REPORT: %s (%s)\n", r.Name, r.ID)
	fmt.Fprintf(&b, "  Generated: %s UTC | ctop\n", r.Timestamp.Format("2006-01-02 15:04:05"))
	b.WriteString(line + "\n\n")

	// Section 1: Status & Runtime
	b.WriteString("[ STATUS & RUNTIME ]\n")
	fmt.Fprintf(&b, "  Container Name   : %s\n", r.Name)
	fmt.Fprintf(&b, "  Container ID     : %s\n", r.ID)
	fmt.Fprintf(&b, "  State            : %s\n", r.State)
	if r.Health != "" {
		fmt.Fprintf(&b, "  Health Status    : %s\n", r.Health)
	}
	fmt.Fprintf(&b, "  Created          : %s\n", r.Created)
	fmt.Fprintf(&b, "  Uptime           : %s\n", r.Uptime)
	if r.Host != "" {
		fmt.Fprintf(&b, "  Host Node        : %s\n", r.Host)
	}
	b.WriteString("\n")

	// Section 2: Resource Usage & Telemetry
	b.WriteString("[ RESOURCE USAGE & TELEMETRY ]\n")
	fmt.Fprintf(&b, "  CPU Utilization  : %d%%\n", r.CPUUtil)
	fmt.Fprintf(&b, "  Memory Usage     : %s / %s (%d%%)\n", formatBytes(r.MemUsage), formatBytes(r.MemLimit), r.MemPercent)
	fmt.Fprintf(&b, "  Network I/O      : Rx: %s (%s/s) | Tx: %s (%s/s)\n", formatBytes(r.NetRx), formatBytes(r.NetRxRate), formatBytes(r.NetTx), formatBytes(r.NetTxRate))
	fmt.Fprintf(&b, "  Disk I/O         : Read: %s (%s/s) | Write: %s (%s/s)\n", formatBytes(r.IOBytesRead), formatBytes(r.IORateRead), formatBytes(r.IOBytesWrite), formatBytes(r.IORateWrite))
	fmt.Fprintf(&b, "  Active PIDs      : %d processes\n", r.Pids)
	b.WriteString("\n")

	// Section 3: Base Image & Execution
	b.WriteString("[ BASE IMAGE & EXECUTION ]\n")
	fmt.Fprintf(&b, "  Image Name / Tag : %s\n", r.Image)
	if r.ImageID != "" {
		fmt.Fprintf(&b, "  Image Digest/ID  : %s\n", r.ImageID)
	}
	if r.Platform != "" {
		fmt.Fprintf(&b, "  Platform / OS    : %s\n", r.Platform)
	}
	if r.ImageSize != "" {
		fmt.Fprintf(&b, "  Virtual Size     : %s\n", r.ImageSize)
	}
	if r.ImageLayers != "" {
		fmt.Fprintf(&b, "  RootFS Layers    : %s\n", r.ImageLayers)
	}
	if r.ImageAuthor != "" {
		fmt.Fprintf(&b, "  Author           : %s\n", r.ImageAuthor)
	}
	if r.ImageCreated != "" {
		fmt.Fprintf(&b, "  Image Created    : %s\n", r.ImageCreated)
	}
	if r.DockerVersion != "" {
		fmt.Fprintf(&b, "  Docker Engine    : %s\n", r.DockerVersion)
	}
	if r.Entrypoint != "" {
		fmt.Fprintf(&b, "  ENTRYPOINT       : %s\n", r.Entrypoint)
	}
	if r.Command != "" {
		fmt.Fprintf(&b, "  CMD              : %s\n", r.Command)
	}
	if r.WorkDir != "" {
		fmt.Fprintf(&b, "  Working Dir      : %s\n", r.WorkDir)
	}
	if r.User != "" {
		fmt.Fprintf(&b, "  User             : %s\n", r.User)
	}
	if r.ImagePorts != "" {
		fmt.Fprintf(&b, "  Exposed Ports    : %s\n", r.ImagePorts)
	}
	if r.ImageVolumes != "" {
		fmt.Fprintf(&b, "  Image Volumes    : %s\n", r.ImageVolumes)
	}
	if r.RestartPolicy != "" {
		fmt.Fprintf(&b, "  Restart Policy   : %s\n", r.RestartPolicy)
	}
	b.WriteString("\n")

	// Section 4: Network Configuration & Ports
	b.WriteString("[ NETWORK CONFIGURATION & PORTS ]\n")
	if len(r.Networks) > 0 {
		for _, net := range r.Networks {
			fmt.Fprintf(&b, "  • %-16s : %s/%d (Gateway: %s, MAC: %s)\n", net.Name, net.IPAddress, net.PrefixLen, net.Gateway, net.Mac)
		}
	} else if r.IPs != "" {
		fmt.Fprintf(&b, "  • IPs            : %s\n", r.IPs)
	}
	if r.Ports != "" {
		fmt.Fprintf(&b, "  • Port Mappings  : %s\n", strings.ReplaceAll(r.Ports, "\n", ", "))
	}
	b.WriteString("\n")

	// Section 5: Mounted Volumes
	if len(r.Mounts) > 0 {
		b.WriteString("[ MOUNTED VOLUMES & STORAGE ]\n")
		for _, m := range r.Mounts {
			fmt.Fprintf(&b, "  • %-24s -> %s (%s, %s)\n", m.Destination, m.Source, m.Type, m.Mode)
		}
		b.WriteString("\n")
	}

	// Section 6: Environment Variables
	if len(r.Env) > 0 {
		b.WriteString("[ ENVIRONMENT VARIABLES (Sanitized) ]\n")
		for _, env := range r.Env {
			fmt.Fprintf(&b, "  • %s\n", env)
		}
		b.WriteString("\n")
	}

	// Section 7: Labels
	if len(r.Labels) > 0 {
		b.WriteString("[ LABELS & ORCHESTRATION ]\n")
		var labelKeys []string
		for k := range r.Labels {
			labelKeys = append(labelKeys, k)
		}
		sort.Strings(labelKeys)
		for _, k := range labelKeys {
			fmt.Fprintf(&b, "  • %-32s = %s\n", k, r.Labels[k])
		}
		b.WriteString("\n")
	}

	// Section 8: Recreate Definitions
	if r.GeneratedRunCmd != "" || r.GeneratedCompose != "" {
		b.WriteString("[ RECREATE DEFINITIONS ]\n")
		if r.GeneratedRunCmd != "" {
			b.WriteString("  --- Equivalent Docker Run Command ---\n")
			for _, line := range strings.Split(r.GeneratedRunCmd, "\n") {
				fmt.Fprintf(&b, "  %s\n", line)
			}
			b.WriteString("\n")
		}
		if r.GeneratedCompose != "" {
			b.WriteString("  --- Equivalent docker-compose.yml ---\n")
			for _, line := range strings.Split(r.GeneratedCompose, "\n") {
				fmt.Fprintf(&b, "  %s\n", line)
			}
			b.WriteString("\n")
		}
	}

	b.WriteString(divider + "\n")
	b.WriteString("  End of Diagnostic Report\n")
	b.WriteString(line + "\n")

	return b.String()
}

// SaveReport exports the container report to the specified directory in JSON, Text, or both formats.
func SaveReport(report *ContainerReport, destDir string, format string) ([]string, error) {
	if destDir == "" {
		destDir = "."
	}
	_ = os.MkdirAll(filepath.Clean(destDir), 0o750)

	name := report.Name
	if name == "" {
		name = report.ID
	}
	if len(name) > 24 {
		name = name[:24]
	}
	ts := time.Now().Format("20060102_150405")

	var savedPaths []string

	if format == "json" || format == "both" {
		jsonData, err := FormatJSON(report)
		if err != nil {
			return nil, err
		}
		jsonFile := filepath.Join(destDir, fmt.Sprintf("ctop_report_%s_%s.json", name, ts))
		if err := os.WriteFile(jsonFile, jsonData, 0o600); err != nil {
			return nil, err
		}
		savedPaths = append(savedPaths, jsonFile)
	}

	if format == "txt" || format == "text" || format == "both" {
		txtReport := FormatTextReport(report)
		txtFile := filepath.Join(destDir, fmt.Sprintf("ctop_report_%s_%s.txt", name, ts))
		if err := os.WriteFile(txtFile, []byte(txtReport), 0o600); err != nil {
			return nil, err
		}
		savedPaths = append(savedPaths, txtFile)
	}

	return savedPaths, nil
}

// DumpText formats container state into a human-readable diagnostic text dump.
func DumpText(id string, meta map[string]string, metrics any) string {
	msg := fmt.Sprintf("logging state for container: %s\n", id)
	for k, v := range meta {
		msg += fmt.Sprintf("Meta.%s = %s\n", k, v)
	}
	if metrics != nil {
		msg += Inspect(metrics)
	}
	return msg
}

// DumpJSON serializes container metadata and telemetry into pretty-printed JSON.
func DumpJSON(id string, meta map[string]string, metrics any) ([]byte, error) {
	snapshot := ContainerSnapshot{
		ID:      id,
		Meta:    meta,
		Metrics: metrics,
	}
	return json.MarshalIndent(snapshot, "", "  ")
}

// Inspect uses reflection to format struct fields and types into key-value lines.
func Inspect(i any) (s string) {
	if i == nil {
		return ""
	}
	val := reflect.ValueOf(i)
	if val.Kind() == reflect.Pointer {
		if val.IsNil() {
			return ""
		}
		val = val.Elem()
	}

	elem := val.Type()
	eName := elem.String()
	for i := 0; i < elem.NumField(); i++ {
		field := elem.Field(i)
		fieldVal := val.Field(i)
		s += fmt.Sprintf("%s.%s = %v (%s)\n", eName, field.Name, fieldVal, field.Type)
	}
	return s
}

func parseMounts(raw string) []MountInfo {
	if raw == "" {
		return nil
	}
	var res []MountInfo
	for _, part := range strings.Split(raw, ";;") {
		fields := strings.Split(part, ":::")
		if len(fields) >= 4 {
			driver := ""
			if len(fields) >= 5 {
				driver = fields[4]
			}
			res = append(res, MountInfo{
				Destination: fields[0],
				Source:      fields[1],
				Type:        fields[2],
				Mode:        fields[3],
				Driver:      driver,
			})
		}
	}
	return res
}

func parseNetworks(raw string) []NetworkInfo {
	if raw == "" {
		return nil
	}
	var res []NetworkInfo
	for _, part := range strings.Split(raw, ";;") {
		fields := strings.Split(part, ":::")
		if len(fields) >= 4 {
			prefix := 0
			if len(fields) >= 5 {
				_, _ = fmt.Sscanf(fields[4], "%d", &prefix)
			}
			res = append(res, NetworkInfo{
				Name:      fields[0],
				IPAddress: fields[1],
				Gateway:   fields[2],
				Mac:       fields[3],
				PrefixLen: prefix,
			})
		}
	}
	return res
}

func parseLabels(raw string) map[string]string {
	if raw == "" {
		return nil
	}
	res := make(map[string]string)
	for _, part := range strings.Split(raw, ";;") {
		idx := strings.Index(part, "=")
		if idx > 0 {
			k := part[:idx]
			v := part[idx+1:]
			if !sanitize.IsSensitiveKey(k) {
				res[k] = v
			}
		}
	}
	return res
}

func parseEnv(raw string) []string {
	if raw == "" {
		return nil
	}
	return sanitize.SanitizeEnv(strings.Split(raw, ";"))
}

func nonNeg[T ~int | ~int64](v T) T {
	if v < 0 {
		return 0
	}
	return v
}

func formatBytes(n int64) string {
	if n <= 0 {
		return "0 B"
	}
	units := []string{"B", "KB", "MB", "GB", "TB"}
	f := float64(n)
	i := 0
	for f >= 1024 && i < len(units)-1 {
		f /= 1024
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%d B", n)
	}
	return fmt.Sprintf("%.2f %s", f, units[i])
}
