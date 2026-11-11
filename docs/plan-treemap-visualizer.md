# Plan: explore a treemap explanation view

Status: **Open design exploration**, 2026-11-10. No visualizer implementation
is selected by this plan. The current text Why view remains the
working baseline. The earlier clickable sketch was a conversation aid, not
an approved layout or a production component.

Related: [browser playground](plan-web-playground.md),
[word-number addition](ideas-sample-projs.md#number-words-addition), and
[provenance semantics](design.md#d20-park-run---why-shows-where-an-answer-came-from).

## What the view should help someone understand

A person who does not write code should be able to answer:

1. What went in, and what came out?
2. Which smaller answers did this calculation use?
3. Which answers came from saved examples, and which came from code?
4. What is missing, and what one answer can I supply to continue?
5. Where can I inspect or edit the relevant example or rule?

The first design case is `WordAdd.eval("twenty-two plus eleven") → 33`.
The important distinction is the rule combining twenty and two, versus the
authored meaning of eleven. A second case replaces eleven with twelve,
leaving a missing vocabulary answer. Shipping is the next real sample for
checking whether the design generalizes. Expense reimbursements could be a
later example, once its policy facts and rules exist as a runnable project.

## Explore alternatives before choosing a layout

"Treemap" describes a family of possible presentations here, not a settled
encoding. Compare these with the same real engine traces:

| Candidate | Potential benefit | Question to resolve |
|---|---|---|
| Nested frames, with readable headers | Containment makes parent/child calls visible; labels get enough room | Does repeated framing waste too much space or feel like an outline with boxes? |
| Ordered icicle/flame-style rows | Depth and siblings are easy to scan; call order can remain stable | Can someone understand containment without a timeline metaphor? |
| Area-weighted treemap | Dense traces can fit into a compact space | What meaningful quantity does area represent, and can leaves stay readable? |
| Existing text tree, optionally collapsible | Familiar labels, good keyboard and narrow-screen behavior | Is better wording and selection enough without another layout? |

Make reviewable alternatives from the actual sample outputs, then choose one
based on comprehension and editing tasks. Do not start with a chart library
or reproduce the earlier sketch by default.

### What rectangle size means is still open

Do not size rectangles by returned numbers: a result of 99 is not more work
than a result of 1, and outputs can also be strings, objects, or booleans.
There are no timing measurements in the current trace, so milliseconds,
execution cost, and proportional performance claims would be invented.

Candidates to compare are space allocated for readable labels, descendant
call count, or equal-width call slots at each depth. Descendant counts measure
recorded call structure, not runtime work: an expensive loop can have no
dispatched children. An area-weighted treemap would need a precise additive
weight definition that avoids counting parent and child work twice.

If no useful quantity warrants area, use an unweighted containment view and
call it an explanation view rather than a quantitative treemap. The display
must say what size means, or avoid suggesting that size measures anything.

## Use the engine's actual provenance

The engine already records a call tree in `internal/engine/trace.go`.
`internal/workspace/workspace.go` exposes a JSON `Step`, and `web/app.js`
renders it with `whyStep`. A step includes:

- the formatted call and output;
- the answer source;
- source path and line;
- an error on the innermost failing call;
- nested dispatched calls, in the order they ran.

This is call provenance, not a trace of every expression, token, assignment,
or arithmetic operator. `NumberWords.parse("twenty-two")` contains two word
lookups, but its `20 + 2` arithmetic is inline code, not another traced node.
Likewise `WordAdd.eval` splits the input inside its body; there is no separate
tokenizer node. Show only recorded calls. Explanatory labels may describe a
function, but must not fabricate executed stages or intermediates.

### Exact examples stop the calculation

For the default expression, the tree contains the addition call, number
parsing calls, and vocabulary lookups. The expression itself has no example.

If someone saves an example for that entire expression, a normal run returns
it directly. Its provenance is one example node with no computed children.
Do not draw hypothetical substeps beneath it. If users need to see the
implementation despite that example, design a separate **Check calculation**
action. `park test` already bypasses the checked function's own examples,
but a selected-check trace is not currently exposed by the workspace API;
that would be a separate backend change. Nested calls during checks still
use their own examples.

### Keep three different concepts separate

| Dimension | Possible labels | Meaning |
|---|---|---|
| Answer origin | Saved example; Code; Unconfirmed code; Resolver proposal; No answer | Where a run's result came from |
| Run outcome | Returned; Needs an answer; Blocked by a child; Error | What happened in this run |
| Implementation check | Not checked; Matches examples; Mismatch; Check incomplete | Whether code agrees with authored examples in a separate check |

The engine source values are `example`, `code`, `unconfirmed code`, `proposed`,
and `unknown`. Do not label all code results "verified" or all returned values
green. Two examples satisfy the current readiness policy, not proof of
correctness. An example-only function is useful data; it does not have a
missing implementation that must be repaired.

Only the innermost failure carries an error message today. Ancestors can be
marked failed because that child did not return. Preserve the distinction
between a missing answer and an execution error, and point the user to the
innermost actionable node. Unreached later calls are absent from the trace;
do not invent them as waiting nodes.

The engine's internal Step has a `Failed` flag, but the workspace JSON Step
currently omits it. If the chosen design needs explicit per-node outcome
badges, expose that field rather than treating the answer-origin field as an
outcome. An unknown lookup is still a question even though its enclosing
calculation could not return.

Test results should refer to the project revision they checked. After an
edit, previous check badges must become stale or disappear until rerun.
Cross-linking per-example checks needs an explicit data mapping; it should
not be inferred from answer-origin colors.

## Interaction questions to prototype

The primary interaction should be selecting a call to inspect its input,
output, origin, and the answer or rule it used. Compare an inline expansion
with a single detail pane; do not assume both are needed.

- **Saved example:** select its precise editable function-table row. Preserve
  the owning file location, including examples authored in another module.
- **Code:** first show the function's description and examples. Source is an
  optional next action, opening the existing editor at the definition.
- **No answer:** reuse the existing question-answer operation, scoped to this
  exact call. Saving adds an example and reruns the original top-level call.
- **Unconfirmed code or proposal:** distinguish a suggested answer from an
  absent answer; offer the appropriate confirmation or correction path.
- **Error:** show the actual message and source link. Do not offer an example
  editor as a universal cure for invalid syntax or failed validation.

Editing arbitrary intermediate results is not part of the first version.
Changing an input belongs at the top-level call or in its example row;
changing a computed result requires a clear choice between adding an example
and changing the rule. Do not make those operations indistinguishable.

The existing workspace bridge has formatted calls and source locations. Before
adding navigation fields, check whether those plus the function snapshot can
identify the right row reliably. If not, propose a structured qualified
function name and canonical input key. A source line alone is not a durable
row identifier after edits.

Repeated calls are separate occurrences. Keep source order, preserve selection
where possible across reruns, and avoid deduplicating them into a graph that
implies a different execution. Any stable visual identity should account for
the call's canonical inputs and its occurrence within its parent.

## Readability and accessibility requirements

- Essential information is available on click, tap, and keyboard focus,
  without relying on hover or color alone.
- Default labels show a short operation, the relevant input, and its result.
  Full calls, large values, and developer details are available on selection.
- Deep traces have a deliberate collapse or drill-down strategy. Tiny leaves
  must not become the only place to find a missing question.
- Selection and drill-down provide orientation back to the top-level call.
- The smallest phone layout remains readable; compare stacking or the text
  tree rather than shrinking a desktop treemap.
- A text representation remains available. Selection must not move keyboard
  focus unexpectedly when the trace updates.
- Comparing reruns must distinguish changed inputs, outputs, and origins. A
  full replay is what happens today; highlighting changes must not claim
  that only those functions were re-evaluated.

## Proposed sequence and review gates

### 1. Collect a small trace corpus

Capture real `workspace.Run` results for:

- default word addition, showing examples composed by code;
- the missing twelve lookup, before and after answering it;
- a whole-expression example, giving a single-node trace;
- invalid `eleven-two`, showing an error and its blocked ancestor;
- a result from unconfirmed code and a resolver proposal;
- shipping, a deeper chain, repeated sibling calls, and large object values.

Keep these as sample data shared by candidate renderers. They must come from
the engine, not a hand-authored idealized computation. Proposal metadata such
as resolver identity is not fully exposed in the current workspace Step;
decide whether the first version needs a bridge extension.

### 2. Review two or three alternatives

Use the same corpus and interaction tasks for every candidate. Include the
current text tree as a baseline. Review a desktop and phone state, a missing
answer, an error, and the single-example case. Choose the layout, size meaning,
selection behavior, and labels before implementing the production renderer.

Ask someone unfamiliar with Park to find why eleven became 11, identify what
made twenty-two become 22, and answer the missing twelve question. Check that
they do not confuse rectangle size with numeric value or origin with test
success. If the nested view makes these tasks harder than the text tree,
revise or stop.

### 3. Build the chosen read-only view

Render the existing `why` data beside the current result, with selection and
source navigation. Keep the text view available. Avoid engine changes unless
a reviewed interaction requires fields the bridge cannot supply.

### 4. Add example and question navigation

Connect selection to the existing tables and question operations. After an
answer, replay the original call and update the trace. Add only the check
status integration that the chosen design actually needs.

### 5. Verify semantics and usability

Automated checks should cover truthful nesting, exact-example leaves,
missing-answer versus error handling, correct row navigation, repeated calls,
and replay after adding an example. Inspect keyboard and phone behavior in
the rendered UI. Do not add timing or incremental-execution claims without
corresponding engine data.

## Decisions still needed

1. Does nesting help the target audience enough to justify another view?
2. Which layout communicates composition best, and what does size mean?
3. How much of a trace is visible before selecting or expanding a call?
4. How are answer origin and separate check results presented without noise?
5. Does this replace the text tree, supplement it, or remain an optional view?

No answer to these questions is implied by the earlier interactive sketch.
