# Plan: Park as a SaaS product

Status: local draft API with SQLite persistence, tracked migrations, source
hygiene, and a durable question queue implemented, 2026-11-11. Hosted accounts,
approvals, production releases, and billing are planned. See the
[durable draft plan](plan-durable-drafts.md).
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
| Function hashes and `park version list` | Implemented, experimental identity scheme | `internal/engine/version.go`; version tests |
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
| Review, promote, production evaluation, rollback | Planned | No production pointer or promotion command exists |
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

This comes before public exposure and production promotion, not after them.

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

Start with **immutable whole-project releases** and one atomic active-release
pointer per project. This is easier to reason about than mixing independently
selected function versions whose dependencies may no longer agree.

A review request pins the draft version, source revision, full file snapshot,
base release, diff, and check report. Any source change requires a new review;
a production-base change requires an explicit rebase or a rejected activation.
Approval checks and pointer replacement occur in a transaction. Two approvals
must not race to overwrite the same production generation.

Promotion gates:

- source validates with the same runtime and limits used by evaluation;
- every implemented function meets readiness policy and passes its examples;
- example-only tables remain valid data and do not need two examples or code;
- unresolved dependencies and unconfirmed check results block activation;
- the check report belongs to the exact reviewed snapshot and runtime version;
- the reviewer has the appropriate project role.

Record the release manifest, authenticated requester/reviewer, checks, and
previous active release. Rollback activates a prior complete release through
the same guarded pointer update. Purity helps reproduce evaluation; it does
not undo actions an external consumer already took based on earlier answers.

Per-function promotion can follow after a dependency-closure model is proven.
It must include required callees and affected callers and cannot rely on
truncated display hashes. Approvals and notifications are distinct operations;
webhooks are a later, explicitly configured integration.

Acceptance: draft edits leave live evaluation unchanged, approved activation
changes the entire project atomically, stale approvals cannot activate, and
rollback restores the previous complete behavior under concurrent requests.

## Version identity hardening before release storage or shared caches

Whole-project source versions hash exact source bytes and ancestry, including
helpers, constants, indentation, and literals. The following gaps concern the
separate experimental **per-function** hash/cache scheme:

- Body normalization currently collapses whitespace textually. It must preserve
  whitespace inside literals and the semantics of block indentation.
- Private helpers, module constants, function values, and namespace references
  must be included through resolved dependencies. Existing engine dependency
  analysis should drive this instead of a second approximate call graph.
- Cross-process dependency tracking is currently conservative; minimal cache
  invalidation is not a guarantee. Recursive groups need an explicit strategy.
- Runtime/hash-schema versions must be included where different semantics
  could otherwise reuse the same cache key or stored identity.
- Tenant-scoped cache keys must prevent cross-tenant value disclosure, even
  when identical source produces identical hashes. Hashes are not permissions.

Required regression cases include string-literal whitespace changes, moved
blocks with changed behavior, helper/constant edits, aliases, recursive groups,
format-only changes, and unrelated function edits. Until this is hardened,
review full source diffs and do not use function hashes alone as release gates.
The local draft service evaluates in-memory projects without the disk cache.

## Milestone 4: production evaluation and operations

Serve only an active immutable release, authenticated and scoped to its project.
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
| Reviewer | Understand a pinned change and its check results | Source/example changes, affected functions, exact revision | Review queue and release history, when implemented |
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
| Draft edits | Validation, rollback on rejection, true concurrent operations | Durable transactions across processes |
| Snapshots | Copy isolation; SQLite restart recovery and backup restore | Retention and hosted operations |
| Diff | Sorted output, full hashes, replacements and deleted functions | Hardened identity/dependencies and source diff UI |
| API | Documented workflow replayed on memory/SQLite with restart per request | Authenticated tenant scope |
| User guide | Same JSON steps drive Python walkthrough and Go handler tests | Hosted UI walkthrough and user comprehension study |
| Release | Not implemented | Pinned checks, roles, atomic activation, stale approvals, rollback |

Run `go test ./...`, `go test -race ./internal/draftstore ./internal/workspace ./internal/server`,
`node --test web/*.test.js`, and `sh web/build.sh` before merging.
Replay the guide against a real local service as well as the in-process tests.
Do not claim hosted readiness, compliance, performance, or completed phases
based only on the presence of a struct or a green example walkthrough.

## Next implementation sequence

1. Harden per-function cache identity and add the regression cases above.
   Immutable whole-project source versions and local playground history are built.
2. SQLite persistence/migrations are built. Add authenticated, scoped workspaces
   and membership before hosting.
3. Extend the local playground connection to authenticated durable workspaces
   and pinned server check reports.
4. Build pinned review requests and immutable whole-project releases.
5. Add release-only evaluation and operational recovery, then consider billing
   and additional integrations after the pilot workflow is demonstrated.
