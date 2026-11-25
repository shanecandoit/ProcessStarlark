package draftstore

import (
	"context"
	"encoding/json"

	"github.com/shanecandoit/ProcessStarlark/internal/workspace"
)

// ExecuteRelease serializes with draft edits across independent connections.
// Failed checks/decisions roll back without changing drafts or production.
func (s *Store) ExecuteRelease(ctx context.Context, c workspace.ReleaseCommand) (workspace.ReleaseResult, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return workspace.ReleaseResult{}, err
	}
	defer tx.Rollback()
	var data string
	if err = tx.QueryRowContext(ctx, `SELECT state_json FROM release_workflow WHERE id=1`).Scan(&data); err != nil {
		return workspace.ReleaseResult{}, err
	}
	var state workspace.ReleaseState
	if err = json.Unmarshal([]byte(data), &state); err != nil {
		return workspace.ReleaseResult{}, err
	}
	result, err := state.Execute(ctx, s.Reference(), func(id string) (*workspace.DraftWorkspace, error) { return readDraft(ctx, tx, id) }, c)
	if err != nil {
		return result, err
	}
	if c.Mutates() {
		if _, err = tx.ExecContext(ctx, `UPDATE release_workflow SET state_json=? WHERE id=1`, encode(state)); err != nil {
			return workspace.ReleaseResult{}, err
		}
	}
	if err = tx.Commit(); err != nil {
		return workspace.ReleaseResult{}, err
	}
	return workspace.FinishRelease(ctx, result, c)
}
