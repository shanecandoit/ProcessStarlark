(function (root) {
  "use strict";
  function initialize(hooks) {
    const $ = id => document.getElementById(id);
    const { Client, key } = root.ParkDrafts;
    let client = new Client(), busy = false, versions = [], serverResult = null;
    const buttons = ["draft-create", "draft-save", "draft-open", "draft-refresh", "draft-branch", "draft-run", "draft-questions-refresh"];
    const answers = new Map();
    const message = text => { $("draft-message").textContent = text; };
    const short = id => id.slice(0, 12);
    const newID = () => "web-" + root.crypto.randomUUID();
    const name = () => $("draft-name").value.trim() || "My experiment";
    const author = () => $("draft-author").value.trim() || "playground";
    const option = (value, text) => new Option(text, value);
    const workingKey = () => hooks.workingKey?.() ?? key(hooks.files());
    const pending = () => Boolean(hooks.pending?.()) || [...$("draft-questions").querySelectorAll("input")].some(input => input.value.trim());
    function render() {
      if (!root.PARK_DRAFT_API) return;
      const draft = client.draft;
      $("draft-save-state").textContent = pending() ? "· finish answers or table edits" : !draft ? "· browser only" : client.dirty(hooks.files()) ? "· local changes" : "· source saved";
      for (const id of buttons) $(id).disabled = busy;
      $("draft-save").disabled = busy || pending() || !draft || !client.dirty(hooks.files());
      $("draft-create").disabled = busy || pending();
      $("draft-export").hidden = !draft;
      if (draft) $("draft-export").href = `/api/v1/drafts/${encodeURIComponent(draft.id)}/export`;
      $("draft-branch").disabled = busy || !versions.length;
      $("draft-run").disabled = busy || !draft || pending() || client.dirty(hooks.files());
      $("draft-questions-refresh").disabled = busy || !draft;
      for (const button of $("draft-questions").querySelectorAll("button")) button.disabled = busy || (button.dataset.action === "answer" && (hooks.pending?.() || client.dirty(hooks.files())));
      $("draft-replace-warning").hidden = !pending() && !client.dirty(hooks.files());
      if (serverResult) {
        const stale = serverResult.draft !== draft?.id || serverResult.revision !== draft?.revision || serverResult.call !== hooks.call();
        $("draft-server-result").textContent = serverResult.text + (stale ? " · Saved source or input changed; run again." : "");
      }
    }
    function preview() {
      const v = versions.find(v => v.id === $("draft-versions").value);
      $("draft-version-preview").hidden = !v;
      if (!v) return;
      $("draft-version-id").textContent = `Version: ${v.id} · Parent: ${v.parent || "root"} · Source: ${v.source_revision}`;
      $("draft-version-file").replaceChildren(...Object.keys(v.files).sort().map(file => option(file, file)));
      previewFile();
    }
    function previewFile() {
      const v = versions.find(v => v.id === $("draft-versions").value);
      $("draft-version-source").textContent = v?.files[$("draft-version-file").value] || "";
    }
    async function history(c) {
      const found = await c.versions();
      if (c !== client) return;
      versions = found;
      $("draft-versions").replaceChildren(...found.map((v, i) => option(v.id, `${i === 0 ? "Latest" : "Earlier"} · ${short(v.id)}${v.parent ? " ← " + short(v.parent) : " · root"}`)));
      preview();
      $("draft-created").textContent = `Created by ${c.draft.created_by} · ${new Date(c.draft.created_at).toLocaleString()}`;
      $("draft-activity").replaceChildren(...(c.draft.history || []).slice(-10).reverse().map(entry => {
        const item = document.createElement("li"); item.textContent = `${entry.author} · ${new Date(entry.timestamp).toLocaleString()} · ${entry.message} · ${short(entry.revision)}`; return item;
      }));
      await questions(c);
    }
    async function list(c) {
      const value = await c.request("drafts");
      if (c !== client) return;
      const selected = c.draft?.id || $("draft-picker").value;
      $("draft-picker").replaceChildren(option("", "Choose a draft"), ...value.drafts.map(d => option(d.id, `${d.name} · ${d.id}`)));
      $("draft-picker").value = selected;
    }
    async function questions(c) {
      const found = await c.questions($("draft-questions-all").checked);
      if (c !== client) return;
      $("draft-question-count").textContent = `(${found.filter(q => q.status === "open").length} open)`;
      $("draft-questions").replaceChildren(...found.map(q => {
        const item = document.createElement("li");
        const label = document.createElement("p"); label.textContent = `${q.call} · ${q.status}`; item.append(label);
        const info = document.createElement("p"); info.className = "hint";
        info.textContent = `Opened by ${q.opened_by_name} · ${new Date(q.opened_at).toLocaleString()}${q.closed_by_name ? " · Closed by " + q.closed_by_name : ""}`; item.append(info);
        if (q.status !== "open") return item;
        const input = document.createElement("input");
        const answerKey = `${c.draft.id}:${q.id}`;
        input.setAttribute("aria-label", `Server answer for ${q.call}`);
        const fn = c.draft.snapshot.processes.flatMap(p => p.functions).find(f => f.qualified === q.function);
        const text = root.ParkView.textOutput(fn);
        input.placeholder = text ? "Text answer" : 'Value, e.g. 12 or "text"'; input.value = answers.get(answerKey) || "";
        input.addEventListener("input", () => { answers.set(answerKey, input.value); render(); }); item.append(input);
        const submit = document.createElement("button"); submit.type = "button"; submit.textContent = "Save answer"; submit.dataset.action = "answer";
        submit.addEventListener("click", () => {
          if (!input.value.trim()) return;
          const typed = input.value; let literal = typed.trim();
          if (text) { try { if (typeof JSON.parse(literal) !== "string") literal = JSON.stringify(typed); } catch { literal = JSON.stringify(typed); } }
          const sent = { ...hooks.files() };
          action(async active => {
            if (active !== c || hooks.pending?.() || c.dirty(hooks.files())) return;
            await c.operation({ kind: "add-example", example: `${q.call} == ${literal}` });
            if (c !== client) return;
            if (answers.get(answerKey) === typed) answers.delete(answerKey);
            await hooks.saved?.(sent, c.draft.files);
            message("Saved the server answer. Run saved source again to reuse it."); await history(c);
          });
        }); item.append(submit);
        const dismiss = document.createElement("button"); dismiss.type = "button"; dismiss.className = "quiet"; dismiss.textContent = "Dismiss question";
        dismiss.addEventListener("click", () => action(async active => {
          if (active !== c) return;
          await c.operation({ kind: "dismiss", call: q.call });
          if (c !== client) return;
          message("Dismissed the server question. Running that missing call again can reopen it."); await questions(c);
        })); item.append(dismiss);
        return item;
      }));
    }
    async function action(work) {
      if (busy) return;
      busy = true; render();
      const c = client;
      c.author = author();
      try { await work(c); }
      catch (error) {
        if (c !== client) return;
        message(error.status === 409 ? "Save conflict. Your local edits are intact. The server changed; inspect its latest source before choosing how to continue." : `${error.message}. Your local edits are intact.`);
        if (error.status === 409) {
          try { await history(c); } catch { /* original conflict remains visible */ }
        }
      } finally { busy = false; render(); }
    }
    function canReplace() {
      if ((!pending() && !client.dirty(hooks.files())) || $("draft-replace-ok").checked) return true;
      message("Your browser work is not saved to this draft. Finish table edits and save or download source first, then select the replacement checkbox to open a server copy.");
      return false;
    }
    async function replace(c, draft, startedKey) {
      if (c !== client) return;
      if (workingKey() !== startedKey) {
        message("Your browser source changed while opening the draft. It is still here; open the server copy again when ready.");
        return;
      }
      await hooks.replace(draft.files);
      serverResult = null; $("draft-server-result").textContent = "";
      $("draft-replace-ok").checked = false;
      message(`Opened ${draft.name}. Source version ${short(draft.revision)}. Editing creates a new version when you save.`);
    }
    $("draft-panel").hidden = !root.PARK_DRAFT_API;
    $("draft-panel").addEventListener("toggle", () => { if ($("draft-panel").open) action(c => list(c)); });
    $("draft-create").addEventListener("click", () => action(async c => {
      const sent = { ...hooks.files() };
      await c.create(newID(), name(), sent);
      if (c !== client) return;
      await hooks.saved?.(sent, c.draft.files);
      serverResult = null; $("draft-server-result").textContent = "";
      message(`Saved ${c.draft.name} · ${short(c.draft.revision)}. Newer local edits still need saving.`);
      await history(c); await list(c);
    }));
    $("draft-save").addEventListener("click", () => action(async c => {
      const sent = { ...hooks.files() };
      await c.save(sent);
      if (c !== client) return;
      await hooks.saved?.(sent, c.draft.files);
      message(`Saved source version ${short(c.draft.revision)}. Saving does not run checks or activate production.`);
      await history(c);
    }));
    $("draft-refresh").addEventListener("click", () => action(c => list(c)));
    $("draft-open").addEventListener("click", () => {
      const id = $("draft-picker").value;
      if (!id || !canReplace()) return;
      const started = workingKey();
      action(async c => { const draft = await c.open(id); await replace(c, draft, started); if (c === client) await history(c); });
    });
    $("draft-branch").addEventListener("click", () => {
      const revision = $("draft-versions").value;
      if (!revision || !canReplace()) return;
      const started = workingKey();
      action(async c => {
        const draft = await c.fork(revision, newID(), name());
        await replace(c, draft, started);
        if (c === client) { await history(c); await list(c); }
      });
    });
    $("draft-versions").addEventListener("change", preview);
    $("draft-version-file").addEventListener("change", previewFile);
    $("draft-questions-refresh").addEventListener("click", () => action(c => questions(c)));
    $("draft-questions-all").addEventListener("change", () => action(c => questions(c)));
    try { $("draft-author").value = root.localStorage.getItem("park-local-author") || "playground"; } catch { /* optional */ }
    client.author = author();
    $("draft-author").addEventListener("input", () => { client.author = author(); try { root.localStorage.setItem("park-local-author", client.author); } catch { /* optional */ } });
    $("draft-run").addEventListener("click", () => action(async c => {
      if (c.dirty(hooks.files()) || hooks.pending?.()) return;
      const executedCall = hooks.call();
      const result = await c.operation({ kind: "run", call: executedCall });
      if (c !== client) return;
      serverResult = {draft:c.draft.id,revision:c.draft.revision,call:executedCall,text:result.output ? `Server result: ${root.ParkView.readable(result.output)} (${result.status})` : result.message || result.status};
      await history(c);
    }));
    client.request("storage").then(value => {
      $("draft-storage-info").textContent = value.storage === "sqlite" ? "SQLite storage: saved drafts, source history, and server questions survive restarts. Keep database backups too." : "Memory storage: server drafts and questions disappear when it stops. Download work you want to keep.";
    }, () => { $("draft-storage-info").textContent = "Storage status unavailable. Download work you want to keep."; });
    render();
    return {
      render,
      detach() {
        client = new Client(undefined,author()); versions = []; serverResult = null;
        $("draft-replace-ok").checked = false;
        $("draft-versions").replaceChildren(option("", "No saved versions"));
        preview(); message("Working in the browser. Create or open a server draft when ready.");
        $("draft-questions").replaceChildren(); $("draft-question-count").textContent = ""; $("draft-server-result").textContent = "";
        $("draft-created").textContent = ""; $("draft-activity").replaceChildren();
        render();
      },
    };
  }
  root.ParkDraftPanel = { initialize };
})(globalThis);
