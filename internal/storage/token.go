package storage

import (
	"errors"
	"strings"
)

type Token struct {
	Name     string
	Endpoint string
	Token    string
}

var ErrTokenDoesNotExist = errors.New("token does not exist")
var ErrMultipleTokens = errors.New("multiple tokens with the same name")

type TokenStore interface {
	List(redactTokens bool) ([]Token, error)
	Get(name string, redactTokens bool) (Token, error)
	Put(Token) error
	Delete(name string) error
}

func NewTokenStore(name string) TokenStore {
	switch strings.ToLower(name) {
	case "keychain":
		return KeychainTokenStore{}
	default:
		panic("storage: unknown TokenStore")
	}
}
