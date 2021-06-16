package storage

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/mitchellh/go-homedir"
	"tailscale.com/atomicfile"
)

const configFile = "config.json"

var (
	// DefaultConfigStore is the ConfigStore that should be used unless otherwise
	// specified.
	DefaultConfigStore = HomeJSONConfigStore{}

	// ErrConfigNotFound is returned if there is no Config in a ConfigStore.
	ErrConfigNotFound = errors.New("config did not exist")
)

// Config represents the contents of a zed configuration file.
type Config struct {
	Version      string
	CurrentToken string
}

// ConfigStore is anything that can persistently store a Config.
type ConfigStore interface {
	Get() (Config, error)
	Put(Config) error
}

// CurrentToken is convenient way to obtain the CurrentToken field from the
// current Config.
func CurrentToken(cs ConfigStore, ts TokenStore) (Token, error) {
	cfg, err := cs.Get()
	if err != nil {
		return Token{}, err
	}

	return ts.Get(cfg.CurrentToken, true)
}

// SetCurrentToken is a convenient way to set the CurrentToken field in a
// the current config.
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

// HomeJSONConfigStore implements a ConfigStore that stores its Config in the
// file "${XDG_CONFIG_HOME:-$HOME/.zed}/config.json".
type HomeJSONConfigStore struct{}

// Enforce that our implementation satisfies the interface.
var _ ConfigStore = HomeJSONConfigStore{}

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

	cfgBytes, err := os.ReadFile(filepath.Join(path, configFile))
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

	return atomicfile.WriteFile(filepath.Join(path, configFile), cfgBytes, 0774)
}
