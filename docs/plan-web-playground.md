# Plan: browser playground

Status: phase 1 done, 2026-11-06. Build with `web/build.sh`, then serve
`web/dist` with any static server, e.g. `python3 -m http.server -d web/dist 8000`. Direction:
[ideas-direction.md](ideas-direction.md).

## Goal

A static web page where someone can try Park with nothing installed:

- edit `.star` files;
- run a call and see its answer, or the question Park asks;
- answer a question inline and see the call succeed;
- confirm proposed answers from unconfirmed code;
- run `park test`;
- see the catalog (`park.html`) update live.

It's a static site: no server, no accounts, no network after load. Hosting is
any static host (GitHub Pages, or a claude.ai artifact).

## Approach: the real engine, compiled to WebAssembly

Compile the existing Go packages to `GOOS=js GOARCH=wasm` so the playground
runs exactly the same semantics as the CLI. Writing a second implementation in
JavaScript would drift. `go.starlark.net` and every `internal/` package already
build for wasm.

The browser has no filesystem, so the project lives in memory as a map from
file name to contents, the same files a project directory holds
(`*.star`, `questions.txt`). JavaScript owns that map. Each call into Go
receives the files and returns results plus any edited files. Go keeps no
state between calls, which keeps the bridge small and easy to test.

```text
 browser (JS)                          Go (wasm)
 ─────────────                         ─────────
 files: {name: text}  ── JSON ──▶  park.snapshot(files)
 editor, call box                  park.run(files, call)
 questions panel      ◀── JSON ──  park.test(files)
 catalog iframe                    park.addExample(files, line)
                                   park.removeExample(files, call)
```

## Seams needed in the Go code

The engine is already pure. Three places touch the disk:

| Where | Today | Change |
|---|---|---|
| `project.LoadWith` | Reads `*.star` from the root directory | Split out `project.FromFiles(root, files)`, which validates an in-memory snapshot; `LoadWith` reads the disk then calls it |
| `engine.dispatch` result cache | `.cache/` under the project root | `engine.Cache` interface; the disk cache is the CLI default, and the playground runs with none |
| `project.LoadQuestions` | Reads `questions.txt` | Split out `project.ParseQuestions(p, bytes)` |

`park test`'s loop moves from `cli` into `engine.Check` so the CLI and the
playground share it. The CLI's output stays byte-for-byte the same.

A new package, `internal/workspace`, holds the playground operations as plain
Go over an in-memory file map, with JSON-friendly results. It has native
tests. `cmd/park-wasm` is a thin `syscall/js` bridge over it.

## Phases

### Phase 1: it runs (done)

- Seams above, with existing tests passing unchanged.
- `internal/workspace`: `Snapshot`, `Run`, `Test`, `AddExample`,
  `RemoveExample`, each mirroring the CLI's publish rules: validate the
  proposed project, merge and prune the question queue, and render the catalog.
- `cmd/park-wasm` bridge.
- `web/`: a single page with a file list, a text editor (plain `<textarea>`),
  a call box, an output pane, a questions panel with inline answers and
  confirm buttons, a test button, and the rendered catalog in an iframe.
- Bundled examples (Roman numerals, robot, shipping, shapes) in a picker.
- Answering a question reruns the last call, a first cut of fill-and-resume.
- `web/build.sh` builds `web/dist/` (`index.html`, `park.wasm`,
  `wasm_exec.js`). Serve locally with any static server.

### Phase 2: worth sharing

- **Share links:** the file map, compressed, in the URL fragment. No server.
- **Persistence:** the last project in `localStorage`, with a reset button.
- **Run off the main thread:** move Go into a Web Worker so a long call can't
  freeze the page. The step limit already bounds work.
- **Download:** the project as a zip that the CLI can open.
- **Editor:** CodeMirror with Python highlighting and error markers at the
  reported line.
- **Size:** measure the wasm bundle. Try `-ldflags="-s -w"` and gzip/brotli
  first. TinyGo only if needed; it may not support everything starlark-go
  uses.

### Phase 3: shows off the idea

- **Why view:** the `--why` provenance tree (ideas #1), drawn as a call tree
  with example and code nodes coloured differently.
- **Fill-and-resume:** answer the innermost question in place and the
  original call reruns automatically (ideas #5).
- **Guided tour:** the README's Roman numeral story as steps a reader clicks
  through.
- **Function tables:** each function shown as a table (inputs as columns,
  output last), with rows added from answered questions highlighted.

## Non-goals

No server-side execution, no accounts, no collaboration, no synthesis or
model calls. The playground stays a static page.

## Risks

- **Bundle size.** Measured in phase 1: 9.0 MB stripped, 2.3 MB gzipped.
  Acceptable for a demo; the host must serve it compressed.
- **Main-thread blocking.** A call can use up to a million Starlark steps.
  Acceptable for phase 1; fixed by the worker in phase 2.
- **Semantic drift.** Avoided by construction: the playground calls the same
  packages as the CLI, and `internal/workspace` tests check that the CLI and
  the workspace write identical `questions.txt` and `park.html` for the same
  commands.
