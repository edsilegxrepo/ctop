package config

import "strings"

// DefaultDownloadDir defines the default host directory for file explorer downloads, report exports, and log saves.
const DefaultDownloadDir = "/tmp"

// defaults
var defaultParams = []*Param{
	{
		Key:   "filterStr",
		Val:   "",
		Label: "Container Name or ID Filter",
	},
	{
		Key:   "sortField",
		Val:   "state",
		Label: "Container Sort Field",
	},
	{
		Key:   "columns",
		Val:   "status,name,id,cpu,mem,net,io,pids,uptime",
		Label: "Enabled Columns",
	},
	{
		Key:   "downloadDir",
		Val:   DefaultDownloadDir,
		Label: "Default Host Download Directory",
	},
	{
		Key:   "icons",
		Val:   "unicode",
		Label: "Icon Style (unicode or nerd)",
	},
	{
		Key:   "probeInterval",
		Val:   "5s",
		Label: "Network Probing Interval",
	},
}

type Param struct {
	Key   string
	Val   string
	Label string
}

// Get Param by key
func Get(k string) *Param {
	lock.RLock()
	defer lock.RUnlock()

	for _, p := range GlobalParams {
		if p.Key == k {
			return p
		}
	}
	return &Param{} // default
}

// GetVal gets Param value by key
func GetVal(k string) string {
	return Get(k).Val
}

// Set param value
func Update(k, v string) {
	p := Get(k)
	log.Noticef("config change [%s]: %s -> %s", k, quote(p.Val), quote(v))

	lock.Lock()
	defer lock.Unlock()
	for _, existing := range GlobalParams {
		if existing.Key == k {
			existing.Val = v
			return
		}
	}
	GlobalParams = append(GlobalParams, &Param{Key: k, Val: v})
}

// GetDownloadDir returns the active download directory or DefaultDownloadDir if unset or blank.
func GetDownloadDir() string {
	dir := strings.TrimSpace(GetVal("downloadDir"))
	if dir == "" {
		return DefaultDownloadDir
	}
	return dir
}

// SetDownloadDir sets the active download directory, falling back to DefaultDownloadDir if blank.
func SetDownloadDir(dir string) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		dir = DefaultDownloadDir
	}
	Update("downloadDir", dir)
}
