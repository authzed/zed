package cmd

import (
	"bufio"
	"os"
	"testing"

	"github.com/samber/lo"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/authzed/spicedb/pkg/tuple"

	"github.com/authzed/zed/pkg/backupformat"
)

func mapRelationshipTuplesToCLIOutput(t *testing.T, input []string) []string {
	t.Helper()

	return lo.Map(input, func(item string, _ int) string {
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

// createTestBackup creates a test backup file with the given schema and relationships.
// It returns the file name of the created backup.
// When the test is done, the file is closed and removed.
func createTestBackup(t *testing.T, cmd *cobra.Command, schema string, relationships []string) string {
	t.Helper()

	f, err := os.CreateTemp(t.TempDir(), "test-backup")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = f.Close()
		_ = os.Remove(f.Name())
	})

	avroWriter := backupformat.NewOcfEncoder(f)
	encoder := &backupformat.RewriteEncoder{Rewriter: backupformat.RewriterFromFlags(cmd), Encoder: avroWriter}
	defer func() {
		require.NoError(t, avroWriter.Close())
	}()
	require.NoError(t, encoder.WriteSchema(schema, "test"))

	for _, rel := range relationships {
		r := tuple.MustParseV1Rel(rel)
		require.NoError(t, encoder.Append(r, ""))
	}

	return f.Name()
}
