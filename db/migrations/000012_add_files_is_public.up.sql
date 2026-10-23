ALTER TABLE files ADD COLUMN is_public BOOLEAN NOT NULL DEFAULT FALSE;
CREATE INDEX idx_files_is_public ON files (is_public) WHERE is_public;
