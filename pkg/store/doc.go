// Package store defines the contract by which resource state attaches to an
// IEEE 2030.5 server.
//
// # This package is server-side state and is destined to move
//
// A resource store is server state. A 2030.5 client has no store: it issues
// requests and parses wire values. Under the four-layer target architecture
// (core is the shared library for client and server, server-go is the server,
// the bridge grafts onto the server), this package and its in-memory
// implementation belong in server-go, and they live in core today only because
// the server currently does. Nothing here is part of the shared
// client-and-server surface, and callers should not treat it as such.
//
// The relocation is sequenced with the layering split, not with this contract.
// See the bridge/core boundary analysis (architecture decision IEEECORE-068)
// for the reasoning. Until the split lands, keep external dependencies on this
// package at zero so that the move stays a move rather than a breaking API
// change for three downstream modules.
//
// # The contract
//
// Four interfaces cross two axes, privilege and scope:
//
//   - [ResourceReader] is a flat collection, read-only.
//   - [ResourceStore] is a flat collection, read-write.
//   - [ScopedReader] is a parent-scoped collection, read-only.
//   - [ScopedStore] is a parent-scoped collection, read-write.
//
// The read/write split is load-bearing rather than decorative. A telemetry
// consumer and an administrative read surface need reads only, and handing
// either one a full store is over-privilege. Taking a reader handle makes
// "this path only reads" structural rather than conventional.
//
// The parent-scoped shape is the majority shape in 2030.5: DER resources,
// meter readings and function set assignments are all addressed as
// (parent, id) pairs.
//
// Deliberately absent from the contract is any equivalent of the in-memory
// implementation's ForParent, which returns a concrete per-parent store and
// materializes one on demand. Create-on-read is not implementable on a durable
// backend and turns a GET for an arbitrary path segment into an allocation.
// It remains available on the in-memory type as an implementation
// convenience; it is not part of the contract.
//
// # Copy semantics
//
// Implementations MUST be safe for concurrent use. Every value crossing the
// store boundary in either direction is an independent copy: a caller may
// mutate a resource it received, or one it passed to Create or Update, without
// affecting stored state. This is what the [Copier] constraint exists to
// guarantee, and it extends to reference-typed fields: a Copy implementation
// that shares a slice or map with its receiver does not satisfy it.
//
// # Error contract
//
// This contract is stated rather than implied because the in-memory
// implementation essentially cannot fail, so callers written against it have
// never met a fallible backend.
//
// Exactly three errors carry defined meaning:
//
//   - [ErrNotFound] means the addressed resource, or its parent, is absent.
//     On a request path this is a 404.
//   - [ErrAlreadyExists] means Create was called for an id already present.
//     The stored resource is unchanged.
//   - [ErrUnsupportedSort] means the requested [SortKey] is not one this
//     implementation can provide. It is never returned in place of silently
//     serving a different order.
//
// Match them with [errors.Is]: an implementation may wrap them with context.
//
// ANY other non-nil error is transient or unknown. A caller MUST surface it,
// which on a request path means a 500. It MUST NOT be flattened into
// not-found, and it MUST NOT be flattened into an empty result. An empty list
// and a failed lookup are different answers, and the error return is the only
// channel that distinguishes them: a failing List returns the same zero-valued
// [ListResult] that a legitimately empty collection does. Answering 200 with
// an empty list because a durable backend was briefly unreachable is a silent
// wrong answer on the wire.
//
// A caller that cannot complete a check MUST fail closed.
//
// # Partial failure
//
// The contract defines no batch or transactional primitive. A caller writing
// several resources, such as a seeding pass over a fleet, may therefore
// half-fail: resources written before the error stay written. Callers that
// need all-or-nothing must implement it themselves, and callers that do not
// must be safe to re-run.
package store
