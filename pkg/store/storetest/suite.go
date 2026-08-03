package storetest

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/GRIDAPPSD/ieee-2030_5-core-go/pkg/store"
)

// ResourceStoreFactory returns a fresh, empty flat store for one subtest.
type ResourceStoreFactory func(t *testing.T) store.ResourceStore[Resource]

// ScopedStoreFactory returns a fresh, empty parent-scoped store for one
// subtest.
type ScopedStoreFactory func(t *testing.T) store.ScopedStore[Resource]

// res builds a Resource with a distinguishable body and a mutable field.
func res(id, body string) Resource {
	return Resource{ID: id, Body: body, Tags: []string{"tag-" + id}}
}

// ids extracts the ID field of each item, so ordering assertions read as a
// sequence rather than as index arithmetic.
func ids(items []Resource) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.ID)
	}
	return out
}

// seedFlat creates resources a through j in a deliberately shuffled order, so
// that a list assertion tests the store's ordering rather than its insertion
// order.
func seedFlat(t *testing.T, s store.ResourceStore[Resource]) {
	t.Helper()
	ctx := context.Background()
	for _, id := range []string{"d", "a", "j", "c", "b", "i", "e", "h", "f", "g"} {
		if err := s.Create(ctx, id, res(id, "body-"+id)); err != nil {
			t.Fatalf("seed Create(%q): %v", id, err)
		}
	}
}

// RunResourceStoreSuite asserts the behaviours every store.ResourceStore
// implementation must satisfy. Run it against any candidate implementation:
// passing it is what "implements the contract" means.
func RunResourceStoreSuite(t *testing.T, newStore ResourceStoreFactory) {
	t.Helper()
	ctx := context.Background()

	t.Run("get absent returns ErrNotFound and the zero value", func(t *testing.T) {
		s := newStore(t)
		got, err := s.Get(ctx, "nope")
		if !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("Get absent err = %v, want ErrNotFound", err)
		}
		if got.ID != "" || got.Body != "" || got.Tags != nil {
			t.Errorf("Get absent returned %+v, want the zero Resource", got)
		}
	})

	t.Run("create then get returns the stored field values", func(t *testing.T) {
		s := newStore(t)
		if err := s.Create(ctx, "a", res("a", "first")); err != nil {
			t.Fatalf("Create: %v", err)
		}
		got, err := s.Get(ctx, "a")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got.ID != "a" || got.Body != "first" {
			t.Errorf("Get = {ID:%q Body:%q}, want {ID:\"a\" Body:\"first\"}", got.ID, got.Body)
		}
		if !slices.Equal(got.Tags, []string{"tag-a"}) {
			t.Errorf("Get Tags = %v, want [tag-a]", got.Tags)
		}
	})

	t.Run("create duplicate returns ErrAlreadyExists and does not overwrite", func(t *testing.T) {
		s := newStore(t)
		if err := s.Create(ctx, "a", res("a", "first")); err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := s.Create(ctx, "a", res("a", "second")); !errors.Is(err, store.ErrAlreadyExists) {
			t.Fatalf("duplicate Create err = %v, want ErrAlreadyExists", err)
		}
		got, err := s.Get(ctx, "a")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got.Body != "first" {
			t.Errorf("Body = %q after rejected Create, want %q unchanged", got.Body, "first")
		}
		count, err := s.Count(ctx)
		if err != nil {
			t.Fatalf("Count: %v", err)
		}
		if count != 1 {
			t.Errorf("Count = %d after rejected Create, want 1", count)
		}
	})

	t.Run("update absent returns ErrNotFound and does not create", func(t *testing.T) {
		s := newStore(t)
		if err := s.Update(ctx, "a", res("a", "body")); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("Update absent err = %v, want ErrNotFound", err)
		}
		count, err := s.Count(ctx)
		if err != nil {
			t.Fatalf("Count: %v", err)
		}
		if count != 0 {
			t.Errorf("Count = %d after rejected Update, want 0: Update must not create", count)
		}
	})

	t.Run("update replaces the stored field values", func(t *testing.T) {
		s := newStore(t)
		if err := s.Create(ctx, "a", res("a", "first")); err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := s.Update(ctx, "a", Resource{ID: "a", Body: "second", Tags: []string{"x"}}); err != nil {
			t.Fatalf("Update: %v", err)
		}
		got, err := s.Get(ctx, "a")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got.Body != "second" {
			t.Errorf("Body = %q after Update, want %q", got.Body, "second")
		}
		if !slices.Equal(got.Tags, []string{"x"}) {
			t.Errorf("Tags = %v after Update, want [x]", got.Tags)
		}
	})

	t.Run("delete removes the resource and is not idempotent", func(t *testing.T) {
		s := newStore(t)
		if err := s.Create(ctx, "a", res("a", "body")); err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := s.Delete(ctx, "a"); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if _, err := s.Get(ctx, "a"); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("Get after Delete err = %v, want ErrNotFound", err)
		}
		if err := s.Delete(ctx, "a"); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("second Delete err = %v, want ErrNotFound", err)
		}
	})

	t.Run("stored values are independent copies", func(t *testing.T) {
		s := newStore(t)
		written := res("a", "body")
		if err := s.Create(ctx, "a", written); err != nil {
			t.Fatalf("Create: %v", err)
		}

		// Mutating what the caller passed in must not reach into the store.
		written.Tags[0] = "mutated-by-writer"

		got, err := s.Get(ctx, "a")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got.Tags[0] != "tag-a" {
			t.Errorf("Tags[0] = %q, want %q: the store aliased the caller's slice on write", got.Tags[0], "tag-a")
		}

		// Mutating what the caller received must not reach into the store.
		got.Tags[0] = "mutated-by-reader"
		again, err := s.Get(ctx, "a")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if again.Tags[0] != "tag-a" {
			t.Errorf("Tags[0] = %q, want %q: the store aliased its own slice on read", again.Tags[0], "tag-a")
		}
	})

	t.Run("list on an empty store is empty and not an error", func(t *testing.T) {
		s := newStore(t)
		result, err := s.List(ctx, store.ListOptions{Limit: 10})
		if err != nil {
			t.Fatalf("List on empty store err = %v, want nil: empty is not an error", err)
		}
		if result.All != 0 || result.Results != 0 || len(result.Items) != 0 {
			t.Errorf("List on empty store = %+v, want all-zero", result)
		}
	})

	t.Run("list ordering and paging", func(t *testing.T) {
		asc := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"}
		desc := slices.Clone(asc)
		slices.Reverse(desc)

		tests := []struct {
			name string
			opts store.ListOptions
			want []string
		}{
			{"default order is ascending by id", store.ListOptions{Limit: 10}, asc},
			{"explicit ascending", store.ListOptions{Limit: 10, Sort: store.SortByIDAsc}, asc},
			{"descending", store.ListOptions{Limit: 10, Sort: store.SortByIDDesc}, desc},
			{"limit truncates in order", store.ListOptions{Limit: 3}, []string{"a", "b", "c"}},
			{"limit truncates in descending order", store.ListOptions{Limit: 3, Sort: store.SortByIDDesc}, []string{"j", "i", "h"}},
			{"start offsets within the order", store.ListOptions{Start: 5, Limit: 3}, []string{"f", "g", "h"}},
			{"start past the end yields nothing", store.ListOptions{Start: 20, Limit: 5}, nil},
			{"limit zero yields nothing", store.ListOptions{Limit: 0}, nil},
			{"after resumes at the next key", store.ListOptions{After: "c", Limit: 3}, []string{"d", "e", "f"}},
			{"after is relative to descending order", store.ListOptions{After: "c", Limit: 3, Sort: store.SortByIDDesc}, []string{"b", "a"}},
			{"after an absent key starts at the next present one", store.ListOptions{After: "cc", Limit: 2}, []string{"d", "e"}},
			{"after the last key yields nothing", store.ListOptions{After: "j", Limit: 5}, nil},
			{"after composes with start", store.ListOptions{After: "c", Start: 2, Limit: 2}, []string{"f", "g"}},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				s := newStore(t)
				seedFlat(t, s)

				result, err := s.List(ctx, tc.opts)
				if err != nil {
					t.Fatalf("List(%+v): %v", tc.opts, err)
				}
				if got := ids(result.Items); !slices.Equal(got, tc.want) {
					t.Errorf("List(%+v) items = %v, want %v", tc.opts, got, tc.want)
				}
				if result.Results != uint32(len(tc.want)) {
					t.Errorf("Results = %d, want %d", result.Results, len(tc.want))
				}
				// All reports the total, never the page size.
				if result.All != 10 {
					t.Errorf("All = %d, want 10 regardless of paging", result.All)
				}
			})
		}
	})

	t.Run("list rejects an unrecognized sort key", func(t *testing.T) {
		s := newStore(t)
		seedFlat(t, s)

		result, err := s.List(ctx, store.ListOptions{Limit: 10, Sort: store.SortKey(200)})
		if !errors.Is(err, store.ErrUnsupportedSort) {
			t.Fatalf("List with unknown sort err = %v, want ErrUnsupportedSort: "+
				"serving the default order instead would page a sequence the caller did not ask for", err)
		}
		if len(result.Items) != 0 || result.Results != 0 {
			t.Errorf("rejected List returned %+v, want no items", result)
		}
	})

	t.Run("count tracks creates and deletes", func(t *testing.T) {
		s := newStore(t)
		seedFlat(t, s)

		count, err := s.Count(ctx)
		if err != nil {
			t.Fatalf("Count: %v", err)
		}
		if count != 10 {
			t.Fatalf("Count = %d, want 10", count)
		}
		if err := s.Delete(ctx, "a"); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		count, err = s.Count(ctx)
		if err != nil {
			t.Fatalf("Count: %v", err)
		}
		if count != 9 {
			t.Errorf("Count after Delete = %d, want 9", count)
		}
	})
}

// RunScopedStoreSuite asserts the behaviours every store.ScopedStore
// implementation must satisfy.
func RunScopedStoreSuite(t *testing.T, newStore ScopedStoreFactory) {
	t.Helper()
	ctx := context.Background()

	t.Run("get under an unknown parent returns ErrNotFound", func(t *testing.T) {
		s := newStore(t)
		if _, err := s.Get(ctx, "nobody", "a"); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("Get under unknown parent err = %v, want ErrNotFound", err)
		}
	})

	t.Run("create then get returns the stored field values", func(t *testing.T) {
		s := newStore(t)
		if err := s.Create(ctx, "p1", "a", res("a", "first")); err != nil {
			t.Fatalf("Create: %v", err)
		}
		got, err := s.Get(ctx, "p1", "a")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got.ID != "a" || got.Body != "first" {
			t.Errorf("Get = {ID:%q Body:%q}, want {ID:\"a\" Body:\"first\"}", got.ID, got.Body)
		}
	})

	t.Run("parents are isolated from each other", func(t *testing.T) {
		s := newStore(t)
		if err := s.Create(ctx, "p1", "a", res("a", "under-p1")); err != nil {
			t.Fatalf("Create p1: %v", err)
		}
		// The same id under a different parent is a different resource, so
		// this must not collide.
		if err := s.Create(ctx, "p2", "a", res("a", "under-p2")); err != nil {
			t.Fatalf("Create p2 with the same id: %v, want success", err)
		}

		got1, err := s.Get(ctx, "p1", "a")
		if err != nil {
			t.Fatalf("Get p1: %v", err)
		}
		got2, err := s.Get(ctx, "p2", "a")
		if err != nil {
			t.Fatalf("Get p2: %v", err)
		}
		if got1.Body != "under-p1" || got2.Body != "under-p2" {
			t.Errorf("bodies = %q / %q, want under-p1 / under-p2: parent scoping leaked", got1.Body, got2.Body)
		}

		if err := s.Delete(ctx, "p1", "a"); err != nil {
			t.Fatalf("Delete p1: %v", err)
		}
		if _, err := s.Get(ctx, "p2", "a"); err != nil {
			t.Errorf("Get p2 after deleting p1 err = %v, want nil: delete crossed the parent boundary", err)
		}
	})

	t.Run("update replaces the stored field values", func(t *testing.T) {
		s := newStore(t)
		if err := s.Create(ctx, "p1", "a", res("a", "first")); err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := s.Update(ctx, "p1", "a", Resource{ID: "a", Body: "second"}); err != nil {
			t.Fatalf("Update: %v", err)
		}
		got, err := s.Get(ctx, "p1", "a")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got.Body != "second" {
			t.Errorf("Body = %q after Update, want %q", got.Body, "second")
		}
	})

	t.Run("update under an unknown parent returns ErrNotFound and does not create", func(t *testing.T) {
		s := newStore(t)
		if err := s.Update(ctx, "nobody", "a", res("a", "body")); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("Update under unknown parent err = %v, want ErrNotFound", err)
		}
		if _, err := s.Get(ctx, "nobody", "a"); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("Get after rejected Update err = %v, want ErrNotFound: Update must not create", err)
		}
	})

	t.Run("update of an unknown id under a known parent returns ErrNotFound", func(t *testing.T) {
		s := newStore(t)
		if err := s.Create(ctx, "p1", "a", res("a", "body")); err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := s.Update(ctx, "p1", "b", res("b", "body")); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("Update unknown id err = %v, want ErrNotFound", err)
		}
	})

	t.Run("create duplicate under the same parent returns ErrAlreadyExists", func(t *testing.T) {
		s := newStore(t)
		if err := s.Create(ctx, "p1", "a", res("a", "first")); err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := s.Create(ctx, "p1", "a", res("a", "second")); !errors.Is(err, store.ErrAlreadyExists) {
			t.Fatalf("duplicate Create err = %v, want ErrAlreadyExists", err)
		}
		got, err := s.Get(ctx, "p1", "a")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got.Body != "first" {
			t.Errorf("Body = %q after rejected Create, want %q unchanged", got.Body, "first")
		}
	})

	t.Run("delete under an unknown parent returns ErrNotFound", func(t *testing.T) {
		s := newStore(t)
		if err := s.Delete(ctx, "nobody", "a"); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("Delete under unknown parent err = %v, want ErrNotFound", err)
		}
	})

	t.Run("list under an unknown parent is empty and not an error", func(t *testing.T) {
		s := newStore(t)
		result, err := s.List(ctx, "nobody", store.ListOptions{Limit: 10})
		if err != nil {
			t.Fatalf("List under unknown parent err = %v, want nil", err)
		}
		if result.All != 0 || result.Results != 0 || len(result.Items) != 0 {
			t.Errorf("List under unknown parent = %+v, want all-zero", result)
		}
	})

	t.Run("count under an unknown parent is zero and not an error", func(t *testing.T) {
		s := newStore(t)
		count, err := s.Count(ctx, "nobody")
		if err != nil {
			t.Fatalf("Count under unknown parent err = %v, want nil", err)
		}
		if count != 0 {
			t.Errorf("Count under unknown parent = %d, want 0", count)
		}
	})

	t.Run("has parent reports true for a parent holding a resource", func(t *testing.T) {
		s := newStore(t)
		if err := s.Create(ctx, "p1", "a", res("a", "body")); err != nil {
			t.Fatalf("Create: %v", err)
		}

		ok, err := s.HasParent(ctx, "p1")
		if err != nil {
			t.Fatalf("HasParent: %v", err)
		}
		if !ok {
			t.Errorf("HasParent(p1) = false, want true: p1 holds a resource")
		}

		// A parent that was never written must not be reported. The contract
		// leaves the post-delete and read-only cases implementation-defined,
		// so neither is asserted here.
		ok, err = s.HasParent(ctx, "never-written")
		if err != nil {
			t.Fatalf("HasParent: %v", err)
		}
		if ok {
			t.Errorf("HasParent(never-written) = true, want false")
		}
	})

	t.Run("parents enumerates every parent holding a resource, in order", func(t *testing.T) {
		s := newStore(t)
		for _, parent := range []string{"p3", "p1", "p2"} {
			if err := s.Create(ctx, parent, "a", res("a", "body")); err != nil {
				t.Fatalf("Create under %q: %v", parent, err)
			}
		}

		parents, err := s.Parents(ctx)
		if err != nil {
			t.Fatalf("Parents: %v", err)
		}
		// Every parent holding a resource must appear, in ascending order.
		// An implementation may report additional materialized-but-empty
		// parents, so this checks containment and ordering, not equality.
		for _, want := range []string{"p1", "p2", "p3"} {
			if !slices.Contains(parents, want) {
				t.Errorf("Parents = %v, missing %q which holds a resource", parents, want)
			}
		}
		if !slices.IsSorted(parents) {
			t.Errorf("Parents = %v, want ascending order", parents)
		}
		if slices.Contains(parents, "never-written") {
			t.Errorf("Parents = %v, contains a parent that was never written", parents)
		}
	})

	t.Run("stored values are independent copies", func(t *testing.T) {
		s := newStore(t)
		written := res("a", "body")
		if err := s.Create(ctx, "p1", "a", written); err != nil {
			t.Fatalf("Create: %v", err)
		}
		written.Tags[0] = "mutated-by-writer"

		got, err := s.Get(ctx, "p1", "a")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got.Tags[0] != "tag-a" {
			t.Errorf("Tags[0] = %q, want %q: the store aliased the caller's slice on write", got.Tags[0], "tag-a")
		}

		got.Tags[0] = "mutated-by-reader"
		again, err := s.Get(ctx, "p1", "a")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if again.Tags[0] != "tag-a" {
			t.Errorf("Tags[0] = %q, want %q: the store aliased its own slice on read", again.Tags[0], "tag-a")
		}
	})

	t.Run("list ordering and paging under a parent", func(t *testing.T) {
		asc := []string{"a", "b", "c", "d", "e"}
		desc := slices.Clone(asc)
		slices.Reverse(desc)

		tests := []struct {
			name string
			opts store.ListOptions
			want []string
		}{
			{"default order is ascending by id", store.ListOptions{Limit: 10}, asc},
			{"descending", store.ListOptions{Limit: 10, Sort: store.SortByIDDesc}, desc},
			{"limit truncates in order", store.ListOptions{Limit: 2}, []string{"a", "b"}},
			{"start offsets within the order", store.ListOptions{Start: 2, Limit: 2}, []string{"c", "d"}},
			{"after resumes at the next key", store.ListOptions{After: "b", Limit: 2}, []string{"c", "d"}},
			{"limit zero yields nothing", store.ListOptions{Limit: 0}, nil},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				s := newStore(t)
				// Shuffled insertion, and a second parent whose resources must
				// never appear in p1's page.
				for _, id := range []string{"c", "e", "a", "d", "b"} {
					if err := s.Create(ctx, "p1", id, res(id, "body-"+id)); err != nil {
						t.Fatalf("Create: %v", err)
					}
					if err := s.Create(ctx, "p2", "other-"+id, res("other-"+id, "body")); err != nil {
						t.Fatalf("Create under p2: %v", err)
					}
				}

				result, err := s.List(ctx, "p1", tc.opts)
				if err != nil {
					t.Fatalf("List(%+v): %v", tc.opts, err)
				}
				if got := ids(result.Items); !slices.Equal(got, tc.want) {
					t.Errorf("List(%+v) items = %v, want %v", tc.opts, got, tc.want)
				}
				if result.Results != uint32(len(tc.want)) {
					t.Errorf("Results = %d, want %d", result.Results, len(tc.want))
				}
				if result.All != 5 {
					t.Errorf("All = %d, want 5: only p1's resources count", result.All)
				}
			})
		}
	})

	t.Run("list rejects an unrecognized sort key", func(t *testing.T) {
		s := newStore(t)
		if err := s.Create(ctx, "p1", "a", res("a", "body")); err != nil {
			t.Fatalf("Create: %v", err)
		}
		result, err := s.List(ctx, "p1", store.ListOptions{Limit: 10, Sort: store.SortKey(200)})
		if !errors.Is(err, store.ErrUnsupportedSort) {
			t.Fatalf("List with unknown sort err = %v, want ErrUnsupportedSort", err)
		}
		if len(result.Items) != 0 {
			t.Errorf("rejected List returned %d items, want none", len(result.Items))
		}
	})

	t.Run("count reflects only the named parent", func(t *testing.T) {
		s := newStore(t)
		if err := s.Create(ctx, "p1", "a", res("a", "body")); err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := s.Create(ctx, "p1", "b", res("b", "body")); err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := s.Create(ctx, "p2", "a", res("a", "body")); err != nil {
			t.Fatalf("Create: %v", err)
		}

		count, err := s.Count(ctx, "p1")
		if err != nil {
			t.Fatalf("Count: %v", err)
		}
		if count != 2 {
			t.Errorf("Count(p1) = %d, want 2", count)
		}
	})
}

// RunTransientFailureSuite asserts the single most consequential property of
// the error contract: that a backend failure is distinguishable from an empty
// store.
//
// The two factories must return stores that are equivalent except that the
// second one's backend is unavailable. newEmpty returns a healthy store with
// no resources; newFailing returns one whose every operation fails.
//
// This is the case the contract exists to prevent. The value channel is
// identical between the two: a failing List returns the same zero-valued
// ListResult that a legitimately empty collection returns, so a caller that
// drops the error cannot tell them apart and answers 200 with an empty list
// for what is really a 500. The error return is the only discriminator, and
// these assertions pin that it actually discriminates.
func RunTransientFailureSuite(t *testing.T, newEmpty ScopedStoreFactory, newFailing ScopedStoreFactory) {
	t.Helper()
	ctx := context.Background()

	t.Run("a failing list is not reported as an empty list", func(t *testing.T) {
		emptyResult, emptyErr := newEmpty(t).List(ctx, "p1", store.ListOptions{Limit: 10})
		if emptyErr != nil {
			t.Fatalf("List on healthy empty store err = %v, want nil", emptyErr)
		}

		failResult, failErr := newFailing(t).List(ctx, "p1", store.ListOptions{Limit: 10})
		if failErr == nil {
			t.Fatal("List on failing store err = nil, want an error: " +
				"a lookup that could not be completed was reported as an empty collection")
		}

		// The failure must not masquerade as either sentinel: ErrNotFound
		// would become a 404 and ErrAlreadyExists is nonsense here. Anything
		// else must reach the caller as a 500.
		if errors.Is(failErr, store.ErrNotFound) {
			t.Errorf("failing List err = %v, must not be ErrNotFound: a transient failure is not a 404", failErr)
		}
		if errors.Is(failErr, store.ErrAlreadyExists) {
			t.Errorf("failing List err = %v, must not be ErrAlreadyExists", failErr)
		}

		// The point of the assertion: the values are identical, so only err
		// carries the difference.
		if failResult.All != emptyResult.All || failResult.Results != emptyResult.Results || len(failResult.Items) != len(emptyResult.Items) {
			t.Logf("note: failing result %+v differs from empty result %+v", failResult, emptyResult)
		}
		if failResult.Results != 0 || len(failResult.Items) != 0 {
			t.Errorf("failing List returned %d items, want none alongside the error", len(failResult.Items))
		}
	})

	t.Run("a failing get is not reported as not found", func(t *testing.T) {
		_, err := newFailing(t).Get(ctx, "p1", "a")
		if err == nil {
			t.Fatal("Get on failing store err = nil, want an error")
		}
		if errors.Is(err, store.ErrNotFound) {
			t.Errorf("failing Get err = %v, must not be ErrNotFound: "+
				"a backend that could not answer has not established that the resource is absent", err)
		}
	})

	t.Run("a failing count is not reported as zero", func(t *testing.T) {
		count, err := newFailing(t).Count(ctx, "p1")
		if err == nil {
			t.Fatal("Count on failing store err = nil, want an error")
		}
		if count != 0 {
			t.Errorf("failing Count = %d, want 0 alongside the error", count)
		}
	})

	t.Run("a failing parent check is not reported as absent", func(t *testing.T) {
		ok, err := newFailing(t).HasParent(ctx, "p1")
		if err == nil {
			t.Fatal("HasParent on failing store err = nil, want an error")
		}
		if ok {
			t.Errorf("failing HasParent = true, want false alongside the error: fail closed")
		}
		if errors.Is(err, store.ErrNotFound) {
			t.Errorf("failing HasParent err = %v, must not be ErrNotFound", err)
		}
	})

	t.Run("a failing parent enumeration is not reported as no parents", func(t *testing.T) {
		parents, err := newFailing(t).Parents(ctx)
		if err == nil {
			t.Fatal("Parents on failing store err = nil, want an error")
		}
		if len(parents) != 0 {
			t.Errorf("failing Parents = %v, want none alongside the error", parents)
		}
	})

	t.Run("a failing write is not reported as success", func(t *testing.T) {
		s := newFailing(t)
		if err := s.Create(ctx, "p1", "a", res("a", "body")); err == nil {
			t.Error("Create on failing store err = nil, want an error")
		}
		if err := s.Update(ctx, "p1", "a", res("a", "body")); err == nil {
			t.Error("Update on failing store err = nil, want an error")
		}
		if err := s.Delete(ctx, "p1", "a"); err == nil {
			t.Error("Delete on failing store err = nil, want an error")
		}
	})
}
