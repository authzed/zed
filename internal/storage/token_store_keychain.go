package storage

import (
	"github.com/keybase/go-keychain"
)

const keychainLabel = "zed token"

type KeychainTokenStore struct{}

var _ TokenStore = KeychainTokenStore{}

func (ks KeychainTokenStore) List(redactTokens bool) ([]Token, error) {
	query := keychain.NewItem()
	query.SetSecClass(keychain.SecClassGenericPassword)
	query.SetLabel(keychainLabel)
	query.SetMatchLimit(keychain.MatchLimitAll)
	query.SetReturnAttributes(true)

	items, err := keychain.QueryItem(query)
	if err != nil {
		return nil, err
	}

	var tokens []Token
	for _, item := range items {
		token := "<redacted>"
		if !redactTokens {
			tokenWithToken, err := ks.Get(item.Account, false)
			if err != nil {
				return nil, err
			}
			token = tokenWithToken.Token
		}

		tokens = append(tokens, Token{
			Name:     item.Account,
			Endpoint: item.Service,
			Token:    token,
		})
	}

	return tokens, nil
}

func (ks KeychainTokenStore) Get(name string, redactedTokens bool) (Token, error) {
	query := keychain.NewItem()
	query.SetSecClass(keychain.SecClassGenericPassword)
	query.SetAccount(name)
	query.SetLabel(keychainLabel)
	query.SetMatchLimit(keychain.MatchLimitOne)
	query.SetReturnAttributes(true)
	query.SetReturnData(!redactedTokens)

	results, err := keychain.QueryItem(query)
	if err != nil {
		return Token{}, err
	}

	if len(results) > 1 {
		return Token{}, ErrMultipleTokens
	}

	if len(results) == 1 {
		token := "<redacted>"
		if !redactedTokens {
			token = string(results[0].Data)
		}
		return Token{
			Name:     name,
			Endpoint: results[0].Service,
			Token:    token,
		}, nil
	}

	return Token{}, ErrTokenDoesNotExist
}

func (ks KeychainTokenStore) Put(t Token) error {
	item := keychain.NewGenericPassword(t.Endpoint, t.Name, keychainLabel, []byte(t.Token), "")
	err := keychain.AddItem(item)
	if err == keychain.ErrorDuplicateItem {
		err := keychain.DeleteGenericPasswordItem(t.Endpoint, t.Name)
		if err != nil {
			return err
		}
		return ks.Put(t)
	}
	return err
}

func (ks KeychainTokenStore) Delete(name string) error {
	item := keychain.NewItem()
	item.SetSecClass(keychain.SecClassGenericPassword)
	item.SetAccount(name)
	item.SetLabel(keychainLabel)
	return keychain.DeleteItem(item)
}
