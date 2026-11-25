# Roadmap

Updated 2026-11-25. This is the product priority list. Detailed implementation
contracts live in [the CLI plan](../plan.md), [SaaS plan](plan-saas.md), and
[design decisions](design.md). There are no delivery dates yet.

## Working today

The CLI and browser playground support examples, deterministic rules, questions,
implementation checks, provenance, and CSV/TSV import. The local backend adds
SQLite persistence, isolated drafts, source history, conflict detection, pinned
reviews, scenario comparisons, release activation, evaluation, and rollback.

The public playground runs in the browser. The backend runs separately on
localhost; actor names are labels. Authenticated collaboration remains planned.

## Next: validate the workflow

Run the [usability sessions](usability-sessions.md) and try one small rule with
a prospective team. Shipping zones, category mappings, and routing tables are
candidate starting points. Record the team's current process and observed
problems before choosing a market or expanding the feature set.

The first milestone is supplying and reusing a missing answer without help or
opening source. Review tasks should establish whether people understand saved,
checked, and live state, comparison coverage, and stale-review recovery.

## Feature priorities

The order below is provisional. Pilot feedback should determine scope.

| Priority | Work | Completion criterion |
|---|---|---|
| 1 | Rule-owner workspace | Answer questions and edit tables with readable values, without source syntax or hashes. Preserve unsaved work and clearly show saved, checked, and live state. |
| 2 | Saved scenarios and comparison history | Retain named scenario sets and reports with their source, release, and runtime pins. Reopen an earlier report without changing its baseline. |
| 3 | Assigned question inbox | Assign missing facts to a project member, deduplicate them, and track resolution. Add notifications if the pilot needs them. |
| 4 | Authenticated shared pilot | Derive identity from sign-in; enforce project membership and reviewer permissions on every operation. Test cross-tenant access and stale writes. |
| 5 | Supported application integration | Document and exercise one application's evaluation flow, including release identity, unresolved outcomes, and errors. Choose an HTTP client or packaged SDK based on that application's needs. |
| 6 | Retention and operational visibility | Measure latency, unknown/error rates, resource use, and storage growth. Define retention, restore, and cleanup policies that preserve releases, reviews, and required source ancestry. |

Authentication and project permissions are prerequisites for exposing the
backend to a shared hosted pilot. Measurement can proceed alongside local
usability work; capacity targets and additional caching should follow results.
See the [local walkthrough](playground-review-guide.md) for the existing flow.

## Later, if a workflow needs it

- **Usage-based impact preview.** Compare a bounded set of retained runtime
  calls across releases. One option is a fixed window of early calls and a
  moving window of recent calls, stored as canonical records with execution
  identity and call order. Define redaction, retention, and coverage before
  recording application inputs. Samples remain separate from examples and
  do not count toward readiness.
- **Money and dates.** Add value types only with explicit representation,
  equality, canonicalization, and input/output rules. Money currently uses
  integer cents in the shipping sample.
- **Intentional exceptions.** Revisit separate override syntax only if a real
  rule needs it. Today every example remains a strict implementation check
  under [D17](design.md#d17-every-example-is-a-strict-implementation-check).
- **External proposals.** The engine resolver hook exists; packaged adapters
  and code-generation workflows do not. Proposed answers require human
  confirmation, and proposed code must pass implementation checks. Keep network
  access and model credentials outside rule code.
- **Cache extensions.** Consider bounded call sampling, cached provenance,
  eviction, or shared caches after measuring workloads. Shared caches also
  require tenant isolation and access control.

Billing, additional integrations, and new visualization layouts have no
committed scope. Revisit them when a pilot establishes a need.
