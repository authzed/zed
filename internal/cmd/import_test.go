package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/structpb"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"github.com/authzed/spicedb/pkg/tuple"

	"github.com/authzed/zed/internal/zedtesting"
)

var fullyConsistent = &v1.Consistency{Requirement: &v1.Consistency_FullyConsistent{FullyConsistent: true}}

func TestImportCmd(t *testing.T) {
	testcases := map[string]struct {
		importSchema bool
		importRels   bool
		prefix       string
		check        string
	}{
		`without_prefix_import_schema_and_rels`: {
			prefix:       "",
			importSchema: true,
			importRels:   true,
			check:        `resource:1#view@user:1[mycaveat]`,
		},
		`with_prefix_import_schema_and_rels`: {
			prefix:       "maria",
			importSchema: true,
			importRels:   true,
			check:        `maria/resource:1#view@maria/user:1[maria/mycaveat]`,
		},
		`with_prefix_with_slash_import_schema_and_rels`: {
			prefix:       "maria/",
			importSchema: true,
			importRels:   true,
			check:        `maria/resource:1#view@maria/user:1[maria/mycaveat]`,
		},
		`with_prefix_import_only_schema`: {
			prefix:       "maria",
			importSchema: true,
			importRels:   false,
			check:        "",
		},
	}

	for testName, test := range testcases {
		t.Run(testName, func(t *testing.T) {
			require := require.New(t)
			cmd := zedtesting.CreateTestCobraCommandWithFlagValue(
				t,
				zedtesting.StringFlag{FlagName: "schema-definition-prefix", FlagValue: test.prefix},
				zedtesting.BoolFlag{FlagName: "schema", FlagValue: test.importSchema},
				zedtesting.BoolFlag{FlagName: "relationships", FlagValue: test.importRels},
				zedtesting.IntFlag{FlagName: "batch-size", FlagValue: 100},
				zedtesting.IntFlag{FlagName: "workers", FlagValue: 1},
			)
			f := filepath.Join("import-test", "happy-path-validation-file.yaml")

			// Set up client
			ctx := t.Context()
			srv := zedtesting.NewTestServer(ctx, t)
			go func() {
				assert.NoError(t, srv.Run(ctx))
			}()
			conn, err := srv.GRPCDialContext(ctx)
			require.NoError(err)
			t.Cleanup(func() {
				conn.Close()
			})

			c, err := zedtesting.ClientFromConn(conn)(cmd)
			require.NoError(err)

			// Run the import and assert we don't have errors
			err = importCmdFunc(cmd, c, c, test.prefix, f)
			require.NoError(err)

			// Run a check with full consistency to assert that the relationships
			// and schema were written
			if test.check != "" {
				rel := tuple.MustParse(test.check)
				resp, err := c.CheckPermission(ctx, &v1.CheckPermissionRequest{
					Consistency: fullyConsistent,
					Subject:     &v1.SubjectReference{Object: &v1.ObjectReference{ObjectType: rel.Subject.ObjectType, ObjectId: rel.Subject.ObjectID}},
					Permission:  "view",
					Resource:    &v1.ObjectReference{ObjectType: rel.Resource.ObjectType, ObjectId: rel.Resource.ObjectID},
					Context: &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"day_of_week": structpb.NewStringValue("friday"),
						},
					},
				})
				require.NoError(err)
				require.Equal(v1.CheckPermissionResponse_PERMISSIONSHIP_HAS_PERMISSION, resp.Permissionship)
			}
		})
	}
}

func TestImportCmdSchemaWithImports(t *testing.T) {
	require := require.New(t)
	cmd := zedtesting.CreateTestCobraCommandWithFlagValue(t,
		zedtesting.StringFlag{FlagName: "schema-definition-prefix"},
		zedtesting.BoolFlag{FlagName: "schema", FlagValue: true},
		zedtesting.BoolFlag{FlagName: "relationships", FlagValue: true},
		zedtesting.IntFlag{FlagName: "batch-size", FlagValue: 100},
		zedtesting.IntFlag{FlagName: "workers", FlagValue: 1},
	)
	f := filepath.Join("import-test", "with-import-validation-file.yaml")

	ctx := t.Context()
	srv := zedtesting.NewTestServer(ctx, t)
	go func() {
		assert.NoError(t, srv.Run(ctx))
	}()
	conn, err := srv.GRPCDialContext(ctx)
	require.NoError(err)
	t.Cleanup(func() {
		conn.Close()
	})

	c, err := zedtesting.ClientFromConn(conn)(cmd)
	require.NoError(err)

	// The YAML points to a .zed file that uses `import "with-import-common.zed"`. WriteSchema
	// rejects `import` statements, so this exercises that the client flattens the schema
	// (via rewriteSchema + SourceFolder) before sending.
	err = importCmdFunc(cmd, c, c, "", f)
	require.NoError(err)

	rel := tuple.MustParse(`resource:1#view@user:1[mycaveat]`)
	resp, err := c.CheckPermission(ctx, &v1.CheckPermissionRequest{
		Consistency: fullyConsistent,
		Subject:     &v1.SubjectReference{Object: &v1.ObjectReference{ObjectType: rel.Subject.ObjectType, ObjectId: rel.Subject.ObjectID}},
		Permission:  "view",
		Resource:    &v1.ObjectReference{ObjectType: rel.Resource.ObjectType, ObjectId: rel.Resource.ObjectID},
		Context: &structpb.Struct{
			Fields: map[string]*structpb.Value{
				"day_of_week": structpb.NewStringValue("friday"),
			},
		},
	})
	require.NoError(err)
	require.Equal(v1.CheckPermissionResponse_PERMISSIONSHIP_HAS_PERMISSION, resp.Permissionship)
}

func TestImportCmdRelationsOnly(t *testing.T) {
	cmd := zedtesting.CreateTestCobraCommandWithFlagValue(
		t,
		zedtesting.StringFlag{FlagName: "schema-definition-prefix"},
		zedtesting.BoolFlag{FlagName: "schema", FlagValue: false},
		zedtesting.BoolFlag{FlagName: "relationships", FlagValue: true},
		zedtesting.IntFlag{FlagName: "batch-size", FlagValue: 100},
		zedtesting.IntFlag{FlagName: "workers", FlagValue: 1},
	)

	// Set up client
	ctx := t.Context()
	srv := zedtesting.NewTestServer(ctx, t)
	go func() {
		assert.NoError(t, srv.Run(ctx))
	}()
	conn, err := srv.GRPCDialContext(ctx)
	require.NoError(t, err)
	t.Cleanup(func() {
		conn.Close()
	})

	c, err := zedtesting.ClientFromConn(conn)(cmd)
	require.NoError(t, err)

	// Write the schema out-of-band so that the import is hitting a realized schema
	schemaBytes, err := os.ReadFile(filepath.Join("import-test", "relations-only-schema.zed"))
	require.NoError(t, err)
	_, err = c.WriteSchema(ctx, &v1.WriteSchemaRequest{
		Schema: string(schemaBytes),
	})
	require.NoError(t, err)

	t.Run("with no schema or schemaFile key in yaml", func(t *testing.T) {
		f := filepath.Join("import-test", "relations-only-validation-file.yaml")
		err = importCmdFunc(cmd, c, c, "", f)
		require.NoError(t, err)

		// Run a check with full consistency to see whether the relationships were written
		resp, err := c.CheckPermission(ctx, &v1.CheckPermissionRequest{
			Consistency: fullyConsistent,
			Subject:     &v1.SubjectReference{Object: &v1.ObjectReference{ObjectType: "user", ObjectId: "1"}},
			Permission:  "view",
			Resource:    &v1.ObjectReference{ObjectType: "resource", ObjectId: "1"},
		})
		require.NoError(t, err)
		require.Equal(t, v1.CheckPermissionResponse_PERMISSIONSHIP_HAS_PERMISSION, resp.Permissionship)

		// Run a ReadSchema to assert the schema was NOT written
		schemaResp, err := c.ReadSchema(ctx, &v1.ReadSchemaRequest{})
		require.NoError(t, err)
		require.Contains(t, schemaResp.SchemaText, `relation user: user`)
		require.Contains(t, schemaResp.SchemaText, `permission view = user`)
	})
	t.Run("with schema present should be ignored", func(t *testing.T) {
		f := filepath.Join("import-test", "relations-only-validation-file-different-schema.yaml")
		err = importCmdFunc(cmd, c, c, "", f)
		require.NoError(t, err)

		// Run a check with full consistency to see whether the relationships
		// and schema are written
		resp, err := c.CheckPermission(ctx, &v1.CheckPermissionRequest{
			Consistency: fullyConsistent,
			Subject:     &v1.SubjectReference{Object: &v1.ObjectReference{ObjectType: "user", ObjectId: "1"}},
			Permission:  "view",
			Resource:    &v1.ObjectReference{ObjectType: "resource", ObjectId: "1"},
		})
		require.NoError(t, err)
		require.Equal(t, v1.CheckPermissionResponse_PERMISSIONSHIP_HAS_PERMISSION, resp.Permissionship)

		// Run a ReadSchema to assert the schema was NOT written
		schemaResp, err := c.ReadSchema(ctx, &v1.ReadSchemaRequest{})
		require.NoError(t, err)
		require.Contains(t, schemaResp.SchemaText, `relation user: user`)
		require.Contains(t, schemaResp.SchemaText, `permission view = user`)
	})
}
