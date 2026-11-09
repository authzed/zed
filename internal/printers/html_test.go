package printers

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
)

func TestDisplayCheckTraceHTML(t *testing.T) {
	trace := &v1.CheckDebugTrace{
		Resource: &v1.ObjectReference{
			ObjectType: "document",
			ObjectId:   "testdoc",
		},
		Permission: "viewer",
		Result:     v1.CheckDebugTrace_PERMISSIONSHIP_HAS_PERMISSION,
		Duration:   durationpb.New(5000000), // 5ms
	}

	html := DisplayCheckTraceHTML(trace, false)

	// Verify HTML structure
	require.Contains(t, html, "<!DOCTYPE html>")
	require.Contains(t, html, "SpiceDB Permission Check Trace")
	require.Contains(t, html, "document:testdoc")
	require.Contains(t, html, "viewer")
	require.Contains(t, html, "has-permission")
	require.Contains(t, html, "</html>")
}

func TestDisplayCheckTraceHTMLWithCaveat(t *testing.T) {
	contextMap := map[string]any{
		"department": "engineering",
	}
	context, _ := structpb.NewStruct(contextMap)

	trace := &v1.CheckDebugTrace{
		Resource: &v1.ObjectReference{
			ObjectType: "document",
			ObjectId:   "testdoc",
		},
		Permission: "admin",
		Result:     v1.CheckDebugTrace_PERMISSIONSHIP_CONDITIONAL_PERMISSION,
		CaveatEvaluationInfo: &v1.CaveatEvalInfo{
			Expression: "is_admin == true",
			Result:     v1.CaveatEvalInfo_RESULT_TRUE,
			CaveatName: "admin_check",
			Context:    context,
		},
	}

	html := DisplayCheckTraceHTML(trace, false)

	// Verify caveat rendering
	require.Contains(t, html, "is_admin == true")
	require.Contains(t, html, "admin_check")
	require.Contains(t, html, "engineering")
	require.Contains(t, html, "caveat-node")
}

func TestDisplayCheckTraceHTMLWithMissingContext(t *testing.T) {
	trace := &v1.CheckDebugTrace{
		Resource: &v1.ObjectReference{
			ObjectType: "document",
			ObjectId:   "testdoc",
		},
		Permission: "admin",
		Result:     v1.CheckDebugTrace_PERMISSIONSHIP_CONDITIONAL_PERMISSION,
		CaveatEvaluationInfo: &v1.CaveatEvalInfo{
			Expression: "is_admin == true",
			Result:     v1.CaveatEvalInfo_RESULT_MISSING_SOME_CONTEXT,
			CaveatName: "admin_check",
			PartialCaveatInfo: &v1.PartialCaveatInfo{
				MissingRequiredContext: []string{"is_admin"},
			},
		},
	}

	html := DisplayCheckTraceHTML(trace, false)

	// Verify missing context display
	require.Contains(t, html, "missing context")
	require.Contains(t, html, "is_admin")
	require.Contains(t, html, "conditional")
}

func TestDisplayCheckTraceHTMLWithSubject(t *testing.T) {
	trace := &v1.CheckDebugTrace{
		Resource: &v1.ObjectReference{
			ObjectType: "document",
			ObjectId:   "testdoc",
		},
		Permission: "writer",
		Result:     v1.CheckDebugTrace_PERMISSIONSHIP_HAS_PERMISSION,
		Subject: &v1.SubjectReference{
			Object: &v1.ObjectReference{
				ObjectType: "user",
				ObjectId:   "alice",
			},
		},
	}

	html := DisplayCheckTraceHTML(trace, false)

	// Verify nested structure
	require.Contains(t, html, "document:testdoc")
	require.Contains(t, html, "writer")
	require.Contains(t, html, "user:alice")
}

func TestDisplayCheckTraceHTMLPermissionType(t *testing.T) {
	trace := &v1.CheckDebugTrace{
		Resource: &v1.ObjectReference{
			ObjectType: "document",
			ObjectId:   "testdoc",
		},
		Permission:     "viewer",
		Result:         v1.CheckDebugTrace_PERMISSIONSHIP_HAS_PERMISSION,
		PermissionType: v1.CheckDebugTrace_PERMISSION_TYPE_PERMISSION,
	}

	html := DisplayCheckTraceHTML(trace, false)

	// Verify permission type styling
	require.Contains(t, html, "permission")
	require.Contains(t, html, "viewer")
}

func TestDisplayCheckTraceHTMLNoPermission(t *testing.T) {
	trace := &v1.CheckDebugTrace{
		Resource: &v1.ObjectReference{
			ObjectType: "document",
			ObjectId:   "testdoc",
		},
		Permission: "admin",
		Result:     v1.CheckDebugTrace_PERMISSIONSHIP_NO_PERMISSION,
	}

	html := DisplayCheckTraceHTML(trace, false)

	// Verify no permission styling
	require.Contains(t, html, "no-permission")
	require.Contains(t, html, "⨉")
}

func TestDisplayBulkCheckTracesHTML(t *testing.T) {
	traces := []*v1.CheckDebugTrace{
		{
			Resource: &v1.ObjectReference{
				ObjectType: "document",
				ObjectId:   "doc1",
			},
			Permission: "viewer",
			Result:     v1.CheckDebugTrace_PERMISSIONSHIP_HAS_PERMISSION,
		},
		{
			Resource: &v1.ObjectReference{
				ObjectType: "document",
				ObjectId:   "doc2",
			},
			Permission: "editor",
			Result:     v1.CheckDebugTrace_PERMISSIONSHIP_NO_PERMISSION,
		},
	}

	html := DisplayBulkCheckTracesHTML(traces)

	// Verify bulk structure
	require.Contains(t, html, "<!DOCTYPE html>")
	require.Contains(t, html, "Check #1")
	require.Contains(t, html, "Check #2")
	require.Contains(t, html, "document:doc1")
	require.Contains(t, html, "document:doc2")
	require.Contains(t, html, "viewer")
	require.Contains(t, html, "editor")

	// Should contain separators between traces
	require.GreaterOrEqual(t, strings.Count(html, "permission-tree"), 2)
}

func TestDisplayBulkCheckTracesHTMLEmpty(t *testing.T) {
	traces := []*v1.CheckDebugTrace{}
	html := DisplayBulkCheckTracesHTML(traces)

	// Should still produce valid HTML structure
	require.Contains(t, html, "<!DOCTYPE html>")
	require.Contains(t, html, "</html>")
}

func TestHTMLEscaping(t *testing.T) {
	trace := &v1.CheckDebugTrace{
		Resource: &v1.ObjectReference{
			ObjectType: "document",
			ObjectId:   "<script>alert('xss')</script>",
		},
		Permission: "viewer",
		Result:     v1.CheckDebugTrace_PERMISSIONSHIP_HAS_PERMISSION,
	}

	html := DisplayCheckTraceHTML(trace, false)

	// Verify XSS protection
	require.NotContains(t, html, "<script>alert('xss')</script>")
	require.Contains(t, html, "&lt;script&gt;")
}

func TestDisplayCheckTraceHTMLWithError(t *testing.T) {
	trace := &v1.CheckDebugTrace{
		Resource: &v1.ObjectReference{
			ObjectType: "document",
			ObjectId:   "report",
		},
		Permission: "viewer",
		Result:     v1.CheckDebugTrace_PERMISSIONSHIP_NO_PERMISSION,
		Duration:   durationpb.New(5000000),
	}

	// Call with hasError=true
	html := DisplayCheckTraceHTML(trace, true)

	// Verify error styling is applied
	require.Contains(t, html, "<!DOCTYPE html>")
	require.Contains(t, html, "document:report")
	require.Contains(t, html, "viewer")

	// When hasError is true, even failed checks should show error indication
	require.Contains(t, html, "no-permission")
}

func TestDisplayCheckTraceHTMLWithCycle(t *testing.T) {
	// Create a synthetic cycle: group:admins -> group:managers -> group:admins
	cycleTrace := &v1.CheckDebugTrace{
		Resource: &v1.ObjectReference{
			ObjectType: "group",
			ObjectId:   "admins",
		},
		Permission:     "member",
		PermissionType: v1.CheckDebugTrace_PERMISSION_TYPE_RELATION,
		Result:         v1.CheckDebugTrace_PERMISSIONSHIP_HAS_PERMISSION,
		Duration:       durationpb.New(10000000),
	}

	// Add sub-problem that creates a cycle
	managers := &v1.CheckDebugTrace{
		Resource: &v1.ObjectReference{
			ObjectType: "group",
			ObjectId:   "managers",
		},
		Permission:     "member",
		PermissionType: v1.CheckDebugTrace_PERMISSION_TYPE_RELATION,
		Result:         v1.CheckDebugTrace_PERMISSIONSHIP_HAS_PERMISSION,
	}

	// Create the cycle: managers references back to admins
	adminsCycle := &v1.CheckDebugTrace{
		Resource: &v1.ObjectReference{
			ObjectType: "group",
			ObjectId:   "admins",
		},
		Permission:     "member",
		PermissionType: v1.CheckDebugTrace_PERMISSION_TYPE_RELATION,
		Result:         v1.CheckDebugTrace_PERMISSIONSHIP_HAS_PERMISSION,
	}

	// Wire up the cycle using the Resolution oneof field
	managers.Resolution = &v1.CheckDebugTrace_SubProblems_{
		SubProblems: &v1.CheckDebugTrace_SubProblems{
			Traces: []*v1.CheckDebugTrace{adminsCycle},
		},
	}
	cycleTrace.Resolution = &v1.CheckDebugTrace_SubProblems_{
		SubProblems: &v1.CheckDebugTrace_SubProblems{
			Traces: []*v1.CheckDebugTrace{managers},
		},
	}

	// Call with hasError=true to trigger cycle detection
	html := DisplayCheckTraceHTML(cycleTrace, true)

	// Verify cycle badge is present
	require.Contains(t, html, `class="badge cycle"`)
	require.Contains(t, html, `>cycle</span>`)

	// Verify cycle icon is rendered in the trace tree (not just in CSS)
	require.Contains(t, html, `<span class="icon cycle">!</span>`)

	// Verify the cycled resources are in the output
	require.Contains(t, html, "group:admins")
	require.Contains(t, html, "group:managers")
}

func TestHTMLRenderOptions(t *testing.T) {
	trace := &v1.CheckDebugTrace{
		Resource: &v1.ObjectReference{
			ObjectType: "document",
			ObjectId:   "doc1",
		},
		Permission: "view",
		Result:     v1.CheckDebugTrace_PERMISSIONSHIP_HAS_PERMISSION,
	}

	renderer := NewHTMLCheckTraceRenderer()
	defer renderer.Release()

	opts := RenderOptions{
		Timestamp:      time.Date(2024, 1, 15, 14, 30, 0, 0, time.UTC),
		Command:        "zed permission check document:doc1 viewer user:alice",
		SpiceDBServer:  "localhost:50051",
		SpiceDBVersion: "v1.28.0",
	}

	html := renderer.RenderWithOptions(trace, false, opts)

	// Verify metadata is present
	require.Contains(t, html, "Generated: 2024-01-15 14:30:00 UTC")
	require.Contains(t, html, "Command:")
	require.Contains(t, html, "zed permission check document:doc1 viewer user:alice")
	require.Contains(t, html, "SpiceDB Server: localhost:50051")
	require.Contains(t, html, "SpiceDB Version: v1.28.0")

	// Verify content is still present
	require.Contains(t, html, "document:doc1")
	require.Contains(t, html, "view")
}

func TestHTMLRenderNilResource(t *testing.T) {
	trace := &v1.CheckDebugTrace{
		Resource:   nil, // Should not panic!
		Permission: "view",
		Result:     v1.CheckDebugTrace_PERMISSIONSHIP_HAS_PERMISSION,
	}

	html := DisplayCheckTraceHTML(trace, false)

	// Should handle gracefully with error message
	require.Contains(t, html, "<!DOCTYPE html>")
	require.Contains(t, html, "malformed trace")
	require.Contains(t, html, "missing resource")
}

func TestHTMLBulkTracesLarge(t *testing.T) {
	// Test large bulk operation doesn't OOM
	traces := make([]CheckTraceWithError, 1000)
	for i := range traces {
		traces[i] = CheckTraceWithError{
			Trace: &v1.CheckDebugTrace{
				Resource: &v1.ObjectReference{
					ObjectType: "document",
					ObjectId:   fmt.Sprintf("doc%d", i),
				},
				Permission: "view",
				Result:     v1.CheckDebugTrace_PERMISSIONSHIP_HAS_PERMISSION,
			},
			HasError: false,
		}
	}

	// Should not panic or OOM
	html := DisplayBulkCheckTracesWithErrorsHTML(traces)

	// Verify structure is valid
	require.Contains(t, html, "<!DOCTYPE html>")
	require.Contains(t, html, "Check #1")
	require.Contains(t, html, "Check #1000")
	require.Contains(t, html, "document:doc0")
	require.Contains(t, html, "document:doc999")
}

func TestHTMLRendererOptionsLeak(t *testing.T) {
	// Test that options don't leak between renders when using pool
	r := NewHTMLCheckTraceRenderer()

	trace1 := &v1.CheckDebugTrace{
		Resource: &v1.ObjectReference{
			ObjectType: "document",
			ObjectId:   "doc1",
		},
		Permission: "view",
		Result:     v1.CheckDebugTrace_PERMISSIONSHIP_HAS_PERMISSION,
	}

	// Render with options
	html1 := r.RenderWithOptions(trace1, false, RenderOptions{
		Command:        "zed permission check document:doc1 viewer user:alice",
		SpiceDBServer:  "localhost:50051",
		SpiceDBVersion: "v1.28.0",
	})
	require.Contains(t, html1, "Command:")
	require.Contains(t, html1, "zed permission check")

	r.Release()

	// Get another renderer (might reuse same instance from pool)
	r2 := NewHTMLCheckTraceRenderer()
	defer r2.Release()

	trace2 := &v1.CheckDebugTrace{
		Resource: &v1.ObjectReference{
			ObjectType: "document",
			ObjectId:   "doc2",
		},
		Permission: "edit",
		Result:     v1.CheckDebugTrace_PERMISSIONSHIP_NO_PERMISSION,
	}

	// Render without options - should not contain previous options
	html2 := r2.Render(trace2, false)
	require.NotContains(t, html2, "zed permission check", "Options leaked from previous render")
	require.NotContains(t, html2, "localhost:50051", "Options leaked from previous render")
	require.Contains(t, html2, "document:doc2")
	require.Contains(t, html2, "edit")
}

func TestHTMLMetadataUsesCSS(t *testing.T) {
	trace := &v1.CheckDebugTrace{
		Resource: &v1.ObjectReference{
			ObjectType: "document",
			ObjectId:   "doc1",
		},
		Permission: "view",
		Result:     v1.CheckDebugTrace_PERMISSIONSHIP_HAS_PERMISSION,
	}

	renderer := NewHTMLCheckTraceRenderer()
	defer renderer.Release()

	opts := RenderOptions{
		Command: "zed permission check",
	}

	html := renderer.RenderWithOptions(trace, false, opts)

	// Verify CSS classes are used instead of inline styles
	require.Contains(t, html, `class="metadata-timestamp"`)
	require.Contains(t, html, `class="metadata-item"`)
	require.Contains(t, html, `class="metadata-code"`)

	// Should NOT contain inline styles
	require.NotContains(t, html, `style="color: #858585; font-size:`)
}

func TestHTMLBulkTracesTimestamp(t *testing.T) {
	// Test that bulk traces have proper timestamp, not zero value
	traces := []CheckTraceWithError{
		{
			Trace: &v1.CheckDebugTrace{
				Resource: &v1.ObjectReference{
					ObjectType: "document",
					ObjectId:   "doc1",
				},
				Permission: "view",
				Result:     v1.CheckDebugTrace_PERMISSIONSHIP_HAS_PERMISSION,
			},
			HasError: false,
		},
		{
			Trace: &v1.CheckDebugTrace{
				Resource: &v1.ObjectReference{
					ObjectType: "document",
					ObjectId:   "doc2",
				},
				Permission: "edit",
				Result:     v1.CheckDebugTrace_PERMISSIONSHIP_NO_PERMISSION,
			},
			HasError: false,
		},
	}

	html := DisplayBulkCheckTracesWithErrorsHTML(traces)

	// Verify timestamp is NOT the zero value
	require.NotContains(t, html, "0001-01-01 00:00:00", "Timestamp should not be zero value")
	require.NotContains(t, html, "0001-01-01", "Timestamp should not be zero value")

	// Verify timestamp is present and reasonable (contains current year)
	require.Contains(t, html, "Generated:")
	currentYear := time.Now().Format("2006")
	require.Contains(t, html, currentYear, "Timestamp should contain current year")

	// Verify both traces are present
	require.Contains(t, html, "document:doc1")
	require.Contains(t, html, "document:doc2")
}

func TestHTMLSubjectWithRelation(t *testing.T) {
	// Test that subject with OptionalRelation is rendered correctly
	trace := &v1.CheckDebugTrace{
		Resource: &v1.ObjectReference{
			ObjectType: "document",
			ObjectId:   "doc1",
		},
		Permission: "view",
		Result:     v1.CheckDebugTrace_PERMISSIONSHIP_HAS_PERMISSION,
		Subject: &v1.SubjectReference{
			Object: &v1.ObjectReference{
				ObjectType: "user",
				ObjectId:   "alice",
			},
			OptionalRelation: "member",
		},
	}

	html := DisplayCheckTraceHTML(trace, false)

	// Verify subject with relation is rendered correctly
	require.Contains(t, html, "user:alice#member")
	require.NotContains(t, html, "user:alice #member", "Should not have space before #")
	require.NotContains(t, html, "user:alice member", "Should use # separator")
}

func TestHTMLSubjectWithoutRelation(t *testing.T) {
	// Test that subject without OptionalRelation doesn't render #
	trace := &v1.CheckDebugTrace{
		Resource: &v1.ObjectReference{
			ObjectType: "document",
			ObjectId:   "doc1",
		},
		Permission: "view",
		Result:     v1.CheckDebugTrace_PERMISSIONSHIP_HAS_PERMISSION,
		Subject: &v1.SubjectReference{
			Object: &v1.ObjectReference{
				ObjectType: "user",
				ObjectId:   "alice",
			},
			OptionalRelation: "",
		},
	}

	html := DisplayCheckTraceHTML(trace, false)

	// Verify subject without relation doesn't have # separator
	require.Contains(t, html, "user:alice")
	require.NotContains(t, html, "user:alice#", "Should not have # when relation is empty")
}

func TestHTMLMissingContextWithNilPartialInfo(t *testing.T) {
	// Test that RESULT_MISSING_SOME_CONTEXT with nil PartialCaveatInfo doesn't panic
	trace := &v1.CheckDebugTrace{
		Resource: &v1.ObjectReference{
			ObjectType: "document",
			ObjectId:   "doc1",
		},
		Permission: "view",
		Result:     v1.CheckDebugTrace_PERMISSIONSHIP_CONDITIONAL_PERMISSION,
		CaveatEvaluationInfo: &v1.CaveatEvalInfo{
			Expression:        "is_admin == true",
			CaveatName:        "admin_check",
			Result:            v1.CaveatEvalInfo_RESULT_MISSING_SOME_CONTEXT,
			PartialCaveatInfo: nil, // ← Nil! Should not panic
		},
	}

	// Should not panic
	html := DisplayCheckTraceHTML(trace, false)

	// Verify it renders gracefully
	require.Contains(t, html, "missing context")
	require.Contains(t, html, "admin_check")
	require.Contains(t, html, "is_admin == true")
}

func TestHTMLMissingContextWithEmptyList(t *testing.T) {
	// Test that RESULT_MISSING_SOME_CONTEXT with empty MissingRequiredContext list
	trace := &v1.CheckDebugTrace{
		Resource: &v1.ObjectReference{
			ObjectType: "document",
			ObjectId:   "doc1",
		},
		Permission: "view",
		Result:     v1.CheckDebugTrace_PERMISSIONSHIP_CONDITIONAL_PERMISSION,
		CaveatEvaluationInfo: &v1.CaveatEvalInfo{
			Expression: "is_admin == true",
			CaveatName: "admin_check",
			Result:     v1.CaveatEvalInfo_RESULT_MISSING_SOME_CONTEXT,
			PartialCaveatInfo: &v1.PartialCaveatInfo{
				MissingRequiredContext: []string{}, // ← Empty list
			},
		},
	}

	html := DisplayCheckTraceHTML(trace, false)

	// Verify it renders gracefully
	require.Contains(t, html, "missing context")
	require.NotContains(t, html, "missing context: ", "Should not show colon when no specific fields")
}

func TestHTMLConditionalPermissionWithNilCaveatInfo(t *testing.T) {
	// Test that CONDITIONAL_PERMISSION with nil CaveatEvaluationInfo renders with default icon
	// This happens with older SpiceDB releases that don't populate CaveatEvaluationInfo
	trace := &v1.CheckDebugTrace{
		Resource: &v1.ObjectReference{
			ObjectType: "document",
			ObjectId:   "doc1",
		},
		Permission:           "view",
		Result:               v1.CheckDebugTrace_PERMISSIONSHIP_CONDITIONAL_PERMISSION,
		CaveatEvaluationInfo: nil, // ← Nil! Older SpiceDB releases
	}

	html := DisplayCheckTraceHTML(trace, false)

	// Verify it renders with default "has permission" icon (not empty)
	require.Contains(t, html, `<span class="icon has-permission">✓</span>`)
	require.NotContains(t, html, `<span class="icon "></span>`, "Icon should not be empty")
	require.Contains(t, html, "document:doc1")
}

func TestHTMLMetadataTruncation(t *testing.T) {
	// Test that very long metadata fields are truncated
	trace := &v1.CheckDebugTrace{
		Resource: &v1.ObjectReference{
			ObjectType: "document",
			ObjectId:   "doc1",
		},
		Permission: "view",
		Result:     v1.CheckDebugTrace_PERMISSIONSHIP_HAS_PERMISSION,
	}

	renderer := NewHTMLCheckTraceRenderer()
	defer renderer.Release()

	// Create very long command (2000 chars)
	longCommand := strings.Repeat("a", 2000)
	opts := RenderOptions{
		Command:        longCommand,
		SpiceDBServer:  strings.Repeat("b", 2000),
		SpiceDBVersion: strings.Repeat("c", 2000),
	}

	html := renderer.RenderWithOptions(trace, false, opts)

	// Verify truncation happened
	require.Contains(t, html, "...")
	require.NotContains(t, html, strings.Repeat("a", 2000), "Long command should be truncated")
	require.NotContains(t, html, strings.Repeat("b", 2000), "Long server should be truncated")
	require.NotContains(t, html, strings.Repeat("c", 2000), "Long version should be truncated")
}

func TestHTMLMetadataUTF8Truncation(t *testing.T) {
	// Test that UTF-8 truncation doesn't panic on multi-byte character boundaries
	trace := &v1.CheckDebugTrace{
		Resource: &v1.ObjectReference{
			ObjectType: "document",
			ObjectId:   "doc1",
		},
		Permission: "view",
		Result:     v1.CheckDebugTrace_PERMISSIONSHIP_HAS_PERMISSION,
	}

	renderer := NewHTMLCheckTraceRenderer()
	defer renderer.Release()

	// Create command with multi-byte UTF-8 characters (Chinese, 3 bytes each)
	// 1500 characters exceeds 1024 rune limit, ensuring truncation
	longCommand := strings.Repeat("你", 1500)
	opts := RenderOptions{
		Command:        longCommand,
		SpiceDBServer:  strings.Repeat("🎉", 1500), // Emoji, 4 bytes each
		SpiceDBVersion: strings.Repeat("א", 1500), // Hebrew, 2 bytes each
	}

	// Should not panic on UTF-8 boundary
	html := renderer.RenderWithOptions(trace, false, opts)

	// Verify truncation happened
	require.Contains(t, html, "...")
	require.NotContains(t, html, strings.Repeat("你", 1500), "Long UTF-8 command should be truncated")
	require.NotContains(t, html, strings.Repeat("🎉", 1500), "Long emoji server should be truncated")
	require.NotContains(t, html, strings.Repeat("א", 1500), "Long Hebrew version should be truncated")

	// Verify HTML is still well-formed (no invalid UTF-8)
	require.Contains(t, html, "<!DOCTYPE html>")
	require.Contains(t, html, "</html>")
}

func TestHTMLBulkRenderWithOptions(t *testing.T) {
	// Test that bulk rendering supports options
	traces := []CheckTraceWithError{
		{
			Trace: &v1.CheckDebugTrace{
				Resource: &v1.ObjectReference{
					ObjectType: "document",
					ObjectId:   "doc1",
				},
				Permission: "view",
				Result:     v1.CheckDebugTrace_PERMISSIONSHIP_HAS_PERMISSION,
			},
			HasError: false,
		},
		{
			Trace: &v1.CheckDebugTrace{
				Resource: &v1.ObjectReference{
					ObjectType: "document",
					ObjectId:   "doc2",
				},
				Permission: "edit",
				Result:     v1.CheckDebugTrace_PERMISSIONSHIP_NO_PERMISSION,
			},
			HasError: false,
		},
	}

	opts := RenderOptions{
		Timestamp:      time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC),
		Command:        "zed permission check-bulk",
		SpiceDBServer:  "grpc.authzed.com:443",
		SpiceDBVersion: "v1.28.0",
	}

	html := DisplayBulkCheckTracesWithErrorsHTMLWithOptions(traces, opts)

	// Verify metadata is present
	require.Contains(t, html, "Generated: 2024-03-15 10:30:00 UTC")
	require.Contains(t, html, "Command:")
	require.Contains(t, html, "zed permission check-bulk")
	require.Contains(t, html, "SpiceDB Server: grpc.authzed.com:443")
	require.Contains(t, html, "SpiceDB Version: v1.28.0")

	// Verify both traces are present
	require.Contains(t, html, "document:doc1")
	require.Contains(t, html, "document:doc2")
	require.Contains(t, html, "Check #1")
	require.Contains(t, html, "Check #2")
}
