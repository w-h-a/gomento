CREATE TABLE jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    type VARCHAR(50) NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}',
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

    CONSTRAINT ck_job_status CHECK (status IN ('pending', 'running', 'success', 'failed'))
);

CREATE INDEX ix_jobs_status ON jobs (status);

ALTER TABLE tasks ADD COLUMN is_thought BOOLEAN NOT NULL DEFAULT FALSE;

CREATE UNIQUE INDEX uq_session_task_order 
ON tasks (session_id, task_order) 
WHERE is_thought = FALSE;