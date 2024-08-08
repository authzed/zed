package storage

import (
	"encoding/json"
	"errors"
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
	Name       string
	Endpoint   string
	APIToken   string
	Insecure   *bool
	NoVerifyCA *bool
	CACert     []byte
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

func (t Token) HasNoVerifyCA() bool {
	return t.NoVerifyCA != nil && *t.NoVerifyCA
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
	ring       keyring.Keyring
}

var _ SecretStore = (*KeychainSecretStore)(nil)

const (
	svcName                   = "zed"
	keyringEntryName          = svcName + " secrets"
	envRecommendation         = "Setting the environment variable `ZED_KEYRING_PASSWORD` to your password will skip prompts\n"
	keyringDoesNotExistPrompt = "Keyring file does not already exist.\nEnter a new non-empty passphrase for the new keyring file: "
	keyringPrompt             = "Enter passphrase to unlock zed keyring: "
	emptyKeyringPasswordError = "Your passphrase must not be empty."
)

func fileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	switch {
	case err == nil:
		return true, nil
	case os.IsNotExist(err):
		return false, nil
	default:
		return false, err
	}
}

func (k *KeychainSecretStore) keyring() (keyring.Keyring, error) {
	if k.ring != nil {
		return k.ring, nil
	}

	keyringPath := filepath.Join(k.ConfigPath, "keyring.jwt")

	ring, err := keyring.Open(keyring.Config{
		ServiceName: "zed",
		FileDir:     keyringPath,
		FilePasswordFunc: func(_ string) (string, error) {
			if password, ok := os.LookupEnv("ZED_KEYRING_PASSWORD"); ok {
				return password, nil
			}

			prompt := keyringPrompt

			// Check if this is the first run where the keyring is created.
			keyringExists, err := fileExists(filepath.Join(keyringPath, keyringEntryName))
			if err != nil {
				return "", err
			}
			if !keyringExists {
				// This is the first run and we're creating a password.
				prompt = keyringDoesNotExistPrompt
			}

			console.Printf(envRecommendation + prompt)
			b, err := term.ReadPassword(int(os.Stdin.Fd()))
			if err != nil {
				return "", err
			}
			console.Printf("\n") // Clear the line after a prompt

			passwordString := string(b)
			if len(passwordString) == 0 {
				// NOTE: we enforce a non-empty keyring password to prevent
				// user frustration around accidentally setting an empty
				// passphrase and then not knowing what it might be.
				return "", errors.New(emptyKeyringPasswordError)
			}

			return passwordString, nil
		},
	})
	if err != nil {
		return ring, err
	}

	k.ring = ring
	return ring, err
}

func (k *KeychainSecretStore) Get() (Secrets, error) {
	ring, err := k.keyring()
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

func (k *KeychainSecretStore) Put(s Secrets) error {
	ring, err := k.keyring()
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
