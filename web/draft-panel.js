(function (root) {
  "use strict";
  function initialize(hooks) {
    const $ = id => document.getElementById(id);
    const { Client, key } = root.ParkDrafts;
    let client = new Client(), busy = false, versions = [];
    const buttons = ["draft-create", "draft-save", "draft-open", "draft-refresh", "draft-branch"];
    const message = text => { $("draft-message").textContent = text; };
    const short = id => id.slice(0, 12);
    const newID = () => "web-" + root.crypto.randomUUID();
    const name = () => $("draft-name").value.trim() || "My experiment";
    const option = (value, text) => new Option(text, value);
    const workingKey = () => hooks.workingKey?.() ?? key(hooks.files());
    const pending = () => Boolean(hooks.pending?.());
    function render() {
      if (!root.PARK_DRAFT_API) return;
      const draft = client.draft;
      $("draft-save-state").textContent = pending() ? "· finish table edits" : !draft ? "· browser only" : client.dirty(hooks.files()) ? "· local changes" : "· source saved";
      for (const id of buttons) $(id).disabled = busy;
      $("draft-save").disabled = busy || pending() || !draft || !client.dirty(hooks.files());
      $("draft-create").disabled = busy || pending();
      $("draft-export").hidden = !draft;
      if (draft) $("draft-export").href = `/api/v1/drafts/${encodeURIComponent(draft.id)}/export`;
      $("draft-branch").disabled = busy || !versions.length;
      $("draft-replace-warning").hidden = !pending() && !client.dirty(hooks.files());
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
    }
    async function list(c) {
      const value = await c.request("drafts");
      if (c !== client) return;
      const selected = c.draft?.id || $("draft-picker").value;
      $("draft-picker").replaceChildren(option("", "Choose a draft"), ...value.drafts.map(d => option(d.id, `${d.name} · ${d.id}`)));
      $("draft-picker").value = selected;
    }
    async function action(work) {
      if (busy) return;
      busy = true; render();
      const c = client;
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
      $("draft-replace-ok").checked = false;
      message(`Opened ${draft.name}. Source version ${short(draft.revision)}. Editing creates a new version when you save.`);
    }
    $("draft-panel").hidden = !root.PARK_DRAFT_API;
    $("draft-panel").addEventListener("toggle", () => { if ($("draft-panel").open) action(c => list(c)); });
    $("draft-create").addEventListener("click", () => action(async c => {
      await c.create(newID(), name(), { ...hooks.files() });
      if (c !== client) return;
      message(`Saved ${c.draft.name} · ${short(c.draft.revision)}. Newer local edits still need saving.`);
      await history(c); await list(c);
    }));
    $("draft-save").addEventListener("click", () => action(async c => {
      await c.save({ ...hooks.files() });
      if (c !== client) return;
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
    render();
    return {
      render,
      detach() {
        client = new Client(); versions = [];
        $("draft-replace-ok").checked = false;
        $("draft-versions").replaceChildren(option("", "No saved versions"));
        preview(); message("Working in the browser. Create or open a server draft when ready."); render();
      },
    };
  }
  root.ParkDraftPanel = { initialize };
})(globalThis);
