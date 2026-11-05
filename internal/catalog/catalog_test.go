package catalog

import (
	"reflect"
	"strings"
	"testing"

	"park/internal/project"
	"park/internal/samples"
)

func TestSelectionAndEscaping(t *testing.T) {
	cases := [][]int{{}, {0}, {0, 1}, {0, 1, 2}, {0, 1, 2, 3}, {0, 1, 3, 4}, {0, 1, 4, 5}}
	for n, want := range cases {
		if got := Selection(n); !reflect.DeepEqual(got, want) {
			t.Errorf("%d: got %v want %v", n, got, want)
		}
	}
	f := &project.Function{Name: "f", Signature: "f(in) out", InputLabel: "in", OutputLabel: "out", Description: "<script>alert(1)</script>", Examples: []samples.Pair{{Input: "<b>", Output: "</b>"}}}
	p := &project.Project{Revision: "revision", Processes: map[string]*project.Process{"P": {Name: "P", Functions: map[string]*project.Function{"f": f}}}}
	html, err := Render(p)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(html), "<script>") || !strings.Contains(string(html), "&lt;script&gt;") {
		t.Fatal("authored content was not escaped")
	}
	if !strings.Contains(string(html), "in:") || !strings.Contains(string(html), "out:") {
		t.Fatal("example labels missing")
	}
}
