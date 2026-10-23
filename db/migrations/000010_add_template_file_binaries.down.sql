ALTER TABLE template_files
    DROP CONSTRAINT template_files_binary_storage,
    DROP COLUMN storage_path,
    DROP COLUMN size,
    DROP COLUMN kind;
