package client_test

import (
	"os"
	"path"
	"testing"

	"github.com/authzed/zed/internal/client"
	"github.com/authzed/zed/internal/storage"
	zedtesting "github.com/authzed/zed/internal/testing"

	"github.com/stretchr/testify/require"
)

func TestGetTokenWithCLIOverride(t *testing.T) {
	require := require.New(t)
	testCert, err := os.CreateTemp("", "")
	require.NoError(err)
	_, err = testCert.Write([]byte("hi"))
	require.NoError(err)
	cmd := zedtesting.CreateTestCobraCommandWithFlagValue(t,
		zedtesting.StringFlag{FlagName: "token", FlagValue: "t1", Changed: true},
		zedtesting.StringFlag{FlagName: "certificate-path", FlagValue: testCert.Name(), Changed: true},
		zedtesting.StringFlag{FlagName: "endpoint", FlagValue: "e1", Changed: true},
		zedtesting.BoolFlag{FlagName: "insecure", FlagValue: true, Changed: true},
		zedtesting.BoolFlag{FlagName: "no-verify-ca", FlagValue: true, Changed: true},
	)

	bTrue := true
	bFalse := false

	// cli args take precedence when defined
	to, err := client.GetTokenWithCLIOverride(cmd, storage.Token{})
	require.NoError(err)
	require.True(to.AnyValue())
	require.Equal("t1", to.APIToken)
	require.Equal("e1", to.Endpoint)
	require.Equal([]byte("hi"), to.CACert)
	require.Equal(&bTrue, to.Insecure)
	require.Equal(&bTrue, to.NoVerifyCA)

	// storage token takes precedence when defined
	cmd = zedtesting.CreateTestCobraCommandWithFlagValue(t,
		zedtesting.StringFlag{FlagName: "token", FlagValue: "", Changed: false},
		zedtesting.StringFlag{FlagName: "certificate-path", FlagValue: "", Changed: false},
		zedtesting.StringFlag{FlagName: "endpoint", FlagValue: "", Changed: false},
		zedtesting.BoolFlag{FlagName: "insecure", FlagValue: true, Changed: false},
		zedtesting.BoolFlag{FlagName: "no-verify-ca", FlagValue: true, Changed: false},
	)
	to, err = client.GetTokenWithCLIOverride(cmd, storage.Token{
		APIToken:   "t2",
		Endpoint:   "e2",
		CACert:     []byte("bye"),
		Insecure:   &bFalse,
		NoVerifyCA: &bFalse,
	})
	require.NoError(err)
	require.True(to.AnyValue())
	require.Equal("t2", to.APIToken)
	require.Equal("e2", to.Endpoint)
	require.Equal([]byte("bye"), to.CACert)
	require.Equal(&bFalse, to.Insecure)
	require.Equal(&bFalse, to.NoVerifyCA)
}

func TestGetCurrentTokenWithCLIOverrideWithoutConfigFile(t *testing.T) {
	// When we refactored the token setting logic, we broke the workflow where zed is used without a saved
	// context. This asserts that that workflow works.
	require := require.New(t)
	cmd := zedtesting.CreateTestCobraCommandWithFlagValue(t,
		zedtesting.StringFlag{FlagName: "token", FlagValue: "t1", Changed: true},
		zedtesting.StringFlag{FlagName: "endpoint", FlagValue: "e1", Changed: true},
		zedtesting.StringFlag{FlagName: "certificate-path", FlagValue: "", Changed: false},
		zedtesting.BoolFlag{FlagName: "insecure", FlagValue: true, Changed: true},
	)

	bTrue := true

	configStore := &storage.JSONConfigStore{ConfigPath: "/not/a/valid/path"}
	secretStore := &storage.KeychainSecretStore{ConfigPath: "/not/a/valid/path"}
	token, err := client.GetCurrentTokenWithCLIOverride(cmd, configStore, secretStore)

	// cli args take precedence when defined
	require.NoError(err)
	require.True(token.AnyValue())
	require.Equal("t1", token.APIToken)
	require.Equal("e1", token.Endpoint)
	require.Equal(&bTrue, token.Insecure)
}

func TestGetCurrentTokenWithCLIOverrideWithoutSecretFile(t *testing.T) {
	// When we refactored the token setting logic, we broke the workflow where zed is used without a saved
	// context. This asserts that that workflow works.
	require := require.New(t)
	cmd := zedtesting.CreateTestCobraCommandWithFlagValue(t,
		zedtesting.StringFlag{FlagName: "token", FlagValue: "t1", Changed: true},
		zedtesting.StringFlag{FlagName: "endpoint", FlagValue: "e1", Changed: true},
		zedtesting.StringFlag{FlagName: "certificate-path", FlagValue: "", Changed: false},
		zedtesting.BoolFlag{FlagName: "insecure", FlagValue: true, Changed: true},
	)

	bTrue := true

	tmpDir, err := os.MkdirTemp("", "")
	require.NoError(err)
	configPath := path.Join(tmpDir, "config.json")
	err = os.WriteFile(configPath, []byte("{}"), 0600)
	require.NoError(err)

	configStore := &storage.JSONConfigStore{ConfigPath: tmpDir}
	secretStore := &storage.KeychainSecretStore{ConfigPath: "/not/a/valid/path"}
	token, err := client.GetCurrentTokenWithCLIOverride(cmd, configStore, secretStore)

	// cli args take precedence when defined
	require.NoError(err)
	require.True(token.AnyValue())
	require.Equal("t1", token.APIToken)
	require.Equal("e1", token.Endpoint)
	require.Equal(&bTrue, token.Insecure)
}

func TestGetCurrentTokenWithCLIOverrideWithInsufficientArgs (t *testing.T) {
	// This is to ensure that insufficient args don't unintentionally validate.
	require := require.New(t)
	cmd := zedtesting.CreateTestCobraCommandWithFlagValue(t,
		zedtesting.StringFlag{FlagName: "token", FlagValue: "", Changed: false},
		zedtesting.StringFlag{FlagName: "endpoint", FlagValue: "e1", Changed: true},
		zedtesting.StringFlag{FlagName: "certificate-path", FlagValue: "", Changed: false},
	)

	configStore, secretStore := client.DefaultStorage()
	_, err := client.GetCurrentTokenWithCLIOverride(cmd, configStore, secretStore)

	// cli args take precedence when defined
	require.NoError(err)
}
