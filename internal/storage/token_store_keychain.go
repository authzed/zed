package storage

import (
	"github.com/99designs/keyring"
)

const keychainSvcName = "zed tokens"

type KeychainTokenStore struct{}

var _ TokenStore = KeychainTokenStore{}

func (ks KeychainTokenStore) List(redactTokens bool) ([]Token, error) {
	ring, err := keyring.Open(keyring.Config{
		ServiceName: keychainSvcName,
	})
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
			Token:    token,
		})
	}

	return tokens, nil
}

func (ks KeychainTokenStore) Get(name string, redactTokens bool) (Token, error) {
	ring, err := keyring.Open(keyring.Config{
		ServiceName: keychainSvcName,
	})
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
		Token:    token,
	}, nil
}

func (ks KeychainTokenStore) Put(t Token) error {
	ring, err := keyring.Open(keyring.Config{
		ServiceName: keychainSvcName,
	})
	if err != nil {
		return err
	}

	err = ring.Set(keyring.Item{
		Key:   t.Name,
		Data:  []byte(t.Token),
		Label: t.Endpoint,
	})
	return err
}

func (ks KeychainTokenStore) Delete(name string) error {
	ring, err := keyring.Open(keyring.Config{
		ServiceName: keychainSvcName,
	})
	if err != nil {
		return err
	}

	return ring.Remove(name)
}
