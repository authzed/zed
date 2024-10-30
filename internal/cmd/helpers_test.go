package cmd

import (
	"bufio"
	"os"
	"testing"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"github.com/authzed/spicedb/pkg/tuple"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"

	"github.com/authzed/zed/pkg/backupformat"
)

func mapRelationshipTuplesToCLIOutput(t *testing.T, input []string) []string {
	t.Helper()

	return lo.Map[string, string](input, func(item string, _ int) string {
		return replaceRelString(item)
	})
}

func readLines(t *testing.T, fileName string) []string {
	t.Helper()

	f, err := os.Open(fileName)
	require.NoError(t, err)
	defer func() {
		_ = f.Close()
	}()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	return lines
}

func createTestBackup(t *testing.T, schema string, relationships []string) string {
	t.Helper()

	f, err := os.CreateTemp("", "test-backup")
	require.NoError(t, err)
	defer f.Close()
	t.Cleanup(func() {
		_ = os.Remove(f.Name())
	})

	avroWriter, err := backupformat.NewEncoder(f, schema, &v1.ZedToken{Token: "test"})
	require.NoError(t, err)
	defer func() {
		require.NoError(t, avroWriter.Close())
	}()

	for _, rel := range relationships {
		r := tuple.MustParseV1Rel(rel)
		require.NoError(t, avroWriter.Append(r))
	}

	return f.Name()
}
