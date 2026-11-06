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

	"park/internal/workspace"
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

func main() {
	ctx := context.Background()
	api := js.Global().Get("Object").New()
	api.Set("load", bridge(func(f workspace.Files, _ []string) workspace.Result { return workspace.Load(f) }))
	api.Set("run", bridge(func(f workspace.Files, a []string) workspace.Result { return workspace.Run(ctx, f, arg(a, 0)) }))
	api.Set("test", bridge(func(f workspace.Files, a []string) workspace.Result { return workspace.Test(ctx, f, arg(a, 0)) }))
	api.Set("addExample", bridge(func(f workspace.Files, a []string) workspace.Result { return workspace.AddExample(f, arg(a, 0)) }))
	api.Set("removeExample", bridge(func(f workspace.Files, a []string) workspace.Result { return workspace.RemoveExample(f, arg(a, 0)) }))
	api.Set("archive", bridge(func(f workspace.Files, _ []string) workspace.Result { return workspace.Archive(f) }))
	js.Global().Set("park", api)
	if ready := js.Global().Get("onParkReady"); ready.Type() == js.TypeFunction {
		ready.Invoke()
	}
	select {}
}
