// Package server exposes a local, single-project draft API with memory or
// SQLite storage, pinned reviews, and local releases. Authentication is separate.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/shanecandoit/ProcessStarlark/internal/workspace"
)

const maxRequestBytes = 4 << 20
const maxDrafts = 16

var draftID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

type draftAPI struct {
	base  workspace.Files
	store DraftStore
}

type response struct {
	Draft  workspace.DraftSnapshot `json:"draft"`
	Result *workspace.Result       `json:"result,omitempty"`
}

type createRequest struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Author       string `json:"author"`
	FromRevision string `json:"from_revision,omitempty"`
}

type operationRequest struct {
	Author          string                   `json:"author"`
	ExpectedVersion *int                     `json:"expected_version"`
	ExpectedState   string                   `json:"expected_state"`
	Operation       workspace.DraftOperation `json:"operation"`
}

// NewDraftHandler copies and validates a read-only reference project. Each draft
// starts from that reference. None of these endpoints can change the reference.
func NewDraftHandler(base workspace.Files) (http.Handler, error) {
	loaded := workspace.Format(base)
	if loaded.Status != workspace.OK {
		return nil, fmt.Errorf("reference project: %s", loaded.Message)
	}
	return NewStoredDraftHandler(&memoryStore{base: loaded.Files, drafts: workspace.NewDraftManager()})
}

func NewStoredDraftHandler(store DraftStore) (http.Handler, error) {
	loaded := workspace.Load(store.Reference())
	if loaded.Status != workspace.OK {
		return nil, fmt.Errorf("reference project: %s", loaded.Message)
	}
	api := &draftAPI{base: loaded.Files, store: store}
	mux := http.NewServeMux()
	api.releaseRoutes(mux)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "storage": store.Kind()})
	})
	mux.HandleFunc("GET /api/v1/storage", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]string{"storage": store.Kind()})
	})
	mux.HandleFunc("GET /api/v1/base", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, http.StatusOK, workspace.Load(api.base)) })
	mux.HandleFunc("POST /api/v1/drafts", api.create)
	mux.HandleFunc("GET /api/v1/drafts", func(w http.ResponseWriter, r *http.Request) {
		drafts, err := api.store.List(r.Context())
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"drafts": drafts})
	})
	mux.HandleFunc("GET /api/v1/drafts/{id}", api.get)
	mux.HandleFunc("POST /api/v1/drafts/{id}/operations", api.operation)
	mux.HandleFunc("GET /api/v1/drafts/{id}/diff", api.diff)
	mux.HandleFunc("GET /api/v1/drafts/{id}/questions", api.questions)
	mux.HandleFunc("GET /api/v1/drafts/{id}/export", api.export)
	mux.HandleFunc("POST /api/v1/drafts/{id}/fork", api.fork)
	mux.HandleFunc("GET /api/v1/drafts/{id}/versions", api.versions)
	mux.HandleFunc("GET /api/v1/drafts/{id}/versions/{revision}", api.version)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		// No CORS support. Reject foreign browser origins even for local use.
		if origin := r.Header.Get("Origin"); origin != "" && origin != "http://"+r.Host {
			writeError(w, http.StatusForbidden, "cross-origin requests are not supported")
			return
		}
		mux.ServeHTTP(w, r)
	}), nil
}

func decode(w http.ResponseWriter, r *http.Request, value any) bool {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		writeError(w, http.StatusUnsupportedMediaType, "send Content-Type: application/json")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	err = decoder.Decode(value)
	if err == nil {
		var extra any
		if decoder.Decode(&extra) != io.EOF {
			err = fmt.Errorf("send exactly one JSON object")
		}
	}
	if err != nil {
		var oversized *http.MaxBytesError
		if errors.As(err, &oversized) {
			writeError(w, http.StatusRequestEntityTooLarge, "request exceeds 4 MiB")
		} else {
			writeError(w, http.StatusBadRequest, err.Error())
		}
		return false
	}
	return true
}

func (api *draftAPI) create(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if !decode(w, r, &req) {
		return
	}
	api.createDraft(w, r, req, "")
}

func (api *draftAPI) createDraft(w http.ResponseWriter, r *http.Request, req createRequest, from string) {
	if !draftID.MatchString(req.ID) || strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Author) == "" {
		writeError(w, http.StatusBadRequest, "id (1-64 letters, digits, _ or -), name, and author are required")
		return
	}
	if from == "" && req.FromRevision != "" {
		writeError(w, http.StatusBadRequest, "from_revision is only supported when forking a draft")
		return
	}
	snap, err := api.store.Create(r.Context(), req.ID, req.Name, req.Author, from, req.FromRevision)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	w.Header().Set("Location", "/api/v1/drafts/"+req.ID)
	writeJSON(w, http.StatusCreated, response{Draft: snap})
}

func (api *draftAPI) find(w http.ResponseWriter, r *http.Request) *workspace.DraftWorkspace {
	draft, err := api.store.Read(r.Context(), r.PathValue("id"))
	if err != nil {
		writeStoreError(w, err)
		return nil
	}
	return draft
}

func (api *draftAPI) get(w http.ResponseWriter, r *http.Request) {
	if draft := api.find(w, r); draft != nil {
		writeJSON(w, http.StatusOK, response{Draft: draft.GetSnapshot(r.URL.Query().Get("history") == "true")})
	}
}

func (api *draftAPI) operation(w http.ResponseWriter, r *http.Request) {
	var req operationRequest
	if !decode(w, r, &req) {
		return
	}
	if req.ExpectedVersion == nil && req.ExpectedState == "" {
		writeError(w, http.StatusPreconditionRequired, "expected_state is required; fetch the latest draft before changing it (legacy expected_version is also accepted)")
		return
	}
	if req.ExpectedVersion != nil && req.ExpectedState != "" {
		writeError(w, http.StatusBadRequest, "send expected_state or legacy expected_version, not both")
		return
	}
	if req.ExpectedVersion != nil && *req.ExpectedVersion < 0 {
		writeError(w, http.StatusBadRequest, "expected_version must be nonnegative")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	result, snap, err := api.store.Apply(ctx, r.PathValue("id"), req.Author, req.ExpectedState, req.ExpectedVersion, req.Operation)
	if err != nil {
		if errors.Is(err, workspace.ErrDraftConflict) {
			writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "draft": snap})
			return
		}
		writeStoreError(w, err)
		return
	}
	status := http.StatusOK
	if result.Status == workspace.Error {
		status = http.StatusUnprocessableEntity
	}
	writeJSON(w, status, response{Draft: snap, Result: &result})
}

func (api *draftAPI) diff(w http.ResponseWriter, r *http.Request) {
	draft := api.find(w, r)
	if draft == nil {
		return
	}
	diff, err := draft.Diff(api.base)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, diff)
}

func (api *draftAPI) export(w http.ResponseWriter, r *http.Request) {
	draft := api.find(w, r)
	if draft == nil {
		return
	}
	snap := draft.GetSnapshot(false)
	result := workspace.Archive(snap.Files)
	if result.Status != workspace.OK {
		writeError(w, http.StatusUnprocessableEntity, result.Message)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="park-draft.zip"`)
	w.Header().Set("X-Park-Draft-Version", fmt.Sprint(snap.Version))
	w.Header().Set("X-Park-Source-Version", snap.Revision)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(result.Archive)
}

func (api *draftAPI) fork(w http.ResponseWriter, r *http.Request) {
	if api.find(w, r) == nil {
		return
	}
	var req createRequest
	if !decode(w, r, &req) {
		return
	}
	api.createDraft(w, r, req, r.PathValue("id"))
}

func (api *draftAPI) versions(w http.ResponseWriter, r *http.Request) {
	if draft := api.find(w, r); draft != nil {
		writeJSON(w, http.StatusOK, map[string]any{"versions": draft.Versions()})
	}
}

func (api *draftAPI) version(w http.ResponseWriter, r *http.Request) {
	if draft := api.find(w, r); draft != nil {
		v, ok := draft.SourceVersion(r.PathValue("revision"))
		if !ok {
			writeError(w, http.StatusNotFound, "source version not found in this draft")
			return
		}
		writeJSON(w, http.StatusOK, v)
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func (api *draftAPI) questions(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	if status != "" && status != "open" && status != "all" {
		writeError(w, 400, "status must be open or all")
		return
	}
	if draft := api.find(w, r); draft != nil {
		writeJSON(w, 200, map[string]any{"questions": draft.Questions(status == "all")})
	}
}

func writeStoreError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, workspace.ErrReleaseChecks):
		status = 422
	case errors.Is(err, workspace.ErrReleaseConflict):
		status = 409
	case errors.Is(err, workspace.ErrReleaseNotFound):
		status = 404
	case errors.Is(err, workspace.ErrReleaseInput):
		status = 400
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		status = 408
	case errors.Is(err, workspace.ErrDraftNotFound):
		status = 404
	case errors.Is(err, workspace.ErrDraftExists), errors.Is(err, workspace.ErrDraftLimit):
		status = 409
	case errors.Is(err, workspace.ErrDraftInput), errors.Is(err, workspace.ErrSourceVersionNotFound):
		status = 400
	}
	writeError(w, status, err.Error())
}
