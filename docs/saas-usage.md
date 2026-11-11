# SaaS Tool Usage Guide

This guide covers the CLI commands for the Park SaaS product's draft→production workflow.

## Overview

The SaaS workflow uses **content-addressed function versioning** to enable safe, granular promotion from draft to production:

1. **Draft workspace** — Edit examples and code freely; hashes update on every change
2. **Version inspection** — `park version list` shows current hashes and readiness
3. **Promotion** — (Future) Atomic promote of changed functions + transitive callers
4. **Production** — (Future) Serves only promoted hashes with per-function cache

## Current Commands

### `park version list`

Shows all functions with their content hashes, readiness status, and example counts.

```bash
park version list
```

**Output:**
```
NumberWords.parse  a8cd0b645892  ready  2 examples
WordAdd.eval       25ea8ced49e9  ready  2 examples
WordValues.value   892a0e1204d3  example-only  27 examples
```

**Columns:**
| Column | Meaning |
|--------|---------|
| Function | Qualified name (Process.function) |
| Hash | First 12 chars of SHA256 content hash |
| Status | `ready` (≥2 examples, implemented), `needs-examples` (<2 examples, implemented), `example-only` (no body) |
| Examples | Count of authored examples |

### `park version diff`

Shows semantic diff between two project states (planned, not yet implemented).

```bash
park version diff
```

## Understanding Function Hashes

The hash is computed from:
1. **Function signature** — name, inputs, output
2. **Normalized body** — whitespace-insensitive, deterministic formatting
3. **Authored examples** — canonical input + output pairs (sorted by input key)
4. **Dependency hashes** — hashes of all called functions (transitive)

**Key property:** Same source = same hash. This enables:
- Cache reuse across workspaces, tenants, time
- Instant rollback (old hash = old behavior)
- Granular promotion (only changed functions move)

## Cache Invalidation Granularity

When you edit a function, only it and its **transitive callers** get new hashes:

```bash
# Initial state
$ park version list
Shipping.quote  6e0e1a540d50  ready  3 examples
Zones.zone      d62382e09893  example-only  4 examples

# Edit Zones.zone (add Germany)
$ park example add 'Zones.zone("DE") == "international"'

# Only Zones.zone and its caller Shipping.quote change
$ park version list
Shipping.quote  28f0174b21ff  ready  3 examples    # CHANGED (depends on Zones.zone)
Zones.zone      fe5d6e2c7610  example-only  5 examples  # CHANGED

# Restore, edit Shipping.quote only (add example)
$ park example add 'Shipping.quote("DE", 800) == 2799'

# Only Shipping.quote changes; Zones.zone unchanged
$ park version list
Shipping.quote  a08bb447e2cc  ready  4 examples    # CHANGED
Zones.zone      d62382e09893  example-only  4 examples  # UNCHANGED ✓
```

This means production cache invalidation is **minimal and predictable**.

## Readiness States

| Status | Meaning | Caches Results? |
|--------|---------|-----------------|
| `ready` | Implemented + ≥2 examples | ✅ Yes |
| `needs-examples` | Implemented but <2 examples | ❌ No (generates proposals) |
| `example-only` | No implementation (`def f(x): pass`) | ❌ No (dispatches to examples) |

Only `ready` functions use the cache. Unready functions always execute and queue proposals for human confirmation.

## Workflow Example: Adding a New Country

### 1. Check current state
```bash
$ park version list
Shipping.quote  6e0e1a540d50  ready  3 examples
Zones.zone      d62382e09893  example-only  4 examples
```

### 2. Business user adds zone (draft)
```bash
$ park example add 'Zones.zone("DE") == "international"'
Added example in zones.star
```

### 3. Verify hash changes
```bash
$ park version list
Shipping.quote  28f0174b21ff  ready  3 examples    # CHANGED
Zones.zone      fe5d6e2c7610  example-only  5 examples  # CHANGED
```

### 4. Test the change
```bash
$ park test
All 7 examples passed
```

### 5. Run a query to verify
```bash
$ park run 'Shipping.quote("DE", 800)'
2799
```

### 6. (Future) Promote to production
```bash
# park promote --functions Zones.zone,Shipping.quote
# Creates promotion request for approval
```

## Integration with `--why`

The `--why` flag now shows function hashes in the provenance tree:

```bash
$ park run --why 'Shipping.quote("DE", 800)'
2799
  Shipping.quote(country="DE", grams=800) = 2799  code     shipping.star:11  hash=28f0174b21ff
    Zones.zone(country="DE") = "international"    example  zones.star:9      hash=fe5d6e2c7610
```

## Future SaaS Commands (Planned)

| Command | Purpose |
|---------|---------|
| `park promote --functions <list>` | Create promotion request for changed functions |
| `park promote --auto` | Auto-detect changed functions + transitive callers |
| `park promotions list` | List pending/approved promotion requests |
| `park promote approve <id>` | Approve and atomically promote to production |
| `park promote rollback <function> <hash>` | Instant rollback to prior promoted hash |
| `park eval 'P.fn(args)'` | Production evaluation endpoint (low latency, cached) |

## Architecture Notes for Operators

- **Function Store** — Content-addressed (SHA256), append-only, shared across draft/prod
- **Cache Layer** — Keyed by `(function_hash, canonical_input)`, invalidated per-function
- **Promotion** — Atomic swap of function hashes in production workspace
- **Rollback** — Instant pointer flip to prior hash (pure functions = safe)
- **Multi-tenant** — Each tenant has isolated function store + cache

## Troubleshooting

### Hash didn't change after edit
- Check if edit is whitespace-only (normalized away)
- Verify you edited the function's own module, not a dependency

### Cache not working
- Function must be `ready` (≥2 examples + implemented)
- Check `park version list` status column
- Run `park test` to ensure examples pass

### Cycle detection error
- Recursive functions are supported (e.g., `countdown(n-1)`)
- True cycles (A→B→A) are still rejected

## Related Documentation

- [SaaS Product Plan](plan-saas.md) — Full architecture and phases
- [Design Decisions](design.md) — D406-409 per-function versioning
- [Reference](reference.md) — File formats, commands, rules