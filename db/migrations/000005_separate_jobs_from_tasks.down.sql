ALTER TABLE tasks DROP CONSTRAINT uq_session_task_order;
ALTER TABLE tasks DROP COLUMN is_thought;
DROP TABLE jobs;