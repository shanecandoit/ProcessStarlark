package server

import (
	"context"
	"net/http"
	"time"

	"github.com/shanecandoit/ProcessStarlark/internal/workspace"
)

type releaseRequest struct {
	Author             string               `json:"author"`
	ExpectedState      string               `json:"expected_state"`
	ExpectedGeneration *int                 `json:"expected_generation"`
	Reason             string               `json:"reason"`
	ReleaseID          string               `json:"release_id"`
	Call               string               `json:"call"`
	DraftRevision      string               `json:"draft_revision"`
	Scenarios          []workspace.Scenario `json:"scenarios"`
}

func (api *draftAPI) releaseRoutes(mux *http.ServeMux) {
	for _, route := range []struct{ pattern, action string }{
		{"GET /api/v1/reviews", "reviews"}, {"GET /api/v1/reviews/{id}", "review"},
		{"GET /api/v1/releases", "releases"}, {"GET /api/v1/releases/{id}", "release"},
		{"GET /api/v1/production", "status"}, {"POST /api/v1/drafts/{id}/reviews", "request"},
		{"POST /api/v1/reviews/{id}/approve", "approve"}, {"POST /api/v1/reviews/{id}/reject", "reject"},
		{"POST /api/v1/drafts/{id}/compare", "compare"},
		{"POST /api/v1/production/rollback", "rollback"}, {"POST /api/v1/production/run", "run"},
	} {
		mux.HandleFunc(route.pattern, func(w http.ResponseWriter, r *http.Request) {
			cmd := workspace.ReleaseCommand{Action: route.action, ID: r.PathValue("id")}
			if r.Method == http.MethodPost {
				var req releaseRequest
				if !decode(w, r, &req) {
					return
				}
				cmd.Author, cmd.ExpectedState, cmd.ExpectedGeneration, cmd.Reason, cmd.Call = req.Author, req.ExpectedState, req.ExpectedGeneration, req.Reason, req.Call
				cmd.DraftRevision, cmd.Scenarios = req.DraftRevision, req.Scenarios
				if cmd.Action == "request" || cmd.Action == "compare" {
					cmd.DraftID, cmd.ID = cmd.ID, ""
				}
				if cmd.Action == "rollback" {
					cmd.ID = req.ReleaseID
				}
			}
			ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
			defer cancel()
			result, err := api.store.ExecuteRelease(ctx, cmd)
			if err != nil {
				writeStoreError(w, err)
				return
			}
			if cmd.Action == "reviews" {
				if result.Reviews == nil {
					result.Reviews = []workspace.Review{}
				}
				writeJSON(w, http.StatusOK, map[string]any{"production": result.Production, "reviews": result.Reviews})
				return
			}
			if cmd.Action == "releases" {
				if result.Releases == nil {
					result.Releases = []workspace.Release{}
				}
				writeJSON(w, http.StatusOK, map[string]any{"production": result.Production, "releases": result.Releases})
				return
			}
			status := http.StatusOK
			if cmd.Action == "request" {
				status = http.StatusCreated
				w.Header().Set("Location", "/api/v1/reviews/"+result.Review.ID)
			}
			if result.Run != nil && result.Run.Status == workspace.Error {
				status = http.StatusUnprocessableEntity
			}
			writeJSON(w, status, result)
		})
	}
}
