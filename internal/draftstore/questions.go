package draftstore

import (
	"context"
	"database/sql"

	"github.com/shanecandoit/ProcessStarlark/internal/workspace"
)

func syncQuestions(ctx context.Context, tx *sql.Tx, id string, questions []workspace.QuestionRecord) error {
	for _, q := range questions {
		if _, err := user(ctx, tx, q.OpenedByName, q.OpenedAt.Format(timeFormat)); err != nil {
			return err
		}
		var closedAt, closedBy any
		if q.ClosedAt != nil {
			closedAt = q.ClosedAt.Format(timeFormat)
			closedBy = q.ClosedBy
			if _, err := user(ctx, tx, q.ClosedByName, q.ClosedAt.Format(timeFormat)); err != nil {
				return err
			}
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO questions VALUES (?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(draft_id,id) DO UPDATE SET question_json=excluded.question_json,status=excluded.status,updated_at=excluded.updated_at,closed_at=excluded.closed_at,closed_by=excluded.closed_by`,
			id, q.ID, q.Function, q.Call, encode(q), q.Status, q.OpenedAt.Format(timeFormat), q.UpdatedAt.Format(timeFormat), closedAt, q.OpenedBy, closedBy)
		if err != nil {
			return err
		}
	}
	return nil
}
