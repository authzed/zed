package commands

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	google_rpc "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"

	"github.com/authzed/zed/internal/client"
)

// mockBulkCheckClient simulates a SpiceDB client for bulk permission checks with debug traces
type mockBulkCheckClient struct {
	v1.SchemaServiceClient
	v1.PermissionsServiceClient
	v1.WatchServiceClient
	v1.ExperimentalServiceClient
	t *testing.T
}

func (m *mockBulkCheckClient) CheckBulkPermissions(_ context.Context, req *v1.CheckBulkPermissionsRequest, _ ...grpc.CallOption) (*v1.CheckBulkPermissionsResponse, error) {
	// Verify tracing is enabled
	require.True(m.t, req.WithTracing, "WithTracing should be enabled for --html")

	// Return mock response with debug traces
	return &v1.CheckBulkPermissionsResponse{
		Pairs: []*v1.CheckBulkPermissionsPair{
			{
				Request: req.Items[0],
				Response: &v1.CheckBulkPermissionsPair_Item{
					Item: &v1.CheckBulkPermissionsResponseItem{
						Permissionship: v1.CheckPermissionResponse_PERMISSIONSHIP_HAS_PERMISSION,
						DebugTrace: &v1.DebugInformation{
							Check: &v1.CheckDebugTrace{
								Resource: &v1.ObjectReference{
									ObjectType: "document",
									ObjectId:   "doc1",
								},
								Permission: "viewer",
								Result:     v1.CheckDebugTrace_PERMISSIONSHIP_HAS_PERMISSION,
								Duration:   durationpb.New(5000000),
							},
						},
					},
				},
			},
			{
				Request: req.Items[1],
				Response: &v1.CheckBulkPermissionsPair_Item{
					Item: &v1.CheckBulkPermissionsResponseItem{
						Permissionship: v1.CheckPermissionResponse_PERMISSIONSHIP_NO_PERMISSION,
						DebugTrace: &v1.DebugInformation{
							Check: &v1.CheckDebugTrace{
								Resource: &v1.ObjectReference{
									ObjectType: "document",
									ObjectId:   "doc2",
								},
								Permission: "admin",
								Result:     v1.CheckDebugTrace_PERMISSIONSHIP_NO_PERMISSION,
								Duration:   durationpb.New(8000000),
							},
						},
					},
				},
			},
		},
	}, nil
}

// TestBulkCheckWithHTMLOutput tests the end-to-end flow of bulk check with --html flag
func TestBulkCheckWithHTMLOutput(t *testing.T) {
	mock := func(*cobra.Command) (client.Client, error) {
		return &mockBulkCheckClient{t: t}, nil
	}

	originalClient := client.NewClient
	client.NewClient = mock
	defer func() {
		client.NewClient = originalClient
	}()

	// Create temporary directory for HTML output
	tmpDir := t.TempDir()
	htmlPath := filepath.Join(tmpDir, "bulk-test.html")

	cmd := &cobra.Command{}
	cmd.Flags().String("revision", "", "optional revision")
	cmd.Flags().Bool("json", false, "output as JSON")
	cmd.Flags().Bool("explain", false, "explain")
	cmd.Flags().Bool("html", false, "html output")
	cmd.Flags().String("html-output", htmlPath, "html output path")
	cmd.Flags().Bool("schema", false, "schema")
	registerConsistencyFlags(cmd.Flags())

	// Set the --html flag
	require.NoError(t, cmd.Flags().Set("html", "true"))
	require.NoError(t, cmd.Flags().Set("html-output", htmlPath))

	// Run bulk check
	err := checkBulkCmdFunc(cmd, []string{
		"document:doc1#viewer@user:alice",
		"document:doc2#admin@user:bob",
	})
	require.NoError(t, err)

	// Verify HTML file was created
	require.FileExists(t, htmlPath, "HTML file should be created")

	// Read and verify HTML content
	htmlContent, err := os.ReadFile(htmlPath)
	require.NoError(t, err)
	htmlStr := string(htmlContent)

	// Verify HTML structure
	require.Contains(t, htmlStr, "<!DOCTYPE html>", "Should be valid HTML")
	require.Contains(t, htmlStr, "SpiceDB Permission Check Trace", "Should have title")
	require.Contains(t, htmlStr, "</html>", "Should be complete HTML")

	// Verify both checks are in the output
	require.Contains(t, htmlStr, "Check #1", "Should have first check")
	require.Contains(t, htmlStr, "Check #2", "Should have second check")

	// Verify trace content
	require.Contains(t, htmlStr, "document:doc1", "Should contain first document")
	require.Contains(t, htmlStr, "document:doc2", "Should contain second document")
	require.Contains(t, htmlStr, "viewer", "Should contain viewer permission")
	require.Contains(t, htmlStr, "admin", "Should contain admin permission")

	// Verify result indicators are present
	require.Contains(t, htmlStr, "has-permission", "Should have success indicator")
	require.Contains(t, htmlStr, "no-permission", "Should have failure indicator")

	// Verify file permissions are secure (0o600 on Unix, file exists on Windows)
	info, err := os.Stat(htmlPath)
	require.NoError(t, err)
	if runtime.GOOS != "windows" {
		require.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "File should have 0o600 permissions")
	} else {
		// On Windows, just verify the file is a regular file
		require.True(t, info.Mode().IsRegular(), "Should be a regular file")
	}
}

// TestBulkCheckWithHTMLAndExplain tests that --html works without --explain
func TestBulkCheckHTMLOnlyMode(t *testing.T) {
	mock := func(*cobra.Command) (client.Client, error) {
		return &mockBulkCheckClient{t: t}, nil
	}

	originalClient := client.NewClient
	client.NewClient = mock
	defer func() {
		client.NewClient = originalClient
	}()

	tmpDir := t.TempDir()
	htmlPath := filepath.Join(tmpDir, "bulk-html-only-test.html")

	cmd := &cobra.Command{}
	cmd.Flags().String("revision", "", "optional revision")
	cmd.Flags().Bool("json", false, "output as JSON")
	cmd.Flags().Bool("explain", false, "explain")
	cmd.Flags().Bool("html", false, "html output")
	cmd.Flags().String("html-output", htmlPath, "html output path")
	cmd.Flags().Bool("schema", false, "schema")
	registerConsistencyFlags(cmd.Flags())

	// Set ONLY --html flag (not --explain)
	require.NoError(t, cmd.Flags().Set("html", "true"))
	require.NoError(t, cmd.Flags().Set("html-output", htmlPath))

	// Run bulk check with only HTML flag
	err := checkBulkCmdFunc(cmd, []string{
		"document:doc1#viewer@user:alice",
		"document:doc2#admin@user:bob",
	})
	require.NoError(t, err)

	// Verify HTML file was created ONCE (not overwritten per-item)
	require.FileExists(t, htmlPath)

	// Read HTML content
	htmlContent, err := os.ReadFile(htmlPath)
	require.NoError(t, err)
	htmlStr := string(htmlContent)

	// Verify aggregated output (both checks in one file)
	checkCount := strings.Count(htmlStr, "Check #")
	require.Equal(t, 2, checkCount, "Should have exactly 2 checks in the aggregated HTML")

	// Verify both traces are complete
	require.Contains(t, htmlStr, "document:doc1")
	require.Contains(t, htmlStr, "document:doc2")

	// Verify it's a single aggregated file, not multiple writes
	require.Contains(t, htmlStr, "<!DOCTYPE html>")
	require.Contains(t, htmlStr, "</html>")
}

// mockSingleCheckClient simulates a SpiceDB client for single permission check with debug trace
type mockSingleCheckClient struct {
	v1.SchemaServiceClient
	v1.PermissionsServiceClient
	v1.WatchServiceClient
	v1.ExperimentalServiceClient
	t           *testing.T
	shouldError bool
}

func (m *mockSingleCheckClient) CheckPermission(_ context.Context, req *v1.CheckPermissionRequest, _ ...grpc.CallOption) (*v1.CheckPermissionResponse, error) {
	// Verify tracing is enabled
	require.True(m.t, req.WithTracing, "WithTracing should be enabled for --html")

	if m.shouldError {
		// Return error with NO_PERMISSION result
		return &v1.CheckPermissionResponse{
			CheckedAt:      nil,
			Permissionship: v1.CheckPermissionResponse_PERMISSIONSHIP_NO_PERMISSION,
			DebugTrace: &v1.DebugInformation{
				Check: &v1.CheckDebugTrace{
					Resource: &v1.ObjectReference{
						ObjectType: "document",
						ObjectId:   "secret",
					},
					Permission: "admin",
					Result:     v1.CheckDebugTrace_PERMISSIONSHIP_NO_PERMISSION,
					Duration:   durationpb.New(3000000),
				},
			},
		}, nil
	}

	// Return successful check with debug trace
	return &v1.CheckPermissionResponse{
		CheckedAt:      nil,
		Permissionship: v1.CheckPermissionResponse_PERMISSIONSHIP_HAS_PERMISSION,
		DebugTrace: &v1.DebugInformation{
			Check: &v1.CheckDebugTrace{
				Resource: &v1.ObjectReference{
					ObjectType: "document",
					ObjectId:   "report",
				},
				Permission: "viewer",
				Result:     v1.CheckDebugTrace_PERMISSIONSHIP_HAS_PERMISSION,
				Duration:   durationpb.New(4000000),
			},
		},
	}, nil
}

// TestSingleCheckWithHTMLSuccess tests single permission check with --html flag (success path)
func TestSingleCheckWithHTMLSuccess(t *testing.T) {
	mock := func(*cobra.Command) (client.Client, error) {
		return &mockSingleCheckClient{t: t, shouldError: false}, nil
	}

	originalClient := client.NewClient
	client.NewClient = mock
	defer func() {
		client.NewClient = originalClient
	}()

	// Create temporary directory for HTML output
	tmpDir := t.TempDir()
	htmlPath := filepath.Join(tmpDir, "single-check.html")

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	cmd.Flags().String("revision", "", "optional revision")
	cmd.Flags().Bool("json", false, "output as JSON")
	cmd.Flags().Bool("explain", false, "explain")
	cmd.Flags().Bool("html", false, "html output")
	cmd.Flags().String("html-output", htmlPath, "html output path")
	cmd.Flags().Bool("schema", false, "schema")
	cmd.Flags().Bool("error-on-no-permission", false, "error on no permission")
	cmd.Flags().String("caveat-context", "", "caveat context")
	registerConsistencyFlags(cmd.Flags())

	// Set the --html flag
	require.NoError(t, cmd.Flags().Set("html", "true"))
	require.NoError(t, cmd.Flags().Set("html-output", htmlPath))

	// Run single check (args: resource, relation, subject)
	err := checkCmdFunc(cmd, []string{"document:report", "viewer", "user:alice"})
	require.NoError(t, err)

	// Verify HTML file was created
	require.FileExists(t, htmlPath, "HTML file should be created")

	// Read and verify HTML content
	htmlContent, err := os.ReadFile(htmlPath)
	require.NoError(t, err)
	htmlStr := string(htmlContent)

	// Verify HTML structure
	require.Contains(t, htmlStr, "<!DOCTYPE html>", "Should be valid HTML")
	require.Contains(t, htmlStr, "SpiceDB Permission Check Trace", "Should have title")
	require.Contains(t, htmlStr, "</html>", "Should be complete HTML")

	// Verify check content
	require.Contains(t, htmlStr, "document:report", "Should contain resource")
	require.Contains(t, htmlStr, "viewer", "Should contain permission")
	require.Contains(t, htmlStr, "has-permission", "Should have success indicator")

	// Verify file permissions are secure (0o600 on Unix, file exists on Windows)
	info, err := os.Stat(htmlPath)
	require.NoError(t, err)
	if runtime.GOOS != "windows" {
		require.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "File should have 0o600 permissions")
	} else {
		require.True(t, info.Mode().IsRegular(), "Should be a regular file")
	}
}

// TestSingleCheckWithHTMLError tests single permission check with --html flag (error/no permission path)
func TestSingleCheckWithHTMLError(t *testing.T) {
	mock := func(*cobra.Command) (client.Client, error) {
		return &mockSingleCheckClient{t: t, shouldError: true}, nil
	}

	originalClient := client.NewClient
	client.NewClient = mock
	defer func() {
		client.NewClient = originalClient
	}()

	// Create temporary directory for HTML output
	tmpDir := t.TempDir()
	htmlPath := filepath.Join(tmpDir, "single-check-error.html")

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	cmd.Flags().String("revision", "", "optional revision")
	cmd.Flags().Bool("json", false, "output as JSON")
	cmd.Flags().Bool("explain", false, "explain")
	cmd.Flags().Bool("html", false, "html output")
	cmd.Flags().String("html-output", htmlPath, "html output path")
	cmd.Flags().Bool("schema", false, "schema")
	cmd.Flags().Bool("error-on-no-permission", false, "error on no permission")
	cmd.Flags().String("caveat-context", "", "caveat context")
	registerConsistencyFlags(cmd.Flags())

	// Set the --html flag
	require.NoError(t, cmd.Flags().Set("html", "true"))
	require.NoError(t, cmd.Flags().Set("html-output", htmlPath))

	// Run single check (args: resource, relation, subject) (expect no permission)
	err := checkCmdFunc(cmd, []string{"document:secret", "admin", "user:bob"})
	require.NoError(t, err) // Should not error unless error-on-no-permission is set

	// Verify HTML file was created
	require.FileExists(t, htmlPath, "HTML file should be created even for no permission")

	// Read and verify HTML content
	htmlContent, err := os.ReadFile(htmlPath)
	require.NoError(t, err)
	htmlStr := string(htmlContent)

	// Verify HTML structure
	require.Contains(t, htmlStr, "<!DOCTYPE html>", "Should be valid HTML")
	require.Contains(t, htmlStr, "SpiceDB Permission Check Trace", "Should have title")
	require.Contains(t, htmlStr, "</html>", "Should be complete HTML")

	// Verify check content shows no permission
	require.Contains(t, htmlStr, "document:secret", "Should contain resource")
	require.Contains(t, htmlStr, "admin", "Should contain permission")
	require.Contains(t, htmlStr, "no-permission", "Should have no-permission indicator")

	// Verify file permissions are secure
	info, err := os.Stat(htmlPath)
	require.NoError(t, err)
	if runtime.GOOS != "windows" {
		require.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "File should have 0o600 permissions")
	} else {
		require.True(t, info.Mode().IsRegular(), "Should be a regular file")
	}
}

// mockBulkCheckClientWithError simulates a SpiceDB client that returns both successful and error responses
type mockBulkCheckClientWithError struct {
	v1.SchemaServiceClient
	v1.PermissionsServiceClient
	v1.WatchServiceClient
	v1.ExperimentalServiceClient
	t *testing.T
}

func (m *mockBulkCheckClientWithError) CheckBulkPermissions(_ context.Context, req *v1.CheckBulkPermissionsRequest, _ ...grpc.CallOption) (*v1.CheckBulkPermissionsResponse, error) {
	require.True(m.t, req.WithTracing, "WithTracing should be enabled for --html")

	// Create a cycle: document:circular -> group:managers -> document:circular (back to start)
	// This creates a real cycle that will be detected by the renderer
	circularRef := &v1.CheckDebugTrace{
		Resource: &v1.ObjectReference{
			ObjectType: "document",
			ObjectId:   "circular",
		},
		Permission: "viewer",
		Result:     v1.CheckDebugTrace_PERMISSIONSHIP_UNSPECIFIED,
		Duration:   durationpb.New(500000),
	}

	managers := &v1.CheckDebugTrace{
		Resource: &v1.ObjectReference{
			ObjectType: "group",
			ObjectId:   "managers",
		},
		Permission: "member",
		Result:     v1.CheckDebugTrace_PERMISSIONSHIP_HAS_PERMISSION,
		Duration:   durationpb.New(800000),
		Resolution: &v1.CheckDebugTrace_SubProblems_{
			SubProblems: &v1.CheckDebugTrace_SubProblems{
				Traces: []*v1.CheckDebugTrace{circularRef}, // This creates the cycle
			},
		},
	}

	cycleTrace := &v1.CheckDebugTrace{
		Resource: &v1.ObjectReference{
			ObjectType: "document",
			ObjectId:   "circular",
		},
		Permission: "viewer",
		Result:     v1.CheckDebugTrace_PERMISSIONSHIP_UNSPECIFIED,
		Duration:   durationpb.New(1000000),
		Resolution: &v1.CheckDebugTrace_SubProblems_{
			SubProblems: &v1.CheckDebugTrace_SubProblems{
				Traces: []*v1.CheckDebugTrace{managers},
			},
		},
	}

	// Encode the debug trace as proto text for the error metadata (matching production format)
	debugInfo := &v1.DebugInformation{Check: cycleTrace}
	encodedTrace, err := prototext.MarshalOptions{}.Marshal(debugInfo)
	require.NoError(m.t, err)

	// Create error response with embedded debug trace using errdetails
	errInfo := &errdetails.ErrorInfo{
		Reason: "CYCLE_DETECTED",
		Domain: "authzed.com",
		Metadata: map[string]string{
			"debug_trace_proto_text": string(encodedTrace),
		},
	}

	// Marshal the ErrorInfo to Any
	errInfoAny, err := anypb.New(errInfo)
	require.NoError(m.t, err)

	cycleError := &v1.CheckBulkPermissionsPair_Error{
		Error: &google_rpc.Status{
			Code:    int32(codes.FailedPrecondition),
			Message: "cycle detected",
			Details: []*anypb.Any{errInfoAny},
		},
	}

	return &v1.CheckBulkPermissionsResponse{
		Pairs: []*v1.CheckBulkPermissionsPair{
			{
				Request: req.Items[0],
				Response: &v1.CheckBulkPermissionsPair_Item{
					Item: &v1.CheckBulkPermissionsResponseItem{
						Permissionship: v1.CheckPermissionResponse_PERMISSIONSHIP_HAS_PERMISSION,
						DebugTrace: &v1.DebugInformation{
							Check: &v1.CheckDebugTrace{
								Resource: &v1.ObjectReference{
									ObjectType: "document",
									ObjectId:   "doc1",
								},
								Permission: "viewer",
								Result:     v1.CheckDebugTrace_PERMISSIONSHIP_HAS_PERMISSION,
								Duration:   durationpb.New(5000000),
							},
						},
					},
				},
			},
			{
				Request:  req.Items[1],
				Response: cycleError,
			},
		},
	}, nil
}

// TestBulkCheckWithHTMLAndError tests that hasError=true is properly set for error responses
func TestBulkCheckWithHTMLAndError(t *testing.T) {
	mock := func(*cobra.Command) (client.Client, error) {
		return &mockBulkCheckClientWithError{t: t}, nil
	}

	originalClient := client.NewClient
	client.NewClient = mock
	defer func() {
		client.NewClient = originalClient
	}()

	tmpDir := t.TempDir()
	htmlPath := filepath.Join(tmpDir, "bulk-with-error.html")

	cmd := &cobra.Command{}
	cmd.Flags().String("revision", "", "optional revision")
	cmd.Flags().Bool("json", false, "output as JSON")
	cmd.Flags().Bool("explain", false, "explain")
	cmd.Flags().Bool("html", false, "html output")
	cmd.Flags().String("html-output", htmlPath, "html output path")
	cmd.Flags().Bool("schema", false, "schema")
	registerConsistencyFlags(cmd.Flags())

	require.NoError(t, cmd.Flags().Set("html", "true"))
	require.NoError(t, cmd.Flags().Set("html-output", htmlPath))

	// Run bulk check with one success and one error
	err := checkBulkCmdFunc(cmd, []string{
		"document:doc1#viewer@user:alice",
		"document:circular#viewer@user:bob",
	})
	require.NoError(t, err)

	// Verify HTML file was created
	require.FileExists(t, htmlPath)

	// Read and verify HTML content
	htmlContent, err := os.ReadFile(htmlPath)
	require.NoError(t, err)
	htmlStr := string(htmlContent)

	// Verify both checks are in the output
	require.Contains(t, htmlStr, "Check #1", "Should have first check")
	require.Contains(t, htmlStr, "Check #2", "Should have second check")

	// Verify the successful check
	require.Contains(t, htmlStr, "document:doc1", "Should contain successful check")
	require.Contains(t, htmlStr, "has-permission", "Should have success indicator")

	// Verify the error check with cycle badge
	require.Contains(t, htmlStr, "document:circular", "Should contain error check")
	require.Contains(t, htmlStr, "badge cycle", "Should have cycle badge for error")
	require.Contains(t, htmlStr, "icon cycle", "Should have cycle icon for error")
}

func TestHTMLDefaultOutputAppendsTimestamp(t *testing.T) {
	mock := func(*cobra.Command) (client.Client, error) {
		return &mockSingleCheckClient{t: t, shouldError: false}, nil
	}

	originalClient := client.NewClient
	client.NewClient = mock
	defer func() {
		client.NewClient = originalClient
	}()

	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	cmd.Flags().String("revision", "", "optional revision")
	cmd.Flags().Bool("json", false, "output as JSON")
	cmd.Flags().Bool("explain", false, "explain")
	cmd.Flags().Bool("html", false, "html output")
	cmd.Flags().String("html-output", "trace.html", "html output path")
	cmd.Flags().Bool("schema", false, "schema")
	cmd.Flags().Bool("error-on-no-permission", false, "error on no permission")
	cmd.Flags().String("caveat-context", "", "caveat context")
	registerConsistencyFlags(cmd.Flags())

	require.NoError(t, cmd.Flags().Set("html", "true"))

	err := checkCmdFunc(cmd, []string{"document:report", "viewer", "user:alice"})
	require.NoError(t, err)

	matches, err := filepath.Glob(filepath.Join(tmpDir, "trace-*.html"))
	require.NoError(t, err)
	require.Len(t, matches, 1)

	info, err := os.Stat(matches[0])
	require.NoError(t, err)
	if runtime.GOOS != "windows" {
		require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	} else {
		require.True(t, info.Mode().IsRegular())
	}
}

func TestHTMLOutputToDirectory(t *testing.T) {
	mock := func(*cobra.Command) (client.Client, error) {
		return &mockSingleCheckClient{t: t, shouldError: false}, nil
	}

	originalClient := client.NewClient
	client.NewClient = mock
	defer func() {
		client.NewClient = originalClient
	}()

	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "traces")
	require.NoError(t, os.MkdirAll(outputDir, 0o755))

	t.Chdir(tmpDir)

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	cmd.Flags().String("revision", "", "optional revision")
	cmd.Flags().Bool("json", false, "output as JSON")
	cmd.Flags().Bool("explain", false, "explain")
	cmd.Flags().Bool("html", false, "html output")
	cmd.Flags().String("html-output", "trace.html", "html output path")
	cmd.Flags().Bool("schema", false, "schema")
	cmd.Flags().Bool("error-on-no-permission", false, "error on no permission")
	cmd.Flags().String("caveat-context", "", "caveat context")
	registerConsistencyFlags(cmd.Flags())

	require.NoError(t, cmd.Flags().Set("html", "true"))
	require.NoError(t, cmd.Flags().Set("html-output", "traces/")) // Directory with trailing slash

	err := checkCmdFunc(cmd, []string{"document:report", "viewer", "user:alice"})
	require.NoError(t, err)

	// Verify file was created in the directory with timestamp
	matches, err := filepath.Glob(filepath.Join(outputDir, "trace-*.html"))
	require.NoError(t, err)
	require.Len(t, matches, 1, "Should create one file in the directory")

	// Verify file has content
	content, err := os.ReadFile(matches[0])
	require.NoError(t, err)
	require.Contains(t, string(content), "<!DOCTYPE html>")
	require.Contains(t, string(content), "document:report")
}

func TestBulkCheckHTMLDefaultOutputAppendsTimestamp(t *testing.T) {
	mock := func(*cobra.Command) (client.Client, error) {
		return &mockBulkCheckClient{t: t}, nil
	}

	originalClient := client.NewClient
	client.NewClient = mock
	defer func() {
		client.NewClient = originalClient
	}()

	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	cmd.Flags().String("revision", "", "optional revision")
	cmd.Flags().Bool("json", false, "output as JSON")
	cmd.Flags().Bool("explain", false, "explain")
	cmd.Flags().Bool("html", false, "html output")
	cmd.Flags().String("html-output", "trace.html", "html output path")
	cmd.Flags().Bool("schema", false, "schema")
	cmd.Flags().Bool("error-on-no-permission", false, "error on no permission")
	cmd.Flags().String("caveat-context", "", "caveat context")
	registerConsistencyFlags(cmd.Flags())

	require.NoError(t, cmd.Flags().Set("html", "true"))

	// Test bulk check with default filename (should append timestamp)
	err := checkBulkCmdFunc(cmd, []string{"document:doc1#viewer@user:alice", "document:doc2#admin@user:bob"})
	require.NoError(t, err)

	matches, err := filepath.Glob(filepath.Join(tmpDir, "trace-*.html"))
	require.NoError(t, err)
	require.Len(t, matches, 1, "Should create one timestamped file")

	// Verify it's a bulk trace with multiple checks
	content, err := os.ReadFile(matches[0])
	require.NoError(t, err)
	require.Contains(t, string(content), "<!DOCTYPE html>")
	require.Contains(t, string(content), "Check #1")
	require.Contains(t, string(content), "Check #2")

	info, err := os.Stat(matches[0])
	require.NoError(t, err)
	if runtime.GOOS != "windows" {
		require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	} else {
		require.True(t, info.Mode().IsRegular())
	}
}
