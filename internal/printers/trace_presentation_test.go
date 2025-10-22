package printers

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/durationpb"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
)

func TestGetTracePresentation_HasPermission(t *testing.T) {
	trace := &v1.CheckDebugTrace{
		Result: v1.CheckDebugTrace_PERMISSIONSHIP_HAS_PERMISSION,
		Resource: &v1.ObjectReference{
			ObjectType: "document",
			ObjectId:   "doc1",
		},
		Permission: "view",
		Duration:   durationpb.New(0),
	}

	pres := GetTracePresentation(trace, false)

	require.Equal(t, "✓", pres.Icon)
	require.Equal(t, "has-permission", pres.IconClass)
	require.False(t, pres.ResourceFaint)
	require.False(t, pres.PermissionFaint)
	require.Empty(t, pres.CacheBadge)
	require.False(t, pres.IsCycle)
}

func TestGetTracePresentation_NoPermission(t *testing.T) {
	trace := &v1.CheckDebugTrace{
		Result: v1.CheckDebugTrace_PERMISSIONSHIP_NO_PERMISSION,
		Resource: &v1.ObjectReference{
			ObjectType: "document",
			ObjectId:   "doc1",
		},
		Permission: "view",
		Duration:   durationpb.New(0),
	}

	pres := GetTracePresentation(trace, false)

	require.Equal(t, "⨉", pres.Icon)
	require.Equal(t, "no-permission", pres.IconClass)
	require.True(t, pres.ResourceFaint, "Resource should be faint for no permission")
	require.True(t, pres.PermissionFaint, "Permission should be faint for no permission")
	require.Empty(t, pres.CacheBadge)
	require.False(t, pres.IsCycle)
}

func TestGetTracePresentation_ConditionalMissingContext(t *testing.T) {
	trace := &v1.CheckDebugTrace{
		Result: v1.CheckDebugTrace_PERMISSIONSHIP_CONDITIONAL_PERMISSION,
		Resource: &v1.ObjectReference{
			ObjectType: "document",
			ObjectId:   "doc1",
		},
		Permission: "view",
		Duration:   durationpb.New(0),
		CaveatEvaluationInfo: &v1.CaveatEvalInfo{
			Result:     v1.CaveatEvalInfo_RESULT_MISSING_SOME_CONTEXT,
			Expression: "department == \"finance\"",
		},
	}

	pres := GetTracePresentation(trace, false)

	require.Equal(t, "?", pres.Icon)
	require.Equal(t, "conditional", pres.IconClass)
	require.True(t, pres.ResourceFaint, "Resource should be faint for conditional")
	require.True(t, pres.PermissionFaint, "Permission should be faint for conditional")
	require.Empty(t, pres.CacheBadge)
	require.False(t, pres.IsCycle)
}

func TestGetTracePresentation_ConditionalFalse(t *testing.T) {
	trace := &v1.CheckDebugTrace{
		Result: v1.CheckDebugTrace_PERMISSIONSHIP_CONDITIONAL_PERMISSION,
		Resource: &v1.ObjectReference{
			ObjectType: "document",
			ObjectId:   "doc1",
		},
		Permission: "view",
		Duration:   durationpb.New(0),
		CaveatEvaluationInfo: &v1.CaveatEvalInfo{
			Result:     v1.CaveatEvalInfo_RESULT_FALSE,
			Expression: "department == \"finance\"",
		},
	}

	pres := GetTracePresentation(trace, false)

	require.Equal(t, "⨉", pres.Icon, "Conditional false should show no permission icon")
	require.Equal(t, "no-permission", pres.IconClass)
	require.True(t, pres.ResourceFaint)
	require.True(t, pres.PermissionFaint)
	require.Empty(t, pres.CacheBadge)
	require.False(t, pres.IsCycle)
}

func TestGetTracePresentation_ConditionalTrue(t *testing.T) {
	trace := &v1.CheckDebugTrace{
		Result: v1.CheckDebugTrace_PERMISSIONSHIP_CONDITIONAL_PERMISSION,
		Resource: &v1.ObjectReference{
			ObjectType: "document",
			ObjectId:   "doc1",
		},
		Permission: "view",
		Duration:   durationpb.New(0),
		CaveatEvaluationInfo: &v1.CaveatEvalInfo{
			Result:     v1.CaveatEvalInfo_RESULT_TRUE,
			Expression: "department == \"finance\"",
		},
	}

	pres := GetTracePresentation(trace, false)

	require.Equal(t, "✓", pres.Icon, "Conditional true should show has permission icon")
	require.Equal(t, "has-permission", pres.IconClass)
	require.False(t, pres.ResourceFaint)
	require.False(t, pres.PermissionFaint)
	require.Empty(t, pres.CacheBadge)
	require.False(t, pres.IsCycle)
}

func TestGetTracePresentation_ConditionalNilCaveatInfo(t *testing.T) {
	// Test for older SpiceDB releases that don't populate CaveatEvaluationInfo
	trace := &v1.CheckDebugTrace{
		Result: v1.CheckDebugTrace_PERMISSIONSHIP_CONDITIONAL_PERMISSION,
		Resource: &v1.ObjectReference{
			ObjectType: "document",
			ObjectId:   "doc1",
		},
		Permission:           "view",
		Duration:             durationpb.New(0),
		CaveatEvaluationInfo: nil, // ← Nil! Should default to "has permission" icon
	}

	pres := GetTracePresentation(trace, false)

	// Verify it defaults to "has permission" icon (not empty)
	require.Equal(t, "✓", pres.Icon, "Nil CaveatEvaluationInfo should default to has permission icon")
	require.Equal(t, "has-permission", pres.IconClass, "Nil CaveatEvaluationInfo should default to has-permission class")
	require.False(t, pres.ResourceFaint)
	require.False(t, pres.PermissionFaint)
	require.Empty(t, pres.CacheBadge)
	require.False(t, pres.IsCycle)
}

func TestGetTracePresentation_Unspecified(t *testing.T) {
	trace := &v1.CheckDebugTrace{
		Result: v1.CheckDebugTrace_PERMISSIONSHIP_UNSPECIFIED,
		Resource: &v1.ObjectReference{
			ObjectType: "document",
			ObjectId:   "doc1",
		},
		Permission: "view",
		Duration:   durationpb.New(0),
	}

	pres := GetTracePresentation(trace, false)

	require.Equal(t, "∵", pres.Icon)
	require.Equal(t, "unspecified", pres.IconClass)
	require.False(t, pres.ResourceFaint)
	require.False(t, pres.PermissionFaint)
	require.Empty(t, pres.CacheBadge)
	require.False(t, pres.IsCycle)
}

func TestGetTracePresentation_CachedSpiceDB(t *testing.T) {
	trace := &v1.CheckDebugTrace{
		Result: v1.CheckDebugTrace_PERMISSIONSHIP_HAS_PERMISSION,
		Resource: &v1.ObjectReference{
			ObjectType: "document",
			ObjectId:   "doc1",
		},
		Permission: "view",
		Duration:   durationpb.New(0),
		Resolution: &v1.CheckDebugTrace_WasCachedResult{
			WasCachedResult: true,
		},
		Source: "spicedb:dispatch",
	}

	pres := GetTracePresentation(trace, false)

	require.Equal(t, "✓", pres.Icon)
	require.Equal(t, "has-permission", pres.IconClass)
	require.Equal(t, "cached by spicedb", pres.CacheBadge)
	require.False(t, pres.IsCycle)
}

func TestGetTracePresentation_CachedMaterialize(t *testing.T) {
	trace := &v1.CheckDebugTrace{
		Result: v1.CheckDebugTrace_PERMISSIONSHIP_HAS_PERMISSION,
		Resource: &v1.ObjectReference{
			ObjectType: "document",
			ObjectId:   "doc1",
		},
		Permission: "view",
		Duration:   durationpb.New(0),
		Resolution: &v1.CheckDebugTrace_WasCachedResult{
			WasCachedResult: true,
		},
		Source: "materialize:table",
	}

	pres := GetTracePresentation(trace, false)

	require.Equal(t, "✓", pres.Icon)
	require.Equal(t, "has-permission", pres.IconClass)
	require.Equal(t, "cached by materialize", pres.CacheBadge)
	require.False(t, pres.IsCycle)
}

func TestGetTracePresentation_CachedUnknownSource(t *testing.T) {
	trace := &v1.CheckDebugTrace{
		Result: v1.CheckDebugTrace_PERMISSIONSHIP_HAS_PERMISSION,
		Resource: &v1.ObjectReference{
			ObjectType: "document",
			ObjectId:   "doc1",
		},
		Permission: "view",
		Duration:   durationpb.New(0),
		Resolution: &v1.CheckDebugTrace_WasCachedResult{
			WasCachedResult: true,
		},
		Source: "custom:backend",
	}

	pres := GetTracePresentation(trace, false)

	require.Equal(t, "✓", pres.Icon)
	require.Equal(t, "has-permission", pres.IconClass)
	require.Equal(t, "cached by custom", pres.CacheBadge)
	require.False(t, pres.IsCycle)
}

func TestGetTracePresentation_CachedEmptySource(t *testing.T) {
	trace := &v1.CheckDebugTrace{
		Result: v1.CheckDebugTrace_PERMISSIONSHIP_HAS_PERMISSION,
		Resource: &v1.ObjectReference{
			ObjectType: "document",
			ObjectId:   "doc1",
		},
		Permission: "view",
		Duration:   durationpb.New(0),
		Resolution: &v1.CheckDebugTrace_WasCachedResult{
			WasCachedResult: true,
		},
		Source: "",
	}

	pres := GetTracePresentation(trace, false)

	require.Equal(t, "✓", pres.Icon)
	require.Equal(t, "has-permission", pres.IconClass)
	require.Equal(t, "cached", pres.CacheBadge)
	require.False(t, pres.IsCycle)
}

func TestGetTracePresentation_Cycle(t *testing.T) {
	// Create a cycle: group:admins#member -> group:managers#member -> group:admins#member
	// The root trace
	cycleTrace := &v1.CheckDebugTrace{
		Resource: &v1.ObjectReference{
			ObjectType: "group",
			ObjectId:   "admins",
		},
		Permission:     "member",
		PermissionType: v1.CheckDebugTrace_PERMISSION_TYPE_RELATION,
		Result:         v1.CheckDebugTrace_PERMISSIONSHIP_HAS_PERMISSION,
		Duration:       durationpb.New(0),
	}

	// Intermediate trace
	managers := &v1.CheckDebugTrace{
		Resource: &v1.ObjectReference{
			ObjectType: "group",
			ObjectId:   "managers",
		},
		Permission:     "member",
		PermissionType: v1.CheckDebugTrace_PERMISSION_TYPE_RELATION,
		Result:         v1.CheckDebugTrace_PERMISSIONSHIP_HAS_PERMISSION,
		Duration:       durationpb.New(0),
	}

	// Cycle back to admins (with empty subproblems to satisfy isPartOfCycle check)
	adminsCycle := &v1.CheckDebugTrace{
		Resource: &v1.ObjectReference{
			ObjectType: "group",
			ObjectId:   "admins",
		},
		Permission:     "member",
		PermissionType: v1.CheckDebugTrace_PERMISSION_TYPE_RELATION,
		Result:         v1.CheckDebugTrace_PERMISSIONSHIP_HAS_PERMISSION,
		Duration:       durationpb.New(0),
		Resolution: &v1.CheckDebugTrace_SubProblems_{
			SubProblems: &v1.CheckDebugTrace_SubProblems{
				Traces: []*v1.CheckDebugTrace{},
			},
		},
	}

	// Wire up the cycle
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

	pres := GetTracePresentation(cycleTrace, true)

	require.Equal(t, "!", pres.Icon, "Cycle should show ! icon")
	require.Equal(t, "cycle", pres.IconClass)
	require.False(t, pres.ResourceFaint, "Cycles should not be faint")
	require.False(t, pres.PermissionFaint, "Cycles should not be faint")
	require.True(t, pres.IsCycle)
}

func TestGetTracePresentation_CycleOverridesCache(t *testing.T) {
	// Test that cycle detection overrides other states when hasError=true
	// Create a simple cycle: doc1#view -> doc1#view
	root := &v1.CheckDebugTrace{
		Result: v1.CheckDebugTrace_PERMISSIONSHIP_HAS_PERMISSION,
		Resource: &v1.ObjectReference{
			ObjectType: "document",
			ObjectId:   "doc1",
		},
		Permission: "view",
		Duration:   durationpb.New(0),
		Source:     "spicedb:dispatch",
	}

	// Create cycle by referencing the same resource+permission (with empty subproblems)
	child := &v1.CheckDebugTrace{
		Result: v1.CheckDebugTrace_PERMISSIONSHIP_HAS_PERMISSION,
		Resource: &v1.ObjectReference{
			ObjectType: "document",
			ObjectId:   "doc1",
		},
		Permission: "view",
		Duration:   durationpb.New(0),
		Resolution: &v1.CheckDebugTrace_SubProblems_{
			SubProblems: &v1.CheckDebugTrace_SubProblems{
				Traces: []*v1.CheckDebugTrace{},
			},
		},
	}

	root.Resolution = &v1.CheckDebugTrace_SubProblems_{
		SubProblems: &v1.CheckDebugTrace_SubProblems{
			Traces: []*v1.CheckDebugTrace{child},
		},
	}

	pres := GetTracePresentation(root, true)

	require.Equal(t, "!", pres.Icon, "Cycle should override has permission icon")
	require.Equal(t, "cycle", pres.IconClass)
	require.True(t, pres.IsCycle)
	require.False(t, pres.ResourceFaint, "Cycle should remove faint styling")
	require.False(t, pres.PermissionFaint, "Cycle should remove faint styling")
}

func TestGetCacheBadge(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		expected string
	}{
		{
			name:     "spicedb source",
			source:   "spicedb:dispatch",
			expected: "cached by spicedb",
		},
		{
			name:     "materialize source",
			source:   "materialize:table",
			expected: "cached by materialize",
		},
		{
			name:     "custom source",
			source:   "redis:key",
			expected: "cached by redis",
		},
		{
			name:     "empty source",
			source:   "",
			expected: "cached",
		},
		{
			name:     "source without colon",
			source:   "spicedb",
			expected: "cached by spicedb",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trace := &v1.CheckDebugTrace{
				Source: tt.source,
			}
			badge := getCacheBadge(trace)
			require.Equal(t, tt.expected, badge)
		})
	}
}

func TestGetCacheBadgeClass(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		expected string
	}{
		{
			name:     "spicedb source",
			source:   "spicedb:dispatch",
			expected: "cached-spicedb",
		},
		{
			name:     "materialize source",
			source:   "materialize:table",
			expected: "cached-materialize",
		},
		{
			name:     "custom source",
			source:   "redis:key",
			expected: "cached",
		},
		{
			name:     "empty source",
			source:   "",
			expected: "cached",
		},
		{
			name:     "source without colon",
			source:   "spicedb",
			expected: "cached-spicedb",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trace := &v1.CheckDebugTrace{
				Source: tt.source,
			}
			class := getCacheBadgeClass(trace)
			require.Equal(t, tt.expected, class)
		})
	}
}
