CREATE TABLE projects (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE TABLE artifacts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

ALTER TABLE sessions ADD COLUMN project_id UUID REFERENCES projects(id) ON DELETE CASCADE;
ALTER TABLE spaces ADD COLUMN project_id UUID REFERENCES projects(id) ON DELETE CASCADE;

DROP INDEX ix_files_global_uq;
DROP INDEX ix_files_space_uq;

ALTER TABLE files DROP COLUMN space_id;
ALTER TABLE files ADD COLUMN artifact_id UUID REFERENCES artifacts(id) ON DELETE CASCADE;