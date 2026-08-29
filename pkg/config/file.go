package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
)

var xdgRe = regexp.MustCompile("^XDG_*")

type File struct {
	Options map[string]string `toml:"options"`
	Toggles map[string]bool   `toml:"toggles"`
}

func exportConfig() File {
	// update columns param from working config
	Update("columns", ColumnsString())

	lock.RLock()
	defer lock.RUnlock()

	c := File{
		Options: make(map[string]string),
		Toggles: make(map[string]bool),
	}

	for _, p := range GlobalParams {
		c.Options[p.Key] = p.Val
	}
	for _, sw := range GlobalSwitches {
		c.Toggles[sw.Key] = sw.Val
	}

	return c
}

func Read() error {
	var config File

	path, err := getConfigPath()
	if err != nil {
		return err
	}

	if _, err := toml.DecodeFile(path, &config); err != nil {
		return err
	}
	for k, v := range config.Options {
		Update(k, v)
	}
	for k, v := range config.Toggles {
		UpdateSwitch(k, v)
	}

	// set working column config, if provided
	colStr := GetVal("columns")
	if len(colStr) > 0 {
		var colNames []string
		for _, s := range strings.Split(colStr, ",") {
			s = strings.TrimSpace(s)
			if s != "" {
				colNames = append(colNames, s)
			}
		}
		SetColumns(colNames)
	}

	return nil
}

func Write() (path string, err error) {
	path, err = getConfigPath()
	if err != nil {
		return path, err
	}

	cfgdir := filepath.Dir(path)
	// create config dir if not exist with restricted permissions
	if _, err := os.Stat(cfgdir); err != nil {
		err = os.MkdirAll(cfgdir, 0o700)
		if err != nil {
			return path, fmt.Errorf("failed to create config dir [%s]: %s", cfgdir, err)
		}
	}

	// remove prior to writing new file
	if err := os.Remove(path); err != nil {
		if !os.IsNotExist(err) {
			return path, err
		}
	}

	// #nosec G304 - configuration file path resolved from user config directory
	file, err := os.OpenFile(filepath.Clean(path), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return path, fmt.Errorf("failed to open config for writing: %s", err)
	}
	defer func() {
		_ = file.Close()
	}()

	writer := toml.NewEncoder(file)
	err = writer.Encode(exportConfig())
	if err != nil {
		return path, fmt.Errorf("failed to write config: %s", err)
	}

	return path, nil
}

// determine config path from environment
func getConfigPath() (path string, err error) {
	if xdgHome, ok := os.LookupEnv("XDG_CONFIG_HOME"); ok && xdgHome != "" {
		return filepath.Clean(filepath.Join(xdgHome, "ctop", "config")), nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = os.Getenv("HOME")
	}

	// Respect existing legacy ~/.ctop file if present
	if homeDir != "" {
		legacyPath := filepath.Clean(filepath.Join(homeDir, ".ctop"))
		if _, statErr := os.Stat(legacyPath); statErr == nil {
			return legacyPath, nil
		}
	}

	if configDir, err := os.UserConfigDir(); err == nil && configDir != "" {
		return filepath.Clean(filepath.Join(configDir, "ctop", "config")), nil
	}

	if homeDir != "" {
		if xdgSupport() {
			return filepath.Clean(filepath.Join(homeDir, ".config", "ctop", "config")), nil
		}
		return filepath.Clean(filepath.Join(homeDir, ".ctop")), nil
	}

	return "", fmt.Errorf("unable to determine user home or config directory")
}

// test for environemnt supporting XDG spec
func xdgSupport() bool {
	for _, e := range os.Environ() {
		if xdgRe.FindAllString(e, 1) != nil {
			return true
		}
	}
	return false
}
