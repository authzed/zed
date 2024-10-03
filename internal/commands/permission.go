package commands

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/authzed/spicedb/pkg/tuple"

	"github.com/authzed/authzed-go/pkg/requestmeta"
	"github.com/authzed/authzed-go/pkg/responsemeta"
	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"github.com/jzelinskie/cobrautil/v2"
	"github.com/jzelinskie/stringz"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/encoding/prototext"

	"github.com/authzed/zed/internal/client"
	"github.com/authzed/zed/internal/console"
	"github.com/authzed/zed/internal/printers"
)

var ErrMultipleConsistencies = errors.New("provided multiple consistency flags")

func registerConsistencyFlags(flags *pflag.FlagSet) {
	flags.String("consistency-at-exactly", "", "evaluate at the provided zedtoken")
	flags.String("consistency-at-least", "", "evaluate at least as consistent as the provided zedtoken")
	flags.Bool("consistency-min-latency", false, "evaluate at the zedtoken preferred by the database")
	flags.Bool("consistency-full", false, "evaluate at the newest zedtoken in the database")
}

func consistencyFromCmd(cmd *cobra.Command) (c *v1.Consistency, err error) {
	if cobrautil.MustGetBool(cmd, "consistency-full") {
		c = &v1.Consistency{Requirement: &v1.Consistency_FullyConsistent{FullyConsistent: true}}
	}
	if atLeast := cobrautil.MustGetStringExpanded(cmd, "consistency-at-least"); atLeast != "" {
		if c != nil {
			return nil, ErrMultipleConsistencies
		}
		c = &v1.Consistency{Requirement: &v1.Consistency_AtLeastAsFresh{AtLeastAsFresh: &v1.ZedToken{Token: atLeast}}}
	}

	// Deprecated (hidden) flag.
	if revision := cobrautil.MustGetStringExpanded(cmd, "revision"); revision != "" {
		if c != nil {
			return nil, ErrMultipleConsistencies
		}
		c = &v1.Consistency{Requirement: &v1.Consistency_AtLeastAsFresh{AtLeastAsFresh: &v1.ZedToken{Token: revision}}}
	}

	if exact := cobrautil.MustGetStringExpanded(cmd, "consistency-at-exactly"); exact != "" {
		if c != nil {
			return nil, ErrMultipleConsistencies
		}
		c = &v1.Consistency{Requirement: &v1.Consistency_AtExactSnapshot{AtExactSnapshot: &v1.ZedToken{Token: exact}}}
	}

	if c == nil {
		c = &v1.Consistency{Requirement: &v1.Consistency_MinimizeLatency{MinimizeLatency: true}}
	}
	return
}

func RegisterPermissionCmd(rootCmd *cobra.Command) *cobra.Command {
	rootCmd.AddCommand(permissionCmd)

	permissionCmd.AddCommand(checkCmd)
	checkCmd.Flags().Bool("json", false, "output as JSON")
	checkCmd.Flags().String("revision", "", "optional revision at which to check")
	_ = checkCmd.Flags().MarkHidden("revision")
	checkCmd.Flags().Bool("explain", false, "requests debug information from SpiceDB and prints out a trace of the requests")
	checkCmd.Flags().Bool("schema", false, "requests debug information from SpiceDB and prints out the schema used")
	checkCmd.Flags().Bool("error-on-no-permission", false, "if true, zed will return exit code 1 if subject does not have unconditional permission")
	checkCmd.Flags().String("caveat-context", "", "the caveat context to send along with the check, in JSON form")
	registerConsistencyFlags(checkCmd.Flags())

	permissionCmd.AddCommand(checkBulkCmd)
	registerConsistencyFlags(checkBulkCmd.Flags())
	checkBulkCmd.Flags().String("revision", "", "optional revision at which to check")
	checkBulkCmd.Flags().Bool("json", false, "output as JSON")

	permissionCmd.AddCommand(expandCmd)
	expandCmd.Flags().Bool("json", false, "output as JSON")
	expandCmd.Flags().String("revision", "", "optional revision at which to check")
	registerConsistencyFlags(expandCmd.Flags())

	// NOTE: `lookup` is an alias of `lookup-resources` (below)
	// and must have the same list of flags in order for it to work.
	permissionCmd.AddCommand(lookupCmd)
	lookupCmd.Flags().Bool("json", false, "output as JSON")
	lookupCmd.Flags().String("revision", "", "optional revision at which to check")
	lookupCmd.Flags().String("caveat-context", "", "the caveat context to send along with the lookup, in JSON form")
	lookupCmd.Flags().Uint32("page-limit", 0, "limit of relations returned per page")
	registerConsistencyFlags(lookupCmd.Flags())

	permissionCmd.AddCommand(lookupResourcesCmd)
	lookupResourcesCmd.Flags().Bool("json", false, "output as JSON")
	lookupResourcesCmd.Flags().String("revision", "", "optional revision at which to check")
	lookupResourcesCmd.Flags().String("caveat-context", "", "the caveat context to send along with the lookup, in JSON form")
	lookupResourcesCmd.Flags().Uint32("page-limit", 0, "limit of relations returned per page")
	registerConsistencyFlags(lookupResourcesCmd.Flags())

	permissionCmd.AddCommand(lookupSubjectsCmd)
	lookupSubjectsCmd.Flags().Bool("json", false, "output as JSON")
	lookupSubjectsCmd.Flags().String("revision", "", "optional revision at which to check")
	lookupSubjectsCmd.Flags().String("caveat-context", "", "the caveat context to send along with the lookup, in JSON form")
	registerConsistencyFlags(lookupSubjectsCmd.Flags())

	return permissionCmd
}

var permissionCmd = &cobra.Command{
	Use:     "permission <subcommand>",
	Short:   "Query the permissions in a permissions system",
	Aliases: []string{"perm"},
}

var checkBulkCmd = &cobra.Command{
	Use:   "bulk <resource:id#permission@subject:id> <resource:id#permission@subject:id> ...",
	Short: "Check a permissions in bulk exists for a resource-subject pairs",
	Args:  cobra.MinimumNArgs(1),
	RunE:  checkBulkCmdFunc,
}

var checkCmd = &cobra.Command{
	Use:               "check <resource:id> <permission> <subject:id>",
	Short:             "Check that a permission exists for a subject",
	Args:              cobra.ExactArgs(3),
	ValidArgsFunction: GetArgs(ResourceID, Permission, SubjectID),
	RunE:              checkCmdFunc,
}

var expandCmd = &cobra.Command{
	Use:               "expand <permission> <resource:id>",
	Short:             "Expand the structure of a permission",
	Args:              cobra.ExactArgs(2),
	ValidArgsFunction: cobra.NoFileCompletions,
	RunE:              expandCmdFunc,
}

var lookupResourcesCmd = &cobra.Command{
	Use:               "lookup-resources <type> <permission> <subject:id>",
	Short:             "Enumerates resources of a given type for which the subject has permission",
	Args:              cobra.ExactArgs(3),
	ValidArgsFunction: GetArgs(ResourceType, Permission, SubjectID),
	RunE:              lookupResourcesCmdFunc,
}

var lookupCmd = &cobra.Command{
	Use:               "lookup <type> <permission> <subject:id>",
	Short:             "Enumerates the resources of a given type for which the subject has permission",
	Args:              cobra.ExactArgs(3),
	ValidArgsFunction: GetArgs(ResourceType, Permission, SubjectID),
	RunE:              lookupResourcesCmdFunc,
	Deprecated:        "prefer lookup-resources",
	Hidden:            true,
}

var lookupSubjectsCmd = &cobra.Command{
	Use:               "lookup-subjects <resource:id> <permission> <subject_type#optional_subject_relation>",
	Short:             "Enumerates the subjects of a given type for which the subject has permission on the resource",
	Args:              cobra.ExactArgs(3),
	ValidArgsFunction: GetArgs(ResourceID, Permission, SubjectTypeWithOptionalRelation),
	RunE:              lookupSubjectsCmdFunc,
}

func checkCmdFunc(cmd *cobra.Command, args []string) error {
	var objectNS, objectID string
	err := stringz.SplitExact(args[0], ":", &objectNS, &objectID)
	if err != nil {
		return err
	}

	relation := args[1]

	subjectNS, subjectID, subjectRel, err := ParseSubject(args[2])
	if err != nil {
		return err
	}

	caveatContext, err := GetCaveatContext(cmd)
	if err != nil {
		return err
	}

	consistency, err := consistencyFromCmd(cmd)
	if err != nil {
		return err
	}

	client, err := client.NewClient(cmd)
	if err != nil {
		return err
	}

	request := &v1.CheckPermissionRequest{
		Resource: &v1.ObjectReference{
			ObjectType: objectNS,
			ObjectId:   objectID,
		},
		Permission: relation,
		Subject: &v1.SubjectReference{
			Object: &v1.ObjectReference{
				ObjectType: subjectNS,
				ObjectId:   subjectID,
			},
			OptionalRelation: subjectRel,
		},
		Context:     caveatContext,
		Consistency: consistency,
	}
	log.Trace().Interface("request", request).Send()

	ctx := cmd.Context()
	if cobrautil.MustGetBool(cmd, "explain") || cobrautil.MustGetBool(cmd, "schema") {
		log.Info().Msg("debugging requested on check")
		ctx = requestmeta.AddRequestHeaders(ctx, requestmeta.RequestDebugInformation)
	}

	var trailerMD metadata.MD
	resp, err := client.CheckPermission(ctx, request, grpc.Trailer(&trailerMD))
	if err != nil {
		var debugInfo *v1.DebugInformation

		// Check for the debug trace contained in the error details.
		if errInfo, ok := grpcErrorInfoFrom(err); ok {
			if encodedDebugInfo, ok := errInfo.Metadata["debug_trace_proto_text"]; ok {
				debugInfo = &v1.DebugInformation{}
				if uerr := prototext.Unmarshal([]byte(encodedDebugInfo), debugInfo); uerr != nil {
					return uerr
				}
			}
		}

		derr := displayDebugInformationIfRequested(cmd, debugInfo, trailerMD, true)
		if derr != nil {
			return derr
		}

		return err
	}

	if cobrautil.MustGetBool(cmd, "json") {
		prettyProto, err := PrettyProto(resp)
		if err != nil {
			return err
		}

		console.Println(string(prettyProto))
		return nil
	}

	switch resp.Permissionship {
	case v1.CheckPermissionResponse_PERMISSIONSHIP_CONDITIONAL_PERMISSION:
		log.Warn().Strs("fields", resp.PartialCaveatInfo.MissingRequiredContext).Msg("missing fields in caveat context")
		console.Println("caveated")

	case v1.CheckPermissionResponse_PERMISSIONSHIP_HAS_PERMISSION:
		console.Println("true")

	case v1.CheckPermissionResponse_PERMISSIONSHIP_NO_PERMISSION:
		console.Println("false")

	default:
		return fmt.Errorf("unknown permission response: %v", resp.Permissionship)
	}

	err = displayDebugInformationIfRequested(cmd, resp.DebugTrace, trailerMD, false)
	if err != nil {
		return err
	}

	if cobrautil.MustGetBool(cmd, "error-on-no-permission") {
		if resp.Permissionship != v1.CheckPermissionResponse_PERMISSIONSHIP_HAS_PERMISSION {
			os.Exit(1)
		}
	}

	return nil
}

func checkBulkCmdFunc(cmd *cobra.Command, args []string) error {
	items := make([]*v1.CheckBulkPermissionsRequestItem, 0, len(args))
	for _, arg := range args {
		rel := tuple.ParseRel(arg)
		if rel == nil {
			return fmt.Errorf("unable to parse relation: %s", arg)
		}

		item := &v1.CheckBulkPermissionsRequestItem{
			Resource: &v1.ObjectReference{
				ObjectType: rel.Resource.ObjectType,
				ObjectId:   rel.Resource.ObjectId,
			},
			Permission: rel.Relation,
			Subject: &v1.SubjectReference{
				Object: &v1.ObjectReference{
					ObjectType: rel.Subject.Object.ObjectType,
					ObjectId:   rel.Subject.Object.ObjectId,
				},
			},
		}
		if rel.OptionalCaveat != nil {
			item.Context = rel.OptionalCaveat.Context
		}
		items = append(items, item)
	}

	consistency, err := consistencyFromCmd(cmd)
	if err != nil {
		return err
	}

	bulk := &v1.CheckBulkPermissionsRequest{
		Consistency: consistency,
		Items:       items,
	}

	log.Trace().Interface("request", bulk).Send()

	ctx := cmd.Context()
	c, err := client.NewClient(cmd)
	if err != nil {
		return err
	}

	resp, err := c.CheckBulkPermissions(ctx, bulk)
	if err != nil {
		return err
	}

	if cobrautil.MustGetBool(cmd, "json") {
		prettyProto, err := PrettyProto(resp)
		if err != nil {
			return err
		}

		console.Println(string(prettyProto))
		return nil
	}

	for _, item := range resp.Pairs {
		console.Printf("%s:%s#%s@%s:%s => ",
			item.Request.Resource.ObjectType, item.Request.Resource.ObjectId, item.Request.Permission, item.Request.Subject.Object.ObjectType, item.Request.Subject.Object.ObjectId)

		switch responseType := item.Response.(type) {
		case *v1.CheckBulkPermissionsPair_Item:
			switch responseType.Item.Permissionship {
			case v1.CheckPermissionResponse_PERMISSIONSHIP_CONDITIONAL_PERMISSION:
				console.Println("caveated")

			case v1.CheckPermissionResponse_PERMISSIONSHIP_HAS_PERMISSION:
				console.Println("true")

			case v1.CheckPermissionResponse_PERMISSIONSHIP_NO_PERMISSION:
				console.Println("false")
			}
		case *v1.CheckBulkPermissionsPair_Error:
			console.Println(fmt.Sprintf("error: %s", responseType.Error))
		}
	}

	return nil
}

func expandCmdFunc(cmd *cobra.Command, args []string) error {
	relation := args[0]

	var objectNS, objectID string
	err := stringz.SplitExact(args[1], ":", &objectNS, &objectID)
	if err != nil {
		return err
	}

	consistency, err := consistencyFromCmd(cmd)
	if err != nil {
		return err
	}

	client, err := client.NewClient(cmd)
	if err != nil {
		return err
	}

	request := &v1.ExpandPermissionTreeRequest{
		Resource: &v1.ObjectReference{
			ObjectType: objectNS,
			ObjectId:   objectID,
		},
		Permission:  relation,
		Consistency: consistency,
	}
	log.Trace().Interface("request", request).Send()

	resp, err := client.ExpandPermissionTree(cmd.Context(), request)
	if err != nil {
		return err
	}

	if cobrautil.MustGetBool(cmd, "json") {
		prettyProto, err := PrettyProto(resp)
		if err != nil {
			return err
		}

		console.Println(string(prettyProto))
		return nil
	}

	tp := printers.NewTreePrinter()
	printers.TreeNodeTree(tp, resp.TreeRoot)
	tp.Print()

	return nil
}

var newLookupResourcesPageCallbackForTests func(readByPage uint)

func lookupResourcesCmdFunc(cmd *cobra.Command, args []string) error {
	objectNS := args[0]
	relation := args[1]
	subjectNS, subjectID, subjectRel, err := ParseSubject(args[2])
	if err != nil {
		return err
	}

	pageLimit := cobrautil.MustGetUint32(cmd, "page-limit")
	caveatContext, err := GetCaveatContext(cmd)
	if err != nil {
		return err
	}

	consistency, err := consistencyFromCmd(cmd)
	if err != nil {
		return err
	}

	client, err := client.NewClient(cmd)
	if err != nil {
		return err
	}

	var cursor *v1.Cursor
	var totalCount uint
	for {
		request := &v1.LookupResourcesRequest{
			ResourceObjectType: objectNS,
			Permission:         relation,
			Subject: &v1.SubjectReference{
				Object: &v1.ObjectReference{
					ObjectType: subjectNS,
					ObjectId:   subjectID,
				},
				OptionalRelation: subjectRel,
			},
			Context:        caveatContext,
			Consistency:    consistency,
			OptionalLimit:  pageLimit,
			OptionalCursor: cursor,
		}
		log.Trace().Interface("request", request).Uint32("page-limit", pageLimit).Send()

		respStream, err := client.LookupResources(cmd.Context(), request)
		if err != nil {
			return err
		}

		var count uint

	stream:
		for {
			resp, err := respStream.Recv()
			switch {
			case errors.Is(err, io.EOF):
				break stream
			case err != nil:
				return err
			default:
				count++
				totalCount++
				if cobrautil.MustGetBool(cmd, "json") {
					prettyProto, err := PrettyProto(resp)
					if err != nil {
						return err
					}

					console.Println(string(prettyProto))
				}

				console.Println(prettyLookupPermissionship(resp.ResourceObjectId, resp.Permissionship, resp.PartialCaveatInfo))
				cursor = resp.AfterResultCursor
			}
		}

		if newLookupResourcesPageCallbackForTests != nil {
			newLookupResourcesPageCallbackForTests(count)
		}
		if count == 0 || pageLimit == 0 || count < uint(pageLimit) {
			log.Trace().Interface("request", request).Uint32("page-limit", pageLimit).Uint("count", totalCount).Send()
			break
		}
	}

	return nil
}

func lookupSubjectsCmdFunc(cmd *cobra.Command, args []string) error {
	var objectNS, objectID string
	err := stringz.SplitExact(args[0], ":", &objectNS, &objectID)
	if err != nil {
		return err
	}

	permission := args[1]

	subjectType, subjectRelation := ParseType(args[2])

	caveatContext, err := GetCaveatContext(cmd)
	if err != nil {
		return err
	}

	consistency, err := consistencyFromCmd(cmd)
	if err != nil {
		return err
	}

	client, err := client.NewClient(cmd)
	if err != nil {
		return err
	}
	request := &v1.LookupSubjectsRequest{
		Resource: &v1.ObjectReference{
			ObjectType: objectNS,
			ObjectId:   objectID,
		},
		Permission:              permission,
		SubjectObjectType:       subjectType,
		OptionalSubjectRelation: subjectRelation,
		Context:                 caveatContext,
		Consistency:             consistency,
	}
	log.Trace().Interface("request", request).Send()

	respStream, err := client.LookupSubjects(cmd.Context(), request)
	if err != nil {
		return err
	}

	for {
		resp, err := respStream.Recv()
		switch {
		case errors.Is(err, io.EOF):
			return nil
		case err != nil:
			return err
		default:
			if cobrautil.MustGetBool(cmd, "json") {
				prettyProto, err := PrettyProto(resp)
				if err != nil {
					return err
				}

				console.Println(string(prettyProto))
			}
			console.Printf("%s:%s%s\n",
				subjectType,
				prettyLookupPermissionship(resp.Subject.SubjectObjectId, resp.Subject.Permissionship, resp.Subject.PartialCaveatInfo),
				excludedSubjectsString(resp.ExcludedSubjects),
			)
		}
	}
}

func excludedSubjectsString(excluded []*v1.ResolvedSubject) string {
	if len(excluded) == 0 {
		return ""
	}

	var b strings.Builder
	fmt.Fprintf(&b, " - {\n")
	for _, subj := range excluded {
		fmt.Fprintf(&b, "\t%s\n", prettyLookupPermissionship(
			subj.SubjectObjectId,
			subj.Permissionship,
			subj.PartialCaveatInfo,
		))
	}
	fmt.Fprintf(&b, "}")
	return b.String()
}

func prettyLookupPermissionship(objectID string, p v1.LookupPermissionship, info *v1.PartialCaveatInfo) string {
	var b strings.Builder
	fmt.Fprint(&b, objectID)
	if p == v1.LookupPermissionship_LOOKUP_PERMISSIONSHIP_CONDITIONAL_PERMISSION {
		fmt.Fprintf(&b, " (caveated, missing context: %s)", strings.Join(info.MissingRequiredContext, ", "))
	}
	return b.String()
}

func displayDebugInformationIfRequested(cmd *cobra.Command, debug *v1.DebugInformation, trailerMD metadata.MD, hasError bool) error {
	if cobrautil.MustGetBool(cmd, "explain") || cobrautil.MustGetBool(cmd, "schema") {
		debugInfo := &v1.DebugInformation{}
		// DebugInformation comes in trailer < 1.30, and in response payload >= 1.30
		if debug == nil {
			found, err := responsemeta.GetResponseTrailerMetadataOrNil(trailerMD, responsemeta.DebugInformation)
			if err != nil {
				return err
			}

			if found == nil {
				log.Warn().Msg("No debugging information returned for the check")
				return nil
			}

			err = protojson.Unmarshal([]byte(*found), debugInfo)
			if err != nil {
				return err
			}
		} else {
			debugInfo = debug
		}

		if debugInfo.Check == nil {
			log.Warn().Msg("No trace found for the check")
			return nil
		}

		if cobrautil.MustGetBool(cmd, "explain") {
			tp := printers.NewTreePrinter()
			printers.DisplayCheckTrace(debugInfo.Check, tp, hasError)
			tp.Print()
		}

		if cobrautil.MustGetBool(cmd, "schema") {
			console.Println()
			console.Println(debugInfo.SchemaUsed)
		}
	}
	return nil
}
