package workspace

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/shanecandoit/ProcessStarlark/internal/engine"
	"github.com/shanecandoit/ProcessStarlark/internal/project"
)

var (
	ErrReleaseInput    = errors.New("invalid release request")
	ErrReleaseConflict = errors.New("release conflict")
	ErrReleaseNotFound = errors.New("review or release not found")
	ErrReleaseChecks   = errors.New("release checks failed")
)

// ReleaseCommand is an operation on the local project's review/release state.
// ExpectedGeneration is a pointer because generation zero is a real precondition.
type ReleaseCommand struct {
	Action             string `json:"action"`
	ID                 string `json:"id,omitempty"`
	DraftID            string `json:"draft_id,omitempty"`
	Author             string `json:"author,omitempty"`
	ExpectedState      string `json:"expected_state,omitempty"`
	ExpectedGeneration *int   `json:"expected_generation,omitempty"`
	Reason             string `json:"reason,omitempty"`
	Call               string `json:"call,omitempty"`
}

type Review struct {
	ID             string            `json:"id"`
	DraftID        string            `json:"draft_id"`
	Revision       string            `json:"revision"`
	SourceRevision string            `json:"source_revision"`
	Runtime        string            `json:"runtime"`
	Requester      string            `json:"requester"`
	CreatedAt      time.Time         `json:"created_at"`
	BaseGeneration int               `json:"base_generation"`
	BaseRelease    string            `json:"base_release"`
	Files          Files             `json:"files"`
	FunctionHashes map[string]string `json:"function_hashes"`
	Checks         engine.Report     `json:"checks"`
	Diff           DraftDiff         `json:"diff"`
	Status         string            `json:"status"`
	Reviewer       string            `json:"reviewer,omitempty"`
	DecidedAt      *time.Time        `json:"decided_at,omitempty"`
	Reason         string            `json:"reason,omitempty"`
	ReleaseID      string            `json:"release_id,omitempty"`
}

type Release struct {
	ID             string            `json:"id"`
	ReviewID       string            `json:"review_id"`
	Revision       string            `json:"revision"`
	SourceRevision string            `json:"source_revision"`
	Runtime        string            `json:"runtime"`
	Files          Files             `json:"files"`
	FunctionHashes map[string]string `json:"function_hashes"`
	Checks         engine.Report     `json:"checks"`
	Requester      string            `json:"requester"`
	Reviewer       string            `json:"reviewer"`
	CreatedAt      time.Time         `json:"created_at"`
}

type Production struct {
	ReleaseID  string `json:"release_id"`
	Generation int    `json:"generation"`
}

type Activation struct {
	Production
	PreviousRelease string    `json:"previous_release"`
	Kind            string    `json:"kind"`
	Author          string    `json:"author"`
	Reason          string    `json:"reason"`
	CreatedAt       time.Time `json:"created_at"`
}

// ReleaseState is serialized by the server-only store, never by browser drafts.
// Releases and activation entries are append-only; only review decisions and
// the production pointer change. Store transactions serialize edits/activation.
type ReleaseState struct {
	Reviews     map[string]Review  `json:"reviews"`
	Releases    map[string]Release `json:"releases"`
	Production  Production         `json:"production"`
	Activations []Activation       `json:"activations"`
}

type ProductionRun struct {
	Production
	Revision string `json:"revision"`
	Runtime  string `json:"runtime"`
	Status   string `json:"status"`
	Output   string `json:"output,omitempty"`
	Message  string `json:"message,omitempty"`
	Why      *Step  `json:"why,omitempty"`
}

type ReleaseResult struct {
	Production  Production     `json:"production"`
	Review      *Review        `json:"review,omitempty"`
	Release     *Release       `json:"release,omitempty"`
	Reviews     []Review       `json:"reviews,omitempty"`
	Releases    []Release      `json:"releases,omitempty"`
	Activations []Activation   `json:"activations,omitempty"`
	Run         *ProductionRun `json:"run,omitempty"`
}

func (c ReleaseCommand) Mutates() bool {
	return c.Action == "request" || c.Action == "approve" || c.Action == "reject" || c.Action == "rollback"
}

// Execute requires the caller to serialize this state with draft writes. All
// validation precedes mutation; a rejected operation leaves the state intact.
func (s *ReleaseState) Execute(ctx context.Context, base Files, read func(string) (*DraftWorkspace, error), c ReleaseCommand) (ReleaseResult, error) {
	result := ReleaseResult{Production: s.Production}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if c.Mutates() && strings.TrimSpace(c.Author) == "" {
		return result, fmt.Errorf("%w: author is required", ErrReleaseInput)
	}
	if c.Action == "request" || c.Action == "approve" || c.Action == "rollback" {
		if c.ExpectedGeneration == nil || *c.ExpectedGeneration < 0 {
			return result, fmt.Errorf("%w: expected_generation is required and must be nonnegative", ErrReleaseInput)
		}
		if *c.ExpectedGeneration != s.Production.Generation {
			return result, fmt.Errorf("%w: production generation changed; fetch production and review the new base", ErrReleaseConflict)
		}
	}
	now := time.Now().UTC()
	switch c.Action {
	case "status":
		result.Activations = append([]Activation{}, s.Activations...)
	case "reviews":
		for _, id := range project.Names(s.Reviews) {
			result.Reviews = append(result.Reviews, s.Reviews[id])
		}
	case "releases":
		for _, id := range project.Names(s.Releases) {
			result.Releases = append(result.Releases, s.Releases[id])
		}
	case "review":
		v, ok := s.Reviews[c.ID]
		if !ok {
			return result, ErrReleaseNotFound
		}
		result.Review = &v
	case "release":
		v, ok := s.Releases[c.ID]
		if !ok {
			return result, ErrReleaseNotFound
		}
		result.Release = &v
	case "request":
		d, err := read(c.DraftID)
		if err != nil {
			return result, err
		}
		snap := d.GetSnapshot(false)
		if c.ExpectedState == "" {
			return result, fmt.Errorf("%w: expected_state is required", ErrReleaseInput)
		}
		if snap.StateToken != c.ExpectedState {
			return result, fmt.Errorf("%w: draft changed; fetch the current draft", ErrReleaseConflict)
		}
		files := sourceFiles(snap.Files)
		p, err := load(files)
		if err != nil {
			return result, err
		}
		checks, err := engine.Check(ctx, p, "")
		if err != nil {
			return result, err
		}
		if err = ctx.Err(); err != nil {
			return result, err
		}
		if checks.Failures != 0 || len(checks.Questions) != 0 {
			details := []string{checks.Summary()}
			for _, line := range checks.Lines {
				if line.Failed {
					details = append(details, line.Text)
				}
			}
			for _, q := range checks.Questions {
				details = append(details, "Unresolved/unconfirmed: "+q.Line())
			}
			return result, fmt.Errorf("%w: %s", ErrReleaseChecks, strings.Join(details, "; "))
		}
		e, err := engine.New(p)
		if err != nil {
			return result, err
		}
		if s.Production.ReleaseID != "" {
			base = s.Releases[s.Production.ReleaseID].Files
		}
		diff, err := d.Diff(base)
		if err != nil {
			return result, err
		}
		v := Review{ID: digest(rand.Text()), DraftID: c.DraftID, Revision: snap.Revision, SourceRevision: snap.SourceRevision,
			Runtime: engine.RuntimeIdentity, Requester: strings.TrimSpace(c.Author), CreatedAt: now, BaseGeneration: s.Production.Generation,
			BaseRelease: s.Production.ReleaseID, Files: files, FunctionHashes: e.FunctionHashes, Checks: checks, Diff: diff, Status: "pending"}
		if s.Reviews == nil {
			s.Reviews = map[string]Review{}
		}
		s.Reviews[v.ID] = v
		result.Review = &v
	case "approve", "reject":
		v, ok := s.Reviews[c.ID]
		if !ok {
			return result, ErrReleaseNotFound
		}
		if v.Status != "pending" {
			return result, fmt.Errorf("%w: review is already %s", ErrReleaseConflict, v.Status)
		}
		if strings.TrimSpace(c.Author) == v.Requester {
			return result, fmt.Errorf("%w: reviewer must differ from requester", ErrReleaseInput)
		}
		if c.Action == "approve" {
			if v.BaseGeneration != s.Production.Generation || v.BaseRelease != s.Production.ReleaseID {
				return result, fmt.Errorf("%w: review has a stale production base; request a new review", ErrReleaseConflict)
			}
			if v.Runtime != engine.RuntimeIdentity {
				return result, fmt.Errorf("%w: runtime changed; request new checks", ErrReleaseConflict)
			}
			d, err := read(v.DraftID)
			if err != nil {
				return result, err
			}
			if d.GetSnapshot(false).Revision != v.Revision {
				return result, fmt.Errorf("%w: draft source changed; request a new review", ErrReleaseConflict)
			}
			release := Release{ID: digest(struct{ Review, Runtime string }{v.ID, v.Runtime}), ReviewID: v.ID, Revision: v.Revision, SourceRevision: v.SourceRevision,
				Runtime: v.Runtime, Files: clone(v.Files), FunctionHashes: v.FunctionHashes, Checks: v.Checks, Requester: v.Requester, Reviewer: strings.TrimSpace(c.Author), CreatedAt: now}
			if s.Releases == nil {
				s.Releases = map[string]Release{}
			}
			s.Releases[release.ID] = release
			v.Status, v.ReleaseID = "approved", release.ID
			s.activate(release.ID, "approve", c.Author, c.Reason, now)
			result.Release = &release
		} else {
			if strings.TrimSpace(c.Reason) == "" {
				return result, fmt.Errorf("%w: rejection reason is required", ErrReleaseInput)
			}
			v.Status = "rejected"
		}
		v.Reviewer, v.Reason, v.DecidedAt = strings.TrimSpace(c.Author), c.Reason, &now
		s.Reviews[v.ID] = v
		result.Review = &v
	case "rollback":
		release, ok := s.Releases[c.ID]
		if !ok {
			return result, ErrReleaseNotFound
		}
		if release.Runtime != engine.RuntimeIdentity {
			return result, fmt.Errorf("%w: release runtime differs; request a new review", ErrReleaseConflict)
		}
		if strings.TrimSpace(c.Reason) == "" {
			return result, fmt.Errorf("%w: rollback reason is required", ErrReleaseInput)
		}
		if release.ID == s.Production.ReleaseID {
			return result, fmt.Errorf("%w: release is already active", ErrReleaseConflict)
		}
		s.activate(release.ID, "rollback", c.Author, c.Reason, now)
		result.Release = &release
	case "run":
		release, ok := s.Releases[s.Production.ReleaseID]
		if !ok {
			return result, fmt.Errorf("%w: no active release; approve a review first", ErrReleaseNotFound)
		}
		if release.Runtime != engine.RuntimeIdentity {
			return result, fmt.Errorf("%w: active release requires a different runtime", ErrReleaseConflict)
		}
		// The store captures this immutable snapshot under its transaction,
		// then evaluates after releasing the lock so activation can proceed.
		result.Release = &release
	default:
		return result, fmt.Errorf("%w: unknown action %q", ErrReleaseInput, c.Action)
	}
	result.Production = s.Production
	return result, nil
}

func (s *ReleaseState) activate(id, kind, author, reason string, now time.Time) {
	previous := s.Production.ReleaseID
	s.Production = Production{ReleaseID: id, Generation: s.Production.Generation + 1}
	s.Activations = append(s.Activations, Activation{Production: s.Production, PreviousRelease: previous, Kind: kind, Author: strings.TrimSpace(author), Reason: reason, CreatedAt: now})
}

// RunRelease evaluates a captured immutable release after the store transaction
// ends. Missing answers stay local to this call; no draft questions are written.
func RunRelease(ctx context.Context, release Release, production Production, call string) ProductionRun {
	answer := Run(ctx, release.Files, call)
	return ProductionRun{Production: production, Revision: release.Revision, Runtime: release.Runtime, Status: answer.Status, Output: answer.Output, Message: answer.Message, Why: answer.Why}
}
