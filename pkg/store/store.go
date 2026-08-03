package store

import (
	"context"
	"errors"
)

// Copier is a constraint for types that can produce independent copies.
// All sep2 resource types must implement this for immutable store boundaries.
//
// Copy must be deep with respect to any field a caller could mutate: a Copy
// that shares a slice, map or pointer with its receiver breaks the store's
// copy semantics even though it satisfies this constraint.
type Copier[T any] interface {
	Copy() T
}

// ResourceReader is the read-only view of a flat resource collection.
//
// This is the handle a telemetry consumer or an administrative read surface
// should hold. It is the read half of [ResourceStore]; see the package
// documentation for copy semantics and the error contract.
type ResourceReader[T Copier[T]] interface {
	// Get returns the resource with the given id, or ErrNotFound.
	Get(ctx context.Context, id string) (T, error)

	// List returns one page of resources in the order named by opts.Sort.
	// An empty collection is not an error.
	List(ctx context.Context, opts ListOptions) (ListResult[T], error)

	// Count returns the number of resources in the collection.
	Count(ctx context.Context) (uint32, error)
}

// ResourceStore provides generic CRUD and list operations for IEEE 2030.5
// resources held in a flat collection.
//
// Implementations MUST be safe for concurrent use. All reads return
// independent copies: callers may mutate what they receive without affecting
// the store.
type ResourceStore[T Copier[T]] interface {
	ResourceReader[T]

	// Create stores resource under id. It returns ErrAlreadyExists if id is
	// already present, in which case the stored resource is unchanged.
	Create(ctx context.Context, id string, resource T) error

	// Update replaces the resource stored under id. It returns ErrNotFound if
	// id is absent; Update never creates.
	Update(ctx context.Context, id string, resource T) error

	// Delete removes the resource stored under id, or returns ErrNotFound.
	Delete(ctx context.Context, id string) error
}

// ScopedReader is the read-only view of a parent-scoped resource collection,
// the shape most of the 2030.5 resource surface uses: DER resources under a
// device, meter readings under a usage point, readings under a meter reading.
//
// Resource ids are unique within a parent, not across parents. The same id
// under two different parents addresses two different resources.
type ScopedReader[T Copier[T]] interface {
	// Get returns the resource stored under (parentID, id). It returns
	// ErrNotFound when either the parent or the resource is absent.
	Get(ctx context.Context, parentID, id string) (T, error)

	// List returns one page of the resources under parentID, in the order
	// named by opts.Sort. An unknown parent yields an empty result and a nil
	// error, exactly as a known but empty parent does; the two are not
	// distinguished. A transient backend failure is NOT either of those and
	// MUST be reported as an error.
	List(ctx context.Context, parentID string, opts ListOptions) (ListResult[T], error)

	// Count returns the number of resources under parentID. An unknown parent
	// counts zero and is not an error.
	Count(ctx context.Context, parentID string) (uint32, error)

	// HasParent reports whether the store knows parentID.
	//
	// It MUST report true for any parent holding at least one resource.
	// Whether it keeps reporting true after the last resource under that
	// parent is removed is implementation-defined, as is whether a parent
	// that has only ever been read is reported: callers must not infer
	// emptiness from it.
	//
	// It is fallible on purpose. On a durable backend this is a query, not a
	// cheap local map lookup, and a caller that cannot complete the check must
	// fail closed rather than assume absence.
	HasParent(ctx context.Context, parentID string) (bool, error)

	// Parents returns the parent ids the store knows, in ascending
	// lexicographic order. Every parent holding at least one resource MUST
	// appear; an implementation MAY additionally report parents it has
	// materialized but which hold nothing.
	//
	// This exists because a consumer that needs "which devices have DER
	// status" cannot otherwise ask: HasParent only answers for a parent the
	// caller already named, which restricts such a consumer to enumerating
	// parents out of some other store and probing one at a time.
	Parents(ctx context.Context) ([]string, error)
}

// ScopedStore provides CRUD and list operations for resources addressed as
// (parent, id) pairs.
//
// Implementations MUST be safe for concurrent use and return independent
// copies, exactly as [ResourceStore] does.
type ScopedStore[T Copier[T]] interface {
	ScopedReader[T]

	// Create stores resource under (parentID, id), establishing the parent if
	// the implementation requires one. It returns ErrAlreadyExists if id is
	// already present under that parent.
	Create(ctx context.Context, parentID, id string, resource T) error

	// Update replaces the resource stored under (parentID, id). It returns
	// ErrNotFound when either the parent or the resource is absent, and never
	// creates either.
	Update(ctx context.Context, parentID, id string, resource T) error

	// Delete removes the resource stored under (parentID, id), or returns
	// ErrNotFound.
	Delete(ctx context.Context, parentID, id string) error
}

// SortKey selects the order in which [ResourceReader.List] and
// [ScopedReader.List] return resources.
//
// Ordering is part of the contract rather than an implementation detail.
// IEEE 2030.5 section 4.6.1 requires each list resource to be returned in a
// defined order, and the paging parameters address positions within that
// order, so a page number means nothing without it. Today the ordering is an
// emergent property of the in-memory store's sorted key slice: a durable
// backend would be free to return rows in whatever order the query planner
// chose, satisfy every method signature here, and silently fail the list
// ordering requirements (DER-PRG-001 through DER-PRG-004, MTR-GEN-010).
type SortKey uint8

const (
	// SortByIDAsc orders by resource id, ascending in byte order. It is the
	// zero value, so it is what a caller that says nothing about ordering
	// gets, and every implementation MUST support it.
	SortByIDAsc SortKey = iota

	// SortByIDDesc orders by resource id, descending in byte order. An
	// implementation that cannot provide it MUST return ErrUnsupportedSort
	// rather than serve a different order.
	SortByIDDesc
)

// String implements fmt.Stringer.
func (k SortKey) String() string {
	switch k {
	case SortByIDAsc:
		return "id-asc"
	case SortByIDDesc:
		return "id-desc"
	default:
		return "unknown"
	}
}

// ListOptions specifies paging parameters per IEEE 2030.5 section 4.6.2 and
// the ordering they are relative to, per section 4.6.1.
type ListOptions struct {
	Start uint32 // s param: first ordinal position (0-based)
	Limit uint32 // l param: max items to return; 0 returns no items
	After string // a param: return items ordered strictly after this key

	// Sort names the order the page is taken from. The zero value,
	// SortByIDAsc, is the required baseline; Start and After are interpreted
	// relative to it. An unrecognized value yields ErrUnsupportedSort.
	Sort SortKey
}

// ListResult contains a page of results with total count metadata.
type ListResult[T any] struct {
	All     uint32 // total matching resources, independent of paging
	Results uint32 // count returned in this page
	Items   []T
}

// Sentinel errors for store operations. These three, and only these three,
// carry defined meaning; see the package documentation for the full error
// contract. Match with errors.Is: implementations may wrap them.
var (
	ErrNotFound        = errors.New("resource not found")
	ErrAlreadyExists   = errors.New("resource already exists")
	ErrUnsupportedSort = errors.New("unsupported sort key")
)
