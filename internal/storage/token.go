package storage

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/99designs/keyring"
	"golang.org/x/term"
)

type Token struct {
	Name     string
	Endpoint string
	ApiToken string
}

var ErrTokenDoesNotExist = errors.New("token does not exist")
var ErrMultipleTokens = errors.New("multiple tokens with the same name")

type TokenStore interface {
	List(redactTokens bool) ([]Token, error)
	Get(name string, redactTokens bool) (Token, error)
	Put(Token) error
	Delete(name string) error
}

const keychainSvcName = "zed tokens"

type KeychainTokenStore struct{}

var _ TokenStore = KeychainTokenStore{}

func openKeyring() (keyring.Keyring, error) {
	path, err := localConfigPath()
	if err != nil {
		return nil, err
	}

	return keyring.Open(keyring.Config{
		ServiceName: keychainSvcName,
		FileDir:     filepath.Join(path, "keyring"),
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

func (ks KeychainTokenStore) List(redactTokens bool) ([]Token, error) {
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

		token := "<redacted>"
		if !redactTokens {
			token = string(item.Data)
		}

		tokens = append(tokens, Token{
			Name:     item.Key,
			Endpoint: item.Label,
			ApiToken: token,
		})
	}

	return tokens, nil
}

func (ks KeychainTokenStore) Get(name string, redactTokens bool) (Token, error) {
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

	token := "<redacted>"
	if !redactTokens {
		token = string(item.Data)
	}

	return Token{
		Name:     item.Key,
		Endpoint: item.Label,
		ApiToken: token,
	}, nil
}

func (ks KeychainTokenStore) Put(t Token) error {
	ring, err := openKeyring()
	if err != nil {
		return err
	}

	err = ring.Set(keyring.Item{
		Key:   t.Name,
		Data:  []byte(t.ApiToken),
		Label: t.Endpoint,
	})
	return err
}

func (ks KeychainTokenStore) Delete(name string) error {
	ring, err := openKeyring()
	if err != nil {
		return err
	}

	return ring.Remove(name)
}
