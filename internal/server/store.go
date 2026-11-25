package server

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/shanecandoit/ProcessStarlark/internal/workspace"
)

type DraftStore interface {
	ExecuteRelease(context.Context, workspace.ReleaseCommand) (workspace.ReleaseResult, error)
	Reference() workspace.Files
	Kind() string
	Read(context.Context, string) (*workspace.DraftWorkspace, error)
	List(context.Context) ([]workspace.DraftSnapshot, error)
	Create(context.Context, string, string, string, string, string) (workspace.DraftSnapshot, error)
	Apply(context.Context, string, string, string, *int, workspace.DraftOperation) (workspace.Result, workspace.DraftSnapshot, error)
}

type memoryStore struct {
	base     workspace.Files
	drafts   *workspace.DraftManager
	mu       sync.Mutex
	releases workspace.ReleaseState
}

func (s *memoryStore) Reference() workspace.Files {
	out := workspace.Files{}
	for n, t := range s.base {
		out[n] = t
	}
	return out
}
func (s *memoryStore) Kind() string { return "memory" }
func (s *memoryStore) Read(_ context.Context, id string) (*workspace.DraftWorkspace, error) {
	d, ok := s.drafts.Get(id)
	if !ok {
		return nil, workspace.ErrDraftNotFound
	}
	return d, nil
}
func (s *memoryStore) List(_ context.Context) ([]workspace.DraftSnapshot, error) {
	return s.drafts.List(), nil
}
func (s *memoryStore) Create(_ context.Context, id, name, author, from, revision string) (workspace.DraftSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.drafts.Get(id); ok {
		return workspace.DraftSnapshot{}, workspace.ErrDraftExists
	}
	if len(s.drafts.List()) >= maxDrafts {
		return workspace.DraftSnapshot{}, workspace.ErrDraftLimit
	}
	var d *workspace.DraftWorkspace
	var err error
	if from == "" {
		d, err = s.drafts.Create(id, name, author, s.base)
	} else {
		d, err = s.drafts.ForkAt(from, id, name, author, revision)
	}
	if err != nil {
		return workspace.DraftSnapshot{}, err
	}
	return d.GetSnapshot(true), nil
}
func (s *memoryStore) Apply(ctx context.Context, id, author, expected string, legacy *int, op workspace.DraftOperation) (workspace.Result, workspace.DraftSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := s.Read(ctx, id)
	if err != nil {
		return workspace.Result{}, workspace.DraftSnapshot{}, err
	}
	if legacy != nil {
		return d.Apply(ctx, author, *legacy, op)
	}
	return d.ApplyState(ctx, author, expected, op)
}

// All source writes and review decisions share the project lock. Copy results
// across the store boundary so callers cannot mutate immutable manifests.
func (s *memoryStore) ExecuteRelease(ctx context.Context, c workspace.ReleaseCommand) (workspace.ReleaseResult, error) {
	s.mu.Lock()
	result, err := s.releases.Execute(ctx, s.Reference(), func(id string) (*workspace.DraftWorkspace, error) { return s.Read(ctx, id) }, c)
	if err != nil {
		s.mu.Unlock()
		return result, err
	}
	b, err := json.Marshal(result)
	s.mu.Unlock()
	if err != nil {
		return workspace.ReleaseResult{}, err
	}
	var copy workspace.ReleaseResult
	err = json.Unmarshal(b, &copy)
	if err != nil {
		return workspace.ReleaseResult{}, err
	}
	return workspace.FinishRelease(ctx, copy, c)
}
