package workspace

import (
	"fmt"
	"strings"
)

// DraftRecord is the storage boundary. Generated catalog/snapshot fields may be
// omitted by a store; RestoreDraft recomputes them from the persisted files.
type DraftRecord struct {
	Draft     DraftSnapshot    `json:"draft"`
	Versions  []SourceVersion  `json:"versions"`
	Questions []QuestionRecord `json:"questions"`
}

func (dw *DraftWorkspace) Record() DraftRecord {
	dw.mu.RLock()
	defer dw.mu.RUnlock()
	out := DraftRecord{Draft: dw.snapshotLocked(true), Questions: dw.questionsLocked(true)}
	for id := dw.head; id != ""; id = dw.versions[id].Parent {
		out.Versions = append(out.Versions, copyVersion(dw.versions[id]))
	}
	return out
}

func RestoreDraft(record DraftRecord) (*DraftWorkspace, error) {
	s := record.Draft
	if strings.TrimSpace(s.ID) == "" || strings.TrimSpace(s.Name) == "" || strings.TrimSpace(s.CreatedBy) == "" || s.CreatedAt.IsZero() || s.UpdatedAt.IsZero() || s.Version < 0 {
		return nil, fmt.Errorf("invalid stored draft metadata")
	}
	p, err := load(s.Files)
	if err != nil {
		return nil, fmt.Errorf("stored source: %w", err)
	}
	versions := map[string]SourceVersion{}
	for _, v := range record.Versions {
		vp, err := load(v.Files)
		if err != nil {
			return nil, fmt.Errorf("stored version %s: %w", v.ID, err)
		}
		if vp.Revision != v.SourceRevision || newSourceVersion(v.Parent, vp.Revision, v.Files).ID != v.ID {
			return nil, fmt.Errorf("stored source version hash mismatch")
		}
		versions[v.ID] = copyVersion(v)
	}
	head, ok := versions[s.Revision]
	if !ok || head.SourceRevision != p.Revision {
		return nil, fmt.Errorf("stored head does not match working source")
	}
	seen := map[string]bool{}
	for id := s.Revision; id != ""; {
		v, ok := versions[id]
		if !ok || seen[id] {
			return nil, fmt.Errorf("stored source ancestry is incomplete or cyclic")
		}
		seen[id] = true
		id = v.Parent
	}
	if !seen[s.BaseRevision] {
		return nil, fmt.Errorf("stored base is outside source ancestry")
	}
	if len(s.History) != s.Version {
		return nil, fmt.Errorf("stored activity sequence is incomplete")
	}
	for i, entry := range s.History {
		if entry.Version != i+1 || !seen[entry.Revision] {
			return nil, fmt.Errorf("stored activity does not match source ancestry")
		}
	}
	dw := &DraftWorkspace{id: s.ID, name: s.Name, files: clone(s.Files), baseRevision: s.BaseRevision, head: s.Revision, versions: versions,
		createdAt: s.CreatedAt, updatedAt: s.UpdatedAt, createdBy: s.CreatedBy, version: s.Version, history: append([]DraftEntry{}, s.History...)}
	for i := range dw.history {
		dw.history[i].FilesChanged = append([]string{}, dw.history[i].FilesChanged...)
	}
	if s.StateToken != "" && s.StateToken != dw.stateTokenLocked() {
		return nil, fmt.Errorf("stored working-state token mismatch")
	}
	dw.questions = map[string]QuestionRecord{}
	for _, q := range record.Questions {
		if q.ID != questionID(q.Question) {
			return nil, fmt.Errorf("stored question identity mismatch")
		}
		dw.questions[q.ID] = copyQuestion(q)
	}
	if record.Questions == nil { // A new fork gets its own queue attribution.
		_, snap, err := publish(dw.files, nil)
		if err != nil {
			return nil, err
		}
		dw.reconcileQuestions(snap.Questions, s.CreatedBy, "create")
	} else {
		_, snap, err := publish(dw.files, nil)
		if err != nil {
			return nil, err
		}
		open := map[string]bool{}
		for _, q := range snap.Questions {
			open[questionID(q)] = true
		}
		for id, q := range dw.questions {
			if q.Status == "open" {
				if !open[id] {
					return nil, fmt.Errorf("stored queue differs from working questions")
				}
				delete(open, id)
			}
		}
		if len(open) != 0 {
			return nil, fmt.Errorf("stored queue is missing an open question")
		}
	}
	return dw, nil
}
