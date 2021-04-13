// keychain implements a very simple abstraction over the macOS and dbus
// keychain.
package keychain

import (
	"fmt"

	"github.com/keybase/go-keychain"
)

func ListByLabel(label string) ([]keychain.QueryResult, error) {
	query := keychain.NewItem()
	query.SetSecClass(keychain.SecClassGenericPassword)
	query.SetLabel(label)
	query.SetMatchLimit(keychain.MatchLimitAll)
	query.SetReturnAttributes(true)

	results, err := keychain.QueryItem(query)
	if err != nil {
		return nil, err
	}

	return results, nil
}

func Get(account, label string, data bool) (*keychain.QueryResult, error) {
	query := keychain.NewItem()
	query.SetSecClass(keychain.SecClassGenericPassword)
	query.SetAccount(account)
	query.SetLabel(label)
	query.SetMatchLimit(keychain.MatchLimitOne)
	query.SetReturnAttributes(true)
	query.SetReturnData(data)

	results, err := keychain.QueryItem(query)
	if err != nil {
		return nil, err
	}

	if len(results) > 1 {
		return nil, fmt.Errorf("Too many results")
	}

	if len(results) == 1 {
		return &results[0], nil
	}

	return nil, nil
}

func Delete(account, label string) error {
	item := keychain.NewItem()
	item.SetSecClass(keychain.SecClassGenericPassword)
	item.SetAccount(account)
	item.SetLabel(label)

	return keychain.DeleteItem(item)
}

func Put(svc, account, label, data string) error {
	item := keychain.NewGenericPassword(svc, account, label, []byte(data), "")
	err := keychain.AddItem(item)
	if err == keychain.ErrorDuplicateItem {
		err := keychain.DeleteGenericPasswordItem(svc, account)
		if err != nil {
			return err
		}
		return Put(svc, account, label, data)
	}
	return err
}
