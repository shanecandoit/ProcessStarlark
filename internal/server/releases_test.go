package server

import (
	"net/http"
	"path/filepath"
	"testing"

	"github.com/shanecandoit/ProcessStarlark/internal/draftstore"
	"github.com/shanecandoit/ProcessStarlark/internal/workspace"
)

func TestDocumentedReleaseWalkthrough(t *testing.T) {
	t.Run("memory", func(t *testing.T) { runDocumentedWorkflow(t, shippingHandler(t), "release-workflow.json") })
	t.Run("sqlite-restart-every-request", func(t *testing.T) {
		original := decodeResponse(t, send(shippingHandler(t), "GET", "/api/v1/base", nil))["files"].(map[string]any)
		base := workspace.Files{}
		for n, v := range original {
			base[n] = v.(string)
		}
		path := filepath.Join(t.TempDir(), "releases.sqlite")
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			store, err := draftstore.Open(path, base)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			h, err := NewStoredDraftHandler(store)
			if err != nil {
				t.Fatal(err)
			}
			h.ServeHTTP(w, r)
		})
		runDocumentedWorkflow(t, handler, "release-workflow.json")
	})
}
