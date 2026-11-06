// Command playground-build assembles the static browser playground in
// web/dist: the page and the bundled sample projects.
// web/build.sh builds the wasm binary and copies wasm_exec.js. Run from the repository root.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Sample projects shown in the playground's picker, in order.
var projects = []struct{ Name, Dir, Call string }{
	{"Roman numerals", "fixtures/roman", `RomanParse.parse("xiv")`},
	{"Shipping quotes", "examples/shipping", `Shipping.quote("DE", 800)`},
	{"Robot on a grid", "examples/robot", `Robot.walk([0, 0], ["right", "down"])`},
	{"Shapes", "examples/shapes", `Shapes.polygon(6)`},
}

type sample struct {
	Name  string            `json:"name"`
	Call  string            `json:"call"`
	Files map[string]string `json:"files"`
}

func main() {
	if err := build(); err != nil {
		fmt.Fprintln(os.Stderr, "playground-build:", err)
		os.Exit(1)
	}
}

func build() error {
	dist := filepath.Join("web", "dist")
	if err := os.MkdirAll(dist, 0755); err != nil {
		return err
	}
	var samples []sample
	for _, p := range projects {
		s := sample{Name: p.Name, Call: p.Call, Files: map[string]string{}}
		entries, err := os.ReadDir(p.Dir)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".star") {
				b, err := os.ReadFile(filepath.Join(p.Dir, e.Name()))
				if err != nil {
					return err
				}
				s.Files[e.Name()] = string(b)
			}
		}
		samples = append(samples, s)
	}
	b, err := json.Marshal(samples)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dist, "samples.js"), []byte("globalThis.PARK_SAMPLES = "+string(b)+";\n"), 0644); err != nil {
		return err
	}
	copies := map[string]string{
		filepath.Join("web", "index.html"): "index.html",
		filepath.Join("web", "app.js"):     "app.js",
		filepath.Join("web", "style.css"):  "style.css",
		filepath.Join("web", "worker.js"):  "worker.js",
	}
	for from, to := range copies {
		b, err := os.ReadFile(from)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dist, to), b, 0644); err != nil {
			return err
		}
	}
	return nil
}
