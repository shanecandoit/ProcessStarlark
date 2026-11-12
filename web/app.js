// Park playground. JavaScript owns the project's files; every operation hands
// them to the Go engine (park.wasm, in worker.js) and takes back the new files
// and a snapshot of the project. See docs/plan-web-playground.md.
"use strict";

const QUESTIONS = "questions.txt";
const STORAGE_KEY = "park-playground-v1";
const SHARE_PREFIX = "#p=";
const TOUR_KEY = "park-tour-v1";   // a tour in progress, kept apart from the reader's own project
const TOUR_PREFIX = "#tour=";      // #tour=shipping or #tour=shipping/4 (1-based step)
const TOUR_SAMPLE = -2;            // state.sample while touring
const $ = (id) => document.getElementById(id);
const view = globalThis.ParkView;
let serverPanel = null;
const UI_KEY = "park-playground-ui-v1";
const preferences = (() => {
  try { return JSON.parse(localStorage.getItem(UI_KEY) || "null") || {}; } catch { return {}; }
})();
const savePreferences = () => { try { localStorage.setItem(UI_KEY, JSON.stringify(preferences)); } catch { /* optional */ } };

const state = {
  files: {},          // name -> source, including questions.txt when non-empty
  active: "",         // file shown in the editor
  snapshot: null,     // last project description from Go
  lastCall: "",       // rerun after an answer, so the call resumes
  fresh: new Set(),   // example keys added by the last edit, highlighted
  view: "functions",
  sample: 0,          // sample the project started from; -1 for a shared link
  tour: null,         // {id, step, last: {call, status, output}} while a tour runs
  editing: null,      // {fn, call}: the table row being edited
  drafts: {},         // typed, unsaved table cells, by function and row
  note: null,         // {fn, kind, text, lines}: what the last table edit did
  importAs: {},       // fn -> "examples" | "propose": where pasted or imported rows go
  friendlyInput: false,
  result: null,
  resultKey: "",
  testKey: "",
  projectID: 0,
  questionDrafts: {},
};

const MAX_IMPORT_BYTES = 2 << 20; // project.MaxFileBytes

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

// ---- Engine: park.wasm in a Web Worker ------------------------------------

const engine = {
  worker: null,
  next: 0,
  waiting: new Map(),
  ready: null,

  start() {
    this.worker = new Worker("worker.js");
    this.ready = new Promise((resolve, reject) => {
      this.worker.onmessage = (e) => {
        const msg = e.data;
        if (msg.ready) resolve();
        else if (msg.fatal) reject(new Error(msg.fatal));
        else {
          const done = this.waiting.get(msg.id);
          this.waiting.delete(msg.id);
          done?.resolve(JSON.parse(msg.result));
        }
      };
      this.worker.onerror = (e) => reject(new Error(e.message || "worker failed"));
    });
    return this.ready;
  },

  call(op, files, args) {
    return this.ready.then(() => new Promise((resolve, reject) => {
      const id = ++this.next;
      this.waiting.set(id, { resolve, reject });
      this.worker.postMessage({ id, op, files, args });
    }));
  },

  // stop abandons a call that is taking too long and starts a fresh engine.
  stop() {
    this.worker.terminate();
    for (const w of this.waiting.values()) w.reject(new Error("stopped"));
    this.waiting.clear();
    return this.start();
  },
};

// Operations run one at a time, each on the files as they are when it starts.
let queue = Promise.resolve();
function call(op, ...args) {
  const projectID = state.projectID;
  const job = queue.then(() => projectID === state.projectID ? execute(op, args, projectID) : { obsolete: true });
  queue = job.catch(() => {});
  return job;
}

let busyTimer = 0;
async function execute(op, args, projectID) {
  clearTimeout(busyTimer);
  busyTimer = setTimeout(() => { document.body.classList.add("busy"); $("stop").hidden = false; }, 800);
  const before = exampleKeys(state.snapshot);
  try {
    const sent = { ...state.files };
    const result = await engine.call(op, sent, args);
    if (projectID !== state.projectID) return { obsolete: true };
    result.projectKey = view.projectKey(sent);
    if (op === "run") result.executedCall = args[0];
    if (result.files) {
      // Keep what the user typed while this call ran: a local edit wins.
      for (const [name, text] of Object.entries(state.files)) if (text !== sent[name]) result.files[name] = text;
      state.files = result.files;
    }
    if (result.snapshot) state.snapshot = result.snapshot;
    if (op === "run" && state.tour) state.tour.last = { call: args[0], status: result.status, output: result.output || "" };
    if (op === "addExample" || op === "putExample" || op === "pasteExamples" || op === "importRows") {
      const after = exampleKeys(state.snapshot);
      state.fresh = new Set([...after].filter((k) => !before.has(k)));
    } else if (op === "removeExample") {
      state.fresh = new Set();
    }
    render();
    save();
    if (state.tour && op !== "archive") setTimeout(tourCheck);
    return result;
  } catch (err) {
    return { status: "error", message: err.message === "stopped" ? "Stopped. The call took too long, so Park restarted." : String(err) };
  } finally {
    clearTimeout(busyTimer);
    document.body.classList.remove("busy");
    $("stop").hidden = true;
  }
}

function exampleKeys(snap) {
  const keys = new Set();
  for (const p of snap?.processes || []) {
    for (const f of p.functions) for (const ex of f.examples) keys.add(rowKey(f.qualified, ex));
  }
  return keys;
}

// ---- Editor: CodeMirror when it loaded, else the plain textarea -----------

const editor = (() => {
  const textarea = $("editor");
  let onEdit = () => {};
  let marked = null;
  if (globalThis.CodeMirror) {
    const cm = CodeMirror.fromTextArea(textarea, {
      mode: "python", lineNumbers: true, indentUnit: 4, tabSize: 4, indentWithTabs: false,
      extraKeys: { Tab: (c) => c.replaceSelection("    ") },
    });
    const docs = new Map(); // one document per file, so each keeps its undo history
    let current = "";
    cm.on("change", (_, change) => { if (change.origin !== "setValue") onEdit(); });
    const doc = (name, text) => {
      if (!docs.has(name)) docs.set(name, CodeMirror.Doc(text, "python"));
      return docs.get(name);
    };
    return {
      onEdit(fn) { onEdit = fn; },
      get: () => cm.getValue(),
      show(name, text) {
        const d = doc(name, text);
        if (current !== name) { cm.swapDoc(d); current = name; }
        if (d.getValue() !== text) {
          const cursor = d.getCursor();
          d.setValue(text);
          d.setCursor(cursor);
        }
      },
      forget(keep) { for (const name of docs.keys()) if (!keep.includes(name)) docs.delete(name); if (!keep.includes(current)) current = ""; },
      reset() { docs.clear(); current = ""; },
      refresh() { cm.refresh(); },
      goto(line) {
        cm.focus();
        cm.setSelection({ line: line - 1, ch: 0 }, { line: line - 1, ch: cm.getLine(line - 1)?.length ?? 0 });
        cm.scrollIntoView({ line: line - 1, ch: 0 }, 80);
      },
      mark(name, line) {
        if (marked) marked.doc.removeLineClass(marked.handle, "background", "cm-error-line");
        marked = null;
        if (name && docs.has(name) && line) {
          const d = docs.get(name);
          const handle = d.addLineClass(Math.min(line, d.lineCount()) - 1, "background", "cm-error-line");
          marked = { doc: d, handle };
        }
      },
    };
  }
  textarea.addEventListener("input", () => onEdit());
  textarea.addEventListener("keydown", (e) => {
    if (e.key === "Tab" && !e.shiftKey) { e.preventDefault(); document.execCommand("insertText", false, "    "); }
  });
  return {
    onEdit(fn) { onEdit = fn; },
    get: () => textarea.value,
    show(_name, text) {
      if (textarea.value !== text) {
        const { selectionStart, selectionEnd } = textarea;
        textarea.value = text;
        textarea.setSelectionRange(selectionStart, selectionEnd);
      }
    },
    forget() {},
    reset() {},
    goto(line) {
      const lines = textarea.value.split("\n");
      const start = lines.slice(0, line - 1).join("\n").length + (line > 1 ? 1 : 0);
      textarea.focus();
      textarea.setSelectionRange(start, start + (lines[line - 1] || "").length);
      textarea.scrollTop = Math.max(0, (line - 4) * parseFloat(getComputedStyle(textarea).lineHeight));
    },
    mark() {},
  };
})();

// errorLocation finds "file.star:line" in an error message.
function errorLocation(text) {
  const m = /([A-Za-z0-9_]+\.star):(\d+)/.exec(text || "");
  return m ? { file: m[1], line: Number(m[2]) } : null;
}

// ---- Rendering ------------------------------------------------------------

function render() {
  serverPanel?.render();
  renderTabs();
  renderEditor();
  renderProblem();
  renderQuestions();
  renderFunctions();
  renderCatalog();
  renderSample();
  renderInput();
  renderResult();
  $("test-stale").hidden = !state.testKey || state.testKey === view.projectKey(state.files);
  renderPoints();
}

function modules() {
  return Object.keys(state.files).filter((n) => n.endsWith(".star")).sort();
}

function renderTabs() {
  $("tabs").replaceChildren(
    ...modules().map((name) =>
      el("button", { type: "button", class: "tab", role: "tab", "aria-selected": String(name === state.active), onclick: () => openFile(name) }, name)),
    el("button", { type: "button", class: "tab add", title: "Add a file", onclick: addFile }, "+ file"),
  );
}

function renderEditor() {
  if (!(state.active in state.files)) state.active = modules()[0] || "";
  editor.forget(modules());
  editor.show(state.active, state.files[state.active] ?? "");
}

function renderProblem() {
  const err = state.snapshot && !state.snapshot.valid ? state.snapshot.error : "";
  const where = errorLocation(err);
  const problem = $("problem");
  problem.hidden = !err;
  problem.textContent = err;
  problem.classList.toggle("link", Boolean(where));
  problem.onclick = where ? () => openFile(where.file, where.line) : null;
  editor.mark(where?.file, where?.line);
}

let shownQuestions = "";
function renderQuestions() {
  const qs = state.snapshot?.questions || [];
  const section = $("questions-section");
  const destination = state.result && ["unresolved", "unconfirmed"].includes(state.result.status) ? $("question-slot") : $("questions-home");
  if (section.parentElement !== destination) destination.append(section);
  section.hidden = qs.length === 0;
  $("qcount").textContent = qs.length ? `(${qs.length})` : "";
  $("qhint").textContent = state.result?.status === "ok" ? "Park has the answers it needs for this run." : "No open questions. Try an input to see what Park knows.";
  $("qhint").hidden = qs.length > 0;
  const key = state.projectID + JSON.stringify(qs);
  if (key !== shownQuestions) { shownQuestions = key; $("questions").replaceChildren(...qs.map(question)); }
}

function question(q) {
  const word = q.function === "WordValues.value" ? view.readable(q.input?.word) : null;
  const textAnswer = view.textOutput(functionFor(q.function));
  const prompt = word !== null ? `What number does “${word}” mean?` : q.function === "Zones.zone" ? `Which shipping zone is “${view.readable(q.input?.country)}” in?` : `${q.hasProposal ? "Confirm an answer for" : "What is the answer for"} ${view.title(q.function)}?`;
  const value = el("input", { "aria-label": prompt, placeholder: word !== null ? "Number" : textAnswer ? "Text answer" : 'Answer, e.g. 9 or "text"', spellcheck: "false", value: state.questionDrafts[q.call] ?? (q.hasProposal ? textAnswer ? view.readable(q.proposed) : q.proposed : ""), inputmode: word !== null ? "numeric" : "text" });
  value.addEventListener("input", () => { state.questionDrafts[q.call] = value.value; serverPanel?.render(); });
  const error = el("p", { class: "question-error", role: "alert" });
  const submit = async () => {
    if (!value.value.trim()) { value.focus(); return; }
    if (word !== null && (!/^\d+$/.test(value.value.trim()) || Number(value.value) > 99)) {
      error.textContent = "Enter a whole number from 0 to 99."; value.focus(); return;
    }
    button.disabled = true;
    let literal = value.value.trim();
    if (textAnswer) {
      try { if (typeof JSON.parse(literal) !== "string") literal = JSON.stringify(value.value); }
      catch { literal = JSON.stringify(value.value); }
    }
    const r = await answer(`${q.call} == ${literal}`);
    button.disabled = false;
    if (r && r.status !== "ok") { error.textContent = r.message; value.focus(); }
    else { delete state.questionDrafts[q.call]; serverPanel?.render(); $("output").focus(); }
  };
  const button = el("button", { type: "button", onclick: submit }, q.hasProposal ? "Confirm example" : "Save example");
  value.addEventListener("keydown", e => { if (e.key === "Enter") { e.preventDefault(); submit(); } });
  const scope = word !== null ? "Save one word meaning. Other expressions can use it too." : "Save the answer for this exact input. Other rules can use this example." + (textAnswer ? " Text answers don't need quotes." : "");
  return el("li", { class: q.hasProposal ? "proposal" : "", "data-question": q.call },
    el("div", { class: "question-heading" }, el("h3", {}, prompt), el("p", { class: "hint" }, scope)),
    word === null ? el("p", { class: "question-input" }, (q.inputs || []).map(n => `${n}: ${view.readable(q.input?.[n])}`).join(" · ")) : null,
    el("div", { class: "question-actions" }, value, button,
      el("button", { type: "button", class: "quiet", onclick: () => dismiss(q.call) }, "Dismiss")),
    q.hasProposal ? el("p", { class: "hint" }, "This is a suggested answer. Correct it before confirming if needed.") : null,
    error,
    el("details", { class: "question-call" }, el("summary", {}, "Call details"), el("code", {}, q.call)),
  );
}

function renderFunctions() {
  const box = $("view-functions");
  const expanded = new Set([...box.querySelectorAll("article.fn")].filter(node => node.querySelector(".function-details")?.open).map(node => node.dataset.fn));
  const procs = state.snapshot?.processes || [];
  if (!procs.length) {
    box.replaceChildren(el("p", { class: "empty" }, state.snapshot?.valid ? "No functions yet." : "Fix the error to see functions."));
    return;
  }
  const functions = procs.flatMap(p => p.functions).sort((a, b) => Number(b.state === "example-only") - Number(a.state === "example-only"));
  box.replaceChildren(...functions.map(f => functionCard(f, expanded.has(f.qualified))));
}

// Each function is a table: inputs, then the output. Rows can be edited like
// a spreadsheet; every change is an ordinary example line in the module.
function functionCard(f, expanded = false) {
  const stateClass = f.state === "needs examples" ? "state needs" : "state";
  const head = el("tr", {}, ...f.inputs.map((n) => el("th", {}, view.column(f.qualified, n))), el("th", { class: "out" }, view.column(f.qualified, f.output)), el("th", { class: "row-actions-heading" }, "Actions"));
  const rows = f.examples.map((ex) => {
    const callText = `${f.qualified}(${ex.inputs.join(", ")})`;
    if (state.editing?.fn === f.qualified && state.editing.call === callText) return editRow(f, [...ex.inputs, ex.output], callText);
    return el("tr", { class: (state.fresh.has(rowKey(f.qualified, ex)) ? "fresh " : "") + "row", title: "Click to edit", onclick: () => { state.editing = { fn: f.qualified, call: callText }; renderFunctions(); focusRow(f.qualified, "tr.editing"); } },
      ...ex.inputs.map((v) => el("td", {}, view.readable(v))),
      el("td", { class: "out" }, view.readable(ex.output)),
      el("td", { class: "row-actions" },
        el("button", { type: "button", class: "x", "aria-label": `Edit example ${callText}`, onclick: e => { e.stopPropagation(); state.editing = { fn: f.qualified, call: callText }; renderFunctions(); focusRow(f.qualified, "tr.editing"); } }, "Edit"),
        el("button", { type: "button", class: "x", title: `Remove example ${callText}`, "aria-label": `Remove example ${callText}`, onclick: (e) => { e.stopPropagation(); call("removeExample", callText); } }, "×")));
  });
  rows.push(editRow(f, [], ""));
  const note = state.note?.fn === f.qualified ? state.note : null;
  return el("article", { class: "fn", "data-fn": f.qualified },
    el("header", {},
      el("h3", {}, view.title(f.qualified)),
      el("span", { class: stateClass }, f.state === "example-only" ? "Saved answers" : f.state === "needs examples" ? "Rule needs examples" : "Rule + examples"),
      el("span", { class: "example-count" }, `${f.examples.length} examples`)),
    f.description ? el("p", { class: "desc" }, f.description) : null,
    el("div", { class: "table-wrap" }, el("table", {}, el("thead", {}, head), el("tbody", {}, rows))),
    note ? el("div", { class: "note note-" + note.kind },
      el("p", {}, note.text),
      ...(note.lines || []).map((l) => el("p", { class: "fail" }, l))) : null,
    el("details", { class: "function-details", open: expanded }, el("summary", {}, "Function details & import"),
      el("code", {}, `${f.qualified.split(".")[0]}.${f.signature}`),
      el("button", { type: "button", class: "quiet", onclick: () => openFile(f.path, f.line) }, "View code"), importBar(f)),
    f.examples.length ? null : el("p", { class: "hint" }, "No rows yet. Type one and press Enter, paste rows from a spreadsheet, or import a CSV or TSV file."));
}

// importBar picks where pasted or imported rows go (examples, or proposals
// to confirm in Questions) and imports a CSV/TSV file. A file's first row may
// name the columns, in any order; without one, rows are inputs then output.
function importBar(f) {
  const mode = el("select", { "aria-label": `Where pasted or imported rows of ${f.qualified} go`, onchange: (e) => { state.importAs[f.qualified] = e.target.value; } },
    el("option", { value: "examples" }, "as examples"),
    el("option", { value: "propose" }, "as proposals to confirm"));
  mode.value = importMode(f.qualified);
  const file = el("input", { type: "file", accept: ".csv,.tsv,.txt,text/csv,text/tab-separated-values,text/plain", hidden: true, onchange: async (e) => {
    const picked = e.target.files?.[0];
    e.target.value = "";
    if (!picked) return;
    if (picked.size > MAX_IMPORT_BYTES) {
      state.note = { fn: f.qualified, kind: "error", text: `${picked.name} is over ${MAX_IMPORT_BYTES >> 20} MB.` };
      renderFunctions();
      return;
    }
    let text;
    try { text = await picked.text(); } catch (err) {
      state.note = { fn: f.qualified, kind: "error", text: `Could not read ${picked.name}: ${err}` };
      renderFunctions();
      return;
    }
    await importText(f.qualified, text, picked.name);
  } });
  return el("div", { class: "importbar" },
    el("label", {}, "Pasted and imported rows go in ", mode),
    el("button", { type: "button", class: "quiet", title: "Read a CSV or TSV file; its first row may name the columns", onclick: () => file.click() }, "Import file…"),
    file);
}

const importMode = (fn) => state.importAs[fn] || "examples";

const rowKey = (fn, ex) => fn + "(" + ex.inputs.join(",") + ")=" + ex.output;

// editRow is the blank row at the bottom of a table (replace = ""), or an
// existing row being edited (replace = its call). Typed text survives
// re-renders as a draft until it's saved.
function editRow(f, values, replace) {
  const draftKey = f.qualified + "\0" + replace;
  const draft = state.drafts[draftKey] || view.editableRow(f, values);
  const names = [...f.inputs, f.output];
  const inputs = names.map((name, i) => el("input", { class: "cell", value: draft[i] ?? "", placeholder: view.column(f.qualified, name), "aria-label": `${name} for a ${replace ? "row being edited" : "new row"} of ${f.qualified}`, spellcheck: "false", autocomplete: "off" }));
  const typed = () => inputs.map((i) => i.value);
  const save = async () => {
    const cells = typed();
    if (!replace && cells.every((c) => !c.trim())) return;
    const r = await call("putExample", f.qualified, JSON.stringify(cells), replace);
    tableResult(f.qualified, r, draftKey);
    if (r.status !== "error") { state.editing = null; renderFunctions(); focusRow(f.qualified, "tr.newrow"); }
  };
  for (const input of inputs) {
    input.addEventListener("input", () => { state.drafts[draftKey] = typed(); serverPanel?.render(); });
    input.addEventListener("keydown", (e) => {
      if (e.key === "Enter") { e.preventDefault(); save(); }
      if (e.key === "Escape") { delete state.drafts[draftKey]; state.editing = null; renderFunctions(); serverPanel?.render(); }
    });
    input.addEventListener("paste", (e) => {
      const text = e.clipboardData?.getData("text/plain") || "";
      if (!/[\t\n]/.test(text.trim())) return; // one value: an ordinary paste into the cell
      e.preventDefault();
      pasteRows(f.qualified, text);
    });
  }
  return el("tr", { class: replace ? "editing" : "newrow" },
    ...inputs.map((input, i) => el("td", { class: i === names.length - 1 ? "out" : "" }, input)),
    el("td", {}, el("button", { type: "button", class: "x add", title: replace ? "Save this row (Enter)" : "Add this row (Enter)", onclick: save }, replace ? "✓" : "+")));
}

async function pasteRows(fn, text) {
  const r = await call("importRows", fn, text, importMode(fn));
  tableResult(fn, r, fn + "\0");
  focusRow(fn, "tr.newrow");
}

async function importText(fn, text, name) {
  const r = await call("importRows", fn, text, importMode(fn));
  if (r.status === "error") r.message = `${name}: ${r.message}`;
  tableResult(fn, r, null);
}

// tableResult shows what a table edit did under that table.
function tableResult(fn, r, draftKey) {
  if (r.status === "error") state.note = { fn, kind: "error", text: r.message };
  else {
    if (draftKey !== null) delete state.drafts[draftKey];
    state.note = { fn, kind: r.lines?.length ? "warn" : "ok", text: r.message, lines: (r.lines || []).map((l) => l.text) };
  }
  renderFunctions();
  serverPanel?.render();
}

function focusRow(fn, which) {
  document.querySelector(`article.fn[data-fn="${CSS.escape(fn)}"] ${which} input`)?.focus();
}

let shownCatalog = "";
function renderCatalog() {
  const html = state.snapshot?.catalog || "";
  if (html && html !== shownCatalog && state.view === "catalog") {
    $("catalog").srcdoc = html;
    shownCatalog = html;
  }
}

function renderSample() {
  const picker = $("sample");
  picker.value = String(state.sample);
  $("reset").disabled = state.sample < 0;
}

function allFunctions() { return (state.snapshot?.processes || []).flatMap(p => p.functions); }
function functionFor(name) { return allFunctions().find(f => f.qualified === name); }
function setCall(text) {
  $("call").value = text;
  const expression = view.expressionFromCall(text);
  state.friendlyInput = expression !== null && preferences.rawCall !== true;
  $("expression").value = expression ?? "";
}
function renderInput() {
  const fn = functionFor("WordAdd.eval");
  const available = fn && fn.inputs.length === 1 && fn.inputs[0] === "text";
  if (!available) state.friendlyInput = false;
  $("expression-entry").hidden = !state.friendlyInput;
  $("call-entry").hidden = state.friendlyInput;
  $("input-toggle").hidden = !available;
  $("input-toggle").textContent = state.friendlyInput ? "Use a function call" : "Use an expression";
  $("runform").querySelector("button[type=submit]").textContent = state.friendlyInput ? "Calculate" : "Run";
  $("try-heading").textContent = state.friendlyInput ? "What would you like to add?" : "Try a function";
  $("input-help").textContent = state.friendlyInput ? "Two English numbers from 0 to 99, joined by “plus.” Use hyphens, as in twenty-two." : "Enter a function call. Its examples below show the inputs it accepts.";
  $("try-suggestions").hidden = !state.friendlyInput || !$("welcome").open;
  $("welcome").hidden = !available;
}
function toggleCode(open, persist = true) {
  $("source-pane").hidden = !open;
  document.body.classList.toggle("code-open", open);
  $("code-toggle").textContent = open ? "Hide code" : "Show code";
  $("code-toggle").setAttribute("aria-expanded", String(open));
  if (open) requestAnimationFrame(() => {
    editor.refresh?.();
    if (persist && matchMedia("(max-width: 1000px)").matches) $("source-pane").scrollIntoView({ block: "start" });
  });
  if (persist) { preferences.code = open; savePreferences(); }
}
function showResult(r, note) {
  if (r.obsolete) return;
  state.result = r;
  state.resultKey = r.projectKey || view.projectKey(state.files);
  state.resultNote = note || "";
  renderResult();
  renderQuestions();
}
let shownWhy = null;
function renderResult() {
  const r = state.result;
  $("result").hidden = !r;
  if (!r) return;
  const stale = state.resultKey !== view.projectKey(state.files);
  const inputChanged = r.executedCall && r.executedCall !== $("call").value.trim();
  const status = $("status");
  const statuses = { ok: "Answer", unresolved: "Needs your answer", unconfirmed: "Answer needs confirmation", error: "Couldn't complete this", failed: "Checks found a disagreement" };
  status.textContent = statuses[r.status] || r.status;
  status.className = "badge s-" + r.status;
  $("output").textContent = r.output ? view.readable(r.output) : "";
  $("result-input").textContent = r.executedCall ? view.expressionFromCall(r.executedCall) ?? r.executedCall : "";
  $("result-stale").hidden = !stale && !inputChanged;
  $("result-stale").textContent = stale ? "The project changed. This explanation is from the previous version. Run again to update it." : "The input changed. Run again for a new answer.";
  let message = r.message || "";
  if (r.status === "ok" && r.why) message = r.why.source === "example" ? "This exact input has a saved answer. The calculation did not run." : r.why.steps?.length ? "A rule worked this out using the smaller answers below." : "The function's rule calculated this answer.";
  if (r.status === "unresolved") message = "One answer is missing. Save it below, and Park will rerun the original request.";
  if (r.status === "unconfirmed") message = "Some rules need more examples. Confirm or correct the suggested answers below.";
  const text = $("message");
  text.replaceChildren([state.resultNote, message].filter(Boolean).join(" "));
  const where = r.status === "error" ? errorLocation(r.message) : null;
  if (where && where.file in state.files && !stale) text.append(" ", el("button", { type: "button", class: "quiet", onclick: () => openFile(where.file, where.line) }, "Inspect code"));
  $("why").hidden = !r.why;
  if (shownWhy !== r) {
    shownWhy = r;
    $("explanation").replaceChildren(...(r.why ? [explainStep(r.why)] : []));
    $("whytree").replaceChildren(...(r.why ? [whyStep(r.why)] : []));
  }
  for (const button of $("explanation").querySelectorAll("button[data-trace-action]")) button.disabled = stale;
  for (const link of $("whytree").querySelectorAll("a")) {
    link.setAttribute("aria-disabled", String(stale));
    link.dataset.stale = String(stale);
  }
}
function traceIsCurrent() { return state.resultKey === view.projectKey(state.files); }
function inspectExamples(s, edit) {
  if (!traceIsCurrent()) return;
  const f = functionFor(s.function);
  if (!f) return;
  selectView("functions");
  const match = view.identity(s, f);
  if (edit && match) state.editing = { fn: f.qualified, call: match.call };
  renderFunctions();
  const card = document.querySelector(`article.fn[data-fn="${CSS.escape(f.qualified)}"]`);
  card?.scrollIntoView({ behavior: "smooth", block: "start" });
  if (edit && match) focusRow(f.qualified, "tr.editing");
}
function explainStep(s, depth = 0) {
  const f = functionFor(s.function);
  const label = view.title(s.function || s.call);
  const origin = view.origins[s.source] || "No answer";
  const outcome = view.outcome(s);
  const values = view.inputs(s);
  const node = el("details", { class: "explain-node", "data-origin": s.source, open: depth < 2 && Boolean(s.steps?.length) },
    el("summary", {}, el("span", { class: "node-heading" }, el("span", { class: "node-title" }, label), el("span", { class: "origin" }, origin)),
      el("span", { class: "node-value" }, values || s.call, el("span", { class: "node-arrow", "aria-label": "returns" }, " → "), el("strong", {}, s.failed ? (view.outcome(s).includes("Waiting") || s.source === "unknown" ? "Waiting" : "No result") : view.readable(s.output))),
      outcome ? el("span", { class: "node-outcome" }, outcome) : null));
  let description = f?.description || "Inspect this function's examples or its code.";
  if (s.source === "example") description = "A saved example answers this exact input. No code ran for this step.";
  if (s.source === "unknown") description = "This input has no saved answer and no implementation. Supply its answer to continue.";
  if (s.source === "proposed") description = `Suggested by ${s.by || "a resolver"}. Confirm or correct this answer before relying on it.`;
  if (s.error) description = s.error;
  const actions = el("div", { class: "node-actions" });
  if (s.source === "unknown" || s.source === "unconfirmed code" || s.source === "proposed") {
    actions.append(el("button", { type: "button", class: "quiet", "data-trace-action": "question", onclick: () => {
      if (!traceIsCurrent()) return;
      const question = (state.snapshot?.questions || []).find(q => q.function === s.function && (s.inputs || []).every(n => q.input?.[n] === s.input?.[n]));
      const q = question && document.querySelector(`[data-question="${CSS.escape(question.call)}"]`);
      q?.scrollIntoView({ behavior: "smooth", block: "center" }); q?.querySelector("input")?.focus();
    } }, s.source === "unknown" ? "Answer this question" : "Review suggested answers"));
  }
  if (f) actions.append(el("button", { type: "button", class: "quiet", "data-trace-action": "examples", onclick: () => inspectExamples(s, s.source === "example") }, s.source === "example" ? "Edit this saved answer" : "See examples"));
  if (s.path) actions.append(el("button", { type: "button", class: "quiet", "data-trace-action": "code", onclick: () => { if (traceIsCurrent()) openFile(s.path, s.line); } }, s.source === "example" ? "See source" : "View code"));
  if (s.steps?.length) node.append(el("div", { class: "node-children" }, s.steps.map(child => explainStep(child, depth + 1))));
  if (s.steps?.length) node.append(el("details", { class: "node-detail" }, el("summary", {}, "Inspect this rule"), el("p", {}, description), actions));
  else node.append(el("div", { class: "node-detail" }, el("p", {}, description), actions));
  return node;
}

// whyStep renders one call of a run's provenance tree (park run --why): the
// call, its answer, where the answer came from, and the calls it made.
function whyStep(s) {
  const source = s.error ? "failed" : s.source || "failed";
  const where = s.path ? `${s.path}:${s.line}` : "";
  return el("li", {},
    el("div", { class: "step" },
      el("code", {}, s.call, el("span", { class: "eq" }, " = "), el("b", {}, s.output)),
      el("span", { class: "src src-" + source.replace(/ /g, "-") }, source),
      where ? el("a", { href: "#", onclick: (e) => { e.preventDefault(); if (traceIsCurrent()) openFile(s.path, s.line); } }, where) : null),
    s.error ? el("p", { class: "step-error" }, s.error.split("\n")[0]) : null,
    s.steps?.length ? el("ul", {}, s.steps.map(whyStep)) : null);
}

function toast(text) {
  const t = $("toast");
  t.textContent = text;
  t.hidden = false;
  clearTimeout(toast.timer);
  toast.timer = setTimeout(() => { t.hidden = true; }, 3500);
}

// ---- Actions --------------------------------------------------------------

async function run(text) {
  text = text.trim();
  if (!text) return;
  state.lastCall = text;
  save();
  showResult(await call("run", text));
}

async function answer(line) {
  const r = await call("addExample", line);
  if (r.obsolete || r.status !== "ok") return r;
  // Resume: rerun the call that raised the question, now that it's answered.
  if (state.lastCall) showResult(await call("run", state.lastCall), "Saved your example and reran the original request.");
  else showResult(r);
  return r;
}

async function dismiss(callText) {
  const r = await call("dismiss", callText);
  if (r.status !== "ok") showResult(r);
  else toast(r.message + ".");
}

function openFile(name, line) {
  if (!(name in state.files)) return;
  state.active = name;
  toggleCode(true);
  render();
  if (line) editor.goto(line);
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
  state.files[state.active] = editor.get();
  serverPanel?.render();
  renderResult();
  $("test-stale").hidden = !state.testKey || state.testKey === view.projectKey(state.files);
  save();
  clearTimeout(loadTimer);
  loadTimer = setTimeout(() => call("load"), 300);
}

function loadProject({ files, call: callText = "", active = "", sample = 0, sampleName, serverDraft = false }) {
  clearTimeout(loadTimer);
  if (!serverDraft) serverPanel?.detach();
  state.projectID++;
  state.files = { ...files };
  state.active = active in files ? active : Object.keys(files).filter((n) => n.endsWith(".star")).sort()[0] || "";
  // Older saves used picker positions. Keep Reset tied to the original
  // sample when new projects are inserted into the picker.
  if (Number.isInteger(sample) && sample >= 0) {
    const name = sampleName ?? ["Shipping quotes", "Roman numerals", "Robot on a grid", "Shapes"][sample];
    sample = globalThis.PARK_SAMPLES.findIndex((s) => s.name === name);
  }
  state.sample = sample === TOUR_SAMPLE || (Number.isInteger(sample) && sample >= 0 && sample < globalThis.PARK_SAMPLES.length) ? sample : -1;
  state.lastCall = callText;
  state.fresh = new Set();
  state.editing = null;
  state.drafts = {};
  state.note = null;
  state.result = null;
  state.testKey = "";
  state.questionDrafts = {};
  editor.reset();
  setCall(callText);
  $("result").hidden = true;
  $("testout").replaceChildren();
  return call("load");
}

function loadSample(i) {
  endTour();
  const s = globalThis.PARK_SAMPLES[i];
  return loadProject({ files: s.files, call: s.call, sample: i, sampleName: s.name });
}

function selectView(view) {
  state.view = view;
  for (const tab of document.querySelectorAll(".tab[data-view]")) tab.setAttribute("aria-selected", String(tab.dataset.view === view));
  for (const v of ["functions", "catalog", "tests"]) $("view-" + v).hidden = v !== view;
  renderCatalog();
}

async function runTests() {
  const r = await call("test");
  if (r.obsolete) return;
  state.testKey = r.projectKey || view.projectKey(state.files);
  $("test-stale").hidden = state.testKey === view.projectKey(state.files);
  $("testout").replaceChildren(...(r.lines || []).map((l) => el("div", { class: l.failed ? "fail" : "" }, l.text)), el("div", {}, r.message || ""));
}

// ---- Saving, sharing, downloading -----------------------------------------

function saved() {
  return { files: state.files, call: $("call").value, active: state.active, sample: state.sample, sampleName: globalThis.PARK_SAMPLES[state.sample]?.name };
}

// save writes immediately: a project is a few kilobytes, and a debounced save
// loses the last edit when the page reloads right after it.
function save() {
  if (!state.active && !Object.keys(state.files).length) return; // nothing loaded yet
  const [key, data] = state.tour ? [TOUR_KEY, { tour: state.tour, project: saved() }] : [STORAGE_KEY, saved()];
  try { localStorage.setItem(key, JSON.stringify(data)); } catch { /* storage may be unavailable */ }
}

function restore(key = STORAGE_KEY) {
  try {
    const data = JSON.parse(localStorage.getItem(key) || "null");
    if (data && data.files && typeof data.files === "object") return data;
  } catch { /* ignore unreadable storage */ }
  return null;
}

const toBase64url = (bytes) => {
  let s = "";
  for (let i = 0; i < bytes.length; i += 0x8000) s += String.fromCharCode(...bytes.subarray(i, i + 0x8000));
  return btoa(s).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
};
const fromBase64url = (text) => Uint8Array.from(atob(text.replace(/-/g, "+").replace(/_/g, "/")), (c) => c.charCodeAt(0));

async function pipe(bytes, transform) {
  return new Uint8Array(await new Response(new Blob([bytes]).stream().pipeThrough(transform)).arrayBuffer());
}

// A share link carries the whole project, compressed, in the URL fragment;
// fragments never reach a server.
async function shareLink() {
  const { files, call: callText, active } = saved();
  const json = new TextEncoder().encode(JSON.stringify({ v: 1, files, call: callText, active }));
  const packed = await pipe(json, new CompressionStream("deflate-raw"));
  return baseURL() + SHARE_PREFIX + toBase64url(packed);
}

function baseURL() { return window.location.href.split("#")[0]; }

async function readShared() {
  const hash = window.location.hash;
  if (!hash.startsWith(SHARE_PREFIX)) return null;
  const json = await pipe(fromBase64url(hash.slice(SHARE_PREFIX.length)), new DecompressionStream("deflate-raw"));
  const data = JSON.parse(new TextDecoder().decode(json));
  if (data.v !== 1 || !data.files || typeof data.files !== "object") throw new Error("unrecognized share link");
  return { files: data.files, call: data.call || "", active: data.active || "", sample: -1 };
}

async function share() {
  const link = await shareLink();
  try {
    await navigator.clipboard.writeText(link);
    toast(`Link copied (${link.length.toLocaleString()} characters). It holds the whole project.`);
  } catch {
    prompt("Copy this link. It holds the whole project:", link);
  }
}

async function download() {
  const r = await call("archive");
  if (!r.archive) { showResult(r); return; }
  const blob = new Blob([fromBase64(r.archive)], { type: "application/zip" });
  const a = el("a", { href: URL.createObjectURL(blob), download: "park-project.zip" });
  document.body.append(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(a.href), 1000);
  toast("Downloaded park-project.zip. Unzip it and run park in the folder.");
}
const fromBase64 = (text) => Uint8Array.from(atob(text), (c) => c.charCodeAt(0));

// ---- Guided tours ---------------------------------------------------------
// Tours are data (web/tours/*.json, bundled as tours.js with a checkpoint per
// step). The build replays each tour through the engine, so every step here
// can be completed. See docs/plan-guided-tours.md.

const ROLES = { ops: "Ops", dev: "Developer" };
// Places a step can point at, by name, so tours don't depend on page layout.
const POINTS = {
  run: () => document.querySelector("#runform button[type=submit]"),
  call: () => state.friendlyInput ? $("expression") : $("call"),
  result: () => $("result"),
  why: () => $("why"),
  questions: () => $("questions"),
  functions: () => $("view-functions"),
  editor: () => document.querySelector(".CodeMirror") || $("editor"),
  addfile: () => document.querySelector(".tab.add"),
  share: () => $("share"),
};
const point = (name) => name.startsWith("function:")
  ? document.querySelector(`article.fn[data-fn="${CSS.escape(name.slice(9))}"]`)
  : POINTS[name]?.();

const tours = () => globalThis.PARK_TOURS || [];
const currentTour = () => state.tour && tours().find((t) => t.id === state.tour.id);
const currentStep = () => currentTour()?.steps[state.tour.step];

// startTour opens a tour at a step (0-based), loading that step's checkpoint.
async function startTour(id, step = 0) {
  const tour = tours().find((t) => t.id === id);
  if (!tour) return false;
  step = Math.max(0, Math.min(step, tour.steps.length - 1));
  save(); // the reader's own project stays in its own slot
  state.tour = { id, step, last: {} };
  await loadCheckpoint(tour.steps[step]);
  enterStep(step);
  return true;
}

async function loadCheckpoint(step) {
  await loadProject({ files: step.checkpoint, call: step.checkpointCall || "", sample: TOUR_SAMPLE });
  if (step.checkpointCall) { state.lastCall = step.checkpointCall; showResult(await call("run", step.checkpointCall)); }
}

let tourTimer = 0;
function enterStep(i, { check = true } = {}) {
  clearTimeout(tourTimer);
  tourTimer = 0;
  state.tour.step = i;
  const step = currentStep();
  if (step.call) setCall(step.call);
  renderInput();
  if (step.point?.includes("why")) $("why").open = true;
  if (step.open) { const [file, line] = step.open.split(":"); openFile(file, Number(line) || 0); }
  if (step.point?.some((p) => p === "functions" || p.startsWith("function:"))) selectView("functions");
  renderTour();
  renderPoints();
  const first = step.point?.find((p) => p.startsWith("function:"));
  if (first) point(first)?.scrollIntoView({ behavior: "smooth", block: "nearest" });
  save();
  if (check) tourCheck();
}

function renderTour() {
  const panel = $("tour");
  const tour = currentTour();
  panel.hidden = !tour;
  $("tourbtn").hidden = Boolean(tour);
  document.body.classList.toggle("touring", Boolean(tour));
  if (!tour) return;
  const i = state.tour.step;
  const step = tour.steps[i];
  const last = i === tour.steps.length - 1;
  $("tourtitle").textContent = tour.title;
  $("tourcount").textContent = `${i + 1} of ${tour.steps.length}`;
  $("tourrole").textContent = ROLES[step.role] || "";
  $("tourrole").hidden = !step.role;
  $("tourrole").className = "role role-" + (step.role || "none");
  $("tourtext").textContent = step.text;
  $("toursnippet").hidden = !step.snippet;
  $("toursnippet").querySelector("pre").textContent = step.snippet || "";
  $("tourdone").hidden = true;
  $("tourback").disabled = i === 0;
  $("tourskip").hidden = !step.skip?.length;
  $("tournext").hidden = Boolean(step.goal) && !last;
  $("tournext").textContent = last ? "Finish" : "Next";
}

function renderPoints() {
  for (const node of document.querySelectorAll(".tour-point")) node.classList.remove("tour-point");
  for (const name of currentStep()?.point || []) point(name)?.classList.add("tour-point");
}

// tourCheck advances when the current step's goal holds. It runs after every
// engine operation; the goal is checked by the same Go code the build uses.
async function tourCheck() {
  const step = currentStep();
  if (!step?.goal || tourTimer) return;
  const at = state.tour.step;
  const r = await engine.call("goal", state.files, [JSON.stringify(state.tour.last), JSON.stringify(step.goal)]).catch(() => null);
  if (!r?.met || !state.tour || state.tour.step !== at) return;
  $("tourdone").hidden = false;
  tourTimer = setTimeout(() => { tourTimer = 0; if (state.tour?.step === at) enterStep(at + 1); }, 1400);
}

// skipStep does the step for the reader. If the reader's project has moved
// away from the tour (an edit can't apply, or the goal still doesn't hold),
// it loads the next step's checkpoint instead.
async function skipStep() {
  const tour = currentTour();
  const at = state.tour.step;
  const step = tour.steps[at];
  let ok = true;
  for (const op of step.skip || []) {
    let r;
    if (op.run) { state.lastCall = op.run; setCall(op.run); r = await call("run", op.run); showResult(r); }
    else if (op.addExample) r = await call("addExample", op.addExample);
    else if (op.dismiss) r = await call("dismiss", op.dismiss);
    else if (op.paste) { selectView("functions"); r = await call("pasteExamples", op.paste.function, op.paste.text); }
    else if (op.setFile) { state.files[op.setFile.file] = op.setFile.text; state.active = op.setFile.file; r = await call("load"); }
    else if (op.replace) {
      const text = state.files[op.replace.file] ?? "";
      if (text.split(op.replace.find).length !== 2) { ok = false; break; }
      state.files[op.replace.file] = text.replace(op.replace.find, op.replace.with);
      state.active = op.replace.file;
      r = await call("load");
    }
    if (!r || r.status === "error") { ok = false; break; }
  }
  if (ok) {
    const r = await engine.call("goal", state.files, [JSON.stringify(state.tour.last), JSON.stringify(step.goal)]).catch(() => null);
    ok = Boolean(r?.met);
  }
  if (!ok && at + 1 < tour.steps.length) {
    await loadCheckpoint(tour.steps[at + 1]);
    toast("Your project had moved on from the tour, so it was reset to this point.");
  }
  if (state.tour?.step === at) { clearTimeout(tourTimer); tourTimer = 0; enterStep(at + 1, { check: false }); }
}

function nextStep() {
  const tour = currentTour();
  if (state.tour.step + 1 < tour.steps.length) enterStep(state.tour.step + 1);
  else finishTour();
}

// finishTour keeps the tour's project as the reader's own.
function finishTour() {
  endTour();
  state.sample = -1;
  save();
  render();
  toast("Tour finished. This project is yours now; Reset or pick a sample to start over.");
}

// exitTour returns to the project the reader had before the tour.
async function exitTour() {
  endTour();
  const own = restore();
  if (own && Object.keys(own.files).some((n) => n.endsWith(".star"))) await loadProject(own);
  else await loadSample(0);
}

function endTour() {
  if (!state.tour) return;
  state.tour = null;
  clearTimeout(tourTimer);
  tourTimer = 0;
  try { localStorage.removeItem(TOUR_KEY); } catch { /* ignore */ }
  renderTour();
  renderPoints();
}

function readTourHash() {
  const hash = window.location.hash;
  if (!hash.startsWith(TOUR_PREFIX)) return null;
  const [id, step] = decodeURIComponent(hash.slice(TOUR_PREFIX.length)).split("/");
  return { id, step: Math.max(0, (Number(step) || 1) - 1) };
}

function savedTour() {
  try {
    const data = JSON.parse(localStorage.getItem(TOUR_KEY) || "null");
    if (data?.tour && data.project?.files && tours().some((t) => t.id === data.tour.id)) return data;
  } catch { /* ignore unreadable storage */ }
  return null;
}

// ---- Startup --------------------------------------------------------------

async function start() {
  $("welcome").open = preferences.welcome !== false;
  $("why").open = preferences.explanation !== false;
  toggleCode(preferences.code === true, false);
  $("code-toggle").addEventListener("click", () => toggleCode($("source-pane").hidden));
  $("code-close").addEventListener("click", () => { toggleCode(false); $("code-toggle").focus(); });
  $("welcome").addEventListener("toggle", () => { preferences.welcome = $("welcome").open; savePreferences(); renderInput(); });
  $("why").addEventListener("toggle", () => { preferences.explanation = $("why").open; savePreferences(); });
  $("welcome-tour").addEventListener("click", () => startTour(tours()[0]?.id));
  $("expression").addEventListener("input", () => {
    $("call").value = `WordAdd.eval(${JSON.stringify($("expression").value)})`;
    save(); renderResult();
  });
  $("input-toggle").addEventListener("click", () => {
    if (state.friendlyInput) { state.friendlyInput = false; preferences.rawCall = true; }
    else {
      const expression = view.expressionFromCall($("call").value);
      if (expression === null) { toast("Use a WordAdd.eval call with one quoted text input to switch to expression entry."); return; }
      $("expression").value = expression; state.friendlyInput = true; preferences.rawCall = false;
    }
    renderInput(); savePreferences();
    (state.friendlyInput ? $("expression") : $("call")).focus();
  });
  for (const button of document.querySelectorAll("[data-expression]")) button.addEventListener("click", () => {
    setCall(`WordAdd.eval(${JSON.stringify(button.dataset.expression)})`);
    renderInput();
    renderResult(); save();
    (state.friendlyInput ? $("expression") : $("call")).focus();
  });
  const picker = $("sample");
  picker.append(el("option", { value: "-1", disabled: true }, "Shared project"));
  globalThis.PARK_SAMPLES.forEach((s, i) => picker.append(el("option", { value: i }, s.name)));
  picker.addEventListener("change", () => loadSample(Number(picker.value)));
  $("reset").addEventListener("click", () => {
    if (confirm("Reset this sample? Your edits to it will be lost.")) loadSample(state.sample);
  });
  $("share").addEventListener("click", share);
  $("download").addEventListener("click", download);
  $("stop").addEventListener("click", () => engine.stop());
  $("runform").addEventListener("submit", (e) => { e.preventDefault(); run($("call").value); });
  $("call").addEventListener("input", () => { save(); renderResult(); });
  editor.onEdit(edited);
  $("test").addEventListener("click", runTests);
  picker.append(el("option", { value: String(TOUR_SAMPLE), disabled: true }, "Guided tour"));
  $("tourbtn").addEventListener("click", () => startTour(tours()[0]?.id));
  $("tourback").addEventListener("click", () => enterStep(state.tour.step - 1, { check: false }));
  $("tournext").addEventListener("click", nextStep);
  $("tourskip").addEventListener("click", skipStep);
  $("tourexit").addEventListener("click", exitTour);
  $("tourcopy").addEventListener("click", () => navigator.clipboard.writeText(currentStep()?.snippet || "").then(() => toast("Copied."), () => toast("Couldn't copy; select the text instead.")));
  $("tourbtn").hidden = !tours().length;
  // A tour link opened while the page is already open only changes the hash.
  window.addEventListener("hashchange", () => {
    const link = readTourHash();
    if (!link) return;
    history.replaceState(null, "", baseURL());
    startTour(link.id, link.step);
  });
  for (const tab of document.querySelectorAll(".tab[data-view]")) tab.addEventListener("click", () => selectView(tab.dataset.view));

  try {
    await engine.start();
  } catch (err) {
    $("loading").textContent = "Couldn't load park.wasm: " + err.message;
    return;
  }
  // Opening order: a share link, a tour link, a tour in progress, the saved
  // project, and on a first visit, the default sample with an optional guide.
  if (globalThis.PARK_DRAFT_API) serverPanel = globalThis.ParkDraftPanel.initialize({
    files: () => state.files,
    pending: () => Object.entries(state.drafts).some(([row, cells]) => functionFor(row.split("\0")[0]) && cells.some(value => value.trim())) || (state.snapshot?.questions || []).some(q => state.questionDrafts[q.call]?.trim()),
    workingKey: () => JSON.stringify([view.projectKey(state.files), state.drafts, state.questionDrafts]),
    replace: files => { endTour(); return loadProject({ files, call: $("call").value, sample: -1, serverDraft: true }); },
  });
  let project = null;
  try {
    project = await readShared();
  } catch (err) {
    toast("Couldn't open the shared link: " + err.message);
  }
  const tourLink = readTourHash();
  if (project || tourLink) history.replaceState(null, "", baseURL()); // saved locally from here on
  const inTour = savedTour();
  const own = restore();
  const hasFiles = (p) => p && Object.keys(p.files).some((n) => n.endsWith(".star"));
  if (hasFiles(project)) await loadProject(project);
  else if (tourLink && await startTour(tourLink.id, tourLink.step)) { /* started */ }
  else if (inTour) {
    state.tour = inTour.tour;
    await loadProject({ ...inTour.project, sample: TOUR_SAMPLE });
    enterStep(state.tour.step, { check: false });
  }
  else if (hasFiles(own)) await loadProject(own);
  else await loadSample(0);
  $("loading").hidden = true;
}

start();
