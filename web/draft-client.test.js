const { test } = require("node:test");
const assert = require("node:assert/strict");
const { Client, acceptSavedSource } = require("./draft-client.js");

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

test("normalized saves converge without overwriting source typed during a request or browser questions", () => {
  const submitted = { "a.star": "a\n\n\n", "b.star": "b\n\n\n", "questions.txt": "local queue" };
  const current = { ...submitted, "b.star": "newer typing", "new.star": "new file" };
  const remote = { "a.star": "a\n", "b.star": "b\n", "questions.txt": "server queue" };
  assert.deepEqual(acceptSavedSource(submitted, current, remote), {
    "a.star": "a\n", "b.star": "newer typing", "new.star": "new file", "questions.txt": "local queue",
  });
  assert.equal(current["a.star"], "a\n\n\n");
});

test("saved-source question flow advances hash tokens and keeps local actor attribution", async () => {
  const initial = draft();
  const queued = { ...initial, state_token: "queued", snapshot: { questions: [{ call: 'Words.value("twelve")' }] } };
  const answered = { ...queued, state_token: "answered", revision: "child" };
  const { client, requests } = fixture([
    { value: { draft: queued, result: { status: "unresolved" } } },
    { value: { questions: [{ id: "q", status: "open" }] } },
    { value: { draft: answered, result: { status: "ok" } } },
    { value: { questions: [{ id: "q", status: "resolved" }] } },
  ]);
  client.author = "Shane"; client.draft = initial;
  assert.equal((await client.operation({ kind: "run", call: 'Words.value("twelve")' })).status, "unresolved");
  assert.equal(client.draft.revision, initial.revision);
  assert.equal((await client.questions())[0].status, "open");
  await client.operation({ kind: "add-example", example: 'Words.value("twelve") == 12' });
  assert.equal((await client.questions(true))[0].status, "resolved");
  assert.equal(requests[0].body.author, "Shane");
  assert.equal(requests[2].body.expected_state, "queued");
  assert.equal(requests[3].path, "/api/v1/drafts/demo/questions?status=all");
});

test("reviews and comparisons send captured pins without changing browser draft state",async()=>{
 const {client,requests}=fixture([{value:{review:{id:"review"}}},{value:{comparison:{id:"report"}}},{status:409,value:{error:"production base changed"}}]);
 client.draft=draft();client.author="developer";
 await client.requestReview(2);
 await client.compare(2,[{name:"Parcel",call:'Shipping.quote("DE", 800)'}]);
 await assert.rejects(client.decideReview("review","approve",2,"checked"),e=>e.status===409);
 assert.equal(requests[0].body.expected_state,"state-before");
 assert.equal(requests[1].body.draft_revision,"head");assert.equal(requests[1].body.expected_generation,2);
 assert.equal(client.draft.state_token,"state-before");assert.equal(client.draft.files["old.star"],"old");
});

test("metadata refresh advances activity tokens only when saved source and the original client state still match",()=>{
 const {client}=fixture([]);client.draft=draft();
 assert.equal(client.acceptMetadata({...draft(),state_token:"activity"},"state-before"),true);
 assert.equal(client.draft.state_token,"activity");
 assert.equal(client.acceptMetadata({...draft(),state_token:"old-response"},"state-before"),false);
 assert.equal(client.acceptMetadata({...draft(),state_token:"changed-source",files:{"old.star":"other source"}},"activity"),false);
 assert.equal(client.draft.state_token,"activity");
});
