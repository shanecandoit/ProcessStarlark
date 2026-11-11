package server

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/shanecandoit/ProcessStarlark/internal/workspace"
)

func shippingHandler(t *testing.T) http.Handler {
	t.Helper()
	files := workspace.Files{}
	paths, err := filepath.Glob("../../examples/shipping/*.star")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		files[filepath.Base(path)] = string(content)
	}
	handler, err := NewDraftHandler(files)
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func send(handler http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	var content []byte
	if body != nil {
		content, _ = json.Marshal(body)
	}
	r := httptest.NewRequest(method, "http://127.0.0.1:8081"+path, bytes.NewReader(content))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}

func decodeResponse(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var value map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &value); err != nil {
		t.Fatal(w.Code, w.Body.String(), err)
	}
	return value
}

func jsonPointer(value any, path string) (any, error) {
	for _, part := range strings.Split(strings.TrimPrefix(path, "/"), "/") {
		part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
		switch current := value.(type) {
		case map[string]any:
			var ok bool
			value, ok = current[part]
			if !ok {
				return nil, fmt.Errorf("missing key %q", part)
			}
		case []any:
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index >= len(current) {
				return nil, fmt.Errorf("invalid index %q", part)
			}
			value = current[index]
		default:
			return nil, fmt.Errorf("cannot traverse %q", part)
		}
	}
	return value, nil
}

// This fixture is also replayed by scripts/draft_walkthrough.py. Every documented
// step must remain executable with the real workspace engine, without a socket.
func TestDocumentedDraftWalkthrough(t *testing.T) {
	handler := shippingHandler(t)
	base := decodeResponse(t, send(handler, "GET", "/api/v1/base", nil))["files"]
	data, err := os.ReadFile("../../docs/tutorials/draft-workflow.json")
	if err != nil {
		t.Fatal(err)
	}
	data = bytes.ReplaceAll(data, []byte("{draft}"), []byte("shipping-guide"))
	var guide struct {
		Steps []struct {
			Title, Method, Path string
			Body                any
			Expect              struct {
				Status             int
				JSON               map[string]any
				ReferenceUnchanged bool   `json:"reference_unchanged"`
				ArchiveMember      string `json:"archive_member"`
				ArchiveContains    string `json:"archive_contains"`
			}
		}
	}
	if err := json.Unmarshal(data, &guide); err != nil {
		t.Fatal(err)
	}
	for _, step := range guide.Steps {
		t.Run(step.Title, func(t *testing.T) {
			w := send(handler, step.Method, step.Path, step.Body)
			if w.Code != step.Expect.Status {
				t.Fatalf("status %d, want %d: %s", w.Code, step.Expect.Status, w.Body.String())
			}
			if step.Expect.ArchiveMember != "" {
				if w.Header().Get("Content-Type") != "application/zip" {
					t.Fatal(w.Header())
				}
				archive, err := zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
				if err != nil {
					t.Fatal(err)
				}
				file, err := archive.Open(step.Expect.ArchiveMember)
				if err != nil {
					t.Fatal(err)
				}
				defer file.Close()
				content, err := io.ReadAll(file)
				if err != nil || !strings.Contains(string(content), step.Expect.ArchiveContains) {
					t.Fatal("export is missing the saved example", err)
				}
				return
			}
			value := decodeResponse(t, w)
			for path, want := range step.Expect.JSON {
				got, err := jsonPointer(value, path)
				if err != nil || !reflect.DeepEqual(got, want) {
					t.Errorf("%s: got %#v (%v), want %#v", path, got, err, want)
				}
			}
			if step.Expect.ReferenceUnchanged && !reflect.DeepEqual(base, value["files"]) {
				t.Fatal("reference files changed")
			}
		})
	}
}

func createDraft(t *testing.T, h http.Handler, id string) {
	t.Helper()
	w := send(h, "POST", "/api/v1/drafts", createRequest{ID: id, Name: "My draft", Author: "ops"})
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestDraftHTTPConcurrentEdits(t *testing.T) {
	h := shippingHandler(t)
	createDraft(t, h, "shared")
	start := make(chan struct{})
	responses := make(chan *httptest.ResponseRecorder, 2)
	var wg sync.WaitGroup
	for _, country := range []string{"DE", "JP"} {
		wg.Go(func() {
			<-start
			responses <- send(h, "POST", "/api/v1/drafts/shared/operations", map[string]any{"author": "ops", "expected_version": 0, "operation": workspace.DraftOperation{Kind: "add-example", Example: fmt.Sprintf(`Zones.zone(%q) == "international"`, country)}})
		})
	}
	close(start)
	wg.Wait()
	close(responses)
	counts := map[int]int{}
	for w := range responses {
		counts[w.Code]++
	}
	if counts[200] != 1 || counts[409] != 1 {
		t.Fatalf("lost conflict: %v", counts)
	}
	snap := decodeResponse(t, send(h, "GET", "/api/v1/drafts/shared?history=true", nil))["draft"].(map[string]any)
	if snap["version"] != float64(1) || len(snap["history"].([]any)) != 1 {
		t.Fatal(snap)
	}
}

func TestDraftHTTPValidationAndFork(t *testing.T) {
	h := shippingHandler(t)
	createDraft(t, h, "original")
	for _, tc := range []struct {
		body   any
		status int
	}{
		{map[string]any{"author": "ops", "operation": map[string]any{"kind": "test"}}, 428},
		{map[string]any{"author": "ops", "expected_version": 0, "operation": map[string]any{"kind": "files", "files": map[string]string{"../bad.star": "pass"}}}, 400},
		{map[string]any{"author": "ops", "expected_version": 0, "operation": map[string]any{"kind": "files", "files": map[string]string{"zones.star": "not valid Starlark"}}}, 422},
		{map[string]any{"author": "ops", "expected_version": 0, "operation": map[string]any{"kind": "unknown"}}, 400},
	} {
		w := send(h, "POST", "/api/v1/drafts/original/operations", tc.body)
		if w.Code != tc.status {
			t.Fatalf("status %d want %d: %s", w.Code, tc.status, w.Body.String())
		}
	}
	if w := send(h, "GET", "/api/v1/drafts/missing", nil); w.Code != 404 {
		t.Fatal(w.Code)
	}
	w := send(h, "POST", "/api/v1/drafts/original/fork", createRequest{ID: "copy", Name: "My experiment", Author: "dev"})
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = send(h, "POST", "/api/v1/drafts/copy/operations", map[string]any{"author": "dev", "expected_version": 0, "operation": workspace.DraftOperation{Kind: "add-example", Example: `Zones.zone("JP") == "international"`}})
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	original := decodeResponse(t, send(h, "GET", "/api/v1/drafts/original?history=true", nil))["draft"].(map[string]any)
	history, _ := original["history"].([]any)
	if original["version"] != float64(0) || len(history) != 0 {
		t.Fatal("failed operations or fork changed original", original)
	}
}

func TestDraftHTTPRejectsMalformedAndForeignRequests(t *testing.T) {
	h := shippingHandler(t)
	for _, tc := range []struct {
		body, contentType, origin string
		status                    int
	}{
		{`{`, "application/json", "", 400},
		{`{} {}`, "application/json", "", 400},
		{`{"unknown":true}`, "application/json", "", 400},
		{`{}`, "text/plain", "", 415},
		{`{}`, "application/json", "https://other.example", 403},
		{`{"name":"` + strings.Repeat("x", maxRequestBytes) + `"}`, "application/json", "", 413},
	} {
		r := httptest.NewRequest("POST", "http://127.0.0.1:8081/api/v1/drafts", strings.NewReader(tc.body))
		r.Header.Set("Content-Type", tc.contentType)
		r.Header.Set("Origin", tc.origin)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Errorf("status %d want %d: %s", w.Code, tc.status, w.Body.String())
		}
	}
}
