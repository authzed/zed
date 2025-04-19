package printers

import (
	"fmt"

	"github.com/jzelinskie/stringz"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
)

func prettySubject(subj *v1.SubjectReference) string {
	if subj.OptionalRelation == "" {
		return fmt.Sprintf(
			"%s:%s",
			stringz.TrimPrefixIndex(subj.Object.ObjectType, "/"),
			subj.Object.ObjectId,
		)
	}
	return fmt.Sprintf(
		"%s:%s->%s",
		stringz.TrimPrefixIndex(subj.Object.ObjectType, "/"),
		subj.Object.ObjectId,
		subj.OptionalRelation,
	)
}

// TreeNodeTree walks an Authzed Tree Node and creates corresponding nodes
// for a treeprinter.
func TreeNodeTree(tp *TreePrinter, treeNode *v1.PermissionRelationshipTree) {
	if treeNode.ExpandedObject != nil {
		tp = tp.Child(fmt.Sprintf(
			"%s:%s->%s",
			stringz.TrimPrefixIndex(treeNode.ExpandedObject.ObjectType, "/"),
			treeNode.ExpandedObject.ObjectId,
			treeNode.ExpandedRelation,
		))
	}
	switch typed := treeNode.TreeType.(type) {
	case *v1.PermissionRelationshipTree_Intermediate:
		switch typed.Intermediate.Operation {
		case v1.AlgebraicSubjectSet_OPERATION_UNION:
			union := tp.Child("union")
			for _, child := range typed.Intermediate.Children {
				TreeNodeTree(union, child)
			}
		case v1.AlgebraicSubjectSet_OPERATION_INTERSECTION:
			intersection := tp.Child("intersection")
			for _, child := range typed.Intermediate.Children {
				TreeNodeTree(intersection, child)
			}
		case v1.AlgebraicSubjectSet_OPERATION_EXCLUSION:
			exclusion := tp.Child("exclusion")
			for _, child := range typed.Intermediate.Children {
				TreeNodeTree(exclusion, child)
			}
		default:
			panic("unknown expand operation")
		}
	case *v1.PermissionRelationshipTree_Leaf:
		for _, subject := range typed.Leaf.Subjects {
			tp.Child(prettySubject(subject))
		}
	default:
		panic("unknown TreeNode type")
	}
}
