package memory

import (
	"context"
	"slices"
	"sync"

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/store"
)

// ScopedStore manages per-parent resource stores.
// For example, MeterReadings scoped under UsagePoints, or Readings
// scoped under MeterReadings. Each parent ID gets its own Store[T].
type ScopedStore[T store.Copier[T]] struct {
	mu     sync.RWMutex
	stores map[string]*Store[T]
}

// NewScopedStore creates a new ScopedStore.
func NewScopedStore[T store.Copier[T]]() *ScopedStore[T] {
	return &ScopedStore[T]{
		stores: make(map[string]*Store[T]),
	}
}

// ForParent returns the Store for the given parent ID, creating it if needed.
//
// ForParent is deliberately NOT part of store.ScopedStore. It returns a
// concrete type, and it materializes a parent bucket on read, which no durable
// backend can implement sensibly and which lets an arbitrary path segment
// allocate. It stays here as an implementation convenience for existing call
// sites; new code should use the contract methods.
func (s *ScopedStore[T]) ForParent(parentID string) *Store[T] {
	s.mu.RLock()
	st, ok := s.stores[parentID]
	s.mu.RUnlock()
	if ok {
		return st
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Double-check after acquiring write lock
	if st, ok := s.stores[parentID]; ok {
		return st
	}

	st = NewStore[T]()
	s.stores[parentID] = st
	return st
}

// HasParent reports whether a store exists for the given parent ID.
//
// The error return is always nil here because the lookup is a local map read,
// but it is part of store.ScopedReader so that a durable implementation, where
// this is a query that can fail, can report that failure instead of returning
// a bare false that a caller would read as "absent".
//
// Note that ForParent materializes a bucket, so a parent that has only ever
// been read is reported present. Callers must not infer emptiness from this.
func (s *ScopedStore[T]) HasParent(_ context.Context, parentID string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.stores[parentID]
	return ok, nil
}

// Parents returns the known parent IDs in ascending order.
//
// The result includes parents materialized by ForParent that hold no
// resources, per the store.ScopedReader contract, which requires only that
// every parent holding at least one resource appears.
func (s *ScopedStore[T]) Parents(_ context.Context) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	parents := make([]string, 0, len(s.stores))
	for parentID := range s.stores {
		parents = append(parents, parentID)
	}
	slices.Sort(parents)
	return parents, nil
}

// Get retrieves a resource from a parent's store.
func (s *ScopedStore[T]) Get(ctx context.Context, parentID, id string) (T, error) {
	return s.ForParent(parentID).Get(ctx, id)
}

// List lists resources from a parent's store with paging.
func (s *ScopedStore[T]) List(ctx context.Context, parentID string, opts store.ListOptions) (store.ListResult[T], error) {
	return s.ForParent(parentID).List(ctx, opts)
}

// Create adds a resource to a parent's store.
func (s *ScopedStore[T]) Create(ctx context.Context, parentID, id string, resource T) error {
	return s.ForParent(parentID).Create(ctx, id, resource)
}

// Update replaces a resource in a parent's store.
//
// Unlike the other scoped operations, Update does not go through ForParent: an
// update against an unknown parent reports ErrNotFound rather than
// materializing an empty bucket for it. Update never creates, at either level.
func (s *ScopedStore[T]) Update(ctx context.Context, parentID, id string, resource T) error {
	s.mu.RLock()
	st, ok := s.stores[parentID]
	s.mu.RUnlock()
	if !ok {
		return store.ErrNotFound
	}
	return st.Update(ctx, id, resource)
}

// Delete removes a resource from a parent's store.
func (s *ScopedStore[T]) Delete(ctx context.Context, parentID, id string) error {
	return s.ForParent(parentID).Delete(ctx, id)
}

// Count returns the number of resources in a parent's store.
func (s *ScopedStore[T]) Count(ctx context.Context, parentID string) (uint32, error) {
	return s.ForParent(parentID).Count(ctx)
}

// DeleteParent removes a parent's whole collection and reports how many
// resources went with it.
//
// It exists for CASCADE deletion: when the parent resource is deleted, the
// children scoped under it have to go too, or the collection outlives the only
// thing that could address it. Deleting the parent and leaving its children is
// not a tidiness problem, it is a correctness one: ForParent materialises a
// bucket on read, so the orphaned collection stays reachable to any code that
// names the same parent id, and a later resource created under a RE-USED id
// would inherit the dead parent's children as if they were its own.
//
// It is one operation rather than a list-then-delete loop for two reasons.
// The store exposes no way to enumerate KEYS, only values, so a loop would have
// to recover each child's id from a field of the child itself, which makes the
// cascade depend on a value a writer controls rather than on the key the store
// is actually indexed by. And a loop is not atomic: a child created between the
// list and the last delete survives the cascade and is orphaned by exactly the
// operation meant to prevent orphans.
//
// An absent parent is not an error. "No bucket" and "an empty bucket" are the
// same fact for a caller asking that nothing be left behind, and there is
// nothing for a cascade to fail at when there is nothing there; reporting
// ErrNotFound would make every caller special-case the case where its work is
// already done. The count distinguishes them for a caller that wants to log
// what it removed.
//
// The bucket is detached under the write lock BEFORE it is counted, so no
// further write can reach it through this ScopedStore and the count cannot
// disagree with what was removed. A writer that already holds the concrete
// *Store from ForParent can still write into the detached bucket, and that
// write is lost; this store offers no transaction that could prevent it, and
// the alternative of holding the parent lock across the child store's lock for
// the whole operation would invert the lock order the rest of this type uses.
func (s *ScopedStore[T]) DeleteParent(ctx context.Context, parentID string) (uint32, error) {
	s.mu.Lock()
	st, ok := s.stores[parentID]
	if ok {
		delete(s.stores, parentID)
	}
	s.mu.Unlock()

	if !ok {
		return 0, nil
	}
	return st.Count(ctx)
}
