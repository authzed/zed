package treeprint

import (
	api "github.com/authzed/authzed-go/arrakisapi/api"
	"github.com/cockroachdb/cockroach/pkg/util/treeprinter"
)

func Namespace(tp treeprinter.Node, nsdef *api.NamespaceDefinition) {
	root := tp.Child(nsdef.GetName())
	for _, relation := range nsdef.GetRelation() {
		relNode := root.Child(relation.GetName())
		if rewrite := relation.GetUsersetRewrite(); rewrite != nil {
			UsersetRewrite(relNode, rewrite)
		}
	}
}

func SetOperationChild(tp treeprinter.Node, child *api.SetOperation_Child) {
	switch child.ChildType.(type) {
	case *api.SetOperation_Child_XThis:
		tp.Child("_this")
	case *api.SetOperation_Child_ComputedUserset:
		ComputedUserset(tp, child.GetComputedUserset())
	case *api.SetOperation_Child_TupleToUserset:
		TupleToUserset(tp, child.GetTupleToUserset())
	case *api.SetOperation_Child_UsersetRewrite:
		UsersetRewrite(tp, child.GetUsersetRewrite())
	}
}

func ComputedUserset(tp treeprinter.Node, userset *api.ComputedUserset) {
	obj := userset.GetObject()
	switch obj {
	case api.ComputedUserset_TUPLE_OBJECT:
		tp.Child("TUPLE_OBJECT: " + userset.GetRelation())
	case api.ComputedUserset_TUPLE_USERSET_OBJECT:
		tp.Child("TUPLE_USERSET_OBJECT " + userset.GetRelation())
	}
}

func TupleToUserset(tp treeprinter.Node, t2u *api.TupleToUserset) {
	relNode := tp.Child(t2u.GetTupleset().GetRelation())
	ComputedUserset(relNode, t2u.GetComputedUserset())
}

func UsersetRewrite(tp treeprinter.Node, rewrite *api.UsersetRewrite) {
	switch rewrite.GetRewriteOperation().(type) {
	case *api.UsersetRewrite_Union:
		childNode := tp.Child("union")
		for _, child := range rewrite.GetUnion().GetChild() {
			SetOperationChild(childNode, child)
		}
	case *api.UsersetRewrite_Intersection:
		childNode := tp.Child("intersection")
		for _, child := range rewrite.GetIntersection().GetChild() {
			SetOperationChild(childNode, child)
		}
	case *api.UsersetRewrite_Exclusion:
		childNode := tp.Child("exclusion")
		for _, child := range rewrite.GetExclusion().GetChild() {
			SetOperationChild(childNode, child)
		}
	}
}
