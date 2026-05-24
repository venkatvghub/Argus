CREATE TABLE IF NOT EXISTS wiki_jobs (
    id          TEXT    PRIMARY KEY,
    repo_id     TEXT    NOT NULL,
    status      TEXT    NOT NULL DEFAULT 'pending',
    total_pages INTEGER NOT NULL DEFAULT 0,
    created_at  TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at  TEXT    NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS wiki_job_pages (
    job_id  TEXT    NOT NULL REFERENCES wiki_jobs(id) ON DELETE CASCADE,
    page_id TEXT    NOT NULL,
    PRIMARY KEY (job_id, page_id)
);

CREATE INDEX IF NOT EXISTS idx_wiki_jobs_repo_id ON wiki_jobs(repo_id);
CREATE INDEX IF NOT EXISTS idx_wiki_job_pages_job_id ON wiki_job_pages(job_id);
