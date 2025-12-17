package mcp

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
)

func createTestServer(t *testing.T) *spiceDBMCPServer {
	ctx := context.Background()

	// Create a real SpiceDB server in memory
	server, err := newSpiceDBServer(ctx)
	require.NoError(t, err)

	// Start the server in the background
	go func() {
		_ = server.Run(ctx)
	}()

	// Get gRPC connection
	conn, err := server.GRPCDialContext(ctx)
	require.NoError(t, err)
	require.NotNil(t, conn)

	return &spiceDBMCPServer{
		conn:          conn,
		schemaService: v1.NewSchemaServiceClient(conn),
		permService:   v1.NewPermissionsServiceClient(conn),
	}
}

func TestReadSchema(t *testing.T) {
	ctx := context.Background()
	server := createTestServer(t)
	defer server.conn.Close()

	t.Run("empty_schema", func(t *testing.T) {
		schema, err := server.readSchema(ctx)

		// SpiceDB returns an error when no schema is defined, which is expected
		require.Error(t, err)
		assert.Contains(t, err.Error(), "No schema has been defined")
		assert.Empty(t, schema)
	})

	t.Run("with_schema", func(t *testing.T) {
		// First write a schema
		expectedSchema := `definition document {
	relation viewer: user
}

definition user {}`

		_, err := server.schemaService.WriteSchema(ctx, &v1.WriteSchemaRequest{
			Schema: expectedSchema,
		})
		require.NoError(t, err)

		// Now read it back
		schema, err := server.readSchema(ctx)

		require.NoError(t, err)
		assert.Equal(t, expectedSchema, schema)
	})
}

func TestReadRelationships(t *testing.T) {
	ctx := context.Background()
	server := createTestServer(t)
	defer server.conn.Close()

	t.Run("empty_relationships", func(t *testing.T) {
		// Write a schema first (needed for ReflectSchema)
		schemaText := `definition document {
	relation viewer: user
}

definition user {}`

		_, err := server.schemaService.WriteSchema(ctx, &v1.WriteSchemaRequest{
			Schema: schemaText,
		})
		require.NoError(t, err)

		relationships, err := server.readRelationships(ctx)

		require.NoError(t, err)
		assert.Empty(t, relationships)
	})

	t.Run("with_relationships", func(t *testing.T) {
		// Write a schema
		schemaText := `definition document {
	relation viewer: user
}

definition user {
	relation admin: user
}`

		_, err := server.schemaService.WriteSchema(ctx, &v1.WriteSchemaRequest{
			Schema: schemaText,
		})
		require.NoError(t, err)

		// Write some relationships
		_, err = server.permService.WriteRelationships(ctx, &v1.WriteRelationshipsRequest{
			Updates: []*v1.RelationshipUpdate{
				{
					Operation: v1.RelationshipUpdate_OPERATION_CREATE,
					Relationship: &v1.Relationship{
						Resource: &v1.ObjectReference{ObjectType: "document", ObjectId: "doc1"},
						Relation: "viewer",
						Subject: &v1.SubjectReference{
							Object: &v1.ObjectReference{ObjectType: "user", ObjectId: "alice"},
						},
					},
				},
				{
					Operation: v1.RelationshipUpdate_OPERATION_CREATE,
					Relationship: &v1.Relationship{
						Resource: &v1.ObjectReference{ObjectType: "user", ObjectId: "charlie"},
						Relation: "admin",
						Subject: &v1.SubjectReference{
							Object: &v1.ObjectReference{ObjectType: "user", ObjectId: "bob"},
						},
					},
				},
			},
		})
		require.NoError(t, err)

		relationships, err := server.readRelationships(ctx)

		require.NoError(t, err)
		assert.Len(t, relationships, 2)

		// Check for expected relationships (order may vary)
		relationshipSet := make(map[string]bool)
		for _, rel := range relationships {
			relationshipSet[rel] = true
		}

		assert.True(t, relationshipSet["document:doc1#viewer@user:alice"])
		assert.True(t, relationshipSet["user:charlie#admin@user:bob"])
	})
}

func TestGetValidationFileHandler(t *testing.T) {
	ctx := context.Background()
	server := createTestServer(t)
	defer server.conn.Close()

	t.Run("with_schema_and_relationships", func(t *testing.T) {
		// Set up schema
		expectedSchema := `definition document {
	relation viewer: user
}

definition user {}`

		_, err := server.schemaService.WriteSchema(ctx, &v1.WriteSchemaRequest{
			Schema: expectedSchema,
		})
		require.NoError(t, err)

		// Set up relationships
		_, err = server.permService.WriteRelationships(ctx, &v1.WriteRelationshipsRequest{
			Updates: []*v1.RelationshipUpdate{
				{
					Operation: v1.RelationshipUpdate_OPERATION_CREATE,
					Relationship: &v1.Relationship{
						Resource: &v1.ObjectReference{ObjectType: "document", ObjectId: "doc1"},
						Relation: "viewer",
						Subject: &v1.SubjectReference{
							Object: &v1.ObjectReference{ObjectType: "user", ObjectId: "alice"},
						},
					},
				},
			},
		})
		require.NoError(t, err)

		request := mcp.ReadResourceRequest{
			Params: mcp.ReadResourceParams{URI: "validation://current"},
		}

		contents, err := server.getValidationFileHandler(ctx, request)

		require.NoError(t, err)
		assert.Len(t, contents, 1)

		content := contents[0].(mcp.TextResourceContents)
		assert.Equal(t, "validation://current", content.URI)
		assert.Equal(t, "application/yaml", content.MIMEType)

		// Parse and verify YAML content
		var validationFile ValidationFile
		err = yaml.Unmarshal([]byte(content.Text), &validationFile)
		require.NoError(t, err)
		assert.Equal(t, expectedSchema, validationFile.Schema)
		assert.Len(t, validationFile.Relationships, 1)
		assert.Equal(t, "document:doc1#viewer@user:alice", validationFile.Relationships[0])
	})
}

func TestGetCurrentSchemaHandler(t *testing.T) {
	ctx := context.Background()
	server := createTestServer(t)
	defer server.conn.Close()

	t.Run("with_schema", func(t *testing.T) {
		expectedSchema := `definition document {
	relation viewer: user
}

definition user {}`

		_, err := server.schemaService.WriteSchema(ctx, &v1.WriteSchemaRequest{
			Schema: expectedSchema,
		})
		require.NoError(t, err)

		request := mcp.ReadResourceRequest{
			Params: mcp.ReadResourceParams{URI: "schema://current"},
		}

		contents, err := server.getCurrentSchemaHandler(ctx, request)

		require.NoError(t, err)
		assert.Len(t, contents, 1)

		content := contents[0].(mcp.TextResourceContents)
		assert.Equal(t, "schema://current", content.URI)
		assert.Equal(t, "text/spicedb-schema", content.MIMEType)
		assert.Equal(t, expectedSchema, content.Text)
	})
}

func TestGetAllRelationshipsHandler(t *testing.T) {
	ctx := context.Background()
	server := createTestServer(t)
	defer server.conn.Close()

	t.Run("with_relationships", func(t *testing.T) {
		// Set up schema
		schemaText := `definition document {
	relation viewer: user
}

definition user {}`

		_, err := server.schemaService.WriteSchema(ctx, &v1.WriteSchemaRequest{
			Schema: schemaText,
		})
		require.NoError(t, err)

		// Set up relationships
		_, err = server.permService.WriteRelationships(ctx, &v1.WriteRelationshipsRequest{
			Updates: []*v1.RelationshipUpdate{
				{
					Operation: v1.RelationshipUpdate_OPERATION_CREATE,
					Relationship: &v1.Relationship{
						Resource: &v1.ObjectReference{ObjectType: "document", ObjectId: "doc1"},
						Relation: "viewer",
						Subject: &v1.SubjectReference{
							Object: &v1.ObjectReference{ObjectType: "user", ObjectId: "alice"},
						},
					},
				},
			},
		})
		require.NoError(t, err)

		request := mcp.ReadResourceRequest{
			Params: mcp.ReadResourceParams{URI: "relationships://all"},
		}

		contents, err := server.getAllRelationshipsHandler(ctx, request)

		require.NoError(t, err)
		assert.Len(t, contents, 1)

		content := contents[0].(mcp.TextResourceContents)
		assert.Equal(t, "relationships://all", content.URI)
		assert.Equal(t, "application/json", content.MIMEType)
		assert.Contains(t, content.Text, "document:doc1#viewer@user:alice")
		assert.Contains(t, content.Text, `"count": 1`)
	})
}

func TestWriteSchemaHandler(t *testing.T) {
	ctx := context.Background()
	server := createTestServer(t)
	defer server.conn.Close()

	t.Run("success", func(t *testing.T) {
		schemaText := `definition document {
	relation viewer: user
}

definition user {}`

		// Create mock request
		request := mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Arguments: map[string]any{
					"schema": schemaText,
				},
			},
		}

		result, err := server.writeSchemaHandler(ctx, request)

		require.NoError(t, err)
		assert.NotNil(t, result)

		// Verify the schema was actually written by reading it back
		schema, err := server.readSchema(ctx)
		require.NoError(t, err)
		assert.Equal(t, schemaText, schema)
	})
}

func TestBuildRelationship(t *testing.T) {
	server := createTestServer(t)
	defer server.conn.Close()

	t.Run("basic_relationship", func(t *testing.T) {
		rel := RelationshipDef{
			ResourceType: "document",
			ResourceID:   "doc1",
			Relation:     "viewer",
			SubjectType:  "user",
			SubjectID:    "alice",
		}

		relationship, err := server.buildRelationship(rel)

		require.NoError(t, err)
		assert.Equal(t, "document", relationship.Resource.ObjectType)
		assert.Equal(t, "doc1", relationship.Resource.ObjectId)
		assert.Equal(t, "viewer", relationship.Relation)
		assert.Equal(t, "user", relationship.Subject.Object.ObjectType)
		assert.Equal(t, "alice", relationship.Subject.Object.ObjectId)
		assert.Empty(t, relationship.Subject.OptionalRelation)
	})

	t.Run("relationship_with_subject_relation", func(t *testing.T) {
		rel := RelationshipDef{
			ResourceType:    "document",
			ResourceID:      "doc1",
			Relation:        "viewer",
			SubjectType:     "group",
			SubjectID:       "admins",
			SubjectRelation: "member",
		}

		relationship, err := server.buildRelationship(rel)

		require.NoError(t, err)
		assert.Equal(t, "member", relationship.Subject.OptionalRelation)
	})

	t.Run("relationship_with_caveat", func(t *testing.T) {
		rel := RelationshipDef{
			ResourceType:  "document",
			ResourceID:    "doc1",
			Relation:      "viewer",
			SubjectType:   "user",
			SubjectID:     "alice",
			CaveatName:    "ip_range",
			CaveatContext: map[string]any{"max_attempts": 3, "enabled": true},
		}

		relationship, err := server.buildRelationship(rel)

		require.NoError(t, err)
		assert.NotNil(t, relationship.OptionalCaveat)
		assert.Equal(t, "ip_range", relationship.OptionalCaveat.CaveatName)
		// Context should be set (not nil) when caveat context is provided
		assert.NotNil(t, relationship.OptionalCaveat.Context)
	})
}

func TestValidationFileStruct(t *testing.T) {
	t.Run("yaml_marshaling", func(t *testing.T) {
		vf := ValidationFile{
			Schema: "definition document {}",
			Relationships: []string{
				"document:doc1#viewer@user:alice",
				"document:doc2#viewer@user:bob",
			},
		}

		yamlData, err := yaml.Marshal(vf)
		require.NoError(t, err)

		var parsed ValidationFile
		err = yaml.Unmarshal(yamlData, &parsed)
		require.NoError(t, err)

		assert.Equal(t, vf.Schema, parsed.Schema)
		assert.Equal(t, vf.Relationships, parsed.Relationships)
	})

	t.Run("empty_validation_file", func(t *testing.T) {
		vf := ValidationFile{}

		yamlData, err := yaml.Marshal(vf)
		require.NoError(t, err)

		var parsed ValidationFile
		err = yaml.Unmarshal(yamlData, &parsed)
		require.NoError(t, err)

		assert.Empty(t, parsed.Schema)
		assert.Empty(t, parsed.Relationships)
	})
}
