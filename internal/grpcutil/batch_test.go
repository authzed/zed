package grpcutil

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

type batch struct {
	no    int
	start int
	end   int
}

func generateItems(n int) []string {
	items := make([]string, n)
	for i := 0; i < n; i++ {
		items[i] = fmt.Sprintf("item %d", i)
	}
	return items
}

func TestConcurrentBatchOrdering(t *testing.T) {
	const batchSize = 3
	const workers = 1 // Set to one to keep everything synchronous.

	tests := []struct {
		name  string
		items []string
		want  []batch
	}{
		{
			name:  "1 item",
			items: generateItems(1),
			want: []batch{
				{0, 0, 1},
			},
		},
		{
			name:  "3 items",
			items: generateItems(3),
			want: []batch{
				{0, 0, 3},
			},
		},
		{
			name:  "5 items",
			items: generateItems(5),
			want: []batch{
				{0, 0, 3},
				{1, 3, 5},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			require := require.New(t)

			gotCh := make(chan batch, len(tt.items))
			fn := func(ctx context.Context, no, start, end int) error {
				gotCh <- batch{no, start, end}
				return nil
			}

			err := ConcurrentBatch(context.Background(), len(tt.items), batchSize, workers, fn)
			require.NoError(err)

			got := make([]batch, len(gotCh))
			i := 0
			for span := range gotCh {
				got[i] = span
				i++

				if i == len(got) {
					break
				}
			}
			require.Equal(tt.want, got)
		})
	}
}

func TestConcurrentBatch(t *testing.T) {
	tests := []struct {
		name      string
		items     []string
		batchSize int
		workers   int
		wantCalls int
	}{
		{
			name:      "5 batches",
			items:     generateItems(50),
			batchSize: 10,
			workers:   3,
			wantCalls: 5,
		},
		{
			name:      "0 batches",
			items:     []string{},
			batchSize: 10,
			workers:   3,
			wantCalls: 0,
		},
		{
			name:      "1 batch",
			items:     generateItems(10),
			batchSize: 10,
			workers:   3,
			wantCalls: 1,
		},
		{
			name:      "1 full batch, 1 partial batch",
			items:     generateItems(15),
			batchSize: 10,
			workers:   3,
			wantCalls: 2,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			require := require.New(t)

			var calls int64
			fn := func(ctx context.Context, no, start, end int) error {
				atomic.AddInt64(&calls, 1)
				return nil
			}
			err := ConcurrentBatch(context.Background(), len(tt.items), tt.batchSize, tt.workers, fn)

			require.NoError(err)
			require.Equal(tt.wantCalls, int(calls))
		})
	}
}
