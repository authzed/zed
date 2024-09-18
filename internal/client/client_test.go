package client_test

import (
	"os"
	"testing"

	"github.com/authzed/zed/internal/client"
	"github.com/authzed/zed/internal/storage"
	zedtesting "github.com/authzed/zed/internal/testing"

	"github.com/stretchr/testify/require"
)

func TestGetTokenWithCLIOverride(t *testing.T) {
	testCert, err := os.CreateTemp("", "")
	require.NoError(t, err)
	_, err = testCert.Write([]byte("hi"))
	require.NoError(t, err)
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
	require.NoError(t, err)
	require.True(t, to.AnyValue())
	require.Equal(t, "t1", to.APIToken)
	require.Equal(t, "e1", to.Endpoint)
	require.Equal(t, []byte("hi"), to.CACert)
	require.Equal(t, &bTrue, to.Insecure)
	require.Equal(t, &bTrue, to.NoVerifyCA)

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
	require.NoError(t, err)
	require.True(t, to.AnyValue())
	require.Equal(t, "t2", to.APIToken)
	require.Equal(t, "e2", to.Endpoint)
	require.Equal(t, []byte("bye"), to.CACert)
	require.Equal(t, &bFalse, to.Insecure)
	require.Equal(t, &bFalse, to.NoVerifyCA)
}
