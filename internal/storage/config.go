package storage

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/mitchellh/go-homedir"
	"tailscale.com/atomicfile"
)

type Config struct {
	Version      string
	CurrentToken string
}

type ConfigStore interface {
	Get() (Config, error)
	Put(Config) error
}

func CurrentToken(cs ConfigStore, ts TokenStore) (Token, error) {
	cfg, err := cs.Get()
	if err != nil {
		return Token{}, err
	}

	return ts.Get(cfg.CurrentToken, false)
}

func SetCurrentToken(name string, cs ConfigStore, ts TokenStore) error {
	// Ensure the token exists
	_, err := ts.Get(name, true)
	if err != nil {
		return err
	}

	cfg, err := cs.Get()
	if err != nil {
		if errors.Is(err, ErrConfigNotFound) {
			cfg = Config{Version: "v1"}
		} else {
			return err
		}
	}

	cfg.CurrentToken = name
	return cs.Put(cfg)
}

type HomeJSONConfigStore struct{}

var _ ConfigStore = HomeJSONConfigStore{}

var ErrConfigNotFound = errors.New("config did not exist")

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

func (s HomeJSONConfigStore) Get() (Config, error) {
	path, err := localConfigPath()
	if err != nil {
		return Config{}, err
	}

	cfgBytes, err := os.ReadFile(filepath.Join(path, "config.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return Config{}, ErrConfigNotFound
		}
		return Config{}, err
	}

	var cfg Config
	if err := json.Unmarshal(cfgBytes, &cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}
func (s HomeJSONConfigStore) Put(cfg Config) error {
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
