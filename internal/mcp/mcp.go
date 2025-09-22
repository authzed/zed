package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gopkg.in/yaml.v3"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"github.com/authzed/spicedb/pkg/cmd/datastore"
	"github.com/authzed/spicedb/pkg/cmd/server"
	"github.com/authzed/spicedb/pkg/cmd/util"

	"github.com/authzed/zed/internal/console"
)

type spiceDBMCPServer struct {
	conn          *grpc.ClientConn
	schemaService v1.SchemaServiceClient
	permService   v1.PermissionsServiceClient
}

type RelationshipDef struct {
	ResourceType    string                 `json:"resource_type"`
	ResourceID      string                 `json:"resource_id"`
	Relation        string                 `json:"relation"`
	SubjectType     string                 `json:"subject_type"`
	SubjectID       string                 `json:"subject_id"`
	SubjectRelation string                 `json:"optional_subject_relation,omitempty"`
	CaveatName      string                 `json:"caveat_name,omitempty"`
	CaveatContext   map[string]interface{} `json:"caveat_context,omitempty"`
	Expiration      *time.Time             `json:"expiration,omitempty"`
}

type ValidationFile struct {
	Schema        string   `yaml:"schema"`
	Relationships []string `yaml:"relationships"`
}

type UpdateRelationshipsArgs struct {
	Create []RelationshipDef `json:"create,omitempty"`
	Touch  []RelationshipDef `json:"touch,omitempty"`
	Delete []RelationshipDef `json:"delete,omitempty"`
}

type DeleteRelationshipsArgs struct {
	ResourceType    string `json:"resource_type,omitempty"`
	ResourceID      string `json:"resource_id,omitempty"`
	Relation        string `json:"relation,omitempty"`
	SubjectType     string `json:"subject_type,omitempty"`
	SubjectID       string `json:"subject_id,omitempty"`
	SubjectRelation string `json:"optional_subject_relation,omitempty"`
}

// SpiceDBMCPServer is the publicly exported type for the SpiceDB MCP server.
type SpiceDBMCPServer = spiceDBMCPServer

// NewSpiceDBMCPServer creates a new instance of the SpiceDB MCP server.
func NewSpiceDBMCPServer() *SpiceDBMCPServer {
	return &spiceDBMCPServer{}
}

// Run starts the MCP server via HTTP streaming on the specified port.
func (smcp *spiceDBMCPServer) Run(portNumber int) error {
	console.Printf("Starting SpiceDB MCP server on port %d...\n", portNumber)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	console.Printf("Configuring SpiceDB server...\n")
	server, err := newSpiceDBServer(ctx)
	if err != nil {
		return fmt.Errorf("unable to configure server: %w", err)
	}
	console.Printf("SpiceDB server configured successfully\n")

	console.Printf("Starting SpiceDB gRPC server...\n")
	var wg errgroup.Group
	wg.Go(func() error {
		if err := server.Run(ctx); err != nil {
			console.Errorf("error while running server: %v\n", err)
			return err
		}
		return nil
	})

	console.Printf("Establishing gRPC connection...\n")
	conn, err := server.GRPCDialContext(ctx)
	if err != nil || conn == nil {
		return fmt.Errorf("unable to get gRPC connection: %w", err)
	}
	console.Printf("gRPC connection established\n")

	console.Printf("Building MCP instructions...\n")
	instructions, err := buildInstructions()
	if err != nil {
		return fmt.Errorf("failed to build instructions: %w", err)
	}

	console.Printf("Creating MCP server with tools and resources...\n")
	// Create a new MCP server with both tools and resources enabled
	s := mcpserver.NewMCPServer(
		"SpiceDB Agent",
		"0.0.1",
		mcpserver.WithToolCapabilities(true),
		mcpserver.WithResourceCapabilities(true, true),
		mcpserver.WithInstructions(instructions),
	)
	httpServer := mcpserver.NewStreamableHTTPServer(s)

	smcp.schemaService = v1.NewSchemaServiceClient(conn)
	smcp.permService = v1.NewPermissionsServiceClient(conn)
	smcp.conn = conn

	console.Printf("Setting up MCP tools and resources...\n")

	// Add schema write tool
	writeSchemaTools := mcp.NewTool("write_schema",
		mcp.WithDescription("Write a SpiceDB schema definition"),
		mcp.WithString("schema",
			mcp.Required(),
			mcp.Description("The schema definition text"),
		),
	)
	s.AddTool(writeSchemaTools, smcp.writeSchemaHandler)

	// Add update relationships tool
	updateRelationshipsTool := mcp.NewTool("update_relationships",
		mcp.WithDescription("Update relationships in SpiceDB with create, touch, and delete operations"),
		mcp.WithArray("create",
			mcp.Description("Array of relationships to create"),
			mcp.Items(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"resource_type":             map[string]any{"type": "string", "description": "Resource type"},
					"resource_id":               map[string]any{"type": "string", "description": "Resource ID"},
					"relation":                  map[string]any{"type": "string", "description": "Relation name"},
					"subject_type":              map[string]any{"type": "string", "description": "Subject type"},
					"subject_id":                map[string]any{"type": "string", "description": "Subject ID"},
					"optional_subject_relation": map[string]any{"type": "string", "description": "Optional subject relation"},
					"caveat_name":               map[string]any{"type": "string", "description": "Optional caveat name"},
					"caveat_context":            map[string]any{"type": "object", "description": "Optional caveat context"},
					"expiration":                map[string]any{"type": "string", "format": "date-time", "description": "Optional expiration time"},
				},
				"required": []string{"resource_type", "resource_id", "relation", "subject_type", "subject_id"},
			}),
		),
		mcp.WithArray("touch",
			mcp.Description("Array of relationships to touch (update timestamp)"),
			mcp.Items(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"resource_type":             map[string]any{"type": "string", "description": "Resource type"},
					"resource_id":               map[string]any{"type": "string", "description": "Resource ID"},
					"relation":                  map[string]any{"type": "string", "description": "Relation name"},
					"subject_type":              map[string]any{"type": "string", "description": "Subject type"},
					"subject_id":                map[string]any{"type": "string", "description": "Subject ID"},
					"optional_subject_relation": map[string]any{"type": "string", "description": "Optional subject relation"},
					"caveat_name":               map[string]any{"type": "string", "description": "Optional caveat name"},
					"caveat_context":            map[string]any{"type": "object", "description": "Optional caveat context"},
					"expiration":                map[string]any{"type": "string", "format": "date-time", "description": "Optional expiration time"},
				},
				"required": []string{"resource_type", "resource_id", "relation", "subject_type", "subject_id"},
			}),
		),
		mcp.WithArray("delete",
			mcp.Description("Array of relationships to delete"),
			mcp.Items(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"resource_type":             map[string]any{"type": "string", "description": "Resource type"},
					"resource_id":               map[string]any{"type": "string", "description": "Resource ID"},
					"relation":                  map[string]any{"type": "string", "description": "Relation name"},
					"subject_type":              map[string]any{"type": "string", "description": "Subject type"},
					"subject_id":                map[string]any{"type": "string", "description": "Subject ID"},
					"optional_subject_relation": map[string]any{"type": "string", "description": "Optional subject relation"},
					"caveat_name":               map[string]any{"type": "string", "description": "Optional caveat name"},
					"caveat_context":            map[string]any{"type": "object", "description": "Optional caveat context"},
					"expiration":                map[string]any{"type": "string", "format": "date-time", "description": "Optional expiration time"},
				},
				"required": []string{"resource_type", "resource_id", "relation", "subject_type", "subject_id"},
			}),
		),
	)
	s.AddTool(updateRelationshipsTool, smcp.updateRelationshipsHandlerWrapper)

	// Add delete relationships tool
	deleteRelationshipsTool := mcp.NewTool("delete_relationships",
		mcp.WithDescription("Delete relationships matching a filter from SpiceDB"),
		mcp.WithString("resource_type",
			mcp.Description("Filter by resource type"),
		),
		mcp.WithString("resource_id",
			mcp.Description("Filter by resource ID"),
		),
		mcp.WithString("relation",
			mcp.Description("Filter by relation name"),
		),
		mcp.WithString("subject_type",
			mcp.Description("Filter by subject type"),
		),
		mcp.WithString("subject_id",
			mcp.Description("Filter by subject ID"),
		),
		mcp.WithString("optional_subject_relation",
			mcp.Description("Filter by optional subject relation"),
		),
	)
	s.AddTool(deleteRelationshipsTool, smcp.deleteRelationshipsHandlerWrapper)

	// Add check permission tool
	checkPermissionTool := mcp.NewTool("check_permission",
		mcp.WithDescription("Check permission with full consistency and debug tracing"),
		mcp.WithString("resource_type",
			mcp.Required(),
			mcp.Description("Resource type (e.g., 'document')"),
		),
		mcp.WithString("resource_id",
			mcp.Required(),
			mcp.Description("Resource ID (e.g., 'document1')"),
		),
		mcp.WithString("permission_or_relation",
			mcp.Required(),
			mcp.Description("Permission or relation to check (e.g., 'view' or 'reader')"),
		),
		mcp.WithString("subject_type",
			mcp.Required(),
			mcp.Description("Subject type (e.g., 'user')"),
		),
		mcp.WithString("subject_id",
			mcp.Required(),
			mcp.Description("Subject ID (e.g., 'user1')"),
		),
		mcp.WithString("optional_subject_relation",
			mcp.Description("Optional subject relation"),
		),
		mcp.WithString("optional_caveat_context_json",
			mcp.Description("Optional caveat context, as a JSON string"),
		),
	)
	s.AddTool(checkPermissionTool, smcp.checkPermissionHandler)

	// Add lookup resources tool
	lookupResourcesTool := mcp.NewTool("lookup_resources",
		mcp.WithDescription("Lookup resources that a subject has permission to access"),
		mcp.WithString("resource_object_type",
			mcp.Required(),
			mcp.Description("Type of resources to lookup (e.g., 'document')"),
		),
		mcp.WithString("permission",
			mcp.Required(),
			mcp.Description("Permission to check (e.g., 'view')"),
		),
		mcp.WithString("subject_type",
			mcp.Required(),
			mcp.Description("Subject type (e.g., 'user')"),
		),
		mcp.WithString("subject_id",
			mcp.Required(),
			mcp.Description("Subject ID (e.g., 'user1')"),
		),
		mcp.WithString("optional_subject_relation",
			mcp.Description("Optional subject relation"),
		),
		mcp.WithString("optional_caveat_context_json",
			mcp.Description("Optional caveat context, as a JSON string"),
		),
	)
	s.AddTool(lookupResourcesTool, smcp.lookupResourcesHandler)

	// Add lookup subjects tool
	lookupSubjectsTool := mcp.NewTool("lookup_subjects",
		mcp.WithDescription("Lookup subjects that have permission to access a resource"),
		mcp.WithString("subject_object_type",
			mcp.Required(),
			mcp.Description("Type of subjects to lookup (e.g., 'user')"),
		),
		mcp.WithString("permission",
			mcp.Required(),
			mcp.Description("Permission to check (e.g., 'view')"),
		),
		mcp.WithString("resource_type",
			mcp.Required(),
			mcp.Description("Resource type (e.g., 'document')"),
		),
		mcp.WithString("resource_id",
			mcp.Required(),
			mcp.Description("Resource ID (e.g., 'document1')"),
		),
		mcp.WithString("optional_resource_relation",
			mcp.Description("Optional resource relation"),
		),
		mcp.WithString("optional_caveat_context_json",
			mcp.Description("Optional caveat context, as a JSON string"),
		),
	)
	s.AddTool(lookupSubjectsTool, smcp.lookupSubjectsHandler)

	// Add resources
	schemaResource := mcp.NewResource(
		"schema://current",
		"Current SpiceDB Schema",
		mcp.WithResourceDescription("The current schema definition in SpiceDB"),
		mcp.WithMIMEType("text/spicedb-schema"),
	)
	s.AddResource(schemaResource, smcp.getCurrentSchemaHandler)

	relationshipsResource := mcp.NewResource(
		"relationships://all",
		"All Relationships",
		mcp.WithResourceDescription("All relationship tuples stored in SpiceDB"),
		mcp.WithMIMEType("application/json"),
	)
	s.AddResource(relationshipsResource, smcp.getAllRelationshipsHandler)

	instructionsResource := mcp.NewResource(
		"instructions://",
		"Instructions for how to use this agent",
		mcp.WithResourceDescription("Instructions for using the SpiceDB agent, including schema design guidelines, relationship formatting, and permission checking rules"),
		mcp.WithMIMEType("text/plain"),
	)
	s.AddResource(instructionsResource, smcp.getInstructionsHandler)

	validationResource := mcp.NewResource(
		"validation://current",
		"Current Validation File",
		mcp.WithResourceDescription("Current validation file containing schema and relationships in SpiceDB validation file format"),
		mcp.WithMIMEType("application/yaml"),
	)
	s.AddResource(validationResource, smcp.getValidationFileHandler)

	console.Printf("Starting MCP HTTP streaming server on port %d...\n", portNumber)

	// Start the HTTP server in a goroutine
	wg.Go(func() error {
		err := httpServer.Start(":" + fmt.Sprint(portNumber))
		if err != nil {
			console.Errorf("Failed to start MCP server: %v\n", err)
			return fmt.Errorf("failed to start MCP server: %w", err)
		}
		return nil
	})

	console.Printf("MCP server started successfully!\n")
	console.Printf("Press Ctrl+C to gracefully shutdown the server...\n")

	// Wait for signal or error
	select {
	case sig := <-sigChan:
		console.Printf("Received signal %v, initiating graceful shutdown...\n", sig)
		cancel() // Cancel the context to signal all goroutines to shut down

		// Close gRPC connection
		if smcp.conn != nil {
			console.Printf("Closing gRPC connection...\n")
			smcp.conn.Close()
		}

		// Wait for all goroutines to finish with timeout
		done := make(chan error)
		go func() {
			done <- wg.Wait()
		}()

		select {
		case err := <-done:
			if err != nil {
				console.Errorf("Error during shutdown: %v\n", err)
				return err
			}
			console.Printf("Graceful shutdown completed\n")
			return nil
		case <-time.After(3 * time.Second):
			console.Printf("Shutdown timeout reached, forcing exit\n")
			return fmt.Errorf("shutdown timeout")
		}

	case <-ctx.Done():
		// Context was cancelled by some other means
		console.Printf("Context cancelled, shutting down...\n")
		if smcp.conn != nil {
			smcp.conn.Close()
		}
		return wg.Wait()
	}
}

func (smcp *spiceDBMCPServer) writeSchemaHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	console.Printf("MCP Tool: write_schema called\n")
	schema, err := request.RequireString("schema")
	if err != nil {
		console.Errorf("write_schema: Failed to get schema parameter: %v\n", err)
		return mcp.NewToolResultError(err.Error()), nil
	}

	console.Printf("write_schema: Writing schema to SpiceDB...\n")
	_, err = smcp.schemaService.WriteSchema(ctx, &v1.WriteSchemaRequest{
		Schema: schema,
	})
	if err != nil {
		console.Errorf("write_schema: Failed to write schema: %v\n", err)
		return mcp.NewToolResultError(fmt.Sprintf("Failed to write schema: %v", err)), nil
	}

	console.Printf("write_schema: Schema written successfully\n")
	return mcp.NewToolResultText("Schema written successfully"), nil
}

func (smcp *spiceDBMCPServer) getCurrentSchemaHandler(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	console.Printf("MCP Resource: schema://current requested\n")
	schema, err := smcp.readSchema(ctx)
	if err != nil {
		console.Errorf("schema://current: Failed to read schema: %v\n", err)
		return []mcp.ResourceContents{
			mcp.TextResourceContents{
				URI:      request.Params.URI,
				MIMEType: "text/spicedb-schema",
				Text:     fmt.Sprintf("Error reading schema: %v", err),
			},
		}, nil
	}

	console.Printf("schema://current: Successfully retrieved schema\n")

	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      request.Params.URI,
			MIMEType: "text/spicedb-schema",
			Text:     schema,
		},
	}, nil
}

func (smcp *spiceDBMCPServer) getInstructionsHandler(_ context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	console.Printf("MCP Resource: instructions:// requested\n")
	instructions, err := buildInstructions()
	if err != nil {
		console.Errorf("instructions://: Failed to build instructions: %v\n", err)
		return []mcp.ResourceContents{
			mcp.TextResourceContents{
				URI:      request.Params.URI,
				MIMEType: "text/plain",
				Text:     fmt.Sprintf("Error building instructions: %v", err),
			},
		}, nil
	}
	console.Printf("instructions://: Successfully built instructions\n")
	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      request.Params.URI,
			MIMEType: "text/plain",
			Text:     strings.TrimSpace(instructions),
		},
	}, nil
}

func (smcp *spiceDBMCPServer) updateRelationshipsHandlerWrapper(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	console.Printf("MCP Tool: update_relationships called\n")
	var args UpdateRelationshipsArgs
	if err := request.BindArguments(&args); err != nil {
		console.Errorf("update_relationships: Failed to parse arguments: %v\n", err)
		return mcp.NewToolResultError(fmt.Sprintf("Failed to parse arguments: %v", err)), nil
	}
	return smcp.updateRelationshipsHandler(ctx, request, args)
}

func (smcp *spiceDBMCPServer) updateRelationshipsHandler(ctx context.Context, _ mcp.CallToolRequest, args UpdateRelationshipsArgs) (*mcp.CallToolResult, error) {
	console.Printf("update_relationships: Processing %d create, %d touch, %d delete operations\n", len(args.Create), len(args.Touch), len(args.Delete))
	updates := make([]*v1.RelationshipUpdate, 0, len(args.Create)+len(args.Touch)+len(args.Delete))

	// Process create operations
	for _, rel := range args.Create {
		relationship, err := smcp.buildRelationship(rel)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to build relationship: %v", err)), nil
		}
		updates = append(updates, &v1.RelationshipUpdate{
			Operation:    v1.RelationshipUpdate_OPERATION_CREATE,
			Relationship: relationship,
		})
	}

	// Process touch operations
	for _, rel := range args.Touch {
		relationship, err := smcp.buildRelationship(rel)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to build relationship: %v", err)), nil
		}
		updates = append(updates, &v1.RelationshipUpdate{
			Operation:    v1.RelationshipUpdate_OPERATION_TOUCH,
			Relationship: relationship,
		})
	}

	// Process delete operations
	for _, rel := range args.Delete {
		relationship, err := smcp.buildRelationship(rel)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to build relationship: %v", err)), nil
		}
		updates = append(updates, &v1.RelationshipUpdate{
			Operation:    v1.RelationshipUpdate_OPERATION_DELETE,
			Relationship: relationship,
		})
	}

	if len(updates) == 0 {
		console.Printf("update_relationships: No operations specified\n")
		return mcp.NewToolResultError("No operations specified"), nil
	}

	console.Printf("update_relationships: Writing %d relationship updates to SpiceDB...\n", len(updates))
	_, err := smcp.permService.WriteRelationships(ctx, &v1.WriteRelationshipsRequest{
		Updates: updates,
	})
	if err != nil {
		console.Errorf("update_relationships: Failed to update relationships: %v\n", err)
		return mcp.NewToolResultError(fmt.Sprintf("Failed to update relationships: %v", err)), nil
	}

	console.Printf("update_relationships: Successfully processed %d relationship operations\n", len(updates))
	return mcp.NewToolResultText(fmt.Sprintf("Successfully processed %d relationship operations", len(updates))), nil
}

func (smcp *spiceDBMCPServer) deleteRelationshipsHandlerWrapper(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	console.Printf("MCP Tool: delete_relationships called\n")
	var args DeleteRelationshipsArgs
	if err := request.BindArguments(&args); err != nil {
		console.Errorf("delete_relationships: Failed to parse arguments: %v\n", err)
		return mcp.NewToolResultError(fmt.Sprintf("Failed to parse arguments: %v", err)), nil
	}
	return smcp.deleteRelationshipsHandler(ctx, request, args)
}

func (smcp *spiceDBMCPServer) deleteRelationshipsHandler(ctx context.Context, _ mcp.CallToolRequest, args DeleteRelationshipsArgs) (*mcp.CallToolResult, error) {
	console.Printf("delete_relationships: Building filter for deletion (resource_type=%s, resource_id=%s, relation=%s, subject_type=%s, subject_id=%s)\n", args.ResourceType, args.ResourceID, args.Relation, args.SubjectType, args.SubjectID)
	// Build the relationship filter
	filter := &v1.RelationshipFilter{}

	if args.ResourceType != "" {
		filter.ResourceType = args.ResourceType
		if args.ResourceID != "" {
			filter.OptionalResourceId = args.ResourceID
		}
	}

	if args.Relation != "" {
		filter.OptionalRelation = args.Relation
	}

	if args.SubjectType != "" {
		subjectFilter := &v1.SubjectFilter{
			SubjectType: args.SubjectType,
		}
		if args.SubjectID != "" {
			subjectFilter.OptionalSubjectId = args.SubjectID
		}
		if args.SubjectRelation != "" {
			subjectFilter.OptionalRelation = &v1.SubjectFilter_RelationFilter{
				Relation: args.SubjectRelation,
			}
		}
		filter.OptionalSubjectFilter = subjectFilter
	}

	console.Printf("delete_relationships: Reading relationships matching filter...\n")
	// First, read the relationships to delete them
	stream, err := smcp.permService.ReadRelationships(ctx, &v1.ReadRelationshipsRequest{
		Consistency: &v1.Consistency{
			Requirement: &v1.Consistency_FullyConsistent{
				FullyConsistent: true,
			},
		},
		RelationshipFilter: filter,
	})
	if err != nil {
		console.Errorf("delete_relationships: Failed to read relationships for deletion: %v\n", err)
		return mcp.NewToolResultError(fmt.Sprintf("Failed to read relationships for deletion: %v", err)), nil
	}

	var updates []*v1.RelationshipUpdate
	for {
		response, err := stream.Recv()
		if err != nil {
			break
		}
		updates = append(updates, &v1.RelationshipUpdate{
			Operation:    v1.RelationshipUpdate_OPERATION_DELETE,
			Relationship: response.Relationship,
		})
	}

	if len(updates) == 0 {
		console.Printf("delete_relationships: No relationships found matching the filter\n")
		return mcp.NewToolResultText("No relationships found matching the filter"), nil
	}

	console.Printf("delete_relationships: Deleting %d relationships...\n", len(updates))
	_, err = smcp.permService.WriteRelationships(ctx, &v1.WriteRelationshipsRequest{
		Updates: updates,
	})
	if err != nil {
		console.Errorf("delete_relationships: Failed to delete relationships: %v\n", err)
		return mcp.NewToolResultError(fmt.Sprintf("Failed to delete relationships: %v", err)), nil
	}

	console.Printf("delete_relationships: Successfully deleted %d relationships\n", len(updates))
	return mcp.NewToolResultText(fmt.Sprintf("Successfully deleted %d relationships", len(updates))), nil
}

func (smcp *spiceDBMCPServer) buildRelationship(rel RelationshipDef) (*v1.Relationship, error) {
	relationship := &v1.Relationship{
		Resource: &v1.ObjectReference{
			ObjectType: rel.ResourceType,
			ObjectId:   rel.ResourceID,
		},
		Relation: rel.Relation,
		Subject: &v1.SubjectReference{
			Object: &v1.ObjectReference{
				ObjectType: rel.SubjectType,
				ObjectId:   rel.SubjectID,
			},
		},
	}

	if rel.SubjectRelation != "" {
		relationship.Subject.OptionalRelation = rel.SubjectRelation
	}

	if rel.CaveatName != "" {
		var caveatContext *structpb.Struct
		if rel.CaveatContext != nil {
			context, err := structpb.NewStruct(rel.CaveatContext)
			if err != nil {
				return nil, fmt.Errorf("failed to create caveat context: %w", err)
			}
			caveatContext = context
		}

		relationship.OptionalCaveat = &v1.ContextualizedCaveat{
			CaveatName: rel.CaveatName,
			Context:    caveatContext,
		}
	}

	if rel.Expiration != nil {
		relationship.OptionalExpiresAt = timestamppb.New(*rel.Expiration)
	}

	return relationship, nil
}

func (smcp *spiceDBMCPServer) getAllRelationshipsHandler(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	console.Printf("MCP Resource: relationships://all requested\n")
	relationshipStrings, err := smcp.readRelationships(ctx)
	if err != nil {
		console.Errorf("relationships://all: Failed to read relationships: %v\n", err)
		return []mcp.ResourceContents{
			mcp.TextResourceContents{
				URI:      request.Params.URI,
				MIMEType: "application/json",
				Text:     fmt.Sprintf(`{"error": "Failed to read relationships: %v"}`, err),
			},
		}, nil
	}

	console.Printf("relationships://all: Retrieved %d relationships\n", len(relationshipStrings))
	jsonData, err := json.MarshalIndent(map[string]interface{}{
		"relationships": relationshipStrings,
		"count":         len(relationshipStrings),
	}, "", "  ")
	if err != nil {
		console.Errorf("relationships://all: Failed to marshal relationships: %v\n", err)
		return []mcp.ResourceContents{
			mcp.TextResourceContents{
				URI:      request.Params.URI,
				MIMEType: "application/json",
				Text:     fmt.Sprintf(`{"error": "Failed to marshal relationships: %v"}`, err),
			},
		}, nil
	}

	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      request.Params.URI,
			MIMEType: "application/json",
			Text:     string(jsonData),
		},
	}, nil
}

func (smcp *spiceDBMCPServer) checkPermissionHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	console.Printf("MCP Tool: check_permission called\n")
	resourceType, err := request.RequireString("resource_type")
	if err != nil {
		console.Errorf("check_permission: Failed to get resource_type parameter: %v\n", err)
		return mcp.NewToolResultError(err.Error()), nil
	}

	resourceID, err := request.RequireString("resource_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	permission, err := request.RequireString("permission_or_relation")
	if err != nil {
		console.Errorf("check_permission: Failed to get permission_or_relation parameter: %v\n", err)
		return mcp.NewToolResultError(err.Error()), nil
	}

	subjectType, err := request.RequireString("subject_type")
	if err != nil {
		console.Errorf("check_permission: Failed to get subject_type parameter: %v\n", err)
		return mcp.NewToolResultError(err.Error()), nil
	}

	subjectID, err := request.RequireString("subject_id")
	if err != nil {
		console.Errorf("check_permission: Failed to get subject_id parameter: %v\n", err)
		return mcp.NewToolResultError(err.Error()), nil
	}

	subjectRelation := request.GetString("optional_subject_relation", "")
	console.Printf("check_permission: Checking permission %s:%s#%s for %s:%s\n", resourceType, resourceID, permission, subjectType, subjectID)

	subject := &v1.SubjectReference{
		Object: &v1.ObjectReference{
			ObjectType: subjectType,
			ObjectId:   subjectID,
		},
	}

	if subjectRelation != "" {
		subject.OptionalRelation = subjectRelation
	}

	caveatContextString := request.GetString("optional_caveat_context_json", "")
	var caveatContext *structpb.Struct
	if caveatContextString != "" {
		caveatContextMap := make(map[string]interface{})
		if err := json.Unmarshal([]byte(caveatContextString), &caveatContextMap); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to parse caveat context: %v", err)), nil
		}

		cc, err := structpb.NewStruct(caveatContextMap)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to parse caveat context: %v", err)), nil
		}
		caveatContext = cc
	}

	console.Printf("check_permission: Executing permission check...\n")
	response, err := smcp.permService.CheckPermission(ctx, &v1.CheckPermissionRequest{
		Resource: &v1.ObjectReference{
			ObjectType: resourceType,
			ObjectId:   resourceID,
		},
		Permission: permission,
		Subject:    subject,
		Context:    caveatContext,
		Consistency: &v1.Consistency{
			Requirement: &v1.Consistency_FullyConsistent{
				FullyConsistent: true,
			},
		},
		WithTracing: true,
	})
	if err != nil {
		console.Errorf("check_permission: Failed to check permission: %v\n", err)
		return mcp.NewToolResultError(fmt.Sprintf("Failed to check permission: %v", err)), nil
	}

	console.Printf("check_permission: Permission check result: %s\n", response.Permissionship.String())

	var result strings.Builder
	result.WriteString(fmt.Sprintf("Permission: %s\n", response.Permissionship.String()))
	result.WriteString(fmt.Sprintf("Checked at revision: %s\n", response.CheckedAt.Token))

	if response.DebugTrace != nil {
		result.WriteString("\nDebug Trace:\n")
		traceJSON, _ := json.MarshalIndent(response.DebugTrace, "", "  ")
		result.WriteString(string(traceJSON))
	}

	return mcp.NewToolResultText(result.String()), nil
}

func (smcp *spiceDBMCPServer) lookupResourcesHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	console.Printf("MCP Tool: lookup_resources called\n")
	resourceObjectType, err := request.RequireString("resource_object_type")
	if err != nil {
		console.Errorf("lookup_resources: Failed to get resource_object_type parameter: %v\n", err)
		return mcp.NewToolResultError(err.Error()), nil
	}

	permission, err := request.RequireString("permission")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	subjectType, err := request.RequireString("subject_type")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	subjectID, err := request.RequireString("subject_id")
	if err != nil {
		console.Errorf("lookup_resources: Failed to get subject_id parameter: %v\n", err)
		return mcp.NewToolResultError(err.Error()), nil
	}

	subjectRelation := request.GetString("optional_subject_relation", "")
	console.Printf("lookup_resources: Looking up %s resources with permission %s for %s:%s\n", resourceObjectType, permission, subjectType, subjectID)

	subject := &v1.SubjectReference{
		Object: &v1.ObjectReference{
			ObjectType: subjectType,
			ObjectId:   subjectID,
		},
	}

	if subjectRelation != "" {
		subject.OptionalRelation = subjectRelation
	}

	caveatContextString := request.GetString("optional_caveat_context_json", "")
	var caveatContext *structpb.Struct
	if caveatContextString != "" {
		caveatContextMap := make(map[string]interface{})
		if err := json.Unmarshal([]byte(caveatContextString), &caveatContextMap); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to parse caveat context: %v", err)), nil
		}

		cc, err := structpb.NewStruct(caveatContextMap)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to parse caveat context: %v", err)), nil
		}
		caveatContext = cc
	}

	console.Printf("lookup_resources: Executing resource lookup...\n")
	stream, err := smcp.permService.LookupResources(ctx, &v1.LookupResourcesRequest{
		ResourceObjectType: resourceObjectType,
		Permission:         permission,
		Subject:            subject,
		Context:            caveatContext,
		Consistency: &v1.Consistency{
			Requirement: &v1.Consistency_FullyConsistent{
				FullyConsistent: true,
			},
		},
	})
	if err != nil {
		console.Errorf("lookup_resources: Failed to lookup resources: %v\n", err)
		return mcp.NewToolResultError(fmt.Sprintf("Failed to lookup resources: %v", err)), nil
	}

	var resources []string
	for {
		response, err := stream.Recv()
		if err != nil {
			break
		}
		resources = append(resources, response.ResourceObjectId)
	}

	console.Printf("lookup_resources: Found %d resources\n", len(resources))
	result, err := json.MarshalIndent(map[string]interface{}{
		"resources": resources,
		"count":     len(resources),
	}, "", "  ")
	if err != nil {
		console.Errorf("lookup_resources: Failed to marshal results: %v\n", err)
		return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal results: %v", err)), nil
	}

	return mcp.NewToolResultText(string(result)), nil
}

func (smcp *spiceDBMCPServer) lookupSubjectsHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	console.Printf("MCP Tool: lookup_subjects called\n")
	subjectObjectType, err := request.RequireString("subject_object_type")
	if err != nil {
		console.Errorf("lookup_subjects: Failed to get subject_object_type parameter: %v\n", err)
		return mcp.NewToolResultError(err.Error()), nil
	}

	permission, err := request.RequireString("permission")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	resourceType, err := request.RequireString("resource_type")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	resourceID, err := request.RequireString("resource_id")
	if err != nil {
		console.Errorf("lookup_subjects: Failed to get resource_id parameter: %v\n", err)
		return mcp.NewToolResultError(err.Error()), nil
	}

	resourceRelation := request.GetString("optional_resource_relation", "")
	console.Printf("lookup_subjects: Looking up %s subjects with permission %s for %s:%s\n", subjectObjectType, permission, resourceType, resourceID)

	resource := &v1.ObjectReference{
		ObjectType: resourceType,
		ObjectId:   resourceID,
	}

	caveatContextString := request.GetString("optional_caveat_context_json", "")
	var caveatContext *structpb.Struct
	if caveatContextString != "" {
		caveatContextMap := make(map[string]interface{})
		if err := json.Unmarshal([]byte(caveatContextString), &caveatContextMap); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to parse caveat context: %v", err)), nil
		}

		cc, err := structpb.NewStruct(caveatContextMap)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to parse caveat context: %v", err)), nil
		}
		caveatContext = cc
	}

	lookupRequest := &v1.LookupSubjectsRequest{
		SubjectObjectType: subjectObjectType,
		Permission:        permission,
		Resource:          resource,
		Context:           caveatContext,
		Consistency: &v1.Consistency{
			Requirement: &v1.Consistency_FullyConsistent{
				FullyConsistent: true,
			},
		},
	}

	if resourceRelation != "" {
		lookupRequest.OptionalSubjectRelation = resourceRelation
	}

	console.Printf("lookup_subjects: Executing subject lookup...\n")
	stream, err := smcp.permService.LookupSubjects(ctx, lookupRequest)
	if err != nil {
		console.Errorf("lookup_subjects: Failed to lookup subjects: %v\n", err)
		return mcp.NewToolResultError(fmt.Sprintf("Failed to lookup subjects: %v", err)), nil
	}

	var subjects []string
	for {
		response, err := stream.Recv()
		if err != nil {
			break
		}
		subjects = append(subjects, response.Subject.SubjectObjectId)
	}

	console.Printf("lookup_subjects: Found %d subjects\n", len(subjects))
	result, err := json.MarshalIndent(map[string]interface{}{
		"subjects": subjects,
		"count":    len(subjects),
	}, "", "  ")
	if err != nil {
		console.Errorf("lookup_subjects: Failed to marshal results: %v\n", err)
		return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal results: %v", err)), nil
	}

	return mcp.NewToolResultText(string(result)), nil
}

func (smcp *spiceDBMCPServer) readSchema(ctx context.Context) (string, error) {
	response, err := smcp.schemaService.ReadSchema(ctx, &v1.ReadSchemaRequest{})
	if err != nil {
		return "", fmt.Errorf("failed to read schema: %w", err)
	}
	return response.SchemaText, nil
}

func (smcp *spiceDBMCPServer) readRelationships(ctx context.Context) ([]string, error) {
	// First, get the schema definitions using ReflectSchema
	reflectResponse, err := smcp.schemaService.ReflectSchema(ctx, &v1.ReflectSchemaRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to reflect schema: %w", err)
	}

	var relationshipStrings []string

	// For each definition, read relationships with that resource type
	for _, definition := range reflectResponse.Definitions {
		relationshipsStream, err := smcp.permService.ReadRelationships(ctx, &v1.ReadRelationshipsRequest{
			Consistency: &v1.Consistency{
				Requirement: &v1.Consistency_FullyConsistent{
					FullyConsistent: true,
				},
			},
			RelationshipFilter: &v1.RelationshipFilter{
				ResourceType: definition.Name,
			},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to read relationships for resource type %s: %w", definition.Name, err)
		}

		// Collect relationships for this resource type
		for {
			response, err := relationshipsStream.Recv()
			if err != nil {
				break
			}

			rel := response.Relationship
			relationshipStr := fmt.Sprintf("%s:%s#%s@%s:%s",
				rel.Resource.ObjectType,
				rel.Resource.ObjectId,
				rel.Relation,
				rel.Subject.Object.ObjectType,
				rel.Subject.Object.ObjectId)

			if rel.Subject.OptionalRelation != "" {
				relationshipStr += "#" + rel.Subject.OptionalRelation
			}

			relationshipStrings = append(relationshipStrings, relationshipStr)
		}
	}

	return relationshipStrings, nil
}

func (smcp *spiceDBMCPServer) getValidationFileHandler(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	console.Printf("MCP Resource: validation://current requested\n")

	// Get current schema
	schema, err := smcp.readSchema(ctx)
	if err != nil {
		console.Errorf("validation://current: Failed to read schema: %v\n", err)
		return []mcp.ResourceContents{
			mcp.TextResourceContents{
				URI:      request.Params.URI,
				MIMEType: "application/yaml",
				Text:     fmt.Sprintf("# Error reading schema: %v", err),
			},
		}, nil
	}

	// Get current relationships
	relationshipStrings, err := smcp.readRelationships(ctx)
	if err != nil {
		console.Errorf("validation://current: Failed to read relationships: %v\n", err)
		return []mcp.ResourceContents{
			mcp.TextResourceContents{
				URI:      request.Params.URI,
				MIMEType: "application/yaml",
				Text:     fmt.Sprintf("# Error reading relationships: %v", err),
			},
		}, nil
	}

	// Create simple validation file structure
	validationFile := ValidationFile{
		Schema:        schema,
		Relationships: relationshipStrings,
	}

	// Marshal to YAML
	yamlData, err := yaml.Marshal(validationFile)
	if err != nil {
		console.Errorf("validation://current: Failed to marshal validation file: %v\n", err)
		return []mcp.ResourceContents{
			mcp.TextResourceContents{
				URI:      request.Params.URI,
				MIMEType: "application/yaml",
				Text:     fmt.Sprintf("# Error marshaling validation file: %v", err),
			},
		}, nil
	}

	console.Printf("validation://current: Successfully created validation file with %d relationships\n", len(relationshipStrings))

	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      request.Params.URI,
			MIMEType: "application/yaml",
			Text:     string(yamlData),
		},
	}, nil
}

func newSpiceDBServer(ctx context.Context) (server.RunnableServer, error) {
	ds, err := datastore.NewDatastore(ctx,
		datastore.DefaultDatastoreConfig().ToOption(),
		datastore.WithRequestHedgingEnabled(false),
	)
	if err != nil {
		return nil, fmt.Errorf("unable to start memdb datastore: %w", err)
	}

	configOpts := []server.ConfigOption{
		server.WithGRPCServer(util.GRPCServerConfig{
			Network: util.BufferedNetwork,
			Enabled: true,
		}),
		server.WithGRPCAuthFunc(func(ctx context.Context) (context.Context, error) {
			return ctx, nil
		}),
		server.WithHTTPGateway(util.HTTPServerConfig{HTTPEnabled: false}),
		server.WithMetricsAPI(util.HTTPServerConfig{HTTPEnabled: false}),
		// disable caching since it's all in memory
		server.WithDispatchCacheConfig(server.CacheConfig{Enabled: false, Metrics: false}),
		server.WithNamespaceCacheConfig(server.CacheConfig{Enabled: false, Metrics: false}),
		server.WithClusterDispatchCacheConfig(server.CacheConfig{Enabled: false, Metrics: false}),
		server.WithDatastore(ds),
	}

	return server.NewConfigWithOptionsAndDefaults(configOpts...).Complete(ctx)
}
