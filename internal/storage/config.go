package storage

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"

	"github.com/jzelinskie/stringz"
)

const configFileName = "config.json"

// ErrConfigNotFound is returned if there is no Config in a ConfigStore.
var ErrConfigNotFound = errors.New("config did not exist")

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

var ErrMissingToken = errors.New("could not find token")

// DefaultToken creates a Token from input, filling any missing values in
// with the current context's defaults.
func DefaultToken(overrideEndpoint, overrideAPIToken string, cs ConfigStore, ss SecretStore) (Token, error) {
	if overrideEndpoint != "" && overrideAPIToken != "" {
		return Token{
			Name:     "env",
			Endpoint: overrideEndpoint,
			APIToken: overrideAPIToken,
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
		APIToken: stringz.DefaultEmpty(overrideAPIToken, token.APIToken),
		Insecure: token.Insecure,
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

	return atomicWriteFile(filepath.Join(s.ConfigPath, configFileName), cfgBytes, 0o774)
}

// atomicWriteFile writes data to filename+some suffix, then renames it into
// filename.
//
// Copyright (c) 2019 Tailscale Inc & AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found
// at the following URL:
// https://github.com/tailscale/tailscale/blob/main/LICENSE
func atomicWriteFile(filename string, data []byte, perm os.FileMode) (err error) {
	f, err := os.CreateTemp(filepath.Dir(filename), filepath.Base(filename)+".tmp")
	if err != nil {
		return err
	}
	tmpName := f.Name()
	defer func() {
		if err != nil {
			f.Close()
			os.Remove(tmpName)
		}
	}()
	if _, err := f.Write(data); err != nil {
		return err
	}
	if runtime.GOOS != "windows" {
		if err := f.Chmod(perm); err != nil {
			return err
		}
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, filename)
}
