-- A command is a script a user keeps once and several templates pull in: a
-- linter wrapper, a release helper, a make shim. The library belongs to the
-- user, so one copy is updated once and every template that references it
-- reads the new version.
--
-- The content shape is the one a template file has (#34): a text command keeps
-- its content in the column, a binary command lives on disk under
-- `<upload-root>/commands/<command-id>/<name>` and the row holds the path. The
-- CHECK keeps the two shapes apart.
--
-- A name is unique per user, not globally: two users may both own `fmt`.

CREATE TABLE commands (
    id           UUID PRIMARY KEY NOT NULL,
    user_id      UUID NOT NULL,
    name         TEXT NOT NULL,
    kind         TEXT NOT NULL DEFAULT 'text'
                 CHECK (kind IN ('text', 'binary')),
    content      TEXT NOT NULL DEFAULT '',
    size         BIGINT NOT NULL DEFAULT 0,
    storage_path TEXT NOT NULL DEFAULT '',
    -- A command is a thing you run unless the user says otherwise.
    is_executable BOOLEAN NOT NULL DEFAULT TRUE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT commands_name_unique UNIQUE (user_id, name),
    CONSTRAINT commands_binary_storage CHECK (
        kind = 'text' OR (storage_path <> '' AND content = '')
    ),
    -- The caps the service enforces are in the schema too: a row that
    -- ignores them is refused by the database, not only by the code path
    -- that happened to be used. 256 KiB of text, 10 MiB of bytes.
    CONSTRAINT commands_text_within_cap CHECK (
        kind <> 'text' OR length(content) <= 262144
    ),
    CONSTRAINT commands_size_within_cap CHECK (size >= 0 AND size <= 10485760)
);

-- The reference: a template pulls a command in at a path inside the project.
--
-- A command can be referenced once per template (one place to put it). A
-- template cannot put two things at one path, so target_path is unique per
-- template as well.
--
-- ON DELETE RESTRICT: a published template does not quietly lose a script
-- because the library entry went away. Deleting a command that a template
-- pulls in is refused until the template stops referencing it; a command that
-- nothing references goes, and the references go with a deleted template
-- (CASCADE) because they have no meaning on their own.
CREATE TABLE template_commands (
    template_id UUID NOT NULL REFERENCES templates (id) ON DELETE CASCADE,
    command_id  UUID NOT NULL REFERENCES commands (id) ON DELETE RESTRICT,
    target_path TEXT NOT NULL,
    PRIMARY KEY (template_id, command_id),
    UNIQUE (template_id, target_path)
);
