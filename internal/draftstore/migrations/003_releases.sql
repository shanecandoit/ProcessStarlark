-- One local project per database. Immutable manifests and the generation pointer
-- commit in the same immediate transaction as review decisions.
CREATE TABLE release_workflow (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    state_json TEXT NOT NULL
);
INSERT INTO release_workflow VALUES (1, '{"reviews":{},"releases":{},"production":{"release_id":"","generation":0},"activations":[]}');
