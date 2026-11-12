CREATE TABLE questions (
    draft_id TEXT NOT NULL REFERENCES drafts(id),
    id TEXT NOT NULL,
    function TEXT NOT NULL,
    call TEXT NOT NULL,
    question_json TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('open', 'resolved', 'dismissed')),
    opened_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    closed_at TEXT,
    opened_by TEXT NOT NULL REFERENCES users(id),
    closed_by TEXT REFERENCES users(id),
    PRIMARY KEY (draft_id, id)
);
CREATE INDEX questions_by_status ON questions(draft_id, status, opened_at, id);
