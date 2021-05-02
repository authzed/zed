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
		return filepath.Join(xdg, "zed"), nil
	}

	home, err := homedir.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".zed"), nil
}

func (s LocalFsContextConfigStore) Get() (*ContextConfig, error) {
	path, err := localConfigPath()
	if err != nil {
		return nil, err
	}

	cfgBytes, err := os.ReadFile(filepath.Join(path, "config.json"))
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
	if err := os.MkdirAll(path, 0774); err != nil {
		return err
	}

	cfgBytes, err := json.Marshal(cfg)
	if err != nil {
		return err
	}

	return atomicfile.WriteFile(filepath.Join(path, "config.json"), cfgBytes, 0774)
}
