# Park for business teams

Park is an early tool for rules that combine a table people maintain with code
that calculates an answer. This page is for rule owners and teams considering
a trial. Developers can start with [Why Park?](why-park.md).

## How it works

A rule owner writes known answers as **examples**: “a 500 g parcel to the US
costs $5.99.” A matching request returns that saved answer, even before there
is code.

A developer writes the general rule for cases outside the examples. Explicit
implementation checks compare the code with the saved answers and report
disagreements. A normal run returns a matching example directly; it does not
check the implementation.

If neither an example nor code can answer a call, Park records a question.
A person can supply the missing fact as another example and rerun the request.
Errors in code are reported separately.

## A walkthrough: shipping quotes

Try the [shipping tour](https://shanecandoit.github.io/ProcessStarlark/#tour=shipping).

1. Operations maintains a country-to-zone table: the US is domestic, Canada
   and Mexico are North America, and France is international.
2. A developer writes a pricing rule: a base fee per zone, plus a fee for each
   started 500 g. Saved quotes provide cases for checking the implementation.
3. A request for an 800 g parcel to Germany needs a zone that is missing.
   Park asks which zone Germany belongs to.
4. Operations saves “international.” The rule can now calculate $27.99.
   Other weights can reuse the same zone entry.
5. The explanation shows the pricing rule and the Germany entry it used.

## Who does what

| Role | Work |
|---|---|
| Rule owner | Maintains examples, answers questions, and confirms proposed answers |
| Developer | Implements general rules and runs implementation checks |
| Reviewer | Inspects saved changes, checks, and scenario comparisons before activation |

Examples can be wrong or incomplete. Checks show whether code agrees with the
saved cases; they do not establish that every possible input is correct.
Changing an answer in a draft does not activate a live release.

## Scope

Pricing tables, category mappings, and routing rules are candidate uses to
try with a team. The shipping sample uses integer cents for money. Rules have
no database, network, or clock access; applications supply their inputs and
handle any external actions. Performance at scale has not been measured.

## Where Park is today

The CLI and browser playground support examples, code, checks, questions,
explanations, editable tables, and CSV/TSV import.

The local backend adds SQLite persistence, isolated drafts, source history,
question tracking, scenario comparisons, reviews, release activation, an
evaluation API, and rollback. See the [review guide](playground-review-guide.md).

Accounts, project permissions, authenticated reviewer roles, and hosted
operations remain planned. Local actor names are labels. The
[roadmap](roadmap.md) starts with testing the workflow with unfamiliar users
and a prospective team.
