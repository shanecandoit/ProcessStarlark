package engine

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shanecandoit/ProcessStarlark/internal/project"
)

// fake answers from a table keyed by Process.fn, and records what it was asked.
type fake struct {
	name    string
	answers map[string]any
	err     error
	asked   []Call
	ctx     context.Context
}

func (f *fake) Name() string { return f.name }
func (f *fake) Resolve(ctx context.Context, c Call) (any, error) {
	f.asked = append(f.asked, c)
	f.ctx = ctx
	if f.err != nil {
		return nil, f.err
	}
	if out, ok := f.answers[c.Function]; ok {
		return out, nil
	}
	return nil, ErrUnknown
}

// shipping loads Zones (example-only) and a ready Shipping.quote that calls it.
func shipping(t *testing.T) (*Engine, string) {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"zones.star": "Process Zones:\n    example: zone(\"US\") == \"domestic\"\n    example: zone(\"FR\") == \"international\"\n\n" +
			"    def zone(country):\n        pass\n",
		"shipping.star": "Process Shipping:\n    example: quote(\"US\", 100) == 500\n    example: quote(\"FR\", 100) == 1500\n\n" +
			"    def quote(country, grams):\n" +
			"        base = {\"domestic\": 500, \"international\": 1500}\n" +
			"        price = base[Zones.zone(country)] + 0 * grams\n" +
			"        return price\n",
	}
	for name, contents := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(contents), 0644); err != nil {
			t.Fatal(err)
		}
	}
	p, err := project.LoadWith(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	e, err := New(p)
	if err != nil {
		t.Fatal(err)
	}
	return e, root
}

func TestResolverAnswersMissAsProposal(t *testing.T) {
	e, root := shipping(t)
	r := &fake{name: "guess", answers: map[string]any{"Zones.zone": "international"}}
	e.Resolvers = []Resolver{r}
	type key struct{}
	ctx := context.WithValue(context.Background(), key{}, "run")
	out, err := e.Run(ctx, "Shipping.quote", map[string]any{"country": "DE", "grams": int64(800)})
	if err != nil || out != int64(1500) {
		t.Fatalf("got %v %v", out, err)
	}
	if len(r.asked) != 1 || r.asked[0].Function != "Zones.zone" || r.asked[0].Input["country"] != "DE" {
		t.Fatalf("asked %+v", r.asked)
	}
	if r.ctx == nil || r.ctx.Value(key{}) != "run" {
		t.Fatal("resolver did not receive the run's context")
	}
	got := e.Proposals()
	if len(got) != 1 || got[0].Function != "Zones.zone" || got[0].Output != "international" || got[0].By != "guess" {
		t.Fatalf("proposals %+v", got)
	}
	// Nothing resting on a proposal is cached, including the confirmed caller.
	if entries, _ := os.ReadDir(filepath.Join(root, ".cache")); len(entries) != 0 {
		t.Fatalf("cached %d results that rest on a resolver", len(entries))
	}
	// Examples still answer first; the resolver is not asked.
	r.asked = nil
	if out, err := run(t, e, "Zones.zone", map[string]any{"country": "US"}); err != nil || out != "domestic" || len(r.asked) != 0 {
		t.Fatalf("example: %v %v asked=%d", out, err, len(r.asked))
	}
}

func TestResolversRunInOrderAndCanPass(t *testing.T) {
	e, _ := shipping(t)
	first := &fake{name: "first"}
	second := &fake{name: "second", answers: map[string]any{"Zones.zone": "domestic"}}
	e.Resolvers = []Resolver{first, second}
	if out, err := run(t, e, "Zones.zone", map[string]any{"country": "CA"}); err != nil || out != "domestic" {
		t.Fatalf("got %v %v", out, err)
	}
	if len(first.asked) != 1 || len(second.asked) != 1 || e.Proposals()[0].By != "second" {
		t.Fatalf("order: %d %d %+v", len(first.asked), len(second.asked), e.Proposals())
	}

	// When every resolver passes, the call is unresolved as before.
	e.Resolvers = []Resolver{&fake{name: "none"}}
	_, err := run(t, e, "Zones.zone", map[string]any{"country": "CA"})
	var u *Unresolved
	if !errors.As(err, &u) || u.Function != "Zones.zone" {
		t.Fatalf("want unresolved, got %v", err)
	}
}

func TestResolverErrorsAndValues(t *testing.T) {
	e, _ := shipping(t)
	e.Resolvers = []Resolver{&fake{name: "broken", err: errors.New("model offline")}}
	_, err := run(t, e, "Zones.zone", map[string]any{"country": "CA"})
	var u *Unresolved
	if err == nil || errors.As(err, &u) || !strings.Contains(err.Error(), "resolver broken") || !strings.Contains(err.Error(), "model offline") {
		t.Fatalf("want resolver error, got %v", err)
	}

	// Go values are normalized to Park values; unsupported ones fail.
	e.Resolvers = []Resolver{&fake{name: "go", answers: map[string]any{"Zones.zone": map[string]any{"n": 3}}}}
	out, err := run(t, e, "Zones.zone", map[string]any{"country": "CA"})
	if m, ok := out.(map[string]any); err != nil || !ok || m["n"] != int64(3) {
		t.Fatalf("normalized: %#v %v", out, err)
	}
	e.Resolvers = []Resolver{&fake{name: "float", answers: map[string]any{"Zones.zone": 1.5}}}
	if _, err := run(t, e, "Zones.zone", map[string]any{"country": "CA"}); err == nil {
		t.Fatal("a float answer was accepted")
	}
}

func TestResolversSkippedInTests(t *testing.T) {
	e, _ := shipping(t)
	r := &fake{name: "guess", answers: map[string]any{"Zones.zone": "international"}}
	e.Resolvers = []Resolver{r}
	e.Testing = true
	var u *Unresolved
	if _, err := run(t, e, "Zones.zone", map[string]any{"country": "CA"}); !errors.As(err, &u) || len(r.asked) != 0 {
		t.Fatalf("park test asked a resolver: %v asked=%d", err, len(r.asked))
	}
}

func TestWhyShowsResolver(t *testing.T) {
	e, _ := shipping(t)
	e.Resolvers = []Resolver{&fake{name: "guess", answers: map[string]any{"Zones.zone": "international"}}}
	e.Why = true
	if _, err := run(t, e, "Shipping.quote", map[string]any{"country": "DE", "grams": int64(800)}); err != nil {
		t.Fatal(err)
	}
	s := e.Trace().Steps[0]
	if s.Source != SourceProposed || s.By != "guess" || s.Answer() != `"international"` {
		t.Fatalf("%+v", s)
	}
	var b bytes.Buffer
	if err := WriteTrace(&b, e.Trace()); err != nil {
		t.Fatal(err)
	}
	if fields := strings.Fields(strings.Split(b.String(), "\n")[1]); strings.Join(fields[2:], " ") != `"international" proposed by guess zones.star:5` {
		t.Fatalf("trace:\n%s", b.String())
	}
}
