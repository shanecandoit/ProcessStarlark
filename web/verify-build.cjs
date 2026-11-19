// Check the shipped assets and exercise the compiled Go bridge, without a server.
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const vm = require("node:vm");
const { execFileSync } = require("node:child_process");

const dist = path.join(__dirname, "dist");

function bundle(file, name) {
  const sandbox = vm.createContext({});
  vm.runInContext(fs.readFileSync(path.join(dist, file), "utf8"), sandbox, { filename: file, timeout: 1000 });
  const data = sandbox[name];
  assert.ok(Array.isArray(data) && data.length, `${file} has no ${name}`);
  return data;
}

async function verify() {
  const html = fs.readFileSync(path.join(dist, "index.html"), "utf8");
  for (const match of html.matchAll(/\b(?:src|href)=["']([^"']+)["']/g)) {
    const url = match[1];
    if (/^(?:[a-z]+:|\/\/|#)/i.test(url)) continue;
    const asset = path.resolve(dist, url.split(/[?#]/)[0]);
    assert.ok(asset.startsWith(dist + path.sep), `asset escapes dist: ${url}`);
    assert.ok(fs.statSync(asset).size > 0, `missing or empty asset: ${url}`);
  }
  for (const name of fs.readdirSync(dist).filter(name => name.endsWith(".js"))) {
    execFileSync(process.execPath, ["--check", path.join(dist, name)], { stdio: "inherit" });
  }
  for (const name of ["worker.js", "wasm_exec.js", "park.wasm"]) {
    assert.ok(fs.statSync(path.join(dist, name)).size > 0, `missing or empty ${name}`);
  }
  const samples = bundle("samples.js", "PARK_SAMPLES");
  const tours = bundle("tours.js", "PARK_TOURS");
  for (const tour of tours) assert.ok(tour.steps.length && Object.keys(tour.files).length, `empty tour ${tour.id}`);

  // A mismatch between park.wasm and wasm_exec.js fails at real startup.
  require(path.join(dist, "wasm_exec.js"));
  const ready = new Promise(resolve => { globalThis.onParkReady = resolve; });
  const go = new globalThis.Go();
  const { instance } = await WebAssembly.instantiate(fs.readFileSync(path.join(dist, "park.wasm")), go.importObject);
  go.run(instance).then(() => {
    console.error("WASM engine exited unexpectedly");
    process.exit(1);
  }, fail);
  await ready;
  function call(op, files, ...args) {
    const result = JSON.parse(globalThis.park[op](JSON.stringify(files), ...args));
    assert.notEqual(result.status, "error", `${op}: ${result.message}`);
    return result;
  }
  for (const sample of samples) {
    assert.equal(call("load", sample.files).status, "ok", sample.name);
    assert.equal(call("test", sample.files, "").status, "ok", `${sample.name} checks`);
    const result = call("run", sample.files, sample.call);
    assert.ok(["ok", "unresolved"].includes(result.status), `${sample.name}: ${result.status}`);
  }
  // Exercise the playground's missing-answer, save, and rerun path in WASM.
  const shipping = samples.find(sample => sample.name === "Shipping quotes");
  assert.ok(shipping, "shipping sample missing");
  assert.equal(call("run", shipping.files, 'Shipping.quote("DE", 800)').status, "unresolved");
  const saved = call("addExample", shipping.files, 'Zones.zone("DE") == "international"');
  assert.equal(saved.status, "ok");
  assert.equal(call("run", saved.files, 'Shipping.quote("DE", 800)').output, "2799");
  console.log(`Playground verified: assets, WASM startup, ${samples.length} samples, ${tours.length} tours, and answer/save/rerun.`);
}

function fail(error) {
  console.error(error);
  process.exit(1);
}

setTimeout(() => fail(new Error("Playground verification timed out")), 20000);
verify().then(() => process.exit(0), fail);
