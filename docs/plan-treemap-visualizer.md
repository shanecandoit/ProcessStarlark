# Plan: explore a treemap explanation view

Status: **Open design exploration**, 2026-11-10. No visualizer implementation
is selected by this plan. The current text Why view remains the
working baseline. The earlier clickable sketch was a conversation aid, not
an approved layout or a production component.

Related: [browser playground](plan-web-playground.md),
[word-number addition](ideas-sample-projs.md#number-words-addition), and
[provenance semantics](design.md#d20-park-run---why-shows-where-an-answer-came-from).

## Recommendation after product, design, and engineering review

The natural-language addition parser already exists in
`examples/word-addition/` and is the default sample and first-visit tour.
The next step is a small onboarding prototype, not another parser or a full
treemap implementation. The following are review perspectives and proposals,
not findings from user research or independent reviewers.

The first question is **can someone supply a missing answer and understand
where it applies?** Understanding a call hierarchy is secondary. A visually
attractive tree does not by itself teach the difference between an example
and a rule.

### Product perspective: define the learning outcome

Target someone who knows the answer to an everyday question but does not
write code. After the first session, they should understand that:

- a saved example answers one exact call;
- a rule can combine saved answers to handle new calls;
- adding one word meaning can help several expressions without teaching a
  new rule;
- Park asks only when execution reaches a call that nothing can answer.

Use a small task to demonstrate this: calculate `twenty-two plus eleven`,
change eleven to twelve, supply the missing meaning, then try
`twelve plus two`. The final reuse matters: obtaining 34 alone could leave
someone thinking they saved the answer for the whole addition.

Number words introduce the mechanism. Shipping should then demonstrate the
practical handoff between a person maintaining a table and a developer
maintaining a rule. Avoid positioning Park as a general natural-language
calculator or suggesting that examples automatically generate code.

### Senior design perspective: show an answer before a call tree

The current playground gives half its desktop width to source, asks for a
"Call" such as `WordAdd.eval(...)`, and links trace rows directly to source
locations. Those are useful developer tools but require programming concepts
before a new visitor can try the idea. The current mobile layout also places
the source pane before the run area.

Prototype this hierarchy for the introductory experience:

1. **Try an expression:** a plain text input containing
   `twenty-two plus eleven`, with the sample's selected function handling it.
   This is a friendlier input to that function, not arbitrary language routing.
2. **Answer:** show 33 prominently, or a specific question when blocked.
3. **How this answer was made:** show the useful smaller inputs and results,
   with saved answers and rules distinguished by words and symbols.
4. **Inspect a piece:** reveal its example row or rule description. Function
   names, source, and the complete execution tree are available on demand.

Keep the input, result, and next required action together. In the missing-word
state, the question "What number does twelve mean?" should be visible without
finding a small rectangle or scrolling through every function table.

The primary action can remain **Save example**, with scope stated beside it:
"Save a meaning for twelve. Other expressions can use it too." A later action
that saves a whole-expression result must describe that different scope.
Avoid "Teach Park the rule," which implies inference that does not occur.

Start with two candidates: a plain-language expandable outline and nested
frames. Both should use the same labels, details, and question editor. This
isolates whether nesting helps; comparing a polished graphic with an unchanged
developer log would mix several design changes into one comparison. Keep an
area-weighted treemap as a later candidate only if it offers a clear benefit.

### Senior engineering perspective: build a truthful presentation layer

Keep the Go engine and its `why` result as the source of execution facts.
Separate those facts from display names, collapsed state, selection, and
layout. The outline and nested-frame renderers should consume the same
presentation model and trigger the existing example and question operations.

Friendly operation names should come from reviewed sample metadata, existing
function descriptions, or a readable function-name fallback. Do not generate
claims about a rule from its source at runtime. In particular, a trace with
children returning 22 and 11 does not, by itself, establish that its parent
added them; other functions can use those values differently.

Folding a pass-through call can reduce noise, but must preserve its meaning.
`NumberWords.parse("eleven")` is a code call that uses a saved vocabulary
answer. If the overview folds that wrapper, label it "Uses saved meaning"
and allow expansion to the original calls. Do not relabel the code node's
direct origin as an example. An exact whole-expression example remains a
single executed node, even if a tutorial can describe a possible calculation.

Associate every displayed run with its project revision. If someone edits
the project, mark the old explanation as belonging to the previous version
until rerun; do not silently point its source locations into changed files.
Question submission must use the selected call's structured identity or its
engine-produced call representation, never reconstruct a call from a shortened
visual label. Reuse existing literal parsing and validation.

For the first prototype, keep code visible through an explicit developer
details action rather than removing it from the product. Do not couple this
experiment to a general application rewrite, graph layout engine, resolver UI,
performance profiler, or incremental evaluator.

## First-session storyboard

| Moment | What the person sees or does | What it must communicate |
|---|---|---|
| Try | Enter `twenty-two plus eleven`; get 33 | This is a working computation, with a concrete input and answer |
| Inspect | Select eleven's saved meaning and twenty-two's combination rule | People supply answers; code composes them |
| Encounter a gap | Enter `twenty-two plus twelve`; see "What number does twelve mean?" | One specific call is missing, not the whole program |
| Answer | Save 12 beside that question; the expression reruns and returns 34 | The saved example is a word meaning and its scope is explicit |
| Reuse | Enter `twelve plus two`; get 14 | The existing code uses the same new row in another expression |
| Check understanding | Ask what happens for an unsaved word, or an exact saved expression | Missing answers become questions; exact examples take precedence |

The final row is a usability-study prompt, not a required quiz in the product.
Disclosure should be reversible: someone can inspect developer details and
return to the same input, selected piece, and result.

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

### 1. Capture the minimum real traces

Start with real `workspace.Run` results for:

- default word addition, showing examples composed by code;
- the missing twelve lookup, before and after answering it;
- reuse in `twelve plus two`;
- a whole-expression example, giving a single-node trace.

Keep these as sample data shared by candidate renderers. They must come from
the engine, not a hand-authored idealized computation. Then check one shipping
trace to catch assumptions specific to numbers. This is enough to begin the
onboarding comparison; do not block the first prototype on every engine state.

Before production, broaden the corpus to invalid `eleven-two`, unconfirmed
code, a resolver proposal, deeper chains, repeated calls, and large object
values. Proposal metadata such as resolver identity is not fully exposed in
the current workspace Step; support can be scoped separately from the first
onboarding experiment.

### 2. Compare an outline with nested frames in the same flow

Use the first-session storyboard, labels, trace corpus, and question editor
for both candidates. Include the current developer text tree as a baseline,
but do not attribute gains from plain-language copy to the rectangle layout.
Review desktop and phone versions, the missing-answer state, and the
single-example case. Choose the layout, size meaning, selection behavior,
and labels before implementing the production renderer.

Run a small formative study with five people unfamiliar with Park. This is a
directional test, not statistical validation. Proposed acceptance targets are:

- at least four can supply twelve's meaning and run a second expression using
  it without opening source or receiving moderator instructions;
- at least four can explain which saved answer changed and what code reused;
- at least four can distinguish an exact saved expression from a computed
  expression after inspecting both;
- record and resolve any observed confusion about automatic rule learning,
  code being verified, or rectangle area measuring the output value.

Record task completion, wrong edits, help requests, and participants' own
explanations. A two-minute path to supplying and reusing the missing answer
is a design target to measure, not a demonstrated result. If a proposed view
requires explaining trees before someone can answer the question, revise it.
If nested frames do not improve comprehension or navigation over the outline,
ship the simpler outline.

### 3. Build one complete interaction with the chosen view

Implement expression entry, result, explanation selection, saving the missing
word meaning, and replay. Keep the developer trace available. Use the existing
engine and mutation operations; extend bridge fields only when the chosen
interaction requires them. A read-only chart alone does not test onboarding.

### 4. Generalize and cover the remaining states

Connect saved-answer selection to exact rows in existing tables, validate
the flow against shipping, and add the error and proposal cases from the
broader corpus. Check status integration, comparison mode, and new layout
options can follow separately. Do not make them prerequisites for a usable
answer-and-reuse flow.

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
