// Explicit server saves. Evaluation and unsaved edits remain in the playground.
(function (root) {
  "use strict";
  const source = files => Object.fromEntries(Object.entries(files).filter(([name]) => name.endsWith(".star")));
  const key = files => JSON.stringify(Object.entries(source(files)).sort(([a], [b]) => a.localeCompare(b)));
  class Client {
    constructor(fetcher = root.fetch.bind(root)) { this.fetch = fetcher; this.draft = null; }
    async request(path, body) {
      const response = await this.fetch("/api/v1/" + path, body === undefined ? {} : {
        method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body),
      });
      const value = await response.json();
      if (!response.ok) {
        const error = new Error(value.error || value.result?.message || `Request failed (${response.status})`);
        error.status = response.status; error.remote = value.draft;
        throw error;
      }
      return value;
    }
    async open(id) {
      const value = await this.request(`drafts/${encodeURIComponent(id)}`);
      this.draft = value.draft;
      return this.draft;
    }
    async create(id, name, files) {
      const value = await this.request("drafts", { id, name, author: "playground" });
      const candidate = new Client(this.fetch);
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
        author: "playground", expected_state: this.draft.state_token,
        operation: { kind: "files", files: target, delete: Object.keys(source(this.draft.files)).filter(name => !(name in target)) },
      });
      this.draft = value.draft;
      return this.draft;
    }
    async versions() {
      if (!this.draft) return [];
      return (await this.request(`drafts/${encodeURIComponent(this.draft.id)}/versions`)).versions;
    }
    async fork(revision, id, name) {
      if (!this.draft) throw new Error("Open a draft first.");
      const value = await this.request(`drafts/${encodeURIComponent(this.draft.id)}/fork`, {
        id, name, author: "playground", from_revision: revision,
      });
      this.draft = value.draft;
      return this.draft;
    }
    dirty(files) { return !this.draft || key(files) !== key(this.draft.files); }
  }
  const api = { Client, source, key };
  root.ParkDrafts = api;
  if (typeof module !== "undefined" && module.exports) module.exports = api;
})(globalThis);
