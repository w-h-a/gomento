ALTER TABLE files ADD COLUMN embedding vector(1536);
CREATE INDEX ON files USING hnsw (embedding vector_cosine_ops);