package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"

	"gopkg.in/yaml.v3"
)

// Config is the top-level service configuration loaded from a .subs.yaml file.
type Config struct {
	Host        string            `yaml:"host"`
	Port        int               `yaml:"port"`
	WorkDir     string            `yaml:"work_dir"`
	APIEnabled  bool              `yaml:"api_enabled"`
	APIHost     string            `yaml:"api_host"`
	APIPort     int               `yaml:"api_port"`
	APIToken    string            `yaml:"api_token"`
	MaxUploadMB int               `yaml:"max_upload_mb"`
	Title       string            `yaml:"title"`
	Description string            `yaml:"description"`
	Headers     map[string]string `yaml:"headers"`

	// baseDir is the directory containing the config file; WorkDir is resolved
	// relative to it.
	baseDir string `yaml:"-"`
}

// listenAddr returns the address the public server should listen on, defaulting
// to 127.0.0.1:9876 when the config does not set host/port.
func (c *Config) listenAddr() string {
	return joinHostPort(c.Host, c.Port, defaultPort)
}

// apiListenAddr returns the address the API server should listen on, defaulting
// to 127.0.0.1:4321 when the config does not set api_host/api_port.
func (c *Config) apiListenAddr() string {
	return joinHostPort(c.APIHost, c.APIPort, defaultAPIPort)
}

// maxUploadBytes returns the multipart upload limit in bytes, defaulting to
// 10 MiB when max_upload_mb is unset.
func (c *Config) maxUploadBytes() int64 {
	mb := c.MaxUploadMB
	if mb <= 0 {
		mb = defaultMaxUploadMB
	}
	return int64(mb) << 20
}

// joinHostPort builds a listen address, defaulting host to 127.0.0.1 and port
// to fallbackPort when unset.
func joinHostPort(host string, port, fallbackPort int) string {
	if host == "" {
		host = defaultHost
	}
	if port == 0 {
		port = fallbackPort
	}
	return net.JoinHostPort(host, strconv.Itoa(port))
}

// resolvedWorkDir returns WorkDir as an absolute path, resolving relative
// values against the directory that contained the config file.
func (c *Config) resolvedWorkDir() string {
	if filepath.IsAbs(c.WorkDir) {
		return c.WorkDir
	}
	return filepath.Join(c.baseDir, c.WorkDir)
}

// locateConfig returns the config path to use. Precedence: explicit argument,
// then ./.subs.yaml, then ~/.subs.yaml, then /etc/subs.yaml.
func locateConfig(arg string) (string, error) {
	if arg != "" {
		if _, err := os.Stat(arg); err != nil {
			return "", fmt.Errorf("config %q: %w", arg, err)
		}
		return arg, nil
	}

	candidates := []string{".subs.yaml"}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, ".subs.yaml"))
	}
	candidates = append(candidates, "/etc/subs.yaml")
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("no config found (looked for ./.subs.yaml, ~/.subs.yaml, /etc/subs.yaml); pass one as an argument")
}

// loadConfig reads and parses the config file at path.
func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if cfg.WorkDir == "" {
		cfg.WorkDir = ".subs"
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	cfg.baseDir = filepath.Dir(abs)
	return &cfg, nil
}
