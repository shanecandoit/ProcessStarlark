// Explicit server saves. Evaluation and unsaved edits remain in the playground.
(function (root) {
  "use strict";
  const source = files => Object.fromEntries(Object.entries(files).filter(([name]) => name.endsWith(".star")));
  const key = files => JSON.stringify(Object.entries(source(files)).sort(([a], [b]) => a.localeCompare(b)));
  function acceptSavedSource(submitted, current, remote) {
    const next = { ...current };
    for (const [name, text] of Object.entries(source(submitted))) {
      if (current[name] !== text) continue;
      if (name in remote) next[name] = remote[name]; else delete next[name];
    }
    return next;
  }
  class Client {
    constructor(fetcher = root.fetch.bind(root), author = "playground") { this.fetch = fetcher; this.author = author; this.draft = null; }
    async request(path, body) {
      const response = await this.fetch("/api/v1/" + path, body === undefined ? {} : {
        method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body),
      });
      const value = await response.json();
      if (!response.ok) {
        const error = new Error(value.error || value.result?.message || value.run?.message || `Request failed (${response.status})`);
        error.status = response.status; error.remote = value.draft;
        throw error;
      }
      return value;
    }
    async open(id) {
      const value = await this.request(`drafts/${encodeURIComponent(id)}?history=true`);
      this.draft = value.draft;
      return this.draft;
    }
    async create(id, name, files) {
      const value = await this.request("drafts", { id, name, author: this.author });
      const candidate = new Client(this.fetch, this.author);
      candidate.draft = value.draft;
      await candidate.save(files);
      this.draft = candidate.draft;
      return this.draft;
    }
    async save(files) {
      if (!this.draft) throw new Error("Create or open a draft first.");
      const target = source(files);
      if (key(target) === key(this.draft.files)) return this.draft;
      const value = await this.request(`drafts/${encodeURIComponent(this.draft.id)}/operations`, {
        author: this.author, expected_state: this.draft.state_token,
        operation: { kind: "files", files: target, delete: Object.keys(source(this.draft.files)).filter(name => !(name in target)) },
      });
      this.draft = value.draft;
      return this.draft;
    }
    async versions() {
      if (!this.draft) return [];
      return (await this.request(`drafts/${encodeURIComponent(this.draft.id)}/versions`)).versions;
    }
    async operation(operation) {
      if (!this.draft) throw new Error("Create or open a draft first.");
      const value = await this.request(`drafts/${encodeURIComponent(this.draft.id)}/operations`, {
        author: this.author, expected_state: this.draft.state_token, operation,
      });
      this.draft = value.draft;
      return value.result;
    }
    async questions(all = false) {
      if (!this.draft) return [];
      return (await this.request(`drafts/${encodeURIComponent(this.draft.id)}/questions${all ? "?status=all" : ""}`)).questions;
    }
    async fork(revision, id, name) {
      if (!this.draft) throw new Error("Open a draft first.");
      const value = await this.request(`drafts/${encodeURIComponent(this.draft.id)}/fork`, {
        id, name, author: this.author, from_revision: revision,
      });
      this.draft = value.draft;
      return this.draft;
    }
    production() { return this.request("production"); }
    reviews() { return this.request("reviews"); }
    releases() { return this.request("releases"); }
    requestReview(generation) {
      if (!this.draft) throw new Error("Open a saved draft first.");
      return this.request(`drafts/${encodeURIComponent(this.draft.id)}/reviews`, { author:this.author, expected_state:this.draft.state_token, expected_generation:generation });
    }
    decideReview(id, decision, generation, reason) {
      if (!["approve", "reject"].includes(decision)) throw new Error("Choose approve or reject.");
      return this.request(`reviews/${encodeURIComponent(id)}/${decision}`, { author:this.author, expected_generation:generation, reason });
    }
    runLive(call) { return this.request("production/run", { call }); }
    rollback(id, generation, reason) {
      return this.request("production/rollback", { author:this.author, release_id:id, expected_generation:generation, reason });
    }
    compare(generation, scenarios) {
      if (!this.draft) throw new Error("Open a saved draft first.");
      return this.request(`drafts/${encodeURIComponent(this.draft.id)}/compare`, { draft_revision:this.draft.revision, expected_generation:generation, scenarios });
    }
    acceptMetadata(remote, expectedState) {
      if (!this.draft || remote.id!==this.draft.id || this.draft.state_token!==expectedState || key(remote.files)!==key(this.draft.files)) return false;
      this.draft=remote;
      return true;
    }
    dirty(files) { return !this.draft || key(files) !== key(this.draft.files); }
  }
  const api = { Client, source, key, acceptSavedSource };
  root.ParkDrafts = api;
  if (typeof module !== "undefined" && module.exports) module.exports = api;
})(globalThis);
