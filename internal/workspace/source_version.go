package workspace

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// SourceVersion is an immutable whole-project source snapshot. Questions,
// generated catalogs, and activity counters are deliberately excluded.
type SourceVersion struct {
	ID             string `json:"id"`
	Parent         string `json:"parent"`
	SourceRevision string `json:"source_revision"`
	Files          Files  `json:"files"`
}

func sourceFiles(files Files) Files {
	out := Files{}
	for name, text := range files {
		if strings.HasSuffix(name, ".star") {
			out[name] = text
		}
	}
	return out
}

func digest(value any) string {
	// All callers use JSON-compatible structs/maps; encoding/json sorts map keys.
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

func newSourceVersion(parent, revision string, files Files) SourceVersion {
	manifest := struct {
		Schema, Parent, SourceRevision string
	}{"park-source-version-v1", parent, revision}
	return SourceVersion{ID: digest(manifest), Parent: parent, SourceRevision: revision, Files: sourceFiles(files)}
}

func copyVersion(v SourceVersion) SourceVersion {
	v.Files = clone(v.Files)
	return v
}

func (dw *DraftWorkspace) stateTokenLocked() string {
	return digest(struct {
		Schema  string
		Draft   string
		Created time.Time
		Head    string
		Serial  int
		Files   Files
	}{"park-draft-state-v1", dw.id, dw.createdAt, dw.head, dw.version, dw.files})
}

// Versions returns the current head followed by its ancestors. Every result
// owns its files; callers cannot mutate stored versions through returned maps.
func (dw *DraftWorkspace) Versions() []SourceVersion {
	dw.mu.RLock()
	defer dw.mu.RUnlock()
	versions := []SourceVersion{}
	for id := dw.head; id != ""; id = dw.versions[id].Parent {
		versions = append(versions, copyVersion(dw.versions[id]))
	}
	return versions
}

func (dw *DraftWorkspace) SourceVersion(id string) (SourceVersion, bool) {
	dw.mu.RLock()
	defer dw.mu.RUnlock()
	v, ok := dw.versions[id]
	return copyVersion(v), ok
}
