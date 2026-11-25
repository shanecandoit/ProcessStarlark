# Plan: Park as a SaaS product

Status: local drafts, hardened function-cache identity, pinned reviews,
whole-project releases, production evaluation, and rollback implemented,
2026-11-19. Hosted accounts, authenticated roles, tenant permissions, and billing
remain planned. Start with the [cache and release usage guide](cache-releases.md)
and [durable draft guide](saas-usage.md).
The [near-term plan](#near-term-plan) prioritizes the local playground workflow,
behavior comparison, question-flow usability, and measured retention/performance.
Authentication is deferred from this sequence.
The local service can serve the playground with explicit draft saves and
immutable source history; the public static playground remains browser-only.
See the [source version plan](plan-source-versions.md) for this implementation.

Start with the [tested usage guide](saas-usage.md). Its
[workflow data](tutorials/draft-workflow.json) is replayed by
`internal/server/drafts_test.go` and `scripts/draft_walkthrough.py`.

## Product outcome

A rule owner supplies a missing answer, a developer checks the affected rules,
and a reviewer can see exactly what would change before it reaches production.
Every production answer should identify the release that supplied it. An edit
in a draft must never change a live answer until an explicit release action.

Number-word addition introduces examples versus rules. Shipping is the first
business workflow: add Germany's zone, get a quote, check the rules, and review
the source and affected function versions. A useful first session ends with a
reusable answer, not merely a diagram or a version hash.

## What exists now

| Capability | State | Evidence |
|---|---|---|
| Pure evaluation, authoritative examples, questions, provenance | Implemented | Engine, CLI, playground, and their tests |
| Approachable example tables and optional visual explanations | Implemented | Static playground; addition and shipping tours |
| Function hashes and `park version list` | Implemented; syntax/runtime/resolved dependency identity | `internal/engine/version.go`; version tests |
| Draft operations, snapshots, history, forking, source/function diff | Implemented; SQLite by default | `internal/workspace/draft.go` and regression tests |
| Local HTTP draft service | Implemented | `go run ./cmd/park-drafts --root examples/shipping` |
| Immutable source versions | Implemented; SQLite by default | SHA-256 IDs, complete source snapshots, parent links, historical forks |
| Playground draft saves/history | Implemented locally | Optional panel via `--web-dir web/dist`; explicit save and branch |
| Stale-write rejection | Implemented | SHA-256 working-state token checked under the draft lock; HTTP 409 |
| Export a draft as an ordinary Park project | Implemented | ZIP export endpoint and walkthrough test |
| SQLite storage, migration ledger, backup/restore | Implemented | `internal/draftstore`, restart/concurrency/rollback tests |
| Basic question queue and local actor IDs/dates | Implemented | List, answer, dismiss, reopen, retain closed items |
| Safe source formatting and lint | Implemented | Literal-preserving formatter; source-save normalization |
| Authenticated collaboration | Planned | Required before a hosted pilot |
| Review, activate, production evaluation, rollback | Implemented locally | Pinned checks, immutable manifests, atomic generation guard, release walkthrough |
| `park version diff` | Placeholder | Use the implemented draft diff endpoint today |

There is no hosted account service or PostgreSQL/Redis store in this repository.
SQLite persists activity with actor IDs and UTC dates; it is not a tamper-proof
audit log. Temporary memory mode remains available with `--db ""`.
Author names in the local API are supplied labels, not authenticated identities.
A shared playground link contains a copy of a project; it is not collaborative
access to a server draft.

## First milestone: a usable local draft workflow

**Implemented:**

1. Start a service with a read-only reference project bound to a local database.
2. Create an isolated draft from that reference, or fork an existing draft.
3. Run an input. Unresolved calls become questions in that draft only.
4. Add an example or edit source files, with whole-project validation.
5. Rerun the input and explicitly check implemented rules against examples.
6. Review source changes and affected function hashes, including added and
   removed examples. Dependency-only changes remain distinguishable from a
   changed function body.
7. Export portable project files, or back up the database with source history,
   questions, dates, users, and activity. Restart with the same reference/database.

Source versions are immutable SHA-256 objects with full source and parent links.
A draft points to its head. Only changed source creates a version; runs/checks
and queued questions do not. A separate opaque `state_token` guards accepted
operations, including changes to questions and activity. The integer `version`
is retained solely as a deprecated compatibility counter. Rejected operations
leave source, head, token, and history unchanged. GETs do not change state.

Memory operations hold a draft lock; SQLite operations reload persisted state
and compare tokens inside a database write transaction. Source objects, head,
working files/questions, users, and activity commit together. This prevents an
old run or example edit from overwriting a newer edit across independent
connections, not just within one Go process.
Memory drafts have separate locks; SQLite writes serialize across the database.
This initial model trades throughput for testable consistency; it is not a claim about
real-time multi-user editing performance.

The HTTP service accepts JSON requests up to 4 MiB and at most 16 drafts. The
command binds only to loopback IPs. Requests from foreign browser origins are
rejected; there are no CORS permissions. These development boundaries are not
an authentication system. SQLite uses WAL on local disk, foreign keys, and
transactional embedded migrations with recorded names, checksums, and dates.
A different startup reference or unsupported/modified migration is rejected.

With `--web-dir`, the playground evaluates normally in WASM and explicitly
saves formatted source through the API. Its saved-source queue can run on the
server, answer/dismiss questions, and inspect closed items and recent activity.
Ordinary browser questions and check reports are not automatically uploaded.

## Milestone 2: durable, authenticated pilot

This comes before public exposure and hosted production promotion. It does not
block the local playground work in the near-term plan.

- Establish tenant, project, draft, and user membership. Author identity must
  come from the authenticated principal, not a submitted JSON label.
- Implemented locally: persist validated draft state, immutable source ancestry,
  question lifecycle, user IDs/dates, activity, and exact state tokens. Restart
  recovery and consistent backup/restore have automated tests.
- Implemented locally: SQLite compare-and-swap inside immediate transactions.
  Independent connection tests cover stale writes; hosted scaling remains open.
- Enforce tenant/project scope on every read, edit, diff, fork, export, and
  eventual release operation. Test guessed IDs and cross-tenant references.
- Set request, source, history, draft count, execution, and concurrency limits.
  Apply cancellation and resource limits to queued as well as running work.
- Connect the playground to these APIs behind an explicit server workspace
  choice. Preserve the existing local browser project and download path.
- Show save state, conflicts, unsaved work, and last checked revision clearly.
  Retain local edits after a conflict; fetch and review the newer draft rather
  than applying last-write-wins.

Acceptance: two authorized users can edit and review a shared draft, stale
writes are rejected without data loss, restarting recovers the exact state,
and another tenant cannot observe any of it. Passing these tests is a release
gate; an unauthenticated localhost demo does not satisfy it.

## Milestone 3: reviewed releases

**Implemented locally.** [Use the review/release API](cache-releases.md) to
request fresh pinned checks, inspect the full source/function diff, approve
and activate a whole-project release, or reject with a reason. The requester
cannot approve using the same actor label. Each accepted activation advances
one production generation. Approval checks the exact draft source head,
runtime identity, and original production base inside the store transaction.
A source edit or competing activation requires a new review. Runs/questions do
not change the pinned source head.

Every implemented function must be ready and pass all authored cases. Missing
or unconfirmed dependencies raised by checks block review. Example-only tables
are valid records. Approved releases retain exact source files, full function
hashes, checks, runtime, requester/reviewer labels, and dates. Rollback atomically
selects an existing whole-project release and records the previous pointer,
actor, reason, and generation. In-flight evaluation uses its captured release.

These are local workflow guarantees. Hosted release gates still need
authenticated reviewer roles and tenant/project membership. Actor labels do
not prove identity. Per-function promotion remains outside this implementation;
a release always selects a complete project. Rollback cannot undo actions an
external consumer already took based on an earlier answer.

## Version identity and caching

**Implemented.** Exact source versions remain separate from execution identity.
Function identity now hashes parsed syntax rather than collapsed text, preserving
literal whitespace and block structure. It uses the engine's resolved dependency
analysis for helpers, constants, aliases, public functions, and namespace values.
A sorted reachable graph handles recursive groups without cyclic hash equations.
Runtime/hash-schema identity and the pinned interpreter version are included.
Formatting/comments and unrelated functions preserve cached results; dependency
changes invalidate callers. Direct member references avoid unrelated namespace
members, while namespace values deliberately include every member.

Regression tests cover literal whitespace, indentation, helpers/constants,
aliases, namespace values, recursive groups, formatting, unrelated edits, actual
stale disk-cache prevention, and check/cache isolation. Review full source and
pinned check results rather than display hashes alone. Shared or tenant caches
are not implemented and will require scoped keys and permissions. Local drafts
and production API runs evaluate in memory without the CLI's disk cache.

## Milestone 4: production evaluation and operations

Implemented locally: serve an active immutable release, returning its release
ID, runtime, and generation. Hosted evaluation still needs authentication and
project scope.
Return the release ID, outcome, and optional provenance. Keep a request pinned
to one release even if another activates while it runs.

A missing answer should return a structured unresolved outcome. If it creates
work for a rule owner, deduplicate and scope that event to the originating
project and release. It must not silently edit a draft or copy customer inputs
into a broadly shared workspace. Decide retention and redaction with users.

Measure latency, error/unknown rates, resource usage, and cache behavior on an
agreed workload before setting capacity claims or SLOs. Add rate limits,
backpressure, graceful shutdown, recovery drills, and release-aware logging.
The previous speculative 10k RPS, millisecond, and cache-hit-rate figures were
not measured acceptance evidence.

## User journey and guidance

| Person | First useful task | Guidance | Once familiar |
|---|---|---|---|
| Rule owner | Answer Germany's missing zone and reuse it | Table-first walkthrough, visible scope of saving | Compact tables, questions, and history |
| Developer | Check a rule and inspect the affected callers | Code and provenance on demand | Editor, checks, diff, and API reference |
| Reviewer | Understand a pinned change and its check results | Source/example changes, affected functions, exact revision | Local review/release API; playground review/release UI available locally |
| Operator | Recover a release or diagnose an unresolved input | Runbook with tested commands and recovery steps | Metrics, scoped logs, restore and rollback tools |

The current guide must always distinguish browser-only exploration, the local
API, and planned hosted behavior. It should include prerequisites, terminal
commands, expected outcomes, recovery from a stale version, and export before
shutdown. A failure must tell the user what can be retried and what stayed
unchanged. Do not ask beginners to understand hashes before adding an answer.

Keep introduction and visual explanations optional and remember disclosure
choices. A visual node identifies an actual executed call; it should open an
exact example or rule. Checks, draft save state, and release state are separate
concepts. "Ready" means enough examples under policy, not "checked" or "live."

## Verification and definition of done

| Area | Current automated evidence | Remaining gate |
|---|---|---|
| Draft edits | Validation, rejection rollback, SQLite transactions across connections | Hosted tenant scope |
| Snapshots | Copy isolation; SQLite restart recovery and backup restore | Retention and hosted operations |
| Diff | Resolved hashes, source before/after UI, pinned scenario comparison | Broader usability evaluation and retained scenario library |
| API | Documented workflow replayed on memory/SQLite with restart per request | Authenticated tenant scope |
| User guide | Same JSON steps drive Python walkthrough and Go handler tests | Local UI walkthrough and user comprehension study |
| Release | Pinned checks, atomic activation, rollback, restart/backup, local UI and browser end-to-end coverage | Usability evaluation; hosted roles later |

Run `go test ./...`, `go test -race ./internal/draftstore ./internal/workspace ./internal/server`,
`node --test web/*.test.js`, and `sh web/build.sh` before merging.
Replay the guide against a real local service as well as the in-process tests.
Do not claim hosted readiness, compliance, performance, or completed phases
based only on the presence of a struct or a green example walkthrough.

## Near-term plan

Revised 2026-11-19. Cache identity, SQLite persistence, local source history,
review/release APIs, rollback, and playground build verification are implemented.
A first pass of priorities 1–3 is now implemented: local review/release UI,
pinned scenario comparisons, and successive-question progress/reuse. See the
[usage guide](playground-review-guide.md). Browser end-to-end tests cover the
initial flows; broader UX evaluation and the actual [usability sessions](usability-sessions.md)
remain open. The table retains the completion criteria, not a claim that every
priority is complete.

| Priority | Work | Completion criterion |
|---|---|---|
| 1 | **Playground review/release workflow** | Save, inspect changes/checks, approve or reject, evaluate live, and roll back through the UI; stale reviews are clearly explained. |
| 2 | **Behavior comparison report — new proposal** | Run named scenarios against an active release and a pinned draft; show changed answers, newly answered calls, unresolved calls, and errors. |
| 3 | **Question-flow polish and usability sessions** | Users can resolve several missing answers and understand which saved facts the rules reused. |
| 4 | **Retention and measured performance** | Inspect storage/cache growth, define retention rules, and benchmark representative workloads before adding more caching. |

### 1. Playground review/release workflow

Use the existing local APIs and actor labels. Show saved draft state, pinned
check results, review status, and active release as separate states. Let users
inspect source/example changes and affected functions, approve or reject with
the existing reviewer separation, evaluate the active release, and select a
retained release for rollback. Explain stale source, runtime, or production-base
conflicts and preserve unsaved work while the user reviews the newer state.

Acceptance is the shipping workflow completed through the UI without curl:
save Germany's zone, inspect the checked change, activate it, verify the live
quote, and roll back. Include a rejection and a stale-review recovery path.

### 2. Behavior comparison report

Start with a small saved set of named, representative shipping requests. Pin
the scenario set, active release, draft source version, and runtime used by a
report. Compare actual runtime outcomes, including changed answers, previously
unresolved calls that now return answers, calls that become unresolved, and
evaluation errors. Show unchanged outcomes too so the compared set is clear.
Do not silently switch the baseline if production or the draft changes; mark
the displayed comparison stale and let the user request a new report.

Keep scenario comparisons separate from implementation checks. Passing the
selected scenarios does not establish that every possible input is unchanged,
and scenario calls do not automatically become authored examples. Evaluation
must not edit either snapshot or a draft's question queue.

Browser end-to-end tests ship with priorities 1 and 2. Cover actual table edits,
saving, reload, stale checks/reviews, conflict recovery, comparison results,
activation, live evaluation, and rollback. Existing Node/WASM smoke checks remain
useful but do not substitute for exercising the page and its controls.

### 3. Question-flow polish and usability sessions

Keep the original request visible while successive missing calls are answered
in order and the request reruns. Make clear which saved fact was added and
where it is reused; keep source and raw calls optional. Test the multi-question
flow as well as reuse in a second request.

Run formative sessions with people unfamiliar with Park, using the existing
[usability tasks and proposed targets](plan-treemap-visualizer.md#2-compare-an-outline-with-nested-frames-in-the-same-flow).
Record completion, wrong edits, help needed, and users' explanations. Prioritize
observed confusion before adding more visualization or language features.

### 4. Retention and measured performance

Measure source/version, review/release, activity/question, and computed-cache
growth separately. Define retention rules that preserve active releases,
pending-review snapshots, required ancestry, and recovery/export needs. Cache
deletion remains safe; source/release retention needs explicit policies.

Benchmark representative cold and warm workloads, repeated and distinct inputs,
dependency changes, latency, cache hits/misses, and storage growth. Use the
results to decide whether production caching, bounded runtime-call sampling,
eviction, or a different release-state storage layout is warranted. Do not
commit to those implementations or performance targets before measurement.

## Deferred from the near-term sequence

Authentication, tenant/project membership, authenticated release roles, and
hosted operations remain prerequisites for a hosted service. They are not
prerequisites for these local workflows. Billing and integrations follow a
demonstrated pilot need. New value types, override syntax, shared caches, and
additional visualization work remain separate proposals to revisit when the
workflow and usability evidence justify them.
