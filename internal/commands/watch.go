package commands

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"

	"github.com/authzed/zed/internal/client"
	"github.com/authzed/zed/internal/console"
)

var (
	watchObjectTypes         []string
	watchRevision            string
	watchTimestamps          bool
	watchRelationshipFilters []string
)

func RegisterWatchCmd(rootCmd *cobra.Command) *cobra.Command {
	watchCmd := &cobra.Command{
		Use:        "watch [object_types, ...] [revision]",
		Short:      "Watches the stream of relationship updates and schema updates from the server",
		Args:       ValidationWrapper(cobra.RangeArgs(0, 2)),
		RunE:       watchCmdFunc,
		Deprecated: "please use `zed relationships watch` instead",
	}

	rootCmd.AddCommand(watchCmd)
	watchCmd.Flags().StringSliceVar(&watchObjectTypes, "object_types", nil, "optional object types to watch updates for")
	watchCmd.Flags().StringVar(&watchRevision, "revision", "", "optional revision at which to start watching")
	watchCmd.Flags().BoolVar(&watchTimestamps, "timestamp", false, "shows timestamp of incoming update events")
	return watchCmd
}

func RegisterWatchRelationshipCmd(parentCmd *cobra.Command) *cobra.Command {
	watchRelationshipsCmd := &cobra.Command{
		Use:   "watch [object_types, ...] [revision]",
		Short: "Watches the stream of relationship updates and schema updates from the server",
		Args:  ValidationWrapper(cobra.RangeArgs(0, 2)),
		RunE:  watchCmdFunc,
		Example: `
zed relationship watch --filter document:finance
zed relationship watch --filter document:finance#view
zed relationship watch --filter document:finance#view@user:anne
`,
	}

	parentCmd.AddCommand(watchRelationshipsCmd)
	watchRelationshipsCmd.Flags().StringSliceVar(&watchObjectTypes, "object_types", nil, "optional object types to watch updates for")
	watchRelationshipsCmd.Flags().StringVar(&watchRevision, "revision", "", "optional revision at which to start watching")
	watchRelationshipsCmd.Flags().BoolVar(&watchTimestamps, "timestamp", false, "shows timestamp of incoming update events")
	watchRelationshipsCmd.Flags().StringSliceVar(&watchRelationshipFilters, "filter", nil, "optional filter(s) for the watch stream")
	return watchRelationshipsCmd
}

func watchCmdFunc(cmd *cobra.Command, _ []string) error {
	client, err := client.NewClient(cmd)
	if err != nil {
		return err
	}
	return watchCmdFuncImpl(cmd, client, processResponse)
}

func watchCmdFuncImpl(cmd *cobra.Command, watchClient v1.WatchServiceClient, processResponse func(resp *v1.WatchResponse)) error {
	console.Printf("starting watch stream over types %v and revision %v\n", watchObjectTypes, watchRevision)

	relFilters := make([]*v1.RelationshipFilter, 0, len(watchRelationshipFilters))
	for _, filter := range watchRelationshipFilters {
		relFilter, err := parseRelationshipFilter(filter)
		if err != nil {
			return err
		}
		relFilters = append(relFilters, relFilter)
	}

	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	signalctx, interruptCancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
	defer interruptCancel()

	req := &v1.WatchRequest{
		OptionalObjectTypes:         watchObjectTypes,
		OptionalRelationshipFilters: relFilters,
		OptionalUpdateKinds: []v1.WatchKind{
			v1.WatchKind_WATCH_KIND_INCLUDE_CHECKPOINTS, // keeps connection open during quiet periods
			v1.WatchKind_WATCH_KIND_INCLUDE_SCHEMA_UPDATES,
		},
	}

	if watchRevision != "" {
		req.OptionalStartCursor = &v1.ZedToken{Token: watchRevision}
	}

	watchStream, err := watchClient.Watch(ctx, req)
	if err != nil {
		return err
	}

	for {
		select {
		case <-signalctx.Done():
			console.Errorf("stream interrupted after program termination\n")
			return nil
		case <-ctx.Done():
			console.Errorf("stream canceled after context cancellation\n")
			return nil
		default:
			resp, err := watchStream.Recv()
			if err != nil {
				ok, err := isRetryable(err)
				if !ok {
					return err
				}

				log.Trace().Err(err).Msg("will retry from the last known revision " + watchRevision)
				req.OptionalStartCursor = &v1.ZedToken{Token: watchRevision}
				watchStream, err = watchClient.Watch(ctx, req)
				if err != nil {
					return err
				}
				continue
			}

			processResponse(resp)
		}
	}
}

func isRetryable(err error) (bool, error) {
	statusErr, ok := status.FromError(err)
	if !ok || (statusErr.Code() != codes.Unavailable) {
		return false, err
	}
	return true, nil
}

func processResponse(resp *v1.WatchResponse) {
	if resp.ChangesThrough != nil {
		watchRevision = resp.ChangesThrough.Token
	}

	if resp.SchemaUpdated {
		if watchTimestamps {
			console.Printf("%v: ", time.Now())
		}
		console.Println("SCHEMA UPDATED")
	}

	for _, update := range resp.Updates {
		if watchTimestamps {
			console.Printf("%v: ", time.Now())
		}

		switch update.Operation {
		case v1.RelationshipUpdate_OPERATION_CREATE:
			console.Printf("CREATED ")

		case v1.RelationshipUpdate_OPERATION_DELETE:
			console.Printf("DELETED ")

		case v1.RelationshipUpdate_OPERATION_TOUCH:
			console.Printf("TOUCHED ")
		}

		subjectRelation := ""
		if update.Relationship.Subject.OptionalRelation != "" {
			subjectRelation = " " + update.Relationship.Subject.OptionalRelation
		}

		console.Printf("%s:%s %s %s:%s%s\n",
			update.Relationship.Resource.ObjectType,
			update.Relationship.Resource.ObjectId,
			update.Relationship.Relation,
			update.Relationship.Subject.Object.ObjectType,
			update.Relationship.Subject.Object.ObjectId,
			subjectRelation,
		)
	}
}

func parseRelationshipFilter(relFilterStr string) (*v1.RelationshipFilter, error) {
	relFilter := &v1.RelationshipFilter{}
	pieces := strings.Split(relFilterStr, "@")
	if len(pieces) > 2 {
		return nil, fmt.Errorf("invalid relationship filter: %s", relFilterStr)
	}

	if len(pieces) == 2 {
		subjectFilter, err := parseSubjectFilter(pieces[1])
		if err != nil {
			return nil, err
		}
		relFilter.OptionalSubjectFilter = subjectFilter
	}

	if len(pieces) > 0 {
		resourcePieces := strings.Split(pieces[0], "#")
		if len(resourcePieces) > 2 {
			return nil, fmt.Errorf("invalid relationship filter: %s", relFilterStr)
		}

		if len(resourcePieces) == 2 {
			relFilter.OptionalRelation = resourcePieces[1]
		}

		resourceTypePieces := strings.Split(resourcePieces[0], ":")
		if len(resourceTypePieces) > 2 {
			return nil, fmt.Errorf("invalid relationship filter: %s", relFilterStr)
		}

		relFilter.ResourceType = resourceTypePieces[0]
		if len(resourceTypePieces) == 2 {
			optionalResourceIDOrPrefix := resourceTypePieces[1]
			if strings.HasSuffix(optionalResourceIDOrPrefix, "%") {
				relFilter.OptionalResourceIdPrefix = strings.TrimSuffix(optionalResourceIDOrPrefix, "%")
			} else {
				relFilter.OptionalResourceId = optionalResourceIDOrPrefix
			}
		}
	}

	return relFilter, nil
}

func parseSubjectFilter(subjectFilterStr string) (*v1.SubjectFilter, error) {
	subjectFilter := &v1.SubjectFilter{}
	pieces := strings.Split(subjectFilterStr, "#")
	if len(pieces) > 2 {
		return nil, fmt.Errorf("invalid subject filter: %s", subjectFilterStr)
	}

	subjectTypePieces := strings.Split(pieces[0], ":")
	if len(subjectTypePieces) > 2 {
		return nil, fmt.Errorf("invalid subject filter: %s", subjectFilterStr)
	}

	subjectFilter.SubjectType = subjectTypePieces[0]
	if len(subjectTypePieces) == 2 {
		subjectFilter.OptionalSubjectId = subjectTypePieces[1]
	}

	if len(pieces) == 2 {
		subjectFilter.OptionalRelation = &v1.SubjectFilter_RelationFilter{
			Relation: pieces[1],
		}
	}

	return subjectFilter, nil
}
