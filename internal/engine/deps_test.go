package engine

import (
	"reflect"
	"testing"

	"github.com/shanecandoit/ProcessStarlark/internal/project"
)

const depsZones = `Process Zones:
    example: zone("US") == "domestic"

    def zone(country):
        pass

    def region(country):
        result = "x"
        return result
`

// A plain file (process Shipping), since only plain files hold constants.
const depsShipping = `BASE = 499
STEP = 100
UNUSED = 7
TABLE = {"domestic": 1}

example: quote("US", 500) == 599

def quote(country, grams):
    cents = BASE + _fee(grams) + TABLE[Zones.zone(country)]
    return cents

def _fee(grams):
    return _steps(grams) * STEP

def _steps(grams):
    return (grams + 499) // 500

def _unused():
    return UNUSED

def shadow(x):
    BASE = x
    result = BASE
    return result

def countdown(n):
    result = 0
    if n > 0:
        result = countdown(n - 1)
    return result

def via_namespace(n):
    result = Shipping.countdown(n) + len(Zones.region("US"))
    return result

def dynamic(name):
    result = getattr(Zones, name)("US")
    return result

def lookup(country):
    pass
`

func TestDeps(t *testing.T) {
	p, err := project.FromFiles("", map[string][]byte{"zones.star": []byte(depsZones), "shipping.star": []byte(depsShipping)})
	if err != nil {
		t.Fatal(err)
	}
	e, err := New(p)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]Deps{
		// Constants through helpers count; an unreached helper does not.
		"Shipping.quote": {Calls: []string{"Zones.zone"}, Helpers: []string{"_fee", "_steps"}, Constants: []string{"BASE", "STEP", "TABLE"}},
		// A local of the same name is not the constant.
		"Shipping.shadow":    {},
		"Shipping.countdown": {Calls: []string{"Shipping.countdown"}},
		// The process's own namespace dispatches like the bare name.
		"Shipping.via_namespace": {Calls: []string{"Shipping.countdown", "Zones.region"}},
		// A namespace used as a value may reach any of its functions.
		"Shipping.dynamic": {Calls: []string{"Zones.region", "Zones.zone"}},
		"Zones.region":     {},
	}
	for fn, want := range cases {
		got, ok := e.Deps(fn)
		if !ok {
			t.Errorf("%s: no deps", fn)
			continue
		}
		if !equalDeps(got, want) {
			t.Errorf("%s: got %+v, want %+v", fn, got, want)
		}
	}
	for _, fn := range []string{"Zones.zone", "Shipping.lookup", "Shipping.nope"} {
		if d, ok := e.Deps(fn); ok {
			t.Errorf("%s has no code but got deps %+v", fn, d)
		}
	}
	if got, want := e.Reach("Shipping.via_namespace"), []string{"Shipping.via_namespace", "Shipping.countdown", "Zones.region"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Reach = %v, want %v", got, want)
	}
	if got, want := e.Reach("Shipping.countdown"), []string{"Shipping.countdown"}; !reflect.DeepEqual(got, want) {
		t.Errorf("recursive Reach = %v, want %v", got, want)
	}
}

func equalDeps(a, b Deps) bool {
	norm := func(s []string) []string {
		if len(s) == 0 {
			return nil
		}
		return s
	}
	return reflect.DeepEqual(norm(a.Calls), norm(b.Calls)) && reflect.DeepEqual(norm(a.Helpers), norm(b.Helpers)) && reflect.DeepEqual(norm(a.Constants), norm(b.Constants))
}
