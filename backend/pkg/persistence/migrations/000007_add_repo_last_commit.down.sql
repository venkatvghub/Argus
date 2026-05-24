-- SQLite does not support DROP COLUMN on older versions; recreate table without last_commit.
CREATE TABLE repositories_backup AS SELECT id, name, local_path, created_at, updated_at FROM repositories;
DROP TABLE repositories;
ALTER TABLE repositories_backup RENAME TO repositories;
