package storage

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/zalando/go-keyring"
)

// ErrTokenNotFound is returned if there is no Token in a ConfigStore.
var ErrTokenNotFound = errors.New("token does not exist")

type Token struct {
	Name     string
	Endpoint string
	ApiToken string
}

func (t Token) SplitApiToken() (prefix, secret string) {
	exploded := strings.Split(t.ApiToken, "_")
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

func (k KeychainSecretStore) Get() (Secrets, error) {
	entry, err := keyring.Get("zed", keyringEntryName)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return Secrets{}, nil // empty is okay!
		}
		return Secrets{}, err
	}

	var s Secrets
	err = json.Unmarshal([]byte(entry), &s)
	return s, err
}

func (k KeychainSecretStore) Put(s Secrets) error {
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}

	return keyring.Set("zed", keyringEntryName, string(data))
}
