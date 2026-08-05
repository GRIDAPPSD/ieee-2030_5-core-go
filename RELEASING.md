# Releasing ieee-2030_5-core-go

Sections 2 to 5, 7 to 9, 11 to 15 and 17 are shared doctrine: they read
identically in the `RELEASING.md` of all four repositories in this family
(`ieee-2030_5-core-go`, `ieee-2030_5-server-go`, `ieee-2030_5-client-go`, and
`gridappsd-ieee-2030_5-go`, the bridge), so a releaser moving between
repositories is not re-learning the process. Sections 1, 6, 10 and 16 hold
this repository's own facts, as does subsection 17.6 inside the shared section
17. Section numbers mean the same thing in all four.

## 1. This repository and its release shape

Module path: `github.com/GRIDAPPSD/ieee-2030_5-core-go`

Core is the shared IEEE 2030.5 protocol layer at the base of the family. It is
the only module here with internal consumers: the server, the client and the
bridge all import it, and none of them imports each other. That makes core's
releases the ones that move other people's work, and it makes section 9's
downstream-impact requirement the load-bearing part of a core release rather
than a formality.

Release shape: a Go library. There is no build artifact and no tag-driven
release workflow. A release is a tag plus `gh release create`, cut by hand
following section 11. Consumers `go get` the module; there is nothing to
attach.

Core is released first, on its own schedule. Each consumer then bumps its own
pin on its own schedule, as its own reviewed change (section 10).
## 2. Versioning

Every module in this family is 0.x. Semver's 0.x carve-out applies: MINOR
carries any change that is not a strict patch, including a breaking one, and
MAJOR is not in play until a 1.0 commitment is made.

The version is not chosen by feel. It is read off the classification built in
section 3, by this rule:

- Any entry categorised `feature` or `breaking` forces at least MINOR.
- A range whose entries are all `bug fix`, `documentation`, `test` or `chore`
  may be PATCH.

State the version WITH that classification as its justification, in the
release notes, so a reader can check the arithmetic instead of trusting it.
"MINOR because the range adds two routes and removes one" is a justification.
"MINOR" on its own is not.

Worked examples from this family, all correctly MINOR:

- an additive release that introduced a new store contract and required no
  consumer change;
- a release that changed an exported field's type from a pointer to a
  complexType, which is breaking, and was still MINOR because the family has
  not committed to 1.0 semantics;
- a release that moved a served address and added a new write surface.

## 3. Classify the FULL commit range since the previous tag

Before choosing a version, enumerate every merge since the previous tag, not
only the change that prompted the release.

```
git fetch origin --tags
git log --first-parent --oneline vPREV..origin/main
```

Classify every merge in that range into the categories in section 5. The
version follows from the whole range's classification, per section 2. Do this
before writing the notes, because the table in section 5.1 IS the version
justification.

This step exists because its absence already cost a correction in public. A
release was cut as a PATCH on the strength of the one change its releaser had
in mind, while the range since the previous tag also carried new routes, a new
exported type and behaviour changes. It was published before anyone re-read
the range, and the notes had to be corrected after the fact. A correction
appended to published notes reaches nobody who already read them.

If the range is empty there is nothing to release. If it contains merges you
did not author, read them: "I only changed one thing" is a statement about
your own work, not about the range.

## 4. First release of a module

A module with no meaningful prior version has no range to diff against, so
section 3's `vPREV..origin/main` does not apply. Use this instead.

1. Confirm there is genuinely no prior release:
   ```
   gh release list --limit 20
   git tag --list
   ```
   A repository can carry tags that are not releases: import markers, backup
   points, phase snapshots. Those are not a prior version and do not seed the
   numbering. A repository with tags but no published release is still a first
   release.
2. Choose `v0.1.0` unless there is a specific reason not to. Do not start at
   `v1.0.0`: that is a stability commitment this family has not made, and it
   cannot be walked back once a consumer has resolved it.
3. Build the section 5 table over the module's history, but do NOT list every
   merge since the repository was created. List the merges a consumer needs to
   know about in order to adopt the module at all, and say so in a line above
   the table: "First release; the table covers the surface a consumer adopts,
   not the full commit history."
4. The Difference column is still mandatory. It reads differently for a first
   release: the difference is measured against having no dependency on this
   module at all, so each cell states what an adopter gains and what it must
   now supply (configuration, certificates, environment variables, a build
   step).
5. Everything else in this document applies unchanged: the same gates, the
   same licensed-material check, the same post-release verification.

## 5. Release notes: the required structure

Release notes are the human record of a release. They are required and they
have a fixed shape. Read a published example before writing your own: the
`v0.13.0` notes on `ieee-2030_5-core-go`, at
https://github.com/GRIDAPPSD/ieee-2030_5-core-go/releases/tag/v0.13.0

Every release's notes contain the following, in this order.

### 5.1 The commits table

One row per merged pull request. Per-merge, not per-commit: a pull request
routinely carries a version bump, a fix and a test as three separate commits,
and listing those separately is noise that buries the one line a reader needs.
The issue link carries the why.

```
### Commits in this release

| Merge | Issue | Category | Change | Difference for consumers |
|---|---|---|---|---|
| [<sha>](<commit url>) ([#<pr>](<pr url>)) | [#<issue>](<issue url>) | <categories> | <what changed> | <what a consumer observes now that they did not before> |
```

Internal card IDs never appear here or anywhere else that ships: not in
release notes, tag messages, commit messages, PR bodies, or source. A public
issue is the anchor for why a change happened; an internal card may link out
to that issue, never the reverse.

Mint an issue when a row needs a why an outside reader cannot infer from its
title. A dependency bump or a typo fix does not: leave the Issue cell empty
rather than minting one to fill it.

Immediately below the table, restate the category set and the version rule, so
the table explains itself to a reader who has never seen this document:

```
Category set: `feature`, `bug fix`, `breaking`, `refactor`, `documentation`,
`test`, `chore`. `breaking` stacks on a primary category rather than replacing
it. Any `feature` or `breaking` entry forces at least MINOR under the 0.x
carve-out, which is how the version below was determined.
```

### 5.2 Categories: a closed set

`feature`, `bug fix`, `breaking`, `refactor`, `documentation`, `test`,
`chore`.

The set is closed, and aligned to the conventional-commit prefixes these
repositories already use, so a merge's own commit message usually names its
category. If a change appears not to fit, that is a signal to split the row,
not to invent a category.

### 5.3 Three rules that make the table load-bearing rather than decorative

**Rule 1: categories drive the version.** Any `feature` or `breaking` entry
forces at least MINOR under the 0.x carve-out. A range whose entries are all
`bug fix`, `documentation`, `test` or `chore` may be PATCH. Enumerating the
range and determining the version are ONE procedure, not two: the table is the
version justification, which is why section 3 runs before a version is chosen.
A releaser who fills in the table honestly cannot then pick a version that
contradicts it.

**Rule 2: `breaking` stacks on a primary category rather than replacing it.**
Write `feature, **breaking**`, not one or the other. A worked case from this
family: one change mounted the WADL-declared LogEvent addresses, which is a
`feature`, and removed the previous `/log` addresses, which is `breaking`.
Forcing a single category loses half the story, and there it was the removal
that broke two consumers, so the half that would have been lost is the half
that mattered. A row may equally read `bug fix, **breaking**` or
`refactor, **breaking**`.

**Rule 3: the Difference column is mandatory and must be non-empty.** It is
what a consumer reads before deciding whether to bump, and it is the field
most likely to be skipped, because the difference is obvious to the person who
just made the change and to nobody else. The contrast, from the same worked
case:

- CHANGE: "mounts `/edev/{id}/lel` and derives `LogEventListLink` on every
  EndDevice read path".
- DIFFERENCE: "any ACL rule, test or client that names `log` now matches
  nothing and fails closed, so LogEvent posting stops working with no error
  that points at the cause".

Only the second tells a consumer to act. Write the Difference cell as what
somebody else observes, not as what you did.

If the honest answer is that nothing observable changes, write that: "no
observable change; internal refactor with identical wire output" is a valid
cell. An empty cell is never valid, because an empty cell cannot be told apart
from a cell nobody thought about.

### 5.4 The rest of the notes

After the table:

- The version and its justification, referencing the table (section 2).
- Breaking changes, each under its own heading, with downstream impact named
  per section 9.
- Added or changed behaviour that needs more than a table cell to explain.
- A **Verified** section stating what was actually built and run, and against
  what, including which env-gated suites were armed and which tests in them
  ran (section 7). A claim that "no consumer needs to change" is stated as
  verified only where something was actually executed; otherwise say what was
  reasoned and what was not.
- Findings that were reported and deliberately NOT fixed in this release, with
  a tracking reference where one exists.
- A **Deployment** section: the tag, the commit SHA it resolves to, and either
  "no Docker image or deploy step; consumers pin the module version in their
  own `go.mod`" for a library release, or the concrete artifact and deployment
  path for a repository that ships one.
## 6. Verification gates before tagging (core)

Run all of these on the exact commit you intend to tag:

```
go build ./...
go vet ./...
go test -count=1 ./...
go test -race -count=1 ./...
gofmt -l .
```

Two things about the output that a first-time releaser will misread:

- **`gofmt -l .` is not expected to be silent.** A vendored stub file under
  `pkg/sep2tls/gotls/` reports, and is excluded in CI for that reason. Read
  the paths it prints and confirm they are only that vendored tree. Treating
  any gofmt output as "the known one" without reading it is how a real
  formatting drift gets waved through.
- **`-race` has surfaced at least one intermittent failure in an assembly
  test.** A single red `-race` run is not automatically a release blocker, but
  it is never ignored either: re-run the failing test individually under
  `-race -v`, and if it passes, say so explicitly in the notes' Verified
  section along with the test name. An unexplained flake left out of the notes
  is indistinguishable from a suppressed failure.

**Env-gated suite in this repository: the schema gate.** `SEP2_SCHEMA_PATH`
supplies the IEEE normative `sep.xsd` from outside the checkout. With it
unset, the schema-gated tests SKIP and the suite is still green.
`SEP2_SCHEMA_REQUIRED=1` arms the gate so a missing or misconfigured schema
FAILS instead of skipping. Section 7 applies in full: run it armed, confirm
zero SKIP, and name the tests in the notes.

Core owns the schema gate for the whole family (`schema` and
`internal/xsdgate`). The WADL gate lives in server-go. The two contracts are
deliberately the same shape, so learning one teaches the other.

**These gates are the machine half of what runs before a tag.** Section 17 is
the other half: an independent read of the whole commit range by somebody who
did not write it. Core's own instrument for that is building the three
consumers against the candidate commit, which is the only check here that
catches a break in code this repository does not contain (17.6).
## 7. Env-gated test suites: confirm they RAN, and name the tests

Several suites in this family are gated behind an environment variable and
SKIP silently when it is unset. A skipped gate and a passing gate report
success indistinguishably: `go test ./...` is green either way, and a releaser
reading only the exit code learns nothing.

This is not a hypothetical. A wire-format regression reached a published
release because the gate that would have caught it skipped, and the release
notes recorded that run as green.

Before tagging, for every env-gated suite this repository has (section 6 lists
them):

1. Run it with the gate ARMED, not merely with the variable set. Where a
   `*_REQUIRED` style variable exists, set it too, so a missing or
   misconfigured input fails the run instead of skipping it.
2. Read the output and confirm zero SKIP results among the gated tests.
   Contrast it against a run with the variable unset, which should show the
   full skip. If the two runs look the same, the gate is not armed and neither
   run proved anything.
3. Name the tests that ran, in the release notes' Verified section. "Schema
   gate passed" is not evidence. "All `TestSchemaGate*` in `pkg/sep2` passed,
   including the new cases this release adds, with zero SKIP" is evidence,
   because a reader can check it.

A releaser who does not have the gated input available should say exactly that
in the notes rather than reporting a green run that proves nothing. An honest
"not run, input unavailable" is useful; a green tick over a skip is worse than
no line at all.

## 8. Licensed material must never ship, and the check must prove it looked

The IEEE 2030.5 normative XML Schema (`sep.xsd`) and normative WADL
(`sep_wadl.xml`) are licensed material. Neither may appear in a commit, in a
tag's tree, in a release artifact, or in any binary built from these
repositories. Both are supplied at test time from outside the checkout, via
environment variable. Where this repository carries a `NOTICE` file, it holds
the full rationale.

Check the tagged tree, not the working tree you tagged from:

```
git fetch origin --tags
git ls-tree -r vX.Y.Z --name-only > tagged-files.txt
echo "files scanned: $(wc -l < tagged-files.txt)"
echo "matches:       $(grep -icE 'sep\.xsd|sep_wadl\.xml' tagged-files.txt || true)"
```

**Report both numbers, and report them together.** A zero-match result from a
command that scanned zero files is not a check, it is a false green, and that
exact failure happened during a release in this family. It was caught only
because the file count was printed beside the match count.

- Zero matches against a plausible file count is a pass.
- Zero matches against a file count of zero means the command was pointed at
  nothing, most often a tag that was never fetched or a typo in the ref. It is
  a FAIL. Fix the ref and run it again.
- A non-zero match count stops the release.

Do not resolve a non-zero match by moving the tag (section 13). Resolve it by
not publishing, fixing the tree and tagging a new version.

A security classifier flagging a release for licensed material is the system
working as intended, not a false alarm to route around. Running this check
before publishing is what makes answering such a flag cheap.

## 9. Downstream impact

Release notes name the downstream impact of every breaking change, per known
consumer, by name. "Breaking" on its own leaves each consumer to work out
whether it is affected, which in practice means most of them will not.

For each known consumer, state one of:

- it does not reference the changed surface, so no change is needed, AND how
  that was determined (a grep, a build, a test run);
- it does reference it, at a named file and line, with a tracking reference;
- it was built and tested against the new version, and passed.

**When a module has no established external consumers**, this requirement does
not evaporate, and silence does not satisfy it. Do this instead:

1. State it plainly in the notes: "No known external consumers as of this
   release." A reader must be able to tell "we checked and there are none"
   apart from "nobody checked".
2. Say how you know. The check is cheap: search the sibling repositories'
   `go.mod` files for this module path, and record the result rather than the
   impression.
3. Describe the impact on a hypothetical adopter anyway, in the same terms:
   what a consumer that DID depend on the changed surface would have to do.
   That is what the Difference column already asks for, and it is what keeps
   the notes useful on the day somebody does adopt the module, which is
   precisely the day nobody will re-derive it.
4. In-repo consumers count. A change that breaks this repository's own binary,
   its own test harness, its own example code or its own deployment
   configuration is a downstream impact and is named the same way.
## 10. Who consumes this module

Three repositories import core:

- `ieee-2030_5-server-go`, the standalone IEEE 2030.5 server;
- `ieee-2030_5-client-go`, the DER client and inverter simulator;
- `gridappsd-ieee-2030_5-go`, the bridge to the GridAPPS-D platform.

Section 9 is therefore never a formality here. Every core release names all
three by name and says, for each, whether it references the changed surface
and how that was determined.

**Read each consumer's current pin rather than trusting a number written
down anywhere.** Pins are deliberately not recorded in this document, because
a recorded pin is wrong within days and a stale pin in a runbook is worse than
no pin at all:

```
for r in ieee-2030_5-server-go ieee-2030_5-client-go gridappsd-ieee-2030_5-go; do
  printf '%s: ' "$r"
  gh api "repos/GRIDAPPSD/$r/contents/go.mod" --jq .content \
    | base64 -d | grep ieee-2030_5-core-go
done
```

Expect them to differ from each other and to lag core, sometimes by several
releases. That is the normal state, not an incident.

**A core release does not propagate on its own.** Consumers pin by exact
version and see a change only when somebody bumps the pin. A bump is its own
reviewed change with its own tests, never a rubber-stamped `go get -u` folded
into an unrelated pull request. A core release is finished when the release is
published; the consumers moving is separate work with its own review.

**`GOPRIVATE=github.com/GRIDAPPSD/*`** is required in a consumer's
environment, locally and in CI. Without it, module resolution attempts a
public proxy and checksum-database lookup that cannot serve the
GRIDAPPSD module, and fails in a way that reads like a network problem.

Some consumers run a daily freshness workflow that compares their pin against
core's `main`, fails visibly when behind, and opens or updates a bump pull
request carrying the evidence. Which repositories have it changes over time,
so check rather than assume:

```
gh api repos/GRIDAPPSD/<repo>/contents/.github/workflows --jq '.[].name'
```

Where a consumer has no such workflow, a core release produces no automated
signal there at all, and the pin moves only when a person remembers.
## 11. Cutting the release

1. Confirm authorization (section 14).
2. Confirm the section 6 gates are green on the exact commit you intend to
   tag, and that the section 7 env-gated suites actually ran.
3. Classify the full range and determine the version (section 3, or section 4
   for a first release).
4. Write the notes (section 5) BEFORE tagging. Writing them first is what
   surfaces a misclassified range while it is still free to fix; writing them
   after means discovering the mistake at a point where the only honest remedy
   is another version.
5. Have the range independently verified and the outcome recorded
   (section 17). Somebody who did not write the range checks the specific
   claims the notes and the cards make about it, and returns CONFIRMED,
   CONTRADICTED or UNVERIFIABLE for each. This step sits here for two reasons:
   after the notes, because the notes are where most of the claims are, and
   before the tag, because the tag is where a wrong claim stops being
   editable. Its depth follows the same classification that chose the version
   (section 17.5), so a patch range gets a light pass and stays affordable.
6. Tag:
   ```
   git tag -a vX.Y.Z -m "vX.Y.Z"
   git push origin vX.Y.Z
   ```
   A lightweight tag (`git tag vX.Y.Z`) is also acceptable; the release in the
   next step is what makes it a full release either way. Tag history in this
   family mixes both kinds, which is why section 12's dereference step is
   written the way it is.
7. Run the section 8 licensed-material check against the pushed tag.
8. Publish the release:
   ```
   gh release create vX.Y.Z --notes-file <path>
   ```
9. Run the section 12 post-release verification before telling anyone the
   release is done.

**A tag is not a release.** Both are required and they serve different
readers. The tag is the machine-facing half: it is what `go get` resolves, and
without it a consumer cannot depend on the version at all. The GitHub release
is the human record: it is where the section 5 notes live, and without it a
consumer can fetch the code but has no way to learn what changed or whether to
act. Stopping after `git push origin vX.Y.Z` leaves a version consumers can
silently pick up with no notes attached to it, which is the worst of the two
halves rather than half the job.

## 12. Post-release verification (mandatory, in this order)

1. **The tag dereferences to the intended commit.**
   ```
   git fetch origin --tags
   git rev-parse "vX.Y.Z^{commit}"
   ```
   Use `^{commit}`. A bare `git rev-parse vX.Y.Z` on an ANNOTATED tag returns
   the tag object's own SHA, which is not the commit SHA and will not match
   the HEAD you meant to tag. On a LIGHTWEIGHT tag the two happen to be the
   same value, which is exactly what makes the annotated case easy to miss: a
   check that was only ever tried against a lightweight tag looks correct and
   is not. `^{commit}` dereferences both kinds uniformly and is the only form
   that answers "what commit does this tag actually point at".

   Compare the result against the commit you intended to tag AND against the
   SHA written into the notes' Deployment section. All three must agree.
2. **The release is not a draft.**
   ```
   gh release view vX.Y.Z --json isDraft
   ```
   `isDraft: true` is a fail. A draft is invisible to anyone not already
   looking for it, so a drafted release is a tag with no human record: the
   same failure section 11 describes, arriving by a different route.
3. **The asset list is what you expect.**
   ```
   gh release view vX.Y.Z --json assets
   ```
   For a library release with nothing to attach, an empty list is correct, and
   a non-empty one is worth reading closely: a stray licensed file is exactly
   the kind of thing that must never be an asset (section 8). For a repository
   that ships an artifact, confirm the expected artifact is present and that
   nothing else is.
4. **The licensed-material check passes against the pushed tag**, with both
   numbers reported (section 8).

## 13. A published tag is immutable

Never move, re-cut, force-push or delete a tag that has been pushed.

The reason is specific to Go consumption. A tag is what `go get` resolves, and
a consumer that already resolved it has the exact bytes recorded in its
`go.sum`. Moving the tag makes one version string resolve to different
content: a consumer that already fetched it keeps the old code and then sees a
checksum mismatch that names no cause, while a consumer fetching it for the
first time silently gets different bytes under a version somebody else has
already reviewed and approved. Neither of them has any way to see that the tag
moved.

If published notes are wrong, EDIT THE NOTES. `gh release edit` is reversible
and touches nothing anyone has already downloaded. If the tagged CODE is
wrong, cut the next version. Never reuse a number.

The same reasoning forbids history rewrites on these repositories generally.
Where one has happened in the past as a deliberate, one-time remediation, that
is a documented exception and not a precedent.

## 14. Authorization

Pushing a tag and publishing a release are irreversible in practice
(section 13). They require the operator's explicit authorization, granted
either directly or stated as a premise in the task that dispatches the
release. An inferred or assumed go-ahead is not authorization, and neither is
the work being finished: "this is ready to release" is a status report, not a
request to release it.

## 15. Provider routing and repository visibility

Every repository in this family has exactly one remote, GitHub. Provider
commands (release creation, pull-request work, issue work) route off the
remote host: GitHub means `gh`, never `glab`. Verify rather than assume:

```
git remote -v
```

If a repository still carries CI or dependency-update configuration files from
an earlier host, they are stale and describe a world these repositories no
longer live in. Do not infer a workflow from their presence, and do not follow
instructions read out of them.

A GitHub Release works the same regardless of repository visibility, so do
not conflate the two: **never change repository visibility as part of a
release**, however routine the release feels. Visibility is a separate
decision with its own review, and it is not meaningfully reversible once
content has been fetched.
## 16. Repository-specific notes and history (core)

**Core is the only module in this family with an established release
history.** The other three are at or near their first release, so section 4
applies to them and rarely to core. Read the current state rather than any
list written here:

```
gh release list --limit 20
```

The `v0.13.0` notes are the reference implementation of section 5's format.
When in doubt about a table cell, read that release and match it.

**A one-time history rewrite happened on 2026-08-02.** Core's history was
rewritten to purge a vendored copy of the licensed schema that predated the
environment-variable design in section 8. The consequences must be stated to
anyone consuming core tags:

- tags `v0.7.0` through `v0.9.0` point at DIFFERENT commits than they
  originally did;
- `v0.6.0` is unchanged;
- everything from `v0.10.0` onward is post-rewrite and clean.

This was a deliberate licensing remediation, performed once. It is not a
precedent, and section 13 stands unchanged: no tag on this repository is ever
moved, re-cut or deleted again.

A pre-rewrite backup mirror exists outside this repository. It deliberately
contains material that may not be redistributed. Never push to it, copy from
it, or quote its contents. Its only purpose is disaster recovery in the hands
of somebody who already knows what it holds.

**A security classifier flagged a core release for licensed material on
2026-08-02**, and the artifact was then verified clean. That is the system
working as intended. Section 8's check, run before publishing and reported
with both numbers, is what makes answering such a flag a two-minute job
instead of an investigation.

**Two packages are slated to move to server-go** (`pkg/store` and
`pkg/sep2srv`) under the target layering. When that happens, the parts of this
document that describe them move with the code. Do not let a release note
assert the current home of a package without checking it.

## 17. Independent verification of the range before tagging

### 17.1 The gate

Before the tag is pushed, one person who did not write the commit range reads
it and returns a verdict on each specific claim the release notes and the cards
make about it. The verdicts are written down. A release with no verification
record does not get tagged.

This is a gate with a recorded outcome, not a suggestion, and it is not a code
review. Code review asks whether the change is any good, and it already
happened, on the pull request. This asks a narrower question: is what we are
about to publish about this range true. Those are different questions, and the
second is the one that has been getting answered wrong.

### 17.2 The reviewer did not author the range

This is the entire mechanism, and it is the part that cannot be traded away for
convenience.

"Did not author" means: wrote none of the commits in `vPREV..<candidate>`, and
did not merge them. The releaser may be the reviewer only if the releaser also
authored none of the range, which is uncommon.

A verification pass by the author reproduces the author's assumptions. The
author already believes the claim, so re-reading their own diff is how they
confirm it. Every contradiction this family has found this way was found by
somebody reading code they had not written, and the same claims had already
survived being restated by the people who wrote them.

Depth scales with the range (17.5). Independence does not. A one-merge
documentation range still gets a reader who did not write it, because that pass
is cheap: a handful of claims and a small diff.

If genuinely nobody else is available, that is a recorded outcome and a weaker
one, not a waiver. Write "no independent reviewer available; the range was
verified by its author" into the notes' Verified section, in those words, so a
reader can weigh the release accordingly. What is never acceptable is a release
that reads as independently verified when it was not.

### 17.3 Name the claims; never ask for a general review

An open-ended "please review this release" produces style notes and a thumbs
up. Naming the claims is what produces contradictions, because a named claim
has a truth value and a general impression does not.

The releaser builds the claim list, drawn from what the release is about to
assert in public:

- every Difference cell in the section 5.1 table;
- every count and every absolute: "every", "all six", "at all fifteen", "none";
- every statement about a consumer: what it does or does not reference, and
  what was built or run to determine that (section 9);
- every "verified" statement in the notes, including which env-gated suites ran
  (section 7);
- every claim already recorded on the cards in the range, especially one
  written from somebody's report rather than from the code.

The reviewer returns, per claim, exactly one of three verdicts, each with
`file:line` evidence:

- **CONFIRMED**: true as written. A claim that holds only in a narrower form is
  not confirmed; see 17.4.
- **CONTRADICTED**: false as written, with what is true instead.
- **UNVERIFIABLE**: cannot be settled from what the reviewer has, with the
  reason (an input not available, a system not running, a claim about intent
  rather than about code). UNVERIFIABLE is a legitimate and useful answer. It
  is never upgraded to CONFIRMED on the grounds that it is probably fine.

Two habits carry most of the weight here. They come from the workspace rule
`.claude/rules/claims-and-provenance.md`, and are restated so this section
stands on its own:

- **Re-run the count; never read it off a commit message.** A claim of the form
  "every X is now a Y" is checked by enumerating X again, in the tree being
  released. Three of the six contradictions in 17.8 were counts whose exception
  the commit message stated plainly, and nobody re-ran the enumeration.
- **Print the denominator.** "0 remaining, across 47 declarations examined" is
  evidence. "0 remaining" is indistinguishable from a search pointed at the
  wrong tree, and a check that ran against nothing looks exactly like a check
  that passed.

### 17.4 A contradiction is the outcome that pays for the gate

A pass that finds nothing has cost one reading. A pass that finds one wrong
claim has stopped a false statement from being published under a version number
that cannot be withdrawn (section 13). The second is the expected case, not an
incident, and a run of passes that never contradict anything is a sign the
claim list is too vague rather than that the claims are unusually good.

Record every contradiction. Do not resolve one quietly by editing the sentence
and moving on: the edit leaves no trace that the claim was ever wrong, and the
same claim tends to return in the next release from the same source.

**"Technically true" is not CONFIRMED.** A claim that is true only in a form
narrower than it was written is recorded as CONTRADICTED, with the narrower
true statement supplied. "Fifteen call sites covering thirteen decisions" is
not "at all fifteen gates", and the gap between those two sentences is where a
reproducible nil-dereference panic lived. If the narrower statement is what
ends up in the notes, the notes were corrected by this gate, which is the gate
working rather than a formality being satisfied.

Resolving a contradiction before tagging is one of:

- change the notes so the claim matches the code, which is the cheap path and
  is always available;
- change the code, which lengthens the range and sends the changed part back
  through 17.1;
- keep the claim, downgraded to what was actually established, with its
  provenance attached: "the implementer reports X" is a different sentence from
  "X", and it is the honest one when nobody re-derived it.

A contradiction that names something failing a build, a test or a gate RIGHT
NOW is not a note. It is a card, filed at its real priority, before the release
continues. A live failure whose signal is already firing is the most expensive
thing to defer, because the signal keeps firing into a channel everybody has
stopped reading.

### 17.5 Depth follows the same signal as the version

Section 2 reads the version off the range's classification. Verification depth
is read off that same classification, so there is one judgment to make rather
than two, and the depth cannot drift away from the risk.

| Range classification (section 3) | Depth |
|---|---|
| All entries `bug fix`, `documentation`, `test` or `chore` (a range that may be PATCH) | **Light pass** |
| Any entry `feature` or `breaking` (a range that forces at least MINOR) | **Full pass** |
| A first release (section 4) | **Full pass** always: there is no prior tag, and every claim is new |

**Light pass.** The reviewer reads the diff of every merge in the range and
checks the claim list against it. It is bounded by the claim list, which for a
patch range is usually a handful of cells. This is kept deliberately cheap: a
gate that makes a patch release unaffordable gets skipped, and a skipped gate
protects nothing.

**Full pass.** The reviewer reads the diffs AND the surrounding code in the
tree being released, and runs whatever mechanical instruments this repository
has (17.6), reading their output rather than a summary of it. The distinction
is load-bearing. A diff shows what changed; it does not show the three
declarations left alone while a claim said all of them moved, and it does not
show that a recorded structure has no reader anywhere in the tree. Both of
those were found by reading the tree, not the diff.

**One rule crosses the split.** Any claim stating a count or an absolute gets
the full check even inside a light range. Those are the claims that have gone
wrong, they go wrong silently, and re-running one enumeration costs a minute.
### 17.6 What this repository gives the reviewer (core)

Core has no Makefile gates and no harness of its own. Its instruments are the
Go toolchain, the schema gate, and the three consumers.

**The consumers are the instrument.** Core is the only module in this family
with internal consumers (section 10), and a claim that a core change is safe
for them is settled by building them, not by reading core. Independent means
the reviewer does this in their own checkout rather than reading a transcript:

```
go mod edit -replace github.com/GRIDAPPSD/ieee-2030_5-core-go=<path to a core checkout at the candidate commit>
go build ./... && go test -count=1 ./...
go mod edit -dropreplace github.com/GRIDAPPSD/ieee-2030_5-core-go
```

At least one consumer for a light range, all three for a full pass, reported
per consumer by name. A repository in this family sat red against core's `main`
for two days while every claim about it was written from reasoning rather than
from a build (17.8). `GOPRIVATE=github.com/GRIDAPPSD/*` is required in the
consumer's environment (section 10), or a resolution failure will read as a
network problem and be dismissed as one.

**Claims about the assembly are counts, and counts are re-run.** Core's
contradicted claims were all one shape: an absolute about a set of fields or
types under `pkg/`. Check one by enumerating the set again in the tree being
released, and report both numbers:

```
grep -rn '<the declaration pattern the claim is about>' --include='*.go' pkg/ | wc -l
```

The pattern belongs to the claim, so derive it from the claim rather than from
this document. What matters is that the reviewer produces a number and the
denominator it came from, and takes neither from a commit message: the commit
message accompanying one of those claims stated its exception plainly and was
read past.

**The schema gate is the reviewer's too.** Section 6 arms it with
`SEP2_SCHEMA_PATH` and `SEP2_SCHEMA_REQUIRED=1`. On a full pass the reviewer
runs it in their own session rather than accepting the releaser's output, and
confirms zero SKIP. Where the schema is not available to them, the claims that
depend on it are UNVERIFIABLE and are recorded that way.

**A light pass here is genuinely light.** A documentation or chore range in
core has few claims and no behaviour to check, so the pass is one diff read,
one consumer build, and the claim list. Do not inflate it, and do not skip the
consumer build: it is the cheapest evidence core has.
### 17.7 Recording the outcome

The verification record lives in the release notes' Verified section (section
5.4), or in an artifact that the Verified section names and links. It states:

- who reviewed, and that they authored none of the range;
- the exact range reviewed, as `vPREV..<sha>`, with the SHA that was tagged;
- the depth, light or full, and the classification that selected it (17.5);
- the counts with their denominator: "14 claims checked: 11 CONFIRMED,
  2 CONTRADICTED, 1 UNVERIFIABLE";
- each CONTRADICTED claim, what was true instead, and how it was resolved;
- each UNVERIFIABLE claim and why it could not be settled.

Contradictions stay in the published notes even when they were fixed before the
tag. A reader learns more from "this claim was corrected during verification"
than from a clean list that hides the correction, and the next releaser learns
where the claims in this repository tend to go wrong.

A record that says only "independently verified" satisfies nothing. It is the
same failure as a green tick over a skipped gate (section 7): unreadable,
uncheckable, and indistinguishable from the case where nobody looked.

### 17.8 Why this exists

On 2026-08-03 and 04, a long run of work in this family was recorded onto cards
and into notes largely from implementers' reports, with a couple of pull
requests spot checked. One independent pass was then run over it, by readers
who had written none of it. That single pass contradicted six claims that had
already been recorded as fact:

- "every store field in the assembly is an interface": three were still
  concrete, and the commit message said so plainly;
- "all six embedding types carry the fix": four did;
- "the absence check is at all fifteen gates": fifteen call sites covering
  thirteen mount decisions, and a nil-dereference panic was reproduced on a
  field the check did not cover;
- "the ledger proves the identifier outlives the resource": the ledger had no
  production reader, the success response was unconditional, and the test
  offered as proof would still have passed with the ledger deleted;
- a release labelled PATCH whose range carried new routes, a new exported type
  and behaviour changes (section 3 exists because of this one);
- a repository in this family that did not compile against core's `main`, with
  its scheduled job red for two days, recorded as a note rather than as a card.

The last one is the argument in miniature: the break existed, a signal was
firing, and nobody looked until somebody was asked to.

The reports behind those claims were substantially accurate. The looseness
entered when they were restated in somebody else's voice, which is why care
alone does not fix this: care was present throughout. The workspace rule
`.claude/rules/claims-and-provenance.md` carries the underlying discipline (a
claim you did not personally verify carries whose claim it is, and never write
a number you did not count). This section applies that rule at the one moment
where a wrong claim stops being editable, which is the tag.
