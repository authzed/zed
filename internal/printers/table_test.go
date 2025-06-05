package printers

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPrintTable(t *testing.T) {
	var buf strings.Builder
	headers := []string{"CURRENT NAME", "ENDPOINT", "TOKEN", "TLS CERT"}
	rows := [][]string{
		{"my-cluster-1", "local-cluster.dedicated.authzed.dev:443", "sdbpk_<redacted>", "system"},
		{"my-cluster-2", "localhost:50051", "<redacted>", "insecure"},
	}

	PrintTable(&buf, headers, rows)
	output := buf.String()

	expectedOutput := ` CURRENT NAME  ENDPOINT                                 TOKEN             TLS CERT 
 my-cluster-1  local-cluster.dedicated.authzed.dev:443  sdbpk_<redacted>  system   
 my-cluster-2  localhost:50051                          <redacted>        insecure 
`

	require.Equal(t, expectedOutput, output)
}
