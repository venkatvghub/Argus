-- SQLite does not support DROP COLUMN in older versions; recreate without updated_at.
CREATE TABLE wiki_pages_new (
    id         TEXT PRIMARY KEY,
    repo_id    TEXT NOT NULL,
    job_id     TEXT NOT NULL,
    type       TEXT NOT NULL,
    subject    TEXT NOT NULL,
    content    TEXT NOT NULL DEFAULT '',
    level      INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);
INSERT INTO wiki_pages_new (id, repo_id, job_id, type, subject, content, level, created_at)
SELECT id, repo_id, job_id, type, subject, content, level, created_at FROM wiki_pages;
DROP TABLE wiki_pages;
ALTER TABLE wiki_pages_new RENAME TO wiki_pages;
CREATE INDEX IF NOT EXISTS idx_wiki_pages_repo_id ON wiki_pages(repo_id);
CREATE INDEX IF NOT EXISTS idx_wiki_pages_job_id ON wiki_pages(job_id);
