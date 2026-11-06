// Park playground. JavaScript owns the project's files; every operation hands
// them to the Go engine (park.wasm) and takes back the new files and a
// snapshot of the project. See docs/plan-web-playground.md.
"use strict";

const QUESTIONS = "questions.txt";
const $ = (id) => document.getElementById(id);

const state = {
  files: {},          // name -> source, including questions.txt when non-empty
  active: "",         // file shown in the editor
  snapshot: null,     // last valid-or-not project description from Go
  lastCall: "",       // rerun after an answer, so the call resumes
  fresh: new Set(),   // example keys added by the last edit, highlighted
  view: "functions",
};

function el(tag, props = {}, ...children) {
  const node = document.createElement(tag);
  for (const [k, v] of Object.entries(props)) {
    if (k === "class") node.className = v;
    else if (k.startsWith("on")) node.addEventListener(k.slice(2), v);
    else if (v !== undefined && v !== null && v !== false) node.setAttribute(k, v === true ? "" : v);
  }
  for (const c of children.flat()) {
    if (c === null || c === undefined || c === false) continue;
    node.append(c instanceof Node ? c : document.createTextNode(String(c)));
  }
  return node;
}

// call runs one Go operation and applies its result.
function call(op, ...args) {
  const result = JSON.parse(globalThis.park[op](JSON.stringify(state.files), ...args));
  const before = exampleKeys(state.snapshot);
  state.files = result.files || state.files;
  state.snapshot = result.snapshot;
  if (op === "addExample") {
    const after = exampleKeys(state.snapshot);
    state.fresh = new Set([...after].filter((k) => !before.has(k)));
  } else if (op === "removeExample") {
    state.fresh = new Set();
  }
  render();
  return result;
}

function exampleKeys(snap) {
  const keys = new Set();
  for (const p of snap?.processes || []) {
    for (const f of p.functions) for (const ex of f.examples) keys.add(f.qualified + "(" + ex.inputs.join(",") + ")");
  }
  return keys;
}

// ---- Rendering ------------------------------------------------------------

function render() {
  renderTabs();
  renderEditor();
  renderProblem();
  renderQuestions();
  renderFunctions();
  renderCatalog();
}

function modules() {
  return Object.keys(state.files).filter((n) => n.endsWith(".star")).sort();
}

function renderTabs() {
  const tabs = $("tabs");
  tabs.replaceChildren(
    ...modules().map((name) =>
      el("button", { type: "button", class: "tab", role: "tab", "aria-selected": String(name === state.active), onclick: () => openFile(name) }, name)),
    el("button", { type: "button", class: "tab add", title: "Add a file", onclick: addFile }, "+ file"),
  );
}

function renderEditor() {
  const editor = $("editor");
  const text = state.files[state.active] ?? "";
  if (editor.value !== text) {
    const { selectionStart, selectionEnd } = editor;
    editor.value = text;
    editor.setSelectionRange(selectionStart, selectionEnd);
  }
}

function renderProblem() {
  const err = state.snapshot && !state.snapshot.valid ? state.snapshot.error : "";
  $("problem").hidden = !err;
  $("problem").textContent = err;
}

function renderQuestions() {
  const qs = state.snapshot?.questions || [];
  $("qcount").textContent = qs.length ? `(${qs.length})` : "";
  $("qhint").hidden = qs.length > 0;
  $("questions").replaceChildren(...(qs.length ? qs.map(question) : [el("li", { class: "empty-row" }, el("span", { class: "empty" }, "No open questions."))]));
}

function question(q) {
  const value = el("input", { "aria-label": `Answer for ${q.call}`, placeholder: "answer, e.g. 9 or \"text\"", spellcheck: "false", value: q.hasProposal ? q.proposed : "" });
  const submit = () => {
    if (!value.value.trim()) { value.focus(); return; }
    answer(`${q.call} == ${value.value.trim()}`);
  };
  value.addEventListener("keydown", (e) => { if (e.key === "Enter") submit(); });
  return el("li", { class: q.hasProposal ? "proposal" : "" },
    el("code", {}, q.call), el("span", { class: "eq" }, "=="), value,
    el("button", { type: "button", onclick: submit }, q.hasProposal ? "Confirm" : "Answer"),
    q.hasProposal ? el("span", { class: "hint" }, "proposed by unconfirmed code; edit it if it's wrong") : null,
  );
}

function renderFunctions() {
  const box = $("view-functions");
  const procs = state.snapshot?.processes || [];
  if (!procs.length) {
    box.replaceChildren(el("p", { class: "empty" }, state.snapshot?.valid ? "No functions yet." : "Fix the error to see functions."));
    return;
  }
  box.replaceChildren(...procs.flatMap((p) => p.functions.map((f) => functionCard(f))));
}

function functionCard(f) {
  const stateClass = f.state === "needs examples" ? "state needs" : "state";
  const head = el("tr", {}, ...f.inputs.map((n) => el("th", {}, n)), el("th", { class: "out" }, f.output), el("th"));
  const rows = f.examples.map((ex) => {
    const callText = `${f.qualified}(${ex.inputs.join(", ")})`;
    const key = f.qualified + "(" + ex.inputs.join(",") + ")";
    return el("tr", { class: state.fresh.has(key) ? "fresh" : "" },
      ...ex.inputs.map((v) => el("td", {}, v)),
      el("td", { class: "out" }, ex.output),
      el("td", {}, el("button", { type: "button", class: "x", title: `Remove example ${callText}`, "aria-label": `Remove example ${callText}`, onclick: () => call("removeExample", callText) }, "×")));
  });
  return el("article", { class: "fn" },
    el("header", {},
      el("code", {}, f.qualified.split(".")[0] + "." + f.signature),
      el("span", { class: stateClass }, f.state),
      el("a", { href: "#", class: "state", onclick: (e) => { e.preventDefault(); openFile(f.path, f.line); } }, `${f.path}:${f.line}`)),
    f.description ? el("p", { class: "desc" }, f.description) : null,
    rows.length ? el("div", { class: "table-wrap" }, el("table", {}, el("thead", {}, head), el("tbody", {}, rows))) : el("p", { class: "empty" }, "No examples yet."));
}

let shownCatalog = "";
function renderCatalog() {
  const html = state.snapshot?.catalog || "";
  if (html && html !== shownCatalog && state.view === "catalog") {
    $("catalog").srcdoc = html;
    shownCatalog = html;
  }
}

function showResult(r, note) {
  $("result").hidden = false;
  const status = $("status");
  status.textContent = r.status;
  status.className = "badge s-" + r.status;
  $("output").textContent = r.output || "";
  $("message").textContent = [note, r.message].filter(Boolean).join(" ");
}

// ---- Actions --------------------------------------------------------------

function run(text) {
  text = text.trim();
  if (!text) return;
  state.lastCall = text;
  showResult(call("run", text));
}

function answer(line) {
  const r = call("addExample", line);
  if (r.status !== "ok") { showResult(r); return; }
  // Resume: rerun the call that raised the question, now that it's answered.
  if (state.lastCall) showResult(call("run", state.lastCall), `Answered ${line.split(" == ")[0]}, then reran ${state.lastCall}.`);
  else showResult(r);
}

function openFile(name, line) {
  if (!(name in state.files)) return;
  state.active = name;
  render();
  if (line) {
    const editor = $("editor");
    const lines = editor.value.split("\n");
    const start = lines.slice(0, line - 1).join("\n").length + (line > 1 ? 1 : 0);
    editor.focus();
    editor.setSelectionRange(start, start + (lines[line - 1] || "").length);
    editor.scrollTop = Math.max(0, (line - 4) * parseFloat(getComputedStyle(editor).lineHeight));
  }
}

function addFile() {
  let name = prompt("New file name (e.g. words.star):", "");
  if (!name) return;
  name = name.trim().toLowerCase().replace(/\s+/g, "_");
  if (!name.endsWith(".star")) name += ".star";
  if (!/^[a-z][a-z0-9_]*\.star$/.test(name) || name in state.files) { alert("Use a new name made of letters, digits, and underscores."); return; }
  const proc = name.slice(0, -5).split("_").filter(Boolean).map((s) => s[0].toUpperCase() + s.slice(1)).join("");
  state.files[name] = `Process ${proc}:\n    """Describe this process."""\n\n    def answer(question):\n        pass\n`;
  state.active = name;
  call("load");
}

let loadTimer = 0;
function edited() {
  state.files[state.active] = $("editor").value;
  clearTimeout(loadTimer);
  loadTimer = setTimeout(() => call("load"), 300);
}

function loadSample(i) {
  const s = globalThis.PARK_SAMPLES[i];
  state.files = { ...s.files };
  state.active = Object.keys(s.files).sort()[0];
  state.lastCall = "";
  state.fresh = new Set();
  $("call").value = s.call;
  $("result").hidden = true;
  $("testout").replaceChildren();
  call("load");
}

function selectView(view) {
  state.view = view;
  for (const tab of document.querySelectorAll(".tab[data-view]")) tab.setAttribute("aria-selected", String(tab.dataset.view === view));
  for (const v of ["functions", "catalog", "tests"]) $("view-" + v).hidden = v !== view;
  renderCatalog();
}

function runTests() {
  const r = call("test");
  const out = $("testout");
  out.replaceChildren(...(r.lines || []).map((l) => el("div", { class: l.failed ? "fail" : "" }, l.text)), el("div", {}, r.message || ""));
}

// ---- Startup --------------------------------------------------------------

function start() {
  const picker = $("sample");
  globalThis.PARK_SAMPLES.forEach((s, i) => picker.append(el("option", { value: i }, s.name)));
  picker.addEventListener("change", () => loadSample(Number(picker.value)));
  $("reset").addEventListener("click", () => loadSample(Number(picker.value)));
  $("runform").addEventListener("submit", (e) => { e.preventDefault(); run($("call").value); });
  $("editor").addEventListener("input", edited);
  $("editor").addEventListener("keydown", (e) => {
    if (e.key === "Tab" && !e.shiftKey) {
      e.preventDefault();
      document.execCommand("insertText", false, "    ");
    }
  });
  $("test").addEventListener("click", runTests);
  for (const tab of document.querySelectorAll(".tab[data-view]")) tab.addEventListener("click", () => selectView(tab.dataset.view));
  loadSample(0);
  $("loading").hidden = true;
}

globalThis.onParkReady = start;

(async () => {
  const go = new Go();
  try {
    let result;
    try {
      result = await WebAssembly.instantiateStreaming(fetch("park.wasm"), go.importObject);
    } catch {
      // Some static servers send the wrong MIME type for .wasm.
      const bytes = await (await fetch("park.wasm")).arrayBuffer();
      result = await WebAssembly.instantiate(bytes, go.importObject);
    }
    go.run(result.instance);
  } catch (err) {
    $("loading").textContent = "Couldn't load park.wasm: " + err;
  }
})();
