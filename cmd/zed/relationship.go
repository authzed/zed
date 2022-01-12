package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/AlecAivazis/survey/v2/terminal"
	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"github.com/authzed/authzed-go/v1"
	"github.com/jzelinskie/cobrautil"
	"github.com/jzelinskie/stringz"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"

	"github.com/authzed/zed/internal/storage"
)

func registerRelationshipCmd(rootCmd *cobra.Command) {
	rootCmd.AddCommand(relationshipCmd)

	relationshipCmd.AddCommand(createCmd)
	createCmd.Flags().Bool("json", false, "output as JSON")

	relationshipCmd.AddCommand(touchCmd)
	touchCmd.Flags().Bool("json", false, "output as JSON")

	relationshipCmd.AddCommand(deleteCmd)
	deleteCmd.Flags().Bool("json", false, "output as JSON")

	relationshipCmd.AddCommand(readCmd)
	readCmd.Flags().Bool("json", false, "output as JSON")
	readCmd.Flags().String("revision", "", "optional revision at which to check")
	readCmd.Flags().String("subject-filter", "", "optional subject filter")

	relationshipCmd.AddCommand(bulkDeleteCmd)
	bulkDeleteCmd.Flags().Bool("force", false, "force deletion immediately without confirmation")
	bulkDeleteCmd.Flags().String("subject-filter", "", "optional subject filter")
	bulkDeleteCmd.Flags().Bool("estimate-count", true, "estimate the count of relationships to be deleted")
}

var relationshipCmd = &cobra.Command{
	Use:   "relationship <subcommand>",
	Short: "perform CRUD operations on the Relationships in a Permissions System",
}

var createCmd = &cobra.Command{
	Use:   "create <resource:id> <relation> <subject:id>",
	Short: "create a Relationship for a Subject",
	Args:  cobra.ExactArgs(3),
	RunE: cobrautil.CommandStack(
		LogCmdFunc,
		writeRelationshipCmdFunc(v1.RelationshipUpdate_OPERATION_CREATE),
	),
}

var touchCmd = &cobra.Command{
	Use:   "touch <resource:id> <relation> <subject:id>",
	Short: "idempotently update a Relationship for a Subject",
	Args:  cobra.ExactArgs(3),
	RunE: cobrautil.CommandStack(
		LogCmdFunc,
		writeRelationshipCmdFunc(v1.RelationshipUpdate_OPERATION_TOUCH),
	),
}

var deleteCmd = &cobra.Command{
	Use:   "delete <resource:id> <relation> <subject:id>",
	Short: "delete a Relationship",
	Args:  cobra.ExactArgs(3),
	RunE: cobrautil.CommandStack(
		LogCmdFunc,
		writeRelationshipCmdFunc(v1.RelationshipUpdate_OPERATION_DELETE),
	),
}

var readCmd = &cobra.Command{
	Use:   "read <resource_type:optional_resource_id> <optional_relation> <optional_subject_type:optional_subject_id#optional_subject_relation>",
	Short: "reads Relationships",
	Args:  cobra.RangeArgs(1, 3),
	RunE:  cobrautil.CommandStack(LogCmdFunc, readRelationships),
}

var bulkDeleteCmd = &cobra.Command{
	Use:   "bulk-delete <resource_type:optional_resource_id> <optional_relation> <optional_subject_type:optional_subject_id#optional_subject_relation>",
	Short: "bulk delete Relationships",
	Args:  cobra.RangeArgs(1, 3),
	RunE:  cobrautil.CommandStack(LogCmdFunc, bulkDeleteRelationships),
}

func bulkDeleteRelationships(cmd *cobra.Command, args []string) error {
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

	client, err := authzed.NewClient(token.Endpoint, dialOptsFromFlags(cmd, token.ApiToken)...)
	if err != nil {
		return err
	}

	request, err := buildReadRequest(cmd, args)
	if err != nil {
		return err
	}

	counter := -1
	if cobrautil.MustGetBool(cmd, "estimate-count") {
		request.Consistency = &v1.Consistency{
			Requirement: &v1.Consistency_FullyConsistent{FullyConsistent: true},
		}

		log.Trace().Interface("request", request).Send()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		resp, err := client.ReadRelationships(ctx, request)
		if err != nil {
			return err
		}

		counter = 0
		for {
			_, err := resp.Recv()
			if err == io.EOF {
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
		message := fmt.Sprintf("Will delete %d relationships. Continue?", counter)
		if counter > 1000 {
			message = "Will delete 1000+ relationships. Continue?"
		}
		if counter < 0 {
			message = "Will delete all matching relationships. Continue?"
		}

		response := false
		err := survey.AskOne(&survey.Confirm{
			Message: message,
		}, &response)
		if err != nil {
			if err == terminal.InterruptErr {
				os.Exit(0)
			}

			return err
		}

		if !response {
			os.Exit(1)
		}
	}

	delRequest := &v1.DeleteRelationshipsRequest{RelationshipFilter: request.RelationshipFilter}
	log.Trace().Interface("request", delRequest).Msg("deleting relationships")

	resp, err := client.DeleteRelationships(context.Background(), delRequest)
	if err != nil {
		return err
	}

	fmt.Println(resp.DeletedAt.GetToken())
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

	subjectFilter := cobrautil.MustGetString(cmd, "subject-filter")
	switch {
	case len(args) > 1:
		readFilter.OptionalRelation = args[1]

	case len(args) == 3:
		if subjectFilter != "" {
			return nil, errors.New("cannot specify subject filter both positionally and via --subject-filter")
		}
		subjectFilter = args[2]
		fallthrough

	case subjectFilter != "":
		if strings.Contains(subjectFilter, ":") {
			subjectNS, subjectID, subjectRel, err := parseSubject(subjectFilter)
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

	if zedtoken := cobrautil.MustGetString(cmd, "revision"); zedtoken != "" {
		request.Consistency = &v1.Consistency{
			Requirement: &v1.Consistency_AtLeastAsFresh{&v1.ZedToken{Token: zedtoken}},
		}
	} else {
		request.Consistency = &v1.Consistency{
			Requirement: &v1.Consistency_FullyConsistent{FullyConsistent: true},
		}
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

	client, err := authzed.NewClient(token.Endpoint, dialOptsFromFlags(cmd, token.ApiToken)...)
	if err != nil {
		return err
	}

	log.Trace().Interface("request", request).Msg("reading relationships")
	resp, err := client.ReadRelationships(context.Background(), request)
	if err != nil {
		return err
	}

	for {
		msg, err := resp.Recv()
		if err == io.EOF {
			return nil
		}

		if err != nil {
			return err
		}

		if cobrautil.MustGetBool(cmd, "json") {
			prettyProto, err := prettyProto(msg)
			if err != nil {
				return err
			}

			fmt.Println(string(prettyProto))
		} else {
			if msg.Relationship.Subject.OptionalRelation != "" {
				fmt.Printf("%s:%s %s %s:%s#%s\n", msg.Relationship.Resource.ObjectType, msg.Relationship.Resource.ObjectId, msg.Relationship.Relation, msg.Relationship.Subject.Object.ObjectType, msg.Relationship.Subject.Object.ObjectId, msg.Relationship.Subject.OptionalRelation)
			} else {
				fmt.Printf("%s:%s %s %s:%s\n", msg.Relationship.Resource.ObjectType, msg.Relationship.Resource.ObjectId, msg.Relationship.Relation, msg.Relationship.Subject.Object.ObjectType, msg.Relationship.Subject.Object.ObjectId)
			}
		}
	}
}

func writeRelationshipCmdFunc(operation v1.RelationshipUpdate_Operation) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
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

		client, err := authzed.NewClient(token.Endpoint, dialOptsFromFlags(cmd, token.ApiToken)...)
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
					},
				},
			},
			OptionalPreconditions: nil,
		}

		log.Trace().Interface("request", request).Msg("writing relationships")
		resp, err := client.WriteRelationships(context.Background(), request)
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

		fmt.Println(resp.WrittenAt.GetToken())
		return nil
	}
}
