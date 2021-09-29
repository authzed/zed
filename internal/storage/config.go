package storage

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/jzelinskie/stringz"
	"tailscale.com/atomicfile"
)

const configFileName = "config.json"

// ErrConfigNotFound is returned if there is no Config in a ConfigStore.
var ErrConfigNotFound = errors.New("config did not exist")

// Config represents the contents of a zed configuration file.
type Config struct {
	Version      string
	CurrentToken string
	Tokens       []Token
}

// ConfigStore is anything that can persistently store a Config.
type ConfigStore interface {
	Get() (Config, error)
	Put(Config) error
}

var ErrMissingToken = errors.New("could not find token")

// DefaultToken creates a Token from input, filling any missing values in
// with the current context's defaults.
func DefaultToken(overrideEndpoint, overrideApiToken string, cs ConfigStore, ss SecretStore) (Token, error) {
	if overrideEndpoint != "" && overrideApiToken != "" {
		return Token{
			Name:     "env",
			Endpoint: overrideEndpoint,
			ApiToken: overrideApiToken,
		}, nil
	}

	token, err := CurrentToken(cs, ss)
	if err != nil {
		if errors.Is(err, ErrConfigNotFound) {
			return Token{}, errors.New("must first save a token: see `zed token save --help`")
		}
		return Token{}, err
	}

	return Token{
		Name:     token.Name,
		Endpoint: stringz.DefaultEmpty(overrideEndpoint, token.Endpoint),
		ApiToken: stringz.DefaultEmpty(overrideApiToken, token.ApiToken),
	}, nil
}

// CurrentToken is convenient way to obtain the CurrentToken field from the
// current Config.
func CurrentToken(cs ConfigStore, ss SecretStore) (Token, error) {
	cfg, err := cs.Get()
	if err != nil {
		return Token{}, err
	}

	return GetToken(cfg.CurrentToken, ss)
}

// SetCurrentToken is a convenient way to set the CurrentToken field in a
// the current config.
func SetCurrentToken(name string, cs ConfigStore, ss SecretStore) error {
	// Ensure the token exists
	if _, err := GetToken(name, ss); err != nil {
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

// JSONConfigStore implements a ConfigStore that stores its Config in a JSON file at the provided ConfigPath.
type JSONConfigStore struct {
	ConfigPath string
}

// Enforce that our implementation satisfies the interface.
var _ ConfigStore = JSONConfigStore{}

// Get parses a Config from the filesystem.
func (s JSONConfigStore) Get() (Config, error) {
	cfgBytes, err := os.ReadFile(filepath.Join(s.ConfigPath, configFileName))
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

// Put overwrites a Config on the filesystem.
func (s JSONConfigStore) Put(cfg Config) error {
	if err := os.MkdirAll(s.ConfigPath, 0o774); err != nil {
		return err
	}

	cfgBytes, err := json.Marshal(cfg)
	if err != nil {
		return err
	}

	return atomicfile.WriteFile(filepath.Join(s.ConfigPath, configFileName), cfgBytes, 0o774)
}
