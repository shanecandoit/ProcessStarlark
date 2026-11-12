const { test } = require("node:test");
const assert = require("node:assert/strict");
const { Client } = require("./draft-client.js");

function fixture(responses) {
  const requests = [];
  const client = new Client(async (path, options) => {
    requests.push({ path, body: options.body && JSON.parse(options.body) });
    const value = responses.shift();
    if (value instanceof Error) throw value;
    return { ok: !value.status || value.status < 400, status: value.status || 200, json: async () => value.value };
  });
  return { client, requests };
}
const draft = (id = "demo", token = "state-before") => ({ id, state_token: token, revision: "head", files: { "old.star": "old", "questions.txt": "queued" } });

test("explicit save sends hash precondition and atomically replaces source files", async () => {
  const after = { ...draft(), state_token: "after", revision: "child", files: { "new.star": "new" } };
  const { client, requests } = fixture([{ value: { draft: after } }]);
  client.draft = draft();
  const files = { "new.star": "new", "questions.txt": "browser queue" };
  const before = structuredClone(files);
  await client.save(files);
  assert.deepEqual(requests[0].body, {
    author: "playground", expected_state: "state-before",
    operation: { kind: "files", files: { "new.star": "new" }, delete: ["old.star"] },
  });
  assert.equal(client.draft.revision, "child");
  assert.deepEqual(files, before);
  assert.equal(client.dirty(files), false);
  assert.equal(client.dirty({ ...files, "new.star": "typed while saving" }), true);
});

test("conflicts and network errors preserve the original token and local source", async () => {
  const remote = draft("demo", "newer-state");
  const { client } = fixture([{ status: 409, value: { error: "stale", draft: remote } }, new Error("offline")]);
  client.draft = draft();
  const files = { "new.star": "local work" };
  await assert.rejects(client.save(files), e => e.status === 409 && e.remote === remote);
  assert.equal(client.draft.state_token, "state-before");
  await assert.rejects(client.save(files), /offline/);
  assert.deepEqual(files, { "new.star": "local work" });
  assert.equal(client.draft.state_token, "state-before");
});

test("no-op save does not create a version or count generated files", async () => {
  const { client, requests } = fixture([]);
  client.draft = draft();
  await client.save({ "questions.txt": "different queue", "old.star": "old" });
  assert.equal(requests.length, 0);
});

test("historical edit forks the selected immutable version rather than editing it", async () => {
  const branch = draft("branch");
  const { client, requests } = fixture([{ value: { draft: branch } }]);
  client.draft = draft();
  await client.fork("parent-hash", "branch", "Try earlier rule");
  assert.equal(requests[0].path, "/api/v1/drafts/demo/fork");
  assert.equal(requests[0].body.from_revision, "parent-hash");
  assert.equal(client.draft.id, "branch");
});

test("failed initial source save does not attach an unrelated server reference", async () => {
  const { client } = fixture([
    { value: { draft: draft("candidate") } },
    { status: 422, value: { result: { message: "invalid source" }, draft: draft("candidate") } },
  ]);
  client.draft = draft("original");
  await assert.rejects(client.create("candidate", "New", { "bad.star": "invalid" }), /invalid source/);
  assert.equal(client.draft.id, "original");
});
