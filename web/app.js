// Park playground. JavaScript owns the project's files; every operation hands
// them to the Go engine (park.wasm, in worker.js) and takes back the new files
// and a snapshot of the project. See docs/plan-web-playground.md.
"use strict";

const QUESTIONS = "questions.txt";
const STORAGE_KEY = "park-playground-v1";
const SHARE_PREFIX = "#p=";
const $ = (id) => document.getElementById(id);

const state = {
  files: {},          // name -> source, including questions.txt when non-empty
  active: "",         // file shown in the editor
  snapshot: null,     // last project description from Go
  lastCall: "",       // rerun after an answer, so the call resumes
  fresh: new Set(),   // example keys added by the last edit, highlighted
  view: "functions",
  sample: 0,          // sample the project started from; -1 for a shared link
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
  const job = queue.then(() => execute(op, args));
  queue = job.catch(() => {});
  return job;
}

let busyTimer = 0;
async function execute(op, args) {
  clearTimeout(busyTimer);
  busyTimer = setTimeout(() => { document.body.classList.add("busy"); $("stop").hidden = false; }, 800);
  const before = exampleKeys(state.snapshot);
  try {
    const sent = { ...state.files };
    const result = await engine.call(op, sent, args);
    if (result.files) {
      // Keep what the user typed while this call ran: a local edit wins.
      for (const [name, text] of Object.entries(state.files)) if (text !== sent[name]) result.files[name] = text;
      state.files = result.files;
    }
    if (result.snapshot) state.snapshot = result.snapshot;
    if (op === "addExample") {
      const after = exampleKeys(state.snapshot);
      state.fresh = new Set([...after].filter((k) => !before.has(k)));
    } else if (op === "removeExample") {
      state.fresh = new Set();
    }
    render();
    save();
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
    for (const f of p.functions) for (const ex of f.examples) keys.add(f.qualified + "(" + ex.inputs.join(",") + ")");
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
  renderTabs();
  renderEditor();
  renderProblem();
  renderQuestions();
  renderFunctions();
  renderCatalog();
  renderSample();
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

function renderSample() {
  const picker = $("sample");
  picker.value = String(state.sample);
  $("reset").disabled = state.sample < 0;
}

function showResult(r, note) {
  $("result").hidden = false;
  const status = $("status");
  status.textContent = r.status;
  status.className = "badge s-" + r.status;
  $("output").textContent = r.output || "";
  const message = [note, r.message].filter(Boolean).join(" ");
  const where = r.status === "error" ? errorLocation(r.message) : null;
  const text = $("message");
  text.replaceChildren(message);
  if (where && where.file in state.files) {
    text.append(" ", el("a", { href: "#", onclick: (e) => { e.preventDefault(); openFile(where.file, where.line); } }, `Show ${where.file}:${where.line}`));
  }
  $("why").hidden = !r.why;
  $("whytree").replaceChildren(...(r.why ? [whyStep(r.why)] : []));
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
      where ? el("a", { href: "#", onclick: (e) => { e.preventDefault(); openFile(s.path, s.line); } }, where) : null),
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
  if (r.status !== "ok") { showResult(r); return; }
  // Resume: rerun the call that raised the question, now that it's answered.
  if (state.lastCall) showResult(await call("run", state.lastCall), `Answered ${line.split(" == ")[0]}, then reran ${state.lastCall}.`);
  else showResult(r);
}

function openFile(name, line) {
  if (!(name in state.files)) return;
  state.active = name;
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
  save();
  clearTimeout(loadTimer);
  loadTimer = setTimeout(() => call("load"), 300);
}

function loadProject({ files, call: callText = "", active = "", sample = 0 }) {
  state.files = { ...files };
  state.active = active in files ? active : Object.keys(files).filter((n) => n.endsWith(".star")).sort()[0] || "";
  state.sample = Number.isInteger(sample) && sample < globalThis.PARK_SAMPLES.length ? sample : -1;
  state.lastCall = callText;
  state.fresh = new Set();
  editor.reset();
  $("call").value = callText;
  $("result").hidden = true;
  $("testout").replaceChildren();
  return call("load");
}

function loadSample(i) {
  const s = globalThis.PARK_SAMPLES[i];
  return loadProject({ files: s.files, call: s.call, sample: i });
}

function selectView(view) {
  state.view = view;
  for (const tab of document.querySelectorAll(".tab[data-view]")) tab.setAttribute("aria-selected", String(tab.dataset.view === view));
  for (const v of ["functions", "catalog", "tests"]) $("view-" + v).hidden = v !== view;
  renderCatalog();
}

async function runTests() {
  const r = await call("test");
  $("testout").replaceChildren(...(r.lines || []).map((l) => el("div", { class: l.failed ? "fail" : "" }, l.text)), el("div", {}, r.message || ""));
}

// ---- Saving, sharing, downloading -----------------------------------------

function saved() {
  return { files: state.files, call: $("call").value, active: state.active, sample: state.sample };
}

// save writes immediately: a project is a few kilobytes, and a debounced save
// loses the last edit when the page reloads right after it.
function save() {
  if (!state.active && !Object.keys(state.files).length) return; // nothing loaded yet
  try { localStorage.setItem(STORAGE_KEY, JSON.stringify(saved())); } catch { /* storage may be unavailable */ }
}

function restore() {
  try {
    const data = JSON.parse(localStorage.getItem(STORAGE_KEY) || "null");
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

// ---- Startup --------------------------------------------------------------

async function start() {
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
  $("call").addEventListener("input", save);
  editor.onEdit(edited);
  $("test").addEventListener("click", runTests);
  for (const tab of document.querySelectorAll(".tab[data-view]")) tab.addEventListener("click", () => selectView(tab.dataset.view));

  try {
    await engine.start();
  } catch (err) {
    $("loading").textContent = "Couldn't load park.wasm: " + err.message;
    return;
  }
  let project = null;
  try {
    project = await readShared();
  } catch (err) {
    toast("Couldn't open the shared link: " + err.message);
  }
  if (project) history.replaceState(null, "", baseURL()); // saved locally from here on
  project ||= restore();
  if (project && Object.keys(project.files).some((n) => n.endsWith(".star"))) await loadProject(project);
  else await loadSample(0);
  $("loading").hidden = true;
}

start();
