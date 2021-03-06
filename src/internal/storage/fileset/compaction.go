package fileset

import (
	"context"
	"time"

	"github.com/pachyderm/pachyderm/v2/src/internal/errors"
	"github.com/pachyderm/pachyderm/v2/src/internal/storage/fileset/index"
)

// IsCompacted returns true if the fileset is already in compacted form.
func (s *Storage) IsCompacted(ctx context.Context, id ID) (bool, error) {
	md, err := s.store.Get(ctx, id)
	if err != nil {
		return false, err
	}
	switch x := md.Value.(type) {
	case *Metadata_Composite:
		ids, err := x.Composite.PointsTo()
		if err != nil {
			return false, err
		}
		ids, err = s.Flatten(ctx, ids)
		if err != nil {
			return false, err
		}
		layers, err := s.getPrimitiveBatch(ctx, ids)
		if err != nil {
			return false, err
		}
		return isCompacted(s.levelFactor, layers), nil
	case *Metadata_Primitive:
		return true, nil
	default:
		// TODO: should it be?
		return false, errors.Errorf("IsCompacted is not defined for empty filesets")
	}
}

func (s *Storage) getPrimitiveBatch(ctx context.Context, ids []ID) ([]*Primitive, error) {
	var layers []*Primitive
	for _, id := range ids {
		prim, err := s.getPrimitive(ctx, id)
		if err != nil {
			return nil, err
		}
		layers = append(layers, prim)
	}
	return layers, nil
}

func isCompacted(factor int64, layers []*Primitive) bool {
	return indexOfCompacted(factor, layers) == len(layers)
}

// indexOfCompacted returns the last value of i for which the "compacted relationship"
// is maintained for all layers[:i+1]
// the "compacted relationship" is defined as leftSize >= (rightSize * factor)
// If there is an element at i+1. It will be the first element which does not satisfy
// the compacted relationship with i.
func indexOfCompacted(factor int64, inputs []*Primitive) int {
	l := len(inputs)
	for i := 0; i < l-1; i++ {
		leftSize := inputs[i].SizeBytes
		rightSize := inputs[i+1].SizeBytes
		if leftSize < rightSize*factor {
			return i
		}
	}
	return l
}

// Compact compacts a set of filesets into an output fileset.
func (s *Storage) Compact(ctx context.Context, ids []ID, ttl time.Duration, opts ...index.Option) (*ID, error) {
	ids, err := s.Flatten(ctx, ids)
	if err != nil {
		return nil, err
	}
	inputs, err := s.getPrimitiveBatch(ctx, ids)
	if err != nil {
		return nil, err
	}
	i := indexOfCompacted(s.levelFactor, inputs)
	if i == len(inputs) {
		return s.Compose(ctx, ids, ttl)
	}
	// merge everything from i onward into a single layer
	merged, err := s.Merge(ctx, ids[i:], ttl)
	if err != nil {
		return nil, err
	}
	// replace everything from i onward with the merged version
	ids2 := []ID{}
	ids2 = append(ids2, ids[:i]...)
	ids2 = append(ids2, *merged)
	return s.Compact(ctx, ids2, ttl)
}

// CompactionTask contains everything needed to perform the smallest unit of compaction
type CompactionTask struct {
	Inputs    []ID
	PathRange *index.PathRange
}

// CompactionWorker can perform CompactionTasks
type CompactionWorker func(ctx context.Context, spec CompactionTask) (*ID, error)

// CompactionBatchWorker can perform batches of CompactionTasks
type CompactionBatchWorker func(ctx context.Context, spec []CompactionTask) ([]ID, error)

// DistributedCompactor performs compaction by fanning out tasks to workers.
type DistributedCompactor struct {
	s          *Storage
	maxFanIn   int
	workerFunc CompactionBatchWorker
}

// NewDistributedCompactor returns a DistributedCompactor which will compact by fanning out
// work to workerFunc, while respecting maxFanIn
// TODO: change this to CompactionWorker after work package changes.
func NewDistributedCompactor(s *Storage, maxFanIn int, workerFunc CompactionBatchWorker) *DistributedCompactor {
	return &DistributedCompactor{
		s:          s,
		maxFanIn:   maxFanIn,
		workerFunc: workerFunc,
	}
}

// Compact runs a compaction on the ids
func (c *DistributedCompactor) Compact(ctx context.Context, ids []ID, ttl time.Duration) (*ID, error) {
	if len(ids) <= c.maxFanIn {
		return c.shardedCompact(ctx, ids, ttl)
	}
	childSize := c.maxFanIn
	for len(ids)/childSize > c.maxFanIn {
		childSize *= c.maxFanIn
	}
	results := []ID{}
	for start := 0; start < len(ids); start += childSize {
		end := start + childSize
		if end > len(ids) {
			end = len(ids)
		}
		id, err := c.Compact(ctx, ids[start:end], ttl)
		if err != nil {
			return nil, err
		}
		results = append(results, *id)
	}
	return c.Compact(ctx, results, ttl)
}

func (c *DistributedCompactor) shardedCompact(ctx context.Context, ids []ID, ttl time.Duration) (*ID, error) {
	var tasks []CompactionTask
	if err := c.s.Shard(ctx, ids, func(pathRange *index.PathRange) error {
		tasks = append(tasks, CompactionTask{
			Inputs:    ids,
			PathRange: pathRange,
		})
		return nil
	}); err != nil {
		return nil, err
	}
	results, err := c.workerFunc(ctx, tasks)
	if err != nil {
		return nil, err
	}
	if len(results) != len(tasks) {
		return nil, errors.Errorf("results are a different length than tasks")
	}
	return c.s.Concat(ctx, results, ttl)
}
