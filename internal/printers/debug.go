package printers

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gookit/color"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"github.com/authzed/spicedb/pkg/genutil/mapz"
	dispatchv1 "github.com/authzed/spicedb/pkg/proto/dispatch/v1"
	"github.com/authzed/spicedb/pkg/tuple"
	"github.com/authzed/zed/internal/console"
)

// DisplayCheckTrace prints out the check trace found in the given debug message.
func DisplayCheckTrace(checkTrace *v1.CheckDebugTrace, tp *TreePrinter, hasError bool) {
	displayCheckTrace(checkTrace, tp, hasError, map[string]struct{}{})
}

func displayCheckTrace(checkTrace *v1.CheckDebugTrace, tp *TreePrinter, hasError bool, encountered map[string]struct{}) {
	red := color.FgRed.Render
	green := color.FgGreen.Render
	cyan := color.FgCyan.Render
	white := color.FgWhite.Render
	faint := color.FgGray.Render
	magenta := color.FgMagenta.Render
	yellow := color.FgYellow.Render

	orange := color.C256(166).Sprint
	purple := color.C256(99).Sprint
	lightgreen := color.C256(35).Sprint
	caveatColor := color.C256(198).Sprint

	hasPermission := green("✓")
	resourceColor := white
	permissionColor := color.FgWhite.Render

	switch checkTrace.PermissionType {
	case v1.CheckDebugTrace_PERMISSION_TYPE_PERMISSION:
		permissionColor = lightgreen
	case v1.CheckDebugTrace_PERMISSION_TYPE_RELATION:
		permissionColor = orange
	}

	switch checkTrace.Result {
	case v1.CheckDebugTrace_PERMISSIONSHIP_CONDITIONAL_PERMISSION:
		switch checkTrace.CaveatEvaluationInfo.Result {
		case v1.CaveatEvalInfo_RESULT_FALSE:
			hasPermission = red("⨉")
			resourceColor = faint
			permissionColor = faint

		case v1.CaveatEvalInfo_RESULT_MISSING_SOME_CONTEXT:
			hasPermission = magenta("?")
			resourceColor = faint
			permissionColor = faint
		}
	case v1.CheckDebugTrace_PERMISSIONSHIP_NO_PERMISSION:
		hasPermission = red("⨉")
		resourceColor = faint
		permissionColor = faint
	case v1.CheckDebugTrace_PERMISSIONSHIP_UNSPECIFIED:
		hasPermission = yellow("∵")
	}

	additional := ""
	if checkTrace.GetWasCachedResult() {
		sourceKind := ""
		source := checkTrace.Source
		if source != "" {
			parts := strings.Split(source, ":")
			if len(parts) > 0 {
				sourceKind = parts[0]
			}
		}
		switch sourceKind {
		case "":
			additional = cyan(" (cached)")

		case "spicedb":
			additional = cyan(" (cached by spicedb)")

		case "materialize":
			additional = purple(" (cached by materialize)")

		default:
			additional = cyan(fmt.Sprintf(" (cached by %s)", sourceKind))
		}
	} else if hasError && isPartOfCycle(checkTrace, map[string]struct{}{}) {
		hasPermission = orange("!")
		resourceColor = white
	}

	isEndOfCycle := false
	if hasError {
		key := cycleKey(checkTrace)
		_, isEndOfCycle = encountered[key]
		if isEndOfCycle {
			additional = color.C256(166).Sprint(" (cycle)")
		}
		encountered[key] = struct{}{}
	}

	timing := ""
	if checkTrace.Duration != nil {
		timing = fmt.Sprintf(" (%s)", checkTrace.Duration.AsDuration().String())
	}

	tp = tp.Child(
		fmt.Sprintf(
			"%s %s:%s %s%s%s",
			hasPermission,
			resourceColor(checkTrace.Resource.ObjectType),
			resourceColor(checkTrace.Resource.ObjectId),
			permissionColor(checkTrace.Permission),
			additional,
			timing,
		),
	)

	if isEndOfCycle {
		return
	}

	if checkTrace.GetCaveatEvaluationInfo() != nil {
		indicator := ""
		exprColor := color.FgWhite.Render
		switch checkTrace.CaveatEvaluationInfo.Result {
		case v1.CaveatEvalInfo_RESULT_FALSE:
			indicator = red("⨉")
			exprColor = faint

		case v1.CaveatEvalInfo_RESULT_TRUE:
			indicator = green("✓")

		case v1.CaveatEvalInfo_RESULT_MISSING_SOME_CONTEXT:
			indicator = magenta("?")
		}

		white := color.HEXStyle("fff")
		white.SetOpts(color.Opts{color.OpItalic})

		contextMap := checkTrace.CaveatEvaluationInfo.Context.AsMap()
		caveatName := checkTrace.CaveatEvaluationInfo.CaveatName

		c := tp.Child(fmt.Sprintf("%s %s %s", indicator, exprColor(checkTrace.CaveatEvaluationInfo.Expression), caveatColor(caveatName)))
		if len(contextMap) > 0 {
			contextJSON, _ := json.MarshalIndent(contextMap, "", "  ")
			c.Child(string(contextJSON))
		} else if checkTrace.CaveatEvaluationInfo.Result != v1.CaveatEvalInfo_RESULT_MISSING_SOME_CONTEXT {
			c.Child(faint("(no matching context found)"))
		}

		if checkTrace.CaveatEvaluationInfo.Result == v1.CaveatEvalInfo_RESULT_MISSING_SOME_CONTEXT {
			c.Child("missing context: " + strings.Join(checkTrace.CaveatEvaluationInfo.PartialCaveatInfo.MissingRequiredContext, ", "))
		}
	}

	if checkTrace.GetSubProblems() != nil {
		for _, subProblem := range checkTrace.GetSubProblems().Traces {
			displayCheckTrace(subProblem, tp, hasError, encountered)
		}
	} else if checkTrace.Result == v1.CheckDebugTrace_PERMISSIONSHIP_HAS_PERMISSION {
		tp.Child(purple(fmt.Sprintf("%s:%s %s", checkTrace.Subject.Object.ObjectType, checkTrace.Subject.Object.ObjectId, checkTrace.Subject.OptionalRelation)))
	}
}

func cycleKey(checkTrace *v1.CheckDebugTrace) string {
	return fmt.Sprintf("%s#%s", tuple.V1StringObjectRef(checkTrace.Resource), checkTrace.Permission)
}

func isPartOfCycle(checkTrace *v1.CheckDebugTrace, encountered map[string]struct{}) bool {
	if checkTrace.GetSubProblems() == nil {
		return false
	}

	encounteredCopy := make(map[string]struct{}, len(encountered))
	for k, v := range encountered {
		encounteredCopy[k] = v
	}

	key := cycleKey(checkTrace)
	if _, ok := encounteredCopy[key]; ok {
		return true
	}

	encounteredCopy[key] = struct{}{}

	for _, subProblem := range checkTrace.GetSubProblems().Traces {
		if isPartOfCycle(subProblem, encounteredCopy) {
			return true
		}
	}

	return false
}

const (
	originalSubjectMarker = "<-- Original subject"
	cycleStartMarker = "<-- Cycle start"
	cycleEndMarker = "<-- Cycle end"
)

func DisplayLookupResourcesTrace(trace *dispatchv1.LookupDebugTrace) {
	seen := mapz.NewSet[string]()
	framesToPrint := make([]string, 0, len(trace.Frames))
	cyclicFrame := ""

	// Iterate through frames to determine whether it's cyclic and
	// construct framesToPrint
	for _, frame := range trace.Frames {
		frameString := frameToString(frame)
		notSeen := seen.Add(frameString)
		framesToPrint = append(framesToPrint, frameString)
		// If we see something we've seen before, we've got a cycle.
		// We mark it as such and stop appending frames.
		if !notSeen {
			cyclicFrame = frameString
			break
		}
	}

	if cyclicFrame != "" {
		printCyclicFrames(framesToPrint, cyclicFrame)
	} else {
		printRecursionDepthFrames(framesToPrint)
	}
}

func printCyclicFrames(frameStrings []string, cyclicFrame string) {
	console.Errorf("Cycle found in data:\n")
	var output strings.Builder
	for i, frameString := range frameStrings {
		switch {
			// The first frame will be the original subject -
			// print it first and mark it.
		case i == 0:
			output.WriteString("\t" + frameString + "\t" + originalSubjectMarker + "\n")
			// We mark the last string as the end of the cycle,
			// since that's where we stopped counting.
		case i == len(frameStrings) - 1:
			output.WriteString("\t" + frameString + "\t" + cycleEndMarker + "\n")
			// We mark the cyclic frame as the start of the cycle.
		case frameString == cyclicFrame:
			output.WriteString("\t" + frameString + "\t" + cycleStartMarker + "\n")
			// Otherwise we just print the frame.
		default:
			output.WriteString("\t" + frameString + "\n")
		}
	}
	console.Errorf(output.String())
}

func printRecursionDepthFrames(frameStrings []string) {
	console.Errorf("Recursion depth hit along path:\n")
	var output strings.Builder
	for i, frameString := range frameStrings {
		if i == 0 {
			output.WriteString("\t" + frameString + "\t" + originalSubjectMarker + "\n")
		} else {
			output.WriteString("\t" + frameString + "\n")
		}
	}
	console.Errorf(output.String())
}

func frameToString(frame *dispatchv1.LookupDebugFrame) string {
	return fmt.Sprintf("%s:%s#%s", frame.ResourceType, frame.ResourceId, frame.Relation)
}
