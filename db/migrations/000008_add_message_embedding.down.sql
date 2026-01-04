DROP INDEX IF EXISTS messages_embedding_idx;

ALTER TABLE messages DROP COLUMN IF EXISTS embedding;