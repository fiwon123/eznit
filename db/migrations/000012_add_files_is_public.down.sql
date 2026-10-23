DROP INDEX IF EXISTS idx_files_is_public;
ALTER TABLE files DROP COLUMN is_public;
