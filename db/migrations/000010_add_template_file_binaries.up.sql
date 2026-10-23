-- A template file is text or binary, and the bytes of a binary file do not
-- belong in a TEXT column: an image, a compiled helper or a script with a
-- non-UTF-8 byte in it would be refused by Postgres or mangled by the driver.
--
-- So a binary file is declared in the row and stored on disk, under
-- `<upload-root>/templates/<template-id>/<file-id>/<name>`; `storage_path` is
-- that location, relative to the upload root and always written with forward
-- slashes so a template published on one machine reads on another.
--
-- The CHECK is the invariant in the schema, not only in the service: a binary
-- row points at a file and holds no content, a text row holds its content and
-- points at nothing.

ALTER TABLE template_files
    ADD COLUMN kind         TEXT   NOT NULL DEFAULT 'text'
                                CHECK (kind IN ('text', 'binary')),
    ADD COLUMN size         BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN storage_path TEXT   NOT NULL DEFAULT '';

ALTER TABLE template_files
    ADD CONSTRAINT template_files_binary_storage CHECK (
        kind = 'text' OR (storage_path <> '' AND content = '')
    );
