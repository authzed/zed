package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/99designs/keyring"
	"github.com/jzelinskie/stringz"
	"golang.org/x/term"

	"github.com/authzed/zed/internal/console"
)

// ErrTokenNotFound is returned if there is no Token in a ConfigStore.
var ErrTokenNotFound = errors.New("token does not exist")

type Token struct {
	Name     string
	Endpoint string
	APIToken string
	Insecure *bool
	CACert   []byte
}

func (t Token) Certificate() (cert []byte, ok bool) {
	if t.CACert != nil && len(t.CACert) > 0 {
		return t.CACert, true
	}
	return nil, false
}

func (t Token) IsInsecure() bool {
	return t.Insecure != nil && *t.Insecure
}

func (t Token) Redacted() string {
	prefix, _ := t.SplitAPIToken()
	if prefix == "" {
		return "<redacted>"
	}

	return stringz.Join("_", prefix, "<redacted>")
}

func (t Token) SplitAPIToken() (prefix, secret string) {
	exploded := strings.Split(t.APIToken, "_")
	return strings.Join(exploded[:len(exploded)-1], "_"), exploded[len(exploded)-1]
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

func PutToken(t Token, ss SecretStore) error {
	secrets, err := ss.Get()
	if err != nil {
		return err
	}

	replaced := false
	for i, token := range secrets.Tokens {
		if token.Name == t.Name {
			secrets.Tokens[i] = t
			replaced = true
		}
	}

	if !replaced {
		secrets.Tokens = append(secrets.Tokens, t)
	}

	return ss.Put(secrets)
}

func RemoveToken(name string, ss SecretStore) error {
	secrets, err := ss.Get()
	if err != nil {
		return err
	}

	for i, token := range secrets.Tokens {
		if token.Name == name {
			secrets.Tokens = append(secrets.Tokens[:i], secrets.Tokens[i+1:]...)
			break
		}
	}

	return ss.Put(secrets)
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
			console.Println()
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
