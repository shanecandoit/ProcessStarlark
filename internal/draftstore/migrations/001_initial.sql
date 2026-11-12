CREATE TABLE reference_project (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    source_hash TEXT NOT NULL,
    files_json TEXT NOT NULL,
    created_at TEXT NOT NULL
);
CREATE TABLE users (
    id TEXT PRIMARY KEY,
    label TEXT NOT NULL UNIQUE,
    created_at TEXT NOT NULL,
    last_seen_at TEXT NOT NULL
);
CREATE TABLE source_snapshots (
    source_hash TEXT PRIMARY KEY,
    files_json TEXT NOT NULL,
    created_at TEXT NOT NULL
);
CREATE TABLE source_versions (
    id TEXT PRIMARY KEY,
    parent_id TEXT REFERENCES source_versions(id),
    source_hash TEXT NOT NULL REFERENCES source_snapshots(source_hash),
    created_at TEXT NOT NULL
);
CREATE TABLE drafts (
    id TEXT PRIMARY KEY,
    head_id TEXT NOT NULL REFERENCES source_versions(id),
    state_token TEXT NOT NULL,
    state_json TEXT NOT NULL,
    created_by TEXT NOT NULL REFERENCES users(id),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE TABLE draft_versions (
    draft_id TEXT NOT NULL REFERENCES drafts(id),
    version_id TEXT NOT NULL REFERENCES source_versions(id),
    PRIMARY KEY (draft_id, version_id)
);
CREATE TABLE activity (
    draft_id TEXT NOT NULL REFERENCES drafts(id),
    sequence INTEGER NOT NULL,
    actor_id TEXT NOT NULL REFERENCES users(id),
    occurred_at TEXT NOT NULL,
    revision TEXT NOT NULL REFERENCES source_versions(id),
    entry_json TEXT NOT NULL,
    PRIMARY KEY (draft_id, sequence)
);
