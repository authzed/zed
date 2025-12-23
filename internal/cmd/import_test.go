package cmd

import (
	"net/url"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/structpb"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"github.com/authzed/spicedb/pkg/tuple"

	zedtesting "github.com/authzed/zed/internal/testing"
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
			cmd := zedtesting.CreateTestCobraCommandWithFlagValue(t,
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

			c, err := zedtesting.ClientFromConn(conn)(cmd)
			require.NoError(err)

			u, err := url.Parse(f)
			require.NoError(err)

			// Run the import and assert we don't have errors
			err = importCmdFunc(cmd, c, c, test.prefix, u)
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
