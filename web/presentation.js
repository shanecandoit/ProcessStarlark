// Display conventions only. Execution and literal validation stay in Go.
(function (root) {
  "use strict";
  const functions = {
    "WordAdd.eval": { title: "Add number words", columns: { text: "Expression", total: "Answer" } },
    "NumberWords.parse": { title: "Read a number", columns: { text: "Number words", value: "Number" } },
    "WordValues.value": { title: "Word meanings", columns: { word: "Word", result: "Meaning" } },
    "Shipping.quote": { title: "Shipping quote" },
    "Zones.zone": { title: "Shipping zones" },
    "Directions.canonical": { title: "Direction words" },
    "Robot.walk": { title: "Move the robot" },
    "Robot.step": { title: "One robot step" },
  };
  const origins = { example: "Saved answer", code: "Rule", "unconfirmed code": "Unconfirmed rule", proposed: "Suggested answer", unknown: "Needs an answer" };
  function readable(literal) {
    if (typeof literal !== "string") return String(literal ?? "");
    try { const value = JSON.parse(literal); if (typeof value === "string") return value; } catch { /* other Starlark literals remain literal */ }
    return literal;
  }
  function expressionFromCall(call) {
    const m = /^\s*WordAdd\.eval\(\s*(?:text\s*=\s*)?("(?:[^"\\]|\\.)*")\s*\)\s*$/.exec(call);
    if (!m) return null;
    try { return JSON.parse(m[1]); } catch { return null; }
  }
  function projectKey(files) {
    return JSON.stringify(Object.keys(files).filter(n => n.endsWith(".star")).sort().map(n => [n, files[n]]));
  }
  function hasUnknown(step) {
    return step.source === "unknown" || (step.steps || []).some(hasUnknown);
  }
  function outcome(step) {
    if (!step.failed) return "";
    if (step.source === "unknown") return "";
    if (hasUnknown(step)) return "Waiting for an answer";
    return step.error ? "Error" : "Stopped by a smaller step";
  }
  function identity(step, f) {
    if (!f || step.function !== f.qualified || !step.input || f.inputs.some(n => !(n in step.input))) return null;
    const inputs = f.inputs.map(n => step.input[n]);
    const ex = f.examples.find(ex => ex.inputs.length === inputs.length && ex.inputs.every((v, i) => v === inputs[i]));
    return ex ? { example: ex, call: `${f.qualified}(${ex.inputs.join(", ")})` } : null;
  }
  function title(fn) { return functions[fn]?.title || fn; }
  function column(fn, name) { return functions[fn]?.columns?.[name] || name.replace(/_/g, " "); }
  function inputs(step) { return (step.inputs || []).map(n => readable(step.input?.[n] ?? "?")).join(" · "); }
  function textOutput(f) {
    return Boolean(f?.examples.length) && f.examples.every(ex => {
      try { return typeof JSON.parse(ex.output) === "string"; } catch { return false; }
    });
  }
  function editableRow(f, values) {
    return values.map((literal, i) => {
      const column = f.examples.map(ex => i < f.inputs.length ? ex.inputs[i] : ex.output);
      try {
        const text = JSON.parse(literal);
        if (typeof text === "string" && text && text.trim() === text && !/^["']/.test(text) && column.length && column.every(value => typeof JSON.parse(value) === "string")) return text;
      } catch { /* preserve exact literals in mixed or structured columns */ }
      return literal;
    });
  }
  const api = { title, column, readable, expressionFromCall, projectKey, outcome, identity, inputs, origins, textOutput, editableRow };
  root.ParkView = api;
  if (typeof module !== "undefined" && module.exports) module.exports = api;
})(globalThis);
