package workspace

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/shanecandoit/ProcessStarlark/internal/engine"
	"github.com/shanecandoit/ProcessStarlark/internal/project"
	"github.com/shanecandoit/ProcessStarlark/internal/samples"
)

// DraftWorkspace is an in-memory, mutable workspace for draft editing.
// Unlike the stateless workspace operations, it maintains state across calls
// and supports branching, history, and concurrent access.
type DraftWorkspace struct {
	mu           sync.RWMutex
	id           string
	name         string
	files        Files
	baseRevision string // Revision of the production version this draft is based on
	createdAt    time.Time
	updatedAt    time.Time
	createdBy    string
	head         string
	versions     map[string]SourceVersion
	version      int          // Deprecated API compatibility: accepted operation count
	history      []DraftEntry // Change history
}

// DraftEntry records a single change to the draft.
type DraftEntry struct {
	Version      int       `json:"version"`
	Timestamp    time.Time `json:"timestamp"`
	Author       string    `json:"author"`
	Message      string    `json:"message"`
	FilesChanged []string  `json:"files_changed"`
	Revision     string    `json:"revision"` // Project revision after this change
}

// DraftSnapshot is the state of a draft workspace for display/API.
type DraftSnapshot struct {
	ID             string       `json:"id"`
	Name           string       `json:"name"`
	BaseRevision   string       `json:"base_revision"`
	Revision       string       `json:"revision"`
	SourceRevision string       `json:"source_revision"`
	StateToken     string       `json:"state_token"`
	Version        int          `json:"version"` // Deprecated: operation count, not source identity
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
	CreatedBy      string       `json:"created_by"`
	Files          Files        `json:"files"`
	Snapshot       Snapshot     `json:"snapshot"`
	History        []DraftEntry `json:"history,omitempty"`
}

// NewDraftWorkspace creates a new draft workspace from production files.
func NewDraftWorkspace(id, name, createdBy string, prodFiles Files) (*DraftWorkspace, error) {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(name) == "" || strings.TrimSpace(createdBy) == "" {
		return nil, fmt.Errorf("draft id, name, and author are required")
	}
	// Load and validate the production files to get base revision
	p, err := project.FromFiles("", modules(prodFiles))
	if err != nil {
		return nil, fmt.Errorf("invalid production files: %w", err)
	}
	_, err = engine.New(p)
	if err != nil {
		return nil, fmt.Errorf("production engine load failed: %w", err)
	}

	now := time.Now().UTC()
	dw := &DraftWorkspace{
		id:           id,
		name:         name,
		files:        clone(prodFiles),
		baseRevision: p.Revision,
		createdAt:    now,
		updatedAt:    now,
		createdBy:    createdBy,
		version:      0,
		history:      []DraftEntry{},
	}

	// Initial snapshot
	next, _, err := publish(dw.files, nil)
	if err != nil {
		return nil, fmt.Errorf("initial publish failed: %w", err)
	}
	dw.files = next
	initial := newSourceVersion("", p.Revision, dw.files)
	dw.head, dw.baseRevision = initial.ID, initial.ID
	dw.versions = map[string]SourceVersion{initial.ID: initial}

	return dw, nil
}

// Fork creates a new draft workspace from this draft's current state.
func (dw *DraftWorkspace) Fork(newID, newName, author string) (*DraftWorkspace, error) {
	return dw.ForkAt(newID, newName, author, "")
}

// ForkAt branches from a retained source version. An empty revision branches
// from the current working state, including its questions.
func (dw *DraftWorkspace) ForkAt(newID, newName, author, revision string) (*DraftWorkspace, error) {
	dw.mu.RLock()
	files := clone(dw.files)
	baseRev := dw.baseRevision
	head := dw.head
	if revision != "" {
		v, ok := dw.versions[revision]
		if !ok {
			dw.mu.RUnlock()
			return nil, fmt.Errorf("source version not found")
		}
		files, head = clone(v.Files), v.ID
	}
	versions := map[string]SourceVersion{}
	// Copy only the selected ancestry, not abandoned branches.
	for id := head; id != ""; id = dw.versions[id].Parent {
		versions[id] = copyVersion(dw.versions[id])
	}
	dw.mu.RUnlock()

	dw2, err := NewDraftWorkspace(newID, newName, author, files)
	if err != nil {
		return nil, err
	}
	// Preserve the original base revision
	dw2.mu.Lock()
	dw2.baseRevision = baseRev
	dw2.head, dw2.versions = head, versions
	dw2.mu.Unlock()
	return dw2, nil
}

// GetSnapshot returns an independent copy, including queued questions.
func (dw *DraftWorkspace) GetSnapshot(includeHistory bool) DraftSnapshot {
	dw.mu.RLock()
	defer dw.mu.RUnlock()
	return dw.snapshotLocked(includeHistory)
}

func (dw *DraftWorkspace) snapshotLocked(includeHistory bool) DraftSnapshot {
	_, snap, err := publish(dw.files, nil)
	if err != nil {
		snap = Snapshot{Error: err.Error()}
	}
	out := DraftSnapshot{ID: dw.id, Name: dw.name, BaseRevision: dw.baseRevision,
		Revision: dw.head, SourceRevision: snap.Revision, StateToken: dw.stateTokenLocked(), Version: dw.version, CreatedAt: dw.createdAt,
		UpdatedAt: dw.updatedAt, CreatedBy: dw.createdBy, Files: clone(dw.files), Snapshot: snap}
	if includeHistory {
		out.History = append([]DraftEntry{}, dw.history...)
		for i := range out.History {
			out.History[i].FilesChanged = append([]string{}, out.History[i].FilesChanged...)
		}
	}
	return out
}

// ErrDraftConflict means another accepted operation advanced this draft.
var ErrDraftConflict = errors.New("draft state conflict")

// DraftOperation is one atomic operation. Files and Delete affect source modules;
// queue changes are produced by run/test/example operations, not edited directly.
type DraftOperation struct {
	Kind    string   `json:"kind"`
	Files   Files    `json:"files,omitempty"`
	Delete  []string `json:"delete,omitempty"`
	Message string   `json:"message,omitempty"`
	Call    string   `json:"call,omitempty"`
	Example string   `json:"example,omitempty"`
	Only    string   `json:"only,omitempty"`
}

// Apply checks the expected operation version and runs against that exact state.
// The lock spans evaluation and commit so stale evaluations cannot replace edits.
// Deprecated: use ApplyState with the snapshot's opaque StateToken.
func (dw *DraftWorkspace) Apply(ctx context.Context, author string, expectedVersion int, op DraftOperation) (Result, DraftSnapshot, error) {
	return dw.applyExpected(ctx, author, &expectedVersion, "", op)
}

// ApplyState checks an opaque hash token, including non-source working state.
func (dw *DraftWorkspace) ApplyState(ctx context.Context, author, expectedState string, op DraftOperation) (Result, DraftSnapshot, error) {
	if expectedState == "" {
		return Result{}, dw.GetSnapshot(false), fmt.Errorf("expected_state is required")
	}
	return dw.applyExpected(ctx, author, nil, expectedState, op)
}

func (dw *DraftWorkspace) apply(ctx context.Context, author string, expected *int, op DraftOperation) (Result, DraftSnapshot, error) {
	return dw.applyExpected(ctx, author, expected, "", op)
}

func (dw *DraftWorkspace) applyExpected(ctx context.Context, author string, expected *int, token string, op DraftOperation) (Result, DraftSnapshot, error) {
	dw.mu.Lock()
	defer dw.mu.Unlock()
	if strings.TrimSpace(author) == "" {
		return Result{}, dw.snapshotLocked(false), fmt.Errorf("author is required")
	}
	if expected != nil && *expected != dw.version {
		return Result{}, dw.snapshotLocked(false), fmt.Errorf("%w: expected %d, current %d", ErrDraftConflict, *expected, dw.version)
	}
	if token != "" && token != dw.stateTokenLocked() {
		return Result{}, dw.snapshotLocked(false), fmt.Errorf("%w: working state changed; fetch and review the latest draft", ErrDraftConflict)
	}
	var result Result
	var message string
	switch op.Kind {
	case "files":
		next := clone(dw.files)
		for name, text := range op.Files {
			if !draftModuleName(name) {
				return Result{}, dw.snapshotLocked(false), fmt.Errorf("invalid source file %q", name)
			}
			next[name] = text
		}
		for _, name := range op.Delete {
			if !draftModuleName(name) {
				return Result{}, dw.snapshotLocked(false), fmt.Errorf("invalid source file %q", name)
			}
			if _, exists := op.Files[name]; exists {
				return Result{}, dw.snapshotLocked(false), fmt.Errorf("cannot edit and delete %q together", name)
			}
			if _, exists := next[name]; !exists {
				return Result{}, dw.snapshotLocked(false), fmt.Errorf("source file %q does not exist", name)
			}
			delete(next, name)
		}
		result, message = Load(next), op.Message
		if message == "" {
			message = "Edit source files"
		}
	case "run":
		result, message = Run(ctx, dw.files, op.Call), "Run: "+op.Call
	case "test":
		result, message = Test(ctx, dw.files, op.Only), "Test: "+op.Only
	case "add-example":
		result, message = AddExample(dw.files, op.Example), "Add example: "+op.Example
	case "remove-example":
		result, message = RemoveExample(dw.files, op.Call), "Remove example: "+op.Call
	case "load":
		result, message = Load(dw.files), "Refresh draft"
	default:
		return Result{}, dw.snapshotLocked(false), fmt.Errorf("unknown draft operation %q", op.Kind)
	}
	if result.Status == Error {
		return result, dw.snapshotLocked(true), nil
	}
	changed := changedFiles(dw.files, result.Files)
	dw.files = clone(result.Files)
	if dw.versions[dw.head].SourceRevision != result.Snapshot.Revision {
		v := newSourceVersion(dw.head, result.Snapshot.Revision, dw.files)
		dw.versions[v.ID], dw.head = v, v.ID
	}
	dw.version++
	dw.updatedAt = time.Now().UTC()
	dw.history = append(dw.history, DraftEntry{Version: dw.version, Timestamp: dw.updatedAt,
		Author: author, Message: message, FilesChanged: changed, Revision: dw.head})
	result.Files = clone(dw.files)
	return result, dw.snapshotLocked(true), nil
}

func draftModuleName(name string) bool {
	return strings.HasSuffix(name, ".star") && len(name) > len(".star") && !strings.ContainsAny(name, "/\\")
}

func changedFiles(before, after Files) []string {
	changed := []string{}
	for name, text := range after {
		if old, ok := before[name]; !ok || old != text {
			changed = append(changed, name)
		}
	}
	for name := range before {
		if _, ok := after[name]; !ok {
			changed = append(changed, name)
		}
	}
	sort.Strings(changed)
	return changed
}

func (dw *DraftWorkspace) ApplyEdits(author string, edits Files, message string) (DraftSnapshot, error) {
	result, snap, err := dw.apply(context.Background(), author, nil, DraftOperation{Kind: "files", Files: edits, Message: message})
	if err == nil && result.Status == Error {
		err = errors.New(result.Message)
	}
	return snap, err
}
func (dw *DraftWorkspace) Run(ctx context.Context, author, call string) (Result, DraftSnapshot, error) {
	return dw.apply(ctx, author, nil, DraftOperation{Kind: "run", Call: call})
}
func (dw *DraftWorkspace) Test(ctx context.Context, author, only string) (Result, DraftSnapshot, error) {
	return dw.apply(ctx, author, nil, DraftOperation{Kind: "test", Only: only})
}
func (dw *DraftWorkspace) AddExample(author, text string) (Result, DraftSnapshot, error) {
	return dw.apply(context.Background(), author, nil, DraftOperation{Kind: "add-example", Example: text})
}
func (dw *DraftWorkspace) RemoveExample(author, call string) (Result, DraftSnapshot, error) {
	return dw.apply(context.Background(), author, nil, DraftOperation{Kind: "remove-example", Call: call})
}
func (dw *DraftWorkspace) Load() (DraftSnapshot, error) {
	dw.mu.Lock()
	defer dw.mu.Unlock()
	return dw.snapshotLocked(false), nil
}

// Diff compares source files and complete function hashes against a supplied
// base. It is deterministic and returns errors rather than an empty success.
func (dw *DraftWorkspace) Diff(prodFiles Files) (DraftDiff, error) {
	dw.mu.RLock()
	files := clone(dw.files)
	version := dw.version
	head := dw.head
	dw.mu.RUnlock()
	base, err := load(prodFiles)
	if err != nil {
		return DraftDiff{}, err
	}
	draft, err := load(files)
	if err != nil {
		return DraftDiff{}, err
	}
	baseVersion := newSourceVersion("", base.Revision, prodFiles)
	diff := DraftDiff{BaseRevision: baseVersion.ID, DraftRevision: head, DraftVersion: version,
		FilesChanged: []string{}, FilesDeleted: []string{}}
	for name, text := range files {
		if !strings.HasSuffix(name, ".star") {
			continue
		}
		if old, ok := prodFiles[name]; !ok || old != text {
			diff.FilesChanged = append(diff.FilesChanged, name)
		}
	}
	for name := range prodFiles {
		if !strings.HasSuffix(name, ".star") {
			continue
		}
		if _, ok := files[name]; !ok {
			diff.FilesDeleted = append(diff.FilesDeleted, name)
		}
	}
	sort.Strings(diff.FilesChanged)
	sort.Strings(diff.FilesDeleted)
	diff.FunctionChanges, err = computeFunctionChanges(base, draft)
	return diff, err
}

// DraftDiff describes the differences between draft and production.
type DraftDiff struct {
	DraftVersion    int              `json:"draft_version"`
	BaseRevision    string           `json:"base_revision"`
	DraftRevision   string           `json:"draft_revision"`
	FilesChanged    []string         `json:"files_changed"`
	FilesDeleted    []string         `json:"files_deleted"`
	FunctionChanges []FunctionChange `json:"function_changes"`
}

type FunctionChange struct {
	Qualified       string `json:"qualified"`
	OldHash         string `json:"old_hash,omitempty"`
	NewHash         string `json:"new_hash,omitempty"`
	OldState        string `json:"old_state,omitempty"`
	NewState        string `json:"new_state,omitempty"`
	ExamplesAdded   int    `json:"examples_added"`
	ExamplesRemoved int    `json:"examples_removed"`
	BodyChanged     bool   `json:"body_changed"`
}

func computeFunctionChanges(base, draft *project.Project) ([]FunctionChange, error) {
	oldVersions, err := engine.ComputeAllFunctionHashes(base)
	if err != nil {
		return nil, err
	}
	newVersions, err := engine.ComputeAllFunctionHashes(draft)
	if err != nil {
		return nil, err
	}
	names := map[string]bool{}
	for name := range oldVersions {
		names[name] = true
	}
	for name := range newVersions {
		names[name] = true
	}
	changes := []FunctionChange{}
	for _, name := range project.Names(names) {
		old, next := oldVersions[name], newVersions[name]
		if old != nil && next != nil && old.Hash == next.Hash {
			continue
		}
		before, after := base.FindFunction(name), draft.FindFunction(name)
		change := FunctionChange{Qualified: name}
		oldExamples, newExamples := map[string]string{}, map[string]string{}
		if old != nil {
			change.OldHash, change.OldState = old.Hash, before.State()
			for _, ex := range before.Examples {
				oldExamples[samples.Key(ex.Input)] = samples.Key(ex.Output)
			}
		}
		if next != nil {
			change.NewHash, change.NewState = next.Hash, after.State()
			for _, ex := range after.Examples {
				newExamples[samples.Key(ex.Input)] = samples.Key(ex.Output)
			}
		}
		for key, value := range newExamples {
			if previous, ok := oldExamples[key]; !ok || previous != value {
				change.ExamplesAdded++
			}
		}
		for key, value := range oldExamples {
			if current, ok := newExamples[key]; !ok || current != value {
				change.ExamplesRemoved++
			}
		}
		change.BodyChanged = base.Body(name) != draft.Body(name)
		changes = append(changes, change)
	}
	return changes, nil
}

// DraftManager manages multiple draft workspaces.
type DraftManager struct {
	mu     sync.RWMutex
	drafts map[string]*DraftWorkspace
}

func NewDraftManager() *DraftManager {
	return &DraftManager{drafts: map[string]*DraftWorkspace{}}
}

func (dm *DraftManager) Create(id, name, author string, prodFiles Files) (*DraftWorkspace, error) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if _, exists := dm.drafts[id]; exists {
		return nil, fmt.Errorf("draft %s already exists", id)
	}
	dw, err := NewDraftWorkspace(id, name, author, prodFiles)
	if err != nil {
		return nil, err
	}
	dm.drafts[id] = dw
	return dw, nil
}

func (dm *DraftManager) Get(id string) (*DraftWorkspace, bool) {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	dw, ok := dm.drafts[id]
	return dw, ok
}

// Fork registers an independent copy while preserving the original base revision.
func (dm *DraftManager) Fork(from, id, name, author string) (*DraftWorkspace, error) {
	return dm.ForkAt(from, id, name, author, "")
}

func (dm *DraftManager) ForkAt(from, id, name, author, revision string) (*DraftWorkspace, error) {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	if _, exists := dm.drafts[id]; exists {
		return nil, fmt.Errorf("draft %s already exists", id)
	}
	parent, ok := dm.drafts[from]
	if !ok {
		return nil, fmt.Errorf("draft %s does not exist", from)
	}
	draft, err := parent.ForkAt(id, name, author, revision)
	if err != nil {
		return nil, err
	}
	dm.drafts[id] = draft
	return draft, nil
}

func (dm *DraftManager) Delete(id string) bool {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	if _, ok := dm.drafts[id]; ok {
		delete(dm.drafts, id)
		return true
	}
	return false
}

func (dm *DraftManager) List() []DraftSnapshot {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	out := []DraftSnapshot{}
	for _, dw := range dm.drafts {
		out = append(out, dw.GetSnapshot(false))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
