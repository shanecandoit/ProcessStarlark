# First-pass usability sessions

Status: protocol ready; no participant sessions or findings recorded yet.
This is a small formative study, not a quantitative product-performance claim.
Use five people unfamiliar with Park if available. Do not count automated browser
tests as participant evidence. Avoid personal/customer data in the sample rules.

## Preparation

Use the [playground review guide](playground-review-guide.md). Give each session
an independent browser profile and fresh local server database. Prepare the
word-addition example with both twelve and thirteen missing for task 1. Prepare
an active shipping baseline and a saved draft for task 3. Keep the code pane
closed initially. Test keyboard navigation and one phone-width layout as well.

Explain the goal and the meaning of the local actor names without explaining
the implementation. Ask participants to think aloud. Allow a task to proceed
without moderator help first; record any hint needed and when it was given.
Do not treat the two-example readiness policy as a correctness guarantee.

## Tasks and neutral prompts

1. **Supply two missing facts.** “Get an answer for twelve plus thirteen.”
   Observe whether the participant can answer both questions, retain the original
   request, and recognize `25` as the composed result.
2. **Reuse an answer.** “Now work out twelve plus two. What did your earlier
   change make possible?” Look for an explanation that a word meaning was saved
   and reused; no new general rule was learned automatically.
3. **Review a shipping change.** “Make Germany an international destination.
   Explain which saved source changed and what the comparison tells you.”
   Observe whether they distinguish live baseline from draft, examples from
   computed results, check results from comparison cases, and coverage limits.
4. **Activate and recover.** “Make the checked change active, then restore the
   previous behavior.” Use two actor labels locally. Observe confirmation of
   live release identity, required rejection/rollback reasons, and recovery.
5. **Encounter staleness.** Advance source or the active release from a second
   browser/API while the participant has a pending review. “What happened, and
   what would you do next?” Check whether they preserve local work and understand
   why refreshing the generation alone cannot make the old review current.

After each task ask: “What changed?”, “What stayed live?”, and “What does the
report let you conclude?” Do not lead with terminology like hashes or dependency
graphs. End by asking which labels/actions were least clear.

## Record one row per participant/task

| Session | Task | Completed unaided? | Time observed | Wrong edits | Help requested/given | Participant explanation | Confusion / severity | Proposed follow-up |
|---|---|---|---|---|---|---|---|---|
| Pending | — | — | — | — | — | — | — | — |

Use anonymous session IDs. Keep observations separate from interpretations.
Record skipped/abandoned tasks and missing measurements rather than inventing
values. Capture screenshots only with the participant's agreement.

## Review gate

Use the existing directional target: at least four of five people can supply
and reuse a missing answer and explain which saved fact the rule used without
opening code or receiving instructions. For the new release tasks, report the
observed completion/confusion counts before setting a new numerical target.
Resolve misleading interpretations about automatic learning, checked-versus-live
state, and comparison coverage before adding more features. Attach the actual
findings and prioritized fixes here after sessions; until then, usability remains
an open milestone.
