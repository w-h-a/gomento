CREATE TABLE tasks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id UUID NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    task_order INTEGER NOT NULL DEFAULT 0,
    data JSONB NOT NULL DEFAULT '{}',
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

    CONSTRAINT ck_task_status CHECK (status IN ('pending', 'running', 'success', 'failed'))
);

CREATE INDEX ix_tasks_session_id ON tasks (session_id);
CREATE INDEX ix_tasks_session_status ON tasks (session_id, status);

ALTER TABLE messages ADD COLUMN task_id UUID REFERENCES tasks(id) ON DELETE SET NULL;
CREATE INDEX ix_messages_task_id ON messages (task_id);