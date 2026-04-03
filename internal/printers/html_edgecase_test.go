package printers

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
)

// TestHTMLXSSProtection verifies that malicious inputs are properly escaped
func TestHTMLXSSProtection(t *testing.T) {
	tests := []struct {
		name      string
		trace     *v1.CheckDebugTrace
		malicious []string // strings that should be escaped
	}{
		{
			name: "script tag in object type",
			trace: &v1.CheckDebugTrace{
				Resource: &v1.ObjectReference{
					ObjectType: "<script>alert('xss')</script>",
					ObjectId:   "doc1",
				},
				Permission: "view",
				Result:     v1.CheckDebugTrace_PERMISSIONSHIP_HAS_PERMISSION,
			},
			malicious: []string{"<script>", "alert('xss')"},
		},
		{
			name: "HTML injection in object ID",
			trace: &v1.CheckDebugTrace{
				Resource: &v1.ObjectReference{
					ObjectType: "document",
					ObjectId:   "<img src=x onerror=alert('xss')>",
				},
				Permission: "view",
				Result:     v1.CheckDebugTrace_PERMISSIONSHIP_HAS_PERMISSION,
			},
			malicious: []string{"<img", "onerror="},
		},
		{
			name: "event handler in permission name",
			trace: &v1.CheckDebugTrace{
				Resource: &v1.ObjectReference{
					ObjectType: "document",
					ObjectId:   "doc1",
				},
				Permission: "view' onload='alert(1)",
				Result:     v1.CheckDebugTrace_PERMISSIONSHIP_HAS_PERMISSION,
			},
			malicious: []string{}, // No HTML tags to escape in this case
		},
		{
			name: "iframe injection",
			trace: &v1.CheckDebugTrace{
				Resource: &v1.ObjectReference{
					ObjectType: "document",
					ObjectId:   "doc1",
				},
				Permission: "view",
				Result:     v1.CheckDebugTrace_PERMISSIONSHIP_HAS_PERMISSION,
				Subject: &v1.SubjectReference{
					Object: &v1.ObjectReference{
						ObjectType: "</div><iframe src=javascript:alert(1)>",
						ObjectId:   "user1",
					},
				},
			},
			malicious: []string{"<iframe"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			html := DisplayCheckTraceHTML(tt.trace, false)

			// Verify HTML structure is present
			require.Contains(t, html, "<!DOCTYPE html>")
			require.Contains(t, html, "</html>")

			// Verify that actual XSS attacks are prevented (tags are escaped, not executed)
			// We check that malicious tags from user input are escaped
			require.NotContains(t, html, "<script>alert", "Unescaped <script> tag from user input")
			require.NotContains(t, html, "<img src=x", "Unescaped <img tag from user input")
			require.NotContains(t, html, "<iframe src=javascript", "Unescaped <iframe tag from user input")

			// If there are HTML tags in the input, verify they're escaped
			if len(tt.malicious) > 0 {
				require.Contains(t, html, "&lt;", "Missing HTML entity encoding for <")
				require.Contains(t, html, "&gt;", "Missing HTML entity encoding for >")
			}
		})
	}
}

// TestHTMLUnicodeHandling verifies correct handling of Unicode characters
func TestHTMLUnicodeHandling(t *testing.T) {
	tests := []struct {
		name     string
		trace    *v1.CheckDebugTrace
		expected []string // strings that should appear in output
	}{
		{
			name: "emoji in object type",
			trace: &v1.CheckDebugTrace{
				Resource: &v1.ObjectReference{
					ObjectType: "document📄",
					ObjectId:   "report",
				},
				Permission: "view",
				Result:     v1.CheckDebugTrace_PERMISSIONSHIP_HAS_PERMISSION,
			},
			expected: []string{"document📄"},
		},
		{
			name: "chinese characters",
			trace: &v1.CheckDebugTrace{
				Resource: &v1.ObjectReference{
					ObjectType: "文档",
					ObjectId:   "报告",
				},
				Permission: "查看",
				Result:     v1.CheckDebugTrace_PERMISSIONSHIP_HAS_PERMISSION,
			},
			expected: []string{"文档", "报告", "查看"},
		},
		{
			name: "arabic RTL text",
			trace: &v1.CheckDebugTrace{
				Resource: &v1.ObjectReference{
					ObjectType: "مستند",
					ObjectId:   "تقرير",
				},
				Permission: "عرض",
				Result:     v1.CheckDebugTrace_PERMISSIONSHIP_HAS_PERMISSION,
			},
			expected: []string{"مستند", "تقرير", "عرض"},
		},
		{
			name: "special unicode symbols",
			trace: &v1.CheckDebugTrace{
				Resource: &v1.ObjectReference{
					ObjectType: "document",
					ObjectId:   "test→→→",
				},
				Permission: "view⚡",
				Result:     v1.CheckDebugTrace_PERMISSIONSHIP_HAS_PERMISSION,
			},
			expected: []string{"→", "⚡"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			html := DisplayCheckTraceHTML(tt.trace, false)

			// Verify HTML structure
			require.Contains(t, html, "<!DOCTYPE html>")
			require.Contains(t, html, "charset=\"UTF-8\"")

			// Verify Unicode strings are preserved
			for _, exp := range tt.expected {
				require.Contains(t, html, exp,
					"HTML missing expected Unicode string: %s", exp)
			}
		})
	}
}

// TestHTMLDeepRecursion tests handling of deeply nested permission traces
func TestHTMLDeepRecursion(t *testing.T) {
	// Create a deep chain: doc1 -> group1 -> group2 -> ... -> group10 -> user1
	root := &v1.CheckDebugTrace{
		Resource: &v1.ObjectReference{
			ObjectType: "document",
			ObjectId:   "doc1",
		},
		Permission: "view",
		Result:     v1.CheckDebugTrace_PERMISSIONSHIP_HAS_PERMISSION,
		Duration:   durationpb.New(1000000),
	}

	current := root
	for i := 1; i <= 10; i++ {
		child := &v1.CheckDebugTrace{
			Resource: &v1.ObjectReference{
				ObjectType: "group",
				ObjectId:   strings.Repeat("g", i), // g, gg, ggg, ...
			},
			Permission: "member",
			Result:     v1.CheckDebugTrace_PERMISSIONSHIP_HAS_PERMISSION,
			Duration:   durationpb.New(100000),
		}

		current.Resolution = &v1.CheckDebugTrace_SubProblems_{
			SubProblems: &v1.CheckDebugTrace_SubProblems{
				Traces: []*v1.CheckDebugTrace{child},
			},
		}
		current = child
	}

	// Add leaf subject at the end
	current.Subject = &v1.SubjectReference{
		Object: &v1.ObjectReference{
			ObjectType: "user",
			ObjectId:   "alice",
		},
	}

	html := DisplayCheckTraceHTML(root, false)

	// Verify HTML is valid
	require.Contains(t, html, "<!DOCTYPE html>")
	require.Contains(t, html, "</html>")

	// Verify all levels are present
	require.Contains(t, html, "document:doc1")
	require.Contains(t, html, "group:g")
	require.Contains(t, html, "group:gggggggggg") // 10 g's
	require.Contains(t, html, "user:alice")

	// Verify proper nesting with details elements
	detailsCount := strings.Count(html, "<details")
	require.GreaterOrEqual(t, detailsCount, 10, "Should have at least 10 nested details elements")
}

// TestHTMLMultipleCaveats tests rendering of traces with multiple caveats
func TestHTMLMultipleCaveats(t *testing.T) {
	// Create a trace with multiple sub-problems, each with different caveat results
	trace := &v1.CheckDebugTrace{
		Resource: &v1.ObjectReference{
			ObjectType: "document",
			ObjectId:   "report",
		},
		Permission: "view",
		Result:     v1.CheckDebugTrace_PERMISSIONSHIP_CONDITIONAL_PERMISSION,
		Duration:   durationpb.New(5000000),
	}

	// Create three sub-problems with different caveat states
	subProblems := []*v1.CheckDebugTrace{
		{
			Resource: &v1.ObjectReference{
				ObjectType: "group",
				ObjectId:   "finance",
			},
			Permission: "member",
			Result:     v1.CheckDebugTrace_PERMISSIONSHIP_CONDITIONAL_PERMISSION,
			CaveatEvaluationInfo: &v1.CaveatEvalInfo{
				Expression: "department == \"finance\"",
				CaveatName: "department_check",
				Result:     v1.CaveatEvalInfo_RESULT_TRUE,
				Context: func() *structpb.Struct {
					s, _ := structpb.NewStruct(map[string]any{
						"department": "finance",
					})
					return s
				}(),
			},
		},
		{
			Resource: &v1.ObjectReference{
				ObjectType: "group",
				ObjectId:   "managers",
			},
			Permission: "member",
			Result:     v1.CheckDebugTrace_PERMISSIONSHIP_CONDITIONAL_PERMISSION,
			CaveatEvaluationInfo: &v1.CaveatEvalInfo{
				Expression: "clearance_level >= 3",
				CaveatName: "clearance_check",
				Result:     v1.CaveatEvalInfo_RESULT_MISSING_SOME_CONTEXT,
				PartialCaveatInfo: &v1.PartialCaveatInfo{
					MissingRequiredContext: []string{"clearance_level"},
				},
			},
		},
		{
			Resource: &v1.ObjectReference{
				ObjectType: "group",
				ObjectId:   "executives",
			},
			Permission: "member",
			Result:     v1.CheckDebugTrace_PERMISSIONSHIP_CONDITIONAL_PERMISSION,
			CaveatEvaluationInfo: &v1.CaveatEvalInfo{
				Expression: "title == \"CEO\"",
				CaveatName: "title_check",
				Result:     v1.CaveatEvalInfo_RESULT_FALSE,
				Context: func() *structpb.Struct {
					s, _ := structpb.NewStruct(map[string]any{
						"title": "Manager",
					})
					return s
				}(),
			},
		},
	}

	trace.Resolution = &v1.CheckDebugTrace_SubProblems_{
		SubProblems: &v1.CheckDebugTrace_SubProblems{
			Traces: subProblems,
		},
	}

	html := DisplayCheckTraceHTML(trace, false)

	// Verify all caveat expressions are present
	require.Contains(t, html, "department == &#34;finance&#34;")
	require.Contains(t, html, "clearance_level &gt;= 3")
	require.Contains(t, html, "title == &#34;CEO&#34;")

	// Verify all caveat names are present
	require.Contains(t, html, "department_check")
	require.Contains(t, html, "clearance_check")
	require.Contains(t, html, "title_check")

	// Verify different caveat results are shown
	require.Contains(t, html, "department")      // Context for RESULT_TRUE
	require.Contains(t, html, "missing context") // For RESULT_MISSING_SOME_CONTEXT
	require.Contains(t, html, "Manager")         // Context for RESULT_FALSE

	// Count caveat nodes (should have 3) - count the divs, not CSS class definitions
	caveatCount := strings.Count(html, `<div class="caveat-node">`)
	require.Equal(t, 3, caveatCount, "Should have exactly 3 caveat node divs")
}

// TestHTMLEmptyAndNilFields tests handling of empty and nil fields
func TestHTMLEmptyAndNilFields(t *testing.T) {
	tests := []struct {
		name  string
		trace *v1.CheckDebugTrace
	}{
		{
			name: "minimal trace with no optional fields",
			trace: &v1.CheckDebugTrace{
				Resource: &v1.ObjectReference{
					ObjectType: "document",
					ObjectId:   "doc1",
				},
				Permission: "view",
				Result:     v1.CheckDebugTrace_PERMISSIONSHIP_HAS_PERMISSION,
			},
		},
		{
			name: "empty object IDs",
			trace: &v1.CheckDebugTrace{
				Resource: &v1.ObjectReference{
					ObjectType: "document",
					ObjectId:   "",
				},
				Permission: "",
				Result:     v1.CheckDebugTrace_PERMISSIONSHIP_UNSPECIFIED,
			},
		},
		{
			name: "trace with nil sub-problems",
			trace: &v1.CheckDebugTrace{
				Resource: &v1.ObjectReference{
					ObjectType: "document",
					ObjectId:   "doc1",
				},
				Permission: "view",
				Result:     v1.CheckDebugTrace_PERMISSIONSHIP_HAS_PERMISSION,
				Resolution: nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should not panic
			html := DisplayCheckTraceHTML(tt.trace, false)

			// Basic structure should be present
			require.Contains(t, html, "<!DOCTYPE html>")
			require.Contains(t, html, "</html>")
			require.Contains(t, html, "SpiceDB Permission Check Trace")
		})
	}
}

// TestHTMLLongStrings tests handling of very long strings
func TestHTMLLongStrings(t *testing.T) {
	longString := strings.Repeat("a", 10000)

	trace := &v1.CheckDebugTrace{
		Resource: &v1.ObjectReference{
			ObjectType: "document",
			ObjectId:   longString,
		},
		Permission: "view",
		Result:     v1.CheckDebugTrace_PERMISSIONSHIP_HAS_PERMISSION,
	}

	html := DisplayCheckTraceHTML(trace, false)

	// Verify HTML structure is maintained
	require.Contains(t, html, "<!DOCTYPE html>")
	require.Contains(t, html, "</html>")

	// Verify long string is present (may be escaped)
	require.Contains(t, html, strings.Repeat("a", 100), "Long string should be present in HTML")

	// Verify HTML is well-formed (balanced tags)
	require.Equal(t, strings.Count(html, "<details"), strings.Count(html, "</details>"))
	require.Equal(t, strings.Count(html, "<div"), strings.Count(html, "</div>"))
}
