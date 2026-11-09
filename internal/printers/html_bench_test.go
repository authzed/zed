package printers

import (
	"testing"

	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
)

// createComplexTrace generates a test trace with the specified depth
func createComplexTrace(depth int) *v1.CheckDebugTrace {
	trace := &v1.CheckDebugTrace{
		Resource: &v1.ObjectReference{
			ObjectType: "document",
			ObjectId:   "doc1",
		},
		Permission: "view",
		Result:     v1.CheckDebugTrace_PERMISSIONSHIP_HAS_PERMISSION,
		Duration:   durationpb.New(1000000),
		Resolution: &v1.CheckDebugTrace_WasCachedResult{
			WasCachedResult: true,
		},
		Source: "spicedb:dispatch",
	}

	if depth > 0 {
		// Add sub-problems
		subTrace1 := &v1.CheckDebugTrace{
			Resource: &v1.ObjectReference{
				ObjectType: "group",
				ObjectId:   "group1",
			},
			Permission: "member",
			Result:     v1.CheckDebugTrace_PERMISSIONSHIP_HAS_PERMISSION,
			Duration:   durationpb.New(500000),
		}

		subTrace2 := &v1.CheckDebugTrace{
			Resource: &v1.ObjectReference{
				ObjectType: "role",
				ObjectId:   "role1",
			},
			Permission: "assigned",
			Result:     v1.CheckDebugTrace_PERMISSIONSHIP_CONDITIONAL_PERMISSION,
			Duration:   durationpb.New(300000),
			CaveatEvaluationInfo: &v1.CaveatEvalInfo{
				Expression: "department == \"engineering\"",
				CaveatName: "department_check",
				Result:     v1.CaveatEvalInfo_RESULT_TRUE,
				Context: func() *structpb.Struct {
					s, _ := structpb.NewStruct(map[string]any{
						"department": "engineering",
					})
					return s
				}(),
			},
		}

		// Recursively create deeper traces
		if depth > 1 {
			subTrace1.Resolution = &v1.CheckDebugTrace_SubProblems_{
				SubProblems: &v1.CheckDebugTrace_SubProblems{
					Traces: []*v1.CheckDebugTrace{createComplexTrace(depth - 1)},
				},
			}
		} else {
			// Leaf node with subject
			subTrace1.Subject = &v1.SubjectReference{
				Object: &v1.ObjectReference{
					ObjectType: "user",
					ObjectId:   "alice",
				},
			}
		}

		trace.Resolution = &v1.CheckDebugTrace_SubProblems_{
			SubProblems: &v1.CheckDebugTrace_SubProblems{
				Traces: []*v1.CheckDebugTrace{subTrace1, subTrace2},
			},
		}
	} else {
		// Leaf node with subject
		trace.Subject = &v1.SubjectReference{
			Object: &v1.ObjectReference{
				ObjectType: "user",
				ObjectId:   "alice",
			},
		}
	}

	return trace
}

// BenchmarkRenderSingleTrace benchmarks rendering a single complex trace
func BenchmarkRenderSingleTrace(b *testing.B) {
	trace := createComplexTrace(10) // 10 levels deep
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = DisplayCheckTraceHTML(trace, false)
	}
}

// BenchmarkRenderSingleTraceShallow benchmarks rendering a shallow trace
func BenchmarkRenderSingleTraceShallow(b *testing.B) {
	trace := createComplexTrace(3) // 3 levels deep
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = DisplayCheckTraceHTML(trace, false)
	}
}

// BenchmarkRenderBulkTraces benchmarks rendering multiple traces in bulk
func BenchmarkRenderBulkTraces(b *testing.B) {
	traces := make([]CheckTraceWithError, 100)
	for i := range traces {
		traces[i] = CheckTraceWithError{
			Trace:    createComplexTrace(5),
			HasError: false,
		}
	}
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = DisplayBulkCheckTracesWithErrorsHTML(traces)
	}
}

// BenchmarkRenderBulkTracesSmall benchmarks rendering a small bulk operation
func BenchmarkRenderBulkTracesSmall(b *testing.B) {
	traces := make([]CheckTraceWithError, 10)
	for i := range traces {
		traces[i] = CheckTraceWithError{
			Trace:    createComplexTrace(5),
			HasError: false,
		}
	}
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = DisplayBulkCheckTracesWithErrorsHTML(traces)
	}
}

// BenchmarkRenderWithCaveats benchmarks rendering traces with multiple caveats
func BenchmarkRenderWithCaveats(b *testing.B) {
	trace := &v1.CheckDebugTrace{
		Resource: &v1.ObjectReference{
			ObjectType: "document",
			ObjectId:   "report",
		},
		Permission: "view",
		Result:     v1.CheckDebugTrace_PERMISSIONSHIP_CONDITIONAL_PERMISSION,
		Duration:   durationpb.New(5000000),
	}

	// Create multiple sub-problems with different caveat states
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

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = DisplayCheckTraceHTML(trace, false)
	}
}

// BenchmarkRendererPoolReuse benchmarks the pool reuse efficiency
func BenchmarkRendererPoolReuse(b *testing.B) {
	trace := createComplexTrace(5)
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		renderer := NewHTMLCheckTraceRenderer()
		_ = renderer.Render(trace, false)
		renderer.Release()
	}
}
