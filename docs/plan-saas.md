# Plan: SaaS Product (Draft → Production Promotion)

Status: **Phase 0 complete (2026-11-11)**, Phases 1-5 planned. Builds on per-function content hashing (D406-409 in design.md) and the product path in ideas-direction.md.

Run the current playground locally:
```sh
sh serve.sh        # http://localhost:8000
```
Live at <https://shanecandoit.github.io/ProcessStarlark/> (GitHub Pages, rebuilt on every push to `main`).

---

## Phase 0: Foundation — Function Versioning ✅ COMPLETE

**Goal:** Every function gets a stable, content-addressed hash. Editing one function invalidates only it and its callers.

| Task | Status | Demo |
|------|--------|------|
| `internal/engine/version.go`: `FunctionVersion` struct, hash computation | ✅ Done | CLI: `park version list` shows hash per function |
| Normalize Starlark body (strip whitespace, sort deps, canonicalize names) | ✅ Done | `park version diff` shows semantic diff (ignores formatting) |
| Dependency graph: compute transitive closure of caller hashes | ✅ Done | Edit `zone()` → `quote()` hash changes, `WordValues.value()` unchanged |
| `engine.Cache` keyed by `(fnHash, inputHash)` instead of project revision | ✅ Done | Cache hit rate demo: edit unrelated function → zero cache invalidation |
| Intra-process dependency detection (function A calls B in same process) | ✅ Done | Recursive functions handled without false cycle detection |
| Persist versions to DB (PostgreSQL: `function_versions` table) | 📋 Planned Phase 1 | Survives restart; `park build` loads from DB not filesystem |

**Acceptance:** All tests pass; `park run --why` shows function hash in tree; cache invalidation granularity verified by integration test (`TestCacheInvalidationGranularity`).

**Effort:** Completed in ~1 week (vs 2-3 planned).

---

## Quick Start: Using the CLI Version Commands

```bash
# List all functions with their content hashes, readiness status, and example counts
park version list

# Show semantic diff between two project states (not yet implemented)
park version diff
```

### Example Output

```bash
$ park version list
NumberWords.parse  a8cd0b645892  ready  2 examples
WordAdd.eval       25ea8ced49e9  ready  2 examples
WordValues.value   892a0e1204d3  example-only  27 examples
```

**Columns:**
- **Function** — Qualified name (Process.function)
- **Hash** — First 12 chars of SHA256 content hash
- **Status** — `ready` (≥2 examples, implemented), `needs-examples` (<2 examples, implemented), `example-only` (no body)
- **Examples** — Count of authored examples

### How Hashes Work

The hash is computed from:
1. **Function signature** — name, inputs, output
2. **Normalized body** — whitespace-insensitive, deterministic formatting
3. **Authored examples** — canonical input + output pairs (sorted by input key)
4. **Dependency hashes** — hashes of all called functions (transitive)

**Key property:** Same source = same hash. Editing `Zones.zone` changes its hash, which changes `Shipping.quote`'s hash (since it calls `Zones.zone`), but `WordValues.value` stays the same.

### Cache Invalidation Granularity

```bash
# Before: all functions have stable hashes
$ park version list
Shipping.quote  6e0e1a540d50  ready  3 examples
Zones.zone      d62382e09893  example-only  4 examples

# Add Germany to zone table (edit Zones.zone only)
$ park example add 'Zones.zone("DE") == "international"'

# Only Zones.zone and Shipping.quote hashes change
$ park version list
Shipping.quote  28f0174b21ff  ready  3 examples    # CHANGED (depends on Zones.zone)
Zones.zone      fe5d6e2c7610  example-only  5 examples  # CHANGED

# Restore, add example to Shipping.quote only
$ park example add 'Shipping.quote("DE", 800) == 2799'

# Only Shipping.quote changes; Zones.zone unchanged
$ park version list
Shipping.quote  a08bb447e2cc  ready  4 examples    # CHANGED
Zones.zone      d62382e09893  example-only  4 examples  # UNCHANGED ✓
```

This enables **atomic promotion with minimal cache invalidation** — only the changed function and its transitive callers get new hashes.

---

### Phase 1: Draft Workspace API & Playground Integration

**Goal:** A mutable workspace where business users edit tables, developers edit code, both see live test results.

| Task | Demo |
|------|------|
| `internal/workspace/draft.go`: in-memory `DraftWorkspace` (extends existing `internal/workspace`) | Playground: "Draft" toggle switches to mutable workspace |
| REST API: `POST /api/v1/draft/files`, `GET /api/v1/draft/snapshot`, `POST /api/v1/draft/run`, `POST /api/v1/draft/test` | `curl` script: create draft → edit zone table → run quote → see `--why` |
| Playground integration: "Open in Draft" button, auto-save to localStorage, shareable draft links | Share a draft link; recipient sees same tables + code, can edit |
| Table editor (D21) connected to draft workspace: edits write to draft, not prod | Paste spreadsheet rows into zone table → new examples in draft |
| Developer view: code editor + inline test results (green/red per example) | Edit pricing rule → test panel updates → all green → "Request Promotion" |

**Acceptance:** Two users (ops + dev) can concurrently edit same draft via playground; `park test` runs on every change; draft link preserves state.

**Effort:** 2 weeks

**Demo script (recordable):**
1. Ops opens draft link → adds Germany to zone table → sees `quote(DE, 800)` unresolved
2. Dev opens same link → sees question in panel → adds pricing rule for international
3. Both see `park test` pass → click "Request Promotion" → promotion request created

---

### Phase 2: Promotion Flow (Draft → Production)

**Goal:** Safe, auditable, atomic promotion with approvals.

| Task | Demo |
|------|------|
| Promotion Request resource: `{ id, functions[], requestedBy, testResults, diff, status }` | UI: "Promotion Requests" list with status badges |
| Diff engine: compare draft vs prod function hashes + example rows | Side-by-side diff: old zone table vs new, old pricing code vs new |
| Approval workflow: required reviewers, `park test` gate, `--why` tree preview | Reviewer clicks request → sees `--why` for each changed function → Approve |
| Atomic promote: copy function versions to prod workspace, update prod pointer | "Promote" button → instant; prod `/eval` serves new versions next request |
| Rollback: one-click revert to any prior promoted hash | "Rollback" button on version history → prod serves old hash instantly |
| Notification webhooks (Slack, email) on promotion/rollback | Slack: "Shipping.quote v10 → v12 promoted by @dev" |

**Acceptance:** Promotion request requires 2+ examples per function + passing `park test`; atomic swap verified under concurrent load; rollback restores exact prior behavior.

**Effort:** 2 weeks

**Demo script:**
1. Create promotion request for Shipping + Zones changes
2. Reviewer sees diff: 3 new zone rows, pricing rule updated, 47 examples pass
3. Click Approve → production instantly serves new versions
4. Simulate traffic: `curl /eval` shows new results immediately
5. Click Rollback → traffic reverts to prior version

---

### Phase 3: Production Runtime (`/eval` Endpoint)

**Goal:** Low-latency, horizontally scalable, per-function cached production evaluator.

| Task | Demo |
|------|------|
| `cmd/park-saas-eval`: Go server with `internal/server/handler.go` | `hey -c 100 -n 10000 http://localhost:8080/eval` → p99 < 5ms |
| Per-function cache (Redis or in-memory with TTL) keyed by `(fnHash, inputHash)` | Cache hit rate dashboard: 95%+ after warmup |
| Cold-start: load promoted versions from function store at startup | Restart server → serves traffic in < 2s |
| Unresolved calls → return 422 with question, auto-queue to draft workspace | `curl /eval` with unknown country → 422 + question appears in draft |
| Metrics: latency, error rate, cache hit/miss, version served | Grafana dashboard per tenant |
| Multi-tenant isolation: separate function stores, caches, rate limits | Tenant A edit doesn't affect Tenant B latency or data |

**Acceptance:** 10k RPS sustained on 2 CPU cores; p99 < 10ms; zero draft code execution in prod; unresolved calls create draft questions.

**Effort:** 1-2 weeks

**Demo script:**
1. Load test: 10k RPS quote requests → latency histogram
2. Inject unknown country → 422 response + question appears in draft playground
3. Answer question in draft → promote → production serves answer without restart

---

### Phase 4: Observability & Operations

**Goal:** Production-grade visibility, debugging, compliance.

| Task | Demo |
|------|------|
| Version history UI: every promoted hash, author, timestamp, diff | "History" tab shows v1→v12 timeline with one-click diff |
| `--why` in production: attach provenance to every `/eval` response (optional header) | `X-Park-Why: true` → response includes full call tree |
| Audit log: immutable record of every promotion, rollback, example edit | Export CSV for compliance review |
| Alerting: test failures in draft, promotion stalls, cache hit rate drops | PagerDuty: "Draft Shipping.quote test failing for 15min" |
| Cost tracking: evals per tenant, cache size, storage | Monthly report: $/million evals |

**Acceptance:** Full audit trail for any production answer; compliance export passes review.

**Effort:** 1 week

---

### Phase 5: Multi-Tenant SaaS Hardening

**Goal:** Ready for paying customers.

| Task | Demo |
|------|------|
| Auth: OAuth2/OIDC (GitHub, Google, SAML), API keys for `/eval` | "Sign in with GitHub" → lands in tenant dashboard |
| Tenant provisioning: self-serve signup, trial limits, billing webhook (Stripe) | New tenant: 30s from signup to working draft |
| Quotas: draft size, evals/month, concurrent promotions | Dashboard shows usage bars |
| Data residency: EU/US region selection at tenant creation | "Create tenant in eu-central-1" |
| SLA: 99.9% uptime, <100ms p99 `/eval`, 1hr RPO | Status page with real metrics |

**Effort:** 2-3 weeks

---

## Demo Matrix (What to Show When)

| Audience | Phase 0 | Phase 1 | Phase 2 | Phase 3 | Phase 4 | Phase 5 |
|----------|---------|---------|---------|---------|---------|---------|
| **Internal eng** | `park version diff` cache granularity | Draft playground collab | Promotion atomic swap | Load test + 422→draft flow | Audit export | Self-serve signup |
| **Business buyer** | — | Table editor + live test | "Request Promotion" button | — | Compliance audit trail | Pricing page + trial |
| **Developer** | Hash stability | Code editor + inline tests | Review `--why` before approve | `/eval` latency + metrics | Debug production answer | API keys + docs |
| **Investor** | — | "Two roles, one workspace" | "Zero-downtime deploy" | "10k RPS on laptop" | "SOC2-ready audit" | "Ready to sell" |

**Priority order for maximum signal:**
1. **Phase 1 Draft Playground** — visible, interactive, shows the core handoff
2. **Phase 2 Promotion** — the "aha" moment: business edit → dev review → live in seconds
3. **Phase 3 `/eval`** — proves it's not a toy; production-grade
4. **Phase 0** — enable the above; demo internally first

---

## Reusable Components (Already Built)

| Component | Location | SaaS Role |
|-----------|----------|-----------|
| Starlark engine + dispatcher | `internal/engine/` | Core evaluation (draft + prod) |
| `--why` provenance tree | `internal/engine/trace.go` | Review diff, production debug |
| `internal/workspace` (in-memory) | `internal/workspace/` | Draft workspace ops |
| `engine.Check` (`park test`) | `internal/engine/check.go` | Promotion gate |
| Table editor (D21) | `web/presentation.js` | Business UI in draft |
| Playground share/persist | `web/app.js` | Draft collaboration links |
| Function signatures from code | `internal/project/` | Auto-discovery, no schema files |
| `engine.Cache` interface | `internal/engine/` | Per-function cache impl |

---

## Open Decisions (Resolve Before Phase 1)

| Decision | Options | Recommendation |
|----------|---------|----------------|
| **Draft isolation** | Per-user branch vs shared draft with row locks | Start: shared draft + optimistic locking (last write wins + conflict banner) |
| **Promotion granularity** | Single function vs transitive closure vs user-selected set | Transitive closure (safety); allow "promote all ready" button |
| **Rollback strategy** | Instant hash pointer flip vs graceful drain | Instant flip (pure functions = safe); add `drainSeconds` config later |
| **Canary releases** | Route % traffic to new hash | Phase 4+; start with feature flag per function |
| **Secrets in examples** | Plaintext vs encrypted vs external reference | Phase 1: plaintext with warning; Phase 5: secret manager integration |
| **Multi-region** | Active-passive vs active-active | Phase 5: active-passive per tenant; active-active later |

---

## Success Criteria (Definition of Done per Phase)

| Phase | Must Work | Metric |
|-------|-----------|--------|
| 0 | `park version list` shows hashes; edit A → only A+B invalidated | 0% unnecessary cache invalidation in integration test |
| 1 | Two users edit same draft via playground; test runs on every keystroke | <500ms draft save → test result |
| 2 | Promotion request created → reviewed → approved → prod serves new version | <5s promote latency; 0 failed promotions in chaos test |
| 3 | `/eval` handles 10k RPS p99<10ms; unknown call → 422 + draft question | 95% cache hit rate after 1k warmup requests |
| 4 | Any production answer traceable to promoted hash + author + timestamp | Audit export passes sample compliance review |
| 5 | New tenant signup → working draft in <30s; Stripe webhook updates plan | 0 manual steps for trial provisioning |

---

## Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Function hashing instability (whitespace, reordering) | Medium | High | Canonicalize aggressively; test with `gofmt`-style normalizer |
| Draft conflicts between ops + dev | High | Medium | Optimistic locking + conflict resolution UI; Phase 1 |
| Promotion "works on draft, fails in prod" | Low | High | Identical engine (WASM + Go); prod reads same function store |
| Cache stampede on cold start | Medium | Medium | Pre-warm cache on deploy; staggered rollout |
| Tenant data leakage | Low | Critical | Row-level security in DB; integration test per tenant |
| `park test` too slow for large projects | Medium | Medium | Parallel test execution; incremental (only changed functions) |

---

## Next Steps

1. **Approve this plan** — confirm scope, sequencing, resourcing
2. **Spike Phase 0** — implement function hashing in `internal/engine/version.go` (1 week)
3. **Design review** — hash algorithm, DB schema, API contracts
4. **Kick off Phase 1** — draft workspace API + playground integration

---

## Appendix: Minimal SaaS API Contract (v1)

```yaml
openapi: 3.0.1
info:
  title: Park SaaS API
  version: 1.0.0
paths:
  /api/v1/draft/files:
    post:
      summary: Write a .star file to draft workspace
      requestBody:
        content:
          application/json:
            schema:
              type: object
              required: [path, content]
              properties:
                path: {type: string}
                content: {type: string}
      responses:
        '200': {description: Saved}
  /api/v1/draft/snapshot:
    get:
      summary: Full project snapshot for playground
      responses:
        '200':
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/Snapshot'
  /api/v1/draft/run:
    post:
      summary: Execute a call in draft workspace
      requestBody:
        content:
          application/json:
            schema:
              type: object
              required: [call]
              properties:
                call: {type: string}
      responses:
        '200':
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/RunResult'
  /api/v1/draft/test:
    post:
      summary: Run park test in draft
      responses:
        '200':
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/TestResult'
  /api/v1/promotions:
    post:
      summary: Create promotion request
      requestBody:
        content:
          application/json:
            schema:
              type: object
              required: [functions]
              properties:
                functions: {type: array, items: {type: string}}
    get:
      summary: List promotion requests
  /api/v1/promotions/{id}:
    get:
      summary: Get promotion request with diff
    post:
      summary: Approve and promote
  /api/v1/production/eval:
    post:
      summary: Production evaluation (API key auth)
      requestBody:
        content:
          application/json:
            schema:
              type: object
              required: [call]
              properties:
                call: {type: string}
      responses:
        '200': {description: Result}
        '422': {description: Unresolved - question queued to draft}
  /api/v1/production/functions/{name}/versions:
    get:
      summary: Version history for a function
  /api/v1/production/functions/{name}/rollback:
    post:
      summary: Rollback to specific version
      requestBody:
        content:
          application/json:
            schema:
              type: object
              required: [versionHash]
              properties:
                versionHash: {type: string}

components:
  schemas:
    Snapshot:
      type: object
      properties:
        files: {type: object, additionalProperties: {type: string}}
        questions: {type: array, items: {type: string}}
        processes: {type: array, items: {$ref: '#/components/schemas/Process'}}
    RunResult:
      type: object
      properties:
        status: {type: string, enum: [ok, unconfirmed, unresolved, failed, error]}
        output: {}
        why: {$ref: '#/components/schemas/WhyTree'}
        questions: {type: array, items: {type: string}}
    TestResult:
      type: object
      properties:
        passed: {type: boolean}
        results: {type: array, items: {$ref: '#/components/schemas/TestCaseResult'}}
    WhyTree:
      type: object
      properties:
        call: {type: string}
        output: {}
        source: {type: string, enum: [example, code, unconfirmed, proposed, unknown]}
        location: {type: string}
        children: {type: array, items: {$ref: '#/components/schemas/WhyTree'}}
```

---

*This plan is a living document. Update as phases complete and decisions resolve.*