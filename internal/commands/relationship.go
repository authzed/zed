package commands

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"github.com/authzed/spicedb/pkg/tuple"
	"github.com/jzelinskie/cobrautil/v2"
	"github.com/jzelinskie/stringz"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"

	"github.com/authzed/zed/internal/client"
	"github.com/authzed/zed/internal/console"
)

func RegisterRelationshipCmd(rootCmd *cobra.Command) *cobra.Command {
	rootCmd.AddCommand(relationshipCmd)

	relationshipCmd.AddCommand(createCmd)
	createCmd.Flags().Bool("json", false, "output as JSON")
	createCmd.Flags().String("caveat", "", `the caveat for the relationship, with format: 'caveat_name:{"some":"context"}'`)

	relationshipCmd.AddCommand(touchCmd)
	touchCmd.Flags().Bool("json", false, "output as JSON")
	touchCmd.Flags().String("caveat", "", `the caveat for the relationship, with format: 'caveat_name:{"some":"context"}'`)

	relationshipCmd.AddCommand(deleteCmd)
	deleteCmd.Flags().Bool("json", false, "output as JSON")

	relationshipCmd.AddCommand(readCmd)
	readCmd.Flags().Bool("json", false, "output as JSON")
	readCmd.Flags().String("revision", "", "optional revision at which to check")
	_ = readCmd.Flags().MarkHidden("revision")
	readCmd.Flags().String("subject-filter", "", "optional subject filter")
	registerConsistencyFlags(readCmd.Flags())

	relationshipCmd.AddCommand(bulkDeleteCmd)
	bulkDeleteCmd.Flags().Bool("force", false, "force deletion immediately without confirmation")
	bulkDeleteCmd.Flags().String("subject-filter", "", "optional subject filter")
	bulkDeleteCmd.Flags().Bool("estimate-count", true, "estimate the count of relationships to be deleted")

	return relationshipCmd
}

var relationshipCmd = &cobra.Command{
	Use:   "relationship <subcommand>",
	Short: "perform CRUD operations on the Relationships in a Permissions System",
}

var createCmd = &cobra.Command{
	Use:   "create <resource:id> <relation> <subject:id>",
	Short: "create a Relationship for a Subject",
	Args:  cobra.ExactArgs(3),
	RunE:  writeRelationshipCmdFunc(v1.RelationshipUpdate_OPERATION_CREATE),
}

var touchCmd = &cobra.Command{
	Use:   "touch <resource:id> <relation> <subject:id>",
	Short: "idempotently update a Relationship for a Subject",
	Args:  cobra.ExactArgs(3),
	RunE:  writeRelationshipCmdFunc(v1.RelationshipUpdate_OPERATION_TOUCH),
}

var deleteCmd = &cobra.Command{
	Use:   "delete <resource:id> <relation> <subject:id>",
	Short: "delete a Relationship",
	Args:  cobra.ExactArgs(3),
	RunE:  writeRelationshipCmdFunc(v1.RelationshipUpdate_OPERATION_DELETE),
}

var readCmd = &cobra.Command{
	Use:   "read <resource_type:optional_resource_id> <optional_relation> <optional_subject_type:optional_subject_id#optional_subject_relation>",
	Short: "reads Relationships",
	Args:  cobra.RangeArgs(1, 3),
	RunE:  readRelationships,
}

var bulkDeleteCmd = &cobra.Command{
	Use:   "bulk-delete <resource_type:optional_resource_id> <optional_relation> <optional_subject_type:optional_subject_id#optional_subject_relation>",
	Short: "bulk delete Relationships",
	Args:  cobra.RangeArgs(1, 3),
	RunE:  bulkDeleteRelationships,
}

func bulkDeleteRelationships(cmd *cobra.Command, args []string) error {
	client, err := client.NewClient(cmd)
	if err != nil {
		return err
	}

	request, err := buildReadRequest(cmd, args)
	if err != nil {
		return err
	}

	counter := -1
	if cobrautil.MustGetBool(cmd, "estimate-count") {
		request.Consistency = &v1.Consistency{Requirement: &v1.Consistency_FullyConsistent{FullyConsistent: true}}

		ctx, cancel := context.WithCancel(cmd.Context())
		defer cancel()

		log.Trace().Interface("request", request).Send()
		resp, err := client.ReadRelationships(ctx, request)
		if err != nil {
			return err
		}

		counter = 0
		for {
			_, err := resp.Recv()
			if errors.Is(err, io.EOF) {
				break
			}

			if err != nil {
				return err
			}

			counter++
			if counter > 1000 {
				cancel()
				break
			}
		}
	}

	if !cobrautil.MustGetBool(cmd, "force") {
		err := performBulkDeletionConfirmation(counter)
		if err != nil {
			return err
		}
	}

	delRequest := &v1.DeleteRelationshipsRequest{RelationshipFilter: request.RelationshipFilter}
	log.Trace().Interface("request", delRequest).Msg("deleting relationships")

	resp, err := client.DeleteRelationships(cmd.Context(), delRequest)
	if err != nil {
		return err
	}

	console.Println(resp.DeletedAt.GetToken())
	return nil
}

func buildReadRequest(cmd *cobra.Command, args []string) (*v1.ReadRelationshipsRequest, error) {
	readFilter := &v1.RelationshipFilter{ResourceType: args[0]}

	if strings.Contains(args[0], ":") {
		err := stringz.SplitExact(args[0], ":", &readFilter.ResourceType, &readFilter.OptionalResourceId)
		if err != nil {
			return nil, err
		}
	}

	if len(args) > 1 {
		readFilter.OptionalRelation = args[1]
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

			readFilter.OptionalSubjectFilter = &v1.SubjectFilter{
				SubjectType:       subjectNS,
				OptionalSubjectId: subjectID,
				OptionalRelation: &v1.SubjectFilter_RelationFilter{
					Relation: subjectRel,
				},
			}
		} else {
			readFilter.OptionalSubjectFilter = &v1.SubjectFilter{
				SubjectType: subjectFilter,
			}
		}
	}

	return &v1.ReadRelationshipsRequest{
		RelationshipFilter: readFilter,
	}, nil
}

func readRelationships(cmd *cobra.Command, args []string) error {
	request, err := buildReadRequest(cmd, args)
	if err != nil {
		return err
	}

	request.Consistency, err = consistencyFromCmd(cmd)
	if err != nil {
		return err
	}

	client, err := client.NewClient(cmd)
	if err != nil {
		return err
	}

	log.Trace().Interface("request", request).Msg("reading relationships")
	resp, err := client.ReadRelationships(cmd.Context(), request)
	if err != nil {
		return err
	}

	for {
		if err := cmd.Context().Err(); err != nil {
			return err
		}

		msg, err := resp.Recv()
		if errors.Is(err, io.EOF) {
			return nil
		}

		if err != nil {
			return err
		}

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
	}
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

func writeRelationshipCmdFunc(operation v1.RelationshipUpdate_Operation) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
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

		var contextualizedCaveat *v1.ContextualizedCaveat
		if operation != v1.RelationshipUpdate_OPERATION_DELETE {
			caveatString := cobrautil.MustGetString(cmd, "caveat")
			if caveatString != "" {
				parts := strings.SplitN(caveatString, ":", 2)
				if len(parts) == 0 {
					return fmt.Errorf("invalid --caveat argument. Must be in format `caveat_name:context`, but found `%s`", caveatString)
				}

				contextualizedCaveat = &v1.ContextualizedCaveat{
					CaveatName: parts[0],
				}

				if len(parts) == 2 {
					context, err := ParseCaveatContext(parts[1])
					if err != nil {
						return err
					}
					contextualizedCaveat.Context = context
				}
			}
		}

		client, err := client.NewClient(cmd)
		if err != nil {
			return err
		}

		request := &v1.WriteRelationshipsRequest{
			Updates: []*v1.RelationshipUpdate{
				{
					Operation: operation,
					Relationship: &v1.Relationship{
						Resource: &v1.ObjectReference{
							ObjectType: objectNS,
							ObjectId:   objectID,
						},
						Relation: relation,
						Subject: &v1.SubjectReference{
							Object: &v1.ObjectReference{
								ObjectType: subjectNS,
								ObjectId:   subjectID,
							},
							OptionalRelation: subjectRel,
						},
						OptionalCaveat: contextualizedCaveat,
					},
				},
			},
			OptionalPreconditions: nil,
		}

		log.Trace().Interface("request", request).Msg("writing relationships")
		resp, err := client.WriteRelationships(cmd.Context(), request)
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

		console.Println(resp.WrittenAt.GetToken())
		return nil
	}
}
