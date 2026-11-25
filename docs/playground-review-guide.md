# Review, compare, and release in the playground

This first pass connects the existing local release APIs to the browser. It
adds named scenario comparisons and a session history of missing facts answered
for a request. Authentication and hosted workspaces remain deferred. Actual
usability sessions are still pending; use the [session guide](usability-sessions.md).

## Start a local workspace

From the repository root:

```sh
sh web/build.sh
go run ./cmd/park-drafts --root examples/shipping --db .park/review-demo.sqlite --web-dir web/dist
```

Open `http://127.0.0.1:8081`, choose **Shipping quotes**, and expand **Drafts and
source versions**. Use a fresh database for the baseline walkthrough below.
The static hosted playground still runs by itself; server review tools appear
only when served by `park-drafts --web-dir`. Actor names are local labels,
not authenticated accounts. Choose a different label for a review decision.

## Establish a checked baseline

1. Set your name to `developer`, then **Save as new draft**.
2. Expand **Review, compare, and release**. Saved draft and active release
   are displayed separately. Initially there is no active release.
3. Choose **Check saved source and request review**. Inspect the pinned source
   version, runtime, implementation-check count, table-record count, and check
   report. Example-only tables are data; they do not run implementation checks.
4. Change your local actor name above to `reviewer`, then **Approve and
   activate**. This creates and activates one whole-project release atomically.
5. Under **Evaluate the active release**, enter `Shipping.quote("DE", 800)`
   and **Run live release**. The baseline cannot answer Germany yet. The result
   identifies its release, generation, and source version.

Review requests run fresh server checks. Unready code, failing checks, missing
dependencies, or unconfirmed dependency answers block a request. A local browser
check is separate from the pinned server check report used for approval.

## Change an answer and compare behavior

1. In **Shipping zones**, add a row: country `DE`, result `international`.
   Text cells do not need quotes. Save the row, then **Save source version**.
2. Return to the release panel. Request a review as `developer`. Inspect the
   affected functions and expand `zones.star` to compare exact before/reviewed
   source. Whitespace inside literals is preserved in this view.
3. Expand **Compare named scenarios with the active release**. The shipping
   defaults are:

   ```json
   [
     {"name":"US parcel","call":"Shipping.quote(\"US\", 500)"},
     {"name":"Germany parcel","call":"Shipping.quote(\"DE\", 800)"}
   ]
   ```

4. Choose **Compare saved draft with live**. The US case is unchanged; Germany
   is newly answered with `2799`. Each side shows its status and output or
   error. The baseline release and draft source version are pinned in the report.
5. **Download pinned report** saves its exact scenarios, runtime, snapshot IDs,
   generation, and outcomes as JSON. This comparison covers the listed cases;
   it is not proof for every possible input, nor an implementation check.
6. Run the live Germany call again before approval: it is still unresolved.
   Switch to `reviewer`, approve, and rerun live: the result is now `2799`.

The comparison report remains visible but is marked as belonging to earlier
inputs after the active release, saved draft, scenario set, or browser source
changes. A refresh fetches current server metadata without replacing browser
source. Run a new comparison explicitly; reports never silently change baseline.
Scenario text is saved in browser storage per draft. Reports themselves are
session results unless downloaded; the server does not retain them in this pass.

## Reject, handle a stale review, or roll back

A pending review can be rejected by a different actor with a nonempty reason.
Rejection is final for that review and leaves the active release unchanged.

Unsaved browser changes disable requesting a review, comparison, and approval.
Save them deliberately or retain them while inspecting the saved snapshot.
A changed server source head or production generation makes a pending review
stale. If another client changes state after your last refresh, the server
rejects the decision and the panel explains the conflict. **Refresh reviews and
live state**, inspect the latest saved source, then request a new review. Local
source and typed table answers are retained; the panel does not retry approval
with a newer generation automatically. If only activity or questions advanced
and saved source still matches, refresh accepts the newer server metadata/token
without touching browser source. A response cannot move the client backward
over a newer operation that completed while it was fetching. The existing draft panel lets you open
a server copy explicitly or save your local source as another draft.

Under **Release history and rollback**, choose a previous release, supply a
reason, and **Roll back to selected release**. The complete earlier project
becomes active and generation advances. Draft source stays as you saved it.
Earlier live answers retain their original release labels and become visibly
stale after the production pointer changes.

## Resolve successive missing facts

In the browser's normal request area, answering a missing call reruns the
original request. If another fact is missing, its answer input comes first and
receives focus. The result shows the original request and the facts you saved
in order. When the trace uses those saved facts, it names them as reused.
Running a second request can show reuse of facts answered earlier in the same
browser session. A direct whole-request example does not claim smaller facts
were used. Loading another project resets this session history; saved examples
remain in source as usual.

For a two-question exercise, use the word-addition sample, remove the `thirteen`
row, and run `twelve plus thirteen`. Supply `12`, then `13`; the result is `25`.
Next run `twelve plus two` to see the same saved twelve meaning reused.
Saved-source server questions also rerun the last server request after an answer,
allowing successive server questions to be answered without manually reentering
the original request. Browser and server queues remain separate.

## Comparison API

`POST /api/v1/drafts/{id}/compare` accepts:

```json
{
  "expected_generation":1,
  "draft_revision":"FULL_SAVED_SOURCE_VERSION_ID",
  "scenarios":[{"name":"Germany parcel","call":"Shipping.quote(\"DE\", 800)"}]
}
```

The source version must belong to that draft's retained ancestry. Historical
versions can be compared explicitly. An active release is required. Its runtime
must match the current engine. The endpoint captures both source snapshots in
the store transaction, then evaluates after releasing the transaction/lock.
It never changes source, examples, tokens, activity, or question queues and
never uses the disk result cache.

HTTP 200 returns `comparison` with report ID, source/runtime pins, scenario-set
hash, normalized scenarios, and rows. Categories are `changed`, `newly_answered`,
`became_unresolved`, `unresolved`, `unchanged`, `error`, and `unconfirmed`. Each
row contains both outcomes and a `changed` flag. Error outcomes can describe
either side, including an invalid call. An unconfirmed outcome is not a confirmed
answer. Missing functions are evaluation errors; missing example-only inputs
are unresolved calls.

Supply 1–32 scenarios with distinct nonempty names (up to 120 UTF-8 bytes) and
nonempty call text (up to 4096 bytes). Invalid JSON, scenario sets, or missing
pins return 400. A stale production generation/runtime returns 409; an unknown
source version or no active release returns 404. The existing 20-second request
context applies across the entire report; cancellation returns no partial report.
Reduce slow scenario sets rather than assuming every scenario gets 20 seconds.

## Developer verification

`sh build.sh` includes the Go checks, JavaScript unit tests/lint, and WASM smoke
verification. For real browser tests after building `web/dist`:

```sh
web/.tools/node_modules/.bin/playwright install chromium --only-shell
npm --prefix web/.tools run test:browser
```

On Linux CI, install browser system dependencies with Playwright's `--with-deps`
flag. Tests start a fresh memory-backed server on an ephemeral loopback port per
test, block external editor assets, and exercise the actual DOM and WASM. Go
comparison tests exercise memory and SQLite stores, historical pins, all outcome
categories, rejected preconditions, and state/queue isolation. The browser suite
covers review/release decisions, live isolation, comparison/download/staleness,
source conflicts, reload, rollback, and the two-question phone-width flow.

The first pass is source before/after inspection rather than an inline diff
editor. It has no server-retained scenario library or comparison history, no
scheduled comparisons, and no automatic polling. Use refresh to observe another
client's changes. Formative usability evidence is still required before calling
the flow validated for new users.
