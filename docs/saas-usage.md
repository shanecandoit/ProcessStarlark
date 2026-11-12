# User guide: work on a rule in a draft

A draft lets you try a change without changing the reference project. In this
walkthrough you add Germany's shipping zone, reuse it in a quote, check the
rules, and review what changed.

**Available today:** a browser playground and a local draft API with SQLite
persistence, tracked migrations, source formatting, and a question queue.
**Planned:** hosted accounts, authenticated shared workspaces, review approvals,
production promotion, and rollback. No command in this guide deploys a change.

## Choose your starting point

| Experience | Use it for | Where changes live |
|---|---|---|
| [Browser playground](https://shanecandoit.github.io/ProcessStarlark/) | Learn examples, rules, and questions without installing anything | This browser, with explicit share/download options |
| Local draft API below | Try the draft workflow, conflict handling, diff, and export | SQLite on local disk; optional temporary memory mode |
| Hosted SaaS | Future collaborative review and production releases | Not available yet |

For a first introduction, use the playground's **Walk me through it** or
[addition tour](https://shanecandoit.github.io/ProcessStarlark/#tour=addition).
Enter `twenty-two plus eleven`, then try the missing twelve meaning. Saving
`12` supplies one word meaning; other expressions can reuse it. You can close
the guide and explanation, and reveal code when needed. Those preferences are
remembered. For source history and server drafts, run the local playground below.
The public static playground remains browser-only.

## View and edit source versions in the playground

From the repository root, build the browser runtime and serve it with the API:

```sh
sh web/build.sh
go run ./cmd/park-drafts --root examples/word-addition --db .park/words.sqlite --web-dir web/dist
```

Open `http://127.0.0.1:8081`. The optional **Drafts and source versions** panel
starts collapsed. Ordinary expression entry, example tables, code, and visual
explanations work as before.

1. Open the panel, enter **Your name (local attribution)**, name your experiment,
   and choose **Save as new draft**.
   This saves the current browser source, starting from the service reference.
   If you chose a different project, the first save replaces the reference's
   source in this new draft only.
2. Try `twenty-two plus twelve`, answer `12`, and run it again. The browser
   computes `34`; the panel now says **local changes**.
3. Choose **Save source version**. It validates the complete source before
   saving. The new version points to the earlier version. Run **Checks**
   separately; saving is not a check or a production release.
4. Select the earlier saved version, expand **Inspect saved source or edit a
   copy**, and choose a file. The preview is read-only;
   inspecting it never replaces your working source. Full version, parent,
   and source hashes appear above the preview.
5. Choose **Edit this version in a new draft** to branch from that saved
   source. Your original draft and its latest source remain available. If you
   have local changes, save or download them first. Opening a server copy
   requires the explicit replacement checkbox when browser source is unsaved.
6. Use **Server drafts**, **Refresh list**, and **Open selected draft** to
   reopen work, including after a server restart with the same database. Source typed while a request runs
   stays in the browser and is marked as needing another save.
7. Download projects for sharing and back up the database for recovery. The
   normal header **Download** saves current browser files, including unsaved
   source and browser questions.

Saves are explicit. Ordinary browser runs/checks/questions stay in WASM.
**Questions for saved source** runs the current input on the server and persists
its questions. Saving source alone does not upload the browser queue or check
reports. See the saved-source queue workflow below.
Finish or cancel any typed table row or question answer before saving source;
the panel labels that state **finish answers or table edits**. Unsaved cells are not files
and are not included in a ZIP download.
A failed save leaves local edits intact. On a conflict, inspect the latest
server version; open that copy after preserving local work, or choose **Save
as new draft** to keep your local changes independently. A conflict never
silently updates your write token and retries an overwrite.

SQLite stores source versions, parent links, questions, local user IDs, UTC
dates, activity, and state tokens across restarts. Browser reload restores
working files, but you must reopen the server draft to attach it. **Recent
activity** shows who performed the latest operations and when. Local names are
unverified labels, not accounts or permission grants. ZIP downloads preserve
project files; database backups preserve version and activity history.

## Keep source tidy

Draft creation, source saves, and example edits normalize source before it is
hashed: at most one blank line outside literals, no trailing spaces/tabs outside
literals, no leading/trailing empty lines, and one final newline. Whitespace in
string/bytes literals and docstrings is preserved, including multiple blank
lines. Indentation is preserved. Historical versions are never reformatted.
Two edits that differ only in this normalized whitespace reuse the same source
snapshot under the same parent; comments and other source changes still count.

In **Show code**, choose **Format source** for the same cleanup in the browser,
or **Lint source** for style suggestions and syntax/project validation. Lint
suggests spaces when indentation uses tabs; it does not guess at indentation
changes. Neither action is a correctness check against examples. Source typed
while a save is running is preserved, including when the server formats it.
The service formats its working copy, not the reference files on disk.

## Work through the saved-source question queue

For a fresh word-number draft, start with the missing twelve meaning:

1. Create/open a draft and save its current source.
2. Put `twenty-two plus twelve` in the expression field.
3. Expand **Questions for saved source** and choose **Run saved source**.
   One question asks for `WordValues.value("twelve")`. Repeating the run keeps
   one open item for the same function/input.
4. Stop the service, start it again with the same `--root` and `--db`, reopen
   the draft, and choose **Refresh questions**. The item and its opened date
   and local actor attribution are still there.
5. Enter `12` in the server answer field and choose **Save answer**. It adds an
   authoritative example, creates a child source version, and resolves the item.
   Text-valued functions such as shipping zones accept plain text answers.
6. Choose **Run saved source** again; the server returns `34`. Select **Show
   closed questions** to inspect the resolved item and who closed it.
7. To defer a question instead, choose **Dismiss question**. It leaves source
   unchanged. Asking that missing call again reopens the same queue item;
   the activity log retains the previous dismissal.

Save unfinished browser source before answering on the server. Source changes
or answers can automatically resolve/prune questions that are no longer
applicable. A `resolved` status means the item left the open queue; it is not a
proof that a rule passes checks. No assignment or notifications are implemented.

The API equivalents are `GET /api/v1/drafts/{id}/questions` for open items,
`?status=all` for history, and the `run`, `add-example`, and `dismiss` operations.
Each question includes an ID, structured call/input, status, opened/updated/
closed times, and actor IDs/names. Queue changes are guarded by the same state
token as source edits.

## Run the complete draft walkthrough

You need a repository checkout, Go 1.26 or newer, and Python 3. There are no
Python package dependencies. Run these commands from the repository root.

**Terminal 1 — start the service:**

```sh
go run ./cmd/park-drafts --root examples/shipping --db .park/shipping.sqlite
```

It listens at `http://127.0.0.1:8081`. The shipping source is copied as the
read-only reference; the service does not write into `examples/shipping`.
This is a local development service without accounts or authentication.
Actor names such as `ops` and `developer` are labels for the walkthrough. The
default storage is `.park/drafts.sqlite` if `--db` is omitted. Use a distinct
database per reference project; a different reference is rejected on restart.

**Terminal 2 — follow the checked guide and save a copy:**

```sh
draft_output_dir=$(mktemp -d)
python3 scripts/draft_walkthrough.py --export "$draft_output_dir/shipping-draft.zip"
```

The script prints a generated draft ID and checks each of these outcomes:

1. **Create a draft:** get its source version and working-state token.
2. **Ask for a quote:** `Shipping.quote("DE", 800)` asks for Germany's zone.
3. **Inspect the queue:** the open question has an ID, status, and actor.
4. **Supply an answer:** save `Zones.zone("DE") == "international"`.
5. **Review queue history:** the same question is now resolved.
6. **Reuse the answer:** the quote returns `2799` cents ($27.99).
7. **Check rules:** implementation examples pass; zone rows remain data.
8. **Review the diff:** only `zones.star` changed; its caller is affected too.
9. **Inspect ancestry:** the source version points to the unchanged parent.
10. **Reject a stale edit:** HTTP 409 preserves the draft. This is expected.
11. **Read activity:** see `ops` and `developer` attribution and UTC dates.
12. **Verify isolation:** the service reference is unchanged.
13. **Export:** download a normal Park project ZIP.

The exact requests and expected outcomes are in
[the executable walkthrough](tutorials/draft-workflow.json). API tests replay
that same file against memory and SQLite, reopening the SQLite database before
every request to verify restart recovery.
The guide uses a new ID on each run; `--draft-id my-shipping-draft` chooses one
explicitly. Reusing an existing ID returns a conflict instead of replacing it.

**Open the exported project:**

```sh
unzip "$draft_output_dir/shipping-draft.zip" -d "$draft_output_dir"
go run ./cmd/park --root "$draft_output_dir/park-project" test
go run ./cmd/park --root "$draft_output_dir/park-project" run 'Shipping.quote("DE", 800)'
```

The last command prints `2799`. You can edit this exported project with the
ordinary CLI. Export preserves code, examples, open questions, and the catalog;
it does not preserve server draft identity, operation history, or approvals.

## Database, migrations, and backups

Restarting with the same database and reference preserves drafts, version
ancestry, questions, user IDs, dates, activity, and exact state tokens. Pending
browser edits still need an explicit save. Temporary mode is available with
`--db ""`; only that mode discards server state on shutdown.

The server embeds the SQL files in `internal/draftstore/migrations/`. Startup
applies missing migrations in a transaction. `schema_migrations` records the
filename, SHA-256 checksum, and UTC application time. A changed applied migration,
a missing migration in the sequence, or an unknown newer schema stops startup.
Add a new migration instead of editing one that has been applied. Migration
numbers order schema changes; source versions continue to use hashes.

Create a consistent database backup, including committed WAL data, while the
service is running or stopped:

```sh
go run ./cmd/park-drafts --root examples/shipping --db .park/shipping.sqlite --backup /tmp/park-shipping-backup.sqlite
```

The destination must not already exist. Store backups wherever you normally
back up local project data. To restore, stop the target service and start it
against the backup file (or a copy) and the same reference project:

```sh
go run ./cmd/park-drafts --root examples/shipping --db /tmp/park-shipping-backup.sqlite
```

Use this command rather than copying only the main database file while WAL
writes are active. ZIP project export is separate: it does not retain database
identity, closed-question records, activity, or the version graph. Exporting or
restoring a database never activates production. The service is still local
and unauthenticated.

## Understand what you are changing

- An **example** answers one exact function input. Adding Germany to the zone
  table supplies a reusable zone meaning, not a special price for one parcel.
- A **rule** computes answers for cases without an exact example. Developers
  change code for structural behavior, such as how weight changes the price.
- A **question** identifies an input that neither examples nor code answered.
  Answer it in the draft and rerun the request. The local API requires this
  explicit rerun; the browser playground reruns its last request automatically.
- A **check** compares implemented rules with their examples. Passing checks
  does not mean every possible input was tested or anything went live.
- A **source version** is an immutable snapshot identified by SHA-256, with
  its own files and a parent hash. `draft.revision` identifies this version;
  `draft.source_revision` hashes exact source content independent of ancestry.
  Reverting to earlier source creates a descendant with a different version
  ID but the same source revision. The hash identifies the saved, formatted source; comments and other source
  changes count after normalization.
- A **state token** guards the current draft, including questions and activity.
  Echo `draft.state_token` as `expected_state` with each operation. Runs/checks
  can change this token without changing the source version. GETs change neither.

The old integer `draft.version` and `expected_version` are deprecated
compatibility fields that count accepted operations. They are not source
version identities. New clients should use the hash token, and send only one
precondition field. A source hash is an identity, not an access credential.

An invalid operation changes neither source, history, nor state token.
A valid run that has no answer is still an accepted operation: its queued
question and history are retained. A check that finds a disagreement is an
accepted check result; inspect `result.status`, not just the HTTP status.

## Make API requests yourself

`curl` examples below are an alternative to the script. Choose a fresh ID if
you already used `shipping-demo`. All POSTs require JSON content type.

```sh
curl -sS http://127.0.0.1:8081/api/v1/drafts \
  -H 'Content-Type: application/json' \
  -d '{"id":"shipping-demo","name":"Add Germany shipping","author":"ops"}'
```

Copy the returned `draft.state_token`. Put that value in `expected_state` in
your next request (replace `PASTE_STATE_TOKEN` below):

```sh
curl -sS http://127.0.0.1:8081/api/v1/drafts/shipping-demo/operations \
  -H 'Content-Type: application/json' \
  -d '{"author":"ops","expected_state":"PASTE_STATE_TOKEN","operation":{"kind":"run","call":"Shipping.quote(\"DE\", 800)"}}'
```

The response contains a new `draft.state_token`, the unchanged source version
in `draft.revision`, a queued question, and `result.status: "unresolved"`.
Use the new token with `operation: {"kind":"add-example", "example":
"Zones.zone(\"DE\") == \"international\""}`. This changes source and creates
an immutable child version. Rerun and check using each newly returned token.
The executable script handles these tokens automatically. Result `output` is
a Starlark literal string, e.g. `"2799"` in JSON, rather than a numeric schema.

| Operation kind | Additional fields | Purpose |
|---|---|---|
| `run` | `call` | Evaluate an input and retain any questions |
| `test` | Optional `only: "Shipping.quote"` | Check all rules or one function |
| `add-example` | `example` | Add an authoritative input/output answer |
| `remove-example` | `call` | Remove that exact example |
| `files` | `files` map, optional `delete` array and `message` | Atomically validate and apply source edits/deletions |
| `dismiss` | `call` | Close an open question without answering it |
| `format` | None | Normalize source and save any actual source change |
| `lint` | None | Return style suggestions and validate the project |
| `load` | None | Refresh and record a draft operation |

File edits take flat `.star` filenames. You cannot edit and delete the same
file in one operation. Invalid source or deleting a required module rejects
the whole operation, preserving the previous working draft.

## Review, experiment, and export

```sh
curl -sS 'http://127.0.0.1:8081/api/v1/drafts/shipping-demo?history=true'
curl -sS http://127.0.0.1:8081/api/v1/drafts/shipping-demo/diff

curl -sS http://127.0.0.1:8081/api/v1/drafts/shipping-demo/fork \
  -H 'Content-Type: application/json' \
  -d '{"id":"shipping-experiment","name":"Try another rule","author":"developer"}'

curl -sS http://127.0.0.1:8081/api/v1/drafts/shipping-demo/export \
  -o "$draft_output_dir/manual-export.zip"
```

Forking copies the current draft, shares its immutable source ancestry, and
starts a new activity history. Add `"from_revision":"FULL_VERSION_HASH"` to
fork a historical source version instead; that branch starts without the
current question queue. Subsequent edits do not cross between drafts.

Read source versions with:

```sh
curl -sS http://127.0.0.1:8081/api/v1/drafts/shipping-demo/versions
# Substitute a full ID from that list for FULL_VERSION_HASH:
curl -sS http://127.0.0.1:8081/api/v1/drafts/shipping-demo/versions/FULL_VERSION_HASH
```

The list walks the head and its parents, newest first. Each object includes
`id`, `parent`, `source_revision`, and its full `.star` `files`. These endpoints
cannot modify a version. `draft.snapshot.revision` is the source content hash;
`draft.revision` is the version ID with ancestry. The export response identifies
the head in `X-Park-Source-Version`.

Diffs list source files and affected functions in sorted order, with full
hashes. Replacing an answer counts as one removed and one added example.
A caller can have a different function hash without a changed body because its
callee changed. These experimental per-function hashes are separate from the
exact whole-project source versions. Review source and check outcomes.
The identity hardening needed for production is described in the
[product plan](plan-saas.md#version-identity-hardening-before-release-storage-or-shared-caches).

There is no **Promote** or **Approve** endpoint yet. Export and review are the
end of today's workflow. A future reviewer will approve a pinned revision and
its matching check report; a later edit will require renewed review.

## Recover from common problems

| Signal | What happened | What to do |
|---|---|---|
| HTTP 409 on an operation | Another accepted operation advanced the draft | GET the latest draft, review the differences, then submit an intentional edit with its current state token |
| HTTP 409 on create/fork | The ID exists, or the local 16-draft limit was reached | Choose a fresh ID; at the limit, keep this database and start another file for additional experiments |
| HTTP 428 | No precondition was supplied | Fetch the draft and include `expected_state` |
| HTTP 422 | Source, literal, call, or evaluation is invalid | Read `result.message`; the previous draft is unchanged |
| HTTP 200 with `result.status: "unresolved"` | A valid run reached a missing answer | Inspect `draft.snapshot.questions`, save an example, then rerun |
| HTTP 200 with `result.status: "failed"` | A rule disagrees with its examples | Inspect `result.lines`; correct the code or answer, then check again |
| HTTP 404 | The ID does not exist in this database or memory session | Check the ID, database path, and whether temporary memory mode was used |
| HTTP 415 / 413 | Wrong content type / request larger than 4 MiB | Send JSON or split source edits into smaller validated requests |
| Export path already exists | The walkthrough avoids overwriting an existing file | Choose another export path; the server draft remains available |
| Startup says the reference differs | The database belongs to another reference | Use its original `--root`, or choose a new `--db` file |
| Startup rejects a migration checksum/schema | Code and applied migrations disagree | Restore the original migration files or run the compatible server version; do not edit the ledger |
| Listen address is already in use | Another service holds that port | Choose another loopback port with `--addr` and use the matching walkthrough `--url` |

Never blindly retry a conflicting source edit with a fresh state token. Keep
your intended change, compare it with the latest source, and decide what to
apply. The local author field does not grant permissions or prove identity.

## Developer verification and current boundaries

```sh
CGO_ENABLED=0 go test ./...
go test -race ./internal/draftstore ./internal/workspace ./internal/server
node --test web/*.test.js
sh web/build.sh
```

The guide replay, stale-write races, snapshot isolation, question retention,
failed validation, diff replacements/deletions, and fork/export behavior are
automated. The HTTP command permits only loopback IPs; it is not a public
hosted service. SQLite persistence and consistent backup/restore are implemented. Tenant
access control, production release manifests, billing, and service-level
guarantees remain future milestones. CI runs the documented HTTP flow on both
backends, source literal-preservation tests, migration upgrades/checksums/rollback,
independent database writers, and question dismissal/reopening flows.

For inspecting a filesystem project today, use `park version list` and
`park run --why`. `park version diff` is currently a placeholder. `--why`
shows actual call provenance; it does not currently print function hashes.

See the [SaaS plan](plan-saas.md), [CLI reference](reference.md), and
[playground guide](plan-web-playground.md).
