// keychain implements a very simple abstraction over the macOS and dbus
// keychain.
package keychain

import (
	"github.com/keybase/go-keychain"
)

func List(svc string) ([]string, error) {
	return keychain.GetAccountsForService(svc)
}

func Get(svc, account string) (password []byte, err error) {
	return keychain.GetGenericPassword(svc, account, svc, "")
}

func Delete(svc, account string) error {
	return keychain.DeleteGenericPasswordItem(svc, account)
}

func Put(svc, account string, password []byte) error {
	keychain.GetGenericPassword(svc, account, svc, "")
	item := keychain.NewGenericPassword(svc, account, svc, password, "")
	err := keychain.AddItem(item)
	if err == keychain.ErrorDuplicateItem {
		err := keychain.DeleteGenericPasswordItem(svc, account)
		if err != nil {
			return err
		}
		return Put(svc, account, password)
	}
	return err
}
