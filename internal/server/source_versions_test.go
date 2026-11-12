package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSourceVersionsAPIAndHistoricalFork(t *testing.T) {
	h := shippingHandler(t)
	created := decodeResponse(t, send(h, "POST", "/api/v1/drafts", createRequest{ID: "source", Name: "Source", Author: "ops"}))["draft"].(map[string]any)
	root, token := created["revision"].(string), created["state_token"].(string)
	op := func(state string, kind, example string) map[string]any {
		return map[string]any{"author": "ops", "expected_state": state, "operation": map[string]any{"kind": kind, "example": example}}
	}
	w := send(h, "POST", "/api/v1/drafts/source/operations", op(token, "add-example", `Zones.zone("DE") == "international"`))
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	saved := decodeResponse(t, w)["draft"].(map[string]any)
	head := saved["revision"].(string)
	w = send(h, "POST", "/api/v1/drafts/source/operations", op(token, "load", ""))
	if w.Code != http.StatusConflict || decodeResponse(t, w)["draft"].(map[string]any)["revision"] != head {
		t.Fatal("hash conflict lost head", w.Code, w.Body.String())
	}
	versions := decodeResponse(t, send(h, "GET", "/api/v1/drafts/source/versions", nil))["versions"].([]any)
	if len(versions) != 2 || versions[0].(map[string]any)["parent"] != root || versions[1].(map[string]any)["id"] != root {
		t.Fatal(versions)
	}
	old := decodeResponse(t, send(h, "GET", "/api/v1/drafts/source/versions/"+root, nil))
	if strings.Contains(old["files"].(map[string]any)["zones.star"].(string), `zone("DE")`) || old["parent"] != "" {
		t.Fatal("historical source was changed", old)
	}
	w = send(h, "POST", "/api/v1/drafts/source/fork", createRequest{ID: "branch", Name: "Branch", Author: "dev", FromRevision: root})
	if w.Code != 201 || decodeResponse(t, w)["draft"].(map[string]any)["revision"] != root {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := send(h, "GET", "/api/v1/drafts/branch/versions/"+head, nil); w.Code != 404 {
		t.Fatal("unrelated descendant exposed", w.Code)
	}
	if w := send(h, "POST", "/api/v1/drafts/source/fork", createRequest{ID: "bad", Name: "Bad", Author: "dev", FromRevision: "unknown"}); w.Code != 400 {
		t.Fatal(w.Code)
	}
	w = send(h, "GET", "/api/v1/drafts/source/export", nil)
	if w.Code != 200 || w.Header().Get("X-Park-Source-Version") != head {
		t.Fatal(w.Code, w.Header())
	}
	ambiguous := op(saved["state_token"].(string), "load", "")
	ambiguous["expected_version"] = 1
	if w := send(h, "POST", "/api/v1/drafts/source/operations", ambiguous); w.Code != 400 {
		t.Fatal("ambiguous preconditions accepted", w.Code)
	}
}

func TestPlaygroundServesSameOriginAPIAndConfig(t *testing.T) {
	dir := t.TempDir()
	if _, err := WithPlayground(shippingHandler(t), dir); err == nil {
		t.Fatal("missing browser build accepted")
	}
	for _, name := range []string{"index.html", "app.js", "draft-client.js", "park.wasm", "worker.js"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("playground"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	h, err := WithPlayground(shippingHandler(t), dir)
	if err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{"/": "playground", "/draft-config.js": "PARK_DRAFT_API = true", "/api/v1/base": "Shipping.quote", "/healthz": `"storage":"memory"`} {
		w := send(h, "GET", path, nil)
		if w.Code != 200 || !strings.Contains(w.Body.String(), want) {
			t.Fatal(path, w.Code, w.Body.String())
		}
	}
}
