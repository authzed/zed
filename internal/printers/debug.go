package printers

import (
	"fmt"

	"github.com/authzed/spicedb/pkg/tuple"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"github.com/cockroachdb/cockroach/pkg/util/treeprinter"
	"github.com/gookit/color"
)

// DisplayCheckTrace prints out the check trace found in the given debug message.
func DisplayCheckTrace(checkTrace *v1.CheckDebugTrace, tp treeprinter.Node, hasError bool) {
	displayCheckTrace(checkTrace, tp, hasError, map[string]struct{}{})
}

func displayCheckTrace(checkTrace *v1.CheckDebugTrace, tp treeprinter.Node, hasError bool, encountered map[string]struct{}) {
	red := color.FgRed.Render
	green := color.FgGreen.Render
	cyan := color.FgCyan.Render
	white := color.FgWhite.Render
	faint := color.FgGray.Render

	orange := color.C256(166).Sprint
	purple := color.C256(99).Sprint
	lightgreen := color.C256(35).Sprint

	hasPermission := green("✓")
	resourceColor := white
	permissionColor := color.FgWhite.Render

	if checkTrace.PermissionType == v1.CheckDebugTrace_PERMISSION_TYPE_PERMISSION {
		permissionColor = lightgreen
	} else if checkTrace.PermissionType == v1.CheckDebugTrace_PERMISSION_TYPE_RELATION {
		permissionColor = orange
	}

	if checkTrace.Result != v1.CheckDebugTrace_PERMISSIONSHIP_HAS_PERMISSION {
		hasPermission = red("⨉")
		resourceColor = faint
		permissionColor = faint
	}

	additional := ""
	if checkTrace.GetWasCachedResult() {
		additional = cyan(" (cached)")
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

	tp = tp.Child(
		fmt.Sprintf(
			"%s %s:%s %s%s",
			hasPermission,
			resourceColor(checkTrace.Resource.ObjectType),
			resourceColor(checkTrace.Resource.ObjectId),
			permissionColor(checkTrace.Permission),
			additional,
		),
	)

	if isEndOfCycle {
		return
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
	return fmt.Sprintf("%s#%s", tuple.StringObjectRef(checkTrace.Resource), checkTrace.Permission)
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
