CREATE TABLE IF NOT EXISTS repo_symbols (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    repo_id TEXT NOT NULL,
    name TEXT NOT NULL,
    type TEXT NOT NULL DEFAULT '',
    file_path TEXT NOT NULL DEFAULT '',
    line INTEGER NOT NULL DEFAULT 0,
    end_line INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_repo_symbols_repo_id ON repo_symbols(repo_id);
