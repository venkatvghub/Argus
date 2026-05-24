CREATE TABLE IF NOT EXISTS markers (
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
CREATE INDEX IF NOT EXISTS idx_markers_repo_id ON markers(repo_id);
