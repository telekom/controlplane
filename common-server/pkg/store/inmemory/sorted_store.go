package inmemory

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/dgraph-io/badger/v4"
	"github.com/pkg/errors"
	"github.com/telekom/controlplane/common-server/internal/informer"
	"github.com/telekom/controlplane/common-server/pkg/problems"
	"github.com/telekom/controlplane/common-server/pkg/store"
)

type SortableStore[T store.Object] struct {
	*InmemoryObjectStore[T]
	expectedSize int
}

var _ store.ObjectStore[store.Object] = &InmemoryObjectStore[store.Object]{}
var _ informer.EventHandler = &InmemoryObjectStore[store.Object]{}

func Sortable[T store.Object](ios *InmemoryObjectStore[T], storeOpts StoreOpts) store.ObjectStore[T] {
	ss := &SortableStore[T]{
		InmemoryObjectStore: ios,
		expectedSize:        200,
	}

	ss.allowedSorts = storeOpts.AllowedSorts

	for _, sp := range ss.allowedSorts {
		ss.sortValueCache.Store(sp, &sync.Map{})
	}

	return ss
}

func NewSortableOrDie[T store.Object](ctx context.Context, storeOpts StoreOpts) store.ObjectStore[T] {
	return Sortable(NewOrDie[T](ctx, storeOpts).(*InmemoryObjectStore[T]), storeOpts)
}

func (s *SortableStore[T]) List(ctx context.Context, listOpts store.ListOpts) (*store.ListResponse[T], error) {
	hasSorters := len(listOpts.Sorters) > 0
	if hasSorters {
		return s.listSorted(ctx, listOpts)
	}
	return s.InmemoryObjectStore.List(ctx, listOpts)
}

func (s *SortableStore[T]) getSortValue(path, key string) any {
	if cache, ok := s.sortValueCache.Load(path); ok {
		if m, ok := cache.(*sync.Map); ok {
			if v, ok := m.Load(key); ok {
				return v
			}
		}
	}
	return nil
}

func (s *SortableStore[T]) listSorted(_ context.Context, listOpts store.ListOpts) (result *store.ListResponse[T], err error) {
	for _, sorter := range listOpts.Sorters {
		if !slices.Contains(s.allowedSorts, sorter.Path) {
			return nil, problems.BadRequest(fmt.Sprintf("sort path %s is not allowed", sorter.Path))
		}
	}

	type sortedItem struct {
		key        string
		data       []byte
		sortValues []any
	}

	opts := badger.DefaultIteratorOptions
	opts.Prefix = []byte(listOpts.Prefix)
	opts.PrefetchValues = true
	items := make([]sortedItem, 0, s.expectedSize)

	err = s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(opts)
		defer it.Close()
		for it.Rewind(); it.ValidForPrefix(opts.Prefix); it.Next() {
			key := string(it.Item().Key())

			data, err := it.Item().ValueCopy(nil)
			if err != nil {
				return errors.Wrap(err, "failed to get value")
			}

			sortValues := make([]any, len(listOpts.Sorters))
			for i, sorter := range listOpts.Sorters {
				sortValues[i] = s.getSortValue(sorter.Path, key)
			}
			items = append(items, sortedItem{key: key, data: data, sortValues: sortValues})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Save the size of the last list to use it as a hint for the next list
	s.expectedSize = len(items)

	sorters := listOpts.Sorters
	slices.SortStableFunc(items, func(a, b sortedItem) int {
		for i := range sorters {
			cmp := compareAny(a.sortValues[i], b.sortValues[i])
			if cmp != 0 {
				if sorters[i].Order == store.SortOrderDesc {
					return -cmp
				}
				return cmp
			}
		}
		return 0
	})

	start := 0
	if listOpts.Cursor != "" {
		for idx, item := range items {
			if item.key == listOpts.Cursor {
				start = idx
				break
			}
		}
	}
	end := min(start+listOpts.Limit, len(items))

	result = &store.ListResponse[T]{
		Items: make([]T, end-start),
	}

	s.log.V(1).Info("list sorted", "start", start, "end", end, "size", len(items))
	for i, item := range items[start:end] {
		err := sonic.Unmarshal(item.data, &result.Items[i])
		if err != nil {
			return nil, errors.Wrap(err, "invalid object")
		}
		if result.Links.Self == "" {
			result.Links.Self = item.key
		}
	}
	if end < len(items) {
		result.Links.Next = items[end].key
	} else {
		result.Links.Next = ""
	}
	return result, nil
}

func compareAny(a, b any) int {
	if a == nil && b == nil {
		return 0
	}
	if a == nil {
		return -1
	}
	if b == nil {
		return 1
	}
	switch aVal := a.(type) {
	case string:
		bVal, ok := b.(string)
		if !ok {
			break
		}
		return cmp.Compare(aVal, bVal)
	case int:
		bVal, ok := b.(int)
		if !ok {
			break
		}
		return cmp.Compare(aVal, bVal)
	case float64:
		bVal, ok := b.(float64)
		if !ok {
			break
		}
		return cmp.Compare(aVal, bVal)
	}

	// fallback
	aStr := fmt.Sprint(a)
	bStr := fmt.Sprint(b)
	return cmp.Compare(aStr, bStr)
}
