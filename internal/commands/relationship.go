package commands

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"

	"github.com/authzed/zed/internal/client"
	"github.com/authzed/zed/internal/console"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"github.com/authzed/spicedb/pkg/tuple"
	"github.com/jzelinskie/cobrautil/v2"
	"github.com/jzelinskie/stringz"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/status"
)

func RegisterRelationshipCmd(rootCmd *cobra.Command) *cobra.Command {
	rootCmd.AddCommand(relationshipCmd)

	relationshipCmd.AddCommand(createCmd)
	createCmd.Flags().Bool("json", false, "output as JSON")
	createCmd.Flags().String("caveat", "", `the caveat for the relationship, with format: 'caveat_name:{"some":"context"}'`)
	createCmd.Flags().IntP("batch-size", "b", 100, "batch size when writing streams of relationships from stdin")

	relationshipCmd.AddCommand(touchCmd)
	touchCmd.Flags().Bool("json", false, "output as JSON")
	touchCmd.Flags().String("caveat", "", `the caveat for the relationship, with format: 'caveat_name:{"some":"context"}'`)
	touchCmd.Flags().IntP("batch-size", "b", 100, "batch size when writing streams of relationships from stdin")

	relationshipCmd.AddCommand(deleteCmd)
	deleteCmd.Flags().Bool("json", false, "output as JSON")
	deleteCmd.Flags().IntP("batch-size", "b", 100, "batch size when deleting streams of relationships from stdin")

	relationshipCmd.AddCommand(readCmd)
	readCmd.Flags().Bool("json", false, "output as JSON")
	readCmd.Flags().String("revision", "", "optional revision at which to check")
	_ = readCmd.Flags().MarkHidden("revision")
	readCmd.Flags().String("subject-filter", "", "optional subject filter")
	readCmd.Flags().Uint32("page-limit", 100, "limit of relations returned per page")
	registerConsistencyFlags(readCmd.Flags())

	relationshipCmd.AddCommand(bulkDeleteCmd)
	bulkDeleteCmd.Flags().Bool("force", false, "force deletion of all elements in batches defined by <optional-limit>")
	bulkDeleteCmd.Flags().String("subject-filter", "", "optional subject filter")
	bulkDeleteCmd.Flags().Uint("optional-limit", 1000, "the max amount of elements to delete. If you want to delete all in batches of size <optional-limit>, set --force to true")
	bulkDeleteCmd.Flags().Bool("estimate-count", true, "estimate the count of relationships to be deleted")
	_ = bulkDeleteCmd.Flags().MarkDeprecated("estimate-count", "no longer used, make use of --optional-limit instead")
	return relationshipCmd
}

var relationshipCmd = &cobra.Command{
	Use:   "relationship <subcommand>",
	Short: "Query and mutate the relationships in a permissions system",
}

var createCmd = &cobra.Command{
	Use:               "create <resource:id> <relation> <subject:id#optional_subject_relation>",
	Short:             "Create a relationship for a subject",
	Args:              StdinOrExactArgs(3),
	ValidArgsFunction: GetArgs(ResourceID, Permission, SubjectTypeWithOptionalRelation),
	RunE:              writeRelationshipCmdFunc(v1.RelationshipUpdate_OPERATION_CREATE, os.Stdin),
}

var touchCmd = &cobra.Command{
	Use:               "touch <resource:id> <relation> <subject:id#optional_subject_relation>",
	Short:             "Idempotently updates a relationship for a subject",
	Args:              StdinOrExactArgs(3),
	ValidArgsFunction: GetArgs(ResourceID, Permission, SubjectTypeWithOptionalRelation),
	RunE:              writeRelationshipCmdFunc(v1.RelationshipUpdate_OPERATION_TOUCH, os.Stdin),
}

var deleteCmd = &cobra.Command{
	Use:               "delete <resource:id> <relation> <subject:id#optional_subject_relation>",
	Short:             "Deletes a relationship",
	Args:              StdinOrExactArgs(3),
	ValidArgsFunction: GetArgs(ResourceID, Permission, SubjectTypeWithOptionalRelation),
	RunE:              writeRelationshipCmdFunc(v1.RelationshipUpdate_OPERATION_DELETE, os.Stdin),
}

var readCmd = &cobra.Command{
	Use:               "read <resource_type:optional_resource_id> <optional_relation> <optional_subject_type:optional_subject_id#optional_subject_relation>",
	Short:             "Enumerates relationships matching the provided pattern",
	Args:              cobra.RangeArgs(1, 3),
	ValidArgsFunction: GetArgs(ResourceID, Permission, SubjectTypeWithOptionalRelation),
	RunE:              readRelationships,
}

var bulkDeleteCmd = &cobra.Command{
	Use:               "bulk-delete <resource_type:optional_resource_id> <optional_relation> <optional_subject_type:optional_subject_id#optional_subject_relation>",
	Short:             "Deletes relationships matching the provided pattern en masse",
	Args:              cobra.RangeArgs(1, 3),
	ValidArgsFunction: GetArgs(ResourceID, Permission, SubjectTypeWithOptionalRelation),
	RunE:              bulkDeleteRelationships,
}

func StdinOrExactArgs(n int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if ok := isArgsViaFile(os.Stdin) && len(args) == 0; ok {
			return nil
		}

		return cobra.ExactArgs(n)(cmd, args)
	}
}

func isArgsViaFile(file *os.File) bool {
	return !isFileTerminal(file)
}

func bulkDeleteRelationships(cmd *cobra.Command, args []string) error {
	spicedbClient, err := client.NewClient(cmd)
	if err != nil {
		return err
	}

	filter, err := buildRelationshipsFilter(cmd, args)
	if err != nil {
		return err
	}

	bar := console.CreateProgressBar("deleting relationships")
	defer func() {
		_ = bar.Finish()
	}()

	allowPartialDeletions := cobrautil.MustGetBool(cmd, "force")
	optionalLimit := cobrautil.MustGetUint(cmd, "optional-limit")
	var resp *v1.DeleteRelationshipsResponse
	for {
		delRequest := &v1.DeleteRelationshipsRequest{
			RelationshipFilter:            filter,
			OptionalLimit:                 uint32(optionalLimit),
			OptionalAllowPartialDeletions: allowPartialDeletions,
		}
		log.Trace().Interface("request", delRequest).Msg("deleting relationships")

		resp, err = spicedbClient.DeleteRelationships(cmd.Context(), delRequest)
		if errorInfo, ok := grpcErrorInfoFrom(err); ok {
			if errorInfo.GetReason() == v1.ErrorReason_ERROR_REASON_TOO_MANY_RELATIONSHIPS_FOR_TRANSACTIONAL_DELETE.String() {
				return fmt.Errorf("could not delete %s, as more than %s relationships were found. Consider increasing --optional-limit or deleting all relationships using --force",
					errorInfo.GetMetadata()["filter_resource_type"],
					errorInfo.GetMetadata()["limit"])
			}
		}
		if err != nil {
			return err
		}

		if resp.DeletionProgress == v1.DeleteRelationshipsResponse_DELETION_PROGRESS_COMPLETE {
			break
		}

		if err := bar.Add(int(optionalLimit)); err != nil {
			return err
		}
	}

	_ = bar.Finish()
	console.Println(resp.DeletedAt.GetToken())
	return nil
}

func grpcErrorInfoFrom(err error) (*errdetails.ErrorInfo, bool) {
	if err == nil {
		return nil, false
	}

	if s, ok := status.FromError(err); ok {
		for _, d := range s.Details() {
			if errInfo, ok := d.(*errdetails.ErrorInfo); ok {
				return errInfo, true
			}
		}
	}

	return nil, false
}

func buildRelationshipsFilter(cmd *cobra.Command, args []string) (*v1.RelationshipFilter, error) {
	filter := &v1.RelationshipFilter{ResourceType: args[0]}

	if strings.Contains(args[0], ":") {
		var resourceID string
		err := stringz.SplitExact(args[0], ":", &filter.ResourceType, &resourceID)
		if err != nil {
			return nil, err
		}

		if strings.HasSuffix(resourceID, "%") {
			filter.OptionalResourceIdPrefix = strings.TrimSuffix(resourceID, "%")
		} else {
			filter.OptionalResourceId = resourceID
		}
	}

	if len(args) > 1 {
		filter.OptionalRelation = args[1]
	}

	subjectFilter := cobrautil.MustGetString(cmd, "subject-filter")
	if len(args) == 3 {
		if subjectFilter != "" {
			return nil, errors.New("cannot specify subject filter both positionally and via --subject-filter")
		}
		subjectFilter = args[2]
	}

	if subjectFilter != "" {
		if strings.Contains(subjectFilter, ":") {
			subjectNS, subjectID, subjectRel, err := ParseSubject(subjectFilter)
			if err != nil {
				return nil, err
			}

			filter.OptionalSubjectFilter = &v1.SubjectFilter{
				SubjectType:       subjectNS,
				OptionalSubjectId: subjectID,
				OptionalRelation: &v1.SubjectFilter_RelationFilter{
					Relation: subjectRel,
				},
			}
		} else {
			filter.OptionalSubjectFilter = &v1.SubjectFilter{
				SubjectType: subjectFilter,
			}
		}
	}

	return filter, nil
}

func readRelationships(cmd *cobra.Command, args []string) error {
	spicedbClient, err := client.NewClient(cmd)
	if err != nil {
		return err
	}

	filter, err := buildRelationshipsFilter(cmd, args)
	if err != nil {
		return err
	}

	request := &v1.ReadRelationshipsRequest{RelationshipFilter: filter}

	limit := cobrautil.MustGetUint32(cmd, "page-limit")
	request.OptionalLimit = limit
	request.Consistency, err = consistencyFromCmd(cmd)
	if err != nil {
		return err
	}

	lastCursor := request.OptionalCursor
	for {
		request.OptionalCursor = lastCursor
		var cursorToken string
		if lastCursor != nil {
			cursorToken = lastCursor.Token
		}
		log.Trace().Interface("request", request).Str("cursor", cursorToken).Msg("reading relationships page")
		readRelClient, err := spicedbClient.ReadRelationships(cmd.Context(), request)
		if err != nil {
			return err
		}

		var relCount uint32
		for {
			if err := cmd.Context().Err(); err != nil {
				return err
			}

			msg, err := readRelClient.Recv()
			if errors.Is(err, io.EOF) {
				break
			}

			if err != nil {
				return err
			}

			lastCursor = msg.AfterResultCursor
			relCount++
			if err := printRelationship(cmd, msg); err != nil {
				return err
			}
		}

		if relCount < limit || limit == 0 {
			return nil
		}

		if relCount > limit {
			log.Warn().Uint32("limit-specified", limit).Uint32("relationships-received", relCount).Msg("page limit ignored, pagination may not be supported by the server, consider updating SpiceDB")
			return nil
		}
	}
}

func printRelationship(cmd *cobra.Command, msg *v1.ReadRelationshipsResponse) error {
	if cobrautil.MustGetBool(cmd, "json") {
		prettyProto, err := PrettyProto(msg)
		if err != nil {
			return err
		}

		console.Println(string(prettyProto))
	} else {
		relString, err := relationshipToString(msg.Relationship)
		if err != nil {
			return err
		}
		console.Println(relString)
	}

	return nil
}

func argsToRelationship(args []string) (*v1.Relationship, error) {
	if len(args) != 3 {
		return nil, fmt.Errorf("expected 3 arguments, but got %d", len(args))
	}

	rel := tupleToRel(args[0], args[1], args[2])
	if rel == nil {
		return nil, errors.New("failed to parse input arguments")
	}

	return rel, nil
}

func relationshipToString(rel *v1.Relationship) (string, error) {
	relString, err := tuple.StringRelationship(rel)
	if err != nil {
		return "", err
	}

	relString = strings.Replace(relString, "@", " ", 1)
	relString = strings.Replace(relString, "#", " ", 1)
	return relString, nil
}

// parseRelationshipLine splits a line of update input that comes from stdin
// and returns the fields representing the 3 arguments. This is to handle
// the fact that relationships specified via stdin can't escape spaces like
// shell arguments.
func parseRelationshipLine(line string) (string, string, string, error) {
	line = strings.TrimSpace(line)
	resourceIdx := strings.IndexFunc(line, unicode.IsSpace)
	if resourceIdx == -1 {
		args := 0
		if line != "" {
			args = 1
		}
		return "", "", "", fmt.Errorf("expected %s to have 3 arguments, but got %v", line, args)
	}

	resource := line[:resourceIdx]
	rest := strings.TrimSpace(line[resourceIdx+1:])
	relationIdx := strings.IndexFunc(rest, unicode.IsSpace)
	if relationIdx == -1 {
		args := 1
		if strings.TrimSpace(rest) != "" {
			args = 2
		}
		return "", "", "", fmt.Errorf("expected %s to have 3 arguments, but got %v", line, args)
	}

	relation := rest[:relationIdx]
	rest = strings.TrimSpace(rest[relationIdx+1:])
	if rest == "" {
		return "", "", "", fmt.Errorf("expected %s to have 3 arguments, but got 2", line)
	}

	return resource, relation, rest, nil
}

func FileRelationshipParser(f *os.File) RelationshipParser {
	scanner := bufio.NewScanner(f)
	return func() (*v1.Relationship, error) {
		if scanner.Scan() {
			res, rel, subj, err := parseRelationshipLine(scanner.Text())
			if err != nil {
				return nil, err
			}
			return tupleToRel(res, rel, subj), nil
		}
		if err := scanner.Err(); err != nil {
			return nil, err
		}
		return nil, ErrExhaustedRelationships
	}
}

func tupleToRel(resource, relation, subject string) *v1.Relationship {
	return tuple.ParseRel(resource + "#" + relation + "@" + subject)
}

func SliceRelationshipParser(args []string) RelationshipParser {
	ran := false
	return func() (*v1.Relationship, error) {
		if ran {
			return nil, ErrExhaustedRelationships
		}
		ran = true
		return tupleToRel(args[0], args[1], args[2]), nil
	}
}

func writeUpdates(ctx context.Context, spicedbClient client.Client, updates []*v1.RelationshipUpdate, json bool) error {
	if len(updates) == 0 {
		return nil
	}
	request := &v1.WriteRelationshipsRequest{
		Updates:               updates,
		OptionalPreconditions: nil,
	}

	log.Trace().Interface("request", request).Msg("writing relationships")
	resp, err := spicedbClient.WriteRelationships(ctx, request)
	if err != nil {
		return err
	}

	if json {
		prettyProto, err := PrettyProto(resp)
		if err != nil {
			return err
		}

		console.Println(string(prettyProto))
	} else {
		console.Println(resp.WrittenAt.GetToken())
	}

	return nil
}

// RelationshipParser is a closure that can produce relationships.
// When there are no more relationships, it will return ErrExhaustedRelationships.
type RelationshipParser func() (*v1.Relationship, error)

// ErrExhaustedRelationships signals that the last producible value of a RelationshipParser
// has already been consumed.
// Functions should return this error to signal a graceful end of input.
var ErrExhaustedRelationships = errors.New("exhausted all relationships")

func writeRelationshipCmdFunc(operation v1.RelationshipUpdate_Operation, input *os.File) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		parser := SliceRelationshipParser(args)
		if isArgsViaFile(input) && len(args) == 0 {
			parser = FileRelationshipParser(input)
		}

		spicedbClient, err := client.NewClient(cmd)
		if err != nil {
			return err
		}

		batchSize := cobrautil.MustGetInt(cmd, "batch-size")
		updateBatch := make([]*v1.RelationshipUpdate, 0)
		doJSON := cobrautil.MustGetBool(cmd, "json")

		for {
			rel, err := parser()
			if errors.Is(err, ErrExhaustedRelationships) {
				return writeUpdates(cmd.Context(), spicedbClient, updateBatch, doJSON)
			} else if err != nil {
				return err
			}

			if operation != v1.RelationshipUpdate_OPERATION_DELETE {
				if err := handleCaveatFlag(cmd, rel); err != nil {
					return err
				}
			}

			updateBatch = append(updateBatch, &v1.RelationshipUpdate{
				Operation:    operation,
				Relationship: rel,
			})
			if len(updateBatch) == batchSize {
				if err := writeUpdates(cmd.Context(), spicedbClient, updateBatch, doJSON); err != nil {
					return err
				}
				updateBatch = nil
			}
		}
	}
}

func handleCaveatFlag(cmd *cobra.Command, rel *v1.Relationship) error {
	caveatString := cobrautil.MustGetString(cmd, "caveat")
	if caveatString != "" {
		if rel.OptionalCaveat != nil {
			return errors.New("cannot specify a caveat in both the relationship and the --caveat flag")
		}

		parts := strings.SplitN(caveatString, ":", 2)
		if len(parts) == 0 {
			return fmt.Errorf("invalid --caveat argument. Must be in format `caveat_name:context`, but found `%s`", caveatString)
		}

		rel.OptionalCaveat = &v1.ContextualizedCaveat{
			CaveatName: parts[0],
		}

		if len(parts) == 2 {
			caveatCtx, err := ParseCaveatContext(parts[1])
			if err != nil {
				return err
			}
			rel.OptionalCaveat.Context = caveatCtx
		}
	}
	return nil
}
