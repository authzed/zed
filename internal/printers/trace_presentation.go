package printers

import (
	"strings"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
)

// TracePresentation contains the visual presentation state for a check trace.
// This struct provides a single source of truth for how traces should be displayed
// across different output formats (terminal, HTML, etc.).
//
// The presentation logic centralizes icon selection, styling, and badge generation
// to eliminate duplication between the terminal (debug.go) and HTML (html.go) renderers.
//
// Example usage:
//
//	pres := GetTracePresentation(checkTrace, hasError)
//	// For HTML: use pres.Icon and pres.IconClass
//	html := fmt.Sprintf(`<span class="icon %s">%s</span>`, pres.IconClass, pres.Icon)
//	// For terminal: map pres.IconClass to terminal colors
//	terminalIcon := mapIconToColor(pres.Icon, pres.IconClass)
type TracePresentation struct {
	// Icon is the visual indicator for the check result:
	//   "✓" = has permission
	//   "⨉" = no permission
	//   "?" = conditional/missing context
	//   "!" = cycle detected
	//   "∵" = unspecified/unknown
	Icon string

	// IconClass is the CSS class for HTML rendering:
	//   "has-permission", "no-permission", "conditional", "cycle", "unspecified"
	IconClass string

	// ResourceFaint indicates if the resource should be displayed with reduced emphasis.
	// Set to true for NO_PERMISSION and CONDITIONAL results to de-emphasize failed checks.
	ResourceFaint bool

	// PermissionFaint indicates if the permission should be displayed with reduced emphasis.
	// Set to true for NO_PERMISSION and CONDITIONAL results to de-emphasize failed checks.
	PermissionFaint bool

	// CacheBadge contains the cache source information if the result was cached.
	// Examples: "cached by spicedb", "cached by materialize", "cached"
	CacheBadge string

	// IsCycle indicates if this check is part of a cycle.
	// When true, Icon will be "!" and IconClass will be "cycle".
	IsCycle bool
}

// cacheSourceInfo maps cache source prefixes to their display labels and CSS classes
type cacheSourceInfo struct {
	label string
	class string
}

var cacheSourceLabels = map[string]cacheSourceInfo{
	"spicedb":     {label: "cached by spicedb", class: "cached-spicedb"},
	"materialize": {label: "cached by materialize", class: "cached-materialize"},
}

// GetTracePresentation determines the visual presentation for a check trace.
// This centralizes the logic for icons, styling, and badges across all output formats.
//
// The function analyzes the check result, caveat evaluation, cache status, and cycle detection
// to produce a consistent presentation state that can be consumed by both terminal and HTML renderers.
//
// Parameters:
//   - checkTrace: The debug trace to analyze
//   - hasError: Indicates if the overall check failed with a gRPC error (e.g., cycle detected).
//     This is NOT the same as PERMISSIONSHIP_NO_PERMISSION - it indicates a gRPC-level failure.
//
// Returns:
//   - TracePresentation: A struct containing icon, styling, and badge information
//
// Icon Selection Logic:
//   - HAS_PERMISSION: "✓" (green in terminal, has-permission class in HTML)
//   - NO_PERMISSION: "⨉" (red in terminal, no-permission class in HTML, with faint styling)
//   - CONDITIONAL with RESULT_FALSE: "⨉" (treated as no permission)
//   - CONDITIONAL with RESULT_MISSING_SOME_CONTEXT: "?" (magenta in terminal, conditional class in HTML)
//   - CONDITIONAL with RESULT_TRUE: "✓" (treated as has permission)
//   - UNSPECIFIED: "∵" (yellow in terminal, unspecified class in HTML)
//   - Cycle detected: "!" (orange in terminal, cycle class in HTML, overrides other icons)
//
// Example:
//
//	checkTrace := &v1.CheckDebugTrace{
//	    Result: v1.CheckDebugTrace_PERMISSIONSHIP_HAS_PERMISSION,
//	    Resource: &v1.ObjectReference{ObjectType: "document", ObjectId: "doc1"},
//	    Permission: "view",
//	    WasCachedResult: true,
//	    Source: "spicedb:dispatch",
//	}
//	pres := GetTracePresentation(checkTrace, false)
//	// pres.Icon == "✓"
//	// pres.IconClass == "has-permission"
//	// pres.CacheBadge == "cached by spicedb"
func GetTracePresentation(checkTrace *v1.CheckDebugTrace, hasError bool) TracePresentation {
	// Default to "has permission" icon (fallback for older SpiceDB releases)
	pres := TracePresentation{
		Icon:      "✓",
		IconClass: "has-permission",
	}

	// Determine icon and styling based on check result
	switch checkTrace.Result {
	case v1.CheckDebugTrace_PERMISSIONSHIP_HAS_PERMISSION:
		pres.Icon = "✓"
		pres.IconClass = "has-permission"

	case v1.CheckDebugTrace_PERMISSIONSHIP_NO_PERMISSION:
		pres.Icon = "⨉"
		pres.IconClass = "no-permission"
		pres.ResourceFaint = true
		pres.PermissionFaint = true

	case v1.CheckDebugTrace_PERMISSIONSHIP_CONDITIONAL_PERMISSION:
		// For conditional permissions, check the caveat evaluation result
		// If CaveatEvaluationInfo is nil (older SpiceDB releases), default icon is already set above
		if checkTrace.CaveatEvaluationInfo != nil {
			switch checkTrace.CaveatEvaluationInfo.Result {
			case v1.CaveatEvalInfo_RESULT_FALSE:
				pres.Icon = "⨉"
				pres.IconClass = "no-permission"
				pres.ResourceFaint = true
				pres.PermissionFaint = true
			case v1.CaveatEvalInfo_RESULT_MISSING_SOME_CONTEXT:
				pres.Icon = "?"
				pres.IconClass = "conditional"
				pres.ResourceFaint = true
				pres.PermissionFaint = true
			default:
				// RESULT_TRUE or unspecified - treat as has permission
				pres.Icon = "✓"
				pres.IconClass = "has-permission"
			}
		}

	case v1.CheckDebugTrace_PERMISSIONSHIP_UNSPECIFIED:
		pres.Icon = "∵"
		pres.IconClass = "unspecified"

	default:
		pres.Icon = "∵"
		pres.IconClass = "unspecified"
	}

	// Determine cache badge
	if checkTrace.GetWasCachedResult() {
		pres.CacheBadge = getCacheBadge(checkTrace)
	}

	// Check for cycles (overrides icon if present)
	if hasError && isPartOfCycle(checkTrace, map[string]struct{}{}) {
		pres.Icon = "!"
		pres.IconClass = "cycle"
		pres.IsCycle = true
		// Cycles are shown prominently, so remove faint styling
		pres.ResourceFaint = false
		pres.PermissionFaint = false
	}

	return pres
}

// getCacheBadge returns the cache badge string for a cached trace.
// It extracts the cache source from the Source field and maps it to a human-readable label.
//
// The Source field format is typically "kind:details" (e.g., "spicedb:dispatch", "materialize:table").
// This function extracts the kind prefix and returns an appropriate label.
//
// Returns:
//   - "cached by spicedb" for source="spicedb:*"
//   - "cached by materialize" for source="materialize:*"
//   - "cached by <kind>" for other known sources
//   - "cached" for empty or unparseable sources
func getCacheBadge(checkTrace *v1.CheckDebugTrace) string {
	source := checkTrace.Source
	if source == "" {
		return "cached"
	}

	// Extract source kind from "kind:details" format
	parts := strings.Split(source, ":")
	if len(parts) == 0 {
		return "cached"
	}

	sourceKind := parts[0]
	if info, exists := cacheSourceLabels[sourceKind]; exists {
		return info.label
	}

	return "cached by " + sourceKind
}

// getCacheBadgeClass returns the CSS class for a cache badge used in HTML rendering.
// It extracts the cache source from the Source field and maps it to a CSS class.
//
// The Source field format is typically "kind:details" (e.g., "spicedb:dispatch", "materialize:table").
// This function extracts the kind prefix and returns an appropriate CSS class for styling.
//
// Returns:
//   - "cached-spicedb" for source="spicedb:*"
//   - "cached-materialize" for source="materialize:*"
//   - "cached" for empty, unparseable, or unknown sources
func getCacheBadgeClass(checkTrace *v1.CheckDebugTrace) string {
	source := checkTrace.Source
	if source == "" {
		return "cached"
	}

	parts := strings.Split(source, ":")
	if len(parts) == 0 {
		return "cached"
	}

	sourceKind := parts[0]
	if info, exists := cacheSourceLabels[sourceKind]; exists {
		return info.class
	}

	return "cached"
}
