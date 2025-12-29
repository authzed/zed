package cmd

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"github.com/authzed/spicedb/pkg/composableschemadsl/compiler"

	zedtesting "github.com/authzed/zed/internal/testing"
)

func TestDeterminePrefixForSchema(t *testing.T) {
	tests := []struct {
		name            string
		existingSchema  string
		specifiedPrefix string
		expectedPrefix  string
	}{
		{
			"empty schema",
			"",
			"",
			"",
		},
		{
			"no prefix, none specified",
			`definition user {}`,
			"",
			"",
		},
		{
			"no prefix, one specified",
			`definition user {}`,
			"test",
			"test",
		},
		{
			"prefix found",
			`definition test/user {}`,
			"",
			"test",
		},
		{
			"multiple prefixes found",
			`definition test/user {}
			
			definition something/resource {}`,
			"",
			"",
		},
		{
			"multiple prefixes found, one specified",
			`definition test/user {}
			
			definition something/resource {}`,
			"foobar",
			"foobar",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			found, err := determinePrefixForSchema(t.Context(), test.specifiedPrefix, nil, &test.existingSchema)
			require.NoError(t, err)
			require.Equal(t, test.expectedPrefix, found)
		})
	}
}

func TestRewriteSchema(t *testing.T) {
	tests := []struct {
		name             string
		existingSchema   string
		definitionPrefix string
		expectedSchema   string
	}{
		{
			"empty schema",
			"",
			"",
			"",
		},
		{
			"empty prefix schema",
			"definition user {}",
			"",
			"definition user {}",
		},
		{
			"empty prefix schema with specified",
			`definition user {}
			
			caveat some_caveat(someCondition int) { someCondition == 42 }
			`,
			"test",
			`definition test/user {}

caveat test/some_caveat(someCondition int) {
	someCondition == 42
}`,
		},
		{
			"prefixed schema with specified",
			"definition foo/user {}",
			"test",
			"definition foo/user {}",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			found, err := rewriteSchema(test.existingSchema, test.definitionPrefix)
			require.NoError(t, err)
			require.Equal(t, test.expectedSchema, found)
		})
	}
}

// testWriter is a simple writer that captures bytes to a buffer
type testWriter struct {
	buffer *[]byte
}

func (tw *testWriter) Write(p []byte) (n int, err error) {
	*tw.buffer = append(*tw.buffer, p...)
	return len(p), nil
}

func TestSchemaCompileOuter(t *testing.T) {
	t.Parallel()

	f := filepath.Join(t.TempDir(), uuid.NewString())

	testCases := map[string]struct {
		outFile      string
		expectStdout bool
	}{
		`use_stdout`: {
			expectStdout: true,
		},
		`use_file`: {
			outFile:      f,
			expectStdout: false,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			files := []string{filepath.Join("preview-test", "composable-schema-root.zed")}
			expected := `definition user {}

definition resource {
	relation user: user
	permission view = user
}
`
			cmd := zedtesting.CreateTestCobraCommandWithFlagValue(t,
				zedtesting.StringFlag{FlagName: "out", FlagValue: tc.outFile},
			)
			usedStdout, err := schemaCompileOuter(cmd, files)
			require.NoError(t, err)
			require.Equal(t, tc.expectStdout, usedStdout)
			if tc.outFile != "" {
				tempOutString, err := os.ReadFile(tc.outFile)
				require.NoError(t, err)
				require.Equal(t, expected, string(tempOutString))
			}
		})
	}
}

func TestSchemaCompileInner(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	files := []string{filepath.Join("preview-test", "composable-schema-root.zed")}
	expected := `definition user {}

definition resource {
	relation user: user
	permission view = user
}
`

	var buf []byte
	writer := &testWriter{buffer: &buf}

	err := schemaCompileInner(files, writer)

	require.NoError(err)
	require.Equal(expected, string(buf))
}

func TestSchemaCompileFileNotFound(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	files := []string{filepath.Join("preview-test", "nonexistent.zed")}

	err := schemaCompileInner(files, io.Discard)
	require.Error(err)
	require.ErrorIs(err, fs.ErrNotExist)
}

func TestSchemaCompileFailureFromReservedKeyword(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	files := []string{filepath.Join("preview-test", "composable-schema-invalid-root.zed")}
	var expectedErr compiler.BaseCompilerError

	err := schemaCompileInner(files, io.Discard)
	require.Error(err)
	require.ErrorAs(err, &expectedErr)
}

// TODO: refactor the impl function to provide a pipe or buffer directly and delegate the input selection to
// another function
//
//nolint:tparallel // these tests can't be parallel because they muck around with the definition of os.Stdin.
func TestSchemaWrite(t *testing.T) {
	t.Parallel()

	// Save original stdin
	oldStdin := os.Stdin
	t.Cleanup(func() {
		os.Stdin = oldStdin
	})

	testCases := map[string]struct {
		schemaMakerFn       func() ([]string, error)
		terminalChecker     *mockTermChecker
		expectErr           string
		expectSchemaWritten string
	}{
		`schema_from_file`: {
			schemaMakerFn: func() ([]string, error) {
				return []string{
					filepath.Join("write-schema-test", "basic.zed"),
				}, nil
			},
			expectSchemaWritten: `definition user {}
definition resource {
  relation view: user
  permission viewer = view
}`,
			terminalChecker: &mockTermChecker{returnVal: false},
		},
		`schema_from_stdin`: {
			schemaMakerFn: func() ([]string, error) {
				schemaContent := "definition user{}\ndefinition document { relation read: user }"
				pipeRead, pipeWrite, err := os.Pipe()
				require.NoError(t, err)
				os.Stdin = pipeRead
				_, err = pipeWrite.WriteString(schemaContent)
				require.NoError(t, err)
				err = pipeWrite.Close()
				require.NoError(t, err)
				return []string{}, nil
			},
			terminalChecker:     &mockTermChecker{returnVal: false},
			expectSchemaWritten: "definition user{}\ndefinition document { relation read: user }",
		},
		`schema_from_stdin_but_terminal`: {
			schemaMakerFn: func() ([]string, error) {
				schemaContent := "definition user{}\ndefinition document { relation read: user }"
				pipeRead, pipeWrite, err := os.Pipe()
				require.NoError(t, err)
				os.Stdin = pipeRead
				_, err = pipeWrite.WriteString(schemaContent)
				require.NoError(t, err)
				err = pipeWrite.Close()
				require.NoError(t, err)
				return []string{}, nil
			},
			terminalChecker: &mockTermChecker{returnVal: true},
			expectErr:       "must provide file path or contents via stdin",
		},
		`empty_schema_errors`: {
			schemaMakerFn: func() ([]string, error) {
				pipeRead, pipeWrite, err := os.Pipe()
				require.NoError(t, err)
				os.Stdin = pipeRead
				_, err = pipeWrite.WriteString("")
				require.NoError(t, err)
				err = pipeWrite.Close()
				require.NoError(t, err)
				return []string{}, nil
			},
			terminalChecker: &mockTermChecker{returnVal: false},
			expectErr:       "attempted to write empty schema",
		},
		`write_failure_errors`: {
			schemaMakerFn: func() ([]string, error) {
				return []string{
					filepath.Join("write-schema-test", "basic.zed"),
				}, errors.New("write error")
			},
			terminalChecker: &mockTermChecker{returnVal: false},
			expectErr:       "error writing schema",
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			cmd := zedtesting.CreateTestCobraCommandWithFlagValue(t,
				zedtesting.StringFlag{FlagName: "schema-definition-prefix", FlagValue: ""},
				zedtesting.BoolFlag{FlagName: "json", FlagValue: true},
			)

			args, writeErr := tc.schemaMakerFn()
			mockWriteSchemaClientt := &mockWriteSchemaClient{}
			if writeErr != nil {
				mockWriteSchemaClientt.writeReturnsError = true
			}

			err := schemaWriteCmdImpl(cmd, args, mockWriteSchemaClientt, tc.terminalChecker)

			if tc.expectErr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.expectErr)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.expectSchemaWritten, mockWriteSchemaClientt.receivedSchema)
			if tc.terminalChecker.captured {
				require.Equal(t, int(os.Stdin.Fd()), tc.terminalChecker.capturedFd, "expected stdin to be checked for terminal")
			}
		})
	}
}

type mockWriteSchemaClient struct {
	existingSchema    string
	receivedSchema    string
	writeReturnsError bool
}

var _ v1.SchemaServiceClient = (*mockWriteSchemaClient)(nil)

func (m *mockWriteSchemaClient) WriteSchema(_ context.Context, in *v1.WriteSchemaRequest, _ ...grpc.CallOption) (*v1.WriteSchemaResponse, error) {
	if m.writeReturnsError {
		return nil, errors.New("error writing schema")
	}
	m.receivedSchema = in.Schema
	return &v1.WriteSchemaResponse{}, nil
}

func (m *mockWriteSchemaClient) ReadSchema(_ context.Context, _ *v1.ReadSchemaRequest, _ ...grpc.CallOption) (*v1.ReadSchemaResponse, error) {
	return &v1.ReadSchemaResponse{
		SchemaText: m.existingSchema,
	}, nil
}

func (m *mockWriteSchemaClient) ReflectSchema(_ context.Context, _ *v1.ReflectSchemaRequest, _ ...grpc.CallOption) (*v1.ReflectSchemaResponse, error) {
	panic("not implemented")
}

func (m *mockWriteSchemaClient) ComputablePermissions(_ context.Context, _ *v1.ComputablePermissionsRequest, _ ...grpc.CallOption) (*v1.ComputablePermissionsResponse, error) {
	panic("not implemented")
}

func (m *mockWriteSchemaClient) DependentRelations(_ context.Context, _ *v1.DependentRelationsRequest, _ ...grpc.CallOption) (*v1.DependentRelationsResponse, error) {
	panic("not implemented")
}

func (m *mockWriteSchemaClient) DiffSchema(_ context.Context, _ *v1.DiffSchemaRequest, _ ...grpc.CallOption) (*v1.DiffSchemaResponse, error) {
	panic("not implemented")
}

type mockTermChecker struct {
	returnVal  bool
	captured   bool
	capturedFd int
}

var _ termChecker = (*mockTermChecker)(nil)

func (m *mockTermChecker) IsTerminal(fd int) bool {
	m.captured = true
	m.capturedFd = fd
	return m.returnVal
}
