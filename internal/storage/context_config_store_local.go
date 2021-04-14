package storage

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/mitchellh/go-homedir"
	"tailscale.com/atomicfile"
)

type LocalFsContextConfigStore struct{}

func localConfigPath() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "zed", "config.json"), nil
	}

	home, err := homedir.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".zed", "config.json"), nil
}

func (s LocalFsContextConfigStore) Get() (*ContextConfig, error) {
	filepath, err := localConfigPath()
	if err != nil {
		return nil, err
	}

	cfgBytes, err := os.ReadFile(filepath)
	if err != nil {
		if os.IsNotExist(err) {
			return &ContextConfig{}, nil
		}
		return nil, err
	}

	var cfg ContextConfig
	if err := json.Unmarshal(cfgBytes, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (s LocalFsContextConfigStore) Put(cfg *ContextConfig) error {
	path, err := localConfigPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0774); err != nil {
		return err
	}

	cfgBytes, err := json.Marshal(cfg)
	if err != nil {
		return err
	}

	return atomicfile.WriteFile(path, cfgBytes, 0774)
}
