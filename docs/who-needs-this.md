
# Who needs this now

These are audience hypotheses to validate, not evidence of customer demand.
The shipping example is the concrete demo; use interviews to choose a first
market before expanding the scope.

## AI is a possible angle, without leading with synthesis.

- AI writing code: teams generating code with AI need a way to say what's right that a non-programmer can write. Examples are that; generated code is checked against them like any other code.
- AI answering questions is a future integration idea, not a current runtime capability. Park rules cannot make network or model calls. An external application could propose model outputs for people to review; the runtime would continue to use explicit examples and deterministic code. Demand for that workflow still needs validation.

## Who has the problem today:

- Data teams cleaning and mapping data: country aliases, vendor names, product categories, unit conversions. Your CountryAlias example is this whole market. Today it's long CASE statements, plus mapping spreadsheets nobody checks.
- Legacy replacements: record the old system's answers as examples, then build the new code against them. Any difference shows up as a disagreement or a question. Consultancies do this work by hand.
- Pricing, fees and entitlements: shipping and carrier rates, SaaS plan features, commissions, insurance rating tables.
- Regulated decisions that must be explained: lending eligibility, insurance, benefits, healthcare billing code mappings. "Why" traced to a named person's example is an audit feature these buyers pay for.
- Support and operations routing: which team gets which ticket, which warehouse ships which order.

## Features these imply:

- Impact preview: "this code change alters 37 answers people have already seen", which needs a log of past calls. For a business buyer, that may be the single most persuasive feature.
- Importing examples from CSV, spreadsheets and logs. These come in as proposals to confirm, consistent with D18.
- Embedding the engine as a package. The WASM build already runs anywhere JavaScript does, so an npm package is close.
