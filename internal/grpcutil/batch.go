package grpcutil

import (
	"context"
	"errors"
	"fmt"
	"runtime"

	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"
)

func minimum(a int, b int) int {
	if a <= b {
		return a
	}
	return b
}

// EachFunc is a callback function that is called for each batch. no is the
// batch number, start is the starting index of this batch in the slice, and
// end is the ending index of this batch in the slice.
type EachFunc func(ctx context.Context, no int, start int, end int) error

// ConcurrentBatch will calculate the minimum number of batches to required to batch n items
// with batchSize batches. For each batch, it will execute the each function.
// These functions will be processed in parallel using maxWorkers number of
// goroutines. If maxWorkers is 1, then batching will happen sychronously. If
// maxWorkers is 0, then GOMAXPROCS number of workers will be used.
//
// If an error occurs during a batch, all the worker's contexts are cancelled
// and the original error is returned.
func ConcurrentBatch(ctx context.Context, n int, batchSize int, maxWorkers int, each EachFunc) error {
	if n < 0 {
		return errors.New("cannot batch items of length < 0")
	} else if n == 0 {
		// Batching zero items is a noop.
		return nil
	}

	if batchSize < 1 {
		return errors.New("cannot batch items with batch size < 1")
	}

	if maxWorkers < 0 {
		return errors.New("cannot batch items with workers < 0")
	} else if maxWorkers == 0 {
		maxWorkers = runtime.GOMAXPROCS(0)
	}

	sem := semaphore.NewWeighted(int64(maxWorkers))
	g, ctx := errgroup.WithContext(ctx)
	numBatches := (n + batchSize - 1) / batchSize
	for i := 0; i < numBatches; i++ {
		if err := sem.Acquire(ctx, 1); err != nil {
			return fmt.Errorf("failed to acquire semaphore for batch number %d: %w", i, err)
		}

		batchNum := i
		g.Go(func() error {
			defer sem.Release(1)
			start := batchNum * batchSize
			end := minimum(start+batchSize, n)
			return each(ctx, batchNum, start, end)
		})
	}
	return g.Wait()
}
