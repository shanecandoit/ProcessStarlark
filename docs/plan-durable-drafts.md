# Plan: durable drafts, source hygiene, and a basic question queue

Status: implemented locally, including automated restart-per-request HTTP
walkthroughs and pull-request CI. Authentication, tenant membership, and reviewed
releases remain future work.

## Scope and order

1. Add safe source formatting: at most one blank line outside literals, no
   trailing horizontal whitespace outside literals, and one final newline.
   Preserve string/bytes literals, comments, indentation, and explicit line
   continuations. Parse before and after formatting; reject invalid source.
   Apply formatting on draft source saves and example edits, before hashing.
   Offer explicit format/lint in the playground; never reformat historical
   versions or rewrite files on disk merely by reading them.
2. Add a server-only `database/sql` store using CGo-free modernc SQLite. Embed
   numbered SQL migrations; record name, SHA-256 checksum, and UTC applied time.
   Apply each migration transactionally, refuse altered applied migrations or
   an unknown schema, and support concurrent startup safely.
3. Persist the reference project, immutable source/version ancestry, complete
   working draft state, state tokens, dates, stable local user IDs, and activity.
   Commit source, head, questions, and activity together. Reload state inside
   each database transaction and check its token so independent connections
   cannot overwrite one another. A rejected operation writes nothing.
4. Expose a basic per-draft question queue: list open calls, deduplicate repeat
   calls, answer via an authoritative example, and dismiss without answering.
   Record opened/updated/closed dates and actor IDs, retain resolved/dismissed
   items for inspection, and reopen a dismissed call when it is asked again.
   Answering/pruning removes a call from the open queue. Generated questions
   change working state, not source versions. No assignment/notifications yet.
5. Add automated end-to-end HTTP flows over a temporary SQLite file: create,
   run into a question, restart, answer, rerun/check, inspect source ancestry,
   dismiss/reopen, reject a stale client, fork, and export. Reuse the existing
   executable shipping guide against memory and SQLite backends. Test migration
   upgrade/checksum/failure, rollback, backup recovery, literal-safe formatting,
   independent writers, and CGo-disabled builds. Keep the browser build free
   of the database dependency.
6. Update README, reference, SaaS plan, and user guide after implementation.
   Document database location, migrations, restart behavior, consistent backup,
   source normalization, queue lifecycle, and conflict recovery.

## Model and boundaries

Source version IDs remain SHA-256 of parent plus source revision. Hash the
actual formatted source snapshot, not a second hidden representation. Keep
author/time attribution in activity records outside the source identity.
Historical source stays immutable; a formatting change creates a descendant.

Use one persisted reference project per database for this local service. Refuse
a different startup reference instead of silently changing existing draft
bases. Store source objects separately from per-draft mutable state and use
foreign keys. Preserve the exact state token across restart. Queue and activity
records belong to a draft and cannot be fetched through another draft.

Local user IDs identify submitted actor labels; they do not authenticate users.
The service remains loopback-only. Authentication, membership, tenant scoping,
pinned server check reports, reviewed releases, and per-function cache hash
hardening remain later slices. SQLite lives on local disk, not a shared network
filesystem. No automatic destructive down migrations.

Implementation uses [`modernc.org/sqlite`](https://pkg.go.dev/modernc.org/sqlite)
and SQLite's [consistent `VACUUM INTO` backup](https://www.sqlite.org/lang_vacuum.html).
CI in `.github/workflows/test.yml` exercises the CGo-disabled server, independent
writers, migrations, rollback, and the executable HTTP guide with a fresh SQLite
connection/handler for every request.
