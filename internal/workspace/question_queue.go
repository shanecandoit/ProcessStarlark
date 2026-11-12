package workspace

import (
	"sort"
	"time"
)

type QuestionRecord struct {
	Question
	ID           string     `json:"id"`
	Status       string     `json:"status"`
	OpenedAt     time.Time  `json:"opened_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	ClosedAt     *time.Time `json:"closed_at,omitempty"`
	OpenedBy     string     `json:"opened_by"`
	OpenedByName string     `json:"opened_by_name"`
	ClosedBy     string     `json:"closed_by,omitempty"`
	ClosedByName string     `json:"closed_by_name,omitempty"`
}

func LocalUserID(label string) string {
	return digest(struct{ Schema, Label string }{"park-local-user-v1", label})
}
func questionID(q Question) string {
	return digest(struct {
		Function string
		Input    map[string]string
	}{q.Function, q.Input})
}

func copyQuestion(q QuestionRecord) QuestionRecord {
	q.Inputs = append([]string{}, q.Inputs...)
	input := map[string]string{}
	for name, value := range q.Input {
		input[name] = value
	}
	q.Input = input
	if q.ClosedAt != nil {
		closed := *q.ClosedAt
		q.ClosedAt = &closed
	}
	return q
}

func (dw *DraftWorkspace) reconcileQuestions(open []Question, author, kind string) {
	if dw.questions == nil {
		dw.questions = map[string]QuestionRecord{}
	}
	seen := map[string]bool{}
	for _, q := range open {
		id := questionID(q)
		seen[id] = true
		r, exists := dw.questions[id]
		if !exists {
			r = QuestionRecord{ID: id, OpenedAt: dw.updatedAt, OpenedBy: LocalUserID(author), OpenedByName: author}
		}
		r.Question = q
		r.Status = "open"
		r.UpdatedAt = dw.updatedAt
		r.ClosedAt = nil
		r.ClosedBy = ""
		r.ClosedByName = ""
		dw.questions[id] = copyQuestion(r)
	}
	for id, r := range dw.questions {
		if r.Status != "open" || seen[id] {
			continue
		}
		r.Status = "resolved"
		if kind == "dismiss" {
			r.Status = "dismissed"
		}
		now := dw.updatedAt
		r.ClosedAt = &now
		r.UpdatedAt = now
		r.ClosedBy = LocalUserID(author)
		r.ClosedByName = author
		dw.questions[id] = r
	}
}

func (dw *DraftWorkspace) Questions(includeClosed bool) []QuestionRecord {
	dw.mu.RLock()
	defer dw.mu.RUnlock()
	return dw.questionsLocked(includeClosed)
}

func (dw *DraftWorkspace) questionsLocked(all bool) []QuestionRecord {
	out := []QuestionRecord{}
	for _, q := range dw.questions {
		if all || q.Status == "open" {
			out = append(out, copyQuestion(q))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].OpenedAt.Equal(out[j].OpenedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].OpenedAt.Before(out[j].OpenedAt)
	})
	return out
}
