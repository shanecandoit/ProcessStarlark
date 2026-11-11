# User guide: work on a rule in a draft

A draft lets you try a change without changing the reference project. In this
walkthrough you add Germany's shipping zone, reuse it in a quote, check the
rules, and review what changed.

**Available today:** a browser playground and a local draft API prototype.
**Planned:** hosted accounts, shared durable workspaces, review approvals,
production promotion, and rollback. No command in this guide deploys a change.

## Choose your starting point

| Experience | Use it for | Where changes live |
|---|---|---|
| [Browser playground](https://shanecandoit.github.io/ProcessStarlark/) | Learn examples, rules, and questions without installing anything | This browser, with explicit share/download options |
| Local draft API below | Try the draft workflow, conflict handling, diff, and export | Server memory until it stops |
| Hosted SaaS | Future collaborative review and production releases | Not available yet |

For a first introduction, use the playground's **Walk me through it** or
[addition tour](https://shanecandoit.github.io/ProcessStarlark/#tour=addition).
Enter `twenty-two plus eleven`, then try the missing twelve meaning. Saving
`12` supplies one word meaning; other expressions can reuse it. You can close
the guide and explanation, and reveal code when needed. Those preferences are
remembered. The playground is not connected to the local draft API yet.

## Run the complete draft walkthrough

You need a repository checkout, Go 1.26 or newer, and Python 3. There are no
Python package dependencies. Run these commands from the repository root.

**Terminal 1 — start the service:**

```sh
go run ./cmd/park-drafts --root examples/shipping
```

It listens at `http://127.0.0.1:8081`. The shipping source is copied as the
read-only reference; the service does not write into `examples/shipping`.
This is a local development service without accounts or authentication.
Actor names such as `ops` and `developer` are labels for the walkthrough.

**Terminal 2 — follow the checked guide and save a copy:**

```sh
draft_output_dir=$(mktemp -d)
python3 scripts/draft_walkthrough.py --export "$draft_output_dir/shipping-draft.zip"
```

The script prints a generated draft ID and checks each of these outcomes:

1. **Create a draft:** an isolated copy of the shipping reference, version 0.
2. **Ask for a quote:** `Shipping.quote("DE", 800)` asks for Germany's zone.
3. **Supply one answer:** save `Zones.zone("DE") == "international"`.
4. **Reuse the answer:** rerun the quote; it returns `2799` cents, or $27.99.
5. **Check the rules:** the implemented pricing rule agrees with its examples.
   The zone table is reported as example-only data, not as tested code.
6. **Review the diff:** only `zones.star` changed. `Zones.zone` and its caller
   `Shipping.quote` have affected function versions. The pricing body did not
   change.
7. **See conflict handling:** an intentionally stale edit gets HTTP 409 and
   does not change the draft. This is an expected successful guide step.
8. **Read history:** see which operations were labeled `ops` or `developer`.
9. **Verify isolation:** the service reference is still unchanged.
10. **Export:** save a normal Park project ZIP, including your new example.

The exact requests and expected outcomes are in
[the executable walkthrough](tutorials/draft-workflow.json). API tests replay
that same file, so these steps are checked against the implementation.
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

**Before stopping:** export any draft you want to keep. Stopping or restarting
the API process discards its drafts. Export is not an automatic server backup,
and starting with an exported project as `--root` creates a new reference;
it does not activate production.

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
- A **draft version** is an operation counter. Accepted runs and checks advance
  it too. A **source revision** describes the source content; it can remain
  unchanged while the operation counter advances. GET requests do neither.

An invalid operation changes neither source, history, nor the version counter.
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

curl -sS http://127.0.0.1:8081/api/v1/drafts/shipping-demo/operations \
  -H 'Content-Type: application/json' \
  -d '{"author":"ops","expected_version":0,"operation":{"kind":"run","call":"Shipping.quote(\"DE\", 800)"}}'

curl -sS http://127.0.0.1:8081/api/v1/drafts/shipping-demo/operations \
  -H 'Content-Type: application/json' \
  -d '{"author":"ops","expected_version":1,"operation":{"kind":"add-example","example":"Zones.zone(\"DE\") == \"international\""}}'
```

Responses contain `draft.version`, `draft.snapshot`, and, for operations,
`result`. In the last response, version is 2 and the question list is empty.
Rerun the quote with `expected_version: 2`, then check with the returned
version. The result's `output` is a Starlark literal string, e.g. `"2799"` in
JSON, rather than a JSON numeric schema.

| Operation kind | Additional fields | Purpose |
|---|---|---|
| `run` | `call` | Evaluate an input and retain any questions |
| `test` | Optional `only: "Shipping.quote"` | Check all rules or one function |
| `add-example` | `example` | Add an authoritative input/output answer |
| `remove-example` | `call` | Remove that exact example |
| `files` | `files` map, optional `delete` array and `message` | Atomically validate and apply source edits/deletions |
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

Forking copies the current draft but starts a new operation counter and history;
its original reference revision is retained. Subsequent edits do not cross
between the parent and fork.

Diffs list source files and affected functions in sorted order, with full
hashes. Replacing an answer counts as one removed and one added example.
A caller can have a different hash without a changed body because its callee
changed. Hashes are inspection aids today; review source and check outcomes.
The identity hardening needed for production is described in the
[product plan](plan-saas.md#version-identity-hardening-before-release-storage-or-shared-caches).

There is no **Promote** or **Approve** endpoint yet. Export and review are the
end of today's workflow. A future reviewer will approve a pinned revision and
its matching check report; a later edit will require renewed review.

## Recover from common problems

| Signal | What happened | What to do |
|---|---|---|
| HTTP 409 on an operation | Another accepted operation advanced the draft | GET the latest draft, review the differences, then submit an intentional edit with its current version |
| HTTP 409 on create/fork | The ID exists, or the local 16-draft limit was reached | Choose a fresh ID; export drafts before restarting to clear the prototype |
| HTTP 428 | No expected version was supplied | Fetch the draft and include `expected_version` |
| HTTP 422 | Source, literal, call, or evaluation is invalid | Read `result.message`; the previous draft is unchanged |
| HTTP 200 with `result.status: "unresolved"` | A valid run reached a missing answer | Inspect `draft.snapshot.questions`, save an example, then rerun |
| HTTP 200 with `result.status: "failed"` | A rule disagrees with its examples | Inspect `result.lines`; correct the code or answer, then check again |
| HTTP 404 | The ID does not exist in this process | Check the ID and whether the service restarted |
| HTTP 415 / 413 | Wrong content type / request larger than 4 MiB | Send JSON or split source edits into smaller validated requests |
| Export path already exists | The walkthrough avoids overwriting an existing file | Choose another export path; the server draft remains available |

Never blindly retry a conflicting source edit with a new version number. Keep
your intended change, compare it with the latest source, and decide what to
apply. The local author field does not grant permissions or prove identity.

## Developer verification and current boundaries

```sh
go test ./...
go test -race ./internal/workspace ./internal/server
node --test web/presentation.test.js
sh web/build.sh
```

The guide replay, stale-write races, snapshot isolation, question retention,
failed validation, diff replacements/deletions, and fork/export behavior are
automated. The HTTP command permits only loopback IPs; it is not a public
hosted service. Persistent storage, tenant access control, production release
manifests, billing, and service-level guarantees remain future milestones.

For inspecting a filesystem project today, use `park version list` and
`park run --why`. `park version diff` is currently a placeholder. `--why`
shows actual call provenance; it does not currently print function hashes.

See the [SaaS plan](plan-saas.md), [CLI reference](reference.md), and
[playground guide](plan-web-playground.md).
