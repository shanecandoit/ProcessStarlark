// Package server exposes a local, single-project draft API. It is an in-memory
// development service, not an authenticated or durable hosted SaaS deployment.
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
	"sync"
	"time"

	"github.com/shanecandoit/ProcessStarlark/internal/workspace"
)

const maxRequestBytes = 4 << 20
const maxDrafts = 16

var draftID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

type draftAPI struct {
	base     workspace.Files
	drafts   *workspace.DraftManager
	createMu sync.Mutex
}

type response struct {
	Draft  workspace.DraftSnapshot `json:"draft"`
	Result *workspace.Result       `json:"result,omitempty"`
}

type createRequest struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Author string `json:"author"`
}

type operationRequest struct {
	Author          string                   `json:"author"`
	ExpectedVersion *int                     `json:"expected_version"`
	Operation       workspace.DraftOperation `json:"operation"`
}

// NewDraftHandler copies and validates a read-only reference project. Each draft
// starts from that reference. None of these endpoints can change the reference.
func NewDraftHandler(base workspace.Files) (http.Handler, error) {
	loaded := workspace.Load(base)
	if loaded.Status != workspace.OK {
		return nil, fmt.Errorf("reference project: %s", loaded.Message)
	}
	api := &draftAPI{base: loaded.Files, drafts: workspace.NewDraftManager()}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "storage": "memory"})
	})
	mux.HandleFunc("GET /api/v1/base", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, http.StatusOK, workspace.Load(api.base)) })
	mux.HandleFunc("POST /api/v1/drafts", api.create)
	mux.HandleFunc("GET /api/v1/drafts", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"drafts": api.drafts.List()})
	})
	mux.HandleFunc("GET /api/v1/drafts/{id}", api.get)
	mux.HandleFunc("POST /api/v1/drafts/{id}/operations", api.operation)
	mux.HandleFunc("GET /api/v1/drafts/{id}/diff", api.diff)
	mux.HandleFunc("GET /api/v1/drafts/{id}/export", api.export)
	mux.HandleFunc("POST /api/v1/drafts/{id}/fork", api.fork)
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
	api.createMu.Lock()
	defer api.createMu.Unlock()
	if _, exists := api.drafts.Get(req.ID); exists {
		writeError(w, http.StatusConflict, "draft id already exists")
		return
	}
	if len(api.drafts.List()) >= maxDrafts {
		writeError(w, http.StatusConflict, "this local service supports at most 16 drafts; export your work before restarting")
		return
	}
	var draft *workspace.DraftWorkspace
	var err error
	if from == "" {
		draft, err = api.drafts.Create(req.ID, req.Name, req.Author, api.base)
	} else {
		draft, err = api.drafts.Fork(from, req.ID, req.Name, req.Author)
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.Header().Set("Location", "/api/v1/drafts/"+req.ID)
	writeJSON(w, http.StatusCreated, response{Draft: draft.GetSnapshot(true)})
}

func (api *draftAPI) find(w http.ResponseWriter, r *http.Request) *workspace.DraftWorkspace {
	draft, ok := api.drafts.Get(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "draft not found")
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
	draft := api.find(w, r)
	if draft == nil {
		return
	}
	var req operationRequest
	if !decode(w, r, &req) {
		return
	}
	if req.ExpectedVersion == nil {
		writeError(w, http.StatusPreconditionRequired, "expected_version is required; fetch the latest draft before changing it")
		return
	}
	if *req.ExpectedVersion < 0 {
		writeError(w, http.StatusBadRequest, "expected_version must be nonnegative")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	result, snap, err := draft.Apply(ctx, req.Author, *req.ExpectedVersion, req.Operation)
	if err != nil {
		if errors.Is(err, workspace.ErrDraftConflict) {
			writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "draft": snap})
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
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

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
