package cmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/authzed/zed/internal/client"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"github.com/authzed/spicedb/pkg/tuple"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func init() {
	zerolog.SetGlobalLevel(zerolog.Disabled)
}

const testSchema = `definition test/resource {
	relation reader: test/user
}

definition test/user {}`

var testRelationships = []string{
	`test/resource:1#reader@test/user:1`,
	`test/resource:2#reader@test/user:2`,
	`test/resource:3#reader@test/user:3`,
}

func TestFilterSchemaDefs(t *testing.T) {
	for _, tt := range []struct {
		name         string
		inputSchema  string
		inputPrefix  string
		outputSchema string
		err          string
	}{
		{
			name:         "empty schema returns as is",
			inputSchema:  "",
			inputPrefix:  "",
			outputSchema: "",
			err:          "",
		},
		{
			name:         "no input prefix matches everything",
			inputSchema:  "definition test {}",
			inputPrefix:  "",
			outputSchema: "definition test {}",
			err:          "",
		},
		{
			name:         "filter over schema without prefix filters everything",
			inputSchema:  "definition test {}",
			inputPrefix:  "myprefix",
			outputSchema: "",
			err:          "filtered all definitions from schema",
		},
		{
			name:         "filter over schema with same prefix returns schema as is",
			inputSchema:  "definition myprefix/test {}\n\ndefinition myprefix/test2 {}",
			inputPrefix:  "myprefix",
			outputSchema: "definition myprefix/test {}\n\ndefinition myprefix/test2 {}",
			err:          "",
		},
		{
			name:         "filter over schema with different prefixes filters non matching namespaces",
			inputSchema:  "definition myprefix/test {}\n\ndefinition myprefix2/test2 {}",
			inputPrefix:  "myprefix",
			outputSchema: "definition myprefix/test {}",
			err:          "",
		},
		{
			name:         "filter over schema caveats with same prefixes returns as is",
			inputSchema:  "caveat myprefix/one(a int) {\n\ta == 1\n}",
			inputPrefix:  "myprefix",
			outputSchema: "caveat myprefix/one(a int) {\n\ta == 1\n}",
			err:          "",
		},
		{
			name:         "filter over unprefixed schema caveats with a prefix filters out",
			inputSchema:  "caveat one(a int) {\n\ta == 1\n}",
			inputPrefix:  "myprefix",
			outputSchema: "",
			err:          "filtered all definitions from schema",
		},
		{
			name:         "filter over schema mixed prefixed/unprefixed caveats filters out",
			inputSchema:  "caveat one(a int) {\n\ta == 1\n}\n\ncaveat myprefix/two(a int) {\n\ta == 2\n}",
			inputPrefix:  "myprefix",
			outputSchema: "caveat myprefix/two(a int) {\n\ta == 2\n}",
			err:          "",
		},
		{
			name:         "filter over schema namespaces and caveats with same prefixes returns as is",
			inputSchema:  "definition myprefix/test {}\n\ncaveat myprefix/one(a int) {\n\ta == 1\n}",
			inputPrefix:  "myprefix",
			outputSchema: "definition myprefix/test {}\n\ncaveat myprefix/one(a int) {\n\ta == 1\n}",
			err:          "",
		},
		{
			name:         "fails on invalid schema",
			inputSchema:  "definition a/test {}\n\ncaveat a/one(a int) {\n\ta == 1\n}",
			inputPrefix:  "a",
			outputSchema: "definition a/test {}\n\ncaveat a/one(a int) {\n\ta == 1\n}",
			err:          "value does not match regex pattern",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tt := tt
			t.Parallel()

			outputSchema, err := filterSchemaDefs(tt.inputSchema, tt.inputPrefix)
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.outputSchema, outputSchema)
			}
		})
	}
}

func TestBackupParseRelsCmdFunc(t *testing.T) {
	for _, tt := range []struct {
		name          string
		filter        string
		schema        string
		relationships []string
		output        []string
		err           string
	}{
		{
			name:          "basic test",
			filter:        "test",
			schema:        testSchema,
			relationships: testRelationships,
			output:        mapRelationshipTuplesToCLIOutput(t, testRelationships),
		},
		{
			name:          "filters out",
			filter:        "test",
			schema:        testSchema,
			relationships: append([]string{"foo/user:0#reader@foo/resource:0"}, testRelationships...),
			output:        mapRelationshipTuplesToCLIOutput(t, testRelationships),
		},
		{
			name:          "allows empty backup",
			filter:        "test",
			schema:        testSchema,
			relationships: nil,
			output:        nil,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tt := tt
			t.Parallel()

			cmd := createTestCobraCommandWithFlagValue(t, stringFlag{"prefix-filter", tt.filter})
			backupName := createTestBackup(t, tt.schema, tt.relationships)
			f, err := os.CreateTemp("", "parse-output")
			require.NoError(t, err)
			defer func() {
				_ = f.Close()
			}()
			t.Cleanup(func() {
				_ = os.Remove(f.Name())
			})

			err = backupParseRelsCmdFunc(cmd, f, []string{backupName})
			require.NoError(t, err)

			lines := readLines(t, f.Name())
			require.Equal(t, tt.output, lines)
		})
	}
}

func TestBackupParseRevisionCmdFunc(t *testing.T) {
	cmd := createTestCobraCommandWithFlagValue(t, stringFlag{"prefix-filter", "test"})
	backupName := createTestBackup(t, testSchema, testRelationships)
	f, err := os.CreateTemp("", "parse-output")
	require.NoError(t, err)
	defer func() {
		_ = f.Close()
	}()
	t.Cleanup(func() {
		_ = os.Remove(f.Name())
	})

	err = backupParseRevisionCmdFunc(cmd, f, []string{backupName})
	require.NoError(t, err)

	lines := readLines(t, f.Name())
	require.Equal(t, []string{"test"}, lines)
}

func TestBackupParseSchemaCmdFunc(t *testing.T) {
	for _, tt := range []struct {
		name          string
		filter        string
		rewriteLegacy bool
		schema        string
		output        []string
		err           string
	}{
		{
			name:   "basic schema test",
			filter: "test",
			schema: testSchema,
			output: strings.Split(testSchema, "\n"),
		},
		{
			name:   "filters schema definitions",
			filter: "test",
			schema: "definition test/user {}\n\ndefinition foo/user {}",
			output: []string{"definition test/user {}"},
		},
		{
			name:          "rewrites short relations",
			filter:        "",
			rewriteLegacy: true,
			schema:        "definition user {relation aa: user}",
			output:        []string{"definition user {", "/* deleted short relation name */"},
		},
		{
			name:          "rewrites legacy missing allowed types",
			filter:        "",
			rewriteLegacy: true,
			schema:        "definition user { relation foo /* missing allowed types */}",
			output:        []string{"definition user {", "/* deleted missing allowed type error */"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tt := tt
			t.Parallel()

			cmd := createTestCobraCommandWithFlagValue(t,
				stringFlag{"prefix-filter", tt.filter},
				boolFlag{"rewrite-legacy", tt.rewriteLegacy})
			backupName := createTestBackup(t, tt.schema, nil)
			f, err := os.CreateTemp("", "parse-output")
			require.NoError(t, err)
			defer func() {
				_ = f.Close()
			}()
			t.Cleanup(func() {
				_ = os.Remove(f.Name())
			})

			err = backupParseSchemaCmdFunc(cmd, f, []string{backupName})
			require.NoError(t, err)

			lines := readLines(t, f.Name())
			require.Equal(t, tt.output, lines)
		})
	}
}

func TestBackupCreateCmdFunc(t *testing.T) {
	cmd := createTestCobraCommandWithFlagValue(t,
		stringFlag{"prefix-filter", ""},
		boolFlag{"rewrite-legacy", false})
	f := filepath.Join(os.TempDir(), uuid.NewString())
	_, err := os.Stat(f)
	require.Error(t, err)
	defer func() {
		_ = os.Remove(f)
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv := newServer(ctx, t)
	go func() {
		require.NoError(t, srv.Run(ctx))
	}()
	conn, err := srv.GRPCDialContext(ctx)
	require.NoError(t, err)

	originalClient := client.NewClient
	defer func() {
		client.NewClient = originalClient
	}()

	client.NewClient = clientFromConn(conn)

	c, err := clientFromConn(conn)(cmd)
	require.NoError(t, err)

	_, err = c.WriteSchema(ctx, &v1.WriteSchemaRequest{Schema: testSchema})
	require.NoError(t, err)

	testRel := "test/resource:1#reader@test/user:1"
	resp, err := c.WriteRelationships(ctx, &v1.WriteRelationshipsRequest{
		Updates: []*v1.RelationshipUpdate{
			{
				Operation:    v1.RelationshipUpdate_OPERATION_TOUCH,
				Relationship: tuple.ParseRel(testRel),
			},
		},
	})
	require.NoError(t, err)

	err = backupCreateCmdFunc(cmd, []string{f})
	require.NoError(t, err)

	d, closer, err := decoderFromArgs(cmd, []string{f})
	require.NoError(t, err)
	defer func() {
		_ = d.Close()
		_ = closer.Close()
	}()

	require.Equal(t, testSchema, d.Schema())
	rel, err := d.Next()
	require.NoError(t, err)
	require.Equal(t, testRel, tuple.MustStringRelationship(rel))
	require.Equal(t, resp.WrittenAt.Token, d.ZedToken().Token)
}

func TestBackupRestoreCmdFunc(t *testing.T) {
	cmd := createTestCobraCommandWithFlagValue(t,
		stringFlag{"prefix-filter", "test"},
		boolFlag{"rewrite-legacy", false},
		boolFlag{"skip-conflicts", false},
		boolFlag{"touch-conflicts", false},
		boolFlag{"disable-retries", false},
		intFlag{"batch-size", 100},
		int64Flag{"batches-per-transaction", 10},
	)
	backupName := createTestBackup(t, testSchema, testRelationships)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv := newServer(ctx, t)
	go func() {
		require.NoError(t, srv.Run(ctx))
	}()
	conn, err := srv.GRPCDialContext(ctx)
	require.NoError(t, err)

	originalClient := client.NewClient
	defer func() {
		client.NewClient = originalClient
	}()

	client.NewClient = clientFromConn(conn)

	c, err := clientFromConn(conn)(cmd)
	require.NoError(t, err)
	err = backupRestoreCmdFunc(cmd, []string{backupName})
	require.NoError(t, err)

	resp, err := c.ReadSchema(ctx, &v1.ReadSchemaRequest{})
	require.NoError(t, err)
	require.Equal(t, testSchema, resp.SchemaText)

	rrCli, err := c.ReadRelationships(ctx, &v1.ReadRelationshipsRequest{
		Consistency: &v1.Consistency{
			Requirement: &v1.Consistency_FullyConsistent{
				FullyConsistent: true,
			},
		},
		RelationshipFilter: &v1.RelationshipFilter{
			ResourceType: "test/resource",
		},
	})
	require.NoError(t, err)

	rrResp, err := rrCli.Recv()
	require.NoError(t, err)

	require.NoError(t, rrCli.CloseSend())
	require.Equal(t, "test/resource:1#reader@test/user:1", tuple.MustStringRelationship(rrResp.Relationship))
}
