CREATE TABLE assets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    container TEXT NOT NULL, 
    path TEXT NOT NULL,      
    etag TEXT,
    sha256 TEXT,
    mime TEXT,
    size_bytes BIGINT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE (container, path)
);

ALTER TABLE messages DROP COLUMN content;
ALTER TABLE messages ADD COLUMN parts JSONB NOT NULL DEFAULT '[]'::jsonb;

CREATE TABLE message_assets (
    message_id UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    asset_id UUID NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
    PRIMARY KEY (message_id, asset_id)
);