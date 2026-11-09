//go:build js && wasm

// Command park-wasm exposes the workspace operations to JavaScript as
// globalThis.park. Each function takes the project's files as a JSON object
// string ({"name.star": "source", ...}) plus string arguments, and returns a
// workspace.Result as a JSON string. Go keeps no state between calls.
package main

import (
	"context"
	"encoding/json"
	"syscall/js"

	"github.com/shanecandoit/ProcessStarlark/internal/workspace"
)

type op func(files workspace.Files, args []string) workspace.Result

func bridge(f op) js.Func {
	return js.FuncOf(func(_ js.Value, args []js.Value) any {
		var files workspace.Files
		var result workspace.Result
		if len(args) == 0 {
			result = workspace.Result{Status: workspace.Error, Message: "missing files"}
		} else if err := json.Unmarshal([]byte(args[0].String()), &files); err != nil {
			result = workspace.Result{Status: workspace.Error, Message: "files: " + err.Error()}
		} else {
			rest := make([]string, len(args)-1)
			for i, a := range args[1:] {
				rest[i] = a.String()
			}
			result = f(files, rest)
		}
		b, err := json.Marshal(result)
		if err != nil {
			return `{"status":"error","message":"encoding result failed"}`
		}
		return string(b)
	})
}

func arg(args []string, i int) string {
	if i < len(args) {
		return args[i]
	}
	return ""
}

// goal(files, lastRun, goal) reports whether a tour step's goal holds, as
// the JSON string {"met": bool}.
func goal(_ js.Value, args []js.Value) any {
	var files workspace.Files
	var last workspace.LastRun
	var g workspace.Goal
	if len(args) < 3 ||
		json.Unmarshal([]byte(args[0].String()), &files) != nil ||
		json.Unmarshal([]byte(args[1].String()), &last) != nil ||
		json.Unmarshal([]byte(args[2].String()), &g) != nil {
		return `{"met":false,"message":"goal: bad arguments"}`
	}
	if workspace.Met(files, last, g) {
		return `{"met":true}`
	}
	return `{"met":false}`
}

func main() {
	ctx := context.Background()
	api := js.Global().Get("Object").New()
	api.Set("load", bridge(func(f workspace.Files, _ []string) workspace.Result { return workspace.Load(f) }))
	api.Set("run", bridge(func(f workspace.Files, a []string) workspace.Result { return workspace.Run(ctx, f, arg(a, 0)) }))
	api.Set("test", bridge(func(f workspace.Files, a []string) workspace.Result { return workspace.Test(ctx, f, arg(a, 0)) }))
	api.Set("addExample", bridge(func(f workspace.Files, a []string) workspace.Result { return workspace.AddExample(f, arg(a, 0)) }))
	api.Set("removeExample", bridge(func(f workspace.Files, a []string) workspace.Result { return workspace.RemoveExample(f, arg(a, 0)) }))
	api.Set("dismiss", bridge(func(f workspace.Files, a []string) workspace.Result { return workspace.Dismiss(f, arg(a, 0)) }))
	api.Set("goal", js.FuncOf(goal))
	api.Set("putExample", bridge(func(f workspace.Files, a []string) workspace.Result {
		var cells []string
		if err := json.Unmarshal([]byte(arg(a, 1)), &cells); err != nil {
			return workspace.Result{Status: workspace.Error, Message: "cells: " + err.Error(), Files: f}
		}
		return workspace.PutExample(ctx, f, arg(a, 0), cells, arg(a, 2))
	}))
	api.Set("pasteExamples", bridge(func(f workspace.Files, a []string) workspace.Result {
		return workspace.PasteExamples(ctx, f, arg(a, 0), arg(a, 1))
	}))
	// importRows(files, fn, text, mode): mode "propose" queues the rows as
	// proposed answers; anything else adds them as examples.
	api.Set("importRows", bridge(func(f workspace.Files, a []string) workspace.Result {
		return workspace.ImportRows(ctx, f, arg(a, 0), arg(a, 1), arg(a, 2) == "propose")
	}))
	api.Set("archive", bridge(func(f workspace.Files, _ []string) workspace.Result { return workspace.Archive(f) }))
	js.Global().Set("park", api)
	if ready := js.Global().Get("onParkReady"); ready.Type() == js.TypeFunction {
		ready.Invoke()
	}
	select {}
}
