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
