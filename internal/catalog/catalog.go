// Package catalog renders a deterministic offline project catalog.
package catalog

import (
	"bytes"
	"fmt"
	"html/template"

	"github.com/shanecandoit/ProcessStarlark/internal/project"
	"github.com/shanecandoit/ProcessStarlark/internal/samples"
)

const MaxBytes = 32 << 20

type limitedBuffer struct{ bytes.Buffer }

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > MaxBytes {
		return 0, fmt.Errorf("catalog exceeds %d bytes", MaxBytes)
	}
	return b.Buffer.Write(p)
}

func (b *limitedBuffer) WriteString(s string) (int, error) { return b.Write([]byte(s)) }

type Field struct{ Name, Value string }
type Row struct {
	Inputs []Field
	Output string
}
type Function struct {
	Anchor, Name, Signature, Description, State, Source, Output, Code string
	Count, Omitted                                                    int
	Rows                                                              []Row
}
type Process struct {
	Name, Description string
	Uses              []string
	Functions         []Function
}

// Question is an unanswered call, or one answered only by unconfirmed code.
type Question struct {
	Call, Proposed string // Proposed is empty when nothing answered the call.
}

type Page struct {
	Revision  string
	Questions []Question // In queue order.
	Processes []Process
}

// Selection returns list positions for the first two and last two, without overlap.
func Selection(count int) []int {
	out := []int{}
	for i := 0; i < count; i++ {
		if i < 2 || i >= count-2 {
			out = append(out, i)
		}
	}
	return out
}

// Render builds the catalog. Open questions are listed first, because each one
// waits for the user to add an example or an implementation.
func Render(p *project.Project, questions []project.Question) ([]byte, error) {
	page := Page{Revision: p.Revision}
	for _, q := range questions {
		view := Question{Call: q.Call}
		if q.HasProposal {
			view.Proposed = samples.Literal(q.Proposed)
		}
		page.Questions = append(page.Questions, view)
	}
	for _, name := range project.Names(p.Processes) {
		proc := p.Processes[name]
		view := Process{Name: name, Description: proc.Description, Uses: proc.Uses}
		for _, name := range project.Names(proc.Functions) {
			f := proc.Functions[name]
			item := Function{Anchor: proc.Name + "." + name, Name: name, Signature: f.Signature, Description: f.Description, State: f.State(), Count: len(f.Examples), Output: f.Output}
			if f.Implemented {
				item.Source = f.Path
				item.Code = p.Code(proc, f)
			}
			for _, i := range Selection(len(f.Examples)) {
				pair := f.Examples[i]
				row := Row{Output: samples.Literal(pair.Output)}
				for _, name := range f.Inputs {
					row.Inputs = append(row.Inputs, Field{Name: name, Value: samples.Literal(pair.Input[name])})
				}
				item.Rows = append(item.Rows, row)
			}
			item.Omitted = item.Count - len(item.Rows)
			view.Functions = append(view.Functions, item)
		}
		page.Processes = append(page.Processes, view)
	}
	var b limitedBuffer
	err := pageTemplate.Execute(&b, page)
	return b.Bytes(), err
}

var pageTemplate = template.Must(template.New("catalog").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Park catalog</title>
<style>body{font:16px/1.6 system-ui,sans-serif;max-width:960px;margin:40px auto;padding:0 24px;color:#25332c;background:#f7f8f4}a{color:#286444}code,pre{font:14px/1.5 ui-monospace,monospace}pre{white-space:pre-wrap;overflow-wrap:anywhere;background:#edf0e9;padding:12px;border-radius:6px}article{margin:24px 0;padding:16px 24px;background:white;border:1px solid #dce3d8;border-radius:10px}.meta{color:#596559}footer{margin:40px 0;font-size:12px;overflow-wrap:anywhere}h2{margin-top:40px}.questions{padding:16px 24px;background:#fff7e0;border:1px solid #e8d49a;border-radius:10px}.questions li{margin:8px 0}summary{cursor:pointer;color:#286444}</style></head>
<body><header><h1>Park</h1><p>Processes, functions, and authored input/output examples.</p></header>
{{if .Questions}}<section id="questions" class="questions"><h2>Questions for you</h2><p>Park needs you for these calls. Confirm answers proposed by code that has fewer than two examples, or answer calls nothing could answer with an example or an implementation.</p><ul>{{range .Questions}}{{if .Proposed}}<li><code>{{.Call}}</code> — code proposes <code>{{.Proposed}}</code><br><span class="meta">If right, confirm:</span> <code class="meta">park example add '{{.Call}} == {{.Proposed}}'</code></li>{{else}}<li><code>{{.Call}}</code> — no answer<br><code class="meta">park example add '{{.Call}} == …'</code></li>{{end}}{{end}}</ul></section>{{end}}
<nav aria-label="Processes"><ul>{{range .Processes}}<li><a href="#{{.Name}}">{{.Name}}</a><ul>{{range .Functions}}<li><a href="#{{.Anchor}}">{{.Signature}}</a></li>{{end}}</ul></li>{{else}}<li>No processes registered.</li>{{end}}</ul></nav>
<main>{{range .Processes}}<section id="{{.Name}}"><h2>{{.Name}}</h2><p>{{.Description}}</p>{{if .Uses}}<p>Uses: {{range .Uses}}<a href="#{{.}}">{{.}}</a> {{end}}</p>{{end}}
{{range .Functions}}{{$fn := .}}<article id="{{.Anchor}}"><h3><code>{{.Signature}}</code></h3><p>{{.Description}}</p><p class="meta">{{.State}} · {{.Count}} authored examples{{if .Source}} · <code>{{.Source}}</code>{{end}}</p>
{{range .Rows}}<div><pre>{{range .Inputs}}{{.Name}}: {{.Value}}
{{end}}{{$fn.Output}}: {{.Output}}</pre></div>{{else}}<p>No authored examples.</p>{{end}}
{{if .Omitted}}<p class="meta">{{.Omitted}} examples omitted; showing first two and last two.</p>{{end}}
{{if .Code}}<details><summary>Show code</summary><pre>{{.Code}}</pre></details>{{end}}</article>{{end}}</section>{{end}}</main>
<footer>Project revision: <code>{{.Revision}}</code></footer></body></html>
`))
