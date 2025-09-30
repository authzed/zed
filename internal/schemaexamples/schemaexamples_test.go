package schemaexamples

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestListExampleSchemas(t *testing.T) {
	schemas, err := ListExampleSchemas()
	require.NoError(t, err)
	require.NotEmpty(t, schemas, "Expected at least one schema")

	for i, schema := range schemas {
		require.NotEmpty(t, schema, "Schema at index %d should not be empty", i)
	}
}
