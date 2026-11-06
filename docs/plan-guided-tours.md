# Plan: guided tours in the playground

Status: plan, 2026-11-06. Nothing built yet. Part of the playground's phase 3
([plan-web-playground.md](plan-web-playground.md)).

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
| `{"ran": C, "output": O}` | The last run was call `C` (and returned `O`, if given). |
| `{"status": S}` | The last run's status was `S` (`ok`, `unresolved`, …). |
| `{"question": C}` | `C` is an open question. |
| `{"example": C, "output": O}` | Some function has an example for call `C` (with output `O`, if given). |
| `{"why": C, "source": S}` | The last why tree contains call `C` answered by `S`. |
| `{"testFails": F}` / `{"testPasses": F}` | The last `park test` run failed or passed for `F`. |
| `{"fileContains": F, "text": T}` | File `F` contains `T`. |
| `{"all": [...]}` | Every listed goal is met. |

Calls are compared after normalizing through the engine's call parser, so
`Zones.zone("DE")` and `Zones.zone(country="DE")` match.

### Skip operations

A skip is a list of operations, applied in order. They are the playground's
existing engine operations plus two plain file edits:

- `{"addExample": "P.fn(args) == value"}` and `{"removeExample": "P.fn(args)"}`
- `{"run": "P.fn(args)"}` and `{"test": ""}`
- `{"replace": {"file": F, "find": A, "with": B}}`: exact text, which must
  occur once.
- `{"setFile": {"file": F, "text": T}}` for new files such as
  `country_alias.star`.

Skipping edits the reader's current project instead of replacing it, so
anything else they changed survives. If an operation can't apply (a reader
deleted the line a `replace` looks for), the page falls back to the step's
**checkpoint**: the project as it would be after replaying every skip from the
start. The build computes each checkpoint (below).

### Places to point at

Steps name places, not CSS selectors, so the page layout can change without
editing tours: `call`, `run`, `result`, `why`, `questions`, `functions`,
`function:Zones.zone`, `tests`, `catalog`, `editor`, `share`. `app.js` maps
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
- At the end: "Keep this project" copies the tour's project into the reader's
  own slot, and "Share" works as usual.

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
- The header gets a **Take the tour** button. The hosted page opens the
  shipping tour on a first visit, but not when it's opened from a share link.

## Tours to write

1. **Two people, one function** (shipping, about 8 steps): the table answers →
   the rule uses the table → Germany raises a question → Ops answers → the
   quote resumes → why → the developer's code disagrees with Ops; the example
   wins and the test flags it → fix the code.
2. **Aliases** (shipping, continued): add `CountryAlias`, where code
   normalizes case and spacing and a table maps spellings. "usa" works, and an
   unknown spelling becomes a question for Ops.
3. **For developers: delete your requirements doc.** Turn a ticket's
   acceptance criteria into examples, write the rule, and watch the questions
   show which cases the ticket never decided.
4. **Roman numerals:** the README's walkthrough, for programmers.

## Phases

1. Format, goal checker, and the build-time replay check (Go), plus the
   shipping tour data. Done when `go test` replays the tour.
2. The tour panel in the page, with goals, skip, Back, and Exit, and a
   separate storage slot. Done when the headless browser check plays the
   whole shipping tour, both by doing each step and by skipping each one.
3. Deep links (`#tour=id/step`), first-visit auto-start, and the remaining
   tours.

## Non-goals

Branching tours, quizzes, analytics, and a visual tour editor. Writing a
tour's JSON by hand is fine while there are a handful of them.
