// Runs the Park engine (park.wasm) off the page's main thread, so a long call
// never freezes the page. Messages in: {id, op, files, args}. Messages out:
// {ready: true}, {fatal: "..."}, or {id, result} where result is the JSON
// string the Go bridge returned. Calls run one at a time, in order.
"use strict";
importScripts("wasm_exec.js");

let markReady;
const ready = new Promise((resolve) => (markReady = resolve));
self.onParkReady = () => markReady();

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
    self.postMessage({ fatal: String(err) });
  }
})();

ready.then(() => self.postMessage({ ready: true }));

self.onmessage = async (event) => {
  await ready;
  const { id, op, files, args } = event.data;
  let result;
  try {
    result = self.park[op](JSON.stringify(files), ...args);
  } catch (err) {
    result = JSON.stringify({ status: "error", message: "engine: " + err, files });
  }
  self.postMessage({ id, result });
};
