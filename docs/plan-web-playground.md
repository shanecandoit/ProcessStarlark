# Plan: browser playground

Status: phases 1 and 2 done, phase 3 started (Why view), 2026-11-06. Direction:
[ideas-direction.md](ideas-direction.md).

Run it locally from the repository root:

```sh
sh serve.sh        # web/build.sh, then a static server on http://localhost:8000
```

`web/build.sh` alone writes `web/dist/`; any static server can host that
folder.

## Goal

A static web page where someone can try Park with nothing installed:

- edit `.star` files;
- run a call and see its answer, or the question Park asks;
- answer a question inline and see the call succeed;
- confirm proposed answers from unconfirmed code;
- run `park test`;
- see the catalog (`park.html`) update live.

It's a static site: no server, no accounts. The only network requests after
the page's own files are for the CodeMirror editor, from cdnjs; if they fail,
the page falls back to a plain text box. Hosting is any static host (GitHub
Pages, or a claude.ai artifact).

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
 page (app.js)                  worker.js                 Go (park.wasm)
 ─────────────                  ─────────                 ──────────────
 files: {name: text}  ─ msg ─▶  park[op](JSON files) ─▶  park.load(files)
 editor, call box                                         park.run(files, call)
 questions panel      ◀─ msg ─  JSON result        ◀─    park.test(files, only)
 function tables                                          park.addExample(files, line)
 catalog iframe                                           park.removeExample(files, call)
                                                          park.archive(files)
```

Every operation returns a `workspace.Result`: a status (`ok`, `unconfirmed`,
`unresolved`, `failed`, `error`), the output or message, the new file map,
and a snapshot (processes, functions, examples as literals, open questions,
and the rendered `park.html`). A run also returns `why`, its provenance
tree (D20).

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
- `web/build.sh` builds `web/dist/`: the page, `park.wasm`, Go's
  `wasm_exec.js`, and `samples.js` (written by `cmd/playground-build`).

### Phase 2: worth sharing (done)

- **Share links.** *Share* copies a link whose fragment is
  `#p=` + base64url(deflate-raw(JSON `{v: 1, files, call, active}`)). Fragments
  never reach a server. The shipping sample makes a link of about 700
  characters. Opening a link loads the project as "Shared project", saves it
  locally, and clears the fragment.
- **Persistence.** The project, the call box, and the open file are saved to
  `localStorage` (`park-playground-v1`) on every change, and restored on
  reload. *Reset* restores the sample's original files, after a confirmation.
  A shared project has no sample to reset to; pick one from the list instead.
- **Off the main thread.** Go runs in `worker.js`. Operations are queued and
  run one at a time, each on the files as they are when it starts; text typed
  while an operation runs is kept. If an operation takes longer than 0.8 s, a
  *Stop* button appears; it terminates the worker and starts a fresh one.
- **Download.** *Download* saves `park-project.zip`: the modules,
  `questions.txt`, and a freshly rendered `park.html` in a `park-project/`
  folder. `workspace.Archive` builds it in Go, and a test checks that the CLI
  opens it and that `park build` reproduces its `park.html` byte for byte.
- **Editor.** CodeMirror 5 with Python highlighting, line numbers, one
  undo history per file, and colours that follow light and dark mode. An
  error that names `file.star:line` marks that line, and clicking the error
  opens it. Without CodeMirror, the plain text box still works.
- **Size.** 9.4 MB stripped (`-ldflags="-s -w"`), 2.4 MB gzipped. Good enough;
  TinyGo isn't worth the risk to starlark-go compatibility yet.

### Phase 3: shows off the idea

- **Why view: done.** Every run shows the `--why` tree (D20) under the
  answer, with sources coloured and each location opening the file at that
  line. `workspace.Run` always records it and returns it as `why`.
- **Fill-and-resume:** answer the innermost question in place and the
  original call reruns automatically (ideas #5). A first cut exists: answering
  any question reruns the last call. Next: show the chain of questions one
  call raises, and answer them in order.
- **Guided tour:** the README's Roman numeral story as steps a reader clicks
  through.
- **Function tables:** done early, in phase 1. Each function is a table with
  inputs as columns and the output last; rows added by an answer are
  highlighted, and each row can be removed.
- **Hosting:** publish `web/dist` (GitHub Pages or an artifact) and link it
  from the README.

## Non-goals

No server-side execution, no accounts, no collaboration, no synthesis or
model calls. The playground stays a static page.

## Risks

- **Bundle size.** 9.4 MB stripped, 2.4 MB gzipped. Acceptable for a demo;
  the host must serve it compressed.
- **Long calls.** Solved by the worker and *Stop*. Inside the worker, the
  engine's five-second deadline may not fire during a tight Starlark loop
  (wasm runs one goroutine at a time), so the step limit and *Stop* are what
  actually bound a call.
- **Share link length.** Links grow with the project. Browsers accept
  very long fragments, but chat apps and email may truncate them. Download is
  the fallback for big projects.
- **Semantic drift.** Avoided by construction: the playground calls the same
  packages as the CLI, and `internal/workspace` tests check that the CLI and
  the workspace write identical `questions.txt` and `park.html` for the same
  commands.
