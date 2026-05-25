CREATE TABLE IF NOT EXISTS repo_files (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    repo_id TEXT NOT NULL,
    path TEXT NOT NULL,
    language TEXT NOT NULL DEFAULT '',
    churn INTEGER NOT NULL DEFAULT 0,
    ownership REAL NOT NULL DEFAULT 0.0,
    author_count INTEGER NOT NULL DEFAULT 0,
    line_coverage REAL NOT NULL DEFAULT -1.0,
    size INTEGER NOT NULL DEFAULT 0,
    primary_author_last_commit DATETIME,
    UNIQUE(repo_id, path)
);
CREATE INDEX IF NOT EXISTS idx_repo_files_repo_id ON repo_files(repo_id);
CREATE INDEX IF NOT EXISTS idx_repo_files_churn ON repo_files(repo_id, churn DESC);
