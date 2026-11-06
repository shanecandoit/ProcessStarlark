// Command playground-build assembles the static browser playground in
// web/dist: the page and the bundled sample projects.
// web/build.sh builds the wasm binary and copies wasm_exec.js. Run from the repository root.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/shanecandoit/ProcessStarlark/internal/workspace"
)

// Sample projects shown in the playground's picker, in order.
var projects = []struct{ Name, Dir, Call string }{
	{"Shipping quotes", "examples/shipping", `Shipping.quote("DE", 800)`},
	{"Roman numerals", "fixtures/roman", `RomanParse.parse("xiv")`},
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
		files, err := readProject(p.Dir)
		if err != nil {
			return err
		}
		samples = append(samples, sample{Name: p.Name, Call: p.Call, Files: files})
	}
	b, err := json.Marshal(samples)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dist, "samples.js"), []byte("globalThis.PARK_SAMPLES = "+string(b)+";\n"), 0644); err != nil {
		return err
	}
	if err := buildTours(dist); err != nil {
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

func readProject(dir string) (map[string]string, error) {
	files := map[string]string{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".star") {
			b, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				return nil, err
			}
			files[e.Name()] = string(b)
		}
	}
	return files, nil
}

// buildTours replays every tour in web/tours through the engine, so a tour
// that can no longer be completed fails the build, and writes them with their
// checkpoints to tours.js.
func buildTours(dist string) error {
	paths, err := filepath.Glob(filepath.Join("web", "tours", "*.json"))
	if err != nil {
		return err
	}
	tours := []workspace.Tour{}
	for _, path := range paths {
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var t workspace.Tour
		if err := json.Unmarshal(b, &t); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		files, err := readProject(t.Start)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		t.Files = files
		if err := workspace.Prepare(context.Background(), &t); err != nil {
			return err
		}
		tours = append(tours, t)
	}
	b, err := json.Marshal(tours)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dist, "tours.js"), []byte("globalThis.PARK_TOURS = "+string(b)+";\n"), 0644)
}
