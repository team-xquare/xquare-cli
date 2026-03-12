package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const configDir = ".xquare"
const globalConfigFile = "config.yaml"

// GlobalConfig stores auth + server URL
type GlobalConfig struct {
	ServerURL string `yaml:"server_url"`
	Token     string `yaml:"token"`
	Username  string `yaml:"username"`
}

// ProjectConfig stores project-level context (from .xquare/config in project dir)
type ProjectConfig struct {
	Project string `yaml:"project"`
}

func globalConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".xquare", globalConfigFile), nil
}

func LoadGlobal() (*GlobalConfig, error) {
	path, err := globalConfigPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &GlobalConfig{ServerURL: defaultServerURL()}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg GlobalConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	// Env var always takes precedence over config file
	if v := os.Getenv("XQUARE_SERVER_URL"); v != "" {
		cfg.ServerURL = v
	} else if cfg.ServerURL == "" {
		cfg.ServerURL = defaultServerURL()
	}
	return &cfg, nil
}

func SaveGlobal(cfg *GlobalConfig) error {
	path, err := globalConfigPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// LoadProject reads .xquare/config from the current directory
func LoadProject() (*ProjectConfig, error) {
	data, err := os.ReadFile(filepath.Join(configDir, "config"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var cfg ProjectConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// SaveProject writes .xquare/config in the current directory
func SaveProject(cfg *ProjectConfig) error {
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(configDir, "config"), data, 0644)
}

func defaultServerURL() string {
	if v := os.Getenv("XQUARE_SERVER_URL"); v != "" {
		return v
	}
	return "https://xquare-api.dsmhs.kr"
}
