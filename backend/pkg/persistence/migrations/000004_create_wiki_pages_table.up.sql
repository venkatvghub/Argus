CREATE TABLE IF NOT EXISTS wiki_pages (
    id         TEXT PRIMARY KEY,
    repo_id    TEXT NOT NULL,
    job_id     TEXT NOT NULL,
    type       TEXT NOT NULL,
    subject    TEXT NOT NULL,
    content    TEXT NOT NULL DEFAULT '',
    level      INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_wiki_pages_repo_id ON wiki_pages(repo_id);
CREATE INDEX IF NOT EXISTS idx_wiki_pages_job_id ON wiki_pages(job_id);
