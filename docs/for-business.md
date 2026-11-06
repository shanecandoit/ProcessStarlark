# Park for business teams

This page is for the people who own business rules (operations, finance,
pricing, compliance) and for anyone deciding whether Park fits their team. No
programming background is needed. Programmers should read
[Why Park?](why-park.md).

## The usual way

Someone who knows the rules writes them down: a spec, a ticket, a spreadsheet
sent by email. A developer reads it, interprets it, and writes code. Then:

- **Cases the document missed get decided by the developer,** usually without
  anyone noticing.
- **The document and the system drift apart.** After a few changes, nobody is
  sure which one is right.
- **Mistakes surface late,** in testing or with a customer.
- **Every change to a rule goes back through a developer,** even when it's a
  single number.

## The Park way

The people who know the answers write them down as **examples**: "a 500 g
parcel to the US costs $5.99". The examples aren't a document *about* the
system. They are the system's answers, from the day they're written. Ask about
a case that's listed, and you get that answer.

Developers then write **code** for the general rule. Code answers the cases
nobody listed. It never overrides a case that is listed. If the code gives a
different answer for a listed case, Park reports the disagreement.

When a case comes up that nobody listed and no code covers, **Park doesn't
guess. It asks.** The question goes to the people who own the rule, and their
answer becomes a new example.

## A walkthrough: shipping quotes

This is the *Shipping quotes* sample in the
[browser playground](https://shanecandoit.github.io/ProcessStarlark/). Press **Run** to follow along.

1. **Operations keeps the zone table.** They list which zone each country is
   in: the US is domestic, Canada and Mexico are North America, France is
   international. There's no code here, only their answers.
2. **A developer writes the pricing rule:** a base fee per zone, plus a fee per
   500 g. Operations' three known quotes (US 500 g, Canada 1200 g, France
   2500 g) check that the rule is right.
3. **A customer asks for a quote to Germany.** Germany isn't in the zone table,
   so Park doesn't make up a zone. It asks: *what zone is Germany?*
4. **Operations answers "international".** That answer becomes a new row in
   their table, and the quote comes back: $27.99.
5. **Anyone can ask why.** Park shows that $27.99 came from the developer's
   pricing rule, which used operations' entry for Germany, with a link to each.

Nobody filed a ticket, and nobody guessed. The developer didn't need to know
which zone Germany is in.

## Who does what

| Role | Does | Doesn't need to |
|---|---|---|
| Rule owner (business) | Writes examples, answers questions, confirms proposed answers | Write or read code |
| Developer | Writes code for the general rule, runs the checks | Know every business case up front |
| Everyone | Reads the same catalog of every function and its examples | Keep a separate spec in sync |

## How Park differs from tools you may already use

| Tool | What it does well | Where Park is different |
|---|---|---|
| Specs and tickets | Easy to write | They describe the system. Park's examples *are* the system's answers, so they can't drift. |
| Spreadsheets | Values and formulas side by side | Nothing checks a formula against the values someone typed in. Park checks the code against every example. |
| Rules engines and decision tables (DMN, BRMS) | Business-editable rules | You still have to write the rule for every case, in the tool's own language, and unmatched cases fall through to a default. In Park you write cases, developers write the rule, and unmatched cases become questions. |
| Low-code and no-code builders | Build without developers | They replace developers and stop where the builder's blocks stop. Park keeps developers and changes the handoff. |
| BDD scenarios (Cucumber, Gherkin) | Business-readable examples | Scenarios are tests run against code. They never answer anything in production, and a missing scenario goes unnoticed. In Park, examples answer, and missing cases are asked about. |
| AI code generation | Writes code quickly | Someone still has to say what's right. In Park, that's the examples, and generated code is checked against them like any other code. |

## What this gets you

- **Requirements that can't drift,** because they're the system's answers.
- **No silent guesses.** Gaps become questions addressed to the rule's owner.
- **Every answer is traceable** to the example someone wrote or the code that
  computed it, with a link to each.
- **Business changes without a developer:** changing a listed answer is a
  one-line edit to an example.
- **Predictable behaviour.** Rules can't reach databases, networks, or the
  clock. The same question always gets the same answer, so results are
  reproducible and easy to audit.

## Good fits

Pricing and fees, eligibility, routing and assignment, classification, code
and category mappings, thresholds: rules that are partly a table people
maintain and partly a formula.

It's not a fit for workflows that send emails or update databases, for large
data processing, or for anything that has to be fast at scale.

## Where Park is today

Early. The core works: examples, code, checks, questions, and "why" are all
real, in a command-line tool and a browser playground. Examples are written in
text files, in a format that is readable but still looks like code.

Not built yet, and needed before a business team could use it day to day:

- a table or form editor for examples and questions;
- accounts, a change history, and approvals;
- an API so other systems can ask Park for answers.

If this matches a problem your team has, that's the conversation we want to
have.
