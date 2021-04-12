package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mitchellh/go-homedir"
	"tailscale.com/atomicfile"

	"github.com/jzelinskie/zed/internal/keychain"
)

type Context struct {
	Name      string
	Tenant    string
	TokenName string
}

func (c Context) String() string {
	if c.Name == "" {
		return "no context"
	}
	return fmt.Sprintf("%s: %s via %s", c.Name, c.Tenant, c.TokenName)
}

type Config struct {
	CurrentContext    string
	AvailableContexts []Context
}

func (cfg *Config) WithContext(c Context) {
	for i, context := range cfg.AvailableContexts {
		if c.Name == context.Name {
			cfg.AvailableContexts[i] = context
			return
		}
	}
	cfg.AvailableContexts = append(cfg.AvailableContexts, c)
}

func Path() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "zed", "config.json"), nil
	}

	home, err := homedir.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".zed", "config.json"), nil
}

func Get() (*Config, error) {
	filepath, err := Path()
	if err != nil {
		return nil, err
	}

	cfgBytes, err := os.ReadFile(filepath)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(cfgBytes, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func Put(cfg *Config) error {
	path, err := Path()
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

func CurrentContext() (*Context, error) {
	cfg, err := Get()
	if err != nil {
		return nil, err
	}

	if cfg.CurrentContext == "" {
		return nil, fmt.Errorf("current context has not been set")
	}

	for _, context := range cfg.AvailableContexts {
		if context.Name == cfg.CurrentContext {
			return &context, nil
		}
	}
	return nil, fmt.Errorf("current context does not exist")
}

func CurrentCredentials(tenantOverride, tokenOverride string) (tenant, token string, err error) {
	if tenantOverride != "" && tokenOverride != "" {
		return tenantOverride, tokenOverride, nil
	}

	context, err := CurrentContext()
	if err != nil {
		return "", "", err
	}

	tokenBytes, err := keychain.Get("authzed.com", context.TokenName)
	if err != nil {
		return "", "", err
	}

	return context.Tenant, string(tokenBytes), nil
}
