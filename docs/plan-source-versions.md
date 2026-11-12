# Plan: immutable source versions and playground drafts

Status, 2026-11-11: slices 1 and 2 implemented locally. Durable storage,
authenticated collaboration, and reviewed releases remain planned.

## Model

Use SHA-256, not an incrementing integer, as source version identity. The
existing project revision already hashes sorted filenames and exact source
bytes with length framing and a codec version. Keep that as `source_revision`.
Comments, spacing, examples, private helpers, and constants all count. This is
deliberately an exact source identity, not a claim of semantic equivalence.

A source version stores its full `.star` file snapshot, `source_revision`, and
one `parent` version ID (empty at the root). Its ID hashes a domain-separated,
schema-versioned manifest containing parent and source revision. Different
ancestry can have the same source revision and different version IDs. Returning
to earlier source creates a new descendant; it never rewrites an old version.
Forks initially share an existing version and its ancestry.

A draft is a mutable head pointer plus working questions and activity history.
Only changed source creates a source version. Runs, checks, and generated
questions do not create source versions. A separate opaque SHA-256 `state_token`
guards *all* accepted operations, including question changes and read-only runs.
Clients echo that token; stale tokens return 409 without changing anything.
The old integer `version` / `expected_version` pair remains a deprecated API
compatibility counter, never a source identity or a release reference.

These whole-project versions do not depend on the experimental per-function
hashes. Function cache identity still needs the separate syntax/dependency
hardening described in [the SaaS plan](plan-saas.md).

## Slices and acceptance

1. **Immutable in-memory version graph and API.** Store full source per version;
   retrieve a version and walk its parents; fork from a selected historical
   version. Preserve ancestry and snapshots across forks. Validate edits before
   moving the head. Test deterministic hashing, literal/indentation changes,
   deletion, reverting, copy isolation, rejected edits, and concurrent token
   conflicts. Keep queues/activity separate from source history.
2. **Explicit playground save and history.** Serve the existing playground on
   the local API's origin. Keep its WASM run/check/table workflow. An optional
   collapsed panel creates or opens server drafts, saves validated local source,
   lists immutable versions, shows historical source read-only, and creates a
   new editable draft from a selected version. Saving is explicit; local edits
   survive network errors and conflicts. Opening a draft/version must not
   silently replace unsaved work. Display short IDs with full IDs available;
   users need not type hashes. Checks remain separate from saving or release.
3. **Durability and collaboration.** Persist immutable objects and atomically
   update draft heads/state tokens. Verify restart/restore, transactions across
   processes, quotas, authenticated tenant scope, and membership. This comes
   before public exposure. Local memory and browser recovery are not backups.
4. **Reviewed releases.** Pin whole-project version plus runtime/check report;
   approve and activate through guarded transactions. Test stale approvals and
   rollback. No save or fork in slices 1–2 activates production.

## Verification and guide

Run Go workspace/server tests under the race detector, all Go tests/vet, frontend
tests, and the WASM build. Replay the shipping guide against the HTTP service.
In the playground, save a missing word meaning, inspect the previous version,
branch from it, and confirm both draft heads remain independent. Check stale
saves with two clients and retain the losing client's local edits.

Update the SaaS plan, usage guide, CLI reference, and README with implemented
routes, the local web command, token recovery, and the in-memory boundary.
