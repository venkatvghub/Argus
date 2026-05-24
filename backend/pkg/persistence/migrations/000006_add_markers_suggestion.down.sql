-- SQLite cannot drop columns in older versions; recreate without suggestion.
CREATE TABLE markers_new (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    repo_id   TEXT    NOT NULL,
    file      TEXT    NOT NULL,
    type      TEXT    NOT NULL,
    severity  TEXT    NOT NULL,
    message   TEXT    NOT NULL,
    line      INTEGER NOT NULL DEFAULT 0,
    deduction REAL    NOT NULL DEFAULT 0,
    category  TEXT    NOT NULL DEFAULT ''
);
INSERT INTO markers_new (id, repo_id, file, type, severity, message, line, deduction, category)
SELECT id, repo_id, file, type, severity, message, line, deduction, category FROM markers;
DROP TABLE markers;
ALTER TABLE markers_new RENAME TO markers;
CREATE INDEX IF NOT EXISTS idx_markers_repo_id ON markers(repo_id);
