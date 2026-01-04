ALTER TABLE files ADD COLUMN space_id UUID REFERENCES spaces(id) ON DELETE CASCADE; -- Nullable
ALTER TABLE files DROP COLUMN artifact_id;

CREATE UNIQUE INDEX ix_files_global_uq ON files (path, filename) WHERE space_id IS NULL;
CREATE UNIQUE INDEX ix_files_space_uq ON files (space_id, path, filename) WHERE space_id IS NOT NULL;

ALTER TABLE spaces DROP COLUMN project_id;
ALTER TABLE sessions DROP COLUMN project_id;

DROP TABLE artifacts;
DROP TABLE projects;