package printers

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"html"
	"strings"
	"sync"
	"time"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
)

//go:embed trace.css
var htmlStyles string

// Note: The CSS file is embedded as-is for readability and maintainability.
// If binary size becomes a concern, consider minifying trace.css at build time:
//   - Use a tool like esbuild, csso, or clean-css
//   - Add a build step: //go:generate css-minify trace.css -o trace.min.css
//   - Update embed directive to: //go:embed trace.min.css
// Current unminified size: ~8KB, minified would be ~5KB (37% savings)

const (
	// traceCapacityHint is the capacity hint per trace (typical trace is 4-16KB)
	// Used for both single renders and per-trace in bulk operations
	traceCapacityHint = 8192

	// maxBulkBuilderCapacity caps bulk pre-allocation to prevent OOM on very large operations
	maxBulkBuilderCapacity = 1024 * 1024 // 1MB

	// cycleMapInitialCapacity is the initial size for the cycle detection map
	cycleMapInitialCapacity = 16

	// nodeContentBufferSize is the buffer size for rendering individual node content (icon + resource + permission + badges + timing)
	// Typical node content is ~100-200 bytes
	nodeContentBufferSize = 256

	// maxMetadataLength limits the length of metadata fields (Command, Server, Version) to prevent DoS
	maxMetadataLength = 1024

	// maxTraceDepth limits recursion depth to prevent stack overflow on malformed traces
	maxTraceDepth = 100
)

var (
	// htmlHeaderPrefix is pre-formatted at init with embedded CSS
	htmlHeaderPrefix string

	// htmlHeaderSuffix closes the header div after timestamp
	htmlHeaderSuffix = `    </div>
`
)

func init() {
	// Pre-format header template with embedded CSS for efficiency
	htmlHeaderPrefix = fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <meta name="generator" content="zed">
    <title>SpiceDB Permission Check Trace</title>
    <style>%s</style>
</head>
<body>
    <div class="header">
        <h1>SpiceDB Permission Check Trace</h1>
        <p>Interactive visualization of permission resolution • Click to expand/collapse</p>
`, htmlStyles)
}

// HTML template constants for the permission trace visualization
const (
	htmlFooter = `
    <div class="legend">
        <h3>Legend</h3>
        <div class="legend-grid">
            <div class="legend-item">
                <span class="icon has-permission">✓</span>
                <span>Has Permission</span>
            </div>
            <div class="legend-item">
                <span class="icon no-permission">⨉</span>
                <span>No Permission</span>
            </div>
            <div class="legend-item">
                <span class="icon conditional">?</span>
                <span>Conditional/Missing Context</span>
            </div>
            <div class="legend-item">
                <span class="icon cycle">!</span>
                <span>Cycle Detected</span>
            </div>
            <div class="legend-item">
                <span class="permission">green</span>
                <span>Permission</span>
            </div>
            <div class="legend-item">
                <span class="relation">orange</span>
                <span>Relation</span>
            </div>
        </div>
    </div>
</body>
</html>
`

	// HTML separator for bulk check output
	bulkCheckSeparator = `<div class="bulk-separator"></div>`

	// Bulk check header template (use with fmt.Sprintf)
	bulkCheckHeader = `<div class="permission-tree"><h3 class="bulk-check-header">Check #%d</h3>`
)

// htmlRendererPool reduces allocations in bulk check operations by reusing renderer instances
var htmlRendererPool = sync.Pool{
	New: func() any {
		return &HTMLCheckTraceRenderer{}
	},
}

// RenderOptions contains optional metadata to include in the HTML output
type RenderOptions struct {
	// Timestamp is when the trace was generated. If zero, current time is used.
	Timestamp time.Time

	// Command is the command that generated this trace (e.g., "zed permission check document:doc1 viewer user:alice")
	Command string

	// SpiceDBServer is the SpiceDB server address (e.g., "grpc.authzed.com:443")
	SpiceDBServer string

	// SpiceDBVersion is the SpiceDB API version
	SpiceDBVersion string
}

// HTMLCheckTraceRenderer generates an interactive HTML visualization of a check trace
type HTMLCheckTraceRenderer struct {
	builder     strings.Builder
	encountered map[string]struct{} // reusable cycle detection map
	options     RenderOptions       // render metadata options
}

// NewHTMLCheckTraceRenderer creates a new HTML renderer (or retrieves one from the pool)
func NewHTMLCheckTraceRenderer() *HTMLCheckTraceRenderer {
	return htmlRendererPool.Get().(*HTMLCheckTraceRenderer)
}

// sanitizeRenderOptions validates and truncates metadata fields to prevent DoS attacks
// Uses rune-aware truncation to avoid splitting multi-byte UTF-8 characters
func sanitizeRenderOptions(opts RenderOptions) RenderOptions {
	sanitized := opts

	// Truncate Command field if too long (rune-safe)
	sanitized.Command = truncateStringRunes(sanitized.Command, maxMetadataLength)

	// Truncate SpiceDBServer field if too long (rune-safe)
	sanitized.SpiceDBServer = truncateStringRunes(sanitized.SpiceDBServer, maxMetadataLength)

	// Truncate SpiceDBVersion field if too long (rune-safe)
	sanitized.SpiceDBVersion = truncateStringRunes(sanitized.SpiceDBVersion, maxMetadataLength)

	return sanitized
}

// truncateStringRunes truncates a string to maxLen runes, avoiding UTF-8 boundary issues
func truncateStringRunes(s string, maxLen int) string {
	// Fast path: if byte length is within limit, string is definitely safe
	if len(s) <= maxLen {
		return s
	}

	// Convert to runes for safe truncation
	runes := []rune(s)
	if len(runes) <= maxLen {
		// String has fewer runes than max, but byte length exceeds limit
		// (e.g., many multi-byte chars). Use byte truncation but ensure we don't split a rune.
		// Since rune count is under limit, we can safely return the full string.
		return s
	}

	// Truncate at rune boundary and add ellipsis
	return string(runes[:maxLen]) + "..."
}

// Release returns the renderer to the pool for reuse
func (h *HTMLCheckTraceRenderer) Release() {
	h.builder.Reset()
	// Clear encountered map for reuse (Go 1.21+)
	if h.encountered != nil {
		clear(h.encountered)
	}
	// Clear options to prevent leaks between renders
	h.options = RenderOptions{}
	htmlRendererPool.Put(h)
}

// Render generates a complete HTML document from the check trace
func (h *HTMLCheckTraceRenderer) Render(checkTrace *v1.CheckDebugTrace, hasError bool) string {
	return h.RenderWithOptions(checkTrace, hasError, RenderOptions{})
}

// RenderWithOptions generates a complete HTML document with optional metadata
func (h *HTMLCheckTraceRenderer) RenderWithOptions(checkTrace *v1.CheckDebugTrace, hasError bool, opts RenderOptions) string {
	h.builder.Reset()
	h.builder.Grow(traceCapacityHint) // hint: typical trace is 4-16KB

	// Validate and truncate metadata fields to prevent DoS
	h.options = sanitizeRenderOptions(opts)

	// Use provided timestamp or current time
	if h.options.Timestamp.IsZero() {
		h.options.Timestamp = time.Now()
	}

	// Initialize or clear encountered map for cycle detection
	if h.encountered == nil {
		h.encountered = make(map[string]struct{}, cycleMapInitialCapacity)
	} else {
		clear(h.encountered) // Go 1.21+
	}

	h.writeHeader()
	h.builder.WriteString("<div class=\"permission-tree\">\n")
	h.renderCheckTrace(checkTrace, hasError, 0)
	h.builder.WriteString("</div>\n")
	h.writeFooter()
	return h.builder.String()
}

func (h *HTMLCheckTraceRenderer) writeHeader() {
	h.builder.WriteString(htmlHeaderPrefix)

	// Write timestamp
	h.builder.WriteString(fmt.Sprintf(
		"        <p class=\"metadata-timestamp\">Generated: %s</p>\n",
		html.EscapeString(h.options.Timestamp.Format("2006-01-02 15:04:05 MST")),
	))

	// Write optional metadata
	if h.options.Command != "" {
		h.builder.WriteString(fmt.Sprintf(
			"        <p class=\"metadata-item\">Command: <code class=\"metadata-code\">%s</code></p>\n",
			html.EscapeString(h.options.Command),
		))
	}
	if h.options.SpiceDBServer != "" {
		h.builder.WriteString(fmt.Sprintf(
			"        <p class=\"metadata-item\">SpiceDB Server: %s</p>\n",
			html.EscapeString(h.options.SpiceDBServer),
		))
	}
	if h.options.SpiceDBVersion != "" {
		h.builder.WriteString(fmt.Sprintf(
			"        <p class=\"metadata-item\">SpiceDB Version: %s</p>\n",
			html.EscapeString(h.options.SpiceDBVersion),
		))
	}

	h.builder.WriteString(htmlHeaderSuffix)
}

func (h *HTMLCheckTraceRenderer) writeFooter() {
	h.builder.WriteString(htmlFooter)
}

// writeNodeContent writes the common node content (icon, resource, permission, badges, timing)
// This helper eliminates duplication between parent and leaf node rendering
func (h *HTMLCheckTraceRenderer) writeNodeContent(pres TracePresentation, checkTrace *v1.CheckDebugTrace, resourceClass, permissionClass, badges, timing string) {
	// Use a temporary buffer to build all content, then write once
	var buf strings.Builder
	buf.Grow(nodeContentBufferSize)

	buf.WriteString(`<span class="icon `)
	buf.WriteString(pres.IconClass)
	buf.WriteString(`">`)
	buf.WriteString(pres.Icon)
	buf.WriteString(`</span>`)

	buf.WriteString(`<span class="resource`)
	buf.WriteString(resourceClass)
	buf.WriteString(`">`)
	buf.WriteString(html.EscapeString(checkTrace.Resource.ObjectType))
	buf.WriteString(`:`)
	buf.WriteString(html.EscapeString(checkTrace.Resource.ObjectId))
	buf.WriteString(`</span>`)

	buf.WriteString(`<span class="`)
	buf.WriteString(permissionClass)
	buf.WriteString(`">`)
	buf.WriteString(html.EscapeString(checkTrace.Permission))
	buf.WriteString(`</span>`)

	buf.WriteString(badges)
	buf.WriteString(timing)

	h.builder.WriteString(buf.String())
}

func (h *HTMLCheckTraceRenderer) renderCheckTrace(checkTrace *v1.CheckDebugTrace, hasError bool, depth int) {
	// Defensive: handle malformed traces
	if checkTrace == nil || checkTrace.Resource == nil {
		h.builder.WriteString(`<div class="error-node">(malformed trace: missing resource)</div>`)
		return
	}

	// Prevent stack overflow on malformed or cyclic traces
	if depth > maxTraceDepth {
		h.builder.WriteString(`<div class="error-node">(trace too deep: max depth exceeded)</div>`)
		return
	}

	// Get presentation state from shared logic
	pres := GetTracePresentation(checkTrace, hasError)

	// Build CSS classes for resource
	resourceClass := ""
	if pres.ResourceFaint {
		resourceClass = " faint"
	}

	// Determine permission class based on permission type
	var permissionClass string
	switch checkTrace.PermissionType {
	case v1.CheckDebugTrace_PERMISSION_TYPE_PERMISSION:
		permissionClass = "permission"
	case v1.CheckDebugTrace_PERMISSION_TYPE_RELATION:
		permissionClass = "relation"
	default:
		permissionClass = "permission"
	}
	if pres.PermissionFaint {
		permissionClass += " faint"
	}

	// Build cache badge HTML
	var badgesBuilder strings.Builder
	if pres.CacheBadge != "" {
		badgeClass := getCacheBadgeClass(checkTrace)
		badgesBuilder.WriteString(`<span class="badge `)
		badgesBuilder.WriteString(badgeClass)
		badgesBuilder.WriteString(`">`)
		badgesBuilder.WriteString(html.EscapeString(pres.CacheBadge))
		badgesBuilder.WriteString(`</span>`)
	} else if pres.IsCycle {
		// Cycle badge already handled by icon
		resourceClass = ""
	}

	isEndOfCycle := false
	if hasError {
		key := cycleKey(checkTrace)
		_, isEndOfCycle = h.encountered[key]
		if isEndOfCycle {
			badgesBuilder.WriteString(`<span class="badge cycle">cycle</span>`)
		}
		h.encountered[key] = struct{}{}
	}
	badges := badgesBuilder.String()

	// Timing
	timing := ""
	if checkTrace.Duration != nil {
		timing = fmt.Sprintf(`<span class="timing">%s</span>`, checkTrace.Duration.AsDuration().String())
	}

	// Cache subproblems to avoid repeated method calls
	subProblems := checkTrace.GetSubProblems()
	caveatInfo := checkTrace.GetCaveatEvaluationInfo()

	// Determine if node has children
	hasChildren := (subProblems != nil && len(subProblems.Traces) > 0) ||
		caveatInfo != nil ||
		(subProblems == nil && checkTrace.Result == v1.CheckDebugTrace_PERMISSIONSHIP_HAS_PERMISSION && checkTrace.Subject != nil && checkTrace.Subject.Object != nil)

	if hasChildren && !isEndOfCycle {
		// Use <details> for nodes with children - native expand/collapse
		// Add ARIA attributes for improved accessibility
		ariaLabel := fmt.Sprintf("%s:%s %s",
			checkTrace.Resource.ObjectType,
			checkTrace.Resource.ObjectId,
			checkTrace.Permission)
		h.builder.WriteString(fmt.Sprintf(`<details open aria-label="%s">`, html.EscapeString(ariaLabel)))
		h.builder.WriteString(`<summary role="button" tabindex="0">`)
		h.writeNodeContent(pres, checkTrace, resourceClass, permissionClass, badges, timing)
		h.builder.WriteString(`</summary>`)
		h.builder.WriteString(`<div class="children" role="group">`)

		// Render caveat evaluation (use cached caveatInfo)
		if caveatInfo != nil {
			h.renderCaveatInfo(caveatInfo)
		}

		// Render sub-problems (use cached subProblems)
		if subProblems != nil {
			for _, subProblem := range subProblems.Traces {
				h.renderCheckTrace(subProblem, hasError, depth+1)
			}
		} else if checkTrace.Result == v1.CheckDebugTrace_PERMISSIONSHIP_HAS_PERMISSION && checkTrace.Subject != nil && checkTrace.Subject.Object != nil {
			// Render subject with neutral bullet icon for visual consistency
			var subjectBuilder strings.Builder
			subjectBuilder.WriteString(html.EscapeString(checkTrace.Subject.Object.ObjectType))
			subjectBuilder.WriteString(":")
			subjectBuilder.WriteString(html.EscapeString(checkTrace.Subject.Object.ObjectId))
			if checkTrace.Subject.OptionalRelation != "" {
				subjectBuilder.WriteString("#")
				subjectBuilder.WriteString(html.EscapeString(checkTrace.Subject.OptionalRelation))
			}
			h.builder.WriteString(`<div class="subject-node"><span class="subject-bullet">•</span> `)
			h.builder.WriteString(subjectBuilder.String())
			h.builder.WriteString(`</div>`)
		}

		h.builder.WriteString(`</div>`)
		h.builder.WriteString(`</details>`)
	} else {
		// Leaf node - just a div, no interaction
		h.builder.WriteString(`<div class="leaf-node">`)
		h.writeNodeContent(pres, checkTrace, resourceClass, permissionClass, badges, timing)
		h.builder.WriteString(`</div>`)
	}
}

func (h *HTMLCheckTraceRenderer) renderCaveatInfo(caveatInfo *v1.CaveatEvalInfo) {
	icon := "✓"
	iconClass := "has-permission"
	exprClass := ""

	switch caveatInfo.Result {
	case v1.CaveatEvalInfo_RESULT_FALSE:
		icon = "⨉"
		iconClass = "no-permission"
		exprClass = " faint"
	case v1.CaveatEvalInfo_RESULT_TRUE:
		icon = "✓"
		iconClass = "has-permission"
	case v1.CaveatEvalInfo_RESULT_MISSING_SOME_CONTEXT:
		icon = "?"
		iconClass = "conditional"
	}

	h.builder.WriteString(`<div class="caveat-node">`)
	h.builder.WriteString(fmt.Sprintf(`<span class="icon %s">%s</span> `, iconClass, icon))
	h.builder.WriteString(fmt.Sprintf(
		`<span class="caveat-expr%s">%s</span> `,
		exprClass,
		html.EscapeString(caveatInfo.Expression),
	))
	h.builder.WriteString(fmt.Sprintf(
		`<span class="caveat-name">%s</span>`,
		html.EscapeString(caveatInfo.CaveatName),
	))

	contextMap := caveatInfo.Context.AsMap()
	if len(contextMap) > 0 {
		// DEFENSIVE: While redundant (AsMap handles nil), this check prevents wasted JSON marshaling
		// when context exists but is empty, which is a valid state in the protobuf model.
		// MarshalIndent can only fail for unsupported types like channels/functions.
		// structpb.Struct guarantees valid JSON types, so error is impossible here.
		contextJSON, err := json.MarshalIndent(contextMap, "", "  ")
		if err != nil {
			// Defensive: handle unexpected marshaling errors (no details wrapper for errors)
			h.builder.WriteString(fmt.Sprintf(
				`<div class="context-json">(error marshaling context: %s)</div>`,
				html.EscapeString(err.Error()),
			))
		} else {
			// Wrap context in collapsible details element for large JSON payloads
			// Only rendered when len(contextMap) > 0, so always has data
			h.builder.WriteString(`<details open class="context-details"><summary class="context-summary">Context</summary>`)
			h.builder.WriteString(fmt.Sprintf(
				`<div class="context-json">%s</div>`,
				html.EscapeString(string(contextJSON)),
			))
			h.builder.WriteString(`</details>`)
		}
	} else if caveatInfo.Result != v1.CaveatEvalInfo_RESULT_MISSING_SOME_CONTEXT {
		// No context data - only show message if not missing context (to avoid duplication)
		h.builder.WriteString(`<div class="no-context">(no matching context found)</div>`)
	}

	if caveatInfo.Result == v1.CaveatEvalInfo_RESULT_MISSING_SOME_CONTEXT {
		if caveatInfo.PartialCaveatInfo != nil && len(caveatInfo.PartialCaveatInfo.MissingRequiredContext) > 0 {
			h.builder.WriteString(fmt.Sprintf(
				`<div class="missing-context">missing context: %s</div>`,
				html.EscapeString(strings.Join(caveatInfo.PartialCaveatInfo.MissingRequiredContext, ", ")),
			))
		} else {
			h.builder.WriteString(`<div class="missing-context">missing context</div>`)
		}
	}

	h.builder.WriteString(`</div>`)
}

// DisplayCheckTraceHTML renders a check trace as HTML
func DisplayCheckTraceHTML(checkTrace *v1.CheckDebugTrace, hasError bool) string {
	return DisplayCheckTraceHTMLWithOptions(checkTrace, hasError, RenderOptions{})
}

// DisplayCheckTraceHTMLWithOptions renders a check trace as HTML with optional metadata
func DisplayCheckTraceHTMLWithOptions(checkTrace *v1.CheckDebugTrace, hasError bool, opts RenderOptions) string {
	renderer := NewHTMLCheckTraceRenderer()
	defer renderer.Release()
	return renderer.RenderWithOptions(checkTrace, hasError, opts)
}

// CheckTraceWithError pairs a check trace with its error status for bulk rendering.
// Note: HasError indicates whether the gRPC check call itself failed (e.g., due to a cycle),
// NOT whether the permission was denied. Permission denial is captured in the trace's Result field.
type CheckTraceWithError struct {
	Trace    *v1.CheckDebugTrace
	HasError bool // true if the check call failed with a gRPC error (e.g., cycle detection)
}

// DisplayBulkCheckTracesHTML renders multiple check traces as a single HTML document
func DisplayBulkCheckTracesHTML(checkTraces []*v1.CheckDebugTrace) string {
	// Convert to CheckTraceWithError for backward compatibility
	tracesWithError := make([]CheckTraceWithError, len(checkTraces))
	for i, trace := range checkTraces {
		tracesWithError[i] = CheckTraceWithError{Trace: trace, HasError: false}
	}
	return DisplayBulkCheckTracesWithErrorsHTML(tracesWithError)
}

// DisplayBulkCheckTracesWithErrorsHTML renders multiple check traces with per-trace error status
func DisplayBulkCheckTracesWithErrorsHTML(tracesWithError []CheckTraceWithError) string {
	return DisplayBulkCheckTracesWithErrorsHTMLWithOptions(tracesWithError, RenderOptions{})
}

// DisplayBulkCheckTracesWithErrorsHTMLWithOptions renders multiple check traces with optional metadata
func DisplayBulkCheckTracesWithErrorsHTMLWithOptions(tracesWithError []CheckTraceWithError, opts RenderOptions) string {
	renderer := NewHTMLCheckTraceRenderer()
	defer renderer.Release()

	renderer.builder.Reset()
	// Cap growth at 1MB to prevent excessive memory allocation for large bulk operations
	growSize := traceCapacityHint * len(tracesWithError)
	if growSize > maxBulkBuilderCapacity {
		growSize = maxBulkBuilderCapacity
	}
	renderer.builder.Grow(growSize)

	// Set options with default timestamp if not provided
	renderer.options = sanitizeRenderOptions(opts)
	if renderer.options.Timestamp.IsZero() {
		renderer.options.Timestamp = time.Now()
	}
	renderer.writeHeader()

	// Initialize encountered map for cycle detection
	if renderer.encountered == nil {
		renderer.encountered = make(map[string]struct{}, cycleMapInitialCapacity)
	}

	for i, item := range tracesWithError {
		if i > 0 {
			renderer.builder.WriteString(bulkCheckSeparator)
			// Clear encountered map between traces (Go 1.21+)
			clear(renderer.encountered)
		}
		renderer.builder.WriteString(fmt.Sprintf(bulkCheckHeader, i+1))
		renderer.renderCheckTrace(item.Trace, item.HasError, 0)
		renderer.builder.WriteString("</div>\n")
	}

	renderer.writeFooter()
	return renderer.builder.String()
}
