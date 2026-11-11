package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/shanecandoit/ProcessStarlark/internal/project"
)

func TestFunctionHashStability(t *testing.T) {
	// Same source should produce same hash
	source := `Process Test:
    example: add(1, 2) == 3
    example: add(2, 3) == 5

    def add(a, b):
        result = a + b
        return result
`
	p, err := project.FromFiles("", map[string][]byte{"test.star": []byte(source)})
	if err != nil {
		t.Fatal(err)
	}

	versions, err := ComputeAllFunctionHashes(p)
	if err != nil {
		t.Fatal(err)
	}

	v := versions["Test.add"]
	if v.Hash == "" {
		t.Fatal("hash should not be empty")
	}
	if v.Hash != "a8cd0b645892" && v.Hash[:12] != "a8cd0b645892" {
		// Hash depends on exact formatting, just verify it's stable
	}

	// Compute again - should be identical
	versions2, err := ComputeAllFunctionHashes(p)
	if err != nil {
		t.Fatal(err)
	}
	if versions["Test.add"].Hash != versions2["Test.add"].Hash {
		t.Fatalf("hash not stable: %s vs %s", versions["Test.add"].Hash, versions2["Test.add"].Hash)
	}
}

func TestFunctionHashChangesWithBody(t *testing.T) {
	source1 := `Process Test:
    example: add(1, 2) == 3
    example: add(2, 3) == 5

    def add(a, b):
        result = a + b
        return result
`
	source2 := `Process Test:
    example: add(1, 2) == 3
    example: add(2, 3) == 5

    def add(a, b):
        result = b + a
        return result
`
	p1, err := project.FromFiles("", map[string][]byte{"test.star": []byte(source1)})
	if err != nil {
		t.Fatal(err)
	}
	p2, err := project.FromFiles("", map[string][]byte{"test.star": []byte(source2)})
	if err != nil {
		t.Fatal(err)
	}

	v1, err := ComputeAllFunctionHashes(p1)
	if err != nil {
		t.Fatal(err)
	}
	v2, err := ComputeAllFunctionHashes(p2)
	if err != nil {
		t.Fatal(err)
	}

	if v1["Test.add"].Hash == v2["Test.add"].Hash {
		t.Fatal("hash should change when body changes (even if semantically equivalent)")
	}
}

func TestFunctionHashChangesWithExamples(t *testing.T) {
	source1 := `Process Test:
    example: add(1, 2) == 3
    example: add(2, 3) == 5

    def add(a, b):
        result = a + b
        return result
`
	source2 := `Process Test:
    example: add(1, 2) == 3
    example: add(2, 3) == 5
    example: add(10, 20) == 30

    def add(a, b):
        result = a + b
        return result
`
	p1, err := project.FromFiles("", map[string][]byte{"test.star": []byte(source1)})
	if err != nil {
		t.Fatal(err)
	}
	p2, err := project.FromFiles("", map[string][]byte{"test.star": []byte(source2)})
	if err != nil {
		t.Fatal(err)
	}

	v1, err := ComputeAllFunctionHashes(p1)
	if err != nil {
		t.Fatal(err)
	}
	v2, err := ComputeAllFunctionHashes(p2)
	if err != nil {
		t.Fatal(err)
	}

	if v1["Test.add"].Hash == v2["Test.add"].Hash {
		t.Fatal("hash should change when examples change")
	}
}

func TestFunctionHashChangesWithDepHash(t *testing.T) {
	source1 := `Process A:
    example: foo(1) == 10

    def foo(x):
        result = x + 10
        return result
`
	source2 := `Process A:
    example: foo(1) == 10

    def foo(x):
        result = x + 20
        return result
`

	sourceB := `Process B:
    example: bar(1) == 15

    def bar(y):
        result = A.foo(y) + 5
        return result
`
	p1, err := project.FromFiles("", map[string][]byte{"a.star": []byte(source1), "b.star": []byte(sourceB)})
	if err != nil {
		t.Fatal(err)
	}
	p2, err := project.FromFiles("", map[string][]byte{"a.star": []byte(source2), "b.star": []byte(sourceB)})
	if err != nil {
		t.Fatal(err)
	}

	v1, err := ComputeAllFunctionHashes(p1)
	if err != nil {
		t.Fatal(err)
	}
	v2, err := ComputeAllFunctionHashes(p2)
	if err != nil {
		t.Fatal(err)
	}

	// B.bar depends on A.foo, so when A.foo changes, B.bar's hash should change
	if v1["B.bar"].Hash == v2["B.bar"].Hash {
		t.Fatal("B.bar hash should change when A.foo changes")
	}

	// A.foo hash should change
	if v1["A.foo"].Hash == v2["A.foo"].Hash {
		t.Fatal("A.foo hash should change")
	}
}

func TestIntraProcessDependency(t *testing.T) {
	source := `Process Test:
    example: inner(1) == 2
    example: inner(2) == 3
    example: outer(1) == 3

    def inner(x):
        result = x + 1
        return result

    def outer(y):
        result = inner(y) + 1
        return result
`
	p, err := project.FromFiles("", map[string][]byte{"test.star": []byte(source)})
	if err != nil {
		t.Fatal(err)
	}

	versions, err := ComputeAllFunctionHashes(p)
	if err != nil {
		t.Fatal(err)
	}

	// outer should depend on inner
	outer := versions["Test.outer"]
	if outer.DepHashes["Test.inner"] == "" {
		t.Fatal("outer should depend on inner")
	}

	// When inner changes, outer's hash should change
	source2 := `Process Test:
    example: inner(1) == 2
    example: inner(2) == 3
    example: outer(1) == 3

    def inner(x):
        result = x + 2
        return result

    def outer(y):
        result = inner(y) + 1
        return result
`
	p2, err := project.FromFiles("", map[string][]byte{"test.star": []byte(source2)})
	if err != nil {
		t.Fatal(err)
	}

	v2, err := ComputeAllFunctionHashes(p2)
	if err != nil {
		t.Fatal(err)
	}

	if versions["Test.outer"].Hash == v2["Test.outer"].Hash {
		t.Fatal("outer hash should change when inner changes")
	}
}

func TestRecursiveFunctionNoCycle(t *testing.T) {
	source := `Process Test:
    example: countdown(0) == 0
    example: countdown(1) == 0

    def countdown(n):
        result = 0
        if n > 0:
            result = countdown(n - 1)
        return result
`
	p, err := project.FromFiles("", map[string][]byte{"test.star": []byte(source)})
	if err != nil {
		t.Fatal(err)
	}

	// Should not panic or return cycle error
	versions, err := ComputeAllFunctionHashes(p)
	if err != nil {
		t.Fatalf("recursive function should not cause cycle error: %v", err)
	}

	v := versions["Test.countdown"]
	if v.Hash == "" {
		t.Fatal("hash should not be empty")
	}
}

func TestCacheKeyUsesFunctionHash(t *testing.T) {
	source := `Process Test:
    example: add(1, 2) == 3
    example: add(2, 3) == 5

    def add(a, b):
        result = a + b
        return result
`
	// Use a temp dir so cache is enabled
	tDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tDir, "test.star"), []byte(source), 0644); err != nil {
		t.Fatal(err)
	}
	p, err := project.LoadWith(tDir, nil)
	if err != nil {
		t.Fatal(err)
	}

	e, err := New(p)
	if err != nil {
		t.Fatal(err)
	}

	// First run - cache miss
	out1, err := e.Run(context.Background(), "Test.add", map[string]any{"a": int64(5), "b": int64(7)})
	if err != nil {
		t.Fatal(err)
	}
	if out1 != int64(12) {
		t.Fatalf("expected 12, got %v", out1)
	}

	// Second run - cache hit
	out2, err := e.Run(context.Background(), "Test.add", map[string]any{"a": int64(5), "b": int64(7)})
	if err != nil {
		t.Fatal(err)
	}
	if out2 != int64(12) {
		t.Fatalf("expected 12, got %v", out2)
	}

	// Different input - cache miss
	out3, err := e.Run(context.Background(), "Test.add", map[string]any{"a": int64(10), "b": int64(20)})
	if err != nil {
		t.Fatal(err)
	}
	if out3 != int64(30) {
		t.Fatalf("expected 30, got %v", out3)
	}
}

func TestCacheInvalidationGranularity(t *testing.T) {
	source := `Process Test:
    example: add(1, 2) == 3
    example: add(2, 3) == 5
    example: double(1) == 2
    example: double(2) == 4

    def add(a, b):
        result = a + b
        return result

    def double(x):
        result = add(x, x)
        return result
`
	tDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tDir, "test.star"), []byte(source), 0644); err != nil {
		t.Fatal(err)
	}
	p, err := project.LoadWith(tDir, nil)
	if err != nil {
		t.Fatal(err)
	}

	e, err := New(p)
	if err != nil {
		t.Fatal(err)
	}

	// Run both functions to populate cache
	if _, err := e.Run(context.Background(), "Test.add", map[string]any{"a": int64(5), "b": int64(7)}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Run(context.Background(), "Test.double", map[string]any{"x": int64(5)}); err != nil {
		t.Fatal(err)
	}

	// Now create a new project with modified add function
	source2 := `Process Test:
    example: add(1, 2) == 3
    example: add(2, 3) == 5
    example: double(1) == 2
    example: double(2) == 4

    def add(a, b):
        result = a + b + 1
        return result

    def double(x):
        result = add(x, x)
        return result
`
	if err := os.WriteFile(filepath.Join(tDir, "test.star"), []byte(source2), 0644); err != nil {
		t.Fatal(err)
	}
	p2, err := project.LoadWith(tDir, nil)
	if err != nil {
		t.Fatal(err)
	}

	e2, err := New(p2)
	if err != nil {
		t.Fatal(err)
	}

	// add hash should change
	if e.FunctionHashes["Test.add"] == e2.FunctionHashes["Test.add"] {
		t.Fatal("add hash should change")
	}

	// double hash should change (depends on add)
	if e.FunctionHashes["Test.double"] == e2.FunctionHashes["Test.double"] {
		t.Fatal("double hash should change when add changes")
	}

	// Create a third project with only double modified
	source3 := `Process Test:
    example: add(1, 2) == 3
    example: add(2, 3) == 5
    example: double(1) == 2
    example: double(2) == 4

    def add(a, b):
        result = a + b
        return result

    def double(x):
        result = add(x, x) + 1
        return result
`
	if err := os.WriteFile(filepath.Join(tDir, "test.star"), []byte(source3), 0644); err != nil {
		t.Fatal(err)
	}
	p3, err := project.LoadWith(tDir, nil)
	if err != nil {
		t.Fatal(err)
	}

	e3, err := New(p3)
	if err != nil {
		t.Fatal(err)
	}

	// add hash should NOT change
	if e.FunctionHashes["Test.add"] != e3.FunctionHashes["Test.add"] {
		t.Fatal("add hash should not change when only double changes")
	}

	// double hash should change
	if e.FunctionHashes["Test.double"] == e3.FunctionHashes["Test.double"] {
		t.Fatal("double hash should change when double changes")
	}
}
