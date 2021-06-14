package storage

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/99designs/keyring"
	"github.com/jzelinskie/stringz"
	"golang.org/x/term"
)

var (
	DefaultTokenStore = KeychainTokenStore{}

	ErrTokenDoesNotExist = errors.New("token does not exist")
	ErrMultipleTokens    = errors.New("multiple tokens with the same name")
)

type Token struct {
	Name     string
	Endpoint string
	Prefix   string
	Secret   string
}

type TokenStore interface {
	List(revealTokens bool) ([]Token, error)
	Get(name string, revealTokens bool) (Token, error)
	Put(name, endpoint, secret string) error
	Delete(name string) error
}

const (
	keychainSvcName = "zed tokens"
	keyringFilename = "keyring.jwt"
	redactedMessage = "<redacted>"
)

type KeychainTokenStore struct{}

var _ TokenStore = KeychainTokenStore{}

func openKeyring() (keyring.Keyring, error) {
	path, err := localConfigPath()
	if err != nil {
		return nil, err
	}

	return keyring.Open(keyring.Config{
		ServiceName: keychainSvcName,
		FileDir:     filepath.Join(path, keyringFilename),
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

func encodeLabel(prefix, endpoint string) string {
	return stringz.Join("@", prefix, endpoint)
}

func decodeLabel(label string) (prefix, endpoint string) {
	if err := stringz.SplitExact(label, "@", &prefix, &endpoint); err != nil {
		return "", label
	}
	return prefix, endpoint
}

func splitAPIToken(token string) (prefix, secret string) {
	exploded := strings.Split(token, "_")
	return strings.Join(exploded[:len(exploded)-1], "_"), exploded[len(exploded)-1]
}

func (ks KeychainTokenStore) List(revealTokens bool) ([]Token, error) {
	ring, err := openKeyring()
	if err != nil {
		return nil, err
	}

	keys, err := ring.Keys()
	if err != nil {
		return nil, err
	}

	var tokens []Token
	for _, key := range keys {
		item, err := ring.Get(key)
		if err != nil {
			return nil, err
		}

		prefix, endpoint := decodeLabel(item.Label)
		secret := redactedMessage
		if revealTokens {
			secret = string(item.Data)
		}

		tokens = append(tokens, Token{
			Name:     item.Key,
			Endpoint: endpoint,
			Prefix:   prefix,
			Secret:   secret,
		})
	}

	return tokens, nil
}

func (ks KeychainTokenStore) Get(name string, revealTokens bool) (Token, error) {
	ring, err := openKeyring()
	if err != nil {
		return Token{}, err
	}

	item, err := ring.Get(name)
	if err != nil {
		if err == keyring.ErrKeyNotFound {
			return Token{}, ErrTokenDoesNotExist
		}
		return Token{}, err
	}

	prefix, endpoint := decodeLabel(item.Label)
	token := redactedMessage
	if revealTokens {
		token = string(item.Data)
	}

	return Token{
		Name:     item.Key,
		Endpoint: endpoint,
		Prefix:   prefix,
		Secret:   token,
	}, nil
}

func (ks KeychainTokenStore) Put(name, endpoint, secret string) error {
	prefix, secret := splitAPIToken(secret)

	ring, err := openKeyring()
	if err != nil {
		return err
	}

	return ring.Set(keyring.Item{
		Key:   name,
		Data:  []byte(secret),
		Label: encodeLabel(prefix, endpoint),
	})
}

func (ks KeychainTokenStore) Delete(name string) error {
	ring, err := openKeyring()
	if err != nil {
		return err
	}

	return ring.Remove(name)
}
