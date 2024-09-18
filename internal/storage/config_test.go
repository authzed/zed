package storage

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTokenWithOverride(t *testing.T) {
	bTrue := true
	referenceToken := Token{
		Name:       "n1",
		Endpoint:   "e1",
		APIToken:   "a1",
		Insecure:   &bTrue,
		NoVerifyCA: &bTrue,
		CACert:     []byte("c1"),
	}

	bFalse := false
	override := Token{
		Name:       "n2",
		Endpoint:   "e2",
		APIToken:   "a2",
		Insecure:   &bFalse,
		NoVerifyCA: &bFalse,
		CACert:     []byte("c2"),
	}

	result, err := TokenWithOverride(override, referenceToken)
	require.NoError(t, err)
	require.Equal(t, "n1", result.Name)
	require.Equal(t, "e2", result.Endpoint)
	require.Equal(t, "a2", result.APIToken)
	require.Equal(t, false, *result.Insecure)
	require.Equal(t, false, *result.NoVerifyCA)
	require.Equal(t, 0, bytes.Compare([]byte("c2"), result.CACert))

	result, err = TokenWithOverride(Token{}, referenceToken)
	require.NoError(t, err)
	require.Equal(t, "n1", result.Name)
	require.Equal(t, "e1", result.Endpoint)
	require.Equal(t, "a1", result.APIToken)
	require.Equal(t, true, *result.Insecure)
	require.Equal(t, true, *result.NoVerifyCA)
	require.Equal(t, 0, bytes.Compare([]byte("c1"), result.CACert))
}
