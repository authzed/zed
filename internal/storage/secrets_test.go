package storage

import (
	"testing"

	"github.com/99designs/keyring"
	"github.com/stretchr/testify/require"
)

func TestTokenAnyValue(t *testing.T) {
	b := false

	require.False(t, Token{}.AnyValue())
	require.False(t, Token{}.AnyValue())
	require.True(t, Token{Endpoint: "foo"}.AnyValue())
	require.True(t, Token{APIToken: "foo"}.AnyValue())
	require.True(t, Token{Insecure: &b}.AnyValue())
	require.True(t, Token{NoVerifyCA: &b}.AnyValue())
	require.True(t, Token{CACert: []byte("a")}.AnyValue())
	require.True(t, Token{HostnameOverride: "foo"}.AnyValue())
}

func TestAllowedBackendsFromEnv(t *testing.T) {
	tests := []struct {
		name            string
		keyringPassword string
		setPassword     bool
		keyringBackend  string
		setBackend      bool
		expected        []keyring.BackendType
		expectError     bool
	}{
		{
			name:     "neither set",
			expected: nil,
		},
		{
			name:            "password alone forces file backend",
			keyringPassword: "hunter2",
			setPassword:     true,
			expected:        []keyring.BackendType{keyring.FileBackend},
		},
		{
			name:            "empty password does not force file backend",
			keyringPassword: "",
			setPassword:     true,
			expected:        nil,
		},
		{
			name:            "backend overrides password",
			keyringPassword: "hunter2",
			setPassword:     true,
			keyringBackend:  "secret-service",
			setBackend:      true,
			expected:        []keyring.BackendType{keyring.SecretServiceBackend},
		},
		{
			name:           "backend alone is honored",
			keyringBackend: "kwallet",
			setBackend:     true,
			expected:       []keyring.BackendType{keyring.KWalletBackend},
		},
		{
			name:           "invalid backend errors",
			keyringBackend: "not-a-real-backend",
			setBackend:     true,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setPassword {
				t.Setenv("ZED_KEYRING_PASSWORD", tt.keyringPassword)
			}
			if tt.setBackend {
				t.Setenv("ZED_KEYRING_BACKEND", tt.keyringBackend)
			}

			backends, err := allowedBackendsFromEnv()
			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.expected, backends)
		})
	}
}
