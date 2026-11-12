package server

import (
	"context"
	"sync"

	"github.com/shanecandoit/ProcessStarlark/internal/workspace"
)

type DraftStore interface {
	Reference() workspace.Files
	Kind() string
	Read(context.Context, string) (*workspace.DraftWorkspace, error)
	List(context.Context) ([]workspace.DraftSnapshot, error)
	Create(context.Context, string, string, string, string, string) (workspace.DraftSnapshot, error)
	Apply(context.Context, string, string, string, *int, workspace.DraftOperation) (workspace.Result, workspace.DraftSnapshot, error)
}

type memoryStore struct {
	base   workspace.Files
	drafts *workspace.DraftManager
	mu     sync.Mutex
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
	d, err := s.Read(ctx, id)
	if err != nil {
		return workspace.Result{}, workspace.DraftSnapshot{}, err
	}
	if legacy != nil {
		return d.Apply(ctx, author, *legacy, op)
	}
	return d.ApplyState(ctx, author, expected, op)
}
