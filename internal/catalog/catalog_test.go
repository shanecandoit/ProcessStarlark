package catalog

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"park/internal/project"
)

func TestSelectionAndEscaping(t *testing.T) {
	cases := [][]int{{}, {0}, {0, 1}, {0, 1, 2}, {0, 1, 2, 3}, {0, 1, 3, 4}, {0, 1, 4, 5}}
	for n, want := range cases {
		if got := Selection(n); !reflect.DeepEqual(got, want) {
			t.Errorf("%d: got %v want %v", n, got, want)
		}
	}
	f := &project.Function{Name: "f", Signature: "f(tag, attribute) markup", Inputs: []string{"tag", "attribute"}, Output: "markup", Description: "<script>alert(1)</script>", Examples: []project.Example{{Input: map[string]any{"tag": "<b>", "attribute": "x"}, Output: "</b>"}}}
	p := &project.Project{Revision: "revision", Processes: map[string]*project.Process{"P": {Name: "P", Functions: map[string]*project.Function{"f": f}}}}
	html, err := Render(p, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(html), "<script>") || !strings.Contains(string(html), "&lt;script&gt;") {
		t.Fatal("authored content was not escaped")
	}
	if !strings.Contains(string(html), "tag: &#34;&lt;b&gt;&#34;\nattribute: &#34;x&#34;\nmarkup:") {
		t.Fatal("example inputs missing or not in signature order:\n" + string(html))
	}
	if strings.Contains(string(html), "Show code") {
		t.Fatal("example-only function shows a code section")
	}
}

func TestCodeIsShownCollapsedAndEscaped(t *testing.T) {
	root := t.TempDir()
	src := "Process P:\n    def f(x):\n        \"\"\"Doc.\"\"\"\n        y = \"<b>\" + x\n        return y\n\n    example: f(\"a\") == \"<b>a\"\n"
	if err := os.WriteFile(filepath.Join(root, "p.star"), []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	p, err := project.LoadWith(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	html, err := Render(p, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := "<details><summary>Show code</summary><pre>def f(x):\n    &#34;&#34;&#34;Doc.&#34;&#34;&#34;\n    y = &#34;&lt;b&gt;&#34; &#43; x\n    return y</pre></details>"
	if !strings.Contains(string(html), want) {
		t.Fatal("code section missing, indented, or unescaped:\n" + string(html))
	}
}
