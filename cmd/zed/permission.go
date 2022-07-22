package main

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/authzed/spicedb/pkg/tuple"

	"github.com/authzed/authzed-go/pkg/requestmeta"
	"github.com/authzed/authzed-go/pkg/responsemeta"
	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"github.com/authzed/authzed-go/v1"
	"github.com/cockroachdb/cockroach/pkg/util/treeprinter"
	"github.com/gookit/color"
	"github.com/jzelinskie/cobrautil"
	"github.com/jzelinskie/stringz"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/authzed/zed/internal/printers"
	"github.com/authzed/zed/internal/storage"
)

func registerPermissionCmd(rootCmd *cobra.Command) {
	rootCmd.AddCommand(permissionCmd)

	permissionCmd.AddCommand(checkCmd)
	checkCmd.Flags().Bool("json", false, "output as JSON")
	checkCmd.Flags().String("revision", "", "optional revision at which to check")
	checkCmd.Flags().Bool("trace", false, "requests the debug information of the check from SpiceDB and prints out a trace of the requests")
	checkCmd.Flags().Bool("schema-used", false, "requests the debug information of the check from SpiceDB and prints out the schema used")

	permissionCmd.AddCommand(expandCmd)
	expandCmd.Flags().Bool("json", false, "output as JSON")
	expandCmd.Flags().String("revision", "", "optional revision at which to check")

	permissionCmd.AddCommand(lookupCmd)
	lookupCmd.Flags().Bool("json", false, "output as JSON")
	lookupCmd.Flags().String("revision", "", "optional revision at which to check")
}

var permissionCmd = &cobra.Command{
	Use:   "permission <subcommand>",
	Short: "perform queries on the Permissions in a Permissions System",
}

var checkCmd = &cobra.Command{
	Use:   "check <resource:id> <permission> <subject:id>",
	Short: "check that a Permission exists for a Subject",
	Args:  cobra.ExactArgs(3),
	RunE:  cobrautil.CommandStack(LogCmdFunc, checkCmdFunc),
}

var expandCmd = &cobra.Command{
	Use:   "expand <permission> <resource:id>",
	Short: "expand the structure of a Permission",
	Args:  cobra.ExactArgs(2),
	RunE:  cobrautil.CommandStack(LogCmdFunc, expandCmdFunc),
}

var lookupCmd = &cobra.Command{
	Use:   "lookup <type> <permission> <subject:id>",
	Short: "lookup the Resources of a given type for which the Subject has Permission",
	Args:  cobra.ExactArgs(3),
	RunE:  cobrautil.CommandStack(LogCmdFunc, lookupCmdFunc),
}

func parseSubject(s string) (namespace, id, relation string, err error) {
	err = stringz.SplitExact(s, ":", &namespace, &id)
	if err != nil {
		return
	}
	err = stringz.SplitExact(id, "#", &id, &relation)
	if err != nil {
		relation = ""
		err = nil
	}
	return
}

func checkCmdFunc(cmd *cobra.Command, args []string) error {
	var objectNS, objectID string
	err := stringz.SplitExact(args[0], ":", &objectNS, &objectID)
	if err != nil {
		return err
	}

	relation := args[1]

	subjectNS, subjectID, subjectRel, err := parseSubject(args[2])
	if err != nil {
		return err
	}

	configStore, secretStore := defaultStorage()
	token, err := storage.DefaultToken(
		cobrautil.MustGetString(cmd, "endpoint"),
		cobrautil.MustGetString(cmd, "token"),
		configStore,
		secretStore,
	)
	if err != nil {
		return err
	}
	log.Trace().Interface("token", token).Send()

	client, err := authzed.NewClient(token.Endpoint, dialOptsFromFlags(cmd, token)...)
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
	}

	if zedtoken := cobrautil.MustGetString(cmd, "revision"); zedtoken != "" {
		request.Consistency = atLeastAsFresh(zedtoken)
	}
	log.Trace().Interface("request", request).Send()

	ctx := context.Background()
	if cobrautil.MustGetBool(cmd, "trace") || cobrautil.MustGetBool(cmd, "schema-used") {
		log.Info().Msg("debugging requested on check")
		ctx = requestmeta.AddRequestHeaders(ctx, requestmeta.RequestDebugInformation)
	}

	var trailerMD metadata.MD
	resp, err := client.CheckPermission(ctx, request, grpc.Trailer(&trailerMD))
	if err != nil {
		derr := displayDebugInformationIfRequested(cmd, trailerMD, true)
		if derr != nil {
			return derr
		}

		return err
	}

	if cobrautil.MustGetBool(cmd, "json") {
		prettyProto, err := prettyProto(resp)
		if err != nil {
			return err
		}

		fmt.Println(string(prettyProto))
		return nil
	}

	fmt.Println(resp.Permissionship == v1.CheckPermissionResponse_PERMISSIONSHIP_HAS_PERMISSION)
	return displayDebugInformationIfRequested(cmd, trailerMD, false)
}

func displayDebugInformationIfRequested(cmd *cobra.Command, trailerMD metadata.MD, hasError bool) error {
	if cobrautil.MustGetBool(cmd, "trace") || cobrautil.MustGetBool(cmd, "schema-used") {
		found, err := responsemeta.GetResponseTrailerMetadataOrNil(trailerMD, responsemeta.DebugInformation)
		if err != nil {
			return err
		}

		if found == nil {
			log.Warn().Msg("No debuging information returned for the check")
			return nil
		}

		debugInfo := &v1.DebugInformation{}
		err = protojson.Unmarshal([]byte(*found), debugInfo)
		if err != nil {
			return err
		}

		if debugInfo.Check == nil {
			log.Warn().Msg("No trace found for the check")
			return nil
		}

		if cobrautil.MustGetBool(cmd, "trace") {
			tp := treeprinter.New()
			displayCheckTrace(debugInfo.Check, tp, hasError, map[string]struct{}{})
			fmt.Println()
			fmt.Println(tp.String())
		}

		if cobrautil.MustGetBool(cmd, "schema-used") {
			fmt.Println()
			fmt.Println(debugInfo.SchemaUsed)
		}
	}
	return nil
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

func expandCmdFunc(cmd *cobra.Command, args []string) error {
	relation := args[0]

	var objectNS, objectID string
	err := stringz.SplitExact(args[1], ":", &objectNS, &objectID)
	if err != nil {
		return err
	}

	configStore, secretStore := defaultStorage()
	token, err := storage.DefaultToken(
		cobrautil.MustGetString(cmd, "endpoint"),
		cobrautil.MustGetString(cmd, "token"),
		configStore,
		secretStore,
	)
	if err != nil {
		return err
	}
	log.Trace().Interface("token", token).Send()

	client, err := authzed.NewClient(token.Endpoint, dialOptsFromFlags(cmd, token)...)
	if err != nil {
		return err
	}

	request := &v1.ExpandPermissionTreeRequest{
		Resource: &v1.ObjectReference{
			ObjectType: objectNS,
			ObjectId:   objectID,
		},
		Permission: relation,
	}

	if zedtoken := cobrautil.MustGetString(cmd, "revision"); zedtoken != "" {
		request.Consistency = atLeastAsFresh(zedtoken)
	}
	log.Trace().Interface("request", request).Send()

	resp, err := client.ExpandPermissionTree(context.Background(), request)
	if err != nil {
		return err
	}

	if cobrautil.MustGetBool(cmd, "json") {
		prettyProto, err := prettyProto(resp)
		if err != nil {
			return err
		}

		fmt.Println(string(prettyProto))
		return nil
	}

	tp := treeprinter.New()
	printers.TreeNodeTree(tp, resp.TreeRoot)
	fmt.Println(tp.String())

	return nil
}

func lookupCmdFunc(cmd *cobra.Command, args []string) error {
	objectNS := args[0]
	relation := args[1]
	subjectNS, subjectID, subjectRel, err := parseSubject(args[2])
	if err != nil {
		return err
	}

	configStore, secretStore := defaultStorage()
	token, err := storage.DefaultToken(
		cobrautil.MustGetString(cmd, "endpoint"),
		cobrautil.MustGetString(cmd, "token"),
		configStore,
		secretStore,
	)
	if err != nil {
		return err
	}
	log.Trace().Interface("token", token).Send()

	client, err := authzed.NewClient(token.Endpoint, dialOptsFromFlags(cmd, token)...)
	if err != nil {
		return err
	}

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
	}

	if zedtoken := cobrautil.MustGetString(cmd, "revision"); zedtoken != "" {
		request.Consistency = atLeastAsFresh(zedtoken)
	}
	log.Trace().Interface("request", request).Send()

	respStream, err := client.LookupResources(context.Background(), request)
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
				prettyProto, err := prettyProto(resp)
				if err != nil {
					return err
				}

				fmt.Println(string(prettyProto))
			}
			fmt.Println(resp.ResourceObjectId)
		}
	}
}
