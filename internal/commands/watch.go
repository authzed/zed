package commands

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

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
	rootCmd.AddCommand(watchCmd)

	watchCmd.Flags().StringSliceVar(&watchObjectTypes, "object_types", nil, "optional object types to watch updates for")
	watchCmd.Flags().StringVar(&watchRevision, "revision", "", "optional revision at which to start watching")
	watchCmd.Flags().BoolVar(&watchTimestamps, "timestamp", false, "shows timestamp of incoming update events")
	return watchCmd
}

func RegisterWatchRelationshipCmd(parentCmd *cobra.Command) *cobra.Command {
	parentCmd.AddCommand(watchRelationshipsCmd)
	watchRelationshipsCmd.Flags().StringSliceVar(&watchObjectTypes, "object_types", nil, "optional object types to watch updates for")
	watchRelationshipsCmd.Flags().StringVar(&watchRevision, "revision", "", "optional revision at which to start watching")
	watchRelationshipsCmd.Flags().BoolVar(&watchTimestamps, "timestamp", false, "shows timestamp of incoming update events")
	watchRelationshipsCmd.Flags().StringSliceVar(&watchRelationshipFilters, "filter", nil, "optional filter(s) for the watch stream. Example: `optional_resource_type:optional_resource_id_or_prefix#optional_relation@optional_subject_filter`")
	return watchRelationshipsCmd
}

var watchCmd = &cobra.Command{
	Use:        "watch [object_types, ...] [start_cursor]",
	Short:      "Watches the stream of relationship updates from the server",
	Args:       ValidationWrapper(cobra.RangeArgs(0, 2)),
	RunE:       watchCmdFunc,
	Deprecated: "deprecated; please use `zed watch relationships` instead",
}

var watchRelationshipsCmd = &cobra.Command{
	Use:   "watch [object_types, ...] [start_cursor]",
	Short: "Watches the stream of relationship updates from the server",
	Args:  ValidationWrapper(cobra.RangeArgs(0, 2)),
	RunE:  watchCmdFunc,
}

func watchCmdFunc(cmd *cobra.Command, _ []string) error {
	console.Printf("starting watch stream over types %v and revision %v\n", watchObjectTypes, watchRevision)

	cli, err := client.NewClient(cmd)
	if err != nil {
		return err
	}

	relFilters := make([]*v1.RelationshipFilter, 0, len(watchRelationshipFilters))
	for _, filter := range watchRelationshipFilters {
		relFilter, err := parseRelationshipFilter(filter)
		if err != nil {
			return err
		}
		relFilters = append(relFilters, relFilter)
	}

	req := &v1.WatchRequest{
		OptionalObjectTypes:         watchObjectTypes,
		OptionalRelationshipFilters: relFilters,
	}
	if watchRevision != "" {
		req.OptionalStartCursor = &v1.ZedToken{Token: watchRevision}
	}

	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	signalctx, interruptCancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
	defer interruptCancel()

	watchStream, err := cli.Watch(ctx, req)
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
				return err
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
