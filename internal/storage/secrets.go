package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/99designs/keyring"
	"golang.org/x/term"
)

// ErrTokenNotFound is returned if there is no Config in a ConfigStore.
var ErrTokenNotFound = errors.New("token does not exist")

type Token struct {
	Name     string
	Endpoint string
	ApiToken string
}

type Secrets struct {
	Tokens []Token
}

type SecretStore interface {
	Get() (Secrets, error)
	Put(s Secrets) error
}

func GetToken(name string, ss SecretStore) (Token, error) {
	secrets, err := ss.Get()
	if err != nil {
		return Token{}, err
	}

	for _, token := range secrets.Tokens {
		if name == token.Name {
			return token, nil
		}
	}

	return Token{}, ErrTokenNotFound
}

type KeychainSecretStore struct {
	ConfigPath string
}

const keyringEntryName = "zed secrets"

var _ SecretStore = KeychainSecretStore{}

func openKeyring(configPath string) (keyring.Keyring, error) {
	return keyring.Open(keyring.Config{
		ServiceName: "zed",
		FileDir:     filepath.Join(configPath, "keyring.jwt"),
		FilePasswordFunc: func(prompt string) (string, error) {
			if password, ok := os.LookupEnv("ZED_KEYRING_PASSWORD"); ok {
				return password, nil
			}

			fmt.Fprintf(os.Stderr, "%s: ", prompt)
			b, err := term.ReadPassword(int(os.Stdin.Fd()))
			if err != nil {
				return "", err
			}
			fmt.Println()
			return string(b), nil
		},
	})
}

func (k KeychainSecretStore) Get() (Secrets, error) {
	ring, err := openKeyring(k.ConfigPath)
	if err != nil {
		return Secrets{}, err
	}

	entry, err := ring.Get(keyringEntryName)
	if err != nil {
		if errors.Is(err, keyring.ErrKeyNotFound) {
			return Secrets{}, nil // empty is okay!
		}
		return Secrets{}, err
	}

	var s Secrets
	err = json.Unmarshal(entry.Data, &s)
	return s, err
}

func (k KeychainSecretStore) Put(s Secrets) error {
	ring, err := openKeyring(k.ConfigPath)
	if err != nil {
		return err
	}

	data, err := json.Marshal(s)
	if err != nil {
		return err
	}

	return ring.Set(keyring.Item{
		Key:  keyringEntryName,
		Data: data,
	})
}
