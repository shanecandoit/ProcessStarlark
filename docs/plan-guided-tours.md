# Plan: guided tours in the playground

Status: phases 1 and 2 built, 2026-11-06, with one tour
([web/tours/shipping.json](../web/tours/shipping.json)). Part of the
playground's phase 3 ([plan-web-playground.md](plan-web-playground.md)).
Try it: <https://shanecandoit.github.io/ProcessStarlark/#tour=shipping>.

## Goal

Someone who has never heard of Park opens a link and, in about two minutes,
reaches the moment where it clicks: a table of examples already answers, a gap
becomes a question instead of a guess, the answer resumes the call, and "why"
traces the result back to the person who wrote each part. No install, no
explanation from us.

The first tour is the shipping story, told with two roles: **Ops**, who owns
the zone table, and the **developer**, who writes the pricing rule. The
handoff between them is the product, so the tour switches between them.

## Tours are data, not scripts

A tour is a JSON file: a starting project and a list of steps. A small
interpreter in `app.js` plays any tour. No step has its own JavaScript.

- Why: a tour's steps can be checked by machine. At build time, Go replays
  every step's "skip" and confirms that its goal is then met, so a change to
  the engine or the samples that breaks a tour fails the build instead of
  breaking the demo.
- Why: people other than the engine's author can write tours, and a tour is a
  reviewable diff.
- Cost: steps can only do what the vocabulary below allows. When a tour
  needs more, extend the vocabulary rather than adding scripts.

### Format

```json
{
  "id": "shipping",
  "title": "Two people, one function",
  "start": "examples/shipping",
  "steps": [
    {
      "role": "ops",
      "text": "You keep the shipping zones. There's no code here, just your answers. Run the call to see the table answer.",
      "call": "Zones.zone(\"FR\")",
      "point": ["run"],
      "goal": { "ran": "Zones.zone(\"FR\")", "output": "\"international\"" }
    },
    {
      "role": "ops",
      "text": "A customer wants a quote to Germany. Germany isn't in your table, so Park asks you instead of guessing. Answer the question.",
      "call": "Shipping.quote(\"DE\", 800)",
      "point": ["run", "questions"],
      "goal": { "example": "Zones.zone(\"DE\")" },
      "skip": [{ "addExample": "Zones.zone(\"DE\") == \"international\"" }]
    },
    {
      "role": "dev",
      "text": "Someone changes the rule so France costs less. Park still answers France with Ops' quote, and the test flags the disagreement.",
      "open": "shipping.star:10",
      "goal": { "testFails": "Shipping.quote" },
      "skip": [{ "replace": { "file": "shipping.star", "find": "\"international\": 1999", "with": "\"international\": 1499" } }]
    }
  ]
}
```

Fields of a step:

| Field | Meaning |
|---|---|
| `role` | Who you are in this step (`ops`, `dev`, or none). Shown as a chip. |
| `text` | What to do and why, in plain language. Short: two sentences. |
| `call` | Prefills the call box. |
| `open` | Opens `file:line` in the editor. |
| `point` | Named places to highlight (see below). |
| `goal` | A condition that completes the step. Without one, the step shows **Next**. |
| `skip` | Operations that complete the step for you. |

### Goals

Each goal is checked against the latest engine result and snapshot, which the
page already has after every operation. No extra engine calls.

| Goal | Met when |
|---|---|
| `{"ran": C, "output": O, "status": S}` | The last run was call `C`, and returned `O` with status `S` (each if given). |
| `{"status": S}` | The last run's status was `S` (`ok`, `unresolved`, …). |
| `{"question": C}` | `C` is an open question. |
| `{"example": C, "output": O}` | Some function has an example for call `C` (with output `O`, if given). |
| `{"function": F}` | `Process.function` `F` is declared. |
| `{"not": G}` | Goal `G` does not hold. |
| `{"all": [...]}` | Every listed goal holds. |

Goals are checked by one Go function, `workspace.Met`. The build calls it
directly, and the page calls it through park.wasm after every engine
operation. Goals on `park test` results, the why tree, or file text can be
added when a tour needs them.

Calls are compared after normalizing through the engine's call parser, so
`Zones.zone("DE")` and `Zones.zone(country="DE")` match.

### Skip operations

A skip is a list of operations, applied in order. They are the playground's
existing engine operations plus two plain file edits:

- `{"run": "P.fn(args)"}`
- `{"addExample": "P.fn(args) == value"}`
- `{"paste": {"function": F, "text": T}}`: pastes spreadsheet rows into
  `F`'s table, as a reader would (D21).
- `{"dismiss": "P.fn(args)"}`: removes an open question without answering it.
  The playground's questions also got a **Dismiss** button for this.
- `{"replace": {"file": F, "find": A, "with": B}}`: exact text, which must
  occur once.
- `{"setFile": {"file": F, "text": T}}` for new files such as
  `country_alias.star`.

Skipping edits the reader's current project instead of replacing it, so
anything else they changed survives. If an operation can't apply (a reader
deleted the line a `replace` looks for), the page falls back to the step's
**checkpoint**: the project as it would be after replaying every skip from the
start. The build computes each checkpoint (below). The same fallback applies
when the operations succeed but the goal still doesn't hold.

### Places to point at

Steps name places, not CSS selectors, so the page layout can change without
editing tours: `call`, `run`, `result`, `why`, `questions`, `functions`, `editor`,
`addfile`, `share`, and `function:Process.fn` (that function's table). `app.js` maps
each name to an element and draws an outline around it.

## Links and saved work

- `#tour=shipping` starts a tour. `#tour=shipping/4` starts at step 4 with
  that step's checkpoint loaded, which is useful for presentations and for
  linking from docs.
- Tours ship with the page (`dist/tours.js`, like `samples.js`), so tour
  links stay short. Share links (`#p=…`) are unchanged.
- **A tour never overwrites the reader's own project.** A tour runs in its own
  slot in `localStorage` (`park-tour-v1:<id>`), and leaving the tour restores
  the project the reader had before. Progress is saved, so a reload continues
  the tour at the same step.
- **Finish** keeps the tour's project as the reader's own. **Exit tour**
  returns to the project they had before. Picking a sample also ends the
  tour.

## Built and checked in Go

`cmd/playground-build` reads `web/tours/*.json`, and for each tour:

1. loads `start` as files;
2. for each step, applies its `skip` with the `internal/workspace`
   operations, the same code the page runs, and checks that the step's `goal`
   is met afterwards;
3. records the resulting files as the step's checkpoint;
4. writes every tour, with checkpoints, to `dist/tours.js`.

A failing tour stops the build with the step number and the unmet goal. A Go
test runs the same check, so `go test ./...` catches broken tours too. The
goal checker is written once in Go and once in JavaScript; a shared table of
goal cases tests both.

## Page

- A tour panel takes the top of the right-hand pane. It shows the role chip,
  the step text, the step number, **Back**, **Skip** (if there's a skip),
  **Next** (if there's no goal), and **Exit tour**. On phones it's a bar
  pinned to the bottom of the screen.
- When a goal is met, the panel marks the step done and moves on after a
  short pause, so the reader sees the result first.
- The header has a **Tour** button. A first visit (nothing saved, no link)
  opens the first tour. A share link always wins over the tour.

## Tours to write

1. **Two people, one function** (shipping, 11 steps; built):
   1. A quote to Germany raises a question.
   2. Ops answers it, and the quote resumes at 2799.
   3. "Why this answer" shows where 2799 came from.
   4. The developer declares an empty `RegionAlias.known` table, and Ops
      fills it by pasting rows from a spreadsheet, without seeing source.
   5. "USA" isn't in the table, so it becomes a question. Ops dismisses it,
      because capitalization is a rule, not a fact.
   6. The developer writes `RegionAlias.code`, which lowercases and trims,
      then looks the name up in Ops' table.
   7. The developer wires it into the quote. `quote("Germany", 800)` now
      gives 2799, and its why tree shows both people's work.
2. **Examples win** (shipping, continued): the developer's code disagrees
   with Ops' quote for France. The example still answers, and `park test`
   flags the disagreement. Needs a goal on test results.
3. **For developers: delete your requirements doc.** Turn a ticket's
   acceptance criteria into examples, write the rule, and watch the questions
   show which cases the ticket never decided.
4. **Roman numerals:** the README's walkthrough, for programmers.

## Phases

1. **Done.** Format, goal checker, and the build-time replay check
   (`workspace.Prepare`, `TestToursReplay`), plus the shipping tour.
2. **Done.** The tour panel, with goals, skip, Back, Exit, and Finish; a
   separate storage slot; deep links (`#tour=id/step`, also when the page is
   already open); and first-visit auto-start. A headless browser check
   plays the whole tour, some steps by hand and some skipped, plus reload,
   deep link, exit, the checkpoint fallback, and phone width.
3. The remaining tours.

## Non-goals

Branching tours, quizzes, analytics, and a visual tour editor. Writing a
tour's JSON by hand is fine while there are a handful of them.
