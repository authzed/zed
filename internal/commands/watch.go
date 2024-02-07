package commands

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/authzed/zed/internal/client"
	"github.com/authzed/zed/internal/console"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"github.com/spf13/cobra"
)

var (
	watchObjectTypes []string
	watchRevision    string
	watchTimestamps  bool
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
	return watchRelationshipsCmd
}

var watchCmd = &cobra.Command{
	Use:        "watch [object_types, ...] [start_cursor]",
	Short:      "Watches the stream of relationship updates from the server",
	Args:       cobra.RangeArgs(0, 2),
	RunE:       watchCmdFunc,
	Deprecated: "deprecated; please use `zed watch relationships` instead",
}

var watchRelationshipsCmd = &cobra.Command{
	Use:   "watch [object_types, ...] [start_cursor]",
	Short: "Watches the stream of relationship updates from the server",
	Args:  cobra.RangeArgs(0, 2),
	RunE:  watchCmdFunc,
}

func watchCmdFunc(cmd *cobra.Command, _ []string) error {
	console.Errorf("starting watch stream over types %v and revision %v\n", watchObjectTypes, watchRevision)

	cli, err := client.NewClient(cmd)
	if err != nil {
		return err
	}

	req := &v1.WatchRequest{
		OptionalObjectTypes: watchObjectTypes,
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
					console.Printf("%v: %v\n", time.Now(), update)
				} else {
					console.Printf("%v\n", update)
				}
			}
		}
	}
}
