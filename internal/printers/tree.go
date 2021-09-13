package printers

import (
	"fmt"

	v0 "github.com/authzed/authzed-go/proto/authzed/api/v0"
	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"github.com/cockroachdb/cockroach/pkg/util/treeprinter"
	"github.com/jzelinskie/stringz"
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
func TreeNodeTree(tp treeprinter.Node, treeNode *v1.PermissionRelationshipTree) {
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

// NamespaceTree walks a Namespace Definition and creates corresponding nodes
// for a treeprinter.
func NamespaceTree(tp treeprinter.Node, nsdef *v0.NamespaceDefinition) {
	root := tp.Child(stringz.TrimPrefixIndex(nsdef.GetName(), "/"))
	for _, relation := range nsdef.GetRelation() {
		relNode := root.Child(relation.GetName())
		if rewrite := relation.GetUsersetRewrite(); rewrite != nil {
			usersetRewriteTree(relNode, rewrite)
		}
	}
}

func setOperationChildTree(tp treeprinter.Node, child *v0.SetOperation_Child) {
	switch child.ChildType.(type) {
	case *v0.SetOperation_Child_XThis:
		tp.Child("_this")
	case *v0.SetOperation_Child_ComputedUserset:
		computedUsersetTree(tp, child.GetComputedUserset())
	case *v0.SetOperation_Child_TupleToUserset:
		tupleToUsersetTree(tp, child.GetTupleToUserset())
	case *v0.SetOperation_Child_UsersetRewrite:
		usersetRewriteTree(tp, child.GetUsersetRewrite())
	}
}

func computedUsersetTree(tp treeprinter.Node, userset *v0.ComputedUserset) {
	obj := userset.GetObject()
	switch obj {
	case v0.ComputedUserset_TUPLE_OBJECT:
		tp.Child("TUPLE_OBJECT: " + userset.GetRelation())
	case v0.ComputedUserset_TUPLE_USERSET_OBJECT:
		tp.Child("TUPLE_USERSET_OBJECT " + userset.GetRelation())
	}
}

func tupleToUsersetTree(tp treeprinter.Node, t2u *v0.TupleToUserset) {
	relNode := tp.Child(t2u.GetTupleset().GetRelation())
	computedUsersetTree(relNode, t2u.GetComputedUserset())
}

func usersetRewriteTree(tp treeprinter.Node, rewrite *v0.UsersetRewrite) {
	switch rewrite.GetRewriteOperation().(type) {
	case *v0.UsersetRewrite_Union:
		childNode := tp.Child("union")
		for _, child := range rewrite.GetUnion().GetChild() {
			setOperationChildTree(childNode, child)
		}
	case *v0.UsersetRewrite_Intersection:
		childNode := tp.Child("intersection")
		for _, child := range rewrite.GetIntersection().GetChild() {
			setOperationChildTree(childNode, child)
		}
	case *v0.UsersetRewrite_Exclusion:
		childNode := tp.Child("exclusion")
		for _, child := range rewrite.GetExclusion().GetChild() {
			setOperationChildTree(childNode, child)
		}
	}
}
