# Using caching, reviews, and releases

The CLI caches confirmed computed answers by function content. The local draft
service supports checked reviews, whole-project release activation, production
evaluation, and rollback with memory or SQLite storage.

The service remains loopback-only. Actor names are supplied local labels, not
login identities or enforced project roles. Use this workflow locally; hosted
accounts and tenant permissions remain future work. The local playground now
includes review/release controls and behavior comparisons; see the
[playground guide](playground-review-guide.md). The static
browser playground has no release service or persistent result cache.

## Try the cache example

From the repository root, copy the example to a disposable directory:

```sh
cache_project=$(mktemp -d)
cp examples/cache/message.star "$cache_project/"
go run ./cmd/park --root "$cache_project" run 'Message.greet("Sam")'
go run ./cmd/park --root "$cache_project" run 'Message.greet("Sam")'
ls "$cache_project/.cache"
go run ./cmd/park --root "$cache_project" version list
go run ./cmd/park --root "$cache_project" test
```

Both calls print `"hello  Sam"` (two spaces). The first computes and writes a
canonical JSON result; the second reads that record. Sam is not an authored
example, so this demonstrates the computed cache rather than example lookup.
The two authored examples continue to count toward readiness; cached answers
never become examples.

Edit `PREFIX` and the two expected answers in the temporary `message.star`,
then rerun. A constant or private-helper edit changes `Message.greet`'s hash,
so the old cached answer cannot be returned. Changing whitespace *inside* a
string is a behavior change. Moving a statement into a different block also
changes identity. Ordinary spacing, comments, blank lines, and equivalent
string quote spelling leave function identity unchanged.

The cache key includes the qualified function, its complete reachable function
graph, canonical named inputs, and a runtime/hash-schema identity. Resolved
references include private helpers, constants, aliases, and namespaces passed
as values. Recursive groups are supported. A direct call to one namespace
member does not depend on unrelated members; passing an entire namespace as a
value conservatively depends on all its functions. Parameter names are part of
the signature and identity. `park version list` shows 12-character display prefixes; cache records and
review/release manifests retain full SHA-256 hashes.

Authored examples always win. Only successful calls resting on ready code are
cached. Failures, unknown calls, and unconfirmed answers are never cached.
`park test` bypasses the tested function's examples and disables computed-cache
reads/writes throughout. `park run --why` recomputes to show the whole execution
tree; it can still populate the cache with confirmed results.

Delete `.cache/` whenever you want to reclaim space or recover from an invalid
record; the next run rebuilds needed results. Old hash-schema records do not
match current keys. There is no automatic eviction. Retaining the first and
most recent runtime calls is a separate, unimplemented sampling proposal.
The browser, drafts, and production API evaluate in memory without disk or
shared result caching. Function hashes are identities, not access permissions.

## Run the release walkthrough

Start with a **new database path**; the walkthrough expects generation zero
and creates two releases before rolling back. This changes the local service's
active release. It does not deploy anything to a hosted environment.

```sh
go run ./cmd/park-drafts --root examples/shipping --db .park/release-demo.sqlite
```

In another terminal:

```sh
python3 scripts/draft_walkthrough.py --workflow release
```

For a different port, start the server with `--addr 127.0.0.1:8082` and pass
`--url http://127.0.0.1:8082` to the walkthrough. Temporary memory mode works
with `--db ""`; it loses all reviews/releases on shutdown.

The walkthrough activates the checked baseline, adds Germany's zone in a
draft, verifies live evaluation is unchanged, reviews the changed source and
function versions, and activates the Germany release. The live quote then
returns `2799`. Finally, rollback restores the baseline's unresolved Germany
answer. Both immutable releases and the changed draft remain available.
[The exact requests](tutorials/release-workflow.json) are also replayed by Go
HTTP tests against memory and SQLite reopened before every request.

## Request a review of your own draft

Save source first using the [draft API](saas-usage.md). Fetch the current draft
and production pointer:

```sh
curl -s http://127.0.0.1:8081/api/v1/drafts/YOUR_DRAFT
curl -s http://127.0.0.1:8081/api/v1/production
```

Copy `draft.state_token` and `production.generation` into this request. A fresh
service has `release_id: ""` and generation `0`; the reference is not live until
someone approves its checked snapshot.

```sh
curl -s -X POST http://127.0.0.1:8081/api/v1/drafts/YOUR_DRAFT/reviews \
  -H 'Content-Type: application/json' \
  -d '{"author":"developer","expected_state":"PASTE_STATE_TOKEN","expected_generation":0}'
```

HTTP 201 returns `review.id`, status `pending`, and the pinned source version,
full source files, runtime identity, full function hashes, check report, source
file changes, and function changes. The diff uses the active release as its
base, or the reference before the first release. Requesting a review freshly
checks every implemented function against every authored example, without the
computed cache. The operation does not change draft files, questions, state
tokens, or activity.

An implemented function needs at least two distinct examples and passing
checks. Example-only tables are valid data without an implementation or a
two-example minimum. Missing and unconfirmed dependencies raised by checks
block review, even if the returned value matches the expected answer. A check
report covers authored cases; it does not prove answers for all future inputs.
HTTP 422 includes failing calls/readiness details in `error`; correct the draft
and request review again.

## Inspect, approve, or reject

```sh
curl -s http://127.0.0.1:8081/api/v1/reviews/REVIEW_ID
curl -s http://127.0.0.1:8081/api/v1/releases/BASE_RELEASE_ID
```

Compare the review's complete `files` against the base release's files (or
`GET /api/v1/base` for the first review). Inspect `diff.function_changes` and
`checks`. A caller can change because a callee changed even when its own body
did not. Full source snapshots are authoritative; display hashes alone do not
approve a change.

Approval requires a different actor label from the requester:

```sh
curl -s -X POST http://127.0.0.1:8081/api/v1/reviews/REVIEW_ID/approve \
  -H 'Content-Type: application/json' \
  -d '{"author":"reviewer","expected_generation":0,"reason":"Checked shipping change"}'
```

Approval and activation are one atomic operation. It creates an immutable
whole-project release, records requester/reviewer/checks, and advances the
production generation. The draft source head, runtime, and production base
must still match the review. Source edits require a new review, including an
edit followed by a revert into a new source version. Runs and question activity
alone do not invalidate pinned source checks. Two competing approvals cannot
both replace the same production generation.

To reject instead, supply a reason:

```sh
curl -s -X POST http://127.0.0.1:8081/api/v1/reviews/REVIEW_ID/reject \
  -H 'Content-Type: application/json' \
  -d '{"author":"reviewer","reason":"Add the missing business cases"}'
```

A decision is final for that review. Rejected reviews remain inspectable and
cannot activate; request a new review when ready.

## Evaluate an active release

```sh
curl -s -X POST http://127.0.0.1:8081/api/v1/production/run \
  -H 'Content-Type: application/json' \
  -d '{"call":"Shipping.quote(\"DE\", 800)"}'
```

The response's `run` includes `release_id`, `generation`, source-version
`revision`, `runtime`, `status`, output, and the actual call explanation in
`why`. A request captures one immutable release before evaluation. Another
activation can proceed while it runs; the answer still identifies the release
captured at its start. Use `run.generation` to interpret that answer, then fetch
production if you need the latest pointer.

An unresolved call returns HTTP 200 with `run.status: "unresolved"` and the
missing call in the explanation. It does not create a draft question. An
invalid call or evaluation error returns HTTP 422 with `run.status: "error"`.
Before the first approval, evaluation returns HTTP 404. Draft edits, review
requests, and checks alone cannot change live answers.

## Roll back a release

List releases, choose a previous ID, and fetch the current generation:

```sh
curl -s http://127.0.0.1:8081/api/v1/releases
curl -s http://127.0.0.1:8081/api/v1/production
curl -s -X POST http://127.0.0.1:8081/api/v1/production/rollback \
  -H 'Content-Type: application/json' \
  -d '{"author":"operator","expected_generation":2,"release_id":"PREVIOUS_RELEASE_ID","reason":"Restore the checked baseline"}'
```

Rollback switches the complete project and increments generation; it does not
rewrite release files, remove reviews, or undo draft edits. The target must
exist, use the current runtime identity, and differ from the active release.
It cannot undo actions an external consumer took based on earlier answers.
`GET /api/v1/production` includes activation history with previous release,
actor, reason, and UTC date. The reference files remain unchanged.

## API and recovery reference

| Method and route | Result / required fields |
|---|---|
| `POST /api/v1/drafts/{id}/reviews` | Fresh checks and pinned review; `author`, `expected_state`, `expected_generation` |
| `POST /api/v1/drafts/{id}/compare` | Compare named scenarios against active release and pinned draft; see [contract](playground-review-guide.md#comparison-api) |
| `GET /api/v1/reviews` | All reviews |
| `GET /api/v1/reviews/{id}` | Full pinned review and decision |
| `POST /api/v1/reviews/{id}/approve` | Atomic release creation/activation; `author`, `expected_generation`, optional `reason` |
| `POST /api/v1/reviews/{id}/reject` | Final rejection; different `author`, nonempty `reason` |
| `GET /api/v1/releases` | Immutable release manifests |
| `GET /api/v1/releases/{id}` | Complete release, source, checks, and attribution |
| `GET /api/v1/production` | Active pointer, generation, activation history |
| `POST /api/v1/production/run` | Evaluate active release; `call` |
| `POST /api/v1/production/rollback` | Switch to a retained release; `author`, `expected_generation`, `release_id`, `reason` |

POST bodies must be JSON and fit the existing 4 MiB request limit. Review
requests have a 20-second overall context; evaluations retain engine step,
stack, value, and per-call time limits. Lists return full manifests in ID order;
this local API has no pagination or retention policy.

HTTP 400 means required fields are missing/invalid, requester and reviewer
labels match, or a rejection/rollback has no reason. HTTP 409 means the draft,
production base, runtime, or review decision is stale. Fetch current state,
review changes, and request a new review; changing only the generation cannot
make an old review's base current. HTTP 404 means an unknown ID or no active
release. Failed decisions leave reviews, production, and drafts unchanged.

SQLite migration `003_releases.sql` adds local review/release storage. Reviews,
release manifests, decisions, and the production pointer commit together under
an immediate transaction shared with draft writes. The local implementation
stores this project's workflow as a JSON record; history growth and hosted
scaling need a later storage design. SQLite restarts and the existing
[backup/restore commands](saas-usage.md) preserve the complete workflow. Exports
remain ordinary source projects and do not include reviews/releases. Keep the
original `--root` when restarting; active production is separate from that
reference. Older binaries refuse the new migration rather than ignore it.

A changed runtime identity prevents activation, rollback, or execution of old
runtime releases. Keep a compatible binary for recovery, or run fresh checks
and approve a new review with the current runtime. Do not edit stored manifests
or migration checksums to bypass this check.
