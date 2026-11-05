# Sample project ideas for Park

The strongest demos have a useful little workflow, several cooperating
processes, and a place where an unknown input can become a new example. A math
exercise shows evaluation; a small tool that gains a new vocabulary entry shows
why Park exists.

These are invented, self-contained project ideas for the v1 CLI. Inputs and
outputs are proposed contracts. They fit v1's strings, integers, booleans,
nulls, lists, and maps.

Keep every process's `.star` module and its single `.yaml` sidecar in the demo
project directory. Each sidecar combines registration, descriptions, signatures,
and authored examples. `park register FILE.star` generates or refreshes it while
preserving authored pairs. `park.html` and the disposable `.cache/` also live in
that directory; `.park/` holds the CLI lock and recovery state.

The YAML below shows selected sidecars. Each function entry names one input and
one output; its rows contain that pair without IDs. Objects hold inputs with
several fields. Add descriptions and implementation references in the same
entry. See the [reference](reference.md) and the [implementation plan](../plan.md)
for the full contract.

The snippets illustrate selected functions, not complete runnable projects. Seed
every implemented function with at least two distinct authored inputs before
executing its body. Example-only functions can start with any number of pairs.

## Shortlist

- Issue router: route issues and teach missing component aliases. Small.
- Data cleanup bench: normalize messy records and validate fields. Small.
- Config migrator: convert structured configs and catch regressions. Small.
- Filename organizer: propose folders using explicit filename rules. Small.
- CLI command translator: compose parsing with vocabulary lookup. Medium.
- Log decoder: explain event codes with example-only knowledge. Small.
- Release notes builder: group and transform lists across processes. Medium.
- Parcel sorter: demonstrate policy thresholds and boundary examples. Small.
- Recipe scaler: compose integer arithmetic with unit conversion. Small.
- Tiny grid game: apply pure state transitions and replay moves. Medium.

## 1. Issue router: best first end-to-end demo

Route a small issue record to a team. The component picks the queue, and an
`urgent` label changes priority. Keep rules explicit rather than inferring
meaning from prose.

Processes:

- `Text.normalize` trims and lowercases component names.
- `ComponentNames.canonical` maps normalized aliases to canonical components.
- `IssueRouter.route` uses both dependencies and chooses a queue and priority.

`component_names.yaml`:

```yaml
version: 1
name: ComponentNames
functions:
  - function: canonical(alias) component
    examples:
      - {alias: "login", component: "auth"}
      - {alias: "sign-in", component: "auth"}
      - {alias: "payments", component: "payments"}
```

`issue_router.yaml`:

```yaml
version: 1
name: IssueRouter
module: issue_router.star
uses: [Text, ComponentNames]
functions:
  - function: route(issue) destination
    implementation: route
    examples:
      - issue: {component: "login", labels: ["bug"]}
        destination: {queue: "identity", priority: "normal"}
      - issue: {component: "payments", labels: ["urgent"]}
        destination: {queue: "checkout", priority: "high"}
```

Leave `ComponentNames.canonical` example-only initially. Give normalization and
routing bodies at least two examples each. With `--interactive`, a new call
whose component is `signin` normalizes successfully, then prompts at the missing
alias. Enter `"auth"`: Park saves the pair, finishes the original call, and
shows the new pair in `park.html`. Without `--interactive`, the unresolved
dependency returns an error.

The prompted answer is short, and the missing knowledge has a clear owner. For
an unrecognized component, ask for its canonical name. A silent default queue
would hide the learning step. Implement queue selection for known canonical
components and document what happens when a taught component has no routing
rule.

## 2. Data cleanup bench: best practical starter

Turn inconsistent imported records into a predictable schema. The caller
supplies parsed records, so Park does not need CSV or filesystem access.

Processes:

- `Text` implements trimming and lowercasing.
- `StatusNames` supplies an example-only status vocabulary.
- `RecordCleanup` assembles and validates the cleaned record.

`status_names.yaml`:

```yaml
version: 1
name: StatusNames
functions:
  - function: canonical(alias) status
    examples:
      - {alias: "in progress", status: "active"}
      - {alias: "wip", status: "active"}
      - {alias: "done", status: "complete"}
```

`record_cleanup.yaml`:

```yaml
version: 1
name: RecordCleanup
module: record_cleanup.star
uses: [Text, StatusNames]
functions:
  - function: clean(record) cleaned
    implementation: clean
    examples:
      - record: {name: "  Ada  ", status: "WIP"}
        cleaned: {name: "Ada", status: "active"}
      - record: {name: "Bo", status: "done"}
        cleaned: {name: "Bo", status: "complete"}
```

Teach `"awaiting review"` as a new status using `--interactive`. Return an
explicit validation result for absent required fields. Distinguish an absent
field from an empty string and null in examples.

Good catalog entries include whitespace cleanup, aliases, already-clean input,
empty names, and unknown statuses. Different map key orders must match the same
authored example.

## 3. Config migrator: best demonstration of regression examples

Convert an invented version-1 config to version 2.

Processes:

- `ConfigValidation` checks required fields and supported versions.
- `Endpoint` builds an endpoint object from host and port fields.
- `ConfigMigration` composes validation and endpoint construction.

`config_migration.yaml`:

```yaml
version: 1
name: ConfigMigration
module: config_migration.star
uses: [ConfigValidation, Endpoint]
functions:
  - function: upgrade(config) migrated
    implementation: upgrade
    examples:
      - config: {version: 1, host: "localhost", port: 8080}
        migrated:
          version: 2
          endpoint: {host: "localhost", port: 8080}
      - config: {version: 1, host: "worker", port: 9000}
        migrated:
          version: 2
          endpoint: {host: "worker", port: 9000}
```

Choose one policy for unknown keys, missing fields, and unsupported versions.
For example, validation returns a list of field errors rather than guessing
defaults. Add examples for each decision before changing the body.

Deliberately introduce a migration bug and show that `park test` catches it by
executing the implementation rather than returning the authored output. Change
the body and show that the old computed cache is no longer used.

## 4. Filename organizer: simplest useful rules demo

Suggest a destination folder from a filename. Return a plan; the caller handles
any file moves outside the pure function.

Processes:

- `Filename.extension` extracts and normalizes an extension.
- `FileKinds.category` maps extensions to folder names.
- `Organizer.destination` composes those functions into a destination path.

`file_kinds.yaml`:

```yaml
version: 1
name: FileKinds
functions:
  - function: category(extension) folder
    examples:
      - {extension: "jpg", folder: "images"}
      - {extension: "txt", folder: "documents"}
```

`organizer.yaml`:

```yaml
version: 1
name: Organizer
module: organizer.star
uses: [Filename, FileKinds]
functions:
  - function: destination(filename) path
    implementation: destination
    examples:
      - {filename: "holiday.JPG", path: "images/holiday.JPG"}
      - {filename: "notes.txt", path: "documents/notes.txt"}
```

Keep extension-to-category mappings example-only so `webp` creates a teaching
moment with `--interactive`. Make case handling, filenames with several dots,
dotfiles, and files without extensions explicit. Start with basenames only.

Four rows in `park.html` make the behavior easy to understand, and each new file
kind makes the catalog visibly grow.

## 5. CLI command translator: a tiny language

Translate a narrow command format into structured actions. Start with
`verb target`: exactly two whitespace-separated words and no quoting rules.

Processes:

- `CommandWords.parse` separates the verb and target.
- `VerbNames.canonical` maps verbs to action names.
- `Actions.translate` composes parsing and vocabulary lookup.

`verb_names.yaml`:

```yaml
version: 1
name: VerbNames
functions:
  - function: canonical(verb) action
    examples:
      - {verb: "show", action: "list"}
      - {verb: "close", action: "complete"}
```

`actions.yaml`:

```yaml
version: 1
name: Actions
module: actions.star
uses: [CommandWords, VerbNames]
functions:
  - function: translate(command) action
    implementation: translate
    examples:
      - command: "show backlog"
        action: {action: "list", target: "backlog"}
      - command: "close TASK-12"
        action: {action: "complete", target: "TASK-12"}
```

Teach `finish` as an alias for `complete`. Add examples for extra whitespace,
unknown verbs, and the wrong number of words. This is an exact little grammar;
free-form natural-language interpretation would be a separate project.

## 6. Log decoder: smallest vocabulary demo

Maintain explanations of invented application event codes. A parser extracts the
code; an example-only function supplies its description and severity.

Processes:

- `LogLine.parse` extracts fields from a fixed line format.
- `EventCodes.describe` holds example-only event knowledge.
- `LogExplanation.explain` combines parsed fields and event descriptions.

`event_codes.yaml`:

```yaml
version: 1
name: EventCodes
functions:
  - function: describe(code) event
    examples:
      - code: "Q101"
        event: {severity: "info", meaning: "job accepted"}
      - code: "Q409"
        event: {severity: "warning", meaning: "duplicate job ignored"}
```

Use lines such as `Q101 job=17`, with a fixed, documented format. With
`--interactive`, an unknown code prompts for a structured answer. Keep the
lookup input to the stable event code. Passing an entire line with a different
job ID every time would prevent useful reuse of the event examples.

This demonstrates how the input boundary determines which examples can be
reused.

## 7. Release notes builder: best list-processing demo

Group supplied change records into release notes. The caller supplies the data;
no GitHub or Git access is required.

Processes:

- `ChangeKinds.canonical` maps change kinds to group names.
- `ChangeSummary.render` produces a summary for each change.
- `ReleaseNotes.group` preserves change order within each group.

`release_notes.yaml`:

```yaml
version: 1
name: ReleaseNotes
module: release_notes.star
uses: [ChangeKinds, ChangeSummary]
functions:
  - function: group(changes) notes
    implementation: group
    examples:
      - changes: [{kind: "feat", title: "Add search"}]
        notes: {Added: ["Add search"], Fixed: []}
      - changes: [{kind: "fix", title: "Handle empty input"}]
        notes: {Added: [], Fixed: ["Handle empty input"]}
```

Teach mappings such as `bugfix` to `Fixed`. Add examples with multiple records
to pin down input ordering. Cover empty input and repeated titles explicitly. A
later `render` function can turn the grouped map into Markdown.

## 8. Parcel sorter: best boundary-testing demo

Choose a sorting lane using an invented warehouse policy:

- Parcels weighing at most 1,000 grams go to `small`.
- Heavier parcels go to `large`.
- Fragile parcels always go to `manual`.

Processes:

- `ParcelValidation` rejects invalid weights and malformed records.
- `LanePolicy` applies the weight and fragility rules.
- `ParcelSort` returns the selected lane and explanation.

`parcel_sort.yaml`:

```yaml
version: 1
name: ParcelSort
module: parcel_sort.star
uses: [ParcelValidation, LanePolicy]
functions:
  - function: route(parcel) decision
    implementation: route
    examples:
      - parcel: {weight_g: 1000, fragile: false}
        decision: {lane: "small", reason: "at or below weight limit"}
      - parcel: {weight_g: 1001, fragile: false}
        decision: {lane: "large", reason: "above weight limit"}
      - parcel: {weight_g: 500, fragile: true}
        decision: {lane: "manual", reason: "fragile"}
```

Cover zero and negative weight through an explicit invalid-input result. Use
this project to show how examples pin down boundaries. Remove enough examples to
leave one distinct authored input. The body then returns to `needs examples`.

## 9. Recipe scaler: best friendly arithmetic demo

Scale ingredient quantities using grams and integer serving counts. Initially
require results to be whole grams; return a validation result when an exact
integer answer is impossible. Avoid silently truncating fractional amounts.

Processes:

- `Units.to_grams` converts a quantity object into integer grams.
- `Portions.scale` uses quantity, original servings, and requested servings.
- `Recipe.scale` applies those operations to a list of ingredient records.

`portions.yaml`:

```yaml
version: 1
name: Portions
module: portions.star
functions:
  - function: scale(request) grams
    implementation: scale
    examples:
      - request: {grams: 300, from_servings: 2, to_servings: 4}
        grams: 600
      - request: {grams: 300, from_servings: 2, to_servings: 1}
        grams: 150
```

`units.yaml`:

```yaml
version: 1
name: Units
module: units.star
functions:
  - function: to_grams(quantity) grams
    implementation: to_grams
    examples:
      - {quantity: {amount: 2, unit: "kg"}, grams: 2000}
      - {quantity: {amount: 250, unit: "g"}, grams: 250}
```

Each function receives one object, rather than several positional arguments. Add
zero-serving and indivisible-quantity examples. Keep unit conversions explicit
and ingredient-independent. Converting cups to grams would need an ingredient's
density as an input or lookup and is a later extension.

## 10. Tiny grid game: most visual demo

Move a robot on a bounded grid. Pass state into every call and return the new
state; there is no hidden mutable game session.

Processes:

- `Directions.delta` converts canonical directions into coordinate changes.
- `Grid.contains` checks a position against the grid dimensions.
- `Robot.step` applies one move and returns the resulting state.

`robot.yaml`:

```yaml
version: 1
name: Robot
module: robot.star
uses: [Directions, Grid]
functions:
  - function: step(request) state
    implementation: step
    examples:
      - request:
          state: {x: 0, y: 0, width: 3, height: 3}
          direction: "east"
        state: {x: 1, y: 0, width: 3, height: 3}
      - request:
          state: {x: 2, y: 0, width: 3, height: 3}
          direction: "east"
        state: {x: 2, y: 0, width: 3, height: 3}
```

Define `north` as decreasing y. Out-of-bounds moves leave the state unchanged.
Teach `right` as an alias for `east` in an example-only direction vocabulary
within its process sidecar. Then replay a move list through `Robot.walk`,
passing starting state and moves in one object. Identical inputs produce
identical results. Obstacles are a later extension.

## A reusable demo story

Use the issue router first, with data cleanup as the next sample. Both split
naturally between implemented transformations and example-only knowledge.

1. Open `park.html` and inspect processes, exports, and seed examples.
2. Run a known input and get an authored output.
3. Run a new input whose dependencies are covered; execute the body and cache
   it.
4. Repeat that call to demonstrate a computed cache hit. For these tiny
   functions, this demonstrates behavior rather than a meaningful performance
   improvement.
5. Run a different input with an unknown alias using `--interactive`. Show the
   unresolved dependency and enter an output value; the original call then
   finishes, and later runs reuse the saved pair.
6. Add enough authored examples to reach six for one function. Check that the
   page shows rows 1, 2, 5, and 6 with two omitted. Remove the sixth pair by its
   input and check that the page now shows rows 1, 2, 4, and 5 with one omitted.
   These numbers describe list positions, not example IDs.
7. Change an implementation, run `park test`, and demonstrate cache
   invalidation.

For example, teach an alias directly or remove it by input:

```sh
park example add --input '"signin"' --output '"auth"' ComponentNames.canonical
park example remove --input '"signin"' ComponentNames.canonical
```

Example lookup is exact after canonical encoding. Adding `"signin"` does not
teach an algorithm that recognizes arbitrary login-related phrases.
Generalization comes from an implemented function.

The only shipped fixture today is `fixtures/roman/`. A second fixture based on
the issue router would exercise more of Park: include two distinct examples for
every implemented function, at least one example-only dependency, one
unknown-input teaching path, and one failing-body test case, while keeping the
project small enough to understand from the catalog.

## V2 demo extension: first and recent calls

Once v1 works, extend the issue router demo with a bounded per-function call
cache. Store each function's first few successful calls and most recent few as
canonical JSON input/output records. Keep the first window fixed and roll the
recent window forward. Store overlapping calls once.

- Run enough inputs to fill both windows, then run several more.
- Show that the earliest retained pairs remain while the recent pairs advance.
- Include calls answered by authored examples, cache hits, and implementations.
- Inspect records containing function, input, output, revision, and call order.
- Keep this runtime view separate from first/last authored examples in YAML.
  Retained calls do not count toward the two-example requirement.

Window sizes and retention details belong to v2. The v1 demo only needs its
computed-result cache and authored-example catalog.
