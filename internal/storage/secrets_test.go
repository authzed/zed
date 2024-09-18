package storage

import (
	"testing"

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
}
