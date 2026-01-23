package grpcutil

import (
	"context"
	"errors"
	"runtime"
	"sync/atomic"

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
// goroutines. If maxWorkers is 1, then batching will happen synchronously. If
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

	var failed atomic.Bool
	sem := semaphore.NewWeighted(int64(maxWorkers))
	g, ctx := errgroup.WithContext(ctx)
	numBatches := (n + batchSize - 1) / batchSize
	for i := 0; i < numBatches; i++ {
		if err := sem.Acquire(ctx, 1); err != nil {
			break
		}

		// After acquiring the semaphore, check whether a previous batch
		// has already failed. This handles the race where a failing batch
		// releases the semaphore before the errgroup cancels the context.
		if failed.Load() {
			sem.Release(1)
			break
		}

		batchNum := i
		g.Go(func() error {
			defer sem.Release(1)
			start := batchNum * batchSize
			end := minimum(start+batchSize, n)
			err := each(ctx, batchNum, start, end)
			if err != nil {
				failed.Store(true)
			}
			return err
		})
	}
	return g.Wait()
}
