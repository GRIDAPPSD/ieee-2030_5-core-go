# Releasing

This is the canonical release runbook for the IEEE 2030.5 Go family:
`ieee-2030_5-core-go`, `ieee-2030_5-server-go`, `ieee-2030_5-client-go`, and
`gridappsd-ieee-2030_5-go` (the bridge). It lives here, in core, and not as
four separate copies. Reasons:

- The release chain is inherently cross-repo: core is released first, then
  each consumer bumps its own pin on its own schedule. A shared doc keeps
  that chain visible in one place instead of scattered across four.
- Core is the shared layer in the target four-layer architecture, and two of
  its packages (`pkg/store` and `pkg/sep2srv`) are slated to move to
  server-go. When that move happens, the release process described here
  moves with the code rather than needing to be rewritten in a second file
  that was never kept in sync with the first.
- The hardest-won material in this document (the schema/WADL licensing
  constraint, the history rewrite, the tag-dereference gotcha, the consumer
  chain not propagating on its own) applies to all four repos, verbatim or
  near enough. Four copies of one process drift; a canonical doc plus three
  short pointers does not.

Each of the other three repos should carry a short `RELEASING.md` stub that
points here and states only what is repo-specific. See "What the other three
repos need" near the end. Those stub files do not exist yet; writing them was
out of scope for this pass (see the report this document shipped with).

## 0. Four repos, three release shapes

- **core, server-go, client-go**: Go libraries. No build artifact, no
  `release.yml` workflow (verified: none of the three has a workflow file
  matching `*release*` as of this writing). A release is a tag plus a
  `gh release create`, cut by hand following this document. Consumers `go
  get` the module; there is nothing to attach.
- **gridappsd-ieee-2030_5-go (the bridge)**: a Go binary (`cmd/bridge`). It
  has its own tag-driven `.github/workflows/release.yml` that builds,
  version-stamps, and canary-smoke-tests the binary on a `v*` tag push, with
  a dry-run-safe default (an accidental manual dispatch rehearses the
  pipeline without publishing). That workflow is not described in detail
  here; this document's manual steps below apply to the library repos. The
  bridge's own `RELEASING.md` stub should describe how the two interact
  rather than restate the library process.

## 1. Versioning

All four modules are 0.x. Per semver's 0.x carve-out, MINOR carries any
change that is not a strict patch, including a breaking one; MAJOR is not in
play until a 1.0 commitment is made.

Two examples from core, both correctly MINOR:

- `v0.11.0` was additive (a new `pkg/store` contract; no consumer required a
  change).
- `v0.10.0` carried a genuinely breaking type change
  (`DERStatus.StateOfChargeStatus` from `*uint16` to a complexType) and was
  still MINOR, because the project has not yet committed to 1.0 semantics.

## 2. Verification gates before tagging

Run, in order:

```
go build ./...
go vet ./...
go test ./...
gofmt -l .
```

Two things a releaser who has not read this section will get wrong:

- **Schema-gated tests SKIP unless `SEP2_SCHEMA_PATH` is set** (core, and
  anywhere else that gates against `sep.xsd`). A green run with visible
  skips is the correct, expected result for a releaser who does not have a
  licensed copy of the IEEE schema on hand. Do not go hunting for a copy to
  force the gate open; `go test ./...` staying green with skips reported is
  not a broken suite.
- **`SEP2_WADL_PATH` is the same pattern in server-go**, gating the WADL
  conformance sweep in `test/conformance/wadl`. Same rule: unset means skip,
  and skip is correct.

To positively confirm the gate ran rather than skipped (CI does this; a
release does not strictly require it, but it is the stronger check), set
`SEP2_SCHEMA_REQUIRED=1` (core) or run the WADL sweep with a copy in place
(server-go). An absent or misconfigured schema then fails the run instead of
silently skipping.

## 3. The licensing constraint (absolute, no exceptions)

Neither `sep.xsd` (IEEE 2030.5 normative XML Schema) nor `sep_wadl.xml`
(IEEE 2030.5 normative WADL) may ever appear in a release artifact, in a
commit, or in any binary this project builds. Neither is committed to any of
these repos; both are supplied at test time via environment variable
(`SEP2_SCHEMA_PATH`, `SEP2_WADL_PATH`) and, in CI, reassembled from split
repository secrets outside the checkout. See each repo's `NOTICE` file for
the full rationale.

Before releasing, verify the tagged tree contains neither file:

```
git show vX.Y.Z -- '**/sep.xsd' '**/sep_wadl.xml'
git ls-tree -r vX.Y.Z --name-only | grep -iE 'sep\.xsd|sep_wadl\.xml'
```

Both commands should come back empty. This is not a formality: a security
classifier correctly flagged a release on 2026-08-02 for exactly this
reason, and the artifact was then verified clean. Treat that flag as the
system working as intended, not as a false alarm to route around.

## 4. History was rewritten on core, 2026-08-02

Core's history was rewritten to purge the vendored schema that predated the
`SEP2_SCHEMA_PATH` design in section 3. Consequences that must be stated to
anyone consuming core tags:

- Tags `v0.7.0` through `v0.9.0` point at **different commits** than they
  originally did.
- `v0.6.0` is unchanged.
- Everything from `v0.10.0` onward is clean (post-rewrite).

**Never force-push, never move or re-cut an existing tag, never run a
history operation on any of these four repos.** The rewrite already
happened once, deliberately, as a one-time licensing remediation; it is not
a precedent for routine use.

A pre-rewrite backup mirror exists at
`/home/debian/repos/backup-core-pre-rewrite-2026-08-02.git`. It deliberately
contains material that may not be redistributed. Never push to it, copy from
it, or quote its contents. Its only purpose is disaster recovery in the
hands of someone who already knows what it holds.

## 5. Repository visibility

All four repos are **private**, and publication (flipping to public) is
blocked pending a GitHub Support purge of stale pull-request refs that still
carry the schema. A GitHub Release on a private repo works fine and is not
blocked by this. **Never change repository visibility as part of a release**
regardless of how routine the release feels; visibility is a separate,
support-gated decision.

## 6. Cutting a release

1. Confirm authorization (section 10).
2. Confirm the verification gates in section 2 are green on the commit you
   intend to tag.
3. Confirm the licensing check in section 3 on that same tree.
4. Tag:
   ```
   git tag -a vX.Y.Z -m "vX.Y.Z"
   git push origin vX.Y.Z
   ```
   A lightweight tag (`git tag vX.Y.Z`, no `-a`) is also acceptable; the
   GitHub release in the next step is what makes it a full release either
   way. Core's own tag history mixes both kinds (see section 7's
   dereference note), so do not treat a lightweight tag as irregular.
5. `gh release create vX.Y.Z --notes-file <path>` with notes structured per
   section 8. Do not attach a build artifact for a library repo (see section
   0); there is nothing to attach.
6. Run the post-release verification in section 7 before telling anyone the
   release is done.

## 7. Post-release verification (mandatory, in this order)

1. **Tag dereferences to the intended commit.**
   ```
   git rev-parse vX.Y.Z^{commit}
   ```
   Use `^{commit}`, not a bare `git rev-parse vX.Y.Z`. Core's tags are a mix
   of annotated and lightweight: `v0.11.0` is an annotated tag object, and a
   bare `git rev-parse v0.11.0` returns the *tag object's own SHA*
   (`c73d4ad6...` as of this writing), not the commit it points at
   (`c70a6345...`). `v0.10.0` is a lightweight tag, where the two SHAs
   happen to be the same thing, which is what makes the annotated case easy
   to miss if you only ever tested against a lightweight tag. `^{commit}`
   dereferences either kind uniformly and is the only form that reliably
   answers "what commit does this tag actually point at."
2. **Not a draft.**
   ```
   gh release view vX.Y.Z --json isDraft
   ```
   `isDraft: true` is a fail; a draft release is invisible to anyone not
   already looking for it.
3. **Zero assets**, for the three library repos.
   ```
   gh release view vX.Y.Z --json assets
   ```
   An empty asset list is correct for a Go module release; there is no
   build artifact to attach, and a non-empty list is worth a second look at
   what got attached and why (see section 3: a stray schema copy is exactly
   the kind of thing that must never be an asset).
4. **Tagged tree free of schema and WADL**, per section 3, re-run against
   the pushed tag specifically (not just the working tree you tagged from).

## 8. Release notes structure

Established by `v0.10.0` and `v0.11.0`. Every release note:

- States what changed and, briefly, why: not just a commit list.
- Calls out breaking changes explicitly, under their own heading, and names
  the downstream impact per known consumer by name (e.g. "bridge does not
  reference this field, no change needed"; "server-go references it in one
  test file, tracked as <issue>, not part of this release").
- States compatibility as **verified**, not assumed, when a claim of "no
  consumer needs to change" is made: state what was actually built and
  tested against what.
- Lists findings that were reported but deliberately not fixed in this
  release, with a tracking reference if one exists.
- Closes with a Deployment section: the tag, the commit it resolves to, and
  for a library release, an explicit "no Docker image or deploy step;
  consumers pin the module version in their own `go.mod`" (or the
  equivalent bridge-specific line, for the one repo that does deploy).

## 9. The consumer chain: a core release does not propagate by itself

Consumers pin core by exact version in `go.mod` and only see a change when
someone bumps that pin. As of this writing, `server-go` and `client-go` both
still pin `v0.6.0` while core is at `v0.11.0`: four core releases landed
without either consumer's pin moving. `gridappsd-ieee-2030_5-go` (the
bridge) pins `v0.10.0`, one release behind current.

A version bump is its own reviewed change with its own tests, never a
rubber-stamped `go get -u` folded into an unrelated PR. `GOPRIVATE=github.com/GRIDAPPSD/*`
is required in the consumer's environment (locally and in CI) to resolve
these modules at all, since `go.sum` verification otherwise tries a public
proxy lookup that cannot see a private repository.

Two of the three consumers carry automation that surfaces drift rather than
relying on someone remembering to check: `gridappsd-ieee-2030_5-go` and
`ieee-2030_5-server-go` both run a daily `core-freshness.yml` workflow that
checks the pin against core's `main`, fails visibly (not silently) when
behind, and opens or updates a `chore/bump-core` pull request carrying the
evidence. `ieee-2030_5-client-go` has no equivalent workflow as of this
writing (see Findings below).

## 10. Authorization

Cutting a release, pushing a tag, and publishing a GitHub release are
irreversible or near-irreversible actions. They require Craig's explicit
authorization, granted either directly or as a stated premise in the task
that dispatches the release. A release should not be cut on an inferred or
assumed go-ahead.

## 11. Provider routing

All four repos have exactly one remote, GitHub, as of the `gitlab` remote
removal on 2026-08-02:

```
origin  https://github.com/GRIDAPPSD/<repo>.git
```

Provider commands (release creation, PR review, issue work) route off
`git remote -v`: GitHub means `gh`, not `glab`. Core's working tree still
carries `.gitlab-ci.yml` and `.gitlab/renovate.json5` from before the remote
removal; both are stale and describe a GitLab-hosted world this repo no
longer lives in. Do not run `glab` against any of these four repos, and do
not follow instructions inferred from those two stale files (see Findings
below; removing them was out of scope for this document).

## What the other three repos need

Each of `ieee-2030_5-server-go`, `ieee-2030_5-client-go`, and
`gridappsd-ieee-2030_5-go` should get a short `RELEASING.md` (a few
paragraphs, not a rewrite of this document) containing:

1. A pointer to this file: `See ieee-2030_5-core-go/RELEASING.md for the
   canonical release runbook; this file states only what is specific to
   this repo.`
2. Its own module path and current tag/version state at time of writing.
3. Repo-specific licensing material this document does not cover:
   - **server-go** needs its own WADL note (section 2 above already folds
     in `SEP2_WADL_PATH`, but server-go's stub should link its own `NOTICE`
     and `test/conformance/README.md`).
   - **client-go**: worth a human check before the stub is written. Its
     `NOTICE` file describes a different history (a private,
     non-redistributable archive of code translated from the EPRI C
     client) than its `README.md` describes (a clean-room client and
     inverter simulator). Confirm which is current before writing anything
     that asserts a licensing posture (see Findings below).
4. For the bridge specifically: how its existing `release.yml` (tag-driven,
   dry-run-safe, builds and publishes the binary automatically) relates to
   the manual steps in sections 6 to 7 above. The bridge's stub is the one
   file in this set that is NOT "same process, different repo name": it has
   real automation this document does not describe.
5. None of the three needs its own copy of sections 3 to 5 (licensing,
   history rewrite, visibility) beyond a pointer; those facts belong to core
   and to the family as a whole, not to any one consumer.

These three stub files were not written in this pass; this document was
scoped to core only.

## Findings / Issues Discovered

- **README badge stale**: `ieee-2030_5-core-go/README.md` line 6 shows a
  `release-v0.7.0` badge; the repo is at `v0.11.0`. LOW. Fix: update the
  badge or make it dynamic.
- **Stale GitLab remnants in core's working tree**: `.gitlab-ci.yml` and
  `.gitlab/renovate.json5` remain despite the `gitlab` remote's removal, and
  actively describe a GitLab-hosted CI/dependency-update world (runner tags,
  a `verify` stage) that no longer applies. LOW, but see section 11: worth
  removing so no future agent infers a `glab` workflow from their presence.
- **client-go has no `core-freshness.yml`**: the other two consumers
  (server-go, bridge) have automated daily drift detection against core's
  `main`; client-go does not, so a core release could go unnoticed there
  indefinitely with no automated signal. MEDIUM. Fix: port the workflow
  from server-go or the bridge.
- **client-go `NOTICE` vs `README.md` inconsistency**: `NOTICE` describes a
  private, non-redistributable archive of EPRI-derived code; `README.md`
  describes a clean-room client and inverter simulator. These read as
  descriptions of two different things. MEDIUM, and specifically a
  licensing-posture question, not a cosmetic one: worth a human read before
  anything is asserted about this repo's distribution status in a release
  note or a stub `RELEASING.md`.
- **core has no `release.yml`**, unlike the bridge. Not a defect (a Go
  library legitimately needs no build/publish automation the way a binary
  does), noted here only so the asymmetry in section 0 is traceable to a
  concrete fact rather than an assumption.
